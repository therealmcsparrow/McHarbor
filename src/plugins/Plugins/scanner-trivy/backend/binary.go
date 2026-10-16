// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sync"
	"time"
)

// pinnedTrivyVersion is the release this plugin build downloads when no trivy is on PATH. Moving
// it means moving every checksum below in the same change.
const pinnedTrivyVersion = "0.75.0"

// trivyArchives pins the SHA-256 of each release archive, from the release's own checksums
// file, so a download that does not match is refused rather than run.
var trivyArchives = map[string]struct{ name, sha256 string }{
	"amd64": {"trivy_" + pinnedTrivyVersion + "_Linux-64bit.tar.gz", "c6e65abddb348e25f10549df887045629cf28cc72453cd1c63acb717316b3f3f"},
	"arm64": {"trivy_" + pinnedTrivyVersion + "_Linux-ARM64.tar.gz", "a1ee9f6ffb7d112b64ff726a2a0717c21175c1114361391f4a132956751a13b3"},
}

// trivyReleaseURL is a variable so tests can point it at a local server.
var trivyReleaseURL = "https://github.com/aquasecurity/trivy/releases/download/v" + pinnedTrivyVersion + "/"

// maxArchiveBytes bounds a download; the real archives are about 50 MiB.
const maxArchiveBytes = 300 << 20

// binaryInstaller keeps a private trivy in the plugin's own install directory. The host
// deletes that directory on uninstall, which is what removes the binary again.
type binaryInstaller struct {
	dir    string
	client *http.Client

	mu      sync.Mutex
	running bool
	lastErr error

	cancel context.CancelFunc
	done   chan struct{}
}

func newBinaryInstaller(dir string) *binaryInstaller {
	return &binaryInstaller{dir: dir, client: &http.Client{Timeout: 10 * time.Minute}}
}

// path is the binary to run: one on PATH first, as the container image ships it, then the
// plugin's own copy. Empty when there is neither.
func (b *binaryInstaller) path() string {
	if p, err := exec.LookPath("trivy"); err == nil {
		return p
	}
	if b == nil || b.dir == "" {
		return ""
	}
	own := filepath.Join(b.dir, "trivy")
	if info, err := os.Stat(own); err == nil && info.Mode().IsRegular() {
		return own
	}
	return ""
}

// status says why no binary is available yet, for the test step and the error a scan returns.
func (b *binaryInstaller) status() string {
	if b == nil {
		return "the trivy binary is not on McHarbor's PATH"
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	switch {
	case b.running:
		return "trivy " + pinnedTrivyVersion + " is being downloaded into the plugin folder; try again in a minute"
	case b.lastErr != nil:
		return "downloading trivy " + pinnedTrivyVersion + " failed: " + b.lastErr.Error()
	}
	return "the trivy binary is not on McHarbor's PATH and the plugin has not downloaded its own yet"
}

// start downloads the pinned release in the background unless a binary is already there.
// The goroutine ends when the download does or when stop cancels it.
func (b *binaryInstaller) start(log func(msg string, args ...any)) {
	if b.path() != "" {
		return
	}
	if _, ok := trivyArchives[runtime.GOARCH]; !ok || runtime.GOOS != "linux" {
		b.setErr(fmt.Errorf("no pinned trivy release for %s/%s", runtime.GOOS, runtime.GOARCH))
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	b.mu.Lock()
	b.running, b.lastErr, b.cancel, b.done = true, nil, cancel, make(chan struct{})
	done := b.done
	b.mu.Unlock()
	go func() { // goroutine lifecycle: ends with the download, or when stop cancels ctx
		defer close(done)
		err := b.install(ctx, runtime.GOARCH)
		b.mu.Lock()
		b.running, b.lastErr = false, err
		b.mu.Unlock()
		if err != nil && log != nil {
			log("scanner-trivy: downloading trivy failed", "version", pinnedTrivyVersion, "error", err)
		}
	}()
}

// stop cancels a download in flight and waits for it to give up.
func (b *binaryInstaller) stop() {
	b.mu.Lock()
	cancel, done := b.cancel, b.done
	b.mu.Unlock()
	if cancel != nil {
		cancel()
		<-done
	}
}

func (b *binaryInstaller) setErr(err error) {
	b.mu.Lock()
	b.lastErr = err
	b.mu.Unlock()
}

// install fetches the archive for arch, refuses it unless its SHA-256 matches the pin, and
// moves the extracted binary into place only once it is complete.
func (b *binaryInstaller) install(ctx context.Context, arch string) error {
	pin := trivyArchives[arch]
	if err := os.MkdirAll(b.dir, 0o755); err != nil {
		return fmt.Errorf("creating %s: %w", b.dir, err)
	}
	archive, err := os.CreateTemp(b.dir, ".download-*")
	if err != nil {
		return fmt.Errorf("creating download file: %w", err)
	}
	defer func() {
		_ = archive.Close()
		_ = os.Remove(archive.Name())
	}()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, trivyReleaseURL+pin.name, nil)
	if err != nil {
		return err
	}
	resp, err := b.client.Do(req)
	if err != nil {
		return fmt.Errorf("fetching %s: %w", pin.name, err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("fetching %s: HTTP %d", pin.name, resp.StatusCode)
	}
	sum := sha256.New()
	n, err := io.Copy(io.MultiWriter(archive, sum), io.LimitReader(resp.Body, maxArchiveBytes+1))
	if err != nil {
		return fmt.Errorf("downloading %s: %w", pin.name, err)
	}
	if n > maxArchiveBytes {
		return fmt.Errorf("%s is larger than %d bytes", pin.name, maxArchiveBytes)
	}
	if got := hex.EncodeToString(sum.Sum(nil)); got != pin.sha256 {
		return fmt.Errorf("%s checksum %s does not match the pinned %s", pin.name, got, pin.sha256)
	}
	if _, err := archive.Seek(0, io.SeekStart); err != nil {
		return err
	}
	return extractBinary(archive, "trivy", b.dir)
}

// extractBinary copies the one regular file named name at the archive root into dir,
// through a temporary file so a half-written binary is never found on disk.
func extractBinary(r io.Reader, name, dir string) error {
	gz, err := gzip.NewReader(r)
	if err != nil {
		return fmt.Errorf("reading archive: %w", err)
	}
	defer func() { _ = gz.Close() }()
	tr := tar.NewReader(gz)
	for {
		hdr, err := tr.Next()
		if errors.Is(err, io.EOF) {
			return fmt.Errorf("the archive holds no %s binary", name)
		}
		if err != nil {
			return fmt.Errorf("reading archive: %w", err)
		}
		if hdr.Typeflag != tar.TypeReg || filepath.Clean(hdr.Name) != name {
			continue
		}
		tmp, err := os.CreateTemp(dir, ".binary-*")
		if err != nil {
			return err
		}
		if _, err := io.Copy(tmp, io.LimitReader(tr, maxArchiveBytes)); err != nil {
			_ = tmp.Close()
			_ = os.Remove(tmp.Name())
			return fmt.Errorf("extracting %s: %w", name, err)
		}
		if err := tmp.Chmod(0o755); err != nil {
			_ = tmp.Close()
			_ = os.Remove(tmp.Name())
			return err
		}
		if err := tmp.Close(); err != nil {
			_ = os.Remove(tmp.Name())
			return err
		}
		return os.Rename(tmp.Name(), filepath.Join(dir, name))
	}
}
