// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package table_views

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/rs/xid"

	"github.com/therealmcsparrow/mcharbor/core/db"
	"github.com/therealmcsparrow/mcharbor/internal/closeutil"
)

// Service owns every query against table_views; each one is scoped by user_id so a
// caller can never reach another user's rows.
type Service struct {
	db *sql.DB
}

// NewService returns a Service backed by database.
func NewService(database *sql.DB) *Service {
	return &Service{db: database}
}

func now() string {
	return time.Now().UTC().Format(time.RFC3339)
}

// View returns the caller's layout (nil when never saved) and filters for tableID.
func (s *Service) View(ctx context.Context, userID, tableID string) (*View, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, kind, name, payload, is_default, created_at, updated_at
		 FROM table_views WHERE user_id = ? AND table_id = ?
		 ORDER BY kind, name COLLATE NOCASE LIMIT 1000`, userID, tableID)
	if err != nil {
		return nil, fmt.Errorf("listing table views: %w", err)
	}
	defer func() { closeutil.Report(rows.Close(), "rows") }()

	view := &View{Filters: []Filter{}}
	for rows.Next() {
		var id, kind, name, payload, createdAt, updatedAt string
		var isDefault bool
		if err := rows.Scan(&id, &kind, &name, &payload, &isDefault, &createdAt, &updatedAt); err != nil {
			return nil, fmt.Errorf("scanning table view: %w", err)
		}
		if kind == "layout" {
			view.Layout = &Layout{ID: id, TableID: tableID, Payload: json.RawMessage(payload), UpdatedAt: updatedAt}
			continue
		}
		view.Filters = append(view.Filters, Filter{
			ID: id, TableID: tableID, Name: name, State: json.RawMessage(payload),
			IsDefault: isDefault, CreatedAt: createdAt, UpdatedAt: updatedAt,
		})
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating table views: %w", err)
	}
	if view.Layout == nil {
		view.Layout = defaultLayout(tableID)
	}
	return view, nil
}

// PutLayout creates or replaces the caller's layout for tableID.
func (s *Service) PutLayout(ctx context.Context, userID, tableID string, payload json.RawMessage) (*Layout, error) {
	clean, err := validateLayout(payload)
	if err != nil {
		return nil, err
	}
	ts := now()
	id := xid.New().String()
	_, err = s.db.ExecContext(ctx,
		`INSERT INTO table_views (id, user_id, table_id, kind, name, payload, is_default, created_at, updated_at)
		 VALUES (?, ?, ?, 'layout', '', ?, 0, ?, ?)
		 ON CONFLICT (user_id, table_id, kind, name) DO UPDATE SET payload = excluded.payload, updated_at = excluded.updated_at`,
		id, userID, tableID, string(clean), ts, ts)
	if err != nil {
		return nil, fmt.Errorf("upserting layout: %w", err)
	}
	var storedID string
	if err := s.db.QueryRowContext(ctx,
		`SELECT id FROM table_views WHERE user_id = ? AND table_id = ? AND kind = 'layout' AND name = ''`,
		userID, tableID).Scan(&storedID); err != nil {
		return nil, fmt.Errorf("reading layout id: %w", err)
	}
	return &Layout{ID: storedID, TableID: tableID, Payload: clean, UpdatedAt: ts}, nil
}

// DeleteLayout removes the caller's layout so the page falls back to its defaults.
func (s *Service) DeleteLayout(ctx context.Context, userID, tableID string) error {
	_, err := s.db.ExecContext(ctx,
		`DELETE FROM table_views WHERE user_id = ? AND table_id = ? AND kind = 'layout'`, userID, tableID)
	if err != nil {
		return fmt.Errorf("deleting layout: %w", err)
	}
	return nil
}

// CreateFilter saves a named filter; setting it as default clears any other default
// in the same transaction so there is never more than one.
func (s *Service) CreateFilter(ctx context.Context, userID, tableID string, in FilterInput) (*Filter, error) {
	name, err := normalizeName(in.Name)
	if err != nil {
		return nil, err
	}
	state, err := validateState(in.State)
	if err != nil {
		return nil, err
	}
	f := &Filter{ID: xid.New().String(), TableID: tableID, Name: name, State: state, IsDefault: in.IsDefault}
	f.CreatedAt = now()
	f.UpdatedAt = f.CreatedAt

	err = db.TxContext(ctx, s.db, func(tx *sql.Tx) error {
		var count int
		if err := tx.QueryRowContext(ctx,
			`SELECT COUNT(*) FROM table_views WHERE user_id = ? AND table_id = ? AND kind = 'filter'`,
			userID, tableID).Scan(&count); err != nil {
			return fmt.Errorf("counting filters: %w", err)
		}
		if count >= MaxFiltersPerTable {
			return ErrLimit
		}
		if err := nameFree(ctx, tx, userID, tableID, name, ""); err != nil {
			return err
		}
		if f.IsDefault {
			if err := clearDefault(ctx, tx, userID, tableID); err != nil {
				return err
			}
		}
		_, err := tx.ExecContext(ctx,
			`INSERT INTO table_views (id, user_id, table_id, kind, name, payload, is_default, created_at, updated_at)
			 VALUES (?, ?, ?, 'filter', ?, ?, ?, ?, ?)`,
			f.ID, userID, tableID, name, string(state), f.IsDefault, f.CreatedAt, f.UpdatedAt)
		if err != nil {
			return fmt.Errorf("inserting filter: %w", err)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return f, nil
}

// UpdateFilter applies a partial update to one of the caller's filters.
func (s *Service) UpdateFilter(ctx context.Context, userID, tableID, id string, in FilterUpdate) (*Filter, error) {
	var out *Filter
	err := db.TxContext(ctx, s.db, func(tx *sql.Tx) error {
		current, err := loadFilter(ctx, tx, userID, tableID, id)
		if err != nil {
			return err
		}
		if in.Name != nil {
			name, err := normalizeName(*in.Name)
			if err != nil {
				return err
			}
			if err := nameFree(ctx, tx, userID, tableID, name, id); err != nil {
				return err
			}
			current.Name = name
		}
		if len(in.State) > 0 {
			state, err := validateState(in.State)
			if err != nil {
				return err
			}
			current.State = state
		}
		if in.IsDefault != nil {
			if *in.IsDefault {
				if err := clearDefault(ctx, tx, userID, tableID); err != nil {
					return err
				}
			}
			current.IsDefault = *in.IsDefault
		}
		current.UpdatedAt = now()
		_, err = tx.ExecContext(ctx,
			`UPDATE table_views SET name = ?, payload = ?, is_default = ?, updated_at = ?
			 WHERE id = ? AND user_id = ?`,
			current.Name, string(current.State), current.IsDefault, current.UpdatedAt, id, userID)
		if err != nil {
			return fmt.Errorf("updating filter: %w", err)
		}
		out = current
		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

// DeleteFilter removes one of the caller's filters and returns it for the audit entry.
func (s *Service) DeleteFilter(ctx context.Context, userID, tableID, id string) (*Filter, error) {
	var out *Filter
	err := db.TxContext(ctx, s.db, func(tx *sql.Tx) error {
		current, err := loadFilter(ctx, tx, userID, tableID, id)
		if err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM table_views WHERE id = ? AND user_id = ?`, id, userID); err != nil {
			return fmt.Errorf("deleting filter: %w", err)
		}
		out = current
		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

// Import inserts localStorage-era state that the caller does not already have on the
// server; it is idempotent so a retried import cannot duplicate anything. An invalid
// tableId skips that whole table, an invalid layout skips only the layout, and each
// table's filters stop once the user's existing row count reaches the per-table cap
// — one bad entry never discards the rest of the batch, only a real DB error rolls back.
func (s *Service) Import(ctx context.Context, userID string, in ImportInput) (int, error) {
	if len(in.Tables) > MaxImportTables {
		return 0, ErrInvalid
	}
	imported := 0
	err := db.TxContext(ctx, s.db, func(tx *sql.Tx) error {
		for _, t := range in.Tables {
			if !ValidTableID(t.TableID) {
				continue
			}
			ts := now()
			if len(t.Layout) > 0 {
				if clean, err := validateLayout(t.Layout); err == nil {
					res, err := tx.ExecContext(ctx,
						`INSERT INTO table_views (id, user_id, table_id, kind, name, payload, is_default, created_at, updated_at)
						 VALUES (?, ?, ?, 'layout', '', ?, 0, ?, ?) ON CONFLICT DO NOTHING`,
						xid.New().String(), userID, t.TableID, string(clean), ts, ts)
					if err != nil {
						return fmt.Errorf("importing layout: %w", err)
					}
					imported += int(db.RowsAffected(res))
				}
			}
			if len(t.Filters) == 0 {
				continue
			}
			var count int
			if err := tx.QueryRowContext(ctx,
				`SELECT COUNT(*) FROM table_views WHERE user_id = ? AND table_id = ? AND kind = 'filter'`,
				userID, t.TableID).Scan(&count); err != nil {
				return fmt.Errorf("counting filters: %w", err)
			}
			for _, f := range t.Filters {
				if count >= MaxFiltersPerTable {
					break
				}
				name, err := normalizeName(f.Name)
				if err != nil {
					continue
				}
				state, err := validateState(f.State)
				if err != nil {
					continue
				}
				res, err := tx.ExecContext(ctx,
					`INSERT INTO table_views (id, user_id, table_id, kind, name, payload, is_default, created_at, updated_at)
					 VALUES (?, ?, ?, 'filter', ?, ?, 0, ?, ?) ON CONFLICT DO NOTHING`,
					xid.New().String(), userID, t.TableID, name, string(state), ts, ts)
				if err != nil {
					return fmt.Errorf("importing filter: %w", err)
				}
				n := int(db.RowsAffected(res))
				imported += n
				count += n
			}
		}
		return nil
	})
	if err != nil {
		return 0, err
	}
	return imported, nil
}

func loadFilter(ctx context.Context, tx *sql.Tx, userID, tableID, id string) (*Filter, error) {
	f := &Filter{ID: id, TableID: tableID}
	var payload string
	err := tx.QueryRowContext(ctx,
		`SELECT name, payload, is_default, created_at, updated_at FROM table_views
		 WHERE id = ? AND user_id = ? AND table_id = ? AND kind = 'filter'`,
		id, userID, tableID).Scan(&f.Name, &payload, &f.IsDefault, &f.CreatedAt, &f.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("loading filter: %w", err)
	}
	f.State = json.RawMessage(payload)
	return f, nil
}

func nameFree(ctx context.Context, tx *sql.Tx, userID, tableID, name, exceptID string) error {
	var existing string
	err := tx.QueryRowContext(ctx,
		`SELECT id FROM table_views WHERE user_id = ? AND table_id = ? AND kind = 'filter' AND name = ? AND id <> ?`,
		userID, tableID, name, exceptID).Scan(&existing)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("checking filter name: %w", err)
	}
	return ErrNameTaken
}

func clearDefault(ctx context.Context, tx *sql.Tx, userID, tableID string) error {
	if _, err := tx.ExecContext(ctx,
		`UPDATE table_views SET is_default = 0 WHERE user_id = ? AND table_id = ? AND kind = 'filter'`,
		userID, tableID); err != nil {
		return fmt.Errorf("clearing default filter: %w", err)
	}
	return nil
}
