// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/therealmcsparrow/mcharbor/core/plugin"
)

func signalCfg(method, url string) map[string]string {
	return map[string]string{"method": method, "server_url": url, "sender_number": "+31600000000", "recipients": "+31611111111"}
}

func TestSendRestCarriesFilesAsBase64Attachments(t *testing.T) {
	var got map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		if err := json.Unmarshal(raw, &got); err != nil {
			t.Errorf("body is not JSON: %v", err)
		}
		w.WriteHeader(http.StatusCreated)
	}))
	defer srv.Close()

	msg := plugin.NotificationMessage{Title: "Report", Message: "attached", Files: []plugin.NotificationFile{
		{Filename: "cap;acity,2026.pdf", ContentType: "application/pdf", Data: []byte("%PDF-1")},
		{Filename: "raw.bin", Data: []byte{0, 1, 2}},
	}}
	if err := NewSignalClient().Send(context.Background(), signalCfg(methodRest, srv.URL), msg); err != nil {
		t.Fatalf("Send: %v", err)
	}
	atts, _ := got["base64_attachments"].([]any)
	if len(atts) != 2 {
		t.Fatalf("base64_attachments = %v, want 2 entries", got["base64_attachments"])
	}
	want0 := "data:application/pdf;filename=cap_acity_2026.pdf;base64," + base64.StdEncoding.EncodeToString([]byte("%PDF-1"))
	want1 := "data:application/octet-stream;filename=raw.bin;base64," + base64.StdEncoding.EncodeToString([]byte{0, 1, 2})
	if atts[0] != want0 || atts[1] != want1 {
		t.Fatalf("attachments = %v", atts)
	}
}

func TestSendRestWithoutFilesOmitsAttachmentKey(t *testing.T) {
	var got map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(raw, &got)
		w.WriteHeader(http.StatusCreated)
	}))
	defer srv.Close()
	if err := NewSignalClient().Send(context.Background(), signalCfg(methodRest, srv.URL), plugin.NotificationMessage{Title: "t", Message: "m"}); err != nil {
		t.Fatalf("Send: %v", err)
	}
	if _, ok := got["base64_attachments"]; ok {
		t.Fatalf("text-only send must not carry base64_attachments: %v", got)
	}
}

func TestSendFilesOnNonRestMethodFailsLoudly(t *testing.T) {
	called := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { called = true }))
	defer srv.Close()
	msg := plugin.NotificationMessage{Title: "t", Message: "m", Files: []plugin.NotificationFile{{Filename: "a.txt", Data: []byte("x")}}}
	err := NewSignalClient().Send(context.Background(), signalCfg(methodBot, srv.URL), msg)
	if !errors.Is(err, errSignalAttachmentsUnsupported) {
		t.Fatalf("err = %v, want errSignalAttachmentsUnsupported", err)
	}
	if called {
		t.Fatal("no request may be made when attachments cannot be carried")
	}
}
