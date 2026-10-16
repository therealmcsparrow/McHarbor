// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package appstore

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/rs/xid"

	"github.com/therealmcsparrow/mcharbor/core/telemetry"
)

// registryMetaRefresher keeps the appstore_registry_meta cache warm without
// ever making a page view wait on a registry. A listing reports what the cache
// holds and queues whatever has gone stale; the workers do the network.
type registryMetaRefresher struct {
	db      *sql.DB
	logger  *slog.Logger
	queue   chan string
	mu      sync.Mutex
	pending map[string]bool
	once    sync.Once
}

func newRegistryMetaRefresher(db *sql.DB, logger *slog.Logger) *registryMetaRefresher {
	queue := make(chan string, registryRefreshQueue)
	telemetry.RegisterQueue("registry_meta_refresh",
		func() float64 { return float64(len(queue)) },
		func() float64 { return float64(cap(queue)) })
	return &registryMetaRefresher{
		db:      db,
		logger:  logger,
		queue:   queue,
		pending: make(map[string]bool),
	}
}

// Start launches the worker pool. The workers exit when ctx is cancelled,
// which is the lifecycle the server shuts down on; Start is idempotent so a
// second Mount cannot double the pool.
func (r *registryMetaRefresher) Start(ctx context.Context) {
	r.once.Do(func() {
		var wg sync.WaitGroup
		for i := 0; i < registryRefreshWorkers; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				r.work(ctx)
			}()
		}
		go func() {
			<-ctx.Done()
			wg.Wait()
			r.logger.Debug("app store registry metadata refresher stopped")
		}()
	})
}

// work pulls image references off the queue until the context ends.
func (r *registryMetaRefresher) work(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case image, ok := <-r.queue:
			if !ok {
				return
			}
			r.refresh(ctx, image)
		}
	}
}

// enqueue schedules a lookup, dropping duplicates and, when the queue is
// saturated, the request itself — the next listing re-queues it.
func (r *registryMetaRefresher) enqueue(image string) {
	image = strings.TrimSpace(image)
	if image == "" {
		return
	}

	r.mu.Lock()
	if r.pending[image] {
		r.mu.Unlock()
		return
	}
	r.pending[image] = true
	r.mu.Unlock()

	select {
	case r.queue <- image:
	default:
		r.mu.Lock()
		delete(r.pending, image)
		r.mu.Unlock()
	}
}

// refresh resolves one image and records the verdict, success or failure.
func (r *registryMetaRefresher) refresh(ctx context.Context, image string) {
	defer func() {
		r.mu.Lock()
		delete(r.pending, image)
		r.mu.Unlock()
	}()

	meta, err := resolveRegistryMeta(ctx, image)
	if err != nil {
		if ctx.Err() != nil {
			return
		}
		r.logger.Debug("app store registry lookup failed", "image", image, "error", err)
		meta = registryMeta{Status: "error", Error: truncateError(err.Error())}
	}
	if err := upsertRegistryMeta(ctx, r.db, image, meta); err != nil {
		r.logger.Error("failed to store app store registry metadata", "image", image, "error", err)
	}
}

// truncateError keeps a cached failure reason short enough to log and read.
func truncateError(message string) string {
	const maxLen = 240
	if len(message) <= maxLen {
		return message
	}
	return message[:maxLen]
}

// upsertRegistryMeta writes one cache row, keyed by the image reference.
func upsertRegistryMeta(ctx context.Context, db *sql.DB, image string, meta registryMeta) error {
	now := time.Now().UTC().Format(time.RFC3339)
	status := meta.Status
	if status == "" {
		status = "error"
	}

	_, err := db.ExecContext(ctx, `
		INSERT INTO appstore_registry_meta
			(id, image_ref, latest_version, released_at, status, error, checked_at, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(image_ref) DO UPDATE SET
			latest_version = excluded.latest_version,
			released_at = excluded.released_at,
			status = excluded.status,
			error = excluded.error,
			checked_at = excluded.checked_at,
			updated_at = excluded.updated_at
	`, xid.New().String(), image, meta.Version, meta.Released, status, meta.Error, now, now, now)
	if err != nil {
		return fmt.Errorf("upserting registry metadata: %w", err)
	}
	return nil
}

// registryMetaStale reports whether a cached row is old enough to look up
// again. A missing row is always stale; a failed one is retried on the shorter
// error TTL so a rate-limited registry is not asked on every request.
func registryMetaStale(meta registryMeta, ok bool) bool {
	if !ok {
		return true
	}
	checked, err := time.Parse(time.RFC3339, meta.CheckedAt)
	if err != nil {
		return true
	}
	ttl := registryMetaTTL
	if meta.Status != "ok" {
		ttl = registryMetaErrorTTL
	}
	return time.Since(checked) > ttl
}

const registryMetaHead = `
		SELECT image_ref, latest_version, released_at, status, checked_at
		FROM appstore_registry_meta
		WHERE image_ref IN (`

const registryMetaTail = `)
		LIMIT 1000
	`

// loadRegistryMeta reads the cached rows for a set of image references.
func (s *Service) loadRegistryMeta(ctx context.Context, images []string) (map[string]registryMeta, error) {
	result := make(map[string]registryMeta, len(images))
	if len(images) == 0 {
		return result, nil
	}

	args := make([]any, 0, len(images))
	for _, image := range images {
		args = append(args, image)
	}
	placeholders := strings.TrimSuffix(strings.Repeat("?,", len(args)), ",")
	query := registryMetaHead + placeholders + registryMetaTail // SQL identifier: placeholders is only "?" repeated len(images) times, never caller input

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("querying registry metadata: %w", err)
	}
	defer func() {
		if err := rows.Close(); err != nil {
			s.logger.Warn("appstore: closing registry metadata rows failed", "error", err)
		}
	}()

	for rows.Next() {
		var image string
		var meta registryMeta
		if err := rows.Scan(&image, &meta.Version, &meta.Released, &meta.Status, &meta.CheckedAt); err != nil {
			return nil, fmt.Errorf("scanning registry metadata: %w", err)
		}
		result[image] = meta
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating registry metadata: %w", err)
	}
	return result, nil
}

// applyRegistryMeta fills the registry-sourced fields on a page of apps and
// queues a refresh for every image whose cache entry is missing or stale.
//
// The cards render whatever is cached — including nothing on a cold install,
// where the frontend falls back to the catalog's own version and sync date —
// because a listing that blocked on 25 registries would be a listing nobody
// waits for.
func (s *Service) applyRegistryMeta(ctx context.Context, apps []AppTemplate) {
	if len(apps) == 0 {
		return
	}

	images := make([]string, 0, len(apps))
	seen := make(map[string]bool, len(apps))
	for _, app := range apps {
		image := strings.TrimSpace(app.Image)
		if image == "" || seen[image] {
			continue
		}
		seen[image] = true
		images = append(images, image)
	}

	cached, err := s.loadRegistryMeta(ctx, images)
	if err != nil {
		s.logger.Error("failed to load app store registry metadata", "error", err)
		return
	}

	for i := range apps {
		image := strings.TrimSpace(apps[i].Image)
		if image == "" {
			continue
		}
		meta, ok := cached[image]
		if ok && meta.Status == "ok" {
			apps[i].RegistryVersion = meta.Version
			apps[i].RegistryUpdatedAt = meta.Released
		}
		if s.registryMeta != nil && registryMetaStale(meta, ok) {
			s.registryMeta.enqueue(image)
		}
	}
}
