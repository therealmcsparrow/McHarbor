// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

import (
	"math"
	"sort"
	"strings"
	"time"
)

// Facts is everything derived from a Snapshot: counts, rates, ratios and
// rankings. Builders read Facts, never raw wire rows, so a change in a
// host response shape lands in one place.
type Facts struct {
	Containers   ContainerFacts
	Images       ImageFacts
	Security     SecurityFacts
	Capacity     CapacityFacts
	Reliability  ReliabilityFacts
	Delivery     DeliveryFacts
	Resilience   ResilienceFacts
	Access       AccessFacts
	Availability AvailabilityFacts
}

// AvailabilityFacts is measured availability, from the health and uptime
// history rather than from the current state.
//
// The distinction matters more than it reads. ContainerFacts.Healthy answers
// "how many workloads are healthy right now", which a report generated at a
// quiet moment will flatter. This answers "how much of the period was each
// workload actually up", which is the question an availability target is
// written against, and it is the only one of the two that can be missed.
type AvailabilityFacts struct {
	Assessed bool

	// ObservedDays is how many days of history actually exist. Reports must
	// quote this, not the period length: the raw series is pruned weekly and
	// the rollup only reaches back as far as the platform has been running.
	ObservedDays  int
	RequestedDays int
	Partial       bool

	Tracked         int
	WithDowntime    int
	WithHealthcheck int
	NoHealthcheck   int

	UptimePercent      float64 // time-weighted across the fleet
	MeanUptimePercent  float64 // unweighted mean per workload
	WorstUptimePercent float64
	ObservedMinutes    float64
	DowntimeMinutes    float64

	Outages          int
	Transitions      int
	OutageRatePerDay float64

	MTTRMinutes float64
	MTBFHours   float64

	Probes           int
	ProbesFailed     int
	ProbePassPercent float64
	MeanProbeMs      float64
	SlowestProbeMs   int

	// ErrorBudget is what remains of a 99.9% monthly budget, as a percentage
	// of the budget rather than of the period. Negative means it is spent.
	ErrorBudgetPercent float64
	TargetPercent      float64

	// Rankings, worst first.
	Worst     []Sized // by uptime percent
	Flappiest []Sized // by transitions per day
	Rows      []uptimeRow
}

// ContainerFacts is the workload picture.
type ContainerFacts struct {
	Total, Running, Stopped, Paused, Restarting, Exited int
	Unhealthy, Healthy, NoHealthcheck                   int
	Protected                                           int
	Stacks                                              int
	Unmanaged                                           int // not part of a stack
	ByStack                                             []Counted
	OldestRunningDays                                   float64
	RunningRatio                                        float64
	UnhealthyRatio                                      float64
}

// ImageFacts is the image estate.
type ImageFacts struct {
	Total, InUse, Unused, Dangling int
	TotalBytes                     int64
	ReclaimableBytes               int64
	MedianAgeDays                  float64
	StaleOver90Days                int
	Largest                        []Sized
}

// SecurityFacts is the assurance picture.
type SecurityFacts struct {
	Assessed                    bool
	ScansTotal                  int
	ImagesScanned               int
	CoveragePercent             float64
	Critical, High, Medium, Low int
	WeightedExposure            float64
	LastScanAge                 float64 // days; -1 when unknown
	TopVulnerableImages         []Sized
	AuditEvents                 int
	AuditCritical, AuditWarn    int
	FailedLogins                int
	PrivilegedActions           int
}

// periodStat is one resource's behaviour over the report's period.
//
// Avg is what the estate normally looks like, Peak is the worst single
// bucket, and P95 is what a bad hour looks like once the one-off
// excursions are set aside. The three are kept separately because
// collapsing them loses the argument: grading on Avg forgives a host
// that is saturated a fifth of the time, and grading on Peak condemns a
// host for one spike in a month. P95 survives both objections, which is
// why it is the figure thresholds are applied to.
type periodStat struct {
	Avg, Peak, P95 float64

	// Samples is raw readings folded into the series, Buckets the number
	// of points they were folded into. Both are reported because "34 %
	// over three buckets" and "34 % over four thousand" are not the same
	// claim, and only one of them belongs in a monthly report.
	Samples int
	Buckets int
}

// grade is the figure thresholds are applied to: the p95 when a series
// was actually measured, and the caller's instantaneous reading when it
// was not. A report with no history still grades — it just grades the
// only evidence it has, and says so elsewhere.
func (p periodStat) grade(fallback float64) float64 {
	if p.Samples == 0 {
		return fallback
	}
	return p.P95
}

// CapacityFacts is the resource picture.
type CapacityFacts struct {
	Assessed bool

	// CPUPercent and MemPercent are the period averages when metric
	// history covers the report, and the instantaneous reading when it
	// does not. They are the headline figures every builder reads; the
	// spread behind them is in CPUStat and MemStat.
	CPUPercent float64
	MemPercent float64

	CPUStat periodStat
	MemStat periodStat

	MemTotalBytes   int64
	MemUsedBytes    int64
	CPUCount        int
	Load1PerCore    float64
	VolumeBytes     int64
	VolumeCount     int
	UnusedVolumes   int
	HeadroomPercent float64
	UptimeDays      float64

	// HistoryAvailable says the figures above were measured over a period
	// rather than sampled once. Covered* is what the metric store actually
	// held, which is not always what the report asked for: the rollup only
	// reaches as far back as it has been running, and a report that
	// averaged six days while printing thirty on its cover would be worse
	// than one that admits the gap.
	HistoryAvailable bool
	CoveredFrom      time.Time
	CoveredTo        time.Time
	CoveredDays      float64
	RequestedDays    float64
}

// CoverageShort reports whether the metric history fell materially short
// of the requested period. The tolerance is a fifth of a day: clock skew
// and a collector that started mid-bucket should not trip a callout,
// while a genuinely truncated month should.
func (f CapacityFacts) CoverageShort() bool {
	return f.HistoryAvailable && f.RequestedDays > 0 &&
		f.CoveredDays < f.RequestedDays-0.2
}

// ReliabilityFacts covers availability and stability.
type ReliabilityFacts struct {
	Assessed            bool
	Restarts            int
	Crashes             int // non-zero exits
	OOMKills            int
	CrashLooping        []Counted
	EventsTotal         int
	RestartRatePerDay   float64
	MeanRecoveryMinutes float64
	RecoverySamples     int
	IncidentDays        int
	TopOffenders        []Counted
	EventsByDay         []Counted
}

// DeliveryFacts is the change picture — DORA's four keys, computed from
// what a container platform can actually observe.
type DeliveryFacts struct {
	Assessed            bool
	Deployments         int
	DeploymentsPerWeek  float64
	Rollbacks           int
	FailedChanges       int
	ChangeFailureRate   float64
	UpdatesApplied      int
	UpdatesFailed       int
	PromotionsTotal     int
	PromotionsSucceeded int
	MeanPromotionHours  float64
	DeploymentsByDay    []Counted
}

// ResilienceFacts is backup and recovery readiness.
type ResilienceFacts struct {
	Assessed          bool
	Plans             int
	PlansEnabled      int
	ContainersCovered int
	CoveragePercent   float64
	Runs              int
	RunsSucceeded     int
	RunsFailed        int
	SuccessRate       float64
	LastSuccessAge    float64 // days; -1 when never
	TotalArchiveBytes int64
	Destinations      int
}

// AccessFacts is the identity picture.
type AccessFacts struct {
	Assessed        bool
	Users           int
	ActiveUsers     int
	InactiveUsers   int
	DormantUsers    int // no login in 90 days
	NeverLoggedIn   int
	Admins          int
	Roles           int
	CustomRoles     int
	Groups          int
	APIKeys         int
	APIKeysActive   int
	APIKeysExpired  int
	APIKeysStale    int // unused for 90 days
	IdentityEnabled int
}

// Counted is a labelled tally, used for rankings and histograms.
type Counted struct {
	Label string
	Count int
}

// Sized is a labelled magnitude.
type Sized struct {
	Label string
	Value float64
}

// analyze derives every fact the builders need.
func analyze(s *Snapshot) Facts {
	var f Facts
	f.Containers = analyzeContainers(s)
	f.Images = analyzeImages(s)
	f.Security = analyzeSecurity(s)
	f.Capacity = analyzeCapacity(s)
	f.Reliability = analyzeReliability(s)
	f.Delivery = analyzeDelivery(s)
	f.Resilience = analyzeResilience(s)
	f.Access = analyzeAccess(s)
	f.Availability = analyzeAvailability(s, f.Containers)
	return f
}

// availabilityTarget is the service level every availability figure in the
// reports is measured against: 99.9% of observed time, which is the
// conventional "three nines" and works out at about 43 minutes a month.
// It is stated in the Methodology annex so a reader can argue with it.
const availabilityTarget = 99.9

// analyzeAvailability turns the host's uptime summary into report facts.
func analyzeAvailability(s *Snapshot, c ContainerFacts) AvailabilityFacts {
	var a AvailabilityFacts
	a.TargetPercent = availabilityTarget
	a.RequestedDays = s.Uptime.RequestedDays
	a.ObservedDays = s.Uptime.ObservedDays
	a.NoHealthcheck = c.NoHealthcheck

	u := s.Uptime
	if !s.available(srcUptime) || u.Totals.Tracked == 0 {
		return a
	}
	a.Assessed = true
	a.Rows = u.Containers

	// "Partial" is asserted whenever the history is materially shorter than
	// the period asked for. A day either side is tolerated: the window is
	// wall-clock and the rollup is bucketed in UTC, so an exact match is the
	// exception rather than the rule.
	if a.RequestedDays > 0 && a.ObservedDays > 0 && a.ObservedDays < a.RequestedDays-1 {
		a.Partial = true
	}

	t := u.Totals
	a.Tracked = t.Tracked
	a.WithDowntime = t.WithDowntime
	a.WithHealthcheck = t.WithHealthcheck
	a.UptimePercent = t.UptimePercent
	a.MeanUptimePercent = t.MeanUptimePercent
	a.WorstUptimePercent = t.WorstUptimePercent
	a.ObservedMinutes = t.ObservedMinutes
	a.DowntimeMinutes = t.DowntimeMinutes
	a.Outages = t.Outages
	a.Transitions = t.Transitions
	a.Probes = t.Probes
	a.ProbesFailed = t.ProbesFailed
	a.ProbePassPercent = t.ProbePassPercent

	if a.Outages > 0 {
		a.MTTRMinutes = a.DowntimeMinutes / float64(a.Outages)
		a.MTBFHours = (a.ObservedMinutes - a.DowntimeMinutes) / float64(a.Outages) / 60
	}
	if a.ObservedDays > 0 {
		a.OutageRatePerDay = float64(a.Outages) / float64(a.ObservedDays)
	}

	// Error budget: the target permits (100 - target)% of observed time to be
	// down. Express what is left as a share of that allowance, so "42% of the
	// budget remains" reads the same whether the period is a week or a month.
	if allowance := a.ObservedMinutes * (100 - a.TargetPercent) / 100; allowance > 0 {
		a.ErrorBudgetPercent = (allowance - a.DowntimeMinutes) / allowance * 100
	} else if a.DowntimeMinutes > 0 {
		a.ErrorBudgetPercent = -100
	}

	var probeMsTotal float64
	for _, r := range u.Containers {
		if r.Samples == 0 {
			continue
		}
		if r.UptimePercent < 100 {
			a.Worst = append(a.Worst, Sized{Label: uptimeLabel(r), Value: r.UptimePercent})
		}
		if r.FlapsPerDay > 0 {
			a.Flappiest = append(a.Flappiest, Sized{Label: uptimeLabel(r), Value: r.FlapsPerDay})
		}
		probeMsTotal += r.MeanProbeMs * float64(r.Probes)
		if r.MaxProbeMs > a.SlowestProbeMs {
			a.SlowestProbeMs = r.MaxProbeMs
		}
	}
	if a.Probes > 0 {
		a.MeanProbeMs = probeMsTotal / float64(a.Probes)
	}

	sort.Slice(a.Worst, func(i, j int) bool { return a.Worst[i].Value < a.Worst[j].Value })
	sort.Slice(a.Flappiest, func(i, j int) bool { return a.Flappiest[i].Value > a.Flappiest[j].Value })
	if len(a.Worst) > 10 {
		a.Worst = a.Worst[:10]
	}
	if len(a.Flappiest) > 10 {
		a.Flappiest = a.Flappiest[:10]
	}
	return a
}

// uptimeLabel names a row for a ranking. The id fallback matters: a container
// removed during the period still has history, and "—" in a table of the
// worst performers helps nobody.
func uptimeLabel(r uptimeRow) string {
	if r.ContainerName != "" {
		return r.ContainerName
	}
	return shortID(r.ContainerID)
}

func analyzeContainers(s *Snapshot) ContainerFacts {
	var c ContainerFacts
	stacks := map[string]int{}
	now := time.Now()

	for _, ct := range s.Containers {
		c.Total++
		switch strings.ToLower(ct.State) {
		case "running":
			c.Running++
		case "paused":
			c.Paused++
		case "restarting":
			c.Restarting++
		case "exited", "dead":
			c.Exited++
			c.Stopped++
		default:
			c.Stopped++
		}
		// Docker reports health inside the human-readable status string;
		// there is no separate field on the list endpoint.
		status := strings.ToLower(ct.Status)
		switch {
		case strings.Contains(status, "(unhealthy)"):
			c.Unhealthy++
		case strings.Contains(status, "(healthy)"):
			c.Healthy++
		case strings.ToLower(ct.State) == "running":
			c.NoHealthcheck++
		}
		if ct.Protected {
			c.Protected++
		}
		if ct.StackName != "" {
			stacks[ct.StackName]++
		} else {
			c.Unmanaged++
		}
		if ct.Created > 0 && strings.EqualFold(ct.State, "running") {
			age := now.Sub(time.Unix(ct.Created, 0)).Hours() / 24
			if age > c.OldestRunningDays {
				c.OldestRunningDays = age
			}
		}
	}
	c.Stacks = len(stacks)
	c.ByStack = topCounted(stacks, 10)
	if c.Total > 0 {
		c.RunningRatio = float64(c.Running) / float64(c.Total)
		c.UnhealthyRatio = float64(c.Unhealthy) / float64(c.Total)
	}
	// The dashboard endpoint is authoritative when the container list was
	// truncated by pagination.
	if s.Dashboard.Containers.Total > c.Total {
		c.Total = s.Dashboard.Containers.Total
		c.Running = s.Dashboard.Containers.Running
		c.Stopped = s.Dashboard.Containers.Stopped
		c.Paused = s.Dashboard.Containers.Paused
		c.Restarting = s.Dashboard.Containers.Restarting
		if c.Total > 0 {
			c.RunningRatio = float64(c.Running) / float64(c.Total)
			c.UnhealthyRatio = float64(c.Unhealthy) / float64(c.Total)
		}
	}
	return c
}

func analyzeImages(s *Snapshot) ImageFacts {
	var f ImageFacts
	now := time.Now()
	var ages []float64
	for _, im := range s.Images {
		f.Total++
		f.TotalBytes += im.Size
		if im.Containers > 0 {
			f.InUse++
		} else {
			f.Unused++
			f.ReclaimableBytes += im.Size
		}
		if len(im.RepoTags) == 0 || (len(im.RepoTags) == 1 && im.RepoTags[0] == "<none>:<none>") {
			f.Dangling++
		}
		if im.Created > 0 {
			age := now.Sub(time.Unix(im.Created, 0)).Hours() / 24
			ages = append(ages, age)
			if age > 90 {
				f.StaleOver90Days++
			}
		}
		f.Largest = append(f.Largest, Sized{Label: imageName(im), Value: float64(im.Size)})
	}
	f.MedianAgeDays = median(ages)
	sort.Slice(f.Largest, func(i, j int) bool { return f.Largest[i].Value > f.Largest[j].Value })
	if len(f.Largest) > 10 {
		f.Largest = f.Largest[:10]
	}
	return f
}

func analyzeSecurity(s *Snapshot) SecurityFacts {
	var f SecurityFacts
	f.Assessed = s.available(srcScans)

	byImage := map[string]scanRow{}
	for _, sc := range s.Scans {
		f.ScansTotal++
		// Keep the most recent scan per image; older ones are history,
		// not exposure.
		if prev, ok := byImage[sc.ImageRef]; !ok || parseTS(sc.CreatedAt).After(parseTS(prev.CreatedAt)) {
			byImage[sc.ImageRef] = sc
		}
	}
	for _, sc := range byImage {
		f.ImagesScanned++
		f.Critical += sc.Critical
		f.High += sc.High
		f.Medium += sc.Medium
		f.Low += sc.Low
		weight := float64(sc.Critical*10 + sc.High*5 + sc.Medium*2)
		if weight > 0 {
			f.TopVulnerableImages = append(f.TopVulnerableImages, Sized{Label: shortImage(sc.ImageRef), Value: weight})
		}
		if age := daysSince(sc.CompletedAt); age >= 0 && (f.LastScanAge < 0 || age < f.LastScanAge || f.LastScanAge == 0) {
			if f.LastScanAge == 0 || age < f.LastScanAge {
				f.LastScanAge = age
			}
		}
	}
	if f.ImagesScanned == 0 {
		f.LastScanAge = -1
	}
	// The summary endpoint counts across the whole estate; prefer it when
	// the per-scan list was paginated away.
	if sum := s.ScanSummary; sum.Critical+sum.High+sum.Medium+sum.Low > f.Critical+f.High+f.Medium+f.Low {
		f.Critical, f.High, f.Medium, f.Low = sum.Critical, sum.High, sum.Medium, sum.Low
	}
	f.WeightedExposure = float64(f.Critical*10 + f.High*5 + f.Medium*2)
	if n := len(s.Images); n > 0 {
		f.CoveragePercent = math.Min(100, 100*float64(f.ImagesScanned)/float64(n))
	}
	sort.Slice(f.TopVulnerableImages, func(i, j int) bool {
		return f.TopVulnerableImages[i].Value > f.TopVulnerableImages[j].Value
	})
	if len(f.TopVulnerableImages) > 8 {
		f.TopVulnerableImages = f.TopVulnerableImages[:8]
	}

	for _, a := range s.Audit {
		f.AuditEvents++
		switch strings.ToLower(a.Severity) {
		case "critical", "error":
			f.AuditCritical++
		case "warning", "warn":
			f.AuditWarn++
		}
		action := strings.ToLower(a.Action)
		if strings.Contains(action, "login") && (strings.Contains(action, "fail") || strings.Contains(action, "denied")) {
			f.FailedLogins++
		}
		if strings.Contains(action, "role") || strings.Contains(action, "permission") ||
			strings.Contains(action, "user.") || strings.Contains(action, "api_key") ||
			strings.Contains(action, "settings") {
			f.PrivilegedActions++
		}
	}
	return f
}

func analyzeCapacity(s *Snapshot) CapacityFacts {
	var f CapacityFacts
	f.Assessed = s.available(srcMetrics) || s.available(srcDockerInfo)
	f.RequestedDays = s.Period.Days

	// The instantaneous reading is the starting point and the fallback.
	// When history covers the period it is overwritten below, because a
	// report that prints a thirty-day cover has no business reporting
	// whatever the host happened to be doing when Generate was pressed.
	f.CPUPercent = s.Host.CPUPercent
	f.MemPercent = s.Host.MemPercent

	if pts := s.HostHistory.Points; s.available(srcMetricsHistory) && len(pts) > 0 {
		f.HistoryAvailable = true
		f.CPUStat = statFrom(pts,
			func(p metricPoint) float64 { return p.CPUPercent },
			func(p metricPoint) float64 { return p.CPUMax })
		f.MemStat = statFrom(pts,
			func(p metricPoint) float64 { return p.MemPercent },
			memPeakPercent)
		f.CPUPercent = f.CPUStat.Avg
		f.MemPercent = f.MemStat.Avg
		f.CoveredFrom, f.CoveredTo, f.CoveredDays = coveredWindow(s.HostHistory)
	}

	f.MemTotalBytes = s.Host.MemTotal
	f.MemUsedBytes = s.Host.MemUsed
	f.CPUCount = s.Host.NCPU
	if f.CPUCount == 0 {
		f.CPUCount = s.Docker.NCPU
	}
	if f.MemTotalBytes == 0 {
		f.MemTotalBytes = s.Docker.MemTotal
	}
	if f.CPUCount > 0 {
		f.Load1PerCore = s.Host.Load1 / float64(f.CPUCount)
	}
	if s.Host.Uptime > 0 {
		f.UptimeDays = float64(s.Host.Uptime) / 86400
	}
	for _, v := range s.Volumes {
		f.VolumeCount++
		if v.UsageData != nil {
			f.VolumeBytes += v.UsageData.Size
		}
		if v.RefCount == 0 {
			f.UnusedVolumes++
		}
	}
	// Headroom is governed by whichever of CPU and memory is tighter —
	// a host with 60 % memory free and 2 % CPU free has no headroom.
	//
	// It is measured at p95 rather than at the average, because headroom
	// is a question about the bad hours: capacity that exists only when
	// the estate is quiet is not headroom, it is a coincidence.
	worst := math.Max(f.CPUStat.grade(f.CPUPercent), f.MemStat.grade(f.MemPercent))
	f.HeadroomPercent = math.Max(0, 100-worst)
	return f
}

// statFrom reduces a metric series to one resource's period statistics.
//
// The mean is weighted by each bucket's sample count. The rollup folds a
// variable number of raw readings into each bucket — a bucket spanning a
// restart holds fewer — and an unweighted mean would give a bucket built
// from two readings the same say as one built from twenty.
//
// The p95 is taken over bucket averages, not over raw samples, because
// by the time a report reads the rollup tier the raw samples are gone.
// It is therefore a p95 of ten-minute means: a real measure of a bad
// hour, but not of a bad second, and the reports say so rather than
// letting the figure imply a resolution it does not have.
func statFrom(points []metricPoint, avg, max func(metricPoint) float64) periodStat {
	var st periodStat
	var sum, weight float64
	vals := make([]float64, 0, len(points))

	for _, pt := range points {
		v := avg(pt)
		w := float64(pt.Samples)
		if w <= 0 {
			w = 1
		}
		sum += v * w
		weight += w

		st.Samples += pt.Samples
		st.Buckets++
		st.Peak = math.Max(st.Peak, math.Max(max(pt), v))
		vals = append(vals, v)
	}

	if weight > 0 {
		st.Avg = sum / weight
	}
	// A series of buckets that each reported a zero sample count is still
	// evidence that the window was observed, so Samples must not be left
	// at zero — grade() reads it as "nothing was measured".
	if st.Samples == 0 && st.Buckets > 0 {
		st.Samples = st.Buckets
	}
	st.P95 = percentile(vals, 95)
	return st
}

// memPeakPercent recovers a bucket's peak memory percentage.
//
// The rollup stores mem_percent_max, but the history endpoint does not
// carry it, so the peak is reconstructed from the bytes it does carry —
// which is exact, not an approximation. When the limit is unknown the
// bucket contributes its own mean rather than a zero: a zero would drag
// the peak down and understate exactly the pressure the column exists
// to show.
func memPeakPercent(p metricPoint) float64 {
	if p.MemLimit > 0 && p.MemUsedMax > 0 {
		return float64(p.MemUsedMax) / float64(p.MemLimit) * 100
	}
	return p.MemPercent
}

// percentile returns the p-th percentile of vals using nearest-rank.
//
// Nearest-rank rather than interpolation: the value returned is one that
// was actually observed, which matters when the figure is going to be
// quoted in a document and compared against a threshold.
func percentile(vals []float64, p float64) float64 {
	if len(vals) == 0 {
		return 0
	}
	sorted := append([]float64(nil), vals...)
	sort.Float64s(sorted)

	rank := int(math.Ceil(p / 100 * float64(len(sorted))))
	if rank < 1 {
		rank = 1
	}
	if rank > len(sorted) {
		rank = len(sorted)
	}
	return sorted[rank-1]
}

// coveredWindow reports what a series actually spans.
//
// It prefers the store's own coveredFrom/coveredTo, which is the honest
// answer, and falls back to the timestamps of the first and last points
// when an older host has not supplied them. Both may be absent, in which
// case the caller learns nothing was covered rather than being handed a
// zero it might mistake for a measurement.
func coveredWindow(series metricSeries) (time.Time, time.Time, float64) {
	first := series.CoveredFrom
	last := series.CoveredTo
	if first == "" || last == "" {
		if len(series.Points) == 0 {
			return time.Time{}, time.Time{}, 0
		}
		first = series.Points[0].Timestamp
		last = series.Points[len(series.Points)-1].Timestamp
	}

	from, errFrom := parseMetricTime(first)
	to, errTo := parseMetricTime(last)
	if errFrom != nil || errTo != nil || to.Before(from) {
		return time.Time{}, time.Time{}, 0
	}
	return from, to, to.Sub(from).Hours() / 24
}

// parseMetricTime accepts the two shapes the metric store emits: RFC3339
// from the rollup reader, and SQLite's own "2006-01-02 15:04:05" from the
// raw tier.
func parseMetricTime(v string) (time.Time, error) {
	if t, err := time.Parse(time.RFC3339, v); err == nil {
		return t.UTC(), nil
	}
	t, err := time.Parse("2006-01-02 15:04:05", v)
	return t.UTC(), err
}

func analyzeReliability(s *Snapshot) ReliabilityFacts {
	var f ReliabilityFacts
	f.Assessed = s.available(srcActivity) || s.available(srcContainers)

	offenders := map[string]int{}
	byDay := map[string]int{}
	// downAt tracks when each container last went down, so the next start
	// gives a recovery interval.
	downAt := map[string]time.Time{}
	var recoveries []float64

	events := append([]activityRow(nil), s.Activity...)
	sort.Slice(events, func(i, j int) bool { return parseTS(events[i].Timestamp).Before(parseTS(events[j].Timestamp)) })

	for _, e := range events {
		f.EventsTotal++
		action := strings.ToLower(e.Action)
		et := strings.ToLower(e.EventType)
		ts := parseTS(e.Timestamp)
		if !ts.IsZero() {
			byDay[ts.Format("2006-01-02")]++
		}
		name := e.ContainerName
		if name == "" {
			name = shortID(e.ContainerID)
		}

		switch {
		case strings.Contains(action, "restart"):
			f.Restarts++
			offenders[name]++
			downAt[name] = ts
		case strings.Contains(action, "oom"):
			f.OOMKills++
			f.Crashes++
			offenders[name]++
			downAt[name] = ts
		case strings.Contains(action, "die") || strings.Contains(action, "kill") ||
			strings.Contains(action, "crash") || strings.Contains(et, "error"):
			f.Crashes++
			offenders[name]++
			downAt[name] = ts
		case strings.Contains(action, "start") || strings.Contains(action, "health_status: healthy"):
			if d, ok := downAt[name]; ok && !ts.IsZero() && ts.After(d) {
				if mins := ts.Sub(d).Minutes(); mins >= 0 && mins < 60*24*7 {
					recoveries = append(recoveries, mins)
				}
				delete(downAt, name)
			}
		}
	}

	// Containers currently stuck restarting are crash-looping now, which
	// matters more to a reader than anything in the event history.
	for _, ct := range s.Containers {
		if strings.EqualFold(ct.State, "restarting") {
			offenders[containerName(ct)] += 3
			f.CrashLooping = append(f.CrashLooping, Counted{Label: containerName(ct), Count: 1})
		}
	}

	f.TopOffenders = topCounted(offenders, 8)
	f.EventsByDay = seriesByDay(byDay, s.Period)
	f.IncidentDays = 0
	for _, d := range f.EventsByDay {
		if d.Count > 0 {
			f.IncidentDays++
		}
	}
	if s.Period.Days > 0 {
		f.RestartRatePerDay = float64(f.Restarts+f.Crashes) / s.Period.Days
	}
	f.RecoverySamples = len(recoveries)
	f.MeanRecoveryMinutes = mean(recoveries)
	return f
}

func analyzeDelivery(s *Snapshot) DeliveryFacts {
	var f DeliveryFacts
	f.Assessed = s.available(srcAudit) || s.available(srcUpdates) || s.available(srcGitops)

	byDay := map[string]int{}
	for _, a := range s.Audit {
		action := strings.ToLower(a.Action)
		ts := parseTS(a.Timestamp)
		isDeploy := strings.Contains(action, "deploy") || strings.Contains(action, "stack.up") ||
			strings.Contains(action, "container.create") || strings.Contains(action, "stack.create") ||
			strings.Contains(action, "stack.update") || strings.Contains(action, "image.pull")
		if isDeploy {
			f.Deployments++
			if !ts.IsZero() {
				byDay[ts.Format("2006-01-02")]++
			}
		}
		if strings.Contains(action, "rollback") || strings.Contains(action, "revert") {
			f.Rollbacks++
		}
		if strings.Contains(action, "fail") && isDeploy {
			f.FailedChanges++
		}
	}
	for _, h := range s.UpdateHist {
		switch strings.ToLower(h.Status) {
		case "success", "succeeded", "ok", "completed":
			f.UpdatesApplied++
			f.Deployments++
		case "failed", "error":
			f.UpdatesFailed++
			f.FailedChanges++
		}
	}
	var promoHours []float64
	for _, p := range s.Promotions {
		f.PromotionsTotal++
		switch strings.ToLower(p.Status) {
		case "succeeded", "success", "completed", "merged":
			f.PromotionsSucceeded++
			f.Deployments++
			if st, ct := parseTS(p.CreatedAt), parseTS(p.CompletedAt); !st.IsZero() && ct.After(st) {
				promoHours = append(promoHours, ct.Sub(st).Hours())
			}
		case "failed", "rejected", "error":
			f.FailedChanges++
		}
	}
	f.MeanPromotionHours = mean(promoHours)
	f.DeploymentsByDay = seriesByDay(byDay, s.Period)
	if s.Period.Days > 0 {
		f.DeploymentsPerWeek = float64(f.Deployments) / s.Period.Days * 7
	}
	if total := f.Deployments; total > 0 {
		f.ChangeFailureRate = 100 * float64(f.FailedChanges+f.Rollbacks) / float64(total)
	}
	return f
}

func analyzeResilience(s *Snapshot) ResilienceFacts {
	var f ResilienceFacts
	f.Assessed = s.available(srcBackups)

	covered := map[string]bool{}
	for _, p := range s.BackupPlans {
		f.Plans++
		if p.Enabled {
			f.PlansEnabled++
		}
		if p.ContainerName != "" {
			covered[p.ContainerName] = true
		}
	}
	f.ContainersCovered = len(covered)

	lastSuccess := time.Time{}
	for _, r := range s.BackupRuns {
		f.Runs++
		switch strings.ToLower(r.Status) {
		case "success", "succeeded", "completed", "ok":
			f.RunsSucceeded++
			f.TotalArchiveBytes += r.ArchiveSize
			if ts := firstTS(r.CompletedAt, r.CreatedAt); ts.After(lastSuccess) {
				lastSuccess = ts
			}
		case "failed", "error", "cancelled", "canceled":
			f.RunsFailed++
		}
	}
	if f.Runs > 0 {
		f.SuccessRate = 100 * float64(f.RunsSucceeded) / float64(f.Runs)
	}
	if lastSuccess.IsZero() {
		f.LastSuccessAge = -1
	} else {
		f.LastSuccessAge = time.Since(lastSuccess).Hours() / 24
	}
	// Coverage is measured against running workloads: a stopped container
	// that nobody backs up is usually a deliberate choice.
	running := 0
	for _, ct := range s.Containers {
		if strings.EqualFold(ct.State, "running") {
			running++
		}
	}
	if running > 0 {
		f.CoveragePercent = math.Min(100, 100*float64(f.ContainersCovered)/float64(running))
	}
	f.Destinations = len(s.Storage)
	return f
}

func analyzeAccess(s *Snapshot) AccessFacts {
	var f AccessFacts
	f.Assessed = s.available(srcUsers)
	now := time.Now()

	for _, u := range s.Users {
		f.Users++
		if u.IsActive {
			f.ActiveUsers++
		} else {
			f.InactiveUsers++
		}
		if strings.EqualFold(u.Role, "admin") || strings.EqualFold(u.Role, "administrator") {
			f.Admins++
		}
		if u.LastLogin == nil || *u.LastLogin == "" {
			f.NeverLoggedIn++
			continue
		}
		if ts := parseTS(*u.LastLogin); !ts.IsZero() && now.Sub(ts).Hours()/24 > 90 {
			f.DormantUsers++
		}
	}
	for _, r := range s.Roles {
		f.Roles++
		if !r.IsSystem {
			f.CustomRoles++
		}
	}
	f.Groups = len(s.Groups)
	for _, k := range s.APIKeys {
		f.APIKeys++
		if k.IsActive {
			f.APIKeysActive++
		}
		if k.ExpiresAt != nil && *k.ExpiresAt != "" {
			if ts := parseTS(*k.ExpiresAt); !ts.IsZero() && ts.Before(now) {
				f.APIKeysExpired++
			}
		}
		if k.LastUsedAt == nil || *k.LastUsedAt == "" {
			f.APIKeysStale++
		} else if ts := parseTS(*k.LastUsedAt); !ts.IsZero() && now.Sub(ts).Hours()/24 > 90 {
			f.APIKeysStale++
		}
	}
	for _, ip := range s.Identity {
		if ip.Enabled {
			f.IdentityEnabled++
		}
	}
	return f
}

// --- scoring ---------------------------------------------------------

// Domain weights. Published in every report's Methodology annex so the
// overall figure can be interrogated rather than taken on trust.
var domainWeights = map[string]int{
	"security":     25,
	"availability": 20,
	"resilience":   20,
	"stability":    15,
	"capacity":     10,
	"delivery":     10,
}

// bandFor maps a score to its band. One place, so a chart, a table and a
// sentence can never disagree about what 69 means.
func bandFor(score int) Band {
	switch {
	case score >= 85:
		return BandStrong
	case score >= 70:
		return BandStable
	case score >= 55:
		return BandWatch
	case score >= 35:
		return BandAtRisk
	default:
		return BandCritical
	}
}

// buildScorecard scores each domain the snapshot can support.
//
// A domain with no data is reported as Not assessed and excluded from
// the weighted mean. It is never scored as good — an absent scanner must
// not read as a clean bill of health.
func buildScorecard(f Facts, s *Snapshot) *Scorecard {
	var domains []DomainRate

	add := func(key, label string, assessed bool, score float64, basis, note string) {
		d := DomainRate{Key: key, Label: label, Weight: domainWeights[key], Basis: basis, Note: note}
		if !assessed {
			d.Band = BandNotAssessed
			d.Score = -1
			domains = append(domains, d)
			return
		}
		d.Score = clampInt(int(math.Round(score)), 0, 100)
		d.Band = bandFor(d.Score)
		domains = append(domains, d)
	}

	// Availability — was what should have been up, up?
	//
	// Two scoring rules, and which one applies depends on whether the health
	// and uptime history answered. Measured uptime is strictly better
	// evidence: the point-in-time rule can only see the instant the report
	// ran, so a workload that was down for six hours and came back an hour
	// ago scores perfectly under it. When history exists, it wins.
	availAssessed := s.available(srcContainers) && f.Containers.Total > 0
	avail := 100.0
	availBasis := "unhealthy share, containers stuck restarting, restart rate per container per day"

	if a := f.Availability; a.Assessed {
		availAssessed = true
		availBasis = "measured uptime against a " + fmtDecimal(a.TargetPercent, 1) +
			"% target, share of the error budget spent, outage rate and healthcheck pass rate"

		// Full marks at target, zero at ten times the permitted downtime.
		// Linear in the shortfall rather than in the uptime percentage,
		// because the interesting range is entirely in the last decimal:
		// 99.9% and 99.0% are a factor of ten apart and would otherwise
		// score within one point of each other.
		allowed := 100 - a.TargetPercent
		shortfall := math.Max(0, a.TargetPercent-a.UptimePercent)
		avail = 100 - 60*math.Min(1, shortfall/(allowed*9))

		// Breadth matters as well as depth: one workload down all period and
		// twenty down briefly are different failures with the same total.
		if a.Tracked > 0 {
			avail -= 20 * math.Min(1, float64(a.WithDowntime)/float64(a.Tracked)*2)
		}
		if a.Probes > 0 && a.ProbePassPercent < 100 {
			avail -= 20 * math.Min(1, (100-a.ProbePassPercent)/10)
		}
	} else {
		avail -= 40 * f.Containers.UnhealthyRatio
		if f.Containers.Total > 0 {
			down := float64(f.Containers.Restarting) / float64(f.Containers.Total)
			avail -= 30 * math.Min(1, down*4)
		}
		if f.Reliability.RestartRatePerDay > 0 && f.Containers.Total > 0 {
			perContainer := f.Reliability.RestartRatePerDay / float64(f.Containers.Total)
			avail -= 30 * math.Min(1, perContainer/0.5)
		}
	}
	add("availability", "Availability", availAssessed, avail, availBasis, availabilityNote(f))

	// Stability — how often does something break, and how fast does it
	// come back?
	stabAssessed := f.Reliability.Assessed
	stab := 100.0
	if s.Period.Days > 0 && f.Containers.Total > 0 {
		crashRate := float64(f.Reliability.Crashes) / s.Period.Days / float64(f.Containers.Total)
		stab -= 45 * math.Min(1, crashRate/0.2)
	}
	stab -= 25 * math.Min(1, float64(len(f.Reliability.CrashLooping))/3)
	stab -= 15 * math.Min(1, float64(f.Reliability.OOMKills)/5)
	if f.Reliability.RecoverySamples > 0 {
		// An hour to recover is the point where this term is fully spent;
		// it matches the <1 h band used for high-severity incidents.
		stab -= 15 * math.Min(1, f.Reliability.MeanRecoveryMinutes/60)
	}
	add("stability", "Stability", stabAssessed, stab,
		"crash rate per container per day, crash-looping workloads, OOM kills, mean recovery time",
		stabilityNote(f))

	// Security — exposure, coverage and freshness together. Any one of
	// them alone flatters the estate.
	secAssessed := f.Security.Assessed
	sec := 100.0
	imageBase := math.Max(1, float64(maxIntOf(f.Images.Total, f.Security.ImagesScanned)))
	sec -= 45 * math.Min(1, f.Security.WeightedExposure/(imageBase*12))
	sec -= 10 * math.Min(1, float64(f.Security.Critical)/3)
	if f.Security.CoveragePercent < 100 {
		sec -= 25 * (1 - f.Security.CoveragePercent/100)
	}
	if f.Security.LastScanAge > 30 {
		sec -= 10 * math.Min(1, (f.Security.LastScanAge-30)/60)
	}
	if f.Security.FailedLogins > 0 {
		sec -= 10 * math.Min(1, float64(f.Security.FailedLogins)/25)
	}
	add("security", "Security", secAssessed, sec,
		"severity-weighted findings per image (critical×10, high×5, medium×2), scan coverage, scan freshness, failed sign-ins",
		securityNote(f))

	// Capacity — headroom on the tighter of CPU and memory.
	capAssessed := f.Capacity.Assessed && (f.Capacity.MemTotalBytes > 0 || f.Capacity.CPUPercent > 0)
	capScore := headroomScore(f.Capacity.HeadroomPercent)
	if f.Capacity.Load1PerCore > 1 {
		capScore -= 15 * math.Min(1, f.Capacity.Load1PerCore-1)
	}
	add("capacity", "Capacity", capAssessed, capScore,
		"headroom on the tighter of CPU and memory, run-queue length per core",
		capacityNote(f))

	// Delivery — DORA's four keys, as far as a container platform can
	// observe them. Lead time is not observable here and is excluded
	// rather than guessed.
	delAssessed := f.Delivery.Assessed && (f.Delivery.Deployments > 0 || f.Delivery.PromotionsTotal > 0)
	del := doraScore(f.Delivery)
	add("delivery", "Delivery", delAssessed, del,
		"deployment frequency and change failure rate, banded against the DORA benchmarks",
		deliveryNote(f))

	// Resilience — is there a recent, successful, broad backup?
	resAssessed := f.Resilience.Assessed
	res := 100.0
	res -= 45 * (1 - math.Min(1, f.Resilience.CoveragePercent/100))
	if f.Resilience.Runs > 0 {
		res -= 30 * (1 - math.Min(1, f.Resilience.SuccessRate/100))
	} else {
		res -= 30
	}
	switch {
	case f.Resilience.LastSuccessAge < 0:
		res -= 25
	case f.Resilience.LastSuccessAge > 7:
		res -= 25 * math.Min(1, (f.Resilience.LastSuccessAge-7)/23)
	}
	add("resilience", "Resilience", resAssessed, res,
		"backup coverage of running workloads, run success rate, age of the last successful backup",
		resilienceNote(f))

	// Weighted mean over the assessed domains only.
	totalWeight, weighted := 0, 0.0
	for _, d := range domains {
		if d.Band == BandNotAssessed {
			continue
		}
		totalWeight += d.Weight
		weighted += float64(d.Score * d.Weight)
	}
	sc := &Scorecard{Domains: domains, Weighting: domainWeights, Overall: -1, Band: BandNotAssessed}
	if totalWeight > 0 {
		sc.Overall = clampInt(int(math.Round(weighted/float64(totalWeight))), 0, 100)
		sc.Band = bandFor(sc.Overall)
	}
	return sc
}

// headroomScore maps free capacity to a score using the published bands.
func headroomScore(headroom float64) float64 {
	switch {
	case headroom >= 40:
		return 92
	case headroom >= 25:
		return 78
	case headroom >= 15:
		return 62
	case headroom >= 5:
		return 45
	default:
		return 25
	}
}

// doraScore bands deployment frequency and change failure rate against
// the DORA benchmarks: elite deploys on demand with a change failure
// rate near 5 %, low performers ship monthly or less.
func doraScore(d DeliveryFacts) float64 {
	freq := 35.0
	switch {
	case d.DeploymentsPerWeek >= 7:
		freq = 100
	case d.DeploymentsPerWeek >= 1:
		freq = 80
	case d.DeploymentsPerWeek >= 0.25:
		freq = 60
	}
	stab := 35.0
	switch {
	case d.Deployments == 0:
		stab = 60 // nothing shipped: no evidence either way
	case d.ChangeFailureRate <= 5:
		stab = 100
	case d.ChangeFailureRate <= 10:
		stab = 85
	case d.ChangeFailureRate <= 20:
		stab = 65
	}
	return (freq + stab) / 2
}

// --- notes -----------------------------------------------------------
//
// One short sentence per domain, stating the single fact that most
// explains the score. These are deterministic; the AI narrative, when
// available, adds prose around them but never replaces them.

func availabilityNote(f Facts) string {
	// Measured history leads when it exists — it is the only statement here
	// that describes the period rather than the moment.
	if a := f.Availability; a.Assessed {
		if a.UptimePercent < a.TargetPercent {
			return "Measured uptime " + fmtDecimal(a.UptimePercent, 2) + "% against a " +
				fmtDecimal(a.TargetPercent, 1) + "% target over " +
				plural(a.ObservedDays, "day", "days") + " of history"
		}
		if a.WithDowntime > 0 {
			return plural(a.WithDowntime, "workload had", "workloads had") +
				" downtime, and the fleet still met its target"
		}
		return "No downtime observed across " + plural(a.Tracked, "workload", "workloads")
	}
	if f.Containers.Unhealthy > 0 {
		return plural(f.Containers.Unhealthy, "workload is", "workloads are") + " failing their health check"
	}
	if f.Containers.Restarting > 0 {
		return plural(f.Containers.Restarting, "workload is", "workloads are") + " stuck restarting"
	}
	if f.Containers.NoHealthcheck > 0 {
		return plural(f.Containers.NoHealthcheck, "running workload has", "running workloads have") + " no health check defined"
	}
	return "All running workloads reported healthy"
}

func stabilityNote(f Facts) string {
	if n := len(f.Reliability.CrashLooping); n > 0 {
		return plural(n, "workload is", "workloads are") + " crash-looping now"
	}
	if f.Reliability.OOMKills > 0 {
		return plural(f.Reliability.OOMKills, "out-of-memory kill", "out-of-memory kills") + " in the period"
	}
	if f.Reliability.Crashes > 0 {
		return plural(f.Reliability.Crashes, "unexpected stop", "unexpected stops") + " in the period"
	}
	return "No unexpected stops recorded in the period"
}

func securityNote(f Facts) string {
	if f.Security.Critical > 0 {
		return plural(f.Security.Critical, "critical finding is", "critical findings are") + " open"
	}
	if f.Security.CoveragePercent < 60 {
		return "Only " + fmtPercent(f.Security.CoveragePercent) + " of images have been scanned"
	}
	if f.Security.High > 0 {
		return plural(f.Security.High, "high-severity finding is", "high-severity findings are") + " open"
	}
	return "No critical or high-severity findings open"
}

func capacityNote(f Facts) string {
	if f.Capacity.HeadroomPercent < 15 {
		return "Only " + fmtPercent(f.Capacity.HeadroomPercent) + " headroom remains on the tighter resource"
	}
	if f.Capacity.UnusedVolumes > 0 {
		return plural(f.Capacity.UnusedVolumes, "volume is", "volumes are") + " unreferenced"
	}
	return fmtPercent(f.Capacity.HeadroomPercent) + " headroom on the tighter resource"
}

func deliveryNote(f Facts) string {
	if f.Delivery.Deployments == 0 {
		return "No changes were deployed in the period"
	}
	if f.Delivery.ChangeFailureRate > 20 {
		return fmtPercent(f.Delivery.ChangeFailureRate) + " of changes needed a rollback or hotfix"
	}
	return fmtDecimal(f.Delivery.DeploymentsPerWeek, 1) + " deployments per week"
}

func resilienceNote(f Facts) string {
	if f.Resilience.Plans == 0 {
		return "No backup plans are configured"
	}
	if f.Resilience.LastSuccessAge < 0 {
		return "No backup has ever completed successfully"
	}
	if f.Resilience.LastSuccessAge > 7 {
		return "Last successful backup was " + fmtDecimal(f.Resilience.LastSuccessAge, 0) + " days ago"
	}
	return fmtPercent(f.Resilience.CoveragePercent) + " of running workloads are covered"
}
