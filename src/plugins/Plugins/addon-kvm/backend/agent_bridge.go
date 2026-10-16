// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"
)

// agentRPC is the host's agent-rpc bridge as this plugin sees it. Every type in it is
// builtin, so the plugin declares it here and asserts the host handle against it rather
// than linking a host interface whose change would stop the plugin loading.
type agentRPC interface {
	// Available reports whether envID can take agent calls: "not_agent_environment",
	// "agent_offline" or "agent_too_old" when it cannot.
	Available(ctx context.Context, envID string) (bool, string)
	// Call sends one message declared under agent-rpc send and returns the reply slot as JSON.
	Call(ctx context.Context, envID, msgType string, payload []byte, onProgress func(msgType string, frame []byte)) ([]byte, error)
	// Subscribe replaces this plugin's sink for the frames declared under agent-rpc observe.
	Subscribe(sink func(envID, msgType string, frame []byte))
}

// guestKindVM marks the events this plugin applies; the lease and container frames that
// carried another kind are not observed here at all.
const guestKindVM = "vm"

// GuestEvent is one unsolicited vm_event frame, with the timestamp the host used to resolve.
type GuestEvent struct {
	EnvID     string
	GuestKind string
	GuestID   string
	Kind      string
	Device    string
	Detail    map[string]string
	At        time.Time
}

const (
	// importProgressLimit bounds the table. Imports run one or two at a time, so reaching
	// it means entries are leaking, and the oldest goes first.
	importProgressLimit = 256
	// importProgressTTL drops an entry whose import went quiet. A call clears its own entry
	// on the way out, but a node that only holds the agent for another node's call never
	// sees that return, so age is the only thing that ends it.
	importProgressTTL = 15 * time.Minute
)

// importProgressEntry is the last vm_import_progress frame for one import.
type importProgressEntry struct {
	phase   string
	disk    int
	percent int
	at      time.Time
}

// agentBridge is everything this plugin asks of an agent, built on the host's generic bridge:
// it probes, carries guest commands, converts observed frames into guest events, and keeps the
// import progress the host used to keep for it.
type agentBridge struct {
	rpc agentRPC

	importMu sync.Mutex
	imports  map[string]importProgressEntry
}

// newAgentBridge asserts the host's handle. It returns nil when the host wired none, or one
// too old to carry agent-rpc, which every caller treats as "no host can run guests".
func newAgentBridge(handle any) *agentBridge {
	rpc, ok := handle.(agentRPC)
	if !ok || rpc == nil {
		return nil
	}
	return &agentBridge{rpc: rpc}
}

// Available reports whether envID can host guests, and which precondition is missing.
func (b *agentBridge) Available(ctx context.Context, envID string) (bool, string) {
	return b.rpc.Available(ctx, envID)
}

// probedCapability is the part of a capability report this plugin reads.
type probedCapability struct {
	CanRunVMs    bool
	QEMUBinaries []probedQEMU
}

type probedQEMU struct {
	MachineTypes []string
	KVMAccel     bool
}

// Probe asks the agent what its host can run.
func (b *agentBridge) Probe(ctx context.Context, envID string) (probedCapability, error) {
	reply, err := b.rpc.Call(ctx, envID, msgComputeProbe, nil, nil)
	if err != nil {
		return probedCapability{}, err
	}
	var payload agentComputePayload
	if err := json.Unmarshal(reply, &payload); err != nil {
		return probedCapability{}, fmt.Errorf("decoding the %s reply: %w", msgComputeProbe, err)
	}
	if payload.Capability == nil {
		return probedCapability{}, errors.New("agent returned no capability report")
	}
	return convertCapability(payload.Capability), nil
}

// convertCapability keeps the two facts guest creation reads from the agent's report.
func convertCapability(in *agentCapability) probedCapability {
	out := probedCapability{CanRunVMs: in.CanRunVMs}
	for _, binary := range in.QEMUBinaries {
		out.QEMUBinaries = append(out.QEMUBinaries, probedQEMU(binary))
	}
	return out
}

// send carries one guest command and returns the reply slot. An import command clears that
// import's progress before and after, so a retry cannot start from the last run's percentage.
func (b *agentBridge) send(ctx context.Context, envID, msgType string, payload []byte) ([]byte, error) {
	if isImportCommand(msgType) {
		if importID := importIDOf(payload); importID != "" {
			b.clearImportProgress(envID, importID)
			defer b.clearImportProgress(envID, importID)
		}
	}
	return b.rpc.Call(ctx, envID, msgType, payload, nil)
}

// isImportCommand names the commands that move a whole disk image across the transport.
func isImportCommand(msgType string) bool {
	return msgType == msgVMImportInspect || msgType == msgVMImportConvert || msgType == msgVMImportStage
}

// importIDOf reads the import id out of a vm payload.
func importIDOf(payload []byte) string {
	var probe agentVMFrame
	if len(payload) == 0 || json.Unmarshal(payload, &probe) != nil || probe.Import == nil {
		return ""
	}
	return probe.Import.ImportID
}

// Subscribe routes the frames this plugin observes: guest events to onEvent, import progress
// into the table ImportProgress reads. The host delivers them from this plugin's own queue.
func (b *agentBridge) Subscribe(onEvent func(GuestEvent)) {
	b.rpc.Subscribe(func(envID, msgType string, frame []byte) {
		switch msgType {
		case msgVMEvent:
			if event, ok := guestEvent(envID, frame, time.Now()); ok && onEvent != nil {
				onEvent(event)
			}
		case msgVMImportProgress:
			b.recordImportProgress(envID, frame, time.Now())
		}
	})
}

// Unsubscribe stops frames reaching this plugin once it shuts down.
func (b *agentBridge) Unsubscribe() {
	b.rpc.Subscribe(nil)
}

// guestEvent turns a vm_event frame into a guest event. A missing or malformed timestamp
// resolves to now, as the host's conversion always did.
func guestEvent(envID string, frame []byte, now time.Time) (GuestEvent, bool) {
	var payload agentVMFrame
	if json.Unmarshal(frame, &payload) != nil || payload.Event == nil {
		return GuestEvent{}, false
	}
	event := payload.Event
	at := now
	if event.Timestamp != "" {
		if parsed, err := time.Parse(time.RFC3339, event.Timestamp); err == nil {
			at = parsed
		}
	}
	return GuestEvent{
		EnvID:     envID,
		GuestKind: guestKindVM,
		GuestID:   event.VMID,
		Kind:      event.Kind,
		Device:    event.Device,
		Detail:    event.Detail,
		At:        at,
	}, true
}

// importPhase names what a progress frame measures. The agent labels its pull frames with
// vm_import_inspect whichever call is pulling, because a pull is the first half of every
// operation that needs the file.
func importPhase(operation string) string {
	switch operation {
	case msgVMImportInspect, msgVMImportStage:
		return importPhasePull
	case msgVMImportConvert:
		return importPhaseConvert
	default:
		return ""
	}
}

// recordImportProgress keeps the last vm_import_progress frame per import. It is recorded
// rather than forwarded because the plugin is blocked on the import call and polls instead.
func (b *agentBridge) recordImportProgress(envID string, frame []byte, now time.Time) {
	var payload agentVMFrame
	if json.Unmarshal(frame, &payload) != nil {
		return
	}
	importID := ""
	if payload.Import != nil {
		importID = payload.Import.ImportID
	}
	if importID == "" && payload.ImportResult != nil {
		importID = payload.ImportResult.ImportID
	}
	phase := importPhase(payload.Operation)
	if importID == "" || phase == "" {
		return
	}
	disk := 0
	if payload.ImportResult != nil && len(payload.ImportResult.Disks) > 0 {
		disk = payload.ImportResult.Disks[0].Index
	}
	percent := min(max(payload.Percent, 0), 100)

	key := importProgressKey(envID, importID)
	b.importMu.Lock()
	defer b.importMu.Unlock()
	if b.imports == nil {
		b.imports = make(map[string]importProgressEntry)
	}
	for k, entry := range b.imports {
		if now.Sub(entry.at) > importProgressTTL {
			delete(b.imports, k)
		}
	}
	if _, known := b.imports[key]; !known && len(b.imports) >= importProgressLimit {
		b.evictOldestImportLocked()
	}
	b.imports[key] = importProgressEntry{phase: phase, disk: disk, percent: percent, at: now}
}

// ImportProgress returns the phase ("pull" or "convert"), the index of the disk being
// converted and its percentage; ok is false when nothing is known.
func (b *agentBridge) ImportProgress(envID, importID string) (phase string, disk, percent int, ok bool) {
	b.importMu.Lock()
	defer b.importMu.Unlock()
	entry, found := b.imports[importProgressKey(envID, importID)]
	if !found || time.Since(entry.at) > importProgressTTL {
		return "", 0, 0, false
	}
	return entry.phase, entry.disk, entry.percent, true
}

func (b *agentBridge) clearImportProgress(envID, importID string) {
	b.importMu.Lock()
	delete(b.imports, importProgressKey(envID, importID))
	b.importMu.Unlock()
}

func (b *agentBridge) evictOldestImportLocked() {
	oldestKey := ""
	var oldest time.Time
	for key, entry := range b.imports {
		if oldestKey == "" || entry.at.Before(oldest) {
			oldestKey, oldest = key, entry.at
		}
	}
	delete(b.imports, oldestKey)
}

func importProgressKey(envID, importID string) string {
	return envID + "\x00" + importID
}
