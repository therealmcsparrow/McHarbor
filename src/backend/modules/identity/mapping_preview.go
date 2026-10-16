// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package identity

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/rs/xid"

	"github.com/therealmcsparrow/mcharbor/core/db"
)

// maxPreviewUsers bounds the users a mapping preview evaluates; the preview is
// an admin read, not a batch job.
const maxPreviewUsers = 1000

// newGroupPrefix marks a group that auto-import would create by name; it has
// no roles yet, so it can only ever add a membership, never a permission.
const newGroupPrefix = "new:"

// MappingConfig is the part of a provider's configuration that decides which
// McHarbor groups a login lands in.
type MappingConfig struct {
	GroupMappingEnabled bool           `json:"groupMappingEnabled"`
	GroupMappings       []GroupMapping `json:"groupMappings"`
	AutoImportGroups    bool           `json:"autoImportGroups"`
	AutoProvision       bool           `json:"autoProvision"`
	DefaultRoleID       string         `json:"defaultRoleId,omitempty"`
}

// MappingPreviewInput is the body of POST /identity-providers/mapping-preview.
// ProviderID is empty while a provider is still being created; the preview then
// covers the sample only, because no user has signed in through it yet.
type MappingPreviewInput struct {
	ProviderID   string        `json:"providerId,omitempty"`
	ProviderType string        `json:"providerType,omitempty"`
	Config       MappingConfig `json:"config"`
	SampleGroups []string      `json:"sampleGroups"`
}

// RoleGrantView is one role at one scope, with display names resolved.
type RoleGrantView struct {
	RoleID        string `json:"roleId"`
	RoleName      string `json:"roleName"`
	EnvironmentID string `json:"environmentId,omitempty"`
	StackName     string `json:"stackName,omitempty"`
}

// GroupView names a McHarbor group; New marks one auto-import would create.
type GroupView struct {
	GroupID   string `json:"groupId,omitempty"`
	GroupName string `json:"groupName"`
	New       bool   `json:"new,omitempty"`
}

// SampleGroupResult is one McHarbor group the sample would join, the provider
// groups that lead there, and the roles that group carries.
type SampleGroupResult struct {
	GroupView
	From  []string        `json:"from"`
	Roles []RoleGrantView `json:"roles"`
}

// SamplePreview answers "someone with these provider groups signs in: what do
// they get?" for a first-time user.
type SamplePreview struct {
	Groups      []SampleGroupResult `json:"groups"`
	Unmapped    []string            `json:"unmapped"`
	DefaultRole *RoleGrantView      `json:"defaultRole,omitempty"`
	Roles       []RoleGrantView     `json:"roles"`
}

// UserImpact is one existing user whose roles would change at next sign-in.
type UserImpact struct {
	UserID   string          `json:"userId"`
	Username string          `json:"username"`
	Gained   []RoleGrantView `json:"gained"`
	Lost     []RoleGrantView `json:"lost"`
	Joined   []GroupView     `json:"joined"`
	Left     []GroupView     `json:"left"`
}

// ImpactPreview compares the stored configuration with the proposed one for
// every user whose last sign-in claims were recorded.
type ImpactPreview struct {
	Evaluated int           `json:"evaluated"`
	Unchanged int           `json:"unchanged"`
	Users     []UserImpact  `json:"users"`
	Unknown   []UserSummary `json:"unknown"`
}

// UserSummary identifies a user without claims on record.
type UserSummary struct {
	UserID   string `json:"userId"`
	Username string `json:"username"`
}

// MappingPreview is the response of the preview endpoint.
type MappingPreview struct {
	Sample SamplePreview  `json:"sample"`
	Impact *ImpactPreview `json:"impact,omitempty"`
}

// membership is a user's group membership and the identity source that wrote it.
type membership struct {
	GroupID string
	Source  string
}

// mappingDecisions returns, per mapped McHarbor group, whether a login with
// claims ends up a member. Mappings apply in order, so the last mapping naming
// a group decides it; applyGroupMappings writes exactly these decisions.
func mappingDecisions(mappings []GroupMapping, claims []string) ([]string, map[string]bool) {
	claimed := make(map[string]bool, len(claims))
	for _, c := range claims {
		claimed[c] = true
	}
	var order []string
	decisions := map[string]bool{}
	for _, m := range mappings {
		if m.McharborGroupID == "" {
			continue
		}
		if _, seen := decisions[m.McharborGroupID]; !seen {
			order = append(order, m.McharborGroupID)
		}
		decisions[m.McharborGroupID] = claimed[m.ProviderGroup]
	}
	return order, decisions
}

// simulateLogin replays the sign-in group sync on a membership set: auto-import
// first, then mappings, and neither when the login carried no groups, exactly
// as ExchangeAndProvision and HandleSAMLACS do.
func simulateLogin(current []membership, cfg MappingConfig, claims []string, source string, groupIDByName map[string]string) map[string]bool {
	state := make(map[string]string, len(current))
	for _, m := range current {
		state[m.GroupID] = m.Source
	}
	if len(claims) > 0 && cfg.AutoImportGroups {
		wanted := map[string]bool{}
		for _, name := range claims {
			id, ok := groupIDByName[name]
			if !ok {
				id = newGroupPrefix + name
			}
			wanted[id] = true
			if _, member := state[id]; !member {
				state[id] = source
			}
		}
		for id, src := range state {
			if src == source && !wanted[id] {
				delete(state, id)
			}
		}
	}
	if len(claims) > 0 && cfg.GroupMappingEnabled {
		_, decisions := mappingDecisions(cfg.GroupMappings, claims)
		for id, member := range decisions {
			if member {
				if _, ok := state[id]; !ok {
					state[id] = ""
				}
			} else {
				delete(state, id)
			}
		}
	}
	out := make(map[string]bool, len(state))
	for id := range state {
		out[id] = true
	}
	return out
}

// grantKey identifies a role at a scope for set arithmetic.
func grantKey(g RoleGrantView) string {
	return g.RoleID + "\x00" + g.EnvironmentID + "\x00" + g.StackName
}

// rolesFor unions the grants of groups, skipping anything the user already
// holds directly, so a group role that duplicates a direct one is no change.
func rolesFor(groups map[string]bool, groupRoles map[string][]RoleGrantView, direct map[string]bool) map[string]RoleGrantView {
	out := map[string]RoleGrantView{}
	for id := range groups {
		for _, g := range groupRoles[id] {
			key := grantKey(g)
			if !direct[key] {
				out[key] = g
			}
		}
	}
	return out
}

// diffGrants returns the grants in a but not in b, sorted for stable output.
func diffGrants(a, b map[string]RoleGrantView) []RoleGrantView {
	out := []RoleGrantView{}
	for key, g := range a {
		if _, ok := b[key]; !ok {
			out = append(out, g)
		}
	}
	sort.Slice(out, func(i, j int) bool { return grantKey(out[i]) < grantKey(out[j]) })
	return out
}

// PreviewMapping computes what a mapping configuration grants, for a sample
// group list and, when the provider exists, for every user with recorded claims.
func (s *Service) PreviewMapping(ctx context.Context, in MappingPreviewInput) (*MappingPreview, error) {
	lookup, err := s.loadMappingLookup(ctx)
	if err != nil {
		return nil, err
	}
	source := in.ProviderType
	var stored *runtimeProvider
	if in.ProviderID != "" {
		provider, err := s.loadRuntimeProvider(ctx, in.ProviderID)
		if err != nil {
			return nil, err
		}
		stored = &provider
		source = provider.ProviderType
	}

	out := &MappingPreview{Sample: s.samplePreview(in, source, lookup)}
	if stored == nil {
		return out, nil
	}
	impact, err := s.impactPreview(ctx, *stored, in.Config, lookup)
	if err != nil {
		return nil, err
	}
	out.Impact = impact
	return out, nil
}

type mappingLookup struct {
	groupIDByName map[string]string
	groupName     map[string]string
	groupRoles    map[string][]RoleGrantView
	roleName      map[string]string
}

func (l mappingLookup) groupView(id string) GroupView {
	if strings.HasPrefix(id, newGroupPrefix) {
		return GroupView{GroupName: strings.TrimPrefix(id, newGroupPrefix), New: true}
	}
	return GroupView{GroupID: id, GroupName: l.groupName[id]}
}

func (s *Service) samplePreview(in MappingPreviewInput, source string, lookup mappingLookup) SamplePreview {
	claims := normaliseClaims(in.SampleGroups)
	joined := simulateLogin(nil, in.Config, claims, source, lookup.groupIDByName)

	from := map[string][]string{}
	for _, claim := range claims {
		if in.Config.AutoImportGroups {
			id, ok := lookup.groupIDByName[claim]
			if !ok {
				id = newGroupPrefix + claim
			}
			from[id] = append(from[id], claim)
		}
	}
	if in.Config.GroupMappingEnabled {
		for _, m := range in.Config.GroupMappings {
			if joined[m.McharborGroupID] && containsString(claims, m.ProviderGroup) && !containsString(from[m.McharborGroupID], m.ProviderGroup) {
				from[m.McharborGroupID] = append(from[m.McharborGroupID], m.ProviderGroup)
			}
		}
	}

	sample := SamplePreview{Groups: []SampleGroupResult{}, Unmapped: []string{}, Roles: []RoleGrantView{}}
	used := map[string]bool{}
	for id := range joined {
		roles := lookup.groupRoles[id]
		if roles == nil {
			roles = []RoleGrantView{}
		}
		sample.Groups = append(sample.Groups, SampleGroupResult{GroupView: lookup.groupView(id), From: from[id], Roles: roles})
		for _, claim := range from[id] {
			used[claim] = true
		}
	}
	sort.Slice(sample.Groups, func(i, j int) bool { return sample.Groups[i].GroupName < sample.Groups[j].GroupName })
	for _, claim := range claims {
		if !used[claim] {
			sample.Unmapped = append(sample.Unmapped, claim)
		}
	}
	all := rolesFor(joined, lookup.groupRoles, nil)
	if in.Config.AutoProvision && in.Config.DefaultRoleID != "" {
		def := RoleGrantView{RoleID: in.Config.DefaultRoleID, RoleName: lookup.roleName[in.Config.DefaultRoleID]}
		sample.DefaultRole = &def
		all[grantKey(def)] = def
	}
	sample.Roles = diffGrants(all, nil)
	return sample
}

func (s *Service) impactPreview(ctx context.Context, provider runtimeProvider, proposed MappingConfig, lookup mappingLookup) (*ImpactPreview, error) {
	var storedMappings []GroupMapping
	if provider.GroupMappingsJSON != "" {
		if err := json.Unmarshal([]byte(provider.GroupMappingsJSON), &storedMappings); err != nil {
			return nil, fmt.Errorf("decoding stored group mappings: %w", err)
		}
	}
	current := MappingConfig{
		GroupMappingEnabled: provider.GroupMappingEnabled,
		GroupMappings:       storedMappings,
		AutoImportGroups:    provider.AutoImportGroups,
	}

	rows, err := s.db.QueryContext(ctx, `
		SELECT u.id, u.username, COALESCE(c.groups_json, '')
		  FROM users u
		  LEFT JOIN identity_group_claims c ON c.user_id = u.id AND c.provider_id = ?
		 WHERE u.identity_provider_id = ? OR c.provider_id IS NOT NULL
		 ORDER BY u.username
		 LIMIT ?`, provider.ID, provider.ID, maxPreviewUsers)
	if err != nil {
		return nil, fmt.Errorf("loading provider users: %w", err)
	}
	type candidate struct {
		id, username string
		claims       []string
		known        bool
	}
	var candidates []candidate
	for rows.Next() {
		var c candidate
		var claimsJSON string
		if err := rows.Scan(&c.id, &c.username, &claimsJSON); err != nil {
			closeRows(rows)
			return nil, fmt.Errorf("scanning provider user: %w", err)
		}
		if claimsJSON != "" {
			c.known = true
			if err := json.Unmarshal([]byte(claimsJSON), &c.claims); err != nil {
				c.known = false
			}
		}
		candidates = append(candidates, c)
	}
	if err := rows.Err(); err != nil {
		closeRows(rows)
		return nil, err
	}
	closeRows(rows)

	impact := &ImpactPreview{Users: []UserImpact{}, Unknown: []UserSummary{}}
	for _, c := range candidates {
		if !c.known {
			impact.Unknown = append(impact.Unknown, UserSummary{UserID: c.id, Username: c.username})
			continue
		}
		members, direct, err := s.userMembershipsAndDirect(ctx, c.id)
		if err != nil {
			return nil, err
		}
		before := simulateLogin(members, current, c.claims, provider.ProviderType, lookup.groupIDByName)
		after := simulateLogin(members, proposed, c.claims, provider.ProviderType, lookup.groupIDByName)
		impact.Evaluated++
		rolesBefore := rolesFor(before, lookup.groupRoles, direct)
		rolesAfter := rolesFor(after, lookup.groupRoles, direct)
		change := UserImpact{
			UserID:   c.id,
			Username: c.username,
			Gained:   diffGrants(rolesAfter, rolesBefore),
			Lost:     diffGrants(rolesBefore, rolesAfter),
			Joined:   groupDiff(after, before, lookup),
			Left:     groupDiff(before, after, lookup),
		}
		if len(change.Gained)+len(change.Lost)+len(change.Joined)+len(change.Left) == 0 {
			impact.Unchanged++
			continue
		}
		impact.Users = append(impact.Users, change)
	}
	return impact, nil
}

func groupDiff(a, b map[string]bool, lookup mappingLookup) []GroupView {
	out := []GroupView{}
	for id := range a {
		if !b[id] {
			out = append(out, lookup.groupView(id))
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].GroupName < out[j].GroupName })
	return out
}

func (s *Service) loadMappingLookup(ctx context.Context) (mappingLookup, error) {
	lookup := mappingLookup{
		groupIDByName: map[string]string{},
		groupName:     map[string]string{},
		groupRoles:    map[string][]RoleGrantView{},
		roleName:      map[string]string{},
	}
	if err := s.loadNamePairs(ctx, `SELECT id, name FROM groups LIMIT 10000`, "groups", func(id, name string) {
		lookup.groupIDByName[name] = id
		lookup.groupName[id] = name
	}); err != nil {
		return lookup, err
	}
	if err := s.loadNamePairs(ctx, `SELECT id, name FROM roles LIMIT 10000`, "roles", func(id, name string) {
		lookup.roleName[id] = name
	}); err != nil {
		return lookup, err
	}

	grantRows, err := s.db.QueryContext(ctx, `
		SELECT group_id, role_id, COALESCE(environment_id, ''), COALESCE(stack_name, '')
		  FROM group_roles
		 LIMIT 50000`)
	if err != nil {
		return lookup, fmt.Errorf("loading group roles: %w", err)
	}
	defer closeRows(grantRows)
	for grantRows.Next() {
		var groupID string
		var g RoleGrantView
		if err := grantRows.Scan(&groupID, &g.RoleID, &g.EnvironmentID, &g.StackName); err != nil {
			return lookup, err
		}
		g.RoleName = lookup.roleName[g.RoleID]
		lookup.groupRoles[groupID] = append(lookup.groupRoles[groupID], g)
	}
	return lookup, grantRows.Err()
}

// loadNamePairs runs a two-column id/name query and hands each row to add, so each result set closes before the next query needs the connection.
func (s *Service) loadNamePairs(ctx context.Context, query, what string, add func(id, name string)) error {
	rows, err := s.db.QueryContext(ctx, query)
	if err != nil {
		return fmt.Errorf("loading %s: %w", what, err)
	}
	defer closeRows(rows)
	for rows.Next() {
		var id, name string
		if err := rows.Scan(&id, &name); err != nil {
			return err
		}
		add(id, name)
	}
	return rows.Err()
}

// loadMemberships reads a user's group memberships and closes the result set before returning.
func (s *Service) loadMemberships(ctx context.Context, userID string) ([]membership, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT group_id, COALESCE(source, '') FROM group_members WHERE user_id = ? LIMIT 10000`, userID)
	if err != nil {
		return nil, fmt.Errorf("loading memberships: %w", err)
	}
	defer closeRows(rows)
	var members []membership
	for rows.Next() {
		var m membership
		if err := rows.Scan(&m.GroupID, &m.Source); err != nil {
			return nil, err
		}
		members = append(members, m)
	}
	return members, rows.Err()
}

func (s *Service) userMembershipsAndDirect(ctx context.Context, userID string) ([]membership, map[string]bool, error) {
	members, err := s.loadMemberships(ctx, userID)
	if err != nil {
		return nil, nil, err
	}

	directRows, err := s.db.QueryContext(ctx, `
		SELECT role_id, COALESCE(environment_id, ''), COALESCE(stack_name, '')
		  FROM user_roles WHERE user_id = ? LIMIT 10000`, userID)
	if err != nil {
		return nil, nil, fmt.Errorf("loading direct roles: %w", err)
	}
	defer closeRows(directRows)
	direct := map[string]bool{}
	for directRows.Next() {
		var g RoleGrantView
		if err := directRows.Scan(&g.RoleID, &g.EnvironmentID, &g.StackName); err != nil {
			return nil, nil, err
		}
		direct[grantKey(g)] = true
	}
	return members, direct, directRows.Err()
}

// recordGroupClaims stores the groups a login carried so a later mapping change
// can be previewed against what each user actually presents, not a guess.
func (s *Service) recordGroupClaims(ctx context.Context, providerID, userID string, claims []string) {
	encoded, err := json.Marshal(normaliseClaims(claims))
	if err != nil {
		return
	}
	now := time.Now().UTC().Format(time.RFC3339)
	res, err := s.db.ExecContext(ctx,
		`UPDATE identity_group_claims SET groups_json = ?, seen_at = ?, updated_at = ? WHERE provider_id = ? AND user_id = ?`,
		string(encoded), now, now, providerID, userID)
	if err == nil {
		if db.RowsAffected(res) > 0 {
			return
		}
		_, err = s.db.ExecContext(ctx,
			`INSERT INTO identity_group_claims (id, provider_id, user_id, groups_json, seen_at, created_at, updated_at)
			 VALUES (?, ?, ?, ?, ?, ?, ?)`,
			xid.New().String(), providerID, userID, string(encoded), now, now, now)
	}
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		logIdentityWarning("recording identity group claims", providerID, userID, err)
	}
}

func normaliseClaims(claims []string) []string {
	out := make([]string, 0, len(claims))
	seen := map[string]bool{}
	for _, c := range claims {
		c = strings.TrimSpace(c)
		if c == "" || seen[c] {
			continue
		}
		seen[c] = true
		out = append(out, c)
	}
	sort.Strings(out)
	return out
}

func containsString(list []string, value string) bool {
	for _, v := range list {
		if v == value {
			return true
		}
	}
	return false
}
