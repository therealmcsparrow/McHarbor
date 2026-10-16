// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package nodelink

import (
	"archive/tar"
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/tls"
	"encoding/hex"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/rs/xid"

	"github.com/therealmcsparrow/mcharbor/core/agent"
	"github.com/therealmcsparrow/mcharbor/core/httpx"
)

// bodyDigestAgent answers every proxied Docker call with the SHA-256 and length of the body
// it received, buffered or streamed, so a test can tell whether the body arrived whole.
func bodyDigestAgent() func(fa *fakeAgent, msg agent.WSMessage) bool {
	var mu sync.Mutex
	bodies := map[string]*bytes.Buffer{}
	reply := func(fa *fakeAgent, id string, body []byte) {
		sum := sha256.Sum256(body)
		fa.send(agent.WSMessage{Type: agent.MsgHTTPResponse, ID: id, HTTPResponse: &agent.WSHTTPResponse{
			StatusCode: http.StatusOK, Body: []byte(hex.EncodeToString(sum[:]) + " " + strconv.Itoa(len(body))),
		}})
	}
	return func(fa *fakeAgent, msg agent.WSMessage) bool {
		switch msg.Type {
		case agent.MsgHTTPRequest:
			reply(fa, msg.ID, msg.HTTPRequest.Body)
		case agent.MsgHTTPRequestStart:
			mu.Lock()
			bodies[msg.ID] = &bytes.Buffer{}
			mu.Unlock()
		case agent.MsgHTTPRequestChunk:
			mu.Lock()
			bodies[msg.ID].Write(msg.StreamChunk.Data)
			mu.Unlock()
		case agent.MsgHTTPRequestEnd:
			mu.Lock()
			body := bodies[msg.ID].Bytes()
			delete(bodies, msg.ID)
			mu.Unlock()
			reply(fa, msg.ID, body)
		default:
			return false
		}
		return true
	}
}

// tarOf builds a tar holding one file of size random bytes.
func tarOf(t *testing.T, size int) []byte {
	t.Helper()
	content := make([]byte, size)
	if _, err := rand.Read(content); err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)
	if err := tw.WriteHeader(&tar.Header{Name: "payload.bin", Mode: 0o644, Size: int64(size), Typeflag: tar.TypeReg}); err != nil {
		t.Fatal(err)
	}
	if _, err := tw.Write(content); err != nil {
		t.Fatal(err)
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

// TestForwardedUnknownLengthTarArrivesWhole: the Docker SDK sends CopyToContainer and
// ImageBuild bodies with ContentLength 0 and a body. Such a tar, larger than a signed body,
// must cross the hop streamed and reach the agent byte for byte, not truncated and refused.
func TestForwardedUnknownLengthTarArrivesWhole(t *testing.T) {
	c := newClusterWith(t, bodyDigestAgent())
	tarball := tarOf(t, 3<<20)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	// A reader NewRequest cannot size, as the SDK's tar stream is: ContentLength stays 0.
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, "http://agent/v1.43/containers/c1/archive?path=%2Ftmp",
		io.MultiReader(bytes.NewReader(tarball)))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/x-tar")
	if req.ContentLength != 0 || req.Body == nil {
		t.Fatalf("the fixture must be an unknown-length body, got length %d", req.ContentLength)
	}

	resp, err := c.serving.Transport("env-1").RoundTrip(req)
	if err != nil {
		t.Fatalf("the forwarded tar must reach the agent: %v", err)
	}
	defer closeIgnoring(resp.Body)
	got, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(tarball)
	if want := hex.EncodeToString(sum[:]) + " " + strconv.Itoa(len(tarball)); resp.StatusCode != http.StatusOK || string(got) != want {
		t.Fatalf("the agent must receive the tar byte for byte: status %d, agent saw %q, sent %q", resp.StatusCode, got, want)
	}
}

// TestSignableBodyNeverSignsPartOfABody: a held body is exactly its declared length or the
// call fails; a body of unknown length is streamed rather than hashed.
func TestSignableBodyNeverSignsPartOfABody(t *testing.T) {
	long := bytes.Repeat([]byte("x"), 2<<20)
	lying := httptest.NewRequest(http.MethodPost, "http://agent/build", bytes.NewReader(long))
	lying.ContentLength = 10
	if _, _, err := signableBody(lying); err == nil {
		t.Fatal("a body longer than its declared length must not be signed in part")
	}
	unknown := httptest.NewRequest(http.MethodPost, "http://agent/build", io.MultiReader(bytes.NewReader(long)))
	unknown.ContentLength = 0
	if _, digest, err := signableBody(unknown); err != nil || digest != streamDigest {
		t.Fatalf("an unknown-length body must be streamed, got digest %q, %v", digest, err)
	}
	if _, digest, err := signableBody(httptest.NewRequest(http.MethodGet, "http://agent/_ping", http.NoBody)); err != nil || digest != emptyDigest() {
		t.Fatalf("no body signs the empty digest, got %q, %v", digest, err)
	}
}

// TestUnknownNodeIsRefusedDespiteAValidSignature: a correctly signed call naming a node that
// is not a live instance is refused before its body or the trigger service is touched, and a
// node whose registry is not wired yet refuses as retryable rather than admitting it.
func TestUnknownNodeIsRefusedDespiteAValidSignature(t *testing.T) {
	receiver := &stubReceiver{}
	receiver.leader.Store(true)
	link, _ := fireNode(t, "node-a", receiver)
	body := []byte(`{"fireId":"f1","actionKey":"plugin:addon-demo:x","source":"addon-demo"}`)

	stranger := Claims{Hop: HopWorkflowTrigger, NodeID: "node-z", Audience: "node-a", Method: http.MethodPost,
		Path: triggerFireClaimPath, ActorID: "addon-demo"}
	req, rec := requestSignedBy(t, NewSigner("shared-secret"), triggerFireRoute, stranger, body)
	w := httptest.NewRecorder()
	link.ServeTriggerFire(w, req)
	if w.Code != http.StatusForbidden || rec.reads.Load() != 0 {
		t.Fatalf("fire from an unknown node: want 403 with no body read, got %d after %d reads", w.Code, rec.reads.Load())
	}
	rpc := Claims{Hop: HopAgentRPC, NodeID: "node-z", Audience: "node-a", EnvID: "env-1", Method: http.MethodPost, Path: "/v1.43/containers/create"}
	req, _ = requestSignedBy(t, NewSigner("shared-secret"), "/api/internal/node/agent-rpc/env-1", rpc, body)
	w = httptest.NewRecorder()
	link.ServeAgentRPC(w, req, "env-1")
	if w.Code != http.StatusForbidden {
		t.Fatalf("agent-rpc from an unknown node: want 403, got %d", w.Code)
	}
	if n := receiver.calls.Load(); n != 0 {
		t.Fatalf("the trigger service ran for an unknown node (%d calls)", n)
	}

	cold := New(NewOwners(migrated(t), "node-a", "https://node-a:8705"), NewSigner("shared-secret"), noAgents{}, quiet())
	cold.SetRequireTLS(false)
	cold.SetTriggerFireReceiver(receiver)
	known := stranger
	known.NodeID = "node-b"
	req, _ = requestSignedBy(t, NewSigner("shared-secret"), triggerFireRoute, known, body)
	w = httptest.NewRecorder()
	cold.ServeTriggerFire(w, req)
	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("a node with no registry yet must refuse as retryable, got %d", w.Code)
	}
}

// TestWhitespaceValuesVerifyAndInvalidValuesAreRefused: a Go server trims header values, so
// an actor name or inner header with surrounding spaces must still verify after a real HTTP
// hop; a value HTTP cannot carry is refused when signing, on the request and on the reply.
func TestWhitespaceValuesVerifyAndInvalidValuesAreRefused(t *testing.T) {
	s := NewSigner("shared-secret")
	var verifyErr error
	var seen Claims
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		ignoreErr(err)
		seen, verifyErr = s.Verify(r, HopAgentRPC, "env-1", "node-a", body)
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	req, err := http.NewRequest(http.MethodPost, srv.URL+"/api/internal/node/agent-rpc/env-1", http.NoBody)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set(HeaderInnerPrefix+"X-Test", "  padded value\t")
	claims := Claims{Hop: HopAgentRPC, NodeID: "node-b", Audience: "node-a", EnvID: "env-1", Method: http.MethodGet,
		Path: "/v1.43/containers/json", ActorID: "user-1", ActorName: " ada lovelace "}
	mustSign(t, s, req, claims, xid.New().String(), emptyDigest())
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	ignoreErr(resp.Body.Close())
	if verifyErr != nil {
		t.Fatalf("values with surrounding whitespace must verify after the hop: %v", verifyErr)
	}
	if seen.ActorName != "ada lovelace" {
		t.Fatalf("the verified actor name must be the trimmed one, got %q", seen.ActorName)
	}

	bad := map[string]func(*http.Request, *Claims){
		"actor name with CRLF": func(_ *http.Request, c *Claims) { c.ActorName = "ada\r\nX-Evil: 1" },
		"inner header with NUL": func(r *http.Request, _ *Claims) {
			r.Header.Set(HeaderInnerPrefix+"X-Registry-Auth", "abc\x00def")
		},
	}
	for name, mutate := range bad {
		r := httptest.NewRequest(http.MethodPost, "/api/internal/node/agent-rpc/env-1", nil)
		c := claims
		mutate(r, &c)
		if err := s.Sign(r, c, xid.New().String(), emptyDigest()); !errors.Is(err, ErrUnsignable) {
			t.Fatalf("%s: signing must be refused, got %v", name, err)
		}
		if r.Header.Get(HeaderSignature) != "" {
			t.Fatalf("%s: a refused request must carry no signature", name)
		}
	}
	reply := http.Header{}
	reply.Set(HeaderInnerPrefix+"Content-Type", "text/plain\nX-Evil: 1")
	if _, err := s.SignReplyHead("node-b", "node-a", "n1", reply); !errors.Is(err, ErrUnsignable) {
		t.Fatalf("a reply head with a newline must be refused, got %v", err)
	}
}

// TestAcceptEmptyRefusesANonEmptyDigest: a body-less hop accepts only the empty-body digest,
// not another body's digest and not a streamed one, and spends no nonce when it refuses.
func TestAcceptEmptyRefusesANonEmptyDigest(t *testing.T) {
	s := NewSigner("shared-secret")
	build := func(digest string) *http.Request {
		req := httptest.NewRequest(http.MethodGet, "/api/internal/node/stats/env-1", nil)
		mustSign(t, s, req, Claims{Hop: HopStats, NodeID: "node-b", Audience: "node-a", EnvID: "env-1", Method: http.MethodGet,
			Path: statsClaimPath}, xid.New().String(), digest)
		return req
	}
	other := sha256.Sum256([]byte("x"))
	for name, digest := range map[string]string{"another body": hex.EncodeToString(other[:]), "streamed": streamDigest} {
		req := build(digest)
		if _, err := s.Authenticate(req, HopStats, "env-1", "node-a"); err != nil {
			t.Fatalf("%s: the fixture must authenticate: %v", name, err)
		}
		if err := s.AcceptEmpty(req); !errors.Is(err, ErrBadSignature) {
			t.Fatalf("%s: a body-less hop must refuse a non-empty digest, got %v", name, err)
		}
	}
	if err := s.AcceptEmpty(build(emptyDigest())); err != nil {
		t.Fatalf("the empty-body digest must be accepted: %v", err)
	}
}

// v3Sign stamps req the way a build before the hop label did, so the test can show such a
// signature fails closed.
func v3Sign(s *Signer, req *http.Request, c Claims, nonce, digest string) {
	timestamp := strconv.FormatInt(time.Now().UTC().Unix(), 10)
	req.Header.Set(HeaderNode, c.NodeID)
	req.Header.Set(HeaderAudience, c.Audience)
	req.Header.Set(HeaderMethod, c.Method)
	req.Header.Set(HeaderPath, c.Path)
	req.Header.Set(HeaderQuery, c.Query)
	req.Header.Set(HeaderActorID, c.ActorID)
	req.Header.Set(HeaderActorName, c.ActorName)
	req.Header.Set(HeaderTimestamp, timestamp)
	req.Header.Set(HeaderNonce, nonce)
	req.Header.Set(HeaderBodyDigest, digest)
	req.Header.Set(HeaderSignature, s.mac(strings.Join([]string{
		"mcharbor-node-link-v3", c.NodeID, c.Audience, c.EnvID, c.Method, c.Path, c.Query,
		timestamp, nonce, c.ActorID, c.ActorName, "", digest, innerHeaderDigest(req.Header),
	}, "\n")))
}

// TestCrossHopSignatureIsRefused: a signature made for one hop never verifies on another,
// agent-rpc refuses the other hops' claim paths even when signed for agent-rpc, and a v3
// signature (no hop label) fails closed.
func TestCrossHopSignatureIsRefused(t *testing.T) {
	s := NewSigner("shared-secret")
	rpcShaped := Claims{NodeID: "node-b", Audience: "node-a", EnvID: "env-1", Method: http.MethodGet, Path: "/v1.43/containers/json"}
	for _, hop := range []string{HopAgentStream, HopStats, HopWorkflowTrigger} {
		c := rpcShaped
		c.Hop = hop
		req := httptest.NewRequest(http.MethodPost, "/api/internal/node/agent-rpc/env-1", nil)
		mustSign(t, s, req, c, xid.New().String(), emptyDigest())
		if _, err := s.Verify(req, HopAgentRPC, "env-1", "node-a", nil); !errors.Is(err, ErrBadSignature) {
			t.Fatalf("a %s signature must not verify on agent-rpc, got %v", hop, err)
		}
	}

	link, _ := fireNode(t, "node-a", &stubReceiver{})
	for _, path := range []string{statsClaimPath, streamClaimPath, triggerFireClaimPath, triggerLeaderClaimPath} {
		c := rpcShaped
		c.Hop, c.Path = HopAgentRPC, path
		req, _ := requestSignedBy(t, s, "/api/internal/node/agent-rpc/env-1", c, nil)
		w := httptest.NewRecorder()
		link.ServeAgentRPC(w, req, "env-1")
		if w.Code != http.StatusUnauthorized {
			t.Fatalf("agent-rpc must refuse the claim path %s, got %d", path, w.Code)
		}
	}

	old := httptest.NewRequest(http.MethodPost, "/api/internal/node/agent-rpc/env-1", nil)
	v3Sign(s, old, rpcShaped, "nonce-v3", emptyDigest())
	if _, err := s.Verify(old, HopAgentRPC, "env-1", "node-a", nil); !errors.Is(err, ErrBadSignature) {
		t.Fatalf("a v3 signature must be refused, got %v", err)
	}
}

// leaderProbe is a signed, body-less leader probe from node-b to node-a.
func leaderProbe(t *testing.T) *http.Request {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, triggerFireRoute, http.NoBody)
	mustSign(t, NewSigner("shared-secret"), req, Claims{Hop: HopWorkflowTrigger, NodeID: "node-b", Audience: "node-a",
		Method: http.MethodPost, Path: triggerLeaderClaimPath}, xid.New().String(), emptyDigest())
	return req
}

// TestReceiverRefusesPlaintextUnlessOptedIn: a node-to-node request that did not arrive over
// TLS is refused before its signature is checked; X-Forwarded-Proto counts only from a
// trusted proxy; the operator's opt-in accepts plain http.
func TestReceiverRefusesPlaintextUnlessOptedIn(t *testing.T) {
	t.Setenv(EnvAllowHTTP, "")
	receiver := &stubReceiver{}
	receiver.leader.Store(true)
	link, _ := fireNode(t, "node-a", receiver)
	link.SetRequireTLS(true)
	status := func(req *http.Request) int {
		w := httptest.NewRecorder()
		link.ServeTriggerFire(w, req)
		return w.Code
	}

	if got := status(leaderProbe(t)); got != http.StatusForbidden {
		t.Fatalf("a plaintext node call must be refused 403, got %d", got)
	}
	if n := receiver.calls.Load(); n != 0 {
		t.Fatalf("the trigger service ran for a plaintext call (%d calls)", n)
	}
	spoofed := leaderProbe(t)
	spoofed.RemoteAddr = "198.51.100.7:4000"
	spoofed.Header.Set("X-Forwarded-Proto", "https")
	if got := status(spoofed); got != http.StatusForbidden {
		t.Fatalf("X-Forwarded-Proto from an untrusted peer must not count, got %d", got)
	}

	direct := leaderProbe(t)
	direct.TLS = &tls.ConnectionState{}
	if got := status(direct); got != http.StatusOK {
		t.Fatalf("a call over TLS must be served, got %d", got)
	}

	resolver, err := httpx.NewClientAddressResolver([]string{"192.0.2.10"})
	if err != nil {
		t.Fatal(err)
	}
	httpx.SetDefaultResolver(resolver)
	t.Cleanup(func() { httpx.SetDefaultResolver(nil) })
	proxied := leaderProbe(t)
	proxied.RemoteAddr = "192.0.2.10:4000"
	proxied.Header.Set("X-Forwarded-Proto", "https")
	if got := status(proxied); got != http.StatusOK {
		t.Fatalf("a trusted proxy's X-Forwarded-Proto: https must count as TLS, got %d", got)
	}

	t.Setenv(EnvAllowHTTP, "1")
	if got := status(leaderProbe(t)); got != http.StatusOK {
		t.Fatalf("with the opt-in a plaintext call is served, got %d", got)
	}
}

// TestNonHTTPSSchemeIsPlaintext: anything that is not an https URL counts as plaintext, not
// only http://; an empty address is no address rather than a plaintext one.
func TestNonHTTPSSchemeIsPlaintext(t *testing.T) {
	cases := []struct {
		advertise string
		want      bool
	}{
		{"https://node-a:8705", false}, {"  HTTPS://node-a:8705	", false},
		{"http://node-a:8705", true}, {"ftp://node-a", true}, {"ws://node-a:8705", true}, {"node-a:8705", true},
		{"//node-a:8705", true}, {"", false},
	}
	for _, tc := range cases {
		if got := IsPlaintextURL(tc.advertise); got != tc.want {
			t.Errorf("IsPlaintextURL(%q) = %v, want %v", tc.advertise, got, tc.want)
		}
	}
	link := New(NewOwners(nil, "node-b", "https://node-b:8705"), NewSigner("s"), noAgents{}, quiet())
	t.Setenv(EnvAllowHTTP, "")
	if err := link.checkPeerURL("node-a", "ftp://node-a"); !errors.Is(err, ErrPlaintextPeer) {
		t.Fatalf("a non-https peer must be refused, got %v", err)
	}
}
