// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package rbac

import (
	"net/http"

	"github.com/therealmcsparrow/mcharbor/core/rbac"
	"github.com/therealmcsparrow/mcharbor/core/response"
)

// PolicyReference is everything the policy editor explains: the resource types a
// policy can target (host and plugin), the variables every expression sees, and the
// helper functions it may call.
type PolicyReference struct {
	ResourceTypes []rbac.ResourceTypeInfo  `json:"resourceTypes"`
	Variables     []rbac.ResourceAttribute `json:"variables"`
	Helpers       []rbac.PolicyHelper      `json:"helpers"`
}

// HandleReference returns the live reference; plugin types appear as soon as their
// plugin loads, so the editor never lists a type nothing can resolve.
func (h *Handler) HandleReference(w http.ResponseWriter, r *http.Request) {
	response.OK(w, PolicyReference{
		ResourceTypes: rbac.ResourceTypes(),
		Variables:     rbac.PolicyVariables(),
		Helpers:       rbac.PolicyHelpers(),
	})
}
