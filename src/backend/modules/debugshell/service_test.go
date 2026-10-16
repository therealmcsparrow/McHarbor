// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package debugshell

import (
	"archive/tar"
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/api/types/system"
	"github.com/moby/moby/client"
	_ "modernc.org/sqlite"

	"github.com/therealmcsparrow/mcharbor/core/audit"
	coredb "github.com/therealmcsparrow/mcharbor/core/db"
)

const testContainerID = "c0ffee0123456789abcdef0123456789abcdef0123456789abcdef0123456789"

func openDB(t *testing.T) *sql.DB {
	t.Helper()
	database, err := sql.Open("sqlite", "file:"+filepath.Join(t.TempDir(), "debugshell.db")+"?_pragma=foreign_keys(1)")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := database.Close(); err != nil {
			t.Errorf("close sqlite: %v", err)
		}
	})
	if err := coredb.Migrate(database, coredb.DriverSQLite); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return database
}

type execCall struct {
	cmd    []string
	user   string
	detach bool
}

// fakeDocker is a daemon holding one container. It records every exec and every
// upload so a test can assert on the absence of a call, not only on a refusal.
type fakeDocker struct {
	mu         sync.Mutex
	running    bool
	readonly   bool
	shell      bool
	gone       bool
	arch       string
	rmExit     int
	rmErr      error
	uploads    [][]byte
	uploadDst  []string
	execs      []execCall
	pending    map[string]execCall
	infoCalls  int
	execSerial int
}

func newFakeDocker() *fakeDocker {
	return &fakeDocker{running: true, arch: "x86_64", pending: map[string]execCall{}}
}

func (f *fakeDocker) ContainerInspect(context.Context, string, client.ContainerInspectOptions) (client.ContainerInspectResult, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.gone {
		return client.ContainerInspectResult{}, notFound{}
	}
	return client.ContainerInspectResult{Container: container.InspectResponse{
		ID:         testContainerID,
		Name:       "/svc",
		State:      &container.State{Running: f.running},
		HostConfig: &container.HostConfig{ReadonlyRootfs: f.readonly},
		Config:     &container.Config{Image: "gcr.io/distroless/static:nonroot"},
	}}, nil
}

func (f *fakeDocker) Info(context.Context, client.InfoOptions) (client.SystemInfoResult, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.infoCalls++
	return client.SystemInfoResult{Info: system.Info{OSType: "linux", Architecture: f.arch}}, nil
}

func (f *fakeDocker) ExecCreate(_ context.Context, _ string, o client.ExecCreateOptions) (client.ExecCreateResult, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.execSerial++
	id := fmt.Sprintf("exec-%d", f.execSerial)
	f.pending[id] = execCall{cmd: o.Cmd, user: o.User}
	return client.ExecCreateResult{ID: id}, nil
}

func (f *fakeDocker) ExecStart(_ context.Context, id string, o client.ExecStartOptions) (client.ExecStartResult, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	call := f.pending[id]
	call.detach = o.Detach
	f.execs = append(f.execs, call)
	f.pending[id] = call
	if len(call.cmd) > 0 && call.cmd[0] == "/bin/sh" && !f.shell {
		return client.ExecStartResult{}, errors.New(`OCI runtime exec failed: exec failed: unable to start container process: exec: "/bin/sh": stat /bin/sh: no such file or directory`)
	}
	if len(call.cmd) > 1 && call.cmd[1] == "rm" && f.rmErr != nil {
		return client.ExecStartResult{}, f.rmErr
	}
	return client.ExecStartResult{}, nil
}

func (f *fakeDocker) ExecInspect(_ context.Context, id string, _ client.ExecInspectOptions) (client.ExecInspectResult, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	call := f.pending[id]
	exit := 0
	if len(call.cmd) > 1 && call.cmd[1] == "rm" {
		exit = f.rmExit
	}
	return client.ExecInspectResult{ID: id, Running: false, ExitCode: exit}, nil
}

func (f *fakeDocker) CopyToContainer(_ context.Context, _ string, o client.CopyToContainerOptions) (client.CopyToContainerResult, error) {
	body, err := io.ReadAll(o.Content)
	if err != nil {
		return client.CopyToContainerResult{}, err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.uploads = append(f.uploads, body)
	f.uploadDst = append(f.uploadDst, o.DestinationPath)
	return client.CopyToContainerResult{}, nil
}

func (f *fakeDocker) rmCalls() []execCall {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []execCall
	for _, e := range f.execs {
		if len(e.cmd) > 1 && e.cmd[1] == "rm" {
			out = append(out, e)
		}
	}
	return out
}

type notFound struct{}

func (notFound) Error() string { return "No such container: svc" }
func (notFound) NotFound()     {}

type recordingAuditor struct {
	mu      sync.Mutex
	entries []audit.Entry
	users   []string
}

func (a *recordingAuditor) Record(userID, _ string, e audit.Entry) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.entries = append(a.entries, e)
	a.users = append(a.users, userID)
	return nil
}

func (a *recordingAuditor) removals() []map[string]any {
	a.mu.Lock()
	defer a.mu.Unlock()
	var out []map[string]any
	for _, e := range a.entries {
		if e.Action != "debug_shell_remove" {
			continue
		}
		var m map[string]any
		if err := json.Unmarshal([]byte(e.Details), &m); err == nil {
			out = append(out, m)
		}
	}
	return out
}

type harness struct {
	svc     *Service
	docker  *fakeDocker
	auditor *recordingAuditor
	clock   *time.Time
	binary  []byte
	db      *sql.DB
}

func newHarness(t *testing.T) *harness {
	t.Helper()
	dir := t.TempDir()
	binary := bytes.Repeat([]byte("busybox-amd64!"), 80_000)
	if err := os.WriteFile(filepath.Join(dir, "busybox-amd64"), binary, 0o755); err != nil {
		t.Fatal(err)
	}
	database := openDB(t)
	docker := newFakeDocker()
	auditor := &recordingAuditor{}
	svc := newService(database, func(context.Context, string) (dockerAPI, error) { return docker, nil }, dir, auditor, nil)
	clock := time.Date(2026, 9, 27, 13, 0, 0, 0, time.UTC)
	svc.now = func() time.Time { return clock }
	execPollInterval = time.Millisecond
	return &harness{svc: svc, docker: docker, auditor: auditor, clock: &clock, binary: binary, db: database}
}

func (h *harness) advance(d time.Duration) { *h.clock = h.clock.Add(d) }

func (h *harness) install(t *testing.T) *Session {
	t.Helper()
	sess, err := h.svc.Install(context.Background(), InstallRequest{EnvironmentID: "env1", ContainerID: "svc", UserID: "u1"})
	if err != nil {
		t.Fatalf("install: %v", err)
	}
	return sess
}

func TestInstallUploadsOneExecutableIntoARunningShelllessContainer(t *testing.T) {
	h := newHarness(t)
	sess := h.install(t)

	if len(h.docker.uploads) != 1 || h.docker.uploadDst[0] != "/" {
		t.Fatalf("want exactly one upload extracted at /, got %d at %v", len(h.docker.uploads), h.docker.uploadDst)
	}
	tr := tar.NewReader(bytes.NewReader(h.docker.uploads[0]))
	var names []string
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		names = append(names, hdr.Name)
		if hdr.Mode != 0o755 || hdr.Uid != 0 || hdr.Gid != 0 {
			t.Errorf("%s: mode %o uid %d gid %d, want 0755 0:0", hdr.Name, hdr.Mode, hdr.Uid, hdr.Gid)
		}
		if hdr.Typeflag == tar.TypeReg {
			body, err := io.ReadAll(tr)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(body, h.binary) {
				t.Errorf("uploaded bytes differ from the bundled binary")
			}
		}
	}
	if strings.Join(names, ",") != ".mcharbor-debug/,.mcharbor-debug/busybox" {
		t.Errorf("archive holds %v; the container must gain exactly one directory and one file", names)
	}

	sum := sha256.Sum256(h.binary)
	if sess.BusyboxDigest != "sha256:"+hex.EncodeToString(sum[:]) || sess.SizeBytes != int64(len(h.binary)) {
		t.Errorf("digest %s size %d do not describe the uploaded bytes", sess.BusyboxDigest, sess.SizeBytes)
	}
	if sess.Arch != "amd64" || sess.Path != InstallPath || !sess.Uploaded || sess.ContainerID != testContainerID {
		t.Errorf("session %+v", sess)
	}
	if !sess.ExpiresAt.Equal(h.clock.Add(SessionTTL)) {
		t.Errorf("expires %v, want 15 minutes out", sess.ExpiresAt)
	}

	entry := InstallAuditEntry(sess)
	var meta map[string]any
	if err := json.Unmarshal([]byte(entry.Details), &meta); err != nil {
		t.Fatal(err)
	}
	if entry.Action != "debug_shell_install" || entry.EntityID != testContainerID || meta["busyboxDigest"] != sess.BusyboxDigest || meta["sessionId"] != sess.ID || meta["arch"] != "amd64" {
		t.Errorf("install audit entry %+v / %v does not carry the digest and session", entry, meta)
	}
}

func TestInstallRefusals(t *testing.T) {
	cases := []struct {
		name  string
		setup func(*fakeDocker)
		want  error
	}{
		{"a container that has a shell", func(f *fakeDocker) { f.shell = true }, ErrHasShell},
		{"a read-only root filesystem", func(f *fakeDocker) { f.readonly = true }, ErrReadonlyRootfs},
		{"a stopped container", func(f *fakeDocker) { f.running = false }, ErrNotRunning},
		{"an unsupported architecture", func(f *fakeDocker) { f.arch = "s390x" }, ErrUnsupportedArch},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			h := newHarness(t)
			c.setup(h.docker)
			_, err := h.svc.Install(context.Background(), InstallRequest{EnvironmentID: "env1", ContainerID: "svc", UserID: "u1"})
			if !errors.Is(err, c.want) {
				t.Fatalf("got %v, want %v", err, c.want)
			}
			if len(h.docker.uploads) != 0 {
				t.Fatalf("a refused install must upload nothing")
			}
			var n int
			if err := h.db.QueryRow(`SELECT COUNT(*) FROM container_debug_sessions`).Scan(&n); err != nil || n != 0 {
				t.Fatalf("a refused install must record no session (n=%d err=%v)", n, err)
			}
		})
	}
}

func TestInstallWithoutTheBundledBinaryIsItsOwnRefusal(t *testing.T) {
	h := newHarness(t)
	h.svc.binDir = t.TempDir()
	_, err := h.svc.Install(context.Background(), InstallRequest{EnvironmentID: "env1", ContainerID: "svc", UserID: "u1"})
	if !errors.Is(err, ErrBinaryUnavailable) {
		t.Fatalf("got %v, want ErrBinaryUnavailable", err)
	}
}

// TestProbeNeverUsesPostHijacked: non-detached exec is postHijacked, which bypasses
// the agent transport. Every exec this package starts must be detached, and the
// Docker surface it is given must not even offer attach.
func TestProbeNeverUsesPostHijacked(t *testing.T) {
	h := newHarness(t)
	sess := h.install(t)
	if err := h.svc.End(t.Context(), sess.ID, TriggerSessionEnd); err != nil {
		t.Fatal(err)
	}
	if len(h.docker.execs) < 2 {
		t.Fatalf("expected the probe and the removal to exec, got %d", len(h.docker.execs))
	}
	for _, e := range h.docker.execs {
		if !e.detach {
			t.Errorf("exec %v was started attached; attach is postHijacked and bypasses the agent", e.cmd)
		}
	}
	api := reflect.TypeOf((*dockerAPI)(nil)).Elem()
	for i := 0; i < api.NumMethod(); i++ {
		if name := api.Method(i).Name; strings.Contains(name, "Attach") {
			t.Errorf("dockerAPI exposes %s; the debug shell must not be able to reach postHijacked", name)
		}
	}
}

func TestCleanupRunsOnEachOfTheFourTriggers(t *testing.T) {
	t.Run("session_end", func(t *testing.T) {
		h := newHarness(t)
		sess := h.install(t)
		if err := h.svc.End(t.Context(), sess.ID, TriggerSessionEnd); err != nil {
			t.Fatal(err)
		}
		assertRemoved(t, h, sess.ID, TriggerSessionEnd)
	})
	t.Run("expiry closes the attached shell first", func(t *testing.T) {
		h := newHarness(t)
		sess := h.install(t)
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		if !h.svc.Track(sess.ID, cancel) {
			t.Fatal("track refused a fresh session")
		}
		h.advance(SessionTTL + time.Second)
		if _, err := h.svc.Reap(context.Background(), TriggerReaper); err != nil {
			t.Fatal(err)
		}
		if ctx.Err() == nil {
			t.Fatal("expiry must close the attached socket")
		}
		assertRemoved(t, h, sess.ID, TriggerExpiry)
	})
	t.Run("reaper", func(t *testing.T) {
		h := newHarness(t)
		sess := h.install(t)
		h.advance(SessionTTL + time.Second)
		if _, err := h.svc.Reap(context.Background(), TriggerReaper); err != nil {
			t.Fatal(err)
		}
		assertRemoved(t, h, sess.ID, TriggerReaper)
	})
	t.Run("startup", func(t *testing.T) {
		h := newHarness(t)
		sess := h.install(t)
		h.advance(SessionTTL + time.Second)
		restarted := newService(h.db, func(context.Context, string) (dockerAPI, error) { return h.docker, nil }, h.svc.binDir, h.auditor, nil)
		restarted.now = h.svc.now
		if _, err := restarted.Reap(context.Background(), TriggerStartup); err != nil {
			t.Fatal(err)
		}
		assertRemoved(t, h, sess.ID, TriggerStartup)
	})
}

func assertRemoved(t *testing.T, h *harness, id string, trigger Trigger) {
	t.Helper()
	rms := h.docker.rmCalls()
	if len(rms) != 1 {
		t.Fatalf("want one removal exec, got %d", len(rms))
	}
	if strings.Join(rms[0].cmd, " ") != InstallPath+" rm -rf "+InstallDir || rms[0].user != "0" {
		t.Errorf("removal ran %v as %q", rms[0].cmd, rms[0].user)
	}
	sess, err := h.svc.get(context.Background(), id)
	if err != nil {
		t.Fatal(err)
	}
	if sess.RemovedAt == nil || sess.RemovalTrigger != string(trigger) || sess.LastError != "" {
		t.Errorf("row not closed by %s: removed=%v trigger=%q err=%q", trigger, sess.RemovedAt, sess.RemovalTrigger, sess.LastError)
	}
	removals := h.auditor.removals()
	if len(removals) != 1 || removals[0]["trigger"] != string(trigger) || removals[0]["succeeded"] != true {
		t.Errorf("debug_shell_remove audit rows: %v", removals)
	}
	if err := h.svc.End(t.Context(), id, TriggerSessionEnd); err != nil || len(h.docker.rmCalls()) != 1 {
		t.Errorf("a second removal must be a no-op (err=%v, rm calls=%d)", err, len(h.docker.rmCalls()))
	}
}

func TestAFailedCleanupIsVisibleAndRetried(t *testing.T) {
	h := newHarness(t)
	sess := h.install(t)
	h.docker.rmExit = 1
	if err := h.svc.End(t.Context(), sess.ID, TriggerSessionEnd); err == nil {
		t.Fatal("a failed removal must report itself")
	}
	status, err := h.svc.Status(context.Background(), "env1", "svc")
	if err != nil {
		t.Fatal(err)
	}
	if !status.LeftBehind || len(status.Sessions) != 1 || status.Sessions[0].LastError == "" {
		t.Fatalf("a failed cleanup must be visible in status: %+v", status)
	}
	removals := h.auditor.removals()
	if len(removals) != 1 || removals[0]["succeeded"] != false || removals[0]["error"] == "" {
		t.Fatalf("a failed cleanup must write a debug_shell_remove row with succeeded=false: %v", removals)
	}

	h.docker.rmExit = 0
	h.advance(SessionTTL + time.Second)
	if n, err := h.svc.Reap(context.Background(), TriggerReaper); err != nil || n != 1 {
		t.Fatalf("the reaper must retry a failed removal: n=%d err=%v", n, err)
	}
	status, err = h.svc.Status(context.Background(), "env1", "svc")
	if err != nil || status.LeftBehind || len(status.Sessions) != 0 {
		t.Fatalf("after a successful retry nothing is left behind: %+v %v", status, err)
	}
}

func TestAStoppedOrRemovedContainerClosesItsRowWithoutExec(t *testing.T) {
	for _, gone := range []bool{false, true} {
		h := newHarness(t)
		sess := h.install(t)
		h.docker.running = false
		h.docker.gone = gone
		if err := h.svc.End(t.Context(), sess.ID, TriggerSessionEnd); err != nil {
			t.Fatal(err)
		}
		if len(h.docker.rmCalls()) != 0 {
			t.Fatalf("gone=%v: nothing can be exec'd into a stopped container", gone)
		}
		row, err := h.svc.get(context.Background(), sess.ID)
		if err != nil || row.RemovedAt == nil {
			t.Fatalf("gone=%v: the row must be closed with a reason (%v)", gone, err)
		}
	}
}

func TestASecondSessionReusesTheInstallAndKeepsItUntilTheLastEnds(t *testing.T) {
	h := newHarness(t)
	first := h.install(t)
	second := h.install(t)
	if len(h.docker.uploads) != 1 || second.Uploaded || second.BusyboxDigest != first.BusyboxDigest {
		t.Fatalf("a live install must be reused, not uploaded again (uploads=%d)", len(h.docker.uploads))
	}
	if err := h.svc.End(t.Context(), first.ID, TriggerSessionEnd); err != nil {
		t.Fatal(err)
	}
	if len(h.docker.rmCalls()) != 0 {
		t.Fatal("the binary must stay while another session uses it")
	}
	if err := h.svc.End(t.Context(), second.ID, TriggerSessionEnd); err != nil {
		t.Fatal(err)
	}
	if len(h.docker.rmCalls()) != 1 {
		t.Fatal("the last session to end must remove the binary")
	}
}

func TestValidateAdmitsOnlyTheOwnerOfALiveUnattachedSession(t *testing.T) {
	h := newHarness(t)
	sess := h.install(t)
	ctx := context.Background()
	if _, err := h.svc.Validate(ctx, sess.ID, "u1", "env1", "svc"); err != nil {
		t.Fatalf("the owner must be admitted: %v", err)
	}
	if _, err := h.svc.Validate(ctx, sess.ID, "u1", "env1", testContainerID[:12]); err != nil {
		t.Fatalf("a short id must match: %v", err)
	}
	refused := []struct {
		name, user, env, ref string
	}{
		{"another user", "u2", "env1", "svc"},
		{"another environment", "u1", "env2", "svc"},
		{"another container", "u1", "env1", "db"},
	}
	for _, c := range refused {
		if _, err := h.svc.Validate(ctx, sess.ID, c.user, c.env, c.ref); !errors.Is(err, ErrSessionInvalid) {
			t.Errorf("%s: got %v", c.name, err)
		}
	}
	if _, err := h.svc.Validate(ctx, "nope", "u1", "env1", "svc"); !errors.Is(err, ErrSessionInvalid) {
		t.Errorf("unknown session: %v", err)
	}
	_, cancel := context.WithCancel(ctx)
	defer cancel()
	h.svc.Track(sess.ID, cancel)
	if _, err := h.svc.Validate(ctx, sess.ID, "u1", "env1", "svc"); !errors.Is(err, ErrSessionInvalid) {
		t.Errorf("an attached session must not admit a second socket: %v", err)
	}
	if h.svc.Track(sess.ID, cancel) {
		t.Error("Track must refuse a second socket on one session")
	}
	h.advance(SessionTTL)
	h.svc.untrack(sess.ID)
	if _, err := h.svc.Validate(ctx, sess.ID, "u1", "env1", "svc"); !errors.Is(err, ErrSessionInvalid) {
		t.Errorf("an expired session: %v", err)
	}
}
