// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

import (
	"errors"
	"testing"
)

// TestClusterCredentialsRefusedWithoutEncryption keeps a kubeconfig, bearer token or CA
// from being stored in plaintext when the host handed the plugin no encryption service.
func TestClusterCredentialsRefusedWithoutEncryption(t *testing.T) {
	store := &clusterStore{}
	if stored, err := store.encrypt("apiVersion: v1"); !errors.Is(err, errClusterEncryptionUnavailable) {
		t.Fatalf("encrypt without encryption = (%q, %v), want errClusterEncryptionUnavailable", stored, err)
	}
	if _, err := store.decrypt("enc:v1:AAAA"); !errors.Is(err, errClusterEncryptionUnavailable) {
		t.Fatalf("decrypt without encryption = %v, want errClusterEncryptionUnavailable", err)
	}
	if blank, err := store.encrypt(""); err != nil || blank != "" {
		t.Fatalf("an empty credential needs no encryption: (%q, %v)", blank, err)
	}
}
