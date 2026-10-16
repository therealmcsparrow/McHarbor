// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package system

import "time"

// OSLogResult contains a bounded host log snapshot.
//
// Notices carries warning codes only - things the operator should act on.
// The discovery output the host script also produces (which log files exist,
// which one was picked as a fallback) is informational and lives in its own
// fields, because rendering it as a warning made every successful read look
// like a failure.
type OSLogResult struct {
	Source string   `json:"source"`
	Tail   int      `json:"tail"`
	Lines  []string `json:"lines"`
	// Notices holds warning codes: permission_denied, no_supported_log_source,
	// no_var_log_dir, no_journalctl, journalctl_failed.
	Notices []string `json:"notices"`
	// AvailableFiles lists log file names discovered under the host's
	// /var/log, so the operator can see what the host actually exposes. On an
	// agent environment it is empty except for a target-less qemu read, which
	// answers with the guests that have a log.
	AvailableFiles []string `json:"availableFiles"`
	// Targets answers a target-less qemu read with the guests McHarbor runs on
	// the host, each with the name it was started under.
	Targets []LogTarget `json:"targets,omitempty"`
	// FallbackFile is set when no configured candidate matched and the
	// largest *.log on the host was read instead.
	FallbackFile string    `json:"fallbackFile"`
	FetchedAt    time.Time `json:"fetchedAt"`
}

// LogTarget is one guest a qemu log read can name: ID goes back as the target,
// Label is the name the guest was started under.
type LogTarget struct {
	ID    string `json:"id"`
	Label string `json:"label,omitempty"`
}

// OSUpdateCheckResult describes available host OS package updates.
type OSUpdateCheckResult struct {
	Manager   string    `json:"manager"`
	Available bool      `json:"available"`
	Updates   []string  `json:"updates"`
	Output    string    `json:"output"`
	CheckedAt time.Time `json:"checkedAt"`
}

// HostOSInfo describes the operating system an environment's host runs.
//
// Unsupported carries the agent's own explanation when the host cannot be
// inspected at all — a containerised agent with no /host mount, typically —
// because "unknown" everywhere would read as a bug rather than a setup choice.
type HostOSInfo struct {
	PrettyName     string `json:"prettyName,omitempty"`
	Distro         string `json:"distro,omitempty"`
	Version        string `json:"version,omitempty"`
	Kernel         string `json:"kernel,omitempty"`
	Arch           string `json:"arch,omitempty"`
	Hostname       string `json:"hostname,omitempty"`
	UptimeSeconds  int64  `json:"uptimeSeconds,omitempty"`
	Manager        string `json:"manager,omitempty"`
	Virtualization string `json:"virtualization,omitempty"`
	RebootRequired bool   `json:"rebootRequired"`
	Unsupported    string `json:"unsupported,omitempty"`
}

// HostUpdateSchedule is when the automatic package check should run, and how
// far an apply on this host is allowed to go.
type HostUpdateSchedule struct {
	Enabled bool     `json:"enabled"`
	Time    string   `json:"time"`
	Days    []string `json:"days"`
	// WithNewPackages lets apt pull in dependencies it does not have yet, which
	// is the only way a kernel ABI bump ever applies. It is the operator's call
	// because it turns an upgrade into one that installs packages nobody listed.
	WithNewPackages bool `json:"withNewPackages"`
	// SkipUnavailable lets an upgrade carry on past a package its mirror no
	// longer serves (apt --fix-missing, dnf/yum --skip-broken). zypper, apk and
	// pacman have no safe equivalent and ignore it.
	SkipUnavailable bool `json:"skipUnavailable"`
	// RemoveUnused runs the manager's autoremove after a successful upgrade
	// (apt, dnf, yum autoremove; pacman orphans). apk and zypper have no safe
	// equivalent and ignore it.
	RemoveUnused bool `json:"removeUnused"`
}

// HostUpdateState is the cached result of the last check plus the schedule that
// produced it, so the tab renders without running a package manager.
type HostUpdateState struct {
	Schedule      HostUpdateSchedule `json:"schedule"`
	Available     bool               `json:"available"`
	Count         int                `json:"count"`
	Manager       string             `json:"manager,omitempty"`
	Updates       []string           `json:"updates,omitempty"`
	LastCheckedAt *time.Time         `json:"lastCheckedAt,omitempty"`
	RunningJobID  string             `json:"runningJobId,omitempty"`
	// RecoveryUnsupported is set when the environment's agent predates Skip
	// unavailable packages and the one-shot download retry, so the tab can say so.
	RecoveryUnsupported bool `json:"recoveryUnsupported,omitempty"`
}

// HostUpdateJobStatus is one apply job as the progress stream reports it.
//
// Hint names the option that would have let a failed run through, for the UI to
// translate; "skipUnavailable" is the only value today.
type HostUpdateJobStatus struct {
	JobID     string    `json:"jobId"`
	Lines     []string  `json:"lines"`
	Offset    int       `json:"offset"`
	Done      bool      `json:"done"`
	Success   bool      `json:"success"`
	Error     string    `json:"error,omitempty"`
	Hint      string    `json:"hint,omitempty"`
	StartedAt time.Time `json:"startedAt"`
}

// OSUpdateApplyRequest confirms the user intentionally started an OS update.
//
// The three options override the environment's stored defaults for this run
// only; absent, the stored defaults apply.
type OSUpdateApplyRequest struct {
	Confirm         bool  `json:"confirm"`
	WithNewPackages *bool `json:"withNewPackages,omitempty"`
	SkipUnavailable *bool `json:"skipUnavailable,omitempty"`
	RemoveUnused    *bool `json:"removeUnused,omitempty"`
}

// OSUpdateApplyResult contains the package-manager update run result.
type OSUpdateApplyResult struct {
	Manager  string    `json:"manager"`
	ExitCode int64     `json:"exitCode"`
	Success  bool      `json:"success"`
	Output   string    `json:"output"`
	RanAt    time.Time `json:"ranAt"`
}

// RestartResult describes the scheduled McHarbor self-restart. The
// call returns as soon as the Docker restart request has been
// queued; the actual container stop/start happens asynchronously
// and the response shown to the user reflects the planned outcome.
type RestartResult struct {
	ContainerID   string    `json:"containerId"`
	ContainerName string    `json:"containerName"`
	ScheduledAt   time.Time `json:"scheduledAt"`
}

// ResetRequest carries the explicit double-confirmation string the
// user must type to authorise a factory reset. The backend rejects
// the request when `Confirm` does not equal `RESET`.
type ResetRequest struct {
	Confirm string `json:"confirm"`
}

// ResetResult mirrors RestartResult for the factory-reset endpoint.
type ResetResult struct {
	ContainerID   string    `json:"containerId"`
	ContainerName string    `json:"containerName"`
	ScheduledAt   time.Time `json:"scheduledAt"`
}
