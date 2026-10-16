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

	"github.com/rs/xid"
)

// Guest health signals, read from what the poller last stored.
const (
	signalVMGuestUnresponsive = "vm_guest_unresponsive"
	signalVMGuestDiskFull     = "vm_guest_disk_full"
	signalVMGuestClockSkew    = "vm_guest_clock_skew"
)

// guestQueryHealth is the vm_guest_query the agent answers with a ping,
// filesystems, load and clock skew.
const guestQueryHealth = "health"

// agentTooOldMarker is the agent's own wording for a guest query it predates.
const agentTooOldMarker = "is not a guest query this agent answers"

const (
	// healthPollInterval is how often each eligible guest is asked; three misses
	// then take about six minutes, long enough to ride out a guest's GC pause.
	healthPollInterval = 2 * time.Minute

	// healthCallTimeout bounds one guest's query, healthSweepTimeout one pass.
	healthCallTimeout  = 20 * time.Second
	healthSweepTimeout = 90 * time.Second

	// healthPerEnvironment caps concurrent queries per host, so a sweep never
	// queues dozens of qemu-ga round trips behind one agent connection.
	healthPerEnvironment = 4

	// healthTooOldRecheck is how long a guest on an agent without the health
	// query is left alone before it is asked again.
	healthTooOldRecheck = time.Hour

	// healthStale is the age after which a stored result no longer raises
	// anything: the poller stopped reaching it, and old data is not the present.
	healthStale = 10 * time.Minute

	// unresponsiveFailures is the streak that makes a guest unresponsive.
	unresponsiveFailures = 3

	// guestBootGrace keeps a freshly started guest quiet while qemu-ga comes up.
	guestBootGrace = 5 * time.Minute

	// diskFirePercent raises and diskClearPercent settles, so a filesystem
	// hovering at the line does not open and close an incident every check.
	diskFirePercent  = 95
	diskClearPercent = 90

	// A large filesystem can be nearly out of space well below 95%, so it also
	// fires under diskFireFree and settles only above diskClearFree.
	diskLargeBytes = 10 << 30
	diskFireFree   = 1 << 30
	diskClearFree  = 2 << 30

	// skewFireMs on skewFireChecks consecutive checks raises, and only a skew
	// back under skewClearMs settles.
	skewFireMs     = 60_000
	skewClearMs    = 30_000
	skewFireChecks = 2

	// maxHealthFilesystems and maxHealthUnsupported bound what a guest can make
	// this plugin store.
	maxHealthFilesystems = 64
	maxHealthUnsupported = 16
)

// guestFilesystem is one guest filesystem as stored; mountpoint and type are
// guest-reported text.
type guestFilesystem struct {
	Mountpoint string `json:"mountpoint"`
	Type       string `json:"type,omitempty"`
	UsedBytes  int64  `json:"usedBytes"`
	TotalBytes int64  `json:"totalBytes"`
}

// guestHealth is the latest health record for one guest, latches included.
type guestHealth struct {
	vmID         string
	envID        string
	runStartedAt time.Time
	checkedAt    time.Time
	reachable    bool
	agentState   string
	failures     int
	failingSince time.Time
	lastOKAt     time.Time
	lastError    string
	filesystems  []guestFilesystem
	load1        *float64
	load5        *float64
	load15       *float64
	clockSkewMs  *int64
	skewOver     int
	skewAlert    bool
	diskAlert    bool
	unsupported  []string
	agentTooOld  bool
}

// healthOutcome is what one probe established.
type healthOutcome int

const (
	healthSkipped healthOutcome = iota
	healthOK
	healthFailed
	healthAbsent
	healthTooOld
)

// healthProbe is one probe's result, before it is folded into the record.
type healthProbe struct {
	outcome healthOutcome
	guest   *agentGuest
	err     string
}

// probeGuestHealth asks one guest's qemu-ga for its health. An unreachable host
// is a skip, never a failure, so an agent outage cannot mark guests unresponsive.
func (s *Service) probeGuestHealth(ctx context.Context, vm VM) healthProbe {
	callCtx, cancel := context.WithTimeout(ctx, healthCallTimeout)
	defer cancel()
	reply, err := s.call(callCtx, vm.EnvironmentID, msgVMGuestQuery, agentPayload{
		VMID: vm.ID, GuestQuery: guestQueryHealth,
	})
	return classifyProbe(ctx, reply, err)
}

// classifyProbe maps an agent reply onto a probe outcome; split out so the
// mapping is testable without an agent.
func classifyProbe(ctx context.Context, reply *agentPayload, err error) healthProbe {
	switch {
	case ctx.Err() != nil, errors.Is(err, ErrHostUnavailable):
		return healthProbe{outcome: healthSkipped}
	case err != nil && strings.Contains(err.Error(), agentTooOldMarker):
		return healthProbe{outcome: healthTooOld}
	case err != nil:
		return healthProbe{outcome: healthFailed, err: boundText(err.Error())}
	case reply == nil || reply.Guest == nil:
		return healthProbe{outcome: healthFailed, err: "the agent returned no guest health"}
	case reply.Guest.AgentState == "absent":
		return healthProbe{outcome: healthAbsent}
	}
	return healthProbe{outcome: healthOK, guest: reply.Guest}
}

// nextGuestHealth folds one probe into the stored record and moves the latches.
// It is pure; the bool is false when nothing should be written.
func nextGuestHealth(prev guestHealth, vm VM, probe healthProbe, now time.Time) (guestHealth, bool) {
	if probe.outcome == healthSkipped {
		return prev, false
	}
	next := prev
	next.vmID, next.envID = vm.ID, vm.EnvironmentID
	started := startedAt(vm)
	if !prev.runStartedAt.Equal(started) {
		next.runStartedAt = started
		next.failures, next.failingSince = 0, time.Time{}
		next.skewOver, next.skewAlert = 0, false
	}
	next.checkedAt = now
	next.agentTooOld = false

	switch probe.outcome {
	case healthTooOld:
		next = guestHealth{vmID: vm.ID, envID: vm.EnvironmentID, runStartedAt: started,
			checkedAt: now, agentTooOld: true, lastOKAt: prev.lastOKAt}
	case healthAbsent:
		next.reachable, next.agentState = false, "absent"
		next.failures, next.failingSince, next.lastError = 0, time.Time{}, ""
	case healthFailed:
		next.reachable = false
		next.failures++
		if next.failures == 1 {
			next.failingSince = now
		}
		next.lastError = probe.err
	case healthOK:
		next.reachable, next.agentState = true, "connected"
		next.failures, next.failingSince, next.lastError = 0, time.Time{}, ""
		next.lastOKAt = now
		applyHealthReply(&next, prev, probe.guest)
	}
	return next, true
}

// applyHealthReply copies a successful reply, bounded, and moves the disk and
// skew latches with their hysteresis.
func applyHealthReply(next *guestHealth, prev guestHealth, guest *agentGuest) {
	next.filesystems = nil
	for _, fs := range guest.Filesystems {
		if len(next.filesystems) == maxHealthFilesystems {
			break
		}
		next.filesystems = append(next.filesystems, guestFilesystem{
			Mountpoint: boundText(fs.Mountpoint), Type: boundText(fs.Type),
			UsedBytes: max(fs.UsedBytes, 0), TotalBytes: max(fs.TotalBytes, 0),
		})
	}
	next.load1, next.load5, next.load15 = guest.Load1, guest.Load5, guest.Load15
	next.clockSkewMs = guest.ClockSkewMs
	next.unsupported = nil
	for _, name := range guest.Unsupported {
		if len(next.unsupported) == maxHealthUnsupported {
			break
		}
		next.unsupported = append(next.unsupported, boundText(name))
	}
	next.diskAlert = diskLatched(prev.diskAlert && prev.runStartedAt.Equal(next.runStartedAt), next.filesystems)

	if next.clockSkewMs == nil {
		next.skewOver, next.skewAlert = 0, false
		return
	}
	skew := absInt64(*next.clockSkewMs)
	if skew > skewFireMs {
		next.skewOver++
	} else {
		next.skewOver = 0
	}
	next.skewAlert = next.skewOver >= skewFireChecks || (next.skewAlert && skew >= skewClearMs)
}

// diskLatched raises when any filesystem crosses the fire line and, once raised,
// holds until every filesystem is back under the clear line.
func diskLatched(wasRaised bool, filesystems []guestFilesystem) bool {
	holding := false
	for _, fs := range filesystems {
		if fs.TotalBytes <= 0 {
			continue
		}
		percent := usedPercent(fs)
		free := fs.TotalBytes - fs.UsedBytes
		large := fs.TotalBytes >= diskLargeBytes
		if percent >= diskFirePercent || (large && free < diskFireFree) {
			return true
		}
		if percent >= diskClearPercent || (large && free < diskClearFree) {
			holding = true
		}
	}
	return wasRaised && holding
}

// usedPercent is a filesystem's usage in whole percent, rounded down.
func usedPercent(fs guestFilesystem) int {
	if fs.TotalBytes <= 0 {
		return 0
	}
	return int(fs.UsedBytes * 100 / fs.TotalBytes)
}

// fullestFilesystem is the one to name in a disk-full detail.
func fullestFilesystem(filesystems []guestFilesystem) (guestFilesystem, bool) {
	var best guestFilesystem
	found := false
	for _, fs := range filesystems {
		if fs.TotalBytes <= 0 {
			continue
		}
		if !found || usedPercent(fs) > usedPercent(best) ||
			(usedPercent(fs) == usedPercent(best) && fs.TotalBytes-fs.UsedBytes < best.TotalBytes-best.UsedBytes) {
			best, found = fs, true
		}
	}
	return best, found
}

func absInt64(value int64) int64 {
	if value < 0 {
		return -value
	}
	return value
}

// classifyGuestHealth raises the qemu-ga signals for a running guest. A result
// from an older run, an older agent or older than healthStale raises nothing.
func classifyGuestHealth(vm VM, health *guestHealth, intents []lifecycleIntent, now time.Time) []classifiedSignal {
	if health == nil || health.agentTooOld || !vm.HostAvailable || vm.State != StateRunning ||
		(vm.RunState != "" && vm.RunState != "running") {
		return nil
	}
	started := startedAt(vm)
	if !health.runStartedAt.Equal(started) || now.Sub(health.checkedAt) > healthStale {
		return nil
	}
	var out []classifiedSignal
	booting := !started.IsZero() && now.Sub(started) < guestBootGrace
	if health.failures >= unresponsiveFailures && !health.lastOKAt.IsZero() && !booting &&
		!coveredByIntent(intents, now, stopIntentActions) {
		out = append(out, classifiedSignal{
			signal: signalVMGuestUnresponsive,
			count:  health.failures,
			at:     health.failingSince,
			detail: fmt.Sprintf("the QEMU guest agent missed %d checks in a row; it last answered at %s",
				health.failures, health.lastOKAt.UTC().Format(time.RFC3339)),
		})
	}
	if health.diskAlert {
		detail := "a guest filesystem is nearly full"
		if fs, ok := fullestFilesystem(health.filesystems); ok {
			detail = fmt.Sprintf("%s is %d%% full, %s free", fs.Mountpoint, usedPercent(fs),
				humanBytes(fs.TotalBytes-fs.UsedBytes))
		}
		out = append(out, classifiedSignal{signal: signalVMGuestDiskFull, detail: detail})
	}
	if health.skewAlert && health.clockSkewMs != nil {
		out = append(out, classifiedSignal{
			signal: signalVMGuestClockSkew,
			detail: fmt.Sprintf("the guest clock is %+d s off the host's", *health.clockSkewMs/1000),
		})
	}
	return out
}

// healthColumns is the select list scanGuestHealth reads.
const healthColumns = `vm_id, environment_id, COALESCE(run_started_at, ''), checked_at, reachable,
	COALESCE(agent_state, ''), consecutive_failures, COALESCE(failing_since, ''),
	COALESCE(last_ok_at, ''), COALESCE(last_error, ''), COALESCE(filesystems, ''),
	load1, load5, load15, clock_skew_ms, skew_over_count, skew_alert, disk_alert,
	COALESCE(unsupported, ''), agent_too_old`

// guestHealthRows reads stored health, for one guest or (vmID "") for all.
func (s *Service) guestHealthRows(ctx context.Context, vmID string) (map[string]guestHealth, error) {
	query := `SELECT ` + healthColumns + ` FROM plugin_addon_kvm_guest_health`
	var args []any
	if vmID != "" {
		query += ` WHERE vm_id = ?`
		args = append(args, vmID)
	}
	query += ` LIMIT ?`
	args = append(args, maxRows)

	rows, err := s.db.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("reading guest health: %w", err)
	}
	defer func() { _ = rows.Close() }()

	out := map[string]guestHealth{}
	for rows.Next() {
		var h guestHealth
		var started, checked, failing, lastOK, filesystems, unsupported string
		var reachable, skewAlert, diskAlert, tooOld int
		var load1, load5, load15 *float64
		var skew *int64
		if err := rows.Scan(&h.vmID, &h.envID, &started, &checked, &reachable, &h.agentState,
			&h.failures, &failing, &lastOK, &h.lastError, &filesystems,
			&load1, &load5, &load15, &skew, &h.skewOver, &skewAlert, &diskAlert,
			&unsupported, &tooOld); err != nil {
			return nil, fmt.Errorf("scanning guest health: %w", err)
		}
		h.runStartedAt, h.checkedAt = parseTime(started), parseTime(checked)
		h.failingSince, h.lastOKAt = parseTime(failing), parseTime(lastOK)
		h.reachable, h.skewAlert, h.diskAlert, h.agentTooOld = reachable == 1, skewAlert == 1, diskAlert == 1, tooOld == 1
		h.load1, h.load5, h.load15, h.clockSkewMs = load1, load5, load15, skew
		if filesystems != "" {
			if err := json.Unmarshal([]byte(filesystems), &h.filesystems); err != nil {
				h.filesystems = nil
			}
		}
		if unsupported != "" {
			if err := json.Unmarshal([]byte(unsupported), &h.unsupported); err != nil {
				h.unsupported = nil
			}
		}
		out[h.vmID] = h
	}
	return out, rows.Err()
}

// saveGuestHealth upserts one guest's record; the row is keyed on vm_id so the
// table holds the latest result only, one row per guest.
func (s *Service) saveGuestHealth(ctx context.Context, h guestHealth) error {
	filesystems, err := json.Marshal(h.filesystems)
	if err != nil {
		return fmt.Errorf("encoding guest filesystems: %w", err)
	}
	unsupported, err := json.Marshal(h.unsupported)
	if err != nil {
		return fmt.Errorf("encoding unsupported commands: %w", err)
	}
	now := time.Now().UTC().Format(time.RFC3339)
	_, err = s.db.Exec(ctx, `
		INSERT INTO plugin_addon_kvm_guest_health
			(id, vm_id, environment_id, run_started_at, checked_at, reachable, agent_state,
			 consecutive_failures, failing_since, last_ok_at, last_error, filesystems,
			 load1, load5, load15, clock_skew_ms, skew_over_count, skew_alert, disk_alert,
			 unsupported, agent_too_old, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(vm_id) DO UPDATE SET
			environment_id = excluded.environment_id, run_started_at = excluded.run_started_at,
			checked_at = excluded.checked_at, reachable = excluded.reachable,
			agent_state = excluded.agent_state, consecutive_failures = excluded.consecutive_failures,
			failing_since = excluded.failing_since, last_ok_at = excluded.last_ok_at,
			last_error = excluded.last_error, filesystems = excluded.filesystems,
			load1 = excluded.load1, load5 = excluded.load5, load15 = excluded.load15,
			clock_skew_ms = excluded.clock_skew_ms, skew_over_count = excluded.skew_over_count,
			skew_alert = excluded.skew_alert, disk_alert = excluded.disk_alert,
			unsupported = excluded.unsupported, agent_too_old = excluded.agent_too_old,
			updated_at = excluded.updated_at`,
		xid.New().String(), h.vmID, h.envID, nullTime(h.runStartedAt),
		h.checkedAt.UTC().Format(time.RFC3339), boolToInt(h.reachable), nullIfEmpty(h.agentState),
		h.failures, nullTime(h.failingSince), nullTime(h.lastOKAt), nullIfEmpty(h.lastError),
		string(filesystems), h.load1, h.load5, h.load15, h.clockSkewMs, h.skewOver,
		boolToInt(h.skewAlert), boolToInt(h.diskAlert), string(unsupported), boolToInt(h.agentTooOld),
		now, now)
	if err != nil {
		return fmt.Errorf("recording guest health: %w", err)
	}
	return nil
}

// nullTime stores a zero time as NULL rather than as year one.
func nullTime(t time.Time) any {
	if t.IsZero() {
		return nil
	}
	return t.UTC().Format(time.RFC3339)
}

// ensureGuestHealthSchema creates the health table on an installation that
// predates migration 0008, since install-time migrations never re-run.
func (s *Service) ensureGuestHealthSchema(ctx context.Context) error {
	statements := []string{
		`CREATE TABLE IF NOT EXISTS plugin_addon_kvm_guest_health (
		    id                   TEXT PRIMARY KEY,
		    vm_id                TEXT NOT NULL UNIQUE,
		    environment_id       TEXT NOT NULL,
		    run_started_at       TEXT,
		    checked_at           TEXT NOT NULL,
		    reachable            INTEGER NOT NULL DEFAULT 0,
		    agent_state          TEXT,
		    consecutive_failures INTEGER NOT NULL DEFAULT 0,
		    failing_since        TEXT,
		    last_ok_at           TEXT,
		    last_error           TEXT,
		    filesystems          TEXT,
		    load1                REAL,
		    load5                REAL,
		    load15               REAL,
		    clock_skew_ms        INTEGER,
		    skew_over_count      INTEGER NOT NULL DEFAULT 0,
		    skew_alert           INTEGER NOT NULL DEFAULT 0,
		    disk_alert           INTEGER NOT NULL DEFAULT 0,
		    unsupported          TEXT,
		    agent_too_old        INTEGER NOT NULL DEFAULT 0,
		    created_at           TEXT NOT NULL,
		    updated_at           TEXT NOT NULL
		)`,
		`CREATE INDEX IF NOT EXISTS idx_plugin_addon_kvm_guest_health_env
		    ON plugin_addon_kvm_guest_health(environment_id)`,
	}
	for _, statement := range statements {
		if _, err := s.db.Exec(ctx, statement); err != nil {
			return fmt.Errorf("preparing the guest health table: %w", err)
		}
	}
	return nil
}

// healthEligible reports whether a guest is worth asking: running on a reachable
// host, with a guest agent that has been seen at least once.
func healthEligible(vm VM, prev guestHealth, known bool, now time.Time) bool {
	if vm.IsTemplate || !vm.HostAvailable || vm.State != StateRunning {
		return false
	}
	if known && prev.agentTooOld && now.Sub(prev.checkedAt) < healthTooOldRecheck {
		return false
	}
	return vm.GuestAgentState == "connected" || (known && !prev.lastOKAt.IsZero())
}

// healthPoller asks every eligible guest's qemu-ga for its health on a fixed
// interval, owned by the plugin so Shutdown can stop it and wait.
type healthPoller struct {
	svc      *Service
	logger   pluginLogger
	interval time.Duration
	sweep    func(ctx context.Context)

	cancel context.CancelFunc
	done   chan struct{}
	once   sync.Once
}

func newHealthPoller(svc *Service, logger pluginLogger) *healthPoller {
	p := &healthPoller{svc: svc, logger: logger, interval: healthPollInterval, done: make(chan struct{})}
	p.sweep = p.sweepAll
	return p
}

// Start runs the loop until Stop is called.
func (p *healthPoller) Start() {
	ctx, cancel := context.WithCancel(context.Background())
	p.cancel = cancel
	go func() {
		defer close(p.done)
		ticker := time.NewTicker(p.interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				sweepCtx, sweepCancel := context.WithTimeout(ctx, healthSweepTimeout)
				p.sweep(sweepCtx)
				sweepCancel()
			}
		}
	}()
}

// Stop cancels an in-flight sweep and waits for the loop to exit; it is safe
// on a poller that never started.
func (p *healthPoller) Stop() {
	p.once.Do(func() {
		if p.cancel == nil {
			return
		}
		p.cancel()
		<-p.done
	})
}

// sweepAll probes every eligible guest, environments in parallel and at most
// healthPerEnvironment guests at a time on each.
func (p *healthPoller) sweepAll(ctx context.Context) {
	guests, err := p.svc.List(ctx, "", false)
	if err != nil {
		p.logger.Warn("kvm: guest health could not list guests", "error", err)
		return
	}
	stored, err := p.svc.guestHealthRows(ctx, "")
	if err != nil {
		p.logger.Warn("kvm: guest health could not read stored results", "error", err)
		return
	}

	now := time.Now().UTC()
	byEnv := map[string][]VM{}
	for _, vm := range guests {
		prev, known := stored[vm.ID]
		if healthEligible(vm, prev, known, now) {
			byEnv[vm.EnvironmentID] = append(byEnv[vm.EnvironmentID], vm)
		}
	}

	var wg sync.WaitGroup
	for _, vms := range byEnv {
		wg.Add(1)
		go func(vms []VM) {
			defer wg.Done()
			slots := make(chan struct{}, healthPerEnvironment)
			var envWG sync.WaitGroup
			for _, vm := range vms {
				select {
				case <-ctx.Done():
					envWG.Wait()
					return
				case slots <- struct{}{}:
				}
				envWG.Add(1)
				go func(vm VM) {
					defer envWG.Done()
					defer func() { <-slots }()
					p.checkOne(ctx, vm, stored[vm.ID])
				}(vm)
			}
			envWG.Wait()
		}(vms)
	}
	wg.Wait()
}

// checkOne probes one guest and stores the folded result.
func (p *healthPoller) checkOne(ctx context.Context, vm VM, prev guestHealth) {
	probe := p.svc.probeGuestHealth(ctx, vm)
	next, write := nextGuestHealth(prev, vm, probe, time.Now().UTC())
	if !write {
		return
	}
	if err := p.svc.saveGuestHealth(ctx, next); err != nil {
		p.logger.Warn("kvm: storing guest health failed", "vm", vm.ID, "error", err)
	}
}
