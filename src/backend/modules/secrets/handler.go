// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package secrets

import (
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/therealmcsparrow/mcharbor/core/audit"
	"github.com/therealmcsparrow/mcharbor/core/auth"
	"github.com/therealmcsparrow/mcharbor/core/i18n"
	"github.com/therealmcsparrow/mcharbor/core/response"
	"github.com/therealmcsparrow/mcharbor/core/router"
	coresecrets "github.com/therealmcsparrow/mcharbor/core/secrets"
)

// revealReasonMinLength is the floor on the mandatory reveal justification.
// Ten characters is not a security control — it is a speed bump that makes
// "x" impossible and forces a sentence, which is what makes the audit entry
// worth reading six months later.
const revealReasonMinLength = 10

// Handler owns request parsing, validation, auth, audit logging and response
// writing for the vault. No SQL lives here.
type Handler struct {
	app     *router.AppDeps
	service *coresecrets.Service
}

// NewHandler builds the secrets handler and its service.
func NewHandler(app *router.AppDeps) *Handler {
	return &Handler{
		app:     app,
		service: coresecrets.NewService(app.DB, app.Encryption, app.RBACService),
	}
}

// respondServiceError maps the service's sentinel errors onto HTTP statuses
// and i18n codes. Anything unrecognised is logged server-side and returned as
// a generic internal error — a raw err.Error() reaching a client is how
// schema details and file paths leak.
func (h *Handler) respondServiceError(w http.ResponseWriter, r *http.Request, op string, err error, fallback i18n.MsgCode) {
	switch {
	case errors.Is(err, coresecrets.ErrNotFound):
		response.NotFoundCode(w, r, i18n.ErrSecretNotFound)
	case errors.Is(err, coresecrets.ErrNameInvalid):
		response.BadRequestCode(w, r, i18n.ErrSecretNameInvalid)
	case errors.Is(err, coresecrets.ErrScopeInvalid), errors.Is(err, coresecrets.ErrKindInvalid):
		response.BadRequestCode(w, r, i18n.ErrSecretNameInvalid)
	case errors.Is(err, coresecrets.ErrDuplicate):
		response.ConflictCode(w, r, i18n.ErrSecretDuplicate)
	case errors.Is(err, coresecrets.ErrInUse):
		response.ConflictCode(w, r, i18n.ErrSecretInUse)
	case errors.Is(err, coresecrets.ErrSystemImmutable):
		response.ForbiddenCode(w, r, i18n.ErrSecretSystemImmutable)
	case errors.Is(err, coresecrets.ErrGlobalDenied):
		response.ForbiddenCode(w, r, i18n.ErrSecretGlobalDenied)
	case errors.Is(err, coresecrets.ErrVersionNotFound):
		response.NotFoundCode(w, r, i18n.ErrSecretNotFound)
	case errors.Is(err, coresecrets.ErrRateLimited):
		response.ErrCode(w, r, http.StatusTooManyRequests, i18n.ErrSecretRevealRateLimited)
	default:
		h.app.Logger.Error("secrets: "+op+" failed", "error", err)
		response.InternalErrorCause(w, r, fallback, err)
	}
}

// HandleList returns a page of secret metadata. Never values.
// GET /api/secrets
func (h *Handler) HandleList(w http.ResponseWriter, r *http.Request) {
	page, perPage := response.ParsePagination(r)

	filter := coresecrets.ListFilter{
		EnvironmentID: response.ParseEnvID(r),
		IncludeSystem: r.URL.Query().Get("includeSystem") == "true",
		Search:        strings.TrimSpace(r.URL.Query().Get("search")),
		Group:         strings.TrimSpace(r.URL.Query().Get("group")),
	}

	items, total, err := h.service.List(r.Context(), filter, page, perPage)
	if err != nil {
		h.respondServiceError(w, r, "list", err, i18n.ErrSecretListFailed)
		return
	}

	response.Paginated(w, items, int64(total), page, perPage)
}

// HandleGet returns one secret's metadata. Never its value.
// GET /api/secrets/{id}
func (h *Handler) HandleGet(w http.ResponseWriter, r *http.Request) {
	sec, err := h.service.ByID(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		h.respondServiceError(w, r, "get", err, i18n.ErrSecretListFailed)
		return
	}
	response.OK(w, sec)
}

// HandleReferences lists what uses a secret.
// GET /api/secrets/{id}/references
func (h *Handler) HandleReferences(w http.ResponseWriter, r *http.Request) {
	refs, err := h.service.References(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		h.respondServiceError(w, r, "references", err, i18n.ErrSecretListFailed)
		return
	}
	response.OK(w, refs)
}

// HandleVersions lists a secret's rotation history — metadata only.
// GET /api/secrets/{id}/versions
func (h *Handler) HandleVersions(w http.ResponseWriter, r *http.Request) {
	versions, err := h.service.Versions(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		h.respondServiceError(w, r, "versions", err, i18n.ErrSecretListFailed)
		return
	}
	response.OK(w, versions)
}

// HandleCreate stores a new secret.
// POST /api/secrets
func (h *Handler) HandleCreate(w http.ResponseWriter, r *http.Request) {
	user := auth.RequireAuth(r)
	if user == nil {
		response.UnauthorizedCode(w, r, i18n.ErrUnauthorized)
		return
	}

	var req coresecrets.CreateRequest
	if err := response.DecodeBody(r, &req); err != nil {
		response.BadRequestCause(w, r, i18n.ErrInvalidBody, err)
		return
	}
	req.Name = strings.ToUpper(strings.TrimSpace(req.Name))
	req.Group = coresecrets.NormalizeGroup(req.Group)
	req.Kind = coresecrets.NormalizeKind(req.Kind)
	if err := coresecrets.ValidateName(req.Name); err != nil {
		response.BadRequestCause(w, r, i18n.ErrSecretNameInvalid, err)
		return
	}
	if req.Value == "" && !req.Generate {
		response.BadRequestCode(w, r, i18n.ErrInvalidBody)
		return
	}

	sec, generated, err := h.service.Create(r.Context(), user.ID, response.ParseEnvID(r), req)
	if err != nil {
		h.respondServiceError(w, r, "create", err, i18n.ErrSecretCreateFailed)
		return
	}

	h.app.AuditLog.Log(r, audit.Entry{
		Action:     "create_secret",
		EntityType: "secret",
		EntityID:   sec.ID,
		EntityName: sec.Name,
		Details:    "scope=" + sec.Scope + " kind=" + sec.Kind + groupDetail(sec),
	})

	// A generated value is shown once, in this response, so the common case
	// needs no elevation later. Everything else returns metadata only.
	if generated != "" {
		response.NoStore(w)
		response.Created(w, coresecrets.ValueResponse{Secret: sec, Value: generated})
		return
	}
	response.Created(w, sec)
}

// HandleRotate replaces a secret's value.
// PUT /api/secrets/{id}
func (h *Handler) HandleRotate(w http.ResponseWriter, r *http.Request) {
	user := auth.RequireAuth(r)
	if user == nil {
		response.UnauthorizedCode(w, r, i18n.ErrUnauthorized)
		return
	}

	var req coresecrets.RotateRequest
	if err := response.DecodeBody(r, &req); err != nil {
		response.BadRequestCause(w, r, i18n.ErrInvalidBody, err)
		return
	}
	if req.Value == "" && !req.Generate {
		response.BadRequestCode(w, r, i18n.ErrInvalidBody)
		return
	}

	id := chi.URLParam(r, "id")
	sec, generated, err := h.service.Rotate(r.Context(), user.ID, response.ParseEnvID(r), id, req)
	if err != nil {
		h.respondServiceError(w, r, "rotate", err, i18n.ErrSecretRotateFailed)
		return
	}

	h.app.AuditLog.Log(r, audit.Entry{
		Action:     "rotate_secret",
		EntityType: "secret",
		EntityID:   sec.ID,
		EntityName: sec.Name,
		Details:    "version=" + strconv.Itoa(sec.Version),
	})

	// Nothing that is already running has picked this up yet, and it will not
	// until something recreates it. A secret that opted into propagating its
	// own rotations gets that done here, in the background.
	h.autoApply(r.Context(), sec)

	if generated != "" {
		response.NoStore(w)
		response.OK(w, coresecrets.ValueResponse{Secret: sec, Value: generated})
		return
	}
	response.OK(w, sec)
}

// HandlePatch updates a secret's metadata.
// PATCH /api/secrets/{id}
func (h *Handler) HandlePatch(w http.ResponseWriter, r *http.Request) {
	user := auth.RequireAuth(r)
	if user == nil {
		response.UnauthorizedCode(w, r, i18n.ErrUnauthorized)
		return
	}

	var req coresecrets.PatchRequest
	if err := response.DecodeBody(r, &req); err != nil {
		response.BadRequestCause(w, r, i18n.ErrInvalidBody, err)
		return
	}

	id := chi.URLParam(r, "id")
	sec, err := h.service.Patch(r.Context(), user.ID, response.ParseEnvID(r), id, req)
	if err != nil {
		h.respondServiceError(w, r, "patch", err, i18n.ErrSecretCreateFailed)
		return
	}

	h.app.AuditLog.Log(r, audit.Entry{
		Action:     "update_secret",
		EntityType: "secret",
		EntityID:   sec.ID,
		EntityName: sec.Name,
		Details:    "scope=" + sec.Scope + " kind=" + sec.Kind + groupDetail(sec),
	})

	response.OK(w, sec)
}

// HandleRollback restores a historical value as the current one. It does not
// display the restored value.
// POST /api/secrets/{id}/rollback
func (h *Handler) HandleRollback(w http.ResponseWriter, r *http.Request) {
	user := auth.RequireAuth(r)
	if user == nil {
		response.UnauthorizedCode(w, r, i18n.ErrUnauthorized)
		return
	}

	var req coresecrets.RollbackRequest
	if err := response.DecodeBody(r, &req); err != nil {
		response.BadRequestCause(w, r, i18n.ErrInvalidBody, err)
		return
	}
	if req.Version <= 0 {
		response.BadRequestCode(w, r, i18n.ErrInvalidBody)
		return
	}

	id := chi.URLParam(r, "id")
	sec, err := h.service.Rollback(r.Context(), user.ID, response.ParseEnvID(r), id, req.Version)
	if err != nil {
		h.respondServiceError(w, r, "rollback", err, i18n.ErrSecretRollbackFailed)
		return
	}

	h.app.AuditLog.Log(r, audit.Entry{
		Action:     "rollback_secret",
		EntityType: "secret",
		EntityID:   sec.ID,
		EntityName: sec.Name,
		Details:    "restored=" + strconv.Itoa(req.Version),
	})

	// A rollback is a rotation that happens to restore an old value, so it
	// leaves everything running exactly as stale as a rotation does.
	h.autoApply(r.Context(), sec)

	response.OK(w, sec)
}

// HandleDelete removes a secret.
// DELETE /api/secrets/{id}[?force=true]
func (h *Handler) HandleDelete(w http.ResponseWriter, r *http.Request) {
	user := auth.RequireAuth(r)
	if user == nil {
		response.UnauthorizedCode(w, r, i18n.ErrUnauthorized)
		return
	}

	id := chi.URLParam(r, "id")
	sec, err := h.service.ByID(r.Context(), id)
	if err != nil {
		h.respondServiceError(w, r, "delete", err, i18n.ErrSecretDeleteFailed)
		return
	}

	force := r.URL.Query().Get("force") == "true"
	if err := h.service.Delete(r.Context(), id, force); err != nil {
		h.respondServiceError(w, r, "delete", err, i18n.ErrSecretDeleteFailed)
		return
	}

	h.app.AuditLog.Log(r, audit.Entry{
		Action:     "delete_secret",
		EntityType: "secret",
		EntityID:   sec.ID,
		EntityName: sec.Name,
		Details:    "scope=" + sec.Scope + " forced=" + strconv.FormatBool(force),
	})

	response.OKMsg(w, r, i18n.MsgSecretDeleted)
}

// HandleGenerate mints a value without storing it, so a client can show the
// user a strong password before deciding to keep it.
// POST /api/secrets/generate
func (h *Handler) HandleGenerate(w http.ResponseWriter, r *http.Request) {
	var req coresecrets.GenerateRequest
	// An empty body is fine here — it means "use the default length".
	if err := response.DecodeBody(r, &req); err != nil && !errors.Is(err, io.EOF) {
		response.BadRequestCode(w, r, i18n.ErrInvalidBody)
		return
	}

	value, err := coresecrets.GenerateValue(req.Length)
	if err != nil {
		h.app.Logger.Error("secrets: generate failed", "error", err)
		response.InternalErrorCause(w, r, i18n.ErrSecretGenerateFailed, err)
		return
	}

	response.NoStore(w)
	response.OK(w, coresecrets.ValueResponse{Value: value})
}

// HandleReveal returns a secret's plaintext value.
//
// This is the one endpoint in the system that hands back a stored credential,
// and the order of operations below is the whole of its defensibility:
//
//  1. POST, not GET — a GET lands in browser history, proxy access logs and
//     referrer headers, and is reachable by CSRF-able navigation.
//  2. The route gate is `secrets.reveal`, which is in no role preset, so
//     reaching this handler at all means an approver granted a time-bounded
//     elevation. `core/rbac/rbac.go` deliberately does not apply elevations on
//     the API-key path, so this is unreachable by an API key.
//  3. A reason of at least ten characters, recorded verbatim.
//  4. A per-user hourly rate limit.
//  5. The audit entry is written **before** the value is read, so a crash or
//     a dropped connection mid-response cannot produce an unlogged disclosure.
//  6. `Cache-Control: no-store`.
//
// POST /api/secrets/{id}/reveal
func (h *Handler) HandleReveal(w http.ResponseWriter, r *http.Request) {
	user := auth.RequireAuth(r)
	if user == nil {
		response.UnauthorizedCode(w, r, i18n.ErrUnauthorized)
		return
	}

	var req coresecrets.RevealRequest
	if err := response.DecodeBody(r, &req); err != nil {
		response.BadRequestCause(w, r, i18n.ErrInvalidBody, err)
		return
	}
	reason := strings.TrimSpace(req.Reason)
	if len([]rune(reason)) < revealReasonMinLength {
		response.BadRequestCode(w, r, i18n.ErrSecretRevealReason)
		return
	}

	id := chi.URLParam(r, "id")
	sec, err := h.service.ByID(r.Context(), id)
	if err != nil {
		h.respondServiceError(w, r, "reveal", err, i18n.ErrSecretRevealFailed)
		return
	}

	if err := h.service.RevealRateLimit(r.Context(), user.ID); err != nil {
		h.respondServiceError(w, r, "reveal rate limit", err, i18n.ErrSecretRevealFailed)
		return
	}

	// Both, not either. The permission (via an approved elevation) says an
	// approver agreed this person may reveal; the sudo window says the person
	// at the keyboard right now is that person. An elevation alone does not
	// survive a stolen session cookie, and a sudo window alone would let
	// anyone with a password read the vault. §5.9 B.
	if err := h.requireSudo(r); err != nil {
		h.app.AuditLog.Log(r, audit.Entry{
			Action:     "reveal_secret_denied",
			Severity:   "warning",
			EntityType: "secret",
			EntityID:   sec.ID,
			EntityName: sec.Name,
			Details:    "reason=no_step_up",
		})
		response.ForbiddenCode(w, r, i18n.ErrSudoRequired)
		return
	}

	// Step 5. Nothing below this line may run before the audit row exists.
	h.app.AuditLog.Log(r, audit.Entry{
		Action:     coresecrets.RevealAuditAction,
		Severity:   "warning",
		EntityType: "secret",
		EntityID:   sec.ID,
		EntityName: sec.Name,
		Details:    "scope=" + sec.Scope + " reason=" + reason,
	})

	value, err := h.service.Reveal(r.Context(), id)
	if err != nil {
		h.respondServiceError(w, r, "reveal", err, i18n.ErrSecretRevealFailed)
		return
	}

	h.app.Logger.Warn("secret revealed",
		"secret", sec.Name, "secret_id", sec.ID, "user", user.Username)

	// Detection beats prevention here, and the volume is naturally low — the
	// rate limit caps it at ten an hour per user. Best-effort: a notification
	// that could not be written must not fail a disclosure that has already
	// been audited.
	h.notifyAdministrators(r, user.Username, sec)

	response.NoStore(w)
	response.OK(w, coresecrets.ValueResponse{Value: value})
}

// requireSudo checks the caller's session for a live step-up window.
//
// The session id is the cookie value, never a parameter, so there is nothing
// here for a caller to spoof.
func (h *Handler) requireSudo(r *http.Request) error {
	if h.app.AuthService == nil {
		return nil
	}
	cookie, err := r.Cookie(auth.SessionCookie)
	if err != nil || cookie.Value == "" {
		return auth.ErrSudoRequired
	}
	ok, err := h.app.AuthService.HasSudoWindow(r.Context(), cookie.Value, auth.SudoScopeSecretsReveal)
	if err != nil {
		h.app.Logger.Error("secrets: checking step-up window failed", "error", err)
		return auth.ErrSudoRequired
	}
	if !ok {
		return auth.ErrSudoRequired
	}
	return nil
}

// notifyAdministrators writes an in-app notification for every reveal.
//
// A disclosure that nobody notices is the shape that matters: a permission
// holder quietly reading the vault looks exactly like legitimate use in every
// metric except "somebody saw it happen".
func (h *Handler) notifyAdministrators(r *http.Request, username string, sec *coresecrets.Secret) {
	if err := h.service.NotifyReveal(r.Context(), username, sec); err != nil {
		h.app.Logger.Warn("secrets: notifying administrators of a reveal failed", "error", err)
	}
}

// groupDetail renders a secret's group for an audit entry, or nothing when it
// has none — an empty "group=" in every pre-grouping entry would be noise.
func groupDetail(sec *coresecrets.Secret) string {
	if sec == nil || sec.Group == nil || *sec.Group == "" {
		return ""
	}
	return " group=" + *sec.Group
}

// secretEnvironment resolves the environment of the secret a /secrets/{id}
// route addresses, for rbac.PinEnvToResource.
func (h *Handler) secretEnvironment(r *http.Request) (string, bool, error) {
	envID, found, err := h.service.EnvironmentOf(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		h.app.Logger.Error("secrets: resolving the secret's environment", "error", err)
	}
	return envID, found, err
}
