// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package identity

import (
	"context"
	"fmt"
	"log/slog"
	"sort"
	"strings"
	"time"

	"github.com/therealmcsparrow/mcharbor/core/plugin"
)

// TestStep is a single named phase of the identity provider connection
// test. Mirrors ai_providers.TestStep, storage_locations.TestStep, and
// communications.TestStep so the same frontend modal pattern renders
// all four. Status is one of "pass" / "warn" / "fail" / "skip"; the
// overall status mirrors the worst step.
type TestStep struct {
	Name    string `json:"name"`
	Status  string `json:"status"`
	Detail  string `json:"detail,omitempty"`
	Latency string `json:"latency,omitempty"`
}

// TestConnectionResult is the JSON shape returned by
// POST /api/identity-providers/{id}/test. The recorded phases are:
// load (DB lookup), plugin (IdentityAdapterClient lookup),
// configuration (decrypt + required-field check against the plugin's
// ConfigSchema), connection (the plugin's own Test), and groups (the
// optional FetchGroups probe).
//
// A failed test is still a successful request: the handler answers 200
// with OverallStatus "fail" so the modal can render which phase broke.
type TestConnectionResult struct {
	ProviderID    string     `json:"providerId"`
	ProviderName  string     `json:"providerName"`
	ProviderType  string     `json:"providerType"`
	OverallStatus string     `json:"overallStatus"`
	StartedAt     time.Time  `json:"startedAt"`
	CompletedAt   time.Time  `json:"completedAt"`
	Duration      string     `json:"duration"`
	Steps         []TestStep `json:"steps"`
	// Groups is a sample of directory group names the groups step read, so
	// the setup wizard can preview mappings against real names.
	Groups []string `json:"groups,omitempty"`
}

// maxTestGroupSample bounds the group names a test result carries.
const maxTestGroupSample = 200

// worstStepStatus returns the most severe status among the steps.
// Ordering (most-severe-first): fail > warn > skip > pass.
func worstStepStatus(steps []TestStep) string {
	worst := "pass"
	for _, s := range steps {
		switch s.Status {
		case "fail":
			return "fail"
		case "warn":
			worst = "warn"
		case "skip":
			if worst == "pass" {
				worst = "skip"
			}
		}
	}
	return worst
}

// stepRecord times one phase so each step reports its own latency —
// the point of the modal is telling a slow upstream apart from a fast
// local misconfiguration.
type stepRecord struct {
	name      string
	startedAt time.Time
}

func newStep(name string) *stepRecord {
	return &stepRecord{name: name, startedAt: time.Now()}
}

func (r *stepRecord) finish(steps []TestStep, status, detail string) []TestStep {
	return append(steps, TestStep{
		Name:    r.name,
		Status:  status,
		Detail:  detail,
		Latency: time.Since(r.startedAt).Round(time.Millisecond).String(),
	})
}

// TestConnectionSteps exercises a configured identity provider and
// returns a per-step report. It replaces the old TestConnection, which
// returned a bare error and left the operator guessing whether the
// failure was a missing plugin, an undecryptable secret, or a genuine
// upstream problem.
//
// Details are host-authored. Upstream and plugin errors are logged
// server-side and reported to the client as a generic phrase — an
// identity provider's error text can carry tokens, URLs, and tenant
// identifiers.
func (s *Service) TestConnectionSteps(ctx context.Context, providerID string) (*TestConnectionResult, error) {
	startedAt := time.Now()

	provider, err := s.loadRuntimeProvider(ctx, providerID)
	if err != nil {
		return nil, err
	}

	steps := make([]TestStep, 0, 5)
	steps = append(steps, TestStep{
		Name:   "load",
		Status: "pass",
		Detail: fmt.Sprintf("provider %s (%s)", provider.Name, provider.ProviderType),
	})

	if !provider.Enabled {
		steps = append(steps, TestStep{
			Name:   "plugin",
			Status: "skip",
			Detail: "provider is disabled — enable it before testing",
		})
		return finishTestConnection(provider, startedAt, steps), nil
	}

	pluginStart := newStep("plugin")
	adapter, spec, ok := s.identityAdapter(provider.ProviderType)
	if !ok {
		steps = pluginStart.finish(steps, "fail", fmt.Sprintf(
			"no plugin registered for %q — install or enable the matching identity plugin",
			provider.ProviderType))
		return finishTestConnection(provider, startedAt, steps), nil
	}
	steps = pluginStart.finish(steps, "pass",
		fmt.Sprintf("plugin resolved for %s", provider.ProviderType))

	configStart := newStep("configuration")
	cfg, err := s.providerToConfigMap(ctx, provider)
	if err != nil {
		s.logTestError("building provider config", providerID, err)
		steps = configStart.finish(steps, "fail",
			"stored configuration could not be decrypted; check the server log")
		return finishTestConnection(provider, startedAt, steps), nil
	}
	if missing := missingRequiredKeys(spec.ConfigSchema, cfg); len(missing) > 0 {
		// Key names only — never the values, which are secrets for
		// at least some of these fields.
		steps = configStart.finish(steps, "fail",
			"required fields are empty: "+strings.Join(missing, ", "))
		return finishTestConnection(provider, startedAt, steps), nil
	}
	steps = configStart.finish(steps, "pass",
		fmt.Sprintf("%d configuration keys resolved", len(cfg)))

	connStart := newStep("connection")
	if err := adapter.Test(ctx, cfg); err != nil {
		s.logTestError("identity provider test", providerID, err)
		steps = connStart.finish(steps, "fail",
			"provider rejected the connection; check the server log")
		return finishTestConnection(provider, startedAt, steps), nil
	}
	steps = connStart.finish(steps, "pass", "provider accepted the connection")

	// Group fetch is an optional capability. A provider whose login
	// works but whose directory API is unreachable is degraded, not
	// broken, so this can only warn.
	groupStart := newStep("groups")
	if !spec.Capabilities.SupportsGroupFetch {
		steps = groupStart.finish(steps, "skip",
			"this provider type does not expose a groups API")
		return finishTestConnection(provider, startedAt, steps), nil
	}
	groups, err := adapter.FetchGroups(ctx, cfg)
	if err != nil {
		s.logTestError("identity provider group fetch", providerID, err)
		steps = groupStart.finish(steps, "warn",
			"group fetch failed; sign-in still works but group mapping will not")
		return finishTestConnection(provider, startedAt, steps), nil
	}
	steps = groupStart.finish(steps, "pass",
		fmt.Sprintf("%d groups readable", len(groups)))

	result := finishTestConnection(provider, startedAt, steps)
	for _, g := range groups {
		if len(result.Groups) >= maxTestGroupSample {
			break
		}
		if name := strings.TrimSpace(g.Name); name != "" {
			result.Groups = append(result.Groups, name)
		}
	}
	sort.Strings(result.Groups)
	return result, nil
}

// missingRequiredKeys returns the names of the plugin's required
// config fields that resolved to empty, sorted so the message is
// stable across runs (Go map iteration is not).
func missingRequiredKeys(schema []plugin.ConfigFieldDecl, cfg map[string]string) []string {
	var missing []string
	for _, field := range schema {
		if !field.Required {
			continue
		}
		if strings.TrimSpace(cfg[field.Key]) == "" {
			missing = append(missing, field.Key)
		}
	}
	sort.Strings(missing)
	return missing
}

func finishTestConnection(provider runtimeProvider, startedAt time.Time, steps []TestStep) *TestConnectionResult {
	completedAt := time.Now()
	return &TestConnectionResult{
		ProviderID:    provider.ID,
		ProviderName:  provider.Name,
		ProviderType:  provider.ProviderType,
		OverallStatus: worstStepStatus(steps),
		StartedAt:     startedAt.UTC(),
		CompletedAt:   completedAt.UTC(),
		Duration:      completedAt.Sub(startedAt).Round(time.Millisecond).String(),
		Steps:         steps,
	}
}

// logTestError records the real error server-side. The client only
// ever sees the host-authored step detail. Uses the package-level slog
// default, matching the rest of this module.
func (s *Service) logTestError(what, providerID string, err error) {
	slog.Error(what+" failed", "error", err, "provider_id", providerID)
}
