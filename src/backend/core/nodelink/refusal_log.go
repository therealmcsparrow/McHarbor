// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package nodelink

import (
	"strconv"
	"sync"
	"time"
)

// refusalLogEvery is the shortest gap between two refusal warnings for one hop. The
// node-to-node routes are public, so without it a stream of junk requests would turn
// into a stream of log lines.
const refusalLogEvery = 10 * time.Second

// maxLoggedHeader bounds how much of a caller-supplied header reaches the log.
const maxLoggedHeader = 64

// refusalLog rate-limits the warning a node writes when it refuses a node-to-node call,
// counting what it suppressed so the next line still says how much happened.
type refusalLog struct {
	mu         sync.Mutex
	last       map[string]time.Time
	suppressed map[string]int
}

func newRefusalLog() *refusalLog {
	return &refusalLog{last: map[string]time.Time{}, suppressed: map[string]int{}}
}

// allow reports whether hop may log now and how many refusals were swallowed since its
// last line. The key set is the fixed list of hops, so the maps stay tiny.
func (r *refusalLog) allow(hop string, now time.Time) (bool, int) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if at, ok := r.last[hop]; ok && now.Sub(at) < refusalLogEvery {
		r.suppressed[hop]++
		return false, 0
	}
	n := r.suppressed[hop]
	r.last[hop] = now
	r.suppressed[hop] = 0
	return true, n
}

// logRefusal writes one bounded, rate-limited warning for a refused call. The peer header
// and every string attribute (the route's env id among them) are attacker-controlled on a
// public route, so they are truncated, and the peer is quoted.
func (l *Link) logRefusal(hop string, err error, peer string, attrs ...any) {
	if l == nil || l.logger == nil || l.refusals == nil {
		return
	}
	ok, suppressed := l.refusals.allow(hop, time.Now())
	if !ok {
		return
	}
	args := make([]any, 0, 8+len(attrs))
	args = append(args, "hop", hop, "error", err, "peer", strconv.Quote(clipLogged(peer)), "suppressed", suppressed)
	for _, a := range attrs {
		if s, isString := a.(string); isString {
			a = clipLogged(s)
		}
		args = append(args, a)
	}
	l.logger.Warn("nodelink: refusing a node-to-node call", args...)
}

// clipLogged bounds a caller-supplied value to maxLoggedHeader bytes.
func clipLogged(s string) string {
	if len(s) > maxLoggedHeader {
		return s[:maxLoggedHeader]
	}
	return s
}
