// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package demo

import (
	"errors"
	"testing"
)

func TestDefaultConfigIsValid(t *testing.T) {
	if err := DefaultConfig().Validate(); err != nil {
		t.Fatalf("defaults must validate: %v", err)
	}
}

func TestValidateNamesTheFieldOutOfRange(t *testing.T) {
	cases := []struct {
		mutate func(*Config)
		field  string
	}{
		{func(c *Config) { c.Environments = 0 }, "environments"},
		{func(c *Config) { c.Containers = 6001 }, "containers"},
		{func(c *Config) { c.HistoryDays = 6 }, "historyDays"},
		{func(c *Config) { c.HistoryDays = 366 }, "historyDays"},
		{func(c *Config) { c.SwarmClusters, c.SwarmNodes = 3, 2 }, "swarmNodes"},
		{func(c *Config) { c.KubernetesClusters = 10 }, "kubernetesClusters"},
	}
	for _, tc := range cases {
		cfg := DefaultConfig()
		tc.mutate(&cfg)
		var fe *FieldError
		if err := cfg.Validate(); !errors.As(err, &fe) || fe.Field != tc.field {
			t.Errorf("%s: got %v", tc.field, err)
		}
	}
	zero := DefaultConfig()
	zero.SwarmClusters, zero.SwarmNodes, zero.SwarmServices = 0, 0, 0
	zero.VirtualMachines, zero.SystemContainers, zero.KubernetesClusters = 0, 0, 0
	if err := zero.Validate(); err != nil {
		t.Fatalf("an estate with no swarm, compute or kubernetes is allowed: %v", err)
	}
}

func TestEffectiveFallsBackToDefaults(t *testing.T) {
	var nilMarker *Marker
	if nilMarker.Effective() != DefaultConfig() || (&Marker{}).Effective() != DefaultConfig() {
		t.Fatal("a marker without sizes must mean the defaults")
	}
	cfg := DefaultConfig()
	cfg.HistoryDays = 30
	if (&Marker{Config: &cfg}).Effective().HistoryDays != 30 {
		t.Fatal("the marker's sizes were ignored")
	}
}
