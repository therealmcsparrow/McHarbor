// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package nodelink

import (
	"bytes"
	"context"
	"database/sql"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	_ "modernc.org/sqlite"

	"github.com/therealmcsparrow/mcharbor/core/agent"
	"github.com/therealmcsparrow/mcharbor/core/audit"
	"github.com/therealmcsparrow/mcharbor/core/auth"
	"github.com/therealmcsparrow/mcharbor/core/db"
)

func bodyRequest(body []byte) *http.Request {
	req := httptest.NewRequest(http.MethodPost, "http://agent/v1.43/containers/create", bytes.NewReader(body))
	req.ContentLength = int64(len(body))
	return req
}

func quiet() *slog.Logger { return slog.New(slog.NewTextHandler(io.Discard, nil)) }

func migrated(t *testing.T) *sql.DB {
	t.Helper()
	database, err := db.Open(db.Config{Driver: db.DriverSQLite, Path: filepath.Join(t.TempDir(), "n.db")})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ignoreErr(database.Close()) })
	if err := db.Migrate(database, db.DriverSQLite); err != nil {
		t.Fatal(err)
	}
	return database
}

// noAgents is a pool that holds nothing, which is what the forwarding half of the link
// looks at: this node has no local transport, so every call has to go over the hop.
type noAgents struct{}

func (noAgents) Transport(string) *agent.AgentTransport { return nil }
func (noAgents) List() []agent.AgentConnection          { return nil }

// TestForwardCarriesIdentityAndTimeout is the contract the audit trail and every Docker
// call depend on: the hop keeps the caller's identity, keeps the caller's remaining
// deadline, verifies as a node, and records which node ran the work.
func TestForwardCarriesIdentityAndTimeout(t *testing.T) {
	signer := NewSigner("shared-secret")
	var seen Claims
	var verifyErr error
	owner := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, readErr := io.ReadAll(r.Body)
		ignoreErr(readErr)
		seen, verifyErr = signer.Verify(r, HopAgentRPC, "env-1", "node-a", body)
		w.Header().Set(HeaderInnerPrefix+"Content-Type", "application/json")
		w.Header().Set(HeaderStatus, "201")
		w.Header().Set(HeaderExecuted, "node-a")
		w.Header().Set(HeaderRPC, "1")
		w.Header().Set(HeaderReplySignature, replyHead(t, signer, "node-b", "node-a", r.Header.Get(HeaderNonce), w.Header()))
		w.WriteHeader(201)
		ignoreErr(w.Write([]byte(`{"Id":"abc"}`)))
	}))
	defer owner.Close()

	database := migrated(t)
	owners := NewOwners(database, "node-b", "http://node-b:8705")
	link := New(owners, signer, noAgents{}, quiet())
	link.SetRequireTLS(false) // an httptest peer is plain http; production requires TLS
	// The owning node is node-a, reachable at the test server.
	claimFor(t, database, "env-1", "node-a", owner.URL)

	ctx := audit.WithExecutor(context.WithValue(context.Background(), auth.UserContextKey, &auth.User{ID: "user-1", Username: "ada"}))
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()

	req := bodyRequest([]byte(`{"Image":"nginx"}`))
	resp, err := link.Transport("env-1").RoundTrip(req.WithContext(ctx))
	if err != nil {
		t.Fatalf("the forwarded call must succeed: %v", err)
	}
	defer func() { ignoreErr(resp.Body.Close()) }()

	if verifyErr != nil {
		t.Fatalf("the owning node must accept the signature: %v", verifyErr)
	}
	if seen.ActorID != "user-1" || seen.ActorName != "ada" {
		t.Fatalf("the caller identity must reach the owning node, got %q/%q", seen.ActorID, seen.ActorName)
	}
	if seen.Timeout <= 0 || seen.Timeout > 20*time.Second {
		t.Fatalf("the caller's remaining deadline must reach the owning node, got %v", seen.Timeout)
	}
	if seen.Method != http.MethodPost || seen.Path != "/v1.43/containers/create" {
		t.Fatalf("the request line must reach the owning node, got %s %s", seen.Method, seen.Path)
	}
	if resp.StatusCode != 201 {
		t.Fatalf("the agent's status must come back, got %d", resp.StatusCode)
	}
	if resp.Header.Get("Content-Type") != "application/json" {
		t.Fatalf("the agent's headers must come back, got %q", resp.Header.Get("Content-Type"))
	}
	if got, err := io.ReadAll(resp.Body); err != nil || string(got) != `{"Id":"abc"}` {
		t.Fatalf("the agent's body must come back, got %q (%v)", got, err)
	}
	if node := audit.ExecutorFromContext(ctx); node != "node-a" {
		t.Fatalf("the audit row must say which node executed the call, got %q", node)
	}
	if link.Proof().Status != ProofProven {
		t.Fatalf("a real cross-node call must be recorded as proof, got %+v", link.Proof())
	}
}

// TestForwardRefusalIsNotAnAgentAnswer guards the one ambiguity the tunnel could have:
// a refusal by the owning node must never read as a response from the Docker daemon.
func TestForwardRefusalIsNotAnAgentAnswer(t *testing.T) {
	signer := NewSigner("shared-secret")
	owner := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		refuse(w, http.StatusConflict, "this node does not hold the agent connection for env-1")
	}))
	defer owner.Close()

	database := migrated(t)
	link := New(NewOwners(database, "node-b", "http://node-b:8705"), signer, noAgents{}, quiet())
	link.SetRequireTLS(false) // an httptest peer is plain http; production requires TLS
	claimFor(t, database, "env-1", "node-a", owner.URL)

	err := roundTripErr(link.Transport("env-1"), bodyRequest(nil))
	if err == nil {
		t.Fatal("a refusal by the owning node must be an error, not a 409 from the daemon")
	}
	if link.Proof().Status != ProofFailed {
		t.Fatalf("a refused hop must be recorded as a failure, got %+v", link.Proof())
	}
}

// TestForwardWithoutAnOwnerFails proves a disconnected agent reads as disconnected from
// every node, rather than as a routing error.
func TestForwardWithoutAnOwnerFails(t *testing.T) {
	link := New(NewOwners(migrated(t), "node-b", "http://node-b:8705"), NewSigner("s"), noAgents{}, quiet())
	link.SetRequireTLS(false) // an httptest peer is plain http; production requires TLS
	if err := roundTripErr(link.Transport("env-nobody"), bodyRequest(nil)); err == nil {
		t.Fatal("an agent no node holds must not be reachable")
	}
}

// TestForwardWithoutASecretFails proves the link refuses rather than forwarding
// unauthenticated when MCHARBOR_SECRET is unset.
func TestForwardWithoutASecretFails(t *testing.T) {
	database := migrated(t)
	link := New(NewOwners(database, "node-b", "http://node-b:8705"), nil, noAgents{}, quiet())
	link.SetRequireTLS(false) // an httptest peer is plain http; production requires TLS
	claimFor(t, database, "env-1", "node-a", "http://node-a:8705")
	if link.CanForward() {
		t.Fatal("a node with no shared secret must not claim it can forward")
	}
	if err := roundTripErr(link.Transport("env-1"), bodyRequest(nil)); err == nil {
		t.Fatal("a node with no shared secret must refuse to forward")
	}
}

// claimFor writes a live claim held by another node, which is the state the forwarding
// half reads.
func claimFor(t *testing.T, database *sql.DB, envID, nodeID, advertise string) {
	t.Helper()
	now := time.Now().UTC()
	if _, err := database.Exec(`INSERT INTO cluster_agent_owners (id, environment_id, node_id, advertise_url,
		claimed_at, expires_at, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		"claim-"+envID+"-"+nodeID, envID, nodeID, advertise,
		now.Format(time.RFC3339), now.Add(OwnerLease).Format(time.RFC3339),
		now.Format(time.RFC3339), now.Format(time.RFC3339)); err != nil {
		t.Fatal(err)
	}
}

// TestRebuildResponseReadsContentLength keeps a forwarded response's length honest, since
// the Docker SDK reads some bodies by length rather than to EOF.
func TestRebuildResponseReadsContentLength(t *testing.T) {
	outer := &http.Response{Header: http.Header{}, Body: io.NopCloser(bytes.NewReader([]byte("hello")))}
	outer.Header.Set(HeaderStatus, "200")
	outer.Header.Set(HeaderInnerPrefix+"Content-Length", strconv.Itoa(5))
	rebuilt, err := rebuildResponse(bodyRequest(nil), outer)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { ignoreErr(rebuilt.Body.Close()) }()
	if rebuilt.ContentLength != 5 {
		t.Fatalf("Content-Length must survive the hop, got %d", rebuilt.ContentLength)
	}
}
