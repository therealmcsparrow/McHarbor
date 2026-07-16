// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package storage_locations

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// TestStep is a single named phase of the storage-connection test.
// Status is "pass", "warn", "fail", or "skip". A test result
// fails the overall connection test only if any step returns
// status="fail"; "warn" is informational (e.g. we couldn't run a
// full write/change/delete cycle on a cloud provider without a
// heavy SDK).
type TestStep struct {
	Name    string `json:"name"`
	Status  string `json:"status"`
	Detail  string `json:"detail,omitempty"`
	Latency string `json:"latency,omitempty"`
}

// TestStorageLocationResult is the JSON shape returned by
// /api/storage-locations/{id}/test. Each step reports its
// outcome so the operator can tell at a glance whether the
// failure was on the write, the change, or the delete (for
// local) or on the network probe (for cloud). OverallStatus
// mirrors the most severe step: pass / warn / fail.
type TestStorageLocationResult struct {
	StorageLocationID string     `json:"storageLocationId"`
	LocationName       string     `json:"locationName"`
	LocationType       string     `json:"locationType"`
	OverallStatus      string     `json:"overallStatus"`
	StartedAt          time.Time  `json:"startedAt"`
	CompletedAt        time.Time  `json:"completedAt"`
	Duration           string     `json:"duration"`
	Steps              []TestStep `json:"steps"`
}

// ErrTestNotApplicable is returned when the test is run against a
// location type the test endpoint can't validate (e.g. an unknown
// location_type). Surfaced as 200 OK with overallStatus="skip" so
// the UI can show the reason rather than treating it as a hard
// error.
var ErrTestNotApplicable = errors.New("test is not applicable for this storage location")

// TestStorageLocation exercises write / change / delete against
// the configured storage location and returns a per-step report.
// For local storage the full cycle runs synchronously inside the
// request context. For cloud storage (S3, OneDrive, Google Drive,
// FTP/SFTP/Samba/SharePoint) the heavy SDKs aren't in the
// storage_locations module's dependency tree (they live in
// container_backups), so the test instead does a TCP-level
// reachability probe to the provider's endpoint URL. That catches
// the most common production failure modes — bad host, wrong
// port, firewall rule, DNS resolution — without pulling in a
// multi-megabyte dependency just for a self-test.
//
// The probe is intentionally short: a 3-second dial timeout per
// attempt. Operators can re-run it freely; it doesn't touch any
// production data.
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
		LocationName:       loc.Name,
		LocationType:       loc.LocationType,
		OverallStatus:      "pass",
		StartedAt:          startedAt.UTC(),
	}

	// 1. Always: credential round-trip. If decrypt succeeds, the
	//    encryption key + stored value are healthy. A failure here
	//    means the operator's encryption key changed or the
	//    database is corrupt — actionable diagnostic, not a
	//    connectivity issue.
	result.Steps = append(result.Steps, s.testCredentialRoundTrip(loc))

	// 2. Per-type test. local runs the full write/change/delete
	//    cycle on the local filesystem. Cloud providers run the
	//    same cycle against their SDK (S3 with a hand-rolled
	//    SigV4 signer, OneDrive / SharePoint via the Graph API).
	//    For types we can't easily exercise here (SFTP/FTP/FTPS/
	//    Samba/Google Drive) the test falls back to a TCP probe so
	//    a misconfigured host/port/firewall still surfaces, and
	//    the actual cycle runs on the next scheduled backup.
	switch loc.LocationType {
	case "local":
		result.Steps = append(result.Steps, s.testLocalRoundTrip(ctx, loc)...)
	case "sftp":
		// TCP probe first so a bad host/port/firewall surfaces
		// as a clean "transport" step instead of a confusing
		// SSH-handshake timeout.
		result.Steps = append(result.Steps, s.testTCPConnect(loc.Host, loc.Port, "tcp", 3*time.Second))
		result.Steps = append(result.Steps, s.testSFTPRoundTrip(ctx, loc)...)
	case "ftp", "ftps":
		result.Steps = append(result.Steps, s.testTCPConnect(loc.Host, loc.Port, "tcp", 3*time.Second))
		result.Steps = append(result.Steps, TestStep{
			Name:   "write",
			Status: "skip",
			Detail: fmt.Sprintf("%s write/change/delete not exercised in self-test (add a server-side smoke test or run the next scheduled backup to verify)", loc.LocationType),
		})
	case "samba":
		result.Steps = append(result.Steps, s.testTCPConnect(loc.Host, 445, "tcp", 3*time.Second))
		result.Steps = append(result.Steps, TestStep{
			Name:   "write", Status: "skip",
			Detail: "samba write/change/delete not exercised in self-test",
		})
	case "aws":
		s3 := newS3TestClientFromLocation(loc)
		if s3 == nil {
			result.Steps = append(result.Steps, TestStep{
				Name: "write", Status: "fail",
				Detail: "AWS location is missing region, bucket, access key, or secret key",
			})
		} else {
			result.Steps = append(result.Steps, s.testS3RoundTrip(ctx, s3)...)
		}
	case "onedrive_personal", "onedrive_business", "sharepoint":
		result.Steps = append(result.Steps, s.testOneDriveRoundTrip(ctx, loc)...)
	case "google_drive":
		// Google Drive needs the OAuth refresh-token grant + the
		// Drive API. We don't import the SDK here; a TCP probe to
		// www.googleapis.com confirms the endpoint is reachable and
		// the operator's egress rules are correct.
		result.Steps = append(result.Steps, s.testTCPConnect("www.googleapis.com", 443, "tcp", 3*time.Second))
		result.Steps = append(result.Steps, TestStep{
			Name: "write", Status: "skip",
			Detail: "google_drive write/change/delete not exercised in self-test (next scheduled backup verifies)",
		})
	default:
		result.Steps = append(result.Steps, TestStep{
			Name:   "transport",
			Status: "skip",
			Detail: fmt.Sprintf("no probe available for storage type %q (covered by the next scheduled backup run)", loc.LocationType),
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
	var clientID, clientSecret, refreshToken, token sql.NullString
	var port sql.NullInt64
	var enabledInt int
	if err := s.db.QueryRowContext(ctx, `
		SELECT id, name, location_type, enabled, COALESCE(base_path, ''),
		       COALESCE(host, ''), port, COALESCE(region, ''), COALESCE(bucket, ''),
		       COALESCE(endpoint, ''), client_id, client_secret, refresh_token, token,
		       COALESCE(tenant_id, ''), COALESCE(site_url, ''), COALESCE(drive_id, ''),
		       COALESCE(share_name, ''), username, password, access_key_id, secret_access_key
		FROM storage_locations WHERE id = ?`, id,
	).Scan(
		&loc.ID, &loc.Name, &loc.LocationType, &enabledInt, &basePath, &host, &port,
		&region, &bucket, &endpoint, &clientID, &clientSecret, &refreshToken, &token,
		&tenantID, &siteURL, &driveID, &shareName, &username, &password,
		&accessKeyID, &secretAccessKey,
	); err != nil {
		return nil, err
	}
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
func (s *Service) testCredentialRoundTrip(loc *testLocationRow) TestStep {
	step := TestStep{Name: "credentials", Status: "pass"}
	start := time.Now()
	defer func() { step.Latency = time.Since(start).String() }()

	switch loc.LocationType {
	case "local", "sftp", "ftp", "ftps", "samba":
		// These don't need secrets at the protocol level beyond
		// the host (handled in the connectivity step). The
		// encryption key round-trip is implicit: if the
		// secrets column was non-null, the Decrypt above
		// would have returned an error.
		step.Detail = fmt.Sprintf("location type %q does not require encrypted credentials", loc.LocationType)
	case "aws":
		if loc.Region == "" && loc.Endpoint == "" {
			step.Status = "fail"
			step.Detail = "AWS location is missing both region and endpoint"
			return step
		}
		if loc.Bucket == "" {
			step.Status = "fail"
			step.Detail = "AWS location is missing bucket name"
			return step
		}
	case "google_drive":
		if loc.ClientID == "" || loc.ClientSecret == "" {
			step.Status = "fail"
			step.Detail = "Google Drive location is missing client id or client secret"
			return step
		}
		if loc.RefreshToken == "" && loc.Token == "" {
			step.Status = "warn"
			step.Detail = "Google Drive location has no refresh token or access token yet (run the OAuth flow first)"
			return step
		}
	case "onedrive_personal", "onedrive_business", "sharepoint":
		if loc.ClientID == "" || loc.ClientSecret == "" {
			step.Status = "fail"
			step.Detail = fmt.Sprintf("%s location is missing client id or client secret", loc.LocationType)
			return step
		}
		if loc.RefreshToken == "" && loc.Token == "" {
			step.Status = "warn"
			step.Detail = fmt.Sprintf("%s location has no refresh token or access token yet (run the OAuth flow first)", loc.LocationType)
			return step
		}
	}
	return step
}

// testTCPConnect does a short TCP dial to host:port. Used for
// every cloud / network storage type to catch the most common
// production failure modes (DNS, firewall, wrong host) before
// the next scheduled backup run.
func (s *Service) testTCPConnect(host string, port int, network string, timeout time.Duration) TestStep {
	step := TestStep{Name: "transport", Status: "pass"}
	start := time.Now()
	defer func() { step.Latency = time.Since(start).String() }()

	host = strings.TrimSpace(host)
	if host == "" {
		step.Status = "fail"
		step.Detail = "no host configured"
		return step
	}
	if port == 0 {
		step.Status = "fail"
		step.Detail = "no port configured"
		return step
	}
	if port < 1 || port > 65535 {
		step.Status = "fail"
		step.Detail = fmt.Sprintf("port %d out of range (1-65535)", port)
		return step
	}
	addr := fmt.Sprintf("%s:%d", host, port)
	dialer := net.Dialer{Timeout: timeout}
	conn, err := dialer.DialContext(context.Background(), network, addr)
	if err != nil {
		step.Status = "fail"
		step.Detail = fmt.Sprintf("dial %s failed: %v", network+" "+addr, err)
		return step
	}
	_ = conn.Close()
	step.Detail = fmt.Sprintf("%s connection to %s succeeded", network, addr)
	return step
}

// testLocalRoundTrip does the full write / change / delete cycle
// against a local storage location. The test artifact is a
// timestamped subdirectory under base_path with three small
// files; the change step renames the first file; the delete step
// removes the whole test directory. We never let the artifact
// leak — every error path includes a best-effort cleanup.
func (s *Service) testLocalRoundTrip(ctx context.Context, loc *testLocationRow) []TestStep {
	steps := make([]TestStep, 0, 4)
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
	// Ensure the directory is wiped if we return early.
	defer func() {
		if err := os.RemoveAll(testDir); err != nil {
			steps = append(steps, TestStep{
				Name: "cleanup", Status: "warn",
				Detail: fmt.Sprintf("failed to remove test directory %s: %v", testDir, err),
			})
		}
	}()

	// Step 1: write the initial file.
	writeStart := time.Now()
	testFile := filepath.Join(testDir, "original.txt")
	if err := os.MkdirAll(testDir, 0o755); err != nil {
		steps = append(steps, TestStep{
			Name: "write", Status: "fail",
			Detail:   fmt.Sprintf("mkdir %s: %v", testDir, err),
			Latency: time.Since(writeStart).String(),
		})
		return steps
	}
	original := []byte("mcharbor-conn-test\n" + suffix + "\n")
	if err := os.WriteFile(testFile, original, 0o644); err != nil {
		steps = append(steps, TestStep{
			Name: "write", Status: "fail",
			Detail:   fmt.Sprintf("write %s: %v", testFile, err),
			Latency: time.Since(writeStart).String(),
		})
		return steps
	}
	steps = append(steps, TestStep{
		Name: "write", Status: "pass",
		Detail: fmt.Sprintf("wrote %d bytes to %s", len(original), testFile),
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
			Detail:   fmt.Sprintf("rename %s -> %s: %v", testFile, renamed, err),
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
			Detail:   fmt.Sprintf("read after rename: %v", err),
			Latency: time.Since(changeStart).String(),
		})
		return steps
	}
	if string(got) != string(original) {
		steps = append(steps, TestStep{
			Name: "change", Status: "fail",
			Detail: "renamed file contents do not match the original",
			Latency: time.Since(changeStart).String(),
		})
		return steps
	}
	steps = append(steps, TestStep{
		Name: "change", Status: "pass",
		Detail: fmt.Sprintf("renamed to %s and contents match", renamed),
		Latency: time.Since(changeStart).String(),
	})

	// Step 3: delete. We use os.Remove on the renamed file so
	// the test is symmetric with the "delete one item" semantics
	// of the OneDrive / S3 test paths (which can't delete a
	// directory in one op). The deferred RemoveAll(testDir) will
	// also clean up the directory.
	deleteStart := time.Now()
	if err := os.Remove(renamed); err != nil {
		steps = append(steps, TestStep{
			Name: "delete", Status: "fail",
			Detail:   fmt.Sprintf("delete %s: %v", renamed, err),
			Latency: time.Since(deleteStart).String(),
		})
		return steps
	}
	// Confirm the file is actually gone.
	if _, err := os.Stat(renamed); !os.IsNotExist(err) {
		steps = append(steps, TestStep{
			Name: "delete", Status: "fail",
			Detail:   fmt.Sprintf("file still exists after delete: %v", err),
			Latency: time.Since(deleteStart).String(),
		})
		return steps
	}
	steps = append(steps, TestStep{
		Name: "delete", Status: "pass",
		Detail:   fmt.Sprintf("removed %s", renamed),
		Latency: time.Since(deleteStart).String(),
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

// testS3RoundTrip writes a small marker object, reads it back
// (with a content check), renames it, then deletes it. The full
// S3 cycle is exercised inline via the SigV4 signer in
// s3_test.go so we don't need to add the full AWS SDK just for
// the self-test endpoint.
//
// Each step is its own TestStep so the operator can tell at a
// glance whether the failure was on write / read / delete.
func (s *Service) testS3RoundTrip(ctx context.Context, c *s3TestClient) []TestStep {
	key := s3TestArtifactKey()
	body := []byte("mcharbor-conn-test\n" + testRandomSuffix() + "\n")

	// 1. write
	writeStart := time.Now()
	if err := c.putObject(ctx, key, body); err != nil {
		return []TestStep{{
			Name: "write", Status: "fail",
			Detail: fmt.Sprintf("PUT %s: %v", key, err),
			Latency: time.Since(writeStart).String(),
		}}
	}

	// 2. read (change = "can I read what I wrote" + same-key rename
	//    to verify S3 supports our key naming convention)
	got, err := c.getObject(ctx, key)
	if err != nil {
		_ = c.deleteObject(ctx, key)
		return []TestStep{
			{Name: "write", Status: "pass", Latency: time.Since(writeStart).String()},
			{Name: "read", Status: "fail", Detail: fmt.Sprintf("GET %s: %v", key, err)},
		}
	}
	if string(got) != string(body) {
		_ = c.deleteObject(ctx, key)
		return []TestStep{
			{Name: "write", Status: "pass", Latency: time.Since(writeStart).String()},
			{Name: "read", Status: "fail", Detail: "object contents do not match what was written"},
		}
	}

	// 3. change = "PUT again with different content under a new
	//    key" (S3 has no rename, so we copy to a new key, then
	//    delete the old one — semantically the same as rename).
	newKey := s3TestArtifactKey()
	changeStart := time.Now()
	if err := c.putObject(ctx, newKey, []byte("changed\n")); err != nil {
		_ = c.deleteObject(ctx, key)
		return []TestStep{
			{Name: "write", Status: "pass", Latency: time.Since(writeStart).String()},
			{Name: "read", Status: "pass"},
			{Name: "change", Status: "fail", Detail: fmt.Sprintf("PUT %s: %v", newKey, err), Latency: time.Since(changeStart).String()},
		}
	}
	if err := c.deleteObject(ctx, key); err != nil {
		_ = c.deleteObject(ctx, newKey)
		return []TestStep{
			{Name: "write", Status: "pass", Latency: time.Since(writeStart).String()},
			{Name: "read", Status: "pass"},
			{Name: "change", Status: "fail", Detail: fmt.Sprintf("DELETE old key: %v", err), Latency: time.Since(changeStart).String()},
		}
	}

	// 4. delete
	deleteStart := time.Now()
	if err := c.deleteObject(ctx, newKey); err != nil {
		return []TestStep{
			{Name: "write", Status: "pass", Latency: time.Since(writeStart).String()},
			{Name: "read", Status: "pass"},
			{Name: "change", Status: "pass", Latency: time.Since(changeStart).String()},
			{Name: "delete", Status: "fail", Detail: fmt.Sprintf("DELETE %s: %v", newKey, err), Latency: time.Since(deleteStart).String()},
		}
	}

	return []TestStep{
		{Name: "write", Status: "pass", Detail: fmt.Sprintf("wrote %d bytes to %s", len(body), key), Latency: time.Since(writeStart).String()},
		{Name: "read", Status: "pass", Detail: "object contents match what was written"},
		{Name: "change", Status: "pass", Detail: fmt.Sprintf("copied to %s and deleted original", newKey), Latency: time.Since(changeStart).String()},
		{Name: "delete", Status: "pass", Detail: fmt.Sprintf("removed %s", newKey), Latency: time.Since(deleteStart).String()},
	}
}

// testOneDriveRoundTrip does the same write/change/delete cycle
// against the user's OneDrive / SharePoint drive via the
// Microsoft Graph API. The OAuth access token is fetched
// synchronously inside the step (it has a 1-hour lifetime and
// we don't want a stale token from a previous request).
func (s *Service) testOneDriveRoundTrip(ctx context.Context, loc *testLocationRow) []TestStep {
	tokenStart := time.Now()
	token, err := onedriveAccessToken(ctx, loc)
	if err != nil {
		return []TestStep{{
			Name: "write", Status: "fail",
			Detail: fmt.Sprintf("fetching access token: %v", err),
			Latency: time.Since(tokenStart).String(),
		}}
	}
	// Determine whether we have a usable write target. With
	// delegated auth (refresh_token present) we can target
	// /me/drive. With app-only client_credentials we can only
	// target a specific drive via /drives/{id}/... If neither
	// works (e.g. someone configured the OneDrive location via
	// client_credentials but never set a valid drive_id), the
	// round-trip can't be exercised — return skip steps with a
	// config hint instead of forcing the operator to read past
	// a 400 they cannot fix.
	hasRefreshToken := strings.TrimSpace(loc.RefreshToken) != ""
	hasRealDriveID := strings.TrimSpace(normalizeDriveID(loc.DriveID)) != ""
	if !hasRefreshToken && !hasRealDriveID {
		return []TestStep{
			{Name: "write", Status: "skip", Detail: "no usable write target: location uses client_credentials (app-only) flow but has no valid drive_id. Either reconfigure the location using delegated auth (OAuth authorization-code flow, so a refresh_token is stored) or paste a real drive id (Microsoft Graph format: b!<opaque-string>) into the location's drive_id field."},
			{Name: "change", Status: "skip", Detail: "skipped because write target is missing (see write step)"},
			{Name: "delete", Status: "skip", Detail: "skipped because write target is missing (see write step)"},
		}
	}
	client := newOneDriveTestClientFromLocation(loc, token)
	if client == nil {
		return []TestStep{{
			Name: "write", Status: "fail",
			Detail: "onedrive client could not be constructed from this location's config",
		}}
	}

	path := onedriveTestArtifactPath()
	body := []byte("mcharbor-conn-test\n" + testRandomSuffix() + "\n")

	// 1. write
	writeStart := time.Now()
	if _, err := client.put(ctx, path, body); err != nil {
		return []TestStep{{
			Name: "write", Status: "fail",
			Detail: fmt.Sprintf("PUT %s: %v", path, err),
			Latency: time.Since(writeStart).String(),
		}}
	}

	// 2. read
	got, err := client.get(ctx, path)
	if err != nil {
		_ = client.delete(ctx, path)
		return []TestStep{
			{Name: "write", Status: "pass", Latency: time.Since(writeStart).String()},
			{Name: "read", Status: "fail", Detail: fmt.Sprintf("GET %s: %v", path, err)},
		}
	}
	if string(got) != string(body) {
		_ = client.delete(ctx, path)
		return []TestStep{
			{Name: "write", Status: "pass", Latency: time.Since(writeStart).String()},
			{Name: "read", Status: "fail", Detail: "file contents do not match what was written"},
		}
	}

	// 3. change = PUT again with different content under the same
	//    path. Graph PUT is full-overwrite, so this is also a
	//    "change" operation in the test's terms.
	changeStart := time.Now()
	if _, err := client.put(ctx, path, []byte("changed\n")); err != nil {
		_ = client.delete(ctx, path)
		return []TestStep{
			{Name: "write", Status: "pass", Latency: time.Since(writeStart).String()},
			{Name: "read", Status: "pass"},
			{Name: "change", Status: "fail", Detail: fmt.Sprintf("PUT %s: %v", path, err), Latency: time.Since(changeStart).String()},
		}
	}

	// 4. delete
	deleteStart := time.Now()
	if err := client.delete(ctx, path); err != nil {
		return []TestStep{
			{Name: "write", Status: "pass", Latency: time.Since(writeStart).String()},
			{Name: "read", Status: "pass"},
			{Name: "change", Status: "pass", Latency: time.Since(changeStart).String()},
			{Name: "delete", Status: "fail", Detail: fmt.Sprintf("DELETE %s: %v", path, err), Latency: time.Since(deleteStart).String()},
		}
	}

	return []TestStep{
		{Name: "write", Status: "pass", Detail: fmt.Sprintf("wrote %d bytes to %s", len(body), path), Latency: time.Since(writeStart).String()},
		{Name: "read", Status: "pass", Detail: "file contents match what was written"},
		{Name: "change", Status: "pass", Detail: "overwrote with new content", Latency: time.Since(changeStart).String()},
		{Name: "delete", Status: "pass", Detail: fmt.Sprintf("removed %s", path), Latency: time.Since(deleteStart).String()},
	}
}

// testSFTPRoundTrip exercises write / change / delete against an
// SFTP server (location_type == "sftp"). It speaks SFTP v3 inline
// on top of `x/crypto/ssh` (see sftp_client.go). The TCP probe
// step is added by the caller in the case branch — this helper
// only reports on what happens after the SSH/SFTP handshake.
//
// The pattern mirrors testS3RoundTrip / testOneDriveRoundTrip:
//   1. write  -- open(file,write|creat|trunc), write, close
//   2. read   -- open(file,read), read back, verify content
//   3. change -- PUT again with different content (creat|trunc)
//   4. delete -- REMOVE the file
//
// Each step is its own TestStep so the operator can see exactly
// where the cycle broke (auth vs. handshake vs. permission vs.
// path).
//
// The artifact lives under the location's configured base_path so
// operators running this against their own server know exactly
// where the leftover file would be if cleanup ever fails. If
// base_path is empty we default to "/" (the SSH user's home).
func (s *Service) testSFTPRoundTrip(ctx context.Context, loc *testLocationRow) []TestStep {
	// Build the artifact path. Strip trailing slashes and join
	// with our random suffix; if base_path is empty we land on
	// "/" which the SSH user can write in.
	basePath := strings.TrimSpace(loc.BasePath)
	if basePath == "" {
		basePath = "/"
	}
	basePath = strings.TrimRight(basePath, "/")
	path := basePath + "/mcharbor-conn-test-" + testRandomSuffix() + ".txt"
	body := []byte("mcharbor-conn-test\n" + testRandomSuffix() + "\n")

	client, dialErr := newSFTPClient(ctx, loc.Host, loc.Port, loc.Username, loc.Password, "", nil)
	if dialErr != nil {
		return []TestStep{{
			Name:   "write",
			Status: "fail",
			Detail: fmt.Sprintf("ssh/sftp handshake failed: %v", dialErr),
		}}
	}
	defer func() { _ = client.Close() }()

	// 1. write
	writeStart := time.Now()
	if err := client.Put(path, body); err != nil {
		return []TestStep{{
			Name:   "write",
			Status: "fail",
			Detail: err.Error(),
			Latency: time.Since(writeStart).String(),
		}}
	}

	// 2. read
	got, err := client.Get(path)
	if err != nil {
		_ = client.Remove(path)
		return []TestStep{
			{Name: "write", Status: "pass", Detail: fmt.Sprintf("wrote %d bytes to %s", len(body), path), Latency: time.Since(writeStart).String()},
			{Name: "read", Status: "fail", Detail: err.Error()},
		}
	}
	if !bytes.Equal(got, body) {
		_ = client.Remove(path)
		return []TestStep{
			{Name: "write", Status: "pass", Detail: fmt.Sprintf("wrote %d bytes to %s", len(body), path), Latency: time.Since(writeStart).String()},
			{Name: "read", Status: "fail", Detail: "file contents do not match what was written"},
		}
	}

	// 3. change -- re-PUT with different content under the same
	//    path. SFTP's OPEN(write|creat|trunc) truncates before
	//    write, so this overwrites cleanly.
	changeStart := time.Now()
	if err := client.Put(path, []byte("changed\n")); err != nil {
		_ = client.Remove(path)
		return []TestStep{
			{Name: "write", Status: "pass", Detail: fmt.Sprintf("wrote %d bytes to %s", len(body), path), Latency: time.Since(writeStart).String()},
			{Name: "read", Status: "pass"},
			{Name: "change", Status: "fail", Detail: err.Error(), Latency: time.Since(changeStart).String()},
		}
	}

	// 4. delete
	deleteStart := time.Now()
	if err := client.Remove(path); err != nil {
		return []TestStep{
			{Name: "write", Status: "pass", Detail: fmt.Sprintf("wrote %d bytes to %s", len(body), path), Latency: time.Since(writeStart).String()},
			{Name: "read", Status: "pass"},
			{Name: "change", Status: "pass", Latency: time.Since(changeStart).String()},
			{Name: "delete", Status: "fail", Detail: err.Error(), Latency: time.Since(deleteStart).String()},
		}
	}

	return []TestStep{
		{Name: "write", Status: "pass", Detail: fmt.Sprintf("wrote %d bytes to %s", len(body), path), Latency: time.Since(writeStart).String()},
		{Name: "read", Status: "pass", Detail: "file contents match what was written"},
		{Name: "change", Status: "pass", Detail: "overwrote with new content", Latency: time.Since(changeStart).String()},
		{Name: "delete", Status: "pass", Detail: fmt.Sprintf("removed %s", path), Latency: time.Since(deleteStart).String()},
	}
}
