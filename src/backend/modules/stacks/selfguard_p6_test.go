// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package stacks

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/therealmcsparrow/mcharbor/core/auth"
	"github.com/therealmcsparrow/mcharbor/core/selfguard"
)

const p6SockCompose = "services:\n  sock:\n    image: docker:cli\n    volumes:\n      - /var/run/docker.sock:/var/run/docker.sock\n"

// TestBackgroundComposeUpIsRefused is M1: a secret-rotation apply, a webhook or an App Store
// autostart has no principal, so compose that reaches McHarbor's host is refused there, while
// an ordinary App Store compose (app-data bind, named volume) still comes up.
func TestBackgroundComposeUpIsRefused(t *testing.T) {
	svc, _ := sgService(t)
	dir := filepath.Join(t.TempDir(), "probe")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	write := func(content string) {
		if err := os.WriteFile(filepath.Join(dir, "docker-compose.yml"), []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	write(p6SockCompose)
	if ref := svc.composeHostAccessRefusal(context.Background(), "docker-compose.yml", dir, nil, "envAAA"); ref == nil {
		t.Fatal("a background up of socket compose on the self environment was let through")
	}
	appStore := "services:\n  jellyfin:\n    image: jellyfin/jellyfin\n    volumes:\n      - /opt/appdata/jellyfin/config:/config\n      - cache:/cache\nvolumes:\n  cache: {}\n"
	write(appStore)
	if ref := svc.composeHostAccessRefusal(context.Background(), "docker-compose.yml", dir, nil, "envAAA"); ref != nil {
		t.Fatalf("a background up of an ordinary App Store compose was refused: %+v", ref)
	}
}

// TestSavingHostAccessComposeIsRefused is M1's save half: PUT /stacks/{name} runs the compose
// check before anything is written, so the content never lands for a later up to find.
func TestSavingHostAccessComposeIsRefused(t *testing.T) {
	svc, _ := sgService(t)
	dir := filepath.Join(t.TempDir(), "saved")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	composePath := filepath.Join(dir, "docker-compose.yml")
	original := "services:\n  web:\n    image: nginx\n"
	if err := os.WriteFile(composePath, []byte(original), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.db.Exec(`INSERT INTO stacks (id, name, environment_id, project_path, compose_file, status, created_at, updated_at)
		VALUES ('st-s', 'saved', 'envAAA', ?, 'docker-compose.yml', 'running', '2026-09-30', '2026-09-30')`, dir); err != nil {
		t.Fatal(err)
	}
	user := context.WithValue(context.Background(), auth.UserContextKey, &auth.User{ID: "ops"})
	sock := p6SockCompose
	if _, err := svc.Update(selfguard.WithDecision(user, false), "saved", UpdateRequest{Compose: &sock}); !errors.Is(err, selfguard.ErrRequired) {
		t.Fatalf("saving socket compose without host_self.manage: %v; want a host_self refusal", err)
	}
	if got, err := os.ReadFile(composePath); err != nil || string(got) != original {
		t.Fatalf("the refused save rewrote the compose file: %q %v", got, err)
	}
	if _, err := svc.Update(selfguard.WithDecision(user, true), "saved", UpdateRequest{Compose: &sock}); err != nil {
		t.Fatalf("saving socket compose with host_self.manage: %v", err)
	}
}
