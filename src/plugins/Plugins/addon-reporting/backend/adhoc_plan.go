// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

// AdHocPlan is what the planner decided, after the host has validated it.
type AdHocPlan struct {
	Sources       []string       `json:"sources"`
	Days          int            `json:"days"`
	EnvironmentID string         `json:"environment_id"`
	Subject       Subject        `json:"subject"`
	Levels        []Level        `json:"levels"`
	Title         string         `json:"title"`
	Caps          map[string]int `json:"caps"`

	// AIConnectionID pins which configured AI connection wrote (and, on
	// replay, must keep writing) this plan's report. It is set by the
	// host from the caller's request, never by the model: planSystem
	// below deliberately never offers "ai_connection_id" in the
	// planner's JSON schema, because letting a model choose its own
	// provider is exactly what pinning exists to prevent. planAdHoc
	// overwrites this field with the host-supplied value immediately
	// after unmarshalling a model reply, so even a reply that happens to
	// include the key has no effect. Persisted on the stored plan so a
	// replayed regenerate reuses the same AI rather than silently
	// drifting to whatever the current default happens to be.
	AIConnectionID string `json:"ai_connection_id,omitempty"`
}

// errPlanFailed is the sentinel wrapped into every pass-1 failure, so a
// caller can tell "the model never produced a usable plan" apart from a
// transport error without string matching.
var errPlanFailed = errors.New("reporting: ad-hoc planning failed")

// extractJSON pulls a JSON object out of a model reply.
//
// The host AI interface has no structured-output mode, so a reply may
// arrive fenced, prefaced, or both. Taking the first brace through the
// last is cruder than a parser and correct for every shape a chat model
// actually emits.
func extractJSON(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.Index(s, "```"); i >= 0 {
		s = s[i+3:]
		if j := strings.IndexByte(s, '\n'); j >= 0 {
			s = s[j+1:]
		}
		if k := strings.LastIndex(s, "```"); k >= 0 {
			s = s[:k]
		}
	}
	start, end := strings.IndexByte(s, '{'), strings.LastIndexByte(s, '}')
	if start < 0 || end <= start {
		return ""
	}
	return s[start : end+1]
}

// validate is where the plan stops being the model's opinion.
//
// Every field is clamped or dropped against what the host knows exists.
// An unknown source key is dropped rather than failing the plan, because
// a model naming one good source and one imaginary one has still
// answered usefully; an empty surviving set is a failure, because an
// ad-hoc report with no data is not a report. A JSON reply that parses
// but carries no usable sources (e.g. "{}") is caught the same way: it
// clears down to zero sources here and errors out below.
//
// Title is clamped to length only — an empty title is left empty, since
// this method has no prompt to fall back to (a pinned plan being
// replayed for a regenerate has no original prompt in scope either).
// planAdHoc fills the prompt-derived fallback after calling this.
//
// availableAIConnections is the current host.AI.Connections() listing,
// reduced to a lookup set (see connectionIDSet in service.go). A pinned
// AIConnectionID that is no longer in it — the connection was deleted or
// disabled since the plan was first made — is dropped to "", which
// degrades the report to the host's default/favourite connection rather
// than failing a regenerate forever over a connection that no longer
// exists. log, when non-nil, gets an Info line naming the dropped id, so
// a silent AI substitution on a replayed pin is visible in the logs
// rather than discoverable only by diffing two stored reports.
func (p AdHocPlan) validate(envs []envRow, availableAIConnections map[string]bool, log schemaLogger) (AdHocPlan, error) {
	known := make(map[string]bool, len(sourceCatalog()))
	for _, d := range sourceCatalog() {
		known[d.Key] = true
	}
	var srcs []string
	seen := make(map[string]bool)
	for _, s := range p.Sources {
		s = strings.TrimSpace(s)
		if known[s] && !seen[s] {
			srcs = append(srcs, s)
			seen[s] = true
		}
	}
	if len(srcs) == 0 {
		return AdHocPlan{}, fmt.Errorf("%w: no usable source", errPlanFailed)
	}
	p.Sources = srcs

	if p.Days < 1 || p.Days > 365 {
		p.Days = 30
	}

	valid := map[Level]bool{Strategic: true, Tactical: true, Operational: true}
	var lv []Level
	for _, l := range p.Levels {
		if valid[l] {
			lv = append(lv, l)
		}
	}
	if len(lv) == 0 {
		lv = []Level{Strategic, Tactical, Operational}
	}
	p.Levels = lv

	caps := make(map[string]int, len(p.Sources))
	for _, s := range p.Sources {
		c := p.Caps[s]
		if c <= 0 || c > maxRowCap {
			c = defaultRowCap
		}
		caps[s] = c
	}
	p.Caps = caps

	// An environment the caller cannot see is not a plan the caller may
	// run. Collection already runs as the requesting user, and `envs` is
	// what that user could list, so membership here is the check.
	if p.EnvironmentID != "" {
		found := false
		for _, e := range envs {
			if e.ID == p.EnvironmentID {
				found = true
				break
			}
		}
		if !found {
			p.EnvironmentID = ""
		}
	}

	if p.AIConnectionID != "" && !availableAIConnections[p.AIConnectionID] {
		if log != nil {
			log.Info("reporting: pinned ai connection no longer available, falling back to default",
				"connection_id", p.AIConnectionID)
		}
		p.AIConnectionID = ""
	}

	p.Title = clampText(p.Title, 120, "")
	return p, nil
}

// reportType turns a validated plan into the synthetic type that drives
// the existing collector. collect() reads rt.Sources and nothing else; a
// planned Docker facet source adds the snapshot sources its projection
// reads, and every facet source is collected by collectFacets.
func (p AdHocPlan) reportType() ReportType {
	var dockerFacets []string
	for _, s := range p.Sources {
		if f, platform, ok := parseFacetSource(s); ok && platform == facetPlatformDocker {
			dockerFacets = append(dockerFacets, f)
		}
	}
	return ReportType{
		Key: TypeAdHoc, Category: "custom", Icon: "sparkles",
		Levels: p.Levels, Sources: withFacetSources(p.Sources, dockerFacets),
		DefaultDays: p.Days, EnvironmentScoped: true,
		// Subjects lists every scope kind Generate's subject gate
		// (rt.supportsSubject) would accept if the planner ever named
		// one. It is aspirational, not reachable this release: planSystem
		// below never offers "subject" in the planner's JSON schema,
		// because the planner has no list of candidate containers,
		// stacks, users or groups to choose from — without one it would
		// have to invent an identifier, which matches nothing and
		// silently narrows the report to empty. Left declared here,
		// deliberately unused, so the gate needs no change once a real
		// candidate list exists to enable this for real.
		Subjects: []SubjectKind{SubjectContainer, SubjectStack, SubjectUser, SubjectGroup},
	}
}

const planSystem = "You plan an IT infrastructure report. Given a question and a menu of " +
	"data sources, reply with ONLY a JSON object: {\"sources\":[string],\"days\":int," +
	"\"environment_id\":string,\"levels\":[\"strategic\"|\"tactical\"|\"operational\"]," +
	"\"title\":string,\"caps\":{source:int}}. Choose the fewest sources that can answer " +
	"the question. `days` is the reporting window. `title` is a short report title. " +
	"`caps` is how many rows of each source you need, at most 200. No prose, no markdown."

// planAdHoc runs pass 1. One retry on an unparseable reply, then a hard
// error: a silently different report is worse than a failed one.
//
// connectionID is the host-chosen AI connection for this generation
// (Generate has already validated it against host.AI.Connections()
// before calling in). It is threaded into every aiComplete call here and
// stamped onto the returned plan's AIConnectionID so a later replay
// pins the same AI — see AdHocPlan's doc comment.
func (s *service) planAdHoc(ctx context.Context, prompt string, envs []envRow, digest map[string]any, connectionID string) (AdHocPlan, error) {
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()

	var availableAIConnections map[string]bool
	if s.host.AI != nil {
		availableAIConnections = connectionIDSet(s.host.AI.Connections())
	}

	menu, err := json.Marshal(sourceCatalog())
	if err != nil {
		return AdHocPlan{}, fmt.Errorf("reporting: encode source menu: %w", err)
	}
	envList := make([]map[string]string, 0, len(envs))
	for _, e := range envs {
		envList = append(envList, map[string]string{"id": e.ID, "name": e.Name})
	}
	envJSON, err := json.Marshal(envList)
	if err != nil {
		return AdHocPlan{}, fmt.Errorf("reporting: encode environments: %w", err)
	}
	digestJSON, err := json.Marshal(digest)
	if err != nil {
		return AdHocPlan{}, fmt.Errorf("reporting: encode digest: %w", err)
	}

	user := "QUESTION:\n" + prompt +
		"\n\nSOURCES:\n" + string(menu) +
		"\n\nENVIRONMENTS:\n" + string(envJSON) +
		"\n\nCURRENT SUMMARY:\n" + string(digestJSON)

	var lastErr error
	// retryHint is what the second attempt is told went wrong. It starts
	// as the JSON-decode message because that is the only failure a
	// transport error (attempt never reaches decode) or a genuinely
	// malformed reply can produce; it is swapped for validate()'s actual
	// complaint below so a retry after "no usable source" does not tell
	// the model its JSON was invalid, which is false and steers it away
	// from the real problem.
	retryHint := "Your previous reply was not valid JSON. Reply with the object only."
	for attempt := 0; attempt < 2; attempt++ {
		msg := user
		if attempt > 0 {
			msg = user + "\n\n" + retryHint
		}
		raw, err := s.aiComplete(ctx, planSystem, msg, 1024, connectionID)
		if err != nil {
			lastErr = err
			continue
		}
		var plan AdHocPlan
		if err := json.Unmarshal([]byte(extractJSON(raw)), &plan); err != nil {
			lastErr = fmt.Errorf("%w: %v", errPlanFailed, err)
			retryHint = "Your previous reply was not valid JSON. Reply with the object only."
			continue
		}
		// Host authority: overwrite whatever the model's reply carried
		// (planSystem never offers this key, but a reply is untrusted
		// input) with the connection the host actually resolved for this
		// request. The model must never pick its own provider.
		plan.AIConnectionID = connectionID
		validated, err := plan.validate(envs, availableAIConnections, s.host.Logger)
		if err != nil {
			lastErr = err
			// validate()'s only failure mode is an empty surviving
			// "sources" set (see validate's doc comment) — every named
			// source was unknown, or none were named at all.
			retryHint = "Your previous reply named no source the SOURCES menu offers, so nothing usable " +
				"survived validation. Reply with the object only, and set \"sources\" to one or more of " +
				"the \"key\" values listed in the SOURCES menu above."
			continue
		}
		if validated.Title == "" {
			validated.Title = clampText(prompt, 60, "Ad-hoc report")
		}
		return validated, nil
	}
	if lastErr == nil {
		lastErr = errors.New("reporting: ad-hoc planner returned no response")
	}
	return AdHocPlan{}, fmt.Errorf("%w: %v", errPlanFailed, lastErr)
}
