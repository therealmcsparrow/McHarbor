// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package alerts

import (
	"strings"
	"time"
)

// messageContext is everything a message — templated or built-in — can
// mention.
type messageContext struct {
	AlertName       string
	Severity        string
	State           string
	ConditionName   string
	ConditionRule   string
	EnvironmentName string
	SubjectName     string
	ValueLabel      string
	Threshold       string
	Duration        time.Duration
	Extra           map[string]string
	AlertID         string
	SubjectID       string
	EnvironmentID   string
	Operator        string
	Now             time.Time
}

// renderMessage produces the body delivered to every destination.
//
// An empty template renders the built-in sentence, so a rule created
// before templates existed reads exactly as it did before.
func renderMessage(template string, mc messageContext) string {
	trimmed := strings.TrimSpace(template)
	if trimmed == "" {
		return defaultMessage(mc)
	}
	return applyPlaceholders(trimmed, mc)
}

// applyPlaceholders does plain string substitution.
//
// Deliberately not text/template. A rule author is an administrator,
// but a template engine that can walk a struct turns a message field
// into a way to read host state, and nothing here needs conditionals
// or loops. An unknown placeholder is left as written rather than
// erroring — a typo in a message is not worth losing the alert over.
func applyPlaceholders(template string, mc messageContext) string {
	replacements := []string{
		"{{alert}}", mc.AlertName,
		"{{severity}}", mc.Severity,
		"{{state}}", mc.State,
		"{{condition}}", mc.ConditionName,
		"{{rule}}", mc.ConditionRule,
		"{{environment}}", mc.EnvironmentName,
		"{{subject}}", mc.SubjectName,
		"{{value}}", mc.ValueLabel,
		"{{threshold}}", mc.Threshold,
		"{{duration}}", formatOptionalDuration(mc.Duration),
		"{{alertId}}", mc.AlertID,
		"{{subjectId}}", mc.SubjectID,
		"{{environmentId}}", mc.EnvironmentID,
		"{{operator}}", mc.Operator,
	}
	if !mc.Now.IsZero() {
		now := mc.Now.UTC()
		replacements = append(replacements,
			"{{time}}", now.Format("15:04"),
			"{{date}}", now.Format("2006-01-02"),
			"{{timestamp}}", now.Format(time.RFC3339),
		)
	}

	out := strings.NewReplacer(replacements...).Replace(template)

	for key, value := range mc.Extra {
		out = strings.ReplaceAll(out, "{{ctx."+key+"}}", value)
	}

	return out
}

// defaultMessage builds the sentence used when a rule has no template.
func defaultMessage(mc messageContext) string {
	subject := strings.TrimSpace(mc.SubjectName)
	if subject == "" {
		subject = "unknown"
	}

	var builder strings.Builder
	builder.WriteString(strings.TrimSpace(mc.ConditionName))
	builder.WriteString(" on ")
	builder.WriteString(subject)

	if environment := strings.TrimSpace(mc.EnvironmentName); environment != "" {
		builder.WriteString(" in ")
		builder.WriteString(environment)
	}

	if mc.State == EventStateResolved {
		builder.WriteString(" has recovered")
		if mc.Duration > 0 {
			builder.WriteString(" after ")
			builder.WriteString(formatDuration(mc.Duration))
		}
		builder.WriteString(".")
		return builder.String()
	}

	if value := strings.TrimSpace(mc.ValueLabel); value != "" {
		builder.WriteString(" — ")
		builder.WriteString(value)
	}

	if rule := strings.TrimSpace(mc.ConditionRule); rule != "" {
		builder.WriteString(" (rule ")
		builder.WriteString(rule)
		builder.WriteString(")")
	}

	builder.WriteString(".")
	return builder.String()
}

// alertTitle is the one-line subject every channel shows above the
// body.
func alertTitle(alert Alert, state string) string {
	name := strings.TrimSpace(alert.Name)
	if name == "" {
		name = "Alert"
	}

	if state == EventStateResolved {
		return "Alert resolved: " + name
	}
	return "Alert triggered: " + name
}

// inAppLevel maps a rule severity onto the in-app notification levels.
func inAppLevel(severity string) string {
	switch normalizeSeverity(severity) {
	case "critical", "warning":
		return "warning"
	default:
		return "info"
	}
}

func formatOptionalDuration(value time.Duration) string {
	if value <= 0 {
		return ""
	}
	return formatDuration(value)
}
