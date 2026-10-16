// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

//go:build linux

package qmp

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"syscall"
	"time"

	"github.com/therealmcsparrow/mcharbor-agent/internal/errlog"
)

// SendFD hands QEMU an open descriptor under name, with getfd over the monitor
// socket. A macvtap has no name QEMU can open, so a NIC plugged into a running
// guest on a direct network reaches it this way and only this way.
func (m *Monitor) SendFD(ctx context.Context, name string, fd int) error {
	conn, ok := m.conn.(*net.UnixConn)
	if !ok {
		return fmt.Errorf("qmp: getfd needs a unix socket monitor")
	}
	if _, ok := ctx.Deadline(); !ok {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, defaultCommandTimeout)
		defer cancel()
	}

	id := fmt.Sprintf("mch-%d", m.seq.Add(1))
	payload, err := json.Marshal(command{Execute: "getfd", Arguments: map[string]string{"fdname": name}, ID: id})
	if err != nil {
		return fmt.Errorf("qmp: encode getfd: %w", err)
	}
	payload = append(payload, '\n')

	ch := make(chan reply, 1)
	m.pendingMu.Lock()
	m.pending[id] = ch
	m.pendingMu.Unlock()

	m.mu.Lock()
	if deadline, ok := ctx.Deadline(); ok {
		errlog.Log(conn.SetWriteDeadline(deadline))
	}
	_, _, err = conn.WriteMsgUnix(payload, syscall.UnixRights(fd), nil)
	errlog.Log(conn.SetWriteDeadline(time.Time{}))
	m.mu.Unlock()
	if err != nil {
		m.pendingMu.Lock()
		delete(m.pending, id)
		m.pendingMu.Unlock()
		return fmt.Errorf("qmp: write getfd: %w", err)
	}
	_, err = m.await(ctx, "getfd", id, ch)
	return err
}
