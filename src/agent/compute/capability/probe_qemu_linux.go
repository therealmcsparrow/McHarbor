// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

//go:build linux

package capability

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/therealmcsparrow/mcharbor-agent/internal/errlog"
)

// deviceNameRE pulls the model out of a `-device help` line, which QEMU writes
// as `name "virtio-mem-pci", bus PCI, desc "..."`.
var deviceNameRE = regexp.MustCompile(`^name\s+"([a-zA-Z0-9._-]+)"`)

// qemuDevices lists the device models this binary can emit.
//
// It is the cheap half of the capability gate: virtio-mem, vhost-user-fs and
// vhost-vsock are all build-time options, so a host whose QEMU lacks one has to
// show the feature unavailable rather than fail the guest's start.
func qemuDevices(ctx context.Context, path string) []string {
	out, err := runBounded(ctx, path, "-device", "help")
	if err != nil && out == "" {
		return nil
	}
	var devices []string
	seen := map[string]bool{}
	sc := bufio.NewScanner(strings.NewReader(out))
	for sc.Scan() {
		m := deviceNameRE.FindStringSubmatch(sc.Text())
		if len(m) != 2 || seen[m[1]] {
			continue
		}
		seen[m[1]] = true
		devices = append(devices, m[1])
	}
	sort.Strings(devices)
	return devices
}

// cpuModelRE matches a `-cpu help` model line: an optional target prefix, then
// the model name. Leading whitespace is significant and handled by the caller.
var cpuModelRE = regexp.MustCompile(`^(?:x86|arm)?\s+([a-zA-Z0-9._+-]+)`)

// qemuCPUModels lists the named CPU models this binary offers.
//
// A guest on `-cpu host` cannot migrate, and the picker that says so needs the
// real list: what a binary offers depends on both its version and the physical
// CPU underneath it, so a hardcoded list would be wrong on half the hosts.
func qemuCPUModels(ctx context.Context, path string) []string {
	out, err := runBounded(ctx, path, "-cpu", "help")
	if err != nil && out == "" {
		return nil
	}
	var models []string
	seen := map[string]bool{}
	sc := bufio.NewScanner(strings.NewReader(out))
	for sc.Scan() {
		line := sc.Text()
		if line == "" {
			continue
		}
		// The CPUID flag listing that follows the models is unindented and
		// space-separated; stopping there keeps flag names out of the model
		// list, where they would read as selectable models.
		if !strings.HasPrefix(line, " ") && !strings.HasPrefix(line, "x86") &&
			!strings.HasPrefix(line, "arm") {
			if len(models) > 0 {
				break
			}
			continue
		}
		m := cpuModelRE.FindStringSubmatch(line)
		if len(m) != 2 || seen[m[1]] {
			continue
		}
		seen[m[1]] = true
		models = append(models, m[1])
	}
	sort.Strings(models)
	return models
}

// qmpSchemaEntry is one member of query-qmp-schema's array. Only commands are
// read; the type graph beside them describes argument shapes McHarbor does not
// introspect.
type qmpSchemaEntry struct {
	Name string `json:"name"`
	Meta string `json:"meta-type"`
}

// qemuQMPCommands asks a throwaway QEMU which QMP verbs it implements.
//
// It runs `-machine none -accel tcg` so nothing is allocated and no guest
// starts: the schema is a property of the binary, and probing a live guest's
// monitor would make the answer depend on whether one happens to be running.
func qemuQMPCommands(ctx context.Context, path string) []string {
	ctx, cancel := context.WithTimeout(ctx, probeTimeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, path,
		"-machine", "none", "-accel", "tcg", "-display", "none",
		"-nodefaults", "-qmp", "stdio")
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil
	}
	if err := cmd.Start(); err != nil {
		return nil
	}
	defer func() {
		errlog.Close(stdin)
		// The monitor exits on EOF; Kill is the backstop for a build that does
		// not, and Wait reaps either way.
		errlog.Log(cmd.Process.Kill())
		errlog.Log(cmd.Wait())
	}()

	dec := json.NewDecoder(stdout)
	var greeting struct {
		QMP json.RawMessage `json:"QMP"`
	}
	if err := dec.Decode(&greeting); err != nil || len(greeting.QMP) == 0 {
		return nil
	}
	// The capabilities negotiation is mandatory: a command sent before it is
	// refused, so the schema query has to come third.
	if _, err := io.WriteString(stdin, "{\"execute\":\"qmp_capabilities\"}\n"); err != nil {
		return nil
	}
	var ack map[string]json.RawMessage
	if err := dec.Decode(&ack); err != nil {
		return nil
	}
	if _, err := io.WriteString(stdin, "{\"execute\":\"query-qmp-schema\"}\n"); err != nil {
		return nil
	}

	// Events can arrive between a command and its reply, so read on until the
	// message that actually carries a return rather than assuming it is next.
	for i := 0; i < 16; i++ {
		var msg struct {
			Return []qmpSchemaEntry `json:"return"`
			Error  json.RawMessage  `json:"error"`
		}
		if err := dec.Decode(&msg); err != nil {
			return nil
		}
		if len(msg.Error) > 0 {
			return nil
		}
		if msg.Return == nil {
			continue
		}
		var commands []string
		for _, entry := range msg.Return {
			if entry.Meta == "command" {
				commands = append(commands, entry.Name)
			}
		}
		sort.Strings(commands)
		return commands
	}
	return nil
}

// probeHugePages reads the host hugepage pool from /proc/meminfo.
//
// Returns nil where the kernel reports no pool section at all, so the UI has
// one "this host has no hugepages" state to render rather than two.
func probeHugePages() *HugePages {
	f, err := os.Open("/proc/meminfo")
	if err != nil {
		return nil
	}
	defer func() { errlog.Close(f) }()

	out := &HugePages{}
	found := false
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		key, value, ok := strings.Cut(sc.Text(), ":")
		if !ok {
			continue
		}
		n, err := strconv.ParseInt(strings.TrimSuffix(strings.TrimSpace(value), " kB"), 10, 64)
		if err != nil {
			continue
		}
		switch key {
		case "HugePages_Total":
			out.Total, found = n, true
		case "HugePages_Free":
			out.Free = n
		case "HugePages_Rsvd":
			out.Reserved = n
		case "HugePages_Surp":
			out.Surplus = n
		case "Hugepagesize":
			out.SizeKB = n
		}
	}
	if !found {
		return nil
	}
	return out
}

// probeNUMANodes enumerates the host's NUMA nodes.
//
// A single-node host reports none: pinning a guest on a box with one memory
// domain is a setting with no effect, and offering it invites the operator to
// believe it did something.
func probeNUMANodes() []NUMANode {
	const root = "/sys/devices/system/node"
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil
	}
	var nodes []NUMANode
	for _, entry := range entries {
		if !strings.HasPrefix(entry.Name(), "node") {
			continue
		}
		id, err := strconv.Atoi(strings.TrimPrefix(entry.Name(), "node"))
		if err != nil {
			continue
		}
		dir := filepath.Join(root, entry.Name())
		node := NUMANode{ID: id, CPUs: readCPUList(filepath.Join(dir, "cpulist"))}
		node.MemoryBytes, node.FreeBytes = readNodeMeminfo(filepath.Join(dir, "meminfo"))
		nodes = append(nodes, node)
	}
	if len(nodes) < 2 {
		return nil
	}
	sort.Slice(nodes, func(i, j int) bool { return nodes[i].ID < nodes[j].ID })
	return nodes
}

// readCPUList expands a kernel cpulist ("0-3,8,12-15") into explicit ids.
func readCPUList(path string) []int {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var cpus []int
	for _, part := range strings.Split(strings.TrimSpace(string(raw)), ",") {
		if part == "" {
			continue
		}
		lo, hi, ranged := strings.Cut(part, "-")
		start, err := strconv.Atoi(lo)
		if err != nil {
			continue
		}
		end := start
		if ranged {
			if end, err = strconv.Atoi(hi); err != nil {
				continue
			}
		}
		// The bound keeps a malformed range from allocating without limit; a
		// real node never spans anything near it.
		for i := start; i <= end && i-start < 4096; i++ {
			cpus = append(cpus, i)
		}
	}
	return cpus
}

// readNodeMeminfo returns a node's total and free memory in bytes.
func readNodeMeminfo(path string) (int64, int64) {
	f, err := os.Open(path)
	if err != nil {
		return 0, 0
	}
	defer func() { errlog.Close(f) }()

	var total, free int64
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		// "Node 0 MemTotal:       16384000 kB"
		fields := strings.Fields(sc.Text())
		if len(fields) < 4 {
			continue
		}
		n, err := strconv.ParseInt(fields[3], 10, 64)
		if err != nil {
			continue
		}
		switch fields[2] {
		case "MemTotal:":
			total = n * 1024
		case "MemFree:":
			free = n * 1024
		}
	}
	return total, free
}

// qemuSPICEUsable reports whether path really starts with a SPICE server.
//
// The QMP schema cannot answer this. Debian and Ubuntu build QEMU modular:
// query-spice is compiled into the core binary as a stub, while the -spice
// option only exists once qemu-system-modules-spice is installed. Trusting the
// schema there emits -spice and QEMU exits before opening its monitor, so the
// probe starts the binary the way a guest start would and waits for QMP.
func qemuSPICEUsable(ctx context.Context, path string) bool {
	ctx, cancel := context.WithTimeout(ctx, probeTimeout)
	defer cancel()

	dir, err := os.MkdirTemp("", "mcharbor-spice-probe-")
	if err != nil {
		return false
	}
	defer func() { errlog.Log(os.RemoveAll(dir)) }()

	cmd := exec.CommandContext(ctx, path,
		"-machine", "none", "-accel", "tcg", "-display", "none", "-nodefaults",
		"-spice", "unix=on,addr="+filepath.Join(dir, "spice.sock")+",disable-ticketing=on",
		"-qmp", "stdio")
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return false
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return false
	}
	if err := cmd.Start(); err != nil {
		return false
	}
	defer func() {
		errlog.Close(stdin)
		errlog.Log(cmd.Process.Kill())
		errlog.Log(cmd.Wait())
	}()

	var greeting struct {
		QMP json.RawMessage `json:"QMP"`
	}
	return json.NewDecoder(stdout).Decode(&greeting) == nil && len(greeting.QMP) > 0
}

// withoutCommand returns commands minus name, for a verb the schema lists but
// the binary cannot actually serve.
func withoutCommand(commands []string, name string) []string {
	out := make([]string, 0, len(commands))
	for _, c := range commands {
		if c != name {
			out = append(out, c)
		}
	}
	return out
}
