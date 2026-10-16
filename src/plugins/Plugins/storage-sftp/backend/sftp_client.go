// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// Package main — sftp_client.go implements the StorageAdapterClient
// half of the storage-sftp plugin. It owns one LocationType ("sftp")
// and four SSH auth methods (password, publickey,
// keyboard_interactive, agent).
//
// Wire shape:
//
//	host  ── cfg map[string]string ──>  sftpClient.Test / Upload / ...
//	client ── sftp.Client, *ssh.Client ─>  server
//
// State: the client is stateless. Each call constructs its own
// (sftp.Client, *ssh.Client) pair, finishes the operation, and
// closes both. The host pools destinations but does not require
// us to reuse connections across calls — back-to-back operations
// from the same destination row are rare (backup runs are
// minutes apart at minimum).
//
// Concurrency: the client is safe for concurrent calls. No
// mutable fields are touched after NewSFTPClient returns.
package main

import (
	"context"
	"crypto/subtle"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/pkg/sftp"
	"github.com/therealmcsparrow/mcharbor/core/plugin"
	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/agent"
)

// Compile-time guarantee that sftpClient satisfies the host's
// adapter contract — including the optional richer test
// interface used by storage_locations/test.go.
var (
	_ plugin.StorageAdapterClient = (*sftpClient)(nil)
	_ plugin.RicherStorageTester  = (*sftpClient)(nil)
)

// sftpClient is the per-destination client. Stateless.
type sftpClient struct{}

// NewSFTPClient is the constructor the host invokes once per
// plugin process. Return a fresh instance — keep the type
// stateless.
func NewSFTPClient() *sftpClient { return &sftpClient{} }

// ─────────────────────────────────────────────────────────────────
// Public StorageAdapterClient surface
// ─────────────────────────────────────────────────────────────────

// Test does the cheapest possible liveness check — connect,
// authenticate, stat the configured subdirectory, then
// disconnect. Does NOT write a probe file (RicherStorageTester
// covers write/change/delete). Surfaced through the host's
// "Test connection" button on the storage location form.
func (c *sftpClient) Test(ctx context.Context, cfg map[string]string) error {
	if err := sftpValidateConfig(cfg); err != nil {
		return err
	}
	sc, sshClient, err := sftpDial(ctx, cfg)
	if err != nil {
		return fmt.Errorf("sftp test: connect: %w", err)
	}
	defer sftpClose(sc, sshClient)

	subdir := sftpSubdir(cfg)
	if subdir != "" {
		if _, err := sc.Stat(subdir); err != nil {
			return fmt.Errorf("sftp test: stat subdirectory %q: %w", subdir, err)
		}
	}
	return nil
}

// TestSteps implements RicherStorageTester. Returns the 4-step
// round-trip (connect / write / change / delete) the host's
// test UI expects. Each step's status text is short and
// operator-facing.
func (c *sftpClient) TestSteps(ctx context.Context, cfg map[string]string) ([]plugin.TestStep, error) {
	if err := sftpValidateConfig(cfg); err != nil {
		return nil, err
	}
	steps := make([]plugin.TestStep, 0, 4)

	// Step 1 — connect.
	sc, sshClient, err := sftpDial(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("connect: %w", err)
	}
	defer sftpClose(sc, sshClient)
	steps = append(steps, plugin.TestStep{
		Name:   "connect",
		Status: "pass",
		Detail: "SSH handshake + auth + sftp session opened",
	})

	// Step 2 — write. Probe path = <subdir>/.mcharbor-sftp-probe.
	probe := sftpJoinPath(sftpSubdir(cfg), ".mcharbor-sftp-probe")
	if err := sc.MkdirAll("/"); err != nil {
		// MkdirAll("/") is a no-op but if it ever errors we want to
		// surface that early.
		return steps, fmt.Errorf("write: mkdir probe dir: %w", err)
	}
	f, err := sc.Create(probe)
	if err != nil {
		return steps, fmt.Errorf("write: create probe: %w", err)
	}
	payload := []byte("mcharbor-sftp-probe\n")
	if _, err := f.Write(payload); err != nil {
		_ = f.Close()
		return steps, fmt.Errorf("write: %w", err)
	}
	if err := f.Close(); err != nil {
		return steps, fmt.Errorf("write: close probe: %w", err)
	}
	steps = append(steps, plugin.TestStep{
		Name:   "write",
		Status: "pass",
		Detail: fmt.Sprintf("wrote %d bytes to %q", len(payload), probe),
	})

	// Step 3 — change. Read back, verify content matches.
	rf, err := sc.Open(probe)
	if err != nil {
		return steps, fmt.Errorf("change: reopen probe: %w", err)
	}
	got, err := io.ReadAll(rf)
	_ = rf.Close()
	if err != nil {
		return steps, fmt.Errorf("change: read probe: %w", err)
	}
	if string(got) != string(payload) {
		return steps, fmt.Errorf("change: payload mismatch (wrote %d, read %d)", len(payload), len(got))
	}
	steps = append(steps, plugin.TestStep{
		Name:   "change",
		Status: "pass",
		Detail: "round-trip read matches round-trip write",
	})

	// Step 4 — delete.
	if err := sc.Remove(probe); err != nil {
		return steps, fmt.Errorf("delete: %w", err)
	}
	steps = append(steps, plugin.TestStep{
		Name:   "delete",
		Status: "pass",
		Detail: "probe file removed",
	})

	return steps, nil
}

// Upload writes the bytes from r to <subdir>/<key> on the
// server, creating intermediate directories as needed. The
// server-side path is constructed with sftpJoinPath so a key
// like "2026-08-10/file.tar.gz" lands under <subdir>/2026-08-10/.
//
// The reader's full content is consumed before returning.
// Callers pass an io.Reader positioned at byte 0; the entire
// remaining payload is copied.
func (c *sftpClient) Upload(ctx context.Context, cfg map[string]string, key string, r io.Reader) error {
	if err := sftpValidateConfig(cfg); err != nil {
		return err
	}
	sc, sshClient, err := sftpDial(ctx, cfg)
	if err != nil {
		return fmt.Errorf("sftp upload: connect: %w", err)
	}
	defer sftpClose(sc, sshClient)

	remote := sftpJoinPath(sftpSubdir(cfg), key)

	// mkdir -p on the parent directory so 2026-08-10/file.tar.gz
	// works as a key (the host writes backups into per-day
	// subfolders).
	parent := pathDir(remote)
	if parent != "" && parent != "." && parent != "/" {
		if err := sc.MkdirAll(parent); err != nil {
			return fmt.Errorf("sftp upload: mkdir -p %q: %w", parent, err)
		}
	}

	f, err := sc.Create(remote)
	if err != nil {
		return fmt.Errorf("sftp upload: create %q: %w", remote, err)
	}
	// Ensure the file handle closes even if io.Copy panics.
	defer func() {
		// sftp.File.Close closes the SFTP handle; ignore errors
		// here because the host already has the data it needs.
		_ = f.Close()
	}()

	written, err := io.Copy(f, r)
	if err != nil {
		return fmt.Errorf("sftp upload: write %q: %w", remote, err)
	}
	if err := f.Close(); err != nil {
		return fmt.Errorf("sftp upload: close %q: %w", remote, err)
	}
	_ = written // available for future metrics
	return nil
}

// Download returns a reader over the bytes at <subdir>/<key>.
// The reader is wrapped so closing it also tears down the SFTP
// session — callers should NOT retain the reader past the
// containing scope.
func (c *sftpClient) Download(ctx context.Context, cfg map[string]string, key string) (io.ReadCloser, error) {
	if err := sftpValidateConfig(cfg); err != nil {
		return nil, err
	}
	sc, sshClient, err := sftpDial(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("sftp download: connect: %w", err)
	}
	remote := sftpJoinPath(sftpSubdir(cfg), key)
	f, err := sc.Open(remote)
	if err != nil {
		sftpClose(sc, sshClient)
		return nil, fmt.Errorf("sftp download: open %q: %w", remote, err)
	}
	return &sftpReader{File: f, sc: sc, sshClient: sshClient}, nil
}

// Delete removes the object at <subdir>/<key>. Not-found is
// treated as success — the goal is "the object is not there",
// which is idempotent against retries.
func (c *sftpClient) Delete(ctx context.Context, cfg map[string]string, key string) error {
	if err := sftpValidateConfig(cfg); err != nil {
		return err
	}
	sc, sshClient, err := sftpDial(ctx, cfg)
	if err != nil {
		return fmt.Errorf("sftp delete: connect: %w", err)
	}
	defer sftpClose(sc, sshClient)

	remote := sftpJoinPath(sftpSubdir(cfg), key)
	if err := sc.Remove(remote); err != nil {
		// Treat "not found" as success.
		if isNotExist(err) {
			return nil
		}
		return fmt.Errorf("sftp delete: %q: %w", remote, err)
	}
	return nil
}

// List returns the immediate children of <subdir>/<prefix>.
// Directories are filtered out (the host's UI is for file
// objects). Results are sorted by name for deterministic output.
func (c *sftpClient) List(ctx context.Context, cfg map[string]string, prefix string) ([]plugin.StorageObject, error) {
	if err := sftpValidateConfig(cfg); err != nil {
		return nil, err
	}
	sc, sshClient, err := sftpDial(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("sftp list: connect: %w", err)
	}
	defer sftpClose(sc, sshClient)

	base := sftpSubdir(cfg)
	if prefix != "" {
		base = sftpJoinPath(base, prefix)
	}
	entries, err := sc.ReadDir(base)
	if err != nil {
		return nil, fmt.Errorf("sftp list: readdir %q: %w", base, err)
	}

	objs := make([]plugin.StorageObject, 0, len(entries))
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		objs = append(objs, plugin.StorageObject{
			Key:          e.Name(),
			Size:         e.Size(),
			LastModified: e.ModTime().UTC(),
		})
	}
	sort.Slice(objs, func(i, j int) bool { return objs[i].Key < objs[j].Key })
	return objs, nil
}

// ConsentURL returns plugin.ErrUnsupported — SFTP destinations
// do not have a delegated OAuth flow. Credentials are entered
// directly on the storage location form (and stored encrypted
// at rest via AES-256-GCM).
//
// Signature per core/plugin.StorageAdapterClient: the host
// hands us (locationID, locationType, publicBaseURL, state,
// cfg). The first four are positional; agents that need
// something from cfg (none of us do) read it from the map.
func (c *sftpClient) ConsentURL(_ context.Context, _, _, _, _ string, _ map[string]string) (string, error) {
	return "", plugin.ErrUnsupported
}

// ConsentCallback is the OAuth callback entrypoint —
// unsupported for the same reason as ConsentURL. The host
// expects a (map, error) return where the map is the
// tokens to merge back into the destination row; returning
// ErrUnsupported tells the host "this destination type does
// not use OAuth" and the host short-circuits the rest of the
// callback flow.
func (c *sftpClient) ConsentCallback(_ context.Context, _, _, _, _, _ string, _ map[string]string) (map[string]string, error) {
	return nil, plugin.ErrUnsupported
}

// ─────────────────────────────────────────────────────────────────
// Config helpers
// ─────────────────────────────────────────────────────────────────

// sftpAuthMethod resolves the auth_method field, falling back
// to "password" when unset or unknown. Mirrors the frontend's
// default.
func sftpAuthMethod(cfg map[string]string) (string, error) {
	v := strings.TrimSpace(cfg["auth_method"])
	switch v {
	case "":
		return sftpAuthPassword, nil
	case sftpAuthPassword,
		sftpAuthPublicKey,
		sftpAuthKeyboardInteractive,
		sftpAuthAgent:
		return v, nil
	default:
		return "", fmt.Errorf("sftp: unknown auth_method %q (expected one of password, publickey, keyboard_interactive, agent)", v)
	}
}

// sftpPort resolves the port field, defaulting to 22. Empty,
// non-numeric, and out-of-range values fall back to 22.
func sftpPort(cfg map[string]string) int {
	v := strings.TrimSpace(cfg["port"])
	if v == "" {
		return sftpDefaultPort
	}
	p, err := strconv.Atoi(v)
	if err != nil || p < 1 || p > 65535 {
		return sftpDefaultPort
	}
	return p
}

// sftpConnectTimeout resolves the connect_timeout field,
// defaulting to sftpDefaultConnectTimeout and flooring at 5s
// so a typo of "0s" can't lock up the host's worker pool.
func sftpConnectTimeout(cfg map[string]string) time.Duration {
	v := strings.TrimSpace(cfg["connect_timeout"])
	if v == "" {
		v = sftpDefaultConnectTimeout
	}
	d, err := time.ParseDuration(v)
	if err != nil || d < 5*time.Second {
		d, _ = time.ParseDuration(sftpDefaultConnectTimeout)
	}
	return d
}

// sftpSubdir returns the path field, normalized: trimmed of
// leading/trailing slashes so it joins cleanly with key. Empty
// when blank.
func sftpSubdir(cfg map[string]string) string {
	v := strings.TrimSpace(cfg["path"])
	v = strings.Trim(v, "/")
	return v
}

// sftpJoinPath builds <subdir>/<key>. The subdir is assumed
// already slash-normalized (sftpSubdir does that). The key is
// trusted as-is (host-issued, no user input).
func sftpJoinPath(subdir, key string) string {
	if subdir == "" {
		return "/" + strings.TrimLeft(key, "/")
	}
	return "/" + subdir + "/" + strings.TrimLeft(key, "/")
}

// pathDir returns the directory component of an SFTP path.
// "/" and "." and "" return themselves (no parent). We avoid
// pulling in path/filepath because that package uses OS-specific
// separators — SFTP always uses "/".
func pathDir(p string) string {
	p = strings.TrimRight(p, "/")
	if p == "" {
		return ""
	}
	i := strings.LastIndex(p, "/")
	if i < 0 {
		return ""
	}
	if i == 0 {
		return "/"
	}
	return p[:i]
}

// sftpValidateConfig rejects obviously-broken destinations
// up front so the host can surface the error on "Save" rather
// than on the first backup run.
func sftpValidateConfig(cfg map[string]string) error {
	if strings.TrimSpace(cfg["server"]) == "" {
		return errors.New("sftp: server is required")
	}
	if strings.TrimSpace(cfg["username"]) == "" {
		return errors.New("sftp: username is required")
	}
	auth, err := sftpAuthMethod(cfg)
	if err != nil {
		return err
	}
	switch auth {
	case sftpAuthPassword:
		if cfg["password"] == "" {
			return errors.New("sftp: password is required for password auth")
		}
	case sftpAuthPublicKey:
		if strings.TrimSpace(cfg["private_key_path"]) == "" {
			return errors.New("sftp: private_key_path is required for publickey auth")
		}
		if _, err := os.Stat(cfg["private_key_path"]); err != nil {
			return fmt.Errorf("sftp: private_key_path %q is not readable: %w", cfg["private_key_path"], err)
		}
	case sftpAuthKeyboardInteractive:
		// Password optional — server may prompt for it via the
		// challenge/response loop.
	case sftpAuthAgent:
		if strings.TrimSpace(cfg["agent_socket"]) == "" {
			return errors.New("sftp: agent_socket is required for agent auth")
		}
	}
	return nil
}

// ─────────────────────────────────────────────────────────────────
// SSH plumbing
// ─────────────────────────────────────────────────────────────────

// buildAuthMethods assembles the slice of ssh.AuthMethod based
// on the resolved auth mode. The host's SSH client attempts
// each in order; we hand it a single best-fit method to keep
// the negotiation predictable.
func buildAuthMethods(cfg map[string]string, auth string) ([]ssh.AuthMethod, error) {
	switch auth {
	case sftpAuthPassword:
		return []ssh.AuthMethod{ssh.Password(cfg["password"])}, nil

	case sftpAuthPublicKey:
		keyBytes, err := os.ReadFile(cfg["private_key_path"])
		if err != nil {
			return nil, fmt.Errorf("sftp: read private key: %w", err)
		}
		var signer ssh.Signer
		passphrase := cfg["private_key_passphrase"]
		if passphrase != "" {
			signer, err = ssh.ParsePrivateKeyWithPassphrase(keyBytes, []byte(passphrase))
		} else {
			signer, err = ssh.ParsePrivateKey(keyBytes)
		}
		if err != nil {
			return nil, fmt.Errorf("sftp: parse private key: %w", err)
		}
		return []ssh.AuthMethod{ssh.PublicKeys(signer)}, nil

	case sftpAuthKeyboardInteractive:
		// Echo the configured password back for password-style
		// prompts; multi-factor prompts that need operator
		// interaction return empty (the host stores no TTY).
		//
		// KeyboardInteractiveChallenge signature (RFC 4256):
		//   func(name, instruction string, questions []string,
		//        echos []bool) (answers []string, err error)
		pass := cfg["password"]
		return []ssh.AuthMethod{
			ssh.KeyboardInteractive(func(_ string, _ string, questions []string, echos []bool) ([]string, error) {
				answers := make([]string, len(questions))
				for i, q := range questions {
					// Heuristic: if the question asks for a password
					// AND the server marked it non-echoed (echoes[i]
					// false ⇒ it's a secret), echo our stored
					// password; otherwise return empty.
					ql := strings.ToLower(q)
					if !echos[i] && (strings.Contains(ql, "password") || strings.Contains(ql, "passphrase") || strings.Contains(ql, "token") || strings.Contains(ql, "pin")) {
						answers[i] = pass
					}
				}
				return answers, nil
			}),
		}, nil

	case sftpAuthAgent:
		conn, err := net.Dial("unix", cfg["agent_socket"])
		if err != nil {
			return nil, fmt.Errorf("sftp: dial agent socket %q: %w", cfg["agent_socket"], err)
		}
		ag := agent.NewClient(conn)
		signers, err := ag.Signers()
		if err != nil {
			_ = conn.Close()
			return nil, fmt.Errorf("sftp: agent signers: %w", err)
		}
		if len(signers) == 0 {
			_ = conn.Close()
			return nil, errors.New("sftp: agent socket connected but no identities loaded")
		}
		return []ssh.AuthMethod{ssh.PublicKeysCallback(ag.Signers)}, nil

	default:
		return nil, fmt.Errorf("sftp: unsupported auth method %q", auth)
	}
}

// buildHostKeyCallback wires the verification strategy:
//
//   - host_key_fingerprint set: SHA-256 strict verification via
//     ssh.FingerprintSHA256 (OpenSSH 6.8+ "SHA256:<base64>"
//     format, the form `ssh-keygen -lf … -E sha256` emits).
//   - empty: ssh.InsecureIgnoreHostKey (TOFU disabled).
//
// We compare inline rather than via knownhosts.New because
// knownhosts expects ~/.ssh/known_hosts lines, not bare
// fingerprints. subtle.ConstantTimeCompare avoids timing
// leaks on the hash compare.
func buildHostKeyCallback(cfg map[string]string) (ssh.HostKeyCallback, error) {
	want := strings.TrimSpace(cfg["host_key_fingerprint"])
	if want == "" {
		return ssh.InsecureIgnoreHostKey(), nil
	}
	// Normalize the configured fingerprint so operators can paste
	// either "SHA256:abcd…" (full form, what ssh-keygen emits) or
	// just "abcd…" (no prefix) and have it match. The result must
	// be upper-cased to compare against ssh.FingerprintSHA256's
	// output, which uses upper-case base64.
	wantNorm := want
	if !strings.HasPrefix(strings.ToUpper(want), "SHA256:") {
		wantNorm = "SHA256:" + want
	}
	wantNorm = strings.ToUpper(strings.TrimSpace(wantNorm))

	return func(_ string, _ net.Addr, key ssh.PublicKey) error {
		// ssh.FingerprintSHA256 returns "SHA256:<base64>" for the
		// key type encoded in `key`. We only support SHA-256
		// fingerprints here — MD5 fingerprints (the OpenSSH < 6.8
		// default) are rejected as a security improvement.
		got := strings.ToUpper(ssh.FingerprintSHA256(key))
		if subtle.ConstantTimeCompare([]byte(got), []byte(wantNorm)) == 1 {
			return nil
		}
		return fmt.Errorf("sftp: server host key fingerprint mismatch (got %s, want %s)", got, want)
	}, nil
}

// sftpDial opens a TCP connection, negotiates SSH, and starts
// an SFTP subsystem session. The returned clients are paired;
// callers must close BOTH (use sftpClose) or leak a goroutine
// and an SSH session.
//
// ctx is honored only as a wall-clock cap: net.Dial doesn't
// take a context, so we run it inside a select against a
// timeout. Once TCP is open, SSH.NewClientConn respects the
// deadline via ClientConfig.Timeout.
func sftpDial(ctx context.Context, cfg map[string]string) (*sftp.Client, *ssh.Client, error) {
	auth, err := sftpAuthMethod(cfg)
	if err != nil {
		return nil, nil, err
	}
	authMethods, err := buildAuthMethods(cfg, auth)
	if err != nil {
		return nil, nil, err
	}
	hostKeyCallback, err := buildHostKeyCallback(cfg)
	if err != nil {
		return nil, nil, err
	}

	addr := net.JoinHostPort(cfg["server"], strconv.Itoa(sftpPort(cfg)))
	timeout := sftpConnectTimeout(cfg)

	// Wall-clock-bounded TCP dial.
	type dialResult struct {
		conn net.Conn
		err  error
	}
	ch := make(chan dialResult, 1)
	go func() {
		c, e := net.DialTimeout("tcp", addr, timeout)
		ch <- dialResult{c, e}
	}()
	var rawConn net.Conn
	select {
	case <-ctx.Done():
		return nil, nil, fmt.Errorf("sftp dial: context canceled: %w", ctx.Err())
	case r := <-ch:
		if r.err != nil {
			return nil, nil, fmt.Errorf("sftp dial %s: %w", addr, r.err)
		}
		rawConn = r.conn
	}

	sshConfig := &ssh.ClientConfig{
		User:            cfg["username"],
		Auth:            authMethods,
		HostKeyCallback: hostKeyCallback,
		Timeout:         timeout,
	}
	sshConn, chans, reqs, err := ssh.NewClientConn(rawConn, addr, sshConfig)
	if err != nil {
		_ = rawConn.Close()
		return nil, nil, fmt.Errorf("sftp ssh handshake: %w", err)
	}
	sshClient := ssh.NewClient(sshConn, chans, reqs)

	sc, err := sftp.NewClient(sshClient)
	if err != nil {
		_ = sshClient.Close()
		return nil, nil, fmt.Errorf("sftp subsystem: %w", err)
	}
	return sc, sshClient, nil
}

// sftpClose shuts down both layers. Closing ssh first stops the
// transport goroutines; closing the sftp.Client releases the
// SFTP subsystem. Errors are ignored — we made our best effort.
func sftpClose(sc *sftp.Client, sshClient *ssh.Client) {
	if sc != nil {
		_ = sc.Close()
	}
	if sshClient != nil {
		_ = sshClient.Close()
	}
}

// ─────────────────────────────────────────────────────────────────
// sftpReader — closes both layers when the host closes the
// downloaded stream. Bundles the sftp.File so callers don't
// need to know about the underlying connection.
// ─────────────────────────────────────────────────────────────────

// sftpReader wraps an sftp.File so closing it tears down the
// whole SSH+SFTP stack. The host only ever sees io.ReadCloser;
// it must not retain the reader past the consumer's scope.
type sftpReader struct {
	*sftp.File
	sc        *sftp.Client
	sshClient *ssh.Client
	closed    bool
}

func (r *sftpReader) Close() error {
	if r.closed {
		return nil
	}
	r.closed = true
	var firstErr error
	if r.File != nil {
		if err := r.File.Close(); err != nil {
			firstErr = err
		}
	}
	sftpClose(r.sc, r.sshClient)
	return firstErr
}

// ─────────────────────────────────────────────────────────────────
// Small utilities
// ─────────────────────────────────────────────────────────────────

// isNotExist checks whether an SFTP error indicates a missing
// file. The SFTP subsystem uses errors that compare equal to
// fs.ErrNotExist on a best-effort basis; fall back to string
// matching for the rest.
func isNotExist(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, os.ErrNotExist) {
		return true
	}
	// sftp returns its own typed errors; match by message in
	// the worst case.
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "not exist") || strings.Contains(msg, "no such file")
}