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
	defaultMiniMaxBaseURL = "https://api.minimax.io/v1"
	defaultMiniMaxModel   = "MiniMax-M3"
	miniMaxHTTPTimeout    = 30 * time.Second
	maxResponseBodyBytes  = int64(4 << 20)
)

type miniMaxClient struct {
	httpClient *http.Client
}

type miniMaxMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
	Name    string `json:"name,omitempty"`
}

type miniMaxCompletionRequest struct {
	Model               string           `json:"model"`
	Messages            []miniMaxMessage `json:"messages"`
	MaxCompletionTokens int              `json:"max_completion_tokens,omitempty"`
	Temperature         float32          `json:"temperature"`
	Stream              bool             `json:"stream"`
	Tools               []miniMaxTool    `json:"tools,omitempty"`
	ToolChoice          string           `json:"tool_choice,omitempty"`
}

type miniMaxTool struct {
	Type     string          `json:"type"`
	Function miniMaxToolFunc `json:"function"`
}

type miniMaxToolFunc struct {
	Name        string         `json:"name"`
	Description string         `json:"description,omitempty"`
	Parameters  map[string]any `json:"parameters"`
}

type miniMaxToolCall struct {
	ID       string `json:"id"`
	Type     string `json:"type"`
	Function struct {
		Name      string `json:"name"`
		Arguments string `json:"arguments"`
	} `json:"function"`
}

type miniMaxCompletionResponse struct {
	Model   string `json:"model"`
	Choices []struct {
		Message struct {
			Content   string            `json:"content"`
			ToolCalls []miniMaxToolCall `json:"tool_calls"`
		} `json:"message"`
		FinishReason string `json:"finish_reason"`
	} `json:"choices"`
	Usage struct {
		PromptTokens     int `json:"prompt_tokens"`
		CompletionTokens int `json:"completion_tokens"`
		TotalTokens      int `json:"total_tokens"`
	} `json:"usage"`
}

type miniMaxModelsResponse struct {
	Data []struct {
		ID string `json:"id"`
	} `json:"data"`
}

func newMiniMaxClient() *miniMaxClient {
	return &miniMaxClient{httpClient: &http.Client{
		Timeout: miniMaxHTTPTimeout,
		CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}}
}

func (c *miniMaxClient) Models(ctx context.Context, config map[string]string) ([]plugin.ModelDecl, error) {
	baseURL, apiKey, err := miniMaxConfig(config)
	if err != nil {
		return nil, err
	}

	body, err := c.request(ctx, http.MethodGet, baseURL+"/models", apiKey, nil)
	if err != nil {
		return nil, err
	}

	var response miniMaxModelsResponse
	if err := json.Unmarshal(body, &response); err != nil {
		return nil, errors.New("minimax returned an invalid model list")
	}

	seen := make(map[string]struct{}, len(response.Data))
	models := make([]plugin.ModelDecl, 0, len(response.Data))
	for _, item := range response.Data {
		id := strings.TrimSpace(item.ID)
		if id == "" {
			continue
		}
		if _, exists := seen[id]; exists {
			continue
		}
		seen[id] = struct{}{}
		models = append(models, plugin.ModelDecl{ID: id, DisplayName: id})
	}
	sort.Slice(models, func(i, j int) bool { return models[i].ID < models[j].ID })
	return models, nil
}

func (c *miniMaxClient) Complete(ctx context.Context, config map[string]string, req plugin.LLMRequest) (*plugin.LLMResponse, error) {
	baseURL, apiKey, err := miniMaxConfig(config)
	if err != nil {
		return nil, err
	}
	if req.MaxTokens < 0 {
		return nil, errors.New("minimax max tokens cannot be negative")
	}
	if req.Temperature < 0 || req.Temperature > 2 {
		return nil, errors.New("minimax temperature must be between 0 and 2")
	}
	model := strings.TrimSpace(req.Model)
	if model == "" {
		model = miniMaxDefaultModel(config)
	}
	messages := make([]miniMaxMessage, 0, len(req.Messages)+1)
	if system := strings.TrimSpace(req.System); system != "" {
		messages = append(messages, miniMaxMessage{Role: "system", Content: system})
	}
	for _, message := range req.Messages {
		role := strings.TrimSpace(message.Role)
		if role == "" {
			return nil, errors.New("minimax message role is required")
		}
		messages = append(messages, miniMaxMessage{
			Role:    role,
			Content: message.Content,
			Name:    strings.TrimSpace(message.Name),
		})
	}
	if len(messages) == 0 {
		return nil, errors.New("minimax request requires at least one message")
	}

	payload := miniMaxCompletionRequest{
		Model:               model,
		Messages:            messages,
		MaxCompletionTokens: req.MaxTokens,
		Temperature:         req.Temperature,
		Stream:              false,
	}
	if len(req.Tools) > 0 {
		payload.Tools = miniMaxTools(req.Tools)
		payload.ToolChoice = "auto"
	}
	body, err := c.request(ctx, http.MethodPost, baseURL+"/chat/completions", apiKey, payload)
	if err != nil {
		return nil, err
	}

	var response miniMaxCompletionResponse
	if err := json.Unmarshal(body, &response); err != nil {
		return nil, errors.New("minimax returned an invalid completion response")
	}
	if len(response.Choices) == 0 {
		return nil, errors.New("minimax returned no completion choices")
	}
	if response.Model == "" {
		response.Model = model
	}
	choice := response.Choices[0]
	toolCalls := make([]plugin.LLMToolCall, 0, len(choice.Message.ToolCalls))
	for _, call := range choice.Message.ToolCalls {
		name := strings.TrimSpace(call.Function.Name)
		if name == "" {
			continue
		}
		args := json.RawMessage(call.Function.Arguments)
		if len(args) == 0 {
			args = json.RawMessage(`{}`)
		}
		toolCalls = append(toolCalls, plugin.LLMToolCall{
			ID:        strings.TrimSpace(call.ID),
			Name:      name,
			Arguments: args,
		})
	}

	return &plugin.LLMResponse{
		Content:   choice.Message.Content,
		ToolCalls: toolCalls,
		Model:     response.Model,
		Usage: plugin.LLMUsage{
			InputTokens:  response.Usage.PromptTokens,
			OutputTokens: response.Usage.CompletionTokens,
			TotalTokens:  response.Usage.TotalTokens,
		},
		FinishReason: choice.FinishReason,
	}, nil
}

func miniMaxTools(tools []plugin.LLMTool) []miniMaxTool {
	out := make([]miniMaxTool, 0, len(tools))
	for _, tool := range tools {
		name := strings.TrimSpace(tool.Name)
		if name == "" {
			continue
		}
		params := tool.Parameters
		if params == nil {
			params = map[string]any{"type": "object", "properties": map[string]any{}}
		}
		out = append(out, miniMaxTool{
			Type: "function",
			Function: miniMaxToolFunc{
				Name:        name,
				Description: strings.TrimSpace(tool.Description),
				Parameters:  params,
			},
		})
	}
	return out
}

func (c *miniMaxClient) Test(ctx context.Context, config map[string]string) (*plugin.AITestResult, error) {
	startedAt := time.Now()
	if _, _, err := miniMaxConfig(config); err != nil {
		return &plugin.AITestResult{
			Success:   false,
			LatencyMs: elapsedMilliseconds(startedAt),
			Message:   err.Error(),
		}, nil
	}

	response, err := c.Complete(ctx, config, plugin.LLMRequest{
		Model:     miniMaxDefaultModel(config),
		Messages:  []plugin.LLMMessage{{Role: "user", Content: "Reply with OK."}},
		MaxTokens: 8,
	})
	if err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return nil, ctxErr
		}
		return &plugin.AITestResult{
			Success:   false,
			LatencyMs: elapsedMilliseconds(startedAt),
			Message:   "MiniMax connection test failed. Verify the API key, base URL, and model.",
		}, nil
	}

	return &plugin.AITestResult{
		Success:   true,
		LatencyMs: elapsedMilliseconds(startedAt),
		Reply:     truncateRunes(strings.TrimSpace(response.Content), 160),
		Message:   "MiniMax connection successful.",
	}, nil
}

func miniMaxConfig(config map[string]string) (string, string, error) {
	apiKey := strings.TrimSpace(config["api_key"])
	if apiKey == "" {
		return "", "", errors.New("minimax API key is required")
	}

	baseURL := strings.TrimSpace(config["base_url"])
	if baseURL == "" {
		baseURL = defaultMiniMaxBaseURL
	}
	parsed, err := url.Parse(baseURL)
	if err != nil || !parsed.IsAbs() || parsed.Host == "" || parsed.Opaque != "" ||
		(parsed.Scheme != "http" && parsed.Scheme != "https") {
		return "", "", errors.New("minimax base URL must be an absolute HTTP or HTTPS URL")
	}
	if parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return "", "", errors.New("minimax base URL must not include credentials, query parameters, or fragments")
	}
	return strings.TrimRight(baseURL, "/"), apiKey, nil
}

func miniMaxDefaultModel(config map[string]string) string {
	if model := strings.TrimSpace(config["default_model"]); model != "" {
		return model
	}
	return defaultMiniMaxModel
}

func (c *miniMaxClient) request(
	ctx context.Context,
	method, endpoint, apiKey string,
	payload any,
) ([]byte, error) {
	var requestBody io.Reader
	if payload != nil {
		encoded, err := json.Marshal(payload)
		if err != nil {
			return nil, errors.New("minimax request could not be encoded")
		}
		requestBody = bytes.NewReader(encoded)
	}

	req, err := http.NewRequestWithContext(ctx, method, endpoint, requestBody)
	if err != nil {
		return nil, errors.New("minimax request could not be created")
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Authorization", "Bearer "+apiKey)
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return nil, ctxErr
		}
		return nil, errors.New("minimax request failed")
	}
	body, err := readAndCloseResponse(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return nil, fmt.Errorf("minimax API returned HTTP %d", resp.StatusCode)
	}
	return body, nil
}

func readAndCloseResponse(body io.ReadCloser) ([]byte, error) {
	responseBody, readErr := io.ReadAll(io.LimitReader(body, maxResponseBodyBytes+1))
	closeErr := body.Close()
	if readErr != nil {
		return nil, errors.New("minimax response could not be read")
	}
	if closeErr != nil {
		return nil, errors.New("minimax response could not be closed")
	}
	if int64(len(responseBody)) > maxResponseBodyBytes {
		return nil, errors.New("minimax response exceeded the size limit")
	}
	return responseBody, nil
}

func elapsedMilliseconds(startedAt time.Time) int {
	return int(time.Since(startedAt) / time.Millisecond)
}

func truncateRunes(value string, max int) string {
	runes := []rune(value)
	if len(runes) <= max {
		return value
	}
	return string(runes[:max])
}

var _ plugin.AIAdapterClient = (*miniMaxClient)(nil)
