// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"

	"github.com/therealmcsparrow/mcharbor/core/plugin"
)

const (
	defaultOllamaBaseURL   = "http://localhost:11434"
	defaultOllamaModel     = "llama3.2"
	ollamaRequestTimeout   = 30 * time.Second
	maxOllamaRequestBytes  = 4 << 20
	maxOllamaResponseBytes = 8 << 20
)

type ollamaClient struct {
	httpClient *http.Client
}

type ollamaTagsResponse struct {
	Models []ollamaModel `json:"models"`
}

type ollamaModel struct {
	Name  string `json:"name"`
	Model string `json:"model"`
}

type ollamaChatRequest struct {
	Model    string            `json:"model"`
	Messages []ollamaMessage   `json:"messages"`
	Stream   bool              `json:"stream"`
	Tools    []ollamaTool      `json:"tools,omitempty"`
	Options  ollamaChatOptions `json:"options"`
}

type ollamaTool struct {
	Type     string         `json:"type"`
	Function ollamaToolFunc `json:"function"`
}

type ollamaToolFunc struct {
	Name        string         `json:"name"`
	Description string         `json:"description,omitempty"`
	Parameters  map[string]any `json:"parameters"`
}

type ollamaToolCall struct {
	Function struct {
		Name      string         `json:"name"`
		Arguments map[string]any `json:"arguments"`
	} `json:"function"`
}

type ollamaMessage struct {
	Role      string           `json:"role"`
	Content   string           `json:"content"`
	ToolCalls []ollamaToolCall `json:"tool_calls,omitempty"`
}

type ollamaChatOptions struct {
	NumPredict  int     `json:"num_predict,omitempty"`
	Temperature float32 `json:"temperature"`
}

type ollamaChatResponse struct {
	Model           string        `json:"model"`
	Message         ollamaMessage `json:"message"`
	Done            bool          `json:"done"`
	DoneReason      string        `json:"done_reason"`
	PromptEvalCount int           `json:"prompt_eval_count"`
	EvalCount       int           `json:"eval_count"`
}

func newOllamaClient() *ollamaClient {
	return &ollamaClient{httpClient: &http.Client{
		Timeout: ollamaRequestTimeout,
		CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}}
}

// ListModels fetches the live model catalog from Ollama's native tags API.
func (c *ollamaClient) ListModels(ctx context.Context, config map[string]string) ([]plugin.ModelDecl, error) {
	body, err := c.do(ctx, config, http.MethodGet, "/api/tags", nil)
	if err != nil {
		return nil, err
	}

	var payload ollamaTagsResponse
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, errors.New("ollama returned an invalid model response")
	}

	models := make([]plugin.ModelDecl, 0, len(payload.Models))
	seen := make(map[string]struct{}, len(payload.Models))
	for _, model := range payload.Models {
		id := strings.TrimSpace(model.Model)
		if id == "" {
			id = strings.TrimSpace(model.Name)
		}
		if id == "" {
			continue
		}
		if _, exists := seen[id]; exists {
			continue
		}
		seen[id] = struct{}{}
		models = append(models, plugin.ModelDecl{
			ID:           id,
			DisplayName:  id,
			Capabilities: []string{"chat"},
		})
	}
	sort.Slice(models, func(i, j int) bool { return models[i].DisplayName < models[j].DisplayName })
	return models, nil
}

// Models implements plugin.AIAdapterClient.
func (c *ollamaClient) Models(ctx context.Context, config map[string]string) ([]plugin.ModelDecl, error) {
	return c.ListModels(ctx, config)
}

// Complete sends one non-streaming request to Ollama's native chat API.
func (c *ollamaClient) Complete(ctx context.Context, config map[string]string, req plugin.LLMRequest) (*plugin.LLMResponse, error) {
	model := strings.TrimSpace(req.Model)
	if model == "" {
		model = configuredOllamaModel(config)
	}
	if req.MaxTokens < 0 || math.IsNaN(float64(req.Temperature)) || math.IsInf(float64(req.Temperature), 0) || req.Temperature < 0 || req.Temperature > 2 {
		return nil, errors.New("invalid ollama completion options")
	}

	messages := make([]ollamaMessage, 0, len(req.Messages)+1)
	if req.System != "" {
		messages = append(messages, ollamaMessage{Role: "system", Content: req.System})
	}
	for _, message := range req.Messages {
		role := strings.ToLower(strings.TrimSpace(message.Role))
		switch role {
		case "system", "user", "assistant":
		default:
			return nil, errors.New("invalid ollama chat message")
		}
		messages = append(messages, ollamaMessage{Role: role, Content: message.Content})
	}
	if len(messages) == 0 {
		return nil, errors.New("ollama request requires at least one message")
	}

	payload := ollamaChatRequest{
		Model:    model,
		Messages: messages,
		Stream:   false,
		Options: ollamaChatOptions{
			NumPredict:  req.MaxTokens,
			Temperature: req.Temperature,
		},
	}
	if len(req.Tools) > 0 {
		payload.Tools = ollamaTools(req.Tools)
	}
	body, err := c.do(ctx, config, http.MethodPost, "/api/chat", payload)
	if err != nil {
		return nil, err
	}

	var result ollamaChatResponse
	if err := json.Unmarshal(body, &result); err != nil || !result.Done {
		return nil, errors.New("ollama returned an invalid chat response")
	}
	if result.Message.Content == "" && len(result.Message.ToolCalls) == 0 {
		return nil, errors.New("ollama response contained no content")
	}
	if result.Model == "" {
		result.Model = model
	}
	if result.DoneReason == "" {
		result.DoneReason = "stop"
	}

	toolCalls := make([]plugin.LLMToolCall, 0, len(result.Message.ToolCalls))
	for _, call := range result.Message.ToolCalls {
		name := strings.TrimSpace(call.Function.Name)
		if name == "" {
			continue
		}
		args, err := json.Marshal(call.Function.Arguments)
		if err != nil || len(args) == 0 {
			args = []byte(`{}`)
		}
		toolCalls = append(toolCalls, plugin.LLMToolCall{
			Name:      name,
			Arguments: args,
		})
	}

	return &plugin.LLMResponse{
		Content:   result.Message.Content,
		ToolCalls: toolCalls,
		Model:     result.Model,
		Usage: plugin.LLMUsage{
			InputTokens:  result.PromptEvalCount,
			OutputTokens: result.EvalCount,
			TotalTokens:  result.PromptEvalCount + result.EvalCount,
		},
		FinishReason: result.DoneReason,
	}, nil
}

func ollamaTools(tools []plugin.LLMTool) []ollamaTool {
	out := make([]ollamaTool, 0, len(tools))
	for _, tool := range tools {
		name := strings.TrimSpace(tool.Name)
		if name == "" {
			continue
		}
		params := tool.Parameters
		if params == nil {
			params = map[string]any{"type": "object", "properties": map[string]any{}}
		}
		out = append(out, ollamaTool{
			Type: "function",
			Function: ollamaToolFunc{
				Name:        name,
				Description: strings.TrimSpace(tool.Description),
				Parameters:  params,
			},
		})
	}
	return out
}

// TestConnection verifies the configured endpoint through the model-list API.
func (c *ollamaClient) TestConnection(ctx context.Context, config map[string]string) (*plugin.AITestResult, error) {
	startedAt := time.Now()
	models, err := c.ListModels(ctx, config)
	latencyMs := int(time.Since(startedAt).Milliseconds())
	if err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return nil, ctxErr
		}
		return &plugin.AITestResult{
			Success:   false,
			LatencyMs: latencyMs,
			Message:   "Unable to connect to Ollama. Verify the URL, credentials, and server status.",
		}, nil
	}

	result := &plugin.AITestResult{
		Success:   true,
		LatencyMs: latencyMs,
		Message:   "Ollama is reachable.",
	}
	if len(models) == 0 {
		result.Message = "Ollama is reachable, but no models are installed."
	} else {
		result.Reply = models[0].ID
	}
	return result, nil
}

// Test implements plugin.AIAdapterClient.
func (c *ollamaClient) Test(ctx context.Context, config map[string]string) (*plugin.AITestResult, error) {
	return c.TestConnection(ctx, config)
}

func configuredOllamaModel(config map[string]string) string {
	if model := strings.TrimSpace(config["default_model"]); model != "" {
		return model
	}
	return defaultOllamaModel
}

func (c *ollamaClient) do(ctx context.Context, config map[string]string, method, endpointPath string, payload any) ([]byte, error) {
	endpoint, err := ollamaEndpoint(config, endpointPath)
	if err != nil {
		return nil, err
	}

	var requestBody io.Reader
	if payload != nil {
		encoded, marshalErr := json.Marshal(payload)
		if marshalErr != nil {
			return nil, errors.New("unable to encode ollama request")
		}
		if len(encoded) > maxOllamaRequestBytes {
			return nil, errors.New("ollama request is too large")
		}
		requestBody = bytes.NewReader(encoded)
	}

	req, err := http.NewRequestWithContext(ctx, method, endpoint, requestBody)
	if err != nil {
		return nil, errors.New("unable to create ollama request")
	}
	req.Header.Set("Accept", "application/json")
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if apiKey := strings.TrimSpace(config["api_key"]); apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+apiKey)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return nil, ctxErr
		}
		return nil, errors.New("ollama request failed")
	}
	body, err := readOllamaBody(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return nil, fmt.Errorf("ollama request returned status %d", resp.StatusCode)
	}
	return body, nil
}

func ollamaEndpoint(config map[string]string, endpointPath string) (string, error) {
	rawBaseURL := strings.TrimSpace(config["base_url"])
	if rawBaseURL == "" {
		rawBaseURL = defaultOllamaBaseURL
	}
	baseURL, err := url.Parse(rawBaseURL)
	if err != nil || !baseURL.IsAbs() || baseURL.Host == "" {
		return "", errors.New("ollama base URL must be an absolute HTTP or HTTPS URL")
	}
	baseURL.Scheme = strings.ToLower(baseURL.Scheme)
	if baseURL.Scheme != "http" && baseURL.Scheme != "https" {
		return "", errors.New("ollama base URL must use HTTP or HTTPS")
	}
	if baseURL.User != nil || baseURL.RawQuery != "" || baseURL.Fragment != "" {
		return "", errors.New("ollama base URL must not contain credentials, a query, or a fragment")
	}
	baseURL.Path = strings.TrimRight(baseURL.Path, "/") + endpointPath
	baseURL.RawPath = ""
	return baseURL.String(), nil
}

func readOllamaBody(body io.ReadCloser) ([]byte, error) {
	data, readErr := io.ReadAll(io.LimitReader(body, maxOllamaResponseBytes+1))
	closeErr := body.Close()
	if readErr != nil {
		return nil, errors.New("unable to read ollama response")
	}
	if closeErr != nil {
		return nil, errors.New("unable to close ollama response")
	}
	if len(data) > maxOllamaResponseBytes {
		return nil, errors.New("ollama response is too large")
	}
	return data, nil
}

var _ plugin.AIAdapterClient = (*ollamaClient)(nil)
