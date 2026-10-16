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
	defaultFireworksBaseURL  = "https://api.fireworks.ai/inference/v1"
	defaultFireworksModel    = "accounts/fireworks/models/llama-v3p3-70b-instruct"
	fireworksRequestTimeout  = 30 * time.Second
	fireworksMaxResponseSize = 4 << 20
)

// fireworksClient speaks the OpenAI-compatible chat-completions dialect
// exposed by Fireworks AI: POST <base>/chat/completions for replies and
// GET <base>/models for the catalog.
type fireworksClient struct {
	httpClient *http.Client
}

type fireworksMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
	Name    string `json:"name,omitempty"`
}

type fireworksChatRequest struct {
	Model       string             `json:"model"`
	Messages    []fireworksMessage `json:"messages"`
	MaxTokens   int                `json:"max_tokens,omitempty"`
	Temperature float32            `json:"temperature"`
	Tools       []fireworksTool    `json:"tools,omitempty"`
	ToolChoice  string             `json:"tool_choice,omitempty"`
}

type fireworksTool struct {
	Type     string            `json:"type"`
	Function fireworksToolFunc `json:"function"`
}

type fireworksToolFunc struct {
	Name        string         `json:"name"`
	Description string         `json:"description,omitempty"`
	Parameters  map[string]any `json:"parameters"`
}

type fireworksToolCall struct {
	ID       string `json:"id"`
	Type     string `json:"type"`
	Function struct {
		Name      string `json:"name"`
		Arguments string `json:"arguments"`
	} `json:"function"`
}

type fireworksChatResponse struct {
	Model   string `json:"model"`
	Choices []struct {
		Message struct {
			Role      string              `json:"role"`
			Content   string              `json:"content"`
			ToolCalls []fireworksToolCall `json:"tool_calls"`
		} `json:"message"`
		FinishReason string `json:"finish_reason"`
	} `json:"choices"`
	Usage struct {
		PromptTokens     int `json:"prompt_tokens"`
		CompletionTokens int `json:"completion_tokens"`
		TotalTokens      int `json:"total_tokens"`
	} `json:"usage"`
}

func newFireworksClient() *fireworksClient {
	return &fireworksClient{httpClient: &http.Client{
		Timeout: fireworksRequestTimeout,
		CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}}
}

func (c *fireworksClient) Test(ctx context.Context, config map[string]string) (*plugin.AITestResult, error) {
	return c.TestConnection(ctx, config)
}

// TestConnection sends a small completion to validate the URL, the
// credentials, and the configured model in one round trip.
func (c *fireworksClient) TestConnection(ctx context.Context, config map[string]string) (*plugin.AITestResult, error) {
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
		Reply:     truncateFireworksText(strings.TrimSpace(result.Content), 256),
		Message:   "connection successful",
	}, nil
}

func (c *fireworksClient) Complete(ctx context.Context, config map[string]string, llmReq plugin.LLMRequest) (*plugin.LLMResponse, error) {
	model := fireworksModelName(config, llmReq.Model)
	messages := make([]fireworksMessage, 0, len(llmReq.Messages)+1)
	if system := strings.TrimSpace(llmReq.System); system != "" {
		messages = append(messages, fireworksMessage{Role: "system", Content: system})
	}
	for _, message := range llmReq.Messages {
		messages = append(messages, fireworksMessage{
			Role:    strings.TrimSpace(message.Role),
			Content: message.Content,
			Name:    strings.TrimSpace(message.Name),
		})
	}
	if len(messages) == 0 {
		return nil, errors.New("fireworks: at least one message is required")
	}
	if llmReq.MaxTokens < 0 {
		return nil, errors.New("fireworks: max tokens cannot be negative")
	}

	requestBody := fireworksChatRequest{
		Model:       model,
		Messages:    messages,
		MaxTokens:   llmReq.MaxTokens,
		Temperature: llmReq.Temperature,
	}
	if len(llmReq.Tools) > 0 {
		requestBody.Tools = fireworksTools(llmReq.Tools)
		requestBody.ToolChoice = "auto"
	}

	var responseBody fireworksChatResponse
	if err := c.doJSON(ctx, config, http.MethodPost, "/chat/completions", requestBody, &responseBody); err != nil {
		return nil, err
	}
	if len(responseBody.Choices) == 0 {
		return nil, errors.New("fireworks: completion returned no choices")
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

func fireworksTools(tools []plugin.LLMTool) []fireworksTool {
	out := make([]fireworksTool, 0, len(tools))
	for _, tool := range tools {
		name := strings.TrimSpace(tool.Name)
		if name == "" {
			continue
		}
		params := tool.Parameters
		if params == nil {
			params = map[string]any{"type": "object", "properties": map[string]any{}}
		}
		out = append(out, fireworksTool{
			Type: "function",
			Function: fireworksToolFunc{
				Name:        name,
				Description: strings.TrimSpace(tool.Description),
				Parameters:  params,
			},
		})
	}
	return out
}

func (c *fireworksClient) Models(ctx context.Context, config map[string]string) ([]plugin.ModelDecl, error) {
	return c.ListModels(ctx, config)
}

// fireworksModel covers the shape differences between OpenAI-compatible
// /models implementations: the context window is reported as
// context_length, context_window, or max_context_length depending on
// the vendor, and some vendors carry a human display name.
type fireworksModel struct {
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
func (c *fireworksClient) ListModels(ctx context.Context, config map[string]string) ([]plugin.ModelDecl, error) {
	var raw json.RawMessage
	if err := c.doJSON(ctx, config, http.MethodGet, "/models", nil, &raw); err != nil {
		return nil, err
	}

	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 {
		return nil, errors.New("fireworks: models response was empty")
	}
	var models []fireworksModel
	if trimmed[0] == '[' {
		if err := json.Unmarshal(trimmed, &models); err != nil {
			return nil, errors.New("fireworks: models response was invalid")
		}
	} else {
		var envelope struct {
			Data []fireworksModel `json:"data"`
		}
		if err := json.Unmarshal(trimmed, &envelope); err != nil {
			return nil, errors.New("fireworks: models response was invalid")
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
			ContextWindow: fireworksContextWindow(model),
			Capabilities:  fireworksModelCapabilities(),
		})
	}
	sort.Slice(result, func(i, j int) bool { return result[i].ID < result[j].ID })
	return result, nil
}

// fireworksContextWindow reads whichever context-length field the vendor
// populated. Zero means "not reported"; the host renders it as unknown.
func fireworksContextWindow(model fireworksModel) int {
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

func fireworksModelCapabilities() []string {
	capabilities := []string{"chat"}
	capabilities = append(capabilities, "tools")
	return capabilities
}

func (c *fireworksClient) doJSON(ctx context.Context, config map[string]string, method, endpointPath string, body, destination any) error {
	endpoint, err := fireworksEndpoint(config, endpointPath)
	if err != nil {
		return err
	}
	apiKey := strings.TrimSpace(config["api_key"])
	if apiKey == "" {
		return errors.New("fireworks: API key is required")
	}

	var requestBody io.Reader
	if body != nil {
		encoded, marshalErr := json.Marshal(body)
		if marshalErr != nil {
			return errors.New("fireworks: request could not be encoded")
		}
		requestBody = bytes.NewReader(encoded)
	}
	req, err := http.NewRequestWithContext(ctx, method, endpoint, requestBody)
	if err != nil {
		return errors.New("fireworks: request could not be created")
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
			return fmt.Errorf("fireworks: request canceled: %w", ctxErr)
		}
		return errors.New("fireworks: API request failed")
	}
	responseBody, err := readFireworksResponse(resp)
	if err != nil {
		return err
	}
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		switch resp.StatusCode {
		case http.StatusUnauthorized, http.StatusForbidden:
			return errors.New("fireworks: authentication failed")
		case http.StatusTooManyRequests:
			return errors.New("fireworks: rate limit exceeded")
		default:
			return fmt.Errorf("fireworks: API returned HTTP status %d", resp.StatusCode)
		}
	}
	if destination == nil {
		return nil
	}
	if err := json.Unmarshal(responseBody, destination); err != nil {
		return errors.New("fireworks: API response was invalid")
	}
	return nil
}

func fireworksEndpoint(config map[string]string, endpointPath string) (string, error) {
	baseURL := strings.TrimSpace(config["base_url"])
	if baseURL == "" {
		baseURL = defaultFireworksBaseURL
	}
	parsed, err := url.ParseRequestURI(baseURL)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return "", errors.New("fireworks: base URL must be an absolute HTTP or HTTPS URL")
	}
	if parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return "", errors.New("fireworks: base URL cannot contain credentials, a query, or a fragment")
	}
	return strings.TrimRight(parsed.String(), "/") + endpointPath, nil
}

func fireworksModelName(config map[string]string, requested string) string {
	if model := strings.TrimSpace(requested); model != "" {
		return model
	}
	if model := strings.TrimSpace(config["default_model"]); model != "" {
		return model
	}
	return defaultFireworksModel
}

func readFireworksResponse(resp *http.Response) ([]byte, error) {
	body, readErr := io.ReadAll(io.LimitReader(resp.Body, fireworksMaxResponseSize+1))
	closeErr := resp.Body.Close()
	if readErr != nil {
		return nil, errors.New("fireworks: API response could not be read")
	}
	if closeErr != nil {
		return nil, errors.New("fireworks: API response could not be closed")
	}
	if len(body) > fireworksMaxResponseSize {
		return nil, errors.New("fireworks: API response exceeded the size limit")
	}
	return body, nil
}

func truncateFireworksText(value string, maxRunes int) string {
	runes := []rune(value)
	if maxRunes <= 0 || len(runes) <= maxRunes {
		return value
	}
	return string(runes[:maxRunes])
}

var _ plugin.AIAdapterClient = (*fireworksClient)(nil)
