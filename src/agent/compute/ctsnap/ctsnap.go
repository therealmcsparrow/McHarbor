// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// Package ctsnap turns a pool snapshot of a container rootfs into a directory the
// backup archiver can read, and finds what a killed agent left behind.
//
// Everything in this file is policy and parsing with the host commands behind a
// Runner, so it is testable on any OS. The mounting and the stat calls live in
// ctsnap_linux.go.
package ctsnap

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path"
	"strings"
	"sync"
	"time"
)

// Prefix marks every snapshot, logical volume and mount directory this package
// creates, so a sweep can tell ours from an operator's without a record a kill would lose.
const Prefix = "mcharbor-ctbk-"

// MountRoot is where LVM snapshots are mounted: under /run so a reboot clears it,
// and fixed so a restarted agent knows where a killed one may have left a mount.
const MountRoot = "/run/mcharbor-agent/ct-snap"

// commandTimeout bounds one query or snapshot command; none of them scale with
// the size of the rootfs, so a minute is already generous.
const commandTimeout = 60 * time.Second

// ErrNotReadable means this pool's snapshot cannot be read as a directory tree on
// this host, so the caller falls back to the cgroup freezer instead.
var ErrNotReadable = errors.New("ctsnap: this pool's snapshot cannot be read as a directory here")

// Runner executes one host command with a timeout and an argv, returning its
// combined output; it is a parameter so the tests can answer for zfs and lvm.
type Runner func(ctx context.Context, timeout time.Duration, name string, args ...string) (string, error)

// SnapshotName is the name every artifact of runID carries; the run id is
// validated because it ends up in a zfs, lvm and mount argv.
func SnapshotName(runID string) (string, error) {
	if !validRunID(runID) {
		return "", fmt.Errorf("ctsnap: run id %q cannot name a snapshot", runID)
	}
	return Prefix + runID, nil
}

// RunIDFromName returns the run a name of ours belongs to, and false for any
// name this package did not produce.
func RunIDFromName(name string) (string, bool) {
	if !strings.HasPrefix(name, Prefix) {
		return "", false
	}
	id := strings.TrimPrefix(name, Prefix)
	if !validRunID(id) {
		return "", false
	}
	return id, true
}

func validRunID(id string) bool {
	if id == "" || len(id) > 64 || strings.HasPrefix(id, "-") {
		return false
	}
	for _, r := range id {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-', r == '_':
		default:
			return false
		}
	}
	return true
}

// Active is the set of runs currently holding a snapshot. A sweep skips them,
// which is what makes sweeping before every backup safe beside a concurrent one.
type Active struct {
	mu   sync.Mutex
	runs map[string]struct{}
}

// NewActive returns an empty registry; an agent that just started holds nothing,
// so everything of ours it finds is stale.
func NewActive() *Active {
	return &Active{runs: make(map[string]struct{})}
}

// Add marks runID as holding a snapshot until the returned call; it is added
// before anything is created, so no sweep can see the artifact unclaimed.
func (a *Active) Add(runID string) (done func()) {
	a.mu.Lock()
	a.runs[runID] = struct{}{}
	a.mu.Unlock()
	var once sync.Once
	return func() {
		once.Do(func() {
			a.mu.Lock()
			delete(a.runs, runID)
			a.mu.Unlock()
		})
	}
}

// Has reports whether runID is in flight. A nil registry holds nothing.
func (a *Active) Has(runID string) bool {
	if a == nil {
		return false
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	_, ok := a.runs[runID]
	return ok
}

// MountEntry is one line of the agent's mountinfo, reduced to what resolving a
// snapshot needs. Root is the path inside the filesystem that is mounted, which
// is "/" except for a bind mount of a subdirectory.
type MountEntry struct {
	Root       string
	MountPoint string
	FSType     string
	Source     string
}

// ParseMountInfo reads the mountinfo format; a line it cannot read is skipped
// rather than failing the whole table.
func ParseMountInfo(data string) []MountEntry {
	var out []MountEntry
	for _, line := range strings.Split(data, "\n") {
		fields := strings.Fields(line)
		sep := -1
		for i := 6; i < len(fields); i++ {
			if fields[i] == "-" {
				sep = i
				break
			}
		}
		if sep < 0 || sep+2 >= len(fields) {
			continue
		}
		out = append(out, MountEntry{
			Root:       unescapeMount(fields[3]),
			MountPoint: unescapeMount(fields[4]),
			FSType:     fields[sep+1],
			Source:     unescapeMount(fields[sep+2]),
		})
	}
	return out
}

// unescapeMount undoes the kernel's octal escaping of space, tab, newline and
// backslash in mountinfo fields.
func unescapeMount(field string) string {
	if !strings.Contains(field, `\`) {
		return field
	}
	var b strings.Builder
	for i := 0; i < len(field); i++ {
		if field[i] == '\\' && i+3 < len(field) && isOctal(field[i+1:i+4]) {
			b.WriteByte((field[i+1]-'0')<<6 | (field[i+2]-'0')<<3 | (field[i+3] - '0'))
			i += 3
			continue
		}
		b.WriteByte(field[i])
	}
	return b.String()
}

func isOctal(s string) bool {
	if len(s) != 3 {
		return false
	}
	for i := 0; i < 3; i++ {
		if s[i] < '0' || s[i] > '7' {
			return false
		}
	}
	return true
}

// ContainingMount finds the mount a path lives on: the longest mount point that
// contains it, and the later of two at the same point because a later mount shadows.
func ContainingMount(entries []MountEntry, p string) (MountEntry, bool) {
	p = path.Clean(p)
	best, found := MountEntry{}, false
	for _, entry := range entries {
		if !within(entry.MountPoint, p) {
			continue
		}
		if !found || len(entry.MountPoint) >= len(best.MountPoint) {
			best, found = entry, true
		}
	}
	return best, found
}

// FSRelative is where p sits inside the mounted filesystem's own root, which is
// the path to read inside a snapshot of that filesystem.
func FSRelative(entry MountEntry, p string) string {
	p = path.Clean(p)
	rel := strings.TrimPrefix(p, path.Clean(entry.MountPoint))
	root := entry.Root
	if root == "" {
		root = "/"
	}
	return path.Join(root, rel)
}

func within(base, p string) bool {
	base = path.Clean(base)
	if base == "/" {
		return strings.HasPrefix(p, "/")
	}
	return p == base || strings.HasPrefix(p, base+"/")
}

// ZFSProps is what deciding whether a dataset's snapshots are reachable needs.
type ZFSProps struct {
	Mountpoint string
	Snapdir    string
	Mounted    string
}

// ReadZFSProps asks zfs for the dataset's mountpoint, snapdir and mounted state
// in one call, keyed by property so the output order does not matter.
func ReadZFSProps(ctx context.Context, run Runner, dataset string) (ZFSProps, error) {
	out, err := run(ctx, commandTimeout, "zfs", "get", "-H", "-o", "property,value",
		"mountpoint,snapdir,mounted", dataset)
	if err != nil {
		return ZFSProps{}, fmt.Errorf("ctsnap: reading the properties of %s: %w (%s)", dataset, err, firstLine(out))
	}
	var props ZFSProps
	for _, line := range strings.Split(out, "\n") {
		key, value, ok := strings.Cut(strings.TrimRight(line, "\r"), "\t")
		if !ok {
			continue
		}
		switch key {
		case "mountpoint":
			props.Mountpoint = value
		case "snapdir":
			props.Snapdir = value
		case "mounted":
			props.Mounted = value
		}
	}
	return props, nil
}

// ZFSSnapshotTree is the directory a snapshot of a mounted dataset is readable
// at. snapdir=hidden only drops .zfs from directory listings; the path still
// resolves, so only snapdir=disabled and a dataset with no mount of its own refuse.
func ZFSSnapshotTree(props ZFSProps, fsRelative, snapName string) (string, error) {
	switch props.Mountpoint {
	case "", "-", "none", "legacy":
		return "", fmt.Errorf("%w: the dataset's mountpoint is %q, so there is no .zfs directory McHarbor can locate",
			ErrNotReadable, props.Mountpoint)
	}
	if !strings.HasPrefix(props.Mountpoint, "/") {
		return "", fmt.Errorf("%w: the dataset's mountpoint %q is not absolute", ErrNotReadable, props.Mountpoint)
	}
	if props.Mounted != "yes" {
		return "", fmt.Errorf("%w: the dataset is not mounted", ErrNotReadable)
	}
	if props.Snapdir == "disabled" {
		return "", fmt.Errorf("%w: snapdir=disabled removes the .zfs directory entirely", ErrNotReadable)
	}
	return path.Join(props.Mountpoint, ".zfs", "snapshot", snapName, fsRelative), nil
}

// ZFSPlan is a resolved ZFS snapshot: which dataset to snapshot, under what
// name, and where the rootfs will be readable once it exists.
type ZFSPlan struct {
	Dataset  string
	Snapshot string
	Tree     string
}

// PlanZFS works out, before anything is created, whether a snapshot of the
// dataset holding rootfs would be readable, so a refusal costs nothing.
func PlanZFS(ctx context.Context, run Runner, mounts []MountEntry, rootfs, runID string) (ZFSPlan, error) {
	name, err := SnapshotName(runID)
	if err != nil {
		return ZFSPlan{}, err
	}
	entry, ok := ContainingMount(mounts, rootfs)
	if !ok || entry.FSType != "zfs" {
		return ZFSPlan{}, fmt.Errorf("%w: %s is not on a mounted ZFS dataset", ErrNotReadable, rootfs)
	}
	if !validDataset(entry.Source) {
		return ZFSPlan{}, fmt.Errorf("%w: %q is not a dataset name McHarbor will pass to zfs", ErrNotReadable, entry.Source)
	}
	props, err := ReadZFSProps(ctx, run, entry.Source)
	if err != nil {
		return ZFSPlan{}, err
	}
	tree, err := ZFSSnapshotTree(props, FSRelative(entry, rootfs), name)
	if err != nil {
		return ZFSPlan{}, err
	}
	return ZFSPlan{Dataset: entry.Source, Snapshot: entry.Source + "@" + name, Tree: tree}, nil
}

func validDataset(name string) bool {
	if name == "" || strings.HasPrefix(name, "-") || strings.HasPrefix(name, "/") || strings.Contains(name, "@") {
		return false
	}
	for _, r := range name {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9',
			r == '-', r == '_', r == '.', r == ':', r == '/':
		default:
			return false
		}
	}
	return true
}

// DeviceLV decodes the volume group and logical volume behind a mount source,
// from either /dev/mapper/<vg>-<lv> (dashes inside a name doubled) or /dev/<vg>/<lv>.
func DeviceLV(source string) (vg, lv string, ok bool) {
	switch {
	case strings.HasPrefix(source, "/dev/mapper/"):
		vg, lv, ok = splitDMName(strings.TrimPrefix(source, "/dev/mapper/"))
	case strings.HasPrefix(source, "/dev/"):
		parts := strings.Split(strings.TrimPrefix(source, "/dev/"), "/")
		if len(parts) != 2 || parts[0] == "mapper" || parts[0] == "disk" {
			return "", "", false
		}
		vg, lv, ok = parts[0], parts[1], true
	default:
		return "", "", false
	}
	if !ok || !validLVMName(vg) || !validLVMName(lv) {
		return "", "", false
	}
	return vg, lv, true
}

// splitDMName undoes device-mapper's escaping: the first single dash separates
// the volume group from the volume, and "--" is a literal dash in either.
func splitDMName(name string) (vg, lv string, ok bool) {
	var b strings.Builder
	for i := 0; i < len(name); i++ {
		if name[i] != '-' {
			b.WriteByte(name[i])
			continue
		}
		if i+1 < len(name) && name[i+1] == '-' {
			b.WriteByte('-')
			i++
			continue
		}
		vg = b.String()
		rest := strings.ReplaceAll(name[i+1:], "--", "-")
		if vg == "" || rest == "" {
			return "", "", false
		}
		return vg, rest, true
	}
	return "", "", false
}

func validLVMName(name string) bool {
	if name == "" || strings.HasPrefix(name, "-") {
		return false
	}
	for _, r := range name {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9',
			r == '-', r == '_', r == '.', r == '+':
		default:
			return false
		}
	}
	return true
}

// MountOptions is how a snapshot of fstype is mounted for reading. xfs refuses a
// second mount of the same UUID without nouuid; nodev/nosuid/noexec because the
// tree is a container's and nothing on the host should execute or open from it.
func MountOptions(fstype string) (string, bool) {
	switch fstype {
	case "ext2", "ext3", "ext4":
		return "ro,nodev,nosuid,noexec", true
	case "xfs":
		return "ro,nouuid,nodev,nosuid,noexec", true
	default:
		return "", false
	}
}

// LVMPlan is a resolved LVM snapshot: the origin, the snapshot volume, where it
// is mounted and where the rootfs will be readable inside that mount.
type LVMPlan struct {
	VG       string
	Origin   string
	Snapshot string
	Device   string
	FSType   string
	Options  string
	MountDir string
	Tree     string
}

// PlanLVM works out whether the volume holding rootfs can be snapshotted and
// mounted here, before anything is created. Only a thin origin qualifies: a
// thick snapshot needs a size, and guessing one wrong fails the snapshot mid-copy.
func PlanLVM(ctx context.Context, run Runner, mounts []MountEntry, rootfs, runID string) (LVMPlan, error) {
	name, err := SnapshotName(runID)
	if err != nil {
		return LVMPlan{}, err
	}
	entry, ok := ContainingMount(mounts, rootfs)
	if !ok {
		return LVMPlan{}, fmt.Errorf("%w: no mount holds %s", ErrNotReadable, rootfs)
	}
	options, ok := MountOptions(entry.FSType)
	if !ok {
		return LVMPlan{}, fmt.Errorf("%w: a %s filesystem is not one McHarbor mounts a snapshot of", ErrNotReadable, entry.FSType)
	}
	vg, lv, ok := DeviceLV(entry.Source)
	if !ok {
		return LVMPlan{}, fmt.Errorf("%w: %s is not a logical volume McHarbor can name", ErrNotReadable, entry.Source)
	}
	out, err := run(ctx, commandTimeout, "lvs", "--reportformat", "json", "-o", "segtype", vg+"/"+lv)
	if err != nil {
		return LVMPlan{}, fmt.Errorf("%w: lvs %s/%s: %w (%s)", ErrNotReadable, vg, lv, err, firstLine(out))
	}
	rows, err := parseLVReport(out)
	if err != nil {
		return LVMPlan{}, err
	}
	if len(rows) != 1 || rows[0].SegType != "thin" {
		return LVMPlan{}, fmt.Errorf("%w: %s/%s is not a thin volume, and a thick snapshot needs a size McHarbor cannot choose",
			ErrNotReadable, vg, lv)
	}
	mountDir := path.Join(MountRoot, name)
	return LVMPlan{
		VG:       vg,
		Origin:   lv,
		Snapshot: name,
		Device:   "/dev/" + vg + "/" + name,
		FSType:   entry.FSType,
		Options:  options,
		MountDir: mountDir,
		Tree:     path.Join(mountDir, FSRelative(entry, rootfs)),
	}, nil
}

// LV is one volume of `lvs --reportformat json`; fields absent from the
// requested columns stay empty.
type LV struct {
	VG      string `json:"vg_name"`
	Name    string `json:"lv_name"`
	Origin  string `json:"origin"`
	SegType string `json:"segtype"`
}

func parseLVReport(out string) ([]LV, error) {
	var report struct {
		Report []struct {
			LV []LV `json:"lv"`
		} `json:"report"`
	}
	if err := json.Unmarshal([]byte(out), &report); err != nil {
		return nil, fmt.Errorf("ctsnap: reading the lvs report: %w", err)
	}
	var rows []LV
	for _, section := range report.Report {
		rows = append(rows, section.LV...)
	}
	return rows, nil
}

func firstLine(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return s[:i]
	}
	return s
}
