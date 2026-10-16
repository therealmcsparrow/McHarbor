// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package swarmsim

import (
	"encoding/binary"
	"net/http"
	"sort"
	"strconv"
	"time"

	"github.com/moby/moby/api/types/swarm"

	"github.com/therealmcsparrow/mcharbor/core/demo/estate"
)

type logSource struct {
	ctr     estate.Container
	details string
}

type taggedLine struct {
	estate.LogLine
	details string
}

// sources snapshots the tasks whose containers have produced output; a
// rejected or still-pending task never started, so it has nothing to say.
func (c *cluster) sources(svcID, taskID string) []logSource {
	var out []logSource
	for _, t := range c.tasks {
		if (svcID != "" && t.ServiceID != svcID) || (taskID != "" && t.ID != taskID) || t.Status.ContainerStatus == nil {
			continue
		}
		ctr := estate.Container{ID: t.ID, Profile: t.profile, State: estate.StateRunning}
		if t.Status.State != swarm.TaskStateRunning {
			ctr.State = estate.StateExited
			ctr.FinishedAt = t.Status.Timestamp
			ctr.Error = t.Status.Err
		}
		out = append(out, logSource{ctr: ctr, details: "com.docker.swarm.node.id=" + t.NodeID + ",com.docker.swarm.service.id=" + t.ServiceID + ",com.docker.swarm.task.id=" + t.ID})
	}
	return out
}

func (b *backend) serviceLogs(w http.ResponseWriter, r *http.Request, ref string) {
	now, unlock := b.lock()
	svc := b.c.findService(ref)
	if svc == nil {
		unlock()
		writeError(w, http.StatusNotFound, "service "+ref+" not found")
		return
	}
	id := svc.ID
	srcs := b.c.sources(id, "")
	unlock()
	b.streamLogs(w, r, now, srcs, func() []logSource {
		_, unlock := b.lock()
		defer unlock()
		return b.c.sources(id, "")
	})
}

func (b *backend) taskLogs(w http.ResponseWriter, r *http.Request, ref string) {
	now, unlock := b.lock()
	t := b.c.findTask(ref)
	if t == nil {
		unlock()
		writeError(w, http.StatusNotFound, "task "+ref+" not found")
		return
	}
	id := t.ID
	srcs := b.c.sources("", id)
	unlock()
	b.streamLogs(w, r, now, srcs, func() []logSource {
		_, unlock := b.lock()
		defer unlock()
		return b.c.sources("", id)
	})
}

// streamLogs writes the merged task output in the multiplexed stream format,
// honouring tail, since, timestamps and details, and keeps writing while
// following until the client goes away.
func (b *backend) streamLogs(w http.ResponseWriter, r *http.Request, now time.Time, srcs []logSource, refresh func() []logSource) {
	q := r.URL.Query()
	tail := 1000
	if t := q.Get("tail"); t != "" && t != "all" {
		if n, err := strconv.Atoi(t); err == nil && n >= 0 {
			tail = n
		}
	}
	var since time.Time
	if s := q.Get("since"); s != "" {
		if f, err := strconv.ParseFloat(s, 64); err == nil && f > 0 {
			since = time.Unix(0, int64(f*1e9))
		}
	}
	stamps := queryBool(r, "timestamps")
	details := queryBool(r, "details")
	wantOut := q.Get("stdout") == "" || queryBool(r, "stdout")
	wantErr := q.Get("stderr") == "" || queryBool(r, "stderr")

	w.Header().Set("Content-Type", "application/vnd.docker.multiplexed-stream")
	w.WriteHeader(http.StatusOK)
	write := func(line taggedLine) bool {
		if (line.Stderr && !wantErr) || (!line.Stderr && !wantOut) {
			return true
		}
		text := line.Text + "\n"
		if details {
			text = line.details + " " + text
		}
		if stamps {
			text = line.Time.UTC().Format(time.RFC3339Nano) + " " + text
		}
		stream := byte(1)
		if line.Stderr {
			stream = 2
		}
		frame := make([]byte, 8, 8+len(text))
		frame[0] = stream
		binary.BigEndian.PutUint32(frame[4:], uint32(len(text)))
		frame = append(frame, text...)
		_, err := w.Write(frame)
		return err == nil
	}

	var lines []taggedLine
	for _, s := range srcs {
		for _, l := range b.c.host.LogLines(s.ctr, now, max(tail, 1)) {
			if !since.IsZero() && l.Time.Before(since) {
				continue
			}
			lines = append(lines, taggedLine{LogLine: l, details: s.details})
		}
	}
	sort.SliceStable(lines, func(i, j int) bool { return lines[i].Time.Before(lines[j].Time) })
	if tail == 0 {
		lines = nil
	} else if len(lines) > tail {
		lines = lines[len(lines)-tail:]
	}
	for _, l := range lines {
		if !write(l) {
			return
		}
	}
	flush(w)
	if !queryBool(r, "follow") {
		return
	}
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	last := now
	for {
		select {
		case <-r.Context().Done():
			return
		case t := <-ticker.C:
			var fresh []taggedLine
			for _, s := range refresh() {
				if s.ctr.State != estate.StateRunning {
					continue
				}
				for _, l := range b.c.host.LogLines(s.ctr, t, 2) {
					if l.Time.After(last) {
						fresh = append(fresh, taggedLine{LogLine: l, details: s.details})
					}
				}
			}
			sort.SliceStable(fresh, func(i, j int) bool { return fresh[i].Time.Before(fresh[j].Time) })
			for _, l := range fresh {
				if !write(l) {
					return
				}
			}
			last = t
			flush(w)
		}
	}
}

func flush(w http.ResponseWriter) {
	if f, ok := w.(http.Flusher); ok {
		f.Flush()
	}
}
