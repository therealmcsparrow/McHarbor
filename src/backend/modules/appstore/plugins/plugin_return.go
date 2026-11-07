// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package plugins

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/therealmcsparrow/mcharbor/core/audit"
	"github.com/therealmcsparrow/mcharbor/core/auth"
	"github.com/therealmcsparrow/mcharbor/core/i18n"
	"github.com/therealmcsparrow/mcharbor/core/response"
	appversion "github.com/therealmcsparrow/mcharbor/core/version"
	"github.com/therealmcsparrow/mcharbor/core/versionorder"
)

// hostVersion is the running release; a variable so a test can stand in a
// prerelease host, since appversion.Current is read once per process.
var hostVersion = appversion.Current

// Refusals a return reports before it touches anything.
var (
	// ErrReturnOnPrereleaseHost refuses a plugin return while the host itself is a prerelease.
	ErrReturnOnPrereleaseHost = errors.New("this McHarbor runs a prerelease; return it to stable before its plugins")
	// ErrNoStableRelease means the catalog holds no stable build this host can take.
	ErrNoStableRelease = errors.New("no stable release of the plugin is available")
	// ErrReturnNotPrerelease means the installed version is already stable, so there is nothing to return from.
	ErrReturnNotPrerelease = errors.New("the installed version is not a prerelease")
)

// The outcomes a return dry-run reports.
const (
	OutcomeClear          = "clear"
	OutcomeBlocked        = "blocked"
	OutcomeUnsigned       = "unsigned"
	OutcomeNoStable       = "no-stable"
	OutcomePrereleaseHost = "prerelease-host"
)

// maxReturnBody bounds the return request body.
const maxReturnBody = 4 << 10

// ReturnRequest is the body of POST /appstore/plugins/{slug}/return.
type ReturnRequest struct {
	// AllowUnsigned is the operator's explicit consent to an unsigned stable build; it is never inferred.
	AllowUnsigned bool `json:"allow_unsigned,omitempty"`
}

// ReturnResult is the success envelope of a return.
type ReturnResult struct {
	Slug string `json:"slug"`
	From string `json:"from"`
	To   string `json:"to"`
	// unsignedConsent and overrideKept feed the audit row only, so they never reach the response.
	unsignedConsent bool
	overrideKept    bool
}

// ReturnCheckResult is the read-only answer to "could this plugin return now".
type ReturnCheckResult struct {
	OK               bool   `json:"ok"`
	Outcome          string `json:"outcome"`
	InstalledVersion string `json:"installed_version,omitempty"`
	TargetVersion    string `json:"target_version,omitempty"`
	// BlockingMigration is the first blocker and BlockingMigrations all of them, sorted.
	BlockingMigration  string   `json:"blocking_migration,omitempty"`
	BlockingMigrations []string `json:"blocking_migrations,omitempty"`
	Unsigned           bool     `json:"unsigned,omitempty"`
}

// returnTarget picks the newest stable build this host meets the floor of.
func (h *Handler) returnTarget(ctx context.Context, slug string) (PluginVersion, error) {
	entries, err := h.svc.listCatalogAll(ctx)
	if err != nil {
		return PluginVersion{}, fmt.Errorf("reading plugin catalog: %w", err)
	}
	for _, e := range entries {
		if e.Slug != slug {
			continue
		}
		if v, ok := newestEligible(e.Versions, hostVersion(), false, false); ok && strings.TrimSpace(v.DownloadURL) != "" {
			return v, nil
		}
	}
	return PluginVersion{}, ErrNoStableRelease
}

func sourceOf(v PluginVersion, allowUnsigned bool) Source {
	return Source{
		URL: v.DownloadURL, Checksum: v.Checksum, Signature: v.Signature,
		Build: v.Build, MinMcharbor: v.MinMcharbor, AllowUnsigned: allowUnsigned,
	}
}

// installedPrerelease returns the installed version of slug when a return applies to it.
func (h *Handler) installedPrerelease(ctx context.Context, slug string) (string, error) {
	if versionorder.IsPrerelease(hostVersion()) {
		return "", ErrReturnOnPrereleaseHost
	}
	prev, _, ok, err := h.svc.installedState(ctx, slug)
	if err != nil {
		return "", err
	}
	if !ok {
		return "", ErrPluginNotFound
	}
	if !versionorder.IsPrerelease(prev.Version) {
		return "", ErrReturnNotPrerelease
	}
	return prev.Version, nil
}

// ReturnCheck runs a return up to the ledger check and stops. It preflights with
// AllowUnsigned false, so bytes no trust key verifies are reported and never
// unpacked, and it discards what it staged; no row, ledger entry or install
// directory changes.
func (h *Handler) ReturnCheck(ctx context.Context, slug string) (ReturnCheckResult, error) {
	installed, err := h.installedPrerelease(ctx, slug)
	if errors.Is(err, ErrReturnOnPrereleaseHost) {
		return ReturnCheckResult{Outcome: OutcomePrereleaseHost}, nil
	}
	if err != nil {
		return ReturnCheckResult{}, err
	}
	res := ReturnCheckResult{InstalledVersion: installed}
	target, err := h.returnTarget(ctx, slug)
	if errors.Is(err, ErrNoStableRelease) {
		res.Outcome = OutcomeNoStable
		return res, nil
	}
	if err != nil {
		return ReturnCheckResult{}, err
	}
	res.TargetVersion = target.Version
	up, ok := h.installer.(Upgrader)
	if !ok {
		return ReturnCheckResult{}, fmt.Errorf("%w: this installer cannot check a return", ErrPlatformUnsupported)
	}
	st, err := up.Preflight(ctx, sourceOf(target, false), nil)
	if errors.Is(err, ErrUnsignedNotConfirmed) {
		res.Outcome, res.Unsigned = OutcomeUnsigned, true
		return res, nil
	}
	if err != nil {
		return ReturnCheckResult{}, err
	}
	defer up.DiscardStaged(st)
	if st.Slug != slug {
		return ReturnCheckResult{}, fmt.Errorf("%w: artifact is %q, not %q", ErrManifestInvalid, st.Slug, slug)
	}
	blockers, err := returnBlockers(ctx, up, st, installed)
	if err != nil {
		return ReturnCheckResult{}, err
	}
	if len(blockers) > 0 {
		res.Outcome, res.BlockingMigrations, res.BlockingMigration = OutcomeBlocked, blockers, blockers[0]
		return res, nil
	}
	res.OK, res.Outcome = true, OutcomeClear
	return res, nil
}

// returnPlugin moves slug from its installed prerelease to the newest stable.
// upgrade repeats the ledger check, so a state that changed since the dry-run is refused.
func (h *Handler) returnPlugin(ctx context.Context, slug string, allowUnsigned bool) (ReturnResult, error) {
	installed, err := h.installedPrerelease(ctx, slug)
	if err != nil {
		return ReturnResult{}, err
	}
	target, err := h.returnTarget(ctx, slug)
	if err != nil {
		return ReturnResult{}, err
	}
	inst, err := h.upgrade(ctx, slug, sourceOf(target, allowUnsigned), nil)
	if err != nil {
		return ReturnResult{}, err
	}
	if err := h.svc.SetRepairState(ctx, slug, hostVersion(), ""); err != nil {
		slog.Warn("plugin return: clearing the repair state failed", "slug", slug, "error", err)
	}
	res := ReturnResult{Slug: slug, From: installed, To: inst.Version}
	if allowUnsigned {
		if now, _, ok, err := h.svc.installedState(ctx, slug); err == nil && ok && !now.Signed {
			res.unsignedConsent = true
		}
	}
	// The plugin's own prerelease override is cleared here, in the operation, so the
	// CLI and a direct API call get it too: left on, the next repair could take the
	// prerelease again and undo the return.
	if _, err := h.svc.SetChannelOverride(ctx, slug, nil); err != nil {
		slog.Warn("plugin return: clearing the prerelease override failed", "slug", slug, "error", err)
		res.overrideKept = true
	}
	h.notifyChange(slug)
	return res, nil
}

// handleReturnCheck serves GET /api/appstore/plugins/{slug}/return/check.
func (h *Handler) handleReturnCheck(w http.ResponseWriter, r *http.Request) {
	slug := chi.URLParam(r, "slug")
	if !slugPattern.MatchString(slug) {
		response.BadRequestCode(w, r, i18n.ErrPluginSlugRequired)
		return
	}
	res, err := h.ReturnCheck(r.Context(), slug)
	if err != nil {
		h.returnFail(w, r, slug, err)
		return
	}
	response.OK(w, res)
}

// handleReturn serves POST /api/appstore/plugins/{slug}/return.
func (h *Handler) handleReturn(w http.ResponseWriter, r *http.Request) {
	user := auth.UserFromContext(r.Context())
	if user == nil {
		response.UnauthorizedCode(w, r, i18n.ErrAuthRequired)
		return
	}
	slug := chi.URLParam(r, "slug")
	if !slugPattern.MatchString(slug) {
		response.BadRequestCode(w, r, i18n.ErrPluginSlugRequired)
		return
	}
	var req ReturnRequest
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxReturnBody))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&req); err != nil && !errors.Is(err, io.EOF) {
		response.BadRequestCause(w, r, i18n.ErrInvalidBody, err)
		return
	}
	res, err := h.returnPlugin(r.Context(), slug, req.AllowUnsigned)
	if err != nil {
		h.returnFail(w, r, slug, err)
		return
	}
	if h.audit != nil {
		h.audit.Log(r, audit.Entry{
			Action:      "plugin_return",
			EntityType:  "plugin",
			EntityID:    slug,
			EntityName:  slug,
			Details:     returnDetails(slug, res, user.Username),
			StateBefore: versionJSON(res.From),
			StateAfter:  versionJSON(res.To),
			ChangedKeys: "version",
		})
	}
	response.OK(w, res)
}

// returnDetails is the audit sentence of a return. It says when unsigned code was
// consented to, and when the override could not be cleared, since both matter later.
func returnDetails(slug string, res ReturnResult, username string) string {
	d := fmt.Sprintf("Plugin %s returned from %s to %s by %s", slug, res.From, res.To, username)
	if res.unsignedConsent {
		d += "; unsigned artifact accepted by explicit consent"
	}
	if res.overrideKept {
		d += "; the plugin's prerelease override could not be cleared"
	}
	return d
}

// versionJSON renders one version as the audit state document.
func versionJSON(v string) string {
	b, err := json.Marshal(map[string]string{"version": v})
	if err != nil {
		return "{}"
	}
	return string(b)
}

// returnFail logs the real error and answers with an i18n code; the cause rides along only where the operator can act on it.
func (h *Handler) returnFail(w http.ResponseWriter, r *http.Request, slug string, err error) {
	switch {
	case errors.Is(err, ErrReturnOnPrereleaseHost):
		response.ConflictCode(w, r, i18n.ErrUpdateReturnOnPrereleaseHost)
	case errors.Is(err, ErrPluginNotFound):
		response.NotFoundCode(w, r, i18n.ErrPluginNotFound)
	case errors.Is(err, ErrReturnNotPrerelease):
		response.BadRequestCode(w, r, i18n.ErrUpdateDowngradeRefused)
	case errors.Is(err, ErrNoStableRelease):
		response.NotFoundCode(w, r, i18n.ErrPluginNoStableRelease)
	case errors.Is(err, ErrMigrationBlocksReturn):
		response.ConflictCause(w, r, i18n.ErrUpdateMigrationBlocksStable, err)
	case errors.Is(err, ErrUnsignedNotConfirmed):
		response.BadRequestCode(w, r, i18n.ErrPluginUnsignedUnconfirmed)
	case errors.Is(err, ErrSignatureInvalid):
		response.BadRequestCode(w, r, i18n.ErrPluginSignatureInvalid)
	case errors.Is(err, ErrManifestInvalid):
		response.BadRequestCode(w, r, i18n.ErrPluginManifestInvalid)
	case errors.Is(err, ErrHostBelowMinimum):
		response.BadRequestCause(w, r, i18n.ErrPluginBuiltForMismatch, err)
	default:
		slog.Error("plugin return failed", "slug", slug, "error", err)
		response.InternalErrorCause(w, r, i18n.ErrPluginUpdateFailed, err)
	}
}
