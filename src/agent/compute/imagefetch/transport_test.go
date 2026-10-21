// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package imagefetch

import (
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
)

// brokenGzipMirror behaves like the AlmaLinux CDN edge that failed in the field: a
// client that offers gzip gets the image gzipped on the fly with a corrupt CRC
// trailer, and a client that does not gets the plain bytes with their length.
func brokenGzipMirror(t *testing.T, image []byte) *httptest.Server {
	t.Helper()
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.Header.Get("Accept-Encoding"), "gzip") {
			w.Header().Set("Content-Length", strconv.Itoa(len(image)))
			if _, err := w.Write(image); err != nil {
				t.Error(err)
			}
			return
		}
		var buf bytes.Buffer
		zw := gzip.NewWriter(&buf)
		if _, err := zw.Write(image); err != nil {
			t.Fatal(err)
		}
		if err := zw.Close(); err != nil {
			t.Fatal(err)
		}
		stream := buf.Bytes()
		// The trailer is CRC-32 then size; flipping a CRC byte is exactly the
		// "gzip: invalid checksum" the reader reports at EOF.
		stream[len(stream)-8] ^= 0xff
		w.Header().Set("Content-Encoding", "gzip")
		if _, err := w.Write(stream); err != nil {
			t.Error(err)
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

// TestFetchIgnoresAMirrorsOnTheFlyGzip: the fetcher must ask for identity bytes, so a
// CDN that gzips a disk image (badly) on request never gets the chance to.
func TestFetchIgnoresAMirrorsOnTheFlyGzip(t *testing.T) {
	image := bytes.Repeat([]byte("pretend qcow2 bytes "), 4096)
	srv := brokenGzipMirror(t, image)

	f, err := New(t.TempDir(), nil)
	if err != nil {
		t.Fatal(err)
	}
	// Trust the test server's certificate but keep the transport newMirrorClient
	// builds, since the transport's settings are what is under test; loopback is
	// opened only because the test server listens there.
	f.client = newMirrorClient(true)
	transport, ok := f.client.Transport.(*http.Transport)
	if !ok {
		t.Fatalf("fetcher transport is %T, want *http.Transport", f.client.Transport)
	}
	transport.TLSClientConfig = srv.Client().Transport.(*http.Transport).TLSClientConfig

	var total int64
	res, err := f.Fetch(context.Background(), Request{
		ImageID:   "img-gzip-edge",
		Kind:      "cloud_image",
		SourceURL: srv.URL + "/AlmaLinux-9-GenericCloud-latest.x86_64.qcow2",
		Checksum:  fmt.Sprintf("%x", sha256.Sum256(image)),
	}, func(p Progress) {
		if p.BytesTotal > total {
			total = p.BytesTotal
		}
	})
	if err != nil {
		t.Fatalf("fetch through an on-the-fly gzip edge: %v", err)
	}
	if !res.Verified {
		t.Fatalf("result = %+v, want verified", res)
	}
	if total != int64(len(image)) {
		t.Errorf("progress total = %d, want the real size %d", total, len(image))
	}
}
