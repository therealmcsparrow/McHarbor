// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

import (
	"bufio"
	"context"
	"crypto/tls"
	"database/sql"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"os/user"
	"strings"
	"time"

	"github.com/therealmcsparrow/mcharbor/core/apppaths"
	"github.com/therealmcsparrow/mcharbor/core/audit"
	"github.com/therealmcsparrow/mcharbor/core/bootstrapfs"
	"github.com/therealmcsparrow/mcharbor/core/config"
	"github.com/therealmcsparrow/mcharbor/core/db"
	"github.com/therealmcsparrow/mcharbor/core/dbconfig"
	"github.com/therealmcsparrow/mcharbor/core/encryption"
	coreinstancebackup "github.com/therealmcsparrow/mcharbor/core/instancebackup"
	modInstanceBackup "github.com/therealmcsparrow/mcharbor/modules/instancebackup"
)

// runRestoreLocal restores an instance export with no server running, for the
// instance whose interface will not start. It is authorised by access to the
// data directory and the export's key, and attributed to the OS user and host,
// because there is no McHarbor identity to check when McHarbor is down.
func runRestoreLocal(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("restore-local", flag.ContinueOnError)
	fs.SetOutput(stderr)
	file := fs.String("file", "", "instance export (.mchx) to restore")
	passFile := fs.String("passphrase-file", "", "file holding the export's passphrase")
	keyFile := fs.String("key-file", "", "the export's key file")
	passStdin := fs.Bool("passphrase-stdin", false, "read the passphrase from standard input")
	mode := fs.String("mode", coreinstancebackup.ModeFull, "full, migration or selective")
	categories := fs.String("categories", "", "comma-separated categories for a selective restore")
	withAudit := fs.Bool("include-audit", false, "also restore audit history (writes a restore anchor into the chain)")
	conflict := fs.String("conflict", "", "selective restore: replace (default) or keep")
	yes := fs.Bool("yes", false, "restore without asking to confirm the plan")
	whileRunning := fs.String("while-running", "", "restore even though McHarbor answers on this host; must name the instance id")
	planOnly := fs.Bool("plan", false, "verify the export and print the plan, then stop")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if *file == "" {
		sayln(stderr, "restore-local: --file is required")
		return 2
	}
	secret, err := localSecret(*passFile, *keyFile, *passStdin, stdin)
	if err != nil {
		sayln(stderr, "restore-local:", err)
		return 2
	}

	dataDir := apppaths.DataDir()
	if msg, err := coreinstancebackup.Recover(dataDir, probeRestoreMarker); err != nil {
		sayln(stderr, "restore-local: finishing an interrupted restore:", err)
		return 1
	} else if msg != "" {
		sayln(stderr, "restore-local:", msg)
	}
	secretPinned := secretFromOperator()
	if !secretPinned {
		s, err := bootstrapfs.EnsureSecret(dataDir)
		if err != nil {
			sayln(stderr, "restore-local: reading the instance secret:", err)
			return 1
		}
		if err := os.Setenv("MCHARBOR_SECRET", s); err != nil {
			sayln(stderr, "restore-local:", err)
			return 1
		}
	}
	cfg, err := loadConfig()
	if err != nil {
		sayln(stderr, "restore-local: loading configuration:", err)
		return 1
	}
	enc, err := encryption.New(cfg.DataDir, cfg.EncryptionKey)
	if err != nil {
		sayln(stderr, "restore-local: loading the encryption key:", err)
		return 1
	}
	dbCfg, _, err := dbconfig.Resolve(cfg, enc)
	if err != nil {
		sayln(stderr, "restore-local: resolving the database:", err)
		return 1
	}
	database, err := db.Open(dbCfg)
	if err != nil {
		sayln(stderr, "restore-local: opening the database:", err)
		return 1
	}
	defer func() {
		if err := database.Close(); err != nil {
			sayln(stderr, "restore-local: closing the database:", err)
		}
	}()
	if err := db.Migrate(database, dbCfg.Driver); err != nil {
		sayln(stderr, "restore-local: bringing the schema up to date:", err)
		return 1
	}

	ctx := context.Background()
	if code := refuseWhileRunning(ctx, cfg, database, *whileRunning, stderr); code != 0 {
		return code
	}

	var cats []string
	for _, c := range strings.Split(*categories, ",") {
		if c = strings.TrimSpace(c); c != "" {
			cats = append(cats, c)
		}
	}
	actor := localActor()
	target := coreinstancebackup.Target{
		DB: database, Driver: dbCfg.Driver, DataDir: cfg.DataDir, EncryptionKey: enc.KeyMaterial(), Secret: cfg.Secret,
		KeyPinned: cfg.EncryptionKey != "", SecretPinned: secretPinned,
	}
	opts := coreinstancebackup.RestoreOptions{
		Secret: secret, Mode: *mode, Categories: cats, IncludeAudit: *withAudit, Conflict: *conflict,
		Actor: actor, Via: "local",
	}
	open := func() (io.ReadCloser, error) { return os.Open(*file) }

	f, err := open()
	if err != nil {
		sayln(stderr, "restore-local:", err)
		return 1
	}
	plan, _, err := coreinstancebackup.PlanRestore(ctx, target, f, opts)
	if closeErr := f.Close(); err == nil && closeErr != nil {
		err = closeErr
	}
	if err != nil {
		sayln(stderr, "restore-local:", err)
		return 1
	}
	printPlan(stdout, plan)
	if len(plan.Refusals) > 0 {
		sayln(stderr, "restore-local: refused:", strings.Join(plan.Refusals, ", "))
		return 1
	}
	if *planOnly {
		return 0
	}
	if !*yes && !confirm(stdin, stderr, "Type RESTORE to replace this instance with the export: ", "RESTORE") {
		sayln(stderr, "restore-local: not confirmed; nothing was changed")
		return 1
	}

	auditLog := audit.NewLogger(database, cfg.Secret)
	opts.Emit = func(ctx context.Context, in coreinstancebackup.Intent) error {
		raw, err := json.Marshal(in)
		if err != nil {
			return err
		}
		sayf(stderr, "restore intent (also in %s): %s\n", coreinstancebackup.LedgerFile, raw)
		return auditLog.RecordContext(ctx, "", in.Actor, audit.Entry{
			Action: "instance.restore.intent", EntityType: "instance_backup", EntityID: in.ID, Details: string(raw), Severity: "warning",
		})
	}
	opts.Progress = func(p coreinstancebackup.Progress) {
		if p.Phase == "tables" && p.Total > 0 {
			sayf(stderr, "\r  %d / %d rows (%s)          ", p.Done, p.Total, p.Item)
		}
	}
	res, err := coreinstancebackup.Restore(ctx, target, open, opts)
	sayln(stderr)
	if err != nil {
		sayln(stderr, "restore-local: restore failed; the instance is unchanged:", err)
		return 1
	}
	if err := modInstanceBackup.RecordLocalRestore(cfg.DataDir, actor, *mode, res); err != nil {
		sayln(stderr, "restore-local: recording the outcome:", err)
	}
	sayf(stdout, "Restored %d rows in %d tables (restore %s).\n", res.Rows, res.Tables, res.ID)
	if res.AnchorID != "" {
		sayf(stdout, "Audit history restored; the chain continues through anchor %s.\n", res.AnchorID)
	}
	sayln(stdout, "Start McHarbor now. Plugins are reinstalled at their recorded versions on that start.")
	return 0
}

func localSecret(passFile, keyFile string, passStdin bool, stdin io.Reader) (coreinstancebackup.Secret, error) {
	var s coreinstancebackup.Secret
	switch {
	case keyFile != "":
		raw, err := os.ReadFile(keyFile)
		if err != nil {
			return s, fmt.Errorf("reading the key file: %w", err)
		}
		s.KeyFile = string(raw)
	case passFile != "":
		raw, err := os.ReadFile(passFile)
		if err != nil {
			return s, fmt.Errorf("reading the passphrase file: %w", err)
		}
		s.Passphrase = strings.TrimRight(string(raw), "\r\n")
	case passStdin:
		line, err := bufio.NewReader(stdin).ReadString('\n')
		if err != nil && !errors.Is(err, io.EOF) {
			return s, fmt.Errorf("reading the passphrase: %w", err)
		}
		s.Passphrase = strings.TrimRight(line, "\r\n")
	case os.Getenv("MCHARBOR_RESTORE_PASSPHRASE") != "":
		s.Passphrase = os.Getenv("MCHARBOR_RESTORE_PASSPHRASE")
	}
	if reason := s.CheckOpen(); reason != "" {
		return s, fmt.Errorf("give the export's key with --key-file, --passphrase-file, --passphrase-stdin or MCHARBOR_RESTORE_PASSPHRASE")
	}
	return s, nil
}

// instanceID is the id an operator must name to restore over a running
// instance: the license install row, the same id the export manifest carries.
func instanceID(ctx context.Context, database *sql.DB) string {
	var id string
	if err := database.QueryRowContext(ctx, `SELECT id FROM license_installs ORDER BY created_at LIMIT 1`).Scan(&id); err != nil {
		return "unknown"
	}
	return id
}

func localActor() string {
	name := "unknown"
	if u, err := user.Current(); err == nil && u.Username != "" {
		name = u.Username
	}
	host, err := os.Hostname()
	if err != nil || host == "" {
		host = "unknown-host"
	}
	return "local:" + name + "@" + host
}

func confirm(stdin io.Reader, stderr io.Writer, prompt, want string) bool {
	say(stderr, prompt)
	line, err := bufio.NewReader(stdin).ReadString('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		return false
	}
	return strings.TrimSpace(line) == want
}

func printPlan(w io.Writer, p coreinstancebackup.Plan) {
	m := p.Manifest
	sayf(w, "Export from %s, McHarbor %s (%s), taken %s by %s\n", m.Hostname, m.Version, m.Backend,
		m.CreatedAt.Format(time.RFC3339), m.CreatedBy)
	sayf(w, "Mode: %s   restores keys: %t   restores audit secret: %t   restores files: %t\n",
		p.Mode, p.RestoresKeys, p.RestoresSecret, p.RestoresFiles)
	for _, c := range p.Categories {
		mark := " "
		if c.Selected {
			mark = "x"
		}
		var here, there int64
		for _, t := range c.Tables {
			here += t.TargetRows
			there += t.ArtifactRows
		}
		sayf(w, "  [%s] %-13s %7d rows now -> %7d from the export\n", mark, c.Name, here, there)
	}
	for _, warn := range p.Warnings {
		sayln(w, "  note:", warn)
	}
	if len(p.Plugins) > 0 {
		sayf(w, "Plugins reinstalled on the next start: %d\n", len(p.Plugins))
	}
}

// refuseWhileRunning stops a local restore when McHarbor answers on this host:
// a local restore of a running instance bypasses the permission and approval
// the web path enforces, so it must be impossible to do by accident.
func refuseWhileRunning(ctx context.Context, cfg *config.Config, database *sql.DB, override string, stderr io.Writer) int {
	scheme := "http"
	if cfg.TLSEnabled {
		scheme = "https"
	}
	// Only asks whether something answers on loopback; the certificate is the
	// instance's own and trusting it proves nothing either way.
	client := &http.Client{Timeout: 3 * time.Second, Transport: &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true, MinVersion: tls.VersionTLS12}, // #nosec G402 -- loopback liveness probe only
	}}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, fmt.Sprintf("%s://127.0.0.1:%d/api/health", scheme, cfg.Port), nil)
	if err != nil {
		return 0
	}
	resp, err := client.Do(req)
	if err != nil {
		return 0
	}
	if err := resp.Body.Close(); err != nil {
		return 0
	}
	if resp.StatusCode >= 500 {
		return 0
	}
	inst := instanceID(ctx, database)
	if override != "" && override == inst {
		sayln(stderr, "restore-local: McHarbor is running here; restoring anyway because --while-running names this instance")
		return 0
	}
	sayf(stderr, "restore-local: McHarbor is running on this host and answering. Restore from Settings -> System -> Instance backup, "+
		"where the restore permissions apply. To restore locally anyway, stop McHarbor or pass --while-running %s\n", inst)
	return 1
}

// sayln writes a console line and logs a failed write, because a closed pipe on the operator's terminal has nobody left to tell.
func sayln(w io.Writer, a ...any) {
	if _, err := fmt.Fprintln(w, a...); err != nil {
		slog.Warn("console write failed", "error", err)
	}
}

// sayf is the formatted form of sayln.
func sayf(w io.Writer, format string, a ...any) {
	if _, err := fmt.Fprintf(w, format, a...); err != nil {
		slog.Warn("console write failed", "error", err)
	}
}

// say is the unformatted, no-newline form of sayln, used for prompts.
func say(w io.Writer, a ...any) {
	if _, err := fmt.Fprint(w, a...); err != nil {
		slog.Warn("console write failed", "error", err)
	}
}
