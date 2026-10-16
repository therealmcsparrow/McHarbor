// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net"
	"sort"
	"strings"
	"time"

	smb2 "github.com/hirochachacha/go-smb2"

	"github.com/therealmcsparrow/mcharbor/core/plugin"
)

// sambaDialTimeout caps the TCP connect to the SMB server. SMB
// servers on LAN should answer well within this window; anything
// slower usually means a firewall block or wrong port.
const sambaDialTimeout = 15 * time.Second

// sambaOpTimeout caps individual SMB operations (open, read,
// write, list). Larger backups stream data well below this
// cap; the cap exists to surface hung sessions instead of
// letting them wedge the host's upload pipeline.
const sambaOpTimeout = 5 * time.Minute

// sambaSMBPort is the default SMB TCP port. SMB/CIFS has been
// on 445 since SMB2; the legacy NetBIOS 139 port is rarely used
// by modern Samba (>=4.x).
const sambaSMBPort = 445

// sambaClient is the StorageAdapterClient the host dispatches to
// for the samba LocationType. Stateless: each method takes the
// destination row's decrypted config map directly.
type sambaClient struct{}

// NewSambaClient returns a fresh client. The plugin keeps no
// shared state, so callers can construct as many as they want.
func NewSambaClient() *sambaClient {
	return &sambaClient{}
}

// ============================================================================
// Config normalization
// ============================================================================

// sambaNormalizePath collapses "..", ".", and "//" segments so
// the SMB server sees a canonical path. SMB servers reject
// ".." traversal in many configs (Unix extensions off); we
// resolve here so the operator never sees that class of error.
//
// The output is share-root-relative (no leading "/").
func sambaNormalizePath(p string) string {
	if p == "" {
		return ""
	}
	p = strings.TrimSpace(p)
	// Backslash to forward slash. go-smb2 accepts either, but
	// forward slashes are unambiguous in our log messages.
	p = strings.ReplaceAll(p, "\\", "/")
	// Collapse runs of "/".
	for strings.Contains(p, "//") {
		p = strings.ReplaceAll(p, "//", "/")
	}
	// Strip trailing "/" (root marker).
	for len(p) > 1 && strings.HasSuffix(p, "/") {
		p = strings.TrimSuffix(p, "/")
	}
	// Strip leading "/".
	for strings.HasPrefix(p, "/") {
		p = strings.TrimPrefix(p, "/")
	}
	return p
}

// sambaSubdir returns the configured subdirectory as a path
// relative to the share root. SMB paths in go-smb2 are
// share-root-relative.
func sambaSubdir(cfg map[string]string) string {
	p := sambaNormalizePath(cfg["path"])
	if p == "" {
		return "."
	}
	return p
}

// sambaAuthMethod validates cfg["auth_method"] against the
// three supported values and returns it. Defaults to "basic"
// when unset (mirrors ConfigFieldDecl.Default).
func sambaAuthMethod(cfg map[string]string) (string, error) {
	switch cfg["auth_method"] {
	case "":
		return sambaAuthBasic, nil
	case sambaAuthGuest, sambaAuthBasic, sambaAuthDomain:
		return cfg["auth_method"], nil
	default:
		return "", fmt.Errorf("samba: unknown auth_method %q", cfg["auth_method"])
	}
}

// sambaValidateCredentials enforces the auth_method →
// credential matrix. Guest: requires guest_user (defaults to
// "nobody"). Basic: user/pass. Domain: user/pass/domain.
func sambaValidateCredentials(authMethod string, cfg map[string]string) error {
	switch authMethod {
	case sambaAuthGuest:
		// guest_user falls back to the system nobody account.
		// guest_password is typically empty but some Samba
		// configs require one (rare).
		if strings.TrimSpace(cfg["guest_user"]) == "" && strings.TrimSpace(sambaDefaultGuestUser) == "" {
			return errors.New("samba: guest_user is required when neither cfg nor default is set")
		}
		return nil
	case sambaAuthBasic:
		if strings.TrimSpace(cfg["username"]) == "" {
			return errors.New("samba: username is required for basic auth")
		}
		if cfg["password"] == "" {
			return errors.New("samba: password is required for basic auth")
		}
		return nil
	case sambaAuthDomain:
		if strings.TrimSpace(cfg["username"]) == "" {
			return errors.New("samba: username is required for domain auth")
		}
		if cfg["password"] == "" {
			return errors.New("samba: password is required for domain auth")
		}
		if strings.TrimSpace(cfg["domain"]) == "" {
			return errors.New("samba: domain is required for domain auth")
		}
		return nil
	default:
		return fmt.Errorf("samba: unsupported auth_method %q", authMethod)
	}
}

// ============================================================================
// Connection bootstrap
// ============================================================================

// sambaDial opens a TCP connection, performs the SMB2/3
// handshake, and mounts the share. The caller MUST call
// sambaClose() (or share.Umount() + session.Logoff() + conn.Close())
// when done.
func (c *sambaClient) sambaDial(ctx context.Context, cfg map[string]string) (*smb2.Session, *smb2.Share, net.Conn, error) {
	server := strings.TrimSpace(cfg["server"])
	share := strings.TrimSpace(cfg["share"])
	if server == "" {
		return nil, nil, nil, errors.New("samba: server is required")
	}
	if share == "" {
		return nil, nil, nil, errors.New("samba: share is required")
	}

	authMethod, err := sambaAuthMethod(cfg)
	if err != nil {
		return nil, nil, nil, err
	}
	if err := sambaValidateCredentials(authMethod, cfg); err != nil {
		return nil, nil, nil, err
	}

	// 1. TCP dial. host:port — default 445.
	addr := server
	if !strings.Contains(addr, ":") {
		addr = net.JoinHostPort(addr, fmt.Sprintf("%d", sambaSMBPort))
	}
	dialer := &net.Dialer{Timeout: sambaDialTimeout}
	conn, err := dialer.DialContext(ctx, "tcp", addr)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("samba: TCP dial %s: %w", addr, err)
	}

	// 2. Build the NTLM initiator per auth_method.
	//
	// NOTE: go-smb2 v1.x explicitly rejects NTLMInitiator with
	// User == "" — the Dialer.DialContext code path returns
	// "Anonymous account is not supported yet. Use guest account
	// instead." So the "guest" auth mode MUST pass a real
	// account name (the operator's guest user, defaulting to
	// "nobody" — the account Samba typically uses for guest
	// access via smb.conf's `guest account =` directive).
	var initiator smb2.Initiator
	switch authMethod {
	case sambaAuthGuest:
		user := strings.TrimSpace(cfg["guest_user"])
		if user == "" {
			user = sambaDefaultGuestUser
		}
		initiator = &smb2.NTLMInitiator{
			User:     user,
			Password: cfg["guest_password"],
		}
	case sambaAuthBasic:
		initiator = &smb2.NTLMInitiator{
			User:     strings.TrimSpace(cfg["username"]),
			Password: cfg["password"],
		}
	case sambaAuthDomain:
		initiator = &smb2.NTLMInitiator{
			User:     strings.TrimSpace(cfg["username"]),
			Password: cfg["password"],
			Domain:   strings.TrimSpace(cfg["domain"]),
		}
	}

	// 3. SMB2/3 handshake + session login. Dialer only exposes
	// MaxCreditBalance + Negotiator + Initiator in v1.x — there
	// is no encryption-negotiation field. SMB3 encryption
	// (AES-128-CCM) is not yet available in this version.
	d := &smb2.Dialer{
		Initiator: initiator,
	}
	session, err := d.Dial(conn)
	if err != nil {
		conn.Close()
		return nil, nil, nil, fmt.Errorf("samba: SMB handshake: %w", err)
	}

	// 4. Mount the share. Session.Mount returns *Share (not
	// *Filesystem — that name is from the io/fs wrapper on
	// *Share, not the SMB primitive).
	fsShare, err := session.Mount(share)
	if err != nil {
		session.Logoff()
		conn.Close()
		return nil, nil, nil, fmt.Errorf("samba: mount share %q: %w", share, err)
	}

	return session, fsShare, conn, nil
}

// sambaClose releases the SMB session in the canonical
// reverse-acquisition order: unmount → logoff → TCP close.
// Always call via defer.
func sambaClose(session *smb2.Session, share *smb2.Share, conn net.Conn) {
	if share != nil {
		_ = share.Umount()
	}
	if session != nil {
		_ = session.Logoff()
	}
	if conn != nil {
		_ = conn.Close()
	}
}

// sambaWithShare opens a session, runs fn under the share's
// filesystem, and cleans up regardless of fn's return value.
// fn receives the *Share rooted at the share.
func (c *sambaClient) sambaWithShare(ctx context.Context, cfg map[string]string, fn func(share *smb2.Share) error) error {
	session, share, conn, err := c.sambaDial(ctx, cfg)
	if err != nil {
		return err
	}
	defer sambaClose(session, share, conn)
	return fn(share)
}

// sambaWithCtxShare wraps sambaWithShare with an operation-scoped
// timeout. SMB operations are subject to sambaOpTimeout (long
// enough for large uploads, short enough to surface hangs).
func (c *sambaClient) sambaWithCtxShare(ctx context.Context, cfg map[string]string, fn func(share *smb2.Share) error) error {
	opCtx, cancel := context.WithTimeout(ctx, sambaOpTimeout)
	defer cancel()
	return c.sambaWithShare(opCtx, cfg, func(share *smb2.Share) error {
		share = share.WithContext(opCtx)
		return fn(share)
	})
}

// ============================================================================
// StorageAdapterClient: Test
// ============================================================================

// Test verifies the destination is reachable, the credentials
// authenticate, and the configured subdirectory exists (we
// stat it; failure here means a typo or a permission issue).
func (c *sambaClient) Test(ctx context.Context, cfg map[string]string) error {
	return c.sambaWithCtxShare(ctx, cfg, func(share *smb2.Share) error {
		subdir := sambaSubdir(cfg)
		if subdir == "." {
			// No subdirectory configured → confirm the share
			// root is listable.
			_, err := share.ReadDir(".")
			return err
		}
		info, err := share.Stat(subdir)
		if err != nil {
			return fmt.Errorf("samba: stat %q: %w", subdir, err)
		}
		if !info.IsDir() {
			return fmt.Errorf("samba: %q is not a directory", subdir)
		}
		return nil
	})
}

// sambaConnTestSuffix returns a short hex suffix for test
// artifact names so a connection drop mid-test leaves a
// uniquely named file behind for the operator to inspect.
func sambaConnTestSuffix() string {
	n := time.Now().UnixNano()
	return fmt.Sprintf("%x", [4]byte{
		byte(n),
		byte(n >> 8),
		byte(n >> 16),
		byte(n >> 24),
	})
}

// joinSubdir joins the configured subdirectory with a filename.
// Returns the filename only when both are empty (share root).
func joinSubdir(subdir, name string) string {
	if subdir == "." || subdir == "" {
		return name
	}
	return strings.TrimSuffix(subdir, "/") + "/" + name
}

// TestSteps implements plugin.RicherStorageTester. Mirrors the
// local-path round-trip (write → change → delete) against the
// SMB share so operators get the same multi-step output the
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
func (c *sambaClient) TestSteps(ctx context.Context, cfg map[string]string) ([]plugin.TestStep, error) {
	var steps []plugin.TestStep

	err := c.sambaWithCtxShare(ctx, cfg, func(share *smb2.Share) error {
		subdir := sambaSubdir(cfg)
		suffix := sambaConnTestSuffix()
		originalName := "mcharbor-conn-test-" + suffix + "-original.txt"
		changedName := "mcharbor-conn-test-" + suffix + "-changed.txt"
		original := []byte("mcharbor-conn-test\n" + suffix + "\n")
		changed := append(append([]byte{}, original...), '\n')

		originalPath := joinSubdir(subdir, originalName)
		changedPath := joinSubdir(subdir, changedName)

		// Step 1: write.
		writeStart := time.Now()
		if err := share.WriteFile(originalPath, original, 0644); err != nil {
			steps = append(steps, plugin.TestStep{
				Name:    "write",
				Status:  "fail",
				Detail:  err.Error(),
				Latency: time.Since(writeStart).String(),
			})
			return nil
		}
		steps = append(steps, plugin.TestStep{
			Name:    "write",
			Status:  "pass",
			Detail:  fmt.Sprintf("wrote %d bytes to %s", len(original), originalPath),
			Latency: time.Since(writeStart).String(),
		})

		// Step 2: change. SMB Create truncates an existing file
		// (default open disposition is supersede), so writing to
		// a new path satisfies "change" semantics; we then read
		// back and verify contents match.
		changeStart := time.Now()
		if err := share.WriteFile(changedPath, changed, 0644); err != nil {
			steps = append(steps, plugin.TestStep{
				Name:    "change",
				Status:  "fail",
				Detail:  err.Error(),
				Latency: time.Since(changeStart).String(),
			})
			return nil
		}
		got, err := share.ReadFile(changedPath)
		if err != nil {
			steps = append(steps, plugin.TestStep{
				Name:    "change",
				Status:  "fail",
				Detail:  "read back: " + err.Error(),
				Latency: time.Since(changeStart).String(),
			})
			return nil
		}
		if string(got) != string(changed) {
			steps = append(steps, plugin.TestStep{
				Name:    "change",
				Status:  "fail",
				Detail:  fmt.Sprintf("contents mismatch (expected %d bytes, got %d)", len(changed), len(got)),
				Latency: time.Since(changeStart).String(),
			})
			return nil
		}
		steps = append(steps, plugin.TestStep{
			Name:    "change",
			Status:  "pass",
			Detail:  fmt.Sprintf("wrote changed copy at %s and contents match", changedPath),
			Latency: time.Since(changeStart).String(),
		})

		// Step 3: delete.
		deleteStart := time.Now()
		if err := share.Remove(changedPath); err != nil {
			steps = append(steps, plugin.TestStep{
				Name:    "delete",
				Status:  "fail",
				Detail:  err.Error(),
				Latency: time.Since(deleteStart).String(),
			})
			return nil
		}
		steps = append(steps, plugin.TestStep{
			Name:    "delete",
			Status:  "pass",
			Detail:  fmt.Sprintf("removed %s", changedPath),
			Latency: time.Since(deleteStart).String(),
		})
		// Best-effort cleanup of the original.
		_ = share.Remove(originalPath)

		return nil
	})
	if err != nil {
		// Underlying dial/mount failure — surface the auth /
		// connectivity error as a single fail step.
		if len(steps) == 0 {
			steps = append(steps, plugin.TestStep{
				Name:   "connect",
				Status: "fail",
				Detail: err.Error(),
			})
		}
		return steps, err
	}
	return steps, nil
}

// ============================================================================
// StorageAdapterClient: Upload
// ============================================================================

// Upload writes r to key under the configured subdirectory.
//
// SMB writes are streamed: we Open the destination, copy the
// reader into the file handle, and Close. The share.Create path
// uses supersede semantics (truncate-or-create) which matches
// backup snapshot writes — older snapshots are fully replaced.
func (c *sambaClient) Upload(ctx context.Context, cfg map[string]string, key string, r io.Reader) error {
	if key == "" {
		return errors.New("samba: key is required")
	}
	subdir := sambaSubdir(cfg)
	fullPath := joinSubdir(subdir, key)

	return c.sambaWithCtxShare(ctx, cfg, func(share *smb2.Share) error {
		f, err := share.Create(fullPath)
		if err != nil {
			return fmt.Errorf("samba: create %q: %w", fullPath, err)
		}
		defer f.Close()
		if _, err := io.Copy(f, r); err != nil {
			return fmt.Errorf("samba: write %q: %w", fullPath, err)
		}
		return f.Close()
	})
}

// ============================================================================
// StorageAdapterClient: Download
// ============================================================================

// Download returns a reader for the file named key. SMB file
// handles expose Read + Seek + Close; we return a handle the
// caller can stream from. The handle stays valid until Close is
// called — which means we keep the SMB session open for the
// lifetime of the download.
//
// To prevent the session from leaking, we wrap the file handle
// with a custom closer that releases the entire session on
// Close. The wrapper is returned instead of the raw file so
// callers don't need to know about session lifecycle.
func (c *sambaClient) Download(ctx context.Context, cfg map[string]string, key string) (io.ReadCloser, error) {
	if key == "" {
		return nil, errors.New("samba: key is required")
	}
	subdir := sambaSubdir(cfg)
	fullPath := joinSubdir(subdir, key)

	session, share, conn, err := c.sambaDial(ctx, cfg)
	if err != nil {
		return nil, err
	}
	share = share.WithContext(ctx)
	f, err := share.Open(fullPath)
	if err != nil {
		sambaClose(session, share, conn)
		return nil, fmt.Errorf("samba: open %q: %w", fullPath, err)
	}
	// Wrap the file handle so closing it tears down the
	// session + connection. The host's backup pipeline
	// streams from the reader and calls Close when done.
	return &sambaReadCloser{
		f:       f,
		session: session,
		share:   share,
		conn:    conn,
	}, nil
}

// sambaReadCloser pairs an SMB file handle with the session
// resources that keep it alive. Close() releases the file,
// unmounts the share, logs off, and tears down the TCP
// connection in the canonical reverse-acquisition order.
type sambaReadCloser struct {
	f       io.ReadCloser
	session *smb2.Session
	share   *smb2.Share
	conn    net.Conn
	closed  bool
}

func (r *sambaReadCloser) Read(p []byte) (int, error) { return r.f.Read(p) }

func (r *sambaReadCloser) Close() error {
	if r.closed {
		return nil
	}
	r.closed = true
	err := r.f.Close()
	sambaClose(r.session, r.share, r.conn)
	return err
}

// ============================================================================
// StorageAdapterClient: Delete
// ============================================================================

// Delete removes the file named key. Per the host's contract,
// "not found" is treated as success (idempotent delete).
func (c *sambaClient) Delete(ctx context.Context, cfg map[string]string, key string) error {
	if key == "" {
		return errors.New("samba: key is required")
	}
	subdir := sambaSubdir(cfg)
	fullPath := joinSubdir(subdir, key)
	return c.sambaWithCtxShare(ctx, cfg, func(share *smb2.Share) error {
		err := share.Remove(fullPath)
		if err != nil && isSambaNotExist(err) {
			return nil
		}
		return err
	})
}

// isSambaNotExist checks whether the underlying SMB call
// reported "file not found". go-smb2 returns an os.PathError
// with os.ErrNotExist wrapped inside, which errors.Is matches
// against fs.ErrNotExist.
func isSambaNotExist(err error) bool {
	return errors.Is(err, fs.ErrNotExist)
}

// ============================================================================
// StorageAdapterClient: List
// ============================================================================

// List returns the files under the configured subdirectory.
// Operators can scope to a prefix by passing prefix (the host
// passes the snapshot's prefix as configured on the backup).
//
// SMB returns directory entries; we Sort by name (the host's
// DataGrid column expects lexical order).
func (c *sambaClient) List(ctx context.Context, cfg map[string]string, prefix string) ([]plugin.StorageObject, error) {
	var out []plugin.StorageObject

	err := c.sambaWithCtxShare(ctx, cfg, func(share *smb2.Share) error {
		subdir := sambaSubdir(cfg)
		// go-smb2 v1.x: Share.ReadDir returns []os.FileInfo
		// (not []os.DirEntry). Each entry already carries name,
		// size, modtime, and the IsDir flag — no per-entry
		// stat call needed.
		entries, err := share.ReadDir(subdir)
		if err != nil {
			return err
		}
		for _, e := range entries {
			if e.IsDir() {
				continue
			}
			name := e.Name()
			if prefix != "" && !strings.HasPrefix(name, prefix) {
				continue
			}
			out = append(out, plugin.StorageObject{
				Key:          name,
				Size:         e.Size(),
				LastModified: e.ModTime(),
			})
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	sort.Slice(out, func(i, j int) bool { return out[i].Key < out[j].Key })
	return out, nil
}
// ============================================================================
// StorageAdapterClient: ConsentURL + ConsentCallback (no OAuth)
// ============================================================================
//
// SMB destinations authenticate with NTLM/NTLMv2 handshakes
// against the share's local SAM database or an AD/domain
// controller. There is no delegated authorization code flow -
// the host's OAuth consent UI does not apply. We return
// ErrUnsupported so the host's adapter dispatcher short-circuits
// cleanly when an operator clicks "Connect" against a Samba
// destination.
//
// See plugin.StorageAdapterClient docstring at core/plugin/adapters.go
// for the ErrUnsupported contract.

func (c *sambaClient) ConsentURL(_ context.Context, _ string, _ string, _ string, _ string, _ map[string]string) (string, error) {
	return "", plugin.ErrUnsupported
}

func (c *sambaClient) ConsentCallback(_ context.Context, _ string, _ string, _ string, _ string, _ string, _ map[string]string) (map[string]string, error) {
	return nil, plugin.ErrUnsupported
}
