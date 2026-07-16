// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

import (
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gorilla/websocket"
)

const agentVersion = "2.0.0"

// Agent handles the WebSocket connection to the McHarbor server.
type Agent struct {
	cfg      Config
	logger   *slog.Logger
	proxy    *Proxy
	transfer *TransferServer
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
	ctx    context.Context
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
		cfg:      cfg,
		logger:   logger,
		proxy:    proxy,
		transfer: NewTransferServer(cfg.TransferListen, cfg.TransferAdvertiseURL, cfg.AgentToken, proxy, logger),
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

// Connect establishes a WebSocket connection and runs the message loop.
// Returns an error when the connection is lost.
func (a *Agent) Connect(ctx context.Context) error {
	// Build WebSocket URL
	wsURL, err := a.buildWSURL()
	if err != nil {
		return fmt.Errorf("building WebSocket URL: %w", err)
	}

	a.logger.Info("connecting to server", "url", wsURL)

	dialer := *websocket.DefaultDialer
	if a.cfg.Insecure {
		a.logger.Warn("TLS verification disabled (insecure mode)")
		dialer.TLSClientConfig = &tls.Config{InsecureSkipVerify: true}
		dialer.Proxy = http.ProxyFromEnvironment
	}

	conn, _, err := dialer.DialContext(ctx, wsURL, nil)
	if err != nil {
		return fmt.Errorf("WebSocket dial: %w", err)
	}
	defer conn.Close()

	// Send auth message
	hostname, _ := os.Hostname()
	dockerVersion := a.proxy.DetectDockerVersion()

	authMsg := WSMessage{
		Type: MsgAuth,
		Auth: &AuthPayload{
			Token:         a.cfg.AgentToken,
			Hostname:      hostname,
			OS:            runtime.GOOS,
			Arch:          runtime.GOARCH,
			AgentVersion:  agentVersion,
			DockerVersion: dockerVersion,
			TransferURL:   a.transferAdvertiseURL(),
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
			bodyReader.CloseWithError(context.Canceled)
		}
		for _, upload := range spooledUploads {
			upload.Cleanup(a.logger)
		}
	}()

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
			conn.WriteJSON(WSMessage{Type: MsgPong})

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
					bodyReader.Close()
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
				bodyReader.Close()
			}

		case MsgHTTPCancel:
			cancelMu.Lock()
			if cancel, ok := cancels[msg.ID]; ok {
				cancel()
				delete(cancels, msg.ID)
			}
			if bodyReader := uploads[msg.ID]; bodyReader != nil {
				bodyReader.CloseWithError(context.Canceled)
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
				a.proxy.ResizeExec(msg.ExecResize.ExecID, msg.ExecResize.Cols, msg.ExecResize.Rows)
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
		}
	}
}

func (a *Agent) transferAdvertiseURL() string {
	if a.transfer == nil {
		return ""
	}
	return a.transfer.AdvertiseURL()
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
	defer reader.Close()

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

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		a.sendTransferResult(conn, MsgTransferResult, payload.TransferID, false, progressReader.bytes.Load(), "", 0, fmt.Errorf("uploading direct image transfer: %w", err))
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
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
	defer reader.Close()

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

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		a.sendTransferResult(conn, MsgTransferResult, payload.TransferID, false, progressReader.bytes.Load(), "", 0, fmt.Errorf("uploading direct archive transfer: %w", err))
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
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

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		a.sendTransferResult(conn, MsgTransferResult, payload.TransferID, false, 0, "", 0, fmt.Errorf("running direct transfer probe: %w", err))
		return
	}
	defer resp.Body.Close()
	responderAgentMarker := strings.TrimSpace(resp.Header.Get("X-McHarbor-Agent-Marker"))
	if resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
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
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		a.sendTransferResult(conn, MsgTransferResult, payload.TransferID, false, 0, "", 0, fmt.Errorf("downloading restore archive: %w", err))
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
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
	conn.WriteJSON(WSMessage{
		Type: MsgTransferProgress,
		Transfer: &TransferPayload{
			TransferID: transferID,
			Bytes:      bytes,
			Stage:      strings.TrimSpace(stage),
		},
	})
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
		_ = writeHostStatsResponse(ctx, conn, id, HostStatsPayload{
			Supported: false,
			Error:     "host stats are not supported on this platform",
		})
		return
	}
	_ = writeHostStatsResponse(ctx, conn, id, stats)
}

// handlePruneRequest runs a docker prune on the local socket and returns
// the result over the WebSocket. Mirrors the server-side handler so the
// server can run prune operations against agent environments.
func (a *Agent) handlePruneRequest(ctx context.Context, conn *websocket.Conn, id string, payload PrunePayload) {
	result, err := a.proxy.Prune(ctx, payload)
	if err != nil {
		a.logger.Warn("agent prune failed", "type", payload.Type, "volumes", payload.Volumes, "error", err)
		_ = writePruneResponse(ctx, conn, id, PrunePayload{
			Type:    payload.Type,
			Volumes: payload.Volumes,
			Success: false,
			Error:   err.Error(),
		})
		return
	}
	_ = writePruneResponse(ctx, conn, id, result)
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
