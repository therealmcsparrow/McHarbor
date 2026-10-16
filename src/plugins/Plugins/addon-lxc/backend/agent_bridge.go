// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
)

// agentRPC is the host's agent-rpc bridge as this plugin sees it. Every type in it is
// builtin, so the plugin declares it here and asserts the host handle against it rather
// than linking a host interface whose change would stop the plugin loading.
type agentRPC interface {
	// Available reports whether envID can take agent calls, and what is missing when not.
	Available(ctx context.Context, envID string) (bool, string)
	// Call sends one message declared under agent-rpc send and returns the reply slot as JSON.
	Call(ctx context.Context, envID, msgType string, payload []byte, onProgress func(msgType string, frame []byte)) ([]byte, error)
	// Subscribe replaces this plugin's sink for the frames declared under agent-rpc observe.
	Subscribe(sink func(envID, msgType string, frame []byte))
}

// agentRPCCoder is how a bridge error names its class; the host's sentinels are not part
// of the plugin surface, so a failure is classified by code and never by errors.Is.
type agentRPCCoder interface {
	AgentRPCCode() string
}

// rpcAgentOutdated is the code of a refusal because the agent is older than the message needs.
const rpcAgentOutdated = "agent_outdated"

// agentRPCCode returns err's bridge code, or "" when err did not come from the bridge.
func agentRPCCode(err error) string {
	var coder agentRPCCoder
	if errors.As(err, &coder) {
		return coder.AgentRPCCode()
	}
	return ""
}

// newAgentRPC asserts the host's handle. It returns a true nil interface when the host
// wired none, or one too old to carry agent-rpc, which every caller reads as "no bridge".
func newAgentRPC(handle any) agentRPC {
	rpc, ok := handle.(agentRPC)
	if !ok {
		return nil
	}
	return rpc
}

// capability is the part of a host's probe report this plugin reads.
type capability struct {
	CanRunCTs       bool             `json:"canRunContainers"`
	UnsupportedWhy  string           `json:"unsupportedWhy,omitempty"`
	HostCPUs        int              `json:"hostCpus,omitempty"`
	HostMemoryBytes int64            `json:"hostMemoryBytes,omitempty"`
	Items           []capabilityItem `json:"items,omitempty"`
}

type capabilityItem struct {
	Key     string `json:"key"`
	Present bool   `json:"present"`
}

// cachedImage is one image in a host's cache.
type cachedImage struct {
	ImageID   string `json:"imageId"`
	LocalPath string `json:"localPath"`
}

// guestEvent is a container event as the backup progress path consumes it.
type guestEvent struct {
	EnvID     string
	GuestKind string
	GuestID   string
	Kind      string
	Detail    map[string]string
}

const guestKindCT = "ct"

// probe asks the agent what its host can run.
func (s *Service) probe(ctx context.Context, envID string) (capability, error) {
	if s.rpc == nil {
		return capability{}, ErrHostUnavailable
	}
	raw, err := s.rpc.Call(ctx, envID, msgComputeProbe, nil, nil)
	if err != nil {
		return capability{}, err
	}
	var reply agentComputePayload
	if err := json.Unmarshal(raw, &reply); err != nil {
		return capability{}, fmt.Errorf("decoding the %s reply: %w", msgComputeProbe, err)
	}
	if reply.Capability == nil {
		return capability{}, errors.New("agent returned no capability report")
	}
	return convertCapability(reply.Capability), nil
}

// convertCapability keeps the host's conversion for the fields this plugin reads.
func convertCapability(in *agentCapability) capability {
	out := capability{
		CanRunCTs: in.CanRunContainers, UnsupportedWhy: in.UnsupportedWhy,
		HostCPUs: in.HostCPUs, HostMemoryBytes: in.HostMemoryBytes,
	}
	for _, item := range in.Items {
		out.Items = append(out.Items, capabilityItem(item))
	}
	return out
}

// imageList reports what the host has cached.
func (s *Service) imageList(ctx context.Context, envID string) ([]cachedImage, error) {
	if s.rpc == nil {
		return nil, ErrHostUnavailable
	}
	raw, err := s.rpc.Call(ctx, envID, msgImageList, nil, nil)
	if err != nil {
		return nil, err
	}
	var reply agentImagePayload
	if err := json.Unmarshal(raw, &reply); err != nil {
		return nil, fmt.Errorf("decoding the %s reply: %w", msgImageList, err)
	}
	out := make([]cachedImage, 0, len(reply.Cached))
	for _, image := range reply.Cached {
		out = append(out, cachedImage(image))
	}
	return out, nil
}

// poolFileRead reads one small file from a pool; relPath is relative to the pool root and
// confined by the agent, and the file travels whole in one message.
func (s *Service) poolFileRead(ctx context.Context, envID, poolID, backend, target, relPath string) ([]byte, error) {
	if s.rpc == nil {
		return nil, ErrHostUnavailable
	}
	body, err := json.Marshal(agentPoolFilePayload{
		Operation: "read", PoolID: poolID, Backend: backend, Target: target, Path: relPath,
	})
	if err != nil {
		return nil, fmt.Errorf("encoding %s: %w", msgPoolFile, err)
	}
	raw, err := s.rpc.Call(ctx, envID, msgPoolFile, body, nil)
	if err != nil {
		return nil, err
	}
	var reply agentPoolFilePayload
	if err := json.Unmarshal(raw, &reply); err != nil {
		return nil, fmt.Errorf("decoding the %s reply: %w", msgPoolFile, err)
	}
	return reply.Content, nil
}

// handleAgentFrame is the sink for the frames this plugin observes: container backup
// progress, which it routes into the run rows.
func (s *Service) handleAgentFrame(envID, msgType string, frame []byte) {
	if msgType != msgCTBackupProgress {
		return
	}
	if event, ok := backupProgressEvent(envID, frame); ok {
		s.SubscribeBackupProgress(event)
	}
}

// backupProgressEvent turns a ct_backup_progress frame into the guest event the run rows
// are written from; the detail map is passed through untouched because its vocabulary
// belongs to this plugin and the agent.
func backupProgressEvent(envID string, frame []byte) (guestEvent, bool) {
	var payload agentPayload
	if json.Unmarshal(frame, &payload) != nil {
		return guestEvent{}, false
	}
	return guestEvent{
		EnvID: envID, GuestKind: guestKindCT, GuestID: payload.ContainerID,
		Kind: "backup_progress", Detail: payload.Progress,
	}, true
}
