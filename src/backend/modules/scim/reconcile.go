// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package scim

import (
	"sort"
	"strings"
)

// User change actions a sync plan can carry.
const (
	ActionLink      = "link"
	ActionUnmatched = "unmatched"
	ActionUpdate    = "update"
	ActionDisable   = "disable"
	ActionSkip      = "skip"
)

// Reasons attached to a disable or skip.
const (
	ReasonInactive       = "inactive"
	ReasonRemoved        = "removed"
	ReasonProtectedAdmin = "protected_admin"
	ReasonAlreadyLinked  = "already_linked"
	ReasonEmptyDirectory = "empty_directory"
)

// UserChange is one row of a sync plan. Only rows that change something are
// listed; unchanged users are counted in the summary instead.
type UserChange struct {
	Action        string   `json:"action"`
	ScimID        string   `json:"scimId"`
	UserName      string   `json:"userName"`
	Email         string   `json:"email,omitempty"`
	DisplayName   string   `json:"displayName,omitempty"`
	LocalUserID   string   `json:"localUserId,omitempty"`
	LocalUsername string   `json:"localUsername,omitempty"`
	Fields        []string `json:"fields,omitempty"`
	Reason        string   `json:"reason,omitempty"`
	Link          bool     `json:"link,omitempty"`
	MappingID     string   `json:"-"`
}

// GroupSummary reports a remote group and the McHarbor group of the same name,
// if any. Groups are read-only in this sync: nothing is created from them.
type GroupSummary struct {
	ScimID       string `json:"scimId"`
	DisplayName  string `json:"displayName"`
	Members      int    `json:"members"`
	LocalGroupID string `json:"localGroupId,omitempty"`
}

// SyncSummary carries the counts a plan (or the run that applied it) produced.
type SyncSummary struct {
	UsersSeen   int `json:"usersSeen"`
	Linked      int `json:"linked"`
	Unmatched   int `json:"unmatched"`
	Updated     int `json:"updated"`
	Disabled    int `json:"disabled"`
	Skipped     int `json:"skipped"`
	Unchanged   int `json:"unchanged"`
	GroupsSeen  int `json:"groupsSeen"`
	PendingLink int `json:"pendingLink"`
}

// SyncPlan is what a sync would do. A dry run returns it; a real run applies
// it. Blocked plans are never applied.
type SyncPlan struct {
	EndpointID    string         `json:"endpointId"`
	GeneratedAt   string         `json:"generatedAt"`
	Users         []UserChange   `json:"users"`
	Groups        []GroupSummary `json:"groups"`
	Summary       SyncSummary    `json:"summary"`
	Blocked       bool           `json:"blocked"`
	BlockedReason string         `json:"blockedReason,omitempty"`
	SeenIDs       []string       `json:"-"`
}

type mappingRow struct {
	ID             string
	ScimUserID     string
	McharborUserID string
	ScimUsername   string
	ScimEmail      string
}

type localUser struct {
	ID          string
	Username    string
	Email       string
	DisplayName string
	Active      bool
	Protected   bool
}

type localGroup struct {
	ID   string
	Name string
}

// computePlan compares a remote directory with the endpoint's mappings and the
// local accounts. It never provisions accounts: a remote user with no local
// match is recorded as unmatched for an admin to resolve.
func computePlan(remoteUsers []User, remoteGroups []Group, mappings []mappingRow, locals []localUser, groups []localGroup) SyncPlan {
	plan := SyncPlan{Users: []UserChange{}, Groups: []GroupSummary{}}

	byScimID := make(map[string]mappingRow, len(mappings))
	linkedLocal := make(map[string]string, len(mappings))
	for _, m := range mappings {
		byScimID[m.ScimUserID] = m
		if m.McharborUserID != "" {
			linkedLocal[m.McharborUserID] = m.ScimUserID
		}
	}
	localByID := make(map[string]localUser, len(locals))
	localByName := make(map[string]localUser, len(locals))
	localByEmail := make(map[string]localUser, len(locals))
	for _, u := range locals {
		localByID[u.ID] = u
		localByName[strings.ToLower(u.Username)] = u
		if u.Email != "" {
			localByEmail[strings.ToLower(u.Email)] = u
		}
	}

	seen := make(map[string]bool, len(remoteUsers))
	for _, remote := range remoteUsers {
		if remote.ID == "" || seen[remote.ID] {
			continue
		}
		seen[remote.ID] = true
		plan.SeenIDs = append(plan.SeenIDs, remote.ID)
		plan.Summary.UsersSeen++
		email := primaryEmail(remote)
		display := remoteDisplayName(remote)
		change := UserChange{ScimID: remote.ID, UserName: remote.UserName, Email: email, DisplayName: display}

		mapping, mapped := byScimID[remote.ID]
		change.MappingID = mapping.ID
		local, linked := localByID[mapping.McharborUserID]
		if !mapped || !linked {
			candidate, found := matchLocal(remote.UserName, email, localByName, localByEmail)
			if found {
				if owner, taken := linkedLocal[candidate.ID]; taken && owner != remote.ID {
					change.Action = ActionSkip
					change.Reason = ReasonAlreadyLinked
					change.LocalUserID = candidate.ID
					change.LocalUsername = candidate.Username
					plan.Summary.Skipped++
					plan.Users = append(plan.Users, change)
					continue
				}
				change.LocalUserID = candidate.ID
				change.LocalUsername = candidate.Username
				change.Link = true
				if !remote.Active && candidate.Active {
					disableOrSkip(&change, candidate, ReasonInactive, &plan.Summary)
				} else {
					change.Action = ActionLink
					plan.Summary.Linked++
				}
				linkedLocal[candidate.ID] = remote.ID
				plan.Users = append(plan.Users, change)
				continue
			}
			plan.Summary.PendingLink++
			if !mapped {
				change.Action = ActionUnmatched
				plan.Summary.Unmatched++
				plan.Users = append(plan.Users, change)
				continue
			}
			plan.Summary.Unchanged++
			continue
		}

		change.LocalUserID = local.ID
		change.LocalUsername = local.Username
		if !remote.Active && local.Active {
			disableOrSkip(&change, local, ReasonInactive, &plan.Summary)
			plan.Users = append(plan.Users, change)
			continue
		}
		change.Fields = changedFields(mapping, local, remote.UserName, email, display)
		if len(change.Fields) == 0 {
			plan.Summary.Unchanged++
			continue
		}
		change.Action = ActionUpdate
		plan.Summary.Updated++
		plan.Users = append(plan.Users, change)
	}

	removed := make([]UserChange, 0)
	for _, m := range mappings {
		if seen[m.ScimUserID] || m.McharborUserID == "" {
			continue
		}
		local, ok := localByID[m.McharborUserID]
		if !ok || !local.Active {
			continue
		}
		change := UserChange{
			ScimID:        m.ScimUserID,
			UserName:      m.ScimUsername,
			Email:         m.ScimEmail,
			LocalUserID:   local.ID,
			LocalUsername: local.Username,
			MappingID:     m.ID,
		}
		disableOrSkip(&change, local, ReasonRemoved, &plan.Summary)
		removed = append(removed, change)
	}
	sort.Slice(removed, func(i, j int) bool { return removed[i].UserName < removed[j].UserName })
	plan.Users = append(plan.Users, removed...)

	if plan.Summary.UsersSeen == 0 && len(removed) > 0 {
		plan.Blocked = true
		plan.BlockedReason = ReasonEmptyDirectory
	}

	groupByName := make(map[string]string, len(groups))
	for _, g := range groups {
		groupByName[strings.ToLower(g.Name)] = g.ID
	}
	for _, g := range remoteGroups {
		if g.ID == "" {
			continue
		}
		plan.Summary.GroupsSeen++
		plan.Groups = append(plan.Groups, GroupSummary{
			ScimID:       g.ID,
			DisplayName:  g.DisplayName,
			Members:      len(g.Members),
			LocalGroupID: groupByName[strings.ToLower(g.DisplayName)],
		})
	}
	return plan
}

func disableOrSkip(change *UserChange, local localUser, reason string, summary *SyncSummary) {
	if local.Protected {
		change.Action = ActionSkip
		change.Reason = ReasonProtectedAdmin
		summary.Skipped++
		return
	}
	change.Action = ActionDisable
	change.Reason = reason
	summary.Disabled++
}

func matchLocal(userName, email string, byName, byEmail map[string]localUser) (localUser, bool) {
	if u, ok := byName[strings.ToLower(userName)]; ok && userName != "" {
		return u, true
	}
	if email != "" {
		if u, ok := byEmail[strings.ToLower(email)]; ok {
			return u, true
		}
	}
	return localUser{}, false
}

func changedFields(m mappingRow, local localUser, userName, email, display string) []string {
	var fields []string
	if m.ScimUsername != userName {
		fields = append(fields, "scimUserName")
	}
	if email != "" && !strings.EqualFold(local.Email, email) {
		fields = append(fields, "email")
	} else if m.ScimEmail != email {
		fields = append(fields, "scimEmail")
	}
	if display != "" && local.DisplayName != display {
		fields = append(fields, "displayName")
	}
	return fields
}

func primaryEmail(u User) string {
	for _, e := range u.Emails {
		if e.Primary && e.Value != "" {
			return e.Value
		}
	}
	for _, e := range u.Emails {
		if e.Value != "" {
			return e.Value
		}
	}
	return ""
}

func remoteDisplayName(u User) string {
	if u.Name == nil {
		return ""
	}
	return strings.TrimSpace(strings.TrimSpace(u.Name.GivenName) + " " + strings.TrimSpace(u.Name.FamilyName))
}
