// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

// familyAgent answers every compute-family request the typed handle sends with a fully
// populated reply of the matching response type, or with a refusal when fail is set. An
// image fetch gets two progress frames first; a fetch of image "hang" gets no answer, so a
// caller's cancellation is what ends it.
type familyAgent struct {
	fail bool

	mu      sync.Mutex
	frames  [][]byte
	replies map[string]*WSMessage
}

var familyReplyType = map[string]string{
	MsgComputeProbeRequest:   MsgComputeProbeResponse,
	MsgComputeConfigRequest:  MsgComputeConfigResponse,
	MsgComputeInstallRequest: MsgComputeInstallResponse,
	MsgComputeMigrateRequest: MsgComputeMigrateResponse,
	MsgPoolDetectRequest:     MsgPoolDetectResponse,
	MsgPoolCreateRequest:     MsgPoolCreateResponse,
	MsgPoolStatsRequest:      MsgPoolStatsResponse,
	MsgPoolRemoveRequest:     MsgPoolRemoveResponse,
	MsgPoolBrowseRequest:     MsgPoolBrowseResponse,
	MsgPoolFileRequest:       MsgPoolFileResponse,
	MsgNetDetectRequest:      MsgNetDetectResponse,
	MsgImageFetchRequest:     MsgImageFetchResult,
	MsgImageListRequest:      MsgImageListResponse,
	MsgImageRemoveRequest:    MsgImageRemoveResult,
}

func (a *familyAgent) raw() [][]byte {
	a.mu.Lock()
	defer a.mu.Unlock()
	return append([][]byte(nil), a.frames...)
}

func (a *familyAgent) reply(msg WSMessage) *WSMessage {
	out := &WSMessage{Type: familyReplyType[msg.Type], ID: msg.ID}
	switch {
	case msg.Compute != nil:
		c := filled[ComputePayload]()
		out.Compute = &c
	case msg.Pool != nil:
		p := filled[ComputePoolPayload]()
		out.Pool = &p
	case msg.Net != nil:
		n := filled[ComputeNetPayload]()
		out.Net = &n
	case msg.ComputeImage != nil:
		i := filled[ComputeImagePayload]()
		out.ComputeImage = &i
	}
	for _, set := range []func(bool, string){
		func(ok bool, e string) {
			if out.Compute != nil {
				out.Compute.Success, out.Compute.Error = ok, e
			}
		},
		func(ok bool, e string) {
			if out.Pool != nil {
				out.Pool.Success, out.Pool.Error = ok, e
			}
		},
		func(ok bool, e string) {
			if out.Net != nil {
				out.Net.Success, out.Net.Error, out.Net.ErrorCode = ok, e, ""
			}
		},
		func(ok bool, e string) {
			if out.ComputeImage != nil {
				out.ComputeImage.Success, out.ComputeImage.Error = ok, e
			}
		},
	} {
		if a.fail {
			set(false, " the host said no ")
		} else {
			set(true, "")
		}
	}
	return out
}

func (a *familyAgent) serve(t *testing.T) *httptest.Server {
	t.Helper()
	upgrader := websocket.Upgrader{}
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			t.Errorf("upgrade: %v", err)
			return
		}
		defer closeIgnoring(conn)
		for {
			_, data, err := conn.ReadMessage()
			if err != nil {
				return
			}
			a.mu.Lock()
			a.frames = append(a.frames, data)
			a.mu.Unlock()
			var msg WSMessage
			if json.Unmarshal(data, &msg) != nil || familyReplyType[msg.Type] == "" {
				continue
			}
			if msg.Type == MsgImageFetchRequest {
				if msg.ComputeImage != nil && msg.ComputeImage.ImageID == "hang" {
					continue
				}
				for _, pct := range []int{30, 70} {
					if err := conn.WriteJSON(WSMessage{Type: MsgImageFetchProgress, ID: msg.ID,
						ComputeImage: &ComputeImagePayload{ImageID: "img", Percent: pct, Phase: "downloading"}}); err != nil {
						return
					}
				}
				// Lets the typed fetch's pump, which does not drain, hand both frames over.
				time.Sleep(50 * time.Millisecond)
			}
			out := a.reply(msg)
			a.mu.Lock()
			if a.replies == nil {
				a.replies = map[string]*WSMessage{}
			}
			a.replies[msg.ID] = out
			a.mu.Unlock()
			if err := conn.WriteJSON(out); err != nil {
				return
			}
		}
	}))
}

func dialFamilyAgent(t *testing.T, version string, fail bool) (*AgentTransport, *familyAgent) {
	t.Helper()
	agent := &familyAgent{fail: fail}
	srv := agent.serve(t)
	t.Cleanup(srv.Close)
	ws, _, err := dialWS("ws" + strings.TrimPrefix(srv.URL, "http"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { closeIgnoring(ws) })
	conn := &AgentConnection{EnvID: "env-1", Version: version, Build: "209912312359", Conn: ws}
	tr := NewAgentTransport(conn, nil, slog.New(slog.NewTextHandler(io.Discard, nil)))
	conn.Transport = tr
	go func() { ignoreErr(tr.ReadLoop()) }()
	return tr, agent
}

// typedCase is one typed compute call the plugin route must reproduce.
type typedCase struct {
	name    string
	msgType string
	// budget is the timeout the typed method applies, read off compute.go.
	budget time.Duration
	typed  func(ctx context.Context, tr *AgentTransport) error
}

var (
	typedRoots = ComputeRoots{VMRoot: "/srv/vm", CTRoot: "/srv/ct", ImageRoot: "/srv/img"}
	typedPool  = ComputePoolPayload{EnvID: "env-1", PoolID: "p1", Name: "fast", Backend: "zfs", Target: "tank/vms"}
	typedFetch = ComputeImagePayload{EnvID: "env-1", ImageID: "img", Kind: "cloud_image",
		SourceURL: "https://mirror/x.qcow2", ChecksumURL: "https://mirror/SUMS", Checksum: "abc",
		ChecksumAlgo: "sha256", SignatureURL: "https://mirror/SUMS.gpg", AllowUnverified: true,
		Filename: "x.qcow2", LibraryURL: "https://mcharbor/lib/1", LibraryToken: "tok"}
	typedMigrate = ComputeMigrate{VMFrom: "/old/vm", CTFrom: "/old/ct", ImageFrom: "/old/img",
		DiskFrom: "/old/disk", DiskTo: "/new/disk", Roots: &typedRoots,
		Volumes: []ComputeMove{{Kind: "volume", From: "/a", To: "/b", GuestID: "vm1", Slot: 2}}}
)

func typedCases() []typedCase {
	withPool := func(extra func(*ComputePoolPayload)) ComputePoolPayload {
		p := typedPool
		extra(&p)
		return p
	}
	plan := typedMigrate
	plan.Plan = true
	repair := typedMigrate
	repair.Repair = true
	return []typedCase{
		{"probe", MsgComputeProbeRequest, computeTimeout, func(ctx context.Context, tr *AgentTransport) error {
			_, err := tr.ComputeProbe(ctx, "env-1")
			return err
		}},
		{"install", MsgComputeInstallRequest, computeLongTimeout, func(ctx context.Context, tr *AgentTransport) error {
			_, err := tr.ComputeInstall(ctx, "env-1", "qemu_img")
			return err
		}},
		{"config", MsgComputeConfigRequest, computeTimeout, func(ctx context.Context, tr *AgentTransport) error {
			_, err := tr.ComputeConfigure(ctx, "env-1", typedRoots)
			return err
		}},
		{"migrate apply", MsgComputeMigrateRequest, 12 * time.Hour, func(ctx context.Context, tr *AgentTransport) error {
			_, err := tr.ComputeMigrate(ctx, "env-1", typedMigrate)
			return err
		}},
		{"migrate plan", MsgComputeMigrateRequest, computeTimeout, func(ctx context.Context, tr *AgentTransport) error {
			_, err := tr.ComputeMigrate(ctx, "env-1", plan)
			return err
		}},
		{"migrate repair", MsgComputeMigrateRequest, computeTimeout, func(ctx context.Context, tr *AgentTransport) error {
			_, err := tr.ComputeMigrate(ctx, "env-1", repair)
			return err
		}},
		{"pool detect", MsgPoolDetectRequest, computeTimeout, func(ctx context.Context, tr *AgentTransport) error {
			_, err := tr.PoolDetect(ctx, "env-1")
			return err
		}},
		{"pool create", MsgPoolCreateRequest, computeLongTimeout, func(ctx context.Context, tr *AgentTransport) error {
			_, err := tr.PoolCreate(ctx, typedPool)
			return err
		}},
		{"pool stats", MsgPoolStatsRequest, computeTimeout, func(ctx context.Context, tr *AgentTransport) error {
			_, err := tr.PoolStats(ctx, typedPool)
			return err
		}},
		{"pool remove", MsgPoolRemoveRequest, computeLongTimeout, func(ctx context.Context, tr *AgentTransport) error {
			return tr.PoolRemove(ctx, withPool(func(p *ComputePoolPayload) { p.Force = true }))
		}},
		{"pool browse", MsgPoolBrowseRequest, computeTimeout, func(ctx context.Context, tr *AgentTransport) error {
			_, err := tr.PoolBrowse(ctx, withPool(func(p *ComputePoolPayload) { p.Path = "iso" }))
			return err
		}},
		{"pool file write", MsgPoolFileRequest, computeTimeout, func(ctx context.Context, tr *AgentTransport) error {
			_, err := tr.PoolFile(ctx, withPool(func(p *ComputePoolPayload) {
				p.Operation, p.Path, p.Mode, p.Content = "write", "seed/user-data", "0644", []byte("#cloud-config\n")
			}))
			return err
		}},
		{"pool file rename", MsgPoolFileRequest, computeTimeout, func(ctx context.Context, tr *AgentTransport) error {
			_, err := tr.PoolFile(ctx, withPool(func(p *ComputePoolPayload) {
				p.Operation, p.Path, p.NewName, p.Recursive = "rename", "a", "b", true
			}))
			return err
		}},
		{"net detect", MsgNetDetectRequest, computeTimeout, func(ctx context.Context, tr *AgentTransport) error {
			_, err := tr.NetDetect(ctx, "env-1")
			return err
		}},
		{"image list", MsgImageListRequest, computeTimeout, func(ctx context.Context, tr *AgentTransport) error {
			_, err := tr.ImageList(ctx, "env-1")
			return err
		}},
		{"image remove", MsgImageRemoveRequest, computeTimeout, func(ctx context.Context, tr *AgentTransport) error {
			return tr.ImageRemove(ctx, "env-1", "img")
		}},
		// The typed fetch has no host deadline of its own; the route bounds it at the transfer
		// budget, which is the agent's own downloadTimeout, so no fetch the agent would finish
		// is cut short. This is the one budget the route tightens.
		{"image fetch", MsgImageFetchRequest, computeImportTimeout, func(ctx context.Context, tr *AgentTransport) error {
			_, err := tr.ImageFetch(ctx, typedFetch, nil)
			return err
		}},
	}
}

// slotOf returns the one envelope slot a recorded frame carries.
func slotOf(t *testing.T, frame []byte, slot PluginSlot) []byte {
	t.Helper()
	var wire map[string]json.RawMessage
	if err := json.Unmarshal(frame, &wire); err != nil {
		t.Fatal(err)
	}
	key := map[PluginSlot]string{SlotCompute: "compute", SlotPool: "pool", SlotNet: "net", SlotComputeImage: "computeImage"}[slot]
	if len(wire) != 3 || wire[key] == nil {
		t.Fatalf("frame %s is not exactly type, id and the %s slot", frame, key)
	}
	return wire[key]
}

// TestCallReproducesEveryTypedComputeMessage is the parity table for the ceiling's compute
// routes: for every message the typed compute handle sends, Call with that message's slot
// writes the same envelope, runs under the same budget, returns the reply the agent sent,
// and words a refusal exactly as the typed method does.
func TestCallReproducesEveryTypedComputeMessage(t *testing.T) {
	for _, tc := range typedCases() {
		t.Run(tc.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			route, ok := PluginRouteFor(tc.msgType)
			if !ok {
				t.Fatalf("%s is not in the ceiling", tc.msgType)
			}

			tr, agent := dialFamilyAgent(t, "2026.10.01", false)
			if err := tc.typed(ctx, tr); err != nil {
				t.Fatalf("typed: %v", err)
			}
			typedFrame := agent.raw()[0]
			slot := slotOf(t, typedFrame, route.Slot)

			reply, err := tr.Call(ctx, "env-1", tc.msgType, slot, nil)
			if err != nil {
				t.Fatalf("Call: %v", err)
			}
			frames := agent.raw()
			if len(frames) != 2 {
				t.Fatalf("agent saw %d frames, want 2", len(frames))
			}
			if a, b := withoutID(t, typedFrame), withoutID(t, frames[1]); !bytes.Equal(a, b) {
				t.Fatalf("envelope differs:\n typed %s\n Call  %s", a, b)
			}

			var id struct {
				ID string `json:"id"`
			}
			if err := json.Unmarshal(frames[1], &id); err != nil {
				t.Fatal(err)
			}
			agent.mu.Lock()
			sent := agent.replies[id.ID]
			agent.mu.Unlock()
			want, err := slotPayload(sent, route.Slot)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(reply, want) {
				t.Fatalf("Call returned %s, the agent sent %s", reply, want)
			}

			got := routeTimeout(route, func(msg *WSMessage) {
				var decoded WSMessage
				if err := json.Unmarshal(frames[1], &decoded); err != nil {
					t.Fatal(err)
				}
				*msg = decoded
			})
			if got != tc.budget {
				t.Fatalf("Call budget %s, typed %s", got, tc.budget)
			}

			failing, _ := dialFamilyAgent(t, "2026.10.01", true)
			typedErr := tc.typed(ctx, failing)
			_, callErr := failing.Call(ctx, "env-1", tc.msgType, slot, nil)
			// Detection, link inventory and the image list never read the agent's success
			// flag on the typed path, so they must not start refusing through Call either.
			if (typedErr == nil) != (callErr == nil) || (typedErr != nil && typedErr.Error() != callErr.Error()) {
				t.Fatalf("refusal wording differs:\n typed %v\n Call  %v", typedErr, callErr)
			}
		})
	}
}

// TestCallCoversTheWholeTypedFamily keeps the table above honest: every compute route in the
// ceiling that is not a guest command is exercised by it.
func TestCallCoversTheWholeTypedFamily(t *testing.T) {
	covered := map[string]bool{}
	for _, tc := range typedCases() {
		covered[tc.msgType] = true
	}
	for _, r := range PluginRoutes() {
		if (r.Slot == SlotCompute || r.Slot == SlotPool || r.Slot == SlotComputeImage ||
			r.Type == MsgNetDetectRequest) && !covered[r.Type] {
			t.Errorf("%s has no typed parity case", r.Type)
		}
	}
}

// TestCallStreamsImageProgressLikeTheTypedFetch holds the one streamed typed call to Call:
// the same progress frames arrive, as the same payloads, in order.
func TestCallStreamsImageProgressLikeTheTypedFetch(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	tr, agent := dialFamilyAgent(t, "2026.09.01", false)

	var typed []string
	if _, err := tr.ImageFetch(ctx, typedFetch, func(p *ComputeImagePayload) {
		data, err := json.Marshal(p)
		if err != nil {
			t.Error(err)
		}
		typed = append(typed, MsgImageFetchProgress+" "+string(data))
	}); err != nil {
		t.Fatal(err)
	}
	slot := slotOf(t, agent.raw()[0], SlotComputeImage)

	var mu sync.Mutex
	var routed []string
	if _, err := tr.Call(ctx, "env-1", MsgImageFetchRequest, slot, func(msgType string, frame []byte) {
		mu.Lock()
		routed = append(routed, msgType+" "+string(frame))
		mu.Unlock()
	}); err != nil {
		t.Fatal(err)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(typed) != 2 || strings.Join(typed, "\n") != strings.Join(routed, "\n") {
		t.Fatalf("progress differs:\n typed %q\n Call  %q", typed, routed)
	}
}

// TestCallCancelsAnAbandonedImageFetch holds Call to the typed fetch's cancellation: when the
// caller gives up, the agent is told to stop, under the fetch's own id, with the same frame.
func TestCallCancelsAnAbandonedImageFetch(t *testing.T) {
	hang := typedFetch
	hang.ImageID = "hang"
	cancelFrame := func(run func(ctx context.Context, tr *AgentTransport) error) []byte {
		tr, agent := dialFamilyAgent(t, "2026.09.01", false)
		ctx, cancel := context.WithTimeout(context.Background(), 150*time.Millisecond)
		defer cancel()
		if err := run(ctx, tr); !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("abandoned fetch returned %v", err)
		}
		deadline := time.Now().Add(3 * time.Second)
		for time.Now().Before(deadline) {
			frames := agent.raw()
			if len(frames) == 2 {
				var fetch, stop struct {
					ID   string `json:"id"`
					Type string `json:"type"`
				}
				if json.Unmarshal(frames[0], &fetch) != nil || json.Unmarshal(frames[1], &stop) != nil {
					t.Fatal("unreadable frame")
				}
				if stop.Type != MsgImageCancelRequest || stop.ID != fetch.ID {
					t.Fatalf("second frame = %s, want a cancel under the fetch's id %s", frames[1], fetch.ID)
				}
				return withoutID(t, frames[1])
			}
			time.Sleep(10 * time.Millisecond)
		}
		t.Fatal("no cancel frame reached the agent")
		return nil
	}
	typed := cancelFrame(func(ctx context.Context, tr *AgentTransport) error {
		_, err := tr.ImageFetch(ctx, hang, nil)
		return err
	})
	payload, err := json.Marshal(hang)
	if err != nil {
		t.Fatal(err)
	}
	routed := cancelFrame(func(ctx context.Context, tr *AgentTransport) error {
		_, err := tr.Call(ctx, "env-1", MsgImageFetchRequest, payload, nil)
		return err
	})
	if !bytes.Equal(typed, routed) {
		t.Fatalf("cancel frame differs:\n typed %s\n Call  %s", typed, routed)
	}
}

// TestCallRefusesALibraryFetchLikeTheTypedFetch holds the image-library floor: the same
// refusal on an old agent, and nothing written to it on either path.
func TestCallRefusesALibraryFetchLikeTheTypedFetch(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	tr, agent := dialFamilyAgent(t, "3.1.0", false)
	library := typedFetch
	library.FromLibrary = true
	_, typedErr := tr.ImageFetch(ctx, library, nil)
	payload, err := json.Marshal(library)
	if err != nil {
		t.Fatal(err)
	}
	_, callErr := tr.Call(ctx, "env-1", MsgImageFetchRequest, payload, nil)
	if typedErr == nil || callErr == nil || typedErr.Error() != callErr.Error() || !errors.Is(callErr, ErrComputeUnsupported) {
		t.Fatalf("library floor differs:\n typed %v\n Call  %v", typedErr, callErr)
	}
	if n := len(agent.raw()); n != 0 {
		t.Fatalf("a refused fetch wrote %d frames", n)
	}
}

// TestCallStripsWhatTheTypedCallsNeverSend keeps the host-built envelope tight on the new
// slots: reply fields and the rootfs-import fields never reach the wire, the operation is the
// host's, and a pool file operation outside the typed set is refused before anything is sent.
func TestCallStripsWhatTheTypedCallsNeverSend(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	tr, agent := dialFamilyAgent(t, "2026.09.01", false)

	img := filled[ComputeImagePayload]()
	img.FromLibrary = false
	payload, err := json.Marshal(img)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tr.Call(ctx, "env-1", MsgImageListRequest, payload, nil); err != nil {
		t.Fatal(err)
	}
	pool := filled[ComputePoolPayload]()
	pool.Operation = "chmod"
	poolJSON, err := json.Marshal(pool)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tr.Call(ctx, "env-1", MsgPoolFileRequest, poolJSON, nil); !errors.Is(err, ErrNotPluginRoutable) {
		t.Fatalf("pool file operation chmod: got %v", err)
	}
	if _, err := tr.Call(ctx, "env-1", MsgPoolStatsRequest, poolJSON, nil); err != nil {
		t.Fatal(err)
	}

	frames := agent.raw()
	if len(frames) != 2 {
		t.Fatalf("agent saw %d frames, want 2 (the refused pool file must not be written)", len(frames))
	}
	var sentImg map[string]json.RawMessage
	if err := json.Unmarshal(slotOf(t, frames[0], SlotComputeImage), &sentImg); err != nil {
		t.Fatal(err)
	}
	for _, field := range []string{"cached", "success", "error", "bytesDone", "percent", "phase", "poolId", "localPath", "targetPath", "verified"} {
		if _, ok := sentImg[field]; ok {
			t.Errorf("image field %q reached the wire", field)
		}
	}
	if string(sentImg["operation"]) != `"list"` || string(sentImg["envId"]) != `"env-1"` {
		t.Errorf("operation/envId on the wire = %s / %s", sentImg["operation"], sentImg["envId"])
	}
	var sentPool map[string]json.RawMessage
	if err := json.Unmarshal(slotOf(t, frames[1], SlotPool), &sentPool); err != nil {
		t.Fatal(err)
	}
	for _, field := range []string{"detected", "stats", "entries", "success", "error"} {
		if _, ok := sentPool[field]; ok {
			t.Errorf("pool field %q reached the wire", field)
		}
	}
	if string(sentPool["operation"]) != `"stats"` {
		t.Errorf("pool operation on the wire = %s, want the host's stats", sentPool["operation"])
	}
}
