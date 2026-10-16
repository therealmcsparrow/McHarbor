// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package compliancepacks

import "strings"

// Inputs is everything a pack run measured. Splitting the measurement from the
// verdict is what lets every check be a table test rather than a live daemon:
// gather.go fills this, checks.go reads it and nothing else.
type Inputs struct {
	// Daemon is empty and DaemonKnown false when the daemon could not be read,
	// which makes every daemon check unknown rather than falsely passing.
	DaemonKnown bool
	Daemon      Daemon

	// ContainersKnown is false when the container list itself failed. A list that
	// loaded but had individual inspects fail records the shortfall in
	// InspectFailures, so "no violations" cannot be confused with "nothing read".
	ContainersKnown  bool
	Containers       []Container
	InspectFailures  int
	ContainersCapped bool
}

// Daemon is the daemon-level measurement, from Docker's GET /info plus the default
// bridge network and the environment's configured connection type.
type Daemon struct {
	ServerVersion string
	StorageDriver string
	Debug         bool
	LoggingDriver string
	LiveRestore   bool
	Experimental  bool
	// SecurityOptions is Docker's own list, e.g. "name=seccomp,profile=builtin",
	// "name=userns", "name=apparmor". Parsed rather than matched whole because the
	// profile suffix varies between engine versions.
	SecurityOptions []string
	AuthzPlugins    []string
	SwarmState      string

	// BridgeICCKnown is false when the default bridge network could not be
	// inspected; BridgeICC is Docker's com.docker.network.bridge.enable_icc,
	// which defaults to true when the option is absent.
	BridgeICCKnown bool
	BridgeICC      bool

	// ConnectionType is the environment's configured transport: socket, tcp, tls,
	// ssh, podman, or agent for a remote agent. It is the only evidence McHarbor
	// has for whether the daemon's API is exposed on the network unauthenticated.
	ConnectionType string
}

// Container is the per-container measurement, from GET /containers/{id}/json.
// Every field is a runtime setting a CIS control names; nothing here is
// identifying beyond the name, which the operator needs to act on a finding.
type Container struct {
	Name string

	User             string
	HasHealthcheck   bool
	CapAdd           []string
	Privileged       bool
	PrivilegedPorts  []string
	NetworkMode      string
	MemoryLimit      int64
	CPUShares        int64
	NanoCPUs         int64
	ReadonlyRootfs   bool
	RestartPolicy    string
	RestartMaxRetry  int
	PIDMode          string
	IPCMode          string
	UTSMode          string
	DeviceCount      int
	SharedMounts     []string
	SecurityOpt      []string
	PIDsLimit        int64
	DockerSocketBind bool
}

// hasSecurityOption reports whether Docker's daemon security-options list carries
// a named feature, matching the "name=<feature>" prefix rather than the whole
// string, since the profile suffix differs between engine versions.
func hasSecurityOption(opts []string, name string) bool {
	for _, o := range opts {
		for _, part := range splitCommas(o) {
			if part == "name="+name {
				return true
			}
		}
	}
	return false
}

// securityOptionValue returns the value of a key inside Docker's security-options
// list, such as the seccomp profile name, or "" when the key is absent.
func securityOptionValue(opts []string, name, key string) string {
	for _, o := range opts {
		parts := splitCommas(o)
		matched := false
		for _, p := range parts {
			if p == "name="+name {
				matched = true
			}
		}
		if !matched {
			continue
		}
		for _, p := range parts {
			if v, ok := strings.CutPrefix(p, key+"="); ok && v != "" {
				return v
			}
		}
	}
	return ""
}

func splitCommas(s string) []string {
	parts := strings.Split(s, ",")
	for i := range parts {
		parts[i] = strings.TrimSpace(parts[i])
	}
	return parts
}
