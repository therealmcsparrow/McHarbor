// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package audit

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
)

// HashRow contains the stable audit row fields covered by row_hash.
//
// Adding a field invalidates every hash already written, so audit_logs columns
// that are denormalised copies of a hashed value — environment_name, derived
// from the hashed environment_id — stay out of it deliberately.
type HashRow struct {
	ID            string  `json:"id"`
	PrevHash      string  `json:"prevHash"`
	UserID        *string `json:"userId,omitempty"`
	Username      *string `json:"username,omitempty"`
	Action        string  `json:"action"`
	Severity      string  `json:"severity"`
	EntityType    *string `json:"entityType,omitempty"`
	EntityID      *string `json:"entityId,omitempty"`
	EntityName    *string `json:"entityName,omitempty"`
	Details       *string `json:"details,omitempty"`
	StateBefore   *string `json:"stateBefore,omitempty"`
	StateAfter    *string `json:"stateAfter,omitempty"`
	ChangedKeys   *string `json:"changedKeys,omitempty"`
	IPAddress     *string `json:"ipAddress,omitempty"`
	EnvironmentID *string `json:"environmentId,omitempty"`
	// APIKeyID is omitted when nil, so rows written before it existed hash to
	// exactly the bytes they were sealed with; the key's name stays out as a
	// denormalised copy, like environment_name.
	APIKeyID  *string `json:"apiKeyId,omitempty"`
	Timestamp string  `json:"timestamp"`
	CreatedAt string  `json:"createdAt"`
	UpdatedAt string  `json:"updatedAt"`
}

// ComputeRowHash returns the HMAC-SHA256 digest for an audit row.
func ComputeRowHash(secret string, row HashRow) (string, error) {
	payload, err := json.Marshal(row)
	if err != nil {
		return "", fmt.Errorf("marshal audit hash row: %w", err)
	}
	mac := hmac.New(sha256.New, []byte(secret))
	if _, err := mac.Write(payload); err != nil {
		return "", fmt.Errorf("hash audit row: %w", err)
	}
	return hex.EncodeToString(mac.Sum(nil)), nil
}
