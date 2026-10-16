// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package updates

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"sort"
	"strings"
	"sync"
	"time"

	coreagent "github.com/therealmcsparrow/mcharbor/core/agent"
	"github.com/therealmcsparrow/mcharbor/internal/closeutil"
	"github.com/therealmcsparrow/mcharbor/modules/agentreleases"
)

const (
	agentUpdateAckTimeout = 10 * time.Minute
	// agentSweepTimeout bounds a whole background sweep. Generous because every
	// agent inside it may take the full ack timeout, finite because a wedged
	// one must not leave the table saying "updating" for the rest of uptime.
	agentSweepTimeout       = 30 * time.Minute
	agentUpdateRetryBackoff = 6 * time.Hour
)

// AgentUpdateResult is the per-agent outcome of an update sweep.
type AgentUpdateResult struct {
	EnvID          string `json:"envId"`
	Hostname       string `json:"hostname,omitempty"`
	CurrentVersion string `json:"currentVersion"`
	TargetVersion  string `json:"targetVersion"`
	Status         string `json:"status"` // "started", "up_to_date", "unsupported", "skipped", "excluded", "failed"
	Mode           string `json:"mode,omitempty"`
	Error          string `json:"error,omitempty"`
}

// AgentInventoryRow is one connected agent, as the Updates table shows it.
//
// Everything here comes from the live connection rather than the environments
// row: a build stamp and an address are properties of the agent that is
// attached right now, and a stored copy would keep describing one that has
// gone.
type AgentInventoryRow struct {
	EnvID       string `json:"envId"`
	Name        string `json:"name"`
	Hostname    string `json:"hostname,omitempty"`
	InstallKind string `json:"installKind,omitempty"`
	RemoteIP    string `json:"remoteIp,omitempty"`
	Version     string `json:"version,omitempty"`
	Build       string `json:"build,omitempty"`
	OS          string `json:"os,omitempty"`
	Arch        string `json:"arch,omitempty"`
	// Outdated is measured against the same target an update sweep would use.
	Outdated bool `json:"outdated"`
	// Updatable is false for an agent too old to accept an update request at
	// all; it has to be replaced by hand, and the table says so rather than
	// offering a checkbox that cannot work.
	Updatable bool `json:"updatable"`
	// TargetVersion and TargetBuild are what *this* agent would move to. The
	// build is per platform: a local binary may exist for one architecture and
	// not another, and an agent is never offered a build that was not made for
	// it.
	TargetVersion string `json:"targetVersion,omitempty"`
	TargetBuild   string `json:"targetBuild,omitempty"`
	// Progress is this agent's place in the sweep running now, absent when
	// none is.
	Progress *AgentRunState `json:"progress,omitempty"`
}

// AgentUpdateSummary is the aggregate returned to the operator.
type AgentUpdateSummary struct {
	TargetVersion string              `json:"targetVersion"`
	Started       int                 `json:"started"`
	UpToDate      int                 `json:"upToDate"`
	Unsupported   int                 `json:"unsupported"`
	Excluded      int                 `json:"excluded"`
	Failed        int                 `json:"failed"`
	Results       []AgentUpdateResult `json:"results"`
}

// AgentUpdater tells outdated remote agents to replace themselves. The target
// is the release the operator selected in the release store, else the agent
// this McHarbor was built with; either way it is something McHarbor serves, so
// no agent is ever sent to GitHub or a registry for its replacement.
type AgentUpdater struct {
	pool     *coreagent.AgentPool
	logger   *slog.Logger
	releases *agentreleases.Store
	bins     AgentBinaries

	mu        sync.Mutex
	attempted map[string]agentAttempt // envID -> last attempt

	// runs is the live state of the sweep happening now, keyed by environment.
	// Kept in memory only: it describes something in flight, and a sweep does
	// not survive a restart for it to describe.
	runMu     sync.Mutex
	runs      map[string]AgentRunState
	runActive bool
}

// AgentRunState is one agent's place in the sweep currently running, which is
// what lets the table show progress per agent rather than one spinner for all.
type AgentRunState struct {
	Status string `json:"status"`
	Mode   string `json:"mode,omitempty"`
	Error  string `json:"error,omitempty"`
	Target string `json:"target,omitempty"`
}

// setRun records one agent's transition.
func (u *AgentUpdater) setRun(envID string, state AgentRunState) {
	u.runMu.Lock()
	defer u.runMu.Unlock()
	if u.runs == nil {
		u.runs = make(map[string]AgentRunState)
	}
	u.runs[envID] = state
}

// RunStates is a copy of the current sweep's per-agent state, and whether a
// sweep is still running.
func (u *AgentUpdater) RunStates() (map[string]AgentRunState, bool) {
	u.runMu.Lock()
	defer u.runMu.Unlock()
	out := make(map[string]AgentRunState, len(u.runs))
	for k, v := range u.runs {
		out[k] = v
	}
	return out, u.runActive
}

// StartUpdate runs a sweep in the background and returns at once.
//
// The RPC to each agent waits for an acknowledgement that can take minutes, and
// holding the operator's request open for all of them is what made the table a
// single spinner. The states are published as the sweep moves through them, so
// the caller polls the inventory instead.
func (u *AgentUpdater) StartUpdate(parent context.Context, force bool, excludedEnvIDs []string) int {
	if u.pool == nil {
		return 0
	}
	excluded := make(map[string]struct{}, len(excludedEnvIDs))
	for _, id := range excludedEnvIDs {
		excluded[strings.TrimSpace(id)] = struct{}{}
	}

	agents := u.pool.List()
	pending := 0
	u.runMu.Lock()
	u.runs = make(map[string]AgentRunState, len(agents))
	for i := range agents {
		if _, skip := excluded[agents[i].EnvID]; skip {
			u.runs[agents[i].EnvID] = AgentRunState{Status: "excluded"}
			continue
		}
		u.runs[agents[i].EnvID] = AgentRunState{Status: "pending"}
		pending++
	}
	u.runActive = true
	u.runMu.Unlock()

	go func() {
		// Detached from the request: the sweep outlives the response by design,
		// and a cancelled caller must not abandon an agent mid-replacement.
		ctx, cancel := context.WithTimeout(context.WithoutCancel(parent), agentSweepTimeout)
		defer cancel()
		u.UpdateOutdated(ctx, force, excludedEnvIDs)
		u.runMu.Lock()
		u.runActive = false
		u.runMu.Unlock()
	}()
	return pending
}

type agentAttempt struct {
	version string
	at      time.Time
}

// AgentBinaries is the agent module's served-binary lookup the updater reads to
// know what an agent would download; main wires it so updates never imports agent.
type AgentBinaries interface {
	AgentBinaryPath(osName, arch string) string
	FileSHA256(path string) (string, error)
	HasLocalAgentBinary(osName, arch string) bool
	LocalAgentBuild() string
}

// NewAgentUpdater creates an updater bound to the live agent pool, the release
// store (nil means only built-in binaries are served) and bins, which must be non-nil.
func NewAgentUpdater(pool *coreagent.AgentPool, logger *slog.Logger, releases *agentreleases.Store, bins AgentBinaries) *AgentUpdater {
	return &AgentUpdater{
		pool:      pool,
		logger:    logger,
		releases:  releases,
		bins:      bins,
		attempted: make(map[string]agentAttempt),
	}
}

// TargetVersion resolves the agent version agents should run: the selected
// release, else the bundled version. It no longer asks GitHub, because a
// version McHarbor has not stored is one it could not deliver.
func (u *AgentUpdater) TargetVersion(ctx context.Context) string {
	version, _ := u.GlobalTarget(ctx)
	return version
}

// GlobalTarget is the target version and build before any per-platform
// narrowing: the selected release's, else the bundled version with the
// locally built stamp.
func (u *AgentUpdater) GlobalTarget(ctx context.Context) (version, build string) {
	if rel, ok := u.targetRelease(ctx); ok {
		return rel.Version, rel.Build
	}
	return coreagent.BundledAgentVersion, u.bins.LocalAgentBuild()
}

func (u *AgentUpdater) targetRelease(ctx context.Context) (agentreleases.Release, bool) {
	if u.releases == nil {
		return agentreleases.Release{}, false
	}
	return u.releases.TargetRelease(ctx)
}

// platformTarget is the version and build an agent on this platform would
// actually receive, following the binary route's precedence: the selected
// release when it carries the platform, else a local build, else the bundled
// version with no build to compare.
func (u *AgentUpdater) platformTarget(ctx context.Context, osName, arch string) (version, build string) {
	if rel, ok := u.targetRelease(ctx); ok {
		for _, a := range rel.Assets {
			if a.OS == osName && a.Arch == arch {
				return rel.Version, rel.Build
			}
		}
	}
	return coreagent.BundledAgentVersion, u.platformTargetBuild(osName, arch)
}

// Inventory lists every connected agent with what the table needs to show.
//
// Names come from the environments table because the pool does not carry one;
// everything else is the live connection. An agent that is connected but has
// no row is skipped rather than shown nameless.
func (u *AgentUpdater) Inventory(ctx context.Context, db *sql.DB) ([]AgentInventoryRow, string, error) {
	target := u.TargetVersion(ctx)
	if u.pool == nil {
		return []AgentInventoryRow{}, target, nil
	}
	states, sweeping := u.RunStates()

	names := map[string]string{}
	kinds := map[string]string{}
	if db != nil {
		rows, err := db.QueryContext(ctx,
			`SELECT id, name, COALESCE(agent_install_kind, '') FROM environments
			  WHERE connection_type = 'agent' LIMIT 1000`)
		if err != nil {
			return nil, target, fmt.Errorf("listing agent environments: %w", err)
		}
		defer func() { closeutil.Report(rows.Close(), "rows") }()
		for rows.Next() {
			var id, name, kind string
			if err := rows.Scan(&id, &name, &kind); err != nil {
				return nil, target, fmt.Errorf("scanning agent environment: %w", err)
			}
			names[id] = name
			kinds[id] = kind
		}
		if err := rows.Err(); err != nil {
			return nil, target, fmt.Errorf("listing agent environments: %w", err)
		}
	}

	agents := u.pool.List()
	out := make([]AgentInventoryRow, 0, len(agents))
	for i := range agents {
		conn := &agents[i]
		name, ok := names[conn.EnvID]
		if !ok {
			continue
		}
		tv, tb := u.platformTarget(ctx, conn.OS, conn.Arch)
		outdated := agentOutdated(conn.Version, conn.Build, tv, tb)
		var progress *AgentRunState
		if state, ok := states[conn.EnvID]; ok && keepProgress(state, sweeping, outdated) {
			copied := state
			progress = &copied
		}
		out = append(out, AgentInventoryRow{
			EnvID:       conn.EnvID,
			Name:        name,
			Hostname:    conn.Hostname,
			InstallKind: kinds[conn.EnvID],
			RemoteIP:    conn.RemoteIP,
			Version:     conn.Version,
			Build:       conn.Build,
			OS:          conn.OS,
			Arch:        conn.Arch,
			Outdated:    outdated,
			Updatable:   coreagent.AgentVersionAtLeast(conn.Version, coreagent.AgentUpdateMinVersion),

			TargetVersion: tv,
			TargetBuild:   tb,
			Progress:      progress,
		})
	}
	sort.Slice(out, func(a, b int) bool { return out[a].Name < out[b].Name })
	return out, target, nil
}

// UpdateOutdated sends an update request to every connected agent whose
// version is older than the target. When force is false an agent that was
// already asked to move to the same version recently is skipped so a
// stale `latest` tag cannot cause a restart loop on every tick. Agents in
// excludedEnvIDs are reported as "excluded" and never contacted.
func (u *AgentUpdater) UpdateOutdated(ctx context.Context, force bool, excludedEnvIDs []string) AgentUpdateSummary {
	target := u.TargetVersion(ctx)
	summary := AgentUpdateSummary{TargetVersion: target, Results: []AgentUpdateResult{}}
	if u.pool == nil {
		return summary
	}
	excluded := make(map[string]struct{}, len(excludedEnvIDs))
	for _, id := range excludedEnvIDs {
		excluded[strings.TrimSpace(id)] = struct{}{}
	}

	agents := u.pool.List()
	results := make([]AgentUpdateResult, len(agents))
	artifacts := map[string]updateArtifacts{}
	var wg sync.WaitGroup
	for i := range agents {
		conn := &agents[i]
		tv, tb := u.platformTarget(ctx, conn.OS, conn.Arch)
		res := AgentUpdateResult{
			EnvID:          conn.EnvID,
			Hostname:       conn.Hostname,
			CurrentVersion: conn.Version,
			TargetVersion:  tv,
		}
		if _, skip := excluded[conn.EnvID]; skip {
			res.Status = "excluded"
			u.setRun(conn.EnvID, AgentRunState{Status: res.Status})
			results[i] = res
			continue
		}
		if !agentOutdated(conn.Version, conn.Build, tv, tb) {
			res.Status = "up_to_date"
			u.setRun(conn.EnvID, AgentRunState{Status: res.Status})
			results[i] = res
			continue
		}
		if !coreagent.AgentVersionAtLeast(conn.Version, coreagent.AgentUpdateMinVersion) {
			res.Status = "unsupported"
			res.Error = fmt.Sprintf("agent %s cannot update itself; requires %s+ (update it manually once)", conn.Version, coreagent.AgentUpdateMinVersion)
			u.setRun(conn.EnvID, AgentRunState{Status: res.Status, Error: res.Error})
			results[i] = res
			continue
		}
		if !force && u.recentlyAttempted(conn.EnvID, tv) {
			res.Status = "skipped"
			u.setRun(conn.EnvID, AgentRunState{Status: res.Status})
			results[i] = res
			continue
		}
		osName, archName, serverURL := conn.OS, conn.Arch, conn.ServerURL
		key := osName + "/" + archName
		art, seen := artifacts[key]
		if !seen {
			art = u.artifactsFor(ctx, osName, archName)
			artifacts[key] = art
		}
		imageLoad := coreagent.SupportsImageLoad(conn.Version, conn.Build)
		payload := art.payload(serverURL, tv, tb, imageLoad)
		if payload.BinaryURL == "" && payload.ImageURL == "" {
			// Nothing to offer is refused here, not sent: an agent that
			// predates this change would read an empty URL as "use GitHub".
			res.Status = "failed"
			res.Error = fmt.Sprintf("McHarbor holds no agent for %s; download a release under Settings > Updates > Agents or build one locally", key)
			u.setRun(conn.EnvID, AgentRunState{Status: res.Status, Error: res.Error})
			results[i] = res
			continue
		}
		transport := u.pool.Transport(conn.EnvID)
		if transport == nil {
			res.Status = "failed"
			res.Error = "agent transport unavailable"
			u.setRun(conn.EnvID, AgentRunState{Status: res.Status, Error: res.Error})
			results[i] = res
			continue
		}
		u.markAttempt(conn.EnvID, tv)
		label := targetLabel(tv, tb)
		u.setRun(conn.EnvID, AgentRunState{Status: "updating", Target: label})
		wg.Add(1)
		go func(i int, res AgentUpdateResult, transport *coreagent.AgentTransport) {
			defer wg.Done()
			reqCtx, cancel := context.WithTimeout(ctx, agentUpdateAckTimeout)
			defer cancel()
			u.logger.Info("agent-update: requesting replacement",
				"env", res.EnvID, "from", res.CurrentVersion, "to", label,
				"binaryUrl", payload.BinaryURL, "imageUrl", payload.ImageURL)
			reply, err := transport.AgentUpdate(reqCtx, payload)
			if err != nil {
				res.Status = "failed"
				res.Error = err.Error()
				if reply != nil && reply.Mode != "" {
					res.Mode = reply.Mode
				}
				u.logger.Warn("agent-update: request failed", "env", res.EnvID, "error", err)
			} else {
				if reply.Mode == "" {
					// Success with no mode is the agent saying it was already
					// current — it downloaded nothing. Recording that as a started
					// replacement is what made a row claim to be replacing itself
					// on every sweep, forever.
					res.Status = "up_to_date"
					u.logger.Info("agent-update: agent reports it is already current",
						"env", res.EnvID, "message", reply.Message)
				} else {
					res.Status = "started"
					res.Mode = reply.Mode
					u.logger.Info("agent-update: replacement started", "env", res.EnvID, "from", res.CurrentVersion, "to", label, "mode", reply.Mode)
				}
			}
			u.setRun(res.EnvID, AgentRunState{
				Status: res.Status, Mode: res.Mode, Error: res.Error,
				Target: label,
			})
			results[i] = res
		}(i, res, transport)
	}
	wg.Wait()

	for _, res := range results {
		switch res.Status {
		case "started":
			summary.Started++
		case "up_to_date":
			summary.UpToDate++
		case "unsupported":
			summary.Unsupported++
		case "excluded":
			summary.Excluded++
		case "failed":
			summary.Failed++
		}
		summary.Results = append(summary.Results, res)
	}
	return summary
}

func (u *AgentUpdater) recentlyAttempted(envID, version string) bool {
	u.mu.Lock()
	defer u.mu.Unlock()
	last, ok := u.attempted[envID]
	return ok && last.version == version && time.Since(last.at) < agentUpdateRetryBackoff
}

func (u *AgentUpdater) markAttempt(envID, version string) {
	u.mu.Lock()
	defer u.mu.Unlock()
	u.attempted[envID] = agentAttempt{version: version, at: time.Now()}
}

// agentOutdated decides whether an agent should be offered an update: the
// target version is newer, or it is the same version with a newer build. A
// build is compared only when both sides have one, so an agent on a platform
// nothing was built for is never called outdated by a stamp it cannot receive.
func agentOutdated(version, build, targetVersion, targetBuild string) bool {
	if !coreagent.AgentVersionAtLeast(version, targetVersion) {
		return true
	}
	if strings.TrimPrefix(strings.TrimSpace(version), "v") != targetVersion {
		return false
	}
	if targetBuild == "" || build == "" {
		return false
	}
	// Both stamps are yyyyMMddHHmm and fixed width, so string order is time
	// order and there is nothing to parse.
	return build < targetBuild
}

// updateArtifacts is what McHarbor can hand one platform in a sweep, resolved
// once per platform rather than once per agent.
type updateArtifacts struct {
	binaryPath string
	binarySHA  string
	imagePath  string
	imageSHA   string
	imageRef   string
	osName     string
	arch       string
}

// artifactsFor resolves the binary (store target, agent-local, agent-bin) and
// the image tarball (store target only) served for a platform.
func (u *AgentUpdater) artifactsFor(ctx context.Context, osName, arch string) updateArtifacts {
	art := updateArtifacts{osName: osName, arch: arch}
	if path := u.bins.AgentBinaryPath(osName, arch); path != "" {
		sum, err := u.bins.FileSHA256(path)
		if err != nil {
			u.logger.Warn("agent-update: hashing the served binary", "path", path, "error", err)
		} else {
			art.binaryPath, art.binarySHA = path, sum
		}
	}
	if u.releases != nil {
		path, sha, ref, _ := u.releases.TargetImage(ctx, osName, arch)
		art.imagePath, art.imageSHA, art.imageRef = path, sha, ref
	}
	return art
}

// payload builds the update request. URLs are absolute, built on the address
// the agent itself dialled, because an agent that predates server-relative
// handling rejects anything without a scheme; serverURL is empty only for a
// connection registered before it was recorded, and the relative form is the
// fallback a current agent resolves itself. The image is offered only to an
// agent that can load it; older ones are sent exactly what they always were.
func (a updateArtifacts) payload(serverURL, version, build string, imageLoad bool) coreagent.AgentUpdatePayload {
	base := strings.TrimRight(strings.TrimSpace(serverURL), "/")
	p := coreagent.AgentUpdatePayload{Version: version, Build: build}
	if a.binaryPath != "" {
		p.BinaryURL = base + "/api/agent/binary/" + a.osName + "/" + a.arch
		p.BinarySHA256 = a.binarySHA
	}
	if imageLoad && a.imagePath != "" {
		p.ImageURL = base + "/api/agent/image/" + a.osName + "/" + a.arch
		p.ImageSHA256 = a.imageSHA
		p.ImageRef = a.imageRef
	}
	return p
}

// platformTargetBuild is the build stamp of the locally built binary for this
// platform, or "" when nothing local was built for it.
func (u *AgentUpdater) platformTargetBuild(osName, arch string) string {
	if !u.bins.HasLocalAgentBinary(osName, arch) {
		return ""
	}
	return u.bins.LocalAgentBuild()
}

// targetLabel names what an agent is moving to, build included when there is
// one. "2026.09.01 (202609191131)" rather than a version that did not change.
func targetLabel(version, build string) string {
	if build != "" {
		return version + " (" + build + ")"
	}
	return version
}

// keepProgress decides whether a finished sweep's verdict is still worth
// showing.
//
// A successful replacement stops being news the moment the agent reconnects on
// the build it was sent: the ordinary badge then tells the truth, and leaving
// "replacing itself" on a row that finished an hour ago is a lie with a
// spinner's authority. A failure stays, because nothing else records it.
func keepProgress(state AgentRunState, sweeping, outdated bool) bool {
	if sweeping {
		return true
	}
	if state.Status == "started" && !outdated {
		return false
	}
	return state.Status == "started" || state.Status == "failed"
}
