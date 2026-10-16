// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package kubeapi

import (
	"crypto/sha256"
	"encoding/hex"
)

// rng is splitmix64, so the whole estate is a pure function of the seed and a
// restart shows the same clusters without anything being stored.
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

func (r *rng) chance(p float64) bool { return float64(r.next()>>11)/float64(1<<53) < p }

func pick[T any](r *rng, items []T) T { return items[r.intn(len(items))] }

const suffixAlphabet = "bcdfghjklmnpqrstvwxz2456789"

// suffix mimics the random name tails the ReplicaSet and pod generators use.
func (r *rng) suffix(n int) string {
	out := make([]byte, n)
	for i := range out {
		out[i] = suffixAlphabet[r.intn(len(suffixAlphabet))]
	}
	return string(out)
}

// uidFor derives a stable UUID-shaped uid from its parts, so the same object
// keeps its uid across restarts of the demo.
func uidFor(parts ...string) string {
	h := sha256.New()
	for _, p := range parts {
		h.Write([]byte(p))
		h.Write([]byte{0})
	}
	sum := h.Sum(nil)
	sum[6] = (sum[6] & 0x0f) | 0x40
	sum[8] = (sum[8] & 0x3f) | 0x80
	x := hex.EncodeToString(sum[:16])
	return x[0:8] + "-" + x[8:12] + "-" + x[12:16] + "-" + x[16:20] + "-" + x[20:32]
}

func hashHex(n int, parts ...string) string {
	h := sha256.New()
	for _, p := range parts {
		h.Write([]byte(p))
		h.Write([]byte{0})
	}
	return hex.EncodeToString(h.Sum(nil))[:n]
}
