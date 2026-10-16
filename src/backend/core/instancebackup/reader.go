// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package instancebackup

import (
	"bufio"
	"compress/gzip"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"path"
	"strings"
)

// Entry kinds returned by Artifact.Next.
const (
	EntryTable = recTable
	EntryBlob  = recBlob
	EntryEnd   = recEnd
)

// Entry is one table or blob in an opened artifact.
type Entry struct {
	Kind    string
	Name    string
	Columns []string
	Rows    int64
	Size    int64
	Mode    uint32
	Blob    string
}

// Artifact is an export opened with its key. Entries are read in order; a
// caller that does not consume an entry's rows or bytes has them skipped.
type Artifact struct {
	Manifest Manifest
	stream   *streamReader
	gz       *gzip.Reader
	pending  Entry
	rowsLeft int64
	blob     io.Reader
}

// Open authenticates the manifest with s and positions the reader at the
// first entry. A wrong key is reported as ErrWrongKey before anything is decrypted.
func Open(r io.Reader, s Secret) (*Artifact, error) {
	br := bufio.NewReaderSize(r, 256<<10)
	m, rawManifest, mac, err := readHeader(br)
	if err != nil {
		return nil, err
	}
	key, err := openKey(s, m.Key)
	if err != nil {
		return nil, err
	}
	if !hmac.Equal([]byte(manifestMAC(key, rawManifest)), []byte(mac)) {
		return nil, fmt.Errorf("%w: the manifest does not match its signature; the export was altered", ErrDamaged)
	}
	cipher, err := cipherFor(key)
	if err != nil {
		return nil, err
	}
	dec, _, err := cipher.DecryptReader(br)
	if err != nil {
		return nil, fmt.Errorf("opening the encrypted contents: %w", err)
	}
	gz, err := gzip.NewReader(dec)
	if err != nil {
		return nil, fmt.Errorf("%w: the encrypted contents are damaged: %w", ErrDamaged, err)
	}
	a := &Artifact{Manifest: m, stream: newStreamReader(gz), gz: gz}
	begin, err := a.stream.next()
	if err != nil {
		return nil, err
	}
	sum := sha256.Sum256(rawManifest)
	if begin.T != recBegin || begin.Manifest != hex.EncodeToString(sum[:]) {
		return nil, fmt.Errorf("%w: the contents belong to a different manifest; the export was altered", ErrDamaged)
	}
	return a, nil
}

// Next skips whatever is left of the current entry and returns the next one.
// The end entry is returned once the stream's checksum has been verified.
func (a *Artifact) Next() (Entry, error) {
	for a.rowsLeft > 0 {
		if _, err := a.Row(); err != nil {
			return Entry{}, err
		}
	}
	if a.blob != nil {
		if _, err := io.Copy(io.Discard, a.blob); err != nil {
			return Entry{}, fmt.Errorf("reading %s: %w", a.pending.Name, err)
		}
		a.blob = nil
	}
	rec, err := a.stream.next()
	if err != nil {
		return Entry{}, err
	}
	e := Entry{Kind: rec.T, Name: rec.Name, Columns: rec.Columns, Rows: rec.Rows, Size: rec.Size, Mode: rec.Mode, Blob: rec.Kind}
	switch rec.T {
	case recTable:
		a.rowsLeft = rec.Rows
	case recBlob:
		if err := checkBlobPath(rec.Name); err != nil {
			return Entry{}, err
		}
		a.blob = &exactReader{r: a.stream.blob(rec.Size), left: rec.Size, name: rec.Name}
	case recEnd:
	default:
		return Entry{}, fmt.Errorf("%w: the export holds an unknown entry %q", ErrDamaged, rec.T)
	}
	a.pending = e
	return e, nil
}

// Row returns the next decoded row of the current table.
func (a *Artifact) Row() ([]any, error) {
	if a.rowsLeft <= 0 {
		return nil, io.EOF
	}
	raw, err := a.stream.row()
	if err != nil {
		return nil, err
	}
	a.rowsLeft--
	if len(raw) != len(a.pending.Columns) {
		return nil, fmt.Errorf("%w: a row of %s has %d values for %d columns", ErrDamaged, a.pending.Name, len(raw), len(a.pending.Columns))
	}
	for i, v := range raw {
		d, err := decodeValue(v)
		if err != nil {
			return nil, fmt.Errorf("%s.%s: %w", a.pending.Name, a.pending.Columns[i], err)
		}
		raw[i] = d
	}
	return raw, nil
}

// Blob returns the current blob's bytes.
func (a *Artifact) Blob() io.Reader {
	if a.blob == nil {
		return strings.NewReader("")
	}
	return a.blob
}

// Close releases the decompressor.
func (a *Artifact) Close() error {
	return a.gz.Close()
}

// exactReader fails if the stream ends before the declared size.
type exactReader struct {
	r    io.Reader
	left int64
	name string
}

func (e *exactReader) Read(p []byte) (int, error) {
	if e.left <= 0 {
		return 0, io.EOF
	}
	n, err := e.r.Read(p)
	e.left -= int64(n)
	if err == io.EOF && e.left > 0 {
		return n, fmt.Errorf("%w: %s ends early; the export is incomplete", ErrIncomplete, e.name)
	}
	return n, err
}

// checkBlobPath keeps a restored file inside the data directory entries an
// export may carry, whatever the artifact claims.
func checkBlobPath(p string) error {
	if p == "" || strings.HasPrefix(p, "/") || strings.Contains(p, "\\") || path.Clean(p) != p || strings.HasPrefix(p, "../") || p == ".." {
		return fmt.Errorf("%w: the export names an unsafe path %q", ErrDamaged, p)
	}
	switch p {
	case KeyEncryption, KeySecret, TLSCACert, TLSCAKey, TLSCert, TLSKey:
		return nil
	}
	for _, root := range FileRoots {
		if p == root || strings.HasPrefix(p, root+"/") {
			return nil
		}
	}
	return fmt.Errorf("%w: the export names a path outside what an export may carry: %q", ErrDamaged, p)
}

// Verify reads an entire artifact, which is the only way to prove it is
// complete, and checks every table and blob against the manifest.
func Verify(ctx context.Context, r io.Reader, s Secret) (_ Manifest, retErr error) {
	a, err := Open(r, s)
	if err != nil {
		return Manifest{}, err
	}
	defer func() {
		if cerr := a.Close(); cerr != nil && retErr == nil {
			retErr = fmt.Errorf("closing the export: %w", cerr)
		}
	}()
	want := make(map[string]int64, len(a.Manifest.Tables))
	for _, t := range a.Manifest.Tables {
		want[t.Name] = t.Rows
	}
	blobs := make(map[string]int64, len(a.Manifest.Blobs))
	for _, b := range a.Manifest.Blobs {
		blobs[b.Path] = b.Size
	}
	seenTables, seenBlobs := 0, 0
	for {
		if err := ctx.Err(); err != nil {
			return Manifest{}, err
		}
		e, err := a.Next()
		if err != nil {
			return Manifest{}, err
		}
		switch e.Kind {
		case EntryEnd:
			if seenTables != len(want) || seenBlobs != len(blobs) {
				return Manifest{}, fmt.Errorf("%w: the export holds fewer entries than its manifest lists", ErrIncomplete)
			}
			return a.Manifest, nil
		case EntryTable:
			n, ok := want[e.Name]
			if !ok || n != e.Rows {
				return Manifest{}, fmt.Errorf("%w: table %s does not match the manifest", ErrDamaged, e.Name)
			}
			seenTables++
		case EntryBlob:
			n, ok := blobs[e.Name]
			if !ok || n != e.Size {
				return Manifest{}, fmt.Errorf("%w: %s does not match the manifest", ErrDamaged, e.Name)
			}
			seenBlobs++
		}
	}
}
