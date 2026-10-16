// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/therealmcsparrow/mcharbor/core/plugin"
)

// truenasClient is the SnapshotAdapterClient the host dispatches
// to for the "truenas" driver. The plugin calls TrueNAS'
// middleware REST API directly from the host process
// (RequiresAgent=false).
//
// Auth flow: TrueNAS expects either:
//   - Basic auth: `Authorization: Basic <base64(user:password)>`.
//   - Bearer auth: `Authorization: Bearer <api_key>`.
// This client uses Bearer; the API key is supplied by the admin
// from Credentials → Local Users → <user> → API Keys.
//
// Snapshot take/remove:
//   - Capability: GET /api/v2.0/pool/dataset?id=<dataset_path>
//     to confirm the dataset exists, plus GET /api/v2.0/zfs/snapshot
//     to enumerate recent snapshots.
//   - Create: POST /api/v2.0/zfs/snapshot with
//     {"dataset":"<dataset_path>", "name":"<mcharbor-<runId>>",
//      "recursive":true}.
//   - Remove: DELETE /api/v2.0/zfs/snapshot/id/<id>.
type truenasClient struct {
	httpClient *http.Client
	host       plugin.HostContext
}

// NewTruenasClient returns the client the host will dispatch to
// for driver="truenas" configs.
func NewTruenasClient() plugin.SnapshotAdapterClient {
	return &truenasClient{}
}

// ensureInit makes sure the httpClient is built. Cheap to call on
// every method.
func (c *truenasClient) ensureInit(cfg map[string]string) {
	if c.httpClient != nil {
		return
	}
	tr := &http.Transport{
		TLSClientConfig: &tls.Config{
			InsecureSkipVerify: cfg["verify_tls"] == "false",
		},
	}
	c.httpClient = &http.Client{Transport: tr, Timeout: 30 * time.Second}
}

// Capability probes the TrueNAS middleware API for the dataset
// configured by the operator. Returns Available=false with the
// middleware error in the Error field when the connection fails.
func (c *truenasClient) Capability(ctx context.Context, cfg map[string]string) (plugin.SnapshotCapability, error) {
	c.ensureInit(cfg)
	if err := c.requireConfig(cfg); err != nil {
		return plugin.SnapshotCapability{Driver: truenasDriver, Available: false, Error: err.Error()}, nil
	}
	ds, err := c.fetchDataset(ctx, cfg, cfg["dataset_path"])
	if err != nil {
		return plugin.SnapshotCapability{Driver: truenasDriver, Available: false, Error: err.Error()}, nil
	}
	targets := []plugin.SnapshotTarget{
		{
			Name:       ds.Name,
			Mountpoint: ds.Mountpoint,
			UsedBytes:  ds.Used,
			Driver:     truenasDriver,
		},
	}
	return plugin.SnapshotCapability{
		Driver:    truenasDriver,
		ToolPath:  "/api/v2.0/zfs/snapshot",
		Version:   ds.RealOrName(),
		Available: true,
		Targets:   targets,
	}, nil
}

// Create takes a ZFS snapshot of the configured dataset and
// embeds the run ID in the snapshot name. The dataset is
// snapshotted recursively so child datasets are covered.
func (c *truenasClient) Create(ctx context.Context, cfg map[string]string, req plugin.SnapshotCreateRequest) (plugin.SnapshotRef, error) {
	c.ensureInit(cfg)
	if err := c.requireConfig(cfg); err != nil {
		return plugin.SnapshotRef{}, err
	}
	dataset := cfg["dataset_path"]
	snapName := fmt.Sprintf("mcharbor-%s", req.SnapshotID)
	id, err := c.createSnapshot(ctx, cfg, dataset, snapName, req.ReadOnly)
	if err != nil {
		return plugin.SnapshotRef{}, err
	}
	return plugin.SnapshotRef{
		SnapshotName: snapName,
		Targets:      []string{dataset},
		CreatedAt:    time.Now().UTC(),
		MountPath:    id, // store the snapshot id for Remove
	}, nil
}

// Remove deletes the snapshot by id. Best-effort — the host
// treats a non-nil error as a warning.
func (c *truenasClient) Remove(ctx context.Context, cfg map[string]string, ref plugin.SnapshotRef) error {
	c.ensureInit(cfg)
	if ref.MountPath == "" {
		return errors.New("snapshot-truenas: snapshot id missing from ref")
	}
	return c.deleteSnapshot(ctx, cfg, ref.MountPath)
}

// requireConfig validates that the mandatory keys are populated.
func (c *truenasClient) requireConfig(cfg map[string]string) error {
	if cfg["api_url"] == "" {
		return errors.New("snapshot-truenas: api_url is required")
	}
	if cfg["api_key"] == "" {
		return errors.New("snapshot-truenas: api_key is required")
	}
	if cfg["dataset_path"] == "" {
		return errors.New("snapshot-truenas: dataset_path is required")
	}
	return nil
}

// truenasDataset mirrors the small slice of /api/v2.0/pool/dataset
// we care about. The middleware returns ~50 fields per dataset;
// we only decode what we use.
type truenasDataset struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	Mountpoint string `json:"mountpoint"`
	Used       int64  `json:"used"`
	Pool       string `json:"pool"`
}

// RealOrName returns the mountpoint when populated, otherwise the
// dataset name. Used for the Capability.Version field.
func (d *truenasDataset) RealOrName() string {
	if d.Mountpoint != "" {
		return d.Mountpoint
	}
	return d.Name
}

// fetchDataset calls /pool/dataset?id=<path> and returns the
// matching record.
func (c *truenasClient) fetchDataset(ctx context.Context, cfg map[string]string, path string) (*truenasDataset, error) {
	url := fmt.Sprintf("%s/pool/dataset?id=%s",
		strings.TrimRight(cfg["api_url"], "/"),
		urlQueryEscape(path),
	)
	resp, err := c.httpClient.Do(reqWithCtxAuth(ctx, "GET", url, cfg["api_key"]))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("TrueNAS dataset lookup returned %d: %s", resp.StatusCode, string(body))
	}
	var out []truenasDataset
	if err := json.Unmarshal(body, &out); err != nil {
		return nil, fmt.Errorf("decode dataset: %w", err)
	}
	for i := range out {
		if out[i].Name == path || out[i].ID == path {
			return &out[i], nil
		}
	}
	return nil, fmt.Errorf("dataset %q not found on TrueNAS", path)
}

// createSnapshot POSTs to /zfs/snapshot and returns the
// snapshot id (used as MountPath for the subsequent Remove).
func (c *truenasClient) createSnapshot(ctx context.Context, cfg map[string]string, dataset, name string, readOnly bool) (string, error) {
	url := strings.TrimRight(cfg["api_url"], "/") + "/zfs/snapshot"
	payload, _ := json.Marshal(map[string]any{
		"dataset":   dataset,
		"name":      name,
		"recursive": true,
		"readonly":  readOnly,
	})
	req, _ := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(payload))
	req.Header.Set("Authorization", "Bearer "+cfg["api_key"])
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 300 {
		return "", fmt.Errorf("TrueNAS snapshot create returned %d: %s", resp.StatusCode, string(body))
	}
	// The middleware returns the new snapshot's id at "snapshot_id"
	// (SCALE) or as a wrapped object; tolerate both.
	var ack struct {
		SnapshotID string `json:"snapshot_id"`
		ID         string `json:"id"`
	}
	if err := json.Unmarshal(body, &ack); err != nil {
		return "", fmt.Errorf("decode create ack: %w", err)
	}
	if ack.SnapshotID != "" {
		return ack.SnapshotID, nil
	}
	return ack.ID, nil
}

// deleteSnapshot hits DELETE /zfs/snapshot/id/<id>. Idempotent:
// TrueNAS returns 404 when the snapshot is already gone, which we
// treat as success.
func (c *truenasClient) deleteSnapshot(ctx context.Context, cfg map[string]string, id string) error {
	url := fmt.Sprintf("%s/zfs/snapshot/id/%s",
		strings.TrimRight(cfg["api_url"], "/"),
		urlQueryEscape(id),
	)
	req, _ := http.NewRequestWithContext(ctx, "DELETE", url, nil)
	req.Header.Set("Authorization", "Bearer "+cfg["api_key"])
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return nil
	}
	if resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("TrueNAS snapshot delete returned %d: %s", resp.StatusCode, string(body))
	}
	return nil
}