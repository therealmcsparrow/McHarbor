// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"

	"github.com/therealmcsparrow/mcharbor-agent/internal/errlog"
)

func quietLogger() *slog.Logger { return slog.New(slog.NewTextHandler(io.Discard, nil)) }

// redirectDocker sends the proxy's "http://docker/..." calls to a test server.
type redirectDocker struct{ target *url.URL }

func (r redirectDocker) RoundTrip(req *http.Request) (*http.Response, error) {
	req = req.Clone(req.Context())
	req.URL.Scheme = r.target.Scheme
	req.URL.Host = r.target.Host
	return http.DefaultTransport.RoundTrip(req)
}

// fakeDocker records every daemon call; a test fails if it sees a registry pull.
type fakeDocker struct {
	mu       sync.Mutex
	calls    []string
	loaded   []byte
	created  string
	loadedOK bool
}

func (f *fakeDocker) handler(t *testing.T) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		f.calls = append(f.calls, r.Method+" "+r.URL.Path)
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/images/create":
			t.Errorf("the agent pulled from a registry: %s", r.URL.String())
			http.Error(w, "no registry here", http.StatusInternalServerError)
		case r.Method == http.MethodPost && r.URL.Path == "/images/load":
			f.loaded = errlog.ReadAll(r.Body)
			f.loadedOK = true
			if _, err := io.WriteString(w, `{"stream":"Loaded image: mcharbor-agent:2026.09.02\n"}`); err != nil {
				t.Errorf("writing load reply: %v", err)
			}
		case r.Method == http.MethodGet && r.URL.Path == "/images/mcharbor-agent:2026.09.02/json":
			if !f.loadedOK {
				http.Error(w, `{"message":"no such image"}`, http.StatusNotFound)
				return
			}
			if _, err := io.WriteString(w, `{"Id":"sha256:img"}`); err != nil {
				t.Errorf("writing inspect reply: %v", err)
			}
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/rename"):
			w.WriteHeader(http.StatusNoContent)
		case r.Method == http.MethodPost && r.URL.Path == "/containers/create":
			var body struct {
				Image string `json:"Image"`
			}
			errlog.Log(json.NewDecoder(r.Body).Decode(&body))
			f.created = body.Image
			if _, err := io.WriteString(w, `{"Id":"new123"}`); err != nil {
				t.Errorf("writing create reply: %v", err)
			}
		case r.Method == http.MethodPost && r.URL.Path == "/containers/new123/start":
			w.WriteHeader(http.StatusNoContent)
		default:
			http.Error(w, "unexpected "+r.Method+" "+r.URL.Path, http.StatusNotFound)
		}
	})
}

func testAgent(t *testing.T, mcharborURL string, client *http.Client, docker *httptest.Server) *Agent {
	t.Helper()
	a := &Agent{
		cfg:        Config{McHarborURL: mcharborURL, AgentToken: "agt_test"},
		logger:     quietLogger(),
		httpClient: client,
		proxy:      &Proxy{logger: quietLogger(), httpClient: &http.Client{}},
	}
	if docker != nil {
		target, err := url.Parse(docker.URL)
		if err != nil {
			t.Fatal(err)
		}
		a.proxy.httpClient = &http.Client{Transport: redirectDocker{target: target}}
	}
	return a
}

func TestAgentUpdateRefusesAnythingButItsOwnMcHarbor(t *testing.T) {
	a := testAgent(t, "wss://mcharbor.example:8705", http.DefaultClient, nil)
	ctx := context.Background()
	for _, u := range []string{
		"https://github.com/therealmcsparrow/mcharbor-agent/releases/download/v2026.09.02/mcharbor-agent-linux-amd64",
		"https://ghcr.io/v2/therealmcsparrow/mcharbor-agent/blobs/sha256:abc",
		"https://mcharbor.example.attacker.net/api/agent/binary/linux/amd64",
	} {
		if _, err := a.updateBinary(ctx, u, ""); err == nil || !strings.Contains(err.Error(), "only from the McHarbor") {
			t.Errorf("binary from %s: %v", u, err)
		}
		if _, err := a.loadImageFromMcHarbor(ctx, AgentUpdatePayload{ImageURL: u, ImageRef: "mcharbor-agent:x"}); err == nil || !strings.Contains(err.Error(), "only from the McHarbor") {
			t.Errorf("image from %s: %v", u, err)
		}
	}
	if _, err := a.updateBinary(ctx, "", ""); err == nil || !strings.Contains(err.Error(), "Settings > Updates > Agents") {
		t.Errorf("an empty binary URL must be refused with the fix, not sent to GitHub: %v", err)
	}
	if _, err := a.loadImageFromMcHarbor(ctx, AgentUpdatePayload{Image: "ghcr.io/therealmcsparrow/mcharbor-agent:2026.09.02"}); err == nil || !strings.Contains(err.Error(), "Settings > Updates > Agents") {
		t.Errorf("a registry image must not be pulled: %v", err)
	}
}

func TestContainerUpdateLoadsTheTarballMcHarborServes(t *testing.T) {
	tarball := []byte("pretend this is a docker save tarball")
	sum := sha256.Sum256(tarball)
	var auth string
	mcharbor := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/agent/image/linux/amd64" {
			http.NotFound(w, r)
			return
		}
		auth = r.Header.Get("Authorization")
		if _, err := w.Write(tarball); err != nil {
			t.Errorf("writing tarball: %v", err)
		}
	}))
	defer mcharbor.Close()
	fd := &fakeDocker{}
	docker := httptest.NewServer(fd.handler(t))
	defer docker.Close()

	a := testAgent(t, mcharbor.URL, mcharbor.Client(), docker)
	self := rawContainer{ID: "old123", Name: "/mcharbor-agent", Config: map[string]json.RawMessage{"Image": json.RawMessage(`"ghcr.io/therealmcsparrow/mcharbor-agent:2026.09.01"`)}, HostConfig: json.RawMessage(`{}`)}
	payload := AgentUpdatePayload{
		Version: "2026.09.02", ImageURL: mcharbor.URL + "/api/agent/image/linux/amd64",
		ImageRef: "mcharbor-agent:2026.09.02", ImageSHA256: hex.EncodeToString(sum[:]),
	}
	msg, err := a.updateContainer(context.Background(), self, payload)
	if err != nil {
		t.Fatalf("update: %v (docker calls %v)", err, fd.calls)
	}
	if auth != "Bearer agt_test" {
		t.Errorf("the image download did not carry the agent token: %q", auth)
	}
	if string(fd.loaded) != string(tarball) {
		t.Errorf("docker load received %q", fd.loaded)
	}
	if fd.created != "mcharbor-agent:2026.09.02" {
		t.Errorf("replacement created from %q, want the loaded image", fd.created)
	}
	if !strings.Contains(msg, "mcharbor-agent:2026.09.02") {
		t.Errorf("message = %q", msg)
	}

	fd2 := &fakeDocker{}
	docker2 := httptest.NewServer(fd2.handler(t))
	defer docker2.Close()
	a2 := testAgent(t, mcharbor.URL, mcharbor.Client(), docker2)
	payload.ImageSHA256 = strings.Repeat("0", 64)
	if _, err := a2.updateContainer(context.Background(), self, payload); err == nil || !strings.Contains(err.Error(), "checksum") {
		t.Fatalf("a tarball that does not match its checksum must be refused: %v", err)
	}
	if fd2.loadedOK || len(fd2.calls) != 0 {
		t.Fatalf("docker was touched after a checksum failure: %v", fd2.calls)
	}
}
