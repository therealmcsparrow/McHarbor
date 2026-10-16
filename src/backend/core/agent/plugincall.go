// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"sync"
	"time"
)

// Call sends one plugin-routable message and returns the agent's reply slot as JSON.
//
// The host builds the envelope; the plugin supplies only the one slot's payload. That
// payload is decoded into the core/agent struct for the route's slot, so unknown fields are
// dropped and nothing reaches a second envelope field, and EnvID is overwritten with envID.
// The budget, version floor, fixed operation and cancellation come from the ceiling, never
// from the caller. Net ensure and remove answer nil on success, as the typed NetEnsure and
// NetRemove do. For the probe, location, relocation, pool, link and image routes an agent
// refusal returns the reply slot beside the error, as the typed methods return what the
// agent still reported.
//
// onProgress receives the route's declared progress frames only, each as its slot's JSON,
// from a goroutine bounded by this call; nil drops them as the typed calls do. When ctx ends
// first on a route with CancelWith, the host sends that message under the request's id.
func (t *AgentTransport) Call(
	ctx context.Context,
	envID, msgType string,
	payload []byte,
	onProgress func(msgType string, frame []byte),
) ([]byte, error) {
	route, ok := PluginRouteFor(msgType)
	if !ok {
		return nil, fmt.Errorf("%w: %q", ErrNotPluginRoutable, msgType)
	}
	if !t.ComputeSupported() {
		return nil, fmt.Errorf("%w: this agent is %s and compute needs %s or newer",
			ErrComputeUnsupported, t.conn.Version, ComputeMinVersion)
	}
	if err := route.Floor.Check(t.conn, msgType, payload); err != nil {
		return nil, err
	}

	var onFrame func(*WSMessage)
	if onProgress != nil && len(route.Progress) > 0 {
		onFrame = func(frame *WSMessage) {
			if !slices.Contains(route.Progress, frame.Type) {
				return
			}
			data, err := slotPayload(frame, route.Slot)
			if err != nil || data == nil {
				return
			}
			onProgress(frame.Type, data)
		}
	}

	switch route.Slot {
	case SlotVM:
		var vm VMPayload
		if err := decodeSlot(msgType, payload, &vm); err != nil {
			return nil, err
		}
		vm.EnvID = envID
		reply, err := t.vmCommand(ctx, msgType, vm, route.Budget.Timeout(), onFrame)
		if err != nil {
			return nil, err
		}
		return json.Marshal(reply)
	case SlotCT:
		var ct CTPayload
		if err := decodeSlot(msgType, payload, &ct); err != nil {
			return nil, err
		}
		ct.EnvID = envID
		reply, err := t.ctCommand(ctx, msgType, ct, route.Budget.Timeout(), onFrame)
		if err != nil {
			return nil, err
		}
		return json.Marshal(reply)
	case SlotNet:
		var net ComputeNetPayload
		if err := decodeSlot(msgType, payload, &net); err != nil {
			return nil, err
		}
		net.EnvID = envID
		switch msgType {
		case MsgNetEnsureRequest:
			return nil, t.NetEnsure(ctx, net)
		case MsgNetRemoveRequest:
			return nil, t.NetRemove(ctx, net)
		}
		detect := ComputeNetPayload{Operation: route.Operation, EnvID: envID}
		return t.callSlot(ctx, route, func(msg *WSMessage) { msg.Net = &detect }, onFrame, nil)
	case SlotCompute:
		var c ComputePayload
		if err := decodeSlot(msgType, payload, &c); err != nil {
			return nil, err
		}
		req := ComputePayload{Operation: route.Operation, EnvID: envID, Roots: c.Roots, Migrate: c.Migrate}
		if c.Install != nil {
			// Only the item crosses: the agent picks the packages, so a plugin cannot name any.
			req.Install = &ComputeInstall{Item: c.Install.Item}
		}
		return t.callSlot(ctx, route, func(msg *WSMessage) { msg.Compute = &req }, onFrame, nil)
	case SlotPool:
		var p ComputePoolPayload
		if err := decodeSlot(msgType, payload, &p); err != nil {
			return nil, err
		}
		if route.Operation != "" {
			p.Operation = route.Operation
		} else if !slices.Contains(poolFileOperations, p.Operation) {
			return nil, fmt.Errorf("%w: %q is not a pool file operation", ErrNotPluginRoutable, p.Operation)
		}
		p.EnvID = envID
		p.Detected, p.Stats, p.Entries, p.Success, p.Error = nil, nil, nil, false, ""
		return t.callSlot(ctx, route, func(msg *WSMessage) { msg.Pool = &p }, onFrame, nil)
	case SlotComputeImage:
		var img ComputeImagePayload
		if err := decodeSlot(msgType, payload, &img); err != nil {
			return nil, err
		}
		img = imageRequest(img)
		img.Operation, img.EnvID = route.Operation, envID
		var abandon func(reqID string)
		if route.CancelWith != "" {
			abandon = func(reqID string) { t.cancelImage(route.CancelWith, reqID, img.ImageID) }
		}
		return t.callSlot(ctx, route, func(msg *WSMessage) { msg.ComputeImage = &img }, onFrame, abandon)
	}
	return nil, fmt.Errorf("%w: %q has no sendable slot", ErrNotPluginRoutable, msgType)
}

// callSlot is one round trip for a route whose reply the typed transport methods judge with
// replyErr. On an agent refusal the reply slot is returned beside the error, because the
// typed methods hand back what the agent still reported (the locations in force, a partial
// relocation) and a plugin moving off them needs the same.
func (t *AgentTransport) callSlot(
	ctx context.Context,
	route PluginRoute,
	build func(*WSMessage),
	onFrame func(*WSMessage),
	onAbandon func(reqID string),
) ([]byte, error) {
	reply, err := t.computeCall(ctx, route.Type, build, routeTimeout(route, build), onFrame, onAbandon)
	if err != nil {
		return nil, err
	}
	checkErr := replyErr(route.Type, reply)
	data, err := slotPayload(reply, route.Slot)
	if err != nil {
		return nil, fmt.Errorf("encoding the %s reply: %w", route.Type, err)
	}
	return data, checkErr
}

// routeTimeout is the budget a route runs under. A relocation is the one payload-dependent
// budget: an apply takes the relocate budget, a plan or a repair the quick one, exactly as
// the typed ComputeMigrate decides.
func routeTimeout(route PluginRoute, build func(*WSMessage)) time.Duration {
	if route.Budget != BudgetRelocate {
		return route.Budget.Timeout()
	}
	var probe WSMessage
	build(&probe)
	if probe.Compute == nil || probe.Compute.Migrate == nil || probe.Compute.Migrate.Plan || probe.Compute.Migrate.Repair {
		return computeTimeout
	}
	return route.Budget.Timeout()
}

// imageRequest keeps only the fields the typed image calls send, so a plugin cannot put
// reply, progress or rootfs-import fields on the wire.
func imageRequest(in ComputeImagePayload) ComputeImagePayload {
	return ComputeImagePayload{
		ImageID: in.ImageID, Kind: in.Kind, SourceURL: in.SourceURL, ChecksumURL: in.ChecksumURL,
		Checksum: in.Checksum, ChecksumAlgo: in.ChecksumAlgo, SignatureURL: in.SignatureURL,
		AllowUnverified: in.AllowUnverified, FromLibrary: in.FromLibrary, Filename: in.Filename,
		LibraryURL: in.LibraryURL, LibraryToken: in.LibraryToken,
	}
}

// cancelImage tells the agent to stop a fetch nobody is waiting for any more, the frame the
// typed ImageFetch sends on the same condition.
func (t *AgentTransport) cancelImage(cancelType, reqID, imageID string) {
	if err := t.conn.WriteJSON(WSMessage{
		Type:         cancelType,
		ID:           reqID,
		ComputeImage: &ComputeImagePayload{Operation: "cancel", ImageID: imageID},
	}); err != nil {
		t.logger.Debug("image fetch cancel failed", "id", reqID, "error", err)
	}
}

// decodeSlot keeps the wording the retired compute bridge always used for a bad payload.
func decodeSlot(msgType string, payload []byte, into any) error {
	if len(payload) == 0 {
		return nil
	}
	if err := json.Unmarshal(payload, into); err != nil {
		return fmt.Errorf("compute: decode %s payload: %w", msgType, err)
	}
	return nil
}

// pluginObserverMu guards the process-wide observer of unsolicited plugin-observable frames.
// One per process, because transports come and go with reconnects while plugins do not.
var (
	pluginObserverMu sync.RWMutex
	pluginObserver   func(envID, msgType string, frame []byte)
)

// SetPluginFrameObserver installs the receiver of every observable unsolicited frame, as its
// slot's JSON. It runs on the transport's read loop, so it must not block.
func SetPluginFrameObserver(fn func(envID, msgType string, frame []byte)) {
	pluginObserverMu.Lock()
	pluginObserver = fn
	pluginObserverMu.Unlock()
}

// notifyPluginObserver hands an observable frame to the observer. Encoding happens only when
// one is installed, so an installation with no agent-rpc plugin pays nothing.
func notifyPluginObserver(envID string, msg *WSMessage) {
	pluginObserverMu.RLock()
	fn := pluginObserver
	pluginObserverMu.RUnlock()
	if fn == nil {
		return
	}
	observed, ok := PluginObservedFor(msg.Type)
	if !ok {
		return
	}
	data, err := slotPayload(msg, observed.Slot)
	if err != nil || data == nil {
		return
	}
	fn(envID, msg.Type, data)
}
