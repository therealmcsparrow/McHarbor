// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

//go:build !linux

package main

import (
	"context"

	"github.com/gorilla/websocket"
)

// handleHostLinkRequest refuses: a macvlan child is a Linux kernel interface.
// It still answers, because a server waiting on silence hangs until its timeout.
func (a *Agent) handleHostLinkRequest(_ context.Context, conn *websocket.Conn, reqID string, req HostLinkPayload) {
	reply := &HostLinkPayload{
		Op:        req.Op,
		Owner:     req.Owner,
		OwnerID:   req.OwnerID,
		Name:      req.Name,
		ErrorCode: HostLinkErrNotLinux,
		Error:     "host links are macvlan interfaces and need a Linux host",
	}
	writeMu.Lock()
	writeErr := conn.WriteJSON(WSMessage{Type: MsgHostLinkResponse, ID: reqID, HostLink: reply})
	writeMu.Unlock()
	if writeErr != nil {
		a.logger.Warn("host links: sending the reply failed", "op", req.Op, "error", writeErr)
	}
}

// startHostLinkReplay has nothing to restore off Linux.
func (a *Agent) startHostLinkReplay(context.Context) {}
