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
	defaultOpenAIBaseURL = "https://api.openai.com/v1"
	defaultOpenAIModel   = "gpt-4o-mini"
	openAIHTTPTimeout    = 120 * time.Second
	modelsBodyLimit      = 4 << 20
	completionBodyLimit  = 8 << 20
)

type openAIClient struct {
	httpClient *http.Client
}

type openAIConfig struct {
	apiKey       string
	baseURL      string
	defaultModel string
}

type openAIMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
	Name    string `json:"name,omitempty"`
}

type chatCompletionRequest struct {
	Model               string          `json:"model"`
	Messages            []openAIMessage `json:"messages"`
	MaxCompletionTokens int             `json:"max_completion_tokens,omitempty"`
	Temperature         *float32        `json:"temperature,omitempty"`
	Tools               []openAITool    `json:"tools,omitempty"`
	ToolChoice          string          `json:"tool_choice,omitempty"`
}

type openAITool struct {
	Type     string         `json:"type"`
	Function openAIToolFunc `json:"function"`
}

type openAIToolFunc struct {
	Name        string         `json:"name"`
	Description string         `json:"description,omitempty"`
	Parameters  map[string]any `json:"parameters"`
}

type openAIToolCall struct {
	ID       string `json:"id"`
	Type     string `json:"type"`
	Function struct {
		Name      string `json:"name"`
		Arguments string `json:"arguments"`
	} `json:"function"`
}

type chatCompletionResponse struct {
	Model   string `json:"model"`
	Choices []struct {
		Message struct {
			Role      string           `json:"role"`
			Content   string           `json:"content"`
			ToolCalls []openAIToolCall `json:"tool_calls"`
		} `json:"message"`
		FinishReason string `json:"finish_reason"`
	} `json:"choices"`
	Usage struct {
		PromptTokens     int `json:"prompt_tokens"`
		CompletionTokens int `json:"completion_tokens"`
		TotalTokens      int `json:"total_tokens"`
	} `json:"usage"`
}

type modelsResponse struct {
	Data []struct {
		ID string `json:"id"`
	} `json:"data"`
}

func newOpenAIClient() *openAIClient {
	return &openAIClient{
		httpClient: &http.Client{
			Timeout: openAIHTTPTimeout,
			CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
				return http.ErrUseLastResponse
			},
		},
	}
}

// Models implements plugin.AIAdapterClient.
func (c *openAIClient) Models(ctx context.Context, config map[string]string) ([]plugin.ModelDecl, error) {
	return c.ListModels(ctx, config)
}

// ListModels returns the models visible to the configured API key.
func (c *openAIClient) ListModels(ctx context.Context, config map[string]string) ([]plugin.ModelDecl, error) {
	cfg, err := resolveOpenAIConfig(config)
	if err != nil {
		return nil, err
	}

	var upstream modelsResponse
	if err := c.doJSON(ctx, http.MethodGet, cfg.baseURL+"/models", cfg.apiKey, nil, modelsBodyLimit, &upstream); err != nil {
		return nil, fmt.Errorf("ai-provider-openai: list models: %w", err)
	}

	models := make([]plugin.ModelDecl, 0, len(upstream.Data))
	for _, model := range upstream.Data {
		modelID := strings.TrimSpace(model.ID)
		if modelID == "" {
			continue
		}
		models = append(models, plugin.ModelDecl{
			ID:          modelID,
			DisplayName: modelID,
		})
	}
	sort.Slice(models, func(i, j int) bool {
		return models[i].ID < models[j].ID
	})
	return models, nil
}

// Test implements plugin.AIAdapterClient.
func (c *openAIClient) Test(ctx context.Context, config map[string]string) (*plugin.AITestResult, error) {
	return c.TestConnection(ctx, config)
}

// TestConnection sends a small completion through the configured model.
func (c *openAIClient) TestConnection(ctx context.Context, config map[string]string) (*plugin.AITestResult, error) {
	startedAt := time.Now()
	response, err := c.Complete(ctx, config, plugin.LLMRequest{
		Messages: []plugin.LLMMessage{
			{Role: "user", Content: "Reply with OK."},
		},
		MaxTokens: 16,
	})
	latencyMs := int(time.Since(startedAt).Milliseconds())
	if err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return nil, err
		}
		return &plugin.AITestResult{
			Success:   false,
			LatencyMs: latencyMs,
			Message:   "OpenAI connection test failed; verify the API key, base URL, and model.",
		}, nil
	}

	reply := strings.TrimSpace(response.Content)
	replyRunes := []rune(reply)
	if len(replyRunes) > 200 {
		reply = string(replyRunes[:200])
	}
	return &plugin.AITestResult{
		Success:   true,
		LatencyMs: latencyMs,
		Reply:     reply,
		Message:   "OpenAI connection is ready.",
	}, nil
}

// Complete implements plugin.AIAdapterClient using Chat Completions.
func (c *openAIClient) Complete(ctx context.Context, config map[string]string, request plugin.LLMRequest) (*plugin.LLMResponse, error) {
	cfg, err := resolveOpenAIConfig(config)
	if err != nil {
		return nil, err
	}
	if request.MaxTokens < 0 {
		return nil, errors.New("ai-provider-openai: max tokens cannot be negative")
	}
	if request.Temperature < 0 || request.Temperature > 2 {
		return nil, errors.New("ai-provider-openai: temperature must be between 0 and 2")
	}

	model := strings.TrimSpace(request.Model)
	if model == "" {
		model = cfg.defaultModel
	}
	messages := make([]openAIMessage, 0, len(request.Messages)+1)
	if system := strings.TrimSpace(request.System); system != "" {
		messages = append(messages, openAIMessage{Role: "system", Content: system})
	}
	for _, message := range request.Messages {
		role := strings.TrimSpace(message.Role)
		if role == "" {
			return nil, errors.New("ai-provider-openai: message role is required")
		}
		messages = append(messages, openAIMessage{
			Role:    role,
			Content: message.Content,
			Name:    strings.TrimSpace(message.Name),
		})
	}
	if len(messages) == 0 {
		return nil, errors.New("ai-provider-openai: at least one message is required")
	}

	payload := chatCompletionRequest{
		Model:               model,
		Messages:            messages,
		MaxCompletionTokens: request.MaxTokens,
	}
	if len(request.Tools) > 0 {
		payload.Tools = openAITools(request.Tools)
		payload.ToolChoice = "auto"
	}
	if request.Temperature > 0 {
		temperature := request.Temperature
		payload.Temperature = &temperature
	}

	var upstream chatCompletionResponse
	if err := c.doJSON(ctx, http.MethodPost, cfg.baseURL+"/chat/completions", cfg.apiKey, payload, completionBodyLimit, &upstream); err != nil {
		return nil, fmt.Errorf("ai-provider-openai: complete: %w", err)
	}
	if len(upstream.Choices) == 0 {
		return nil, errors.New("ai-provider-openai: completion returned no choices")
	}

	choice := upstream.Choices[0]
	responseModel := strings.TrimSpace(upstream.Model)
	if responseModel == "" {
		responseModel = model
	}
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
		Model:     responseModel,
		Usage: plugin.LLMUsage{
			InputTokens:  upstream.Usage.PromptTokens,
			OutputTokens: upstream.Usage.CompletionTokens,
			TotalTokens:  upstream.Usage.TotalTokens,
		},
		FinishReason: choice.FinishReason,
	}, nil
}

func openAITools(tools []plugin.LLMTool) []openAITool {
	out := make([]openAITool, 0, len(tools))
	for _, tool := range tools {
		name := strings.TrimSpace(tool.Name)
		if name == "" {
			continue
		}
		params := tool.Parameters
		if params == nil {
			params = map[string]any{"type": "object", "properties": map[string]any{}}
		}
		out = append(out, openAITool{
			Type: "function",
			Function: openAIToolFunc{
				Name:        name,
				Description: strings.TrimSpace(tool.Description),
				Parameters:  params,
			},
		})
	}
	return out
}

func resolveOpenAIConfig(config map[string]string) (openAIConfig, error) {
	apiKey := strings.TrimSpace(config["api_key"])
	if apiKey == "" {
		return openAIConfig{}, errors.New("ai-provider-openai: API key is required")
	}

	baseURL := strings.TrimSpace(config["base_url"])
	if baseURL == "" {
		baseURL = defaultOpenAIBaseURL
	}
	parsed, err := url.Parse(baseURL)
	if err != nil || !parsed.IsAbs() || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return openAIConfig{}, errors.New("ai-provider-openai: base URL must be an absolute HTTP or HTTPS URL")
	}
	if parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return openAIConfig{}, errors.New("ai-provider-openai: base URL cannot contain credentials, a query, or a fragment")
	}

	model := strings.TrimSpace(config["default_model"])
	if model == "" {
		model = defaultOpenAIModel
	}
	return openAIConfig{
		apiKey:       apiKey,
		baseURL:      strings.TrimRight(parsed.String(), "/"),
		defaultModel: model,
	}, nil
}

func (c *openAIClient) doJSON(
	ctx context.Context,
	method string,
	endpoint string,
	apiKey string,
	payload any,
	bodyLimit int64,
	destination any,
) error {
	var body io.Reader
	if payload != nil {
		encoded, err := json.Marshal(payload)
		if err != nil {
			return fmt.Errorf("encode request: %w", err)
		}
		body = bytes.NewReader(encoded)
	}

	request, err := http.NewRequestWithContext(ctx, method, endpoint, body)
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}
	request.Header.Set("Accept", "application/json")
	request.Header.Set("Authorization", "Bearer "+apiKey)
	if payload != nil {
		request.Header.Set("Content-Type", "application/json")
	}

	response, err := c.httpClient.Do(request)
	if err != nil {
		return fmt.Errorf("request failed: %w", err)
	}
	responseBody, err := readAndCloseBody(response.Body, bodyLimit)
	if err != nil {
		return err
	}
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return upstreamStatusError(response.StatusCode)
	}
	if len(responseBody) == 0 {
		return errors.New("provider returned an empty response")
	}
	if err := json.Unmarshal(responseBody, destination); err != nil {
		return fmt.Errorf("decode response: %w", err)
	}
	return nil
}

func readAndCloseBody(body io.ReadCloser, limit int64) ([]byte, error) {
	data, readErr := io.ReadAll(io.LimitReader(body, limit+1))
	closeErr := body.Close()
	if readErr != nil {
		return nil, fmt.Errorf("read response: %w", readErr)
	}
	if closeErr != nil {
		return nil, fmt.Errorf("close response: %w", closeErr)
	}
	if int64(len(data)) > limit {
		return nil, errors.New("provider response exceeded the size limit")
	}
	return data, nil
}

func upstreamStatusError(statusCode int) error {
	switch statusCode {
	case http.StatusUnauthorized, http.StatusForbidden:
		return errors.New("provider authentication failed")
	case http.StatusNotFound:
		return errors.New("provider endpoint or model was not found")
	case http.StatusTooManyRequests:
		return errors.New("provider rate limit was exceeded")
	case http.StatusBadRequest, http.StatusUnprocessableEntity:
		return errors.New("provider rejected the request")
	default:
		if statusCode >= http.StatusInternalServerError {
			return errors.New("provider service is unavailable")
		}
		return fmt.Errorf("provider request failed with status %d", statusCode)
	}
}

var _ plugin.AIAdapterClient = (*openAIClient)(nil)
