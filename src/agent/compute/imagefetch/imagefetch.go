// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// Package imagefetch downloads and verifies guest images on the host.
//
// Images never transit the McHarbor backend. A 700 MB qcow2 pushed through the
// agent WebSocket would compete with console traffic for no benefit, so the
// agent fetches straight from the distribution's own mirror to the host and
// reports progress back over the transport instead.
//
// # Verification is not optional
//
// This code runs as root and writes a file that will become a guest's boot
// disk. An unverified image is an unsigned executable with extra steps. So:
//
//   - A checksum, wherever one exists, is checked, and no flag suppresses a
//     mismatch.
//   - An image that publishes none — an operator's URL, or a catalog entry for
//     a publisher like Microsoft that ships no digest — downloads only with an
//     explicit acknowledgement that the backend records in the audit row.
//   - Where the distribution publishes a detached signature over its checksum
//     file, that signature is verified too, against a key shipped with the
//     agent rather than one fetched alongside the file it authenticates.
//
// The checksum is computed while the bytes stream to disk, so a large image is
// not read twice, and the file is only moved into place after it verifies —
// a partial or corrupt download is never visible under its final name.
package imagefetch

import (
	"context"
	"crypto/sha256"
	"crypto/sha512"
	"errors"
	"fmt"
	"hash"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/therealmcsparrow/mcharbor-agent/internal/errlog"
)

// Artifact kinds, matching compute_images.kind.
const (
	KindCloudImage = "cloud_image"
	KindRootfs     = "rootfs"
	KindISO        = "iso"
)

// downloadTimeout bounds a whole fetch. Distribution images run to a few
// gigabytes and mirrors can be slow, so this is generous — but it is finite,
// because a stalled transfer that never fails is worse than one that does.
const downloadTimeout = 6 * time.Hour

// metadataTimeout bounds the small side fetches: the checksum file and its
// signature.
const metadataTimeout = 2 * time.Minute

// maxMetadataBytes caps a checksum or signature file. These are kilobytes; a
// server answering with gigabytes is either broken or hostile.
const maxMetadataBytes = 4 << 20 // 4 MiB

// progressInterval is how often a download reports back. Every chunk would
// flood the transport; this is frequent enough for a smooth bar.
const progressInterval = 2 * time.Second

var (
	// ErrChecksumMismatch means the bytes on disk are not the bytes that were
	// promised. The partial file is removed before this is returned.
	ErrChecksumMismatch = errors.New("imagefetch: checksum mismatch")
	// ErrUnverified means no checksum was available and the caller did not
	// explicitly accept that.
	ErrUnverified = errors.New("imagefetch: refusing to accept an image with no checksum")
	// ErrSignature means a detached signature did not verify.
	ErrSignature = errors.New("imagefetch: signature verification failed")
	// ErrCancelled means the caller cancelled the fetch.
	ErrCancelled = errors.New("imagefetch: cancelled")
	// ErrInUse means an image still backs a live guest disk and cannot be
	// evicted.
	ErrInUse = errors.New("imagefetch: image is in use")
	// ErrLibraryMiss means a library image is not in the cache root, which on
	// this host almost always means the shared library is not mounted there.
	ErrLibraryMiss = errors.New("imagefetch: image is not in the shared library")
)

// Request describes one image to fetch.
type Request struct {
	ImageID string
	Kind    string
	// SourceURL must be https. A plain-http mirror would make the checksum the
	// only defence, and an attacker who can rewrite the image can rewrite the
	// checksum file beside it.
	SourceURL string
	// ChecksumURL points at a SHA256SUMS-style file listing many artifacts;
	// the entry for this image's filename is selected from it.
	ChecksumURL string
	// Checksum, when set, is used directly and ChecksumURL is not fetched.
	Checksum     string
	ChecksumAlgo string
	// SignatureURL is the distribution's signature. What it signs follows from
	// where it points: the ChecksumURL itself means the checksum file is signed
	// inline (clearsigned), SourceURL plus .asc/.sig/.gpg means a detached
	// signature over the image, and anything else a detached signature over
	// the checksum file.
	SignatureURL string
	// AllowUnverified is the operator's explicit acknowledgement. It is
	// honoured only when no checksum is available at all — it never suppresses
	// a mismatch.
	AllowUnverified bool
	// FromLibrary marks an image the operator uploaded into the shared image
	// library, which is this fetcher's cache root seen from McHarbor's side of
	// a bind mount. There is nothing to download: the file is already here, or
	// the mount is missing and no URL would help.
	FromLibrary bool
	// LibraryURL and LibraryToken let this host pull the file from the server
	// when the library directory is not shared with it. Empty means the file
	// must already be here.
	LibraryURL   string
	LibraryToken string
	// Filename is the library file's name. A library image has no SourceURL to
	// take an extension from, and the extension is what QEMU reads a disk's
	// format from.
	Filename string
}

// Result is a completed fetch.
type Result struct {
	ImageID   string
	LocalPath string
	SizeBytes int64
	Checksum  string
	Verified  bool
	Signed    bool
}

// Fetch phases, reported alongside the percentage.
//
// The percentage counts bytes off the wire, so it reaches 100 and then sits
// there through the digest and the unpack — which on a 400 MB xz is most of
// the wait. Without the phase the operator is watching a finished-looking bar
// for minutes and reasonably concludes it has hung.
const (
	PhaseDownloading   = "downloading"
	PhaseVerifying     = "verifying"
	PhaseDecompressing = "decompressing"
	PhaseInstalling    = "installing"
)

// Progress is one sample during a download.
type Progress struct {
	ImageID    string
	BytesDone  int64
	BytesTotal int64
	Percent    int
	// Phase is what the agent is doing now, not how far the bytes have got.
	Phase string
}

// Cached is one image already on the host.
type Cached struct {
	ImageID    string
	Kind       string
	LocalPath  string
	SizeBytes  int64
	Verified   bool
	InUse      bool
	LastUsedAt time.Time
}

// Fetcher downloads images into a cache directory.
type Fetcher struct {
	// root is where images live, normally /var/lib/mcharbor/compute/images.
	// Atomic because an operator can move the cache onto shared storage while
	// the agent runs, and every read below happens on a fetch goroutine.
	root atomic.Value
	// client is reused so connections are pooled across a multi-image fetch.
	// It verifies TLS strictly and must keep doing so: it talks to public
	// distribution mirrors, where a bad certificate is a reason to stop.
	client *http.Client
	// serverClient talks to McHarbor itself, for pulling an uploaded image.
	// Separate from client because the two have different trust: McHarbor is
	// commonly reached on a LAN address its self-signed certificate does not
	// cover, which the operator has already accepted for the control channel
	// and every archive transfer. Relaxing the mirror client for that would
	// silently weaken every distribution download too. nil falls back to the
	// strict client.
	serverClient *http.Client
	// keyring holds the distribution signing keys shipped with the agent.
	keyring Keyring

	// inFlight tracks running fetches so a second request for the same image
	// joins rather than starting a competing download onto the same path.
	mu       sync.Mutex
	inFlight map[string]context.CancelFunc
}

// Keyring verifies distribution signatures. It is an interface so the OpenPGP
// implementation stays optional and testable.
type Keyring interface {
	// Verify checks a detached signature over signed. A nil error means the
	// signature is good and made by a key this agent ships.
	Verify(signed, signature []byte) error
	// VerifyReader is Verify over a stream, for a signature over an image too
	// large to hold in memory.
	VerifyReader(signed io.Reader, signature []byte) error
	// VerifyClearsigned checks an inline-signed document and returns only the
	// text the signature covers, so nothing outside it is ever parsed.
	VerifyClearsigned(document []byte) ([]byte, error)
}

// Signature modes, chosen by signatureModeOf.
const (
	sigNone     = ""
	sigChecksum = "checksum"
	sigInline   = "inline"
	sigImage    = "image"
)

// signatureModeOf says what req's signature covers; see Request.SignatureURL.
func signatureModeOf(req Request) string {
	switch {
	case req.SignatureURL == "":
		return sigNone
	case req.ChecksumURL != "" && req.SignatureURL == req.ChecksumURL:
		return sigInline
	case req.SignatureURL == req.SourceURL+".asc",
		req.SignatureURL == req.SourceURL+".sig",
		req.SignatureURL == req.SourceURL+".gpg":
		return sigImage
	default:
		return sigChecksum
	}
}

// New builds a fetcher rooted at a cache directory.
func New(root string, keyring Keyring) (*Fetcher, error) {
	if root == "" {
		return nil, errors.New("imagefetch: empty cache root")
	}
	if err := os.MkdirAll(root, 0o700); err != nil {
		return nil, fmt.Errorf("imagefetch: create cache %s: %w", root, err)
	}
	f := &Fetcher{keyring: keyring,
		client:   newMirrorClient(false),
		inFlight: make(map[string]context.CancelFunc),
	}
	f.root.Store(root)
	return f, nil
}

// SetRoot moves the cache directory.
//
// An in-flight download keeps writing to the directory it started in — the
// temporary file is already open there — so the caller changes the root only
// when nothing is being fetched.
func (f *Fetcher) SetRoot(root string) error {
	if root == "" {
		return errors.New("imagefetch: empty cache root")
	}
	if err := os.MkdirAll(root, 0o700); err != nil {
		return fmt.Errorf("imagefetch: create cache %s: %w", root, err)
	}
	f.root.Store(root)
	return nil
}

// UseServerClient sets the client used to pull from McHarbor, which carries
// the agent's own TLS settings rather than the mirror client's.
func (f *Fetcher) UseServerClient(client *http.Client) {
	f.serverClient = client
}

// ServerClient returns the client used to pull from McHarbor, or nil when the
// agent wired none. Shared with the import path, which pulls a staged upload
// from the same place and must honour the same TLS settings.
func (f *Fetcher) ServerClient() *http.Client { return f.serverClient }

// Root returns the cache directory.
func (f *Fetcher) Root() string {
	root, _ := f.root.Load().(string)
	return root
}

// Fetch downloads and verifies an image, reporting progress through onProgress.
//
// onProgress may be nil. It is called from the calling goroutine, never
// concurrently, so an implementation does not need its own locking.
func (f *Fetcher) Fetch(ctx context.Context, req Request, onProgress func(Progress)) (Result, error) {
	if req.ImageID == "" {
		return Result{}, errors.New("imagefetch: empty image id")
	}
	// Checked before the URL, because a library image has no URL and would
	// otherwise be rejected by a rule written for mirrors.
	if req.FromLibrary {
		return f.adoptLibrary(ctx, req, onProgress)
	}
	if err := validateSourceURL(req.SourceURL); err != nil {
		return Result{}, err
	}

	target, err := f.imagePath(req)
	if err != nil {
		return Result{}, err
	}

	// An image already present and verified is not downloaded again. This is
	// what makes "create ten VMs from Ubuntu 24.04" fetch once.
	if info, err := os.Stat(target); err == nil && info.Size() > 0 {
		return Result{
			ImageID:   req.ImageID,
			LocalPath: target,
			SizeBytes: info.Size(),
			Verified:  true,
		}, nil
	}

	ctx, cancel := context.WithTimeout(ctx, downloadTimeout)
	defer cancel()

	f.mu.Lock()
	if _, running := f.inFlight[req.ImageID]; running {
		f.mu.Unlock()
		return Result{}, fmt.Errorf("imagefetch: %s is already downloading", req.ImageID)
	}
	f.inFlight[req.ImageID] = cancel
	f.mu.Unlock()
	defer func() {
		f.mu.Lock()
		delete(f.inFlight, req.ImageID)
		f.mu.Unlock()
	}()

	// Resolve the expected checksum before spending bandwidth, so an image
	// that could never have verified fails in seconds rather than after a
	// multi-gigabyte download.
	expected, algo, signed, err := f.resolveChecksum(ctx, req)
	if err != nil {
		return Result{}, err
	}
	if expected == "" && !req.AllowUnverified {
		return Result{}, fmt.Errorf("%w: %s", ErrUnverified, req.SourceURL)
	}

	// A signature over the image is fetched before the download, so a missing
	// one fails in seconds, and checked against the bytes as served, before any
	// decompression and before the file takes its final name.
	var verifyDownload func(path string) error
	if signatureModeOf(req) == sigImage {
		verifyDownload, err = f.imageSignatureCheck(ctx, req)
		if err != nil {
			return Result{}, err
		}
		signed = true
	}

	sum, size, err := f.download(ctx, req, target, expected, algo, verifyDownload, onProgress)
	if err != nil {
		return Result{}, err
	}

	return Result{
		ImageID:   req.ImageID,
		LocalPath: target,
		SizeBytes: size,
		Checksum:  sum,
		Verified:  expected != "",
		Signed:    signed,
	}, nil
}

// adoptLibrary accepts an image the operator uploaded into the shared library.
//
// Two shapes of host. One mounts the same storage McHarbor writes uploads to,
// and the bytes are already here — nothing transfers, which is the whole point
// of a shared library. The other is a remote host with its own disk, where the
// file cannot appear by itself; that one is given a URL to pull it from, and
// falls back to doing so.
//
// Without the URL there is still nothing to try, which is why the miss keeps
// its own error: no mirror or retry fixes an unmounted library.
func (f *Fetcher) adoptLibrary(ctx context.Context, req Request, onProgress func(Progress)) (Result, error) {
	target, err := f.pathFor(req.ImageID, req.SourceURL, req.Filename, req.Kind)
	if err != nil {
		return Result{}, err
	}

	info, err := os.Stat(target)
	if (err != nil || info.Size() == 0) && req.LibraryURL != "" {
		if err := f.pullLibrary(ctx, req, target, onProgress); err != nil {
			return Result{}, err
		}
		info, err = os.Stat(target)
	}
	if err != nil || info.Size() == 0 {
		return Result{}, fmt.Errorf("%w: %s", ErrLibraryMiss, target)
	}

	if req.Checksum == "" {
		if !req.AllowUnverified {
			return Result{}, fmt.Errorf("%w: %s", ErrUnverified, target)
		}
		return Result{ImageID: req.ImageID, LocalPath: target, SizeBytes: info.Size()}, nil
	}

	// McHarbor hashed these bytes as it received them. Re-hashing here is what
	// makes the mount itself part of the trust chain: a library on shared
	// storage is writable by things that are not McHarbor.
	sum, err := f.verifyLibraryFile(ctx, target, req, info.Size(), onProgress)
	if err != nil {
		return Result{}, err
	}
	if !strings.EqualFold(sum, req.Checksum) {
		// Left in place rather than deleted. The library is McHarbor's, this
		// file may be the operator's only copy, and an agent that quietly
		// removes it turns a mismatch into data loss.
		return Result{}, fmt.Errorf("%w: %s expected %s, got %s",
			ErrChecksumMismatch, target, req.Checksum, sum)
	}

	return Result{
		ImageID:   req.ImageID,
		LocalPath: target,
		SizeBytes: info.Size(),
		Checksum:  sum,
		Verified:  true,
	}, nil
}

// verifyLibraryFile hashes a library file, reporting progress on the same
// channel a download uses so the UI needs no second notion of "working".
func (f *Fetcher) verifyLibraryFile(
	ctx context.Context,
	target string,
	req Request,
	size int64,
	onProgress func(Progress),
) (string, error) {
	hasher, err := newHasher(req.ChecksumAlgo)
	if err != nil {
		return "", err
	}

	file, err := os.Open(target)
	if err != nil {
		return "", fmt.Errorf("imagefetch: open %s: %w", target, err)
	}
	defer func() { errlog.Close(file) }()

	if _, err := copyWithProgress(ctx, io.Discard, file, hasher, req.ImageID, size, onProgress); err != nil {
		return "", err
	}
	return fmt.Sprintf("%x", hasher.Sum(nil)), nil
}

// Cancel aborts an in-flight fetch. The partial file is removed by the
// download path's own cleanup, so a cancelled image never lingers.
func (f *Fetcher) Cancel(imageID string) bool {
	f.mu.Lock()
	cancel, ok := f.inFlight[imageID]
	f.mu.Unlock()
	if ok {
		cancel()
	}
	return ok
}

// download streams the body to a temporary file, hashing as it goes, and moves
// it into place only after it verifies.
//
// The temporary name matters: a partially written image under its final name
// would be picked up by the "already present" check above and handed to a guest
// as a boot disk.
func (f *Fetcher) download(
	ctx context.Context,
	req Request,
	target, expected, algo string,
	verifyDownload func(path string) error,
	onProgress func(Progress),
) (string, int64, error) {
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, req.SourceURL, nil)
	if err != nil {
		return "", 0, fmt.Errorf("imagefetch: build request: %w", err)
	}
	httpReq.Header.Set("User-Agent", userAgent)

	resp, err := f.client.Do(httpReq)
	if err != nil {
		if ctx.Err() != nil {
			return "", 0, ErrCancelled
		}
		return "", 0, fmt.Errorf("imagefetch: GET %s: %w", req.SourceURL, err)
	}
	defer func() { errlog.Log(resp.Body.Close()) }()

	if resp.StatusCode != http.StatusOK {
		return "", 0, fmt.Errorf("imagefetch: GET %s: HTTP %d", req.SourceURL, resp.StatusCode)
	}

	tmp, err := os.CreateTemp(f.Root(), ".partial-*")
	if err != nil {
		return "", 0, fmt.Errorf("imagefetch: create temp file: %w", err)
	}
	tmpName := tmp.Name()
	// Removed on every failure path including cancellation, so a interrupted
	// download leaves nothing behind.
	committed := false
	defer func() {
		errlog.Close(tmp)
		if !committed {
			errlog.Log(os.Remove(tmpName))
		}
	}()

	hasher, err := newHasher(algo)
	if err != nil {
		return "", 0, err
	}

	written, err := copyWithProgress(ctx, tmp, resp.Body, hasher, req.ImageID,
		resp.ContentLength, onProgress)
	if err != nil {
		return "", 0, err
	}
	if err := tmp.Sync(); err != nil {
		return "", 0, fmt.Errorf("imagefetch: flush %s: %w", tmpName, err)
	}
	if err := tmp.Close(); err != nil {
		return "", 0, fmt.Errorf("imagefetch: close %s: %w", tmpName, err)
	}

	reportPhase(onProgress, req.ImageID, PhaseVerifying, written, written)
	sum := fmt.Sprintf("%x", hasher.Sum(nil))
	if expected != "" && !strings.EqualFold(sum, expected) {
		return "", 0, fmt.Errorf("%w: %s expected %s, got %s",
			ErrChecksumMismatch, req.SourceURL, expected, sum)
	}
	if verifyDownload != nil {
		if err := verifyDownload(tmpName); err != nil {
			return "", 0, err
		}
	}

	// Unpacking happens after the digest matches, never before: the checksum a
	// distribution publishes covers the compressed artifact it serves.
	staged := tmpName
	unpacked := ""
	if algo, _ := compressionPlan(req); algo != "" {
		reportPhase(onProgress, req.ImageID, PhaseDecompressing, written, written)
		name, size, err := decompressFile(ctx, tmpName, f.Root(), algo, req.ImageID)
		if err != nil {
			return "", 0, err
		}
		staged, unpacked, written = name, name, size
	}
	installed := false
	defer func() {
		if unpacked != "" && !installed {
			errlog.Log(os.Remove(unpacked))
		}
	}()

	reportPhase(onProgress, req.ImageID, PhaseInstalling, written, written)
	if err := os.Chmod(staged, 0o600); err != nil {
		return "", 0, fmt.Errorf("imagefetch: chmod %s: %w", staged, err)
	}
	// Rename is atomic within a filesystem, so the final name appears only
	// once the content behind it is complete and verified.
	if err := os.Rename(staged, target); err != nil {
		return "", 0, fmt.Errorf("imagefetch: install %s: %w", target, err)
	}
	installed = true
	// The compressed download is left to the deferred cleanup above unless it
	// is itself the file that was installed.
	if unpacked == "" {
		committed = true
	}

	return sum, written, nil
}

// copyWithProgress streams src to dst and the hasher together, reporting
// progress on a timer rather than per chunk.
func copyWithProgress(
	ctx context.Context,
	dst io.Writer,
	src io.Reader,
	hasher hash.Hash,
	imageID string,
	total int64,
	onProgress func(Progress),
) (int64, error) {
	// One 1 MiB buffer for the whole transfer. Hashing happens through a
	// MultiWriter so the bytes are read once, not once to write and again to
	// verify.
	buf := make([]byte, 1<<20)
	// A nil hasher means the caller is not verifying as it copies — the library
	// pull re-hashes afterwards instead. MultiWriter would panic on the nil
	// interface, so it is left out rather than passed in.
	sink := io.Writer(dst)
	if hasher != nil {
		sink = io.MultiWriter(dst, hasher)
	}

	var written int64
	lastReport := time.Now()

	for {
		select {
		case <-ctx.Done():
			return written, ErrCancelled
		default:
		}

		n, readErr := src.Read(buf)
		if n > 0 {
			if _, err := sink.Write(buf[:n]); err != nil {
				return written, fmt.Errorf("imagefetch: write: %w", err)
			}
			written += int64(n)

			if onProgress != nil && time.Since(lastReport) >= progressInterval {
				lastReport = time.Now()
				onProgress(Progress{
					ImageID:    imageID,
					BytesDone:  written,
					BytesTotal: total,
					Percent:    percent(written, total),
					Phase:      PhaseDownloading,
				})
			}
		}
		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			if ctx.Err() != nil {
				return written, ErrCancelled
			}
			return written, fmt.Errorf("imagefetch: read: %w", readErr)
		}
	}

	if onProgress != nil {
		onProgress(Progress{
			ImageID:    imageID,
			BytesDone:  written,
			BytesTotal: total,
			Percent:    100,
			Phase:      PhaseDownloading,
		})
	}
	return written, nil
}

// reportPhase says what is happening now, keeping the byte counters so the bar
// does not jump backwards.
func reportPhase(onProgress func(Progress), imageID, phase string, done, total int64) {
	if onProgress == nil {
		return
	}
	onProgress(Progress{
		ImageID: imageID, BytesDone: done, BytesTotal: total,
		Percent: 100, Phase: phase,
	})
}

func percent(done, total int64) int {
	if total <= 0 {
		return -1
	}
	pct := int(done * 100 / total)
	if pct > 100 {
		return 100
	}
	return pct
}

// resolveChecksum works out what the image's hash should be.
//
// An explicit Checksum wins. Otherwise the checksum file is fetched and, when a
// signature is available, verified before a single line of it is trusted —
// verifying after parsing would mean acting on attacker-controlled data first.
func (f *Fetcher) resolveChecksum(ctx context.Context, req Request) (sum, algo string, signed bool, err error) {
	algo = req.ChecksumAlgo
	if algo == "" {
		algo = "sha256"
	}

	if req.Checksum != "" {
		return strings.TrimSpace(req.Checksum), algo, false, nil
	}
	if req.ChecksumURL == "" {
		return "", algo, false, nil
	}
	if err := validateSourceURL(req.ChecksumURL); err != nil {
		return "", algo, false, err
	}

	body, err := f.fetchMetadata(ctx, req.ChecksumURL)
	if err != nil {
		return "", algo, false, err
	}

	switch signatureModeOf(req) {
	case sigChecksum:
		if err := validateSourceURL(req.SignatureURL); err != nil {
			return "", algo, false, err
		}
		if f.keyring == nil {
			return "", algo, false, fmt.Errorf(
				"%w: a signature was published for %s but this agent ships no keyring",
				ErrSignature, req.ChecksumURL)
		}
		sig, err := f.fetchMetadata(ctx, req.SignatureURL)
		if err != nil {
			return "", algo, false, err
		}
		if err := f.keyring.Verify(body, sig); err != nil {
			return "", algo, false, fmt.Errorf("%w: %s: %w", ErrSignature, req.ChecksumURL, err)
		}
		signed = true
	case sigInline:
		if f.keyring == nil {
			return "", algo, false, fmt.Errorf(
				"%w: %s is signed inline but this agent ships no keyring", ErrSignature, req.ChecksumURL)
		}
		text, err := f.keyring.VerifyClearsigned(body)
		if err != nil {
			return "", algo, false, fmt.Errorf("%w: %s: %w", ErrSignature, req.ChecksumURL, err)
		}
		body = text
		signed = true
	}

	filename := filenameFromURL(req.SourceURL)
	sum, err = findChecksum(string(body), filename)
	if err != nil {
		return "", algo, signed, err
	}
	return sum, algo, signed, nil
}

// imageSignatureCheck fetches a detached signature over the image and returns
// the check to run on the downloaded file.
func (f *Fetcher) imageSignatureCheck(ctx context.Context, req Request) (func(string) error, error) {
	if err := validateSourceURL(req.SignatureURL); err != nil {
		return nil, err
	}
	if f.keyring == nil {
		return nil, fmt.Errorf("%w: %s is signed but this agent ships no keyring", ErrSignature, req.SourceURL)
	}
	sig, err := f.fetchMetadata(ctx, req.SignatureURL)
	if err != nil {
		return nil, err
	}
	return func(path string) error {
		file, err := os.Open(path) //nolint:gosec // path is this fetcher's own temp file
		if err != nil {
			return fmt.Errorf("imagefetch: reopen %s for its signature: %w", path, err)
		}
		defer errlog.Close(file)
		if err := f.keyring.VerifyReader(file, sig); err != nil {
			return fmt.Errorf("%w: %s: %w", ErrSignature, req.SourceURL, err)
		}
		return nil
	}, nil
}

// fetchMetadata retrieves a small side file under a tight timeout and size cap.
func (f *Fetcher) fetchMetadata(ctx context.Context, rawURL string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, metadataTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, fmt.Errorf("imagefetch: build request for %s: %w", rawURL, err)
	}
	req.Header.Set("User-Agent", userAgent)

	resp, err := f.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("imagefetch: GET %s: %w", rawURL, err)
	}
	defer func() { errlog.Log(resp.Body.Close()) }()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("imagefetch: GET %s: HTTP %d", rawURL, resp.StatusCode)
	}
	// LimitReader rather than trusting Content-Length, which a server is free
	// to understate.
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxMetadataBytes))
	if err != nil {
		return nil, fmt.Errorf("imagefetch: read %s: %w", rawURL, err)
	}
	return body, nil
}

// findChecksum picks this image's line out of a SHA256SUMS-style file.
//
// The format is "<hex>  <filename>", with distributions varying on the
// separator and on a leading "*" for binary mode, so matching is on the
// filename field rather than on exact line shape.
func findChecksum(body, filename string) (string, error) {
	if filename == "" {
		return "", errors.New("imagefetch: cannot match a checksum without a filename")
	}
	for _, line := range strings.Split(body, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		name := strings.TrimPrefix(fields[len(fields)-1], "*")
		if name == filename || filepath.Base(name) == filename {
			return fields[0], nil
		}
	}
	return "", fmt.Errorf("imagefetch: no checksum for %q in the published checksum file", filename)
}

// pullLibrary downloads an uploaded image from McHarbor itself, for a host
// that does not share the library directory.
//
// The bytes land in a temporary file beside the target and are renamed only
// once the transfer completes, so an interrupted pull never leaves a partial
// file under the name the adopt path will then trust. Verification is the
// caller's: adoptLibrary re-hashes whatever ends up at target, by the same rule
// it applies to a file that was already there.
func (f *Fetcher) pullLibrary(ctx context.Context, req Request, target string, onProgress func(Progress)) error {
	ctx, cancel := context.WithTimeout(ctx, downloadTimeout)
	defer cancel()

	request, err := http.NewRequestWithContext(ctx, http.MethodGet, req.LibraryURL, nil)
	if err != nil {
		return fmt.Errorf("imagefetch: building library request: %w", err)
	}
	if req.LibraryToken != "" {
		request.Header.Set("Authorization", "Bearer "+req.LibraryToken)
	}

	client := f.serverClient
	if client == nil {
		client = f.client
	}
	resp, err := client.Do(request)
	if err != nil {
		return fmt.Errorf("imagefetch: pulling library image: %w", err)
	}
	defer func() { errlog.Log(resp.Body.Close()) }()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("imagefetch: library image download returned %d", resp.StatusCode)
	}

	if err := os.MkdirAll(filepath.Dir(target), 0o700); err != nil {
		return fmt.Errorf("imagefetch: preparing library directory: %w", err)
	}
	tmp, err := os.CreateTemp(filepath.Dir(target), ".mcharbor-library-*")
	if err != nil {
		return fmt.Errorf("imagefetch: staging library image: %w", err)
	}
	tmpName := tmp.Name()
	defer func() {
		errlog.Close(tmp)
		errlog.Log(os.Remove(tmpName))
	}()

	if err := os.Chmod(tmpName, 0o600); err != nil {
		return fmt.Errorf("imagefetch: securing library image: %w", err)
	}
	if _, err := copyWithProgress(ctx, tmp, resp.Body, nil, req.ImageID, resp.ContentLength, onProgress); err != nil {
		return err
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("imagefetch: closing library image: %w", err)
	}
	if err := os.Rename(tmpName, target); err != nil {
		return fmt.Errorf("imagefetch: publishing library image: %w", err)
	}
	return nil
}

// compressionPlan reports how a downloaded artifact is compressed and what the
// disk inside it is called, for the kinds where a single file is a boot disk.
//
// A rootfs is exempt: it arrives as an archive that the container path unpacks
// itself, and handing it a bare stream would lose the file tree.
func compressionPlan(req Request) (algo string, innerExt string) {
	if req.Kind == KindRootfs {
		return "", ""
	}
	name := filenameFromURL(req.SourceURL)
	if name == "" {
		name = req.Filename
	}
	return compressionFor(name)
}

// imagePath is pathFor with the compression suffix resolved away, so an image
// published as `…qcow2.xz` is cached, and looked up, under the name it will
// actually have once unpacked.
func (f *Fetcher) imagePath(req Request) (string, error) {
	target, err := f.pathFor(req.ImageID, req.SourceURL, req.Filename, req.Kind)
	if err != nil {
		return "", err
	}
	algo, innerExt := compressionPlan(req)
	if algo == "" {
		return target, nil
	}
	if innerExt == "" {
		innerExt = extensionForKind(req.Kind)
	}
	return strings.TrimSuffix(target, filepath.Ext(target)) + innerExt, nil
}

// pathFor builds the cache path for an image.
//
// The name is derived from the image id and the URL's filename extension, never
// from the URL path itself: a source URL is remote input, and letting it choose
// a path component in a root-owned cache directory is a traversal waiting to
// happen.
func (f *Fetcher) pathFor(imageID, sourceURL, name, kind string) (string, error) {
	if err := validateImageID(imageID); err != nil {
		return "", err
	}
	ext := filepath.Ext(filenameFromURL(sourceURL))
	if ext == "" && name != "" {
		ext = filepath.Ext(name)
	}
	// Only a short, known-safe extension is carried over, for readability in
	// the cache directory.
	switch strings.ToLower(ext) {
	case ".qcow2", ".img", ".iso", ".raw", ".gz", ".xz", ".zst", ".tar", ".bz2":
	default:
		ext = ""
	}
	if ext == "" {
		ext = extensionForKind(kind)
	}
	return filepath.Join(f.Root(), imageID+ext), nil
}

// extensionForKind names a file from what McHarbor says it is.
//
// The extension is not decoration: the backend refuses an installer disc whose
// path does not end in .iso or .img, and QEMU reads a disk's format from it. A
// library image carries no source URL to take one from, so when the filename
// does not arrive either, the kind is the remaining authority — and it is the
// same fact the extension was encoding all along.
//
// Only the two kinds with an unambiguous extension are named. A rootfs arrives
// compressed in several shapes and guessing one would be worse than leaving it
// bare.
func extensionForKind(kind string) string {
	switch kind {
	case KindISO:
		return ".iso"
	case KindCloudImage:
		return ".img"
	default:
		return ""
	}
}

// validateImageID rejects anything that is not a plain identifier.
func validateImageID(id string) error {
	if id == "" {
		return errors.New("imagefetch: empty image id")
	}
	if len(id) > 64 {
		return fmt.Errorf("imagefetch: image id %q is too long", id)
	}
	for _, r := range id {
		switch {
		case r >= 'a' && r <= 'z',
			r >= 'A' && r <= 'Z',
			r >= '0' && r <= '9',
			r == '-', r == '_':
		default:
			return fmt.Errorf("imagefetch: image id %q contains an invalid character %q", id, r)
		}
	}
	return nil
}

// validateSourceURL requires https.
//
// Plain http would leave the checksum as the only integrity defence, and
// anyone able to rewrite the image in flight can rewrite the checksum file
// fetched over the same channel. The signature path is the only thing that
// would survive that, and not every distribution publishes one.
func validateSourceURL(raw string) error {
	if raw == "" {
		return errors.New("imagefetch: empty URL")
	}
	parsed, err := url.Parse(raw)
	if err != nil {
		return fmt.Errorf("imagefetch: invalid URL %q: %w", raw, err)
	}
	if parsed.Scheme != "https" {
		return fmt.Errorf("imagefetch: refusing %q: image sources must be https", raw)
	}
	if parsed.Host == "" {
		return fmt.Errorf("imagefetch: URL %q has no host", raw)
	}
	return nil
}

func filenameFromURL(raw string) string {
	parsed, err := url.Parse(raw)
	if err != nil {
		return ""
	}
	return filepath.Base(parsed.Path)
}

func newHasher(algo string) (hash.Hash, error) {
	switch strings.ToLower(algo) {
	case "", "sha256":
		return sha256.New(), nil
	case "sha512":
		return sha512.New(), nil
	default:
		return nil, fmt.Errorf("imagefetch: unsupported checksum algorithm %q", algo)
	}
}

const userAgent = "mcharbor-agent"

// List reports what is cached.
//
// inUse is supplied by the caller because only it knows which images back a
// live guest disk. Those are never evicted, whatever the LRU says.
func (f *Fetcher) List(inUse map[string]bool) ([]Cached, error) {
	entries, err := os.ReadDir(f.Root())
	if err != nil {
		return nil, fmt.Errorf("imagefetch: read cache: %w", err)
	}

	out := make([]Cached, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || strings.HasPrefix(entry.Name(), ".partial-") {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			continue
		}
		id := strings.TrimSuffix(entry.Name(), filepath.Ext(entry.Name()))
		out = append(out, Cached{
			ImageID:    id,
			LocalPath:  filepath.Join(f.Root(), entry.Name()),
			SizeBytes:  info.Size(),
			Verified:   true,
			InUse:      inUse[id],
			LastUsedAt: info.ModTime(),
		})
	}
	return out, nil
}

// Remove evicts a cached image, refusing while it backs a live guest.
func (f *Fetcher) Remove(imageID string, inUse map[string]bool) error {
	if err := validateImageID(imageID); err != nil {
		return err
	}
	if inUse[imageID] {
		return fmt.Errorf("%w: %s backs a running guest's disk", ErrInUse, imageID)
	}

	entries, err := os.ReadDir(f.Root())
	if err != nil {
		return fmt.Errorf("imagefetch: read cache: %w", err)
	}
	for _, entry := range entries {
		name := entry.Name()
		if strings.TrimSuffix(name, filepath.Ext(name)) != imageID {
			continue
		}
		// Joined against the cache root, and the id was validated above, so
		// this cannot address anything outside the cache.
		if err := os.Remove(filepath.Join(f.Root(), name)); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("imagefetch: remove %s: %w", name, err)
		}
	}
	return nil
}

// Evict enforces the per-environment cache cap, removing least-recently-used
// images until the total fits.
//
// Images backing a live guest are skipped regardless of age: reclaiming space
// by deleting a running VM's backing file would corrupt the guest, which is a
// far worse outcome than a full cache.
func (f *Fetcher) Evict(capBytes int64, inUse map[string]bool) ([]string, error) {
	if capBytes <= 0 {
		return nil, nil
	}
	cached, err := f.List(inUse)
	if err != nil {
		return nil, err
	}

	var total int64
	for _, image := range cached {
		total += image.SizeBytes
	}
	if total <= capBytes {
		return nil, nil
	}

	// Oldest first.
	for i := 0; i < len(cached); i++ {
		for j := i + 1; j < len(cached); j++ {
			if cached[j].LastUsedAt.Before(cached[i].LastUsedAt) {
				cached[i], cached[j] = cached[j], cached[i]
			}
		}
	}

	var evicted []string
	for _, image := range cached {
		if total <= capBytes {
			break
		}
		if image.InUse {
			continue
		}
		if err := os.Remove(image.LocalPath); err != nil {
			continue
		}
		total -= image.SizeBytes
		evicted = append(evicted, image.ImageID)
	}
	return evicted, nil
}
