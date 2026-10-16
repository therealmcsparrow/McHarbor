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
	"strings"
	"time"

	"github.com/therealmcsparrow/mcharbor/core/plugin"
)

const (
	defaultAnthropicBaseURL   = "https://api.anthropic.com/v1"
	defaultAnthropicModel     = "claude-haiku-4-5"
	defaultAnthropicMaxTokens = 4096
	anthropicAPIVersion       = "2023-06-01"
	anthropicHTTPTimeout      = 30 * time.Second
	maxAnthropicResponseBytes = 4 << 20
	maxAnthropicModelPages    = 10
)

type anthropicClient struct {
	httpClient *http.Client
}

type anthropicMessageRequest struct {
	Model       string             `json:"model"`
	System      string             `json:"system,omitempty"`
	Messages    []anthropicMessage `json:"messages"`
	MaxTokens   int                `json:"max_tokens"`
	Temperature float32            `json:"temperature"`
	Tools       []anthropicTool    `json:"tools,omitempty"`
}

type anthropicTool struct {
	Name        string         `json:"name"`
	Description string         `json:"description,omitempty"`
	InputSchema map[string]any `json:"input_schema"`
}

type anthropicMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type anthropicMessageResponse struct {
	Content []struct {
		Type  string         `json:"type"`
		Text  string         `json:"text"`
		ID    string         `json:"id"`
		Name  string         `json:"name"`
		Input map[string]any `json:"input"`
	} `json:"content"`
	Model      string `json:"model"`
	StopReason string `json:"stop_reason"`
	Usage      struct {
		InputTokens  int `json:"input_tokens"`
		OutputTokens int `json:"output_tokens"`
	} `json:"usage"`
}

type anthropicModelsResponse struct {
	Data []struct {
		ID          string `json:"id"`
		DisplayName string `json:"display_name"`
	} `json:"data"`
	HasMore bool   `json:"has_more"`
	LastID  string `json:"last_id"`
}

func newAnthropicClient() *anthropicClient {
	return &anthropicClient{
		httpClient: &http.Client{
			Timeout: anthropicHTTPTimeout,
			CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
				return http.ErrUseLastResponse
			},
		},
	}
}

// Test implements plugin.AIAdapterClient.
func (c *anthropicClient) Test(ctx context.Context, config map[string]string) (*plugin.AITestResult, error) {
	return c.TestConnection(ctx, config)
}

// TestConnection verifies credentials and reachability using the models endpoint.
func (c *anthropicClient) TestConnection(ctx context.Context, config map[string]string) (*plugin.AITestResult, error) {
	startedAt := time.Now()
	result := &plugin.AITestResult{}

	if strings.TrimSpace(config["api_key"]) == "" {
		result.Message = "Anthropic API key is required"
		return result, nil
	}
	if _, err := anthropicEndpoint(config, "models", nil); err != nil {
		result.Message = "Anthropic base URL must be an absolute HTTP or HTTPS URL"
		return result, nil
	}

	models, err := c.ListModels(ctx, config)
	result.LatencyMs = int(time.Since(startedAt) / time.Millisecond)
	if err != nil {
		result.Message = "Anthropic connection test failed; check the base URL and API key"
		return result, nil
	}

	result.Success = true
	result.Message = "Anthropic connection successful"
	result.Reply = fmt.Sprintf("%d models available", len(models))
	return result, nil
}

// Complete sends a non-streaming request to Anthropic's Messages API.
func (c *anthropicClient) Complete(ctx context.Context, config map[string]string, req plugin.LLMRequest) (*plugin.LLMResponse, error) {
	if strings.TrimSpace(config["api_key"]) == "" {
		return nil, errors.New("anthropic API key is required")
	}
	if req.MaxTokens < 0 {
		return nil, errors.New("anthropic max tokens cannot be negative")
	}
	if math.IsNaN(float64(req.Temperature)) || math.IsInf(float64(req.Temperature), 0) || req.Temperature < 0 || req.Temperature > 1 {
		return nil, errors.New("anthropic temperature must be between 0 and 1")
	}

	model := strings.TrimSpace(req.Model)
	if model == "" {
		model = strings.TrimSpace(config["default_model"])
	}
	if model == "" {
		model = defaultAnthropicModel
	}

	system := strings.TrimSpace(req.System)
	messages := make([]anthropicMessage, 0, len(req.Messages))
	for _, message := range req.Messages {
		role := strings.ToLower(strings.TrimSpace(message.Role))
		content := strings.TrimSpace(message.Content)
		switch role {
		case "system":
			if content != "" {
				if system != "" {
					system += "\n\n"
				}
				system += content
			}
		case "user", "assistant":
			if content == "" {
				return nil, errors.New("anthropic message content is required")
			}
			messages = append(messages, anthropicMessage{Role: role, Content: content})
		default:
			return nil, errors.New("anthropic message role must be user, assistant, or system")
		}
	}
	if len(messages) == 0 {
		return nil, errors.New("anthropic request requires at least one user or assistant message")
	}

	maxTokens := req.MaxTokens
	if maxTokens == 0 {
		maxTokens = defaultAnthropicMaxTokens
	}
	payload := anthropicMessageRequest{
		Model:       model,
		System:      system,
		Messages:    messages,
		MaxTokens:   maxTokens,
		Temperature: req.Temperature,
	}
	if len(req.Tools) > 0 {
		payload.Tools = anthropicTools(req.Tools)
	}

	var response anthropicMessageResponse
	if err := c.doJSON(ctx, config, http.MethodPost, "messages", nil, payload, &response); err != nil {
		return nil, err
	}

	var content strings.Builder
	toolCalls := make([]plugin.LLMToolCall, 0)
	for _, block := range response.Content {
		if block.Type == "text" {
			content.WriteString(block.Text)
		}
		if block.Type == "tool_use" {
			name := strings.TrimSpace(block.Name)
			if name == "" {
				continue
			}
			args, err := json.Marshal(block.Input)
			if err != nil || len(args) == 0 {
				args = []byte(`{}`)
			}
			toolCalls = append(toolCalls, plugin.LLMToolCall{
				ID:        strings.TrimSpace(block.ID),
				Name:      name,
				Arguments: args,
			})
		}
	}
	if content.Len() == 0 && len(toolCalls) == 0 {
		return nil, errors.New("anthropic response contained no text")
	}
	if response.Model == "" {
		response.Model = model
	}

	return &plugin.LLMResponse{
		Content:   content.String(),
		ToolCalls: toolCalls,
		Model:     response.Model,
		Usage: plugin.LLMUsage{
			InputTokens:  response.Usage.InputTokens,
			OutputTokens: response.Usage.OutputTokens,
			TotalTokens:  response.Usage.InputTokens + response.Usage.OutputTokens,
		},
		FinishReason: response.StopReason,
	}, nil
}

func anthropicTools(tools []plugin.LLMTool) []anthropicTool {
	out := make([]anthropicTool, 0, len(tools))
	for _, tool := range tools {
		name := strings.TrimSpace(tool.Name)
		if name == "" {
			continue
		}
		params := tool.Parameters
		if params == nil {
			params = map[string]any{"type": "object", "properties": map[string]any{}}
		}
		out = append(out, anthropicTool{
			Name:        name,
			Description: strings.TrimSpace(tool.Description),
			InputSchema: params,
		})
	}
	return out
}

// Models implements plugin.AIAdapterClient.
func (c *anthropicClient) Models(ctx context.Context, config map[string]string) ([]plugin.ModelDecl, error) {
	return c.ListModels(ctx, config)
}

// ListModels retrieves the model catalog from Anthropic's Models API.
func (c *anthropicClient) ListModels(ctx context.Context, config map[string]string) ([]plugin.ModelDecl, error) {
	if strings.TrimSpace(config["api_key"]) == "" {
		return nil, errors.New("anthropic API key is required")
	}

	models := make([]plugin.ModelDecl, 0, 32)
	seen := make(map[string]struct{})
	afterID := ""
	for page := 0; page < maxAnthropicModelPages; page++ {
		query := url.Values{"limit": []string{"100"}}
		if afterID != "" {
			query.Set("after_id", afterID)
		}

		var response anthropicModelsResponse
		if err := c.doJSON(ctx, config, http.MethodGet, "models", query, nil, &response); err != nil {
			return nil, err
		}
		for _, model := range response.Data {
			id := strings.TrimSpace(model.ID)
			if id == "" {
				continue
			}
			if _, ok := seen[id]; ok {
				continue
			}
			seen[id] = struct{}{}
			displayName := strings.TrimSpace(model.DisplayName)
			if displayName == "" {
				displayName = id
			}
			models = append(models, plugin.ModelDecl{
				ID:           id,
				DisplayName:  displayName,
				Capabilities: []string{"text"},
			})
		}
		if !response.HasMore {
			return models, nil
		}
		afterID = strings.TrimSpace(response.LastID)
		if afterID == "" {
			return nil, errors.New("anthropic models response was incomplete")
		}
	}

	return nil, errors.New("anthropic models response exceeded the page limit")
}

func (c *anthropicClient) doJSON(
	ctx context.Context,
	config map[string]string,
	method string,
	resource string,
	query url.Values,
	payload any,
	out any,
) error {
	endpoint, err := anthropicEndpoint(config, resource, query)
	if err != nil {
		return err
	}

	var body io.Reader
	if payload != nil {
		encoded, marshalErr := json.Marshal(payload)
		if marshalErr != nil {
			return errors.New("failed to encode anthropic request")
		}
		body = bytes.NewReader(encoded)
	}

	request, err := http.NewRequestWithContext(ctx, method, endpoint, body)
	if err != nil {
		return errors.New("failed to create anthropic request")
	}
	request.Header.Set("Accept", "application/json")
	request.Header.Set("x-api-key", strings.TrimSpace(config["api_key"]))
	request.Header.Set("anthropic-version", anthropicAPIVersion)
	if payload != nil {
		request.Header.Set("Content-Type", "application/json")
	}

	response, err := c.httpClient.Do(request)
	if err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return fmt.Errorf("anthropic request canceled: %w", ctxErr)
		}
		return errors.New("anthropic request failed")
	}
	responseBody, err := readAnthropicResponse(response)
	if err != nil {
		return err
	}
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return fmt.Errorf("anthropic request failed with status %d", response.StatusCode)
	}
	if out == nil {
		return nil
	}
	if err := json.Unmarshal(responseBody, out); err != nil {
		return errors.New("anthropic returned an invalid response")
	}
	return nil
}

func anthropicEndpoint(config map[string]string, resource string, query url.Values) (string, error) {
	baseURL := strings.TrimSpace(config["base_url"])
	if baseURL == "" {
		baseURL = defaultAnthropicBaseURL
	}
	parsed, err := url.Parse(baseURL)
	if err != nil || !parsed.IsAbs() || parsed.Host == "" {
		return "", errors.New("anthropic base URL must be absolute")
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return "", errors.New("anthropic base URL must use HTTP or HTTPS")
	}
	if parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return "", errors.New("anthropic base URL cannot contain credentials, a query, or a fragment")
	}

	basePath := strings.TrimRight(parsed.Path, "/")
	if strings.HasSuffix(basePath, "/v1") {
		parsed.Path = basePath + "/" + resource
	} else {
		parsed.Path = basePath + "/v1/" + resource
	}
	parsed.RawPath = ""
	parsed.RawQuery = query.Encode()
	return parsed.String(), nil
}

func readAnthropicResponse(response *http.Response) ([]byte, error) {
	if response.Body == nil {
		return nil, errors.New("anthropic returned an empty response body")
	}
	body, readErr := io.ReadAll(io.LimitReader(response.Body, maxAnthropicResponseBytes+1))
	closeErr := response.Body.Close()
	if readErr != nil {
		return nil, errors.New("failed to read anthropic response")
	}
	if closeErr != nil {
		return nil, errors.New("failed to close anthropic response")
	}
	if len(body) > maxAnthropicResponseBytes {
		return nil, errors.New("anthropic response exceeded the size limit")
	}
	return body, nil
}

var _ plugin.AIAdapterClient = (*anthropicClient)(nil)
