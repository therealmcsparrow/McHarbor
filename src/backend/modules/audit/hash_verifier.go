// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package audit

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	coreAudit "github.com/therealmcsparrow/mcharbor/core/audit"
	"github.com/therealmcsparrow/mcharbor/core/inapp"
	"github.com/therealmcsparrow/mcharbor/core/quiesce"
	coreSettings "github.com/therealmcsparrow/mcharbor/core/settings"
	"github.com/therealmcsparrow/mcharbor/core/telemetry"
)

const hashVerifyInterval = time.Hour

// HashVerifier periodically validates the audit hash chain.
type HashVerifier struct {
	service     *Service
	auditLog    *coreAudit.Logger
	logger      *slog.Logger
	coordinator verifierCoordinator
	nodeID      string
	cancel      context.CancelFunc
	lastFailure string
}

// verifierCoordinator is the cluster-coordinator subset needed by the verifier.
type verifierCoordinator interface {
	LeaderOf(ctx context.Context, name string) bool
	NodeID() string
}

// NewHashVerifier creates a scheduled audit hash-chain verifier.
func NewHashVerifier(service *Service, auditLog *coreAudit.Logger, logger *slog.Logger, coord verifierCoordinator, nodeID string) *HashVerifier {
	return &HashVerifier{service: service, auditLog: auditLog, logger: logger, coordinator: coord, nodeID: nodeID}
}

// Start runs the verifier once at startup and then hourly.
func (v *HashVerifier) Start(ctx context.Context) {
	if v == nil || v.service == nil || v.auditLog == nil {
		return
	}
	workerCtx, cancel := context.WithCancel(ctx)
	v.cancel = cancel
	go func() {
		v.run(workerCtx)
		ticker := time.NewTicker(hashVerifyInterval)
		defer ticker.Stop()
		for {
			select {
			case <-workerCtx.Done():
				return
			case <-ticker.C:
				quiesce.Run(func() { v.run(workerCtx) })
			}
		}
	}()
}

// Stop stops the verifier loop.
func (v *HashVerifier) Stop() {
	if v != nil && v.cancel != nil {
		v.cancel()
	}
}

func (v *HashVerifier) run(ctx context.Context) {
	if !coreSettings.ReadAuditSettingBoolContext(ctx, v.service.db, "audit.hash_chain.enabled", true) {
		if v.logger != nil {
			v.logger.Debug("audit hash verifier: disabled by rollout flag")
		}
		return
	}
	if v.coordinator != nil && !v.coordinator.LeaderOf(ctx, "audit-hash-verifier") {
		if v.logger != nil {
			v.logger.Debug("audit hash verifier: not leader this tick; skipping", "node", v.nodeID)
		}
		return
	}

	verifyCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	result, err := v.service.VerifyHashChain(verifyCtx)
	if err != nil {
		telemetry.AuditChainVerified(false, 0)
		v.recordFailure(ctx, "verification error", err.Error())
		return
	}

	severity := "success"
	failureKey := ""
	switch {
	case !result.KeyConfigured:
		severity = "error"
		failureKey = "key-not-configured"
	case !result.Verified:
		severity = "error"
		failureKey = fmt.Sprintf("%s:%s", nullString(result.FirstBrokenID), result.Reason)
	case !result.Complete:
		severity = "warning"
	}
	telemetry.AuditChainVerified(severity == "success", int(result.CheckedRows))

	v.auditLog.Log(nil, coreAudit.Entry{
		Action:     "audit.verified.scheduled",
		Severity:   severity,
		EntityType: "audit_log",
		Details: fmt.Sprintf(
			"verified=%t complete=%t key_configured=%t checked=%d hashed=%d unsealed=%d truncated=%t reason=%s",
			result.Verified, result.Complete, result.KeyConfigured, result.CheckedRows, result.HashedRows, result.UnsealedRows, result.Truncated, result.Reason,
		),
	})

	if failureKey == "" {
		v.lastFailure = ""
		return
	}
	if failureKey == v.lastFailure {
		return
	}
	v.lastFailure = failureKey
	message := "Audit hash-chain verification failed"
	if !result.KeyConfigured {
		message = "Audit hash-chain HMAC key is not configured; set MCHARBOR_SECRET"
	} else if strings.TrimSpace(result.Reason) != "" {
		message = fmt.Sprintf("Audit hash-chain verification failed at row %s: %s", nullString(result.FirstBrokenID), result.Reason)
	}
	if err := inapp.CreateBroadcastContext(ctx, v.service.db, inapp.Record{
		Level:      "error",
		Title:      "Audit verification failed",
		Message:    message,
		Action:     "audit.verified.scheduled",
		EntityType: "audit_log",
		EntityID:   nullString(result.FirstBrokenID),
	}); err != nil && v.logger != nil {
		v.logger.Error("audit hash verifier: in-app notification failed", "error", err)
	}
}

func (v *HashVerifier) recordFailure(ctx context.Context, reason, detail string) {
	if v.logger != nil {
		v.logger.Error("audit hash verification failed", "reason", reason, "detail", detail)
	}
	if reason == v.lastFailure {
		return
	}
	v.lastFailure = reason
	v.auditLog.Log(nil, coreAudit.Entry{
		Action:     "audit.verified.scheduled",
		Severity:   "error",
		EntityType: "audit_log",
		Details:    fmt.Sprintf("verified=false complete=false reason=%s", reason),
	})
	if err := inapp.CreateBroadcastContext(ctx, v.service.db, inapp.Record{
		Level:      "error",
		Title:      "Audit verification failed",
		Message:    fmt.Sprintf("Audit hash-chain verification failed: %s", reason),
		Action:     "audit.verified.scheduled",
		EntityType: "audit_log",
	}); err != nil && v.logger != nil {
		v.logger.Error("audit hash verifier: in-app notification failed", "error", err)
	}
}

func nullString(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}
