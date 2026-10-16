// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package system

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/api/types/swarm"
	dockerclient "github.com/moby/moby/client"

	"github.com/therealmcsparrow/mcharbor/core/auth"
	coredocker "github.com/therealmcsparrow/mcharbor/core/docker"
	"github.com/therealmcsparrow/mcharbor/core/plugincaps"
)

// composeProjectLabel is the label that ties McHarbor's own container to the
// services deployed beside it, which a reboot must leave for systemd to stop.
const composeProjectLabel = "com.docker.compose.project"

// swarmTaskLabel marks a container Swarm owns; Swarm reschedules it, so McHarbor
// never restarts one itself.
const swarmTaskLabel = "com.docker.swarm.task.id"

// dockerRebootHost drives one Docker daemon for the graceful stop.
type dockerRebootHost struct {
	cli *dockerclient.Client
	// skipIDs and skipProject keep McHarbor's own container, and the stack it
	// was deployed with, running until systemd stops them after the reboot is issued.
	skipIDs     map[string]bool
	skipProject string
}

// newDockerRebootHost wraps cli and works out whether McHarbor itself runs on
// that daemon, in which case its own container and compose project are spared.
func newDockerRebootHost(ctx context.Context, cli *dockerclient.Client) (*dockerRebootHost, bool) {
	host := &dockerRebootHost{cli: cli, skipIDs: map[string]bool{}}
	self, err := coredocker.CurrentMcHarborContainer(ctx, cli)
	if err != nil || self.ID == "" {
		return host, false
	}
	host.skipIDs[self.ID] = true
	if self.Config != nil {
		host.skipProject = self.Config.Labels[composeProjectLabel]
	}
	return host, true
}

// spared reports whether a container is left for systemd rather than stopped by
// McHarbor: McHarbor itself, the agent it talks through, and protected resources.
func (d *dockerRebootHost) spared(summary container.Summary) bool {
	if d.skipIDs[summary.ID] {
		return true
	}
	if d.skipProject != "" && summary.Labels[composeProjectLabel] == d.skipProject {
		return true
	}
	return coredocker.IsProtectedContainer(summary.Names, summary.Image, summary.Labels) ||
		coredocker.IsAgentContainer(summary.Names, summary.Image, summary.Labels)
}

// RunningContainers lists the running containers McHarbor will stop, with each
// one's own stop signal and restart policy read from its configuration.
func (d *dockerRebootHost) RunningContainers(ctx context.Context) ([]rebootContainer, error) {
	listCtx, cancel := context.WithTimeout(ctx, hostCallTimeout)
	defer cancel()
	summariesRes, err := d.cli.ContainerList(listCtx, dockerclient.ContainerListOptions{})
	if err != nil {
		return nil, fmt.Errorf("listing containers: %w", err)
	}
	summaries := summariesRes.Items

	var (
		mu     sync.Mutex
		wg     sync.WaitGroup
		out    []rebootContainer
		failed error
	)
	for _, summary := range summaries {
		if d.spared(summary) {
			continue
		}
		wg.Add(1)
		go func(summary container.Summary) {
			defer wg.Done()
			inspectCtx, cancel := context.WithTimeout(ctx, hostCallTimeout)
			defer cancel()
			infoRes, err := d.cli.ContainerInspect(inspectCtx, summary.ID, dockerclient.ContainerInspectOptions{})
			info := infoRes.Container
			mu.Lock()
			defer mu.Unlock()
			if err != nil {
				failed = fmt.Errorf("inspecting container %s: %w", summary.ID, err)
				return
			}
			out = append(out, describeContainer(summary, info))
		}(summary)
	}
	wg.Wait()
	if failed != nil {
		return nil, failed
	}
	return out, nil
}

// describeContainer reduces an inspect to what the graceful stop needs.
func describeContainer(summary container.Summary, info container.InspectResponse) rebootContainer {
	ctr := rebootContainer{ID: summary.ID, StopSignal: "SIGTERM"}
	if info.ID != "" {
		ctr.Name = strings.TrimPrefix(info.Name, "/")
		if info.HostConfig != nil {
			ctr.RestartPolicy = string(info.HostConfig.RestartPolicy.Name)
		}
	}
	if ctr.Name == "" && len(summary.Names) > 0 {
		ctr.Name = strings.TrimPrefix(summary.Names[0], "/")
	}
	if info.Config != nil && strings.TrimSpace(info.Config.StopSignal) != "" {
		ctr.StopSignal = strings.TrimSpace(info.Config.StopSignal)
	}
	ctr.SwarmTask = summary.Labels[swarmTaskLabel] != ""
	return ctr
}

// SignalStop sends the container its own stop signal and nothing else. Docker's
// stop endpoint is not used because it escalates to SIGKILL when its timeout ends.
func (d *dockerRebootHost) SignalStop(ctx context.Context, c rebootContainer) error {
	killCtx, cancel := context.WithTimeout(ctx, hostCallTimeout)
	defer cancel()
	_, err := d.cli.ContainerKill(killCtx, c.ID, dockerclient.ContainerKillOptions{Signal: c.StopSignal})
	return err
}

// StillRunning returns the ids from ids that are still running.
func (d *dockerRebootHost) StillRunning(ctx context.Context, ids []string) ([]string, error) {
	listCtx, cancel := context.WithTimeout(ctx, hostCallTimeout)
	defer cancel()
	summariesRes, err := d.cli.ContainerList(listCtx, dockerclient.ContainerListOptions{})
	if err != nil {
		return nil, fmt.Errorf("listing containers: %w", err)
	}
	summaries := summariesRes.Items
	running := make(map[string]bool, len(summaries))
	for _, summary := range summaries {
		running[summary.ID] = true
	}
	still := make([]string, 0, len(ids))
	for _, id := range ids {
		if running[id] {
			still = append(still, id)
		}
	}
	return still, nil
}

// Start starts one container again.
func (d *dockerRebootHost) Start(ctx context.Context, id string) error {
	startCtx, cancel := context.WithTimeout(ctx, hostCallTimeout)
	defer cancel()
	_, err := d.cli.ContainerStart(startCtx, id, dockerclient.ContainerStartOptions{})
	return err
}

// Swarm roles a host can hold.
const (
	swarmRoleManager = "manager"
	swarmRoleWorker  = "worker"
)

// rebootSwarm is the slice of the Swarm API the reboot needs: pausing the node so
// its tasks move elsewhere as they stop, and restoring it afterwards.
type rebootSwarm interface {
	Role(ctx context.Context) (role, nodeID string, err error)
	Availability(ctx context.Context, nodeID string) (string, error)
	SetAvailability(ctx context.Context, nodeID, availability string) error
}

// Role reports whether the daemon is a Swarm manager, a worker, or neither.
func (d *dockerRebootHost) Role(ctx context.Context) (string, string, error) {
	infoCtx, cancel := context.WithTimeout(ctx, hostCallTimeout)
	defer cancel()
	infoRes, err := d.cli.Info(infoCtx, dockerclient.InfoOptions{})
	info := infoRes.Info
	if err != nil {
		return "", "", fmt.Errorf("reading docker info: %w", err)
	}
	if info.Swarm.LocalNodeState != swarm.LocalNodeStateActive {
		return "", "", nil
	}
	if info.Swarm.ControlAvailable {
		return swarmRoleManager, info.Swarm.NodeID, nil
	}
	return swarmRoleWorker, info.Swarm.NodeID, nil
}

// Availability reads the node's current availability.
func (d *dockerRebootHost) Availability(ctx context.Context, nodeID string) (string, error) {
	nodeCtx, cancel := context.WithTimeout(ctx, hostCallTimeout)
	defer cancel()
	inspected, err := d.cli.NodeInspect(nodeCtx, nodeID, dockerclient.NodeInspectOptions{})
	if err != nil {
		return "", fmt.Errorf("inspecting swarm node: %w", err)
	}
	node := inspected.Node
	return string(node.Spec.Availability), nil
}

// SetAvailability moves the node to availability, re-reading its version so the
// update is not rejected as stale.
func (d *dockerRebootHost) SetAvailability(ctx context.Context, nodeID, availability string) error {
	nodeCtx, cancel := context.WithTimeout(ctx, hostCallTimeout)
	defer cancel()
	inspected, err := d.cli.NodeInspect(nodeCtx, nodeID, dockerclient.NodeInspectOptions{})
	if err != nil {
		return fmt.Errorf("inspecting swarm node: %w", err)
	}
	node := inspected.Node
	spec := node.Spec
	spec.Availability = swarm.NodeAvailability(availability)
	if _, err := d.cli.NodeUpdate(nodeCtx, nodeID, dockerclient.NodeUpdateOptions{Version: node.Version, Spec: spec}); err != nil {
		return fmt.Errorf("updating swarm node availability: %w", err)
	}
	return nil
}

// guestRequestTimeout bounds one shutdown request. The plugin keeps the guest's grace on the
// agent after this; only McHarbor stops listening, and polls for the outcome instead.
const guestRequestTimeout = 20 * time.Second

// drainListTimeout bounds one list of running guests, which is one agent query by the plugin.
const drainListTimeout = 90 * time.Second

// maxDrainResponse bounds what one drain route may answer with.
const maxDrainResponse = 1 << 20

// guestRebootHost drives the guests an agent host runs through the plugins that declare a
// reboot-drain, one adapter per declared subject type, so no step of the graceful stop
// branches on a guest kind or reads a plugin's tables.
type guestRebootHost struct {
	adapters []rebootGuestAdapter
}

// rebootGuestAdapter is one guest family's slice of the graceful stop: which of its guests
// are up, and a request that one shut itself down.
type rebootGuestAdapter interface {
	kind() string
	running(ctx context.Context) ([]rebootGuest, error)
	requestShutdown(ctx context.Context, g rebootGuest) (escalated bool, err error)
}

// pluginRouteServer is the live plugin router, which serves only plugin routes.
type pluginRouteServer interface {
	ServeHTTP(w http.ResponseWriter, r *http.Request)
}

// newGuestRebootHost builds one adapter per reboot-drain declaration. A subject no loaded
// plugin declares simply has no guests to drain.
func newGuestRebootHost(drains []plugincaps.Capability, plugins pluginRouteServer, envID string, principal *auth.User) *guestRebootHost {
	host := &guestRebootHost{}
	for _, decl := range drains {
		host.adapters = append(host.adapters, &drainAdapter{decl: decl, plugins: plugins, envID: envID, principal: principal})
	}
	return host
}

// adapter looks a guest's family up among the registered adapters.
func (c *guestRebootHost) adapter(kind string) (rebootGuestAdapter, bool) {
	for _, a := range c.adapters {
		if a.kind() == kind {
			return a, true
		}
	}
	return nil, false
}

// RunningGuests asks every registered adapter which of its guests are up.
func (c *guestRebootHost) RunningGuests(ctx context.Context) ([]rebootGuest, error) {
	var out []rebootGuest
	for _, a := range c.adapters {
		up, err := a.running(ctx)
		if err != nil {
			return nil, err
		}
		out = append(out, up...)
	}
	return out, nil
}

// errNoGuestAdapter refuses a guest no registered adapter drives, rather than guessing
// which plugin to ask.
var errNoGuestAdapter = errors.New("no loaded plugin drains this guest kind")

// RequestShutdown hands the guest to its family's adapter. A request that times out is
// still in progress on the agent, not failed.
func (c *guestRebootHost) RequestShutdown(ctx context.Context, g rebootGuest) error {
	a, ok := c.adapter(g.Kind)
	if !ok {
		return fmt.Errorf("%w: %q", errNoGuestAdapter, g.Kind)
	}
	escalated, err := a.requestShutdown(ctx, g)
	if stillWaiting(err) {
		err = nil
	}
	if escalated {
		err = errors.New("the agent escalated to a hard stop")
	}
	return err
}

// stillWaiting reports whether err only means nobody waited for the answer: the
// shutdown is still running on the agent.
func stillWaiting(err error) bool {
	if err == nil {
		return false
	}
	return errors.Is(err, context.DeadlineExceeded) || strings.Contains(err.Error(), "deadline exceeded")
}

// StillRunning returns the guests from guests that are still up, asking an adapter only
// when it has one of them.
func (c *guestRebootHost) StillRunning(ctx context.Context, guests []rebootGuest) ([]rebootGuest, error) {
	byKind := map[string]map[string]rebootGuest{}
	for _, guest := range guests {
		if byKind[guest.Kind] == nil {
			byKind[guest.Kind] = map[string]rebootGuest{}
		}
		byKind[guest.Kind][guest.ID] = guest
	}
	var out []rebootGuest
	for _, a := range c.adapters {
		wanted := byKind[a.kind()]
		if len(wanted) == 0 {
			continue
		}
		up, err := a.running(ctx)
		if err != nil {
			return nil, err
		}
		for _, guest := range up {
			if original, ok := wanted[guest.ID]; ok {
				out = append(out, original)
			}
		}
	}
	return out, nil
}

// drainAdapter reaches one plugin's reboot-drain routes in-process, as the reboot's
// principal, so the plugin's own permission middleware judges the call as it would a
// browser's, and only the plugin knows its guests, their state and its intent ledger.
type drainAdapter struct {
	decl      plugincaps.Capability
	plugins   pluginRouteServer
	envID     string
	principal *auth.User
}

func (a *drainAdapter) kind() string { return a.decl.SubjectType }

// drainGuest is one entry of a drain's running list.
type drainGuest struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Graceful bool   `json:"graceful"`
}

// drainStop is a drain's answer to a stop request.
type drainStop struct {
	Escalated bool `json:"escalated"`
}

func (a *drainAdapter) running(ctx context.Context) ([]rebootGuest, error) {
	listCtx, cancel := context.WithTimeout(ctx, drainListTimeout)
	defer cancel()
	var listed []drainGuest
	if err := a.call(listCtx, http.MethodGet, a.decl.Routes["running"], nil, &listed); err != nil {
		return nil, err
	}
	out := make([]rebootGuest, 0, len(listed))
	for _, g := range listed {
		if g.ID == "" {
			continue
		}
		out = append(out, rebootGuest{Kind: a.kind(), ID: g.ID, Name: g.Name, Graceful: g.Graceful})
	}
	return out, nil
}

func (a *drainAdapter) requestShutdown(ctx context.Context, g rebootGuest) (bool, error) {
	stopCtx, cancel := context.WithTimeout(ctx, guestRequestTimeout)
	defer cancel()
	body, err := json.Marshal(map[string]string{"id": g.ID})
	if err != nil {
		return false, fmt.Errorf("encoding the stop request: %w", err)
	}
	var answer drainStop
	err = a.call(stopCtx, http.MethodPost, a.decl.Routes["stop"], body, &answer)
	if stopCtx.Err() != nil {
		// The plugin gave up because this side stopped listening; the guest's grace keeps
		// running on the agent, which StillRunning then watches.
		return false, stopCtx.Err()
	}
	return answer.Escalated, err
}

// drainResponse is a bounded in-memory response, so a plugin cannot make a drain hold more
// than it will ever read.
type drainResponse struct {
	header   http.Header
	status   int
	body     bytes.Buffer
	overflow bool
}

func (d *drainResponse) Header() http.Header { return d.header }

func (d *drainResponse) WriteHeader(status int) {
	if d.status == 0 {
		d.status = status
	}
}

func (d *drainResponse) Write(p []byte) (int, error) {
	if d.status == 0 {
		d.status = http.StatusOK
	}
	if d.body.Len()+len(p) > maxDrainResponse {
		d.overflow = true
		return len(p), nil
	}
	return d.body.Write(p)
}

// drainEnvelope is the envelope every plugin route answers with.
type drainEnvelope struct {
	Success bool            `json:"success"`
	Data    json.RawMessage `json:"data"`
	Code    string          `json:"code"`
	Error   string          `json:"error"`
}

// call serves one drain route the way the /api mount would hand it to the plugin router:
// absolute URL, route path relative to /api, the environment as ?env=, and the principal as
// the authenticated user.
func (a *drainAdapter) call(ctx context.Context, method, path string, body []byte, into any) error {
	if a.plugins == nil {
		return fmt.Errorf("%s: the plugin router is not wired", a.decl.Plugin)
	}
	if a.principal == nil || a.principal.ID == "" {
		return fmt.Errorf("%s: a drain call needs the reboot's principal", a.decl.Plugin)
	}
	ctx = context.WithValue(ctx, auth.UserContextKey, a.principal)
	rctx := chi.NewRouteContext()
	rctx.RoutePath = strings.TrimPrefix(path, "/api")
	ctx = context.WithValue(ctx, chi.RouteCtxKey, rctx)
	target := path + "?" + url.Values{"env": {a.envID}}.Encode()
	req, err := http.NewRequestWithContext(ctx, method, target, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("building the %s drain request: %w", a.decl.Plugin, err)
	}
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	rec := &drainResponse{header: http.Header{}}
	a.plugins.ServeHTTP(rec, req)
	if rec.status == 0 {
		rec.status = http.StatusOK
	}
	if rec.overflow {
		return fmt.Errorf("%s answered its drain route with more than %d bytes", a.decl.Plugin, maxDrainResponse)
	}
	raw := rec.body.Bytes()
	var env drainEnvelope
	if err := json.Unmarshal(raw, &env); err != nil || rec.status >= http.StatusBadRequest || !env.Success {
		detail := strings.TrimSpace(env.Code + " " + env.Error)
		if detail == "" {
			detail = strings.TrimSpace(string(raw))
		}
		return fmt.Errorf("%s refused its %s drain route (%d): %s", a.decl.Plugin, method, rec.status, detail)
	}
	if into == nil || len(env.Data) == 0 {
		return nil
	}
	if err := json.Unmarshal(env.Data, into); err != nil {
		return fmt.Errorf("%s answered its drain route with an unreadable body: %w", a.decl.Plugin, err)
	}
	return nil
}

// kubernetesProbeScript looks for a kubelet among the host's processes. The
// utility container shares the host's PID namespace, so no chroot is needed.
const kubernetesProbeScript = `for p in kubelet kubelite k3s-server k3s-agent; do if pgrep -x "$p" >/dev/null 2>&1; then echo "MCHARBOR_K8S=$p"; fi; done`

// uptimeScript reads the host's uptime, which is how a finished reboot is told
// apart from a host that merely stayed reachable.
const uptimeScript = `chroot /host cat /proc/uptime`

// rebootCommandScript asks the host's systemd to reboot ten seconds from now, so
// the command returns and its container is removed before shutdown begins.
// systemctl reboot stops every unit in order; nothing is forced.
func rebootCommandScript() string {
	unit := "mcharbor-reboot-" + strconv.FormatInt(time.Now().Unix(), 10)
	return `chroot /host systemd-run --on-active=10 --unit=` + unit +
		` --description=McHarbor-scheduled-reboot /bin/sh -c "systemctl reboot"`
}

// parseKubernetesProbe returns the kubelet process names the probe found.
func parseKubernetesProbe(output string) []string {
	var found []string
	for _, line := range strings.Split(output, "\n") {
		if name, ok := strings.CutPrefix(strings.TrimSpace(line), "MCHARBOR_K8S="); ok && name != "" {
			found = append(found, name)
		}
	}
	return found
}

// parseUptime reads the first field of /proc/uptime.
func parseUptime(output string) (time.Duration, error) {
	fields := strings.Fields(output)
	if len(fields) == 0 {
		return 0, fmt.Errorf("empty uptime output")
	}
	seconds, err := strconv.ParseFloat(fields[0], 64)
	if err != nil {
		return 0, fmt.Errorf("parsing uptime: %w", err)
	}
	return time.Duration(seconds * float64(time.Second)), nil
}
