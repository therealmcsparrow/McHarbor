// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package push

import (
	"net/url"
	"strings"

	"github.com/therealmcsparrow/mcharbor/core/alertnotice"
	"github.com/therealmcsparrow/mcharbor/core/rbac"
)

// NotifyAlert pushes an alert state change to subscribed users who may view
// alerts in its environment. A container alert deep-links to that container;
// anything else opens the inbox.
func (s *Service) NotifyAlert(a alertnotice.Notice) {
	s.Notify(AlertNotification(a), Audience{Permission: rbac.PermAlertsView, EnvID: a.EnvironmentID})
}

// AlertNotification renders a notice as the push payload.
func AlertNotification(a alertnotice.Notice) Notification {
	link := "/oncall/alerts"
	if a.SourceType == alertnotice.SourceContainer && a.SubjectID != "" && a.EnvironmentID != "" {
		link = "/oncall/containers/" + url.PathEscape(a.EnvironmentID) + "/" + url.PathEscape(a.SubjectID)
	}
	body := a.Message
	if r := []rune(body); len(r) > 240 {
		body = string(r[:239]) + "…"
	}
	key := MsgAlertTriggered
	if a.State == alertnotice.StateResolved {
		key = MsgAlertResolved
	}
	name := strings.TrimSpace(a.Name)
	if name == "" {
		name = "Alert"
	}
	return Notification{
		Title:    a.Title,
		Body:     body,
		Severity: a.Severity,
		URL:      link,
		Tag:      "alert-" + a.AlertID + "-" + a.SubjectID,
		Key:      key,
		Args:     map[string]string{"name": name},
	}
}
