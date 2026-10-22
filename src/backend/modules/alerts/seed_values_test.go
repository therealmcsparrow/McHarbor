// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package alerts

import (
	"context"
	"encoding/json"
	"testing"
)

func TestSeededRulesCarryTheAgreedThresholds(t *testing.T) {
	db := openSeedDB(t)
	if err := NewService(db).SeedDefaultRules(context.Background()); err != nil {
		t.Fatal(err)
	}
	want := map[string]struct {
		severity, key string
		threshold     float64
		forSeconds    int
	}{
		"Host disk usage warning":  {"warning", conditionHostFSPercent, 85, 300},
		"Host disk usage critical": {"critical", conditionHostFSPercent, 95, 0},
		"Host disk data missing":   {"warning", conditionHostFSStale, 0, 600},
	}
	for name, w := range want {
		var severity, key, raw string
		if err := db.QueryRow(`SELECT severity, condition_key, condition_config FROM alerts WHERE name = ?`, name).Scan(&severity, &key, &raw); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		var cfg ConditionConfig
		if err := json.Unmarshal([]byte(raw), &cfg); err != nil {
			t.Fatal(err)
		}
		if severity != w.severity || key != w.key || cfg.Threshold != w.threshold || cfg.ForSeconds != w.forSeconds {
			t.Fatalf("%s = %s %s %v/%ds, want %s %s %v/%ds", name, severity, key, cfg.Threshold, cfg.ForSeconds, w.severity, w.key, w.threshold, w.forSeconds)
		}
	}
}
