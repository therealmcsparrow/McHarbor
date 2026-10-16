// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"
)

// filesRPC adds the host's pool file browser to fakeRPC, holding files by their
// pool-relative path.
type filesRPC struct {
	*fakeRPC
	mu    sync.Mutex
	files map[string]string
}

// Call answers pool_file_request reads from the held files and leaves every other
// message to fakeRPC, as the host's bridge carries both.
func (f *filesRPC) Call(ctx context.Context, envID, messageType string, body []byte, onProgress func(string, []byte)) ([]byte, error) {
	if messageType != msgPoolFile {
		return f.fakeRPC.Call(ctx, envID, messageType, body, onProgress)
	}
	var req agentPoolFilePayload
	if err := json.Unmarshal(body, &req); err != nil {
		return nil, err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	content, ok := f.files[req.Path]
	if req.Operation != "read" || !ok {
		return nil, errors.New("storage: not found")
	}
	return json.Marshal(agentPoolFilePayload{Content: []byte(content)})
}

func (f *filesRPC) put(relPath, body string) {
	f.mu.Lock()
	f.files[relPath] = body
	f.mu.Unlock()
}

func newFilesService(t *testing.T) (*Service, *filesRPC) {
	t.Helper()
	base := &fakeRPC{canRun: true}
	svc, _ := newTestService(t, base)
	files := &filesRPC{fakeRPC: base, files: map[string]string{}}
	svc.rpc = files
	return svc, files
}

func startReply(pid int) func(string, agentPayload) ([]byte, error) {
	return func(messageType string, payload agentPayload) ([]byte, error) {
		if messageType == msgCTStart {
			return json.Marshal(agentPayload{Success: true, State: &agentState{
				ContainerID: payload.ContainerID, State: StateRunning, InitPID: pid,
			}})
		}
		return []byte(`{"success":true}`), nil
	}
}

const alpineRelease = "NAME=\"Alpine Linux\"\nID=alpine\nVERSION_ID=3.22.1\nPRETTY_NAME=\"Alpine Linux v3.22\"\n"

func TestCreateRecordsTheDistributionFromTheRootfs(t *testing.T) {
	svc, files := newFilesService(t)
	ctx := context.Background()
	files.reply = func(messageType string, payload agentPayload) ([]byte, error) {
		if messageType == msgCTCreate {
			// The agent has laid the rootfs down by the time it answers.
			files.put("containers/"+payload.ContainerID+"/rootfs/usr/lib/os-release", alpineRelease)
		}
		return []byte(`{"success":true}`), nil
	}

	ct, err := svc.Create(ctx, createRequest())
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if ct.OSFamily != "alpine" || ct.OSVersion != "3.22.1" {
		t.Fatalf("os = %q %q, want alpine 3.22.1 from usr/lib/os-release", ct.OSFamily, ct.OSVersion)
	}
}

func TestStartRecordsTheStartTimeAndAMissingDistribution(t *testing.T) {
	svc, files := newFilesService(t)
	ctx := context.Background()
	ct, err := svc.Create(ctx, createRequest())
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if ct.OSFamily != "" || ct.LastStartedAt != nil {
		t.Fatalf("fresh row = os %q, started %v; want both empty", ct.OSFamily, ct.LastStartedAt)
	}
	files.put("containers/"+ct.ID+"/rootfs/etc/os-release", "ID=\"debian\"\nVERSION_ID=\"13\"\n")
	files.reply = startReply(4242)

	before := time.Now().Add(-time.Second)
	if err := svc.Start(ctx, ct.ID); err != nil {
		t.Fatalf("Start: %v", err)
	}
	got, err := svc.Get(ctx, ct.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.LastStartedAt == nil || got.LastStartedAt.Before(before) {
		t.Fatalf("lastStartedAt = %v after a successful start", got.LastStartedAt)
	}
	if got.OSFamily != "debian" || got.OSVersion != "13" {
		t.Fatalf("os = %q %q, want debian 13", got.OSFamily, got.OSVersion)
	}
	if got.InitPID != 4242 {
		t.Fatalf("initPid = %d, want 4242", got.InitPID)
	}
}

func TestStoppingOrCrashingClearsTheInitPID(t *testing.T) {
	svc, files := newFilesService(t)
	ctx := context.Background()
	ct, err := svc.Create(ctx, createRequest())
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	files.reply = startReply(4242)
	if err := svc.Start(ctx, ct.ID); err != nil {
		t.Fatalf("Start: %v", err)
	}
	files.reply = nil
	if err := svc.Stop(ctx, ct.ID); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	got, err := svc.Get(ctx, ct.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.State != StateStopped || got.InitPID != 0 {
		t.Fatalf("after stop: state %q, initPid %d; want stopped with no pid", got.State, got.InitPID)
	}

	files.reply = startReply(5151)
	if err := svc.Start(ctx, ct.ID); err != nil {
		t.Fatalf("second Start: %v", err)
	}
	// An older agent reconciles a dead container with its stale pid.
	if err := svc.applyState(ctx, ct.ID, &agentState{ContainerID: ct.ID, State: StateCrashed, InitPID: 5151}, StateCrashed); err != nil {
		t.Fatal(err)
	}
	got, err = svc.Get(ctx, ct.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.State != StateCrashed || got.InitPID != 0 {
		t.Fatalf("after a crash: state %q, initPid %d; want crashed with no pid", got.State, got.InitPID)
	}
}

func TestStatsOfARunningContainerComeFromTheAgentsSample(t *testing.T) {
	svc, files := newFilesService(t)
	ctx := context.Background()
	ct, err := svc.Create(ctx, createRequest())
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	files.reply = startReply(4242)
	if err := svc.Start(ctx, ct.ID); err != nil {
		t.Fatalf("Start: %v", err)
	}

	files.reply = func(messageType string, payload agentPayload) ([]byte, error) {
		if messageType == msgCTQuery && payload.Operation == "stats" {
			return []byte(`{"success":true,"stats":{"containerId":"` + payload.ContainerID +
				`","cpuPercent":12.5,"memoryBytes":1048576,"memoryMaxBytes":536870912,"pidsCurrent":3,` +
				`"pidsMax":4096,"netRxBytes":2048,"sampledAt":"2026-09-27T18:30:00Z"}}`), nil
		}
		return []byte(`{"success":true}`), nil
	}
	stats, err := svc.Stats(ctx, ct.ID)
	if err != nil {
		t.Fatalf("Stats: %v", err)
	}
	if stats.CPUPercent != 12.5 || stats.MemoryBytes != 1048576 || stats.PidsCurrent != 3 || stats.NetRxBytes != 2048 {
		t.Fatalf("stats = %+v", stats)
	}

	files.reply = func(string, agentPayload) ([]byte, error) { return []byte(`{"success":true}`), nil }
	if _, err := svc.Stats(ctx, ct.ID); !errors.Is(err, ErrStatsUnavailable) {
		t.Fatalf("a reply without a sample = %v, want ErrStatsUnavailable rather than not running", err)
	}
	recorder := httptest.NewRecorder()
	(&Handler{logger: quietLogger{}}).respondErr(recorder, httptest.NewRequest(http.MethodGet, "/", nil), "stats", ErrStatsUnavailable)
	var envelope apiEnvelope
	if err := json.Unmarshal(recorder.Body.Bytes(), &envelope); err != nil {
		t.Fatal(err)
	}
	if recorder.Code != http.StatusConflict || envelope.Code != "err.ct.stats_unavailable" {
		t.Fatalf("answered %d %q, want 409 err.ct.stats_unavailable", recorder.Code, envelope.Code)
	}
}

func TestParseOSReleaseAndPoolRelative(t *testing.T) {
	family, version := parseOSRelease("# comment\nID='Ubuntu'\nVERSION_ID=\"24.04\"\nID_LIKE=debian\n")
	if family != "ubuntu" || version != "24.04" {
		t.Fatalf("parsed %q %q, want ubuntu 24.04", family, version)
	}
	if rel, ok := poolRelative("/pool/", "/pool/containers/x/rootfs"); !ok || rel != "containers/x/rootfs" {
		t.Fatalf("poolRelative = %q, %v", rel, ok)
	}
	for _, rootfs := range []string{"/pool", "/other/containers/x/rootfs", "/pool-2/containers/x/rootfs"} {
		if _, ok := poolRelative("/pool", rootfs); ok {
			t.Fatalf("%q was accepted as inside /pool", rootfs)
		}
	}
}
