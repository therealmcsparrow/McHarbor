// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package imagefetch

import (
	"compress/bzip2"
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/klauspost/compress/zstd"
	"github.com/therealmcsparrow/mcharbor-agent/internal/errlog"
	"github.com/ulikunitz/xz"
)

// compressionAlgos maps a single-file compression suffix to the algorithm that
// unpacks it. QEMU reads none of these: an image left compressed is a file a
// guest cannot boot from.
var compressionAlgos = map[string]string{
	".xz":  "xz",
	".gz":  "gzip",
	".zst": "zstd",
	".bz2": "bzip2",
}

// diskExtensions are the inner names worth carrying over to the unpacked file,
// because QEMU reads a disk's format from its extension.
var diskExtensions = map[string]bool{".qcow2": true, ".img": true, ".iso": true, ".raw": true}

// compressionFor reports how name is compressed and what it is called
// underneath, or "" when the bytes are already a disk image.
//
// A `.tar.*` name is an archive holding many files, which is the rootfs path's
// business and never a guest disk, so it is left compressed on purpose.
func compressionFor(name string) (algo string, innerExt string) {
	outer := strings.ToLower(filepath.Ext(name))
	algo, ok := compressionAlgos[outer]
	if !ok {
		return "", ""
	}
	inner := strings.ToLower(filepath.Ext(strings.TrimSuffix(name, filepath.Ext(name))))
	if inner == ".tar" {
		return "", ""
	}
	if !diskExtensions[inner] {
		inner = ""
	}
	return algo, inner
}

// decompressReader wraps r for algo, returning a closer for the decoders that
// hold resources of their own.
func decompressReader(algo string, r io.Reader) (io.Reader, func(), error) {
	switch algo {
	case "xz":
		reader, err := xz.NewReader(r)
		if err != nil {
			return nil, nil, fmt.Errorf("imagefetch: open xz stream: %w", err)
		}
		return reader, func() {}, nil
	case "gzip":
		reader, err := gzip.NewReader(r)
		if err != nil {
			return nil, nil, fmt.Errorf("imagefetch: open gzip stream: %w", err)
		}
		return reader, func() { errlog.Close(reader) }, nil
	case "zstd":
		decoder, err := zstd.NewReader(r)
		if err != nil {
			return nil, nil, fmt.Errorf("imagefetch: open zstd stream: %w", err)
		}
		return decoder, decoder.Close, nil
	case "bzip2":
		return bzip2.NewReader(r), func() {}, nil
	default:
		return nil, nil, fmt.Errorf("imagefetch: unsupported compression %q", algo)
	}
}

// decompressFile unpacks src into a new temp file in dir and returns its name
// and size; the caller renames it into place once it is happy with it.
//
// The unpacked file is staged rather than written to the final name for the
// same reason the download is: a half-written image under the name the cache
// check trusts would be handed to a guest as a boot disk.
func decompressFile(ctx context.Context, src, dir, algo, imageID string) (string, int64, error) {
	in, err := os.Open(src)
	if err != nil {
		return "", 0, fmt.Errorf("imagefetch: open %s: %w", src, err)
	}
	defer func() { errlog.Close(in) }()

	reader, closeReader, err := decompressReader(algo, in)
	if err != nil {
		return "", 0, err
	}
	defer closeReader()

	out, err := os.CreateTemp(dir, ".unpacked-*")
	if err != nil {
		return "", 0, fmt.Errorf("imagefetch: create temp file: %w", err)
	}
	outName := out.Name()
	done := false
	defer func() {
		errlog.Close(out)
		if !done {
			errlog.Log(os.Remove(outName))
		}
	}()

	written, err := copyWithProgress(ctx, out, reader, nil, imageID, -1, nil)
	if err != nil {
		return "", 0, err
	}
	if err := out.Sync(); err != nil {
		return "", 0, fmt.Errorf("imagefetch: flush %s: %w", outName, err)
	}
	if err := out.Close(); err != nil {
		return "", 0, fmt.Errorf("imagefetch: close %s: %w", outName, err)
	}

	done = true
	return outName, written, nil
}
