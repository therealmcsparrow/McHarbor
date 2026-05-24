// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package bootstrap

import (
	"context"
	"database/sql"
	"log/slog"

	"github.com/therealmcsparrow/mcharbor/core/docker"
	coreSettings "github.com/therealmcsparrow/mcharbor/core/settings"
	"github.com/therealmcsparrow/mcharbor/modules/alerts"
	"github.com/therealmcsparrow/mcharbor/modules/appstore"
	"github.com/therealmcsparrow/mcharbor/modules/containers"
	dockerinfo "github.com/therealmcsparrow/mcharbor/modules/docker_info"
	"github.com/therealmcsparrow/mcharbor/modules/metrics"
	"github.com/therealmcsparrow/mcharbor/modules/scans"
	"github.com/therealmcsparrow/mcharbor/modules/stacks"
)

type appStoreStackInstaller struct {
	svc *stacks.Service
}

func (a appStoreStackInstaller) CreateInstalledStack(ctx context.Context, input appstore.StackInstallInput) (*appstore.StackInstallOutput, error) {
	_ = ctx

	stack, err := a.svc.Create(stacks.CreateRequest{
		Name:          input.Name,
		Compose:       input.Compose,
		EnvVars:       input.EnvVars,
		Description:   input.Description,
		EnvironmentID: input.EnvironmentID,
		AutoStart:     input.AutoStart,
	})
	if err != nil {
		return nil, err
	}

	return &appstore.StackInstallOutput{
		ID:     stack.ID,
		Name:   stack.Name,
		Status: stack.Status,
	}, nil
}

type appStoreScanner struct {
	svc *scans.Service
}

func (a appStoreScanner) ScanOnInstall(ctx context.Context, imageRef, environmentID, scanner string) (*appstore.InstallScannerResult, error) {
	scan, err := a.svc.StartScanSync(ctx, scans.StartScanInput{
		ImageRef:      imageRef,
		Scanner:       scanner,
		EnvironmentID: environmentID,
	})
	if err != nil {
		return nil, err
	}

	return &appstore.InstallScannerResult{
		TotalVulns:    scan.TotalVulns,
		CriticalCount: scan.CriticalCount,
		HighCount:     scan.HighCount,
		MediumCount:   scan.MediumCount,
		LowCount:      scan.LowCount,
	}, nil
}

type alertMetricsSource struct {
	svc *metrics.Service
}

func (a alertMetricsSource) AllContainerStats(ctx context.Context, envID string) ([]alerts.MetricSample, error) {
	stats, err := a.svc.AllContainerStats(ctx, envID)
	if err != nil {
		return nil, err
	}

	items := make([]alerts.MetricSample, 0, len(stats))
	for _, stat := range stats {
		items = append(items, alerts.MetricSample{
			ID:         stat.ID,
			Name:       stat.Name,
			CPUPercent: stat.CPUPercent,
			MemPercent: stat.MemPercent,
		})
	}

	return items, nil
}

func (a alertMetricsSource) HostInfo(ctx context.Context, envID string) (*alerts.HostMetrics, error) {
	hostInfo, err := a.svc.HostInfo(ctx, envID)
	if err != nil {
		return nil, err
	}

	return &alerts.HostMetrics{DiskTotal: hostInfo.Disk.Total}, nil
}

type alertContainerSource struct {
	svc *containers.Service
}

func (a alertContainerSource) ListContainers(ctx context.Context, envID string, all bool) ([]alerts.ContainerSummary, error) {
	items, err := a.svc.List(ctx, envID, all)
	if err != nil {
		return nil, err
	}

	results := make([]alerts.ContainerSummary, 0, len(items))
	for _, item := range items {
		results = append(results, alerts.ContainerSummary{
			ID:    item.ID,
			Names: item.Names,
			State: item.State,
		})
	}

	return results, nil
}

func (a alertContainerSource) InspectContainer(ctx context.Context, envID, id string) (*alerts.ContainerInspect, error) {
	info, err := a.svc.Inspect(ctx, envID, id)
	if err != nil {
		return nil, err
	}

	var state *alerts.ContainerInspectState
	if info.State != nil {
		state = &alerts.ContainerInspectState{FinishedAt: info.State.FinishedAt}
	}

	return &alerts.ContainerInspect{State: state}, nil
}

func (a alertContainerSource) CheckImageUpdates(ctx context.Context, envID string, containerIDs []string) ([]alerts.ImageUpdateCheck, error) {
	items, err := a.svc.CheckImageUpdates(ctx, envID, containerIDs)
	if err != nil {
		return nil, err
	}

	results := make([]alerts.ImageUpdateCheck, 0, len(items))
	for _, item := range items {
		results = append(results, alerts.ImageUpdateCheck{
			ContainerID:     item.ContainerID,
			ContainerName:   item.ContainerName,
			CurrentDigest:   item.CurrentDigest,
			RemoteDigest:    item.RemoteDigest,
			UpdateAvailable: item.UpdateAvailable,
		})
	}

	return results, nil
}

type alertSystemInfoSource struct {
	svc *dockerinfo.Service
}

func (a alertSystemInfoSource) SystemInfo(ctx context.Context, envID string) (*alerts.SystemInfo, error) {
	info, err := a.svc.SystemInfo(ctx, envID)
	if err != nil {
		return nil, err
	}

	return &alerts.SystemInfo{DriverStatus: info.DriverStatus}, nil
}

// NewAlertsEngineDeps builds the alert-engine adapters from the Docker-backed modules.
func NewAlertsEngineDeps(db *sql.DB, dockerPool *docker.ClientPool) alerts.EngineDeps {
	return alerts.EngineDeps{
		Metrics:    alertMetricsSource{svc: metrics.NewService(dockerPool)},
		Containers: alertContainerSource{svc: containers.NewService(dockerPool, db)},
		SystemInfo: alertSystemInfoSource{svc: dockerinfo.NewService(dockerPool)},
	}
}

// NewAppStoreService builds the app store service with concrete install and scan adapters.
func NewAppStoreService(db *sql.DB, dockerPool *docker.ClientPool, dataDir string, logger *slog.Logger) *appstore.Service {
	stackSvc := stacks.NewService(db, dockerPool, dataDir)
	scannerSettings := coreSettings.ReadScannerSettings(db)
	scannerRegistry := scans.NewScannerRegistry(scannerSettings.ClairURL)
	scanSvc := scans.NewService(db, scannerRegistry, logger)

	return appstore.NewService(
		db,
		appStoreStackInstaller{svc: stackSvc},
		appStoreScanner{svc: scanSvc},
		logger,
	)
}
