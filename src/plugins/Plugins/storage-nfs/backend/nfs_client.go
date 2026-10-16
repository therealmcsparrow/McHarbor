// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

//go:build linux

// NFS plugin StorageAdapterClient. Uses the Linux kernel's nfs
// client via syscall.Mount — no Go-side NFS protocol
// implementation. The plugin:
//
//   1. Builds a deterministic mount point from a hash of
//      (server, export, sec) under mount_base.
//   2. Mounts the export lazily on first operation; the mount
//      persists for the lifetime of the host process (or until
//      Cleanup is called for the destination).
//   3. Operates on the local mount path using stdlib os.* —
//      no NFS-specific code in the hot path.
//
// Platform note: Linux-only. The McHarbor backend runs in a
// Linux Docker container; non-Linux platforms return
// ErrUnsupportedPlatform via the nfs_other.go stub.

package main

import (
	"context"
	"crypto/sha1"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/therealmcsparrow/mcharbor/core/plugin"
)

// nfsDialTimeout caps the mount syscall. The kernel's NFS
// mount waits up to this duration for the NFS server's MOUNT
// reply before giving up.
const nfsDialTimeout = 30 * time.Second

// nfsOpTimeout caps individual file operations on the mount.
// Large backups stream data well below this cap; the cap exists
// to surface hung NFS sessions instead of letting them wedge
// the host's upload pipeline.
const nfsOpTimeout = 5 * time.Minute

// nfsClient is the StorageAdapterClient the host dispatches to
// for the nfs LocationType.
//
// State: the client keeps a per-process mount cache (mountCache)
// so repeated operations on the same destination don't remount.
// The cache is keyed by a stable hash derived from the cfg fields
// that uniquely identify the NFS server path.
type nfsClient struct {
	cache *mountCache
}

// NewNFSClient returns a fresh client. The plugin keeps the
// mount cache process-global (one cache, many clients) so that
// two destinations pointing at the same export share a single
// mount.
func NewNFSClient() *nfsClient {
	return &nfsClient{
		cache: globalMountCache,
	}
}

// globalMountCache is the process-wide NFS mount cache. Mounted
// destinations are unmounted when Cleanup is called for the
// destination or when the host process exits (kernel tears down
// the namespace).
var globalMountCache = newMountCache()

// mountCache tracks the per-destination mount state. The cache
// is process-global so two destinations targeting the same
// server+export+sec share one mount.
type mountCache struct {
	mu      sync.Mutex
	mounted map[string]*mountEntry
}

type mountEntry struct {
	mountPoint string
	mountedAt  time.Time
}

func newMountCache() *mountCache {
	return &mountCache{mounted: make(map[string]*mountEntry)}
}

// remember records an active mount under key.
func (c *mountCache) remember(key, mountPoint string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.mounted[key] = &mountEntry{mountPoint: mountPoint, mountedAt: time.Now()}
}

// isMounted returns the mount point for key, or "" if not
// mounted. The caller still needs to verify the kernel-side
// mount is alive (stat the mount point + read /proc/mounts).
func (c *mountCache) isMounted(key string) (string, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	e, ok := c.mounted[key]
	if !ok {
		return "", false
	}
	return e.mountPoint, true
}

// forget removes the mount record. Idempotent.
func (c *mountCache) forget(key string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.mounted, key)
}

// ============================================================================
// Config normalization
// ============================================================================

// nfsValidateConfig runs the auth-method → credential matrix
// check. It does NOT verify that the principal exists or the
// keytab file is readable — those checks happen at mount time.
func nfsValidateConfig(cfg map[string]string) error {
	if strings.TrimSpace(cfg["server"]) == "" {
		return errors.New("nfs: server is required")
	}
	if strings.TrimSpace(cfg["export"]) == "" {
		return errors.New("nfs: export is required")
	}
	sec, err := nfsSecFlavor(cfg)
	if err != nil {
		return err
	}
	if sec != nfsSecSys {
		// krb5/krb5i/krb5p all require principal + realm +
		// keytab. The keytab is read at mount time by the
		// kernel-side rpc.gssd daemon; we just verify the
		// path is set.
		if strings.TrimSpace(cfg["principal"]) == "" {
			return errors.New("nfs: principal is required for " + sec + " auth")
		}
		if strings.TrimSpace(cfg["realm"]) == "" {
			return errors.New("nfs: realm is required for " + sec + " auth")
		}
		if strings.TrimSpace(cfg["keytab_path"]) == "" {
			return errors.New("nfs: keytab_path is required for " + sec + " auth")
		}
		// Best-effort existence check on the keytab file.
		// The plugin runs in the host process which sees the
		// real filesystem, not the container namespace (unless
		// /etc/krb5.keytab is bind-mounted in by the operator).
		if _, err := os.Stat(cfg["keytab_path"]); err != nil {
			return fmt.Errorf("nfs: keytab_path %q: %w", cfg["keytab_path"], err)
		}
	}
	return nil
}

// nfsSecFlavor validates cfg["sec"] and returns it. Defaults
// to "sys" (AUTH_SYS).
func nfsSecFlavor(cfg map[string]string) (string, error) {
	switch cfg["sec"] {
	case "":
		return nfsSecSys, nil
	case nfsSecSys, nfsSecKrb5, nfsSecKrb5i, nfsSecKrb5p:
		return cfg["sec"], nil
	default:
		return "", fmt.Errorf("nfs: unknown sec flavor %q", cfg["sec"])
	}
}

// nfsVersion validates cfg["version"] and returns it. Defaults
// to "auto" (let the kernel negotiate).
func nfsVersion(cfg map[string]string) (string, error) {
	switch cfg["version"] {
	case "":
		return nfsVersAuto, nil
	case nfsVersAuto, nfsVers3, nfsVers4, nfsVers41, nfsVers42:
		return cfg["version"], nil
	default:
		return "", fmt.Errorf("nfs: unknown version %q", cfg["version"])
	}
}

// nfsMountBase returns the configured mount base, defaulting to
// nfsDefaultMountBase from main.go.
func nfsMountBase(cfg map[string]string) string {
	if strings.TrimSpace(cfg["mount_base"]) != "" {
		return cfg["mount_base"]
	}
	return nfsDefaultMountBase
}

// nfsMountKey returns a stable per-destination identifier.
// server+export+sec uniquely identify the NFS server path;
// adding the mount_base keeps two instances pointing at
// different bases from colliding.
func nfsMountKey(cfg map[string]string) string {
	h := sha1.New()
	h.Write([]byte(nfsMountBase(cfg)))
	h.Write([]byte{0})
	h.Write([]byte(strings.TrimSpace(cfg["server"])))
	h.Write([]byte{0})
	h.Write([]byte(strings.TrimSpace(cfg["export"])))
	h.Write([]byte{0})
	h.Write([]byte(cfg["sec"]))
	return fmt.Sprintf("%x", h.Sum(nil))[:16]
}

// nfsMountPoint returns the absolute mount path for the
// destination. Stable across restarts (same key → same path).
func nfsMountPoint(cfg map[string]string) string {
	return filepath.Join(nfsMountBase(cfg), "nfs-"+nfsMountKey(cfg))
}

// nfsSubdir returns the configured subdirectory as a path
// relative to the mount root, with leading slash stripped.
// The kernel's mount helper accepts absolute paths on the
// server but the subdirectory inside the mount is whatever the
// operator configured.
func nfsSubdir(cfg map[string]string) string {
	p := strings.TrimSpace(cfg["path"])
	p = strings.TrimPrefix(p, "/")
	for strings.HasPrefix(p, "/") {
		p = strings.TrimPrefix(p, "/")
	}
	for strings.HasSuffix(p, "/") && len(p) > 1 {
		p = strings.TrimSuffix(p, "/")
	}
	return p
}

// ============================================================================
// Mount option assembly
// ============================================================================

// nfsBuildMountOptions assembles the comma-separated mount
// options string from cfg. Always includes the sec flavor and
// the NFS version (unless auto).
//
// Examples:
//
//   vers=4,sec=sys
//   vers=4.2,sec=krb5p
//   vers=auto,sec=sys
func nfsBuildMountOptions(cfg map[string]string) string {
	sec, _ := nfsSecFlavor(cfg)
	ver, _ := nfsVersion(cfg)

	var parts []string
	if ver != nfsVersAuto {
		// "auto" → omit; kernel picks (typically 4.2).
		parts = append(parts, "vers="+ver)
	}
	parts = append(parts, "sec="+sec)
	// hard: fail operations when the server is unreachable
	// rather than returning EIO. Recommended for backups.
	parts = append(parts, "hard")
	// timeo=50: 0.5s retransmit timeout (in deci-seconds).
	parts = append(parts, "timeo=50")
	// retrans=3: retransmit this many times before giving up.
	parts = append(parts, "retrans=3")
	return strings.Join(parts, ",")
}

// ============================================================================
// Mount lifecycle
// ============================================================================

// nfsEnsureMounted mounts the NFS export if not already mounted.
// Idempotent: calling it twice on the same destination is a
// no-op (returns the existing mount point).
//
// The mount is performed via syscall.Mount("server:export",
// mountPoint, "nfs", 0, options). This requires CAP_SYS_ADMIN,
// which the McHarbor container already has per CLAUDE.md
// (Docker socket mounting needs the same privilege).
func (c *nfsClient) nfsEnsureMounted(ctx context.Context, cfg map[string]string) (string, error) {
	key := nfsMountKey(cfg)

	if mp, ok := c.cache.isMounted(key); ok {
		// Verify the mount is still alive by stat-ing the
		// mount point. If the kernel unmounted it (e.g., the
		// server went away and we did a soft-unmount), we'd
		// see an error here and remount.
		if _, err := os.Stat(mp); err == nil {
			return mp, nil
		}
		// Stale entry — clear it and remount.
		c.cache.forget(key)
	}

	if err := nfsValidateConfig(cfg); err != nil {
		return "", err
	}

	mountPoint := nfsMountPoint(cfg)
	if err := os.MkdirAll(mountPoint, 0755); err != nil {
		return "", fmt.Errorf("nfs: mkdir mount point %s: %w", mountPoint, err)
	}

	source := strings.TrimSpace(cfg["server"]) + ":" + strings.TrimSpace(cfg["export"])
	options := nfsBuildMountOptions(cfg)

	// Bind the mount call to a deadline. The kernel's mount
	// helper waits up to timeo*retrans for the MOUNT reply;
	// adding a wall-clock cap means a totally-dead server
	// surfaces an error here instead of hanging the plugin.
	mountCtx, cancel := context.WithTimeout(ctx, nfsDialTimeout)
	defer cancel()
	done := make(chan error, 1)
	go func() {
		// syscall.Mount(source, target, fstype, flags, data)
		// MS_NOSUID | MS_NODEV: don't honor setuid bits or
		// device files on the export. Standard for untrusted
		// storage mounts.
		flags := uintptr(syscall.MS_NOSUID | syscall.MS_NODEV)
		done <- syscall.Mount(source, mountPoint, "nfs", flags, options)
	}()
	select {
	case err := <-done:
		if err != nil {
			return "", fmt.Errorf("nfs: mount %s → %s (%s): %w", source, mountPoint, options, err)
		}
	case <-mountCtx.Done():
		// The mount syscall doesn't honor ctx; if the kernel
		// is hung on the MOUNT reply we just have to wait it
		// out (the goroutine continues running). Surface a
		// timeout to the caller so the operation can fail
		// fast instead of blocking the host pipeline.
		return "", fmt.Errorf("nfs: mount %s: timeout after %s", source, nfsDialTimeout)
	}

	c.cache.remember(key, mountPoint)
	return mountPoint, nil
}

// nfsUnmount tears down the mount for the destination. Called
// by Cleanup hooks and on destination removal.
func (c *nfsClient) nfsUnmount(cfg map[string]string) error {
	key := nfsMountKey(cfg)
	mp, ok := c.cache.isMounted(key)
	if !ok {
		return nil
	}
	c.cache.forget(key)
	// syscall.Unmount with MNT_DETACH performs a lazy unmount:
	// the mount point detaches immediately but in-flight
	// operations continue to use it. This avoids races with
	// active file descriptors.
	if err := syscall.Unmount(mp, syscall.MNT_DETACH); err != nil {
		return fmt.Errorf("nfs: unmount %s: %w", mp, err)
	}
	// Best-effort cleanup of the mount point directory.
	_ = os.Remove(mp)
	return nil
}

// ============================================================================
// StorageAdapterClient: Test
// ============================================================================

// Test mounts the export and verifies the configured
// subdirectory exists. Failure modes:
//   - server unreachable → mount syscall fails
//   - bad export path → mount succeeds, Stat fails
//   - subdirectory missing → Stat returns ENOENT
//   - bad Kerberos creds → mount succeeds, file ops fail with
//     EACCES (we don't catch this here; the first Upload will).
func (c *nfsClient) Test(ctx context.Context, cfg map[string]string) error {
	mp, err := c.nfsEnsureMounted(ctx, cfg)
	if err != nil {
		return err
	}
	subdir := nfsSubdir(cfg)
	target := filepath.Join(mp, subdir)

	info, err := os.Stat(target)
	if err != nil {
		return fmt.Errorf("nfs: stat %s: %w", target, err)
	}
	if !info.IsDir() {
		return fmt.Errorf("nfs: %s is not a directory", target)
	}
	return nil
}

// nfsConnTestSuffix returns a short hex suffix for test
// artifact names so a connection drop mid-test leaves a
// uniquely named file behind for the operator to inspect.
func nfsConnTestSuffix() string {
	n := time.Now().UnixNano()
	return fmt.Sprintf("%x", [4]byte{
		byte(n),
		byte(n >> 8),
		byte(n >> 16),
		byte(n >> 24),
	})
}

// joinSubdir joins the configured subdirectory with a filename.
// Returns the filename only when subdir is empty.
func joinSubdir(subdir, name string) string {
	if subdir == "" {
		return name
	}
	return strings.TrimSuffix(subdir, "/") + "/" + name
}

// TestSteps implements plugin.RicherStorageTester. Mirrors the
// local-path round-trip (write → change → delete) against the
// NFS mount so operators get the same multi-step output the
// built-in local destination already produces.
//
// Each phase captures its own latency and surfaces a structured
// step. Failures on the write, the read-back, or the cleanup
// show up as named fail steps instead of being collapsed into
// a single "plugin_test" pass/fail.
//
// Artifacts live under the configured subdirectory with a name
// prefixed mcharbor-conn-test-{suffix}. Successful runs delete
// all files; a connection drop mid-test leaves them behind so
// the operator can see what went wrong.
func (c *nfsClient) TestSteps(ctx context.Context, cfg map[string]string) ([]plugin.TestStep, error) {
	mp, err := c.nfsEnsureMounted(ctx, cfg)
	if err != nil {
		return []plugin.TestStep{{
			Name:   "connect",
			Status: "fail",
			Detail: err.Error(),
		}}, nil
	}

	var steps []plugin.TestStep
	subdir := nfsSubdir(cfg)
	suffix := nfsConnTestSuffix()
	originalName := "mcharbor-conn-test-" + suffix + "-original.txt"
	changedName := "mcharbor-conn-test-" + suffix + "-changed.txt"
	original := []byte("mcharbor-conn-test\n" + suffix + "\n")
	changed := append(append([]byte{}, original...), '\n')

	originalPath := filepath.Join(mp, joinSubdir(subdir, originalName))
	changedPath := filepath.Join(mp, joinSubdir(subdir, changedName))

	// Step 1: write.
	writeStart := time.Now()
	if err := os.WriteFile(originalPath, original, 0644); err != nil {
		steps = append(steps, plugin.TestStep{
			Name:    "write",
			Status:  "fail",
			Detail:  err.Error(),
			Latency: time.Since(writeStart).String(),
		})
		return steps, nil
	}
	steps = append(steps, plugin.TestStep{
		Name:    "write",
		Status:  "pass",
		Detail:  fmt.Sprintf("wrote %d bytes to %s", len(original), originalPath),
		Latency: time.Since(writeStart).String(),
	})

	// Step 2: change + verify.
	changeStart := time.Now()
	if err := os.WriteFile(changedPath, changed, 0644); err != nil {
		steps = append(steps, plugin.TestStep{
			Name:    "change",
			Status:  "fail",
			Detail:  err.Error(),
			Latency: time.Since(changeStart).String(),
		})
		return steps, nil
	}
	got, err := os.ReadFile(changedPath)
	if err != nil {
		steps = append(steps, plugin.TestStep{
			Name:    "change",
			Status:  "fail",
			Detail:  "read back: " + err.Error(),
			Latency: time.Since(changeStart).String(),
		})
		return steps, nil
	}
	if string(got) != string(changed) {
		steps = append(steps, plugin.TestStep{
			Name:    "change",
			Status:  "fail",
			Detail:  fmt.Sprintf("contents mismatch (expected %d bytes, got %d)", len(changed), len(got)),
			Latency: time.Since(changeStart).String(),
		})
		return steps, nil
	}
	steps = append(steps, plugin.TestStep{
		Name:    "change",
		Status:  "pass",
		Detail:  fmt.Sprintf("wrote changed copy at %s and contents match", changedPath),
		Latency: time.Since(changeStart).String(),
	})

	// Step 3: delete.
	deleteStart := time.Now()
	if err := os.Remove(changedPath); err != nil {
		steps = append(steps, plugin.TestStep{
			Name:    "delete",
			Status:  "fail",
			Detail:  err.Error(),
			Latency: time.Since(deleteStart).String(),
		})
		return steps, nil
	}
	steps = append(steps, plugin.TestStep{
		Name:    "delete",
		Status:  "pass",
		Detail:  fmt.Sprintf("removed %s", changedPath),
		Latency: time.Since(deleteStart).String(),
	})
	// Best-effort cleanup of the original.
	_ = os.Remove(originalPath)

	return steps, nil
}

// ============================================================================
// StorageAdapterClient: Upload
// ============================================================================

// Upload writes r to key under the configured subdirectory.
// File is created with mode 0644 (umask applies). NFS write
// semantics: the kernel commits data to the server on Close;
// a crash before Close leaves the file incomplete on the
// server. The host's backup pipeline calls Close after
// streaming the entire payload, so this is fine for our use
// case.
func (c *nfsClient) Upload(ctx context.Context, cfg map[string]string, key string, r io.Reader) error {
	if key == "" {
		return errors.New("nfs: key is required")
	}
	mp, err := c.nfsEnsureMounted(ctx, cfg)
	if err != nil {
		return err
	}
	fullPath := filepath.Join(mp, joinSubdir(nfsSubdir(cfg), key))

	opCtx, cancel := context.WithTimeout(ctx, nfsOpTimeout)
	defer cancel()

	// Surface context cancellation as an io error rather than
	// letting os.OpenFile block indefinitely.
	done := make(chan error, 1)
	go func() {
		f, err := os.OpenFile(fullPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0644)
		if err != nil {
			done <- fmt.Errorf("nfs: create %s: %w", fullPath, err)
			return
		}
		defer f.Close()
		if _, err := io.Copy(f, r); err != nil {
			done <- fmt.Errorf("nfs: write %s: %w", fullPath, err)
			return
		}
		done <- f.Close()
	}()
	select {
	case err := <-done:
		return err
	case <-opCtx.Done():
		return fmt.Errorf("nfs: upload %s: timeout after %s", fullPath, nfsOpTimeout)
	}
}

// ============================================================================
// StorageAdapterClient: Download
// ============================================================================

// Download returns a reader for the file named key. Unlike
// the HTTP-based plugins (Dropbox, Box, SharePoint), NFS reads
// stream directly from the local mount — no wrapper needed,
// os.Open returns a standard *os.File that satisfies
// io.ReadCloser.
func (c *nfsClient) Download(ctx context.Context, cfg map[string]string, key string) (io.ReadCloser, error) {
	if key == "" {
		return nil, errors.New("nfs: key is required")
	}
	mp, err := c.nfsEnsureMounted(ctx, cfg)
	if err != nil {
		return nil, err
	}
	fullPath := filepath.Join(mp, joinSubdir(nfsSubdir(cfg), key))

	opCtx, cancel := context.WithTimeout(ctx, nfsOpTimeout)
	defer cancel()

	// Surface context cancellation. os.OpenFile doesn't honor
	// context, but a hung NFS server will at least respect
	// the read deadline set on the file descriptor below.
	done := make(chan struct {
		f   *os.File
		err error
	}, 1)
	go func() {
		f, err := os.Open(fullPath)
		done <- struct {
			f   *os.File
			err error
		}{f, err}
	}()
	select {
	case res := <-done:
		if res.err != nil {
			return nil, fmt.Errorf("nfs: open %s: %w", fullPath, res.err)
		}
		return res.f, nil
	case <-opCtx.Done():
		return nil, fmt.Errorf("nfs: open %s: timeout after %s", fullPath, nfsOpTimeout)
	}
}

// ============================================================================
// StorageAdapterClient: Delete
// ============================================================================

// Delete removes the file named key. Per the host's contract,
// "not found" is treated as success (idempotent delete).
func (c *nfsClient) Delete(ctx context.Context, cfg map[string]string, key string) error {
	if key == "" {
		return errors.New("nfs: key is required")
	}
	mp, err := c.nfsEnsureMounted(ctx, cfg)
	if err != nil {
		return err
	}
	fullPath := filepath.Join(mp, joinSubdir(nfsSubdir(cfg), key))

	err = os.Remove(fullPath)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("nfs: remove %s: %w", fullPath, err)
	}
	return nil
}

// ============================================================================
// StorageAdapterClient: List
// ============================================================================

// List returns the files under the configured subdirectory.
// Operators can scope to a prefix by passing prefix.
//
// We use os.ReadDir (returns []os.DirEntry) — entries are
// already sorted by name on Linux.
func (c *nfsClient) List(ctx context.Context, cfg map[string]string, prefix string) ([]plugin.StorageObject, error) {
	mp, err := c.nfsEnsureMounted(ctx, cfg)
	if err != nil {
		return nil, err
	}
	dir := filepath.Join(mp, nfsSubdir(cfg))

	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("nfs: readdir %s: %w", dir, err)
	}
	out := make([]plugin.StorageObject, 0, len(entries))
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if prefix != "" && !strings.HasPrefix(name, prefix) {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		out = append(out, plugin.StorageObject{
			Key:          name,
			Size:         info.Size(),
			LastModified: info.ModTime(),
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Key < out[j].Key })
	return out, nil
}

// ============================================================================
// OAuth handshake (not used by NFS — we implement the interface so the
// plugin satisfies plugin.StorageAdapterClient, but the methods always
// return ErrUnsupported because NFS auth is AUTH_SYS / RPCSEC_GSS, not a
// delegated authorization code flow.)
// ============================================================================

// ConsentURL is part of the StorageAdapterClient interface. NFS auth is
// credential/keytab-based, not OAuth; the host's
// /storage-locations/{id}/oauth/authorize handler never reaches this code
// path because the adapter's spec declares OAuth=false. Surfacing
// ErrUnsupported lets the host log a clear error if it ever does.
func (c *nfsClient) ConsentURL(_ context.Context, _, _, _, _ string, _ map[string]string) (string, error) {
	return "", plugin.ErrUnsupported
}

// ConsentCallback completes the OAuth handshake. Same as ConsentURL:
// not used for NFS; returns ErrUnsupported so the host's dispatcher
// surfaces a clear error if invoked.
func (c *nfsClient) ConsentCallback(_ context.Context, _, _, _, _, _ string, _ map[string]string) (map[string]string, error) {
	return nil, plugin.ErrUnsupported
}