// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"
)

const transferReceiverTTL = 30 * time.Minute
const (
	transferKindImage = "image"
	transferKindProbe = "probe"
)

type transferReceiver struct {
	token     string
	kind      string
	expiresAt time.Time
}

type transferReceiverAuthCheck struct {
	allowed       bool
	existed       bool
	expired       bool
	kindMatched   bool
	bearerPresent bool
	tokenMatched  bool
	receiverKind  string
}

type transferReporter func(WSMessage)

// TransferServer receives one-use direct uploads from peer agents.
type TransferServer struct {
	listen       string
	advertiseURL string
	agentMarker  string
	proxy        *Proxy
	logger       *slog.Logger
	server       *http.Server
	receivers    map[string]transferReceiver
	reporter     transferReporter
	mu           sync.Mutex
	reporterMu   sync.Mutex
}

func NewTransferServer(listen, advertiseURL, agentToken string, proxy *Proxy, logger *slog.Logger) *TransferServer {
	listen = strings.TrimSpace(listen)
	advertiseURL = strings.TrimRight(strings.TrimSpace(advertiseURL), "/")
	if listen == "" || advertiseURL == "" {
		return nil
	}
	return &TransferServer{
		listen:       listen,
		advertiseURL: advertiseURL,
		agentMarker:  shortTokenFingerprint(agentToken),
		proxy:        proxy,
		logger:       logger,
		receivers:    make(map[string]transferReceiver),
	}
}

func (s *TransferServer) Start(ctx context.Context) error {
	if s == nil {
		return nil
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/api/transfer/image/", s.handleImageUpload)
	mux.HandleFunc("/api/transfer/probe/", s.handleProbeUpload)
	s.server = &http.Server{
		Handler:           mux,
		ReadHeaderTimeout: 15 * time.Second,
	}
	listener, err := net.Listen("tcp", s.listen)
	if err != nil {
		return fmt.Errorf("listening for direct transfers: %w", err)
	}
	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := s.server.Shutdown(shutdownCtx); err != nil {
			s.logger.Warn("direct transfer server shutdown failed", "error", err)
		}
	}()
	go func() {
		s.logger.Info("direct transfer receiver listening", "listen", s.listen)
		if err := s.server.Serve(listener); err != nil && err != http.ErrServerClosed {
			s.logger.Error("direct transfer receiver stopped", "error", err)
		}
	}()
	return nil
}

func (s *TransferServer) AdvertiseURL() string {
	if s == nil {
		return ""
	}
	return s.advertiseURL
}

func (s *TransferServer) SetReporter(reporter transferReporter) {
	if s == nil {
		return
	}
	s.reporterMu.Lock()
	s.reporter = reporter
	s.reporterMu.Unlock()
}

func (s *TransferServer) Prepare(transferID, token string) (string, *TransferReceiverMarker, error) {
	return s.prepareReceiver(transferID, token, transferKindImage, "/api/transfer/image/")
}

func (s *TransferServer) PrepareProbe(transferID, token string) (string, *TransferReceiverMarker, error) {
	return s.prepareReceiver(transferID, token, transferKindProbe, "/api/transfer/probe/")
}

func (s *TransferServer) prepareReceiver(transferID, token, kind, pathPrefix string) (string, *TransferReceiverMarker, error) {
	if s == nil {
		return "", nil, fmt.Errorf("direct transfer receiver is not configured")
	}
	transferID = strings.TrimSpace(transferID)
	token = strings.TrimSpace(token)
	if transferID == "" || token == "" {
		return "", nil, fmt.Errorf("invalid direct transfer receiver request")
	}

	now := time.Now()
	expiresAt := now.Add(transferReceiverTTL)
	s.mu.Lock()
	for id, receiver := range s.receivers {
		if now.After(receiver.expiresAt) {
			delete(s.receivers, id)
		}
	}
	s.receivers[transferID] = transferReceiver{
		token:     token,
		kind:      kind,
		expiresAt: expiresAt,
	}
	s.mu.Unlock()

	receiverURL := s.advertiseURL + pathPrefix + transferID
	marker := &TransferReceiverMarker{
		TransferID:       transferID,
		Kind:             kind,
		ExpiresAt:        expiresAt.UTC().Format(time.RFC3339),
		TokenFingerprint: shortTokenFingerprint(token),
		AgentMarker:      s.agentMarker,
	}
	s.logger.Debug("direct transfer receiver prepared", "transferId", transferID, "kind", kind, "url", receiverURL, "tokenFingerprint", marker.TokenFingerprint, "agentMarker", marker.AgentMarker)
	return receiverURL, marker, nil
}

func (s *TransferServer) Cancel(transferID string) {
	if s == nil {
		return
	}
	s.mu.Lock()
	delete(s.receivers, transferID)
	s.mu.Unlock()
	s.logger.Debug("direct transfer receiver cancelled", "transferId", transferID)
}

func (s *TransferServer) handleImageUpload(w http.ResponseWriter, r *http.Request) {
	s.writeReceiverHeaders(w)
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	transferID := strings.TrimPrefix(r.URL.Path, "/api/transfer/image/")
	if transferID == "" || strings.Contains(transferID, "/") {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}

	check := s.consumeReceiver(transferID, transferKindImage, r.Header.Get("Authorization"))
	if !check.allowed {
		s.logReceiverAuthFailure(transferID, transferKindImage, r.RemoteAddr, check)
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		if r.Body != nil {
			io.Copy(io.Discard, io.LimitReader(r.Body, 1024))
		}
		return
	}
	s.logger.Debug("direct transfer receiver authorized", "transferId", transferID, "kind", transferKindImage, "remote", r.RemoteAddr)

	if err := s.proxy.LoadImage(r.Context(), r.Body); err != nil {
		s.logger.Warn("direct transfer image load failed", "transferId", transferID, "error", err)
		http.Error(w, "image load failed", http.StatusBadGateway)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (s *TransferServer) handleProbeUpload(w http.ResponseWriter, r *http.Request) {
	s.writeReceiverHeaders(w)
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	transferID := strings.TrimPrefix(r.URL.Path, "/api/transfer/probe/")
	if transferID == "" || strings.Contains(transferID, "/") {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}

	check := s.consumeReceiver(transferID, transferKindProbe, r.Header.Get("Authorization"))
	if !check.allowed {
		s.logReceiverAuthFailure(transferID, transferKindProbe, r.RemoteAddr, check)
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		if r.Body != nil {
			io.Copy(io.Discard, io.LimitReader(r.Body, 1024))
		}
		return
	}
	s.logger.Debug("direct transfer receiver authorized", "transferId", transferID, "kind", transferKindProbe, "remote", r.RemoteAddr)
	if r.Body != nil {
		io.Copy(io.Discard, io.LimitReader(r.Body, 1024))
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *TransferServer) consumeReceiver(transferID, kind, authHeader string) transferReceiverAuthCheck {
	check := transferReceiverAuthCheck{
		bearerPresent: strings.HasPrefix(authHeader, "Bearer ") && strings.TrimSpace(strings.TrimPrefix(authHeader, "Bearer ")) != "",
	}
	token := strings.TrimPrefix(authHeader, "Bearer ")

	s.mu.Lock()
	receiver, ok := s.receivers[transferID]
	check.existed = ok
	check.receiverKind = receiver.kind
	if ok && time.Now().After(receiver.expiresAt) {
		delete(s.receivers, transferID)
		check.expired = true
		ok = false
	}
	if ok {
		delete(s.receivers, transferID)
	}
	s.mu.Unlock()

	if !ok {
		return check
	}
	check.kindMatched = receiver.kind == kind
	if !check.kindMatched {
		return check
	}
	check.tokenMatched = subtle.ConstantTimeCompare([]byte(token), []byte(receiver.token)) == 1
	check.allowed = check.tokenMatched
	return check
}

func (s *TransferServer) logReceiverAuthFailure(transferID, kind, remote string, check transferReceiverAuthCheck) {
	s.logger.Debug("direct transfer receiver authorization failed",
		"transferId", transferID,
		"kind", kind,
		"remote", remote,
		"receiverExists", check.existed,
		"receiverExpired", check.expired,
		"receiverKind", check.receiverKind,
		"kindMatched", check.kindMatched,
		"bearerPresent", check.bearerPresent,
		"tokenMatched", check.tokenMatched,
		"agentMarker", s.agentMarker,
	)
	s.reportReceiverAuthFailure(transferID, kind, remote, check)
}

func (s *TransferServer) reportReceiverAuthFailure(transferID, kind, remote string, check transferReceiverAuthCheck) {
	s.reporterMu.Lock()
	reporter := s.reporter
	s.reporterMu.Unlock()
	if reporter == nil {
		return
	}
	reporter(WSMessage{
		Type: MsgTransferResult,
		Transfer: &TransferPayload{
			TransferID: transferID,
			Kind:       kind,
			StatusCode: http.StatusUnauthorized,
			Success:    false,
			Error:      "direct transfer receiver authorization failed",
			Diagnostic: &TransferAuthDiagnostic{
				ReceiverExists:       check.existed,
				ReceiverExpired:      check.expired,
				ReceiverKind:         check.receiverKind,
				KindMatched:          check.kindMatched,
				BearerPresent:        check.bearerPresent,
				TokenMatched:         check.tokenMatched,
				RemoteAddr:           remote,
				ResponderAgentMarker: s.agentMarker,
			},
		},
	})
}

func (s *TransferServer) writeReceiverHeaders(w http.ResponseWriter) {
	if strings.TrimSpace(s.agentMarker) != "" {
		w.Header().Set("X-McHarbor-Agent-Marker", s.agentMarker)
	}
	w.Header().Set("X-McHarbor-Transfer-Receiver", "mcharbor-agent")
}

func shortTokenFingerprint(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])[:16]
}
