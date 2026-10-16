// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package agentrpc

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	coreagent "github.com/therealmcsparrow/mcharbor/core/agent"
)

// TestComputeFoundationRoutesAgainstTheDemoAgent is the end-to-end floor for the ceiling's
// compute routes, as addon-compute declares them: probe, pool detect/create/browse/file,
// link detection, the image list, a fetch that streams progress, one the caller abandons,
// and a relocation plan, all through the real transport to the compute simulator.
func TestComputeFoundationRoutesAgainstTheDemoAgent(t *testing.T) {
	conn := connectDemoAgent(t)
	b := quietBridge(demoPool{conn: conn})
	view := b.ForPlugin("addon-compute", []string{
		coreagent.MsgComputeProbeRequest, coreagent.MsgComputeMigrateRequest,
		coreagent.MsgPoolDetectRequest, coreagent.MsgPoolCreateRequest, coreagent.MsgPoolBrowseRequest,
		coreagent.MsgPoolFileRequest, coreagent.MsgNetDetectRequest,
		coreagent.MsgImageListRequest, coreagent.MsgImageFetchRequest,
	}, []string{coreagent.MsgComputeMigrateProgress, coreagent.MsgNetLeaseEvent})

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	call := func(msgType, payload string, onProgress func(string, []byte)) []byte {
		t.Helper()
		reply, err := view.Call(ctx, "env-demo", msgType, []byte(payload), onProgress)
		if err != nil {
			t.Fatalf("%s: %v", msgType, err)
		}
		return reply
	}

	var probe coreagent.ComputePayload
	if err := json.Unmarshal(call(coreagent.MsgComputeProbeRequest, ``, nil), &probe); err != nil ||
		probe.Capability == nil || !probe.Capability.CanRunVMs {
		t.Fatalf("probe = %+v, %v", probe, err)
	}

	var detect coreagent.ComputePoolPayload
	if err := json.Unmarshal(call(coreagent.MsgPoolDetectRequest, ``, nil), &detect); err != nil || len(detect.Detected) == 0 {
		t.Fatalf("pool detect = %+v, %v", detect, err)
	}
	pool := `{"poolId":"p-e2e","name":"e2e","backend":"dir","target":"/var/lib/mcharbor/pools/e2e"}`
	var created coreagent.ComputePoolPayload
	if err := json.Unmarshal(call(coreagent.MsgPoolCreateRequest, pool, nil), &created); err != nil || created.Stats == nil {
		t.Fatalf("pool create = %+v, %v", created, err)
	}
	var browsed coreagent.ComputePoolPayload
	if err := json.Unmarshal(call(coreagent.MsgPoolBrowseRequest, pool, nil), &browsed); err != nil || !browsed.Success {
		t.Fatalf("pool browse = %+v, %v", browsed, err)
	}
	var read coreagent.ComputePoolPayload
	readReq := strings.TrimSuffix(pool, "}") + `,"operation":"read","path":"README"}`
	if err := json.Unmarshal(call(coreagent.MsgPoolFileRequest, readReq, nil), &read); err != nil ||
		!strings.Contains(string(read.Content), "demo mode") {
		t.Fatalf("pool file read = %+v, %v", read, err)
	}

	var links coreagent.ComputeNetPayload
	if err := json.Unmarshal(call(coreagent.MsgNetDetectRequest, ``, nil), &links); err != nil || len(links.Links) == 0 {
		t.Fatalf("net detect = %+v, %v", links, err)
	}

	var mu sync.Mutex
	var percents []int
	fetched := call(coreagent.MsgImageFetchRequest, `{"imageId":"e2e-img","kind":"cloud_image","sourceUrl":"https://mirror/x.qcow2"}`,
		func(msgType string, frame []byte) {
			var p coreagent.ComputeImagePayload
			if msgType != coreagent.MsgImageFetchProgress || json.Unmarshal(frame, &p) != nil {
				t.Errorf("unexpected progress %s %s", msgType, frame)
				return
			}
			mu.Lock()
			percents = append(percents, p.Percent)
			mu.Unlock()
		})
	if !strings.Contains(string(fetched), `"success":true`) {
		t.Fatalf("image fetch = %s", fetched)
	}
	mu.Lock()
	if len(percents) != 5 || percents[4] != 100 {
		t.Errorf("fetch progress = %v, want the simulator's five frames", percents)
	}
	mu.Unlock()
	if listed := call(coreagent.MsgImageListRequest, ``, nil); !strings.Contains(string(listed), `"imageId":"e2e-img"`) {
		t.Fatalf("image list after the fetch = %s", listed)
	}

	short, stop := context.WithTimeout(ctx, 200*time.Millisecond)
	_, err := view.Call(short, "env-demo", coreagent.MsgImageFetchRequest, []byte(`{"imageId":"abandoned","kind":"cloud_image"}`), nil)
	stop()
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("abandoned fetch = %v", err)
	}
	time.Sleep(1200 * time.Millisecond)
	if strings.Contains(string(call(coreagent.MsgImageListRequest, ``, nil)), "abandoned") {
		t.Fatal("the simulator finished a fetch the caller abandoned; the cancel never reached it")
	}

	var plan coreagent.ComputePayload
	if err := json.Unmarshal(call(coreagent.MsgComputeMigrateRequest, `{"migrate":{"plan":true,"diskFrom":"/a","diskTo":"/b"}}`, nil), &plan); err != nil ||
		plan.Migrate == nil || len(plan.Migrate.Notes) == 0 {
		t.Fatalf("relocation plan = %+v, %v", plan, err)
	}
}
