// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

import (
	"context"
	"fmt"
	"strings"
)

// ciphertextPrefix marks a value the host encryption service wrote. The plugin checks
// it itself rather than importing the host's encryption package.
const ciphertextPrefix = "enc:v1:"

// clusterCredentialColumns are the credential columns the store encrypts on write.
var clusterCredentialColumns = []string{"kubeconfig", "bearer_token", "ca_cert"}

// plaintextCredential is one credential still stored as plaintext.
type plaintextCredential struct{ id, value string }

// pendingPlaintext reads one column's plaintext rows and releases the cursor before
// returning, so the caller can write on a single-connection database.
func (s *clusterStore) pendingPlaintext(ctx context.Context, column string) ([]plaintextCredential, error) {
	rows, err := s.db.Query(ctx, `SELECT id, `+column+` FROM plugin_kubernetes_clusters
		WHERE `+column+` <> '' AND `+column+` NOT LIKE 'enc:v1:%' ORDER BY id LIMIT 1000`) // SQL identifier: column is one of clusterCredentialColumns
	if err != nil {
		return nil, fmt.Errorf("selecting plaintext %s: %w", column, err)
	}
	var out []plaintextCredential
	for rows.Next() {
		var row plaintextCredential
		if err := rows.Scan(&row.id, &row.value); err != nil {
			_ = rows.Close()
			return nil, fmt.Errorf("scanning plaintext %s: %w", column, err)
		}
		out = append(out, row)
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return nil, fmt.Errorf("reading plaintext %s: %w", column, err)
	}
	if err := rows.Close(); err != nil {
		return nil, fmt.Errorf("closing plaintext %s: %w", column, err)
	}
	return out, nil
}

// backfillEncryption encrypts every cluster credential stored before the store
// encrypted on write. Each write is conditional on the value read, so a credential
// saved in between is never overwritten; a second run finds nothing to do.
func (s *clusterStore) backfillEncryption(ctx context.Context) (map[string]int, error) {
	if s.enc == nil {
		return nil, errClusterEncryptionUnavailable
	}
	encrypted := map[string]int{}
	for _, column := range clusterCredentialColumns {
		pending, err := s.pendingPlaintext(ctx, column)
		if err != nil {
			return encrypted, err
		}
		for _, row := range pending {
			if strings.HasPrefix(row.value, ciphertextPrefix) {
				continue
			}
			cipher, err := s.enc.EncryptString(row.value)
			if err != nil {
				return encrypted, fmt.Errorf("encrypting %s of cluster %s: %w", column, row.id, err)
			}
			n, err := s.db.Exec(ctx, `UPDATE plugin_kubernetes_clusters SET `+column+` = ? WHERE id = ? AND `+column+` = ?`, // SQL identifier: column is one of clusterCredentialColumns
				cipher, row.id, row.value)
			if err != nil {
				return encrypted, fmt.Errorf("encrypting %s of cluster %s: %w", column, row.id, err)
			}
			encrypted[column] += int(n)
		}
	}
	return encrypted, nil
}
