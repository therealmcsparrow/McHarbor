// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/rs/xid"
)

// Importing a guest from another hypervisor.
//
// The operator uploads a disk or an OVA, McHarbor stages it, and the agent on
// the target host pulls it, reads it and converts it into volumes in a pool.
// The conversion never happens here: qemu-img is a compute-host dependency the
// McHarbor container deliberately does not carry, and a 200 GB VMDK converted
// on the server would have to cross the network twice to get where it is going.
//
// The staged upload reaches the agent through a one-time grant on a public
// route, which is the same shape the compute plugin's image library already
// uses for the same reason: the agent holds no session, and the alternative is
// pushing gigabytes down the WebSocket control channel.

// importStagingSubdir is where uploads wait under the host's data directory.
const importStagingSubdir = "kvm-imports"

// maxImportBytes bounds one upload at 2 TiB — larger than any disk somebody
// imports over a browser, small enough that a runaway client cannot fill the
// disk unnoticed.
const maxImportBytes int64 = 2 << 40

// importGrantTTL bounds how long the agent's pull token stays usable. Long
// enough for a very large disk over a slow link.
const importGrantTTL = 12 * time.Hour

// importExtensions are the sources this feature accepts.
//
// An allow-list rather than "whatever qemu-img reads": a format McHarbor cannot
// name a source hypervisor for is one nobody can support. The agent checks the
// same set — this copy stops a bad upload before it is written, that one is the
// boundary that matters.
var importExtensions = map[string]string{
	".ova":   "VMware / OVF appliance",
	".ovf":   "OVF descriptor",
	".vmdk":  "VMware",
	".vhdx":  "Hyper-V",
	".vhd":   "Hyper-V (legacy)",
	".vdi":   "VirtualBox",
	".hdd":   "Parallels",
	".hds":   "Parallels",
	".qcow2": "QEMU / KVM",
	".qcow":  "QEMU / KVM",
	".img":   "Raw disk",
	".raw":   "Raw disk",
}

// Import states.
const (
	ImportUploaded   = "uploaded"
	ImportInspecting = "inspecting"
	ImportInspected  = "inspected"
	ImportConverting = "converting"
	ImportReady      = "ready"
	ImportFailed     = "failed"
)

// Errors the import handlers map onto i18n codes.
var (
	ErrImportEmpty     = errors.New("no file was uploaded")
	ErrImportTooLarge  = errors.New("the uploaded file exceeds the size limit")
	ErrImportExtension = errors.New("unsupported source format")
	ErrImportState     = errors.New("that import is not in a state where this is possible")
	ErrImportStaging   = errors.New("the import staging directory is not writable")
)

// Import is one guest being brought in from another hypervisor.
type Import struct {
	ID       string `json:"id"`
	FileName string `json:"fileName"`
	// SourceKind is the hypervisor the extension names, for display.
	SourceKind string `json:"sourceKind"`
	SizeBytes  int64  `json:"sizeBytes"`
	State      string `json:"state"`
	Progress   int    `json:"progress"`
	LastError  string `json:"lastError,omitempty"`

	EnvironmentID string `json:"environmentId,omitempty"`
	// HostPath is where the environment's host put the upload. Once it is set
	// nothing else needs McHarbor's copy: an inspect, a conversion and a retry
	// after a restart all read it directly.
	HostPath string `json:"hostPath,omitempty"`

	// Inspection is what the agent found: the disks, and — for an OVA — the
	// guest's shape as its descriptor recorded it.
	Inspection *ImportInspection `json:"inspection,omitempty"`
	// VMID is the guest this import produced, once it has produced one.
	VMID string `json:"vmId,omitempty"`

	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

// ImportInspection mirrors the agent's VMImportResult.
type ImportInspection struct {
	Kind  string       `json:"kind"`
	Disks []ImportDisk `json:"disks,omitempty"`

	Name           string   `json:"name,omitempty"`
	VCPUs          int      `json:"vcpus,omitempty"`
	MemoryMB       int64    `json:"memoryMb,omitempty"`
	Firmware       string   `json:"firmware,omitempty"`
	GuestOSFamily  string   `json:"guestOsFamily,omitempty"`
	GuestOSVariant string   `json:"guestOsVariant,omitempty"`
	NICCount       int      `json:"nicCount,omitempty"`
	Warnings       []string `json:"warnings,omitempty"`
}

// ImportDisk is one disk in a source.
type ImportDisk struct {
	Index        int    `json:"index"`
	SourceFile   string `json:"sourceFile"`
	SourceFormat string `json:"sourceFormat"`
	VirtualSize  int64  `json:"virtualSize"`
	ActualSize   int64  `json:"actualSize,omitempty"`
	VolumePath   string `json:"volumePath,omitempty"`
}

// ImportConvertRequest is what the operator chose on the review screen.
type ImportConvertRequest struct {
	EnvironmentID string `json:"environmentId"`
	Name          string `json:"name"`

	PoolID      string `json:"poolId"`
	PoolBackend string `json:"poolBackend"`
	PoolTarget  string `json:"poolTarget"`

	NetworkID string `json:"networkId"`
	Bridge    string `json:"bridge"`
	VLANTag   int    `json:"vlanTag,omitempty"`
	// AttachKind, ParentLink and LinkMode carry a direct network's attachment,
	// denormalised onto the NIC exactly as Bridge is.
	AttachKind string `json:"attachKind,omitempty"`
	ParentLink string `json:"parentLink,omitempty"`
	LinkMode   string `json:"linkMode,omitempty"`

	// Overrides for what the descriptor said, or for what it did not say.
	VCPUs    int   `json:"vcpus,omitempty"`
	MemoryMB int64 `json:"memoryMb,omitempty"`
	// GuestOSFamily decides the devices the guest gets. A bare disk image
	// describes nothing, and a Windows guest handed Linux defaults boots to a
	// black screen on a virtio display it has no driver for.
	GuestOSFamily string `json:"guestOsFamily,omitempty"`
	// Firmware matters more than any other override: a guest installed under
	// BIOS does not boot under UEFI, and the operator is the one who knows.
	Firmware string `json:"firmware,omitempty"`
	// SecureBoot and TPMEnabled are what a Windows 11 image needs and what an
	// older guest must not be given: Secure Boot without UEFI is refused by
	// validateFirmware rather than silently dropped.
	SecureBoot bool `json:"secureBoot,omitempty"`
	TPMEnabled bool `json:"tpmEnabled,omitempty"`
	Autostart  bool `json:"autostart,omitempty"`
}

// agentImportSpec mirrors the protocol's VMImportSpec.
type agentImportSpec struct {
	ImportID    string `json:"importId"`
	SourcePath  string `json:"sourcePath,omitempty"`
	SourceURL   string `json:"sourceUrl,omitempty"`
	SourceToken string `json:"sourceToken,omitempty"`
	FileName    string `json:"fileName,omitempty"`

	PoolID      string `json:"poolId,omitempty"`
	PoolBackend string `json:"poolBackend,omitempty"`
	PoolTarget  string `json:"poolTarget,omitempty"`
	VMID        string `json:"vmId,omitempty"`
	Format      string `json:"format,omitempty"`
}

// agentImportResult mirrors the protocol's VMImportResult.
type agentImportResult struct {
	ImportID string       `json:"importId"`
	Kind     string       `json:"kind"`
	Disks    []ImportDisk `json:"disks,omitempty"`
	// StagedPath is where the host put the upload, and the answer to
	// vm_import_stage.
	StagedPath string `json:"stagedPath,omitempty"`

	Name           string   `json:"name,omitempty"`
	VCPUs          int      `json:"vcpus,omitempty"`
	MemoryMB       int64    `json:"memoryMb,omitempty"`
	Firmware       string   `json:"firmware,omitempty"`
	GuestOSFamily  string   `json:"guestOsFamily,omitempty"`
	GuestOSVariant string   `json:"guestOsVariant,omitempty"`
	NICCount       int      `json:"nicCount,omitempty"`
	Warnings       []string `json:"warnings,omitempty"`
}

// importGrant authorizes one agent to pull one staged upload.
type importGrant struct {
	token string
	path  string
	// importID lets the pull handler attribute the bytes it serves, which is
	// the only place in the server that knows how far a transfer has got.
	importID string
	expires  time.Time
}

// importGrants issues and redeems those grants.
//
// Not single-use: a transfer that drops half way is retried by the agent, and
// invalidating on first contact would turn every retry into a failure somebody
// has to chase. It expires instead.
type importGrants struct {
	mu     sync.Mutex
	grants map[string]importGrant
}

func newImportGrants() *importGrants {
	return &importGrants{grants: make(map[string]importGrant)}
}

// issue records a grant for one staged file.
func (g *importGrants) issue(path, importID string) (string, string, error) {
	idBytes := make([]byte, 16)
	if _, err := rand.Read(idBytes); err != nil {
		return "", "", err
	}
	tokenBytes := make([]byte, 32)
	if _, err := rand.Read(tokenBytes); err != nil {
		return "", "", err
	}
	id := hex.EncodeToString(idBytes)
	token := hex.EncodeToString(tokenBytes)

	g.mu.Lock()
	defer g.mu.Unlock()
	g.sweepLocked()
	g.grants[id] = importGrant{token: token, path: path, importID: importID,
		expires: time.Now().Add(importGrantTTL)}
	return id, token, nil
}

// redeem returns the path a grant authorizes.
//
// Constant-time, because this is the only thing between an unauthenticated
// route and the staging directory: a comparison that returned early would leak
// the token through how long a wrong guess takes to reject.
func (g *importGrants) redeem(id, token string) (string, string, bool) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.sweepLocked()

	grant, ok := g.grants[id]
	if !ok {
		return "", "", false
	}
	if subtle.ConstantTimeCompare([]byte(grant.token), []byte(token)) != 1 {
		return "", "", false
	}
	return grant.path, grant.importID, true
}

// revoke drops a grant once its transfer is finished with.
func (g *importGrants) revoke(id string) {
	g.mu.Lock()
	delete(g.grants, id)
	g.mu.Unlock()
}

// sweepLocked discards expired grants.
func (g *importGrants) sweepLocked() {
	now := time.Now()
	for id, grant := range g.grants {
		if now.After(grant.expires) {
			delete(g.grants, id)
		}
	}
}

// ensureImportSchema creates the import table if it is missing.
//
// Paired with 0003_imports.sql for the same reason the schedule tables are:
// a plugin's migrations run once, at install.
func (s *Service) ensureImportSchema(ctx context.Context) error {
	statements := []string{
		`CREATE TABLE IF NOT EXISTS plugin_addon_kvm_imports (
		    id             TEXT PRIMARY KEY,
		    file_name      TEXT NOT NULL,
		    source_kind    TEXT NOT NULL DEFAULT '',
		    staged_path    TEXT NOT NULL,
		    size_bytes     INTEGER NOT NULL DEFAULT 0,
		    state          TEXT NOT NULL,
		    progress       INTEGER NOT NULL DEFAULT 0,
		    last_error     TEXT,
		    environment_id TEXT,
		    host_path      TEXT,
		    inspection     TEXT,
		    vm_id          TEXT,
		    created_at     TEXT NOT NULL,
		    updated_at     TEXT NOT NULL
		)`,
		`CREATE INDEX IF NOT EXISTS idx_plugin_addon_kvm_imports_state
		    ON plugin_addon_kvm_imports(state)`,
	}
	for _, statement := range statements {
		if _, err := s.db.Exec(ctx, statement); err != nil {
			return fmt.Errorf("preparing the import table: %w", err)
		}
	}
	// A plugin's migrations run once, at install, so an installation that
	// predates host staging needs the column added here. SQLite has no
	// ADD COLUMN IF NOT EXISTS; a duplicate is the expected second run.
	if _, err := s.db.Exec(ctx,
		`ALTER TABLE plugin_addon_kvm_imports ADD COLUMN host_path TEXT`); err != nil &&
		!strings.Contains(strings.ToLower(err.Error()), "duplicate column") {
		return fmt.Errorf("adding the import host path column: %w", err)
	}
	return nil
}

// importStagingRoot is where uploads are kept.
func (s *Service) importStagingRoot() string {
	return filepath.Join(s.workDir, importStagingSubdir)
}

// ensureImportStaging creates the staging directory on first use.
//
// 0o700 because these are bootable disks: anything that can rewrite one owns
// every guest imported from it.
func (s *Service) ensureImportStaging() (string, error) {
	root := s.importStagingRoot()
	if err := os.MkdirAll(root, 0o700); err != nil {
		return "", fmt.Errorf("%w: %s: %v", ErrImportStaging, root, err)
	}
	return root, nil
}

// ImportSourceKind names the hypervisor an extension belongs to.
func ImportSourceKind(fileName string) (string, bool) {
	kind, ok := importExtensions[strings.ToLower(filepath.Ext(fileName))]
	return kind, ok
}

// ReceiveImport streams one upload into staging and records it.
//
// The file is written under the import's own id rather than the operator's
// filename: two people importing "disk.vmdk" on the same afternoon must not
// overwrite each other, and the original name survives as the display name.
func (s *Service) ReceiveImport(ctx context.Context, fileName, envID string, src io.Reader) (Import, error) {
	kind, ok := ImportSourceKind(fileName)
	if !ok {
		return Import{}, fmt.Errorf("%w: %q", ErrImportExtension, filepath.Ext(fileName))
	}
	root, err := s.ensureImportStaging()
	if err != nil {
		return Import{}, err
	}

	id := xid.New().String()
	target := filepath.Join(root, id+strings.ToLower(filepath.Ext(fileName)))

	// Written under a partial name and renamed only once complete, so an
	// interrupted upload is never picked up as a source to convert.
	tmp, err := os.CreateTemp(root, ".partial-*")
	if err != nil {
		return Import{}, fmt.Errorf("%w: %v", ErrImportStaging, err)
	}
	tmpName := tmp.Name()
	committed := false
	defer func() {
		_ = tmp.Close()
		if !committed {
			_ = os.Remove(tmpName)
		}
	}()

	written, err := copyImportBytes(ctx, tmp, src)
	if err != nil {
		return Import{}, err
	}
	if written == 0 {
		return Import{}, ErrImportEmpty
	}
	if err := tmp.Sync(); err != nil {
		return Import{}, fmt.Errorf("%w: %v", ErrImportStaging, err)
	}
	if err := tmp.Close(); err != nil {
		return Import{}, fmt.Errorf("%w: %v", ErrImportStaging, err)
	}
	if err := os.Chmod(tmpName, 0o600); err != nil {
		return Import{}, fmt.Errorf("%w: %v", ErrImportStaging, err)
	}
	if err := os.Rename(tmpName, target); err != nil {
		return Import{}, fmt.Errorf("%w: %v", ErrImportStaging, err)
	}
	committed = true

	now := time.Now().UTC()
	record := Import{
		ID: id, FileName: filepath.Base(fileName), SourceKind: kind,
		SizeBytes: written, State: ImportUploaded, EnvironmentID: envID,
		CreatedAt: now, UpdatedAt: now,
	}
	if _, err := s.db.Exec(ctx, `
		INSERT INTO plugin_addon_kvm_imports
			(id, file_name, source_kind, staged_path, size_bytes, state,
			 environment_id, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		record.ID, record.FileName, record.SourceKind, target, record.SizeBytes,
		record.State, nullIfEmpty(envID), now.Format(time.RFC3339), now.Format(time.RFC3339)); err != nil {
		_ = os.Remove(target)
		return Import{}, fmt.Errorf("recording the import: %w", err)
	}
	return record, nil
}

// copyImportBytes streams src to dst under the size ceiling, honouring
// cancellation so a browser that goes away does not leave a goroutine writing
// terabytes to disk.
func copyImportBytes(ctx context.Context, dst io.Writer, src io.Reader) (int64, error) {
	buf := make([]byte, 1<<20)
	var written int64
	for {
		select {
		case <-ctx.Done():
			return written, ctx.Err()
		default:
		}
		n, readErr := src.Read(buf)
		if n > 0 {
			if written+int64(n) > maxImportBytes {
				return written, ErrImportTooLarge
			}
			if _, err := dst.Write(buf[:n]); err != nil {
				return written, fmt.Errorf("%w: %v", ErrImportStaging, err)
			}
			written += int64(n)
		}
		if errors.Is(readErr, io.EOF) {
			return written, nil
		}
		if readErr != nil {
			return written, fmt.Errorf("reading the upload: %w", readErr)
		}
	}
}

// ListImports returns every import, newest first.
func (s *Service) ListImports(ctx context.Context) ([]Import, error) {
	rows, err := s.db.Query(ctx, `
		SELECT id, file_name, source_kind, size_bytes, state, progress,
		       COALESCE(last_error, ''), COALESCE(environment_id, ''),
		       COALESCE(host_path, ''),
		       COALESCE(inspection, ''), COALESCE(vm_id, ''), created_at, updated_at
		  FROM plugin_addon_kvm_imports
		 ORDER BY created_at DESC
		 LIMIT ?`, maxRows)
	if err != nil {
		return nil, fmt.Errorf("listing imports: %w", err)
	}
	defer func() { _ = rows.Close() }()

	out := []Import{}
	for rows.Next() {
		record, scanErr := scanImport(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		out = append(out, record)
	}
	return out, rows.Err()
}

// importScanner is the one method scanImport needs.
type importScanner interface {
	Scan(dest ...any) error
}

// scanImport reads one import row.
func scanImport(rows importScanner) (Import, error) {
	var out Import
	var inspection, created, updated string
	if err := rows.Scan(&out.ID, &out.FileName, &out.SourceKind, &out.SizeBytes,
		&out.State, &out.Progress, &out.LastError, &out.EnvironmentID, &out.HostPath,
		&inspection, &out.VMID, &created, &updated); err != nil {
		return Import{}, fmt.Errorf("scanning an import: %w", err)
	}
	if inspection != "" {
		var parsed ImportInspection
		if err := json.Unmarshal([]byte(inspection), &parsed); err == nil {
			out.Inspection = &parsed
		}
	}
	out.CreatedAt, _ = time.Parse(time.RFC3339, created)
	out.UpdatedAt, _ = time.Parse(time.RFC3339, updated)
	return out, nil
}

// GetImport reads one import, with the staged path the caller needs to act.
func (s *Service) GetImport(ctx context.Context, id string) (Import, string, error) {
	rows, err := s.db.Query(ctx, `
		SELECT id, file_name, source_kind, size_bytes, state, progress,
		       COALESCE(last_error, ''), COALESCE(environment_id, ''),
		       COALESCE(host_path, ''),
		       COALESCE(inspection, ''), COALESCE(vm_id, ''), created_at, updated_at,
		       COALESCE(staged_path, '')
		  FROM plugin_addon_kvm_imports
		 WHERE id = ?
		 LIMIT 1`, id)
	if err != nil {
		return Import{}, "", fmt.Errorf("reading an import: %w", err)
	}
	defer func() { _ = rows.Close() }()
	if !rows.Next() {
		return Import{}, "", ErrNotFound
	}

	var out Import
	var inspection, created, updated, staged string
	if err := rows.Scan(&out.ID, &out.FileName, &out.SourceKind, &out.SizeBytes,
		&out.State, &out.Progress, &out.LastError, &out.EnvironmentID, &out.HostPath,
		&inspection, &out.VMID, &created, &updated, &staged); err != nil {
		return Import{}, "", fmt.Errorf("scanning an import: %w", err)
	}
	if inspection != "" {
		var parsed ImportInspection
		if err := json.Unmarshal([]byte(inspection), &parsed); err == nil {
			out.Inspection = &parsed
		}
	}
	out.CreatedAt, _ = time.Parse(time.RFC3339, created)
	out.UpdatedAt, _ = time.Parse(time.RFC3339, updated)
	return out, staged, nil
}

// setImportState records a state change.
func (s *Service) setImportState(ctx context.Context, id, state, errText string) {
	stamp := time.Now().UTC().Format(time.RFC3339)
	if _, err := s.db.Exec(ctx, `
		UPDATE plugin_addon_kvm_imports
		   SET state = ?, last_error = ?, updated_at = ?
		 WHERE id = ?`,
		state, nullIfEmpty(errText), stamp, id); err != nil {
		s.logger.Error("kvm: recording an import's state failed", "import", id, "error", err)
	}
}

// importSource decides how the host will reach this import's file.
//
// A staged import is already on the host, so it is named by path and nothing is
// authorized, transferred or able to expire - which is what makes an inspect or
// a retried conversion survive a McHarbor restart. Only an import uploaded
// before host staging existed still falls back to a pull grant.
func (s *Service) importSource(record Import, staged, id string) (agentImportSpec, func(), error) {
	spec := agentImportSpec{ImportID: id, FileName: record.FileName}
	if record.HostPath != "" {
		spec.SourcePath = record.HostPath
		return spec, func() {}, nil
	}
	grantID, token, err := s.imports.issue(staged, id)
	if err != nil {
		return agentImportSpec{}, func() {}, fmt.Errorf("authorizing the import transfer: %w", err)
	}
	// Relative: only the agent knows how it reaches McHarbor, so it joins this
	// against the address it dialled.
	spec.SourceURL = "/api/vms/imports/internal/" + grantID
	spec.SourceToken = token
	return spec, func() { s.imports.revoke(grantID) }, nil
}

// StageImportOnHost hands a finished upload to the environment that will run
// the guest, and drops McHarbor's copy once the host has it.
//
// This is what moves an import off the control plane: from here on the file
// lives beside the pool it will be converted into, so neither a restart nor an
// expired grant can interrupt the conversion, and the source crosses the
// network once instead of once per operation.
func (s *Service) StageImportOnHost(ctx context.Context, id, envID string) (Import, error) {
	record, staged, err := s.GetImport(ctx, id)
	if err != nil {
		return Import{}, err
	}
	if record.HostPath != "" {
		return record, nil
	}
	if s.rpc == nil {
		return Import{}, ErrHostUnavailable
	}

	grantID, token, err := s.imports.issue(staged, id)
	if err != nil {
		return Import{}, fmt.Errorf("authorizing the import transfer: %w", err)
	}
	defer s.imports.revoke(grantID)

	// The transfer outlives the upload request for the same reason an inspect
	// does: a 40 GB disk does not cross a network inside an HTTP deadline.
	runCtx, cancel := context.WithTimeout(context.Background(), importRunTimeout)
	// The outcome is written after a run that outlives this request, so the writes keep
	// the request's values but not its cancellation.
	writeCtx := context.WithoutCancel(ctx)
	if _, err := s.importRuns.start(id, "staging", cancel); err != nil {
		cancel()
		return Import{}, err
	}
	defer func() {
		s.importRuns.finish(id)
		cancel()
	}()
	stopWatch := s.watchImportProgress(id, envID, nil)
	defer stopWatch()

	reply, err := s.call(runCtx, envID, msgVMImportStage, agentPayload{
		Import: &agentImportSpec{
			ImportID:    id,
			SourceURL:   "/api/vms/imports/internal/" + grantID,
			SourceToken: token,
			FileName:    record.FileName,
		},
	})
	stopWatch()
	if err != nil {
		return Import{}, err
	}
	if reply.ImportResult == nil || reply.ImportResult.StagedPath == "" {
		return Import{}, fmt.Errorf("the host did not say where it put the upload")
	}
	hostPath := reply.ImportResult.StagedPath

	stamp := time.Now().UTC().Format(time.RFC3339)
	if _, err := s.db.Exec(writeCtx, `
		UPDATE plugin_addon_kvm_imports
		   SET host_path = ?, environment_id = ?, staged_path = '', updated_at = ?
		 WHERE id = ?`, hostPath, envID, stamp, id); err != nil {
		return Import{}, fmt.Errorf("recording where the host staged the import: %w", err)
	}
	// Only now: while the row still named it a crash here would leave the file
	// behind, and after it the row points at the host's copy and McHarbor's is
	// dead weight the size of a disk image.
	if staged != "" {
		if err := os.Remove(staged); err != nil && !errors.Is(err, os.ErrNotExist) {
			s.logger.Warn("kvm: removing the local copy of a staged import failed",
				"import", id, "path", staged, "error", err)
		}
	}

	record.HostPath = hostPath
	record.EnvironmentID = envID
	return record, nil
}

// discardOnHost asks the host to drop an import's staged file.
//
// Best effort and logged: the operator asked for the record to go, and a host
// that is unreachable must not stop it going.
func (s *Service) discardOnHost(ctx context.Context, record Import) {
	if record.HostPath == "" || record.EnvironmentID == "" || s.rpc == nil {
		return
	}
	if _, err := s.call(ctx, record.EnvironmentID, msgVMImportDiscard, agentPayload{
		Import: &agentImportSpec{ImportID: record.ID},
	}); err != nil {
		s.logger.Warn("kvm: the host kept the staged copy of a deleted import",
			"import", record.ID, "environment", record.EnvironmentID, "error", err)
	}
}

// InspectImport asks a host to read the staged source.
//
// The environment is part of the question, not an implementation detail: the
// inspection runs on the host that will do the conversion, so a source that
// this host's qemu-img cannot read is reported now rather than after the
// operator has filled in a form.
func (s *Service) InspectImport(ctx context.Context, id, envID string) (Import, error) {
	record, staged, err := s.GetImport(ctx, id)
	if err != nil {
		return Import{}, err
	}
	if s.rpc == nil {
		return Import{}, ErrHostUnavailable
	}
	if record.HostPath != "" && record.EnvironmentID != "" {
		// The file is on that host and nowhere else, so that is the host that
		// can read it — whatever the caller asked for.
		envID = record.EnvironmentID
	}

	source, release, err := s.importSource(record, staged, id)
	if err != nil {
		return Import{}, err
	}
	defer release()

	// The run outlives this request. A 40 GB pull cannot finish inside any
	// sane HTTP deadline, and tying it to the caller is what left this record
	// saying "inspecting" every time a tab closed or the server restarted.
	runCtx, cancel := context.WithTimeout(context.Background(), importRunTimeout)
	// The outcome is written after a run that outlives this request, so the writes keep
	// the request's values but not its cancellation.
	writeCtx := context.WithoutCancel(ctx)
	job, err := s.importRuns.start(id, "inspect", cancel)
	if err != nil {
		cancel()
		return Import{}, err
	}
	defer func() {
		s.importRuns.finish(id)
		cancel()
	}()
	_ = job

	s.setImportStateProgress(writeCtx, id, ImportInspecting, "", 0)
	stopWatch := s.watchImportProgress(id, envID, nil)
	defer stopWatch()

	reply, err := s.call(runCtx, envID, msgVMImportInspect, agentPayload{Import: &source})
	stopWatch()
	if err != nil {
		if errors.Is(runCtx.Err(), context.Canceled) {
			return Import{}, fmt.Errorf("%w: the inspection was cancelled", ErrImportState)
		}
		s.setImportStateProgress(writeCtx, id, ImportFailed, err.Error(), 0)
		return Import{}, err
	}
	if reply.ImportResult == nil {
		s.setImportStateProgress(writeCtx, id, ImportFailed, "the host returned no inspection", 0)
		return Import{}, fmt.Errorf("the host returned no inspection")
	}

	inspection := toInspection(reply.ImportResult)
	encoded, err := json.Marshal(inspection)
	if err != nil {
		return Import{}, fmt.Errorf("encoding the inspection: %w", err)
	}
	stamp := time.Now().UTC().Format(time.RFC3339)
	if _, err := s.db.Exec(writeCtx, `
		UPDATE plugin_addon_kvm_imports
		   SET state = ?, inspection = ?, environment_id = ?, last_error = NULL, updated_at = ?
		 WHERE id = ?`,
		ImportInspected, string(encoded), envID, stamp, id); err != nil {
		return Import{}, fmt.Errorf("recording the inspection: %w", err)
	}

	record.State = ImportInspected
	record.EnvironmentID = envID
	record.Inspection = &inspection
	record.LastError = ""
	return record, nil
}

// toInspection converts the agent's reply into the stored shape.
func toInspection(in *agentImportResult) ImportInspection {
	return ImportInspection{
		Kind: in.Kind, Disks: in.Disks,
		Name: in.Name, VCPUs: in.VCPUs, MemoryMB: in.MemoryMB,
		Firmware: in.Firmware, GuestOSFamily: in.GuestOSFamily,
		GuestOSVariant: in.GuestOSVariant, NICCount: in.NICCount,
		Warnings: in.Warnings,
	}
}

// ConvertImport converts the source and creates the guest it describes.
//
// The VM row is written first so the converted volumes have an id to be named
// after, and rolled back if the conversion fails — a guest listed with disks
// that were never produced is worse than no guest at all.
func (s *Service) ConvertImport(ctx context.Context, id string, req ImportConvertRequest) (VM, error) {
	record, staged, err := s.GetImport(ctx, id)
	if err != nil {
		return VM{}, err
	}
	if record.Inspection == nil {
		return VM{}, fmt.Errorf("%w: it has not been inspected yet", ErrImportState)
	}
	if s.rpc == nil {
		return VM{}, ErrHostUnavailable
	}
	if record.HostPath != "" && record.EnvironmentID != "" {
		// The file is on that host. Converting it anywhere else would mean
		// transferring it again, so the guest is created where its disk already is.
		req.EnvironmentID = record.EnvironmentID
	}

	pool, err := s.resolvePool(ctx, req.EnvironmentID, PoolRef{ID: req.PoolID, Backend: req.PoolBackend, Target: req.PoolTarget})
	if err != nil {
		return VM{}, err
	}
	req.PoolBackend, req.PoolTarget = pool.Backend, pool.Target

	capability, err := s.rpc.Probe(ctx, req.EnvironmentID)
	if err != nil {
		return VM{}, err
	}
	if !capability.CanRunVMs {
		return VM{}, ErrNoCapability
	}
	machineType := pickMachineType(capability)
	if machineType == "" {
		return VM{}, fmt.Errorf("this host's QEMU offers no usable machine type")
	}

	vm := s.importedVM(record, req, machineType)
	if err := validateImportedVM(vm, req); err != nil {
		return VM{}, err
	}
	if err := s.insertVM(ctx, vm); err != nil {
		return VM{}, err
	}
	rollback := func() {
		if _, err := s.db.Exec(context.Background(),
			`DELETE FROM plugin_addon_kvm_vms WHERE id = ?`, vm.ID); err != nil {
			s.logger.Warn("kvm: rolling back a failed import", "vm", vm.ID, "error", err)
		}
	}

	source, release, err := s.importSource(record, staged, id)
	if err != nil {
		rollback()
		return VM{}, err
	}
	defer release()

	runCtx, cancel := context.WithTimeout(context.Background(), importRunTimeout)
	// The outcome is written after a run that outlives this request, so the writes keep
	// the request's values but not its cancellation.
	writeCtx := context.WithoutCancel(ctx)
	if _, err := s.importRuns.start(id, "conversion", cancel); err != nil {
		cancel()
		rollback()
		return VM{}, err
	}
	defer func() {
		s.importRuns.finish(id)
		cancel()
	}()

	s.setImportStateProgress(writeCtx, id, ImportConverting, "", 0)

	source.PoolID = req.PoolID
	source.PoolBackend = req.PoolBackend
	source.PoolTarget = req.PoolTarget
	source.VMID = vm.ID
	source.Format = "qcow2"

	// Stopped as soon as the call returns, before the outcome is written: a
	// late poll must not land a lower percentage over the finished row's 100.
	stopWatch := s.watchImportProgress(id, req.EnvironmentID, record.Inspection.Disks)
	defer stopWatch()
	reply, err := s.call(runCtx, req.EnvironmentID, msgVMImportConvert, agentPayload{
		VMID:   vm.ID,
		Import: &source,
	})
	stopWatch()
	if err != nil {
		rollback()
		s.setImportState(writeCtx, id, ImportFailed, err.Error())
		s.auditConvertFailed(id, vm.ID, err.Error())
		return VM{}, err
	}
	if reply.ImportResult == nil || len(reply.ImportResult.Disks) == 0 {
		rollback()
		s.setImportState(writeCtx, id, ImportFailed, "the host converted no disks")
		s.auditConvertFailed(id, vm.ID, "the host converted no disks")
		return VM{}, fmt.Errorf("the host converted no disks")
	}

	for _, disk := range reply.ImportResult.Disks {
		if disk.VolumePath == "" {
			continue
		}
		recorded, diskErr := s.recordImportedDisk(writeCtx, vm, req, disk)
		if diskErr != nil {
			rollback()
			s.setImportState(writeCtx, id, ImportFailed, diskErr.Error())
			s.auditConvertFailed(id, vm.ID, diskErr.Error())
			return VM{}, diskErr
		}
		vm.Disks = append(vm.Disks, recorded)
	}

	nic, err := s.attachNIC(writeCtx, vm.ID, NICRequest{
		NetworkID: req.NetworkID, Bridge: req.Bridge, VLANTag: req.VLANTag,
		AttachKind: req.AttachKind, ParentLink: req.ParentLink, LinkMode: req.LinkMode,
		Model: defaultNICModel(vm.GuestOSFamily),
	})
	if err != nil {
		rollback()
		s.setImportState(writeCtx, id, ImportFailed, err.Error())
		s.auditConvertFailed(id, vm.ID, err.Error())
		return VM{}, err
	}
	vm.NICs = []NIC{nic}

	if _, err := s.call(writeCtx, vm.EnvironmentID, msgVMDefine, agentPayload{
		VMID: vm.ID, Spec: s.buildSpec(writeCtx, vm),
	}); err != nil {
		rollback()
		s.setImportState(writeCtx, id, ImportFailed, err.Error())
		s.auditConvertFailed(id, vm.ID, err.Error())
		return VM{}, err
	}

	stamp := time.Now().UTC().Format(time.RFC3339)
	if _, err := s.db.Exec(writeCtx, `
		UPDATE plugin_addon_kvm_imports
		   SET state = ?, progress = 100, vm_id = ?, last_error = NULL, updated_at = ?
		 WHERE id = ?`,
		ImportReady, vm.ID, stamp, id); err != nil {
		s.logger.Warn("kvm: recording a finished import failed", "import", id, "error", err)
	}
	s.recordAudit("plugin.kvm.import.convert_finished", map[string]any{
		"importId": id, "vmId": vm.ID, "name": vm.Name,
		"environmentId": vm.EnvironmentID, "diskCount": len(vm.Disks),
	})
	return vm, nil
}

// auditConvertFailed records a conversion that did not finish.
//
// The request that started it returned long ago, so without this the only
// record is a `last_error` on a row the operator has to already be looking at.
func (s *Service) auditConvertFailed(id, vmID, reason string) {
	s.recordAudit("plugin.kvm.import.convert_failed", map[string]any{
		"importId": id, "vmId": vmID, "error": reason,
	})
}

// importedVM builds the guest row from the inspection and the operator's
// overrides.
//
// The operator wins wherever they answered, because the descriptor describes
// the machine as it was and they are the one deciding what it should be now.
func (s *Service) importedVM(record Import, req ImportConvertRequest, machineType string) VM {
	found := record.Inspection
	now := time.Now().UTC()

	name := strings.TrimSpace(req.Name)
	if name == "" {
		name = strings.TrimSpace(found.Name)
	}
	if name == "" {
		name = strings.TrimSuffix(record.FileName, filepath.Ext(record.FileName))
	}

	vcpus := req.VCPUs
	if vcpus <= 0 {
		vcpus = found.VCPUs
	}
	if vcpus <= 0 {
		vcpus = 2
	}
	memory := req.MemoryMB
	if memory <= 0 {
		memory = found.MemoryMB
	}
	if memory <= 0 {
		memory = 2048
	}

	// Firmware defaults to BIOS rather than to McHarbor's usual UEFI. An
	// imported guest was installed somewhere else, and booting a BIOS
	// installation under UEFI produces a black screen with no diagnostic —
	// whereas the descriptor says so whenever it knows.
	firmware := req.Firmware
	if firmware == "" {
		firmware = found.Firmware
	}
	if firmware == "" {
		firmware = "bios"
	}

	family := defaultString(req.GuestOSFamily, defaultString(found.GuestOSFamily, OSFamilyLinux))
	return VM{
		ID:             xid.New().String(),
		EnvironmentID:  req.EnvironmentID,
		Name:           name,
		Description:    "Imported from " + record.FileName,
		State:          StateDefined,
		GuestOSFamily:  family,
		GuestOSVariant: found.GuestOSVariant,
		VCPUs:          vcpus,
		CPUModel:       "host",
		CPUSockets:     1,
		CPUCores:       vcpus,
		CPUThreads:     1,
		MemoryMB:       memory,
		MachineType:    machineType,
		Firmware:       firmware,
		SecureBoot:     req.SecureBoot,
		TPMEnabled:     req.TPMEnabled,
		DisplayType:    defaultDisplay(family),
		Autostart:      req.Autostart,
		UUID:           uuid.NewString(),
		CreatedAt:      now,
		UpdatedAt:      now,
	}
}

// validateImportedVM applies the same floor a hand-made guest gets.
//
// The same validators the create form uses, not a looser copy: a guest that
// arrived through an import is still a guest, and an OVF that asked for 64 MB
// of memory should be refused here rather than fail to boot later.
func validateImportedVM(vm VM, req ImportConvertRequest) error {
	if err := validateGuestName(vm.Name); err != nil {
		return err
	}
	if err := validateVCPUs(vm.VCPUs); err != nil {
		return err
	}
	if err := validateMemory(vm.MemoryMB, 0); err != nil {
		return err
	}
	if err := validateFirmware(vm.Firmware, vm.SecureBoot); err != nil {
		return err
	}
	if req.PoolID == "" || req.PoolBackend == "" || req.PoolTarget == "" {
		return invalid(codePoolRequired, "a storage pool is required")
	}
	if req.NetworkID == "" || !hasAttachment(req.AttachKind, req.Bridge, req.ParentLink) {
		return invalid(codeNetworkRequired, "a network is required")
	}
	return nil
}

// recordImportedDisk stores one converted volume.
//
// The first disk is the boot disk: an OVF lists its disks in the order the
// original guest booted them, and the agent preserves that order.
func (s *Service) recordImportedDisk(ctx context.Context, vm VM, req ImportConvertRequest, disk ImportDisk) (Disk, error) {
	now := time.Now().UTC()
	stored := Disk{
		ID:          xid.New().String(),
		VMID:        vm.ID,
		PoolID:      req.PoolID,
		PoolBackend: req.PoolBackend,
		PoolTarget:  req.PoolTarget,
		Slot:        disk.Index,
		Bus:         defaultDiskBus(vm.GuestOSFamily),
		VolumePath:  disk.VolumePath,
		Format:      "qcow2",
		SizeBytes:   disk.VirtualSize,
		CacheMode:   "none",
		Discard:     true,
		IsBoot:      disk.Index == 0,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	if _, err := s.db.Exec(ctx, `
		INSERT INTO plugin_addon_kvm_disks
			(id, vm_id, pool_id, pool_backend, pool_target, slot, bus, volume_path,
			 format, size_bytes, backing_path, cache_mode, discard, readonly,
			 is_boot, is_cdrom, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, NULL, ?, 1, 0, ?, 0, ?, ?)`,
		stored.ID, stored.VMID, stored.PoolID, stored.PoolBackend, stored.PoolTarget,
		stored.Slot, stored.Bus, stored.VolumePath, stored.Format, stored.SizeBytes,
		stored.CacheMode, boolToInt(stored.IsBoot),
		now.Format(time.RFC3339), now.Format(time.RFC3339)); err != nil {
		return Disk{}, fmt.Errorf("recording an imported disk: %w", err)
	}
	return stored, nil
}

// DeleteImport removes an import and its staged file.
//
// The guest it produced is left alone: it is a machine now, not an import.
func (s *Service) DeleteImport(ctx context.Context, id string) error {
	record, staged, err := s.GetImport(ctx, id)
	if err != nil {
		return err
	}
	s.discardOnHost(ctx, record)
	if _, err := s.db.Exec(ctx,
		`DELETE FROM plugin_addon_kvm_imports WHERE id = ?`, id); err != nil {
		return fmt.Errorf("deleting an import: %w", err)
	}
	// Only inside staging, whatever the column says: the path comes from a
	// database row, and a row reading "/etc/passwd" must resolve nowhere.
	if staged != "" && strings.HasPrefix(filepath.Clean(staged), s.importStagingRoot()) {
		if err := os.Remove(staged); err != nil && !errors.Is(err, os.ErrNotExist) {
			s.logger.Warn("kvm: removing a staged import failed", "path", staged, "error", err)
		}
	}
	return nil
}
