// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

import (
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"path"
	"regexp"
	"strings"
	"time"

	"github.com/GehirnInc/crypt/sha512_crypt"

	"github.com/therealmcsparrow/mcharbor/core/plugin"
)

// Row scanning, validation and the small conversions around them.

// Subordinate id allocation. Each container gets a full 65536-wide range so
// that every uid a normal distribution uses — including the 65534 nobody — maps
// to something distinct on the host. Narrower ranges save nothing real and
// break packages that assume the full space exists.
const (
	// Starts high enough to clear anything a host distribution allocates for
	// real users, which conventionally stops well below 100000.
	idRangeBase = int64(1000000)
	idRangeSize = int64(65536)
	// A uid_t is 32-bit. Past this there is no range left to hand out, and
	// wrapping around into somebody else's is worse than refusing.
	idRangeCeiling = int64(4294967295)
)

// --- Scanning -------------------------------------------------------------

func scanContainer(rows plugin.Rows) (Container, error) {
	var ct Container
	var privileged, nesting, autostart, isTemplate int
	var initArgv, appliedLimits, limitsAppliedAt string
	var observedIPs, lastStarted, created, updated string

	if err := rows.Scan(&ct.ID, &ct.EnvironmentID, &ct.Name, &ct.Description, &ct.State,
		&ct.OSFamily, &ct.OSVersion,
		&ct.PoolID, &ct.PoolBackend, &ct.PoolTarget, &ct.RootfsPath, &ct.TemplatePath,
		&ct.InitSystem, &initArgv,
		&privileged, &nesting,
		&ct.UIDMapStart, &ct.UIDMapCount, &ct.GIDMapStart, &ct.GIDMapCount,
		&ct.IDMapMechanism,
		&ct.Limits.CPUMax, &ct.Limits.CPUWeight, &ct.Limits.CPUSet,
		&ct.Limits.MemoryMaxBytes, &ct.Limits.MemoryHighBytes,
		&ct.Limits.SwapMaxBytes, &ct.Limits.PidsMax, &ct.Limits.IOMax,
		&ct.Limits.DiskQuotaBytes,
		&appliedLimits, &limitsAppliedAt,
		&autostart, &ct.AutostartOrder, &ct.AutostartDelayS,
		&isTemplate, &ct.TemplateSourceID,
		&observedIPs, &ct.UnitName, &ct.InitPID,
		&lastStarted, &ct.ExitReason, &ct.LastError, &created, &updated); err != nil {
		return Container{}, err
	}

	ct.Privileged = privileged == 1
	ct.Nesting = nesting == 1
	ct.Autostart = autostart == 1
	ct.IsTemplate = isTemplate == 1
	ct.LastStartedAt = parseTimePtr(lastStarted)
	ct.LimitsAppliedAt = parseTimePtr(limitsAppliedAt)
	ct.CreatedAt = parseTime(created)
	ct.UpdatedAt = parseTime(updated)

	if initArgv != "" {
		var argv []string
		if err := json.Unmarshal([]byte(initArgv), &argv); err == nil {
			ct.InitArgv = argv
		}
	}
	if observedIPs != "" {
		var ips []string
		if err := json.Unmarshal([]byte(observedIPs), &ips); err == nil {
			ct.ObservedIPs = ips
		}
	}
	if appliedLimits != "" {
		// A blob this build cannot read means an older row shape. Dropped
		// rather than shown, because a half-decoded set of applied limits would
		// claim the kernel accepted values it may never have seen.
		var applied Limits
		if err := json.Unmarshal([]byte(appliedLimits), &applied); err == nil {
			ct.Applied = &applied
		}
	}
	return ct, nil
}

func scanNIC(rows plugin.Rows) (NIC, error) {
	var nic NIC
	var dns, created, updated string

	if err := rows.Scan(&nic.ID, &nic.ContainerID, &nic.NetworkID, &nic.Bridge,
		&nic.VLANTag, &nic.AttachKind, &nic.ParentLink, &nic.LinkMode,
		&nic.Slot, &nic.GuestName, &nic.MAC, &nic.VethHost,
		&nic.AddressMode, &nic.StaticIP, &nic.StaticGW,
		&nic.StaticIPv6, &nic.StaticGW6, &dns,
		&created, &updated); err != nil {
		return NIC{}, err
	}

	if dns != "" {
		var servers []string
		if err := json.Unmarshal([]byte(dns), &servers); err == nil {
			nic.StaticDNS = servers
		}
	}
	nic.CreatedAt = parseTime(created)
	nic.UpdatedAt = parseTime(updated)
	return nic, nil
}

func scanMount(rows plugin.Rows) (Mount, error) {
	var mount Mount
	var readonly, shift int
	var created, updated string

	if err := rows.Scan(&mount.ID, &mount.ContainerID, &mount.Kind, &mount.Source,
		&mount.Destination, &readonly, &shift, &created, &updated); err != nil {
		return Mount{}, err
	}

	mount.ReadOnly = readonly == 1
	mount.ShiftIDs = shift == 1
	mount.CreatedAt = parseTime(created)
	mount.UpdatedAt = parseTime(updated)
	return mount, nil
}

func scanSnapshot(rows plugin.Rows) (Snapshot, error) {
	var snap Snapshot
	var wasFrozen int
	var created, updated string

	if err := rows.Scan(&snap.ID, &snap.ContainerID, &snap.ParentID, &snap.Name,
		&snap.Description, &snap.Mechanism, &wasFrozen, &snap.SizeBytes,
		&snap.ChainDepth, &snap.Handle, &created, &updated); err != nil {
		return Snapshot{}, err
	}

	snap.WasFrozen = wasFrozen == 1
	snap.CreatedAt = parseTime(created)
	snap.UpdatedAt = parseTime(updated)
	return snap, nil
}

func scanProvisioning(rows plugin.Rows) (Provisioning, error) {
	var prov Provisioning
	var keys, hash, applied, created, updated string

	if err := rows.Scan(&prov.ContainerID, &prov.Hostname, &prov.Timezone,
		&keys, &hash, &prov.NetConfigFormat, &applied,
		&created, &updated); err != nil {
		return Provisioning{}, err
	}

	if keys != "" {
		var parsed []string
		if err := json.Unmarshal([]byte(keys), &parsed); err == nil {
			prov.SSHKeys = parsed
		}
	}
	// The hash itself never leaves this function. See the note on
	// Provisioning.HasPassword.
	prov.HasPassword = hash != ""
	prov.AppliedAt = parseTimePtr(applied)
	prov.CreatedAt = parseTime(created)
	prov.UpdatedAt = parseTime(updated)
	return prov, nil
}

// --- Secrets --------------------------------------------------------------

// hashPassword produces a SHA-512 crypt(3) hash the agent can write straight
// into /etc/shadow.
//
// Hashed here rather than on the agent so that the plaintext exists only for
// the lifetime of one HTTP request and never crosses the agent transport, is
// never in an agent log, and is never on the container host's disk.
func hashPassword(plaintext string) (string, error) {
	if plaintext == "" {
		return "", nil
	}

	// A 16-character salt from crypto/rand. math/rand here would make every
	// container's hash cheaper to attack than its password deserves, for no
	// saving at all at this call rate.
	salt, err := randomSalt(16)
	if err != nil {
		return "", err
	}

	crypter := sha512_crypt.New()
	// The $6$ prefix selects SHA-512; the default round count is what every
	// distribution's own passwd uses.
	hash, err := crypter.Generate([]byte(plaintext), []byte("$6$"+salt))
	if err != nil {
		return "", fmt.Errorf("hashing the password: %w", err)
	}
	return hash, nil
}

// saltAlphabet is the character set crypt(3) salts use.
const saltAlphabet = "./0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz"

// randomSalt draws a salt uniformly from the crypt alphabet.
//
// The rejection loop avoids the modulo bias a naive `b % 64` would introduce.
// The alphabet happens to be exactly 64 characters so the bias would be zero
// here — but writing it correctly means a future change to the alphabet cannot
// quietly introduce one.
func randomSalt(length int) (string, error) {
	out := make([]byte, 0, length)
	buf := make([]byte, 1)

	for len(out) < length {
		if _, err := rand.Read(buf); err != nil {
			return "", fmt.Errorf("generating a salt: %w", err)
		}
		if int(buf[0]) >= (256/len(saltAlphabet))*len(saltAlphabet) {
			continue
		}
		out = append(out, saltAlphabet[int(buf[0])%len(saltAlphabet)])
	}
	return string(out), nil
}

// generateMAC returns a locally-administered unicast MAC under McHarbor's own
// prefix.
//
// The same 02:4d:43 prefix addon-kvm uses, deliberately: a container and a VM
// are equally McHarbor's, and one prefix means one place to look when an
// unfamiliar MAC turns up on a switch.
func generateMAC() (string, error) {
	suffix := make([]byte, 3)
	if _, err := rand.Read(suffix); err != nil {
		return "", fmt.Errorf("generating a MAC address: %w", err)
	}
	// 0x02 in the first octet is the locally-administered bit, and the low bit
	// clear keeps it unicast — a multicast MAC produces a guest that cannot be
	// reached and gives no clue why.
	return fmt.Sprintf("02:4d:43:%02x:%02x:%02x", suffix[0], suffix[1], suffix[2]), nil
}

// --- Naming ---------------------------------------------------------------

// unitName is the transient systemd unit backing one container. The
// `mcharbor-` prefix is what makes the reconcile pass able to tell McHarbor's
// units from everything else on the host.
func unitName(containerID string) string {
	return "mcharbor-ct-" + containerID
}

// rootfsPath is where a container's filesystem lives inside its pool.
func rootfsPath(poolTarget, containerID string) string {
	return path.Join(poolTarget, "containers", containerID, "rootfs")
}

// vethName is the host end of a container's veth pair.
//
// Linux caps an interface name at 15 characters, so the container id is
// truncated rather than the name being silently rejected by the kernel at
// creation time. The first eight characters of an xid still carry its
// timestamp and machine id, so collisions inside one host are not realistic.
func vethName(containerID string, slot int) string {
	short := containerID
	if len(short) > 8 {
		short = short[:8]
	}
	return fmt.Sprintf("mcv%s%d", short, slot)
}

// --- Pools and mechanisms -------------------------------------------------

// supportsCoW reports whether a pool backend can clone without copying.
func supportsCoW(backend string) bool {
	switch backend {
	case "zfs", "btrfs", "lvm":
		return true
	default:
		return false
	}
}

// mechanismFor maps a pool backend to the snapshot mechanism it offers.
//
// A dir pool gets rsync: a real copy rather than a snapshot. Correct, much
// slower, and labelled honestly in the UI rather than presented as the
// instant operation the other three are.
func mechanismFor(backend string) string {
	switch backend {
	case "zfs":
		return MechZFS
	case "btrfs":
		return MechBtrfs
	case "lvm":
		return MechLVM
	default:
		return MechRsync
	}
}

// snapshotHandle is the backend-native name used to revert or delete.
//
// The agent's own handle wins when it sent one; the fallback exists so a
// snapshot taken against an agent that did not report a handle is still
// addressable rather than being recorded with an empty one.
func snapshotHandle(reply *agentPayload, ct Container, name string) string {
	if reply != nil && reply.SnapshotName != "" {
		return reply.SnapshotName
	}
	return ct.RootfsPath + "@" + name
}

// idmapMechanism picks between an idmapped mount and a recursive chown.
//
// An idmapped mount is instant and leaves the rootfs shareable; the chown
// fallback rewrites every inode, takes minutes on a large rootfs, and is
// one-way. Kernel 5.12 brought idmapped mounts, so anything older gets the
// slow path rather than a failure.
func idmapMechanism(capability capability) string {
	for _, item := range capability.Items {
		if item.Key == "idmapped_mounts" {
			if item.Present {
				return "idmapped"
			}
			return "chown"
		}
	}
	return "chown"
}

// netConfigFormat picks the file a static address has to be written into.
//
// A container has no cloud-init and no guaranteed DHCP client, so a static
// address means writing the distribution's own config. Getting this wrong
// leaves a container with no network and no route to fix it except the
// console, which is why it follows the detected OS family rather than a
// default that happens to suit Debian.
func netConfigFormat(osFamily string) string {
	switch strings.ToLower(osFamily) {
	case "debian", "ubuntu":
		return "interfaces"
	case "alpine":
		return "interfaces"
	default:
		return "systemd-networkd"
	}
}

// --- Limits ---------------------------------------------------------------

// withLimitDefaults fills in the limits every container should have.
//
// PidsMax in particular is not left empty: without it one container can consume
// the host's entire pid space and take every other container, the agent and
// sshd down with it. A default the operator can raise is far better than a
// footgun they have to know to disarm.
func withLimitDefaults(limits Limits) Limits {
	if limits.PidsMax <= 0 {
		limits.PidsMax = 4096
	}
	if limits.CPUWeight <= 0 {
		// cgroup v2's own default. Set explicitly so that a container inherits
		// a known weight rather than whatever its parent slice happens to have.
		limits.CPUWeight = 100
	}
	// MemoryHigh below MemoryMax gives the kernel somewhere to throttle before
	// it starts killing processes, which turns "the container died" into "the
	// container went slow" — a far more diagnosable failure. 90% is enough
	// headroom to matter and not so much that it throttles healthy workloads.
	if limits.MemoryMaxBytes > 0 && limits.MemoryHighBytes == 0 {
		limits.MemoryHighBytes = limits.MemoryMaxBytes / 10 * 9
	}
	return limits
}

func limitsToAgent(limits Limits) *agentLimits {
	return &agentLimits{
		CPUMax:          limits.CPUMax,
		CPUWeight:       limits.CPUWeight,
		CPUSet:          limits.CPUSet,
		MemoryMaxBytes:  limits.MemoryMaxBytes,
		MemoryHighBytes: limits.MemoryHighBytes,
		SwapMaxBytes:    limits.SwapMaxBytes,
		PidsMax:         limits.PidsMax,
		IOMax:           limits.IOMax,
		DiskQuotaBytes:  limits.DiskQuotaBytes,
	}
}

func limitsFromAgent(limits agentLimits) Limits {
	return Limits(limits)
}

// clampedFields names the limits the kernel did not take as asked.
//
// Reported per field rather than as a single "some values were adjusted",
// because which value moved is the entire useful content of the message: a
// memory.max rounded up to a page boundary is noise, and a pids.max clamped to
// a tenth of what was asked is a capacity problem.
func clampedFields(requested, applied Limits) []string {
	var out []string
	add := func(name string, differs bool) {
		if differs {
			out = append(out, name)
		}
	}

	add("cpuMax", requested.CPUMax != applied.CPUMax)
	add("cpuWeight", requested.CPUWeight != applied.CPUWeight)
	add("cpuset", requested.CPUSet != applied.CPUSet)
	add("memoryMaxBytes", requested.MemoryMaxBytes != applied.MemoryMaxBytes)
	add("memoryHighBytes", requested.MemoryHighBytes != applied.MemoryHighBytes)
	add("swapMaxBytes", requested.SwapMaxBytes != applied.SwapMaxBytes)
	add("pidsMax", requested.PidsMax != applied.PidsMax)
	add("ioMax", requested.IOMax != applied.IOMax)
	add("diskQuotaBytes", requested.DiskQuotaBytes != applied.DiskQuotaBytes)
	return out
}

func statsFromAgent(stats agentStats) Stats {
	return Stats{
		ContainerID:     stats.ContainerID,
		CPUPercent:      stats.CPUPercent,
		MemoryBytes:     stats.MemoryBytes,
		MemoryMaxBytes:  stats.MemoryMaxBytes,
		MemoryHighBytes: stats.MemoryHighBytes,
		SwapBytes:       stats.SwapBytes,
		PidsCurrent:     stats.PidsCurrent,
		PidsMax:         stats.PidsMax,
		IOReadBytes:     stats.IOReadBytes,
		IOWriteBytes:    stats.IOWriteBytes,
		NetRxBytes:      stats.NetRxBytes,
		NetTxBytes:      stats.NetTxBytes,
		RootfsUsedBytes: stats.RootfsUsedBytes,
		SampledAt:       parseTime(stats.SampledAt),
	}
}

// --- Validation -----------------------------------------------------------

// containerNameRE is what a container name may be.
//
// It doubles as the hostname and as part of a systemd unit name, so it is held
// to the stricter of those: a DNS label. Rejecting here means never discovering
// at start time that systemd will not accept the unit.
var containerNameRE = regexp.MustCompile(`^[a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?$`)

func validateCreate(req CreateRequest) error {
	if req.ImageID == "" && req.TemplateID == "" {
		return errors.New("a container needs an image or a template to start from")
	}
	if req.ImageID != "" && req.TemplateID != "" {
		return errors.New("a container starts from an image or a template, not both")
	}
	return validateDefinition(req)
}

// validateRecreate checks a container definition whose rootfs comes from a
// backup. Naming an image or a template as well is refused rather than ignored:
// the restore would silently overwrite whatever it unpacked.
func validateRecreate(req CreateRequest) error {
	if req.ImageID != "" || req.TemplateID != "" {
		return errors.New("a container recreated from a backup takes its rootfs from the backup, not an image or a template")
	}
	return validateDefinition(req)
}

// validateDefinition is every create rule that does not concern where the rootfs
// comes from.
func validateDefinition(req CreateRequest) error {
	if req.EnvironmentID == "" {
		return errors.New("a host is required")
	}
	if err := validateContainerName(req.Name); err != nil {
		return err
	}
	// The backend and target may be omitted: the server resolves them from the
	// pool's earlier use on this host, and checks a first use there.
	if req.PoolID == "" {
		return errors.New("a storage pool is required")
	}

	switch req.InitSystem {
	case "", InitSystemd, InitOpenRC, InitNone:
	default:
		return fmt.Errorf("%q is not an init system this supports", req.InitSystem)
	}

	if err := validateLimits(req.Limits); err != nil {
		return err
	}
	for _, nic := range req.NICs {
		if !hasAttachment(nic.AttachKind, nic.Bridge, nic.ParentLink) {
			return errors.New("every interface needs a bridge or a parent interface")
		}
		if nic.AddressMode == "static" && nic.StaticIP == "" {
			return errors.New("a static interface needs an address")
		}
		if err := validateNICIPv6(nic.AttachKind, nic.StaticIPv6, nic.StaticGW6); err != nil {
			return err
		}
	}
	return nil
}

func validateContainerName(name string) error {
	if name == "" {
		return errors.New("a name is required")
	}
	if !containerNameRE.MatchString(name) {
		return fmt.Errorf("%q is not a usable name: use lowercase letters, digits "+
			"and hyphens, starting and ending with a letter or digit", name)
	}
	return nil
}

func validateLimits(limits Limits) error {
	if limits.MemoryMaxBytes < 0 || limits.MemoryHighBytes < 0 || limits.SwapMaxBytes < 0 {
		return errors.New("a memory limit cannot be negative")
	}
	// memory.high above memory.max is silently useless: the kernel would never
	// reach the throttle before the hard limit killed the process. Refused here
	// so the operator learns it now rather than from an OOM kill later.
	if limits.MemoryMaxBytes > 0 && limits.MemoryHighBytes > limits.MemoryMaxBytes {
		return errors.New("the soft memory limit cannot be above the hard limit")
	}
	if limits.PidsMax < 0 {
		return errors.New("a process limit cannot be negative")
	}
	if limits.CPUWeight != 0 && (limits.CPUWeight < 1 || limits.CPUWeight > 10000) {
		return errors.New("the cpu weight must be between 1 and 10000")
	}
	if limits.CPUMax != "" && !cpuMaxRE.MatchString(limits.CPUMax) {
		return errors.New(`the cpu limit must be "max" or two numbers, quota and period`)
	}
	return nil
}

// cpuMaxRE matches cgroup v2's `cpu.max` format: a quota and a period, or the
// literal "max".
var cpuMaxRE = regexp.MustCompile(`^(max|[0-9]+)( [0-9]+)?$`)

// validateMount is the second of three checks on a mount.
//
// The handler checked that the caller holds the permission the mount kind
// requires. This checks the mount itself: a volume must live inside the
// container's own pool, so a caller cannot label an arbitrary host path as a
// volume and mount it with only ct.manage. The agent checks a third time,
// independently — this one is UX and defence in depth, the agent's is the
// security boundary.
func validateMount(ct Container, req CreateMountRequest) error {
	if req.Source == "" || req.Destination == "" {
		return fmt.Errorf("%w: a mount needs a source and a destination", ErrBadMount)
	}
	if !path.IsAbs(req.Destination) {
		return fmt.Errorf("%w: the destination must be an absolute path", ErrBadMount)
	}
	// Cleaned before comparison: "/pool/../etc" is not inside "/pool", and a
	// prefix test on the raw string would say it is.
	destination := path.Clean(req.Destination)
	if destination == "/" {
		return fmt.Errorf("%w: the container root cannot be replaced by a mount", ErrBadMount)
	}

	switch req.Kind {
	case MountVolume:
		source := path.Clean(req.Source)
		if !path.IsAbs(source) {
			return fmt.Errorf("%w: a volume source must be an absolute path", ErrBadMount)
		}
		pool := path.Clean(ct.PoolTarget)
		if source != pool && !strings.HasPrefix(source, pool+"/") {
			return fmt.Errorf("%w: a volume must live inside the container's pool", ErrBadMount)
		}
		return nil
	case MountBind, MountDevice:
		// Anything at all, and that is the point: this is why the kind needs
		// ct.privileged.
		if !path.IsAbs(path.Clean(req.Source)) {
			return fmt.Errorf("%w: the source must be an absolute path", ErrBadMount)
		}
		return nil
	default:
		return fmt.Errorf("%w: %q is not a mount kind", ErrBadMount, req.Kind)
	}
}

// requiresPrivilege reports whether a mount request needs ct.privileged.
//
// A volume is a directory this plugin created inside a pool it already
// controls. A bind is an arbitrary host path — bind /etc and a container root
// rewrites the host's shadow file — and a device is worse.
func requiresPrivilege(kind string) bool {
	return kind != MountVolume
}

// --- Agent errors ---------------------------------------------------------

// agentError turns an unsuccessful agent reply into an error.
//
// The agent's own text is preserved for the server log and for the AI diagnosis
// path; the handler maps it onto an i18n code before anything reaches a client.
func agentError(reply *agentPayload) error {
	if reply == nil {
		return errors.New("the agent sent no answer")
	}
	if reply.Error != "" {
		if reply.ErrorCode != "" {
			return fmt.Errorf("%s (%s)", reply.Error, reply.ErrorCode)
		}
		return errors.New(reply.Error)
	}
	return errors.New("the agent refused the operation without saying why")
}

// --- Small helpers --------------------------------------------------------

func parseTime(value string) time.Time {
	if value == "" {
		return time.Time{}
	}
	if parsed, err := time.Parse(time.RFC3339, value); err == nil {
		return parsed
	}
	if parsed, err := time.Parse("2006-01-02 15:04:05", value); err == nil {
		return parsed
	}
	return time.Time{}
}

func parseTimePtr(value string) *time.Time {
	parsed := parseTime(value)
	if parsed.IsZero() {
		return nil
	}
	return &parsed
}

func boolToInt(value bool) int {
	if value {
		return 1
	}
	return 0
}

func nullIfEmpty(value string) any {
	if value == "" {
		return nil
	}
	return value
}

func nullIfZero(value int) any {
	if value == 0 {
		return nil
	}
	return value
}

func nullIfZero64(value int64) any {
	if value == 0 {
		return nil
	}
	return value
}

func defaultString(value, fallback string) string {
	if value == "" {
		return fallback
	}
	return value
}

// isUniqueViolation reports whether an error is a UNIQUE constraint failure.
//
// The plugin DB handle returns the driver's text rather than a typed error, so
// this matches on the message. It is used only to turn a duplicate name, MAC or
// mount destination into a clear answer rather than a generic failure.
func isUniqueViolation(err error) bool {
	if err == nil {
		return false
	}
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "unique constraint") ||
		strings.Contains(message, "unique index")
}
