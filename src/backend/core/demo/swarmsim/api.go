// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package swarmsim

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/moby/moby/api/types/swarm"
)

type backend struct {
	sim *Sim
	c   *cluster
}

// errSequence is SwarmKit's wording for a stale version token; the Swarm
// plugin recognises it by this text and answers 409 instead of a bare 500.
const errSequence = "rpc error: code = Unknown desc = update out of sequence"

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		return
	}
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"message": msg})
}

func notFound(w http.ResponseWriter) { writeError(w, http.StatusNotFound, "page not found") }

// lock takes the simulation lock and brings the cluster up to now; every
// handler that reads or writes swarm state starts here.
func (b *backend) lock() (time.Time, func()) {
	b.sim.mu.Lock()
	now := b.sim.now()
	b.c.advance(now)
	return now, b.sim.mu.Unlock
}

// Info is the Swarm section of /info: an active manager with control
// available, which is exactly what the plugin's capability probe checks.
func (b *backend) Info() *swarm.Info {
	b.sim.mu.Lock()
	defer b.sim.mu.Unlock()
	c := b.c
	info := &swarm.Info{
		NodeID:           c.selfID,
		LocalNodeState:   swarm.LocalNodeStateActive,
		ControlAvailable: true,
		Nodes:            len(c.nodes),
	}
	for _, n := range c.nodes {
		if n.ID == c.selfID {
			info.NodeAddr = n.Status.Addr
		}
		if n.Spec.Role == swarm.NodeRoleManager {
			info.Managers++
			info.RemoteManagers = append(info.RemoteManagers, swarm.Peer{NodeID: n.ID, Addr: n.Status.Addr + ":2377"})
		}
	}
	ci := clone(c.info)
	info.Cluster = &ci
	return info
}

// ServeSwarm answers the swarm-only routes and the overlay half of /networks.
func (b *backend) ServeSwarm(w http.ResponseWriter, r *http.Request, path string) bool {
	under := func(root string) (string, bool) {
		if path == root || strings.HasPrefix(path, root+"/") {
			return strings.TrimPrefix(path, root), true
		}
		return "", false
	}
	if rest, ok := under("/swarm"); ok {
		b.swarm(w, r, rest)
		return true
	}
	if rest, ok := under("/nodes"); ok {
		b.nodes(w, r, rest)
		return true
	}
	if rest, ok := under("/services"); ok {
		b.services(w, r, rest)
		return true
	}
	if rest, ok := under("/tasks"); ok {
		b.tasksRoute(w, r, rest)
		return true
	}
	if rest, ok := under("/secrets"); ok {
		b.secretsRoute(w, r, rest)
		return true
	}
	if rest, ok := under("/configs"); ok {
		b.configsRoute(w, r, rest)
		return true
	}
	if rest, ok := under("/networks"); ok {
		return b.networks(w, r, rest)
	}
	return false
}

// checkVersion rejects a write carrying a version other than the current one,
// the optimistic-concurrency contract every SwarmKit update enforces.
func checkVersion(w http.ResponseWriter, r *http.Request, current swarm.Version) bool {
	raw := r.URL.Query().Get("version")
	if raw == "" {
		return true
	}
	v, err := strconv.ParseUint(raw, 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid swarm version")
		return false
	}
	if v != current.Index {
		writeError(w, http.StatusBadRequest, errSequence)
		return false
	}
	return true
}

func (b *backend) swarm(w http.ResponseWriter, r *http.Request, rest string) {
	switch rest {
	case "/init", "/join", "/leave", "/unlock":
		writeError(w, http.StatusServiceUnavailable, "Swarm "+strings.TrimPrefix(rest, "/")+" is not available in demo mode.")
		return
	}
	now, unlock := b.lock()
	defer unlock()
	c := b.c
	switch {
	case rest == "" && r.Method == http.MethodGet:
		writeJSON(w, http.StatusOK, swarm.Swarm{ClusterInfo: c.info, JoinTokens: c.tokens})
	case rest == "/unlockkey" && r.Method == http.MethodGet:
		if c.unlockKey == "" {
			writeError(w, http.StatusNotFound, "no unlock key is set")
			return
		}
		writeJSON(w, http.StatusOK, swarm.UnlockKeyResponse{UnlockKey: c.unlockKey})
	case rest == "/update" && r.Method == http.MethodPost:
		if !checkVersion(w, r, c.info.Version) {
			return
		}
		var spec swarm.Spec
		if err := json.NewDecoder(r.Body).Decode(&spec); err != nil {
			writeError(w, http.StatusBadRequest, "invalid swarm spec")
			return
		}
		if spec.Labels == nil {
			spec.Labels = map[string]string{}
		}
		if spec.Name == "" {
			spec.Name = "default"
		}
		c.info.Spec = spec
		if queryBool(r, "rotateWorkerToken") {
			c.tokens.Worker = c.joinToken()
		}
		if queryBool(r, "rotateManagerToken") {
			c.tokens.Manager = c.joinToken()
		}
		switch {
		case !spec.EncryptionConfig.AutoLockManagers:
			c.unlockKey = ""
		case c.unlockKey == "" || queryBool(r, "rotateManagerUnlockKey"):
			c.unlockKey = c.newUnlockKey()
		}
		c.info.Version = c.bump()
		c.info.UpdatedAt = now
		w.WriteHeader(http.StatusOK)
	default:
		notFound(w)
	}
}

func (c *cluster) findNode(ref string) *swarm.Node {
	for _, n := range c.nodes {
		if n.ID == ref || n.Description.Hostname == ref {
			return n
		}
	}
	for _, n := range c.nodes {
		if ref != "" && strings.HasPrefix(n.ID, ref) {
			return n
		}
	}
	return nil
}

func (b *backend) nodes(w http.ResponseWriter, r *http.Request, rest string) {
	now, unlock := b.lock()
	defer unlock()
	c := b.c
	if rest == "" {
		if r.Method != http.MethodGet {
			notFound(w)
			return
		}
		f := parseFilters(r)
		out := []swarm.Node{}
		for _, n := range c.nodes {
			if b.matchNode(f, n) {
				out = append(out, *n)
			}
		}
		writeJSON(w, http.StatusOK, out)
		return
	}
	ref, action, _ := strings.Cut(strings.TrimPrefix(rest, "/"), "/")
	n := c.findNode(ref)
	if n == nil {
		writeError(w, http.StatusNotFound, "node "+ref+" not found")
		return
	}
	switch {
	case action == "" && r.Method == http.MethodGet:
		writeJSON(w, http.StatusOK, n)
	case action == "update" && r.Method == http.MethodPost:
		b.updateNode(w, r, n, now)
	case action == "" && r.Method == http.MethodDelete:
		b.removeNode(w, r, n, now)
	default:
		notFound(w)
	}
}

func (b *backend) updateNode(w http.ResponseWriter, r *http.Request, n *swarm.Node, now time.Time) {
	c := b.c
	if !checkVersion(w, r, n.Version) {
		return
	}
	var spec swarm.NodeSpec
	if err := json.NewDecoder(r.Body).Decode(&spec); err != nil {
		writeError(w, http.StatusBadRequest, "invalid node spec")
		return
	}
	switch spec.Availability {
	case swarm.NodeAvailabilityActive, swarm.NodeAvailabilityPause, swarm.NodeAvailabilityDrain:
	case "":
		spec.Availability = n.Spec.Availability
	default:
		writeError(w, http.StatusBadRequest, "invalid availability: "+string(spec.Availability))
		return
	}
	switch spec.Role {
	case swarm.NodeRoleManager, swarm.NodeRoleWorker:
	case "":
		spec.Role = n.Spec.Role
	default:
		writeError(w, http.StatusBadRequest, "invalid role: "+string(spec.Role))
		return
	}
	if spec.Role == swarm.NodeRoleWorker && n.Spec.Role == swarm.NodeRoleManager {
		if n.ID == c.selfID {
			writeError(w, http.StatusBadRequest, "Demoting the manager that serves this demo environment is not available in demo mode.")
			return
		}
		managers := 0
		for _, m := range c.nodes {
			if m.Spec.Role == swarm.NodeRoleManager {
				managers++
			}
		}
		if managers <= 1 {
			writeError(w, http.StatusBadRequest, "rpc error: code = FailedPrecondition desc = attempting to demote the last manager of the swarm")
			return
		}
	}
	if spec.Labels == nil {
		spec.Labels = map[string]string{}
	}
	if spec.Role != n.Spec.Role {
		if spec.Role == swarm.NodeRoleManager {
			n.ManagerStatus = &swarm.ManagerStatus{Reachability: swarm.ReachabilityReachable, Addr: n.Status.Addr + ":2377"}
		} else {
			n.ManagerStatus = nil
		}
	}
	n.Spec = spec
	n.Version = c.bump()
	n.UpdatedAt = now
	for _, svc := range c.services {
		c.reconcile(svc, now)
	}
	w.WriteHeader(http.StatusOK)
}

func (b *backend) removeNode(w http.ResponseWriter, r *http.Request, n *swarm.Node, now time.Time) {
	c := b.c
	if n.ID == c.selfID || n.Spec.Role == swarm.NodeRoleManager {
		writeError(w, http.StatusBadRequest, "rpc error: code = FailedPrecondition desc = node "+n.ID+" is a cluster manager and is a member of the raft cluster. It must be demoted to worker before removal")
		return
	}
	if n.Status.State != swarm.NodeStateDown && !queryBool(r, "force") {
		writeError(w, http.StatusBadRequest, "rpc error: code = FailedPrecondition desc = node "+n.ID+" is not down and can't be removed")
		return
	}
	kept := c.nodes[:0]
	for _, m := range c.nodes {
		if m.ID != n.ID {
			kept = append(kept, m)
		}
	}
	c.nodes = kept
	for _, t := range c.tasks {
		if t.NodeID == n.ID && t.DesiredState == swarm.TaskStateRunning {
			t.stages = nil
			t.DesiredState = swarm.TaskStateShutdown
			t.Status = swarm.TaskStatus{Timestamp: now, State: swarm.TaskStateOrphaned, Message: "node removed from the swarm", ContainerStatus: t.Status.ContainerStatus}
			t.UpdatedAt = now
			t.Version = c.bump()
		}
	}
	for _, svc := range c.services {
		c.reconcile(svc, now)
	}
	w.WriteHeader(http.StatusOK)
}

func (b *backend) tasksRoute(w http.ResponseWriter, r *http.Request, rest string) {
	if strings.HasSuffix(rest, "/logs") && r.Method == http.MethodGet {
		b.taskLogs(w, r, strings.TrimSuffix(strings.TrimPrefix(rest, "/"), "/logs"))
		return
	}
	_, unlock := b.lock()
	defer unlock()
	c := b.c
	if rest == "" && r.Method == http.MethodGet {
		f := parseFilters(r)
		out := []swarm.Task{}
		for _, t := range c.tasks {
			if b.matchTask(f, t) {
				out = append(out, t.Task)
			}
		}
		writeJSON(w, http.StatusOK, out)
		return
	}
	ref := strings.TrimPrefix(rest, "/")
	if r.Method != http.MethodGet || strings.Contains(ref, "/") {
		notFound(w)
		return
	}
	t := c.findTask(ref)
	if t == nil {
		writeError(w, http.StatusNotFound, "task "+ref+" not found")
		return
	}
	writeJSON(w, http.StatusOK, t.Task)
}

func (c *cluster) findTask(ref string) *task {
	for _, t := range c.tasks {
		if t.ID == ref {
			return t
		}
	}
	for _, t := range c.tasks {
		if ref != "" && strings.HasPrefix(t.ID, ref) {
			return t
		}
	}
	return nil
}
