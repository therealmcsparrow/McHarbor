// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package metrics

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	coreagent "github.com/therealmcsparrow/mcharbor/core/agent"
)

// Reason keys a failed host filesystem read is classified into.
const (
	// HostFSReasonAgentOffline means the environment's agent is not connected.
	HostFSReasonAgentOffline = "agent_offline"
	// HostFSReasonAgentTooOld means the agent predates host stats (before 1.6.0).
	HostFSReasonAgentTooOld = "agent_too_old"
	// HostFSReasonReadFailed means no usable reading came back although the host should have answered.
	HostFSReasonReadFailed = "read_failed"
)

var (
	// ErrHostFSAgentOffline is returned when the environment's agent is not connected.
	ErrHostFSAgentOffline = errors.New("agent is not connected")
	// ErrHostFSAgentTooOld is returned when the agent cannot report host stats.
	ErrHostFSAgentTooOld = errors.New("agent does not support host stats")
	// ErrHostFSReadFailed is returned when no usable filesystem reading was obtained.
	ErrHostFSReadFailed = errors.New("host filesystem read failed")
	// ErrHostFSEnvNotFound is returned for an environment id that is not stored, so a caller-chosen id never becomes a tracker key.
	ErrHostFSEnvNotFound = errors.New("environment not found")
	// ErrHostFSUnsupported is returned for a remote environment without an agent, where McHarbor's own disk would be the wrong host.
	ErrHostFSUnsupported = errors.New("host filesystem is not measurable for this environment type")
)

type hostKind int

const (
	hostKindAgent hostKind = iota
	hostKindLocal
	hostKindOther
)

// hostFSBackend is the seam between HostFS and the pools, so the decision
// "whose disk is this" is testable without a Docker daemon or an agent.
type hostFSBackend interface {
	resolve(ctx context.Context, envID string) (string, error)
	exists(ctx context.Context, envID string) (bool, error)
	kind(ctx context.Context, envID string) (hostKind, error)
	agentStats(ctx context.Context, envID string) (coreagent.HostStatsPayload, error)
	localRoot() (total, used int64, ok bool)
}

// backend returns the injected seam, or the pool-backed one.
func (s *Service) backend() hostFSBackend {
	if s.fsb != nil {
		return s.fsb
	}
	return poolBackend{s: s}
}

type poolBackend struct{ s *Service }

func (p poolBackend) resolve(ctx context.Context, envID string) (string, error) {
	return p.s.dockerPool.ResolveEnvironmentIDContext(ctx, envID)
}

// exists reports whether envID is a stored environment; without a database it
// cannot tell, so it answers yes and the later read fails on its own.
func (p poolBackend) exists(ctx context.Context, envID string) (bool, error) {
	if p.s.db == nil {
		return true, nil
	}
	var one int
	err := p.s.db.QueryRowContext(ctx, "SELECT 1 FROM environments WHERE id = ? LIMIT 1", envID).Scan(&one)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("looking up environment: %w", err)
	}
	return true, nil
}

func (p poolBackend) kind(ctx context.Context, envID string) (hostKind, error) {
	if p.s.dockerPool.IsAgentEnvContext(ctx, envID) {
		return hostKindAgent, nil
	}
	host, err := p.s.dockerPool.DockerHostContext(ctx, envID)
	if err != nil {
		return hostKindOther, fmt.Errorf("resolving docker host: %w", err)
	}
	if host == "" || strings.HasPrefix(host, "unix://") || strings.HasPrefix(host, "npipe://") {
		return hostKindLocal, nil
	}
	return hostKindOther, nil
}

func (p poolBackend) agentStats(ctx context.Context, envID string) (coreagent.HostStatsPayload, error) {
	if p.s.agentPool == nil {
		return coreagent.HostStatsPayload{}, ErrHostFSAgentOffline
	}
	t := p.s.agentPool.ResolveTransport(envID)
	if t == nil {
		return coreagent.HostStatsPayload{}, ErrHostFSAgentOffline
	}
	if !t.HostStatsSupported() {
		return coreagent.HostStatsPayload{}, ErrHostFSAgentTooOld
	}
	callCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	stats, supported, err := t.HostStats(callCtx)
	if err != nil {
		return coreagent.HostStatsPayload{}, fmt.Errorf("%w: %w", ErrHostFSReadFailed, err)
	}
	stats.Supported = supported
	return stats, nil
}

func (poolBackend) localRoot() (int64, int64, bool) { return readRootFSUsage() }

// HostFS reports the root filesystem of the host behind envID without any
// Docker call, so a wedged daemon on a full disk cannot hide the disk. An agent
// environment is read through its agent and never from McHarbor's own disk.
func (s *Service) HostFS(ctx context.Context, envID string) (HostFSUsage, error) {
	usage, err := s.readHostFS(ctx, envID)
	if err == nil {
		s.recordHostFS(ctx, envID, usage)
	} else if !errors.Is(err, ErrHostFSUnsupported) {
		s.recordHostFSFailure(ctx, envID, err)
	}
	return usage, err
}

// readHostFS is HostFS without the tracker, so the status path can record once.
func (s *Service) readHostFS(ctx context.Context, envID string) (HostFSUsage, error) {
	b := s.backend()
	kind, err := b.kind(ctx, envID)
	if err != nil {
		return HostFSUsage{}, fmt.Errorf("%w: %w", ErrHostFSReadFailed, err)
	}
	switch kind {
	case hostKindAgent:
		stats, err := b.agentStats(ctx, envID)
		if err != nil {
			return HostFSUsage{}, err
		}
		return usageFromAgentStats(stats)
	case hostKindLocal:
		total, used, ok := b.localRoot()
		if !ok || total <= 0 {
			return HostFSUsage{}, ErrHostFSReadFailed
		}
		return newHostFSUsage(total, used), nil
	default:
		return HostFSUsage{}, ErrHostFSUnsupported
	}
}

// usageFromAgentStats turns an agent payload into a reading; a payload without
// a filesystem total is an error because "0 of 0" would read as healthy.
func usageFromAgentStats(stats coreagent.HostStatsPayload) (HostFSUsage, error) {
	if !stats.Supported {
		return HostFSUsage{}, ErrHostFSAgentTooOld
	}
	if stats.FSTotal <= 0 {
		return HostFSUsage{}, ErrHostFSReadFailed
	}
	return newHostFSUsage(stats.FSTotal, stats.FSUsed), nil
}

func newHostFSUsage(total, used int64) HostFSUsage {
	return HostFSUsage{Path: "/", Total: total, Used: used, Percent: float64(used) / float64(total) * 100.0}
}

// hostFSReason maps a failed read onto the closed reason enum.
func hostFSReason(err error) string {
	switch {
	case errors.Is(err, ErrHostFSAgentOffline):
		return HostFSReasonAgentOffline
	case errors.Is(err, ErrHostFSAgentTooOld):
		return HostFSReasonAgentTooOld
	default:
		return HostFSReasonReadFailed
	}
}
