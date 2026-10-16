// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// Package storageconfig decodes the per-location `config` column that
// storage_locations writes and container_backups reads at upload time.
// Both modules need the same decoder; hosting it here keeps
// container_backups independent of the storage_locations module.
package storageconfig

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/therealmcsparrow/mcharbor/core/encryption"
)

// Decode parses a storage location's config column. The column holds
// either a JSON object (legacy plaintext rows) or an encrypted JSON
// object; anything that does not start with "{" is decrypted first.
// Empty and "{}" decode to nil.
func Decode(enc *encryption.Service, raw string) (map[string]string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" || raw == "{}" {
		return nil, nil
	}
	if !strings.HasPrefix(raw, "{") {
		if enc == nil {
			return nil, fmt.Errorf("storage credential encryption service is not configured")
		}
		decrypted, err := enc.Decrypt(raw)
		if err != nil {
			return nil, fmt.Errorf("decrypting storage location config: %w", err)
		}
		raw = decrypted
	}
	var cfg map[string]string
	if err := json.Unmarshal([]byte(raw), &cfg); err != nil {
		return nil, fmt.Errorf("decoding storage location config: %w", err)
	}
	if len(cfg) == 0 {
		return nil, nil
	}
	return cfg, nil
}
