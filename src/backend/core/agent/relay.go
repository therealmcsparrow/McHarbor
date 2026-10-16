// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package agent

import (
	"errors"
	"strings"

	"github.com/gorilla/websocket"
)

// RelayTap receives the agent frames that belong to a session another node is running
// through this node's agent connection.
//
// DeliverRelay is called from ReadLoop, which multiplexes every session of the agent, so
// it must never block: a tap that cannot keep up drops or tears itself down instead.
type RelayTap interface {
	DeliverRelay(raw []byte, msg *WSMessage)
}

// AddRelayTap routes every frame the agent sends under id to tap instead of to local
// dispatch. It must be called before the frame that opens the session is written, for
// the same reason StartConsole registers before it writes.
func (t *AgentTransport) AddRelayTap(id string, tap RelayTap) {
	if id == "" || tap == nil {
		return
	}
	t.mu.Lock()
	t.relayTaps[id] = tap
	t.mu.Unlock()
}

// RemoveRelayTap drops id's tap, but only while it still belongs to tap, so a relay that
// is tearing down cannot remove a registration another relay made since.
func (t *AgentTransport) RemoveRelayTap(id string, tap RelayTap) {
	t.mu.Lock()
	if current, ok := t.relayTaps[id]; ok && current == tap {
		delete(t.relayTaps, id)
	}
	t.mu.Unlock()
}

// RelayTapCount reports how many ids are currently relayed to another node.
func (t *AgentTransport) RelayTapCount() int {
	t.mu.Lock()
	defer t.mu.Unlock()
	return len(t.relayTaps)
}

// relayTapFor finds the tap for an agent frame by its request id, or by the transfer id
// that transfer and backup frames are keyed on instead.
func (t *AgentTransport) relayTapFor(msg *WSMessage) RelayTap {
	t.mu.Lock()
	defer t.mu.Unlock()
	if len(t.relayTaps) == 0 {
		return nil
	}
	for _, key := range RelayKeys(msg) {
		if tap, ok := t.relayTaps[key]; ok {
			return tap
		}
	}
	return nil
}

// RelayKeys lists the ids a frame can be routed by: its request id, and the transfer id a
// transfer or backup frame carries in its payload rather than in ID.
func RelayKeys(msg *WSMessage) []string {
	keys := make([]string, 0, 2)
	if msg.ID != "" {
		keys = append(keys, msg.ID)
	}
	if msg.Transfer != nil && msg.Transfer.TransferID != "" {
		keys = append(keys, msg.Transfer.TransferID)
	}
	if msg.Backup != nil && msg.Backup.TransferID != "" {
		keys = append(keys, msg.Backup.TransferID)
	}
	return keys
}

// Done is closed when the transport stops, whichever side ended it.
func (t *AgentTransport) Done() <-chan struct{} { return t.done }

// CloseReason says why the transport stopped, in words an operator can act on; it is
// empty while the transport runs.
func (t *AgentTransport) CloseReason() string {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.closeReason
}

// SetDisconnectReason sets what CloseReason reports when the socket drops without a
// close frame, so a relayed session can name the node link rather than "the agent".
func (t *AgentTransport) SetDisconnectReason(reason string) {
	t.mu.Lock()
	t.disconnectReason = reason
	t.mu.Unlock()
}

// ConnectionInfo returns the agent's identity and capabilities without its socket, which
// is what another node needs to run a virtual connection with the same feature gates.
func (t *AgentTransport) ConnectionInfo() AgentConnection {
	c := t.conn
	c.mu.Lock()
	version := c.Version
	c.mu.Unlock()
	return AgentConnection{
		EnvID:              c.EnvID,
		Hostname:           c.Hostname,
		OS:                 c.OS,
		Arch:               c.Arch,
		Version:            version,
		Build:              c.Build,
		RemoteIP:           c.RemoteIP,
		DockerVer:          c.DockerVer,
		TLSMode:            c.TLSMode,
		ServerURL:          c.ServerURL,
		TransferURL:        c.TransferURL,
		TransferCandidates: append([]string(nil), c.TransferCandidates...),
	}
}

// finish records why the read loop ended and ends every session still open on it, so a
// terminal does not wait on a socket nothing will ever read from again.
func (t *AgentTransport) finish(err error) {
	t.mu.Lock()
	reason := closeReasonFor(err, t.disconnectReason)
	t.closeReason = reason
	sessions := make([]*ExecSession, 0, len(t.execSessions))
	for id, s := range t.execSessions {
		sessions = append(sessions, s)
		delete(t.execSessions, id)
	}
	terminals := make([]chan *WSMessage, 0, len(t.hostTerminalWaiters))
	for _, ch := range t.hostTerminalWaiters {
		terminals = append(terminals, ch)
	}
	t.mu.Unlock()

	for _, s := range sessions {
		s.end(reason)
	}
	// A nil frame is the pump's end-of-session signal; the waiter's owner still pops it.
	for _, ch := range terminals {
		select {
		case ch <- nil:
		default:
		}
	}
	t.doneOnce.Do(func() { close(t.done) })
}

// closeReasonFor prefers the reason the far end wrote in its close frame, then the one the
// transport was told to expect, and only then the generic one.
func closeReasonFor(err error, fallback string) string {
	var closeErr *websocket.CloseError
	if errors.As(err, &closeErr) && strings.TrimSpace(closeErr.Text) != "" {
		return closeErr.Text
	}
	if strings.TrimSpace(fallback) != "" {
		return fallback
	}
	return "the agent disconnected"
}

// Reason says why the session ended when it ended because the transport under it stopped;
// it is empty when the agent ended the session itself.
func (s *ExecSession) Reason() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.reason
}

// end closes the session once, recording reason when there is one.
func (s *ExecSession) end(reason string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.reason == "" {
		s.reason = reason
	}
	select {
	case <-s.DoneCh:
	default:
		close(s.DoneCh)
	}
}
