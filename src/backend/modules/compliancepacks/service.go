// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package compliancepacks

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"sort"
	"time"

	"github.com/rs/xid"

	"github.com/therealmcsparrow/mcharbor/core/db"
	"github.com/therealmcsparrow/mcharbor/modules/compliance"
)

// runRetention is how long a recorded run is kept. A pack exists so "was this
// check passing last month" has an answer, so the window is a year rather than
// the days a metrics series keeps.
const runRetention = 365 * 24 * time.Hour

const runListSelect = `SELECT id, framework, environment_id, trigger_kind, started_by, started_at, finished_at, status,
		checks_pass, checks_warn, checks_fail, checks_info, checks_unknown, last_error
	FROM compliance_pack_runs`

const runListTail = ` ORDER BY started_at DESC, id DESC LIMIT ? OFFSET ?`

// maxRunPage caps a list read the way response.ParsePagination does, so an
// unbounded query cannot be built from a handler that forgot to paginate.
const maxRunPage = 1000

// Service persists pack runs and their control results. It owns all SQL for the
// module; the runner and the handler go through it.
type Service struct {
	db     *sql.DB
	logger *slog.Logger
	now    func() time.Time
}

// NewService returns a Service backed by database. It takes the database rather
// than all of AppDeps because persistence is the only dependency it has.
func NewService(database *sql.DB, logger *slog.Logger) *Service {
	if logger == nil {
		logger = slog.Default()
	}
	return &Service{db: database, logger: logger, now: time.Now}
}

// StartRun records a run as running before any check executes, so a run that
// crashes is visibly unfinished rather than absent. actor is empty for a
// scheduled run and the user's ID for a manual one.
func (s *Service) StartRun(ctx context.Context, framework, envID, trigger, actor string) (string, error) {
	id := xid.New().String()
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO compliance_pack_runs (id, framework, environment_id, trigger_kind, started_by, started_at, status)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		id, framework, envID, trigger, actor, s.now().UTC().Format(time.RFC3339), RunRunning)
	if err != nil {
		return "", fmt.Errorf("insert compliance pack run: %w", err)
	}
	return id, nil
}

// SaveResults writes one row per control and closes the run with its summary. The
// two are one transaction: a summary that disagrees with the rows behind it is the
// one inconsistency an auditor would notice first.
func (s *Service) SaveResults(ctx context.Context, runID string, results []Result) error {
	summary := Summarize(results)
	return s.inTx(ctx, func(tx *sql.Tx) error {
		for _, r := range results {
			facts, err := json.Marshal(r.Facts)
			if err != nil {
				return fmt.Errorf("marshal facts for %s: %w", r.ID, err)
			}
			reasons, err := json.Marshal(r.Reasons)
			if err != nil {
				return fmt.Errorf("marshal reasons for %s: %w", r.ID, err)
			}
			if _, err := tx.ExecContext(ctx,
				`INSERT INTO compliance_pack_results (id, run_id, control_id, control_group, status, facts, reasons)
				 VALUES (?, ?, ?, ?, ?, ?, ?)`,
				xid.New().String(), runID, r.ID, groupOf(r.ID), string(r.Status), string(facts), string(reasons)); err != nil {
				return fmt.Errorf("insert result %s: %w", r.ID, err)
			}
		}
		_, err := tx.ExecContext(ctx,
			`UPDATE compliance_pack_runs
			 SET status = ?, finished_at = ?, updated_at = ?,
			     checks_pass = ?, checks_warn = ?, checks_fail = ?, checks_info = ?, checks_unknown = ?
			 WHERE id = ?`,
			RunCompleted, s.now().UTC().Format(time.RFC3339), s.now().UTC().Format(time.RFC3339),
			summary.Pass, summary.Warn, summary.Fail, summary.Info, summary.Unknown, runID)
		if err != nil {
			return fmt.Errorf("finish compliance pack run: %w", err)
		}
		return nil
	})
}

// FailRun closes a run that could not complete, keeping the message so the row
// can render it beside its state instead of the operator reading a server log.
func (s *Service) FailRun(ctx context.Context, runID, message string) error {
	stamp := s.now().UTC().Format(time.RFC3339)
	result, err := s.db.ExecContext(ctx,
		`UPDATE compliance_pack_runs SET status = ?, finished_at = ?, updated_at = ?, last_error = ? WHERE id = ?`,
		RunFailed, stamp, stamp, message, runID)
	if err != nil {
		return fmt.Errorf("fail compliance pack run: %w", err)
	}
	if db.RowsAffected(result) == 0 {
		return sql.ErrNoRows
	}
	return nil
}

// ListRuns returns runs newest first, optionally narrowed to one framework.
func (s *Service) ListRuns(ctx context.Context, framework string, limit, offset int) ([]Run, int, error) {
	if limit <= 0 || limit > maxRunPage {
		limit = 25
	}
	where, args := "", []any{}
	if framework != "" {
		where, args = " WHERE framework = ?", []any{framework}
	}
	var total int
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM compliance_pack_runs`+where, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count compliance pack runs: %w", err)
	}
	query := runListSelect + where + runListTail // SQL identifier: where is one of two literals chosen above, never caller input
	rows, err := s.db.QueryContext(ctx, query, append(args, limit, offset)...)
	if err != nil {
		return nil, 0, fmt.Errorf("select compliance pack runs: %w", err)
	}
	defer func() {
		if err := rows.Close(); err != nil {
			slog.Warn("compliance packs: closing rows failed", "error", err)
		}
	}()
	out := []Run{}
	for rows.Next() {
		r, err := scanRun(rows)
		if err != nil {
			return nil, 0, err
		}
		out = append(out, r)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("scan compliance pack runs: %w", err)
	}
	return out, total, nil
}

// LatestRun returns the newest completed run for a framework, or false when the
// pack has never been run. A pack with no run says so rather than showing an
// empty control table that reads as "nothing wrong".
func (s *Service) LatestRun(ctx context.Context, framework string) (Run, bool, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT id, framework, environment_id, trigger_kind, started_by, started_at, finished_at, status,
		        checks_pass, checks_warn, checks_fail, checks_info, checks_unknown, last_error
		 FROM compliance_pack_runs WHERE framework = ? ORDER BY started_at DESC, id DESC LIMIT 1`, framework)
	r, err := scanRun(row)
	if errors.Is(err, sql.ErrNoRows) {
		return Run{}, false, nil
	}
	if err != nil {
		return Run{}, false, err
	}
	return r, true, nil
}

// RunDetail returns a run with its control results in framework order.
func (s *Service) RunDetail(ctx context.Context, runID string) (RunDetail, bool, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT id, framework, environment_id, trigger_kind, started_by, started_at, finished_at, status,
		        checks_pass, checks_warn, checks_fail, checks_info, checks_unknown, last_error
		 FROM compliance_pack_runs WHERE id = ?`, runID)
	run, err := scanRun(row)
	if errors.Is(err, sql.ErrNoRows) {
		return RunDetail{}, false, nil
	}
	if err != nil {
		return RunDetail{}, false, err
	}
	results, err := s.results(ctx, runID)
	if err != nil {
		return RunDetail{}, false, err
	}
	if f, ok := FrameworkByKey(run.Framework); ok {
		order := ControlOrder(f)
		sort.SliceStable(results, func(i, j int) bool { return order[results[i].ID] < order[results[j].ID] })
	}
	return RunDetail{Run: run, Results: results}, true, nil
}

func (s *Service) results(ctx context.Context, runID string) ([]Result, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT control_id, status, facts, reasons FROM compliance_pack_results
		 WHERE run_id = ? ORDER BY control_id LIMIT ?`, runID, maxRunPage)
	if err != nil {
		return nil, fmt.Errorf("select compliance pack results: %w", err)
	}
	defer func() {
		if err := rows.Close(); err != nil {
			slog.Warn("compliance packs: closing rows failed", "error", err)
		}
	}()
	out := []Result{}
	for rows.Next() {
		var r Result
		var status, facts, reasons string
		if err := rows.Scan(&r.ID, &status, &facts, &reasons); err != nil {
			return nil, fmt.Errorf("scan compliance pack result: %w", err)
		}
		r.Status = compliance.Status(status)
		if err := json.Unmarshal([]byte(facts), &r.Facts); err != nil {
			return nil, fmt.Errorf("unmarshal facts for %s: %w", r.ID, err)
		}
		if err := json.Unmarshal([]byte(reasons), &r.Reasons); err != nil {
			return nil, fmt.Errorf("unmarshal reasons for %s: %w", r.ID, err)
		}
		if r.Facts == nil {
			r.Facts = []Fact{}
		}
		if r.Reasons == nil {
			r.Reasons = []string{}
		}
		out = append(out, r)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("scan compliance pack results: %w", err)
	}
	return out, nil
}

// Prune removes runs past the retention window and the results behind them.
// Results are deleted first: a result row whose run is gone is invisible, and
// invisible rows accumulate forever.
func (s *Service) Prune(ctx context.Context) error {
	cutoff := s.now().UTC().Add(-runRetention).Format(time.RFC3339)
	return s.inTx(ctx, func(tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx,
			`DELETE FROM compliance_pack_results WHERE run_id IN
			 (SELECT id FROM compliance_pack_runs WHERE started_at < ?)`, cutoff); err != nil {
			return fmt.Errorf("prune compliance pack results: %w", err)
		}
		if _, err := tx.ExecContext(ctx,
			`DELETE FROM compliance_pack_runs WHERE started_at < ?`, cutoff); err != nil {
			return fmt.Errorf("prune compliance pack runs: %w", err)
		}
		return nil
	})
}

// scanner is the shared shape of sql.Row and sql.Rows, so one scan function
// serves both the list and the single-row reads.
type scanner interface {
	Scan(dest ...any) error
}

func scanRun(row scanner) (Run, error) {
	var r Run
	var finished sql.NullString
	var sum compliance.Summary
	if err := row.Scan(&r.ID, &r.Framework, &r.EnvironmentID, &r.Trigger, &r.StartedBy, &r.StartedAt,
		&finished, &r.Status, &sum.Pass, &sum.Warn, &sum.Fail, &sum.Info, &sum.Unknown, &r.LastError); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Run{}, err
		}
		return Run{}, fmt.Errorf("scan compliance pack run: %w", err)
	}
	r.FinishedAt, r.Summary = finished.String, sum
	return r, nil
}

// groupOf resolves a control's group from the registry so the results table can be
// filtered by section without re-reading the framework.
func groupOf(controlID string) string {
	for _, f := range Frameworks() {
		for _, c := range f.Controls {
			if c.ID == controlID {
				return c.Group
			}
		}
	}
	return ""
}

// inTx runs fn in a transaction bound to ctx and rolls back on error; db.Tx
// cannot be used because it opens its transaction on context.Background().
func (s *Service) inTx(ctx context.Context, fn func(tx *sql.Tx) error) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("beginning transaction: %w", err)
	}
	if err := fn(tx); err != nil {
		db.RollbackTx(tx)
		return err
	}
	return tx.Commit()
}
