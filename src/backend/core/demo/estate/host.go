// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package estate

import (
	"errors"
	"maps"
	"slices"
	"sort"
	"strings"
	"time"
)

// ErrNotFound is returned when a container, image, volume, network or exec does
// not exist on the host; the Docker API layer maps it to a 404.
var ErrNotFound = errors.New("no such object")

// ErrConflict is returned when an operation is refused by the object's state,
// such as removing a running container; the Docker API layer maps it to a 409.
var ErrConflict = errors.New("conflict")

func cloneContainer(c *Container) Container {
	out := *c
	out.Labels = maps.Clone(c.Labels)
	out.Networks = maps.Clone(c.Networks)
	out.Env = slices.Clone(c.Env)
	out.Ports = slices.Clone(c.Ports)
	out.Mounts = slices.Clone(c.Mounts)
	return out
}

// Containers returns a snapshot of every container, newest first like Docker.
func (h *Host) Containers() []Container {
	h.mu.RLock()
	defer h.mu.RUnlock()
	out := make([]Container, 0, len(h.containers))
	for _, c := range h.containers {
		out = append(out, cloneContainer(c))
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Created.Equal(out[j].Created) {
			return out[i].Name < out[j].Name
		}
		return out[i].Created.After(out[j].Created)
	})
	return out
}

// resolve finds a container by full id, name (with or without the leading
// slash) or unique id prefix, the three ways the Docker API accepts. Caller holds mu.
func (h *Host) resolve(ref string) *Container {
	ref = strings.TrimPrefix(ref, "/")
	if c, ok := h.containers[ref]; ok {
		return c
	}
	if id, ok := h.byName[ref]; ok {
		return h.containers[id]
	}
	if len(ref) < 4 {
		return nil
	}
	var found *Container
	for id, c := range h.containers {
		if strings.HasPrefix(id, ref) {
			if found != nil {
				return nil
			}
			found = c
		}
	}
	return found
}

// Container returns a snapshot of the container ref names.
func (h *Host) Container(ref string) (Container, error) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	c := h.resolve(ref)
	if c == nil {
		return Container{}, ErrNotFound
	}
	return cloneContainer(c), nil
}

func (h *Host) mutate(ref string, fn func(c *Container) (string, error)) (Container, error) {
	h.mu.Lock()
	c := h.resolve(ref)
	if c == nil {
		h.mu.Unlock()
		return Container{}, ErrNotFound
	}
	action, err := fn(c)
	snap := cloneContainer(c)
	h.mu.Unlock()
	if err != nil {
		return snap, err
	}
	for _, a := range strings.Split(action, ",") {
		if a != "" {
			h.emitContainer(&snap, a)
		}
	}
	return snap, nil
}

// StartContainer moves a stopped container to running.
func (h *Host) StartContainer(ref string) (Container, error) {
	return h.mutate(ref, func(c *Container) (string, error) {
		switch c.State {
		case StateRunning, StateRestarting:
			return "", nil
		case StatePaused:
			return "", ErrConflict
		}
		c.State = StateRunning
		c.ExitCode = 0
		c.Error = ""
		c.StartedAt = time.Now()
		if c.Health == "unhealthy" || c.Health == "" && c.Labels["demo.health"] != "" {
			c.Health = "starting"
		}
		return "start", nil
	})
}

// StopContainer stops a running or paused container with a clean exit.
func (h *Host) StopContainer(ref string) (Container, error) {
	return h.mutate(ref, func(c *Container) (string, error) {
		if c.State != StateRunning && c.State != StatePaused && c.State != StateRestarting {
			return "", nil
		}
		c.State = StateExited
		c.ExitCode = 0
		c.FinishedAt = time.Now()
		if c.Health != "" {
			c.Health = "unhealthy"
		}
		return "kill,die,stop", nil
	})
}

// KillContainer stops a container the way SIGKILL does, exit code 137.
func (h *Host) KillContainer(ref string) (Container, error) {
	return h.mutate(ref, func(c *Container) (string, error) {
		if c.State != StateRunning && c.State != StatePaused && c.State != StateRestarting {
			return "", ErrConflict
		}
		c.State = StateExited
		c.ExitCode = 137
		c.FinishedAt = time.Now()
		return "kill,die", nil
	})
}

// RestartContainer stops and starts a container in one step.
func (h *Host) RestartContainer(ref string) (Container, error) {
	return h.mutate(ref, func(c *Container) (string, error) {
		c.State = StateRunning
		c.ExitCode = 0
		c.Error = ""
		c.StartedAt = time.Now()
		if c.Health != "" {
			c.Health = "healthy"
		}
		return "kill,die,stop,start,restart", nil
	})
}

// PauseContainer freezes a running container.
func (h *Host) PauseContainer(ref string) (Container, error) {
	return h.mutate(ref, func(c *Container) (string, error) {
		if c.State != StateRunning {
			return "", ErrConflict
		}
		c.State = StatePaused
		return "pause", nil
	})
}

// UnpauseContainer resumes a paused container.
func (h *Host) UnpauseContainer(ref string) (Container, error) {
	return h.mutate(ref, func(c *Container) (string, error) {
		if c.State != StatePaused {
			return "", ErrConflict
		}
		c.State = StateRunning
		return "unpause", nil
	})
}

// RenameContainer changes a container's name, refusing a name already in use.
func (h *Host) RenameContainer(ref, name string) (Container, error) {
	name = strings.TrimPrefix(name, "/")
	return h.mutate(ref, func(c *Container) (string, error) {
		if other, ok := h.byName[name]; ok && other != c.ID {
			return "", ErrConflict
		}
		delete(h.byName, c.Name)
		c.Name = name
		h.byName[name] = c.ID
		return "rename", nil
	})
}

// RemoveContainer deletes a container; a running one needs force, as in Docker.
func (h *Host) RemoveContainer(ref string, force bool) error {
	h.mu.Lock()
	c := h.resolve(ref)
	if c == nil {
		h.mu.Unlock()
		return ErrNotFound
	}
	if (c.State == StateRunning || c.State == StatePaused) && !force {
		h.mu.Unlock()
		return ErrConflict
	}
	snap := cloneContainer(c)
	delete(h.containers, c.ID)
	delete(h.byName, c.Name)
	h.mu.Unlock()
	if snap.State == StateRunning {
		h.emitContainer(&snap, "kill")
		h.emitContainer(&snap, "die")
	}
	h.emitContainer(&snap, "destroy")
	return nil
}

// AddContainer inserts c, minting an id when it has none, and reports "create".
func (h *Host) AddContainer(c *Container) (Container, error) {
	h.mu.Lock()
	if c.Name == "" {
		c.Name = randomName(newRNG(uint64(time.Now().UnixNano())))
	}
	c.Name = strings.TrimPrefix(c.Name, "/")
	if _, taken := h.byName[c.Name]; taken {
		h.mu.Unlock()
		return Container{}, ErrConflict
	}
	if c.ID == "" {
		c.ID = newRNG(uint64(time.Now().UnixNano()) ^ hashString(c.Name)).hex(64)
	}
	if c.Created.IsZero() {
		c.Created = time.Now()
	}
	if c.State == "" {
		c.State = StateCreated
	}
	if c.Labels == nil {
		c.Labels = map[string]string{}
	}
	if c.Networks == nil {
		c.Networks = map[string]string{}
	}
	if c.Hostname == "" {
		c.Hostname = c.ID[:12]
	}
	if img := h.imageByRef(c.Image); img != nil {
		c.ImageID = img.ID
	}
	h.containers[c.ID] = c
	h.byName[c.Name] = c.ID
	snap := cloneContainer(c)
	h.mu.Unlock()
	h.emitContainer(&snap, "create")
	return snap, nil
}

func randomName(r *rng) string {
	adjectives := []string{"brave", "calm", "eager", "fancy", "gentle", "happy", "jolly", "keen", "lucid", "mellow", "nifty", "quirky", "serene", "tender", "vivid", "witty"}
	people := []string{"turing", "hopper", "lovelace", "ritchie", "thompson", "torvalds", "wozniak", "knuth", "liskov", "hamilton", "shannon", "babbage", "noether", "curie", "tesla", "euler"}
	return pick(r, adjectives) + "_" + pick(r, people)
}

// Stack returns the Compose project name, or nil.
func (h *Host) Stack(name string) *Stack {
	h.mu.RLock()
	defer h.mu.RUnlock()
	s := h.stacks[name]
	if s == nil {
		return nil
	}
	cp := *s
	cp.Env = maps.Clone(s.Env)
	return &cp
}

// Stacks returns every Compose project on the host, sorted by name.
func (h *Host) Stacks() []Stack {
	h.mu.RLock()
	defer h.mu.RUnlock()
	out := make([]Stack, 0, len(h.stacks))
	for _, s := range h.stacks {
		cp := *s
		cp.Env = maps.Clone(s.Env)
		out = append(out, cp)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// PutStack records or replaces a Compose project's file text.
func (h *Host) PutStack(s Stack) {
	h.mu.Lock()
	defer h.mu.Unlock()
	cp := s
	h.stacks[s.Name] = &cp
}

// ProjectContainers returns the ids of every container labelled with project.
func (h *Host) ProjectContainers(project string) []string {
	h.mu.RLock()
	defer h.mu.RUnlock()
	var ids []string
	for id, c := range h.containers {
		if c.Labels["com.docker.compose.project"] == project {
			ids = append(ids, id)
		}
	}
	sort.Strings(ids)
	return ids
}

// Exec returns a snapshot of the exec session id.
func (h *Host) Exec(id string) (Exec, error) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	e, ok := h.execs[id]
	if !ok {
		return Exec{}, ErrNotFound
	}
	return *e, nil
}

// CreateExec registers an exec session in a running container.
func (h *Host) CreateExec(ref string, cmd []string, tty bool) (Exec, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	c := h.resolve(ref)
	if c == nil {
		return Exec{}, ErrNotFound
	}
	if c.State != StateRunning {
		return Exec{}, ErrConflict
	}
	e := &Exec{ID: newRNG(uint64(time.Now().UnixNano())).hex(64), ContainerID: c.ID, Cmd: cmd, Tty: tty}
	h.execs[e.ID] = e
	return *e, nil
}

// FinishExec marks an exec session done with exitCode.
func (h *Host) FinishExec(id string, exitCode int) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if e, ok := h.execs[id]; ok {
		e.Running = false
		e.ExitCode = exitCode
	}
}

// SetExecRunning marks an exec session as attached and running.
func (h *Host) SetExecRunning(id string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if e, ok := h.execs[id]; ok {
		e.Running = true
	}
}
