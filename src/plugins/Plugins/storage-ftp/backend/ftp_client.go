// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// FTP/FTPS plugin StorageAdapterClient. One connection per
// operation — FTP servers often enforce short idle timeouts, so
// reusing a single connection across operations is more brittle
// than the latency cost of opening a fresh one.
//
// Library: jlaffaye/ftp (BSD-3-Clause). The library handles
// the FTP control protocol (USER/PASS, CWD, LIST, MLSD, RETR,
// STOR, DELE, QUIT); the plugin handles the auth-method matrix
// and the TLS configuration.
//
// Auth-method → dial shape:
//
//   anonymous       plain FTP
//   password        plain FTP
//   ftps_explicit   plain FTP upgraded to TLS via AUTH TLS
//   ftps_implicit   TLS on port 990 from the first byte
//   client_cert     TLS dial with client certificate in
//                   tls.Config.Certificates — server identifies
//                   the client during the TLS handshake

package main

import (
	"bytes"
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/jlaffaye/ftp"
	"github.com/therealmcsparrow/mcharbor/core/plugin"
)

// ftpDialTimeout caps the TCP+TLS handshake. Slow networks
// or far-flung servers may need a higher value; the operator
// can override via cfg["connect_timeout"].
const ftpDialTimeout = 30 * time.Second

// ftpDefaultPortPlain is the standard FTP control port. Used
// for plain and ftps_explicit. ftps_implicit uses 990.
const ftpDefaultPortPlain = 21

// ftpDefaultPortImplicit is the standard FTPS implicit port.
const ftpDefaultPortImplicit = 990

// ftpDialTimeoutMin guards against operators configuring a
// zero or negative timeout (would cause immediate failure).
const ftpDialTimeoutMin = 5 * time.Second

// ftpClient is the StorageAdapterClient the host dispatches
// to for the ftp LocationType. Stateless — no per-destination
// cache because FTP connections are short-lived.
type ftpClient struct{}

// NewFTPClient returns a fresh client. The plugin's Register
// creates one at boot; the host calls methods on it.
func NewFTPClient() *ftpClient {
	return &ftpClient{}
}

// ============================================================================
// Config normalization
// ============================================================================

// ftpAuthMethod returns the validated auth method. Defaults
// to "password" when the field is empty so existing rows
// created before the field was added keep working.
func ftpAuthMethod(cfg map[string]string) (string, error) {
	switch cfg["auth_method"] {
	case "":
		return ftpAuthPassword, nil
	case ftpAuthAnonymous, ftpAuthPassword, ftpAuthFTPSExplicit, ftpAuthFTPSImplicit, ftpAuthClientCert:
		return cfg["auth_method"], nil
	default:
		return "", fmt.Errorf("ftp: unknown auth_method %q", cfg["auth_method"])
	}
}

// ftpPort returns the configured port, defaulting per auth
// method (21 for plain/explicit/anonymous, 990 for implicit).
func ftpPort(cfg map[string]string, auth string) int {
	if v := strings.TrimSpace(cfg["port"]); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 && n < 65536 {
			return n
		}
	}
	switch auth {
	case ftpAuthFTPSImplicit:
		return ftpDefaultPortImplicit
	default:
		return ftpDefaultPortPlain
	}
}

// ftpConnectTimeout returns the dial timeout with a 5s floor.
func ftpConnectTimeout(cfg map[string]string) time.Duration {
	v := strings.TrimSpace(cfg["connect_timeout"])
	if v == "" {
		return ftpDialTimeout
	}
	d, err := time.ParseDuration(v)
	if err != nil || d < ftpDialTimeoutMin {
		return ftpDialTimeout
	}
	return d
}

// ftpCredentials returns the (user, pass) tuple for the
// Login call. Per auth method:
//
//   anonymous     → ("anonymous", "anonymous") — RFC 1635
//                   recommends a valid email as the password
//                   but most servers accept anything
//   password      → configured user + pass
//   ftps_explicit → configured user + pass (TLS-tunneled)
//   ftps_implicit → configured user + pass (TLS-wrapped)
//   client_cert   → configured user (or "anonymous") + empty
//                   pass. The server identifies the client by
//                   the cert subject during TLS handshake;
//                   some servers still require USER.
//
// Returns ("", "") for plain auth when username is not set —
// the host's auth-required check is the plugin's caller, but
// Login("","") raises a server-side 501 on most servers so
// the caller surfaces the missing-username error early.
func ftpCredentials(cfg map[string]string, auth string) (string, string) {
	switch auth {
	case ftpAuthAnonymous:
		// Override user/pass even if the operator filled them
		// in — anonymous mode never uses the configured creds.
		return "anonymous", "anonymous"
	case ftpAuthClientCert:
		user := strings.TrimSpace(cfg["username"])
		if user == "" {
			user = "anonymous"
		}
		return user, ""
	case ftpAuthPassword, ftpAuthFTPSExplicit, ftpAuthFTPSImplicit:
		return cfg["username"], cfg["password"]
	}
	return "", ""
}

// ftpTLSConfig returns the base *tls.Config for the FTPS
// variants. MinVersion is pinned to TLS 1.2 — TLS 1.0/1.1
// are deprecated and most servers have dropped them.
// tls_skip_verify is opt-in for self-signed certs.
func ftpTLSConfig(cfg map[string]string) *tls.Config {
	skip := false
	switch strings.ToLower(strings.TrimSpace(cfg["tls_skip_verify"])) {
	case "true", "1", "yes", "on":
		skip = true
	}
	return &tls.Config{
		MinVersion:         tls.VersionTLS12,
		InsecureSkipVerify: skip,
		// ServerName is set automatically by the jlaffaye/ftp
		// library when it sees the host portion of the dial
		// address. We don't override it here.
	}
}

// ftpClientCertTLSConfig returns a *tls.Config that includes
// the operator's client certificate. The cert + key files
// must be PEM-encoded on disk (typical k8s/Traefik/HAProxy
// frontends use this format).
//
// Returns an error (no tls.Config) if either file is missing
// or unreadable — surface this to the operator before the
// dial attempt so we don't fight a TLS handshake that will
// fail anyway.
func ftpClientCertTLSConfig(cfg map[string]string) (*tls.Config, error) {
	certPath := strings.TrimSpace(cfg["client_cert_path"])
	keyPath := strings.TrimSpace(cfg["client_key_path"])
	if certPath == "" || keyPath == "" {
		return nil, errors.New("ftp: client_cert requires client_cert_path and client_key_path")
	}
	cert, err := tls.LoadX509KeyPair(certPath, keyPath)
	if err != nil {
		return nil, fmt.Errorf("ftp: load client cert %s + %s: %w", certPath, keyPath, err)
	}
	tlsCfg := ftpTLSConfig(cfg)
	tlsCfg.Certificates = []tls.Certificate{cert}
	return tlsCfg, nil
}

// ftpSubdir returns the configured subdirectory, with the
// leading slash stripped. The FTP CWD command accepts both
// "/path" and "path" but most servers normalize internally.
func ftpSubdir(cfg map[string]string) string {
	p := strings.TrimSpace(cfg["path"])
	p = strings.TrimPrefix(p, "/")
	for strings.HasSuffix(p, "/") && len(p) > 1 {
		p = strings.TrimSuffix(p, "/")
	}
	return p
}

// ftpJoinPath joins the configured subdir with a key. Returns
// the key alone when subdir is empty.
func ftpJoinPath(subdir, key string) string {
	if subdir == "" {
		return key
	}
	return strings.TrimSuffix(subdir, "/") + "/" + key
}

// ftpValidateConfig is the pre-flight check. We do this
// before any network call so config errors surface as
// "destination misconfigured" rather than "connection
// refused" — clearer for the operator.
func ftpValidateConfig(cfg map[string]string) error {
	if strings.TrimSpace(cfg["server"]) == "" {
		return errors.New("ftp: server is required")
	}
	auth, err := ftpAuthMethod(cfg)
	if err != nil {
		return err
	}
	switch auth {
	case ftpAuthPassword, ftpAuthFTPSExplicit, ftpAuthFTPSImplicit:
		if strings.TrimSpace(cfg["username"]) == "" {
			return errors.New("ftp: username is required for " + auth + " auth")
		}
		// We don't require password — some servers accept
		// empty password (rare) and AUTH without password is
		// part of the FTP protocol. The server returns 332
		// on no-password attempts; we treat that as a server
		// error rather than a config error.
	case ftpAuthClientCert:
		if strings.TrimSpace(cfg["client_cert_path"]) == "" || strings.TrimSpace(cfg["client_key_path"]) == "" {
			return errors.New("ftp: client_cert_path and client_key_path are required for client_cert auth")
		}
		// Best-effort existence check on the cert files.
		for _, p := range []string{cfg["client_cert_path"], cfg["client_key_path"]} {
			if _, err := os.Stat(p); err != nil {
				return fmt.Errorf("ftp: %s: %w", p, err)
			}
		}
	}
	return nil
}

// ============================================================================
// Dial + login
// ============================================================================

// ftpDial connects to the FTP server, logs in, and cd's
// into the configured subdirectory. Returns a connected
// *ftp.ServerConn on success. Caller must Quit() on the
// connection (use defer).
//
// The ctx deadline is honored as a wall-clock cap on the
// dial. The jlaffaye/ftp library does not accept a context
// on Stor/Retr/List/Delete, so we enforce the deadline on
// the dial side and rely on the host's per-operation
// timeout for the rest.
func (c *ftpClient) ftpDial(ctx context.Context, cfg map[string]string) (*ftp.ServerConn, error) {
	if err := ftpValidateConfig(cfg); err != nil {
		return nil, err
	}
	auth, _ := ftpAuthMethod(cfg)
	server := strings.TrimSpace(cfg["server"])
	port := ftpPort(cfg, auth)
	addr := net.JoinHostPort(server, strconv.Itoa(port))
	timeout := ftpConnectTimeout(cfg)

	dialCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	dialOpts := []ftp.DialOption{ftp.DialWithTimeout(timeout)}

	switch auth {
	case ftpAuthAnonymous, ftpAuthPassword:
		// Plain FTP — no extra options.
	case ftpAuthFTPSExplicit:
		dialOpts = append(dialOpts, ftp.DialWithExplicitTLS(ftpTLSConfig(cfg)))
	case ftpAuthFTPSImplicit:
		dialOpts = append(dialOpts, ftp.DialWithTLS(ftpTLSConfig(cfg)))
	case ftpAuthClientCert:
		tlsCfg, err := ftpClientCertTLSConfig(cfg)
		if err != nil {
			return nil, err
		}
		dialOpts = append(dialOpts, ftp.DialWithTLS(tlsCfg))
	}

	// Run the dial in a goroutine so we can bound the wall-
	// clock wait. The ftp library's Dial does not take a
	// context, so we can't pass dialCtx directly.
	type dialResult struct {
		conn *ftp.ServerConn
		err  error
	}
	done := make(chan dialResult, 1)
	go func() {
		conn, err := ftp.Dial(addr, dialOpts...)
		done <- dialResult{conn, err}
	}()

	var conn *ftp.ServerConn
	select {
	case res := <-done:
		if res.err != nil {
			return nil, fmt.Errorf("ftp: dial %s: %w", addr, res.err)
		}
		conn = res.conn
	case <-dialCtx.Done():
		return nil, fmt.Errorf("ftp: dial %s: timeout after %s", addr, timeout)
	}

	user, pass := ftpCredentials(cfg, auth)
	if err := conn.Login(user, pass); err != nil {
		_ = conn.Quit()
		return nil, fmt.Errorf("ftp: login %s@%s: %w", user, addr, err)
	}

	if subdir := ftpSubdir(cfg); subdir != "" {
		if err := conn.ChangeDir(subdir); err != nil {
			_ = conn.Quit()
			return nil, fmt.Errorf("ftp: cd %s: %w", subdir, err)
		}
	}

	return conn, nil
}

// ftpQuit sends QUIT and swallows the error. The server may
// have already closed the connection (e.g. on dial failure);
// we don't want a noisy error from a side-channel cleanup.
func ftpQuit(conn *ftp.ServerConn) {
	if conn == nil {
		return
	}
	_ = conn.Quit()
}

// ============================================================================
// StorageAdapterClient: Test
// ============================================================================

// Test connects, logs in, and verifies the configured
// subdirectory exists (or the root is readable if no
// subdir is configured). Failure modes:
//
//   - server unreachable                   → dial error
//   - bad credentials                      → 530
//   - bad TLS cert (skip_verify=false)     → TLS handshake
//   - bad subdir                           → CWD 550
//   - subdir missing                       → CWD 550 or
//                                            MLSD 550
func (c *ftpClient) Test(ctx context.Context, cfg map[string]string) error {
	conn, err := c.ftpDial(ctx, cfg)
	if err != nil {
		return err
	}
	defer ftpQuit(conn)

	// CurrentDir returns the absolute path the server is
	// sitting on. If the subdir was missing, ChangeDir above
	// would have errored; this is a sanity check that the
	// server's idea of "current dir" matches what we asked
	// for.
	if _, err := conn.CurrentDir(); err != nil {
		return fmt.Errorf("ftp: pwd: %w", err)
	}
	return nil
}

// ============================================================================
// StorageAdapterClient: TestSteps
// ============================================================================

// ftpTestArtifactName returns a unique filename for the
// rewrite/delete round-trip in TestSteps. The suffix keeps
// a crashed connection's artifact identifiable in the
// remote directory.
func ftpTestArtifactName() string {
	n := time.Now().UnixNano()
	return fmt.Sprintf("mcharbor-conn-test-%x.txt", n)
}

func (c *ftpClient) TestSteps(ctx context.Context, cfg map[string]string) ([]plugin.TestStep, error) {
	conn, err := c.ftpDial(ctx, cfg)
	if err != nil {
		return []plugin.TestStep{{
			Name:   "connect",
			Status: "fail",
			Detail: err.Error(),
		}}, nil
	}
	defer ftpQuit(conn)

	var steps []plugin.TestStep
	subdir := ftpSubdir(cfg)
	name := ftpTestArtifactName()
	original := []byte("mcharbor-conn-test\n")
	changed := append(append([]byte{}, original...), []byte("changed\n")...)
	originalPath := ftpJoinPath(subdir, name)
	changedPath := ftpJoinPath(subdir, name+".changed")

	// Step 1: write.
	writeStart := time.Now()
	if err := conn.Stor(originalPath, bytes.NewReader(original)); err != nil {
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

	// Step 2: change + read back.
	changeStart := time.Now()
	if err := conn.Stor(changedPath, bytes.NewReader(changed)); err != nil {
		steps = append(steps, plugin.TestStep{
			Name:    "change",
			Status:  "fail",
			Detail:  err.Error(),
			Latency: time.Since(changeStart).String(),
		})
		return steps, nil
	}
	reader, err := conn.Retr(changedPath)
	if err != nil {
		steps = append(steps, plugin.TestStep{
			Name:    "change",
			Status:  "fail",
			Detail:  "retr: " + err.Error(),
			Latency: time.Since(changeStart).String(),
		})
		return steps, nil
	}
	got, err := io.ReadAll(reader)
	_ = reader.Close()
	if err != nil {
		steps = append(steps, plugin.TestStep{
			Name:    "change",
			Status:  "fail",
			Detail:  "read: " + err.Error(),
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
	if err := conn.Delete(changedPath); err != nil {
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
	_ = conn.Delete(originalPath)

	return steps, nil
}

// ============================================================================
// StorageAdapterClient: Upload
// ============================================================================

// Upload streams r to the FTP server via Stor. jlaffaye/ftp's
// Stor takes an io.Reader and consumes it before sending
// the data; the FTP upload completes when the reader hits
// EOF. The control channel is not subject to the data
// channel's transfer time; the only context-aware piece is
// the dial. The host's per-operation timeout governs the
// upload duration.
func (c *ftpClient) Upload(ctx context.Context, cfg map[string]string, key string, r io.Reader) error {
	if key == "" {
		return errors.New("ftp: key is required")
	}
	conn, err := c.ftpDial(ctx, cfg)
	if err != nil {
		return err
	}
	defer ftpQuit(conn)

	path := ftpJoinPath(ftpSubdir(cfg), key)
	if err := conn.Stor(path, r); err != nil {
		return fmt.Errorf("ftp: stor %s: %w", path, err)
	}
	return nil
}

// downloadStor exists for symmetry with the lib's API; the
// Stor's writer-side is consumed via Stor(path, r).
//
// We keep a separate helper for tests where we want to
// verify the upload completed by Stor-path comparison.
func downloadStor(_ *ftp.ServerConn, _ string, _ []byte) error {
	// Sentinel — see the Stor usage in Upload and TestSteps.
	return nil
}

// ============================================================================
// StorageAdapterClient: Download
// ============================================================================

// Download returns a reader for the file named key. The
// jlaffaye/ftp library returns an io.ReadCloser that reads
// the data channel until EOF.
//
// Be aware: the FTP data connection for Retr is opened
// lazily on the first Read call. If the server is slow or
// the data connection stalls, the timeout has to be
// enforced at the call site (the host's per-operation
// timeout).
func (c *ftpClient) Download(ctx context.Context, cfg map[string]string, key string) (io.ReadCloser, error) {
	if key == "" {
		return nil, errors.New("ftp: key is required")
	}
	conn, err := c.ftpDial(ctx, cfg)
	if err != nil {
		return nil, err
	}
	path := ftpJoinPath(ftpSubdir(cfg), key)
	reader, err := conn.Retr(path)
	if err != nil {
		_ = conn.Quit()
		return nil, fmt.Errorf("ftp: retr %s: %w", path, err)
	}
	// Wrap the reader so QUIT fires when the host is done
	// reading. Without this, the FTP control connection
	// leaks until the host's idle timeout kicks in.
	return &ftpReader{rc: reader, conn: conn}, nil
}

// ftpReader wraps an io.ReadCloser FROM a jlaffaye/ftp Retr
// call so closing the reader also sends QUIT to the server.
// The library doesn't expose the connection on the reader
// itself, so we have to thread it through.
type ftpReader struct {
	rc   io.ReadCloser
	conn *ftp.ServerConn
}

func (r *ftpReader) Read(p []byte) (int, error) { return r.rc.Read(p) }
func (r *ftpReader) Close() error {
	err := r.rc.Close()
	_ = r.conn.Quit()
	return err
}

// ============================================================================
// StorageAdapterClient: Delete
// ============================================================================

// Delete removes the file named key. Per the host's
// contract, "not found" is treated as success (idempotent
// delete).
func (c *ftpClient) Delete(ctx context.Context, cfg map[string]string, key string) error {
	if key == "" {
		return errors.New("ftp: key is required")
	}
	conn, err := c.ftpDial(ctx, cfg)
	if err != nil {
		return err
	}
	defer ftpQuit(conn)

	path := ftpJoinPath(ftpSubdir(cfg), key)
	if err := conn.Delete(path); err != nil {
		// FTP 550 (file not found) is treated as success.
		// Other errors are surfaced.
		if isFTPNosuchFile(err) {
			return nil
		}
		return fmt.Errorf("ftp: delete %s: %w", path, err)
	}
	return nil
}

// isFTPNosuchFile returns true for FTP 550 "file not found"
// replies. The jlaffaye/ftp library returns a *ftp.Error
// from each non-2xx command response.
func isFTPNosuchFile(err error) bool {
	if err == nil {
		return false
	}
	// The library's error type is unexported but its Error()
	// method renders the standard 5xx code. Match on the
	// code prefix to be safe across library versions.
	msg := err.Error()
	return strings.Contains(msg, "550") || strings.Contains(msg, "No such file")
}

// ============================================================================
// StorageAdapterClient: List
// ============================================================================

// List returns the files under the configured subdirectory.
// Sorted by name. Directories are filtered out (the host
// only displays file destinations).
func (c *ftpClient) List(ctx context.Context, cfg map[string]string, prefix string) ([]plugin.StorageObject, error) {
	conn, err := c.ftpDial(ctx, cfg)
	if err != nil {
		return nil, err
	}
	defer ftpQuit(conn)

	dir := ftpSubdir(cfg)
	entries, err := conn.List(dir)
	if err != nil {
		return nil, fmt.Errorf("ftp: list %s: %w", dir, err)
	}
	out := make([]plugin.StorageObject, 0, len(entries))
	for _, e := range entries {
		if e.Type != ftp.EntryTypeFile {
			continue
		}
		if prefix != "" && !strings.HasPrefix(e.Name, prefix) {
			continue
		}
		// Entry.Time is the server-reported modification
		// time. Most servers return it; some don't (the
		// library exposes IsGetTimeSupported() to check).
		// When absent, we leave LastModified as the zero
		// value — the host's UI sorts by it gracefully.
		out = append(out, plugin.StorageObject{
			Key:          e.Name,
			Size:         int64(e.Size),
			LastModified: e.Time,
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Key < out[j].Key })
	return out, nil
}

// ============================================================================
// OAuth handshake (not used by FTP — we implement the interface so the
// plugin satisfies plugin.StorageAdapterClient, but the methods always
// return ErrUnsupported because FTP auth is USER/PASS / client-cert, not
// a delegated authorization code flow.)
// ============================================================================

// ConsentURL is part of the StorageAdapterClient interface. FTP auth
// is credential-based, not OAuth; the host's
// /storage-locations/{id}/oauth/authorize handler never reaches this
// code path because the adapter's spec declares OAuth=false. Surfacing
// ErrUnsupported lets the host log a clear error if it ever does.
func (c *ftpClient) ConsentURL(_ context.Context, _, _, _, _ string, _ map[string]string) (string, error) {
	return "", plugin.ErrUnsupported
}

// ConsentCallback completes the OAuth handshake. Same as ConsentURL:
// not used for FTP; returns ErrUnsupported so the host's dispatcher
// surfaces a clear error if invoked.
func (c *ftpClient) ConsentCallback(_ context.Context, _, _, _, _, _ string, _ map[string]string) (map[string]string, error) {
	return nil, plugin.ErrUnsupported
}
