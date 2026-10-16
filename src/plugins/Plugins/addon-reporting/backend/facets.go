// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

import (
	"context"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/therealmcsparrow/mcharbor/core/plugin"
)

// Cross-platform facets.
//
// A facet is one question every platform answers in one row shape — what
// runs, what it is allocated against, whether it is recoverable, whether
// it is up, what is reachable, what it consumes. The platform plugins
// answer through the host's reporting family; this plugin answers for
// Docker itself, from the snapshot it already collected, so a facet table
// lists a container, a VM and a pod side by side under one set of
// columns.
//
// Nothing here knows which platform plugins exist. Platforms arrive as
// data, in the order the host registered them, and one that is not
// installed is simply absent from the table.

// facetPlatformDocker is the one platform this plugin answers for itself.
const facetPlatformDocker = "docker"

// facetSourcePrefix marks an ad-hoc source key that names one
// (facet, platform) pair: "facet.inventory.docker".
const facetSourcePrefix = "facet."

// facetPlatform is one platform's answer to one facet.
type facetPlatform struct {
	Platform string
	Label    string
	// Provider is the contributing plugin's slug; empty for Docker, which
	// this plugin answers itself.
	Provider string
	OK       bool
	Note     string
	Result   plugin.ReportFacetResult
}

// facetData is everything collected for one facet.
type facetData struct {
	Facet     string
	Platforms []facetPlatform
	// Note is set when no platform plugin could be asked at all — no
	// reporting bridge, or nothing declares the facet — so the report says
	// "Docker only" rather than implying the other platforms are empty.
	Note string
}

// facetDockerSources are the snapshot sources the Docker projection of
// each facet reads. A report that takes a facet collects these too, so the
// Docker rows are never missing merely because the report's own source
// list did not happen to need them.
var facetDockerSources = map[string][]string{
	plugin.ReportFacetInventory:  {srcContainers},
	plugin.ReportFacetCapacity:   {srcDockerInfo, srcMetrics, srcCapacity},
	plugin.ReportFacetProtection: {srcContainers, srcBackups},
	plugin.ReportFacetHealth:     {srcContainers, srcUptime},
	plugin.ReportFacetExposure:   {srcContainers},
	plugin.ReportFacetCost:       {srcContainers, srcContainerMetrics},
}

// facetHandle is the host's facet consumer handle, or nil when the host
// wired no reporting bridge or was built before facets existed.
func facetHandle() plugin.ReportFacetHandle {
	if providerHost == nil {
		return nil
	}
	h, ok := providerHost.(plugin.ReportFacetHandle)
	if !ok {
		return nil
	}
	return h
}

// facetProviders lists the platform plugins declaring facet, never Docker.
// A provider claiming the Docker platform is ignored: this plugin owns it,
// and printing two sets of container rows would double every total.
func facetProviders(facet string) []plugin.ReportFacetProvider {
	h := facetHandle()
	if h == nil {
		return nil
	}
	var out []plugin.ReportFacetProvider
	for _, p := range h.FacetProviders(facet) {
		if strings.TrimSpace(p.Decl.Platform) == "" || p.Decl.Platform == facetPlatformDocker {
			continue
		}
		out = append(out, p)
	}
	return out
}

// facetRequest is the request handed to every provider for one report. It
// carries the cookie only: plugin.ReportFacetRequest has no Authorization
// field, and providers authorise from UserID rather than replaying the
// caller's credentials.
func facetRequest(in GenerateInput, period Period) plugin.ReportFacetRequest {
	return plugin.ReportFacetRequest{
		EnvironmentID: in.EnvironmentID,
		Environment:   in.Environment,
		PeriodStart:   period.Start,
		PeriodEnd:     period.End,
		Cookie:        in.Cookie,
		Lang:          in.Lang,
		User:          in.User,
		UserID:        in.UserID,
	}
}

// reportFacets is every facet a report type reads: its declared facets
// plus any facet an ad-hoc plan named through a "facet.<f>.<p>" source.
func reportFacets(rt ReportType) []string {
	seen := map[string]bool{}
	var out []string
	add := func(f string) {
		if isFacet(f) && !seen[f] {
			seen[f] = true
			out = append(out, f)
		}
	}
	for _, f := range rt.Facets {
		add(f)
	}
	for _, s := range rt.Sources {
		if f, _, ok := parseFacetSource(s); ok {
			add(f)
		}
	}
	return out
}

// isFacet reports whether f is one of the closed facet set.
func isFacet(f string) bool {
	for _, k := range allFacets() {
		if k == f {
			return true
		}
	}
	return false
}

// allFacets is the closed set in print order.
func allFacets() []string {
	return []string{
		plugin.ReportFacetInventory, plugin.ReportFacetCapacity, plugin.ReportFacetProtection,
		plugin.ReportFacetHealth, plugin.ReportFacetExposure, plugin.ReportFacetCost,
	}
}

// facetSourceKey names one (facet, platform) pair as an ad-hoc source.
func facetSourceKey(facet, platform string) string {
	return facetSourcePrefix + facet + "." + platform
}

// parseFacetSource splits "facet.<facet>.<platform>".
func parseFacetSource(key string) (string, string, bool) {
	if !strings.HasPrefix(key, facetSourcePrefix) {
		return "", "", false
	}
	parts := strings.SplitN(strings.TrimPrefix(key, facetSourcePrefix), ".", 2)
	if len(parts) != 2 || !isFacet(parts[0]) || parts[1] == "" {
		return "", "", false
	}
	return parts[0], parts[1], true
}

// withFacetSources returns sources plus the Docker sources each facet's
// projection needs, without duplicates and in a stable order.
func withFacetSources(sources, facets []string) []string {
	seen := make(map[string]bool, len(sources))
	out := make([]string, 0, len(sources)+4)
	for _, s := range sources {
		if !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	for _, f := range facets {
		for _, s := range facetDockerSources[f] {
			if !seen[s] {
				seen[s] = true
				out = append(out, s)
			}
		}
	}
	return out
}

// collectFacets asks every declaring platform for each facet and adds this
// plugin's own Docker answer.
//
// A provider that fails is recorded as a not-OK source with a one-line
// note and never fails the document. The provider's error text is not
// used for that note: it can carry a hostname or a path, and the annex is
// circulated.
//
// narrowed is true when the report covers one object: the Docker rows are
// already narrowed to it, and another platform's estate would be out of
// scope, so no platform plugin is asked.
func collectFacets(ctx context.Context, snap *Snapshot, facets []string, req plugin.ReportFacetRequest, narrowed bool) {
	if len(facets) == 0 {
		return
	}
	ctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()

	h := facetHandle()
	var (
		mu      sync.Mutex
		wg      sync.WaitGroup
		results = make(map[string]*facetData, len(facets))
		sources []SourceStatus
	)
	for _, facet := range facets {
		wg.Add(1)
		go func(facet string) {
			defer wg.Done()
			data := &facetData{Facet: facet}
			data.Platforms = append(data.Platforms, dockerFacet(snap, facet, req))

			var contribs []plugin.ReportFacetContribution
			switch {
			case narrowed:
				data.Note = "This report is narrowed to one object, so only Docker is covered."
			case h == nil:
				data.Note = "No platform plugin could be asked on this installation, so only Docker is covered."
			case len(facetProviders(facet)) == 0:
				data.Note = "No installed platform plugin contributes this view, so only Docker is covered."
			default:
				r := req
				r.Facet = facet
				contribs = h.CollectFacet(ctx, facet, r)
			}

			var own []SourceStatus
			for _, c := range contribs {
				p, st, extra, keep := facetFromContribution(facet, c)
				if !keep {
					continue
				}
				data.Platforms = append(data.Platforms, p)
				own = append(own, st)
				own = append(own, extra...)
			}
			mu.Lock()
			results[facet] = data
			sources = append(sources, own...)
			mu.Unlock()
		}(facet)
	}
	wg.Wait()

	if snap.Facets == nil {
		snap.Facets = map[string]*facetData{}
	}
	for k, v := range results {
		snap.Facets[k] = v
	}
	sort.SliceStable(sources, func(i, j int) bool { return sources[i].Key < sources[j].Key })
	snap.Sources = append(snap.Sources, sources...)
}

// facetFromContribution converts one provider outcome. keep is false for a
// contribution that names no platform, or one claiming Docker.
func facetFromContribution(facet string, c plugin.ReportFacetContribution) (facetPlatform, SourceStatus, []SourceStatus, bool) {
	platform := strings.ToLower(strings.TrimSpace(c.Provider.Decl.Platform))
	if platform == "" || platform == facetPlatformDocker {
		return facetPlatform{}, SourceStatus{}, nil, false
	}
	label := platformLabel(c.Provider)
	p := facetPlatform{Platform: platform, Label: label, Provider: c.Provider.Slug, OK: c.Err == nil}
	st := SourceStatus{
		Key:   "facet_" + facet + "_" + platform,
		Label: label + " " + facetNoun(facet),
		OK:    c.Err == nil,
	}
	if c.Err != nil {
		p.Note = "the platform did not answer"
		st.Note = p.Note
		return p, st, nil, true
	}
	p.Result = c.Result
	stampPlatform(&p.Result, platform)
	var extra []SourceStatus
	for _, s := range providerSources(c.Result.Sources) {
		s.Key = "facet_" + facet + "_" + platform + "_" + s.Key
		extra = append(extra, s)
		if !s.OK {
			// A provider that answered but was refused its own data (the
			// caller lacks the platform's view key) is reported as withheld,
			// not as an empty platform.
			p.Note = orDash(s.Note, "partly unavailable")
		}
	}
	return p, st, extra, true
}

// stampPlatform makes every row carry its platform, whatever the provider
// wrote, so a provider cannot put rows under another platform's name.
func stampPlatform(r *plugin.ReportFacetResult, platform string) {
	for i := range r.Inventory {
		r.Inventory[i].Platform = platform
	}
	for i := range r.Capacity {
		r.Capacity[i].Platform = platform
	}
	for i := range r.Protection {
		r.Protection[i].Platform = platform
	}
	for i := range r.Health {
		r.Health[i].Platform = platform
	}
	for i := range r.Exposure {
		r.Exposure[i].Platform = platform
	}
	for i := range r.Cost {
		r.Cost[i].Platform = platform
	}
}

// platformLabel is how a document names a contributed platform. The label
// key is the provider's i18n key and documents are English, so the
// provider's display name is used, falling back to the token itself.
func platformLabel(p plugin.ReportFacetProvider) string {
	return orDash(strings.TrimSpace(p.DisplayName), titleCase(p.Decl.Platform))
}

// facetNoun is the facet as a reader says it.
func facetNoun(facet string) string {
	switch facet {
	case plugin.ReportFacetInventory:
		return "inventory"
	case plugin.ReportFacetCapacity:
		return "capacity"
	case plugin.ReportFacetProtection:
		return "protection"
	case plugin.ReportFacetHealth:
		return "health"
	case plugin.ReportFacetExposure:
		return "exposure"
	case plugin.ReportFacetCost:
		return "consumption"
	}
	return facet
}

// facet returns the collected data for one facet, or nil.
func (s *Snapshot) facet(f string) *facetData {
	if s == nil || s.Facets == nil {
		return nil
	}
	return s.Facets[f]
}

// platformNames lists the platforms that answered, in table order.
func (d *facetData) platformNames() []string {
	if d == nil {
		return nil
	}
	var out []string
	for _, p := range d.Platforms {
		if p.OK {
			out = append(out, p.Label)
		}
	}
	return out
}

// platform returns one platform's answer, or nil.
func (d *facetData) platform(token string) *facetPlatform {
	if d == nil {
		return nil
	}
	for i := range d.Platforms {
		if d.Platforms[i].Platform == token {
			return &d.Platforms[i]
		}
	}
	return nil
}

// firstNonEmpty is orDash without the dash, for values stored on a row
// rather than printed: an empty field must stay empty so a table can
// decide how to show it.
func firstNonEmpty(candidates ...string) string {
	for _, c := range candidates {
		if c = strings.TrimSpace(c); c != "" {
			return c
		}
	}
	return ""
}
