// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package cli

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

func TestExecRunsOnceAndExitsWithTheCommandsStatus(t *testing.T) {
	status := 3
	f := groupServer(t, map[string]func(http.ResponseWriter, string){
		"POST /api/containers/web/exec": func(w http.ResponseWriter, _ string) {
			ok(w, map[string]any{"stdout": "hi\n", "stderr": "warn\n", "exitCode": status, "durationMs": 4})
		},
	})
	code, out, stderr := run(t, f, goodKey, "container", "exec", "web", "--user", "root", "--", "ls", "-la", "/")
	if code != 3 || out != "hi\n" || !strings.Contains(stderr, "warn") || strings.Contains(stderr, "Error:") {
		t.Fatalf("exit %d, out %q, stderr %q", code, out, stderr)
	}
	f.mu.Lock()
	body := f.bodies["POST /api/containers/web/exec"]
	f.mu.Unlock()
	if !strings.Contains(body, `"command":["ls","-la","/"]`) || !strings.Contains(body, `"user":"root"`) {
		t.Fatalf("sent %s", body)
	}

	status = 0
	if code, _, stderr := run(t, f, goodKey, "container", "exec", "web", "--", "true"); code != ExitOK {
		t.Fatalf("a zero status: exit %d, %s", code, stderr)
	}

	before := len(f.seen())
	if code, _, stderr := run(t, f, goodKey, "container", "exec", "web", "ls"); code != ExitUsage || !strings.Contains(stderr, "after --") {
		t.Fatalf("a command without --: exit %d, %q", code, stderr)
	}
	if got := len(f.seen()) - before; got != 0 {
		t.Fatalf("a usage error sent %d requests", got)
	}
}

type shellServer struct {
	*httptest.Server
	mu      sync.Mutex
	header  http.Header
	query   string
	frames  []terminalFrame
	handle  func(c *websocket.Conn)
	refuse  int
	gotBoth chan struct{}
}

func newShellServer(t *testing.T, handle func(c *websocket.Conn)) *shellServer {
	t.Helper()
	s := &shellServer{handle: handle, gotBoth: make(chan struct{})}
	up := websocket.Upgrader{CheckOrigin: func(r *http.Request) bool { return r.Header.Get("Origin") == s.URL }}
	s.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/terminal/ws" {
			http.NotFound(w, r)
			return
		}
		s.mu.Lock()
		s.header, s.query = r.Header.Clone(), r.URL.RawQuery
		s.mu.Unlock()
		if s.refuse != 0 {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(s.refuse)
			if _, err := io.WriteString(w, `{"success":false,"error":"denied","code":"err.rbac.permission_denied"}`); err != nil {
				t.Error(err)
			}
			return
		}
		c, err := up.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer func() { warnErr(c.Close()) }()
		s.handle(c)
	}))
	t.Cleanup(s.Close)
	return s
}

// readUntilInputAndResize records client frames until both kinds have arrived.
func (s *shellServer) readUntilInputAndResize(c *websocket.Conn) {
	var input, resize bool
	for !input || !resize {
		var f terminalFrame
		if err := c.ReadJSON(&f); err != nil {
			return
		}
		s.mu.Lock()
		s.frames = append(s.frames, f)
		s.mu.Unlock()
		input = input || f.Type == "input"
		resize = resize || f.Type == "resize"
	}
}

func fakeTerminal(t *testing.T, in string) {
	t.Helper()
	oldIn, oldTTY, oldRaw, oldSize := stdin, stdinIsTerminal, makeRaw, terminalSize
	stdin = strings.NewReader(in)
	stdinIsTerminal = func() bool { return true }
	makeRaw = func() (func(), error) { return func() {}, nil }
	terminalSize = func() (int, int, bool) { return 120, 40, true }
	t.Cleanup(func() { stdin, stdinIsTerminal, makeRaw, terminalSize = oldIn, oldTTY, oldRaw, oldSize })
}

func runShellCLI(t *testing.T, url string, args ...string) (int, string, string) {
	t.Helper()
	t.Setenv("MCHARBOR_CONFIG_DIR", t.TempDir())
	t.Setenv("MCHARBOR_NO_KEYRING", "1")
	t.Setenv("MCHARBOR_CONTEXT", "")
	t.Setenv("MCHARBOR_ENV", "prod")
	t.Setenv("MCHARBOR_URL", url)
	t.Setenv("MCHARBOR_API_KEY", goodKey)
	var out, errOut strings.Builder
	code := Execute(args, &out, &errOut)
	return code, out.String(), errOut.String()
}

func TestShellPumpsKeysAndSizeAndPrintsTheShell(t *testing.T) {
	fakeTerminal(t, "ls\n")
	var s *shellServer
	s = newShellServer(t, func(c *websocket.Conn) {
		if c.WriteMessage(websocket.TextMessage, []byte("McHarbor shell access\r\n")) != nil {
			return
		}
		s.readUntilInputAndResize(c)
		if c.WriteMessage(websocket.BinaryMessage, []byte("bin etc usr\r\n")) != nil {
			return
		}
		if c.WriteMessage(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, "")) != nil {
			return
		}
		time.Sleep(50 * time.Millisecond)
	})

	code, out, stderr := runShellCLI(t, s.URL, "container", "exec", "web", "--shell", "/bin/bash", "-w", "/srv")
	if code != ExitOK || out != "McHarbor shell access\r\nbin etc usr\r\n" {
		t.Fatalf("exit %d, out %q, stderr %q", code, out, stderr)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.header.Get("Authorization") != "Bearer "+goodKey {
		t.Fatalf("the key was not sent as a bearer header")
	}
	for _, want := range []string{"container=web", "shell=%2Fbin%2Fbash", "workDir=%2Fsrv", "env=prod"} {
		if !strings.Contains(s.query, want) {
			t.Fatalf("query %q lacks %s", s.query, want)
		}
	}
	var typed string
	var sized bool
	for _, f := range s.frames {
		switch f.Type {
		case "input":
			typed += f.Data
		case "resize":
			sized = f.Cols == 120 && f.Rows == 40
		}
	}
	if typed != "ls\n" || !sized {
		raw := valueOr(json.Marshal(s.frames))
		t.Fatalf("frames %s", raw)
	}
}

func TestShellReportsTheServersErrorAndDenial(t *testing.T) {
	fakeTerminal(t, "")
	s := newShellServer(t, func(c *websocket.Conn) {
		if c.WriteMessage(websocket.TextMessage, []byte(`{"type":"error","data":"failed to create exec session"}`)) != nil {
			return
		}
		time.Sleep(50 * time.Millisecond)
	})
	code, _, stderr := runShellCLI(t, s.URL, "container", "exec", "web")
	if code != ExitRemote || !strings.Contains(stderr, "failed to create exec session") {
		t.Fatalf("an error frame: exit %d, %q", code, stderr)
	}

	s.refuse = http.StatusForbidden
	code, _, stderr = runShellCLI(t, s.URL, "container", "exec", "web")
	if code != ExitForbidden || !strings.Contains(stderr, "terminal.access or containers.manage") {
		t.Fatalf("a refused upgrade: exit %d, %q", code, stderr)
	}

	stdinIsTerminal = func() bool { return false }
	s.refuse = 0
	if code, _, stderr := runShellCLI(t, s.URL, "container", "exec", "web"); code != ExitUsage || !strings.Contains(stderr, "needs a terminal") {
		t.Fatalf("no terminal: exit %d, %q", code, stderr)
	}
}
