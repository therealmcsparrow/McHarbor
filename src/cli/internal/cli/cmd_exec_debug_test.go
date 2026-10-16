// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package cli

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

// debugServer is a McHarbor that answers the debug-shell routes, the exec route
// and the terminal socket, and remembers every request in order.
type debugServer struct {
	*httptest.Server
	mu       sync.Mutex
	calls    []string
	bodies   map[string]string
	wsQuery  string
	install  func(w http.ResponseWriter)
	refuseWS int
}

func newDebugServer(t *testing.T) *debugServer {
	t.Helper()
	s := &debugServer{bodies: map[string]string{}}
	s.install = func(w http.ResponseWriter) {
		w.WriteHeader(http.StatusCreated)
		ok(w, map[string]any{"sessionId": "sess1", "path": "/.mcharbor-debug/busybox", "arch": "amd64",
			"busyboxDigest": "sha256:abc", "sizeBytes": 1218448, "installed": true, "expiresAt": "2026-09-27T13:15:00Z"})
	}
	up := websocket.Upgrader{CheckOrigin: func(*http.Request) bool { return true }}
	s.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, err := io.ReadAll(r.Body)
		if err != nil {
			t.Error(err)
		}
		key := r.Method + " " + r.URL.Path
		s.mu.Lock()
		s.calls = append(s.calls, key+"?"+r.URL.RawQuery)
		s.bodies[key] = string(raw)
		s.mu.Unlock()
		switch key {
		case "POST /api/containers/svc/debug-shell":
			s.install(w)
		case "DELETE /api/containers/svc/debug-shell":
			ok(w, map[string]any{"removed": 1})
		case "POST /api/containers/svc/exec":
			ok(w, map[string]any{"stdout": "app\netc\n", "stderr": "", "exitCode": 2})
		case "GET /api/terminal/ws":
			s.mu.Lock()
			s.wsQuery = r.URL.RawQuery
			refuse := s.refuseWS
			s.mu.Unlock()
			if refuse != 0 {
				fail(w, refuse, "err.rbac.permission_denied", "denied")
				return
			}
			c, err := up.Upgrade(w, r, nil)
			if err != nil {
				return
			}
			defer func() { warnErr(c.Close()) }()
			if c.WriteMessage(websocket.BinaryMessage, []byte("/ # ")) != nil {
				return
			}
			if c.WriteMessage(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, "")) != nil {
				return
			}
			time.Sleep(50 * time.Millisecond)
		default:
			fail(w, http.StatusNotFound, "err.not_found", "no route "+key)
		}
	}))
	t.Cleanup(s.Close)
	return s
}

func (s *debugServer) seen(prefix string) []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []string
	for _, c := range s.calls {
		if strings.HasPrefix(c, prefix) {
			out = append(out, c)
		}
	}
	return out
}

func TestExecDebugOpensTheUploadedShell(t *testing.T) {
	fakeTerminal(t, "")
	s := newDebugServer(t)
	code, out, stderr := runShellCLI(t, s.URL, "container", "exec", "svc", "--debug")
	if code != ExitOK || out != "/ # " {
		t.Fatalf("exit %d, out %q, stderr %q", code, out, stderr)
	}
	if !strings.Contains(stderr, "installed at /.mcharbor-debug/busybox") || !strings.Contains(stderr, "sha256:abc") {
		t.Fatalf("the operator must be told what was placed in the container: %q", stderr)
	}
	if install := s.seen("POST /api/containers/svc/debug-shell"); len(install) != 1 || !strings.Contains(install[0], "env=prod") {
		t.Fatalf("install calls %v", install)
	}
	s.mu.Lock()
	query := s.wsQuery
	s.mu.Unlock()
	if !strings.Contains(query, "debugSession=sess1") || strings.Contains(query, "shell=") {
		t.Fatalf("the socket must attach to the debug session and name no shell: %q", query)
	}
}

func TestExecDebugWithACommandRunsBusyboxAndRemovesIt(t *testing.T) {
	s := newDebugServer(t)
	code, out, stderr := runShellCLI(t, s.URL, "container", "exec", "svc", "--debug", "--", "ls", "/app")
	if code != 2 || out != "app\netc\n" {
		t.Fatalf("the command's own status and output must come back: exit %d, out %q, stderr %q", code, out, stderr)
	}
	s.mu.Lock()
	body := s.bodies["POST /api/containers/svc/exec"]
	s.mu.Unlock()
	if !strings.Contains(body, `"command":["/.mcharbor-debug/busybox","ls","/app"]`) {
		t.Fatalf("exec sent %s", body)
	}
	removed := s.seen("DELETE /api/containers/svc/debug-shell")
	if len(removed) != 1 || !strings.Contains(removed[0], "session=sess1") {
		t.Fatalf("the one-shot run must remove its session at once: %v", removed)
	}
}

func TestExecDebugRefusedNamesTheKeyAndTouchesNothingElse(t *testing.T) {
	fakeTerminal(t, "")
	s := newDebugServer(t)
	s.install = func(w http.ResponseWriter) {
		fail(w, http.StatusForbidden, "err.rbac.permission_denied", "You do not have permission to perform this action")
	}
	code, _, stderr := runShellCLI(t, s.URL, "container", "exec", "svc", "--debug")
	if code != ExitForbidden || !strings.Contains(stderr, "containers.debug_shell") {
		t.Fatalf("exit %d, stderr %q", code, stderr)
	}
	if len(s.seen("GET /api/terminal/ws")) != 0 || len(s.seen("POST /api/containers/svc/exec")) != 0 {
		t.Fatalf("a refused install must open no shell and run nothing: %v", s.calls)
	}
}

func TestExecDebugOnAContainerWithAShellKeepsTheServersMessage(t *testing.T) {
	fakeTerminal(t, "")
	s := newDebugServer(t)
	s.install = func(w http.ResponseWriter) {
		fail(w, http.StatusConflict, "err.container.has_shell", "This container already has a shell; open the ordinary terminal instead")
	}
	code, _, stderr := runShellCLI(t, s.URL, "container", "exec", "svc", "--debug")
	if code != ExitConflict || !strings.Contains(stderr, "already has a shell") {
		t.Fatalf("exit %d, stderr %q", code, stderr)
	}
}

func TestExecDebugRemovesTheInstallWhenTheSocketIsRefused(t *testing.T) {
	fakeTerminal(t, "")
	s := newDebugServer(t)
	s.refuseWS = http.StatusForbidden
	code, _, stderr := runShellCLI(t, s.URL, "container", "exec", "svc", "--debug")
	if code != ExitForbidden || !strings.Contains(stderr, "containers.debug_shell") {
		t.Fatalf("exit %d, stderr %q", code, stderr)
	}
	if removed := s.seen("DELETE /api/containers/svc/debug-shell"); len(removed) != 1 {
		t.Fatalf("an install whose shell never opened must be removed at once: %v", removed)
	}
}

func TestExecDebugRejectsAShellOverride(t *testing.T) {
	s := newDebugServer(t)
	code, _, stderr := runShellCLI(t, s.URL, "container", "exec", "svc", "--debug", "--shell", "/bin/bash")
	if code != ExitUsage || !strings.Contains(stderr, "--shell") {
		t.Fatalf("exit %d, stderr %q", code, stderr)
	}
	if len(s.seen("")) != 0 {
		t.Fatalf("a usage error must send nothing: %v", s.calls)
	}
}
