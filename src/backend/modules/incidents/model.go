// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// Package incidents turns the signals McHarbor already records into incidents,
// assembles the evidence around them, and has a model narrate it — read-only,
// opt-in per environment, attributed, and bounded in cost.
package incidents

// Signals that open or extend an incident.
const (
	SignalCrashLoop  = "crash_loop"
	SignalExitRepeat = "exit_nonzero"
	SignalOOM        = "oom"
	SignalUnhealthy  = "unhealthy"
	SignalAlert      = "alert"
)

// Incident and narrative statuses.
const (
	StatusOpen     = "open"
	StatusResolved = "resolved"

	NarrativeNone    = "none"
	NarrativePending = "pending"
	NarrativeReady   = "ready"
	NarrativeFailed  = "failed"
	NarrativeRefused = "refused"
)

// Incident is one problem with one container, from its first signal until it
// has been quiet long enough to call resolved.
type Incident struct {
	ID              string         `json:"id"`
	EnvID           string         `json:"envId"`
	SubjectType     string         `json:"subjectType"`
	SubjectName     string         `json:"subjectName"`
	SubjectID       string         `json:"subjectId,omitempty"`
	Source          string         `json:"source,omitempty"`
	StackName       string         `json:"stackName,omitempty"`
	Signal          string         `json:"signal"`
	Status          string         `json:"status"`
	SignalCount     int            `json:"signalCount"`
	FirstSignalAt   string         `json:"firstSignalAt"`
	LastSignalAt    string         `json:"lastSignalAt"`
	AlertEventID    string         `json:"alertEventId,omitempty"`
	Evidence        *Evidence      `json:"evidence,omitempty"`
	EvidenceAt      string         `json:"evidenceAt,omitempty"`
	Narrative       *Narrative     `json:"narrative,omitempty"`
	NarrativePrompt string         `json:"narrativePrompt,omitempty"`
	NarrativeMeta   *ModelMeta     `json:"narrativeMeta,omitempty"`
	NarrativeStatus string         `json:"narrativeStatus"`
	NarrativeError  string         `json:"narrativeError,omitempty"`
	NarratedAt      string         `json:"narratedAt,omitempty"`
	Timeline        []TimelineItem `json:"timeline,omitempty"`
	Summary         string         `json:"summary,omitempty"`
	ResolvedAt      string         `json:"resolvedAt,omitempty"`
	CreatedAt       string         `json:"createdAt"`
}

// Evidence is the bounded, redacted bundle an incident is judged on. It is
// what the incident view shows and, verbatim, what a model is shown.
type Evidence struct {
	WindowStart  string         `json:"windowStart"`
	WindowEnd    string         `json:"windowEnd"`
	Container    ContainerFacts `json:"container"`
	Events       []EventItem    `json:"events"`
	Logs         []string       `json:"logs"`
	LogsNote     string         `json:"logsNote,omitempty"`
	ImageChanges []ImageChange  `json:"imageChanges"`
	Audit        []AuditItem    `json:"audit"`
	Deploys      []DeployItem   `json:"deploys"`
	Alerts       []AlertItem    `json:"alerts"`
	HostCPU      []MetricPoint  `json:"hostCpu"`
	HostMem      []MetricPoint  `json:"hostMem"`
	ContainerCPU []MetricPoint  `json:"containerCpu"`
	ContainerMem []MetricPoint  `json:"containerMem"`
	Drift        []DriftItem    `json:"drift"`
	Subject      *SubjectFacts  `json:"subject,omitempty"`
	Sections     []EvidenceItem `json:"sections,omitempty"`
	Truncated    bool           `json:"truncated,omitempty"`
}

// SubjectFacts describes a plugin-raised subject the way ContainerFacts describes a
// container; Facts are the plugin's own short key/value pairs.
type SubjectFacts struct {
	Type   string            `json:"type"`
	ID     string            `json:"id"`
	Name   string            `json:"name"`
	Source string            `json:"source"`
	Facts  map[string]string `json:"facts,omitempty"`
}

// EvidenceItem is one plugin evidence section: Kind is log, table or text.
type EvidenceItem struct {
	Title   string     `json:"title"`
	Kind    string     `json:"kind"`
	Lines   []string   `json:"lines,omitempty"`
	Columns []string   `json:"columns,omitempty"`
	Rows    [][]string `json:"rows,omitempty"`
	Note    string     `json:"note,omitempty"`
}

// ContainerFacts is what the container looked like when the evidence was taken.
type ContainerFacts struct {
	Name      string `json:"name"`
	Image     string `json:"image,omitempty"`
	ImageID   string `json:"imageId,omitempty"`
	State     string `json:"state,omitempty"`
	ExitCode  *int   `json:"exitCode,omitempty"`
	OOMKilled bool   `json:"oomKilled,omitempty"`
	Restarts  int    `json:"restarts,omitempty"`
	Health    string `json:"health,omitempty"`
	Stack     string `json:"stack,omitempty"`
}

// EventItem is one Docker event about the container.
type EventItem struct {
	At       string `json:"at"`
	Action   string `json:"action"`
	ExitCode string `json:"exitCode,omitempty"`
	Image    string `json:"image,omitempty"`
}

// ImageChange is the container being recreated on another image.
type ImageChange struct {
	At   string `json:"at"`
	From string `json:"from"`
	To   string `json:"to"`
}

// AuditItem is someone doing something to the container, its stack or its
// environment shortly before or during the incident.
type AuditItem struct {
	At     string `json:"at"`
	Who    string `json:"who"`
	Action string `json:"action"`
	Entity string `json:"entity"`
}

// DeployItem is a GitOps promotion to the environment.
type DeployItem struct {
	At       string `json:"at"`
	Pipeline string `json:"pipeline"`
	Stage    string `json:"stage"`
	Commit   string `json:"commit"`
	Status   string `json:"status"`
}

// AlertItem is an alert that fired or resolved about the container.
type AlertItem struct {
	At      string `json:"at"`
	Alert   string `json:"alert"`
	State   string `json:"state"`
	Message string `json:"message"`
}

// MetricPoint is one downsampled sample.
type MetricPoint struct {
	At    string  `json:"at"`
	Value float64 `json:"value"`
}

// DriftItem is a confirmed departure from a desired state that covers the container.
type DriftItem struct {
	Watch    string `json:"watch"`
	Field    string `json:"field"`
	Expected string `json:"expected"`
	Actual   string `json:"actual"`
}

// Narrative is what the model concluded, in the shape the prompt demands.
type Narrative struct {
	Summary      string   `json:"summary"`
	StartedAt    string   `json:"startedAt,omitempty"`
	Changes      []string `json:"changes"`
	Causes       []Cause  `json:"causes"`
	Confidence   string   `json:"confidence"`
	Undetermined bool     `json:"undetermined"`
}

// Cause is one probable cause and the evidence the model says supports it.
type Cause struct {
	Title      string   `json:"title"`
	Evidence   []string `json:"evidence"`
	Confidence string   `json:"confidence"`
}

// ModelMeta attributes a model's output: who asked, which model, when, and a
// hash of exactly what it was shown.
type ModelMeta struct {
	Provider     string `json:"provider"`
	Connection   string `json:"connection"`
	Model        string `json:"model"`
	RequestedBy  string `json:"requestedBy"`
	At           string `json:"at"`
	PromptSHA256 string `json:"promptSha256"`
	Proactive    bool   `json:"proactive"`
}

// TimelineItem is one line of a post-incident timeline.
type TimelineItem struct {
	At     string `json:"at"`
	Kind   string `json:"kind"`
	Who    string `json:"who,omitempty"`
	Detail string `json:"detail"`
}

// Settings is one environment's narration consent and limits. Nothing is sent
// to a model for an environment until someone opts it in.
type Settings struct {
	EnvID        string `json:"envId"`
	NarrateOptIn bool   `json:"narrateOptIn"`
	Proactive    bool   `json:"proactive"`
	HourlyCap    int    `json:"hourlyCap"`
	OptedInBy    string `json:"optedInBy,omitempty"`
	OptedInAt    string `json:"optedInAt,omitempty"`
}

// Finding is a behavioural pattern a deterministic detector found. The
// numbers are the detector's; a model may only explain them.
type Finding struct {
	ID              string             `json:"id"`
	EnvID           string             `json:"envId"`
	SubjectType     string             `json:"subjectType"`
	SubjectName     string             `json:"subjectName"`
	Kind            string             `json:"kind"`
	Status          string             `json:"status"`
	Metrics         map[string]float64 `json:"metrics"`
	FirstSeenAt     string             `json:"firstSeenAt"`
	LastSeenAt      string             `json:"lastSeenAt"`
	Explanation     string             `json:"explanation,omitempty"`
	ExplanationMeta *ModelMeta         `json:"explanationMeta,omitempty"`
}

// Finding kinds.
const (
	FindingMemoryClimb         = "memory_climb"
	FindingHostMemoryFull      = "host_memory_forecast"
	FindingScheduledRestart    = "scheduled_restarts"
	FindingPostDeployErrors    = "post_deploy_failures"
	FindingHostDiskFull        = "host_disk_forecast"
	FindingStorageLocationFull = "storage_location_forecast"
)
