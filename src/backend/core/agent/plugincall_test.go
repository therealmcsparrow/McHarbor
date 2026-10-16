// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package agent

import (
	"bytes"
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
)

// recordingAgent keeps every raw frame the server writes and answers each request with a
// successful result of the matching family. A vm_clone_request is answered with a clone
// progress frame, a foreign progress frame under the same id, and then the result.
type recordingAgent struct {
	mu     sync.Mutex
	frames [][]byte
}

func (a *recordingAgent) raw() [][]byte {
	a.mu.Lock()
	defer a.mu.Unlock()
	return append([][]byte(nil), a.frames...)
}

func (a *recordingAgent) serve(t *testing.T) *httptest.Server {
	t.Helper()
	upgrader := websocket.Upgrader{}
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			t.Errorf("upgrade: %v", err)
			return
		}
		defer closeIgnoring(conn)
		for {
			_, data, err := conn.ReadMessage()
			if err != nil {
				return
			}
			a.mu.Lock()
			a.frames = append(a.frames, data)
			a.mu.Unlock()
			var msg WSMessage
			if json.Unmarshal(data, &msg) != nil {
				continue
			}
			var replies []WSMessage
			switch {
			case msg.Type == MsgVMCloneRequest:
				replies = []WSMessage{
					{Type: MsgVMCloneProgress, ID: msg.ID, VM: &VMPayload{Percent: 40}},
					{Type: MsgVMMigrateProgress, ID: msg.ID, VM: &VMPayload{Percent: 99}},
					{Type: MsgVMCloneResult, ID: msg.ID, VM: &VMPayload{Success: true, VMID: "clone"}},
				}
			case msg.VM != nil:
				replies = []WSMessage{{Type: MsgVMResult, ID: msg.ID, VM: &VMPayload{Success: true, VMID: msg.VM.VMID}}}
			case msg.CT != nil:
				replies = []WSMessage{{Type: MsgCTResult, ID: msg.ID, CT: &CTPayload{Success: true}}}
			case msg.Net != nil:
				replies = []WSMessage{{Type: MsgNetRemoveResponse, ID: msg.ID, Net: &ComputeNetPayload{Success: true}}}
			}
			for _, reply := range replies {
				if err := conn.WriteJSON(reply); err != nil {
					return
				}
			}
		}
	}))
}

func dialRecordingAgent(t *testing.T, version, build string) (*AgentTransport, *recordingAgent) {
	t.Helper()
	agent := &recordingAgent{}
	srv := agent.serve(t)
	t.Cleanup(srv.Close)
	ws, _, err := dialWS("ws" + strings.TrimPrefix(srv.URL, "http"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { closeIgnoring(ws) })
	conn := &AgentConnection{EnvID: "env-1", Version: version, Build: build, Conn: ws}
	tr := NewAgentTransport(conn, nil, slog.New(slog.NewTextHandler(io.Discard, nil)))
	conn.Transport = tr
	go func() { ignoreErr(tr.ReadLoop()) }()
	return tr, agent
}

// withoutID drops the request id, the one field two sends of the same message differ in.
func withoutID(t *testing.T, frame []byte) []byte {
	t.Helper()
	var id struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(frame, &id); err != nil || id.ID == "" {
		t.Fatalf("frame %s carries no id: %v", frame, err)
	}
	return bytes.Replace(frame, []byte(`"id":"`+id.ID+`"`), []byte(`"id":""`), 1)
}

// TestCallWritesTheSameBytesAsTheTypedCommands pins step 1's promise: routing a plugin's
// payload through Call puts exactly the bytes on the wire the typed VMCommand and
// CTCommand put there, so the agent and the demo simulator cannot tell the paths apart.
func TestCallWritesTheSameBytesAsTheTypedCommands(t *testing.T) {
	tr, agent := dialRecordingAgent(t, "2026.09.01", "209912312359")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	vm := VMPayload{VMID: "vm-1", EnvID: "env-1", GraceSeconds: 30, DeleteVolumes: true}
	vmJSON, err := json.Marshal(vm)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tr.VMCommand(ctx, MsgVMShutdown, vm, 0); err != nil {
		t.Fatal(err)
	}
	if _, err := tr.Call(ctx, "env-1", MsgVMShutdown, vmJSON, nil); err != nil {
		t.Fatal(err)
	}
	ct := CTPayload{ContainerID: "ct-1", EnvID: "env-1"}
	ctJSON, err := json.Marshal(ct)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tr.CTCommand(ctx, MsgCTStart, ct, 0); err != nil {
		t.Fatal(err)
	}
	if _, err := tr.Call(ctx, "env-1", MsgCTStart, ctJSON, nil); err != nil {
		t.Fatal(err)
	}

	frames := agent.raw()
	if len(frames) != 4 {
		t.Fatalf("agent saw %d frames, want 4", len(frames))
	}
	for _, pair := range [][2]int{{0, 1}, {2, 3}} {
		typed, routed := withoutID(t, frames[pair[0]]), withoutID(t, frames[pair[1]])
		if !bytes.Equal(typed, routed) {
			t.Errorf("envelope differs:\n typed  %s\n routed %s", typed, routed)
		}
	}
}

// TestCallDecodesIntoTheOneSlot proves the envelope is host-built: unknown fields and a
// second slot in the plugin's payload never reach the wire, and the environment id is the
// host's, not the plugin's.
func TestCallDecodesIntoTheOneSlot(t *testing.T) {
	tr, agent := dialRecordingAgent(t, "2026.09.01", "209912312359")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	payload := []byte(`{"vmId":"vm-1","envId":"someone-else","bogus":true,"ct":{"containerId":"x"},"type":"exec_start"}`)
	if _, err := tr.Call(ctx, "env-1", MsgVMStart, payload, nil); err != nil {
		t.Fatal(err)
	}
	frames := agent.raw()
	if len(frames) != 1 {
		t.Fatalf("agent saw %d frames, want 1", len(frames))
	}
	var wire map[string]json.RawMessage
	if err := json.Unmarshal(frames[0], &wire); err != nil {
		t.Fatal(err)
	}
	if string(wire["type"]) != `"vm_start"` {
		t.Fatalf("type on the wire = %s", wire["type"])
	}
	if _, ok := wire["ct"]; ok {
		t.Fatal("a second envelope slot from the plugin's payload reached the wire")
	}
	var vm map[string]json.RawMessage
	if err := json.Unmarshal(wire["vm"], &vm); err != nil {
		t.Fatal(err)
	}
	if string(vm["envId"]) != `"env-1"` {
		t.Fatalf("envId on the wire = %s, want the host's env-1", vm["envId"])
	}
	for _, field := range []string{"bogus", "ct", "type"} {
		if _, ok := vm[field]; ok {
			t.Errorf("unknown field %q reached the wire", field)
		}
	}
}

// TestCallRefusesBeforeWriting covers the two refusals that must never touch the socket:
// a type outside the ceiling, and a floor the agent does not clear.
func TestCallRefusesBeforeWriting(t *testing.T) {
	tr, agent := dialRecordingAgent(t, "3.7.0", "")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	for _, msgType := range []string{MsgExecStart, MsgHostTerminalStart, MsgAgentUpdateRequest, MsgComposeRun, MsgHTTPRequest, MsgNetAdoptConfirm, MsgImageCancelRequest, MsgVMResult} {
		if _, err := tr.Call(ctx, "env-1", msgType, []byte(`{}`), nil); !errors.Is(err, ErrNotPluginRoutable) {
			t.Errorf("%s: got %v, want ErrNotPluginRoutable", msgType, err)
		}
	}
	_, err := tr.Call(ctx, "env-1", MsgVMDiskList, nil, nil)
	if !errors.Is(err, ErrComputeUnsupported) || !strings.Contains(err.Error(), "needs agent "+FreeVolumeMinVersion) {
		t.Fatalf("vm_disk_list on 3.7.0: got %v", err)
	}
	if n := len(agent.raw()); n != 0 {
		t.Fatalf("a refused call wrote %d frames to the agent", n)
	}
}

// TestCallHandsOverOnlyDeclaredProgress guards the progress path: a clone's own progress
// frame reaches the caller, a different progress type under the same id does not.
func TestCallHandsOverOnlyDeclaredProgress(t *testing.T) {
	tr, _ := dialRecordingAgent(t, "2026.09.01", "209912312359")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var mu sync.Mutex
	var got []string
	reply, err := tr.Call(ctx, "env-1", MsgVMCloneRequest, []byte(`{"vmId":"vm-1"}`), func(msgType string, frame []byte) {
		mu.Lock()
		got = append(got, msgType+" "+string(frame))
		mu.Unlock()
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(reply), `"vmId":"clone"`) {
		t.Fatalf("reply = %s", reply)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(got) != 1 || !strings.HasPrefix(got[0], MsgVMCloneProgress+" ") || !strings.Contains(got[0], `"percent":40`) {
		t.Fatalf("progress handed over = %q, want exactly the clone's own frame", got)
	}
}

// TestObservableFramesReachThePluginObserver guards the observe table at the read loop:
// an observable frame reaches the observer as its slot's JSON, anything else does not.
func TestObservableFramesReachThePluginObserver(t *testing.T) {
	var mu sync.Mutex
	var seen []string
	SetPluginFrameObserver(func(envID, msgType string, frame []byte) {
		mu.Lock()
		seen = append(seen, envID+"|"+msgType+"|"+string(frame))
		mu.Unlock()
	})
	t.Cleanup(func() { SetPluginFrameObserver(nil) })

	tr := &AgentTransport{conn: &AgentConnection{EnvID: "env-9"}}
	tr.dispatchComputeEvent(&WSMessage{Type: MsgVMEvent, VM: &VMPayload{Event: &VMEvent{VMID: "vm-1", Kind: "shutdown"}}})
	tr.dispatchComputeEvent(&WSMessage{Type: MsgNetLeaseEvent, Net: &ComputeNetPayload{Lease: &ComputeLease{IP: "10.0.0.2"}}})
	tr.dispatchComputeEvent(&WSMessage{Type: MsgVMState, VM: &VMPayload{VMID: "vm-1"}})

	mu.Lock()
	defer mu.Unlock()
	if len(seen) != 2 {
		t.Fatalf("observer saw %d frames, want 2: %q", len(seen), seen)
	}
	if !strings.HasPrefix(seen[0], "env-9|vm_event|{") || !strings.Contains(seen[0], `"kind":"shutdown"`) {
		t.Errorf("vm_event arrived as %q", seen[0])
	}
	if !strings.HasPrefix(seen[1], "env-9|net_lease_event|{") || !strings.Contains(seen[1], `"10.0.0.2"`) {
		t.Errorf("net_lease_event arrived as %q", seen[1])
	}
}
