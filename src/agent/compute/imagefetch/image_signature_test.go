// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package imagefetch

import (
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/ProtonMail/go-crypto/openpgp"
	"github.com/ProtonMail/go-crypto/openpgp/armor"
)

// signingMirror serves an image, its checksum and a detached armored signature
// over the image made by a throwaway key; tamper swaps the image after signing.
func signingMirror(t *testing.T, tamper bool) (*httptest.Server, Keyring) {
	t.Helper()
	entity, err := openpgp.NewEntity("test", "", "test@example.invalid", nil)
	if err != nil {
		t.Fatal(err)
	}
	image := []byte("pretend qcow2 bytes")
	var sig bytes.Buffer
	if err := openpgp.ArmoredDetachSign(&sig, entity, bytes.NewReader(image), nil); err != nil {
		t.Fatal(err)
	}
	served := image
	if tamper {
		served = []byte("pretend qcow2 bytes, rewritten on the mirror")
	}
	sum := fmt.Sprintf("%x", sha256.Sum256(served))

	var pub bytes.Buffer
	w, err := armor.Encode(&pub, openpgp.PublicKeyType, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := entity.Serialize(w); err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	ring, err := NewKeyring(pub.Bytes())
	if err != nil {
		t.Fatal(err)
	}

	serve := func(body []byte) http.HandlerFunc {
		return func(w http.ResponseWriter, _ *http.Request) {
			if _, err := w.Write(body); err != nil {
				t.Error(err)
			}
		}
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/img.qcow2", serve(served))
	mux.HandleFunc("/img.qcow2.asc", serve(sig.Bytes()))
	mux.HandleFunc("/SHA256SUMS", serve(fmt.Appendf(nil, "%s *img.qcow2\n", sum)))
	srv := httptest.NewTLSServer(mux)
	t.Cleanup(srv.Close)
	return srv, ring
}

func fetchSigned(t *testing.T, tamper bool) (res Result, target string, err error) {
	t.Helper()
	srv, ring := signingMirror(t, tamper)
	f, err := New(t.TempDir(), ring)
	if err != nil {
		t.Fatal(err)
	}
	f.client = srv.Client()
	req := Request{
		ImageID:      "img-test",
		Kind:         "cloud_image",
		SourceURL:    srv.URL + "/img.qcow2",
		ChecksumURL:  srv.URL + "/SHA256SUMS",
		SignatureURL: srv.URL + "/img.qcow2.asc",
	}
	target, err = f.imagePath(req)
	if err != nil {
		t.Fatal(err)
	}
	res, err = f.Fetch(context.Background(), req, nil)
	return res, target, err
}

// TestFetchVerifiesASignatureOverTheImage: the Alpine shape. The checksum alone
// would pass a rewritten image whose checksum file was rewritten with it.
func TestFetchVerifiesASignatureOverTheImage(t *testing.T) {
	res, _, err := fetchSigned(t, false)
	if err != nil {
		t.Fatalf("a correctly signed image was refused: %v", err)
	}
	if !res.Signed || !res.Verified {
		t.Fatalf("result = %+v, want verified and signed", res)
	}

	_, target, err := fetchSigned(t, true)
	if !errors.Is(err, ErrSignature) {
		t.Fatalf("a rewritten image: err = %v, want ErrSignature", err)
	}
	if _, statErr := os.Stat(target); !os.IsNotExist(statErr) {
		t.Fatalf("the rejected image was installed at %s", target)
	}
}
