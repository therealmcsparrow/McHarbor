// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package procenv

import (
	"slices"
	"strings"
	"testing"
)

var environ = []string{
	"PATH=/usr/bin", "HOME=/root", "DOCKER_HOST=unix:///var/run/docker.sock", "COMPOSE_HTTP_TIMEOUT=120",
	"HTTPS_PROXY=http://proxy:3128", "no_proxy=localhost", "LC_ALL=C.UTF-8", "SystemRoot=C:\\Windows",
	"ENCRYPTION_KEY=root-encryption-key", "MCHARBOR_SECRET=shared-secret", "MCHARBOR_DB_DSN=postgres://u:p@db/x",
	"MCHARBOR_BACKUP_KEY=backup-key", "ENCRYPTION_KEY_FILE=/run/secrets/k", "SMTP_PASSWORD=mail-secret",
	"OIDC_CLIENT_SECRET=oidc", "DATA_DIR=/app/data",
}

// TestChildEnvironmentIsAnAllowList is H4 of the 2026-09-30 review: compose inherited
// McHarbor's whole environment, so `image: ${ENCRYPTION_KEY}` printed the key in an error
// and `labels: {x: ${MCHARBOR_DB_DSN}}` published the DSN on a viewer-readable label.
func TestChildEnvironmentIsAnAllowList(t *testing.T) {
	got := Child(environ)
	for _, want := range []string{
		"PATH=/usr/bin", "HOME=/root", "DOCKER_HOST=unix:///var/run/docker.sock", "COMPOSE_HTTP_TIMEOUT=120",
		"HTTPS_PROXY=http://proxy:3128", "no_proxy=localhost", "LC_ALL=C.UTF-8", "SystemRoot=C:\\Windows",
	} {
		if !slices.Contains(got, want) {
			t.Errorf("child environment lost %q, which docker compose needs", want)
		}
	}
	for _, entry := range got {
		for _, leak := range []string{"ENCRYPTION_KEY", "MCHARBOR_SECRET", "MCHARBOR_DB_DSN", "MCHARBOR_BACKUP_KEY", "SMTP_PASSWORD", "OIDC_CLIENT_SECRET", "DATA_DIR"} {
			if strings.HasPrefix(entry, leak) {
				t.Errorf("child environment carries %q", entry)
			}
		}
	}
}

// TestWithoutRootOfTrustKeepsTheRest backs the backup-key helper, which runs McHarbor's own
// binary and needs its ordinary configuration but never its keys.
func TestWithoutRootOfTrustKeepsTheRest(t *testing.T) {
	got := WithoutRootOfTrust(environ)
	for _, entry := range got {
		name, _, _ := strings.Cut(entry, "=")
		if isRootOfTrust(name) {
			t.Errorf("helper environment carries %q", entry)
		}
	}
	if !slices.Contains(got, "DATA_DIR=/app/data") || !slices.Contains(got, "SMTP_PASSWORD=mail-secret") {
		t.Errorf("helper environment lost ordinary configuration: %q", got)
	}
}
