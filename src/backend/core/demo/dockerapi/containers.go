// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package dockerapi

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/netip"
	"strconv"
	"strings"
	"time"

	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/api/types/mount"
	"github.com/moby/moby/api/types/network"

	"github.com/therealmcsparrow/mcharbor/core/demo/estate"
)

func (s *Server) containerSummary(c estate.Container) container.Summary {
	ports := make([]container.PortSummary, 0, len(c.Ports))
	for _, p := range c.Ports {
		port := container.PortSummary{PrivatePort: uint16(p.Private), Type: p.Proto}
		if p.Public > 0 {
			port.IP = netip.IPv4Unspecified()
			port.PublicPort = uint16(p.Public)
		}
		ports = append(ports, port)
	}
	nets := map[string]*network.EndpointSettings{}
	for name, ip := range c.Networks {
		nets[name] = &network.EndpointSettings{NetworkID: name, IPAddress: ipAddr(ip), IPPrefixLen: 16, Gateway: ipAddr(gatewayOf(ip)), MacAddress: hwAddr(macOf(c.ID + name))}
	}
	mounts := make([]container.MountPoint, 0, len(c.Mounts))
	for _, m := range c.Mounts {
		mounts = append(mounts, container.MountPoint{Type: mountType(m.Type), Name: m.Name, Source: m.Source, Destination: m.Destination, Driver: "local", RW: m.RW, Propagation: ""})
	}
	sum := container.Summary{
		ID:              c.ID,
		Names:           []string{"/" + c.Name},
		Image:           c.Image,
		ImageID:         c.ImageID,
		Command:         c.Command,
		Created:         c.Created.Unix(),
		Ports:           ports,
		Labels:          c.Labels,
		State:           container.ContainerState(c.State),
		Status:          statusText(c, s.now()),
		NetworkSettings: &container.NetworkSettingsSummary{Networks: nets},
		Mounts:          mounts,
	}
	sum.HostConfig.NetworkMode = firstNetwork(c)
	return sum
}

func firstNetwork(c estate.Container) string {
	for name := range c.Networks {
		return name
	}
	return "bridge"
}

func gatewayOf(ip string) string {
	parts := strings.Split(ip, ".")
	if len(parts) != 4 {
		return ""
	}
	return parts[0] + "." + parts[1] + ".0.1"
}

func macOf(seed string) string {
	h := estate.StableHash(seed)
	return fmt.Sprintf("02:42:%02x:%02x:%02x:%02x", byte(h>>24), byte(h>>16), byte(h>>8), byte(h))
}

// ipAddr parses a fixture address; the estate stores addresses as strings, and a
// blank one must stay the zero Addr so it serialises the way an unset field does.
func ipAddr(s string) netip.Addr {
	a, err := netip.ParseAddr(s)
	if err != nil {
		return netip.Addr{}
	}
	return a
}

// ipPrefix is ipAddr for a CIDR.
func ipPrefix(s string) netip.Prefix {
	p, err := netip.ParsePrefix(s)
	if err != nil {
		return netip.Prefix{}
	}
	return p
}

// hwAddr parses a fixture MAC address, leaving a malformed one unset.
func hwAddr(s string) network.HardwareAddr {
	hw, err := net.ParseMAC(s)
	if err != nil {
		return nil
	}
	return network.HardwareAddr(hw)
}

func statusText(c estate.Container, now time.Time) string {
	switch c.State {
	case estate.StateRunning:
		s := "Up " + humanDuration(now.Sub(c.StartedAt))
		if c.Health != "" {
			s += " (" + c.Health + ")"
		}
		return s
	case estate.StatePaused:
		return "Up " + humanDuration(now.Sub(c.StartedAt)) + " (Paused)"
	case estate.StateRestarting:
		return "Restarting (" + strconv.Itoa(c.ExitCode) + ") " + humanDuration(time.Duration(c.RestartCount%50+1)*time.Second) + " ago"
	case estate.StateCreated:
		return "Created"
	default:
		return "Exited (" + strconv.Itoa(c.ExitCode) + ") " + humanDuration(now.Sub(c.FinishedAt)) + " ago"
	}
}

func humanDuration(d time.Duration) string {
	switch {
	case d < time.Minute:
		return "Less than a second"
	case d < 2*time.Minute:
		return "About a minute"
	case d < time.Hour:
		return strconv.Itoa(int(d.Minutes())) + " minutes"
	case d < 2*time.Hour:
		return "About an hour"
	case d < 48*time.Hour:
		return strconv.Itoa(int(d.Hours())) + " hours"
	case d < 14*24*time.Hour:
		return strconv.Itoa(int(d.Hours()/24)) + " days"
	case d < 60*24*time.Hour:
		return strconv.Itoa(int(d.Hours()/24/7)) + " weeks"
	default:
		return strconv.Itoa(int(d.Hours()/24/30)) + " months"
	}
}

func mountType(t string) mount.Type { return mount.Type(t) }

func (s *Server) containers(w http.ResponseWriter, r *http.Request, rest string) {
	switch {
	case rest == "/json" && r.Method == http.MethodGet:
		s.listContainers(w, r)
		return
	case rest == "/create" && r.Method == http.MethodPost:
		s.createContainer(w, r)
		return
	case rest == "/prune" && r.Method == http.MethodPost:
		ids, reclaimed := s.host.PruneContainers()
		writeJSON(w, http.StatusOK, container.PruneReport{ContainersDeleted: ids, SpaceReclaimed: uint64(reclaimed)})
		return
	}
	parts := strings.SplitN(strings.TrimPrefix(rest, "/"), "/", 2)
	id := parts[0]
	action := ""
	if len(parts) == 2 {
		action = parts[1]
	}
	h := s.host
	var err error
	switch {
	case action == "" && r.Method == http.MethodDelete:
		err = h.RemoveContainer(id, queryBool(r, "force"))
		if err == nil {
			w.WriteHeader(http.StatusNoContent)
			return
		}
	case action == "json" && r.Method == http.MethodGet:
		c, gerr := h.Container(id)
		if gerr == nil {
			writeJSON(w, http.StatusOK, s.inspectContainer(c))
			return
		}
		err = gerr
	case action == "start":
		_, err = h.StartContainer(id)
	case action == "stop":
		_, err = h.StopContainer(id)
	case action == "restart":
		_, err = h.RestartContainer(id)
	case action == "kill":
		_, err = h.KillContainer(id)
	case action == "pause":
		_, err = h.PauseContainer(id)
	case action == "unpause":
		_, err = h.UnpauseContainer(id)
	case action == "rename":
		_, err = h.RenameContainer(id, r.URL.Query().Get("name"))
	case action == "update":
		if _, gerr := h.Container(id); gerr != nil {
			err = gerr
			break
		}
		writeJSON(w, http.StatusOK, container.UpdateResponse{Warnings: []string{}})
		return
	case action == "wait":
		c, gerr := h.Container(id)
		if gerr != nil {
			err = gerr
			break
		}
		writeJSON(w, http.StatusOK, container.WaitResponse{StatusCode: int64(c.ExitCode)})
		return
	case action == "logs":
		s.logs(w, r, id)
		return
	case action == "stats":
		s.stats(w, r, id)
		return
	case action == "top":
		s.top(w, id)
		return
	case action == "changes":
		writeJSON(w, http.StatusOK, []container.FilesystemChange{{Kind: container.ChangeModify, Path: "/tmp"}, {Kind: container.ChangeAdd, Path: "/tmp/.cache"}})
		return
	case action == "exec" && r.Method == http.MethodPost:
		s.createExec(w, r, id)
		return
	case action == "resize":
		w.WriteHeader(http.StatusOK)
		return
	default:
		notInDemo(w)
		return
	}
	if err != nil {
		writeEstateError(w, err, "container: "+id)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) listContainers(w http.ResponseWriter, r *http.Request) {
	all := queryBool(r, "all")
	f := parseFilters(r)
	limit := atoiOr(r.URL.Query().Get("limit"), 0)
	out := []container.Summary{}
	for _, c := range s.host.Containers() {
		if !all && len(f["status"]) == 0 && c.State != estate.StateRunning && c.State != estate.StateRestarting && c.State != estate.StatePaused {
			continue
		}
		if st := f["status"]; len(st) > 0 && !contains(st, string(c.State)) {
			continue
		}
		if !labelsMatch(c.Labels, f["label"]) {
			continue
		}
		if names := f["name"]; len(names) > 0 && !anySubstring(c.Name, names) {
			continue
		}
		if ids := f["id"]; len(ids) > 0 && !anyPrefix(c.ID, ids) {
			continue
		}
		if hs := f["health"]; len(hs) > 0 && !contains(hs, c.Health) {
			continue
		}
		sum := s.containerSummary(c)
		if queryBool(r, "size") {
			sum.SizeRw = c.SizeRw
			sum.SizeRootFs = c.SizeRw + 120<<20
		}
		out = append(out, sum)
		if limit > 0 && len(out) >= limit {
			break
		}
	}
	writeJSON(w, http.StatusOK, out)
}

func anySubstring(s string, subs []string) bool {
	for _, sub := range subs {
		if strings.Contains(s, strings.TrimPrefix(sub, "/")) {
			return true
		}
	}
	return false
}

func anyPrefix(s string, prefixes []string) bool {
	for _, p := range prefixes {
		if strings.HasPrefix(s, p) {
			return true
		}
	}
	return false
}

func (s *Server) inspectContainer(c estate.Container) container.InspectResponse {
	now := s.now()
	state := &container.State{
		Status:     container.ContainerState(c.State),
		Running:    c.State == estate.StateRunning || c.State == estate.StatePaused,
		Paused:     c.State == estate.StatePaused,
		Restarting: c.State == estate.StateRestarting,
		OOMKilled:  strings.Contains(c.Error, "OOMKilled"),
		ExitCode:   c.ExitCode,
		Error:      c.Error,
		StartedAt:  c.StartedAt.UTC().Format(time.RFC3339Nano),
		FinishedAt: "0001-01-01T00:00:00Z",
	}
	if state.Running {
		state.Pid = int(estate.StableHash(c.ID)%60000) + 1000
	}
	if !c.FinishedAt.IsZero() {
		state.FinishedAt = c.FinishedAt.UTC().Format(time.RFC3339Nano)
	}
	if c.Health != "" {
		state.Health = &container.Health{Status: container.HealthStatus(c.Health), FailingStreak: failing(c.Health), Log: healthLog(c, now)}
	}
	exposed := network.PortSet{}
	bindings := network.PortMap{}
	netPorts := network.PortMap{}
	for _, p := range c.Ports {
		port, ok := network.PortFrom(uint16(p.Private), network.IPProtocol(p.Proto))
		if !ok {
			continue
		}
		exposed[port] = struct{}{}
		if p.Public > 0 {
			b := []network.PortBinding{{HostIP: netip.IPv4Unspecified(), HostPort: strconv.Itoa(p.Public)}}
			bindings[port] = b
			netPorts[port] = b
		} else {
			netPorts[port] = nil
		}
	}
	endpoints := map[string]*network.EndpointSettings{}
	for name, ip := range c.Networks {
		endpoints[name] = &network.EndpointSettings{NetworkID: name, EndpointID: fmt.Sprintf("%064x", estate.StableHash(c.ID+name)), Gateway: ipAddr(gatewayOf(ip)), IPAddress: ipAddr(ip), IPPrefixLen: 16, MacAddress: hwAddr(macOf(c.ID + name)), Aliases: []string{c.Labels["com.docker.compose.service"], c.Name}}
	}
	var binds []string
	mounts := make([]container.MountPoint, 0, len(c.Mounts))
	for _, m := range c.Mounts {
		mounts = append(mounts, container.MountPoint{Type: mountType(m.Type), Name: m.Name, Source: m.Source, Destination: m.Destination, Driver: "local", Mode: "z", RW: m.RW})
		if m.Type == "volume" {
			binds = append(binds, m.Name+":"+m.Destination+":rw")
		}
	}
	args := strings.Fields(c.Command)
	path := ""
	if len(args) > 0 {
		path, args = args[0], args[1:]
	}
	hostCfg := &container.HostConfig{
		Binds:         binds,
		NetworkMode:   container.NetworkMode(firstNetwork(c)),
		PortBindings:  bindings,
		RestartPolicy: container.RestartPolicy{Name: container.RestartPolicyMode(c.RestartPolicy)},
		LogConfig:     container.LogConfig{Type: "json-file", Config: map[string]string{"max-size": "10m", "max-file": "3"}},
	}
	hostCfg.Memory = c.MemLimit
	return container.InspectResponse{
		ID:             c.ID,
		Created:        c.Created.UTC().Format(time.RFC3339Nano),
		Path:           path,
		Args:           args,
		State:          state,
		Image:          c.ImageID,
		ResolvConfPath: "/var/lib/docker/containers/" + c.ID + "/resolv.conf",
		HostnamePath:   "/var/lib/docker/containers/" + c.ID + "/hostname",
		HostsPath:      "/var/lib/docker/containers/" + c.ID + "/hosts",
		LogPath:        "/var/lib/docker/containers/" + c.ID + "/" + c.ID + "-json.log",
		Name:           "/" + c.Name,
		RestartCount:   c.RestartCount,
		Driver:         "overlay2",
		Platform:       "linux",
		HostConfig:     hostCfg,
		Mounts:         mounts,
		Config: &container.Config{
			Hostname:     c.Hostname,
			User:         c.User,
			ExposedPorts: exposed,
			Env:          c.Env,
			Cmd:          strings.Fields(c.Command),
			Image:        c.Image,
			WorkingDir:   c.WorkingDir,
			Labels:       c.Labels,
		},
		NetworkSettings: &container.NetworkSettings{
			SandboxID: fmt.Sprintf("%064x", estate.StableHash("sb"+c.ID)),
			Ports:     netPorts,
			Networks:  endpoints,
		},
	}
}

func failing(health string) int {
	if health == "unhealthy" {
		return 3
	}
	return 0
}

func healthLog(c estate.Container, now time.Time) []*container.HealthcheckResult {
	out := []*container.HealthcheckResult{}
	for i := 4; i >= 0; i-- {
		start := now.Add(-time.Duration(i*30) * time.Second)
		res := &container.HealthcheckResult{Start: start, End: start.Add(40 * time.Millisecond), ExitCode: 0, Output: "ok\n"}
		if c.Health == "unhealthy" {
			res.ExitCode = 1
			res.Output = "wget: can't connect to remote host (127.0.0.1): Connection refused\n"
		}
		out = append(out, res)
	}
	return out
}

// logs writes the container's output in the multiplexed stream format a
// non-TTY container produces, and keeps writing new lines while following.
func (s *Server) logs(w http.ResponseWriter, r *http.Request, id string) {
	c, err := s.host.Container(id)
	if err != nil {
		writeEstateError(w, err, "container: "+id)
		return
	}
	q := r.URL.Query()
	tail := 200
	if t := q.Get("tail"); t != "" && t != "all" {
		if n, perr := strconv.Atoi(t); perr == nil {
			tail = n
		}
	} else if t == "all" {
		tail = 1000
	}
	stamps := queryBool(r, "timestamps")
	wantOut := q.Get("stdout") == "" || queryBool(r, "stdout")
	wantErr := q.Get("stderr") == "" || queryBool(r, "stderr")
	w.Header().Set("Content-Type", "application/vnd.docker.multiplexed-stream")
	w.WriteHeader(http.StatusOK)
	write := func(line estate.LogLine) bool {
		if (line.Stderr && !wantErr) || (!line.Stderr && !wantOut) {
			return true
		}
		text := line.Text + "\n"
		if stamps {
			text = line.Time.UTC().Format(time.RFC3339Nano) + " " + text
		}
		stream := byte(1)
		if line.Stderr {
			stream = 2
		}
		frame := make([]byte, 8, 8+len(text))
		frame[0] = stream
		binary.BigEndian.PutUint32(frame[4:], uint32(len(text)))
		frame = append(frame, text...)
		_, werr := w.Write(frame)
		return werr == nil
	}
	now := s.now()
	for _, line := range s.host.LogLines(c, now, tail) {
		if !write(line) {
			return
		}
	}
	flush(w)
	if !queryBool(r, "follow") || c.State != estate.StateRunning {
		return
	}
	ticker := time.NewTicker(3 * time.Second)
	defer ticker.Stop()
	last := now
	for {
		select {
		case <-r.Context().Done():
			return
		case t := <-ticker.C:
			for _, line := range s.host.LogLines(c, t, 2) {
				if line.Time.After(last) {
					if !write(line) {
						return
					}
					last = line.Time
				}
			}
			flush(w)
		}
	}
}

func (s *Server) statsDoc(c estate.Container, t time.Time) container.StatsResponse {
	cur := s.host.SampleContainer(c, t)
	cpus := uint32(max(1, s.host.CPUs))
	sysNow := uint64(t.UnixNano()) * uint64(cpus)
	sysPrev := uint64(t.Add(-time.Second).UnixNano()) * uint64(cpus)
	usage := func(pct float64, base uint64) uint64 { return base + uint64(pct/100*1e9) }
	doc := container.StatsResponse{
		Name: "/" + c.Name,
		ID:   c.ID,
	}
	doc.Read = t
	doc.PreRead = t.Add(-time.Second)
	doc.CPUStats = container.CPUStats{CPUUsage: container.CPUUsage{TotalUsage: usage(cur.CPUPercent, cur.CPUTotalNS)}, SystemUsage: sysNow, OnlineCPUs: cpus}
	doc.PreCPUStats = container.CPUStats{CPUUsage: container.CPUUsage{TotalUsage: cur.CPUTotalNS}, SystemUsage: sysPrev, OnlineCPUs: cpus}
	doc.MemoryStats = container.MemoryStats{Usage: uint64(cur.MemUsed), Limit: uint64(max(cur.MemLimit, 1)), Stats: map[string]uint64{"inactive_file": uint64(cur.MemUsed / 20)}}
	doc.PidsStats = container.PidsStats{Current: uint64(cur.PIDs), Limit: 4096}
	doc.Networks = map[string]container.NetworkStats{"eth0": {RxBytes: uint64(cur.NetRx), TxBytes: uint64(cur.NetTx), RxPackets: uint64(cur.NetRx / 900), TxPackets: uint64(cur.NetTx / 900)}}
	doc.BlkioStats = container.BlkioStats{IoServiceBytesRecursive: []container.BlkioStatEntry{{Major: 8, Op: "read", Value: uint64(cur.BlockRead)}, {Major: 8, Op: "write", Value: uint64(cur.BlockWrite)}}}
	return doc
}

func (s *Server) stats(w http.ResponseWriter, r *http.Request, id string) {
	c, err := s.host.Container(id)
	if err != nil {
		writeEstateError(w, err, "container: "+id)
		return
	}
	stream := r.URL.Query().Get("stream")
	if stream == "0" || stream == "false" || queryBool(r, "one-shot") {
		writeJSON(w, http.StatusOK, s.statsDoc(c, s.now()))
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	enc := json.NewEncoder(w)
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		if cur, gerr := s.host.Container(id); gerr == nil {
			c = cur
		}
		if err := enc.Encode(s.statsDoc(c, s.now())); err != nil {
			return
		}
		flush(w)
		select {
		case <-r.Context().Done():
			return
		case <-ticker.C:
		}
	}
}

func (s *Server) top(w http.ResponseWriter, id string) {
	c, err := s.host.Container(id)
	if err != nil {
		writeEstateError(w, err, "container: "+id)
		return
	}
	if c.State != estate.StateRunning {
		writeError(w, http.StatusConflict, "Container "+id+" is not running")
		return
	}
	pid := int(estate.StableHash(c.ID)%60000) + 1000
	procs := [][]string{{"root", strconv.Itoa(pid), "1", "0", "Sep27", "?", "00:01:12", c.Command}}
	workers := int(estate.StableHash(c.ID)%5) + 1
	for i := 1; i <= workers; i++ {
		procs = append(procs, []string{"1000", strconv.Itoa(pid + i), strconv.Itoa(pid), "0", "Sep27", "?", "00:00:" + fmt.Sprintf("%02d", 10+i*7), c.Command + " (worker " + strconv.Itoa(i) + ")"})
	}
	writeJSON(w, http.StatusOK, container.TopResponse{Titles: []string{"UID", "PID", "PPID", "C", "STIME", "TTY", "TIME", "CMD"}, Processes: procs})
}

type createBody struct {
	container.Config
	HostConfig       *container.HostConfig     `json:"HostConfig"`
	NetworkingConfig *network.NetworkingConfig `json:"NetworkingConfig"`
}

func (s *Server) createContainer(w http.ResponseWriter, r *http.Request) {
	var body createBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid container config")
		return
	}
	c := &estate.Container{
		Name:          r.URL.Query().Get("name"),
		Image:         body.Image,
		Command:       strings.Join(append(append([]string{}, body.Entrypoint...), body.Cmd...), " "),
		Env:           body.Env,
		Labels:        body.Labels,
		Hostname:      body.Hostname,
		User:          body.User,
		WorkingDir:    body.WorkingDir,
		Profile:       "api",
		CPUBase:       2,
		MemBase:       96 << 20,
		MemLimit:      512 << 20,
		RestartPolicy: "no",
		Networks:      map[string]string{},
	}
	if body.HostConfig != nil {
		c.RestartPolicy = string(body.HostConfig.RestartPolicy.Name)
		if body.HostConfig.Memory > 0 {
			c.MemLimit = body.HostConfig.Memory
		}
		for port, binds := range body.HostConfig.PortBindings {
			p := estate.PortBinding{Private: int(port.Num()), Proto: string(port.Proto())}
			if len(binds) > 0 {
				p.Public = atoiOr(binds[0].HostPort, 0)
			}
			c.Ports = append(c.Ports, p)
		}
		for _, b := range body.HostConfig.Binds {
			parts := strings.Split(b, ":")
			if len(parts) >= 2 {
				kind := "volume"
				if strings.HasPrefix(parts[0], "/") {
					kind = "bind"
				}
				c.Mounts = append(c.Mounts, estate.MountPoint{Type: kind, Name: parts[0], Source: parts[0], Destination: parts[1], RW: true})
			}
		}
	}
	if body.NetworkingConfig != nil {
		for name := range body.NetworkingConfig.EndpointsConfig {
			c.Networks[name] = fmt.Sprintf("172.%d.0.%d", 18+estate.StableHash(name)%200, 2+estate.StableHash(c.Name)%250)
		}
	}
	if len(c.Networks) == 0 {
		c.Networks["bridge"] = fmt.Sprintf("172.17.0.%d", 2+estate.StableHash(c.Name+time.Now().String())%250)
	}
	if _, err := s.host.Image(c.Image); err != nil {
		writeError(w, http.StatusNotFound, "No such image: "+c.Image)
		return
	}
	created, err := s.host.AddContainer(c)
	if err != nil {
		writeError(w, http.StatusConflict, "Conflict. The container name \"/"+c.Name+"\" is already in use.")
		return
	}
	writeJSON(w, http.StatusCreated, container.CreateResponse{ID: created.ID, Warnings: []string{}})
}

func (s *Server) createExec(w http.ResponseWriter, r *http.Request, id string) {
	var body container.ExecCreateRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid exec config")
		return
	}
	e, err := s.host.CreateExec(id, body.Cmd, body.Tty)
	if err != nil {
		writeEstateError(w, err, "container: "+id)
		return
	}
	writeJSON(w, http.StatusCreated, container.ExecCreateResponse{ID: e.ID})
}

// exec answers exec start and inspect. A detached start is what shell
// detection uses, so it completes at once with exit 0 for any shell probe.
func (s *Server) exec(w http.ResponseWriter, r *http.Request, rest string) {
	id, action, _ := strings.Cut(rest, "/")
	switch action {
	case "start":
		if _, err := s.host.Exec(id); err != nil {
			writeEstateError(w, err, "exec instance: "+id)
			return
		}
		s.host.FinishExec(id, 0)
		w.Header().Set("Content-Type", "application/vnd.docker.raw-stream")
		w.WriteHeader(http.StatusOK)
	case "json":
		e, err := s.host.Exec(id)
		if err != nil {
			writeEstateError(w, err, "exec instance: "+id)
			return
		}
		exitCode := e.ExitCode
		writeJSON(w, http.StatusOK, container.ExecInspectResponse{
			ID:          e.ID,
			ContainerID: e.ContainerID,
			Running:     e.Running,
			ExitCode:    &exitCode,
			Pid:         int(estate.StableHash(e.ID) % 60000),
		})
	case "resize":
		w.WriteHeader(http.StatusOK)
	default:
		notInDemo(w)
	}
}
