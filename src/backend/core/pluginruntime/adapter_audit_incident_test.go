// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package pluginruntime

import (
	"context"
	"database/sql"
	"io"
	"log/slog"
	"net/http"
	"path/filepath"
	"testing"

	_ "modernc.org/sqlite"

	"github.com/therealmcsparrow/mcharbor/core/audit"
	coredb "github.com/therealmcsparrow/mcharbor/core/db"
	"github.com/therealmcsparrow/mcharbor/core/plugin"
	"github.com/therealmcsparrow/mcharbor/core/plugin/specs/incident"
)

type fakeChat struct{}

func (fakeChat) Test(context.Context, map[string]string) error { return nil }
func (fakeChat) Send(context.Context, map[string]string, plugin.NotificationMessage) error {
	return nil
}

type fakeITSM struct {
	fakeChat
	opened int
}

func (f *fakeITSM) Open(context.Context, map[string]string, incident.Event) (incident.Ref, error) {
	f.opened++
	return incident.Ref{ExternalID: "x"}, nil
}
func (f *fakeITSM) Update(context.Context, map[string]string, incident.Ref, incident.Event) error {
	return nil
}
func (f *fakeITSM) Resolve(context.Context, map[string]string, incident.Ref, incident.Event) error {
	return nil
}

type fakeITSMWebhook struct{ fakeITSM }

func (fakeITSMWebhook) VerifyWebhook(map[string]string, http.Header, []byte) error { return nil }
func (fakeITSMWebhook) ParseWebhook(context.Context, map[string]string, http.Header, []byte) ([]incident.RemoteStatus, error) {
	return nil, nil
}

type fakeITSMFull struct{ fakeITSMWebhook }

func (fakeITSMFull) Poll(context.Context, map[string]string, []incident.Ref) ([]incident.RemoteStatus, error) {
	return nil, nil
}

func newTestAuditor(t *testing.T) *adapterAuditor {
	t.Helper()
	database, err := sql.Open("sqlite", "file:"+filepath.Join(t.TempDir(), "a.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := database.Close(); err != nil {
			t.Logf("closing db: %v", err)
		}
	})
	if err := coredb.Migrate(database, coredb.DriverSQLite); err != nil {
		t.Fatal(err)
	}
	return newAdapterAuditor(audit.NewLogger(database, "s"), slog.New(slog.NewTextHandler(io.Discard, nil)))
}

// TestAuditedClientForwardsLifecycle guards the trap in the foundation plan:
// the audit wrapper must keep exactly the optional interfaces the plugin has,
// or every ITSM plugin silently degrades to one-way Send.
func TestAuditedClientForwardsLifecycle(t *testing.T) {
	aud := newTestAuditor(t)
	inner := &fakeITSM{}
	wrapped := wrapNotificationClient(aud, "comm-x", "x", inner)
	lc, ok := wrapped.(incident.LifecycleClient)
	if !ok {
		t.Fatal("wrapped ITSM client lost LifecycleClient")
	}
	if _, err := lc.Open(context.Background(), nil, incident.Event{}); err != nil || inner.opened != 1 {
		t.Fatalf("open did not reach the plugin: %v %d", err, inner.opened)
	}
	if _, ok := wrapped.(incident.WebhookParser); ok {
		t.Fatal("a client without a webhook parser gained one")
	}
	if _, ok := wrapNotificationClient(aud, "comm-y", "y", fakeChat{}).(incident.LifecycleClient); ok {
		t.Fatal("chat client must not gain LifecycleClient")
	}
	full := wrapNotificationClient(aud, "comm-z", "z", &fakeITSMFull{})
	_, hasWebhook := full.(incident.WebhookParser)
	_, hasPoll := full.(incident.Poller)
	if !hasWebhook || !hasPoll {
		t.Fatalf("full client lost an interface: webhook=%v poll=%v", hasWebhook, hasPoll)
	}
	webhookOnly := wrapNotificationClient(aud, "comm-w", "w", &fakeITSMWebhook{})
	if _, ok := webhookOnly.(incident.Poller); ok {
		t.Fatal("a webhook-only client gained a poller")
	}
}
