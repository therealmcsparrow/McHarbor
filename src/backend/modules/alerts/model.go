// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package alerts

import "strings"

// Alert represents an alert rule.
//
// A rule is (source, condition) + how to compare it + what to match +
// where to send it. SourceType and ConditionKey resolve to a
// declaration registered by a built-in source or by a plugin; the
// engine never switches on them itself.
type Alert struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Severity string `json:"severity"`

	// SourceType and ConditionKey identify what this rule watches.
	SourceType   string `json:"sourceType"`
	ConditionKey string `json:"conditionKey"`

	// Condition is the structured threshold/duration/window config.
	Condition ConditionConfig `json:"condition"`

	// Target is a container/environment name or id pattern, or "*"
	// for all. Ignored by host-wide conditions.
	Target string `json:"target"`

	// EnvironmentIDs restricts the rule. Empty means every active
	// environment.
	EnvironmentIDs []string `json:"environmentIds"`

	// Destinations are the communication channels this rule notifies.
	Destinations []AlertDestination `json:"destinations"`
	SendInApp    bool               `json:"sendInApp"`

	CooldownSeconds  int    `json:"cooldownSeconds"`
	NotifyOnRecovery bool   `json:"notifyOnRecovery"`
	MessageTemplate  string `json:"messageTemplate"`

	Enabled   bool   `json:"enabled"`
	CreatedAt string `json:"createdAt"`
	UpdatedAt string `json:"updatedAt"`

	// SourceAvailable reports whether SourceType currently resolves
	// to a registered source. False means the owning plugin is gone;
	// the rule is kept and shown as unavailable rather than deleted,
	// because reinstalling the plugin must bring the rule back.
	SourceAvailable bool `json:"sourceAvailable"`

	// legacyCondition carries the pre-migration free-text condition
	// so the service can backfill Condition on first read. Never
	// serialised — it exists to be consumed and forgotten.
	legacyCondition string
}

// ConditionConfig is the structured replacement for the old free-text
// condition string. Which fields matter depends on the condition's
// declared kind: threshold conditions use Operator/Threshold, duration
// conditions use ForSeconds, count-in-window uses both plus
// WindowSeconds.
type ConditionConfig struct {
	Operator  string  `json:"operator,omitempty"`
	Threshold float64 `json:"threshold,omitempty"`
	Unit      string  `json:"unit,omitempty"`

	// ForSeconds is how long the condition must hold before the rule
	// notifies. For duration-kind conditions this *is* the threshold.
	ForSeconds int `json:"forSeconds,omitempty"`

	// WindowSeconds is the measurement window for count-in-window
	// conditions ("5 restarts in 10 minutes").
	WindowSeconds int `json:"windowSeconds,omitempty"`

	// Params carries values for the condition's declared extra
	// fields, keyed by ConfigFieldDecl.Key.
	Params map[string]string `json:"params,omitempty"`
}

// AlertDestination is one communication channel a rule notifies.
type AlertDestination struct {
	ID        string `json:"id"`
	ChannelID string `json:"channelId"`

	// MinSeverity, when set, delivers only when the rule's severity
	// ranks at or above it.
	MinSeverity string `json:"minSeverity,omitempty"`

	// ChannelName and ChannelType are joined in for display. Empty
	// ChannelName means the channel no longer exists — the row is
	// kept so the operator can see what broke.
	ChannelName string `json:"channelName,omitempty"`
	ChannelType string `json:"channelType,omitempty"`
}

// DestinationInput is one destination as submitted by the client.
type DestinationInput struct {
	ChannelID   string `json:"channelId"`
	MinSeverity string `json:"minSeverity"`
}

// CreateAlertInput is the request body for creating an alert rule.
type CreateAlertInput struct {
	Name             string             `json:"name"`
	Severity         string             `json:"severity"`
	SourceType       string             `json:"sourceType"`
	ConditionKey     string             `json:"conditionKey"`
	Condition        ConditionConfig    `json:"condition"`
	Target           string             `json:"target"`
	EnvironmentIDs   []string           `json:"environmentIds"`
	Destinations     []DestinationInput `json:"destinations"`
	SendInApp        bool               `json:"sendInApp"`
	CooldownSeconds  int                `json:"cooldownSeconds"`
	NotifyOnRecovery *bool              `json:"notifyOnRecovery"`
	MessageTemplate  string             `json:"messageTemplate"`
}

// UpdateAlertInput is the request body for updating an alert rule.
// Pointer fields allow partial updates; a nil Destinations slice
// leaves the existing destinations alone, while an empty non-nil one
// clears them.
type UpdateAlertInput struct {
	Name             *string             `json:"name"`
	Severity         *string             `json:"severity"`
	SourceType       *string             `json:"sourceType"`
	ConditionKey     *string             `json:"conditionKey"`
	Condition        *ConditionConfig    `json:"condition"`
	Target           *string             `json:"target"`
	EnvironmentIDs   *[]string           `json:"environmentIds"`
	Destinations     *[]DestinationInput `json:"destinations"`
	SendInApp        *bool               `json:"sendInApp"`
	CooldownSeconds  *int                `json:"cooldownSeconds"`
	NotifyOnRecovery *bool               `json:"notifyOnRecovery"`
	MessageTemplate  *string             `json:"messageTemplate"`
	Enabled          *bool               `json:"enabled"`
}

// AlertEvent is one historical firing or recovery.
type AlertEvent struct {
	ID              string          `json:"id"`
	AlertID         string          `json:"alertId"`
	AlertName       string          `json:"alertName"`
	SourceType      string          `json:"sourceType"`
	ConditionKey    string          `json:"conditionKey"`
	EnvironmentID   string          `json:"environmentId,omitempty"`
	EnvironmentName string          `json:"environmentName,omitempty"`
	SubjectID       string          `json:"subjectId,omitempty"`
	SubjectName     string          `json:"subjectName,omitempty"`
	State           string          `json:"state"`
	Severity        string          `json:"severity"`
	Value           float64         `json:"value"`
	ValueLabel      string          `json:"valueLabel,omitempty"`
	Message         string          `json:"message"`
	Deliveries      []EventDelivery `json:"deliveries"`
	CreatedAt       string          `json:"createdAt"`
}

// EventDelivery records one channel's outcome for one event.
//
// ErrorCode is a code, never a provider error string: a failing
// webhook can echo its own URL — token included — back in the body.
type EventDelivery struct {
	ChannelID string `json:"channelId"`
	OK        bool   `json:"ok"`
	ErrorCode string `json:"errorCode,omitempty"`
	// Mode is "incident" when the channel took a ticket operation instead
	// of a message; OK then means the operation was queued.
	Mode string `json:"mode,omitempty"`
}

// Event states.
const (
	EventStateFiring   = "firing"
	EventStateResolved = "resolved"
)

// ConditionCatalog is the API shape of everything that can be alerted
// on: one entry per registered source.
//
// The spec types in core/plugin/specs/alerting carry no JSON tags —
// they are a Go contract, not a wire format — so the catalog is a
// deliberate DTO rather than a re-serialisation of the spec.
type ConditionCatalog struct {
	Sources []CatalogSource `json:"sources"`
}

// CatalogSource is one registered alert source and its conditions.
type CatalogSource struct {
	SourceType  string             `json:"sourceType"`
	DisplayName string             `json:"displayName"`
	Description string             `json:"description,omitempty"`
	Slug        string             `json:"slug,omitempty"`
	Icon        string             `json:"icon,omitempty"`
	BuiltIn     bool               `json:"builtIn"`
	Conditions  []CatalogCondition `json:"conditions"`
}

// CatalogCondition is one thing that can be alerted on.
type CatalogCondition struct {
	Key              string             `json:"key"`
	DisplayName      string             `json:"displayName"`
	Description      string             `json:"description,omitempty"`
	Kind             string             `json:"kind"`
	Unit             string             `json:"unit,omitempty"`
	Operators        []string           `json:"operators"`
	DefaultOperator  string             `json:"defaultOperator,omitempty"`
	DefaultThreshold float64            `json:"defaultThreshold,omitempty"`
	SupportsFor      bool               `json:"supportsFor"`
	TargetKind       string             `json:"targetKind"`
	Fields           []CatalogFieldDecl `json:"fields,omitempty"`
	// I18nKey is set for built-in conditions so the frontend can
	// translate them; plugin conditions ship their own localised
	// DisplayName and leave it empty.
	I18nKey string `json:"i18nKey,omitempty"`
}

// CatalogFieldDecl is one extra per-rule parameter a condition takes.
type CatalogFieldDecl struct {
	Key         string `json:"key"`
	Type        string `json:"type"`
	Label       string `json:"label"`
	Required    bool   `json:"required,omitempty"`
	Help        string `json:"help,omitempty"`
	Placeholder string `json:"placeholder,omitempty"`
}

var validAlertSeverities = map[string]bool{
	"critical": true,
	"warning":  true,
	"info":     true,
}

// severityRank orders severities for min-severity destination
// filtering. Unknown values rank lowest so a malformed value can only
// ever deliver more, never silently swallow an alert.
func severityRank(severity string) int {
	switch strings.ToLower(strings.TrimSpace(severity)) {
	case "critical":
		return 3
	case "warning":
		return 2
	case "info":
		return 1
	default:
		return 0
	}
}

// meetsMinSeverity reports whether a rule at ruleSeverity should be
// delivered to a destination requiring minSeverity.
func meetsMinSeverity(ruleSeverity, minSeverity string) bool {
	if strings.TrimSpace(minSeverity) == "" {
		return true
	}
	return severityRank(ruleSeverity) >= severityRank(minSeverity)
}
