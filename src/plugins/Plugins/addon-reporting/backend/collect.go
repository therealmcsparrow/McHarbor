// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

import (
	"context"
	"sync"
	"time"
)

// Source keys. These name both the collector and the row it produces in
// the Methodology annex, so a reader can see exactly what was consulted
// and what was not available.
const (
	srcEnvironments = "environments"
	srcDockerInfo   = "docker_info"
	srcContainers   = "containers"
	srcImages       = "images"
	srcVolumes      = "volumes"
	srcNetworks     = "networks"
	srcStacks       = "stacks"
	srcScans        = "scans"
	srcAudit        = "audit"
	// srcRunbookRuns is the record of an operator following a runbook: who
	// responded, how it ended, and which step it stopped at. It is lifecycle
	// history, and the step a runbook is repeatedly abandoned at is the number
	// that tells an operator their runbook is wrong.
	srcRunbookRuns = "runbook_runs"
	srcActivity    = "activity"
	srcAlerts      = "alerts"
	srcEvents      = "events"
	srcAutoheal    = "autoheal"
	srcMetrics     = "metrics"
	srcUsers       = "users"
	srcRoles       = "roles"
	srcGroups      = "groups"
	srcAPIKeys     = "api_keys"
	srcIdentity    = "identity"
	srcBackups     = "backups"
	srcBackupLogs  = "backup_logs"
	srcStorage     = "storage_locations"
	srcSchedules   = "schedules"
	srcRegistries  = "registries"
	srcUpdates     = "updates"
	srcGitops      = "gitops"
	srcSystem      = "system"
	srcVersions    = "versions"
	srcAgents      = "agents"
	srcPlugins     = "plugins"
	srcSettings    = "settings"
	// srcUptime is the measured health and uptime history: per-container
	// availability, outages and healthcheck probe outcomes over the report's
	// own period. Every other source describes the present; this one is the
	// only source that describes what actually happened, which is why the
	// availability, incident and readiness reports all lead with it.
	srcUptime = "uptime_history"

	// srcContainerMetrics is the measured per-container resource series over
	// the report's own period. Like srcUptime it describes what happened
	// rather than what is true this second, which is what makes honest
	// per-workload attribution possible at all.
	srcContainerMetrics = "container_metrics"

	// srcCapacity is the capacity forecast: per-subject used-of-total off the
	// retained hourly series, with a date for the subjects that have enough
	// history and a named refusal for the ones that do not. The refusals are part
	// of the data on purpose — a capacity page that is blank because eleven of
	// twelve subjects were refused is a broken report, and one that lists them as
	// "insufficient history" is a useful one.
	srcCapacity = "capacity_forecast"

	// srcMetricsHistory is a sub-key of srcMetrics rather than a source in
	// its own right: the metrics source issues two calls, one for the host's
	// identity (engine version, OS, kernel, hostname — not a time series and
	// must never become one) and one for the period series. They need
	// separate availability because Snapshot.available returns the first
	// status matching a key, so a shared key would make "did the history
	// load" unanswerable. srcContainers does the same with "dashboard".
	srcMetricsHistory = "metrics_history"
)

// Snapshot is everything a builder may read.
//
// Every field is optional. A collector that could not run leaves its
// slice nil and records why in Sources; builders check for emptiness and
// say "not available" rather than reporting a zero as a fact. A report
// that silently shows "0 critical vulnerabilities" because the scanner
// module is disabled is worse than no report.
type Snapshot struct {
	Period Period

	// Subject is the object the report was narrowed to, empty when it
	// covers the whole estate. Set by applySubject after collection, so
	// every builder sees the same already-narrowed data and none of them
	// has to know the filter exists.
	Subject Subject

	Environments []envRow
	Docker       dockerInfo

	// Host is the instantaneous reading. It supplies host identity to
	// engineTable — engine version, OS, kernel, hostname — which are
	// configuration rather than measurements and must not be averaged. It
	// also serves as the fallback when no history exists yet.
	Host hostMetrics

	// HostHistory is the host series over the report's period, and is what
	// the capacity figures are derived from when it is present.
	HostHistory metricSeries

	// ContainerHistory is one series per container with data in the period.
	ContainerHistory []metricSeries

	Dashboard  dashboardStats
	Containers []containerRow
	Images     []imageRow
	Volumes    []volumeRow
	Networks   []networkRow
	Stacks     []stackRow

	Scans       []scanRow
	ScanSummary scanSummary

	Audit       []auditRow
	Activity    []activityRow
	Alerts      []alertRow
	RunbookRuns []runbookRunRow

	// AlertEvents is the alert firing history inside the period, and
	// AutohealRestarts every restart the auto-heal engine made in it.
	AlertEvents      []alertEventRow
	AutohealRestarts []autohealRow

	Users    []userRow
	Roles    []roleRow
	Groups   []groupRow
	APIKeys  []apiKeyRow
	Identity []identityRow

	BackupPlans []backupPlanRow
	BackupRuns  []backupRunRow
	BackupLogs  []backupLogRow
	Storage     []storageRow
	Schedules   []scheduleRow

	Registries []registryRow
	Updates    []updatePolicyRow
	UpdateHist []updateHistoryRow
	SelfUpdate selfUpdateState

	Pipelines  []pipelineRow
	Promotions []promotionRow

	Versions versionInfo
	Agents   []agentRow
	Plugins  []pluginRow

	Uptime uptimeSummary

	// Capacity is the forecast payload, refusals included.
	Capacity capacityReport

	// Facets is the cross-platform answer per facet: Docker's own
	// projection plus every platform plugin that contributed. See facets.go.
	Facets map[string]*facetData

	// Resilience, Governance and Runbooks are the family-owned slices; see
	// families.go.
	Resilience resilienceData
	Runbooks   runbooksData
	Governance governanceData

	Sources []SourceStatus
}

// Period is the window a report covers.
type Period struct {
	Start, End  time.Time
	Days        float64
	PointInTime bool
}

// --- wire shapes -----------------------------------------------------
//
// These are deliberately narrow. Several host endpoints return more than
// a report may show — /environments carries socket paths, TLS material,
// kubeconfig and agent tokens — and decoding into a narrow struct means
// those values are dropped at the JSON boundary rather than being
// carried around in plugin memory waiting to be leaked into a document.

type envRow struct {
	ID            string  `json:"id"`
	Name          string  `json:"name"`
	Orchestrator  string  `json:"orchestratorType"`
	Connection    string  `json:"connectionType"`
	IsDefault     bool    `json:"isDefault"`
	IsActive      bool    `json:"isActive"`
	DockerVersion *string `json:"dockerVersion"`
	K8sVersion    *string `json:"k8sVersion"`
	LastConnected *string `json:"lastConnected"`
	AgentStatus   *string `json:"agentStatus"`
	AgentVersion  *string `json:"agentVersion"`
	AgentOS       *string `json:"agentOs"`
	AgentArch     *string `json:"agentArch"`
	AgentLastSeen *string `json:"agentLastSeen"`
	Timezone      string  `json:"timezone"`
	AutoUpdate    bool    `json:"autoUpdateEnabled"`
	CreatedAt     string  `json:"createdAt"`
}

type dockerInfo struct {
	ServerVersion  string `json:"serverVersion"`
	APIVersion     string `json:"apiVersion"`
	OS             string `json:"os"`
	Architecture   string `json:"architecture"`
	KernelVersion  string `json:"kernelVersion"`
	Hostname       string `json:"hostname"`
	NCPU           int    `json:"ncpu"`
	MemTotal       int64  `json:"memTotal"`
	StorageDriver  string `json:"storageDriver"`
	CgroupDriver   string `json:"cgroupDriver"`
	CgroupVersion  string `json:"cgroupVersion"`
	DefaultRuntime string `json:"defaultRuntime"`
}

type hostMetrics struct {
	NCPU          int     `json:"ncpu"`
	MemTotal      int64   `json:"memTotal"`
	MemUsed       int64   `json:"memUsed"`
	MemPercent    float64 `json:"memPercent"`
	CPUPercent    float64 `json:"cpuPercent"`
	Load1         float64 `json:"load1"`
	Load5         float64 `json:"load5"`
	Load15        float64 `json:"load15"`
	ServerVersion string  `json:"serverVersion"`
	OS            string  `json:"os"`
	KernelVersion string  `json:"kernelVersion"`
	Hostname      string  `json:"hostname"`
	Uptime        int64   `json:"uptime"`
}

// metricPoint is one bucket of the host's metric history.
//
// CPUPercent is the bucket's mean and CPUMax its highest sample, so a
// series carries both the sustained level and the spike without the
// report having to choose between them. Samples is the number of raw
// readings folded into the bucket: a bucket built from two samples is
// not the same evidence as one built from twenty, and a mean over
// buckets that ignored the difference would quietly overweight the
// quiet ones.
type metricPoint struct {
	Timestamp  string `json:"timestamp"`
	Resolution string `json:"resolution"` // "raw" (30s) | "rollup" (bucketed)
	Samples    int    `json:"samples"`

	CPUPercent float64 `json:"cpuPercent"`
	CPUMax     float64 `json:"cpuMax"`
	MemPercent float64 `json:"memPercent"`
	MemUsed    int64   `json:"memUsed"`
	MemUsedMax int64   `json:"memUsedMax"`
	MemLimit   int64   `json:"memLimit"`
}

// metricSeries is a metric history over a window.
//
// From/To is what the report asked for; CoveredFrom/CoveredTo is what
// the store actually held. The two differ for as long as the rollup is
// younger than the period being reported on, and the gap between them
// is the whole reason a report can state its own shortfall rather than
// averaging a shorter period and printing the longer one on the cover.
type metricSeries struct {
	EnvironmentID string `json:"environmentId"`
	ContainerID   string `json:"containerId,omitempty"`
	Name          string `json:"name,omitempty"`

	From        string `json:"from"`
	To          string `json:"to"`
	CoveredFrom string `json:"coveredFrom,omitempty"`
	CoveredTo   string `json:"coveredTo,omitempty"`

	BucketSeconds int `json:"bucketSeconds"`

	Points []metricPoint `json:"points"`
}

type dashboardStats struct {
	Containers struct {
		Total      int `json:"total"`
		Running    int `json:"running"`
		Stopped    int `json:"stopped"`
		Paused     int `json:"paused"`
		Restarting int `json:"restarting"`
		Killed     int `json:"killed"`
	} `json:"containers"`
	Images   int `json:"images"`
	Volumes  int `json:"volumes"`
	Networks int `json:"networks"`
	Stacks   int `json:"stacks"`
}

type containerRow struct {
	ID           string            `json:"Id"`
	Names        []string          `json:"Names"`
	Image        string            `json:"Image"`
	ImageID      string            `json:"ImageID"`
	Created      int64             `json:"Created"`
	State        string            `json:"State"`
	Status       string            `json:"Status"`
	Labels       map[string]string `json:"Labels"`
	StackName    string            `json:"StackName"`
	StackService string            `json:"StackService"`
	Protected    bool              `json:"Protected"`

	// Ports is read for the exposure facet: which container port is
	// published on which host port and interface.
	Ports []containerPort `json:"Ports"`

	// Mounts and NetworkSettings are decoded only so a report narrowed to
	// one workload can name the volumes and networks that workload
	// actually uses. Neither is rendered directly: a bind mount's Source
	// is a host path, which is exactly the kind of value a circulated
	// document has no business carrying, so only `Name` is ever read and
	// only for volume mounts.
	Mounts []struct {
		Type string `json:"Type"`
		Name string `json:"Name"`
	} `json:"Mounts"`
	NetworkSettings struct {
		Networks map[string]struct {
			NetworkID string `json:"NetworkID"`
		} `json:"Networks"`
	} `json:"NetworkSettings"`
}

// containerPort is one port binding; PublicPort 0 means not published.
type containerPort struct {
	IP          string `json:"IP"`
	PrivatePort uint16 `json:"PrivatePort"`
	PublicPort  uint16 `json:"PublicPort"`
	Type        string `json:"Type"`
}

type imageRow struct {
	ID          string   `json:"Id"`
	RepoTags    []string `json:"RepoTags"`
	RepoDigests []string `json:"RepoDigests"`
	Created     int64    `json:"Created"`
	Size        int64    `json:"Size"`
	Containers  int64    `json:"Containers"`
	Protected   bool     `json:"Protected"`
}

type volumeRow struct {
	Name      string `json:"Name"`
	Driver    string `json:"Driver"`
	CreatedAt string `json:"CreatedAt"`
	Scope     string `json:"Scope"`
	RefCount  int    `json:"RefCount"`
	Protected bool   `json:"Protected"`
	UsageData *struct {
		Size     int64 `json:"Size"`
		RefCount int64 `json:"RefCount"`
	} `json:"UsageData"`
}

type networkRow struct {
	ID         string `json:"Id"`
	Name       string `json:"Name"`
	Driver     string `json:"Driver"`
	Scope      string `json:"Scope"`
	Internal   bool   `json:"Internal"`
	Containers int    `json:"Containers"`
	BuiltIn    bool   `json:"BuiltIn"`
}

type stackRow struct {
	Name       string `json:"name"`
	Status     string `json:"status"`
	Services   int    `json:"services"`
	Containers int    `json:"containers"`
	Source     string `json:"source"`
	UpdatedAt  string `json:"updatedAt"`
}

type scanRow struct {
	ID          string `json:"id"`
	ImageRef    string `json:"imageRef"`
	Scanner     string `json:"scanner"`
	Status      string `json:"status"`
	Severity    string `json:"severity"`
	TotalVulns  int    `json:"totalVulns"`
	Critical    int    `json:"criticalCount"`
	High        int    `json:"highCount"`
	Medium      int    `json:"mediumCount"`
	Low         int    `json:"lowCount"`
	StartedAt   string `json:"startedAt"`
	CompletedAt string `json:"completedAt"`
	CreatedAt   string `json:"createdAt"`
}

type scanSummary struct {
	Critical int `json:"critical"`
	High     int `json:"high"`
	Medium   int `json:"medium"`
	Low      int `json:"low"`
}

type auditRow struct {
	ID         string  `json:"id"`
	Username   *string `json:"username"`
	Action     string  `json:"action"`
	Severity   string  `json:"severity"`
	EntityType *string `json:"entityType"`
	EntityName *string `json:"entityName"`
	Timestamp  string  `json:"timestamp"`
}

// runbookRunRow is one operator response, as GET /api/runbooks/runs returns it.
type runbookRunRow struct {
	ID          string             `json:"id"`
	RunbookName string             `json:"runbookName"`
	SubjectName string             `json:"subjectName"`
	IncidentID  string             `json:"incidentId"`
	StartedBy   string             `json:"startedByName"`
	StartedAt   string             `json:"startedAt"`
	FinishedAt  string             `json:"finishedAt"`
	Outcome     string             `json:"outcome"`
	Results     []runbookResultRow `json:"results"`
}

// runbookResultRow is one step of a run, which is where "where it stopped"
// comes from.
type runbookResultRow struct {
	StepPosition int    `json:"stepPosition"`
	StepTitle    string `json:"stepTitle"`
	State        string `json:"state"`
}

type activityRow struct {
	ID            string `json:"id"`
	ContainerID   string `json:"containerId"`
	ContainerName string `json:"containerName"`
	EventType     string `json:"eventType"`
	Action        string `json:"action"`
	Timestamp     string `json:"timestamp"`
}

type alertRow struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Severity  string `json:"severity"`
	Type      string `json:"type"`
	Condition string `json:"condition"`
	Target    string `json:"target"`
	Enabled   bool   `json:"enabled"`
}

// alertEventRow is one alert firing. Message is not decoded: it is an
// operator-written template and can name anything, including a host.
type alertEventRow struct {
	ID              string `json:"id"`
	AlertName       string `json:"alertName"`
	SourceType      string `json:"sourceType"`
	EnvironmentName string `json:"environmentName"`
	SubjectName     string `json:"subjectName"`
	State           string `json:"state"`
	Severity        string `json:"severity"`
	ValueLabel      string `json:"valueLabel"`
	CreatedAt       string `json:"createdAt"`
}

// autohealRow is one auto-heal restart, as the audit log records it.
type autohealRow struct {
	ID         string `json:"id"`
	EntityName string `json:"entityName"`
	Timestamp  string `json:"timestamp"`
}

// inPeriod keeps the rows whose timestamp falls inside the period. A row
// with no readable timestamp is kept: dropping it would make an event
// vanish because of a formatting difference.
func inPeriod[T any](rows []T, p Period, at func(T) string) []T {
	if p.Start.IsZero() || p.End.IsZero() || p.PointInTime {
		return rows
	}
	out := rows[:0]
	for _, r := range rows {
		t := parseTS(at(r))
		if t.IsZero() || (!t.Before(p.Start) && !t.After(p.End)) {
			out = append(out, r)
		}
	}
	return out
}

type userRow struct {
	ID        string  `json:"id"`
	Username  string  `json:"username"`
	Email     *string `json:"email"`
	Role      string  `json:"role"`
	IsActive  bool    `json:"isActive"`
	LastLogin *string `json:"lastLogin"`
	CreatedAt string  `json:"createdAt"`
}

type roleRow struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Permissions []string `json:"permissions"`
	IsSystem    bool     `json:"isSystem"`
}

type groupRow struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	IsSystem    bool   `json:"isSystem"`
	MemberCount int    `json:"memberCount"`
}

type apiKeyRow struct {
	ID         string   `json:"id"`
	Username   string   `json:"username"`
	Name       string   `json:"name"`
	Scopes     []string `json:"scopes"`
	ExpiresAt  *string  `json:"expiresAt"`
	LastUsedAt *string  `json:"lastUsedAt"`
	IsActive   bool     `json:"isActive"`
	CreatedAt  string   `json:"createdAt"`
	// KeyPrefix is deliberately not decoded. It is not a secret, but a
	// report is copied and forwarded, and key fragments in a circulated
	// document invite someone to go looking for the rest.
}

type identityRow struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Type    string `json:"type"`
	Enabled bool   `json:"enabled"`
}

type backupPlanRow struct {
	ID             string `json:"id"`
	Name           string `json:"name"`
	ContainerName  string `json:"containerName"`
	Cron           string `json:"cron"`
	Enabled        bool   `json:"enabled"`
	RetentionCount int    `json:"retentionCount"`
	RetentionDays  int    `json:"retentionDays"`
	IncludeConfig  bool   `json:"includeConfig"`
	IncludeVolumes bool   `json:"includeFilesystem"`
}

type backupRunRow struct {
	ID            string `json:"id"`
	PlanID        string `json:"planId"`
	ContainerName string `json:"containerName"`
	Status        string `json:"status"`
	ArchiveSize   int64  `json:"archiveSize"`
	Error         string `json:"error"`
	StartedAt     string `json:"startedAt"`
	CompletedAt   string `json:"completedAt"`
	CreatedAt     string `json:"createdAt"`
}

type backupLogRow struct {
	ID            string  `json:"id"`
	PlanName      *string `json:"planName"`
	ContainerName *string `json:"containerName"`
	Action        string  `json:"action"`
	Phase         string  `json:"phase"`
	Severity      string  `json:"severity"`
	Message       string  `json:"message"`
	CreatedAt     string  `json:"createdAt"`
}

type storageRow struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Type    string `json:"type"`
	Enabled bool   `json:"enabled"`
}

type scheduleRow struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Cron      string `json:"cron"`
	Action    string `json:"action"`
	Target    string `json:"target"`
	Enabled   bool   `json:"enabled"`
	LastRunAt string `json:"lastRunAt"`
	NextRunAt string `json:"nextRunAt"`
}

type registryRow struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	URL       string `json:"url"`
	IsDefault bool   `json:"isDefault"`
	// Credentials are never decoded.
}

type updatePolicyRow struct {
	ID            string `json:"id"`
	Name          string `json:"name"`
	ContainerName string `json:"containerMatch"`
	ImageMatch    string `json:"imageMatch"`
	Schedule      string `json:"schedule"`
	Strategy      string `json:"strategy"`
	Enabled       bool   `json:"enabled"`
	LastRunAt     string `json:"lastRunAt"`
	LastRunStatus string `json:"lastRunStatus"`
}

type updateHistoryRow struct {
	ID         string `json:"id"`
	Container  string `json:"container"`
	OldImage   string `json:"oldImage"`
	NewImage   string `json:"newImage"`
	Status     string `json:"status"`
	Message    string `json:"message"`
	ExecutedAt string `json:"executedAt"`
}

type selfUpdateState struct {
	CurrentVersion  string `json:"currentVersion"`
	LatestVersion   string `json:"latestVersion"`
	UpdateAvailable bool   `json:"updateAvailable"`
	LastCheckedAt   string `json:"lastCheckedAt"`
}

type pipelineRow struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Repo    string `json:"repo"`
	Enabled bool   `json:"enabled"`
}

type promotionRow struct {
	ID          string `json:"id"`
	PipelineID  string `json:"pipelineId"`
	FromEnv     string `json:"fromEnv"`
	ToEnv       string `json:"toEnv"`
	Status      string `json:"status"`
	RequestedBy string `json:"requestedBy"`
	CreatedAt   string `json:"createdAt"`
	CompletedAt string `json:"completedAt"`
}

type versionInfo struct {
	McHarbor struct {
		Version string `json:"version"`
		Commit  string `json:"commit"`
		Built   string `json:"built"`
	} `json:"mcharbor"`
	Agents []struct {
		EnvironmentID string `json:"environmentId"`
		Name          string `json:"name"`
		Version       string `json:"version"`
	} `json:"agents"`
}

type agentRow struct {
	EnvironmentID string `json:"environmentId"`
	Name          string `json:"name"`
	Status        string `json:"status"`
	Version       string `json:"version"`
	Hostname      string `json:"hostname"`
	OS            string `json:"os"`
	Arch          string `json:"arch"`
	LastSeen      string `json:"lastSeen"`
}

type pluginRow struct {
	Slug    string `json:"slug"`
	Name    string `json:"name"`
	Version string `json:"version"`
	Enabled bool   `json:"enabled"`
	Loaded  bool   `json:"loaded"`
}

// uptimeSummary is the host's fleet availability payload.
//
// ObservedDays is the field that keeps this honest. The raw sample series is
// pruned after a week and only the daily rollup reaches further back, so a
// report asked for ninety days may only have thirty days of history to work
// with. Every figure derived from this struct is qualified by ObservedDays,
// never by the period on the cover.
type uptimeSummary struct {
	WindowStart string `json:"windowStart"`
	WindowEnd   string `json:"windowEnd"`

	RequestedDays int `json:"requestedDays"`
	ObservedDays  int `json:"observedDays"`

	SampleIntervalSeconds int `json:"sampleIntervalSeconds"`
	RawRetentionDays      int `json:"rawRetentionDays"`
	RollupRetentionDays   int `json:"rollupRetentionDays"`

	Containers []uptimeRow    `json:"containers"`
	Totals     uptimeTotalRow `json:"totals"`
}

type uptimeRow struct {
	ContainerID   string `json:"containerId"`
	ContainerName string `json:"containerName"`

	FirstDay string `json:"firstDay"`
	LastDay  string `json:"lastDay"`
	Days     int    `json:"days"`

	Samples          int `json:"samples"`
	RunningSamples   int `json:"runningSamples"`
	DownSamples      int `json:"downSamples"`
	UnhealthySamples int `json:"unhealthySamples"`

	ObservedMinutes float64 `json:"observedMinutes"`
	DowntimeMinutes float64 `json:"downtimeMinutes"`
	UptimePercent   float64 `json:"uptimePercent"`
	HealthyPercent  float64 `json:"healthyPercent"`

	Outages     int `json:"outages"`
	Transitions int `json:"transitions"`

	MTBFHours   float64 `json:"mtbfHours"`
	MTTRMinutes float64 `json:"mttrMinutes"`
	FlapsPerDay float64 `json:"flapsPerDay"`

	Probes           int     `json:"probes"`
	ProbesPassed     int     `json:"probesPassed"`
	ProbesFailed     int     `json:"probesFailed"`
	ProbePassPercent float64 `json:"probePassPercent"`
	MeanProbeMs      float64 `json:"meanProbeMs"`
	MaxProbeMs       int     `json:"maxProbeMs"`
	HasHealthcheck   bool    `json:"hasHealthcheck"`
	LastFailureAt    string  `json:"lastFailureAt"`

	CurrentState  string `json:"currentState"`
	CurrentHealth string `json:"currentHealth"`
}

type uptimeTotalRow struct {
	Containers         int     `json:"containers"`
	Tracked            int     `json:"tracked"`
	WithDowntime       int     `json:"withDowntime"`
	WithHealthcheck    int     `json:"withHealthcheck"`
	Samples            int     `json:"samples"`
	ObservedMinutes    float64 `json:"observedMinutes"`
	DowntimeMinutes    float64 `json:"downtimeMinutes"`
	UptimePercent      float64 `json:"uptimePercent"`
	Outages            int     `json:"outages"`
	Transitions        int     `json:"transitions"`
	Probes             int     `json:"probes"`
	ProbesFailed       int     `json:"probesFailed"`
	ProbePassPercent   float64 `json:"probePassPercent"`
	MeanUptimePercent  float64 `json:"meanUptimePercent"`
	WorstUptimePercent float64 `json:"worstUptimePercent"`
}

// --- collection ------------------------------------------------------

// collect fetches every source the report type declares, concurrently.
//
// Concurrency matters more than it looks: a full executive report touches
// around twenty endpoints, several of which talk to the Docker daemon,
// and over the remote-agent transport a single call can take a couple of
// seconds. Sequentially that is a minute; in parallel it is a few seconds.
// onSource, when non-nil, is called once per source as it finishes, with
// the source's human label. It runs on the collecting goroutine, so an
// implementation must be safe to call concurrently.
func collect(ctx context.Context, api *hostAPI, rt ReportType, period Period, onSource func(label string)) *Snapshot {
	snap := &Snapshot{Period: period}
	jobs := collectJobs(ctx, api, snap, period)

	var wg sync.WaitGroup
	seen := map[string]bool{}
	for _, key := range rt.Sources {
		job, ok := jobs[key]
		if !ok || seen[key] {
			continue
		}
		seen[key] = true
		wg.Add(1)
		go func(name string, f func()) {
			defer wg.Done()
			f()
			if onSource != nil {
				onSource(name)
			}
		}(key, job)
	}
	wg.Wait()

	snap.Sources = api.sources()
	return snap
}

// plannedSources counts the sources collect will actually fetch for a
// type, so a progress indicator can be scaled before collection starts.
//
// It reads the same table collect reads rather than a second hand-kept
// list: several report types declare sources no collector implements
// (the host exposes no list endpoint for them), and a total that counted
// those would leave a progress bar stuck short of the end.
func plannedSources(rt ReportType) int {
	jobs := collectJobs(context.Background(), nil, &Snapshot{}, Period{})
	seen := map[string]bool{}
	n := 0
	for _, key := range rt.Sources {
		if seen[key] {
			continue
		}
		seen[key] = true
		if _, ok := jobs[key]; ok {
			n++
		}
	}
	return n
}

// historyWindowStart is the `from` for the host-metrics history call. A
// point-in-time report has period.Start == period.End, which the host refuses
// as an empty window, so it reads the trailing day instead.
func historyWindowStart(period Period, from string) string {
	if period.End.After(period.Start) {
		return from
	}
	return period.End.UTC().Add(-24 * time.Hour).Format(time.RFC3339)
}

// collectJobs is the source table: one closure per source, keyed by the
// name the Methodology annex prints.
//
// It is a function rather than a literal inside collect so the key set
// can be read without running anything — the closures capture api and
// snap but are never invoked when only the keys are wanted.
func collectJobs(ctx context.Context, api *hostAPI, snap *Snapshot, period Period) map[string]func() {
	from := period.Start.UTC().Format(time.RFC3339)
	to := period.End.UTC().Format(time.RFC3339)
	window := map[string]string{"from": from, "to": to, "per_page": "100"}
	historyFrom := historyWindowStart(period, from)

	jobs := map[string]func(){
		srcEnvironments: func() {
			api.getList(ctx, srcEnvironments, "Environments", "/api/environments", nil, &snap.Environments)
		},
		srcDockerInfo: func() {
			api.get(ctx, srcDockerInfo, "Docker engine", "/api/docker/info", nil, &snap.Docker)
		},
		srcMetrics: func() {
			// Two calls, two different kinds of fact. The point-in-time
			// reading supplies host identity — engine version, OS, kernel,
			// hostname — which is configuration and must not be averaged
			// into a period figure; it also stands in as the fallback
			// utilisation when no history exists yet.
			api.get(ctx, srcMetrics, "Host metrics", "/api/metrics/host", nil, &snap.Host)
			// The window is passed explicitly rather than as a range
			// shorthand: a report covers the period printed on its cover,
			// not the last N days from whenever the generate button
			// happened to be pressed.
			api.get(ctx, srcMetricsHistory, "Host metrics history", "/api/metrics/host/history",
				map[string]string{"from": historyFrom, "to": to}, &snap.HostHistory)
		},
		srcContainerMetrics: func() {
			api.get(ctx, srcContainerMetrics, "Container metrics history",
				"/api/metrics/containers/history",
				map[string]string{"from": from, "to": to, "limit": "500"}, &snap.ContainerHistory)
		},
		srcContainers: func() {
			api.getList(ctx, srcContainers, "Containers", "/api/containers", map[string]string{"all": "true"}, &snap.Containers)
			api.get(ctx, "dashboard", "Dashboard totals", "/api/dashboard/stats", nil, &snap.Dashboard)
		},
		srcImages: func() {
			api.getList(ctx, srcImages, "Images", "/api/images", nil, &snap.Images)
		},
		srcUptime: func() {
			// The window is passed explicitly rather than as `days`: a report
			// covers the period printed on its cover, not the last N days
			// from whenever the generate button happened to be pressed.
			api.get(ctx, srcUptime, "Health and uptime history", "/api/containers/health/summary",
				map[string]string{"from": from, "to": to, "limit": "500"}, &snap.Uptime)
		},
		srcCapacity: func() {
			// No period is passed: the window is the retained series' own
			// horizon, which the payload states, and asking for a shorter one
			// would make the forecast worse than the one the product shows.
			api.get(ctx, srcCapacity, "Capacity forecast", "/api/metrics/capacity", nil, &snap.Capacity)
		},
		srcVolumes: func() {
			api.getList(ctx, srcVolumes, "Volumes", "/api/volumes", nil, &snap.Volumes)
		},
		srcNetworks: func() {
			api.getList(ctx, srcNetworks, "Networks", "/api/networks", nil, &snap.Networks)
		},
		srcStacks: func() {
			api.getList(ctx, srcStacks, "Stacks", "/api/stacks", nil, &snap.Stacks)
		},
		srcScans: func() {
			api.getList(ctx, srcScans, "Vulnerability scans", "/api/scans", nil, &snap.Scans)
			api.get(ctx, "scan_summary", "Vulnerability summary", "/api/scans/summary", nil, &snap.ScanSummary)
		},
		srcAudit: func() {
			api.getList(ctx, srcAudit, "Audit log", "/api/audit", window, &snap.Audit)
		},
		srcRunbookRuns: func() {
			api.getList(ctx, srcRunbookRuns, "Runbook runs", "/api/runbooks/runs", window, &snap.RunbookRuns)
		},
		srcActivity: func() {
			api.getList(ctx, srcActivity, "Container activity", "/api/activity", window, &snap.Activity)
		},
		srcAlerts: func() {
			api.getList(ctx, srcAlerts, "Alert rules", "/api/alerts", nil, &snap.Alerts)
		},
		srcEvents: func() {
			// The endpoint takes no window, so the newest page is read and
			// trimmed to the period here; an event outside it is not part of
			// what this report covers.
			var rows []alertEventRow
			if api.getList(ctx, srcEvents, "Alert events", "/api/alerts/events",
				map[string]string{"per_page": "500"}, &rows) {
				snap.AlertEvents = inPeriod(rows, period, func(r alertEventRow) string { return r.CreatedAt })
			}
		},
		srcAutoheal: func() {
			// Every auto-heal restart is written to the audit log under its
			// own action, which is the only durable record of one.
			p := map[string]string{"action": "autoheal", "from": from, "to": to, "per_page": "500"}
			api.getList(ctx, srcAutoheal, "Auto-heal restarts", "/api/audit", p, &snap.AutohealRestarts)
		},
		srcUsers: func() {
			api.getList(ctx, srcUsers, "Users", "/api/users", nil, &snap.Users)
		},
		srcRoles: func() {
			api.getList(ctx, srcRoles, "Roles", "/api/roles", nil, &snap.Roles)
		},
		srcGroups: func() {
			api.getList(ctx, srcGroups, "Groups", "/api/groups", nil, &snap.Groups)
		},
		srcAPIKeys: func() {
			api.getList(ctx, srcAPIKeys, "API keys", "/api/api-keys", nil, &snap.APIKeys)
		},
		srcIdentity: func() {
			api.getList(ctx, srcIdentity, "Identity providers", "/api/identity-providers", nil, &snap.Identity)
		},
		srcBackups: func() {
			api.getList(ctx, srcBackups, "Backup plans", "/api/container-backups", nil, &snap.BackupPlans)
			api.getList(ctx, "backup_runs", "Backup runs", "/api/container-backups/runs", map[string]string{"per_page": "100"}, &snap.BackupRuns)
		},
		srcBackupLogs: func() {
			api.getList(ctx, srcBackupLogs, "Backup log", "/api/backup-logs", map[string]string{"per_page": "100"}, &snap.BackupLogs)
		},
		srcStorage: func() {
			api.getList(ctx, srcStorage, "Storage locations", "/api/storage-locations", nil, &snap.Storage)
		},
		srcSchedules: func() {
			api.getList(ctx, srcSchedules, "Schedules", "/api/schedules", nil, &snap.Schedules)
		},
		srcRegistries: func() {
			api.getList(ctx, srcRegistries, "Registries", "/api/registries", nil, &snap.Registries)
		},
		srcUpdates: func() {
			api.getList(ctx, srcUpdates, "Update policies", "/api/updates", nil, &snap.Updates)
			api.get(ctx, "self_update", "Platform update state", "/api/updates/state", nil, &snap.SelfUpdate)
		},
		srcGitops: func() {
			api.getList(ctx, srcGitops, "GitOps pipelines", "/api/gitops/pipelines", nil, &snap.Pipelines)
			api.getList(ctx, "promotions", "Promotions", "/api/gitops/promotions", nil, &snap.Promotions)
		},
		srcVersions: func() {
			api.get(ctx, srcVersions, "Component versions", "/api/versions", nil, &snap.Versions)
		},
		srcAgents: func() {
			api.getList(ctx, srcAgents, "Remote agents", "/api/agents", nil, &snap.Agents)
		},
		srcPlugins: func() {
			api.getList(ctx, srcPlugins, "Installed plugins", "/api/appstore/plugins/installed", nil, &snap.Plugins)
		},
		srcSystem: func() {
			api.get(ctx, srcSystem, "Platform version", "/api/versions", nil, &snap.Versions)
		},
		srcSettings: func() {
			// Settings are read only to report retention and policy
			// posture; the response is not stored verbatim because it can
			// carry configuration a report has no business reproducing.
			api.get(ctx, srcSettings, "Settings", "/api/settings/retention", nil, nil)
		},
	}
	for _, fam := range reportFamilies() {
		for key, job := range fam.jobs(ctx, api, snap, period) {
			jobs[key] = job
		}
	}
	return jobs
}

// available reports whether a source answered, so builders can say
// "not assessed" instead of inventing a zero.
func (s *Snapshot) available(key string) bool {
	for _, st := range s.Sources {
		if st.Key == key {
			return st.OK
		}
	}
	return false
}

// note returns why a source is missing, for the Methodology annex.
func (s *Snapshot) note(key string) string {
	for _, st := range s.Sources {
		if st.Key == key {
			return st.Note
		}
	}
	return "not requested"
}

// capacityReport mirrors metrics.CapacityReport, narrowed to what a document
// prints. It carries no cost, rate or currency field, and must not gain one.
type capacityReport struct {
	Subjects     []capacitySubject `json:"subjects"`
	WindowDays   int               `json:"windowDays"`
	RequiredDays float64           `json:"requiredDays"`
	AmberPercent float64           `json:"amberPercent"`
	RedPercent   float64           `json:"redPercent"`
}

// capacitySubject is one forecast. Refusal is populated exactly when Red is nil.
type capacitySubject struct {
	SubjectKind   string  `json:"subjectKind"`
	SubjectID     string  `json:"subjectId"`
	SubjectName   string  `json:"subjectName"`
	UsedBytes     int64   `json:"usedBytes"`
	TotalBytes    int64   `json:"totalBytes"`
	UsedPercent   float64 `json:"usedPercent"`
	ObservedDays  float64 `json:"observedDays"`
	RequiredDays  float64 `json:"requiredDays"`
	PercentPerDay float64 `json:"percentPerDay"`
	R2            float64 `json:"r2"`
	Refusal       string  `json:"refusal"`
	Red           *struct {
		Percent float64 `json:"percent"`
		Days    float64 `json:"days"`
		At      string  `json:"at"`
	} `json:"red"`
}
