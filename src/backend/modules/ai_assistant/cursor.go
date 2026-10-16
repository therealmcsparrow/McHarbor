// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package ai_assistant

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	ai_tools "github.com/therealmcsparrow/mcharbor/modules/ai_assistant/tools"
)

// Cursor describes the active focus of a conversation. It is persisted
// as cursor_json on the conversation row, restored on the next turn,
// and rendered into the system prompt as a context block. The model
// can also pin or move the focus via the entity.relate tool, which
// returns the parents / children it can walk to.
type Cursor struct {
	// Focus is the single entity the conversation is currently
	// centered on. nil = "no specific entity, just chat".
	Focus *Focal `json:"focus,omitempty"`

	// Trail is the LRU list of focals the user has touched in this
	// conversation, newest first. Capped at trailMax so the JSON
	// stays small.
	Trail []Focal `json:"trail,omitempty"`

	// EnvID pins the assistant to one environment; overrides
	// request-level EnvironmentID when set.
	EnvID string `json:"envId,omitempty"`

	// UpdatedAt is the last time the cursor was written.
	UpdatedAt string `json:"updatedAt,omitempty"`
}

// Focal is a pointer into the McHarbor entity graph. Kinds are the
// same labels the entity.relate tool accepts.
type Focal struct {
	Kind  string `json:"kind"` // container, image, stack, env, workflow, scan, user
	ID    string `json:"id"`
	Name  string `json:"name,omitempty"` // best-effort display name
	Label string `json:"label,omitempty"`
}

// Allowed kinds live in ai_tools (see tools/kinds.go) so the graph
// walker and the cursor agree on the closed set.

// IsValidFocalKind reports whether k is one of the supported entity
// kinds. Empty kind is not valid. Wraps the ai_tools predicate so
// the parent package doesn't need to redeclare the set.
func IsValidFocalKind(k string) bool {
	return ai_tools.IsValidFocalKind(k)
}

// CursorUpdate is the body of PUT /ai-assistant/conversations/:id/cursor.
// The frontend uses it to pin / clear the focus before the next chat
// turn, or to read back the current state via GET.
type CursorUpdate struct {
	Focus *Focal `json:"focus,omitempty"`
	EnvID string `json:"envId,omitempty"`
	Clear bool   `json:"clear,omitempty"`
}

// ConversationCursor returns the persisted cursor for a conversation. Missing
// rows return an empty cursor (no error) so the frontend can render
// the panel from a fresh state without special-casing.
func (s *Service) ConversationCursor(ctx context.Context, conversationID string) (*Cursor, error) {
	if err := s.requireConversationOwner(ctx, conversationID, currentUserID(ctx)); err != nil {
		return nil, err
	}
	return loadCursor(ctx, s.db, conversationID)
}

// UpdateCursor applies the update to the persisted cursor. Clearing
// wipes the focus + trail; setting a focus moves the old one to the
// trail (via SetFocus). An EnvID update is applied only when non-empty.
func (s *Service) UpdateCursor(ctx context.Context, conversationID string, u CursorUpdate) (*Cursor, error) {
	if err := s.requireConversationOwner(ctx, conversationID, currentUserID(ctx)); err != nil {
		return nil, err
	}
	cur, err := loadCursor(ctx, s.db, conversationID)
	if err != nil {
		return nil, err
	}
	if u.Clear {
		cur.Focus = nil
		cur.Trail = nil
	} else if u.Focus != nil {
		cur.SetFocus(u.Focus)
	}
	if u.EnvID != "" {
		cur.EnvID = u.EnvID
	}
	if err := saveCursor(ctx, s.db, conversationID, cur); err != nil {
		return nil, err
	}
	return cur, nil
}

const trailMax = 6

// emptyCursor returns a fresh cursor with a stamped timestamp.
func emptyCursor() *Cursor {
	return &Cursor{UpdatedAt: time.Now().UTC().Format(time.RFC3339)}
}

// loadCursor reads cursor_json for a conversation, returning an empty
// cursor if the row has no cursor yet. A corrupt JSON blob is treated
// as "no cursor" so a bad write never wedges the chat.
func loadCursor(ctx context.Context, db *sql.DB, conversationID string) (*Cursor, error) {
	if db == nil || conversationID == "" {
		return emptyCursor(), nil
	}
	var raw sql.NullString
	if err := db.QueryRowContext(ctx,
		`SELECT cursor_json FROM ai_assistant_conversations WHERE id = ?`,
		conversationID,
	).Scan(&raw); err != nil {
		if err == sql.ErrNoRows {
			return emptyCursor(), nil
		}
		return emptyCursor(), nil
	}
	if !raw.Valid || strings.TrimSpace(raw.String) == "" {
		return emptyCursor(), nil
	}
	var c Cursor
	if err := json.Unmarshal([]byte(raw.String), &c); err != nil {
		return emptyCursor(), nil //nolint:nilerr // an unreadable stored cursor resets to empty instead of failing the conversation
	}
	if c.Focus != nil && !IsValidFocalKind(c.Focus.Kind) {
		c.Focus = nil
	}
	return &c, nil
}

// saveCursor writes the cursor back to the conversation row. The
// updated_at column is bumped in the same statement so the
// conversation re-orders to the top of ListConversations.
func saveCursor(ctx context.Context, db *sql.DB, conversationID string, c *Cursor) error {
	if db == nil || conversationID == "" || c == nil {
		return nil
	}
	if c.Focus != nil && !IsValidFocalKind(c.Focus.Kind) {
		c.Focus = nil
	}
	c.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
	b, err := json.Marshal(c)
	if err != nil {
		return fmt.Errorf("marshal cursor: %w", err)
	}
	_, err = db.ExecContext(ctx,
		`UPDATE ai_assistant_conversations
		 SET cursor_json = ?, updated_at = ?
		 WHERE id = ?`,
		string(b), c.UpdatedAt, conversationID,
	)
	if err != nil {
		return fmt.Errorf("save cursor: %w", err)
	}
	return nil
}

// SetFocus replaces the cursor's focus (if valid) and prepends the
// previous focus to the trail. Empty focus clears the cursor. The
// trail is capped at trailMax entries.
func (c *Cursor) SetFocus(next *Focal) {
	if c == nil {
		return
	}
	if next == nil {
		c.Focus = nil
		return
	}
	if !IsValidFocalKind(next.Kind) || next.ID == "" {
		return
	}
	if c.Focus != nil {
		if c.Focus.Kind == next.Kind && c.Focus.ID == next.ID {
			c.Focus = next
			return
		}
		c.Trail = append([]Focal{*c.Focus}, c.Trail...)
		if len(c.Trail) > trailMax {
			c.Trail = c.Trail[:trailMax]
		}
	}
	c.Focus = next
}

// renderCursor returns a human-readable block suitable for injection
// into the system prompt. Returns an empty string when the cursor has
// no focus and no trail; callers should treat that as "no cursor".
func (c *Cursor) render() string {
	if c == nil {
		return ""
	}
	var sb strings.Builder
	if c.Focus != nil {
		sb.WriteString("# Conversation focus\n")
		sb.WriteString("Current entity: " + c.Focus.Kind + " id=" + c.Focus.ID)
		if c.Focus.Name != "" {
			sb.WriteString(" name=" + c.Focus.Name)
		}
		if c.Focus.Label != "" {
			sb.WriteString(" (" + c.Focus.Label + ")")
		}
		sb.WriteString("\n")
		if c.EnvID != "" {
			sb.WriteString("Environment scope: " + c.EnvID + "\n")
		}
	}
	if len(c.Trail) > 0 {
		sb.WriteString("Recent entities: ")
		for i, t := range c.Trail {
			if i > 0 {
				sb.WriteString(", ")
			}
			sb.WriteString(t.Kind + ":" + t.ID)
			if t.Name != "" {
				sb.WriteString("(" + t.Name + ")")
			}
		}
		sb.WriteString("\n")
	}
	if c.Focus == nil && len(c.Trail) == 0 {
		return ""
	}
	sb.WriteString("Use entity.relate to walk the graph; use containers.list / images.list / stacks.list to expand.\n")
	return sb.String()
}
