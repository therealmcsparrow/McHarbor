// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package hostservices

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/go-chi/chi/v5"
	_ "modernc.org/sqlite"

	coreagent "github.com/therealmcsparrow/mcharbor/core/agent"
	"github.com/therealmcsparrow/mcharbor/core/apispec"
	"github.com/therealmcsparrow/mcharbor/core/audit"
	"github.com/therealmcsparrow/mcharbor/core/auth"
	coredb "github.com/therealmcsparrow/mcharbor/core/db"
	"github.com/therealmcsparrow/mcharbor/core/i18n"
	"github.com/therealmcsparrow/mcharbor/core/rbac"
	"github.com/therealmcsparrow/mcharbor/core/router"
)

func TestValidateUnit(t *testing.T) {
	cases := []struct {
		name string
		ok   bool
	}{
		{"nginx.service", true},
		{"getty@tty1.service", true},
		{"a_b-c:d.service", true},
		{"", false},
		{"nginx", false},
		{"nginx.timer", false},
		{"../etc/passwd.service", false},
		{"..service", false},
		{"a/b.service", false},
		{`a\b.service`, false},
		{"my unit.service", false},
		{"-H.service", false},
		{".hidden.service", false},
		{"nginx.service\n", false},
		{strings.Repeat("a", 260) + ".service", false},
	}
	for _, c := range cases {
		if err := ValidateUnit(c.name); (err == nil) != c.ok {
			t.Errorf("ValidateUnit(%q) = %v, want ok=%v", c.name, err, c.ok)
		}
	}
}

func TestGuardTable(t *testing.T) {
	protected := map[string]string{
		"mcharbor-agent.service": "agent", "ssh.service": "ssh", "sshd.service": "ssh",
		"systemd-journald.service": "systemd", "dbus.service": "dbus", "dbus-broker.service": "dbus",
		"docker.service": "runtime", "containerd.service": "runtime",
	}
	for unit, reason := range protected {
		if got := Protection(unit); got != reason {
			t.Errorf("Protection(%s) = %q, want %q", unit, got, reason)
		}
		for _, action := range []string{"stop", "disable", "mask"} {
			if !ActionRefused(unit, action) {
				t.Errorf("%s %s was allowed", action, unit)
			}
		}
		for _, action := range []string{"start", "restart", "enable"} {
			if ActionRefused(unit, action) {
				t.Errorf("%s %s was refused", action, unit)
			}
		}
	}
	for _, unit := range []string{"nginx.service", "sshguard.service", "dockerd-rootless.service"} {
		if ActionRefused(unit, "stop") {
			t.Errorf("stop %s was refused", unit)
		}
	}
}

// fakeLink is a scripted agent that records every request it was sent, so a
// test can prove a refusal never reached the host.
type fakeLink struct {
	mu        sync.Mutex
	supported bool
	capable   *bool
	requests  []coreagent.HostServicePayload
	reply     func(coreagent.HostServicePayload) *coreagent.HostServicePayload
}

func (f *fakeLink) HostServicesSupported() bool           { return f.supported }
func (f *fakeLink) HostServicesVersion() (string, string) { return "2026.09.01", "202609271515" }
func (f *fakeLink) HostServicesCapable() (bool, bool) {
	if f.capable == nil {
		return false, false
	}
	return *f.capable, true
}
func (f *fakeLink) HostService(_ context.Context, p coreagent.HostServicePayload) (*coreagent.HostServicePayload, error) {
	f.mu.Lock()
	f.requests = append(f.requests, p)
	f.mu.Unlock()
	if f.reply != nil {
		return f.reply(p), nil
	}
	return &coreagent.HostServicePayload{Op: p.Op, Success: true}, nil
}

func (f *fakeLink) sent() []coreagent.HostServicePayload {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]coreagent.HostServicePayload(nil), f.requests...)
}

func openDB(t *testing.T) *sql.DB {
	t.Helper()
	database, err := sql.Open("sqlite", "file:"+filepath.Join(t.TempDir(), "hostservices.db")+"?_pragma=foreign_keys(1)")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := database.Close(); err != nil {
			t.Log(err)
		}
	})
	if err := coredb.Migrate(database, coredb.DriverSQLite); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	for _, row := range [][3]string{{"env-native", "agent", "native"}, {"env-container", "agent", "container"}, {"env-socket", "socket", ""}} {
		if _, err := database.Exec(`INSERT INTO environments (id, name, connection_type, agent_install_kind) VALUES (?, ?, ?, NULLIF(?, ''))`, row[0], row[0], row[1], row[2]); err != nil {
			t.Fatalf("seed %s: %v", row[0], err)
		}
	}
	return database
}

func newTestService(database *sql.DB, link *fakeLink) *Service {
	return &Service{db: database, resolve: func(envID string) agentLink {
		if link == nil || envID != "env-native" && envID != "env-container" {
			return nil
		}
		return link
	}}
}

func TestAvailabilityReasons(t *testing.T) {
	database := openDB(t)
	no := false
	cases := []struct {
		env    string
		link   *fakeLink
		reason string
	}{
		{"env-socket", &fakeLink{supported: true}, "not_agent"},
		{"env-container", &fakeLink{supported: true}, "not_native"},
		{"env-native", nil, "offline"},
		{"env-native", &fakeLink{supported: false}, "agent_outdated"},
		{"env-native", &fakeLink{supported: true, capable: &no}, "no_systemd"},
		{"env-native", &fakeLink{supported: true}, ""},
	}
	for _, c := range cases {
		avail, _, err := newTestService(database, c.link).Availability(context.Background(), c.env)
		if err != nil {
			t.Fatal(err)
		}
		if avail.Reason != c.reason || avail.Supported != (c.reason == "") {
			t.Errorf("%s: availability = %+v, want reason %q", c.env, avail, c.reason)
		}
		if avail.MinBuild != coreagent.HostServicesMinBuild {
			t.Errorf("the refusal must name the build it needs, got %q", avail.MinBuild)
		}
	}
	if _, _, err := newTestService(database, nil).Availability(context.Background(), "nope"); !errors.Is(err, ErrEnvironmentNotFound) {
		t.Errorf("unknown env: %v", err)
	}
}

func TestProtectedStopAndDisableNeverReachTheAgent(t *testing.T) {
	link := &fakeLink{supported: true}
	svc := newTestService(openDB(t), link)
	for _, unit := range []string{"sshd.service", "mcharbor-agent.service", "docker.service", "systemd-logind.service", "dbus.service"} {
		for _, action := range []string{"stop", "disable"} {
			_, err := svc.Action(context.Background(), EnvTarget("env-native"), unit, action)
			var refusal *Refusal
			if !errors.As(err, &refusal) || refusal.Code != i18n.ErrHostServicesProtected || refusal.Status != http.StatusConflict {
				t.Errorf("%s %s: err = %v, want 409 protected", action, unit, err)
			}
		}
	}
	if sent := link.sent(); len(sent) != 0 {
		t.Fatalf("refused actions reached the agent: %+v", sent)
	}
}

func TestAgentRestartIsAllowedAndCarriesTheWarning(t *testing.T) {
	link := &fakeLink{supported: true, reply: func(p coreagent.HostServicePayload) *coreagent.HostServicePayload {
		return &coreagent.HostServicePayload{Op: p.Op, Success: true, Warning: coreagent.HostServiceWarningAgentRestart}
	}}
	result, err := newTestService(openDB(t), link).Action(context.Background(), EnvTarget("env-native"), "mcharbor-agent.service", "restart")
	if err != nil || result.Warning != coreagent.HostServiceWarningAgentRestart {
		t.Fatalf("result = %+v, err = %v", result, err)
	}
}

func TestInvalidUnitNeverReachesTheAgent(t *testing.T) {
	link := &fakeLink{supported: true}
	svc := newTestService(openDB(t), link)
	ctx := context.Background()
	for _, unit := range []string{"../../etc/shadow.service", "a b.service", "nginx", "-x.service"} {
		if _, err := svc.Action(ctx, EnvTarget("env-native"), unit, "start"); err == nil {
			t.Errorf("action on %q accepted", unit)
		}
		if _, err := svc.Override(ctx, EnvTarget("env-native"), unit); err == nil {
			t.Errorf("override read of %q accepted", unit)
		}
		if _, err := svc.SaveOverride(ctx, EnvTarget("env-native"), unit, OverrideRequest{Content: "[Service]\n"}); err == nil {
			t.Errorf("override write of %q accepted", unit)
		}
		if err := svc.RevertOverride(ctx, EnvTarget("env-native"), unit); err == nil {
			t.Errorf("revert of %q accepted", unit)
		}
	}
	if sent := link.sent(); len(sent) != 0 {
		t.Fatalf("invalid units reached the agent: %+v", sent)
	}
}

func TestAgentRefusalCarriesSystemdsReason(t *testing.T) {
	link := &fakeLink{supported: true, reply: func(p coreagent.HostServicePayload) *coreagent.HostServicePayload {
		return &coreagent.HostServicePayload{Op: p.Op, ErrorCode: "failed", Error: "Job for nginx.service failed because the control process exited with error code."}
	}}
	_, err := newTestService(openDB(t), link).Action(context.Background(), EnvTarget("env-native"), "nginx.service", "start")
	var refusal *Refusal
	if !errors.As(err, &refusal) || refusal.Code != i18n.ErrHostServicesActionFailed || !strings.Contains(refusal.Detail, "control process exited") {
		t.Fatalf("err = %v", err)
	}
}

func TestListMarksProtectedUnits(t *testing.T) {
	link := &fakeLink{supported: true, reply: func(p coreagent.HostServicePayload) *coreagent.HostServicePayload {
		return &coreagent.HostServicePayload{Op: p.Op, Success: true, Units: []coreagent.HostServiceUnit{
			{Name: "nginx.service", ActiveState: "failed", FailureReason: "result: exit-code"},
			{Name: "sshd.service", ActiveState: "active"},
			{Name: "mcharbor-agent.service", ActiveState: "active"},
			{Name: "../evil.service"},
		}}
	}}
	list, err := newTestService(openDB(t), link).List(context.Background(), EnvTarget("env-native"))
	if err != nil {
		t.Fatal(err)
	}
	if len(list.Units) != 3 {
		t.Fatalf("units = %+v; an invalid name from the agent must be dropped", list.Units)
	}
	byName := map[string]Unit{}
	for _, u := range list.Units {
		byName[u.Name] = u
	}
	if byName["sshd.service"].Protected != "ssh" || !byName["mcharbor-agent.service"].IsAgent || byName["nginx.service"].Protected != "" {
		t.Fatalf("units = %+v", list.Units)
	}
}

func TestListOnAnOutdatedAgentAnswersWithTheReason(t *testing.T) {
	link := &fakeLink{supported: false}
	list, err := newTestService(openDB(t), link).List(context.Background(), EnvTarget("env-native"))
	if err != nil || list.Availability.Reason != "agent_outdated" || len(list.Units) != 0 || len(link.sent()) != 0 {
		t.Fatalf("list = %+v, err = %v", list, err)
	}
}

func mountForTest(t *testing.T, database *sql.DB, link *fakeLink) (http.Handler, *apispec.Registry) {
	t.Helper()
	app := &router.AppDeps{DB: database, RBACService: rbac.NewService(database), AuditLog: audit.NewLogger(database, "test-secret"), Logger: slog.Default()}
	h := &Handler{app: app, svc: newTestService(database, link)}
	mux := chi.NewRouter()
	reg := apispec.NewRegistry()
	MountRoutes(apispec.NewRouter(mux, reg, "/api", apispec.GroupProtected), app, h)
	return mux, reg
}

func as(userID string, req *http.Request) *http.Request {
	return req.WithContext(context.WithValue(req.Context(), auth.UserContextKey, &auth.User{ID: userID, Username: userID}))
}

func TestRoutesAreGatedOnTheHostServicesKeys(t *testing.T) {
	_, reg := mountForTest(t, openDB(t), &fakeLink{supported: true})
	want := map[string]string{
		"GET /api/host-services":                         string(rbac.PermHostServicesView),
		"GET /api/host-services/{unit}/override":         string(rbac.PermHostServicesView),
		"PUT /api/host-services/{unit}/override":         string(rbac.PermHostServicesManage),
		"DELETE /api/host-services/{unit}/override":      string(rbac.PermHostServicesManage),
		"POST /api/host-services/{unit}/{action}":        string(rbac.PermHostServicesManage),
		"POST /api/host-services/daemon-reload":          string(rbac.PermHostServicesManage),
		"GET /api/host-services/self":                    string(rbac.PermHostSelfView),
		"GET /api/host-services/self/{unit}/override":    string(rbac.PermHostSelfView),
		"PUT /api/host-services/self/{unit}/override":    string(rbac.PermHostSelfManage),
		"DELETE /api/host-services/self/{unit}/override": string(rbac.PermHostSelfManage),
		"POST /api/host-services/self/{unit}/{action}":   string(rbac.PermHostSelfManage),
		"POST /api/host-services/self/daemon-reload":     string(rbac.PermHostSelfManage),
	}
	for _, op := range reg.Operations() {
		key := op.Method + " " + strings.TrimSuffix(op.Path, "/")
		perm, ok := want[key]
		if !ok {
			t.Errorf("undeclared route %s", key)
			continue
		}
		delete(want, key)
		if keys := op.PermissionKeys(); len(keys) != 1 || keys[0] != perm {
			t.Errorf("%s is gated on %v, want %s", key, keys, perm)
		}
		wantScope := rbac.ScopeEnvironment
		if strings.Contains(key, "/self") {
			wantScope = rbac.ScopeGlobal
		}
		if len(op.Perms) != 1 || op.Perms[0].Scope != wantScope {
			t.Errorf("%s is gated with scope %+v, want %s", key, op.Perms, wantScope)
		}
	}
	if len(want) != 0 {
		t.Errorf("routes not declared: %v", want)
	}
}

func TestMutationsAreRefusedWithoutThePermission(t *testing.T) {
	database := openDB(t)
	link := &fakeLink{supported: true}
	mux, _ := mountForTest(t, database, link)
	for _, req := range []*http.Request{
		httptest.NewRequest(http.MethodPost, "/host-services/nginx.service/stop?env=env-native", nil),
		httptest.NewRequest(http.MethodPut, "/host-services/nginx.service/override?env=env-native", strings.NewReader(`{"content":"[Service]\n"}`)),
		httptest.NewRequest(http.MethodDelete, "/host-services/nginx.service/override?env=env-native", nil),
		httptest.NewRequest(http.MethodPost, "/host-services/daemon-reload?env=env-native", nil),
		httptest.NewRequest(http.MethodGet, "/host-services?env=env-native", nil),
		httptest.NewRequest(http.MethodPost, "/host-services/self/nginx.service/stop", nil),
		httptest.NewRequest(http.MethodPut, "/host-services/self/nginx.service/override", strings.NewReader(`{"content":"[Service]
"}`)),
		httptest.NewRequest(http.MethodDelete, "/host-services/self/nginx.service/override", nil),
		httptest.NewRequest(http.MethodPost, "/host-services/self/daemon-reload", nil),
		httptest.NewRequest(http.MethodGet, "/host-services/self", nil),
	} {
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, as("nobody", req))
		if rec.Code != http.StatusForbidden {
			t.Errorf("%s %s: status %d, want 403", req.Method, req.URL.Path, rec.Code)
		}
	}
	if sent := link.sent(); len(sent) != 0 {
		t.Fatalf("refused requests reached the agent: %+v", sent)
	}
}

func TestOverrideSaveIsAuditedWithTheContentHash(t *testing.T) {
	database := openDB(t)
	link := &fakeLink{supported: true, reply: func(p coreagent.HostServicePayload) *coreagent.HostServicePayload {
		return &coreagent.HostServicePayload{Op: p.Op, Success: true, OverridePath: "/etc/systemd/system/nginx.service.d/override.conf", Reloaded: true}
	}}
	mux, _ := mountForTest(t, database, link)
	content := "[Service]\nRestart=always\n"
	body, err := json.Marshal(OverrideRequest{Content: content, Reload: true})
	if err != nil {
		t.Fatal(err)
	}
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, as("system", httptest.NewRequest(http.MethodPut, "/host-services/nginx.service/override?env=env-native", strings.NewReader(string(body)))))
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
	}
	var details, envID string
	if err := database.QueryRow(`SELECT details, COALESCE(environment_id, '') FROM audit_logs WHERE action = 'host_service_override_saved' AND entity_id = 'nginx.service'`).Scan(&details, &envID); err != nil {
		t.Fatalf("no audit row: %v", err)
	}
	if !strings.Contains(details, ContentHash(content)) || envID != "env-native" {
		t.Fatalf("audit details %q env %q must carry the sha256 and the environment", details, envID)
	}
	if sent := link.sent(); len(sent) != 1 || sent[0].Content != content || !sent[0].Reload {
		t.Fatalf("sent = %+v", sent)
	}
}

func TestProtectedStopOverHTTPIsAuditedAndRefused(t *testing.T) {
	database := openDB(t)
	link := &fakeLink{supported: true}
	mux, _ := mountForTest(t, database, link)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, as("system", httptest.NewRequest(http.MethodPost, "/host-services/sshd.service/stop?env=env-native", nil)))
	if rec.Code != http.StatusConflict || !strings.Contains(rec.Body.String(), string(i18n.ErrHostServicesProtected)) {
		t.Fatalf("status %d body %s", rec.Code, rec.Body.String())
	}
	var details string
	if err := database.QueryRow(`SELECT details FROM audit_logs WHERE action = 'host_service_stop' AND entity_id = 'sshd.service'`).Scan(&details); err != nil || !strings.Contains(details, "refused") {
		t.Fatalf("audit = %q, %v", details, err)
	}
	if len(link.sent()) != 0 {
		t.Fatal("a protected stop reached the agent")
	}
}

func TestSystemdReasonReachesTheErrorText(t *testing.T) {
	database := openDB(t)
	reason := "Job for nginx.service failed because the control process exited with error code."
	link := &fakeLink{supported: true, reply: func(p coreagent.HostServicePayload) *coreagent.HostServicePayload {
		return &coreagent.HostServicePayload{Op: p.Op, ErrorCode: "failed", Error: reason}
	}}
	mux, _ := mountForTest(t, database, link)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, as("system", httptest.NewRequest(http.MethodPost, "/host-services/nginx.service/start?env=env-native", nil)))
	var body struct {
		Success bool   `json:"success"`
		Error   string `json:"error"`
		Code    string `json:"code"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if rec.Code != http.StatusConflict || body.Success || body.Code != string(i18n.ErrHostServicesActionFailed) || !strings.Contains(body.Error, reason) {
		t.Fatalf("status %d body %+v; the operator must see systemd's own reason", rec.Code, body)
	}
}
