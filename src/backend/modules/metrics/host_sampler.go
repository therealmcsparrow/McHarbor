// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package metrics

import (
	"context"
	"math"
	"time"

	dockerclient "github.com/moby/moby/client"

	"github.com/therealmcsparrow/mcharbor/core/docker"
)

// HostReading is one real reading of a host's CPU and memory, as opposed to the
// sum of its containers' figures.
type HostReading struct {
	CPUPercent float64
	MemUsed    int64
	// MemTotal is zero when the daemon would not report it.
	MemTotal int64
	// IO is the host's own network and disk counters, nil when the host cannot
	// be read that deeply (an agent that predates them, or a local install
	// without /host/proc mounted).
	IO *HostIO
}

// HostIO is a host's cumulative byte counters over its physical NICs and disks,
// which is every byte its containers, guests and the host itself moved.
type HostIO struct {
	NetRx, NetTx, DiskRead, DiskWrite int64
}

// HostReader returns the live host reading for envID, or ok=false when none is
// available (old agent, unreachable host, non-Linux server).
type HostReader func(ctx context.Context, envID string) (HostReading, bool)

// hostReadTimeout bounds one host reading so a wedged agent cannot hold a
// collector slot or a stats-push ack past it.
const hostReadTimeout = 15 * time.Second

// ReadHost reads the same CPU and memory figures GET /metrics/host serves: from
// /proc on the McHarbor process for a local environment, and over the agent
// transport for a remote one. It deliberately skips the disk-usage sweep HostInfo
// pays for, because the collector runs this every tick.
func (s *Service) ReadHost(ctx context.Context, envID string) (HostReading, bool) {
	var r HostReading
	if envID != "" && s.dockerPool.IsAgentEnvContext(ctx, envID) {
		if s.agentPool == nil {
			return HostReading{}, false
		}
		t := s.agentPool.ResolveTransport(envID)
		if t == nil || !t.HostStatsSupported() {
			return HostReading{}, false
		}
		statsCtx, cancel := context.WithTimeout(ctx, hostReadTimeout)
		stats, supported, err := t.HostStats(statsCtx)
		cancel()
		if err != nil || !supported {
			return HostReading{}, false
		}
		r.CPUPercent, r.MemUsed = stats.CPUPercent, stats.MemUsed
		if stats.IO != nil {
			r.IO = &HostIO{
				NetRx: stats.IO.NetRxBytes, NetTx: stats.IO.NetTxBytes,
				DiskRead: stats.IO.DiskReadBytes, DiskWrite: stats.IO.DiskWriteBytes,
			}
		}
	} else {
		cpu, memUsed, _, _, _, ok := readHostStats()
		if !ok {
			return HostReading{}, false
		}
		r.CPUPercent, r.MemUsed = cpu, int64(memUsed)
		r.IO = readLocalHostIO(localHostProc, "/proc", "/sys")
	}
	r.MemTotal = s.hostMemTotal(ctx, envID)
	return r, true
}

// hostMemTotal asks the daemon for the host's memory, as HostInfo does; zero
// means unknown and the caller keeps its own denominator.
func (s *Service) hostMemTotal(ctx context.Context, envID string) int64 {
	cli, err := s.dockerPool.GetContext(ctx, envID)
	if err != nil {
		return 0
	}
	infoCtx, cancel := context.WithTimeout(ctx, docker.CallTimeout)
	defer cancel()
	res, err := cli.Info(infoCtx, dockerclient.InfoOptions{})
	if err != nil {
		return 0
	}
	return res.Info.MemTotal
}

// mergeHost overlays a real host reading onto the container aggregate: CPU and
// memory come from the host, and so do the I/O counters when the host reports
// them — the container sum misses the host's own and its guests' traffic. With
// no reading the aggregate is returned untouched, which is the old behaviour.
func mergeHost(agg hostAggregate, h HostReading, ok bool) hostAggregate {
	if !ok {
		return agg
	}
	agg.cpuPct = int(math.Round(h.CPUPercent))
	agg.memUsed = h.MemUsed
	if h.MemTotal > 0 {
		agg.memLimit = h.MemTotal
	}
	agg.memPct = 0
	if agg.memLimit > 0 {
		agg.memPct = int(float64(agg.memUsed) / float64(agg.memLimit) * 100)
	}
	if h.IO != nil {
		agg.netRx, agg.netTx = h.IO.NetRx, h.IO.NetTx
		agg.blockRead, agg.blockWrite = h.IO.DiskRead, h.IO.DiskWrite
		agg.hasIO = true
	}
	return agg
}

// HostReader returns the collector's live host reader, so the stats-push path
// records the same source the pull path does.
func (c *Collector) HostReader() HostReader {
	return func(ctx context.Context, envID string) (HostReading, bool) {
		return c.capacityService().ReadHost(ctx, envID)
	}
}

// readHost reads the host through the injected reader, or the live one.
func (c *Collector) readHost(ctx context.Context, envID string) (HostReading, bool) {
	if c.hostReader != nil {
		return c.hostReader(ctx, envID)
	}
	return c.HostReader()(ctx, envID)
}
