// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package agent

import (
	"errors"
	"fmt"
	"log/slog"
	"strings"

	"golang.org/x/crypto/ssh"
)

// errEscalationUnavailable is the install refusing to start on a host where no
// privileged command can be run at all.
var errEscalationUnavailable = errors.New("agent: no privilege escalation available on this host")

// Prerequisite keys. Stable strings: the UI toggles them by key and the deploy
// request names the ones the operator agreed to install.
const (
	PrereqDocker  = "docker"
	PrereqCompose = "docker_compose"
	PrereqKVM     = "kvm_device"
	PrereqCPUVirt = "cpu_virt"
	PrereqQEMU    = "qemu"
	PrereqQEMUImg = "qemu_img"
	PrereqSWTPM   = "swtpm"
	PrereqOVMF    = "ovmf"
)

// ProbeScope narrows a probe to the question the operator actually asked.
//
// The create dialog asks two separate ones — "is Docker installed" and "can
// this host run virtual machines" — and a scoped probe answers in one SSH round
// trip instead of running every check for both buttons.
type ProbeScope string

// Probe scopes. ScopeAll is the deploy-time probe and reports everything.
const (
	ScopeAll    ProbeScope = ""
	ScopeDocker ProbeScope = "docker"
	ScopeVM     ProbeScope = "vm"
)

// scopeItems lists the prerequisite keys each scope reports.
var scopeItems = map[ProbeScope][]string{
	ScopeDocker: {PrereqDocker, PrereqCompose},
	ScopeVM:     {PrereqCPUVirt, PrereqKVM, PrereqQEMU, PrereqQEMUImg, PrereqOVMF, PrereqSWTPM},
}

// Valid reports whether the scope is one the probe understands.
func (s ProbeScope) Valid() bool {
	return s == ScopeAll || s == ScopeDocker || s == ScopeVM
}

// wants reports whether this scope includes the given prerequisite key.
func (s ProbeScope) wants(key string) bool {
	if s == ScopeAll {
		return true
	}
	for _, candidate := range scopeItems[s] {
		if candidate == key {
			return true
		}
	}
	return false
}

// Prerequisite is one dependency of a deployment, and what to do when it is
// missing.
//
// Remediation is the command for this host's own distribution, not a generic
// hint. "Install swtpm" is not actionable at three in the morning;
// "apt-get install -y swtpm" is.
type Prerequisite struct {
	Key         string `json:"key"`
	Label       string `json:"label"`
	Present     bool   `json:"present"`
	Detail      string `json:"detail,omitempty"`
	Remediation string `json:"remediation,omitempty"`
	// Required marks what the chosen deploy method cannot work without, as
	// opposed to what only the compute plugins need.
	Required bool `json:"required"`
}

// PrerequisiteReport is what a probe found on one host.
type PrerequisiteReport struct {
	OS      string         `json:"os"`
	Arch    string         `json:"arch"`
	Distro  string         `json:"distro,omitempty"`
	Items   []Prerequisite `json:"items"`
	Missing []string       `json:"missing,omitempty"`
	// Escalation is how an install would run a privileged command here, so the
	// operator sees "su" or "no route" before pressing Install rather than
	// after. See escalate.go.
	Escalation Escalation `json:"escalation,omitempty"`
	// NeedsRootPassword marks the case the operator can fix from the dialog:
	// su is the only route and no root password has been supplied.
	NeedsRootPassword bool `json:"needsRootPassword,omitempty"`
	// Scope echoes the question this report answers, so a UI showing two
	// checks side by side can tell which button a late reply belongs to.
	Scope ProbeScope `json:"scope,omitempty"`
}

// packageManager describes how one distribution family installs things.
type packageManager struct {
	install  string
	packages map[string]string
}

// managers maps an os-release ID_LIKE family onto its installer.
//
// Keyed on the family rather than the distribution so a derivative — Linux
// Mint, Pop!_OS, Rocky — resolves through its parent without needing its own
// entry.
var managers = map[string]packageManager{
	"debian": {
		install: "apt-get update && apt-get install -y",
		packages: map[string]string{
			PrereqQEMU:    "qemu-system-x86",
			PrereqQEMUImg: "qemu-utils",
			PrereqSWTPM:   "swtpm swtpm-tools",
			PrereqOVMF:    "ovmf",
		},
	},
	"rhel": {
		install: "dnf install -y",
		packages: map[string]string{
			PrereqQEMU:    "qemu-kvm",
			PrereqQEMUImg: "qemu-img",
			PrereqSWTPM:   "swtpm swtpm-tools",
			PrereqOVMF:    "edk2-ovmf",
		},
	},
	"suse": {
		install: "zypper install -y",
		packages: map[string]string{
			PrereqQEMU:    "qemu-x86",
			PrereqQEMUImg: "qemu-tools",
			PrereqSWTPM:   "swtpm",
			PrereqOVMF:    "qemu-ovmf-x86_64",
		},
	},
	"arch": {
		install: "pacman -Sy --noconfirm",
		packages: map[string]string{
			PrereqQEMU:    "qemu-full",
			PrereqQEMUImg: "qemu-img",
			PrereqSWTPM:   "swtpm",
			PrereqOVMF:    "edk2-ovmf",
		},
	},
	"alpine": {
		install: "apk add --no-cache",
		packages: map[string]string{
			PrereqQEMU:    "qemu-system-x86_64",
			PrereqQEMUImg: "qemu-img",
			PrereqSWTPM:   "swtpm",
			PrereqOVMF:    "ovmf",
		},
	},
}

// dockerInstallCommand is Docker's own convenience script.
//
// Deliberately the upstream one rather than a distribution package: the distro
// packages lag badly and several ship a `docker.io` that is too old for the
// compose plugin, which would leave an operator with a "successful" install
// that cannot run a stack.
const dockerInstallCommand = "curl -fsSL https://get.docker.com | sh"

// detectDistroFamily reads /etc/os-release and returns the family this host
// installs packages like.
//
// ID_LIKE is preferred over ID because it is what derivatives set: Ubuntu says
// `debian`, Rocky says `rhel fedora`, and keying on ID alone would need an
// entry per distribution forever.
func detectDistroFamily(client *ssh.Client) (string, string) {
	out, err := runSSHCommand(client,
		". /etc/os-release 2>/dev/null && echo \"$ID|$ID_LIKE\" || echo \"|\"")
	if err != nil {
		return "", ""
	}
	parts := strings.SplitN(strings.TrimSpace(out), "|", 2)
	id := strings.TrimSpace(parts[0])
	likes := ""
	if len(parts) > 1 {
		likes = strings.TrimSpace(parts[1])
	}

	for _, candidate := range append(strings.Fields(likes), id) {
		switch candidate {
		case "debian", "ubuntu":
			return id, "debian"
		case "rhel", "fedora", "centos":
			return id, "rhel"
		case "suse", "opensuse", "opensuse-leap", "opensuse-tumbleweed":
			return id, "suse"
		case "arch", "archlinux":
			return id, "arch"
		case "alpine":
			return id, "alpine"
		}
	}
	return id, ""
}

// ProbePrerequisites reports what a host is missing for the requested deploy.
//
// Read-only: every check is a `command -v` or a file test, and nothing is
// installed here. A scoped probe answers one question and skips the rest, so
// the Docker button does not pay for the QEMU checks.
//
// Required means "required for what was asked": in ScopeVM the hypervisor set
// blocks virtual machines and is marked as such, while the deploy-time probe
// leaves it optional because a container-only host is complete without it.
func ProbePrerequisites(client *ssh.Client, method DeployMethod, priv privilege, scope ProbeScope) PrerequisiteReport {
	osName, arch, osErr := detectRemoteOS(client)
	if osErr != nil {
		slog.Debug("agent prereqs: remote OS detection failed", "error", osErr)
	}
	distro, family := detectDistroFamily(client)
	report := PrerequisiteReport{
		OS:                osName,
		Arch:              arch,
		Distro:            distro,
		Escalation:        priv.Mode,
		NeedsRootPassword: priv.NeedsRootPassword,
		Scope:             scope,
	}

	has := func(binary string) bool {
		out, err := runSSHCommand(client,
			fmt.Sprintf("command -v %s >/dev/null 2>&1 && echo yes || echo no", binary))
		return err == nil && strings.TrimSpace(out) == "yes"
	}

	test := func(command string) bool {
		out, err := runSSHCommand(client, command+" && echo yes || echo no")
		return err == nil && strings.TrimSpace(out) == "yes"
	}

	add := func(key, label, binary string, required bool) {
		if !scope.wants(key) {
			return
		}
		item := Prerequisite{Key: key, Label: label, Present: has(binary), Required: required}
		if !item.Present {
			item.Remediation = remediationFor(key, family)
			report.Missing = append(report.Missing, key)
		}
		report.Items = append(report.Items, item)
	}

	// Docker is required only for the container deploy. A binary deploy runs
	// the agent under systemd and needs nothing from Docker.
	add(PrereqDocker, "Docker Engine", "docker", method == DeployDocker || scope == ScopeDocker)

	if scope.wants(PrereqCompose) {
		compose := Prerequisite{Key: PrereqCompose, Label: "Docker Compose plugin", Required: false}
		if out, err := runSSHCommand(client,
			"docker compose version >/dev/null 2>&1 && echo yes || echo no"); err == nil {
			compose.Present = strings.TrimSpace(out) == "yes"
		}
		if !compose.Present {
			compose.Remediation = dockerInstallCommand
			compose.Detail = "Stacks need the compose plugin."
			report.Missing = append(report.Missing, PrereqCompose)
		}
		report.Items = append(report.Items, compose)
	}

	// The compute set. Optional for a deploy — they are what the Virtual
	// Machines plugin needs afterwards, and a container-only host is complete
	// without them — but each one blocks a VM, so a ScopeVM probe says so.
	vmRequired := scope == ScopeVM

	// The CPU flag is reported before /dev/kvm because it is what separates
	// "virtualization is off in firmware" from "silicon that never had it",
	// and neither is fixable from here.
	if scope.wants(PrereqCPUVirt) {
		virt := Prerequisite{
			Key:     PrereqCPUVirt,
			Label:   "CPU virtualization (VT-x / AMD-V)",
			Present: test("grep -Eqm1 '(vmx|svm)' /proc/cpuinfo"),
		}
		if !virt.Present {
			virt.Detail = "The CPU does not advertise VT-x or AMD-V. On a physical host this is " +
				"almost always disabled in BIOS/UEFI; in a guest it means nested virtualization is off."
			report.Missing = append(report.Missing, PrereqCPUVirt)
		}
		report.Items = append(report.Items, virt)
	}

	if scope.wants(PrereqKVM) {
		kvm := Prerequisite{Key: PrereqKVM, Label: "/dev/kvm", Required: vmRequired}
		switch {
		case test("test -r /dev/kvm && test -w /dev/kvm"):
			kvm.Present = true
		case test("test -e /dev/kvm"):
			kvm.Detail = "/dev/kvm exists but this user cannot open it. Add the account the agent " +
				"runs as to the kvm group."
			kvm.Remediation = "usermod -aG kvm $USER"
		default:
			kvm.Detail = "/dev/kvm does not exist. The kvm module is not loaded, or hardware " +
				"virtualization is disabled in firmware."
			kvm.Remediation = "modprobe kvm_intel || modprobe kvm_amd"
		}
		if !kvm.Present {
			report.Missing = append(report.Missing, PrereqKVM)
		}
		report.Items = append(report.Items, kvm)
	}

	add(PrereqQEMU, "QEMU system emulator", "qemu-system-x86_64", vmRequired)
	add(PrereqQEMUImg, "qemu-img", "qemu-img", vmRequired)

	if scope.wants(PrereqOVMF) {
		ovmf := Prerequisite{Key: PrereqOVMF, Label: "OVMF UEFI firmware", Required: false}
		if out, err := runSSHCommand(client,
			"ls /usr/share/OVMF/OVMF_CODE*.fd /usr/share/edk2/ovmf/OVMF_CODE*.fd "+
				">/dev/null 2>&1 && echo yes || echo no"); err == nil {
			ovmf.Present = strings.TrimSpace(out) == "yes"
		}
		if !ovmf.Present {
			ovmf.Remediation = remediationFor(PrereqOVMF, family)
			ovmf.Detail = "UEFI guests, and every Windows guest, need it."
			report.Missing = append(report.Missing, PrereqOVMF)
		}
		report.Items = append(report.Items, ovmf)
	}

	add(PrereqSWTPM, "swtpm (TPM 2.0 for guests)", "swtpm", false)

	return report
}

// remediationFor builds the install command for one prerequisite on one family.
//
// An unknown family returns an empty string rather than a guess: a wrong
// command pasted into a root shell is worse than no command.
func remediationFor(key, family string) string {
	if key == PrereqDocker || key == PrereqCompose {
		return dockerInstallCommand
	}
	manager, ok := managers[family]
	if !ok {
		return ""
	}
	pkg, ok := manager.packages[key]
	if !ok {
		return ""
	}
	return manager.install + " " + pkg
}

// InstallPrerequisites runs the remediation for each key the operator agreed to,
// and returns the combined transcript.
//
// Only keys present in `wanted` are touched, and each is re-checked against the
// probe first: a prerequisite that appeared while the dialog was open is left
// alone rather than reinstalled. Privileged commands run through whatever
// escalation the probe detected — root, sudo or su — so a host without sudo
// installs the same way one with it does.
func InstallPrerequisites(client *ssh.Client, report PrerequisiteReport, wanted []string, priv privilege) (string, error) {
	if len(wanted) == 0 {
		return "", nil
	}
	if priv.Mode == EscalateUnavailable {
		return "", errEscalationUnavailable
	}
	agreed := make(map[string]bool, len(wanted))
	for _, key := range wanted {
		agreed[key] = true
	}

	var transcript strings.Builder
	for _, item := range report.Items {
		if item.Present || !agreed[item.Key] || item.Remediation == "" {
			continue
		}
		transcript.WriteString("$ " + item.Remediation + "\n")
		out, err := priv.run(client, item.Remediation)
		transcript.WriteString(out)
		if err != nil {
			return transcript.String(), fmt.Errorf("installing %s: %w", item.Key, err)
		}
		transcript.WriteString("\n")
	}
	return transcript.String(), nil
}
