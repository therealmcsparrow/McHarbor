// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package agent

import (
	"context"
	"time"

	coreagent "github.com/therealmcsparrow/mcharbor/core/agent"
)

// The offline replay path writes rows that two other modules own: the
// activity feed's event taxonomy and the metrics module's sample tables.
// Rather than importing those modules (a module must not depend on another
// module), the agent declares the two narrow contracts it needs here and
// main.go injects an implementation built in internal/bootstrap.
//
// Both are optional: with no bridge installed the corresponding half of a
// buffered batch is dropped rather than stored, and the batch is still
// acknowledged — the same outcome as an environment with that tracking
// switch turned off.

// OfflineEventClassifier maps a raw Docker event onto the activity feed's
// subject / severity taxonomy so replayed events are indistinguishable from
// events the live collector recorded.
type OfflineEventClassifier interface {
	// SubjectTypeForDockerEvent returns the activity subject type for a
	// Docker event type, and false when the type is not tracked.
	SubjectTypeForDockerEvent(eventType string) (string, bool)
	// LifecycleSeverity returns the (severity, state) pair for an event.
	LifecycleSeverity(eventType, action string) (string, string)
}

// OfflineMetricsIngester writes one tick of buffered container samples
// through the metrics module's own ingest path, so replayed samples land in
// the same tables and rollups as live ones.
type OfflineMetricsIngester interface {
	IngestSamples(ctx context.Context, envID string, sampledAt time.Time, samples []coreagent.OfflineMetricEntry) error
}

// SetOfflineBridges installs the cross-module collaborators the offline
// replay path delegates to. Called from main.go at mount time.
func (s *Service) SetOfflineBridges(classifier OfflineEventClassifier, ingester OfflineMetricsIngester) {
	s.eventClassifier = classifier
	s.metricsIngester = ingester
}
