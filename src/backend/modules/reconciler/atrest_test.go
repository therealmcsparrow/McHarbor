// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package reconciler

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"

	"github.com/therealmcsparrow/mcharbor/core/encryption"
	"github.com/therealmcsparrow/mcharbor/core/envredact"
	"github.com/therealmcsparrow/mcharbor/core/router"
)

func rawDesiredConfig(t *testing.T, loop *Loop, id string) string {
	t.Helper()
	var raw string
	if err := loop.service.db.QueryRow(`SELECT COALESCE(config, '') FROM desired_states WHERE id = ?`, id).Scan(&raw); err != nil {
		t.Fatal(err)
	}
	return raw
}

// TestDesiredStateEnvIsEncryptedAndMasked guards a container's pinned credentials: the
// config is ciphertext at rest, the API withholds credential-looking env values, and an
// edit that sends the mask back keeps the stored value instead of pinning the mask.
func TestDesiredStateEnvIsEncryptedAndMasked(t *testing.T) {
	loop, database, _, _ := newLoopFixture(t)
	r, _ := withResponder(t, loop, database)
	h := NewHandler(&router.AppDeps{DB: database, Logger: slog.New(slog.NewTextHandler(io.Discard, nil))}, r)
	mux := chi.NewRouter()
	mux.Post("/reconciler", h.HandleCreate)
	mux.Get("/reconciler", h.HandleList)
	mux.Get("/reconciler/{id}", h.HandleGet)
	mux.Put("/reconciler/{id}", h.HandleUpdate)
	call := func(method, path, body string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(method, path, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		return w
	}

	created := call(http.MethodPost, "/reconciler",
		`{"name":"db","containerName":"db","imageRef":"postgres:16","config":"{\"env\":{\"DB_PASSWORD\":\"hunter2-secret\",\"MODE\":\"blue\"}}"}`)
	if created.Code != http.StatusCreated {
		t.Fatalf("create: %d %s", created.Code, created.Body.String())
	}
	if strings.Contains(created.Body.String(), "hunter2-secret") {
		t.Fatalf("create response carries the credential: %s", created.Body.String())
	}
	var envelope struct {
		Data DesiredState `json:"data"`
	}
	if err := json.Unmarshal(created.Body.Bytes(), &envelope); err != nil {
		t.Fatal(err)
	}
	id := envelope.Data.ID

	raw := rawDesiredConfig(t, loop, id)
	if !encryption.IsEncrypted(raw) || strings.Contains(raw, "hunter2-secret") {
		t.Fatalf("config stored as %q, want ciphertext", raw)
	}

	for _, path := range []string{"/reconciler/" + id, "/reconciler"} {
		w := call(http.MethodGet, path, "")
		if w.Code != http.StatusOK {
			t.Fatalf("GET %s: %d", path, w.Code)
		}
		body := w.Body.String()
		if strings.Contains(body, "hunter2-secret") || strings.Contains(body, "enc:v1:") {
			t.Fatalf("GET %s leaks the credential: %s", path, body)
		}
		if !strings.Contains(body, "blue") {
			t.Fatalf("GET %s withheld an ordinary value: %s", path, body)
		}
	}

	masked := Redacted(DesiredState{Config: `{"env":{"DB_PASSWORD":"x-secret-1","MODE":"green"}}`}).Config
	if !strings.Contains(masked, envredact.Marker) {
		t.Fatalf("Redacted did not mask: %s", masked)
	}
	body, err := json.Marshal(map[string]string{"config": strings.ReplaceAll(masked, "x-secret-1", "")})
	if err != nil {
		t.Fatal(err)
	}
	if w := call(http.MethodPut, "/reconciler/"+id, string(body)); w.Code != http.StatusOK {
		t.Fatalf("update with the mask: %d %s", w.Code, w.Body.String())
	}
	ds, err := loop.service.ByID(t.Context(), id)
	if err != nil {
		t.Fatal(err)
	}
	cfg, err := ParseConfig(ds.Config, false)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Env["DB_PASSWORD"] != "hunter2-secret" || cfg.Env["MODE"] != "green" {
		t.Fatalf("after a masked round-trip env = %v, want the stored password and the new mode", cfg.Env)
	}
	if !encryption.IsEncrypted(rawDesiredConfig(t, loop, id)) {
		t.Fatal("an update stored the config in plaintext")
	}
}

// TestDesiredSpecMaskRoundTrip: a plugin kind's credential-looking pinned value is
// withheld in a response and restored from storage when the mask comes back.
func TestDesiredSpecMaskRoundTrip(t *testing.T) {
	out := Redacted(DesiredState{Spec: map[string]string{"replicas": "3", "api_token": "tok-123456789"}})
	if out.Spec["api_token"] != envredact.Marker || out.Spec["replicas"] != "3" {
		t.Fatalf("redacted spec = %v", out.Spec)
	}
	restored := restoreSpec(map[string]string{"replicas": "4", "api_token": envredact.Marker, "new_token": envredact.Marker},
		map[string]string{"replicas": "3", "api_token": "tok-123456789"})
	if restored["api_token"] != "tok-123456789" || restored["replicas"] != "4" {
		t.Fatalf("restored spec = %v", restored)
	}
	if _, pinned := restored["new_token"]; pinned {
		t.Fatal("a mask with nothing stored behind it was pinned as a desired value")
	}
}

// TestDesiredStateBackfill seals rows written before encryption, leaves sealed and
// empty ones alone, and is a no-op on the second run.
func TestDesiredStateBackfill(t *testing.T) {
	loop, database, _, _ := newLoopFixture(t)
	svc := loop.service
	sealed, err := svc.seal(`{"env":{"A":"b"}}`)
	if err != nil {
		t.Fatal(err)
	}
	for _, row := range [][3]string{
		{"bf-plain", `{"env":{"DB_PASSWORD":"legacy-pass"}}`, `{"token":"legacy-token"}`},
		{"bf-sealed", sealed, ""},
		{"bf-empty", "", ""},
	} {
		if _, err := database.Exec(`INSERT INTO desired_states (id, name, container_name, image_ref, desired_status, config, spec, created_at, updated_at)
			VALUES (?, ?, 'c', 'i:1', 'running', ?, ?, '2026-01-01T00:00:00Z', '2026-01-01T00:00:00Z')`, row[0], row[0], row[1], row[2]); err != nil {
			t.Fatal(err)
		}
	}
	before := map[string]string{}
	for _, id := range []string{"bf-sealed", "bf-empty"} {
		before[id] = rawDesiredConfig(t, loop, id)
	}

	first, err := svc.BackfillEncryption(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if first < 1 {
		t.Fatalf("first backfill encrypted %d rows, want the plaintext ones", first)
	}
	var cfg, spec string
	if err := database.QueryRow(`SELECT config, spec FROM desired_states WHERE id = 'bf-plain'`).Scan(&cfg, &spec); err != nil {
		t.Fatal(err)
	}
	if !encryption.IsEncrypted(cfg) || !encryption.IsEncrypted(spec) {
		t.Fatalf("legacy row still plaintext: config=%q spec=%q", cfg, spec)
	}
	ds, err := svc.ByID(t.Context(), "bf-plain")
	if err != nil || ds == nil || !strings.Contains(ds.Config, "legacy-pass") || ds.Spec["token"] != "legacy-token" {
		t.Fatalf("sealed legacy row reads back as %+v (%v)", ds, err)
	}
	for id, was := range before {
		if got := rawDesiredConfig(t, loop, id); got != was {
			t.Fatalf("%s changed from %q to %q", id, was, got)
		}
	}
	again, err := svc.BackfillEncryption(t.Context())
	if err != nil || again != 0 {
		t.Fatalf("second backfill = (%d, %v), want a no-op", again, err)
	}
}

// TestDesiredStateRefusedWithoutEncryption keeps a config from being stored in
// plaintext by a service built without encryption.
func TestDesiredStateRefusedWithoutEncryption(t *testing.T) {
	loop, _, _, _ := newLoopFixture(t)
	svc := NewService(loop.service.db, nil)
	if _, err := svc.Create(t.Context(), CreateDesiredStateInput{Name: "x", Kind: KindContainer, ContainerName: "x", ImageRef: "x:1",
		Config: `{"env":{"A":"b"}}`}); err == nil || !strings.Contains(err.Error(), ErrReconcilerEncryptionUnavailable.Error()) {
		t.Fatalf("create without encryption = %v, want ErrReconcilerEncryptionUnavailable", err)
	}
}
