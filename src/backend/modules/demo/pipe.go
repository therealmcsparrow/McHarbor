// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package demo

import (
	"context"
	"errors"
	"net"
	"sync"
)

// pipeListener is a net.Listener whose connections are in-memory pipes, so the
// demo agents reach the agent handler without a socket or the network limits.
type pipeListener struct {
	conns chan net.Conn
	done  chan struct{}
	once  sync.Once
}

func newPipeListener() *pipeListener {
	return &pipeListener{conns: make(chan net.Conn), done: make(chan struct{})}
}

var errListenerClosed = errors.New("demo pipe listener closed")

// Accept returns the server end of the next dialled pipe.
func (l *pipeListener) Accept() (net.Conn, error) {
	select {
	case c := <-l.conns:
		return c, nil
	case <-l.done:
		return nil, errListenerClosed
	}
}

// Close stops Accept and refuses further dials.
func (l *pipeListener) Close() error {
	l.once.Do(func() { close(l.done) })
	return nil
}

// Addr names the listener for logs.
func (l *pipeListener) Addr() net.Addr { return pipeAddr("demo-agents") }

// dial hands the server a new pipe whose remote address reads as remote, the
// address the environment page shows for the agent.
func (l *pipeListener) dial(ctx context.Context, remote string) (net.Conn, error) {
	server, client := net.Pipe()
	select {
	case l.conns <- &addrConn{Conn: server, remote: pipeAddr(remote)}:
		return client, nil
	case <-l.done:
		return nil, errListenerClosed
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

type pipeAddr string

func (a pipeAddr) Network() string { return "tcp" }
func (a pipeAddr) String() string  { return string(a) }

type addrConn struct {
	net.Conn
	remote net.Addr
}

func (c *addrConn) RemoteAddr() net.Addr { return c.remote }
