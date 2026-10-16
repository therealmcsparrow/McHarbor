// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package nodelink

import (
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"

	"github.com/therealmcsparrow/mcharbor/core/httpx"
)

// EnvAllowHTTP is the operator's opt-in to node-to-node calls over plain http in every
// multi-node mode, for a private network where the path between the nodes is already trusted.
// It applies to both directions: this node sends to http peers and accepts plaintext calls.
const EnvAllowHTTP = "MCHARBOR_NODE_LINK_ALLOW_HTTP"

// ErrPlaintextPeer means a peer advertises an address that is not https while this node
// requires TLS between nodes; nothing signed is sent to it.
var ErrPlaintextPeer = errors.New("nodelink: peer advertises an address that is not https and " + EnvAllowHTTP + " is not set")

// errPlaintextRequest means a node-to-node request reached this node without TLS.
var errPlaintextRequest = errors.New("nodelink: node-to-node request did not arrive over TLS")

// PlaintextAllowed reports whether the operator opted in to plain http between nodes.
func PlaintextAllowed() bool {
	return strings.TrimSpace(os.Getenv(EnvAllowHTTP)) == "1"
}

// IsPlaintextURL reports whether an advertise URL would carry node calls without TLS: any
// address that does not parse as an https URL. An empty address is not plaintext; it is no
// address, which the callers report on its own.
func IsPlaintextURL(advertise string) bool {
	trimmed := strings.TrimSpace(advertise)
	if trimmed == "" {
		return false
	}
	u, err := url.Parse(trimmed)
	return err != nil || !strings.EqualFold(u.Scheme, "https")
}

// SetRequireTLS switches the refusal to send signed calls to a non-https peer and to accept
// them without TLS. New turns it on in every mode and production never turns it off; tests
// turn it off to reach httptest servers.
func (l *Link) SetRequireTLS(on bool) {
	if l != nil {
		l.requireTLS.Store(on)
	}
}

// checkPeerURL refuses a plaintext peer when TLS is required and the operator has not
// opted in, so a signed body, nonce and signature never cross an unencrypted path.
func (l *Link) checkPeerURL(nodeID, advertise string) error {
	if l.requireTLS.Load() && IsPlaintextURL(advertise) && !PlaintextAllowed() {
		return fmt.Errorf("%w: node %s at %s", ErrPlaintextPeer, nodeID, strings.TrimSpace(advertise))
	}
	return nil
}

// refusePlaintext answers 403 to a node-to-node request that did not arrive over TLS, unless
// this node opted in with EnvAllowHTTP. Enforcing TLS only on send would let a peer whose
// build or configuration is wrong deliver signed calls in the clear without anyone noticing.
func (l *Link) refusePlaintext(w http.ResponseWriter, r *http.Request, hop string) bool {
	if !l.requireTLS.Load() || PlaintextAllowed() || arrivedOverTLS(r) {
		return false
	}
	l.logRefusal(hop, errPlaintextRequest, r.Header.Get(HeaderNode))
	refuse(w, http.StatusForbidden, "node-to-node calls must arrive over https; "+EnvAllowHTTP+"=1 on this node accepts plain http on a trusted private network")
	return true
}

// arrivedOverTLS reports whether r reached this node over TLS: its own listener terminated
// it, or a proxy inside MCHARBOR_TRUSTED_PROXY_CIDRS did and says so in X-Forwarded-Proto.
// The header from any other peer is ignored, since a client can send whatever it likes.
func arrivedOverTLS(r *http.Request) bool {
	if r.TLS != nil {
		return true
	}
	peer := strings.TrimSpace(r.RemoteAddr)
	if host, _, err := net.SplitHostPort(peer); err == nil {
		peer = host
	}
	if !httpx.IsTrustedProxy(peer) {
		return false
	}
	proto := r.Header.Get("X-Forwarded-Proto")
	if i := strings.IndexByte(proto, ','); i >= 0 {
		proto = proto[:i]
	}
	return strings.EqualFold(strings.TrimSpace(proto), "https")
}
