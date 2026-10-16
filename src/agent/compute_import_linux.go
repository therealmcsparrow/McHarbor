// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

//go:build linux

package main

import (
	"archive/tar"
	"bufio"
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/therealmcsparrow/mcharbor-agent/compute/storage"
	"github.com/therealmcsparrow/mcharbor-agent/internal/errlog"
)

// Importing a guest from another hypervisor.
//
// VMware, Hyper-V, VirtualBox and Parallels all write disk formats qemu-img
// already reads, so an import is an inspection and a conversion rather than an
// emulation problem. What each product adds on top is a description of the
// machine — an OVF descriptor, in the OVA case — which is where the processor
// count, the memory and the firmware come from instead of the operator copying
// them out of the old UI by hand.
//
// Both halves run here rather than on McHarbor, for the same reason image
// downloads do: a 200 GB VMDK would otherwise cross the WebSocket transport
// twice, and qemu-img is a dependency a compute host already has and the
// McHarbor container deliberately does not.
//
// What is NOT carried over, and is reported as a warning instead of being
// silently dropped: vendor guest tools, paravirtual controllers with no KVM
// equivalent, and any snapshot chain inside the source. A converted disk is the
// guest's current state, flattened.

// importStagingDir is where a pulled or unpacked source lives while it is being
// worked on. Under the compute root so it shares the guest volumes' disk rather
// than filling /tmp or /var with something that can be hundreds of gigabytes.
const importStagingDir = "imports"

// importPullTimeout bounds fetching a staged upload from McHarbor.
const importPullTimeout = 6 * time.Hour

// importConvertTimeout bounds one qemu-img conversion.
const importConvertTimeout = 12 * time.Hour

// importInfoTimeout bounds a qemu-img metadata read.
const importInfoTimeout = 2 * time.Minute

// importArchiveExtensions are the sources unpacked rather than converted.
var importArchiveExtensions = map[string]bool{
	".ova": true, ".tar": true, ".tgz": true,
}

// importDiskExtensions are the disk formats qemu-img reads that this feature
// offers. It is an allow-list rather than "whatever qemu-img accepts": a source
// McHarbor cannot name a hypervisor for is one nobody can support.
var importDiskExtensions = map[string]string{
	".vmdk":  "vmdk",      // VMware
	".vhdx":  "vhdx",      // Hyper-V, current
	".vhd":   "vpc",       // Hyper-V, legacy; qemu calls it vpc
	".vdi":   "vdi",       // VirtualBox
	".hdd":   "parallels", // Parallels
	".hds":   "parallels",
	".qcow2": "qcow2",
	".qcow":  "qcow",
	".img":   "raw",
	".raw":   "raw",
}

// maxOVFBytes bounds the descriptor read out of an archive. An OVF is a few
// kilobytes; anything approaching this is not one.
const maxOVFBytes = 8 << 20

// maxArchiveEntries bounds how many members an archive may carry, so a tar
// bomb is refused rather than unpacked.
const maxArchiveEntries = 4096

// maxArchiveBytes bounds one unpacked archive at 4 TiB.
const maxArchiveBytes int64 = 4 << 40

// Errors this file raises, worded for an operator.
var (
	errImportNoSource = errors.New(
		"compute: the import named no source file")
	errImportUnsupported = errors.New(
		"compute: that file is not a disk image McHarbor can import")
	errImportNoDisks = errors.New(
		"compute: no disks were found in that file")
	errImportOutsideRoot = errors.New(
		"compute: the import source is outside the directories this host allows")
	errImportArchiveTooBig = errors.New(
		"compute: that archive is larger than the import limit")
)

// qemuImgProgress matches the `(52.31/100%)` qemu-img prints with -p.
var qemuImgProgress = regexp.MustCompile(`\(([0-9.]+)/100%\)`)

// importRoot is where this host stages imports.
func importRoot() string {
	return filepath.Join(computeRoot, importStagingDir)
}

// handleVMImport answers the four vm_import_* messages.
func (rt *computeRuntime) handleVMImport(ctx context.Context, messageType string, req *VMPayload, reply *VMPayload) error {
	if req.Import == nil {
		return fmt.Errorf("compute: %s carried no import request", messageType)
	}
	spec := *req.Import

	switch messageType {
	case MsgVMImportStage:
		return rt.stageImport(ctx, spec, reply)
	case MsgVMImportDiscard:
		return rt.discardImport(spec, reply)
	}

	source, cleanup, err := rt.resolveImportSource(ctx, spec)
	if err != nil {
		return err
	}
	defer cleanup()

	result, err := rt.inspectImport(ctx, spec, source)
	if err != nil {
		return err
	}

	if messageType == MsgVMImportInspect {
		reply.ImportResult = result
		return nil
	}
	if err := rt.convertImport(ctx, spec, result); err != nil {
		return err
	}
	reply.ImportResult = result
	return nil
}

// resolveImportSource produces a local path for the source, downloading it when
// McHarbor is serving a staged upload.
//
// The returned cleanup removes only what this call created: a file the operator
// put on the host themselves is left exactly where it was.
func (rt *computeRuntime) resolveImportSource(ctx context.Context, spec VMImportSpec) (string, func(), error) {
	noop := func() {}

	if spec.SourcePath != "" {
		clean := filepath.Clean(spec.SourcePath)
		// The agent runs as root, and this path arrives from the control
		// plane. Confining it to the directories compute owns is what stops an
		// import being a way to read /etc/shadow into a guest disk.
		if !withinAnyRoot(clean, computeRoot, rt.images.Root()) {
			return "", noop, fmt.Errorf("%w: %s", errImportOutsideRoot, clean)
		}
		if _, err := os.Stat(clean); err != nil {
			return "", noop, fmt.Errorf("compute: the import source %s is not readable: %w", clean, err)
		}
		return clean, noop, nil
	}

	if spec.SourceURL == "" {
		return "", noop, errImportNoSource
	}

	target, err := rt.downloadImportSource(ctx, spec)
	if err != nil {
		return "", noop, err
	}
	cleanup := func() {
		root := importDir(spec.ImportID)
		if err := os.RemoveAll(root); err != nil {
			rt.logger.Warn("compute: clearing an import's staging directory failed",
				"dir", root, "error", err)
		}
	}
	return target, cleanup, nil
}

// importDir is where one import's source lives on this host.
func importDir(importID string) string {
	return filepath.Join(importRoot(), filepath.Base(importID))
}

// downloadImportSource pulls the upload from McHarbor into this host's staging
// directory and leaves it there. The caller decides whether it survives.
func (rt *computeRuntime) downloadImportSource(ctx context.Context, spec VMImportSpec) (string, error) {
	root := importDir(spec.ImportID)
	if err := os.MkdirAll(root, 0o700); err != nil {
		return "", fmt.Errorf("compute: create %s: %w", root, err)
	}
	name := filepath.Base(spec.FileName)
	if name == "" || name == "." || name == string(filepath.Separator) {
		name = "source.img"
	}
	target := filepath.Join(root, name)
	if err := rt.pullImportSource(ctx, spec, target); err != nil {
		if removeErr := os.RemoveAll(root); removeErr != nil {
			rt.logger.Warn("compute: clearing a failed import download failed",
				"dir", root, "error", removeErr)
		}
		return "", err
	}
	return target, nil
}

// stageImport answers vm_import_stage: the upload is pulled once and kept, so
// the inspect and the conversion that follow need nothing from McHarbor but the
// instruction to run.
//
// An already-staged source is reported rather than fetched again, which is what
// makes a retry after an interrupted conversion cost nothing.
func (rt *computeRuntime) stageImport(ctx context.Context, spec VMImportSpec, reply *VMPayload) error {
	if existing, err := stagedImportPath(spec.ImportID); err == nil && existing != "" {
		reply.ImportResult = &VMImportResult{ImportID: spec.ImportID, StagedPath: existing}
		return nil
	}
	target, err := rt.downloadImportSource(ctx, spec)
	if err != nil {
		return err
	}
	reply.ImportResult = &VMImportResult{ImportID: spec.ImportID, StagedPath: target}
	return nil
}

// stagedImportPath returns the file already staged for an import, if any.
func stagedImportPath(importID string) (string, error) {
	entries, err := os.ReadDir(importDir(importID))
	if err != nil {
		return "", err
	}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		info, err := entry.Info()
		if err != nil || info.Size() == 0 {
			continue
		}
		return filepath.Join(importDir(importID), entry.Name()), nil
	}
	return "", nil
}

// discardImport answers vm_import_discard: the operator deleted the import, so
// the disk this host was holding for it goes too.
func (rt *computeRuntime) discardImport(spec VMImportSpec, reply *VMPayload) error {
	root := importDir(spec.ImportID)
	if err := os.RemoveAll(root); err != nil {
		return fmt.Errorf("compute: clearing %s: %w", root, err)
	}
	reply.ImportResult = &VMImportResult{ImportID: spec.ImportID}
	return nil
}

// withinAnyRoot reports whether path sits inside one of the given roots.
func withinAnyRoot(path string, roots ...string) bool {
	for _, root := range roots {
		if root == "" {
			continue
		}
		clean := filepath.Clean(root)
		if path == clean || strings.HasPrefix(path, clean+string(filepath.Separator)) {
			return true
		}
	}
	return false
}

// pullImportSource streams a staged upload from McHarbor onto this host.
//
// Straight to disk, never through memory: these are disk images, and there is
// no size an agent can assume is safe to buffer.
func (rt *computeRuntime) pullImportSource(ctx context.Context, spec VMImportSpec, target string) error {
	url := rt.absoluteServerURL(spec.SourceURL)
	if url == "" {
		return fmt.Errorf("compute: the import source URL could not be resolved against this host's McHarbor address")
	}

	ctx, cancel := context.WithTimeout(ctx, importPullTimeout)
	defer cancel()

	request, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return fmt.Errorf("compute: preparing the import download: %w", err)
	}
	if spec.SourceToken != "" {
		request.Header.Set("Authorization", "Bearer "+spec.SourceToken)
	}

	// Logged before the request, not after: when this stalls, the URL it
	// stalled on is the whole diagnosis, and nothing else records it.
	rt.logger.Info("compute: downloading the import source",
		"import", spec.ImportID, "url", url, "target", target)

	started := time.Now()
	resp, err := rt.importClient().Do(request)
	if err != nil {
		return fmt.Errorf("compute: downloading the import source from %s: %w", url, err)
	}
	defer func() { errlog.Close(resp.Body) }()
	if resp.StatusCode != http.StatusOK {
		// The token is single-use, so a 404 here usually means it was already
		// redeemed — worth distinguishing from McHarbor being unreachable.
		return fmt.Errorf("compute: McHarbor answered %s for the import source at %s",
			resp.Status, url)
	}

	file, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		return fmt.Errorf("compute: create %s: %w", target, err)
	}
	defer func() { errlog.Close(file) }()

	written, err := rt.copyImportSource(ctx, spec, file, resp.Body, resp.ContentLength)
	if err != nil {
		return err
	}
	rt.logger.Info("compute: the import source is on this host",
		"import", spec.ImportID, "bytes", written,
		"seconds", int(time.Since(started).Seconds()))
	return file.Sync()
}

// importStallTimeout is how long a started download may produce no bytes at
// all before it is called dead.
//
// Distinct from the overall budget: a transfer that is merely slow keeps
// resetting this, while one whose connection has gone away without closing
// fails in minutes instead of riding the whole import timeout in silence.
const importStallTimeout = 5 * time.Minute

// copyImportSource streams the body to disk, reporting progress and refusing to
// wait forever on a connection that has stopped delivering.
func (rt *computeRuntime) copyImportSource(ctx context.Context, spec VMImportSpec, dst io.Writer, src io.Reader, total int64) (int64, error) {
	buf := make([]byte, 1<<20)
	var written int64
	lastReport := -1
	lastData := time.Now()
	lastTick := time.Now()

	for {
		if err := ctx.Err(); err != nil {
			return written, fmt.Errorf("compute: the import download was cancelled after %d bytes: %w", written, err)
		}
		n, readErr := src.Read(buf)
		if n > 0 {
			if _, writeErr := dst.Write(buf[:n]); writeErr != nil {
				return written, fmt.Errorf("compute: writing the import source: %w", writeErr)
			}
			written += int64(n)
			lastData = time.Now()
		}
		if readErr == io.EOF {
			rt.sendPullProgress(spec, 100)
			return written, nil
		}
		if readErr != nil {
			return written, fmt.Errorf("compute: reading the import source after %d bytes: %w", written, readErr)
		}
		if n == 0 && time.Since(lastData) > importStallTimeout {
			return written, fmt.Errorf(
				"compute: the import download stalled after %d bytes with no data for %s",
				written, importStallTimeout)
		}
		// Throttled to one message a second, and only on a changed percent:
		// the server persists these, and a 37 GB pull would otherwise write a
		// row per megabyte.
		if total > 0 && time.Since(lastTick) >= time.Second {
			lastTick = time.Now()
			if percent := int(written * 100 / total); percent != lastReport {
				lastReport = percent
				rt.sendPullProgress(spec, percent)
			}
		}
	}
}

// sendPullProgress publishes how far the download has got.
//
// The same message the conversion uses, so the UI has one progress channel for
// an import rather than a silent first half and a reported second.
func (rt *computeRuntime) sendPullProgress(spec VMImportSpec, percent int) {
	errlog.Log(rt.send(WSMessage{Type: MsgVMImportProgress, VM: &VMPayload{
		Operation:    MsgVMImportInspect,
		VMID:         spec.VMID,
		Percent:      percent,
		Import:       &VMImportSpec{ImportID: spec.ImportID},
		ImportResult: &VMImportResult{ImportID: spec.ImportID},
	}}))
}

// importClient is the HTTP client used to pull from McHarbor.
//
// The agent's own client, so it honours MCHARBOR_INSECURE exactly as the
// control channel and the image library pull do — McHarbor is commonly reached
// on a LAN address its certificate does not cover, and that has already been
// accepted once for the connection carrying this very request.
//
// http.DefaultClient is never the answer: it verifies certificates whatever
// MCHARBOR_INSECURE says, and it carries no deadlines, so the one case this
// fallback existed for is also the one where it fails worst.
func (rt *computeRuntime) importClient() *http.Client {
	if rt.images != nil {
		if client := rt.images.ServerClient(); client != nil {
			return client
		}
	}
	return rt.serverClient
}

// inspectImport reads what a source contains.
func (rt *computeRuntime) inspectImport(ctx context.Context, spec VMImportSpec, source string) (*VMImportResult, error) {
	result := &VMImportResult{ImportID: spec.ImportID, Kind: "disk"}

	name := spec.FileName
	if name == "" {
		name = filepath.Base(source)
	}
	ext := strings.ToLower(filepath.Ext(name))

	if importArchiveExtensions[ext] {
		result.Kind = "ova"
		if err := rt.inspectArchive(ctx, source, result); err != nil {
			return nil, err
		}
	} else {
		if _, known := importDiskExtensions[ext]; !known {
			return nil, fmt.Errorf("%w: %q", errImportUnsupported, ext)
		}
		info, err := qemuImgInfo(ctx, source)
		if err != nil {
			return nil, err
		}
		result.Disks = []VMImportDisk{{
			Index: 0, SourceFile: source, SourceFormat: info.Format,
			VirtualSize: info.VirtualSize, ActualSize: info.ActualSize,
		}}
	}

	if len(result.Disks) == 0 {
		return nil, errImportNoDisks
	}
	rt.probeBootDisk(ctx, result)
	return result, nil
}

// gptSignature marks LBA 1 of a GUID partition table.
var gptSignature = []byte("EFI PART")

// Partition type GUIDs that only Windows creates, in the mixed-endian form
// they are stored in: the Microsoft Reserved partition and the Windows
// recovery partition.
var windowsPartitionGUIDs = [][]byte{
	{0x16, 0xE3, 0xC9, 0xE3, 0x5C, 0x0B, 0xB8, 0x4D, 0x81, 0x7D, 0xF9, 0x2D, 0xF0, 0x02, 0x15, 0xAE},
	{0xA4, 0xBB, 0x94, 0xDE, 0xD1, 0x06, 0x40, 0x4D, 0xA1, 0x6A, 0xBF, 0xD5, 0x01, 0x79, 0xD6, 0xAC},
}

// probeBootDisk reads the head of the first disk and fills in what the source
// never said.
//
// An OVF descriptor names the firmware and the guest's OS; a bare VHDX or VMDK
// names nothing, and defaulting those to BIOS and Linux gives a Windows guest a
// virtio display it has no driver for. The partition table answers both
// questions: a GPT means UEFI, and a Microsoft Reserved partition means
// Windows. Best effort — an unreadable head leaves the defaults alone.
func (rt *computeRuntime) probeBootDisk(ctx context.Context, result *VMImportResult) {
	if result.Firmware != "" && result.GuestOSFamily != "" {
		return
	}
	disk := result.Disks[0]
	head, err := readDiskHead(ctx, disk.SourceFile, disk.SourceFormat)
	if err != nil {
		rt.logger.Debug("compute: could not read the import's partition table",
			"file", disk.SourceFile, "error", err)
		return
	}
	if len(head) < 1024 || !bytes.Equal(head[512:520], gptSignature) {
		return
	}
	if result.Firmware == "" {
		result.Firmware = "uefi"
	}
	if result.GuestOSFamily != "" {
		return
	}
	for _, guid := range windowsPartitionGUIDs {
		if bytes.Contains(head, guid) {
			result.GuestOSFamily = "windows"
			return
		}
	}
}

// readDiskHead returns the first mebibyte of an image in raw form.
//
// qemu-img is the only thing here that can read a VHDX or a VMDK, and `dd` is
// the one subcommand that will hand back a byte range rather than convert the
// whole disk.
func readDiskHead(ctx context.Context, path, format string) ([]byte, error) {
	dir, err := os.MkdirTemp("", "mcharbor-head-")
	if err != nil {
		return nil, err
	}
	defer func() { errlog.Log(os.RemoveAll(dir)) }()

	target := filepath.Join(dir, "head.raw")
	args := []string{"dd", "-O", "raw", "bs=512", "count=2048"}
	if format != "" {
		args = append(args, "-f", format)
	}
	args = append(args, "if="+path, "of="+target)

	probeCtx, cancel := context.WithTimeout(ctx, computeQuickTimeout)
	defer cancel()
	if out, err := exec.CommandContext(probeCtx, "qemu-img", args...).CombinedOutput(); err != nil {
		return nil, fmt.Errorf("qemu-img dd: %w (%s)", err, strings.TrimSpace(string(out)))
	}
	return os.ReadFile(target)
}

// inspectArchive unpacks an OVA beside itself and reads its descriptor.
//
// Unpacked rather than streamed because the disks inside have to be converted
// afterwards, and qemu-img needs a seekable file. The archive's own member
// names are never trusted as paths: an OVA is a file somebody else produced.
func (rt *computeRuntime) inspectArchive(ctx context.Context, source string, result *VMImportResult) error {
	dir := source + ".unpacked"
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("compute: create %s: %w", dir, err)
	}

	file, err := os.Open(source)
	if err != nil {
		return fmt.Errorf("compute: reading %s: %w", source, err)
	}
	defer func() { errlog.Close(file) }()

	var reader io.Reader = bufio.NewReaderSize(file, 1<<20)
	if strings.HasSuffix(strings.ToLower(source), ".tgz") {
		gz, gzErr := gzip.NewReader(reader)
		if gzErr != nil {
			return fmt.Errorf("compute: reading %s: %w", source, gzErr)
		}
		defer func() { errlog.Close(gz) }()
		reader = gz
	}

	var descriptor []byte
	diskFiles := map[string]string{}
	archive := tar.NewReader(reader)
	entries := 0
	var written int64

	for {
		header, nextErr := archive.Next()
		if errors.Is(nextErr, io.EOF) {
			break
		}
		if nextErr != nil {
			return fmt.Errorf("compute: reading %s: %w", source, nextErr)
		}
		if entries++; entries > maxArchiveEntries {
			return fmt.Errorf("%w: it has more than %d members", errImportArchiveTooBig, maxArchiveEntries)
		}
		if header.Typeflag != tar.TypeReg {
			continue
		}

		// filepath.Base is the whole defence against a member called
		// "../../etc/cron.d/x": an OVA's layout is flat, so nothing legitimate
		// is lost by refusing to honour directories in it at all.
		name := filepath.Base(header.Name)
		if name == "" || name == "." || name == ".." {
			continue
		}
		lower := strings.ToLower(name)

		if strings.HasSuffix(lower, ".ovf") {
			descriptor, err = io.ReadAll(io.LimitReader(archive, maxOVFBytes))
			if err != nil {
				return fmt.Errorf("compute: reading the OVF descriptor: %w", err)
			}
			continue
		}
		if _, known := importDiskExtensions[filepath.Ext(lower)]; !known {
			// Manifests, certificates and vendor extras. Skipped quietly:
			// they are normal contents, not a problem to report.
			continue
		}

		target := filepath.Join(dir, name)
		size, copyErr := writeArchiveMember(archive, target, maxArchiveBytes-written)
		if copyErr != nil {
			return copyErr
		}
		written += size
		diskFiles[lower] = target
	}

	ordered := applyOVF(descriptor, diskFiles, result)
	if len(ordered) == 0 {
		// A descriptor that named nothing usable, or none at all. Falling back
		// to the disks actually present beats refusing an archive whose files
		// are right there.
		for _, path := range diskFiles {
			ordered = append(ordered, path)
		}
	}

	for i, path := range ordered {
		info, infoErr := qemuImgInfo(ctx, path)
		if infoErr != nil {
			return infoErr
		}
		result.Disks = append(result.Disks, VMImportDisk{
			Index: i, SourceFile: path, SourceFormat: info.Format,
			VirtualSize: info.VirtualSize, ActualSize: info.ActualSize,
		})
	}
	return nil
}

// writeArchiveMember extracts one file under the remaining byte budget.
func writeArchiveMember(archive *tar.Reader, target string, budget int64) (int64, error) {
	if budget <= 0 {
		return 0, errImportArchiveTooBig
	}
	file, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		return 0, fmt.Errorf("compute: create %s: %w", target, err)
	}
	defer func() { errlog.Close(file) }()

	written, err := io.Copy(file, io.LimitReader(archive, budget))
	if err != nil {
		return written, fmt.Errorf("compute: unpacking %s: %w", target, err)
	}
	if written == budget {
		return written, errImportArchiveTooBig
	}
	return written, nil
}

// qemuImgInfoResult is the subset of `qemu-img info --output=json` used here.
type qemuImgInfoResult struct {
	Format      string `json:"format"`
	VirtualSize int64  `json:"virtual-size"`
	ActualSize  int64  `json:"actual-size"`
}

// qemuImgInfo reads a disk's real format and size.
//
// The detected format is used rather than the file's extension for the
// conversion below: a .img written by VirtualBox is frequently a VDI, and
// converting it as raw would produce a disk that boots to nothing.
func qemuImgInfo(ctx context.Context, path string) (qemuImgInfoResult, error) {
	ctx, cancel := context.WithTimeout(ctx, importInfoTimeout)
	defer cancel()

	var out qemuImgInfoResult
	// -U reads an image another process has open, which is the normal state of
	// a file just written, and is safe here because nothing is writing to it.
	cmd := exec.CommandContext(ctx, "qemu-img", "info", "-U", "--output=json", path)
	raw, err := cmd.Output()
	if err != nil {
		return out, fmt.Errorf("compute: reading %s with qemu-img: %w", filepath.Base(path), err)
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return out, fmt.Errorf("compute: qemu-img described %s in a way McHarbor could not read: %w",
			filepath.Base(path), err)
	}
	return out, nil
}

// convertImport turns every source disk into a volume in the target pool.
func (rt *computeRuntime) convertImport(ctx context.Context, spec VMImportSpec, result *VMImportResult) error {
	driver, err := rt.pools.Get(spec.PoolBackend)
	if err != nil {
		return err
	}
	pool := storage.Pool{ID: spec.PoolID, Backend: spec.PoolBackend, Target: spec.PoolTarget}

	format := spec.Format
	if format == "" {
		format = "qcow2"
	}

	for i := range result.Disks {
		disk := &result.Disks[i]

		// The volume is allocated through the pool's own driver rather than by
		// qemu-img writing a file: on zfs or lvm the target is a block device
		// that has to exist first, and going through the driver is what makes
		// dir, zfs, btrfs and lvm all work from one code path.
		volume, createErr := driver.CreateVolume(ctx, storage.CreateVolumeRequest{
			Pool:   pool,
			Name:   fmt.Sprintf("%s-%d", spec.VMID, disk.Index),
			Kind:   storage.VolumeDisk,
			Size:   disk.VirtualSize,
			Format: format,
			Sparse: true,
		})
		if createErr != nil {
			return fmt.Errorf("compute: allocating a volume for disk %d: %w", disk.Index, createErr)
		}

		if convertErr := rt.runConvert(ctx, spec, disk, volume); convertErr != nil {
			// The half-written volume is removed: leaving it would look like a
			// successful import with a disk that will not boot.
			if rmErr := driver.DeleteVolume(ctx, pool, volume.Path); rmErr != nil {
				rt.logger.Warn("compute: removing a failed import's volume failed",
					"path", volume.Path, "error", rmErr)
			}
			return convertErr
		}
		disk.VolumePath = volume.Path
	}
	return nil
}

// runConvert converts one disk, reporting progress as it goes.
func (rt *computeRuntime) runConvert(ctx context.Context, spec VMImportSpec, disk *VMImportDisk, volume storage.Volume) error {
	ctx, cancel := context.WithTimeout(ctx, importConvertTimeout)
	defer cancel()

	targetFormat := volume.Format
	if targetFormat == "" {
		targetFormat = "raw"
	}

	args := []string{
		"convert", "-p",
		// The source format is stated rather than probed. qemu-img's own
		// warning about this is well earned: a probed format is a format an
		// attacker-supplied header can choose.
		"-f", disk.SourceFormat,
		"-O", targetFormat,
		disk.SourceFile, volume.Path,
	}
	cmd := exec.CommandContext(ctx, "qemu-img", args...)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("compute: converting disk %d: %w", disk.Index, err)
	}
	cmd.Stderr = cmd.Stdout

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("compute: converting disk %d: %w", disk.Index, err)
	}
	rt.reportConvertProgress(spec, disk.Index, stdout)

	if err := cmd.Wait(); err != nil {
		return fmt.Errorf("compute: converting disk %d failed: %w", disk.Index, err)
	}
	return nil
}

// reportConvertProgress forwards qemu-img's percentage to McHarbor.
//
// qemu-img writes `(52.31/100%)` separated by carriage returns rather than
// newlines, so the scanner splits on both — a line-based reader would see one
// enormous line and report nothing until the conversion finished.
func (rt *computeRuntime) reportConvertProgress(spec VMImportSpec, index int, stdout io.Reader) {
	scanner := bufio.NewScanner(stdout)
	scanner.Split(scanProgressChunks)
	last := -1
	for scanner.Scan() {
		match := qemuImgProgress.FindStringSubmatch(scanner.Text())
		if match == nil {
			continue
		}
		value, err := strconv.ParseFloat(match[1], 64)
		if err != nil {
			continue
		}
		percent := int(value)
		if percent == last {
			continue
		}
		last = percent
		errlog.Log(rt.send(WSMessage{Type: MsgVMImportProgress, VM: &VMPayload{
			Operation: MsgVMImportConvert,
			VMID:      spec.VMID,
			Percent:   percent,
			Import:    &VMImportSpec{ImportID: spec.ImportID},
			ImportResult: &VMImportResult{
				ImportID: spec.ImportID,
				Disks:    []VMImportDisk{{Index: index}},
			},
		}}))
	}
}

// scanProgressChunks splits on newlines and carriage returns alike.
func scanProgressChunks(data []byte, atEOF bool) (int, []byte, error) {
	for i, b := range data {
		if b == '\n' || b == '\r' {
			return i + 1, data[:i], nil
		}
	}
	if atEOF && len(data) > 0 {
		return len(data), data, nil
	}
	return 0, nil, nil
}

// --- OVF ------------------------------------------------------------------

// ovfEnvelope is the subset of an OVF descriptor McHarbor reads.
type ovfEnvelope struct {
	References struct {
		Files []struct {
			ID   string `xml:"id,attr"`
			Href string `xml:"href,attr"`
		} `xml:"File"`
	} `xml:"References"`
	DiskSection struct {
		Disks []struct {
			DiskID   string `xml:"diskId,attr"`
			FileRef  string `xml:"fileRef,attr"`
			Capacity string `xml:"capacity,attr"`
		} `xml:"Disk"`
	} `xml:"DiskSection"`
	VirtualSystem struct {
		ID   string `xml:"id,attr"`
		Name string `xml:"Name"`
		OS   struct {
			ID          string `xml:"id,attr"`
			OSType      string `xml:"osType,attr"`
			Description string `xml:"Description"`
		} `xml:"OperatingSystemSection"`
		Hardware struct {
			Items []struct {
				ResourceType    string `xml:"ResourceType"`
				VirtualQuantity string `xml:"VirtualQuantity"`
				AllocationUnits string `xml:"AllocationUnits"`
				HostResource    string `xml:"HostResource"`
				Description     string `xml:"Description"`
				ElementName     string `xml:"ElementName"`
			} `xml:"Item"`
			Configs []struct {
				Key   string `xml:"key,attr"`
				Value string `xml:"value,attr"`
			} `xml:"Config"`
		} `xml:"VirtualHardwareSection"`
	} `xml:"VirtualSystem"`
}

// CIM resource types an OVF item can carry. Only the four that change how the
// guest is built are read; the rest are hardware McHarbor decides for itself.
const (
	ovfResourceProcessor = "3"
	ovfResourceMemory    = "4"
	ovfResourceEthernet  = "10"
	ovfResourceDisk      = "17"
)

// applyOVF reads the descriptor onto the result and returns the disk paths in
// the order the descriptor listed them.
//
// Order matters: the first disk is the one the original guest booted from, and
// importing a two-disk machine with its data volume first would produce a guest
// that does not boot for no visible reason.
func applyOVF(descriptor []byte, diskFiles map[string]string, result *VMImportResult) []string {
	if len(descriptor) == 0 {
		return nil
	}
	var envelope ovfEnvelope
	if err := xml.Unmarshal(descriptor, &envelope); err != nil {
		result.Warnings = append(result.Warnings,
			"the OVF descriptor could not be read, so the guest's processors and memory were not imported")
		return nil
	}

	result.Name = strings.TrimSpace(envelope.VirtualSystem.Name)
	if result.Name == "" {
		result.Name = strings.TrimSpace(envelope.VirtualSystem.ID)
	}
	result.GuestOSVariant = strings.TrimSpace(envelope.VirtualSystem.OS.Description)
	result.GuestOSFamily = guestFamilyFromOVF(envelope.VirtualSystem.OS.OSType,
		envelope.VirtualSystem.OS.Description)

	for _, item := range envelope.VirtualSystem.Hardware.Items {
		switch strings.TrimSpace(item.ResourceType) {
		case ovfResourceProcessor:
			if value, err := strconv.Atoi(strings.TrimSpace(item.VirtualQuantity)); err == nil {
				result.VCPUs = value
			}
		case ovfResourceMemory:
			result.MemoryMB = ovfMemoryMB(item.VirtualQuantity, item.AllocationUnits)
		case ovfResourceEthernet:
			result.NICCount++
		}
	}

	for _, config := range envelope.VirtualSystem.Hardware.Configs {
		if strings.EqualFold(config.Key, "firmware") {
			if strings.EqualFold(config.Value, "efi") {
				result.Firmware = "uefi"
			} else {
				result.Firmware = "bios"
			}
		}
	}

	hrefByID := map[string]string{}
	for _, file := range envelope.References.Files {
		hrefByID[file.ID] = file.Href
	}

	var ordered []string
	seen := map[string]bool{}
	for _, disk := range envelope.DiskSection.Disks {
		href := hrefByID[disk.FileRef]
		if href == "" {
			continue
		}
		path, found := diskFiles[strings.ToLower(filepath.Base(href))]
		if !found || seen[path] {
			continue
		}
		seen[path] = true
		ordered = append(ordered, path)
	}
	// Anything in the archive the descriptor did not mention still gets
	// imported, after the ones it did. Dropping a disk silently would lose
	// data; appending it keeps the boot order the descriptor asked for.
	for _, path := range diskFiles {
		if !seen[path] {
			ordered = append(ordered, path)
		}
	}

	if len(envelope.DiskSection.Disks) > 0 && result.VCPUs == 0 {
		result.Warnings = append(result.Warnings,
			"the OVF descriptor named no processor count, so McHarbor's default is used")
	}
	return ordered
}

// ovfMemoryMB converts an OVF memory quantity into megabytes.
//
// OVF states its unit as a CIM programmatic unit — "byte * 2^20" for MiB,
// "byte * 2^30" for GiB — rather than a name, and a descriptor that omits it
// means MiB by convention.
func ovfMemoryMB(quantity, units string) int64 {
	value, err := strconv.ParseInt(strings.TrimSpace(quantity), 10, 64)
	if err != nil || value <= 0 {
		return 0
	}
	normalized := strings.ReplaceAll(strings.ToLower(strings.TrimSpace(units)), " ", "")
	switch normalized {
	case "", "byte*2^20", "megabytes":
		return value
	case "byte*2^30", "gigabytes":
		return value * 1024
	case "byte*2^10", "kilobytes":
		return value / 1024
	case "byte", "bytes":
		return value / (1024 * 1024)
	default:
		return value
	}
}

// guestFamilyFromOVF maps an OVF operating-system hint onto McHarbor's family,
// which is what decides the display adapter and driver defaults.
func guestFamilyFromOVF(osType, description string) string {
	hint := strings.ToLower(osType + " " + description)
	switch {
	case strings.Contains(hint, "windows"):
		return "windows"
	case strings.Contains(hint, "bsd"):
		return "bsd"
	case hint == " " || strings.TrimSpace(hint) == "":
		return ""
	default:
		return "linux"
	}
}
