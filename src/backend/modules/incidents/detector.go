// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package incidents

import (
	"context"
	"database/sql"
	"log/slog"
	"time"
)

// Thresholds a signal must cross. One non-zero exit is a restart; a pattern of
// them is an incident.
const (
	crashLoopExits   = 3
	crashLoopWindow  = 10 * time.Minute
	repeatExits      = 2
	repeatExitWindow = 30 * time.Minute
	quietToResolve   = 30 * time.Minute
)

var signalStrength = map[string]int{
	SignalExitRepeat: 1, SignalAlert: 2, SignalUnhealthy: 3, SignalOOM: 4, SignalCrashLoop: 5,
}

// Detector turns stored events into incidents. It reads what the activity
// collector and the alert engine already wrote, keyed on insertion time,
// because the collector polls and may store an event minutes after it happened.
type Detector struct {
	DB     *sql.DB
	Store  *Store
	Now    func() time.Time
	Logger *slog.Logger
	cursor time.Time
}

func (d *Detector) warn(msg string, args ...any) {
	if d.Logger != nil {
		d.Logger.Warn(msg, args...)
	}
}

// Detection is one signal about one container found in a scan.
type Detection struct {
	EnvID, Subject, Stack, Signal, AlertEventID string
	Count                                       int
	At                                          string
}

// ScanResult says which incidents a scan opened and which it escalated.
type ScanResult struct {
	Opened    []*Incident
	Escalated []*Incident
}

func (d *Detector) now() time.Time {
	if d.Now != nil {
		return d.Now()
	}
	return time.Now()
}

// Scan reads events stored since the last scan and opens or extends incidents.
func (d *Detector) Scan(ctx context.Context) (*ScanResult, error) {
	now := d.now().UTC()
	since := d.cursor
	if since.IsZero() {
		since = now.Add(-crashLoopWindow)
	}
	found, err := d.containerSignals(ctx, since, now)
	if err != nil {
		return nil, err
	}
	alerts, err := d.alertSignals(ctx, since)
	if err != nil {
		return nil, err
	}
	res := &ScanResult{}
	for _, det := range append(found, alerts...) {
		if err := d.apply(ctx, det, res); err != nil {
			return nil, err
		}
	}
	for _, det := range d.pluginSignals(ctx, since) {
		if err := d.applyPlugin(ctx, det, res); err != nil {
			return nil, err
		}
	}
	d.cursor = now
	return res, nil
}

func (d *Detector) apply(ctx context.Context, det Detection, res *ScanResult) error {
	open, err := d.Store.OpenFor(ctx, det.EnvID, det.Subject)
	if err != nil {
		return err
	}
	if open == nil {
		in, err := d.Store.Open(ctx, Incident{
			EnvID: det.EnvID, SubjectType: "container", SubjectName: det.Subject, StackName: det.Stack,
			Signal: det.Signal, SignalCount: det.Count, FirstSignalAt: det.At, LastSignalAt: det.At, AlertEventID: det.AlertEventID,
		})
		if err != nil {
			return err
		}
		res.Opened = append(res.Opened, in)
		return nil
	}
	signal := open.Signal
	escalated := signalStrength[det.Signal] > signalStrength[signal]
	if escalated {
		signal = det.Signal
	}
	if err := d.Store.Extend(ctx, open.ID, signal, det.Count, det.At, det.AlertEventID); err != nil {
		return err
	}
	if escalated {
		open.Signal = signal
		res.Escalated = append(res.Escalated, open)
	}
	return nil
}

type containerKey struct{ env, name string }

// containerSignals classifies each container that logged a failure-shaped event
// since the cursor, looking back over the crash-loop windows to count.
func (d *Detector) containerSignals(ctx context.Context, since, now time.Time) ([]Detection, error) {
	events, err := NewStore(d.DB).failureEvents(ctx, since.Add(-repeatExitWindow).Format(time.RFC3339), now.Add(-repeatExitWindow).Format(time.RFC3339))
	if err != nil {
		return nil, err
	}
	type tally struct {
		stack                                 string
		exits10, exits30, newExits, oom, sick int
		last                                  string
	}
	byContainer := map[containerKey]*tally{}
	order := []containerKey{}
	for _, ev := range events {
		env, name, action, at, meta, created := ev.env, ev.name, ev.action, ev.at, ev.meta, ev.created
		if name == "" {
			continue
		}
		k := containerKey{env, name}
		t := byContainer[k]
		if t == nil {
			t = &tally{}
			byContainer[k] = t
			order = append(order, k)
		}
		attrs := eventAttrs(meta)
		if p := attrs["com.docker.compose.project"]; p != "" {
			t.stack = p
		}
		isNew := created >= since.Format(time.RFC3339)
		when, perr := time.Parse(time.RFC3339, at)
		switch action {
		case "oom":
			if isNew {
				t.oom++
			}
		case "die":
			if attrs["exitCode"] == "0" || attrs["exitCode"] == "" {
				continue
			}
			t.exits30++
			if perr == nil && now.Sub(when) <= crashLoopWindow {
				t.exits10++
			}
			if isNew {
				t.newExits++
			}
		default:
			if isNew {
				t.sick++
			}
		}
		if at > t.last {
			t.last = at
		}
	}
	var out []Detection
	for _, k := range order {
		t := byContainer[k]
		det := Detection{EnvID: k.env, Subject: k.name, Stack: t.stack, At: t.last}
		switch {
		case t.newExits > 0 && t.exits10 >= crashLoopExits:
			det.Signal, det.Count = SignalCrashLoop, t.newExits
		case t.oom > 0:
			det.Signal, det.Count = SignalOOM, t.oom
		case t.sick > 0:
			det.Signal, det.Count = SignalUnhealthy, t.sick
		case t.newExits > 0 && t.exits30 >= repeatExits:
			det.Signal, det.Count = SignalExitRepeat, t.newExits
		default:
			continue
		}
		if det.At == "" {
			det.At = now.Format(time.RFC3339)
		}
		out = append(out, det)
	}
	return out, nil
}

func (d *Detector) alertSignals(ctx context.Context, since time.Time) ([]Detection, error) {
	return NewStore(d.DB).firingContainerAlerts(ctx, since.Format(time.RFC3339))
}

// Settled reports whether a quiet incident's container is actually fine. A
// container that died and stayed down is quiet too, and that incident stays open.
func (d *Detector) Settled(ctx context.Context, in *Incident) (bool, error) {
	if in.Source != "" {
		return d.pluginSettled(ctx, in)
	}
	action, meta, found, err := NewStore(d.DB).lastContainerEvent(ctx, in.SubjectName, in.EnvID, in.FirstSignalAt)
	if err != nil {
		return false, err
	}
	if !found {
		return true, nil
	}
	switch action {
	case "oom", "health_status: unhealthy":
		return false, nil
	case "die":
		code := eventAttrs(meta)["exitCode"]
		return code == "0" || code == "", nil
	}
	return true, nil
}
