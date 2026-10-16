// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package deviceauth

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	_ "modernc.org/sqlite"

	"github.com/therealmcsparrow/mcharbor/core/audit"
	"github.com/therealmcsparrow/mcharbor/core/auth"
	coredb "github.com/therealmcsparrow/mcharbor/core/db"
	"github.com/therealmcsparrow/mcharbor/core/rbac"
	"github.com/therealmcsparrow/mcharbor/core/router"
	apikeys "github.com/therealmcsparrow/mcharbor/modules/api_keys"
)

type fakeMinter struct {
	calls  int
	userID string
	input  apikeys.CreateAPIKeyInput
}

func (f *fakeMinter) Create(_ context.Context, userID string, in *apikeys.CreateAPIKeyInput) (*apikeys.CreateAPIKeyResult, error) {
	f.calls++
	f.userID = userID
	f.input = *in
	return &apikeys.CreateAPIKeyResult{APIKey: apikeys.APIKey{ID: "key1", UserID: userID, Name: in.Name}, Key: "mch_plaintext"}, nil
}

func openDB(t *testing.T) *sql.DB {
	t.Helper()
	database, err := sql.Open("sqlite", "file:"+filepath.Join(t.TempDir(), "device.db")+"?_pragma=foreign_keys(1)")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := database.Close(); err != nil {
			t.Errorf("closing test database: %v", err)
		}
	})
	if err := coredb.Migrate(database, coredb.DriverSQLite); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	for _, stmt := range []string{
		`INSERT INTO users (id, username, password_hash) VALUES ('alice', 'alice', 'x')`,
		`INSERT INTO roles (id, name, permissions) VALUES ('r-view', 'r-view', '["containers.view","api_keys.manage"]')`,
		`INSERT INTO user_roles (id, user_id, role_id) VALUES ('ur1', 'alice', 'r-view')`,
	} {
		if _, err := database.Exec(stmt); err != nil {
			t.Fatalf("seed: %v", err)
		}
	}
	return database
}

type clock struct{ t time.Time }

func (c *clock) now() time.Time          { return c.t }
func (c *clock) advance(d time.Duration) { c.t = c.t.Add(d) }
func newClock() *clock                   { return &clock{t: time.Date(2026, 9, 26, 12, 0, 0, 0, time.UTC)} }

func TestTheKeyIsIssuedOnceAfterApprovalAndOnlyAHashIsStored(t *testing.T) {
	database := openDB(t)
	minter := &fakeMinter{}
	c := newClock()
	svc := NewService(database, minter)
	svc.now = c.now
	ctx := context.Background()

	start, err := svc.Start(ctx, StartInput{Host: "laptop\x1b[31m", OSUser: "carlo"})
	if err != nil {
		t.Fatal(err)
	}
	var stored, host string
	if err := database.QueryRow(`SELECT device_code_hash, requested_host FROM device_codes`).Scan(&stored, &host); err != nil {
		t.Fatal(err)
	}
	if stored == start.DeviceCode || strings.Contains(stored, start.DeviceCode) {
		t.Fatal("the device code is stored in plaintext")
	}
	if strings.ContainsRune(host, '\x1b') {
		t.Fatalf("a control character reached the approver's screen: %q", host)
	}

	if _, err := svc.Poll(ctx, start.DeviceCode); !errors.Is(err, ErrPending) {
		t.Fatalf("first poll: %v", err)
	}
	c.advance(2 * time.Second)
	if _, err := svc.Poll(ctx, start.DeviceCode); !errors.Is(err, ErrSlowDown) {
		t.Fatalf("a poll inside the interval was not slowed down: %v", err)
	}
	c.advance(pollInterval)

	scopes := []string{"containers.view"}
	if err := svc.Approve(ctx, strings.ToLower(strings.ReplaceAll(start.UserCode, "-", "")), "alice",
		ApproveInput{Name: "laptop", Scopes: scopes}); err != nil {
		t.Fatalf("approve with a typed, undashed, lowercase code: %v", err)
	}
	key, err := svc.Poll(ctx, start.DeviceCode)
	if err != nil || key.Key != "mch_plaintext" {
		t.Fatalf("poll after approval: %+v, %v", key, err)
	}
	if minter.userID != "alice" || len(minter.input.Scopes) != 1 || minter.input.Name != "laptop" {
		t.Fatalf("the key was minted for the wrong user or grants: %+v", minter)
	}
	c.advance(pollInterval)
	if _, err := svc.Poll(ctx, start.DeviceCode); !errors.Is(err, ErrInvalid) {
		t.Fatalf("a second poll after issue was not refused: %v", err)
	}
	if minter.calls != 1 {
		t.Fatalf("the key was minted %d times", minter.calls)
	}
}

func TestDeniedAndExpiredRequestsNeverIssueAKey(t *testing.T) {
	database := openDB(t)
	minter := &fakeMinter{}
	c := newClock()
	svc := NewService(database, minter)
	svc.now = c.now
	ctx := context.Background()

	denied, err := svc.Start(ctx, StartInput{})
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.Deny(ctx, denied.UserCode, "alice"); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Poll(ctx, denied.DeviceCode); !errors.Is(err, ErrDenied) {
		t.Fatalf("denied: %v", err)
	}
	if err := svc.Approve(ctx, denied.UserCode, "alice", ApproveInput{Name: "x", Scopes: []string{"a"}}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("a denied request was approvable afterwards: %v", err)
	}

	late, err := svc.Start(ctx, StartInput{})
	if err != nil {
		t.Fatal(err)
	}
	c.advance(codeTTL + time.Second)
	if _, err := svc.Poll(ctx, late.DeviceCode); !errors.Is(err, ErrExpired) {
		t.Fatalf("expired: %v", err)
	}
	if _, err := svc.Lookup(ctx, late.UserCode); !errors.Is(err, ErrNotFound) {
		t.Fatalf("an expired request is still shown to approvers: %v", err)
	}
	if err := svc.Approve(ctx, late.UserCode, "alice", ApproveInput{Name: "x", Scopes: []string{"a"}}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("an expired request was approvable: %v", err)
	}
	if minter.calls != 0 {
		t.Fatalf("a key was minted %d times", minter.calls)
	}
}

func TestUserCodesUseOnlyTheUnambiguousAlphabet(t *testing.T) {
	for i := 0; i < 500; i++ {
		code, err := randomUserCode()
		if err != nil {
			t.Fatal(err)
		}
		if len(code) != 9 || code[4] != '-' {
			t.Fatalf("code %q is not XXXX-XXXX", code)
		}
		for _, r := range strings.ReplaceAll(code, "-", "") {
			if !strings.ContainsRune(userCodeAlphabet, r) {
				t.Fatalf("code %q contains %q", code, r)
			}
		}
	}
}

func TestTheHTTPLayerSpeaksTheDeviceFlowAndRefusesOverbroadKeys(t *testing.T) {
	database := openDB(t)
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	rbacSvc := rbac.NewService(database)
	keys := apikeys.NewService(database, rbacSvc)
	app := &router.AppDeps{DB: database, Logger: logger, AuditLog: audit.NewLogger(database, "")}
	h := &Handler{app: app, svc: NewService(database, keys), grants: keys}

	call := func(handler http.HandlerFunc, method, path, body string, user string, params map[string]string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(method, path, bytes.NewBufferString(body))
		rctx := chi.NewRouteContext()
		for k, v := range params {
			rctx.URLParams.Add(k, v)
		}
		ctx := context.WithValue(req.Context(), chi.RouteCtxKey, rctx)
		if user != "" {
			ctx = context.WithValue(ctx, auth.UserContextKey, &auth.User{ID: user, Username: user})
		}
		rec := httptest.NewRecorder()
		handler(rec, req.WithContext(ctx))
		return rec
	}
	pollErr := func(rec *httptest.ResponseRecorder) string {
		var env struct {
			Data struct {
				Error string `json:"error"`
			} `json:"data"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &env); err != nil {
			t.Fatalf("body %s: %v", rec.Body.String(), err)
		}
		return env.Data.Error
	}

	rec := call(h.HandleStart, http.MethodPost, "/api/device/code", `{"host":"laptop"}`, "", nil)
	var started struct {
		Data StartResult `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &started); err != nil || started.Data.DeviceCode == "" {
		t.Fatalf("start: %d %s", rec.Code, rec.Body.String())
	}
	if rec.Header().Get("Cache-Control") == "" {
		t.Fatal("the device code response may be cached")
	}
	token := `{"deviceCode":"` + started.Data.DeviceCode + `"}`

	if rec := call(h.HandleToken, http.MethodPost, "/api/device/token", token, "", nil); rec.Code != http.StatusBadRequest || pollErr(rec) != "authorization_pending" {
		t.Fatalf("pending: %d %s", rec.Code, rec.Body.String())
	}
	if rec := call(h.HandleToken, http.MethodPost, "/api/device/token", token, "", nil); rec.Code != http.StatusTooManyRequests || pollErr(rec) != "slow_down" {
		t.Fatalf("slow_down: %d %s", rec.Code, rec.Body.String())
	}

	params := map[string]string{"userCode": started.Data.UserCode}
	over := `{"name":"laptop","scopes":["containers.manage"]}`
	if rec := call(h.HandleApprove, http.MethodPost, "/approve", over, "alice", params); rec.Code != http.StatusBadRequest {
		t.Fatalf("an approval granting more than the approver holds returned %d", rec.Code)
	}
	if rec := call(h.HandleDeny, http.MethodPost, "/deny", "", "alice", params); rec.Code != http.StatusOK {
		t.Fatalf("deny: %d", rec.Code)
	}
	if rec := call(h.HandleToken, http.MethodPost, "/api/device/token", token, "", nil); rec.Code != http.StatusForbidden || pollErr(rec) != "access_denied" {
		t.Fatalf("access_denied: %d %s", rec.Code, rec.Body.String())
	}
	if rec := call(h.HandleToken, http.MethodPost, "/api/device/token", `{"deviceCode":"forged"}`, "", nil); pollErr(rec) != "invalid_grant" {
		t.Fatalf("invalid_grant: %d %s", rec.Code, rec.Body.String())
	}
}
