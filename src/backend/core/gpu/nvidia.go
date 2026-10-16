// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// Package gpu reads GPU telemetry from the host McHarbor runs on.
//
// # Why nvidia-smi and not a library
//
// Docker's stats API carries no GPU data at all — there is no field to
// read, on any API version. NVML would be the richer source but needs
// cgo and links against the host driver, which this project cannot do
// (pure-Go, no CGO, `modernc.org/sqlite`). That leaves the CLI.
//
// # Why the binary is not in the image
//
// `nvidia-smi` is deliberately NOT installed by the Dockerfile. When a
// container is started with GPU access, the NVIDIA container runtime
// bind-mounts the host's own `nvidia-smi` and its driver libraries into
// the container. Shipping our own copy would be a version skew waiting
// to happen: the binary must match the host driver, and only the host
// knows which that is.
//
// The practical consequence is that GPU telemetry appears only when the
// operator has given the McHarbor container GPU access:
//
//	deploy:
//	  resources:
//	    reservations:
//	      devices:
//	        - capabilities: [gpu]
//
// Without it `Available` reports false and every caller degrades to "no
// GPUs" — never an error, because a host with no GPU is the normal case,
// not a fault.
//
// # Scope
//
// This reads the GPUs of the host the McHarbor *process* runs on. It is
// not per-environment: a remote Docker host's GPUs are not visible here.
// Callers must not present this as telemetry for an arbitrary
// environment.
package gpu

import (
	"context"
	"errors"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"time"
)

// queryTimeout bounds one nvidia-smi invocation. The command is local
// and normally answers in tens of milliseconds; a driver in a bad state
// can hang it indefinitely, and a metrics collector must not inherit
// that.
const queryTimeout = 5 * time.Second

// availabilityTTL caches the "is nvidia-smi present" answer. The lookup
// itself is cheap, but the probe runs on every metrics tick and every
// host-info request, and the answer changes only when the container is
// recreated.
const availabilityTTL = time.Minute

// Device is one GPU as reported by the driver.
type Device struct {
	Index int    `json:"index"`
	Name  string `json:"name"`
	// Utilization is the percentage of time the GPU was busy over the
	// driver's own sampling window, 0-100.
	Utilization float64 `json:"utilization"`
	MemoryUsed  int64   `json:"memoryUsed"`
	MemoryTotal int64   `json:"memoryTotal"`
	// Temperature is degrees Celsius. Zero when the driver does not
	// report one, which some virtualised GPUs do not.
	Temperature float64 `json:"temperature"`
}

// MemoryPercent is memory used as a percentage of total, or 0 when the
// driver reported no total.
func (d Device) MemoryPercent() float64 {
	if d.MemoryTotal <= 0 {
		return 0
	}
	return float64(d.MemoryUsed) / float64(d.MemoryTotal) * 100
}

// Reader queries the local GPUs.
//
// Safe for concurrent use: the only mutable state is the availability
// cache, guarded by a mutex.
type Reader struct {
	mu          sync.Mutex
	cachedAt    time.Time
	cachedPath  string
	cachedFound bool
}

// NewReader builds a Reader.
func NewReader() *Reader { return &Reader{} }

// ErrUnavailable is returned when nvidia-smi is not present. Callers
// should treat it as "this host has no GPU telemetry", not as a failure
// worth surfacing as an error.
var ErrUnavailable = errors.New("gpu: nvidia-smi is not available in this container")

// binaryPath returns the cached path to nvidia-smi.
func (r *Reader) binaryPath() (string, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if time.Since(r.cachedAt) < availabilityTTL && !r.cachedAt.IsZero() {
		return r.cachedPath, r.cachedFound
	}
	path, err := exec.LookPath("nvidia-smi")
	r.cachedAt = time.Now()
	r.cachedPath = path
	r.cachedFound = err == nil
	return r.cachedPath, r.cachedFound
}

// Available reports whether GPU telemetry can be read at all. Cheap
// enough to call on every render.
func (r *Reader) Available() bool {
	_, ok := r.binaryPath()
	return ok
}

// Devices returns one entry per visible GPU.
//
// Returns (nil, ErrUnavailable) when nvidia-smi is absent. An empty
// slice with a nil error means the binary ran and reported no GPUs,
// which is a different state worth distinguishing: the first says the
// container has no GPU access, the second says it has access to nothing.
func (r *Reader) Devices(ctx context.Context) ([]Device, error) {
	path, ok := r.binaryPath()
	if !ok {
		return nil, ErrUnavailable
	}

	ctx, cancel := context.WithTimeout(ctx, queryTimeout)
	defer cancel()

	// `--format=csv,noheader,nounits` keeps the output parseable: no
	// header row to skip, and bare numbers rather than "72 %" / "8192
	// MiB" that would need unit stripping per field.
	cmd := exec.CommandContext(ctx, path,
		"--query-gpu=index,name,utilization.gpu,memory.used,memory.total,temperature.gpu",
		"--format=csv,noheader,nounits")

	out, err := cmd.Output()
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			return nil, errors.New("gpu: nvidia-smi exited with an error")
		}
		return nil, errors.New("gpu: running nvidia-smi failed")
	}

	var devices []Device
	for _, line := range strings.Split(string(out), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		device, ok := parseDeviceLine(line)
		if !ok {
			// One malformed row must not discard the others. A mixed
			// fleet can include a GPU whose driver omits a field, and
			// reporting the rest is better than reporting nothing.
			continue
		}
		devices = append(devices, device)
	}
	if devices == nil {
		devices = []Device{}
	}
	return devices, nil
}

// parseDeviceLine parses one CSV row. Returns ok=false when the row does
// not have the six expected fields or the index cannot be read — the two
// cases where the row identifies nothing usable.
func parseDeviceLine(line string) (Device, bool) {
	parts := strings.Split(line, ",")
	if len(parts) < 6 {
		return Device{}, false
	}
	for i := range parts {
		parts[i] = strings.TrimSpace(parts[i])
	}

	index, err := strconv.Atoi(parts[0])
	if err != nil {
		return Device{}, false
	}

	// Everything after the index degrades to a zero value rather than
	// failing the row. "[N/A]" is what nvidia-smi prints for a metric a
	// particular card does not expose, and a card with no temperature
	// sensor is still a card worth listing.
	return Device{
		Index:       index,
		Name:        parts[1],
		Utilization: parseFloat(parts[2]),
		MemoryUsed:  parseMiB(parts[3]),
		MemoryTotal: parseMiB(parts[4]),
		Temperature: parseFloat(parts[5]),
	}, true
}

func parseFloat(raw string) float64 {
	v, err := strconv.ParseFloat(raw, 64)
	if err != nil {
		return 0
	}
	return v
}

// parseMiB converts nvidia-smi's mebibyte figure to bytes, so the value
// matches every other memory field in the API.
func parseMiB(raw string) int64 {
	v, err := strconv.ParseFloat(raw, 64)
	if err != nil {
		return 0
	}
	return int64(v * 1024 * 1024)
}
