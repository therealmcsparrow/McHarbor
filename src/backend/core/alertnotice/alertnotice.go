// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// Package alertnotice is the shape of one alert state change as a notification
// sink sees it, so a sink such as push can render it without importing alerts.
package alertnotice

// Notice is one alert state change as a notification sink sees it, after the
// rule's own destinations have been delivered to.
type Notice struct {
	AlertID       string
	Name          string
	Title         string
	Message       string
	Severity      string
	State         string
	SourceType    string
	EnvironmentID string
	SubjectID     string
	SubjectName   string
}

// SourceContainer is the Notice.SourceType of alerts whose subject is a
// container; it must equal the alerts module's built-in container source type.
const SourceContainer = "container"

// StateResolved is the Notice.State of an alert that stopped firing; it must
// equal the alerts module's EventStateResolved.
const StateResolved = "resolved"
