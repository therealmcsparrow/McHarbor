// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package instancebackup

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	coredb "github.com/therealmcsparrow/mcharbor/core/db"
	"github.com/therealmcsparrow/mcharbor/core/dbconfig"
	"github.com/therealmcsparrow/mcharbor/core/encryption"
)

// LedgerFile is the append-only restore ledger in the data directory. It lives
// outside audit_logs because a restore overwrites that table, and it rides along
// in every later export so the history of restores is never lost.
const LedgerFile = "restore-ledger.jsonl"

const (
	restoreDir  = ".restore"
	journalFile = "restore-journal.json"

	phaseCommitting = "committing"
	phaseCommitted  = "committed"
)

type journal struct {
	ID             string   `json:"id"`
	Phase          string   `json:"phase"`
	RestoresKeys   bool     `json:"restoresKeys"`
	RestoresSecret bool     `json:"restoresSecret"`
	RestoresFiles  bool     `json:"restoresFiles"`
	OldKey         string   `json:"oldKey"`
	Done           []string `json:"done,omitempty"`
}

// LedgerEntry is one line of the restore ledger.
type LedgerEntry struct {
	At     string  `json:"at"`
	Phase  string  `json:"phase"`
	ID     string  `json:"id,omitempty"`
	Intent *Intent `json:"intent,omitempty"`
	Error  string  `json:"error,omitempty"`
}

// AppendLedger adds one entry to the ledger and syncs it, because the entry
// that matters most is written moments before the database is replaced.
func AppendLedger(dataDir string, e LedgerEntry) error {
	if e.At == "" {
		e.At = time.Now().UTC().Format(time.RFC3339)
	}
	if e.Intent != nil && e.ID == "" {
		e.ID = e.Intent.ID
	}
	raw, err := json.Marshal(e)
	if err != nil {
		return err
	}
	f, err := os.OpenFile(filepath.Join(dataDir, LedgerFile), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return fmt.Errorf("opening the restore ledger: %w", err)
	}
	if _, err := f.Write(append(raw, '\n')); err != nil {
		logCleanup("f", f.Close())
		return fmt.Errorf("writing the restore ledger: %w", err)
	}
	if err := f.Sync(); err != nil {
		logCleanup("f", f.Close())
		return fmt.Errorf("syncing the restore ledger: %w", err)
	}
	return f.Close()
}

// ReadLedger returns the ledger's entries, oldest first.
func ReadLedger(dataDir string) ([]LedgerEntry, error) {
	raw, err := os.ReadFile(filepath.Join(dataDir, LedgerFile))
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("reading the restore ledger: %w", err)
	}
	var out []LedgerEntry
	for _, line := range bytes.Split(raw, []byte{'\n'}) {
		if len(bytes.TrimSpace(line)) == 0 {
			continue
		}
		var e LedgerEntry
		if err := json.Unmarshal(line, &e); err != nil {
			continue
		}
		out = append(out, e)
	}
	return out, nil
}

func writeJournal(dataDir string, j journal) error {
	raw, err := json.Marshal(j)
	if err != nil {
		return err
	}
	p := filepath.Join(dataDir, journalFile)
	tmp := p + ".tmp"
	if err := os.WriteFile(tmp, raw, 0o600); err != nil {
		return fmt.Errorf("writing the restore journal: %w", err)
	}
	if err := os.Rename(tmp, p); err != nil {
		return fmt.Errorf("writing the restore journal: %w", err)
	}
	return nil
}

func readJournal(dataDir string) (journal, bool, error) {
	var j journal
	raw, err := os.ReadFile(filepath.Join(dataDir, journalFile))
	if errors.Is(err, fs.ErrNotExist) {
		return j, false, nil
	}
	if err != nil {
		return j, false, fmt.Errorf("reading the restore journal: %w", err)
	}
	if err := json.Unmarshal(raw, &j); err != nil {
		return j, false, fmt.Errorf("the restore journal is damaged: %w", err)
	}
	return j, true, nil
}

func removeJournal(dataDir string) error {
	if err := os.Remove(filepath.Join(dataDir, journalFile)); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("removing the restore journal: %w", err)
	}
	return nil
}

func cleanup(staging string) {
	logCleanup("staging directory", os.RemoveAll(staging))
}

// stageBlob writes one blob into the staging area. Files and certificates
// are staged only for a full restore; key material is always staged, and
// finish decides whether it is used.
func stageBlob(staging string, e Entry, r io.Reader, plan Plan) error {
	var rel string
	switch e.Blob {
	case BlobKey:
		rel = e.Name
	case BlobTLS, BlobFile:
		if !plan.RestoresFiles {
			return nil
		}
		rel = "files/" + e.Name
		if e.Blob == BlobTLS {
			rel = e.Name
		}
	default:
		return nil
	}
	dst := filepath.Join(staging, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(dst), 0o700); err != nil {
		return fmt.Errorf("staging %s: %w", e.Name, err)
	}
	mode := fs.FileMode(e.Mode).Perm()
	if mode == 0 {
		mode = 0o600
	}
	f, err := os.OpenFile(dst, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, mode)
	if err != nil {
		return fmt.Errorf("staging %s: %w", e.Name, err)
	}
	if _, err := io.Copy(f, r); err != nil {
		logCleanup("f", f.Close())
		return fmt.Errorf("staging %s: %w", e.Name, err)
	}
	return f.Close()
}

// finish rolls a committed restore forward: key material, then certificates,
// then data files. Every step is safe to repeat, because Recover runs it
// again after a crash part-way through.
func finish(dataDir, id string) error {
	j, ok, err := readJournal(dataDir)
	if err != nil {
		return err
	}
	if !ok || j.ID != id {
		return fmt.Errorf("the restore journal does not describe restore %s", id)
	}
	staging := filepath.Join(dataDir, restoreDir, id)
	step := func(name string, fn func() error) error {
		if slices.Contains(j.Done, name) {
			return nil
		}
		if err := fn(); err != nil {
			return fmt.Errorf("%s: %w", name, err)
		}
		j.Done = append(j.Done, name)
		return writeJournal(dataDir, j)
	}
	if j.RestoresKeys {
		if err := step("keys", func() error { return swapKey(dataDir, staging, j.OldKey) }); err != nil {
			return err
		}
	}
	if j.RestoresSecret {
		if err := step("secret", func() error {
			return replaceFile(filepath.Join(staging, filepath.FromSlash(KeySecret)), filepath.Join(dataDir, ".secret"))
		}); err != nil {
			return err
		}
	}
	if j.RestoresFiles {
		if err := step("tls", func() error { return swapTLS(dataDir, staging) }); err != nil {
			return err
		}
		for _, root := range FileRoots {
			if root == LedgerFile {
				continue
			}
			root := root
			if err := step("files:"+root, func() error { return swapRoot(dataDir, staging, root) }); err != nil {
				return err
			}
		}
	}
	if err := removeJournal(dataDir); err != nil {
		return err
	}
	cleanup(filepath.Join(dataDir, restoreDir))
	return AppendLedger(dataDir, LedgerEntry{Phase: "done", ID: id})
}

func swapKey(dataDir, staging, oldKey string) error {
	newKey, err := os.ReadFile(filepath.Join(staging, filepath.FromSlash(KeyEncryption)))
	if err != nil {
		return err
	}
	if dbconfig.Exists(dataDir) {
		newEnc, err := encryption.New("", base64.StdEncoding.EncodeToString(newKey))
		if err != nil {
			return err
		}
		if _, err := dbconfig.Load(dataDir, newEnc); err != nil {
			oldEnc, err := encryption.New("", oldKey)
			if err != nil {
				return err
			}
			stored, err := dbconfig.Load(dataDir, oldEnc)
			if err != nil {
				return fmt.Errorf("reading the database connection with the old key: %w", err)
			}
			if err := dbconfig.Save(dataDir, newEnc, *stored); err != nil {
				return fmt.Errorf("re-encrypting the database connection: %w", err)
			}
		}
	}
	tmp := filepath.Join(dataDir, ".encryption_key.restore")
	if err := os.WriteFile(tmp, newKey, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, filepath.Join(dataDir, ".encryption_key"))
}

func swapTLS(dataDir, staging string) error {
	tlsDir := filepath.Join(dataDir, "tls")
	if err := os.MkdirAll(tlsDir, 0o700); err != nil {
		return err
	}
	stagedCA := filepath.Join(staging, filepath.FromSlash(TLSCACert))
	newCA, err := os.ReadFile(stagedCA)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	oldCA, err := os.ReadFile(filepath.Join(tlsDir, "ca.pem"))
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	for _, f := range []struct{ logical, name string }{{TLSCACert, "ca.pem"}, {TLSCAKey, "ca-key.pem"}, {TLSCert, "cert.pem"}, {TLSKey, "key.pem"}} {
		src := filepath.Join(staging, filepath.FromSlash(f.logical))
		if _, err := os.Stat(src); err == nil {
			if err := replaceFile(src, filepath.Join(tlsDir, f.name)); err != nil {
				return err
			}
		}
	}
	_, stagedCert := os.Stat(filepath.Join(staging, filepath.FromSlash(TLSCert)))
	if stagedCert != nil && !bytes.Equal(oldCA, newCA) {
		// The serving certificate was issued by the CA just replaced; removing
		// it makes the next start issue one from the restored CA, which is the
		// one every agent pins.
		for _, name := range []string{"cert.pem", "key.pem"} {
			if err := os.Remove(filepath.Join(tlsDir, name)); err != nil && !errors.Is(err, fs.ErrNotExist) {
				return err
			}
		}
	}
	return nil
}

func swapRoot(dataDir, staging, root string) error {
	target := filepath.Join(dataDir, root)
	staged := filepath.Join(staging, "files", root)
	old := filepath.Join(staging, "previous", root)
	if _, err := os.Stat(target); err == nil {
		if err := os.MkdirAll(filepath.Dir(old), 0o700); err != nil {
			return err
		}
		if err := os.Rename(target, old); err != nil {
			return err
		}
	}
	if _, err := os.Stat(staged); err == nil {
		return os.Rename(staged, target)
	}
	return nil
}

func replaceFile(src, dst string) error {
	raw, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	tmp := dst + ".restore"
	if err := os.WriteFile(tmp, raw, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, dst)
}

// Recover finishes or undoes a restore the previous process did not complete.
// It runs before the encryption key is loaded, so the key it leaves in place
// is the one the rest of startup reads. committed reports whether the
// database holds the restore's marker; it is asked only when the journal
// cannot tell.
func Recover(dataDir string, committed func(id string) (bool, error)) (string, error) {
	j, ok, err := readJournal(dataDir)
	if err != nil {
		return "", err
	}
	if !ok {
		cleanup(filepath.Join(dataDir, restoreDir))
		return "", nil
	}
	if j.Phase == phaseCommitting {
		done, err := committed(j.ID)
		if err != nil {
			return "", fmt.Errorf("checking whether restore %s committed: %w", j.ID, err)
		}
		if !done {
			if err := removeJournal(dataDir); err != nil {
				return "", err
			}
			cleanup(filepath.Join(dataDir, restoreDir))
			return "rolled back restore " + j.ID, AppendLedger(dataDir, LedgerEntry{Phase: "rolled-back", ID: j.ID})
		}
		j.Phase = phaseCommitted
		if err := writeJournal(dataDir, j); err != nil {
			return "", err
		}
	}
	if err := finish(dataDir, j.ID); err != nil {
		return "", err
	}
	return "completed restore " + j.ID, nil
}

// Pending reports whether a restore journal is present.
func Pending(dataDir string) bool {
	_, ok, err := readJournal(dataDir)
	return ok || err != nil || strings.TrimSpace(dataDir) == ""
}

// HasMarker reports whether the database committed restore id.
func HasMarker(ctx context.Context, database *sql.DB, driver coredb.Driver, id string) (bool, error) {
	var v string
	err := database.QueryRowContext(ctx, rebind(driver, "SELECT value FROM settings WHERE "+coredb.QuoteIdent(driver, "key")+" = ?"), MarkerKey).Scan(&v)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return v == id, nil
}
