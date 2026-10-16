// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

import (
	"context"
	"fmt"

	"github.com/therealmcsparrow/mcharbor/core/plugin"
)

// Workflow nodes.
//
// These are infrastructure nodes, not guest-lifecycle ones — starting and
// stopping guests belongs to addon-kvm and addon-lxc, which own those objects.
// What this plugin contributes is the three things a compute workflow needs to
// *decide* with:
//
//   - "is this host actually able to run what I am about to ask for?"
//   - "is there room in the pool before I allocate another disk?"
//   - "make sure this image is on the host before the create step runs."
//
// Each returns its answer on `msg.payload` and routes a failure down the error
// port rather than failing the run, because a workflow that checks capacity is
// usually one that wants to branch on the answer.

// Node keys, plugin-local. The host exposes them on the canvas as
// `plugin:addon-compute:<key>`.
const (
	nodeCapability  = "compute-host-capability"
	nodePoolCheck   = "compute-pool-capacity"
	nodeImageEnsure = "compute-ensure-image"
)

// registerNodes attaches the three descriptors and their executors.
func (p *ComputePlugin) registerNodes(reg plugin.NodeRegistry) error {
	nodes := []struct {
		key  string
		desc plugin.NodeDescriptor
	}{
		{
			key: nodeCapability,
			desc: plugin.NodeDescriptor{
				Category:    "Compute",
				DisplayName: "Host capability",
				Description: "Read what a compute host can run. Routes to the error port when the host cannot run the guest type you ask about.",
				Icon:        "IconServerBolt",
				Inputs:      []string{"input"},
				Outputs:     []string{"output", "error"},
				// `view` rather than `manage`: this node reads and never
				// changes anything on the host.
				Requires: "compute.view",
				Fields: []map[string]any{
					{
						"key":      "environment",
						"label":    "Host",
						"type":     "environment",
						"required": false,
						"help":     "Leave empty to use the workflow run's environment.",
					},
					{
						"key":     "requireGuestKind",
						"label":   "Require support for",
						"type":    "select",
						"default": "any",
						"options": []map[string]any{
							{"value": "any", "label": "Anything"},
							{"value": "vm", "label": "Virtual machines"},
							{"value": "ct", "label": "System containers"},
						},
						"help": "Route to the error port when the host cannot run this guest type.",
					},
				},
			},
		},
		{
			key: nodePoolCheck,
			desc: plugin.NodeDescriptor{
				Category:    "Compute",
				DisplayName: "Pool capacity",
				Description: "Sample a storage pool and branch on how full it is. Use it before allocating a guest disk — a pool that fills pauses every guest on it.",
				Icon:        "IconDatabase",
				Inputs:      []string{"input"},
				Outputs:     []string{"output", "error"},
				Requires:    "compute.view",
				Fields: []map[string]any{
					{
						"key":      "environment",
						"label":    "Host",
						"type":     "environment",
						"required": false,
					},
					{
						"key":      "poolName",
						"label":    "Pool name",
						"type":     "text",
						"required": true,
					},
					{
						"key":     "maxUsedPercent",
						"label":   "Fail above",
						"type":    "number",
						"default": 85,
						"help":    "Route to the error port when the pool is fuller than this percentage.",
					},
				},
			},
		},
		{
			key: nodeImageEnsure,
			desc: plugin.NodeDescriptor{
				Category:    "Compute",
				DisplayName: "Ensure image",
				Description: "Make sure a catalog image is downloaded to a host before a create step runs. Downloads it if missing, does nothing if already there.",
				Icon:        "IconPhoto",
				Inputs:      []string{"input"},
				Outputs:     []string{"output", "error"},
				// This one downloads, so it takes `manage`.
				Requires: "compute.manage",
				Fields: []map[string]any{
					{
						"key":      "environment",
						"label":    "Host",
						"type":     "environment",
						"required": false,
					},
					{
						"key":      "imageId",
						"label":    "Image",
						"type":     "text",
						"required": true,
						"help":     "The catalog image id, from the Images page.",
					},
				},
			},
		},
	}

	for _, node := range nodes {
		desc := node.desc
		desc.Execute = p.execNode
		if err := reg.Add(node.key, desc); err != nil {
			return fmt.Errorf("compute: register node %s: %w", node.key, err)
		}
	}
	return nil
}

// execNode is the one executor behind all three nodes.
func (p *ComputePlugin) execNode(ctx context.Context, in plugin.NodeExecInput) (plugin.NodeExecOutput, error) {
	if p.svc == nil || p.svc.rpc == nil {
		return errorPort(in, "compute is not available on this installation"), nil
	}

	envID := in.EnvironmentID
	if envID == "" {
		return errorPort(in, "this node needs a host"), nil
	}

	switch in.Key {
	case nodeCapability:
		return p.execCapabilityNode(ctx, in, envID)
	case nodePoolCheck:
		return p.execPoolNode(ctx, in, envID)
	case nodeImageEnsure:
		return p.execEnsureImageNode(ctx, in, envID)
	default:
		return plugin.NodeExecOutput{}, fmt.Errorf("compute: unknown node %q", in.Key)
	}
}

func (p *ComputePlugin) execCapabilityNode(
	ctx context.Context,
	in plugin.NodeExecInput,
	envID string,
) (plugin.NodeExecOutput, error) {
	capability, err := p.svc.Probe(ctx, envID)
	if err != nil {
		return errorPort(in, err.Error()), nil
	}

	required, _ := in.Config["requireGuestKind"].(string)
	switch required {
	case guestVM:
		if !capability.CanRunVMs {
			return errorPort(in, "this host cannot run virtual machines"), nil
		}
	case guestCT:
		if !capability.CanRunCTs {
			return errorPort(in, "this host cannot run system containers"), nil
		}
	}

	// Blockers are surfaced as data rather than as an error, so a workflow can
	// report them without the run failing.
	blockers := make([]map[string]any, 0)
	for _, item := range capability.Items {
		if item.Critical && !item.Present {
			blockers = append(blockers, map[string]any{
				"key":         item.Key,
				"detail":      item.Detail,
				"remediation": item.Remediation,
				"blocks":      item.Blocks,
			})
		}
	}

	return outputPort(in, map[string]any{
		"environmentId":    envID,
		"os":               capability.OS,
		"arch":             capability.Arch,
		"kernelVersion":    capability.KernelVersion,
		"canRunVms":        capability.CanRunVMs,
		"canRunContainers": capability.CanRunCTs,
		"seccompBuiltIn":   capability.SeccompBuiltIn,
		"supervisorMode":   capability.SupervisorMode,
		"blockers":         blockers,
	}), nil
}

func (p *ComputePlugin) execPoolNode(
	ctx context.Context,
	in plugin.NodeExecInput,
	envID string,
) (plugin.NodeExecOutput, error) {
	name, _ := in.Config["poolName"].(string)
	if name == "" {
		return errorPort(in, "this node needs a pool name"), nil
	}

	pools, err := p.svc.ListPools(ctx, envID)
	if err != nil {
		return errorPort(in, err.Error()), nil
	}

	for _, pool := range pools {
		if pool.Name != name {
			continue
		}
		fresh, err := p.svc.RefreshPool(ctx, pool.ID)
		if err != nil {
			return errorPort(in, err.Error()), nil
		}

		percent := 0
		if fresh.CapacityBytes > 0 {
			percent = int(fresh.UsedBytes * 100 / fresh.CapacityBytes)
		}
		payload := map[string]any{
			"environmentId":  envID,
			"poolId":         fresh.ID,
			"name":           fresh.Name,
			"backend":        fresh.Backend,
			"capacityBytes":  fresh.CapacityBytes,
			"usedBytes":      fresh.UsedBytes,
			"allocatedBytes": fresh.AllocatedBytes,
			"usedPercent":    percent,
			"volumeCount":    fresh.VolumeCount,
			"instantClones":  fresh.Instant,
		}

		if limit := configInt(in.Config, "maxUsedPercent", 85); percent > limit {
			// The payload rides the error port too, so the branch that handles
			// a full pool can still say how full.
			return plugin.NodeExecOutput{
				Port: "error",
				Msg:  withPayload(in.Msg, payload, fmt.Sprintf("pool %s is %d%% full", name, percent)),
			}, nil
		}
		return outputPort(in, payload), nil
	}

	return errorPort(in, fmt.Sprintf("no pool named %q on this host", name)), nil
}

func (p *ComputePlugin) execEnsureImageNode(
	ctx context.Context,
	in plugin.NodeExecInput,
	envID string,
) (plugin.NodeExecOutput, error) {
	imageID, _ := in.Config["imageId"].(string)
	if imageID == "" {
		return errorPort(in, "this node needs an image"), nil
	}

	images, err := p.svc.ListImages(ctx, envID, "")
	if err != nil {
		return errorPort(in, err.Error()), nil
	}

	for _, image := range images {
		if image.ID != imageID {
			continue
		}
		// Already downloaded: nothing to do, and saying so is more useful than
		// re-downloading.
		if image.Cache != nil && image.Cache.State == CacheStateReady {
			return outputPort(in, map[string]any{
				"environmentId":  envID,
				"imageId":        image.ID,
				"displayName":    image.DisplayName,
				"localPath":      image.Cache.LocalPath,
				"verified":       image.Cache.Verified,
				"alreadyPresent": true,
			}), nil
		}

		// A workflow cannot type an acknowledgement, so an image with no
		// checksum is refused here rather than waived silently. The operator
		// downloads it once by hand, with the waiver on their own audit row.
		cache, err := p.svc.FetchImage(ctx, imageID, ImageFetchRequest{EnvironmentID: envID})
		if err != nil {
			return errorPort(in, err.Error()), nil
		}
		return outputPort(in, map[string]any{
			"environmentId":  envID,
			"imageId":        image.ID,
			"displayName":    image.DisplayName,
			"localPath":      cache.LocalPath,
			"verified":       cache.Verified,
			"alreadyPresent": false,
		}), nil
	}

	return errorPort(in, fmt.Sprintf("no catalog image with id %q", imageID)), nil
}

// outputPort routes a payload down the success port.
func outputPort(in plugin.NodeExecInput, payload map[string]any) plugin.NodeExecOutput {
	return plugin.NodeExecOutput{Port: "output", Msg: withPayload(in.Msg, payload, "")}
}

// errorPort routes a reason down the error port.
//
// Returning an error from the executor would fail the whole run; routing keeps
// the decision with the workflow, which is what a check node is for.
func errorPort(in plugin.NodeExecInput, reason string) plugin.NodeExecOutput {
	return plugin.NodeExecOutput{Port: "error", Msg: withPayload(in.Msg, nil, reason)}
}

// withPayload copies the incoming message and sets payload / error on it.
//
// The copy matters: the same msg object reaches every branch of a fan-out, and
// mutating it in place would have one branch's result appear in another's.
func withPayload(msg map[string]any, payload map[string]any, errText string) map[string]any {
	out := make(map[string]any, len(msg)+2)
	for key, value := range msg {
		out[key] = value
	}
	if payload != nil {
		out["payload"] = payload
	}
	if errText != "" {
		out["error"] = errText
	}
	return out
}

// configInt reads a numeric config value, tolerating the float64 a JSON decode
// produces.
func configInt(config map[string]any, key string, fallback int) int {
	switch value := config[key].(type) {
	case float64:
		return int(value)
	case int:
		return value
	case int64:
		return int(value)
	default:
		return fallback
	}
}
