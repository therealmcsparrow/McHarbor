// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

import (
	"context"
	"fmt"
	"net/http"
	"path"
	"regexp"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/rs/xid"
)

// Share is one virtiofs directory handed to a guest.
type Share struct {
	ID        string    `json:"id"`
	VMID      string    `json:"vmId"`
	Tag       string    `json:"tag"`
	HostPath  string    `json:"hostPath"`
	ReadOnly  bool      `json:"readOnly"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`

	// MountCommand is the line to run inside the guest. It is built here
	// rather than in the UI because the syntax is the one thing an operator
	// reliably gets wrong, and it is the same on every guest.
	MountCommand string `json:"mountCommand,omitempty"`
}

// shareTagPattern is what a guest can actually mount by.
//
// virtiofs tags travel to the guest as a device name; anything with a slash, a
// space or a shell metacharacter in it either fails to mount or mounts
// something other than what was meant.
var shareTagPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,35}$`)

// shareRootsSetting names the per-host setting listing the directories an
// operator is willing to share out.
const shareRootsSetting = "share_roots"

// listShares returns a guest's shares, oldest first.
func (s *Service) listShares(ctx context.Context, vmID string) ([]Share, error) {
	rows, err := s.db.Query(ctx, `
		SELECT id, vm_id, tag, host_path, read_only, created_at, updated_at
		  FROM plugin_addon_kvm_shares
		 WHERE vm_id = ?
		 ORDER BY created_at
		 LIMIT 1000`, vmID)
	if err != nil {
		return nil, fmt.Errorf("listing shares: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var out []Share
	for rows.Next() {
		var share Share
		var readOnly int
		var created, updated string
		if err := rows.Scan(&share.ID, &share.VMID, &share.Tag, &share.HostPath,
			&readOnly, &created, &updated); err != nil {
			return nil, fmt.Errorf("scanning a share: %w", err)
		}
		share.ReadOnly = readOnly == 1
		share.CreatedAt = parseTime(created)
		share.UpdatedAt = parseTime(updated)
		share.MountCommand = mountCommandFor(share.Tag)
		out = append(out, share)
	}
	return out, rows.Err()
}

// mountCommandFor is the line an operator runs inside the guest.
func mountCommandFor(tag string) string {
	return fmt.Sprintf("mount -t virtiofs %s /mnt/%s", tag, tag)
}

// ShareRequest is a new share.
type ShareRequest struct {
	Tag      string `json:"tag"`
	HostPath string `json:"hostPath"`
	ReadOnly bool   `json:"readOnly"`
}

// AddShare records a virtiofs share for a guest.
//
// The host path is checked against the operator's configured roots here, on
// the server, because this is the security boundary the whole feature turns
// on: a share of / or /etc hands the host to the guest, and the guest is the
// untrusted party. The agent re-checks that the path is a directory, but it
// cannot know which roots this installation permits.
func (s *Service) AddShare(ctx context.Context, vmID string, req ShareRequest) (*Share, error) {
	vm, err := s.Get(ctx, vmID)
	if err != nil {
		return nil, err
	}

	tag := strings.TrimSpace(req.Tag)
	if !shareTagPattern.MatchString(tag) {
		return nil, fmt.Errorf("%w: a mount tag is letters, digits, dot, dash or underscore",
			ErrInvalidResize)
	}
	hostPath, err := s.validateSharePath(ctx, vm.EnvironmentID, req.HostPath)
	if err != nil {
		return nil, err
	}

	existing, err := s.listShares(ctx, vmID)
	if err != nil {
		return nil, err
	}
	for _, share := range existing {
		if strings.EqualFold(share.Tag, tag) {
			return nil, ErrShareTagTaken
		}
	}

	now := time.Now().UTC().Format(time.RFC3339)
	share := Share{
		ID: xid.New().String(), VMID: vmID, Tag: tag,
		HostPath: hostPath, ReadOnly: req.ReadOnly,
	}
	if _, err := s.db.Exec(ctx, `
		INSERT INTO plugin_addon_kvm_shares
		       (id, vm_id, tag, host_path, read_only, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)`,
		share.ID, share.VMID, share.Tag, share.HostPath,
		boolToInt(share.ReadOnly), now, now); err != nil {
		return nil, fmt.Errorf("recording the share: %w", err)
	}

	share.CreatedAt = parseTime(now)
	share.UpdatedAt = share.CreatedAt
	share.MountCommand = mountCommandFor(share.Tag)
	return &share, nil
}

// RemoveShare drops a share.
//
// It takes effect at the guest's next start: detaching a vhost-user device
// from a live guest whose filesystem is mounted would hang every process
// inside it holding a file open.
func (s *Service) RemoveShare(ctx context.Context, vmID, shareID string) error {
	affected, err := s.db.Exec(ctx,
		`DELETE FROM plugin_addon_kvm_shares WHERE id = ? AND vm_id = ?`, shareID, vmID)
	if err != nil {
		return fmt.Errorf("removing the share: %w", err)
	}
	if affected == 0 {
		return ErrNotFound
	}
	return nil
}

// validateSharePath refuses anything outside the operator's allowed roots.
//
// The default when nothing is configured is to refuse everything rather than
// to allow everything: an unconfigured installation that silently permitted
// `/` would be the worst possible default for a feature whose failure mode is
// handing the host to a guest.
func (s *Service) validateSharePath(ctx context.Context, envID, raw string) (string, error) {
	hostPath := path.Clean(strings.TrimSpace(raw))
	if hostPath == "" || !strings.HasPrefix(hostPath, "/") {
		return "", fmt.Errorf("%w: a shared path must be absolute", ErrInvalidResize)
	}
	// Cleaning resolves `..` textually, so this cannot be reached by a path
	// that merely looks traversing — but a symlink inside an allowed root can
	// still point elsewhere, which is why the daemon also runs sandboxed.
	if strings.Contains(hostPath, "..") {
		return "", ErrSharePathRefused
	}

	roots := s.shareRoots(ctx, envID)
	if len(roots) == 0 {
		return "", fmt.Errorf("%w: no share roots are configured for this host", ErrSharePathRefused)
	}
	for _, root := range roots {
		root = path.Clean(root)
		if hostPath == root || strings.HasPrefix(hostPath, root+"/") {
			return hostPath, nil
		}
	}
	return "", ErrSharePathRefused
}

// shareRoots reads the directories this host is willing to share out.
func (s *Service) shareRoots(ctx context.Context, envID string) []string {
	raw, err := s.setting(ctx, envID, shareRootsSetting)
	if err != nil || strings.TrimSpace(raw) == "" {
		return nil
	}
	var roots []string
	for _, entry := range strings.Split(raw, "\n") {
		entry = strings.TrimSpace(entry)
		if entry != "" && strings.HasPrefix(entry, "/") {
			roots = append(roots, entry)
		}
	}
	return roots
}

// setting reads one per-host setting, empty when it has never been set.
func (s *Service) setting(ctx context.Context, envID, key string) (string, error) {
	rows, err := s.db.Query(ctx, `
		SELECT value FROM plugin_addon_kvm_host_settings
		 WHERE environment_id = ? AND key = ?`, envID, key)
	if err != nil {
		return "", fmt.Errorf("reading host setting %s: %w", key, err)
	}
	defer func() { _ = rows.Close() }()

	if !rows.Next() {
		return "", rows.Err()
	}
	var value string
	if err := rows.Scan(&value); err != nil {
		return "", fmt.Errorf("reading host setting %s: %w", key, err)
	}
	return value, rows.Err()
}

// setSetting writes one per-host setting.
func (s *Service) setSetting(ctx context.Context, envID, key, value string) error {
	now := time.Now().UTC().Format(time.RFC3339)
	if _, err := s.db.Exec(ctx, `
		INSERT INTO plugin_addon_kvm_host_settings
		       (environment_id, key, value, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?)
		ON CONFLICT(environment_id, key)
		DO UPDATE SET value = excluded.value, updated_at = excluded.updated_at`,
		envID, key, value, now, now); err != nil {
		return fmt.Errorf("saving host setting %s: %w", key, err)
	}
	return nil
}

// HandleGetShareRoots serves GET /api/vms/share-roots.
func (h *Handler) HandleGetShareRoots(w http.ResponseWriter, r *http.Request) {
	envID := strings.TrimSpace(r.URL.Query().Get("env"))
	if envID == "" {
		fail(w, r, http.StatusBadRequest, codeBadRequest)
		return
	}

	roots := h.svc.shareRoots(r.Context(), envID)
	if roots == nil {
		roots = []string{}
	}
	ok(w, map[string]any{"roots": roots})
}

// HandleSetShareRoots serves PUT /api/vms/share-roots.
//
// Changing this widens what any guest on this host can be given, so it is
// gated on vm.privileged rather than vm.manage — the same key as device
// passthrough, for the same reason.
func (h *Handler) HandleSetShareRoots(w http.ResponseWriter, r *http.Request) {
	envID := strings.TrimSpace(r.URL.Query().Get("env"))
	if envID == "" {
		fail(w, r, http.StatusBadRequest, codeBadRequest)
		return
	}

	var req struct {
		Roots []string `json:"roots"`
	}
	if err := decode(r, &req); err != nil {
		failErr(w, r, http.StatusBadRequest, codeBadRequest, err)
		return
	}

	var cleaned []string
	for _, root := range req.Roots {
		root = path.Clean(strings.TrimSpace(root))
		if root == "" || !strings.HasPrefix(root, "/") {
			continue
		}
		// "/" is refused outright rather than merely discouraged: allowing it
		// would make every later path check pass and turn the whole allow-list
		// into decoration.
		if root == "/" || strings.Contains(root, "..") {
			fail(w, r, http.StatusBadRequest, codeSharePathRefused)
			return
		}
		cleaned = append(cleaned, root)
	}

	if err := h.svc.setSetting(r.Context(), envID,
		shareRootsSetting, strings.Join(cleaned, "\n")); err != nil {
		h.respondErr(w, r, "save share roots", err)
		return
	}
	ok(w, map[string]any{"roots": cleaned})
}

// HandleListShares serves GET /api/vms/{id}/shares.
func (h *Handler) HandleListShares(w http.ResponseWriter, r *http.Request) {
	shares, err := h.svc.listShares(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		h.respondErr(w, r, "list shares", err)
		return
	}
	if shares == nil {
		shares = []Share{}
	}
	ok(w, shares)
}

// HandleAddShare serves POST /api/vms/{id}/shares.
func (h *Handler) HandleAddShare(w http.ResponseWriter, r *http.Request) {
	var req ShareRequest
	if err := decode(r, &req); err != nil {
		failErr(w, r, http.StatusBadRequest, codeBadRequest, err)
		return
	}

	share, err := h.svc.AddShare(r.Context(), chi.URLParam(r, "id"), req)
	if err != nil {
		h.respondErr(w, r, "add share", err)
		return
	}
	okData(w, share, msgShareAdded)
}

// HandleRemoveShare serves DELETE /api/vms/{id}/shares/{shareId}.
func (h *Handler) HandleRemoveShare(w http.ResponseWriter, r *http.Request) {
	if err := h.svc.RemoveShare(r.Context(),
		chi.URLParam(r, "id"), chi.URLParam(r, "shareId")); err != nil {
		h.respondErr(w, r, "remove share", err)
		return
	}
	okMsg(w, msgShareRemoved)
}
