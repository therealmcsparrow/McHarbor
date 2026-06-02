// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

import (
	"context"
	"crypto/subtle"
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

// TransferServer receives one-use direct uploads from peer agents.
type TransferServer struct {
	listen       string
	advertiseURL string
	proxy        *Proxy
	logger       *slog.Logger
	server       *http.Server
	receivers    map[string]transferReceiver
	mu           sync.Mutex
}

func NewTransferServer(listen, advertiseURL string, proxy *Proxy, logger *slog.Logger) *TransferServer {
	listen = strings.TrimSpace(listen)
	advertiseURL = strings.TrimRight(strings.TrimSpace(advertiseURL), "/")
	if listen == "" || advertiseURL == "" {
		return nil
	}
	return &TransferServer{
		listen:       listen,
		advertiseURL: advertiseURL,
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

func (s *TransferServer) Prepare(transferID, token string) (string, error) {
	return s.prepareReceiver(transferID, token, transferKindImage, "/api/transfer/image/")
}

func (s *TransferServer) PrepareProbe(transferID, token string) (string, error) {
	return s.prepareReceiver(transferID, token, transferKindProbe, "/api/transfer/probe/")
}

func (s *TransferServer) prepareReceiver(transferID, token, kind, pathPrefix string) (string, error) {
	if s == nil {
		return "", fmt.Errorf("direct transfer receiver is not configured")
	}
	transferID = strings.TrimSpace(transferID)
	token = strings.TrimSpace(token)
	if transferID == "" || token == "" {
		return "", fmt.Errorf("invalid direct transfer receiver request")
	}

	now := time.Now()
	s.mu.Lock()
	for id, receiver := range s.receivers {
		if now.After(receiver.expiresAt) {
			delete(s.receivers, id)
		}
	}
	s.receivers[transferID] = transferReceiver{
		token:     token,
		kind:      kind,
		expiresAt: now.Add(transferReceiverTTL),
	}
	s.mu.Unlock()

	return s.advertiseURL + pathPrefix + transferID, nil
}

func (s *TransferServer) Cancel(transferID string) {
	if s == nil {
		return
	}
	s.mu.Lock()
	delete(s.receivers, transferID)
	s.mu.Unlock()
}

func (s *TransferServer) handleImageUpload(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	transferID := strings.TrimPrefix(r.URL.Path, "/api/transfer/image/")
	if transferID == "" || strings.Contains(transferID, "/") {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}

	if !s.consumeReceiver(transferID, transferKindImage, r.Header.Get("Authorization")) {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		if r.Body != nil {
			io.Copy(io.Discard, io.LimitReader(r.Body, 1024))
		}
		return
	}

	if err := s.proxy.LoadImage(r.Context(), r.Body); err != nil {
		s.logger.Warn("direct transfer image load failed", "transferId", transferID, "error", err)
		http.Error(w, "image load failed", http.StatusBadGateway)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (s *TransferServer) handleProbeUpload(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	transferID := strings.TrimPrefix(r.URL.Path, "/api/transfer/probe/")
	if transferID == "" || strings.Contains(transferID, "/") {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}

	if !s.consumeReceiver(transferID, transferKindProbe, r.Header.Get("Authorization")) {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		if r.Body != nil {
			io.Copy(io.Discard, io.LimitReader(r.Body, 1024))
		}
		return
	}
	if r.Body != nil {
		io.Copy(io.Discard, io.LimitReader(r.Body, 1024))
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *TransferServer) consumeReceiver(transferID, kind, authHeader string) bool {
	token := strings.TrimPrefix(authHeader, "Bearer ")

	s.mu.Lock()
	receiver, ok := s.receivers[transferID]
	if ok && time.Now().After(receiver.expiresAt) {
		delete(s.receivers, transferID)
		ok = false
	}
	if ok {
		delete(s.receivers, transferID)
	}
	s.mu.Unlock()

	if !ok || receiver.kind != kind {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(token), []byte(receiver.token)) == 1
}
