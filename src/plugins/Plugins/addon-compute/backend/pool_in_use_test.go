// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

import (
	"context"
	"errors"
	"strings"
	"testing"
)

// refusingRPC answers every call with one reply slot and one error, the way the
// host bridge hands back a pool refusal.
type refusingRPC struct {
	reply string
	err   error
}

func (r refusingRPC) Available(context.Context, string) (bool, string) { return true, "" }

func (r refusingRPC) Call(context.Context, string, string, []byte, func(string, []byte)) ([]byte, error) {
	return []byte(r.reply), r.err
}

func (refusingRPC) Subscribe(func(string, string, []byte)) {}

// TestPoolRemoveReadsTheRefusalCode: the agent's errorCode decides ErrPoolInUse, so
// rewording the agent's sentence cannot turn "still in use" into a generic failure, and
// a sentence that happens to contain the old words cannot fake it.
func TestPoolRemoveReadsTheRefusalCode(t *testing.T) {
	refused := errors.New("agent failed to remove pool")

	b := newAgentBridge(refusingRPC{
		reply: `{"errorCode":"pool_in_use","error":"pool data still holds 2 volumes: vm1, vm2"}`,
		err:   refused,
	})
	err := b.PoolRemove(context.Background(), "env1", PoolRequest{PoolID: "p1"})
	if !errors.Is(err, ErrPoolInUse) {
		t.Fatalf("coded refusal: err = %v, want ErrPoolInUse", err)
	}
	if !strings.Contains(err.Error(), "vm1, vm2") {
		t.Fatalf("the agent's detail was lost: %v", err)
	}

	b = newAgentBridge(refusingRPC{
		reply: `{"error":"disk still holds a lock"}`,
		err:   errors.New("disk still holds a lock"),
	})
	if err := b.PoolRemove(context.Background(), "env1", PoolRequest{PoolID: "p1"}); errors.Is(err, ErrPoolInUse) {
		t.Fatalf("an uncoded refusal was read as pool-in-use from its text: %v", err)
	}
}
