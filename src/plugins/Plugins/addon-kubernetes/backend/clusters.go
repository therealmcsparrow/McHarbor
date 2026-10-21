// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/rs/xid"
)

// clusterStore owns `plugin_kubernetes_clusters`.
//
// Clusters used to be rows in the host's `environments` table with an
// `orchestrator_type` of "kubernetes". That coupling is what kept
// `k8s.io/client-go` in the host binary and a Kubernetes-shaped branch in the
// environments module, so the table moved here with the rest of the feature.
//
// Migration 147 in the host copies the existing rows across before dropping
// the columns, carrying the *ciphertext* rather than the plaintext: the
// plugin's `Encryption` handle is the host's own AES-256-GCM service, so what
// the host encrypted the plugin decrypts, and no credential is ever in the
// clear during the hand-off.
type clusterStore struct {
	db  pluginDB
	enc pluginEncryption
}

// Cluster is what the API returns. It deliberately carries no credential:
// `kubeconfig`, `bearerToken` and `caCert` are write-only fields, present on
// the create/update request shapes and absent from every response.
type Cluster struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
	ServerURL string `json:"serverUrl"`
	IsDefault bool   `json:"isDefault"`
	IsActive  bool   `json:"isActive"`
	// HasKubeconfig / HasToken tell the UI which connection style a cluster
	// was saved with, so the edit form can say "kubeconfig on file" instead
	// of rendering an empty box that looks like the credential was lost.
	HasKubeconfig bool `json:"hasKubeconfig"`
	HasToken      bool `json:"hasToken"`
	// EnvironmentID links the cluster to the McHarbor environment it runs on, which is
	// what puts it on that environment's card. Empty means it runs elsewhere.
	EnvironmentID string `json:"environmentId"`
	CreatedAt     string `json:"createdAt"`
	UpdatedAt     string `json:"updatedAt"`
}

// ClusterInput is the create/update body. Every credential field is a pointer
// so "absent" and "cleared" stay distinguishable on update — the same
// nil-means-leave-alone contract the host's environments module used.
type ClusterInput struct {
	Name        *string `json:"name"`
	Namespace   *string `json:"namespace"`
	ServerURL   *string `json:"serverUrl"`
	Kubeconfig  *string `json:"kubeconfig"`
	BearerToken *string `json:"bearerToken"`
	CACert      *string `json:"caCert"`
	IsDefault   *bool   `json:"isDefault"`
	IsActive    *bool   `json:"isActive"`
	// EnvironmentID is "" to unlink; nil leaves it as stored.
	EnvironmentID *string `json:"environmentId"`
}

var errClusterNotFound = errors.New("cluster not found")

const clusterColumns = `id, name, namespace, server_url, kubeconfig, bearer_token,
	ca_cert, is_default, is_active, created_at, updated_at, environment_id`

// errBadEnvironmentID refuses an environment id that could not be one, before it is stored.
var errBadEnvironmentID = errors.New("that is not an environment id")

// reEnvironmentID is the shape of a McHarbor environment id (an xid or a slug).
var reEnvironmentID = regexp.MustCompile(`^[A-Za-z0-9_-]{1,64}$`)

// environmentIDOf trims and checks an optional environment id.
func environmentIDOf(raw *string) (string, error) {
	id := strings.TrimSpace(deref(raw))
	if id != "" && !reEnvironmentID.MatchString(id) {
		return "", errBadEnvironmentID
	}
	return id, nil
}

// ensureSchema creates the table when it is missing.
//
// The host only applies a plugin's migrations/*.sql on a fresh install, so an
// existing install upgrading into a build that adds one does not get it
// re-run. Doing it here as well means the plugin cannot load into a state
// where every query fails on a missing table — the same self-heal the
// reporting plugin does for the same reason.
func (s *clusterStore) ensureSchema(ctx context.Context) error {
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS plugin_kubernetes_clusters (
			id           TEXT PRIMARY KEY,
			name         TEXT    NOT NULL,
			namespace    TEXT    NOT NULL DEFAULT '',
			server_url   TEXT    NOT NULL DEFAULT '',
			kubeconfig   TEXT    NOT NULL DEFAULT '',
			bearer_token TEXT    NOT NULL DEFAULT '',
			ca_cert      TEXT    NOT NULL DEFAULT '',
			is_default   INTEGER NOT NULL DEFAULT 0,
			is_active    INTEGER NOT NULL DEFAULT 1,
			created_at   TEXT    NOT NULL,
			updated_at   TEXT    NOT NULL
		)`,
		`CREATE INDEX IF NOT EXISTS idx_plugin_kubernetes_clusters_default
			ON plugin_kubernetes_clusters (is_default, is_active)`,
	}
	for _, stmt := range stmts {
		if _, err := s.db.Exec(ctx, stmt); err != nil {
			return fmt.Errorf("kubernetes: ensure schema: %w", err)
		}
	}
	// Added after the table shipped, so an existing install gains it here; the
	// duplicate-column error is the "already there" answer, not a failure.
	if _, err := s.db.Exec(ctx,
		`ALTER TABLE plugin_kubernetes_clusters ADD COLUMN environment_id TEXT NOT NULL DEFAULT ''`); err != nil &&
		!strings.Contains(strings.ToLower(err.Error()), "duplicate column") {
		return fmt.Errorf("kubernetes: ensure schema: %w", err)
	}
	return nil
}

// list returns every cluster, newest last, without credentials.
func (s *clusterStore) list(ctx context.Context) ([]Cluster, error) {
	rows, err := s.db.Query(ctx, `SELECT `+clusterColumns+`
		FROM plugin_kubernetes_clusters
		ORDER BY is_default DESC, created_at ASC
		LIMIT 1000`)
	if err != nil {
		return nil, fmt.Errorf("listing clusters: %w", err)
	}
	defer func() { _ = rows.Close() }()

	clusters := make([]Cluster, 0, 8)
	for rows.Next() {
		cluster, _, scanErr := scanCluster(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		clusters = append(clusters, cluster)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("listing clusters: %w", err)
	}
	return clusters, nil
}

// get returns one cluster without credentials.
func (s *clusterStore) get(ctx context.Context, id string) (Cluster, error) {
	cluster, _, err := s.row(ctx, id)
	return cluster, err
}

// connection returns one cluster's decrypted connection details.
func (s *clusterStore) connection(ctx context.Context, id string) (*Connection, error) {
	_, secrets, err := s.row(ctx, id)
	if err != nil {
		return nil, err
	}
	conn := &Connection{ServerURL: secrets.serverURL}
	if conn.Kubeconfig, err = s.decrypt(secrets.kubeconfig); err != nil {
		return nil, fmt.Errorf("decrypting kubeconfig: %w", err)
	}
	if conn.BearerToken, err = s.decrypt(secrets.bearerToken); err != nil {
		return nil, fmt.Errorf("decrypting bearer token: %w", err)
	}
	if conn.CACert, err = s.decrypt(secrets.caCert); err != nil {
		return nil, fmt.Errorf("decrypting ca cert: %w", err)
	}
	return conn, nil
}

// defaultClusterID resolves the "" / "default" alias.
func (s *clusterStore) defaultClusterID(ctx context.Context) (string, error) {
	for _, query := range []string{
		`SELECT id FROM plugin_kubernetes_clusters WHERE is_default = 1 AND is_active = 1 LIMIT 1`,
		`SELECT id FROM plugin_kubernetes_clusters WHERE is_active = 1 ORDER BY created_at ASC LIMIT 1`,
	} {
		rows, err := s.db.Query(ctx, query)
		if err != nil {
			return "", fmt.Errorf("resolving default cluster: %w", err)
		}
		var id string
		found := rows.Next()
		if found {
			if scanErr := rows.Scan(&id); scanErr != nil {
				_ = rows.Close()
				return "", fmt.Errorf("resolving default cluster: %w", scanErr)
			}
		}
		_ = rows.Close()
		if found && id != "" {
			return id, nil
		}
	}
	return "", ErrNoClusters
}

// create inserts a cluster and returns it without credentials.
func (s *clusterStore) create(ctx context.Context, in ClusterInput) (Cluster, error) {
	name := strings.TrimSpace(deref(in.Name))
	if name == "" {
		return Cluster{}, fmt.Errorf("cluster name is required")
	}
	if err := guardKubeconfigInput(in.Kubeconfig); err != nil {
		return Cluster{}, err
	}
	kubeconfig, err := s.encrypt(deref(in.Kubeconfig))
	if err != nil {
		return Cluster{}, fmt.Errorf("encrypting kubeconfig: %w", err)
	}
	bearerToken, err := s.encrypt(deref(in.BearerToken))
	if err != nil {
		return Cluster{}, fmt.Errorf("encrypting bearer token: %w", err)
	}
	caCert, err := s.encrypt(deref(in.CACert))
	if err != nil {
		return Cluster{}, fmt.Errorf("encrypting ca cert: %w", err)
	}
	if kubeconfig == "" && (strings.TrimSpace(deref(in.ServerURL)) == "" || bearerToken == "") {
		return Cluster{}, fmt.Errorf("cluster needs a kubeconfig, or a server URL and bearer token")
	}
	environmentID, err := environmentIDOf(in.EnvironmentID)
	if err != nil {
		return Cluster{}, err
	}

	now := time.Now().UTC().Format(time.RFC3339)
	id := xid.New().String()
	isDefault := derefBool(in.IsDefault)

	if isDefault {
		if err := s.clearDefault(ctx); err != nil {
			return Cluster{}, err
		}
	}
	if _, err := s.db.Exec(ctx, `INSERT INTO plugin_kubernetes_clusters
		(`+clusterColumns+`)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		id, name, strings.TrimSpace(deref(in.Namespace)), strings.TrimSpace(deref(in.ServerURL)),
		kubeconfig, bearerToken, caCert, boolToInt(isDefault), boolToInt(derefBoolDefault(in.IsActive, true)),
		now, now, environmentID,
	); err != nil {
		return Cluster{}, fmt.Errorf("creating cluster: %w", err)
	}
	// The first cluster is the default one, whatever the caller asked: a
	// pool that resolves "" to nothing would make the plugin unusable until
	// somebody noticed the flag.
	if err := s.ensureOneDefault(ctx); err != nil {
		return Cluster{}, err
	}
	return s.get(ctx, id)
}

// update applies a partial change. Nil credential fields are left as stored.
func (s *clusterStore) update(ctx context.Context, id string, in ClusterInput) (Cluster, error) {
	if _, err := s.get(ctx, id); err != nil {
		return Cluster{}, err
	}
	if err := guardKubeconfigInput(in.Kubeconfig); err != nil {
		return Cluster{}, err
	}

	sets := make([]string, 0, 8)
	args := make([]any, 0, 9)
	appendSet := func(column string, value any) {
		sets = append(sets, column+" = ?")
		args = append(args, value)
	}

	if in.Name != nil {
		name := strings.TrimSpace(*in.Name)
		if name == "" {
			return Cluster{}, fmt.Errorf("cluster name is required")
		}
		appendSet("name", name)
	}
	if in.Namespace != nil {
		appendSet("namespace", strings.TrimSpace(*in.Namespace))
	}
	if in.ServerURL != nil {
		appendSet("server_url", strings.TrimSpace(*in.ServerURL))
	}
	for _, field := range []struct {
		column string
		value  *string
	}{
		{"kubeconfig", in.Kubeconfig},
		{"bearer_token", in.BearerToken},
		{"ca_cert", in.CACert},
	} {
		if field.value == nil {
			continue
		}
		cipher, err := s.encrypt(*field.value)
		if err != nil {
			return Cluster{}, fmt.Errorf("encrypting %s: %w", field.column, err)
		}
		appendSet(field.column, cipher)
	}
	if in.IsActive != nil {
		appendSet("is_active", boolToInt(*in.IsActive))
	}
	if in.EnvironmentID != nil {
		environmentID, err := environmentIDOf(in.EnvironmentID)
		if err != nil {
			return Cluster{}, err
		}
		appendSet("environment_id", environmentID)
	}
	if in.IsDefault != nil && *in.IsDefault {
		if err := s.clearDefault(ctx); err != nil {
			return Cluster{}, err
		}
		appendSet("is_default", 1)
	}

	if len(sets) > 0 {
		appendSet("updated_at", time.Now().UTC().Format(time.RFC3339))
		args = append(args, id)
		if _, err := s.db.Exec(ctx,
			`UPDATE plugin_kubernetes_clusters SET `+strings.Join(sets, ", ")+` WHERE id = ?`,
			args...,
		); err != nil {
			return Cluster{}, fmt.Errorf("updating cluster: %w", err)
		}
	}
	if err := s.ensureOneDefault(ctx); err != nil {
		return Cluster{}, err
	}
	return s.get(ctx, id)
}

// remove deletes a cluster.
func (s *clusterStore) remove(ctx context.Context, id string) error {
	affected, err := s.db.Exec(ctx, `DELETE FROM plugin_kubernetes_clusters WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("deleting cluster: %w", err)
	}
	if affected == 0 {
		return errClusterNotFound
	}
	return s.ensureOneDefault(ctx)
}

// --- internals ----------------------------------------------------------

// clusterSecrets is the credential half of a row. Kept apart from Cluster so
// there is no shape that can be handed to a JSON encoder by accident.
type clusterSecrets struct {
	serverURL   string
	kubeconfig  string
	bearerToken string
	caCert      string
}

func (s *clusterStore) row(ctx context.Context, id string) (Cluster, clusterSecrets, error) {
	rows, err := s.db.Query(ctx,
		`SELECT `+clusterColumns+` FROM plugin_kubernetes_clusters WHERE id = ?`, id)
	if err != nil {
		return Cluster{}, clusterSecrets{}, fmt.Errorf("reading cluster: %w", err)
	}
	defer func() { _ = rows.Close() }()
	if !rows.Next() {
		if err := rows.Err(); err != nil {
			return Cluster{}, clusterSecrets{}, fmt.Errorf("reading cluster: %w", err)
		}
		return Cluster{}, clusterSecrets{}, errClusterNotFound
	}
	return scanCluster(rows)
}

// scanRow is the subset of plugin.Rows scanCluster needs, so both the list
// loop and the single-row read share one column order.
type scanRow interface{ Scan(dest ...any) error }

func scanCluster(rows scanRow) (Cluster, clusterSecrets, error) {
	var (
		c                  Cluster
		sec                clusterSecrets
		isDefault, isAlive int
	)
	if err := rows.Scan(
		&c.ID, &c.Name, &c.Namespace, &sec.serverURL,
		&sec.kubeconfig, &sec.bearerToken, &sec.caCert,
		&isDefault, &isAlive, &c.CreatedAt, &c.UpdatedAt, &c.EnvironmentID,
	); err != nil {
		return Cluster{}, clusterSecrets{}, fmt.Errorf("scanning cluster: %w", err)
	}
	c.ServerURL = sec.serverURL
	c.IsDefault = isDefault == 1
	c.IsActive = isAlive == 1
	c.HasKubeconfig = sec.kubeconfig != ""
	c.HasToken = sec.bearerToken != ""
	return c, sec, nil
}

func (s *clusterStore) clearDefault(ctx context.Context) error {
	if _, err := s.db.Exec(ctx,
		`UPDATE plugin_kubernetes_clusters SET is_default = 0 WHERE is_default = 1`); err != nil {
		return fmt.Errorf("clearing default cluster: %w", err)
	}
	return nil
}

// ensureOneDefault promotes the oldest active cluster when nothing is marked
// default — after a create that did not ask for it, or a delete that removed
// the one that was.
func (s *clusterStore) ensureOneDefault(ctx context.Context) error {
	rows, err := s.db.Query(ctx,
		`SELECT COUNT(*) FROM plugin_kubernetes_clusters WHERE is_default = 1 AND is_active = 1`)
	if err != nil {
		return fmt.Errorf("counting default clusters: %w", err)
	}
	var count int
	if rows.Next() {
		if scanErr := rows.Scan(&count); scanErr != nil {
			_ = rows.Close()
			return fmt.Errorf("counting default clusters: %w", scanErr)
		}
	}
	_ = rows.Close()
	if count > 0 {
		return nil
	}
	if _, err := s.db.Exec(ctx, `UPDATE plugin_kubernetes_clusters
		SET is_default = 1
		WHERE id = (SELECT id FROM plugin_kubernetes_clusters
		            WHERE is_active = 1 ORDER BY created_at ASC LIMIT 1)`); err != nil {
		return fmt.Errorf("promoting default cluster: %w", err)
	}
	return nil
}

// errClusterEncryptionUnavailable refuses to store or read a kubeconfig, token or CA
// without the host encryption service, rather than keeping credentials in plaintext.
var errClusterEncryptionUnavailable = errors.New("kubernetes: cluster credentials cannot be stored without encryption")

func (s *clusterStore) encrypt(plain string) (string, error) {
	if plain == "" {
		return "", nil
	}
	if s.enc == nil {
		return "", errClusterEncryptionUnavailable
	}
	return s.enc.EncryptString(plain)
}

func (s *clusterStore) decrypt(cipher string) (string, error) {
	if cipher == "" {
		return "", nil
	}
	if s.enc == nil {
		return "", errClusterEncryptionUnavailable
	}
	return s.enc.DecryptString(cipher)
}

func deref(v *string) string {
	if v == nil {
		return ""
	}
	return *v
}

func derefBool(v *bool) bool { return v != nil && *v }

func derefBoolDefault(v *bool, fallback bool) bool {
	if v == nil {
		return fallback
	}
	return *v
}

func boolToInt(v bool) int {
	if v {
		return 1
	}
	return 0
}
