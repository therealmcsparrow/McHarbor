// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package container_backups

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/rs/xid"

	"github.com/therealmcsparrow/mcharbor/core/i18n"
)

const (
	tempDirEnv              = "MCHARBOR_BACKUP_TEMP_DIR"
	tempDirSetting          = "backup_temp_dir"
	tempRetiredSetting      = "backup_temp_dir_retired"
	tempSettingsCategory    = "backups"
	tempRetiredMax          = 10
	tempSourceEnv           = "env"
	tempSourceSetting       = "setting"
	tempSourceDefault       = "default"
	tempProbePrefix         = "mcharbor-probe-"
	tempSettingQueryTimeout = 10 * time.Second
)

// tempRootPrefixes names the only top-level entries a temp root may hold that McHarbor created; a sweep or
// cleanup must never delete anything else, because an admin may point the root at a shared disk.
var tempRootPrefixes = []string{"mcharbor-run-", "mcharbor-backup-entry-", "mcharbor-frozen-", tempProbePrefix, "restore-subset-"}

var (
	errTempNotAbsolute   = errors.New("backup temp folder must be an absolute path")
	errTempTraversal     = errors.New("backup temp folder must not contain a .. segment")
	errTempSensitive     = errors.New("backup temp folder is a protected location")
	errTempWorldWritable = errors.New("backup temp folder is writable by every user")
	errTempNotWritable   = errors.New("backup temp folder is not writable")
	errTempEnvOverride   = errors.New("backup temp folder is set by " + tempDirEnv)
)

// posixSensitiveRoots are refused with everything beneath them: writing archives there can break the host.
var posixSensitiveRoots = []string{"/etc", "/proc", "/sys", "/dev", "/boot", "/bin", "/sbin", "/lib", "/lib64", "/usr/bin", "/usr/sbin", "/usr/lib"}

// BackupTempStatus is the answer of GET and PUT /container-backups/temp.
type BackupTempStatus struct {
	Path           string   `json:"path"`
	Source         string   `json:"source"`
	SettingPath    string   `json:"settingPath"`
	EnvOverride    bool     `json:"envOverride"`
	EnvInvalidCode string   `json:"envInvalidCode"`
	DefaultPath    string   `json:"defaultPath"`
	SizeBytes      int64    `json:"sizeBytes"`
	Entries        int      `json:"entries"`
	ActiveRuns     int      `json:"activeRuns"`
	RetiredPaths   []string `json:"retiredPaths"`
}

var (
	envInvalidMu     sync.Mutex
	envInvalidLogged string
)

// tempErrCode maps a validateTempRoot failure to its i18n code, so the handler and the env fallback agree.
func tempErrCode(err error) i18n.MsgCode {
	switch {
	case errors.Is(err, errTempNotAbsolute):
		return i18n.ErrAbsolutePathReq
	case errors.Is(err, errTempTraversal):
		return i18n.ErrPathTraversal
	case errors.Is(err, errTempSensitive):
		return i18n.ErrBackupTempPathSensitive
	case errors.Is(err, errTempWorldWritable):
		return i18n.ErrBackupTempWorldWritable
	case errors.Is(err, errTempNotWritable):
		return i18n.ErrBackupTempNotWritable
	}
	return ""
}

// defaultTempRoot is where new runs work when nothing else is configured.
func defaultTempRoot(dataDir string) string {
	abs, err := filepath.Abs(dataDir)
	if err != nil {
		abs = filepath.Clean(dataDir)
	}
	return filepath.Join(abs, "backups", "tmp")
}

type rowQuerier interface {
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

// readTempSetting returns the stored value of key, or "" when the row is absent or empty.
func readTempSetting(ctx context.Context, q rowQuerier, key string) (string, error) {
	var v sql.NullString
	err := q.QueryRowContext(ctx, `SELECT value FROM settings WHERE key = ?`, key).Scan(&v)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("reading setting %s: %w", key, err)
	}
	return v.String, nil
}

// resolveTempRoot picks the root new runs use: the env var, then the saved setting, then the default under dataDir.
// A rejected env value falls back to the default and is reported in envInvalid; it is logged once per distinct value.
func resolveTempRoot(ctx context.Context, db *sql.DB, dataDir string, getenv func(string) string) (root, source string, envInvalid i18n.MsgCode) {
	fallback := defaultTempRoot(dataDir)
	if raw := getenv(tempDirEnv); raw != "" {
		clean, err := validateTempRoot(raw, dataDir)
		if err == nil {
			return clean, tempSourceEnv, ""
		}
		envInvalid = tempErrCode(err)
		logEnvInvalidOnce(raw, err)
		return fallback, tempSourceDefault, envInvalid
	}
	qctx, cancel := context.WithTimeout(ctx, tempSettingQueryTimeout)
	defer cancel()
	saved, err := readTempSetting(qctx, db, tempDirSetting)
	if err != nil {
		slog.Warn("backup temp folder setting unreadable, using the default", "error", err)
		return fallback, tempSourceDefault, ""
	}
	if saved != "" {
		clean, err := checkTempRootPath(saved, dataDir)
		if err == nil {
			return clean, tempSourceSetting, ""
		}
		logStoredRootRejectedOnce(tempDirSetting, saved, err)
	}
	return fallback, tempSourceDefault, ""
}

var (
	storedRejectedMu     sync.Mutex
	storedRejectedLogged = map[string]struct{}{}
)

// logStoredRootRejectedOnce logs a stored root that failed re-validation once per distinct value, because the
// resolver runs on every backup and sweep and a row written outside the dedicated route must not flood the log.
func logStoredRootRejectedOnce(setting, raw string, err error) {
	storedRejectedMu.Lock()
	defer storedRejectedMu.Unlock()
	k := setting + "\x00" + raw
	if _, ok := storedRejectedLogged[k]; ok {
		return
	}
	if len(storedRejectedLogged) >= 64 {
		storedRejectedLogged = map[string]struct{}{}
	}
	storedRejectedLogged[k] = struct{}{}
	slog.Error("stored backup temp folder rejected, ignoring it", "setting", setting, "path", raw, "error", err)
}

// safeRetiredRoots keeps only the retired roots that still pass the cheap path checks, so a tampered list can never
// point the sweep at a protected or relative location.
func safeRetiredRoots(roots []string, dataDir string) []string {
	out := make([]string, 0, len(roots))
	for _, raw := range roots {
		clean, err := checkTempRootPath(raw, dataDir)
		if err != nil {
			logStoredRootRejectedOnce(tempRetiredSetting, raw, err)
			continue
		}
		out = append(out, clean)
	}
	return out
}

// checkTempRootPath is the part of validateTempRoot that only reads the path: absolute, no `..` segment, not a
// protected location before or after symlink resolution. It never creates or writes anything.
func checkTempRootPath(raw, dataDir string) (string, error) {
	if raw == "" || !filepath.IsAbs(raw) {
		return "", errTempNotAbsolute
	}
	if hasDotDotSegment(raw) {
		return "", errTempTraversal
	}
	clean := filepath.Clean(raw)
	if tempPathSensitive(clean, dataDir) || tempPathSensitive(resolveExistingPath(clean), dataDir) {
		return "", errTempSensitive
	}
	return clean, nil
}

func logEnvInvalidOnce(raw string, err error) {
	envInvalidMu.Lock()
	defer envInvalidMu.Unlock()
	if envInvalidLogged == raw {
		return
	}
	envInvalidLogged = raw
	slog.Error("backup temp folder from environment rejected, using the default", "env", tempDirEnv, "error", err)
}

// validateTempRoot checks raw against the allow-list rules and probes it for writability, returning the cleaned
// path. The `..` check runs on the raw input because Clean would erase the evidence.
func validateTempRoot(raw, dataDir string) (string, error) {
	clean, err := checkTempRootPath(raw, dataDir)
	if err != nil {
		return "", err
	}
	if runtime.GOOS != "windows" {
		if info, err := os.Stat(clean); err == nil && info.IsDir() && info.Mode().Perm()&0o002 != 0 {
			return "", errTempWorldWritable
		}
	}
	if err := probeTempRoot(clean); err != nil {
		return "", fmt.Errorf("%w: %w", errTempNotWritable, err)
	}
	return clean, nil
}

func hasDotDotSegment(raw string) bool {
	parts := strings.FieldsFunc(raw, func(r rune) bool { return r == '/' || r == os.PathSeparator })
	for _, p := range parts {
		if p == ".." {
			return true
		}
	}
	return false
}

// resolveExistingPath resolves symlinks in the longest existing prefix of p and re-appends the rest, so a
// not-yet-created folder under a link into /etc is still seen for what it is.
func resolveExistingPath(p string) string {
	rest := ""
	cur := p
	for {
		if resolved, err := filepath.EvalSymlinks(cur); err == nil {
			return filepath.Join(resolved, rest)
		}
		parent := filepath.Dir(cur)
		if parent == cur {
			return p
		}
		rest = filepath.Join(filepath.Base(cur), rest)
		cur = parent
	}
}

// pathWithin reports whether child equals parent or sits beneath it.
func pathWithin(parent, child string) bool {
	rel, err := filepath.Rel(parent, child)
	if err != nil {
		return false
	}
	return rel != ".." && !strings.HasPrefix(rel, ".."+string(os.PathSeparator))
}

// tempPathSensitive refuses the filesystem root, the POSIX system folders, and the data directory, the local backup
// directory or any ancestor of either.
func tempPathSensitive(p, dataDir string) bool {
	if filepath.Dir(p) == p {
		return true
	}
	if os.PathSeparator == '/' {
		for _, root := range posixSensitiveRoots {
			if pathWithin(root, p) {
				return true
			}
		}
	}
	abs, err := filepath.Abs(dataDir)
	if err != nil {
		abs = filepath.Clean(dataDir)
	}
	for _, d := range []string{abs, resolveExistingPath(abs)} {
		if pathWithin(p, d) || pathWithin(p, LocalBackupDir(d)) {
			return true
		}
	}
	return false
}

// probeTempRoot creates dir if needed and proves McHarbor can create, write and delete a file there. The probe file
// is created with O_EXCL under an xid name and always removed.
func probeTempRoot(dir string) (err error) {
	if err = os.MkdirAll(dir, 0o750); err != nil {
		return fmt.Errorf("creating temp folder: %w", err)
	}
	probe := filepath.Join(dir, tempProbePrefix+xid.New().String())
	f, err := os.OpenFile(probe, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return fmt.Errorf("creating probe file: %w", err)
	}
	defer func() {
		if rmErr := os.Remove(probe); rmErr != nil && !errors.Is(rmErr, fs.ErrNotExist) && err == nil {
			err = fmt.Errorf("removing probe file: %w", rmErr)
		}
	}()
	if _, werr := f.Write([]byte("mcharbor")); werr != nil {
		if cerr := f.Close(); cerr != nil {
			slog.Debug("closing probe file after failed write", "error", cerr)
		}
		return fmt.Errorf("writing probe file: %w", werr)
	}
	if cerr := f.Close(); cerr != nil {
		return fmt.Errorf("closing probe file: %w", cerr)
	}
	return nil
}

// isTempRootEntry reports whether name is a top-level entry McHarbor itself creates in a temp root.
func isTempRootEntry(name string) bool {
	for _, p := range tempRootPrefixes {
		if strings.HasPrefix(name, p) {
			return true
		}
	}
	return false
}

// scanTempRoot counts the McHarbor-named top-level entries of root and the bytes beneath them; symlinks are not followed.
func scanTempRoot(root string) (entries int, size int64) {
	list, err := os.ReadDir(root)
	if err != nil {
		return 0, 0
	}
	for _, e := range list {
		if !isTempRootEntry(e.Name()) {
			continue
		}
		entries++
		walkErr := filepath.WalkDir(filepath.Join(root, e.Name()), func(_ string, d fs.DirEntry, err error) error {
			if err != nil {
				return nil //nolint:nilerr // an unreadable leaf only under-counts the size shown to the admin
			}
			if !d.IsDir() {
				if info, ierr := d.Info(); ierr == nil {
					size += info.Size()
				}
			}
			return nil
		})
		if walkErr != nil {
			slog.Debug("backup temp size scan stopped", "root", root, "error", walkErr)
		}
	}
	return entries, size
}

// retiredTempRoots returns the saved retired roots, newest first.
func retiredTempRoots(ctx context.Context, q rowQuerier) ([]string, error) {
	raw, err := readTempSetting(ctx, q, tempRetiredSetting)
	if err != nil || raw == "" {
		return []string{}, err
	}
	var roots []string
	if err := json.Unmarshal([]byte(raw), &roots); err != nil {
		return []string{}, fmt.Errorf("parsing retired backup temp folders: %w", err)
	}
	if roots == nil {
		roots = []string{}
	}
	return roots, nil
}

// TempStatus reports the root new runs use, where it came from, and what McHarbor-named data sits in it and in retired roots.
func (s *Service) TempStatus(ctx context.Context) (*BackupTempStatus, error) {
	root, source, invalid := resolveTempRoot(ctx, s.db, s.dataDir, s.getenv)
	qctx, cancel := context.WithTimeout(ctx, tempSettingQueryTimeout)
	defer cancel()
	saved, err := readTempSetting(qctx, s.db, tempDirSetting)
	if err != nil {
		return nil, err
	}
	retired, err := retiredTempRoots(qctx, s.db)
	if err != nil {
		return nil, err
	}
	st := &BackupTempStatus{
		Path:           root,
		Source:         source,
		SettingPath:    saved,
		EnvOverride:    s.getenv(tempDirEnv) != "",
		EnvInvalidCode: string(invalid),
		DefaultPath:    defaultTempRoot(s.dataDir),
		RetiredPaths:   retired,
		ActiveRuns:     activeTempOwners(),
	}
	seen := map[string]bool{}
	for _, r := range append([]string{root}, retired...) {
		if seen[r] {
			continue
		}
		seen[r] = true
		n, b := scanTempRoot(r)
		st.Entries += n
		st.SizeBytes += b
	}
	return st, nil
}

// SaveTempRoot validates and stores raw as the temp location ("" resets to the default), and retires the previous
// root in the same transaction. It returns the old and new resolved roots for the audit entry.
func (s *Service) SaveTempRoot(ctx context.Context, raw string) (oldRoot, newRoot string, err error) {
	if s.getenv(tempDirEnv) != "" {
		return "", "", errTempEnvOverride
	}
	newRoot = defaultTempRoot(s.dataDir)
	if raw != "" {
		if newRoot, err = validateTempRoot(raw, s.dataDir); err != nil {
			return "", "", err
		}
	}
	oldRoot, _, _ = resolveTempRoot(ctx, s.db, s.dataDir, s.getenv)

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return "", "", fmt.Errorf("beginning backup temp save: %w", err)
	}
	defer func() {
		if rbErr := tx.Rollback(); rbErr != nil && !errors.Is(rbErr, sql.ErrTxDone) {
			slog.Debug("rolling back backup temp save", "error", rbErr)
		}
	}()

	if raw == "" {
		if _, err = tx.ExecContext(ctx, `DELETE FROM settings WHERE key = ?`, tempDirSetting); err != nil {
			return "", "", fmt.Errorf("clearing %s: %w", tempDirSetting, err)
		}
	} else if err = writeTempSetting(ctx, tx, tempDirSetting, newRoot); err != nil {
		return "", "", err
	}

	retired, err := retiredTempRoots(ctx, tx)
	if err != nil {
		return "", "", err
	}
	next := make([]string, 0, tempRetiredMax)
	if oldRoot != newRoot {
		next = append(next, oldRoot)
	}
	for _, r := range retired {
		if r != newRoot && r != oldRoot {
			next = append(next, r)
		}
	}
	if len(next) > tempRetiredMax {
		next = next[:tempRetiredMax]
	}
	encoded, err := json.Marshal(next)
	if err != nil {
		return "", "", fmt.Errorf("encoding retired backup temp folders: %w", err)
	}
	if err = writeTempSetting(ctx, tx, tempRetiredSetting, string(encoded)); err != nil {
		return "", "", err
	}
	if err = tx.Commit(); err != nil {
		return "", "", fmt.Errorf("committing backup temp save: %w", err)
	}
	return oldRoot, newRoot, nil
}

func writeTempSetting(ctx context.Context, tx *sql.Tx, key, value string) error {
	now := time.Now().UTC().Format(time.RFC3339)
	_, err := tx.ExecContext(ctx,
		`INSERT INTO settings (id, category, key, value, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?)
		 ON CONFLICT(key) DO UPDATE SET value = excluded.value, updated_at = excluded.updated_at, category = excluded.category`,
		xid.New().String(), tempSettingsCategory, key, value, now, now)
	if err != nil {
		return fmt.Errorf("writing setting %s: %w", key, err)
	}
	return nil
}
