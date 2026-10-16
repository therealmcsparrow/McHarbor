// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package cli

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

const goodKey = "mch_goodkey000000000000"

// fakeServer answers like McHarbor: the envelope, the error codes, and a
// record of every request, so a test can prove a refused command sent nothing.
type fakeServer struct {
	*httptest.Server
	mu       sync.Mutex
	requests []*http.Request
	bodies   map[string]string
	runPolls int
}

func (f *fakeServer) seen() []*http.Request {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]*http.Request(nil), f.requests...)
}

func ok(w http.ResponseWriter, data any) {
	w.Header().Set("Content-Type", "application/json")
	warnErr(json.NewEncoder(w).Encode(map[string]any{"success": true, "data": data}))
}

func fail(w http.ResponseWriter, status int, code, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	warnErr(json.NewEncoder(w).Encode(map[string]any{"success": false, "error": msg, "code": code}))
}

func newFakeServer(t *testing.T) *fakeServer {
	t.Helper()
	f := &fakeServer{bodies: map[string]string{}}
	f.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw := valueOr(io.ReadAll(r.Body))
		f.mu.Lock()
		f.requests = append(f.requests, r.Clone(r.Context()))
		f.bodies[r.Method+" "+r.URL.Path] = string(raw)
		f.mu.Unlock()
		if r.URL.Path == "/api/health" {
			ok(w, map[string]string{"status": "ok"})
			return
		}
		if r.Header.Get("Authorization") != "Bearer "+goodKey {
			fail(w, http.StatusUnauthorized, "err.auth.required", "Authentication required")
			return
		}
		switch {
		case r.URL.Path == "/api/auth/session":
			ok(w, map[string]any{"id": "u1", "username": "ci-bot"})
		case r.URL.Path == "/api/containers" && r.Method == http.MethodGet:
			ok(w, []map[string]any{{"Id": "abcdef1234567890", "Names": []string{"/web"}, "Image": "nginx:1", "State": "running", "Status": "Up 2 hours"}})
		case r.URL.Path == "/api/environments":
			ok(w, map[string]any{"items": []map[string]any{{"id": "e1", "name": "prod", "stackCount": 3}}, "total": 1, "page": 1, "perPage": 25})
		case r.URL.Path == "/api/volumes" && r.Method == http.MethodPost:
			fail(w, http.StatusForbidden, "err.rbac.permission_denied", "You do not have permission to perform this action")
		case strings.HasPrefix(r.URL.Path, "/api/volumes/") && r.Method == http.MethodDelete:
			w.WriteHeader(http.StatusNoContent)
		case r.URL.Path == "/api/volumes/missing":
			fail(w, http.StatusNotFound, "err.volume.not_found", "Volume not found")
		case r.URL.Path == "/api/stacks/web/down":
			ok(w, map[string]any{"success": true})
		case r.URL.Path == "/api/stacks/busy/up":
			fail(w, http.StatusConflict, "err.stack.busy", "Stack is busy")
		case r.URL.Path == "/api/containers/crash/start":
			fail(w, http.StatusInternalServerError, "err.container.start_failed", "Failed to start container")
		case r.URL.Path == "/api/containers/slow/start":
			time.Sleep(500 * time.Millisecond)
			ok(w, nil)
		case r.URL.Path == "/api/container-backups/p1/run":
			ok(w, map[string]any{"id": "r1", "status": "running", "progressStage": "archiving"})
		case r.URL.Path == "/api/container-backups/p2/run":
			ok(w, map[string]any{"id": "r2", "status": "running"})
		case r.URL.Path == "/api/container-backups/runs/r1":
			f.mu.Lock()
			f.runPolls++
			n := f.runPolls
			f.mu.Unlock()
			if n < 2 {
				ok(w, map[string]any{"id": "r1", "status": "uploading", "progressStage": "uploading", "progressMessage": "to s3"})
				return
			}
			ok(w, map[string]any{"id": "r1", "status": "success", "archiveSize": 2048})
		case r.URL.Path == "/api/container-backups/runs/r2":
			ok(w, map[string]any{"id": "r2", "status": "failure", "error": "disk full"})
		case r.URL.Path == "/api/gitops/approvals/ap1/approve":
			fail(w, http.StatusForbidden, "err.rbac.permission_denied", "You do not have permission to perform this action")
		case r.URL.Path == "/api/gitops/approvals/ap2/reject":
			ok(w, map[string]any{"id": "ap2", "status": "rejected"})
		case r.URL.Path == "/api/about":
			ok(w, map[string]any{"version": "2026.09.01", "build": "202609250000"})
		default:
			fail(w, http.StatusNotFound, "err.not_found", "no route "+r.Method+" "+r.URL.Path)
		}
	}))
	t.Cleanup(f.Close)
	return f
}

func run(t *testing.T, f *fakeServer, key string, args ...string) (int, string, string) {
	t.Helper()
	t.Setenv("MCHARBOR_CONFIG_DIR", t.TempDir())
	t.Setenv("MCHARBOR_NO_KEYRING", "1")
	t.Setenv("MCHARBOR_CONTEXT", "")
	t.Setenv("MCHARBOR_ENV", "")
	if f != nil {
		t.Setenv("MCHARBOR_URL", f.URL)
	}
	t.Setenv("MCHARBOR_API_KEY", key)
	var out, errOut bytes.Buffer
	code := Execute(args, &out, &errOut)
	return code, out.String(), errOut.String()
}

func TestExitCodesAreFrozen(t *testing.T) {
	f := newFakeServer(t)
	cases := []struct {
		name string
		key  string
		args []string
		want int
	}{
		{"success", goodKey, []string{"container", "list"}, ExitOK},
		{"auth", "mch_wrong", []string{"container", "list"}, ExitAuth},
		{"forbidden", goodKey, []string{"volume", "create", "v1"}, ExitForbidden},
		{"not found", goodKey, []string{"volume", "get", "missing"}, ExitNotFound},
		{"conflict", goodKey, []string{"stack", "up", "busy"}, ExitConflict},
		{"remote", goodKey, []string{"container", "start", "crash"}, ExitRemote},
		{"timeout", goodKey, []string{"--timeout", "100ms", "container", "start", "slow"}, ExitTimeout},
		{"unknown subcommand", goodKey, []string{"container", "frobnicate"}, ExitUsage},
		{"unknown flag", goodKey, []string{"container", "list", "--nope"}, ExitUsage},
		{"bad output", goodKey, []string{"-o", "xml", "container", "list"}, ExitUsage},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if code, _, stderr := run(t, f, c.key, c.args...); code != c.want {
				t.Fatalf("exit %d, want %d; stderr %s", code, c.want, stderr)
			}
		})
	}
}

func TestDeniedCommandNamesTheMissingPermission(t *testing.T) {
	f := newFakeServer(t)
	code, _, stderr := run(t, f, goodKey, "volume", "create", "v1")
	if code != ExitForbidden || !strings.Contains(stderr, "missing permission: volumes.manage") {
		t.Fatalf("exit %d, stderr %q; want 4 naming volumes.manage", code, stderr)
	}

	code, _, stderr = run(t, f, goodKey, "--json", "volume", "create", "v1")
	var e Error
	if err := json.Unmarshal([]byte(stderr), &e); err != nil {
		t.Fatalf("--json error is not JSON: %q", stderr)
	}
	if code != ExitForbidden || e.Code != ExitForbidden || len(e.Permissions) != 1 || e.Permissions[0] != "volumes.manage" || e.ServerCode != "err.rbac.permission_denied" {
		t.Fatalf("machine-readable error = %+v", e)
	}
}

func TestIrreversibleChangeNeedsTheTargetNamed(t *testing.T) {
	f := newFakeServer(t)
	before := len(f.seen())
	code, _, stderr := run(t, f, goodKey, "--yes", "volume", "remove", "data")
	if code != ExitUsage || !strings.Contains(stderr, "--confirm data") {
		t.Fatalf("--yes alone: exit %d, stderr %q", code, stderr)
	}
	code, _, _ = run(t, f, goodKey, "volume", "remove", "data", "--confirm", "other")
	if code != ExitUsage {
		t.Fatalf("--confirm naming another resource: exit %d, want 2", code)
	}
	if got := len(f.seen()) - before; got != 0 {
		t.Fatalf("a refused removal still sent %d requests", got)
	}
	if code, out, stderr := run(t, f, goodKey, "volume", "remove", "data", "--confirm", "data"); code != ExitOK || !strings.Contains(out, "removed") {
		t.Fatalf("confirmed removal: exit %d, out %q, stderr %q", code, out, stderr)
	}
}

func TestDisruptiveChangeNeedsYesWithoutATerminal(t *testing.T) {
	f := newFakeServer(t)
	before := len(f.seen())
	if code, _, _ := run(t, f, goodKey, "stack", "down", "web"); code != ExitUsage {
		t.Fatalf("stack down without --yes: exit %d, want 2", code)
	}
	if got := len(f.seen()) - before; got != 0 {
		t.Fatalf("an unconfirmed stack down sent %d requests", got)
	}
	if code, _, stderr := run(t, f, goodKey, "--yes", "stack", "down", "web"); code != ExitOK {
		t.Fatalf("stack down --yes: exit %d, %s", code, stderr)
	}
}

func TestInteractivePromptAcceptsTheTypedName(t *testing.T) {
	f := newFakeServer(t)
	restore := stdin
	restoreTTY := stdinIsTerminal
	t.Cleanup(func() { stdin, stdinIsTerminal = restore, restoreTTY })
	stdinIsTerminal = func() bool { return true }

	stdin = strings.NewReader("wrong\n")
	if code, _, _ := run(t, f, goodKey, "volume", "remove", "data"); code != ExitUsage {
		t.Fatalf("mistyped name: exit %d, want 2", code)
	}
	stdin = strings.NewReader("data\n")
	if code, _, stderr := run(t, f, goodKey, "volume", "remove", "data"); code != ExitOK {
		t.Fatalf("typed name: exit %d, %s", code, stderr)
	}
}

func TestScopedCommandsSendTheEnvironmentAndTheKeyNeverPrints(t *testing.T) {
	f := newFakeServer(t)
	code, out, stderr := run(t, f, goodKey, "--env", "e1", "container", "list")
	if code != ExitOK {
		t.Fatalf("exit %d: %s", code, stderr)
	}
	var last *http.Request
	for _, r := range f.seen() {
		if r.URL.Path == "/api/containers" {
			last = r
		}
	}
	if last == nil || last.URL.Query().Get("env") != "e1" {
		t.Fatalf("container list did not carry env=e1")
	}
	if !strings.Contains(out, "web") || !strings.Contains(out, "abcdef123456") || strings.Contains(out, "abcdef1234567") {
		t.Fatalf("table output = %q", out)
	}
	if strings.Contains(out+stderr, goodKey) {
		t.Fatal("the API key was printed")
	}
}

func TestPaginatedListsAreUnwrapped(t *testing.T) {
	f := newFakeServer(t)
	code, out, _ := run(t, f, goodKey, "-o", "tsv", "env", "list")
	if code != ExitOK || strings.TrimSpace(out) != "e1\tprod\t\t\t\t\t3" {
		t.Fatalf("tsv = %q", out)
	}
}

func TestFollowedRunPrintsProgressToStderrAndResultToStdout(t *testing.T) {
	f := newFakeServer(t)
	restore := pollInterval
	pollInterval = 10 * time.Millisecond
	t.Cleanup(func() { pollInterval = restore })

	code, out, stderr := run(t, f, goodKey, "backup", "plan", "run", "p1")
	if code != ExitOK {
		t.Fatalf("exit %d: %s", code, stderr)
	}
	if !strings.Contains(stderr, "uploading: to s3") || strings.Contains(out, "uploading") {
		t.Fatalf("progress must go to stderr only; stdout %q stderr %q", out, stderr)
	}
	if !strings.Contains(out, "success") {
		t.Fatalf("final state missing from stdout: %q", out)
	}

	code, _, stderr = run(t, f, goodKey, "backup", "plan", "run", "p2")
	if code != ExitRemote || !strings.Contains(stderr, "disk full") {
		t.Fatalf("failed run: exit %d, stderr %q; want 7 with the reason", code, stderr)
	}

	code, out, _ = run(t, f, goodKey, "backup", "plan", "run", "p1", "--no-follow")
	if code != ExitOK || !strings.Contains(out, "backup run get r1") {
		t.Fatalf("--no-follow: exit %d, out %q", code, out)
	}
}

func TestLogFollowResumesFromTheLastEvent(t *testing.T) {
	var mu sync.Mutex
	var cursors []string
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		calls++
		n := calls
		cursors = append(cursors, r.Header.Get("Last-Event-ID"))
		mu.Unlock()
		w.Header().Set("Content-Type", "text/event-stream")
		if n == 1 {
			emit(w, "retry: 1000\n\nid: c1\nevent: log\ndata: {\"message\":\"one\"}\n\nid: c2\nevent: log\ndata: {\"message\":\"two\"}\n\nevent: end\ndata: {\"reason\":\"session_limit\",\"cursor\":\"c2\"}\n\n")
			return
		}
		emit(w, "id: c3\nevent: log\ndata: {\"message\":\"three\"}\n\nevent: end\ndata: {\"reason\":\"eof\",\"cursor\":\"c3\"}\n\n")
	}))
	t.Cleanup(srv.Close)
	t.Setenv("MCHARBOR_URL", srv.URL)
	code, out, stderr := run(t, nil, goodKey, "container", "logs", "web", "-f")
	if code != ExitOK {
		t.Fatalf("exit %d: %s", code, stderr)
	}
	if out != "one\ntwo\nthree\n" {
		t.Fatalf("followed output = %q; no line may be lost or repeated", out)
	}
	if len(cursors) != 2 || cursors[1] != "c2" {
		t.Fatalf("resume cursors = %q, want the second request to carry c2", cursors)
	}
}

func TestLoginStoresTheKeyOutsideTheConfigAndLogoutRemovesIt(t *testing.T) {
	f := newFakeServer(t)
	dir := t.TempDir()
	t.Setenv("MCHARBOR_CONFIG_DIR", dir)
	t.Setenv("MCHARBOR_NO_KEYRING", "1")
	t.Setenv("MCHARBOR_API_KEY", "")
	t.Setenv("MCHARBOR_URL", "")
	restore := stdin
	t.Cleanup(func() { stdin = restore })
	stdin = strings.NewReader(goodKey + "\n")

	var out, errOut bytes.Buffer
	if code := Execute([]string{"login", f.URL, "--name", "ci", "--api-key-stdin"}, &out, &errOut); code != ExitOK {
		t.Fatalf("login exit %d: %s", code, errOut.String())
	}
	cfg, err := os.ReadFile(filepath.Join(dir, "config.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(cfg), goodKey) {
		t.Fatal("the API key was written into config.yaml")
	}
	creds, err := os.ReadFile(filepath.Join(dir, "credentials.yaml"))
	if err != nil || !strings.Contains(string(creds), goodKey) {
		t.Fatalf("credentials file missing the key: %v", err)
	}
	if info, err := os.Stat(filepath.Join(dir, "credentials.yaml")); err == nil && info.Mode().Perm()&0o077 != 0 && os.PathSeparator == '/' {
		t.Fatalf("credentials file mode %v is readable by others", info.Mode().Perm())
	}

	out.Reset()
	if code := Execute([]string{"whoami"}, &out, &errOut); code != ExitOK || !strings.Contains(out.String(), "ci-bot") {
		t.Fatalf("whoami through the stored context: exit %d, out %q", code, out.String())
	}
	if code := Execute([]string{"logout"}, &out, &errOut); code != ExitOK {
		t.Fatalf("logout exit %d", code)
	}
	if code := Execute([]string{"whoami"}, &out, &errOut); code != ExitAuth {
		t.Fatalf("whoami after logout: exit %d, want 3", code)
	}
}

func TestCompatibility(t *testing.T) {
	cases := map[[2]string]string{
		{"2026.09.01", "2026.09.01"}: "compatible",
		{"2026.10.01", "2026.09.01"}: "server is newer",
		{"2026.09.01", "2026.09.02"}: "server is older",
		{"2026.09.10", "2026.09.9"}:  "server is newer",
	}
	for in, want := range cases {
		if got := compatibility(in[0], in[1]); !strings.HasPrefix(got, want) {
			t.Errorf("compatibility(%s, %s) = %q, want %q…", in[0], in[1], got, want)
		}
	}
}

func TestSSEParser(t *testing.T) {
	var got []sseEvent
	err := readSSE(strings.NewReader(": comment\nid: 1\nevent: log\ndata: a\ndata: b\n\nevent: end\ndata: {}\n\n"), func(e sseEvent) bool {
		got = append(got, e)
		return true
	})
	if err != nil || len(got) != 2 || got[0].Data != "a\nb" || got[0].ID != "1" || got[1].Event != "end" {
		t.Fatalf("events = %+v, err %v", got, err)
	}
}

func TestGitopsApprovalCommands(t *testing.T) {
	f := newFakeServer(t)
	code, _, stderr := run(t, f, goodKey, "--yes", "gitops", "approval", "approve", "ap1")
	if code != ExitForbidden || !strings.Contains(stderr, "missing permission: gitops.approve") {
		t.Fatalf("approve without the key: exit %d, stderr %q", code, stderr)
	}
	if code, _, _ := run(t, f, goodKey, "gitops", "approval", "reject", "ap2"); code != ExitUsage {
		t.Fatalf("reject without --yes off a terminal: exit %d, want 2", code)
	}
	if code, out, stderr := run(t, f, goodKey, "--yes", "gitops", "approval", "reject", "ap2", "--note", "not now"); code != ExitOK || !strings.Contains(out, "rejected") {
		t.Fatalf("reject: exit %d, out %q, stderr %q", code, out, stderr)
	}
	f.mu.Lock()
	body := f.bodies["POST /api/gitops/approvals/ap2/reject"]
	f.mu.Unlock()
	if !strings.Contains(body, "not now") {
		t.Fatalf("the note did not reach the server: %q", body)
	}
}

func TestHangingKeyringFallsBackToTheFile(t *testing.T) {
	t.Setenv("MCHARBOR_CONFIG_DIR", t.TempDir())
	t.Setenv("MCHARBOR_API_KEY", "")
	block := make(chan struct{})
	t.Cleanup(func() { close(block) })
	restoreAvail, restoreTimeout := keyringAvailable, keyringTimeout
	restoreGet, restoreSet := keyringGet, keyringSet
	t.Cleanup(func() {
		keyringAvailable, keyringTimeout, keyringGet, keyringSet = restoreAvail, restoreTimeout, restoreGet, restoreSet
	})
	keyringAvailable = func() bool { return true }
	keyringTimeout = 50 * time.Millisecond
	keyringSet = func(_, _, _ string) error { <-block; return nil }
	keyringGet = func(_, _ string) (string, error) { <-block; return "", nil }

	started := time.Now()
	where, err := storeAPIKey("ci", goodKey)
	if err != nil || where != "file" {
		t.Fatalf("store with a hanging keyring: where=%q err=%v, want the file", where, err)
	}
	key, source, err := loadAPIKey("ci")
	if err != nil || key != goodKey || source != "file" {
		t.Fatalf("load with a hanging keyring: key=%q source=%q err=%v", key, source, err)
	}
	if elapsed := time.Since(started); elapsed > 2*time.Second {
		t.Fatalf("a hanging keyring held the CLI for %s", elapsed)
	}

	stored := map[string]string{}
	keyringSet = func(_, user, secret string) error { stored[user] = secret; return nil }
	keyringGet = func(_, user string) (string, error) { return stored[user], nil }
	if where, err := storeAPIKey("desk", goodKey); err != nil || where != "keyring" {
		t.Fatalf("store with a working keyring: where=%q err=%v", where, err)
	}
	if _, source, err := loadAPIKey("desk"); err != nil || source != "keyring" {
		t.Fatalf("a working keyring was not used: source=%q", source)
	}
}
