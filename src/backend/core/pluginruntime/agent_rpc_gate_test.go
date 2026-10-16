// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package pluginruntime

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"gopkg.in/yaml.v3"

	coreagent "github.com/therealmcsparrow/mcharbor/core/agent"
	"github.com/therealmcsparrow/mcharbor/core/agentrpc"
	"github.com/therealmcsparrow/mcharbor/core/plugin/host"
	"github.com/therealmcsparrow/mcharbor/core/plugincaps"
)

// pluginAgentRPC is the interface a plugin declares locally and asserts HostContext.AgentRPC
// against; builtin types only, so this test holds the loader to what a plugin can see.
type pluginAgentRPC interface {
	Available(ctx context.Context, envID string) (bool, string)
	Call(ctx context.Context, envID, msgType string, payload []byte, onProgress func(msgType string, frame []byte)) ([]byte, error)
	Subscribe(sink func(envID, msgType string, frame []byte))
}

// TestAgentRPCViewRequiresTokenAndCapability is the loader half of the agent-rpc gate: the
// bridge is reachable only by a plugin whose manifest carries both the agent.rpc token and
// an agent-rpc declaration, the old compute.rpc token grants nothing, and what a plugin gets
// is its narrowed view, never the host's bridge.
func TestAgentRPCViewRequiresTokenAndCapability(t *testing.T) {
	bridge := agentrpc.New(nil, nil)
	decl := &plugincaps.Capability{Kind: plugincaps.KindAgentRPC, Send: []string{coreagent.MsgVMQuery}}

	cases := []struct {
		name   string
		bridge *agentrpc.Bridge
		perms  []string
		decl   *plugincaps.Capability
	}{
		{"no token", bridge, []string{"audit.emit"}, decl},
		{"no declaration", bridge, []string{agentRPCToken}, nil},
		{"the retired compute.rpc token", bridge, []string{"compute.rpc"}, decl},
		{"no host bridge", nil, []string{agentRPCToken}, decl},
		{"no permissions", bridge, nil, decl},
	}
	for _, c := range cases {
		if got := agentRPCFor(c.bridge, "addon-kvm", c.perms, c.decl); got != nil {
			t.Errorf("%s: the plugin must receive nil, got %T", c.name, got)
		}
	}

	got := agentRPCFor(bridge, "addon-kvm", []string{"audit.emit", agentRPCToken}, decl)
	if _, isBridge := got.(*agentrpc.Bridge); isBridge {
		t.Fatal("a plugin must receive its narrowed view, never the host's bridge")
	}
	handle, ok := got.(pluginAgentRPC)
	if !ok {
		t.Fatalf("token plus agent-rpc must yield a handle a plugin can assert, got %T", got)
	}
	if _, err := handle.Call(context.Background(), "env-1", coreagent.MsgVMStart, nil, nil); !errors.Is(err, agentrpc.ErrUndeclared) {
		t.Fatalf("an undeclared type through the loader's view: got %v", err)
	}
}

// TestAgentRPCDeclarationIsReadFromTheInstalledManifest covers agentRPCDeclFor: present,
// absent, and a manifest whose capabilities do not validate.
func TestAgentRPCDeclarationIsReadFromTheInstalledManifest(t *testing.T) {
	write := func(body string) string {
		dir := t.TempDir()
		if err := os.WriteFile(filepath.Join(dir, "plugin.yaml"), []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
		return dir
	}
	declared := write("slug: addon-kvm\ncontributes:\n  capabilities:\n    - kind: agent-rpc\n      send: [vm_query]\n      observe: [vm_event]\n")
	if d := agentRPCDeclFor("addon-kvm", declared); d == nil || len(d.Send) != 1 || d.Observe[0] != coreagent.MsgVMEvent {
		t.Fatalf("declared manifest read as %+v", d)
	}
	if d := agentRPCDeclFor("addon-kvm", write("slug: addon-kvm\n")); d != nil {
		t.Fatalf("a manifest without agent-rpc read as %+v", d)
	}
	if d := agentRPCDeclFor("addon-kvm", write("slug: addon-kvm\ncontributes:\n  capabilities:\n    - kind: agent-rpc\n      send: [exec_start]\n")); d != nil {
		t.Fatalf("an outside-ceiling declaration read as %+v", d)
	}
	if d := agentRPCDeclFor("addon-kvm", t.TempDir()); d != nil {
		t.Fatal("a missing manifest must grant nothing")
	}
}

// TestShippedManifestsDeclareOnlyKnownScopeTokens reads every in-tree plugin.yaml and holds its
// permission tokens to the host's schema, so renaming a token on one side alone (as agent.rpc
// replaced compute.rpc) fails here rather than at install. The agent.rpc token is only
// meaningful beside an agent-rpc declaration, so the two must travel together.
func TestShippedManifestsDeclareOnlyKnownScopeTokens(t *testing.T) {
	root := filepath.Join("..", "..", "..", "plugins", "Plugins")
	dirs, err := os.ReadDir(root)
	if errors.Is(err, os.ErrNotExist) {
		t.Skipf("plugin source tree not present at %s", root)
	}
	if err != nil {
		t.Fatal(err)
	}
	carriers := 0
	for _, d := range dirs {
		dir := filepath.Join(root, d.Name())
		raw, err := os.ReadFile(filepath.Join(dir, "plugin.yaml"))
		if err != nil {
			continue
		}
		var manifest struct {
			Permissions []string `yaml:"permissions"`
		}
		if err := yaml.Unmarshal(raw, &manifest); err != nil {
			t.Errorf("%s: %v", d.Name(), err)
			continue
		}
		for _, token := range manifest.Permissions {
			if !host.KnownScopeToken(token) {
				t.Errorf("%s: permission token %q is not one the host schema knows", d.Name(), token)
			}
		}
		if slices.Contains(manifest.Permissions, agentRPCToken) {
			carriers++
			if agentRPCDeclFor(d.Name(), dir) == nil {
				t.Errorf("%s: declares %s without a valid agent-rpc capability, so it would receive nothing", d.Name(), agentRPCToken)
			}
		}
	}
	if carriers < 3 {
		t.Errorf("%d in-tree plugins carry %s, want at least the three compute plugins", carriers, agentRPCToken)
	}
}
