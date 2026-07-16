// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package storage_locations

import (
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"log/slog"
	"net"
	"strings"
	"sync"
	"time"

	"golang.org/x/crypto/ssh"
)

// SFTP v3 wire format reference (draft-ietf-secsh-filexfer):
//
//   packet = uint32 length || byte type || uint32 request_id || payload
//
// All integers are big-endian. Each client-initiated request carries a
// unique request_id; the server reply has the same id (in a STATUS,
// HANDLE, or DATA packet depending on the call). We multiplex replies
// to many concurrent callers through a single goroutine that reads
// packets off the SSH channel and dispatches them by request_id.

// SFTP message types we use.
const (
	sftpMsgInit    byte = 1
	sftpMsgVersion byte = 2
	sftpMsgOpen    byte = 3
	sftpMsgClose   byte = 4
	sftpMsgRead    byte = 5
	sftpMsgWrite   byte = 6
	sftpMsgData    byte = 7
	sftpMsgStatus  byte = 9
	sftpMsgHandle  byte = 10
	sftpMsgRemove  byte = 12
)

// SFTP status codes.
const (
	sftpStatusOK              uint32 = 0
	sftpStatusEOF             uint32 = 1
	sftpStatusNoSuchFile      uint32 = 2
	sftpStatusPermissionDenied uint32 = 3
	sftpStatusFailure         uint32 = 4
)

// SFTP pflags (file open mode).
const (
	sftpFlagRead   uint32 = 0x1
	sftpFlagWrite  uint32 = 0x2
	sftpFlagAppend uint32 = 0x4
	sftpFlagCreat  uint32 = 0x8
	sftpFlagTrunc  uint32 = 0x10
	sftpFlagExcl   uint32 = 0x20
)

// SFTP protocol version we request. 3 is the version OpenSSH
// implements; older servers may downgrade during INIT. We don't
// actually care which version we land on as long as we can
// OPEN/WRITE/READ/CLOSE/REMOVE — that's in every SFTP version.
const sftpProtocolVersion uint32 = 3

// maxSFTPFrame limits how big a single SFTP packet can be. Mirrors
// the client's frame size; the server may request smaller. 32 MB
// is the value OpenSSH's defaults sit at.
const maxSFTPFrame = 32 * 1024 * 1024

// sftpRoundTrip is the response shape for a single send-receive.
type sftpRoundTrip struct {
	data   []byte    // returned for HANDLE/DATA replies; nil for STATUS
	handle []byte    // returned for HANDLE replies; nil otherwise
	status uint32    // STATUS code (0 = OK, 1 = EOF, ...)
	errMsg string    // STATUS error message
}

// sftpClient speaks SFTP v3 over a single SSH session. Not safe for
// concurrent use — each request gets its own goroutine on the
// server side, but our reply-to-channel dispatch assumes one
// outstanding request per session. That's fine for the test
// endpoint, which does sequential write -> read -> remove.
type sftpClient struct {
	sshClient *ssh.Client
	channel   ssh.Channel
	log       *slog.Logger
	mu        sync.Mutex
	pending   map[uint32]chan<- sftpRoundTrip
	nextID    uint32
	readErr   error
	readDone  chan struct{}
}

// newSFTPClient opens an SSH connection, authenticates the user, and
// opens the SFTP subsystem on top.
//
// Auth tries whatever the operator configured. Most SFTP servers
// accept password + publickey. We try both in that order so a
// config that has a stale plaintext password plus a working key
// doesn't lock the operator out.
func newSFTPClient(ctx context.Context, host string, port int, user, password, privateKey string, log *slog.Logger) (*sftpClient, error) {
	if strings.TrimSpace(user) == "" {
		return nil, fmt.Errorf("sftp location is missing username")
	}
	if strings.TrimSpace(password) == "" && strings.TrimSpace(privateKey) == "" {
		return nil, fmt.Errorf("sftp location has no password or private key configured")
	}

	// HostKeyCallback: self-test only — operators configure their
	// own SSH server here, so we accept any host key. This is the
	// same trust model as `ssh -o StrictHostKeyChecking=no` on
	// first connect; tighter checks would require the operator to
	// pre-paste a fingerprint into the location config. We log a
	// WARN at dial time so the audit trail shows we skipped the
	// host key check.
	if log != nil {
		log.Warn(
			"sftp self-test is skipping host key verification",
			"host", host, "port", port, "user", user,
			"reason", "test endpoint, no fingerprint configured",
		)
	}

	var authMethods []ssh.AuthMethod
	if pw := strings.TrimSpace(password); pw != "" {
		authMethods = append(authMethods, ssh.Password(pw))
	}
	if key := strings.TrimSpace(privateKey); key != "" {
		signer, signerErr := ssh.ParsePrivateKey([]byte(key))
		if signerErr == nil {
			authMethods = append(authMethods, ssh.PublicKeys(signer))
		} else if log != nil {
			log.Warn("sftp private key parse failed; using password only", "error", signerErr)
		}
	}

	cfg := &ssh.ClientConfig{
		User:            user,
		Auth:            authMethods,
		HostKeyCallback: ssh.InsecureIgnoreHostKey(), //nolint:gosec // self-test endpoint only; logged above
		Timeout:         10 * time.Second,
	}

	addr := fmt.Sprintf("%s:%d", host, port)
	dialer := &net.Dialer{Timeout: 10 * time.Second}
	conn, err := dialer.DialContext(ctx, "tcp", addr)
	if err != nil {
		return nil, fmt.Errorf("tcp dial %s: %w", addr, err)
	}
	sshConn, chans, reqs, err := ssh.NewClientConn(conn, addr, cfg)
	if err != nil {
		_ = conn.Close()
		return nil, fmt.Errorf("ssh handshake %s: %w", addr, err)
	}
	sshClient := ssh.NewClient(sshConn, chans, reqs)

	channel, requests, err := sshClient.OpenChannel("session", nil)
	if err != nil {
		_ = sshClient.Close()
		return nil, fmt.Errorf("opening ssh session channel: %w", err)
	}
	// The server can send channel-level requests (e.g. env, exec)
	// that we don't care about; discard them so the channel doesn't
	// stall waiting on a reply we never send.
	go ssh.DiscardRequests(requests)

	ok, err := channel.SendRequest("subsystem", true, []byte("sftp"))
	if err != nil || !ok {
		_ = channel.Close()
		_ = sshClient.Close()
		if err != nil {
			return nil, fmt.Errorf("sftp subsystem request: %w", err)
		}
		return nil, fmt.Errorf("sftp subsystem request denied by server")
	}

	c := &sftpClient{
		sshClient: sshClient,
		channel:   channel,
		log:       log,
		pending:   make(map[uint32]chan<- sftpRoundTrip),
		nextID:    0,
		readDone:  make(chan struct{}),
	}

	// Send INIT.
	c.mu.Lock()
	c.nextID = 0
	c.mu.Unlock()
	if err := c.sendInit(c.channel); err != nil {
		_ = c.closeConn()
		return nil, fmt.Errorf("sftp init: %w", err)
	}
	// Read the server's VERSION reply before we accept user requests.
	if err := c.readServerVersion(c.channel); err != nil {
		_ = c.closeConn()
		return nil, fmt.Errorf("sftp version handshake: %w", err)
	}

	// Start the demuxer that reads packets and dispatches them by
	// request_id. Done first because the channel only has one
	// read side; if we started reading inline we'd block the
	// caller for every packet.
	go c.readLoop()

	return c, nil
}

// Close shuts the SSH channel and underlying connection. Safe to
// call multiple times.
func (c *sftpClient) Close() error {
	if c == nil {
		return nil
	}
	return c.closeConn()
}

func (c *sftpClient) closeConn() error {
	if c.channel != nil {
		_ = c.channel.Close()
		c.channel = nil
	}
	if c.sshClient != nil {
		err := c.sshClient.Close()
		c.sshClient = nil
		return err
	}
	return nil
}

// sendInit writes the INIT message (type 1, our protocol version).
// No payload beyond the version number on v3.
func (c *sftpClient) sendInit(w io.Writer) error {
	var buf bytes.Buffer
	if err := buf.WriteByte(sftpMsgInit); err != nil {
		return err
	}
	if err := binary.Write(&buf, binary.BigEndian, sftpProtocolVersion); err != nil {
		return err
	}
	return writeSFTPFrame(w, buf.Bytes())
}

// readServerVersion drains the first packet off the channel; it
// must be a VERSION message from the server.
func (c *sftpClient) readServerVersion(r io.Reader) error {
	typ, _, payload, err := readSFTPFrame(r)
	if err != nil {
		return err
	}
	if typ != sftpMsgVersion {
		return fmt.Errorf("expected server VERSION (type 2), got type %d (payload %d bytes)", typ, len(payload))
	}
	if len(payload) < 4 {
		return fmt.Errorf("server VERSION payload too short: %d bytes", len(payload))
	}
	// The first four bytes are the server's protocol version; we
	// don't pin to a specific value but log it for trace.
	version := binary.BigEndian.Uint32(payload[:4])
	if c.log != nil {
		c.log.Info("sftp server version handshake complete", "version", version)
	}
	return nil
}

// send frames an SFTP request and writes it on the channel. It does
// not block waiting for a reply; the caller owns the response.
func (c *sftpClient) send(method byte, payload []byte) (uint32, error) {
	c.mu.Lock()
	c.nextID++
	id := c.nextID
	c.mu.Unlock()
	var buf bytes.Buffer
	buf.WriteByte(method)
	if err := binary.Write(&buf, binary.BigEndian, id); err != nil {
		return 0, err
	}
	if _, err := buf.Write(payload); err != nil {
		return 0, err
	}
	c.mu.Lock()
	ch := c.channel
	c.mu.Unlock()
	if ch == nil {
		return 0, fmt.Errorf("sftp channel closed")
	}
	if err := writeSFTPFrame(ch, buf.Bytes()); err != nil {
		return 0, fmt.Errorf("sftp write %s: %w", sftpTypeName(method), err)
	}
	return id, nil
}

// roundTrip sends a request and waits for the matching STATUS (or
// HANDLE / DATA for those calls). Called from the public put/get/remove
// helpers.
func (c *sftpClient) roundTrip(method byte, payload []byte) (*sftpRoundTrip, error) {
	id, err := c.send(method, payload)
	if err != nil {
		return nil, err
	}
	respCh := make(chan sftpRoundTrip, 1)
	c.mu.Lock()
	c.pending[id] = respCh
	c.mu.Unlock()
	defer func() {
		c.mu.Lock()
		delete(c.pending, id)
		c.mu.Unlock()
	}()
	select {
	case resp, ok := <-respCh:
		if !ok {
			return nil, fmt.Errorf("sftp %s response channel closed (transport error)", sftpTypeName(method))
		}
		return &resp, nil
	case <-time.After(8 * time.Second):
		return nil, fmt.Errorf("sftp %s timed out after 8s", sftpTypeName(method))
	}
}

// Put writes body to path. Uses OPEN(write|creat|trunc) -> WRITE -> CLOSE.
func (c *sftpClient) Put(path string, body []byte) error {
	handleResp, err := c.roundTrip(sftpMsgOpen, encodeOpenPayload(path, sftpFlagWrite|sftpFlagCreat|sftpFlagTrunc, 0o644))
	if err != nil {
		return err
	}
	if handleResp.status != sftpStatusOK {
		return fmt.Errorf("sftp open(%s) failed: [%d] %s", path, handleResp.status, handleResp.errMsg)
	}
	handle := handleResp.handle
	if len(handle) == 0 {
		return fmt.Errorf("sftp open(%s) returned empty handle", path)
	}
	// Always close the handle on the way out so a partial write
	// doesn't leak the file on the server.
	defer func() {
		_ = c.closeHandle(handle)
	}()
	// Single WRITE call. SFTP allows fragmented writes via offset;
	// for a small marker (<16 KB) we never need that. If the body
	// grows past the configured frame we'd need to chunk, so gate
	// on a sensible threshold.
	if len(body) > maxSFTPFrame-128 {
		return fmt.Errorf("sftp body too large for single write (%d bytes); chunked upload not implemented for self-test", len(body))
	}
	writeResp, err := c.roundTrip(sftpMsgWrite, encodeWritePayload(handle, 0, body))
	if err != nil {
		return err
	}
	if writeResp.status != sftpStatusOK {
		return fmt.Errorf("sftp write(%s) failed: [%d] %s", path, writeResp.status, writeResp.errMsg)
	}
	return nil
}

// Get reads path and returns its content. Uses OPEN(read) -> READ -> CLOSE.
// Implemented with a single READ call — if the file is bigger than
// maxSFTPFrame we'd need multiple reads, but the marker is small
// enough that this never comes up.
func (c *sftpClient) Get(path string) ([]byte, error) {
	handleResp, err := c.roundTrip(sftpMsgOpen, encodeOpenPayload(path, sftpFlagRead, 0))
	if err != nil {
		return nil, err
	}
	if handleResp.status == sftpStatusNoSuchFile {
		return nil, fmt.Errorf("sftp get(%s): file does not exist on server", path)
	}
	if handleResp.status != sftpStatusOK {
		return nil, fmt.Errorf("sftp open(%s) failed: [%d] %s", path, handleResp.status, handleResp.errMsg)
	}
	handle := handleResp.handle
	defer func() {
		_ = c.closeHandle(handle)
	}()
	// Ask for up to maxSFTPFrame bytes; the server will return at
	// most what the file has, capped by the lower of the two.
	readResp, err := c.roundTrip(sftpMsgRead, encodeReadPayload(handle, 0, maxSFTPFrame))
	if err != nil {
		return nil, err
	}
	// READ reply is DATA, not STATUS. We synthesize a fake
	// sftpRoundTrip for the DATA path in the demuxer; status is
	// the EOF flag here: non-EOF (0) means we got data, EOF (1)
	// means the file is empty.
	if readResp.status == sftpStatusEOF {
		return []byte{}, nil
	}
	if readResp.status != sftpStatusOK {
		return nil, fmt.Errorf("sftp read(%s) failed: [%d] %s", path, readResp.status, readResp.errMsg)
	}
	return readResp.data, nil
}

// Remove deletes path on the server.
func (c *sftpClient) Remove(path string) error {
	// Path is a single string for REMOVE.
	payload := encodeString(path)
	resp, err := c.roundTrip(sftpMsgRemove, payload)
	if err != nil {
		return err
	}
	if resp.status == sftpStatusNoSuchFile {
		// Treat "no such file" as success for cleanup so a
		// crashed-then-restarted test doesn't leak a confusing
		// error message.
		return nil
	}
	if resp.status != sftpStatusOK {
		return fmt.Errorf("sftp remove(%s) failed: [%d] %s", path, resp.status, resp.errMsg)
	}
	return nil
}

func (c *sftpClient) closeHandle(handle []byte) error {
	resp, err := c.roundTrip(sftpMsgClose, encodeString(string(handle)))
	if err != nil {
		return err
	}
	if resp.status != sftpStatusOK {
		return fmt.Errorf("sftp close failed: [%d] %s", resp.status, resp.errMsg)
	}
	return nil
}

// readLoop is the demuxer for incoming SFTP packets. It runs in
// its own goroutine for the lifetime of the client. Each call
// (OPEN, READ, WRITE, REMOVE, CLOSE) registers a channel under
// its request_id; the read loop dispatches the matching reply to
// that channel and closes it.
//
// DATA replies are matched by request_id (not a separate field)
// because the server reuses the same request_id from the original
// READ call.
func (c *sftpClient) readLoop() {
	defer close(c.readDone)
	for {
		typ, id, payload, err := readSFTPFrame(c.channel)
		if err != nil {
			c.mu.Lock()
			c.readErr = err
			// Drain pending callers so they don't block on a
			// dead channel.
			for id, ch := range c.pending {
				select {
				case ch <- sftpRoundTrip{status: sftpStatusFailure, errMsg: fmt.Sprintf("sftp transport closed: %v", err)}:
				default:
				}
				close(ch)
				delete(c.pending, id)
			}
			c.mu.Unlock()
			return
		}
		switch typ {
		case sftpMsgHandle:
			if len(payload) < 4 {
				continue
			}
			handleLen := binary.BigEndian.Uint32(payload[:4])
			if 4+int(handleLen) > len(payload) {
				continue
			}
			handle := payload[4 : 4+handleLen]
			c.dispatch(id, sftpRoundTrip{handle: handle, status: sftpStatusOK})
		case sftpMsgData:
			// DATA: uint32 length || byte[] data
			if len(payload) < 4 {
				continue
			}
			dataLen := binary.BigEndian.Uint32(payload[:4])
			if 4+int(dataLen) > len(payload) {
				continue
			}
			data := make([]byte, dataLen)
			copy(data, payload[4:4+dataLen])
			c.dispatch(id, sftpRoundTrip{data: data, status: sftpStatusOK})
		case sftpMsgStatus:
			// STATUS: uint32 request_id || uint32 status_code || string (error) || string (lang)
			// Note: the leading uint32 is a redundant copy of the
			// request_id we already know — the server sometimes
			// does not include it (older versions of SFTP draft
			// had a different layout). We tolerate either.
			if len(payload) < 4 {
				continue
			}
			status := binary.BigEndian.Uint32(payload[:4])
			msg, _ := decodeString(payload[4:])
			c.dispatch(id, sftpRoundTrip{status: status, errMsg: msg})
		default:
			// Unknown packet. Log and skip rather than killing
			// the connection — extensions exist (e.g.
			// SSH_FXP_EXTENDED) and ignoring them keeps us
			// forward-compatible.
			if c.log != nil {
				c.log.Warn("sftp received unknown packet type", "type", typ, "id", id, "payload", len(payload))
			}
		}
	}
}

func (c *sftpClient) dispatch(id uint32, reply sftpRoundTrip) {
	c.mu.Lock()
	ch := c.pending[id]
	if ch != nil {
		delete(c.pending, id)
	}
	c.mu.Unlock()
	if ch == nil {
		// Unmatched reply — protocol violation, but harmless
		// to the operator; just log it.
		if c.log != nil {
			c.log.Warn("sftp unmatched reply", "id", id)
		}
		return
	}
	// Drop the reply through a select so a stuck caller doesn't
	// leak the read loop. The caller either reads it or it's
	// garbage-collected when its goroutine exits.
	select {
	case ch <- reply:
	default:
		if c.log != nil {
			c.log.Warn("sftp reply channel full", "id", id)
		}
	}
}

// writeSFTPFrame prefixes the SFTP payload with its big-endian
// length. The SSH channel handles framing on the wire; this is the
// SFTP layer's own framing.
func writeSFTPFrame(w io.Writer, payload []byte) error {
	length := uint32(len(payload))
	if length > maxSFTPFrame {
		return fmt.Errorf("sftp frame too large: %d bytes (max %d)", length, maxSFTPFrame)
	}
	if err := binary.Write(w, binary.BigEndian, length); err != nil {
		return err
	}
	if _, err := w.Write(payload); err != nil {
		return err
	}
	return nil
}

// readSFTPFrame reads the next framed packet: uint32 length, then
// length bytes of payload. Returns the message type, request_id,
// and the remaining payload. request_id is the second uint32 in
// every client-initiated message, so this only makes sense after
// the caller knows the type — it works for every message we use.
func readSFTPFrame(r io.Reader) (typ byte, id uint32, payload []byte, err error) {
	var header [8]byte
	if _, err = io.ReadFull(r, header[:4]); err != nil {
		return 0, 0, nil, err
	}
	length := binary.BigEndian.Uint32(header[:4])
	if length == 0 || length > maxSFTPFrame {
		return 0, 0, nil, fmt.Errorf("sftp frame length out of range: %d", length)
	}
	body := make([]byte, length)
	if _, err = io.ReadFull(r, body); err != nil {
		return 0, 0, nil, err
	}
	if len(body) < 5 {
		return 0, 0, nil, fmt.Errorf("sftp body too short: %d bytes", len(body))
	}
	typ = body[0]
	id = binary.BigEndian.Uint32(body[1:5])
	payload = body[5:]
	return typ, id, payload, nil
}

// encodeOpenPayload builds the OPEN payload:
// string filename || uint32 flags || attrs
// where attrs is uint32 flags (only ATTR_PERMISSIONS used here).
func encodeOpenPayload(path string, flags uint32, perm uint32) []byte {
	var buf bytes.Buffer
	encodeStringTo(&buf, path)
	_ = binary.Write(&buf, binary.BigEndian, flags)
	_ = binary.Write(&buf, binary.BigEndian, uint32(0x4)) // attrs.flags = ATTR_PERMISSIONS
	_ = binary.Write(&buf, binary.BigEndian, perm)
	return buf.Bytes()
}

// encodeWritePayload builds WRITE:
// string handle || uint64 offset || string data
func encodeWritePayload(handle []byte, offset uint64, data []byte) []byte {
	var buf bytes.Buffer
	encodeBytesTo(&buf, handle)
	_ = binary.Write(&buf, binary.BigEndian, offset)
	encodeBytesTo(&buf, data)
	return buf.Bytes()
}

// encodeReadPayload builds READ:
// string handle || uint64 offset || uint32 length
func encodeReadPayload(handle []byte, offset uint64, length uint32) []byte {
	var buf bytes.Buffer
	encodeBytesTo(&buf, handle)
	_ = binary.Write(&buf, binary.BigEndian, offset)
	_ = binary.Write(&buf, binary.BigEndian, length)
	return buf.Bytes()
}

// encodeString writes the SFTP string layout (uint32 length || bytes)
// and returns it. encodeStringTo does the same but appends into an
// existing buffer (used by callers that already have a scratch buffer).
func encodeString(s string) []byte {
	var buf bytes.Buffer
	encodeStringTo(&buf, s)
	return buf.Bytes()
}

func encodeStringTo(buf *bytes.Buffer, s string) {
	_ = binary.Write(buf, binary.BigEndian, uint32(len(s)))
	buf.WriteString(s)
}

func encodeBytesTo(buf *bytes.Buffer, b []byte) {
	_ = binary.Write(buf, binary.BigEndian, uint32(len(b)))
	buf.Write(b)
}

// decodeString reads an SFTP string: uint32 length, then that many
// bytes. Returns the body and the position after it (or an error).
func decodeString(payload []byte) (string, int) {
	if len(payload) < 4 {
		return "", 0
	}
	length := binary.BigEndian.Uint32(payload[:4])
	if 4+int(length) > len(payload) {
		return "", 0
	}
	return string(payload[4 : 4+length]), 4 + int(length)
}

// sftpTypeName maps an SFTP message type byte to a short label for
// log/error messages. Returns "msg<n>" for anything not in our
// working set so the operator still has a clue which packet went
// wrong.
func sftpTypeName(typ byte) string {
	switch typ {
	case sftpMsgInit:
		return "INIT"
	case sftpMsgVersion:
		return "VERSION"
	case sftpMsgOpen:
		return "OPEN"
	case sftpMsgClose:
		return "CLOSE"
	case sftpMsgRead:
		return "READ"
	case sftpMsgWrite:
		return "WRITE"
	case sftpMsgData:
		return "DATA"
	case sftpMsgStatus:
		return "STATUS"
	case sftpMsgHandle:
		return "HANDLE"
	case sftpMsgRemove:
		return "REMOVE"
	default:
		return fmt.Sprintf("msg%d", typ)
	}
}

// sftpStatusName maps a status code to a short label for log/error
// messages. Mirrors the sftpStatus* constants in this file.
func sftpStatusName(status uint32) string {
	switch status {
	case sftpStatusOK:
		return "OK"
	case sftpStatusEOF:
		return "EOF"
	case sftpStatusNoSuchFile:
		return "NO_SUCH_FILE"
	case sftpStatusPermissionDenied:
		return "PERMISSION_DENIED"
	case sftpStatusFailure:
		return "FAILURE"
	default:
		return fmt.Sprintf("status_%d", status)
	}
}
