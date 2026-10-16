// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package system

import (
	"context"
	"fmt"
	"path"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode"
)

// Host device categories, in the order the picker groups them.
const (
	DeviceCategorySerial  = "serial"
	DeviceCategoryGPU     = "gpu"
	DeviceCategoryAudio   = "audio"
	DeviceCategoryVideo   = "video"
	DeviceCategoryInput   = "input"
	DeviceCategoryNetwork = "network"
	DeviceCategoryUSB     = "usb"
	DeviceCategorySystem  = "system"
	DeviceCategoryBlock   = "block"
)

var deviceCategoryOrder = map[string]int{
	DeviceCategorySerial: 0, DeviceCategoryGPU: 1, DeviceCategoryAudio: 2, DeviceCategoryVideo: 3,
	DeviceCategoryInput: 4, DeviceCategoryNetwork: 5, DeviceCategoryUSB: 6, DeviceCategorySystem: 7,
	DeviceCategoryBlock: 8,
}

const (
	// maxHostDevices caps the list so a host with thousands of partitions or
	// input nodes cannot produce a response the picker would choke on.
	maxHostDevices = 512
	// maxHostDeviceLines bounds how much script output the parser will read.
	maxHostDeviceLines = 8000
	// maxHostDeviceField bounds any single path, label or udev value.
	maxHostDeviceField = 256
	// hostDevicesTimeout is the whole budget, image check and container included.
	hostDevicesTimeout = 30 * time.Second
)

// HostDevice is one character or block device node on an environment's host
// that could be mapped into a container.
type HostDevice struct {
	Path       string   `json:"path"`
	Type       string   `json:"type"`
	Major      int      `json:"major"`
	Minor      int      `json:"minor"`
	Category   string   `json:"category"`
	Label      string   `json:"label,omitempty"`
	StablePath string   `json:"stablePath,omitempty"`
	Links      []string `json:"links,omitempty"`
	Driver     string   `json:"driver,omitempty"`
	Dangerous  bool     `json:"dangerous"`
}

// HostDeviceList is the answer to GET /system/host-devices; Truncated says the
// host had more nodes than the cap and the operator may need manual entry.
type HostDeviceList struct {
	Devices     []HostDevice `json:"devices"`
	Truncated   bool         `json:"truncated"`
	CollectedAt time.Time    `json:"collectedAt"`
}

// hostDevicesScript enumerates device nodes under the read-only /host mount.
// It never writes, and every path it prints is re-validated by the parser.
const hostDevicesScript = `cd /host/dev 2>/dev/null || exit 0
{
n=0
for p in ttyUSB* ttyACM* ttyAMA* ttyS* dri/card* dri/renderD* nvidia* nvidia-caps/* snd/* video* media* input/event* hidraw* net/tun fuse kvm bus/usb/*/* sd[a-z] sd[a-z][0-9]* sd[a-z][a-z] nvme*n* mmcblk*; do
  [ -L "$p" ] && continue
  if [ -c "$p" ]; then k=c; ty=char; elif [ -b "$p" ]; then k=b; ty=block; else continue; fi
  case "$p" in ttyS[0-9]*) st=$(cat "/host/sys/class/tty/$p/type" 2>/dev/null); [ -n "$st" ] && [ "$st" != 0 ] || continue;; esac
  n=$((n+1)); if [ "$n" -gt 600 ]; then echo "T|1"; break; fi
  set -- $(stat -c '%t %T' "$p" 2>/dev/null)
  [ -n "$2" ] || continue
  ma=$((0x$1)); mi=$((0x$2))
  echo "D|/dev/$p|$ty|$ma|$mi"
  f="/host/run/udev/data/$k$ma:$mi"
  if [ -r "$f" ]; then grep -E '^E:(ID_VENDOR|ID_MODEL|ID_SERIAL|ID_VENDOR_FROM_DATABASE|ID_MODEL_FROM_DATABASE|ID_FS_LABEL)=' "$f" | head -n 8 | sed "s#^E:#U|/dev/$p|#"; fi
  case "$p" in dri/*) drv=$(readlink "/host/sys/class/drm/${p#dri/}/device/driver" 2>/dev/null); [ -n "$drv" ] && echo "R|/dev/$p|${drv##*/}";; esac
done
for l in serial/by-id/* serial/by-path/* disk/by-id/* disk/by-path/* dri/by-path/* snd/by-id/* snd/by-path/* input/by-id/* input/by-path/* v4l/by-id/* v4l/by-path/*; do
  [ -L "$l" ] || continue
  r=$(readlink -f "$l" 2>/dev/null) || continue
  case "$r" in /host/dev/*) echo "L|/dev/$l|${r#/host}";; esac
done
echo "OK"
} 2>/dev/null | head -n 8000`

// HostDevices lists the device nodes on envID's host. It runs through the
// read-only utility container, which the agent's Docker proxy also serves.
func (s *Service) HostDevices(ctx context.Context, envID string) (*HostDeviceList, error) {
	cli, err := s.pool.GetContext(ctx, envID)
	if err != nil {
		return nil, fmt.Errorf("getting docker client for %s: %w", envID, err)
	}
	result, err := s.runHostCommandWith(ctx, cli, hostDevicesScript, false, hostDevicesTimeout)
	if err != nil {
		return nil, fmt.Errorf("enumerating host devices on %s: %w", envID, err)
	}
	list := parseHostDevices(result.Output)
	list.CollectedAt = time.Now().UTC()
	return &list, nil
}

type rawHostDevice struct {
	device HostDevice
	udev   map[string]string
}

// parseHostDevices turns the script's line records into a sorted, bounded list.
// Anything that is not a clean path under /dev is dropped rather than trusted.
func parseHostDevices(output string) HostDeviceList {
	list := HostDeviceList{Devices: []HostDevice{}}
	byPath := map[string]*rawHostDevice{}
	order := []string{}
	links := map[string][]string{}

	lines := strings.Split(output, "\n")
	if len(lines) > maxHostDeviceLines {
		lines = lines[:maxHostDeviceLines]
		list.Truncated = true
	}
	for _, line := range lines {
		line = strings.TrimRight(line, "\r")
		kind, rest, ok := strings.Cut(line, "|")
		if !ok {
			continue
		}
		switch kind {
		case "D":
			parts := strings.Split(rest, "|")
			if len(parts) != 4 || !validDevPath(parts[0]) || byPath[parts[0]] != nil {
				continue
			}
			if parts[1] != "char" && parts[1] != "block" {
				continue
			}
			major, errMajor := strconv.Atoi(parts[2])
			minor, errMinor := strconv.Atoi(parts[3])
			if errMajor != nil || errMinor != nil || major < 0 || minor < 0 {
				continue
			}
			if len(order) >= maxHostDevices {
				list.Truncated = true
				continue
			}
			byPath[parts[0]] = &rawHostDevice{
				device: HostDevice{Path: parts[0], Type: parts[1], Major: major, Minor: minor},
				udev:   map[string]string{},
			}
			order = append(order, parts[0])
		case "U":
			devPath, pair, ok := strings.Cut(rest, "|")
			key, value, okPair := strings.Cut(pair, "=")
			raw := byPath[devPath]
			if !ok || !okPair || raw == nil {
				continue
			}
			raw.udev[key] = cleanField(value)
		case "R":
			devPath, driver, ok := strings.Cut(rest, "|")
			if raw := byPath[devPath]; ok && raw != nil {
				raw.device.Driver = cleanField(driver)
			}
		case "L":
			link, target, ok := strings.Cut(rest, "|")
			if ok && validDevPath(link) && validDevPath(target) {
				links[target] = append(links[target], link)
			}
		case "T":
			list.Truncated = true
		}
	}

	for _, devPath := range order {
		raw := byPath[devPath]
		device := raw.device
		device.Category = deviceCategory(device.Path, device.Type)
		device.Dangerous = device.Type == "block"
		device.Links = links[devPath]
		sort.Strings(device.Links)
		device.StablePath = stableDevicePath(device.Links)
		device.Label = deviceLabel(raw.udev, device.StablePath, device.Driver)
		list.Devices = append(list.Devices, device)
	}
	sort.SliceStable(list.Devices, func(i, j int) bool {
		a, b := list.Devices[i], list.Devices[j]
		if deviceCategoryOrder[a.Category] != deviceCategoryOrder[b.Category] {
			return deviceCategoryOrder[a.Category] < deviceCategoryOrder[b.Category]
		}
		return a.Path < b.Path
	})
	return list
}

// validDevPath accepts only a canonical absolute path strictly under /dev.
func validDevPath(p string) bool {
	if len(p) > maxHostDeviceField || !strings.HasPrefix(p, "/dev/") || path.Clean(p) != p {
		return false
	}
	for _, r := range p {
		if unicode.IsControl(r) || r == '|' {
			return false
		}
	}
	return true
}

func cleanField(value string) string {
	value = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return -1
		}
		return r
	}, value)
	if len(value) > maxHostDeviceField {
		value = value[:maxHostDeviceField]
	}
	return strings.TrimSpace(value)
}

func deviceCategory(devPath, devType string) string {
	if devType == "block" {
		return DeviceCategoryBlock
	}
	name := strings.TrimPrefix(devPath, "/dev/")
	switch {
	case strings.HasPrefix(name, "tty"):
		return DeviceCategorySerial
	case strings.HasPrefix(name, "dri/"), strings.HasPrefix(name, "nvidia"):
		return DeviceCategoryGPU
	case strings.HasPrefix(name, "snd/"):
		return DeviceCategoryAudio
	case strings.HasPrefix(name, "video"), strings.HasPrefix(name, "media"):
		return DeviceCategoryVideo
	case strings.HasPrefix(name, "input/"), strings.HasPrefix(name, "hidraw"):
		return DeviceCategoryInput
	case name == "net/tun":
		return DeviceCategoryNetwork
	case strings.HasPrefix(name, "bus/usb/"):
		return DeviceCategoryUSB
	default:
		return DeviceCategorySystem
	}
}

// stableDevicePath prefers a by-id name over by-path, and a model-derived
// by-id over wwn-/eui. identifiers nobody recognises.
func stableDevicePath(links []string) string {
	best, bestRank := "", 99
	for _, link := range links {
		rank := 3
		base := path.Base(link)
		switch {
		case strings.Contains(link, "/by-id/") && !strings.HasPrefix(base, "wwn-") && !strings.HasPrefix(base, "nvme-eui."):
			rank = 0
		case strings.Contains(link, "/by-id/"):
			rank = 1
		case strings.Contains(link, "/by-path/"):
			rank = 2
		}
		if rank < bestRank || (rank == bestRank && link < best) {
			best, bestRank = link, rank
		}
	}
	return best
}

func deviceLabel(udev map[string]string, stablePath, driver string) string {
	vendor := firstNonEmpty(udev["ID_VENDOR_FROM_DATABASE"], underscoresToSpaces(udev["ID_VENDOR"]))
	model := firstNonEmpty(udev["ID_MODEL_FROM_DATABASE"], underscoresToSpaces(udev["ID_MODEL"]))
	switch {
	case model != "" && vendor != "" && !strings.HasPrefix(strings.ToLower(model), strings.ToLower(vendor)):
		return vendor + " " + model
	case model != "":
		return model
	case udev["ID_FS_LABEL"] != "":
		return udev["ID_FS_LABEL"]
	case udev["ID_SERIAL"] != "":
		return underscoresToSpaces(udev["ID_SERIAL"])
	case vendor != "":
		return vendor
	case stablePath != "" && strings.Contains(stablePath, "/by-id/"):
		return path.Base(stablePath)
	default:
		return driver
	}
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}

func underscoresToSpaces(value string) string {
	return strings.TrimSpace(strings.ReplaceAll(value, "_", " "))
}
