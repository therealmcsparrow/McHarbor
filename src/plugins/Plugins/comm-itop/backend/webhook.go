// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"html"
	"net/http"
	"strconv"
	"strings"

	"github.com/therealmcsparrow/mcharbor/core/plugin/specs/incident"
)

// secretHeader carries the channel's inbound secret; iTop cannot sign webhooks, so the Remote
// application connection sends this static header instead.
const secretHeader = "X-McHarbor-Secret"

// pollBatch caps the ids in one OQL IN list, keeping the query well inside URL and SQL limits.
const pollBatch = 100

// errWebhookAuth is deliberately bare: the public route must not reveal which check failed.
var errWebhookAuth = errors.New("comm-itop: webhook authentication failed")

// VerifyWebhook compares the secret header with the host's channel secret through SHA-256 digests
// in constant time, and fails closed when either side is empty.
func (c *Client) VerifyWebhook(cfg map[string]string, header http.Header, _ []byte) error {
	want := cfg["_webhook_secret"]
	got := header.Get(secretHeader)
	if want == "" || got == "" {
		return errWebhookAuth
	}
	w := sha256.Sum256([]byte(want))
	g := sha256.Sum256([]byte(got))
	if subtle.ConstantTimeCompare(w[:], g[:]) != 1 {
		return errWebhookAuth
	}
	return nil
}

// webhookPayload is the JSON the documented ActionWebhook template produces; every value is a
// string because iTop substitutes placeholders as text.
type webhookPayload struct {
	ID     string `json:"id"`
	Ref    string `json:"ref"`
	Class  string `json:"class"`
	Title  string `json:"title"`
	Status string `json:"status"`
	Agent  string `json:"agent"`
}

// ParseWebhook maps one iTop object to a remote status; tickets without the McHarbor marker and
// unknown statuses are dropped, because the ITSM may inform the host but never create links.
func (c *Client) ParseWebhook(_ context.Context, cfg map[string]string, _ http.Header, body []byte) ([]incident.RemoteStatus, error) {
	var p webhookPayload
	if err := json.Unmarshal(body, &p); err != nil {
		return nil, operatorError("the iTop webhook payload is not the documented JSON template")
	}
	title := html.UnescapeString(strings.TrimSpace(p.Title))
	if !markerPattern.MatchString(title) {
		return nil, nil
	}
	id, err := strconv.Atoi(strings.TrimSpace(p.ID))
	if err != nil || id <= 0 {
		return nil, nil
	}
	state, ok := remoteState(p.Status)
	if !ok {
		return nil, nil
	}
	class := strings.TrimSpace(p.Class)
	if class != "Incident" && class != "UserRequest" {
		class = orDefault(strings.TrimSpace(cfg["ticket_class"]), "Incident")
	}
	base, err := baseURL(strings.TrimSpace(cfg["instance_url"]))
	if err != nil {
		base = ""
	}
	ext := strconv.Itoa(id)
	return []incident.RemoteStatus{{
		Ref:      incident.Ref{ExternalID: ext, Number: html.UnescapeString(strings.TrimSpace(p.Ref)), URL: ticketURL(base, class, ext)},
		State:    state,
		Assignee: html.UnescapeString(strings.TrimSpace(p.Agent)),
	}}, nil
}

// Poll reads every ref's status with one OQL query per batch; refs whose id is not an integer
// are skipped, so a corrupt link can neither reach OQL nor stall the rest of the channel.
func (c *Client) Poll(ctx context.Context, raw map[string]string, refs []incident.Ref) ([]incident.RemoteStatus, error) {
	cfg, err := parseConfig(raw)
	if err != nil {
		return nil, err
	}
	seen := make(map[int]bool, len(refs))
	var ids []string
	for _, r := range refs {
		id, err := ticketID(r)
		if err != nil || seen[id] {
			continue
		}
		seen[id] = true
		ids = append(ids, strconv.Itoa(id))
	}
	var out []incident.RemoteStatus
	for start := 0; start < len(ids); start += pollBatch {
		end := min(start+pollBatch, len(ids))
		oql := "SELECT " + cfg.Class + " WHERE id IN (" + strings.Join(ids[start:end], ",") + ")"
		objs, err := c.get(ctx, cfg, cfg.Class, oql, "id,ref,status,agent_id_friendlyname,last_update")
		if err != nil {
			return out, err
		}
		for _, o := range objs {
			state, ok := remoteState(o.field("status"))
			if !ok {
				continue
			}
			ref := c.ref(cfg, o)
			out = append(out, incident.RemoteStatus{Ref: ref, State: state, Assignee: o.field("agent_id_friendlyname")})
		}
	}
	return out, nil
}
