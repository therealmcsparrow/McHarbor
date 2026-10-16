// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package computesim

import (
	"context"
	"errors"
	"fmt"
	"math"
	"strings"
	"time"

	coreagent "github.com/therealmcsparrow/mcharbor/core/agent"
	"github.com/therealmcsparrow/mcharbor/core/demo/estate"
	"github.com/therealmcsparrow/mcharbor/core/demo/fakeagent"
)

var (
	errFrozen    = errors.New("the container is frozen; unfreeze it instead")
	errNotFrozen = errors.New("the container is not frozen")
)

// ctTransition applies one lifecycle operation to a system container.
func ctTransition(c *ct, op string, now time.Time) error {
	running := c.State == stateRunning
	frozen := c.State == stateFrozen
	start := func() {
		c.State, c.LastError, c.ExitReason, c.StartedAt = stateRunning, "", "", now
		c.InitPID = 10000 + int(estate.StableHash(c.ID+now.String())%400000)
	}
	switch op {
	case coreagent.MsgCTStart:
		switch {
		case running:
			return nil
		case frozen:
			return errFrozen
		}
		start()
		return nil
	case coreagent.MsgCTShutdown, coreagent.MsgCTStop:
		if running || frozen {
			c.State, c.StartedAt, c.InitPID = stateStopped, time.Time{}, 0
			c.ExitReason = "shutdown"
			if op == coreagent.MsgCTStop {
				c.ExitReason = "killed"
			}
		}
		return nil
	case coreagent.MsgCTRestart:
		if frozen {
			return errFrozen
		}
		start()
		return nil
	case coreagent.MsgCTFreeze:
		switch {
		case frozen:
			return nil
		case !running:
			return errNotRunning
		}
		c.State = stateFrozen
		return nil
	case coreagent.MsgCTUnfreeze:
		switch {
		case running:
			return nil
		case !frozen:
			return errNotFrozen
		}
		c.State = stateRunning
		return nil
	}
	return fmt.Errorf("unknown lifecycle operation %s", op)
}

// ctSnapshotCreate records a container snapshot; the lxc plugin names it by the
// row id it is about to write, so that id is kept.
func ctSnapshotCreate(c *ct, id, name string, now time.Time) (*snapshot, error) {
	if name == "" {
		return nil, errors.New("the snapshot carried no name")
	}
	for _, snap := range c.Snapshots {
		if snap.Name == name || (id != "" && snap.ID == id) {
			return nil, fmt.Errorf("a snapshot named %q already exists", name)
		}
	}
	if id == "" {
		id = stableID(0, now, "snap", c.ID, name)
	}
	parent := ""
	if n := len(c.Snapshots); n > 0 {
		parent = c.Snapshots[n-1].ID
	}
	snap := &snapshot{ID: id, ParentID: parent, Name: name, Handle: c.RootfsPath + "@" + name, CreatedAt: now,
		SizeBytes: int64(len(c.Snapshots)+1) * 256 << 20, savedState: c.State}
	c.Snapshots = append(c.Snapshots, snap)
	return snap, nil
}

// ctSnapshotRevert rolls the rootfs back and discards every later snapshot,
// the same pruning the lxc plugin applies to its rows.
func ctSnapshotRevert(c *ct, id string) error {
	if c.State == stateRunning || c.State == stateFrozen {
		return errors.New("stop the container before reverting it")
	}
	for i, snap := range c.Snapshots {
		if snap.ID == id {
			c.Snapshots = c.Snapshots[:i+1]
			return nil
		}
	}
	return nil
}

func ctSnapshotDelete(c *ct, id string) {
	kept := c.Snapshots[:0]
	var removed *snapshot
	for _, snap := range c.Snapshots {
		if snap.ID == id {
			removed = snap
			continue
		}
		kept = append(kept, snap)
	}
	c.Snapshots = kept
	if removed == nil {
		return
	}
	for _, snap := range c.Snapshots {
		if snap.ParentID == removed.ID {
			snap.ParentID = removed.ParentID
		}
	}
}

func ctState(c *ct) coreagent.CTState {
	st := coreagent.CTState{ContainerID: c.ID, State: c.State, UnitName: "mcharbor-ct-" + c.ID, ExitReason: c.ExitReason, LastError: c.LastError}
	switch c.State {
	case stateRunning, stateFrozen:
		st.UnitActive, st.UnitSub, st.InitPID = "active", "running", c.InitPID
		st.StartedAt = c.StartedAt.UTC().Format(time.RFC3339)
		for _, n := range c.NICs {
			if n.IP != "" {
				st.IPs = append(st.IPs, n.IP)
			}
		}
	case stateCrashed:
		st.UnitActive, st.UnitSub = "failed", "failed"
	default:
		st.UnitActive, st.UnitSub = "inactive", "dead"
	}
	return st
}

func (s *Sim) handleCT(ctx context.Context, host *estate.Host, msg *coreagent.WSMessage, send fakeagent.Sender) {
	req := msg.CT
	if req == nil {
		req = &coreagent.CTPayload{}
	}
	reply := &coreagent.CTPayload{Operation: msg.Type, EnvID: req.EnvID, ContainerID: req.ContainerID}
	out := &coreagent.WSMessage{Type: coreagent.MsgCTResult, ID: msg.ID, CT: reply}
	switch msg.Type {
	case coreagent.MsgCTBackupRun, coreagent.MsgCTBackupRestore:
		out.Type = coreagent.MsgCTBackupResult
		reply.Error, reply.ErrorCode = ErrDemoRefused, "demo_mode"
		s.deliver(send, out)
		return
	case coreagent.MsgCTCloneRequest:
		s.startCTClone(ctx, host, msg, req, send)
		return
	}
	if msg.Type == coreagent.MsgCTQuery && req.Operation != "" {
		reply.Operation = req.Operation
	}
	s.mu.Lock()
	hs := s.hostFor(host)
	var err error
	if hs == nil {
		err = errors.New("this demo host runs no guests")
	} else {
		err = s.applyCT(hs, msg.Type, req, reply)
	}
	s.mu.Unlock()
	if err != nil {
		reply.Error = err.Error()
	} else {
		reply.Success = true
	}
	s.deliver(send, out)
}

// ctFor returns the container id names, adopting one the simulator has not seen.
func (s *Sim) ctFor(hs *hostSim, id string, spec *coreagent.CTSpec) *ct {
	c, ok := hs.cts[id]
	if !ok {
		c = &ct{ID: id, Name: "ct-" + id, State: stateStopped, CreatedAt: s.now(), OS: ctOSes["debian"],
			Role: ctRoles["nginx"], CPUs: 1, MemoryMax: 512 << 20, PidsMax: 2048, load: 0.05}
		hs.cts[id] = c
		hs.ctOrder = append(hs.ctOrder, id)
	}
	if spec != nil {
		applyCTSpec(c, spec)
	}
	return c
}

func applyCTSpec(c *ct, spec *coreagent.CTSpec) {
	if spec.Name != "" {
		c.Name = spec.Name
	}
	if spec.RootfsPath != "" {
		c.RootfsPath = spec.RootfsPath
	}
	if spec.PoolID != "" {
		c.PoolID = spec.PoolID
	}
	c.Privileged, c.Nesting = spec.Privileged, spec.Nesting
	if spec.Limits != nil {
		applyLimits(c, spec.Limits)
	}
	if len(spec.NICs) > 0 {
		nics := make([]*nic, 0, len(spec.NICs))
		for _, n := range spec.NICs {
			ip := n.StaticIP
			if i := strings.Index(ip, "/"); i > 0 {
				ip = ip[:i]
			}
			if ip == "" {
				ip = ipFromMAC(n.MAC)
			}
			nics = append(nics, &nic{ID: n.NICID, NetworkID: n.NetworkID, Bridge: n.Bridge, Slot: n.Slot, MAC: n.MAC, IP: ip})
		}
		c.NICs = nics
	}
}

func applyLimits(c *ct, l *coreagent.CTLimits) {
	if l.MemoryMaxBytes > 0 {
		c.MemoryMax = l.MemoryMaxBytes
	}
	if l.PidsMax > 0 {
		c.PidsMax = l.PidsMax
	}
	if l.DiskQuotaBytes > 0 {
		c.DiskQuota = l.DiskQuotaBytes
	}
	var quota, period int
	if n, err := fmt.Sscanf(l.CPUMax, "%d %d", &quota, &period); err == nil && n == 2 && period > 0 {
		c.CPUs = max(1, quota/period)
	}
}

func limitsOf(c *ct) *coreagent.CTLimits {
	return &coreagent.CTLimits{CPUMax: fmt.Sprintf("%d 100000", c.CPUs*100000), CPUWeight: 100,
		MemoryMaxBytes: c.MemoryMax, MemoryHighBytes: c.MemoryMax / 10 * 9, PidsMax: c.PidsMax, DiskQuotaBytes: c.DiskQuota}
}

// applyCT performs one ct-family command against hs; callers hold s.mu.
func (s *Sim) applyCT(hs *hostSim, op string, req, reply *coreagent.CTPayload) error {
	now := s.now()
	if op == coreagent.MsgCTQuery && req.Operation != "stats" {
		ids := make([]string, 0, len(req.States))
		for _, st := range req.States {
			ids = append(ids, st.ContainerID)
		}
		if req.ContainerID != "" {
			ids = append(ids, req.ContainerID)
		}
		if len(ids) == 0 {
			ids = append(ids, hs.ctOrder...)
		}
		for _, id := range ids {
			reply.States = append(reply.States, ctState(s.ctFor(hs, id, nil)))
		}
		return nil
	}
	if op == coreagent.MsgCTCancel {
		return nil
	}
	if req.ContainerID == "" {
		return errors.New("the command named no container")
	}
	if op == coreagent.MsgCTDelete {
		if _, ok := hs.cts[req.ContainerID]; ok {
			delete(hs.cts, req.ContainerID)
			hs.ctOrder = removeID(hs.ctOrder, req.ContainerID)
		}
		return nil
	}
	c := s.ctFor(hs, req.ContainerID, req.Spec)
	switch op {
	case coreagent.MsgCTCreate:
		reply.SizeBytes = c.DiskQuota / 20
		return nil
	case coreagent.MsgCTStart, coreagent.MsgCTShutdown, coreagent.MsgCTStop, coreagent.MsgCTRestart,
		coreagent.MsgCTFreeze, coreagent.MsgCTUnfreeze:
		if err := ctTransition(c, op, now); err != nil {
			return err
		}
		st := ctState(c)
		reply.State = &st
		return nil
	case coreagent.MsgCTQuery:
		if c.State != stateRunning && c.State != stateFrozen {
			return errNotRunning
		}
		reply.Stats = ctStats(c, now)
		return nil
	case coreagent.MsgCTProcesses:
		if c.State != stateRunning && c.State != stateFrozen {
			return errNotRunning
		}
		reply.Processes = ctProcesses(c)
		return nil
	case coreagent.MsgCTUpdateLimits:
		if req.Limits != nil {
			applyLimits(c, req.Limits)
		}
		reply.Applied = limitsOf(c)
		return nil
	case coreagent.MsgCTSnapshotCreate:
		snap, err := ctSnapshotCreate(c, req.SnapshotID, req.SnapshotName, now)
		if err != nil {
			return err
		}
		reply.SnapshotName, reply.SizeBytes = snap.Handle, snap.SizeBytes
		return nil
	case coreagent.MsgCTSnapshotRevert:
		return ctSnapshotRevert(c, req.SnapshotID)
	case coreagent.MsgCTSnapshotDelete:
		ctSnapshotDelete(c, req.SnapshotID)
		return nil
	}
	return fmt.Errorf("%s is not simulated", op)
}

func ctStats(c *ct, now time.Time) *coreagent.CTStats {
	uptime := math.Max(now.Sub(c.StartedAt).Seconds(), 1)
	cpu := c.load * 100 * (0.5 + wave(c.ID+"cpu", now, 75))
	if c.State == stateFrozen {
		cpu = 0
	}
	mem := int64(float64(c.MemoryMax) * (0.2 + 0.35*wave(c.ID+"mem", now, 500)))
	return &coreagent.CTStats{
		ContainerID: c.ID, CPUPercent: math.Round(cpu*10) / 10, CPUUsageNSec: int64(uptime * c.load * 1e9),
		MemoryBytes: mem, MemoryMaxBytes: c.MemoryMax, MemoryHighBytes: c.MemoryMax / 10 * 9,
		PidsCurrent: int64(12 + len(c.Role.procs)*4), PidsMax: c.PidsMax,
		IOReadBytes: int64(uptime * 20000 * c.load), IOWriteBytes: int64(uptime * 14000 * c.load),
		NetRxBytes: int64(uptime * 18000 * c.load), NetTxBytes: int64(uptime * 9000 * c.load),
		RootfsUsedBytes: c.DiskQuota / 100 * 37, SampledAt: now.Format(time.RFC3339),
	}
}

func ctProcesses(c *ct) []coreagent.CTProcess {
	initCmd := "/sbin/init"
	if c.OS.init == "openrc" {
		initCmd = "/sbin/openrc-init"
	}
	procs := []coreagent.CTProcess{{PID: 1, HostPID: c.InitPID, User: "root", Command: initCmd, MemoryBytes: 9 << 20}}
	if c.OS.init == "systemd" {
		procs = append(procs,
			coreagent.CTProcess{PID: 38, HostPID: c.InitPID + 38, PPID: 1, User: "root", Command: "/lib/systemd/systemd-journald", MemoryBytes: 18 << 20},
			coreagent.CTProcess{PID: 71, HostPID: c.InitPID + 71, PPID: 1, User: "systemd-network", Command: "/lib/systemd/systemd-networkd", MemoryBytes: 7 << 20})
	}
	procs = append(procs, coreagent.CTProcess{PID: 112, HostPID: c.InitPID + 112, PPID: 1, User: "root", Command: "/usr/sbin/sshd -D", MemoryBytes: 6 << 20})
	for i, cmd := range c.Role.procs {
		pid := 200 + i*17
		procs = append(procs, coreagent.CTProcess{PID: pid, HostPID: c.InitPID + pid, PPID: 1, User: "app",
			Command: cmd, CPUPercent: math.Round(c.load*40*10) / 10, MemoryBytes: c.MemoryMax / int64(4+i)})
	}
	return procs
}

// startCTClone copies a container with a few progress frames, off the read loop.
func (s *Sim) startCTClone(ctx context.Context, host *estate.Host, msg *coreagent.WSMessage, req *coreagent.CTPayload, send fakeagent.Sender) {
	go func() {
		reply := &coreagent.CTPayload{Operation: msg.Type, EnvID: req.EnvID, ContainerID: req.ContainerID}
		out := &coreagent.WSMessage{Type: coreagent.MsgCTResult, ID: msg.ID, CT: reply}
		if req.Clone == nil || req.Clone.TargetContainerID == "" {
			reply.Error = "the clone request named no target container"
			s.deliver(send, out)
			return
		}
		for pct := 25; pct < 100; pct += 25 {
			select {
			case <-ctx.Done():
				return
			case <-time.After(120 * time.Millisecond):
			}
			s.deliver(send, &coreagent.WSMessage{Type: coreagent.MsgCTCloneProgress, ID: msg.ID, CT: &coreagent.CTPayload{Operation: "progress", ContainerID: req.Clone.TargetContainerID, Percent: pct}})
		}
		s.mu.Lock()
		hs := s.hostFor(host)
		if hs == nil {
			s.mu.Unlock()
			reply.Error = "this demo host runs no guests"
			s.deliver(send, out)
			return
		}
		source := s.ctFor(hs, req.ContainerID, nil)
		clone := *source
		clone.ID, clone.State, clone.StartedAt, clone.InitPID, clone.LastError, clone.ExitReason = req.Clone.TargetContainerID, stateStopped, time.Time{}, 0, "", ""
		clone.Snapshots, clone.IsTemplate, clone.CreatedAt, clone.NICs = nil, false, s.now(), nil
		clone.Name = source.Name + "-clone"
		clone.RootfsPath = strings.Replace(source.RootfsPath, source.ID, clone.ID, 1)
		if req.Clone.Spec != nil {
			applyCTSpec(&clone, req.Clone.Spec)
		}
		hs.cts[clone.ID] = &clone
		hs.ctOrder = append(hs.ctOrder, clone.ID)
		s.mu.Unlock()
		reply.Success = true
		reply.SizeBytes = clone.DiskQuota / 20
		s.deliver(send, out)
	}()
}
