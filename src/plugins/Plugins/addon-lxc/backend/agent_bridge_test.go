// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"slices"
	"sort"
	"strings"
	"testing"
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
			*list = append(*list, strings.TrimPrefix(trimmed, "- "))
		}
	}
	if len(send) == 0 {
		t.Fatal("plugin.yaml declares no agent-rpc send list")
	}
	return send, observe
}

// constantMessages returns every message type named in wire_agent.go's const block, keyed by
// its Go identifier.
func constantMessages(t *testing.T) map[string]string {
	t.Helper()
	raw, err := os.ReadFile("wire_agent.go")
	if err != nil {
		t.Fatal(err)
	}
	out := map[string]string{}
	for _, m := range regexp.MustCompile(`(?m)^\s*(msg\w+)\s*=\s*"([a-z_]+)"`).FindAllStringSubmatch(string(raw), -1) {
		out[m[1]] = m[2]
	}
	return out
}

// TestManifestDeclaresExactlyWhatThePluginSends keeps plugin.yaml's agent-rpc list honest in
// both directions: nothing sent is undeclared (the host refuses it), and nothing declared is
// unused (a declaration is a grant, and an unused grant is reach nobody reviewed). What the
// plugin sends is every message constant that code outside wire_agent.go actually uses.
func TestManifestDeclaresExactlyWhatThePluginSends(t *testing.T) {
	sources, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	var code strings.Builder
	for _, file := range sources {
		if file == "wire_agent.go" || strings.HasSuffix(file, "_test.go") {
			continue
		}
		raw, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		code.Write(raw)
	}
	var used []string
	for ident, msgType := range constantMessages(t) {
		if !regexp.MustCompile(`\b` + ident + `\b`).MatchString(code.String()) {
			t.Errorf("%s (%s) is declared in wire_agent.go and used nowhere", ident, msgType)
			continue
		}
		if msgType != msgCTBackupProgress {
			used = append(used, msgType)
		}
	}
	send, observe := declaredAgentRPC(t)
	sort.Strings(used)
	sort.Strings(send)
	if !slices.Equal(send, used) {
		t.Errorf("agent-rpc send = %v, the plugin sends %v", send, used)
	}
	if want := []string{msgCTBackupProgress}; !slices.Equal(observe, want) {
		t.Errorf("agent-rpc observe = %v, the plugin handles %v", observe, want)
	}
}

// subset reports every leaf of got whose value differs from the same place in want; the
// host's conversion carried more fields than this plugin reads, so extra keys in want are
// expected.
func subset(t *testing.T, what string, got any, want []byte) {
	t.Helper()
	gotJSON, err := json.Marshal(got)
	if err != nil {
		t.Fatal(err)
	}
	var g, w any
	if err := json.Unmarshal(gotJSON, &g); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(want, &w); err != nil {
		t.Fatal(err)
	}
	if m, ok := g.(map[string]any); !ok || len(m) == 0 {
		t.Errorf("%s converted to nothing", what)
	}
	var walk func(path string, g, w any)
	walk = func(path string, g, w any) {
		switch gv := g.(type) {
		case map[string]any:
			wv, ok := w.(map[string]any)
			if !ok {
				t.Errorf("%s: %s is not an object in the host's conversion", what, path)
				return
			}
			for key, value := range gv {
				walk(path+"."+key, value, wv[key])
			}
		case []any:
			wv, ok := w.([]any)
			if !ok || len(wv) < len(gv) {
				t.Errorf("%s: %s = %v, the host's conversion gave %v", what, path, g, w)
				return
			}
			for i, value := range gv {
				walk(path+"[]", value, wv[i])
			}
		default:
			if !reflect.DeepEqual(g, w) {
				t.Errorf("%s: %s = %v, the host's conversion gave %v", what, path, g, w)
			}
		}
	}
	walk("", g, w)
}

// TestConversionsMatchTheHost is the conversion-parity floor for the move off the host's
// typed handle: each call this plugin used to make through it sends the slot the host wrote
// from the same input, and turns the fully populated reply into what the host returned, for
// every field the plugin reads.
func TestConversionsMatchTheHost(t *testing.T) {
	ctx := context.Background()
	svc, _ := newTestService(t, &fakeRPC{canRun: true})
	rpc := &fixtureRPC{t: t}
	svc.rpc = rpc

	capability, err := svc.probe(ctx, "env-1")
	if err != nil {
		t.Fatal(err)
	}
	subset(t, "capability", capability, readFixture(t, "converted", "capability.json"))
	if len(capability.Items) == 0 {
		t.Error("the probe kept no checklist item")
	}
	assertEmptyRequest(t, rpc, msgComputeProbe)

	images, err := svc.imageList(ctx, "env-1")
	if err != nil || len(images) != 1 {
		t.Fatalf("imageList = %v, %v", images, err)
	}
	var cached []json.RawMessage
	if err := json.Unmarshal(readFixture(t, "converted", "cached_images.json"), &cached); err != nil {
		t.Fatal(err)
	}
	subset(t, "cached image", images[0], cached[0])
	assertEmptyRequest(t, rpc, msgImageList)

	var request struct {
		Args []json.RawMessage `json:"args"`
		Wire map[string]any    `json:"wire"`
	}
	if err := json.Unmarshal(readFixture(t, "requests", "pool_file.json"), &request); err != nil {
		t.Fatal(err)
	}
	var pool struct{ ID, Backend, Target string }
	var file struct{ Path string }
	if err := json.Unmarshal(request.Args[0], &pool); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(request.Args[1], &file); err != nil {
		t.Fatal(err)
	}
	body, err := svc.poolFileRead(ctx, "env-1", pool.ID, pool.Backend, pool.Target, file.Path)
	if err != nil {
		t.Fatal(err)
	}
	var sent map[string]any
	if err := json.Unmarshal(rpc.payloads[msgPoolFile], &sent); err != nil {
		t.Fatal(err)
	}
	if sent["operation"] != "read" {
		t.Errorf("pool file operation = %v, want read", sent["operation"])
	}
	for _, key := range []string{"poolId", "backend", "target", "path"} {
		if sent[key] != request.Wire[key] {
			t.Errorf("pool file %s = %v, the host wrote %v", key, sent[key], request.Wire[key])
		}
	}
	var want []byte
	if err := json.Unmarshal(readFixture(t, "converted", "pool_file_content.json"), &want); err != nil {
		t.Fatal(err)
	}
	if string(body) != string(want) {
		t.Errorf("pool file content = %q, the host returned %q", body, want)
	}

	event, ok := backupProgressEvent("env-1", readFixture(t, "slots", "ct.json"))
	if !ok || event.EnvID != "env-1" || event.GuestKind != "ct" || event.GuestID != "ContainerID" ||
		event.Kind != "backup_progress" || !reflect.DeepEqual(event.Detail, map[string]string{"key": "Progress"}) {
		t.Errorf("ct_backup_progress frame produced %+v", event)
	}
}

// fixtureRPC answers every call with the slot fixture for its message family and records
// the payloads, refusing anything plugin.yaml does not declare as the host's view does.
type fixtureRPC struct {
	t        *testing.T
	payloads map[string][]byte
}

func (f *fixtureRPC) Available(context.Context, string) (bool, string) { return true, "" }

func (f *fixtureRPC) Call(_ context.Context, _, msgType string, payload []byte, _ func(string, []byte)) ([]byte, error) {
	send, _ := declaredAgentRPC(f.t)
	if !slices.Contains(send, msgType) {
		f.t.Errorf("the plugin sent %q, which plugin.yaml does not declare", msgType)
		return nil, errors.New("undeclared")
	}
	if f.payloads == nil {
		f.payloads = map[string][]byte{}
	}
	f.payloads[msgType] = payload
	slot := map[string]string{msgComputeProbe: "compute", msgImageList: "compute_image", msgPoolFile: "pool"}[msgType]
	if slot == "" {
		slot = "ct"
	}
	return readFixture(f.t, "slots", slot+".json"), nil
}

func (f *fixtureRPC) Subscribe(func(string, string, []byte)) {}

// assertEmptyRequest holds a request the host wrote with nothing but its own operation and
// environment to send no fields of the plugin's.
func assertEmptyRequest(t *testing.T, rpc *fixtureRPC, msgType string) {
	t.Helper()
	if sent, ok := rpc.payloads[msgType]; !ok || len(sent) != 0 {
		t.Errorf("%s carried %q, the host's request held only its own fields", msgType, sent)
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
		{"ct", &agentPayload{}},
		{"compute", &agentComputePayload{}},
		{"compute_image", &agentImagePayload{}},
		{"pool", &agentPoolFilePayload{}},
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
		if v.Len() > 0 && v.Type().Elem().Kind() != reflect.Uint8 {
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

// codedErr is a bridge error as the host builds it: text plus a code.
type codedErr struct{ code, text string }

func (e *codedErr) Error() string        { return e.text }
func (e *codedErr) AgentRPCCode() string { return e.code }

// TestAgentTooOldIsClassifiedByCodeOrWording keeps both ways a version refusal reaches this
// plugin: the bridge's agent_outdated code, and the "needs agent" wording the bridge keeps.
// Any other failure must stay a plain error, or a flaky agent would read as an old one.
func TestAgentTooOldIsClassifiedByCodeOrWording(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want error
	}{
		{"code alone", &codedErr{rpcAgentOutdated, "the agent is old"}, ErrAgentTooOld},
		{"wording alone", errors.New("ct_backup_run needs agent 2026.09.01 build 1 or newer"), ErrAgentTooOld},
		{"other code", &codedErr{"agent_offline", "the agent is offline"}, nil},
	}
	for _, tc := range cases {
		rpc := &fakeRPC{canRun: true, reply: func(string, agentPayload) ([]byte, error) { return nil, tc.err }}
		svc, _ := newTestService(t, rpc)
		_, err := svc.call(context.Background(), "env-1", msgCTBackupRun, &agentPayload{})
		if tc.want == nil {
			if err == nil || errors.Is(err, ErrAgentTooOld) || errors.Is(err, ErrAgentTooOldRootfs) {
				t.Errorf("%s: err = %v, want a plain failure", tc.name, err)
			}
			continue
		}
		if !errors.Is(err, tc.want) {
			t.Errorf("%s: err = %v, want %v", tc.name, err, tc.want)
		}
	}
	rpc := &fakeRPC{canRun: true, reply: func(string, agentPayload) ([]byte, error) {
		return nil, &codedErr{rpcAgentOutdated, "too old"}
	}}
	svc, _ := newTestService(t, rpc)
	if _, err := svc.call(context.Background(), "env-1", msgCTDelete, &agentPayload{}); !errors.Is(err, ErrAgentTooOldRootfs) {
		t.Errorf("ct_delete on an old agent = %v, want the rootfs floor", err)
	}
}

// TestBackupProgressFramesReachTheRunRows drives the observed frame through the sink the
// plugin registers: a ct_backup_progress frame updates the running row, and any other frame
// is ignored.
func TestBackupProgressFramesReachTheRunRows(t *testing.T) {
	svc, db := newTestService(t, &fakeRPC{canRun: true})
	if _, err := db.Exec(`INSERT INTO plugin_addon_lxc_backup_runs
		(id, container_id, environment_id, status, started_at, created_at, updated_at)
		VALUES ('run-1', 'ct-1', 'env-1', 'running', 'x', 'x', 'x')`); err != nil {
		t.Fatal(err)
	}
	frame := []byte(`{"containerId":"ct-1","progress":{"runId":"run-1","stage":"archiving","bytes":"2048","total":"4096"}}`)
	read := func() (stage string, bytes int64) {
		if err := db.QueryRow(`SELECT progress_stage, bytes_total FROM plugin_addon_lxc_backup_runs WHERE id = 'run-1'`).
			Scan(&stage, &bytes); err != nil {
			t.Fatal(err)
		}
		return stage, bytes
	}

	svc.handleAgentFrame("env-1", "vm_event", frame)
	if stage, bytes := read(); stage != "" || bytes != 0 {
		t.Fatalf("a frame of another type wrote %q %d", stage, bytes)
	}
	svc.handleAgentFrame("env-1", msgCTBackupProgress, frame)
	if stage, bytes := read(); stage != "archiving" || bytes != 2048 {
		t.Fatalf("progress = %q %d, want archiving 2048", stage, bytes)
	}
}

// TestServiceNeedsTheAgentRPCHandle covers the two hosts that cannot carry the bridge: none
// wired, and one whose handle lacks Call and Subscribe.
func TestServiceNeedsTheAgentRPCHandle(t *testing.T) {
	if newAgentRPC(nil) != nil || newAgentRPC(struct{}{}) != nil {
		t.Fatal("a handle without the agent-rpc methods was accepted")
	}
	svc, _ := newTestService(t, nil)
	if _, err := svc.probe(context.Background(), "env-1"); !errors.Is(err, ErrHostUnavailable) {
		t.Errorf("probe with no bridge = %v, want ErrHostUnavailable", err)
	}
	if newAgentRPC(&fakeRPC{}) == nil {
		t.Error("a handle with Available, Call and Subscribe was refused")
	}
}
