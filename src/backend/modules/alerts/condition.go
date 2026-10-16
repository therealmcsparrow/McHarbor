// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package alerts

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/therealmcsparrow/mcharbor/core/plugin/specs/alerting"
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

var defaultOperators = []string{">", ">=", "<", "<=", "=="}

// errConditionInvalid is returned when a submitted condition does not
// satisfy the declaration it names.
var errConditionInvalid = errors.New("alert condition invalid")

// decodeConditionConfig reads the stored JSON blob. A malformed or
// empty blob yields a zero config rather than an error: the rule still
// resolves, and the declaration's defaults fill the gaps. Refusing to
// load a rule because its config JSON is unreadable would take an
// operator's alerting offline over a formatting problem.
func decodeConditionConfig(raw string) ConditionConfig {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" || trimmed == "{}" {
		return ConditionConfig{}
	}

	var cfg ConditionConfig
	if err := json.Unmarshal([]byte(trimmed), &cfg); err != nil {
		return ConditionConfig{}
	}
	return cfg
}

// encodeConditionConfig serialises a config for storage.
func encodeConditionConfig(cfg ConditionConfig) (string, error) {
	data, err := json.Marshal(cfg)
	if err != nil {
		return "", fmt.Errorf("encoding condition config: %w", err)
	}
	return string(data), nil
}

// decodeEnvironmentIDs reads the stored JSON array. Empty means every
// active environment.
func decodeEnvironmentIDs(raw string) []string {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" || trimmed == "[]" {
		return nil
	}

	var ids []string
	if err := json.Unmarshal([]byte(trimmed), &ids); err != nil {
		return nil
	}

	out := make([]string, 0, len(ids))
	for _, id := range ids {
		if trimmedID := strings.TrimSpace(id); trimmedID != "" {
			out = append(out, trimmedID)
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// encodeEnvironmentIDs serialises the scope list for storage.
func encodeEnvironmentIDs(ids []string) (string, error) {
	if len(ids) == 0 {
		return "", nil
	}
	data, err := json.Marshal(ids)
	if err != nil {
		return "", fmt.Errorf("encoding environment scope: %w", err)
	}
	return string(data), nil
}

// applyConditionDefaults fills unset fields from the declaration, so a
// rule saved before a condition grew a default still evaluates the way
// the declaration says it should.
func applyConditionDefaults(cfg ConditionConfig, decl alerting.AlertConditionDecl) ConditionConfig {
	if cfg.Operator == "" {
		cfg.Operator = decl.DefaultOperator
	}
	if cfg.Operator == "" && (decl.Kind == alerting.KindThreshold || decl.Kind == alerting.KindCountInWindow) {
		cfg.Operator = ">"
	}
	if cfg.Threshold == 0 && decl.Kind == alerting.KindThreshold {
		cfg.Threshold = decl.DefaultThreshold
	}
	if cfg.Unit == "" {
		cfg.Unit = decl.Unit
	}
	return cfg
}

// validateCondition checks a submitted config against its declaration.
//
// This is the whole point of the structured builder: the free-text
// field it replaces silently fell back to a default threshold when it
// could not parse what the operator typed, so a rule could watch
// something nobody asked for. Here a bad operator is a 400.
func validateCondition(cfg ConditionConfig, decl alerting.AlertConditionDecl) error {
	operators := decl.Operators
	if len(operators) == 0 {
		operators = defaultOperators
	}

	switch decl.Kind {
	case alerting.KindThreshold, alerting.KindCountInWindow:
		if cfg.Operator == "" {
			return fmt.Errorf("%w: operator is required", errConditionInvalid)
		}
		if !containsString(operators, cfg.Operator) {
			return fmt.Errorf("%w: operator %q is not allowed for %s", errConditionInvalid, cfg.Operator, decl.Key)
		}
	case alerting.KindDuration:
		if cfg.ForSeconds < 0 {
			return fmt.Errorf("%w: duration must not be negative", errConditionInvalid)
		}
	case alerting.KindEvent:
		// Nothing to validate: an event has no threshold of its own.
	default:
		return fmt.Errorf("%w: unknown condition kind %q", errConditionInvalid, decl.Kind)
	}

	if decl.Kind == alerting.KindCountInWindow && cfg.WindowSeconds <= 0 {
		return fmt.Errorf("%w: window is required for %s", errConditionInvalid, decl.Key)
	}
	if cfg.ForSeconds < 0 || cfg.WindowSeconds < 0 {
		return fmt.Errorf("%w: durations must not be negative", errConditionInvalid)
	}

	for _, field := range decl.Fields {
		if !field.Required {
			continue
		}
		if strings.TrimSpace(cfg.Params[field.Key]) == "" {
			return fmt.Errorf("%w: %s is required", errConditionInvalid, field.Key)
		}
	}

	return nil
}

// conditionDisplay renders a human-readable summary used in messages
// and in the rule card, e.g. "> 80% for 5m" or "stopped > 10m".
func conditionDisplay(cfg ConditionConfig, decl alerting.AlertConditionDecl) string {
	var base string

	switch decl.Kind {
	case alerting.KindThreshold:
		base = formatComparison(cfg.Operator, cfg.Threshold, cfg.Unit)
	case alerting.KindCountInWindow:
		base = fmt.Sprintf("%s in %s",
			formatComparison(cfg.Operator, cfg.Threshold, cfg.Unit),
			formatDuration(time.Duration(cfg.WindowSeconds)*time.Second))
	case alerting.KindDuration:
		if cfg.ForSeconds > 0 {
			return "for " + formatDuration(time.Duration(cfg.ForSeconds)*time.Second)
		}
		return decl.DisplayName
	default:
		base = decl.DisplayName
	}

	if cfg.ForSeconds > 0 && decl.Kind != alerting.KindDuration {
		base += " for " + formatDuration(time.Duration(cfg.ForSeconds)*time.Second)
	}
	return base
}

// -----------------------------------------------------------------------------
// Legacy free-text conditions
//
// Rules created before the structured builder stored strings like "> 80%",
// ">= 10 GB" or "5m". Migration 079 could not translate them — doing it in SQL
// would mean a second implementation of this grammar, and two implementations
// drift. Instead the service backfills a rule on first read using exactly the
// parsers that produced the old behaviour, so the reading is identical by
// construction and the conversion happens once.
// -----------------------------------------------------------------------------

// parseLegacyCondition converts a pre-migration condition string into a
// structured config for the condition it was mapped onto.
func parseLegacyCondition(conditionKey, raw string, decl alerting.AlertConditionDecl) ConditionConfig {
	switch decl.Kind {
	case alerting.KindDuration:
		legacy := parseDurationCondition(raw)
		return ConditionConfig{ForSeconds: int(legacy.Threshold / time.Second)}

	case alerting.KindThreshold:
		// docker_disk accepted byte units as well as percentages; the
		// other thresholds were percentages only.
		if conditionKey == conditionDockerDisk {
			legacy := parseDiskCondition(raw)
			unit := "%"
			if legacy.UseBytes {
				unit = legacy.UnitLabel
			}
			return ConditionConfig{
				Operator:  legacy.Comparison.Operator,
				Threshold: legacy.Comparison.Threshold,
				Unit:      unit,
			}
		}
		legacy := parseThresholdCondition(raw, decl.DefaultThreshold, true)
		return ConditionConfig{
			Operator:  legacy.Operator,
			Threshold: legacy.Threshold,
			Unit:      decl.Unit,
		}

	default:
		return ConditionConfig{}
	}
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

func parseThresholdCondition(raw string, defaultThreshold float64, percent bool) comparisonCondition {
	unit := ""
	if percent {
		unit = "%"
	}

	fallback := comparisonCondition{
		Operator:  ">",
		Threshold: defaultThreshold,
		Display:   formatComparison(">", defaultThreshold, unit),
	}

	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return fallback
	}

	match := comparisonConditionPattern.FindStringSubmatch(trimmed)
	if len(match) == 0 {
		return fallback
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
		Display:   formatComparison(operator, threshold, unit),
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
				Display:   formatComparison(">", threshold, "%"),
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
				Display:   formatComparison(operator, threshold, unitLabel),
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
			Display:   formatComparison(operator, threshold, "%"),
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

// parseExtendedDuration adds a day suffix to time.ParseDuration, which
// stops at hours.
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

// -----------------------------------------------------------------------------
// Shared helpers
// -----------------------------------------------------------------------------

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

func formatComparison(operator string, threshold float64, unit string) string {
	if operator == "" {
		operator = ">"
	}
	switch unit {
	case "":
		return operator + " " + formatFloat(threshold)
	case "%":
		// A percent sign binds to its number; a word does not.
		return operator + " " + formatFloat(threshold) + "%"
	default:
		return operator + " " + formatFloat(threshold) + " " + unit
	}
}

// formatFloat renders a measurement for a human.
//
// Counts are whole things — "5 attempts", not "5.0 attempts" — while a
// percentage or a load average earns its decimal. Trimming a trailing
// zero costs nothing and stops a threshold reading like a measurement
// that was precise to a tenth when the operator typed an integer.
func formatFloat(value float64) string {
	if value == math.Trunc(value) && math.Abs(value) < 1e15 {
		return strconv.FormatFloat(value, 'f', 0, 64)
	}
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

func containsString(items []string, value string) bool {
	for _, item := range items {
		if item == value {
			return true
		}
	}
	return false
}

// matchesAlertTarget reports whether any candidate matches the rule's
// target pattern list. An empty target (or "*") matches everything.
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
