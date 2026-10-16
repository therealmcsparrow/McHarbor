// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package audit

import (
	"context"
	"strings"
)

const (
	actionPermissionDenied = "permission.denied"
	actionABACDenied       = "abac.denied"
)

// DenialRow is one RBAC or ABAC denial with the fields the audit row packs into
// its details string pulled out, so the tab can filter and link without parsing.
type DenialRow struct {
	ID              string  `json:"id"`
	Timestamp       string  `json:"timestamp"`
	Kind            string  `json:"kind"`
	UserID          string  `json:"userId"`
	Username        string  `json:"username"`
	Permission      string  `json:"permission"`
	Route           string  `json:"route"`
	EnvironmentID   *string `json:"environmentId,omitempty"`
	EnvironmentName *string `json:"environmentName,omitempty"`
	PolicyID        string  `json:"policyId,omitempty"`
	ResourceType    string  `json:"resourceType,omitempty"`
	ResourceID      string  `json:"resourceId,omitempty"`
	IPAddress       *string `json:"ipAddress,omitempty"`
}

// denialDetails is what a denial row's details string carries.
type denialDetails struct {
	Permission   string
	Route        string
	Environment  string
	PolicyID     string
	ResourceType string
	ResourceID   string
	UserID       string
}

// parseDenialDetails reads the space-separated key=value pairs the rbac denial
// hooks write; unknown keys are ignored so a hook can add fields safely.
func parseDenialDetails(details string) denialDetails {
	var out denialDetails
	for _, field := range strings.Fields(details) {
		key, value, ok := strings.Cut(field, "=")
		if !ok {
			continue
		}
		switch key {
		case "permission", "perm":
			out.Permission = value
		case "path":
			out.Route = value
		case "env":
			out.Environment = value
		case "policy_id":
			out.PolicyID = value
		case "resource_type":
			out.ResourceType = value
		case "resource_id":
			out.ResourceID = value
		case "user_id":
			out.UserID = value
		}
	}
	return out
}

// permissionPatterns matches a permission key in either denial format: RBAC
// writes `permission=<key> env=…`, ABAC writes `perm=<key>` mid-string or last.
func permissionPatterns(permission string) []string {
	p := likeEscape(permission)
	return []string{"%permission=" + p + " %", "%permission=" + p, "%perm=" + p + " %", "%perm=" + p}
}

// denialFromLog projects an audit row onto a DenialRow.
func denialFromLog(log Log) DenialRow {
	parsed := parseDenialDetails(stringPtrValue(log.Details))
	row := DenialRow{
		ID:              log.ID,
		Timestamp:       log.Timestamp,
		Kind:            "rbac",
		UserID:          stringPtrValue(log.UserID),
		Username:        stringPtrValue(log.Username),
		Permission:      parsed.Permission,
		Route:           parsed.Route,
		EnvironmentID:   log.EnvironmentID,
		EnvironmentName: log.EnvironmentName,
		PolicyID:        parsed.PolicyID,
		ResourceType:    parsed.ResourceType,
		ResourceID:      parsed.ResourceID,
		IPAddress:       log.IPAddress,
	}
	if log.Action == actionABACDenied {
		row.Kind = "abac"
		if row.PolicyID == "" {
			row.PolicyID = stringPtrValue(log.EntityID)
		}
	}
	if row.UserID == "" {
		row.UserID = parsed.UserID
	}
	if row.UserID == "" && log.Action == actionPermissionDenied {
		row.UserID = stringPtrValue(log.EntityID)
	}
	if row.Username == "" && log.Action == actionPermissionDenied {
		row.Username = stringPtrValue(log.EntityName)
	}
	return row
}

// ListDenials pages permission.denied and abac.denied rows; the filter's actor,
// permission and environment narrow the SQL so the total stays honest.
func (s *Service) ListDenials(ctx context.Context, filter listFilter, permission string) ([]DenialRow, int64, error) {
	filter.Actions = []string{actionPermissionDenied, actionABACDenied}
	filter.Action = ""
	filter.ActorInDetails = true
	if permission != "" {
		filter.DetailsAny = permissionPatterns(permission)
	}
	logs, total, err := s.List(ctx, filter)
	if err != nil {
		return nil, 0, err
	}
	rows := make([]DenialRow, 0, len(logs))
	missing := []string{}
	for _, log := range logs {
		row := denialFromLog(log)
		if row.Username == "" && row.UserID != "" {
			missing = append(missing, row.UserID)
		}
		rows = append(rows, row)
	}
	if len(missing) > 0 {
		names := s.usernamesByID(ctx, missing)
		for i := range rows {
			if rows[i].Username == "" {
				rows[i].Username = names[rows[i].UserID]
			}
		}
	}
	return rows, total, nil
}
