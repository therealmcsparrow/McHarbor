// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package agentrpc

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	coreagent "github.com/therealmcsparrow/mcharbor/core/agent"
)

// untouchablePool fails the test if the bridge ever resolves an environment, which is how
// a refusal is shown to happen before the agent is reached.
type untouchablePool struct{ t *testing.T }

func (p untouchablePool) IsAgentEnv(string) bool {
	p.t.Error("the pool was consulted for a call that should have been refused first")
	return true
}

func (p untouchablePool) AgentConnection(string) (*coreagent.AgentConnection, bool) {
	p.t.Error("the pool was consulted for a call that should have been refused first")
	return nil, false
}

// fixedPool answers from fields, for the environment-resolution codes.
type fixedPool struct {
	agentEnv bool
	conn     *coreagent.AgentConnection
}

func (p fixedPool) IsAgentEnv(string) bool { return p.agentEnv }

func (p fixedPool) AgentConnection(string) (*coreagent.AgentConnection, bool) {
	return p.conn, p.conn != nil
}

func quietBridge(pool AgentPool) *Bridge {
	return New(pool, slog.New(slog.NewTextHandler(io.Discard, nil)))
}

// agentRPCCoder is the interface a plugin declares locally to read the code.
type agentRPCCoder interface{ AgentRPCCode() string }

func codeOf(t *testing.T, err error) string {
	t.Helper()
	var coder agentRPCCoder
	if !errors.As(err, &coder) {
		t.Fatalf("error %v does not expose AgentRPCCode", err)
	}
	return coder.AgentRPCCode()
}

func TestViewRefusesAnUndeclaredType(t *testing.T) {
	v := quietBridge(untouchablePool{t}).ForPlugin("addon-demo", []string{coreagent.MsgVMQuery}, nil)
	for _, msgType := range []string{coreagent.MsgVMStart, coreagent.MsgExecStart, coreagent.MsgCTStart} {
		_, err := v.Call(context.Background(), "env-1", msgType, nil, nil)
		if !errors.Is(err, ErrUndeclared) || codeOf(t, err) != CodeRefused {
			t.Errorf("%s: got %v, want an undeclared refusal", msgType, err)
		}
	}
}

// TestDeclarationCannotWidenTheCeiling proves ForPlugin narrows: a type outside the host's
// ceiling is dropped even if a hand-built declaration lists it.
func TestDeclarationCannotWidenTheCeiling(t *testing.T) {
	v := quietBridge(untouchablePool{t}).ForPlugin("addon-demo",
		[]string{coreagent.MsgExecStart, coreagent.MsgHostTerminalStart, coreagent.MsgAgentUpdateRequest},
		[]string{coreagent.MsgExecOutput})
	if len(v.send) != 0 || len(v.observe) != 0 {
		t.Fatalf("outside-ceiling types survived: send=%v observe=%v", v.send, v.observe)
	}
	if _, err := v.Call(context.Background(), "env-1", coreagent.MsgExecStart, nil, nil); !errors.Is(err, ErrUndeclared) {
		t.Fatalf("exec_start: got %v", err)
	}
}

func TestCallNamesWhyTheEnvironmentCannotTakeIt(t *testing.T) {
	cases := []struct {
		name string
		pool AgentPool
		env  string
		code string
	}{
		{"no environment", fixedPool{}, "", CodeNotAgentEnv},
		{"not an agent environment", fixedPool{}, "env-1", CodeNotAgentEnv},
		{"no pool", nil, "env-1", CodeNotAgentEnv},
		{"agent offline", fixedPool{agentEnv: true}, "env-1", CodeAgentOffline},
	}
	for _, tc := range cases {
		_, err := quietBridge(tc.pool).Call(context.Background(), tc.env, coreagent.MsgVMQuery, nil, nil)
		if err == nil || codeOf(t, err) != tc.code {
			t.Errorf("%s: got %v, want code %s", tc.name, err, tc.code)
		}
	}
	_, err := quietBridge(fixedPool{agentEnv: true}).Call(context.Background(), "env-1", coreagent.MsgVMQuery, nil, nil)
	if !errors.Is(err, coreagent.ErrAgentOffline) {
		t.Fatalf("an offline refusal must still unwrap to the transport sentinel, got %v", err)
	}
}

func TestClassifyKeepsTheTransportWording(t *testing.T) {
	floor := fmt.Errorf("%w: ct_backup_run needs agent 2026.09.01 build 202609270250 or newer", coreagent.ErrComputeUnsupported)
	cases := []struct {
		err  error
		code string
	}{
		{floor, CodeAgentOutdated},
		{fmt.Errorf("%w: gone", coreagent.ErrAgentOffline), CodeAgentOffline},
		{fmt.Errorf("%w: eth0 carries br0", coreagent.ErrUplinkBusy), CodeUplinkBusy},
		{fmt.Errorf("%w: lan0 belongs to the operator", coreagent.ErrHostLinkNameTaken), CodeNameTaken},
		{fmt.Errorf("%w: eth9", coreagent.ErrHostLinkParentNotFound), CodeParentNotFound},
		{fmt.Errorf("%w: operation not permitted", coreagent.ErrHostLinkPermissionDenied), CodePermissionDenied},
		{fmt.Errorf("%w: %q", coreagent.ErrNotPluginRoutable, "exec_start"), CodeRefused},
		{errors.New("agent failed vm_start: no such unit"), CodeRefused},
		{context.DeadlineExceeded, CodeRefused},
	}
	for _, tc := range cases {
		got := classify(tc.err)
		if codeOf(t, got) != tc.code || got.Error() != tc.err.Error() || !errors.Is(got, tc.err) {
			t.Errorf("classify(%v) = %v code %s, want code %s with the same wording", tc.err, got, codeOf(t, got), tc.code)
		}
	}
}

// waitFor polls cond, for the queue goroutines that deliver after Observe returns.
func waitFor(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", what)
}

func TestSubscribeDeliversOnlyObservedFrames(t *testing.T) {
	b := quietBridge(nil)
	var mu sync.Mutex
	var got []string
	b.ForPlugin("addon-kvm", []string{coreagent.MsgVMQuery}, []string{coreagent.MsgVMEvent}).
		Subscribe(func(envID, msgType string, frame []byte) {
			mu.Lock()
			got = append(got, envID+"|"+msgType+"|"+string(frame))
			mu.Unlock()
		})
	var silent atomic.Int32
	b.ForPlugin("addon-quiet", []string{coreagent.MsgVMQuery}, nil).
		Subscribe(func(string, string, []byte) { silent.Add(1) })

	b.Observe("env-1", coreagent.MsgNetLeaseEvent, []byte(`{"lease":{}}`))
	b.Observe("env-1", coreagent.MsgVMEvent, []byte(`{"event":{"kind":"stop"}}`))
	b.Observe("env-1", coreagent.MsgCTBackupProgress, []byte(`{}`))

	waitFor(t, "the vm_event", func() bool {
		mu.Lock()
		defer mu.Unlock()
		return len(got) >= 1
	})
	time.Sleep(50 * time.Millisecond)
	mu.Lock()
	defer mu.Unlock()
	if len(got) != 1 || got[0] != `env-1|vm_event|{"event":{"kind":"stop"}}` {
		t.Fatalf("delivered %q, want only the vm_event", got)
	}
	if n := silent.Load(); n != 0 {
		t.Fatalf("a plugin that observes nothing received %d frames", n)
	}
}

// TestSlowSinkDoesNotBlock is the read-loop guarantee: a sink that never returns fills its
// own queue and starts losing frames, while Observe keeps returning and another plugin keeps
// receiving.
func TestSlowSinkDoesNotBlock(t *testing.T) {
	b := quietBridge(nil)
	stuck := make(chan struct{})
	t.Cleanup(func() { close(stuck) })
	b.ForPlugin("addon-slow", []string{coreagent.MsgVMQuery}, []string{coreagent.MsgVMEvent}).
		Subscribe(func(string, string, []byte) { <-stuck })
	var fast atomic.Int32
	b.ForPlugin("addon-fast", []string{coreagent.MsgVMQuery}, []string{coreagent.MsgVMEvent}).
		Subscribe(func(string, string, []byte) { fast.Add(1) })

	const frames = queueSize * 4
	returned := make(chan struct{})
	go func() {
		for range frames {
			b.Observe("env-1", coreagent.MsgVMEvent, []byte(`{}`))
		}
		close(returned)
	}()
	select {
	case <-returned:
	case <-time.After(2 * time.Second):
		t.Fatal("Observe blocked behind a stuck sink; the agent read loop would stall")
	}
	if dropped := b.Dropped("addon-slow"); dropped == 0 {
		t.Fatal("a stuck sink's queue overflowed without counting a single drop")
	}
	waitFor(t, "the fast plugin to receive frames", func() bool { return fast.Load() > 0 })
}

func TestUnsubscribeStopsDelivery(t *testing.T) {
	b := quietBridge(nil)
	var n atomic.Int32
	b.ForPlugin("addon-kvm", nil, []string{coreagent.MsgVMEvent}).Subscribe(func(string, string, []byte) { n.Add(1) })
	b.Unsubscribe("addon-kvm")
	b.Observe("env-1", coreagent.MsgVMEvent, []byte(`{}`))
	time.Sleep(50 * time.Millisecond)
	if n.Load() != 0 {
		t.Fatal("an unloaded plugin still received a frame")
	}
}

func TestPanickingSinkIsContained(t *testing.T) {
	b := quietBridge(nil)
	var after atomic.Int32
	b.ForPlugin("addon-kvm", nil, []string{coreagent.MsgVMEvent}).Subscribe(func(_, _ string, frame []byte) {
		if string(frame) == "boom" {
			var none []byte
			_ = none[len(frame)]
		}
		after.Add(1)
	})
	b.Observe("env-1", coreagent.MsgVMEvent, []byte("boom"))
	b.Observe("env-1", coreagent.MsgVMEvent, []byte("ok"))
	waitFor(t, "the frame after the panic", func() bool { return after.Load() == 1 })
}
