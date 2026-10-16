// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package secrets

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestRevealIsInNoRolePreset is the assertion step 6.7 of the secrets-manager
// plan asks for.
//
// `secrets.reveal` is obtained through an approved, time-bounded elevation and
// through nothing else. A role preset that carried it would hand a standing
// credential-disclosure right to everyone a role author built from that
// preset, silently, and the elevation machinery around it would become
// decorative.
//
// This reads the migration files rather than the database on purpose: a preset
// only ever gets a key by being written into one, and catching it at the
// source catches it before it ships.
//
// **If this fails, do not add the key to a preset "temporarily".** Grant the
// elevation instead — that is what it is for.
func TestRevealIsInNoRolePreset(t *testing.T) {
	dir := filepath.Join("..", "..", "core", "db", "migrations")
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("reading migrations: %v", err)
	}

	scanned := 0
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".sql") {
			continue
		}
		body, err := os.ReadFile(filepath.Join(dir, entry.Name()))
		if err != nil {
			t.Fatalf("reading %s: %v", entry.Name(), err)
		}
		scanned++

		for _, line := range strings.Split(string(body), "\n") {
			// Comments are where the exclusion is explained, so they are not
			// evidence of a grant.
			trimmed := strings.TrimSpace(line)
			if strings.HasPrefix(trimmed, "--") {
				continue
			}
			if !strings.Contains(line, "role_presets") && !strings.Contains(line, "'secrets.reveal'") {
				continue
			}
			if strings.Contains(line, "'secrets.reveal'") {
				t.Errorf("%s grants secrets.reveal to a role preset: %s",
					entry.Name(), strings.TrimSpace(line))
			}
		}
	}

	if scanned < 50 {
		t.Fatalf("only %d migrations scanned; the path is probably wrong and this test proved nothing", scanned)
	}
}

// TestManageGlobalIsInNoRolePreset guards the other key that deliberately
// reaches no preset: creating a credential every environment reads is an
// administrator action, and this install provisions no Administrator preset.
func TestManageGlobalIsInNoRolePreset(t *testing.T) {
	dir := filepath.Join("..", "..", "core", "db", "migrations")
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("reading migrations: %v", err)
	}

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".sql") {
			continue
		}
		body, err := os.ReadFile(filepath.Join(dir, entry.Name()))
		if err != nil {
			t.Fatalf("reading %s: %v", entry.Name(), err)
		}
		for _, line := range strings.Split(string(body), "\n") {
			if strings.HasPrefix(strings.TrimSpace(line), "--") {
				continue
			}
			if strings.Contains(line, "'secrets.manage_global'") {
				t.Errorf("%s grants secrets.manage_global to a role preset: %s",
					entry.Name(), strings.TrimSpace(line))
			}
		}
	}
}
