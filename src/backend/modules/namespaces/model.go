// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package namespaces

// NamespaceSummary is a lightweight namespace representation.
type NamespaceSummary struct {
	Name      string            `json:"name"`
	Status    string            `json:"status"`
	Age       string            `json:"age"`
	Labels    map[string]string `json:"labels,omitempty"`
	CreatedAt string            `json:"createdAt"`
}
