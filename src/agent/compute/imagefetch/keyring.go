// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package imagefetch

import (
	"bytes"
	"errors"
	"fmt"
	"io"

	"github.com/ProtonMail/go-crypto/openpgp"
	"github.com/ProtonMail/go-crypto/openpgp/clearsign"
)

// OpenPGP verification of the detached signatures distributions publish over
// their checksum files.
//
// The keys are shipped with the agent binary, never fetched. A key downloaded
// alongside the thing it authenticates proves only that both came from the same
// place, which is precisely what an attacker who can rewrite the mirror
// controls. Pinning is the whole value.

// ErrNoKeys means the agent shipped without any signing keys, so signature
// verification cannot be attempted.
var ErrNoKeys = errors.New("imagefetch: no distribution signing keys are bundled")

// pgpKeyring verifies against a fixed set of armored public keys.
type pgpKeyring struct {
	entities openpgp.EntityList
}

var _ Keyring = (*pgpKeyring)(nil)

// NewKeyring builds a keyring from armored public keys.
//
// Every key must parse. A keyring that silently dropped an unparseable key
// would fail verification later with an error that blames the signature rather
// than the build.
func NewKeyring(armoredKeys ...[]byte) (Keyring, error) {
	ring := &pgpKeyring{}
	for i, armored := range armoredKeys {
		entities, err := openpgp.ReadArmoredKeyRing(bytes.NewReader(armored))
		if err != nil {
			return nil, fmt.Errorf("imagefetch: parse bundled signing key %d: %w", i, err)
		}
		ring.entities = append(ring.entities, entities...)
	}
	return ring, nil
}

// Verify checks a detached signature over signed.
func (k *pgpKeyring) Verify(signed, signature []byte) error {
	if len(k.entities) == 0 {
		return ErrNoKeys
	}
	// Both the armored and binary forms are accepted: distributions publish
	// .asc and .gpg for the same artifact and which one a catalog entry points
	// at is data, not code.
	if _, err := openpgp.CheckArmoredDetachedSignature(
		k.entities, bytes.NewReader(signed), bytes.NewReader(signature), nil,
	); err == nil {
		return nil
	}
	if _, err := openpgp.CheckDetachedSignature(
		k.entities, bytes.NewReader(signed), bytes.NewReader(signature), nil,
	); err != nil {
		return fmt.Errorf("imagefetch: no bundled key verifies this signature: %w", err)
	}
	return nil
}

// VerifyReader checks a detached signature over a stream. The armored or binary
// form is chosen from the signature's own header, because the stream can only be
// read once and a multi-gigabyte image is not worth reading twice.
func (k *pgpKeyring) VerifyReader(signed io.Reader, signature []byte) error {
	if len(k.entities) == 0 {
		return ErrNoKeys
	}
	if bytes.HasPrefix(bytes.TrimSpace(signature), []byte("-----BEGIN PGP SIGNATURE")) {
		if _, err := openpgp.CheckArmoredDetachedSignature(k.entities, signed, bytes.NewReader(signature), nil); err != nil {
			return fmt.Errorf("imagefetch: no bundled key verifies this signature: %w", err)
		}
		return nil
	}
	if _, err := openpgp.CheckDetachedSignature(k.entities, signed, bytes.NewReader(signature), nil); err != nil {
		return fmt.Errorf("imagefetch: no bundled key verifies this signature: %w", err)
	}
	return nil
}

// VerifyClearsigned checks an inline-signed document and returns the text the
// signature covers. Text outside the signed block is dropped, never returned: a
// mirror could append a forged checksum line after a genuinely signed block.
func (k *pgpKeyring) VerifyClearsigned(document []byte) ([]byte, error) {
	if len(k.entities) == 0 {
		return nil, ErrNoKeys
	}
	block, _ := clearsign.Decode(document)
	if block == nil {
		return nil, errors.New("imagefetch: the checksum file is not signed inline")
	}
	if _, err := block.VerifySignature(k.entities, nil); err != nil {
		return nil, fmt.Errorf("imagefetch: no bundled key verifies this signature: %w", err)
	}
	return block.Plaintext, nil
}
