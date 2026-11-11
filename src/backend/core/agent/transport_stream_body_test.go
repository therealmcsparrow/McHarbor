// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package agent

import (
	"bytes"
	"io"
	"net/http"
	"testing"
)

func TestShouldStreamRequestBody(t *testing.T) {
	pipeReader, pipeWriter := io.Pipe()
	t.Cleanup(func() {
		if err := pipeWriter.Close(); err != nil {
			t.Errorf("closing pipe writer: %v", err)
		}
		if err := pipeReader.Close(); err != nil {
			t.Errorf("closing pipe reader: %v", err)
		}
	})

	pipeReq, err := http.NewRequest(http.MethodPut, "http://docker/containers/x/archive", pipeReader)
	if err != nil {
		t.Fatalf("building pipe request: %v", err)
	}
	if pipeReq.ContentLength != 0 {
		t.Fatalf("precondition: pipe request ContentLength = %d, want 0", pipeReq.ContentLength)
	}

	withLength := func(n int64) *http.Request {
		req, err := http.NewRequest(http.MethodPost, "http://docker/build", bytes.NewReader(make([]byte, 1)))
		if err != nil {
			t.Fatalf("building request: %v", err)
		}
		req.ContentLength = n
		return req
	}
	noBody, err := http.NewRequest(http.MethodPost, "http://docker/containers/x/start", http.NoBody)
	if err != nil {
		t.Fatalf("building NoBody request: %v", err)
	}
	nilBody, err := http.NewRequest(http.MethodGet, "http://docker/containers/json", nil)
	if err != nil {
		t.Fatalf("building nil-body request: %v", err)
	}

	cases := []struct {
		name string
		req  *http.Request
		want bool
	}{
		{"pipe body reporting zero length streams", pipeReq, true},
		{"explicit NoBody stays buffered", noBody, false},
		{"nil body stays buffered", nilBody, false},
		{"small known length stays buffered", withLength(512), false},
		{"exactly the buffered limit stays buffered", withLength(maxBufferedAgentRequestBytes), false},
		{"one byte over the buffered limit streams", withLength(maxBufferedAgentRequestBytes + 1), true},
		{"unknown length streams", withLength(-1), true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := shouldStreamRequestBody(tc.req); got != tc.want {
				t.Fatalf("shouldStreamRequestBody = %v, want %v (ContentLength %d)", got, tc.want, tc.req.ContentLength)
			}
		})
	}
}
