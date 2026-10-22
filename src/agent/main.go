// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/caarlos0/env/v11"
)

// Config holds agent configuration loaded from environment variables.
type Config struct {
	McHarborURL string `env:"MCHARBOR_URL,required"`
	AgentToken  string `env:"MCHARBOR_AGENT_TOKEN,required"`
	DockerHost  string `env:"DOCKER_HOST" envDefault:"unix:///var/run/docker.sock"`
	LogLevel    string `env:"LOG_LEVEL" envDefault:"info"`
	Insecure    bool   `env:"MCHARBOR_INSECURE" envDefault:"false"`
	// Pin is the SHA-256 fingerprint of the server certificate this agent
	// will accept, in either the colon-separated or the bare hex form. It
	// is the safe answer to a privately-issued certificate: unlike
	// MCHARBOR_INSECURE it verifies one exact key, and it takes precedence
	// when both are set.
	Pin string `env:"MCHARBOR_PIN"`
	// ForceTLS promotes ws:// → wss:// and http:// → https:// in
	// the URL the agent dials. Set by the host install script when
	// the host is reachable over HTTPS; defaults to false so plain
	// HTTP deployments (still legal behind a closed VPN) work as
	// before.
	ForceTLS bool `env:"MCHARBOR_FORCE_TLS" envDefault:"false"`
	// TransferListen is where the direct agent-to-agent transfer receiver
	// binds. It defaults to every interface on 8788 so direct transfer works
	// without configuration; set it to "" to disable the receiver entirely and
	// force every transfer through the McHarbor server relay.
	TransferListen string `env:"MCHARBOR_TRANSFER_LISTEN" envDefault:"0.0.0.0:8788"`
	// TransferAdvertiseURL overrides address discovery. Leave it unset unless
	// the receiver is reached through port forwarding or a reverse proxy that
	// interface enumeration cannot infer.
	TransferAdvertiseURL string `env:"MCHARBOR_TRANSFER_ADVERTISE_URL"`
	RetireContainerID    string `env:"MCHARBOR_RETIRE_CONTAINER_ID"`
	ComposeDir           string `env:"MCHARBOR_COMPOSE_DIR" envDefault:"/var/lib/mcharbor-agent/compose"`
	// DataDir holds the agent's local state, currently the offline buffer
	// (offline.db). Mount it as a volume so buffered data survives a
	// container recreate; without one it survives restarts only.
	DataDir string `env:"MCHARBOR_DATA_DIR" envDefault:"/var/lib/mcharbor-agent"`
	// OfflineBufferEnabled turns on local buffering of Docker events,
	// container stats and the agent's own log while McHarbor is unreachable.
	// Everything buffered is replayed and cleared on the next connection.
	OfflineBufferEnabled bool `env:"MCHARBOR_OFFLINE_BUFFER" envDefault:"true"`
	// OfflineMetricsInterval is the stats sampling cadence in seconds while
	// offline. 30 matches the server-side collector.
	OfflineMetricsInterval int `env:"MCHARBOR_OFFLINE_METRICS_INTERVAL" envDefault:"30"`
	// LicenseKey is read only to warn that it is ignored. An agent's
	// environment is a seat of the McHarbor that manages it, so the agent
	// claiming one of its own would count that host twice.
	LicenseKey string `env:"MCHARBOR_LICENSE_KEY"`
}

func main() {
	// THIS MUST BE THE FIRST STATEMENT IN main(). Do not move it, and do not
	// add anything above it.
	//
	// libcontainer runs a container's init by re-executing this very binary
	// inside the new namespaces. When that happens the process arrives here
	// already half-way into a container: its namespaces are set up, a C
	// constructor has already run, and its root has not been pivoted yet.
	// Anything the normal agent startup would do in that state — parse config,
	// open a log file, create the offline database, dial the WebSocket — is
	// done in the wrong namespace against the wrong root, and the failure
	// surfaces somewhere far from its cause.
	//
	// runContainerInit hands control to libcontainer and never returns.
	if isContainerInitInvocation(os.Args) {
		runContainerInit()
	}

	// The macvtap shim, for the same reason and with the same constraint: it
	// has to place file descriptors and exec QEMU without ever having opened
	// the agent's log, database or socket. runNetdevExec never returns.
	if isNetdevExecInvocation(os.Args) {
		runNetdevExec(os.Args)
	}

	// The bundled CLI is a client of the McHarbor server, not part of the agent,
	// so it runs before any agent configuration is read or required.
	if args, ok := cliArgs(os.Args); ok {
		runCLI(args)
	}

	cfg := Config{}
	if err := env.Parse(&cfg); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to load config: %v\n", err)
		os.Exit(1)
	}

	// Set up logger
	level := slog.LevelInfo
	switch cfg.LogLevel {
	case "debug":
		level = slog.LevelDebug
	case "warn":
		level = slog.LevelWarn
	case "error":
		level = slog.LevelError
	}
	// The tee handler forwards to stdout and, while offline, into the local
	// buffer so the agent's own log reaches McHarbor once it reconnects.
	logTee := newOfflineLogHandler(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: level}))
	logger := slog.New(logTee)
	slog.SetDefault(logger)

	logger.Info("starting mcharbor-agent",
		"version", agentVersion,
		"server", cfg.McHarborURL,
		"dockerHost", cfg.DockerHost,
	)
	if detectInstallKind() == InstallKindNative {
		if exe, err := os.Executable(); err == nil {
			if resolved, err := filepath.EvalSymlinks(exe); err == nil {
				exe = resolved
			}
			if err := ensureCLILink(exe); err != nil {
				logger.Warn("the mcharbor CLI link was not made", "error", err)
			}
		}
	}

	sweepOrphanTemps(os.TempDir(), time.Now(), logger)

	ctx, cancel := context.WithCancel(context.Background())

	// Handle shutdown signals
	done := make(chan os.Signal, 1)
	signal.Notify(done, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-done
		logger.Info("shutting down...")
		cancel()
	}()

	agent := NewAgent(cfg, logger)
	if cfg.OfflineBufferEnabled {
		if buffer := openOfflineBuffer(cfg, agent, logger); buffer != nil {
			agent.SetOfflineBuffer(buffer)
			logTee.SetBuffer(buffer)
			defer buffer.Close()
		}
	}
	agent.startHostLinkReplay(ctx)
	if err := agent.StartTransferServer(ctx); err != nil {
		logger.Warn("direct transfer receiver disabled", "error", err)
	}
	if cfg.RetireContainerID != "" {
		go agent.RetireContainer(ctx, cfg.RetireContainerID)
	}
	if strings.TrimSpace(cfg.LicenseKey) != "" {
		logger.Warn("MCHARBOR_LICENSE_KEY is ignored: an agent no longer claims a licence seat; " +
			"its environment is counted by the McHarbor that manages it")
	}
	RunWithReconnect(ctx, agent, logger)

	logger.Info("agent stopped")
}

// openOfflineBuffer opens the store under DataDir, falling back to the OS temp
// directory when DataDir is not writable (a read-only root, a missing volume).
// Returns nil, with a warning logged, when neither location works so the agent
// still runs — just without buffering.
func openOfflineBuffer(cfg Config, agent *Agent, logger *slog.Logger) *OfflineBuffer {
	candidates := []string{
		filepath.Join(cfg.DataDir, "offline.db"),
		filepath.Join(os.TempDir(), "mcharbor-agent", "offline.db"),
	}
	for i, path := range candidates {
		store, err := OpenOfflineStore(path, logger)
		if err != nil {
			logger.Warn("offline store unavailable", "path", path, "error", err)
			continue
		}
		if i > 0 {
			logger.Warn("offline buffer is using a temporary location; set MCHARBOR_DATA_DIR to a persistent volume", "path", path)
		}
		logger.Info("offline buffer ready", "path", path, "metricsInterval", cfg.OfflineMetricsInterval)
		return NewOfflineBuffer(store, agent.proxy, time.Duration(cfg.OfflineMetricsInterval)*time.Second, logger)
	}
	logger.Warn("offline buffering disabled: no writable location")
	return nil
}
