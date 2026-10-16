// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package fakeagent

import (
	"strconv"
	"sync"
	"time"

	coreagent "github.com/therealmcsparrow/mcharbor/core/agent"
	"github.com/therealmcsparrow/mcharbor/core/demo/estate"
)

// statsPusher sends one batch of container samples per interval and waits for
// the server's ack before the next, which is the contract the real agent keeps.
type statsPusher struct {
	mu       sync.Mutex
	session  string
	seq      uint64
	interval time.Duration
	inFlight bool
	done     chan struct{}
	once     sync.Once
}

func (p *statsPusher) stop() { p.once.Do(func() { close(p.done) }) }

func clampInterval(sec int) time.Duration {
	return time.Duration(min(max(sec, 5), 300)) * time.Second
}

func (s *session) startStats(msg *coreagent.WSMessage) {
	sec := 15
	if msg.StatsPush != nil && msg.StatsPush.IntervalSeconds > 0 {
		sec = msg.StatsPush.IntervalSeconds
	}
	s.mu.Lock()
	if s.stats != nil {
		s.stats.stop()
	}
	p := &statsPusher{session: s.agent.cfg.Host.Key + "-" + strconv.FormatInt(time.Now().UnixNano(), 36), interval: clampInterval(sec), done: make(chan struct{})}
	s.stats = p
	s.mu.Unlock()
	go s.pushLoop(p)
}

func (s *session) ackStats(msg *coreagent.WSMessage) {
	s.mu.Lock()
	p := s.stats
	s.mu.Unlock()
	if p == nil || msg.StatsPush == nil || msg.StatsPush.Session != p.session {
		return
	}
	p.mu.Lock()
	p.inFlight = false
	if msg.StatsPush.IntervalSeconds > 0 {
		p.interval = clampInterval(msg.StatsPush.IntervalSeconds)
	}
	p.mu.Unlock()
	if msg.StatsPush.Stop {
		p.stop()
	}
}

func (s *session) pushLoop(p *statsPusher) {
	host := s.agent.cfg.Host
	for {
		p.mu.Lock()
		interval := p.interval
		p.mu.Unlock()
		select {
		case <-s.ctx.Done():
			return
		case <-p.done:
			return
		case <-time.After(interval):
		}
		p.mu.Lock()
		if p.inFlight {
			p.mu.Unlock()
			continue
		}
		p.inFlight = true
		p.seq++
		seq := p.seq
		p.mu.Unlock()
		now := time.Now()
		stamp := now.UTC().Format(time.RFC3339)
		var samples []coreagent.OfflineMetricEntry
		for _, c := range host.Containers() {
			if c.State != estate.StateRunning {
				continue
			}
			sm := host.SampleContainer(c, now)
			samples = append(samples, coreagent.OfflineMetricEntry{
				ID:            p.session + "-" + strconv.FormatUint(seq, 10) + "-" + c.ID[:12],
				ContainerID:   c.ID,
				ContainerName: c.Name,
				ContainerKey:  c.Name,
				CPUPercent:    sm.CPUPercent,
				MemUsed:       sm.MemUsed,
				MemLimit:      sm.MemLimit,
				NetRx:         sm.NetRx,
				NetTx:         sm.NetTx,
				BlockRead:     sm.BlockRead,
				BlockWrite:    sm.BlockWrite,
				PIDs:          sm.PIDs,
				SampledAt:     stamp,
			})
		}
		err := s.send(&coreagent.WSMessage{Type: coreagent.MsgStatsPush, StatsPush: &coreagent.StatsPushPayload{Session: p.session, Seq: seq, SampledAt: stamp, Samples: samples, IntervalSeconds: int(interval / time.Second)}})
		if err != nil {
			return
		}
		go func(seq uint64) {
			select {
			case <-time.After(min(interval, 15*time.Second)):
			case <-p.done:
				return
			}
			p.mu.Lock()
			if p.seq == seq {
				p.inFlight = false
			}
			p.mu.Unlock()
		}(seq)
	}
}
