// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package agent

import (
	"io"

	"github.com/gorilla/websocket"
)

// ignoreErr swallows the results of a teardown call whose failure carries no signal in a test.
func ignoreErr(...any) {}

// closeIgnoring closes c during teardown, where an already-closed socket is the normal case.
func closeIgnoring(c io.Closer) { ignoreErr(c.Close()) }

// dialWS dials a test websocket and always releases the handshake response body.
func dialWS(url string) (*websocket.Conn, int, error) {
	conn, resp, err := websocket.DefaultDialer.Dial(url, nil)
	status := 0
	if resp != nil {
		status = resp.StatusCode
		ignoreErr(resp.Body.Close())
	}
	return conn, status, err
}
