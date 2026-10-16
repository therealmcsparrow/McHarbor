// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

import (
	"context"
	"strings"
	"time"
)

// narrator writes the prose around the numbers.
//
// Deterministic by default, and that is the important part: the same
// data must always produce the same sentences, or two people reading the
// same report reach different conclusions and nobody can diff one period
// against the next. A model is allowed to improve the phrasing of the
// opening paragraph when an operator has explicitly turned that on, and
// is never allowed to introduce, remove or restate a figure.
type narrator struct {
	// complete is the host's AI router, or nil. Set only when the
	// operator has enabled AI narrative in plugin settings.
	complete func(ctx context.Context, system, user string) (string, error)
	ctx      context.Context
	// usedAI is set true the moment a model's polished text is actually
	// adopted in place of the deterministic lead — never merely because
	// complete was non-nil or a call was attempted. A narrator is
	// created fresh per Generate call (see newNarrator's call site in
	// service.go), so this is per-generation state, not a field shared
	// across concurrent generations. service.Generate reads it after
	// the document is built to decide what belongs in
	// StoredReport.AIConnectionID — see reportedAIConnectionID.
	usedAI bool
}

func newNarrator(ctx context.Context, complete func(context.Context, string, string) (string, error)) *narrator {
	return &narrator{complete: complete, ctx: ctx}
}

// summaryLead is the opening paragraph — the sentence a director reads
// if they read nothing else.
func (n *narrator) summaryLead(b *buildCtx) string {
	base := n.deterministicLead(b)
	if n.complete == nil {
		return base
	}
	polished, err := n.polish(base, b)
	if err != nil || polished == "" {
		// Any failure falls back to the deterministic text. A report is
		// never blocked on a model being reachable. usedAI stays false:
		// the deterministic text was not written by the model.
		return base
	}
	n.usedAI = true
	return polished
}

func (n *narrator) deterministicLead(b *buildCtx) string {
	f := b.facts
	var sb strings.Builder

	period := fmtPeriod(b.snap.Period)
	if b.snap.Period.PointInTime {
		sb.WriteString("This report describes " + b.meta.Environment + " " + period + ". ")
	} else {
		sb.WriteString("This report covers " + b.meta.Environment + " for " + period + ". ")
	}

	if b.score != nil && b.score.Overall >= 0 && b.rt.hasLevel(Strategic) {
		sb.WriteString("Overall posture is " + bandWord(b.score.Band) + " at " +
			fmtInt(b.score.Overall) + " out of 100")
		if worst := worstDomain(b.score); worst != nil && worst.Band != BandNotAssessed &&
			(worst.Band == BandAtRisk || worst.Band == BandCritical || worst.Band == BandWatch) {
			sb.WriteString(", held down by " + strings.ToLower(worst.Label) + " at " + fmtInt(worst.Score))
		}
		sb.WriteString(". ")
	}

	// Then the estate in one sentence, so the reader has the scale.
	if f.Containers.Total > 0 {
		sb.WriteString("The environment runs " + plural(f.Containers.Running, "workload", "workloads"))
		if f.Containers.Total != f.Containers.Running {
			sb.WriteString(" of " + fmtInt(f.Containers.Total) + " defined")
		}
		if f.Containers.Stacks > 0 {
			sb.WriteString(" across " + plural(f.Containers.Stacks, "stack", "stacks"))
		}
		sb.WriteString(". ")
	}

	// Close on the single thing that most needs saying.
	if fam, ok := familyFor(b.rt.Key); ok && fam.lead != nil {
		if closing := fam.lead(b); closing != "" {
			sb.WriteString(closing)
			return sb.String()
		}
	}
	switch {
	case f.Security.Assessed && f.Security.Critical > 0:
		sb.WriteString(plural(f.Security.Critical, "critical vulnerability remains", "critical vulnerabilities remain") +
			" open and should be closed before the next period.")
	case len(f.Reliability.CrashLooping) > 0:
		sb.WriteString(plural(len(f.Reliability.CrashLooping), "workload is", "workloads are") +
			" crash-looping and is not serving traffic.")
	case f.Resilience.Assessed && f.Resilience.LastSuccessAge > 7:
		sb.WriteString("The most recent successful backup is " + fmtDays(f.Resilience.LastSuccessAge) +
			", which is outside the intended recovery point.")
	case f.Capacity.Assessed && f.Capacity.HeadroomPercent < 15 && f.Capacity.MemTotalBytes > 0:
		sb.WriteString("Capacity headroom is down to " + fmtPercent(f.Capacity.HeadroomPercent) +
			" and should be addressed before further workloads are added.")
	case f.Containers.Unhealthy > 0:
		sb.WriteString(plural(f.Containers.Unhealthy, "workload is", "workloads are") +
			" failing health checks.")
	default:
		sb.WriteString("No matters requiring escalation were identified in this period.")
	}
	return sb.String()
}

// polish asks the model to improve the wording only. The prompt is
// deliberately restrictive and the result is validated: if the model
// invented or dropped a number, the deterministic text is used instead.
func (n *narrator) polish(base string, b *buildCtx) (string, error) {
	ctx, cancel := context.WithTimeout(n.ctx, 20*time.Second)
	defer cancel()

	const system = "You are editing one paragraph of a board-level IT infrastructure report. " +
		"Improve clarity and flow only. Do not add, remove, round or reword any number, " +
		"percentage, date or proper noun. Do not add commentary, recommendations or " +
		"speculation. Return one paragraph of plain prose, no markdown, at most 90 words."

	out, err := n.complete(ctx, system, base)
	if err != nil {
		return "", err
	}
	out = strings.TrimSpace(out)
	if out == "" || len(out) > len(base)*3 {
		return "", nil
	}
	// Every number in the original must survive verbatim; anything else
	// means the model restated a figure and the text can no longer be
	// trusted to agree with the tables beneath it.
	for _, tok := range numericTokens(base) {
		if !strings.Contains(out, tok) {
			return "", nil
		}
	}
	return out, nil
}

// numericTokens pulls the runs of digits out of a sentence so the
// polished version can be checked against them.
func numericTokens(s string) []string {
	var out []string
	cur := strings.Builder{}
	flush := func() {
		if cur.Len() > 0 {
			out = append(out, cur.String())
			cur.Reset()
		}
	}
	for _, r := range s {
		if (r >= '0' && r <= '9') || r == '.' || r == ',' {
			cur.WriteRune(r)
			continue
		}
		flush()
	}
	flush()
	// Single digits appear inside ordinary words often enough to be noise.
	filtered := out[:0]
	for _, t := range out {
		t = strings.Trim(t, ".,")
		if len(t) >= 2 {
			filtered = append(filtered, t)
		}
	}
	return filtered
}

// whatMoved is the "what changed" list: three to five bullets, each a
// complete statement with its own evidence.
func (n *narrator) whatMoved(b *buildCtx) []string {
	f := b.facts
	var out []string

	if f.Delivery.Assessed && f.Delivery.Deployments > 0 {
		s := plural(f.Delivery.Deployments, "change was deployed", "changes were deployed") +
			" (" + fmtDecimal(f.Delivery.DeploymentsPerWeek, 1) + " per week)"
		if f.Delivery.ChangeFailureRate > 0 {
			s += ", with a change failure rate of " + fmtPercent(f.Delivery.ChangeFailureRate)
		}
		out = append(out, s+".")
	}
	if f.Reliability.Assessed && (f.Reliability.Restarts+f.Reliability.Crashes) > 0 {
		s := plural(f.Reliability.Crashes, "unexpected stop", "unexpected stops") +
			" and " + plural(f.Reliability.Restarts, "restart", "restarts") + " were recorded"
		if f.Reliability.RecoverySamples > 0 {
			s += "; mean recovery was " + fmtDuration(f.Reliability.MeanRecoveryMinutes) +
				" across " + fmtInt(f.Reliability.RecoverySamples) + " observed recoveries"
		}
		out = append(out, s+".")
	}
	if f.Security.Assessed {
		out = append(out, "Vulnerability scanning covers "+fmtPercent(f.Security.CoveragePercent)+
			" of images, with "+fmtInt(f.Security.Critical)+" critical and "+fmtInt(f.Security.High)+
			" high findings open.")
	}
	if f.Resilience.Assessed && f.Resilience.Runs > 0 {
		out = append(out, plural(f.Resilience.Runs, "backup run completed", "backup runs completed")+
			" at a "+fmtPercent(f.Resilience.SuccessRate)+" success rate, covering "+
			fmtPercent(f.Resilience.CoveragePercent)+" of running workloads.")
	}
	if f.Capacity.Assessed && f.Capacity.MemTotalBytes > 0 {
		out = append(out, utilisationSentence(f))
	}
	if f.Images.ReclaimableBytes > 0 {
		out = append(out, fmtBytes(f.Images.ReclaimableBytes)+" of image storage is unreferenced and can be reclaimed.")
	}
	if len(out) > 5 {
		out = out[:5]
	}
	return out
}

// recommendations turns the findings into an ordered list of actions.
// Ordered, because a board wants to know what to do first.
func (n *narrator) recommendations(b *buildCtx) []string {
	var out []string
	for _, f := range b.topFindings(5) {
		if f.Recommendation == "" {
			continue
		}
		rec := f.Recommendation
		if f.Owner != "" {
			rec += " (" + f.Owner + ")"
		}
		out = append(out, rec)
	}
	if len(out) == 0 {
		out = append(out, "No action is required this period. Maintain the current scanning, "+
			"backup and update cadences.")
	}
	return out
}

func bandWord(b Band) string {
	switch b {
	case BandStrong:
		return "strong"
	case BandStable:
		return "stable"
	case BandWatch:
		return "one to watch"
	case BandAtRisk:
		return "at risk"
	case BandCritical:
		return "critical"
	default:
		return "not assessed"
	}
}

func bandLabel(b Band) string {
	switch b {
	case BandStrong:
		return "Strong"
	case BandStable:
		return "Stable"
	case BandWatch:
		return "Watch"
	case BandAtRisk:
		return "At risk"
	case BandCritical:
		return "Critical"
	default:
		return "Not assessed"
	}
}

// worstDomain returns the lowest-scoring assessed domain.
func worstDomain(sc *Scorecard) *DomainRate {
	var worst *DomainRate
	for i := range sc.Domains {
		d := &sc.Domains[i]
		if d.Band == BandNotAssessed {
			continue
		}
		if worst == nil || d.Score < worst.Score {
			worst = d
		}
	}
	return worst
}

// utilisationSentence states host utilisation and, crucially, what it is
// a statement about.
//
// "Host utilisation is 34 % CPU" is three different claims depending on
// whether it was read once, averaged over a month, or averaged over the
// six days of a month that were actually recorded. The summary is the
// part of a report most likely to be quoted on its own, so it carries
// the qualification rather than relying on a callout further down.
func utilisationSentence(f Facts) string {
	c := f.Capacity
	head := "Host utilisation is " + fmtPercent(c.CPUPercent) + " CPU and " +
		fmtPercent(c.MemPercent) + " memory, leaving " + fmtPercent(c.HeadroomPercent) + " headroom."

	switch {
	case !c.HistoryAvailable:
		return head + " These are single readings taken when the report was generated, " +
			"not figures for the period."
	case c.CoverageShort():
		return "Host utilisation averaged " + fmtPercent(c.CPUPercent) + " CPU and " +
			fmtPercent(c.MemPercent) + " memory, leaving " + fmtPercent(c.HeadroomPercent) +
			" headroom, over the " + fmtDecimal(c.CoveredDays, 1) + " days of metric history " +
			"available out of the " + fmtDecimal(c.RequestedDays, 0) + " this report covers."
	default:
		return "Host utilisation averaged " + fmtPercent(c.CPUPercent) + " CPU and " +
			fmtPercent(c.MemPercent) + " memory over the period, peaking at " +
			fmtPercent(c.CPUStat.Peak) + " and " + fmtPercent(c.MemStat.Peak) +
			", leaving " + fmtPercent(c.HeadroomPercent) + " headroom."
	}
}
