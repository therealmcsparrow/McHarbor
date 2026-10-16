// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package gitops

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	"github.com/therealmcsparrow/mcharbor/core/encryption"
)

// ErrWebhookSecretMissing reports that a repository has no webhook secret, which
// is the one condition under which an inbound delivery is refused outright.
var ErrWebhookSecretMissing = errors.New("gitops: repository has no webhook secret configured")

// ErrWebhookSignatureInvalid reports a delivery whose HMAC does not match the
// stored secret. It is deliberately indistinguishable from a missing signature
// to a caller, so a prober learns nothing from the difference.
var ErrWebhookSignatureInvalid = errors.New("gitops: webhook signature verification failed")

// ErrWebhookEncryptionUnavailable refuses to mint or read a webhook secret without the
// encryption service, rather than storing the signing key in plaintext.
var ErrWebhookEncryptionUnavailable = errors.New("gitops: webhook secret cannot be stored without encryption")

// GenerateWebhookSecret mints a fresh secret for repoID, stores it encrypted and
// returns the plaintext. The plaintext is returned here and nowhere else — this
// is the only call that can ever produce it, so a lost secret is rotated, not
// recovered.
func (s *Service) GenerateWebhookSecret(ctx context.Context, repoID string) (string, error) {
	if repoID == "" {
		return "", errors.New("gitops: repository id is required")
	}
	exists, err := s.repoExists(ctx, repoID)
	if err != nil {
		return "", err
	}
	if !exists {
		return "", fmt.Errorf("gitops: repository %s not found", repoID)
	}

	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", fmt.Errorf("gitops: generating webhook secret: %w", err)
	}
	plain := "whsec_" + hex.EncodeToString(raw)

	if s.enc == nil {
		return "", ErrWebhookEncryptionUnavailable
	}
	stored, encErr := s.enc.Encrypt(plain)
	if encErr != nil {
		return "", fmt.Errorf("gitops: encrypting webhook secret: %w", encErr)
	}

	if _, err := s.db.ExecContext(ctx,
		`UPDATE git_repos SET webhook_secret = ?, updated_at = ? WHERE id = ?`,
		stored, time.Now().UTC().Format(time.RFC3339), repoID,
	); err != nil {
		return "", fmt.Errorf("gitops: storing webhook secret: %w", err)
	}
	return plain, nil
}

// ClearWebhookSecret removes a repository's secret, which refuses every
// subsequent delivery for it rather than falling back to accepting unsigned ones.
func (s *Service) ClearWebhookSecret(ctx context.Context, repoID string) error {
	if _, err := s.db.ExecContext(ctx,
		`UPDATE git_repos SET webhook_secret = '', updated_at = ? WHERE id = ?`,
		time.Now().UTC().Format(time.RFC3339), repoID,
	); err != nil {
		return fmt.Errorf("gitops: clearing webhook secret: %w", err)
	}
	return nil
}

// WebhookSecretConfigured reports whether repoID can accept a delivery. It is
// what the API exposes, because the secret itself must never leave the server.
func (s *Service) WebhookSecretConfigured(ctx context.Context, repoID string) (bool, error) {
	secret, err := s.webhookSecret(ctx, repoID)
	if errors.Is(err, ErrWebhookSecretMissing) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return secret != "", nil
}

// webhookSecret decrypts a repository's stored secret, returning
// ErrWebhookSecretMissing when there is none so callers fail closed by default
// rather than by remembering to check for an empty string.
func (s *Service) webhookSecret(ctx context.Context, repoID string) (string, error) {
	var stored sql.NullString
	err := s.db.QueryRowContext(ctx,
		`SELECT webhook_secret FROM git_repos WHERE id = ?`, repoID,
	).Scan(&stored)
	if errors.Is(err, sql.ErrNoRows) {
		return "", fmt.Errorf("gitops: repository %s not found", repoID)
	}
	if err != nil {
		return "", fmt.Errorf("gitops: reading webhook secret: %w", err)
	}
	if stored.String == "" {
		return "", ErrWebhookSecretMissing
	}
	if !encryption.IsEncrypted(stored.String) {
		return stored.String, nil
	}
	if s.enc == nil {
		return "", ErrWebhookEncryptionUnavailable
	}
	plain, decErr := s.enc.Decrypt(stored.String)
	if decErr != nil {
		return "", fmt.Errorf("gitops: decrypting webhook secret for %s: %w", repoID, decErr)
	}
	return plain, nil
}
