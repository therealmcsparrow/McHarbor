// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package envredact

import (
	"errors"
	"strings"
	"testing"
)

const storedCompose = `# stack file
x-common-env: &common
  SHARED_TOKEN: shared-token-value
services:
  web:
    image: nginx:1.27
    env_file: ./web.env
    environment:
      <<: *common
      DB_PASSWORD: hunter2hunter2   # keep this comment
      QUOTED_SECRET: "quoted secret value"
      MODE: blue
      FROM_ENV: ${DB_PASSWORD}
      EMPTY_KEY:
  worker:
    image: busybox
    environment:
      - API_TOKEN=tok-abcdef
      - "PRIVATE_KEY=pk value"
      - PLAIN=1
  mcharbor:
    image: ghcr.io/therealmcsparrow/mcharbor:latest
    environment:
      ENCRYPTION_KEY: root-key-value-123
      LOG_LEVEL: info
`

func isMcHarborForTest(service, image string) bool {
	return service == "mcharbor" || strings.HasPrefix(image, "ghcr.io/therealmcsparrow/mcharbor")
}

// TestComposeWithholdsEnvironmentValuesInPlace is the viewer's compose tab: credentials under
// environment: in both forms are masked, everything else — comments, env_file, references,
// ordinary values — reads exactly as the operator wrote it.
func TestComposeWithholdsEnvironmentValuesInPlace(t *testing.T) {
	got, withheld := Policy{}.Compose(storedCompose, isMcHarborForTest)
	for _, secret := range []string{"hunter2hunter2", "quoted secret value", "tok-abcdef", "pk value", "shared-token-value", "root-key-value-123"} {
		if strings.Contains(got, secret) {
			t.Errorf("masked compose still contains %q:\n%s", secret, got)
		}
	}
	for _, kept := range []string{
		"# stack file", "env_file: ./web.env", "DB_PASSWORD: " + Marker + "   # keep this comment",
		`QUOTED_SECRET: "` + Marker + `"`, "MODE: blue", "FROM_ENV: ${DB_PASSWORD}", "EMPTY_KEY:",
		"- API_TOKEN=" + Marker, `- "PRIVATE_KEY=` + Marker + `"`, "- PLAIN=1", "LOG_LEVEL: info",
	} {
		if !strings.Contains(got, kept) {
			t.Errorf("masked compose lost %q:\n%s", kept, got)
		}
	}
	if len(withheld) != 6 {
		t.Errorf("withheld = %q, want six names", withheld)
	}
}

// TestComposeRevealKeepsTheRootOfTrust holds for a stacks.manage reveal: McHarbor's own keys,
// and any value carrying a key this process runs with, stay masked.
func TestComposeRevealKeepsTheRootOfTrust(t *testing.T) {
	got, _ := Policy{Reveal: true, Pinned: PinnedValues("shared-token-value")}.Compose(storedCompose, isMcHarborForTest)
	if !strings.Contains(got, "DB_PASSWORD: hunter2hunter2") || !strings.Contains(got, "- API_TOKEN=tok-abcdef") {
		t.Errorf("a permitted reveal withheld ordinary values:\n%s", got)
	}
	if strings.Contains(got, "root-key-value-123") || strings.Contains(got, "shared-token-value") {
		t.Errorf("a reveal returned McHarbor's own key or a pinned value:\n%s", got)
	}
}

// TestRestoreComposeKeepsStoredValues is the write path: saving the masked text, with one
// ordinary edit, writes the stored credentials back and never the marker.
func TestRestoreComposeKeepsStoredValues(t *testing.T) {
	masked, _ := Policy{}.Compose(storedCompose, isMcHarborForTest)
	edited := strings.Replace(masked, "MODE: blue", "MODE: green", 1)
	got, err := RestoreCompose(edited, storedCompose)
	if err != nil {
		t.Fatalf("RestoreCompose: %v", err)
	}
	want := strings.Replace(storedCompose, "MODE: blue", "MODE: green", 1)
	if got != want {
		t.Fatalf("restored compose differs from the stored file plus the edit:\n%s\n--- want ---\n%s", got, want)
	}
}

// TestRestoreComposeRefusesAMarkerItCannotPlace keeps bullets off disk when there is no value
// to put back or the marker sits where no environment value lives.
func TestRestoreComposeRefusesAMarkerItCannotPlace(t *testing.T) {
	masked, _ := Policy{}.Compose(storedCompose, isMcHarborForTest)
	cases := map[string]string{
		"new key":        strings.Replace(masked, "MODE: blue", "NEW_SECRET: "+Marker, 1),
		"renamed svc":    strings.Replace(masked, "  worker:", "  jobs:", 1),
		"outside env":    strings.Replace(masked, "image: busybox", "image: "+Marker, 1),
		"block scalar":   strings.Replace(masked, "DB_PASSWORD: "+Marker, "DB_PASSWORD: |\n        "+Marker, 1),
		"does not parse": masked + "\n  : : [",
	}
	for name, doc := range cases {
		if out, err := RestoreCompose(doc, storedCompose); !errors.Is(err, ErrComposeMaskedValue) {
			t.Errorf("%s: err = %v, want ErrComposeMaskedValue (output contains marker: %v)", name, err, strings.Contains(out, Marker))
		}
	}
	if out, err := RestoreCompose("services: {}\n", storedCompose); err != nil || out != "services: {}\n" {
		t.Errorf("a document without the marker must pass through: %q, %v", out, err)
	}
}

// TestComposeThatDoesNotParseIsStillMasked covers a half-edited file a viewer opens: the
// walk cannot run, so assignment-shaped lines are masked instead.
func TestComposeThatDoesNotParseIsStillMasked(t *testing.T) {
	got, withheld := Policy{}.Compose("services:\n  web:\n    environment:\n      DB_PASSWORD: hunter2hunter2\n  : : [\n", nil)
	if strings.Contains(got, "hunter2hunter2") || len(withheld) != 1 {
		t.Fatalf("unparseable compose leaked a value: %q (%q)", got, withheld)
	}
}
