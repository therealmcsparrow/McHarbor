// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// Package nativeupdate installs a newer McHarbor release on a native (deb, rpm or tarball) host:
// it downloads the release's package, verifies it against the release's SHA256SUMS and hands the
// install to a transient systemd unit, so the restart the install triggers cannot kill it halfway.
package nativeupdate

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"time"

	appversion "github.com/therealmcsparrow/mcharbor/core/version"
)

// DefaultReleaseURL is the release the native updater installs, the same one the update check reads.
const DefaultReleaseURL = "https://api.github.com/repos/therealmcsparrow/mcharbor/releases/latest"

// Kinds an install can report through MCHARBOR_INSTALL_KIND that the updater can upgrade in place.
const (
	KindDeb     = "deb"
	KindRPM     = "rpm"
	KindTarball = "tarball"
)

const (
	checksumsAsset  = "SHA256SUMS"
	smallLimit      = 1 << 20
	packageLimit    = 1 << 30
	metadataTimeout = 30 * time.Second
)

// Errors Supported and Apply return, so a caller can name the reason to the operator.
var (
	// ErrUnsupportedKind is a container, apk or unknown install, which this package does not upgrade.
	ErrUnsupportedKind = errors.New("this install kind is not upgraded in place")
	// ErrNeedsRoot is a native install running as the sandboxed mcharbor account, which cannot install packages.
	ErrNeedsRoot = errors.New("installing a package needs McHarbor to run as root")
	// ErrNoSystemd is a host without systemd-run, which the install needs to outlive the restart.
	ErrNoSystemd = errors.New("systemd-run is not available")
	// ErrNoPackage is a release that publishes no package for this install kind and architecture.
	ErrNoPackage = errors.New("the release has no package for this host")
	// ErrChecksum is a package whose digest does not match the release's SHA256SUMS.
	ErrChecksum = errors.New("the downloaded package does not match the release checksum")
)

var (
	versionPattern   = regexp.MustCompile(`^[0-9]+\.[0-9]+\.[0-9]+$`)
	assetNamePattern = regexp.MustCompile(`^[A-Za-z0-9._+-]+$`)
)

// Updater installs the latest release on this host. The unexported fields are seams for tests.
type Updater struct {
	Kind       string
	DataDir    string
	ReleaseURL string
	Client     *http.Client
	Logger     *slog.Logger

	goos         string
	goarch       string
	euid         func() int
	lookPath     func(string) (string, error)
	run          func(ctx context.Context, name string, args ...string) ([]byte, error)
	assetAllowed func(*url.URL) bool
	now          func() time.Time
}

// IsNativeKind reports an install kind that runs on the host rather than in a container; apk counts,
// so the operator is told why it cannot update itself instead of being offered a container update.
func IsNativeKind(kind string) bool {
	kind = strings.TrimSpace(kind)
	return kind != "" && kind != "container"
}

// New returns an Updater for this process: kind is MCHARBOR_INSTALL_KIND and dataDir is DATA_DIR,
// under which the package and the install log are kept.
func New(kind, dataDir string, logger *slog.Logger) *Updater {
	return &Updater{Kind: strings.TrimSpace(kind), DataDir: dataDir, ReleaseURL: DefaultReleaseURL, Logger: logger}
}

// Result names what Apply started, so the operator can find it: the transient unit's journal and the log file.
type Result struct {
	Version string `json:"version"`
	Asset   string `json:"asset"`
	Unit    string `json:"unit"`
	LogPath string `json:"logPath"`
}

// Supported reports nil when Apply can work on this host, or the reason it cannot.
func (u *Updater) Supported() error {
	if u.goosOr() != "linux" {
		return ErrUnsupportedKind
	}
	tool := ""
	switch u.Kind {
	case KindDeb:
		tool = "dpkg"
	case KindRPM:
		tool = "rpm"
	case KindTarball:
		tool = "tar"
	default:
		return ErrUnsupportedKind
	}
	if u.euidOr() != 0 {
		return ErrNeedsRoot
	}
	if _, err := u.lookPathOr("systemd-run"); err != nil {
		return ErrNoSystemd
	}
	if _, err := u.lookPathOr(tool); err != nil {
		return fmt.Errorf("%w: %s is not installed", ErrUnsupportedKind, tool)
	}
	return nil
}

// Apply downloads and verifies the latest release's package and starts its install in a transient
// systemd unit. It returns once the unit has started; the install then restarts McHarbor.
func (u *Updater) Apply(ctx context.Context) (Result, error) {
	if err := u.Supported(); err != nil {
		return Result{}, err
	}
	dataDir, err := filepath.Abs(u.DataDir)
	if err != nil || strings.ContainsAny(dataDir, "'\n") {
		return Result{}, fmt.Errorf("unusable data directory %q", u.DataDir)
	}
	rel, err := u.latestRelease(ctx)
	if err != nil {
		return Result{}, err
	}
	version := strings.TrimPrefix(strings.TrimSpace(rel.TagName), "v")
	if !versionPattern.MatchString(version) {
		return Result{}, fmt.Errorf("the release names an invalid version %q", rel.TagName)
	}
	asset, ok := findAsset(rel.Assets, u.Kind, version, u.goarchOr())
	if !ok {
		return Result{}, fmt.Errorf("%w: %s %s/%s", ErrNoPackage, u.Kind, version, u.goarchOr())
	}
	sumsAsset, ok := assetNamed(rel.Assets, checksumsAsset)
	if !ok {
		return Result{}, fmt.Errorf("%w: the release publishes no %s", ErrChecksum, checksumsAsset)
	}
	sums, err := u.fetchChecksums(ctx, sumsAsset.URL)
	if err != nil {
		return Result{}, err
	}
	want, ok := sums[asset.Name]
	if !ok {
		return Result{}, fmt.Errorf("%w: %s is not listed in %s", ErrChecksum, asset.Name, checksumsAsset)
	}

	dir := filepath.Join(dataDir, "updates", "native")
	if err := os.RemoveAll(dir); err != nil {
		return Result{}, fmt.Errorf("clearing the previous update: %w", err)
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return Result{}, fmt.Errorf("creating the update directory: %w", err)
	}
	pkg := filepath.Join(dir, asset.Name)
	if err := u.download(ctx, asset.URL, pkg, want); err != nil {
		return Result{}, err
	}
	logPath := filepath.Join(dir, "install.log")
	script := filepath.Join(dir, "install-update.sh")
	if err := os.WriteFile(script, []byte(installScript(u.Kind, version, dir, pkg, logPath)), 0o700); err != nil {
		return Result{}, fmt.Errorf("writing the install script: %w", err)
	}
	unit := "mcharbor-native-update-" + u.nowOr().UTC().Format("20060102150405")
	out, err := u.runOr(ctx, "systemd-run", "--unit="+unit, "--collect", "--quiet",
		"--description=McHarbor native update to "+version, "/bin/sh", script)
	if err != nil {
		return Result{}, fmt.Errorf("starting %s: %w: %s", unit, err, strings.TrimSpace(string(out)))
	}
	if u.Logger != nil {
		u.Logger.Info("native self-update started", "version", version, "asset", asset.Name, "unit", unit, "log", logPath)
	}
	return Result{Version: version, Asset: asset.Name, Unit: unit, LogPath: logPath}, nil
}

// installScript is what the transient unit runs. Every path in it is built from the data directory,
// which Apply refuses when it holds a quote, and from asset names the release lookup validated.
func installScript(kind, version, dir, pkg, logPath string) string {
	var install string
	switch kind {
	case KindDeb:
		// dpkg, not apt: apt skips a rebuild that keeps the version, and the package has no new dependencies to fetch.
		install = fmt.Sprintf("DEBIAN_FRONTEND=noninteractive dpkg -i --force-confold '%s'", pkg)
	case KindRPM:
		install = fmt.Sprintf("rpm -U --replacepkgs '%s'", pkg)
	default:
		unpacked := filepath.Join(dir, "unpacked")
		install = fmt.Sprintf("mkdir -p '%[1]s' && tar -xzf '%[2]s' -C '%[1]s' && '%[1]s/mcharbor/install.sh'", unpacked, pkg)
	}
	return fmt.Sprintf(`#!/bin/sh
exec >> '%[1]s' 2>&1
echo "== McHarbor native update to %[2]s started $(date -u '+%%Y-%%m-%%dT%%H:%%M:%%SZ')"
if %[3]s; then
    echo "== McHarbor native update to %[2]s finished"
else
    status=$?
    echo "== McHarbor native update to %[2]s failed with exit code $status"
    exit $status
fi
`, logPath, version, install)
}

// releaseAsset is one downloadable file of a release.
type releaseAsset struct {
	Name string `json:"name"`
	URL  string `json:"browser_download_url"`
}

type release struct {
	TagName string         `json:"tag_name"`
	Assets  []releaseAsset `json:"assets"`
}

// packageArch is how each package format names an architecture.
func packageArch(kind, goarch string) string {
	switch kind {
	case KindRPM:
		switch goarch {
		case "amd64":
			return "x86_64"
		case "arm64":
			return "aarch64"
		}
	case KindDeb, KindTarball:
		return goarch
	}
	return goarch
}

// findAsset picks the release file a kind installs from, by the names the Prod workflow publishes.
func findAsset(assets []releaseAsset, kind, version, goarch string) (releaseAsset, bool) {
	arch := packageArch(kind, goarch)
	for _, a := range assets {
		if !assetNamePattern.MatchString(a.Name) {
			continue
		}
		switch kind {
		case KindDeb:
			if a.Name == fmt.Sprintf("mcharbor_%s_%s.deb", version, arch) {
				return a, true
			}
		case KindRPM:
			if strings.HasPrefix(a.Name, "mcharbor-"+version+"-") && strings.HasSuffix(a.Name, "."+arch+".rpm") {
				return a, true
			}
		case KindTarball:
			if a.Name == fmt.Sprintf("mcharbor-%s-linux-%s.tar.gz", version, arch) {
				return a, true
			}
		}
	}
	return releaseAsset{}, false
}

func assetNamed(assets []releaseAsset, name string) (releaseAsset, bool) {
	for _, a := range assets {
		if a.Name == name {
			return a, true
		}
	}
	return releaseAsset{}, false
}

// parseChecksums reads sha256sum output: "<hex>  <name>", with "*" marking binary mode.
func parseChecksums(r io.Reader) map[string]string {
	out := map[string]string{}
	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) != 2 || len(fields[0]) != 64 {
			continue
		}
		if _, err := hex.DecodeString(fields[0]); err != nil {
			continue
		}
		out[strings.TrimPrefix(fields[1], "*")] = strings.ToLower(fields[0])
	}
	return out
}

func (u *Updater) latestRelease(ctx context.Context) (release, error) {
	ctx, cancel := context.WithTimeout(ctx, metadataTimeout)
	defer cancel()
	body, err := u.get(ctx, u.ReleaseURL, "application/vnd.github+json", false)
	if err != nil {
		return release{}, err
	}
	defer u.close(body)
	var rel release
	if err := json.NewDecoder(io.LimitReader(body, smallLimit)).Decode(&rel); err != nil {
		return release{}, fmt.Errorf("decoding the release: %w", err)
	}
	return rel, nil
}

func (u *Updater) fetchChecksums(ctx context.Context, raw string) (map[string]string, error) {
	ctx, cancel := context.WithTimeout(ctx, metadataTimeout)
	defer cancel()
	body, err := u.get(ctx, raw, "application/octet-stream", true)
	if err != nil {
		return nil, err
	}
	defer u.close(body)
	return parseChecksums(io.LimitReader(body, smallLimit)), nil
}

// download streams one asset to dest through a .part file and keeps it only when its digest is want.
func (u *Updater) download(ctx context.Context, raw, dest, want string) error {
	body, err := u.get(ctx, raw, "application/octet-stream", true)
	if err != nil {
		return err
	}
	defer u.close(body)
	part := dest + ".part"
	f, err := os.OpenFile(part, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		return fmt.Errorf("creating the package file: %w", err)
	}
	h := sha256.New()
	n, copyErr := io.Copy(io.MultiWriter(f, h), io.LimitReader(body, packageLimit+1))
	if closeErr := f.Close(); copyErr == nil {
		copyErr = closeErr
	}
	if copyErr == nil && n > packageLimit {
		copyErr = fmt.Errorf("the package is larger than %d bytes", packageLimit)
	}
	if copyErr == nil && hex.EncodeToString(h.Sum(nil)) != want {
		copyErr = ErrChecksum
	}
	if copyErr == nil {
		copyErr = os.Rename(part, dest)
	}
	if copyErr != nil {
		if rmErr := os.Remove(part); rmErr != nil && !errors.Is(rmErr, os.ErrNotExist) && u.Logger != nil {
			u.Logger.Warn("native self-update: removing a refused download failed", "path", part, "error", rmErr)
		}
		return copyErr
	}
	return nil
}

// get fetches one URL. An asset URL comes out of a response body, so it and every redirect hop must
// stay on GitHub; the release URL itself is configuration and is not held to that list.
func (u *Updater) get(ctx context.Context, raw, accept string, asset bool) (io.ReadCloser, error) {
	allowed := u.assetAllowed
	if allowed == nil {
		allowed = githubHost
	}
	parsed, err := url.Parse(raw)
	if err != nil || (asset && !allowed(parsed)) {
		return nil, fmt.Errorf("refusing download url %q", raw)
	}
	base := u.Client
	if base == nil {
		base = http.DefaultClient
	}
	client := &http.Client{
		Transport: base.Transport,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 5 || !allowed(req.URL) {
				return fmt.Errorf("refusing redirect to %s", req.URL.Host)
			}
			return nil
		},
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, parsed.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("building the request for %s: %w", raw, err)
	}
	req.Header.Set("Accept", accept)
	req.Header.Set("User-Agent", "McHarbor/"+appversion.Current())
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetching %s: %w", raw, err)
	}
	if resp.StatusCode != http.StatusOK {
		u.close(resp.Body)
		return nil, fmt.Errorf("fetching %s: status %d", raw, resp.StatusCode)
	}
	return resp.Body, nil
}

func githubHost(u *url.URL) bool {
	if u == nil || u.Scheme != "https" || u.Port() != "" {
		return false
	}
	host := strings.ToLower(u.Hostname())
	return host == "github.com" || strings.HasSuffix(host, ".github.com") || strings.HasSuffix(host, ".githubusercontent.com")
}

func (u *Updater) close(c io.Closer) {
	if err := c.Close(); err != nil && u.Logger != nil {
		u.Logger.Warn("native self-update: closing a response failed", "error", err)
	}
}

func (u *Updater) goosOr() string {
	if u.goos != "" {
		return u.goos
	}
	return runtime.GOOS
}

func (u *Updater) goarchOr() string {
	if u.goarch != "" {
		return u.goarch
	}
	return runtime.GOARCH
}

func (u *Updater) euidOr() int {
	if u.euid != nil {
		return u.euid()
	}
	return os.Geteuid()
}

func (u *Updater) lookPathOr(name string) (string, error) {
	if u.lookPath != nil {
		return u.lookPath(name)
	}
	return exec.LookPath(name)
}

func (u *Updater) runOr(ctx context.Context, name string, args ...string) ([]byte, error) {
	if u.run != nil {
		return u.run(ctx, name, args...)
	}
	ctx, cancel := context.WithTimeout(ctx, metadataTimeout)
	defer cancel()
	return exec.CommandContext(ctx, name, args...).CombinedOutput()
}

func (u *Updater) nowOr() time.Time {
	if u.now != nil {
		return u.now()
	}
	return time.Now()
}
