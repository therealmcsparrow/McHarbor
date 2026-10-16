// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha1" //nolint:gosec // the test speaks SPICE's SHA-1 OAEP ticket like a real client
	"crypto/x509"
	"encoding/binary"
	"errors"
	"io"
	"net"
	"testing"
	"time"
)

const spiceTestCaps = uint32(1<<spiceCapAuthSelection | 1<<spiceCapAuthSpice)

func spiceTestHeader(size int) []byte {
	header := make([]byte, spiceLinkHeaderBytes)
	binary.LittleEndian.PutUint32(header[0:4], spiceMagic)
	binary.LittleEndian.PutUint32(header[4:8], spiceMajorVersion)
	binary.LittleEndian.PutUint32(header[8:12], 2)
	binary.LittleEndian.PutUint32(header[12:16], uint32(size))
	return header
}

func spiceTestLinkMess() []byte {
	body := make([]byte, spiceLinkMessFixedBytes+4)
	body[4] = 1 // main channel
	binary.LittleEndian.PutUint32(body[6:10], 1)
	binary.LittleEndian.PutUint32(body[14:18], spiceLinkMessFixedBytes)
	binary.LittleEndian.PutUint32(body[18:22], spiceTestCaps)
	return append(spiceTestHeader(len(body)), body...)
}

func spiceTestReply(t *testing.T, key *rsa.PrivateKey) []byte {
	t.Helper()
	der, err := x509.MarshalPKIXPublicKey(&key.PublicKey)
	if err != nil {
		t.Fatal(err)
	}
	body := make([]byte, spiceLinkReplyFixedBytes+4)
	copy(body[4:4+spiceTicketPubKeyBytes], der)
	binary.LittleEndian.PutUint32(body[166:170], 1)
	binary.LittleEndian.PutUint32(body[174:178], spiceLinkReplyFixedBytes)
	binary.LittleEndian.PutUint32(body[178:182], spiceTestCaps)
	return append(spiceTestHeader(len(body)), body...)
}

// fakeQEMU plays SPICE's server side and reports the ticket it received.
func fakeQEMU(t *testing.T, conn net.Conn, key *rsa.PrivateKey, tickets chan<- []byte) {
	if _, err := readSpiceLink(conn); err != nil {
		tickets <- nil
		return
	}
	if _, err := conn.Write(spiceTestReply(t, key)); err != nil {
		tickets <- nil
		return
	}
	mechanism := make([]byte, 4)
	if _, err := io.ReadFull(conn, mechanism); err != nil {
		tickets <- nil
		return
	}
	ticket := make([]byte, spiceTicketBytes)
	if _, err := io.ReadFull(conn, ticket); err != nil {
		tickets <- nil
		return
	}
	plain, err := rsa.DecryptOAEP(sha1.New(), nil, key, ticket, nil) //nolint:gosec // SPICE's own ticket scheme
	if err != nil {
		tickets <- []byte("undecryptable")
		return
	}
	tickets <- plain
	if _, err := conn.Write(spiceLinkResult(spiceLinkErrOK)); err != nil {
		t.Errorf("fake QEMU verdict: %v", err)
	}
}

// spiceClient plays remote-viewer: it encrypts the password to whatever key the
// link reply carries, which is the relay's, never QEMU's.
func spiceClient(conn net.Conn, password string) (uint32, error) {
	if _, err := conn.Write(spiceTestLinkMess()); err != nil {
		return 0, err
	}
	reply, err := readSpiceLink(conn)
	if err != nil {
		return 0, err
	}
	pub, err := x509.ParsePKIXPublicKey(reply.body[4 : 4+spiceTicketPubKeyBytes])
	if err != nil {
		return 0, err
	}
	ticket, err := rsa.EncryptOAEP(sha1.New(), rand.Reader, pub.(*rsa.PublicKey), append([]byte(password), 0), nil) //nolint:gosec // SPICE's own ticket scheme
	if err != nil {
		return 0, err
	}
	if _, err := conn.Write(spiceLinkResult(spiceCapAuthSpice)); err != nil {
		return 0, err
	}
	if _, err := conn.Write(ticket); err != nil {
		return 0, err
	}
	verdict := make([]byte, 4)
	if _, err := io.ReadFull(conn, verdict); err != nil {
		return 0, err
	}
	return binary.LittleEndian.Uint32(verdict), nil
}

// spiceRelayOutcome is what one relayed handshake produced on each side.
type spiceRelayOutcome struct {
	verdict    uint32
	qemuTicket []byte
	relayErr   error
}

// closePipe closes one end of a test pipe; a second close is expected and quiet.
func closePipe(t *testing.T, conn net.Conn) {
	if err := conn.Close(); err != nil && !errors.Is(err, io.ErrClosedPipe) {
		t.Logf("closing a test pipe: %v", err)
	}
}

func runSpiceRelay(t *testing.T, password, presented string) spiceRelayOutcome {
	t.Helper()
	qemuKey, err := rsa.GenerateKey(rand.Reader, 1024)
	if err != nil {
		t.Fatal(err)
	}
	relayKey, err := newSpiceRelayKey()
	if err != nil {
		t.Fatal(err)
	}
	clientSide, relayClient := net.Pipe()
	relayUpstream, qemuSide := net.Pipe()
	t.Cleanup(func() {
		for _, conn := range []net.Conn{clientSide, relayClient, relayUpstream, qemuSide} {
			closePipe(t, conn)
		}
	})

	tickets := make(chan []byte, 1)
	go fakeQEMU(t, qemuSide, qemuKey, tickets)

	relayDone := make(chan error, 1)
	go func() {
		err := relaySpiceLink(relayClient, relayUpstream, relayKey, password)
		if err == nil {
			// The verdict is QEMU's: carry it through as the byte copy would.
			if _, copyErr := io.CopyN(relayClient, relayUpstream, 4); copyErr != nil {
				t.Errorf("carrying the verdict: %v", copyErr)
			}
		} else {
			closePipe(t, relayUpstream)
		}
		relayDone <- err
	}()

	var outcome spiceRelayOutcome
	verdict, clientErr := spiceClient(clientSide, presented)
	if clientErr != nil {
		t.Fatalf("client: %v", clientErr)
	}
	outcome.verdict = verdict
	select {
	case outcome.relayErr = <-relayDone:
	case <-time.After(10 * time.Second):
		t.Fatal("relay did not finish")
	}
	select {
	case outcome.qemuTicket = <-tickets:
	case <-time.After(10 * time.Second):
		t.Fatal("fake QEMU did not finish")
	}
	return outcome
}

func TestSpiceRelayLetsTheRightPasswordThrough(t *testing.T) {
	outcome := runSpiceRelay(t, "correct-horse", "correct-horse")
	if outcome.relayErr != nil {
		t.Fatalf("relay refused a correct password: %v", outcome.relayErr)
	}
	if outcome.verdict != spiceLinkErrOK {
		t.Fatalf("client verdict %d, want OK", outcome.verdict)
	}
	// QEMU must get a ticket it can decrypt with its own key; the operator's
	// password is the relay's business and never goes further.
	if string(outcome.qemuTicket) != "\x00" {
		t.Fatalf("QEMU received %q, want an empty password", outcome.qemuTicket)
	}
}

func TestSpiceRelayRefusesTheWrongPasswordBeforeQEMUSeesAnything(t *testing.T) {
	outcome := runSpiceRelay(t, "correct-horse", "battery-staple")
	if !errors.Is(outcome.relayErr, errSpiceDenied) {
		t.Fatalf("relay error %v, want errSpiceDenied", outcome.relayErr)
	}
	if outcome.verdict != spiceLinkErrPermissionDenied {
		t.Fatalf("client verdict %d, want permission denied", outcome.verdict)
	}
	if outcome.qemuTicket != nil {
		t.Fatalf("QEMU received a ticket after a refused password: %q", outcome.qemuTicket)
	}
}

func TestSpicePasswordCheckIsExact(t *testing.T) {
	for _, tc := range []struct {
		plain, want string
		ok          bool
	}{
		{"abc\x00", "abc", true},
		{"abc", "abc", true},
		{"abcd\x00", "abc", false},
		{"", "", false},
	} {
		if got := passwordMatches([]byte(tc.plain), tc.want); got != tc.ok {
			t.Fatalf("passwordMatches(%q, %q) = %v, want %v", tc.plain, tc.want, got, tc.ok)
		}
	}
}
