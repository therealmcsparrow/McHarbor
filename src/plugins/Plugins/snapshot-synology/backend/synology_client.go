// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/therealmcsparrow/mcharbor/core/plugin"
)

// synologyClient is the SnapshotAdapterClient the host dispatches
// to for the "synology" driver. The plugin calls Synology DSM's
// REST API directly from the host process (RequiresAgent=false).
//
// Auth flow: POST /webapi/entry.cgi?api=SYNO.API.Auth with
// account + passwd + session=Optional. The returned sid is sent
// on every subsequent request as `?_sid=<sid>` (or as
// `Cookie: id=<sid>`). Sessions expire after the DSM's idle
// timeout (default 30 minutes); the client re-auths lazily.
//
// Snapshot take/remove over DSM 7.x:
//   - Create: POST /webapi/entry.cgi?api=SYNO.Backup.Service.Repository
//     with method=create, share_name=X, snap_name=mcharbor-<runId>.
//   - Remove: POST same endpoint with method=delete.
//   - List: POST with method=list to discover available shares.
type synologyClient struct {
	httpClient *http.Client
	host       plugin.HostContext
}

// NewSynologyClient returns the client the host will dispatch to
// for driver="synology" configs.
func NewSynologyClient() plugin.SnapshotAdapterClient {
	return &synologyClient{}
}

// ensureInit makes sure the httpClient is built. Cheap to call on
// every method.
func (c *synologyClient) ensureInit(cfg map[string]string) {
	if c.httpClient != nil {
		return
	}
	tr := &http.Transport{
		TLSClientConfig: &tls.Config{
			InsecureSkipVerify: cfg["ignore_certificate_errors"] == "true",
		},
	}
	c.httpClient = &http.Client{Transport: tr, Timeout: 30 * time.Second}
}

// synologyAuthTimeout caps the login round-trip.
const synologyAuthTimeout = 30 * time.Second

// synologyScope invalidates a stale session so the next call
// re-auths. Cheap (single in-memory field).
func (c *synologyClient) invalidateSession() {
	// No persistent state — we re-auth on every Capability call
	// (admin UI probe) and once per Create/Remove pair at backup
	// time. The DSM session cache is a non-issue.
}

// Capability probes the DSM for available shares via the
// login + list round-trip. Returns Available=false with the DSM
// error in the Error field when the connection fails.
func (c *synologyClient) Capability(ctx context.Context, cfg map[string]string) (plugin.SnapshotCapability, error) {
	c.ensureInit(cfg)
	shares, err := c.listShares(ctx, cfg)
	if err != nil {
		return plugin.SnapshotCapability{
			Driver:    synologyDriver,
			Available: false,
			Error:     err.Error(),
		}, nil
	}
	targets := make([]plugin.SnapshotTarget, 0, len(shares))
	for _, share := range shares {
		targets = append(targets, plugin.SnapshotTarget{
			Name:       share,
			Mountpoint: "/volume1/" + share,
			Driver:     synologyDriver,
		})
	}
	return plugin.SnapshotCapability{
		Driver:    synologyDriver,
		Version:   "Synology DSM 7.x",
		Available: true,
		Targets:   targets,
	}, nil
}

// Create takes a snapshot of the share name in the config. The
// returned SnapshotRef.SnapshotName embeds the run ID so Remove
// can find it.
func (c *synologyClient) Create(ctx context.Context, cfg map[string]string, req plugin.SnapshotCreateRequest) (plugin.SnapshotRef, error) {
	c.ensureInit(cfg)
	share := cfg["share_name"]
	if share == "" {
		return plugin.SnapshotRef{}, errors.New("snapshot-synology: share_name is required")
	}
	snapName := fmt.Sprintf("mcharbor-%s", req.SnapshotID)
	if err := c.takeSnapshot(ctx, cfg, share, snapName); err != nil {
		return plugin.SnapshotRef{}, err
	}
	return plugin.SnapshotRef{
		SnapshotName: snapName,
		Targets:      []string{share},
		CreatedAt:    time.Now().UTC(),
	}, nil
}

// Remove deletes the snapshot repository entry. Best-effort —
// the host treats a non-nil error as a warning.
func (c *synologyClient) Remove(ctx context.Context, cfg map[string]string, ref plugin.SnapshotRef) error {
	c.ensureInit(cfg)
	if len(ref.Targets) == 0 {
		return nil
	}
	return c.removeSnapshot(ctx, cfg, ref.Targets[0], ref.SnapshotName)
}

// listShares authenticates and returns the share names the DSM
// exposes. Implements the minimum needed for the admin UI's
// picker; doesn't expose full share metadata.
func (c *synologyClient) listShares(ctx context.Context, cfg map[string]string) ([]string, error) {
	sid, err := c.login(ctx, cfg)
	if err != nil {
		return nil, err
	}
	apiInfo := "SYNO.Backup.Service.Repository"
	apiVersion, err := c.apiVersion(ctx, cfg, sid, apiInfo)
	if err != nil {
		return nil, err
	}
	url := fmt.Sprintf("%s/webapi/entry.cgi?api=%s&version=%s&method=list&_sid=%s",
		strings.TrimRight(cfg["dsm_url"], "/"), apiInfo, apiVersion, sid)
	resp, err := c.httpClient.Do(reqWithCtx(ctx, "GET", url))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	var payload struct {
		Success bool   `json:"success"`
		Data    struct {
			Repositories []struct {
				Name string `json:"name"`
			} `json:"repositories"`
		} `json:"data"`
		Error struct {
			Code int    `json:"code"`
			Text string `json:"text"`
		} `json:"error"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return nil, err
	}
	if !payload.Success {
		return nil, fmt.Errorf("Synology list failed: %s (code %d)", payload.Error.Text, payload.Error.Code)
	}
	out := make([]string, 0, len(payload.Data.Repositories))
	for _, r := range payload.Data.Repositories {
		out = append(out, r.Name)
	}
	return out, nil
}

// takeSnapshot creates a snapshot repository for the given share
// with the supplied name. The DSM maps each (share, name) pair to
// a unique snapshot; remove finds it by the same pair.
func (c *synologyClient) takeSnapshot(ctx context.Context, cfg map[string]string, share, name string) error {
	sid, err := c.login(ctx, cfg)
	if err != nil {
		return err
	}
	apiInfo := "SYNO.Backup.Service.Repository"
	apiVersion, err := c.apiVersion(ctx, cfg, sid, apiInfo)
	if err != nil {
		return err
	}
	url := fmt.Sprintf("%s/webapi/entry.cgi?api=%s&version=%s&method=create&_sid=%s",
		strings.TrimRight(cfg["dsm_url"], "/"), apiInfo, apiVersion, sid)
	form := strings.NewReader(fmt.Sprintf("share_name=%s&snap_name=%s", share, name))
	resp, err := c.httpClient.Do(reqWithCtxBody(ctx, "POST", url, form))
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	var payload struct {
		Success bool `json:"success"`
		Error   struct {
			Code int    `json:"code"`
			Text string `json:"text"`
		} `json:"error"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return err
	}
	if !payload.Success {
		return fmt.Errorf("Synology create failed: %s (code %d)", payload.Error.Text, payload.Error.Code)
	}
	return nil
}

// removeSnapshot deletes the snapshot repository identified by
// (share, name). Idempotent: a missing snapshot returns an error
// from the DSM; we treat code 408 (not found) as a no-op.
func (c *synologyClient) removeSnapshot(ctx context.Context, cfg map[string]string, share, name string) error {
	sid, err := c.login(ctx, cfg)
	if err != nil {
		return err
	}
	apiInfo := "SYNO.Backup.Service.Repository"
	apiVersion, err := c.apiVersion(ctx, cfg, sid, apiInfo)
	if err != nil {
		return err
	}
	url := fmt.Sprintf("%s/webapi/entry.cgi?api=%s&version=%s&method=delete&_sid=%s",
		strings.TrimRight(cfg["dsm_url"], "/"), apiInfo, apiVersion, sid)
	form := strings.NewReader(fmt.Sprintf("share_name=%s&snap_name=%s", share, name))
	resp, err := c.httpClient.Do(reqWithCtxBody(ctx, "POST", url, form))
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	var payload struct {
		Success bool `json:"success"`
		Error   struct {
			Code int    `json:"code"`
			Text string `json:"text"`
		} `json:"error"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return err
	}
	if !payload.Success {
		// 408 = "object not found" — already removed by another
		// path, treat as success.
		if payload.Error.Code == 408 {
			return nil
		}
		return fmt.Errorf("Synology delete failed: %s (code %d)", payload.Error.Text, payload.Error.Code)
	}
	return nil
}

// login posts to SYNO.API.Auth and returns the session id.
func (c *synologyClient) login(ctx context.Context, cfg map[string]string) (string, error) {
	apiInfo := "SYNO.API.Auth"
	apiVersion, err := c.apiVersion(ctx, cfg, "", apiInfo)
	if err != nil {
		return "", err
	}
	url := fmt.Sprintf("%s/webapi/entry.cgi?api=%s&version=%s&method=login&account=%s&passwd=%s&format=cookie",
		strings.TrimRight(cfg["dsm_url"], "/"),
		apiInfo,
		apiVersion,
		urlQueryEscape(cfg["username"]),
		urlQueryEscape(cfg["password"]),
	)
	resp, err := c.httpClient.Do(reqWithCtx(ctx, "GET", url))
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	var payload struct {
		Success bool `json:"success"`
		Data    struct {
			SID string `json:"sid"`
		} `json:"data"`
		Error struct {
			Code int    `json:"code"`
			Text string `json:"text"`
		} `json:"error"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return "", err
	}
	if !payload.Success {
		return "", fmt.Errorf("Synology login failed: %s (code %d)", payload.Error.Text, payload.Error.Code)
	}
	return payload.Data.SID, nil
}

// apiVersion fetches the DSM's API version for the given API.
// sid is empty for unauthenticated endpoints (SYNO.API.Auth).
func (c *synologyClient) apiVersion(ctx context.Context, cfg map[string]string, sid, api string) (string, error) {
	url := fmt.Sprintf("%s/webapi/entry.cgi?api=SYNO.API.Info&version=1&method=query",
		strings.TrimRight(cfg["dsm_url"], "/"))
	if sid != "" {
		url += "&_sid=" + sid
	}
	resp, err := c.httpClient.Do(reqWithCtx(ctx, "GET", url))
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	var payload struct {
		Success bool `json:"success"`
		Data    map[string]string `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return "", err
	}
	if !payload.Success {
		return "", errors.New("Synology API info not reachable")
	}
	v, ok := payload.Data[api]
	if !ok {
		return "", fmt.Errorf("Synology API %q not available on this DSM", api)
	}
	return v, nil
}
