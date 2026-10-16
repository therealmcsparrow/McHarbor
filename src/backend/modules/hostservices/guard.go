// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// Package hostservices manages a native agent host's systemd services: the
// list, lifecycle actions, drop-in overrides and daemon-reload.
package hostservices

import (
	"errors"
	"regexp"
	"strings"

	coreagent "github.com/therealmcsparrow/mcharbor/core/agent"
)

// unitPattern is the only unit name shape accepted. It admits no path
// separator, so a name can never climb out of the drop-in directory.
var unitPattern = regexp.MustCompile(`^[A-Za-z0-9@._:-]+\.service$`)

// unitMaxLen bounds a unit name well above systemd's own limit.
const unitMaxLen = 256

// overrideMaxBytes bounds a drop-in the server will forward to the agent.
const overrideMaxBytes = 64 << 10

// ErrInvalidUnit is returned for any name ValidateUnit refuses.
var ErrInvalidUnit = errors.New("invalid unit name")

// ValidateUnit refuses anything but a plain .service unit name. The agent runs
// the same check; a leading dash would otherwise reach systemctl as an option.
func ValidateUnit(name string) error {
	if name == "" || len(name) > unitMaxLen {
		return ErrInvalidUnit
	}
	if strings.ContainsAny(name, `/\`) || strings.Contains(name, "..") {
		return ErrInvalidUnit
	}
	if strings.HasPrefix(name, "-") || strings.HasPrefix(name, ".") {
		return ErrInvalidUnit
	}
	if !unitPattern.MatchString(name) {
		return ErrInvalidUnit
	}
	return nil
}

// Protection names why a unit may not be stopped or disabled from McHarbor, or
// returns "" when it may. It must match hostServiceProtection in the agent:
// both sides refuse, so neither a patched server nor a forged request alone can
// cut the host off from the operator.
func Protection(unit string) string {
	switch {
	case strings.HasPrefix(unit, "mcharbor-agent"):
		return "agent"
	case unit == "ssh.service" || unit == "sshd.service":
		return "ssh"
	case strings.HasPrefix(unit, "systemd-"):
		return "systemd"
	case strings.HasPrefix(unit, "dbus"):
		return "dbus"
	case unit == "docker.service" || unit == "containerd.service":
		return "runtime"
	}
	return ""
}

// IsAgentUnit reports whether unit is the McHarbor agent's own service, whose
// restart drops the connection and reconnects.
func IsAgentUnit(unit string) bool {
	return strings.HasPrefix(unit, "mcharbor-agent")
}

// ValidAction reports whether action is one the Services tab offers.
func ValidAction(action string) bool {
	switch action {
	case coreagent.HostServiceActionStart, coreagent.HostServiceActionStop, coreagent.HostServiceActionRestart,
		coreagent.HostServiceActionEnable, coreagent.HostServiceActionDisable:
		return true
	}
	return false
}

// ActionRefused reports whether action on unit is refused by the guard: stop,
// disable and mask on a protected unit. Restart stays allowed because it brings
// the unit straight back.
func ActionRefused(unit, action string) bool {
	return actionRefusedBy(Protection, unit, action)
}

// SelfProtection is Protection for the machine McHarbor itself runs on: the same
// list, plus McHarbor's own units, and the runtime named as the one McHarbor runs on.
func SelfProtection(unit string) string {
	switch {
	case strings.HasPrefix(unit, "mcharbor") && !IsAgentUnit(unit):
		return "mcharbor"
	case unit == "docker.service" || unit == "containerd.service":
		return "self_runtime"
	}
	return Protection(unit)
}

// RestartsMcHarbor reports whether restarting unit on McHarbor's own machine
// takes McHarbor down with it, so the restart is queued rather than awaited.
func RestartsMcHarbor(unit string) bool {
	switch SelfProtection(unit) {
	case "mcharbor", "self_runtime":
		return true
	}
	return false
}

// actionRefusedBy applies the stop/disable/mask rule under a given guard, so the
// environment and self hosts share one rule with different protected lists.
func actionRefusedBy(protect func(string) string, unit, action string) bool {
	switch action {
	case coreagent.HostServiceActionStop, coreagent.HostServiceActionDisable, "mask":
		return protect(unit) != ""
	}
	return false
}
