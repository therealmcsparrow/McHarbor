// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/gorilla/websocket"
	"github.com/therealmcsparrow/mcharbor-agent/internal/errlog"
)

const (
	agentUpdateModeContainer = "container"
	agentUpdateModeBinary    = "binary"
	agentUpdateOldPrefix     = "mcharbor-agent-old-"
	agentBinaryMinSize       = 1 << 20 // a real agent binary is several MB
	retireContainerEnv       = "MCHARBOR_RETIRE_CONTAINER_ID"
	// agentUpdateDownloadTimeout bounds one download from McHarbor; an image
	// tarball is a hundred megabytes or more over what may be a slow link.
	agentUpdateDownloadTimeout = 10 * time.Minute
	// imageLoadTimeout bounds `docker load`, which unpacks every layer.
	imageLoadTimeout = 10 * time.Minute
)

// errNoUpdateArtifact is the refusal when McHarbor offered nothing for this
// agent's shape. It names the fix, because there is no public fallback left.
func errNoUpdateArtifact(kind string) error {
	return fmt.Errorf("McHarbor offered no agent %s for %s/%s; download a release under Settings > Updates > Agents, or build one locally",
		kind, runtime.GOOS, runtime.GOARCH)
}

// handleAgentUpdateRequest replaces the running agent with the requested
// version. Inside a container it downloads the image tarball McHarbor serves,
// loads it through the local Docker API and starts a replacement container
// (which removes this one on boot via MCHARBOR_RETIRE_CONTAINER_ID); as a bare
// binary it downloads the binary McHarbor serves, swaps it in place and
// re-execs. Nothing is ever fetched from GitHub or a registry. The response is
// written before the current process goes away so the server learns the outcome.
func (a *Agent) handleAgentUpdateRequest(ctx context.Context, conn *websocket.Conn, id string, payload AgentUpdatePayload) {
	version := strings.TrimPrefix(strings.TrimSpace(payload.Version), "v")
	if version == "" {
		writeAgentUpdateResponse(conn, id, AgentUpdatePayload{Success: false, Error: "target version is required"})
		return
	}
	// The build is part of the identity, not decoration. Comparing only the
	// version made every rebuild a no-op that still answered "success": the
	// server recorded a replacement, the table said so, and nothing had moved.
	// An empty target build means the server has no local artifact to offer, so
	// the version is all there is to go on.
	target := strings.TrimSpace(payload.Build)
	if version == agentVersion && (target == "" || target == agentBuild) {
		writeAgentUpdateResponse(conn, id, AgentUpdatePayload{
			Success: true, Version: version, Message: "agent already runs " + agentVersion +
				" (" + agentBuild + ")",
		})
		return
	}

	self, err := a.proxy.selfContainer(ctx)
	if err == nil {
		a.logger.Info("agent update: container mode", "container", self.ID, "target", version)
		msg, err := a.updateContainer(ctx, self, payload)
		if err != nil {
			a.logger.Warn("agent update failed", "mode", agentUpdateModeContainer, "error", err)
			writeAgentUpdateResponse(conn, id, AgentUpdatePayload{
				Success: false, Version: version, Mode: agentUpdateModeContainer, Error: err.Error(),
			})
			return
		}
		writeAgentUpdateResponse(conn, id, AgentUpdatePayload{
			Success: true, Version: version, Mode: agentUpdateModeContainer, Message: msg,
		})
		return
	}
	a.logger.Debug("agent update: not running as a container", "reason", err)

	exe, err := a.updateBinary(ctx, payload.BinaryURL, payload.BinarySHA256)
	if err != nil {
		a.logger.Warn("agent update failed", "mode", agentUpdateModeBinary, "error", err)
		writeAgentUpdateResponse(conn, id, AgentUpdatePayload{
			Success: false, Version: version, Mode: agentUpdateModeBinary, Error: err.Error(),
		})
		return
	}
	writeAgentUpdateResponse(conn, id, AgentUpdatePayload{
		Success: true, Version: version, Mode: agentUpdateModeBinary,
		Message: "binary replaced at " + exe + "; restarting",
	})
	// Give the response a moment to flush, then replace the process image.
	go func() {
		time.Sleep(1500 * time.Millisecond)
		a.logger.Info("agent update: restarting into new binary", "path", exe)
		if err := reexecSelf(exe); err != nil {
			a.logger.Error("agent update: re-exec failed; exiting so the supervisor restarts us", "error", err)
			os.Exit(0)
		}
	}()
}

func writeAgentUpdateResponse(conn *websocket.Conn, id string, payload AgentUpdatePayload) {
	writeMu.Lock()
	defer writeMu.Unlock()
	errlog.Log(conn.WriteJSON(WSMessage{Type: MsgAgentUpdateResponse, ID: id, AgentUpdate: &payload}))
}

// mcharborURL resolves a download URL the server sent and refuses anything that
// does not point at the McHarbor this agent is connected to. That refusal is the
// whole guarantee: a compromised or confused request cannot send the agent to
// GitHub, a registry or anywhere else for its replacement.
func (a *Agent) mcharborURL(raw, kind string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", errNoUpdateArtifact(kind)
	}
	if !strings.Contains(raw, "://") {
		// A server-relative path, the same shape the import pull and the backup
		// archive use. Only the agent knows which of McHarbor's addresses this
		// host can reach, so the server may send a path and the agent joins it.
		resolved, err := a.resolveServerURL(raw)
		if err != nil {
			return "", fmt.Errorf("resolving the update download URL: %w", err)
		}
		raw = resolved
	}
	if !a.isMcHarborURL(raw) {
		return "", fmt.Errorf("refusing to download an agent update from %q: agents update only from the McHarbor they are connected to", raw)
	}
	// A plain-http McHarbor is allowed: the agent is already taking its orders
	// over that same connection, so refusing to download from it would protect
	// nothing while making a LAN install unable to update at all.
	return raw, nil
}

// downloadFromMcHarbor streams rawURL into a new file in dir while hashing it,
// and removes the file unless the digest matches wantSHA (when one was given).
// The agent token is sent so authenticated routes such as the image work.
func (a *Agent) downloadFromMcHarbor(ctx context.Context, rawURL, dir, pattern, wantSHA string) (string, int64, error) {
	dlCtx, cancel := context.WithTimeout(ctx, agentUpdateDownloadTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(dlCtx, http.MethodGet, rawURL, nil)
	if err != nil {
		return "", 0, fmt.Errorf("building download request: %w", err)
	}
	req.Header.Set("User-Agent", "mcharbor-agent/"+agentVersion)
	req.Header.Set("Authorization", "Bearer "+a.cfg.AgentToken)
	// The agent's own client, not http.DefaultClient: McHarbor is commonly
	// reached on a LAN address its certificate does not cover, and that has
	// already been accepted once for the connection carrying this very request.
	resp, err := a.downloadClient().Do(req)
	if err != nil {
		return "", 0, fmt.Errorf("downloading %s: %w", rawURL, err)
	}
	defer func() { errlog.Log(resp.Body.Close()) }()
	if resp.StatusCode != http.StatusOK {
		return "", 0, fmt.Errorf("download returned status %d for %s", resp.StatusCode, rawURL)
	}
	tmp, err := os.CreateTemp(dir, pattern)
	if err != nil {
		return "", 0, fmt.Errorf("creating temp file in %s: %w", dir, err)
	}
	path := tmp.Name()
	h := sha256.New()
	n, err := io.Copy(io.MultiWriter(tmp, h), resp.Body)
	if closeErr := tmp.Close(); err == nil && closeErr != nil {
		err = closeErr
	}
	if err == nil && wantSHA != "" && !strings.EqualFold(hex.EncodeToString(h.Sum(nil)), strings.TrimSpace(wantSHA)) {
		err = fmt.Errorf("the download from %s does not match the checksum McHarbor sent", rawURL)
	}
	if err != nil {
		errlog.Log(os.Remove(path))
		return "", 0, err
	}
	return path, n, nil
}

// --- container mode ---------------------------------------------------

// rawContainer is the subset of `GET /containers/{id}/json` needed to clone
// the running agent. Config and HostConfig are kept as raw JSON so every
// option the operator set (binds, ports, restart policy, labels, ...)
// survives the round trip untouched.
type rawContainer struct {
	ID              string                     `json:"Id"`
	Name            string                     `json:"Name"`
	Config          map[string]json.RawMessage `json:"Config"`
	HostConfig      json.RawMessage            `json:"HostConfig"`
	NetworkSettings struct {
		Networks map[string]json.RawMessage `json:"Networks"`
	} `json:"NetworkSettings"`
}

// selfContainer inspects the container this process runs in. Docker sets
// the container hostname to the short container id, so inspecting the
// hostname succeeds only inside a container that can see the socket.
func (p *Proxy) selfContainer(ctx context.Context) (rawContainer, error) {
	hostname, err := os.Hostname()
	if err != nil || hostname == "" {
		return rawContainer{}, fmt.Errorf("hostname unavailable")
	}
	if _, statErr := os.Stat("/.dockerenv"); statErr != nil {
		return rawContainer{}, fmt.Errorf("no /.dockerenv marker")
	}
	body, err := p.dockerJSON(ctx, http.MethodGet, "/containers/"+url.PathEscape(hostname)+"/json", nil)
	if err != nil {
		return rawContainer{}, err
	}
	var c rawContainer
	if err := json.Unmarshal(body, &c); err != nil {
		return rawContainer{}, fmt.Errorf("decoding self inspect: %w", err)
	}
	if c.ID == "" {
		return rawContainer{}, fmt.Errorf("self inspect returned no id")
	}
	return c, nil
}

// updateContainer loads the image McHarbor serves and starts a replacement
// container from it. There is no registry path: an update without an image
// URL is refused with the fix spelled out.
func (a *Agent) updateContainer(ctx context.Context, self rawContainer, payload AgentUpdatePayload) (string, error) {
	target, err := a.loadImageFromMcHarbor(ctx, payload)
	if err != nil {
		return "", err
	}

	name := strings.TrimPrefix(self.Name, "/")
	oldName := agentUpdateOldPrefix + fmt.Sprintf("%d", time.Now().Unix())
	if err := a.proxy.renameContainer(ctx, self.ID, oldName); err != nil {
		return "", fmt.Errorf("renaming current container: %w", err)
	}

	newID, err := a.proxy.createReplacement(ctx, self, name, target)
	if err != nil {
		if rbErr := a.proxy.renameContainer(ctx, self.ID, name); rbErr != nil {
			a.logger.Warn("agent update: rollback rename failed", "error", rbErr)
		}
		return "", fmt.Errorf("creating replacement container: %w", err)
	}
	if err := a.proxy.startContainer(ctx, newID); err != nil {
		rmCtx, cancelRm := context.WithTimeout(ctx, hostOpTimeout)
		defer cancelRm()
		if rmErr := a.proxy.RemoveContainer(rmCtx, newID); rmErr != nil {
			a.logger.Warn("agent update: rollback remove failed", "error", rmErr)
		}
		if rbErr := a.proxy.renameContainer(ctx, self.ID, name); rbErr != nil {
			a.logger.Warn("agent update: rollback rename failed", "error", rbErr)
		}
		return "", fmt.Errorf("starting replacement container: %w", err)
	}
	return fmt.Sprintf("replacement container %s started from %s; this container will be retired", name, target), nil
}

// loadImageFromMcHarbor downloads the image tarball, checks its digest, loads
// it through this host's own Docker API and confirms the named reference exists.
func (a *Agent) loadImageFromMcHarbor(ctx context.Context, payload AgentUpdatePayload) (string, error) {
	ref := strings.TrimSpace(payload.ImageRef)
	if strings.TrimSpace(payload.ImageURL) == "" || ref == "" {
		return "", errNoUpdateArtifact("image")
	}
	imageURL, err := a.mcharborURL(payload.ImageURL, "image")
	if err != nil {
		return "", err
	}
	path, _, err := a.downloadFromMcHarbor(ctx, imageURL, os.TempDir(), ".mcharbor-agent-image-*.tar", payload.ImageSHA256)
	if err != nil {
		return "", err
	}
	defer func() {
		if rmErr := os.Remove(path); rmErr != nil && !errors.Is(rmErr, os.ErrNotExist) {
			a.logger.Warn("agent update: removing image tarball", "path", path, "error", rmErr)
		}
	}()
	loadCtx, cancel := context.WithTimeout(ctx, imageLoadTimeout)
	defer cancel()
	if err := a.proxy.loadImage(loadCtx, path); err != nil {
		return "", fmt.Errorf("loading the agent image: %w", err)
	}
	inspectCtx, cancelInspect := context.WithTimeout(ctx, hostOpTimeout)
	defer cancelInspect()
	if _, err := a.proxy.dockerJSON(inspectCtx, http.MethodGet, "/images/"+url.PathEscape(ref)+"/json", nil); err != nil {
		return "", fmt.Errorf("the loaded tarball does not contain %s: %w", ref, err)
	}
	return ref, nil
}

// loadImage streams a `docker save` tarball into `POST /images/load` and reads
// the progress stream, where the daemon reports a failed load.
func (p *Proxy) loadImage(ctx context.Context, path string) error {
	f, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("opening %s: %w", path, err)
	}
	defer errlog.Close(f)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "http://docker/images/load?quiet=1", f)
	if err != nil {
		return fmt.Errorf("building load request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-tar")
	resp, err := p.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("load request: %w", err)
	}
	defer func() { errlog.Log(resp.Body.Close()) }()
	if resp.StatusCode >= 300 {
		body := errlog.ReadAll(io.LimitReader(resp.Body, 4096))
		return fmt.Errorf("load returned status %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	dec := json.NewDecoder(resp.Body)
	for {
		var line struct {
			Error string `json:"error"`
		}
		if err := dec.Decode(&line); err != nil {
			if errors.Is(err, io.EOF) {
				return nil
			}
			return fmt.Errorf("reading load progress: %w", err)
		}
		if line.Error != "" {
			return errors.New(line.Error)
		}
	}
}

// createReplacement clones the running container's Config/HostConfig with
// the new image and a retire marker pointing at the current container.
func (p *Proxy) createReplacement(ctx context.Context, self rawContainer, name, image string) (string, error) {
	cfg := make(map[string]json.RawMessage, len(self.Config)+2)
	for k, v := range self.Config {
		cfg[k] = v
	}
	imageJSON, err := json.Marshal(image)
	if err != nil {
		return "", fmt.Errorf("encoding image reference: %w", err)
	}
	cfg["Image"] = imageJSON
	// Let Docker assign a fresh hostname; the old one is the old short id.
	delete(cfg, "Hostname")

	var env []string
	if raw, ok := cfg["Env"]; ok {
		errlog.Log(json.Unmarshal(raw, &env))
	}
	kept := env[:0]
	for _, kv := range env {
		if !strings.HasPrefix(kv, retireContainerEnv+"=") {
			kept = append(kept, kv)
		}
	}
	kept = append(kept, retireContainerEnv+"="+self.ID)
	envJSON, err := json.Marshal(kept)
	if err != nil {
		return "", fmt.Errorf("encoding container environment: %w", err)
	}
	cfg["Env"] = envJSON

	cfg["HostConfig"] = self.HostConfig

	// Re-attach a single user-defined network; the default drivers are
	// implied by HostConfig.NetworkMode and need no endpoint config.
	networkMode := ""
	var hc struct {
		NetworkMode string `json:"NetworkMode"`
	}
	errlog.Log(json.Unmarshal(self.HostConfig, &hc))
	networkMode = hc.NetworkMode
	switch {
	case networkMode == "", networkMode == "default", networkMode == "bridge",
		networkMode == "host", networkMode == "none", strings.HasPrefix(networkMode, "container:"):
	default:
		if _, ok := self.NetworkSettings.Networks[networkMode]; ok {
			endpoints := map[string]any{"EndpointsConfig": map[string]any{networkMode: map[string]any{}}}
			endpointsJSON, err := json.Marshal(endpoints)
			if err != nil {
				return "", fmt.Errorf("encoding network endpoints: %w", err)
			}
			cfg["NetworkingConfig"] = endpointsJSON
		}
	}

	body, err := json.Marshal(cfg)
	if err != nil {
		return "", fmt.Errorf("encoding create body: %w", err)
	}
	resp, err := p.dockerJSON(ctx, http.MethodPost, "/containers/create?name="+url.QueryEscape(name), body)
	if err != nil {
		return "", err
	}
	var created struct {
		ID string `json:"Id"`
	}
	if err := json.Unmarshal(resp, &created); err != nil {
		return "", fmt.Errorf("decoding create response: %w", err)
	}
	if created.ID == "" {
		return "", fmt.Errorf("create returned no container id")
	}
	return created.ID, nil
}

func (p *Proxy) renameContainer(ctx context.Context, id, name string) error {
	_, err := p.dockerJSON(ctx, http.MethodPost, "/containers/"+url.PathEscape(id)+"/rename?name="+url.QueryEscape(name), nil)
	return err
}

func (p *Proxy) startContainer(ctx context.Context, id string) error {
	_, err := p.dockerJSON(ctx, http.MethodPost, "/containers/"+url.PathEscape(id)+"/start", nil)
	return err
}

// dockerJSON performs one Docker API call and returns the response body,
// treating any non-2xx status as an error carrying the daemon's message.
func (p *Proxy) dockerJSON(ctx context.Context, method, path string, body []byte) ([]byte, error) {
	var reader io.Reader
	if body != nil {
		reader = bytes.NewReader(body)
	}
	req, err := http.NewRequestWithContext(ctx, method, "http://docker"+path, reader)
	if err != nil {
		return nil, fmt.Errorf("building docker request: %w", err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := p.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("docker %s %s: %w", method, path, err)
	}
	defer func() { errlog.Log(resp.Body.Close()) }()
	data, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return nil, fmt.Errorf("reading docker response: %w", err)
	}
	if resp.StatusCode >= 300 {
		msg := strings.TrimSpace(string(data))
		var daemonErr struct {
			Message string `json:"message"`
		}
		if json.Unmarshal(data, &daemonErr) == nil && daemonErr.Message != "" {
			msg = daemonErr.Message
		}
		return nil, fmt.Errorf("docker %s %s returned %d: %s", method, path, resp.StatusCode, msg)
	}
	return data, nil
}

// --- binary mode ------------------------------------------------------

// updateBinary downloads the binary McHarbor serves next to the running
// executable, checks its digest when McHarbor sent one, and atomically renames
// it into place. Returns the executable path so the caller can re-exec it.
func (a *Agent) updateBinary(ctx context.Context, rawURL, wantSHA string) (string, error) {
	downloadURL, err := a.mcharborURL(rawURL, "binary")
	if err != nil {
		return "", err
	}
	exe, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("resolving executable: %w", err)
	}
	if resolved, err := filepath.EvalSymlinks(exe); err == nil {
		exe = resolved
	}
	tmpPath, n, err := a.downloadFromMcHarbor(ctx, downloadURL, filepath.Dir(exe), ".mcharbor-agent-update-*", wantSHA)
	if err != nil {
		return "", err
	}
	cleanup := func() { errlog.Log(os.Remove(tmpPath)) }
	if n < agentBinaryMinSize {
		cleanup()
		return "", fmt.Errorf("downloaded file is too small (%d bytes) to be an agent binary", n)
	}
	if err := os.Chmod(tmpPath, 0o755); err != nil {
		cleanup()
		return "", fmt.Errorf("chmod new binary: %w", err)
	}
	if err := replaceExecutable(tmpPath, exe); err != nil {
		cleanup()
		return "", fmt.Errorf("replacing executable: %w", err)
	}
	if err := ensureCLILink(exe); err != nil {
		a.logger.Warn("agent updated, but the mcharbor CLI link was not made", "error", err)
	}
	return exe, nil
}

// isMcHarborURL reports whether value addresses the McHarbor this agent is
// connected to.
//
// Host and port only: the scheme is exactly what is in question, since the
// point is to allow a plain-http download from a server whose control channel
// is already plain ws. Anything else still has to be https.
func (a *Agent) isMcHarborURL(value string) bool {
	if value == "" {
		return false
	}
	target, err := url.Parse(value)
	if err != nil || target.Host == "" {
		return false
	}
	base, err := url.Parse(a.cfg.McHarborURL)
	if err != nil || base.Host == "" {
		return false
	}
	return strings.EqualFold(target.Host, base.Host)
}

// downloadClient is the HTTP client an update download uses.
//
// Falls back to a fresh client rather than http.DefaultClient so the timeout
// and the MCHARBOR_INSECURE contract hold even before the agent's own client
// has been built.
func (a *Agent) downloadClient() *http.Client {
	if a.httpClient != nil {
		return a.httpClient
	}
	return newAgentHTTPClient(a.cfg.Insecure, a.cfg.Pin, 0)
}
