// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package rbac

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// The permission lifecycle spans five places that are maintained by hand and
// checked by nobody:
//
//  1. a Permission const in permissions.go
//  2. an entry in CatalogSeed() with a category, description and scope
//  3. membership in AllPermissions
//  4. a key in PERM in src/frontend/core/auth/permissionKeys.ts
//  5. a RequirePermission on the route that uses it
//
// A const with no CatalogSeed entry has no scope, is invisible in the role
// editor, and fails closed for everyone — while looking, in the diff, exactly
// like a governed capability. These tests make all four static layers agree.
// Layer 5 is covered by the apispec registry, which records the permission off
// the middleware chain itself.

// knownDrift is the backlog that predates the contract test. It is SHRINK-ONLY:
// removing a line as the drift is fixed is the point, and a test failure that
// tempts you to add one means a new capability was shipped half-governed.
//
// Seeded 2026-09-25. The four entries are keys with three dotted segments
// rather than the <category>.<action> the permission rule documents. They are
// live, granted, and referenced from role presets and the frontend, so renaming
// them is a migration and not a tidy-up — recorded here so the deviation stays
// visible instead of being quietly normalised away.
var knownDrift = map[string]string{
	"account.self.manage":    "three segments; renaming touches role presets and stored grants",
	"account.password.reset": "three segments; renaming touches role presets and stored grants",
	"rbac.policies.view":     "three segments; renaming touches role presets and stored grants",
	"rbac.policies.manage":   "three segments; renaming touches role presets and stored grants",
}

// permKeyRe enforces what the plugin installer actually rejects: anything that
// is not lowercase dotted snake_case. Hyphens and colons are the failure that
// blocked every comm plugin at install time.
var permKeyRe = regexp.MustCompile(`^[a-z][a-z0-9_]*(\.[a-z][a-z0-9_]*)+$`)

// conventionRe is the stricter two-segment form the permission rule documents.
// Deviations are reported against knownDrift rather than failing outright.
var conventionRe = regexp.MustCompile(`^[a-z][a-z0-9_]*\.[a-z][a-z0-9_]*$`)

var validScopes = map[string]bool{
	"global": true, "environment": true, "stack": true, "owner": true, "account": true,
}

func catalogByKey(t *testing.T) map[string]PermissionEntry {
	t.Helper()
	out := make(map[string]PermissionEntry)
	for _, e := range CatalogSeed() {
		key := string(e.Key)
		if _, dup := out[key]; dup {
			t.Errorf("CatalogSeed lists %q twice", key)
		}
		out[key] = e
	}
	return out
}

func allPermissionSet() map[string]bool {
	all := AllPermissions()
	out := make(map[string]bool, len(all))
	for _, p := range all {
		out[string(p)] = true
	}
	return out
}

// frontendPermKeys parses the PERM object in the hand-maintained frontend
// mirror. It is deliberately a text scan: importing TypeScript is not possible
// here, and the file's shape is stable and simple.
func frontendPermKeys(t *testing.T) map[string]bool {
	t.Helper()
	path := filepath.Join("..", "..", "..", "frontend", "core", "auth", "permissionKeys.ts")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("cannot read the frontend permission mirror at %s: %v", path, err)
	}

	re := regexp.MustCompile(`(?m)^\s*[A-Z0-9_]+\s*:\s*'([^']+)'`)
	out := map[string]bool{}
	for _, m := range re.FindAllStringSubmatch(string(raw), -1) {
		out[m[1]] = true
	}
	if len(out) == 0 {
		t.Fatal("parsed zero keys from permissionKeys.ts; the parser is broken, not the mirror")
	}
	return out
}

func sortedKeys(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func TestEveryPermissionIsInTheCatalog(t *testing.T) {
	catalog := catalogByKey(t)

	var missing []string
	for _, p := range AllPermissions() {
		key := string(p)
		if _, ok := catalog[key]; !ok {
			if _, excused := knownDrift[key]; excused {
				continue
			}
			missing = append(missing, key)
		}
	}
	sort.Strings(missing)

	if len(missing) > 0 {
		t.Errorf("%d permission(s) in AllPermissions have no CatalogSeed() entry, so they have no "+
			"scope and are invisible in the role editor:\n  %s",
			len(missing), strings.Join(missing, "\n  "))
	}
}

func TestEveryCatalogEntryIsInAllPermissions(t *testing.T) {
	all := allPermissionSet()

	var missing []string
	for _, e := range CatalogSeed() {
		key := string(e.Key)
		if !all[key] {
			if _, excused := knownDrift[key]; excused {
				continue
			}
			missing = append(missing, key)
		}
	}
	sort.Strings(missing)

	if len(missing) > 0 {
		t.Errorf("%d catalog entr(ies) are missing from AllPermissions:\n  %s",
			len(missing), strings.Join(missing, "\n  "))
	}
}

func TestCatalogEntriesAreWellFormed(t *testing.T) {
	for _, e := range CatalogSeed() {
		key := string(e.Key)
		if !permKeyRe.MatchString(key) {
			t.Errorf("%q is not <category>.<action> in lowercase snake_case; the plugin "+
				"installer rejects keys with hyphens or colons", key)
		}
		if e.Category == "" {
			t.Errorf("%s has no category", key)
		}
		if e.Action == "" {
			t.Errorf("%s has no action", key)
		}
		if strings.TrimSpace(e.Description) == "" {
			t.Errorf("%s has no description; the role editor shows the key alone", key)
		}
		if !validScopes[e.Scope] {
			t.Errorf("%s has scope %q, which is not one of global, environment, stack, owner, account",
				key, e.Scope)
		}
		if want := strings.SplitN(key, ".", 2); len(want) == 2 && want[0] != e.Category {
			t.Errorf("%s declares category %q, which does not match the key's own prefix %q",
				key, e.Category, want[0])
		}
	}
}

// TestKeysFollowTheDocumentedConvention reports keys that work but do not match
// the <category>.<action> shape the permission rule specifies. Every current
// deviation is in knownDrift; a new one is a finding.
func TestKeysFollowTheDocumentedConvention(t *testing.T) {
	var deviating []string
	for _, e := range CatalogSeed() {
		key := string(e.Key)
		if conventionRe.MatchString(key) {
			continue
		}
		if _, excused := knownDrift[key]; excused {
			continue
		}
		deviating = append(deviating, key)
	}
	sort.Strings(deviating)

	if len(deviating) > 0 {
		t.Errorf("%d permission key(s) are not <category>.<action>:\n  %s",
			len(deviating), strings.Join(deviating, "\n  "))
	}
}

func TestFrontendMirrorMatchesBackendCatalog(t *testing.T) {
	frontend := frontendPermKeys(t)
	backend := allPermissionSet()
	for _, e := range CatalogSeed() {
		backend[string(e.Key)] = true
	}

	var missingInFrontend, extraInFrontend []string
	for key := range backend {
		if !frontend[key] {
			if _, excused := knownDrift[key]; excused {
				continue
			}
			missingInFrontend = append(missingInFrontend, key)
		}
	}
	for key := range frontend {
		if !backend[key] {
			if _, excused := knownDrift[key]; excused {
				continue
			}
			extraInFrontend = append(extraInFrontend, key)
		}
	}
	sort.Strings(missingInFrontend)
	sort.Strings(extraInFrontend)

	if len(missingInFrontend) > 0 {
		t.Errorf("%d backend permission(s) have no PERM entry in permissionKeys.ts, so no button "+
			"can be gated on them without a bare string literal:\n  %s",
			len(missingInFrontend), strings.Join(missingInFrontend, "\n  "))
	}
	if len(extraInFrontend) > 0 {
		t.Errorf("%d PERM entr(ies) in permissionKeys.ts name a permission the backend does not "+
			"define, so any gate using them denies everyone forever:\n  %s",
			len(extraInFrontend), strings.Join(extraInFrontend, "\n  "))
	}
}

// TestKnownDriftIsShrinkOnly fails when the baseline is padded rather than
// drained. The count is the ratchet: it may fall, never rise.
func TestKnownDriftIsShrinkOnly(t *testing.T) {
	const seeded = 4 // 2026-09-25 baseline: four three-segment keys
	if len(knownDrift) > seeded {
		t.Errorf("knownDrift has grown from %d to %d entries. It is shrink-only: a new "+
			"half-governed permission is a defect, not a baseline. Entries added:\n  %s",
			seeded, len(knownDrift), strings.Join(sortedKeys(driftKeys()), "\n  "))
	}
}

func driftKeys() map[string]bool {
	out := map[string]bool{}
	for k := range knownDrift {
		out[k] = true
	}
	return out
}
