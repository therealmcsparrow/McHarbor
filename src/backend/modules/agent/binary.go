// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package agent

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/therealmcsparrow/mcharbor/core/i18n"
	"github.com/therealmcsparrow/mcharbor/core/response"
	appversion "github.com/therealmcsparrow/mcharbor/core/version"
	"github.com/therealmcsparrow/mcharbor/core/versionorder"
)

// agentBinaryDir is where the image puts the agent built alongside this host.
const agentBinaryDir = "agent-bin"

// localAgentDir is the repo's own agent/ folder, bind-mounted in. It holds
// whatever was last built by scripts/development/build-agent.sh and takes
// precedence over the binary baked into the image, so a rebuild is offered to
// connected agents without republishing anything.
const localAgentDir = "agent-local"

// localAgentBuildFile records the stamp the local binaries were built with.
// Written by the build script; a directory without it is still served, it just
// cannot take part in the build-stamp comparison.
const localAgentBuildFile = "BUILD"

// binaryPartPattern bounds the os and arch path segments.
//
// They are joined into a filename, so anything outside this alphabet is a path
// traversal attempt rather than a platform.
var binaryPartPattern = regexp.MustCompile(`^[a-z0-9_]{1,16}$`)

// AgentBinaryPath returns the on-disk path of the agent served for a platform,
// or "" when McHarbor has none.
//
// Precedence: the operator-selected target release from the release store,
// then the locally built agent-local/, then the image-bundled agent-bin/. The
// target wins because choosing it is an explicit act; the other two are
// whatever happened to be built last, and both are trusted as built-in.
//
//nolint:revive // used by internal/bootstrap outside this partition
func AgentBinaryPath(osName, arch string) string {
	return binaryPath(context.Background(), osName, arch)
}

// binaryPath is AgentBinaryPath with the caller's context bounding the release-store lookup.
func binaryPath(ctx context.Context, osName, arch string) string {
	if !binaryPartPattern.MatchString(osName) || !binaryPartPattern.MatchString(arch) {
		return ""
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if t := releaseTarget(); t != nil {
		if path, _, _ := t.TargetBinaryPath(ctx, osName, arch); path != "" {
			return path
		}
	}
	// The locally built folder wins over the image's copy. It is the one an
	// operator just rebuilt; the image's copy is whatever this release shipped with.
	for _, dir := range []string{localAgentDir, agentBinaryDir} {
		if path := usableBinary(dir, osName, arch); path != "" {
			return path
		}
	}
	return ""
}

// usableBinary returns the agent binary in dir for a platform, or "" when
// there is none worth serving.
func usableBinary(dir, osName, arch string) string {
	if name := usableBinaryName(dir, osName, arch); name != "" {
		return filepath.Join(dir, name)
	}
	return ""
}

// usableBinaryName is usableBinary's file name inside dir, so a caller can open
// it through os.OpenInRoot instead of joining a path.
func usableBinaryName(dir, osName, arch string) string {
	base := "mcharbor-agent-" + osName + "-" + arch
	names := []string{base}
	if osName == "windows" {
		// The build script and the agent's own self-update both name the
		// Windows asset with its extension; older layouts did not.
		names = []string{base + ".exe", base}
	}
	for _, name := range names {
		info, err := os.Stat(filepath.Join(dir, name))
		if err != nil || info.IsDir() || info.Size() < agentBinaryMinSize {
			continue
		}
		return name
	}
	return ""
}

// openBundled opens the agent shipped with this host (agent-local, then
// agent-bin), never a stored release, and returns it with its digest taken from
// the same handle, so the bytes served are the bytes hashed. The caller closes it.
func openBundled(osName, arch string) (*os.File, string, error) {
	if !binaryPartPattern.MatchString(osName) || !binaryPartPattern.MatchString(arch) {
		return nil, "", fmt.Errorf("no bundled agent for %q/%q", osName, arch)
	}
	for _, dir := range []string{localAgentDir, agentBinaryDir} {
		name := usableBinaryName(dir, osName, arch)
		if name == "" {
			continue
		}
		f, err := os.OpenInRoot(dir, name)
		if err != nil {
			slog.Warn("opening the bundled agent binary failed", "error", err)
			continue
		}
		h := sha256.New()
		if _, err := io.Copy(h, f); err != nil {
			closeErr := f.Close()
			return nil, "", errors.Join(fmt.Errorf("hashing the bundled agent: %w", err), closeErr)
		}
		if _, err := f.Seek(0, io.SeekStart); err != nil {
			closeErr := f.Close()
			return nil, "", errors.Join(fmt.Errorf("rewinding the bundled agent: %w", err), closeErr)
		}
		return f, hex.EncodeToString(h.Sum(nil)), nil
	}
	return nil, "", fmt.Errorf("no bundled agent for %s/%s", osName, arch)
}

// BundledBinaryDigest is the SHA-256 of the agent shipped with this host. An
// agent return advertises it for the host's own version, so it must come from
// the file HandleAgentBinaryVersion serves and not from the selected release.
func BundledBinaryDigest(osName, arch string) (string, error) {
	f, sum, err := openBundled(osName, arch)
	if err != nil {
		return "", err
	}
	if err := f.Close(); err != nil {
		slog.Warn("closing the bundled agent file failed", "error", err)
	}
	return sum, nil
}

// LocalAgentBuild reports the build stamp of the binaries in the local agent
// folder, or "" when there is no stamp to compare against.
//
// Read from disk on every call rather than cached: the whole point is that an
// operator rebuilds while McHarbor is running, and a cached answer would hide
// exactly the thing this exists to notice.
func LocalAgentBuild() string {
	data, err := os.ReadFile(filepath.Join(localAgentDir, localAgentBuildFile))
	if err != nil {
		return ""
	}
	stamp := strings.TrimSpace(string(data))
	if !buildStampPattern.MatchString(stamp) {
		return ""
	}
	return stamp
}

// buildStampPattern bounds a yyyyMMddHHmm stamp, so a stray file cannot make
// every agent look outdated.
var buildStampPattern = regexp.MustCompile(`^[0-9]{12}$`)

// HasLocalAgentBinary reports whether the local folder carries a binary for
// this platform, which is what makes a build-stamp comparison meaningful: an
// agent on a platform nothing was built for must never be called outdated.
func HasLocalAgentBinary(osName, arch string) bool {
	if !binaryPartPattern.MatchString(osName) || !binaryPartPattern.MatchString(arch) {
		return false
	}
	return usableBinary(localAgentDir, osName, arch) != ""
}

// agentBinaryMinSize guards against serving a truncated or placeholder file. A
// real agent is several megabytes.
const agentBinaryMinSize = 1 << 20

// HandleAgentBinary serves GET /api/agent/binary/{os}/{arch}.
//
// Public, like the install script beside it: the bytes are a signed public
// release or a built-in binary, a session would break `curl | sh` on a host
// with none, and agents older than this change download it without credentials.
// It serves only store, agent-local or agent-bin files under a name built from
// a bounded alphabet, so it cannot be walked out of its directory. The digest is
// sent as X-Checksum-Sha256 so the install script can check what arrived.
func (h *Handler) HandleAgentBinary(w http.ResponseWriter, r *http.Request) {
	osName := chi.URLParam(r, "os")
	arch := chi.URLParam(r, "arch")

	path := binaryPath(r.Context(), osName, arch)
	if path == "" {
		http.NotFound(w, r)
		return
	}

	if sum, err := FileSHA256(path); err == nil {
		w.Header().Set("X-Checksum-Sha256", sum)
	} else {
		h.app.Logger.Warn("agent: hashing served binary", "path", path, "error", err)
	}
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Disposition",
		`attachment; filename="mcharbor-agent-`+osName+`-`+arch+`"`)
	http.ServeFile(w, r, path)
}

// HandleAgentImage serves GET /api/agent/image/{os}/{arch}: the target
// release's agent image as a `docker save` tarball, for a container agent to
// load instead of pulling from a registry.
//
// Unlike the binary it requires the agent's own token as a bearer credential:
// only agents at AgentImageLoadMinBuild call it, all of which send one, so
// nothing forces it open.
func (h *Handler) HandleAgentImage(w http.ResponseWriter, r *http.Request) {
	token := strings.TrimSpace(strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer "))
	if token == "" || token == r.Header.Get("Authorization") {
		response.UnauthorizedCode(w, r, i18n.ErrUnauthorized)
		return
	}
	if _, err := h.service.ValidateAgentToken(r.Context(), token); err != nil {
		h.app.Logger.Warn("agent: image download with an invalid token", "error", err)
		response.UnauthorizedCode(w, r, i18n.ErrUnauthorized)
		return
	}
	osName := chi.URLParam(r, "os")
	arch := chi.URLParam(r, "arch")
	path, sum := imagePath(r.Context(), osName, arch)
	if path == "" {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("X-Checksum-Sha256", sum)
	w.Header().Set("Content-Type", "application/x-tar")
	w.Header().Set("Content-Disposition", `attachment; filename="mcharbor-agent-image-`+osName+`-`+arch+`.tar"`)
	http.ServeFile(w, r, path)
}

// imagePath returns the target release's image tarball for a platform and
// its digest, or "" when the store holds none. There is no built-in fallback:
// an image is only ever one McHarbor pulled and recorded itself.
func imagePath(ctx context.Context, osName, arch string) (path, sha string) {
	if !binaryPartPattern.MatchString(osName) || !binaryPartPattern.MatchString(arch) {
		return "", ""
	}
	store := releaseTarget()
	if store == nil {
		return "", ""
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	path, sha, _, _ = store.TargetImage(ctx, osName, arch)
	return path, sha
}

// FileSHA256 hashes a served file, so a download can be checked end to end.
func FileSHA256(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", fmt.Errorf("opening %s: %w", path, err)
	}
	defer func() {
		if err := f.Close(); err != nil {
			slog.Warn("closing served agent file failed", "error", err)
		}
	}()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", fmt.Errorf("hashing %s: %w", path, err)
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// servedImageRef is the reference the target release's agent image loads as,
// or "" when McHarbor holds no image. The manual install instructions use it in
// place of a registry reference.
func servedImageRef(ctx context.Context) string {
	store := releaseTarget()
	if store == nil {
		return ""
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	return store.TargetImageRef(ctx)
}

// versionedPattern is the allow-list for the {version} path segment: three
// numeric parts and an optional alphanumeric dot-separated prerelease, so "..",
// separators, "_" and a second "-" cannot reach a path.
var versionedPattern = regexp.MustCompile(`^[0-9]{1,4}\.[0-9]{1,3}\.[0-9]{1,4}(-[0-9A-Za-z]+(\.[0-9A-Za-z]+)*)?$`)

// maxServedVersionLen bounds the version segment before any pattern runs.
const maxServedVersionLen = 128

// validServedVersion accepts a version only when the allow-list and the strict
// parser the rest of McHarbor orders versions with both read it the same way.
func validServedVersion(v string) bool {
	if len(v) > maxServedVersionLen || !versionedPattern.MatchString(v) {
		return false
	}
	_, err := versionorder.Parse(v)
	return err == nil
}

// authorizeVersioned runs the checks both version-specific routes share and
// returns the validated path segments. The agent's bearer token must be valid,
// the platform and version must pass their allow-lists, and the token's own
// environment must be pinned to exactly this version, so a valid token for one
// agent never reads another agent's release or any other stored version.
func (h *Handler) authorizeVersioned(w http.ResponseWriter, r *http.Request) (osName, arch, version string, ok bool) {
	token := strings.TrimSpace(strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer "))
	if token == "" || token == r.Header.Get("Authorization") {
		response.UnauthorizedCode(w, r, i18n.ErrUnauthorized)
		return "", "", "", false
	}
	envID, err := h.service.ValidateAgentToken(r.Context(), token)
	if err != nil {
		h.app.Logger.Warn("agent: versioned download with an invalid token", "error", err)
		response.UnauthorizedCode(w, r, i18n.ErrUnauthorized)
		return "", "", "", false
	}
	osName, arch, version = chi.URLParam(r, "os"), chi.URLParam(r, "arch"), chi.URLParam(r, "version")
	if !binaryPartPattern.MatchString(osName) || !binaryPartPattern.MatchString(arch) || !validServedVersion(version) {
		http.NotFound(w, r)
		return "", "", "", false
	}
	pinned, err := pinnedReturnVersion(r.Context(), h.app.DB, envID)
	if err != nil {
		h.app.Logger.Error("agent: reading the pin for a versioned download", "env", envID, "error", err)
		response.InternalErrorCode(w, r, i18n.ErrInternalServer)
		return "", "", "", false
	}
	if pinned == "" || pinned != version {
		h.app.Logger.Warn("agent: versioned download refused, the agent is not pinned to this version", "env", envID, "version", version)
		response.ForbiddenCode(w, r, i18n.ErrForbidden)
		return "", "", "", false
	}
	return osName, arch, version, true
}

// HandleAgentBinaryVersion serves GET /api/agent/binary/{os}/{arch}/{version}:
// the binary of one exact version, for an agent returning to a stable release
// while a prerelease is the global target. Token-gated and limited to the
// agent pinned to that version, unlike the public latest-target route.
func (h *Handler) HandleAgentBinaryVersion(w http.ResponseWriter, r *http.Request) {
	osName, arch, version, ok := h.authorizeVersioned(w, r)
	if !ok {
		return
	}
	var f *os.File
	var sum string
	if store := versionedReleases(); store != nil {
		opened, digest, err := store.OpenReleaseAsset(r.Context(), false, osName, arch, version)
		if err == nil {
			f, sum = opened, digest
		}
	}
	if f == nil && version == appversion.Current() {
		// The agent bundled with this host is the one version McHarbor holds
		// outside the store.
		opened, digest, err := openBundled(osName, arch)
		if err != nil {
			h.app.Logger.Warn("agent: opening the bundled binary", "error", err)
		} else {
			f, sum = opened, digest
		}
	}
	if f == nil {
		http.NotFound(w, r)
		return
	}
	h.serveOpened(w, r, f, sum, "application/octet-stream", "mcharbor-agent-"+osName+"-"+arch)
}

// HandleAgentImageVersion serves GET /api/agent/image/{os}/{arch}/{version}:
// the `docker save` tarball of one exact stored version, under the same token
// and pin checks as the binary.
func (h *Handler) HandleAgentImageVersion(w http.ResponseWriter, r *http.Request) {
	osName, arch, version, ok := h.authorizeVersioned(w, r)
	if !ok {
		return
	}
	store := versionedReleases()
	if store == nil {
		http.NotFound(w, r)
		return
	}
	f, sum, err := store.OpenReleaseAsset(r.Context(), true, osName, arch, version)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	h.serveOpened(w, r, f, sum, "application/x-tar", "mcharbor-agent-image-"+osName+"-"+arch+".tar")
}

// serveOpened streams f with its digest header and closes it.
func (h *Handler) serveOpened(w http.ResponseWriter, r *http.Request, f *os.File, sum, contentType, name string) {
	defer func() {
		if err := f.Close(); err != nil {
			h.app.Logger.Warn("agent: closing a served file", "error", err)
		}
	}()
	info, err := f.Stat()
	if err != nil {
		h.app.Logger.Error("agent: stat of a served file", "error", err)
		response.InternalErrorCode(w, r, i18n.ErrInternalServer)
		return
	}
	if sum != "" {
		w.Header().Set("X-Checksum-Sha256", sum)
	}
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Content-Disposition", `attachment; filename="`+name+`"`)
	http.ServeContent(w, r, name, info.ModTime(), f)
}
