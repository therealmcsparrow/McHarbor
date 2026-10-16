// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha1" //nolint:gosec // SPICE's ticket exchange is fixed at RSA-OAEP with SHA-1; the relay must speak it
	"crypto/subtle"
	"crypto/x509"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"strings"
)

// The SPICE link handshake, as far as the remote-viewer relay needs it.
//
// QEMU's SPICE socket runs with ticketing off, because only the agent can reach
// it. A remote-viewer on the operator's machine reaches a TCP port the agent
// opens instead, and the relay on that port is what checks the password. It
// does so inside SPICE's own ticket exchange rather than in front of it: the
// relay swaps QEMU's public key in the link reply for its own, decrypts the
// ticket the client encrypted to it, and only then lets bytes through. So a
// stock remote-viewer works unmodified, and the password never crosses the
// wire in the clear.
//
// All integers on the wire are little-endian; the structures are packed.

const (
	spiceMagic           = 0x51444552 // "REDQ"
	spiceMajorVersion    = 2
	spiceLinkHeaderBytes = 16
	// spiceLinkMaxBody bounds what the relay reads before it has authenticated
	// anybody. A real link message is under a hundred bytes.
	spiceLinkMaxBody = 4096
	// spiceTicketPubKeyBytes is a 1024-bit RSA SubjectPublicKeyInfo in DER,
	// the size SPICE fixes in its link reply.
	spiceTicketPubKeyBytes = 162
	// spiceTicketBytes is one RSA-1024 block, the encrypted ticket.
	spiceTicketBytes = 128
	// spicePasswordMax is SPICE's own password length limit.
	spicePasswordMax = 60

	spiceCapAuthSelection = 0
	spiceCapAuthSpice     = 1

	spiceLinkErrOK               = 0
	spiceLinkErrPermissionDenied = 7

	spiceLinkMessFixedBytes  = 18
	spiceLinkReplyFixedBytes = 4 + spiceTicketPubKeyBytes + 12
)

// errSpiceDenied is a client that presented the wrong password or asked for an
// authentication mechanism the relay does not check.
var errSpiceDenied = errors.New("spice relay: the ticket was refused")

// spiceLinkMessage is one link-stage message: the raw header and body, kept
// whole so it can be forwarded byte for byte.
type spiceLinkMessage struct {
	header [spiceLinkHeaderBytes]byte
	body   []byte
}

// readSpiceLink reads one link message and checks its framing.
func readSpiceLink(r io.Reader) (spiceLinkMessage, error) {
	var msg spiceLinkMessage
	if _, err := io.ReadFull(r, msg.header[:]); err != nil {
		return msg, fmt.Errorf("spice relay: reading a link header: %w", err)
	}
	if binary.LittleEndian.Uint32(msg.header[0:4]) != spiceMagic {
		return msg, errors.New("spice relay: not a SPICE link message")
	}
	if binary.LittleEndian.Uint32(msg.header[4:8]) != spiceMajorVersion {
		return msg, errors.New("spice relay: unsupported SPICE protocol version")
	}
	size := binary.LittleEndian.Uint32(msg.header[12:16])
	if size > spiceLinkMaxBody {
		return msg, fmt.Errorf("spice relay: link message of %d bytes is too large", size)
	}
	msg.body = make([]byte, size)
	if _, err := io.ReadFull(r, msg.body); err != nil {
		return msg, fmt.Errorf("spice relay: reading a link body: %w", err)
	}
	return msg, nil
}

// bytes returns the message as it goes on the wire.
func (m spiceLinkMessage) bytes() []byte {
	out := make([]byte, 0, len(m.header)+len(m.body))
	out = append(out, m.header[:]...)
	return append(out, m.body...)
}

// spiceCaps reads the common capability words at the given offsets of a body.
func spiceCaps(body []byte, countAt, offsetAt int) ([]uint32, error) {
	if len(body) < offsetAt+4 {
		return nil, errors.New("spice relay: link message too short")
	}
	count := binary.LittleEndian.Uint32(body[countAt : countAt+4])
	offset := binary.LittleEndian.Uint32(body[offsetAt : offsetAt+4])
	if count > 64 || uint64(offset)+uint64(count)*4 > uint64(len(body)) {
		return nil, errors.New("spice relay: link capabilities out of range")
	}
	caps := make([]uint32, count)
	for i := range caps {
		at := int(offset) + i*4
		caps[i] = binary.LittleEndian.Uint32(body[at : at+4])
	}
	return caps, nil
}

// clientCommonCaps reads the common capabilities of a SpiceLinkMess.
func clientCommonCaps(body []byte) ([]uint32, error) {
	if len(body) < spiceLinkMessFixedBytes {
		return nil, errors.New("spice relay: link message too short")
	}
	return spiceCaps(body, 6, 14)
}

// serverReply reads a SpiceLinkReply's error, public key and common caps.
func serverReply(body []byte) (uint32, []byte, []uint32, error) {
	if len(body) < spiceLinkReplyFixedBytes {
		return 0, nil, nil, errors.New("spice relay: link reply too short")
	}
	code := binary.LittleEndian.Uint32(body[0:4])
	key := body[4 : 4+spiceTicketPubKeyBytes]
	caps, err := spiceCaps(body, 4+spiceTicketPubKeyBytes, 4+spiceTicketPubKeyBytes+8)
	return code, key, caps, err
}

// hasSpiceCap reports whether bit is set in a capability word list.
func hasSpiceCap(caps []uint32, bit int) bool {
	word := bit / 32
	return word < len(caps) && caps[word]&(1<<(bit%32)) != 0
}

// spiceLinkResult is the four-byte verdict the server sends after the ticket.
func spiceLinkResult(code uint32) []byte {
	out := make([]byte, 4)
	binary.LittleEndian.PutUint32(out, code)
	return out
}

// spiceRelayKey is the relay's own ticket key and its DER encoding.
type spiceRelayKey struct {
	private *rsa.PrivateKey
	der     []byte
}

// newSpiceRelayKey makes the per-ticket RSA key clients encrypt the password to.
// 1024 bits is not a choice: SPICE fixes the public key field at 162 bytes.
func newSpiceRelayKey() (spiceRelayKey, error) {
	private, err := rsa.GenerateKey(rand.Reader, 1024)
	if err != nil {
		return spiceRelayKey{}, fmt.Errorf("spice relay: generating the ticket key: %w", err)
	}
	der, err := x509.MarshalPKIXPublicKey(&private.PublicKey)
	if err != nil {
		return spiceRelayKey{}, fmt.Errorf("spice relay: encoding the ticket key: %w", err)
	}
	if len(der) != spiceTicketPubKeyBytes {
		return spiceRelayKey{}, fmt.Errorf("spice relay: ticket key encodes to %d bytes, SPICE needs %d",
			len(der), spiceTicketPubKeyBytes)
	}
	return spiceRelayKey{private: private, der: der}, nil
}

// passwordMatches compares a decrypted ticket with the expected password in
// constant time; the ticket is NUL-terminated on the wire.
func passwordMatches(plain []byte, want string) bool {
	got := strings.TrimRight(string(plain), "\x00")
	if len(got) > spicePasswordMax || want == "" {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(got), []byte(want)) == 1
}

// relaySpiceLink runs the link stage between a remote-viewer and QEMU, checking
// the password in the middle. On success both streams are positioned just after
// the ticket, and the caller copies bytes in both directions — QEMU's own link
// verdict is the first thing that flows back.
func relaySpiceLink(client, upstream io.ReadWriter, key spiceRelayKey, password string) error {
	mess, err := readSpiceLink(client)
	if err != nil {
		return err
	}
	clientCaps, err := clientCommonCaps(mess.body)
	if err != nil {
		return err
	}
	if _, err := upstream.Write(mess.bytes()); err != nil {
		return fmt.Errorf("spice relay: forwarding the link message: %w", err)
	}

	reply, err := readSpiceLink(upstream)
	if err != nil {
		return err
	}
	code, qemuKeyDER, serverCaps, err := serverReply(reply.body)
	if err != nil {
		return err
	}
	if code != spiceLinkErrOK {
		// QEMU refused the channel itself; the client may as well hear why.
		_, writeErr := client.Write(reply.bytes())
		return errors.Join(fmt.Errorf("spice relay: QEMU refused the link with code %d", code), writeErr)
	}
	qemuKey, err := x509.ParsePKIXPublicKey(qemuKeyDER)
	if err != nil {
		return fmt.Errorf("spice relay: reading QEMU's ticket key: %w", err)
	}
	qemuRSA, ok := qemuKey.(*rsa.PublicKey)
	if !ok {
		return errors.New("spice relay: QEMU's ticket key is not RSA")
	}

	swapped := reply
	swapped.body = append([]byte(nil), reply.body...)
	copy(swapped.body[4:4+spiceTicketPubKeyBytes], key.der)
	if _, err := client.Write(swapped.bytes()); err != nil {
		return fmt.Errorf("spice relay: answering the client: %w", err)
	}

	selection := hasSpiceCap(clientCaps, spiceCapAuthSelection) && hasSpiceCap(serverCaps, spiceCapAuthSelection)
	if selection {
		mechanism := make([]byte, 4)
		if _, err := io.ReadFull(client, mechanism); err != nil {
			return fmt.Errorf("spice relay: reading the auth mechanism: %w", err)
		}
		if binary.LittleEndian.Uint32(mechanism) != spiceCapAuthSpice {
			// SASL would carry its own credentials, which the relay cannot check.
			_, writeErr := client.Write(spiceLinkResult(spiceLinkErrPermissionDenied))
			return errors.Join(errSpiceDenied, writeErr)
		}
	}

	ticket := make([]byte, spiceTicketBytes)
	if _, err := io.ReadFull(client, ticket); err != nil {
		return fmt.Errorf("spice relay: reading the ticket: %w", err)
	}
	plain, err := rsa.DecryptOAEP(sha1.New(), nil, key.private, ticket, nil) //nolint:gosec // SPICE fixes SHA-1 in its OAEP ticket
	if err != nil || !passwordMatches(plain, password) {
		_, writeErr := client.Write(spiceLinkResult(spiceLinkErrPermissionDenied))
		return errors.Join(errSpiceDenied, writeErr)
	}

	// QEMU has ticketing off and ignores the ticket's content, but it still
	// reads one; an empty password encrypted to its own key is the honest one.
	forward, err := rsa.EncryptOAEP(sha1.New(), rand.Reader, qemuRSA, []byte{0}, nil) //nolint:gosec // SPICE fixes SHA-1 in its OAEP ticket
	if err != nil {
		return fmt.Errorf("spice relay: encrypting QEMU's ticket: %w", err)
	}
	if selection {
		if _, err := upstream.Write(spiceLinkResult(spiceCapAuthSpice)); err != nil {
			return fmt.Errorf("spice relay: forwarding the auth mechanism: %w", err)
		}
	}
	if _, err := upstream.Write(forward); err != nil {
		return fmt.Errorf("spice relay: forwarding the ticket: %w", err)
	}
	return nil
}
