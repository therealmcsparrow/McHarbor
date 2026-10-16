// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package kubeapi

import (
	"fmt"
	"strings"
	"time"
)

type logOptions struct {
	container  string
	previous   bool
	tail       int
	limitBytes int
	timestamps bool
}

type apiError struct {
	code    int
	reason  string
	message string
}

func (e *apiError) Error() string { return e.message }

func badRequest(msg string) *apiError {
	return &apiError{code: 400, reason: "BadRequest", message: msg}
}

type logLine struct {
	at   time.Time
	text string
}

func (s *Sim) podLogs(c *cluster, p *pod, opt logOptions, now time.Time) (string, *apiError) {
	var ct *container
	if opt.container == "" {
		if len(p.containers) > 1 {
			names := make([]string, 0, len(p.containers))
			for _, x := range p.containers {
				names = append(names, x.name)
			}
			return "", badRequest(fmt.Sprintf("a container name must be specified for pod %s, choose one of: [%s]", p.name, strings.Join(names, " ")))
		}
		ct = &p.containers[0]
	} else {
		for i := range p.containers {
			if p.containers[i].name == opt.container {
				ct = &p.containers[i]
			}
		}
		if ct == nil {
			return "", badRequest(fmt.Sprintf("container %s is not valid for pod %s", opt.container, p.name))
		}
	}
	primary := ct.name == p.containers[0].name
	st := s.stateOf(c, p, now)
	switch {
	case p.node == "":
		return "", badRequest(fmt.Sprintf("container %q in pod %q is waiting to start: pod is not scheduled", ct.name, p.name))
	case st.waiting == "ContainerCreating":
		return "", badRequest(fmt.Sprintf("container %q in pod %q is waiting to start: ContainerCreating", ct.name, p.name))
	case st.waiting == "ImagePullBackOff" && primary:
		return "", badRequest(fmt.Sprintf("container %q in pod %q is waiting to start: trying and failing to pull image", ct.name, p.name))
	}
	crash := primary && p.fault == "crashloop"
	if opt.previous && (!primary || s.restartsAt(p, now) <= 0) {
		return "", badRequest(fmt.Sprintf("previous terminated container %q in pod %q not found", ct.name, p.name))
	}
	lines := s.logLines(p, ct, opt.previous, crash, now)
	if opt.tail >= 0 && opt.tail < len(lines) {
		lines = lines[len(lines)-opt.tail:]
	}
	var b strings.Builder
	for _, l := range lines {
		if opt.timestamps {
			b.WriteString(l.at.UTC().Format(time.RFC3339Nano))
			b.WriteByte(' ')
		}
		b.WriteString(l.text)
		b.WriteByte('\n')
	}
	out := b.String()
	if opt.limitBytes > 0 && len(out) > opt.limitBytes {
		out = out[:opt.limitBytes]
	}
	return out, nil
}

func (s *Sim) logLines(p *pod, ct *container, previous, crash bool, now time.Time) []logLine {
	seed := uint64(0)
	for _, b := range []byte(hashHex(16, p.uid, ct.name, fmt.Sprint(previous))) {
		seed = seed*31 + uint64(b)
	}
	r := newRNG(seed)
	count := 240
	start := p.readyAt
	end := now.Add(-time.Duration(r.between(1, 8)) * time.Second)
	if p.fault == "completed" || p.fault == "failed" {
		count = 14
		end = maxTime(p.owner.completed, start.Add(time.Second))
	}
	if crash {
		count = 36
		ref := crashRef(p, s.base)
		last := ref.Add(time.Duration(now.Sub(ref)/crashPeriod) * crashPeriod)
		end = last
		start = last.Add(-4 * time.Second)
	} else if previous {
		count = 120
		end = now.Add(-12 * time.Minute)
		start = end.Add(-6 * time.Hour)
	}
	if !end.After(start) {
		start = end.Add(-time.Minute)
	}
	if steady := !previous && !crash && p.fault != "completed" && p.fault != "failed"; steady && end.Sub(start) < time.Duration(count)*2*time.Second {
		count = max(3, int(end.Sub(start)/(2*time.Second)))
	}
	step := end.Sub(start) / time.Duration(count)
	if step > 45*time.Second && !previous && !crash && p.fault != "completed" && p.fault != "failed" {
		step = time.Duration(r.between(5, 45)) * time.Second
		start = end.Add(-step * time.Duration(count))
	}
	out := make([]logLine, 0, count)
	for i := 0; i < count; i++ {
		at := start.Add(step * time.Duration(i)).Add(time.Duration(r.intn(900)) * time.Millisecond)
		out = append(out, logLine{at: at, text: renderLine(r, ct, p, at, i, count, crash, previous)})
	}
	return out
}

var (
	appPaths   = []string{"/api/v1/items", "/api/v1/orders", "/healthz", "/api/v1/users/me", "/api/v1/search", "/metrics", "/api/v1/cart", "/readyz"}
	appMsgs    = []string{"request completed", "cache hit", "cache miss, loading from store", "published event", "consumed batch", "flushed metrics", "token refreshed"}
	klogMsgs   = []string{"\"Reconciling\" controller=\"%s\"", "\"Successfully synced\" key=\"%s\"", "\"Starting workers\" count=4", "\"Watch close\" resource=\"*v1.Pod\" total=%d", "\"Lease renewed\" identity=\"%s\""}
	userAgents = []string{"Mozilla/5.0 (Macintosh; Intel Mac OS X 14_5) AppleWebKit/605.1.15", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) Chrome/128.0", "kube-probe/1.30", "curl/8.8.0"}
)

func renderLine(r *rng, ct *container, p *pod, at time.Time, i, count int, crash, previous bool) string {
	if crash && i >= count-4 {
		switch count - i {
		case 4:
			return fmt.Sprintf(`{"level":"info","ts":"%s","msg":"connecting to database","host":"postgres.%s.svc.cluster.local:5432"}`, at.UTC().Format(time.RFC3339Nano), p.ns)
		case 3:
			return fmt.Sprintf(`{"level":"error","ts":"%s","msg":"database connection failed","error":"dial tcp 10.96.%d.%d:5432: connect: connection refused","attempt":5}`, at.UTC().Format(time.RFC3339Nano), r.between(1, 254), r.between(2, 254))
		case 2:
			return fmt.Sprintf(`{"level":"fatal","ts":"%s","msg":"startup failed, exiting","error":"exhausted retries connecting to database"}`, at.UTC().Format(time.RFC3339Nano))
		default:
			return "exit status 1"
		}
	}
	if previous && p.fault == "oom" && i == count-1 {
		return fmt.Sprintf(`{"level":"warn","ts":"%s","msg":"join window buffer above soft limit","buffered_mb":%d}`, at.UTC().Format(time.RFC3339Nano), r.between(3800, 4090))
	}
	if i == 0 {
		return fmt.Sprintf(`{"level":"info","ts":"%s","msg":"starting %s","image":"%s","pod":"%s"}`, at.UTC().Format(time.RFC3339Nano), ct.name, ct.image, p.name)
	}
	switch ct.style {
	case "nginx":
		status := pick(r, []int{200, 200, 200, 200, 200, 301, 304, 404, 200, 502})
		return fmt.Sprintf(`10.%d.%d.%d - - [%s] "%s %s HTTP/1.1" %d %d "-" "%s" %.3f`,
			r.between(40, 46), r.between(1, 12), r.between(2, 250), at.UTC().Format("02/Jan/2006:15:04:05 -0700"),
			pick(r, []string{"GET", "GET", "GET", "POST"}), pick(r, appPaths), status, r.between(120, 48000), pick(r, userAgents), float64(r.between(1, 900))/1000)
	case "postgres":
		return fmt.Sprintf("%s UTC [%d] LOG:  %s", at.UTC().Format("2006-01-02 15:04:05.000"), r.between(60, 4000), pick(r, []string{
			"checkpoint starting: time", "checkpoint complete: wrote 412 buffers (2.5%); 0 WAL file(s) added, 0 removed, 1 recycled",
			"automatic vacuum of table \"app.public.events\": index scans: 1", "connection received: host=10.42.3.17 port=51234",
			"connection authorized: user=app database=app application_name=api", "disconnection: session time: 0:00:04.211 user=app database=app"}))
	case "redis":
		return fmt.Sprintf("1:M %s %s", at.UTC().Format("02 Jan 2006 15:04:05.000"), pick(r, []string{
			"* 100 changes in 300 seconds. Saving...", "* Background saving started by pid 412", "* DB saved on disk", "* Background saving terminated with success",
			"* Fork CoW for RDB: current 2 MB, peak 2 MB, average 1 MB"}))
	case "envoy":
		return fmt.Sprintf(`[%s] "%s %s HTTP/1.1" 200 - 0 %d %d %d "-" "%s" "%s" "127.0.0.1:8080"`, at.UTC().Format("2006-01-02T15:04:05.000Z"),
			pick(r, []string{"GET", "POST"}), pick(r, appPaths), r.between(0, 9000), r.between(1, 80), r.between(1, 70), pick(r, userAgents), uidFor(p.uid, fmt.Sprint(i)))
	case "infra":
		msg := pick(r, klogMsgs)
		if strings.Contains(msg, "%d") {
			msg = fmt.Sprintf(msg, r.between(10, 900))
		} else if strings.Contains(msg, "%s") {
			msg = fmt.Sprintf(msg, p.ns+"/"+p.owner.name)
		}
		return fmt.Sprintf("%s%s %7d %s:%d] %s", pick(r, []string{"I", "I", "I", "I", "W"}), at.UTC().Format("0102 15:04:05.000000"), 1,
			pick(r, []string{"controller.go", "reflector.go", "leaderelection.go", "server.go"}), r.between(40, 900), msg)
	case "job":
		steps := []string{"starting run", "acquired lock", "processing batch", "processing batch", "processing batch", "uploading results", "released lock", "run finished"}
		msg := steps[i%len(steps)]
		if p.fault == "failed" && i == count-1 {
			return fmt.Sprintf(`{"level":"error","ts":"%s","msg":"verification failed","error":"checksum mismatch on snapshot 2026-09-26T03:30Z"}`, at.UTC().Format(time.RFC3339Nano))
		}
		return fmt.Sprintf(`{"level":"info","ts":"%s","msg":"%s","batch":%d}`, at.UTC().Format(time.RFC3339Nano), msg, i)
	}
	level := pick(r, []string{"info", "info", "info", "info", "info", "info", "debug", "warn"})
	status := 200
	if level == "warn" {
		status = pick(r, []int{429, 499, 503})
	}
	return fmt.Sprintf(`{"level":"%s","ts":"%s","caller":"%s","msg":"%s","method":"%s","path":"%s","status":%d,"duration_ms":%d,"request_id":"%s"}`,
		level, at.UTC().Format(time.RFC3339Nano), pick(r, []string{"server/http.go:142", "store/cache.go:88", "events/bus.go:57", "worker/loop.go:201"}),
		pick(r, appMsgs), pick(r, []string{"GET", "GET", "POST", "PUT"}), pick(r, appPaths), status, r.between(1, 480), hashHex(16, p.uid, fmt.Sprint(i)))
}
