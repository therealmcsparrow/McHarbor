// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

//go:build linux

package cloudinit

import (
	"fmt"
	"os"
	"path/filepath"

	diskfs "github.com/diskfs/go-diskfs"
	"github.com/diskfs/go-diskfs/disk"
	"github.com/diskfs/go-diskfs/filesystem"
	"github.com/diskfs/go-diskfs/filesystem/iso9660"
	"github.com/therealmcsparrow/mcharbor-agent/internal/errlog"
)

// seedSizeBytes is the image size. The three documents together are a few
// kilobytes, but ISO9660 has fixed overhead — system area, volume descriptors,
// path tables — and a volume too small to hold them fails at format time rather
// than at write time. 1 MiB is comfortably past that and still negligible on
// any pool.
const seedSizeBytes = 1 << 20

// isoBlockSize is the logical block size ISO9660 requires.
const isoBlockSize = 2048

// WriteSeedISO writes the seed as an ISO9660 volume labelled CIDATA.
//
// The image is built at a temporary path and renamed into place, so a guest can
// never be handed a half-written seed — which would present as cloud-init
// silently doing nothing, one of the harder failures to diagnose from inside a
// guest.
func WriteSeedISO(path string, seed Seed) error {
	if err := ContainsPlaintextSecret(seed); err != nil {
		return err
	}

	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("cloudinit: prepare %s: %w", filepath.Dir(path), err)
	}

	tmpPath := path + ".building"
	// A leftover from an interrupted previous attempt would make Create fail
	// or, worse, be partially reused.
	errlog.Log(os.Remove(tmpPath))

	committed := false
	defer func() {
		if !committed {
			errlog.Log(os.Remove(tmpPath))
		}
	}()

	// SectorSizeDefault is 512, and go-diskfs uses the disk's logical block
	// size as the filesystem's — which ISO9660 rejects, because the standard
	// fixes a logical block at 2048. The default therefore fails at format
	// time with "blocksize for ISO9660 must be one of 2048, 4096, 8192", after
	// the guest, its disk and its network are already defined.
	image, err := diskfs.Create(tmpPath, seedSizeBytes, isoBlockSize)
	if err != nil {
		return fmt.Errorf("cloudinit: create seed image: %w", err)
	}
	// Closed explicitly below on the success path, so this covers the error
	// paths only. The flag is not belt-and-braces: go-diskfs nils its backend
	// on Close and the second call dereferences it, so closing twice panics
	// rather than returning the error this used to discard.
	imageClosed := false
	defer func() {
		if !imageClosed {
			errlog.Close(image)
		}
	}()

	fs, err := image.CreateFilesystem(disk.FilesystemSpec{
		Partition:   0,
		FSType:      filesystem.TypeISO9660,
		VolumeLabel: VolumeLabel,
	})
	if err != nil {
		return fmt.Errorf("cloudinit: create ISO9660 filesystem: %w", err)
	}

	files := []struct {
		name    string
		content []byte
	}{
		{"/meta-data", seed.MetaData},
		{"/user-data", seed.UserData},
		{"/network-config", seed.NetworkConfig},
	}
	for _, file := range files {
		if len(file.content) == 0 {
			continue
		}
		if err := writeFile(fs, file.name, file.content); err != nil {
			return err
		}
	}

	iso, ok := fs.(*iso9660.FileSystem)
	if !ok {
		return fmt.Errorf("cloudinit: unexpected filesystem type %T", fs)
	}
	// Finalize writes the volume descriptors and the path table. Without it
	// the file exists but is not a mountable ISO, and the guest finds no
	// datasource.
	//
	// RockRidge is enabled because cloud-init reads the files by their exact
	// lowercase names; plain ISO9660 would uppercase them and truncate to 8.3,
	// at which point the datasource is not recognised.
	if err := iso.Finalize(iso9660.FinalizeOptions{
		VolumeIdentifier: VolumeLabel,
		RockRidge:        true,
	}); err != nil {
		return fmt.Errorf("cloudinit: finalize seed image: %w", err)
	}

	if err := image.Close(); err != nil {
		return fmt.Errorf("cloudinit: close seed image: %w", err)
	}
	imageClosed = true
	// 0600 for the same reason the pool is 0700. The seed carries no plaintext
	// secret by construction, but it does carry the operator's SSH public keys
	// and the guest's identity, and there is no reason for other local users to
	// read it.
	if err := os.Chmod(tmpPath, 0o600); err != nil {
		return fmt.Errorf("cloudinit: chmod seed image: %w", err)
	}
	if err := os.Rename(tmpPath, path); err != nil {
		return fmt.Errorf("cloudinit: install seed image at %s: %w", path, err)
	}
	committed = true
	return nil
}

// writeFile puts one document into the filesystem.
func writeFile(fs filesystem.FileSystem, name string, content []byte) error {
	f, err := fs.OpenFile(name, os.O_CREATE|os.O_RDWR)
	if err != nil {
		return fmt.Errorf("cloudinit: create %s in seed: %w", name, err)
	}
	n, err := f.Write(content)
	if err != nil {
		return fmt.Errorf("cloudinit: write %s in seed: %w", name, err)
	}
	if n != len(content) {
		return fmt.Errorf("cloudinit: short write for %s in seed: %d of %d bytes",
			name, n, len(content))
	}
	return nil
}
