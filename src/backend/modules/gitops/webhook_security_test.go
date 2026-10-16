// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package gitops

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/therealmcsparrow/mcharbor/core/auth"
	"github.com/therealmcsparrow/mcharbor/core/encryption"
	"github.com/therealmcsparrow/mcharbor/core/rbac"
	"github.com/therealmcsparrow/mcharbor/core/router"
)

// rawBody is deliberately not what json.Marshal would produce for the same
// value: the keys are out of struct order and there is whitespace between them.
// A verifier that re-marshals a decoded struct cannot match a signature over
// these bytes, which is exactly the defect these tests exist to keep out.
const rawBody = `{ "action": "opened",
  "prTitle": "Add the thing",
  "prNumber": "42",
  "sourceBranch": "feature/Thing",
  "targetBranch": "main",
  "commitSha": "deadbeef",
  "repoId": "repo-1" }`

// encryptedAppDeps is testAppDeps with a real encryption service, which minting or
// reading a webhook secret now requires.
func encryptedAppDeps(t *testing.T, database *sql.DB) *router.AppDeps {
	t.Helper()
	enc, err := encryption.New(t.TempDir(), "")
	if err != nil {
		t.Fatalf("encryption: %v", err)
	}
	deps := testAppDeps(database)
	deps.Encryption = enc
	return deps
}

// TestWebhookSecretRefusedWithoutEncryption keeps the signing secret from ever being
// stored in plaintext when the service was built without encryption.
func TestWebhookSecretRefusedWithoutEncryption(t *testing.T) {
	database := newTestDB(t)
	seedPipeline(t, database, "repo-1", "pipe-1", 5, 72)
	svc := NewService(database, nil)
	if _, err := svc.GenerateWebhookSecret(t.Context(), "repo-1"); !errors.Is(err, ErrWebhookEncryptionUnavailable) {
		t.Fatalf("generate without encryption = %v, want ErrWebhookEncryptionUnavailable", err)
	}
	var stored string
	if err := database.QueryRow(`SELECT COALESCE(webhook_secret, '') FROM git_repos WHERE id = 'repo-1'`).Scan(&stored); err != nil {
		t.Fatal(err)
	}
	if stored != "" {
		t.Fatalf("a secret was stored without encryption: %q", stored)
	}
}

func signBody(secret, body string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(body))
	return "sha256=" + hex.EncodeToString(mac.Sum(nil))
}

// webhookRouter mounts the public webhook route the way Mount does, with the
// preview path wired to a fake provisioner so a successful delivery can be
// observed end to end.
func webhookRouter(t *testing.T) (*chi.Mux, *fakeProvisioner, *Service) {
	t.Helper()
	database := newTestDB(t)
	seedPipeline(t, database, "repo-1", "pipe-1", 5, 72)

	h := NewHandler(encryptedAppDeps(t, database))
	prov := &fakeProvisioner{db: database}
	h.service.SetPermissionChecker(allowAll{})
	h.service.SetEnvironmentResolver(func() (EnvironmentProvisioner, bool) { return prov, true })

	r := chi.NewRouter()
	r.Post("/webhooks/gitops/pr", h.HandlePullRequestWebhook)
	return r, prov, h.service
}

func post(t *testing.T, r *chi.Mux, body string, headers map[string]string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/webhooks/gitops/pr?repo=repo-1", strings.NewReader(body))
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	return rec
}

// TestWebhookWithNoSignatureIsRefused is the finding this whole step exists for:
// before the fix the handler passed the signature header in the secret slot and
// an empty secret, so the check never ran and any anonymous caller could drive
// the lifecycle.
func TestWebhookWithNoSignatureIsRefused(t *testing.T) {
	r, prov, svc := webhookRouter(t)
	setWebhookSecret(t, svc.db, "repo-1", "s3cret")

	rec := post(t, r, rawBody, nil)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("unsigned delivery: HTTP %d %s, want 401", rec.Code, rec.Body.String())
	}
	if n := countRows(t, svc.db, `SELECT COUNT(*) FROM gitops_pr_previews`); n != 0 {
		t.Fatalf("an unsigned delivery created %d preview rows, want 0", n)
	}
	if creates, _ := prov.counts(); creates != 0 {
		t.Fatalf("an unsigned delivery reached the provisioner %d times, want 0", creates)
	}
}

// TestWebhookWithAWrongSignatureIsRefused covers a caller who knows the shape of
// the header but not the secret.
func TestWebhookWithAWrongSignatureIsRefused(t *testing.T) {
	r, prov, svc := webhookRouter(t)
	setWebhookSecret(t, svc.db, "repo-1", "s3cret")

	rec := post(t, r, rawBody, map[string]string{
		"X-Hub-Signature-256": signBody("not-the-secret", rawBody),
	})
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("wrongly signed delivery: HTTP %d, want 401", rec.Code)
	}
	if n := countRows(t, svc.db, `SELECT COUNT(*) FROM gitops_pr_previews`); n != 0 {
		t.Fatalf("a wrongly signed delivery created %d preview rows, want 0", n)
	}
	if creates, _ := prov.counts(); creates != 0 {
		t.Fatalf("a wrongly signed delivery reached the provisioner")
	}
}

// TestWebhookSignedOverTheRawBodyIsAccepted proves the HMAC is taken over the
// bytes the forge sent. The body is formatted so that a re-marshalled struct
// would produce a different digest.
func TestWebhookSignedOverTheRawBodyIsAccepted(t *testing.T) {
	r, prov, svc := webhookRouter(t)
	setWebhookSecret(t, svc.db, "repo-1", "s3cret")

	rec := post(t, r, rawBody, map[string]string{
		"X-Hub-Signature-256": signBody("s3cret", rawBody),
		"X-GitHub-Event":      "pull_request",
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("correctly signed delivery: HTTP %d %s, want 200", rec.Code, rec.Body.String())
	}
	if n := countRows(t, svc.db, `SELECT COUNT(*) FROM gitops_pr_previews WHERE pr_number = '42'`); n != 1 {
		t.Fatalf("a correctly signed delivery created %d previews for PR 42, want 1", n)
	}
	if creates, _ := prov.counts(); creates != 1 {
		t.Fatalf("the provisioner was called %d times, want 1", creates)
	}
}

// TestWebhookBodyThatDiffersFromWhatWasSignedIsRefused is the tamper case: a
// valid signature for one body must not admit another.
func TestWebhookBodyThatDiffersFromWhatWasSignedIsRefused(t *testing.T) {
	r, _, svc := webhookRouter(t)
	setWebhookSecret(t, svc.db, "repo-1", "s3cret")

	signature := signBody("s3cret", rawBody)
	tampered := strings.Replace(rawBody, `"42"`, `"43"`, 1)
	rec := post(t, r, tampered, map[string]string{"X-Hub-Signature-256": signature})
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("tampered body: HTTP %d, want 401", rec.Code)
	}
	if n := countRows(t, svc.db, `SELECT COUNT(*) FROM gitops_pr_previews`); n != 0 {
		t.Fatalf("a tampered body created %d preview rows, want 0", n)
	}
}

// TestWebhookForARepositoryWithNoSecretIsRefused asserts Option A's chosen
// behaviour rather than assuming it: no secret means no delivery, and an install
// that has not configured one is closed rather than open.
func TestWebhookForARepositoryWithNoSecretIsRefused(t *testing.T) {
	r, prov, svc := webhookRouter(t)

	rec := post(t, r, rawBody, map[string]string{
		"X-Hub-Signature-256": signBody("anything", rawBody),
	})
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("unconfigured repository: HTTP %d, want 401", rec.Code)
	}
	if n := countRows(t, svc.db, `SELECT COUNT(*) FROM gitops_pr_previews`); n != 0 {
		t.Fatalf("an unconfigured repository created %d preview rows, want 0", n)
	}
	if creates, _ := prov.counts(); creates != 0 {
		t.Fatalf("an unconfigured repository reached the provisioner")
	}
}

// TestGitLabTokenDeliveryIsVerified covers the forge that sends the secret itself
// rather than an HMAC.
func TestGitLabTokenDeliveryIsVerified(t *testing.T) {
	r, _, svc := webhookRouter(t)
	setWebhookSecret(t, svc.db, "repo-1", "s3cret")

	if rec := post(t, r, rawBody, map[string]string{"X-Gitlab-Token": "wrong"}); rec.Code != http.StatusUnauthorized {
		t.Fatalf("wrong GitLab token: HTTP %d, want 401", rec.Code)
	}
	if rec := post(t, r, rawBody, map[string]string{"X-Gitlab-Token": "s3cret"}); rec.Code != http.StatusOK {
		t.Fatalf("correct GitLab token: HTTP %d, want 200", rec.Code)
	}
}

// TestGeneratedWebhookSecretIsShownOnceAndNeverAgain is the agent-token pattern:
// the plaintext exists in exactly one response, and the status endpoint can only
// say whether one is configured.
func TestGeneratedWebhookSecretIsShownOnceAndNeverAgain(t *testing.T) {
	database := newTestDB(t)
	seedPipeline(t, database, "repo-1", "pipe-1", 5, 72)
	h := NewHandler(encryptedAppDeps(t, database))

	r := chi.NewRouter()
	r.Post("/gitops/repos/{repoId}/webhook-secret", h.HandleGenerateWebhookSecret)
	r.Get("/gitops/repos/{repoId}/webhook-secret", h.HandleWebhookSecretStatus)

	authed := func(method, path string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(method, path, nil)
		req = req.WithContext(context.WithValue(req.Context(), auth.UserContextKey, &auth.User{ID: "u1", Username: "admin"}))
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)
		return rec
	}

	created := authed(http.MethodPost, "/gitops/repos/repo-1/webhook-secret")
	if created.Code != http.StatusCreated {
		t.Fatalf("generate: HTTP %d %s", created.Code, created.Body.String())
	}
	if !strings.Contains(created.Body.String(), "whsec_") {
		t.Fatalf("generate did not return the plaintext once: %s", created.Body.String())
	}

	status := authed(http.MethodGet, "/gitops/repos/repo-1/webhook-secret")
	if status.Code != http.StatusOK {
		t.Fatalf("status: HTTP %d", status.Code)
	}
	if strings.Contains(status.Body.String(), "whsec_") {
		t.Fatalf("the status endpoint leaked the secret: %s", status.Body.String())
	}
	if !strings.Contains(status.Body.String(), `"configured":true`) {
		t.Fatalf("status did not report the secret as configured: %s", status.Body.String())
	}
}

// TestValidateStagesRefusesAnEnvironmentThatDoesNotExist closes the hole where a
// stage could name any environment id in the install — including one that was
// never there — because the target was only checked for being non-empty.
func TestValidateStagesRefusesAnEnvironmentThatDoesNotExist(t *testing.T) {
	database := newTestDB(t)
	svc, _ := newPreviewService(t, database, allowAll{})

	err := svc.validateStages(context.Background(), "u1", []StageInput{
		{StageName: "prod", StageIndex: 0, TargetEnvironmentID: "env-that-never-existed", DeployMode: DeployAuto},
	})
	if err == nil || !strings.Contains(err.Error(), "does not exist") {
		t.Fatalf("validateStages accepted a non-existent environment: %v", err)
	}
}

// TestValidateStagesRefusesAnEnvironmentTheCallerCannotSee is the escalation the
// old code allowed: gitops.manage alone could point a stage at an environment the
// caller holds no environments.view for.
func TestValidateStagesRefusesAnEnvironmentTheCallerCannotSee(t *testing.T) {
	database := newTestDB(t)
	now := time.Now().UTC().Format(time.RFC3339)
	if _, err := database.Exec(
		`INSERT INTO environments (id, name, connection_type, is_default, is_active, created_at, updated_at)
		 VALUES ('env-prod', 'Production', 'socket', 0, 1, ?, ?)`, now, now); err != nil {
		t.Fatal(err)
	}
	svc, _ := newPreviewService(t, database, denyEverything{})

	err := svc.validateStages(context.Background(), "u1", []StageInput{
		{StageName: "prod", StageIndex: 0, TargetEnvironmentID: "env-prod", DeployMode: DeployAuto},
	})
	if err == nil || !strings.Contains(err.Error(), "environments.view") {
		t.Fatalf("validateStages accepted an environment the caller cannot see: %v", err)
	}
}

// TestValidateStagesStillAllowsAPreviewStageWithNoTarget keeps the one legitimate
// waiver: a pr_preview stage's environment is created per pull request.
func TestValidateStagesStillAllowsAPreviewStageWithNoTarget(t *testing.T) {
	database := newTestDB(t)
	svc, _ := newPreviewService(t, database, allowAll{})
	if err := svc.validateStages(context.Background(), "u1", []StageInput{
		{StageName: "preview", StageIndex: 0, DeployMode: DeployPRPreview},
	}); err != nil {
		t.Fatalf("a preview stage with no target was refused: %v", err)
	}
}

// denyEverything refuses every permission.
type denyEverything struct{}

func (denyEverything) HasPermission(context.Context, string, string, rbac.Permission) (bool, error) {
	return false, nil
}
