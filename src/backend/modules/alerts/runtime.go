// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package alerts

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/therealmcsparrow/mcharbor/core/encryption"
	"github.com/therealmcsparrow/mcharbor/core/inapp"
	corenotify "github.com/therealmcsparrow/mcharbor/core/notify"
)

const (
	alertPollInterval      = 30 * time.Second
	alertImagePollInterval = 30 * time.Minute
	alertStartupDelay      = 10 * time.Second
	alertStateRetention    = 24 * time.Hour
)

var (
	comparisonConditionPattern = regexp.MustCompile(`(?i)^\s*(>=|<=|==|>|<)?\s*([0-9]+(?:\.[0-9]+)?)\s*([a-z%]*)\s*$`)
	durationConditionPattern   = regexp.MustCompile(`(?i)([0-9]+(?:\.[0-9]+)?\s*[smhd])`)
)

var byteUnits = map[string]float64{
	"b":   1,
	"kb":  1024,
	"kib": 1024,
	"mb":  1024 * 1024,
	"mib": 1024 * 1024,
	"gb":  1024 * 1024 * 1024,
	"gib": 1024 * 1024 * 1024,
	"tb":  1024 * 1024 * 1024 * 1024,
	"tib": 1024 * 1024 * 1024 * 1024,
	"pb":  1024 * 1024 * 1024 * 1024 * 1024,
	"pib": 1024 * 1024 * 1024 * 1024 * 1024,
}

type engineState struct {
	Active   bool
	LastSeen time.Time
}

type environmentRef struct {
	ID   string
	Name string
}

type comparisonCondition struct {
	Operator  string
	Threshold float64
	Display   string
}

type diskCondition struct {
	Comparison comparisonCondition
	UseBytes   bool
	Divisor    float64
	UnitLabel  string
}

type durationCondition struct {
	Threshold time.Duration
	Display   string
}

// Engine evaluates enabled alert rules and delivers notifications on state changes.
type Engine struct {
	db            *sql.DB
	logger        *slog.Logger
	service       *Service
	metricsSvc    MetricsSource
	containersSvc ContainerSource
	dockerInfoSvc SystemInfoSource
	sendChannel   func(context.Context, corenotify.ChannelRequest) (*corenotify.Delivery, error)
	sendInApp     func(inapp.Record) error

	cancel  context.CancelFunc
	wg      sync.WaitGroup
	stateMu sync.Mutex
	states  map[string]engineState
}

// NewEngine creates a background alerts engine.
func NewEngine(db *sql.DB, enc *encryption.Service, logger *slog.Logger, deps EngineDeps) *Engine {
	dispatcher := corenotify.NewDispatcher(db, enc)

	return &Engine{
		db:            db,
		logger:        logger,
		service:       NewService(db),
		metricsSvc:    deps.Metrics,
		containersSvc: deps.Containers,
		dockerInfoSvc: deps.SystemInfo,
		sendChannel:   dispatcher.SendChannel,
		sendInApp: func(record inapp.Record) error {
			return inapp.CreateBroadcast(db, record)
		},
		states: make(map[string]engineState),
	}
}

// Start launches the background alert evaluation loop.
func (e *Engine) Start() {
	ctx, cancel := context.WithCancel(context.Background())
	e.cancel = cancel

	e.wg.Add(1)
	go e.run(ctx)

	e.logger.Info("alerts engine started", "interval", alertPollInterval.String(), "imageInterval", alertImagePollInterval.String())
}

// Stop shuts down the background alert evaluation loop.
func (e *Engine) Stop() {
	if e.cancel != nil {
		e.cancel()
	}
	e.wg.Wait()
}

func (e *Engine) run(ctx context.Context) {
	defer e.wg.Done()

	startup := time.NewTimer(alertStartupDelay)
	generalTicker := time.NewTicker(alertPollInterval)
	imageTicker := time.NewTicker(alertImagePollInterval)
	defer startup.Stop()
	defer generalTicker.Stop()
	defer imageTicker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-startup.C:
			e.evaluateStandardAlerts(ctx)
			e.evaluateImageUpdateAlerts(ctx)
		case <-generalTicker.C:
			e.evaluateStandardAlerts(ctx)
		case <-imageTicker.C:
			e.evaluateImageUpdateAlerts(ctx)
		}
	}
}

func (e *Engine) evaluateStandardAlerts(ctx context.Context) {
	alerts, err := e.service.ListEnabled(ctx)
	if err != nil {
		e.logger.Error("alerts engine: list enabled alerts failed", "error", err)
		return
	}
	if len(alerts) == 0 {
		e.pruneStates(time.Now().UTC())
		return
	}

	envs, err := e.listDockerEnvironments(ctx)
	if err != nil {
		e.logger.Error("alerts engine: list environments failed", "error", err)
		return
	}
	if len(envs) == 0 {
		return
	}

	metricAlerts := make([]Alert, 0, len(alerts))
	downAlerts := make([]Alert, 0, len(alerts))
	diskAlerts := make([]Alert, 0, len(alerts))
	for _, alert := range alerts {
		switch alert.Type {
		case "cpu", "memory":
			metricAlerts = append(metricAlerts, alert)
		case "container_down":
			downAlerts = append(downAlerts, alert)
		case "disk":
			diskAlerts = append(diskAlerts, alert)
		}
	}

	for _, env := range envs {
		if len(metricAlerts) > 0 && e.metricsSvc != nil {
			stats, err := e.metricsSvc.AllContainerStats(ctx, env.ID)
			if err != nil {
				e.logger.Warn("alerts engine: metric snapshot failed", "environment", env.Name, "error", err)
			} else {
				e.evaluateMetricAlerts(ctx, env, metricAlerts, stats)
			}
		}

		if len(downAlerts) > 0 && e.containersSvc != nil {
			items, err := e.containersSvc.ListContainers(ctx, env.ID, true)
			if err != nil {
				e.logger.Warn("alerts engine: container list failed", "environment", env.Name, "error", err)
			} else {
				e.evaluateContainerDownAlerts(ctx, env, downAlerts, items)
			}
		}

		if len(diskAlerts) > 0 && e.metricsSvc != nil && e.dockerInfoSvc != nil {
			hostInfo, err := e.metricsSvc.HostInfo(ctx, env.ID)
			if err != nil {
				e.logger.Warn("alerts engine: host info failed", "environment", env.Name, "error", err)
				continue
			}

			systemInfo, err := e.dockerInfoSvc.SystemInfo(ctx, env.ID)
			if err != nil {
				e.logger.Warn("alerts engine: docker info failed", "environment", env.Name, "error", err)
				continue
			}

			e.evaluateDiskAlerts(ctx, env, diskAlerts, hostInfo, systemInfo)
		}
	}

	e.pruneStates(time.Now().UTC())
}

func (e *Engine) evaluateImageUpdateAlerts(ctx context.Context) {
	alerts, err := e.service.ListEnabled(ctx)
	if err != nil {
		e.logger.Error("alerts engine: list enabled alerts failed", "error", err)
		return
	}

	imageAlerts := make([]Alert, 0, len(alerts))
	for _, alert := range alerts {
		if alert.Type == "image_update" {
			imageAlerts = append(imageAlerts, alert)
		}
	}
	if len(imageAlerts) == 0 {
		return
	}

	envs, err := e.listDockerEnvironments(ctx)
	if err != nil {
		e.logger.Error("alerts engine: list environments failed", "error", err)
		return
	}

	for _, env := range envs {
		if e.containersSvc == nil {
			return
		}

		results, err := e.containersSvc.CheckImageUpdates(ctx, env.ID, nil)
		if err != nil {
			e.logger.Warn("alerts engine: image update check failed", "environment", env.Name, "error", err)
			continue
		}

		now := time.Now().UTC()
		for _, alert := range imageAlerts {
			prefix := statePrefix(alert, env.ID)
			seen := make(map[string]struct{}, len(results))

			for _, result := range results {
				if !matchesAlertTarget(alert.Target, result.ContainerName, result.ContainerID) {
					continue
				}

				key := prefix + result.ContainerID
				seen[key] = struct{}{}

				if !e.transitionState(key, now, result.UpdateAvailable) {
					continue
				}

				message := fmt.Sprintf(
					"Container %s in environment %s has an image update available.",
					quotedSubject(result.ContainerName),
					quotedSubject(env.Name),
				)
				if result.CurrentDigest != "" && result.RemoteDigest != "" {
					message = fmt.Sprintf(
						"Container %s in environment %s has an image update available (%s -> %s).",
						quotedSubject(result.ContainerName),
						quotedSubject(env.Name),
						shortDigest(result.CurrentDigest),
						shortDigest(result.RemoteDigest),
					)
				}

				e.deliver(ctx, alert, alertTitle(alert), message)
			}

			e.clearUnseen(prefix, seen, now)
		}
	}
}

func (e *Engine) evaluateMetricAlerts(ctx context.Context, env environmentRef, alerts []Alert, stats []MetricSample) {
	now := time.Now().UTC()

	for _, alert := range alerts {
		condition := parseThresholdCondition(alert.Condition, 80, true)
		prefix := statePrefix(alert, env.ID)
		seen := make(map[string]struct{}, len(stats))

		for _, stat := range stats {
			if !matchesAlertTarget(alert.Target, stat.Name, stat.ID) {
				continue
			}

			key := prefix + stat.ID
			seen[key] = struct{}{}

			var metricLabel string
			var actual float64
			switch alert.Type {
			case "cpu":
				metricLabel = "CPU usage"
				actual = stat.CPUPercent
			case "memory":
				metricLabel = "memory usage"
				actual = stat.MemPercent
			default:
				continue
			}

			triggered := compareValue(actual, condition.Operator, condition.Threshold)
			if !e.transitionState(key, now, triggered) {
				continue
			}

			message := fmt.Sprintf(
				"Container %s in environment %s reached %s of %s (rule %s).",
				quotedSubject(stat.Name),
				quotedSubject(env.Name),
				formatFloat(actual)+"%",
				metricLabel,
				condition.Display,
			)

			e.deliver(ctx, alert, alertTitle(alert), message)
		}

		e.clearUnseen(prefix, seen, now)
	}
}

func (e *Engine) evaluateContainerDownAlerts(ctx context.Context, env environmentRef, alerts []Alert, items []ContainerSummary) {
	now := time.Now().UTC()
	finishedAtCache := make(map[string]time.Time, len(items))

	for _, alert := range alerts {
		condition := parseDurationCondition(alert.Condition)
		prefix := statePrefix(alert, env.ID)
		seen := make(map[string]struct{}, len(items))

		for _, item := range items {
			name := containerName(item)
			if !matchesAlertTarget(alert.Target, name, item.ID) {
				continue
			}

			key := prefix + item.ID
			seen[key] = struct{}{}

			isDown := !strings.EqualFold(strings.TrimSpace(item.State), "running")
			downFor := time.Duration(0)
			if isDown {
				if finishedAt, ok := finishedAtCache[item.ID]; ok {
					if !finishedAt.IsZero() {
						downFor = now.Sub(finishedAt)
					}
				} else {
					info, err := e.containersSvc.InspectContainer(ctx, env.ID, item.ID)
					if err != nil {
						e.logger.Debug("alerts engine: inspect stopped container failed", "container", item.ID, "environment", env.Name, "error", err)
					} else if info.State != nil && info.State.FinishedAt != "" {
						finishedAt, err := time.Parse(time.RFC3339Nano, info.State.FinishedAt)
						if err == nil {
							finishedAtCache[item.ID] = finishedAt
							downFor = now.Sub(finishedAt)
						}
					}
				}
			}

			triggered := isDown
			if condition.Threshold > 0 && isDown && downFor > 0 {
				triggered = downFor >= condition.Threshold
			}

			if !e.transitionState(key, now, triggered) {
				continue
			}

			message := fmt.Sprintf(
				"Container %s in environment %s is down.",
				quotedSubject(name),
				quotedSubject(env.Name),
			)
			if condition.Threshold > 0 && downFor > 0 {
				message = fmt.Sprintf(
					"Container %s in environment %s has been down for %s (rule %s).",
					quotedSubject(name),
					quotedSubject(env.Name),
					formatDuration(downFor),
					condition.Display,
				)
			}

			e.deliver(ctx, alert, alertTitle(alert), message)
		}

		e.clearUnseen(prefix, seen, now)
	}
}

func (e *Engine) evaluateDiskAlerts(ctx context.Context, env environmentRef, alerts []Alert, hostInfo *HostMetrics, systemInfo *SystemInfo) {
	capacityBytes, ok := resolveDiskCapacity(systemInfo, hostInfo.DiskTotal)
	if !ok || capacityBytes <= 0 {
		return
	}

	usagePercent := (float64(hostInfo.DiskTotal) / float64(capacityBytes)) * 100
	now := time.Now().UTC()

	for _, alert := range alerts {
		condition := parseDiskCondition(alert.Condition)
		prefix := statePrefix(alert, env.ID)
		seen := make(map[string]struct{}, 1)

		if !matchesAlertTarget(alert.Target, env.Name, env.ID) {
			e.clearUnseen(prefix, seen, now)
			continue
		}

		key := prefix + env.ID
		seen[key] = struct{}{}

		actual := usagePercent
		valueLabel := formatFloat(actual) + "%"
		if condition.UseBytes {
			actual = float64(hostInfo.DiskTotal) / condition.Divisor
			valueLabel = formatFloat(actual) + " " + condition.UnitLabel
		}

		if !e.transitionState(key, now, compareValue(actual, condition.Comparison.Operator, condition.Comparison.Threshold)) {
			e.clearUnseen(prefix, seen, now)
			continue
		}

		message := fmt.Sprintf(
			"Environment %s reached Docker disk usage of %s (rule %s).",
			quotedSubject(env.Name),
			valueLabel,
			condition.Comparison.Display,
		)
		e.deliver(ctx, alert, alertTitle(alert), message)
		e.clearUnseen(prefix, seen, now)
	}
}

func (e *Engine) deliver(ctx context.Context, alert Alert, title, message string) {
	if alert.SendInApp {
		if err := e.sendInApp(inapp.Record{
			Level:      inAppLevel(alert.Severity),
			Title:      title,
			Message:    message,
			Action:     "alert.triggered",
			EntityType: "alert",
			EntityID:   alert.ID,
		}); err != nil {
			e.logger.Error("alerts engine: in-app delivery failed", "alert", alert.ID, "error", err)
		}
	}

	if strings.TrimSpace(alert.ChannelID) != "" {
		if _, err := e.sendChannel(ctx, corenotify.ChannelRequest{
			ChannelID: alert.ChannelID,
			Title:     title,
			Message:   message,
		}); err != nil {
			e.logger.Error("alerts engine: channel delivery failed", "alert", alert.ID, "channel", alert.ChannelID, "error", err)
		}
	}
}

func (e *Engine) transitionState(key string, now time.Time, active bool) bool {
	e.stateMu.Lock()
	defer e.stateMu.Unlock()

	state := e.states[key]
	shouldNotify := active && !state.Active
	state.Active = active
	state.LastSeen = now
	e.states[key] = state

	return shouldNotify
}

func (e *Engine) clearUnseen(prefix string, seen map[string]struct{}, now time.Time) {
	e.stateMu.Lock()
	defer e.stateMu.Unlock()

	for key, state := range e.states {
		if !strings.HasPrefix(key, prefix) {
			continue
		}
		if _, ok := seen[key]; ok {
			continue
		}
		state.Active = false
		state.LastSeen = now
		e.states[key] = state
	}
}

func (e *Engine) pruneStates(now time.Time) {
	cutoff := now.Add(-alertStateRetention)

	e.stateMu.Lock()
	defer e.stateMu.Unlock()

	for key, state := range e.states {
		if state.LastSeen.Before(cutoff) {
			delete(e.states, key)
		}
	}
}

func (e *Engine) listDockerEnvironments(ctx context.Context) ([]environmentRef, error) {
	rows, err := e.db.QueryContext(ctx,
		`SELECT id, name
		 FROM environments
		 WHERE is_active = 1
		   AND orchestrator_type = 'docker'
		 ORDER BY is_default DESC, name ASC
		 LIMIT 200`,
	)
	if err != nil {
		return nil, fmt.Errorf("listing docker environments: %w", err)
	}
	defer rows.Close()

	envs := make([]environmentRef, 0, 16)
	for rows.Next() {
		var env environmentRef
		if err := rows.Scan(&env.ID, &env.Name); err != nil {
			return nil, fmt.Errorf("scanning docker environment: %w", err)
		}
		envs = append(envs, env)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating docker environments: %w", err)
	}

	return envs, nil
}

func parseThresholdCondition(raw string, defaultThreshold float64, percent bool) comparisonCondition {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return comparisonCondition{
			Operator:  ">",
			Threshold: defaultThreshold,
			Display:   formatComparison(">", defaultThreshold, percent, ""),
		}
	}

	match := comparisonConditionPattern.FindStringSubmatch(trimmed)
	if len(match) == 0 {
		return comparisonCondition{
			Operator:  ">",
			Threshold: defaultThreshold,
			Display:   formatComparison(">", defaultThreshold, percent, ""),
		}
	}

	operator := strings.TrimSpace(match[1])
	if operator == "" {
		operator = ">"
	}

	threshold, err := strconv.ParseFloat(match[2], 64)
	if err != nil {
		threshold = defaultThreshold
	}

	return comparisonCondition{
		Operator:  operator,
		Threshold: threshold,
		Display:   formatComparison(operator, threshold, percent, ""),
	}
}

func parseDiskCondition(raw string) diskCondition {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		threshold := 80.0
		return diskCondition{
			Comparison: comparisonCondition{
				Operator:  ">",
				Threshold: threshold,
				Display:   formatComparison(">", threshold, true, ""),
			},
		}
	}

	match := comparisonConditionPattern.FindStringSubmatch(trimmed)
	if len(match) == 0 {
		return parseDiskCondition("")
	}

	operator := strings.TrimSpace(match[1])
	if operator == "" {
		operator = ">"
	}

	threshold, err := strconv.ParseFloat(match[2], 64)
	if err != nil {
		threshold = 80
	}

	unit := strings.ToLower(strings.TrimSpace(match[3]))
	if divisor, ok := byteUnits[unit]; ok {
		unitLabel := strings.ToUpper(unit)
		return diskCondition{
			Comparison: comparisonCondition{
				Operator:  operator,
				Threshold: threshold,
				Display:   formatComparison(operator, threshold, false, unitLabel),
			},
			UseBytes:  true,
			Divisor:   divisor,
			UnitLabel: unitLabel,
		}
	}

	return diskCondition{
		Comparison: comparisonCondition{
			Operator:  operator,
			Threshold: threshold,
			Display:   formatComparison(operator, threshold, true, ""),
		},
	}
}

func parseDurationCondition(raw string) durationCondition {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return durationCondition{}
	}

	value := durationConditionPattern.FindString(trimmed)
	if value == "" {
		return durationCondition{Display: trimmed}
	}

	normalized := strings.ReplaceAll(strings.ToLower(strings.TrimSpace(value)), " ", "")
	threshold, err := parseExtendedDuration(normalized)
	if err != nil {
		return durationCondition{}
	}

	return durationCondition{
		Threshold: threshold,
		Display:   "stopped > " + formatDuration(threshold),
	}
}

func parseExtendedDuration(value string) (time.Duration, error) {
	if strings.HasSuffix(value, "d") {
		number := strings.TrimSuffix(value, "d")
		parsed, err := strconv.ParseFloat(number, 64)
		if err != nil {
			return 0, err
		}
		return time.Duration(parsed * float64(24*time.Hour)), nil
	}

	return time.ParseDuration(value)
}

func compareValue(actual float64, operator string, threshold float64) bool {
	switch operator {
	case ">":
		return actual > threshold
	case "<":
		return actual < threshold
	case ">=":
		return actual >= threshold
	case "<=":
		return actual <= threshold
	case "==":
		return actual == threshold
	default:
		return false
	}
}

func matchesAlertTarget(target string, candidates ...string) bool {
	patterns := splitAlertTargets(target)
	if len(patterns) == 0 {
		return true
	}

	for _, candidate := range candidates {
		normalizedCandidate := strings.ToLower(strings.TrimSpace(candidate))
		if normalizedCandidate == "" {
			continue
		}

		for _, pattern := range patterns {
			if pattern == "*" {
				return true
			}
			if wildcardMatch(pattern, normalizedCandidate) {
				return true
			}
		}
	}

	return false
}

func splitAlertTargets(target string) []string {
	trimmed := strings.TrimSpace(target)
	if trimmed == "" || trimmed == "*" {
		return nil
	}

	rawParts := strings.FieldsFunc(trimmed, func(r rune) bool {
		return r == ',' || r == '\n'
	})

	parts := make([]string, 0, len(rawParts))
	for _, part := range rawParts {
		normalized := strings.ToLower(strings.TrimSpace(part))
		if normalized == "" {
			continue
		}
		parts = append(parts, normalized)
	}

	return parts
}

func wildcardMatch(pattern, candidate string) bool {
	if pattern == candidate {
		return true
	}

	expression := "^" + regexp.QuoteMeta(pattern) + "$"
	expression = strings.ReplaceAll(expression, `\*`, ".*")
	matched, err := regexp.MatchString(expression, candidate)
	if err != nil {
		return false
	}

	return matched
}

func resolveDiskCapacity(info *SystemInfo, usedBytes int64) (int64, bool) {
	if total, ok := readDriverStatusBytes(info, "data space total", "space total", "total space"); ok && total > 0 {
		return total, true
	}

	if available, ok := readDriverStatusBytes(info, "data space available", "space available", "available space"); ok && available >= 0 {
		return usedBytes + available, true
	}

	return 0, false
}

func readDriverStatusBytes(info *SystemInfo, keys ...string) (int64, bool) {
	if info == nil {
		return 0, false
	}

	lookup := make(map[string]struct{}, len(keys))
	for _, key := range keys {
		lookup[strings.ToLower(strings.TrimSpace(key))] = struct{}{}
	}

	for _, pair := range info.DriverStatus {
		if len(pair) < 2 {
			continue
		}

		label := strings.ToLower(strings.TrimSpace(pair[0]))
		if _, ok := lookup[label]; !ok {
			continue
		}

		parsed, ok := parseByteCount(pair[1])
		if ok && parsed > 0 {
			return parsed, true
		}
	}

	return 0, false
}

func parseByteCount(value string) (int64, bool) {
	match := comparisonConditionPattern.FindStringSubmatch(strings.TrimSpace(value))
	if len(match) == 0 {
		return 0, false
	}

	amount, err := strconv.ParseFloat(match[2], 64)
	if err != nil {
		return 0, false
	}

	unit := strings.ToLower(strings.TrimSpace(match[3]))
	divisor, ok := byteUnits[unit]
	if !ok {
		return 0, false
	}

	return int64(amount * divisor), true
}

func statePrefix(alert Alert, envID string) string {
	return alert.Type + ":" + alert.ID + ":" + envID + ":"
}

func alertTitle(alert Alert) string {
	name := strings.TrimSpace(alert.Name)
	if name == "" {
		name = humanizeAlertType(alert.Type)
	}
	return "Alert triggered: " + name
}

func humanizeAlertType(alertType string) string {
	switch alertType {
	case "cpu":
		return "CPU usage"
	case "memory":
		return "Memory usage"
	case "disk":
		return "Disk usage"
	case "container_down":
		return "Container down"
	case "image_update":
		return "Image update"
	default:
		return "Alert"
	}
}

func inAppLevel(severity string) string {
	switch strings.ToLower(strings.TrimSpace(severity)) {
	case "critical", "warning":
		return "warning"
	default:
		return "info"
	}
}

func containerName(item ContainerSummary) string {
	if len(item.Names) == 0 {
		return item.ID[:12]
	}
	return strings.TrimPrefix(item.Names[0], "/")
}

func quotedSubject(value string) string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return "unknown"
	}
	return trimmed
}

func shortDigest(value string) string {
	parts := strings.SplitN(value, ":", 2)
	if len(parts) == 2 {
		value = parts[1]
	}
	if len(value) > 12 {
		return value[:12]
	}
	return value
}

func formatComparison(operator string, threshold float64, percent bool, unit string) string {
	suffix := unit
	if percent {
		suffix = "%"
	}
	if suffix != "" {
		return operator + " " + formatFloat(threshold) + suffix
	}
	return operator + " " + formatFloat(threshold)
}

func formatFloat(value float64) string {
	return strconv.FormatFloat(value, 'f', 1, 64)
}

func formatDuration(value time.Duration) string {
	if value >= 24*time.Hour {
		return strconv.FormatFloat(value.Hours()/24, 'f', 1, 64) + "d"
	}
	if value >= time.Hour {
		return value.Truncate(time.Minute).String()
	}
	if value >= time.Minute {
		return value.Truncate(time.Second).String()
	}
	return value.Truncate(time.Second).String()
}
