// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// importProgressInterval is how often a running transfer writes its byte count
// back to the row. A 40 GB pull produces millions of writes through the copy
// buffer, and persisting each one would spend the import's whole duration in
// SQLite.
const importProgressInterval = 2 * time.Second

// importRunTimeout bounds one inspect or convert. A 40 GB pull over the agent
// transport on a slow link genuinely takes hours, and the agent gives itself
// six; the server allows a little more so the agent's own error wins the race.
const importRunTimeout = 8 * time.Hour

// importProgressPoll is how often a running import reads the progress the bridge
// keeps from the agent. A variable so a test does not wait out real seconds.
var importProgressPoll = importProgressInterval

// Progress phases, in the order an import passes through them.
const (
	importPhasePull    = "pull"
	importPhaseConvert = "convert"
)

// importJobs tracks the inspect and convert runs that are in flight.
//
// A job outlives the HTTP request that asked for it — an operator who closes
// the dialog has not cancelled a 40 GB transfer — so the cancel func is the
// only handle anything has on it afterwards.
type importJobs struct {
	mu   sync.Mutex
	jobs map[string]*importJob
}

// importJob is one running inspect or convert.
type importJob struct {
	cancel     context.CancelFunc
	kind       string
	started    time.Time
	transfered atomic.Int64

	// progressMu guards the furthest point written to the row, so the pull byte
	// count and the agent's own reports, which race each other, never move the
	// percentage backwards.
	progressMu sync.Mutex
	phase      string
	percent    int
}

// importPhaseRank orders the phases; an unknown phase ranks zero and is ignored.
func importPhaseRank(phase string) int {
	switch phase {
	case importPhasePull:
		return 1
	case importPhaseConvert:
		return 2
	default:
		return 0
	}
}

// advance records percent for phase and reports whether it moved the job
// forward: a later phase starts again from its own percentage, an earlier phase
// or a lower percentage in the same phase is refused.
func (job *importJob) advance(phase string, percent int) bool {
	rank := importPhaseRank(phase)
	if rank == 0 {
		return false
	}
	job.progressMu.Lock()
	defer job.progressMu.Unlock()
	current := importPhaseRank(job.phase)
	if rank < current || (rank == current && percent <= job.percent) {
		return false
	}
	job.phase, job.percent = phase, percent
	return true
}

func newImportJobs() *importJobs {
	return &importJobs{jobs: make(map[string]*importJob)}
}

// start registers a job, refusing a second one for the same import.
func (j *importJobs) start(id, kind string, cancel context.CancelFunc) (*importJob, error) {
	j.mu.Lock()
	defer j.mu.Unlock()
	if existing, ok := j.jobs[id]; ok {
		return nil, fmt.Errorf("%w: a %s is already running for this import", ErrImportState, existing.kind)
	}
	job := &importJob{cancel: cancel, kind: kind, started: time.Now()}
	j.jobs[id] = job
	return job, nil
}

// finish drops a job once it has ended, however it ended.
func (j *importJobs) finish(id string) {
	j.mu.Lock()
	delete(j.jobs, id)
	j.mu.Unlock()
}

// get returns a running job, or nil.
func (j *importJobs) get(id string) *importJob {
	j.mu.Lock()
	defer j.mu.Unlock()
	return j.jobs[id]
}

// cancel stops a running job and reports whether there was one.
func (j *importJobs) cancel(id string) bool {
	j.mu.Lock()
	job, ok := j.jobs[id]
	j.mu.Unlock()
	if !ok {
		return false
	}
	job.cancel()
	return true
}

// addTransferred records bytes served for an import and returns the new total.
func (s *Service) addTransferred(id string, n int64) int64 {
	job := s.importRuns.get(id)
	if job == nil {
		return 0
	}
	return job.transfered.Add(n)
}

// advanceImportProgress writes percent to a running import's row, but only when
// it moves the import forward within its phase.
func (s *Service) advanceImportProgress(id, phase string, percent int) {
	job := s.importRuns.get(id)
	if job == nil {
		return
	}
	percent = min(max(percent, 0), 100)
	if job.advance(phase, percent) {
		s.setImportProgress(id, percent)
	}
}

// watchImportProgress polls the bridge's recorded progress for a running import
// and returns the func that stops it; stop waits for the poller to exit and is
// safe to call more than once. disks spreads a multi-disk conversion over one bar.
func (s *Service) watchImportProgress(id, envID string, disks []ImportDisk) func() {
	source := s.rpc
	if source == nil {
		return func() {}
	}
	done := make(chan struct{})
	exited := make(chan struct{})
	// goroutine lifecycle: ends when the returned stop func closes done.
	go func() {
		defer close(exited)
		ticker := time.NewTicker(importProgressPoll)
		defer ticker.Stop()
		for {
			select {
			case <-done:
				return
			case <-ticker.C:
				phase, disk, percent, known := source.ImportProgress(envID, id)
				if !known {
					continue
				}
				if phase == importPhaseConvert {
					percent = overallConvertPercent(disks, disk, percent)
				}
				s.advanceImportProgress(id, phase, percent)
			}
		}
	}()
	var once sync.Once
	return func() {
		once.Do(func() {
			close(done)
			<-exited
		})
	}
}

// overallConvertPercent turns one disk's percentage into the whole conversion's.
// The agent restarts at zero for every disk, which on its own would read as the
// import going backwards.
func overallConvertPercent(disks []ImportDisk, disk, percent int) int {
	if len(disks) < 2 {
		return percent
	}
	for position, candidate := range disks {
		if candidate.Index == disk {
			return (position*100 + percent) / len(disks)
		}
	}
	return percent
}

// setImportProgress stores a percentage on the row.
//
// Percent rather than bytes because the row already carries SizeBytes, and a
// reader that has both has to decide which one to trust when a retry restarts
// the transfer from zero.
func (s *Service) setImportProgress(id string, percent int) {
	if percent < 0 {
		percent = 0
	}
	if percent > 100 {
		percent = 100
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if _, err := s.db.Exec(ctx, `
		UPDATE plugin_addon_kvm_imports
		   SET progress = ?, updated_at = ?
		 WHERE id = ?`, percent, time.Now().UTC().Format(time.RFC3339), id); err != nil {
		s.logger.Debug("kvm: recording import progress failed", "import", id, "error", err)
	}
}

// CancelImport stops a running inspect or convert and returns the import to a
// state the operator can act on again.
func (s *Service) CancelImport(ctx context.Context, id string) (Import, error) {
	record, _, err := s.GetImport(ctx, id)
	if err != nil {
		return Import{}, err
	}
	if !s.importRuns.cancel(id) && record.State != ImportInspecting && record.State != ImportConverting {
		return Import{}, fmt.Errorf("%w: nothing is running for this import", ErrImportState)
	}

	// Back to whichever resting state the record can support: an import that
	// was already inspected keeps its inspection and can be converted again,
	// one that was not returns to the uploaded file it still has on disk.
	next := ImportUploaded
	if record.Inspection != nil {
		next = ImportInspected
	}
	s.setImportStateProgress(ctx, id, next, "cancelled", 0)

	record.State = next
	record.Progress = 0
	record.LastError = "cancelled"
	return record, nil
}

// setImportStateProgress writes state, error and progress in one statement.
func (s *Service) setImportStateProgress(ctx context.Context, id, state, errText string, percent int) {
	var errValue any
	if errText != "" {
		errValue = errText
	}
	if _, err := s.db.Exec(ctx, `
		UPDATE plugin_addon_kvm_imports
		   SET state = ?, last_error = ?, progress = ?, updated_at = ?
		 WHERE id = ?`,
		state, errValue, percent, time.Now().UTC().Format(time.RFC3339), id); err != nil {
		s.logger.Warn("kvm: recording import state failed", "import", id, "error", err)
	}
}

// reconcileImports returns interrupted imports to a usable state at boot.
//
// An inspect or convert lives in memory, so a restart ends it without ending
// the row that describes it. Left alone the record says "inspecting" forever
// and the UI waits on a job that no longer exists — which is exactly what a
// 39 GB VHDX did across a night of rebuilds.
func (s *Service) reconcileImports(ctx context.Context) error {
	rows, err := s.db.Query(ctx, `
		SELECT id, inspection
		  FROM plugin_addon_kvm_imports
		 WHERE state IN (?, ?)
		 LIMIT 1000`, ImportInspecting, ImportConverting)
	if err != nil {
		return fmt.Errorf("reading interrupted imports: %w", err)
	}
	defer func() { _ = rows.Close() }()

	type stuck struct {
		id       string
		hasInspt bool
	}
	pending := make([]stuck, 0, 4)
	for rows.Next() {
		var id string
		var inspection *string
		if err := rows.Scan(&id, &inspection); err != nil {
			return fmt.Errorf("reading an interrupted import: %w", err)
		}
		pending = append(pending, stuck{id: id, hasInspt: inspection != nil && *inspection != ""})
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("reading interrupted imports: %w", err)
	}

	for _, entry := range pending {
		next := ImportUploaded
		if entry.hasInspt {
			next = ImportInspected
		}
		s.setImportStateProgress(ctx, entry.id, next,
			"interrupted by a McHarbor restart; the uploaded file is still staged", 0)
		s.logger.Info("kvm: recovered an interrupted import", "import", entry.id, "state", next)
	}
	s.sweepImportStaging()
	return nil
}

// sweepImportStaging deletes the temporary files of uploads that never
// finished.
//
// ReceiveImport writes under a `.partial-` name and renames only once the whole
// body has arrived, so a partial is by definition an upload whose process is
// gone — a restart during a 40 GB transfer leaves one behind, and nothing ever
// looked at them again. Four of them had accumulated to 84 GB before this
// existed.
func (s *Service) sweepImportStaging() {
	root := s.importStagingRoot()
	entries, err := os.ReadDir(root)
	if err != nil {
		return
	}
	freed := int64(0)
	count := 0
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasPrefix(entry.Name(), ".partial-") {
			continue
		}
		if info, statErr := entry.Info(); statErr == nil {
			freed += info.Size()
		}
		if err := os.Remove(filepath.Join(root, entry.Name())); err != nil {
			s.logger.Warn("kvm: removing an abandoned upload failed",
				"file", entry.Name(), "error", err)
			continue
		}
		count++
	}
	if count > 0 {
		s.logger.Info("kvm: cleared abandoned uploads", "files", count, "bytes", freed)
	}
}

// countingFile is the staged file with a byte counter around it.
//
// The counter sits on the reader rather than the ResponseWriter because
// http.ServeContent needs an io.ReadSeeker, and because the agent's retries
// re-seek: counting reads gives the true position in the file, which a counter
// on the writer could not.
type countingFile struct {
	*os.File
	svc      *Service
	importID string
	total    int64
	last     time.Time
}

// Read forwards to the file and records how far the transfer has got.
func (c *countingFile) Read(p []byte) (int, error) {
	n, err := c.File.Read(p)
	if n > 0 && c.importID != "" && c.total > 0 {
		moved := c.svc.addTransferred(c.importID, int64(n))
		if time.Since(c.last) >= importProgressInterval {
			c.last = time.Now()
			c.svc.advanceImportProgress(c.importID, importPhasePull, int(moved*100/c.total))
		}
	}
	return n, err
}

// Seek resets the counter when the agent restarts the transfer, so a retry
// does not report a percentage built from two passes over the same file.
func (c *countingFile) Seek(offset int64, whence int) (int64, error) {
	pos, err := c.File.Seek(offset, whence)
	if err == nil && c.importID != "" {
		if job := c.svc.importRuns.get(c.importID); job != nil {
			job.transfered.Store(pos)
		}
	}
	return pos, err
}
