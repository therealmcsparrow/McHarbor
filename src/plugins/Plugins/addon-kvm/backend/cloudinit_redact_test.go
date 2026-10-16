// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"

	"github.com/therealmcsparrow/mcharbor/core/auth"
	"github.com/therealmcsparrow/mcharbor/core/envredact"
)

const testUserData = `#cloud-config
write_files:
  - path: /etc/app.env
    content: |
      API_TOKEN=tok-abcdef123
      LOG_LEVEL=info
chpasswd:
  list: |
    root:hunter2hunter2
runcmd:
  - curl -o /tmp/x https://deploy:s3cr3t-pass@example.com/app
  - echo ready
db_password: plain-db-password
hostname_note: web-01
`

// userDataSecrets are the credentials testUserData carries; none may reach a viewer.
var userDataSecrets = []string{"tok-abcdef123", "hunter2hunter2", "s3cr3t-pass", "plain-db-password"}

func newCloudInitHandler(t *testing.T) (*Handler, chi.Router) {
	t.Helper()
	svc, _, db := newIntentTestService(t)
	insertIntentTestVM(t, db, "vm1", StateStopped)
	if _, err := svc.writeCloudInit(context.Background(), "vm1", CloudInitRequest{
		Hostname: "web-01", UserDataExtra: testUserData,
	}); err != nil {
		t.Fatal(err)
	}
	h := NewHandler(svc, nil, nil, quietLogger{})
	r := chi.NewRouter()
	r.Get("/api/vms/{id}", h.HandleGet)
	r.Get("/api/vms/{id}/cloud-init", h.HandleCloudInit)
	return h, r
}

func getAs(t *testing.T, r chi.Router, target string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, target, nil)
	req = req.WithContext(context.WithValue(req.Context(), auth.UserContextKey, &auth.User{ID: "u1", Username: "u1"}))
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	return rec
}

// TestVMDetailWithholdsUserDataCredentials: vm.view returned the operator's cloud-init
// user-data verbatim, including every password and token typed into it.
func TestVMDetailWithholdsUserDataCredentials(t *testing.T) {
	_, r := newCloudInitHandler(t)
	rec := getAs(t, r, "/api/vms/vm1")
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /api/vms/vm1: HTTP %d, body %s", rec.Code, rec.Body.String())
	}
	var body struct {
		Data struct {
			CloudInit struct {
				Hostname      string `json:"hostname"`
				UserDataExtra string `json:"userDataExtra"`
			} `json:"cloudInit"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	userData := body.Data.CloudInit.UserDataExtra
	for _, secret := range userDataSecrets {
		if strings.Contains(rec.Body.String(), secret) {
			t.Errorf("the VM detail carried %q:\n%s", secret, userData)
		}
	}
	for _, kept := range []string{"LOG_LEVEL=info", "echo ready", "web-01", envredact.Marker} {
		if !strings.Contains(userData, kept) {
			t.Errorf("masked user-data lost %q:\n%s", kept, userData)
		}
	}
}

// TestCloudInitRevealReturnsStoredUserData is the permitted half; the manifest (below) is what
// puts it behind vm.manage and the audit trail.
func TestCloudInitRevealReturnsStoredUserData(t *testing.T) {
	_, r := newCloudInitHandler(t)
	rec := getAs(t, r, "/api/vms/vm1/cloud-init")
	if rec.Code != http.StatusOK {
		t.Fatalf("GET cloud-init: HTTP %d, body %s", rec.Code, rec.Body.String())
	}
	if got := rec.Header().Get("Cache-Control"); !strings.Contains(got, "no-store") {
		t.Errorf("Cache-Control = %q, want no-store", got)
	}
	var body struct {
		Data CloudInit `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Data.UserDataExtra != testUserData {
		t.Fatalf("revealed user-data differs from what was stored:\n%s", body.Data.UserDataExtra)
	}
}

// TestCreateRefusesMaskedUserData: pasting the masked detail back into a create would boot a
// guest with bullets for a password, so it is refused before anything is written.
func TestCreateRefusesMaskedUserData(t *testing.T) {
	req := CreateRequest{
		EnvironmentID: "env1",
		CloudInit:     &CloudInitRequest{UserDataExtra: "db_password: " + envredact.Marker + "\n"},
	}
	err := validateCreate(req)
	var field *validationError
	if !errors.As(err, &field) || field.code != codeUserDataMasked {
		t.Fatalf("validateCreate = %v, want %s", err, codeUserDataMasked)
	}
}

// TestUserDataThatIsNotYAMLIsStillMasked covers the line-scan fallback.
func TestUserDataThatIsNotYAMLIsStillMasked(t *testing.T) {
	got := maskUserData("#!/bin/sh\nexport AWS_SECRET_ACCESS_KEY=abc123secret\n: [ unbalanced\n")
	if strings.Contains(got, "abc123secret") || !strings.Contains(got, "AWS_SECRET_ACCESS_KEY="+envredact.Marker) {
		t.Fatalf("shell user-data not masked: %q", got)
	}
}

// TestCloudInitRouteIsManageGatedAndAudited reads the manifest: the reveal route must take
// vm.manage and carry an audit declaration, or the host serves it to viewers unrecorded.
func TestCloudInitRouteIsManageGatedAndAudited(t *testing.T) {
	raw, err := os.ReadFile("../plugin.yaml")
	if err != nil {
		t.Fatal(err)
	}
	text := strings.ReplaceAll(string(raw), "\r\n", "\n")
	at := strings.Index(text, `path: "/api/vms/{id}/cloud-init"`)
	if at < 0 {
		t.Fatal("the manifest declares no /api/vms/{id}/cloud-init route")
	}
	if !strings.HasSuffix(strings.TrimRight(text[:at], " "), "- method: GET\n") {
		t.Fatalf("the cloud-init route is not a GET:\n%s", text[max(0, at-40):at])
	}
	block := text[at:]
	if end := strings.Index(block, "- method:"); end >= 0 {
		block = block[:end]
	}
	for _, want := range []string{
		`requires_permission: "vm.manage"`, "audit_required: true",
		`audit_action: "plugin.kvm.vm.cloud_init_revealed"`, `audit_entity_type: "vm"`,
		`audit_severity: "warning"`,
	} {
		if !strings.Contains(block, want) {
			t.Errorf("cloud-init route lacks %s:\n%s", want, block)
		}
	}
}
