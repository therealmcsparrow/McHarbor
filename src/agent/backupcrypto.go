// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

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
	"strings"
)

const (
	backupCryptoAlgorithm = "AES-256-GCM-CHUNKED"
	backupCryptoKeyLength = 32
	backupCryptoNonceSize = 12
	backupCryptoChunkSize = 1 << 20
	backupCryptoMagic     = "MCBKPENC1\n"
)

type backupCryptoService struct {
	masterKey []byte
	keyID     string
}

type backupCryptoMetadata struct {
	Algorithm string
	KeyID     string
}

type backupCryptoFileHeader struct {
	Format     string `json:"format"`
	Algorithm  string `json:"algorithm"`
	KeyID      string `json:"keyId"`
	ChunkSize  int    `json:"chunkSize"`
	WrappedKey string `json:"wrappedKey"`
}

func newBackupCryptoFromKeyMaterial(value string) (*backupCryptoService, error) {
	key, err := parseBackupKey([]byte(value))
	if err != nil {
		return nil, err
	}
	sum := sha256.Sum256(key)
	return &backupCryptoService{masterKey: key, keyID: "sha256:" + hex.EncodeToString(sum[:8])}, nil
}

func (s *backupCryptoService) EncryptWriter(writer io.Writer) (io.WriteCloser, backupCryptoMetadata, error) {
	dek := make([]byte, backupCryptoKeyLength)
	if _, err := rand.Read(dek); err != nil {
		return nil, backupCryptoMetadata{}, fmt.Errorf("generating backup data key: %w", err)
	}
	wrapped, err := s.wrapDataKey(dek)
	if err != nil {
		return nil, backupCryptoMetadata{}, err
	}
	header, err := json.Marshal(backupCryptoFileHeader{
		Format:     "mcharbor.container.backup.encrypted.v1",
		Algorithm:  backupCryptoAlgorithm,
		KeyID:      s.keyID,
		ChunkSize:  backupCryptoChunkSize,
		WrappedKey: base64.StdEncoding.EncodeToString(wrapped),
	})
	if err != nil {
		return nil, backupCryptoMetadata{}, fmt.Errorf("encoding backup encryption header: %w", err)
	}
	if err := backupWriteAll(writer, []byte(backupCryptoMagic)); err != nil {
		return nil, backupCryptoMetadata{}, fmt.Errorf("writing backup encryption magic: %w", err)
	}
	var headerLen [4]byte
	binary.BigEndian.PutUint32(headerLen[:], uint32(len(header)))
	if err := backupWriteAll(writer, headerLen[:]); err != nil {
		return nil, backupCryptoMetadata{}, fmt.Errorf("writing backup encryption header length: %w", err)
	}
	if err := backupWriteAll(writer, header); err != nil {
		return nil, backupCryptoMetadata{}, fmt.Errorf("writing backup encryption header: %w", err)
	}
	block, err := aes.NewCipher(dek)
	if err != nil {
		return nil, backupCryptoMetadata{}, fmt.Errorf("creating backup archive cipher: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, backupCryptoMetadata{}, fmt.Errorf("creating backup archive AEAD: %w", err)
	}
	return &backupChunkWriter{writer: writer, gcm: gcm, buffer: make([]byte, 0, backupCryptoChunkSize)}, backupCryptoMetadata{Algorithm: backupCryptoAlgorithm, KeyID: s.keyID}, nil
}

func (s *backupCryptoService) DecryptReader(reader io.Reader) (io.ReadCloser, error) {
	header, err := readBackupCryptoHeader(reader)
	if err != nil {
		return nil, err
	}
	if header.Algorithm != backupCryptoAlgorithm {
		return nil, fmt.Errorf("unsupported backup encryption algorithm")
	}
	if header.KeyID != s.keyID {
		return nil, fmt.Errorf("backup encryption key does not match archive key id")
	}
	dek, err := s.unwrapDataKey(header)
	if err != nil {
		return nil, err
	}
	block, err := aes.NewCipher(dek)
	if err != nil {
		return nil, fmt.Errorf("creating backup archive cipher: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("creating backup archive AEAD: %w", err)
	}
	return &backupChunkReader{reader: reader, gcm: gcm}, nil
}

func (s *backupCryptoService) wrapDataKey(dek []byte) ([]byte, error) {
	block, err := aes.NewCipher(s.masterKey)
	if err != nil {
		return nil, fmt.Errorf("creating backup key wrap cipher: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("creating backup key wrap AEAD: %w", err)
	}
	nonce := make([]byte, backupCryptoNonceSize)
	if _, err := rand.Read(nonce); err != nil {
		return nil, fmt.Errorf("generating backup key wrap nonce: %w", err)
	}
	return gcm.Seal(nonce, nonce, dek, []byte(s.keyID)), nil
}

func (s *backupCryptoService) unwrapDataKey(header backupCryptoFileHeader) ([]byte, error) {
	wrapped, err := base64.StdEncoding.DecodeString(header.WrappedKey)
	if err != nil {
		return nil, fmt.Errorf("decoding wrapped backup key: %w", err)
	}
	if len(wrapped) <= backupCryptoNonceSize {
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
	dek, err := gcm.Open(nil, wrapped[:backupCryptoNonceSize], wrapped[backupCryptoNonceSize:], []byte(header.KeyID))
	if err != nil {
		return nil, fmt.Errorf("unwrapping backup data key: %w", err)
	}
	return dek, nil
}

type backupChunkWriter struct {
	writer io.Writer
	gcm    cipher.AEAD
	buffer []byte
	closed bool
}

func (w *backupChunkWriter) Write(data []byte) (int, error) {
	if w.closed {
		return 0, fmt.Errorf("backup encryption writer is closed")
	}
	written := 0
	for len(data) > 0 {
		available := backupCryptoChunkSize - len(w.buffer)
		if available > len(data) {
			available = len(data)
		}
		w.buffer = append(w.buffer, data[:available]...)
		data = data[available:]
		written += available
		if len(w.buffer) == backupCryptoChunkSize {
			if err := w.flush(); err != nil {
				return written, err
			}
		}
	}
	return written, nil
}

func (w *backupChunkWriter) Close() error {
	if w.closed {
		return nil
	}
	w.closed = true
	if len(w.buffer) == 0 {
		return nil
	}
	return w.flush()
}

func (w *backupChunkWriter) flush() error {
	nonce := make([]byte, backupCryptoNonceSize)
	if _, err := rand.Read(nonce); err != nil {
		return fmt.Errorf("generating backup chunk nonce: %w", err)
	}
	ciphertext := w.gcm.Seal(nil, nonce, w.buffer, nil)
	var length [4]byte
	binary.BigEndian.PutUint32(length[:], uint32(len(ciphertext)))
	if err := backupWriteAll(w.writer, length[:]); err != nil {
		return fmt.Errorf("writing backup encrypted chunk length: %w", err)
	}
	if err := backupWriteAll(w.writer, nonce); err != nil {
		return fmt.Errorf("writing backup encrypted chunk nonce: %w", err)
	}
	if err := backupWriteAll(w.writer, ciphertext); err != nil {
		return fmt.Errorf("writing backup encrypted chunk: %w", err)
	}
	w.buffer = w.buffer[:0]
	return nil
}

type backupChunkReader struct {
	reader    io.Reader
	gcm       cipher.AEAD
	plaintext []byte
	eof       bool
}

func (r *backupChunkReader) Read(data []byte) (int, error) {
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

func (r *backupChunkReader) Close() error {
	return nil
}

func (r *backupChunkReader) readChunk() error {
	var lengthBytes [4]byte
	if _, err := io.ReadFull(r.reader, lengthBytes[:]); err != nil {
		if errors.Is(err, io.EOF) {
			return io.EOF
		}
		return fmt.Errorf("reading backup encrypted chunk length: %w", err)
	}
	length := binary.BigEndian.Uint32(lengthBytes[:])
	if length == 0 || length > backupCryptoChunkSize+uint32(r.gcm.Overhead()) {
		return fmt.Errorf("backup encrypted chunk size is invalid")
	}
	nonce := make([]byte, backupCryptoNonceSize)
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

func readBackupCryptoHeader(reader io.Reader) (backupCryptoFileHeader, error) {
	var prefix [len(backupCryptoMagic)]byte
	if _, err := io.ReadFull(reader, prefix[:]); err != nil {
		return backupCryptoFileHeader{}, fmt.Errorf("reading backup encryption magic: %w", err)
	}
	if string(prefix[:]) != backupCryptoMagic {
		return backupCryptoFileHeader{}, fmt.Errorf("backup archive is not an encrypted mcharbor backup")
	}
	var headerLenBytes [4]byte
	if _, err := io.ReadFull(reader, headerLenBytes[:]); err != nil {
		return backupCryptoFileHeader{}, fmt.Errorf("reading backup encryption header length: %w", err)
	}
	headerLen := binary.BigEndian.Uint32(headerLenBytes[:])
	if headerLen == 0 || headerLen > 1<<20 {
		return backupCryptoFileHeader{}, fmt.Errorf("backup encryption header size is invalid")
	}
	headerBytes := make([]byte, int(headerLen))
	if _, err := io.ReadFull(reader, headerBytes); err != nil {
		return backupCryptoFileHeader{}, fmt.Errorf("reading backup encryption header: %w", err)
	}
	var header backupCryptoFileHeader
	if err := json.Unmarshal(headerBytes, &header); err != nil {
		return backupCryptoFileHeader{}, fmt.Errorf("decoding backup encryption header: %w", err)
	}
	return header, nil
}

func backupWriteAll(writer io.Writer, data []byte) error {
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

func parseBackupKey(data []byte) ([]byte, error) {
	trimmed := strings.TrimSpace(string(data))
	if decoded, err := base64.StdEncoding.DecodeString(trimmed); err == nil && len(decoded) == backupCryptoKeyLength {
		return decoded, nil
	}
	if decoded, err := hex.DecodeString(trimmed); err == nil && len(decoded) == backupCryptoKeyLength {
		return decoded, nil
	}
	if len(data) == backupCryptoKeyLength {
		return append([]byte(nil), data...), nil
	}
	if len([]byte(trimmed)) == backupCryptoKeyLength {
		return []byte(trimmed), nil
	}
	return nil, fmt.Errorf("backup encryption key must be 32 raw bytes, 64 hex characters, or base64-encoded 32 bytes")
}
