// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package storage_locations

import (
	"bytes"
	"errors"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/therealmcsparrow/mcharbor/core/backupcrypto"
)

func testCrypto(t *testing.T) *backupcrypto.Service {
	t.Helper()
	svc, err := backupcrypto.NewFromKeyMaterial(strings.Repeat("ab", 32))
	if err != nil {
		t.Fatal(err)
	}
	return svc
}

// waitWithin runs wait on its own goroutine so a regression fails the test
// instead of hanging the whole package run.
func waitWithin(t *testing.T, wait func() error) error {
	t.Helper()
	done := make(chan error, 1)
	go func() { done <- wait() }()
	select {
	case err := <-done:
		return err
	case <-time.After(5 * time.Second):
		t.Fatal("wait did not return: an abandoned stream still holds the upload open")
		return nil
	}
}

func TestAnAbandonedEncryptedStreamDoesNotHang(t *testing.T) {
	src := bytes.NewReader(bytes.Repeat([]byte("x"), 4<<20))
	_, wait := encryptStream(testCrypto(t), src)
	if err := waitWithin(t, wait); !errors.Is(err, errStreamAbandoned) {
		t.Fatalf("an adapter that read nothing must surface errStreamAbandoned, got %v", err)
	}
}

func TestADrainedEncryptedStreamRoundTrips(t *testing.T) {
	crypto := testCrypto(t)
	payload := bytes.Repeat([]byte("guest-bytes "), 50_000)
	reader, wait := encryptStream(crypto, bytes.NewReader(payload))
	sealed, err := io.ReadAll(reader)
	if err != nil {
		t.Fatal(err)
	}
	if err := waitWithin(t, wait); err != nil {
		t.Fatalf("a fully read stream must finish cleanly, got %v", err)
	}
	plain, _, err := crypto.DecryptReader(bytes.NewReader(sealed))
	if err != nil {
		t.Fatal(err)
	}
	got, err := io.ReadAll(plain)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, payload) {
		t.Fatalf("round trip lost data: %d bytes in, %d out", len(payload), len(got))
	}
}
