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
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"

	"github.com/therealmcsparrow/mcharbor/core/plugin"
)

const (
	defaultLmstudioBaseURL  = "http://localhost:1234/v1"
	defaultLmstudioModel    = "local-model"
	lmstudioRequestTimeout  = 30 * time.Second
	lmstudioMaxResponseSize = 4 << 20
)

// lmstudioClient speaks the OpenAI-compatible chat-completions dialect
// exposed by LM Studio (local): POST <base>/chat/completions for replies and
// GET <base>/models for the catalog.
type lmstudioClient struct {
	httpClient *http.Client
}

type lmstudioMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
	Name    string `json:"name,omitempty"`
}

type lmstudioChatRequest struct {
	Model       string            `json:"model"`
	Messages    []lmstudioMessage `json:"messages"`
	MaxTokens   int               `json:"max_tokens,omitempty"`
	Temperature float32           `json:"temperature"`
	Tools       []lmstudioTool    `json:"tools,omitempty"`
	ToolChoice  string            `json:"tool_choice,omitempty"`
}

type lmstudioTool struct {
	Type     string           `json:"type"`
	Function lmstudioToolFunc `json:"function"`
}

type lmstudioToolFunc struct {
	Name        string         `json:"name"`
	Description string         `json:"description,omitempty"`
	Parameters  map[string]any `json:"parameters"`
}

type lmstudioToolCall struct {
	ID       string `json:"id"`
	Type     string `json:"type"`
	Function struct {
		Name      string `json:"name"`
		Arguments string `json:"arguments"`
	} `json:"function"`
}

type lmstudioChatResponse struct {
	Model   string `json:"model"`
	Choices []struct {
		Message struct {
			Role      string             `json:"role"`
			Content   string             `json:"content"`
			ToolCalls []lmstudioToolCall `json:"tool_calls"`
		} `json:"message"`
		FinishReason string `json:"finish_reason"`
	} `json:"choices"`
	Usage struct {
		PromptTokens     int `json:"prompt_tokens"`
		CompletionTokens int `json:"completion_tokens"`
		TotalTokens      int `json:"total_tokens"`
	} `json:"usage"`
}

func newLmstudioClient() *lmstudioClient {
	return &lmstudioClient{httpClient: &http.Client{
		Timeout: lmstudioRequestTimeout,
		CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}}
}

func (c *lmstudioClient) Test(ctx context.Context, config map[string]string) (*plugin.AITestResult, error) {
	return c.TestConnection(ctx, config)
}

// TestConnection sends a small completion to validate the URL, the
// credentials, and the configured model in one round trip.
func (c *lmstudioClient) TestConnection(ctx context.Context, config map[string]string) (*plugin.AITestResult, error) {
	startedAt := time.Now()
	result, err := c.Complete(ctx, config, plugin.LLMRequest{
		Messages:    []plugin.LLMMessage{{Role: "user", Content: "Reply with OK."}},
		MaxTokens:   16,
		Temperature: 0,
	})
	latency := int(time.Since(startedAt) / time.Millisecond)
	if err != nil {
		return &plugin.AITestResult{Success: false, LatencyMs: latency, Message: err.Error()}, nil
	}
	return &plugin.AITestResult{
		Success:   true,
		LatencyMs: latency,
		Reply:     truncateLmstudioText(strings.TrimSpace(result.Content), 256),
		Message:   "connection successful",
	}, nil
}

func (c *lmstudioClient) Complete(ctx context.Context, config map[string]string, llmReq plugin.LLMRequest) (*plugin.LLMResponse, error) {
	model := lmstudioModelName(config, llmReq.Model)
	messages := make([]lmstudioMessage, 0, len(llmReq.Messages)+1)
	if system := strings.TrimSpace(llmReq.System); system != "" {
		messages = append(messages, lmstudioMessage{Role: "system", Content: system})
	}
	for _, message := range llmReq.Messages {
		messages = append(messages, lmstudioMessage{
			Role:    strings.TrimSpace(message.Role),
			Content: message.Content,
			Name:    strings.TrimSpace(message.Name),
		})
	}
	if len(messages) == 0 {
		return nil, errors.New("lmstudio: at least one message is required")
	}
	if llmReq.MaxTokens < 0 {
		return nil, errors.New("lmstudio: max tokens cannot be negative")
	}

	requestBody := lmstudioChatRequest{
		Model:       model,
		Messages:    messages,
		MaxTokens:   llmReq.MaxTokens,
		Temperature: llmReq.Temperature,
	}
	if len(llmReq.Tools) > 0 {
		requestBody.Tools = lmstudioTools(llmReq.Tools)
		requestBody.ToolChoice = "auto"
	}

	var responseBody lmstudioChatResponse
	if err := c.doJSON(ctx, config, http.MethodPost, "/chat/completions", requestBody, &responseBody); err != nil {
		return nil, err
	}
	if len(responseBody.Choices) == 0 {
		return nil, errors.New("lmstudio: completion returned no choices")
	}

	responseModel := strings.TrimSpace(responseBody.Model)
	if responseModel == "" {
		responseModel = model
	}
	choice := responseBody.Choices[0]
	toolCalls := make([]plugin.LLMToolCall, 0, len(choice.Message.ToolCalls))
	for _, call := range choice.Message.ToolCalls {
		name := strings.TrimSpace(call.Function.Name)
		if name == "" {
			continue
		}
		arguments := json.RawMessage(call.Function.Arguments)
		if len(arguments) == 0 {
			arguments = json.RawMessage(`{}`)
		}
		toolCalls = append(toolCalls, plugin.LLMToolCall{
			ID:        strings.TrimSpace(call.ID),
			Name:      name,
			Arguments: arguments,
		})
	}
	return &plugin.LLMResponse{
		Content:   choice.Message.Content,
		ToolCalls: toolCalls,
		Model:     responseModel,
		Usage: plugin.LLMUsage{
			InputTokens:  responseBody.Usage.PromptTokens,
			OutputTokens: responseBody.Usage.CompletionTokens,
			TotalTokens:  responseBody.Usage.TotalTokens,
		},
		FinishReason: choice.FinishReason,
	}, nil
}

func lmstudioTools(tools []plugin.LLMTool) []lmstudioTool {
	out := make([]lmstudioTool, 0, len(tools))
	for _, tool := range tools {
		name := strings.TrimSpace(tool.Name)
		if name == "" {
			continue
		}
		params := tool.Parameters
		if params == nil {
			params = map[string]any{"type": "object", "properties": map[string]any{}}
		}
		out = append(out, lmstudioTool{
			Type: "function",
			Function: lmstudioToolFunc{
				Name:        name,
				Description: strings.TrimSpace(tool.Description),
				Parameters:  params,
			},
		})
	}
	return out
}

func (c *lmstudioClient) Models(ctx context.Context, config map[string]string) ([]plugin.ModelDecl, error) {
	return c.ListModels(ctx, config)
}

// lmstudioModel covers the shape differences between OpenAI-compatible
// /models implementations: the context window is reported as
// context_length, context_window, or max_context_length depending on
// the vendor, and some vendors carry a human display name.
type lmstudioModel struct {
	ID               string `json:"id"`
	Name             string `json:"name"`
	DisplayName      string `json:"display_name"`
	ContextLength    int    `json:"context_length"`
	ContextWindow    int    `json:"context_window"`
	MaxContextLength int    `json:"max_context_length"`
}

// ListModels returns the chat models the endpoint advertises. The
// response is accepted both as a bare array and as the OpenAI
// `{"data": [...]}` envelope because vendors differ on which they emit.
func (c *lmstudioClient) ListModels(ctx context.Context, config map[string]string) ([]plugin.ModelDecl, error) {
	var raw json.RawMessage
	if err := c.doJSON(ctx, config, http.MethodGet, "/models", nil, &raw); err != nil {
		return nil, err
	}

	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 {
		return nil, errors.New("lmstudio: models response was empty")
	}
	var models []lmstudioModel
	if trimmed[0] == '[' {
		if err := json.Unmarshal(trimmed, &models); err != nil {
			return nil, errors.New("lmstudio: models response was invalid")
		}
	} else {
		var envelope struct {
			Data []lmstudioModel `json:"data"`
		}
		if err := json.Unmarshal(trimmed, &envelope); err != nil {
			return nil, errors.New("lmstudio: models response was invalid")
		}
		models = envelope.Data
	}

	result := make([]plugin.ModelDecl, 0, len(models))
	seen := make(map[string]struct{}, len(models))
	for _, model := range models {
		// Gemini's compatibility layer prefixes every id with
		// "models/"; the chat endpoint accepts the bare id.
		id := strings.TrimPrefix(strings.TrimSpace(model.ID), "models/")
		if id == "" {
			continue
		}
		if _, duplicate := seen[id]; duplicate {
			continue
		}
		seen[id] = struct{}{}
		displayName := strings.TrimSpace(model.DisplayName)
		if displayName == "" {
			displayName = strings.TrimSpace(model.Name)
		}
		// Gemini repeats the "models/" prefix in the human name too.
		displayName = strings.TrimPrefix(displayName, "models/")
		if displayName == "" {
			displayName = id
		}
		result = append(result, plugin.ModelDecl{
			ID:            id,
			DisplayName:   displayName,
			ContextWindow: lmstudioContextWindow(model),
			Capabilities:  lmstudioModelCapabilities(),
		})
	}
	sort.Slice(result, func(i, j int) bool { return result[i].ID < result[j].ID })
	return result, nil
}

// lmstudioContextWindow reads whichever context-length field the vendor
// populated. Zero means "not reported"; the host renders it as unknown.
func lmstudioContextWindow(model lmstudioModel) int {
	switch {
	case model.ContextLength > 0:
		return model.ContextLength
	case model.ContextWindow > 0:
		return model.ContextWindow
	case model.MaxContextLength > 0:
		return model.MaxContextLength
	default:
		return 0
	}
}

func lmstudioModelCapabilities() []string {
	capabilities := []string{"chat"}
	capabilities = append(capabilities, "tools")
	return capabilities
}

func (c *lmstudioClient) doJSON(ctx context.Context, config map[string]string, method, endpointPath string, body, destination any) error {
	endpoint, err := lmstudioEndpoint(config, endpointPath)
	if err != nil {
		return err
	}
	apiKey := strings.TrimSpace(config["api_key"])

	var requestBody io.Reader
	if body != nil {
		encoded, marshalErr := json.Marshal(body)
		if marshalErr != nil {
			return errors.New("lmstudio: request could not be encoded")
		}
		requestBody = bytes.NewReader(encoded)
	}
	req, err := http.NewRequestWithContext(ctx, method, endpoint, requestBody)
	if err != nil {
		return errors.New("lmstudio: request could not be created")
	}
	req.Header.Set("Accept", "application/json")
	if apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+apiKey)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return fmt.Errorf("lmstudio: request canceled: %w", ctxErr)
		}
		return errors.New("lmstudio: API request failed")
	}
	responseBody, err := readLmstudioResponse(resp)
	if err != nil {
		return err
	}
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		switch resp.StatusCode {
		case http.StatusUnauthorized, http.StatusForbidden:
			return errors.New("lmstudio: authentication failed")
		case http.StatusTooManyRequests:
			return errors.New("lmstudio: rate limit exceeded")
		default:
			return fmt.Errorf("lmstudio: API returned HTTP status %d", resp.StatusCode)
		}
	}
	if destination == nil {
		return nil
	}
	if err := json.Unmarshal(responseBody, destination); err != nil {
		return errors.New("lmstudio: API response was invalid")
	}
	return nil
}

func lmstudioEndpoint(config map[string]string, endpointPath string) (string, error) {
	baseURL := strings.TrimSpace(config["base_url"])
	if baseURL == "" {
		baseURL = defaultLmstudioBaseURL
	}
	parsed, err := url.ParseRequestURI(baseURL)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return "", errors.New("lmstudio: base URL must be an absolute HTTP or HTTPS URL")
	}
	if parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return "", errors.New("lmstudio: base URL cannot contain credentials, a query, or a fragment")
	}
	return strings.TrimRight(parsed.String(), "/") + endpointPath, nil
}

func lmstudioModelName(config map[string]string, requested string) string {
	if model := strings.TrimSpace(requested); model != "" {
		return model
	}
	if model := strings.TrimSpace(config["default_model"]); model != "" {
		return model
	}
	return defaultLmstudioModel
}

func readLmstudioResponse(resp *http.Response) ([]byte, error) {
	body, readErr := io.ReadAll(io.LimitReader(resp.Body, lmstudioMaxResponseSize+1))
	closeErr := resp.Body.Close()
	if readErr != nil {
		return nil, errors.New("lmstudio: API response could not be read")
	}
	if closeErr != nil {
		return nil, errors.New("lmstudio: API response could not be closed")
	}
	if len(body) > lmstudioMaxResponseSize {
		return nil, errors.New("lmstudio: API response exceeded the size limit")
	}
	return body, nil
}

func truncateLmstudioText(value string, maxRunes int) string {
	runes := []rune(value)
	if maxRunes <= 0 || len(runes) <= maxRunes {
		return value
	}
	return string(runes[:maxRunes])
}

var _ plugin.AIAdapterClient = (*lmstudioClient)(nil)
