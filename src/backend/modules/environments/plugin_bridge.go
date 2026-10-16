// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package environments

import (
	"context"
	"database/sql"
	"strings"
)

// PluginBridge answers plugin.EnvironmentHandle on the host's behalf.
type PluginBridge struct {
	db *sql.DB
}

// NewPluginBridge returns the host-side environment name lookup plugins use.
func NewPluginBridge(db *sql.DB) *PluginBridge {
	return &PluginBridge{db: db}
}

// EnvironmentName returns the environment's display name, falling back to the
// id.
//
// Never empty and never an error: the callers name a storage path after it,
// and a missing name must degrade to an id rather than fail the operation
// that needed it.
func (b *PluginBridge) EnvironmentName(ctx context.Context, environmentID string) string {
	environmentID = strings.TrimSpace(environmentID)
	if b == nil || b.db == nil || environmentID == "" {
		return environmentID
	}
	var name string
	err := b.db.QueryRowContext(ctx,
		"SELECT name FROM environments WHERE id = ? LIMIT 1", environmentID).Scan(&name)
	if err != nil || strings.TrimSpace(name) == "" {
		return environmentID
	}
	return name
}
