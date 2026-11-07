// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package updates

import (
	"context"
	"database/sql"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/go-chi/chi/v5"

	coreagent "github.com/therealmcsparrow/mcharbor/core/agent"
	"github.com/therealmcsparrow/mcharbor/core/apispec"
	"github.com/therealmcsparrow/mcharbor/core/auth"
	"github.com/therealmcsparrow/mcharbor/core/rbac"
	"github.com/therealmcsparrow/mcharbor/modules/agentreleases"
)

const (
	retPrerelease = "2026.11.02-alpha.1"
	retStable     = "2026.11.01"
)

type fakeBins struct{ path string }

func (f fakeBins) AgentBinaryPath(string, string) string { return f.path }
func (f fakeBins) FileSHA256(string) (string, error)     { return "stored-prerelease-sha", nil }
func (f fakeBins) BundledBinaryDigest(string, string) (string, error) {
	return "bundled-sha", nil
}
func (f fakeBins) HasLocalAgentBinary(string, string) bool { return false }
func (f fakeBins) LocalAgentBuild() string                 { return "" }

type fakeStable struct {
	cand agentreleases.ReturnCandidate
	ok   bool
}

func (f fakeStable) NewestStable(context.Context, string, string, bool) (agentreleases.ReturnCandidate, bool) {
	return f.cand, f.ok
}

type returnFixture struct {
	t       *testing.T
	h       *Handler
	db      *sql.DB
	checker *SelfUpdateChecker
	updater *AgentUpdater
	pool    *coreagent.AgentPool
	rbacSvc *rbac.Service
	mux     *chi.Mux

	mu       sync.Mutex
	sent     []coreagent.AgentUpdatePayload
	sendErr  error
	sendMode string
	hold     chan struct{}
	sendNil  bool
	onSend   func()
}

func newReturnFixture(t *testing.T, kind string) *returnFixture {
	t.Helper()
	h, database := channelsHandler(t)
	f := &returnFixture{t: t, h: h, db: database, sendMode: "binary"}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	for _, id := range []string{"env1", "env2"} {
		if _, err := database.Exec(`INSERT INTO environments (id, name, connection_type, agent_install_kind, created_at, updated_at)
			VALUES (?, ?, 'agent', ?, 'now', 'now')`, id, "edge-"+id, kind); err != nil {
			t.Fatal(err)
		}
	}
	f.pool = coreagent.NewAgentPool(logger)
	for _, id := range []string{"env1", "env2"} {
		f.pool.Register(id, &coreagent.AgentConnection{EnvID: id, OS: "linux", Arch: "amd64", Version: retPrerelease, ServerURL: "https://mc.example/"})
	}
	f.updater = NewAgentUpdater(f.pool, logger, nil, fakeBins{})
	f.updater.returns = fakeStable{cand: agentreleases.ReturnCandidate{Version: retStable, Build: "202611050000", BinarySHA: "bin-sha", ImageSHA: "img-sha", ImageRef: "mcharbor-agent:" + retStable}, ok: true}
	f.updater.hostVersion = func() string { return retPrerelease }
	f.updater.send = func(_ context.Context, _ string, p coreagent.AgentUpdatePayload) (*coreagent.AgentUpdatePayload, error) {
		f.mu.Lock()
		f.sent = append(f.sent, p)
		hold, onSend, sendErr, sendNil := f.hold, f.onSend, f.sendErr, f.sendNil
		f.mu.Unlock()
		if hold != nil {
			<-hold
		}
		if onSend != nil {
			onSend()
		}
		if sendErr != nil {
			return nil, sendErr
		}
		if sendNil {
			return nil, nil
		}
		return &coreagent.AgentUpdatePayload{Mode: f.sendMode}, nil
	}
	f.checker = NewSelfUpdateChecker(database, nil, logger)
	f.checker.SetAgentUpdater(f.updater)
	h.SetSelfUpdateChecker(f.checker)
	f.rbacSvc = rbac.NewService(database)
	f.mux = chi.NewRouter()
	rt := apispec.NewRouter(f.mux, apispec.NewRegistry(), "/api", apispec.GroupProtected)
	rt.Route("/updates", func(r *apispec.Router) { mountAgentReturnRoutes(r, f.rbacSvc, h) })
	return f
}

func (f *returnFixture) sentCount() int {
	f.updater.WaitReturns()
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.sent)
}

// grant gives userID perm through a role of its own. environments.manage is
// global in the catalog, so it needs envID "" to take effect; updates.manage
// is environment scoped and takes the environment it applies to.
func (f *returnFixture) grant(userID, envID, perm string) {
	f.t.Helper()
	if _, err := f.db.Exec(`INSERT OR IGNORE INTO users (id, username, email, password_hash, created_at, updated_at) VALUES (?, ?, ?, 'x', 'now', 'now')`,
		userID, userID, userID+"@example.test"); err != nil {
		f.t.Fatal(err)
	}
	roleID := "role-" + userID + "-" + perm
	if _, err := f.db.Exec(`INSERT INTO roles (id, name, permissions) VALUES (?, ?, ?)`, roleID, roleID, `["`+perm+`"]`); err != nil {
		f.t.Fatal(err)
	}
	var env any
	if envID != "" {
		env = envID
	}
	if _, err := f.db.Exec(`INSERT INTO user_roles (id, user_id, role_id, environment_id) VALUES (?, ?, ?, ?)`, "ur-"+roleID, userID, roleID, env); err != nil {
		f.t.Fatal(err)
	}
}

// grantBoth gives userID both keys a return needs on env1.
func (f *returnFixture) grantBoth(userID string) {
	f.t.Helper()
	f.grant(userID, "env1", "updates.manage")
	f.grant(userID, "", "environments.manage")
}

func (f *returnFixture) do(method, path, userID string) *httptest.ResponseRecorder {
	rec := f.doNoWait(method, path, userID)
	f.updater.WaitReturns()
	return rec
}

func (f *returnFixture) doNoWait(method, path, userID string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, nil)
	if userID != "" {
		req = req.WithContext(context.WithValue(req.Context(), auth.UserContextKey, &auth.User{ID: userID, Username: userID}))
	}
	rec := httptest.NewRecorder()
	f.mux.ServeHTTP(rec, req)
	return rec
}

func (f *returnFixture) settingValue(key string) string {
	f.t.Helper()
	var v string
	err := f.db.QueryRow(`SELECT value FROM settings WHERE key = ?`, key).Scan(&v)
	if errors.Is(err, sql.ErrNoRows) {
		return ""
	}
	if err != nil {
		f.t.Fatal(err)
	}
	return v
}

func (f *returnFixture) exclusions() []string {
	f.t.Helper()
	s, err := f.checker.Settings(context.Background())
	if err != nil {
		f.t.Fatal(err)
	}
	return s.AgentExcludedEnvIDs
}

func (f *returnFixture) rowOf(envID string) AgentInventoryRow {
	f.t.Helper()
	rows, _, err := f.updater.Inventory(context.Background(), f.db)
	if err != nil {
		f.t.Fatal(err)
	}
	for _, r := range rows {
		if r.EnvID == envID {
			return r
		}
	}
	f.t.Fatalf("no inventory row for %s", envID)
	return AgentInventoryRow{}
}

func (f *returnFixture) auditRows(action string) []map[string]string {
	f.t.Helper()
	rows, err := f.db.Query(`SELECT entity_id, details, state_before, state_after FROM audit_logs WHERE action = ? ORDER BY id`, action)
	if err != nil {
		f.t.Fatal(err)
	}
	defer func() {
		if cerr := rows.Close(); cerr != nil {
			f.t.Error(cerr)
		}
	}()
	var out []map[string]string
	for rows.Next() {
		var id, details, before, after string
		if err := rows.Scan(&id, &details, &before, &after); err != nil {
			f.t.Fatal(err)
		}
		out = append(out, map[string]string{"id": id, "details": details, "before": before, "after": after})
	}
	if err := rows.Err(); err != nil {
		f.t.Fatal(err)
	}
	return out
}

const (
	returnPath = "/updates/agents/env1/return"
	pinPath    = "/updates/agents/env1/pin"
)

func TestAgentReturn(t *testing.T) {
	t.Run("both keys move the agent to its platform's newest stable and pin it once", func(t *testing.T) {
		f := newReturnFixture(t, "native")
		f.grantBoth("op")
		rec := f.do(http.MethodPost, returnPath, "op")
		if rec.Code != http.StatusOK {
			t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
		}
		if f.sentCount() != 1 {
			t.Fatalf("expected one move, got %d", f.sentCount())
		}
		p := f.sent[0]
		if p.Version != retStable || p.BinaryURL != "https://mc.example/api/agent/binary/linux/amd64/"+retStable || p.BinarySHA256 != "bin-sha" || p.ImageURL != "" {
			t.Fatalf("payload %+v", p)
		}
		if got := f.exclusions(); len(got) != 1 || got[0] != "env1" {
			t.Fatalf("only the returned agent is excluded, got %v", got)
		}
		if !f.rowOf("env1").Pinned || f.rowOf("env2").Pinned {
			t.Fatalf("only env1 is pinned")
		}
		audit := f.auditRows("agent_return")
		if len(audit) != 1 || audit[0]["id"] != "env1" || !strings.Contains(audit[0]["after"], `"exclusionAdded":true`) || !strings.Contains(audit[0]["before"], retPrerelease) {
			t.Fatalf("audit %v", audit)
		}
	})

	t.Run("an agent that reports no install kind is refused before anything is sent or pinned", func(t *testing.T) {
		f := newReturnFixture(t, "")
		f.grantBoth("op")
		rec := f.do(http.MethodPost, returnPath, "op")
		if rec.Code != http.StatusConflict {
			t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
		}
		f.updater.WaitReturns()
		if f.sentCount() != 0 {
			t.Fatalf("a binary was sent to an agent whose mode is unknown: %+v", f.sent)
		}
		if f.rowOf("env1").Pinned || len(f.exclusions()) != 0 {
			t.Fatalf("a refused return left a pin: %v", f.exclusions())
		}
	})

	t.Run("a sweep skips the pinned agent, even from settings read before the pin", func(t *testing.T) {
		f := newReturnFixture(t, "native")
		f.grantBoth("op")
		stale, err := f.checker.Settings(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		if rec := f.do(http.MethodPost, returnPath, "op"); rec.Code != http.StatusOK {
			t.Fatalf("status %d", rec.Code)
		}
		saved, err := f.checker.SaveSettings(context.Background(), stale)
		if err != nil {
			t.Fatal(err)
		}
		if !containsID(saved.AgentExcludedEnvIDs, "env1") || !containsID(f.exclusions(), "env1") {
			t.Fatalf("a stale save dropped the pin: saved %v now %v", saved.AgentExcludedEnvIDs, f.exclusions())
		}
		// A whole-struct save racing the pin can leave the exclusion row without
		// it; the read must still list the agent.
		if _, err := f.db.Exec(`UPDATE settings SET value = '[]' WHERE key = ?`, selfUpdateAgentExclKey); err != nil {
			t.Fatal(err)
		}
		if !containsID(f.exclusions(), "env1") {
			t.Fatalf("Settings must union the pins into the exclusions")
		}
		before := f.sentCount()
		summary := f.updater.UpdateOutdated(context.Background(), true, f.exclusions())
		excluded := false
		for _, r := range summary.Results {
			if r.EnvID == "env1" && r.Status == "excluded" {
				excluded = true
			}
		}
		if !excluded || f.sentCount() != before {
			t.Fatalf("the sweep contacted the pinned agent: %+v", summary.Results)
		}
	})

	t.Run("one key alone is refused and changes nothing", func(t *testing.T) {
		f := newReturnFixture(t, "native")
		f.grant("updates-only", "env1", "updates.manage")
		f.grant("env-only", "", "environments.manage")
		f.grant("other-env", "env2", "updates.manage")
		f.grant("other-env", "", "environments.manage")
		for _, user := range []string{"updates-only", "env-only", "other-env", ""} {
			rec := f.do(http.MethodPost, returnPath, user)
			want := http.StatusForbidden
			if user == "" {
				want = http.StatusUnauthorized
			}
			if rec.Code != want {
				t.Errorf("%q: status %d, want %d", user, rec.Code, want)
			}
		}
		if f.sentCount() != 0 || f.settingValue(pinListKey) != "" || len(f.auditRows("agent_return")) != 0 {
			t.Fatalf("a refused return changed state")
		}
		if rec := f.do(http.MethodDelete, pinPath, "env-only"); rec.Code != http.StatusForbidden {
			t.Fatalf("pin removal with environments.manage alone: status %d", rec.Code)
		}
		if rec := f.do(http.MethodDelete, pinPath, "updates-only"); rec.Code != http.StatusForbidden {
			t.Fatalf("pin removal with updates.manage alone: status %d", rec.Code)
		}
		f.grantBoth("both")
		if rec := f.do(http.MethodPost, returnPath, "both"); rec.Code != http.StatusOK {
			t.Fatalf("both keys: status %d: %s", rec.Code, rec.Body.String())
		}
	})

	t.Run("a refusal before the pin writes nothing", func(t *testing.T) {
		cases := map[string]func(f *returnFixture){
			"no stable release": func(f *returnFixture) { f.updater.returns = fakeStable{} },
			"disconnected": func(f *returnFixture) {
				f.pool = coreagent.NewAgentPool(slog.New(slog.NewTextHandler(io.Discard, nil)))
				f.updater.pool = f.pool
			},
			"not a prerelease": func(f *returnFixture) {
				f.pool = coreagent.NewAgentPool(slog.New(slog.NewTextHandler(io.Discard, nil)))
				f.pool.Register("env1", &coreagent.AgentConnection{EnvID: "env1", OS: "linux", Arch: "amd64", Version: retStable})
				f.updater.pool = f.pool
			},
		}
		for name, setup := range cases {
			f := newReturnFixture(t, "native")
			f.grantBoth("op")
			setup(f)
			rec := f.do(http.MethodPost, returnPath, "op")
			if rec.Code != http.StatusConflict {
				t.Errorf("%s: status %d, want 409 (%s)", name, rec.Code, rec.Body.String())
			}
			if f.sentCount() != 0 || f.settingValue(pinListKey) != "" || f.settingValue(selfUpdateAgentExclKey) != "" || len(f.auditRows("agent_return")) != 0 {
				t.Errorf("%s: a refusal changed state", name)
			}
		}
	})

	t.Run("a failed move keeps the pin and a redacted reason, and a retry clears the reason", func(t *testing.T) {
		f := newReturnFixture(t, "native")
		f.grantBoth("op")
		f.sendErr = errors.New("download failed: Authorization: Bearer s3cr3tvalue1234567890 via /var/run/docker.sock")
		rec := f.do(http.MethodPost, returnPath, "op")
		if rec.Code != http.StatusOK {
			t.Fatalf("the move runs after the answer, so the answer is started: status %d", rec.Code)
		}
		row := f.rowOf("env1")
		if !row.Pinned || row.ReturnReason == "" || row.Version != retPrerelease {
			t.Fatalf("row after a failed move: %+v", row)
		}
		if strings.Contains(row.ReturnReason, "s3cr3tvalue") || strings.Contains(row.ReturnReason, "docker.sock") || strings.Contains(rec.Body.String(), "s3cr3tvalue") {
			t.Fatalf("the reason leaks: %q / %s", row.ReturnReason, rec.Body.String())
		}
		if !containsID(f.exclusions(), "env1") || len(f.auditRows("agent_return")) != 1 {
			t.Fatalf("the pin must stay and be audited")
		}
		f.mu.Lock()
		f.sendErr = nil
		f.mu.Unlock()
		if rec := f.do(http.MethodPost, returnPath, "op"); rec.Code != http.StatusOK {
			t.Fatalf("retry status %d", rec.Code)
		}
		if row := f.rowOf("env1"); !row.Pinned || row.ReturnReason != "" {
			t.Fatalf("a retry must clear the reason: %+v", row)
		}
		audit := f.auditRows("agent_return")
		if len(audit) != 2 || !strings.Contains(audit[0]["before"], `"pinned":false`) || !strings.Contains(audit[1]["before"], `"pinned":true`) || !strings.Contains(audit[1]["before"], retStable) {
			t.Fatalf("the retry must audit the pin it replaced as the old value: %v", audit)
		}
	})

	t.Run("the answer does not wait for the agent, and a late failure is still recorded", func(t *testing.T) {
		f := newReturnFixture(t, "native")
		f.grantBoth("op")
		f.hold = make(chan struct{})
		f.sendErr = errors.New("agent refused")
		rec := f.doNoWait(http.MethodPost, returnPath, "op")
		if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"started":true`) {
			t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
		}
		states, _ := f.updater.RunStates()
		if row := f.rowOf("env1"); !row.Pinned || row.ReturnReason != "" || states["env1"].Status != "updating" {
			t.Fatalf("while the agent works the row is updating and pinned: %+v %+v", row, states["env1"])
		}
		close(f.hold)
		f.updater.WaitReturns()
		if row := f.rowOf("env1"); row.ReturnReason == "" {
			t.Fatalf("the failure must be recorded once the agent answers: %+v", row)
		}
	})

	t.Run("a caller that went away still gets the failure recorded", func(t *testing.T) {
		f := newReturnFixture(t, "native")
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		f.onSend = cancel
		f.sendErr = errors.New("agent refused")
		if _, err := f.updater.Return(ctx, f.db, "env1"); err != nil {
			t.Fatal(err)
		}
		f.updater.WaitReturns()
		if row := f.rowOf("env1"); row.ReturnReason == "" {
			t.Fatalf("a cancelled request lost the reason: %+v", row)
		}
	})

	t.Run("a transport that answers nothing is a failure, not a panic", func(t *testing.T) {
		f := newReturnFixture(t, "native")
		f.sendNil = true
		if _, err := f.updater.Return(context.Background(), f.db, "env1"); err != nil {
			t.Fatal(err)
		}
		f.updater.WaitReturns()
		if row := f.rowOf("env1"); row.ReturnReason == "" {
			t.Fatalf("a nil reply must be recorded as a failure: %+v", row)
		}
	})

	t.Run("a reason is hidden once the agent runs the pinned version", func(t *testing.T) {
		pin := pinRecord{Version: retStable, Reason: "agent refused"}
		if got := liveReturnReason(pin, retPrerelease); got != "agent refused" {
			t.Fatalf("still on the prerelease, got %q", got)
		}
		if got := liveReturnReason(pin, retStable); got != "" {
			t.Fatalf("the agent moved, got %q", got)
		}
	})

	t.Run("removal releases the pin and its exclusion, and audits old and new", func(t *testing.T) {
		f := newReturnFixture(t, "native")
		f.grantBoth("op")
		f.sendErr = errors.New("agent refused")
		f.do(http.MethodPost, returnPath, "op")
		if rec := f.do(http.MethodDelete, pinPath, "op"); rec.Code != http.StatusOK {
			t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
		}
		row := f.rowOf("env1")
		if row.Pinned || row.ReturnReason != "" || containsID(f.exclusions(), "env1") {
			t.Fatalf("removal left state behind: %+v %v", row, f.exclusions())
		}
		audit := f.auditRows("agent_pin_remove")
		if len(audit) != 1 || !strings.Contains(audit[0]["before"], `"pinned":true`) || !strings.Contains(audit[0]["before"], retStable) || !strings.Contains(audit[0]["after"], `"pinned":false`) {
			t.Fatalf("audit %v", audit)
		}
		if rec := f.do(http.MethodDelete, pinPath, "op"); rec.Code != http.StatusNotFound {
			t.Fatalf("removing a missing pin: status %d", rec.Code)
		}
	})

	t.Run("removal keeps an exclusion the operator set themselves", func(t *testing.T) {
		f := newReturnFixture(t, "native")
		f.grantBoth("op")
		if _, err := f.checker.SaveSettings(context.Background(), SelfUpdateSettings{Enabled: true, AgentExcludedEnvIDs: []string{"env1"}}); err != nil {
			t.Fatal(err)
		}
		f.do(http.MethodPost, returnPath, "op")
		if audit := f.auditRows("agent_return"); len(audit) != 1 || !strings.Contains(audit[0]["after"], `"exclusionAdded":false`) {
			t.Fatalf("audit %v", audit)
		}
		f.do(http.MethodDelete, pinPath, "op")
		if !containsID(f.exclusions(), "env1") {
			t.Fatalf("removal dropped the operator's own exclusion")
		}
	})

	t.Run("the bundled binary is offered only from a stable host, and only when it is the newest", func(t *testing.T) {
		f := newReturnFixture(t, "native")
		f.grantBoth("op")
		f.updater.bins = fakeBins{path: "/agent-bin/mcharbor-agent-linux-amd64"}
		f.updater.returns = fakeStable{cand: agentreleases.ReturnCandidate{Version: "2026.10.01", BinarySHA: "store-sha"}, ok: true}
		f.updater.hostVersion = func() string { return retStable }
		if rec := f.do(http.MethodPost, returnPath, "op"); rec.Code != http.StatusOK {
			t.Fatalf("status %d", rec.Code)
		}
		if p := f.sent[0]; p.Version != retStable || p.BinarySHA256 != "bundled-sha" {
			t.Fatalf("the newer bundled agent must win: %+v", p)
		}
		g := newReturnFixture(t, "native")
		g.grantBoth("op")
		g.updater.bins = fakeBins{path: "/agent-bin/mcharbor-agent-linux-amd64"}
		g.updater.returns = fakeStable{}
		if rec := g.do(http.MethodPost, returnPath, "op"); rec.Code != http.StatusConflict {
			t.Fatalf("a prerelease host must not offer its bundled agent: status %d", rec.Code)
		}
	})

	t.Run("the bundled digest is the bundled file's, not the selected prerelease's", func(t *testing.T) {
		f := newReturnFixture(t, "native")
		f.grantBoth("op")
		f.updater.bins = fakeBins{path: "/stored/prerelease/mcharbor-agent-linux-amd64"}
		f.updater.returns = fakeStable{}
		f.updater.hostVersion = func() string { return retStable }
		if rec := f.do(http.MethodPost, returnPath, "op"); rec.Code != http.StatusOK {
			t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
		}
		if p := f.sent[0]; p.BinarySHA256 != "bundled-sha" {
			t.Fatalf("the agent checks this digest against the bundled file the version route serves: %+v", p)
		}
	})

	t.Run("a container agent is moved by image and refused without one", func(t *testing.T) {
		f := newReturnFixture(t, "container")
		f.grantBoth("op")
		if rec := f.do(http.MethodPost, returnPath, "op"); rec.Code != http.StatusOK {
			t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
		}
		p := f.sent[0]
		if p.ImageURL != "https://mc.example/api/agent/image/linux/amd64/"+retStable || p.ImageSHA256 != "img-sha" || p.ImageRef != "mcharbor-agent:"+retStable {
			t.Fatalf("payload %+v", p)
		}
		g := newReturnFixture(t, "container")
		g.grantBoth("op")
		g.updater.returns = fakeStable{}
		g.updater.bins = fakeBins{path: "/agent-bin/x"}
		g.updater.hostVersion = func() string { return retStable }
		if rec := g.do(http.MethodPost, returnPath, "op"); rec.Code != http.StatusConflict || g.sentCount() != 0 {
			t.Fatalf("a bundled binary cannot move a container agent: status %d", rec.Code)
		}
	})

	t.Run("a hostile env id never reaches a query", func(t *testing.T) {
		f := newReturnFixture(t, "native")
		f.grantBoth("op")
		for _, id := range []string{"..", "env1%27%20OR%201=1", "a%20b", strings.Repeat("a", 80)} {
			for _, m := range []string{http.MethodPost, http.MethodDelete} {
				suffix := "/return"
				if m == http.MethodDelete {
					suffix = "/pin"
				}
				rec := f.do(m, "/updates/agents/"+id+suffix, "op")
				if rec.Code == http.StatusOK {
					t.Errorf("%s %q succeeded", m, id)
				}
			}
		}
		if f.sentCount() != 0 {
			t.Fatalf("a hostile id caused a move")
		}
	})
}

func TestInventoryFlagsWhetherEachAgentIsConnected(t *testing.T) {
	f := newReturnFixture(t, "binary")
	if _, err := f.db.Exec(`INSERT INTO environments (id, name, connection_type, agent_install_kind, created_at, updated_at)
		VALUES ('env3', 'edge-env3', 'agent', 'binary', 'now', 'now')`); err != nil {
		t.Fatal(err)
	}
	rows, _, err := f.updater.Inventory(context.Background(), f.db)
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]AgentInventoryRow{}
	for _, r := range rows {
		got[r.EnvID] = r
	}
	if !got["env1"].Connected || !got["env2"].Connected {
		t.Fatalf("a registered agent must read as connected: %+v", got)
	}
	off, ok := got["env3"]
	if !ok || off.Connected || off.Updatable || off.Outdated {
		t.Fatalf("an agent with no connection must be listed as not connected and not actionable: %+v", off)
	}
}
