// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package estate

import (
	"fmt"
	"strconv"
)

// rng is splitmix64: tiny, fast and fully determined by its seed, which is
// what keeps the estate identical across restarts without storing it.
type rng struct{ state uint64 }

func newRNG(seed uint64) *rng { return &rng{state: seed} }

func (r *rng) next() uint64 {
	r.state += 0x9e3779b97f4a7c15
	z := r.state
	z = (z ^ (z >> 30)) * 0xbf58476d1ce4e5b9
	z = (z ^ (z >> 27)) * 0x94d049bb133111eb
	return z ^ (z >> 31)
}

func (r *rng) intn(n int) int {
	if n <= 0 {
		return 0
	}
	return int(r.next() % uint64(n))
}

func (r *rng) between(lo, hi int) int { return lo + r.intn(hi-lo+1) }

func (r *rng) float() float64 { return float64(r.next()>>11) / float64(1<<53) }

func (r *rng) chance(p float64) bool { return r.float() < p }

func (r *rng) hex(n int) string {
	out := make([]byte, 0, n)
	for len(out) < n {
		out = append(out, fmt.Sprintf("%016x", r.next())...)
	}
	return string(out[:n])
}

func pick[T any](r *rng, items []T) T { return items[r.intn(len(items))] }

func hashString(s string) uint64 {
	const offset, prime = 14695981039346656037, 1099511628211
	h := uint64(offset)
	for i := 0; i < len(s); i++ {
		h ^= uint64(s[i])
		h *= prime
	}
	return h
}

func itoa(n int) string { return strconv.Itoa(n) }

// StableHash is a deterministic 64-bit hash of s for callers that need stable
// synthetic identifiers derived from a name.
func StableHash(s string) uint64 { return hashString(s) }
