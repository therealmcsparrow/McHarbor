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
	"strconv"
	"strings"
	"time"

	"github.com/therealmcsparrow/mcharbor/core/plugin"
)

const (
	defaultGeminiBaseURL  = "https://generativelanguage.googleapis.com/v1beta/openai"
	defaultGeminiModel    = "gemini-2.5-flash"
	geminiRequestTimeout  = 30 * time.Second
	geminiMaxResponseSize = 4 << 20
	geminiMaxAttempts     = 3
	geminiRetryBaseDelay  = 750 * time.Millisecond
	geminiMaxRetryDelay   = 60 * time.Second
)

// geminiClient speaks the OpenAI-compatible chat-completions dialect
// exposed by Google Gemini: POST <base>/chat/completions for replies and
// GET <base>/models for the catalog.
type geminiClient struct {
	httpClient *http.Client
}

type geminiMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
	Name    string `json:"name,omitempty"`
}

type geminiChatRequest struct {
	Model       string          `json:"model"`
	Messages    []geminiMessage `json:"messages"`
	MaxTokens   int             `json:"max_tokens,omitempty"`
	Temperature float32         `json:"temperature"`
	Tools       []geminiTool    `json:"tools,omitempty"`
	ToolChoice  string          `json:"tool_choice,omitempty"`
}

type geminiTool struct {
	Type     string         `json:"type"`
	Function geminiToolFunc `json:"function"`
}

type geminiToolFunc struct {
	Name        string         `json:"name"`
	Description string         `json:"description,omitempty"`
	Parameters  map[string]any `json:"parameters"`
}

type geminiToolCall struct {
	ID       string `json:"id"`
	Type     string `json:"type"`
	Function struct {
		Name      string `json:"name"`
		Arguments string `json:"arguments"`
	} `json:"function"`
}

type geminiChatResponse struct {
	Model   string `json:"model"`
	Choices []struct {
		Message struct {
			Role      string           `json:"role"`
			Content   string           `json:"content"`
			ToolCalls []geminiToolCall `json:"tool_calls"`
		} `json:"message"`
		FinishReason string `json:"finish_reason"`
	} `json:"choices"`
	Usage struct {
		PromptTokens     int `json:"prompt_tokens"`
		CompletionTokens int `json:"completion_tokens"`
		TotalTokens      int `json:"total_tokens"`
	} `json:"usage"`
}

func newGeminiClient() *geminiClient {
	return &geminiClient{httpClient: &http.Client{
		Timeout: geminiRequestTimeout,
		CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}}
}

func (c *geminiClient) Test(ctx context.Context, config map[string]string) (*plugin.AITestResult, error) {
	return c.TestConnection(ctx, config)
}

// TestConnection sends a small completion to validate the URL, the
// credentials, and the configured model in one round trip.
func (c *geminiClient) TestConnection(ctx context.Context, config map[string]string) (*plugin.AITestResult, error) {
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
		Reply:     truncateGeminiText(strings.TrimSpace(result.Content), 256),
		Message:   "connection successful",
	}, nil
}

func (c *geminiClient) Complete(ctx context.Context, config map[string]string, llmReq plugin.LLMRequest) (*plugin.LLMResponse, error) {
	model := geminiModelName(config, llmReq.Model)
	messages := make([]geminiMessage, 0, len(llmReq.Messages)+1)
	if system := strings.TrimSpace(llmReq.System); system != "" {
		messages = append(messages, geminiMessage{Role: "system", Content: system})
	}
	for _, message := range llmReq.Messages {
		messages = append(messages, geminiMessage{
			Role:    strings.TrimSpace(message.Role),
			Content: message.Content,
			Name:    strings.TrimSpace(message.Name),
		})
	}
	if len(messages) == 0 {
		return nil, errors.New("gemini: at least one message is required")
	}
	if llmReq.MaxTokens < 0 {
		return nil, errors.New("gemini: max tokens cannot be negative")
	}

	requestBody := geminiChatRequest{
		Model:       model,
		Messages:    messages,
		MaxTokens:   llmReq.MaxTokens,
		Temperature: llmReq.Temperature,
	}
	if len(llmReq.Tools) > 0 {
		requestBody.Tools = geminiTools(llmReq.Tools)
		requestBody.ToolChoice = "auto"
	}

	var responseBody geminiChatResponse
	if err := c.doJSON(ctx, config, http.MethodPost, "/chat/completions", requestBody, &responseBody); err != nil {
		return nil, err
	}
	if len(responseBody.Choices) == 0 {
		return nil, errors.New("gemini: completion returned no choices")
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

func geminiTools(tools []plugin.LLMTool) []geminiTool {
	out := make([]geminiTool, 0, len(tools))
	for _, tool := range tools {
		name := strings.TrimSpace(tool.Name)
		if name == "" {
			continue
		}
		params := tool.Parameters
		if params == nil {
			params = map[string]any{"type": "object", "properties": map[string]any{}}
		}
		out = append(out, geminiTool{
			Type: "function",
			Function: geminiToolFunc{
				Name:        name,
				Description: strings.TrimSpace(tool.Description),
				Parameters:  params,
			},
		})
	}
	return out
}

func (c *geminiClient) Models(ctx context.Context, config map[string]string) ([]plugin.ModelDecl, error) {
	return c.ListModels(ctx, config)
}

// geminiModel covers the shape differences between OpenAI-compatible
// /models implementations: the context window is reported as
// context_length, context_window, or max_context_length depending on
// the vendor, and some vendors carry a human display name.
type geminiModel struct {
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
func (c *geminiClient) ListModels(ctx context.Context, config map[string]string) ([]plugin.ModelDecl, error) {
	var raw json.RawMessage
	if err := c.doJSON(ctx, config, http.MethodGet, "/models", nil, &raw); err != nil {
		return nil, err
	}

	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 {
		return nil, errors.New("gemini: models response was empty")
	}
	var models []geminiModel
	if trimmed[0] == '[' {
		if err := json.Unmarshal(trimmed, &models); err != nil {
			return nil, errors.New("gemini: models response was invalid")
		}
	} else {
		var envelope struct {
			Data []geminiModel `json:"data"`
		}
		if err := json.Unmarshal(trimmed, &envelope); err != nil {
			return nil, errors.New("gemini: models response was invalid")
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
			ContextWindow: geminiContextWindow(model),
			Capabilities:  geminiModelCapabilities(),
		})
	}
	sort.Slice(result, func(i, j int) bool { return result[i].ID < result[j].ID })
	return result, nil
}

// geminiContextWindow reads whichever context-length field the vendor
// populated. Zero means "not reported"; the host renders it as unknown.
func geminiContextWindow(model geminiModel) int {
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

func geminiModelCapabilities() []string {
	capabilities := []string{"chat"}
	capabilities = append(capabilities, "tools")
	capabilities = append(capabilities, "vision")
	return capabilities
}

func (c *geminiClient) doJSON(ctx context.Context, config map[string]string, method, endpointPath string, body, destination any) error {
	endpoint, err := geminiEndpoint(config, endpointPath)
	if err != nil {
		return err
	}
	apiKey := strings.TrimSpace(config["api_key"])
	if apiKey == "" {
		return errors.New("gemini: API key is required")
	}

	var encoded []byte
	if body != nil {
		marshalled, marshalErr := json.Marshal(body)
		if marshalErr != nil {
			return errors.New("gemini: request could not be encoded")
		}
		encoded = marshalled
	}

	var lastErr error
	var serverDelay time.Duration
	for attempt := 1; attempt <= geminiMaxAttempts; attempt++ {
		if attempt > 1 {
			// Google answers 503/UNAVAILABLE whenever the model is momentarily
			// overloaded; the assistant fires up to six tool-calling rounds per
			// turn, so one unretried blip kills the whole answer.
			delay := geminiRetryBaseDelay * time.Duration(1<<(attempt-2))
			// A quota rejection names its own cooldown, and it is far longer
			// than any backoff worth guessing. Waiting less just burns another
			// request against the same exhausted quota.
			if serverDelay > delay {
				delay = serverDelay
			}
			if delay > geminiMaxRetryDelay {
				delay = geminiMaxRetryDelay
			}
			if deadline, ok := ctx.Deadline(); ok && time.Until(deadline) <= delay {
				return lastErr
			}
			timer := time.NewTimer(delay)
			select {
			case <-ctx.Done():
				timer.Stop()
				return fmt.Errorf("gemini: request canceled: %w", ctx.Err())
			case <-timer.C:
			}
		}

		var requestBody io.Reader
		if encoded != nil {
			requestBody = bytes.NewReader(encoded)
		}
		req, reqErr := http.NewRequestWithContext(ctx, method, endpoint, requestBody)
		if reqErr != nil {
			return errors.New("gemini: request could not be created")
		}
		req.Header.Set("Accept", "application/json")
		if apiKey != "" {
			req.Header.Set("Authorization", "Bearer "+apiKey)
		}
		if encoded != nil {
			req.Header.Set("Content-Type", "application/json")
		}

		resp, doErr := c.httpClient.Do(req)
		if doErr != nil {
			if ctxErr := ctx.Err(); ctxErr != nil {
				return fmt.Errorf("gemini: request canceled: %w", ctxErr)
			}
			lastErr = errors.New("gemini: API request failed")
			continue
		}
		responseBody, readErr := readGeminiResponse(resp)
		if readErr != nil {
			return readErr
		}
		if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
			statusErr := geminiStatusError(resp.StatusCode, responseBody)
			if !geminiRetryableStatus(resp.StatusCode) {
				return statusErr
			}
			serverDelay = geminiRetryAfter(resp.Header, responseBody)
			lastErr = statusErr
			continue
		}
		if destination == nil {
			return nil
		}
		if err := json.Unmarshal(responseBody, destination); err != nil {
			return errors.New("gemini: API response was invalid")
		}
		return nil
	}
	return lastErr
}

func geminiRetryableStatus(status int) bool {
	switch status {
	case http.StatusTooManyRequests,
		http.StatusInternalServerError,
		http.StatusBadGateway,
		http.StatusServiceUnavailable,
		http.StatusGatewayTimeout:
		return true
	default:
		return false
	}
}

func geminiStatusError(status int, body []byte) error {
	switch status {
	case http.StatusUnauthorized, http.StatusForbidden:
		return errors.New("gemini: authentication failed")
	case http.StatusTooManyRequests:
		if detail := geminiErrorMessage(body); detail != "" {
			return fmt.Errorf("gemini: rate limit exceeded: %s", detail)
		}
		return errors.New("gemini: rate limit exceeded")
	}
	if detail := geminiErrorMessage(body); detail != "" {
		return fmt.Errorf("gemini: API returned HTTP status %d: %s", status, detail)
	}
	return fmt.Errorf("gemini: API returned HTTP status %d", status)
}

func geminiRetryAfter(header http.Header, body []byte) time.Duration {
	if raw := strings.TrimSpace(header.Get("Retry-After")); raw != "" {
		if seconds, err := strconv.Atoi(raw); err == nil && seconds > 0 {
			return time.Duration(seconds) * time.Second
		}
	}
	var envelope struct {
		Error struct {
			Details []struct {
				Type       string `json:"@type"`
				RetryDelay string `json:"retryDelay"`
			} `json:"details"`
		} `json:"error"`
	}
	if err := json.Unmarshal(body, &envelope); err != nil {
		return 0
	}
	for _, detail := range envelope.Error.Details {
		if delay := geminiParseRetryDelay(detail.RetryDelay); delay > 0 {
			return delay
		}
	}
	return 0
}

// geminiParseRetryDelay reads the protobuf Duration spelling Google uses in
// RetryInfo ("38s", "1.5s"), which time.ParseDuration accepts as-is.
func geminiParseRetryDelay(value string) time.Duration {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return 0
	}
	delay, err := time.ParseDuration(trimmed)
	if err != nil || delay <= 0 {
		return 0
	}
	return delay
}

func geminiErrorMessage(body []byte) string {
	var envelope struct {
		Error struct {
			Message string `json:"message"`
			Status  string `json:"status"`
		} `json:"error"`
	}
	if err := json.Unmarshal(body, &envelope); err != nil {
		return ""
	}
	message := strings.TrimSpace(envelope.Error.Message)
	if message == "" {
		message = strings.TrimSpace(envelope.Error.Status)
	}
	return truncateGeminiText(message, 256)
}

func geminiEndpoint(config map[string]string, endpointPath string) (string, error) {
	baseURL := strings.TrimSpace(config["base_url"])
	if baseURL == "" {
		baseURL = defaultGeminiBaseURL
	}
	parsed, err := url.ParseRequestURI(baseURL)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return "", errors.New("gemini: base URL must be an absolute HTTP or HTTPS URL")
	}
	if parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return "", errors.New("gemini: base URL cannot contain credentials, a query, or a fragment")
	}
	return strings.TrimRight(parsed.String(), "/") + endpointPath, nil
}

func geminiModelName(config map[string]string, requested string) string {
	if model := strings.TrimSpace(requested); model != "" {
		return model
	}
	if model := strings.TrimSpace(config["default_model"]); model != "" {
		return model
	}
	return defaultGeminiModel
}

func readGeminiResponse(resp *http.Response) ([]byte, error) {
	body, readErr := io.ReadAll(io.LimitReader(resp.Body, geminiMaxResponseSize+1))
	closeErr := resp.Body.Close()
	if readErr != nil {
		return nil, errors.New("gemini: API response could not be read")
	}
	if closeErr != nil {
		return nil, errors.New("gemini: API response could not be closed")
	}
	if len(body) > geminiMaxResponseSize {
		return nil, errors.New("gemini: API response exceeded the size limit")
	}
	return body, nil
}

func truncateGeminiText(value string, maxRunes int) string {
	runes := []rune(value)
	if maxRunes <= 0 || len(runes) <= maxRunes {
		return value
	}
	return string(runes[:maxRunes])
}

var _ plugin.AIAdapterClient = (*geminiClient)(nil)
