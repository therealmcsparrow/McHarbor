// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package container_backups

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/therealmcsparrow/mcharbor/core/plugin"
)

// pluginStorageUploadClient is the slice of plugin.StorageAdapterClient
// the upload dispatcher needs. The runtime is stashed in
// Service.pluginStorageAdapter as `any`; the cast happens here.
// Adding Upload to the SDK requires updating this interface in
// lockstep.
type pluginStorageUploadClient interface {
	Upload(ctx context.Context, cfg map[string]string, key string, r io.Reader) error
}

// resolveStorageAdapter looks up the plugin-owned StorageAdapter
// for the given locationType. Same typing pattern as the snapshot
// adapter resolver in snapshot_config_dao.go: the runtime is
// stashed in Service.pluginStorageAdapter as `any` and the
// method asserts to plugin.StorageAdapterSource at boot. When
// the runtime is unset (the plugin loader hasn't been wired
// yet, or the plugin providing the location is disabled) the
// bool is false and the caller falls back to the built-in
// switch.
func (s *Service) resolveStorageAdapter(locationType string) (plugin.StorageAdapterClient, plugin.StorageAdapterSpec, bool) {
	if s.pluginStorageAdapter == nil {
		return nil, plugin.StorageAdapterSpec{}, false
	}
	src, ok := s.pluginStorageAdapter.(plugin.StorageAdapterSource)
	if !ok {
		return nil, plugin.StorageAdapterSpec{}, false
	}
	cli, spec, ok := src.StorageAdapter(locationType)
	if !ok || cli == nil {
		return nil, plugin.StorageAdapterSpec{}, false
	}
	return cli, spec, true
}

// buildLocationConfigForUpload constructs the config map the
// plugin receives in Upload. All credentials were already
// decrypted by backupStorageDestination; the map values are
// plaintext and the plugin reads them via cfg["client_id"],
// cfg["folder_id"], etc. The AES-256-GCM layer never sees the
// plaintext again past this map boundary.
//
// Plugin-owned LocationTypes (google_drive, onedrive_personal,
// onedrive_business, sharepoint, …) — when the plugin's
// ConfigForm is wired (see ProviderFormOutlet in the dialog) —
// persist every credential into storage_locations.config as a
// JSON map. That map is the source of truth and the first-class
// column fallback below is only consulted when the plugin
// doesn't populate the JSON map (e.g. on installations that
// pre-date the ProviderFormOutlet wiring and used the host's
// hardcoded form).
func (s *Service) buildLocationConfigForUpload(loc *backupStorageDestination) map[string]string {
	cfg := make(map[string]string, len(loc.Config)+10)
	for k, v := range loc.Config {
		if v != "" {
			cfg[k] = v
		}
	}
	cfg["location_type"] = loc.LocationType
	cfg["name"] = loc.Name
	if v := loc.BasePath; v != "" {
		cfg["base_path"] = v
	}
	if v := loc.Host; v != "" {
		cfg["host"] = v
	}
	if v := loc.ClientID; v != "" {
		cfg["client_id"] = v
	}
	if v := loc.ClientSecret; v != "" {
		cfg["client_secret"] = v
	}
	if v := loc.RefreshToken; v != "" {
		cfg["refresh_token"] = v
	}
	if v := loc.Token; v != "" {
		cfg["token"] = v
	}
	if v := loc.TenantID; v != "" {
		cfg["tenant_id"] = v
	}
	if v := loc.DriveID; v != "" {
		cfg["drive_id"] = v
	}
	if v := loc.ShareName; v != "" {
		cfg["share_name"] = v
	}
	return cfg
}

// uploadViaStorageAdapter dispatches a backup upload through the
// plugin's StorageAdapterClient.Upload. Opens archivePath,
// hands the reader to the plugin, and lets the plugin own every
// byte of the transport (HTTP, S3 SDK, OAuth refresh, etc.).
//
// Returns the storage key the plugin wrote to (the caller already
// recorded remotePath in container_backup_run_destinations before
// this call ran).
func (s *Service) uploadViaStorageAdapter(
	ctx context.Context,
	adapter pluginStorageUploadClient,
	cfg map[string]string,
	archivePath, key string,
) (string, error) {
	file, err := os.Open(archivePath)
	if err != nil {
		return "", fmt.Errorf("opening backup archive for plugin upload: %w", err)
	}
	defer closeLogged(file, "backup archive")

	if err := adapter.Upload(ctx, cfg, key, file); err != nil {
		return "", fmt.Errorf("plugin storage upload (%s): %w", strings.TrimSpace(cfg["location_type"]), err)
	}
	return key, nil
}
