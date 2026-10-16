// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package stacks

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/therealmcsparrow/mcharbor/core/docker"
)

const symlinkStackCompose = "services:\n  web:\n    image: nginx\n    volumes:\n      - /srv/site:/html\n  sneaky:\n    image: docker:cli\n    volumes:\n      - /tmp/x/s:/var/run/docker.sock\n"

func writeSymlinkStack(t *testing.T) string {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "linked")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "docker-compose.yml"), []byte(symlinkStackCompose), 0o600); err != nil {
		t.Fatal(err)
	}
	return dir
}

// TestComposeUpWithASymlinkedBindIsRefused is TOFIX-03 for compose: an up whose service binds
// a planted link is refused with the service named, after one probe covering every service.
func TestComposeUpWithASymlinkedBindIsRefused(t *testing.T) {
	svc, self := sgService(t)
	self.links = map[string]bool{"/tmp/x/s": true}
	dir := writeSymlinkStack(t)

	ref := svc.composeHostAccessRefusal(context.Background(), "docker-compose.yml", dir, nil, "envAAA")
	if ref == nil || !slices.Contains(ref.Reasons, "sneaky:"+docker.HostAccessBindSymlink) || slices.Contains(ref.Reasons, "web:"+docker.HostAccessBindSymlink) {
		t.Fatalf("refusal %+v; want sneaky:%s only", ref, docker.HostAccessBindSymlink)
	}
	if len(self.probes) != 1 || len(self.probes[0]) != 2 {
		t.Fatalf("probes %v; want one probe over both sources", self.probes)
	}

	self.links = nil
	if ref := svc.composeHostAccessRefusal(context.Background(), "docker-compose.yml", dir, nil, "envAAA"); ref != nil {
		t.Fatalf("the same compose with no link was refused: %+v", ref)
	}
	self.probeErr = errors.New("helper timed out")
	ref = svc.composeHostAccessRefusal(context.Background(), "docker-compose.yml", dir, nil, "envAAA")
	if ref == nil || !slices.Contains(ref.Reasons, docker.HostAccessUnresolved) {
		t.Fatalf("a failed probe let the up through: %+v", ref)
	}
}

// TestComposeSaveDoesNotProbe keeps the helper to the up: a link planted after a save would
// beat a save-time check anyway, so a save runs only the string checks.
func TestComposeSaveDoesNotProbe(t *testing.T) {
	svc, self := sgService(t)
	self.links = map[string]bool{"/tmp/x/s": true}
	reasons, _ := svc.composeHostAccessReasons(context.Background(), []byte(symlinkStackCompose), nil, writeSymlinkStack(t), nil, "envAAA", false)
	if len(reasons) != 0 || len(self.probes) != 0 {
		t.Fatalf("a save probed or refused: reasons %v, probes %v", reasons, self.probes)
	}
}
