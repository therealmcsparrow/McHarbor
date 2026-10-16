// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package workflows

import (
	"errors"
	"io"
	"log/slog"
	"testing"
)

// TestWorkflowPayloadRefusedWithoutEncryption keeps canvas data and variables, which can
// hold credentials typed into a node, from being stored in plaintext by a service built
// without encryption.
func TestWorkflowPayloadRefusedWithoutEncryption(t *testing.T) {
	svc := NewService(nil, nil, slog.New(slog.NewTextHandler(io.Discard, nil)), nil, nil, "", nil)
	stored, err := svc.encryptWorkflowField(`{"password":"hunter2"}`, "canvas data")
	if !errors.Is(err, ErrWorkflowEncryptionUnavailable) {
		t.Fatalf("encrypt without encryption = (%q, %v), want ErrWorkflowEncryptionUnavailable", stored, err)
	}
	if _, err := svc.decryptWorkflowField("enc:v1:AAAA", "canvas data"); !errors.Is(err, ErrWorkflowEncryptionUnavailable) {
		t.Fatalf("decrypt of ciphertext without encryption = %v, want ErrWorkflowEncryptionUnavailable", err)
	}
	if plain, err := svc.decryptWorkflowField(`{"nodes":[]}`, "canvas data"); err != nil || plain != `{"nodes":[]}` {
		t.Fatalf("a legacy plaintext row must still read: (%q, %v)", plain, err)
	}
}
