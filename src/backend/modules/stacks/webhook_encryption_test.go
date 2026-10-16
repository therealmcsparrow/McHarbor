// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package stacks

import (
	"errors"
	"testing"
)

// TestWebhookSecretRefusedWithoutEncryption keeps a stack webhook's signing secret from
// being stored in plaintext by a service built without encryption.
func TestWebhookSecretRefusedWithoutEncryption(t *testing.T) {
	svc := NewService(nil, nil, t.TempDir(), nil)
	stored, err := svc.encryptWebhookSecret("s3cret")
	if !errors.Is(err, ErrWebhookEncryptionUnavailable) {
		t.Fatalf("encrypt without encryption = (%q, %v), want ErrWebhookEncryptionUnavailable", stored, err)
	}
	if _, err := svc.decryptWebhookSecret("enc:v1:AAAA"); !errors.Is(err, ErrWebhookEncryptionUnavailable) {
		t.Fatalf("decrypt of ciphertext without encryption = %v, want ErrWebhookEncryptionUnavailable", err)
	}
	if blank, err := svc.encryptWebhookSecret(""); err != nil || blank != "" {
		t.Fatalf("an empty secret needs no encryption: (%q, %v)", blank, err)
	}
}
