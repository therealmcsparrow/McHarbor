// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package nodelink

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"
)

func signed(t *testing.T, s *Signer, c Claims, nonce string) *http.Request {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/api/internal/node/agent-rpc/"+c.EnvID, nil)
	mustSign(t, s, req, c, nonce, streamDigest)
	return req
}

// TestSignerRoundTrip is the whole point of the signature: a call another node made is
// accepted with its claims intact, and every way of changing it in flight is refused.
func TestSignerRoundTrip(t *testing.T) {
	s := NewSigner("shared-secret")
	if s == nil {
		t.Fatal("a signer with a secret must not be nil")
	}
	claims := Claims{Hop: HopAgentRPC, NodeID: "node-a", Audience: "node-b", EnvID: "env-1", Method: http.MethodGet, Path: "/v1.43/containers/json",
		Query: "all=1", ActorID: "user-1", ActorName: "ada", Timeout: 12 * time.Second}

	got, err := s.Verify(signed(t, s, claims, "nonce-1"), HopAgentRPC, "env-1", "node-b", nil)
	if err != nil {
		t.Fatalf("a correctly signed call must verify: %v", err)
	}
	if got.ActorID != "user-1" || got.ActorName != "ada" {
		t.Fatalf("the caller identity must survive the hop, got %q/%q", got.ActorID, got.ActorName)
	}
	if got.Timeout != 12*time.Second {
		t.Fatalf("the caller timeout must survive the hop, got %v", got.Timeout)
	}
	if got.Path != claims.Path || got.Query != claims.Query || got.Method != claims.Method {
		t.Fatalf("the request line must survive the hop, got %+v", got)
	}
}

// TestSignerRefusals covers each way a forwarded call can be wrong. Each is separate
// because they fail for different reasons and a single table would hide which.
func TestSignerRefusals(t *testing.T) {
	s := NewSigner("shared-secret")
	claims := Claims{Hop: HopAgentRPC, NodeID: "node-a", Audience: "node-b", EnvID: "env-1", Method: http.MethodPost, Path: "/v1.43/containers/x/stop"}

	t.Run("unsigned", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/api/internal/node/agent-rpc/env-1", nil)
		if _, err := s.Verify(req, HopAgentRPC, "env-1", "node-b", nil); !errors.Is(err, ErrNotSigned) {
			t.Fatalf("an unsigned request must be ErrNotSigned, got %v", err)
		}
	})

	t.Run("tampered path", func(t *testing.T) {
		req := signed(t, s, claims, "nonce-path")
		req.Header.Set(HeaderPath, "/v1.43/containers/x/kill")
		if _, err := s.Verify(req, HopAgentRPC, "env-1", "node-b", nil); err == nil {
			t.Fatal("rewriting the path in flight must be refused")
		}
	})

	t.Run("other environment", func(t *testing.T) {
		req := signed(t, s, claims, "nonce-env")
		if _, err := s.Verify(req, HopAgentRPC, "env-2", "node-b", nil); err == nil {
			t.Fatal("a signature for one environment must not verify against another")
		}
	})

	t.Run("replayed nonce", func(t *testing.T) {
		req := signed(t, s, claims, "nonce-replay")
		if _, err := s.Verify(req, HopAgentRPC, "env-1", "node-b", nil); err != nil {
			t.Fatalf("the first use must verify: %v", err)
		}
		replay := signed(t, s, claims, "nonce-replay")
		replay.Header.Set(HeaderSignature, req.Header.Get(HeaderSignature))
		replay.Header.Set(HeaderTimestamp, req.Header.Get(HeaderTimestamp))
		if _, err := s.Verify(replay, HopAgentRPC, "env-1", "node-b", nil); err == nil {
			t.Fatal("a nonce may be used once")
		}
	})

	t.Run("stale timestamp", func(t *testing.T) {
		req := signed(t, s, claims, "nonce-stale")
		req.Header.Set(HeaderTimestamp, strconv.FormatInt(time.Now().Add(-10*time.Minute).Unix(), 10))
		if _, err := s.Verify(req, HopAgentRPC, "env-1", "node-b", nil); err == nil {
			t.Fatal("a timestamp outside the skew window must be refused")
		}
	})

	t.Run("other secret", func(t *testing.T) {
		other := NewSigner("a-different-secret")
		if _, err := other.Verify(signed(t, s, claims, "nonce-secret"), HopAgentRPC, "env-1", "node-b", nil); err == nil {
			t.Fatal("a node with a different MCHARBOR_SECRET must not be trusted")
		}
	})

	t.Run("tampered body", func(t *testing.T) {
		body := []byte(`{"Image":"nginx"}`)
		req := httptest.NewRequest(http.MethodPost, "/api/internal/node/agent-rpc/env-1", nil)
		reader, digest, err := signableBody(bodyRequest(body))
		if err != nil {
			t.Fatal(err)
		}
		_ = reader
		mustSign(t, s, req, claims, "nonce-body", digest)
		if _, err := s.Verify(req, HopAgentRPC, "env-1", "node-b", []byte(`{"Image":"malicious"}`)); err == nil {
			t.Fatal("a rewritten body must be refused when the body was signed")
		}
	})
}

// TestNewSignerWithoutSecret proves the link refuses to sign with nothing rather than
// signing with an empty key, which every node would then accept from anyone.
func TestNewSignerWithoutSecret(t *testing.T) {
	if NewSigner("   ") != nil {
		t.Fatal("no shared secret must mean no signer")
	}
}
