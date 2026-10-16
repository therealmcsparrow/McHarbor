// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package compliancepacks

import "sort"

// FrameworkCIS is the CIS Docker Benchmark, the first framework a pack covers.
// The framework name is a proper noun and is never translated.
const FrameworkCIS = "cis-docker-benchmark"

// Coverage is how much of a control McHarbor can answer. It is declared per
// control rather than inferred, because the honest half of a pack is the set of
// controls it cannot check.
type Coverage string

// Coverage levels, worst-informed last.
const (
	// CoverageMechanical means a check reads live state and produces a verdict.
	CoverageMechanical Coverage = "mechanical"
	// CoverageEvidenced means McHarbor records the evidence an auditor would ask
	// for but does not judge it. The pack lists the control and names the source.
	CoverageEvidenced Coverage = "evidenced"
	// CoverageNotCovered means McHarbor cannot see the control at all. Saying so is
	// the point; guessing is the failure this whole feature exists to avoid.
	CoverageNotCovered Coverage = "notCovered"
)

// EvidenceKind says what sort of thing an evidence source is, so a mapping
// document can be read without opening the code.
type EvidenceKind string

// Evidence kinds.
const (
	EvidenceDockerAPI EvidenceKind = "dockerApi"
	EvidenceRoute     EvidenceKind = "route"
	EvidenceTable     EvidenceKind = "table"
	EvidenceSetting   EvidenceKind = "setting"
	EvidenceAbsent    EvidenceKind = "absent"
)

// EvidenceSource is one place a control's answer comes from. Ref is written the
// way an auditor can act on it — a route as mounted, a table name, an environment
// variable — never a prose paraphrase.
type EvidenceSource struct {
	Key   string       `json:"key"`
	Kind  EvidenceKind `json:"kind"`
	Ref   string       `json:"ref"`
	Notes string       `json:"notes,omitempty"`
}

// Evidence source keys. A control cites these; controls_test.go asserts every
// cited key resolves and that no source is orphaned, because a control pointing at
// evidence that does not exist is the defect this feature can most easily ship.
const (
	EvDockerInfo    = "docker.info"
	EvDockerBridge  = "docker.network.bridge"
	EvDockerInspect = "docker.container.inspect"
	EvEnvConnection = "environments.connection_type"
	EvAuditLog      = "audit_logs"
	EvScans         = "scans"
	EvImages        = "images"

	EvAbsentHostFS       = "absent.hostFilesystem"
	EvAbsentHostAudit    = "absent.hostAuditd"
	EvAbsentOrganisation = "absent.organisational"
)

// evidenceCatalog is the set of sources the CIS pack's controls cite, and nothing
// more. controls_test.go asserts both directions — every cited key resolves and no
// key is orphaned — so a source kept here after its control was removed is a test
// failure rather than a mapping document claiming McHarbor reads something no
// control reads. The evidence behind SOC 2 and ISO 27001 lives in their mapping
// documents, because neither has a check runner to cite it from.
func evidenceCatalog() []EvidenceSource {
	return []EvidenceSource{
		{EvDockerInfo, EvidenceDockerAPI, "GET /info", "Docker Engine daemon configuration as the daemon reports it"},
		{EvDockerBridge, EvidenceDockerAPI, "GET /networks/bridge", "Options on the default bridge network"},
		{EvDockerInspect, EvidenceDockerAPI, "GET /containers/{id}/json", "Per-container runtime configuration"},
		{EvEnvConnection, EvidenceTable, "environments.connection_type", "socket, tcp, tls, ssh or podman as configured in McHarbor"},
		{EvAuditLog, EvidenceTable, "audit_logs", "Who did what to which object, with the actor recorded"},
		{EvScans, EvidenceTable, "scans", "Image vulnerability scan results, per image"},
		{EvImages, EvidenceDockerAPI, "GET /images/json", "Image inventory, including unused images"},

		{EvAbsentHostFS, EvidenceAbsent, "none",
			"McHarbor has no view of the Docker host's filesystem: partitions, file ownership and file permissions are outside what it can read"},
		{EvAbsentHostAudit, EvidenceAbsent, "none",
			"auditd rules on the Docker host are not readable from McHarbor; its own audit log is a different record and does not substitute"},
		{EvAbsentOrganisation, EvidenceAbsent, "none",
			"A policy or process control with no technical state to read"},
	}
}

// Scope is what a control's check reads.
type Scope string

// Control scopes.
const (
	// ScopeDaemon checks read the daemon measurement once.
	ScopeDaemon Scope = "daemon"
	// ScopeContainer checks read every container and aggregate the violations.
	ScopeContainer Scope = "container"
	// ScopeNone marks a control with no check: evidenced or not covered.
	ScopeNone Scope = "none"
)

// Control is one framework control. Title and Requirement are English reference
// text for the mapping document and the report; the frontend translates the
// control through its i18n bundle keyed by ID, the way a posture section is.
type Control struct {
	ID          string   `json:"id"`
	Group       string   `json:"group"`
	Title       string   `json:"title"`
	Requirement string   `json:"requirement"`
	Coverage    Coverage `json:"coverage"`
	Scope       Scope    `json:"scope"`
	Evidence    []string `json:"evidence"`

	// daemonCheck and containerCheck are the mechanical half. Exactly one is set
	// on a CoverageMechanical control and neither on any other, which
	// controls_test.go asserts: a mechanical control with no check would silently
	// disappear from every run.
	daemonCheck    func(Inputs) Result
	containerCheck func(Inputs) Result
}

// cisControls is the CIS Docker Benchmark registry: a data table keyed by control
// ID rather than a switch, so adding a control is one entry and a missing check is
// a test failure instead of an empty row.
//
// Group names match the benchmark's own section names. Sections 1 and 3 are host
// filesystem controls and section 6 is organisational; all three are declared here
// as not covered rather than left out, because a pack that lists only what it can
// check reads as a clean bill of health.
func cisControls() []Control {
	return []Control{
		// --- 1 Host configuration: McHarbor has no host filesystem view ---
		{ID: "1.1.1", Group: "host", Title: "A separate partition for containers has been created",
			Requirement: "/var/lib/docker is on its own partition or logical volume.",
			Coverage:    CoverageNotCovered, Scope: ScopeNone, Evidence: []string{EvAbsentHostFS}},
		{ID: "1.1.4", Group: "host", Title: "Auditing is configured for Docker files and directories",
			Requirement: "auditd rules cover /var/lib/docker, /etc/docker and the daemon unit files.",
			Coverage:    CoverageNotCovered, Scope: ScopeNone, Evidence: []string{EvAbsentHostAudit}},
		{ID: "1.2.1", Group: "host", Title: "The Docker Engine version is recorded",
			Requirement: "The engine is kept up to date; the running version is known.",
			Coverage:    CoverageMechanical, Scope: ScopeDaemon, Evidence: []string{EvDockerInfo},
			daemonCheck: checkEngineVersion},

		// --- 2 Docker daemon configuration ---
		{ID: "2.1", Group: "daemon", Title: "Network traffic between containers on the default bridge is restricted",
			Requirement: "The default bridge sets com.docker.network.bridge.enable_icc to false.",
			Coverage:    CoverageMechanical, Scope: ScopeDaemon, Evidence: []string{EvDockerBridge},
			daemonCheck: checkBridgeICC},
		{ID: "2.2", Group: "daemon", Title: "The daemon logging level is info",
			Requirement: "The daemon does not run with debug logging, which records request detail indefinitely.",
			Coverage:    CoverageMechanical, Scope: ScopeDaemon, Evidence: []string{EvDockerInfo},
			daemonCheck: checkDaemonDebug},
		{ID: "2.5", Group: "daemon", Title: "A supported storage driver is in use",
			Requirement: "The daemon does not use aufs or the legacy devicemapper loopback driver.",
			Coverage:    CoverageMechanical, Scope: ScopeDaemon, Evidence: []string{EvDockerInfo},
			daemonCheck: checkStorageDriver},
		{ID: "2.6", Group: "daemon", Title: "The daemon API is not reachable without authentication",
			Requirement: "A daemon exposed over TCP uses TLS client authentication; a local socket is not exposed at all.",
			Coverage:    CoverageMechanical, Scope: ScopeDaemon, Evidence: []string{EvEnvConnection},
			daemonCheck: checkDaemonTransport},
		{ID: "2.8", Group: "daemon", Title: "User namespace support is enabled",
			Requirement: "The daemon runs with userns-remap so a container root is not the host root.",
			Coverage:    CoverageMechanical, Scope: ScopeDaemon, Evidence: []string{EvDockerInfo},
			daemonCheck: checkUserNamespace},
		{ID: "2.11", Group: "daemon", Title: "Authorization for Docker client commands is enabled",
			Requirement: "An authorization plugin decides daemon requests.",
			Coverage:    CoverageMechanical, Scope: ScopeDaemon, Evidence: []string{EvDockerInfo},
			daemonCheck: checkAuthorizationPlugin},
		{ID: "2.12", Group: "daemon", Title: "Centralized and remote logging is configured",
			Requirement: "The default logging driver sends container output off the host.",
			Coverage:    CoverageMechanical, Scope: ScopeDaemon, Evidence: []string{EvDockerInfo},
			daemonCheck: checkRemoteLogging},
		{ID: "2.13", Group: "daemon", Title: "Live restore is enabled",
			Requirement: "Containers keep running while the daemon is unavailable.",
			Coverage:    CoverageMechanical, Scope: ScopeDaemon, Evidence: []string{EvDockerInfo},
			daemonCheck: checkLiveRestore},
		{ID: "2.15", Group: "daemon", Title: "A seccomp profile is applied by default",
			Requirement: "The daemon's default seccomp profile is not disabled.",
			Coverage:    CoverageMechanical, Scope: ScopeDaemon, Evidence: []string{EvDockerInfo},
			daemonCheck: checkDaemonSeccomp},
		{ID: "2.17", Group: "daemon", Title: "Experimental features are not enabled",
			Requirement: "The daemon is not an experimental build.",
			Coverage:    CoverageMechanical, Scope: ScopeDaemon, Evidence: []string{EvDockerInfo},
			daemonCheck: checkExperimental},

		// --- 3 Daemon configuration files: host filesystem again ---
		{ID: "3.15", Group: "files", Title: "Docker socket file ownership and permissions are correct",
			Requirement: "/var/run/docker.sock is owned by root:docker with mode 660 or stricter.",
			Coverage:    CoverageNotCovered, Scope: ScopeNone, Evidence: []string{EvAbsentHostFS}},
		{ID: "3.17", Group: "files", Title: "daemon.json ownership and permissions are correct",
			Requirement: "/etc/docker/daemon.json is owned by root:root with mode 644 or stricter.",
			Coverage:    CoverageNotCovered, Scope: ScopeNone, Evidence: []string{EvAbsentHostFS}},

		// --- 4 Container images and build files ---
		{ID: "4.1", Group: "images", Title: "Containers run as a non-root user",
			Requirement: "A container declares a user rather than running as root.",
			Coverage:    CoverageMechanical, Scope: ScopeContainer, Evidence: []string{EvDockerInspect},
			containerCheck: checkContainerUser},
		{ID: "4.5", Group: "images", Title: "Image provenance is verified before deployment",
			Requirement: "Content trust or an equivalent signature check gates what may run.",
			Coverage:    CoverageNotCovered, Scope: ScopeNone, Evidence: []string{EvAbsentOrganisation},
			// DOCKER_CONTENT_TRUST is a client-side environment variable on whatever
			// machine pulled the image; a daemon cannot report it after the fact.
		},
		{ID: "4.6", Group: "images", Title: "Containers declare a health check",
			Requirement: "Every container has a HEALTHCHECK, from its image or its run configuration.",
			Coverage:    CoverageMechanical, Scope: ScopeContainer, Evidence: []string{EvDockerInspect},
			containerCheck: checkContainerHealthcheck},

		// --- 5 Container runtime ---
		{ID: "5.3", Group: "runtime", Title: "Linux kernel capabilities are restricted",
			Requirement: "No container adds capabilities beyond Docker's default set.",
			Coverage:    CoverageMechanical, Scope: ScopeContainer, Evidence: []string{EvDockerInspect},
			containerCheck: checkContainerCapAdd},
		{ID: "5.4", Group: "runtime", Title: "Privileged containers are not used",
			Requirement: "No container runs with --privileged.",
			Coverage:    CoverageMechanical, Scope: ScopeContainer, Evidence: []string{EvDockerInspect},
			containerCheck: checkContainerPrivileged},
		{ID: "5.7", Group: "runtime", Title: "Privileged ports are not mapped",
			Requirement: "No container publishes to a host port below 1024.",
			Coverage:    CoverageMechanical, Scope: ScopeContainer, Evidence: []string{EvDockerInspect},
			containerCheck: checkContainerPrivilegedPorts},
		{ID: "5.9", Group: "runtime", Title: "The host network namespace is not shared",
			Requirement: "No container runs with --network=host.",
			Coverage:    CoverageMechanical, Scope: ScopeContainer, Evidence: []string{EvDockerInspect},
			containerCheck: checkContainerHostNetwork},
		{ID: "5.10", Group: "runtime", Title: "Container memory usage is limited",
			Requirement: "Every container has a memory limit, so one cannot exhaust the host.",
			Coverage:    CoverageMechanical, Scope: ScopeContainer, Evidence: []string{EvDockerInspect},
			containerCheck: checkContainerMemory},
		{ID: "5.11", Group: "runtime", Title: "CPU priority is set",
			Requirement: "Every container has CPU shares or a CPU quota.",
			Coverage:    CoverageMechanical, Scope: ScopeContainer, Evidence: []string{EvDockerInspect},
			containerCheck: checkContainerCPU},
		{ID: "5.12", Group: "runtime", Title: "The container root filesystem is mounted read only",
			Requirement: "Every container runs with a read-only root filesystem and explicit writable volumes.",
			Coverage:    CoverageMechanical, Scope: ScopeContainer, Evidence: []string{EvDockerInspect},
			containerCheck: checkContainerReadonlyRootfs},
		{ID: "5.14", Group: "runtime", Title: "The on-failure restart policy is bounded",
			Requirement: "A restarting container uses on-failure with at most five attempts, not always.",
			Coverage:    CoverageMechanical, Scope: ScopeContainer, Evidence: []string{EvDockerInspect},
			containerCheck: checkContainerRestartPolicy},
		{ID: "5.15", Group: "runtime", Title: "The host process namespace is not shared",
			Requirement: "No container runs with --pid=host.",
			Coverage:    CoverageMechanical, Scope: ScopeContainer, Evidence: []string{EvDockerInspect},
			containerCheck: checkContainerHostPID},
		{ID: "5.16", Group: "runtime", Title: "The host IPC namespace is not shared",
			Requirement: "No container runs with --ipc=host.",
			Coverage:    CoverageMechanical, Scope: ScopeContainer, Evidence: []string{EvDockerInspect},
			containerCheck: checkContainerHostIPC},
		{ID: "5.17", Group: "runtime", Title: "Host devices are not exposed directly",
			Requirement: "No container is given a host device with --device.",
			Coverage:    CoverageMechanical, Scope: ScopeContainer, Evidence: []string{EvDockerInspect},
			containerCheck: checkContainerDevices},
		{ID: "5.19", Group: "runtime", Title: "Mount propagation is not shared",
			Requirement: "No bind mount uses shared propagation, which lets a container write to the host mount tree.",
			Coverage:    CoverageMechanical, Scope: ScopeContainer, Evidence: []string{EvDockerInspect},
			containerCheck: checkContainerMountPropagation},
		{ID: "5.20", Group: "runtime", Title: "The host UTS namespace is not shared",
			Requirement: "No container runs with --uts=host.",
			Coverage:    CoverageMechanical, Scope: ScopeContainer, Evidence: []string{EvDockerInspect},
			containerCheck: checkContainerHostUTS},
		{ID: "5.21", Group: "runtime", Title: "The default seccomp profile is not disabled",
			Requirement: "No container runs with seccomp=unconfined.",
			Coverage:    CoverageMechanical, Scope: ScopeContainer, Evidence: []string{EvDockerInspect},
			containerCheck: checkContainerSeccomp},
		{ID: "5.25", Group: "runtime", Title: "Containers cannot acquire additional privileges",
			Requirement: "Every container runs with no-new-privileges.",
			Coverage:    CoverageMechanical, Scope: ScopeContainer, Evidence: []string{EvDockerInspect},
			containerCheck: checkContainerNoNewPrivileges},
		{ID: "5.28", Group: "runtime", Title: "The PIDs cgroup limit is used",
			Requirement: "Every container has a PIDs limit, so a fork bomb cannot exhaust the host.",
			Coverage:    CoverageMechanical, Scope: ScopeContainer, Evidence: []string{EvDockerInspect},
			containerCheck: checkContainerPIDsLimit},
		{ID: "5.31", Group: "runtime", Title: "The Docker socket is not mounted inside containers",
			Requirement: "No container binds /var/run/docker.sock, which is equivalent to host root.",
			Coverage:    CoverageMechanical, Scope: ScopeContainer, Evidence: []string{EvDockerInspect},
			containerCheck: checkContainerDockerSocket},

		// --- 6 Security operations: process controls, evidenced not judged ---
		{ID: "6.1", Group: "operations", Title: "Image sprawl is avoided",
			Requirement: "Unused images are removed rather than accumulating on hosts.",
			Coverage:    CoverageEvidenced, Scope: ScopeNone, Evidence: []string{EvImages, EvScans}},
		{ID: "6.2", Group: "operations", Title: "Container sprawl is avoided",
			Requirement: "Stopped and abandoned containers are removed.",
			Coverage:    CoverageEvidenced, Scope: ScopeNone, Evidence: []string{EvDockerInspect, EvAuditLog}},

		// --- 7 Swarm ---
		{ID: "7.1", Group: "swarm", Title: "Swarm mode is not enabled unless it is needed",
			Requirement: "A host that does not orchestrate with Swarm leaves Swarm mode inactive.",
			Coverage:    CoverageMechanical, Scope: ScopeDaemon, Evidence: []string{EvDockerInfo},
			daemonCheck: checkSwarmMode},
	}
}

// Framework describes one pack: its controls and the evidence sources they cite.
// The name is the framework's own proper noun and is not translated.
type Framework struct {
	Key         string           `json:"key"`
	Name        string           `json:"name"`
	Description string           `json:"description"`
	Controls    []Control        `json:"controls"`
	Evidence    []EvidenceSource `json:"evidence"`
	// Runnable is false for a framework with no mechanical checks, which is how
	// SOC 2 and ISO 27001 are declared: a mapping, not a check suite.
	Runnable bool `json:"runnable"`
}

// Frameworks returns every pack McHarbor ships. Only the CIS Docker Benchmark has
// a check runner; SOC 2 and ISO 27001 are evidence mappings and live in
// .documentation/compliance/ rather than here, because neither has mechanical
// state to read and a stub check suite would imply otherwise.
func Frameworks() []Framework {
	return []Framework{
		{
			Key:  FrameworkCIS,
			Name: "CIS Docker Benchmark",
			Description: "Evidence for the CIS Docker Benchmark: daemon and container-runtime checks read from " +
				"a live Docker daemon, plus the controls McHarbor cannot see.",
			Controls: cisControls(),
			Evidence: evidenceCatalog(),
			Runnable: true,
		},
	}
}

// FrameworkByKey returns a framework by key, or false. An unknown key is a bad
// request, not a silently empty pack.
func FrameworkByKey(key string) (Framework, bool) {
	for _, f := range Frameworks() {
		if f.Key == key {
			return f, true
		}
	}
	return Framework{}, false
}

// ControlOrder is the order a run's results are read back in: by group as the
// benchmark numbers them, then by control ID numerically rather than
// lexically, so 5.9 precedes 5.10.
func ControlOrder(f Framework) map[string]int {
	ids := make([]string, 0, len(f.Controls))
	for _, c := range f.Controls {
		ids = append(ids, c.ID)
	}
	sort.SliceStable(ids, func(i, j int) bool { return lessControlID(ids[i], ids[j]) })
	order := make(map[string]int, len(ids))
	for i, id := range ids {
		order[id] = i
	}
	return order
}

// lessControlID compares dotted control IDs segment by segment as integers, so
// "5.9" sorts before "5.10" and "1.1.1" before "1.2.1".
func lessControlID(a, b string) bool {
	as, bs := splitDots(a), splitDots(b)
	for i := 0; i < len(as) && i < len(bs); i++ {
		if as[i] != bs[i] {
			return as[i] < bs[i]
		}
	}
	return len(as) < len(bs)
}

func splitDots(s string) []int {
	out := []int{}
	n, have := 0, false
	for i := 0; i < len(s); i++ {
		if s[i] == '.' {
			out, n, have = append(out, n), 0, false
			continue
		}
		if s[i] >= '0' && s[i] <= '9' {
			n, have = n*10+int(s[i]-'0'), true
		}
	}
	if have || len(out) > 0 {
		out = append(out, n)
	}
	return out
}
