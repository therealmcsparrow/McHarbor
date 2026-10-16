// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package imagefetch

import (
	"bytes"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ProtonMail/go-crypto/openpgp"
)

// TestBundledKeysMatchTheirPins: every embedded key parses and is exactly the key
// its pin names, so a swapped file cannot ride along unnoticed.
func TestBundledKeysMatchTheirPins(t *testing.T) {
	entries, err := bundledKeyFiles.ReadDir("keys")
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != len(PinnedFingerprints) {
		t.Fatalf("%d key files bundled, %d pinned", len(entries), len(PinnedFingerprints))
	}
	for _, entry := range entries {
		data, err := bundledKeyFiles.ReadFile("keys/" + entry.Name())
		if err != nil {
			t.Fatal(err)
		}
		ring, err := openpgp.ReadArmoredKeyRing(bytes.NewReader(data))
		if err != nil {
			t.Fatalf("%s: %v", entry.Name(), err)
		}
		if len(ring) != 1 {
			t.Fatalf("%s carries %d keys, want exactly one", entry.Name(), len(ring))
		}
		got := strings.ToUpper(hex.EncodeToString(ring[0].PrimaryKey.Fingerprint))
		if got != PinnedFingerprints[entry.Name()] {
			t.Errorf("%s fingerprint %s, pinned %s", entry.Name(), got, PinnedFingerprints[entry.Name()])
		}
	}
	if _, err := BundledKeys(); err != nil {
		t.Fatal(err)
	}
}

func bundledRing(t *testing.T) Keyring {
	t.Helper()
	keys, err := BundledKeys()
	if err != nil {
		t.Fatal(err)
	}
	ring, err := NewKeyring(keys...)
	if err != nil {
		t.Fatal(err)
	}
	return ring
}

func fixture(t *testing.T, name string) []byte {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatal(err)
	}
	return data
}

// TestRealDistributionSignaturesVerify uses files the distributions actually
// published, so a key that parses but cannot verify (an expired subkey, a hash
// go-crypto refuses) fails here rather than on an operator's first download.
func TestRealDistributionSignaturesVerify(t *testing.T) {
	ring := bundledRing(t)

	sums := fixture(t, "ubuntu-noble-SHA256SUMS")
	if err := ring.Verify(sums, fixture(t, "ubuntu-noble-SHA256SUMS.gpg")); err != nil {
		t.Fatalf("Ubuntu cloud SHA256SUMS: %v", err)
	}
	if err := ring.VerifyReader(bytes.NewReader(sums), fixture(t, "ubuntu-noble-SHA256SUMS.gpg")); err != nil {
		t.Fatalf("Ubuntu cloud SHA256SUMS as a stream: %v", err)
	}
	tampered := append(append([]byte{}, sums...), []byte("0000 evil.img\n")...)
	if err := ring.Verify(tampered, fixture(t, "ubuntu-noble-SHA256SUMS.gpg")); err == nil {
		t.Fatal("a tampered SHA256SUMS verified")
	}

	for _, name := range []string{"fedora-41-CHECKSUM", "fedora-44-CHECKSUM"} {
		text, err := ring.VerifyClearsigned(fixture(t, name))
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if bytes.Contains(text, []byte("BEGIN PGP")) || !bytes.Contains(text, []byte("SHA256 (")) {
			t.Fatalf("%s: returned text is not just the signed checksums", name)
		}
	}
}

// TestInlineSignatureDropsTextOutsideTheBlock: a line appended after a genuinely
// signed block must never reach the checksum parser.
func TestInlineSignatureDropsTextOutsideTheBlock(t *testing.T) {
	doc := append(fixture(t, "fedora-44-CHECKSUM"), []byte("\nSHA256 (evil.qcow2) = deadbeef\n")...)
	text, err := bundledRing(t).VerifyClearsigned(doc)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(text, []byte("evil.qcow2")) {
		t.Fatal("text after the signed block was returned")
	}
	if _, err := bundledRing(t).VerifyClearsigned([]byte("SHA256 (x) = 00\n")); err == nil {
		t.Fatal("an unsigned checksum file passed as inline-signed")
	}
}

func TestSignatureModeFollowsWhereTheSignaturePoints(t *testing.T) {
	src := "https://example.org/img.qcow2"
	sums := "https://example.org/SHA256SUMS"
	for _, tc := range []struct {
		sig, want string
	}{
		{"", sigNone},
		{sums, sigInline},
		{src + ".asc", sigImage},
		{src + ".sig", sigImage},
		{sums + ".gpg", sigChecksum},
	} {
		if got := signatureModeOf(Request{SourceURL: src, ChecksumURL: sums, SignatureURL: tc.sig}); got != tc.want {
			t.Errorf("signature %q: mode %q, want %q", tc.sig, got, tc.want)
		}
	}
}
