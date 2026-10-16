// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package debugshell

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/containerd/errdefs"
	"github.com/moby/moby/client"
	"github.com/rs/xid"

	"github.com/therealmcsparrow/mcharbor/core/audit"
	coredocker "github.com/therealmcsparrow/mcharbor/core/docker"
)

// Auditor records a row with no request behind it; the cleanup triggers that run
// from the reaper still have to be attributable to the user who installed.
type Auditor interface {
	Record(userID, username string, e audit.Entry) error
}

// Service owns container_debug_sessions, the uploads and the removals. One
// instance per process, because the live-socket registry must be shared.
type Service struct {
	db      *sql.DB
	clients func(ctx context.Context, envID string) (dockerAPI, error)
	binDir  string
	logger  *slog.Logger
	auditor Auditor
	now     func() time.Time

	mu   sync.Mutex
	live map[string]context.CancelFunc

	removing sync.Mutex

	stopOnce sync.Once
	cancel   context.CancelFunc
	wg       sync.WaitGroup
}

// NewService returns a Service over pool. auditor may be nil in tests; the bundled
// binaries are looked up in ResolveBinDir.
func NewService(db *sql.DB, pool *coredocker.ClientPool, auditor Auditor, logger *slog.Logger) *Service {
	clients := func(context.Context, string) (dockerAPI, error) { return nil, errors.New("debugshell: no docker pool") }
	if pool != nil {
		clients = func(ctx context.Context, envID string) (dockerAPI, error) {
			cli, err := pool.GetContext(ctx, envID)
			if err != nil {
				return nil, err
			}
			return cli, nil
		}
	}
	return newService(db, clients, ResolveBinDir(), auditor, logger)
}

func newService(db *sql.DB, clients func(context.Context, string) (dockerAPI, error), binDir string, auditor Auditor, logger *slog.Logger) *Service {
	if logger == nil {
		logger = slog.Default()
	}
	return &Service{
		db:      db,
		clients: clients,
		binDir:  binDir,
		logger:  logger,
		auditor: auditor,
		now:     func() time.Time { return time.Now().UTC() },
		live:    map[string]context.CancelFunc{},
	}
}

// ResolveBinDir finds the bundled busybox binaries: MCHARBOR_DEBUG_BIN_DIR, else
// debug/ beside the executable (the image's /app and a tarball's /opt/mcharbor).
func ResolveBinDir() string {
	if dir := strings.TrimSpace(os.Getenv("MCHARBOR_DEBUG_BIN_DIR")); dir != "" {
		return dir
	}
	if exe, err := os.Executable(); err == nil {
		dir := filepath.Join(filepath.Dir(exe), "debug")
		if st, statErr := os.Stat(dir); statErr == nil && st.IsDir() {
			return dir
		}
	}
	return "/app/debug"
}

// Install uploads busybox into a running, shell-less, writable container and opens
// a session on it, or reuses a live install on the same container without uploading.
func (s *Service) Install(ctx context.Context, req InstallRequest) (*Session, error) {
	cli, err := s.clients(ctx, req.EnvironmentID)
	if err != nil {
		return nil, fmt.Errorf("docker client for %q: %w", req.EnvironmentID, err)
	}
	inspectCtx, cancel := context.WithTimeout(ctx, dockerCallTimeout)
	infoRes, err := cli.ContainerInspect(inspectCtx, req.ContainerID, client.ContainerInspectOptions{})
	info := infoRes.Container
	cancel()
	if err != nil {
		return nil, fmt.Errorf("inspecting %s: %w", req.ContainerID, err)
	}
	name := strings.TrimPrefix(info.Name, "/")
	if coredocker.ClassifyContainer(info) != coredocker.SelfTargetNone {
		return nil, ErrProtected
	}
	if info.State == nil || !info.State.Running {
		return nil, ErrNotRunning
	}
	if info.HostConfig != nil && info.HostConfig.ReadonlyRootfs {
		return nil, ErrReadonlyRootfs
	}

	now := s.now()
	sess := &Session{
		ID:            xid.New().String(),
		EnvironmentID: req.EnvironmentID,
		ContainerID:   info.ID,
		ContainerName: name,
		UserID:        req.UserID,
		APIKeyID:      req.APIKeyID,
		Path:          InstallPath,
		ExpiresAt:     now.Add(SessionTTL),
		CreatedAt:     now,
	}

	existing, err := s.liveInstall(ctx, req.EnvironmentID, info.ID, "")
	if err != nil {
		return nil, err
	}
	if existing != nil {
		sess.Arch, sess.BusyboxDigest, sess.SizeBytes = existing.Arch, existing.BusyboxDigest, existing.SizeBytes
	} else {
		shell, err := hasShell(ctx, cli, info.ID)
		if err != nil {
			return nil, fmt.Errorf("probing %s for a shell: %w", info.ID, err)
		}
		if shell {
			return nil, ErrHasShell
		}
		infoCtx, infoCancel := context.WithTimeout(ctx, dockerCallTimeout)
		daemonRes, err := cli.Info(infoCtx, client.InfoOptions{})
		daemon := daemonRes.Info
		infoCancel()
		if err != nil {
			return nil, fmt.Errorf("reading daemon info for %q: %w", req.EnvironmentID, err)
		}
		arch, ok := archFor(daemon)
		if !ok {
			return nil, fmt.Errorf("%w: %s/%s", ErrUnsupportedArch, daemon.OSType, daemon.Architecture)
		}
		binary, err := os.ReadFile(filepath.Join(s.binDir, "busybox-"+arch))
		if err != nil {
			return nil, fmt.Errorf("%w: %w", ErrBinaryUnavailable, err)
		}
		archive, err := archiveFor(binary)
		if err != nil {
			return nil, err
		}
		copyCtx, copyCancel := context.WithTimeout(ctx, dockerCallTimeout)
		_, err = cli.CopyToContainer(copyCtx, info.ID, client.CopyToContainerOptions{DestinationPath: "/", Content: archive})
		copyCancel()
		if err != nil {
			return nil, fmt.Errorf("uploading busybox into %s: %w", info.ID, err)
		}
		sum := sha256.Sum256(binary)
		sess.Arch, sess.BusyboxDigest, sess.SizeBytes, sess.Uploaded = arch, "sha256:"+hex.EncodeToString(sum[:]), int64(len(binary)), true
	}

	if err := s.insert(ctx, sess); err != nil {
		return nil, err
	}
	return sess, nil
}

// InstallAuditEntry is the debug_shell_install row: the digest of the uploaded
// bytes answers "what was put into that container" after the image is gone.
func InstallAuditEntry(sess *Session) audit.Entry {
	meta, err := json.Marshal(map[string]any{
		"environmentId": sess.EnvironmentID,
		"path":          sess.Path,
		"arch":          sess.Arch,
		"sizeBytes":     sess.SizeBytes,
		"busyboxDigest": sess.BusyboxDigest,
		"sessionId":     sess.ID,
		"expiresAt":     sess.ExpiresAt.Format(time.RFC3339),
		"uploaded":      sess.Uploaded,
	})
	if err != nil {
		meta = []byte(fmt.Sprintf(`{"sessionId":%q}`, sess.ID))
	}
	return audit.Entry{
		Action:        "debug_shell_install",
		Severity:      "warning",
		EntityType:    "container",
		EntityID:      sess.ContainerID,
		EntityName:    sess.ContainerName,
		Details:       string(meta),
		EnvironmentID: sess.EnvironmentID,
	}
}

// Validate returns the session a shell may attach to: present, unexpired, not
// removed, not already attached, owned by userID, and for the named container.
func (s *Service) Validate(ctx context.Context, sessionID, userID, envID, containerRef string) (*Session, error) {
	sess, err := s.get(ctx, sessionID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrSessionInvalid
		}
		return nil, err
	}
	switch {
	case sess.RemovedAt != nil,
		!sess.ExpiresAt.After(s.now()),
		sess.EnvironmentID != envID,
		userID != "system" && sess.UserID != userID,
		!matchesContainer(sess, containerRef),
		s.isLive(sessionID):
		return nil, ErrSessionInvalid
	}
	return sess, nil
}

// Track registers the cancel of an attached shell, so expiry can close its socket.
// It returns false when a shell is already attached: one session, one socket.
func (s *Service) Track(sessionID string, cancel context.CancelFunc) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, taken := s.live[sessionID]; taken {
		return false
	}
	s.live[sessionID] = cancel
	return true
}

func (s *Service) untrack(sessionID string) context.CancelFunc {
	s.mu.Lock()
	defer s.mu.Unlock()
	cancel := s.live[sessionID]
	delete(s.live, sessionID)
	return cancel
}

func (s *Service) isLive(sessionID string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, ok := s.live[sessionID]
	return ok
}

// End is trigger one: the shell's socket closed, for any reason. It keeps parent's
// values but drops its cancellation, because the request that carried the socket is already gone.
func (s *Service) End(parent context.Context, sessionID string, trigger Trigger) error {
	s.untrack(sessionID)
	ctx, cancel := context.WithTimeout(context.WithoutCancel(parent), 2*dockerCallTimeout)
	defer cancel()
	return s.remove(ctx, sessionID, trigger)
}

// Status lists the container's open sessions, and reports leftBehind when any of
// them is past expiry or failed its cleanup, so a stray busybox is visible here.
func (s *Service) Status(ctx context.Context, envID, containerRef string) (*SessionStatus, error) {
	open, err := s.openFor(ctx, envID, containerRef)
	if err != nil {
		return nil, err
	}
	now := s.now()
	out := &SessionStatus{Sessions: open}
	for _, sess := range open {
		if sess.LastError != "" || !sess.ExpiresAt.After(now) {
			out.LeftBehind = true
		}
	}
	return out, nil
}

// Close ends the container's open sessions, or only sessionID when it is set,
// closing any attached shell first. It returns how many it removed.
func (s *Service) Close(ctx context.Context, envID, containerRef, sessionID string) (int, error) {
	open, err := s.openFor(ctx, envID, containerRef)
	if err != nil {
		return 0, err
	}
	removed := 0
	var firstErr error
	for _, sess := range open {
		if sessionID != "" && sess.ID != sessionID {
			continue
		}
		if cancel := s.untrack(sess.ID); cancel != nil {
			cancel()
		}
		if err := s.remove(ctx, sess.ID, TriggerSessionEnd); err != nil {
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		removed++
	}
	return removed, firstErr
}

// Reap removes every session past its expiry that is still open, closing its
// shell first. It is the reaper's minute tick, and the startup sweep.
func (s *Service) Reap(ctx context.Context, trigger Trigger) (int, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT `+sessionColumns+` FROM container_debug_sessions
		WHERE removed_at IS NULL AND expires_at <= ? ORDER BY expires_at LIMIT 1000`, formatTime(s.now()))
	if err != nil {
		return 0, fmt.Errorf("listing expired debug sessions: %w", err)
	}
	expired, err := scanSessions(rows)
	if err != nil {
		return 0, err
	}
	removed := 0
	var firstErr error
	for _, sess := range expired {
		t := trigger
		if cancel := s.untrack(sess.ID); cancel != nil {
			cancel()
			t = TriggerExpiry
		}
		if err := s.remove(ctx, sess.ID, t); err != nil {
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		removed++
	}
	return removed, firstErr
}

// remove deletes the install unless another live session still uses it. It is
// idempotent and never fatal: an already-removed row is a no-op, and a failed
// removal keeps the row open with last_error so the reaper retries it.
func (s *Service) remove(ctx context.Context, sessionID string, trigger Trigger) error {
	s.removing.Lock()
	defer s.removing.Unlock()

	sess, err := s.get(ctx, sessionID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil
		}
		return err
	}
	if sess.RemovedAt != nil {
		return nil
	}

	others, err := s.liveInstall(ctx, sess.EnvironmentID, sess.ContainerID, sess.ID)
	if err != nil {
		return err
	}
	if others != nil {
		return s.finish(ctx, sess, trigger, "retained")
	}

	cli, err := s.clients(ctx, sess.EnvironmentID)
	if err != nil {
		return s.fail(ctx, sess, trigger, fmt.Errorf("docker client: %w", err))
	}
	inspectCtx, cancel := context.WithTimeout(ctx, dockerCallTimeout)
	infoRes, err := cli.ContainerInspect(inspectCtx, sess.ContainerID, client.ContainerInspectOptions{})
	info := infoRes.Container
	cancel()
	if err != nil {
		if errdefs.IsNotFound(err) {
			return s.finish(ctx, sess, trigger, "container_gone")
		}
		return s.fail(ctx, sess, trigger, fmt.Errorf("inspecting container: %w", err))
	}
	if info.State == nil || !info.State.Running {
		return s.finish(ctx, sess, trigger, "container_not_running")
	}
	code, err := runDetached(ctx, cli, sess.ContainerID, "0", []string{sess.Path, "rm", "-rf", InstallDir})
	if err != nil {
		return s.fail(ctx, sess, trigger, err)
	}
	if code != 0 {
		return s.fail(ctx, sess, trigger, fmt.Errorf("rm -rf %s exited %d", InstallDir, code))
	}
	return s.finish(ctx, sess, trigger, "removed")
}

func (s *Service) finish(ctx context.Context, sess *Session, trigger Trigger, outcome string) error {
	now := formatTime(s.now())
	if _, err := s.db.ExecContext(ctx, `UPDATE container_debug_sessions
		SET removed_at = ?, removal_trigger = ?, last_error = '', updated_at = ? WHERE id = ? AND removed_at IS NULL`,
		now, string(trigger), now, sess.ID); err != nil {
		return fmt.Errorf("marking debug session %s removed: %w", sess.ID, err)
	}
	s.auditRemove(ctx, sess, trigger, true, outcome, "")
	return nil
}

func (s *Service) fail(ctx context.Context, sess *Session, trigger Trigger, cause error) error {
	s.logger.Error("debug shell: removal failed", "session", sess.ID, "container", sess.ContainerID, "env", sess.EnvironmentID, "trigger", trigger, "error", cause)
	now := formatTime(s.now())
	if _, err := s.db.ExecContext(ctx, `UPDATE container_debug_sessions
		SET last_error = ?, removal_trigger = ?, updated_at = ? WHERE id = ? AND removed_at IS NULL`,
		cause.Error(), string(trigger), now, sess.ID); err != nil {
		return fmt.Errorf("recording failed removal of %s: %w (after %w)", sess.ID, err, cause)
	}
	s.auditRemove(ctx, sess, trigger, false, "failed", cause.Error())
	return fmt.Errorf("removing debug shell %s: %w", sess.ID, cause)
}

func (s *Service) auditRemove(ctx context.Context, sess *Session, trigger Trigger, succeeded bool, outcome, errText string) {
	if s.auditor == nil {
		return
	}
	meta, err := json.Marshal(map[string]any{
		"sessionId": sess.ID,
		"trigger":   string(trigger),
		"succeeded": succeeded,
		"outcome":   outcome,
		"error":     errText,
	})
	if err != nil {
		meta = []byte(fmt.Sprintf(`{"sessionId":%q}`, sess.ID))
	}
	severity := "info"
	if !succeeded {
		severity = "error"
	}
	entry := audit.Entry{
		Action:        "debug_shell_remove",
		Severity:      severity,
		EntityType:    "container",
		EntityID:      sess.ContainerID,
		EntityName:    sess.ContainerName,
		Details:       string(meta),
		EnvironmentID: sess.EnvironmentID,
	}
	if err := s.auditor.Record(sess.UserID, s.username(ctx, sess.UserID), entry); err != nil {
		s.logger.Error("debug shell: audit write failed", "session", sess.ID, "error", err)
	}
}

func (s *Service) username(ctx context.Context, userID string) string {
	if userID == "" {
		return ""
	}
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	var name string
	if err := s.db.QueryRowContext(ctx, `SELECT username FROM users WHERE id = ? LIMIT 1`, userID).Scan(&name); err != nil {
		return ""
	}
	return name
}

const sessionColumns = `id, environment_id, container_id, container_name, user_id, api_key_id, path, arch,
	busybox_digest, size_bytes, uploaded, expires_at, removed_at, removal_trigger, last_error, created_at`

func (s *Service) insert(ctx context.Context, sess *Session) error {
	uploaded := 0
	if sess.Uploaded {
		uploaded = 1
	}
	now := formatTime(sess.CreatedAt)
	_, err := s.db.ExecContext(ctx, `INSERT INTO container_debug_sessions
		(id, environment_id, container_id, container_name, user_id, api_key_id, path, arch, busybox_digest,
		 size_bytes, uploaded, expires_at, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		sess.ID, sess.EnvironmentID, sess.ContainerID, sess.ContainerName, sess.UserID, sess.APIKeyID,
		sess.Path, sess.Arch, sess.BusyboxDigest, sess.SizeBytes, uploaded, formatTime(sess.ExpiresAt), now, now)
	if err != nil {
		return fmt.Errorf("recording debug session %s: %w", sess.ID, err)
	}
	return nil
}

func (s *Service) get(ctx context.Context, id string) (*Session, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT `+sessionColumns+` FROM container_debug_sessions WHERE id = ? LIMIT 1`, id)
	if err != nil {
		return nil, fmt.Errorf("reading debug session %s: %w", id, err)
	}
	list, err := scanSessions(rows)
	if err != nil {
		return nil, err
	}
	if len(list) == 0 {
		return nil, sql.ErrNoRows
	}
	return &list[0], nil
}

// liveInstall returns an unexpired, unremoved session on the container other than
// exclude, whose upload a new session may share and whose removal must wait.
func (s *Service) liveInstall(ctx context.Context, envID, containerID, exclude string) (*Session, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT `+sessionColumns+` FROM container_debug_sessions
		WHERE environment_id = ? AND container_id = ? AND removed_at IS NULL AND expires_at > ? AND id <> ?
		ORDER BY created_at LIMIT 1`, envID, containerID, formatTime(s.now()), exclude)
	if err != nil {
		return nil, fmt.Errorf("looking up live debug installs: %w", err)
	}
	list, err := scanSessions(rows)
	if err != nil {
		return nil, err
	}
	if len(list) == 0 {
		return nil, nil
	}
	return &list[0], nil
}

func (s *Service) openFor(ctx context.Context, envID, containerRef string) ([]Session, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT `+sessionColumns+` FROM container_debug_sessions
		WHERE environment_id = ? AND removed_at IS NULL ORDER BY created_at LIMIT 1000`, envID)
	if err != nil {
		return nil, fmt.Errorf("listing debug sessions: %w", err)
	}
	all, err := scanSessions(rows)
	if err != nil {
		return nil, err
	}
	out := make([]Session, 0, len(all))
	for _, sess := range all {
		if matchesContainer(&sess, containerRef) {
			out = append(out, sess)
		}
	}
	return out, nil
}

func scanSessions(rows *sql.Rows) ([]Session, error) {
	defer func() {
		if err := rows.Close(); err != nil {
			slog.Warn("debug shell: closing session rows failed", "error", err)
		}
	}()
	var out []Session
	for rows.Next() {
		var sess Session
		var uploaded int
		var expires, created string
		var removed sql.NullString
		if err := rows.Scan(&sess.ID, &sess.EnvironmentID, &sess.ContainerID, &sess.ContainerName, &sess.UserID,
			&sess.APIKeyID, &sess.Path, &sess.Arch, &sess.BusyboxDigest, &sess.SizeBytes, &uploaded, &expires,
			&removed, &sess.RemovalTrigger, &sess.LastError, &created); err != nil {
			return nil, fmt.Errorf("scanning debug session: %w", err)
		}
		sess.Uploaded = uploaded != 0
		sess.ExpiresAt = parseTime(expires)
		sess.CreatedAt = parseTime(created)
		if removed.Valid && removed.String != "" {
			t := parseTime(removed.String)
			sess.RemovedAt = &t
		}
		out = append(out, sess)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("reading debug sessions: %w", err)
	}
	return out, nil
}

// matchesContainer accepts the forms an operator types: the full id, a short id
// prefix of at least four characters, or the name with or without its slash.
func matchesContainer(sess *Session, ref string) bool {
	ref = strings.TrimPrefix(strings.TrimSpace(ref), "/")
	if ref == "" {
		return false
	}
	if ref == sess.ContainerID || ref == sess.ContainerName {
		return true
	}
	return len(ref) >= 4 && strings.HasPrefix(sess.ContainerID, ref)
}

func formatTime(t time.Time) string { return t.UTC().Format(time.RFC3339Nano) }

func parseTime(v string) time.Time {
	t, err := time.Parse(time.RFC3339Nano, v)
	if err != nil {
		return time.Time{}
	}
	return t
}
