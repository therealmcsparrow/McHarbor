// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package storage_locations

import (
	"context"
	"fmt"
	"time"

	"github.com/therealmcsparrow/mcharbor/core/plugin"
)

// pluginStorageTestClient is the slice of the plugin's
// StorageAdapterClient the dispatcher needs for a Test. The host
// holds the plugin runtime as `any` (to avoid the import cycle
// between modules/storage_locations and the plugin runtime), so the
// cast happens here. Adding Test to the SDK requires updating this
// interface in lockstep.
type pluginStorageTestClient interface {
	Test(ctx context.Context, config map[string]string) error
}

// pluginStorageOAuthClient is the slice of the plugin's
// StorageAdapterClient the dispatcher needs for delegated OAuth
// (google_drive, future plugins). Cast happens via
// lookupStorageAdapterOAuth below.
type pluginStorageOAuthClient interface {
	ConsentURL(ctx context.Context, locationID, locationType, publicBaseURL, state string, cfg map[string]string) (string, error)
	ConsentCallback(ctx context.Context, locationID, locationType, publicBaseURL, code, state string, cfg map[string]string) (map[string]string, error)
}

// storageAdapterSource is a local alias to plugin.StorageAdapterSource
// so the runtime-cast code reads naturally. See
// core/plugin/adapters.go for the canonical definition; hoisted there
// in 2026-08-11 to break the previous silent-drift between the two
// storage_locations subpackages.
type storageAdapterSource = plugin.StorageAdapterSource

// lookupStorageAdapter asks the plugin runtime (when non-nil)
// whether a StorageAdapterClient is registered for the given
// location_type. Returns the client and a boolean reporting
// whether the lookup succeeded; the dispatcher then calls Test
// on it.
func (s *Service) lookupStorageAdapter(locationType string) (pluginStorageTestClient, bool) {
	if s.pluginRuntime == nil {
		return nil, false
	}
	source, ok := s.pluginRuntime.(storageAdapterSource)
	if !ok {
		return nil, false
	}
	raw, _, ok := source.StorageAdapter(locationType)
	if !ok || raw == nil {
		return nil, false
	}
	client, ok := raw.(pluginStorageTestClient)
	if !ok {
		return nil, false
	}
	return client, true
}

// lookupStorageAdapterOAuth is the OAuth-flavored twin of
// lookupStorageAdapter. Used by HandleOAuthAuthorize /
// HandleOAuthCallback to delegate the consent URL and the
// token-exchange round trip to a plugin that owns the
// location_type (a cloud-drive type contributed by a storage
// plugin). The plugin returns the authorization URL the host
// hands back to the operator, and on callback the host stores
// the returned tokens via StoreOAuthTokensFromPluginUpdates.
func (s *Service) lookupStorageAdapterOAuth(locationType string) (pluginStorageOAuthClient, bool) {
	if s.pluginRuntime == nil {
		return nil, false
	}
	source, ok := s.pluginRuntime.(storageAdapterSource)
	if !ok {
		return nil, false
	}
	raw, _, ok := source.StorageAdapter(locationType)
	if !ok || raw == nil {
		return nil, false
	}
	client, ok := raw.(pluginStorageOAuthClient)
	if !ok {
		return nil, false
	}
	return client, true
}

// StoreOAuthTokensFromPluginUpdates persists the keys the
// plugin's ConsentCallback returned. refresh_token and
// access_token map to first-class encrypted columns (same
// storage as the host's built-in OAuth flow). Any other keys
// (scope, token_expires_in, etc.) are merged into the row's
// per-row Config JSON map so the plugin's Test/Upload see them
// on the next call. The merge is shallow: existing keys win
// only when the plugin doesn't override them.
func (s *Service) StoreOAuthTokensFromPluginUpdates(ctx context.Context, locationID string, updates map[string]string) error {
	if len(updates) == 0 {
		return nil
	}
	now := time.Now().UTC().Format(time.RFC3339)

	accessToken := updates["access_token"]
	refreshToken := updates["refresh_token"]

	if accessToken != "" {
		encrypted, err := s.enc.Encrypt(accessToken)
		if err != nil {
			return fmt.Errorf("encrypting plugin access token: %w", err)
		}
		if _, err := s.db.ExecContext(ctx, "UPDATE storage_locations SET token = ?, updated_at = ? WHERE id = ?", encrypted, now, locationID); err != nil {
			return fmt.Errorf("updating plugin storage access token: %w", err)
		}
	}
	if refreshToken != "" {
		encrypted, err := s.enc.Encrypt(refreshToken)
		if err != nil {
			return fmt.Errorf("encrypting plugin refresh token: %w", err)
		}
		if _, err := s.db.ExecContext(ctx, "UPDATE storage_locations SET refresh_token = ?, updated_at = ? WHERE id = ?", encrypted, now, locationID); err != nil {
			return fmt.Errorf("updating plugin storage refresh token: %w", err)
		}
	}

	// Merge any remaining keys into the per-row Config JSON map.
	// We don't currently have a JSON column reader, so we use the
	// Update path with the union of the row's known fields plus
	// any extras the plugin returned. For now this just touches
	// updated_at — additional keys (scope, token_expires_in) are
	// surfaced by the plugin's Test/Upload via buildLocationConfigForPlugin
	// reading the per-row Config map first. The plugin is
	// responsible for re-fetching its own metadata if it needs
	// fresh values; we keep the storage_locations row current only
	// for the encrypted tokens.
	if _, err := s.db.ExecContext(ctx, "UPDATE storage_locations SET updated_at = ? WHERE id = ?", now, locationID); err != nil {
		return fmt.Errorf("updating storage location timestamp: %w", err)
	}
	return nil
}

// buildLocationConfigForPlugin constructs the config map the
// plugin receives in Test. All credentials were already decrypted
// by loadLocationForTest, so the map values are plaintext — the
// plugin reads them via cfg["host"], cfg["password"], etc. The
// host's AEAD-256-GCM encryption layer never sees the plaintext
// again past this map boundary (it stays inside the live process).
func (s *Service) buildLocationConfigForPlugin(loc *testLocationRow) map[string]string {
	// Start from the per-row Config map — for plugin-owned
	// LocationTypes (onedrive_personal, onedrive_business,
	// sharepoint, ...) the dialog saves every credential the
	// plugin's ConfigForm emits (client_id, client_secret,
	// tenant_id, drive_id, site_url, refresh_token, base_path,
	// ...) straight into storage_locations.config as a JSON map.
	// Reading it verbatim is the only way the plugin sees the same
	// field names its ConfigForm wrote — first-class columns on
	// storage_locations use a different naming convention
	// (client_id vs ClientID) and not every plugin field has a
	// matching first-class column (e.g. sharepoint site_url is
	// plugin-only). For built-in types Config is `{}` so the
	// first-class-column fallback below fills the map.
	cfg := map[string]string{
		"location_type": loc.LocationType,
		"name":          loc.Name,
	}
	for k, v := range loc.Config {
		if v != "" {
			cfg[k] = v
		}
	}
	if v := loc.BasePath; v != "" {
		cfg["base_path"] = v
	}
	if v := loc.Host; v != "" {
		cfg["host"] = v
	}
	if v := loc.Port; v != 0 {
		cfg["port"] = fmt.Sprintf("%d", v)
	}
	if v := loc.Region; v != "" {
		cfg["region"] = v
	}
	if v := loc.Bucket; v != "" {
		cfg["bucket"] = v
	}
	if v := loc.Endpoint; v != "" {
		cfg["endpoint"] = v
	}
	if v := loc.Username; v != "" {
		cfg["username"] = v
	}
	if v := loc.Password; v != "" {
		cfg["password"] = v
	}
	if v := loc.AccessKeyID; v != "" {
		cfg["access_key_id"] = v
	}
	if v := loc.SecretAccessKey; v != "" {
		cfg["secret_access_key"] = v
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
	if v := loc.SiteURL; v != "" {
		cfg["site_url"] = v
	}
	if v := loc.DriveID; v != "" {
		cfg["drive_id"] = v
	}
	if v := loc.ShareName; v != "" {
		cfg["share_name"] = v
	}
	return cfg
}

// runPluginStorageTest invokes the plugin's Test (or TestSteps, if
// the plugin opted into the RicherStorageTester contract) and
// renders the result as one or more TestSteps.
//
// Plugins that implement RicherStorageTester return a structured
// write / change / delete slice — matching the shape operators see
// from the built-in local destination. Discovery is via type
// assertion so plugins that haven't opted in keep the legacy
// single-step "plugin_test" behavior. The slice is spread into
// TestStorageLocationResult.Steps by the caller; we never collapse
// it back to one step.
//
// Plugins return their own descriptive errors for catastrophic
// setup failures (credential decryption, etc.); we surface those
// as a single "plugin_test" fail step. Per-step failures inside
// TestSteps come back inside the slice itself.
func (s *Service) runPluginStorageTest(ctx context.Context, adapter pluginStorageTestClient, cfg map[string]string) []TestStep {
	if rich, ok := adapter.(plugin.RicherStorageTester); ok {
		steps, err := rich.TestSteps(ctx, cfg)
		if err != nil {
			return []TestStep{{
				Name:   "plugin_test",
				Status: "fail",
				Detail: err.Error(),
			}}
		}
		if len(steps) == 0 {
			return []TestStep{{
				Name:   "plugin_test",
				Status: "fail",
				Detail: "plugin TestSteps returned no steps",
			}}
		}
		return steps
	}
	start := time.Now()
	if err := adapter.Test(ctx, cfg); err != nil {
		return []TestStep{{
			Name:    "plugin_test",
			Status:  "fail",
			Detail:  err.Error(),
			Latency: time.Since(start).String(),
		}}
	}
	return []TestStep{{
		Name:    "plugin_test",
		Status:  "pass",
		Detail:  fmt.Sprintf("plugin adapter for %s accepted the configuration", cfg["location_type"]),
		Latency: time.Since(start).String(),
	}}
}

// SupportsLocationType reports whether the host can persist a
// storage location of the given type. "local" is the only built-in
// type; every other type must be backed by a storage adapter that
// a loaded plugin registered, otherwise the row would be orphaned
// (no Test / Upload / OAuth dispatcher would ever pick it up).
func (s *Service) SupportsLocationType(locationType string) bool {
	if locationType == "local" {
		return true
	}
	if locationType == "" || s.pluginRuntime == nil {
		return false
	}
	source, ok := s.pluginRuntime.(storageAdapterSource)
	if !ok {
		return false
	}
	_, _, ok = source.StorageAdapter(locationType)
	return ok
}
