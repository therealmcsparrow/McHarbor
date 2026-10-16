// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package nodelink

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/rs/xid"

	"github.com/therealmcsparrow/mcharbor/core/db"
)

// OwnerLease is how long a claim on an agent connection stays valid without renewal.
//
// Three renewal intervals, matching the instance registry's alive window: a node that
// dies stops renewing and its claim expires, so the agent's next node can take it over
// rather than waiting for a human.
const OwnerLease = 45 * time.Second

// OwnerRenewEvery is the renewal interval the lease length is derived from.
const OwnerRenewEvery = 15 * time.Second

// ErrOwnedElsewhere means another node holds a live claim on this agent connection.
//
// It is not an error condition in a healthy cluster — an agent reconnecting to a second
// node while the first still holds it is exactly the race the lease exists to settle.
var ErrOwnedElsewhere = errors.New("nodelink: another node holds this agent connection")

// Owner is one node's live claim on one agent connection.
type Owner struct {
	EnvID        string    `json:"envId"`
	NodeID       string    `json:"nodeId"`
	AdvertiseURL string    `json:"advertiseUrl"`
	ExpiresAt    time.Time `json:"expiresAt"`
}

// Owners records which node holds each remote agent's WebSocket, as a lease rather than
// a flag, so a node that dies with agents attached does not strand them.
type Owners struct {
	database  *sql.DB
	nodeID    string
	advertise string
	lease     time.Duration
	now       func() time.Time
}

// NewOwners builds the lease store for this node; advertise is the base URL other nodes
// reach this one on, and an empty one makes this node unable to accept forwarded calls.
func NewOwners(database *sql.DB, nodeID, advertise string) *Owners {
	return &Owners{database: database, nodeID: nodeID, advertise: advertise, lease: OwnerLease, now: time.Now}
}

// NodeID is the node these claims are made under.
func (o *Owners) NodeID() string { return o.nodeID }

// AdvertiseURL is the base URL this node publishes for forwarded calls.
func (o *Owners) AdvertiseURL() string { return o.advertise }

// Claim takes or renews this node's claim on envID's agent connection.
//
// The UPDATE only matches this node's own row or an expired one, so a claim is never
// stolen from a live node; the INSERT covers the first claim and loses the race to the
// UNIQUE index rather than to a read-then-write.
func (o *Owners) Claim(ctx context.Context, envID string) error {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	now := o.now().UTC()
	nowText := now.Format(time.RFC3339)
	expires := now.Add(o.lease).Format(time.RFC3339)
	res, err := o.database.ExecContext(ctx, `UPDATE cluster_agent_owners SET node_id = ?, advertise_url = ?,
		claimed_at = ?, expires_at = ?, updated_at = ? WHERE environment_id = ? AND (node_id = ? OR expires_at < ?)`,
		o.nodeID, o.advertise, nowText, expires, nowText, envID, o.nodeID, nowText)
	if err != nil {
		return fmt.Errorf("nodelink: claiming %s: %w", envID, err)
	}
	if db.RowsAffected(res) > 0 {
		return nil
	}
	_, err = o.database.ExecContext(ctx, `INSERT INTO cluster_agent_owners (id, environment_id, node_id,
		advertise_url, claimed_at, expires_at, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		xid.New().String(), envID, o.nodeID, o.advertise, nowText, expires, nowText, nowText)
	if err == nil {
		return nil
	}
	// The row exists and belongs to a node whose lease has not run out.
	if owner, ok, lookupErr := o.Owner(ctx, envID); lookupErr == nil && ok && owner.NodeID != o.nodeID {
		return fmt.Errorf("%w: %s", ErrOwnedElsewhere, owner.NodeID)
	}
	return fmt.Errorf("nodelink: claiming %s: %w", envID, err)
}

// Release drops this node's claim, so the agent's next node takes it over at once
// instead of after the lease expires.
func (o *Owners) Release(ctx context.Context, envID string) error {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if _, err := o.database.ExecContext(ctx,
		`DELETE FROM cluster_agent_owners WHERE environment_id = ? AND node_id = ?`, envID, o.nodeID); err != nil {
		return fmt.Errorf("nodelink: releasing %s: %w", envID, err)
	}
	return nil
}

// Owner returns the live claim on envID, or false when nobody holds one.
func (o *Owners) Owner(ctx context.Context, envID string) (Owner, bool, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	var out Owner
	var expires string
	err := o.database.QueryRowContext(ctx, `SELECT environment_id, node_id, advertise_url, expires_at
		FROM cluster_agent_owners WHERE environment_id = ?`, envID).
		Scan(&out.EnvID, &out.NodeID, &out.AdvertiseURL, &expires)
	if errors.Is(err, sql.ErrNoRows) {
		return Owner{}, false, nil
	}
	if err != nil {
		return Owner{}, false, fmt.Errorf("nodelink: reading owner of %s: %w", envID, err)
	}
	at, err := time.Parse(time.RFC3339, expires)
	if err != nil || o.now().UTC().After(at) {
		return Owner{}, false, nil
	}
	out.ExpiresAt = at
	return out, true, nil
}

// List returns every live claim, for the topology view and the cross-node diagnostic.
func (o *Owners) List(ctx context.Context) ([]Owner, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	rows, err := o.database.QueryContext(ctx, `SELECT environment_id, node_id, advertise_url, expires_at
		FROM cluster_agent_owners ORDER BY environment_id LIMIT 1000`)
	if err != nil {
		return nil, fmt.Errorf("nodelink: listing owners: %w", err)
	}
	defer func() {
		if cerr := rows.Close(); cerr != nil {
			slog.Debug("nodelink: closing owner rows failed", "error", cerr)
		}
	}()
	now := o.now().UTC()
	out := []Owner{}
	for rows.Next() {
		var owner Owner
		var expires string
		if err := rows.Scan(&owner.EnvID, &owner.NodeID, &owner.AdvertiseURL, &expires); err != nil {
			return nil, fmt.Errorf("nodelink: scanning owner: %w", err)
		}
		at, err := time.Parse(time.RFC3339, expires)
		if err != nil || now.After(at) {
			continue
		}
		owner.ExpiresAt = at
		out = append(out, owner)
	}
	return out, rows.Err()
}
