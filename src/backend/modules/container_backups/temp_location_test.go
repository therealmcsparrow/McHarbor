// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package container_backups

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	_ "modernc.org/sqlite"

	coredb "github.com/therealmcsparrow/mcharbor/core/db"
)

func tempTestDB(t *testing.T) *sql.DB {
	t.Helper()
	database, err := sql.Open("sqlite", "file:"+filepath.Join(t.TempDir(), "temp.db")+"?_pragma=foreign_keys(1)")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := database.Close(); err != nil {
			t.Log(err)
		}
	})
	if err := coredb.Migrate(database, coredb.DriverSQLite); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return database
}

func saveTempSetting(t *testing.T, database *sql.DB, value string) {
	t.Helper()
	if _, err := database.Exec(`INSERT INTO settings (id, key, value, category) VALUES ('s-bt', 'backup_temp_dir', ?, 'backups')`, value); err != nil {
		t.Fatalf("seed setting: %v", err)
	}
}

func noEnv(string) string { return "" }

func envOf(value string) func(string) string {
	return func(key string) string {
		if key == "MCHARBOR_BACKUP_TEMP_DIR" {
			return value
		}
		return ""
	}
}

func skipOnWindows(t *testing.T, why string) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip(why)
	}
}

func TestResolveTempRootDefaultsUnderTheDataDir(t *testing.T) {
	dataDir := filepath.Join(t.TempDir(), "data")
	root, source, invalid := resolveTempRoot(context.Background(), tempTestDB(t), dataDir, noEnv)
	want := filepath.Join(dataDir, "backups", "tmp")
	if root != want || source != "default" || invalid != "" {
		t.Fatalf("resolveTempRoot = (%q, %q, %q), want (%q, default, empty)", root, source, invalid, want)
	}
}

func TestResolveTempRootEmptySettingIsTheDefault(t *testing.T) {
	dataDir := filepath.Join(t.TempDir(), "data")
	database := tempTestDB(t)
	saveTempSetting(t, database, "")
	root, source, _ := resolveTempRoot(context.Background(), database, dataDir, noEnv)
	if root != filepath.Join(dataDir, "backups", "tmp") || source != "default" {
		t.Fatalf("an empty saved value must read as unset, got (%q, %q)", root, source)
	}
}

func TestResolveTempRootSettingBeatsDefault(t *testing.T) {
	database := tempTestDB(t)
	saved := filepath.Join(t.TempDir(), "saved")
	saveTempSetting(t, database, saved)
	root, source, _ := resolveTempRoot(context.Background(), database, filepath.Join(t.TempDir(), "data"), noEnv)
	if root != saved || source != "setting" {
		t.Fatalf("resolveTempRoot = (%q, %q), want (%q, setting)", root, source, saved)
	}
}

func TestResolveTempRootEnvBeatsSetting(t *testing.T) {
	database := tempTestDB(t)
	saveTempSetting(t, database, filepath.Join(t.TempDir(), "saved"))
	fromEnv := filepath.Join(t.TempDir(), "fromenv")
	root, source, invalid := resolveTempRoot(context.Background(), database, filepath.Join(t.TempDir(), "data"), envOf(fromEnv))
	if root != fromEnv || source != "env" || invalid != "" {
		t.Fatalf("resolveTempRoot = (%q, %q, %q), want (%q, env, empty)", root, source, invalid, fromEnv)
	}
}

func TestResolveTempRootInvalidEnvFallsBackAndReportsTheCode(t *testing.T) {
	dataDir := filepath.Join(t.TempDir(), "data")
	database := tempTestDB(t)
	root, source, invalid := resolveTempRoot(context.Background(), database, dataDir, envOf("relative/dir"))
	if root != filepath.Join(dataDir, "backups", "tmp") || source != "default" {
		t.Fatalf("an invalid env value must fall back to the default root, got (%q, %q)", root, source)
	}
	if invalid == "" {
		t.Fatal("an invalid env value must be reported through envInvalid")
	}
}

func TestValidateTempRootRefusals(t *testing.T) {
	base := t.TempDir()
	dataDir := filepath.Join(base, "data")
	sep := string(os.PathSeparator)
	cases := []struct {
		name string
		raw  string
		want error
	}{
		{"empty", "", errTempNotAbsolute},
		{"relative", "relative" + sep + "dir", errTempNotAbsolute},
		{"dot relative", "." + sep + "tmp", errTempNotAbsolute},
		{"dotdot segment in raw input", base + sep + "a" + sep + ".." + sep + "b", errTempTraversal},
		{"trailing dotdot", base + sep + "a" + sep + "..", errTempTraversal},
		{"data dir itself", dataDir, errTempSensitive},
		{"local backup dir", LocalBackupDir(dataDir), errTempSensitive},
		{"parent of the local backup dir", filepath.Join(dataDir, "backups"), errTempSensitive},
		{"ancestor of the data dir", base, errTempSensitive},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := validateTempRoot(c.raw, dataDir)
			if !errors.Is(err, c.want) {
				t.Fatalf("validateTempRoot(%q) = (%q, %v), want %v", c.raw, got, err, c.want)
			}
		})
	}
}

func TestValidateTempRootRefusesSensitiveRootsAndTheirChildren(t *testing.T) {
	skipOnWindows(t, "the sensitive roots are POSIX paths")
	dataDir := filepath.Join(t.TempDir(), "data")
	roots := []string{"/etc", "/proc", "/sys", "/dev", "/boot", "/bin", "/sbin", "/lib", "/lib64", "/usr/bin", "/usr/sbin", "/usr/lib"}
	for _, root := range roots {
		for _, raw := range []string{root, root + "/mcharbor-tmp", root + "/a/b"} {
			if _, err := validateTempRoot(raw, dataDir); !errors.Is(err, errTempSensitive) {
				t.Errorf("validateTempRoot(%q) = %v, want errTempSensitive", raw, err)
			}
		}
	}
	if _, err := validateTempRoot("/", dataDir); !errors.Is(err, errTempSensitive) {
		t.Errorf("validateTempRoot(/) = %v, want errTempSensitive", err)
	}
}

func TestValidateTempRootRefusesASymlinkIntoASensitiveRoot(t *testing.T) {
	skipOnWindows(t, "symlinks into /etc are POSIX only")
	if _, err := os.Stat("/etc"); err != nil {
		t.Skip("no /etc on this host")
	}
	base := t.TempDir()
	link := filepath.Join(base, "innocent")
	if err := os.Symlink("/etc", link); err != nil {
		t.Skipf("cannot create a symlink: %v", err)
	}
	for _, raw := range []string{link, filepath.Join(link, "mcharbor-tmp")} {
		if _, err := validateTempRoot(raw, filepath.Join(base, "data")); !errors.Is(err, errTempSensitive) {
			t.Errorf("validateTempRoot(%q) = %v, want errTempSensitive: the check must run on the resolved path", raw, err)
		}
	}
}

func TestValidateTempRootRefusesAWorldWritableRoot(t *testing.T) {
	skipOnWindows(t, "mode bits are POSIX only")
	base := t.TempDir()
	open := filepath.Join(base, "open")
	if err := os.Mkdir(open, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(open, 0o777); err != nil {
		t.Fatal(err)
	}
	if _, err := validateTempRoot(open, filepath.Join(base, "data")); !errors.Is(err, errTempWorldWritable) {
		t.Fatalf("validateTempRoot(world-writable) = %v, want errTempWorldWritable", err)
	}
}

func TestValidateTempRootRefusesWhenTheProbeFails(t *testing.T) {
	base := t.TempDir()
	blocker := filepath.Join(base, "file")
	if err := os.WriteFile(blocker, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	raw := filepath.Join(blocker, "sub")
	if _, err := validateTempRoot(raw, filepath.Join(base, "data")); !errors.Is(err, errTempNotWritable) {
		t.Fatalf("validateTempRoot(%q) = %v, want errTempNotWritable", raw, err)
	}
}

func TestValidateTempRootAcceptsAFreshFolderAndReturnsItCleaned(t *testing.T) {
	base := t.TempDir()
	want := filepath.Join(base, "spool", "tmp")
	got, err := validateTempRoot(want+string(os.PathSeparator), filepath.Join(base, "data"))
	if err != nil || got != want {
		t.Fatalf("validateTempRoot = (%q, %v), want (%q, nil)", got, err, want)
	}
}

func TestValidateTempRootAcceptsTheDefaultRootUnderTheDataDir(t *testing.T) {
	base := t.TempDir()
	dataDir := filepath.Join(base, "data")
	want := filepath.Join(dataDir, "backups", "tmp")
	if got, err := validateTempRoot(want, dataDir); err != nil || got != want {
		t.Fatalf("the default root must stay valid, got (%q, %v)", got, err)
	}
}

func TestProbeTempRootCreatesTheFolderAndLeavesNoProbeFile(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "a", "b")
	if err := probeTempRoot(dir); err != nil {
		t.Fatalf("probeTempRoot: %v", err)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("the probe must create the folder: %v", err)
	}
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), "mcharbor-probe-") {
			t.Errorf("probe file %q was left behind", e.Name())
		}
	}
}

func TestProbeTempRootFailsOnAFile(t *testing.T) {
	file := filepath.Join(t.TempDir(), "plain")
	if err := os.WriteFile(file, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := probeTempRoot(file); err == nil {
		t.Fatal("probeTempRoot on a regular file must fail")
	}
}
