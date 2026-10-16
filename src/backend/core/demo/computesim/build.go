// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package computesim

import (
	"fmt"
	"strings"
	"time"

	"github.com/therealmcsparrow/mcharbor/core/demo/estate"
)

const (
	gib = int64(1) << 30
	tib = int64(1) << 40
)

// vmTemplates are the golden images each host keeps; they count as VMs because
// the kvm plugin stores them in the same table.
var vmTemplates = map[string][]string{
	"prod-eu-1": {"ubuntu2404", "win2022"},
	"prod-us-1": {"rocky9"},
	"homelab":   {"debian12"},
	"dev":       {"ubuntu2404"},
}

var ctTemplates = map[string][]string{
	"prod-eu-1": {"ubuntu"},
	"homelab":   {"alpine", "debian"},
}

// crashedVMs and crashedCTs name the guests that are deliberately broken, so
// the failure states in the UI always have something to show.
var crashedVMs = map[string]struct {
	index    int
	runState string
	message  string
}{
	"prod-eu-2":     {index: 6, runState: "guest-panicked", message: "QEMU reported guest-panicked: kernel BUG at mm/slub.c:4502 (demo)"},
	"data-platform": {index: 3, runState: "io-error", message: "block I/O error on drive virtio-disk1: No space left on device (demo)"},
}

var crashedCTs = map[string]struct {
	index   int
	message string
}{
	"homelab": {index: 4, message: "init exited with status 1: error while loading shared libraries: libnode.so.109 (demo)"},
}

// planFor returns the built-in plan for a host key, or a small default plan for
// a host the catalogue does not name.
func planFor(key string) hostPlan {
	plan := hostPlans[key]
	if plan.vms == 0 && plan.cts == 0 {
		plan = hostPlan{vms: 4, cts: 3, vmRoles: []string{"app", "ubuntu-dev"}, ctRoles: []string{"nginx", "redis"},
			pools: []string{"local"}, networks: []string{"vm-lan"}}
	}
	return plan
}

func buildHost(h *estate.Host, index int, seed uint64, now time.Time, macs map[string]bool, plan hostPlan) *hostSim {
	r := newRNG(seed ^ estate.StableHash("compute|"+h.Key))
	hs := &hostSim{host: h, index: index, vms: map[string]*vm{}, cts: map[string]*ct{}, probedAt: now.Add(-37 * time.Minute)}
	b := &builder{hs: hs, r: r, seed: seed, now: now, counters: map[string]int{}, macs: macs}
	b.buildPools(plan.pools)
	b.buildNetworks(plan.networks)
	b.buildVMs(plan)
	b.buildCTs(plan)
	b.buildImages()
	return hs
}

type builder struct {
	hs       *hostSim
	r        *rng
	seed     uint64
	now      time.Time
	counters map[string]int
	macs     map[string]bool
}

func (b *builder) key() string { return b.hs.host.Key }

func (b *builder) buildPools(names []string) {
	h := b.hs.host
	for i, name := range names {
		p := &pool{Name: name, IsDefault: i == 0, CreatedAt: h.BootedAt.Add(-time.Duration(200+i) * 24 * time.Hour)}
		switch name {
		case "local":
			p.Backend, p.Target, p.Capacity = "dir", "/var/lib/mcharbor/pools/local", maxInt64(h.DiskTotal/4, 2*tib)
		case "tank":
			p.Backend, p.Target, p.Capacity = "zfs", "tank/vms", maxInt64(h.DiskTotal/2, 8*tib)
		case "fast-nvme":
			p.Backend, p.Target, p.Capacity = "lvm", "vg_nvme/thinpool", 7680*gib
		case "archive":
			p.Backend, p.Target, p.Capacity = "dir", "/mnt/nas/mcharbor", 40*tib
		default:
			p.Backend, p.Target, p.Capacity = "dir", "/var/lib/mcharbor/pools/"+name, 2*tib
		}
		p.baseUsed = p.Capacity / 100 * int64(b.r.between(3, 11))
		p.ID = stableID(b.seed, p.CreatedAt, "pool", b.key(), name)
		b.hs.pools = append(b.hs.pools, p)
	}
}

func (b *builder) buildNetworks(names []string) {
	h := b.hs.host
	managed := 0
	for i, name := range names {
		n := &network{Name: name, IsDefault: i == 0, CreatedAt: h.BootedAt.Add(-time.Duration(190+i) * 24 * time.Hour)}
		octet := b.hs.index * 10
		switch name {
		case "prod-vlan":
			n.Mode, n.Bridge, n.VLAN, n.Uplink = "bridged", "br0", 100+b.hs.index*10, "bond0"
			n.prefix = fmt.Sprintf("172.20.%d.", b.hs.index)
		case "lan":
			n.Mode, n.Bridge, n.Uplink = "bridged", "br0", "enp3s0"
			n.prefix = "192.168.1."
		default:
			n.Mode = "managed"
			n.Bridge = fmt.Sprintf("mcharbor-br%d", managed)
			managed++
			third := 1
			switch name {
			case "backend":
				third = 2
			case "lab-nat":
				third = 50
			}
			n.prefix = fmt.Sprintf("10.%d.%d.", octet, third)
			n.DHCP = true
			n.NAT = name != "backend"
			n.RangeStart, n.RangeEnd = n.prefix+"100", n.prefix+"250"
		}
		n.Subnet = n.prefix + "0/24"
		n.Gateway = n.prefix + "1"
		n.nextHost = 20
		if n.DHCP {
			n.nextHost = 100
		}
		n.ID = stableID(b.seed, n.CreatedAt, "net", b.key(), name)
		b.hs.networks = append(b.hs.networks, n)
	}
}

func (b *builder) poolNamed(name string) *pool {
	for _, p := range b.hs.pools {
		if p.Name == name {
			return p
		}
	}
	return b.hs.pools[0]
}

func (b *builder) hasPool(name string) bool {
	for _, p := range b.hs.pools {
		if p.Name == name {
			return true
		}
	}
	return false
}

// nextName numbers a role's guests per host and gives a single-instance role
// its bare name; ok is false when a single role is already taken.
func (b *builder) nextName(prefix string, single bool) (string, bool) {
	n := b.counters[prefix]
	if single && n > 0 {
		return "", false
	}
	b.counters[prefix] = n + 1
	if single {
		return prefix, true
	}
	return fmt.Sprintf("%s-%02d", prefix, n+1), true
}

func (b *builder) mac(prefix, id string, slot int) string {
	for salt := 0; ; salt++ {
		v := estate.StableHash(fmt.Sprintf("%s|%s|%d|%d", prefix, id, slot, salt))
		mac := fmt.Sprintf("%s:%02x:%02x:%02x", prefix, byte(v>>16), byte(v>>8), byte(v))
		if !b.macs[mac] {
			b.macs[mac] = true
			return mac
		}
	}
}

func (b *builder) allocIP(n *network) string {
	if n.nextHost > 250 {
		return ""
	}
	ip := n.prefix + fmt.Sprint(n.nextHost)
	n.nextHost++
	return ip
}

func (b *builder) buildVMs(plan hostPlan) {
	templates := vmTemplates[b.key()]
	if len(templates) > plan.vms {
		templates = templates[:plan.vms]
	}
	crash, hasCrash := crashedVMs[b.key()]
	roleVMs := plan.vms - len(templates)
	made := 0
	for idx := 0; made < roleVMs && idx < roleVMs*4; idx++ {
		roleKey := plan.vmRoles[idx%len(plan.vmRoles)]
		role := vmRoles[roleKey]
		name, ok := b.nextName(role.prefix, role.single)
		if !ok {
			continue
		}
		g := b.newVM(name, role, pick(b.r, role.os), false)
		switch {
		case hasCrash && made == crash.index:
			g.State, g.RunState, g.LastError = stateCrashed, crash.runState, crash.message
			g.StartedAt = time.Time{}
		case roleKey == "pfsense" || roleKey == "home-assist":
			g.State = stateRunning
		case roleKey == "win11-gaming":
			g.State = stateStopped
		default:
			roll := b.r.float()
			switch {
			case roll < 0.82:
				g.State = stateRunning
			case roll < 0.85:
				g.State = statePaused
			default:
				g.State = stateStopped
			}
		}
		b.finishVM(g)
		made++
	}
	for _, osKey := range templates {
		prof := osProfiles[osKey]
		name := "tpl-" + strings.ReplaceAll(strings.TrimPrefix(prof.variant, "win"), ".", "")
		if prof.family == "windows" {
			name = "tpl-" + prof.variant
		}
		role := vmRole{vcpus: []int{2}, memoryMB: []int64{4096}, diskGB: []int64{40}, desc: "Golden image for " + prof.name}
		g := b.newVM(name, role, osKey, true)
		g.State = stateStopped
		b.finishVM(g)
	}
}

func (b *builder) newVM(name string, role vmRole, osKey string, template bool) *vm {
	prof := osProfiles[osKey]
	created := b.now.Add(-time.Duration(b.r.between(20, 700))*24*time.Hour - time.Duration(b.r.intn(86400))*time.Second)
	id := stableID(b.seed, created, "vm", b.key(), name)
	g := &vm{
		ID: id, Name: name, Description: role.desc, OS: prof,
		VCPUs: pick(b.r, role.vcpus), MemoryMB: pick(b.r, role.memoryMB),
		MachineType: "pc-q35-8.2", UUID: stableUUID(id), CreatedAt: created,
		IsTemplate: template, Autostart: !template && b.r.chance(0.6),
		load: 0.04 + b.r.float()*0.5,
	}
	boot := b.poolNamed("local")
	if b.hasPool("tank") && b.r.chance(0.45) {
		boot = b.poolNamed("tank")
	}
	g.Disks = append(g.Disks, b.newDisk(g, boot, 0, pick(b.r, role.diskGB)*gib, true))
	if role.dataGB > 0 && !template {
		data := boot
		switch {
		case b.hasPool("fast-nvme"):
			data = b.poolNamed("fast-nvme")
		case b.hasPool("tank"):
			data = b.poolNamed("tank")
		}
		g.Disks = append(g.Disks, b.newDisk(g, data, 1, role.dataGB*gib, false))
	}
	return g
}

func (b *builder) newDisk(g *vm, p *pool, slot int, size int64, boot bool) *disk {
	d := &disk{PoolID: p.ID, Slot: slot, Bus: "virtio-blk", SizeBytes: size, IsBoot: boot}
	if g.OS.family == "windows" {
		d.Bus = "virtio-scsi"
	}
	d.ID = stableID(b.seed, g.CreatedAt, "disk", g.ID, fmt.Sprint(slot))
	d.Format, d.VolumePath = volumeFor(p, "vm-"+g.ID+"-disk"+fmt.Sprint(slot))
	return d
}

// volumeFor names a volume the way each pool backend lays it out on a host.
func volumeFor(p *pool, name string) (format, path string) {
	switch p.Backend {
	case "zfs":
		return "raw", "/dev/zvol/" + p.Target + "/" + name
	case "lvm":
		vg := strings.SplitN(p.Target, "/", 2)[0]
		return "raw", "/dev/" + vg + "/" + name
	default:
		return "qcow2", strings.TrimRight(p.Target, "/") + "/" + name + ".qcow2"
	}
}

func (b *builder) finishVM(g *vm) {
	if g.State == stateRunning || g.State == statePaused {
		g.StartedAt = b.now.Add(-time.Duration(b.r.between(2, 60*24)) * time.Hour)
		g.RunState = g.State
	}
	nets := b.hs.networks
	if !g.IsTemplate {
		g.NICs = append(g.NICs, b.newNIC(g.ID, g.CreatedAt, nets[0], 0, "52:54:00"))
		if len(nets) > 1 && b.r.chance(0.25) {
			g.NICs = append(g.NICs, b.newNIC(g.ID, g.CreatedAt, nets[1], 1, "52:54:00"))
		}
	}
	if !g.IsTemplate && b.r.chance(0.32) {
		g.Snapshots = b.newSnapshots(g.ID, g.CreatedAt, b.r.between(1, 3), g.OS.guestAgent, g.State)
		g.Current = g.Snapshots[len(g.Snapshots)-1].ID
	}
	b.hs.vms[g.ID] = g
	b.hs.vmOrder = append(b.hs.vmOrder, g.ID)
}

func (b *builder) newNIC(owner string, created time.Time, n *network, slot int, ouiPrefix string) *nic {
	return &nic{
		ID:        stableID(b.seed, created, "nic", owner, fmt.Sprint(slot)),
		NetworkID: n.ID, Bridge: n.Bridge, Slot: slot,
		MAC: b.mac(ouiPrefix, owner, slot), IP: b.allocIP(n),
	}
}

func (b *builder) newSnapshots(owner string, created time.Time, count int, agent bool, state string) []*snapshot {
	span := b.now.Sub(created)
	offset := b.r.intn(len(snapshotNames))
	out := make([]*snapshot, 0, count)
	parent := ""
	for i := 0; i < count; i++ {
		at := created.Add(span * time.Duration(i+1) / time.Duration(count+1)).Truncate(time.Second)
		name := snapshotNames[(offset+i)%len(snapshotNames)]
		s := &snapshot{
			ID: stableID(b.seed, at, "snap", owner, name), ParentID: parent,
			Name: name, Handle: name, CreatedAt: at,
			SizeBytes: int64(b.r.between(1, 24)) * gib, Consistency: "crash", savedState: stateOr(state, stateStopped),
		}
		if agent {
			s.Consistency = "filesystem"
		}
		out = append(out, s)
		parent = s.ID
	}
	return out
}

func (b *builder) buildCTs(plan hostPlan) {
	templates := ctTemplates[b.key()]
	if len(templates) > plan.cts {
		templates = templates[:plan.cts]
	}
	crash, hasCrash := crashedCTs[b.key()]
	roleCTs := plan.cts - len(templates)
	made := 0
	for idx := 0; made < roleCTs && idx < roleCTs*4; idx++ {
		role := ctRoles[plan.ctRoles[idx%len(plan.ctRoles)]]
		name, ok := b.nextName("ct|"+role.prefix, role.single)
		if !ok {
			continue
		}
		c := b.newCT(strings.TrimPrefix(name, "ct|"), role, pick(b.r, role.os), false)
		switch {
		case hasCrash && made == crash.index:
			c.State, c.LastError, c.ExitReason = stateCrashed, crash.message, "init_exited"
		default:
			roll := b.r.float()
			switch {
			case roll < 0.84:
				c.State = stateRunning
			case roll < 0.87:
				c.State = stateFrozen
			default:
				c.State = stateStopped
			}
		}
		b.finishCT(c)
		made++
	}
	for _, osKey := range templates {
		o := ctOSes[osKey]
		role := ctRole{cpus: []int{1}, memMB: []int64{512}, diskGB: 4, desc: "Container template (" + o.pretty + ")"}
		c := b.newCT("tpl-"+o.family+"-"+strings.ReplaceAll(o.version, ".", ""), role, osKey, true)
		c.State = stateStopped
		b.finishCT(c)
	}
}

func (b *builder) newCT(name string, role ctRole, osKey string, template bool) *ct {
	created := b.now.Add(-time.Duration(b.r.between(10, 600))*24*time.Hour - time.Duration(b.r.intn(86400))*time.Second)
	id := stableID(b.seed, created, "ct", b.key(), name)
	p := b.poolNamed("local")
	if b.hasPool("tank") && b.r.chance(0.5) {
		p = b.poolNamed("tank")
	}
	c := &ct{
		ID: id, Name: name, Description: role.desc, OS: ctOSes[osKey], Role: role,
		PoolID: p.ID, CreatedAt: created, CPUs: pick(b.r, role.cpus),
		MemoryMax: pick(b.r, role.memMB) << 20, PidsMax: 4096, DiskQuota: role.diskGB * gib,
		IsTemplate: template, Autostart: !template && b.r.chance(0.7), load: 0.02 + b.r.float()*0.4,
		Nesting: strings.Contains(name, "gitea") || strings.Contains(name, "jupyter"),
	}
	c.RootfsPath = rootfsFor(p, id)
	return c
}

// rootfsFor names a container's root filesystem the way each backend holds it.
func rootfsFor(p *pool, id string) string {
	if p.Backend == "zfs" {
		return "/" + p.Target + "/ct-" + id
	}
	return strings.TrimRight(p.Target, "/") + "/ct/" + id + "/rootfs"
}

func (b *builder) finishCT(c *ct) {
	if c.State == stateRunning || c.State == stateFrozen {
		c.StartedAt = b.now.Add(-time.Duration(b.r.between(1, 90*24)) * time.Hour)
		c.InitPID = 10000 + int(estate.StableHash(c.ID)%400000)
	}
	if !c.IsTemplate {
		c.NICs = append(c.NICs, b.newNIC(c.ID, c.CreatedAt, b.hs.networks[0], 0, "00:16:3e"))
		if b.r.chance(0.25) {
			c.Snapshots = b.newSnapshots(c.ID, c.CreatedAt, b.r.between(1, 2), false, c.State)
			for _, s := range c.Snapshots {
				s.Handle = c.RootfsPath + "@" + s.Name
				s.SizeBytes /= 8
			}
		}
	}
	b.hs.cts[c.ID] = c
	b.hs.ctOrder = append(b.hs.ctOrder, c.ID)
}

func (b *builder) buildImages() {
	used := b.now.Add(-time.Duration(b.r.between(1, 20)) * 24 * time.Hour)
	b.hs.images = []cachedImage{
		{ImageID: "img-ubuntu-2404-amd64", Kind: "cloud_image", LocalPath: "/var/lib/mcharbor/compute/images/img-ubuntu-2404-amd64.qcow2", SizeBytes: 593 << 20, LastUsed: used},
		{ImageID: "img-debian-12-amd64", Kind: "cloud_image", LocalPath: "/var/lib/mcharbor/compute/images/img-debian-12-amd64.qcow2", SizeBytes: 421 << 20, LastUsed: used.Add(-72 * time.Hour)},
		{ImageID: "img-alpine-321-rootfs-amd64", Kind: "rootfs", LocalPath: "/var/lib/mcharbor/compute/images/img-alpine-321-rootfs-amd64.tar.gz", SizeBytes: 3 << 20, LastUsed: used.Add(-9 * time.Hour)},
		{ImageID: "img-ubuntu-2404-rootfs-amd64", Kind: "rootfs", LocalPath: "/var/lib/mcharbor/compute/images/img-ubuntu-2404-rootfs-amd64.tar.gz", SizeBytes: 29 << 20, LastUsed: used.Add(-30 * time.Hour)},
	}
}

func maxInt64(a, b int64) int64 {
	if a > b {
		return a
	}
	return b
}

// poolStats derives a pool's capacity sample from what the simulator has put
// in it, with no randomness, so the seeded row and a live sample agree.
func poolStats(hs *hostSim, p *pool) (capacity, used, allocated int64, volumes int) {
	for _, id := range hs.vmOrder {
		for _, d := range hs.vms[id].Disks {
			if d.PoolID == p.ID && !d.IsCDROM {
				allocated += d.SizeBytes
				volumes++
			}
		}
	}
	for _, id := range hs.ctOrder {
		if c := hs.cts[id]; c.PoolID == p.ID {
			allocated += c.DiskQuota
			volumes++
		}
	}
	fill := int64(45)
	if p.Backend == "dir" {
		fill = 60
	}
	used = p.baseUsed + allocated/100*fill
	if ceiling := p.Capacity / 100 * 94; used > ceiling {
		used = ceiling
	}
	return p.Capacity, used, allocated, volumes
}
