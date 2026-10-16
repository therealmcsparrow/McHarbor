// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package alerts

import (
	"context"
	"sync/atomic"
	"time"

	"github.com/therealmcsparrow/mcharbor/core/plugin/specs/incident"
)

// deliveryInput is one alert transition as delivery needs it. StateKey is
// empty for a test send, which never opens a ticket.
type deliveryInput struct {
	Title           string
	Message         string
	State           string
	StateKey        string
	EnvironmentID   string
	EnvironmentName string
	SubjectID       string
	SubjectName     string
}

// IncidentEmitter is the ITSM side of delivery: a channel whose plugin can
// open tickets gets a lifecycle event instead of a message. Declared here so
// alerts never imports the module that implements it.
type IncidentEmitter interface {
	ChannelType(ctx context.Context, channelID string) (string, error)
	Supports(channelType string) bool
	Emit(ctx context.Context, channelID string, ev incident.Event) error
}

var incidentEmitter atomic.Pointer[IncidentEmitter]

// SetIncidentEmitter installs the ITSM emitter; nil removes it.
func SetIncidentEmitter(e IncidentEmitter) {
	if e == nil {
		incidentEmitter.Store(nil)
		return
	}
	incidentEmitter.Store(&e)
}

// emitIncident hands a transition to the ITSM outbox when the channel can take
// it. routed reports that it did, ok that the operation was queued.
func emitIncident(ctx context.Context, rule Alert, channelID string, in deliveryInput) (routed, ok bool) {
	p := incidentEmitter.Load()
	if p == nil || in.StateKey == "" {
		return false, false
	}
	em := *p
	channelType, err := em.ChannelType(ctx, channelID)
	if err != nil || !em.Supports(channelType) {
		return false, false
	}
	state := incident.StateFiring
	if in.State == EventStateResolved {
		state = incident.StateResolved
	}
	ev := incident.Event{
		DedupKey: in.StateKey, State: state, Severity: incident.Severity(normalizeSeverity(rule.Severity)),
		Title: in.Title, Message: in.Message, Source: rule.SourceType, AlertID: rule.ID, AlertName: rule.Name,
		EnvironmentID: in.EnvironmentID, EnvironmentName: in.EnvironmentName, SubjectID: in.SubjectID,
		SubjectName: in.SubjectName, Labels: map[string]string{"condition": rule.ConditionKey}, OccurredAt: time.Now().UTC(),
	}
	return true, em.Emit(ctx, channelID, ev) == nil
}
