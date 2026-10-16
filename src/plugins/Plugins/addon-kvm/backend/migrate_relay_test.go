// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

import (
	"bytes"
	"context"
	"errors"
	"io"
	"testing"
	"time"
)

func TestTheRelayJoinsOneUploadToOneDownload(t *testing.T) {
	relay := newMigrationRelay()
	grant, err := relay.issue()
	if err != nil {
		t.Fatal(err)
	}
	// Each token opens only its own direction.
	if _, err := relay.redeem(grant.id, grant.receiveToken, true); !errors.Is(err, errRelayGrant) {
		t.Fatalf("receive token opened the upload: %v", err)
	}
	send, err := relay.redeem(grant.id, grant.sendToken, true)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := relay.redeem(grant.id, grant.sendToken, true); !errors.Is(err, errRelayGrant) {
		t.Fatalf("a send token was usable twice: %v", err)
	}
	receive, err := relay.redeem(grant.id, grant.receiveToken, false)
	if err != nil {
		t.Fatal(err)
	}

	payload := bytes.Repeat([]byte("disk"), 64<<10)
	uploaded := make(chan error, 1)
	go func() {
		_, err := relay.upload(context.Background(), send, "qcow2", int64(len(payload)), bytes.NewReader(payload))
		uploaded <- err
	}()
	body, format, size, err := relay.download(context.Background(), receive)
	if err != nil {
		t.Fatal(err)
	}
	got, err := io.ReadAll(body)
	if err != nil {
		t.Fatal(err)
	}
	if format != "qcow2" || size != int64(len(payload)) || !bytes.Equal(got, payload) {
		t.Fatalf("download = %d bytes (%s, announced %d), want %d", len(got), format, size, len(payload))
	}
	select {
	case err := <-uploaded:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("the upload never finished")
	}
}

func TestARevokedRelayEndsTheTransfer(t *testing.T) {
	relay := newMigrationRelay()
	grant, err := relay.issue()
	if err != nil {
		t.Fatal(err)
	}
	send, err := relay.redeem(grant.id, grant.sendToken, true)
	if err != nil {
		t.Fatal(err)
	}
	uploaded := make(chan error, 1)
	go func() {
		_, err := relay.upload(context.Background(), send, "raw", 0, bytes.NewReader([]byte("x")))
		uploaded <- err
	}()
	receive, err := relay.redeem(grant.id, grant.receiveToken, false)
	if err != nil {
		t.Fatal(err)
	}
	relay.revoke(grant.id)
	body, _, _, err := relay.download(context.Background(), receive)
	if err == nil {
		if _, err = io.ReadAll(body); err == nil {
			t.Fatal("a revoked relay still delivered")
		}
	}
	select {
	case <-uploaded:
	case <-time.After(5 * time.Second):
		t.Fatal("a revoked relay left the upload hanging")
	}
}
