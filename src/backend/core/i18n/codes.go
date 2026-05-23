// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package i18n

// MsgCode is a machine-readable message identifier returned in API responses.
type MsgCode string

// --- Common ---
const (
	ErrUnauthorized    MsgCode = "err.unauthorized"
	ErrForbidden       MsgCode = "err.forbidden"
	ErrNotFound        MsgCode = "err.not_found"
	ErrInternalServer  MsgCode = "err.internal_server"
	ErrInvalidBody     MsgCode = "err.invalid_body"
	ErrRateLimitExceed MsgCode = "err.rate_limit_exceeded"
	ErrPanicRecovery   MsgCode = "err.panic_recovery"
	ErrSelfTarget      MsgCode = "err.self_target"
	ErrInvalidPath     MsgCode = "err.invalid_path"
	ErrPathTraversal   MsgCode = "err.path_traversal"
	ErrAbsolutePathReq MsgCode = "err.absolute_path_required"
)

// --- Auth ---
const (
	ErrAuthRequired           MsgCode = "err.auth.required"
	ErrAuthInvalidCredentials MsgCode = "err.auth.invalid_credentials"
	ErrAuthAccountDisabled    MsgCode = "err.auth.account_disabled"
	ErrAuthUsernameTaken      MsgCode = "err.auth.username_taken"
	ErrAuthUsernameRequired   MsgCode = "err.auth.username_required"
	ErrAuthPasswordShort      MsgCode = "err.auth.password_short"
	ErrAuthSetupCompleted     MsgCode = "err.auth.setup_completed"
	MsgAuthLoggedOut          MsgCode = "msg.auth.logged_out"
)

// --- Containers ---
const (
	ErrContainerNotFound         MsgCode = "err.container.not_found"
	ErrContainerListFailed       MsgCode = "err.container.list_failed"
	ErrContainerCreateFailed     MsgCode = "err.container.create_failed"
	ErrContainerInspectFailed    MsgCode = "err.container.inspect_failed"
	ErrContainerRemoveFailed     MsgCode = "err.container.remove_failed"
	ErrContainerStartFailed      MsgCode = "err.container.start_failed"
	ErrContainerStopFailed       MsgCode = "err.container.stop_failed"
	ErrContainerRestartFailed    MsgCode = "err.container.restart_failed"
	ErrContainerPauseFailed      MsgCode = "err.container.pause_failed"
	ErrContainerUnpauseFailed    MsgCode = "err.container.unpause_failed"
	ErrContainerKillFailed       MsgCode = "err.container.kill_failed"
	ErrContainerUpdateFailed     MsgCode = "err.container.update_failed"
	ErrContainerRecreateFailed   MsgCode = "err.container.recreate_failed"
	ErrContainerLogsFailed       MsgCode = "err.container.logs_failed"
	ErrContainerStatsFailed      MsgCode = "err.container.stats_failed"
	ErrContainerTopFailed        MsgCode = "err.container.top_failed"
	ErrContainerFilesFailed      MsgCode = "err.container.files_failed"
	ErrContainerShellsFailed     MsgCode = "err.container.shells_failed"
	ErrContainerServicesFailed   MsgCode = "err.container.services_failed"
	ErrContainerImageRequired    MsgCode = "err.container.image_required"
	ErrContainerSelfStop         MsgCode = "err.container.self_stop"
	ErrContainerSelfRemove       MsgCode = "err.container.self_remove"
	ErrContainerSelfRestart      MsgCode = "err.container.self_restart"
	ErrContainerSelfPause        MsgCode = "err.container.self_pause"
	ErrContainerSelfKill         MsgCode = "err.container.self_kill"
	MsgContainerStarted          MsgCode = "msg.container.started"
	MsgContainerStopped          MsgCode = "msg.container.stopped"
	MsgContainerRestarted        MsgCode = "msg.container.restarted"
	MsgContainerPaused           MsgCode = "msg.container.paused"
	MsgContainerUnpaused         MsgCode = "msg.container.unpaused"
	MsgContainerKilled           MsgCode = "msg.container.killed"
	ErrContainerActionFailed     MsgCode = "err.container.action_failed"
	MsgContainerActionCompleted  MsgCode = "msg.container.action_completed"
	ErrContainerFileReadFailed   MsgCode = "err.container.file_read_failed"
	ErrContainerFileWriteFailed  MsgCode = "err.container.file_write_failed"
	ErrContainerFileUploadFailed MsgCode = "err.container.file_upload_failed"
	ErrContainerFileMkdirFailed  MsgCode = "err.container.file_mkdir_failed"
	ErrContainerFileRenameFailed MsgCode = "err.container.file_rename_failed"
	ErrContainerFileChmodFailed  MsgCode = "err.container.file_chmod_failed"
	ErrContainerFileDeleteFailed MsgCode = "err.container.file_delete_failed"
	ErrContainerFileTooLarge     MsgCode = "err.container.file_too_large"
	ErrContainerFileNameRequired MsgCode = "err.container.file_name_required"
	ErrContainerFileModeInvalid  MsgCode = "err.container.file_mode_invalid"
)

// --- Container Image Updates ---
const (
	ErrContainerUpdateCheckFailed MsgCode = "err.container.update_check_failed"
	ErrContainerImageUpdateFailed MsgCode = "err.container.image_update_failed"
)

// --- Images ---
const (
	ErrImageNotFound      MsgCode = "err.image.not_found"
	ErrImageListFailed    MsgCode = "err.image.list_failed"
	ErrImageInspectFailed MsgCode = "err.image.inspect_failed"
	ErrImageRemoveFailed  MsgCode = "err.image.remove_failed"
	ErrImagePullFailed    MsgCode = "err.image.pull_failed"
	ErrImageTagFailed     MsgCode = "err.image.tag_failed"
	ErrImageHistoryFailed MsgCode = "err.image.history_failed"
	ErrImagePruneFailed   MsgCode = "err.image.prune_failed"
	ErrImageRefRequired   MsgCode = "err.image.ref_required"
	ErrImageRepoRequired  MsgCode = "err.image.repo_required"
	ErrImageExportFailed  MsgCode = "err.image.export_failed"
	ErrImageImportFailed  MsgCode = "err.image.import_failed"
	MsgImageRemoved       MsgCode = "msg.image.removed"
	MsgImageTagged        MsgCode = "msg.image.tagged"
	MsgImageImported      MsgCode = "msg.image.imported"
)

// --- Volumes ---
const (
	ErrVolumeNotFound      MsgCode = "err.volume.not_found"
	ErrVolumeListFailed    MsgCode = "err.volume.list_failed"
	ErrVolumeInspectFailed MsgCode = "err.volume.inspect_failed"
	ErrVolumeCreateFailed  MsgCode = "err.volume.create_failed"
	ErrVolumeRemoveFailed  MsgCode = "err.volume.remove_failed"
	ErrVolumePruneFailed   MsgCode = "err.volume.prune_failed"
	ErrVolumeNameRequired  MsgCode = "err.volume.name_required"
	MsgVolumeCreated       MsgCode = "msg.volume.created"
	MsgVolumeRemoved       MsgCode = "msg.volume.removed"
)

// --- Networks ---
const (
	ErrNetworkNotFound         MsgCode = "err.network.not_found"
	ErrNetworkListFailed       MsgCode = "err.network.list_failed"
	ErrNetworkInspectFailed    MsgCode = "err.network.inspect_failed"
	ErrNetworkCreateFailed     MsgCode = "err.network.create_failed"
	ErrNetworkRemoveFailed     MsgCode = "err.network.remove_failed"
	ErrNetworkConnectFailed    MsgCode = "err.network.connect_failed"
	ErrNetworkDisconnectFailed MsgCode = "err.network.disconnect_failed"
	ErrNetworkNameRequired     MsgCode = "err.network.name_required"
	ErrNetworkContainerReq     MsgCode = "err.network.container_required"
	MsgNetworkCreated          MsgCode = "msg.network.created"
	MsgNetworkRemoved          MsgCode = "msg.network.removed"
	MsgNetworkConnected        MsgCode = "msg.network.connected"
	MsgNetworkDisconnected     MsgCode = "msg.network.disconnected"
)

// --- Stacks ---
const (
	ErrStackNotFound          MsgCode = "err.stack.not_found"
	ErrStackListFailed        MsgCode = "err.stack.list_failed"
	ErrStackCreateFailed      MsgCode = "err.stack.create_failed"
	ErrStackUpdateFailed      MsgCode = "err.stack.update_failed"
	ErrStackRemoveFailed      MsgCode = "err.stack.remove_failed"
	ErrStackDeployFailed      MsgCode = "err.stack.deploy_failed"
	ErrStackStopFailed        MsgCode = "err.stack.stop_failed"
	ErrStackDownFailed        MsgCode = "err.stack.down_failed"
	ErrStackRestartFailed     MsgCode = "err.stack.restart_failed"
	ErrStackLogsFailed        MsgCode = "err.stack.logs_failed"
	ErrStackComposeFailed     MsgCode = "err.stack.compose_not_found"
	ErrStackNameRequired      MsgCode = "err.stack.name_required"
	ErrStackComposeRequired   MsgCode = "err.stack.compose_required"
	ErrStackAlreadyManaged    MsgCode = "err.stack.already_managed"
	ErrStackAdoptFailed       MsgCode = "err.stack.adopt_failed"
	ErrStackPreviewFailed     MsgCode = "err.stack.preview_failed"
	ErrStackWebhookNotFound   MsgCode = "err.stack.webhook_not_found"
	ErrStackWebhookFailed     MsgCode = "err.stack.webhook_failed"
	ErrStackPruneFailed       MsgCode = "err.stack.prune_failed"
	ErrStackEnvVarsFailed     MsgCode = "err.stack.env_vars_failed"
	MsgStackDeployed          MsgCode = "msg.stack.deployed"
	MsgStackRemoved           MsgCode = "msg.stack.removed"
	MsgStackAdopted           MsgCode = "msg.stack.adopted"
	MsgStackPruned            MsgCode = "msg.stack.pruned"
	MsgStackEnvVarsUpdated    MsgCode = "msg.stack.env_vars_updated"
	ErrStackUpdateCheckFailed MsgCode = "err.stack.update_check_failed"
)

// --- Environments ---
const (
	ErrEnvNotFound            MsgCode = "err.env.not_found"
	ErrEnvListFailed          MsgCode = "err.env.list_failed"
	ErrEnvCreateFailed        MsgCode = "err.env.create_failed"
	ErrEnvUpdateFailed        MsgCode = "err.env.update_failed"
	ErrEnvRemoveFailed        MsgCode = "err.env.remove_failed"
	ErrEnvNameRequired        MsgCode = "err.env.name_required"
	ErrEnvConnRequired        MsgCode = "err.env.connection_required"
	ErrEnvConnFailed          MsgCode = "err.env.connection_failed"
	ErrEnvInvalidOrchestrator MsgCode = "err.env.invalid_orchestrator"
	ErrEnvInvalidConnType     MsgCode = "err.env.invalid_connection_type"
	ErrEnvInvalidTimezone     MsgCode = "err.env.invalid_timezone"
	MsgEnvCreated             MsgCode = "msg.env.created"
	MsgEnvRemoved             MsgCode = "msg.env.removed"
)

// --- Terminal ---
const (
	ErrTerminalFailed    MsgCode = "err.terminal.failed"
	ErrTerminalUpgrade   MsgCode = "err.terminal.upgrade_failed"
	ErrTerminalContainer MsgCode = "err.terminal.container_required"
)

// --- Logs ---
const (
	ErrLogsFailed       MsgCode = "err.logs.failed"
	ErrLogsContainerReq MsgCode = "err.logs.container_required"
)

// --- Events ---
const (
	ErrEventsFailed MsgCode = "err.events.failed"
)

// --- Dashboard ---
const (
	ErrDashboardFailed MsgCode = "err.dashboard.failed"
)

// --- Blueprints ---
const (
	ErrBlueprintNotFound        MsgCode = "err.blueprint.not_found"
	ErrBlueprintListFailed      MsgCode = "err.blueprint.list_failed"
	ErrBlueprintCreateFailed    MsgCode = "err.blueprint.create_failed"
	ErrBlueprintUpdateFailed    MsgCode = "err.blueprint.update_failed"
	ErrBlueprintRemoveFailed    MsgCode = "err.blueprint.remove_failed"
	ErrBlueprintNameRequired    MsgCode = "err.blueprint.name_required"
	ErrBlueprintComposeRequired MsgCode = "err.blueprint.compose_required"
	ErrBlueprintStackNameReq    MsgCode = "err.blueprint.stack_name_required"
)

// --- Reconciler ---
const (
	ErrReconcilerFailed        MsgCode = "err.reconciler.failed"
	ErrReconcilerNotFound      MsgCode = "err.reconciler.not_found"
	ErrReconcilerListFailed    MsgCode = "err.reconciler.list_failed"
	ErrReconcilerCreateFailed  MsgCode = "err.reconciler.create_failed"
	ErrReconcilerUpdateFailed  MsgCode = "err.reconciler.update_failed"
	ErrReconcilerRemoveFailed  MsgCode = "err.reconciler.remove_failed"
	ErrReconcilerNameRequired  MsgCode = "err.reconciler.name_required"
	ErrReconcilerContainerReq  MsgCode = "err.reconciler.container_required"
	ErrReconcilerImageRequired MsgCode = "err.reconciler.image_required"
	ErrReconcilerDockerFailed  MsgCode = "err.reconciler.docker_failed"
)

// --- Git ---
const (
	ErrGitFailed       MsgCode = "err.git.failed"
	ErrGitCloneFailed  MsgCode = "err.git.clone_failed"
	ErrGitNotFound     MsgCode = "err.git.not_found"
	ErrGitListFailed   MsgCode = "err.git.list_failed"
	ErrGitCreateFailed MsgCode = "err.git.create_failed"
	ErrGitUpdateFailed MsgCode = "err.git.update_failed"
	ErrGitRemoveFailed MsgCode = "err.git.remove_failed"
	ErrGitNameRequired MsgCode = "err.git.name_required"
	ErrGitUrlRequired  MsgCode = "err.git.url_required"
)

// --- Webhooks ---
const (
	ErrWebhookNotFound     MsgCode = "err.webhook.not_found"
	ErrWebhookListFailed   MsgCode = "err.webhook.list_failed"
	ErrWebhookCreateFailed MsgCode = "err.webhook.create_failed"
	ErrWebhookUpdateFailed MsgCode = "err.webhook.update_failed"
	ErrWebhookRemoveFailed MsgCode = "err.webhook.remove_failed"
	ErrWebhookNameRequired MsgCode = "err.webhook.name_required"
	ErrWebhookUrlRequired  MsgCode = "err.webhook.url_required"
)

// --- Agent ---
const (
	ErrAgentNotFound               MsgCode = "err.agent.not_found"
	ErrAgentListFailed             MsgCode = "err.agent.list_failed"
	ErrAgentStatusFailed           MsgCode = "err.agent.status_failed"
	ErrAgentTokenFailed            MsgCode = "err.agent.token_failed"
	ErrAgentDeployFailed           MsgCode = "err.agent.deploy_failed"
	ErrAgentDeploySSHRequired      MsgCode = "err.agent.deploy_ssh_required"
	ErrAgentDeployHostKeyRequired  MsgCode = "err.agent.deploy_host_key_required"
	ErrAgentDeployInvalidMethod    MsgCode = "err.agent.deploy_invalid_method"
	ErrAgentDeploySSHKeyInvalid    MsgCode = "err.agent.deploy_ssh_key_invalid"
	ErrAgentDeployHostKeyMismatch  MsgCode = "err.agent.deploy_host_key_mismatch"
	ErrAgentDeploySSHConnectFailed MsgCode = "err.agent.deploy_ssh_connect_failed"
	ErrAgentDeployOSDetectFailed   MsgCode = "err.agent.deploy_os_detect_failed"
	ErrAgentDeployCommandFailed    MsgCode = "err.agent.deploy_command_failed"
	ErrAgentInstallTokenFailed     MsgCode = "err.agent.install_token_failed"
	MsgAgentTokenRegenerated       MsgCode = "msg.agent.token_regenerated"
	MsgAgentDeployed               MsgCode = "msg.agent.deployed"
)

// --- Settings ---
const (
	ErrSettingsFailed           MsgCode = "err.settings.failed"
	ErrSettingsUpdateFailed     MsgCode = "err.settings.update_failed"
	ErrSettingsNoSettings       MsgCode = "err.settings.no_settings"
	ErrSettingsTLSInvalid       MsgCode = "err.settings.tls_invalid"
	ErrSettingsTLSPairReq       MsgCode = "err.settings.tls_pair_required"
	MsgSettingsUpdated          MsgCode = "msg.settings.updated"
	MsgAgentSettingsUpdated     MsgCode = "msg.agent_settings.updated"
	ErrAgentSettingsInvalid     MsgCode = "err.agent_settings.invalid"
	ErrRetentionSettingsInvalid MsgCode = "err.retention_settings.invalid"
	MsgRetentionSettingsUpdated MsgCode = "msg.retention_settings.updated"
)

// --- Users ---
const (
	ErrUserNotFound          MsgCode = "err.user.not_found"
	ErrUserListFailed        MsgCode = "err.user.list_failed"
	ErrUserCreateFailed      MsgCode = "err.user.create_failed"
	ErrUserUpdateFailed      MsgCode = "err.user.update_failed"
	ErrUserRemoveFailed      MsgCode = "err.user.remove_failed"
	ErrUserSelfDelete        MsgCode = "err.user.self_delete"
	ErrUserPasswordRequired  MsgCode = "err.user.password_required"
	ErrUserPasswordIncorrect MsgCode = "err.user.password_incorrect"
	MsgUserPasswordUpdated   MsgCode = "msg.user.password_updated"
)

// --- Kubernetes: Pods ---
const (
	ErrPodNotFound     MsgCode = "err.pod.not_found"
	ErrPodListFailed   MsgCode = "err.pod.list_failed"
	ErrPodGetFailed    MsgCode = "err.pod.get_failed"
	ErrPodDeleteFailed MsgCode = "err.pod.delete_failed"
	ErrPodLogsFailed   MsgCode = "err.pod.logs_failed"
	MsgPodDeleted      MsgCode = "msg.pod.deleted"
)

// --- Kubernetes: Deployments ---
const (
	ErrDeploymentNotFound      MsgCode = "err.deployment.not_found"
	ErrDeploymentListFailed    MsgCode = "err.deployment.list_failed"
	ErrDeploymentGetFailed     MsgCode = "err.deployment.get_failed"
	ErrDeploymentScaleFailed   MsgCode = "err.deployment.scale_failed"
	ErrDeploymentRestartFailed MsgCode = "err.deployment.restart_failed"
	ErrDeploymentDeleteFailed  MsgCode = "err.deployment.delete_failed"
	MsgDeploymentScaled        MsgCode = "msg.deployment.scaled"
	MsgDeploymentRestarted     MsgCode = "msg.deployment.restarted"
	MsgDeploymentDeleted       MsgCode = "msg.deployment.deleted"
)

// --- Kubernetes: Services ---
const (
	ErrK8sServiceNotFound     MsgCode = "err.k8s_service.not_found"
	ErrK8sServiceListFailed   MsgCode = "err.k8s_service.list_failed"
	ErrK8sServiceGetFailed    MsgCode = "err.k8s_service.get_failed"
	ErrK8sServiceDeleteFailed MsgCode = "err.k8s_service.delete_failed"
)

// --- Kubernetes: Namespaces ---
const (
	ErrNamespaceNotFound   MsgCode = "err.namespace.not_found"
	ErrNamespaceListFailed MsgCode = "err.namespace.list_failed"
	ErrNamespaceGetFailed  MsgCode = "err.namespace.get_failed"
)

// --- Activity ---
const (
	ErrActivityListFailed MsgCode = "err.activity.list_failed"
)

// --- Audit ---
const (
	ErrAuditListFailed MsgCode = "err.audit.list_failed"
)

// --- Notifications ---
const (
	ErrNotificationNotFound     MsgCode = "err.notification.not_found"
	ErrNotificationListFailed   MsgCode = "err.notification.list_failed"
	ErrNotificationCreateFailed MsgCode = "err.notification.create_failed"
	ErrNotificationUpdateFailed MsgCode = "err.notification.update_failed"
	ErrNotificationRemoveFailed MsgCode = "err.notification.remove_failed"
	ErrNotificationNameRequired MsgCode = "err.notification.name_required"
	ErrNotificationTypeRequired MsgCode = "err.notification.type_required"
	ErrNotificationInvalidType  MsgCode = "err.notification.invalid_type"
)

// --- In-App Notifications ---
const (
	ErrInAppNotificationNotFound     MsgCode = "err.in_app_notification.not_found"
	ErrInAppNotificationListFailed   MsgCode = "err.in_app_notification.list_failed"
	ErrInAppNotificationReadFailed   MsgCode = "err.in_app_notification.read_failed"
	ErrInAppNotificationDeleteFailed MsgCode = "err.in_app_notification.delete_failed"
	ErrInAppNotificationCountFailed  MsgCode = "err.in_app_notification.count_failed"
)

// --- Registry ---
const (
	ErrRegistryFailed       MsgCode = "err.registry.failed"
	ErrRegistryNotFound     MsgCode = "err.registry.not_found"
	ErrRegistryListFailed   MsgCode = "err.registry.list_failed"
	ErrRegistryCreateFailed MsgCode = "err.registry.create_failed"
	ErrRegistryUpdateFailed MsgCode = "err.registry.update_failed"
	ErrRegistryRemoveFailed MsgCode = "err.registry.remove_failed"
	ErrRegistryNameRequired MsgCode = "err.registry.name_required"
	ErrRegistryUrlRequired  MsgCode = "err.registry.url_required"
)

// --- Scans ---
const (
	ErrScanFailed             MsgCode = "err.scan.failed"
	ErrScanNotFound           MsgCode = "err.scan.not_found"
	ErrScanListFailed         MsgCode = "err.scan.list_failed"
	ErrScanCreateFailed       MsgCode = "err.scan.create_failed"
	ErrScanImageRequired      MsgCode = "err.scan.image_required"
	ErrScanDeleteFailed       MsgCode = "err.scan.delete_failed"
	ErrScanSummaryFailed      MsgCode = "err.scan.summary_failed"
	ErrScannerNotAvailable    MsgCode = "err.scan.scanner_not_available"
	ErrScannerInvalid         MsgCode = "err.scan.scanner_invalid"
	ErrScanAlreadyRunning     MsgCode = "err.scan.already_running"
	MsgScanStarted            MsgCode = "msg.scan.started"
	MsgScanDeleted            MsgCode = "msg.scan.deleted"
	ErrScannerSettingsInvalid MsgCode = "err.scanner.settings_invalid"
	MsgScannerSettingsUpdated MsgCode = "msg.scanner.settings_updated"
)

// --- Updates ---
const (
	ErrUpdateCheckFailed  MsgCode = "err.update.check_failed"
	ErrUpdateNotFound     MsgCode = "err.update.not_found"
	ErrUpdateListFailed   MsgCode = "err.update.list_failed"
	ErrUpdateCreateFailed MsgCode = "err.update.create_failed"
	ErrUpdateRemoveFailed MsgCode = "err.update.remove_failed"
	ErrUpdateNameRequired MsgCode = "err.update.name_required"
)

// --- Plugins ---
const (
	ErrPluginListFailed     MsgCode = "err.plugin.list_failed"
	ErrPluginNotFound       MsgCode = "err.plugin.not_found"
	ErrPluginInstallFailed  MsgCode = "err.plugin.install_failed"
	ErrPluginRemoveFailed   MsgCode = "err.plugin.remove_failed"
	ErrPluginNameRequired   MsgCode = "err.plugin.name_required"
	ErrPluginSourceRequired MsgCode = "err.plugin.source_required"
)

// --- Schedules ---
const (
	ErrScheduleListFailed     MsgCode = "err.schedule.list_failed"
	ErrScheduleNotFound       MsgCode = "err.schedule.not_found"
	ErrScheduleCreateFailed   MsgCode = "err.schedule.create_failed"
	ErrScheduleUpdateFailed   MsgCode = "err.schedule.update_failed"
	ErrScheduleRemoveFailed   MsgCode = "err.schedule.remove_failed"
	ErrScheduleNameRequired   MsgCode = "err.schedule.name_required"
	ErrScheduleCronRequired   MsgCode = "err.schedule.cron_required"
	ErrScheduleActionRequired MsgCode = "err.schedule.action_required"
	ErrScheduleTargetRequired MsgCode = "err.schedule.target_required"
)

// --- Alerts ---
const (
	ErrAlertListFailed          MsgCode = "err.alert.list_failed"
	ErrAlertNotFound            MsgCode = "err.alert.not_found"
	ErrAlertCreateFailed        MsgCode = "err.alert.create_failed"
	ErrAlertUpdateFailed        MsgCode = "err.alert.update_failed"
	ErrAlertRemoveFailed        MsgCode = "err.alert.remove_failed"
	ErrAlertNameRequired        MsgCode = "err.alert.name_required"
	ErrAlertTypeRequired        MsgCode = "err.alert.type_required"
	ErrAlertDestinationRequired MsgCode = "err.alert.destination_required"
)

// --- Metrics ---
const (
	ErrMetricsFailed              MsgCode = "err.metrics.failed"
	ErrMetricsHostInfoFailed      MsgCode = "err.metrics.host_info_failed"
	ErrMetricsStatsFailed         MsgCode = "err.metrics.stats_failed"
	ErrMetricsContainerRequired   MsgCode = "err.metrics.container_required"
	ErrMetricsContainerNotFound   MsgCode = "err.metrics.container_not_found"
	ErrMetricsContainerNotRunning MsgCode = "err.metrics.container_not_running"
	ErrMetricsStreamNotSupported  MsgCode = "err.metrics.stream_not_supported"
)

// --- Workflows ---
const (
	ErrWorkflowNotFound         MsgCode = "err.workflow.not_found"
	ErrWorkflowListFailed       MsgCode = "err.workflow.list_failed"
	ErrWorkflowCreateFailed     MsgCode = "err.workflow.create_failed"
	ErrWorkflowUpdateFailed     MsgCode = "err.workflow.update_failed"
	ErrWorkflowRemoveFailed     MsgCode = "err.workflow.remove_failed"
	ErrWorkflowRunFailed        MsgCode = "err.workflow.run_failed"
	ErrWorkflowNameRequired     MsgCode = "err.workflow.name_required"
	ErrLinkMessageFetchFailed   MsgCode = "err.workflow.link_message_fetch_failed"
	ErrLinkOutputListFailed     MsgCode = "err.workflow.link_output_list_failed"
	ErrWorkflowNodeListFailed   MsgCode = "err.workflow_node.list_failed"
	ErrWorkflowNodeUpdateFailed MsgCode = "err.workflow_node.update_failed"
	ErrWorkflowNodeKeyRequired  MsgCode = "err.workflow_node.key_required"
)

// --- App Store ---
const (
	ErrAppStoreListFailed    MsgCode = "err.appstore.list_failed"
	ErrAppStoreInstallFailed MsgCode = "err.appstore.install_failed"
	ErrAppStoreNotFound      MsgCode = "err.appstore.not_found"
	ErrAppStoreSlugRequired  MsgCode = "err.appstore.slug_required"
)

// --- RBAC ---
const (
	ErrRBACPermissionDenied MsgCode = "err.rbac.permission_denied"
)

// --- Roles ---
const (
	ErrRoleNotFound     MsgCode = "err.role.not_found"
	ErrRoleListFailed   MsgCode = "err.role.list_failed"
	ErrRoleCreateFailed MsgCode = "err.role.create_failed"
	ErrRoleUpdateFailed MsgCode = "err.role.update_failed"
	ErrRoleRemoveFailed MsgCode = "err.role.remove_failed"
	ErrRoleNameRequired MsgCode = "err.role.name_required"
	ErrRoleNameTaken    MsgCode = "err.role.name_taken"
	ErrRoleSystemLocked MsgCode = "err.role.system_locked"
	MsgRoleCreated      MsgCode = "msg.role.created"
	MsgRoleUpdated      MsgCode = "msg.role.updated"
	MsgRoleRemoved      MsgCode = "msg.role.removed"
)

// --- Groups ---
const (
	ErrGroupNotFound       MsgCode = "err.group.not_found"
	ErrGroupListFailed     MsgCode = "err.group.list_failed"
	ErrGroupCreateFailed   MsgCode = "err.group.create_failed"
	ErrGroupUpdateFailed   MsgCode = "err.group.update_failed"
	ErrGroupRemoveFailed   MsgCode = "err.group.remove_failed"
	ErrGroupNameRequired   MsgCode = "err.group.name_required"
	ErrGroupNameTaken      MsgCode = "err.group.name_taken"
	ErrGroupMemberExists   MsgCode = "err.group.member_exists"
	ErrGroupSystemLocked   MsgCode = "err.group.system_locked"
	MsgGroupCreated        MsgCode = "msg.group.created"
	MsgGroupUpdated        MsgCode = "msg.group.updated"
	MsgGroupRemoved        MsgCode = "msg.group.removed"
	MsgGroupMemberAdded    MsgCode = "msg.group.member_added"
	MsgGroupMemberRemoved  MsgCode = "msg.group.member_removed"
	MsgGroupRoleAssigned   MsgCode = "msg.group.role_assigned"
	MsgGroupRoleUnassigned MsgCode = "msg.group.role_unassigned"
)

// --- API Keys ---
const (
	ErrAPIKeyNotFound     MsgCode = "err.api_key.not_found"
	ErrAPIKeyListFailed   MsgCode = "err.api_key.list_failed"
	ErrAPIKeyCreateFailed MsgCode = "err.api_key.create_failed"
	ErrAPIKeyRevokeFailed MsgCode = "err.api_key.revoke_failed"
	ErrAPIKeyNameRequired MsgCode = "err.api_key.name_required"
	ErrAPIKeyInvalid      MsgCode = "err.api_key.invalid"
	ErrAPIKeyRevoked      MsgCode = "err.api_key.revoked"
	ErrAPIKeyExpired      MsgCode = "err.api_key.expired"
	MsgAPIKeyCreated      MsgCode = "msg.api_key.created"
	MsgAPIKeyRevoked      MsgCode = "msg.api_key.revoked"
)

// --- User Roles ---
const (
	ErrUserRoleAssignFailed   MsgCode = "err.user_role.assign_failed"
	ErrUserRoleUnassignFailed MsgCode = "err.user_role.unassign_failed"
	ErrUserRoleListFailed     MsgCode = "err.user_role.list_failed"
	ErrUserRoleExists         MsgCode = "err.user_role.exists"
	MsgUserRoleAssigned       MsgCode = "msg.user_role.assigned"
	MsgUserRoleUnassigned     MsgCode = "msg.user_role.unassigned"
)

// --- User Groups ---
const (
	ErrUserGroupListFailed MsgCode = "err.user_group.list_failed"
)

// --- Docker Info ---
const (
	ErrDockerInfoFailed MsgCode = "err.docker_info.failed"
)

// --- Widgets ---
const (
	ErrWidgetListFailed      MsgCode = "err.widget.list_failed"
	ErrWidgetNotFound        MsgCode = "err.widget.not_found"
	ErrWidgetInstallFailed   MsgCode = "err.widget.install_failed"
	ErrWidgetUninstallFailed MsgCode = "err.widget.uninstall_failed"
	ErrWidgetUpdateFailed    MsgCode = "err.widget.update_failed"
	ErrWidgetKeyRequired     MsgCode = "err.widget.key_required"
	ErrWidgetKeyExists       MsgCode = "err.widget.key_exists"
	ErrWidgetBuiltinDelete   MsgCode = "err.widget.builtin_cannot_delete"
	MsgWidgetInstalled       MsgCode = "msg.widget.installed"
	MsgWidgetUninstalled     MsgCode = "msg.widget.uninstalled"
)

// --- Custom Nodes ---
const (
	ErrCustomNodeListFailed   MsgCode = "err.custom_node.list_failed"
	ErrCustomNodeNotFound     MsgCode = "err.custom_node.not_found"
	ErrCustomNodeKeyRequired  MsgCode = "err.custom_node.key_required"
	ErrCustomNodeKeyExists    MsgCode = "err.custom_node.key_exists"
	ErrCustomNodeCreateFailed MsgCode = "err.custom_node.create_failed"
	ErrCustomNodeUpdateFailed MsgCode = "err.custom_node.update_failed"
	ErrCustomNodeDeleteFailed MsgCode = "err.custom_node.delete_failed"
	ErrCustomNodeCodeRequired MsgCode = "err.custom_node.code_required"
	MsgCustomNodeCreated      MsgCode = "msg.custom_node.created"
	MsgCustomNodeUpdated      MsgCode = "msg.custom_node.updated"
	MsgCustomNodeDeleted      MsgCode = "msg.custom_node.deleted"
)

// --- Identity Providers ---
const (
	ErrIdentityNotFound          MsgCode = "err.identity.not_found"
	ErrIdentityListFailed        MsgCode = "err.identity.list_failed"
	ErrIdentityCreateFailed      MsgCode = "err.identity.create_failed"
	ErrIdentityUpdateFailed      MsgCode = "err.identity.update_failed"
	ErrIdentityRemoveFailed      MsgCode = "err.identity.remove_failed"
	ErrIdentityNameRequired      MsgCode = "err.identity.name_required"
	ErrIdentityClientRequired    MsgCode = "err.identity.client_id_required"
	ErrIdentitySecretRequired    MsgCode = "err.identity.client_secret_required"
	ErrIdentityTenantRequired    MsgCode = "err.identity.tenant_id_required"
	ErrIdentityIssuerRequired    MsgCode = "err.identity.issuer_url_required"
	ErrIdentityMetadataRequired  MsgCode = "err.identity.metadata_url_required"
	ErrIdentityInvalidType       MsgCode = "err.identity.invalid_type"
	ErrIdentityDisabled          MsgCode = "err.identity.disabled"
	ErrOIDCStateFailed           MsgCode = "err.oidc.state_failed"
	ErrOIDCStateInvalid          MsgCode = "err.oidc.state_invalid"
	ErrOIDCStateExpired          MsgCode = "err.oidc.state_expired"
	ErrOIDCExchangeFailed        MsgCode = "err.oidc.exchange_failed"
	ErrOIDCUserInfoFailed        MsgCode = "err.oidc.userinfo_failed"
	ErrOIDCProvisionFailed       MsgCode = "err.oidc.provision_failed"
	ErrIdentityFetchGroupsFailed MsgCode = "err.identity.fetch_groups_failed"
	ErrIdentityTestFailed        MsgCode = "err.identity.test_failed"
	MsgIdentityCreated           MsgCode = "msg.identity.created"
	MsgIdentityUpdated           MsgCode = "msg.identity.updated"
	MsgIdentityRemoved           MsgCode = "msg.identity.removed"
	MsgIdentityTestSuccess       MsgCode = "msg.identity.test_success"
)

// --- Email Servers ---
const (
	ErrEmailServerNotFound       MsgCode = "err.email_server.not_found"
	ErrEmailServerListFailed     MsgCode = "err.email_server.list_failed"
	ErrEmailServerCreateFailed   MsgCode = "err.email_server.create_failed"
	ErrEmailServerUpdateFailed   MsgCode = "err.email_server.update_failed"
	ErrEmailServerRemoveFailed   MsgCode = "err.email_server.remove_failed"
	ErrEmailServerNameRequired   MsgCode = "err.email_server.name_required"
	ErrEmailServerTypeInvalid    MsgCode = "err.email_server.type_invalid"
	ErrEmailServerFromRequired   MsgCode = "err.email_server.from_required"
	ErrEmailServerHostRequired   MsgCode = "err.email_server.host_required"
	ErrEmailServerPortRequired   MsgCode = "err.email_server.port_required"
	ErrEmailServerAuthInvalid    MsgCode = "err.email_server.auth_invalid"
	ErrEmailServerEncInvalid     MsgCode = "err.email_server.encryption_invalid"
	ErrEmailServerCredRequired   MsgCode = "err.email_server.credentials_required"
	ErrEmailServerClientRequired MsgCode = "err.email_server.client_id_required"
	ErrEmailServerSecretRequired MsgCode = "err.email_server.client_secret_required"
	ErrEmailServerTenantRequired MsgCode = "err.email_server.tenant_id_required"
	ErrEmailServerTestFailed     MsgCode = "err.email_server.test_failed"
	ErrEmailServerTestToRequired MsgCode = "err.email_server.test_to_required"
	MsgEmailServerCreated        MsgCode = "msg.email_server.created"
	MsgEmailServerUpdated        MsgCode = "msg.email_server.updated"
	MsgEmailServerRemoved        MsgCode = "msg.email_server.removed"
	MsgEmailServerDefaultSet     MsgCode = "msg.email_server.default_set"
	MsgEmailServerTestSent       MsgCode = "msg.email_server.test_sent"
)

// --- Communication Channels ---
const (
	ErrCommChannelNotFound           MsgCode = "err.comm_channel.not_found"
	ErrCommChannelListFailed         MsgCode = "err.comm_channel.list_failed"
	ErrCommChannelCreateFailed       MsgCode = "err.comm_channel.create_failed"
	ErrCommChannelUpdateFailed       MsgCode = "err.comm_channel.update_failed"
	ErrCommChannelRemoveFailed       MsgCode = "err.comm_channel.remove_failed"
	ErrCommChannelNameRequired       MsgCode = "err.comm_channel.name_required"
	ErrCommChannelTypeInvalid        MsgCode = "err.comm_channel.type_invalid"
	ErrCommChannelWebhookRequired    MsgCode = "err.comm_channel.webhook_required"
	ErrCommChannelServerRequired     MsgCode = "err.comm_channel.server_required"
	ErrCommChannelTokenRequired      MsgCode = "err.comm_channel.token_required"
	ErrCommChannelTopicRequired      MsgCode = "err.comm_channel.topic_required"
	ErrCommChannelChatIDRequired     MsgCode = "err.comm_channel.chat_id_required"
	ErrCommChannelPhoneRequired      MsgCode = "err.comm_channel.phone_required"
	ErrCommChannelSenderRequired     MsgCode = "err.comm_channel.sender_required"
	ErrCommChannelRecipientsRequired MsgCode = "err.comm_channel.recipients_required"
	ErrCommChannelPriorityInvalid    MsgCode = "err.comm_channel.priority_invalid"
	ErrCommChannelTelegramAdminRequired MsgCode = "err.comm_channel.telegram_admin_required"
	ErrCommChannelTestFailed         MsgCode = "err.comm_channel.test_failed"
	MsgCommChannelCreated            MsgCode = "msg.comm_channel.created"
	MsgCommChannelUpdated            MsgCode = "msg.comm_channel.updated"
	MsgCommChannelRemoved            MsgCode = "msg.comm_channel.removed"
	MsgCommChannelDefaultSet         MsgCode = "msg.comm_channel.default_set"
	MsgCommChannelTestSent           MsgCode = "msg.comm_channel.test_sent"
)
