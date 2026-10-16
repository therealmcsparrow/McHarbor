// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package scans

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	plugin "github.com/therealmcsparrow/mcharbor/core/plugin"
)

// scannerTestTimeout bounds one Test invocation.
//
// Shorter than a scan's timeout on purpose: a test is something an
// operator sits and waits for, and a probe that takes as long as a
// full image scan is one they will assume has hung.
const scannerTestTimeout = 30 * time.Second

// TestStep is a single named phase of a scanner test. Mirrors
// communications.TestStep and storage_locations.TestStep so the same
// frontend modal pattern renders all three. Status is one of
// "pass" / "warn" / "fail" / "skip"; the overall status is the worst
// step status.
type TestStep struct {
	Name    string `json:"name"`
	Status  string `json:"status"`
	Detail  string `json:"detail,omitempty"`
	Latency string `json:"latency,omitempty"`
}

// ScannerTestResult is the JSON shape returned by
// POST /api/scans/configs/{id}/test.
//
// The host records the phases it controls — load, config, plugin
// lookup, availability — and the adapter contributes whatever it can
// check about itself. The split matters: "the row is fine and the
// plugin is installed but the binary is missing" is three passes and a
// fail, and an operator reading that knows to rebuild the image rather
// than re-enter their config.
type ScannerTestResult struct {
	ScannerID     string     `json:"scannerId"`
	ScannerName   string     `json:"scannerName"`
	ScannerType   string     `json:"scannerType"`
	OverallStatus string     `json:"overallStatus"`
	StartedAt     time.Time  `json:"startedAt"`
	CompletedAt   time.Time  `json:"completedAt"`
	Duration      string     `json:"duration"`
	Steps         []TestStep `json:"steps"`
}

// worstStepStatus returns the most severe status among steps.
// "fail" > "warn" > "skip" > "pass". Empty list → "skip".
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
	if len(steps) == 0 {
		return "skip"
	}
	return worst
}

// stepRecord tracks one phase's start time so the result can report
// per-step latency.
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

// TestScanner exercises one configured scanner and returns a per-step
// report.
//
// It never returns an error for a *failing* test — only for a request
// that could not be answered at all (no such row). A scanner that is
// disabled, whose plugin is gone, or whose binary is missing is a
// successful diagnosis, and the caller renders it. That is the same
// contract TestChannel and TestStorageLocation use, and the reason
// their modals can show a failure instead of a toast.
func (s *ConfigService) TestScanner(ctx context.Context, id string) (*ScannerTestResult, error) {
	startedAt := time.Now()

	row, err := s.readRow(ctx, id)
	if err != nil {
		return nil, err
	}

	steps := make([]TestStep, 0, 6)
	steps = append(steps, TestStep{
		Name:   "load",
		Status: "pass",
		Detail: fmt.Sprintf("scanner %s (%s)", row.name, row.scannerType),
	})

	// A disabled scanner is skipped rather than failed. It is not
	// broken — the operator switched it off — and colouring that red
	// would train them to ignore red.
	if !row.enabled {
		steps = append(steps, TestStep{
			Name:   "config",
			Status: "skip",
			Detail: "scanner is disabled — enable it to use it for scans",
		})
		return finishScannerTest(row, startedAt, steps), nil
	}

	pluginStart := newStep("plugin")
	spec, client, ok := s.spec(row.scannerType)
	if !ok {
		steps = pluginStart.finish(steps, "fail",
			fmt.Sprintf("no plugin registered for %q — install the matching scanner-%s plugin from the App Store",
				row.scannerType, row.scannerType))
		return finishScannerTest(row, startedAt, steps), nil
	}
	steps = pluginStart.finish(steps, "pass",
		fmt.Sprintf("%s adapter resolved", spec.DisplayName))

	// Required-field check runs after the plugin lookup because the
	// schema that says which fields are required is the adapter's.
	configStart := newStep("config")
	cfg, err := s.resolvedConfig(spec, row)
	if err != nil {
		// Decryption failed: the encryption key changed or the row is
		// corrupt. The underlying error can name the field, so it goes
		// to the log and the operator gets the actionable summary.
		s.logTestError("resolving scanner config", id, row.scannerType, err)
		steps = configStart.finish(steps, "fail",
			"stored credentials could not be decrypted — the encryption key may have changed")
		return finishScannerTest(row, startedAt, steps), nil
	}
	if missing := missingRequiredFields(spec, cfg); len(missing) > 0 {
		steps = configStart.finish(steps, "fail",
			fmt.Sprintf("required field(s) empty: %s", strings.Join(missing, ", ")))
		return finishScannerTest(row, startedAt, steps), nil
	}
	steps = configStart.finish(steps, "pass",
		fmt.Sprintf("%d field(s) configured", len(spec.ConfigSchema)))

	availStart := newStep("availability")
	if !client.Available(ctx, cfg) {
		// The two reasons are genuinely different repairs, so the
		// detail says which one this adapter is.
		detail := "the engine reports it cannot run — check its configuration"
		if spec.Capabilities.RequiresLocalBinary {
			detail = fmt.Sprintf("the %s binary was not found on PATH in this container", row.scannerType)
		}
		steps = availStart.finish(steps, "fail", detail)
		return finishScannerTest(row, startedAt, steps), nil
	}
	steps = availStart.finish(steps, "pass", "the engine is ready to run")

	// Adapters that declared SupportsTest=false are not asked. Saying
	// "passed" for a check that never ran would be the one thing this
	// modal exists to prevent.
	if !spec.Capabilities.SupportsTest {
		steps = append(steps, TestStep{
			Name:   "engine",
			Status: "skip",
			Detail: "this engine does not implement a self-test",
		})
		return finishScannerTest(row, startedAt, steps), nil
	}

	testCtx, cancel := context.WithTimeout(ctx, scannerTestTimeout)
	defer cancel()
	steps = append(steps, s.runAdapterTest(testCtx, row, spec, client, cfg)...)

	return finishScannerTest(row, startedAt, steps), nil
}

// runAdapterTest asks the adapter to check itself.
//
// An adapter implementing RicherScannerTester reports its own phases;
// one that implements only Test yields a single `engine` step. The
// type assertion is the whole discovery mechanism, so an adapter that
// never heard of the richer interface keeps working unchanged.
func (s *ConfigService) runAdapterTest(
	ctx context.Context,
	row scannerRow,
	spec plugin.ImageScannerAdapterSpec,
	client plugin.ImageScannerAdapterClient,
	cfg map[string]string,
) []TestStep {
	if rich, ok := client.(plugin.RicherScannerTester); ok {
		start := time.Now()
		pluginSteps, err := rich.TestSteps(ctx, cfg)
		if err != nil {
			s.logTestError("adapter TestSteps", row.id, row.scannerType, err)
			return []TestStep{{
				Name:    "engine",
				Status:  "fail",
				Detail:  "the engine's self-test failed; check the server log",
				Latency: time.Since(start).Round(time.Millisecond).String(),
			}}
		}
		if len(pluginSteps) == 0 {
			// An adapter that declares the interface and returns
			// nothing has a bug. Say so rather than rendering a gap.
			return []TestStep{{
				Name:    "engine",
				Status:  "warn",
				Detail:  "the engine reported no test steps",
				Latency: time.Since(start).Round(time.Millisecond).String(),
			}}
		}
		out := make([]TestStep, 0, len(pluginSteps))
		for _, st := range pluginSteps {
			out = append(out, TestStep{
				Name:    st.Name,
				Status:  normalizeStepStatus(st.Status),
				Detail:  st.Detail,
				Latency: st.Latency,
			})
		}
		return out
	}

	engineStart := newStep("engine")
	if err := client.Test(ctx, cfg); err != nil {
		s.logTestError("adapter Test", row.id, row.scannerType, err)
		return engineStart.finish(nil, "fail",
			"the engine rejected the test; check the server log")
	}
	return engineStart.finish(nil, "pass",
		fmt.Sprintf("%s responded successfully", spec.DisplayName))
}

// missingRequiredFields lists the adapter-declared required keys that
// are empty in the resolved config.
func missingRequiredFields(spec plugin.ImageScannerAdapterSpec, cfg map[string]string) []string {
	var missing []string
	for _, decl := range spec.ConfigSchema {
		if !decl.Required {
			continue
		}
		if strings.TrimSpace(cfg[decl.Key]) == "" {
			missing = append(missing, decl.Key)
		}
	}
	return missing
}

// normalizeStepStatus folds an adapter's status onto the four the UI
// renders. An unrecognised value becomes "warn" rather than being
// dropped or silently treated as a pass: a step the host cannot
// interpret is exactly the thing an operator should look at.
func normalizeStepStatus(status string) string {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "pass", "fail", "warn", "skip":
		return strings.ToLower(strings.TrimSpace(status))
	default:
		return "warn"
	}
}

// logTestError records the real error server-side. Nothing from it
// reaches the operator — an engine's error text can carry a registry
// credential or a Clair URL with an embedded token.
func (s *ConfigService) logTestError(stage, id, scannerType string, err error) {
	if errors.Is(err, context.DeadlineExceeded) {
		slog.Warn("scans: scanner test timed out",
			"stage", stage, "scanner_id", id, "scanner_type", scannerType)
		return
	}
	slog.Error("scans: scanner test failed",
		"stage", stage, "scanner_id", id, "scanner_type", scannerType, "error", err)
}

func finishScannerTest(row scannerRow, startedAt time.Time, steps []TestStep) *ScannerTestResult {
	completedAt := time.Now()
	return &ScannerTestResult{
		ScannerID:     row.id,
		ScannerName:   row.name,
		ScannerType:   row.scannerType,
		OverallStatus: worstStepStatus(steps),
		StartedAt:     startedAt,
		CompletedAt:   completedAt,
		Duration:      completedAt.Sub(startedAt).Round(time.Millisecond).String(),
		Steps:         steps,
	}
}
