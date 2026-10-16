// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// Package fakeagent is the in-process stand-in for a remote agent in demo mode.
// It authenticates with the real agent handshake and answers every message
// from a simulated host, so the rest of McHarbor cannot tell it from a real one.
package fakeagent

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"

	coreagent "github.com/therealmcsparrow/mcharbor/core/agent"
	"github.com/therealmcsparrow/mcharbor/core/demo/dockerapi"
	"github.com/therealmcsparrow/mcharbor/core/demo/estate"
)

// Sender writes one message to the server; it is safe for concurrent use.
type Sender func(msg *coreagent.WSMessage) error

// Extension answers a family of messages the core agent does not, such as the
// compute messages. Handle returns false when the message is not its to answer.
type Extension interface {
	Handle(ctx context.Context, host *estate.Host, msg *coreagent.WSMessage, send Sender) bool
}

// Config shapes one demo agent.
type Config struct {
	Host       *estate.Host
	Token      string
	Version    string
	Build      string
	Dial       func(ctx context.Context, token string) (*websocket.Conn, error)
	Logger     *slog.Logger
	Extensions []Extension
	// Swarm makes this host a simulated swarm manager when set.
	Swarm dockerapi.SwarmBackend
}

// Agent is one demo agent; Run keeps it connected until its context ends.
type Agent struct {
	cfg    Config
	docker http.Handler
}

// New returns an Agent for cfg.
func New(cfg Config) *Agent {
	if cfg.Logger == nil {
		cfg.Logger = slog.Default()
	}
	srv := dockerapi.New(cfg.Host)
	if cfg.Swarm != nil {
		srv.SetSwarm(cfg.Swarm)
	}
	return &Agent{cfg: cfg, docker: srv}
}

// Run connects, serves and reconnects with backoff until ctx is cancelled, the
// way a real agent survives a McHarbor restart.
func (a *Agent) Run(ctx context.Context) {
	backoff := time.Second
	for {
		err := a.session(ctx)
		if ctx.Err() != nil {
			return
		}
		a.cfg.Logger.Debug("demo agent disconnected", "host", a.cfg.Host.Key, "error", err)
		select {
		case <-ctx.Done():
			return
		case <-time.After(backoff):
		}
		backoff = min(backoff*2, 30*time.Second)
	}
}

type session struct {
	agent   *Agent
	conn    *websocket.Conn
	writeMu sync.Mutex
	ctx     context.Context //nolint:containedctx // session lifetime root, cancelled when the websocket closes

	mu       sync.Mutex
	requests map[string]*pendingRequest
	shells   map[string]*shell
	stats    *statsPusher
}

func (a *Agent) session(parent context.Context) error {
	conn, err := a.cfg.Dial(parent, a.cfg.Token)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithCancel(parent)
	defer cancel()
	s := &session{agent: a, conn: conn, ctx: ctx, requests: map[string]*pendingRequest{}, shells: map[string]*shell{}}
	defer func() {
		if cerr := conn.Close(); cerr != nil {
			a.cfg.Logger.Debug("demo agent close", "error", cerr)
		}
	}()
	go func() {
		<-ctx.Done()
		if cerr := conn.Close(); cerr != nil {
			a.cfg.Logger.Debug("demo agent close on cancel", "error", cerr)
		}
	}()
	h := a.cfg.Host
	auth := &coreagent.WSMessage{Type: coreagent.MsgAuth, Auth: &coreagent.AuthPayload{
		Token:         a.cfg.Token,
		Hostname:      h.Hostname,
		OS:            "linux",
		Arch:          h.Arch,
		AgentVersion:  a.cfg.Version,
		AgentBuild:    a.cfg.Build,
		DockerVersion: h.DockerVersion,
		InstallKind:   "binary",
		Capabilities:  []string{coreagent.HostServiceCapability},
		TLSMode:       "pinned",
		HostIP:        hostIP(h),
	}}
	if err := s.send(auth); err != nil {
		return err
	}
	var reply coreagent.WSMessage
	if err := conn.ReadJSON(&reply); err != nil {
		return err
	}
	if reply.Type != coreagent.MsgAuthResult || reply.AuthResult == nil || !reply.AuthResult.Success {
		return errors.New("demo agent authentication refused")
	}
	defer s.closeAll()
	for {
		_, raw, err := conn.ReadMessage()
		if err != nil {
			return err
		}
		var msg coreagent.WSMessage
		if err := json.Unmarshal(raw, &msg); err != nil {
			continue
		}
		s.dispatch(ctx, &msg)
	}
}

func hostIP(h *estate.Host) string {
	n := estate.StableHash(h.Key)
	return "10." + itoa(int(n%40)+10) + "." + itoa(int(n>>8%250)) + "." + itoa(int(n>>16%250)+2)
}

func (s *session) send(msg *coreagent.WSMessage) error {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	return s.conn.WriteJSON(msg)
}

func (s *session) closeAll() {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, p := range s.requests {
		p.cancel()
	}
	for _, sh := range s.shells {
		sh.close()
	}
	if s.stats != nil {
		s.stats.stop()
	}
}

func (s *session) dispatch(ctx context.Context, msg *coreagent.WSMessage) {
	switch msg.Type {
	case coreagent.MsgPing:
		s.reply(&coreagent.WSMessage{Type: coreagent.MsgPong})
	case coreagent.MsgPong:
	case coreagent.MsgHTTPRequest:
		s.startHTTP(ctx, msg.ID, msg.HTTPRequest, nil)
	case coreagent.MsgHTTPRequestStart:
		s.beginStreamedRequest(msg)
	case coreagent.MsgHTTPRequestChunk:
		s.appendRequestChunk(msg)
	case coreagent.MsgHTTPRequestEnd:
		s.finishStreamedRequest(ctx, msg.ID)
	case coreagent.MsgHTTPCancel:
		s.cancelRequest(msg.ID)
	case coreagent.MsgExecStart, coreagent.MsgExecInput, coreagent.MsgExecResize, coreagent.MsgExecEnd:
		s.handleExec(msg)
	case coreagent.MsgHostTerminalStart, coreagent.MsgHostTerminalInput, coreagent.MsgHostTerminalResize, coreagent.MsgHostTerminalEnd:
		s.handleHostTerminal(msg)
	case coreagent.MsgComposeRun:
		go s.handleCompose(msg)
	case coreagent.MsgComposeCancel:
	case coreagent.MsgStatsSubscribe:
		s.startStats(msg)
	case coreagent.MsgStatsPushAck:
		s.ackStats(msg)
	default:
		if s.handleHostRPC(msg) {
			return
		}
		for _, ext := range s.agent.cfg.Extensions {
			if ext.Handle(ctx, s.agent.cfg.Host, msg, s.send) {
				return
			}
		}
		s.agent.cfg.Logger.Debug("demo agent ignored message", "type", msg.Type)
	}
}

func (s *session) reply(msg *coreagent.WSMessage) {
	if err := s.send(msg); err != nil {
		s.agent.cfg.Logger.Debug("demo agent write failed", "type", msg.Type, "error", err)
	}
}
