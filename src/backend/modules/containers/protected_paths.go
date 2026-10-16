// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package containers

import (
	"context"
	"errors"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"
	"sync"
	"time"

	cerrdefs "github.com/containerd/errdefs"
	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/client"

	"github.com/therealmcsparrow/mcharbor/core/auth"
	coredocker "github.com/therealmcsparrow/mcharbor/core/docker"
	"github.com/therealmcsparrow/mcharbor/core/i18n"
	"github.com/therealmcsparrow/mcharbor/core/rbac"
	"github.com/therealmcsparrow/mcharbor/core/response"
	"github.com/therealmcsparrow/mcharbor/internal/closeutil"
)

// keyFileNames are the file names McHarbor keeps its own keys in: the data-directory secret
// and key, and the Docker secrets the _FILE variables conventionally point at. They are
// refused on every container, because a peer node or a sidecar can hold a copy.
var keyFileNames = map[string]bool{
	".secret":                 true,
	".encryption_key":         true,
	"mcharbor_backup_key":     true,
	"mcharbor_secret":         true,
	"mcharbor_encryption_key": true,
	"mcharbor_db_dsn":         true,
}

// dockerSecretsDir is where Docker and Compose mount secrets inside a container.
const dockerSecretsDir = "/run/secrets"

// maxSymlinkHops bounds path resolution, as the kernel's own ELOOP limit does.
const maxSymlinkHops = 40

// dataSourceCacheTTL is how long McHarbor's own data-volume identity is trusted before it
// is read again; a recreate with a different volume is picked up within this window.
const dataSourceCacheTTL = 5 * time.Minute

// pathVerdict is what the file-browser guard decided about one path.
type pathVerdict int

const (
	pathAllowed pathVerdict = iota
	pathNeedsSystemManage
	pathKeyFile
)

// isKeyFilePath reports whether cleaned names one of McHarbor's key files, by name or as one
// of the _FILE paths this process was configured with.
func (h *Handler) isKeyFilePath(cleaned string) bool {
	if keyFileNames[path.Base(cleaned)] {
		return true
	}
	for _, configured := range h.configuredKeyFiles() {
		if cleaned == configured {
			return true
		}
	}
	return false
}

// configuredKeyFiles returns the cleaned _FILE paths this process read its keys from.
func (h *Handler) configuredKeyFiles() []string {
	cfg := h.app.Config
	if cfg == nil {
		return nil
	}
	var out []string
	for _, p := range []string{cfg.SecretFile, cfg.EncryptionKeyFile, cfg.DBDSNFile, cfg.BackupEncryptionKeyFile} {
		if strings.TrimSpace(p) != "" {
			out = append(out, cleanContainerPath(filepath.ToSlash(p)))
		}
	}
	return out
}

// within reports whether p is root itself or anything below it.
func within(p, root string) bool {
	if root == "" {
		return false
	}
	if root == "/" {
		return true
	}
	return p == root || strings.HasPrefix(p, root+"/")
}

// protectedRoots lists the directories of info that only system.manage may browse: on a
// McHarbor container its data directory, the directories of its _FILE secrets and
// /run/secrets; on any container, every mount of McHarbor's own data volume.
func (h *Handler) protectedRoots(ctx context.Context, info container.InspectResponse) []string {
	var roots []string
	if coredocker.IsMcHarborInstance(info) {
		roots = append(roots, dockerSecretsDir)
		if cfg := h.app.Config; cfg != nil && strings.TrimSpace(cfg.DataDir) != "" {
			roots = append(roots, cleanContainerPath(filepath.ToSlash(cfg.DataDir)))
		}
		for _, f := range h.configuredKeyFiles() {
			roots = append(roots, path.Dir(f))
		}
	}
	sources := h.dataSources(ctx)
	if len(sources) == 0 {
		return roots
	}
	for _, m := range info.Mounts {
		for _, src := range sources {
			if src != "" && (m.Name == src || m.Source == src) {
				roots = append(roots, cleanContainerPath(m.Destination))
				break
			}
		}
	}
	return roots
}

// verdict classifies one resolved path against the protected roots of its container.
func (h *Handler) verdict(p string, roots []string) pathVerdict {
	if h.isKeyFilePath(p) {
		return pathKeyFile
	}
	if within(p, "/proc") {
		return pathNeedsSystemManage
	}
	for _, root := range roots {
		if within(p, root) {
			return pathNeedsSystemManage
		}
	}
	return pathAllowed
}

// guardProtectedPath answers a file-browser request itself when p reaches a McHarbor key
// file (refused for everybody), /proc, McHarbor's data directory or secrets, or a mount of
// its data volume without system.manage, and audits every refusal and every permitted
// system.manage access. A true return means the handler may continue.
func (h *Handler) guardProtectedPath(w http.ResponseWriter, r *http.Request, envID, id, action, p string) bool {
	cleaned := cleanContainerPath(p)
	if h.isKeyFilePath(cleaned) {
		h.logProtectedPathAudit(r, envID, id, action+"_denied", cleaned, "key_file")
		response.ForbiddenCode(w, r, i18n.ErrKeyFilePathForbidden)
		return false
	}
	info, err := h.svc.Inspect(r.Context(), envID, id)
	if err != nil {
		if cerrdefs.IsNotFound(err) {
			response.NotFoundCode(w, r, i18n.ErrContainerNotFound)
			return false
		}
		// Unresolvable target: refuse rather than open a path whose owner is unknown.
		h.app.Logger.Warn("inspecting a container before a file-browser access failed", "env", envID, "id", id, "error", err)
		response.ForbiddenCode(w, r, i18n.ErrProtectedPathForbidden)
		return false
	}
	roots := h.protectedRoots(r.Context(), info)
	paths := []string{cleaned}
	if len(roots) > 0 {
		// A symlink in a parent directory is followed by the Docker archive API, so a
		// sensitive container's path is judged by where it really lands as well.
		resolved, resolveErr := h.svc.resolveContainerPath(r.Context(), envID, info.ID, cleaned)
		if resolveErr != nil {
			h.app.Logger.Warn("resolving a file-browser path failed", "env", envID, "id", id, "error", resolveErr)
			h.logProtectedPathAudit(r, envID, id, action+"_denied", cleaned, "unresolved")
			response.ForbiddenCode(w, r, i18n.ErrProtectedPathForbidden)
			return false
		}
		if resolved != cleaned {
			paths = append(paths, resolved)
		}
	}
	worst := pathAllowed
	for _, candidate := range paths {
		if v := h.verdict(candidate, roots); v > worst {
			worst = v
		}
	}
	switch worst {
	case pathKeyFile:
		h.logProtectedPathAudit(r, envID, id, action+"_denied", cleaned, "key_file")
		response.ForbiddenCode(w, r, i18n.ErrKeyFilePathForbidden)
		return false
	case pathNeedsSystemManage:
		if !h.holdsSystemManage(r) {
			h.logProtectedPathAudit(r, envID, id, action+"_denied", cleaned, "system_manage_required")
			response.ForbiddenCode(w, r, i18n.ErrProtectedPathForbidden)
			return false
		}
		h.logProtectedPathAudit(r, envID, id, action, cleaned, "system_manage")
	}
	return true
}

// holdsSystemManage mirrors the terminal's rule for a protected container's shell: the
// global system.manage grant, failing closed on any lookup error.
func (h *Handler) holdsSystemManage(r *http.Request) bool {
	user := auth.RequireAuth(r)
	if user == nil {
		return false
	}
	if user.ID == "system" {
		return true
	}
	if h.app.RBACService == nil {
		return false
	}
	allowed, err := h.app.RBACService.HasPermission(r.Context(), user.ID, "", rbac.PermSystemManage)
	if err != nil {
		h.app.Logger.Error("checking system.manage for the file browser failed", "user", user.ID, "error", err)
		return false
	}
	if !allowed {
		rbac.RecordDenied(r, user, rbac.PermSystemManage, "")
	}
	return allowed
}

// logProtectedPathAudit records one file-browser access to a protected path at warning.
func (h *Handler) logProtectedPathAudit(r *http.Request, envID, id, action, p, reason string) {
	if h.app.AuditLog == nil {
		return
	}
	h.logContainerAuditSeverity(r, envID, action, id, "warning", "path="+p+" protected="+reason)
}

// dataSources returns the volume names and host paths behind McHarbor's own data
// directory, so a container that mounts them is treated as holding McHarbor's database.
func (h *Handler) dataSources(ctx context.Context) []string {
	if h.dataSourceFn != nil {
		return h.dataSourceFn(ctx)
	}
	if h.app.Config == nil {
		return nil
	}
	return h.dataCache.get(ctx, h.app.Config.DataDir)
}

// dataSourceCache remembers McHarbor's data-volume identity for dataSourceCacheTTL.
type dataSourceCache struct {
	mu      sync.Mutex
	at      time.Time
	sources []string
}

func (c *dataSourceCache) get(ctx context.Context, dataDir string) []string {
	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.at.IsZero() && time.Since(c.at) < dataSourceCacheTTL {
		return c.sources
	}
	c.sources = lookupDataSources(ctx, dataDir)
	c.at = time.Now()
	return c.sources
}

// lookupDataSources reads the mount behind the data directory from McHarbor's own
// container on the local daemon; running outside a container, the host path itself is the
// source a sidecar would bind.
func lookupDataSources(ctx context.Context, dataDir string) []string {
	dataDir = strings.TrimSpace(dataDir)
	if dataDir == "" {
		return nil
	}
	cli, err := client.New(client.FromEnv)
	if err != nil {
		return nil
	}
	defer func() { closeutil.Report(cli.Close(), "cli") }()
	lookupCtx, cancel := context.WithTimeout(ctx, coredocker.CallTimeout)
	defer cancel()
	self, err := coredocker.CurrentMcHarborContainer(lookupCtx, cli)
	if err != nil {
		if abs, absErr := filepath.Abs(dataDir); absErr == nil {
			return []string{filepath.ToSlash(abs)}
		}
		return nil
	}
	var out []string
	for _, m := range self.Mounts {
		if cleanContainerPath(m.Destination) != cleanContainerPath(filepath.ToSlash(dataDir)) {
			continue
		}
		for _, v := range []string{m.Name, m.Source} {
			if v != "" {
				out = append(out, v)
			}
		}
	}
	return out
}

// errSymlinkLoop is returned when a path's symlinks do not settle within maxSymlinkHops.
var errSymlinkLoop = errors.New("too many symbolic links")

// resolveContainerPath follows the symlinks in p inside the container, component by
// component, the way the Docker archive API does. Components that do not exist yet (a file
// about to be written) are kept as given.
func (s *Service) resolveContainerPath(ctx context.Context, envID, id, p string) (string, error) {
	cli, err := s.getClient(ctx, envID)
	if err != nil {
		return "", err
	}
	ctx, cancel := context.WithTimeout(ctx, coredocker.CallTimeout)
	defer cancel()

	pending := strings.Split(strings.TrimPrefix(cleanContainerPath(p), "/"), "/")
	resolved := "/"
	hops := 0
	for len(pending) > 0 {
		part := pending[0]
		pending = pending[1:]
		if part == "" || part == "." {
			continue
		}
		next := path.Join(resolved, part)
		res, statErr := cli.ContainerStatPath(ctx, id, client.ContainerStatPathOptions{Path: next})
		if statErr != nil {
			if cerrdefs.IsNotFound(statErr) {
				return path.Join(append([]string{next}, pending...)...), nil
			}
			return "", statErr
		}
		if res.Stat.Mode&os.ModeSymlink == 0 || res.Stat.LinkTarget == "" {
			resolved = next
			continue
		}
		hops++
		if hops > maxSymlinkHops {
			return "", errSymlinkLoop
		}
		target := res.Stat.LinkTarget
		if !strings.HasPrefix(target, "/") {
			target = path.Join(resolved, target)
		}
		pending = append(strings.Split(strings.TrimPrefix(path.Clean(target), "/"), "/"), pending...)
		resolved = "/"
	}
	return resolved, nil
}
