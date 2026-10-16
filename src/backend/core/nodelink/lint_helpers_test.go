// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package nodelink

import (
	"io"
	"net/http"
	"testing"

	"github.com/gorilla/websocket"
)

// ignoreErr swallows the results of a teardown call whose failure carries no signal in a test.
func ignoreErr(...any) {}

// closeIgnoring closes c during teardown, where an already-closed socket is the normal case.
func closeIgnoring(c io.Closer) { ignoreErr(c.Close()) }

// dialWS dials a test websocket (nil d means the default dialer) and always releases the handshake response body.
func dialWS(d *websocket.Dialer, url string, header http.Header) (*websocket.Conn, int, error) {
	if d == nil {
		d = websocket.DefaultDialer
	}
	conn, resp, err := d.Dial(url, header)
	status := 0
	if resp != nil {
		status = resp.StatusCode
		ignoreErr(resp.Body.Close())
	}
	return conn, status, err
}

// roundTripErr issues req and returns only the error, closing any response body so the transport is not leaked.
func roundTripErr(rt http.RoundTripper, req *http.Request) error {
	resp, err := rt.RoundTrip(req)
	if resp != nil {
		ignoreErr(resp.Body.Close())
	}
	return err
}

// mustSign signs req for a test and fails it when Sign refuses.
func mustSign(t testing.TB, s *Signer, req *http.Request, c Claims, nonce, digest string) {
	t.Helper()
	if err := s.Sign(req, c, nonce, digest); err != nil {
		t.Fatalf("signing the test request: %v", err)
	}
}

// replyHead signs a test reply head; it reports rather than stops on failure, because it
// runs on the test server's goroutine.
func replyHead(t testing.TB, s *Signer, requester, responder, nonce string, h http.Header) string {
	signature, err := s.SignReplyHead(requester, responder, nonce, h)
	if err != nil {
		t.Errorf("signing the test reply head: %v", err)
	}
	return signature
}
