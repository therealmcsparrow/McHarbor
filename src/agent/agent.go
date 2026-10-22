// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// Command mcharbor-agent is the McHarbor remote agent: it connects outbound to
// the server over WebSocket and proxies Docker and host operations for hosts
// that cannot expose their Docker socket.
package main

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"os"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gorilla/websocket"
	"github.com/therealmcsparrow/mcharbor-agent/internal/errlog"
)

// hostOpTimeout bounds one host-side operation the agent runs on behalf of the
// server: a container unpause, a runtime probe, a stats read, an image removal.
const hostOpTimeout = 30 * time.Second

// probeHTTPTimeout bounds one peer-reachability probe, so an unresponsive peer
// cannot hold the caller waiting on a transfer path decision.
const probeHTTPTimeout = 10 * time.Second

const agentVersion = "2026.10.02"

// agentBuild is the yyyyMMddHHmm stamp of this binary, set with -ldflags at
// build time. It stays "dev" for a `go build` from a source tree, which is
// the honest answer for a binary nobody stamped.
var agentBuild = "dev"

// newAgentHTTPClient builds the client the agent uses for plain HTTP(S) calls
// back to the McHarbor server and to peer agents: backup archive upload,
// restore archive download, direct image/archive transfer, transfer probe.
//
// It must honour MCHARBOR_INSECURE for the same reason the WebSocket dialer
// does — a self-signed server certificate whose SANs cover the container
// address but not the LAN address the agent dials would otherwise connect the
// control channel and fail every archive transfer.
// The probe variant passes a short timeout: the server may try several
// candidate addresses for one target, and an unreachable address is often
// black-holed rather than refused, most commonly a container bridge address
// that only exists on the target host. Left to the operating system's TCP
// timeout, one such address would stall the whole selection for minutes; the
// probe is a few bytes, so a short bound is safe. A zero timeout means no
// client-side bound, which is what the archive transfers need.
func newAgentHTTPClient(insecure bool, pin string, timeout time.Duration) *http.Client {
	// Built explicitly rather than left to http.DefaultTransport, because the
	// insecure variant used to replace it wholesale and inherited none of its
	// deadlines. A transfer to a black-holed address then had no bound at all
	// below the caller's context: a 6-hour import context meant six hours of
	// silence for what is a failure to connect.
	transport := &http.Transport{
		Proxy: http.ProxyFromEnvironment,
		DialContext: (&net.Dialer{
			Timeout:   agentDialTimeout,
			KeepAlive: 30 * time.Second,
		}).DialContext,
		ForceAttemptHTTP2:     true,
		MaxIdleConns:          100,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   agentTLSHandshakeTimeout,
		ExpectContinueTimeout: time.Second,
		// Bounds the wait for the *first* byte of the response only, so a long
		// download is unaffected while a server that accepts a connection and
		// then says nothing is not mistaken for slow progress.
		ResponseHeaderTimeout: agentResponseHeaderTimeout,
	}
	// A pin verifies one exact key and therefore outranks the blanket
	// skip-verify; a malformed pin falls through to the old behaviour
	// rather than leaving the agent unable to connect at all.
	if pinned, err := pinnedTLSConfig(pin); err == nil && pinned != nil {
		transport.TLSClientConfig = pinned
	} else if insecure {
		transport.TLSClientConfig = &tls.Config{InsecureSkipVerify: true} //nolint:gosec // opt-in via MCHARBOR_INSECURE, same contract as the WebSocket dialer
	}
	return &http.Client{Timeout: timeout, Transport: transport}
}

// Deadlines every agent HTTP call inherits. None of them bounds a transfer:
// they bound getting one started, which is the part that fails silently.
const (
	agentDialTimeout           = 20 * time.Second
	agentTLSHandshakeTimeout   = 15 * time.Second
	agentResponseHeaderTimeout = 60 * time.Second
)

// Agent handles the WebSocket connection to the McHarbor server.
type Agent struct {
	cfg      Config
	logger   *slog.Logger
	proxy    *Proxy
	transfer *TransferServer
	// offline is nil when local buffering is disabled or its store could
	// not be opened; every use is guarded.
	offline     *OfflineBuffer
	offlineAcks *offlineAcks
	// httpClient carries every non-WebSocket call to the McHarbor server or a
	// peer agent; probeClient is the same client with the short reachability
	// bound. Both honour cfg.Insecure.
	httpClient  *http.Client
	probeClient *http.Client
	// authenticated records whether the connection that is running (or the
	// one that just ended) got past auth. RunWithReconnect reads it to tell
	// "the server dropped a healthy session" from "the server is down", and
	// only the latter should escalate the backoff.
	authenticated atomic.Bool
	// hostLinks holds the desired state of every host link this agent made,
	// replayed at startup because a macvlan child does not survive a reboot.
	hostLinks *hostLinkStore
}

// Authenticated reports whether the most recent connection attempt reached a
// successful auth_result.
func (a *Agent) Authenticated() bool {
	return a.authenticated.Load()
}

type transferProgressReader struct {
	reader     io.Reader
	conn       *websocket.Conn
	transferID string
	stage      string
	bytes      atomic.Int64
	lastEmit   time.Time
}

type uploadBuffer struct {
	ch     chan []byte
	done   chan struct{}
	buf    []byte
	err    error
	closed bool
	mu     sync.Mutex
}

type spooledUpload struct {
	ctx    context.Context //nolint:containedctx // upload-lifetime context paired with its cancel func; the spool outlives the call that created it
	cancel context.CancelFunc
	req    *WSHTTPRequest
	file   *os.File
	path   string
	bytes  int64
}

func newUploadBuffer() *uploadBuffer {
	return &uploadBuffer{
		ch:   make(chan []byte, 256),
		done: make(chan struct{}),
	}
}

func newSpooledUpload(ctx context.Context, req *WSHTTPRequest) (*spooledUpload, error) {
	file, err := os.CreateTemp("", "mcharbor-agent-image-load-upload-*.tar")
	if err != nil {
		return nil, fmt.Errorf("creating temporary image upload archive: %w", err)
	}
	reqCtx, cancel := context.WithCancel(ctx)
	return &spooledUpload{
		ctx:    reqCtx,
		cancel: cancel,
		req:    req,
		file:   file,
		path:   file.Name(),
	}, nil
}

func (u *spooledUpload) Write(data []byte) error {
	n, err := u.file.Write(data)
	u.bytes += int64(n)
	if err != nil {
		return fmt.Errorf("writing temporary image upload archive: %w", err)
	}
	if n != len(data) {
		return io.ErrShortWrite
	}
	return nil
}

func (u *spooledUpload) Rewind() error {
	if _, err := u.file.Seek(0, io.SeekStart); err != nil {
		return fmt.Errorf("rewinding temporary image upload archive: %w", err)
	}
	return nil
}

func (u *spooledUpload) Cleanup(logger *slog.Logger) {
	u.cancel()
	if err := u.file.Close(); err != nil {
		logger.Warn("close temporary image upload archive failed", "error", err, "path", u.path)
	}
	if err := os.Remove(u.path); err != nil && !os.IsNotExist(err) {
		logger.Warn("remove temporary image upload archive failed", "error", err, "path", u.path)
	}
}

func (b *uploadBuffer) Push(data []byte) error {
	b.mu.Lock()
	if b.closed {
		err := b.err
		if err == nil {
			err = io.ErrClosedPipe
		}
		b.mu.Unlock()
		return err
	}
	b.mu.Unlock()

	chunk := make([]byte, len(data))
	copy(chunk, data)
	select {
	case b.ch <- chunk:
		return nil
	case <-b.done:
		b.mu.Lock()
		err := b.err
		b.mu.Unlock()
		if err != nil {
			return err
		}
		return io.ErrClosedPipe
	}
}

func (b *uploadBuffer) Read(p []byte) (int, error) {
	if len(b.buf) > 0 {
		n := copy(p, b.buf)
		b.buf = b.buf[n:]
		return n, nil
	}

	select {
	case chunk := <-b.ch:
		n := copy(p, chunk)
		if n < len(chunk) {
			b.buf = chunk[n:]
		}
		return n, nil
	default:
	}

	select {
	case chunk := <-b.ch:
		n := copy(p, chunk)
		if n < len(chunk) {
			b.buf = chunk[n:]
		}
		return n, nil
	case <-b.done:
		select {
		case chunk := <-b.ch:
			n := copy(p, chunk)
			if n < len(chunk) {
				b.buf = chunk[n:]
			}
			return n, nil
		default:
		}
		b.mu.Lock()
		err := b.err
		b.mu.Unlock()
		if err != nil {
			return 0, err
		}
		return 0, io.EOF
	}
}

func (b *uploadBuffer) Close() error {
	b.closeWithError(nil)
	return nil
}

func (b *uploadBuffer) CloseWithError(err error) error {
	b.closeWithError(err)
	return nil
}

func (b *uploadBuffer) closeWithError(err error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.closed {
		return
	}
	b.closed = true
	b.err = err
	close(b.done)
}

// NewAgent creates a new agent instance.
func NewAgent(cfg Config, logger *slog.Logger) *Agent {
	proxy := NewProxy(cfg.DockerHost, logger)
	return &Agent{
		cfg:         cfg,
		logger:      logger,
		proxy:       proxy,
		transfer:    NewTransferServer(cfg.TransferListen, cfg.TransferAdvertiseURL, cfg.AgentToken, proxy, logger),
		offlineAcks: newOfflineAcks(),
		httpClient:  newAgentHTTPClient(cfg.Insecure, cfg.Pin, 0),
		probeClient: newAgentHTTPClient(cfg.Insecure, cfg.Pin, probeHTTPTimeout),
		hostLinks:   newHostLinkStore(cfg.DataDir),
	}
}

// SetOfflineBuffer attaches the local buffer used while disconnected.
func (a *Agent) SetOfflineBuffer(b *OfflineBuffer) { a.offline = b }

// MarkOffline records a failed or lost connection so the buffer starts
// collecting. No-op when buffering is disabled.
func (a *Agent) MarkOffline(ctx context.Context) {
	if a.offline != nil {
		a.offline.MarkOffline(ctx)
	}
}

func (a *Agent) StartTransferServer(ctx context.Context) error {
	if a.transfer == nil {
		return nil
	}
	return a.transfer.Start(ctx)
}

func (a *Agent) RetireContainer(ctx context.Context, containerID string) {
	retireCtx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	if err := a.proxy.RemoveContainer(retireCtx, containerID); err != nil {
		a.logger.Warn("retired agent container removal failed", "container", containerID, "error", err)
		return
	}
	a.logger.Info("retired agent container removed", "container", containerID)
}

// keepalivePeriod is how often the agent sends a WebSocket control ping and
// keepaliveTimeout how long it waits for any frame before treating the
// connection as dead and reconnecting. The agent pings on its own schedule
// rather than relying on the server's configurable app-level ping interval,
// so raising that interval can never starve this deadline.
const (
	keepalivePeriod  = 30 * time.Second
	keepaliveTimeout = 90 * time.Second
)

// redactWSURL strips the query string before the connect URL is logged.
//
// The enrolment token travels as `?token=`, so logging the URL whole wrote a
// live credential into the agent's stdout at Info level, on every connect and
// every reconnect. That lands in `docker logs`, in McHarbor's own log viewer,
// and in whatever ships logs off the host — three places a token has no
// business being, for a value that grants an agent its environment.
func redactWSURL(raw string) string {
	if i := strings.IndexByte(raw, '?'); i >= 0 {
		return raw[:i]
	}
	return raw
}

// tlsMode reports how the control channel to the host is secured, so the UI can
// tell an operator that an agent is dialling in over plaintext or with
// certificate verification switched off.
func (a *Agent) tlsMode(wsURL string) string {
	if !strings.HasPrefix(wsURL, "wss://") {
		return "plaintext"
	}
	switch {
	case strings.TrimSpace(a.cfg.Pin) != "":
		return "pinned"
	case a.cfg.Insecure:
		return "insecure"
	default:
		return "verified"
	}
}

// Connect establishes a WebSocket connection and runs the message loop.
// Returns an error when the connection is lost.
func (a *Agent) Connect(ctx context.Context) error {
	a.authenticated.Store(false)

	// Build WebSocket URL
	wsURL, err := a.buildWSURL()
	if err != nil {
		return fmt.Errorf("building WebSocket URL: %w", err)
	}

	a.logger.Info("connecting to server", "url", redactWSURL(wsURL))

	dialer := *websocket.DefaultDialer
	pinned, pinErr := pinnedTLSConfig(a.cfg.Pin)
	if pinErr != nil {
		return fmt.Errorf("invalid MCHARBOR_PIN: %w", pinErr)
	}
	switch {
	case pinned != nil:
		dialer.TLSClientConfig = pinned
		dialer.Proxy = http.ProxyFromEnvironment
	case a.cfg.Insecure:
		a.logger.Warn("TLS verification disabled (insecure mode); set MCHARBOR_PIN to verify the server certificate instead")
		dialer.TLSClientConfig = &tls.Config{InsecureSkipVerify: true}
		dialer.Proxy = http.ProxyFromEnvironment
	}

	conn, resp, err := dialer.DialContext(ctx, wsURL, nil)
	if resp != nil && resp.Body != nil {
		errlog.Log(resp.Body.Close())
	}
	if err != nil {
		return fmt.Errorf("WebSocket dial: %w", err)
	}
	defer errlog.Close(conn)

	// Send auth message
	hostname, err := os.Hostname()
	if err != nil {
		a.logger.Warn("reading hostname failed; authenticating without one", "error", err)
	}
	dockerVersion := a.proxy.DetectDockerVersion(ctx)

	authMsg := WSMessage{
		Type: MsgAuth,
		Auth: &AuthPayload{
			Token:         a.cfg.AgentToken,
			Hostname:      hostname,
			OS:            runtime.GOOS,
			Arch:          runtime.GOARCH,
			AgentVersion:  agentVersion,
			AgentBuild:    agentBuild,
			HostIP:        primaryOutboundIP(ctx),
			DockerVersion: dockerVersion,
			InstallKind:   detectInstallKind(),
			Capabilities:  agentCapabilities(),
			TLSMode:       a.tlsMode(wsURL),
			TransferURL:   a.transferAdvertiseURL(),

			TransferCandidates: a.transferCandidates(),
			TransferPort:       a.transferPort(),
		},
	}
	if err := conn.WriteJSON(authMsg); err != nil {
		return fmt.Errorf("sending auth: %w", err)
	}

	// Wait for auth result
	var result WSMessage
	if err := conn.ReadJSON(&result); err != nil {
		return fmt.Errorf("reading auth result: %w", err)
	}
	if result.Type != MsgAuthResult || result.AuthResult == nil || !result.AuthResult.Success {
		errMsg := "unknown error"
		if result.AuthResult != nil {
			errMsg = result.AuthResult.Error
		}
		return fmt.Errorf("auth rejected: %s", errMsg)
	}

	a.logger.Info("authenticated", "envId", result.AuthResult.EnvID)
	a.authenticated.Store(true)

	// connCtx ends with this connection so the replay goroutine never
	// outlives the socket it writes to.
	connCtx, connCancel := context.WithCancel(ctx)
	defer connCancel()

	// Protocol-level keepalive, independent of the server's app-level ping
	// interval. Without it a socket the network dropped without a FIN keeps
	// this agent parked in ReadJSON — it never notices, never reconnects, and
	// the server holds the pool slot until its own kernel gives up. Every
	// inbound frame (including the pong to our ping) pushes the deadline out.
	if err := conn.SetReadDeadline(time.Now().Add(keepaliveTimeout)); err != nil {
		return fmt.Errorf("setting read deadline: %w", err)
	}
	conn.SetPongHandler(func(string) error {
		return conn.SetReadDeadline(time.Now().Add(keepaliveTimeout))
	})
	conn.SetPingHandler(func(appData string) error {
		if err := conn.SetReadDeadline(time.Now().Add(keepaliveTimeout)); err != nil {
			return err
		}
		// Same contract as every other writer on this socket: one at a time.
		writeMu.Lock()
		defer writeMu.Unlock()
		err := conn.WriteControl(websocket.PongMessage, []byte(appData), time.Now().Add(keepalivePeriod/2))
		if errors.Is(err, websocket.ErrCloseSent) {
			return nil
		}
		return err
	})
	go func() {
		ticker := time.NewTicker(keepalivePeriod)
		defer ticker.Stop()
		for {
			select {
			case <-connCtx.Done():
				return
			case <-ticker.C:
				writeMu.Lock()
				err := conn.WriteControl(websocket.PingMessage, nil, time.Now().Add(keepalivePeriod/2))
				writeMu.Unlock()
				if err != nil {
					a.logger.Debug("keepalive ping failed", "error", err)
					return
				}
			}
		}
	}()
	if a.offline != nil {
		// Stop collecting before replaying so nothing lands behind the flush.
		a.offline.MarkOnline()
		go a.flushOffline(connCtx, conn)
	}

	// Compute writes from several goroutines at once — console pumps, stats
	// samplers, guest events — and the socket permits one writer, so they all
	// go through the same mutex every other producer uses. Cleared on the way
	// out so nothing writes into a connection that has gone.
	setComputeSender(func(msg WSMessage) error {
		writeMu.Lock()
		defer writeMu.Unlock()
		return conn.WriteJSON(msg)
	})
	defer setComputeSender(nil)

	if a.transfer != nil {
		a.transfer.SetReporter(func(msg WSMessage) {
			writeMu.Lock()
			defer writeMu.Unlock()
			if err := conn.WriteJSON(msg); err != nil {
				a.logger.Warn("direct transfer diagnostic report failed", "type", msg.Type, "error", err)
			}
		})
		defer a.transfer.SetReporter(nil)
	}

	// Track in-flight request cancellations
	var cancelMu sync.Mutex
	cancels := make(map[string]context.CancelFunc)
	composeCancels := make(map[string]context.CancelFunc)
	transferCancels := make(map[string]context.CancelFunc)
	uploads := make(map[string]*uploadBuffer)
	spooledUploads := make(map[string]*spooledUpload)
	defer func() {
		cancelMu.Lock()
		defer cancelMu.Unlock()
		for _, cancel := range cancels {
			cancel()
		}
		for _, cancel := range composeCancels {
			cancel()
		}
		for _, cancel := range transferCancels {
			cancel()
		}
		for _, bodyReader := range uploads {
			errlog.Log(bodyReader.CloseWithError(context.Canceled))
		}
		for _, upload := range spooledUploads {
			upload.Cleanup(a.logger)
		}
	}()

	// The stats pusher lives and dies with this connection. It is touched only
	// from this loop, so it needs no lock of its own.
	var statsPush *statsPusher
	var statsPushDone chan struct{}
	sendStats := func(msg WSMessage) error {
		writeMu.Lock()
		defer writeMu.Unlock()
		return conn.WriteJSON(msg)
	}

	// Message loop
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		var msg WSMessage
		if err := conn.ReadJSON(&msg); err != nil {
			return fmt.Errorf("reading message: %w", err)
		}

		switch msg.Type {
		case MsgPing:
			// Every other producer writes under writeMu and the socket
			// permits exactly one writer; this reply is the most frequent
			// write the agent makes, so an unsynchronised one here is the
			// likeliest frame corruption in the whole session.
			writeMu.Lock()
			if err := conn.WriteJSON(WSMessage{Type: MsgPong}); err != nil {
				writeMu.Unlock()
				return fmt.Errorf("writing pong: %w", err)
			}
			writeMu.Unlock()

		case MsgHTTPRequest:
			if msg.HTTPRequest == nil {
				continue
			}
			reqCtx, reqCancel := context.WithCancel(ctx)
			cancelMu.Lock()
			cancels[msg.ID] = reqCancel
			cancelMu.Unlock()

			go func(id string, req *WSHTTPRequest) {
				defer func() {
					cancelMu.Lock()
					delete(cancels, id)
					cancelMu.Unlock()
					reqCancel()
				}()
				a.proxy.HandleRequest(reqCtx, conn, id, req)
			}(msg.ID, msg.HTTPRequest)

		case MsgHTTPRequestStart:
			if msg.HTTPRequest == nil {
				continue
			}
			if strings.HasSuffix(msg.HTTPRequest.Path, "/images/load") {
				upload, err := newSpooledUpload(ctx, msg.HTTPRequest)
				if err != nil {
					a.proxy.sendErrorResponse(conn, msg.ID, http.StatusBadGateway, err)
					continue
				}

				cancelMu.Lock()
				cancels[msg.ID] = upload.cancel
				spooledUploads[msg.ID] = upload
				cancelMu.Unlock()
				continue
			}

			reqCtx, reqCancel := context.WithCancel(ctx)
			bodyReader := newUploadBuffer()

			cancelMu.Lock()
			cancels[msg.ID] = reqCancel
			uploads[msg.ID] = bodyReader
			cancelMu.Unlock()

			go func(id string, req *WSHTTPRequest) {
				defer func() {
					cancelMu.Lock()
					delete(cancels, id)
					delete(uploads, id)
					cancelMu.Unlock()
					reqCancel()
					errlog.Close(bodyReader)
				}()
				a.proxy.HandleRequestStream(reqCtx, conn, id, req, bodyReader)
			}(msg.ID, msg.HTTPRequest)

		case MsgHTTPRequestChunk:
			if msg.StreamChunk == nil {
				continue
			}
			cancelMu.Lock()
			upload := spooledUploads[msg.ID]
			bodyReader := uploads[msg.ID]
			cancelMu.Unlock()
			if upload != nil {
				if err := upload.Write(msg.StreamChunk.Data); err != nil {
					a.logger.Warn("spooled request body write failed", "id", msg.ID, "path", upload.req.Path, "error", err)
					cancelMu.Lock()
					delete(cancels, msg.ID)
					delete(spooledUploads, msg.ID)
					cancelMu.Unlock()
					upload.Cleanup(a.logger)
					a.proxy.sendErrorResponse(conn, msg.ID, http.StatusBadGateway, err)
				}
			} else if bodyReader != nil {
				if err := bodyReader.Push(msg.StreamChunk.Data); err != nil {
					a.logger.Warn("request body stream write failed", "id", msg.ID, "error", err)
				}
			}

		case MsgHTTPRequestEnd:
			cancelMu.Lock()
			upload := spooledUploads[msg.ID]
			delete(spooledUploads, msg.ID)
			bodyReader := uploads[msg.ID]
			delete(uploads, msg.ID)
			cancelMu.Unlock()
			if upload != nil {
				go func(id string, upload *spooledUpload) {
					defer func() {
						cancelMu.Lock()
						delete(cancels, id)
						cancelMu.Unlock()
						upload.Cleanup(a.logger)
					}()
					if err := upload.Rewind(); err != nil {
						a.proxy.sendErrorResponse(conn, id, http.StatusBadGateway, err)
						return
					}
					a.logger.Info("received staged Docker image upload", "id", id, "path", upload.req.Path, "bytes", upload.bytes)
					a.proxy.HandleRequestStream(upload.ctx, conn, id, upload.req, upload.file)
				}(msg.ID, upload)
			} else if bodyReader != nil {
				errlog.Close(bodyReader)
			}

		case MsgHTTPCancel:
			cancelMu.Lock()
			if cancel, ok := cancels[msg.ID]; ok {
				cancel()
				delete(cancels, msg.ID)
			}
			if bodyReader := uploads[msg.ID]; bodyReader != nil {
				errlog.Log(bodyReader.CloseWithError(context.Canceled))
				delete(uploads, msg.ID)
			}
			if spooledUpload := spooledUploads[msg.ID]; spooledUpload != nil {
				spooledUpload.Cleanup(a.logger)
				delete(spooledUploads, msg.ID)
			}
			cancelMu.Unlock()

		case MsgExecStart:
			if msg.ExecStart == nil {
				continue
			}
			go a.proxy.HandleExec(ctx, conn, msg.ID, msg.ExecStart.ExecID)

		case MsgExecInput:
			if msg.StreamChunk != nil {
				a.proxy.WriteExecInput(msg.ID, msg.StreamChunk.Data)
			}

		case MsgExecResize:
			if msg.ExecResize != nil {
				a.proxy.ResizeExec(ctx, msg.ExecResize.ExecID, msg.ExecResize.Cols, msg.ExecResize.Rows)
			}

		case MsgExecEnd:
			a.proxy.CloseExec(msg.ID)

		case MsgComposeRun:
			if msg.Compose == nil {
				continue
			}
			composeCtx, composeCancel := context.WithCancel(ctx)
			cancelMu.Lock()
			composeCancels[msg.ID] = composeCancel
			cancelMu.Unlock()
			go func(id string, payload ComposePayload) {
				defer func() {
					cancelMu.Lock()
					delete(composeCancels, id)
					cancelMu.Unlock()
					composeCancel()
				}()
				a.runCompose(composeCtx, conn, id, payload)
			}(msg.ID, *msg.Compose)

		case MsgComposeCancel:
			cancelMu.Lock()
			if cancel, ok := composeCancels[msg.ID]; ok {
				cancel()
				delete(composeCancels, msg.ID)
			}
			cancelMu.Unlock()

		case MsgTransferPrepare:
			if msg.Transfer == nil {
				continue
			}
			a.handleTransferPrepare(conn, msg.Transfer)

		case MsgTransferImage:
			if msg.Transfer == nil {
				continue
			}
			transferCtx, transferCancel := context.WithCancel(ctx)
			cancelMu.Lock()
			transferCancels[msg.Transfer.TransferID] = transferCancel
			cancelMu.Unlock()
			go func(payload TransferPayload) {
				defer func() {
					cancelMu.Lock()
					delete(transferCancels, payload.TransferID)
					cancelMu.Unlock()
					transferCancel()
				}()
				a.runImageTransfer(transferCtx, conn, payload)
			}(*msg.Transfer)

		case MsgTransferArchive:
			if msg.Transfer == nil {
				continue
			}
			transferCtx, transferCancel := context.WithCancel(ctx)
			cancelMu.Lock()
			transferCancels[msg.Transfer.TransferID] = transferCancel
			cancelMu.Unlock()
			go func(payload TransferPayload) {
				defer func() {
					cancelMu.Lock()
					delete(transferCancels, payload.TransferID)
					cancelMu.Unlock()
					transferCancel()
				}()
				a.runArchiveTransfer(transferCtx, conn, payload)
			}(*msg.Transfer)

		case MsgTransferProbe:
			if msg.Transfer == nil {
				continue
			}
			transferCtx, transferCancel := context.WithCancel(ctx)
			cancelMu.Lock()
			transferCancels[msg.Transfer.TransferID] = transferCancel
			cancelMu.Unlock()
			go func(payload TransferPayload) {
				defer func() {
					cancelMu.Lock()
					delete(transferCancels, payload.TransferID)
					cancelMu.Unlock()
					transferCancel()
				}()
				a.runTransferProbe(transferCtx, conn, payload)
			}(*msg.Transfer)

		case MsgTransferRestore:
			if msg.Transfer == nil {
				continue
			}
			transferCtx, transferCancel := context.WithCancel(ctx)
			cancelMu.Lock()
			transferCancels[msg.Transfer.TransferID] = transferCancel
			cancelMu.Unlock()
			go func(payload TransferPayload) {
				defer func() {
					cancelMu.Lock()
					delete(transferCancels, payload.TransferID)
					cancelMu.Unlock()
					transferCancel()
				}()
				a.runRestoreTransfer(transferCtx, conn, payload)
			}(*msg.Transfer)

		case MsgBackupRun:
			if msg.Backup == nil {
				continue
			}
			transferCtx, transferCancel := context.WithCancel(ctx)
			cancelMu.Lock()
			transferCancels[msg.Backup.TransferID] = transferCancel
			cancelMu.Unlock()
			go func(payload BackupPayload) {
				defer func() {
					cancelMu.Lock()
					delete(transferCancels, payload.TransferID)
					cancelMu.Unlock()
					transferCancel()
				}()
				a.runBackup(transferCtx, conn, payload)
			}(*msg.Backup)

		case MsgBackupRestore:
			if msg.Backup == nil {
				continue
			}
			transferCtx, transferCancel := context.WithCancel(ctx)
			cancelMu.Lock()
			transferCancels[msg.Backup.TransferID] = transferCancel
			cancelMu.Unlock()
			go func(payload BackupPayload) {
				defer func() {
					cancelMu.Lock()
					delete(transferCancels, payload.TransferID)
					cancelMu.Unlock()
					transferCancel()
				}()
				a.runBackupRestore(transferCtx, conn, payload)
			}(*msg.Backup)

		case MsgHostStatsRequest:
			a.handleHostStatsRequest(ctx, conn, msg.ID)

		// Host OS inventory and package updates. Each runs on its own goroutine:
		// an apt-get update against a slow mirror blocks for minutes and the read
		// loop still has every other guest's console to serve.
		case MsgHostOSInfoRequest:
			go a.handleHostOSInfoRequest(ctx, conn, msg.ID)

		case MsgHostUpdateCheck:
			go a.handleHostUpdateCheck(ctx, conn, msg.ID)

		case MsgHostUpdateApply:
			go a.handleHostUpdateApply(ctx, conn, msg.ID, msg.HostUpdate)

		// Host systemd services, native Linux agents only. Its own goroutine,
		// because a stop blocks for the unit's whole stop timeout.
		case MsgHostServiceRequest:
			if msg.HostService == nil {
				continue
			}
			go a.handleHostServiceRequest(ctx, conn, msg.ID, *msg.HostService)

		// Host links: a named macvlan child on the host itself, for Docker and
		// compute networks alike. Its own goroutine, like the services above.
		case MsgHostLinkRequest:
			if msg.HostLink == nil {
				continue
			}
			go a.handleHostLinkRequest(ctx, conn, msg.ID, *msg.HostLink)

		// Compute: KVM virtual machines and LXC system containers.
		//
		// Every one of these is handled on its own goroutine with an explicit
		// context, because a guest operation can take minutes and the read loop
		// must keep serving console traffic for every other guest while it runs.
		// handleComputeMessage owns the per-family timeout: a capability probe
		// gets seconds, a disk allocation gets hours.
		case MsgComputeProbeRequest, MsgComputeInstallRequest,
			// compute_config_request was absent from this list, so every
			// location McHarbor pushed was dropped by the default branch and
			// the operator's choice silently never took effect.
			MsgComputeConfigRequest, MsgComputeMigrateRequest,
			MsgPoolDetectRequest, MsgPoolCreateRequest, MsgPoolStatsRequest,
			MsgPoolRemoveRequest, MsgPoolBrowseRequest, MsgPoolFileRequest,
			MsgNetDetectRequest, MsgNetEnsureRequest, MsgNetRemoveRequest,
			MsgImageFetchRequest, MsgImageListRequest, MsgImageRemoveRequest,
			MsgImageCancelRequest,
			MsgVMDefine, MsgVMStart, MsgVMShutdown, MsgVMStop, MsgVMReset,
			MsgVMPause, MsgVMResume, MsgVMDelete, MsgVMQuery, MsgVMStats,
			MsgHostDeviceList,
			MsgVMLog, MsgVMDiskList, MsgVMDiskCreate, MsgVMDiskResize, MsgVMDiskDelete,
			MsgVMSnapshotCreate, MsgVMSnapshotRevert, MsgVMSnapshotDelete,
			MsgVMImportInspect, MsgVMImportConvert, MsgVMImportStage, MsgVMImportDiscard,
			MsgVMCloneRequest, MsgVMGuestQuery, MsgVMGuestAgentDisc, MsgVMMigrate, MsgVMCancel,
			MsgVMQEMUCapability, MsgVMMemorySet, MsgVMCPUSet, MsgVMThrottleSet,
			MsgVMImageInspect, MsgVMBackupRun, MsgVMBackupRestore,
			MsgVMSpiceTicket, MsgVMDiskHotplug, MsgVMMediaChange, MsgVMNICHotplug,
			MsgNetAdoptConfirm,
			MsgCTCreate, MsgCTStart, MsgCTShutdown, MsgCTStop, MsgCTRestart,
			MsgCTFreeze, MsgCTUnfreeze, MsgCTDelete, MsgCTQuery, MsgCTProcesses,
			MsgCTUpdateLimits, MsgCTSnapshotCreate, MsgCTSnapshotRevert,
			MsgCTSnapshotDelete, MsgCTCloneRequest, MsgCTCancel,
			MsgCTBackupRun, MsgCTBackupRestore:
			go a.handleComputeMessage(ctx, msg)

		// Console byte streams. These are NOT dispatched on a goroutine per
		// frame: the session's own pump owns the stream, and spawning a
		// goroutine per input frame would reorder keystrokes.
		case MsgConsoleStart:
			if msg.Console == nil {
				continue
			}
			go a.handleConsoleStart(ctx, msg.ID, *msg.Console)

		case MsgConsoleInput, MsgConsoleResize, MsgConsoleEnd:
			if msg.Console == nil {
				continue
			}
			a.handleConsoleFrame(ctx, msg.Type, msg.ID, *msg.Console)

		case MsgSnapshotDetectRequest, MsgSnapshotCapabilityRequest,
			MsgSnapshotCreateRequest, MsgSnapshotRemoveRequest:
			if msg.Snapshot == nil {
				continue
			}
			// Five minutes is the outer bound for any snapshot call.
			// A create on a large LVM volume is the slow case; detect
			// and remove finish in well under a second.
			snapshotCtx, snapshotCancel := context.WithTimeout(ctx, 5*time.Minute)
			go func(id string, payload SnapshotPayload) {
				defer snapshotCancel()
				a.handleSnapshotRequest(snapshotCtx, conn, id, payload)
			}(msg.ID, *msg.Snapshot)

		case MsgOfflineBatchAck:
			a.offlineAcks.deliver(msg.ID, msg.Offline)

		case MsgStatsSubscribe:
			if msg.StatsPush == nil {
				continue
			}
			interval := time.Duration(msg.StatsPush.IntervalSeconds) * time.Second
			if statsPush != nil && !channelClosed(statsPushDone) {
				statsPush.Retune(interval)
				continue
			}
			statsPush = newStatsPusher(a.proxy, a.offline, sendStats, interval, a.logger)
			statsPushDone = make(chan struct{})
			a.logger.Info("stats push started", "interval", clampStatsInterval(interval))
			go func(p *statsPusher, done chan struct{}) {
				defer close(done)
				p.Run(connCtx)
			}(statsPush, statsPushDone)

		case MsgStatsPushAck:
			if statsPush != nil {
				statsPush.DeliverAck(msg.StatsPush)
			}

		case MsgAgentUpdateRequest:
			if msg.AgentUpdate == nil {
				continue
			}
			go a.handleAgentUpdateRequest(ctx, conn, msg.ID, *msg.AgentUpdate)

		case MsgPruneRequest:
			if msg.Prune == nil {
				continue
			}
			pruneCtx, pruneCancel := context.WithTimeout(ctx, 5*time.Minute)
			go func(id string, payload PrunePayload) {
				defer pruneCancel()
				a.handlePruneRequest(pruneCtx, conn, id, payload)
			}(msg.ID, *msg.Prune)

		case MsgHostTerminalStart:
			if msg.HostTerminal == nil {
				msg.HostTerminal = &HostTerminalStartPayload{}
			}
			a.handleHostTerminalStart(ctx, conn, msg.ID, *msg.HostTerminal)

		case MsgHostTerminalInput:
			if msg.StreamChunk == nil {
				continue
			}
			execID := parseExecIDFromHeader(msg.StreamChunk.Data)
			if execID == "" {
				continue
			}
			headerLen := len(execIDHeaderPrefix) + len(execID) + 1
			if len(msg.StreamChunk.Data) < headerLen {
				continue
			}
			if err := writeHostTerminalInput(execID, msg.StreamChunk.Data[headerLen:]); err != nil {
				a.logger.Debug("host terminal input write failed", "id", execID, "error", err)
			}

		case MsgHostTerminalResize:
			if msg.HostTerminalRes == nil {
				continue
			}
			if err := resizeHostTerminal(msg.HostTerminalRes.ExecID, msg.HostTerminalRes.Cols, msg.HostTerminalRes.Rows); err != nil {
				a.logger.Debug("host terminal resize failed", "id", msg.HostTerminalRes.ExecID, "error", err)
			}

		case MsgHostTerminalEnd:
			if msg.HostTerminalEnd == nil {
				continue
			}
			closeHostTerminal(msg.HostTerminalEnd.ExecID)

		case MsgHostLogRequest:
			if msg.HostLogRequest == nil {
				msg.HostLogRequest = &HostLogRequestPayload{Source: "system", Tail: 200}
			}
			go func(reqID string, payload HostLogRequestPayload) {
				a.handleHostLogRequest(ctx, conn, reqID, payload)
			}(msg.ID, *msg.HostLogRequest)

		case MsgTransferCancel:
			if msg.Transfer == nil {
				continue
			}
			cancelMu.Lock()
			if cancel, ok := transferCancels[msg.Transfer.TransferID]; ok {
				cancel()
				delete(transferCancels, msg.Transfer.TransferID)
			}
			cancelMu.Unlock()
			if a.transfer != nil {
				a.transfer.Cancel(msg.Transfer.TransferID)
			}

		default:
			// Never silent, and never unanswered. A message type missing from
			// the cases above used to be logged and dropped, which the server
			// cannot tell from an operation that is simply slow — it waits out
			// its whole timeout for a reply that will never come. Seven compute
			// types shipped unrouted that way, and every feature behind them
			// looked like a hang instead of a missing handler.
			a.logger.Warn("agent: no handler for this message type; refusing it",
				"type", msg.Type, "id", msg.ID)
			if msg.ID != "" {
				a.replyComputeError(msg, fmt.Errorf(
					"this agent has no handler for %s; it is too old for this "+
						"operation, or the operation was never wired", msg.Type))
			}
		}
	}
}

func (a *Agent) transferAdvertiseURL() string {
	if a.transfer == nil {
		return ""
	}
	return a.transfer.AdvertiseURL()
}

func (a *Agent) transferCandidates() []string {
	if a.transfer == nil {
		return nil
	}
	return a.transfer.Candidates()
}

func (a *Agent) transferPort() int {
	if a.transfer == nil {
		return 0
	}
	return a.transfer.Port()
}

func (a *Agent) handleTransferPrepare(conn *websocket.Conn, payload *TransferPayload) {
	var uploadURL string
	var receiver *TransferReceiverMarker
	var err error
	switch payload.Kind {
	case transferKindProbe:
		uploadURL, receiver, err = a.transfer.PrepareProbe(payload.TransferID, payload.Token)
	case transferKindArchive:
		uploadURL, receiver, err = a.transfer.PrepareArchive(payload.TransferID, payload.Token, payload.ContainerID, payload.TargetPath)
	default:
		uploadURL, receiver, err = a.transfer.Prepare(payload.TransferID, payload.Token)
	}
	if err != nil {
		a.sendTransferResult(conn, MsgTransferReady, payload.TransferID, false, 0, "", 0, err)
		return
	}
	response := &TransferPayload{
		TransferID: payload.TransferID,
		Success:    true,
		URL:        uploadURL,
		Receiver:   receiver,
	}
	writeMu.Lock()
	if writeErr := conn.WriteJSON(WSMessage{Type: MsgTransferReady, Transfer: response}); writeErr != nil {
		a.logger.Warn("direct transfer ready write failed", "transferId", payload.TransferID, "error", writeErr)
	}
	writeMu.Unlock()
}

func (a *Agent) runImageTransfer(ctx context.Context, conn *websocket.Conn, payload TransferPayload) {
	if strings.TrimSpace(payload.TransferID) == "" || strings.TrimSpace(payload.ImageRef) == "" || strings.TrimSpace(payload.URL) == "" || strings.TrimSpace(payload.Token) == "" {
		a.sendTransferResult(conn, MsgTransferResult, payload.TransferID, false, 0, "", 0, fmt.Errorf("invalid direct image transfer request"))
		return
	}

	reader, err := a.proxy.SaveImage(ctx, payload.ImageRef)
	if err != nil {
		a.sendTransferResult(conn, MsgTransferResult, payload.TransferID, false, 0, "", 0, err)
		return
	}
	defer errlog.Close(reader)

	progressReader := &transferProgressReader{
		reader:     reader,
		conn:       conn,
		transferID: payload.TransferID,
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, payload.URL, progressReader)
	if err != nil {
		a.sendTransferResult(conn, MsgTransferResult, payload.TransferID, false, 0, "", 0, fmt.Errorf("building direct transfer upload request: %w", err))
		return
	}
	req.Header.Set("Authorization", "Bearer "+payload.Token)
	req.Header.Set("Content-Type", "application/x-tar")

	resp, err := a.httpClient.Do(req)
	if err != nil {
		a.sendTransferResult(conn, MsgTransferResult, payload.TransferID, false, progressReader.bytes.Load(), "", 0, fmt.Errorf("uploading direct image transfer: %w", err))
		return
	}
	defer func() { errlog.Log(resp.Body.Close()) }()
	if resp.StatusCode >= 300 {
		body := errlog.ReadAll(io.LimitReader(resp.Body, 4096))
		a.sendTransferResult(conn, MsgTransferResult, payload.TransferID, false, progressReader.bytes.Load(), "", resp.StatusCode, fmt.Errorf("target direct upload returned status %d: %s", resp.StatusCode, strings.TrimSpace(string(body))))
		return
	}
	if _, err := io.Copy(io.Discard, resp.Body); err != nil {
		a.sendTransferResult(conn, MsgTransferResult, payload.TransferID, false, progressReader.bytes.Load(), "", resp.StatusCode, fmt.Errorf("reading direct upload response: %w", err))
		return
	}

	a.sendTransferResult(conn, MsgTransferResult, payload.TransferID, true, progressReader.bytes.Load(), "", resp.StatusCode, nil)
}

func (a *Agent) runArchiveTransfer(ctx context.Context, conn *websocket.Conn, payload TransferPayload) {
	if strings.TrimSpace(payload.TransferID) == "" || strings.TrimSpace(payload.ContainerID) == "" || strings.TrimSpace(payload.SourcePath) == "" || strings.TrimSpace(payload.URL) == "" || strings.TrimSpace(payload.Token) == "" {
		a.sendTransferResult(conn, MsgTransferResult, payload.TransferID, false, 0, "", 0, fmt.Errorf("invalid direct archive transfer request"))
		return
	}

	reader, _, err := a.proxy.CopyArchiveFromContainer(ctx, payload.ContainerID, payload.SourcePath)
	if err != nil {
		a.sendTransferResult(conn, MsgTransferResult, payload.TransferID, false, 0, "", 0, err)
		return
	}
	defer errlog.Close(reader)

	progressReader := &transferProgressReader{
		reader:     reader,
		conn:       conn,
		transferID: payload.TransferID,
		stage:      "archive",
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, payload.URL, progressReader)
	if err != nil {
		a.sendTransferResult(conn, MsgTransferResult, payload.TransferID, false, 0, "", 0, fmt.Errorf("building direct archive transfer request: %w", err))
		return
	}
	req.Header.Set("Authorization", "Bearer "+payload.Token)
	req.Header.Set("Content-Type", "application/x-tar")

	resp, err := a.httpClient.Do(req)
	if err != nil {
		a.sendTransferResult(conn, MsgTransferResult, payload.TransferID, false, progressReader.bytes.Load(), "", 0, fmt.Errorf("uploading direct archive transfer: %w", err))
		return
	}
	defer func() { errlog.Log(resp.Body.Close()) }()
	if resp.StatusCode >= 300 {
		body := errlog.ReadAll(io.LimitReader(resp.Body, 4096))
		a.sendTransferResult(conn, MsgTransferResult, payload.TransferID, false, progressReader.bytes.Load(), "", resp.StatusCode, fmt.Errorf("target direct archive upload returned status %d: %s", resp.StatusCode, strings.TrimSpace(string(body))))
		return
	}
	if _, err := io.Copy(io.Discard, resp.Body); err != nil {
		a.sendTransferResult(conn, MsgTransferResult, payload.TransferID, false, progressReader.bytes.Load(), "", resp.StatusCode, fmt.Errorf("reading direct archive upload response: %w", err))
		return
	}

	a.sendTransferResult(conn, MsgTransferResult, payload.TransferID, true, progressReader.bytes.Load(), "", resp.StatusCode, nil)
}

func (a *Agent) runTransferProbe(ctx context.Context, conn *websocket.Conn, payload TransferPayload) {
	if strings.TrimSpace(payload.TransferID) == "" || strings.TrimSpace(payload.URL) == "" || strings.TrimSpace(payload.Token) == "" {
		a.sendTransferResult(conn, MsgTransferResult, payload.TransferID, false, 0, "", 0, fmt.Errorf("invalid direct transfer probe request"))
		return
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, payload.URL, strings.NewReader("mcharbor-agent-direct-transfer-probe"))
	if err != nil {
		a.sendTransferResult(conn, MsgTransferResult, payload.TransferID, false, 0, "", 0, fmt.Errorf("building direct transfer probe request: %w", err))
		return
	}
	req.Header.Set("Authorization", "Bearer "+payload.Token)
	req.Header.Set("Content-Type", "text/plain; charset=utf-8")

	resp, err := a.probeClient.Do(req)
	if err != nil {
		a.sendTransferResult(conn, MsgTransferResult, payload.TransferID, false, 0, "", 0, fmt.Errorf("running direct transfer probe: %w", err))
		return
	}
	defer func() { errlog.Log(resp.Body.Close()) }()
	responderAgentMarker := strings.TrimSpace(resp.Header.Get("X-McHarbor-Agent-Marker"))
	if resp.StatusCode >= 300 {
		body := errlog.ReadAll(io.LimitReader(resp.Body, 4096))
		a.sendTransferResultWithResponder(conn, MsgTransferResult, payload.TransferID, false, 0, "", resp.StatusCode, responderAgentMarker, fmt.Errorf("target direct probe returned status %d: %s", resp.StatusCode, strings.TrimSpace(string(body))))
		return
	}
	if _, err := io.Copy(io.Discard, resp.Body); err != nil {
		a.sendTransferResultWithResponder(conn, MsgTransferResult, payload.TransferID, false, 0, "", resp.StatusCode, responderAgentMarker, fmt.Errorf("reading direct probe response: %w", err))
		return
	}

	a.sendTransferResultWithResponder(conn, MsgTransferResult, payload.TransferID, true, 0, "", resp.StatusCode, responderAgentMarker, nil)
}

func (a *Agent) runRestoreTransfer(ctx context.Context, conn *websocket.Conn, payload TransferPayload) {
	if strings.TrimSpace(payload.TransferID) == "" || strings.TrimSpace(payload.URL) == "" || strings.TrimSpace(payload.Token) == "" {
		a.sendTransferResult(conn, MsgTransferResult, payload.TransferID, false, 0, "", 0, fmt.Errorf("invalid restore transfer request"))
		return
	}

	archiveURL, err := a.resolveServerURL(payload.URL)
	if err != nil {
		a.sendTransferResult(conn, MsgTransferResult, payload.TransferID, false, 0, "", 0, err)
		return
	}
	tmp, err := os.CreateTemp("", "mcharbor-agent-restore-*.tar")
	if err != nil {
		a.sendTransferResult(conn, MsgTransferResult, payload.TransferID, false, 0, "", 0, fmt.Errorf("creating temporary restore archive: %w", err))
		return
	}
	tmpPath := tmp.Name()
	defer func() {
		if closeErr := tmp.Close(); closeErr != nil {
			a.logger.Warn("close temporary restore archive failed", "path", tmpPath, "error", closeErr)
		}
		if removeErr := os.Remove(tmpPath); removeErr != nil && !os.IsNotExist(removeErr) {
			a.logger.Warn("remove temporary restore archive failed", "path", tmpPath, "error", removeErr)
		}
	}()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, archiveURL, nil)
	if err != nil {
		a.sendTransferResult(conn, MsgTransferResult, payload.TransferID, false, 0, "", 0, fmt.Errorf("building restore transfer request: %w", err))
		return
	}
	req.Header.Set("Authorization", "Bearer "+payload.Token)
	resp, err := a.httpClient.Do(req)
	if err != nil {
		a.sendTransferResult(conn, MsgTransferResult, payload.TransferID, false, 0, "", 0, fmt.Errorf("downloading restore archive: %w", err))
		return
	}
	defer func() { errlog.Log(resp.Body.Close()) }()
	if resp.StatusCode >= 300 {
		body := errlog.ReadAll(io.LimitReader(resp.Body, 4096))
		a.sendTransferResult(conn, MsgTransferResult, payload.TransferID, false, 0, "", resp.StatusCode, fmt.Errorf("restore archive download returned status %d: %s", resp.StatusCode, strings.TrimSpace(string(body))))
		return
	}

	progressReader := &transferProgressReader{
		reader:     resp.Body,
		conn:       conn,
		transferID: payload.TransferID,
		stage:      "download",
	}
	written, err := io.Copy(tmp, progressReader)
	if err != nil {
		a.sendTransferResult(conn, MsgTransferResult, payload.TransferID, false, progressReader.bytes.Load(), "", resp.StatusCode, fmt.Errorf("writing temporary restore archive: %w", err))
		return
	}
	if payload.Size > 0 && written != payload.Size {
		a.sendTransferResult(conn, MsgTransferResult, payload.TransferID, false, written, "", resp.StatusCode, fmt.Errorf("restore archive size mismatch: expected %d bytes, got %d", payload.Size, written))
		return
	}
	a.sendTransferProgress(conn, payload.TransferID, written, "apply")
	if _, err := tmp.Seek(0, io.SeekStart); err != nil {
		a.sendTransferResult(conn, MsgTransferResult, payload.TransferID, false, written, "", resp.StatusCode, fmt.Errorf("rewinding temporary restore archive: %w", err))
		return
	}
	if payload.Kind == transferKindImage {
		if err := a.proxy.LoadImage(ctx, tmp); err != nil {
			a.sendTransferResult(conn, MsgTransferResult, payload.TransferID, false, written, "", resp.StatusCode, err)
			return
		}
	} else {
		if strings.TrimSpace(payload.ContainerID) == "" {
			a.sendTransferResult(conn, MsgTransferResult, payload.TransferID, false, written, "", resp.StatusCode, fmt.Errorf("container id is required"))
			return
		}
		if err := a.proxy.CopyArchiveToContainer(ctx, payload.ContainerID, payload.TargetPath, tmp, written); err != nil {
			a.sendTransferResult(conn, MsgTransferResult, payload.TransferID, false, written, "", resp.StatusCode, err)
			return
		}
	}

	a.sendTransferResult(conn, MsgTransferResult, payload.TransferID, true, written, "", resp.StatusCode, nil)
}

func (r *transferProgressReader) Read(p []byte) (int, error) {
	n, err := r.reader.Read(p)
	if n > 0 {
		total := r.bytes.Add(int64(n))
		if r.lastEmit.IsZero() || time.Since(r.lastEmit) >= 3*time.Second {
			r.lastEmit = time.Now()
			sendTransferProgress(r.conn, r.transferID, total, r.stage)
		}
	}
	return n, err
}

func (a *Agent) sendTransferProgress(conn *websocket.Conn, transferID string, bytes int64, stage string) {
	sendTransferProgress(conn, transferID, bytes, stage)
}

func sendTransferProgress(conn *websocket.Conn, transferID string, bytes int64, stage string) {
	writeMu.Lock()
	errlog.Log(conn.WriteJSON(WSMessage{
		Type: MsgTransferProgress,
		Transfer: &TransferPayload{
			TransferID: transferID,
			Bytes:      bytes,
			Stage:      strings.TrimSpace(stage),
		},
	}))
	writeMu.Unlock()
}

func (a *Agent) sendTransferResult(conn *websocket.Conn, msgType, transferID string, success bool, bytes int64, uploadURL string, statusCode int, err error) {
	a.sendTransferResultWithResponder(conn, msgType, transferID, success, bytes, uploadURL, statusCode, "", err)
}

func (a *Agent) sendTransferResultWithResponder(conn *websocket.Conn, msgType, transferID string, success bool, bytes int64, uploadURL string, statusCode int, responderAgentMarker string, err error) {
	payload := &TransferPayload{
		TransferID:           transferID,
		Success:              success,
		Bytes:                bytes,
		URL:                  uploadURL,
		StatusCode:           statusCode,
		ResponderAgentMarker: strings.TrimSpace(responderAgentMarker),
	}
	if err != nil {
		payload.Error = err.Error()
	}
	writeMu.Lock()
	if writeErr := conn.WriteJSON(WSMessage{Type: msgType, Transfer: payload}); writeErr != nil {
		a.logger.Warn("direct transfer result write failed", "transferId", transferID, "type", msgType, "error", writeErr)
	}
	writeMu.Unlock()
}

// buildWSURL constructs the WebSocket URL from the server URL.
func (a *Agent) buildWSURL() (string, error) {
	u, err := url.Parse(a.cfg.McHarborURL)
	if err != nil {
		return "", err
	}

	// Convert http(s) to ws(s)
	switch u.Scheme {
	case "https":
		u.Scheme = "wss"
	case "http":
		u.Scheme = "ws"
	case "ws", "wss":
		// Already correct
	default:
		return "", fmt.Errorf("unsupported scheme: %s", u.Scheme)
	}

	// MCHARBOR_FORCE_TLS promotes ws → wss and http → https when
	// the host is HTTPS-only. Only promote up — never downgrade.
	if a.cfg.ForceTLS {
		switch u.Scheme {
		case "ws":
			u.Scheme = "wss"
		case "http":
			u.Scheme = "https"
		}
	}

	u.Path = "/api/agent/ws"
	q := u.Query()
	q.Set("token", a.cfg.AgentToken)
	u.RawQuery = q.Encode()

	return u.String(), nil
}

func (a *Agent) resolveServerURL(value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", fmt.Errorf("server URL is required")
	}
	parsed, err := url.Parse(value)
	if err != nil {
		return "", fmt.Errorf("parsing server URL: %w", err)
	}
	if parsed.IsAbs() {
		return parsed.String(), nil
	}

	base, err := url.Parse(a.cfg.McHarborURL)
	if err != nil {
		return "", fmt.Errorf("parsing McHarbor URL: %w", err)
	}
	switch base.Scheme {
	case "ws":
		base.Scheme = "http"
	case "wss":
		base.Scheme = "https"
	case "http", "https":
	default:
		return "", fmt.Errorf("unsupported McHarbor URL scheme: %s", base.Scheme)
	}
	// MCHARBOR_FORCE_TLS promotes ws → wss and http → https when
	// the host is HTTPS-only. The promotion happens against the
	// base URL before resolving any relative reference.
	if a.cfg.ForceTLS {
		switch base.Scheme {
		case "ws":
			base.Scheme = "wss"
		case "http":
			base.Scheme = "https"
		}
	}
	base.Path = "/"
	base.RawQuery = ""
	base.Fragment = ""
	return base.ResolveReference(parsed).String(), nil
}

// handleHostStatsRequest reads /proc and returns a HostStatsPayload over the
// WebSocket. The server uses this to populate the Host tab on environments
// connected through the agent.
func (a *Agent) handleHostStatsRequest(ctx context.Context, conn *websocket.Conn, id string) {
	stats, ok := readHostStats()
	if !ok {
		errlog.Log(writeHostStatsResponse(ctx, conn, id, HostStatsPayload{
			Supported: false,
			Error:     "host stats are not supported on this platform",
		}))
		return
	}
	errlog.Log(writeHostStatsResponse(ctx, conn, id, stats))
}

// handlePruneRequest runs a docker prune on the local socket and returns
// the result over the WebSocket. Mirrors the server-side handler so the
// server can run prune operations against agent environments.
func (a *Agent) handlePruneRequest(ctx context.Context, conn *websocket.Conn, id string, payload PrunePayload) {
	result, err := a.proxy.Prune(ctx, payload)
	if err != nil {
		a.logger.Warn("agent prune failed", "type", payload.Type, "volumes", payload.Volumes, "error", err)
		errlog.Log(writePruneResponse(ctx, conn, id, PrunePayload{
			Type:    payload.Type,
			Volumes: payload.Volumes,
			Success: false,
			Error:   err.Error(),
		}))
		return
	}
	errlog.Log(writePruneResponse(ctx, conn, id, result))
}

func writeHostStatsResponse(ctx context.Context, conn *websocket.Conn, id string, payload HostStatsPayload) error {
	writeMu.Lock()
	defer writeMu.Unlock()
	return conn.WriteJSON(WSMessage{
		Type:      MsgHostStatsResponse,
		ID:        id,
		HostStats: &payload,
	})
}

func writePruneResponse(ctx context.Context, conn *websocket.Conn, id string, payload PrunePayload) error {
	writeMu.Lock()
	defer writeMu.Unlock()
	return conn.WriteJSON(WSMessage{
		Type:  MsgPruneResponse,
		ID:    id,
		Prune: &payload,
	})
}
