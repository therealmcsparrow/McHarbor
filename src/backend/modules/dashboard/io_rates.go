// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package dashboard

import (
	"database/sql"
	"time"
)

// historyPoints is how many points each chart series carries.
const historyPoints = 60

// hostMetricsTimeLayout is how host_metrics.timestamp is written.
const hostMetricsTimeLayout = "2006-01-02 15:04:05"

// ioSample is one host_metrics row's cumulative I/O counters, oldest first.
type ioSample struct {
	at, label                           string
	netRx, netTx, blockRead, blockWrite sql.NullInt64
}

// ioRates turns consecutive cumulative counters into bytes per second, labelled
// with the later sample's time. A pair with a missing counter or unreadable time
// yields no point; a counter that went backwards (a host reboot, an agent that
// reconnected with fresh counters) yields 0 rather than a negative spike.
func ioRates(samples []ioSample) (netRx, netTx, blockRead, blockWrite []TimeValue) {
	for i := 1; i < len(samples); i++ {
		prev, cur := samples[i-1], samples[i]
		before, errBefore := time.Parse(hostMetricsTimeLayout, prev.at)
		after, errAfter := time.Parse(hostMetricsTimeLayout, cur.at)
		if errBefore != nil || errAfter != nil {
			continue
		}
		seconds := after.Sub(before).Seconds()
		if seconds <= 0 {
			continue
		}
		netRx = appendRate(netRx, cur.label, prev.netRx, cur.netRx, seconds)
		netTx = appendRate(netTx, cur.label, prev.netTx, cur.netTx, seconds)
		blockRead = appendRate(blockRead, cur.label, prev.blockRead, cur.blockRead, seconds)
		blockWrite = appendRate(blockWrite, cur.label, prev.blockWrite, cur.blockWrite, seconds)
	}
	return lastN(netRx, historyPoints), lastN(netTx, historyPoints),
		lastN(blockRead, historyPoints), lastN(blockWrite, historyPoints)
}

func appendRate(series []TimeValue, label string, prev, cur sql.NullInt64, seconds float64) []TimeValue {
	if !prev.Valid || !cur.Valid {
		return series
	}
	rate := 0.0
	if cur.Int64 >= prev.Int64 {
		rate = float64(cur.Int64-prev.Int64) / seconds
	}
	return append(series, TimeValue{Timestamp: label, Value: rate})
}

// lastN keeps the newest n points of an oldest-first series.
func lastN(series []TimeValue, n int) []TimeValue {
	if len(series) <= n {
		return series
	}
	return series[len(series)-n:]
}
