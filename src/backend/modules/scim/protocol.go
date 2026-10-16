// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package scim

import (
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/therealmcsparrow/mcharbor/core/i18n"
)

// Group is the SCIM 2.0 group representation. McHarbor roles
// map onto groups; members are user ids joined through user_roles.
type Group struct {
	Schemas     []string `json:"schemas"`
	ID          string   `json:"id"`
	DisplayName string   `json:"displayName"`
	Description string   `json:"description,omitempty"`
	Members     []Member `json:"members,omitempty"`
	Meta        *Meta    `json:"meta,omitempty"`
}

// Member is one entry in a group's members array.
type Member struct {
	Value   string `json:"value"`
	Display string `json:"display,omitempty"`
	Type    string `json:"type,omitempty"`
}

// encodeSCIM writes v as JSON. We re-use the standard encoder
// rather than pulling in a third-party SCIM-specific marshaler
// because our SCIM shapes are all plain JSON with the right tag
// annotations.
func encodeSCIM(w http.ResponseWriter, v any) {
	if err := json.NewEncoder(w).Encode(v); err != nil {
		slog.Warn("scim: writing response failed", "error", err)
	}
}

// writeSCIMJSON is the canonical responder for /api/scim/v2/*. It
// sets the SCIM content type per RFC 7644 §3.8 and writes v.
func writeSCIMJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/scim+json")
	w.WriteHeader(status)
	encodeSCIM(w, v)
}

// --- i18n helpers ----------------------------------------------------
//
// These thin wrappers exist so callers don't have to repeat the
// qualified i18n constant name on every line. They also let us
// scope the SCIM-specific codes to a single file in case we ever
// need to change the response shape (e.g. add an extra error
// detail field).

func i18nErrSCIMFilterInvalid() i18n.MsgCode { return i18n.ErrSCIMFilterInvalid }
func i18nErrInternalServer() i18n.MsgCode    { return i18n.ErrInternalServer }
func i18nErrInvalidBody() i18n.MsgCode       { return i18n.ErrInvalidBody }
func i18nErrNotFound() i18n.MsgCode          { return i18n.ErrNotFound }

// ServiceProviderConfig returns the discovery document advertised
// at /api/scim/v2/ServiceProviderConfig.
func ServiceProviderConfig() ProviderConfig {
	return ProviderConfig{
		Schemas:        []string{"urn:ietf:params:scim:schemas:core:2.0:ServiceProviderConfig"},
		Patch:          Supported{Supported: false},
		Bulk:           BulkSupport{Supported: Supported{Supported: false}, MaxOperations: 0, MaxPayloadSize: 0},
		Filter:         Supported{Supported: true},
		ChangePassword: Supported{Supported: false},
		Sort:           Supported{Supported: false},
		ETag:           Supported{Supported: false},
		AuthenticationSchemes: []AuthenticationScheme{{
			Type:        "httpbearer",
			Name:        "Bearer Token",
			Description: "Authentication scheme using a bearer token in the Authorization header",
			SpecURI:     "https://tools.ietf.org/html/rfc6750",
		}},
	}
}

// Schemas returns the static list of resource schemas supported.
func Schemas() []map[string]any {
	return []map[string]any{
		{
			"id":          "urn:ietf:params:scim:schemas:core:2.0:User",
			"name":        "User",
			"description": "User account",
		},
		{
			"id":          "urn:ietf:params:scim:schemas:core:2.0:Group",
			"name":        "Group",
			"description": "Group (role) of users",
		},
	}
}
