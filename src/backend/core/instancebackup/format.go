// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package instancebackup

import (
	"bufio"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"hash"
	"io"
	"strings"
	"time"
)

// Magic is the first line of every artifact; the version after the slash is
// the container format, independent of the McHarbor version.
const Magic = "MCHARBOR-INSTANCE-EXPORT/1"

// FileExtension is the suffix exports are written and downloaded with.
const FileExtension = ".mchx"

// maxHeaderLine bounds the plaintext lines read before decryption, so a
// hostile file cannot make the reader buffer without limit.
const maxHeaderLine = 8 << 20

// TableInventory is one table's row count at export time.
type TableInventory struct {
	Name string `json:"name"`
	Rows int64  `json:"rows"`
}

// PluginRecord is one installed plugin. The binary is never exported: it is
// bound to the build that produced it, so a restore reinstalls it from the catalog.
type PluginRecord struct {
	Slug    string `json:"slug"`
	Version string `json:"version"`
	Enabled bool   `json:"enabled"`
	Source  string `json:"source"`
}

// Manifest is the plaintext description of an export, readable without the
// key so an operator holding several can tell them apart.
type Manifest struct {
	Format          int              `json:"format"`
	Version         string           `json:"version"`
	Build           string           `json:"build"`
	Backend         string           `json:"backend"`
	CreatedAt       time.Time        `json:"createdAt"`
	CreatedBy       string           `json:"createdBy"`
	InstanceID      string           `json:"instanceId"`
	Hostname        string           `json:"hostname"`
	Schema          []string         `json:"schema"`
	Tables          []TableInventory `json:"tables"`
	IncludesMetrics bool             `json:"includesMetrics"`
	Plugins         []PluginRecord   `json:"plugins"`
	Blobs           []BlobInventory  `json:"blobs"`
	Key             KeyInfo          `json:"key"`
	Note            string           `json:"note,omitempty"`
}

// BlobInventory is one file or key carried in the export, named by its
// logical path, never by the host path it came from.
type BlobInventory struct {
	Path string `json:"path"`
	Size int64  `json:"size"`
	Kind string `json:"kind"`
}

// TotalRows is the sum of the inventory's row counts.
func (m Manifest) TotalRows() int64 {
	var n int64
	for _, t := range m.Tables {
		n += t.Rows
	}
	return n
}

// record kinds in the decrypted stream.
const (
	recBegin = "begin"
	recTable = "table"
	recBlob  = "blob"
	recEnd   = "end"
)

type record struct {
	T        string   `json:"t"`
	Name     string   `json:"name,omitempty"`
	Columns  []string `json:"columns,omitempty"`
	Rows     int64    `json:"rows,omitempty"`
	Size     int64    `json:"size,omitempty"`
	Mode     uint32   `json:"mode,omitempty"`
	Kind     string   `json:"kind,omitempty"`
	Manifest string   `json:"manifest,omitempty"`
	SHA256   string   `json:"sha256,omitempty"`
	Records  int64    `json:"records,omitempty"`
}

// writeHeader writes the plaintext part: magic, manifest and its MAC.
func writeHeader(w io.Writer, manifest []byte, mac string) error {
	if _, err := fmt.Fprintf(w, "%s\n%s\n{\"mac\":%q}\n", Magic, manifest, mac); err != nil {
		return fmt.Errorf("writing the export header: %w", err)
	}
	return nil
}

// ReadManifest reads the plaintext manifest without any key. The result is
// unauthenticated until Open verifies its MAC.
func ReadManifest(r io.Reader) (Manifest, error) {
	m, _, _, err := readHeader(bufio.NewReaderSize(r, 64<<10))
	return m, err
}

func readHeader(br *bufio.Reader) (Manifest, []byte, string, error) {
	var m Manifest
	magic, err := readLine(br)
	if err != nil {
		return m, nil, "", fmt.Errorf("%w: not a McHarbor instance export: %w", ErrNotExport, err)
	}
	if string(magic) != Magic {
		if strings.HasPrefix(string(magic), "MCHARBOR-INSTANCE-EXPORT/") {
			return m, nil, "", fmt.Errorf("%w: this export uses format %s, which this McHarbor cannot read", ErrUnsupportedFormat, strings.TrimPrefix(string(magic), "MCHARBOR-INSTANCE-EXPORT/"))
		}
		return m, nil, "", fmt.Errorf("%w: not a McHarbor instance export", ErrNotExport)
	}
	raw, err := readLine(br)
	if err != nil {
		return m, nil, "", fmt.Errorf("%w: reading the manifest: %w", ErrDamaged, err)
	}
	if err := json.Unmarshal(raw, &m); err != nil {
		return m, nil, "", fmt.Errorf("%w: the manifest is damaged: %w", ErrDamaged, err)
	}
	macLine, err := readLine(br)
	if err != nil {
		return m, nil, "", fmt.Errorf("%w: reading the manifest signature: %w", ErrDamaged, err)
	}
	var mac struct {
		MAC string `json:"mac"`
	}
	if err := json.Unmarshal(macLine, &mac); err != nil || mac.MAC == "" {
		return m, nil, "", fmt.Errorf("%w: the manifest signature is damaged", ErrDamaged)
	}
	return m, raw, mac.MAC, nil
}

func readLine(br *bufio.Reader) ([]byte, error) {
	var out []byte
	for {
		chunk, isPrefix, err := br.ReadLine()
		if err != nil {
			return nil, err
		}
		out = append(out, chunk...)
		if len(out) > maxHeaderLine {
			return nil, fmt.Errorf("%w: line longer than %d bytes", ErrDamaged, maxHeaderLine)
		}
		if !isPrefix {
			return out, nil
		}
	}
}

// streamWriter writes records and hashes everything it writes, so the end
// record can witness the whole stream.
type streamWriter struct {
	w       *bufio.Writer
	h       hash.Hash
	records int64
}

func newStreamWriter(w io.Writer) *streamWriter {
	return &streamWriter{w: bufio.NewWriterSize(w, 256<<10), h: sha256.New()}
}

func (s *streamWriter) write(p []byte) error {
	s.h.Write(p)
	_, err := s.w.Write(p)
	return err
}

func (s *streamWriter) header(r record) error {
	raw, err := json.Marshal(r)
	if err != nil {
		return err
	}
	s.records++
	return s.write(append(raw, '\n'))
}

func (s *streamWriter) row(values []any) error {
	raw, err := json.Marshal(values)
	if err != nil {
		return err
	}
	return s.write(append(raw, '\n'))
}

func (s *streamWriter) end() error {
	raw, err := json.Marshal(record{T: recEnd, SHA256: hex.EncodeToString(s.h.Sum(nil)), Records: s.records})
	if err != nil {
		return err
	}
	if _, err := s.w.Write(append(raw, '\n')); err != nil {
		return err
	}
	return s.w.Flush()
}

// streamReader is the inverse of streamWriter.
type streamReader struct {
	r       *bufio.Reader
	h       hash.Hash
	records int64
	ended   bool
}

func newStreamReader(r io.Reader) *streamReader {
	return &streamReader{r: bufio.NewReaderSize(r, 256<<10), h: sha256.New()}
}

// next returns the next record header. The end record is checked here: a
// stream whose digest or record count does not match was cut or altered.
func (s *streamReader) next() (record, error) {
	line, err := readLine(s.r)
	if err != nil {
		if errors.Is(err, io.EOF) {
			return record{}, fmt.Errorf("%w: the export ends early; it is incomplete", ErrIncomplete)
		}
		return record{}, err
	}
	var rec record
	if err := json.Unmarshal(line, &rec); err != nil {
		return record{}, fmt.Errorf("%w: the export is damaged: %w", ErrDamaged, err)
	}
	if rec.T == recEnd {
		if rec.SHA256 != hex.EncodeToString(s.h.Sum(nil)) || rec.Records != s.records {
			return record{}, fmt.Errorf("%w: the export's contents do not match its checksum; it was altered or damaged", ErrDamaged)
		}
		s.ended = true
		return rec, nil
	}
	s.h.Write(line)
	s.h.Write([]byte{'\n'})
	s.records++
	return rec, nil
}

func (s *streamReader) row() ([]any, error) {
	line, err := readLine(s.r)
	if err != nil {
		if errors.Is(err, io.EOF) {
			return nil, fmt.Errorf("%w: the export ends early; it is incomplete", ErrIncomplete)
		}
		return nil, err
	}
	s.h.Write(line)
	s.h.Write([]byte{'\n'})
	dec := json.NewDecoder(strings.NewReader(string(line)))
	dec.UseNumber()
	var values []any
	if err := dec.Decode(&values); err != nil {
		return nil, fmt.Errorf("%w: a row is damaged: %w", ErrDamaged, err)
	}
	return values, nil
}

// blob returns a reader over the next size bytes, hashed as they are read.
func (s *streamReader) blob(size int64) io.Reader {
	return io.TeeReader(io.LimitReader(s.r, size), s.h)
}

// encodeValue turns a scanned database value into a JSON value that decodes
// back to something every driver binds. Binary and time values are tagged
// because JSON has no type for either.
func encodeValue(v any, binary bool) any {
	switch x := v.(type) {
	case nil:
		return nil
	case []byte:
		if binary {
			return map[string]string{"b": base64.StdEncoding.EncodeToString(x)}
		}
		return string(x)
	case time.Time:
		return map[string]string{"t": x.UTC().Format(time.RFC3339Nano)}
	default:
		return x
	}
}

// decodeValue is the inverse of encodeValue.
func decodeValue(v any) (any, error) {
	switch x := v.(type) {
	case json.Number:
		if i, err := x.Int64(); err == nil {
			return i, nil
		}
		f, err := x.Float64()
		if err != nil {
			return nil, fmt.Errorf("%w: a number is damaged: %w", ErrDamaged, err)
		}
		return f, nil
	case map[string]any:
		if s, ok := x["b"].(string); ok {
			return base64.StdEncoding.DecodeString(s)
		}
		if s, ok := x["t"].(string); ok {
			return time.Parse(time.RFC3339Nano, s)
		}
		return nil, fmt.Errorf("%w: a value has an unknown tag", ErrDamaged)
	default:
		return x, nil
	}
}
