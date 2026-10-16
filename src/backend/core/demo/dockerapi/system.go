// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package dockerapi

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/api/types/events"
	"github.com/moby/moby/api/types/image"
	"github.com/moby/moby/api/types/registry"
	"github.com/moby/moby/api/types/swarm"
	"github.com/moby/moby/api/types/system"
	"github.com/moby/moby/api/types/volume"
	"github.com/opencontainers/go-digest"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"

	"github.com/therealmcsparrow/mcharbor/core/demo/estate"
)

func (s *Server) version(w http.ResponseWriter) {
	h := s.host
	engine := system.ComponentVersion{Name: "Engine", Version: h.DockerVersion, Details: map[string]string{
		"ApiVersion":    APIVersion,
		"Arch":          goArch(h.Arch),
		"BuildTime":     "2024-09-20T11:41:18.000000000+00:00",
		"Experimental":  "false",
		"GitCommit":     "41ca978",
		"GoVersion":     "go1.22.7",
		"KernelVersion": h.Kernel,
		"MinAPIVersion": "1.24",
		"Os":            "linux",
	}}
	writeJSON(w, http.StatusOK, system.VersionResponse{
		Platform:      system.PlatformInfo{Name: "Docker Engine - Community"},
		Components:    []system.ComponentVersion{engine, {Name: "containerd", Version: "1.7.22"}, {Name: "runc", Version: "1.1.14"}},
		Version:       h.DockerVersion,
		APIVersion:    APIVersion,
		MinAPIVersion: "1.24",
		GitCommit:     "41ca978",
		GoVersion:     "go1.22.7",
		Os:            "linux",
		Arch:          goArch(h.Arch),
		KernelVersion: h.Kernel,
		BuildTime:     "2024-09-20T11:41:18.000000000+00:00",
	})
}

func goArch(arch string) string {
	if arch == "aarch64" {
		return "arm64"
	}
	return "amd64"
}

func (s *Server) info(w http.ResponseWriter) {
	h := s.host
	ctrs := h.Containers()
	var running, paused, stopped int
	for _, c := range ctrs {
		switch c.State {
		case "running", "restarting":
			running++
		case "paused":
			paused++
		default:
			stopped++
		}
	}
	imgs, _ := h.Images()
	swarmInfo := swarm.Info{LocalNodeState: swarm.LocalNodeStateInactive}
	if s.swarm != nil {
		if si := s.swarm.Info(); si != nil {
			swarmInfo = *si
		}
	}
	writeJSON(w, http.StatusOK, system.Info{
		Swarm:              swarmInfo,
		ID:                 h.Key + "-" + strconv.FormatUint(uint64(len(h.Key))*7919, 16),
		Containers:         len(ctrs),
		ContainersRunning:  running,
		ContainersPaused:   paused,
		ContainersStopped:  stopped,
		Images:             len(imgs),
		Driver:             "overlay2",
		DriverStatus:       [][2]string{{"Backing Filesystem", "extfs"}, {"Supports d_type", "true"}, {"Using metacopy", "false"}, {"Native Overlay Diff", "true"}},
		MemoryLimit:        true,
		SwapLimit:          true,
		CPUCfsPeriod:       true,
		CPUCfsQuota:        true,
		CPUShares:          true,
		CPUSet:             true,
		PidsLimit:          true,
		IPv4Forwarding:     true,
		Debug:              false,
		NFd:                240 + len(ctrs)*6,
		NGoroutines:        180 + len(ctrs)*3,
		SystemTime:         s.now().Format(time.RFC3339Nano),
		LoggingDriver:      "json-file",
		CgroupDriver:       "systemd",
		CgroupVersion:      "2",
		NEventsListener:    1,
		KernelVersion:      h.Kernel,
		OperatingSystem:    osName(h),
		OSVersion:          h.OSVersion,
		OSType:             "linux",
		Architecture:       h.Arch,
		IndexServerAddress: "https://index.docker.io/v1/",
		RegistryConfig:     &registry.ServiceConfig{IndexConfigs: map[string]*registry.IndexInfo{"docker.io": {Name: "docker.io", Official: true, Secure: true}}},
		NCPU:               h.CPUs,
		MemTotal:           h.MemTotal,
		DockerRootDir:      "/var/lib/docker",
		Name:               h.Hostname,
		Labels:             []string{"region=" + h.Region, "role=" + h.Role},
		ServerVersion:      h.DockerVersion,
		Runtimes:           map[string]system.RuntimeWithStatus{"runc": {Runtime: system.Runtime{Path: "runc"}}},
		DefaultRuntime:     "runc",
		LiveRestoreEnabled: true,
		Isolation:          "",
		InitBinary:         "docker-init",
		ContainerdCommit:   system.Commit{ID: "7f7fdf5fed64eb6a7caf99b3e12efcf9d60e311c"},
		RuncCommit:         system.Commit{ID: "v1.1.14-0-g2c9f560"},
		InitCommit:         system.Commit{ID: "de40ad0"},
		SecurityOptions:    []string{"name=seccomp,profile=builtin", "name=cgroupns"},
	})
}

func osName(h *estate.Host) string {
	switch h.Distro {
	case "ubuntu":
		return "Ubuntu " + h.OSVersion + " LTS"
	case "debian":
		return "Debian GNU/Linux " + h.OSVersion + " (bookworm)"
	case "rocky":
		return "Rocky Linux " + h.OSVersion
	case "fedora":
		return "Fedora Linux " + h.OSVersion + " (Server Edition)"
	case "alpine":
		return "Alpine Linux v" + h.OSVersion
	}
	return "Linux"
}

// legacyDiskUsage is the /system/df body an API 1.47 daemon returns. The api module
// only models the 1.52+ shape, and the client decodes this one for older versions.
type legacyDiskUsage struct {
	LayersSize int64
	Images     []image.Summary
	Containers []container.Summary
	Volumes    []volume.Volume
}

func (s *Server) diskUsage(w http.ResponseWriter) {
	h := s.host
	imgs, used := h.Images()
	du := legacyDiskUsage{}
	for i := range imgs {
		img := imgs[i]
		sum := imageSummary(img, used[img.ID])
		du.Images = append(du.Images, sum)
		du.LayersSize += img.Size
	}
	for _, c := range h.Containers() {
		sum := s.containerSummary(c)
		sum.SizeRw = c.SizeRw
		du.Containers = append(du.Containers, sum)
	}
	vols, refs := h.Volumes()
	for _, v := range vols {
		vv := volumeDoc(v)
		vv.UsageData = &volume.UsageData{Size: v.SizeBytes, RefCount: int64(refs[v.Name])}
		du.Volumes = append(du.Volumes, vv)
	}
	writeJSON(w, http.StatusOK, du)
}

func imageSummary(img estate.Image, containers int) image.Summary {
	digests := []string{}
	if len(img.RepoTags) > 0 {
		digests = []string{repoOf(img.RepoTags[0]) + "@" + img.Digest}
	}
	tags := img.RepoTags
	if tags == nil {
		tags = []string{}
	}
	return image.Summary{
		Containers:  int64(containers),
		Created:     img.Created.Unix(),
		ID:          img.ID,
		Labels:      img.Labels,
		RepoDigests: digests,
		RepoTags:    tags,
		Size:        img.Size,
		SharedSize:  -1,
	}
}

// events streams container, image, volume and network events as they happen.
// A request with an until bound in the past is answered empty, as the daemon does.
func (s *Server) events(w http.ResponseWriter, r *http.Request) {
	ch, cancel := s.host.Subscribe()
	defer cancel()
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	flush(w)
	if until := r.URL.Query().Get("until"); until != "" {
		if sec, err := strconv.ParseInt(until, 10, 64); err == nil && sec <= s.now().Unix() {
			return
		}
	}
	f := parseFilters(r)
	enc := json.NewEncoder(w)
	for {
		select {
		case <-r.Context().Done():
			return
		case ev := <-ch:
			if types, ok := f["type"]; ok && !contains(types, ev.Type) {
				continue
			}
			msg := events.Message{
				Type:     events.Type(ev.Type),
				Action:   events.Action(ev.Action),
				Actor:    events.Actor{ID: ev.ActorID, Attributes: ev.Attributes},
				Scope:    "local",
				Time:     ev.Time.Unix(),
				TimeNano: ev.Time.UnixNano(),
			}
			if err := enc.Encode(msg); err != nil {
				return
			}
			flush(w)
		}
	}
}

func contains(list []string, v string) bool {
	for _, x := range list {
		if x == v {
			return true
		}
	}
	return false
}

func (s *Server) distribution(w http.ResponseWriter, r *http.Request, ref string) {
	img, err := s.host.Image(ref)
	dg := img.Digest
	if err != nil {
		dg = "sha256:" + fmt.Sprintf("%064x", estate.StableHash(ref))
	}
	writeJSON(w, http.StatusOK, registry.DistributionInspect{
		Descriptor: ocispec.Descriptor{MediaType: "application/vnd.oci.image.index.v1+json", Digest: digest.Digest(dg), Size: 1609},
		Platforms:  []ocispec.Platform{{Architecture: goArch(s.host.Arch), OS: "linux"}},
	})
}
