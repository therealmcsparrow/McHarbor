// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package containers

import (
	"net/http"
	"strings"

	"github.com/therealmcsparrow/mcharbor/core/auth"
	"github.com/therealmcsparrow/mcharbor/core/i18n"
	"github.com/therealmcsparrow/mcharbor/core/rbac"
	"github.com/therealmcsparrow/mcharbor/core/response"
	"github.com/therealmcsparrow/mcharbor/core/secrets"
)

// secretMountRoot is the directory file-based secret injection materialises
// plaintext into. Anything at or below it is a stored credential in the clear,
// which is why the file browser treats it as a separate privilege from the
// `containers.view` that opens the rest of the filesystem.
var secretMountRoot = strings.TrimSuffix(secrets.SecretMountPath, "/")

// isSecretPath reports whether p is the secret mount itself or anything under
// it.
func isSecretPath(p string) bool {
	cleaned := cleanContainerPath(p)
	return cleaned == secretMountRoot || strings.HasPrefix(cleaned, secretMountRoot+"/")
}

// hideSecretEntries drops the secret mount from a directory listing so a user
// without `secrets.reveal` never learns the path exists. Listing `/run` is the
// case that matters: the gate below stops the descent, but an unfiltered parent
// still advertises the target.
func hideSecretEntries(entries []FileEntry) []FileEntry {
	out := entries[:0]
	for _, entry := range entries {
		if isSecretPath(entry.Path) {
			continue
		}
		out = append(out, entry)
	}
	return out
}

// canRevealSecrets reports whether the caller holds `secrets.reveal` for envID.
//
// It fails closed: an unresolvable RBAC service or a lookup error denies, because
// the alternative is handing back a credential on the strength of a database
// hiccup.
func (h *Handler) canRevealSecrets(r *http.Request, envID string) bool {
	user := auth.RequireAuth(r)
	if user == nil || h.app.RBACService == nil {
		return false
	}
	allowed, err := h.app.RBACService.HasPermission(r.Context(), user.ID, envID, rbac.PermSecretsReveal)
	if err != nil {
		h.app.Logger.Error("checking secrets.reveal for file browser failed",
			"env", envID, "user", user.ID, "error", err)
		return false
	}
	return allowed
}

// guardSecretPath answers the request itself when p reaches into the secret
// mount and the caller may not reveal secrets, and audits the attempt either
// way. A true return means the handler may continue. It runs guardProtectedPath
// first, so every file-browser route that calls it also keeps McHarbor's own
// database, keys and secrets out of reach.
//
// Both outcomes are logged at warning: a permitted read is still a plaintext
// credential leaving the vault, and the reveal endpoint records its successes
// the same way.
func (h *Handler) guardSecretPath(
	w http.ResponseWriter, r *http.Request, envID, id, action, p string,
) bool {
	if !h.guardProtectedPath(w, r, envID, id, action, p) {
		return false
	}
	if !isSecretPath(p) {
		return true
	}
	if !h.canRevealSecrets(r, envID) {
		h.logSecretFileAudit(r, envID, id, action+"_denied", p)
		response.ForbiddenCode(w, r, i18n.ErrSecretPathForbidden)
		return false
	}
	h.logSecretFileAudit(r, envID, id, action, p)
	return true
}

// logSecretFileAudit records one attempt to reach the secret mount through the
// container file browser.
func (h *Handler) logSecretFileAudit(r *http.Request, envID, id, action, p string) {
	h.logContainerAuditSeverity(r, envID, action, id, "warning",
		"path="+cleanContainerPath(p)+" mount="+secretMountRoot)
}
