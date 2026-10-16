// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package telemetry

import (
	"sync/atomic"
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

var (
	reconcilerPass = newHistogramVec("reconciler_pass_duration_seconds",
		"Duration of one background reconcile pass over every desired state.",
		[]float64{.05, .1, .25, .5, 1, 2.5, 5, 10, 30, 60, 120}, "outcome")
	reconcilerStates = newGaugeVec("reconciler_desired_states",
		"Desired states by what the last pass found: in_sync, drifted, pending (drift seen once, not yet confirmed) or unknown (environment unreachable).",
		"state")
	reconcilerTransitions = newCounterVec("reconciler_drift_transitions_total",
		"Desired states whose recorded drift flag changed, by direction (detected, resolved).", "direction")

	reconcilerOldestCheck atomic.Int64
)

func init() {
	Registry.MustRegister(prometheus.NewGaugeFunc(prometheus.GaugeOpts{
		Namespace: namespace,
		Name:      "reconciler_lag_seconds",
		Help:      "Age of the stalest successful drift check across all desired states; it keeps rising if the loop stalls or an environment stays unreachable.",
	}, func() float64 {
		oldest := reconcilerOldestCheck.Load()
		if oldest == 0 {
			return 0
		}
		return time.Since(time.Unix(0, oldest)).Seconds()
	}))
}

// ReconcilerStates is what one reconcile pass found.
type ReconcilerStates struct {
	InSync, Drifted, Pending, Unknown int
}

// ReconcilerPass records how long one pass took and whether it could run at all.
func ReconcilerPass(duration time.Duration, failed bool) {
	outcome := "ok"
	if failed {
		outcome = "failed"
	}
	reconcilerPass.WithLabelValues(outcome).Observe(duration.Seconds())
}

// ReconcilerObserved publishes what a pass found. oldestCheck is the least recent
// successful check across every desired state; zero, when there are none, reports
// no lag rather than a stale one.
func ReconcilerObserved(states ReconcilerStates, oldestCheck time.Time) {
	reconcilerStates.WithLabelValues("in_sync").Set(float64(states.InSync))
	reconcilerStates.WithLabelValues("drifted").Set(float64(states.Drifted))
	reconcilerStates.WithLabelValues("pending").Set(float64(states.Pending))
	reconcilerStates.WithLabelValues("unknown").Set(float64(states.Unknown))
	SetReconcilerOldestCheck(oldestCheck)
}

// SetReconcilerOldestCheck sets the timestamp reconciler_lag_seconds is measured from.
func SetReconcilerOldestCheck(t time.Time) {
	if t.IsZero() {
		reconcilerOldestCheck.Store(0)
		return
	}
	reconcilerOldestCheck.Store(t.UnixNano())
}

// ReconcilerDriftTransition counts a recorded drift flag flipping.
func ReconcilerDriftTransition(detected bool) {
	if detected {
		reconcilerTransitions.WithLabelValues("detected").Inc()
		return
	}
	reconcilerTransitions.WithLabelValues("resolved").Inc()
}
