// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package support

import (
	"archive/zip"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"time"

	"github.com/rs/xid"

	appversion "github.com/therealmcsparrow/mcharbor/core/version"
	"github.com/therealmcsparrow/mcharbor/internal/closeutil"
)

// Bundle errors the handler maps onto response codes.
var (
	ErrNoCategories   = errors.New("support: no bundle categories chosen")
	ErrBadCategory    = errors.New("support: unknown bundle category")
	ErrBundleRunning  = errors.New("support: a bundle is already being generated")
	ErrBundleNotFound = errors.New("support: bundle not found")
	ErrBundleNotReady = errors.New("support: bundle is not ready")
)

var bundleIDPattern = regexp.MustCompile(`^[a-z0-9]{20}$`)

// previewLimit caps each previewed file; the preview says when it was cut, and the bundle
// itself always carries the full redacted file.
const previewLimit = 64 << 10

// bundleFile is one file of a bundle before it is written.
type bundleFile struct {
	name string
	data []byte
}

// normalizeCategories validates and de-duplicates a category selection in canonical order.
func normalizeCategories(in []string) ([]string, error) {
	if len(in) == 0 {
		return nil, ErrNoCategories
	}
	want := map[string]bool{}
	for _, c := range in {
		found := false
		for _, known := range BundleCategories {
			if c == known {
				found = true
			}
		}
		if !found {
			return nil, ErrBadCategory
		}
		want[c] = true
	}
	out := make([]string, 0, len(want))
	for _, c := range BundleCategories {
		if want[c] {
			out = append(out, c)
		}
	}
	return out, nil
}

// buildFiles gathers every chosen category and scrubs the result. Both the preview and the
// written bundle come from here, so the preview is exactly what the zip will hold.
func (s *Service) buildFiles(ctx context.Context, cats []string, progress func(step string, pct int)) ([]bundleFile, error) {
	files := []bundleFile{}
	for i, c := range cats {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if progress != nil {
			progress(c, 5+i*85/len(cats))
		}
		v, err := s.collect(ctx, c)
		if err != nil {
			s.d.Logger.Warn("support: collecting bundle category", "category", c, "error", err)
			v = map[string]string{"error": "collection failed; see the server log"}
		}
		raw, err := json.MarshalIndent(v, "", "  ")
		if err != nil {
			return nil, fmt.Errorf("encoding %s: %w", c, err)
		}
		files = append(files, bundleFile{name: c + ".json", data: []byte(s.scrubber.Scrub(string(raw)))})
	}
	manifest := map[string]any{
		"product":     "McHarbor",
		"version":     appversion.Current(),
		"build":       appversion.Build(),
		"generatedAt": s.now().UTC().Format(time.RFC3339),
		"categories":  cats,
		"redaction":   "Environment values are printed only for allow-listed variables; every file is scrubbed for credentials, URLs with passwords, PEM blocks, bearer tokens and known secret values. Kubeconfigs, tokens, keys, passwords, agent tokens and API keys are never collected.",
	}
	raw, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("encoding manifest: %w", err)
	}
	return append([]bundleFile{{name: "manifest.json", data: raw}}, files...), nil
}

// collect returns one category's content. Every query names its columns explicitly so a
// credential column added later can never ride along.
func (s *Service) collect(ctx context.Context, cat string) (any, error) {
	switch cat {
	case CatSystem:
		environ, lookup := processEnv()
		lookup = configuredLookup(lookup, map[string]bool{"MCHARBOR_SECRET": s.d.SecretSet, "ENCRYPTION_KEY": s.d.EncryptionKeySet, "MCHARBOR_DB_DSN": s.d.DBDSNSet})
		host, hostErr := os.Hostname()
		if hostErr != nil {
			host = ""
		}
		return map[string]any{
			"version": appversion.Current(), "build": appversion.Build(),
			"goVersion": runtime.Version(), "platform": runtime.GOOS + "/" + runtime.GOARCH,
			"hostname": host, "numCPU": runtime.NumCPU(),
			"databaseDriver": s.d.DatabaseDriver, "encryptionKey": s.EncryptionKeySource(),
			"environment": RedactedEnv(lookup, environ, s.scrubber),
		}, nil
	case CatLogs:
		if s.d.Logs == nil {
			return map[string]any{"retained": false, "note": "This process keeps no log history; collect `docker logs` from the McHarbor container."}, nil
		}
		return map[string]any{"retained": true, "lines": s.d.Logs.Snapshot()}, nil
	case CatPlugins:
		states, ok := []PluginState(nil), false
		if s.d.Plugins != nil {
			states, ok = s.d.Plugins()
		}
		installed, err := s.installedPlugins(ctx)
		if err != nil {
			return nil, err
		}
		return map[string]any{"runtimeActive": ok, "loader": states, "enabled": installed}, nil
	case CatCluster:
		if s.d.Cluster == nil {
			return map[string]any{"available": false}, nil
		}
		view, ok := s.d.Cluster(ctx)
		return map[string]any{"available": ok, "status": view}, nil
	case CatDatabase:
		applied, pending, unknown, err := s.migrationState(ctx)
		if err != nil {
			return nil, err
		}
		return map[string]any{
			"driver": s.d.DatabaseDriver, "version": s.databaseVersion(ctx),
			"applied": applied, "pending": pending, "notInThisBuild": unknown,
		}, nil
	case CatEnvironments:
		return s.environmentSummary(ctx)
	case CatAudit:
		return s.auditDenials(ctx)
	case CatDiagnostics:
		return s.Diagnostics(ctx, nil)
	}
	return nil, ErrBadCategory
}

func (s *Service) environmentSummary(ctx context.Context) (any, error) {
	rows, err := s.d.DB.QueryContext(ctx, `SELECT id, name, connection_type, is_active,
		COALESCE(agent_status, ''), COALESCE(agent_version, ''), COALESCE(docker_version, '')
		FROM environments ORDER BY name LIMIT 1000`)
	if err != nil {
		return nil, fmt.Errorf("listing environments: %w", err)
	}
	defer func() { closeutil.Report(rows.Close(), "rows") }()
	type env struct {
		ID           string `json:"id"`
		Name         string `json:"name"`
		Connection   string `json:"connectionType"`
		Active       bool   `json:"active"`
		AgentStatus  string `json:"agentStatus,omitempty"`
		AgentVersion string `json:"agentVersion,omitempty"`
		Docker       string `json:"dockerVersion,omitempty"`
	}
	out := []env{}
	for rows.Next() {
		var e env
		var active int
		if err := rows.Scan(&e.ID, &e.Name, &e.Connection, &active, &e.AgentStatus, &e.AgentVersion, &e.Docker); err != nil {
			return nil, fmt.Errorf("scanning environment: %w", err)
		}
		e.Active = active == 1
		out = append(out, e)
	}
	return out, rows.Err()
}

func (s *Service) auditDenials(ctx context.Context) (any, error) {
	now := s.now().UTC()
	out := map[string]any{}
	// bounded by the two fixed windows in the literal below.
	for _, w := range []struct {
		key string
		d   time.Duration
	}{{"last24h", 24 * time.Hour}, {"last7d", 7 * 24 * time.Hour}} {
		counts, err := s.denialCounts(ctx, now.Add(-w.d))
		if err != nil {
			return nil, err
		}
		out[w.key] = counts
	}
	return out, nil
}

// denialCounts tallies permission and ABAC denials recorded since the given time, by action.
func (s *Service) denialCounts(ctx context.Context, since time.Time) (map[string]int, error) {
	rows, err := s.d.DB.QueryContext(ctx, `SELECT action, COUNT(*) FROM audit_logs
		WHERE action IN ('permission.denied', 'abac.denied') AND created_at >= ?
		GROUP BY action LIMIT 1000`, since.Format(time.RFC3339))
	if err != nil {
		return nil, fmt.Errorf("counting denials: %w", err)
	}
	defer func() { closeutil.Report(rows.Close(), "rows") }()
	counts := map[string]int{}
	for rows.Next() {
		var action string
		var n int
		if err := rows.Scan(&action, &n); err != nil {
			return nil, fmt.Errorf("scanning denials: %w", err)
		}
		counts[action] = n
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("counting denials: %w", err)
	}
	return counts, nil
}

// Preview returns exactly what a bundle with cats would contain, each file capped for display.
func (s *Service) Preview(ctx context.Context, cats []string) (BundlePreview, error) {
	cats, err := normalizeCategories(cats)
	if err != nil {
		return BundlePreview{}, err
	}
	files, err := s.buildFiles(ctx, cats, nil)
	if err != nil {
		return BundlePreview{}, err
	}
	p := BundlePreview{Categories: cats, Files: make([]PreviewFile, 0, len(files))}
	for _, f := range files {
		pf := PreviewFile{Name: f.name, SizeBytes: int64(len(f.data)), Content: string(f.data)}
		if len(f.data) > previewLimit {
			pf.Content, pf.Truncated = string(f.data[:previewLimit]), true
		}
		p.TotalBytes += pf.SizeBytes
		p.Files = append(p.Files, pf)
	}
	return p, nil
}

// StartBundle begins generating a bundle in the background and returns its running record.
// Only one runs at a time: two concurrent bundles would double the diagnostics load for no gain.
func (s *Service) StartBundle(cats []string, createdBy string) (Bundle, error) {
	cats, err := normalizeCategories(cats)
	if err != nil {
		return Bundle{}, err
	}
	s.mu.Lock()
	if len(s.running) > 0 {
		s.mu.Unlock()
		return Bundle{}, ErrBundleRunning
	}
	b := &Bundle{
		ID: xid.New().String(), Status: BundleRunning, Categories: cats,
		CreatedAt: s.now().UTC().Format(time.RFC3339), CreatedBy: createdBy, Step: "start",
	}
	s.running[b.ID] = b
	snapshot := *b
	s.wg.Add(1)
	s.mu.Unlock()
	go func() {
		defer s.wg.Done()
		s.generate(s.ctx, b)
	}()
	return snapshot, nil
}

func (s *Service) setProgress(b *Bundle, step string, pct int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	b.Step, b.Progress = step, pct
}

func (s *Service) generate(parent context.Context, b *Bundle) {
	ctx, cancel := context.WithTimeout(parent, 5*time.Minute)
	defer cancel()
	final := s.writeBundle(ctx, b)
	if err := s.writeMeta(final); err != nil {
		s.d.Logger.Error("support: writing bundle record", "id", final.ID, "error", err)
	}
	s.mu.Lock()
	delete(s.running, b.ID)
	s.mu.Unlock()
	s.prune()
}

func (s *Service) writeBundle(ctx context.Context, b *Bundle) Bundle {
	fail := func(err error) Bundle {
		s.d.Logger.Error("support: bundle generation failed", "id", b.ID, "error", err)
		s.mu.Lock()
		defer s.mu.Unlock()
		out := *b
		out.Status, out.Error, out.FinishedAt = BundleFailed, "generation_failed", s.now().UTC().Format(time.RFC3339)
		return out
	}
	if err := os.MkdirAll(s.bundleDir(), 0o700); err != nil {
		return fail(fmt.Errorf("creating bundle directory: %w", err))
	}
	files, err := s.buildFiles(ctx, b.Categories, func(step string, pct int) { s.setProgress(b, step, pct) })
	if err != nil {
		return fail(err)
	}
	s.setProgress(b, "archive", 92)
	final := filepath.Join(s.bundleDir(), b.ID+".zip")
	tmp := final + ".partial"
	if err := writeZip(tmp, files); err != nil {
		s.removeQuiet(tmp)
		return fail(err)
	}
	if err := os.Rename(tmp, final); err != nil {
		s.removeQuiet(tmp)
		return fail(fmt.Errorf("finalising bundle: %w", err))
	}
	st, err := os.Stat(final)
	if err != nil {
		return fail(fmt.Errorf("reading bundle size: %w", err))
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	out := *b
	out.Status, out.Progress, out.Step, out.SizeBytes = BundleReady, 100, "done", st.Size()
	out.FinishedAt = s.now().UTC().Format(time.RFC3339)
	return out
}

func writeZip(path string, files []bundleFile) error {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		return fmt.Errorf("creating bundle: %w", err)
	}
	zw := zip.NewWriter(f)
	for _, bf := range files {
		w, err := zw.Create(bf.name)
		if err != nil {
			closeutil.Report(f.Close(), "f")
			return fmt.Errorf("adding %s: %w", bf.name, err)
		}
		if _, err := w.Write(bf.data); err != nil {
			closeutil.Report(f.Close(), "f")
			return fmt.Errorf("writing %s: %w", bf.name, err)
		}
	}
	if err := zw.Close(); err != nil {
		closeutil.Report(f.Close(), "f")
		return fmt.Errorf("closing archive: %w", err)
	}
	return f.Close()
}

func (s *Service) writeMeta(b Bundle) error {
	raw, err := json.Marshal(b)
	if err != nil {
		return fmt.Errorf("encoding bundle record: %w", err)
	}
	if err := os.MkdirAll(s.bundleDir(), 0o700); err != nil {
		return fmt.Errorf("creating bundle directory: %w", err)
	}
	return os.WriteFile(filepath.Join(s.bundleDir(), b.ID+".json"), raw, 0o600)
}

// ListBundles returns running bundles first, then finished ones newest first.
func (s *Service) ListBundles() ([]Bundle, error) {
	out := []Bundle{}
	s.mu.Lock()
	for _, b := range s.running {
		out = append(out, *b)
	}
	s.mu.Unlock()
	entries, err := os.ReadDir(s.bundleDir())
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return nil, fmt.Errorf("reading bundle directory: %w", err)
	}
	var done []Bundle
	for _, e := range entries {
		name := e.Name()
		if filepath.Ext(name) != ".json" || !bundleIDPattern.MatchString(name[:len(name)-5]) {
			continue
		}
		raw, err := os.ReadFile(filepath.Join(s.bundleDir(), name))
		if err != nil {
			continue
		}
		var b Bundle
		if json.Unmarshal(raw, &b) == nil && b.ID != "" {
			done = append(done, b)
		}
	}
	sort.Slice(done, func(i, j int) bool {
		return done[i].CreatedAt > done[j].CreatedAt || (done[i].CreatedAt == done[j].CreatedAt && done[i].ID > done[j].ID)
	})
	return append(out, done...), nil
}

// BundleByID returns one bundle's record.
func (s *Service) BundleByID(id string) (Bundle, error) {
	if !bundleIDPattern.MatchString(id) {
		return Bundle{}, ErrBundleNotFound
	}
	s.mu.Lock()
	if b, ok := s.running[id]; ok {
		out := *b
		s.mu.Unlock()
		return out, nil
	}
	s.mu.Unlock()
	raw, err := os.ReadFile(filepath.Join(s.bundleDir(), id+".json"))
	if err != nil {
		return Bundle{}, ErrBundleNotFound
	}
	var b Bundle
	if err := json.Unmarshal(raw, &b); err != nil {
		return Bundle{}, ErrBundleNotFound
	}
	return b, nil
}

// BundlePath returns the zip of a ready bundle; the id is validated so it can never name a
// path outside the bundle directory.
func (s *Service) BundlePath(id string) (string, Bundle, error) {
	b, err := s.BundleByID(id)
	if err != nil {
		return "", Bundle{}, err
	}
	if b.Status != BundleReady {
		return "", b, ErrBundleNotReady
	}
	p := filepath.Join(s.bundleDir(), id+".zip")
	if _, err := os.Stat(p); err != nil {
		return "", b, ErrBundleNotFound
	}
	return p, b, nil
}

// DeleteBundle removes a finished bundle and its record.
func (s *Service) DeleteBundle(id string) (Bundle, error) {
	b, err := s.BundleByID(id)
	if err != nil {
		return Bundle{}, err
	}
	if b.Status == BundleRunning {
		return b, ErrBundleRunning
	}
	for _, ext := range []string{".zip", ".json"} {
		if err := os.Remove(filepath.Join(s.bundleDir(), id+ext)); err != nil && !errors.Is(err, fs.ErrNotExist) {
			return b, fmt.Errorf("deleting bundle: %w", err)
		}
	}
	return b, nil
}

// prune keeps the newest s.keep finished bundles and deletes the rest.
func (s *Service) prune() {
	list, err := s.ListBundles()
	if err != nil {
		s.d.Logger.Warn("support: listing bundles for pruning", "error", err)
		return
	}
	kept := 0
	for _, b := range list {
		if b.Status == BundleRunning {
			continue
		}
		kept++
		if kept <= s.keep {
			continue
		}
		if _, err := s.DeleteBundle(b.ID); err != nil {
			s.d.Logger.Warn("support: pruning bundle", "id", b.ID, "error", err)
		}
	}
}

// Keep is how many finished bundles history retains.
func (s *Service) Keep() int { return s.keep }

func (s *Service) removeQuiet(path string) {
	if err := os.Remove(path); err != nil && !errors.Is(err, fs.ErrNotExist) {
		s.d.Logger.Warn("support: removing partial bundle", "path", path, "error", err)
	}
}
