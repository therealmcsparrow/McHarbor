// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package pluginruntime

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"sync"
	"time"

	"github.com/therealmcsparrow/mcharbor/core/plugin"
	"github.com/therealmcsparrow/mcharbor/core/plugin/specs/reporting"
)

const actReportingFacet = "plugin.adapter.reporting.facet"

var reFacetPlatform = regexp.MustCompile(`^[a-z][a-z0-9_-]{0,31}$`)

// validateFacetDecls checks a spec's facets before anything is stored, and
// returns the client's contributor half when facets are declared. A facet
// declared by a client that cannot answer it would print an empty platform
// forever, so that is refused here rather than discovered in a document.
func validateFacetDecls(spec plugin.ReportProviderSpec, client plugin.ReportProviderClient) (plugin.ReportFacetContributor, error) {
	if len(spec.Facets) == 0 {
		return nil, nil
	}
	contributor, ok := client.(plugin.ReportFacetContributor)
	if !ok {
		return nil, fmt.Errorf("reporting adapter %s: declares facets but its client does not implement ContributeFacet", spec.Slug)
	}
	seen := map[string]bool{}
	for _, f := range spec.Facets {
		if !reporting.IsFacet(f.Facet) {
			return nil, fmt.Errorf("reporting adapter %s: facet %q is not one of %v", spec.Slug, f.Facet, reporting.Facets())
		}
		if !reFacetPlatform.MatchString(f.Platform) {
			return nil, fmt.Errorf("reporting adapter %s: facet %s: platform %q must match %s", spec.Slug, f.Facet, f.Platform, reFacetPlatform)
		}
		key := facetOwnerKey(f.Facet, f.Platform)
		if seen[key] {
			return nil, fmt.Errorf("reporting adapter %s: facet %s declared twice", spec.Slug, key)
		}
		seen[key] = true
	}
	return contributor, nil
}

func facetOwnerKey(facet, platform string) string { return facet + "/" + platform }

// facetOwnersFree refuses a (facet, platform) pair another provider already
// owns. Callers hold a.mu.
func (a *adapterRegistry) facetOwnersFree(spec plugin.ReportProviderSpec) error {
	for _, f := range spec.Facets {
		key := facetOwnerKey(f.Facet, f.Platform)
		if owner, taken := a.reportingFacetOwner[key]; taken {
			return fmt.Errorf("reporting adapter %s: facet %s already registered by %s", spec.Slug, key, owner)
		}
	}
	return nil
}

// storeFacets records a validated spec's facets. Callers hold a.mu.
func (a *adapterRegistry) storeFacets(spec plugin.ReportProviderSpec, client plugin.ReportFacetContributor) {
	if client == nil || len(spec.Facets) == 0 {
		return
	}
	if a.reportingFacetClients == nil {
		a.reportingFacetClients = make(map[string]plugin.ReportFacetContributor)
	}
	if a.reportingFacetOwner == nil {
		a.reportingFacetOwner = make(map[string]string)
	}
	a.reportingFacetClients[spec.Slug] = client
	for _, f := range spec.Facets {
		a.reportingFacetOwner[facetOwnerKey(f.Facet, f.Platform)] = spec.Slug
	}
}

// pruneFacets drops an uninstalled provider's facets. Callers hold a.mu.
func (a *adapterRegistry) pruneFacets(slug string) {
	delete(a.reportingFacetClients, slug)
	for key, owner := range a.reportingFacetOwner {
		if owner == slug {
			delete(a.reportingFacetOwner, key)
		}
	}
}

// facetProviders lists the declarations for one facet in registration order,
// with each owner's contributor.
func (a *adapterRegistry) facetProviders(facet string) ([]plugin.ReportFacetProvider, []plugin.ReportFacetContributor) {
	a.mu.RLock()
	defer a.mu.RUnlock()
	var provs []plugin.ReportFacetProvider
	var clients []plugin.ReportFacetContributor
	for _, slug := range a.reportingOrder {
		spec, ok := a.reportingSpecMap[slug]
		if !ok {
			continue
		}
		cli := a.reportingFacetClients[slug]
		if cli == nil {
			continue
		}
		for _, d := range spec.Facets {
			if d.Facet != facet {
				continue
			}
			provs = append(provs, plugin.ReportFacetProvider{Slug: slug, DisplayName: spec.DisplayName, Decl: d})
			clients = append(clients, cli)
		}
	}
	return provs, clients
}

// FacetProviders implements ReportFacetHandle.
func (a *reportingAdapter) FacetProviders(facet string) []plugin.ReportFacetProvider {
	provs, _ := a.reg.facetProviders(facet)
	return provs
}

// CollectFacet implements ReportFacetHandle. Providers run concurrently
// because each one is an agent or cluster round trip; the slowest provider
// sets the wall clock rather than their sum.
func (a *reportingAdapter) CollectFacet(ctx context.Context, facet string, req plugin.ReportFacetRequest) []plugin.ReportFacetContribution {
	provs, clients := a.reg.facetProviders(facet)
	out := make([]plugin.ReportFacetContribution, len(provs))
	req.Facet = facet
	var wg sync.WaitGroup
	for i := range provs {
		out[i].Provider = provs[i]
		wg.Add(1)
		// goroutine lifecycle: bounded by ctx and joined by wg.Wait below.
		go func(i int) {
			defer wg.Done()
			res, err := safeContribute(ctx, clients[i], req)
			out[i].Result = res
			if err != nil {
				out[i].Err = fmt.Errorf("reporting provider %s facet %s: %w", provs[i].Slug, facet, err)
			}
		}(i)
	}
	wg.Wait()
	return out
}

var errFacetPanic = errors.New("facet provider panicked")

// safeContribute contains a provider panic to that provider's contribution,
// so one broken plugin cannot take down every shared report.
func safeContribute(ctx context.Context, c plugin.ReportFacetContributor, req plugin.ReportFacetRequest) (res plugin.ReportFacetResult, err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("%w: %v", errFacetPanic, r)
		}
	}()
	return c.ContributeFacet(ctx, req)
}

type auditedFacetContributor struct {
	inner plugin.ReportFacetContributor
	aud   *adapterAuditor
	slug  string
	key   string
}

// wrapFacetContributor audits failed facet contributions. Success is not
// audited: every shared report asks every provider, and the Build row of the
// report itself already records the run.
func wrapFacetContributor(aud *adapterAuditor, slug, key string, inner plugin.ReportFacetContributor) plugin.ReportFacetContributor {
	if aud == nil || aud.log == nil || inner == nil {
		return inner
	}
	return &auditedFacetContributor{inner: inner, aud: aud, slug: slug, key: key}
}

func (c *auditedFacetContributor) ContributeFacet(ctx context.Context, req plugin.ReportFacetRequest) (plugin.ReportFacetResult, error) {
	start := time.Now()
	res, err := c.inner.ContributeFacet(ctx, req)
	c.aud.record(ctx, c.slug, actReportingFacet, c.key, start, err, auditOnFailure, map[string]any{
		"facet":          req.Facet,
		"environment_id": req.EnvironmentID,
		"requested_by":   req.User,
	})
	return res, err
}
