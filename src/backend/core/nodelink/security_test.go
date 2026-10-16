// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package nodelink

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/rs/xid"
)

// countingServer counts every request that reaches it, whatever it is.
func countingServer(t *testing.T) (*httptest.Server, *atomic.Int32) {
	t.Helper()
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits.Add(1)
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(srv.Close)
	return srv, &hits
}

// redirectingServer answers every request with a 307 to target, the shape that would
// re-send a signed POST, body and headers included, to another host.
func redirectingServer(t *testing.T, target string) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, target+r.URL.Path, http.StatusTemporaryRedirect)
	}))
	t.Cleanup(srv.Close)
	return srv
}

// TestLinkDoesNotFollowRedirects: a peer answering 307 must not make this node re-send a
// signed call, nonce and signature included, to the host the peer names.
func TestLinkDoesNotFollowRedirects(t *testing.T) {
	elsewhere, hits := countingServer(t)
	peer := redirectingServer(t, elsewhere.URL)

	database := migrated(t)
	link := New(NewOwners(database, "node-b", "http://node-b:8705"), NewSigner("shared-secret"), noAgents{}, quiet())
	link.SetRequireTLS(false) // an httptest peer is plain http; production requires TLS
	claimFor(t, database, "env-1", "node-a", peer.URL)
	if err := roundTripErr(link.Transport("env-1"), bodyRequest([]byte(`{"Image":"nginx"}`))); err == nil {
		t.Fatal("a 307 from the owning node must be a refusal, not a followed redirect")
	}

	link.SetPeers(func(context.Context) ([]Peer, error) {
		return []Peer{{NodeID: "node-a", AdvertiseURL: peer.URL}}, nil
	})
	link.rememberLeader("node-a")
	if _, err := link.ForwardTriggerFire(context.Background(), TriggerFire{FireID: "f1", ActionKey: "plugin:addon-demo:x", Source: "addon-demo"}); err == nil {
		t.Fatal("a 307 from the leader must be a failed delivery")
	}
	if n := hits.Load(); n != 0 {
		t.Fatalf("the redirect target received %d signed requests; it must receive none", n)
	}
}

// recordingBody fails the test's expectations if anything reads it.
type recordingBody struct {
	reads atomic.Int32
	r     *bytes.Reader
}

func (b *recordingBody) Read(p []byte) (int, error) {
	b.reads.Add(1)
	return b.r.Read(p)
}

func (b *recordingBody) Close() error { return nil }

// fireRequest builds a request to this node's fire route whose body records reads.
func fireRequest(t *testing.T, claims Claims, body []byte, mutate func(*http.Request)) (*http.Request, *recordingBody) {
	t.Helper()
	rec := &recordingBody{r: bytes.NewReader(body)}
	req := httptest.NewRequest(http.MethodPost, triggerFireRoute, nil)
	req.Body = rec
	req.ContentLength = int64(len(body))
	sum := sha256.Sum256(body)
	mustSign(t, NewSigner("shared-secret"), req, claims, xid.New().String(), hex.EncodeToString(sum[:]))
	if mutate != nil {
		mutate(req)
	}
	return req, rec
}

// TestNodeEndpointsRefuseBeforeReadingTheBody: an unsigned, stale, misaddressed or
// unbounded request is refused from its headers; not one byte of its body is read.
func TestNodeEndpointsRefuseBeforeReadingTheBody(t *testing.T) {
	receiver := &stubReceiver{}
	receiver.leader.Store(true)
	link, _ := fireNode(t, "node-a", receiver)
	body := []byte(`{"fireId":"f1","actionKey":"plugin:addon-demo:x","source":"addon-demo"}`)
	claims := Claims{Hop: HopWorkflowTrigger, NodeID: "node-b", Audience: "node-a", Method: http.MethodPost, Path: triggerFireClaimPath, ActorID: "addon-demo"}

	cases := []struct {
		name   string
		mutate func(*http.Request)
		want   int
	}{
		{"no signature", func(r *http.Request) { r.Header.Del(HeaderSignature) }, http.StatusUnauthorized},
		{"skewed timestamp", func(r *http.Request) {
			r.Header.Set(HeaderTimestamp, strconv.FormatInt(time.Now().Add(-10*time.Minute).Unix(), 10))
		}, http.StatusUnauthorized},
		{"addressed to another node", func(r *http.Request) { r.Header.Set(HeaderAudience, "node-c") }, http.StatusUnauthorized},
		{"oversized length", func(r *http.Request) { r.ContentLength = maxSignedBody + 1 }, http.StatusRequestEntityTooLarge},
		{"unknown length", func(r *http.Request) { r.ContentLength = -1 }, http.StatusRequestEntityTooLarge},
	}
	for _, tc := range cases {
		t.Run("fire/"+tc.name, func(t *testing.T) {
			req, rec := fireRequest(t, claims, body, tc.mutate)
			w := httptest.NewRecorder()
			link.ServeTriggerFire(w, req)
			if w.Code != tc.want {
				t.Fatalf("want %d, got %d", tc.want, w.Code)
			}
			if n := rec.reads.Load(); n != 0 {
				t.Fatalf("the body was read %d times before the refusal", n)
			}
		})
		t.Run("agent-rpc/"+tc.name, func(t *testing.T) {
			rpcClaims := Claims{Hop: HopAgentRPC, NodeID: "node-b", Audience: "node-a", EnvID: "env-1", Method: http.MethodPost, Path: "/v1.43/containers/create"}
			req, rec := fireRequest(t, rpcClaims, body, tc.mutate)
			w := httptest.NewRecorder()
			link.ServeAgentRPC(w, req, "env-1")
			if w.Code != tc.want {
				t.Fatalf("want %d, got %d", tc.want, w.Code)
			}
			if n := rec.reads.Load(); n != 0 {
				t.Fatalf("the body was read %d times before the refusal", n)
			}
		})
	}
	if n := receiver.calls.Load(); n != 0 {
		t.Fatalf("the trigger service ran for a refused request (%d calls)", n)
	}
}

// TestAudienceIsBoundIntoTheSignature: a call signed for node-c is refused by node-a,
// whether the audience header is left alone or rewritten to name node-a.
func TestAudienceIsBoundIntoTheSignature(t *testing.T) {
	receiver := &stubReceiver{}
	receiver.leader.Store(true)
	_, srv := fireNode(t, "node-a", receiver)
	body := []byte(`{"fireId":"f1","actionKey":"plugin:addon-demo:x","source":"addon-demo"}`)
	forC := Claims{Hop: HopWorkflowTrigger, NodeID: "node-b", Audience: "node-c", Method: http.MethodPost, Path: triggerFireClaimPath, ActorID: "addon-demo"}

	if got := statusOf(t, signedFire(t, srv.URL, forC, "", body)); got != http.StatusUnauthorized {
		t.Fatalf("a fire addressed to node-c must be refused by node-a, got %d", got)
	}
	rewritten := signedFire(t, srv.URL, forC, "", body)
	rewritten.Header.Set(HeaderAudience, "node-a")
	if got := statusOf(t, rewritten); got != http.StatusUnauthorized {
		t.Fatalf("rewriting the audience header must break the signature, got %d", got)
	}
	if n := receiver.calls.Load(); n != 0 {
		t.Fatalf("the trigger service ran for a misaddressed fire (%d calls)", n)
	}
}

// v1Sign stamps req the way a build before the audience claim did, so the test can show
// such a signature is refused rather than accepted alongside the new one.
func v1Sign(s *Signer, req *http.Request, c Claims, nonce, digest string) {
	timestamp := strconv.FormatInt(time.Now().UTC().Unix(), 10)
	req.Header.Set(HeaderNode, c.NodeID)
	req.Header.Set(HeaderMethod, c.Method)
	req.Header.Set(HeaderPath, c.Path)
	req.Header.Set(HeaderQuery, c.Query)
	req.Header.Set(HeaderActorID, c.ActorID)
	req.Header.Set(HeaderTimestamp, timestamp)
	req.Header.Set(HeaderNonce, nonce)
	req.Header.Set(HeaderBodyDigest, digest)
	req.Header.Set(HeaderSignature, s.mac(strings.Join([]string{
		"mcharbor-node-link-v1", c.NodeID, c.EnvID, c.Method, c.Path, c.Query, timestamp, nonce, c.ActorID, digest,
	}, "\n")))
}

// TestOldFormatSignatureIsRefused: a v1 signature (no audience) fails closed, with or
// without an audience header added by hand.
func TestOldFormatSignatureIsRefused(t *testing.T) {
	s := NewSigner("shared-secret")
	claims := Claims{Hop: HopAgentRPC, NodeID: "node-b", EnvID: "env-1", Method: http.MethodGet, Path: "/v1.43/containers/json"}

	bare := httptest.NewRequest(http.MethodPost, "/api/internal/node/agent-rpc/env-1", nil)
	v1Sign(s, bare, claims, "nonce-v1", streamDigest)
	if _, err := s.Verify(bare, HopAgentRPC, "env-1", "node-a", nil); !errors.Is(err, ErrBadSignature) {
		t.Fatalf("a v1 signature must be refused, got %v", err)
	}
	addressed := httptest.NewRequest(http.MethodPost, "/api/internal/node/agent-rpc/env-1", nil)
	v1Sign(s, addressed, claims, "nonce-v1b", streamDigest)
	addressed.Header.Set(HeaderAudience, "node-a")
	if _, err := s.Verify(addressed, HopAgentRPC, "env-1", "node-a", nil); !errors.Is(err, ErrBadSignature) {
		t.Fatalf("a v1 signature with an audience header added must still be refused, got %v", err)
	}
}

// forgedLeader answers every request 200 with a plausible, unsigned fire result.
func forgedLeader(t *testing.T) (*httptest.Server, *atomic.Int32) {
	t.Helper()
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits.Add(1)
		w.Header().Set("Content-Type", "application/json")
		ignoreErr(w.Write([]byte(`{"fireId":"f1","started":1,"leader":true,"node":"node-a"}`)))
	}))
	t.Cleanup(srv.Close)
	return srv, &hits
}

// TestUnsignedReplyIsNotADelivery: a 200 that the addressed node did not sign counts as a
// failure (so the sender retries) and never becomes the remembered leader.
func TestUnsignedReplyIsNotADelivery(t *testing.T) {
	forged, hits := forgedLeader(t)
	sender, _ := fireNode(t, "node-b", nil)
	sender.SetPeers(func(context.Context) ([]Peer, error) {
		return []Peer{{NodeID: "node-a", AdvertiseURL: forged.URL}}, nil
	})
	fire := TriggerFire{FireID: "f1", ActionKey: "plugin:addon-demo:x", Source: "addon-demo"}

	if _, err := sender.ForwardTriggerFire(context.Background(), fire); !errors.Is(err, ErrNoTriggerLeader) {
		t.Fatalf("an unsigned probe reply must fail the delivery, got %v", err)
	}
	if _, _, last := sender.triggerWiring(); last != "" {
		t.Fatalf("an unsigned reply must not set the remembered leader, got %q", last)
	}

	sender.rememberLeader("node-x")
	sender.SetPeers(func(context.Context) ([]Peer, error) {
		return []Peer{{NodeID: "node-x", AdvertiseURL: forged.URL}}, nil
	})
	if _, err := sender.ForwardTriggerFire(context.Background(), fire); !errors.Is(err, ErrNoTriggerLeader) {
		t.Fatalf("an unsigned fire reply from the remembered leader must fail the delivery, got %v", err)
	}
	if hits.Load() < 2 {
		t.Fatalf("both deliveries should have reached the forged node, got %d requests", hits.Load())
	}
}

// TestPlaintextPeerRefusedWithoutOptIn: with TLS required, nothing signed goes to an
// http:// peer until the operator sets MCHARBOR_NODE_LINK_ALLOW_HTTP=1.
func TestPlaintextPeerRefusedWithoutOptIn(t *testing.T) {
	t.Setenv(EnvAllowHTTP, "")
	receiver := &stubReceiver{}
	receiver.leader.Store(true)
	_, leaderSrv := fireNode(t, "node-a", receiver)
	sender, _ := fireNode(t, "node-b", nil)
	sender.SetRequireTLS(true)
	sender.SetPeers(func(context.Context) ([]Peer, error) {
		return []Peer{{NodeID: "node-a", AdvertiseURL: leaderSrv.URL}}, nil
	})
	fire := TriggerFire{FireID: "f1", ActionKey: "plugin:addon-demo:x", Source: "addon-demo"}

	if _, err := sender.ForwardTriggerFire(context.Background(), fire); !errors.Is(err, ErrPlaintextPeer) {
		t.Fatalf("an http peer must be refused while TLS is required, got %v", err)
	}
	if n := receiver.calls.Load(); n != 0 {
		t.Fatalf("the plaintext peer was reached %d times", n)
	}

	database := migrated(t)
	agentLink := New(NewOwners(database, "node-b", "https://node-b:8705"), NewSigner("shared-secret"), noAgents{}, quiet())
	agentLink.SetRequireTLS(true)
	claimFor(t, database, "env-1", "node-a", leaderSrv.URL)
	if err := roundTripErr(agentLink.Transport("env-1"), bodyRequest(nil)); !errors.Is(err, ErrPlaintextPeer) {
		t.Fatalf("an agent call to an http owner must be refused while TLS is required, got %v", err)
	}

	t.Setenv(EnvAllowHTTP, "1")
	if _, err := sender.ForwardTriggerFire(context.Background(), fire); err != nil {
		t.Fatalf("with the opt-in the http peer is used: %v", err)
	}
}
