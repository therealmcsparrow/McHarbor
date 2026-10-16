// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"
)

// agentRPC is the host's agent-rpc bridge as this plugin sees it. Every type in it is
// builtin, so the plugin declares it here and asserts the host handle against it rather
// than linking a host interface whose change would stop the plugin loading.
type agentRPC interface {
	// Available reports whether envID can take agent calls: "not_agent_environment",
	// "agent_offline" or "agent_too_old" when it cannot.
	Available(ctx context.Context, envID string) (bool, string)
	// Call sends one message declared under agent-rpc send and returns the reply slot as
	// JSON. On an agent refusal of a probe, location, relocation, pool, link or image call
	// the reply slot comes back beside the error.
	Call(ctx context.Context, envID, msgType string, payload []byte, onProgress func(msgType string, frame []byte)) ([]byte, error)
	// Subscribe replaces this plugin's sink for the frames declared under agent-rpc observe.
	Subscribe(sink func(envID, msgType string, frame []byte))
}

// agentRPCCoder is how a bridge error names its class. The host's sentinels are not part of
// the plugin surface, so a failure is classified by code, never by errors.Is or by text.
type agentRPCCoder interface {
	AgentRPCCode() string
}

// The codes the bridge's errors carry.
const (
	rpcNotAgentEnv   = "not_agent_env"
	rpcAgentOffline  = "agent_offline"
	rpcAgentOutdated = "agent_outdated"
	rpcUplinkBusy    = "uplink_busy"
)

// agentPoolInUse is the agent's errorCode for a pool removal refused because guest
// volumes remain on it.
const agentPoolInUse = "pool_in_use"

// agentRPCCode returns err's bridge code, or "" when err did not come from the bridge.
func agentRPCCode(err error) string {
	var coder agentRPCCoder
	if errors.As(err, &coder) {
		return coder.AgentRPCCode()
	}
	return ""
}

// agentBridge is everything this plugin asks of an agent, built on the host's generic
// bridge: it encodes each request into the wire slot, decodes the reply, converts it into
// the plugin's model, and keeps the relocation progress the host used to keep for it.
type agentBridge struct {
	rpc agentRPC

	migrateMu  sync.RWMutex
	migrations map[string]MigrationProgress
}

// newAgentBridge asserts the host's handle. It returns nil when the host wired none, or one
// too old to carry agent-rpc, which every caller treats as "compute is unavailable".
func newAgentBridge(handle any) *agentBridge {
	rpc, ok := handle.(agentRPC)
	if !ok || rpc == nil {
		return nil
	}
	return &agentBridge{rpc: rpc}
}

// call sends one message with an encoded payload and decodes the reply slot into out. The
// reply is decoded even beside an error, because an agent refusal can still carry what the
// operator needs to see.
func (b *agentBridge) call(ctx context.Context, envID, msgType string, payload, out any, onProgress func(string, []byte)) error {
	var body []byte
	if payload != nil {
		encoded, err := json.Marshal(payload)
		if err != nil {
			return fmt.Errorf("encoding %s: %w", msgType, err)
		}
		body = encoded
	}
	reply, err := b.rpc.Call(ctx, envID, msgType, body, onProgress)
	if out != nil && len(reply) > 0 {
		if decodeErr := json.Unmarshal(reply, out); decodeErr != nil && err == nil {
			return fmt.Errorf("decoding the %s reply: %w", msgType, decodeErr)
		}
	}
	return err
}

// Available reports whether envID can host guests, and which precondition is missing.
func (b *agentBridge) Available(ctx context.Context, envID string) (bool, string) {
	return b.rpc.Available(ctx, envID)
}

// Probe asks the agent what its host can run.
func (b *agentBridge) Probe(ctx context.Context, envID string) (Capability, error) {
	var reply agentComputePayload
	if err := b.call(ctx, envID, msgComputeProbe, nil, &reply, nil); err != nil {
		return Capability{}, err
	}
	if reply.Capability == nil {
		return Capability{}, errors.New("agent returned no capability report")
	}
	return convertCapability(reply.Capability), nil
}

// Install asks the agent to install the packages for one capability item. What the agent ran
// and the host's fresh capability come back also when the install failed, so the operator sees
// the package manager's own words.
func (b *agentBridge) Install(ctx context.Context, envID, item string) (InstallResult, error) {
	var reply agentComputePayload
	err := b.call(ctx, envID, msgComputeInstall, agentComputePayload{Install: &agentInstall{Item: item}}, &reply, nil)
	result := InstallResult{Item: item}
	if reply.Install != nil {
		result.Packages, result.Output = reply.Install.Packages, reply.Install.Output
	}
	if reply.Capability != nil {
		capability := convertCapability(reply.Capability)
		result.Capability = &capability
	}
	return result, err
}

// ConfigureRoots tells the agent where to keep its compute state. The locations in force
// come back even when the agent refused, because those are what the operator must see.
func (b *agentBridge) ConfigureRoots(ctx context.Context, envID string, roots RootsConfig) (RootsConfig, error) {
	var reply agentComputePayload
	err := b.call(ctx, envID, msgComputeConfig, agentComputePayload{Roots: wireRoots(roots)}, &reply, nil)
	return domainRoots(reply.Roots), err
}

// MigrateRoots moves the host's compute state, or plans or repairs a move. While an apply
// runs, compute_migrate_progress frames update MigrationProgress for envID.
func (b *agentBridge) MigrateRoots(ctx context.Context, envID string, req MigrationRequest) (MigrationResult, error) {
	apply := !req.Plan && !req.Repair
	if apply {
		b.setMigrationProgress(envID, MigrationProgress{At: time.Now()})
	}
	var reply agentComputePayload
	err := b.call(ctx, envID, msgComputeMigrate, agentComputePayload{Migrate: wireMigrate(req)}, &reply, nil)
	if apply {
		b.clearMigrationProgress(envID)
	}
	if reply.Migrate == nil {
		return MigrationResult{}, err
	}
	return domainMigrationResult(reply.Migrate), err
}

// MigrationProgress is the last progress a running relocation on envID reported.
func (b *agentBridge) MigrationProgress(envID string) (MigrationProgress, bool) {
	b.migrateMu.RLock()
	defer b.migrateMu.RUnlock()
	progress, ok := b.migrations[envID]
	return progress, ok
}

func (b *agentBridge) setMigrationProgress(envID string, progress MigrationProgress) {
	b.migrateMu.Lock()
	if b.migrations == nil {
		b.migrations = map[string]MigrationProgress{}
	}
	b.migrations[envID] = progress
	b.migrateMu.Unlock()
}

func (b *agentBridge) clearMigrationProgress(envID string) {
	b.migrateMu.Lock()
	delete(b.migrations, envID)
	b.migrateMu.Unlock()
}

// recordMigrateProgress takes one compute_migrate_progress frame. The agent puts the
// percentage in Entries and the bytes so far in Bytes.
func (b *agentBridge) recordMigrateProgress(envID string, frame []byte) {
	var payload agentComputePayload
	if json.Unmarshal(frame, &payload) != nil || payload.Migrate == nil {
		return
	}
	b.setMigrationProgress(envID, MigrationProgress{
		Percent: payload.Migrate.Entries,
		Bytes:   payload.Migrate.Bytes,
		At:      time.Now(),
	})
}

// PoolDetect enumerates the storage backends usable on the host.
func (b *agentBridge) PoolDetect(ctx context.Context, envID string) ([]PoolBackend, error) {
	var reply agentPoolPayload
	if err := b.call(ctx, envID, msgPoolDetect, nil, &reply, nil); err != nil {
		return nil, err
	}
	return domainPoolBackends(reply.Detected), nil
}

// PoolCreate initializes a pool and returns its first capacity sample.
func (b *agentBridge) PoolCreate(ctx context.Context, envID string, req PoolRequest) (PoolStats, error) {
	var reply agentPoolPayload
	payload := agentPoolPayload{PoolID: req.PoolID, Name: req.Name, Backend: req.Backend, Target: req.Target}
	if err := b.call(ctx, envID, msgPoolCreate, payload, &reply, nil); err != nil {
		return PoolStats{}, err
	}
	return domainPoolStats(reply.Stats), nil
}

// PoolStats samples a pool's capacity.
func (b *agentBridge) PoolStats(ctx context.Context, envID string, pool PoolRef) (PoolStats, error) {
	var reply agentPoolPayload
	if err := b.call(ctx, envID, msgPoolStats, wirePool(pool), &reply, nil); err != nil {
		return PoolStats{}, err
	}
	return domainPoolStats(reply.Stats), nil
}

// PoolRemove detaches a pool; without Force one that still holds volumes is refused.
func (b *agentBridge) PoolRemove(ctx context.Context, envID string, req PoolRequest) error {
	payload := agentPoolPayload{PoolID: req.PoolID, Name: req.Name, Backend: req.Backend, Target: req.Target, Force: req.Force}
	var reply agentPoolPayload
	err := b.call(ctx, envID, msgPoolRemove, payload, &reply, nil)
	if err != nil && reply.ErrorCode == agentPoolInUse {
		return fmt.Errorf("%w: %s", ErrPoolInUse, strings.TrimSpace(reply.Error))
	}
	return err
}

// PoolBrowse lists one directory in a pool. Confinement is the agent's: it is the only side
// that can resolve a symlink against the real filesystem.
func (b *agentBridge) PoolBrowse(ctx context.Context, envID string, pool PoolRef, dir string) ([]PoolFile, error) {
	payload := wirePool(pool)
	payload.Path = dir
	var reply agentPoolPayload
	if err := b.call(ctx, envID, msgPoolBrowse, payload, &reply, nil); err != nil {
		return nil, err
	}
	return domainPoolFiles(reply.Entries), nil
}

// PoolFile performs one file operation in a pool and returns the bytes a read produced.
// A file travels whole in one message, so only small files take this path.
func (b *agentBridge) PoolFile(ctx context.Context, envID string, pool PoolRef, req PoolFileRequest) ([]byte, error) {
	payload := wirePool(pool)
	payload.Operation, payload.Path, payload.NewName = req.Operation, req.Path, req.NewName
	payload.Mode, payload.Content, payload.Recursive = req.Mode, req.Content, req.Recursive
	var reply agentPoolPayload
	if err := b.call(ctx, envID, msgPoolFile, payload, &reply, nil); err != nil {
		return nil, err
	}
	return reply.Content, nil
}

// NetDetect enumerates the host's links and bridges.
func (b *agentBridge) NetDetect(ctx context.Context, envID string) ([]HostLink, error) {
	var reply agentNetReply
	if err := b.call(ctx, envID, msgNetDetect, nil, &reply, nil); err != nil {
		return nil, err
	}
	return domainLinks(reply.Links), nil
}

// NetEnsure creates or verifies a bridged or managed network.
func (b *agentBridge) NetEnsure(ctx context.Context, envID string, req NetworkRequest) error {
	return b.call(ctx, envID, msgNetEnsure, wireNetwork(req), nil, nil)
}

// NetRemove tears a bridged or managed network down.
func (b *agentBridge) NetRemove(ctx context.Context, envID string, req NetworkRequest) error {
	return b.call(ctx, envID, msgNetRemove, wireNetwork(req), nil, nil)
}

// ImageFetch has the agent download and verify an image; only progress crosses back.
// onProgress may be nil and is never called concurrently or after this returns. Giving
// up on ctx makes the host tell the agent to stop.
func (b *agentBridge) ImageFetch(ctx context.Context, envID string, req ImageRequest, onProgress func(ImageProgress)) (ImageResult, error) {
	var forward func(string, []byte)
	if onProgress != nil {
		forward = func(msgType string, frame []byte) {
			var p agentImagePayload
			if msgType != msgImageProgress || json.Unmarshal(frame, &p) != nil {
				return
			}
			onProgress(domainImageProgress(p))
		}
	}
	var reply agentImagePayload
	if err := b.call(ctx, envID, msgImageFetch, wireImageRequest(req), &reply, forward); err != nil {
		return ImageResult{}, err
	}
	return domainImageResult(reply), nil
}

// ImageList reports what the host has cached.
func (b *agentBridge) ImageList(ctx context.Context, envID string) ([]CachedImage, error) {
	var reply agentImagePayload
	if err := b.call(ctx, envID, msgImageList, nil, &reply, nil); err != nil {
		return nil, err
	}
	return domainCachedImages(reply.Cached), nil
}

// ImageRemove evicts a cached image; one backing a live guest disk is refused.
func (b *agentBridge) ImageRemove(ctx context.Context, envID, imageID string) error {
	return b.call(ctx, envID, msgImageRemove, agentImagePayload{ImageID: imageID}, nil, nil)
}

// send carries one guest command this plugin borrows (vm_query, vm_shutdown) or a direct
// network (net ensure/remove), with a payload it already encoded.
func (b *agentBridge) send(ctx context.Context, envID, msgType string, payload []byte) ([]byte, error) {
	return b.rpc.Call(ctx, envID, msgType, payload, nil)
}

// Subscribe routes the frames this plugin observes: leases to onLease, relocation progress
// into MigrationProgress. The host delivers them from this plugin's own queue, in order.
func (b *agentBridge) Subscribe(onLease func(GuestEvent)) {
	b.rpc.Subscribe(func(envID, msgType string, frame []byte) {
		switch msgType {
		case msgNetLeaseEvent:
			if event, ok := leaseEvent(envID, frame); ok && onLease != nil {
				onLease(event)
			}
		case msgComputeMigrateProgress:
			b.recordMigrateProgress(envID, frame)
		}
	})
}

// Unsubscribe stops frames reaching this plugin once it shuts down.
func (b *agentBridge) Unsubscribe() {
	b.rpc.Subscribe(nil)
}

// leaseEvent turns a net_lease_event frame into the guest event the lease table records.
// The hostname is guest-supplied and carried as display data only.
func leaseEvent(envID string, frame []byte) (GuestEvent, bool) {
	var payload agentNetReply
	if json.Unmarshal(frame, &payload) != nil || payload.Lease == nil {
		return GuestEvent{}, false
	}
	lease := payload.Lease
	return GuestEvent{
		EnvID:     envID,
		GuestKind: lease.GuestKind,
		GuestID:   lease.GuestID,
		Kind:      "lease_" + lease.Action,
		Detail: map[string]string{
			"networkId": lease.NetworkID,
			"mac":       lease.MAC,
			"ip":        lease.IP,
			"hostname":  lease.Hostname,
		},
		At: time.Now(),
	}, true
}

func wireRoots(r RootsConfig) *agentRoots {
	return &agentRoots{VMRoot: r.VMRoot, CTRoot: r.CTRoot, ImageRoot: r.ImageRoot}
}

func domainRoots(r *agentRoots) RootsConfig {
	if r == nil {
		return RootsConfig{}
	}
	return RootsConfig{VMRoot: r.VMRoot, CTRoot: r.CTRoot, ImageRoot: r.ImageRoot}
}

// wireMigrate carries what the host's MigrateRoots carried: roots always present, and a
// volume's size never sent, because the agent measures it.
func wireMigrate(req MigrationRequest) *agentMigrate {
	out := &agentMigrate{
		Plan: req.Plan, Repair: req.Repair, Roots: wireRoots(req.Roots),
		VMFrom: req.VMFrom, CTFrom: req.CTFrom, ImageFrom: req.ImageFrom,
		DiskFrom: req.DiskFrom, DiskTo: req.DiskTo,
	}
	for _, move := range req.Volumes {
		out.Volumes = append(out.Volumes, agentMove{
			Kind: move.Kind, From: move.From, To: move.To, GuestID: move.GuestID, Slot: move.Slot,
		})
	}
	return out
}

func domainMigrationResult(in *agentMigrate) MigrationResult {
	out := MigrationResult{Entries: in.Entries, Bytes: in.Bytes, Notes: in.Notes, Completed: in.Completed}
	for _, move := range in.Moved {
		out.Moved = append(out.Moved, MigrationMove(move))
	}
	for _, failure := range in.Failures {
		out.Failures = append(out.Failures, MigrationFailure(failure))
	}
	return out
}

func wirePool(pool PoolRef) agentPoolPayload {
	return agentPoolPayload{PoolID: pool.ID, Name: pool.Name, Backend: pool.Backend, Target: pool.Target}
}

func domainPoolBackends(in []agentPoolBackend) []PoolBackend {
	out := make([]PoolBackend, 0, len(in))
	for _, backend := range in {
		entry := PoolBackend{
			Backend: backend.Backend, Available: backend.Available, Detail: backend.Detail,
			Remediation: backend.Remediation, Instant: backend.Instant,
		}
		for _, c := range backend.Candidates {
			entry.Candidates = append(entry.Candidates, PoolCandidate(c))
		}
		out = append(out, entry)
	}
	return out
}

func domainPoolStats(in *agentPoolStats) PoolStats {
	if in == nil {
		return PoolStats{}
	}
	return PoolStats{
		CapacityBytes: in.CapacityBytes, UsedBytes: in.UsedBytes, FreeBytes: in.FreeBytes,
		AllocatedBytes: in.AllocatedBytes, VolumeCount: in.VolumeCount, SampledAt: parseRFC3339(in.SampledAt),
	}
}

func domainPoolFiles(in []agentPoolFile) []PoolFile {
	out := make([]PoolFile, 0, len(in))
	for _, e := range in {
		out = append(out, PoolFile(e))
	}
	return out
}

func domainLinks(in []agentHostLink) []HostLink {
	out := make([]HostLink, 0, len(in))
	for _, l := range in {
		out = append(out, HostLink(l))
	}
	return out
}

func convertCapability(in *agentCapability) Capability {
	out := Capability{
		OS: in.OS, Arch: in.Arch, KernelVersion: in.KernelVersion, AgentVersion: in.AgentVersion,
		Supported: in.Supported, UnsupportedWhy: in.UnsupportedWhy,
		CanRunVMs: in.CanRunVMs, CanRunCTs: in.CanRunContainers,
		SeccompBuiltIn: in.SeccompBuiltIn, SupervisorMode: in.SupervisorMode,
		Bridges:  domainLinks(in.Bridges),
		HostCPUs: in.HostCPUs, HostMemoryBytes: in.HostMemoryBytes,
		HostSockets: in.HostSockets, HostPhysicalCores: in.HostPhysicalCores,
		ProbedAt: parseRFC3339(in.ProbedAt),
	}
	for _, item := range in.Items {
		out.Items = append(out.Items, CapabilityItem(item))
	}
	for _, binary := range in.QEMUBinaries {
		out.QEMUBinaries = append(out.QEMUBinaries, QEMUBinary(binary))
	}
	return out
}

// wireNetwork is the bridged or managed network as the agent reads it.
func wireNetwork(req NetworkRequest) agentNetwork {
	return agentNetwork{
		NetworkID: req.NetworkID, Name: req.Name, Mode: req.Mode, Bridge: req.Bridge,
		VLANTag: req.VLANTag, SubnetCIDR: req.SubnetCIDR, GatewayIP: req.GatewayIP,
		DHCPEnabled: req.DHCPEnabled, DHCPRangeStart: req.DHCPRangeStart, DHCPRangeEnd: req.DHCPRangeEnd,
		NATEnabled: req.NATEnabled, UplinkLink: req.UplinkLink,
	}
}

func wireImageRequest(req ImageRequest) agentImagePayload {
	return agentImagePayload{
		ImageID: req.ImageID, Kind: req.Kind, SourceURL: req.SourceURL, ChecksumURL: req.ChecksumURL,
		Checksum: req.Checksum, ChecksumAlgo: req.ChecksumAlgo, SignatureURL: req.SignatureURL,
		AllowUnverified: req.AllowUnverified, FromLibrary: req.FromLibrary, Filename: req.Filename,
		LibraryURL: req.LibraryURL, LibraryToken: req.LibraryToken,
	}
}

func domainImageProgress(p agentImagePayload) ImageProgress {
	return ImageProgress{ImageID: p.ImageID, BytesDone: p.BytesDone, BytesTotal: p.BytesTotal, Phase: p.Phase, Percent: p.Percent}
}

func domainImageResult(r agentImagePayload) ImageResult {
	return ImageResult{
		ImageID: r.ImageID, LocalPath: r.LocalPath, SizeBytes: r.SizeBytes,
		Checksum: r.Checksum, Verified: r.Verified, Signed: r.Signed,
	}
}

func domainCachedImages(in []agentCachedImage) []CachedImage {
	out := make([]CachedImage, 0, len(in))
	for _, image := range in {
		out = append(out, CachedImage{
			ImageID: image.ImageID, Kind: image.Kind, LocalPath: image.LocalPath, SizeBytes: image.SizeBytes,
			Verified: image.Verified, InUse: image.InUse, LastUsedAt: parseRFC3339(image.LastUsedAt),
		})
	}
	return out
}

// parseRFC3339 reads a timestamp the agent sent as text; an empty or malformed one is the
// zero time, as the host's conversions always left it.
func parseRFC3339(s string) time.Time {
	if s == "" {
		return time.Time{}
	}
	parsed, err := time.Parse(time.RFC3339, s)
	if err != nil {
		return time.Time{}
	}
	return parsed
}
