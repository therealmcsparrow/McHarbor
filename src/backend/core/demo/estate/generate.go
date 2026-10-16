// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package estate

import (
	"fmt"
	"sort"
	"strings"
	"time"
)

// DefaultSeed is the seed the demo estate is built from; a fixed value is what
// makes the estate the same after every restart without persisting it.
const DefaultSeed uint64 = 0x6d63686172626f72

var instanceSuffixes = []string{"b2b", "canary", "mobile", "partners", "legacy", "v2", "blue", "green", "internal", "eu", "us", "apac"}

var distroInfo = map[string][3]string{
	"ubuntu": {"Ubuntu 24.04.1 LTS", "24.04", "6.8.0-45-generic"},
	"debian": {"Debian GNU/Linux 12 (bookworm)", "12", "6.1.0-25-amd64"},
	"rocky":  {"Rocky Linux 9.4 (Blue Onyx)", "9.4", "5.14.0-427.33.1.el9_4.x86_64"},
	"fedora": {"Fedora Linux 40 (Server Edition)", "40", "6.10.11-200.fc40.x86_64"},
	"alpine": {"Alpine Linux v3.20", "3.20.3", "6.6.52-0-lts"},
}

// Build creates the whole estate from seed. now anchors every relative
// timestamp, so uptimes and ages look plausible whenever the demo starts.
func Build(seed uint64, now time.Time) *Estate {
	e := &Estate{byKey: make(map[string]*Host)}
	for i, spec := range hostSpecs {
		h := buildHost(spec, seed+uint64(i)*7919, now)
		e.Hosts = append(e.Hosts, h)
		e.byKey[h.Key] = h
	}
	return e
}

// ContainerCount returns how many containers the estate holds across hosts.
func (e *Estate) ContainerCount() int {
	n := 0
	for _, h := range e.Hosts {
		h.mu.RLock()
		n += len(h.containers)
		h.mu.RUnlock()
	}
	return n
}

func buildHost(spec hostSpec, seed uint64, now time.Time) *Host {
	r := newRNG(seed)
	h := newHost(spec.key)
	h.seed = seed
	h.Name = spec.name
	h.Region = spec.region
	h.Role = spec.role
	h.Hostname = spec.key + ".acme.internal"
	info := distroInfo[spec.distro]
	h.OS = "linux"
	h.Distro = spec.distro
	h.OSVersion = info[1]
	h.Kernel = info[2]
	if spec.arch == "aarch64" {
		h.Kernel = strings.Replace(strings.Replace(h.Kernel, "amd64", "arm64", 1), "x86_64", "aarch64", 1)
	}
	h.Arch = spec.arch
	h.CPUs = spec.cpus
	h.MemTotal = int64(spec.memGB) << 30
	h.DiskTotal = int64(spec.diskTB) << 40
	h.DiskUsed = h.DiskTotal / 100 * int64(r.between(28, 71))
	h.DockerVersion = pick(r, []string{"27.3.1", "27.2.1", "26.1.4"})
	h.BootedAt = now.Add(-time.Duration(r.between(3, 140)) * 24 * time.Hour)
	h.Compute = spec.compute

	addBaseNetworks(h, r, now)
	used := make(map[string]int)
	count := 0
	for round := 0; count < spec.target; round++ {
		for _, tplName := range spec.stacks {
			if count >= spec.target {
				break
			}
			tpl := stackTemplates[tplName]
			project := tpl.name
			if n := used[tpl.name]; n > 0 {
				project = tpl.name + "-" + instanceSuffixes[(n-1)%len(instanceSuffixes)]
				if n > len(instanceSuffixes) {
					project += itoa(n / len(instanceSuffixes))
				}
			}
			used[tpl.name]++
			count += addStack(h, r, spec, project, tpl, now)
		}
		if round > 40 {
			break
		}
	}
	addLeftovers(h, r, now)
	return h
}

func addBaseNetworks(h *Host, r *rng, now time.Time) {
	for _, n := range []struct{ name, driver, subnet string }{
		{"bridge", "bridge", "172.17.0.0/16"},
		{"host", "host", ""},
		{"none", "null", ""},
	} {
		gw := ""
		if n.subnet != "" {
			gw = strings.TrimSuffix(n.subnet, "0.0/16") + "0.1"
		}
		net := &Network{ID: r.hex(64), Name: n.name, Driver: n.driver, Scope: "local", Subnet: n.subnet, Gateway: gw, Created: h.BootedAt}
		h.networks[net.ID] = net
	}
}

func imageFor(h *Host, ref string, r *rng, now time.Time) *Image {
	if img := h.imageByRef(ref); img != nil {
		return img
	}
	ir := newRNG(hashString(ref))
	img := &Image{
		ID:       "sha256:" + ir.hex(64),
		RepoTags: []string{normalizeRef(ref)},
		Digest:   "sha256:" + ir.hex(64),
		Created:  now.Add(-time.Duration(ir.between(5, 200)) * 24 * time.Hour),
		Size:     int64(ir.between(18, 1400)) << 20,
		Arch:     map[string]string{"x86_64": "amd64", "aarch64": "arm64"}[h.Arch],
		OS:       "linux",
		Labels:   map[string]string{"org.opencontainers.image.source": "https://" + strings.Split(ref, ":")[0]},
	}
	h.images[img.ID] = img
	return img
}

func addStack(h *Host, r *rng, spec hostSpec, project string, tpl stackTemplate, now time.Time) int {
	netName := project + "_default"
	subnetOctet := 18 + len(h.networks)%230
	netw := &Network{
		ID:      r.hex(64),
		Name:    netName,
		Driver:  "bridge",
		Scope:   "local",
		Subnet:  fmt.Sprintf("172.%d.0.0/16", subnetOctet),
		Gateway: fmt.Sprintf("172.%d.0.1", subnetOctet),
		Created: now.Add(-time.Duration(r.between(10, 300)) * 24 * time.Hour),
		Labels:  map[string]string{"com.docker.compose.project": project, "com.docker.compose.network": "default"},
	}
	h.networks[netw.ID] = netw
	stackAge := time.Duration(r.between(2, 280)) * 24 * time.Hour
	added := 0
	var compose strings.Builder
	compose.WriteString("name: " + project + "\n\nservices:\n")
	volumes := []string{}
	for _, svc := range tpl.services {
		replicas := svc.replicas
		if replicas == 0 {
			replicas = 1
		}
		if spec.role == "edge" || spec.role == "staging" || spec.role == "development" {
			replicas = max(1, replicas/2)
		}
		img := imageFor(h, svc.image, r, now)
		volName := ""
		if svc.mount != "" {
			volName = project + "_" + svc.name + "-data"
			h.volumes[volName] = &Volume{
				Name:      volName,
				Driver:    "local",
				Created:   now.Add(-stackAge),
				Labels:    map[string]string{"com.docker.compose.project": project, "com.docker.compose.volume": svc.name + "-data"},
				SizeBytes: int64(r.between(40, 90000)) << 20,
			}
			volumes = append(volumes, svc.name+"-data")
		}
		writeComposeService(&compose, svc, replicas)
		for i := 1; i <= replicas; i++ {
			c := newStackContainer(h, r, spec, project, svc, i, img, volName, netw, now, stackAge)
			h.containers[c.ID] = c
			h.byName[c.Name] = c.ID
			added++
		}
	}
	if len(volumes) > 0 {
		compose.WriteString("\nvolumes:\n")
		for _, v := range volumes {
			compose.WriteString("  " + v + ":\n")
		}
	}
	h.stacks[project] = &Stack{Name: project, Compose: compose.String(), Env: map[string]string{"COMPOSE_PROJECT_NAME": project, "TZ": "Europe/Amsterdam"}}
	return added
}

func writeComposeService(b *strings.Builder, svc serviceTemplate, replicas int) {
	b.WriteString("  " + svc.name + ":\n")
	b.WriteString("    image: " + svc.image + "\n")
	if svc.command != "" {
		b.WriteString("    command: " + svc.command + "\n")
	}
	b.WriteString("    restart: unless-stopped\n")
	if replicas > 1 {
		fmt.Fprintf(b, "    deploy:\n      replicas: %d\n", replicas)
	}
	if len(svc.env) > 0 {
		b.WriteString("    environment:\n")
		for _, e := range svc.env {
			b.WriteString("      - " + e + "\n")
		}
	}
	if svc.publish && svc.port > 0 && replicas == 1 {
		fmt.Fprintf(b, "    ports:\n      - \"%d:%d\"\n", svc.port, svc.port)
	}
	if svc.mount != "" {
		b.WriteString("    volumes:\n      - " + svc.name + "-data:" + svc.mount + "\n")
	}
	if svc.health {
		b.WriteString("    healthcheck:\n      test: [\"CMD-SHELL\", \"wget -qO- http://localhost:" + itoa(max(svc.port, 80)) + "/health || exit 1\"]\n      interval: 30s\n")
	}
	fmt.Fprintf(b, "    mem_limit: %dm\n", svc.memMB*2)
}

func newStackContainer(h *Host, r *rng, spec hostSpec, project string, svc serviceTemplate, n int, img *Image, volName string, netw *Network, now time.Time, stackAge time.Duration) *Container {
	name := fmt.Sprintf("%s-%s-%d", project, svc.name, n)
	created := now.Add(-stackAge + time.Duration(r.between(0, 3600))*time.Second)
	started := created.Add(time.Duration(r.between(5, 90)) * time.Second)
	if r.chance(0.6) {
		started = now.Add(-time.Duration(r.between(1, int(stackAge.Hours())+1)) * time.Hour)
	}
	c := &Container{
		ID:            r.hex(64),
		Name:          name,
		Image:         svc.image,
		ImageID:       img.ID,
		Command:       svc.command,
		Created:       created,
		State:         StateRunning,
		StartedAt:     started,
		RestartPolicy: "unless-stopped",
		Profile:       svc.profile,
		CPUBase:       svc.cpu * (0.6 + r.float()*0.8),
		MemBase:       int64(float64(svc.memMB)*(0.7+r.float()*0.5)) << 20,
		MemLimit:      int64(svc.memMB*2) << 20,
		SizeRw:        int64(r.between(0, 400)) << 20,
		Env:           append([]string{"PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin", "TZ=Europe/Amsterdam"}, svc.env...),
		Hostname:      "",
		Networks:      map[string]string{netw.Name: fmt.Sprintf("%s.%d.%d", strings.TrimSuffix(netw.Gateway, ".0.1"), r.between(0, 3), r.between(2, 250))},
		Labels: map[string]string{
			"com.docker.compose.project":              project,
			"com.docker.compose.service":              svc.name,
			"com.docker.compose.container-number":     itoa(n),
			"com.docker.compose.oneoff":               "False",
			"com.docker.compose.version":              "2.29.7",
			"com.docker.compose.project.working_dir":  "/opt/stacks/" + project,
			"com.docker.compose.project.config_files": "/opt/stacks/" + project + "/docker-compose.yml",
			"com.docker.compose.config-hash":          r.hex(64),
			"com.docker.compose.image":                img.ID,
		},
	}
	c.Hostname = c.ID[:12]
	if c.Command == "" {
		c.Command = defaultCommand(svc.profile)
	}
	if svc.port > 0 {
		p := PortBinding{Private: svc.port, Proto: "tcp"}
		if svc.publish && n == 1 {
			p.Public = svc.port
		}
		c.Ports = append(c.Ports, p)
	}
	if volName != "" {
		c.Mounts = append(c.Mounts, MountPoint{Type: "volume", Name: volName, Source: "/var/lib/docker/volumes/" + volName + "/_data", Destination: svc.mount, RW: true})
	}
	if svc.health {
		c.Health = "healthy"
		c.Labels["demo.health"] = "1"
	}
	applyStateMix(c, r, spec, now)
	return c
}

func applyStateMix(c *Container, r *rng, spec hostSpec, now time.Time) {
	stopped := 0.05
	if spec.role == "development" || spec.role == "staging" {
		stopped = 0.16
	}
	if spec.role == "ci" {
		stopped = 0.12
	}
	roll := r.float()
	switch {
	case roll < stopped:
		c.State = StateExited
		c.FinishedAt = now.Add(-time.Duration(r.between(1, 400)) * time.Hour)
		c.ExitCode = pick(r, []int{0, 0, 0, 1, 137, 143})
		if c.Health != "" {
			c.Health = "unhealthy"
		}
	case roll < stopped+0.012:
		c.State = StateRestarting
		c.ExitCode = 1
		c.RestartCount = r.between(4, 60)
		c.Error = ""
	case roll < stopped+0.022:
		c.State = StatePaused
	case roll < stopped+0.04 && c.Health != "":
		c.Health = "unhealthy"
	case roll < stopped+0.046:
		c.State = StateExited
		c.ExitCode = 1
		c.Error = pick(r, []string{
			"OCI runtime create failed: unable to start container process: exec: \"/app/start.sh\": permission denied",
			"container exited: database \"orders\" does not exist",
			"panic: runtime error: invalid memory address or nil pointer dereference",
			"Error: listen EADDRINUSE: address already in use :::3000",
			"OOMKilled: container exceeded its memory limit",
		})
		c.FinishedAt = now.Add(-time.Duration(r.between(1, 72)) * time.Hour)
	}
	if c.State == StateRunning && r.chance(0.06) {
		c.RestartCount = r.between(1, 5)
	}
}

func defaultCommand(profile string) string {
	switch profile {
	case "postgres":
		return "postgres"
	case "redis":
		return "redis-server --appendonly yes"
	case "mysql":
		return "mariadbd"
	case "nginx":
		return "nginx -g 'daemon off;'"
	case "java":
		return "/opt/java/openjdk/bin/java -jar /app/app.jar"
	case "prometheus":
		return "/bin/prometheus --config.file=/etc/prometheus/prometheus.yml --storage.tsdb.path=/prometheus"
	case "php":
		return "apache2-foreground"
	case "node", "web":
		return "node server.js"
	case "python":
		return "python -m app"
	default:
		return "/entrypoint.sh"
	}
}

func addLeftovers(h *Host, r *rng, now time.Time) {
	tagged := make([]*Image, 0, len(h.images))
	for _, img := range h.images {
		tagged = append(tagged, img)
	}
	sort.Slice(tagged, func(i, j int) bool { return tagged[i].ID < tagged[j].ID })
	for _, img := range tagged {
		for v := 0; v < r.between(0, 3); v++ {
			old := &Image{
				ID:      "sha256:" + r.hex(64),
				Digest:  "sha256:" + r.hex(64),
				Created: img.Created.Add(-time.Duration(r.between(10, 200)) * 24 * time.Hour),
				Size:    img.Size - int64(r.between(0, 40))<<20,
				Arch:    img.Arch,
				OS:      "linux",
			}
			if r.chance(0.4) && len(img.RepoTags) > 0 {
				repo := strings.Split(img.RepoTags[0], ":")[0]
				old.RepoTags = []string{repo + ":" + pick(r, []string{"previous", "rollback", "2024.06", "1.0", "stable-old"})}
			}
			h.images[old.ID] = old
		}
	}
	for i := 0; i < r.between(6, 30); i++ {
		name := r.hex(64)
		h.volumes[name] = &Volume{Name: name, Driver: "local", Created: now.Add(-time.Duration(r.between(1, 300)) * 24 * time.Hour), SizeBytes: int64(r.between(0, 2000)) << 20}
	}
}
