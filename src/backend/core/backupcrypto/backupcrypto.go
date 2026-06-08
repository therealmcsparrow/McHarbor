// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package backupcrypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
)

const (
	Algorithm = "AES-256-GCM-CHUNKED"

	keyLength = 32
	nonceSize = 12
	chunkSize = 1 << 20
	magic     = "MCBKPENC1\n"
)

// Metadata describes the encryption envelope stored with a backup archive.
type Metadata struct {
	Algorithm string `json:"algorithm"`
	KeyID     string `json:"keyId"`
}

type fileHeader struct {
	Format     string `json:"format"`
	Algorithm  string `json:"algorithm"`
	KeyID      string `json:"keyId"`
	ChunkSize  int    `json:"chunkSize"`
	WrappedKey string `json:"wrappedKey"`
}

// Service encrypts backup archive streams with envelope encryption.
type Service struct {
	masterKey []byte
	keyID     string
}

// NewFromKeyFile loads a Docker-secret-backed backup master key.
func NewFromKeyFile(path string) (*Service, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading backup encryption key file: %w", err)
	}
	return NewFromKeyMaterial(string(data))
}

// NewFromKeyMaterial loads a backup master key from a one-time user supplied value.
func NewFromKeyMaterial(value string) (*Service, error) {
	key, err := parseKey([]byte(value))
	if err != nil {
		return nil, err
	}
	return newServiceFromKey(key), nil
}

func newServiceFromKey(key []byte) *Service {
	sum := sha256.Sum256(key)
	return &Service{
		masterKey: key,
		keyID:     "sha256:" + hex.EncodeToString(sum[:8]),
	}
}

// DecryptReader reads and decrypts an encrypted backup archive envelope.
func (s *Service) DecryptReader(reader io.Reader) (io.ReadCloser, Metadata, error) {
	header, err := readHeader(reader)
	if err != nil {
		return nil, Metadata{}, err
	}
	if header.Algorithm != Algorithm {
		return nil, Metadata{}, fmt.Errorf("unsupported backup encryption algorithm")
	}
	if header.KeyID != s.keyID {
		return nil, Metadata{}, fmt.Errorf("backup encryption key does not match archive key id")
	}

	dek, err := s.unwrapDataKey(header)
	if err != nil {
		return nil, Metadata{}, err
	}
	block, err := aes.NewCipher(dek)
	if err != nil {
		return nil, Metadata{}, fmt.Errorf("creating backup archive cipher: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, Metadata{}, fmt.Errorf("creating backup archive AEAD: %w", err)
	}

	return &chunkReader{reader: reader, gcm: gcm}, Metadata{Algorithm: header.Algorithm, KeyID: header.KeyID}, nil
}

func readHeader(reader io.Reader) (fileHeader, error) {
	var prefix [len(magic)]byte
	if _, err := io.ReadFull(reader, prefix[:]); err != nil {
		return fileHeader{}, fmt.Errorf("reading backup encryption magic: %w", err)
	}
	if string(prefix[:]) != magic {
		return fileHeader{}, fmt.Errorf("backup archive is not an encrypted mcharbor backup")
	}

	var headerLenBytes [4]byte
	if _, err := io.ReadFull(reader, headerLenBytes[:]); err != nil {
		return fileHeader{}, fmt.Errorf("reading backup encryption header length: %w", err)
	}
	headerLen := binary.BigEndian.Uint32(headerLenBytes[:])
	if headerLen == 0 || headerLen > 1<<20 {
		return fileHeader{}, fmt.Errorf("backup encryption header size is invalid")
	}

	headerBytes := make([]byte, int(headerLen))
	if _, err := io.ReadFull(reader, headerBytes); err != nil {
		return fileHeader{}, fmt.Errorf("reading backup encryption header: %w", err)
	}

	var header fileHeader
	if err := json.Unmarshal(headerBytes, &header); err != nil {
		return fileHeader{}, fmt.Errorf("decoding backup encryption header: %w", err)
	}
	if header.Format != "mcharbor.container.backup.encrypted.v1" {
		return fileHeader{}, fmt.Errorf("unsupported backup encryption format")
	}
	return header, nil
}

func (s *Service) unwrapDataKey(header fileHeader) ([]byte, error) {
	wrapped, err := base64.StdEncoding.DecodeString(header.WrappedKey)
	if err != nil {
		return nil, fmt.Errorf("decoding wrapped backup key: %w", err)
	}
	if len(wrapped) <= nonceSize {
		return nil, fmt.Errorf("wrapped backup key is invalid")
	}

	block, err := aes.NewCipher(s.masterKey)
	if err != nil {
		return nil, fmt.Errorf("creating backup key unwrap cipher: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("creating backup key unwrap AEAD: %w", err)
	}
	nonce := wrapped[:nonceSize]
	ciphertext := wrapped[nonceSize:]
	dek, err := gcm.Open(nil, nonce, ciphertext, []byte(header.KeyID))
	if err != nil {
		return nil, fmt.Errorf("unwrapping backup data key: %w", err)
	}
	if len(dek) != keyLength {
		return nil, fmt.Errorf("backup data key length is invalid")
	}
	return dek, nil
}

type chunkReader struct {
	reader    io.Reader
	gcm       cipher.AEAD
	plaintext []byte
	eof       bool
}

func (r *chunkReader) Read(data []byte) (int, error) {
	for len(r.plaintext) == 0 && !r.eof {
		if err := r.readChunk(); err != nil {
			if errors.Is(err, io.EOF) {
				r.eof = true
				break
			}
			return 0, err
		}
	}
	if len(r.plaintext) == 0 && r.eof {
		return 0, io.EOF
	}
	n := copy(data, r.plaintext)
	r.plaintext = r.plaintext[n:]
	return n, nil
}

func (r *chunkReader) Close() error {
	return nil
}

func (r *chunkReader) readChunk() error {
	var lengthBytes [4]byte
	if _, err := io.ReadFull(r.reader, lengthBytes[:]); err != nil {
		if errors.Is(err, io.EOF) {
			return io.EOF
		}
		return fmt.Errorf("reading backup encrypted chunk length: %w", err)
	}
	length := binary.BigEndian.Uint32(lengthBytes[:])
	if length == 0 || length > chunkSize+uint32(r.gcm.Overhead()) {
		return fmt.Errorf("backup encrypted chunk size is invalid")
	}

	nonce := make([]byte, nonceSize)
	if _, err := io.ReadFull(r.reader, nonce); err != nil {
		return fmt.Errorf("reading backup encrypted chunk nonce: %w", err)
	}
	ciphertext := make([]byte, int(length))
	if _, err := io.ReadFull(r.reader, ciphertext); err != nil {
		return fmt.Errorf("reading backup encrypted chunk: %w", err)
	}
	plaintext, err := r.gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return fmt.Errorf("decrypting backup encrypted chunk: %w", err)
	}
	r.plaintext = plaintext
	return nil
}

// KeyID returns the non-secret identifier for the active master key.
func (s *Service) KeyID() string {
	return s.keyID
}

// EncryptWriter writes an encrypted backup archive envelope to writer.
func (s *Service) EncryptWriter(writer io.Writer) (io.WriteCloser, Metadata, error) {
	dek := make([]byte, keyLength)
	if _, err := rand.Read(dek); err != nil {
		return nil, Metadata{}, fmt.Errorf("generating backup data key: %w", err)
	}

	wrapped, err := s.wrapDataKey(dek)
	if err != nil {
		return nil, Metadata{}, err
	}

	header, err := json.Marshal(fileHeader{
		Format:     "mcharbor.container.backup.encrypted.v1",
		Algorithm:  Algorithm,
		KeyID:      s.keyID,
		ChunkSize:  chunkSize,
		WrappedKey: base64.StdEncoding.EncodeToString(wrapped),
	})
	if err != nil {
		return nil, Metadata{}, fmt.Errorf("encoding backup encryption header: %w", err)
	}

	if err := writeAll(writer, []byte(magic)); err != nil {
		return nil, Metadata{}, fmt.Errorf("writing backup encryption magic: %w", err)
	}
	if len(header) > int(^uint32(0)) {
		return nil, Metadata{}, fmt.Errorf("backup encryption header too large")
	}
	var headerLen [4]byte
	binary.BigEndian.PutUint32(headerLen[:], uint32(len(header)))
	if err := writeAll(writer, headerLen[:]); err != nil {
		return nil, Metadata{}, fmt.Errorf("writing backup encryption header length: %w", err)
	}
	if err := writeAll(writer, header); err != nil {
		return nil, Metadata{}, fmt.Errorf("writing backup encryption header: %w", err)
	}

	block, err := aes.NewCipher(dek)
	if err != nil {
		return nil, Metadata{}, fmt.Errorf("creating backup archive cipher: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, Metadata{}, fmt.Errorf("creating backup archive AEAD: %w", err)
	}

	return &chunkWriter{
		writer: writer,
		gcm:    gcm,
		buffer: make([]byte, 0, chunkSize),
	}, Metadata{Algorithm: Algorithm, KeyID: s.keyID}, nil
}

func (s *Service) wrapDataKey(dek []byte) ([]byte, error) {
	block, err := aes.NewCipher(s.masterKey)
	if err != nil {
		return nil, fmt.Errorf("creating backup key wrap cipher: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("creating backup key wrap AEAD: %w", err)
	}
	nonce := make([]byte, nonceSize)
	if _, err := rand.Read(nonce); err != nil {
		return nil, fmt.Errorf("generating backup key wrap nonce: %w", err)
	}
	return gcm.Seal(nonce, nonce, dek, []byte(s.keyID)), nil
}

type chunkWriter struct {
	writer io.Writer
	gcm    cipher.AEAD
	buffer []byte
	closed bool
}

func (w *chunkWriter) Write(data []byte) (int, error) {
	if w.closed {
		return 0, fmt.Errorf("backup encryption writer is closed")
	}

	written := 0
	for len(data) > 0 {
		available := chunkSize - len(w.buffer)
		if available > len(data) {
			available = len(data)
		}
		w.buffer = append(w.buffer, data[:available]...)
		data = data[available:]
		written += available

		if len(w.buffer) == chunkSize {
			if err := w.flush(); err != nil {
				return written, err
			}
		}
	}
	return written, nil
}

func (w *chunkWriter) Close() error {
	if w.closed {
		return nil
	}
	w.closed = true
	if len(w.buffer) == 0 {
		return nil
	}
	return w.flush()
}

func (w *chunkWriter) flush() error {
	nonce := make([]byte, nonceSize)
	if _, err := rand.Read(nonce); err != nil {
		return fmt.Errorf("generating backup chunk nonce: %w", err)
	}
	ciphertext := w.gcm.Seal(nil, nonce, w.buffer, nil)
	if len(ciphertext) > int(^uint32(0)) {
		return fmt.Errorf("backup encrypted chunk too large")
	}
	var length [4]byte
	binary.BigEndian.PutUint32(length[:], uint32(len(ciphertext)))
	if err := writeAll(w.writer, length[:]); err != nil {
		return fmt.Errorf("writing backup encrypted chunk length: %w", err)
	}
	if err := writeAll(w.writer, nonce); err != nil {
		return fmt.Errorf("writing backup encrypted chunk nonce: %w", err)
	}
	if err := writeAll(w.writer, ciphertext); err != nil {
		return fmt.Errorf("writing backup encrypted chunk: %w", err)
	}
	w.buffer = w.buffer[:0]
	return nil
}

func writeAll(writer io.Writer, data []byte) error {
	for len(data) > 0 {
		n, err := writer.Write(data)
		if err != nil {
			return err
		}
		if n == 0 {
			return io.ErrUnexpectedEOF
		}
		data = data[n:]
	}
	return nil
}

func parseKey(data []byte) ([]byte, error) {
	trimmed := strings.TrimSpace(string(data))
	if decoded, err := base64.StdEncoding.DecodeString(trimmed); err == nil && len(decoded) == keyLength {
		return decoded, nil
	}
	if decoded, err := hex.DecodeString(trimmed); err == nil && len(decoded) == keyLength {
		return decoded, nil
	}
	if len(data) == keyLength {
		return append([]byte(nil), data...), nil
	}
	if len([]byte(trimmed)) == keyLength {
		return []byte(trimmed), nil
	}
	return nil, fmt.Errorf("backup encryption key must be 32 raw bytes, 64 hex characters, or base64-encoded 32 bytes")
}
