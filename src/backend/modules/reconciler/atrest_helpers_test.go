// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package reconciler

import (
	"testing"

	"github.com/therealmcsparrow/mcharbor/core/encryption"
)

// testEncryption returns a real encryption service: desired states refuse to store a
// config or spec without one.
func testEncryption(t *testing.T) *encryption.Service {
	t.Helper()
	enc, err := encryption.New(t.TempDir(), "")
	if err != nil {
		t.Fatalf("encryption: %v", err)
	}
	return enc
}
