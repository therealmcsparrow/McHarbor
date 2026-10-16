// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// Package permissions maps each operationId to the permission keys its route
// enforces, so a refused command can name the key instead of a bare 403.
package permissions

//go:generate go run ../../tools/permgen -spec ../../../backend/api/openapi.json -out permissions.gen.go

// For returns the permission keys the operation requires, or nil when the
// operation is ungated or unknown to the spec this CLI was built from.
func For(operationID string) []string {
	return byOperation[operationID]
}
