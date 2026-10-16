// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"sort"
	"strings"
	"testing"
	"time"
)

// agentWireDir is the golden fixture set the host generates from its agent protocol structs
// (slots) and from the conversions it performed before this plugin owned them (converted,
// requests). This plugin reads them as data and never links the host structs.
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
		line := scanner.Text()
		trimmed := strings.TrimSpace(line)
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
			*list = append(*list, strings.TrimPrefix(trimmed, "- "))
		}
	}
	if len(send) == 0 {
		t.Fatal("plugin.yaml declares no agent-rpc send list")
	}
	return send, observe
}

// fakeAgentRPC answers every call with the slot fixture for the message's family, refuses
// anything plugin.yaml does not declare (as the host's view does), and records each payload.
type fakeAgentRPC struct {
	t        *testing.T
	declared map[string]bool
	payloads map[string][]byte
	replyErr error
	sink     func(envID, msgType string, frame []byte)
}

var slotFor = map[string]string{
	msgComputeProbe: "compute", msgComputeConfig: "compute", msgComputeMigrate: "compute", msgComputeInstall: "compute",
	msgPoolDetect: "pool", msgPoolCreate: "pool", msgPoolStats: "pool", msgPoolRemove: "pool",
	msgPoolBrowse: "pool", msgPoolFile: "pool",
	msgNetDetect: "net", msgNetEnsure: "net", msgNetRemove: "net",
	msgImageFetch: "compute_image", msgImageList: "compute_image", msgImageRemove: "compute_image",
	msgVMQuery: "vm", msgVMShutdown: "vm",
}

func newFakeAgentRPC(t *testing.T) *fakeAgentRPC {
	send, _ := declaredAgentRPC(t)
	f := &fakeAgentRPC{t: t, declared: map[string]bool{}, payloads: map[string][]byte{}}
	for _, s := range send {
		f.declared[s] = true
	}
	return f
}

func (f *fakeAgentRPC) Available(context.Context, string) (bool, string) { return true, "" }

func (f *fakeAgentRPC) Call(_ context.Context, _, msgType string, payload []byte, onProgress func(string, []byte)) ([]byte, error) {
	if !f.declared[msgType] {
		f.t.Errorf("the plugin sent %q, which plugin.yaml does not declare", msgType)
		return nil, errors.New("undeclared")
	}
	f.payloads[msgType] = payload
	slot := readFixture(f.t, "slots", slotFor[msgType]+".json")
	if msgType == msgImageFetch && onProgress != nil {
		onProgress(msgImageProgress, slot)
	}
	return slot, f.replyErr
}

func (f *fakeAgentRPC) Subscribe(sink func(envID, msgType string, frame []byte)) { f.sink = sink }

// sameJSON compares two JSON documents by value.
func sameJSON(t *testing.T, what string, got any, want []byte) {
	t.Helper()
	gotJSON, err := json.Marshal(got)
	if err != nil {
		t.Fatal(err)
	}
	var a, b any
	if err := json.Unmarshal(gotJSON, &a); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(want, &b); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(a, b) {
		t.Errorf("%s differs from the host's conversion:\n plugin %s\n host   %s", what, gotJSON, want)
	}
}

// requestCase decodes a requests/<name>.json fixture: the host's input and the slot it wrote.
type requestCase struct {
	Args []json.RawMessage `json:"args"`
	Wire json.RawMessage   `json:"wire"`
}

func loadRequest(t *testing.T, name string) requestCase {
	t.Helper()
	var rc requestCase
	if err := json.Unmarshal(readFixture(t, "requests", name+".json"), &rc); err != nil {
		t.Fatal(err)
	}
	return rc
}

func arg[T any](t *testing.T, rc requestCase, i int) T {
	t.Helper()
	var out T
	if err := json.Unmarshal(rc.Args[i], &out); err != nil {
		t.Fatal(err)
	}
	return out
}

// sameWire compares what the plugin sent with what the host's typed call wrote. envId and
// the fixed operation are written by the host on the plugin's behalf, so they are not the
// plugin's to send; every other field must match.
func sameWire(t *testing.T, name string, sent []byte, rc requestCase) {
	t.Helper()
	var got, want map[string]any
	if len(sent) > 0 {
		if err := json.Unmarshal(sent, &got); err != nil {
			t.Fatal(err)
		}
	}
	if err := json.Unmarshal(rc.Wire, &want); err != nil {
		t.Fatal(err)
	}
	delete(want, "envId")
	if name != "pool_file" {
		delete(want, "operation")
	}
	if got == nil {
		got = map[string]any{}
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("%s: the plugin sent %s, the host's typed call wrote %v", name, sent, want)
	}
}

// TestBridgeMatchesTheHostConversions is the conversion-parity floor for the move off the
// host's typed handle: for every call this plugin used to make through it, the plugin now
// sends the slot the host wrote from the same input, and turns the fully populated reply
// into exactly the value the host returned. A field dropped in the port fails here.
func TestBridgeMatchesTheHostConversions(t *testing.T) {
	fake := newFakeAgentRPC(t)
	b := newAgentBridge(fake)
	ctx := context.Background()
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	converted := func(name string) []byte { return readFixture(t, "converted", name+".json") }
	covered := map[string]bool{}
	checkConverted := func(name string, v any) {
		covered[name] = true
		sameJSON(t, name, v, converted(name))
	}
	checkRequest := func(name, msgType string) {
		covered["request:"+name] = true
		sameWire(t, name, fake.payloads[msgType], loadRequest(t, name))
	}

	capability, err := b.Probe(ctx, "env-1")
	must(err)
	checkConverted("capability", capability)
	checkRequest("compute_probe", msgComputeProbe)

	rc := loadRequest(t, "compute_config")
	roots, err := b.ConfigureRoots(ctx, "env-1", arg[RootsConfig](t, rc, 0))
	must(err)
	checkConverted("roots", roots)
	checkRequest("compute_config", msgComputeConfig)

	rc = loadRequest(t, "compute_migrate")
	result, err := b.MigrateRoots(ctx, "env-1", arg[MigrationRequest](t, rc, 0))
	must(err)
	checkConverted("migration_result", result)
	checkRequest("compute_migrate", msgComputeMigrate)

	backends, err := b.PoolDetect(ctx, "env-1")
	must(err)
	checkConverted("pool_backends", backends)
	checkRequest("pool_detect", msgPoolDetect)

	rc = loadRequest(t, "pool_create")
	stats, err := b.PoolCreate(ctx, "env-1", arg[PoolRequest](t, rc, 0))
	must(err)
	checkConverted("pool_stats", stats)
	checkRequest("pool_create", msgPoolCreate)

	rc = loadRequest(t, "pool_stats")
	sample, err := b.PoolStats(ctx, "env-1", arg[PoolRef](t, rc, 0))
	must(err)
	sameJSON(t, "pool_stats sample", sample, converted("pool_stats"))
	checkRequest("pool_stats", msgPoolStats)

	rc = loadRequest(t, "pool_remove")
	must(b.PoolRemove(ctx, "env-1", arg[PoolRequest](t, rc, 0)))
	checkRequest("pool_remove", msgPoolRemove)

	rc = loadRequest(t, "pool_browse")
	files, err := b.PoolBrowse(ctx, "env-1", arg[PoolRef](t, rc, 0), arg[string](t, rc, 1))
	must(err)
	checkConverted("pool_files", files)
	checkRequest("pool_browse", msgPoolBrowse)

	rc = loadRequest(t, "pool_file")
	content, err := b.PoolFile(ctx, "env-1", arg[PoolRef](t, rc, 0), arg[PoolFileRequest](t, rc, 1))
	must(err)
	checkConverted("pool_file_content", content)
	checkRequest("pool_file", msgPoolFile)

	links, err := b.NetDetect(ctx, "env-1")
	must(err)
	checkConverted("host_links", links)
	checkRequest("net_detect", msgNetDetect)

	rc = loadRequest(t, "net_ensure")
	must(b.NetEnsure(ctx, "env-1", arg[NetworkRequest](t, rc, 0)))
	checkRequest("net_ensure", msgNetEnsure)
	rc = loadRequest(t, "net_remove")
	must(b.NetRemove(ctx, "env-1", arg[NetworkRequest](t, rc, 0)))
	checkRequest("net_remove", msgNetRemove)

	rc = loadRequest(t, "compute_image_fetch")
	var progress []ImageProgress
	fetched, err := b.ImageFetch(ctx, "env-1", arg[ImageRequest](t, rc, 0), func(p ImageProgress) { progress = append(progress, p) })
	must(err)
	checkConverted("image_result", fetched)
	if len(progress) != 1 {
		t.Fatalf("image fetch forwarded %d progress frames, want 1", len(progress))
	}
	checkConverted("image_progress", progress[0])
	checkRequest("compute_image_fetch", msgImageFetch)

	cached, err := b.ImageList(ctx, "env-1")
	must(err)
	checkConverted("cached_images", cached)
	checkRequest("compute_image_list", msgImageList)

	rc = loadRequest(t, "compute_image_remove")
	must(b.ImageRemove(ctx, "env-1", arg[string](t, rc, 0)))
	checkRequest("compute_image_remove", msgImageRemove)

	var leases []GuestEvent
	b.Subscribe(func(ev GuestEvent) { leases = append(leases, ev) })
	fake.sink("env-1", msgNetLeaseEvent, readFixture(t, "slots", "net.json"))
	if len(leases) != 1 || leases[0].At.IsZero() {
		t.Fatalf("lease frame produced %+v", leases)
	}
	leases[0].At = time.Time{}
	checkConverted("lease_event", leases[0])
	fake.sink("env-1", msgComputeMigrateProgress, readFixture(t, "slots", "compute.json"))
	moving, ok := b.MigrationProgress("env-1")
	if !ok || moving.At.IsZero() {
		t.Fatal("relocation frame recorded no progress")
	}
	moving.At = time.Time{}
	checkConverted("migration_progress", moving)

	// Every golden the host wrote is held to a port here, so a conversion the host adds a
	// fixture for cannot be left unported. The guest frames this plugin never observes are
	// held to the guest plugins' ports instead, each in its own tests.
	for _, name := range []string{"vm_events", "import_progress", "ct_events"} {
		covered[name] = true
	}
	for _, dir := range []string{"converted", "requests"} {
		entries, err := os.ReadDir(filepath.Join(agentWireDir, dir))
		if err != nil {
			t.Fatal(err)
		}
		for _, e := range entries {
			name := strings.TrimSuffix(e.Name(), ".json")
			if dir == "requests" {
				name = "request:" + name
			}
			if !covered[name] {
				t.Errorf("%s/%s has no parity check in this plugin", dir, e.Name())
			}
		}
	}
}

// TestWireStructsDecodeEveryFieldTheyHold holds each local wire struct to the host's slot
// fixture: the fixture sets every protocol field, so a field of ours that decodes to zero
// has a tag the agent does not send.
func TestWireStructsDecodeEveryFieldTheyHold(t *testing.T) {
	cases := []struct {
		slot string
		into any
	}{
		{"compute", &agentComputePayload{}},
		{"pool", &agentPoolPayload{}},
		{"net", &agentNetReply{}},
		{"net", &agentNetwork{}},
		{"compute_image", &agentImagePayload{}},
		{"vm", &agentVMStates{}},
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

// TestManifestDeclaresExactlyWhatThePluginSends keeps plugin.yaml's agent-rpc list honest in
// both directions: nothing sent is undeclared (the host refuses it), and nothing declared
// is unused (a declaration is a grant, and an unused grant is reach nobody reviewed).
func TestManifestDeclaresExactlyWhatThePluginSends(t *testing.T) {
	send, observe := declaredAgentRPC(t)
	used := make([]string, 0, len(slotFor))
	for msgType := range slotFor {
		used = append(used, msgType)
	}
	sort.Strings(used)
	sort.Strings(send)
	if !slices.Equal(send, used) {
		t.Errorf("agent-rpc send = %v, the plugin sends %v", send, used)
	}
	sort.Strings(observe)
	if want := []string{msgComputeMigrateProgress, msgNetLeaseEvent}; !slices.Equal(observe, want) {
		t.Errorf("agent-rpc observe = %v, the plugin handles %v", observe, want)
	}
}

// codedErr is a bridge error as the host builds it: text plus a code.
type codedErr struct{ code, text string }

func (e *codedErr) Error() string        { return e.text }
func (e *codedErr) AgentRPCCode() string { return e.code }

// TestClassifyMapsBridgeCodes replaces the old errors.Is against the host's compute
// sentinels: a bridge failure is classified by its code, wherever it is wrapped, and the
// words alone never decide it.
func TestClassifyMapsBridgeCodes(t *testing.T) {
	h := &Handler{logger: quietLogger{}}
	cases := []struct {
		code   string
		status int
		want   string
	}{
		{rpcAgentOutdated, http.StatusConflict, codeAgentOutdated},
		{rpcNotAgentEnv, http.StatusConflict, codeNotAgentEnv},
		{rpcAgentOffline, http.StatusConflict, codeAgentOffline},
		{rpcUplinkBusy, http.StatusBadRequest, codeUplinkBusy},
		{"refused", http.StatusBadGateway, codeAgentFailed},
	}
	for _, tc := range cases {
		err := fmt.Errorf("pool p1 file write: %w", &codedErr{code: tc.code, text: "agent failed: something"})
		status, code, _ := h.classify("op", err)
		if status != tc.status || code != tc.want {
			t.Errorf("code %q: got %d %q, want %d %q", tc.code, status, code, tc.status, tc.want)
		}
	}
	// The wording the old sentinels carried maps to nothing without a code.
	status, code, _ := h.classify("op", errors.New("compute: this agent is too old for that"))
	if status != http.StatusBadGateway || code != codeAgentFailed {
		t.Errorf("uncoded outdated wording: got %d %q", status, code)
	}
}

// TestBridgeKeepsTheReplyBesideARefusal covers the one place a refusal carries data: the
// locations still in force come back with the error, as they did from the typed handle.
func TestBridgeKeepsTheReplyBesideARefusal(t *testing.T) {
	fake := newFakeAgentRPC(t)
	fake.replyErr = &codedErr{code: "refused", text: "agent refused the compute locations: a guest is running"}
	b := newAgentBridge(fake)
	roots, err := b.ConfigureRoots(context.Background(), "env-1", RootsConfig{VMRoot: "/new"})
	if agentRPCCode(err) != "refused" {
		t.Fatalf("err = %v", err)
	}
	if roots.VMRoot != "VMRoot" {
		t.Fatalf("the locations in force were dropped with the refusal: %+v", roots)
	}
	if _, err := b.Probe(context.Background(), "env-1"); err == nil {
		t.Fatal("a refused probe returned a capability")
	}
}

// TestRelocationProgressLivesInThePlugin covers the state this plugin took over from the
// host: an apply opens a progress entry, frames move it, and the reply closes it.
func TestRelocationProgressLivesInThePlugin(t *testing.T) {
	fake := newFakeAgentRPC(t)
	b := newAgentBridge(fake)
	b.Subscribe(nil)

	started := make(chan struct{})
	release := make(chan struct{})
	blocking := &blockingRPC{fakeAgentRPC: fake, started: started, release: release}
	b.rpc = blocking
	done := make(chan error, 1)
	go func() {
		_, err := b.MigrateRoots(context.Background(), "env-1", MigrationRequest{DiskFrom: "/a", DiskTo: "/b"})
		done <- err
	}()
	<-started
	if _, ok := b.MigrationProgress("env-1"); !ok {
		t.Fatal("a running apply has no progress entry")
	}
	fake.sink("env-1", msgComputeMigrateProgress, []byte(`{"migrate":{"entries":42,"bytes":900}}`))
	if p, _ := b.MigrationProgress("env-1"); p.Percent != 42 || p.Bytes != 900 {
		t.Fatalf("progress = %+v", p)
	}
	close(release)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if _, ok := b.MigrationProgress("env-1"); ok {
		t.Fatal("progress outlived the relocation")
	}
	if _, err := b.MigrateRoots(context.Background(), "env-1", MigrationRequest{Plan: true}); err != nil {
		t.Fatal(err)
	}
	if _, ok := b.MigrationProgress("env-1"); ok {
		t.Fatal("a plan opened a progress entry")
	}
}

type blockingRPC struct {
	*fakeAgentRPC
	started chan struct{}
	release chan struct{}
	once    bool
}

func (b *blockingRPC) Call(ctx context.Context, envID, msgType string, payload []byte, onProgress func(string, []byte)) ([]byte, error) {
	if msgType == msgComputeMigrate && !b.once {
		b.once = true
		close(b.started)
		<-b.release
	}
	return b.fakeAgentRPC.Call(ctx, envID, msgType, payload, onProgress)
}

// TestBridgeNeedsTheAgentRPCHandle covers the assertion: no handle, or one without Call and
// Subscribe (a host predating agent-rpc), leaves compute unavailable rather than half-wired.
func TestBridgeNeedsTheAgentRPCHandle(t *testing.T) {
	if newAgentBridge(nil) != nil {
		t.Fatal("a nil handle produced a bridge")
	}
	type typedOnly struct{ agentRPCCoder }
	if newAgentBridge(typedOnly{}) != nil {
		t.Fatal("a handle without Call and Subscribe produced a bridge")
	}
	if newAgentBridge(newFakeAgentRPC(t)) == nil {
		t.Fatal("an agent-rpc handle produced no bridge")
	}
}
