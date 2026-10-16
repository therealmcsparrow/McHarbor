// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package cli

import (
	"os"
	"path/filepath"
	"slices"
	"testing"

	"gopkg.in/yaml.v3"
)

// TestAgentRPCCeilingMirrorsTheContract holds the CLI's copy of the host's agent message
// ceiling to the contract the host's golden test reads, so `plugin dev validate` accepts
// exactly the declarations the loader accepts.
func TestAgentRPCCeilingMirrorsTheContract(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("..", "..", "..", "contracts", "plugincaps", "agent_rpc_ceiling.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	var contract struct {
		Send []struct {
			Type string `yaml:"type"`
		} `yaml:"send"`
		Observe []struct {
			Type string `yaml:"type"`
		} `yaml:"observe"`
	}
	if err := yaml.Unmarshal(raw, &contract); err != nil {
		t.Fatal(err)
	}
	var send, observe []string
	for _, s := range contract.Send {
		send = append(send, s.Type)
	}
	for _, o := range contract.Observe {
		observe = append(observe, o.Type)
	}
	if len(send) == 0 || len(observe) == 0 {
		t.Fatal("the contract lists nothing; the test is not reading the file it thinks it is")
	}
	if !slices.Equal(send, pluginAgentRPCSend) {
		t.Errorf("CLI send ceiling drifted from the contract:\n cli      %v\n contract %v", pluginAgentRPCSend, send)
	}
	if !slices.Equal(observe, pluginAgentRPCObserve) {
		t.Errorf("CLI observe table drifted from the contract:\n cli      %v\n contract %v", pluginAgentRPCObserve, observe)
	}
}
