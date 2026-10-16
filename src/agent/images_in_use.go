// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"github.com/therealmcsparrow/mcharbor-agent/compute/imagefetch"
)

// Which cached images back a guest disk.
//
// The answer is read from the disks themselves at query time — each disk's
// qcow2 backing chain, followed to its end — rather than from a record of what
// the image was when the disk was made. A rebase, a snapshot overlay or a
// storage move changes the chain, and a stale record is how an image a guest
// still reads through gets evicted.
//
// The disks come from two places: every started guest's own QEMU command line,
// and a ledger of volumes this agent created over a backing file, so a guest
// that was defined from an image and never started is covered too.

// maxBackingDepth bounds one chain walk. A chain that loops would otherwise
// spin forever, and no real guest is this deep: snapshot chains are capped far
// below it.
const maxBackingDepth = 64

// backedImages walks every disk's backing chain and reports which cached images
// the chains reach, by image id.
func backedImages(disks []string, backingOf func(path string) string, cached []imagefetch.Cached) map[string]bool {
	byPath := make(map[string]string, len(cached))
	for _, image := range cached {
		byPath[canonicalPath(image.LocalPath)] = image.ImageID
	}

	inUse := make(map[string]bool)
	for _, disk := range disks {
		current := canonicalPath(disk)
		seen := map[string]bool{current: true}
		for depth := 0; depth < maxBackingDepth; depth++ {
			backing := backingOf(current)
			if backing == "" {
				break
			}
			if !filepath.IsAbs(backing) {
				// qcow2 resolves a relative backing name against the image's
				// own directory, which is what makes a guest folder movable.
				backing = filepath.Join(filepath.Dir(current), backing)
			}
			backing = canonicalPath(backing)
			if seen[backing] {
				break
			}
			seen[backing] = true
			if id, ok := byPath[backing]; ok {
				inUse[id] = true
			}
			current = backing
		}
	}
	return inUse
}

// canonicalPath resolves symlinks where it can, so a cache reached through a
// linked directory still matches the path a backing reference names.
func canonicalPath(path string) string {
	if resolved, err := filepath.EvalSymlinks(path); err == nil {
		return resolved
	}
	return filepath.Clean(path)
}

// diskFilesFromArgv returns every file a QEMU command line opens as a block
// node. The values were escaped for QEMU's option syntax, where a literal
// comma is written twice.
func diskFilesFromArgv(argv []string) []string {
	var files []string
	for i := 0; i+1 < len(argv); i++ {
		if argv[i] != "-blockdev" && argv[i] != "-drive" {
			continue
		}
		options := splitQEMUOptions(argv[i+1])
		for _, option := range options {
			if value, ok := strings.CutPrefix(option, "filename="); ok && value != "" {
				files = append(files, value)
			} else if value, ok := strings.CutPrefix(option, "file="); ok && strings.HasPrefix(value, "/") {
				files = append(files, value)
			}
		}
	}
	return files
}

// splitQEMUOptions splits on single commas and turns ",," back into ",".
func splitQEMUOptions(value string) []string {
	var out []string
	var current strings.Builder
	for i := 0; i < len(value); i++ {
		if value[i] != ',' {
			current.WriteByte(value[i])
			continue
		}
		if i+1 < len(value) && value[i+1] == ',' {
			current.WriteByte(',')
			i++
			continue
		}
		out = append(out, current.String())
		current.Reset()
	}
	return append(out, current.String())
}

// guestArgvFiles reads every guest's args.json under root and returns the disk
// files named there. A guest never started has no args file and is covered by
// the ledger instead.
func guestArgvFiles(root string) []string {
	matches, err := filepath.Glob(filepath.Join(root, "*", "args.json"))
	if err != nil {
		return nil
	}
	var files []string
	for _, path := range matches {
		body, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		var args struct {
			Argv []string `json:"argv"`
		}
		if json.Unmarshal(body, &args) != nil {
			continue
		}
		files = append(files, diskFilesFromArgv(args.Argv)...)
	}
	return files
}

// backedVolumeLedger remembers volumes this agent created over a backing file.
// It stores the volume paths only; what they back onto is read afresh at query
// time, never trusted from here.
type backedVolumeLedger struct {
	mu   sync.Mutex
	path string
}

// record adds a volume. Failure to record is returned so the caller can log it;
// it never fails the create, because the argv scan still covers a started guest.
func (l *backedVolumeLedger) record(volume string) error {
	if l == nil || l.path == "" || volume == "" {
		return nil
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	volumes, err := l.readLocked()
	if err != nil {
		return err
	}
	for _, existing := range volumes {
		if existing == volume {
			return nil
		}
	}
	return l.writeLocked(append(volumes, volume))
}

// volumes returns the recorded volumes that still exist, dropping the rest.
func (l *backedVolumeLedger) volumes() []string {
	if l == nil || l.path == "" {
		return nil
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	volumes, err := l.readLocked()
	if err != nil {
		return nil
	}
	kept := volumes[:0]
	for _, volume := range volumes {
		if _, err := os.Stat(volume); err == nil {
			kept = append(kept, volume)
		}
	}
	if len(kept) != len(volumes) {
		if err := l.writeLocked(kept); err != nil {
			fmt.Fprintf(os.Stderr, "compute: pruning the backed-volume ledger: %v\n", err)
		}
	}
	return append([]string(nil), kept...)
}

func (l *backedVolumeLedger) readLocked() ([]string, error) {
	body, err := os.ReadFile(l.path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("compute: reading %s: %w", l.path, err)
	}
	var volumes []string
	if err := json.Unmarshal(body, &volumes); err != nil {
		return nil, fmt.Errorf("compute: parsing %s: %w", l.path, err)
	}
	return volumes, nil
}

func (l *backedVolumeLedger) writeLocked(volumes []string) error {
	sort.Strings(volumes)
	body, err := json.Marshal(volumes)
	if err != nil {
		return fmt.Errorf("compute: encoding the backed-volume ledger: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(l.path), 0o700); err != nil {
		return fmt.Errorf("compute: preparing %s: %w", l.path, err)
	}
	temp := l.path + ".tmp"
	if err := os.WriteFile(temp, body, 0o600); err != nil {
		return fmt.Errorf("compute: writing %s: %w", temp, err)
	}
	if err := os.Rename(temp, l.path); err != nil {
		return fmt.Errorf("compute: replacing %s: %w", l.path, err)
	}
	return nil
}
