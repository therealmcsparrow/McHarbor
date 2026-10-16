// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package identity

import (
	"database/sql"
	"errors"
	"net/http"
	"strings"

	"github.com/therealmcsparrow/mcharbor/core/audit"
	"github.com/therealmcsparrow/mcharbor/core/httpx"
	"github.com/therealmcsparrow/mcharbor/core/i18n"
	"github.com/therealmcsparrow/mcharbor/core/response"
)

// ProviderEndpointURLs are the addresses an operator registers on the identity
// provider's side. They come from the same base URL the login flow uses, so a
// reverse proxy or configured public URL is reflected exactly.
type ProviderEndpointURLs struct {
	OIDCRedirectURI string `json:"oidcRedirectUri"`
	LoginURL        string `json:"loginUrl,omitempty"`
	SAMLACSURL      string `json:"samlAcsUrl,omitempty"`
	SAMLMetadataURL string `json:"samlMetadataUrl,omitempty"`
}

// EndpointURLs builds the provider-facing URLs; the per-provider ones need an
// id, so a provider still being created only gets the shared OIDC redirect.
func EndpointURLs(baseURL, providerID string) ProviderEndpointURLs {
	base := strings.TrimRight(baseURL, "/") + "/api/identity-providers"
	out := ProviderEndpointURLs{OIDCRedirectURI: base + "/callback"}
	if providerID != "" {
		out.LoginURL = base + "/" + providerID + "/authorize"
		out.SAMLACSURL = base + "/" + providerID + "/acs"
		out.SAMLMetadataURL = base + "/" + providerID + "/metadata"
	}
	return out
}

// HandleEndpointURLs returns the callback, ACS and metadata URLs for the
// optional ?providerId=, checking the provider exists when one is named.
func (h *Handler) HandleEndpointURLs(w http.ResponseWriter, r *http.Request) {
	providerID := strings.TrimSpace(r.URL.Query().Get("providerId"))
	if providerID != "" {
		provider, err := h.service.ByID(r.Context(), providerID)
		if err != nil {
			h.app.Logger.Error("identity endpoint urls lookup failed", "error", err, "id", providerID)
			response.InternalErrorCause(w, r, i18n.ErrIdentityListFailed, err)
			return
		}
		if provider == nil {
			response.NotFoundCode(w, r, i18n.ErrIdentityNotFound)
			return
		}
	}
	response.OK(w, EndpointURLs(httpx.BaseURL(r), providerID))
}

// HandleMetadataImport reads SAML IdP metadata from a URL or pasted XML and
// returns what it found plus the config keys the host can prefill.
func (h *Handler) HandleMetadataImport(w http.ResponseWriter, r *http.Request) {
	var input MetadataImportInput
	if err := response.DecodeBody(r, &input); err != nil {
		response.BadRequestCause(w, r, i18n.ErrInvalidBody, err)
		return
	}
	result, err := h.service.ImportMetadata(r.Context(), input)
	if err != nil {
		reason := metadataFailureReason(err)
		h.app.Logger.Warn("identity metadata import failed", "error", err, "reason", reason)
		response.ErrCodeWithData(w, r, http.StatusBadRequest, i18n.ErrInvalidBody, map[string]string{"reason": reason})
		return
	}
	h.app.AuditLog.Log(r, audit.Entry{
		Action:     "identity_provider.metadata_imported",
		Severity:   "info",
		EntityType: "identity_provider",
		EntityName: result.Metadata.EntityID,
	})
	response.OK(w, result)
}

func metadataFailureReason(err error) string {
	for _, known := range []error{ErrMetadataInvalidURL, ErrMetadataFetchFailed, ErrMetadataNotSAML, ErrMetadataNoIdP, ErrMetadataInputMissing} {
		if errors.Is(err, known) {
			return known.Error()
		}
	}
	return "unknown"
}

// HandleMappingPreview shows what a group-mapping configuration would grant,
// for sample provider groups and for users who already signed in. Read-only.
func (h *Handler) HandleMappingPreview(w http.ResponseWriter, r *http.Request) {
	var input MappingPreviewInput
	if err := response.DecodeBody(r, &input); err != nil {
		response.BadRequestCause(w, r, i18n.ErrInvalidBody, err)
		return
	}
	if len(input.SampleGroups) > 500 || len(input.Config.GroupMappings) > 500 {
		response.ErrCodeWithData(w, r, http.StatusBadRequest, i18n.ErrInvalidBody, map[string]string{"reason": "too_many_entries"})
		return
	}
	preview, err := h.service.PreviewMapping(r.Context(), input)
	if errors.Is(err, sql.ErrNoRows) {
		response.NotFoundCode(w, r, i18n.ErrIdentityNotFound)
		return
	}
	if err != nil {
		h.app.Logger.Error("identity mapping preview failed", "error", err, "provider", input.ProviderID)
		response.InternalErrorCause(w, r, i18n.ErrInternalServer, err)
		return
	}
	response.OK(w, preview)
}
