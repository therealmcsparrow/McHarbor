// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

import (
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/therealmcsparrow/mcharbor/core/plugin"
	"github.com/therealmcsparrow/mcharbor/core/plugin/specs/incident"
)

var (
	_ plugin.NotificationAdapterClient = (*Client)(nil)
	_ incident.LifecycleClient         = (*Client)(nil)
	_ incident.WebhookParser           = (*Client)(nil)
	_ incident.Poller                  = (*Client)(nil)
)

// allowHTTP lets tests talk to an httptest server over plain http; nothing in production sets it,
// so a channel can never be pointed at a cleartext endpoint that would receive the agent password.
var allowHTTP = false

// Client is stateless apart from a SessionID cache keyed by endpoint, login and password, so one
// plugin instance serves any number of channels without logging in on every call.
type Client struct {
	http     *http.Client
	mu       sync.Mutex
	sessions map[string]string
}

// NewClient returns a Client whose transport never follows redirects, because a redirected POST
// would resend the agent password to wherever the Location header points.
func NewClient() *Client {
	return &Client{
		http: &http.Client{
			Timeout: 35 * time.Second,
			CheckRedirect: func(*http.Request, []*http.Request) error {
				return http.ErrUseLastResponse
			},
		},
		sessions: map[string]string{},
	}
}

type config struct {
	BaseURL, Script, Endpoint, Webservice             string
	Flavour, UserLogin, Password, Queue, CustomerUser string
	TicketType, DedupField, CloseState                string
	ResolvedStates, CancelledStates                   map[string]bool
	Severity                                          map[incident.Severity]string
}

var errConfig = errors.New("comm-otrs: invalid configuration")

func configError(msg string) error {
	return fmt.Errorf("%w: %w: %s", errConfig, incident.ErrOperatorMessage, msg)
}

func parseConfig(m map[string]string) (config, error) {
	base := strings.TrimRight(strings.TrimSpace(m["instance_url"]), "/")
	u, err := url.Parse(base)
	if err != nil || u.Host == "" || u.Opaque != "" {
		return config{}, configError("instance_url must be an absolute https:// URL")
	}
	if u.User != nil || u.RawQuery != "" || u.Fragment != "" || strings.Contains(base, "?") || strings.Contains(base, "#") {
		return config{}, configError("instance_url must not carry credentials, a query or a fragment")
	}
	if u.Scheme != "https" && !(allowHTTP && u.Scheme == "http") {
		return config{}, configError("instance_url must use https://")
	}
	flavour := strings.ToLower(strings.TrimSpace(m["flavour"]))
	switch flavour {
	case "":
		flavour = "znuny"
	case "znuny", "otobo", "otrs":
	default:
		return config{}, configError("flavour must be znuny, otobo or otrs")
	}
	script := strings.Trim(strings.TrimSpace(m["script_path"]), "/")
	if script == "" {
		script = "otrs"
		if flavour == "otobo" {
			script = "otobo"
		}
	}
	if strings.ContainsAny(script, "?#\\") || strings.Contains(script, "..") {
		return config{}, configError("script_path must be a plain path segment such as otrs")
	}
	ws := strings.TrimSpace(m["webservice"])
	if ws == "" || strings.TrimSpace(m["user_login"]) == "" || m["password"] == "" ||
		strings.TrimSpace(m["default_assignment_group"]) == "" || strings.TrimSpace(m["customer_user"]) == "" ||
		strings.TrimSpace(m["dedup_field"]) == "" {
		return config{}, configError("webservice, user_login, password, queue, customer_user and dedup_field are required")
	}
	dedup := strings.TrimSpace(m["dedup_field"])
	for _, r := range dedup {
		if !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9') {
			return config{}, configError("dedup_field must be a dynamic field name of letters and digits")
		}
	}
	sev, err := severityMap(m["severity_map"])
	if err != nil {
		return config{}, err
	}
	closeState := strings.TrimSpace(m["close_state"])
	if closeState == "" {
		closeState = "closed successful"
	}
	return config{
		BaseURL:         base,
		Script:          script,
		Endpoint:        base + "/" + script + "/nph-genericinterface.pl/Webservice/" + url.PathEscape(ws),
		Webservice:      ws,
		Flavour:         flavour,
		UserLogin:       strings.TrimSpace(m["user_login"]),
		Password:        m["password"],
		Queue:           strings.TrimSpace(m["default_assignment_group"]),
		CustomerUser:    strings.TrimSpace(m["customer_user"]),
		TicketType:      strings.TrimSpace(m["ticket_type"]),
		DedupField:      dedup,
		CloseState:      closeState,
		ResolvedStates:  csvSet(m["resolved_states"]),
		CancelledStates: csvSet(m["cancelled_states"]),
		Severity:        sev,
	}, nil
}

func (cfg config) ref(id, number string) incident.Ref {
	return incident.Ref{
		ExternalID: id,
		Number:     number,
		URL:        cfg.BaseURL + "/" + cfg.Script + "/index.pl?Action=AgentTicketZoom;TicketID=" + url.QueryEscape(id),
	}
}
