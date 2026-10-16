// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

import (
	"context"
	"errors"
	"fmt"
	"html"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/therealmcsparrow/mcharbor/core/plugin"
	"github.com/therealmcsparrow/mcharbor/core/plugin/specs/incident"
)

// Client is stateless apart from its transport and clock, which tests replace.
type Client struct {
	http *http.Client
	now  func() time.Time
}

// NewClient returns a Client on hc, or on a client that never follows redirects when hc is nil,
// because a 307/308 would replay the credential-bearing form body to the redirect's host.
func NewClient(hc *http.Client) *Client {
	if hc == nil {
		hc = &http.Client{
			Timeout:       35 * time.Second,
			CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
		}
	}
	return &Client{http: hc, now: time.Now}
}

var (
	_ plugin.NotificationAdapterClient = (*Client)(nil)
	_ incident.LifecycleClient         = (*Client)(nil)
	_ incident.WebhookParser           = (*Client)(nil)
	_ incident.Poller                  = (*Client)(nil)
)

// openStatuses excludes every terminal state of both classes, so a marker search never revives
// a ticket a human already finished with.
const openStatuses = "status NOT IN ('resolved','closed','rejected')"

// needsAssign lists the states from which ev_resolve is illegal until ev_assign has run.
var needsAssign = map[string]bool{
	"new": true, "escalated_tto": true, "pending": true, "dispatched": true, "redispatched": true, "approved": true,
}

// Test proves the credentials, the organization, the caller and the assignment trio all exist in
// iTop, and creates nothing, so a green test means Open and Resolve will not be refused.
func (c *Client) Test(ctx context.Context, raw map[string]string) error {
	cfg, err := parseConfig(raw)
	if err != nil {
		return err
	}
	if _, err := c.rest(ctx, cfg, map[string]any{"operation": "list_operations"}); err != nil {
		return err
	}
	if err := c.expectOne(ctx, cfg, "Organization", "SELECT Organization WHERE name = "+oqlQuote(cfg.OrgName), "organization "+strconv.Quote(cfg.OrgName)); err != nil {
		return err
	}
	if cfg.CallerEmail != "" {
		if err := c.expectOne(ctx, cfg, "Person", "SELECT Person WHERE email = "+oqlQuote(cfg.CallerEmail), "caller "+strconv.Quote(cfg.CallerEmail)); err != nil {
			return err
		}
	}
	checks := []struct {
		class string
		id    int
		what  string
	}{
		{"Team", cfg.TeamID, "team_id"},
		{"Person", cfg.AgentID, "agent_id"},
		{"Service", cfg.ServiceID, "service_id"},
	}
	if cfg.CreateTeamID > 0 {
		checks = append(checks, struct {
			class string
			id    int
			what  string
		}{"Team", cfg.CreateTeamID, "default_assignment_group"})
	}
	for _, chk := range checks {
		oql := "SELECT " + chk.class + " WHERE id = " + strconv.Itoa(chk.id)
		if err := c.expectOne(ctx, cfg, chk.class, oql, chk.what+" "+strconv.Itoa(chk.id)); err != nil {
			return err
		}
	}
	links, err := c.get(ctx, cfg, "lnkPersonToTeam",
		"SELECT lnkPersonToTeam WHERE team_id = "+strconv.Itoa(cfg.TeamID)+" AND person_id = "+strconv.Itoa(cfg.AgentID), "id")
	if err != nil {
		return err
	}
	if len(links) == 0 {
		return operatorError("agent %d is not a member of team %d; iTop refuses to assign a ticket to them", cfg.AgentID, cfg.TeamID)
	}
	return nil
}

func (c *Client) expectOne(ctx context.Context, cfg itopConfig, class, oql, what string) error {
	objs, err := c.get(ctx, cfg, class, oql, "id")
	if err != nil {
		return err
	}
	if len(objs) != 1 {
		return operatorError("iTop found %d objects for %s, expected exactly 1", len(objs), what)
	}
	return nil
}

// Send is the plain-notification fallback: it creates a ticket without a marker, so it is never
// deduplicated or resolved by the incident lifecycle.
func (c *Client) Send(ctx context.Context, raw map[string]string, msg plugin.NotificationMessage) error {
	cfg, err := parseConfig(raw)
	if err != nil {
		return err
	}
	sev := incident.SeverityInfo
	switch msg.Level {
	case "error":
		sev = incident.SeverityCritical
	case "warning":
		sev = incident.SeverityWarning
	}
	title := strings.TrimSpace(msg.Title)
	if title == "" {
		title = "McHarbor notification"
	}
	body := strings.TrimSpace(msg.Message)
	if body == "" {
		body = title
	}
	desc := "<p>" + strings.ReplaceAll(html.EscapeString(body), "\n", "<br>") + "</p>"
	_, err = c.create(ctx, cfg, truncateRunes(title, maxTitleRunes), desc, sev)
	return err
}

// Open returns the open ticket carrying ev's marker, reopens a recently resolved one when the
// reopen window allows, and only otherwise creates one, so a retried Open never duplicates.
func (c *Client) Open(ctx context.Context, raw map[string]string, ev incident.Event) (incident.Ref, error) {
	cfg, err := parseConfig(raw)
	if err != nil {
		return incident.Ref{}, err
	}
	if err := ev.Validate(); err != nil {
		return incident.Ref{}, err
	}
	m := marker(ev.DedupKey)
	like := "title LIKE " + oqlQuote("%"+m+"%")
	objs, err := c.get(ctx, cfg, cfg.Class, "SELECT "+cfg.Class+" WHERE "+like+" AND "+openStatuses, "id,ref,status,title")
	if err != nil {
		return incident.Ref{}, err
	}
	if o, ok := firstMarked(objs, m); ok {
		return c.ref(cfg, o), nil
	}
	if cfg.ReopenWindow > 0 {
		ref, ok, err := c.reopen(ctx, cfg, like, m, ev)
		if err != nil || ok {
			return ref, err
		}
	}
	o, err := c.create(ctx, cfg, markerTitle(m, ev.Title), describeHTML(ev), ev.Severity)
	if err != nil {
		return incident.Ref{}, err
	}
	return c.ref(cfg, o), nil
}

// reopen applies ev_reopen to a marker ticket resolved inside the window; the window is measured
// by iTop's own clock, so the two servers' time zones never need to agree.
func (c *Client) reopen(ctx context.Context, cfg itopConfig, like, m string, ev incident.Event) (incident.Ref, bool, error) {
	oql := "SELECT " + cfg.Class + " WHERE " + like + " AND status = 'resolved' AND resolution_date > DATE_SUB(NOW(), INTERVAL " +
		strconv.Itoa(cfg.ReopenWindow) + " MINUTE)"
	objs, err := c.get(ctx, cfg, cfg.Class, oql, "id,ref,status,title")
	if err != nil {
		return incident.Ref{}, false, err
	}
	o, ok := firstMarked(objs, m)
	if !ok {
		return incident.Ref{}, false, nil
	}
	id, err := strconv.Atoi(o.Key.String())
	if err != nil {
		return incident.Ref{}, false, nil
	}
	err = c.stimulus(ctx, cfg, id, "ev_reopen", map[string]any{"private_log": logItem("Reopened by McHarbor: " + ev.Title + "\n" + describeText(ev))})
	if errors.Is(err, incident.ErrOperatorMessage) {
		return incident.Ref{}, false, nil
	}
	if err != nil {
		return incident.Ref{}, false, err
	}
	return c.ref(cfg, o), true, nil
}

func firstMarked(objs []restObject, m string) (restObject, bool) {
	for _, o := range objs {
		if strings.HasPrefix(o.field("title"), m) {
			return o, true
		}
	}
	return restObject{}, false
}

func (c *Client) create(ctx context.Context, cfg itopConfig, title, descHTML string, sev incident.Severity) (restObject, error) {
	sf, ok := cfg.Severity[sev]
	if !ok {
		sf = cfg.Severity[incident.SeverityInfo]
	}
	fields := map[string]any{
		"org_id":      "SELECT Organization WHERE name = " + oqlQuote(cfg.OrgName),
		"title":       title,
		"description": descHTML,
		"impact":      sf.Impact,
		"urgency":     sf.Urgency,
		"origin":      cfg.Origin,
	}
	if cfg.CallerEmail != "" {
		fields["caller_id"] = "SELECT Person WHERE email = " + oqlQuote(cfg.CallerEmail)
	}
	if cfg.CreateTeamID > 0 {
		fields["team_id"] = cfg.CreateTeamID
	}
	resp, err := c.rest(ctx, cfg, map[string]any{
		"operation": "core/create", "class": cfg.Class, "comment": "Created by McHarbor",
		"output_fields": "id,ref,status", "fields": fields,
	})
	if err != nil {
		return restObject{}, err
	}
	objs := resp.sorted()
	if len(objs) == 0 {
		return restObject{}, operatorError("iTop accepted the create but returned no %s", cfg.Class)
	}
	return objs[0], nil
}

func (c *Client) ref(cfg itopConfig, o restObject) incident.Ref {
	id := o.Key.String()
	if id == "" {
		id = o.field("id")
	}
	return incident.Ref{ExternalID: id, Number: o.field("ref"), URL: ticketURL(cfg.BaseURL, cfg.Class, id)}
}

// ticketID refuses anything but a positive integer, because the id is placed into OQL and a
// link row is the one input the operator does not type.
func ticketID(ref incident.Ref) (int, error) {
	id, err := strconv.Atoi(strings.TrimSpace(ref.ExternalID))
	if err != nil || id <= 0 {
		return 0, operatorError("the linked iTop ticket id %q is not a number", sanitize(ref.ExternalID))
	}
	return id, nil
}

func logItem(msg string) map[string]any {
	return map[string]any{"add_item": map[string]any{"message": msg, "format": "text"}}
}

// Update appends the repeat to the private log; a refused update is checked against iTop so a
// deleted ticket orphans the link instead of parking the op.
func (c *Client) Update(ctx context.Context, raw map[string]string, ref incident.Ref, ev incident.Event) error {
	cfg, err := parseConfig(raw)
	if err != nil {
		return err
	}
	id, err := ticketID(ref)
	if err != nil {
		return err
	}
	_, err = c.rest(ctx, cfg, map[string]any{
		"operation": "core/update", "class": cfg.Class, "key": id, "comment": "McHarbor",
		"output_fields": "id,status", "fields": map[string]any{"private_log": logItem(ev.Title + "\n" + describeText(ev))},
	})
	if err != nil && errors.Is(err, incident.ErrOperatorMessage) {
		if objs, gerr := c.get(ctx, cfg, cfg.Class, "SELECT "+cfg.Class+" WHERE id = "+strconv.Itoa(id), "id"); gerr == nil && len(objs) == 0 {
			return fmt.Errorf("comm-itop: %s %d: %w", cfg.Class, id, incident.ErrNotFound)
		}
	}
	return err
}

// Resolve reads the status first, returns nil when a human already finished the ticket, assigns
// a still-unassigned one, then resolves with the fields iTop makes mandatory in that state.
func (c *Client) Resolve(ctx context.Context, raw map[string]string, ref incident.Ref, ev incident.Event) error {
	cfg, err := parseConfig(raw)
	if err != nil {
		return err
	}
	id, err := ticketID(ref)
	if err != nil {
		return err
	}
	objs, err := c.get(ctx, cfg, cfg.Class, "SELECT "+cfg.Class+" WHERE id = "+strconv.Itoa(id), "id,ref,status")
	if err != nil {
		return err
	}
	if len(objs) == 0 {
		return fmt.Errorf("comm-itop: %s %d: %w", cfg.Class, id, incident.ErrNotFound)
	}
	status := objs[0].field("status")
	switch {
	case status == "resolved" || status == "closed" || status == "rejected":
		return nil
	case needsAssign[status]:
		if err := c.stimulus(ctx, cfg, id, "ev_assign", map[string]any{"team_id": cfg.TeamID, "agent_id": cfg.AgentID}); err != nil {
			return err
		}
	case status == "assigned" || status == "escalated_ttr":
	default:
		return operatorError("iTop %s %d is in state %q, from which McHarbor cannot resolve it", cfg.Class, id, sanitize(status))
	}
	solution := describeText(ev)
	if err := c.stimulus(ctx, cfg, id, "ev_resolve", map[string]any{
		"service_id": cfg.ServiceID, "resolution_code": cfg.ResolutionCode, "solution": solution,
		"private_log": logItem("Resolved by McHarbor: " + ev.Title),
	}); err != nil {
		return err
	}
	if cfg.CloseOnResolve {
		return c.stimulus(ctx, cfg, id, "ev_close", map[string]any{})
	}
	return nil
}

func (c *Client) stimulus(ctx context.Context, cfg itopConfig, id int, stim string, fields map[string]any) error {
	_, err := c.rest(ctx, cfg, map[string]any{
		"operation": "core/apply_stimulus", "class": cfg.Class, "key": id, "stimulus": stim,
		"comment": "McHarbor", "output_fields": "id,status", "fields": fields,
	})
	return err
}
