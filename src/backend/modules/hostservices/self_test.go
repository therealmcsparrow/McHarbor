// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package hostservices

import (
	"context"
	"database/sql"
	"encoding/base64"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/therealmcsparrow/mcharbor/core/apispec"
	"github.com/therealmcsparrow/mcharbor/core/audit"
	"github.com/therealmcsparrow/mcharbor/core/i18n"
	"github.com/therealmcsparrow/mcharbor/core/rbac"
	"github.com/therealmcsparrow/mcharbor/core/router"
	"github.com/therealmcsparrow/mcharbor/core/selfhost"
)

// errUnreachable stands in for the system module's unreachable error.
type errUnreachable struct{ reason string }

func (e errUnreachable) Error() string { return e.reason }

// fakeSelfHost is a scripted machine: it records every script it was asked to
// run and answers with output, so a test can prove what reached the host.
type fakeSelfHost struct {
	mu      sync.Mutex
	scripts []string
	rw      []bool
	output  func(script string) string
	err     error
}

func (f *fakeSelfHost) run(_ context.Context, script string, readWrite bool, _ time.Duration) (*selfhost.Result, error) {
	f.mu.Lock()
	f.scripts = append(f.scripts, script)
	f.rw = append(f.rw, readWrite)
	f.mu.Unlock()
	if f.err != nil {
		return nil, f.err
	}
	out := ""
	if f.output != nil {
		out = f.output(script)
	}
	return &selfhost.Result{Output: out}, nil
}

func (f *fakeSelfHost) ran() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.scripts...)
}

func reasonOf(err error) (string, bool) {
	var u errUnreachable
	if errors.As(err, &u) {
		return u.reason, true
	}
	return "", false
}

func newSelfService(database *sql.DB, host *fakeSelfHost) *Service {
	svc := newTestService(database, nil)
	svc.self = &selfLink{run: host.run, unreachable: reasonOf}
	return svc
}

// marker renders one emitted line the way the TTY delivers it, with CRLF.
func marker(key, value string) string {
	return selfMarker + key + " " + value + "\r\n"
}

func b64(text string) string { return base64.StdEncoding.EncodeToString([]byte(text)) }

func TestSelfListParsesTheHostAndMarksMcHarborsOwnUnits(t *testing.T) {
	unitsJSON := `[{"unit":"nginx.service","load":"loaded","active":"failed","sub":"failed","description":"web"},` +
		`{"unit":"mcharbor.service","load":"loaded","active":"active","sub":"running","description":"McHarbor"},` +
		`{"unit":"docker.service","load":"loaded","active":"active","sub":"running","description":"Docker"},` +
		`{"unit":"mcharbor-agent.service","load":"loaded","active":"active","sub":"running","description":"agent"}]`
	filesJSON := `[{"unit_file":"/usr/lib/systemd/system/nginx.service","state":"enabled"},{"unit_file":"cron.service","state":"disabled"}]`
	show := "Id=nginx.service\nMainPID=0\nResult=exit-code\nExecMainStatus=1\nExecMainCode=1\n\nId=mcharbor.service\nMainPID=42\n"
	host := &fakeSelfHost{output: func(string) string {
		return "banner\r\n" + marker("units_json_rc", "0") + marker("units_json", b64(unitsJSON)) +
			marker("units_plain_rc", "0") + marker("units_plain", "") +
			marker("files_json_rc", "0") + marker("files_json", b64(filesJSON)) +
			marker("show_rc", "0") + marker("show", b64(show))
	}}
	list, err := newSelfService(openDB(t), host).List(context.Background(), SelfTarget())
	if err != nil {
		t.Fatal(err)
	}
	if !list.Availability.Supported {
		t.Fatalf("availability = %+v", list.Availability)
	}
	byName := map[string]Unit{}
	for _, u := range list.Units {
		byName[u.Name] = u
	}
	if len(byName) != 5 {
		t.Fatalf("units = %+v; the unloaded cron unit file must still be listed", list.Units)
	}
	if u := byName["mcharbor.service"]; u.Protected != "mcharbor" || !u.RestartsMcHarbor || u.MainPID != 42 {
		t.Errorf("mcharbor.service = %+v", u)
	}
	if u := byName["docker.service"]; u.Protected != "self_runtime" || !u.RestartsMcHarbor {
		t.Errorf("docker.service = %+v", u)
	}
	if u := byName["mcharbor-agent.service"]; u.Protected != "agent" || u.IsAgent || u.RestartsMcHarbor {
		t.Errorf("an agent unit on McHarbor's own host is protected but its restart drops no connection: %+v", u)
	}
	if u := byName["nginx.service"]; u.UnitFileState != "enabled" || !strings.Contains(u.FailureReason, "exited with status 1") {
		t.Errorf("nginx.service = %+v", u)
	}
	scripts := host.ran()
	if len(scripts) != 1 || !strings.HasPrefix(scripts[0], "chroot /host sh -c '") {
		t.Fatalf("scripts = %q; the list must be one run on the host", scripts)
	}
}

func TestSelfListExplainsAnUnreachableHost(t *testing.T) {
	reason := "McHarbor runs in a container but cannot find itself on the Docker daemon it is connected to."
	host := &fakeSelfHost{err: fmt.Errorf("resolving: %w", errUnreachable{reason: reason})}
	list, err := newSelfService(openDB(t), host).List(context.Background(), SelfTarget())
	if err != nil {
		t.Fatal(err)
	}
	if list.Availability.Supported || list.Availability.Reason != "self_unreachable" || list.Availability.Detail != reason || len(list.Units) != 0 {
		t.Fatalf("list = %+v", list)
	}
}

func TestSelfListExplainsAHostWithoutSystemd(t *testing.T) {
	host := &fakeSelfHost{output: func(string) string { return marker("nosystemd", "1") }}
	list, err := newSelfService(openDB(t), host).List(context.Background(), SelfTarget())
	if err != nil || list.Availability.Reason != "self_no_systemd" || list.Availability.Supported {
		t.Fatalf("list = %+v, err = %v", list, err)
	}
}

func TestSelfProtectedStopNeverReachesTheHost(t *testing.T) {
	host := &fakeSelfHost{}
	svc := newSelfService(openDB(t), host)
	for _, unit := range []string{"mcharbor.service", "docker.service", "containerd.service", "sshd.service", "mcharbor-agent.service", "systemd-journald.service"} {
		for _, action := range []string{"stop", "disable"} {
			_, err := svc.Action(context.Background(), SelfTarget(), unit, action)
			var refusal *Refusal
			if !errors.As(err, &refusal) || refusal.Code != i18n.ErrHostServicesProtected {
				t.Errorf("%s %s on the self host: err = %v, want protected", action, unit, err)
			}
		}
	}
	if scripts := host.ran(); len(scripts) != 0 {
		t.Fatalf("refused actions reached the host: %q", scripts)
	}
}

func TestSelfRestartOfTheRuntimeIsQueuedAndWarns(t *testing.T) {
	host := &fakeSelfHost{output: func(string) string { return marker("action_rc", "0") + marker("action", "") }}
	svc := newSelfService(openDB(t), host)
	result, err := svc.Action(context.Background(), SelfTarget(), "docker.service", "restart")
	if err != nil || result.Warning != WarningMcHarborRestart {
		t.Fatalf("result = %+v, err = %v", result, err)
	}
	if scripts := host.ran(); len(scripts) != 1 || !strings.Contains(scripts[0], "--no-block restart -- docker.service") {
		t.Fatalf("scripts = %q; a restart that takes McHarbor down must not be awaited", scripts)
	}
	result, err = svc.Action(context.Background(), SelfTarget(), "nginx.service", "restart")
	if err != nil || result.Warning != "" {
		t.Fatalf("result = %+v, err = %v", result, err)
	}
	if last := host.ran()[1]; strings.Contains(last, "--no-block") || !strings.Contains(last, "systemctl --no-ask-password restart -- nginx.service") {
		t.Fatalf("script = %q", last)
	}
}

func TestSelfActionFailureCarriesSystemdsReason(t *testing.T) {
	reason := "Job for nginx.service failed because the control process exited with error code."
	host := &fakeSelfHost{output: func(string) string { return marker("action_rc", "1") + marker("action", b64(reason)) }}
	_, err := newSelfService(openDB(t), host).Action(context.Background(), SelfTarget(), "nginx.service", "start")
	var refusal *Refusal
	if !errors.As(err, &refusal) || refusal.Code != i18n.ErrHostServicesActionFailed || !strings.Contains(refusal.Detail, "control process exited") {
		t.Fatalf("err = %v", err)
	}
}

func TestSelfOverrideSaveCarriesContentSafelyAndReloads(t *testing.T) {
	content := "[Service]\nEnvironment='A=it'\"s\"\nExecStart=\n"
	host := &fakeSelfHost{output: func(string) string { return marker("written", "1") + marker("reload_rc", "0") + marker("reload", "") }}
	result, err := newSelfService(openDB(t), host).SaveOverride(context.Background(), SelfTarget(), "nginx.service",
		OverrideRequest{Content: content, Reload: true})
	if err != nil || !result.Reloaded || result.Path != "/etc/systemd/system/nginx.service.d/override.conf" {
		t.Fatalf("result = %+v, err = %v", result, err)
	}
	script := host.ran()[0]
	inner := strings.TrimSuffix(strings.TrimPrefix(script, "chroot /host sh -c '"), "'")
	if strings.Contains(inner, "'") {
		t.Fatalf("the content broke out of the quoted script: %q", script)
	}
	if !strings.Contains(inner, b64(content)) || !strings.Contains(inner, "systemctl daemon-reload") {
		t.Fatalf("script = %q", inner)
	}
	if !host.rw[0] {
		t.Fatal("a write must run with a read-write host mount")
	}
}

func TestSelfOverrideReadDecodesTheDropIn(t *testing.T) {
	host := &fakeSelfHost{output: func(string) string {
		return marker("effective_rc", "0") + marker("effective", b64("# /usr/lib/systemd/system/nginx.service\n[Unit]\n")) +
			marker("exists", "1") + marker("size", "18") + marker("override", b64("[Service]\nNice=5\n"))
	}}
	override, err := newSelfService(openDB(t), host).Override(context.Background(), SelfTarget(), "nginx.service")
	if err != nil || !override.Exists || override.Content != "[Service]\nNice=5\n" || !strings.Contains(override.Effective, "[Unit]") {
		t.Fatalf("override = %+v, err = %v", override, err)
	}
}

func TestSelfStepFailureNamesTheStep(t *testing.T) {
	host := &fakeSelfHost{output: func(string) string { return marker("failed", "mkdir") + marker("err", b64("Read-only file system")) }}
	err := newSelfService(openDB(t), host).RevertOverride(context.Background(), SelfTarget(), "nginx.service")
	var refusal *Refusal
	if !errors.As(err, &refusal) || refusal.Code != i18n.ErrHostServicesOverrideFailed || !strings.Contains(refusal.Detail, "Read-only file system") {
		t.Fatalf("err = %v", err)
	}
}

func TestEnvironmentScopedGrantDoesNotReachTheSelfHost(t *testing.T) {
	database := openDB(t)
	for _, stmt := range []string{
		`INSERT INTO users (id, username, password_hash) VALUES ('scoped', 'scoped', 'x')`,
		`INSERT INTO roles (id, name, permissions) VALUES ('svc-role', 'svc-role', '["host_services.view","host_services.manage"]')`,
		`INSERT INTO user_roles (id, user_id, role_id, environment_id) VALUES ('ur-scoped', 'scoped', 'svc-role', 'env-native')`,
	} {
		if _, err := database.Exec(stmt); err != nil {
			t.Fatalf("%s: %v", stmt, err)
		}
	}
	host := &fakeSelfHost{output: func(string) string { return marker("reload_rc", "0") + marker("reload", "") }}
	app := &router.AppDeps{DB: database, RBACService: rbac.NewService(database), AuditLog: audit.NewLogger(database, "test-secret"), Logger: slog.Default()}
	h := &Handler{app: app, svc: newSelfService(database, host)}
	mux := chi.NewRouter()
	MountRoutes(apispec.NewRouter(mux, apispec.NewRegistry(), "/api", apispec.GroupProtected), app, h)

	for _, path := range []string{"/host-services/self/daemon-reload", "/host-services/self/daemon-reload?env=env-native"} {
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, as("scoped", httptest.NewRequest(http.MethodPost, path, nil)))
		if rec.Code != http.StatusForbidden {
			t.Errorf("POST %s with an env-scoped grant: status %d, want 403", path, rec.Code)
		}
	}
	if scripts := host.ran(); len(scripts) != 0 {
		t.Fatalf("an environment-scoped grant reached McHarbor's own host: %q", scripts)
	}
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, as("scoped", httptest.NewRequest(http.MethodPost, "/host-services/daemon-reload?env=env-native", nil)))
	if rec.Code == http.StatusForbidden {
		t.Fatal("the grant must be live on its own environment, or the 403s above prove nothing")
	}

	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, as("system", httptest.NewRequest(http.MethodPost, "/host-services/self/daemon-reload", nil)))
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
	}
	var details, envID string
	if err := database.QueryRow(`SELECT details, COALESCE(environment_id, '') FROM audit_logs WHERE action = 'host_service_daemon_reload' AND COALESCE(environment_id, '') = ''`).Scan(&details, &envID); err != nil {
		t.Fatalf("no audit row: %v", err)
	}
	if envID != "" || !strings.HasPrefix(details, "McHarbor's own host: ") {
		t.Fatalf("audit details %q env %q must name McHarbor's own host", details, envID)
	}
}
