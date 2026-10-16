// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package agentreleases

import (
	"bytes"
	"crypto/ed25519"
	"encoding/base64"
	"fmt"
	"regexp"
	"strings"
)

// Files every release carries beside its binaries.
const (
	sumsFile  = "SHA256SUMS"
	sigFile   = "SHA256SUMS.sig"
	buildFile = "BUILD"
	imageFile = "image.txt"
)

var (
	versionPattern = regexp.MustCompile(`^[0-9]{1,4}\.[0-9]{1,3}\.[0-9]{1,4}$`)
	buildPattern   = regexp.MustCompile(`^[0-9]{12}$`)
	sumLinePattern = regexp.MustCompile(`^([0-9a-fA-F]{64}) [ *]([A-Za-z0-9._-]{1,128})$`)
	binaryPattern  = regexp.MustCompile(`^mcharbor-agent-([a-z0-9_]{1,16})-([a-z0-9_]{1,16})(\.exe)?$`)
	// imageRefPattern accepts only a digest-pinned reference: a signed image.txt
	// that could name a moving tag would sign nothing at all.
	imageRefPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9._/:-]{0,254}@sha256:[0-9a-f]{64}$`)
	platformPattern = regexp.MustCompile(`^linux/[a-z0-9_]{1,16}$`)
)

// ValidVersion reports whether v is a calendar version McHarbor will store.
// It bounds what reaches a URL path and a directory name, so it is strict.
func ValidVersion(v string) bool {
	return versionPattern.MatchString(v)
}

// ParseSums reads a sha256sum-format file into name → lower-case hex digest.
// Any malformed line refuses the whole file: a half-parsed list would let a
// tampered entry slip through as "not listed" instead of failing.
func ParseSums(data []byte) (map[string]string, error) {
	out := map[string]string{}
	for _, raw := range strings.Split(strings.ReplaceAll(string(data), "\r\n", "\n"), "\n") {
		line := strings.TrimSpace(raw)
		if line == "" {
			continue
		}
		m := sumLinePattern.FindStringSubmatch(line)
		if m == nil {
			return nil, fmt.Errorf("%w: unreadable line %q", ErrSumsMalformed, line)
		}
		name := m[2]
		if _, dup := out[name]; dup {
			return nil, fmt.Errorf("%w: %s is listed twice", ErrSumsMalformed, name)
		}
		out[name] = strings.ToLower(m[1])
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("%w: no entries", ErrSumsMalformed)
	}
	return out, nil
}

// VerifySums checks sig over sums against every pinned key and returns the
// fingerprint of the key that verified. The signature is base64 of a raw
// ed25519 signature, exactly what pluginsigner -file writes; raw bytes are
// accepted too so a hand-signed mirror is not refused over encoding.
func VerifySums(sums, sig []byte, keys []ed25519.PublicKey) (string, error) {
	trimmed := bytes.TrimSpace(sig)
	if len(trimmed) == 0 {
		return "", ErrSignatureMissing
	}
	raw := trimmed
	if decoded, err := base64.StdEncoding.DecodeString(string(trimmed)); err == nil {
		raw = decoded
	}
	if len(raw) != ed25519.SignatureSize {
		return "", fmt.Errorf("%w: signature is %d bytes", ErrSignatureInvalid, len(raw))
	}
	for _, pub := range keys {
		if ed25519.Verify(pub, sums, raw) {
			return KeyFingerprint(pub), nil
		}
	}
	return "", ErrSignatureInvalid
}

// binaryPlatform returns the os and arch a release asset name targets, or
// ok=false for anything that is not an agent binary.
func binaryPlatform(name string) (osName, arch string, ok bool) {
	m := binaryPattern.FindStringSubmatch(name)
	if m == nil {
		return "", "", false
	}
	if m[3] != "" && m[1] != "windows" {
		return "", "", false
	}
	return m[1], m[2], true
}

// BinaryAssetName is the file name an agent binary for a platform is published under.
func BinaryAssetName(osName, arch string) string {
	name := "mcharbor-agent-" + osName + "-" + arch
	if osName == "windows" {
		name += ".exe"
	}
	return name
}

// ImageAssetName is the tarball an agent image for a platform is stored under.
func ImageAssetName(osName, arch string) string {
	return "image-" + osName + "-" + arch + ".tar"
}
