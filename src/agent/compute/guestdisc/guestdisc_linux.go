// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

//go:build linux

// Package guestdisc builds the small ISO9660 volume McHarbor attaches to a
// guest so its operator can install the QEMU guest agent from inside it.
package guestdisc

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	diskfs "github.com/diskfs/go-diskfs"
	"github.com/diskfs/go-diskfs/disk"
	"github.com/diskfs/go-diskfs/filesystem"
	"github.com/diskfs/go-diskfs/filesystem/iso9660"
	"github.com/therealmcsparrow/mcharbor-agent/internal/errlog"
)

// VolumeLabel is what the guest sees the disc mounted as.
const VolumeLabel = "MCHARBOR-GA"

// discRevision is part of every built filename, so a disc built by an older
// agent is replaced rather than reused when these scripts change.
const discRevision = "v3"

// isoBlockSize is the logical block size ISO9660 fixes. go-diskfs defaults to
// 512 and the format call then fails, as the cloud-init seed writer found.
const isoBlockSize = 2048

// isoOverheadBytes is the slack added on top of the payload: ISO9660's system
// area, volume descriptors and path tables need room a payload-sized volume
// does not have.
const isoOverheadBytes = 4 << 20

// virtioToolsURL is the Windows payload, pinned to an immutable archive path.
//
// The project publishes no digest for its artifacts, so the checksum below was
// computed from this exact file; both move together when virtio-win releases.
const virtioToolsURL = "https://fedorapeople.org/groups/virt/virtio-win/direct-downloads/" +
	"archive-virtio/virtio-win-0.1.302-1/virtio-win-guest-tools.exe"

// virtioToolsSHA256 is the digest of virtioToolsURL, verified 2026-09-20.
const virtioToolsSHA256 = "d8ae9ea1e943384ac195e012cd36f82c3161620e5fc9529be4e42e6f420e222d"

// virtioToolsSizeBytes bounds the download so a redirect to something else
// cannot fill the cache disk.
const virtioToolsSizeBytes int64 = 31642483

// virtioToolsName is the installer's name on the disc, referenced by
// windowsScript.
const virtioToolsName = "virtio-win-guest-tools.exe"

// fetchTimeout bounds the one download this package makes.
const fetchTimeout = 10 * time.Minute

// Families are the discs a host builds, one per guest family it can serve.
func Families() []string { return []string{"linux", "bsd", "windows"} }

// Path is where family's disc lives under root, whether or not it is built.
//
// Deterministic so the request path can answer with a stat rather than a
// build: the disc is prepared when the agent starts, not when an operator
// clicks.
func Path(root, family string) string {
	return filepath.Join(root,
		fmt.Sprintf("mcharbor-guest-agent-%s-%s.iso", normalizeFamily(family), discRevision))
}

// Ready reports whether family's disc has been built on this host.
func Ready(root, family string) bool {
	info, err := os.Stat(Path(root, family))
	return err == nil && info.Size() > 0
}

// Build writes the disc for family into root and returns its path, reusing one
// that is already there.
//
// Windows carries the virtio installer because a Windows guest with no virtio
// NIC driver has no network to download it over, which is the case the disc
// exists to solve; Linux and BSD install from their own repositories.
func Build(ctx context.Context, root, family string) (string, error) {
	if strings.TrimSpace(root) == "" {
		return "", fmt.Errorf("guestdisc: empty cache root")
	}
	family = normalizeFamily(family)

	path := Path(root, family)
	if Ready(root, family) {
		return path, nil
	}

	files := []payloadFile{{name: "/README.txt", content: []byte(readmeText)}}
	switch family {
	case "windows":
		installer, err := virtioTools(ctx, root)
		if err != nil {
			return "", err
		}
		files = append(files,
			payloadFile{name: "/install-guest-agent.ps1", content: []byte(windowsScript)},
			payloadFile{name: "/" + virtioToolsName, content: installer},
		)
	case "bsd":
		files = append(files, payloadFile{name: "/install-guest-agent.sh", content: []byte(bsdScript)})
	default:
		files = append(files, payloadFile{name: "/install-guest-agent.sh", content: []byte(linuxScript)})
	}

	if err := writeISO(path, files); err != nil {
		return "", err
	}
	return path, nil
}

// payloadFile is one document on the disc.
type payloadFile struct {
	name    string
	content []byte
}

// normalizeFamily maps what the guest reported onto the three discs this
// package builds, defaulting to linux because that is what an unknown guest
// most often is.
func normalizeFamily(family string) string {
	switch strings.ToLower(strings.TrimSpace(family)) {
	case "windows":
		return "windows"
	case "bsd", "freebsd", "openbsd", "netbsd":
		return "bsd"
	default:
		return "linux"
	}
}

// writeISO builds the volume at a temporary path and renames it into place, so
// a guest is never handed a half-written disc.
func writeISO(path string, files []payloadFile) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("guestdisc: prepare %s: %w", filepath.Dir(path), err)
	}

	size := int64(isoOverheadBytes)
	for _, file := range files {
		size += int64(len(file.content))
	}

	tmpPath := path + ".building"
	errlog.Log(os.Remove(tmpPath))

	committed := false
	defer func() {
		if !committed {
			errlog.Log(os.Remove(tmpPath))
		}
	}()

	image, err := diskfs.Create(tmpPath, size, isoBlockSize)
	if err != nil {
		return fmt.Errorf("guestdisc: create image: %w", err)
	}
	// go-diskfs nils its backend on Close and the second call dereferences it,
	// so this covers the error paths only and the success path clears the flag.
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
		return fmt.Errorf("guestdisc: create ISO9660 filesystem: %w", err)
	}

	for _, file := range files {
		if err := writeFile(fs, file.name, file.content); err != nil {
			return err
		}
	}

	iso, ok := fs.(*iso9660.FileSystem)
	if !ok {
		return fmt.Errorf("guestdisc: unexpected filesystem type %T", fs)
	}
	// RockRidge keeps the lowercase names the scripts and the README are
	// referred to by; plain ISO9660 would uppercase and truncate them to 8.3.
	if err := iso.Finalize(iso9660.FinalizeOptions{
		VolumeIdentifier: VolumeLabel,
		RockRidge:        true,
	}); err != nil {
		return fmt.Errorf("guestdisc: finalize image: %w", err)
	}

	if err := image.Close(); err != nil {
		return fmt.Errorf("guestdisc: close image: %w", err)
	}
	imageClosed = true
	// 0644 rather than the seed's 0600: this disc carries no secret, and QEMU
	// may read it as a user that is not the one that built it.
	if err := os.Chmod(tmpPath, 0o644); err != nil {
		return fmt.Errorf("guestdisc: chmod image: %w", err)
	}
	if err := os.Rename(tmpPath, path); err != nil {
		return fmt.Errorf("guestdisc: install image at %s: %w", path, err)
	}
	committed = true
	return nil
}

// writeFile puts one document into the filesystem.
func writeFile(fs filesystem.FileSystem, name string, content []byte) error {
	f, err := fs.OpenFile(name, os.O_CREATE|os.O_RDWR)
	if err != nil {
		return fmt.Errorf("guestdisc: create %s: %w", name, err)
	}
	n, err := f.Write(content)
	if err != nil {
		return fmt.Errorf("guestdisc: write %s: %w", name, err)
	}
	if n != len(content) {
		return fmt.Errorf("guestdisc: short write for %s: %d of %d bytes", name, n, len(content))
	}
	return nil
}

// virtioTools returns the Windows installer, downloading it once into root and
// verifying it against the pinned digest every time it is read back.
func virtioTools(ctx context.Context, root string) ([]byte, error) {
	cached := filepath.Join(root, ".mcharbor-"+virtioToolsName)

	if content, err := os.ReadFile(cached); err == nil {
		if digest(content) == virtioToolsSHA256 {
			return content, nil
		}
		// A corrupt or superseded copy is removed rather than reused: leaving
		// it would make every later build fail the same way with no path out.
		errlog.Log(os.Remove(cached))
	}

	content, err := download(ctx)
	if err != nil {
		return nil, err
	}
	if err := os.WriteFile(cached, content, 0o644); err != nil {
		// Not fatal. The disc can still be built; only the next build pays for
		// the download again.
		errlog.Log(os.Remove(cached))
	}
	return content, nil
}

// download fetches the pinned installer and verifies it before returning.
func download(ctx context.Context) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, fetchTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, virtioToolsURL, nil)
	if err != nil {
		return nil, fmt.Errorf("guestdisc: build request: %w", err)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("guestdisc: fetch guest tools: %w", err)
	}
	defer func() { errlog.Close(resp.Body) }()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("guestdisc: fetch guest tools: %s", resp.Status)
	}

	content, err := io.ReadAll(io.LimitReader(resp.Body, virtioToolsSizeBytes+1))
	if err != nil {
		return nil, fmt.Errorf("guestdisc: read guest tools: %w", err)
	}
	if sum := digest(content); sum != virtioToolsSHA256 {
		return nil, fmt.Errorf("guestdisc: guest tools digest is %s, expected %s",
			sum, virtioToolsSHA256)
	}
	return content, nil
}

// digest is the hex sha256 the pin is compared against.
func digest(content []byte) string {
	sum := sha256.Sum256(content)
	return hex.EncodeToString(sum[:])
}
