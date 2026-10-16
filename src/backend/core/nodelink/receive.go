// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package nodelink

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

// ServeAgentRPC runs a forwarded call on this node's agent connection.
//
// It refuses rather than chains: a node that no longer holds the agent answers 409, so a
// stale lease produces one readable failure instead of a hop between nodes.
func (l *Link) ServeAgentRPC(w http.ResponseWriter, r *http.Request, envID string) {
	if refuseBrowser(w, r) {
		return
	}
	if l == nil || l.signer == nil {
		refuse(w, http.StatusServiceUnavailable, "this node has no shared secret and cannot verify a forwarded call")
		return
	}
	if l.refusePlaintext(w, r, HopAgentRPC) {
		return
	}

	// The HMAC over the headers and the claimed digest is checked before a byte of the body
	// is read, so only a caller holding the shared secret makes this node read anything.
	claims, err := l.signer.Authenticate(r, HopAgentRPC, envID, l.NodeID())
	if err != nil {
		l.refuseUnverified(w, r, HopAgentRPC, err, "env", envID)
		return
	}
	// The hop label already binds the signature to this route; a Docker path that names
	// another hop's claim path is refused as well, so no claim is ever read two ways.
	if isOtherHopPath(claims.Path) {
		l.logRefusal(HopAgentRPC, fmt.Errorf("%w: signed for another hop's path %s", ErrBadSignature, claims.Path),
			claims.NodeID, "env", envID)
		refuse(w, http.StatusUnauthorized, "the node signature is not valid")
		return
	}
	if !l.admitVerified(w, r, HopAgentRPC, claims.NodeID) {
		return
	}
	body, digest := r.Body, r.Header.Get(HeaderBodyDigest)
	var buffered []byte
	if digest != streamDigest {
		// A hashed body is always sent with its length, never above maxSignedBody.
		if r.ContentLength < 0 || r.ContentLength > maxSignedBody {
			refuse(w, http.StatusRequestEntityTooLarge, "a digest-signed body must declare a length of at most 1 MiB")
			return
		}
		raw, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxSignedBody))
		if err != nil {
			refuse(w, http.StatusBadRequest, "the forwarded body could not be read")
			return
		}
		buffered = raw
		body = io.NopCloser(bytes.NewReader(raw))
	}

	if err := l.signer.Accept(r, buffered); err != nil {
		l.logRefusal(HopAgentRPC, err, r.Header.Get(HeaderNode), "env", envID)
		refuse(w, http.StatusUnauthorized, "the node signature is not valid")
		return
	}

	transport := l.local.Transport(envID)
	if transport == nil {
		refuse(w, http.StatusConflict, "this node does not hold the agent connection for "+envID)
		return
	}

	inner, cancel, err := l.rebuildRequest(r, claims, body)
	if err != nil {
		refuse(w, http.StatusBadRequest, "the forwarded request could not be rebuilt")
		return
	}
	defer cancel()

	l.logger.Debug("nodelink: executing a forwarded agent call", "env", envID, "peer", claims.NodeID,
		"actor", claims.ActorID, "method", claims.Method, "path", claims.Path)

	resp, err := transport.RoundTrip(inner)
	if err != nil {
		refuse(w, http.StatusBadGateway, "the agent call failed: "+err.Error())
		return
	}
	defer closeQuietly(resp.Body)

	header := w.Header()
	for name, values := range resp.Header {
		if len(values) > 0 {
			header.Set(HeaderInnerPrefix+name, values[0])
		}
	}
	header.Set(HeaderStatus, strconv.Itoa(resp.StatusCode))
	header.Set(HeaderExecuted, l.NodeID())
	header.Set(HeaderRPC, "1")
	header.Set("Content-Type", "application/octet-stream")
	// The asking node acts on this status and these headers, so they are signed; the body
	// streams after them and rests on TLS.
	signature, err := l.signer.SignReplyHead(claims.NodeID, l.NodeID(), strings.TrimSpace(r.Header.Get(HeaderNonce)), header)
	if err != nil {
		l.logger.Warn("nodelink: an agent reply carries a header that cannot be forwarded", "env", envID,
			"peer", claims.NodeID, "error", err)
		for name := range header {
			header.Del(name)
		}
		refuse(w, http.StatusBadGateway, "the agent reply carries a header that cannot be forwarded intact")
		return
	}
	header.Set(HeaderReplySignature, signature)
	// The agent's status is the tunnel's status, so an asking node that only reads the
	// outer code still reads the right one.
	w.WriteHeader(resp.StatusCode)
	stream(w, resp.Body)
}

// isOtherHopPath reports whether path is the claim path of a hop other than agent-rpc.
func isOtherHopPath(path string) bool {
	return path == statsClaimPath || path == streamClaimPath || strings.HasPrefix(path, "/workflow-trigger/")
}

// rebuildRequest turns the signed claims back into the request the agent transport
// expects, with the asking node's remaining timeout applied here too.
func (l *Link) rebuildRequest(r *http.Request, claims Claims, body io.Reader) (*http.Request, context.CancelFunc, error) {
	target := &url.URL{Scheme: "http", Host: "agent", Path: claims.Path, RawQuery: claims.Query}
	ctx, cancel := context.WithCancel(r.Context())
	if claims.Timeout > 0 {
		cancel()
		ctx, cancel = context.WithTimeout(r.Context(), claims.Timeout)
	}
	inner, err := http.NewRequestWithContext(ctx, claims.Method, target.String(), body)
	if err != nil {
		cancel()
		return nil, func() {}, err
	}
	for name, values := range r.Header {
		if !strings.HasPrefix(name, HeaderInnerPrefix) || len(values) == 0 {
			continue
		}
		inner.Header.Set(strings.TrimPrefix(name, HeaderInnerPrefix), values[0])
	}
	if length, err := strconv.ParseInt(inner.Header.Get("Content-Length"), 10, 64); err == nil {
		inner.ContentLength = length
	} else if body == nil {
		inner.ContentLength = 0
	} else {
		inner.ContentLength = -1
	}
	return inner, cancel, nil
}

// stream copies the agent's body to the reply, flushing so a long-lived Docker stream
// arrives line by line rather than buffer by buffer.
func stream(w http.ResponseWriter, body io.Reader) {
	flusher, canFlush := w.(http.Flusher)
	if canFlush {
		flusher.Flush()
	}
	buf := make([]byte, 32*1024)
	for {
		n, err := body.Read(buf)
		if n > 0 {
			if _, writeErr := w.Write(buf[:n]); writeErr != nil {
				return
			}
			if canFlush {
				flusher.Flush()
			}
		}
		if err != nil {
			return
		}
	}
}

// refuseBrowser answers 403 to any request carrying an Origin header. A node never sends
// one, so its presence means a browser, and the refusal comes before anything else runs.
func refuseBrowser(w http.ResponseWriter, r *http.Request) bool {
	if r.Header.Get("Origin") == "" {
		return false
	}
	refuse(w, http.StatusForbidden, "a node-to-node endpoint does not accept browser requests")
	return true
}

// refuse answers a tunnel-level failure. It deliberately does not set HeaderRPC, which
// is how the asking node tells a refusal here from a refusal by the Docker daemon.
func refuse(w http.ResponseWriter, status int, detail string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if _, err := w.Write([]byte(`{"success":false,"error":` + strconv.Quote(detail) + `}`)); err != nil {
		return // the asking node hung up; the status line above already carries the refusal
	}
}
