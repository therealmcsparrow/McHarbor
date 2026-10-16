// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package bootstrap

import (
	"github.com/therealmcsparrow/mcharbor/core/router"
	modagent "github.com/therealmcsparrow/mcharbor/modules/agent"
	"github.com/therealmcsparrow/mcharbor/modules/agentreleases"
	"github.com/therealmcsparrow/mcharbor/modules/updates"
)

// WireAgentReleases creates the shared agent release store before the agent
// module mounts, because its binary route serves the target from the first
// request, and points the agent module at whichever store is current.
func WireAgentReleases(app *router.AppDeps) {
	agentreleases.Shared(app)
	modagent.SetReleaseSource(CurrentAgentRelease)
}

// CurrentAgentRelease returns the current release store as the agent module's
// ReleaseTarget, or a nil interface (never a typed nil) when there is none.
func CurrentAgentRelease() modagent.ReleaseTarget {
	if s := agentreleases.Current(); s != nil {
		return s
	}
	return nil
}

// NewAgentBinaries returns the agent module's served-binary lookup as the
// updates module's AgentBinaries, so the updater never imports the agent module.
func NewAgentBinaries() updates.AgentBinaries { return agentBinaries{} }

type agentBinaries struct{}

func (agentBinaries) AgentBinaryPath(osName, arch string) string {
	return modagent.AgentBinaryPath(osName, arch)
}

func (agentBinaries) FileSHA256(path string) (string, error) { return modagent.FileSHA256(path) }

func (agentBinaries) HasLocalAgentBinary(osName, arch string) bool {
	return modagent.HasLocalAgentBinary(osName, arch)
}

func (agentBinaries) LocalAgentBuild() string { return modagent.LocalAgentBuild() }
