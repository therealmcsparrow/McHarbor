// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
)

func TestInstallSendsOnlyTheItemAndKeepsTheReplyWhenItFails(t *testing.T) {
	fake := newFakeAgentRPC(t)
	fake.replyErr = errors.New("installing qemu-utils failed: exit status 100")
	b := newAgentBridge(fake)

	result, err := b.Install(context.Background(), "env-1", "qemu_img")
	if err == nil {
		t.Fatal("the agent's refusal must reach the caller")
	}
	var sent map[string]any
	if err := json.Unmarshal(fake.payloads[msgComputeInstall], &sent); err != nil {
		t.Fatal(err)
	}
	install, _ := sent["install"].(map[string]any)
	if len(sent) != 1 || len(install) != 1 || install["item"] != "qemu_img" {
		t.Fatalf("the request must carry the item alone, sent %v", sent)
	}
	if result.Output == "" || len(result.Packages) == 0 || result.Capability == nil {
		t.Fatalf("a failed install still reports what ran and the fresh capability, got %+v", result)
	}
}

func TestInstallRefusesAnItemNoPackageProvides(t *testing.T) {
	svc := &Service{}
	for _, item := range []string{"kvm_device", "iommu", "", "qemu-utils"} {
		if _, err := svc.InstallCapability(context.Background(), "env-1", item); !errors.Is(err, ErrInvalidTarget) {
			t.Errorf("InstallCapability(%q) = %v, want ErrInvalidTarget before the agent is asked", item, err)
		}
	}
}
