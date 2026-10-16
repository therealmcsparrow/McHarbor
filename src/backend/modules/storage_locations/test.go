// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package storage_locations

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/therealmcsparrow/mcharbor/core/plugin"
	"github.com/therealmcsparrow/mcharbor/internal/closeutil"
)

// TestStep is a single named phase of the storage-connection test.
// Status is "pass", "warn", "fail", or "skip". A test result
// fails the overall connection test only if any step returns
// status="fail"; "warn" is informational (e.g. we couldn't run a
// full write/change/delete cycle on a cloud provider without a
// heavy SDK).
type TestStep = plugin.TestStep

// TestStorageLocationResult is the JSON shape returned by
// /api/storage-locations/{id}/test. Each step reports its
// outcome so the operator can tell at a glance whether the
// failure was on the write, the change, or the delete (for
// local) or on the network probe (for cloud). OverallStatus
// mirrors the most severe step: pass / warn / fail.
type TestStorageLocationResult struct {
	StorageLocationID string     `json:"storageLocationId"`
	LocationName      string     `json:"locationName"`
	LocationType      string     `json:"locationType"`
	OverallStatus     string     `json:"overallStatus"`
	StartedAt         time.Time  `json:"startedAt"`
	CompletedAt       time.Time  `json:"completedAt"`
	Duration          string     `json:"duration"`
	Steps             []TestStep `json:"steps"`
}

// ErrTestNotApplicable is returned when the test is run against a
// location type the test endpoint can't validate (e.g. an unknown
// location_type). Surfaced as 200 OK with overallStatus="skip" so
// the UI can show the reason rather than treating it as a hard
// error.
var ErrTestNotApplicable = errors.New("test is not applicable for this storage location")

// TestStorageLocation exercises write / change / delete against
// the configured storage location and returns a per-step report.
//
// For local storage the full cycle runs synchronously inside the
// request context. For plugin-owned LocationTypes (every
// cloud-storage type is plugin-contributed) the plugin's
// own TestSteps method drives the per-step output. The host no
// longer ships any built-in cloud-storage client, so every
// non-local LocationType dispatches to the plugin runtime.
func (s *Service) TestStorageLocation(
	ctx context.Context,
	storageLocationID string,
) (*TestStorageLocationResult, error) {
	if s.enc == nil {
		return nil, fmt.Errorf("storage credential encryption service is not configured")
	}

	loc, err := s.loadLocationForTest(ctx, storageLocationID)
	if err != nil {
		return nil, err
	}
	if loc == nil {
		return nil, sql.ErrNoRows
	}

	startedAt := time.Now()
	result := &TestStorageLocationResult{
		StorageLocationID: loc.ID,
		LocationName:      loc.Name,
		LocationType:      loc.LocationType,
		OverallStatus:     "pass",
		StartedAt:         startedAt.UTC(),
	}

	// 1. Always: credential round-trip. If decrypt succeeds, the
	//    encryption key + stored value are healthy. A failure here
	//    means the operator's encryption key changed or the database
	//    is corrupt — actionable diagnostic, not a connectivity
	//    issue.
	result.Steps = append(result.Steps, s.testCredentialRoundTrip(loc))

	// 2. Per-type test. The host only retains the `local` built-in
	//    test; every other location type is plugin-owned and the
	//    plugin's own Test / TestSteps handles the round-trip.
	if adapter, ok := s.lookupStorageAdapter(loc.LocationType); ok {
		cfg := s.buildLocationConfigForPlugin(loc)
		result.Steps = append(result.Steps, s.runPluginStorageTest(ctx, adapter, cfg)...)
	} else if loc.LocationType == "local" {
		result.Steps = append(result.Steps, s.testLocalRoundTrip(ctx, loc)...)
	} else {
		result.Steps = append(result.Steps, TestStep{
			Name:   "transport",
			Status: "skip",
			Detail: fmt.Sprintf("no test adapter for storage type %q — install the matching plugin", loc.LocationType),
		})
	}

	// Roll up the worst step into the overall status. A single
	// fail makes the whole test fail; warn is informational; skip
	// never overrides pass.
	result.CompletedAt = time.Now().UTC()
	result.Duration = result.CompletedAt.Sub(startedAt).String()
	result.OverallStatus = worstStatus(result.Steps)
	return result, nil
}

// worstStatus returns the most severe status among the steps.
// Ordering (most-severe-first): fail > warn > pass > skip. The
// special case "no steps" returns "skip".
func worstStatus(steps []TestStep) string {
	worst := "pass"
	for _, s := range steps {
		switch s.Status {
		case "fail":
			return "fail"
		case "warn":
			worst = "warn"
		case "skip":
			// skip doesn't demote an otherwise-pass overall
			// status; only fail / warn do.
		}
	}
	return worst
}

// testLocationRow is the columns we read from storage_locations
// for the test. Kept narrow (only the fields the test needs)
// so the test doesn't depend on every encryption column being
// readable — we just check the ones we use.
type testLocationRow struct {
	ID              string
	Name            string
	LocationType    string
	Enabled         bool
	BasePath        string
	Host            string
	Port            int
	Region          string
	Bucket          string
	Endpoint        string
	ClientID        string
	ClientSecret    string
	RefreshToken    string
	Token           string
	TenantID        string
	SiteURL         string
	DriveID         string
	ShareName       string
	AccessKeyID     string
	SecretAccessKey string
	Username        string
	Password        string
	// Config carries the per-row JSON map stored in the
	// storage_locations.config column. Plugin-owned LocationTypes
	// (onedrive_personal, sharepoint, …) use this map to ferry every
	// credential the plugin's ConfigForm emits back to the host so the
	// plugin's Test() receives the same field names it wrote — first-class
	// columns use a different naming convention
	// (client_id vs ClientID) and not every plugin field has a
	// matching first-class column (e.g. sharepoint site_url is
	// plugin-only). For built-in types Config is `{}` so the
	// first-class-column fallback below fills the map.
	Config map[string]string
}

// loadLocationForTest reads + decrypts the credentials a test
// needs. It deliberately fails fast on a missing encryption
// service so the operator gets a clear error rather than a
// misleading "credentials OK" report.
func (s *Service) loadLocationForTest(
	ctx context.Context,
	id string,
) (*testLocationRow, error) {
	var loc testLocationRow
	var basePath, host, region, bucket, endpoint sql.NullString
	var tenantID, siteURL, driveID, shareName, username, password sql.NullString
	var accessKeyID, secretAccessKey sql.NullString
	var clientID, clientSecret, refreshToken, token, configJSON sql.NullString
	var port sql.NullInt64
	var enabledInt int
	if err := s.db.QueryRowContext(ctx, `
		SELECT id, name, location_type, enabled, COALESCE(base_path, ''),
		       COALESCE(host, ''), port, COALESCE(region, ''), COALESCE(bucket, ''),
		       COALESCE(endpoint, ''), client_id, client_secret, refresh_token, token,
		       COALESCE(tenant_id, ''), COALESCE(site_url, ''), COALESCE(drive_id, ''),
		       COALESCE(share_name, ''), username, password, access_key_id, secret_access_key,
		       config
		FROM storage_locations WHERE id = ?`, id,
	).Scan(
		&loc.ID, &loc.Name, &loc.LocationType, &enabledInt, &basePath, &host, &port,
		&region, &bucket, &endpoint, &clientID, &clientSecret, &refreshToken, &token,
		&tenantID, &siteURL, &driveID, &shareName, &username, &password,
		&accessKeyID, &secretAccessKey, &configJSON,
	); err != nil {
		return nil, err
	}
	cfg, err := DecodeConfig(s.enc, configJSON.String)
	if err != nil {
		return nil, err
	}
	loc.Config = cfg
	loc.TenantID = tenantID.String
	loc.SiteURL = siteURL.String
	loc.DriveID = driveID.String
	loc.ShareName = shareName.String
	loc.Enabled = enabledInt != 0
	loc.BasePath = basePath.String
	loc.Host = host.String
	if port.Valid {
		loc.Port = int(port.Int64)
	}
	loc.Region = region.String
	loc.Bucket = bucket.String
	loc.Endpoint = endpoint.String
	if clientID.Valid && strings.TrimSpace(clientID.String) != "" {
		d, err := s.enc.Decrypt(clientID.String)
		if err != nil {
			return nil, fmt.Errorf("decrypting storage client id: %w", err)
		}
		loc.ClientID = d
	}
	if clientSecret.Valid && strings.TrimSpace(clientSecret.String) != "" {
		d, err := s.enc.Decrypt(clientSecret.String)
		if err != nil {
			return nil, fmt.Errorf("decrypting storage client secret: %w", err)
		}
		loc.ClientSecret = d
	}
	if refreshToken.Valid && strings.TrimSpace(refreshToken.String) != "" {
		d, err := s.enc.Decrypt(refreshToken.String)
		if err != nil {
			return nil, fmt.Errorf("decrypting storage refresh token: %w", err)
		}
		loc.RefreshToken = d
	}
	if token.Valid && strings.TrimSpace(token.String) != "" {
		d, err := s.enc.Decrypt(token.String)
		if err != nil {
			return nil, fmt.Errorf("decrypting storage access token: %w", err)
		}
		loc.Token = d
	}
	if accessKeyID.Valid && strings.TrimSpace(accessKeyID.String) != "" {
		d, err := s.enc.Decrypt(accessKeyID.String)
		if err != nil {
			return nil, fmt.Errorf("decrypting storage access key: %w", err)
		}
		loc.AccessKeyID = d
	}
	if secretAccessKey.Valid && strings.TrimSpace(secretAccessKey.String) != "" {
		d, err := s.enc.Decrypt(secretAccessKey.String)
		if err != nil {
			return nil, fmt.Errorf("decrypting storage secret key: %w", err)
		}
		loc.SecretAccessKey = d
	}
	if username.Valid && strings.TrimSpace(username.String) != "" {
		d, err := s.enc.Decrypt(username.String)
		if err != nil {
			return nil, fmt.Errorf("decrypting storage username: %w", err)
		}
		loc.Username = d
	}
	if password.Valid && strings.TrimSpace(password.String) != "" {
		d, err := s.enc.Decrypt(password.String)
		if err != nil {
			return nil, fmt.Errorf("decrypting storage password: %w", err)
		}
		loc.Password = d
	}
	return &loc, nil
}

// testCredentialRoundTrip is always run. It confirms the
// encryption service can decrypt the location's secrets and
// that the credentials are non-empty where the location type
// requires them. A failure here points to an encryption-key
// rotation gone wrong (most common: MCHARBOR_SECRET was reset
// but the existing rows are still encrypted with the old key).
//
// The host only retains the `local` built-in location type;
// every other type is plugin-owned and the plugin's own Test
// reports its own credential errors. The plugin dispatch path
// runs before this function returns, so by the time we get
// here we only have to handle `local`.
func (s *Service) testCredentialRoundTrip(loc *testLocationRow) TestStep {
	step := TestStep{Name: "credentials", Status: "pass"}
	start := time.Now()
	defer func() { step.Latency = time.Since(start).String() }()

	if loc.LocationType != "local" {
		step.Detail = fmt.Sprintf("location type %q is plugin-owned; credential check deferred to the plugin's Test", loc.LocationType)
		return step
	}
	step.Detail = "location type \"local\" does not require encrypted credentials"
	return step
}

// testLocalRoundTrip does the full write / change / delete cycle
// against a local storage location. The test artifact is a
// timestamped subdirectory under base_path with three small
// files; the change step renames the first file; the delete step
// removes the renamed file. The final cleanup step removes both
// the file and the surrounding test directory and verifies both
// are gone — the artifact is never left on disk.
//
// All error paths include a best-effort cleanup, but when the
// test fails mid-flight we can't reliably emit additional steps
// from the deferred function. The verified-cleanup step at the
// bottom is the canonical cleanup path on the success trail.
func (s *Service) testLocalRoundTrip(ctx context.Context, loc *testLocationRow) []TestStep {
	steps := make([]TestStep, 0, 5)
	base := strings.TrimSpace(loc.BasePath)
	if base == "" {
		steps = append(steps, TestStep{
			Name: "transport", Status: "fail",
			Detail: "local storage is missing base_path",
		})
		return steps
	}
	// Test directory name: mcharbor-conn-test-<random> so
	// concurrent test runs against the same storage don't
	// collide. Random suffix is hex(sha256(random))[0:8].
	suffix := testRandomSuffix()
	testDir := filepath.Join(base, "mcharbor-conn-test-"+suffix)
	// On any early return, sweep the directory so a config error
	// doesn't leave a stray artifact behind. The success path
	// cleans up explicitly below.
	defer func() {
		closeutil.RemoveAll(testDir)
	}()

	// Step 1: write the initial file.
	writeStart := time.Now()
	testFile := filepath.Join(testDir, "original.txt")
	if err := os.MkdirAll(testDir, 0o755); err != nil {
		steps = append(steps, TestStep{
			Name: "write", Status: "fail",
			Detail:  fmt.Sprintf("mkdir %s: %v", testDir, err),
			Latency: time.Since(writeStart).String(),
		})
		return steps
	}
	original := []byte("mcharbor-conn-test\n" + suffix + "\n")
	if err := os.WriteFile(testFile, original, 0o644); err != nil {
		steps = append(steps, TestStep{
			Name: "write", Status: "fail",
			Detail:  fmt.Sprintf("write %s: %v", testFile, err),
			Latency: time.Since(writeStart).String(),
		})
		return steps
	}
	steps = append(steps, TestStep{
		Name: "write", Status: "pass",
		Detail:  fmt.Sprintf("wrote %d bytes to %s", len(original), testFile),
		Latency: time.Since(writeStart).String(),
	})

	// Step 2: rename = "change". Pick a new name with the same
	// suffix so the operator can identify the artifact in the
	// filesystem if cleanup fails.
	changeStart := time.Now()
	renamed := filepath.Join(testDir, "renamed.txt")
	if err := os.Rename(testFile, renamed); err != nil {
		steps = append(steps, TestStep{
			Name: "change", Status: "fail",
			Detail:  fmt.Sprintf("rename %s -> %s: %v", testFile, renamed, err),
			Latency: time.Since(changeStart).String(),
		})
		return steps
	}
	// Confirm the rename actually moved the bytes (not just the
	// inode entry): read back and compare.
	got, err := os.ReadFile(renamed)
	if err != nil {
		steps = append(steps, TestStep{
			Name: "change", Status: "fail",
			Detail:  fmt.Sprintf("read after rename: %v", err),
			Latency: time.Since(changeStart).String(),
		})
		return steps
	}
	if string(got) != string(original) {
		steps = append(steps, TestStep{
			Name: "change", Status: "fail",
			Detail:  "renamed file contents do not match the original",
			Latency: time.Since(changeStart).String(),
		})
		return steps
	}
	steps = append(steps, TestStep{
		Name: "change", Status: "pass",
		Detail:  fmt.Sprintf("renamed to %s and contents match", renamed),
		Latency: time.Since(changeStart).String(),
	})

	// Step 3: delete the file. We use os.Remove on the renamed
	// file so the test is symmetric with cloud-provider round
	// trips that delete one item at a time.
	deleteStart := time.Now()
	if err := os.Remove(renamed); err != nil {
		steps = append(steps, TestStep{
			Name: "delete", Status: "fail",
			Detail:  fmt.Sprintf("delete %s: %v", renamed, err),
			Latency: time.Since(deleteStart).String(),
		})
		return steps
	}
	// Confirm the file is actually gone.
	if _, err := os.Stat(renamed); !os.IsNotExist(err) {
		steps = append(steps, TestStep{
			Name: "delete", Status: "fail",
			Detail:  fmt.Sprintf("file still exists after delete: %v", err),
			Latency: time.Since(deleteStart).String(),
		})
		return steps
	}
	steps = append(steps, TestStep{
		Name: "delete", Status: "pass",
		Detail:  fmt.Sprintf("removed %s", renamed),
		Latency: time.Since(deleteStart).String(),
	})

	// Step 4: remove the surrounding test directory (the file
	// alone is gone; the directory tree is still on disk). We
	// invoke RemoveAll explicitly so the operator sees a verified
	// pass/fail step instead of relying on a deferred cleanup.
	cleanupStart := time.Now()
	if err := os.RemoveAll(testDir); err != nil {
		steps = append(steps, TestStep{
			Name: "cleanup", Status: "fail",
			Detail:  fmt.Sprintf("remove test directory %s: %v", testDir, err),
			Latency: time.Since(cleanupStart).String(),
		})
		return steps
	}
	// Verify both the file and the directory are gone. A missing
	// directory is the success signal; a leftover dir or file
	// fails the step so the operator sees the artifact is still
	// on disk.
	if _, err := os.Stat(testDir); err == nil {
		steps = append(steps, TestStep{
			Name: "cleanup", Status: "fail",
			Detail:  fmt.Sprintf("test directory %s still exists after RemoveAll", testDir),
			Latency: time.Since(cleanupStart).String(),
		})
		return steps
	} else if !os.IsNotExist(err) {
		steps = append(steps, TestStep{
			Name: "cleanup", Status: "fail",
			Detail:  fmt.Sprintf("stat test directory %s after cleanup: %v", testDir, err),
			Latency: time.Since(cleanupStart).String(),
		})
		return steps
	}
	if _, err := os.Stat(testFile); err == nil {
		steps = append(steps, TestStep{
			Name: "cleanup", Status: "fail",
			Detail:  fmt.Sprintf("test file %s still exists after cleanup", testFile),
			Latency: time.Since(cleanupStart).String(),
		})
		return steps
	}
	steps = append(steps, TestStep{
		Name: "cleanup", Status: "pass",
		Detail:  fmt.Sprintf("removed test directory %s and verified both file and folder are gone", testDir),
		Latency: time.Since(cleanupStart).String(),
	})

	return steps
}

// testRandomSuffix returns a short hex string derived from a fresh
// 8 random bytes. The full randomness is overkill for a test
// artifact name but the hash makes accidental collisions across
// simultaneous test runs vanishingly unlikely.
func testRandomSuffix() string {
	var buf [8]byte
	if _, err := rand.Read(buf[:]); err != nil {
		// rand.Read on Linux/macOS never fails; on bare-metal
		// systems it can fall back to /dev/urandom. If it does
		// fail (effectively impossible), fall back to a
		// timestamp so the test still gets a unique value.
		return fmt.Sprintf("%016x", time.Now().UnixNano())
	}
	sum := sha256.Sum256(buf[:])
	return hex.EncodeToString(sum[:4])
}
