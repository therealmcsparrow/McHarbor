// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/therealmcsparrow/mcharbor-agent/internal/errlog"
)

// Bounds on the interval the server may ask for. The server clamps too; these
// keep a malformed subscribe from spinning the pusher or parking it for a day.
const (
	statsPushMinInterval = 5 * time.Second
	statsPushMaxInterval = 300 * time.Second
)

// statsPushAckTimeout caps how long one batch waits for its ack before it is
// handed to the offline buffer; a longer interval does not stretch it.
const statsPushAckTimeout = 15 * time.Second

// statsPusher keeps one Docker stats stream open per running container and
// pushes the latest document of each once per interval. The cost of a push is
// then one message per agent, not one round trip per container.
type statsPusher struct {
	proxy   *Proxy
	logger  *slog.Logger
	offline *OfflineBuffer
	send    func(WSMessage) error
	session string

	acks       chan *StatsPushPayload
	retune     chan time.Duration
	interval   time.Duration
	ackTimeout time.Duration

	mu      sync.Mutex
	latest  map[string]OfflineMetricEntry
	streams map[string]*statsStream
	wg      sync.WaitGroup
	seq     uint64
}

func newStatsPusher(proxy *Proxy, offline *OfflineBuffer, send func(WSMessage) error, interval time.Duration, logger *slog.Logger) *statsPusher {
	return &statsPusher{
		proxy:    proxy,
		logger:   logger,
		offline:  offline,
		send:     send,
		session:  newOfflineID(),
		acks:     make(chan *StatsPushPayload, 4),
		retune:   make(chan time.Duration, 1),
		interval: clampStatsInterval(interval),

		ackTimeout: statsPushAckTimeout,
		latest:     make(map[string]OfflineMetricEntry),
		streams:    make(map[string]*statsStream),
	}
}

// statsStream is one open stats stream. Streams are compared by pointer, so a
// stream that ends late cannot remove the one that replaced it after a restart.
type statsStream struct {
	stop context.CancelFunc
}

// channelClosed reports whether done has been closed, without blocking.
func channelClosed(done chan struct{}) bool {
	select {
	case <-done:
		return true
	default:
		return false
	}
}

func clampStatsInterval(d time.Duration) time.Duration {
	return min(max(d, statsPushMinInterval), statsPushMaxInterval)
}

// Retune changes the interval of a running pusher. A newer value replaces one
// the loop has not picked up yet.
func (p *statsPusher) Retune(d time.Duration) {
	select {
	case <-p.retune:
	default:
	}
	p.retune <- clampStatsInterval(d)
}

// DeliverAck hands a stats_push_ack from the read loop to the waiting batch.
// An ack nobody is waiting for — a late one after its timeout — is dropped.
func (p *statsPusher) DeliverAck(ack *StatsPushPayload) {
	if ack == nil {
		return
	}
	select {
	case p.acks <- ack:
	default:
	}
}

// Run pushes until ctx ends or the server answers with stop, and closes every
// stats stream before returning.
func (p *statsPusher) Run(ctx context.Context) {
	streamCtx, stopStreams := context.WithCancel(ctx)
	defer func() {
		stopStreams()
		p.wg.Wait()
	}()

	p.reconcile(streamCtx)
	timer := time.NewTimer(p.interval)
	defer timer.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case d := <-p.retune:
			p.interval = d
			timer.Reset(d)
			continue
		case <-timer.C:
		}
		p.reconcile(streamCtx)
		if stop := p.pushOnce(ctx); stop {
			p.logger.Info("stats push stopped by server")
			return
		}
		timer.Reset(p.interval)
	}
}

// pushOnce sends the current batch and waits for its ack. It reports true when
// the server asked the agent to stop pushing.
func (p *statsPusher) pushOnce(ctx context.Context) bool {
	samples := p.snapshot()
	if len(samples) == 0 {
		return false
	}
	p.seq++
	sampledAt := time.Now().UTC().Format(time.RFC3339)
	for i := range samples {
		samples[i].SampledAt = sampledAt
	}
	payload := &StatsPushPayload{Session: p.session, Seq: p.seq, SampledAt: sampledAt, Samples: samples}
drain:
	for {
		select {
		case <-p.acks:
		default:
			break drain
		}
	}
	if err := p.send(WSMessage{Type: MsgStatsPush, StatsPush: payload}); err != nil {
		p.spool(ctx, samples, fmt.Errorf("sending stats batch: %w", err))
		return false
	}

	waitCtx, cancel := context.WithTimeout(ctx, min(p.interval, p.ackTimeout))
	defer cancel()
	for {
		select {
		case <-waitCtx.Done():
			p.spool(ctx, samples, fmt.Errorf("no acknowledgement for stats batch %d", payload.Seq))
			return false
		case ack := <-p.acks:
			if ack.Session != p.session || ack.Seq != payload.Seq {
				continue
			}
			if ack.Stop {
				return true
			}
			if !ack.Accepted {
				p.spool(ctx, samples, fmt.Errorf("server rejected stats batch %d: %s", payload.Seq, ack.Error))
			}
			if ack.IntervalSeconds > 0 {
				p.interval = clampStatsInterval(time.Duration(ack.IntervalSeconds) * time.Second)
			}
			return false
		}
	}
}

// spool writes an unacknowledged batch to the offline buffer, which replays it
// through the acknowledged offline path on the next connection. The server
// dedupes that replay, so a batch whose ack was lost is not counted twice.
func (p *statsPusher) spool(ctx context.Context, samples []OfflineMetricEntry, cause error) {
	if p.offline == nil {
		p.logger.Debug("stats batch dropped; offline buffering disabled", "error", cause)
		return
	}
	spoolCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
	defer cancel()
	if err := p.offline.Store().AddMetrics(spoolCtx, samples); err != nil {
		p.logger.Warn("stats batch not buffered", "cause", cause, "error", err)
		return
	}
	p.offline.Store().Trim(spoolCtx)
	p.logger.Debug("stats batch buffered for replay", "cause", cause, "samples", len(samples))
}

// snapshot copies the latest document of every streaming container.
func (p *statsPusher) snapshot() []OfflineMetricEntry {
	p.mu.Lock()
	defer p.mu.Unlock()
	out := make([]OfflineMetricEntry, 0, len(p.latest))
	for _, m := range p.latest {
		out = append(out, m)
	}
	return out
}

// reconcile opens a stream for every running container that has none and
// closes the streams of containers that stopped. One list call per interval is
// the only per-interval Docker request the push path makes.
func (p *statsPusher) reconcile(ctx context.Context) {
	listCtx, cancel := context.WithTimeout(ctx, hostOpTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(listCtx, http.MethodGet, "http://docker/containers/json", nil)
	if err != nil {
		return
	}
	resp, err := p.proxy.httpClient.Do(req)
	if err != nil {
		p.logger.Debug("stats push: listing containers failed", "error", err)
		return
	}
	var running []dockerContainerListItem
	decodeErr := json.NewDecoder(resp.Body).Decode(&running)
	errlog.Log(resp.Body.Close())
	if resp.StatusCode >= 300 || decodeErr != nil {
		p.logger.Debug("stats push: container list unreadable", "status", resp.StatusCode, "error", decodeErr)
		return
	}

	alive := make(map[string]bool, len(running))
	p.mu.Lock()
	defer p.mu.Unlock()
	for _, c := range running {
		alive[c.ID] = true
		if _, ok := p.streams[c.ID]; ok {
			continue
		}
		name := ""
		if len(c.Names) > 0 {
			name = strings.TrimPrefix(c.Names[0], "/")
		}
		sctx, stop := context.WithCancel(ctx)
		handle := &statsStream{stop: stop}
		p.streams[c.ID] = handle
		p.wg.Add(1)
		go p.stream(sctx, handle, c.ID, name, containerKey(c.Labels, name))
	}
	for id, handle := range p.streams {
		if !alive[id] {
			handle.stop()
			delete(p.streams, id)
			delete(p.latest, id)
		}
	}
}

// stream follows one container's stats until the container stops or ctx ends.
// Each daemon document already carries the previous sample, so its CPU
// percentage is computed exactly as the one-shot path computes it.
func (p *statsPusher) stream(ctx context.Context, handle *statsStream, id, name, key string) {
	defer p.wg.Done()
	defer func() {
		handle.stop()
		p.mu.Lock()
		if p.streams[id] == handle {
			delete(p.streams, id)
			delete(p.latest, id)
		}
		p.mu.Unlock()
	}()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		"http://docker/containers/"+url.PathEscape(id)+"/stats?stream=true", nil)
	if err != nil {
		return
	}
	resp, err := p.proxy.httpClient.Do(req)
	if err != nil {
		return
	}
	defer func() { errlog.Log(resp.Body.Close()) }()
	if resp.StatusCode >= 300 {
		return
	}
	dec := json.NewDecoder(resp.Body)
	for {
		var stat dockerStats
		if err := dec.Decode(&stat); err != nil {
			return
		}
		m := buildOfflineMetric(id, name, key, &stat, "")
		p.mu.Lock()
		if p.streams[id] == handle {
			p.latest[id] = m
		}
		p.mu.Unlock()
	}
}
