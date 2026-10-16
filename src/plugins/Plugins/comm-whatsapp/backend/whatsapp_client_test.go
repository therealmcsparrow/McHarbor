// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/therealmcsparrow/mcharbor/core/plugin"
)

type recordedCall struct {
	path        string
	auth        string
	contentType string
	json        map[string]any
	fields      map[string]string
	filename    string
	fileType    string
	fileData    string
}

func fakeCloud(t *testing.T) (*httptest.Server, *[]recordedCall) {
	t.Helper()
	var mu sync.Mutex
	var calls []recordedCall
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c := recordedCall{path: r.URL.Path, auth: r.Header.Get("Authorization"), contentType: r.Header.Get("Content-Type")}
		if mr, err := r.MultipartReader(); err == nil {
			c.fields = map[string]string{}
			for {
				p, err := mr.NextPart()
				if err != nil {
					break
				}
				b, _ := io.ReadAll(p)
				if p.FileName() != "" {
					c.filename, c.fileType, c.fileData = p.FileName(), p.Header.Get("Content-Type"), string(b)
				} else {
					c.fields[p.FormName()] = string(b)
				}
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"id":"MEDIA-42"}`))
		} else {
			raw, _ := io.ReadAll(r.Body)
			_ = json.Unmarshal(raw, &c.json)
			w.WriteHeader(http.StatusOK)
		}
		mu.Lock()
		calls = append(calls, c)
		mu.Unlock()
	}))
	old := whatsappCloudBase
	whatsappCloudBase = srv.URL
	t.Cleanup(func() { whatsappCloudBase = old; srv.Close() })
	return srv, &calls
}

func cloudCfg() map[string]string {
	return map[string]string{"method": methodCloud, "phone_number_id": "555", "access_token": "tok", "recipient_phone": "+31611111111"}
}

func TestSendCloudUploadsMediaThenSendsByID(t *testing.T) {
	_, calls := fakeCloud(t)
	msg := plugin.NotificationMessage{Title: "Report", Message: "attached", Files: []plugin.NotificationFile{
		{Filename: "capacity.pdf", ContentType: "application/pdf", Data: []byte("%PDF-1")},
	}}
	if err := NewWhatsappClient().Send(context.Background(), cloudCfg(), msg); err != nil {
		t.Fatalf("Send: %v", err)
	}
	c := *calls
	if len(c) != 3 {
		t.Fatalf("got %d calls, want text, upload, document send", len(c))
	}
	if c[0].path != "/v21.0/555/messages" || c[0].json["type"] != "text" {
		t.Fatalf("first call = %+v, want the text message", c[0])
	}
	up := c[1]
	if up.path != "/v21.0/555/media" || up.auth != "Bearer tok" {
		t.Fatalf("upload call = %+v", up)
	}
	if up.fields["messaging_product"] != "whatsapp" || up.fields["type"] != "application/pdf" {
		t.Fatalf("upload fields = %v", up.fields)
	}
	if up.filename != "capacity.pdf" || up.fileType != "application/pdf" || up.fileData != "%PDF-1" {
		t.Fatalf("upload file = %q %q %q", up.filename, up.fileType, up.fileData)
	}
	send := c[2]
	doc, _ := send.json["document"].(map[string]any)
	if send.path != "/v21.0/555/messages" || send.json["type"] != "document" || send.json["to"] != "+31611111111" ||
		doc["id"] != "MEDIA-42" || doc["filename"] != "capacity.pdf" {
		t.Fatalf("document send = %+v", send.json)
	}
}

func TestSendCloudWithoutFilesMakesOneTextCall(t *testing.T) {
	_, calls := fakeCloud(t)
	if err := NewWhatsappClient().Send(context.Background(), cloudCfg(), plugin.NotificationMessage{Title: "t", Message: "m"}); err != nil {
		t.Fatalf("Send: %v", err)
	}
	if len(*calls) != 1 {
		t.Fatalf("got %d calls, want 1", len(*calls))
	}
}

func TestSendCloudUploadFailureIsReported(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v21.0/555/media" {
			http.Error(w, `{"error":"too large"}`, http.StatusBadRequest)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()
	old := whatsappCloudBase
	whatsappCloudBase = srv.URL
	defer func() { whatsappCloudBase = old }()
	msg := plugin.NotificationMessage{Title: "t", Message: "m", Files: []plugin.NotificationFile{{Filename: "a.pdf", Data: []byte("x")}}}
	err := NewWhatsappClient().Send(context.Background(), cloudCfg(), msg)
	if err == nil {
		t.Fatal("want the upload failure to surface")
	}
}

func TestSendFilesOnNonCloudMethodFailsLoudly(t *testing.T) {
	cfg := map[string]string{"method": methodGateway, "gateway_url": "http://127.0.0.1:1", "gateway_token": "x", "recipient_phone": "+31611111111"}
	msg := plugin.NotificationMessage{Title: "t", Message: "m", Files: []plugin.NotificationFile{{Filename: "a.txt", Data: []byte("x")}}}
	if err := NewWhatsappClient().Send(context.Background(), cfg, msg); !errors.Is(err, errWhatsappAttachmentsUnsupported) {
		t.Fatalf("err = %v, want errWhatsappAttachmentsUnsupported", err)
	}
}
