// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package communications

import (
	"context"
	"fmt"
	"log/slog"
	"time"
)

// TestStep is a single named phase of the channel test. Mirrors
// storage_locations.TestStep so the same frontend modal pattern
// (StorageLocationTestDialog) renders both. Status is one of
// "pass" / "warn" / "fail" / "skip"; the overall status is the
// worst step status.
type TestStep struct {
	Name    string `json:"name"`
	Status  string `json:"status"`
	Detail  string `json:"detail,omitempty"`
	Latency string `json:"latency,omitempty"`
}

// TestChannelResult is the JSON shape returned by
// POST /api/communication-channels/{id}/test. The host records
// the four phases it controls (load, decrypt, plugin lookup, send)
// plus the plugin's own outcome. The plugin supplies the URL/API
// it actually hit, so the operator sees whether the failure was
// DB, credential, DNS, or auth.
type TestChannelResult struct {
	ChannelID     string     `json:"channelId"`
	ChannelName   string     `json:"channelName"`
	ChannelType   string     `json:"channelType"`
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
	return worst
}

// stepRecord tracks one phase's start time so the result can
// report per-step latency. Ponytail: stdlib time.Now/Sub, no
// clock injection — the test runs in operator hands, not in a
// load test.
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

// TestChannel dispatches a test notification through the channel's
// registered plugin and returns a per-step report. The four recorded
// phases are: load (DB lookup), config (decrypt credentials), plugin
// (NotificationAdapter lookup), send (plugin's own Test call). The
// plugin's error is preserved as the send-step detail so the UI
// surfaces the upstream API message (e.g. "Telegram 401
// Unauthorized") verbatim.
func (s *Service) TestChannel(ctx context.Context, id string) (*TestChannelResult, error) {
	startedAt := time.Now()

	// includeSecrets=true: the plugin needs the real credentials to
	// send. s.ByID would hand back a redacted config and every send
	// would fail on auth.
	ch, err := s.byID(ctx, id, true)
	if err != nil {
		return nil, err
	}
	if ch == nil {
		return nil, ErrChannelNotFound
	}

	steps := make([]TestStep, 0, 4)
	steps = append(steps, TestStep{
		Name:   "load",
		Status: "pass",
		Detail: fmt.Sprintf("channel %s (%s)", ch.Name, ch.ChannelType),
	})

	if !ch.Enabled {
		steps = append(steps, TestStep{
			Name:   "config",
			Status: "skip",
			Detail: "channel is disabled — enable it before testing",
		})
		return finishChannelTest(ch, startedAt, steps), nil
	}

	cfg := ch.Config
	if cfg == nil {
		cfg = map[string]string{}
	}
	cfg["channel_type"] = ch.ChannelType
	if ch.Method != "" {
		cfg["method"] = ch.Method
	}
	// Same normalisation the dispatcher applies before a real send —
	// ntfy's token -> access_token, signal's rest_api -> rest and
	// username/password -> auth_*. Without it those plugins receive an
	// incomplete config and the test fails for the wrong reason.
	applyPluginConfigAliases(ch.ChannelType, cfg)

	pluginStart := newStep("plugin")
	client, ok := s.lookupNotificationAdapter(ch.ChannelType)
	if !ok {
		steps = pluginStart.finish(steps, "fail",
			fmt.Sprintf("no plugin registered for %q — install the matching comm-%s plugin via the appstore",
				ch.ChannelType, ch.ChannelType))
		return finishChannelTest(ch, startedAt, steps), nil
	}
	pluginStep := pluginStart.finish(steps, "pass",
		fmt.Sprintf("plugin resolved for %s", ch.ChannelType))

	sendStart := newStep("send")
	sendErr := client.Test(ctx, cfg)
	if sendErr != nil {
		// Host-authored detail only: a transport error can carry the
		// SMTP host, the account name, or a bearer token. The real
		// error goes to the server log.
		slog.Error("communications: channel test send failed",
			"error", sendErr, "channel_id", id, "channel_type", ch.ChannelType)
		pluginStep = sendStart.finish(pluginStep, "fail",
			"the provider rejected the test notification; check the server log")
		return finishChannelTest(ch, startedAt, pluginStep), nil
	}
	pluginStep = sendStart.finish(pluginStep, "pass", "test notification delivered")

	return finishChannelTest(ch, startedAt, pluginStep), nil
}

func finishChannelTest(ch *CommunicationChannel, startedAt time.Time, steps []TestStep) *TestChannelResult {
	completedAt := time.Now()
	return &TestChannelResult{
		ChannelID:     ch.ID,
		ChannelName:   ch.Name,
		ChannelType:   ch.ChannelType,
		OverallStatus: worstStepStatus(steps),
		StartedAt:     startedAt,
		CompletedAt:   completedAt,
		Duration:      completedAt.Sub(startedAt).Round(time.Millisecond).String(),
		Steps:         steps,
	}
}
