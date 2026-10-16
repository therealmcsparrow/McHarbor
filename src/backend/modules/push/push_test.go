// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package push

import (
	"context"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	webpush "github.com/SherClockHolmes/webpush-go"
	_ "modernc.org/sqlite"

	"github.com/therealmcsparrow/mcharbor/core/alertnotice"
	coredb "github.com/therealmcsparrow/mcharbor/core/db"
	"github.com/therealmcsparrow/mcharbor/core/encryption"
	"github.com/therealmcsparrow/mcharbor/core/rbac"
)

type fakeSender struct {
	mu       sync.Mutex
	status   map[string]int
	sent     []string
	payloads [][]byte
}

func (f *fakeSender) Send(_ context.Context, payload []byte, sub *webpush.Subscription, _ *webpush.Options) (int, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.sent = append(f.sent, sub.Endpoint)
	f.payloads = append(f.payloads, payload)
	if code, ok := f.status[sub.Endpoint]; ok {
		return code, nil
	}
	return 201, nil
}

func validRequest(endpoint string) SubscribeRequest {
	var req SubscribeRequest
	req.Endpoint = endpoint
	p := make([]byte, 65)
	p[0] = 4
	req.Keys.P256dh = base64.RawURLEncoding.EncodeToString(p)
	req.Keys.Auth = base64.RawURLEncoding.EncodeToString(make([]byte, 16))
	req.Label = "phone"
	return req
}

func newTestService(t *testing.T) (*Service, *sql.DB, *fakeSender) {
	t.Helper()
	database, err := sql.Open("sqlite", "file:"+filepath.Join(t.TempDir(), "push.db")+"?_pragma=foreign_keys(1)")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := database.Close(); err != nil {
			t.Error(err)
		}
	})
	if err := coredb.Migrate(database, coredb.DriverSQLite); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	for _, stmt := range []string{
		`INSERT INTO users (id, username, password_hash) VALUES ('alice', 'alice', 'x'), ('bob', 'bob', 'x')`,
		`INSERT INTO roles (id, name, permissions) VALUES ('r-alerts', 'r-alerts', '["alerts.view"]')`,
		`INSERT INTO user_roles (id, user_id, role_id) VALUES ('ur1', 'alice', 'r-alerts')`,
	} {
		if _, err := database.Exec(stmt); err != nil {
			t.Fatalf("seed: %v", err)
		}
	}
	enc, err := encryption.New(t.TempDir(), "")
	if err != nil {
		t.Fatal(err)
	}
	fake := &fakeSender{status: map[string]int{}}
	svc := NewService(database, enc, rbac.NewService(database), slog.New(slog.NewTextHandler(io.Discard, nil)), "https://example.test")
	svc.sender = fake
	return svc, database, fake
}

func TestOnlyKnownPushServicesAreAccepted(t *testing.T) {
	good := []string{
		"https://fcm.googleapis.com/fcm/send/abc",
		"https://updates.push.services.mozilla.com/wpush/v2/abc",
		"https://web.push.apple.com/QGuQyavXutnMei",
		"https://wns2-par02p.notify.windows.com/w/?token=abc",
	}
	for _, e := range good {
		if err := validateSubscription(validRequest(e)); err != nil {
			t.Errorf("%s rejected: %v", e, err)
		}
	}
	bad := []string{
		"http://fcm.googleapis.com/fcm/send/abc",
		"https://169.254.169.254/latest/meta-data",
		"https://localhost/push",
		"https://mcharbor.internal/push",
		"https://fcm.googleapis.com.evil.test/x",
		"https://fcm.googleapis.com:8443/x",
		"https://user@fcm.googleapis.com/x",
	}
	for _, e := range bad {
		if err := validateSubscription(validRequest(e)); !errors.Is(err, ErrInvalidSubscription) {
			t.Errorf("%s accepted; the server would POST to it", e)
		}
	}
	short := validRequest(good[0])
	short.Keys.Auth = base64.RawURLEncoding.EncodeToString(make([]byte, 8))
	if err := validateSubscription(short); !errors.Is(err, ErrInvalidSubscription) {
		t.Error("an 8-byte auth secret was accepted")
	}
}

func TestSubscriptionsAreOwnedAndNeverExposeCredentials(t *testing.T) {
	svc, database, _ := newTestService(t)
	ctx := context.Background()
	sub, err := svc.Subscribe(ctx, "alice", validRequest("https://fcm.googleapis.com/fcm/send/alice-1"))
	if err != nil {
		t.Fatal(err)
	}
	again, err := svc.Subscribe(ctx, "alice", validRequest("https://fcm.googleapis.com/fcm/send/alice-1"))
	if err != nil || again.ID != sub.ID {
		t.Fatalf("resubscribing the same browser made a new device: %v %v", again, err)
	}
	if err := svc.Delete(ctx, "bob", sub.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("bob deleted alice's device: %v", err)
	}
	list, err := svc.List(ctx, "alice")
	if err != nil || len(list) != 1 {
		t.Fatalf("list = %v, %v", list, err)
	}
	raw, err := json.Marshal(list)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), "fcm.googleapis.com") || strings.Contains(strings.ToLower(string(raw)), "p256dh") {
		t.Fatalf("the device list exposes credentials: %s", raw)
	}
	var stored string
	if err := database.QueryRow(`SELECT endpoint FROM push_subscriptions WHERE id = ?`, sub.ID).Scan(&stored); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(stored, "fcm.googleapis.com") {
		t.Fatal("the endpoint is stored in plaintext")
	}
	if err := svc.Delete(ctx, "alice", sub.ID); err != nil {
		t.Fatal(err)
	}
}

func TestDeliveryRespectsPermissionsAndPrunesDeadDevices(t *testing.T) {
	svc, database, fake := newTestService(t)
	ctx := context.Background()
	for _, s := range []struct{ user, endpoint string }{
		{"alice", "https://fcm.googleapis.com/fcm/send/alive"},
		{"alice", "https://fcm.googleapis.com/fcm/send/gone"},
		{"alice", "https://fcm.googleapis.com/fcm/send/flaky"},
		{"bob", "https://fcm.googleapis.com/fcm/send/bob"},
	} {
		if _, err := svc.Subscribe(ctx, s.user, validRequest(s.endpoint)); err != nil {
			t.Fatal(err)
		}
	}
	fake.status["https://fcm.googleapis.com/fcm/send/gone"] = 410
	fake.status["https://fcm.googleapis.com/fcm/send/flaky"] = 503

	svc.deliver(ctx, Notification{Title: "Alert triggered: disk", Body: "90%", Severity: "warning", URL: "/oncall/alerts"},
		Audience{Permission: rbac.PermAlertsView})

	for _, e := range fake.sent {
		if strings.HasSuffix(e, "/bob") {
			t.Fatal("bob holds no alerts.view but was notified")
		}
	}
	if len(fake.sent) != 3 {
		t.Fatalf("sent to %d devices, want alice's 3", len(fake.sent))
	}
	var n int
	if err := database.QueryRow(`SELECT COUNT(*) FROM push_subscriptions WHERE user_id = 'alice'`).Scan(&n); err != nil || n != 2 {
		t.Fatalf("alice has %d devices, want 2 after the 410 was removed: %v", n, err)
	}
	var failures int
	var next sql.NullString
	if err := database.QueryRow(`SELECT failure_count, next_attempt_at FROM push_subscriptions WHERE failure_count > 0`).Scan(&failures, &next); err != nil {
		t.Fatalf("the 503 was not recorded: %v", err)
	}
	if failures != 1 || !next.Valid {
		t.Fatalf("503 bookkeeping: failures=%d next=%v", failures, next)
	}

	fake.sent = nil
	svc.deliver(ctx, Notification{Title: "again"}, Audience{Permission: rbac.PermAlertsView})
	for _, e := range fake.sent {
		if strings.HasSuffix(e, "/flaky") {
			t.Fatal("a device in back-off was retried immediately")
		}
	}

	svc.now = func() time.Time { return time.Now().Add(31 * 24 * time.Hour) }
	svc.pruneStale(ctx)
	if err := database.QueryRow(`SELECT COUNT(*) FROM push_subscriptions WHERE failure_count > 0`).Scan(&n); err != nil || n != 0 {
		t.Fatalf("a device failing for 31 days survived: %d %v", n, err)
	}
}

func TestPayloadCarriesOnlyTheFiveFields(t *testing.T) {
	svc, _, fake := newTestService(t)
	ctx := context.Background()
	if _, err := svc.Subscribe(ctx, "alice", validRequest("https://fcm.googleapis.com/fcm/send/a")); err != nil {
		t.Fatal(err)
	}
	svc.deliver(ctx, Notification{Title: "t", Body: "b", Severity: "error", URL: "/oncall/alerts", Tag: "x"}, Audience{})
	var got map[string]any
	if err := json.Unmarshal(fake.payloads[0], &got); err != nil {
		t.Fatal(err)
	}
	for k := range got {
		switch k {
		case "title", "body", "severity", "url", "tag":
		default:
			t.Fatalf("the push payload leaks field %q", k)
		}
	}
}

func TestRegenerateRemovesEveryDeviceAndKeepsTheKeyEncrypted(t *testing.T) {
	svc, database, _ := newTestService(t)
	ctx := context.Background()
	first, err := svc.PublicKey(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Subscribe(ctx, "alice", validRequest("https://fcm.googleapis.com/fcm/send/a")); err != nil {
		t.Fatal(err)
	}
	second, removed, err := svc.Regenerate(ctx)
	if err != nil || second == first || removed != 1 {
		t.Fatalf("regenerate: key changed=%v removed=%d err=%v", second != first, removed, err)
	}
	var priv string
	if err := database.QueryRow(`SELECT value FROM settings WHERE key = ?`, settingPrivate).Scan(&priv); err != nil {
		t.Fatal(err)
	}
	_, plain, err := svc.keys(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if priv == plain || priv == "" {
		t.Fatal("the VAPID private key is stored in plaintext")
	}
}

func TestAlertNotificationDeepLinks(t *testing.T) {
	n := AlertNotification(alertnotice.Notice{AlertID: "a1", Title: "Alert triggered: web down", SourceType: alertnotice.SourceContainer,
		EnvironmentID: "e1", SubjectID: "abc123", Message: strings.Repeat("x", 500)})
	if n.URL != "/oncall/containers/e1/abc123" {
		t.Fatalf("container alert url = %q", n.URL)
	}
	if len([]rune(n.Body)) > 240 {
		t.Fatalf("body not clipped: %d runes", len([]rune(n.Body)))
	}
	if other := AlertNotification(alertnotice.Notice{AlertID: "a2", SourceType: "host"}); other.URL != "/oncall/alerts" {
		t.Fatalf("host alert url = %q", other.URL)
	}
}

func TestEachRecipientGetsTheirOwnLanguage(t *testing.T) {
	svc, database, fake := newTestService(t)
	ctx := context.Background()
	for _, stmt := range []string{
		`INSERT INTO users (id, username, password_hash, preferred_language) VALUES ('carl', 'carl', 'x', 'de'), ('dora', 'dora', 'x', '')`,
		`UPDATE users SET preferred_language = 'nl' WHERE id = 'alice'`,
		`UPDATE settings SET value = 'fr' WHERE key = 'default_language'`,
	} {
		if _, err := database.Exec(stmt); err != nil {
			t.Fatalf("seed: %v", err)
		}
	}
	for _, s := range []struct{ user, endpoint string }{
		{"alice", "https://fcm.googleapis.com/fcm/send/alice"},
		{"carl", "https://fcm.googleapis.com/fcm/send/carl"},
		{"dora", "https://fcm.googleapis.com/fcm/send/dora"},
	} {
		if _, err := svc.Subscribe(ctx, s.user, validRequest(s.endpoint)); err != nil {
			t.Fatal(err)
		}
	}
	n := AlertNotification(alertnotice.Notice{AlertID: "a1", Name: "disk", Title: "Alert triggered: disk", Message: "90% on /data", State: "firing"})
	svc.deliver(ctx, n, Audience{})

	got := map[string]map[string]any{}
	for i, endpoint := range fake.sent {
		var p map[string]any
		if err := json.Unmarshal(fake.payloads[i], &p); err != nil {
			t.Fatal(err)
		}
		got[endpoint[strings.LastIndex(endpoint, "/")+1:]] = p
		for k := range p {
			if k == "key" || k == "args" || k == "Key" || k == "Args" {
				t.Fatalf("the catalog key leaked into the payload: %v", p)
			}
		}
	}
	for user, want := range map[string]string{
		"alice": "Waarschuwing actief: disk",
		"carl":  "Alarm ausgelöst: disk",
		"dora":  "Alerte déclenchée : disk",
	} {
		if got[user]["title"] != want {
			t.Errorf("%s got %q, want %q", user, got[user]["title"], want)
		}
		if got[user]["body"] != "90% on /data" {
			t.Errorf("%s: the operator's own message was rewritten: %q", user, got[user]["body"])
		}
	}

	if _, err := database.Exec(`UPDATE settings SET value = 'xx' WHERE key = 'default_language'`); err != nil {
		t.Fatal(err)
	}
	if lang := svc.defaultLanguage(ctx); lang != "en" {
		t.Fatalf("an unsupported default gave %q", lang)
	}
}

func TestEveryCatalogLanguageHasEveryMessage(t *testing.T) {
	for lang, msgs := range catalog {
		for key := range catalog["en"] {
			if m, ok := msgs[key]; !ok || m.title == "" {
				t.Errorf("%s lacks %s", lang, key)
			}
		}
	}
	for _, lang := range []string{"en", "nl", "de", "es", "fr", "pt"} {
		if _, ok := catalog[lang]; !ok {
			t.Errorf("the catalog has no %s", lang)
		}
	}
}

// TestSubscriptionsAreCappedPerUser: every alert fans out to each device, so one
// account cannot register an unbounded number; a known browser still refreshes.
func TestSubscriptionsAreCappedPerUser(t *testing.T) {
	svc, _, _ := newTestService(t)
	ctx := context.Background()
	for i := 0; i < maxSubscriptionsPerUser; i++ {
		if _, err := svc.Subscribe(ctx, "alice", validRequest(fmt.Sprintf("https://fcm.googleapis.com/fcm/send/dev-%d", i))); err != nil {
			t.Fatalf("device %d under the cap: %v", i, err)
		}
	}
	if _, err := svc.Subscribe(ctx, "alice", validRequest("https://fcm.googleapis.com/fcm/send/one-too-many")); !errors.Is(err, ErrTooManySubscriptions) {
		t.Fatalf("the device past the cap: got %v, want ErrTooManySubscriptions", err)
	}
	if _, err := svc.Subscribe(ctx, "alice", validRequest("https://fcm.googleapis.com/fcm/send/dev-0")); err != nil {
		t.Fatalf("re-subscribing a known browser must still work: %v", err)
	}
}
