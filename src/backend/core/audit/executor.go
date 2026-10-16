// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package audit

import (
	"context"
	"net/http"
	"sync"
)

// executorHolder carries the node that actually ran a request's remote work.
//
// A pointer in the request context rather than a context value, because the node is
// discovered deep inside the Docker round trip — long after the context was built — and
// a context value cannot be written back up the call stack.
type executorHolder struct {
	mu   sync.Mutex
	node string
}

type executorKey struct{}

// WithExecutor installs the holder the forwarding transport writes into. Every API
// request gets one, so a handler that never leaves this node simply leaves it empty.
func WithExecutor(ctx context.Context) context.Context {
	if ctx.Value(executorKey{}) != nil {
		return ctx
	}
	return context.WithValue(ctx, executorKey{}, &executorHolder{})
}

// RecordExecutor names the node that executed this request's agent call, so the audit
// row says where the work ran and not only which node was asked.
func RecordExecutor(ctx context.Context, nodeID string) {
	holder, ok := ctx.Value(executorKey{}).(*executorHolder)
	if !ok || nodeID == "" {
		return
	}
	holder.mu.Lock()
	defer holder.mu.Unlock()
	holder.node = nodeID
}

// ExecutorFromContext is the node recorded for this request, empty when the work ran
// here.
func ExecutorFromContext(ctx context.Context) string {
	holder, ok := ctx.Value(executorKey{}).(*executorHolder)
	if !ok {
		return ""
	}
	holder.mu.Lock()
	defer holder.mu.Unlock()
	return holder.node
}

// ExecutorMiddleware installs the holder for every API request.
func ExecutorMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		next.ServeHTTP(w, r.WithContext(WithExecutor(r.Context())))
	})
}

// executorOf is the value written into audit_logs.exec_node_id, nil when the action ran
// on the node that recorded it.
func executorOf(r *http.Request) *string {
	if r == nil {
		return nil
	}
	node := ExecutorFromContext(r.Context())
	if node == "" {
		return nil
	}
	return &node
}
