// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

import (
	"crypto/rand"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/GehirnInc/crypt/sha512_crypt"

	"github.com/therealmcsparrow/mcharbor/core/plugin"
)

// Row scanning, validation and the small conversions around them.

func scanVM(rows plugin.Rows) (VM, error) {
	var vm VM
	var secureBoot, tpm, autostart, isTemplate int
	var reportedIPs, lastStarted, created, updated string
	var virtioMem, hugePages, ioThread int
	var numaHostNodes string

	if err := rows.Scan(&vm.ID, &vm.EnvironmentID, &vm.Name, &vm.Description, &vm.State,
		&vm.GuestOSFamily, &vm.GuestOSVariant,
		&vm.VCPUs, &vm.CPUModel, &vm.CPUSockets, &vm.CPUCores, &vm.CPUThreads,
		&vm.MemoryMB, &vm.MemoryMaxMB,
		&vm.MachineType, &vm.Firmware, &secureBoot, &tpm, &vm.DisplayType,
		&autostart, &vm.AutostartOrder, &vm.AutostartDelayS,
		&isTemplate, &vm.TemplateSourceID,
		&vm.GuestAgentState, &reportedIPs,
		&vm.UUID, &vm.UnitName,
		&lastStarted, &vm.LastError, &created, &updated,
		&vm.CurrentSnapshotID,
		&vm.RunState, &vm.GuestHostname,
		&vm.GuestOSName, &vm.GuestOSVersion,
		&vm.VCPUsMax, &vm.MemoryCurrentMB,
		&virtioMem, &hugePages,
		&vm.NUMAPolicy, &numaHostNodes,
		&vm.VSockCID, &ioThread,
		&vm.DisplayProfileID,
		&vm.DisplayWidth, &vm.DisplayHeight); err != nil {
		return VM{}, err
	}

	vm.VirtioMem = virtioMem == 1
	vm.HugePages = hugePages == 1
	vm.IOThread = ioThread == 1
	vm.DisplayAdapter = effectiveDisplay(vm.DisplayType, vm.GuestOSFamily)
	vm.DisplayProfileID = effectiveProfileID(vm.DisplayProfileID)
	if numaHostNodes != "" {
		// Same treatment as the reported addresses below: a blob an older
		// build wrote in a shape this one does not know is dropped rather
		// than rendered, because the policy that produced it is re-derivable.
		var nodes []int
		if err := json.Unmarshal([]byte(numaHostNodes), &nodes); err == nil {
			vm.NUMAHostNodes = nodes
		}
	}

	vm.SecureBoot = secureBoot == 1
	vm.TPMEnabled = tpm == 1
	vm.Autostart = autostart == 1
	vm.IsTemplate = isTemplate == 1
	vm.LastStartedAt = parseTimePtr(lastStarted)
	vm.CreatedAt = parseTime(created)
	vm.UpdatedAt = parseTime(updated)

	if reportedIPs != "" {
		// Guest-supplied. A value that will not parse is dropped rather than
		// shown: a malformed blob here means an older agent wrote a shape this
		// build does not know, and the fix is to ask the guest again.
		var ips []string
		if err := json.Unmarshal([]byte(reportedIPs), &ips); err == nil {
			vm.GuestReportedIPs = ips
		}
	}
	return vm, nil
}

func scanDisk(rows plugin.Rows) (Disk, error) {
	var disk Disk
	var discard, readonly, isBoot, isCDROM, shareRW int
	var created, updated string

	if err := rows.Scan(&disk.ID, &disk.VMID, &disk.PoolID, &disk.PoolBackend,
		&disk.PoolTarget, &disk.Slot, &disk.Bus, &disk.VolumePath, &disk.Format,
		&disk.SizeBytes, &disk.BackingPath, &disk.CacheMode,
		&discard, &readonly, &isBoot, &isCDROM, &created, &updated,
		&disk.BootIndex, &shareRW,
		&disk.ReadIOPS, &disk.WriteIOPS,
		&disk.ReadBPS, &disk.WriteBPS); err != nil {
		return Disk{}, err
	}

	disk.ShareRW = shareRW == 1
	disk.Discard = discard == 1
	disk.ReadOnly = readonly == 1
	disk.IsBoot = isBoot == 1
	disk.IsCDROM = isCDROM == 1
	disk.CreatedAt = parseTime(created)
	disk.UpdatedAt = parseTime(updated)
	return disk, nil
}

func scanNIC(rows plugin.Rows) (NIC, error) {
	var nic NIC
	var linkDown int
	var created, updated string

	if err := rows.Scan(&nic.ID, &nic.VMID, &nic.NetworkID, &nic.Bridge, &nic.VLANTag,
		&nic.AttachKind, &nic.ParentLink, &nic.LinkMode,
		&nic.Slot, &nic.Model, &nic.MAC, &nic.TapName, &nic.PCIAddr, &linkDown,
		&created, &updated); err != nil {
		return NIC{}, err
	}
	nic.LinkDown = linkDown == 1
	nic.CreatedAt = parseTime(created)
	nic.UpdatedAt = parseTime(updated)
	return nic, nil
}

func scanSnapshot(rows plugin.Rows) (Snapshot, error) {
	var snapshot Snapshot
	var withMemory int
	var created, updated string

	if err := rows.Scan(&snapshot.ID, &snapshot.VMID, &snapshot.ParentID, &snapshot.Name,
		&snapshot.Description, &snapshot.Mechanism, &snapshot.Consistency,
		&withMemory, &snapshot.SizeBytes, &snapshot.ChainDepth, &snapshot.Handle,
		&created, &updated); err != nil {
		return Snapshot{}, err
	}

	snapshot.WithMemory = withMemory == 1
	snapshot.CreatedAt = parseTime(created)
	snapshot.UpdatedAt = parseTime(updated)
	return snapshot, nil
}

// --- Password hashing -----------------------------------------------------

// hashPassword produces a SHA-512 crypt(3) hash for cloud-init.
//
// The plaintext never leaves this function. It is hashed the moment it arrives,
// stored only as a hash, and only the hash ever reaches the seed image — which
// matters because that image is readable by anything that can read the pool.
// cloud-init's chpasswd accepts this form on every supported distribution.
func hashPassword(plaintext string) (string, error) {
	if plaintext == "" {
		return "", nil
	}

	// A 16-character salt from crypto/rand. math/rand here would make every
	// guest's hash cheaper to attack than its password deserves, for no saving
	// at all at this call rate.
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
		// 256 is 4 × 64, so every byte maps evenly; the guard keeps that true
		// if the alphabet ever changes.
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
// 52:54:00 is QEMU's convention and is deliberately not used: a homelab running
// both McHarbor guests and hand-made QEMU VMs on one segment would otherwise be
// one collision away from two machines sharing a MAC, which presents as
// intermittent network loss and names nothing.
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

// --- Validation -----------------------------------------------------------

func validateCreate(req CreateRequest) error {
	if req.EnvironmentID == "" {
		return invalid(codeHostRequired, "a host is required")
	}
	if userDataCarriesMarker(req) {
		return invalid(codeUserDataMasked, "cloud-init user-data carries the redaction marker")
	}
	if err := validateGuestName(req.Name); err != nil {
		return err
	}
	if err := validateVCPUs(req.VCPUs); err != nil {
		return err
	}
	if err := validateMemory(req.MemoryMB, req.MemoryMaxMB); err != nil {
		return err
	}
	if err := validateFirmware(defaultString(req.Firmware, "uefi"), req.SecureBoot); err != nil {
		return err
	}
	if req.DiskBus != "" {
		if err := validateDiskBus(req.DiskBus); err != nil {
			return err
		}
	}
	if req.PoolID == "" || req.PoolBackend == "" || req.PoolTarget == "" {
		return invalid(codePoolRequired, "a storage pool is required")
	}
	if req.DiskVolumePath != "" {
		// An adopted volume brings its own size, so a size is not asked for;
		// what it does need is to be inside the pool it claims to be in, which
		// the service re-checks against the agent's own listing as well.
		if err := validateVolumePath(req.PoolBackend, req.PoolTarget, req.DiskVolumePath); err != nil {
			return err
		}
	} else if req.DiskSizeGB < 1 {
		return invalid(codeDiskRequired, "a disk size is required")
	}
	if req.NetworkID == "" || !hasAttachment(req.AttachKind, req.Bridge, req.ParentLink) {
		return invalid(codeNetworkRequired, "a network is required")
	}
	// A guest with neither an image nor an installer disc is allowed: it boots
	// to the firmware prompt, which is what an adopted disk that already holds
	// a system, a PXE install, or a disc attached later all need.
	return nil
}

// validateVolumePath refuses a volume path that is not inside its pool.
//
// The agent validates independently and the service re-lists the pool before
// adopting anything — this is the cheap first refusal, so a path typed into the
// API by hand is named as wrong here rather than reaching a root process.
func validateVolumePath(backend, target, path string) error {
	if path == "" || !strings.HasPrefix(path, "/") || strings.Contains(path, "..") {
		return invalid(codeVolumePath, fmt.Sprintf("%q is not a volume path", path))
	}
	// Only a filesystem pool addresses its volumes by their pool's path. A zfs
	// pool's target is a dataset name and its volumes live under /dev/zvol, and
	// an lvm pool's is a "vg/thinpool" pair whose volumes live under /dev — so
	// containment is checked where it means something and left to the agent's
	// listing everywhere else.
	if backend == "dir" || backend == "btrfs" {
		if !strings.HasPrefix(path, strings.TrimSuffix(target, "/")+"/") {
			return invalid(codeVolumePath, fmt.Sprintf("%q is not inside the pool", path))
		}
	}
	return nil
}

// validationError pairs the sentence the server log wants with the i18n code
// the client renders, so a rejected form can name the field it stumbled on.
type validationError struct {
	code   string
	reason string
}

// Error returns the reason written to the log; the client is sent code instead.
func (e *validationError) Error() string { return e.reason }

// invalid builds a validation error for one rejected field.
func invalid(code, reason string) error {
	return &validationError{code: code, reason: reason}
}

// validateGuestName keeps a name usable as a hostname and safe as a path
// component.
func validateGuestName(name string) error {
	if name == "" {
		return invalid(codeNameRequired, "a name is required")
	}
	if len([]rune(name)) > 120 {
		return invalid(codeNameTooLong, "that name is too long")
	}
	for _, r := range name {
		if r < 0x20 || r == 0x7f {
			return invalid(codeNameInvalid, "a name cannot contain control characters")
		}
	}
	return nil
}

// guestHostname reduces a display name to something cloud-init will accept.
//
// The two are different things, as they are for a snapshot: a name is the
// operator's, a hostname is RFC 1123 and the agent refuses anything else. A
// name that is already a valid hostname survives this unchanged.
func guestHostname(name string) string {
	var b strings.Builder
	lastDash := true
	for _, r := range strings.ToLower(strings.TrimSpace(name)) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
			lastDash = false
		default:
			if !lastDash {
				b.WriteRune('-')
				lastDash = true
			}
		}
	}
	slug := strings.Trim(b.String(), "-")
	if len(slug) > 63 {
		slug = strings.Trim(slug[:63], "-")
	}
	return slug
}

// validateDeviceAddress checks a passthrough address before it reaches the
// agent.
//
// The agent validates independently — that is the security boundary — but a
// malformed address caught here gives the operator a clear message instead of a
// QEMU error.
func validateDeviceAddress(kind, address string) error {
	switch kind {
	case "pci":
		// 0000:03:00.0 or 03:00.0
		parts := strings.Split(address, ":")
		if len(parts) != 2 && len(parts) != 3 {
			return fmt.Errorf("%q is not a PCI address", address)
		}
		slot, function, ok := strings.Cut(parts[len(parts)-1], ".")
		if !ok {
			return fmt.Errorf("%q is not a PCI address", address)
		}
		for _, part := range append(parts[:len(parts)-1], slot, function) {
			if _, err := strconv.ParseUint(part, 16, 32); err != nil {
				return fmt.Errorf("%q is not a PCI address", address)
			}
		}
		return nil

	case "usb":
		vendor, product, ok := strings.Cut(address, ":")
		if !ok || len(vendor) != 4 || len(product) != 4 {
			return fmt.Errorf("%q is not a USB vendor:product address", address)
		}
		for _, part := range []string{vendor, product} {
			if _, err := strconv.ParseUint(part, 16, 32); err != nil {
				return fmt.Errorf("%q is not a USB vendor:product address", address)
			}
		}
		return nil

	case "block":
		if !strings.HasPrefix(address, "/dev/") {
			return fmt.Errorf("a host block device must be a /dev path")
		}
		return nil

	case "u2f":
		// Emulated, so there is no host device and nothing to address. A
		// value here would be silently ignored on the agent, which is worse
		// than refusing it.
		if address != "" {
			return fmt.Errorf("a virtual security key takes no host address")
		}
		return nil

	default:
		return fmt.Errorf("%q is not a device kind", kind)
	}
}

// validateMediaPath checks an installer disc before it reaches the agent.
//
// The path is passed to QEMU as a read-only block device on the host, so it is
// held to an absolute path with no traversal and a disc image suffix rather
// than trusted because it came from the image library.
func validateMediaPath(path string) error {
	if path == "" {
		return ErrMediaPath
	}
	if !strings.HasPrefix(path, "/") || strings.Contains(path, "..") {
		return ErrMediaPath
	}
	lower := strings.ToLower(path)
	if !strings.HasSuffix(lower, ".iso") && !strings.HasSuffix(lower, ".img") {
		return ErrMediaPath
	}
	return nil
}

// --- Defaults -------------------------------------------------------------

// pickMachineType chooses a concrete, versioned machine type from what the
// host's QEMU actually offers.
//
// Pinning matters: `q35` is an alias that moves with the QEMU package, so a
// guest created against it can find its hardware changed by a host upgrade.
// The newest concrete q35 is picked, and recorded, and never changed silently.
func pickMachineType(capability probedCapability) string {
	for _, binary := range capability.QEMUBinaries {
		if !binary.KVMAccel {
			continue
		}
		best := ""
		var bestVersion []int
		for _, machine := range binary.MachineTypes {
			// Numeric comparison, and aliases skipped outright. Ubuntu ships
			// codename machine types beside the versioned ones, and under a
			// string comparison every one of them beats every number ('z' >
			// '9') — which pinned new guests to pc-q35-zesty, a 2017 model.
			version, ok := parseMachineVersion(machine, "pc-q35-")
			if !ok {
				continue
			}
			if bestVersion == nil || compareMachineVersions(version, bestVersion) > 0 {
				best, bestVersion = machine, version
			}
		}
		if best != "" {
			return best
		}
		// No versioned q35 at all — a very old or very unusual build. The
		// alias is worse than a pin but better than refusing to create.
		for _, machine := range binary.MachineTypes {
			if machine == "q35" {
				return "q35"
			}
		}
	}
	return ""
}

// parseMachineVersion pulls the dotted version off a versioned machine type.
//
// It reports false for a distribution alias such as `pc-q35-jammy`, which is
// the whole point: an alias carries no orderable version.
func parseMachineVersion(machine, prefix string) ([]int, bool) {
	rest, found := strings.CutPrefix(machine, prefix)
	if !found || rest == "" {
		return nil, false
	}
	parts := strings.Split(rest, ".")
	version := make([]int, 0, len(parts))
	for _, part := range parts {
		number, err := strconv.Atoi(part)
		if err != nil {
			return nil, false
		}
		version = append(version, number)
	}
	return version, true
}

// compareMachineVersions orders two dotted versions numerically, so 10.0 sorts
// above 9.0 rather than below it as a string comparison would have them.
func compareMachineVersions(a, b []int) int {
	for i := 0; i < len(a) || i < len(b); i++ {
		left, right := 0, 0
		if i < len(a) {
			left = a[i]
		}
		if i < len(b) {
			right = b[i]
		}
		if left != right {
			if left > right {
				return 1
			}
			return -1
		}
	}
	return 0
}

// defaultNICModel picks an adapter the guest can actually drive.
//
// Windows ships no virtio-net driver, so an imported Windows guest given the
// usual virtio adapter boots with no network and no way to fetch one. e1000 is
// emulated and inbox everywhere.
func defaultNICModel(family string) string {
	if family == OSFamilyWindows {
		return "e1000"
	}
	return "virtio-net"
}

// defaultDisplay picks a display adapter by guest family, and must match the
// agent's own defaultDisplay. Windows gets QXL because it ships no virtio-gpu
// driver; everything else gets virtio-gpu, the one adapter that follows the
// console window under both VNC and SPICE.
func defaultDisplay(family string) string {
	if family == OSFamilyWindows {
		return "qxl"
	}
	return "virtio-gpu"
}

// effectiveDisplay is the adapter a guest boots with: its stored choice, or the
// family default when the operator picked Auto. Resolving it here rather than on
// the agent keeps the stored row the truth about what a running guest has.
func effectiveDisplay(displayType, family string) string {
	if displayType == "" {
		return defaultDisplay(family)
	}
	return displayType
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

// splitList and joinList store a small string list in one column.
//
// A newline separator rather than a comma: SSH keys and DNS names never contain
// one, and a comma-separated list would break the first time somebody pasted a
// key with a comment in it.
func splitList(value string) []string {
	if value == "" {
		return nil
	}
	parts := strings.Split(value, "\n")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		trimmed := strings.TrimSpace(part)
		if trimmed != "" {
			out = append(out, trimmed)
		}
	}
	return out
}

func joinList(values []string) string {
	return strings.Join(values, "\n")
}

// isUniqueViolation reports whether an error is a UNIQUE constraint failure.
//
// The plugin DB handle returns the driver's text rather than a typed error, so
// this matches on the message. It is used only to turn a duplicate name or MAC
// into a clear answer rather than a generic failure.
func isUniqueViolation(err error) bool {
	if err == nil {
		return false
	}
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "unique constraint") ||
		strings.Contains(message, "unique index")
}
