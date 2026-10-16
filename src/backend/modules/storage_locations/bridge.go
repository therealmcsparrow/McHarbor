// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// Package storage_locations manages backup and object-storage destinations and the bridge plugins upload through.
package storage_locations

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/therealmcsparrow/mcharbor/core/plugin"
	"github.com/therealmcsparrow/mcharbor/core/pluginruntime"
	"github.com/therealmcsparrow/mcharbor/core/router"
	"github.com/therealmcsparrow/mcharbor/internal/closeutil"
)

// HostBridge is the host side of plugin.StorageHandle.
//
// A plugin names a location by id; this resolves the row, decrypts its
// credentials, and dispatches to the storage adapter that owns the
// location type. No credential crosses the plugin boundary.
type HostBridge struct {
	app     *router.AppDeps
	service *Service
}

// NewHostBridge builds the bridge the plugin loader hands to plugins.
func NewHostBridge(app *router.AppDeps, service *Service) *HostBridge {
	return &HostBridge{app: app, service: service}
}

// Upload writes data to key under the given location.
//
// Both outcomes are audited. The success row alone told an operator
// which uploads worked and nothing at all about the ones that did not —
// the case they actually go to the audit log to investigate.
func (b *HostBridge) Upload(
	ctx context.Context,
	pluginSlug, locationID, key string,
	data []byte,
) (err error) {
	// The audit row is written from a defer so every return path below
	// is covered, including the ones that fail before a destination is
	// resolved. loc is captured by reference for the same reason.
	var loc *StorageLocation
	defer func() {
		b.auditUpload(ctx, pluginSlug, locationID, key, len(data), loc, err)
	}()

	loc, err = b.service.ByID(ctx, locationID)
	if err != nil {
		return fmt.Errorf("resolving storage location: %w", err)
	}
	if loc == nil {
		return fmt.Errorf("storage location %s not found", locationID)
	}
	if !loc.Enabled {
		return fmt.Errorf("storage location %s is disabled", loc.Name)
	}

	// Plugin adapter first, built-in second — the same order
	// container_backups uses (storage_upload.go). `local` is a
	// host-owned destination: no credentials, no provider, and no
	// storage-local plugin, so resolveAdapter will never match it. This
	// bridge originally had only the adapter half of that pattern, which
	// made the one destination every picker offers the one destination no
	// plugin could write to.
	if adapter, ok := b.resolveAdapter(loc.LocationType); ok {
		cfg, cfgErr := b.buildConfig(ctx, loc)
		if cfgErr != nil {
			return cfgErr
		}
		if upErr := adapter.Upload(ctx, cfg, key, bytes.NewReader(data)); upErr != nil {
			// The adapter's error can carry provider detail; it is logged
			// with context and returned to the plugin, which is host-side
			// code, not a client response.
			return fmt.Errorf("storage upload (%s): %w", loc.LocationType, upErr)
		}
	} else if loc.LocationType == "local" {
		if localErr := uploadLocal(loc, key, data); localErr != nil {
			return localErr
		}
	} else {
		return fmt.Errorf("no storage adapter registered for %s — install the matching plugin", loc.LocationType)
	}

	return nil
}

// auditUpload records one bridge upload attempt, succeeded or failed.
//
// pluginSlug is host-supplied by the loader's storageAdapter, so a
// plugin cannot attribute an upload to a different plugin.
func (b *HostBridge) auditUpload(ctx context.Context, pluginSlug, locationID, key string, size int, loc *StorageLocation, err error) {
	if b.app == nil || b.app.AuditLog == nil {
		return
	}
	fields := map[string]any{
		"location_id": locationID,
		"key":         key,
		"bytes":       size,
	}
	severity := "success"
	if loc != nil {
		fields["location_type"] = loc.LocationType
	}
	if err != nil {
		severity = "error"
		fields["outcome"] = "failure"
		fields["error"] = pluginruntime.RedactAuditError(err)
	} else {
		fields["outcome"] = "success"
	}
	// The upload's own ctx may already be cancelled, which is often why it failed; the audit row must still land.
	if auditErr := b.app.AuditLog.EmitPluginSeverityContext(context.WithoutCancel(ctx), pluginSlug, "plugin.storage.uploaded", severity, fields); auditErr != nil && b.app.Logger != nil {
		b.app.Logger.Error("storage bridge: audit upload", "error", auditErr, "location_id", locationID)
	}
}

// Locations lists the enabled destinations a plugin may upload to.
// Names and types only — never a credential.
func (b *HostBridge) Locations(ctx context.Context) []pluginruntime.StorageLocationInfo {
	list, err := b.service.List(ctx)
	if err != nil {
		if b.app != nil && b.app.Logger != nil {
			b.app.Logger.Error("storage bridge: listing locations", "error", err)
		}
		return nil
	}

	out := make([]pluginruntime.StorageLocationInfo, 0, len(list))
	for _, l := range list {
		if !l.Enabled {
			continue
		}
		out = append(out, pluginruntime.StorageLocationInfo{
			ID:           l.ID,
			Name:         l.Name,
			LocationType: l.LocationType,
		})
	}
	return out
}

// uploadLocal writes to the host filesystem under the location's base
// path.
//
// Two things this must get right, neither of which the caller can be
// trusted with:
//
//  1. Containment. `key` comes from a plugin. The reporting plugin
//     sanitises its own, but that is one caller's discipline, not a
//     property of the boundary — so the resolved path is checked against
//     the base and anything landing outside it is refused. A `..` or an
//     absolute key must not become a write anywhere on the host.
//  2. Atomicity. A temp file in the destination directory, renamed into
//     place. Anything watching the folder — a sync client, a backup
//     agent, the operator — must never open a half-written document.
func uploadLocal(loc *StorageLocation, key string, data []byte) error {
	base := strings.TrimSpace(loc.BasePath)
	if base == "" {
		return fmt.Errorf("storage location %s has no base path", loc.Name)
	}
	base = filepath.Clean(base)

	target := filepath.Clean(filepath.Join(base, filepath.FromSlash(key)))
	rel, err := filepath.Rel(base, target)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return fmt.Errorf("storage key escapes the base path of location %s", loc.Name)
	}

	dir := filepath.Dir(target)
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return fmt.Errorf("creating local storage directory: %w", err)
	}

	tmp, err := os.CreateTemp(dir, ".mcharbor-upload-*")
	if err != nil {
		return fmt.Errorf("creating local storage temp file: %w", err)
	}
	tmpName := tmp.Name()
	committed := false
	defer func() {
		if !committed {
			// Best effort: the write already failed, and a leftover
			// dotfile is a smaller problem than masking the real error.
			closeutil.Remove(tmpName)
		}
	}()

	if _, err := tmp.Write(data); err != nil {
		closeutil.Report(tmp.Close(), "tmp")
		return fmt.Errorf("writing local storage file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("closing local storage file: %w", err)
	}
	// 0640 rather than the 0600 CreateTemp gives us: the destination is
	// routinely a bind mount an operator reads from outside the
	// container, and a file only root can read is not a delivered report.
	if err := os.Chmod(tmpName, 0o640); err != nil {
		return fmt.Errorf("setting local storage file mode: %w", err)
	}
	if err := os.Rename(tmpName, target); err != nil {
		return fmt.Errorf("finalising local storage file: %w", err)
	}
	committed = true
	return nil
}

func (b *HostBridge) resolveAdapter(locationType string) (plugin.StorageAdapterClient, bool) {
	if b.app == nil || b.app.PluginAdapters == nil {
		return nil, false
	}
	src, ok := b.app.PluginAdapters.(plugin.StorageAdapterSource)
	if !ok {
		return nil, false
	}
	cli, _, ok := src.StorageAdapter(locationType)
	if !ok || cli == nil {
		return nil, false
	}
	return cli, true
}

// buildConfig assembles the plaintext config map the adapter reads.
//
// Typed columns are laid down first and the plugin's own JSON config is
// overlaid on top. That order matters: a plugin-owned destination
// persists every field into the JSON map and leaves the typed columns
// NULL, so reading the columns alone yields an empty config for exactly
// the destinations plugins care about most.
func (b *HostBridge) buildConfig(ctx context.Context, loc *StorageLocation) (map[string]string, error) {
	cfg := map[string]string{
		"location_type": loc.LocationType,
		"name":          loc.Name,
	}
	put := func(k, v string) {
		if strings.TrimSpace(v) != "" {
			cfg[k] = v
		}
	}
	put("base_path", loc.BasePath)
	put("host", loc.Host)
	put("region", loc.Region)
	put("bucket", loc.Bucket)
	put("endpoint", loc.Endpoint)
	put("tenant_id", loc.TenantID)
	put("site_url", loc.SiteURL)
	put("drive_id", loc.DriveID)
	put("share_name", loc.ShareName)
	if loc.Port > 0 {
		cfg["port"] = strconv.Itoa(loc.Port)
	}

	secrets, err := b.service.DecryptedSecrets(ctx, loc.ID)
	if err != nil {
		return nil, fmt.Errorf("decrypting storage credentials: %w", err)
	}
	if secrets != nil {
		put("client_id", secrets.ClientID)
		put("client_secret", secrets.ClientSecret)
		put("refresh_token", secrets.RefreshToken)
		put("token", secrets.Token)
		put("username", secrets.Username)
		put("password", secrets.Password)
		put("private_key", secrets.PrivateKey)
		put("passphrase", secrets.Passphrase)
		put("access_key_id", secrets.AccessKeyID)
		put("secret_access_key", secrets.SecretAccessKey)
		for k, v := range secrets.Extra {
			put(k, v)
		}
	}
	return cfg, nil
}
