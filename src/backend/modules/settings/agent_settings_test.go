// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package settings

import (
	"context"
	"path/filepath"
	"testing"

	coredb "github.com/therealmcsparrow/mcharbor/core/db"
	"github.com/therealmcsparrow/mcharbor/internal/closeutil"
)

func newAgentSettingsService(t *testing.T) *Service {
	t.Helper()
	database, err := coredb.OpenSQLite(filepath.Join(t.TempDir(), "settings.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { closeutil.Report(database.Close(), "database") })
	if err := coredb.Migrate(database, coredb.DriverSQLite); err != nil {
		t.Fatal(err)
	}
	return NewService(database, nil, t.TempDir(), nil, nil, nil, 0)
}

func agentInput(push int) AgentSettingsInput {
	return AgentSettingsInput{EventMode: "poll", EventPollInterval: 30, PingInterval: 30, RequestTimeout: 30, MetricsEnabled: true, StatsPushInterval: push}
}

func TestStatsPushIntervalRoundTrips(t *testing.T) {
	svc := newAgentSettingsService(t)
	if got := svc.AgentSettings(context.Background()).StatsPushInterval; got != 10 {
		t.Fatalf("default push interval = %d, want 10", got)
	}
	if err := svc.UpdateAgentSettings(context.Background(), agentInput(60)); err != nil {
		t.Fatal(err)
	}
	if got := svc.AgentSettings(context.Background()).StatsPushInterval; got != 60 {
		t.Fatalf("stored push interval = %d, want 60", got)
	}
	if err := svc.UpdateAgentSettings(context.Background(), agentInput(0)); err != nil {
		t.Fatal(err)
	}
	if got := svc.AgentSettings(context.Background()).StatsPushInterval; got != 60 {
		t.Fatalf("a client that omits the field reset the interval to %d", got)
	}
}

func TestStatsPushIntervalIsBounded(t *testing.T) {
	svc := newAgentSettingsService(t)
	for _, bad := range []int{1, 4, 301, 3600} {
		if err := svc.UpdateAgentSettings(context.Background(), agentInput(bad)); err == nil {
			t.Errorf("push interval %d was accepted", bad)
		}
	}
}
