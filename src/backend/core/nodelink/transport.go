// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package nodelink

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/rs/xid"

	"github.com/therealmcsparrow/mcharbor/core/audit"
	"github.com/therealmcsparrow/mcharbor/core/auth"
)

// ErrNoOwner means no node holds a live claim on this agent, which is what a genuinely
// disconnected agent looks like from any node in the cluster.
var ErrNoOwner = errors.New("nodelink: no node holds this agent connection")

// ErrCannotForward means this node has no shared secret or no advertised address, so a
// remote-owned agent is unreachable from here however alive it is.
var ErrCannotForward = errors.New("nodelink: this node is not configured to forward agent calls")

// RoutingTransport is the round tripper behind an agent environment's Docker client.
//
// It resolves the owner per request rather than at construction, so a client cached
// while the agent was local keeps working after the agent reconnects to another node,
// and the other way round.
type RoutingTransport struct {
	link  *Link
	envID string
}

// RoundTrip runs the request on the local agent transport when this node holds the
// connection, and forwards it to the owning node when it does not.
func (rt *RoutingTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if local := rt.link.local.Transport(rt.envID); local != nil {
		return local.RoundTrip(req)
	}
	owner, ok := rt.link.RemoteOwner(req.Context(), rt.envID)
	if !ok {
		return nil, fmt.Errorf("%w: env %s", ErrNoOwner, rt.envID)
	}
	if !rt.link.CanForward() {
		return nil, fmt.Errorf("%w: agent for env %s is held by node %s", ErrCannotForward, rt.envID, owner.NodeID)
	}
	if strings.TrimSpace(owner.AdvertiseURL) == "" {
		return nil, fmt.Errorf("nodelink: node %s holds env %s and advertises no address; set MCHARBOR_ADVERTISE_URL on it",
			owner.NodeID, rt.envID)
	}
	return rt.forward(req, owner)
}

// forward tunnels one request to the owning node and hands back the agent's own
// response, headers and status included, with the body still streaming.
func (rt *RoutingTransport) forward(req *http.Request, owner Owner) (*http.Response, error) {
	if err := rt.link.checkPeerURL(owner.NodeID, owner.AdvertiseURL); err != nil {
		rt.link.recordProof(Proof{Status: ProofFailed, NodeID: owner.NodeID, EnvID: rt.envID,
			At: time.Now().UTC(), Error: err.Error()})
		return nil, err
	}
	body, digest, err := signableBody(req)
	if err != nil {
		return nil, err
	}

	target := strings.TrimRight(owner.AdvertiseURL, "/") + "/api/internal/node/agent-rpc/" + rt.envID
	outbound, err := http.NewRequestWithContext(req.Context(), http.MethodPost, target, body)
	if err != nil {
		return nil, fmt.Errorf("nodelink: building the forwarded request: %w", err)
	}
	outbound.ContentLength = req.ContentLength
	if digest == streamDigest && req.ContentLength <= 0 {
		outbound.ContentLength = -1
	}
	// Never application/json: the /api chain caps a JSON body at 8 MiB and an image
	// load is larger than that by design.
	outbound.Header.Set("Content-Type", "application/octet-stream")
	for name, values := range req.Header {
		if len(values) > 0 {
			outbound.Header.Set(HeaderInnerPrefix+name, values[0])
		}
	}

	claims := Claims{
		Hop:      HopAgentRPC,
		NodeID:   rt.link.NodeID(),
		Audience: owner.NodeID,
		EnvID:    rt.envID,
		Method:   req.Method,
		Path:     req.URL.Path,
		Query:    req.URL.RawQuery,
	}
	if user := auth.UserFromContext(req.Context()); user != nil {
		claims.ActorID, claims.ActorName = user.ID, user.Username
	}
	if deadline, ok := req.Context().Deadline(); ok {
		if remaining := time.Until(deadline); remaining > 0 {
			claims.Timeout = remaining
		}
	}
	nonce := xid.New().String()
	if err := rt.link.signer.Sign(outbound, claims, nonce, digest); err != nil {
		closeBody(body)
		return nil, fmt.Errorf("nodelink: signing the call to node %s: %w", owner.NodeID, err)
	}

	resp, err := rt.link.client.Do(outbound)
	if err != nil {
		rt.link.recordProof(Proof{Status: ProofFailed, NodeID: owner.NodeID, EnvID: rt.envID,
			At: time.Now().UTC(), Error: err.Error()})
		return nil, fmt.Errorf("nodelink: forwarding to node %s: %w", owner.NodeID, err)
	}
	if resp.Header.Get(HeaderRPC) != "1" {
		detail := tunnelRefusal(resp)
		rt.link.recordProof(Proof{Status: ProofFailed, NodeID: owner.NodeID, EnvID: rt.envID,
			At: time.Now().UTC(), Error: detail})
		return nil, fmt.Errorf("nodelink: node %s refused the forwarded call: %s", owner.NodeID, detail)
	}
	// An agent answer is acted on as the daemon's own, so one the owning node did not sign
	// for this request is an error, never a response.
	if !rt.link.signer.VerifyReplyHead(resp.Header.Get(HeaderReplySignature), rt.link.NodeID(), owner.NodeID, nonce, resp.Header) {
		closeQuietly(resp.Body)
		err := fmt.Errorf("%w: node %s sent an agent reply that is not signed for this request", ErrBadSignature, owner.NodeID)
		rt.link.recordProof(Proof{Status: ProofFailed, NodeID: owner.NodeID, EnvID: rt.envID,
			At: time.Now().UTC(), Error: err.Error()})
		return nil, err
	}

	// The work ran there, so the audit row for this request says so.
	audit.RecordExecutor(req.Context(), executedBy(resp, owner.NodeID))
	rt.link.recordProof(Proof{Status: ProofProven, NodeID: owner.NodeID, EnvID: rt.envID, At: time.Now().UTC()})
	return rebuildResponse(req, resp)
}

// signableBody returns the body to send and the digest to sign.
//
// A body of known length small enough to hold is hashed into the signature; any other is
// streamed and signed as such, because buffering an image load to hash it would defeat the
// streaming the agent transport goes to some trouble to support. The Docker SDK sends a tar
// or build context with ContentLength 0 and a body, which is unknown length, not empty.
// A held body must be exactly its declared length: part of a body is never signed or sent.
func signableBody(req *http.Request) (io.Reader, string, error) {
	if req.Body == nil || req.Body == http.NoBody {
		return nil, emptyDigest(), nil
	}
	if req.ContentLength <= 0 || req.ContentLength > maxSignedBody {
		return req.Body, streamDigest, nil
	}
	raw, err := io.ReadAll(io.LimitReader(req.Body, maxSignedBody+1))
	closeQuietly(req.Body)
	if err != nil {
		return nil, "", fmt.Errorf("nodelink: reading the request body: %w", err)
	}
	if int64(len(raw)) != req.ContentLength {
		return nil, "", fmt.Errorf("nodelink: the request body does not match its declared length of %d bytes; refusing to sign part of it", req.ContentLength)
	}
	sum := sha256.Sum256(raw)
	return bytes.NewReader(raw), hex.EncodeToString(sum[:]), nil
}

// closeBody closes a body signableBody handed back, when the call is abandoned before sending.
func closeBody(body io.Reader) {
	if c, ok := body.(io.Closer); ok {
		closeQuietly(c)
	}
}

// executedBy is the node the reply says ran the call, falling back to the node that was
// asked when an older peer does not say.
func executedBy(resp *http.Response, asked string) string {
	if named := strings.TrimSpace(resp.Header.Get(HeaderExecuted)); named != "" {
		return named
	}
	return asked
}

// tunnelRefusal reads the short body a refusing node sends, for the error the operator
// sees. The body is bounded because a refusal is a sentence, not a stream.
func tunnelRefusal(resp *http.Response) string {
	defer closeQuietly(resp.Body)
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 2048))
	if err != nil || len(raw) == 0 {
		return resp.Status
	}
	return resp.Status + " " + strings.TrimSpace(string(raw))
}

// rebuildResponse turns the tunnel's reply back into the agent's response: the status
// and headers it carried under the prefix, and its body still unread.
func rebuildResponse(req *http.Request, resp *http.Response) (*http.Response, error) {
	status := resp.StatusCode
	if coded, err := strconv.Atoi(resp.Header.Get(HeaderStatus)); err == nil && coded > 0 {
		status = coded
	}
	inner := http.Header{}
	for name, values := range resp.Header {
		if !strings.HasPrefix(name, HeaderInnerPrefix) || len(values) == 0 {
			continue
		}
		inner.Set(strings.TrimPrefix(name, HeaderInnerPrefix), values[0])
	}
	out := &http.Response{
		Status:        strconv.Itoa(status) + " " + http.StatusText(status),
		StatusCode:    status,
		Proto:         "HTTP/1.1",
		ProtoMajor:    1,
		ProtoMinor:    1,
		Header:        inner,
		Body:          resp.Body,
		ContentLength: -1,
		Request:       req,
	}
	if length, err := strconv.ParseInt(inner.Get("Content-Length"), 10, 64); err == nil && length >= 0 {
		out.ContentLength = length
	}
	return out, nil
}

// closeQuietly closes c on a path where a close failure cannot change the outcome, logging it at debug so it is not lost.
func closeQuietly(c io.Closer) {
	if err := c.Close(); err != nil {
		slog.Debug("nodelink: close failed", "error", err)
	}
}
