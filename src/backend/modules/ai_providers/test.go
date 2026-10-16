// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package ai_providers

import (
	"context"
	"errors"
	"fmt"
	"time"
)

// TestStep is a single named phase of the AI provider connection
// test. Mirrors storage_locations.TestStep and communications.TestStep
// so the same frontend modal pattern renders all three. Status is one
// of "pass" / "warn" / "fail" / "skip"; the overall status mirrors the
// worst step.
type TestStep struct {
	Name    string `json:"name"`
	Status  string `json:"status"`
	Detail  string `json:"detail,omitempty"`
	Latency string `json:"latency,omitempty"`
}

// TestConnectionResult is the JSON shape returned by
// POST /api/ai-providers/{id}/test. The recorded phases are: load
// (DB lookup), decrypt (API key), plugin (AIAdapterClient lookup),
// send (live test prompt). The plugin's own TestResult (success /
// latency / reply / message) is folded into the send step's detail
// so the UI surfaces the upstream model reply verbatim.
type TestConnectionResult struct {
	ConnectionID   string     `json:"connectionId"`
	ConnectionName string     `json:"connectionName"`
	ProviderType   string     `json:"providerType"`
	Model          string     `json:"model,omitempty"`
	OverallStatus  string     `json:"overallStatus"`
	StartedAt      time.Time  `json:"startedAt"`
	CompletedAt    time.Time  `json:"completedAt"`
	Duration       string     `json:"duration"`
	Steps          []TestStep `json:"steps"`
}

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

type stepRecord struct {
	name      string
	startedAt time.Time
}

func newStep(name string) *stepRecord {
	return &stepRecord{name: name, startedAt: time.Now()}
}

func (r *stepRecord) finish(steps []TestStep, status, detail string) []TestStep {
	latency := time.Since(r.startedAt).Round(time.Millisecond).String()
	return append(steps, TestStep{
		Name:    r.name,
		Status:  status,
		Detail:  detail,
		Latency: latency,
	})
}

// TestConnection dispatches a tiny prompt through the connection's
// registered ai-provider-* plugin and returns a per-step report.
// Replaces the old Service.Test that returned a single TestResult —
// the new shape lets the UI render the same per-step modal that
// storage_locations and communications already use, so the operator
// sees which phase failed (decrypt vs plugin lookup vs API call).
func (s *Service) TestConnection(ctx context.Context, id string) (*TestConnectionResult, error) {
	startedAt := time.Now()

	row, err := s.ByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if row == nil {
		return nil, ErrConnectionNotFound
	}

	steps := make([]TestStep, 0, 4)
	steps = append(steps, TestStep{
		Name:   "load",
		Status: "pass",
		Detail: fmt.Sprintf("connection %s (%s)", row.Name, row.ProviderType),
	})

	if !row.Enabled {
		steps = append(steps, TestStep{
			Name:   "decrypt",
			Status: "skip",
			Detail: "connection is disabled — enable it before testing",
		})
		return finishTestConnection(row, startedAt, steps), nil
	}

	decryptStart := newStep("decrypt")
	cfg, err := s.PlaintextConfig(ctx, row)
	if err != nil {
		// An OAuth connection with no token is not a decryption
		// failure -- the operator simply has not signed in yet, and
		// saying so is the difference between one click and a support
		// ticket.
		detail := "stored credential could not be decrypted"
		if errors.Is(err, ErrOAuthNotConnected) {
			detail = "connection is not signed in — use Sign in before testing"
		}
		steps = decryptStart.finish(steps, "fail", detail)
		return finishTestConnection(row, startedAt, steps), nil
	}
	credential := "API key decrypted" //nolint:gosec // G101: step label text, not a credential
	if row.AuthMethod == AuthMethodOAuth {
		credential = "OAuth access token decrypted" //nolint:gosec // G101: step label text, not a credential
	}
	decryptStep := decryptStart.finish(steps, "pass", credential)

	pluginStart := newStep("plugin")
	adapter, ok := s.lookupAIAdapter(row.ProviderType)
	if !ok {
		steps = pluginStart.finish(decryptStep, "fail",
			fmt.Sprintf("no plugin registered for %q — install the matching ai-provider-%s plugin via the appstore",
				row.ProviderType, row.ProviderType))
		return finishTestConnection(row, startedAt, steps), nil
	}
	pluginStep := pluginStart.finish(decryptStep, "pass",
		fmt.Sprintf("plugin resolved for %s", row.ProviderType))

	sendStart := newStep("send")
	result, callErr := adapter.Test(ctx, cfg)
	if callErr != nil {
		pluginStep = sendStart.finish(pluginStep, "fail", "provider request failed; check the server log")
		return finishTestConnection(row, startedAt, pluginStep), nil //nolint:nilerr // provider failure is reported as a failed step in the result, not an error
	}
	switch {
	case result == nil:
		pluginStep = sendStart.finish(pluginStep, "fail", "plugin returned no test result")
	case !result.Success:
		detail := result.Message
		if result.Reply != "" {
			detail = result.Reply + " — " + result.Message
		}
		if detail == "" {
			detail = "plugin reported failure without a message"
		}
		pluginStep = sendStart.finish(pluginStep, "fail", detail)
	case result.Reply != "":
		pluginStep = sendStart.finish(pluginStep, "pass",
			fmt.Sprintf("reply received (%dms): %s", result.LatencyMs, result.Reply))
	default:
		pluginStep = sendStart.finish(pluginStep, "pass",
			fmt.Sprintf("test prompt accepted (%dms)", result.LatencyMs))
	}

	return finishTestConnection(row, startedAt, pluginStep), nil
}

func finishTestConnection(row *AIProviderConnection, startedAt time.Time, steps []TestStep) *TestConnectionResult {
	completedAt := time.Now()
	return &TestConnectionResult{
		ConnectionID:   row.ID,
		ConnectionName: row.Name,
		ProviderType:   row.ProviderType,
		Model:          row.DefaultModel,
		OverallStatus:  worstStepStatus(steps),
		StartedAt:      startedAt,
		CompletedAt:    completedAt,
		Duration:       completedAt.Sub(startedAt).Round(time.Millisecond).String(),
		Steps:          steps,
	}
}
