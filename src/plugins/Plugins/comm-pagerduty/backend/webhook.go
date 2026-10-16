// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/therealmcsparrow/mcharbor/core/plugin/specs/incident"
)

var errSignature = errors.New("comm-pagerduty: webhook signature rejected")

// VerifyWebhook accepts any one valid v1 signature, because PagerDuty sends several during a
// secret rotation; non-v1 schemes are skipped so a future scheme cannot downgrade the check.
func (c *Client) VerifyWebhook(config map[string]string, header http.Header, body []byte) error {
	secret := firstNonEmpty(strings.TrimSpace(config["webhook_signing_secret"]), config["_webhook_secret"])
	if secret == "" || len(body) == 0 {
		return errSignature
	}
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	want := mac.Sum(nil)
	for _, part := range strings.Split(header.Get("X-PagerDuty-Signature"), ",") {
		part = strings.TrimSpace(part)
		if !strings.HasPrefix(part, "v1=") {
			continue
		}
		got, err := hex.DecodeString(strings.TrimPrefix(part, "v1="))
		if err != nil {
			continue
		}
		if hmac.Equal(want, got) {
			return nil
		}
	}
	return errSignature
}

type pdWebhook struct {
	Event struct {
		ID           string    `json:"id"`
		EventType    string    `json:"event_type"`
		ResourceType string    `json:"resource_type"`
		OccurredAt   time.Time `json:"occurred_at"`
		Agent        struct {
			Summary string `json:"summary"`
		} `json:"agent"`
		Data pdIncident `json:"data"`
	} `json:"event"`
}

type pdIncident struct {
	ID                 string    `json:"id"`
	Number             int       `json:"number"`
	IncidentNum        int       `json:"incident_number"`
	Status             string    `json:"status"`
	IncidentKey        string    `json:"incident_key"`
	HTMLURL            string    `json:"html_url"`
	CreatedAt          time.Time `json:"created_at"`
	LastStatusChangeAt time.Time `json:"last_status_change_at"`
	Assignees          []struct {
		Summary string `json:"summary"`
	} `json:"assignees"`
}

// ParseWebhook turns one v3 delivery into at most one status; the host drops statuses for
// tickets it never opened, so incidents from other integrations are harmless noise.
func (c *Client) ParseWebhook(_ context.Context, _ map[string]string, _ http.Header, body []byte) ([]incident.RemoteStatus, error) {
	var wh pdWebhook
	if err := json.Unmarshal(body, &wh); err != nil {
		return nil, fmt.Errorf("comm-pagerduty: decode webhook: %w", err)
	}
	if wh.Event.ResourceType != "incident" || wh.Event.Data.IncidentKey == "" {
		return nil, nil
	}
	st, ok := toRemoteStatus(wh.Event.Data, wh.Event.OccurredAt)
	if !ok {
		return nil, nil
	}
	if st.Assignee == "" && wh.Event.EventType == "incident.acknowledged" {
		st.Assignee = wh.Event.Agent.Summary
	}
	return []incident.RemoteStatus{st}, nil
}

// toRemoteStatus maps PagerDuty's three statuses and drops anything else, because a guessed
// state would move a link the vendor never moved.
func toRemoteStatus(d pdIncident, at time.Time) (incident.RemoteStatus, bool) {
	var state incident.RemoteState
	switch d.Status {
	case "triggered":
		state = incident.RemoteOpen
	case "acknowledged":
		state = incident.RemoteAcknowledged
	case "resolved":
		state = incident.RemoteResolved
	default:
		return incident.RemoteStatus{}, false
	}
	num := d.Number
	if num == 0 {
		num = d.IncidentNum
	}
	st := incident.RemoteStatus{
		Ref:       incident.Ref{ExternalID: d.IncidentKey, URL: d.HTMLURL},
		DedupKey:  d.IncidentKey,
		State:     state,
		UpdatedAt: at,
	}
	if num > 0 {
		st.Ref.Number = "#" + strconv.Itoa(num)
	}
	if len(d.Assignees) > 0 {
		st.Assignee = d.Assignees[0].Summary
	}
	return st, true
}
