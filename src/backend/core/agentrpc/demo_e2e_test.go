// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package agentrpc

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gorilla/websocket"

	coreagent "github.com/therealmcsparrow/mcharbor/core/agent"
	"github.com/therealmcsparrow/mcharbor/core/demo/computesim"
	"github.com/therealmcsparrow/mcharbor/core/demo/estate"
	"github.com/therealmcsparrow/mcharbor/core/demo/fakeagent"
)

// demoPool serves the one demo agent connection.
type demoPool struct{ conn *coreagent.AgentConnection }

func (p demoPool) IsAgentEnv(envID string) bool { return envID == "env-demo" }

func (p demoPool) AgentConnection(envID string) (*coreagent.AgentConnection, bool) {
	return p.conn, envID == "env-demo" && p.conn != nil
}

// connectDemoAgent runs the real handshake, transport and read loop against a demo agent
// carrying the compute simulator, the way demo mode wires them.
func connectDemoAgent(t *testing.T) *coreagent.AgentConnection {
	t.Helper()
	ready := make(chan *coreagent.AgentConnection, 1)
	up := websocket.Upgrader{}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ws, err := up.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		var auth coreagent.WSMessage
		if err := ws.ReadJSON(&auth); err != nil || auth.Auth == nil {
			t.Errorf("first frame = %+v, %v", auth, err)
			return
		}
		if err := ws.WriteJSON(coreagent.WSMessage{Type: coreagent.MsgAuthResult,
			AuthResult: &coreagent.AuthResultPayload{Success: true, EnvID: "env-demo"}}); err != nil {
			return
		}
		conn := &coreagent.AgentConnection{EnvID: "env-demo", Version: auth.Auth.AgentVersion,
			Build: auth.Auth.AgentBuild, Capabilities: auth.Auth.Capabilities, Conn: ws}
		conn.Transport = coreagent.NewAgentTransport(conn, nil, logger)
		ready <- conn
		if err := conn.Transport.ReadLoopContext(r.Context()); err != nil {
			t.Log(err)
		}
	}))
	t.Cleanup(srv.Close)

	e := estate.Build(estate.DefaultSeed, time.Now())
	sim := computesim.New(e, 7, time.Now())
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	a := fakeagent.New(fakeagent.Config{
		Host: e.Host("prod-eu-1"), Token: "agt_test", Version: "2026.09.01", Build: "209901010000",
		Logger:     logger,
		Extensions: []fakeagent.Extension{sim},
		Dial: func(ctx context.Context, token string) (*websocket.Conn, error) {
			ws, resp, err := websocket.DefaultDialer.DialContext(ctx, "ws"+strings.TrimPrefix(srv.URL, "http")+"/?token="+token, nil)
			if resp != nil && resp.Body != nil {
				if cerr := resp.Body.Close(); cerr != nil {
					t.Log(cerr)
				}
			}
			return ws, err
		},
	})
	go a.Run(ctx)
	select {
	case conn := <-ready:
		return conn
	case <-time.After(5 * time.Second):
		t.Fatal("demo agent never completed the handshake")
	}
	return nil
}

// TestBridgeAgainstTheDemoAgent is the end-to-end floor for the bridge: a plugin view calls
// through the real transport to the compute simulator, is refused what it did not declare,
// and receives the guest events it declared to observe.
func TestBridgeAgainstTheDemoAgent(t *testing.T) {
	conn := connectDemoAgent(t)
	// The constructor main.go wires the process's bridge with, so a bridge that is built
	// without becoming the transport's observer shows up as missing events here.
	b := NewInstalled(demoPool{conn: conn}, quietBridge(nil).logger)
	t.Cleanup(func() { coreagent.SetPluginFrameObserver(nil) })

	view := b.ForPlugin("addon-kvm",
		[]string{coreagent.MsgVMQuery, coreagent.MsgVMPause, coreagent.MsgVMResume, coreagent.MsgCTQuery},
		[]string{coreagent.MsgVMEvent})
	var mu sync.Mutex
	var events []string
	view.Subscribe(func(envID, msgType string, frame []byte) {
		var vm coreagent.VMPayload
		if msgType != coreagent.MsgVMEvent || envID != "env-demo" || json.Unmarshal(frame, &vm) != nil || vm.Event == nil {
			t.Errorf("unexpected frame %s %s %s", envID, msgType, frame)
			return
		}
		mu.Lock()
		events = append(events, vm.Event.Kind)
		mu.Unlock()
	})

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	if ok, reason := view.Available(ctx, "env-demo"); !ok {
		t.Fatalf("demo host unavailable: %s", reason)
	}
	raw, err := view.Call(ctx, "env-demo", coreagent.MsgVMQuery, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	var query coreagent.VMPayload
	if err := json.Unmarshal(raw, &query); err != nil || len(query.States) == 0 {
		t.Fatalf("vm_query = %s, %v", raw, err)
	}
	var target string
	for _, st := range query.States {
		if st.State == "running" {
			target = st.VMID
			break
		}
	}
	if target == "" {
		t.Fatal("the demo host has no running guest")
	}

	for _, op := range []string{coreagent.MsgVMPause, coreagent.MsgVMResume} {
		body, err := json.Marshal(coreagent.VMPayload{VMID: target})
		if err != nil {
			t.Fatal(err)
		}
		reply, err := view.Call(ctx, "env-demo", op, body, nil)
		if err != nil || !strings.Contains(string(reply), `"success":true`) {
			t.Fatalf("%s = %s, %v", op, reply, err)
		}
	}

	_, err = view.Call(ctx, "env-demo", coreagent.MsgVMShutdown, []byte(`{"vmId":"`+target+`"}`), nil)
	if !errors.Is(err, ErrUndeclared) {
		t.Fatalf("undeclared vm_shutdown: got %v", err)
	}

	cts, err := view.Call(ctx, "env-demo", coreagent.MsgCTQuery, []byte(`{"operation":"reconcile"}`), nil)
	if err != nil || !strings.Contains(string(cts), `"states"`) {
		t.Fatalf("ct_query = %s, %v", cts, err)
	}

	waitFor(t, "the pause and resume events", func() bool {
		mu.Lock()
		defer mu.Unlock()
		return strings.Join(events, ",") == "stop,resume"
	})
}
