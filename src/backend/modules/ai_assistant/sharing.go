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
	"strings"
	"time"

	"github.com/rs/xid"
)

// ShareRequest is the body of POST /ai-assistant/conversations/{id}/share.
// "user" or "role" picks the target; the matching entry in
// ai_assistant_shared is upserted. The caller (grantedBy) is set
// server-side from the authenticated user.
type ShareRequest struct {
	// Target is the username (or role name, when TargetIsRole=true)
	// to share with.
	Target string `json:"target"`
	// Permission is "view" or "comment". Empty = "view".
	Permission string `json:"permission"`
	// TargetIsRole treats Target as a role name rather than a user.
	TargetIsRole bool `json:"targetIsRole"`
}

// Share is one row of ai_assistant_shared.
type Share struct {
	ConversationID string `json:"conversationId"`
	SharedWith     string `json:"sharedWith"`
	Permission     string `json:"permission"`
	GrantedBy      string `json:"grantedBy"`
	CreatedAt      string `json:"createdAt"`
}

// ShareConversation grants a user (or role) view/comment access to
// a conversation. The conversation owner is the only one allowed
// to share; the service checks ownership and returns 403 otherwise.
func (s *Service) ShareConversation(ctx context.Context, conversationID string, req ShareRequest, grantedBy string) (*Share, error) {
	if strings.TrimSpace(req.Target) == "" {
		return nil, errors.New("target is required")
	}
	if req.Permission == "" {
		req.Permission = "view"
	}
	if req.Permission != "view" && req.Permission != "comment" {
		return nil, fmt.Errorf("invalid permission %q (must be view or comment)", req.Permission)
	}
	owner, err := s.conversationOwner(ctx, conversationID)
	if err != nil {
		return nil, err
	}
	if owner == "" {
		return nil, fmt.Errorf("conversation not found")
	}
	if owner != grantedBy {
		return nil, fmt.Errorf("only the owner can share a conversation")
	}
	now := time.Now().UTC().Format(time.RFC3339)
	_, err = s.db.ExecContext(ctx,
		`INSERT INTO ai_assistant_shared (conversation_id, shared_with, permission, granted_by, created_at)
		 VALUES (?, ?, ?, ?, ?)
		 ON CONFLICT(conversation_id, shared_with) DO UPDATE SET
		   permission = excluded.permission,
		   granted_by = excluded.granted_by`,
		conversationID, req.Target, req.Permission, grantedBy, now,
	)
	if err != nil {
		return nil, fmt.Errorf("share conversation: %w", err)
	}
	return &Share{
		ConversationID: conversationID,
		SharedWith:     req.Target,
		Permission:     req.Permission,
		GrantedBy:      grantedBy,
		CreatedAt:      now,
	}, nil
}

// UnshareConversation removes a share entry. Owner-only.
func (s *Service) UnshareConversation(ctx context.Context, conversationID, target, grantedBy string) error {
	owner, err := s.conversationOwner(ctx, conversationID)
	if err != nil {
		return err
	}
	if owner == "" {
		return fmt.Errorf("conversation not found")
	}
	if owner != grantedBy {
		return fmt.Errorf("only the owner can unshare a conversation")
	}
	_, err = s.db.ExecContext(ctx,
		`DELETE FROM ai_assistant_shared WHERE conversation_id = ? AND shared_with = ?`,
		conversationID, target,
	)
	return err
}

// ListShares returns every share entry for one conversation.
func (s *Service) ListShares(ctx context.Context, conversationID string) ([]Share, error) {
	if err := s.requireConversationOwner(ctx, conversationID, currentUserID(ctx)); err != nil {
		return nil, err
	}
	rows, err := s.db.QueryContext(ctx,
		`SELECT conversation_id, shared_with, permission, granted_by, created_at
		 FROM ai_assistant_shared WHERE conversation_id = ?`,
		conversationID,
	)
	if err != nil {
		return nil, err
	}
	defer func() {
		if err := rows.Close(); err != nil {
			slog.Warn("closing sql rows failed", "error", err)
		}
	}()
	out := make([]Share, 0)
	for rows.Next() {
		var sh Share
		if err := rows.Scan(&sh.ConversationID, &sh.SharedWith, &sh.Permission, &sh.GrantedBy, &sh.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, sh)
	}
	return out, rows.Err()
}

// conversationOwner returns the user_id of the conversation owner,
// or "" if no such conversation exists.
func (s *Service) conversationOwner(ctx context.Context, conversationID string) (string, error) {
	var owner string
	err := s.db.QueryRowContext(ctx,
		`SELECT user_id FROM ai_assistant_conversations WHERE id = ?`,
		conversationID,
	).Scan(&owner)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", nil
		}
		return "", err
	}
	return owner, nil
}

func (s *Service) requireConversationOwner(ctx context.Context, conversationID, userID string) error {
	owner, err := s.conversationOwner(ctx, conversationID)
	if err != nil {
		return err
	}
	if owner == "" || owner != userID {
		return errors.New("conversation not found")
	}
	return nil
}

// --- Pin / Unpin ---

// PinConversation sets pinned=1 on the conversation row. The owner
// can pin; shared viewers can pin their own copy via a future
// enhancement (not in scope for Phase 6).
func (s *Service) PinConversation(ctx context.Context, conversationID, userID string) error {
	owner, err := s.conversationOwner(ctx, conversationID)
	if err != nil {
		return err
	}
	if owner == "" {
		return fmt.Errorf("conversation not found")
	}
	if owner != userID {
		return fmt.Errorf("only the owner can pin a conversation")
	}
	_, err = s.db.ExecContext(ctx,
		`UPDATE ai_assistant_conversations SET pinned = 1, updated_at = ? WHERE id = ?`,
		time.Now().UTC().Format(time.RFC3339), conversationID,
	)
	return err
}

// UnpinConversation sets pinned=0. Same ownership check as Pin.
func (s *Service) UnpinConversation(ctx context.Context, conversationID, userID string) error {
	owner, err := s.conversationOwner(ctx, conversationID)
	if err != nil {
		return err
	}
	if owner == "" {
		return fmt.Errorf("conversation not found")
	}
	if owner != userID {
		return fmt.Errorf("only the owner can unpin a conversation")
	}
	_, err = s.db.ExecContext(ctx,
		`UPDATE ai_assistant_conversations SET pinned = 0, updated_at = ? WHERE id = ?`,
		time.Now().UTC().Format(time.RFC3339), conversationID,
	)
	return err
}

// --- Export ---

// ExportFormat picks the wire shape of an exported conversation.
type ExportFormat string

const (
	// ExportJSON exports a conversation as the structured JSON bundle.
	ExportJSON ExportFormat = "json"
	// ExportMarkdown exports a conversation as readable Markdown.
	ExportMarkdown ExportFormat = "markdown"
)

// ExportBundle is the structured export shape. JSON serialisation
// is identical to the body of GET /conversations/{id}/export?format=json.
// Markdown rendering is generated server-side from the same data.
type ExportBundle struct {
	ConversationID string       `json:"conversationId"`
	Title          string       `json:"title"`
	Owner          string       `json:"owner"`
	EnvironmentID  string       `json:"environmentId,omitempty"`
	Cursor         *Cursor      `json:"cursor,omitempty"`
	Messages       []Message    `json:"messages"`
	ExportedAt     string       `json:"exportedAt"`
	Format         ExportFormat `json:"format"`
	Markdown       string       `json:"markdown,omitempty"`
}

// ExportConversation assembles a conversation + its messages +
// cursor into one self-contained bundle. format picks the
// `Format` field; for "markdown" the bundle also includes a
// pre-rendered Markdown body. The frontend can hit the endpoint
// with ?format=markdown and forward the body to the user's browser
// as a downloadable .md file.
func (s *Service) ExportConversation(ctx context.Context, conversationID string, format ExportFormat) (*ExportBundle, error) {
	if err := s.requireConversationOwner(ctx, conversationID, currentUserID(ctx)); err != nil {
		return nil, err
	}
	row := s.db.QueryRowContext(ctx,
		`SELECT id, user_id, COALESCE(title, ''), COALESCE(environment_id, ''), COALESCE(cursor_json, '')
		 FROM ai_assistant_conversations WHERE id = ?`,
		conversationID,
	)
	var (
		bundle    ExportBundle
		envID     string
		cursorRaw string
	)
	if err := row.Scan(&bundle.ConversationID, &bundle.Owner, &bundle.Title, &envID, &cursorRaw); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("conversation not found")
		}
		return nil, err
	}
	bundle.EnvironmentID = envID
	bundle.ExportedAt = time.Now().UTC().Format(time.RFC3339)
	bundle.Format = format
	if cursorRaw != "" {
		var c Cursor
		if err := json.Unmarshal([]byte(cursorRaw), &c); err == nil {
			bundle.Cursor = &c
		}
	}
	msgs, err := s.ListMessages(ctx, conversationID)
	if err != nil {
		return nil, err
	}
	bundle.Messages = msgs

	if format == ExportMarkdown {
		bundle.Markdown = renderMarkdown(&bundle)
	}
	return &bundle, nil
}

// renderMarkdown produces a self-contained Markdown export. The
// header carries the conversation metadata; each message is a
// blockquote for user / a code block for assistant, so the file
// reads cleanly in any Markdown viewer.
func renderMarkdown(b *ExportBundle) string {
	var sb strings.Builder
	sb.WriteString("# " + b.Title + "\n\n")
	if b.Owner != "" {
		sb.WriteString("**Owner:** " + b.Owner + "\n")
	}
	if b.EnvironmentID != "" {
		sb.WriteString("**Environment:** " + b.EnvironmentID + "\n")
	}
	if b.Cursor != nil && b.Cursor.Focus != nil {
		sb.WriteString("**Focus:** " + b.Cursor.Focus.Kind + " " + b.Cursor.Focus.ID)
		if b.Cursor.Focus.Name != "" {
			sb.WriteString(" (" + b.Cursor.Focus.Name + ")")
		}
		sb.WriteString("\n")
	}
	if b.ExportedAt != "" {
		sb.WriteString("**Exported:** " + b.ExportedAt + "\n")
	}
	sb.WriteString("\n---\n\n")
	for _, m := range b.Messages {
		switch m.Role {
		case "user":
			sb.WriteString("## 👤 User\n\n")
			sb.WriteString(m.Content + "\n\n")
		case "assistant":
			sb.WriteString("## 🤖 Assistant\n\n")
			sb.WriteString(m.Content + "\n\n")
		case "system":
			sb.WriteString("> " + m.Content + "\n\n")
		default:
			sb.WriteString("## " + m.Role + "\n\n")
			sb.WriteString(m.Content + "\n\n")
		}
	}
	return sb.String()
}

// prevent unused imports when callers go through service-only paths.
var _ = xid.New
var _ = json.Marshal
