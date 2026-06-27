// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"

	"github.com/gorilla/websocket"
)

type DockerContainerInspect struct {
	ID     string `json:"Id"`
	Name   string `json:"Name"`
	Config *struct {
		Image string `json:"Image"`
	} `json:"Config"`
	Mounts []struct {
		Type        string `json:"Type"`
		Destination string `json:"Destination"`
	} `json:"Mounts"`
}

// Proxy handles forwarding Docker API requests to the local Docker socket.
type Proxy struct {
	dockerHost string
	httpClient *http.Client
	logger     *slog.Logger
	execMu     sync.RWMutex
	execConns  map[string]net.Conn
}

// NewProxy creates a new Docker API proxy.
func NewProxy(dockerHost string, logger *slog.Logger) *Proxy {
	transport := &http.Transport{DisableCompression: true}

	if strings.HasPrefix(dockerHost, "unix://") {
		socketPath := strings.TrimPrefix(dockerHost, "unix://")
		transport.DialContext = func(ctx context.Context, _, _ string) (net.Conn, error) {
			return net.Dial("unix", socketPath)
		}
	}

	return &Proxy{
		dockerHost: dockerHost,
		httpClient: &http.Client{Transport: transport},
		logger:     logger,
		execConns:  make(map[string]net.Conn),
	}
}

// DetectDockerVersion tries to get the Docker API version from the local daemon.
// Prune runs a Docker prune operation locally. Mirrors the server-side logic
// in src/backend/modules/metrics/service.go so per-resource totals can be
// aggregated identically. Returns ItemsDeleted and SpaceReclaimed.
//
// Supported types: "system" (containers + images + networks, +volumes when
// payload.Volumes=true), "builder", "volumes", "images", "containers",
// "networks".
func (p *Proxy) Prune(ctx context.Context, payload PrunePayload) (PrunePayload, error) {
	out := PrunePayload{Type: payload.Type, Volumes: payload.Volumes}

	do := func(method, path string, body io.Reader, contentType string) (int64, int64, error) {
		var req *http.Request
		var err error
		if body != nil {
			req, err = http.NewRequestWithContext(ctx, method, "http://docker"+path, body)
			if err != nil {
				return 0, 0, err
			}
			req.Header.Set("Content-Type", contentType)
		} else {
			req, err = http.NewRequestWithContext(ctx, method, "http://docker"+path, nil)
			if err != nil {
				return 0, 0, err
			}
		}
		resp, err := p.httpClient.Do(req)
		if err != nil {
			return 0, 0, err
		}
		defer resp.Body.Close()
		if resp.StatusCode >= 300 {
			respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
			return 0, 0, fmt.Errorf("prune %s returned status %d: %s", path, resp.StatusCode, strings.TrimSpace(string(respBody)))
		}
		var report struct {
			SpaceReclaimed    int64    `json:"SpaceReclaimed"`
			ContainersDeleted []string `json:"ContainersDeleted"`
			ImagesDeleted     []string `json:"ImagesDeleted"`
			VolumesDeleted    []string `json:"VolumesDeleted"`
			NetworksDeleted   []string `json:"NetworksDeleted"`
			CachesDeleted     []string `json:"CachesDeleted"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&report); err != nil {
			return 0, 0, fmt.Errorf("decoding prune response: %w", err)
		}
		items := int64(len(report.ContainersDeleted)) +
			int64(len(report.ImagesDeleted)) +
			int64(len(report.VolumesDeleted)) +
			int64(len(report.NetworksDeleted)) +
			int64(len(report.CachesDeleted))
		return items, report.SpaceReclaimed, nil
	}

	switch payload.Type {
	case "system":
		imgItems, imgReclaim, err := do("POST", "/images/prune", strings.NewReader("{}"), "application/json")
		if err != nil {
			return out, fmt.Errorf("system prune (images): %w", err)
		}
		ctnItems, ctnReclaim, err := do("POST", "/containers/prune", strings.NewReader("{}"), "application/json")
		if err != nil {
			return out, fmt.Errorf("system prune (containers): %w", err)
		}
		netItems, _, err := do("POST", "/networks/prune", strings.NewReader("{}"), "application/json")
		if err != nil {
			return out, fmt.Errorf("system prune (networks): %w", err)
		}
		out.ItemsDeleted = imgItems + ctnItems + netItems
		out.SpaceReclaimed = imgReclaim + ctnReclaim
		if payload.Volumes {
			volItems, volReclaim, err := do("POST", "/volumes/prune", strings.NewReader("{}"), "application/json")
			if err != nil {
				return out, fmt.Errorf("system prune (volumes): %w", err)
			}
			out.ItemsDeleted += volItems
			out.SpaceReclaimed += volReclaim
		}
	case "builder":
		items, reclaim, err := do("POST", "/build/prune?all=0", strings.NewReader("{}"), "application/json")
		if err != nil {
			return out, fmt.Errorf("builder prune: %w", err)
		}
		out.ItemsDeleted = items
		out.SpaceReclaimed = reclaim
	case "volumes":
		items, reclaim, err := do("POST", "/volumes/prune", strings.NewReader("{}"), "application/json")
		if err != nil {
			return out, fmt.Errorf("volume prune: %w", err)
		}
		out.ItemsDeleted = items
		out.SpaceReclaimed = reclaim
	case "images":
		items, reclaim, err := do("POST", "/images/prune", strings.NewReader("{}"), "application/json")
		if err != nil {
			return out, fmt.Errorf("image prune: %w", err)
		}
		out.ItemsDeleted = items
		out.SpaceReclaimed = reclaim
	case "containers":
		items, reclaim, err := do("POST", "/containers/prune", strings.NewReader("{}"), "application/json")
		if err != nil {
			return out, fmt.Errorf("container prune: %w", err)
		}
		out.ItemsDeleted = items
		out.SpaceReclaimed = reclaim
	case "networks":
		items, _, err := do("POST", "/networks/prune", strings.NewReader("{}"), "application/json")
		if err != nil {
			return out, fmt.Errorf("network prune: %w", err)
		}
		out.ItemsDeleted = items
	default:
		return out, fmt.Errorf("unknown prune type %q", payload.Type)
	}

	out.Success = true
	return out, nil
}

func (p *Proxy) DetectDockerVersion() string {
	req, err := http.NewRequest("GET", "http://docker/version", nil)
	if err != nil {
		return "unknown"
	}
	resp, err := p.httpClient.Do(req)
	if err != nil {
		return "unknown"
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	// Simple extraction — the version JSON contains "ApiVersion":"X.XX"
	s := string(body)
	idx := strings.Index(s, `"ApiVersion":"`)
	if idx < 0 {
		return "unknown"
	}
	s = s[idx+len(`"ApiVersion":"`):]
	end := strings.Index(s, `"`)
	if end < 0 {
		return "unknown"
	}
	return s[:end]
}

func (p *Proxy) SaveImage(ctx context.Context, ref string) (io.ReadCloser, error) {
	values := url.Values{}
	values.Set("names", ref)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://docker/images/get?"+values.Encode(), nil)
	if err != nil {
		return nil, fmt.Errorf("building image save request: %w", err)
	}
	resp, err := p.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("saving image %s: %w", ref, err)
	}
	if resp.StatusCode >= 300 {
		defer resp.Body.Close()
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return nil, fmt.Errorf("saving image %s returned status %d: %s", ref, resp.StatusCode, strings.TrimSpace(string(body)))
	}
	return resp.Body, nil
}

func (p *Proxy) LoadImage(ctx context.Context, body io.Reader) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "http://docker/images/load?quiet=1", body)
	if err != nil {
		return fmt.Errorf("building image load request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-tar")
	resp, err := p.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("loading image: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return fmt.Errorf("loading image returned status %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	if _, err := io.Copy(io.Discard, resp.Body); err != nil {
		return fmt.Errorf("reading image load response: %w", err)
	}
	return nil
}

func (p *Proxy) InspectContainer(ctx context.Context, containerID string) (DockerContainerInspect, error) {
	containerID = strings.TrimSpace(containerID)
	if containerID == "" {
		return DockerContainerInspect{}, fmt.Errorf("container id is required")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://docker/containers/"+url.PathEscape(containerID)+"/json", nil)
	if err != nil {
		return DockerContainerInspect{}, fmt.Errorf("building container inspect request: %w", err)
	}
	resp, err := p.httpClient.Do(req)
	if err != nil {
		return DockerContainerInspect{}, fmt.Errorf("inspecting container %s: %w", containerID, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return DockerContainerInspect{}, fmt.Errorf("inspecting container %s returned status %d: %s", containerID, resp.StatusCode, strings.TrimSpace(string(body)))
	}
	var inspect DockerContainerInspect
	if err := json.NewDecoder(resp.Body).Decode(&inspect); err != nil {
		return DockerContainerInspect{}, fmt.Errorf("decoding container inspect: %w", err)
	}
	return inspect, nil
}

// dockerContainerListEntry is a subset of the Docker /containers/json
// response we need to fall back to a name-based lookup when the plan's
// stored container id has rotated (typical of `docker compose up`).
type dockerContainerListEntry struct {
	ID    string   `json:"Id"`
	Names []string `json:"Names"`
}

// ListContainersByName returns the live id of the container whose name
// matches `name` (without the leading slash) or "" if no match is found.
// Used as the fallback path of resolveContainerForBackup when the
// stored container id no longer exists.
func (p *Proxy) ListContainersByName(ctx context.Context, name string) (string, error) {
	name = strings.TrimPrefix(strings.TrimSpace(name), "/")
	if name == "" {
		return "", nil
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://docker/containers/json?all=1", nil)
	if err != nil {
		return "", fmt.Errorf("building container list request: %w", err)
	}
	resp, err := p.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("listing containers: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return "", fmt.Errorf("listing containers returned status %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	var entries []dockerContainerListEntry
	if err := json.NewDecoder(resp.Body).Decode(&entries); err != nil {
		return "", fmt.Errorf("decoding container list: %w", err)
	}
	for _, entry := range entries {
		for _, rawName := range entry.Names {
			if strings.TrimPrefix(rawName, "/") == name {
				return entry.ID, nil
			}
		}
	}
	return "", nil
}

// resolveContainerForBackup looks up the live container for `payload`.
// It tries ContainerInspect first; on 404 (e.g. the stored id has been
// rotated by `docker compose up`) it falls back to a name-based lookup
// against ContainerList. The returned id is what subsequent pause/
// export/copy operations should use, and the original payload is
// unchanged so the manifest inside the archive keeps recording the
// plan's stored id for traceability.
func (p *Proxy) resolveContainerForBackup(ctx context.Context, payload BackupPayload) (DockerContainerInspect, string, error) {
	inspect, err := p.InspectContainer(ctx, payload.ContainerID)
	if err == nil {
		return inspect, payload.ContainerID, nil
	}
	if !isContainerNotFound(err) {
		return DockerContainerInspect{}, payload.ContainerID, err
	}
	if payload.ContainerName == "" {
		return DockerContainerInspect{}, payload.ContainerID, fmt.Errorf("inspecting container for backup: %w", err)
	}
	resolvedID, listErr := p.ListContainersByName(ctx, payload.ContainerName)
	if listErr != nil {
		return DockerContainerInspect{}, payload.ContainerID, fmt.Errorf("resolving container by name: %w", listErr)
	}
	if resolvedID == "" {
		return DockerContainerInspect{}, payload.ContainerID, fmt.Errorf("no live container named %q for backup", payload.ContainerName)
	}
	inspect, err = p.InspectContainer(ctx, resolvedID)
	if err != nil {
		return DockerContainerInspect{}, resolvedID, fmt.Errorf("inspecting resolved container %s: %w", resolvedID, err)
	}
	return inspect, resolvedID, nil
}

// isContainerNotFound reports whether `err` from InspectContainer is a
// 404 / "No such container" response. Anything else (network error,
// 500, decode failure, etc.) is treated as a hard error.
func isContainerNotFound(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "returned status 404") || strings.Contains(msg, "No such container")
}

func (p *Proxy) ContainerLogs(ctx context.Context, containerID string) (io.ReadCloser, error) {
	containerID = strings.TrimSpace(containerID)
	if containerID == "" {
		return nil, fmt.Errorf("container id is required")
	}
	values := url.Values{}
	values.Set("stdout", "1")
	values.Set("stderr", "1")
	values.Set("timestamps", "1")
	values.Set("tail", "all")
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://docker/containers/"+url.PathEscape(containerID)+"/logs?"+values.Encode(), nil)
	if err != nil {
		return nil, fmt.Errorf("building container logs request: %w", err)
	}
	resp, err := p.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("reading container logs: %w", err)
	}
	if resp.StatusCode >= 300 {
		defer resp.Body.Close()
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return nil, fmt.Errorf("reading container logs returned status %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	return resp.Body, nil
}

func (p *Proxy) ExportContainer(ctx context.Context, containerID string) (io.ReadCloser, error) {
	containerID = strings.TrimSpace(containerID)
	if containerID == "" {
		return nil, fmt.Errorf("container id is required")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://docker/containers/"+url.PathEscape(containerID)+"/export", nil)
	if err != nil {
		return nil, fmt.Errorf("building container export request: %w", err)
	}
	resp, err := p.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("exporting container filesystem: %w", err)
	}
	if resp.StatusCode >= 300 {
		defer resp.Body.Close()
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return nil, fmt.Errorf("exporting container filesystem returned status %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	return resp.Body, nil
}

// PauseContainer asks the local Docker daemon to pause the container so
// that subsequent snapshot reads (export, archive copy, logs) are
// point-in-time consistent.
//
// Pause is best-effort. Some runtimes (Windows containers, certain
// custom runtimes) reject pause with an error; the caller should treat
// the returned bool as "did we actually pause" and skip the matching
// unpause if not.
func (p *Proxy) PauseContainer(ctx context.Context, containerID string) error {
	containerID = strings.TrimSpace(containerID)
	if containerID == "" {
		return fmt.Errorf("container id is required")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "http://docker/containers/"+url.PathEscape(containerID)+"/pause", nil)
	if err != nil {
		return fmt.Errorf("building container pause request: %w", err)
	}
	resp, err := p.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("pausing container: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return fmt.Errorf("pausing container returned status %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	return nil
}

// UnpauseContainer releases a previously paused container.
func (p *Proxy) UnpauseContainer(ctx context.Context, containerID string) error {
	containerID = strings.TrimSpace(containerID)
	if containerID == "" {
		return fmt.Errorf("container id is required")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "http://docker/containers/"+url.PathEscape(containerID)+"/unpause", nil)
	if err != nil {
		return fmt.Errorf("building container unpause request: %w", err)
	}
	resp, err := p.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("unpausing container: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return fmt.Errorf("unpausing container returned status %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	return nil
}

func (p *Proxy) CopyArchiveFromContainer(ctx context.Context, containerID, sourcePath string) (io.ReadCloser, int64, error) {
	containerID = strings.TrimSpace(containerID)
	sourcePath = strings.TrimSpace(sourcePath)
	if containerID == "" {
		return nil, 0, fmt.Errorf("container id is required")
	}
	if sourcePath == "" {
		return nil, 0, fmt.Errorf("source path is required")
	}
	values := url.Values{}
	values.Set("path", sourcePath)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://docker/containers/"+url.PathEscape(containerID)+"/archive?"+values.Encode(), nil)
	if err != nil {
		return nil, 0, fmt.Errorf("building container archive copy request: %w", err)
	}
	resp, err := p.httpClient.Do(req)
	if err != nil {
		return nil, 0, fmt.Errorf("copying archive from container %s: %w", containerID, err)
	}
	if resp.StatusCode >= 300 {
		defer resp.Body.Close()
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return nil, 0, fmt.Errorf("copying archive from container %s returned status %d: %s", containerID, resp.StatusCode, strings.TrimSpace(string(body)))
	}
	return resp.Body, resp.ContentLength, nil
}

func (p *Proxy) CopyArchiveToContainer(ctx context.Context, containerID, targetPath string, body io.Reader, size int64) error {
	containerID = strings.TrimSpace(containerID)
	targetPath = strings.TrimSpace(targetPath)
	if containerID == "" {
		return fmt.Errorf("container id is required")
	}
	if targetPath == "" {
		targetPath = "/"
	}
	values := url.Values{}
	values.Set("path", targetPath)
	values.Set("noOverwriteDirNonDir", "1")
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, "http://docker/containers/"+url.PathEscape(containerID)+"/archive?"+values.Encode(), body)
	if err != nil {
		return fmt.Errorf("building container archive restore request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-tar")
	if size >= 0 {
		req.ContentLength = size
	}
	resp, err := p.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("restoring archive to container %s: %w", containerID, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return fmt.Errorf("restoring archive to container %s returned status %d: %s", containerID, resp.StatusCode, strings.TrimSpace(string(body)))
	}
	if _, err := io.Copy(io.Discard, resp.Body); err != nil {
		return fmt.Errorf("reading container archive restore response: %w", err)
	}
	return nil
}

func (p *Proxy) RemoveContainer(ctx context.Context, containerID string) error {
	containerID = strings.TrimSpace(containerID)
	if containerID == "" {
		return fmt.Errorf("container id is required")
	}
	values := url.Values{}
	values.Set("force", "1")
	values.Set("v", "0")
	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, "http://docker/containers/"+url.PathEscape(containerID)+"?"+values.Encode(), nil)
	if err != nil {
		return fmt.Errorf("building container remove request: %w", err)
	}
	resp, err := p.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("removing container %s: %w", containerID, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNoContent || resp.StatusCode == http.StatusNotFound {
		if _, err := io.Copy(io.Discard, resp.Body); err != nil {
			return fmt.Errorf("reading container remove response: %w", err)
		}
		return nil
	}
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
	return fmt.Errorf("removing container %s returned status %d: %s", containerID, resp.StatusCode, strings.TrimSpace(string(body)))
}

// HandleRequest processes a proxied Docker API request and sends the response back.
func (p *Proxy) HandleRequest(ctx context.Context, conn *websocket.Conn, id string, wsReq *WSHTTPRequest) {
	var bodyReader io.Reader
	if len(wsReq.Body) > 0 {
		bodyReader = bytes.NewReader(wsReq.Body)
	}
	p.handleRequestWithBody(ctx, conn, id, wsReq, bodyReader)
}

// HandleRequestStream processes a proxied Docker API request with a streamed body.
func (p *Proxy) HandleRequestStream(ctx context.Context, conn *websocket.Conn, id string, wsReq *WSHTTPRequest, bodyReader io.Reader) {
	p.handleRequestWithBody(ctx, conn, id, wsReq, bodyReader)
}

func (p *Proxy) handleRequestWithBody(ctx context.Context, conn *websocket.Conn, id string, wsReq *WSHTTPRequest, bodyReader io.Reader) {
	var cleanup func()
	if shouldSpoolRequestBody(wsReq, bodyReader) {
		spooledBody, bodyCleanup, err := p.spoolRequestBody(bodyReader)
		if err != nil {
			p.sendErrorResponse(conn, id, http.StatusBadGateway, err)
			return
		}
		bodyReader = spooledBody
		cleanup = bodyCleanup
	}
	if cleanup != nil {
		defer cleanup()
	}

	// Build the real HTTP request
	urlStr := fmt.Sprintf("http://docker%s", wsReq.Path)
	if wsReq.Query != "" {
		urlStr += "?" + wsReq.Query
	}

	req, err := http.NewRequestWithContext(ctx, wsReq.Method, urlStr, bodyReader)
	if err != nil {
		p.sendErrorResponse(conn, id, http.StatusBadGateway, err)
		return
	}
	if bodyFile, ok := bodyReader.(*os.File); ok {
		if stat, statErr := bodyFile.Stat(); statErr == nil {
			req.ContentLength = stat.Size()
		}
	}

	for k, v := range wsReq.Headers {
		req.Header.Set(k, v)
	}

	resp, err := p.httpClient.Do(req)
	if err != nil {
		p.sendErrorResponse(conn, id, http.StatusBadGateway, err)
		return
	}
	defer resp.Body.Close()

	// Check if this is a streaming response
	if isStreamingResponse(wsReq, resp) {
		p.handleStreamingResponse(ctx, conn, id, resp)
		return
	}

	// Regular response — read full body
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		p.sendErrorResponse(conn, id, http.StatusBadGateway, err)
		return
	}

	headers := make(map[string]string)
	for k, v := range resp.Header {
		if len(v) > 0 {
			headers[k] = v[0]
		}
	}

	msg := WSMessage{
		Type: MsgHTTPResponse,
		ID:   id,
		HTTPResponse: &WSHTTPResponse{
			StatusCode: resp.StatusCode,
			Headers:    headers,
			Body:       body,
		},
	}

	writeMu.Lock()
	conn.WriteJSON(msg)
	writeMu.Unlock()
}

// handleStreamingResponse sends a streaming response back in chunks.
func (p *Proxy) handleStreamingResponse(ctx context.Context, conn *websocket.Conn, id string, resp *http.Response) {
	headers := make(map[string]string)
	for k, v := range resp.Header {
		if len(v) > 0 {
			headers[k] = v[0]
		}
	}

	// Send stream start
	startMsg := WSMessage{
		Type: MsgHTTPResponseStart,
		ID:   id,
		StreamStart: &WSStreamStart{
			StatusCode: resp.StatusCode,
			Headers:    headers,
		},
	}
	writeMu.Lock()
	conn.WriteJSON(startMsg)
	writeMu.Unlock()

	// Stream chunks
	buf := make([]byte, 32*1024)
streamLoop:
	for {
		select {
		case <-ctx.Done():
			break streamLoop
		default:
		}

		n, err := resp.Body.Read(buf)
		if n > 0 {
			chunk := make([]byte, n)
			copy(chunk, buf[:n])

			chunkMsg := WSMessage{
				Type: MsgHTTPResponseChunk,
				ID:   id,
				StreamChunk: &WSStreamChunk{
					Data: chunk,
				},
			}
			writeMu.Lock()
			writeErr := conn.WriteJSON(chunkMsg)
			writeMu.Unlock()
			if writeErr != nil {
				return
			}
		}
		if err != nil {
			break
		}
	}

	// Send stream end
	endMsg := WSMessage{Type: MsgHTTPResponseEnd, ID: id}
	writeMu.Lock()
	conn.WriteJSON(endMsg)
	writeMu.Unlock()
}

// sendErrorResponse sends an error response for a failed request.
func (p *Proxy) sendErrorResponse(conn *websocket.Conn, id string, status int, err error) {
	p.logger.Error("proxy error", "id", id, "error", err)

	body := fmt.Sprintf(`{"message":"%s"}`, err.Error())
	msg := WSMessage{
		Type: MsgHTTPResponse,
		ID:   id,
		HTTPResponse: &WSHTTPResponse{
			StatusCode: status,
			Headers:    map[string]string{"Content-Type": "application/json"},
			Body:       []byte(body),
		},
	}
	writeMu.Lock()
	conn.WriteJSON(msg)
	writeMu.Unlock()
}

func shouldSpoolRequestBody(wsReq *WSHTTPRequest, bodyReader io.Reader) bool {
	if wsReq == nil || bodyReader == nil {
		return false
	}
	if _, ok := bodyReader.(*os.File); ok {
		return false
	}
	return strings.HasSuffix(wsReq.Path, "/images/load")
}

func (p *Proxy) spoolRequestBody(bodyReader io.Reader) (io.Reader, func(), error) {
	archiveFile, err := os.CreateTemp("", "mcharbor-agent-image-load-*.tar")
	if err != nil {
		return nil, nil, fmt.Errorf("creating temporary image archive: %w", err)
	}
	archivePath := archiveFile.Name()
	cleanup := func() {
		if err := archiveFile.Close(); err != nil {
			p.logger.Warn("close temporary image archive failed", "error", err, "path", archivePath)
		}
		if err := os.Remove(archivePath); err != nil && !os.IsNotExist(err) {
			p.logger.Warn("remove temporary image archive failed", "error", err, "path", archivePath)
		}
	}
	if _, err := io.Copy(archiveFile, bodyReader); err != nil {
		cleanup()
		return nil, nil, fmt.Errorf("spooling image archive: %w", err)
	}
	if _, err := archiveFile.Seek(0, io.SeekStart); err != nil {
		cleanup()
		return nil, nil, fmt.Errorf("rewinding image archive: %w", err)
	}
	return archiveFile, cleanup, nil
}

// isStreamingResponse checks if the response is a streaming/chunked response.
func isStreamingResponse(req *WSHTTPRequest, resp *http.Response) bool {
	ct := resp.Header.Get("Content-Type")

	if isOneShotStatsRequest(req) {
		return false
	}

	// Go's http.Transport moves Transfer-Encoding from headers to
	// resp.TransferEncoding field, so check that instead of the header.
	for _, te := range resp.TransferEncoding {
		if strings.EqualFold(te, "chunked") {
			return true
		}
	}

	// Unknown content length usually means streaming
	if resp.ContentLength < 0 {
		return true
	}

	// Multiplexed streams
	if ct == "application/vnd.docker.raw-stream" || ct == "application/vnd.docker.multiplexed-stream" {
		return true
	}
	// Tar archives (container filesystem copy)
	if ct == "application/x-tar" || ct == "application/octet-stream" {
		return true
	}
	return false
}

func isOneShotStatsRequest(req *WSHTTPRequest) bool {
	if req == nil {
		return false
	}
	if req.Method != http.MethodGet || !strings.Contains(req.Path, "/containers/") || !strings.HasSuffix(req.Path, "/stats") {
		return false
	}

	values, err := url.ParseQuery(req.Query)
	if err != nil {
		return false
	}
	stream := strings.ToLower(strings.TrimSpace(values.Get("stream")))
	return stream == "0" || stream == "false"
}

// HandleExec starts an exec attach session and streams I/O over WebSocket.
func (p *Proxy) HandleExec(ctx context.Context, wsConn *websocket.Conn, sessionID, execID string) {
	rawConn, reader, err := p.rawExecAttach(execID)
	if err != nil {
		p.logger.Error("exec attach failed", "sessionID", sessionID, "error", err)
		endMsg := WSMessage{Type: MsgExecEnd, ID: sessionID}
		writeMu.Lock()
		wsConn.WriteJSON(endMsg)
		writeMu.Unlock()
		return
	}
	defer rawConn.Close()
	done := make(chan struct{})
	go func() {
		select {
		case <-ctx.Done():
			rawConn.Close()
		case <-done:
		}
	}()
	defer close(done)

	// Register connection for input forwarding
	p.execMu.Lock()
	p.execConns[sessionID] = rawConn
	p.execMu.Unlock()
	defer func() {
		p.execMu.Lock()
		delete(p.execConns, sessionID)
		p.execMu.Unlock()
	}()

	// Read Docker stdout and send as exec_output
	buf := make([]byte, 4096)
execLoop:
	for {
		select {
		case <-ctx.Done():
			break execLoop
		default:
		}

		n, readErr := reader.Read(buf)
		if n > 0 {
			chunk := make([]byte, n)
			copy(chunk, buf[:n])

			msg := WSMessage{
				Type:        MsgExecOutput,
				ID:          sessionID,
				StreamChunk: &WSStreamChunk{Data: chunk},
			}
			writeMu.Lock()
			writeErr := wsConn.WriteJSON(msg)
			writeMu.Unlock()
			if writeErr != nil {
				return
			}
		}
		if readErr != nil {
			break
		}
	}

	// Send exec_end
	endMsg := WSMessage{Type: MsgExecEnd, ID: sessionID}
	writeMu.Lock()
	wsConn.WriteJSON(endMsg)
	writeMu.Unlock()
}

// WriteExecInput writes stdin data to an active exec session.
func (p *Proxy) WriteExecInput(sessionID string, data []byte) {
	p.execMu.RLock()
	conn, ok := p.execConns[sessionID]
	p.execMu.RUnlock()
	if ok {
		conn.Write(data)
	}
}

// ResizeExec resizes an exec session terminal via the Docker API.
func (p *Proxy) ResizeExec(execID string, cols, rows uint) {
	url := fmt.Sprintf("http://docker/exec/%s/resize?h=%d&w=%d", execID, rows, cols)
	req, err := http.NewRequest("POST", url, nil)
	if err != nil {
		p.logger.Warn("exec resize request failed", "execID", execID, "error", err)
		return
	}
	resp, err := p.httpClient.Do(req)
	if err != nil {
		p.logger.Warn("exec resize failed", "execID", execID, "error", err)
		return
	}
	defer resp.Body.Close()
	if _, err := io.Copy(io.Discard, resp.Body); err != nil {
		p.logger.Warn("exec resize response read failed", "execID", execID, "error", err)
	}
}

// CloseExec closes an active exec session.
func (p *Proxy) CloseExec(sessionID string) {
	p.execMu.Lock()
	conn, ok := p.execConns[sessionID]
	delete(p.execConns, sessionID)
	p.execMu.Unlock()
	if ok {
		conn.Close()
	}
}

// rawExecAttach performs a raw HTTP exec attach to the Docker socket.
func (p *Proxy) rawExecAttach(execID string) (net.Conn, *bufio.Reader, error) {
	var conn net.Conn
	var err error

	if strings.HasPrefix(p.dockerHost, "unix://") {
		socketPath := strings.TrimPrefix(p.dockerHost, "unix://")
		conn, err = net.Dial("unix", socketPath)
	} else if strings.HasPrefix(p.dockerHost, "tcp://") {
		addr := strings.TrimPrefix(p.dockerHost, "tcp://")
		conn, err = net.Dial("tcp", addr)
	} else {
		// Try unix socket at the raw path
		conn, err = net.Dial("unix", p.dockerHost)
	}

	if err != nil {
		return nil, nil, fmt.Errorf("dialing Docker: %w", err)
	}

	body := `{"Detach":false,"Tty":true}`
	req := fmt.Sprintf(
		"POST /exec/%s/start HTTP/1.1\r\n"+
			"Host: docker\r\n"+
			"Content-Type: application/json\r\n"+
			"Connection: Upgrade\r\n"+
			"Upgrade: tcp\r\n"+
			"Content-Length: %d\r\n"+
			"\r\n%s",
		execID, len(body), body,
	)

	if _, err = conn.Write([]byte(req)); err != nil {
		conn.Close()
		return nil, nil, fmt.Errorf("writing request: %w", err)
	}

	reader := bufio.NewReader(conn)
	resp, err := http.ReadResponse(reader, nil)
	if err != nil {
		conn.Close()
		return nil, nil, fmt.Errorf("reading response: %w", err)
	}

	if resp.StatusCode != http.StatusSwitchingProtocols {
		respBody, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		conn.Close()
		return nil, nil, fmt.Errorf("unexpected status %d: %s", resp.StatusCode, string(respBody))
	}

	return conn, reader, nil
}

// writeMu protects concurrent WebSocket writes from the proxy.
var writeMu sync.Mutex
