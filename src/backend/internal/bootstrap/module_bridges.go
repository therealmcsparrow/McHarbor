// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// Package bootstrap holds the cross-module bridges. Each consumer module declares the narrow interface
// it needs; the adapters here satisfy it from the owning module's concrete
// service, and main.go registers the result in the AppDeps service
// registry. This is the only place (besides main.go) that may import two
// feature modules at once.
package bootstrap

import (
	"context"
	"database/sql"
	"errors"
	"time"

	coreagent "github.com/therealmcsparrow/mcharbor/core/agent"
	"github.com/therealmcsparrow/mcharbor/core/docker"
	"github.com/therealmcsparrow/mcharbor/core/encryption"
	"github.com/therealmcsparrow/mcharbor/modules/activity"
	modagent "github.com/therealmcsparrow/mcharbor/modules/agent"
	aiassistant "github.com/therealmcsparrow/mcharbor/modules/ai_assistant"
	aiproviders "github.com/therealmcsparrow/mcharbor/modules/ai_providers"
	"github.com/therealmcsparrow/mcharbor/modules/container_backups"
	"github.com/therealmcsparrow/mcharbor/modules/containers"
	"github.com/therealmcsparrow/mcharbor/modules/metrics"
	modmfa "github.com/therealmcsparrow/mcharbor/modules/mfa"
	"github.com/therealmcsparrow/mcharbor/modules/users"

	"github.com/therealmcsparrow/mcharbor/core/rbac"
	apikeys "github.com/therealmcsparrow/mcharbor/modules/api_keys"
	modauth "github.com/therealmcsparrow/mcharbor/modules/auth"
)

// agentOfflineBridge implements modagent.OfflineEventClassifier and
// modagent.OfflineMetricsIngester from the activity and metrics modules.
type agentOfflineBridge struct {
	db *sql.DB
}

// NewAgentOfflineBridge builds the classifier + ingester the agent module's
// offline replay path delegates to.
func NewAgentOfflineBridge(db *sql.DB) *agentOfflineBridge {
	return &agentOfflineBridge{db: db}
}

func (b *agentOfflineBridge) SubjectTypeForDockerEvent(eventType string) (string, bool) {
	return activity.SubjectTypeForDockerEvent(eventType)
}

func (b *agentOfflineBridge) LifecycleSeverity(eventType, action string) (string, string) {
	return activity.LifecycleSeverity(eventType, action)
}

func (b *agentOfflineBridge) IngestSamples(ctx context.Context, envID string, sampledAt time.Time, samples []coreagent.OfflineMetricEntry) error {
	stats := make([]metrics.ContainerMetric, 0, len(samples))
	for _, m := range samples {
		stats = append(stats, metrics.ContainerMetric{
			ID:         m.ContainerID,
			Name:       m.ContainerName,
			Key:        m.ContainerKey,
			CPUPercent: m.CPUPercent,
			MemUsage:   m.MemUsed,
			MemLimit:   m.MemLimit,
			NetRx:      m.NetRx,
			NetTx:      m.NetTx,
			BlockRead:  m.BlockRead,
			BlockWrite: m.BlockWrite,
			PIDs:       uint64(m.PIDs),
		})
	}
	return metrics.IngestSamples(ctx, b.db, envID, sampledAt, stats)
}

// Compile-time checks against the consumer contracts.
var (
	_ modagent.OfflineEventClassifier = (*agentOfflineBridge)(nil)
	_ modagent.OfflineMetricsIngester = (*agentOfflineBridge)(nil)
)

// containerRecreator adapts containers.Service.Recreate to the
// container_backups.ContainerRecreator contract (the create response is
// not needed by a config restore).
type containerRecreator struct {
	svc *containers.Service
}

// NewContainerRecreator builds the recreate bridge for config restores.
// A config restore recreates a container from its stored Config.Env, which
// can carry ${secret:NAME}, so this bridge needs the encryption service to
// build a resolver.
func NewContainerRecreator(pool *docker.ClientPool, db *sql.DB, dataDir string, enc *encryption.Service) container_backups.ContainerRecreator {
	return containerRecreator{svc: containers.NewService(pool, db, dataDir, enc)}
}

func (r containerRecreator) Recreate(ctx context.Context, envID, containerID string, req docker.RecreateRequest) (string, error) {
	resp, err := r.svc.Recreate(ctx, envID, containerID, req)
	if err != nil {
		return "", err
	}
	return resp.ID, nil
}

// mfaVerifier adapts the mfa module's service to the second-factor contract
// the login flow declares, translating that module's sentinel errors into
// the auth module's own so errors.Is keeps working across the boundary.
type mfaVerifier struct {
	svc *modmfa.Service
}

// NewMFAVerifier builds the second-factor bridge the login flow uses.
func NewMFAVerifier(db *sql.DB, enc *encryption.Service) modauth.MFAVerifier {
	return mfaVerifier{svc: modmfa.NewService(db, enc)}
}

// translateMFAError maps the mfa module's sentinels onto the auth module's.
func translateMFAError(err error) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, modmfa.ErrChallengeInvalid):
		return modauth.ErrMFAChallengeInvalid
	case errors.Is(err, modmfa.ErrInvalidCode):
		return modauth.ErrMFAInvalidCode
	case errors.Is(err, modmfa.ErrNotEnrolled):
		return modauth.ErrMFANotEnrolled
	default:
		return err
	}
}

func (v mfaVerifier) IsEnabled(ctx context.Context, userID string) (bool, error) {
	ok, err := v.svc.IsEnabled(ctx, userID)
	return ok, translateMFAError(err)
}

func (v mfaVerifier) CreateChallenge(ctx context.Context, userID string) (string, error) {
	id, err := v.svc.CreateChallenge(ctx, userID)
	return id, translateMFAError(err)
}

func (v mfaVerifier) PeekChallenge(ctx context.Context, challengeID string) (string, error) {
	id, err := v.svc.PeekChallenge(ctx, challengeID)
	return id, translateMFAError(err)
}

func (v mfaVerifier) VerifyForLogin(ctx context.Context, userID, code string) error {
	return translateMFAError(v.svc.VerifyForLogin(ctx, userID, code))
}

func (v mfaVerifier) ConsumeChallenge(ctx context.Context, challengeID string) (string, error) {
	id, err := v.svc.ConsumeChallenge(ctx, challengeID)
	return id, translateMFAError(err)
}

// NewAPIKeyArchiver builds the tombstone-writer a user delete runs before
// the API-key cascade fires.
func NewAPIKeyArchiver(db *sql.DB, rbacSvc *rbac.Service) users.APIKeyArchiver {
	return apikeys.NewService(db, rbacSvc)
}

// aiProviderResolver narrows ai_providers.Service.Resolve to the
// aiassistant.ProviderResolver contract.
type aiProviderResolver struct {
	svc *aiproviders.Service
}

// NewAIProviderResolver builds the provider-connection resolver the AI
// assistant reads its chat routing from.
func NewAIProviderResolver(db *sql.DB, enc *encryption.Service, pluginAdapters any) aiassistant.ProviderResolver {
	return aiProviderResolver{svc: aiproviders.NewService(db, enc, pluginAdapters)}
}

func (r aiProviderResolver) Resolve(ctx context.Context, connectionID string) (*aiassistant.ProviderConnection, map[string]string, error) {
	row, config, err := r.svc.Resolve(ctx, connectionID)
	if err != nil {
		return nil, nil, err
	}

	return toAIProviderConnection(row), config, nil
}

func (r aiProviderResolver) Default(ctx context.Context) (*aiassistant.ProviderConnection, error) {
	row, err := r.svc.Default(ctx)
	if err != nil {
		return nil, err
	}
	return toAIProviderConnection(row), nil
}

// toAIProviderConnection narrows a provider row to the fields chat routing
// and the kill switch read.
func toAIProviderConnection(row *aiproviders.AIProviderConnection) *aiassistant.ProviderConnection {
	if row == nil {
		return nil
	}
	return &aiassistant.ProviderConnection{
		ID:           row.ID,
		Name:         row.Name,
		ProviderType: row.ProviderType,
		BaseURL:      row.BaseURL,
		DefaultModel: row.DefaultModel,
		Enabled:      row.Enabled,
	}
}
