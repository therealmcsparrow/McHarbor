// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package telemetry

import (
	"context"
	"errors"
	"sync"
	"time"
)

// Agent request outcomes. They are the only values agent_requests_total's
// outcome label can take.
const (
	AgentOutcomeOK        = "ok"
	AgentOutcomeTimeout   = "timeout"
	AgentOutcomeCancelled = "cancelled"
	AgentOutcomeClosed    = "closed"
	AgentOutcomeSendError = "send_error"
	AgentOutcomeError     = "error"
)

var (
	seenEnvMu sync.Mutex
	seenEnvs  = map[string]bool{}
)

// PermissionDenied counts one refusal of perm by a route gate.
func PermissionDenied(perm string) {
	permissionDenials.WithLabelValues(labelOrUnknown(perm)).Inc()
}

// AgentConnected counts an agent joining the pool, and a reconnect when the
// environment has connected before during this process's life.
func AgentConnected(envID string) {
	envID = labelOrUnknown(envID)
	agentConnects.WithLabelValues(envID).Inc()
	seenEnvMu.Lock()
	seen := seenEnvs[envID]
	seenEnvs[envID] = true
	seenEnvMu.Unlock()
	if seen {
		agentReconnects.WithLabelValues(envID).Inc()
	}
}

// AgentDisconnected counts an agent leaving the pool.
func AgentDisconnected(envID string) {
	agentDisconnects.WithLabelValues(labelOrUnknown(envID)).Inc()
}

// AgentHandshakeRejected counts a handshake refused at stage, which is one of
// upgrade, auth, protocol or duplicate.
func AgentHandshakeRejected(stage string) {
	agentRejected.WithLabelValues(boundedLabel(stage, "token", "upgrade", "auth", "protocol", "duplicate")).Inc()
}

// AgentRequestStarted marks a proxied request in flight and returns the
// function that records its outcome; call it exactly once.
func AgentRequestStarted(envID string) func(outcome string) {
	envID = labelOrUnknown(envID)
	started := time.Now()
	agentInFlight.WithLabelValues(envID).Inc()
	var once sync.Once
	return func(outcome string) {
		once.Do(func() {
			agentInFlight.WithLabelValues(envID).Dec()
			outcome = boundedLabel(outcome, AgentOutcomeOK, AgentOutcomeTimeout, AgentOutcomeCancelled,
				AgentOutcomeClosed, AgentOutcomeSendError, AgentOutcomeError)
			agentRequests.WithLabelValues(envID, outcome).Inc()
			if outcome == AgentOutcomeOK {
				agentRoundTrip.WithLabelValues(envID).Observe(time.Since(started).Seconds())
				agentLastResponse.WithLabelValues(envID).SetToCurrentTime()
			}
		})
	}
}

// AgentRPC starts timing one agent RPC; op must be a protocol message type or a
// fixed name, never caller data. Call the returned function once with the result.
func AgentRPC(envID, op string) func(err error) {
	envID = labelOrUnknown(envID)
	op = labelOrUnknown(op)
	started := time.Now()
	return func(err error) {
		outcome := AgentOutcomeOK
		switch {
		case err == nil:
		case errors.Is(err, context.DeadlineExceeded):
			outcome = AgentOutcomeTimeout
		case errors.Is(err, context.Canceled):
			outcome = AgentOutcomeCancelled
		default:
			outcome = AgentOutcomeError
		}
		agentRPCs.WithLabelValues(envID, op, outcome).Inc()
		agentRPCDuration.WithLabelValues(envID, op).Observe(time.Since(started).Seconds())
	}
}

// AgentDropped counts n pending requests or messages discarded for reason.
func AgentDropped(envID, reason string, n int) {
	if n <= 0 {
		return
	}
	agentDropped.WithLabelValues(labelOrUnknown(envID),
		boundedLabel(reason, "disconnect", "orphan_response", "waiter_full", "invalid_message", "stream_overflow")).Add(float64(n))
}

// AgentKeepalive records one ping-to-pong round trip.
func AgentKeepalive(envID string, latency time.Duration) {
	agentKeepalive.WithLabelValues(labelOrUnknown(envID)).Observe(latency.Seconds())
}

// AgentOfflineBatch records one replay batch; oldest is the earliest entry
// timestamp in it, or zero when none parsed.
func AgentOfflineBatch(envID, kind string, accepted bool, rows int, remaining int64, oldest time.Time) {
	kind = boundedLabel(kind, "logs", "events", "metrics")
	outcome := "accepted"
	if !accepted {
		outcome = "failed"
	}
	agentOfflineBatches.WithLabelValues(kind, outcome).Inc()
	if !accepted {
		return
	}
	agentOfflineRows.WithLabelValues(kind).Add(float64(rows))
	agentOfflineRemaining.WithLabelValues(labelOrUnknown(envID), kind).Set(float64(remaining))
	if !oldest.IsZero() {
		agentOfflineLag.WithLabelValues(kind).Observe(time.Since(oldest).Seconds())
	}
}

// WorkflowRun records one finished workflow run.
func WorkflowRun(trigger, outcome string, duration time.Duration) {
	outcome = boundedLabel(outcome, "completed", "failed", "cancelled", "running")
	workflowRuns.WithLabelValues(boundedLabel(trigger, "manual", "auto", "link", "webhook", "schedule"), outcome).Inc()
	workflowDuration.WithLabelValues(outcome).Observe(duration.Seconds())
}

// ScheduleFired records a schedule firing, late by the gap between its
// intended minute and now.
func ScheduleFired(scheduler string, intended time.Time) {
	scheduler = boundedLabel(scheduler, "workflow", "container_backup")
	scheduleFires.WithLabelValues(scheduler).Inc()
	if intended.IsZero() {
		return
	}
	late := time.Since(intended)
	if late < 0 {
		late = 0
	}
	scheduleLateness.WithLabelValues(scheduler).Observe(late.Seconds())
}

// BackupJobFinished records a container backup or restore run.
func BackupJobFinished(operation, outcome string, duration time.Duration) {
	backupJobs.WithLabelValues(boundedLabel(operation, "backup", "restore"),
		boundedLabel(outcome, "success", "failure", "cancelled")).Observe(duration.Seconds())
}

// CollectorSweep records one metrics collector sweep.
func CollectorSweep(duration time.Duration) {
	collectorSweep.Observe(duration.Seconds())
}

// AuditChainVerified records the result of an audit hash-chain verification.
func AuditChainVerified(ok bool, checkedRows int) {
	if ok {
		auditVerifyOK.Set(1)
	} else {
		auditVerifyOK.Set(0)
	}
	auditVerifyRows.Set(float64(checkedRows))
	auditVerifyAt.SetToCurrentTime()
}

// PluginLoadFailed counts a failed load of the plugin slug.
func PluginLoadFailed(slug string) {
	pluginLoadFailures.WithLabelValues(labelOrUnknown(slug)).Inc()
}
