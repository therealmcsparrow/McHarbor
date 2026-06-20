// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

//go:build linux

package metrics

import (
	"os"
	"strconv"
	"strings"
	"time"

	"golang.org/x/sys/unix"
)

// readHostStats reads CPU%, used memory, and load average from /proc.
// Returns ok=false when the host filesystem does not expose the expected
// files (e.g. non-Linux platforms).
func readHostStats() (cpuPercent, memUsed float64, load1, load5, load15 float64, ok bool) {
	cpuPercent, ok = readCPUPercent()
	if !ok {
		return 0, 0, 0, 0, 0, false
	}
	memUsed, ok = readMemUsed()
	if !ok {
		return 0, 0, 0, 0, 0, false
	}
	load1, load5, load15, ok = readLoadAvg()
	if !ok {
		return 0, 0, 0, 0, 0, false
	}
	return cpuPercent, memUsed, load1, load5, load15, true
}

// readCPUPercent reads /proc/stat twice with a short delay and returns the
// aggregate CPU usage percentage across all cores.
func readCPUPercent() (float64, bool) {
	first, ok := readProcStat()
	if !ok {
		return 0, false
	}
	time.Sleep(200 * time.Millisecond)
	second, ok := readProcStat()
	if !ok {
		return 0, false
	}
	totalDelta := second.total - first.total
	idleDelta := second.idle - first.idle
	if totalDelta <= 0 || idleDelta < 0 {
		return 0, true
	}
	busy := totalDelta - idleDelta
	return float64(busy) / float64(totalDelta) * 100.0, true
}

type procStatSnapshot struct {
	total uint64
	idle  uint64
}

func readProcStat() (procStatSnapshot, bool) {
	data, err := os.ReadFile("/proc/stat")
	if err != nil {
		return procStatSnapshot{}, false
	}
	lines := strings.Split(string(data), "\n")
	for _, line := range lines {
		if !strings.HasPrefix(line, "cpu ") {
			continue
		}
		fields := strings.Fields(line)
		// cpu user nice system idle iowait irq softirq steal guest guest_nice
		if len(fields) < 5 {
			return procStatSnapshot{}, false
		}
		var values [10]uint64
		for i := 1; i < len(fields) && i < len(values)+1; i++ {
			n, err := strconv.ParseUint(fields[i], 10, 64)
			if err != nil {
				return procStatSnapshot{}, false
			}
			values[i-1] = n
		}
		var total uint64
		for _, v := range values {
			total += v
		}
		return procStatSnapshot{
			total: total,
			idle:  values[3] + values[4],
		}, true
	}
	return procStatSnapshot{}, false
}

// readMemUsed reads /proc/meminfo and returns used memory in bytes.
func readMemUsed() (float64, bool) {
	data, err := os.ReadFile("/proc/meminfo")
	if err != nil {
		return 0, false
	}
	var totalKB, availKB uint64
	for _, line := range strings.Split(string(data), "\n") {
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		switch fields[0] {
		case "MemTotal:":
			n, err := strconv.ParseUint(fields[1], 10, 64)
			if err != nil {
				return 0, false
			}
			totalKB = n
		case "MemAvailable:":
			n, err := strconv.ParseUint(fields[1], 10, 64)
			if err != nil {
				return 0, false
			}
			availKB = n
		}
	}
	if totalKB == 0 {
		return 0, false
	}
	usedKB := totalKB
	if availKB > 0 && availKB <= totalKB {
		usedKB = totalKB - availKB
	}
	return float64(usedKB) * 1024.0, true
}

// readLoadAvg reads /proc/loadavg and returns the 1/5/15 minute load averages.
func readLoadAvg() (float64, float64, float64, bool) {
	data, err := os.ReadFile("/proc/loadavg")
	if err != nil {
		return 0, 0, 0, false
	}
	fields := strings.Fields(string(data))
	if len(fields) < 3 {
		return 0, 0, 0, false
	}
	l1, err1 := strconv.ParseFloat(fields[0], 64)
	l5, err2 := strconv.ParseFloat(fields[1], 64)
	l15, err3 := strconv.ParseFloat(fields[2], 64)
	if err1 != nil || err2 != nil || err3 != nil {
		return 0, 0, 0, false
	}
	return l1, l5, l15, true
}

// readRootFSUsage returns the total and used bytes for the host root
// filesystem via statfs(2).
func readRootFSUsage() (int64, int64, bool) {
	var stat unix.Statfs_t
	if err := unix.Statfs("/", &stat); err != nil {
		return 0, 0, false
	}
	bsize := int64(stat.Bsize)
	if bsize <= 0 {
		return 0, 0, false
	}
	total := int64(stat.Blocks) * bsize
	free := int64(stat.Bavail) * bsize
	used := total - free
	if used < 0 {
		used = 0
	}
	return total, used, true
}

// readUptime reads the host uptime in seconds from /proc/uptime.
// Returns 0 if unavailable.
func readUptime() int64 {
	data, err := os.ReadFile("/proc/uptime")
	if err != nil {
		return 0
	}
	fields := strings.Fields(string(data))
	if len(fields) < 1 {
		return 0
	}
	secs, err := strconv.ParseFloat(fields[0], 64)
	if err != nil {
		return 0
	}
	return int64(secs)
}
