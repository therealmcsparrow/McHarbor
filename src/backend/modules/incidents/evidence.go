// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package incidents

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"regexp"
	"strings"
	"time"

	"github.com/moby/moby/api/pkg/stdcopy"
	"github.com/moby/moby/client"

	"github.com/therealmcsparrow/mcharbor/core/redact"
)

// Bounds on what one bundle holds, so a noisy container cannot make the
// evidence — or the prompt built from it — unbounded.
const (
	maxEvidenceBytes = 64 << 10
	maxLogLines      = 150
	maxLogLineBytes  = 400
	maxEvents        = 120
	maxRows          = 40
	maxPoints        = 40
	lookBefore       = 30 * time.Minute
)

// DockerReader is the part of a Docker client evidence needs: read-only.
type DockerReader interface {
	ContainerInspect(ctx context.Context, containerID string, options client.ContainerInspectOptions) (client.ContainerInspectResult, error)
	ContainerLogs(ctx context.Context, containerID string, options client.ContainerLogsOptions) (client.ContainerLogsResult, error)
}

// Assembler builds evidence bundles from the database and, for logs and the
// container's current state, the environment itself.
type Assembler struct {
	DB        *sql.DB
	ReaderFor func(envID string) (DockerReader, error)
	Now       func() time.Time
}

// socketRegex catches Docker endpoints and socket paths, which the redaction
// shared with the assistant does not know about and must never leave.
var socketRegex = regexp.MustCompile(`(?i)(unix|npipe|tcp)://[^\s"']+|[^\s"']*docker\.sock\b`)

// Assemble gathers everything McHarbor knows about the container around the
// incident. Each source that cannot be read is left empty with the reason in
// place of the data; the bundle is never refused for one missing part.
func (a *Assembler) Assemble(ctx context.Context, in *Incident) (*Evidence, error) {
	now := time.Now
	if a.Now != nil {
		now = a.Now
	}
	first, err := time.Parse(time.RFC3339, in.FirstSignalAt)
	if err != nil {
		first = now()
	}
	start, end := first.Add(-lookBefore), now().UTC()
	if in.ResolvedAt != "" {
		if t, err := time.Parse(time.RFC3339, in.ResolvedAt); err == nil {
			end = t
		}
	}
	ev := &Evidence{
		WindowStart: start.UTC().Format(time.RFC3339), WindowEnd: end.Format(time.RFC3339),
		Container: ContainerFacts{Name: in.SubjectName, Stack: in.StackName},
		Events:    []EventItem{}, Logs: []string{}, ImageChanges: []ImageChange{}, Audit: []AuditItem{},
		Deploys: []DeployItem{}, Alerts: []AlertItem{}, HostCPU: []MetricPoint{}, HostMem: []MetricPoint{},
		ContainerCPU: []MetricPoint{}, ContainerMem: []MetricPoint{}, Drift: []DriftItem{},
	}
	s, e := ev.WindowStart, ev.WindowEnd
	if in.Source != "" {
		ev.Container = ContainerFacts{}
		a.pluginEvidence(ctx, in, start, end, ev)
		for _, step := range []func() error{
			func() error { return a.audit(ctx, in, start.Add(-90*time.Minute).UTC().Format(time.RFC3339), e, ev) },
			func() error { return a.alerts(ctx, in, s, e, ev) },
		} {
			if err := step(); err != nil {
				return nil, err
			}
		}
		return finish(ev)
	}
	steps := []func() error{
		func() error { return a.events(ctx, in, s, e, ev) },
		func() error {
			return a.imageChanges(ctx, in, start.Add(-24*time.Hour).UTC().Format(time.RFC3339), e, ev)
		},
		func() error { return a.audit(ctx, in, start.Add(-90*time.Minute).UTC().Format(time.RFC3339), e, ev) },
		func() error { return a.deploys(ctx, in, start.Add(-24*time.Hour).UTC().Format(time.RFC3339), e, ev) },
		func() error { return a.alerts(ctx, in, s, e, ev) },
		func() error { return a.metrics(ctx, in, s, e, ev) },
		func() error { return a.drift(ctx, in, ev) },
	}
	for _, step := range steps {
		if err := step(); err != nil {
			return nil, err
		}
	}
	a.live(ctx, in, ev)
	return finish(ev)
}

func (a *Assembler) events(ctx context.Context, in *Incident, start, end string, ev *Evidence) error {
	items, err := NewStore(a.DB).containerEvents(ctx, in.SubjectName, in.EnvID, start, end, maxEvents)
	ev.Events = append(ev.Events, items...)
	return err
}

func (a *Assembler) imageChanges(ctx context.Context, in *Incident, start, end string, ev *Evidence) error {
	creations, err := NewStore(a.DB).containerCreations(ctx, in.SubjectName, in.EnvID, start, end)
	if err != nil {
		return err
	}
	prev := ""
	for _, c := range creations {
		if c.image != "" && prev != "" && c.image != prev {
			ev.ImageChanges = append(ev.ImageChanges, ImageChange{At: c.at, From: prev, To: c.image})
		}
		if c.image != "" {
			prev = c.image
		}
	}
	return nil
}

func (a *Assembler) audit(ctx context.Context, in *Incident, start, end string, ev *Evidence) error {
	names := []any{in.SubjectName}
	if in.StackName != "" {
		names = append(names, in.StackName)
	}
	items, err := NewStore(a.DB).auditRows(ctx, names, in.EnvID, start, end, maxRows)
	ev.Audit = append(ev.Audit, items...)
	return err
}

func (a *Assembler) deploys(ctx context.Context, in *Incident, start, end string, ev *Evidence) error {
	if in.EnvID == "" {
		return nil
	}
	items, err := NewStore(a.DB).promotionsInto(ctx, in.EnvID, start, end, maxRows)
	ev.Deploys = append(ev.Deploys, items...)
	return err
}

func (a *Assembler) alerts(ctx context.Context, in *Incident, start, end string, ev *Evidence) error {
	items, err := NewStore(a.DB).alertEventItems(ctx, in.SubjectName, in.EnvID, start, end, maxRows)
	ev.Alerts = append(ev.Alerts, items...)
	return err
}

// metricsLayout is how the metrics collector stores sample times. It is not
// RFC3339, and the two do not compare as strings, so windows are converted.
const metricsLayout = "2006-01-02 15:04:05"

func toMetricsTime(rfc string) string {
	t, err := time.Parse(time.RFC3339, rfc)
	if err != nil {
		return rfc
	}
	return t.UTC().Format(metricsLayout)
}

func (a *Assembler) metrics(ctx context.Context, in *Incident, start, end string, ev *Evidence) error {
	var err error
	store := NewStore(a.DB)
	start, end = toMetricsTime(start), toMetricsTime(end)
	if in.EnvID != "" {
		if ev.HostCPU, err = store.metricSeries(ctx, seriesHostCPU, "", in.EnvID, start, end); err != nil {
			return fmt.Errorf("reading host cpu: %w", err)
		}
		if ev.HostMem, err = store.metricSeries(ctx, seriesHostMem, "", in.EnvID, start, end); err != nil {
			return fmt.Errorf("reading host memory: %w", err)
		}
	}
	if ev.ContainerCPU, err = store.metricSeries(ctx, seriesContainerCPU, in.SubjectName, in.EnvID, start, end); err != nil {
		return fmt.Errorf("reading container cpu: %w", err)
	}
	if ev.ContainerMem, err = store.metricSeries(ctx, seriesContainerMem, in.SubjectName, in.EnvID, start, end); err != nil {
		return fmt.Errorf("reading container memory: %w", err)
	}
	for _, s := range []*[]MetricPoint{&ev.HostCPU, &ev.HostMem, &ev.ContainerCPU, &ev.ContainerMem} {
		if *s == nil {
			*s = []MetricPoint{}
		}
	}
	return nil
}

func (a *Assembler) drift(ctx context.Context, in *Incident, ev *Evidence) error {
	items, err := NewStore(a.DB).driftItems(ctx, in.EnvID, in.SubjectName, in.StackName)
	ev.Drift = append(ev.Drift, items...)
	return err
}

// live reads the container's current state and its last log lines. The
// environment may be unreachable; that is recorded, not fatal.
func (a *Assembler) live(ctx context.Context, in *Incident, ev *Evidence) {
	if a.ReaderFor == nil {
		ev.LogsNote = "unavailable"
		return
	}
	cli, err := a.ReaderFor(in.EnvID)
	if err != nil {
		ev.LogsNote = "unreachable"
		return
	}
	cctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	infoRes, err := cli.ContainerInspect(cctx, in.SubjectName, client.ContainerInspectOptions{})
	if err != nil {
		ev.LogsNote = "not_found"
		return
	}
	info := infoRes.Container
	if info.Config != nil {
		ev.Container.Image = info.Config.Image
	}
	ev.Container.ImageID = shortID(info.Image)
	ev.Container.Restarts = info.RestartCount
	if info.State != nil {
		ev.Container.State = string(info.State.Status)
		code := info.State.ExitCode
		ev.Container.ExitCode = &code
		ev.Container.OOMKilled = info.State.OOMKilled
		if info.State.Health != nil {
			ev.Container.Health = string(info.State.Health.Status)
		}
	}
	rc, err := cli.ContainerLogs(cctx, in.SubjectName, client.ContainerLogsOptions{
		ShowStdout: true, ShowStderr: true, Timestamps: true, Tail: fmt.Sprint(maxLogLines),
	})
	if err != nil {
		ev.LogsNote = "logs_unavailable"
		return
	}
	defer func() {
		if cerr := rc.Close(); cerr != nil {
			slog.Warn("incidents: closing container log stream failed", "error", cerr)
		}
	}()
	raw, err := io.ReadAll(io.LimitReader(rc, 1<<20))
	if err != nil {
		ev.LogsNote = "logs_unavailable"
		return
	}
	text := raw
	if info.Config == nil || !info.Config.Tty {
		var out bytes.Buffer
		if _, err := stdcopy.StdCopy(&out, &out, bytes.NewReader(raw)); err == nil {
			text = out.Bytes()
		}
	}
	for _, line := range strings.Split(strings.TrimRight(string(text), "\n"), "\n") {
		if line == "" {
			continue
		}
		if len(line) > maxLogLineBytes {
			line = line[:maxLogLineBytes] + "…"
		}
		ev.Logs = append(ev.Logs, line)
	}
}

// eventAttrs decodes a lifecycle event's attributes; a malformed blob costs
// only that event's details, never the bundle.
func eventAttrs(meta string) map[string]string {
	attrs := map[string]string{}
	if meta == "" {
		return attrs
	}
	if err := json.Unmarshal([]byte(meta), &attrs); err != nil {
		return map[string]string{}
	}
	return attrs
}

func shortID(id string) string {
	id = strings.TrimPrefix(id, "sha256:")
	if len(id) > 12 {
		return id[:12]
	}
	return id
}

// downsample keeps at most n evenly spaced points, always keeping the last.
func downsample(pts []MetricPoint, n int) []MetricPoint {
	if len(pts) <= n {
		return pts
	}
	out := make([]MetricPoint, 0, n)
	step := float64(len(pts)-1) / float64(n-1)
	for i := 0; i < n; i++ {
		out = append(out, pts[int(float64(i)*step+0.5)])
	}
	return out
}

// Redact removes what must never reach a model: the assistant's secret and
// credential patterns, emails and addresses, and Docker endpoints. It walks
// the bundle as generic JSON so no field can be forgotten.
func Redact(ev *Evidence) (*Evidence, error) {
	raw, err := json.Marshal(ev)
	if err != nil {
		return nil, fmt.Errorf("encoding evidence: %w", err)
	}
	var generic any
	if err := json.Unmarshal(raw, &generic); err != nil {
		return nil, fmt.Errorf("decoding evidence: %w", err)
	}
	// Hardened patterns first: the standard profile masks any long run of
	// base64, which would cut a JWT's signature out and leave its header and
	// claims in a shape the JWT pattern no longer recognises.
	generic = redact.WideHardened(generic)
	generic = redact.Value(generic, redact.Standard)
	generic = walkStrings(generic, func(s string) string { return socketRegex.ReplaceAllString(s, "[REDACTED:endpoint]") })
	raw, err = json.Marshal(generic)
	if err != nil {
		return nil, fmt.Errorf("encoding redacted evidence: %w", err)
	}
	var out Evidence
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, fmt.Errorf("decoding redacted evidence: %w", err)
	}
	return &out, nil
}

func walkStrings(v any, fn func(string) string) any {
	switch t := v.(type) {
	case string:
		return fn(t)
	case []any:
		for i := range t {
			t[i] = walkStrings(t[i], fn)
		}
		return t
	case map[string]any:
		for k, val := range t {
			t[k] = walkStrings(val, fn)
		}
		return t
	}
	return v
}

// finish redacts the bundle and trims it to the size ceiling, dropping the
// oldest log lines and events first because the failure is at the end.
func finish(ev *Evidence) (*Evidence, error) {
	out, err := Redact(ev)
	if err != nil {
		return nil, err
	}
	for i := 0; i < 200; i++ {
		raw, err := json.Marshal(out)
		if err != nil {
			return nil, fmt.Errorf("encoding evidence: %w", err)
		}
		if len(raw) <= maxEvidenceBytes {
			return out, nil
		}
		out.Truncated = true
		switch {
		case len(out.Logs) > 20:
			out.Logs = out.Logs[len(out.Logs)/4:]
		case len(out.Events) > 20:
			out.Events = out.Events[len(out.Events)/4:]
		case len(out.Audit) > 10:
			out.Audit = out.Audit[len(out.Audit)/4:]
		default:
			out.ContainerCPU, out.ContainerMem, out.HostCPU, out.HostMem = nil, nil, nil, nil
			if len(out.Logs) > 5 {
				out.Logs = out.Logs[len(out.Logs)/2:]
			}
		}
	}
	return out, nil
}
