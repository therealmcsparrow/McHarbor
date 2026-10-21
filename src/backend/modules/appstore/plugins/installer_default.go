// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package plugins

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/google/uuid"
	"gopkg.in/yaml.v3"

	"github.com/therealmcsparrow/mcharbor/core/audit"
	"github.com/therealmcsparrow/mcharbor/core/egress"
	"github.com/therealmcsparrow/mcharbor/core/encryption"
	"github.com/therealmcsparrow/mcharbor/core/plugin"
	"github.com/therealmcsparrow/mcharbor/core/pluginslug"
	appversion "github.com/therealmcsparrow/mcharbor/core/version"
)

// httpDownloadTimeout bounds the entire HTTP fetch (dial + TLS +
// headers + body). Most plugin tarballs are 1-30 MiB so 30s leaves
// headroom for slow networks without hanging on a stalled peer.
const httpDownloadTimeout = 30 * time.Second

// httpDownloadMaxBytes caps the response body at 50 MiB to bound
// memory. Plugin tarballs today are typically <2 MiB; the cap is a
// safety net against a malicious server streaming unbounded data.
const httpDownloadMaxBytes = 50 * 1024 * 1024

// DefaultInstaller is the production Installer. It owns the install
// pipeline (verify → extract → manifest → migrations → persist),
// the 7-step uninstall gate chain, and the Configure flow. All
// behavior that touches the host filesystem or DB lives here.
type DefaultInstaller struct {
	dataDir    string
	db         *sql.DB
	svc        *Service
	enc        *encryption.Service
	audit      *audit.Logger
	contrib    Contributor
	logger     Logger
	trustKeys  []ed25519.PublicKey
	platformOK bool
	// requireSignature restores the old refuse-on-unsigned behavior.
	// Off by default: an operator running a self-hosted host must be
	// able to install a plugin nobody signed, and confirms it before it
	// extracts. Set MCHARBOR_PLUGIN_REQUIRE_SIGNATURE=true on a host
	// that should only ever run tarballs signed by a pinned key.
	requireSignature bool
	// catalogSync re-projects plugin-declared catalog sources into
	// settings_catalogs after an install or uninstall, so the category a
	// plugin brings appears and disappears without waiting for a restart.
	// nil in tests and wherever the host wired no catalogs service.
	catalogSync func(context.Context) error
	store       PackageStore
	// catalogPurge removes the catalog sources left behind for kinds no
	// remaining plugin declares. Injected from bootstrap because
	// settings_catalogs belongs to the catalogs module, not to this one.
	catalogPurge func(context.Context, []string) (int, error)
}

// WithCatalogPurge attaches the purge the PurgeData option calls. Set rather
// than passed to NewInstaller for the same reason WithCatalogSync is: the
// catalogs service is constructed after the installer.
func (d *DefaultInstaller) WithCatalogPurge(purge func(context.Context, []string) (int, error)) *DefaultInstaller {
	d.catalogPurge = purge
	return d
}

// WithCatalogSync attaches the projection that publishes plugin-declared
// catalog sources. Set rather than passed to NewInstaller because the
// catalogs service is constructed after the installer.
func (d *DefaultInstaller) WithCatalogSync(sync func(context.Context) error) *DefaultInstaller {
	d.catalogSync = sync
	return d
}

// syncCatalogs runs the projection and reports a failure without failing the
// operation around it: the rows are re-projected at the next boot anyway, and
// an install that already succeeded must not be reported as broken because a
// settings row is one restart late.
func (d *DefaultInstaller) syncCatalogs(ctx context.Context, slug string) {
	if d.catalogSync == nil {
		return
	}
	if err := d.catalogSync(ctx); err != nil && d.logger != nil {
		d.logger.Warn("plugin catalogs not published, retrying at next boot",
			"slug", slug, "error", err)
	}
}

// Logger is the small subset of *slog.Logger the installer needs. We
// keep it as an interface so the host can swap in a quieter logger
// during tests.
type Logger interface {
	Info(msg string, args ...any)
	Warn(msg string, args ...any)
	Error(msg string, args ...any)
}

// NewInstaller wires up the installer with everything it needs.
// trustKeys are the pinned ed25519 public keys used to verify the
// tarball signature. Pass nil to skip signature verification.
//
// An unsigned tarball — no trust keys configured, or a signature none
// of them verify — installs only when Source.AllowUnsigned carries the
// operator's consent. It is then reported through the progress stream,
// logged, flagged on the audit entry, and stored on the install record. Set
// MCHARBOR_PLUGIN_REQUIRE_SIGNATURE=true to make an unsigned tarball
// a hard failure again.
func NewInstaller(
	dataDir string,
	db *sql.DB,
	svc *Service,
	enc *encryption.Service,
	auditLog *audit.Logger,
	contrib Contributor,
	logger Logger,
	trustKeys []ed25519.PublicKey,
) *DefaultInstaller {
	return &DefaultInstaller{
		dataDir:          dataDir,
		db:               db,
		svc:              svc,
		enc:              enc,
		audit:            auditLog,
		contrib:          contrib,
		logger:           logger,
		trustKeys:        trustKeys,
		platformOK:       runtime.GOOS == "linux",
		requireSignature: RequireSignatureFromEnv(),
	}
}

// --- Install -----------------------------------------------------

// Install verifies the tarball, extracts it into
// <dataDir>/plugins/<slug>@<version>/, runs migrations, and
// persists the appstore_plugins row. It does NOT call Init() —
// Init() runs on the next host boot (or on Enable).
//
// progress is optional; nil falls back to a no-op callback.
func (d *DefaultInstaller) Install(ctx context.Context, src Source, progress ProgressFunc) (*Installed, error) {
	return d.install(ctx, src, progress, 0)
}

// install is Install with a recursion depth, so a dependency's own
// dependencies cannot loop. Depth zero is an operator asking for a plugin;
// anything deeper is the installer pulling one in.
func (d *DefaultInstaller) install(
	ctx context.Context,
	src Source,
	progress ProgressFunc,
	depth int,
) (*Installed, error) {
	if progress == nil {
		progress = noProgress
	}
	st, err := d.stage(ctx, src, progress)
	if err != nil {
		return nil, err
	}
	return d.installStaged(ctx, st, progress, depth)
}

// stage runs every check that needs no change to the host: platform, target
// release, download, signature and consent, extraction and manifest validation.
// It leaves the extracted tree in a staging directory the caller owns.
func (d *DefaultInstaller) stage(ctx context.Context, src Source, progress ProgressFunc) (*Staged, error) {
	if !d.platformOK {
		return nil, ErrPlatformUnsupported
	}
	if err := rejectIncompatible(src.BuiltFor); err != nil {
		return nil, err
	}

	// 1. Resolve tarball bytes (download or accept upload).
	progress("download", 5, "resolving tarball")
	tarball, err := d.resolveTarball(ctx, src)
	if err != nil {
		return nil, fmt.Errorf("resolving tarball: %w", err)
	}

	// 2. Check the signature. Unsigned is allowed — and said out loud,
	// here and on every surface downstream — unless the operator asked
	// for the strict policy.
	progress("verify", 15, "verifying signature")
	signed, err := d.verifyTarball(ctx, src, tarball)
	if err != nil {
		return nil, fmt.Errorf("verifying signature: %w", err)
	}
	if !signed {
		// Consent is checked here, after verification, because only the
		// server knows whether a trust key verifies the bytes it fetched.
		if !src.AllowUnsigned {
			if d.logger != nil {
				d.logger.Warn("refusing unsigned plugin without confirmation", "source", src.URL)
			}
			return nil, ErrUnsignedNotConfirmed
		}
		progress("verify", 20, "plugin is not signed by a trusted key — installing at your own risk")
		if d.logger != nil {
			d.logger.Warn("installing unsigned plugin", "source", src.URL)
		}
	}

	// 3. Extract into staging, parse manifest.
	progress("extract", 30, "extracting tarball")
	installDir, manifest, err := d.extractAndReadManifest(tarball)
	if err != nil {
		// A required field or an unknown scope token is a manifest the host
		// refuses, and callers map that one sentinel to its i18n code.
		if !errors.Is(err, ErrManifestInvalid) && (errors.Is(err, plugin.ErrScopeViolation) || isManifestError(err)) {
			return nil, fmt.Errorf("%w: %w", ErrManifestInvalid, err)
		}
		return nil, err
	}
	// 3b. JSON-schema validation (architecture §5). Strict mode is the
	// default; the MCHARBOR_PLUGIN_RELAXED_SCHEMA env var softens to
	// "unknown scope tokens allowed" for legacy plugins.
	if err := plugin.ValidateManifestSchema(manifest); err != nil {
		d.removeAll(installDir)
		return nil, fmt.Errorf("%w: %w", ErrManifestInvalid, err)
	}
	if err := pluginslug.Check(manifest.Slug); err != nil {
		d.removeAll(installDir)
		return nil, fmt.Errorf("%w: %w", ErrManifestInvalid, err)
	}
	src.Tarball = tarball
	return &Staged{
		Source:   src,
		Dir:      installDir,
		Slug:     manifest.Slug,
		Version:  manifest.Version,
		Signed:   signed,
		manifest: manifest,
		tarball:  tarball,
	}, nil
}

// isManifestError reports whether err carries a *ManifestError.
func isManifestError(err error) bool {
	var me *ManifestError
	return errors.As(err, &me)
}

// installStaged places a staged artifact and records it. It owns st.Dir from
// here on: every failure removes whatever it placed.
func (d *DefaultInstaller) installStaged(
	ctx context.Context,
	st *Staged,
	progress ProgressFunc,
	depth int,
) (*Installed, error) {
	if progress == nil {
		progress = noProgress
	}
	src, installDir, manifest, signed, tarball := st.Source, st.Dir, st.manifest, st.Signed, st.tarball
	slug := manifest.Slug
	version := manifest.Version
	pluginsDir := filepath.Join(d.dataDir, "plugins")
	if err := os.MkdirAll(pluginsDir, 0o755); err != nil {
		d.removeAll(installDir)
		return nil, fmt.Errorf("creating plugins dir: %w", err)
	}
	target := filepath.Join(pluginsDir, slug+"@"+version)

	// Refuse if already installed.
	existing, err := d.svc.LookupVersion(ctx, slug)
	if err != nil {
		d.removeAll(installDir)
		return nil, err
	}
	if existing != "" {
		d.removeAll(installDir)
		return nil, fmt.Errorf("%w: %s@%s", ErrAlreadyInstalled, slug, existing)
	}

	// Move staging into final location.
	if err := os.Rename(installDir, target); err != nil {
		d.removeAll(installDir)
		return nil, fmt.Errorf("placing install: %w", err)
	}

	// 4. Run plugin migrations (best-effort; failures abort the install).
	progress("migrate", 50, "running migrations")
	if err := d.runMigrations(ctx, target, slug); err != nil {
		d.removeAll(target)
		return nil, fmt.Errorf("running migrations: %w", err)
	}

	// 5. Persist the appstore_plugins row (enabled=1 — plugin is
	// immediately active on next boot or runtime rescan; the
	// runtime loader picks it up unconditionally).
	progress("persist", 75, "persisting install record")
	if _, err := d.svc.InsertInstalledRowWithBuild(ctx, slug, version, src.Build,
		manifest.Name, manifest.Author, manifest.Desc, signed); err != nil {
		d.removeAll(target)
		return nil, err
	}
	// The artifact cleared rejectIncompatible, so it was built for this
	// release; recording that is what lets the reconciler spot a later
	// upgrade instead of reloading a .so the new host cannot open.
	if err := d.svc.SetRepairState(ctx, slug, appversion.Current(), ""); err != nil {
		d.removeAll(target)
		return nil, err
	}
	if err := d.recordManifestContributions(ctx, slug, manifest); err != nil {
		d.removeAll(target)
		return nil, err
	}
	// Apply the manifest's preset grants so viewer / editor / operator
	// can reach the plugin the moment it is installed, the way the
	// host's own features are. Add-only: an operator's later edits to
	// a preset are theirs, and uninstall strips every plugin key back
	// out. A failure here is a broken install — the plugin would be
	// reachable by admins only — so it aborts like any other step.
	if err := d.svc.GrantKeysToRoles(ctx, presetGrants(manifest.AddsPerms)); err != nil {
		d.removeAll(target)
		return nil, fmt.Errorf("granting preset permissions: %w", err)
	}
	d.storePackage(ctx, src, tarball, slug, version)

	// 6. Audit.
	if d.audit != nil {
		d.emitAudit(ctx, slug, "plugin.install", map[string]any{
			"version":  version,
			"author":   manifest.Author,
			"signed":   signed,
			"unsigned": !signed,
		})
	}

	// 7. Pull in whatever this plugin requires. After the install record
	// exists, so a dependency that names this plugin back finds it installed
	// and stops rather than recursing into it.
	d.installDependencies(ctx, manifest, progress, depth, src.AllowUnsigned)

	// After the dependencies, so a plugin whose catalog lives in the
	// foundation it pulled in publishes both in one pass.
	d.syncCatalogs(ctx, slug)

	progress("done", 100, fmt.Sprintf("installed %s@%s", slug, version))
	return &Installed{
		Slug:        slug,
		Version:     version,
		Path:        target,
		NeedsEnable: false,
	}, nil
}

// recordManifestContributions clears whatever a previous version of this
// plugin registered and re-records what the manifest declares now.
//
// The clear runs whenever the manifest declares any recordable contribution,
// not only a theme: an early return keyed on themes alone left a
// catalog-contributing plugin holding stale rows from its previous version.
func (d *DefaultInstaller) recordManifestContributions(ctx context.Context, slug string, manifest *plugin.Manifest) error {
	if d.contrib == nil || manifest == nil {
		return nil
	}
	if len(manifest.Contribs.Themes) == 0 && len(manifest.Contribs.Catalogs) == 0 {
		return nil
	}
	if err := d.contrib.ClearAll(ctx, slug); err != nil {
		return fmt.Errorf("clearing prior contributions: %w", err)
	}
	for _, c := range manifest.Contribs.Catalogs {
		if err := d.contrib.RecordCatalog(ctx, slug, c.ID, c.Kind, c.Name, c.URL); err != nil {
			return fmt.Errorf("recording plugin catalog %s: %w", c.ID, err)
		}
	}
	for _, theme := range manifest.Contribs.Themes {
		defaultVariant := theme.DefaultVariant
		if defaultVariant == "" {
			defaultVariant = "dark"
		}
		if err := d.contrib.RecordTheme(ctx, slug, theme.ID, theme.Name, theme.Description, theme.CSSPath, theme.SwatchHue, defaultVariant, theme.HasDark, theme.HasLight); err != nil {
			return fmt.Errorf("recording plugin theme %s: %w", theme.ID, err)
		}
	}
	return nil
}

// resolveTarball fetches the bytes from src.URL or returns src.Tarball.
//
// Supported src.URL schemes:
//   - file://  — reads the file from the host filesystem. Used by
//     the dev catalog at /app/data/plugin-catalog/files/<tarball>.
//     Path is taken from URL.Path (filepath.FromSlash applied).
//   - https:// — standard download via net/http. Redirects to a
//     non-loopback host are refused; redirects to loopback or HTTPS
//     are followed once.
//   - http://  — allowed only for loopback hosts (127.0.0.1,
//     ::1, localhost) so dev tooling on the same machine works
//     without TLS while production HTTPS still requires HTTPS.
//
// Any other scheme returns an error so the caller (and ultimately
// the admin UI) sees an actionable message rather than a 500.
func (d *DefaultInstaller) resolveTarball(ctx context.Context, src Source) ([]byte, error) {
	if len(src.Tarball) > 0 {
		return src.Tarball, nil
	}
	if src.URL == "" {
		return nil, errors.New("source has neither URL nor Tarball")
	}
	u, err := url.Parse(src.URL)
	if err != nil {
		return nil, fmt.Errorf("parsing source url: %w", err)
	}
	switch strings.ToLower(u.Scheme) {
	case "file":
		return os.ReadFile(filepath.FromSlash(u.Path))
	case "https":
		return downloadHTTP(ctx, u)
	case "http":
		host := strings.ToLower(u.Hostname())
		if host != "127.0.0.1" && host != "localhost" && host != "::1" {
			return nil, fmt.Errorf("http:// source allowed only for loopback hosts, got %q", host)
		}
		return downloadHTTP(ctx, u)
	default:
		return nil, fmt.Errorf("unsupported source scheme: %s", u.Scheme)
	}
}

// downloadHTTP performs the GET. Caller (resolveTarball) has
// already vetted the scheme; this helper focuses on transport
// safety: a bounded timeout, redirect-loop protection, a body
// size cap, and HTTPS-only redirects except back to loopback.
func downloadHTTP(ctx context.Context, u *url.URL) ([]byte, error) {
	// Loopback stays open for the http mirror resolveTarball allows; the egress
	// guard still refuses link-local and the metadata service on every hop.
	client := egress.NewClient(egress.Options{
		Timeout: httpDownloadTimeout,
		Policy:  egress.Policy{AllowLoopback: true},
	})

	// Refuse redirects to non-HTTPS / non-loopback destinations.
	client.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		if len(via) >= 5 {
			return errors.New("too many redirects")
		}
		if req.URL.Scheme == "https" {
			return nil
		}
		host := strings.ToLower(req.URL.Hostname())
		if req.URL.Scheme == "http" && (host == "127.0.0.1" || host == "localhost" || host == "::1") {
			return nil
		}
		return fmt.Errorf("redirect to %s blocked (HTTPS or loopback required)", req.URL.String())
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("download %s: %w", u.String(), err)
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("download %s: %w", u.String(), err)
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			slog.Debug("plugin install: closing download body failed", "error", err)
		}
	}()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("download %s: status %d", u.String(), resp.StatusCode)
	}

	limited := io.LimitReader(resp.Body, httpDownloadMaxBytes+1)
	var buf bytes.Buffer
	if _, err := io.Copy(&buf, limited); err != nil {
		return nil, fmt.Errorf("read body from %s: %w", u.String(), err)
	}
	if buf.Len() > httpDownloadMaxBytes {
		return nil, fmt.Errorf("download %s exceeds %d byte cap", u.String(), httpDownloadMaxBytes)
	}
	return buf.Bytes(), nil
}

// RequireSignatureFromEnv reads MCHARBOR_PLUGIN_REQUIRE_SIGNATURE. Anything
// but a truthy value keeps the permissive default, and the boot posture check
// reads it through here so the warning and the installer cannot disagree.
func RequireSignatureFromEnv() bool {
	v := strings.ToLower(strings.TrimSpace(os.Getenv("MCHARBOR_PLUGIN_REQUIRE_SIGNATURE")))
	return v == "1" || v == "true" || v == "yes" || v == "on"
}

// verifyTarball reports whether the tarball carries a valid ed25519
// signature from one of the configured trust keys.
//
// Three signature carriers are accepted, in order of preference:
//
//  1. src.Signature — base64, taken from the catalog entry. This is
//     what first-party plugins ship: the signature travels with the
//     entry that names the archive, so it is available before a byte
//     is downloaded.
//  2. A <tarball>.sig sidecar next to the archive, same base64 form.
//     This is what src/plugins/scripts/sign-plugin.py writes and what
//     a third party publishing their own tarball would serve.
//  3. The legacy shape: the last 64 bytes of the archive are the
//     signature over everything before them. Kept so tarballs packaged
//     against the earlier scheme keep verifying.
//
// It returns an error only under the strict policy
// (MCHARBOR_PLUGIN_REQUIRE_SIGNATURE). Otherwise an unsigned or
// untrusted tarball is (false, nil): the caller requires consent, warns,
// records the state, and continues. That is the whole of the third-party plugin
// story — a host that can only run what its vendor signed is not
// self-hosted, and an operator who fetched a tarball deliberately is
// making a call the host should surface rather than override.
func (d *DefaultInstaller) verifyTarball(ctx context.Context, src Source, tarball []byte) (bool, error) {
	if len(d.trustKeys) == 0 {
		if d.requireSignature {
			if d.logger != nil {
				d.logger.Error("MCHARBOR_PLUGIN_REQUIRE_SIGNATURE is set but no trust keys are configured")
			}
			return false, ErrSignatureInvalid
		}
		if d.logger != nil {
			d.logger.Warn("installing plugin without signature verification (no trust keys)")
		}
		return false, nil
	}

	if sig := decodeSignature(src.Signature); sig != nil && d.verifySignature(tarball, sig) {
		return true, nil
	}
	if sig := decodeSignature(d.fetchSidecar(ctx, src.URL)); sig != nil && d.verifySignature(tarball, sig) {
		return true, nil
	}
	// Legacy: signature appended to the archive rather than carried
	// beside it.
	if len(tarball) >= ed25519.SignatureSize {
		payload := tarball[:len(tarball)-ed25519.SignatureSize]
		if d.verifySignature(payload, tarball[len(tarball)-ed25519.SignatureSize:]) {
			return true, nil
		}
	}

	if d.requireSignature {
		return false, ErrSignatureInvalid
	}
	return false, nil
}

// ProbeSignature resolves src and reports whether a trust key verifies it,
// without installing anything. The returned Source carries the fetched bytes,
// so the install that follows verifies exactly what was probed.
func (d *DefaultInstaller) ProbeSignature(ctx context.Context, src Source) (Source, bool, error) {
	tarball, err := d.resolveTarball(ctx, src)
	if err != nil {
		return src, false, fmt.Errorf("resolving tarball: %w", err)
	}
	signed, err := d.verifyTarball(ctx, src, tarball)
	if err != nil {
		return src, false, fmt.Errorf("verifying signature: %w", err)
	}
	src.Tarball = tarball
	return src, signed, nil
}

// verifySignature checks payload against every configured trust key.
func (d *DefaultInstaller) verifySignature(payload, sig []byte) bool {
	for _, pub := range d.trustKeys {
		if ed25519.Verify(pub, payload, sig) {
			return true
		}
	}
	return false
}

// decodeSignature turns the base64 form used by the catalog and the
// .sig sidecar into raw bytes. Anything that is not a well-formed
// 64-byte signature returns nil, so a malformed value falls through to
// the next carrier instead of failing the install outright.
func decodeSignature(encoded string) []byte {
	encoded = strings.TrimSpace(encoded)
	if encoded == "" {
		return nil
	}
	raw, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil || len(raw) != ed25519.SignatureSize {
		return nil
	}
	return raw
}

// fetchSidecar reads <url>.sig, returning "" when there is none. A
// missing sidecar is the normal case for an unsigned plugin, so every
// failure here is quiet: the caller treats it as "no signature offered"
// and the install continues down the unsigned path.
func (d *DefaultInstaller) fetchSidecar(ctx context.Context, url string) string {
	if url == "" {
		return ""
	}
	b, err := d.resolveTarball(ctx, Source{URL: url + ".sig"})
	if err != nil || len(b) > 4096 {
		return ""
	}
	return string(b)
}

// extractAndReadManifest untars into a staging dir and returns the
// parsed manifest. Validates required fields.
func (d *DefaultInstaller) extractAndReadManifest(tarball []byte) (string, *plugin.Manifest, error) {
	staging, err := os.MkdirTemp(d.dataDir, "plugin-staging-*")
	if err != nil {
		return "", nil, fmt.Errorf("creating staging dir: %w", err)
	}
	if err := untar(tarball, staging); err != nil {
		d.removeAll(staging)
		return "", nil, fmt.Errorf("extracting tarball: %w", err)
	}

	// Read manifest from staging/plugin.yaml.
	mb, err := os.ReadFile(filepath.Join(staging, "plugin.yaml"))
	if err != nil {
		d.removeAll(staging)
		return "", nil, fmt.Errorf("%w: plugin.yaml missing", ErrManifestInvalid)
	}
	var m plugin.Manifest
	if err := yaml.Unmarshal(mb, &m); err != nil {
		d.removeAll(staging)
		return "", nil, fmt.Errorf("%w: %w", ErrManifestInvalid, err)
	}
	if err := validateManifest(&m); err != nil {
		d.removeAll(staging)
		return "", nil, err
	}
	return staging, &m, nil
}

// validateManifest enforces required fields per architecture §5.
func validateManifest(m *plugin.Manifest) error {
	if m.Slug == "" {
		return &ManifestError{Field: "slug", Reason: "required"}
	}
	if m.Version == "" {
		return &ManifestError{Field: "version", Reason: "required"}
	}
	if m.Name == "" {
		return &ManifestError{Field: "name", Reason: "required"}
	}
	if m.Mcharbor == "" {
		return &ManifestError{Field: "mcharbor", Reason: "required (min host version)"}
	}
	for _, scope := range m.Perms {
		if !isKnownScope(scope) {
			return fmt.Errorf("%w: scope %q not recognized", plugin.ErrScopeViolation, scope)
		}
	}
	return nil
}

// rejectIncompatible refuses an artifact compiled against a different core
// release, so the operator sees the version pair instead of the opaque
// "different version of package" error plugin.Open would raise at load.
func rejectIncompatible(builtFor string) error {
	if builtFor == "" || builtFor == appversion.Current() {
		return nil
	}
	return fmt.Errorf("%w: built for %s, host is %s",
		ErrBuiltForMismatch, builtFor, appversion.Current())
}

// isKnownScope defers to the canonical token list in
// core/plugin/host rather than keeping a second copy here.
//
// The hand-copied switch this replaced had drifted badly. It was
// missing menu.sidebar.add, i18n.namespace.add,
// rbac.permission.register, db.migrations.run, dashboard.widget.add,
// docker.socket.read, identity.issue_session and
// scanner.adapter.register, and it compared exactly instead of
// stripping the colon-separated argument that the schema validator
// explicitly supports. Because validateManifest runs before
// ValidateManifestSchema, that stale list rejected every addon,
// scanner, logging and LDAP plugin with a scope violation naming a
// token the real list has always accepted.
func isKnownScope(s string) bool {
	return plugin.KnownScopeToken(s)
}

// runMigrations executes every <install>/migrations/*.sql file in
// alphabetical order, each in its own transaction with a 5-minute
// timeout. Plugin migrations must use the plugin_<slug>_ table
// prefix; the loader's pluginDB adapter enforces the same at
// runtime.
//
// A file already recorded in plugin_migrations for this slug is skipped. Only
// the first install of a slug runs the whole set: an uninstall that keeps the
// tables records them as applied, so the reinstall behind a repair or an
// update runs what is genuinely new and nothing else. Re-running an old one is
// not harmless — the ALTER TABLE a column migration is made of fails outright
// on the second pass.
func (d *DefaultInstaller) runMigrations(ctx context.Context, installDir, slug string) error {
	migDir := filepath.Join(installDir, "migrations")
	entries, err := os.ReadDir(migDir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return err
	}
	applied, err := d.appliedMigrations(ctx, slug)
	if err != nil {
		return err
	}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".sql") {
			continue
		}
		if applied[e.Name()] {
			continue
		}
		b, err := os.ReadFile(filepath.Join(migDir, e.Name()))
		if err != nil {
			return err
		}
		mctx, cancel := context.WithTimeout(ctx, 5*time.Minute)
		err = runMigrationTx(mctx, d.db, string(b))
		cancel()
		if err != nil {
			return fmt.Errorf("applying %s: %w", e.Name(), err)
		}
		if err := d.recordMigration(ctx, slug, e.Name()); err != nil {
			return fmt.Errorf("recording %s: %w", e.Name(), err)
		}
	}
	return nil
}

// appliedMigrations reads the migration files already applied for one slug.
func (d *DefaultInstaller) appliedMigrations(ctx context.Context, slug string) (map[string]bool, error) {
	rows, err := d.db.QueryContext(ctx,
		`SELECT filename FROM plugin_migrations WHERE origin = ? LIMIT 1000`, slug)
	if err != nil {
		return nil, fmt.Errorf("reading the migration ledger: %w", err)
	}
	defer func() {
		if err := rows.Close(); err != nil {
			slog.Warn("plugin install: closing migration ledger rows failed", "error", err)
		}
	}()
	applied := make(map[string]bool)
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, err
		}
		applied[name] = true
	}
	return applied, rows.Err()
}

// recordMigration notes one applied migration file, tolerating a row that is
// already there so a backfill and a fresh apply cannot collide.
func (d *DefaultInstaller) recordMigration(ctx context.Context, slug, filename string) error {
	now := time.Now().UTC().Format(time.RFC3339)
	_, err := d.db.ExecContext(ctx,
		`INSERT OR IGNORE INTO plugin_migrations (id, origin, filename, applied_at, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		uuid.NewString(), slug, filename, now, now, now)
	return err
}

// recordMigrationsApplied marks every migration file still on disk as applied,
// and reports whether the install carried any to record.
//
// It runs during a table-keeping uninstall, while the files are still there:
// the tables it did not drop already carry what those files built, and step 7
// is about to delete the directory that says which ones they were.
//
// False means the installed tarball predates migrations being packaged, so
// there is no way to know which of the incoming set the existing tables already
// have. The caller drops them rather than reinstall onto a schema it cannot
// reason about — a failed install would strand the data anyway.
func (d *DefaultInstaller) recordMigrationsApplied(ctx context.Context, slug, migDir string) (bool, error) {
	entries, err := os.ReadDir(migDir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return false, nil
		}
		return false, err
	}
	recorded := false
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".sql") {
			continue
		}
		if err := d.recordMigration(ctx, slug, e.Name()); err != nil {
			return false, err
		}
		recorded = true
	}
	return recorded, nil
}

// clearMigrationLedger forgets every migration recorded for one slug, so a
// later install of it starts on an empty schema and runs the whole set again.
func (d *DefaultInstaller) clearMigrationLedger(ctx context.Context, slug string) error {
	_, err := d.db.ExecContext(ctx, `DELETE FROM plugin_migrations WHERE origin = ?`, slug)
	return err
}

func runMigrationTx(ctx context.Context, db *sql.DB, sqlText string) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() {
		if err := tx.Rollback(); err != nil && !errors.Is(err, sql.ErrTxDone) {
			slog.Warn("plugin install: rolling back migration failed", "error", err)
		}
	}()
	if _, err := tx.ExecContext(ctx, sqlText); err != nil {
		return err
	}
	return tx.Commit()
}

// --- Uninstall ----------------------------------------------------

// presetRoleIDs maps the preset names a manifest may use in
// `adds_permissions[].presets` to the system role ids that carry
// them. Admin holds a wildcard and is deliberately absent: granting
// it a concrete key would only make the role editor noisier.
var presetRoleIDs = map[string]string{
	"viewer":   "role_viewer",
	"editor":   "role_editor",
	"operator": "role_operator",
}

// presetGrants turns a manifest's permission declarations into the
// role id → keys map GrantKeysToRoles consumes. Unknown preset names
// are ignored rather than failing the install; a typo in one plugin's
// manifest should cost that key its preset grant, not the install.
func presetGrants(decls []plugin.PermissionDecl) map[string][]string {
	out := make(map[string][]string)
	for _, decl := range decls {
		if decl.Key == "" {
			continue
		}
		for _, preset := range decl.Presets {
			roleID, ok := presetRoleIDs[strings.ToLower(strings.TrimSpace(preset))]
			if !ok {
				continue
			}
			out[roleID] = append(out[roleID], decl.Key)
		}
	}
	return out
}

// Uninstall runs the 7-step gate chain from architecture §7.6.
// Each step logs and progresses; if any step fails the function
// returns an error and stops.
//
// Role grants do not block an uninstall. Step 5 strips every key the
// plugin declared from every role that carries it — system presets
// and custom roles alike — and the names of those roles come back in
// the result so the admin is told what changed. Refusing instead
// used to leave upgraded installs unable to uninstall a plugin at all,
// because the presets carry its keys by design.
func (d *DefaultInstaller) Uninstall(ctx context.Context, slug string, opts UninstallOptions, progress ProgressFunc) (*Uninstalled, error) {
	return d.uninstall(ctx, slug, opts, progress, 0)
}

// uninstall is Uninstall with a recursion depth, matching install.
func (d *DefaultInstaller) uninstall(
	ctx context.Context,
	slug string,
	opts UninstallOptions,
	progress ProgressFunc,
	depth int,
) (*Uninstalled, error) {
	if !d.platformOK {
		return nil, ErrPlatformUnsupported
	}
	if progress == nil {
		progress = noProgress
	}

	version, err := d.svc.LookupVersion(ctx, slug)
	if err != nil {
		return nil, err
	}
	if version == "" {
		return nil, ErrPluginNotFound
	}

	// Step 1 — Shutdown (host-side stub: the host already keeps
	// the registry of loaded plugins; on Uninstall we ask it to
	// Shutdown via Contributor.ClearAll so any in-flight request
	// gets a graceful path. The actual *goplugin.Plugin lifecycle
	// is closed by host process restart in Phase A.)
	progress("step1", 10, "shutdown plugin")
	if d.contrib != nil {
		if err := d.contrib.ClearAll(ctx, slug); err != nil {
			return nil, fmt.Errorf("step 1 (shutdown): %w", err)
		}
	}

	// Step 2 — Unregister SCIM schemas. Phase A has no SCIM
	// plugin surface; the loader's accessRegistry.UnregisterSCIM
	// is the future integration point. No-op today.

	// Step 3 — Remove ABAC rules by origin.
	progress("step3", 25, "removing ABAC rules")
	if err := d.svc.DropABACRules(ctx, slug); err != nil {
		return nil, fmt.Errorf("step 3 (drop ABAC): %w", err)
	}

	// Step 4 — Drop routes from plugin_routes.
	progress("step4", 40, "dropping routes")
	if err := d.svc.DropRoutes(ctx, slug); err != nil {
		return nil, fmt.Errorf("step 4 (drop routes): %w", err)
	}
	if err := d.svc.DropMenu(ctx, slug); err != nil {
		return nil, fmt.Errorf("step 4 (drop menu): %w", err)
	}
	if err := d.svc.DropWidgets(ctx, slug); err != nil {
		return nil, fmt.Errorf("step 4 (drop widgets): %w", err)
	}
	if err := d.svc.DropNodes(ctx, slug); err != nil {
		return nil, fmt.Errorf("step 4 (drop nodes): %w", err)
	}

	// Step 5 — Strip the plugin's keys from every role that grants
	// them, then drop the permission rows. Stripping first means a
	// failure here leaves roles naming keys the catalog still defines,
	// which the role editor can show, rather than the reverse.
	progress("step5", 55, "removing permissions")
	keys, err := d.svc.PermissionsForPlugin(ctx, slug)
	if err != nil {
		return nil, fmt.Errorf("step 5 (load permissions): %w", err)
	}
	stripped, err := d.svc.StripKeysFromRoles(ctx, keys)
	if err != nil {
		return nil, fmt.Errorf("step 5 (strip role grants): %w", err)
	}
	if len(stripped) > 0 && d.logger != nil {
		d.logger.Warn("plugin uninstall: stripped permission grants from roles",
			"slug", slug, "roles", strings.Join(stripped, ", "))
	}
	if err := d.svc.DropPermissions(ctx, slug); err != nil {
		return nil, fmt.Errorf("step 5 (drop permissions): %w", err)
	}

	// Step 6 — Drop the plugin's tables.
	//
	// The prefix comes from the installed manifest when it declares one.
	// A plugin that was renamed after its migrations shipped keeps the
	// table names those migrations wrote, and `plugin_<new-slug>_` would
	// match none of them; `table_prefix` is how such a plugin says so.
	// Everything else falls back to the derived default, which is also
	// what an unreadable manifest gets — a best-effort sweep beats
	// aborting the uninstall over a file step 7 is about to delete.
	progress("step6", 75, "dropping plugin tables")
	prefix := TablePrefixFor(slug)
	// Read once and keep it: step 7 deletes this directory, and the prune
	// below needs the manifest's `requires` list after that has happened.
	var removed *plugin.Manifest
	if mb, err := os.ReadFile(filepath.Join(d.dataDir, "plugins", slug+"@"+version, "plugin.yaml")); err == nil {
		var m plugin.Manifest
		if err := yaml.Unmarshal(mb, &m); err == nil {
			removed = &m
			if m.TablePrefix != "" {
				prefix = m.TablePrefix
			}
		}
	}
	// A repair or an update takes the plugin out only to put the same one back,
	// so the rows in its tables are the operator's work and not this operation's
	// to destroy. The migrations that built them are recorded instead, because
	// the reinstall must not run them a second time.
	kept := false
	if opts.KeepTables {
		recorded, err := d.recordMigrationsApplied(ctx, slug,
			filepath.Join(d.dataDir, "plugins", slug+"@"+version, "migrations"))
		if err != nil {
			return nil, fmt.Errorf("step 6 (recording migrations): %w", err)
		}
		kept = recorded
		if !kept && d.logger != nil {
			d.logger.Warn("plugin install carries no migrations to record; "+
				"its tables are dropped rather than reinstalled onto blindly",
				"slug", slug, "version", version)
		}
	}
	if !kept {
		if err := d.svc.DropTables(ctx, prefix); err != nil {
			return nil, fmt.Errorf("step 6 (drop tables): %w", err)
		}
		if err := d.clearMigrationLedger(ctx, slug); err != nil {
			return nil, fmt.Errorf("step 6 (clearing the migration ledger): %w", err)
		}
	}

	// Step 7 — Delete files.
	progress("step7", 90, "deleting files")
	target := filepath.Join(d.dataDir, "plugins", slug+"@"+version)
	if opts.SetAside {
		// An upgrade keeps the previous version whole until its replacement is
		// in, so a failed install can be rolled back rather than leave nothing.
		if err := d.setAside(target, slug, version); err != nil {
			return nil, fmt.Errorf("step 7 (set aside files): %w", err)
		}
	} else {
		if err := os.RemoveAll(target); err != nil {
			return nil, fmt.Errorf("step 7 (delete files): %w", err)
		}
		d.forgetPackage(ctx, slug, version)
	}

	// Final: remove the appstore_plugins row.
	if err := d.svc.DeleteRow(ctx, slug); err != nil {
		return nil, fmt.Errorf("removing install record: %w", err)
	}

	if d.audit != nil {
		d.emitAudit(ctx, slug, "plugin.uninstall", map[string]any{
			"version":        version,
			"roles_stripped": stripped,
			"purge_data":     opts.PurgeData,
		})
	}

	// Remove any dependency this plugin was the last to need. After the row is
	// gone, so this plugin does not count itself as its own dependent.
	//
	// opts travels with it: a dependency the host installed itself, being
	// removed as part of a purging uninstall, purges too. Without that,
	// uninstalling addon-kvm with the box ticked would prune addon-compute and
	// leave the VM image catalog behind — the dependency owns it, not the
	// plugin the operator actually clicked.
	//
	// An upgrade puts this plugin straight back, so nothing it requires has
	// lost its last dependent and a prune would only remove what it reinstalls.
	if !opts.SetAside {
		d.pruneDependencies(ctx, removed, opts, progress, depth)
	}

	// Step 8 — the opt-in purge, last because it is the only step whose
	// failure does not mean the uninstall failed. The plugin is already gone
	// by here; a purge that could not finish is logged and leaves rows behind
	// rather than reporting an error that invites a retry finding no plugin.
	var configPurged, catalogsPurged int
	if opts.PurgeData {
		progress("step8", 97, "clearing plugin data")
		configPurged, catalogsPurged = d.purgeData(ctx, slug, removed)
	}

	// After the prune, so a foundation removed along with its last dependent
	// has its own catalog reaped in the same pass. ClearAll in step 1 already
	// dropped the declarations; this reaps the settings rows projected from
	// them.
	d.syncCatalogs(ctx, slug)

	progress("done", 100, fmt.Sprintf("uninstalled %s@%s", slug, version))
	return &Uninstalled{
		Slug:           slug,
		Version:        version,
		RolesStripped:  stripped,
		ConfigPurged:   configPurged,
		CatalogsPurged: catalogsPurged,
	}, nil
}

// purgeData removes what an ordinary uninstall keeps: the plugin's config rows,
// encrypted secrets included, and the catalog sources left behind for kinds it
// declared. Returns what each step removed, for the audit line and the caller.
//
// The kinds come from the removed plugin's manifest; a manifest that could not
// be read leaves catalogs alone rather than guessing at which kinds were its.
func (d *DefaultInstaller) purgeData(ctx context.Context, slug string, removed *plugin.Manifest) (int, int) {
	configPurged, err := d.svc.DropConfig(ctx, slug)
	if err != nil && d.logger != nil {
		d.logger.Warn("plugin uninstall: config not purged", "slug", slug, "error", err)
	}

	if removed == nil || d.catalogPurge == nil {
		return configPurged, 0
	}
	kinds := make([]string, 0, len(removed.Contribs.Catalogs))
	for _, c := range removed.Contribs.Catalogs {
		kinds = append(kinds, c.Kind)
	}
	if len(kinds) == 0 {
		return configPurged, 0
	}
	catalogsPurged, err := d.catalogPurge(ctx, kinds)
	if err != nil && d.logger != nil {
		d.logger.Warn("plugin uninstall: catalogs not purged", "slug", slug, "error", err)
	}
	return configPurged, catalogsPurged
}

// --- Configure ---------------------------------------------------

// --- helpers -----------------------------------------------------

func noProgress(string, int, string) {}

// removeAll deletes path and logs a failure, because every caller is already on an error path with a better error to return.
func (d *DefaultInstaller) removeAll(path string) {
	if err := os.RemoveAll(path); err != nil {
		d.warn("plugin install: removing leftover directory "+path, "", err)
	}
}

// emitAudit records a plugin audit event when an audit logger is wired in, logging rather than failing the install if the write is refused.
func (d *DefaultInstaller) emitAudit(ctx context.Context, slug, event string, fields map[string]any) {
	if d.audit == nil {
		return
	}
	if err := d.audit.EmitPluginSeverityContext(ctx, slug, event, "", fields); err != nil {
		d.warn("plugin audit: emitting "+event, slug, err)
	}
}

// untar reads a gzipped tarball and writes its entries into dst.
// Paths are sanitized: entries containing ".." are rejected.
func untar(data []byte, dst string) error {
	gz, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("gzip: %w", err)
	}
	defer func() {
		if err := gz.Close(); err != nil {
			slog.Debug("plugin install: closing gzip reader failed", "error", err)
		}
	}()
	tr := tar.NewReader(gz)
	for {
		hdr, err := tr.Next()
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return err
		}
		clean := filepath.Clean(hdr.Name)
		if strings.HasPrefix(clean, "..") || strings.Contains(clean, "/../") {
			return fmt.Errorf("refusing path %q", hdr.Name)
		}
		target := filepath.Join(dst, clean)
		switch hdr.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, 0o755); err != nil {
				return err
			}
		case tar.TypeReg:
			if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
				return err
			}
			f, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, os.FileMode(hdr.Mode)&0o777)
			if err != nil {
				return err
			}
			if _, err := io.Copy(f, tr); err != nil {
				return errors.Join(err, f.Close())
			}
			if err := f.Close(); err != nil {
				return err
			}
		default:
			// Skip symlinks, devices, etc.
		}
	}
}

// fingerprintTarball returns a sha256 prefix useful for diagnostic
// logs without leaking the entire payload. Returns "" if empty.
func fingerprintTarball(b []byte) string {
	if len(b) == 0 {
		return ""
	}
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:8])
}

// silence unused-import warnings for diagnostic helpers that may be
// wired into error paths or future steps.
var _ = fingerprintTarball
