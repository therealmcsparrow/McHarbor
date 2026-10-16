// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// Package capacity owns the retained capacity series and the projections read
// off it.
//
// It lives in core rather than in a feature module because three modules need
// it and none of them should import the others: modules/metrics writes the
// series and serves it, modules/alerts compares a projection against an
// operator's threshold, and modules/incidents stores one as a finding.
// modules/metrics imported no sibling module before capacity forecasting and
// still does not.
//
// The arithmetic is core/forecast's. Nothing here computes a line.
package capacity

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"time"

	"github.com/rs/xid"

	coredb "github.com/therealmcsparrow/mcharbor/core/db"
	"github.com/therealmcsparrow/mcharbor/core/forecast"
)

// timeLayout is the stored timestamp format, matching every other metrics table
// so a join or a range scan never has to translate.
const timeLayout = "2006-01-02 15:04:05"

// BucketSeconds is the series cadence: one sample per subject per hour.
// Filesystem capacity does not move fast enough to justify 2,880 rows a day per
// subject, and a fit needs days rather than minutes.
const BucketSeconds = 3600

// Subject kinds. A rootfs subject is the environment's own filesystem, readable
// on both transports; a storage_location subject is a backup destination, which
// is what a backup actually fills.
const (
	KindRootFS          = "rootfs"
	KindStorageLocation = "storage_location"
)

// Reading is one capacity observation about to be written: used of total, for one
// subject, at one time.
type Reading struct {
	EnvironmentID string
	SubjectKind   string
	SubjectID     string
	SubjectName   string
	UsedBytes     int64
	TotalBytes    int64
}

// TruncateBucket rounds t down to the start of its bucket. Both the writer and
// any on-read sampler must use this, or the unique index stops collapsing two
// writers in the same hour into one row.
func TruncateBucket(t time.Time) time.Time {
	return t.UTC().Truncate(BucketSeconds * time.Second)
}

// Write records one reading in its bucket, replacing the row already there.
//
// A reading whose total is zero is dropped rather than stored: a destination
// whose provider reports no ceiling, or one that never opted into quota
// reporting, is *absent* from the series, and "0 of 0" in a forecast reads as a
// full disk.
func Write(ctx context.Context, db *sql.DB, r Reading, at time.Time) error {
	if r.EnvironmentID == "" || r.SubjectID == "" || r.TotalBytes <= 0 {
		return nil
	}
	stamp := TruncateBucket(at).Format(timeLayout)
	res, err := db.ExecContext(ctx,
		`UPDATE capacity_samples SET used_bytes = ?, total_bytes = ?, subject_name = ?, bucket_seconds = ?,
		 updated_at = CURRENT_TIMESTAMP
		 WHERE environment_id = ? AND subject_kind = ? AND subject_id = ? AND sampled_at = ?`,
		r.UsedBytes, r.TotalBytes, r.SubjectName, BucketSeconds,
		r.EnvironmentID, r.SubjectKind, r.SubjectID, stamp)
	if err != nil {
		return fmt.Errorf("updating capacity sample: %w", err)
	}
	if coredb.RowsAffected(res) > 0 {
		return nil
	}
	_, err = db.ExecContext(ctx,
		`INSERT INTO capacity_samples (id, environment_id, subject_kind, subject_id, subject_name, sampled_at,
		 bucket_seconds, used_bytes, total_bytes) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		xid.New().String(), r.EnvironmentID, r.SubjectKind, r.SubjectID, r.SubjectName, stamp,
		BucketSeconds, r.UsedBytes, r.TotalBytes)
	if err != nil {
		return fmt.Errorf("inserting capacity sample: %w", err)
	}
	return nil
}

// Point is one stored sample, read back.
type Point struct {
	At         time.Time
	UsedBytes  int64
	TotalBytes int64
}

// Subject is one forecastable thing and its retained series, newest last.
type Subject struct {
	EnvironmentID string
	Kind          string
	ID            string
	Name          string
	Points        []Point
}

// Series returns every subject's samples since the cutoff, for one environment
// or for all of them when envID is empty.
//
// The row cap is deliberately generous rather than absent: an hourly series over
// a 30-day horizon is 720 rows per subject, so 200,000 covers roughly 270
// subjects before it truncates, and a truncated series still fits — it simply
// loses its oldest points, which is the half a forecast needs least.
func Series(ctx context.Context, db *sql.DB, envID string, since time.Time) ([]Subject, error) {
	args := []any{since.UTC().Format(timeLayout)}
	where := "sampled_at >= ?"
	if envID != "" {
		where += " AND environment_id = ?"
		args = append(args, envID)
	}
	rows, err := db.QueryContext(ctx,
		`SELECT environment_id, subject_kind, subject_id, COALESCE(subject_name, ''), sampled_at, used_bytes, total_bytes
		 FROM capacity_samples WHERE `+where+` ORDER BY sampled_at LIMIT 200000`, args...)
	if err != nil {
		return nil, fmt.Errorf("reading capacity series: %w", err)
	}
	defer func() {
		if cerr := rows.Close(); cerr != nil {
			slog.Debug("capacity: closing the series rows failed", "error", cerr)
		}
	}()

	index := map[[3]string]int{}
	out := []Subject{}
	for rows.Next() {
		var env, kind, id, name, at string
		var used, total int64
		if err := rows.Scan(&env, &kind, &id, &name, &at, &used, &total); err != nil {
			return nil, fmt.Errorf("scanning capacity sample: %w", err)
		}
		t, err := time.Parse(timeLayout, at)
		if err != nil {
			continue
		}
		key := [3]string{env, kind, id}
		pos, seen := index[key]
		if !seen {
			out = append(out, Subject{EnvironmentID: env, Kind: kind, ID: id, Name: name})
			pos = len(out) - 1
			index[key] = pos
		}
		if name != "" {
			out[pos].Name = name
		}
		out[pos].Points = append(out[pos].Points, Point{At: t, UsedBytes: used, TotalBytes: total})
	}
	return out, rows.Err()
}

// Prune drops samples past the retention horizon. It shares
// retention_metrics_rollup_days rather than minting a sixth retention key: a
// capacity series shorter than the metrics rollup would make the disk forecast
// worse than the memory one for no stated reason.
func Prune(ctx context.Context, db *sql.DB, days int) (int64, error) {
	if days < 1 {
		days = 1
	}
	cutoff := time.Now().UTC().AddDate(0, 0, -days).Format(timeLayout)
	res, err := db.ExecContext(ctx, "DELETE FROM capacity_samples WHERE sampled_at < ?", cutoff)
	if err != nil {
		return 0, fmt.Errorf("pruning capacity samples: %w", err)
	}
	return coredb.RowsAffected(res), nil
}

// percentSeries converts stored bytes into the percentage series a fit reads. A
// point whose total is zero is skipped rather than treated as full.
func percentSeries(pts []Point) []forecast.Sample {
	out := make([]forecast.Sample, 0, len(pts))
	for _, p := range pts {
		if p.TotalBytes <= 0 {
			continue
		}
		out = append(out, forecast.Sample{At: p.At, Value: float64(p.UsedBytes) / float64(p.TotalBytes) * 100})
	}
	return out
}
