// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package agentreleases

import (
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/rs/xid"

	coreagent "github.com/therealmcsparrow/mcharbor/core/agent"
	"github.com/therealmcsparrow/mcharbor/core/cronspec"
	"github.com/therealmcsparrow/mcharbor/core/db"
)

const (
	settingsCategory = "agent_releases"
	settingsKey      = "agent_release_settings"
	targetKey        = "agent_release_target"
	targetBuildKey   = "agent_release_target_build"
	stateKey         = "agent_release_state"
	pinsKey          = "agent_release_pins"

	defaultCheckCron = "0 5 * * *"
	defaultKeep      = 3
	maxKeep          = 20

	smallFetchTimeout = 30 * time.Second
	assetFetchTimeout = 10 * time.Minute
	// pullBudget bounds a whole background pull: every binary plus every image
	// save, each of which has its own tighter bound.
	pullBudget = 60 * time.Minute
)

var imageRepositoryPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9._/:-]{0,254}$`)

// Store is the agent release store: one row per version and build in
// agent_releases, files under DATA_DIR/agent-releases/<version>_<build>/. Every
// method is safe for concurrent use.
type Store struct {
	db     *sql.DB
	logger *slog.Logger
	root   string
	client *http.Client
	keys   []ed25519.PublicKey
	images ImageStore
	inUse  func() []string
	now    func() time.Time
	bg     context.Context //nolint:containedctx // lifecycle root for background image pulls, outliving any one request

	imageMu sync.Mutex
	pullMu  sync.Mutex
	pulling map[string]bool
}

// Options configures a Store. Keys nil means the compiled-in pinned keys.
type Options struct {
	DB         *sql.DB
	Logger     *slog.Logger
	Root       string
	Client     *http.Client
	Keys       []ed25519.PublicKey
	Images     ImageStore
	InUse      func() []string
	Background context.Context //nolint:containedctx // lifecycle root handed in from main so background pulls stop at shutdown
}

// NewStore builds a Store and creates its directory. Failing to parse the pinned
// keys is returned, because a store that cannot verify must not start pulling.
func NewStore(opts Options) (*Store, error) {
	if opts.DB == nil || opts.Root == "" {
		return nil, errors.New("agent release store needs a database and a root directory")
	}
	keys := opts.Keys
	if keys == nil {
		pinned, err := PinnedKeys()
		if err != nil {
			return nil, err
		}
		keys = pinned
	}
	if err := os.MkdirAll(opts.Root, 0o755); err != nil {
		return nil, fmt.Errorf("creating %s: %w", opts.Root, err)
	}
	bg := opts.Background
	if bg == nil {
		bg = context.Background()
	}
	if err := ensurePerBuildIndex(bg, opts.DB); err != nil {
		return nil, err
	}
	s := &Store{
		db:      opts.DB,
		logger:  opts.Logger,
		root:    opts.Root,
		client:  opts.Client,
		keys:    keys,
		images:  opts.Images,
		inUse:   opts.InUse,
		now:     time.Now,
		bg:      bg,
		pulling: map[string]bool{},
	}
	if s.logger == nil {
		s.logger = slog.Default()
	}
	if s.client == nil {
		s.client = &http.Client{}
	}
	if s.inUse == nil {
		s.inUse = func() []string { return nil }
	}
	return s, nil
}

// perBuildSchema lets several builds of one version be stored side by side.
// Migration 118 made version alone unique; the swap lives here rather than in a
// new migration because any edit under core/db changes every plugin's ABI.
var perBuildSchema = []string{
	`DROP INDEX IF EXISTS idx_agent_releases_version`,
	`CREATE UNIQUE INDEX IF NOT EXISTS idx_agent_releases_version_build ON agent_releases(version, build)`,
}

func ensurePerBuildIndex(ctx context.Context, database *sql.DB) error {
	ctx, cancel := context.WithTimeout(ctx, smallFetchTimeout)
	defer cancel()
	for _, statement := range perBuildSchema {
		if _, err := database.ExecContext(ctx, statement); err != nil {
			return fmt.Errorf("agent release per-build index: %w", err)
		}
	}
	return nil
}

// Root is the directory releases are stored under.
func (s *Store) Root() string { return s.root }

// DefaultSettings is what a fresh install uses: check daily, notify only, keep three.
func DefaultSettings() Settings {
	return Settings{
		Enabled:         true,
		Mode:            ModeNotify,
		CheckCron:       defaultCheckCron,
		Keep:            defaultKeep,
		Sources:         DefaultSources(),
		ImageRepository: DefaultImageRepository,
		ImagePlatforms:  []string{"linux/amd64"},
	}
}

// NormalizeSettings validates and fills in defaults. The cron is parsed here so
// a schedule the loop would silently never fire is refused at save time.
func NormalizeSettings(in Settings) (Settings, error) {
	out := in
	switch out.Mode {
	case "":
		out.Mode = ModeNotify
	case ModeNotify, ModeDownload, ModeRollout:
	default:
		return in, settingsError("bad_mode")
	}
	out.CheckCron = strings.TrimSpace(out.CheckCron)
	if out.CheckCron == "" {
		out.CheckCron = defaultCheckCron
	}
	if !cronspec.Valid(out.CheckCron) {
		return in, settingsError("bad_cron")
	}
	if out.Keep == 0 {
		out.Keep = defaultKeep
	}
	if out.Keep < 1 || out.Keep > maxKeep {
		return in, settingsError("bad_keep")
	}
	if len(out.Sources) == 0 {
		out.Sources = DefaultSources()
	}
	sources, err := NormalizeSources(out.Sources)
	if err != nil {
		return in, err
	}
	out.Sources = sources
	out.ImageRepository = strings.TrimSpace(out.ImageRepository)
	if out.ImageRepository == "" {
		out.ImageRepository = DefaultImageRepository
	}
	if !imageRepositoryPattern.MatchString(out.ImageRepository) || strings.Contains(out.ImageRepository, "@") {
		return in, settingsError("bad_repository")
	}
	platforms := make([]string, 0, len(out.ImagePlatforms))
	seen := map[string]bool{}
	for _, p := range out.ImagePlatforms {
		p = strings.TrimSpace(p)
		if !platformPattern.MatchString(p) {
			return in, settingsError("bad_platform")
		}
		if !seen[p] {
			seen[p] = true
			platforms = append(platforms, p)
		}
	}
	out.ImagePlatforms = platforms
	return out, nil
}

// Settings reads the stored configuration, falling back to defaults.
func (s *Store) Settings(ctx context.Context) (Settings, error) {
	raw, err := s.readSetting(ctx, settingsKey)
	if err != nil {
		return DefaultSettings(), err
	}
	if raw == "" {
		return DefaultSettings(), nil //nolint:nilerr // a stored value that no longer validates falls back to defaults so the page still loads
	}
	var out Settings
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		return DefaultSettings(), fmt.Errorf("decoding agent release settings: %w", err)
	}
	if out.ImagePlatforms == nil {
		out.ImagePlatforms = []string{}
	}
	normalized, err := NormalizeSettings(out)
	if err != nil {
		return DefaultSettings(), nil //nolint:nilerr // a stored value that no longer validates falls back to defaults so the page still loads
	}
	return normalized, nil
}

// SaveSettings validates and stores the configuration.
func (s *Store) SaveSettings(ctx context.Context, in Settings) (Settings, error) {
	out, err := NormalizeSettings(in)
	if err != nil {
		return in, err
	}
	payload, err := json.Marshal(out)
	if err != nil {
		return in, fmt.Errorf("encoding agent release settings: %w", err)
	}
	if err := s.writeSetting(ctx, settingsKey, string(payload)); err != nil {
		return in, err
	}
	return out, nil
}

// State returns the cached check result.
func (s *Store) State(ctx context.Context) (State, error) {
	raw, err := s.readSetting(ctx, stateKey)
	if err != nil || raw == "" {
		return State{}, err
	}
	var st State
	if err := json.Unmarshal([]byte(raw), &st); err != nil {
		return State{}, fmt.Errorf("decoding agent release state: %w", err)
	}
	return st, nil
}

func (s *Store) saveState(ctx context.Context, st State) error {
	payload, err := json.Marshal(st)
	if err != nil {
		return fmt.Errorf("encoding agent release state: %w", err)
	}
	return s.writeSetting(ctx, stateKey, string(payload))
}

// Target is the version agents are moved to, or "" when the operator has not
// chosen one and the locally built or image-bundled agent is served instead.
func (s *Store) Target(ctx context.Context) (string, error) {
	return s.readSetting(ctx, targetKey)
}

// targetKeys is the selected version and build. A target saved before builds
// were stored side by side has no build and means the newest ready one.
func (s *Store) targetKeys(ctx context.Context) (version, build string, err error) {
	if version, err = s.Target(ctx); err != nil || version == "" {
		return version, "", err
	}
	build, err = s.readSetting(ctx, targetBuildKey)
	return version, build, err
}

// SetTarget selects a ready build as the one every install and update serves;
// build "" means the newest ready build of version, and version "" clears it.
func (s *Store) SetTarget(ctx context.Context, version, build string) error {
	version = strings.TrimSpace(version)
	build = strings.TrimSpace(build)
	if version != "" {
		rel, err := s.Get(ctx, version, build)
		if err != nil {
			return err
		}
		if rel.Status != StatusReady {
			return ErrReleaseNotReady
		}
		build = rel.Build
	} else {
		build = ""
	}
	if err := s.writeSetting(ctx, targetKey, version); err != nil {
		return err
	}
	return s.writeSetting(ctx, targetBuildKey, build)
}

// Check asks the configured sources which version is newest and caches the
// answer. It downloads nothing; the scheduler decides what happens next.
func (s *Store) Check(ctx context.Context) (State, error) {
	settings, err := s.Settings(ctx)
	if err != nil {
		return State{}, err
	}
	st, err := s.State(ctx)
	if err != nil {
		s.logger.Warn("agent-releases: previous state unreadable", "error", err)
	}
	checkCtx, cancel := context.WithTimeout(ctx, smallFetchTimeout)
	defer cancel()
	info, src, checkErr := s.latestFrom(checkCtx, settings.Sources)
	st.LastCheckedAt = s.now().UTC().Format(time.RFC3339)
	if checkErr != nil {
		st.LastError = checkErr.Error()
	} else {
		st.LatestVersion, st.LatestBuild, st.LatestSource, st.LastError = info.Version, info.Build, src.URL, ""
		st.Available = info.availableVersions()
	}
	if err := s.saveState(ctx, st); err != nil {
		return st, err
	}
	return st, checkErr
}

// StartPull pulls version in the background and returns once its row says
// "pulling"; the table polls for the outcome. build "" means the newest build.
// andTarget makes the verified build the target, which is what an operator's
// download asks for; every build stored before it stays stored.
func (s *Store) StartPull(ctx context.Context, version, build, by string, andTarget bool) error {
	if !ValidVersion(version) {
		return ErrInvalidVersion
	}
	if build != "" && !buildPattern.MatchString(build) {
		return ErrInvalidBuild
	}
	want, stored, done := s.planPull(ctx, version, build)
	if done {
		if andTarget {
			return s.SetTarget(ctx, stored.Version, stored.Build)
		}
		return nil
	}
	if !s.claim(version) {
		return ErrReleaseBusy
	}
	id, err := s.markPulling(ctx, version, want, by)
	if err != nil {
		s.release(version)
		return err
	}
	go func() { //nolint:contextcheck // the pull outlives the request and is bound to the store lifecycle context s.bg
		defer s.release(version)
		pullCtx, cancel := context.WithTimeout(s.bg, pullBudget)
		defer cancel()
		rel, err := s.pull(pullCtx, id, version, want, by)
		if err != nil {
			s.logger.Warn("agent-releases: pull failed", "version", version, "build", want, "error", err)
			return
		}
		if andTarget {
			if err := s.SetTarget(pullCtx, rel.Version, rel.Build); err != nil {
				s.logger.Warn("agent-releases: setting the downloaded build as target", "version", rel.Version, "build", rel.Build, "error", err)
			}
		}
	}()
	return nil
}

// Pull pulls version synchronously; the scheduler and tests use it. build ""
// means the newest build.
func (s *Store) Pull(ctx context.Context, version, build, by string) (Release, error) {
	if !ValidVersion(version) {
		return Release{}, ErrInvalidVersion
	}
	if build != "" && !buildPattern.MatchString(build) {
		return Release{}, ErrInvalidBuild
	}
	want, stored, done := s.planPull(ctx, version, build)
	if done {
		return stored, nil
	}
	if !s.claim(version) {
		return Release{}, ErrReleaseBusy
	}
	defer s.release(version)
	id, err := s.markPulling(ctx, version, want, by)
	if err != nil {
		return Release{}, err
	}
	return s.pull(ctx, id, version, want, by)
}

// planPull decides which build a pull fetches, or returns the stored build that
// already answers it. A stored build is never replaced: a newer one lands
// beside it, so the build agents run today stays available to step back to.
func (s *Store) planPull(ctx context.Context, version, build string) (want string, stored Release, done bool) {
	if build != "" {
		if rel, err := s.Get(ctx, version, build); err == nil && rel.Status == StatusReady {
			return build, rel, true
		}
		return build, Release{}, false
	}
	rel, err := s.Get(ctx, version, "")
	if err != nil || rel.Status != StatusReady {
		return "", Release{}, false
	}
	if !s.newerBuildListed(ctx, rel) {
		return rel.Build, rel, true
	}
	return s.newestListedBuild(ctx, version), Release{}, false
}

// newestListedBuild is the newest build the last check saw for version.
func (s *Store) newestListedBuild(ctx context.Context, version string) string {
	st, err := s.State(ctx)
	if err != nil {
		return ""
	}
	newest := ""
	if st.LatestVersion == version {
		newest = st.LatestBuild
	}
	for _, v := range st.Available {
		if v.Version == version && v.Build > newest {
			newest = v.Build
		}
	}
	return newest
}

// newerBuildListed reports whether the last check saw a later build of rel's
// version than the stored one; the build is what tells two artifacts of one
// version apart, so a rebuilt release must be fetchable. A pinned build never
// is: the operator stepped back on purpose, and a check must not undo that.
func (s *Store) newerBuildListed(ctx context.Context, rel Release) bool {
	if pins, err := s.pins(ctx); err == nil && rel.Build != "" && pins[rel.Version] == rel.Build {
		return false
	}
	st, err := s.State(ctx)
	if err != nil {
		return false
	}
	for _, v := range st.Available {
		if v.Version == rel.Version && v.Build > rel.Build {
			return true
		}
	}
	return st.LatestVersion == rel.Version && st.LatestBuild > rel.Build
}

func (s *Store) claim(version string) bool {
	s.pullMu.Lock()
	defer s.pullMu.Unlock()
	if s.pulling[version] {
		return false
	}
	s.pulling[version] = true
	return true
}

func (s *Store) release(version string) {
	s.pullMu.Lock()
	defer s.pullMu.Unlock()
	delete(s.pulling, version)
}

// pull tries each source in order, filling the row id. A source that lacks the
// version is skipped quietly; one that serves something failing verification is
// logged loudly and still skipped, so a tampered mirror cannot block the
// official source. A failure marks only this row; other builds are untouched.
func (s *Store) pull(ctx context.Context, id, version, build, by string) (Release, error) {
	settings, err := s.Settings(ctx)
	if err != nil {
		return Release{}, err
	}
	var reasons []string
	refused := false
	for _, src := range settings.Sources {
		rel, err := s.pullFrom(ctx, src, version, build, settings)
		if err == nil {
			rel.ID = id
			rel.PulledBy = by
			if err := s.finish(ctx, rel); err != nil {
				return rel, err
			}
			if err := s.updatePin(ctx, version, build); err != nil {
				s.logger.Warn("agent-releases: recording the chosen build", "version", version, "build", build, "error", err)
			}
			if err := s.Prune(ctx); err != nil {
				s.logger.Warn("agent-releases: retention sweep failed", "error", err)
			}
			return s.Get(ctx, version, rel.Build)
		}
		if isVerificationError(err) {
			refused = true
			s.logger.Warn("agent-releases: source served a release that failed verification",
				"source", src.URL, "version", version, "error", err)
		}
		reasons = append(reasons, src.URL+": "+err.Error())
	}
	cause := fmt.Errorf("%w: %s", ErrNoSourceAvailable, strings.Join(reasons, "; "))
	sigStatus := ""
	if refused {
		sigStatus = SignatureRefused
	}
	if err := s.fail(ctx, id, by, sigStatus, cause.Error()); err != nil {
		s.logger.Warn("agent-releases: recording failed pull", "version", version, "error", err)
	}
	return Release{}, cause
}

func isVerificationError(err error) bool {
	return errors.Is(err, ErrSignatureMissing) || errors.Is(err, ErrSignatureInvalid) ||
		errors.Is(err, ErrChecksumMismatch) || errors.Is(err, ErrSumsMalformed) || errors.Is(err, ErrNoAssets)
}

// pullFrom downloads and verifies one version from one source into a staging
// directory and moves it into place only when every check passed.
func (s *Store) pullFrom(ctx context.Context, src Source, version, wantBuild string, settings Settings) (Release, error) {
	staging := filepath.Join(s.root, ".staging-"+version+"-"+xid.New().String())
	if err := os.MkdirAll(staging, 0o755); err != nil {
		return Release{}, fmt.Errorf("creating %s: %w", staging, err)
	}
	defer func() {
		if err := os.RemoveAll(staging); err != nil {
			s.logger.Warn("agent-releases: removing staging directory", "path", staging, "error", err)
		}
	}()

	folder := wantBuild
	if folder == "" {
		folder = s.indexedBuild(ctx, src, version)
	}
	sums, err := s.fetchSmallBounded(ctx, src.releaseURL(version, folder, sumsFile))
	// A named build has exactly one home; falling back to the flat layout would
	// store whatever build sits there instead of the one the operator picked.
	if errors.Is(err, errNotFound) && folder != "" && wantBuild == "" {
		folder = ""
		sums, err = s.fetchSmallBounded(ctx, src.assetURL(version, sumsFile))
	}
	if err != nil {
		return Release{}, err
	}
	sig, err := s.fetchSmallBounded(ctx, src.releaseURL(version, folder, sigFile))
	if errors.Is(err, errNotFound) {
		return Release{}, ErrSignatureMissing
	}
	if err != nil {
		return Release{}, err
	}
	keyFP, err := VerifySums(sums, sig, s.keys)
	if err != nil {
		return Release{}, err
	}
	entries, err := ParseSums(sums)
	if err != nil {
		return Release{}, err
	}

	names := make([]string, 0, len(entries))
	for name := range entries {
		names = append(names, name)
	}
	sort.Strings(names)
	var assets []Asset
	var size int64
	for _, name := range names {
		osName, arch, ok := binaryPlatform(name)
		if !ok {
			continue
		}
		fetchCtx, cancel := context.WithTimeout(ctx, assetFetchTimeout)
		n, err := s.fetchVerified(fetchCtx, src.releaseURL(version, folder, name), filepath.Join(staging, name), entries[name])
		cancel()
		if err != nil {
			return Release{}, err
		}
		assets = append(assets, Asset{Name: name, Kind: AssetBinary, OS: osName, Arch: arch, SHA256: entries[name], Size: n})
		size += n
	}
	if len(assets) == 0 {
		return Release{}, ErrNoAssets
	}

	build := ""
	if want, ok := entries[buildFile]; ok {
		data, err := s.fetchListed(ctx, src.releaseURL(version, folder, buildFile), want)
		if err != nil {
			return Release{}, err
		}
		if candidate := strings.TrimSpace(string(data)); buildPattern.MatchString(candidate) {
			build = candidate
		}
	}
	if wantBuild != "" && build != wantBuild {
		return Release{}, fmt.Errorf("%w: %s has %q, want %q", ErrBuildMismatch, src.URL, build, wantBuild)
	}
	pinnedImage := ""
	if want, ok := entries[imageFile]; ok {
		data, err := s.fetchListed(ctx, src.releaseURL(version, folder, imageFile), want)
		if err != nil {
			return Release{}, err
		}
		candidate := strings.TrimSpace(string(data))
		if !imageRefPattern.MatchString(candidate) {
			return Release{}, fmt.Errorf("%w: image.txt does not name a digest-pinned image", ErrSumsMalformed)
		}
		pinnedImage = candidate
	}
	if err := os.WriteFile(filepath.Join(staging, sumsFile), sums, 0o644); err != nil {
		return Release{}, fmt.Errorf("writing %s: %w", sumsFile, err)
	}
	if err := os.WriteFile(filepath.Join(staging, sigFile), sig, 0o644); err != nil {
		return Release{}, fmt.Errorf("writing %s: %w", sigFile, err)
	}

	imageErr := ""
	imageRef := ""
	if len(settings.ImagePlatforms) > 0 {
		imgAssets, err := s.saveImages(ctx, staging, version, pinnedImage, settings.ImageRepository, settings.ImagePlatforms)
		for _, a := range imgAssets {
			assets = append(assets, a)
			size += a.Size
		}
		if err != nil {
			imageErr = err.Error()
			s.logger.Warn("agent-releases: image save failed; binaries are still stored", "version", version, "error", err)
		}
		if len(imgAssets) > 0 {
			imageRef = LocalImageRef(version)
		}
	}

	// Only a re-download of the very same build lands on an existing directory;
	// it is moved aside rather than deleted first, so it is never served empty.
	final := s.buildDir(version, build)
	aside := filepath.Join(s.root, ".old-"+version+"-"+xid.New().String())
	if err := os.Rename(final, aside); err != nil && !errors.Is(err, os.ErrNotExist) {
		return Release{}, fmt.Errorf("moving %s aside: %w", final, err)
	}
	if err := os.Rename(staging, final); err != nil {
		if rerr := os.Rename(aside, final); rerr != nil && !errors.Is(rerr, os.ErrNotExist) {
			s.logger.Warn("agent-releases: restoring previous build", "version", version, "error", rerr)
		}
		return Release{}, fmt.Errorf("moving %s into place: %w", version, err)
	}
	if err := os.RemoveAll(aside); err != nil {
		s.logger.Warn("agent-releases: removing previous build", "path", aside, "error", err)
	}
	return Release{
		Version: version, Build: build, SourceKind: src.Kind, SourceURL: src.URL,
		Status: StatusReady, SignatureStatus: SignatureVerified, SignatureKey: keyFP,
		Assets: assets, SizeBytes: size, ImageRef: imageRef, ImageError: imageErr,
	}, nil
}

func (s *Store) fetchSmallBounded(ctx context.Context, rawURL string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, smallFetchTimeout)
	defer cancel()
	return s.fetchSmall(ctx, rawURL)
}

// fetchListed reads a small file that SHA256SUMS lists and checks its digest.
func (s *Store) fetchListed(ctx context.Context, rawURL, want string) ([]byte, error) {
	data, err := s.fetchSmallBounded(ctx, rawURL)
	if err != nil {
		return nil, err
	}
	sum := sha256.Sum256(data)
	if hex.EncodeToString(sum[:]) != want {
		return nil, fmt.Errorf("%w: %s", ErrChecksumMismatch, rawURL)
	}
	return data, nil
}

// Delete removes one stored build. The target and any build a connected agent
// runs are refused, so a delete can never strand an install or a rollback.
// build "" is accepted only while version has a single row.
func (s *Store) Delete(ctx context.Context, version, build string) error {
	if !ValidVersion(version) {
		return ErrInvalidVersion
	}
	if build != "" && !buildPattern.MatchString(build) {
		return ErrInvalidBuild
	}
	rel, err := s.exact(ctx, version, build)
	if err != nil {
		return err
	}
	if rel.Status == StatusPulling {
		return ErrReleaseBusy
	}
	targetVersion, targetBuild, err := s.targetKeys(ctx)
	if err != nil {
		return err
	}
	if s.isTarget(ctx, rel, targetVersion, targetBuild) {
		return ErrReleaseIsTarget
	}
	if inUse(s.inUse(), rel) {
		return ErrReleaseInUse
	}
	return s.remove(ctx, rel)
}

// exact finds the row for version and build; build "" matches the row without
// one, or the only row of version, and never guesses between several.
func (s *Store) exact(ctx context.Context, version, build string) (Release, error) {
	rows, err := s.rowsOf(ctx, version)
	if err != nil {
		return Release{}, err
	}
	for _, rel := range rows {
		if rel.Build == build {
			return rel, nil
		}
	}
	switch {
	case build == "" && len(rows) == 1:
		return rows[0], nil
	case build == "" && len(rows) > 1:
		return Release{}, ErrInvalidBuild
	}
	return Release{}, ErrReleaseNotFound
}

func (s *Store) remove(ctx context.Context, rel Release) error {
	if err := os.RemoveAll(s.releaseDir(rel)); err != nil {
		return fmt.Errorf("removing release files for %s %s: %w", rel.Version, rel.Build, err)
	}
	if _, err := s.db.ExecContext(ctx, `DELETE FROM agent_releases WHERE id = ?`, rel.ID); err != nil {
		return fmt.Errorf("deleting release row %s %s: %w", rel.Version, rel.Build, err)
	}
	pins, err := s.pins(ctx)
	if err != nil {
		s.logger.Warn("agent-releases: pins unreadable", "error", err)
		return nil
	}
	if rel.Build == "" || pins[rel.Version] != rel.Build {
		return nil
	}
	return s.setPin(ctx, rel.Version, "")
}

// buildDir is where a build's files live. A release with no BUILD file keeps
// the bare version directory it always had.
func (s *Store) buildDir(version, build string) string {
	if build == "" {
		return filepath.Join(s.root, version)
	}
	return filepath.Join(s.root, version+"_"+build)
}

// releaseDir is rel's directory, falling back to the bare version directory a
// build stored before builds were kept side by side still sits in.
func (s *Store) releaseDir(rel Release) string {
	dir := s.buildDir(rel.Version, rel.Build)
	if rel.Build == "" {
		return dir
	}
	if _, err := os.Stat(dir); err == nil {
		return dir
	}
	legacy := filepath.Join(s.root, rel.Version)
	if _, err := os.Stat(legacy); err == nil {
		return legacy
	}
	return dir
}

// isTarget reports whether rel is the selected build; a target saved without a
// build means the newest ready build of its version.
func (s *Store) isTarget(ctx context.Context, rel Release, targetVersion, targetBuild string) bool {
	if targetVersion == "" || rel.Version != targetVersion {
		return false
	}
	if targetBuild != "" {
		return rel.Build == targetBuild
	}
	newest, err := s.Get(ctx, targetVersion, "")
	return err == nil && newest.ID == rel.ID
}

// inUse matches a build against what connected agents run: "version@build"
// when the agent reports its build, the bare version when it does not.
func inUse(used []string, rel Release) bool {
	return contains(used, rel.Version+"@"+rel.Build) || contains(used, rel.Version)
}

// pins is the build the operator chose for each version over a newer one.
func (s *Store) pins(ctx context.Context) (map[string]string, error) {
	raw, err := s.readSetting(ctx, pinsKey)
	if err != nil || raw == "" {
		return map[string]string{}, err
	}
	out := map[string]string{}
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		return map[string]string{}, fmt.Errorf("decoding agent release pins: %w", err)
	}
	return out, nil
}

// setPin records build as the chosen build of version; "" forgets the choice.
func (s *Store) setPin(ctx context.Context, version, build string) error {
	pins, err := s.pins(ctx)
	if err != nil {
		s.logger.Warn("agent-releases: pins unreadable, starting afresh", "error", err)
	}
	if pins[version] == build {
		return nil
	}
	if build == "" {
		delete(pins, version)
	} else {
		pins[version] = build
	}
	payload, err := json.Marshal(pins)
	if err != nil {
		return fmt.Errorf("encoding agent release pins: %w", err)
	}
	return s.writeSetting(ctx, pinsKey, string(payload))
}

// updatePin pins a named build that is older than the newest one listed for its
// version, and unpins the version otherwise, so choosing the newest build again
// hands it back to the automatic checks.
func (s *Store) updatePin(ctx context.Context, version, build string) error {
	if build == "" {
		return s.setPin(ctx, version, "")
	}
	st, err := s.State(ctx)
	if err != nil {
		return err
	}
	newest := ""
	if st.LatestVersion == version {
		newest = st.LatestBuild
	}
	for _, v := range st.Available {
		if v.Version == version && v.Build > newest {
			newest = v.Build
		}
	}
	if build < newest {
		return s.setPin(ctx, version, build)
	}
	return s.setPin(ctx, version, "")
}

// Prune applies the retention setting: the newest Keep releases stay, and
// anything older goes unless it is the target or running on a connected agent.
func (s *Store) Prune(ctx context.Context) error {
	settings, err := s.Settings(ctx)
	if err != nil {
		return err
	}
	releases, err := s.List(ctx)
	if err != nil {
		return err
	}
	kept := 0
	for _, rel := range releases {
		// Only stored releases count and only they are pruned: a failed pull
		// must never push the last good release out of the store.
		if rel.Status != StatusReady {
			continue
		}
		if kept < settings.Keep {
			kept++
			continue
		}
		if rel.IsTarget || rel.InUse {
			continue
		}
		if err := s.remove(ctx, rel); err != nil {
			return err
		}
		s.logger.Info("agent-releases: pruned by retention", "version", rel.Version, "build", rel.Build)
	}
	return nil
}

// List returns every stored release, newest version first, with IsTarget and
// InUse filled in.
func (s *Store) List(ctx context.Context) ([]Release, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT `+releaseColumns+` FROM agent_releases ORDER BY created_at DESC LIMIT 1000`)
	if err != nil {
		return nil, fmt.Errorf("listing agent releases: %w", err)
	}
	defer func() {
		if err := rows.Close(); err != nil {
			slog.Warn("closing sql rows failed", "error", err)
		}
	}()
	targetVersion, targetBuild, err := s.targetKeys(ctx)
	if err != nil {
		return nil, err
	}
	used := s.inUse()
	pins, err := s.pins(ctx)
	if err != nil {
		s.logger.Warn("agent-releases: pins unreadable", "error", err)
	}
	out := []Release{}
	for rows.Next() {
		rel, err := scanRelease(rows)
		if err != nil {
			return nil, err
		}
		rel.InUse = inUse(used, rel)
		rel.Pinned = rel.Build != "" && pins[rel.Version] == rel.Build
		out = append(out, rel)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("listing agent releases: %w", err)
	}
	sort.SliceStable(out, func(i, j int) bool { return newer(out[i].Version, out[i].Build, out[j].Version, out[j].Build) })
	targetIdx := -1
	for i := range out {
		if out[i].Version == targetVersion && targetBuild != "" && out[i].Build == targetBuild {
			targetIdx = i
		}
	}
	if targetIdx < 0 && targetBuild == "" {
		targetIdx = firstReadyOf(out, targetVersion)
	}
	if targetIdx >= 0 {
		out[targetIdx].IsTarget = true
	}
	return out, nil
}

// firstReadyOf is the index of the newest ready build of version in a list
// sorted newest first, or -1.
func firstReadyOf(list []Release, version string) int {
	if version == "" {
		return -1
	}
	for i := range list {
		if list[i].Version == version && list[i].Status == StatusReady {
			return i
		}
	}
	return -1
}

// Get returns the release of version at build. build "" means the newest ready
// build, else the newest row of any status, so a pull in flight is still seen.
func (s *Store) Get(ctx context.Context, version, build string) (Release, error) {
	if build != "" {
		row := s.db.QueryRowContext(ctx, `SELECT `+releaseColumns+` FROM agent_releases WHERE version = ? AND build = ?`, version, build)
		rel, err := scanRelease(row)
		if errors.Is(err, sql.ErrNoRows) {
			return Release{}, ErrReleaseNotFound
		}
		return rel, err
	}
	rows, err := s.rowsOf(ctx, version)
	if err != nil {
		return Release{}, err
	}
	if len(rows) == 0 {
		return Release{}, ErrReleaseNotFound
	}
	if i := firstReadyOf(rows, version); i >= 0 {
		return rows[i], nil
	}
	return rows[0], nil
}

// rowsOf returns every row of version, newest build first.
func (s *Store) rowsOf(ctx context.Context, version string) ([]Release, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT `+releaseColumns+` FROM agent_releases WHERE version = ? LIMIT 1000`, version)
	if err != nil {
		return nil, fmt.Errorf("reading release %s: %w", version, err)
	}
	defer func() { db.CloseRows(rows) }()
	out := []Release{}
	for rows.Next() {
		rel, err := scanRelease(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, rel)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("reading release %s: %w", version, err)
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Build > out[j].Build })
	return out, nil
}

// Overview gathers everything the settings table needs in one read.
func (s *Store) Overview(ctx context.Context) (Overview, error) {
	releases, err := s.List(ctx)
	if err != nil {
		return Overview{}, err
	}
	target, err := s.Target(ctx)
	if err != nil {
		return Overview{}, err
	}
	targetBuild := ""
	if rel, ok := s.targetRelease(ctx); ok {
		targetBuild = rel.Build
	}
	st, err := s.State(ctx)
	if err != nil {
		return Overview{}, err
	}
	settings, err := s.Settings(ctx)
	if err != nil {
		return Overview{}, err
	}
	return Overview{Releases: releases, Target: target, TargetBuild: targetBuild, State: st, Settings: settings}, nil
}

// TargetBinaryPath returns the stored binary of the target release for a
// platform, with that release's version and build; "" when there is none.
func (s *Store) TargetBinaryPath(ctx context.Context, osName, arch string) (path, version, build string) {
	rel, ok := s.targetRelease(ctx)
	if !ok {
		return "", "", ""
	}
	for _, a := range rel.Assets {
		if a.Kind == AssetBinary && a.OS == osName && a.Arch == arch {
			p := filepath.Join(s.releaseDir(rel), a.Name)
			if info, err := os.Stat(p); err == nil && !info.IsDir() && info.Size() == a.Size {
				return p, rel.Version, rel.Build
			}
		}
	}
	return "", rel.Version, rel.Build
}

// TargetImage returns the stored image tarball of the target release for a
// platform, its digest and the reference it loads as; path "" when there is none.
func (s *Store) TargetImage(ctx context.Context, osName, arch string) (path, sha, ref, version string) {
	rel, ok := s.targetRelease(ctx)
	if !ok {
		return "", "", "", ""
	}
	for _, a := range rel.Assets {
		if a.Kind == AssetImage && a.OS == osName && a.Arch == arch {
			p := filepath.Join(s.releaseDir(rel), a.Name)
			if info, err := os.Stat(p); err == nil && !info.IsDir() && info.Size() == a.Size {
				return p, a.SHA256, LocalImageRef(rel.Version), rel.Version
			}
		}
	}
	return "", "", "", rel.Version
}

// TargetRelease returns the ready target release, if one is selected.
func (s *Store) TargetRelease(ctx context.Context) (Release, bool) {
	return s.targetRelease(ctx)
}

// TargetImageRef is the reference the target release's agent image loads as,
// or "" when no ready target carries one; the agent module's install
// instructions read it without depending on Release.
func (s *Store) TargetImageRef(ctx context.Context) string {
	rel, ok := s.targetRelease(ctx)
	if !ok {
		return ""
	}
	return rel.ImageRef
}

func (s *Store) targetRelease(ctx context.Context) (Release, bool) {
	target, build, err := s.targetKeys(ctx)
	if err != nil || target == "" {
		return Release{}, false
	}
	rel, err := s.Get(ctx, target, build)
	if err != nil || rel.Status != StatusReady {
		return Release{}, false
	}
	return rel, true
}

const releaseColumns = `id, version, build, source_kind, source_url, status, signature_status, signature_key,
	assets, size_bytes, image_ref, image_error, last_error, pulled_at, pulled_by, created_at, updated_at`

type rowScanner interface {
	Scan(dest ...any) error
}

func scanRelease(row rowScanner) (Release, error) {
	var rel Release
	var assets string
	if err := row.Scan(&rel.ID, &rel.Version, &rel.Build, &rel.SourceKind, &rel.SourceURL, &rel.Status,
		&rel.SignatureStatus, &rel.SignatureKey, &assets, &rel.SizeBytes, &rel.ImageRef, &rel.ImageError,
		&rel.LastError, &rel.PulledAt, &rel.PulledBy, &rel.CreatedAt, &rel.UpdatedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return rel, err
		}
		return rel, fmt.Errorf("scanning agent release: %w", err)
	}
	rel.Assets = []Asset{}
	if assets != "" {
		if err := json.Unmarshal([]byte(assets), &rel.Assets); err != nil {
			return rel, fmt.Errorf("decoding assets of %s: %w", rel.Version, err)
		}
	}
	return rel, nil
}

// markPulling records that a pull of version at build is under way, creating
// the row or resetting a failed one, and returns its id so the table shows
// progress from the first poll. build "" is a pull whose files decide the build.
func (s *Store) markPulling(ctx context.Context, version, build, by string) (string, error) {
	now := s.now().UTC().Format(time.RFC3339)
	var id string
	err := s.db.QueryRowContext(ctx,
		`SELECT id FROM agent_releases WHERE version = ? AND build = ? AND status <> ?`,
		version, build, StatusReady).Scan(&id)
	switch {
	case err == nil:
		if _, err := s.db.ExecContext(ctx,
			`UPDATE agent_releases SET status = ?, last_error = '', pulled_by = ?, updated_at = ? WHERE id = ?`,
			StatusPulling, by, now, id); err != nil {
			return "", fmt.Errorf("marking %s as pulling: %w", version, err)
		}
		return id, nil
	case !errors.Is(err, sql.ErrNoRows):
		return "", fmt.Errorf("reading release %s: %w", version, err)
	}
	id = xid.New().String()
	if _, err := s.db.ExecContext(ctx,
		`INSERT INTO agent_releases (id, version, build, status, pulled_by, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?)`,
		id, version, build, StatusPulling, by, now, now); err != nil {
		return "", fmt.Errorf("recording pull of %s: %w", version, err)
	}
	return id, nil
}

// finish records a verified build on its pull row. A row that already held the
// same version and build is the same artifact fetched again, so it is folded
// into this one rather than left as a duplicate.
func (s *Store) finish(ctx context.Context, rel Release) error {
	assets, err := json.Marshal(rel.Assets)
	if err != nil {
		return fmt.Errorf("encoding assets: %w", err)
	}
	dupes, err := s.rowsOf(ctx, rel.Version)
	if err != nil {
		return err
	}
	for _, d := range dupes {
		if d.ID == rel.ID || d.Build != rel.Build {
			continue
		}
		if dir := s.releaseDir(d); dir != s.buildDir(rel.Version, rel.Build) {
			if err := os.RemoveAll(dir); err != nil {
				s.logger.Warn("agent-releases: removing superseded copy", "path", dir, "error", err)
			}
		}
		if _, err := s.db.ExecContext(ctx, `DELETE FROM agent_releases WHERE id = ?`, d.ID); err != nil {
			return fmt.Errorf("folding duplicate row of %s %s: %w", rel.Version, rel.Build, err)
		}
	}
	now := s.now().UTC().Format(time.RFC3339)
	_, err = s.db.ExecContext(ctx,
		`UPDATE agent_releases SET build = ?, source_kind = ?, source_url = ?, status = ?, signature_status = ?,
		        signature_key = ?, assets = ?, size_bytes = ?, image_ref = ?, image_error = ?, last_error = '',
		        pulled_at = ?, pulled_by = ?, updated_at = ?
		  WHERE id = ?`,
		rel.Build, rel.SourceKind, rel.SourceURL, StatusReady, rel.SignatureStatus, rel.SignatureKey, string(assets),
		rel.SizeBytes, rel.ImageRef, rel.ImageError, now, rel.PulledBy, now, rel.ID)
	if err != nil {
		return fmt.Errorf("recording release %s: %w", rel.Version, err)
	}
	return nil
}

// fail marks one pull row failed. Its files never left staging, so no other
// build's directory is touched.
func (s *Store) fail(ctx context.Context, id, by, sigStatus, reason string) error {
	if len(reason) > 4000 {
		reason = reason[:4000]
	}
	now := s.now().UTC().Format(time.RFC3339)
	_, err := s.db.ExecContext(ctx,
		`UPDATE agent_releases SET status = ?, signature_status = ?, assets = '[]', size_bytes = 0, image_ref = '',
		        last_error = ?, pulled_by = ?, updated_at = ? WHERE id = ?`,
		StatusFailed, sigStatus, reason, by, now, id)
	if err != nil {
		return fmt.Errorf("recording failed pull %s: %w", id, err)
	}
	return nil
}

func (s *Store) readSetting(ctx context.Context, key string) (string, error) {
	var value string
	err := s.db.QueryRowContext(ctx, `SELECT value FROM settings WHERE category = ? AND key = ?`, settingsCategory, key).Scan(&value)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("reading setting %s: %w", key, err)
	}
	return value, nil
}

func (s *Store) writeSetting(ctx context.Context, key, value string) error {
	now := s.now().UTC().Format(time.RFC3339)
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO settings (id, category, key, value, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?)
		 ON CONFLICT(key) DO UPDATE SET value = excluded.value, updated_at = excluded.updated_at, category = excluded.category`,
		xid.New().String(), settingsCategory, key, value, now, now)
	if err != nil {
		return fmt.Errorf("writing setting %s: %w", key, err)
	}
	return nil
}

// newer reports whether (v1, b1) is a later release than (v2, b2): version
// first, then the build stamp, which is fixed-width so string order is time order.
func newer(v1, b1, v2, b2 string) bool {
	if v1 != v2 {
		return !coreagent.AgentVersionAtLeast(v2, v1)
	}
	return b1 > b2
}

// Newer is newer for callers outside the package.
func Newer(v1, b1, v2, b2 string) bool { return newer(v1, b1, v2, b2) }

func contains(list []string, v string) bool {
	for _, item := range list {
		if strings.TrimPrefix(strings.TrimSpace(item), "v") == v {
			return true
		}
	}
	return false
}

// formatSize is used by audit details so a pull reads "37.2 MB" rather than bytes.
func formatSize(n int64) string {
	return strconv.FormatFloat(float64(n)/1e6, 'f', 1, 64) + " MB"
}
