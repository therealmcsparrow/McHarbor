// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package agentrpc

import (
	"context"
	"testing"

	coreagent "github.com/therealmcsparrow/mcharbor/core/agent"
)

// TestOpenConsoleRefusesBeforeReachingAnAgent holds the console gateway's bridge to the same
// environment rules as Call: a non-agent environment, an offline agent and an outdated one
// each refuse with their own code, and nothing is opened.
func TestOpenConsoleRefusesBeforeReachingAnAgent(t *testing.T) {
	req := ConsoleRequest{Kind: "serial", GuestKind: "vm", GuestID: "g1"}
	cases := []struct {
		name string
		pool AgentPool
		env  string
		want string
	}{
		{"no environment", fixedPool{agentEnv: true}, "", CodeNotAgentEnv},
		{"not an agent environment", fixedPool{}, "env-1", CodeNotAgentEnv},
		{"agent offline", fixedPool{agentEnv: true}, "env-1", CodeAgentOffline},
		{"agent outdated", fixedPool{agentEnv: true, conn: outdatedConn()}, "env-1", CodeAgentOutdated},
	}
	for _, c := range cases {
		stream, err := quietBridge(c.pool).OpenConsole(context.Background(), c.env, req)
		if stream != nil || err == nil {
			t.Fatalf("%s: opened a console (%v, %v)", c.name, stream, err)
		}
		if got := codeOf(t, err); got != c.want {
			t.Errorf("%s: code %q, want %q", c.name, got, c.want)
		}
	}
}

func outdatedConn() *coreagent.AgentConnection {
	conn := &coreagent.AgentConnection{EnvID: "env-1", Version: "1.0.0"}
	conn.Transport = coreagent.NewAgentTransport(conn, nil, nil)
	return conn
}
