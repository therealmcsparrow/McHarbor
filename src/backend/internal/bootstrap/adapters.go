// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package bootstrap

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"

	coreagent "github.com/therealmcsparrow/mcharbor/core/agent"
	"github.com/therealmcsparrow/mcharbor/core/backupcrypto"
	"github.com/therealmcsparrow/mcharbor/core/docker"
	"github.com/therealmcsparrow/mcharbor/core/encryption"
	coreSettings "github.com/therealmcsparrow/mcharbor/core/settings"
	"github.com/therealmcsparrow/mcharbor/modules/alerts"
	"github.com/therealmcsparrow/mcharbor/modules/appstore"
	"github.com/therealmcsparrow/mcharbor/modules/container_backups"
	"github.com/therealmcsparrow/mcharbor/modules/containers"
	dockerinfo "github.com/therealmcsparrow/mcharbor/modules/docker_info"
	"github.com/therealmcsparrow/mcharbor/modules/metrics"
	"github.com/therealmcsparrow/mcharbor/modules/scans"
	"github.com/therealmcsparrow/mcharbor/modules/stacks"
	"github.com/therealmcsparrow/mcharbor/modules/storage_locations"
	"github.com/therealmcsparrow/mcharbor/modules/workflows"
)

type appStoreStackInstaller struct {
	svc *stacks.Service
}

func (a appStoreStackInstaller) CreateInstalledStack(ctx context.Context, input appstore.StackInstallInput) (*appstore.StackInstallOutput, error) {
	stack, err := a.svc.Create(ctx, stacks.CreateRequest{
		Name:          input.Name,
		Compose:       input.Compose,
		EnvVars:       input.EnvVars,
		Description:   input.Description,
		EnvironmentID: input.EnvironmentID,
		AutoStart:     false,
	})
	if err != nil {
		return nil, err
	}

	if input.AutoStart {
		result := a.svc.Up(ctx, stack.Name)
		if !result.Success {
			envID := ""
			if input.EnvironmentID != nil {
				envID = *input.EnvironmentID
			}
			cleanupErr := a.svc.RemoveStack(ctx, envID, stack.Name)
			if cleanupErr != nil {
				if deleteErr := a.svc.Delete(stack.Name, false); deleteErr != nil {
					return nil, fmt.Errorf("starting stack: %s; cleanup failed: %w; metadata cleanup failed: %w", result.Error, cleanupErr, deleteErr)
				}
				return nil, fmt.Errorf("starting stack: %s; resource cleanup failed: %w", result.Error, cleanupErr)
			}
			return nil, fmt.Errorf("starting stack: %s", result.Error)
		}
		stack, err = a.svc.ByName(stack.Name)
		if err != nil {
			return nil, err
		}
		if stack == nil {
			return nil, fmt.Errorf("created stack %q was not found after start", input.Name)
		}
	}

	return &appstore.StackInstallOutput{
		ID:     stack.ID,
		Name:   stack.Name,
		Status: stack.Status,
	}, nil
}

func (a appStoreStackInstaller) RemoveInstalledStack(ctx context.Context, environmentID, stackName string) error {
	return a.svc.RemoveStack(ctx, environmentID, stackName)
}

func (a appStoreStackInstaller) RemoveInstalledStackWithProgress(ctx context.Context, environmentID, stackName string, progress appstore.StackRemovalProgress) error {
	return a.svc.RemoveStackWithProgress(ctx, environmentID, stackName, progress)
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

type workflowScanner struct {
	db     *sql.DB
	logger *slog.Logger
}

func (w workflowScanner) StartScan(ctx context.Context, input workflows.ImageScanInput) (*workflows.ImageScanResult, error) {
	scan, err := w.service().StartScan(ctx, scans.StartScanInput{
		ImageRef:      input.ImageRef,
		Scanner:       input.Scanner,
		EnvironmentID: input.EnvironmentID,
	})
	return toWorkflowScanResult(scan), err
}

func (w workflowScanner) StartScanSync(ctx context.Context, input workflows.ImageScanInput) (*workflows.ImageScanResult, error) {
	scan, err := w.service().StartScanSync(ctx, scans.StartScanInput{
		ImageRef:      input.ImageRef,
		Scanner:       input.Scanner,
		EnvironmentID: input.EnvironmentID,
	})
	return toWorkflowScanResult(scan), err
}

func (w workflowScanner) service() *scans.Service {
	scannerSettings := coreSettings.ReadScannerSettings(w.db)
	scannerRegistry := scans.NewScannerRegistry(scannerSettings.ClairURL)
	return scans.NewService(w.db, scannerRegistry, w.logger)
}

func toWorkflowScanResult(scan *scans.Scan) *workflows.ImageScanResult {
	if scan == nil {
		return nil
	}
	return &workflows.ImageScanResult{
		ID:            scan.ID,
		ImageRef:      scan.ImageRef,
		Scanner:       scan.Scanner,
		Status:        scan.Status,
		Severity:      scan.Severity,
		TotalVulns:    scan.TotalVulns,
		CriticalCount: scan.CriticalCount,
		HighCount:     scan.HighCount,
		MediumCount:   scan.MediumCount,
		LowCount:      scan.LowCount,
	}
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
func NewAlertsEngineDeps(db *sql.DB, dockerPool *docker.ClientPool, agentPool *coreagent.AgentPool) alerts.EngineDeps {
	return alerts.EngineDeps{
		Metrics:    alertMetricsSource{svc: metrics.NewServiceWithAgent(dockerPool, agentPool)},
		Containers: alertContainerSource{svc: containers.NewService(dockerPool, db, "")},
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

// NewWorkflowScanner builds the workflow vulnerability scanner adapter.
func NewWorkflowScanner(db *sql.DB, logger *slog.Logger) workflows.ImageScanner {
	return workflowScanner{db: db, logger: logger}
}

// -----------------------------------------------------------------------------
// Workflow ↔ Container Backups / Stacks / Storage Locations runtime adapters.
//
// These adapters keep the workflows package free of cross-module imports.
// Concrete implementations live here and are injected from main.go via
// the Set*Runtime setters on workflows.TriggerService.
// -----------------------------------------------------------------------------

// workflowContainerBackupRunner forwards workflow container-backup node
// requests to the container_backups package.
type workflowContainerBackupRunner struct {
	svc *container_backups.Service
}

func (r workflowContainerBackupRunner) RunAdhoc(ctx context.Context, envID, containerID string, input workflows.ContainerBackupInput) (any, error) {
	return r.svc.RunAdhoc(ctx, envID, containerID, container_backups.RunBackupInput{
		Name:              input.Name,
		StorageLocationID: input.StorageLocationID,
		IncludeConfig:     input.IncludeConfig,
		IncludeLogs:       input.IncludeLogs,
		IncludeFilesystem: input.IncludeFilesystem,
		IncludeImage:      input.IncludeImage,
		SelectedMounts:    input.SelectedMounts,
	})
}

func (r workflowContainerBackupRunner) RunPlan(ctx context.Context, planID string) (any, error) {
	return r.svc.RunPlan(ctx, planID)
}

func (r workflowContainerBackupRunner) Download(ctx context.Context, runID string) (*workflows.ContainerBackupDownload, error) {
	download, err := r.svc.Download(ctx, runID)
	if err != nil || download == nil {
		return nil, err
	}
	return &workflows.ContainerBackupDownload{
		RunID:       download.RunID,
		Path:        download.Path,
		FileName:    download.FileName,
		ContentType: download.ContentType,
		Size:        download.Size,
		ModTime:     download.ModTime,
	}, nil
}

// NewWorkflowContainerBackupRuntime builds the workflow → container_backups adapter.
func NewWorkflowContainerBackupRuntime(db *sql.DB, dockerPool *docker.ClientPool, dataDir string, backupCrypto *backupcrypto.Service, enc *encryption.Service, logger *slog.Logger) workflows.ContainerBackupRunner {
	return workflowContainerBackupRunner{
		svc: container_backups.NewService(db, dockerPool, dataDir, backupCrypto, enc, logger),
	}
}

// workflowStackContainerLinker forwards workflow stack-link/unlink node
// requests to the stacks package.
type workflowStackContainerLinker struct {
	svc *stacks.Service
}

func (l workflowStackContainerLinker) LinkContainer(ctx context.Context, envID string, req workflows.LinkContainerRequest) (any, error) {
	return l.svc.LinkContainer(ctx, envID, stacks.LinkContainerRequest{
		ContainerID: req.ContainerID,
		StackName:   req.StackName,
		ServiceName: req.ServiceName,
	})
}

func (l workflowStackContainerLinker) UnlinkContainer(envID, containerID string) error {
	return l.svc.UnlinkContainer(envID, containerID)
}

// NewWorkflowStackContainerLinkerRuntime builds the workflow → stacks adapter.
func NewWorkflowStackContainerLinkerRuntime(db *sql.DB, dockerPool *docker.ClientPool, dataDir string) workflows.StackContainerLinker {
	return workflowStackContainerLinker{svc: stacks.NewService(db, dockerPool, dataDir)}
}

// workflowStorageLocationReader forwards workflow storage-location node
// requests to the storage_locations package.
type workflowStorageLocationReader struct {
	svc *storage_locations.Service
}

func (r workflowStorageLocationReader) List(ctx context.Context) ([]workflows.StorageLocationSummary, error) {
	items, err := r.svc.List(ctx)
	if err != nil {
		return nil, err
	}
	summaries := make([]workflows.StorageLocationSummary, 0, len(items))
	for _, item := range items {
		summaries = append(summaries, workflows.StorageLocationSummary{
			ID:           item.ID,
			Name:         item.Name,
			LocationType: item.LocationType,
			BasePath:     item.BasePath,
			Enabled:      item.Enabled,
		})
	}
	return summaries, nil
}

func (r workflowStorageLocationReader) ByID(ctx context.Context, id string) (*workflows.StorageLocationSummary, error) {
	item, err := r.svc.ByID(ctx, id)
	if err != nil || item == nil {
		return nil, err
	}
	return &workflows.StorageLocationSummary{
		ID:           item.ID,
		Name:         item.Name,
		LocationType: item.LocationType,
		BasePath:     item.BasePath,
		Enabled:      item.Enabled,
	}, nil
}

// NewWorkflowStorageLocationRuntime builds the workflow → storage_locations adapter.
func NewWorkflowStorageLocationRuntime(db *sql.DB, enc *encryption.Service) workflows.StorageLocationReader {
	return workflowStorageLocationReader{
		svc: storage_locations.NewService(db, enc),
	}
}

// storageLocationsBackupMigrator forwards the storage-locations migration
// endpoint to container_backups and translates the module's sentinel
// errors into the storage_locations package's re-exported errors so the
// handler can use errors.Is against the storage_locations package without
// importing container_backups.
type storageLocationsBackupMigrator struct {
	svc *container_backups.Service
}

func (m storageLocationsBackupMigrator) MigrateCompletedRunsToLocalStorage(ctx context.Context, locationID string) (any, error) {
	result, err := m.svc.MigrateCompletedRunsToLocalStorage(ctx, locationID)
	if err != nil {
		switch {
		case errors.Is(err, container_backups.ErrBackupMigrationStorageNotLocal):
			return nil, storage_locations.ErrBackupMigrationStorageNotLocal
		case errors.Is(err, container_backups.ErrBackupMigrationStorageDisabled):
			return nil, storage_locations.ErrBackupMigrationStorageDisabled
		}
		return nil, err
	}
	return result, nil
}

// NewStorageLocationsBackupMigrator builds the storage_locations → container_backups adapter.
func NewStorageLocationsBackupMigrator(db *sql.DB, dockerPool *docker.ClientPool, dataDir string, backupCrypto *backupcrypto.Service, enc *encryption.Service, logger *slog.Logger) storage_locations.BackupMigrator {
	return storageLocationsBackupMigrator{
		svc: container_backups.NewService(db, dockerPool, dataDir, backupCrypto, enc, logger),
	}
}
