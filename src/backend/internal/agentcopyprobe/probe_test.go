// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package agentcopyprobe

import (
	"bytes"
	"crypto/sha256"
	"crypto/tls"
	"encoding/hex"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/rs/xid"
	"github.com/therealmcsparrow/mcharbor/internal/closeutil"
)

// probe drives a running McHarbor over its public API, so every Docker call it
// causes travels the target environment's real transport — the agent's, for an
// agent environment.
type probe struct {
	t    *testing.T
	base string
	key  string
	env  string
	http *http.Client
}

type envelope struct {
	Success bool            `json:"success"`
	Data    json.RawMessage `json:"data"`
	Error   string          `json:"error"`
	Code    string          `json:"code"`
}

func (p *probe) call(method, path string, query url.Values, body io.Reader, contentType string) (int, []byte, time.Duration) {
	p.t.Helper()
	if query == nil {
		query = url.Values{}
	}
	query.Set("env", p.env)
	req, err := http.NewRequest(method, p.base+path+"?"+query.Encode(), body)
	if err != nil {
		p.t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer "+p.key)
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	start := time.Now()
	resp, err := p.http.Do(req)
	if err != nil {
		p.t.Fatalf("%s %s: %v", method, path, err)
	}
	defer func() { closeutil.Report(resp.Body.Close(), "resp.Body") }()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		p.t.Fatalf("%s %s: reading body: %v", method, path, err)
	}
	return resp.StatusCode, raw, time.Since(start)
}

func (p *probe) jsonCall(method, path string, query url.Values, body any, want int) json.RawMessage {
	p.t.Helper()
	var reader io.Reader
	contentType := ""
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			p.t.Fatal(err)
		}
		reader, contentType = bytes.NewReader(raw), "application/json"
	}
	status, raw, _ := p.call(method, path, query, reader, contentType)
	if status != want {
		p.t.Fatalf("%s %s: status %d, want %d: %s", method, path, status, want, raw)
	}
	var env envelope
	if err := json.Unmarshal(raw, &env); err != nil {
		p.t.Fatalf("%s %s: not a McHarbor envelope: %s", method, path, raw)
	}
	return env.Data
}

func (p *probe) download(containerID, path string) []byte {
	p.t.Helper()
	status, raw, _ := p.call(http.MethodGet, "/api/containers/"+containerID+"/files/content", url.Values{"path": {path}, "download": {"true"}}, nil, "")
	if status != http.StatusOK {
		p.t.Fatalf("download %s: status %d: %s", path, status, raw)
	}
	return raw
}

func digest(b []byte) string {
	sum := sha256.Sum256(b)
	return "sha256:" + hex.EncodeToString(sum[:])
}

// TestAgentCopyToContainer is TOFIX-05: a ~1.1 MB CopyToContainer through a native
// agent, verified byte for byte. It is skipped unless MCHARBOR_PROBE_URL is set.
//
//	MCHARBOR_PROBE_URL      https://localhost:8705
//	MCHARBOR_PROBE_KEY      an API key holding containers.*, images.manage (+ containers.debug_shell)
//	MCHARBOR_PROBE_ENV      the agent environment id
//	MCHARBOR_PROBE_BINARY   the busybox to upload (mode upload)
//	MCHARBOR_PROBE_MODE     upload (existing files/upload route) or debug-shell (the new route)
//	MCHARBOR_PROBE_IMAGE      an image already on the host; registry.k8s.io/pause:3.10 by default
//	MCHARBOR_PROBE_ENTRYPOINT keeps it running, e.g. "sleep 3600", for an image whose default exits
//	MCHARBOR_PROBE_PULL       1 to pull the image first (left on the host afterwards)
//	MCHARBOR_PROBE_INSECURE   1 to accept the instance's self-signed certificate
func TestAgentCopyToContainer(t *testing.T) {
	base := strings.TrimRight(os.Getenv("MCHARBOR_PROBE_URL"), "/")
	if base == "" {
		t.Skip("MCHARBOR_PROBE_URL not set; this measures a live instance")
	}
	transport := http.DefaultTransport.(*http.Transport).Clone()
	if os.Getenv("MCHARBOR_PROBE_INSECURE") == "1" {
		transport.TLSClientConfig = &tls.Config{InsecureSkipVerify: true} //nolint:gosec // lab instance with a self-signed certificate
	}
	p := &probe{t: t, base: base, key: os.Getenv("MCHARBOR_PROBE_KEY"), env: os.Getenv("MCHARBOR_PROBE_ENV"),
		http: &http.Client{Transport: transport, Timeout: 5 * time.Minute}}
	mode := os.Getenv("MCHARBOR_PROBE_MODE")
	if mode == "" {
		mode = "upload"
	}
	image := os.Getenv("MCHARBOR_PROBE_IMAGE")
	if image == "" {
		image = "registry.k8s.io/pause:3.10"
	}

	if os.Getenv("MCHARBOR_PROBE_PULL") == "1" {
		p.jsonCall(http.MethodPost, "/api/images", nil, map[string]string{"image": image}, http.StatusOK)
		t.Logf("pulled %s; it is left on the host, because whether it was there before cannot be told apart", image)
	}

	name := "mcharbor-agent-probe-" + xid.New().String()
	spec := map[string]any{"name": name, "image": image, "labels": map[string]string{"eu.mcharbor.probe": "agent-copy"}}
	if entrypoint := strings.Fields(os.Getenv("MCHARBOR_PROBE_ENTRYPOINT")); len(entrypoint) > 0 {
		spec["entrypoint"] = entrypoint
	}
	created := p.jsonCall(http.MethodPost, "/api/containers", nil, spec, http.StatusCreated)
	var ref struct {
		ID string `json:"Id"`
	}
	if err := json.Unmarshal(created, &ref); err != nil || ref.ID == "" {
		t.Fatalf("container create answered %s", created)
	}
	t.Cleanup(func() {
		status, raw, _ := p.call(http.MethodDelete, "/api/containers/"+ref.ID, url.Values{"force": {"true"}}, nil, "")
		if status != http.StatusOK && status != http.StatusNoContent {
			t.Errorf("removing probe container %s: status %d: %s", ref.ID, status, raw)
		}
	})
	p.jsonCall(http.MethodPost, "/api/containers/"+ref.ID+"/start", nil, nil, http.StatusOK)

	switch mode {
	case "upload":
		binary, err := os.ReadFile(os.Getenv("MCHARBOR_PROBE_BINARY"))
		if err != nil {
			t.Fatalf("MCHARBOR_PROBE_BINARY: %v", err)
		}
		for i := 1; i <= 3; i++ {
			var body bytes.Buffer
			mw := multipart.NewWriter(&body)
			fw, err := mw.CreateFormFile("files", "probe-busybox")
			if err != nil {
				t.Fatal(err)
			}
			if _, err := fw.Write(binary); err != nil {
				t.Fatal(err)
			}
			if err := mw.Close(); err != nil {
				t.Fatal(err)
			}
			status, raw, took := p.call(http.MethodPost, "/api/containers/"+ref.ID+"/files/upload", url.Values{"path": {"/"}}, &body, mw.FormDataContentType())
			if status != http.StatusOK {
				t.Fatalf("upload %d: status %d: %s", i, status, raw)
			}
			back := p.download(ref.ID, "/probe-busybox")
			t.Logf("run %d: %d bytes up in %s; read back %d bytes, %s (sent %s)", i, len(binary), took.Round(time.Millisecond), len(back), digest(back), digest(binary))
			if !bytes.Equal(back, binary) {
				t.Fatalf("run %d: the bytes that arrived differ from the bytes sent", i)
			}
		}
	case "debug-shell":
		start := time.Now()
		data := p.jsonCall(http.MethodPost, "/api/containers/"+ref.ID+"/debug-shell", nil, nil, http.StatusCreated)
		took := time.Since(start)
		var sess struct {
			SessionID     string `json:"sessionId"`
			Path          string `json:"path"`
			BusyboxDigest string `json:"busyboxDigest"`
			SizeBytes     int    `json:"sizeBytes"`
		}
		if err := json.Unmarshal(data, &sess); err != nil || sess.SessionID == "" {
			t.Fatalf("install answered %s", data)
		}
		back := p.download(ref.ID, sess.Path)
		t.Logf("install: %d bytes in %s; read back %s, audit digest %s", sess.SizeBytes, took.Round(time.Millisecond), digest(back), sess.BusyboxDigest)
		if digest(back) != sess.BusyboxDigest || len(back) != sess.SizeBytes {
			t.Fatalf("the uploaded busybox does not match the digest the install audited")
		}
		removed := p.jsonCall(http.MethodDelete, "/api/containers/"+ref.ID+"/debug-shell", url.Values{"session": {sess.SessionID}}, nil, http.StatusOK)
		t.Logf("remove: %s", removed)
		status := p.jsonCall(http.MethodGet, "/api/containers/"+ref.ID+"/debug-shell", nil, nil, http.StatusOK)
		if !strings.Contains(string(status), `"sessions":[]`) && !strings.Contains(string(status), `"sessions":null`) {
			t.Fatalf("after removal the container still reports sessions: %s", status)
		}
		if code, _, _ := p.call(http.MethodGet, "/api/containers/"+ref.ID+"/files/content", url.Values{"path": {sess.Path}}, nil, ""); code == http.StatusOK {
			t.Fatalf("%s is still in the container after removal", sess.Path)
		}
	default:
		t.Fatalf("unknown MCHARBOR_PROBE_MODE %q", mode)
	}
}
