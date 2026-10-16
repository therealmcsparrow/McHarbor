// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package inappnotifications

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"regexp"
	"strings"
	"time"

	"github.com/rs/xid"
)

// Notification represents a single in-app notification item.
type Notification struct {
	ID         string `json:"id"`
	Level      string `json:"level"`
	Title      string `json:"title"`
	Message    string `json:"message"`
	Action     string `json:"action,omitempty"`
	EntityType string `json:"entityType,omitempty"`
	EntityID   string `json:"entityId,omitempty"`
	CreatedAt  string `json:"createdAt"`
	Read       bool   `json:"read"`
	ReadAt     string `json:"readAt,omitempty"`
}

// Service handles in-app notification queries and mutations.
type Service struct {
	db *sql.DB
}

// NewService creates a new in-app notification service.
func NewService(db *sql.DB) *Service {
	return &Service{db: db}
}

// ListFilter narrows a user's notification list. Mode is contains, exact or regex;
// empty fields match everything.
type ListFilter struct {
	Query  string
	Mode   string
	Levels []string
	Status string
}

// maxRegexScan bounds how many rows a regex search reads, because a regex cannot be
// pushed into SQL and is matched in Go before paginating.
const maxRegexScan = 1000

// ErrInvalidPattern is returned for a regex search that does not compile.
var ErrInvalidPattern = errors.New("invalid search pattern")

// ListForUser returns one page of the user's notifications with read state,
// unread first, filtered on the server so a search covers every page.
func (s *Service) ListForUser(ctx context.Context, userID string, page, perPage int, f ListFilter) ([]Notification, int64, error) {
	where := ` FROM in_app_notifications n
		 LEFT JOIN in_app_notification_reads r
		   ON r.notification_id = n.id AND r.user_id = ?
		 WHERE r.deleted_at IS NULL`
	args := []any{userID}
	switch f.Status {
	case "unread":
		where += " AND r.read_at IS NULL"
	case "read":
		where += " AND r.read_at IS NOT NULL"
	}
	if len(f.Levels) > 0 {
		where += " AND n.level IN (?" + strings.Repeat(", ?", len(f.Levels)-1) + ")"
		for _, l := range f.Levels {
			args = append(args, l)
		}
	}
	q := strings.TrimSpace(f.Query)
	var re *regexp.Regexp
	switch {
	case q == "":
	case f.Mode == "regex":
		if len(q) > 256 {
			return nil, 0, ErrInvalidPattern
		}
		compiled, err := regexp.Compile("(?i)" + q)
		if err != nil {
			return nil, 0, ErrInvalidPattern
		}
		re = compiled
	case f.Mode == "exact":
		where += " AND (LOWER(n.title) = LOWER(?) OR LOWER(n.message) = LOWER(?))"
		args = append(args, q, q)
	default:
		like := "%" + strings.NewReplacer("!", "!!", "%", "!%", "_", "!_").Replace(strings.ToLower(q)) + "%"
		where += " AND (LOWER(n.title) LIKE ? ESCAPE '!' OR LOWER(n.message) LIKE ? ESCAPE '!')"
		args = append(args, like, like)
	}
	order := " ORDER BY CASE WHEN r.read_at IS NULL THEN 0 ELSE 1 END ASC, n.created_at DESC"
	cols := "SELECT n.id, n.level, n.title, n.message, n.action, n.entity_type, n.entity_id, n.created_at, r.read_at"

	if re != nil {
		all, err := s.scanAll(ctx, cols+where+order+" LIMIT ?", append(args, maxRegexScan)...)
		if err != nil {
			return nil, 0, err
		}
		matched := make([]Notification, 0, len(all))
		for _, n := range all {
			if re.MatchString(n.Title) || re.MatchString(n.Message) {
				matched = append(matched, n)
			}
		}
		start := (page - 1) * perPage
		if start > len(matched) {
			start = len(matched)
		}
		end := start + perPage
		if end > len(matched) {
			end = len(matched)
		}
		return matched[start:end], int64(len(matched)), nil
	}

	var total int64
	if err := s.db.QueryRowContext(ctx, "SELECT COUNT(*)"+where, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("counting in-app notifications: %w", err)
	}
	items, err := s.scanAll(ctx, cols+where+order+" LIMIT ? OFFSET ?", append(args, perPage, (page-1)*perPage)...)
	if err != nil {
		return nil, 0, err
	}
	return items, total, nil
}

func (s *Service) scanAll(ctx context.Context, query string, args ...any) ([]Notification, error) {
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("listing in-app notifications: %w", err)
	}
	defer closeRows(rows)
	items := []Notification{}
	for rows.Next() {
		item, err := scanNotification(rows)
		if err != nil {
			return nil, fmt.Errorf("scanning in-app notification row: %w", err)
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating in-app notification rows: %w", err)
	}
	return items, nil
}

// Counts is what the filter chips show: the user's notifications by read state and
// by level, unfiltered, so a chip says how many it would reveal.
type Counts struct {
	Total   int64            `json:"total"`
	Unread  int64            `json:"unread"`
	ByLevel map[string]int64 `json:"byLevel"`
}

// CountsForUser returns the user's notification counts for the filter chips.
func (s *Service) CountsForUser(ctx context.Context, userID string) (Counts, error) {
	out := Counts{ByLevel: map[string]int64{}}
	rows, err := s.db.QueryContext(ctx,
		`SELECT n.level, COUNT(*), SUM(CASE WHEN r.read_at IS NULL THEN 1 ELSE 0 END)
		 FROM in_app_notifications n
		 LEFT JOIN in_app_notification_reads r
		   ON r.notification_id = n.id AND r.user_id = ?
		 WHERE r.deleted_at IS NULL
		 GROUP BY n.level`, userID)
	if err != nil {
		return out, fmt.Errorf("counting in-app notifications: %w", err)
	}
	defer closeRows(rows)
	for rows.Next() {
		var level string
		var n, unread int64
		if err := rows.Scan(&level, &n, &unread); err != nil {
			return out, fmt.Errorf("scanning in-app notification counts: %w", err)
		}
		out.ByLevel[level] = n
		out.Total += n
		out.Unread += unread
	}
	return out, rows.Err()
}

// UnreadCount returns the number of unread notifications for the user.
func (s *Service) UnreadCount(ctx context.Context, userID string) (int64, error) {
	var count int64
	if err := s.db.QueryRowContext(
		ctx,
		`SELECT COUNT(*)
		 FROM in_app_notifications n
		 LEFT JOIN in_app_notification_reads r
		   ON r.notification_id = n.id AND r.user_id = ?
		 WHERE r.deleted_at IS NULL
		   AND r.read_at IS NULL`,
		userID,
	).Scan(&count); err != nil {
		return 0, fmt.Errorf("counting unread in-app notifications: %w", err)
	}

	return count, nil
}

// MarkRead marks a single notification as read for the user.
// Returns false when the notification does not exist.
func (s *Service) MarkRead(ctx context.Context, notificationID, userID string) (bool, error) {
	var exists string
	if err := s.db.QueryRowContext(ctx, "SELECT id FROM in_app_notifications WHERE id = ?", notificationID).Scan(&exists); err == sql.ErrNoRows {
		return false, nil
	} else if err != nil {
		return false, fmt.Errorf("checking in-app notification existence: %w", err)
	}

	now := time.Now().UTC().Format(time.RFC3339)
	if _, err := s.db.ExecContext(
		ctx,
		`INSERT INTO in_app_notification_reads
		 (id, notification_id, user_id, read_at, deleted_at, created_at, updated_at)
		 VALUES (?, ?, ?, ?, NULL, ?, ?)
		 ON CONFLICT(notification_id, user_id) DO UPDATE SET
		   read_at = excluded.read_at,
		   deleted_at = NULL,
		   updated_at = excluded.updated_at`,
		xid.New().String(),
		notificationID,
		userID,
		now,
		now,
		now,
	); err != nil {
		return false, fmt.Errorf("marking in-app notification as read: %w", err)
	}

	return true, nil
}

// DeleteForUser hides a single notification from the current user's inbox.
// Returns false when the notification does not exist.
func (s *Service) DeleteForUser(ctx context.Context, notificationID, userID string) (bool, error) {
	var exists string
	if err := s.db.QueryRowContext(ctx, "SELECT id FROM in_app_notifications WHERE id = ?", notificationID).Scan(&exists); err == sql.ErrNoRows {
		return false, nil
	} else if err != nil {
		return false, fmt.Errorf("checking in-app notification existence: %w", err)
	}

	now := time.Now().UTC().Format(time.RFC3339)
	if _, err := s.db.ExecContext(
		ctx,
		`INSERT INTO in_app_notification_reads
		 (id, notification_id, user_id, read_at, deleted_at, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?)
		 ON CONFLICT(notification_id, user_id) DO UPDATE SET
		   deleted_at = excluded.deleted_at,
		   updated_at = excluded.updated_at`,
		xid.New().String(),
		notificationID,
		userID,
		now,
		now,
		now,
		now,
	); err != nil {
		return false, fmt.Errorf("deleting in-app notification for user: %w", err)
	}

	return true, nil
}

// MarkAllRead marks every unread notification as read for the user.
func (s *Service) MarkAllRead(ctx context.Context, userID string) (int64, error) {
	const batchSize = 500

	var totalMarked int64
	for {
		ids, err := s.unreadIDs(ctx, userID, batchSize)
		if err != nil {
			return totalMarked, err
		}
		if len(ids) == 0 {
			return totalMarked, nil
		}

		tx, err := s.db.BeginTx(ctx, nil)
		if err != nil {
			return totalMarked, fmt.Errorf("beginning mark-all-read transaction: %w", err)
		}

		now := time.Now().UTC().Format(time.RFC3339)
		for _, notificationID := range ids {
			if _, err := tx.ExecContext(
				ctx,
				`INSERT OR IGNORE INTO in_app_notification_reads
				 (id, notification_id, user_id, read_at, created_at, updated_at)
				 VALUES (?, ?, ?, ?, ?, ?)`,
				xid.New().String(),
				notificationID,
				userID,
				now,
				now,
				now,
			); err != nil {
				if rbErr := tx.Rollback(); rbErr != nil && !errors.Is(rbErr, sql.ErrTxDone) {
					slog.Warn("in_app_notifications: rolling back mark-all-read", "error", rbErr)
				}
				return totalMarked, fmt.Errorf("marking all in-app notifications as read: %w", err)
			}
		}

		if err := tx.Commit(); err != nil {
			return totalMarked, fmt.Errorf("committing mark-all-read transaction: %w", err)
		}

		totalMarked += int64(len(ids))
		if len(ids) < batchSize {
			return totalMarked, nil
		}
	}
}

func (s *Service) unreadIDs(ctx context.Context, userID string, limit int) ([]string, error) {
	rows, err := s.db.QueryContext(
		ctx,
		`SELECT n.id
		 FROM in_app_notifications n
		 LEFT JOIN in_app_notification_reads r
		   ON r.notification_id = n.id AND r.user_id = ?
		 WHERE r.deleted_at IS NULL
		   AND r.read_at IS NULL
		 ORDER BY n.created_at DESC
		 LIMIT ?`,
		userID,
		limit,
	)
	if err != nil {
		return nil, fmt.Errorf("listing unread in-app notification ids: %w", err)
	}
	defer closeRows(rows)

	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("scanning unread in-app notification id: %w", err)
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating unread in-app notification ids: %w", err)
	}

	return ids, nil
}

func scanNotification(scanner interface {
	Scan(dest ...any) error
}) (Notification, error) {
	var item Notification
	var action sql.NullString
	var entityType sql.NullString
	var entityID sql.NullString
	var readAt sql.NullString

	if err := scanner.Scan(
		&item.ID,
		&item.Level,
		&item.Title,
		&item.Message,
		&action,
		&entityType,
		&entityID,
		&item.CreatedAt,
		&readAt,
	); err != nil {
		return Notification{}, err
	}

	item.Action = action.String
	item.EntityType = entityType.String
	item.EntityID = entityID.String
	item.Read = readAt.Valid
	item.ReadAt = readAt.String

	return item, nil
}

// closeRows releases a result set and logs a failed Close, which a deferred call cannot return.
func closeRows(rows *sql.Rows) {
	if err := rows.Close(); err != nil {
		slog.Warn("in_app_notifications: closing rows", "error", err)
	}
}
