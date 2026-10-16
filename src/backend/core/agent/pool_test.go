// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package agent

import (
	"bytes"
	"io"
	"log/slog"
	"net/http"
	"testing"
)

func TestRemoveIfCurrentKeepsNewerConnection(t *testing.T) {
	pool := NewAgentPool(slog.New(slog.NewTextHandler(io.Discard, nil)))
	oldConn := &AgentConnection{EnvID: "env1", Hostname: "old"}
	newConn := &AgentConnection{EnvID: "env1", Hostname: "new"}

	pool.conns["env1"] = oldConn
	pool.conns["env1"] = newConn
	pool.RemoveIfCurrent("env1", oldConn)

	got, ok := pool.Get("env1")
	if !ok {
		t.Fatal("expected newer connection to remain registered")
	}
	if got != newConn {
		t.Fatalf("expected newer connection, got %#v", got)
	}
}

func TestRemoveIfCurrentRemovesMatchingConnection(t *testing.T) {
	pool := NewAgentPool(slog.New(slog.NewTextHandler(io.Discard, nil)))
	conn := &AgentConnection{EnvID: "env1", Hostname: "agent"}

	pool.conns["env1"] = conn
	pool.RemoveIfCurrent("env1", conn)

	if _, ok := pool.Get("env1"); ok {
		t.Fatal("expected matching connection to be removed")
	}
}

func TestShouldStreamRequestBodyForUnknownLength(t *testing.T) {
	req, err := http.NewRequest(http.MethodPost, "http://docker/images/load", io.NopCloser(bytes.NewReader([]byte("tar"))))
	if err != nil {
		t.Fatal(err)
	}
	req.ContentLength = -1

	if !shouldStreamRequestBody(req) {
		t.Fatal("expected unknown-length request body to stream")
	}
}

func TestShouldStreamRequestBodyKeepsSmallKnownBodyInline(t *testing.T) {
	req, err := http.NewRequest(http.MethodPost, "http://docker/containers/create", bytes.NewReader([]byte("{}")))
	if err != nil {
		t.Fatal(err)
	}

	if shouldStreamRequestBody(req) {
		t.Fatal("expected small known-length request body to stay inline")
	}
}

func TestAgentSupportsRequestStreamingRequiresVersionOneThreeThree(t *testing.T) {
	if agentSupportsRequestStreaming("1.2.1") {
		t.Fatal("expected agent 1.2.1 to be too old for request streaming")
	}
	if agentSupportsRequestStreaming("1.3.0") {
		t.Fatal("expected agent 1.3.0 to be too old for request streaming")
	}
	if !agentSupportsRequestStreaming("1.3.3") {
		t.Fatal("expected agent 1.3.3 to support request streaming")
	}
	if !agentSupportsRequestStreaming("v2.0.0") {
		t.Fatal("expected future major versions to support request streaming")
	}
}
