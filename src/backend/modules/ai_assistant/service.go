// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package ai_assistant

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"time"

	"github.com/rs/xid"

	"github.com/therealmcsparrow/mcharbor/core/audit"
	coredb "github.com/therealmcsparrow/mcharbor/core/db"
	"github.com/therealmcsparrow/mcharbor/core/plugin"
	"github.com/therealmcsparrow/mcharbor/core/rbac"
	ai_tools "github.com/therealmcsparrow/mcharbor/modules/ai_assistant/tools"
)

// assistantQuickQueryTimeout bounds the short config and token reads on the
// assistant's request path.
const assistantQuickQueryTimeout = 5 * time.Second

// Service implements the plugin-backed AI assistant. Provider credentials
// and model selection come from the host-owned default provider connection;
// provider-specific protocol behavior remains inside the loaded plugin.
type Service struct {
	db            *sql.DB
	env           EnvProvider
	rbac          *rbac.Service
	log           *slog.Logger
	audit         *audit.Logger
	providers     ProviderResolver
	pluginRuntime any
}

// ErrEnvironmentAccessDenied reports that the caller may not use the requested environment as assistant context.
var ErrEnvironmentAccessDenied = errors.New("environment access denied")

// ErrProviderRateLimited marks an upstream quota rejection so the handler can
// tell the user to wait instead of reporting a generic outage.
var ErrProviderRateLimited = errors.New("ai provider rate limited")

// isProviderRateLimit classifies a provider plugin failure by message, because
// an error crossing the .so boundary carries no type this package can assert on.
func isProviderRateLimit(err error) bool {
	if err == nil {
		return false
	}
	lowered := strings.ToLower(err.Error())
	return strings.Contains(lowered, "rate limit") ||
		strings.Contains(lowered, "quota") ||
		strings.Contains(lowered, "too many requests")
}

const maxAssistantContentBytes = 256 << 10

// EnvProvider exposes the live environment data used to build context.
// Implemented by bootstrap.NewAIEnvProvider — keeps this package free
// of heavy imports.
type EnvProvider interface {
	Summary(ctx context.Context, envID string) (*EnvSummary, error)
}

// ContainerDetailer is an optional capability an EnvProvider can
// implement to surface a deeper view of a single container. Providers
// that don't implement it simply won't include per-container details
// in the assistant's context.
type ContainerDetailer interface {
	SummaryContainer(ctx context.Context, envID, containerID string) (*EnvContainer, string, error)
}

// EnvSummary is the snapshot fed into the assistant for grounding.
type EnvSummary struct {
	EnvironmentID   string         `json:"environmentId"`
	EnvironmentName string         `json:"environmentName"`
	Orchestrator    string         `json:"orchestrator"`
	Runtime         string         `json:"runtime"`
	Containers      []EnvContainer `json:"containers"`
	Images          int            `json:"images"`
	Volumes         int            `json:"volumes"`
	Networks        int            `json:"networks"`
	Stacks          []string       `json:"stacks"`
	OpenAlerts      int            `json:"openAlerts"`
	CPUPercent      int            `json:"cpuPercent"`
	MemoryPercent   int            `json:"memoryPercent"`
	DiskPercent     int            `json:"diskPercent"`
	RecentEvents    []string       `json:"recentEvents"`
}

// EnvContainer describes one container in the EnvSummary.
type EnvContainer struct {
	Name   string `json:"name"`
	Image  string `json:"image"`
	State  string `json:"state"`
	Status string `json:"status"`
	CPUPct int    `json:"cpuPct"`
	MemPct int    `json:"memPct"`
}

// NewService creates a new AI assistant service. A nil providers bridge is
// tolerated: chat then fails with ErrProviderPluginNotLoaded rather than
// panicking (see providers_bridge.go).
func NewService(db *sql.DB, env EnvProvider, providers ProviderResolver, pluginRuntime any) *Service {
	if env == nil {
		env = noopEnv{}
	}
	if providers == nil {
		providers = noProviderResolver{}
	}
	return &Service{
		db:            db,
		env:           env,
		log:           slog.Default(),
		providers:     providers,
		pluginRuntime: pluginRuntime,
	}
}

// SetRBAC installs the RBAC service so callProvider can render the
// per-user tool catalog into the system prompt. Optional — when unset
// the catalog is omitted (the model still gets the live env snapshot).
func (s *Service) SetRBAC(r *rbac.Service) {
	s.rbac = r
}

// SetAudit installs the central audit logger so tool executions driven by
// the chat loop (not just the standalone /tools/call endpoint) land in the
// audit log. Optional — when unset those executions are not audited by the
// tool layer, though the action-plane rows are still written by the handler.
func (s *Service) SetAudit(l *audit.Logger) {
	s.audit = l
}

// SetLogger installs the structured logger used for diagnostics such
// as the LLM-provider-failure fallback path. Optional — when unset the
// service uses slog.Default().
func (s *Service) SetLogger(l *slog.Logger) {
	if l == nil {
		l = slog.Default()
	}
	s.log = l
}

// ChatRequest is the request body for POST /ai-assistant/chat.
type ChatRequest struct {
	ConversationID string `json:"conversationId,omitempty"`
	EnvironmentID  string `json:"environmentId,omitempty"`
	ContainerID    string `json:"containerId,omitempty"`
	Message        string `json:"message"`
	// ConnectionID pins the request to one ai_provider_connections
	// row. Empty keeps the previous behaviour — the estate default —
	// so every existing caller (and any client that predates the chat
	// composer's provider picker) is unaffected.
	//
	// It is a *connection* id, not a provider type, because one plugin
	// can back several connections: a prod and a dev OpenAI key are two
	// rows of provider_type "openai", and a type would not tell them
	// apart.
	ConnectionID string `json:"connectionId,omitempty"`
	// Cursor lets the frontend pin the conversation's focus before
	// the model runs. Optional; empty = keep the persisted cursor.
	Cursor *Focal `json:"cursor,omitempty"`
	UserID string `json:"-"` // populated by the handler, never serialized
}

// ChatResponse is the response body returned by the chat endpoint.
type ChatResponse struct {
	ConversationID string   `json:"conversationId"`
	UserMessage    Message  `json:"userMessage"`
	Assistant      Message  `json:"assistant"`
	Suggestions    []string `json:"suggestions,omitempty"`
	// Cursor is the (possibly updated) cursor after the turn. The
	// frontend renders the focus + trail from this; the model sees
	// a render of it in the system prompt on the next turn.
	Cursor *Cursor `json:"cursor,omitempty"`
}

// Message is a single chat message.
type Message struct {
	ID        string `json:"id"`
	Role      string `json:"role"`
	Content   string `json:"content"`
	CreatedAt string `json:"createdAt"`
}

// Chat handles one user turn. It loads (or creates) the conversation,
// persists the user message, generates a plugin-backed response, and
// persists the assistant message.
func (s *Service) Chat(ctx context.Context, req ChatRequest) (*ChatResponse, error) {
	req.Message = strings.TrimSpace(req.Message)
	if req.Message == "" {
		return nil, errors.New("message is required")
	}
	if len(req.Message) > 16<<10 {
		return nil, errors.New("message exceeds 16 KiB")
	}

	convID := req.ConversationID
	createdConversation := convID == ""
	if createdConversation {
		convID = xid.New().String()
	} else {
		var storedEnvironmentID string
		if err := s.db.QueryRowContext(ctx,
			`SELECT COALESCE(environment_id, '') FROM ai_assistant_conversations
			 WHERE id = ? AND user_id = ?`,
			convID, currentUserID(ctx),
		).Scan(&storedEnvironmentID); errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("conversation not found")
		} else if err != nil {
			return nil, fmt.Errorf("looking up conversation: %w", err)
		}
		if req.EnvironmentID == "" {
			req.EnvironmentID = storedEnvironmentID
		} else if storedEnvironmentID != "" && req.EnvironmentID != storedEnvironmentID {
			return nil, ErrEnvironmentAccessDenied
		}
	}
	if req.ContainerID != "" && req.EnvironmentID == "" {
		return nil, ErrEnvironmentAccessDenied
	}
	if err := s.authorizeEnvironmentAccess(ctx, req.UserID, req.EnvironmentID); err != nil {
		return nil, err
	}

	if createdConversation {
		title := req.Message
		if len(title) > 80 {
			title = title[:80]
		}
		if _, err := s.db.ExecContext(ctx,
			`INSERT INTO ai_assistant_conversations (id, user_id, title, environment_id, created_at, updated_at)
			 VALUES (?, ?, ?, ?, ?, ?)`,
			convID, currentUserID(ctx), title, nullString(req.EnvironmentID),
			time.Now().UTC().Format(time.RFC3339), time.Now().UTC().Format(time.RFC3339),
		); err != nil {
			return nil, fmt.Errorf("creating conversation: %w", err)
		}
	} else {
		result, err := s.db.ExecContext(ctx,
			`UPDATE ai_assistant_conversations
			 SET environment_id = COALESCE(environment_id, ?), updated_at = ?
			 WHERE id = ? AND user_id = ?
			   AND (COALESCE(environment_id, '') = '' OR environment_id = ?)`,
			nullString(req.EnvironmentID), time.Now().UTC().Format(time.RFC3339),
			convID, currentUserID(ctx), req.EnvironmentID,
		)
		if err != nil {
			return nil, fmt.Errorf("updating conversation: %w", err)
		}
		if coredb.RowsAffected(result) == 0 {
			return nil, ErrEnvironmentAccessDenied
		}
	}

	userMsg := Message{
		ID:        xid.New().String(),
		Role:      "user",
		Content:   req.Message,
		CreatedAt: time.Now().UTC().Format(time.RFC3339),
	}
	if err := s.persistMessage(ctx, convID, userMsg); err != nil {
		return nil, err
	}

	// Cursor persistence is not part of the minimal schema-backed chat
	// surface. Keep request focus scoped to this turn.
	cur := emptyCursor()
	cur.EnvID = req.EnvironmentID
	if req.Cursor != nil {
		cur.SetFocus(req.Cursor)
	}

	reply, suggestions, err := s.generate(ctx, convID, req, cur)
	if err != nil {
		s.cleanupFailedTurn(ctx, convID, userMsg.ID, createdConversation)
		return nil, err
	}

	asstMsg := Message{
		ID:        xid.New().String(),
		Role:      "assistant",
		Content:   reply,
		CreatedAt: time.Now().UTC().Format(time.RFC3339),
	}
	if err := s.persistMessage(ctx, convID, asstMsg); err != nil {
		return nil, err
	}

	return &ChatResponse{
		ConversationID: convID,
		UserMessage:    userMsg,
		Assistant:      asstMsg,
		Suggestions:    suggestions,
		Cursor:         cur,
	}, nil
}

func (s *Service) authorizeEnvironmentAccess(ctx context.Context, userID, environmentID string) error {
	if environmentID == "" {
		return nil
	}
	if s.rbac == nil {
		return ErrEnvironmentAccessDenied
	}
	for _, permission := range []rbac.Permission{
		rbac.PermEnvironmentsView,
		rbac.PermContainersView,
		rbac.PermImagesView,
		rbac.PermVolumesView,
		rbac.PermNetworksView,
		rbac.PermStacksView,
	} {
		allowed, err := s.rbac.HasPermission(ctx, userID, environmentID, permission)
		if err != nil {
			return fmt.Errorf("checking environment permission: %w", err)
		}
		if !allowed {
			return ErrEnvironmentAccessDenied
		}
	}
	return nil
}

func (s *Service) cleanupFailedTurn(ctx context.Context, conversationID, messageID string, createdConversation bool) {
	cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), assistantQuickQueryTimeout)
	defer cancel()
	query := `DELETE FROM ai_assistant_messages WHERE id = ? AND conversation_id = ?`
	args := []any{messageID, conversationID}
	if createdConversation {
		query = `DELETE FROM ai_assistant_conversations WHERE id = ? AND user_id = ?`
		args = []any{conversationID, currentUserID(ctx)}
	}
	if _, err := s.db.ExecContext(cleanupCtx, query, args...); err != nil {
		s.log.Warn("ai assistant: failed to clean up unsuccessful turn", "conversationID", conversationID, "error", err)
	}
}

// ListConversations returns the user's recent conversations (newest first).
func (s *Service) ListConversations(ctx context.Context, limit int) ([]Conversation, error) {
	if limit <= 0 || limit > 100 {
		limit = 25
	}
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, title, COALESCE(environment_id, ''), created_at, updated_at
		 FROM ai_assistant_conversations
		 WHERE user_id = ?
		 ORDER BY updated_at DESC LIMIT ?`,
		currentUserID(ctx), limit,
	)
	if err != nil {
		return nil, err
	}
	defer func() {
		if err := rows.Close(); err != nil {
			slog.Warn("closing sql rows failed", "error", err)
		}
	}()

	var out []Conversation
	for rows.Next() {
		var c Conversation
		if err := rows.Scan(&c.ID, &c.Title, &c.EnvironmentID, &c.CreatedAt, &c.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	filtered := out[:0]
	for _, conversation := range out {
		if err := s.authorizeEnvironmentAccess(ctx, currentUserID(ctx), conversation.EnvironmentID); err != nil {
			if errors.Is(err, ErrEnvironmentAccessDenied) {
				continue
			}
			return nil, err
		}
		filtered = append(filtered, conversation)
	}
	return filtered, nil
}

// Conversation is the list-view of a chat session.
type Conversation struct {
	ID            string `json:"id"`
	Title         string `json:"title"`
	EnvironmentID string `json:"environmentId,omitempty"`
	CreatedAt     string `json:"createdAt"`
	UpdatedAt     string `json:"updatedAt"`
}

// ListMessages returns the messages of one conversation.
func (s *Service) ListMessages(ctx context.Context, conversationID string) ([]Message, error) {
	var environmentID string
	if err := s.db.QueryRowContext(ctx,
		`SELECT COALESCE(environment_id, '') FROM ai_assistant_conversations
		 WHERE id = ? AND user_id = ?`,
		conversationID, currentUserID(ctx),
	).Scan(&environmentID); errors.Is(err, sql.ErrNoRows) {
		return []Message{}, nil
	} else if err != nil {
		return nil, err
	}
	if err := s.authorizeEnvironmentAccess(ctx, currentUserID(ctx), environmentID); err != nil {
		return nil, err
	}

	rows, err := s.db.QueryContext(ctx,
		`SELECT id, role, content, created_at FROM (
		   SELECT m.rowid AS message_order, m.id, m.role, m.content, m.created_at
		   FROM ai_assistant_messages m
		   JOIN ai_assistant_conversations c ON c.id = m.conversation_id
		   WHERE m.conversation_id = ? AND c.user_id = ?
		   ORDER BY m.created_at DESC, m.rowid DESC LIMIT 500
		 ) ORDER BY created_at ASC, message_order ASC`,
		conversationID, currentUserID(ctx),
	)
	if err != nil {
		return nil, err
	}
	defer func() {
		if err := rows.Close(); err != nil {
			slog.Warn("closing sql rows failed", "error", err)
		}
	}()

	var out []Message
	for rows.Next() {
		var m Message
		if err := rows.Scan(&m.ID, &m.Role, &m.Content, &m.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

// generate resolves the default provider connection and dispatches through
// its registered plugin adapter. Request focus is included in the system prompt.
func (s *Service) generate(ctx context.Context, conversationID string, req ChatRequest, cur *Cursor) (string, []string, error) {
	envID := req.EnvironmentID
	if cur != nil && cur.EnvID != "" {
		envID = cur.EnvID
	}

	var envSummary *EnvSummary
	if envID != "" {
		var err error
		envSummary, err = s.env.Summary(ctx, envID)
		if err != nil {
			return "", nil, fmt.Errorf("loading environment summary: %w", err)
		}
	}

	// Optional container deep-dive: if the user opened the dialog from a
	// container page, include an inspect snapshot so the assistant can
	// answer questions about that specific container.
	var containerCtx *EnvContainer
	var containerImage string
	if req.ContainerID != "" && envID != "" && s.env != nil {
		if dp, ok := s.env.(ContainerDetailer); ok {
			c, img, derr := dp.SummaryContainer(ctx, envID, req.ContainerID)
			if derr != nil {
				return "", nil, fmt.Errorf("loading container summary: %w", derr)
			}
			containerCtx = c
			containerImage = img
		}
	}

	if s.providers == nil {
		return "", nil, ErrProviderPluginNotLoaded
	}
	reply, _, err := s.callDefaultPluginProvider(
		ctx, conversationID, req.UserID, envSummary, containerCtx, containerImage, cur, req.ConnectionID,
	)
	if err != nil {
		return "", nil, err
	}
	return reply, nil, nil
}

type aiPluginRuntime interface {
	AIAdapter(providerType string) (plugin.AIAdapterClient, plugin.AIAdapterSpec, bool)
}

func (s *Service) callDefaultPluginProvider(
	ctx context.Context,
	conversationID string,
	userID string,
	env *EnvSummary,
	container *EnvContainer,
	containerImage string,
	cur *Cursor,
	connectionID string,
) (string, string, error) {
	connection, config, err := s.providers.Resolve(ctx, connectionID)
	if err != nil {
		return "", "", err
	}
	runtime, ok := s.pluginRuntime.(aiPluginRuntime)
	if !ok || runtime == nil {
		return "", connection.ProviderType, ErrProviderPluginNotLoaded
	}
	adapter, spec, ok := runtime.AIAdapter(connection.ProviderType)
	if !ok || adapter == nil {
		return "", connection.ProviderType, ErrProviderPluginNotLoaded
	}

	model := connection.DefaultModel
	if model == "" {
		model = spec.DefaultModel
	}
	envID := ""
	if env != nil {
		envID = env.EnvironmentID
	}
	history, err := s.ListMessages(ctx, conversationID)
	if err != nil {
		return "", connection.ProviderType, fmt.Errorf("loading conversation history: %w", err)
	}
	if len(history) > 20 {
		history = history[len(history)-20:]
	}
	messages := make([]plugin.LLMMessage, 0, len(history))
	for _, message := range history {
		if message.Role != "user" && message.Role != "assistant" && message.Role != "system" {
			continue
		}
		messages = append(messages, plugin.LLMMessage{Role: message.Role, Content: message.Content})
	}

	// Inject any action outcomes from the previous turn so the model
	// sees what happened to the actions it queued (executed, denied,
	// failed, expired). Without this the next user turn has no
	// context and the model just re-emits the same tool call.
	var lastUserTS string
	for i := len(history) - 1; i >= 0; i-- {
		if history[i].Role == "user" {
			lastUserTS = history[i].CreatedAt
			break
		}
	}
	if lastUserTS == "" {
		lastUserTS = time.Now().UTC().Add(-assistantHistoryWindow).Format(time.RFC3339)
	}
	if outcomes, oerr := LoadRecentActionOutcomes(ctx, s.db, conversationID, lastUserTS); oerr != nil {
		s.log.Warn("ai assistant: load action outcomes", "error", oerr)
	} else if len(outcomes) > 0 {
		outcomesHeader := plugin.LLMMessage{
			Role:    "user",
			Content: "Outcomes of actions from the previous turn:",
		}
		messages = append(messages, outcomesHeader)
		messages = append(messages, outcomes...)
	}

	catalog := s.toolCatalog(ctx, userID, envID)
	pluginTools := BuildPluginTools(ctx, s.actionAuthorizer(), userID, envID)
	systemPrompt := buildSystemPrompt(env, "", catalog, cur)
	if env != nil {
		systemPrompt += "\n\n# Environment snapshot\n" + envSnapshotText(env)
	}
	if container != nil {
		systemPrompt += "\n\n# Container snapshot\n" + containerSnapshotText(container, containerImage)
	}
	if cur != nil {
		if block := cur.render(); block != "" {
			systemPrompt += "\n\n# Conversation cursor\n" + block
		}
	}

	callCtx, cancel := context.WithTimeout(ctx, 90*time.Second)
	defer cancel()

	scope := ActionScope{
		UserID:         userID,
		EnvID:          envID,
		ConversationID: conversationID,
	}
	pluginSupportsTools := spec.Capabilities.SupportsTools

	var finalReply string
	var lastResult *plugin.LLMResponse
	var pendingReply string
	for iter := 0; iter < 6; iter++ {
		result, err := adapter.Complete(callCtx, config, plugin.LLMRequest{
			Model:       model,
			System:      systemPrompt,
			Messages:    messages,
			Tools:       pluginTools,
			MaxTokens:   2048,
			Temperature: 0.2,
		})
		if err != nil {
			if isProviderRateLimit(err) {
				return "", connection.ProviderType, fmt.Errorf("completing with provider plugin: %w: %w", ErrProviderRateLimited, err)
			}
			return "", connection.ProviderType, fmt.Errorf("completing with provider plugin: %w", err)
		}
		if result == nil {
			return "", connection.ProviderType, fmt.Errorf("provider plugin returned an empty response")
		}
		lastResult = result

		if len(result.ToolCalls) == 0 || !pluginSupportsTools {
			break
		}

		assistantTurn := plugin.LLMMessage{Role: "assistant", Content: result.Content}
		anyPending := false
		var pendingActions []string
		for _, call := range result.ToolCalls {
			modelMsg, pendingAction, dispatchErr := DispatchPluginToolCall(callCtx, s, scope, call)
			if dispatchErr != nil {
				if strings.TrimSpace(assistantTurn.Content) != "" {
					messages = append(messages, assistantTurn)
				}
				// The raw error can name tables, paths or upstream responses; the
				// model only needs to know the call failed, so the detail stays in the log.
				s.log.Error("ai assistant: tool dispatch failed", "tool", call.Name, "user", scope.UserID, "error", dispatchErr)
				messages = append(messages, plugin.LLMMessage{
					Role:    "user",
					Content: fmt.Sprintf("tool %s failed", call.Name),
				})
				continue
			}
			if pendingAction != nil {
				anyPending = true
				pendingActions = append(pendingActions, pendingAction.ID)
			}
			if strings.TrimSpace(assistantTurn.Content) != "" {
				messages = append(messages, assistantTurn)
			}
			messages = append(messages, plugin.LLMMessage{
				Role:    "user",
				Content: fmt.Sprintf("tool %s result: %s", call.Name, modelMsg),
			})
		}
		if anyPending {
			pendingReply = "I prepared an action that requires your approval before it runs. Review the pending action in the assistant action panel, then approve and execute it if it looks correct."
			if len(pendingActions) > 0 {
				pendingReply += " Action ID: " + strings.Join(pendingActions, ", ") + "."
			}
			break
		}
	}

	if lastResult == nil {
		return "", connection.ProviderType, fmt.Errorf("provider plugin returned an empty response")
	}

	if pendingReply != "" {
		return pendingReply, connection.ProviderType, nil
	}

	finalReply = strings.TrimSpace(lastResult.Content)
	if toolName, toolArgs, ok := parseAssistantToolCall(finalReply); ok {
		toolReply, err := s.resolveAssistantToolCall(ctx, userID, envID, conversationID, toolName, toolArgs, catalog, connection.ID)
		if err != nil {
			return "", connection.ProviderType, err
		}
		if strings.TrimSpace(toolReply) != "" {
			return toolReply, connection.ProviderType, nil
		}
	}
	if finalReply == "" {
		return "", connection.ProviderType, fmt.Errorf("provider plugin returned an empty response")
	}
	if len(finalReply) > maxAssistantContentBytes {
		return "", connection.ProviderType, fmt.Errorf("provider plugin response exceeds %d bytes", maxAssistantContentBytes)
	}
	return finalReply, connection.ProviderType, nil
}

type assistantToolCall struct {
	Tool      string          `json:"tool"`
	Name      string          `json:"name"`
	Arguments json.RawMessage `json:"arguments"`
	ToolCall  *struct {
		Name      string          `json:"name"`
		Arguments json.RawMessage `json:"arguments"`
	} `json:"tool_call"`
}

func parseAssistantToolCall(content string) (string, json.RawMessage, bool) {
	trimmed := strings.TrimSpace(content)
	if !strings.HasPrefix(trimmed, "{") || !strings.HasSuffix(trimmed, "}") {
		return "", nil, false
	}
	var call assistantToolCall
	if err := json.Unmarshal([]byte(trimmed), &call); err != nil {
		return "", nil, false
	}
	name := strings.TrimSpace(call.Tool)
	args := call.Arguments
	if name == "" {
		name = strings.TrimSpace(call.Name)
	}
	if call.ToolCall != nil {
		if name == "" {
			name = strings.TrimSpace(call.ToolCall.Name)
		}
		if len(args) == 0 {
			args = call.ToolCall.Arguments
		}
	}
	if name == "" {
		return "", nil, false
	}
	if len(args) == 0 {
		args = json.RawMessage(`{}`)
	}
	return name, args, true
}

func (s *Service) resolveAssistantToolCall(ctx context.Context, userID, envID, conversationID, toolName string, args json.RawMessage, catalog []ai_tools.Descriptor, connectionID string) (string, error) {
	tool, ok := ai_tools.Get(toolName)
	if !ok {
		return "", nil
	}
	if !tool.ReadOnly || tool.RequiresConsent {
		action, err := s.PreviewAction(ctx, ActionPreviewRequest{
			ConversationID: conversationID,
			Tool:           toolName,
			Arguments:      args,
			Scope: ActionScope{
				UserID:         userID,
				EnvID:          envID,
				ConversationID: conversationID,
			},
		})
		if err != nil {
			return "", fmt.Errorf("previewing AI action: %w", err)
		}
		return "I prepared an action that requires your approval before it runs. Review action " + action.ID + " in the assistant action panel, then approve and execute it if it looks correct.", nil
	}

	result, callErr := ai_tools.Call(ctx, rbacAuthorizer{svc: s.rbac}, nil, ai_tools.GetCache(), ai_tools.CallRequest{
		Name:      toolName,
		Arguments: args,
		Scope: ai_tools.Scope{
			UserID:         userID,
			EnvID:          envID,
			ConversationID: conversationID,
		},
	})
	if callErr != nil {
		return "", fmt.Errorf("calling AI tool %s: %s", toolName, callErr.Message)
	}
	data, err := json.Marshal(result.Data)
	if err != nil {
		return "", fmt.Errorf("encoding AI tool result: %w", err)
	}

	connection, config, err := s.providers.Resolve(ctx, connectionID)
	if err != nil {
		return "", err
	}
	runtime, ok := s.pluginRuntime.(aiPluginRuntime)
	if !ok || runtime == nil {
		return "", ErrProviderPluginNotLoaded
	}
	adapter, spec, ok := runtime.AIAdapter(connection.ProviderType)
	if !ok || adapter == nil {
		return "", ErrProviderPluginNotLoaded
	}
	model := connection.DefaultModel
	if model == "" {
		model = spec.DefaultModel
	}
	prompt := buildToolResultPrompt(catalog)
	callCtx, cancel := context.WithTimeout(ctx, 90*time.Second)
	defer cancel()
	reply, err := adapter.Complete(callCtx, config, plugin.LLMRequest{
		Model:  model,
		System: prompt,
		Messages: []plugin.LLMMessage{
			{Role: "user", Content: "The user asked a McHarbor question. A read-only tool was executed to ground the answer."},
			{Role: "user", Content: "Tool: " + toolName + "\nArguments: " + string(args) + "\nResult JSON:\n" + string(data)},
		},
		MaxTokens:   2048,
		Temperature: 0.2,
	})
	if err != nil {
		return "", fmt.Errorf("summarizing AI tool result: %w", err)
	}
	if reply == nil || strings.TrimSpace(reply.Content) == "" {
		return string(data), nil
	}
	return reply.Content, nil
}

func buildToolResultPrompt(catalog []ai_tools.Descriptor) string {
	var sb strings.Builder
	sb.WriteString("You are the McHarbor AI assistant. Answer from the supplied tool result only. ")
	sb.WriteString("Do not reveal secrets. If the result is insufficient, say what tool or permission is needed next.")
	if len(catalog) > 0 {
		sb.WriteString("\n\nAvailable follow-up tools: ")
		for i, d := range catalog {
			if i > 0 {
				sb.WriteString(", ")
			}
			sb.WriteString(d.Name)
		}
	}
	return sb.String()
}

// callProvider dispatches a user message to the configured LLM provider
// using the per-provider wire format defined in providers.go.
// BeginStreamTurn is the streaming counterpart of Chat's preflight:
// it creates the conversation row, persists the user message, loads
// the cursor, and gathers everything the stream callback needs to
// assemble the LLM request. The conversation row is committed before
// any tokens are emitted so a stream that aborts partway still
// produces a usable thread.
// FinishStreamTurn persists the assistant's reply and the (possibly
// updated) cursor. The returned StreamFinal is what the SSE `done`
// frame echoes back to the frontend.
// streamProvider is the streaming counterpart to callProvider. It
// builds the same LLMRequest, then invokes the LLM client's Stream
// method, forwarding each chunk to onChunk. Returns the provider
// (so the caller can echo provider metadata in the SSE done frame)
// and the assembled final reply.
// buildSystemPrompt constructs the system message that frames the
// assistant's role and injects the live environment context plus the
// per-user tool catalog. When a cursor is supplied, a short block
// identifying the active focus is appended so the model knows what
// entity the conversation is anchored on.
func buildSystemPrompt(env *EnvSummary, userMessage string, catalog []ai_tools.Descriptor, cur *Cursor) string {
	var sb strings.Builder
	sb.WriteString("You are the McHarbor AI assistant, an expert DevOps helper for a self-hosted Docker management platform. ")
	sb.WriteString("Answer concisely, technically, and only with information grounded in the environment snapshot provided by the user. ")
	if env != nil {
		sb.WriteString("Current orchestrator: " + env.Orchestrator + ". Runtime: " + env.Runtime + ". ")
	}
	sb.WriteString("If the user asks for an action, return the exact `docker` command. ")
	sb.WriteString("If you don't know, say so — do not invent API endpoints or environment names.")
	if cur != nil {
		if cur.Focus != nil {
			sb.WriteString("\n\n# Active focus\n")
			sb.WriteString("The conversation is anchored on " + cur.Focus.Kind + " id=" + cur.Focus.ID)
			if cur.Focus.Name != "" {
				sb.WriteString(" (" + cur.Focus.Name + ")")
			}
			sb.WriteString(". Use entity.relate to walk parents/children, or pin a new focal via the request.")
		}
		if len(cur.Trail) > 0 {
			sb.WriteString(" Recent entities: ")
			for i, t := range cur.Trail {
				if i > 0 {
					sb.WriteString(", ")
				}
				sb.WriteString(t.Kind + ":" + t.ID)
			}
			sb.WriteString(".")
		}
	}
	if len(catalog) > 0 {
		sb.WriteString("\n\n# Available tools\n")
		sb.WriteString("The following tools are available via the host's structured tool-calling channel — invoke them by emitting a tool call, never by writing JSON in your reply. Read-only tools execute automatically and the result is fed back to you in the next turn. Mutating or consent-required tools create a pending action and the user must approve before they run. Do not invent tool names.\n")
		for _, d := range catalog {
			sb.WriteString("- " + d.Name + ": " + d.Description)
			if !d.ReadOnly || d.RequiresConsent {
				sb.WriteString(" (approval required)")
			}
			sb.WriteString("\n")
		}
	}
	return sb.String()
}

// toolCatalog returns the per-user tool catalog. Phase 1 ships
// read-only tools; the catalog is filtered by the user's effective
// permissions and the request's environment scope.
func (s *Service) toolCatalog(ctx context.Context, userID, envID string) []ai_tools.Descriptor {
	if s.rbac == nil {
		return nil
	}
	authz := rbacAuthorizer{svc: s.rbac}
	return ai_tools.CatalogFor(ctx, authz, userID, envID)
}

// rbacAuthorizer adapts *rbac.Service to the ai_tools.Authorizer
// interface. Defined here so the service file keeps the adapter local.
type rbacAuthorizer struct {
	svc *rbac.Service
}

func (a rbacAuthorizer) HasPermission(ctx context.Context, userID, envID string, perm rbac.Permission) (bool, error) {
	if a.svc == nil {
		return false, nil
	}
	return a.svc.HasPermission(ctx, userID, envID, perm)
}

// envSnapshotText formats the env summary as a compact context block.
func envSnapshotText(env *EnvSummary) string {
	var sb strings.Builder
	sb.WriteString("orchestrator=" + env.Orchestrator)
	sb.WriteString(" runtime=" + env.Runtime)
	sb.WriteString(" containers=" + strconv.Itoa(len(env.Containers)))
	sb.WriteString(" images=" + strconv.Itoa(env.Images))
	sb.WriteString(" volumes=" + strconv.Itoa(env.Volumes))
	sb.WriteString(" networks=" + strconv.Itoa(env.Networks))
	sb.WriteString(" stacks=" + strconv.Itoa(len(env.Stacks)))
	sb.WriteString(" openAlerts=" + strconv.Itoa(env.OpenAlerts))
	if env.CPUPercent > 0 || env.MemoryPercent > 0 || env.DiskPercent > 0 {
		fmt.Fprintf(&sb, " cpu=%d%% mem=%d%% disk=%d%%", env.CPUPercent, env.MemoryPercent, env.DiskPercent)
	}
	if len(env.Containers) > 0 {
		sb.WriteString("\ncontainers:\n")
		for i, c := range env.Containers {
			if i >= 8 {
				break
			}
			sb.WriteString("- " + c.Name + " (" + c.State + ") image=" + c.Image)
			if c.CPUPct > 0 || c.MemPct > 0 {
				fmt.Fprintf(&sb, " cpu=%d%% mem=%d%%", c.CPUPct, c.MemPct)
			}
			sb.WriteString("\n")
		}
	}
	return sb.String()
}

func (s *Service) persistMessage(ctx context.Context, conversationID string, m Message) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO ai_assistant_messages (id, conversation_id, role, content, created_at)
		 VALUES (?, ?, ?, ?, ?)`,
		m.ID, conversationID, m.Role, m.Content, m.CreatedAt,
	)
	if err != nil {
		return fmt.Errorf("persisting message: %w", err)
	}
	return nil
}

func nullString(s string) interface{} {
	if s == "" {
		return nil
	}
	return s
}

// currentUserID returns the authenticated user ID from the request
// context. The handler stamps the ID via withUserID before calling.
func currentUserID(ctx context.Context) string {
	return userIDFromContext(ctx)
}

// noopEnv is the default EnvProvider when no real provider is wired in.
type noopEnv struct{}

func (noopEnv) Summary(_ context.Context, _ string) (*EnvSummary, error) {
	return &EnvSummary{
		EnvironmentName: "(no environment)",
		Orchestrator:    "unknown",
		Runtime:         "unknown",
		Containers:      nil,
		Stacks:          nil,
		RecentEvents:    nil,
	}, nil
}

// MarshalEnv returns the env summary as JSON, useful for logging/diagnostics.
func MarshalEnv(env *EnvSummary) ([]byte, error) {
	return json.Marshal(env)
}

// containerSnapshotText formats the per-container deep-dive as a
// compact context block fed to the LLM.
func containerSnapshotText(c *EnvContainer, image string) string {
	var sb strings.Builder
	sb.WriteString("Focused container:\n")
	sb.WriteString(" name=" + c.Name)
	sb.WriteString(" state=" + c.State)
	sb.WriteString(" status=" + c.Status)
	if image != "" {
		sb.WriteString(" image=" + image)
	}
	return sb.String()
}
