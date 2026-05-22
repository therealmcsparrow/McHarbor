// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package workflows

import (
	"archive/tar"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/docker/docker/api/types/build"
	"golang.org/x/crypto/ssh"
)

func (s *Service) executeHTTPRequest(ctx context.Context, node *CanvasNode, msg Msg) (string, Msg, error) {
	urlStr, _ := node.Config["url"].(string)
	method, _ := node.Config["method"].(string)
	if method == "" {
		method = http.MethodGet
	}
	if strings.TrimSpace(urlStr) == "" {
		return "", nil, fmt.Errorf("url is required")
	}

	headers := configMap(node.Config["headers"])
	bodySource, _ := node.Config["body_source"].(string)
	if bodySource == "" {
		bodySource = "payload"
	}

	bodyBytes := []byte(nil)
	if raw, ok := GetPath(msg, bodySource); ok {
		bodyBytes = encodeWorkflowBody(raw)
	}

	reqCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(reqCtx, strings.ToUpper(method), urlStr, bytes.NewReader(bodyBytes))
	if err != nil {
		return "", nil, fmt.Errorf("creating http request: %w", err)
	}
	for k, v := range headers {
		req.Header.Set(k, fmt.Sprintf("%v", v))
	}
	if req.Header.Get("Content-Type") == "" && len(bodyBytes) > 0 {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		out := CloneMsg(msg)
		out = EnsureMsgID(out)
		out["payload"] = map[string]interface{}{"error": "http request failed", "url": urlStr, "method": method}
		return "error", out, nil
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	parsedBody := decodeWorkflowBody(resp.Header.Get("Content-Type"), respBody)

	out := CloneMsg(msg)
	out = EnsureMsgID(out)
	out["payload"] = parsedBody
	out["statusCode"] = resp.StatusCode
	out["headers"] = flattenHeaders(resp.Header)
	out["_request"] = map[string]interface{}{"url": urlStr, "method": strings.ToUpper(method)}
	if resp.StatusCode >= 400 {
		return "error", out, nil
	}
	return "output", out, nil
}

func (s *Service) executeLoopRuntime(node *CanvasNode, msg Msg, flowCtx *FlowContext) (string, Msg, error) {
	itemsField, _ := node.Config["items_field"].(string)
	if itemsField == "" {
		itemsField = "payload"
	}
	summary, emissions, err := splitMessagesForPath(node.ID, msg, itemsField, "done")
	if err != nil {
		return "error", summary, nil
	}
	if flowCtx != nil && flowCtx.Runtime != nil {
		flowCtx.Runtime.setEmissions(node.ID, emissions)
	}
	return "done", summary, nil
}

func (s *Service) executeRangeRuntime(node *CanvasNode, msg Msg, flowCtx *FlowContext) (string, Msg, error) {
	property, _ := node.Config["property"].(string)
	if property == "" {
		property = "payload"
	}
	summary, emissions, err := splitMessagesForPath(node.ID, msg, property, "output")
	if err != nil {
		return "error", summary, nil
	}
	if flowCtx != nil && flowCtx.Runtime != nil {
		flowCtx.Runtime.setEmissions(node.ID, emissions)
	}
	return "output", summary, nil
}

func (s *Service) executeJoinRuntime(node *CanvasNode, msg Msg, flowCtx *FlowContext) (string, Msg, error) {
	if flowCtx == nil || flowCtx.Runtime == nil {
		return "output", msg, nil
	}

	inputCount := int(configFloat(node.Config, "input_count", 2))
	if inputCount < 1 {
		inputCount = 1
	}
	buffer := append(flowCtx.Runtime.joinBuffers[node.ID], DeepCloneMsg(msg))
	flowCtx.Runtime.joinBuffers[node.ID] = buffer
	if len(buffer) < inputCount {
		return "", nil, nil
	}
	delete(flowCtx.Runtime.joinBuffers, node.ID)

	combineMode, _ := node.Config["combine_mode"].(string)
	if combineMode == "" {
		combineMode = "last"
	}

	out := DeepCloneMsg(buffer[len(buffer)-1])
	out = EnsureMsgID(out)
	switch combineMode {
	case "merge":
		merged := make(map[string]interface{})
		for _, item := range buffer {
			if payload, ok := item["payload"].(map[string]interface{}); ok {
				for k, v := range payload {
					merged[k] = v
				}
			}
		}
		out["payload"] = merged
	case "array":
		items := make([]interface{}, 0, len(buffer))
		for _, item := range buffer {
			if payload, ok := item["payload"]; ok {
				items = append(items, payload)
			} else {
				items = append(items, item)
			}
		}
		out["payload"] = items
	}
	out["_join"] = map[string]interface{}{"combine_mode": combineMode, "count": len(buffer)}
	return "output", out, nil
}

func (s *Service) executeAggregateRuntime(node *CanvasNode, msg Msg, flowCtx *FlowContext) (string, Msg, error) {
	mode, _ := node.Config["mode"].(string)
	if mode == "" {
		mode = "list"
	}
	groupBy, _ := node.Config["group_by"].(string)

	if payload, ok := msg["payload"].([]interface{}); ok {
		return "output", aggregateImmediate(msg, payload, mode, groupBy), nil
	}

	if flowCtx == nil || flowCtx.Runtime == nil {
		return "output", aggregateImmediate(msg, []interface{}{extractAggregateValue(msg, groupBy)}, mode, groupBy), nil
	}

	expected := partsCount(msg)
	if expected <= 0 {
		return "output", aggregateImmediate(msg, []interface{}{extractAggregateValue(msg, groupBy)}, mode, groupBy), nil
	}

	buf := flowCtx.Runtime.aggregateBuffers[node.ID]
	if buf == nil {
		buf = &aggregateBuffer{Expected: expected}
		flowCtx.Runtime.aggregateBuffers[node.ID] = buf
	}
	buf.Messages = append(buf.Messages, DeepCloneMsg(msg))
	if buf.Expected <= 0 {
		buf.Expected = expected
	}
	if len(buf.Messages) < buf.Expected {
		return "", nil, nil
	}
	delete(flowCtx.Runtime.aggregateBuffers, node.ID)

	values := make([]interface{}, 0, len(buf.Messages))
	for _, item := range buf.Messages {
		values = append(values, extractAggregateValue(item, groupBy))
	}
	out := aggregateImmediate(buf.Messages[len(buf.Messages)-1], values, mode, groupBy)
	return "output", out, nil
}

func (s *Service) executeRateLimitRuntime(node *CanvasNode, msg Msg, flowCtx *FlowContext) (string, Msg, error) {
	limit := int(configFloat(node.Config, "limit", 10))
	windowSeconds := int(configFloat(node.Config, "window_seconds", 60))
	if limit <= 0 || windowSeconds <= 0 {
		return "output", msg, nil
	}

	workflowID := ""
	if flowCtx != nil {
		workflowID, _ = flowCtx.FlowVars["_workflowId"].(string)
	}
	if workflowID == "" {
		return "output", msg, nil
	}

	key := "workflow:rate-limit:" + workflowID + ":" + node.ID
	now := time.Now().UTC()
	cutoff := now.Add(-time.Duration(windowSeconds) * time.Second)

	var timestamps []int64
	var raw string
	if err := s.db.QueryRow("SELECT value FROM workflow_kv WHERE key = ? LIMIT 1", key).Scan(&raw); err == nil && raw != "" {
		_ = json.Unmarshal([]byte(raw), &timestamps)
	}

	pruned := timestamps[:0]
	for _, ts := range timestamps {
		if time.Unix(ts, 0).After(cutoff) {
			pruned = append(pruned, ts)
		}
	}
	if len(pruned) >= limit {
		return "", nil, nil
	}

	pruned = append(pruned, now.Unix())
	valueBytes, _ := json.Marshal(pruned)
	expiresAt := now.Add(time.Duration(windowSeconds+60) * time.Second).Format("2006-01-02 15:04:05")
	if _, err := s.db.Exec(
		"INSERT OR REPLACE INTO workflow_kv (key, value, expires_at, updated_at) VALUES (?, ?, ?, datetime('now'))",
		key, string(valueBytes), expiresAt,
	); err != nil {
		s.logger.Warn("workflows: rate limit state update failed", "error", err, "workflowID", workflowID, "nodeID", node.ID)
	}
	return "output", msg, nil
}

func (s *Service) executeDeduplicateRuntime(node *CanvasNode, msg Msg) (string, Msg, error) {
	property, _ := node.Config["property"].(string)
	if property == "" {
		property = "payload"
	}
	uniqueBy, _ := node.Config["unique_by"].(string)

	out := CloneMsg(msg)
	out = EnsureMsgID(out)

	raw, ok := GetPath(out, property)
	if !ok {
		out["payload"] = map[string]interface{}{"error": "property not found", "property": property}
		return "error", out, nil
	}

	items, ok := raw.([]interface{})
	if !ok {
		return "output", out, nil
	}

	seen := make(map[string]struct{}, len(items))
	filtered := make([]interface{}, 0, len(items))
	for _, item := range items {
		key := deduplicateKey(item, uniqueBy)
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		filtered = append(filtered, item)
	}

	SetPath(out, property, filtered)
	out["_deduplicate"] = map[string]interface{}{"original": len(items), "unique": len(filtered)}
	return "output", out, nil
}

func (s *Service) executeWebhookResponseRuntime(node *CanvasNode, msg Msg, flowCtx *FlowContext) (string, Msg, error) {
	statusCode := int(configFloat(node.Config, "status_code", 200))
	contentType, _ := node.Config["content_type"].(string)
	if contentType == "" {
		contentType = "application/json"
	}
	body, _ := node.Config["body"].(string)

	out := CloneMsg(msg)
	out = EnsureMsgID(out)
	out["res"] = map[string]interface{}{
		"status":      statusCode,
		"contentType": contentType,
		"body":        body,
	}

	headers := map[string]string{"Content-Type": contentType}
	if flowCtx != nil && flowCtx.Runtime != nil {
		flowCtx.Runtime.setResponse(statusCode, headers, responseBodyFromMsg(body, contentType, out))
	}
	return "output", out, nil
}

func (s *Service) executeHTTPResponseRuntime(node *CanvasNode, msg Msg, flowCtx *FlowContext) (string, Msg, error) {
	statusCode := int(configFloat(node.Config, "status_code", 200))
	headers := configMap(node.Config["headers"])

	out := CloneMsg(msg)
	out = EnsureMsgID(out)
	hdrs := make(map[string]interface{}, len(headers))
	respHeaders := make(map[string]string, len(headers))
	for k, v := range headers {
		hdrs[k] = v
		respHeaders[k] = fmt.Sprintf("%v", v)
	}
	out["res"] = map[string]interface{}{"status": statusCode}
	out["headers"] = hdrs

	if flowCtx != nil && flowCtx.Runtime != nil {
		flowCtx.Runtime.setResponse(statusCode, respHeaders, responseBodyFromMsg("", respHeaders["Content-Type"], out))
	}
	return "output", out, nil
}

func (s *Service) executeSSHExecRuntime(ctx context.Context, node *CanvasNode, msg Msg) (string, Msg, error) {
	host, _ := node.Config["host"].(string)
	command, _ := node.Config["command"].(string)
	user, _ := node.Config["user"].(string)
	password, _ := node.Config["password"].(string)
	privateKey, _ := node.Config["private_key"].(string)
	port := configPort(node.Config["port"], 22)

	endpoint, err := parseRemoteEndpoint(host, port)
	if err != nil {
		return "", nil, err
	}
	if user == "" {
		user = endpoint.User
	}
	if password == "" {
		password = endpoint.Password
	}
	if privateKey == "" {
		privateKey = endpoint.PrivateKey
	}
	if user == "" {
		user = "root"
	}

	authMethods := make([]ssh.AuthMethod, 0, 2)
	if password != "" {
		authMethods = append(authMethods, ssh.Password(password))
	}
	if privateKey != "" {
		signer, err := ssh.ParsePrivateKey([]byte(privateKey))
		if err == nil {
			authMethods = append(authMethods, ssh.PublicKeys(signer))
		}
	}
	if len(authMethods) == 0 {
		return "", nil, fmt.Errorf("ssh credentials are required")
	}

	clientConfig := &ssh.ClientConfig{
		User:            user,
		Auth:            authMethods,
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		Timeout:         15 * time.Second,
	}

	client, err := ssh.Dial("tcp", endpoint.Address(), clientConfig)
	if err != nil {
		out := CloneMsg(msg)
		out = EnsureMsgID(out)
		out["payload"] = map[string]interface{}{"error": "ssh connection failed", "host": endpoint.Host}
		return "error", out, nil
	}
	defer client.Close()

	session, err := client.NewSession()
	if err != nil {
		out := CloneMsg(msg)
		out = EnsureMsgID(out)
		out["payload"] = map[string]interface{}{"error": "ssh session failed", "host": endpoint.Host}
		return "error", out, nil
	}
	defer session.Close()

	outputBytes, err := session.CombinedOutput(command)
	out := CloneMsg(msg)
	out = EnsureMsgID(out)
	out["payload"] = map[string]interface{}{
		"host":    endpoint.Host,
		"port":    endpoint.Port,
		"user":    user,
		"command": command,
		"output":  string(outputBytes),
	}
	if err != nil {
		out["payload"].(map[string]interface{})["error"] = "ssh command failed"
		return "error", out, nil
	}
	return "output", out, nil
}

func (s *Service) executeFTPUploadRuntime(ctx context.Context, node *CanvasNode, msg Msg) (string, Msg, error) {
	host, _ := node.Config["host"].(string)
	remotePath, _ := node.Config["remote_path"].(string)
	localPath, _ := node.Config["local_path"].(string)
	username, _ := node.Config["username"].(string)
	password, _ := node.Config["password"].(string)
	protocol, _ := node.Config["protocol"].(string)
	sourceMode, _ := node.Config["source_mode"].(string)
	property, _ := node.Config["property"].(string)
	port := configPort(node.Config["port"], 0)
	if protocol == "" {
		protocol = "ftp"
	}
	if property == "" {
		property = "payload"
	}
	if strings.TrimSpace(host) == "" || strings.TrimSpace(remotePath) == "" {
		return "", nil, fmt.Errorf("host and remote_path are required")
	}

	sourcePath := ""
	cleanup := func() {}
	useLocalPath := strings.EqualFold(sourceMode, "file") || (sourceMode == "" && strings.TrimSpace(localPath) != "")
	if useLocalPath {
		if strings.TrimSpace(localPath) == "" {
			return "", nil, fmt.Errorf("local_path is required when source_mode is file")
		}
		sourcePath = resolveWorkflowPath(localPath)
	} else {
		raw, ok := GetPath(msg, property)
		if !ok {
			out := CloneMsg(msg)
			out = EnsureMsgID(out)
			out["payload"] = map[string]interface{}{"error": "property not found", "property": property}
			return "error", out, nil
		}
		tmpPath, tmpCleanup, err := writeValueToTempFile(raw)
		if err != nil {
			return "", nil, err
		}
		sourcePath = tmpPath
		cleanup = tmpCleanup
	}
	defer cleanup()

	targetURL, err := buildTransferURL(protocol, host, remotePath, username, password, port)
	if err != nil {
		return "", nil, err
	}

	cmd := exec.CommandContext(ctx, "curl", "--fail", "--silent", "--show-error", "--create-dirs", "--upload-file", sourcePath, targetURL)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		out := CloneMsg(msg)
		out = EnsureMsgID(out)
		out["payload"] = map[string]interface{}{
			"error":      "upload failed",
			"protocol":   protocol,
			"remotePath": remotePath,
			"detail":     strings.TrimSpace(stderr.String()),
		}
		return "error", out, nil
	}

	out := CloneMsg(msg)
	out = EnsureMsgID(out)
	out["payload"] = map[string]interface{}{
		"protocol":   protocol,
		"remotePath": remotePath,
		"url":        targetURL,
		"status":     "uploaded",
		"output":     strings.TrimSpace(stdout.String()),
	}
	return "output", out, nil
}

func (s *Service) executeStackDeployRuntime(ctx context.Context, node *CanvasNode, msg Msg, envID string) (string, Msg, error) {
	stackName, _ := node.Config["stack_name"].(string)
	if strings.TrimSpace(stackName) == "" {
		return "", nil, fmt.Errorf("stack name is required")
	}
	if envID != "" && s.pool.IsAgentEnv(envID) {
		out := CloneMsg(msg)
		out = EnsureMsgID(out)
		out["payload"] = map[string]interface{}{"error": "stack deploy is not supported for agent environments", "stack": stackName}
		return "error", out, nil
	}

	composeBytes, sourceDesc, err := resolveComposeBytes(node, msg)
	if err != nil {
		s.logger.Warn("workflows: resolve compose bytes failed", "error", err, "stack", stackName)
		return "error", workflowErrorMsg(msg, sanitizeComposeResolutionError(err), map[string]interface{}{"stack": stackName}), nil
	}

	tempDir, err := os.MkdirTemp("", "workflow-compose-*")
	if err != nil {
		return "", nil, fmt.Errorf("creating temp compose directory: %w", err)
	}
	defer os.RemoveAll(tempDir)

	composePath := filepath.Join(tempDir, "docker-compose.yml")
	if err := os.WriteFile(composePath, composeBytes, 0o644); err != nil {
		return "", nil, fmt.Errorf("writing compose file: %w", err)
	}

	cmd := exec.CommandContext(ctx, "docker", "compose", "-f", "docker-compose.yml", "-p", stackName, "up", "-d")
	cmd.Dir = tempDir
	cmd.Env = os.Environ()
	if envID != "" {
		if host, err := s.pool.DockerHost(envID); err == nil && host != "" {
			cmd.Env = append(filteredEnv(cmd.Env, "DOCKER_HOST"), "DOCKER_HOST="+host)
		}
	}

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		out := CloneMsg(msg)
		out = EnsureMsgID(out)
		out["payload"] = map[string]interface{}{
			"error":  "stack deploy failed",
			"stack":  stackName,
			"source": sourceDesc,
			"output": strings.TrimSpace(stderr.String()),
		}
		return "error", out, nil
	}

	out := CloneMsg(msg)
	out = EnsureMsgID(out)
	out["payload"] = map[string]interface{}{
		"stack":  stackName,
		"source": sourceDesc,
		"status": "deployed",
		"output": strings.TrimSpace(stdout.String()),
	}
	return "output", out, nil
}

func (s *Service) executeImageBuildRuntime(ctx context.Context, node *CanvasNode, msg Msg, envID string) (string, Msg, error) {
	cli, err := s.pool.Get(envID)
	if err != nil {
		return "", nil, fmt.Errorf("docker connection failed: %w", err)
	}

	tag, _ := node.Config["tag"].(string)
	dockerfileName, _ := node.Config["dockerfile"].(string)
	contextPath, _ := node.Config["context_path"].(string)
	target, _ := node.Config["target"].(string)
	noCache, _ := node.Config["no_cache"].(bool)
	buildArgs := buildArgsMap(configMap(node.Config["build_args"]))
	if tag == "" {
		return "", nil, fmt.Errorf("tag is required")
	}
	if dockerfileName == "" {
		dockerfileName = "Dockerfile"
	}
	if contextPath == "" {
		contextPath = "."
	}

	buildRoot := resolveWorkflowPath(contextPath)
	tarReader, err := tarDirectory(buildRoot)
	if err != nil {
		out := CloneMsg(msg)
		out = EnsureMsgID(out)
		out["payload"] = map[string]interface{}{"error": "build context unavailable", "contextPath": contextPath}
		return "error", out, nil
	}

	buildCtx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()

	resp, err := cli.ImageBuild(buildCtx, tarReader, build.ImageBuildOptions{
		Dockerfile: dockerfileName,
		Tags:       []string{tag},
		Remove:     true,
		Target:     target,
		NoCache:    noCache,
		BuildArgs:  buildArgs,
	})
	if err != nil {
		out := CloneMsg(msg)
		out = EnsureMsgID(out)
		out["payload"] = map[string]interface{}{"error": "image build failed", "tag": tag}
		return "error", out, nil
	}
	defer resp.Body.Close()

	buildOutput, _ := io.ReadAll(resp.Body)
	out := CloneMsg(msg)
	out = EnsureMsgID(out)
	out["payload"] = map[string]interface{}{
		"tag":         tag,
		"dockerfile":  dockerfileName,
		"contextPath": contextPath,
		"target":      target,
		"noCache":     noCache,
		"buildArgs":   configMap(node.Config["build_args"]),
		"status":      "built",
		"output":      string(buildOutput),
	}
	return "output", out, nil
}

func splitMessagesForPath(nodeID string, msg Msg, path, port string) (Msg, []nodeEmission, error) {
	out := CloneMsg(msg)
	out = EnsureMsgID(out)
	raw, ok := GetPath(out, path)
	if !ok {
		out["payload"] = map[string]interface{}{"error": "property not found", "property": path}
		return out, nil, fmt.Errorf("property not found")
	}

	items, ok := raw.([]interface{})
	if !ok {
		return out, nil, nil
	}

	groupID := fmt.Sprintf("%v", out["_msgid"])
	emissions := make([]nodeEmission, 0, len(items))
	for i, item := range items {
		next := DeepCloneMsg(msg)
		next = EnsureMsgID(next)
		SetPath(next, path, item)
		next["parts"] = map[string]interface{}{
			"id":    groupID + ":" + nodeID,
			"type":  "array",
			"count": len(items),
			"index": i,
		}
		emissions = append(emissions, nodeEmission{Port: port, Msg: next})
	}

	out["payload"] = items
	out["parts"] = map[string]interface{}{
		"id":      groupID + ":" + nodeID,
		"type":    "array",
		"count":   len(items),
		"emitted": len(emissions),
	}
	return out, emissions, nil
}

func aggregateImmediate(msg Msg, values []interface{}, mode, groupBy string) Msg {
	out := DeepCloneMsg(msg)
	out = EnsureMsgID(out)
	switch mode {
	case "count":
		out["payload"] = len(values)
	case "sum":
		sum := 0.0
		for _, value := range values {
			sum += floatValue(value)
		}
		out["payload"] = sum
	default:
		out["payload"] = values
	}
	out["_aggregate"] = map[string]interface{}{
		"mode":     mode,
		"group_by": groupBy,
		"count":    len(values),
	}
	return out
}

func extractAggregateValue(msg Msg, groupBy string) interface{} {
	if groupBy != "" {
		if value, ok := GetPath(msg, groupBy); ok {
			return value
		}
		if value, ok := GetPath(msg, "payload."+groupBy); ok {
			return value
		}
	}
	if value, ok := msg["payload"]; ok {
		return value
	}
	return nil
}

func partsCount(msg Msg) int {
	if msg == nil {
		return 0
	}
	parts, _ := msg["parts"].(map[string]interface{})
	if parts == nil {
		return 0
	}
	return int(floatValue(parts["count"]))
}

func responseBodyFromMsg(explicitBody, contentType string, msg Msg) []byte {
	if explicitBody != "" {
		return []byte(explicitBody)
	}
	if msg == nil {
		return nil
	}
	payload, ok := msg["payload"]
	if !ok {
		return nil
	}
	if strings.HasPrefix(strings.ToLower(contentType), "application/json") {
		body, err := json.Marshal(payload)
		if err == nil {
			return body
		}
	}
	switch value := payload.(type) {
	case string:
		return []byte(value)
	case []byte:
		return value
	default:
		body, err := json.Marshal(value)
		if err == nil {
			return body
		}
	}
	return []byte(fmt.Sprintf("%v", payload))
}

func resolveComposeBytes(node *CanvasNode, msg Msg) ([]byte, string, error) {
	composeSource, _ := node.Config["compose_source"].(string)
	switch strings.ToLower(strings.TrimSpace(composeSource)) {
	case "inline":
		if raw, ok := node.Config["compose_content"].(string); ok && strings.TrimSpace(raw) != "" {
			return []byte(raw), "config.compose_content", nil
		}
		return nil, "", fmt.Errorf("compose_content is required when compose_source is inline")
	case "file":
		if raw, ok := node.Config["compose_path"].(string); ok && strings.TrimSpace(raw) != "" {
			fullPath := resolveWorkflowPath(raw)
			data, err := os.ReadFile(fullPath)
			if err != nil {
				return nil, "", fmt.Errorf("compose file not found")
			}
			return data, raw, nil
		}
		return nil, "", fmt.Errorf("compose_path is required when compose_source is file")
	case "message":
		return resolveComposeBytesFromMessage(msg)
	}

	if raw, ok := node.Config["compose_content"].(string); ok && strings.TrimSpace(raw) != "" {
		return []byte(raw), "config.compose_content", nil
	}
	if raw, ok := node.Config["compose_path"].(string); ok && strings.TrimSpace(raw) != "" {
		fullPath := resolveWorkflowPath(raw)
		data, err := os.ReadFile(fullPath)
		if err != nil {
			return nil, "", fmt.Errorf("compose file not found")
		}
		return data, raw, nil
	}
	return resolveComposeBytesFromMessage(msg)
}

func resolveComposeBytesFromMessage(msg Msg) ([]byte, string, error) {
	if payload, ok := msg["payload"].(string); ok && strings.TrimSpace(payload) != "" {
		return []byte(payload), "msg.payload", nil
	}
	if payload, ok := msg["payload"].(map[string]interface{}); ok {
		if compose, ok := payload["compose"].(string); ok && strings.TrimSpace(compose) != "" {
			return []byte(compose), "msg.payload.compose", nil
		}
		if composePath, ok := payload["compose_path"].(string); ok && strings.TrimSpace(composePath) != "" {
			fullPath := resolveWorkflowPath(composePath)
			data, err := os.ReadFile(fullPath)
			if err != nil {
				return nil, "", fmt.Errorf("compose file not found")
			}
			return data, composePath, nil
		}
	}
	return nil, "", fmt.Errorf("compose_content, compose_path, or msg.payload compose data is required")
}

func resolveWorkflowPath(path string) string {
	cleaned := filepath.Clean(path)
	if filepath.IsAbs(cleaned) {
		return cleaned
	}
	dataDir := os.Getenv("DATA_DIR")
	if dataDir == "" {
		dataDir = "/app/data"
	}
	return filepath.Join(dataDir, "files", cleaned)
}

func tarDirectory(root string) (io.Reader, error) {
	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)

	walkErr := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}

		relPath, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		relPath = filepath.ToSlash(relPath)

		header, err := tar.FileInfoHeader(info, "")
		if err != nil {
			return err
		}
		header.Name = relPath
		if err := tw.WriteHeader(header); err != nil {
			return err
		}

		file, err := os.Open(path)
		if err != nil {
			return err
		}
		if _, err := io.Copy(tw, file); err != nil {
			file.Close()
			return err
		}
		if err := file.Close(); err != nil {
			return err
		}
		return nil
	})
	if walkErr != nil {
		_ = tw.Close()
		return nil, walkErr
	}
	if err := tw.Close(); err != nil {
		return nil, err
	}
	return bytes.NewReader(buf.Bytes()), nil
}

func writeValueToTempFile(raw interface{}) (string, func(), error) {
	payloadBytes := encodeWorkflowBody(raw)
	tmpFile, err := os.CreateTemp("", "workflow-upload-*")
	if err != nil {
		return "", func() {}, fmt.Errorf("creating temp upload file: %w", err)
	}
	if _, err := tmpFile.Write(payloadBytes); err != nil {
		tmpFile.Close()
		os.Remove(tmpFile.Name())
		return "", func() {}, fmt.Errorf("writing temp upload file: %w", err)
	}
	if err := tmpFile.Close(); err != nil {
		os.Remove(tmpFile.Name())
		return "", func() {}, fmt.Errorf("closing temp upload file: %w", err)
	}
	return tmpFile.Name(), func() { _ = os.Remove(tmpFile.Name()) }, nil
}

func buildTransferURL(protocol, host, remotePath, username, password string, port int) (string, error) {
	trimmedHost := strings.TrimSpace(host)
	if trimmedHost == "" {
		return "", fmt.Errorf("host is required")
	}

	if !strings.Contains(trimmedHost, "://") {
		trimmedHost = protocol + "://" + trimmedHost
	}
	u, err := url.Parse(trimmedHost)
	if err != nil {
		return "", err
	}
	if u.Scheme == "" {
		u.Scheme = protocol
	}
	hostName := u.Hostname()
	if hostName == "" {
		hostName = u.Host
	}
	if port > 0 {
		u.Host = net.JoinHostPort(hostName, strconv.Itoa(port))
	} else if hostName != "" && u.Port() == "" {
		u.Host = hostName
	}
	if username != "" {
		if password != "" {
			u.User = url.UserPassword(username, password)
		} else {
			u.User = url.User(username)
		}
	}
	u.Path = joinURLPath(u.Path, remotePath)
	return u.String(), nil
}

func joinURLPath(basePath, extra string) string {
	basePath = strings.TrimSuffix(basePath, "/")
	extra = "/" + strings.TrimPrefix(extra, "/")
	if basePath == "" {
		return extra
	}
	return basePath + extra
}

type remoteEndpoint struct {
	Host       string
	Port       int
	User       string
	Password   string
	PrivateKey string
}

func (e remoteEndpoint) Address() string {
	return fmt.Sprintf("%s:%d", e.Host, e.Port)
}

func parseRemoteEndpoint(raw string, defaultPort int) (remoteEndpoint, error) {
	value := strings.TrimSpace(raw)
	if value == "" {
		return remoteEndpoint{}, fmt.Errorf("host is required")
	}
	if !strings.Contains(value, "://") {
		value = "ssh://" + value
	}
	u, err := url.Parse(value)
	if err != nil {
		return remoteEndpoint{}, fmt.Errorf("invalid host")
	}

	port := defaultPort
	if u.Port() != "" {
		if n, err := strconv.Atoi(u.Port()); err == nil {
			port = n
		}
	}

	endpoint := remoteEndpoint{
		Host: u.Hostname(),
		Port: port,
	}
	if u.User != nil {
		endpoint.User = u.User.Username()
		if password, ok := u.User.Password(); ok {
			endpoint.Password = password
		}
	}
	if endpoint.Host == "" {
		return remoteEndpoint{}, fmt.Errorf("host is required")
	}
	return endpoint, nil
}

func configMap(value interface{}) map[string]interface{} {
	m, _ := value.(map[string]interface{})
	if m == nil {
		return map[string]interface{}{}
	}
	return m
}

func flattenHeaders(headers http.Header) map[string]interface{} {
	out := make(map[string]interface{}, len(headers))
	for key, values := range headers {
		if len(values) == 1 {
			out[key] = values[0]
		} else {
			items := make([]interface{}, len(values))
			for i, value := range values {
				items[i] = value
			}
			out[key] = items
		}
	}
	return out
}

func encodeWorkflowBody(raw interface{}) []byte {
	switch value := raw.(type) {
	case nil:
		return nil
	case []byte:
		return value
	case string:
		return []byte(value)
	default:
		bodyBytes, err := json.Marshal(value)
		if err != nil {
			return []byte(fmt.Sprintf("%v", value))
		}
		return bodyBytes
	}
}

func decodeWorkflowBody(contentType string, body []byte) interface{} {
	if len(body) == 0 {
		return ""
	}
	if strings.Contains(strings.ToLower(contentType), "json") {
		var parsed interface{}
		if err := json.Unmarshal(body, &parsed); err == nil {
			return parsed
		}
	}
	return string(body)
}

func floatValue(raw interface{}) float64 {
	switch value := raw.(type) {
	case float64:
		return value
	case float32:
		return float64(value)
	case int:
		return float64(value)
	case int64:
		return float64(value)
	case int32:
		return float64(value)
	case json.Number:
		n, _ := value.Float64()
		return n
	case string:
		n, _ := strconv.ParseFloat(strings.TrimSpace(value), 64)
		return n
	default:
		return 0
	}
}

func configPort(raw interface{}, fallback int) int {
	switch value := raw.(type) {
	case float64:
		if value > 0 {
			return int(value)
		}
	case int:
		if value > 0 {
			return value
		}
	case string:
		if n, err := strconv.Atoi(value); err == nil && n > 0 {
			return n
		}
	}
	return fallback
}

func filteredEnv(env []string, key string) []string {
	prefix := key + "="
	out := make([]string, 0, len(env))
	for _, value := range env {
		if strings.HasPrefix(value, prefix) {
			continue
		}
		out = append(out, value)
	}
	return out
}

func buildArgsMap(values map[string]interface{}) map[string]*string {
	if len(values) == 0 {
		return nil
	}
	out := make(map[string]*string, len(values))
	for key, value := range values {
		stringValue := fmt.Sprintf("%v", value)
		out[key] = &stringValue
	}
	return out
}

func deduplicateKey(item interface{}, uniqueBy string) string {
	if uniqueBy != "" {
		if m, ok := item.(map[string]interface{}); ok {
			if value, ok := GetPath(m, uniqueBy); ok {
				return fmt.Sprintf("%T:%v", value, value)
			}
		}
	}

	data, err := json.Marshal(item)
	if err == nil {
		return string(data)
	}
	return fmt.Sprintf("%T:%v", item, item)
}
