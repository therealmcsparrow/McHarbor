// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package agent

import (
	"errors"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// protocolConst is one Msg* constant read out of protocol.go with its trailing comment.
type protocolConst struct {
	Name, Value, Comment string
}

// protocolMessages parses protocol.go rather than listing constants by hand, so a message
// type added to the protocol is covered by these tests the moment it exists.
func protocolMessages(t *testing.T) []protocolConst {
	t.Helper()
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "protocol.go", nil, parser.ParseComments)
	if err != nil {
		t.Fatal(err)
	}
	var out []protocolConst
	for _, decl := range file.Decls {
		gen, ok := decl.(*ast.GenDecl)
		if !ok || gen.Tok != token.CONST {
			continue
		}
		for _, spec := range gen.Specs {
			vs := spec.(*ast.ValueSpec)
			for i, name := range vs.Names {
				if !strings.HasPrefix(name.Name, "Msg") || i >= len(vs.Values) {
					continue
				}
				lit, ok := vs.Values[i].(*ast.BasicLit)
				if !ok || lit.Kind != token.STRING {
					continue
				}
				value, err := strconv.Unquote(lit.Value)
				if err != nil {
					t.Fatal(err)
				}
				comment := ""
				if vs.Comment != nil {
					comment = vs.Comment.Text()
				}
				out = append(out, protocolConst{Name: name.Name, Value: value, Comment: comment})
			}
		}
	}
	if len(out) < 150 {
		t.Fatalf("read only %d Msg constants from protocol.go; the parser is not reading the file it thinks it is", len(out))
	}
	return out
}

type ceilingContract struct {
	Send []struct {
		Type      string   `yaml:"type"`
		Slot      string   `yaml:"slot"`
		Budget    string   `yaml:"budget"`
		Floor     string   `yaml:"floor"`
		Progress  []string `yaml:"progress"`
		Operation string   `yaml:"operation"`
		Cancel    string   `yaml:"cancel"`
	} `yaml:"send"`
	Observe []struct {
		Type string `yaml:"type"`
		Slot string `yaml:"slot"`
	} `yaml:"observe"`
}

// TestPluginRouteCeilingMatchesTheGoldenContract pins the routable and observable sets,
// field for field. Widening the ceiling is a reviewed change to
// src/contracts/plugincaps/agent_rpc_ceiling.yaml, which the CLI's mirror reads too.
func TestPluginRouteCeilingMatchesTheGoldenContract(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("..", "..", "..", "contracts", "plugincaps", "agent_rpc_ceiling.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	var golden ceilingContract
	if err := yaml.Unmarshal(raw, &golden); err != nil {
		t.Fatal(err)
	}

	routes := PluginRoutes()
	if len(routes) != len(golden.Send) {
		t.Fatalf("ceiling has %d routes, the golden contract %d", len(routes), len(golden.Send))
	}
	for i, r := range routes {
		g := golden.Send[i]
		if r.Type != g.Type || string(r.Slot) != g.Slot || string(r.Budget) != g.Budget ||
			string(r.Floor) != g.Floor || !slices.Equal(r.Progress, g.Progress) ||
			r.Operation != g.Operation || r.CancelWith != g.Cancel {
			t.Errorf("route %d = %+v, golden %+v", i, r, g)
		}
	}

	observed := PluginObservable()
	if len(observed) != len(golden.Observe) {
		t.Fatalf("observe table has %d types, the golden contract %d", len(observed), len(golden.Observe))
	}
	for i, o := range observed {
		g := golden.Observe[i]
		if o.Type != g.Type || string(o.Slot) != g.Slot {
			t.Errorf("observed %d = %+v, golden %+v", i, o, g)
		}
	}
}

// TestNeverRoutableFamilies is the security floor: no terminal, shell, self-update,
// compose, Docker proxy, file-transfer or backup message may ever be sendable or observable
// by a plugin, whatever else the ceiling gains later.
func TestNeverRoutableFamilies(t *testing.T) {
	forbidden := []string{"exec_", "host_terminal_", "agent_update_", "compose_", "http_",
		"transfer_", "backup_", "auth", "console_", "host_update_", "host_service_",
		"snapshot_", "offline_", "stats_", "prune_"}
	checked := 0
	for _, c := range protocolMessages(t) {
		for _, prefix := range forbidden {
			if !strings.HasPrefix(c.Value, prefix) {
				continue
			}
			checked++
			if _, ok := PluginRouteFor(c.Value); ok {
				t.Errorf("%s (%q) is plugin-routable", c.Name, c.Value)
			}
			if _, ok := PluginObservedFor(c.Value); ok {
				t.Errorf("%s (%q) is plugin-observable", c.Name, c.Value)
			}
		}
	}
	if checked < 40 {
		t.Fatalf("only %d forbidden-family messages checked; the prefixes no longer match the protocol", checked)
	}

	// Two compute messages the host sends on its own and a plugin must never: the external
	// switch confirmation, which cancels the agent's rollback of an adapter adoption and is
	// only safe once the host has seen the agent still answering, and the image cancel, which
	// the host sends under the id of a fetch the plugin has given up on.
	for _, msgType := range []string{MsgNetAdoptConfirm, MsgImageCancelRequest} {
		if _, ok := PluginRouteFor(msgType); ok {
			t.Errorf("%q is plugin-routable", msgType)
		}
	}
	for _, r := range PluginRoutes() {
		if r.CancelWith == "" {
			continue
		}
		if _, ok := PluginRouteFor(r.CancelWith); ok {
			t.Errorf("%s's cancel message %q is itself plugin-routable", r.Type, r.CancelWith)
		}
	}
}

// TestCeilingFollowsTheProtocolDirection holds every routable type to a Server->Agent
// message and every observed one to Agent->Server: a plugin must never make the host send
// the agent something only the agent emits, or subscribe to a request.
func TestCeilingFollowsTheProtocolDirection(t *testing.T) {
	byValue := map[string]protocolConst{}
	for _, c := range protocolMessages(t) {
		byValue[c.Value] = c
	}
	for _, r := range PluginRoutes() {
		c, ok := byValue[r.Type]
		if !ok {
			t.Errorf("route %q is not a protocol message", r.Type)
			continue
		}
		// vm_stats is commented Agent->Server, but the request and its reply share the type:
		// the agent dispatches it as a request (src/agent/agent.go) and answers under it.
		if !strings.Contains(c.Comment, "Server->Agent") && r.Type != MsgVMStats {
			t.Errorf("route %s (%q) is not a Server->Agent message: %q", c.Name, r.Type, c.Comment)
		}
		for _, p := range r.Progress {
			if pc, ok := byValue[p]; !ok || !strings.Contains(pc.Comment, "Agent->Server") {
				t.Errorf("progress frame %q of %q is not an Agent->Server message", p, r.Type)
			}
		}
	}
	for _, o := range PluginObservable() {
		c, ok := byValue[o.Type]
		if !ok || !strings.Contains(c.Comment, "Agent->Server") {
			t.Errorf("observed %q is not an Agent->Server message", o.Type)
		}
	}
}

// TestPluginRouteForCannotWidenTheCeiling guards the copies: a caller that edits what it
// was handed must not change what the next caller sees.
func TestPluginRouteForCannotWidenTheCeiling(t *testing.T) {
	r, _ := PluginRouteFor(MsgVMCloneRequest)
	r.Progress[0] = MsgExecOutput
	all := PluginRoutes()
	all[0].Progress = append(all[0].Progress, MsgExecOutput)
	again, _ := PluginRouteFor(MsgVMCloneRequest)
	if !slices.Equal(again.Progress, []string{MsgVMCloneProgress}) {
		t.Fatalf("progress list was widened through a returned copy: %v", again.Progress)
	}
	for _, r := range PluginRoutes() {
		if slices.Contains(r.Progress, MsgExecOutput) {
			t.Fatalf("%s gained exec_output through a returned copy", r.Type)
		}
	}
}

func TestPluginFloorsRefuseOldAgents(t *testing.T) {
	old := &AgentConnection{Version: "3.7.0"}
	sameVersionOldBuild := &AgentConnection{Version: CTBackupMinVersion, Build: "202609010000"}
	current := &AgentConnection{Version: CTBackupMinVersion, Build: "209912312359"}
	cases := []struct {
		name    string
		floor   PluginFloor
		conn    *AgentConnection
		msg     string
		payload string
		refused string
	}{
		{"free volume on 3.7.0", FloorFreeVolume, old, MsgVMDiskList, "", "vm_disk_list needs agent 3.8.0 or newer"},
		{"free volume on current", FloorFreeVolume, current, MsgVMLog, "", ""},
		{"free volume without a connection", FloorFreeVolume, nil, MsgVMLog, "", "needs agent"},
		{"ct backup on an old build", FloorCTBackup, sameVersionOldBuild, MsgCTBackupRun, "", "ct_backup_run needs agent " + CTBackupMinVersion + " build " + CTBackupMinBuild},
		{"ct backup on current", FloorCTBackup, current, MsgCTBackupRestore, "", ""},
		{"rootfs create from image on an old build", FloorCTRootfs, sameVersionOldBuild, MsgCTCreate, `{"imageId":"i"}`, "needs agent " + CTRootfsMinVersion + " build " + CTRootfsMinBuild},
		{"create over a restored rootfs on an old build", FloorCTRootfs, sameVersionOldBuild, MsgCTCreate, `{"spec":{"name":"a"}}`, ""},
		{"delete with rootfs on an old build", FloorCTRootfs, sameVersionOldBuild, MsgCTDelete, `{"deleteRootfs":true}`, "needs agent"},
		{"managed network on 3.3.0", FloorGuestNetworking, &AgentConnection{Version: "3.3.0"}, MsgNetEnsureRequest, `{"mode":"managed"}`, "this agent is 3.3.0 and managed or shared networks need 3.4.0 or newer"},
		{"bridged network on 3.3.0", FloorGuestNetworking, &AgentConnection{Version: "3.3.0"}, MsgNetEnsureRequest, `{"mode":"bridged"}`, ""},
		{"no floor", FloorNone, old, MsgVMStart, "", ""},
		{"library fetch on 3.1.0", FloorImageLibrary, &AgentConnection{Version: "3.1.0"}, MsgImageFetchRequest, `{"fromLibrary":true}`, "this agent is 3.1.0 and the shared image library needs " + ImageLibraryMinVersion + " or newer"},
		{"mirror fetch on 3.1.0", FloorImageLibrary, &AgentConnection{Version: "3.1.0"}, MsgImageFetchRequest, `{"sourceUrl":"https://x"}`, ""},
		{"library fetch on current", FloorImageLibrary, current, MsgImageFetchRequest, `{"fromLibrary":true}`, ""},
	}
	for _, tc := range cases {
		err := tc.floor.Check(tc.conn, tc.msg, []byte(tc.payload))
		if tc.refused == "" {
			if err != nil {
				t.Errorf("%s: refused with %v", tc.name, err)
			}
			continue
		}
		if err == nil || !errors.Is(err, ErrComputeUnsupported) || !strings.Contains(err.Error(), tc.refused) {
			t.Errorf("%s: got %v, want ErrComputeUnsupported containing %q", tc.name, err, tc.refused)
		}
	}
}
