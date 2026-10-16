// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"slices"
	"sort"
	"strings"
	"testing"
	"time"
)

// agentWireDir is the golden fixture set the host generates from its agent protocol structs
// (slots) and from the conversions it performed before this plugin owned them (converted).
// This plugin reads them as data and never links the host structs.
var agentWireDir = filepath.Join("..", "..", "..", "..", "contracts", "agentwire")

func readFixture(t *testing.T, parts ...string) []byte {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(append([]string{agentWireDir}, parts...)...))
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

// declaredAgentRPC reads this plugin's agent-rpc send and observe lists out of plugin.yaml.
// A line scanner rather than a YAML library, so the test adds no dependency to the module.
func declaredAgentRPC(t *testing.T) (send, observe []string) {
	t.Helper()
	f, err := os.Open(filepath.Join("..", "plugin.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := f.Close(); err != nil {
			t.Log(err)
		}
	}()
	var in bool
	var list *[]string
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		trimmed := strings.TrimSpace(scanner.Text())
		switch {
		case trimmed == "- kind: agent-rpc":
			in = true
		case in && strings.HasPrefix(trimmed, "- kind:"):
			in = false
		case in && trimmed == "send:":
			list = &send
		case in && trimmed == "observe:":
			list = &observe
		case in && list != nil && strings.HasPrefix(trimmed, "- "):
			*list = append(*list, strings.TrimSpace(strings.TrimPrefix(trimmed, "- ")))
		}
	}
	if err := scanner.Err(); err != nil {
		t.Fatal(err)
	}
	return send, observe
}

// sourceAgentTypes reads every agent message type the plugin's own source names as a
// msg* string constant, so a new command cannot be added without the declaration noticing.
func sourceAgentTypes(t *testing.T) []string {
	t.Helper()
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	pattern := regexp.MustCompile(`(?m)^\s*(?:const\s+)?msg[A-Za-z]+\s*=\s*"((?:vm|host_device|compute|ct|net|pool)_[a-z_]+)"`)
	seen := map[string]bool{}
	for _, file := range files {
		if strings.HasSuffix(file, "_test.go") {
			continue
		}
		raw, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		for _, match := range pattern.FindAllStringSubmatch(string(raw), -1) {
			seen[match[1]] = true
		}
	}
	out := make([]string, 0, len(seen))
	for msgType := range seen {
		out = append(out, msgType)
	}
	sort.Strings(out)
	return out
}

// TestManifestDeclaresExactlyWhatThePluginSends keeps plugin.yaml's agent-rpc list honest in
// both directions: nothing sent is undeclared (the host refuses it), and nothing declared is
// unused (a declaration is a grant, and an unused grant is reach nobody reviewed). The
// observed frames are checked the same way, and the source is scanned so a new command
// constant cannot skip the list.
func TestManifestDeclaresExactlyWhatThePluginSends(t *testing.T) {
	send, observe := declaredAgentRPC(t)
	sort.Strings(send)
	sent := slices.Clone(sentMessageTypes)
	sort.Strings(sent)
	if !slices.Equal(send, sent) {
		t.Errorf("agent-rpc send = %v, the plugin sends %v", send, sent)
	}
	sort.Strings(observe)
	observed := slices.Clone(observedMessageTypes)
	sort.Strings(observed)
	if !slices.Equal(observe, observed) {
		t.Errorf("agent-rpc observe = %v, the plugin handles %v", observe, observed)
	}
	all := append(slices.Clone(sent), observed...)
	sort.Strings(all)
	if source := sourceAgentTypes(t); !slices.Equal(source, all) {
		t.Errorf("message constants in the source = %v, declared = %v", source, all)
	}
}

// TestWireStructsDecodeEveryFieldTheyHold holds each local wire struct to the host's slot
// fixture: the fixture sets every protocol field, so a field of ours that decodes to zero has
// a tag the agent does not send.
func TestWireStructsDecodeEveryFieldTheyHold(t *testing.T) {
	cases := []struct {
		slot string
		into any
	}{
		{"compute", &agentComputePayload{}},
		{"vm", &agentVMFrame{}},
	}
	for _, tc := range cases {
		if err := json.Unmarshal(readFixture(t, "slots", tc.slot+".json"), tc.into); err != nil {
			t.Fatalf("%s: %v", tc.slot, err)
		}
		var zero []string
		zeroFields(reflect.ValueOf(tc.into).Elem(), reflect.TypeOf(tc.into).Elem().Name(), &zero)
		if len(zero) > 0 {
			t.Errorf("%T decodes these fields to zero from the %s fixture: %v", tc.into, tc.slot, zero)
		}
	}
}

func zeroFields(v reflect.Value, path string, zero *[]string) {
	if v.IsZero() {
		*zero = append(*zero, path)
		return
	}
	switch v.Kind() {
	case reflect.Pointer:
		zeroFields(v.Elem(), path, zero)
	case reflect.Slice:
		if v.Type().Elem().Kind() != reflect.Uint8 {
			zeroFields(v.Index(0), path+"[0]", zero)
		}
	case reflect.Struct:
		for i := range v.NumField() {
			if f := v.Type().Field(i); f.IsExported() {
				zeroFields(v.Field(i), path+"."+f.Name, zero)
			}
		}
	}
}

// agentFake is a bare agentRPC that answers each message type from a table.
type agentFake struct {
	reply map[string][]byte
	err   error
	calls []string
	sink  func(envID, msgType string, frame []byte)
}

func (f *agentFake) Available(context.Context, string) (bool, string) { return true, "" }

func (f *agentFake) Call(_ context.Context, _, msgType string, _ []byte, _ func(string, []byte)) ([]byte, error) {
	f.calls = append(f.calls, msgType)
	return f.reply[msgType], f.err
}

func (f *agentFake) Subscribe(sink func(envID, msgType string, frame []byte)) { f.sink = sink }

// TestProbeMatchesTheHostConversion feeds the host's compute slot fixture through Probe and
// holds what this plugin keeps to the host's converted capability golden, which is what the
// typed Probe returned before the conversion moved here.
func TestProbeMatchesTheHostConversion(t *testing.T) {
	var golden struct {
		CanRunVMs    bool `json:"canRunVms"`
		QEMUBinaries []struct {
			MachineTypes []string `json:"machineTypes"`
			KVMAccel     bool     `json:"kvmAccel"`
		} `json:"qemuBinaries"`
	}
	if err := json.Unmarshal(readFixture(t, "converted", "capability.json"), &golden); err != nil {
		t.Fatal(err)
	}
	fake := &agentFake{reply: map[string][]byte{msgComputeProbe: readFixture(t, "slots", "compute.json")}}
	got, err := newAgentBridge(fake).Probe(context.Background(), "env1")
	if err != nil {
		t.Fatal(err)
	}
	if got.CanRunVMs != golden.CanRunVMs || len(got.QEMUBinaries) != len(golden.QEMUBinaries) || len(got.QEMUBinaries) == 0 {
		t.Fatalf("probe = %+v, golden = %+v", got, golden)
	}
	for i, binary := range golden.QEMUBinaries {
		if got.QEMUBinaries[i].KVMAccel != binary.KVMAccel || !slices.Equal(got.QEMUBinaries[i].MachineTypes, binary.MachineTypes) {
			t.Errorf("binary %d = %+v, golden = %+v", i, got.QEMUBinaries[i], binary)
		}
	}
	if !got.CanRunVMs || !got.QEMUBinaries[0].KVMAccel {
		t.Fatal("the fixture sets both flags, so the conversion dropped one")
	}
}

// TestProbeFailures covers the two ways a probe can not answer: the call fails, or the agent
// replies with no report.
func TestProbeFailures(t *testing.T) {
	boom := errors.New("agent failed compute_probe_request: boom")
	if _, err := newAgentBridge(&agentFake{err: boom}).Probe(context.Background(), "env1"); !errors.Is(err, boom) {
		t.Fatalf("a failed call = %v, want the agent's error", err)
	}
	empty := &agentFake{reply: map[string][]byte{msgComputeProbe: []byte(`{}`)}}
	if _, err := newAgentBridge(empty).Probe(context.Background(), "env1"); err == nil {
		t.Fatal("a reply with no capability report must be an error")
	}
	garbled := &agentFake{reply: map[string][]byte{msgComputeProbe: []byte(`not json`)}}
	if _, err := newAgentBridge(garbled).Probe(context.Background(), "env1"); err == nil {
		t.Fatal("an undecodable reply must be an error")
	}
}

// TestGuestEventConversionMatchesTheHost pins the rules of the host's HandleVMEvent: the
// guest kind is vm, the agent's RFC 3339 timestamp wins, and a missing or malformed one
// resolves to the receive time; a frame with no event is not an event.
func TestGuestEventConversionMatchesTheHost(t *testing.T) {
	now := time.Date(2026, 9, 30, 12, 0, 0, 0, time.UTC)
	frame := func(timestamp string) []byte {
		return fmt.Appendf(nil, `{"event":{"vmId":"vm1","kind":"shutdown","device":"virtio0","detail":{"reason":"guest-shutdown"},"timestamp":%q}}`, timestamp)
	}
	cases := []struct {
		name string
		in   []byte
		ok   bool
		at   time.Time
	}{
		{"agent timestamp", frame("2026-09-30T11:59:00Z"), true, time.Date(2026, 9, 30, 11, 59, 0, 0, time.UTC)},
		{"no timestamp", frame(""), true, now},
		{"malformed timestamp", frame("Timestamp"), true, now},
		{"no event", []byte(`{"percent":4}`), false, time.Time{}},
		{"not json", []byte(`x`), false, time.Time{}},
	}
	for _, tc := range cases {
		got, ok := guestEvent("env1", tc.in, now)
		if ok != tc.ok {
			t.Fatalf("%s: ok = %v, want %v", tc.name, ok, tc.ok)
		}
		if !ok {
			continue
		}
		want := GuestEvent{
			EnvID: "env1", GuestKind: "vm", GuestID: "vm1", Kind: "shutdown", Device: "virtio0",
			Detail: map[string]string{"reason": "guest-shutdown"}, At: tc.at,
		}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("%s: event = %+v, want %+v", tc.name, got, want)
		}
	}
}

// TestSubscribeRoutesOnlyWhatItObserves holds the dispatch: vm_event reaches the sink,
// vm_import_progress reaches the table, anything else is dropped, and Unsubscribe clears the
// host's sink.
func TestSubscribeRoutesOnlyWhatItObserves(t *testing.T) {
	fake := &agentFake{}
	bridge := newAgentBridge(fake)
	var events []GuestEvent
	bridge.Subscribe(func(e GuestEvent) { events = append(events, e) })
	if fake.sink == nil {
		t.Fatal("Subscribe installed no sink")
	}

	fake.sink("env1", msgVMEvent, []byte(`{"event":{"vmId":"vm1","kind":"stop"}}`))
	fake.sink("env1", "net_lease_event", []byte(`{"event":{"vmId":"vm2","kind":"stop"}}`))
	fake.sink("env1", msgVMImportProgress, []byte(`{"operation":"vm_import_inspect","percent":30,"import":{"importId":"imp1"}}`))
	if len(events) != 1 || events[0].GuestID != "vm1" || events[0].Kind != "stop" {
		t.Fatalf("events = %+v, want only the vm_event", events)
	}
	if phase, _, percent, ok := bridge.ImportProgress("env1", "imp1"); !ok || phase != importPhasePull || percent != 30 {
		t.Fatalf("import progress = %q %d%% %v", phase, percent, ok)
	}

	bridge.Unsubscribe()
	if fake.sink != nil {
		t.Fatal("Unsubscribe left the sink installed")
	}
}

// TestImportProgressMatchesTheHostTable pins the rules of the host's HandleImportProgress:
// the phase comes from the operation (pull for inspect and stage), the disk from the first
// result disk, the percentage is clamped, and a frame with no id or no phase is ignored.
func TestImportProgressMatchesTheHostTable(t *testing.T) {
	now := time.Now()
	cases := []struct {
		name    string
		frame   string
		phase   string
		disk    int
		percent int
		known   bool
	}{
		{"inspect is a pull", `{"operation":"vm_import_inspect","percent":40,"import":{"importId":"i"}}`, "pull", 0, 40, true},
		{"stage is a pull", `{"operation":"vm_import_stage","percent":10,"import":{"importId":"i"}}`, "pull", 0, 10, true},
		{"convert reads the disk", `{"operation":"vm_import_convert","percent":60,"importResult":{"importId":"i","disks":[{"index":2}]}}`, "convert", 2, 60, true},
		{"over 100 clamps", `{"operation":"vm_import_convert","percent":250,"import":{"importId":"i"}}`, "convert", 0, 100, true},
		{"negative clamps", `{"operation":"vm_import_convert","percent":-5,"import":{"importId":"i"}}`, "convert", 0, 0, true},
		{"unknown operation", `{"operation":"vm_start","percent":40,"import":{"importId":"i"}}`, "", 0, 0, false},
		{"no id", `{"operation":"vm_import_convert","percent":40}`, "", 0, 0, false},
		{"not json", `x`, "", 0, 0, false},
	}
	for _, tc := range cases {
		bridge := newAgentBridge(&agentFake{})
		bridge.recordImportProgress("env1", []byte(tc.frame), now)
		phase, disk, percent, known := bridge.ImportProgress("env1", "i")
		if known != tc.known || phase != tc.phase || disk != tc.disk || percent != tc.percent {
			t.Errorf("%s: got %q disk %d %d%% known=%v, want %q disk %d %d%% known=%v",
				tc.name, phase, disk, percent, known, tc.phase, tc.disk, tc.percent, tc.known)
		}
	}
}

// TestImportProgressExpiresAndIsBounded covers the two ways the table is kept from leaking:
// an entry older than the TTL is gone, and the table never grows past its limit.
func TestImportProgressExpiresAndIsBounded(t *testing.T) {
	bridge := newAgentBridge(&agentFake{})
	old := time.Now().Add(-importProgressTTL - time.Minute)
	bridge.recordImportProgress("env1", []byte(`{"operation":"vm_import_convert","percent":9,"import":{"importId":"stale"}}`), old)
	if _, _, _, ok := bridge.ImportProgress("env1", "stale"); ok {
		t.Fatal("an entry past the TTL was still served")
	}

	for i := range importProgressLimit + 40 {
		frame := fmt.Sprintf(`{"operation":"vm_import_convert","percent":1,"import":{"importId":"imp%d"}}`, i)
		bridge.recordImportProgress("env1", []byte(frame), time.Now().Add(time.Duration(i)*time.Millisecond))
	}
	bridge.importMu.Lock()
	size := len(bridge.imports)
	bridge.importMu.Unlock()
	if size > importProgressLimit {
		t.Fatalf("the table grew to %d entries, limit %d", size, importProgressLimit)
	}
	if _, _, _, ok := bridge.ImportProgress("env1", "imp0"); ok {
		t.Fatal("the oldest entry should have been evicted first")
	}
	last := fmt.Sprintf("imp%d", importProgressLimit+39)
	if _, _, _, ok := bridge.ImportProgress("env1", last); !ok {
		t.Fatal("the newest entry was evicted")
	}
}

// TestImportCommandsClearTheirProgress guards the retry: a run of an import must not start
// from the last run's percentage, so an import command clears its entry before the call and
// after it, and any other command leaves the table alone.
func TestImportCommandsClearTheirProgress(t *testing.T) {
	fake := &agentFake{}
	bridge := newAgentBridge(fake)
	seed := func() {
		bridge.recordImportProgress("env1", []byte(`{"operation":"vm_import_convert","percent":90,"import":{"importId":"imp1"}}`), time.Now())
	}

	seed()
	if _, err := bridge.send(context.Background(), "env1", msgVMStart, []byte(`{"import":{"importId":"imp1"}}`)); err != nil {
		t.Fatal(err)
	}
	if _, _, _, ok := bridge.ImportProgress("env1", "imp1"); !ok {
		t.Fatal("a vm_start cleared an import's progress")
	}

	var during bool
	probe := &duringFake{agentFake: fake, look: func() {
		_, _, _, during = bridge.ImportProgress("env1", "imp1")
	}}
	bridge.rpc = probe
	for _, msgType := range []string{msgVMImportInspect, msgVMImportConvert, msgVMImportStage} {
		probe.look = func() { _, _, _, during = bridge.ImportProgress("env1", "imp1") }
		seed()
		during = true
		if _, err := bridge.send(context.Background(), "env1", msgType, []byte(`{"import":{"importId":"imp1"}}`)); err != nil {
			t.Fatal(err)
		}
		if during {
			t.Errorf("%s: the old percentage was still there when the call started", msgType)
		}
		probe.look = seed
		if _, err := bridge.send(context.Background(), "env1", msgType, []byte(`{"import":{"importId":"imp1"}}`)); err != nil {
			t.Fatal(err)
		}
		if _, _, _, ok := bridge.ImportProgress("env1", "imp1"); ok {
			t.Errorf("%s: progress survived the end of the call", msgType)
		}
	}
}

// duringFake runs look at the moment the agent is called.
type duringFake struct {
	*agentFake
	look func()
}

func (d *duringFake) Call(ctx context.Context, envID, msgType string, payload []byte, onProgress func(string, []byte)) ([]byte, error) {
	d.look()
	return d.agentFake.Call(ctx, envID, msgType, payload, onProgress)
}

// TestBridgeNeedsTheAgentRPCHandle covers the assertion: no handle, or one without Call and
// Subscribe (a host predating agent-rpc), leaves the plugin read-only rather than half-wired.
func TestBridgeNeedsTheAgentRPCHandle(t *testing.T) {
	if newAgentBridge(nil) != nil {
		t.Fatal("a nil handle produced a bridge")
	}
	var typedOnly struct{ Available func() }
	if newAgentBridge(typedOnly) != nil {
		t.Fatal("a handle without Call and Subscribe produced a bridge")
	}
	if newAgentBridge(&agentFake{}) == nil {
		t.Fatal("an agent-rpc handle produced no bridge")
	}
}

// TestSendPassesTheReplyAndTheError keeps the guest command path a plain carrier: the reply
// slot comes back untouched and an agent error is not reworded.
func TestSendPassesTheReplyAndTheError(t *testing.T) {
	boom := errors.New("agent failed vm_start: no such guest")
	fake := &agentFake{reply: map[string][]byte{msgVMStart: []byte(`{"success":true}`)}}
	reply, err := newAgentBridge(fake).send(context.Background(), "env1", msgVMStart, []byte(`{}`))
	if err != nil || string(reply) != `{"success":true}` {
		t.Fatalf("send = %s, %v", reply, err)
	}
	fake.err = boom
	if _, err := newAgentBridge(fake).send(context.Background(), "env1", msgVMStart, nil); !errors.Is(err, boom) {
		t.Fatalf("send error = %v, want the agent's", err)
	}
}

// TestProbeKeepsAnUnacceleratedBinaryUnaccelerated guards the flag the machine-type pick
// depends on: a QEMU without KVM must not read as accelerated after conversion.
func TestProbeKeepsAnUnacceleratedBinaryUnaccelerated(t *testing.T) {
	reply := `{"capability":{"canRunVms":false,"qemuBinaries":[{"kvmAccel":false,"machineTypes":["pc-q35-8.2"]},{"kvmAccel":true,"machineTypes":["pc-q35-9.0"]}]}}`
	fake := &agentFake{reply: map[string][]byte{msgComputeProbe: []byte(reply)}}
	got, err := newAgentBridge(fake).Probe(context.Background(), "env1")
	if err != nil {
		t.Fatal(err)
	}
	if got.CanRunVMs || len(got.QEMUBinaries) != 2 || got.QEMUBinaries[0].KVMAccel || !got.QEMUBinaries[1].KVMAccel {
		t.Fatalf("probe = %+v", got)
	}
	if pickMachineType(got) != "pc-q35-9.0" {
		t.Fatalf("machine type = %q, want the accelerated binary's", pickMachineType(got))
	}
}

// TestRecordingPrunesWhatWentQuiet holds the sweep that ends an entry no call will clear.
func TestRecordingPrunesWhatWentQuiet(t *testing.T) {
	bridge := newAgentBridge(&agentFake{})
	old := time.Now().Add(-importProgressTTL - time.Minute)
	bridge.recordImportProgress("env1", []byte(`{"operation":"vm_import_convert","percent":9,"import":{"importId":"stale"}}`), old)
	bridge.recordImportProgress("env1", []byte(`{"operation":"vm_import_convert","percent":9,"import":{"importId":"fresh"}}`), time.Now())
	bridge.importMu.Lock()
	defer bridge.importMu.Unlock()
	if _, kept := bridge.imports[importProgressKey("env1", "stale")]; kept || len(bridge.imports) != 1 {
		t.Fatalf("table = %v, want only the fresh entry", bridge.imports)
	}
}
