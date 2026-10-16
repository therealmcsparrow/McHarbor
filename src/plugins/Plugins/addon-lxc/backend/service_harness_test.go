// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"testing"

	"github.com/therealmcsparrow/mcharbor/core/plugin"
	_ "modernc.org/sqlite"
)

type sqliteHandle struct{ db *sql.DB }

func (h *sqliteHandle) Exec(ctx context.Context, query string, args ...any) (int64, error) {
	result, err := h.db.ExecContext(ctx, query, args...)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}

func (h *sqliteHandle) Query(ctx context.Context, query string, args ...any) (plugin.Rows, error) {
	return h.db.QueryContext(ctx, query, args...)
}

func (h *sqliteHandle) KV() plugin.KVStore { return nil }

type quietLogger struct{}

func (quietLogger) Info(string, ...any)  {}
func (quietLogger) Warn(string, ...any)  {}
func (quietLogger) Error(string, ...any) {}
func (quietLogger) Debug(string, ...any) {}

type sentCommand struct {
	messageType string
	payload     agentPayload
}

type fakeRPC struct {
	mu     sync.Mutex
	sent   []sentCommand
	reply  func(messageType string, payload agentPayload) ([]byte, error)
	canRun bool
	// cached is the host image cache; nil holds just the image createRequest names.
	cached []string
	sink   func(envID, msgType string, frame []byte)
}

func (f *fakeRPC) Available(context.Context, string) (bool, string) { return true, "" }

// Call answers the three read messages the service asks the host for, and records and
// replies to every guest command, as the host's bridge would.
func (f *fakeRPC) Call(_ context.Context, _, messageType string, body []byte, _ func(string, []byte)) ([]byte, error) {
	switch messageType {
	case msgComputeProbe:
		return json.Marshal(agentComputePayload{Capability: &agentCapability{CanRunContainers: f.canRun}})
	case msgImageList:
		ids := f.cached
		if ids == nil {
			ids = []string{"img-alpine-321-rootfs-amd64"}
		}
		reply := agentImagePayload{}
		for _, id := range ids {
			reply.Cached = append(reply.Cached, agentCachedImage{ImageID: id, LocalPath: "/cache/" + id})
		}
		return json.Marshal(reply)
	}
	var payload agentPayload
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, err
	}
	f.mu.Lock()
	f.sent = append(f.sent, sentCommand{messageType: messageType, payload: payload})
	f.mu.Unlock()
	if f.reply != nil {
		return f.reply(messageType, payload)
	}
	return []byte(`{"success":true}`), nil
}

func (f *fakeRPC) Subscribe(sink func(envID, msgType string, frame []byte)) { f.sink = sink }

func (f *fakeRPC) last(messageType string) (agentPayload, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for index := len(f.sent) - 1; index >= 0; index-- {
		if f.sent[index].messageType == messageType {
			return f.sent[index].payload, true
		}
	}
	return agentPayload{}, false
}

func newTestService(t *testing.T, rpc *fakeRPC) (*Service, *sql.DB) {
	t.Helper()
	db, err := sql.Open("sqlite", "file:"+filepath.Join(t.TempDir(), "lxc.db")+"?_pragma=foreign_keys(1)")
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = db.Close() })

	files, err := filepath.Glob(filepath.Join("..", "migrations", "*.sql"))
	if err != nil || len(files) == 0 {
		t.Fatalf("no migrations found: %v", err)
	}
	sort.Strings(files)
	for _, file := range files {
		body, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := db.Exec(string(body)); err != nil {
			t.Fatalf("applying %s: %v", file, err)
		}
	}

	svc := NewService(&sqliteHandle{db: db}, rpcOrNil(rpc), nil, nil, nil, quietLogger{})
	if err := svc.EnsureSchema(context.Background()); err != nil {
		t.Fatalf("EnsureSchema: %v", err)
	}
	return svc, db
}

// rpcOrNil keeps a nil *fakeRPC from becoming a non-nil interface holding nil.
func rpcOrNil(rpc *fakeRPC) any {
	if rpc == nil {
		return nil
	}
	return rpc
}
