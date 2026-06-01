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

const agentVersion = "1.3.5"

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
		transfer: NewTransferServer(cfg.TransferListen, cfg.TransferAdvertiseURL, proxy, logger),
	}
}

func (a *Agent) StartTransferServer(ctx context.Context) error {
	if a.transfer == nil {
		return nil
	}
	return a.transfer.Start(ctx)
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

	// Track in-flight request cancellations
	var cancelMu sync.Mutex
	cancels := make(map[string]context.CancelFunc)
	transferCancels := make(map[string]context.CancelFunc)
	uploads := make(map[string]*uploadBuffer)
	spooledUploads := make(map[string]*spooledUpload)
	defer func() {
		cancelMu.Lock()
		defer cancelMu.Unlock()
		for _, cancel := range cancels {
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
	uploadURL, err := a.transfer.Prepare(payload.TransferID, payload.Token)
	if err != nil {
		a.sendTransferResult(conn, MsgTransferReady, payload.TransferID, false, 0, "", err)
		return
	}
	a.sendTransferResult(conn, MsgTransferReady, payload.TransferID, true, 0, uploadURL, nil)
}

func (a *Agent) runImageTransfer(ctx context.Context, conn *websocket.Conn, payload TransferPayload) {
	if strings.TrimSpace(payload.TransferID) == "" || strings.TrimSpace(payload.ImageRef) == "" || strings.TrimSpace(payload.URL) == "" || strings.TrimSpace(payload.Token) == "" {
		a.sendTransferResult(conn, MsgTransferResult, payload.TransferID, false, 0, "", fmt.Errorf("invalid direct image transfer request"))
		return
	}

	reader, err := a.proxy.SaveImage(ctx, payload.ImageRef)
	if err != nil {
		a.sendTransferResult(conn, MsgTransferResult, payload.TransferID, false, 0, "", err)
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
		a.sendTransferResult(conn, MsgTransferResult, payload.TransferID, false, 0, "", fmt.Errorf("building direct transfer upload request: %w", err))
		return
	}
	req.Header.Set("Authorization", "Bearer "+payload.Token)
	req.Header.Set("Content-Type", "application/x-tar")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		a.sendTransferResult(conn, MsgTransferResult, payload.TransferID, false, progressReader.bytes.Load(), "", fmt.Errorf("uploading direct image transfer: %w", err))
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		a.sendTransferResult(conn, MsgTransferResult, payload.TransferID, false, progressReader.bytes.Load(), "", fmt.Errorf("target direct upload returned status %d: %s", resp.StatusCode, strings.TrimSpace(string(body))))
		return
	}
	if _, err := io.Copy(io.Discard, resp.Body); err != nil {
		a.sendTransferResult(conn, MsgTransferResult, payload.TransferID, false, progressReader.bytes.Load(), "", fmt.Errorf("reading direct upload response: %w", err))
		return
	}

	a.sendTransferResult(conn, MsgTransferResult, payload.TransferID, true, progressReader.bytes.Load(), "", nil)
}

func (r *transferProgressReader) Read(p []byte) (int, error) {
	n, err := r.reader.Read(p)
	if n > 0 {
		total := r.bytes.Add(int64(n))
		if r.lastEmit.IsZero() || time.Since(r.lastEmit) >= 3*time.Second {
			r.lastEmit = time.Now()
			writeMu.Lock()
			r.conn.WriteJSON(WSMessage{
				Type: MsgTransferProgress,
				Transfer: &TransferPayload{
					TransferID: r.transferID,
					Bytes:      total,
				},
			})
			writeMu.Unlock()
		}
	}
	return n, err
}

func (a *Agent) sendTransferResult(conn *websocket.Conn, msgType, transferID string, success bool, bytes int64, uploadURL string, err error) {
	payload := &TransferPayload{
		TransferID: transferID,
		Success:    success,
		Bytes:      bytes,
		URL:        uploadURL,
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
