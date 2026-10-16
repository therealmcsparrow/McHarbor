// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// Package lifecycle records and serves the timeline of what happened to
// containers, images and other Docker subjects, including plugin-emitted events.
package lifecycle

import (
	"context"
	"database/sql"
	"encoding/json"

	"github.com/therealmcsparrow/mcharbor/core/pluginruntime"
)

// HostBridge lets a plugin write the Logging timeline without reaching the
// table, which its scoped DB handle would refuse anyway.
type HostBridge struct {
	svc *Service
}

// NewHostBridge returns the bridge the plugin runtime hands to every plugin.
func NewHostBridge(db *sql.DB) *HostBridge {
	return &HostBridge{svc: NewService(db)}
}

var _ pluginruntime.LifecycleRecorder = (*HostBridge)(nil)

// Record writes one timeline entry, stamping source with the plugin's slug so
// the row is distinguishable from the Docker collector's.
func (b *HostBridge) Record(ctx context.Context, source string, ev pluginruntime.LifecycleRecord) error {
	if b == nil || b.svc == nil {
		return nil
	}

	var metadata *string
	if len(ev.Metadata) > 0 {
		encoded, err := json.Marshal(ev.Metadata)
		if err != nil {
			return err
		}
		text := string(encoded)
		metadata = &text
	}

	return b.svc.Insert(ctx, LifecycleEvent{
		EnvironmentID: nullIfBlank(ev.EnvironmentID),
		SubjectType:   ev.SubjectType,
		SubjectID:     ev.SubjectID,
		SubjectName:   nullIfBlank(ev.SubjectName),
		EventType:     ev.EventType,
		Action:        ev.Action,
		State:         nullIfBlank(ev.State),
		Severity:      ev.Severity,
		Metadata:      metadata,
		Source:        source,
	})
}

func nullIfBlank(value string) *string {
	if value == "" {
		return nil
	}
	return &value
}
