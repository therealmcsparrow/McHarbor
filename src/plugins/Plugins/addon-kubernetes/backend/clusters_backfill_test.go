// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

import (
	"context"
	"strings"
	"testing"
)

// racingDB lets a concurrent writer change a row between the backfill's read and its
// conditional write.
type racingDB struct {
	sqliteDB
	race func()
}

func (r *racingDB) Exec(ctx context.Context, query string, args ...any) (int64, error) {
	if r.race != nil && strings.HasPrefix(strings.TrimSpace(query), "UPDATE plugin_kubernetes_clusters SET kubeconfig") {
		race := r.race
		r.race = nil
		race()
	}
	return r.sqliteDB.Exec(ctx, query, args...)
}

func insertCluster(t *testing.T, store *clusterStore, id, kubeconfig, token, ca string) {
	t.Helper()
	if _, err := store.db.Exec(context.Background(), `INSERT INTO plugin_kubernetes_clusters
		(id, name, kubeconfig, bearer_token, ca_cert, created_at, updated_at) VALUES (?, ?, ?, ?, ?, '2026-01-01', '2026-01-01')`,
		id, id, kubeconfig, token, ca); err != nil {
		t.Fatal(err)
	}
}

func storedCredentials(t *testing.T, store *clusterStore, id string) (kubeconfig, token, ca string) {
	t.Helper()
	rows, err := store.db.Query(context.Background(), `SELECT kubeconfig, bearer_token, ca_cert FROM plugin_kubernetes_clusters WHERE id = ?`, id)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = rows.Close() }()
	if !rows.Next() {
		t.Fatalf("cluster %s not found", id)
	}
	if err := rows.Scan(&kubeconfig, &token, &ca); err != nil {
		t.Fatal(err)
	}
	return kubeconfig, token, ca
}

// TestClusterCredentialBackfill seals plaintext credentials, leaves ciphertext and empty
// values alone, is a no-op on the second run, and never overwrites a value a concurrent
// writer changed after the backfill read it.
func TestClusterCredentialBackfill(t *testing.T) {
	store := legacyStore(t)
	if err := store.ensureSchema(context.Background()); err != nil {
		t.Fatal(err)
	}
	insertCluster(t, store, "plain", "apiVersion: v1", "tok-plain", "CA-PLAIN")
	insertCluster(t, store, "sealed", "enc:v1:apiVersion: v1", "enc:v1:tok", "")
	insertCluster(t, store, "empty", "", "", "")

	got, err := store.backfillEncryption(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if got["kubeconfig"] != 1 || got["bearer_token"] != 1 || got["ca_cert"] != 1 {
		t.Fatalf("first backfill = %v, want one row per column", got)
	}
	kc, tok, ca := storedCredentials(t, store, "plain")
	if kc != "enc:v1:apiVersion: v1" || tok != "enc:v1:tok-plain" || ca != "enc:v1:CA-PLAIN" {
		t.Fatalf("plaintext row after backfill = %q %q %q", kc, tok, ca)
	}
	if conn, err := store.connection(context.Background(), "plain"); err != nil || conn.Kubeconfig != "apiVersion: v1" || conn.BearerToken != "tok-plain" {
		t.Fatalf("sealed row decrypts to %+v (%v)", conn, err)
	}
	if kc, tok, _ := storedCredentials(t, store, "sealed"); kc != "enc:v1:apiVersion: v1" || tok != "enc:v1:tok" {
		t.Fatalf("an already-encrypted row changed: %q %q", kc, tok)
	}
	if kc, tok, ca := storedCredentials(t, store, "empty"); kc != "" || tok != "" || ca != "" {
		t.Fatalf("an empty row changed: %q %q %q", kc, tok, ca)
	}
	again, err := store.backfillEncryption(context.Background())
	if err != nil || again["kubeconfig"]+again["bearer_token"]+again["ca_cert"] != 0 {
		t.Fatalf("second backfill = %v (%v), want a no-op", again, err)
	}

	insertCluster(t, store, "raced", "kube-old", "", "")
	base := store.db.(sqliteDB)
	store.db = &racingDB{sqliteDB: base, race: func() {
		if _, err := base.Exec(context.Background(), `UPDATE plugin_kubernetes_clusters SET kubeconfig = 'enc:v1:kube-new' WHERE id = 'raced'`); err != nil {
			t.Fatal(err)
		}
	}}
	if _, err := store.backfillEncryption(context.Background()); err != nil {
		t.Fatal(err)
	}
	if kc, _, _ := storedCredentials(t, store, "raced"); kc != "enc:v1:kube-new" {
		t.Fatalf("the backfill overwrote a concurrent write: %q", kc)
	}
}
