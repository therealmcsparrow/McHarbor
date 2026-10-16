// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package agent

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/rs/xid"

	"github.com/therealmcsparrow/mcharbor/core/telemetry"
)

// The host side of the compute transport.
//
// This is the counterpart to snapshot.go and follows the same shape: register a
// waiter, write the request, block until the agent answers. What differs is the
// buffering policy, and it differs deliberately.
//
// # Compute commands are never buffered offline
//
// The agent's offline store replays buffered messages when a partition heals.
// That is right for stats and for DHCP leases, which are stale information at
// worst. It is wrong for "start this VM": a start replayed twenty minutes after
// the operator gave up and started it somewhere else is a correctness hazard,
// not a convenience. So every call here fails fast with a reconnect prompt and
// nothing compute-related is ever queued.

// computeTimeout bounds an ordinary compute round trip. The agent answers these
// from local state or a short syscall.
const computeTimeout = 60 * time.Second

// computeLongTimeout bounds the operations that genuinely take time on the
// agent side: creating a large disk, importing a rootfs, taking a snapshot with
// a guest freeze.
const computeLongTimeout = 30 * time.Minute

// computeImportTimeout bounds a guest import, which is in a different class
// again: the agent pulls the whole disk image off McHarbor before it can read
// or convert it, so the round trip is bounded by the size of the file and the
// speed of the link rather than by any work the agent does.
//
// It matches the agent's own importPullTimeout. They were 30 minutes here and
// six hours there, so a 39 GB VHDX had the server give up on a transfer the
// agent was still happily running — reported to the operator as "context
// deadline exceeded" with nothing to say which side had stopped waiting.
const computeImportTimeout = 6 * time.Hour

var (
	// ErrComputeUnsupported means the connected agent is too old. The service
	// layer maps this to an i18n code and the UI offers an inline update
	// action rather than a generic failure.
	ErrComputeUnsupported = errors.New("agent does not support compute")
	// ErrAgentOffline means there is no live transport for this environment.
	ErrAgentOffline = errors.New("agent is offline")
	// ErrUplinkBusy means the host refused the adapter an external network was
	// to be built on. The agent names what is stacked on it; the reason has to
	// reach the operator, so it is typed rather than logged.
	ErrUplinkBusy = errors.New("adapter is in use on the host")

	// ErrHostLinkNameTaken means the host already has an interface of the requested
	// name that McHarbor does not own; the agent never takes one over.
	ErrHostLinkNameTaken = errors.New("host interface name is taken")
	// ErrHostLinkParentNotFound means the adapter a host interface stacks on is not on the host.
	ErrHostLinkParentNotFound = errors.New("parent interface not found on the host")
	// ErrHostLinkPermissionDenied means the agent lacks CAP_NET_ADMIN on the host.
	ErrHostLinkPermissionDenied = errors.New("agent may not change host interfaces")
)

// ComputeSupported reports whether the connected agent can host guests.
func (t *AgentTransport) ComputeSupported() bool {
	return t.conn.SupportsFeature(ComputeMinVersion)
}

// dispatchComputeEvent hands an unsolicited guest frame to the plugin frame observer, which
// the agent-rpc bridge installs once at wiring time and fans out to the plugins that declared
// the type. A frame nobody observes is dropped: events are additive, and an installation with
// no compute plugin has nothing to hand them to.
func (t *AgentTransport) dispatchComputeEvent(msg *WSMessage) {
	notifyPluginObserver(t.conn.EnvID, msg)
}

// computeRPC is the one round trip every call below shares.
func (t *AgentTransport) computeRPC(
	ctx context.Context,
	messageType string,
	build func(*WSMessage),
	timeout time.Duration,
) (*WSMessage, error) {
	return t.computeCall(ctx, messageType, build, timeout, nil, nil)
}

// computeCall is computeRPC with an optional progress receiver. With onFrame set, frames the
// agent streams under the request's id are handed to it from a goroutine bounded by this
// call; without it nothing is registered and progress frames are dropped as before.
// onAbandon, when set, runs with the request id if the caller's context or the budget ends
// first, so a request the agent would keep working on can be cancelled there too.
func (t *AgentTransport) computeCall(
	ctx context.Context,
	messageType string,
	build func(*WSMessage),
	timeout time.Duration,
	onFrame func(*WSMessage),
	onAbandon func(reqID string),
) (r0 *WSMessage, err error) {
	observe := telemetry.AgentRPC(t.conn.EnvID, messageType)
	defer func() { observe(err) }()
	if !t.ComputeSupported() {
		return nil, fmt.Errorf("%w: this agent is %s and compute needs %s or newer",
			ErrComputeUnsupported, t.conn.Version, ComputeMinVersion)
	}

	if timeout <= 0 {
		timeout = computeTimeout
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	reqID := xid.New().String()
	ch := make(chan *WSMessage, 1)

	// Registered before the write. A reply that arrived first would have no
	// waiter to go to and the call would hang until its own timeout — the same
	// ordering hazard as the console stream, in a smaller place.
	var progressCh chan *WSMessage
	if onFrame != nil {
		progressCh = make(chan *WSMessage, 8)
	}
	t.mu.Lock()
	t.computeWaiters[reqID] = ch
	if progressCh != nil {
		t.computeStreams[reqID] = progressCh
	}
	t.mu.Unlock()
	defer func() {
		t.mu.Lock()
		delete(t.computeWaiters, reqID)
		delete(t.computeStreams, reqID)
		t.mu.Unlock()
	}()

	msg := WSMessage{Type: messageType, ID: reqID}
	build(&msg)

	if err := t.conn.WriteJSON(msg); err != nil {
		return nil, fmt.Errorf("sending %s to agent: %w", messageType, err)
	}

	if progressCh != nil {
		done := make(chan struct{})
		stopped := make(chan struct{})
		// goroutine lifecycle: ends when this call returns and closes done.
		go func() {
			defer close(stopped)
			for {
				select {
				case <-done:
					return
				case frame := <-progressCh:
					if frame != nil {
						onFrame(frame)
					}
				}
			}
		}()
		// The read loop queues a progress frame before the reply that follows it, so
		// draining after the pump stops hands over every frame that preceded the reply,
		// and onFrame is never called once this call has returned.
		defer func() {
			close(done)
			<-stopped
			for {
				select {
				case frame := <-progressCh:
					if frame != nil {
						onFrame(frame)
					}
				default:
					return
				}
			}
		}()
	}

	select {
	case <-ctx.Done():
		if onAbandon != nil {
			onAbandon(reqID)
		}
		return nil, ctx.Err()
	case reply := <-ch:
		if reply == nil {
			return nil, fmt.Errorf("empty %s response from agent", messageType)
		}
		return reply, nil
	case <-t.done:
		// Deliberately not queued for replay. See the package comment.
		return nil, fmt.Errorf("%w: the agent disconnected while the operation was in flight",
			ErrAgentOffline)
	}
}

// ComputeProbe asks the agent what its host can run.
func (t *AgentTransport) ComputeProbe(ctx context.Context, envID string) (*ComputeCapability, error) {
	reply, err := t.computeRPC(ctx, MsgComputeProbeRequest, func(msg *WSMessage) {
		msg.Compute = &ComputePayload{Operation: "probe", EnvID: envID}
	}, computeTimeout)
	if err != nil {
		return nil, err
	}
	if err := replyErr(MsgComputeProbeRequest, reply); err != nil {
		return nil, err
	}
	return reply.Compute.Capability, nil
}

// ComputeInstall asks the agent to install the packages for one capability item. The agent
// answers with what it ran and a fresh capability report, also when the install failed.
func (t *AgentTransport) ComputeInstall(ctx context.Context, envID, item string) (*ComputePayload, error) {
	reply, err := t.computeRPC(ctx, MsgComputeInstallRequest, func(msg *WSMessage) {
		msg.Compute = &ComputePayload{Operation: "install", EnvID: envID, Install: &ComputeInstall{Item: item}}
	}, computeLongTimeout)
	if err != nil {
		return nil, err
	}
	return reply.Compute, replyErr(MsgComputeInstallRequest, reply)
}

// replyErr judges the agent's answer to one of the typed compute requests. The typed
// methods and Call share it, so the two paths cannot word the same failure differently.
func replyErr(msgType string, reply *WSMessage) error {
	if reply == nil {
		reply = &WSMessage{}
	}
	switch msgType {
	case MsgComputeProbeRequest:
		// The agent answers a refusal on the same message type with the reason
		// filled in and no report — an unreachable systemd bus, a missing binary,
		// a runtime that would not start. Reporting "no capability report" for
		// those would replace the one sentence that says what to fix.
		c := reply.Compute
		return computeReplyErr(c, "capability report", "the capability probe", c != nil && c.Capability != nil)
	case MsgComputeConfigRequest:
		c := reply.Compute
		return computeReplyErr(c, "location report", "the compute locations", c != nil && c.Roots != nil)
	case MsgComputeInstallRequest:
		c := reply.Compute
		return computeReplyErr(c, "install report", "the package install", c != nil && c.Install != nil)
	case MsgComputeMigrateRequest:
		c := reply.Compute
		return computeReplyErr(c, "migration report", "the migration", c != nil && c.Migrate != nil)
	case MsgPoolDetectRequest:
		if reply.Pool == nil {
			return errors.New("agent returned no pool detection result")
		}
	case MsgPoolCreateRequest, MsgPoolStatsRequest, MsgPoolRemoveRequest, MsgPoolBrowseRequest, MsgPoolFileRequest:
		_, err := requirePoolSuccess(reply, poolWhat[msgType])
		return err
	case MsgNetDetectRequest:
		if reply.Net == nil {
			return errors.New("agent returned no link inventory")
		}
	case MsgImageFetchRequest:
		if reply.ComputeImage == nil {
			return errors.New("agent returned no image fetch result")
		}
		if !reply.ComputeImage.Success {
			return fmt.Errorf("agent failed to fetch image: %s", strings.TrimSpace(reply.ComputeImage.Error))
		}
	case MsgImageListRequest:
		if reply.ComputeImage == nil {
			return errors.New("agent returned no image inventory")
		}
	case MsgImageRemoveRequest:
		if reply.ComputeImage == nil {
			return errors.New("agent returned no result for image removal")
		}
		if !reply.ComputeImage.Success {
			return fmt.Errorf("agent failed to evict image: %s", strings.TrimSpace(reply.ComputeImage.Error))
		}
	}
	return nil
}

// poolWhat names each pool request in its failure, as the typed methods always have.
var poolWhat = map[string]string{
	MsgPoolCreateRequest: "create pool",
	MsgPoolStatsRequest:  "sample pool",
	MsgPoolRemoveRequest: "remove pool",
	MsgPoolBrowseRequest: "browse pool",
	MsgPoolFileRequest:   "pool file operation",
}

// computeReplyErr is the compute family's judgement: no payload, a refusal with or without
// the agent's reason, or a success that carries no report.
func computeReplyErr(c *ComputePayload, report, refused string, hasReport bool) error {
	if c == nil {
		return fmt.Errorf("agent returned no %s", report)
	}
	if !c.Success {
		if c.Error != "" {
			return fmt.Errorf("agent refused %s: %s", refused, c.Error)
		}
		return fmt.Errorf("agent refused %s", refused)
	}
	if !hasReport {
		return fmt.Errorf("agent returned no %s", report)
	}
	return nil
}

// ComputeConfigure tells the agent where to keep VM state, container rootfs
// trees and cached images, and returns the locations actually in effect.
//
// The agent declines a move that would strand a running guest, so the reply is
// read rather than assumed: it is the only thing that says whether the
// operator's new location is live or merely stored.
func (t *AgentTransport) ComputeConfigure(ctx context.Context, envID string, roots ComputeRoots) (*ComputeRoots, error) {
	reply, err := t.computeRPC(ctx, MsgComputeConfigRequest, func(msg *WSMessage) {
		msg.Compute = &ComputePayload{Operation: "config", EnvID: envID, Roots: &roots}
	}, computeTimeout)
	if err != nil {
		return nil, err
	}
	if err := replyErr(MsgComputeConfigRequest, reply); err != nil {
		if reply.Compute != nil {
			return reply.Compute.Roots, err
		}
		return nil, err
	}
	return reply.Compute.Roots, nil
}

// ComputeMigrate asks the host to move what it already holds to the new
// locations, or — with plan set — to report what that would involve.
//
// The timeout is the long one: this is a disk-sized copy on the far side, and
// the only thing that knows how long it takes is the disk.
func (t *AgentTransport) ComputeMigrate(ctx context.Context, envID string, req ComputeMigrate) (*ComputeMigrate, error) {
	timeout := computeTimeout
	if !req.Plan && !req.Repair {
		timeout = computeMigrateTimeout
	}
	reply, err := t.computeRPC(ctx, MsgComputeMigrateRequest, func(msg *WSMessage) {
		msg.Compute = &ComputePayload{Operation: "migrate", EnvID: envID, Migrate: &req}
	}, timeout)
	if err != nil {
		return nil, err
	}
	if err := replyErr(MsgComputeMigrateRequest, reply); err != nil {
		if reply.Compute != nil {
			return reply.Compute.Migrate, err
		}
		return nil, err
	}
	return reply.Compute.Migrate, nil
}

// computeMigrateTimeout bounds a relocation. Copying a host's guests across
// filesystems is measured in hours, and the operator's cancel is the real
// control.
const computeMigrateTimeout = 12 * time.Hour

// PoolDetect asks which pool backends exist on the host.
func (t *AgentTransport) PoolDetect(ctx context.Context, envID string) ([]ComputePoolBackend, error) {
	reply, err := t.computeRPC(ctx, MsgPoolDetectRequest, func(msg *WSMessage) {
		msg.Pool = &ComputePoolPayload{Operation: "detect", EnvID: envID}
	}, computeTimeout)
	if err != nil {
		return nil, err
	}
	if err := replyErr(MsgPoolDetectRequest, reply); err != nil {
		return nil, err
	}
	return reply.Pool.Detected, nil
}

// PoolCreate initializes a pool on the host.
func (t *AgentTransport) PoolCreate(ctx context.Context, payload ComputePoolPayload) (*ComputePoolPayload, error) {
	payload.Operation = "create"
	reply, err := t.computeRPC(ctx, MsgPoolCreateRequest, func(msg *WSMessage) {
		msg.Pool = &payload
	}, computeLongTimeout)
	if err != nil {
		return nil, err
	}
	return requirePoolSuccess(reply, "create pool")
}

// PoolStats samples a pool's capacity.
func (t *AgentTransport) PoolStats(ctx context.Context, payload ComputePoolPayload) (*ComputePoolStats, error) {
	payload.Operation = "stats"
	reply, err := t.computeRPC(ctx, MsgPoolStatsRequest, func(msg *WSMessage) {
		msg.Pool = &payload
	}, computeTimeout)
	if err != nil {
		return nil, err
	}
	result, err := requirePoolSuccess(reply, "sample pool")
	if err != nil {
		return nil, err
	}
	return result.Stats, nil
}

// PoolRemove detaches a pool.
func (t *AgentTransport) PoolRemove(ctx context.Context, payload ComputePoolPayload) error {
	payload.Operation = "remove"
	reply, err := t.computeRPC(ctx, MsgPoolRemoveRequest, func(msg *WSMessage) {
		msg.Pool = &payload
	}, computeLongTimeout)
	if err != nil {
		return err
	}
	_, err = requirePoolSuccess(reply, "remove pool")
	return err
}

// PoolBrowse lists one directory inside a pool.
func (t *AgentTransport) PoolBrowse(ctx context.Context, payload ComputePoolPayload) ([]ComputePoolFile, error) {
	payload.Operation = "browse"
	reply, err := t.computeRPC(ctx, MsgPoolBrowseRequest, func(msg *WSMessage) {
		msg.Pool = &payload
	}, computeTimeout)
	if err != nil {
		return nil, err
	}
	result, err := requirePoolSuccess(reply, "browse pool")
	if err != nil {
		return nil, err
	}
	return result.Entries, nil
}

// PoolFile performs one read, write, mkdir, delete or rename in a pool, and
// returns the file's bytes when the operation was a read.
func (t *AgentTransport) PoolFile(ctx context.Context, payload ComputePoolPayload) ([]byte, error) {
	reply, err := t.computeRPC(ctx, MsgPoolFileRequest, func(msg *WSMessage) {
		msg.Pool = &payload
	}, computeTimeout)
	if err != nil {
		return nil, err
	}
	result, err := requirePoolSuccess(reply, "pool file operation")
	if err != nil {
		return nil, err
	}
	return result.Content, nil
}

func requirePoolSuccess(reply *WSMessage, what string) (*ComputePoolPayload, error) {
	if reply.Pool == nil {
		return nil, fmt.Errorf("agent returned no result for %s", what)
	}
	if !reply.Pool.Success {
		return reply.Pool, fmt.Errorf("agent failed to %s: %s", what, strings.TrimSpace(reply.Pool.Error))
	}
	return reply.Pool, nil
}

// NetDetect enumerates the host's links and bridges.
func (t *AgentTransport) NetDetect(ctx context.Context, envID string) ([]ComputeHostLink, error) {
	reply, err := t.computeRPC(ctx, MsgNetDetectRequest, func(msg *WSMessage) {
		msg.Net = &ComputeNetPayload{Operation: "detect", EnvID: envID}
	}, computeTimeout)
	if err != nil {
		return nil, err
	}
	if err := replyErr(MsgNetDetectRequest, reply); err != nil {
		return nil, err
	}
	return reply.Net.Links, nil
}

// NetEnsure creates or verifies a managed network.
//
// External mode is the one case with a second round trip. Adopting the host's
// own adapter arms a rollback on the agent, and this reply arriving at all is
// the proof the change did not cut the host off — so the confirmation is sent
// the moment it does. If the adapter took the host down, nothing arrives here,
// nothing is confirmed, and the agent restores it on its own.
func (t *AgentTransport) NetEnsure(ctx context.Context, payload ComputeNetPayload) error {
	if err := guestNetworkingFloor(t.conn, payload.Mode); err != nil {
		return err
	}
	if netUsesHostLinks(payload) {
		if err := t.hostLinksFloor(); err != nil {
			return err
		}
	}
	payload.Operation = "ensure"
	reply, err := t.computeRPC(ctx, MsgNetEnsureRequest, func(msg *WSMessage) {
		msg.Net = &payload
	}, computeLongTimeout)
	if err != nil {
		return err
	}
	if err := requireNetSuccess(reply, "create network"); err != nil {
		return err
	}
	if payload.Mode == "external" {
		return t.NetAdoptConfirm(ctx, payload.NetworkID)
	}
	return nil
}

// NetAdoptConfirm cancels an external switch's rollback timer.
func (t *AgentTransport) NetAdoptConfirm(ctx context.Context, networkID string) error {
	reply, err := t.computeRPC(ctx, MsgNetAdoptConfirm, func(msg *WSMessage) {
		msg.Net = &ComputeNetPayload{Operation: "confirm", NetworkID: networkID}
	}, computeTimeout)
	if err != nil {
		return err
	}
	return requireNetSuccess(reply, "confirm external network")
}

// NetRemove tears a managed network down.
func (t *AgentTransport) NetRemove(ctx context.Context, payload ComputeNetPayload) error {
	payload.Operation = "remove"
	reply, err := t.computeRPC(ctx, MsgNetRemoveRequest, func(msg *WSMessage) {
		msg.Net = &payload
	}, computeTimeout)
	if err != nil {
		return err
	}
	return requireNetSuccess(reply, "remove network")
}

func requireNetSuccess(reply *WSMessage, what string) error {
	if reply.Net == nil {
		return fmt.Errorf("agent returned no result for %s", what)
	}
	if !reply.Net.Success {
		detail := strings.TrimSpace(reply.Net.Error)
		switch reply.Net.ErrorCode {
		case "uplink_busy":
			return fmt.Errorf("%w: %s", ErrUplinkBusy, detail)
		case HostLinkErrNameTaken:
			return fmt.Errorf("%w: %s", ErrHostLinkNameTaken, detail)
		case HostLinkErrParentNotFound:
			return fmt.Errorf("%w: %s", ErrHostLinkParentNotFound, detail)
		case HostLinkErrPermissionDenied:
			return fmt.Errorf("%w: %s", ErrHostLinkPermissionDenied, detail)
		}
		return fmt.Errorf("agent failed to %s: %s", what, detail)
	}
	return nil
}

// ImageFetch downloads and verifies an image on the host, reporting progress.
//
// onProgress is called from a dedicated goroutine whose lifetime is bounded by
// this call, so it stops when the fetch does.
func (t *AgentTransport) ImageFetch(
	ctx context.Context,
	payload ComputeImagePayload,
	onProgress func(*ComputeImagePayload),
) (*ComputeImagePayload, error) {
	if !t.ComputeSupported() {
		return nil, fmt.Errorf("%w: this agent is %s and compute needs %s or newer",
			ErrComputeUnsupported, t.conn.Version, ComputeMinVersion)
	}

	if err := imageLibraryFloor(t.conn, payload.FromLibrary); err != nil {
		return nil, err
	}

	payload.Operation = "fetch"
	reqID := xid.New().String()

	replyCh := make(chan *WSMessage, 1)
	progressCh := make(chan *WSMessage, 8)

	t.mu.Lock()
	t.computeWaiters[reqID] = replyCh
	t.computeStreams[reqID] = progressCh
	t.mu.Unlock()
	defer func() {
		t.mu.Lock()
		delete(t.computeWaiters, reqID)
		delete(t.computeStreams, reqID)
		t.mu.Unlock()
	}()

	// An image fetch runs for as long as the mirror takes. The caller's context
	// is the only bound, which is what makes the UI's cancel button real.
	if err := t.conn.WriteJSON(WSMessage{
		Type:         MsgImageFetchRequest,
		ID:           reqID,
		ComputeImage: &payload,
	}); err != nil {
		return nil, fmt.Errorf("sending image fetch to agent: %w", err)
	}

	// The progress pump stops when this function returns, via done. Every
	// goroutine gets a stop path.
	done := make(chan struct{})
	defer close(done)
	if onProgress != nil {
		go func() {
			for {
				select {
				case <-done:
					return
				case msg := <-progressCh:
					if msg != nil && msg.ComputeImage != nil {
						onProgress(msg.ComputeImage)
					}
				}
			}
		}()
	}

	select {
	case <-ctx.Done():
		// Tell the agent to stop downloading rather than leaving it to finish
		// a fetch nobody is waiting for and fill the cache.
		if err := t.conn.WriteJSON(WSMessage{
			Type:         MsgImageCancelRequest,
			ID:           reqID,
			ComputeImage: &ComputeImagePayload{Operation: "cancel", ImageID: payload.ImageID},
		}); err != nil {
			t.logger.Debug("image fetch cancel failed", "id", reqID, "error", err)
		}
		return nil, ctx.Err()
	case reply := <-replyCh:
		if err := replyErr(MsgImageFetchRequest, reply); err != nil {
			if reply != nil && reply.ComputeImage != nil {
				return reply.ComputeImage, err
			}
			return nil, err
		}
		return reply.ComputeImage, nil
	case <-t.done:
		return nil, fmt.Errorf("%w: the agent disconnected during the image fetch", ErrAgentOffline)
	}
}

// ImageList reports what the host has cached.
func (t *AgentTransport) ImageList(ctx context.Context, envID string) ([]ComputeCachedImage, error) {
	reply, err := t.computeRPC(ctx, MsgImageListRequest, func(msg *WSMessage) {
		msg.ComputeImage = &ComputeImagePayload{Operation: "list", EnvID: envID}
	}, computeTimeout)
	if err != nil {
		return nil, err
	}
	if err := replyErr(MsgImageListRequest, reply); err != nil {
		return nil, err
	}
	return reply.ComputeImage.Cached, nil
}

// ImageRemove evicts a cached image.
func (t *AgentTransport) ImageRemove(ctx context.Context, envID, imageID string) error {
	reply, err := t.computeRPC(ctx, MsgImageRemoveRequest, func(msg *WSMessage) {
		msg.ComputeImage = &ComputeImagePayload{Operation: "remove", EnvID: envID, ImageID: imageID}
	}, computeTimeout)
	if err != nil {
		return err
	}
	return replyErr(MsgImageRemoveRequest, reply)
}

// VMCommand sends one vm-family command and waits for its result.
func (t *AgentTransport) VMCommand(
	ctx context.Context,
	messageType string,
	payload VMPayload,
	timeout time.Duration,
) (*VMPayload, error) {
	return t.vmCommand(ctx, messageType, payload, timeout, nil)
}

func (t *AgentTransport) vmCommand(
	ctx context.Context,
	messageType string,
	payload VMPayload,
	timeout time.Duration,
	onFrame func(*WSMessage),
) (*VMPayload, error) {
	reply, err := t.computeCall(ctx, messageType, func(msg *WSMessage) {
		msg.VM = &payload
	}, timeout, onFrame, nil)
	if err != nil {
		return nil, err
	}
	if reply.VM == nil {
		return nil, fmt.Errorf("agent returned no result for %s", messageType)
	}
	if !reply.VM.Success && reply.VM.Error != "" {
		// The raw text is returned to the service layer, which logs it and
		// hands the client an i18n code plus a support id. It never reaches a
		// client verbatim.
		return reply.VM, fmt.Errorf("agent failed %s: %s", messageType, strings.TrimSpace(reply.VM.Error))
	}
	return reply.VM, nil
}

// CTCommand sends one ct-family command and waits for its result.
func (t *AgentTransport) CTCommand(
	ctx context.Context,
	messageType string,
	payload CTPayload,
	timeout time.Duration,
) (*CTPayload, error) {
	return t.ctCommand(ctx, messageType, payload, timeout, nil)
}

func (t *AgentTransport) ctCommand(
	ctx context.Context,
	messageType string,
	payload CTPayload,
	timeout time.Duration,
	onFrame func(*WSMessage),
) (*CTPayload, error) {
	if ctUsesHostLinks(payload) {
		if err := t.hostLinksFloor(); err != nil {
			return nil, err
		}
	}
	reply, err := t.computeCall(ctx, messageType, func(msg *WSMessage) {
		msg.CT = &payload
	}, timeout, onFrame, nil)
	if err != nil {
		return nil, err
	}
	if reply.CT == nil {
		return nil, fmt.Errorf("agent returned no result for %s", messageType)
	}
	if !reply.CT.Success && reply.CT.Error != "" {
		return reply.CT, fmt.Errorf("agent failed %s: %s", messageType, strings.TrimSpace(reply.CT.Error))
	}
	return reply.CT, nil
}

// ComputeLongTimeout exposes the long-operation budget to callers that build
// their own command payloads.
func ComputeLongTimeout() time.Duration { return computeLongTimeout }

// ComputeImportTimeout exposes the guest-import budget.
func ComputeImportTimeout() time.Duration { return computeImportTimeout }

// ComputeTimeout exposes the ordinary round-trip budget.
func ComputeTimeout() time.Duration { return computeTimeout }
