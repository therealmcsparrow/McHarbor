// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// Package i18n holds the message codes and translations that let the backend return codes instead of raw error text.
package i18n

// MsgCode is a machine-readable message identifier returned in API responses.
type MsgCode string

// --- Common ---
const (
	ErrUnauthorized            MsgCode = "err.unauthorized"
	ErrForbidden               MsgCode = "err.forbidden"
	ErrNotFound                MsgCode = "err.not_found"
	ErrPermissionDenied        MsgCode = "err.permission_denied"
	ErrPermissionNotElevatable MsgCode = "err.permission_not_elevatable"
	ErrInternalServer          MsgCode = "err.internal_server"
	ErrInvalidBody             MsgCode = "err.invalid_body"
	ErrRateLimitExceed         MsgCode = "err.rate_limit_exceeded"
	ErrPanicRecovery           MsgCode = "err.panic_recovery"
	ErrSelfTarget              MsgCode = "err.self_target"
	ErrProtectedTarget         MsgCode = "err.protected_target"
	ErrInvalidPath             MsgCode = "err.invalid_path"
	ErrPathTraversal           MsgCode = "err.path_traversal"
	ErrAbsolutePathReq         MsgCode = "err.absolute_path_required"
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
	ErrContainerNotFound                       MsgCode = "err.container.not_found"
	ErrContainerListFailed                     MsgCode = "err.container.list_failed"
	ErrContainerCreateFailed                   MsgCode = "err.container.create_failed"
	ErrContainerInspectFailed                  MsgCode = "err.container.inspect_failed"
	ErrContainerRemoveFailed                   MsgCode = "err.container.remove_failed"
	ErrContainerStartFailed                    MsgCode = "err.container.start_failed"
	ErrContainerStopFailed                     MsgCode = "err.container.stop_failed"
	ErrContainerRestartFailed                  MsgCode = "err.container.restart_failed"
	ErrContainerPauseFailed                    MsgCode = "err.container.pause_failed"
	ErrContainerUnpauseFailed                  MsgCode = "err.container.unpause_failed"
	ErrContainerKillFailed                     MsgCode = "err.container.kill_failed"
	ErrContainerUpdateFailed                   MsgCode = "err.container.update_failed"
	ErrContainerRenameFailed                   MsgCode = "err.container.rename_failed"
	ErrContainerRecreateFailed                 MsgCode = "err.container.recreate_failed"
	ErrContainerPruneFailed                    MsgCode = "err.container.prune_failed"
	ErrContainerLogsFailed                     MsgCode = "err.container.logs_failed"
	ErrContainerStatsFailed                    MsgCode = "err.container.stats_failed"
	ErrContainerTopFailed                      MsgCode = "err.container.top_failed"
	ErrContainerFilesFailed                    MsgCode = "err.container.files_failed"
	ErrSecretPathForbidden                     MsgCode = "err.container.secret_path_forbidden"
	ErrContainerShellsFailed                   MsgCode = "err.container.shells_failed"
	ErrContainerServicesFailed                 MsgCode = "err.container.services_failed"
	ErrContainerDiffFailed                     MsgCode = "err.container.diff_failed"
	ErrContainerExecBadRequest                 MsgCode = "err.container.exec_bad_request"
	ErrContainerExecFailed                     MsgCode = "err.container.exec_failed"
	ErrContainerExecHistoryFailed              MsgCode = "err.container.exec_history_failed"
	ErrContainerHealthFailed                   MsgCode = "err.container.health_failed"
	ErrContainerImageRequired                  MsgCode = "err.container.image_required"
	ErrContainerNameRequired                   MsgCode = "err.container.name_required"
	ErrContainerNameInvalid                    MsgCode = "err.container.name_invalid"
	ErrContainerSelfStop                       MsgCode = "err.container.self_stop"
	ErrContainerSelfRemove                     MsgCode = "err.container.self_remove"
	ErrContainerSelfRestart                    MsgCode = "err.container.self_restart"
	ErrContainerSelfPause                      MsgCode = "err.container.self_pause"
	ErrContainerSelfKill                       MsgCode = "err.container.self_kill"
	MsgContainerStarted                        MsgCode = "msg.container.started"
	MsgContainerStopped                        MsgCode = "msg.container.stopped"
	MsgContainerRestarted                      MsgCode = "msg.container.restarted"
	MsgContainerPaused                         MsgCode = "msg.container.paused"
	MsgContainerUnpaused                       MsgCode = "msg.container.unpaused"
	MsgContainerKilled                         MsgCode = "msg.container.killed"
	MsgContainerRenamed                        MsgCode = "msg.container.renamed"
	ErrContainerActionFailed                   MsgCode = "err.container.action_failed"
	ErrContainerBackupKeyMissing               MsgCode = "err.container.backup_key_missing"
	ErrContainerBackupRestoreKeyRequired       MsgCode = "err.container.backup_restore_key_required"
	ErrContainerBackupRestoreKeyInvalid        MsgCode = "err.container.backup_restore_key_invalid"
	ErrContainerBackupRestoreConfigUnsupported MsgCode = "err.container.backup_restore_config_unsupported"
	ErrContainerBackupRestoreConfigFsConflict  MsgCode = "err.container.backup_restore_config_fs_conflict"
	ErrContainerBackupRunNotCancellable        MsgCode = "err.container.backup_run_not_cancellable"
	ErrContainerBackupDestinationRequired      MsgCode = "err.container.backup_destination_required"
	ErrContainerBackupRestoreTreeEntriesFailed MsgCode = "err.container.backup_restore_tree_entries_failed"
	ErrContainerBackupRestoreMountMissing      MsgCode = "err.container.backup_restore_mount_missing"
	ErrContainerBackupRestoreConfigMissing     MsgCode = "err.container.backup_restore_config_missing"
	MsgContainerActionCompleted                MsgCode = "msg.container.action_completed"
	ErrContainerFileReadFailed                 MsgCode = "err.container.file_read_failed"
	ErrContainerFileWriteFailed                MsgCode = "err.container.file_write_failed"
	ErrContainerFileUploadFailed               MsgCode = "err.container.file_upload_failed"
	ErrContainerFileMkdirFailed                MsgCode = "err.container.file_mkdir_failed"
	ErrContainerFileRenameFailed               MsgCode = "err.container.file_rename_failed"
	ErrContainerFileChmodFailed                MsgCode = "err.container.file_chmod_failed"
	ErrContainerFileDeleteFailed               MsgCode = "err.container.file_delete_failed"
	ErrContainerFileTooLarge                   MsgCode = "err.container.file_too_large"
	ErrContainerFileNameRequired               MsgCode = "err.container.file_name_required"
	ErrContainerFileModeInvalid                MsgCode = "err.container.file_mode_invalid"
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
	ErrNetworkBuiltIn          MsgCode = "err.network.builtin"
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
	ErrStackLinkFailed        MsgCode = "err.stack.link_failed"
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

// --- Step-up re-authentication ---
const (
	ErrSudoFailed   MsgCode = "err.auth.sudo_failed"
	ErrSudoRequired MsgCode = "err.auth.sudo_required"
)

// --- Secrets vault ---
const (
	ErrSecretNotFound          MsgCode = "err.secret.not_found"
	ErrSecretNameInvalid       MsgCode = "err.secret.name_invalid"
	ErrSecretDuplicate         MsgCode = "err.secret.duplicate"
	ErrSecretInUse             MsgCode = "err.secret.in_use"
	ErrSecretSystemImmutable   MsgCode = "err.secret.system_immutable"
	ErrSecretGlobalDenied      MsgCode = "err.secret.global_denied"
	ErrSecretCreateFailed      MsgCode = "err.secret.create_failed"
	ErrSecretRotateFailed      MsgCode = "err.secret.rotate_failed"
	ErrSecretDeleteFailed      MsgCode = "err.secret.delete_failed"
	ErrSecretDecryptFailed     MsgCode = "err.secret.decrypt_failed"
	ErrSecretRevealDenied      MsgCode = "err.secret.reveal_denied"
	ErrSecretRevealFailed      MsgCode = "err.secret.reveal_failed"
	ErrSecretRevealReason      MsgCode = "err.secret.reveal_reason"
	ErrSecretRevealRateLimited MsgCode = "err.secret.reveal_rate_limited"
	ErrSecretListFailed        MsgCode = "err.secret.list_failed"
	ErrSecretRollbackFailed    MsgCode = "err.secret.rollback_failed"
	ErrSecretGenerateFailed    MsgCode = "err.secret.generate_failed"
	ErrSecretImpactFailed      MsgCode = "err.secret.impact_failed"
	ErrSecretApplyFailed       MsgCode = "err.secret.apply_failed"
	ErrSecretPruneFailed       MsgCode = "err.secret.prune_failed"
	MsgSecretCreated           MsgCode = "msg.secret.created"
	MsgSecretRotated           MsgCode = "msg.secret.rotated"
	MsgSecretUpdated           MsgCode = "msg.secret.updated"
	MsgSecretDeleted           MsgCode = "msg.secret.deleted"
	MsgSecretRolledBack        MsgCode = "msg.secret.rolled_back"
	MsgSecretRevealed          MsgCode = "msg.secret.revealed"
	MsgSecretApplied           MsgCode = "msg.secret.applied"
	MsgSecretReferencesPruned  MsgCode = "msg.secret.references_pruned"
)

// --- Environments ---
const (
	ErrEnvNotFound        MsgCode = "err.env.not_found"
	ErrEnvListFailed      MsgCode = "err.env.list_failed"
	ErrEnvCreateFailed    MsgCode = "err.env.create_failed"
	ErrEnvUpdateFailed    MsgCode = "err.env.update_failed"
	ErrEnvRemoveFailed    MsgCode = "err.env.remove_failed"
	ErrEnvNameRequired    MsgCode = "err.env.name_required"
	ErrEnvConnRequired    MsgCode = "err.env.connection_required"
	ErrEnvConnFailed      MsgCode = "err.env.connection_failed"
	ErrEnvInvalidConnType MsgCode = "err.env.invalid_connection_type"

	// Migration and merge.
	ErrEnvSameConnType       MsgCode = "err.env.same_connection_type"
	ErrEnvNoPendingMigration MsgCode = "err.env.no_pending_migration"
	ErrEnvMergeIntoSelf      MsgCode = "err.env.merge_into_self"
	ErrEnvMergeFailed        MsgCode = "err.env.merge_failed"

	ErrEnvInvalidAgentToken      MsgCode = "err.env.invalid_agent_token"
	ErrEnvInvalidTimezone        MsgCode = "err.env.invalid_timezone"
	ErrEnvInvalidLogRetention    MsgCode = "err.env.invalid_log_retention"
	ErrEnvInvalidContainerPrune  MsgCode = "err.env.invalid_container_prune_days"
	ErrEnvInvalidMetricRetention MsgCode = "err.env.invalid_metric_retention"
	ErrEnvInvalidAutoUpdate      MsgCode = "err.env.invalid_auto_update_window"
	ErrEnvInvalidAutoUpdateDays  MsgCode = "err.env.invalid_auto_update_days"
	MsgEnvCreated                MsgCode = "msg.env.created"
	MsgEnvRemoved                MsgCode = "msg.env.removed"
	MsgEnvMigrated               MsgCode = "msg.env.migrated"
	MsgEnvMigrationPending       MsgCode = "msg.env.migration_pending"
	MsgEnvMigrationCancelled     MsgCode = "msg.env.migration_cancelled"
	MsgEnvMerged                 MsgCode = "msg.env.merged"
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
	// ErrLogsAgentOutdated names the one log failure the operator can act on:
	// the source exists, their agent is too old to read it.
	ErrLogsAgentOutdated MsgCode = "err.logs.agent_outdated"
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
	ErrGitFailed MsgCode = "err.git.failed"
	// ErrGitSyncFailed is returned when a clone or fetch fails. The real
	// transport error is logged server-side and never sent to the client.
	ErrGitSyncFailed MsgCode = "err.git.sync_failed"
	// ErrGitAuthUnsupported names the one failure a user can act on: the
	// repository asks for an auth type the sync does not implement yet.
	ErrGitAuthUnsupported MsgCode = "err.git.auth_unsupported"
	ErrGitCloneFailed     MsgCode = "err.git.clone_failed"
	ErrGitNotFound        MsgCode = "err.git.not_found"
	ErrGitListFailed      MsgCode = "err.git.list_failed"
	ErrGitCreateFailed    MsgCode = "err.git.create_failed"
	ErrGitUpdateFailed    MsgCode = "err.git.update_failed"
	ErrGitRemoveFailed    MsgCode = "err.git.remove_failed"
	ErrGitNameRequired    MsgCode = "err.git.name_required"
	ErrGitUrlRequired     MsgCode = "err.git.url_required"
)

// --- GitOps ---
const (
	ErrGitOpsPipelineNotFound     MsgCode = "err.gitops.pipeline_not_found"
	ErrGitOpsStageNotFound        MsgCode = "err.gitops.stage_not_found"
	ErrGitOpsApprovalNotFound     MsgCode = "err.gitops.approval_not_found"
	ErrGitOpsPreviewNotFound      MsgCode = "err.gitops.preview_not_found"
	ErrGitOpsListFailed           MsgCode = "err.gitops.list_failed"
	ErrGitOpsCreateFailed         MsgCode = "err.gitops.create_failed"
	ErrGitOpsUpdateFailed         MsgCode = "err.gitops.update_failed"
	ErrGitOpsRemoveFailed         MsgCode = "err.gitops.remove_failed"
	ErrGitOpsPromoteFailed        MsgCode = "err.gitops.promote_failed"
	ErrGitOpsApproveFailed        MsgCode = "err.gitops.approve_failed"
	ErrGitOpsRejectFailed         MsgCode = "err.gitops.reject_failed"
	ErrGitOpsPreviewFailed        MsgCode = "err.gitops.preview_failed"
	ErrGitOpsWebhookFailed        MsgCode = "err.gitops.webhook_failed"
	ErrGitOpsNameRequired         MsgCode = "err.gitops.name_required"
	ErrGitOpsRepoMissing          MsgCode = "err.gitops.repo_missing"
	ErrGitOpsStageOrderInvalid    MsgCode = "err.gitops.stage_order_invalid"
	ErrGitOpsTargetEnvRequired    MsgCode = "err.gitops.target_env_required"
	ErrGitOpsApprovalExpired      MsgCode = "err.gitops.approval_expired"
	ErrGitOpsNoCommitDetected     MsgCode = "err.gitops.no_commit_detected"
	ErrGitOpsWebhookSignatureBad  MsgCode = "err.gitops.webhook_signature_bad"
	ErrGitOpsWebhookSecretMissing MsgCode = "err.gitops.webhook_secret_missing"
	ErrGitOpsPreviewDestroyFailed MsgCode = "err.gitops.preview_destroy_failed"
	ErrGitOpsPreviewQuotaReached  MsgCode = "err.gitops.preview_quota_reached"
	MsgGitOpsWebhookIgnored       MsgCode = "msg.gitops.webhook_ignored"
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
	ErrAgentDeployPrereqFailed     MsgCode = "err.agent.deploy_prereq_failed"
	ErrAgentPrereqNoneSelected     MsgCode = "err.agent.prereq_none_selected"
	ErrAgentPrereqNoEscalation     MsgCode = "err.agent.prereq_no_escalation"
	ErrAgentDeploySecretUnresolved MsgCode = "err.agent.deploy_secret_unresolved"
	ErrAgentDeployNoSaved          MsgCode = "err.agent.deploy_no_saved"
	ErrAgentDeployCommandFailed    MsgCode = "err.agent.deploy_command_failed"
	ErrAgentInstallTokenFailed     MsgCode = "err.agent.install_token_failed"
	ErrAgentUpdateFailed           MsgCode = "err.agent.update_failed"
	ErrAgentLogsFailed             MsgCode = "err.agent.logs_failed"
	MsgAgentTokenRegenerated       MsgCode = "msg.agent.token_regenerated"
	MsgAgentDeployed               MsgCode = "msg.agent.deployed"
)

// --- Settings ---
const (
	ErrSettingsFailed                MsgCode = "err.settings.failed"
	ErrSettingsUpdateFailed          MsgCode = "err.settings.update_failed"
	ErrSettingsNoSettings            MsgCode = "err.settings.no_settings"
	ErrSettingsTLSInvalid            MsgCode = "err.settings.tls_invalid"
	ErrSettingsTLSPairReq            MsgCode = "err.settings.tls_pair_required"
	ErrSettingsTLSRegenerate         MsgCode = "err.settings.tls_regenerate"
	ErrSettingsTLSRegenUserCert      MsgCode = "err.settings.tls_regen_user_cert"
	ErrSettingsTLSNoCA               MsgCode = "err.settings.tls_no_ca"
	ErrSettingsBackupKeyInvalid      MsgCode = "err.settings.backup_key_invalid"
	ErrSettingsComposeProjectMissing MsgCode = "err.settings.compose_project_missing"
	ErrSettingsIdleTimeoutInvalid    MsgCode = "err.settings.idle_timeout_invalid"
	ErrSettingsMFARequirementInvalid MsgCode = "err.settings.mfa_requirement_invalid"
	MsgSettingsUpdated               MsgCode = "msg.settings.updated"
	MsgAgentSettingsUpdated          MsgCode = "msg.agent_settings.updated"
	ErrAgentSettingsInvalid          MsgCode = "err.agent_settings.invalid"
	ErrRetentionSettingsInvalid      MsgCode = "err.retention_settings.invalid"
	MsgRetentionSettingsUpdated      MsgCode = "msg.retention_settings.updated"
	ErrToastSettingsInvalid          MsgCode = "err.toast_settings.invalid"
	MsgToastSettingsUpdated          MsgCode = "msg.toast_settings.updated"
	ErrAppearanceSettingsInvalid     MsgCode = "err.appearance_settings.invalid"
	MsgAppearanceSettingsUpdated     MsgCode = "msg.appearance_settings.updated"
	ErrBrandingInvalid               MsgCode = "err.branding.invalid"
	MsgBrandingUpdated               MsgCode = "msg.branding.updated"
	MsgAuditLogPurged                MsgCode = "msg.audit_log.purged"
	MsgActivityLogPurged             MsgCode = "msg.activity_log.purged"
)

// --- Users ---
const (
	ErrUserNotFound                  MsgCode = "err.user.not_found"
	ErrProfileIdentityProviderLocked MsgCode = "err.profile.identity_provider_locked"
	ErrUserListFailed                MsgCode = "err.user.list_failed"
	ErrUserCreateFailed              MsgCode = "err.user.create_failed"
	ErrUserCreateNoDefault           MsgCode = "err.user.create_no_default_role"
	ErrUserUpdateFailed              MsgCode = "err.user.update_failed"
	ErrUserRemoveFailed              MsgCode = "err.user.remove_failed"
	ErrUserSelfDelete                MsgCode = "err.user.self_delete"
	ErrUserPasswordRequired          MsgCode = "err.user.password_required"
	ErrUserPasswordIncorrect         MsgCode = "err.user.password_incorrect"
	MsgUserPasswordUpdated           MsgCode = "msg.user.password_updated"
)

// --- Activity ---
const (
	ErrActivityListFailed MsgCode = "err.activity.list_failed"
)

// --- Audit ---
const (
	ErrAuditListFailed             MsgCode = "err.audit.list_failed"
	ErrAuditForwarderListFailed    MsgCode = "err.audit.forwarder_list_failed"
	ErrAuditForwarderSaveFailed    MsgCode = "err.audit.forwarder_save_failed"
	ErrAuditForwarderTestFailed    MsgCode = "err.audit.forwarder_test_failed"
	ErrAuditForwarderHistoryFailed MsgCode = "err.audit.forwarder_history_failed"
	ErrAuditForwarderUnknown       MsgCode = "err.audit.forwarder_unknown"
	ErrAuditForwarderTypeRequired  MsgCode = "err.audit.forwarder_type_required"
	ErrAuditForwarderDeleteFailed  MsgCode = "err.audit.forwarder_delete_failed"
	ErrAuditForwarderNotConfigured MsgCode = "err.audit.forwarder_not_configured"
	ErrAuditForwarderFieldRequired MsgCode = "err.audit.forwarder_field_required"
	ErrAuditForwarderNameRequired  MsgCode = "err.audit.forwarder_name_required"
	ErrAuditForwarderCreateFailed  MsgCode = "err.audit.forwarder_create_failed"
	MsgAuditForwarderSaved         MsgCode = "msg.audit.forwarder_saved"
	MsgAuditForwarderTested        MsgCode = "msg.audit.forwarder_tested"
	MsgAuditForwarderDeleted       MsgCode = "msg.audit.forwarder_deleted"
	MsgAuditForwarderCreated       MsgCode = "msg.audit.forwarder_created"
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

// --- Table Views ---
const (
	ErrTableViewNotFound  MsgCode = "err.table_view.not_found"
	ErrTableViewInvalid   MsgCode = "err.table_view.invalid"
	ErrTableViewLimit     MsgCode = "err.table_view.limit"
	ErrTableViewNameTaken MsgCode = "err.table_view.name_taken"
	ErrTableViewFailed    MsgCode = "err.table_view.failed"
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

	// Scanner administration — the Security > Scanners tab, which
	// configures instances of the plugin-provided scanning engines.
	// ErrScannerUnavailable is deliberately absent: the existing
	// ErrScannerNotAvailable above already says exactly this, and
	// minting a second code for it would leave two strings a
	// translator has to keep in sync and an operator could see either
	// of, depending on which code path they hit.
	ErrScannerAdapterUnknown MsgCode = "err.scanner.adapter_unknown"
	ErrScannerNotConfigured  MsgCode = "err.scanner.not_configured"
	ErrNoScannerConfigured   MsgCode = "err.scanner.none_configured"
	ErrScannerNameRequired   MsgCode = "err.scanner.name_required"
	ErrScannerFieldRequired  MsgCode = "err.scanner.field_required"
	ErrScannerListFailed     MsgCode = "err.scanner.list_failed"
	ErrScannerCreateFailed   MsgCode = "err.scanner.create_failed"
	ErrScannerSaveFailed     MsgCode = "err.scanner.save_failed"
	ErrScannerDeleteFailed   MsgCode = "err.scanner.delete_failed"
	ErrScannerTestFailed     MsgCode = "err.scanner.test_failed"
	MsgScannerSaved          MsgCode = "msg.scanner.saved"
	MsgScannerDeleted        MsgCode = "msg.scanner.deleted"
	MsgScannerTested         MsgCode = "msg.scanner.tested"
)

// --- Updates ---
const (
	ErrUpdateCheckFailed MsgCode = "err.update.check_failed"

	// Host OS inventory and package updates on an environment's host.
	ErrHostOSInfoFailed        MsgCode = "err.host.os_info_failed"
	ErrHostDevicesFailed       MsgCode = "err.host.devices_failed"
	ErrHostUpdateStateFailed   MsgCode = "err.host.update_state_failed"
	ErrHostUpdateApplyFailed   MsgCode = "err.host.update_apply_failed"
	ErrHostUpdateJobNotFound   MsgCode = "err.host.update_job_not_found"
	MsgHostUpdateScheduleSaved MsgCode = "msg.host.update_schedule_saved"
	MsgAgentAccessSaved        MsgCode = "msg.agent.access_saved"
	ErrUpdateNotFound          MsgCode = "err.update.not_found"
	ErrUpdateListFailed        MsgCode = "err.update.list_failed"
	ErrUpdateCreateFailed      MsgCode = "err.update.create_failed"
	ErrUpdateRemoveFailed      MsgCode = "err.update.remove_failed"
	ErrUpdateNameRequired      MsgCode = "err.update.name_required"
)

// --- Plugins ---
const (
	ErrPluginListFailed       MsgCode = "err.plugin.list_failed"
	ErrPluginNotFound         MsgCode = "err.plugin.not_found"
	ErrPluginInstallFailed    MsgCode = "err.plugin.install_failed"
	ErrPluginBuiltForMismatch MsgCode = "err.plugin.built_for_mismatch"
	ErrPluginRemoveFailed     MsgCode = "err.plugin.remove_failed"
	ErrPluginNameRequired     MsgCode = "err.plugin.name_required"
	ErrPluginSourceRequired   MsgCode = "err.plugin.source_required"
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
	ErrAlertConditionUnknown    MsgCode = "err.alert.condition_unknown"
	ErrAlertConditionInvalid    MsgCode = "err.alert.condition_invalid"
	ErrAlertTestFailed          MsgCode = "err.alert.test_failed"
	ErrAlertEventListFailed     MsgCode = "err.alert.event_list_failed"
	MsgAlertTestSent            MsgCode = "msg.alert.test_sent"
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
	ErrMetricsHistoryFailed       MsgCode = "err.metrics.history_failed"
	ErrMetricsRangeInvalid        MsgCode = "err.metrics.range_invalid"
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
	ErrAppStoreListFailed      MsgCode = "err.appstore.list_failed"
	ErrAppStoreInstallFailed   MsgCode = "err.appstore.install_failed"
	ErrAppStoreUninstallFailed MsgCode = "err.appstore.uninstall_failed"
	ErrAppStoreNotFound        MsgCode = "err.appstore.not_found"
	ErrAppStoreSlugRequired    MsgCode = "err.appstore.slug_required"
)

// --- RBAC ---
const (
	ErrRBACPermissionDenied MsgCode = "err.rbac.permission_denied"
)

// --- System ---
// Codes for self-management actions on the McHarbor application
// (restart, future install / uninstall hooks). The endpoint that
// schedules a restart lives in modules/system.
const (
	ErrSystemRestartNotScheduled  MsgCode = "err.system.restart_not_scheduled"
	ErrSystemRestartInspectFailed MsgCode = "err.system.restart_inspect_failed"
	ErrSystemRestartNotLocal      MsgCode = "err.system.restart_not_local"
	ErrSystemSelfUpdateFailed     MsgCode = "err.system.self_update_failed"
	ErrSystemSelfUpdateNotLocal   MsgCode = "err.system.self_update_not_local"
	ErrSystemResetFailed          MsgCode = "err.system.reset_failed"
	ErrSystemResetNotLocal        MsgCode = "err.system.reset_not_local"
	ErrSystemResetConfirmMismatch MsgCode = "err.system.reset_confirm_mismatch"
	MsgSystemRestartScheduled     MsgCode = "msg.system.restart_scheduled"
	MsgSystemSelfUpdateScheduled  MsgCode = "msg.system.self_update_scheduled"
	MsgAgentUpdateTriggered       MsgCode = "msg.agent.update_triggered"
	MsgSystemResetScheduled       MsgCode = "msg.system.reset_scheduled"
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

	ErrAPIKeyGrantsRequired     MsgCode = "err.api_key.grants_required"
	ErrAPIKeyGrantInvalid       MsgCode = "err.api_key.grant_invalid"
	ErrAPIKeyGrantExceedsCaller MsgCode = "err.api_key.grant_exceeds_caller"
	ErrAPIKeyUpdateFailed       MsgCode = "err.api_key.update_failed"
	ErrAPIKeyDeleteFailed       MsgCode = "err.api_key.delete_failed"
	ErrAPIKeyForbidden          MsgCode = "err.api_key.forbidden"
	MsgAPIKeyUpdated            MsgCode = "msg.api_key.updated"
	MsgAPIKeyDeleted            MsgCode = "msg.api_key.deleted"

	ErrAPIKeyUsageFailed      MsgCode = "err.api_key.usage_failed"
	ErrAPIKeyRangeInvalid     MsgCode = "err.api_key.range_invalid"
	ErrAPIKeyUsageClearFailed MsgCode = "err.api_key.usage_clear_failed"
	MsgAPIKeyUsageCleared     MsgCode = "msg.api_key.usage_cleared"
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
const ()

// --- Communication Channels ---
const (
	ErrCommChannelNotFound              MsgCode = "err.comm_channel.not_found"
	ErrCommChannelListFailed            MsgCode = "err.comm_channel.list_failed"
	ErrCommChannelCreateFailed          MsgCode = "err.comm_channel.create_failed"
	ErrCommChannelUpdateFailed          MsgCode = "err.comm_channel.update_failed"
	ErrCommChannelRemoveFailed          MsgCode = "err.comm_channel.remove_failed"
	ErrCommChannelNameRequired          MsgCode = "err.comm_channel.name_required"
	ErrCommChannelTypeInvalid           MsgCode = "err.comm_channel.type_invalid"
	ErrCommChannelWebhookRequired       MsgCode = "err.comm_channel.webhook_required"
	ErrCommChannelServerRequired        MsgCode = "err.comm_channel.server_required"
	ErrCommChannelTokenRequired         MsgCode = "err.comm_channel.token_required"
	ErrCommChannelTopicRequired         MsgCode = "err.comm_channel.topic_required"
	ErrCommChannelChatIDRequired        MsgCode = "err.comm_channel.chat_id_required"
	ErrCommChannelPhoneRequired         MsgCode = "err.comm_channel.phone_required"
	ErrCommChannelSenderRequired        MsgCode = "err.comm_channel.sender_required"
	ErrCommChannelRecipientsRequired    MsgCode = "err.comm_channel.recipients_required"
	ErrCommChannelPriorityInvalid       MsgCode = "err.comm_channel.priority_invalid"
	ErrCommChannelTelegramAdminRequired MsgCode = "err.comm_channel.telegram_admin_required"
	ErrCommChannelTestFailed            MsgCode = "err.comm_channel.test_failed"
	MsgCommChannelCreated               MsgCode = "msg.comm_channel.created"
	MsgCommChannelUpdated               MsgCode = "msg.comm_channel.updated"
	MsgCommChannelRemoved               MsgCode = "msg.comm_channel.removed"
	MsgCommChannelDefaultSet            MsgCode = "msg.comm_channel.default_set"
	MsgCommChannelTestSent              MsgCode = "msg.comm_channel.test_sent"
)

// --- Password reset / policy ---
const (
	ErrPasswordResetInvalid  MsgCode = "err.password_reset.invalid"
	ErrPasswordResetExpired  MsgCode = "err.password_reset.expired"
	ErrPasswordResetConsumed MsgCode = "err.password_reset.consumed"
	ErrPasswordResetFailed   MsgCode = "err.password_reset.failed"
	ErrPasswordTooShort      MsgCode = "err.password.too_short"
	ErrPasswordTooWeak       MsgCode = "err.password.too_weak"
	ErrPasswordReused        MsgCode = "err.password.reused"
	ErrPasswordNoUpper       MsgCode = "err.password.no_upper"
	ErrPasswordNoLower       MsgCode = "err.password.no_lower"
	ErrPasswordNoDigit       MsgCode = "err.password.no_digit"
	ErrPasswordNoSymbol      MsgCode = "err.password.no_symbol"
	MsgPasswordResetSent     MsgCode = "msg.password_reset.sent"
	MsgPasswordResetConsumed MsgCode = "msg.password_reset.consumed"
)

// --- SCIM ---
const (
	ErrSCIMFilterInvalid   MsgCode = "err.scim.filter_invalid"
	ErrSCIMTokenInvalid    MsgCode = "err.scim.token_invalid"
	ErrSCIMNotFound        MsgCode = "err.scim.not_found"
	ErrSCIMCreateFailed    MsgCode = "err.scim.create_failed"
	ErrSCIMUpdateFailed    MsgCode = "err.scim.update_failed"
	ErrSCIMDeleteFailed    MsgCode = "err.scim.delete_failed"
	MsgSCIMEndpointCreated MsgCode = "msg.scim.endpoint_created"
	MsgSCIMEndpointUpdated MsgCode = "msg.scim.endpoint_updated"
	MsgSCIMEndpointRemoved MsgCode = "msg.scim.endpoint_removed"
)

// --- Plugin install / update ---
const (
	ErrPluginSlugRequired         MsgCode = "err.plugin.slug_required"
	ErrPluginURLOrTarballRequired MsgCode = "err.plugin.url_or_tarball_required"
	ErrPluginUpdateNoCatalogEntry MsgCode = "err.plugin.update_no_catalog_entry"
	ErrPluginUpdateFailed         MsgCode = "err.plugin.update_failed"
	ErrPluginInvalidTarball       MsgCode = "err.plugin.invalid_tarball"
	ErrPluginSignatureInvalid     MsgCode = "err.plugin.signature_invalid"
	ErrPluginNotInstalled         MsgCode = "err.plugin.not_installed"
	ErrPluginAlreadyInstalled     MsgCode = "err.plugin.already_installed"
	ErrPluginUnsignedUnconfirmed  MsgCode = "err.plugin.unsigned_confirmation_required"
	MsgPluginInstalled            MsgCode = "msg.plugin.installed"
	MsgPluginUpdated              MsgCode = "msg.plugin.updated"
	MsgPluginUninstalled          MsgCode = "msg.plugin.uninstalled"
)

// --- Plugin install flow (additional) ---
const (
	ErrPluginValuesRequired   MsgCode = "err.plugin.values_required"
	ErrPluginManifestInvalid  MsgCode = "err.plugin.manifest_invalid"
	ErrPluginPermissionInUse  MsgCode = "err.plugin.permission_in_use"
	ErrPluginUninstallBlocked MsgCode = "err.plugin.uninstall_blocked"
	ErrPlatformUnsupported    MsgCode = "err.platform.unsupported"
)

// --- SCIM (additional) ---
const (
	ErrSCIMEndpointNotFound  MsgCode = "err.scim.endpoint_not_found"
	ErrSCIMEndpointNameTaken MsgCode = "err.scim.endpoint_name_taken"
	MsgSCIMSynced            MsgCode = "msg.scim.synced"
)

// --- User account lifecycle ---
const (
	ErrUserAlreadyDisabled        MsgCode = "err.user.already_disabled"
	ErrUserNotDisabled            MsgCode = "err.user.not_disabled"
	ErrUserLastAdmin              MsgCode = "err.user.last_admin"
	ErrUserIdentityProviderLocked MsgCode = "err.user.identity_provider_locked"
	ErrUserSessionListFailed      MsgCode = "err.user.session_list_failed"
	ErrUserSessionRevokeFailed    MsgCode = "err.user.session_revoke_failed"
	ErrUserPasswordSetFailed      MsgCode = "err.user.password_set_failed"
	MsgUserDisabled               MsgCode = "msg.user.disabled"
	MsgUserEnabled                MsgCode = "msg.user.enabled"
	MsgUserUnlocked               MsgCode = "msg.user.unlocked"
	MsgUserForceReset             MsgCode = "msg.user.force_reset"
	MsgUserPasswordAdminSet       MsgCode = "msg.user.password_admin_set"
	MsgUserSessionsRevoked        MsgCode = "msg.user.sessions_revoked"
	MsgUserSessionRevoked         MsgCode = "msg.user.session_revoked"
)

// --- RBAC policies ---
const (
	ErrRBACPolicyNotFound          MsgCode = "err.rbac.policy_not_found"
	ErrRBACPolicyInvalid           MsgCode = "err.rbac.policy_invalid"
	ErrRBACPolicyCreateFailed      MsgCode = "err.rbac.policy_create_failed"
	ErrRBACPolicyUpdateFailed      MsgCode = "err.rbac.policy_update_failed"
	ErrRBACPolicyDeleteFailed      MsgCode = "err.rbac.policy_delete_failed"
	ErrRBACPolicyExpressionInvalid MsgCode = "err.rbac.policy_expression_invalid"
	ErrRBACPolicyExpressionTimeout MsgCode = "err.rbac.policy_expression_timeout"
	ErrRBACTestFailed              MsgCode = "err.rbac.test_failed"
	ErrRBACPolicyTimeout           MsgCode = "err.rbac.policy_timeout"
	MsgRBACPolicyCreated           MsgCode = "msg.rbac.policy_created"
	MsgRBACPolicyUpdated           MsgCode = "msg.rbac.policy_updated"
	MsgRBACPolicyRemoved           MsgCode = "msg.rbac.policy_removed"
	MsgRBACPolicyEnabled           MsgCode = "msg.rbac.policy_enabled"
	MsgRBACPolicyDisabled          MsgCode = "msg.rbac.policy_disabled"

	ErrAIProviderListFailed   MsgCode = "err.ai_provider.list_failed"
	ErrAIProviderNotFound     MsgCode = "err.ai_provider.not_found"
	ErrAIProviderNameRequired MsgCode = "err.ai_provider.name_required"
	ErrAIProviderTypeRequired MsgCode = "err.ai_provider.type_required"
	ErrAIProviderCreateFailed MsgCode = "err.ai_provider.create_failed"
	ErrAIProviderUpdateFailed MsgCode = "err.ai_provider.update_failed"
	ErrAIProviderRemoveFailed MsgCode = "err.ai_provider.remove_failed"
	ErrAIProviderTestFailed   MsgCode = "err.ai_provider.test_failed"
	MsgAIProviderDefaultSet   MsgCode = "msg.ai_provider.default_set"

	// AI provider OAuth ("sign in to connect", the alternative to an API key).
	ErrAIProviderOAuthNotConfigured MsgCode = "err.ai_provider.oauth_not_configured"
	ErrAIProviderOAuthRejected      MsgCode = "err.ai_provider.oauth_rejected"
	ErrAIProviderOAuthFailed        MsgCode = "err.ai_provider.oauth_failed"
	MsgAIProviderOAuthDisconnected  MsgCode = "msg.ai_provider.oauth_disconnected"
	ErrAIDisabled                   MsgCode = "err.ai.disabled"
	ErrAIRateLimited                MsgCode = "err.ai.rate_limited"
)

// --- MFA ---
const (
	ErrMFAStatusFailed     MsgCode = "err.mfa.status_failed"
	ErrMFAEnrollFailed     MsgCode = "err.mfa.enroll_failed"
	ErrMFAAlreadyEnabled   MsgCode = "err.mfa.already_enabled"
	ErrMFANotEnrolled      MsgCode = "err.mfa.not_enrolled"
	ErrMFAInvalidCode      MsgCode = "err.mfa.invalid_code"
	ErrMFADisableFailed    MsgCode = "err.mfa.disable_failed"
	ErrMFAChallengeInvalid MsgCode = "err.mfa.challenge_invalid"
	ErrMFARequired         MsgCode = "err.mfa.required"
)

// --- License ---
const (
	ErrLicenseReadFailed          MsgCode = "err.license.read_failed"
	ErrLicenseNotActivated        MsgCode = "err.license.not_activated"
	ErrLicenseKeyRequired         MsgCode = "err.license.key_required"
	ErrLicenseKeyUnknown          MsgCode = "err.license.key_unknown"
	ErrLicenseSeatLimit           MsgCode = "err.license.seat_limit"
	ErrLicensePendingApproval     MsgCode = "err.license.pending_approval"
	ErrLicenseNotStarted          MsgCode = "err.license.not_started"
	ErrLicenseExpired             MsgCode = "err.license.expired"
	ErrLicenseSuspended           MsgCode = "err.license.suspended"
	ErrLicenseRevoked             MsgCode = "err.license.revoked"
	ErrLicenseServerNotConfigured MsgCode = "err.license.server_not_configured"
	ErrLicenseServerUnavailable   MsgCode = "err.license.server_unavailable"
	ErrLicenseServerRejected      MsgCode = "err.license.server_rejected"
	ErrLicenseRequestRejected     MsgCode = "err.license.request_rejected"
	ErrLicenseRequestFailed       MsgCode = "err.license.request_failed"
	ErrLicenseDeactivateFailed    MsgCode = "err.license.deactivate_failed"
	ErrLicenseNoticeInvalid       MsgCode = "err.license.notice_invalid"
	MsgLicenseDeactivated         MsgCode = "msg.license.deactivated"
	MsgLicenseRefreshed           MsgCode = "msg.license.refreshed"
	MsgLicenseNoticeRecorded      MsgCode = "msg.license.notice_recorded"

	// Compute: the core half of KVM virtual machines and LXC system
	// containers. Only the codes core itself returns live here — the console
	// gateway's and the version gate's. Everything the compute plugins
	// surface (per-guest, per-pool, per-image errors) belongs to their own
	// i18n namespaces, because the plugins own those features.
	ErrComputeAgentRequired  MsgCode = "err.compute.agent_required"
	ErrComputeAgentOffline   MsgCode = "err.compute.agent_offline"
	ErrComputeAgentTooOld    MsgCode = "err.compute.agent_too_old"
	ErrComputeUnsupportedOS  MsgCode = "err.compute.unsupported_os"
	ErrConsoleTicketInvalid  MsgCode = "err.compute.console_ticket_invalid"
	ErrConsoleTooManyViewers MsgCode = "err.compute.console_too_many_viewers"
	ErrConsoleUnavailable    MsgCode = "err.compute.console_unavailable"
	ErrConsoleKindInvalid    MsgCode = "err.compute.console_kind_invalid"
)
