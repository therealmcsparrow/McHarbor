// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package computesim

import (
	"context"
	"encoding/binary"
	"encoding/json"
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
	"github.com/therealmcsparrow/mcharbor/core/demo/estate"
	"github.com/therealmcsparrow/mcharbor/core/demo/fakeagent"
)

// connect runs the real server-side handshake, AgentTransport and ReadLoop
// against a fake agent carrying the simulator, the way demo mode wires them.
func connect(t *testing.T, hostKey string) (*coreagent.AgentTransport, *Sim, *estate.Host) {
	t.Helper()
	ready := make(chan *coreagent.AgentTransport, 1)
	up := websocket.Upgrader{}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := up.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		var auth coreagent.WSMessage
		if err := conn.ReadJSON(&auth); err != nil || auth.Auth == nil {
			t.Errorf("first frame = %+v, %v", auth, err)
			return
		}
		if err := conn.WriteJSON(coreagent.WSMessage{Type: coreagent.MsgAuthResult, AuthResult: &coreagent.AuthResultPayload{Success: true, EnvID: "env-demo"}}); err != nil {
			return
		}
		tr := coreagent.NewAgentTransport(&coreagent.AgentConnection{EnvID: "env-demo", Version: auth.Auth.AgentVersion, Build: auth.Auth.AgentBuild, Capabilities: auth.Auth.Capabilities, Conn: conn}, nil, logger)
		ready <- tr
		if err := tr.ReadLoopContext(r.Context()); err != nil {
			t.Log(err)
		}
	}))
	t.Cleanup(srv.Close)

	e := estate.Build(estate.DefaultSeed, time.Now())
	sim := New(e, 7, time.Now())
	host := e.Host(hostKey)
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	a := fakeagent.New(fakeagent.Config{
		Host: host, Token: "agt_test", Version: "2026.09.01", Build: "209901010000",
		Logger:     logger,
		Extensions: []fakeagent.Extension{sim},
		Dial: func(ctx context.Context, token string) (*websocket.Conn, error) {
			conn, resp, err := websocket.DefaultDialer.DialContext(ctx, "ws"+strings.TrimPrefix(srv.URL, "http")+"/?token="+token, nil)
			if resp != nil && resp.Body != nil {
				if cerr := resp.Body.Close(); cerr != nil {
					t.Log(cerr)
				}
			}
			return conn, err
		},
	})
	go a.Run(ctx)
	select {
	case tr := <-ready:
		return tr, sim, host
	case <-time.After(5 * time.Second):
		t.Fatal("demo agent never completed the handshake")
	}
	return nil, nil, nil
}

type eventSink struct {
	mu     sync.Mutex
	events []coreagent.VMEvent
}

// observe is the plugin frame observer: every unsolicited frame a plugin may subscribe to
// arrives here as its slot's JSON, which is all a guest plugin ever sees of one.
func (s *eventSink) observe(_, msgType string, frame []byte) {
	if msgType != coreagent.MsgVMEvent {
		return
	}
	var payload coreagent.VMPayload
	if json.Unmarshal(frame, &payload) != nil || payload.Event == nil {
		return
	}
	s.mu.Lock()
	s.events = append(s.events, *payload.Event)
	s.mu.Unlock()
}

func (s *eventSink) kinds() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]string, 0, len(s.events))
	for _, ev := range s.events {
		out = append(out, ev.Kind)
	}
	return out
}

func TestComputeThroughTheRealAgentTransport(t *testing.T) {
	sink := &eventSink{}
	coreagent.SetPluginFrameObserver(sink.observe)
	t.Cleanup(func() { coreagent.SetPluginFrameObserver(nil) })

	tr, sim, _ := connect(t, "prod-eu-1")
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	capability, err := tr.ComputeProbe(ctx, "env-demo")
	if err != nil || capability.HostCPUs == 0 || !capability.CanRunVMs || !capability.CanRunContainers || len(capability.QEMUBinaries) == 0 {
		t.Fatalf("probe = %+v, %v", capability, err)
	}
	backends, err := tr.PoolDetect(ctx, "env-demo")
	if err != nil || len(backends) == 0 {
		t.Fatalf("pool detect = %+v, %v", backends, err)
	}
	hs := sim.hosts["prod-eu-1"]
	stats, err := tr.PoolStats(ctx, coreagent.ComputePoolPayload{PoolID: hs.pools[0].ID, Backend: hs.pools[0].Backend, Target: hs.pools[0].Target})
	if err != nil || stats.CapacityBytes == 0 || stats.VolumeCount == 0 || stats.UsedBytes > stats.CapacityBytes {
		t.Fatalf("pool stats = %+v, %v", stats, err)
	}
	links, err := tr.NetDetect(ctx, "env-demo")
	if err != nil || len(links) < 3 {
		t.Fatalf("net detect = %+v, %v", links, err)
	}

	query, err := tr.VMCommand(ctx, coreagent.MsgVMQuery, coreagent.VMPayload{}, 0)
	if err != nil || len(query.States) != len(hs.vms) {
		t.Fatalf("vm_query returned %d states (%v), want %d", len(query.States), err, len(hs.vms))
	}
	var target string
	for _, st := range query.States {
		if st.State == stateRunning {
			target = st.VMID
			break
		}
	}
	stats2, err := tr.VMCommand(ctx, coreagent.MsgVMStats, coreagent.VMPayload{VMID: target}, 0)
	if err != nil || stats2.Stats == nil || stats2.Stats.MemoryMaxBytes == 0 {
		t.Fatalf("vm_stats = %+v, %v", stats2, err)
	}
	guest, err := tr.VMCommand(ctx, coreagent.MsgVMGuestQuery, coreagent.VMPayload{VMID: target, GuestQuery: "network"}, 0)
	if err != nil || guest.Guest == nil {
		t.Fatalf("vm_guest_query = %+v, %v", guest, err)
	}

	for _, op := range []string{coreagent.MsgVMPause, coreagent.MsgVMResume, coreagent.MsgVMShutdown} {
		reply, err := tr.VMCommand(ctx, op, coreagent.VMPayload{VMID: target}, 0)
		if err != nil || !reply.Success {
			t.Fatalf("%s = %+v, %v", op, reply, err)
		}
	}
	after, err := tr.VMCommand(ctx, coreagent.MsgVMQuery, coreagent.VMPayload{States: []coreagent.VMState{{VMID: target}}}, 0)
	if err != nil || len(after.States) != 1 || after.States[0].State != stateStopped {
		t.Fatalf("after shutdown vm_query = %+v, %v", after, err)
	}
	if _, err := tr.VMCommand(ctx, coreagent.MsgVMStats, coreagent.VMPayload{VMID: target}, 0); err == nil {
		t.Fatal("vm_stats on a stopped guest succeeded")
	}
	snap, err := tr.VMCommand(ctx, coreagent.MsgVMSnapshotCreate, coreagent.VMPayload{VMID: target, SnapshotName: "e2e"}, 0)
	if err != nil || snap.SnapshotName != "e2e" {
		t.Fatalf("snapshot create = %+v, %v", snap, err)
	}
	refused, err := tr.VMCommand(ctx, coreagent.MsgVMImportInspect, coreagent.VMPayload{}, 0)
	if err == nil || refused == nil || refused.Error != ErrDemoRefused {
		t.Fatalf("vm_import_inspect = %+v, %v; want the demo refusal", refused, err)
	}
	// Every migration phase and the remote-viewer ticket answer with the demo
	// refusal rather than silence, which would hang the caller for hours.
	for _, op := range []string{coreagent.MsgVMSpiceTicket, coreagent.MsgVMMigrate} {
		refused, err := tr.VMCommand(ctx, op, coreagent.VMPayload{VMID: target,
			Migrate: &coreagent.VMMigrateSpec{Phase: coreagent.VMMigratePhaseLiveCheck}}, 5*time.Second)
		if err == nil || refused == nil || refused.Error != ErrDemoRefused {
			t.Fatalf("%s = %+v, %v; want the demo refusal", op, refused, err)
		}
	}
	clone, err := tr.VMCommand(ctx, coreagent.MsgVMCloneRequest, coreagent.VMPayload{VMID: target, Clone: &coreagent.VMCloneSpec{TargetVMID: "clone-e2e"}}, 0)
	if err != nil || !clone.Success {
		t.Fatalf("clone = %+v, %v", clone, err)
	}

	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) && strings.Join(sink.kinds(), ",") != "stop,resume,powerdown" {
		time.Sleep(20 * time.Millisecond)
	}
	if got := strings.Join(sink.kinds(), ","); got != "stop,resume,powerdown" {
		t.Fatalf("vm events = %q, want stop,resume,powerdown", got)
	}

	cts, err := tr.CTCommand(ctx, coreagent.MsgCTQuery, coreagent.CTPayload{Operation: "reconcile"}, 0)
	if err != nil || len(cts.States) != len(hs.cts) {
		t.Fatalf("ct reconcile returned %d states (%v), want %d", len(cts.States), err, len(hs.cts))
	}
	var ctID string
	for _, st := range cts.States {
		if st.State == stateRunning {
			ctID = st.ContainerID
			break
		}
	}
	frozen, err := tr.CTCommand(ctx, coreagent.MsgCTFreeze, coreagent.CTPayload{ContainerID: ctID}, 0)
	if err != nil || frozen.State == nil || frozen.State.State != stateFrozen {
		t.Fatalf("ct_freeze = %+v, %v", frozen, err)
	}
	ctStats, err := tr.CTCommand(ctx, coreagent.MsgCTQuery, coreagent.CTPayload{Operation: "stats", ContainerID: ctID}, 0)
	if err != nil || ctStats.Stats == nil || ctStats.Stats.MemoryMaxBytes == 0 {
		t.Fatalf("ct stats = %+v, %v", ctStats, err)
	}
	backup, err := tr.CTCommand(ctx, coreagent.MsgCTBackupRun, coreagent.CTPayload{ContainerID: ctID}, 0)
	if err == nil || backup.Error != ErrDemoRefused {
		t.Fatalf("ct_backup_run = %+v, %v; want the demo refusal", backup, err)
	}

	var progress int
	img, err := tr.ImageFetch(ctx, coreagent.ComputeImagePayload{ImageID: "img-rocky-9", Kind: "cloud_image"}, func(*coreagent.ComputeImagePayload) { progress++ })
	if err != nil || !img.Verified || img.LocalPath == "" {
		t.Fatalf("image fetch = %+v, %v", img, err)
	}
	cached, err := tr.ImageList(ctx, "env-demo")
	if err != nil || len(cached) != 5 {
		t.Fatalf("image list = %d, %v; want the 4 seeded plus the fetched one", len(cached), err)
	}
}

func readConsole(t *testing.T, s *coreagent.ConsoleSession, until func([]byte) bool) []byte {
	t.Helper()
	var got []byte
	deadline := time.After(5 * time.Second)
	for !until(got) {
		select {
		case data := <-s.OutputCh:
			got = append(got, data...)
		case <-s.Done():
			t.Fatalf("console ended: %s", s.Err())
		case <-deadline:
			t.Fatalf("console produced %q before the deadline", got)
		}
	}
	return got
}

func TestConsolesThroughTheRealAgentTransport(t *testing.T) {
	tr, sim, _ := connect(t, "homelab")
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	hs := sim.hosts["homelab"]
	var linux, stopped string
	for _, id := range hs.vmOrder {
		g := hs.vms[id]
		if g.State == stateRunning && g.OS.family == "linux" && linux == "" {
			linux = id
		}
		if g.State == stateStopped && stopped == "" {
			stopped = id
		}
	}

	serial, err := tr.StartConsole(ctx, coreagent.ConsoleKindSerial, "vm", linux, nil, 80, 24)
	if err != nil {
		t.Fatal(err)
	}
	readConsole(t, serial, func(b []byte) bool { return strings.Contains(string(b), "login: ") })
	if err := serial.Send([]byte("root\r")); err != nil {
		t.Fatal(err)
	}
	readConsole(t, serial, func(b []byte) bool { return strings.Contains(string(b), "Password: ") })
	if err := serial.Send([]byte("x\rhostname\r")); err != nil {
		t.Fatal(err)
	}
	readConsole(t, serial, func(b []byte) bool { return strings.Contains(string(b), hs.vms[linux].Name+"\r\n") })
	if err := serial.Close(); err != nil {
		t.Fatal(err)
	}

	refused, err := tr.StartConsole(ctx, coreagent.ConsoleKindSerial, "vm", stopped, nil, 80, 24)
	if err != nil {
		t.Fatal(err)
	}
	select {
	case <-refused.Done():
		if !strings.Contains(refused.Err(), "not running") {
			t.Fatalf("console on a stopped guest ended with %q", refused.Err())
		}
	case <-time.After(5 * time.Second):
		t.Fatal("console on a stopped guest never ended")
	}

	vnc, err := tr.StartConsole(ctx, coreagent.ConsoleKindVNC, "vm", linux, nil, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	if got := readConsole(t, vnc, func(b []byte) bool { return len(b) >= 12 }); string(got[:12]) != "RFB 003.008\n" {
		t.Fatalf("banner = %q", got)
	}
	if err := vnc.Send([]byte("RFB 003.008\n")); err != nil {
		t.Fatal(err)
	}
	if got := readConsole(t, vnc, func(b []byte) bool { return len(b) >= 2 }); got[0] != 1 || got[1] != 1 {
		t.Fatalf("security types = %v, want [1 1]", got)
	}
	if err := vnc.Send([]byte{1}); err != nil {
		t.Fatal(err)
	}
	if got := readConsole(t, vnc, func(b []byte) bool { return len(b) >= 4 }); binary.BigEndian.Uint32(got) != 0 {
		t.Fatalf("security result = %v", got)
	}
	if err := vnc.Send([]byte{1}); err != nil {
		t.Fatal(err)
	}
	init := readConsole(t, vnc, func(b []byte) bool {
		return len(b) >= 24 && len(b) >= 24+int(binary.BigEndian.Uint32(b[20:24]))
	})
	if w, h := binary.BigEndian.Uint16(init[0:2]), binary.BigEndian.Uint16(init[2:4]); w != screenWidth || h != screenHeight {
		t.Fatalf("server init %dx%d", w, h)
	}
	if name := string(init[24:]); !strings.Contains(name, hs.vms[linux].Name) {
		t.Fatalf("desktop name = %q", name)
	}

	bgr := pixelFormat{bpp: 32, depth: 24, trueColour: true, rMax: 255, gMax: 255, bMax: 255, rShift: 0, gShift: 8, bShift: 16}
	setFormat := append([]byte{0, 0, 0, 0}, bgr.bytes()...)
	encodings := []byte{2, 0, 0, 2, 0, 0, 0, 0, 0xff, 0xff, 0xff, 0x21}
	request := []byte{3, 0, 0, 0, 0, 0, 0x04, 0x00, 0x03, 0x00}
	if err := vnc.Send(append(append(setFormat, encodings...), request...)); err != nil {
		t.Fatal(err)
	}
	want := 16 + screenWidth*screenHeight*4
	update := readConsole(t, vnc, func(b []byte) bool { return len(b) >= want })
	if update[0] != 0 || binary.BigEndian.Uint16(update[2:4]) != 1 {
		t.Fatalf("update header = %v", update[:4])
	}
	if w, h := binary.BigEndian.Uint16(update[8:10]), binary.BigEndian.Uint16(update[10:12]); int(w) != screenWidth || int(h) != screenHeight {
		t.Fatalf("rectangle %dx%d", w, h)
	}
	first := update[16:20]
	if first[0] != colHeader>>16 || first[1] != (colHeader>>8)&0xff || first[2] != colHeader&0xff {
		t.Fatalf("top-left pixel %x is not the header colour in the client's red-first format", first)
	}

	if err := vnc.Send([]byte{3, 1, 0, 0, 0, 0, 0x04, 0x00, 0x03, 0x00, 4, 1, 0, 0, 0, 0, 0, 'r'}); err != nil {
		t.Fatal(err)
	}
	partial := readConsole(t, vnc, func(b []byte) bool { return len(b) >= 16 })
	if h := binary.BigEndian.Uint16(partial[10:12]); int(h) != 3*lineHeight {
		t.Fatalf("a keystroke repainted %d rows, want only the login band", h)
	}
	if err := vnc.Close(); err != nil {
		t.Fatal(err)
	}
}
