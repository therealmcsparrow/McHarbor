// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package fakeagent

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/moby/moby/api/pkg/stdcopy"
	"github.com/moby/moby/client"

	coreagent "github.com/therealmcsparrow/mcharbor/core/agent"
	"github.com/therealmcsparrow/mcharbor/core/demo/estate"
)

// serverSide stands up the handshake the real agent handler performs and hands
// the resulting server transport, running its real ReadLoop, to the test.
func serverSide(t *testing.T) (*httptest.Server, <-chan *coreagent.AgentTransport) {
	t.Helper()
	ready := make(chan *coreagent.AgentTransport, 1)
	up := websocket.Upgrader{}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("token") != "agt_test" {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		conn, err := up.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		var auth coreagent.WSMessage
		if err := conn.ReadJSON(&auth); err != nil || auth.Type != coreagent.MsgAuth || auth.Auth == nil {
			t.Errorf("first frame = %+v, %v", auth, err)
			return
		}
		if err := conn.WriteJSON(coreagent.WSMessage{Type: coreagent.MsgAuthResult, AuthResult: &coreagent.AuthResultPayload{Success: true, EnvID: "env-demo"}}); err != nil {
			return
		}
		tr := coreagent.NewAgentTransport(&coreagent.AgentConnection{EnvID: "env-demo", Version: auth.Auth.AgentVersion, Build: auth.Auth.AgentBuild, Capabilities: auth.Auth.Capabilities, Conn: conn}, nil, logger)
		ready <- tr
		if err := tr.ReadLoopContext(r.Context()); err != nil {
			t.Log(err)
		}
	}))
	t.Cleanup(srv.Close)
	return srv, ready
}

func connect(t *testing.T, hostKey string) (*coreagent.AgentTransport, *estate.Host, *client.Client) {
	t.Helper()
	srv, ready := serverSide(t)
	host := estate.Build(estate.DefaultSeed, time.Now()).Host(hostKey)
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	a := New(Config{
		Host:    host,
		Token:   "agt_test",
		Version: "2026.09.01",
		Build:   "209901010000",
		Logger:  slog.New(slog.NewTextHandler(io.Discard, nil)),
		Dial: func(ctx context.Context, token string) (*websocket.Conn, error) {
			conn, resp, err := websocket.DefaultDialer.DialContext(ctx, "ws"+strings.TrimPrefix(srv.URL, "http")+"/?token="+token, nil)
			if resp != nil && resp.Body != nil {
				if cerr := resp.Body.Close(); cerr != nil {
					t.Log(cerr)
				}
			}
			return conn, err
		},
	})
	go a.Run(ctx)
	var tr *coreagent.AgentTransport
	select {
	case tr = <-ready:
	case <-time.After(5 * time.Second):
		t.Fatal("demo agent never completed the handshake")
	}
	cli, err := client.New(client.WithHTTPClient(&http.Client{Transport: tr}))
	if err != nil {
		t.Fatal(err)
	}
	return tr, host, cli
}

func TestDockerThroughTheRealAgentTransport(t *testing.T) {
	_, host, cli := connect(t, "prod-eu-2")
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	listRes, err := cli.ContainerList(ctx, client.ContainerListOptions{All: true})
	if err != nil {
		t.Fatal(err)
	}
	list := listRes.Items
	if len(list) != len(host.Containers()) {
		t.Fatalf("listed %d, want %d", len(list), len(host.Containers()))
	}
	var running string
	for _, c := range list {
		if c.State == "running" {
			running = c.ID
			break
		}
	}
	rc, err := cli.ContainerLogs(ctx, running, client.ContainerLogsOptions{ShowStdout: true, ShowStderr: true, Tail: "40"})
	if err != nil {
		t.Fatal(err)
	}
	var out, errOut strings.Builder
	if _, err := stdcopy.StdCopy(&out, &errOut, rc); err != nil {
		t.Fatalf("logs through the streamed path: %v", err)
	}
	if err := rc.Close(); err != nil {
		t.Fatal(err)
	}
	if strings.Count(out.String()+errOut.String(), "\n") != 40 {
		t.Fatal("streamed logs lost lines on the way through the transport")
	}
	if _, err := cli.ContainerRestart(ctx, running, client.ContainerRestartOptions{}); err != nil {
		t.Fatal(err)
	}
}

func TestHostRPCsThroughTheRealAgentTransport(t *testing.T) {
	tr, host, _ := connect(t, "homelab")
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	stats, ok, err := tr.HostStats(ctx)
	if err != nil || !ok || stats.CPUPercent <= 0 {
		t.Fatalf("host stats = %+v, %v, %v", stats, ok, err)
	}
	svc, err := tr.HostService(ctx, coreagent.HostServicePayload{Op: coreagent.HostServiceOpList})
	if err != nil || len(svc.Units) == 0 {
		t.Fatalf("host services = %+v, %v", svc, err)
	}
	refused, err := tr.HostService(ctx, coreagent.HostServicePayload{Op: coreagent.HostServiceOpAction, Unit: "docker.service", Action: coreagent.HostServiceActionStop})
	if err == nil && refused.ErrorCode != "protected" {
		t.Fatalf("stopping docker.service = %+v, want protected", refused)
	}

	project := host.Stacks()[0].Name
	res, err := tr.RunCompose(ctx, coreagent.ComposePayload{ProjectName: project, Args: []string{"stop"}})
	if err != nil || !res.Success {
		t.Fatalf("compose stop = %+v, %v", res, err)
	}
	for _, id := range host.ProjectContainers(project) {
		c, err := host.Container(id)
		if err != nil {
			t.Fatal(err)
		}
		if c.State == estate.StateRunning {
			t.Fatalf("%s still running after compose stop", c.Name)
		}
	}
	res, err = tr.RunCompose(ctx, coreagent.ComposePayload{ProjectName: "hello", Args: []string{"up", "-d"}, Files: map[string]string{"docker-compose.yml": "services:\n  web:\n    image: nginx:1.27.2\n    deploy:\n      replicas: 2\n"}})
	if err != nil || !res.Success {
		t.Fatalf("compose up = %+v, %v", res, err)
	}
	if got := len(host.ProjectContainers("hello")); got != 2 {
		t.Fatalf("compose up created %d containers, want 2", got)
	}
}
