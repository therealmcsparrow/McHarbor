// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package nodelink

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/rs/xid"
)

// TestPeerModeRequiresTLSByDefault: a link built with no HA mode in mind (the default
// "peer" mode of a multi-node deployment) refuses to send a forwarded Docker call or a
// trigger fire to an http:// peer until the operator opts in.
func TestPeerModeRequiresTLSByDefault(t *testing.T) {
	t.Setenv(EnvAllowHTTP, "")
	peer, hits := countingServer(t)
	database := migrated(t)
	link := New(NewOwners(database, "node-b", "https://node-b:8705"), NewSigner("shared-secret"), noAgents{}, quiet())
	claimFor(t, database, "env-1", "node-a", peer.URL)

	if err := roundTripErr(link.Transport("env-1"), bodyRequest([]byte(`{"Image":"nginx"}`))); !errors.Is(err, ErrPlaintextPeer) {
		t.Fatalf("a forwarded Docker call to an http owner must be refused by default, got %v", err)
	}
	link.SetPeers(func(context.Context) ([]Peer, error) {
		return []Peer{{NodeID: "node-a", AdvertiseURL: peer.URL}}, nil
	})
	fire := TriggerFire{FireID: "f1", ActionKey: "plugin:addon-demo:x", Source: "addon-demo"}
	if _, err := link.ForwardTriggerFire(context.Background(), fire); !errors.Is(err, ErrPlaintextPeer) {
		t.Fatalf("a trigger fire to an http peer must be refused by default, got %v", err)
	}
	if n := hits.Load(); n != 0 {
		t.Fatalf("the http peer received %d signed requests; it must receive none", n)
	}
}

// requestSignedBy builds a request to path whose body counts reads, signed by s.
func requestSignedBy(t *testing.T, s *Signer, path string, claims Claims, body []byte) (*http.Request, *recordingBody) {
	t.Helper()
	rec := &recordingBody{r: bytes.NewReader(body)}
	req := httptest.NewRequest(http.MethodPost, path, nil)
	req.Body = rec
	req.ContentLength = int64(len(body))
	sum := sha256.Sum256(body)
	mustSign(t, s, req, claims, xid.New().String(), hex.EncodeToString(sum[:]))
	return req, rec
}

// TestWrongSignatureIsRefusedBeforeTheBodyIsRead: a request with every header a peer would
// send, a plausible length and a signature made with the wrong key is refused on the HMAC
// alone; not one byte of its (large) body is read.
func TestWrongSignatureIsRefusedBeforeTheBodyIsRead(t *testing.T) {
	receiver := &stubReceiver{}
	receiver.leader.Store(true)
	link, _ := fireNode(t, "node-a", receiver)
	forger := NewSigner("not-the-shared-secret")
	body := bytes.Repeat([]byte("x"), 512<<10)

	fireClaims := Claims{Hop: HopWorkflowTrigger, NodeID: "node-b", Audience: "node-a", Method: http.MethodPost, Path: triggerFireClaimPath, ActorID: "addon-demo"}
	req, rec := requestSignedBy(t, forger, triggerFireRoute, fireClaims, body)
	w := httptest.NewRecorder()
	link.ServeTriggerFire(w, req)
	if w.Code != http.StatusUnauthorized || rec.reads.Load() != 0 {
		t.Fatalf("fire: want 401 with no body read, got %d after %d reads", w.Code, rec.reads.Load())
	}

	rpcClaims := Claims{Hop: HopAgentRPC, NodeID: "node-b", Audience: "node-a", EnvID: "env-1", Method: http.MethodPost, Path: "/v1.43/containers/create"}
	req, rec = requestSignedBy(t, forger, "/api/internal/node/agent-rpc/env-1", rpcClaims, body)
	w = httptest.NewRecorder()
	link.ServeAgentRPC(w, req, "env-1")
	if w.Code != http.StatusUnauthorized || rec.reads.Load() != 0 {
		t.Fatalf("agent-rpc: want 401 with no body read, got %d after %d reads", w.Code, rec.reads.Load())
	}
	if n := receiver.calls.Load(); n != 0 {
		t.Fatalf("the trigger service ran for a forged request (%d calls)", n)
	}
}

// TestDigestMismatchKeepsTheNonce: a correctly signed fire whose body does not hash to the
// signed digest is refused and does not spend its nonce, so the same signed request with
// the right body still goes through.
func TestDigestMismatchKeepsTheNonce(t *testing.T) {
	receiver := &stubReceiver{}
	receiver.leader.Store(true)
	link, _ := fireNode(t, "node-a", receiver)
	good := []byte(`{"fireId":"f1","actionKey":"plugin:addon-demo:x","source":"addon-demo"}`)
	bad := bytes.Repeat([]byte("y"), len(good))
	claims := Claims{Hop: HopWorkflowTrigger, NodeID: "node-b", Audience: "node-a", Method: http.MethodPost, Path: triggerFireClaimPath, ActorID: "addon-demo"}

	signedReq, _ := requestSignedBy(t, NewSigner("shared-secret"), triggerFireRoute, claims, good)
	tampered := httptest.NewRequest(http.MethodPost, triggerFireRoute, bytes.NewReader(bad))
	tampered.Header = signedReq.Header.Clone()
	w := httptest.NewRecorder()
	link.ServeTriggerFire(w, tampered)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("a body that does not match the signed digest must be refused, got %d", w.Code)
	}
	if n := receiver.calls.Load(); n != 0 {
		t.Fatalf("the trigger service ran for a tampered body (%d calls)", n)
	}

	retry := httptest.NewRequest(http.MethodPost, triggerFireRoute, bytes.NewReader(good))
	retry.Header = signedReq.Header.Clone()
	w = httptest.NewRecorder()
	link.ServeTriggerFire(w, retry)
	if w.Code != http.StatusOK {
		t.Fatalf("the refused body must not have spent the nonce; the retry got %d", w.Code)
	}
}

// v2Sign stamps req the way a build before the v3 claims did, so the test can show such a
// signature fails closed.
func v2Sign(s *Signer, req *http.Request, c Claims, nonce, digest string) {
	timestamp := strconv.FormatInt(time.Now().UTC().Unix(), 10)
	req.Header.Set(HeaderNode, c.NodeID)
	req.Header.Set(HeaderAudience, c.Audience)
	req.Header.Set(HeaderMethod, c.Method)
	req.Header.Set(HeaderPath, c.Path)
	req.Header.Set(HeaderQuery, c.Query)
	req.Header.Set(HeaderActorID, c.ActorID)
	req.Header.Set(HeaderTimestamp, timestamp)
	req.Header.Set(HeaderNonce, nonce)
	req.Header.Set(HeaderBodyDigest, digest)
	req.Header.Set(HeaderSignature, s.mac(strings.Join([]string{
		"mcharbor-node-link-v2", c.NodeID, c.Audience, c.EnvID, c.Method, c.Path, c.Query, timestamp, nonce, c.ActorID, digest,
	}, "\n")))
}

// TestSignatureCoversTimeoutActorNameAndInnerHeaders: every field the receiver applies is
// signed, so rewriting the timeout, the actor name or any forwarded inner header (the
// Content-Length the agent request is rebuilt with, X-Registry-Auth) breaks the signature,
// and a v2 signature, which covered none of them, is refused.
func TestSignatureCoversTimeoutActorNameAndInnerHeaders(t *testing.T) {
	s := NewSigner("shared-secret")
	claims := Claims{Hop: HopAgentRPC, NodeID: "node-b", Audience: "node-a", EnvID: "env-1", Method: http.MethodPost, Path: "/v1.43/images/create",
		ActorID: "user-1", ActorName: "ada", Timeout: 20 * time.Second}
	build := func() *http.Request {
		req := httptest.NewRequest(http.MethodPost, "/api/internal/node/agent-rpc/env-1", nil)
		req.Header.Set(HeaderInnerPrefix+"Content-Length", "0")
		req.Header.Set(HeaderInnerPrefix+"X-Registry-Auth", "eyJ1c2VybmFtZSI6ImFkYSJ9")
		mustSign(t, s, req, claims, xid.New().String(), streamDigest)
		return req
	}
	if _, err := s.Verify(build(), HopAgentRPC, "env-1", "node-a", nil); err != nil {
		t.Fatalf("an untouched request must verify: %v", err)
	}
	cases := map[string]func(*http.Request){
		"timeout":              func(r *http.Request) { r.Header.Set(HeaderTimeoutMS, "3600000") },
		"timeout removed":      func(r *http.Request) { r.Header.Del(HeaderTimeoutMS) },
		"actor name":           func(r *http.Request) { r.Header.Set(HeaderActorName, "root") },
		"inner content length": func(r *http.Request) { r.Header.Set(HeaderInnerPrefix+"Content-Length", "1048576") },
		"inner registry auth": func(r *http.Request) {
			r.Header.Set(HeaderInnerPrefix+"X-Registry-Auth", "eyJ1c2VybmFtZSI6Im1hbGxvcnkifQ")
		},
		"inner header added":   func(r *http.Request) { r.Header.Set(HeaderInnerPrefix+"X-Extra", "1") },
		"inner header dropped": func(r *http.Request) { r.Header.Del(HeaderInnerPrefix + "X-Registry-Auth") },
	}
	for name, mutate := range cases {
		req := build()
		mutate(req)
		if _, err := s.Verify(req, HopAgentRPC, "env-1", "node-a", nil); !errors.Is(err, ErrBadSignature) {
			t.Fatalf("%s: a rewritten field must break the signature, got %v", name, err)
		}
	}

	old := httptest.NewRequest(http.MethodPost, "/api/internal/node/agent-rpc/env-1", nil)
	v2Sign(s, old, claims, "nonce-v2", streamDigest)
	if _, err := s.Verify(old, HopAgentRPC, "env-1", "node-a", nil); !errors.Is(err, ErrBadSignature) {
		t.Fatalf("a v2 signature must be refused, got %v", err)
	}
}

// ownerReplying runs an agent-rpc and stats owner that answers with reply, which the test
// signs (or not) however it likes.
func ownerReplying(t *testing.T, reply http.HandlerFunc) *Link {
	t.Helper()
	owner := httptest.NewServer(reply)
	t.Cleanup(owner.Close)
	database := migrated(t)
	link := New(NewOwners(database, "node-b", "http://node-b:8705"), NewSigner("shared-secret"), noAgents{}, quiet())
	link.SetRequireTLS(false) // an httptest peer is plain http; production requires TLS
	claimFor(t, database, "env-1", "node-a", owner.URL)
	return link
}

// TestAgentReplyMustBeSignedForTheRequest: a 200 agent answer the owning node did not sign,
// or signed with another key, or signed for another request, is an error, not a response.
func TestAgentReplyMustBeSignedForTheRequest(t *testing.T) {
	forger := NewSigner("not-the-shared-secret")
	genuine := NewSigner("shared-secret")
	replies := map[string]func(w http.ResponseWriter, r *http.Request){
		"unsigned": func(http.ResponseWriter, *http.Request) {},
		"wrong key": func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set(HeaderReplySignature, replyHead(t, forger, "node-b", "node-a", r.Header.Get(HeaderNonce), w.Header()))
		},
		"another request": func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set(HeaderReplySignature, replyHead(t, genuine, "node-b", "node-a", "some-other-nonce", w.Header()))
		},
		"status rewritten": func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set(HeaderReplySignature, replyHead(t, genuine, "node-b", "node-a", r.Header.Get(HeaderNonce), w.Header()))
			w.Header().Set(HeaderStatus, "204")
		},
	}
	for name, sign := range replies {
		link := ownerReplying(t, func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set(HeaderInnerPrefix+"Content-Type", "application/json")
			w.Header().Set(HeaderStatus, "200")
			w.Header().Set(HeaderExecuted, "node-a")
			w.Header().Set(HeaderRPC, "1")
			sign(w, r)
			ignoreErr(w.Write([]byte(`{"Id":"forged"}`)))
		})
		if err := roundTripErr(link.Transport("env-1"), bodyRequest(nil)); !errors.Is(err, ErrBadSignature) {
			t.Fatalf("%s: an agent reply not signed for this request must fail, got %v", name, err)
		}
	}
}

// TestStatsReplyMustBeSignedForTheRequest: a stats snapshot the owning node did not sign, or
// whose body differs from what it signed, is an error rather than this node's truth.
func TestStatsReplyMustBeSignedForTheRequest(t *testing.T) {
	genuine := NewSigner("shared-secret")
	body := []byte(`{"samples":[{"id":"forged"}]}`)
	replies := map[string]func(w http.ResponseWriter, r *http.Request){
		"unsigned": func(w http.ResponseWriter, _ *http.Request) { ignoreErr(w.Write(body)) },
		"body rewritten": func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set(HeaderReplySignature, genuine.SignReply("node-b", "node-a", r.Header.Get(HeaderNonce), []byte(`{}`)))
			ignoreErr(w.Write(body))
		},
	}
	for name, reply := range replies {
		link := ownerReplying(t, reply)
		if _, ok, err := link.RemoteSnapshot(context.Background(), "env-1"); ok || !errors.Is(err, ErrBadSignature) {
			t.Fatalf("%s: a stats reply not signed for this request must fail, got ok=%v err=%v", name, ok, err)
		}
	}
}

// TestNonceSweepForgetsExpiredNonces: the sweep goroutine, not the request path, empties the
// replay store of nonces past the window, and keeps the fresh ones.
func TestNonceSweepForgetsExpiredNonces(t *testing.T) {
	s := NewSigner("shared-secret")
	s.mu.Lock()
	s.seen["expired"] = time.Now().Add(-10 * time.Minute)
	s.seen["fresh"] = time.Now()
	s.mu.Unlock()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	s.startNonceSweep(ctx, 5*time.Millisecond)

	deadline := time.Now().Add(5 * time.Second)
	for {
		s.mu.Lock()
		_, expired := s.seen["expired"]
		_, fresh := s.seen["fresh"]
		s.mu.Unlock()
		if !expired {
			if !fresh {
				t.Fatal("the sweep must keep a nonce still inside the window")
			}
			return
		}
		if time.Now().After(deadline) {
			t.Fatal("the sweep never removed the expired nonce")
		}
		time.Sleep(5 * time.Millisecond)
	}
}

// TestRefusalLogClipsEveryAttribute: the env id comes from the route of a public endpoint,
// so it is bounded in the log exactly like the peer header.
func TestRefusalLogClipsEveryAttribute(t *testing.T) {
	var buf bytes.Buffer
	link := New(nil, nil, nil, slog.New(slog.NewTextHandler(&buf, nil)))
	link.logRefusal(HopStats, ErrBadSignature, strings.Repeat("p", 500), "env", strings.Repeat("e", 500))
	out := buf.String()
	if strings.Contains(out, strings.Repeat("e", maxLoggedHeader+1)) || strings.Contains(out, strings.Repeat("p", maxLoggedHeader+1)) {
		t.Fatalf("a caller-supplied value reached the log unbounded: %s", out)
	}
	if !strings.Contains(out, strings.Repeat("e", maxLoggedHeader)) {
		t.Fatalf("the env attribute must still be logged, clipped: %s", out)
	}
}
