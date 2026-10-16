// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

import (
	"sort"
)

// Webhook thresholds. A retry is an attempt for the same event within
// govRetryWindow of a failed one; "currently failing" is the newest
// govFailingStreak attempts all having failed.
const (
	govRetryWindowMinutes = 15
	govFailingStreak      = 3
)

// webhookStat is one webhook's delivery record over the period.
type webhookStat struct {
	Hook         govWebhookRow
	Read         bool
	Truncated    bool
	Attempts     int
	Failed       int
	Retries      int
	Durations    []float64
	LastSuccess  string
	Streak       int
	FailureRate  float64
	CodeClasses  map[string]int
	CurrentlyBad bool
}

// webhookFacts is what the webhook-delivery report derives.
type webhookFacts struct {
	Assessed      bool
	DeliveriesOK  bool
	Hooks         int
	Active        int
	Unread        int
	Stats         []*webhookStat
	Attempts      int
	Failed        int
	Retries       int
	FailureRate   float64
	MedianMs      float64
	CodeClasses   map[string]int
	OKStamps      []string
	FailStamps    []string
	Failing       []*webhookStat
	Unreliable    []*webhookStat
	Silent        []*webhookStat
	PlainHTTP     []*webhookStat
	AnyTruncation bool
}

func govWebhookStats(snap *Snapshot) webhookFacts {
	g := snap.Governance
	p := snap.Period
	f := webhookFacts{
		Assessed:     snap.available(srcWebhooks),
		DeliveriesOK: snap.available(srcWebhookDeliveries),
		CodeClasses:  map[string]int{},
	}
	if !f.Assessed {
		return f
	}
	var durations []float64
	for _, h := range g.Webhooks {
		f.Hooks++
		if h.IsActive {
			f.Active++
		}
		st := &webhookStat{Hook: h, CodeClasses: map[string]int{}}
		f.Stats = append(f.Stats, st)
		if h.IsActive && h.Scheme == "http" {
			f.PlainHTTP = append(f.PlainHTTP, st)
		}
		rows, ok := g.Deliveries[h.ID]
		if !ok {
			f.Unread++
			continue
		}
		st.Read = true
		if len(rows) >= govDeliveriesPage && govInPeriod(rows[len(rows)-1].CreatedAt, p) {
			st.Truncated = true
			f.AnyTruncation = true
		}
		// Rows arrive newest first; walk oldest first so a retry is
		// recognised by the failure that preceded it.
		lastFail := map[string]string{}
		for i := len(rows) - 1; i >= 0; i-- {
			d := rows[i]
			if !govInPeriod(d.CreatedAt, p) {
				continue
			}
			st.Attempts++
			class := govStatusClass(d)
			st.CodeClasses[class]++
			f.CodeClasses[class]++
			if d.Duration > 0 {
				st.Durations = append(st.Durations, float64(d.Duration))
				durations = append(durations, float64(d.Duration))
			}
			if prev, seen := lastFail[d.Event]; seen {
				if gap := parseTS(d.CreatedAt).Sub(parseTS(prev)).Minutes(); gap >= 0 && gap <= govRetryWindowMinutes {
					st.Retries++
				}
				delete(lastFail, d.Event)
			}
			if d.Success {
				st.LastSuccess = d.CreatedAt
				f.OKStamps = append(f.OKStamps, d.CreatedAt)
			} else {
				st.Failed++
				lastFail[d.Event] = d.CreatedAt
				f.FailStamps = append(f.FailStamps, d.CreatedAt)
			}
		}
		// The streak is read from the newest attempts regardless of the
		// period: "is it failing now" is a question about the present.
		for _, d := range rows {
			if d.Success {
				break
			}
			st.Streak++
		}
		st.FailureRate = pct(st.Failed, st.Attempts)
		st.CurrentlyBad = h.IsActive && st.Streak >= govFailingStreak
		f.Attempts += st.Attempts
		f.Failed += st.Failed
		f.Retries += st.Retries
		switch {
		case st.CurrentlyBad:
			f.Failing = append(f.Failing, st)
		case st.Attempts >= 5 && st.FailureRate >= 20:
			f.Unreliable = append(f.Unreliable, st)
		}
		if h.IsActive && st.Attempts == 0 {
			f.Silent = append(f.Silent, st)
		}
	}
	f.FailureRate = pct(f.Failed, f.Attempts)
	f.MedianMs = median(durations)
	sort.SliceStable(f.Stats, func(i, j int) bool {
		if f.Stats[i].Failed != f.Stats[j].Failed {
			return f.Stats[i].Failed > f.Stats[j].Failed
		}
		return f.Stats[i].Hook.Name < f.Stats[j].Hook.Name
	})
	return f
}

// govStatusClass buckets a delivery by outcome; a zero status means the
// request never got an HTTP answer at all.
func govStatusClass(d govDeliveryRow) string {
	switch {
	case d.ResponseStatus == 0:
		return "No response"
	case d.ResponseStatus < 300:
		return "2xx"
	case d.ResponseStatus < 400:
		return "3xx"
	case d.ResponseStatus < 500:
		return "4xx"
	default:
		return "5xx"
	}
}

func webhookKPIs(f webhookFacts) []KPI {
	if !f.Assessed {
		return []KPI{}
	}
	return []KPI{
		{Label: "Webhooks", Value: fmtInt(f.Hooks), Hint: fmtInt(f.Active) + " active"},
		{Label: "Deliveries", Value: fmtInt(f.Attempts), Hint: "attempts in the period"},
		{Label: "Failure rate", Value: govPercentOrDash(f.FailureRate, f.Attempts),
			Target: "≤ 1%", Status: govInvRateStatus(f.FailureRate, f.Attempts, 1, 5)},
		{Label: "Retries", Value: fmtInt(f.Retries), Status: countStatus(f.Retries, 0, 10)},
		{Label: "Failing now", Value: fmtInt(len(f.Failing)), Status: countStatus(len(f.Failing), 0, 0),
			Hint: "last " + fmtInt(govFailingStreak) + " attempts failed"},
	}
}

func webhookFindings(f webhookFacts) []Finding {
	if !f.Assessed {
		return nil
	}
	var out []Finding
	if n := len(f.Failing); n > 0 {
		out = append(out, Finding{
			Severity: SevHigh,
			Title:    plural(n, "webhook endpoint is failing every delivery", "webhook endpoints are failing every delivery"),
			Detail: "Endpoints: " + joinAnd(govHookLabels(f.Failing)) + ". Their most recent attempts all failed, so whatever " +
				"listens there — a chat channel, an incident tool, a pipeline — is not hearing from this platform.",
			Recommendation: "Check each endpoint is still reachable and still accepts the signature; disable webhooks whose receiver has been retired.",
			Owner:          "Integration owner",
		})
	}
	if n := len(f.Unreliable); n > 0 {
		out = append(out, Finding{
			Severity:       SevMedium,
			Title:          plural(n, "webhook fails at least one delivery in five", "webhooks fail at least one delivery in five"),
			Detail:         "Endpoints: " + joinAnd(govHookLabels(f.Unreliable)) + ". Intermittent failure means some events are lost without anyone seeing a pattern.",
			Recommendation: "Compare the response codes: 4xx points at the payload or signature, 5xx and no response at the receiver.",
			Owner:          "Integration owner",
		})
	}
	if n := len(f.PlainHTTP); n > 0 {
		out = append(out, Finding{
			Severity:       SevMedium,
			Title:          plural(n, "active webhook delivers over plain HTTP", "active webhooks deliver over plain HTTP"),
			Detail:         "Endpoints: " + joinAnd(govHookLabels(f.PlainHTTP)) + ". Event payloads and their signatures cross the network unencrypted.",
			Recommendation: "Move each receiver to HTTPS.",
			Owner:          "Integration owner",
		})
	}
	if n := len(f.Silent); n > 0 && f.DeliveriesOK {
		out = append(out, Finding{
			Severity:       SevLow,
			Title:          plural(n, "active webhook sent nothing in the period", "active webhooks sent nothing in the period"),
			Detail:         "Either none of their subscribed events occurred, or the subscription no longer matches any event the platform emits.",
			Recommendation: "Send a test delivery from Webhooks and confirm the event list.",
			Owner:          "Integration owner",
		})
	}
	return out
}

func webhookLead(f webhookFacts) string {
	switch {
	case !f.Assessed:
		return "The webhook list could not be read, so outbound delivery is not assessed."
	case f.Hooks == 0:
		return "No outbound webhook is configured."
	case len(f.Failing) > 0:
		return plural(len(f.Failing), "webhook endpoint is", "webhook endpoints are") + " failing every recent delivery."
	case f.Attempts == 0:
		return "No webhook delivery was attempted in this period."
	default:
		return plural(f.Attempts, "delivery was", "deliveries were") + " attempted with a " +
			fmtPercent(f.FailureRate) + " failure rate."
	}
}

func buildWebhookDelivery(b *buildCtx) []Section {
	snap := b.snap
	f := govWebhookStats(snap)
	var out []Section

	ta := section(Tactical, "Delivery health")
	ta.Blocks = append(ta.Blocks,
		para("Every event this platform pushes to another system goes through a webhook. A failed delivery "+
			"is an alert nobody received. Endpoints are identified by host only: a webhook URL frequently "+
			"carries its credential in the path or query, and a report is not the place to print it."))
	if !f.Assessed {
		ta.Blocks = append(ta.Blocks, govNotAssessed(snap, srcWebhooks, "The webhook list"))
		return append(out, ta)
	}
	if !f.DeliveriesOK && f.Hooks > 0 {
		ta.Blocks = append(ta.Blocks, govNotAssessed(snap, srcWebhookDeliveries, "Delivery history"))
	} else if f.Unread > 0 {
		ta.Blocks = append(ta.Blocks, callout(StatusInfo, "Partial read",
			plural(f.Unread, "webhook's", "webhooks'")+" delivery history was not read, so the figures below cover the rest."))
	}
	if f.AnyTruncation {
		ta.Blocks = append(ta.Blocks, callout(StatusInfo, "Busy endpoints",
			"At least one webhook delivered more than "+fmtInt(govDeliveriesPage)+" times in the period; only its most recent "+
				"attempts are counted. Shorten the period for complete figures."))
	}
	if f.Attempts > 0 {
		ta.Blocks = append(ta.Blocks,
			chartBlock(Chart{
				Kind: ChartStackedBar, Title: "Deliveries per day", Height: 120, ShowLegend: true,
				Series: []ChartSeries{
					{Name: "Delivered", Status: StatusGood, Points: govDaySeries(f.OKStamps, snap.Period)},
					{Name: "Failed", Status: StatusBad, Points: govDaySeries(f.FailStamps, snap.Period)},
				},
			}),
			webhookCodeChart(f))
	}
	ta.Blocks = append(ta.Blocks, webhookTable(f))
	out = append(out, ta)

	op := section(Operational, "Endpoint detail")
	op.Blocks = append(op.Blocks, webhookSubscriptionTable(f))
	out = append(out, op)
	return out
}

func webhookCodeChart(f webhookFacts) Block {
	order := []string{"2xx", "3xx", "4xx", "5xx", "No response"}
	tones := map[string]Status{"2xx": StatusGood, "3xx": StatusInfo, "4xx": StatusWarn, "5xx": StatusBad, "No response": StatusBad}
	pts := []ChartPoint{}
	for _, k := range order {
		if n := f.CodeClasses[k]; n > 0 {
			pts = append(pts, ChartPoint{Label: k, Value: float64(n), Status: tones[k]})
		}
	}
	return chartBlock(Chart{
		Kind: ChartDonut, Title: "Responses received", Unit: "attempts", Height: 130,
		Series:  []ChartSeries{{Points: pts}},
		Caption: "4xx points at the payload or signature; 5xx and no response at the receiver.",
	})
}

func webhookTable(f webhookFacts) Block {
	rows := make([][]Cell, 0, len(f.Stats))
	for _, s := range f.Stats {
		state, tone := "Active", StatusGood
		switch {
		case !s.Hook.IsActive:
			state, tone = "Disabled", StatusMuted
		case s.CurrentlyBad:
			state, tone = "Failing", StatusBad
		}
		attempts := "—"
		rate := "—"
		if s.Read {
			attempts = fmtInt(s.Attempts)
			rate = govPercentOrDash(s.FailureRate, s.Attempts)
		}
		rows = append(rows, []Cell{
			cell(s.Hook.Name),
			mono(s.Hook.Host),
			pill(state, tone),
			cell(attempts),
			cellStatus(rate, govInvRateStatus(s.FailureRate, s.Attempts, 1, 20)),
			cell(fmtInt(s.Retries)),
			cell(govMsOrDash(median(s.Durations), len(s.Durations))),
			cell(orDash(fmtDateTime(parseTS(s.LastSuccess)))),
		})
	}
	if len(rows) == 0 {
		return callout(StatusInfo, "No webhooks", "No outbound webhook is configured on this installation.")
	}
	return table([]Column{
		{Label: "Webhook", Width: 1.8},
		{Label: "Endpoint host", Width: 2},
		{Label: "State", Width: 0.9},
		{Label: "Attempts", Width: 0.9, Align: "right"},
		{Label: "Failure rate", Width: 0.9, Align: "right"},
		{Label: "Retries", Width: 0.8, Align: "right"},
		{Label: "Median time", Width: 1, Align: "right"},
		{Label: "Last success", Width: 1.5},
	}, rows, "Delivery by endpoint", "A retry is a second attempt for the same event within "+
		fmtInt(govRetryWindowMinutes)+" minutes of a failure.")
}

func webhookSubscriptionTable(f webhookFacts) Block {
	rows := make([][]Cell, 0, len(f.Stats))
	for _, s := range f.Stats {
		scheme := s.Hook.Scheme
		tone := StatusGood
		if scheme != "https" {
			tone = StatusWarn
		}
		events := "All events"
		if len(s.Hook.Events) > 0 {
			events = joinAnd(s.Hook.Events)
		}
		rows = append(rows, []Cell{
			cell(s.Hook.Name),
			pill(orDash(scheme), tone),
			cell(govCap(events, 120)),
			cell(fmtDate(parseTS(s.Hook.CreatedAt))),
			cellStatus(fmtInt(s.Streak), countStatus(s.Streak, 0, govFailingStreak-1)),
		})
	}
	if len(rows) == 0 {
		return divider()
	}
	return table([]Column{
		{Label: "Webhook", Width: 1.8},
		{Label: "Transport", Width: 0.9},
		{Label: "Subscribed events", Width: 3},
		{Label: "Created", Width: 1.2},
		{Label: "Failing streak", Width: 1, Align: "right"},
	}, rows, "Subscriptions", "The failing streak counts consecutive failed attempts, newest first.")
}

func govHookLabels(list []*webhookStat) []string {
	out := make([]string, 0, len(list))
	for _, s := range list {
		out = append(out, s.Hook.Name+" ("+s.Hook.Host+")")
	}
	return out
}

func govInvRateStatus(rate float64, n int, good, warn float64) Status {
	if n == 0 {
		return StatusNeutral
	}
	return invThresholdStatus(rate, good, warn)
}
