// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package qmp

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/therealmcsparrow/mcharbor-agent/internal/errlog"
)

// This file is the typed command surface: a Go func and a Go result shape per
// QMP command McHarbor uses, so `query-status` is not a map[string]any at the
// call site. The set is deliberately closed — a command that is not here is a
// command the product does not issue.

// StatusResult is the answer to query-status.
type StatusResult struct {
	Running    bool   `json:"running"`
	Singlestep bool   `json:"singlestep"`
	Status     string `json:"status"` // running | paused | shutdown | prelaunch | postmigrate | ...
}

// Status queries the guest's run state. This is the authoritative source for a
// VM's state; events are only a prompt to re-read it.
func (m *Monitor) Status(ctx context.Context) (StatusResult, error) {
	var out StatusResult
	err := m.RunInto(ctx, "query-status", nil, &out)
	return out, err
}

// CPUInfo is one vCPU as reported by query-cpus-fast.
type CPUInfo struct {
	CPUIndex int    `json:"cpu-index"`
	ThreadID int    `json:"thread-id"`
	Target   string `json:"target"`
	QOMPath  string `json:"qom-path"`
}

// CPUs lists the guest's vCPUs and their host thread ids.
func (m *Monitor) CPUs(ctx context.Context) ([]CPUInfo, error) {
	var out []CPUInfo
	err := m.RunInto(ctx, "query-cpus-fast", nil, &out)
	return out, err
}

// MemorySizeSummary is the answer to query-memory-size-summary.
type MemorySizeSummary struct {
	BaseMemory    int64 `json:"base-memory"`
	PluggedMemory int64 `json:"plugged-memory,omitempty"`
}

// MemorySummary reports the guest's configured memory.
func (m *Monitor) MemorySummary(ctx context.Context) (MemorySizeSummary, error) {
	var out MemorySizeSummary
	err := m.RunInto(ctx, "query-memory-size-summary", nil, &out)
	return out, err
}

// BalloonResult is the answer to query-balloon: how much memory the guest is
// actually holding, when a virtio-balloon device is present.
type BalloonResult struct {
	Actual int64 `json:"actual"`
}

// Balloon reads the current balloon target. It fails on a guest with no balloon
// device, which the caller treats as "no reclaim reporting" rather than an
// error worth surfacing.
func (m *Monitor) Balloon(ctx context.Context) (BalloonResult, error) {
	var out BalloonResult
	err := m.RunInto(ctx, "query-balloon", nil, &out)
	return out, err
}

// SetBalloon sets the balloon target in bytes.
func (m *Monitor) SetBalloon(ctx context.Context, bytes int64) error {
	_, err := m.Run(ctx, "balloon", map[string]interface{}{"value": bytes})
	return err
}

// BlockDevice is one block device from query-block.
type BlockDevice struct {
	Device    string `json:"device"`
	QDev      string `json:"qdev,omitempty"`
	Removable bool   `json:"removable"`
	Inserted  *struct {
		File        string `json:"file"`
		NodeName    string `json:"node-name,omitempty"`
		BackingFile string `json:"backing_file,omitempty"`
		VirtualSize int64  `json:"virtual-size"`
		ActualSize  int64  `json:"actual-size,omitempty"`
		Format      string `json:"format,omitempty"`
		ReadOnly    bool   `json:"ro"`
	} `json:"inserted,omitempty"`
}

// Blocks lists the guest's block devices.
func (m *Monitor) Blocks(ctx context.Context) ([]BlockDevice, error) {
	var out []BlockDevice
	err := m.RunInto(ctx, "query-block", nil, &out)
	return out, err
}

// BlockStat is one device's IO counters from query-blockstats.
type BlockStat struct {
	Device string `json:"device"`
	Stats  struct {
		ReadBytes  int64 `json:"rd_bytes"`
		WriteBytes int64 `json:"wr_bytes"`
		ReadOps    int64 `json:"rd_operations"`
		WriteOps   int64 `json:"wr_operations"`
	} `json:"stats"`
}

// BlockStats samples per-device IO counters for the metrics pipeline.
func (m *Monitor) BlockStats(ctx context.Context) ([]BlockStat, error) {
	var out []BlockStat
	err := m.RunInto(ctx, "query-blockstats", nil, &out)
	return out, err
}

// ResizeBlock grows a block device that is attached to a running guest.
//
// Shrink is not expressible through this API by design: the service layer
// refuses it before reaching here, because shrinking a live filesystem from
// outside destroys data and a warning is not enough.
func (m *Monitor) ResizeBlock(ctx context.Context, nodeName string, sizeBytes int64) error {
	if sizeBytes <= 0 {
		return fmt.Errorf("qmp: block_resize: refusing non-positive size %d", sizeBytes)
	}
	_, err := m.Run(ctx, "block_resize", map[string]interface{}{
		"node-name": nodeName,
		"size":      sizeBytes,
	})
	return err
}

// SnapshotSync takes an external qcow2 snapshot of one device: the current file
// becomes the backing file and writes continue into a new overlay. This is the
// `dir` pool's snapshot mechanism; CoW pools use their own driver instead.
//
// overlayNode must be a fresh node name. QEMU refuses `node-name` without
// `snapshot-node-name` outright — "New overlay node-name missing" — so the two
// are one argument as far as callers are concerned.
func (m *Monitor) SnapshotSync(ctx context.Context, nodeName, overlayNode, overlayPath string) error {
	_, err := m.Run(ctx, "blockdev-snapshot-sync", map[string]interface{}{
		"node-name":          nodeName,
		"snapshot-node-name": overlayNode,
		"snapshot-file":      overlayPath,
		"format":             "qcow2",
	})
	return err
}

// BlockNode is one entry of query-named-block-nodes.
//
// Backing files opened implicitly are reported here under generated node names
// such as `#block0123`, which is what makes an intermediate layer addressable
// without having been named on the command line.
type BlockNode struct {
	NodeName string `json:"node-name"`
	File     string `json:"file"`
	Drv      string `json:"drv"`
	Image    struct {
		Filename    string `json:"filename"`
		Format      string `json:"format"`
		BackingFile string `json:"backing-filename"`
	} `json:"image"`
	BackingFileDepth int    `json:"backing_file_depth"`
	BackingNode      string `json:"backing_file"`
}

// BlockNodes lists every open block node, backing layers included.
func (m *Monitor) BlockNodes(ctx context.Context) ([]BlockNode, error) {
	var out []BlockNode
	err := m.RunInto(ctx, "query-named-block-nodes", nil, &out)
	return out, err
}

// NodeForFile finds the open format node whose image is path.
//
// Matching on the filename rather than a remembered node name is what survives
// a chain that has already moved: after a live snapshot the guest's top layer
// is a node QEMU named, not the `blk<slot>` from the command line.
func (m *Monitor) NodeForFile(ctx context.Context, path string) (BlockNode, error) {
	nodes, err := m.BlockNodes(ctx)
	if err != nil {
		return BlockNode{}, err
	}
	for _, node := range nodes {
		// The file-level node carries the same filename as its format node, so
		// the format layer is selected explicitly; committing a `file` node is
		// not an operation QEMU offers.
		if node.Image.Filename == path && node.Drv != "file" && node.Drv != "host_device" {
			return node, nil
		}
	}
	return BlockNode{}, fmt.Errorf("%w: no open block node for %s", ErrNotFound, path)
}

// TopNodeForFile finds the node the guest is actually writing to, starting from
// a file somewhere in its chain.
//
// The server's record of a guest's active file can be one layer behind: a live
// snapshot that failed after QEMU had already created the overlay leaves the
// guest writing to a file McHarbor never recorded, and every snapshot after it
// then asks to snapshot a node that is now a backing file — which QEMU refuses
// with "is busy: node is used as backing hd of ...". Walking up the chain from
// whatever path we were given lands on the real top either way, and the caller
// reports it back so the record catches up.
func (m *Monitor) TopNodeForFile(ctx context.Context, path string) (BlockNode, error) {
	nodes, err := m.BlockNodes(ctx)
	if err != nil {
		return BlockNode{}, err
	}

	var current BlockNode
	found := false
	for _, node := range nodes {
		if node.Image.Filename == path && node.Drv != "file" && node.Drv != "host_device" {
			current, found = node, true
			break
		}
	}
	if !found {
		return BlockNode{}, fmt.Errorf("%w: no open block node for %s", ErrNotFound, path)
	}

	// Bounded by the number of open nodes: a chain cannot be longer than the
	// list, and a cycle would otherwise spin here forever.
	for range nodes {
		next, ok := nodeBackedBy(nodes, current.Image.Filename)
		if !ok {
			break
		}
		current = next
	}
	return current, nil
}

// nodeBackedBy returns the format node whose backing file is filename.
func nodeBackedBy(nodes []BlockNode, filename string) (BlockNode, bool) {
	for _, node := range nodes {
		if node.Drv == "file" || node.Drv == "host_device" {
			continue
		}
		if node.Image.BackingFile == filename {
			return node, true
		}
	}
	return BlockNode{}, false
}

// NodeAbove finds the open node whose backing file is path — the layer the
// guest writes through to reach it.
//
// Asked for by the file below rather than by the file above, because the path
// the server has for the upper layer can be stale while the chain itself is
// exactly what QEMU says it is.
func (m *Monitor) NodeAbove(ctx context.Context, path string) (BlockNode, error) {
	nodes, err := m.BlockNodes(ctx)
	if err != nil {
		return BlockNode{}, err
	}
	node, ok := nodeBackedBy(nodes, path)
	if !ok {
		return BlockNode{}, fmt.Errorf("%w: nothing open is backed by %s", ErrNotFound, path)
	}
	return node, nil
}

// InChain reports whether a file is open in the guest's current chain.
func (m *Monitor) InChain(ctx context.Context, path string) bool {
	_, err := m.NodeForFile(ctx, path)
	return err == nil
}

// ErrNotFound reports that the monitor knows nothing by that name.
var ErrNotFound = errors.New("qmp: not found")

// BlockCommit folds one layer of a backing chain into the layer below it.
//
// This is the live equivalent of `qemu-img rebase`: QEMU owns the write lock on
// every file in a running guest's chain, so a snapshot can only be deleted
// underneath it by asking QEMU to do the merge.
//
// topNode empty commits the active layer, which QEMU cannot finish on its own —
// it reaches BLOCK_JOB_READY and waits for the caller to pivot.
func (m *Monitor) BlockCommit(ctx context.Context, jobID, device, topNode, baseNode string) error {
	args := map[string]interface{}{
		"job-id": jobID,
		"device": device,
		// Kept until the caller has read its final state, so a job that failed
		// is still reportable rather than vanishing with the error.
		"auto-dismiss": false,
	}
	if topNode != "" {
		args["top-node"] = topNode
	}
	if baseNode != "" {
		args["base-node"] = baseNode
	}
	_, err := m.Run(ctx, "block-commit", args)
	return err
}

// BlockJobComplete pivots a ready active-layer commit onto its base.
func (m *Monitor) BlockJobComplete(ctx context.Context, jobID string) error {
	_, err := m.Run(ctx, "block-job-complete", map[string]interface{}{"device": jobID})
	return err
}

// BlockJobDismiss drops a finished job from the job list.
func (m *Monitor) BlockJobDismiss(ctx context.Context, jobID string) error {
	_, err := m.Run(ctx, "block-job-dismiss", map[string]interface{}{"id": jobID})
	return err
}

// BlockJob is one entry of query-block-jobs.
type BlockJob struct {
	Device string `json:"device"`
	Type   string `json:"type"`
	Len    int64  `json:"len"`
	Offset int64  `json:"offset"`
	Status string `json:"status"` // created | running | ready | pending | concluded | null
	Error  string `json:"error,omitempty"`
}

// BlockJobs lists the block jobs QEMU is running.
func (m *Monitor) BlockJobs(ctx context.Context) ([]BlockJob, error) {
	var out []BlockJob
	err := m.RunInto(ctx, "query-block-jobs", nil, &out)
	return out, err
}

// AwaitBlockJob drives one commit to completion.
//
// Polled rather than driven off events, because the event channel is shared and
// a snapshot delete must not depend on nobody else having drained it. pivot
// says whether this is an active-layer commit, which stops at `ready` and needs
// an explicit completion before it concludes.
func (m *Monitor) AwaitBlockJob(ctx context.Context, jobID string, pivot bool, onProgress func(offset, total int64)) error {
	completed := false
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(250 * time.Millisecond):
		}

		jobs, err := m.BlockJobs(ctx)
		if err != nil {
			return fmt.Errorf("qmp: polling the block job: %w", err)
		}
		var job *BlockJob
		for i := range jobs {
			if jobs[i].Device == jobID {
				job = &jobs[i]
				break
			}
		}
		if job == nil {
			// Gone without a concluded state: the job finished and was reaped
			// before this poll saw it, which is success.
			return nil
		}
		// Reported every poll. A multi-gigabyte copy that says nothing for
		// twenty minutes is indistinguishable from one that has wedged, and
		// the operator's only recourse is to guess.
		if onProgress != nil {
			onProgress(job.Offset, job.Len)
		}

		switch job.Status {
		case "ready":
			if !pivot || completed {
				continue
			}
			if err := m.BlockJobComplete(ctx, jobID); err != nil {
				return fmt.Errorf("qmp: completing the block job: %w", err)
			}
			completed = true
		case "concluded", "null":
			if job.Error != "" {
				errlog.Log(m.BlockJobDismiss(ctx, jobID))
				return fmt.Errorf("qmp: the block job failed: %s", job.Error)
			}
			return m.BlockJobDismiss(ctx, jobID)
		}
	}
}

// NetRxFilter is one NIC's receive filter, which carries the MAC the guest has
// actually programmed. Guest-reported and therefore display-only.
type NetRxFilter struct {
	Name string `json:"name"`
	MAC  string `json:"main-mac"`
}

// RxFilters reads per-NIC receive filters.
func (m *Monitor) RxFilters(ctx context.Context) ([]NetRxFilter, error) {
	var out []NetRxFilter
	err := m.RunInto(ctx, "query-rx-filter", nil, &out)
	return out, err
}

// PCIDevice is one device on a PCI bus, as query-pci reports it.
type PCIDevice struct {
	Bus      int    `json:"bus"`
	Slot     int    `json:"slot"`
	Function int    `json:"function"`
	QdevID   string `json:"qdev_id"`
}

// PCIBus is one bus and the devices on it.
type PCIBus struct {
	Bus     int         `json:"bus"`
	Devices []PCIDevice `json:"devices"`
}

// PCISlots maps each device's qdev id to the slot QEMU gave it, as the 0x02
// form the command line takes.
//
// Read once after a guest starts so the slot can be pinned from then on:
// QEMU allocates in command-line order, so adding any device ahead of a NIC
// silently renames the guest's interface and costs it its address.
func (m *Monitor) PCISlots(ctx context.Context) (map[string]string, error) {
	var buses []PCIBus
	if err := m.RunInto(ctx, "query-pci", nil, &buses); err != nil {
		return nil, err
	}

	slots := make(map[string]string)
	for _, bus := range buses {
		for _, device := range bus.Devices {
			if device.QdevID == "" {
				continue
			}
			slots[device.QdevID] = fmt.Sprintf("0x%02x", device.Slot)
		}
	}
	return slots, nil
}

// Powerdown sends an ACPI power button press. The guest decides what to do with
// it, which is why the caller pairs this with a grace period and an escalation
// to Quit.
func (m *Monitor) Powerdown(ctx context.Context) error {
	_, err := m.Run(ctx, "system_powerdown", nil)
	return err
}

// Reset hard-resets the guest, equivalent to the reset button.
func (m *Monitor) Reset(ctx context.Context) error {
	_, err := m.Run(ctx, "system_reset", nil)
	return err
}

// Pause stops the guest's vCPUs. The guest keeps its memory and resumes exactly
// where it stopped.
func (m *Monitor) Pause(ctx context.Context) error {
	_, err := m.Run(ctx, "stop", nil)
	return err
}

// Resume restarts stopped vCPUs.
func (m *Monitor) Resume(ctx context.Context) error {
	_, err := m.Run(ctx, "cont", nil)
	return err
}

// Quit terminates QEMU immediately. This is the escalation after an ACPI grace
// period elapses, and the caller records that the escalation happened.
func (m *Monitor) Quit(ctx context.Context) error {
	_, err := m.Run(ctx, "quit", nil)
	// QEMU exits without answering, so the monitor's socket closing is the
	// expected outcome rather than a failure.
	if err != nil && (strings.Contains(err.Error(), "closed") || IsClosed(err)) {
		return nil
	}
	return err
}

// AddDevice hotplugs a device. Whether this is available at all depends on the
// guest and the QEMU version, which the caller checks with Supports before
// offering the action.
func (m *Monitor) AddDevice(ctx context.Context, args map[string]interface{}) error {
	_, err := m.Run(ctx, "device_add", args)
	return err
}

// DelDevice unplugs a device by its qdev id. Removal is a request the guest may
// ignore; the caller confirms with query-pci rather than assuming.
func (m *Monitor) DelDevice(ctx context.Context, id string) error {
	_, err := m.Run(ctx, "device_del", map[string]interface{}{"id": id})
	return err
}

// AddNetdev attaches a host network backend to a running guest.
func (m *Monitor) AddNetdev(ctx context.Context, args map[string]interface{}) error {
	_, err := m.Run(ctx, "netdev_add", args)
	return err
}

// DelNetdev removes a host network backend by id.
func (m *Monitor) DelNetdev(ctx context.Context, id string) error {
	_, err := m.Run(ctx, "netdev_del", map[string]interface{}{"id": id})
	return err
}

// MigrateParameters bounds a migration. Both fields are advisory targets QEMU
// works toward rather than guarantees.
type MigrateParameters struct {
	MaxBandwidth  int64 `json:"max-bandwidth,omitempty"`  // bytes/second
	DowntimeLimit int64 `json:"downtime-limit,omitempty"` // milliseconds
}

// SetMigrateParameters applies bandwidth and downtime targets before a
// migration starts.
func (m *Monitor) SetMigrateParameters(ctx context.Context, p MigrateParameters) error {
	args := map[string]interface{}{}
	if p.MaxBandwidth > 0 {
		args["max-bandwidth"] = p.MaxBandwidth
	}
	if p.DowntimeLimit > 0 {
		args["downtime-limit"] = p.DowntimeLimit
	}
	if len(args) == 0 {
		return nil
	}
	_, err := m.Run(ctx, "migrate-set-parameters", args)
	return err
}

// Migrate starts an outbound migration to a URI such as tcp:host:port.
func (m *Monitor) Migrate(ctx context.Context, uri string, blockMigration bool) error {
	args := map[string]interface{}{"uri": uri}
	if blockMigration {
		// Non-shared storage: QEMU carries the disks too, which is slow and is
		// labelled as such in the UI rather than chosen silently.
		args["blk"] = true
	}
	_, err := m.Run(ctx, "migrate", args)
	return err
}

// MigrateStatus is the answer to query-migrate.
type MigrateStatus struct {
	Status string `json:"status"` // setup | active | completed | failed | cancelled
	RAM    *struct {
		Transferred int64 `json:"transferred"`
		Remaining   int64 `json:"remaining"`
		Total       int64 `json:"total"`
	} `json:"ram,omitempty"`
	Disk *struct {
		Transferred int64 `json:"transferred"`
		Remaining   int64 `json:"remaining"`
		Total       int64 `json:"total"`
	} `json:"disk,omitempty"`
}

// Percent reports migration progress across both RAM and disk, or -1 when QEMU
// has not published enough to say.
func (s MigrateStatus) Percent() int {
	var done, total int64
	if s.RAM != nil {
		done += s.RAM.Transferred
		total += s.RAM.Total
	}
	if s.Disk != nil {
		done += s.Disk.Transferred
		total += s.Disk.Total
	}
	if total <= 0 {
		return -1
	}
	pct := int(done * 100 / total)
	if pct > 100 {
		return 100
	}
	return pct
}

// MigrateStatus polls migration progress.
func (m *Monitor) MigrateStatus(ctx context.Context) (MigrateStatus, error) {
	var out MigrateStatus
	err := m.RunInto(ctx, "query-migrate", nil, &out)
	return out, err
}

// MigrateCancel aborts an in-flight migration.
func (m *Monitor) MigrateCancel(ctx context.Context) error {
	_, err := m.Run(ctx, "migrate_cancel", nil)
	return err
}

// SaveMemoryState writes the guest's full machine state to a file and waits for
// it to finish.
//
// QEMU has no "savevm to a path" command over QMP — `savevm` is an HMP command
// and writes into a qcow2's own snapshot area, which does not work for a guest
// whose disks are on zfs or lvm. Migrating to a file is the supported way to
// get the same thing, and it works whatever the disks are.
//
// The guest must already be paused: migration of a running guest converges by
// re-copying dirty pages, and for a snapshot there is nothing to converge
// towards. The caller pauses and decides whether to resume.
func (m *Monitor) SaveMemoryState(ctx context.Context, path string) error {
	// `exec:` rather than the `file:` URI, which only exists from QEMU 8.2.
	// This path has to work on whatever the operator's distribution ships.
	if err := m.SetMigrateParameters(ctx, MigrateParameters{}); err != nil {
		// Not fatal: the defaults are fine for a local file, and an older QEMU
		// may not accept every parameter. The save itself is what matters.
		_ = err
	}
	if _, err := m.Run(ctx, "migrate", map[string]interface{}{
		"uri": "exec:cat > " + shellSingleQuote(path),
	}); err != nil {
		return fmt.Errorf("qmp: starting the memory-state save: %w", err)
	}

	for {
		select {
		case <-ctx.Done():
			// Leave nothing running behind a cancelled save: an abandoned
			// migration keeps QEMU in a state where the next one is refused.
			cancelCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
			errlog.Log(m.MigrateCancel(cancelCtx))
			cancel()
			return ctx.Err()
		case <-time.After(250 * time.Millisecond):
		}

		status, err := m.MigrateStatus(ctx)
		if err != nil {
			return fmt.Errorf("qmp: polling the memory-state save: %w", err)
		}
		switch status.Status {
		case "completed":
			return nil
		case "failed":
			return errors.New("qmp: the memory-state save failed")
		case "cancelled":
			return errors.New("qmp: the memory-state save was cancelled")
		}
	}
}

// shellSingleQuote wraps a path for the shell QEMU's `exec:` migration spawns.
//
// The path is McHarbor's own — a runtime directory joined with a validated
// snapshot name — but it reaches a shell, and a path that is safe today is not
// a reason to hand the next one over unquoted.
func shellSingleQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", `'\''`) + "'"
}

// IsClosed reports whether an error means the monitor is gone rather than that
// a command was refused.
func IsClosed(err error) bool {
	return err != nil && strings.Contains(err.Error(), ErrClosed.Error())
}

// AsError extracts the QMP error from an error chain, so a caller can match on
// Class. It returns nil when the failure was transport-level rather than a
// command QEMU refused.
func AsError(err error) *Error {
	var qe *Error
	if errors.As(err, &qe) {
		return qe
	}
	return nil
}
