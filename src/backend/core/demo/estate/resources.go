// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package estate

import (
	"maps"
	"slices"
	"sort"
	"strings"
	"time"
)

func normalizeRef(ref string) string {
	ref = strings.TrimPrefix(ref, "docker.io/")
	ref = strings.TrimPrefix(ref, "library/")
	if !strings.Contains(ref, ":") && !strings.Contains(ref, "@") {
		ref += ":latest"
	}
	return ref
}

// imageByRef finds an image by id, id prefix or tag. Caller holds mu.
func (h *Host) imageByRef(ref string) *Image {
	if img, ok := h.images[ref]; ok {
		return img
	}
	bare := strings.TrimPrefix(ref, "sha256:")
	want := normalizeRef(ref)
	for _, img := range h.images {
		if strings.HasPrefix(strings.TrimPrefix(img.ID, "sha256:"), bare) && len(bare) >= 4 {
			return img
		}
		for _, tag := range img.RepoTags {
			if normalizeRef(tag) == want {
				return img
			}
		}
	}
	return nil
}

// Images returns a snapshot of every image with how many containers use it.
func (h *Host) Images() ([]Image, map[string]int) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	out := make([]Image, 0, len(h.images))
	for _, img := range h.images {
		cp := *img
		cp.RepoTags = slices.Clone(img.RepoTags)
		out = append(out, cp)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Created.After(out[j].Created) })
	used := make(map[string]int)
	for _, c := range h.containers {
		used[c.ImageID]++
	}
	return out, used
}

// Image returns a snapshot of the image ref names.
func (h *Host) Image(ref string) (Image, error) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	img := h.imageByRef(ref)
	if img == nil {
		return Image{}, ErrNotFound
	}
	cp := *img
	cp.RepoTags = slices.Clone(img.RepoTags)
	return cp, nil
}

// PullImage makes ref present, as a pull would, and returns it.
func (h *Host) PullImage(ref string) Image {
	h.mu.Lock()
	if img := h.imageByRef(ref); img != nil {
		cp := *img
		h.mu.Unlock()
		h.emit(Event{Type: "image", Action: "pull", ActorID: normalizeRef(ref), Attributes: map[string]string{"name": normalizeRef(ref)}})
		return cp
	}
	r := newRNG(hashString(h.Key + ref))
	img := &Image{
		ID:       "sha256:" + r.hex(64),
		RepoTags: []string{normalizeRef(ref)},
		Digest:   "sha256:" + r.hex(64),
		Created:  time.Now().Add(-time.Duration(r.between(24, 24*90)) * time.Hour),
		Size:     int64(r.between(20, 900)) << 20,
		Arch:     h.Arch,
		OS:       "linux",
	}
	h.images[img.ID] = img
	cp := *img
	h.mu.Unlock()
	h.emit(Event{Type: "image", Action: "pull", ActorID: normalizeRef(ref), Attributes: map[string]string{"name": normalizeRef(ref)}})
	return cp
}

// TagImage adds tag to the image ref names.
func (h *Host) TagImage(ref, tag string) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	img := h.imageByRef(ref)
	if img == nil {
		return ErrNotFound
	}
	tag = normalizeRef(tag)
	if !slices.Contains(img.RepoTags, tag) {
		img.RepoTags = append(img.RepoTags, tag)
	}
	return nil
}

// RemoveImage deletes an image; one in use needs force.
func (h *Host) RemoveImage(ref string, force bool) (Image, error) {
	h.mu.Lock()
	img := h.imageByRef(ref)
	if img == nil {
		h.mu.Unlock()
		return Image{}, ErrNotFound
	}
	if !force {
		for _, c := range h.containers {
			if c.ImageID == img.ID {
				h.mu.Unlock()
				return Image{}, ErrConflict
			}
		}
	}
	cp := *img
	delete(h.images, img.ID)
	h.mu.Unlock()
	h.emit(Event{Type: "image", Action: "delete", ActorID: cp.ID, Attributes: map[string]string{"name": cp.ID}})
	return cp, nil
}

// PruneImages removes images no container uses; dangling only unless all.
func (h *Host) PruneImages(all bool) ([]Image, int64) {
	h.mu.Lock()
	used := make(map[string]bool)
	for _, c := range h.containers {
		used[c.ImageID] = true
	}
	var removed []Image
	var reclaimed int64
	for id, img := range h.images {
		if used[id] || (!all && len(img.RepoTags) > 0) {
			continue
		}
		removed = append(removed, *img)
		reclaimed += img.Size
		delete(h.images, id)
	}
	h.mu.Unlock()
	for _, img := range removed {
		h.emit(Event{Type: "image", Action: "delete", ActorID: img.ID})
	}
	return removed, reclaimed
}

// Volumes returns every volume and which ones a container mounts.
func (h *Host) Volumes() ([]Volume, map[string]int) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	out := make([]Volume, 0, len(h.volumes))
	for _, v := range h.volumes {
		cp := *v
		cp.Labels = maps.Clone(v.Labels)
		out = append(out, cp)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	refs := make(map[string]int)
	for _, c := range h.containers {
		for _, m := range c.Mounts {
			if m.Type == "volume" {
				refs[m.Name]++
			}
		}
	}
	return out, refs
}

// Volume returns the volume named name.
func (h *Host) Volume(name string) (Volume, error) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	v, ok := h.volumes[name]
	if !ok {
		return Volume{}, ErrNotFound
	}
	cp := *v
	cp.Labels = maps.Clone(v.Labels)
	return cp, nil
}

// CreateVolume makes a named volume, returning the existing one on a repeat.
func (h *Host) CreateVolume(name, driver string, labels map[string]string) Volume {
	h.mu.Lock()
	if v, ok := h.volumes[name]; ok {
		cp := *v
		h.mu.Unlock()
		return cp
	}
	if name == "" {
		name = newRNG(uint64(time.Now().UnixNano())).hex(64)
	}
	if driver == "" {
		driver = "local"
	}
	v := &Volume{Name: name, Driver: driver, Created: time.Now(), Labels: labels}
	h.volumes[name] = v
	cp := *v
	h.mu.Unlock()
	h.emit(Event{Type: "volume", Action: "create", ActorID: name, Attributes: map[string]string{"driver": driver}})
	return cp
}

// RemoveVolume deletes a volume no container mounts, unless forced.
func (h *Host) RemoveVolume(name string, force bool) error {
	h.mu.Lock()
	if _, ok := h.volumes[name]; !ok {
		h.mu.Unlock()
		return ErrNotFound
	}
	if !force {
		for _, c := range h.containers {
			for _, m := range c.Mounts {
				if m.Type == "volume" && m.Name == name {
					h.mu.Unlock()
					return ErrConflict
				}
			}
		}
	}
	delete(h.volumes, name)
	h.mu.Unlock()
	h.emit(Event{Type: "volume", Action: "destroy", ActorID: name})
	return nil
}

// PruneVolumes removes every volume no container mounts.
func (h *Host) PruneVolumes() ([]string, int64) {
	_, refs := h.Volumes()
	h.mu.Lock()
	var removed []string
	var reclaimed int64
	for name, v := range h.volumes {
		if refs[name] == 0 {
			removed = append(removed, name)
			reclaimed += v.SizeBytes
			delete(h.volumes, name)
		}
	}
	h.mu.Unlock()
	for _, name := range removed {
		h.emit(Event{Type: "volume", Action: "destroy", ActorID: name})
	}
	return removed, reclaimed
}

// Networks returns every network on the host.
func (h *Host) Networks() []Network {
	h.mu.RLock()
	defer h.mu.RUnlock()
	out := make([]Network, 0, len(h.networks))
	for _, n := range h.networks {
		cp := *n
		cp.Labels = maps.Clone(n.Labels)
		out = append(out, cp)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

func (h *Host) networkByRef(ref string) *Network {
	if n, ok := h.networks[ref]; ok {
		return n
	}
	for _, n := range h.networks {
		if n.Name == ref || (len(ref) >= 4 && strings.HasPrefix(n.ID, ref)) {
			return n
		}
	}
	return nil
}

// Network returns the network ref names and the containers attached to it,
// keyed by container id with their address.
func (h *Host) Network(ref string) (Network, map[string]Container, error) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	n := h.networkByRef(ref)
	if n == nil {
		return Network{}, nil, ErrNotFound
	}
	attached := make(map[string]Container)
	for id, c := range h.containers {
		if _, ok := c.Networks[n.Name]; ok {
			attached[id] = cloneContainer(c)
		}
	}
	cp := *n
	cp.Labels = maps.Clone(n.Labels)
	return cp, attached, nil
}

// CreateNetwork adds a bridge network with the next free /24 subnet.
func (h *Host) CreateNetwork(name, driver string, internal bool, labels map[string]string) (Network, error) {
	h.mu.Lock()
	for _, n := range h.networks {
		if n.Name == name {
			h.mu.Unlock()
			return Network{}, ErrConflict
		}
	}
	if driver == "" {
		driver = "bridge"
	}
	octet := 18 + len(h.networks)%200
	n := &Network{
		ID:       newRNG(hashString(h.Key+name) ^ uint64(time.Now().UnixNano())).hex(64),
		Name:     name,
		Driver:   driver,
		Scope:    "local",
		Subnet:   "172." + itoa(octet) + ".0.0/16",
		Gateway:  "172." + itoa(octet) + ".0.1",
		Internal: internal,
		Created:  time.Now(),
		Labels:   labels,
	}
	h.networks[n.ID] = n
	cp := *n
	h.mu.Unlock()
	h.emit(Event{Type: "network", Action: "create", ActorID: n.ID, Attributes: map[string]string{"name": name, "type": driver}})
	return cp, nil
}

// RemoveNetwork deletes a network with no attached containers.
func (h *Host) RemoveNetwork(ref string) error {
	h.mu.Lock()
	n := h.networkByRef(ref)
	if n == nil {
		h.mu.Unlock()
		return ErrNotFound
	}
	if n.Name == "bridge" || n.Name == "host" || n.Name == "none" {
		h.mu.Unlock()
		return ErrConflict
	}
	for _, c := range h.containers {
		if _, ok := c.Networks[n.Name]; ok {
			h.mu.Unlock()
			return ErrConflict
		}
	}
	delete(h.networks, n.ID)
	h.mu.Unlock()
	h.emit(Event{Type: "network", Action: "destroy", ActorID: n.ID, Attributes: map[string]string{"name": n.Name}})
	return nil
}

// ConnectNetwork attaches a container to a network with a fresh address.
func (h *Host) ConnectNetwork(netRef, ctrRef string) error {
	h.mu.Lock()
	n := h.networkByRef(netRef)
	c := h.resolve(ctrRef)
	if n == nil || c == nil {
		h.mu.Unlock()
		return ErrNotFound
	}
	base := strings.TrimSuffix(n.Gateway, ".1")
	c.Networks[n.Name] = base + "." + itoa(2+len(c.Networks)+int(hashString(c.ID)%200))
	name := n.Name
	id := n.ID
	h.mu.Unlock()
	h.emit(Event{Type: "network", Action: "connect", ActorID: id, Attributes: map[string]string{"name": name, "container": c.ID}})
	return nil
}

// DisconnectNetwork detaches a container from a network.
func (h *Host) DisconnectNetwork(netRef, ctrRef string) error {
	h.mu.Lock()
	n := h.networkByRef(netRef)
	c := h.resolve(ctrRef)
	if n == nil || c == nil {
		h.mu.Unlock()
		return ErrNotFound
	}
	delete(c.Networks, n.Name)
	name := n.Name
	id := n.ID
	h.mu.Unlock()
	h.emit(Event{Type: "network", Action: "disconnect", ActorID: id, Attributes: map[string]string{"name": name, "container": c.ID}})
	return nil
}

// PruneNetworks removes custom networks no container is attached to.
func (h *Host) PruneNetworks() []string {
	h.mu.Lock()
	inUse := make(map[string]bool)
	for _, c := range h.containers {
		for name := range c.Networks {
			inUse[name] = true
		}
	}
	var removed []string
	for id, n := range h.networks {
		if inUse[n.Name] || n.Name == "bridge" || n.Name == "host" || n.Name == "none" {
			continue
		}
		removed = append(removed, n.Name)
		delete(h.networks, id)
	}
	h.mu.Unlock()
	return removed
}

// PruneContainers removes every stopped container.
func (h *Host) PruneContainers() ([]string, int64) {
	h.mu.Lock()
	var removed []Container
	var reclaimed int64
	for id, c := range h.containers {
		if c.State == StateExited || c.State == StateCreated || c.State == StateDead {
			removed = append(removed, cloneContainer(c))
			reclaimed += c.SizeRw
			delete(h.containers, id)
			delete(h.byName, c.Name)
		}
	}
	h.mu.Unlock()
	ids := make([]string, 0, len(removed))
	for i := range removed {
		ids = append(ids, removed[i].ID)
		h.emitContainer(&removed[i], "destroy")
	}
	return ids, reclaimed
}
