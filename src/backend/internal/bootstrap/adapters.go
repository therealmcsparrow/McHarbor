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
	"github.com/therealmcsparrow/mcharbor/core/audit"
	"github.com/therealmcsparrow/mcharbor/core/backupcrypto"
	"github.com/therealmcsparrow/mcharbor/core/docker"
	"github.com/therealmcsparrow/mcharbor/core/encryption"
	hostplugin "github.com/therealmcsparrow/mcharbor/core/plugin"
	"github.com/therealmcsparrow/mcharbor/core/plugin/host"
	"github.com/therealmcsparrow/mcharbor/modules/alerts"
	"github.com/therealmcsparrow/mcharbor/modules/appstore"
	"github.com/therealmcsparrow/mcharbor/modules/appstore/plugins"
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
		Name:           input.Name,
		Compose:        input.Compose,
		EnvVars:        input.EnvVars,
		Description:    input.Description,
		EnvironmentID:  input.EnvironmentID,
		AutoStart:      false,
		SecretFileMode: input.SecretFileMode,
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
				if deleteErr := a.svc.Delete(ctx, stack.Name, false); deleteErr != nil {
					return nil, fmt.Errorf("starting stack: %s; cleanup failed: %w; metadata cleanup failed: %w", result.Error, cleanupErr, deleteErr)
				}
				return nil, fmt.Errorf("starting stack: %s; resource cleanup failed: %w", result.Error, cleanupErr)
			}
			if result.Refusal != nil {
				return nil, fmt.Errorf("starting stack: %w", result.Refusal)
			}
			return nil, fmt.Errorf("starting stack: %s", result.Error)
		}
		stack, err = a.svc.ByName(ctx, stack.Name)
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

// PresentStackNames answers the App Store's installed-flag reconciliation:
// which stacks does this environment actually have right now. A false second
// return means the environment could not be inspected, and the store leaves
// its records alone rather than reading an offline agent as an uninstall.
func (a appStoreStackInstaller) PresentStackNames(ctx context.Context, environmentID string) (map[string]bool, bool) {
	return a.svc.PresentStackNames(ctx, environmentID)
}

func (a appStoreStackInstaller) RemoveInstalledStack(ctx context.Context, environmentID, stackName string) error {
	return a.svc.RemoveStack(ctx, environmentID, stackName)
}

func (a appStoreStackInstaller) RemoveInstalledStackWithProgress(ctx context.Context, environmentID, stackName string, progress appstore.StackRemovalProgress) error {
	return a.svc.RemoveStackWithProgress(ctx, environmentID, stackName, progress)
}

// PluginRuntimeProvider hands back the loaded plugin runtime, or nil
// when there is none yet.
//
// It is a function rather than a value because of boot ordering: the
// App Store service is constructed well before the plugin runtime
// exists, and a value captured at construction time would be nil
// forever. Both scan adapters below resolve it per call, so the first
// scan after the runtime lands sees it.
type PluginRuntimeProvider func() any

func (p PluginRuntimeProvider) resolve() any {
	if p == nil {
		return nil
	}
	return p()
}

type appStoreScanner struct {
	db      *sql.DB
	enc     *encryption.Service
	logger  *slog.Logger
	runtime PluginRuntimeProvider
}

func (a appStoreScanner) ScanOnInstall(ctx context.Context, imageRef, environmentID, scanner string) (*appstore.InstallScannerResult, error) {
	svc := scans.NewService(a.db, scans.NewConfigService(a.db, a.enc, a.runtime.resolve()), a.logger)
	scan, err := svc.StartScanSync(ctx, scans.StartScanInput{
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
	enc    *encryption.Service
	logger *slog.Logger
	// runtime yields the plugin runtime. Without it the workflow
	// vulnerability-scan node resolves no scanner adapters and every
	// scan node fails, so it is resolved per call rather than captured
	// at construction.
	runtime PluginRuntimeProvider
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
	return scans.NewService(w.db, scans.NewConfigService(w.db, w.enc, w.runtime.resolve()), w.logger)
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

	return &alerts.HostMetrics{
		CPUPercent: hostInfo.Host.CPUPercent,
		MemPercent: hostInfo.Host.MemPercent,
		Load1:      hostInfo.Host.Load1,
		Load5:      hostInfo.Host.Load5,
		Load15:     hostInfo.Host.Load15,
		MemTotal:   hostInfo.Host.MemTotal,
		DiskTotal:  hostInfo.Disk.Total,
	}, nil
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
		state = &alerts.ContainerInspectState{
			FinishedAt: info.State.FinishedAt,
			StartedAt:  info.State.StartedAt,
			ExitCode:   info.State.ExitCode,
			OOMKilled:  info.State.OOMKilled,
		}
		// Health is nil for an image that declares no healthcheck,
		// which is most of them. The alerts module keeps its own
		// shape so it never imports the Docker SDK.
		if info.State.Health != nil {
			state.Health = &alerts.ContainerHealth{Status: string(info.State.Health.Status)}
		}
	}

	return &alerts.ContainerInspect{State: state, RestartCount: info.RestartCount}, nil
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
		Containers: alertContainerSource{svc: containers.NewService(dockerPool, db, "", nil)},
		SystemInfo: alertSystemInfoSource{svc: dockerinfo.NewService(dockerPool)},
		// The platform source reads backup runs, scans, workflow runs
		// and audit rows other modules own — read-only, and only
		// through this handle.
		DB: db,
	}
}

// NewAppStoreService builds the app store service with concrete
// install and scan adapters.
//
// pluginRuntime is a provider, not a value: this runs during module
// mounting, long before the plugin runtime is loaded, so a value
// captured here would be nil for the life of the process and
// scan-on-install would never find an engine.
func NewAppStoreService(db *sql.DB, dockerPool *docker.ClientPool, dataDir string, enc *encryption.Service, logger *slog.Logger, pluginRuntime PluginRuntimeProvider) *appstore.Service {
	stackSvc := stacks.NewService(db, dockerPool, dataDir, enc)
	installer := appStoreStackInstaller{svc: stackSvc}

	svc := appstore.NewService(
		db,
		installer,
		appStoreScanner{db: db, enc: enc, logger: logger, runtime: pluginRuntime},
		logger,
	)
	// Lets the store clear the installed flag for an app whose containers were
	// removed outside it — by hand from the Docker overview, on a local or an
	// agent environment alike.
	svc.SetStackPresence(installer)

	return svc
}

// workflowPluginNodes forwards `plugin:<slug>:<key>` canvas actions to
// the plugin runtime's node executors. The runtime is resolved per
// call for the same reason the scanner is: it lands after the
// workflow module is mounted.
type workflowPluginNodes struct {
	runtime PluginRuntimeProvider
}

func (w workflowPluginNodes) source() (hostplugin.WorkflowNodeExecutorSource, bool) {
	src, ok := w.runtime.resolve().(hostplugin.WorkflowNodeExecutorSource)
	return src, ok && src != nil
}

func (w workflowPluginNodes) IsPluginNode(actionKey string) bool {
	src, ok := w.source()
	return ok && src.HasWorkflowNode(actionKey)
}

func (w workflowPluginNodes) ExecutePluginNode(ctx context.Context, actionKey string, in workflows.PluginNodeInput) (string, map[string]interface{}, error) {
	src, ok := w.source()
	if !ok {
		return "", nil, errors.New("plugin runtime unavailable")
	}
	out, err := src.ExecuteWorkflowNode(ctx, actionKey, hostplugin.NodeExecInput{
		Key:           in.Key,
		Config:        in.Config,
		Msg:           in.Msg,
		EnvironmentID: in.EnvironmentID,
		UserID:        in.UserID,
		WorkflowID:    in.WorkflowID,
	})
	if err != nil {
		return "", nil, err
	}
	return out.Port, out.Msg, nil
}

// NewWorkflowPluginNodeRuntime builds the bridge that lets the workflow
// engine run nodes contributed by installed plugins.
func NewWorkflowPluginNodeRuntime(pluginRuntime PluginRuntimeProvider) workflows.PluginNodeExecutor {
	return workflowPluginNodes{runtime: pluginRuntime}
}

// NewWorkflowScanner builds the workflow vulnerability scanner adapter.
//
// pluginRuntime may resolve to nil on a host with the plugin runtime
// disabled; scan nodes then fail with "no scanner configured", which
// is honest — there is no engine to run.
func NewWorkflowScanner(db *sql.DB, enc *encryption.Service, logger *slog.Logger, pluginRuntime PluginRuntimeProvider) workflows.ImageScanner {
	return workflowScanner{db: db, enc: enc, logger: logger, runtime: pluginRuntime}
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
	// pluginAdapters resolves the loaded plugin runtime lazily. The
	// runtime is populated after the workflow module is mounted, so a
	// value captured at construction time would always be nil and
	// every workflow backup to a plugin-owned destination would fail
	// its upload.
	pluginAdapters func() any
}

// ready injects the current plugin runtime before each call.
func (r workflowContainerBackupRunner) ready() *container_backups.Service {
	if r.pluginAdapters != nil {
		if rt := r.pluginAdapters(); rt != nil {
			r.svc.SetPluginStorageAdapter(rt)
		}
	}
	return r.svc
}

func (r workflowContainerBackupRunner) RunAdhoc(ctx context.Context, envID, containerID string, input workflows.ContainerBackupInput) (any, error) {
	return r.ready().RunAdhoc(ctx, envID, containerID, container_backups.RunBackupInput{
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
	return r.ready().RunPlan(ctx, planID)
}

func (r workflowContainerBackupRunner) Restore(ctx context.Context, runID string, input workflows.ContainerBackupRestoreInput) (any, error) {
	authorized := input.SelfHostAuthorized
	return r.ready().Restore(ctx, runID, container_backups.RestoreBackupInput{
		SecretKey:              input.SecretKey,
		RestoreItems:           input.RestoreItems,
		WorkflowSelfAuthorized: &authorized,
	})
}

func (r workflowContainerBackupRunner) ListPlans(ctx context.Context, envID, containerID string) (any, error) {
	return r.svc.ListPlans(ctx, envID, containerID)
}

func (r workflowContainerBackupRunner) ListRuns(ctx context.Context, envID, containerID string) (any, error) {
	return r.svc.ListRuns(ctx, envID, containerID)
}

func (r workflowContainerBackupRunner) Run(ctx context.Context, runID string) (any, error) {
	run, err := r.svc.Run(ctx, runID)
	if err != nil || run == nil {
		return nil, err
	}
	return run, nil
}

func (r workflowContainerBackupRunner) CancelRun(ctx context.Context, runID string) (any, error) {
	return r.svc.CancelRun(ctx, runID)
}

func (r workflowContainerBackupRunner) DeleteRun(ctx context.Context, runID string) (bool, error) {
	return r.ready().DeleteRun(ctx, runID)
}

func (r workflowContainerBackupRunner) Download(ctx context.Context, runID string) (*workflows.ContainerBackupDownload, error) {
	download, err := r.ready().Download(ctx, runID)
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
func NewWorkflowContainerBackupRuntime(db *sql.DB, dockerPool *docker.ClientPool, dataDir string, backupCrypto *backupcrypto.Service, enc *encryption.Service, logger *slog.Logger, pluginAdapters func() any) workflows.ContainerBackupRunner {
	return workflowContainerBackupRunner{
		svc:            container_backups.NewService(db, dockerPool, dataDir, backupCrypto, enc, logger),
		pluginAdapters: pluginAdapters,
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
	return l.svc.UnlinkContainer(context.Background(), envID, containerID) // the workflows.StackContainerLinker interface carries no ctx
}

// NewWorkflowStackContainerLinkerRuntime builds the workflow → stacks adapter.
func NewWorkflowStackContainerLinkerRuntime(db *sql.DB, dockerPool *docker.ClientPool, dataDir string, enc *encryption.Service) workflows.StackContainerLinker {
	return workflowStackContainerLinker{svc: stacks.NewService(db, dockerPool, dataDir, enc)}
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

// workflowStorageObjectOperator forwards storage-object and
// storage-location-test workflow nodes to the storage_locations
// package, which dispatches to the adapter owning the location type.
type workflowStorageObjectOperator struct {
	svc            *storage_locations.Service
	pluginAdapters func() any
}

func (o workflowStorageObjectOperator) ready() *storage_locations.Service {
	if o.pluginAdapters != nil {
		if rt := o.pluginAdapters(); rt != nil {
			o.svc.SetPluginRuntime(rt)
		}
	}
	return o.svc
}

func (o workflowStorageObjectOperator) ListObjects(ctx context.Context, locationID, prefix string) ([]workflows.StorageObjectSummary, error) {
	items, err := o.ready().ListObjects(ctx, locationID, prefix)
	if err != nil {
		return nil, err
	}
	out := make([]workflows.StorageObjectSummary, 0, len(items))
	for _, item := range items {
		out = append(out, workflows.StorageObjectSummary{Key: item.Key, Size: item.Size, LastModified: item.LastModified})
	}
	return out, nil
}

func (o workflowStorageObjectOperator) UploadObject(ctx context.Context, locationID, key string, data []byte) error {
	return o.ready().UploadObject(ctx, locationID, key, data)
}

func (o workflowStorageObjectOperator) DownloadObject(ctx context.Context, locationID, key string) ([]byte, error) {
	return o.ready().DownloadObject(ctx, locationID, key)
}

func (o workflowStorageObjectOperator) DeleteObject(ctx context.Context, locationID, key string) error {
	return o.ready().DeleteObject(ctx, locationID, key)
}

func (o workflowStorageObjectOperator) Test(ctx context.Context, locationID string) (*workflows.StorageTestResult, error) {
	result, err := o.ready().TestStorageLocation(ctx, locationID)
	if err != nil || result == nil {
		return nil, err
	}
	steps := make([]workflows.StorageTestStep, 0, len(result.Steps))
	for _, step := range result.Steps {
		steps = append(steps, workflows.StorageTestStep{Name: step.Name, Status: step.Status, Detail: step.Detail, Latency: step.Latency})
	}
	return &workflows.StorageTestResult{
		StorageLocationID: result.StorageLocationID,
		LocationName:      result.LocationName,
		LocationType:      result.LocationType,
		OverallStatus:     result.OverallStatus,
		Duration:          result.Duration,
		Steps:             steps,
	}, nil
}

func (o workflowStorageObjectOperator) Usage(ctx context.Context, locationID string) (*workflows.StorageUsageSummary, error) {
	usage, err := o.ready().Usage(ctx, locationID)
	if err != nil || usage == nil {
		return nil, err
	}
	return &workflows.StorageUsageSummary{
		Supported:  usage.Supported,
		UsedBytes:  usage.UsedBytes,
		TotalBytes: usage.TotalBytes,
		FreeBytes:  usage.FreeBytes,
		Percent:    usage.Percent,
	}, nil
}

// NewWorkflowStorageObjectRuntime builds the workflow → storage_locations object adapter.
func NewWorkflowStorageObjectRuntime(db *sql.DB, enc *encryption.Service, pluginAdapters func() any) workflows.StorageObjectOperator {
	return workflowStorageObjectOperator{
		svc:            storage_locations.NewService(db, enc),
		pluginAdapters: pluginAdapters,
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

// NewPluginStoreHandler builds the /api/appstore/plugins HTTP surface
// using the settings_catalogs-driven resolver. It bridges three
// packages:
//
//   - plugins.NewService consumes the resolver and merges entries from
//     every source (built-in GitHub + dev /mnt/plugins + any user
//     additions) by slug. *Service also satisfies plugins.Contributor
//     (compile-time asserted at installer.go:101), so the same value
//     is passed to NewInstaller as the contribution tracker — no
//     separate adapter type needed.
//   - plugins.NewInstaller is the production DefaultInstaller. It owns
//     the verify → extract → manifest → migrations → persist pipeline
//     plus the 7-step uninstall gate chain and the Configure flow.
//   - audit.NewLogger is the audit-log dependency the Handler requires.
//
// Trust keys come from host.LoadTrustedKeysFromEnv: the
// MCHARBOR_PLUGIN_TRUST_KEYS path first, then the
// $DATA_DIR/plugin-catalog/signing-key.pub fallback. The image bakes
// its public key in and points the env var at it, so first-party
// plugins verify out of the box.
//
// With no key configured the slice is empty and every install is
// recorded unsigned — allowed, warned about, and labelled, which is
// the documented policy. A key that fails to parse is different: that
// is a misconfigured host, so it is logged loudly rather than silently
// downgrading every future install to unsigned.
// catalogSync re-projects plugin-declared catalog sources into
// settings_catalogs. Passed as a function rather than the catalogs service so
// bootstrap keeps its one-way dependency on modules/appstore.
func NewPluginStoreHandler(db *sql.DB, resolver plugins.CatalogResolver, enc *encryption.Service, logger *slog.Logger, dataDir string, catalogSync func(context.Context) error, catalogPurge func(context.Context, []string) (int, error)) *plugins.Handler {
	svc := plugins.NewService(db, resolver, "")
	trustKeys, err := host.LoadTrustedKeysFromEnv()
	if err != nil {
		logger.Error("plugin trust keys could not be loaded; installs will be recorded unsigned",
			"error", err)
		trustKeys = nil
	}
	if len(trustKeys) > 0 {
		logger.Info("plugin signature verification enabled", "trust_keys", len(trustKeys))
	}
	installer := plugins.NewInstaller(
		dataDir,
		db,
		svc,
		enc,
		audit.NewLogger(db, ""),
		svc, // doubles as Contributor — see var _ Contributor = (*Service)(nil)
		logger,
		trustKeys,
	).WithCatalogSync(catalogSync).WithCatalogPurge(catalogPurge)
	return plugins.NewHandlerWithDataDir(installer, svc, audit.NewLogger(db, ""), dataDir)
}
