// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package hostservices

// Availability says whether the Services tab can work for an environment and,
// when it cannot, why — as data, so the tab explains itself instead of failing.
type Availability struct {
	Supported bool `json:"supported"`
	// Reason is "not_agent", "not_native", "offline", "agent_outdated" or
	// "no_systemd" when Supported is false; on McHarbor's own host it is
	// "self_unreachable" or "self_no_systemd".
	Reason string `json:"reason,omitempty"`
	// Detail is the sentence explaining a self_unreachable host, shown verbatim
	// as the host updates section shows it.
	Detail       string `json:"detail,omitempty"`
	InstallKind  string `json:"installKind,omitempty"`
	AgentVersion string `json:"agentVersion,omitempty"`
	AgentBuild   string `json:"agentBuild,omitempty"`
	MinVersion   string `json:"minVersion"`
	MinBuild     string `json:"minBuild"`
}

// Unit is one systemd service with the guard's verdict attached, so the UI
// disables a protected row's stop and disable before anyone clicks them.
type Unit struct {
	Name          string `json:"name"`
	Description   string `json:"description,omitempty"`
	LoadState     string `json:"loadState,omitempty"`
	ActiveState   string `json:"activeState,omitempty"`
	SubState      string `json:"subState,omitempty"`
	UnitFileState string `json:"unitFileState,omitempty"`
	MainPID       int    `json:"mainPid,omitempty"`
	Result        string `json:"result,omitempty"`
	FailureReason string `json:"failureReason,omitempty"`
	Template      bool   `json:"template,omitempty"`
	// Protected is the guard's reason key ("agent", "ssh", "systemd", "dbus",
	// "runtime", and on McHarbor's own host "mcharbor" or "self_runtime"),
	// empty when stop and disable are allowed.
	Protected string `json:"protected,omitempty"`
	IsAgent   bool   `json:"isAgent,omitempty"`
	// RestartsMcHarbor marks a unit on McHarbor's own host whose restart takes
	// McHarbor down with it, so the UI warns before confirming.
	RestartsMcHarbor bool `json:"restartsMcHarbor,omitempty"`
}

// UnitList is GET /host-services; not named ListResponse because that collides
// with the generated client wrapper of the hostservicesList operation.
type UnitList struct {
	Availability Availability `json:"availability"`
	Units        []Unit       `json:"units"`
	Truncated    bool         `json:"truncated,omitempty"`
}

// WarningMcHarborRestart answers a restart on McHarbor's own host that takes
// McHarbor down with it: the restart is queued, not awaited.
const WarningMcHarborRestart = "mcharbor_restart"

// ActionResult answers a lifecycle action. Warning is "agent_restart" when the
// agent restarts its own unit and the connection is about to drop, and
// "mcharbor_restart" when a self-host restart takes McHarbor down with it.
type ActionResult struct {
	Unit    string `json:"unit"`
	Action  string `json:"action"`
	Warning string `json:"warning,omitempty"`
}

// Override is the effective unit and its McHarbor-managed drop-in.
type Override struct {
	Unit      string `json:"unit"`
	Effective string `json:"effective"`
	Content   string `json:"content"`
	Exists    bool   `json:"exists"`
	Path      string `json:"path"`
	Truncated bool   `json:"truncated,omitempty"`
}

// OverrideRequest is PUT /host-services/{unit}/override.
type OverrideRequest struct {
	Content string `json:"content"`
	// Reload runs daemon-reload after the write; without it systemd keeps
	// using the old definition until the next reload.
	Reload bool `json:"reload"`
	// Verify runs systemd-analyze verify and returns its findings as a
	// warning; it never blocks the save.
	Verify bool `json:"verify"`
}

// OverrideResult answers a save.
type OverrideResult struct {
	Unit         string `json:"unit"`
	Path         string `json:"path"`
	Reloaded     bool   `json:"reloaded"`
	VerifyOutput string `json:"verifyOutput,omitempty"`
	SHA256       string `json:"sha256"`
}

// ErrorDetail carries what systemd said when it refused, so the operator sees
// the host's own reason rather than a generic failure.
type ErrorDetail struct {
	Detail string `json:"detail,omitempty"`
}
