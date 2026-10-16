// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package cli

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync"
	"testing"
)

// stackEnvServer answers the stack env routes the way McHarbor does: credential-looking values
// masked unless reveal=true, and the whole set replaced on PUT.
type stackEnvServer struct {
	*fakeServer
	mu      sync.Mutex
	reveals []string
	envs    []string
	puts    []map[string]string
	denyRev bool
}

func newStackEnvServer(t *testing.T) *stackEnvServer {
	t.Helper()
	s := &stackEnvServer{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/api/health":
			ok(w, map[string]string{"status": "ok"})
		case r.URL.Path == "/api/about":
			ok(w, map[string]any{"version": "2026.09.01", "build": "202609250000"})
		case r.URL.Path == "/api/stacks/web/env-vars" && r.Method == http.MethodGet:
			s.mu.Lock()
			s.reveals = append(s.reveals, r.URL.Query().Get("reveal"))
			s.envs = append(s.envs, r.URL.Query().Get("env"))
			deny := s.denyRev
			s.mu.Unlock()
			if r.URL.Query().Get("reveal") == "true" {
				if deny {
					fail(w, http.StatusForbidden, "err.rbac.permission_denied", "You do not have permission to perform this action")
					return
				}
				ok(w, map[string]string{"DB_HOST": "db", "DB_PASSWORD": "hunter2"})
				return
			}
			ok(w, map[string]string{"DB_HOST": "db", "DB_PASSWORD": stackEnvMask})
		case r.URL.Path == "/api/stacks/web/env-vars" && r.Method == http.MethodPut:
			raw := valueOr(io.ReadAll(r.Body))
			var body map[string]string
			if err := json.Unmarshal(raw, &body); err != nil {
				fail(w, http.StatusBadRequest, "err.invalid_body", "bad body")
				return
			}
			s.mu.Lock()
			s.envs = append(s.envs, r.URL.Query().Get("env"))
			s.puts = append(s.puts, body)
			s.mu.Unlock()
			ok(w, nil)
		default:
			fail(w, http.StatusNotFound, "err.not_found", "no route "+r.URL.Path)
		}
	}))
	t.Cleanup(srv.Close)
	s.fakeServer = &fakeServer{Server: srv}
	return s
}

func TestStackEnvRevealIsOptIn(t *testing.T) {
	s := newStackEnvServer(t)
	for _, args := range [][]string{{"stack", "env", "web"}, {"stack", "env", "web", "--reveal"}} {
		if code, _, stderr := run(t, s.fakeServer, goodKey, args...); code != ExitOK {
			t.Fatalf("%v: exit %d: %s", args, code, stderr)
		}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.reveals) != 2 || s.reveals[0] != "" || s.reveals[1] != "true" {
		t.Fatalf("reveal query per call = %q, want [\"\" \"true\"]", s.reveals)
	}
}

func TestStackEnvShowsMaskedValuesByDefault(t *testing.T) {
	s := newStackEnvServer(t)
	for _, format := range []string{"table", "json"} {
		code, stdout, stderr := run(t, s.fakeServer, goodKey, "-o", format, "stack", "env", "web")
		if code != ExitOK {
			t.Fatalf("-o %s: exit %d: %s", format, code, stderr)
		}
		if strings.Contains(stdout, "hunter2") || !strings.Contains(stdout, stackEnvMask) {
			t.Fatalf("-o %s printed the wrong values: %s", format, stdout)
		}
	}
	_, stdout, _ := run(t, s.fakeServer, goodKey, "-o", "json", "stack", "env", "web", "--reveal")
	var got map[string]string
	if err := json.Unmarshal([]byte(stdout), &got); err != nil || got["DB_PASSWORD"] != "hunter2" {
		t.Fatalf("--reveal json = %s (%v), want the clear value in a key/value object", stdout, err)
	}
}

func TestStackEnvScopesToEnvironment(t *testing.T) {
	s := newStackEnvServer(t)
	if code, _, stderr := run(t, s.fakeServer, goodKey, "--env", "env42", "stack", "env", "web"); code != ExitOK {
		t.Fatalf("exit %d: %s", code, stderr)
	}
	if code, _, stderr := run(t, s.fakeServer, goodKey, "--env", "env42", "stack", "env", "set", "web", "A=1"); code != ExitOK {
		t.Fatalf("exit %d: %s", code, stderr)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.envs) != 3 {
		t.Fatalf("expected a GET, a GET and a PUT, saw envs %q", s.envs)
	}
	for _, e := range s.envs {
		if e != "env42" {
			t.Fatalf("a call went out with env=%q, want env42 on every call: %q", e, s.envs)
		}
	}
}

func TestStackEnvRevealRefusalNamesManage(t *testing.T) {
	s := newStackEnvServer(t)
	s.denyRev = true
	code, _, stderr := run(t, s.fakeServer, goodKey, "stack", "env", "web", "--reveal")
	if code != ExitForbidden || !strings.Contains(stderr, "stacks.manage") {
		t.Fatalf("exit %d, stderr %q: want a forbidden exit naming stacks.manage, not the view key", code, stderr)
	}
}

func TestStackEnvSetSendsWholeSetKeepingMasks(t *testing.T) {
	s := newStackEnvServer(t)
	code, _, stderr := run(t, s.fakeServer, goodKey, "stack", "env", "set", "web", "LOG=debug", "URL=http://x?a=b")
	if code != ExitOK {
		t.Fatalf("exit %d: %s", code, stderr)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	want := map[string]string{"DB_HOST": "db", "DB_PASSWORD": stackEnvMask, "LOG": "debug", "URL": "http://x?a=b"}
	if len(s.puts) != 1 || !reflect.DeepEqual(s.puts[0], want) {
		t.Fatalf("PUT bodies = %v, want one body %v", s.puts, want)
	}
	for _, r := range s.reveals {
		if r != "" {
			t.Fatalf("set asked the server to reveal (reveal=%q); it only needs the masked view", r)
		}
	}
}

func TestStackEnvSetRefusesBadInputWithoutARequest(t *testing.T) {
	s := newStackEnvServer(t)
	for _, arg := range []string{"NOEQUALS", "1BAD=x", "BAD KEY=x", "DB_PASSWORD=" + stackEnvMask} {
		if code, _, _ := run(t, s.fakeServer, goodKey, "stack", "env", "set", "web", arg); code != ExitUsage {
			t.Fatalf("%q: exit %d, want a usage error", arg, code)
		}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.puts) != 0 || len(s.reveals) != 0 {
		t.Fatalf("invalid input still reached the server: puts %v, gets %v", s.puts, s.reveals)
	}
}

func TestStackEnvUnsetNeedsConfirmationAndRemovesOnlyTheKey(t *testing.T) {
	s := newStackEnvServer(t)
	if code, _, _ := run(t, s.fakeServer, goodKey, "stack", "env", "unset", "web", "DB_HOST"); code != ExitUsage {
		t.Fatalf("unset without --yes: exit %d, want a refusal", code)
	}
	s.mu.Lock()
	if len(s.puts) != 0 {
		s.mu.Unlock()
		t.Fatalf("an unconfirmed unset wrote: %v", s.puts)
	}
	s.mu.Unlock()

	if code, _, stderr := run(t, s.fakeServer, goodKey, "--yes", "stack", "env", "unset", "web", "DB_HOST"); code != ExitOK {
		t.Fatalf("exit %d: %s", code, stderr)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	want := map[string]string{"DB_PASSWORD": stackEnvMask}
	if len(s.puts) != 1 || !reflect.DeepEqual(s.puts[0], want) {
		t.Fatalf("PUT bodies = %v, want one body %v", s.puts, want)
	}
}

func TestStackEnvUnsetMissingKeyIsNotFound(t *testing.T) {
	s := newStackEnvServer(t)
	code, _, stderr := run(t, s.fakeServer, goodKey, "--yes", "stack", "env", "unset", "web", "NOPE")
	if code != ExitNotFound {
		t.Fatalf("exit %d (%s), want not found", code, stderr)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.puts) != 0 {
		t.Fatalf("unsetting a missing key still wrote: %v", s.puts)
	}
}
