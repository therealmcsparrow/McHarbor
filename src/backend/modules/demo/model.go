// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package demo

import coredemo "github.com/therealmcsparrow/mcharbor/core/demo"

// Status is what the demo banner and the settings card render.
type Status struct {
	// Available is whether demo mode is offered at all: a demo_mode file in
	// McHarbor's root. Without it Settings hides the section unless a demo runs.
	Available      bool    `json:"available"`
	Active         bool    `json:"active"`
	DatabaseExists bool    `json:"databaseExists"`
	Clustered      bool    `json:"clustered"`
	CanEnter       bool    `json:"canEnter"`
	EnteredAt      string  `json:"enteredAt,omitempty"`
	Counts         *Counts `json:"counts,omitempty"`
	// Config is the running demo's sizes, or a parked demo's; nil when there is none.
	Config   *coredemo.Config `json:"config,omitempty"`
	Defaults coredemo.Config  `json:"defaults"`
	Limits   coredemo.Limits  `json:"limits"`
}

// ChangeRequest is the optional body of enter and reset: the sizes to build.
type ChangeRequest struct {
	Config *coredemo.Config `json:"config,omitempty"`
}

// Counts sizes the running simulation, so the card can say what the demo holds.
type Counts struct {
	Environments     int `json:"environments"`
	Containers       int `json:"containers"`
	Stacks           int `json:"stacks"`
	VirtualMachines  int `json:"virtualMachines"`
	SystemContainers int `json:"systemContainers"`
	Clusters         int `json:"clusters"`
	Pods             int `json:"pods"`
	SwarmClusters    int `json:"swarmClusters"`
	SwarmNodes       int `json:"swarmNodes"`
	SwarmServices    int `json:"swarmServices"`
	HistoryDays      int `json:"historyDays"`
}

// Outcome reports whether the restart that applies an enter, exit or reset was
// scheduled, or whether the operator must restart the instance by hand.
type Outcome struct {
	RestartScheduled bool `json:"restartScheduled"`
	RestartRequired  bool `json:"restartRequired"`
}
