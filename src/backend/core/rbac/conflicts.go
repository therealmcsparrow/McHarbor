// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package rbac

import (
	"regexp"
	"sort"
	"strings"
)

// Conflict kinds reported by DetectConflicts.
const (
	ConflictWildcardGrant    = "wildcard_grant"
	ConflictCategoryWildcard = "category_wildcard"
	ConflictPolicyShadowed   = "policy_shadowed"
	ConflictPolicyTie        = "policy_tie"
)

// ConflictWarning is one finding about the grant model. It names the subject and,
// for a shadowed policy, the policy that shadows it, so the UI can link both.
type ConflictWarning struct {
	Kind        string   `json:"kind"`
	Severity    string   `json:"severity"`
	SubjectType string   `json:"subjectType"`
	SubjectID   string   `json:"subjectId"`
	SubjectName string   `json:"subjectName"`
	RelatedID   string   `json:"relatedId,omitempty"`
	RelatedName string   `json:"relatedName,omitempty"`
	Permissions []string `json:"permissions"`
}

// RoleGrantSet is the part of a role conflict detection reads.
type RoleGrantSet struct {
	ID          string
	Name        string
	Permissions []string
	IsSystem    bool
}

var categoryWildcardRe = regexp.MustCompile(`^[a-z][a-z0-9_.]*\.\*$`)

// DetectConflicts reports wildcard grants on roles and ABAC policies that can
// never decide because an earlier policy always decides first. It is pure so the
// rules can be tested without a database.
//
// A "category.*" grant is flagged separately: the engine matches keys exactly and
// only "*" is a wildcard, so such an entry silently grants nothing.
func DetectConflicts(roles []RoleGrantSet, policies []ABACPolicy) []ConflictWarning {
	out := []ConflictWarning{}
	for _, r := range roles {
		var category []string
		for _, p := range r.Permissions {
			switch {
			case Permission(p) == PermWildcard:
				severity := "warning"
				if r.IsSystem {
					severity = "info"
				}
				out = append(out, ConflictWarning{Kind: ConflictWildcardGrant, Severity: severity, SubjectType: "role",
					SubjectID: r.ID, SubjectName: r.Name, Permissions: []string{p}})
			case categoryWildcardRe.MatchString(p):
				category = append(category, p)
			}
		}
		if len(category) > 0 {
			out = append(out, ConflictWarning{Kind: ConflictCategoryWildcard, Severity: "warning", SubjectType: "role",
				SubjectID: r.ID, SubjectName: r.Name, Permissions: category})
		}
	}
	return append(out, policyConflicts(policies)...)
}

// policyConflicts compares every enabled policy pair in evaluation order.
func policyConflicts(policies []ABACPolicy) []ConflictWarning {
	enabled := make([]ABACPolicy, 0, len(policies))
	for _, p := range policies {
		if p.Enabled {
			enabled = append(enabled, p)
		}
	}
	sort.SliceStable(enabled, func(i, j int) bool {
		if enabled[i].Priority != enabled[j].Priority {
			return enabled[i].Priority < enabled[j].Priority
		}
		return enabled[i].Name < enabled[j].Name
	})

	var out []ConflictWarning
	for j := range enabled {
		later := enabled[j]
		for i := 0; i < j; i++ {
			earlier := enabled[i]
			shared := sharedPermissions(earlier.Permissions, later.Permissions)
			if len(shared) == 0 || !resourceCovers(earlier.ResourceType, later.ResourceType) {
				continue
			}
			if alwaysMatches(earlier.Expression) || normalizeExpression(earlier.Expression) == normalizeExpression(later.Expression) {
				out = append(out, ConflictWarning{Kind: ConflictPolicyShadowed, Severity: "warning", SubjectType: "policy",
					SubjectID: later.ID, SubjectName: later.Name, RelatedID: earlier.ID, RelatedName: earlier.Name, Permissions: shared})
				break
			}
			if earlier.Priority == later.Priority && earlier.Effect != later.Effect {
				out = append(out, ConflictWarning{Kind: ConflictPolicyTie, Severity: "info", SubjectType: "policy",
					SubjectID: later.ID, SubjectName: later.Name, RelatedID: earlier.ID, RelatedName: earlier.Name, Permissions: shared})
			}
		}
	}
	return out
}

// resourceCovers reports whether a policy filtered on earlier would also be
// evaluated for every request the later policy sees.
func resourceCovers(earlier, later string) bool {
	return earlier == "" || earlier == later
}

func sharedPermissions(a, b []string) []string {
	set := make(map[string]bool, len(a))
	for _, p := range a {
		set[p] = true
	}
	var out []string
	for _, p := range b {
		if set[p] {
			out = append(out, p)
		}
	}
	sort.Strings(out)
	return out
}

func normalizeExpression(expr string) string {
	var b strings.Builder
	for _, r := range expr {
		switch r {
		case ' ', '\t', '\n', '\r', ';':
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}

// alwaysMatches recognises the literal forms of an expression that is true for
// every request; anything subtler is left alone rather than guessed at.
func alwaysMatches(expr string) bool {
	n := strings.Trim(normalizeExpression(expr), "()")
	switch n {
	case "true", "returntrue", "1", "!0", "!!1", "!false":
		return true
	}
	return false
}
