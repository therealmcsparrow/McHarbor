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
	"sync/atomic"
	"testing"

	"github.com/rs/xid"
)

// stubReceiver answers like a trigger service that does, or does not, lead.
type stubReceiver struct {
	leader atomic.Bool
	calls  atomic.Int32
}

func (s *stubReceiver) ReceiveTriggerFire(context.Context, TriggerFire) (TriggerFireResult, error) {
	s.calls.Add(1)
	if !s.leader.Load() {
		return TriggerFireResult{}, ErrNotLeader
	}
	return TriggerFireResult{Started: 1}, nil
}

func (s *stubReceiver) LeadsWorkflowTrigger(context.Context) bool {
	s.calls.Add(1)
	return s.leader.Load()
}

// testPeers is the live registry every test node sees, so a signed call from any of these
// ids passes the live-instance check; a test that forwards replaces it with SetPeers.
func testPeers(context.Context) ([]Peer, error) {
	peers := []Peer{}
	for _, id := range []string{"node-a", "node-b", "node-c", "node-x"} {
		peers = append(peers, Peer{NodeID: id, AdvertiseURL: "https://" + id + ".invalid"})
	}
	return peers, nil
}

func fireNode(t *testing.T, id string, receiver TriggerFireReceiver) (*Link, *httptest.Server) {
	t.Helper()
	srv := httptest.NewServer(nil)
	t.Cleanup(srv.Close)
	link := New(NewOwners(migrated(t), id, srv.URL), NewSigner("shared-secret"), noAgents{}, quiet())
	link.SetRequireTLS(false) // an httptest peer is plain http; production requires TLS
	link.SetTriggerFireReceiver(receiver)
	link.SetPeers(testPeers)
	srv.Config.Handler = http.HandlerFunc(link.ServeTriggerFire)
	return link, srv
}

func signedFire(t *testing.T, url string, claims Claims, digest string, body []byte) *http.Request {
	t.Helper()
	req, err := http.NewRequest(http.MethodPost, url+triggerFireRoute, bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	if digest == "" {
		sum := sha256.Sum256(body)
		digest = hex.EncodeToString(sum[:])
	}
	mustSign(t, NewSigner("shared-secret"), req, claims, xid.New().String(), digest)
	return req
}

func statusOf(t *testing.T, req *http.Request) int {
	t.Helper()
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	ignoreErr(resp.Body.Close())
	return resp.StatusCode
}

// TestTriggerFireRefusesAnythingButASignedFire: the hop is public, so every shape a
// browser or a replayed signature could take is refused before the trigger service runs.
func TestTriggerFireRefusesAnythingButASignedFire(t *testing.T) {
	receiver := &stubReceiver{}
	receiver.leader.Store(true)
	_, srv := fireNode(t, "node-a", receiver)
	body := []byte(`{"fireId":"f1","actionKey":"plugin:addon-demo:demo-trigger","source":"addon-demo"}`)
	fireClaims := Claims{Hop: HopWorkflowTrigger, NodeID: "node-b", Audience: "node-a", Method: http.MethodPost, Path: triggerFireClaimPath, ActorID: "addon-demo"}

	withOrigin := signedFire(t, srv.URL, fireClaims, "", body)
	withOrigin.Header.Set("Origin", "https://evil.example")
	if got := statusOf(t, withOrigin); got != http.StatusForbidden {
		t.Fatalf("a request with an Origin header must be refused 403, got %d", got)
	}

	unsigned, err := http.NewRequest(http.MethodPost, srv.URL+triggerFireRoute, bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	if got := statusOf(t, unsigned); got != http.StatusUnauthorized {
		t.Fatalf("an unsigned fire must be refused 401, got %d", got)
	}

	statsSigned := signedFire(t, srv.URL, Claims{Hop: HopStats, NodeID: "node-b", Audience: "node-a", Method: http.MethodGet, Path: statsClaimPath}, "", body)
	if got := statusOf(t, statsSigned); got != http.StatusUnauthorized {
		t.Fatalf("a signature for another hop must be refused 401, got %d", got)
	}

	streamed := signedFire(t, srv.URL, fireClaims, streamDigest, body)
	if got := statusOf(t, streamed); got != http.StatusUnauthorized {
		t.Fatalf("a fire signed without a body digest must be refused 401, got %d", got)
	}

	tampered := signedFire(t, srv.URL, fireClaims, "", body)
	tampered.Body = http.NoBody
	tampered.ContentLength = 0
	if got := statusOf(t, tampered); got != http.StatusUnauthorized {
		t.Fatalf("a body that does not match the signed digest must be refused 401, got %d", got)
	}

	if n := receiver.calls.Load(); n != 0 {
		t.Fatalf("the trigger service ran for a refused request (%d calls)", n)
	}

	if got := statusOf(t, signedFire(t, srv.URL, fireClaims, "", body)); got != http.StatusOK {
		t.Fatalf("a correctly signed fire should be accepted, got %d", got)
	}
}

// TestForwardTriggerFireFindsTheLeader: a node that does not lead answers 409 and the
// fire moves on; the node that accepted is tried first next time.
func TestForwardTriggerFireFindsTheLeader(t *testing.T) {
	notLeader := &stubReceiver{}
	leader := &stubReceiver{}
	leader.leader.Store(true)
	_, srvA := fireNode(t, "node-a", notLeader)
	_, srvC := fireNode(t, "node-c", leader)
	sender, _ := fireNode(t, "node-b", nil)
	sender.SetPeers(func(context.Context) ([]Peer, error) {
		return []Peer{
			{NodeID: "node-b", AdvertiseURL: "http://self.invalid"},
			{NodeID: "node-a", AdvertiseURL: srvA.URL},
			{NodeID: "node-c", AdvertiseURL: srvC.URL},
		}, nil
	})
	fire := TriggerFire{FireID: "f1", ActionKey: "plugin:addon-demo:demo-trigger", Source: "addon-demo"}

	res, err := sender.ForwardTriggerFire(context.Background(), fire)
	if err != nil {
		t.Fatalf("forward: %v", err)
	}
	if res.ExecutedBy != "node-c" || res.Started != 1 {
		t.Fatalf("want node-c to evaluate the fire, got %+v", res)
	}
	fire.FireID = "f2"
	if _, err := sender.ForwardTriggerFire(context.Background(), fire); err != nil {
		t.Fatalf("second forward: %v", err)
	}
	if n := notLeader.calls.Load(); n != 1 {
		t.Fatalf("the remembered leader should be offered the second fire first; node-a was asked %d times", n)
	}

	leader.leader.Store(false)
	fire.FireID = "f3"
	_, err = sender.ForwardTriggerFire(context.Background(), fire)
	if !errors.Is(err, ErrNoTriggerLeader) {
		t.Fatalf("with no leader anywhere the forward must report ErrNoTriggerLeader, got %v", err)
	}
}
