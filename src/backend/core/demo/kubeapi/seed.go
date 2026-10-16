// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package kubeapi

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/binary"
	"fmt"
	"time"

	"github.com/rs/xid"
)

// demoEpoch is the timestamp baked into every demo cluster id, so the ids are
// valid xids that sort together and never collide with a real, clock-based one.
var demoEpoch = time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

// ClusterID returns the deterministic plugin_kubernetes_clusters id for a
// cluster key, so re-seeding updates the same row instead of adding another.
func ClusterID(key string) string {
	sum := sha256.Sum256([]byte("mcharbor-demo-k8s:" + key))
	var b [12]byte
	binary.BigEndian.PutUint32(b[:4], uint32(demoEpoch.Unix()))
	copy(b[4:], sum[:8])
	id, err := xid.FromBytes(b[:])
	if err != nil {
		return fmt.Sprintf("demok8s%013x", sum[:7])
	}
	return id.String()
}

// SeedDB writes one plugin_kubernetes_clusters row per simulated cluster, all
// pointing at serverURL with their own bearer token. The token is stored as
// plaintext, which the plugin's decrypt passes through; re-running updates rows.
func SeedDB(ctx context.Context, db *sql.DB, serverURL string, clusters []ClusterInfo, tokenFor func(key string) string) error {
	if db == nil {
		return fmt.Errorf("seeding demo kubernetes clusters: nil database")
	}
	if tokenFor == nil {
		return fmt.Errorf("seeding demo kubernetes clusters: nil token function")
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("seeding demo kubernetes clusters: begin: %w", err)
	}
	now := time.Now().UTC().Format(time.RFC3339)
	for i, cl := range clusters {
		token := tokenFor(cl.Key)
		if token == "" {
			if rbErr := tx.Rollback(); rbErr != nil {
				return fmt.Errorf("seeding demo kubernetes clusters: empty token for %s (rollback: %w)", cl.Key, rbErr)
			}
			return fmt.Errorf("seeding demo kubernetes clusters: empty token for %s", cl.Key)
		}
		isDefault := 0
		if i == 0 {
			isDefault = 1
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO plugin_kubernetes_clusters
			(id, name, namespace, server_url, kubeconfig, bearer_token, ca_cert, is_default, is_active, created_at, updated_at)
			VALUES (?, ?, '', ?, '', ?, '', ?, 1, ?, ?)
			ON CONFLICT(id) DO UPDATE SET
				name = excluded.name, server_url = excluded.server_url, bearer_token = excluded.bearer_token,
				is_default = excluded.is_default, is_active = 1, updated_at = excluded.updated_at`,
			ClusterID(cl.Key), cl.Name, serverURL, token, isDefault, now, now,
		); err != nil {
			if rbErr := tx.Rollback(); rbErr != nil {
				return fmt.Errorf("seeding demo kubernetes cluster %s: %w (rollback: %w)", cl.Key, err, rbErr)
			}
			return fmt.Errorf("seeding demo kubernetes cluster %s: %w", cl.Key, err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("seeding demo kubernetes clusters: commit: %w", err)
	}
	return nil
}
