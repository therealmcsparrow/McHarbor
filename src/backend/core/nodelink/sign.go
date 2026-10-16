// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package nodelink

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"golang.org/x/net/http/httpguts"
)

// Header names the node link adds to a forwarded request. They are namespaced so a
// reverse proxy that passes unknown headers through cannot collide with them.
const (
	HeaderNode       = "X-Mcharbor-Node"
	HeaderTimestamp  = "X-Mcharbor-Node-Timestamp"
	HeaderNonce      = "X-Mcharbor-Node-Nonce"
	HeaderSignature  = "X-Mcharbor-Node-Signature"
	HeaderMethod     = "X-Mcharbor-Node-Method"
	HeaderPath       = "X-Mcharbor-Node-Path"
	HeaderQuery      = "X-Mcharbor-Node-Query"
	HeaderTimeoutMS  = "X-Mcharbor-Node-Timeout-Ms"
	HeaderActorID    = "X-Mcharbor-Node-Actor-Id"
	HeaderActorName  = "X-Mcharbor-Node-Actor-Name"
	HeaderBodyDigest = "X-Mcharbor-Node-Body-Digest"
	// HeaderAudience names the one node a signed call is addressed to, so a captured
	// call cannot be replayed against any other node of the cluster.
	HeaderAudience    = "X-Mcharbor-Node-Audience"
	HeaderInnerPrefix = "X-Mcharbor-Node-H-"
	// HeaderStatus and HeaderExecuted are set on the reply: the agent's own status
	// code, and the node that actually ran the call, which the audit row records.
	HeaderStatus   = "X-Mcharbor-Node-Status"
	HeaderExecuted = "X-Mcharbor-Node-Executed"
	// HeaderRPC marks a reply that carries an agent response rather than a
	// tunnel-level refusal, so a 401 from the signature check is never mistaken
	// for a 401 from the Docker daemon.
	HeaderRPC = "X-Mcharbor-Node-Rpc"
	// HeaderReplySignature authenticates a reply whose content the asking node acts on
	// (a trigger fire's result, a leader probe), so a forged 200 is never taken as delivery.
	HeaderReplySignature = "X-Mcharbor-Node-Reply-Signature"
)

// signatureSkew is how far apart two nodes' clocks may be before a forwarded call is
// refused. Wide enough for NTP jitter, narrow enough that a captured request is dead
// long before anyone could act on it.
const signatureSkew = 2 * time.Minute

// streamDigest is the body digest of a request whose body is streamed and therefore
// cannot be hashed before it is sent.
const streamDigest = "stream"

// maxSignedBody is the largest body hashed into the signature. Above it the body is
// streamed and its integrity rests on TLS alone, which is why every multi-node deployment
// requires https between the nodes unless the operator opts out (plaintext.go).
const maxSignedBody = 1 << 20

// nonceSweepEvery is how often the nonce store forgets nonces older than the replay window.
const nonceSweepEvery = signatureSkew

// ErrNotSigned means the request carried no node signature at all, which is what an
// ordinary client request looks like to this middleware.
var ErrNotSigned = errors.New("nodelink: request is not signed by a node")

// ErrBadSignature means the request was signed and the signature did not verify: a
// wrong MCHARBOR_SECRET, a replay, a clock too far out, or a call addressed to another node.
var ErrBadSignature = errors.New("nodelink: node signature is not valid")

// ErrUnsignable means a claim or forwarded header holds a value HTTP cannot carry intact
// (a control character, CR or LF), so it is refused at signing instead of failing closed
// at the receiver after the peer's server or client rewrote it.
var ErrUnsignable = errors.New("nodelink: a header value cannot be signed")

// canonicalVersion labels the signed string. v2 added the audience; v3 the timeout, the actor
// name and the inner headers; v4 the hop and whitespace-trimmed values. An older signature
// never verifies, which is the fail-closed behaviour a mixed-build cluster gets instead of
// accepting unsigned fields.
const canonicalVersion = "mcharbor-node-link-v4"

// replyVersion labels a signed reply, distinct from every request so neither can stand in
// for the other.
const replyVersion = "mcharbor-node-link-reply-v1"

// replyHeadVersion labels a signed streamed reply (an agent-rpc answer), whose signature
// covers the status and headers but not the body, which streams under TLS.
const replyHeadVersion = "mcharbor-node-link-reply-head-v1"

// Claims is what a verified forwarded request asserts about itself.
type Claims struct {
	// Hop is the route the call is signed for (HopAgentRPC, HopStats, ...). The receiver
	// supplies its own and never reads it from the headers, so a signature for one hop is
	// worthless on every other.
	Hop string
	// NodeID is the node the signer says it is. Anyone holding the shared secret can name any
	// id, so it identifies a node only as far as admitVerified's live-registry check does.
	NodeID string
	// Audience is the node the call is addressed to; the receiver refuses any other.
	Audience  string
	EnvID     string
	Method    string
	Path      string
	Query     string
	ActorID   string
	ActorName string
	Timeout   time.Duration
}

// Signer signs and verifies node-to-node calls with a key derived from the
// deployment's shared secret, which every node of a cluster already holds.
type Signer struct {
	key []byte

	// seen remembers recently accepted nonces so a captured request cannot be
	// replayed inside the clock-skew window. It is per node and per process,
	// which is enough: a replay only does damage where it is accepted. Only a fully
	// verified request adds to it, and StartNonceSweep empties it of expired entries.
	mu   sync.Mutex
	seen map[string]time.Time
}

// NewSigner derives the link key from secret, or returns nil when no shared secret is
// configured — the link then refuses to forward rather than signing with nothing.
func NewSigner(secret string) *Signer {
	secret = strings.TrimSpace(secret)
	if secret == "" {
		return nil
	}
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte("mcharbor-node-link-v1"))
	return &Signer{key: mac.Sum(nil), seen: map[string]time.Time{}}
}

// canonical is the string both sides sign. Every field that decides what the receiving
// node will do is in it, so none of them can be rewritten in flight: timeoutMS is the raw
// timeout header and inner the digest of the forwarded request's own headers. Every value is
// trimmed, because the receiver's header parser trims what the sender set.
func canonical(c Claims, timestamp, nonce, digest, timeoutMS, inner string) string {
	fields := []string{
		c.Hop, c.NodeID, c.Audience, c.EnvID, c.Method, c.Path, c.Query,
		timestamp, nonce, c.ActorID, c.ActorName, timeoutMS, digest, inner,
	}
	for i := range fields {
		fields[i] = strings.TrimSpace(fields[i])
	}
	return canonicalVersion + "\n" + strings.Join(fields, "\n")
}

// innerHeaderDigest hashes every HeaderInnerPrefix header, name lowercased, first value
// only (the only one the receiver applies) and trimmed, sorted, so Content-Length and
// X-Registry-Auth are covered and a header added, dropped or rewritten in flight breaks it.
func innerHeaderDigest(h http.Header) string {
	lines := make([]string, 0, len(h))
	for name, values := range h {
		if len(values) == 0 || !strings.HasPrefix(http.CanonicalHeaderKey(name), HeaderInnerPrefix) {
			continue
		}
		lines = append(lines, strings.ToLower(name)+":"+strings.TrimSpace(values[0]))
	}
	sort.Strings(lines)
	sum := sha256.Sum256([]byte(strings.Join(lines, "\n")))
	return hex.EncodeToString(sum[:])
}

// emptyDigest is the body digest of a call that carries no body.
func emptyDigest() string {
	sum := sha256.Sum256(nil)
	return hex.EncodeToString(sum[:])
}

// checkSignable refuses a header set holding a name or value HTTP cannot carry intact. Go's
// client refuses such a request and its server rewrites CR/LF in a reply to spaces, so a
// signature over the raw value could never verify; saying so here names the header.
func checkSignable(h http.Header, onlyPrefix string) error {
	for name, values := range h {
		if onlyPrefix != "" && !strings.HasPrefix(http.CanonicalHeaderKey(name), onlyPrefix) {
			continue
		}
		if !httpguts.ValidHeaderFieldName(name) {
			return fmt.Errorf("%w: header name %q", ErrUnsignable, clipLogged(name))
		}
		for _, v := range values {
			if !httpguts.ValidHeaderFieldValue(v) {
				return fmt.Errorf("%w: header %s carries a control character", ErrUnsignable, name)
			}
		}
	}
	return nil
}

func (s *Signer) mac(payload string) string {
	mac := hmac.New(sha256.New, s.key)
	mac.Write([]byte(payload))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

// Sign stamps req with the claims and their signature; digest is the body digest, or
// streamDigest when the body is too large to hash first. The inner headers must already be
// set on req, because the signature covers them. It refuses, with ErrUnsignable, a claim
// without a hop or any node-link header value HTTP cannot carry, and then leaves req unsigned.
func (s *Signer) Sign(req *http.Request, c Claims, nonce, digest string) error {
	req.Header.Del(HeaderSignature)
	if strings.TrimSpace(c.Hop) == "" {
		return fmt.Errorf("%w: the claims name no hop", ErrUnsignable)
	}
	timestamp := strconv.FormatInt(time.Now().UTC().Unix(), 10)
	req.Header.Set(HeaderNode, c.NodeID)
	req.Header.Set(HeaderAudience, c.Audience)
	req.Header.Set(HeaderMethod, c.Method)
	req.Header.Set(HeaderPath, c.Path)
	req.Header.Set(HeaderQuery, c.Query)
	req.Header.Set(HeaderActorID, c.ActorID)
	req.Header.Set(HeaderActorName, c.ActorName)
	req.Header.Set(HeaderTimestamp, timestamp)
	req.Header.Set(HeaderNonce, nonce)
	req.Header.Set(HeaderBodyDigest, digest)
	timeoutMS := ""
	if c.Timeout > 0 {
		timeoutMS = strconv.FormatInt(c.Timeout.Milliseconds(), 10)
		req.Header.Set(HeaderTimeoutMS, timeoutMS)
	} else {
		req.Header.Del(HeaderTimeoutMS)
	}
	if err := checkSignable(req.Header, HeaderNode); err != nil {
		return err
	}
	req.Header.Set(HeaderSignature, s.mac(canonical(c, timestamp, nonce, digest, timeoutMS, innerHeaderDigest(req.Header))))
	return nil
}

// precheck refuses, from headers alone and without any key, a request that cannot verify:
// unsigned, without a nonce, stamped outside the skew window, or addressed to another node.
// It is a cheap filter, not authentication; Authenticate is.
func precheck(req *http.Request, audience string) error {
	if req.Header.Get(HeaderSignature) == "" {
		return ErrNotSigned
	}
	if req.Header.Get(HeaderNonce) == "" {
		return fmt.Errorf("%w: nonce is missing", ErrBadSignature)
	}
	unix, err := strconv.ParseInt(strings.TrimSpace(req.Header.Get(HeaderTimestamp)), 10, 64)
	if err != nil {
		return fmt.Errorf("%w: timestamp is missing or malformed", ErrBadSignature)
	}
	age := time.Since(time.Unix(unix, 0))
	if age < -signatureSkew || age > signatureSkew {
		return fmt.Errorf("%w: timestamp is %s out", ErrBadSignature, age.Round(time.Second))
	}
	if audience == "" || strings.TrimSpace(req.Header.Get(HeaderAudience)) != audience {
		return fmt.Errorf("%w: addressed to another node", ErrBadSignature)
	}
	return nil
}

// Authenticate verifies the HMAC over the headers and the body digest the request claims,
// in constant time, reading no body byte, and returns the claims. hop and envID come from
// the route and audience is this node's own id, never the headers. It does not spend the
// nonce or check the body against the digest; Accept does both once the body has been read.
func (s *Signer) Authenticate(req *http.Request, hop, envID, audience string) (Claims, error) {
	if err := precheck(req, audience); err != nil {
		return Claims{}, err
	}
	if hop == "" {
		return Claims{}, fmt.Errorf("%w: the receiving route names no hop", ErrBadSignature)
	}
	header := func(name string) string { return strings.TrimSpace(req.Header.Get(name)) }
	c := Claims{
		Hop:       hop,
		NodeID:    header(HeaderNode),
		Audience:  audience,
		EnvID:     envID,
		Method:    header(HeaderMethod),
		Path:      header(HeaderPath),
		Query:     header(HeaderQuery),
		ActorID:   header(HeaderActorID),
		ActorName: header(HeaderActorName),
	}
	timeoutMS := header(HeaderTimeoutMS)
	if ms, err := strconv.ParseInt(timeoutMS, 10, 64); err == nil && ms > 0 {
		c.Timeout = time.Duration(ms) * time.Millisecond
	}
	want := s.mac(canonical(c, req.Header.Get(HeaderTimestamp), req.Header.Get(HeaderNonce),
		req.Header.Get(HeaderBodyDigest), timeoutMS, innerHeaderDigest(req.Header)))
	if subtle.ConstantTimeCompare([]byte(req.Header.Get(HeaderSignature)), []byte(want)) != 1 {
		return Claims{}, ErrBadSignature
	}
	return c, nil
}

// Accept finishes an authenticated request once its body is read: the body must hash to
// the signed digest (nil hashes as an empty body; only a streamed body is not hashed), and
// only then is the nonce spent, so a body damaged in flight does not burn the request's nonce.
func (s *Signer) Accept(req *http.Request, body []byte) error {
	digest := strings.TrimSpace(req.Header.Get(HeaderBodyDigest))
	if digest != streamDigest {
		sum := sha256.Sum256(body)
		if subtle.ConstantTimeCompare([]byte(digest), []byte(hex.EncodeToString(sum[:]))) != 1 {
			return fmt.Errorf("%w: body digest does not match", ErrBadSignature)
		}
	}
	if !s.firstUse(strings.TrimSpace(req.Header.Get(HeaderNonce))) {
		return fmt.Errorf("%w: nonce has already been used", ErrBadSignature)
	}
	return nil
}

// AcceptEmpty is Accept for a hop that carries no body (stats, stream, leader probe): the
// signed digest must be exactly that of an empty body, so a streamed digest is refused too.
func (s *Signer) AcceptEmpty(req *http.Request) error {
	if subtle.ConstantTimeCompare([]byte(strings.TrimSpace(req.Header.Get(HeaderBodyDigest))), []byte(emptyDigest())) != 1 {
		return fmt.Errorf("%w: a body-less call must sign the empty body digest", ErrBadSignature)
	}
	return s.Accept(req, nil)
}

// Verify is Authenticate followed by Accept, for a caller that already holds the body (or
// has none): it returns the claims only when the whole request verified.
func (s *Signer) Verify(req *http.Request, hop, envID, audience string, body []byte) (Claims, error) {
	c, err := s.Authenticate(req, hop, envID, audience)
	if err != nil {
		return Claims{}, err
	}
	if err := s.Accept(req, body); err != nil {
		return Claims{}, err
	}
	return c, nil
}

// firstUse reports whether nonce has not been accepted before and records it. It does not
// walk the store; the sweep goroutine forgets expired nonces off the request path.
func (s *Signer) firstUse(nonce string) bool {
	if nonce == "" {
		return false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, used := s.seen[nonce]; used {
		return false
	}
	s.seen[nonce] = time.Now()
	return true
}

// sweepNonces forgets nonces older than twice the skew window, past which a replay is
// refused by its timestamp anyway.
func (s *Signer) sweepNonces(now time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for value, at := range s.seen {
		if now.Sub(at) > 2*signatureSkew {
			delete(s.seen, value)
		}
	}
}

// StartNonceSweep forgets expired nonces every nonceSweepEvery until ctx ends, so the
// replay store stays bounded without a request ever paying for the walk.
func (s *Signer) StartNonceSweep(ctx context.Context) {
	s.startNonceSweep(ctx, nonceSweepEvery)
}

func (s *Signer) startNonceSweep(ctx context.Context, every time.Duration) {
	if s == nil {
		return
	}
	go func() { // goroutine lifecycle: ends when ctx is cancelled
		ticker := time.NewTicker(every)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case now := <-ticker.C:
				s.sweepNonces(now)
			}
		}
	}()
}

// replyCanonical is the string a signed reply covers: which request it answers (the asking
// node, the answering node and the request nonce) and the exact reply body.
func replyCanonical(requester, responder, nonce string, body []byte) string {
	sum := sha256.Sum256(body)
	return strings.Join([]string{replyVersion, requester, responder, nonce, hex.EncodeToString(sum[:])}, "\n")
}

// SignReply signs body as responder's answer to requester's request carrying nonce.
func (s *Signer) SignReply(requester, responder, nonce string, body []byte) string {
	return s.mac(replyCanonical(requester, responder, nonce, body))
}

// VerifyReply reports whether signature is responder's signature over body in answer to
// the request this node (requester) sent with nonce; a reply to any other request fails.
func (s *Signer) VerifyReply(signature, requester, responder, nonce string, body []byte) bool {
	if signature == "" || nonce == "" {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(signature), []byte(s.mac(replyCanonical(requester, responder, nonce, body)))) == 1
}

// replyHeadCanonical is the string a signed streamed reply covers: the request it answers,
// the agent's status, the executing node, the RPC marker and every forwarded response
// header. The body is not in it; it streams, so its integrity rests on TLS. Values are
// trimmed, as the asking node's client trims what this node's server sends.
func replyHeadCanonical(requester, responder, nonce string, h http.Header) string {
	return strings.Join([]string{replyHeadVersion, requester, responder, nonce,
		strings.TrimSpace(h.Get(HeaderStatus)), strings.TrimSpace(h.Get(HeaderExecuted)), strings.TrimSpace(h.Get(HeaderRPC)),
		innerHeaderDigest(h), streamDigest}, "\n")
}

// SignReplyHead signs the status and headers of responder's streamed answer to requester's
// request carrying nonce; h must already hold every header the reply sends. It refuses, with
// ErrUnsignable, a reply header HTTP cannot carry, since Go's server would rewrite it in flight.
func (s *Signer) SignReplyHead(requester, responder, nonce string, h http.Header) (string, error) {
	if err := checkSignable(h, ""); err != nil {
		return "", err
	}
	return s.mac(replyHeadCanonical(requester, responder, nonce, h)), nil
}

// VerifyReplyHead reports whether signature is responder's signature over the status and
// headers h of its answer to the request this node (requester) sent with nonce.
func (s *Signer) VerifyReplyHead(signature, requester, responder, nonce string, h http.Header) bool {
	if signature == "" || nonce == "" {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(signature), []byte(s.mac(replyHeadCanonical(requester, responder, nonce, h)))) == 1
}
