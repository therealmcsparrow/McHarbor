# Errors

Every failure carries a `code`. It is stable across releases and across languages, and it is
the only field a client should branch on. `error` is the same message already translated
into the request's `Accept-Language`, and its wording is not a contract.

```json
{
  "success": false,
  "error": "Container not found",
  "code": "err.container.not_found"
}
```

## Reading a code

`err.<area>.<condition>` for failures, `msg.<area>.<outcome>` for confirmations. The handful
of codes with no area (`err.unauthorized`, `err.forbidden`, `err.not_found`,
`err.invalid_body`, `err.internal_server`) can come from anywhere.

A `msg.*` code is not an error. It travels with `success: true` on endpoints whose only
payload is a confirmation: `POST /api/containers/{id}/start` answers
`{ "success": true, "data": { "message": "Container started" }, "code": "msg.container.started" }`.

## The codes you will meet everywhere

| Code | HTTP | Means |
|---|---|---|
| `err.unauthorized` | 401 | No credential, or one that no longer works |
| `err.auth.required` | 401 | The route needs a session and there is none |
| `err.forbidden` | 403 | Authenticated, but not allowed |
| `err.permission_denied` | 403 | The specific permission check failed |
| `err.not_found` | 404 | No such row |
| `err.invalid_body` | 400 | Unparseable JSON, or a required field missing |
| `err.internal_server` | 500 | Something broke. The real cause is in the server log |
| `err.rate_limit_exceeded` | 429 | The auth group's 10-per-minute ceiling |
| `err.panic_recovery` | 500 | A handler panicked and the recovery middleware caught it |
| `err.protected_target` | 400 | The resource is protected and must be unlocked first |
| `err.self_target` | 400 | The action would act on the caller, or on McHarbor's own container |
| `err.path_traversal`, `err.invalid_path`, `err.absolute_path_required` | 400 | A file path failed validation |

## Codes worth handling specifically

| Code | Why it matters |
|---|---|
| `err.api_key.invalid` / `.revoked` / `.expired` | Distinguishes "wrong key" from "key that used to work"; retry logic differs |
| `err.mfa.challenge_invalid` / `err.mfa.invalid_code` | The first means start over, the second means try the digits again |
| `err.password_reset.expired` / `.consumed` / `.invalid` | Reset-link failures a user needs told apart |
| `err.container.self_stop` / `.self_remove` / `.self_restart` / `.self_kill` / `.self_pause` | McHarbor refusing to act on its own container |
| `err.container.backup_restore_key_required` / `.key_invalid` | A restore needs the backup encryption key |
| `err.permission_not_elevatable` | The requested key cannot be granted temporarily at all |
| `err.system.reset_confirm_mismatch` | The double-confirmation string did not match |

## Every code

660 codes, grouped as they are grouped in the source. The name in brackets is the Go
constant, for anyone reading the backend alongside this.

## Common (14)
- err.unauthorized   [ErrUnauthorized]
- err.forbidden   [ErrForbidden]
- err.not_found   [ErrNotFound]
- err.permission_denied   [ErrPermissionDenied]
- err.permission_not_elevatable   [ErrPermissionNotElevatable]
- err.internal_server   [ErrInternalServer]
- err.invalid_body   [ErrInvalidBody]
- err.rate_limit_exceeded   [ErrRateLimitExceed]
- err.panic_recovery   [ErrPanicRecovery]
- err.self_target   [ErrSelfTarget]
- err.protected_target   [ErrProtectedTarget]
- err.invalid_path   [ErrInvalidPath]
- err.path_traversal   [ErrPathTraversal]
- err.absolute_path_required   [ErrAbsolutePathReq]

## Auth (8)
- err.auth.required   [ErrAuthRequired]
- err.auth.invalid_credentials   [ErrAuthInvalidCredentials]
- err.auth.account_disabled   [ErrAuthAccountDisabled]
- err.auth.username_taken   [ErrAuthUsernameTaken]
- err.auth.username_required   [ErrAuthUsernameRequired]
- err.auth.password_short   [ErrAuthPasswordShort]
- err.auth.setup_completed   [ErrAuthSetupCompleted]
- msg.auth.logged_out   [MsgAuthLoggedOut]

## Containers (63)
- err.container.not_found   [ErrContainerNotFound]
- err.container.list_failed   [ErrContainerListFailed]
- err.container.create_failed   [ErrContainerCreateFailed]
- err.container.inspect_failed   [ErrContainerInspectFailed]
- err.container.remove_failed   [ErrContainerRemoveFailed]
- err.container.start_failed   [ErrContainerStartFailed]
- err.container.stop_failed   [ErrContainerStopFailed]
- err.container.restart_failed   [ErrContainerRestartFailed]
- err.container.pause_failed   [ErrContainerPauseFailed]
- err.container.unpause_failed   [ErrContainerUnpauseFailed]
- err.container.kill_failed   [ErrContainerKillFailed]
- err.container.update_failed   [ErrContainerUpdateFailed]
- err.container.rename_failed   [ErrContainerRenameFailed]
- err.container.recreate_failed   [ErrContainerRecreateFailed]
- err.container.prune_failed   [ErrContainerPruneFailed]
- err.container.logs_failed   [ErrContainerLogsFailed]
- err.container.stats_failed   [ErrContainerStatsFailed]
- err.container.top_failed   [ErrContainerTopFailed]
- err.container.files_failed   [ErrContainerFilesFailed]
- err.container.shells_failed   [ErrContainerShellsFailed]
- err.container.services_failed   [ErrContainerServicesFailed]
- err.container.diff_failed   [ErrContainerDiffFailed]
- err.container.exec_bad_request   [ErrContainerExecBadRequest]
- err.container.exec_failed   [ErrContainerExecFailed]
- err.container.exec_history_failed   [ErrContainerExecHistoryFailed]
- err.container.health_failed   [ErrContainerHealthFailed]
- err.container.image_required   [ErrContainerImageRequired]
- err.container.name_required   [ErrContainerNameRequired]
- err.container.name_invalid   [ErrContainerNameInvalid]
- err.container.self_stop   [ErrContainerSelfStop]
- err.container.self_remove   [ErrContainerSelfRemove]
- err.container.self_restart   [ErrContainerSelfRestart]
- err.container.self_pause   [ErrContainerSelfPause]
- err.container.self_kill   [ErrContainerSelfKill]
- msg.container.started   [MsgContainerStarted]
- msg.container.stopped   [MsgContainerStopped]
- msg.container.restarted   [MsgContainerRestarted]
- msg.container.paused   [MsgContainerPaused]
- msg.container.unpaused   [MsgContainerUnpaused]
- msg.container.killed   [MsgContainerKilled]
- msg.container.renamed   [MsgContainerRenamed]
- err.container.action_failed   [ErrContainerActionFailed]
- err.container.backup_key_missing   [ErrContainerBackupKeyMissing]
- err.container.backup_restore_key_required   [ErrContainerBackupRestoreKeyRequired]
- err.container.backup_restore_key_invalid   [ErrContainerBackupRestoreKeyInvalid]
- err.container.backup_restore_config_unsupported   [ErrContainerBackupRestoreConfigUnsupported]
- err.container.backup_restore_config_fs_conflict   [ErrContainerBackupRestoreConfigFsConflict]
- err.container.backup_run_not_cancellable   [ErrContainerBackupRunNotCancellable]
- err.container.backup_destination_required   [ErrContainerBackupDestinationRequired]
- err.container.backup_restore_tree_entries_failed   [ErrContainerBackupRestoreTreeEntriesFailed]
- err.container.backup_restore_mount_missing   [ErrContainerBackupRestoreMountMissing]
- err.container.backup_restore_config_missing   [ErrContainerBackupRestoreConfigMissing]
- msg.container.action_completed   [MsgContainerActionCompleted]
- err.container.file_read_failed   [ErrContainerFileReadFailed]
- err.container.file_write_failed   [ErrContainerFileWriteFailed]
- err.container.file_upload_failed   [ErrContainerFileUploadFailed]
- err.container.file_mkdir_failed   [ErrContainerFileMkdirFailed]
- err.container.file_rename_failed   [ErrContainerFileRenameFailed]
- err.container.file_chmod_failed   [ErrContainerFileChmodFailed]
- err.container.file_delete_failed   [ErrContainerFileDeleteFailed]
- err.container.file_too_large   [ErrContainerFileTooLarge]
- err.container.file_name_required   [ErrContainerFileNameRequired]
- err.container.file_mode_invalid   [ErrContainerFileModeInvalid]

## Container Image Updates (2)
- err.container.update_check_failed   [ErrContainerUpdateCheckFailed]
- err.container.image_update_failed   [ErrContainerImageUpdateFailed]

## Images (15)
- err.image.not_found   [ErrImageNotFound]
- err.image.list_failed   [ErrImageListFailed]
- err.image.inspect_failed   [ErrImageInspectFailed]
- err.image.remove_failed   [ErrImageRemoveFailed]
- err.image.pull_failed   [ErrImagePullFailed]
- err.image.tag_failed   [ErrImageTagFailed]
- err.image.history_failed   [ErrImageHistoryFailed]
- err.image.prune_failed   [ErrImagePruneFailed]
- err.image.ref_required   [ErrImageRefRequired]
- err.image.repo_required   [ErrImageRepoRequired]
- err.image.export_failed   [ErrImageExportFailed]
- err.image.import_failed   [ErrImageImportFailed]
- msg.image.removed   [MsgImageRemoved]
- msg.image.tagged   [MsgImageTagged]
- msg.image.imported   [MsgImageImported]

## Volumes (9)
- err.volume.not_found   [ErrVolumeNotFound]
- err.volume.list_failed   [ErrVolumeListFailed]
- err.volume.inspect_failed   [ErrVolumeInspectFailed]
- err.volume.create_failed   [ErrVolumeCreateFailed]
- err.volume.remove_failed   [ErrVolumeRemoveFailed]
- err.volume.prune_failed   [ErrVolumePruneFailed]
- err.volume.name_required   [ErrVolumeNameRequired]
- msg.volume.created   [MsgVolumeCreated]
- msg.volume.removed   [MsgVolumeRemoved]

## Networks (14)
- err.network.not_found   [ErrNetworkNotFound]
- err.network.list_failed   [ErrNetworkListFailed]
- err.network.inspect_failed   [ErrNetworkInspectFailed]
- err.network.create_failed   [ErrNetworkCreateFailed]
- err.network.remove_failed   [ErrNetworkRemoveFailed]
- err.network.connect_failed   [ErrNetworkConnectFailed]
- err.network.disconnect_failed   [ErrNetworkDisconnectFailed]
- err.network.name_required   [ErrNetworkNameRequired]
- err.network.container_required   [ErrNetworkContainerReq]
- err.network.builtin   [ErrNetworkBuiltIn]
- msg.network.created   [MsgNetworkCreated]
- msg.network.removed   [MsgNetworkRemoved]
- msg.network.connected   [MsgNetworkConnected]
- msg.network.disconnected   [MsgNetworkDisconnected]

## Stacks (27)
- err.stack.not_found   [ErrStackNotFound]
- err.stack.list_failed   [ErrStackListFailed]
- err.stack.create_failed   [ErrStackCreateFailed]
- err.stack.update_failed   [ErrStackUpdateFailed]
- err.stack.remove_failed   [ErrStackRemoveFailed]
- err.stack.deploy_failed   [ErrStackDeployFailed]
- err.stack.stop_failed   [ErrStackStopFailed]
- err.stack.down_failed   [ErrStackDownFailed]
- err.stack.restart_failed   [ErrStackRestartFailed]
- err.stack.logs_failed   [ErrStackLogsFailed]
- err.stack.compose_not_found   [ErrStackComposeFailed]
- err.stack.name_required   [ErrStackNameRequired]
- err.stack.compose_required   [ErrStackComposeRequired]
- err.stack.already_managed   [ErrStackAlreadyManaged]
- err.stack.adopt_failed   [ErrStackAdoptFailed]
- err.stack.preview_failed   [ErrStackPreviewFailed]
- err.stack.link_failed   [ErrStackLinkFailed]
- err.stack.webhook_not_found   [ErrStackWebhookNotFound]
- err.stack.webhook_failed   [ErrStackWebhookFailed]
- err.stack.prune_failed   [ErrStackPruneFailed]
- err.stack.env_vars_failed   [ErrStackEnvVarsFailed]
- msg.stack.deployed   [MsgStackDeployed]
- msg.stack.removed   [MsgStackRemoved]
- msg.stack.adopted   [MsgStackAdopted]
- msg.stack.pruned   [MsgStackPruned]
- msg.stack.env_vars_updated   [MsgStackEnvVarsUpdated]
- err.stack.update_check_failed   [ErrStackUpdateCheckFailed]

## Environments (18)
- err.env.not_found   [ErrEnvNotFound]
- err.env.list_failed   [ErrEnvListFailed]
- err.env.create_failed   [ErrEnvCreateFailed]
- err.env.update_failed   [ErrEnvUpdateFailed]
- err.env.remove_failed   [ErrEnvRemoveFailed]
- err.env.name_required   [ErrEnvNameRequired]
- err.env.connection_required   [ErrEnvConnRequired]
- err.env.connection_failed   [ErrEnvConnFailed]
- err.env.invalid_connection_type   [ErrEnvInvalidConnType]
- err.env.invalid_agent_token   [ErrEnvInvalidAgentToken]
- err.env.invalid_timezone   [ErrEnvInvalidTimezone]
- err.env.invalid_log_retention   [ErrEnvInvalidLogRetention]
- err.env.invalid_container_prune_days   [ErrEnvInvalidContainerPrune]
- err.env.invalid_metric_retention   [ErrEnvInvalidMetricRetention]
- err.env.invalid_auto_update_window   [ErrEnvInvalidAutoUpdate]
- err.env.invalid_auto_update_days   [ErrEnvInvalidAutoUpdateDays]
- msg.env.created   [MsgEnvCreated]
- msg.env.removed   [MsgEnvRemoved]

## Terminal (3)
- err.terminal.failed   [ErrTerminalFailed]
- err.terminal.upgrade_failed   [ErrTerminalUpgrade]
- err.terminal.container_required   [ErrTerminalContainer]

## Logs (2)
- err.logs.failed   [ErrLogsFailed]
- err.logs.container_required   [ErrLogsContainerReq]

## Events (1)
- err.events.failed   [ErrEventsFailed]

## Dashboard (1)
- err.dashboard.failed   [ErrDashboardFailed]

## Blueprints (8)
- err.blueprint.not_found   [ErrBlueprintNotFound]
- err.blueprint.list_failed   [ErrBlueprintListFailed]
- err.blueprint.create_failed   [ErrBlueprintCreateFailed]
- err.blueprint.update_failed   [ErrBlueprintUpdateFailed]
- err.blueprint.remove_failed   [ErrBlueprintRemoveFailed]
- err.blueprint.name_required   [ErrBlueprintNameRequired]
- err.blueprint.compose_required   [ErrBlueprintComposeRequired]
- err.blueprint.stack_name_required   [ErrBlueprintStackNameReq]

## Reconciler (10)
- err.reconciler.failed   [ErrReconcilerFailed]
- err.reconciler.not_found   [ErrReconcilerNotFound]
- err.reconciler.list_failed   [ErrReconcilerListFailed]
- err.reconciler.create_failed   [ErrReconcilerCreateFailed]
- err.reconciler.update_failed   [ErrReconcilerUpdateFailed]
- err.reconciler.remove_failed   [ErrReconcilerRemoveFailed]
- err.reconciler.name_required   [ErrReconcilerNameRequired]
- err.reconciler.container_required   [ErrReconcilerContainerReq]
- err.reconciler.image_required   [ErrReconcilerImageRequired]
- err.reconciler.docker_failed   [ErrReconcilerDockerFailed]

## Git (9)
- err.git.failed   [ErrGitFailed]
- err.git.clone_failed   [ErrGitCloneFailed]
- err.git.not_found   [ErrGitNotFound]
- err.git.list_failed   [ErrGitListFailed]
- err.git.create_failed   [ErrGitCreateFailed]
- err.git.update_failed   [ErrGitUpdateFailed]
- err.git.remove_failed   [ErrGitRemoveFailed]
- err.git.name_required   [ErrGitNameRequired]
- err.git.url_required   [ErrGitUrlRequired]

## GitOps (20)
- err.gitops.pipeline_not_found   [ErrGitOpsPipelineNotFound]
- err.gitops.stage_not_found   [ErrGitOpsStageNotFound]
- err.gitops.approval_not_found   [ErrGitOpsApprovalNotFound]
- err.gitops.preview_not_found   [ErrGitOpsPreviewNotFound]
- err.gitops.list_failed   [ErrGitOpsListFailed]
- err.gitops.create_failed   [ErrGitOpsCreateFailed]
- err.gitops.update_failed   [ErrGitOpsUpdateFailed]
- err.gitops.remove_failed   [ErrGitOpsRemoveFailed]
- err.gitops.promote_failed   [ErrGitOpsPromoteFailed]
- err.gitops.approve_failed   [ErrGitOpsApproveFailed]
- err.gitops.reject_failed   [ErrGitOpsRejectFailed]
- err.gitops.preview_failed   [ErrGitOpsPreviewFailed]
- err.gitops.webhook_failed   [ErrGitOpsWebhookFailed]
- err.gitops.name_required   [ErrGitOpsNameRequired]
- err.gitops.repo_missing   [ErrGitOpsRepoMissing]
- err.gitops.stage_order_invalid   [ErrGitOpsStageOrderInvalid]
- err.gitops.target_env_required   [ErrGitOpsTargetEnvRequired]
- err.gitops.approval_expired   [ErrGitOpsApprovalExpired]
- err.gitops.no_commit_detected   [ErrGitOpsNoCommitDetected]
- err.gitops.webhook_signature_bad   [ErrGitOpsWebhookSignatureBad]

## Webhooks (7)
- err.webhook.not_found   [ErrWebhookNotFound]
- err.webhook.list_failed   [ErrWebhookListFailed]
- err.webhook.create_failed   [ErrWebhookCreateFailed]
- err.webhook.update_failed   [ErrWebhookUpdateFailed]
- err.webhook.remove_failed   [ErrWebhookRemoveFailed]
- err.webhook.name_required   [ErrWebhookNameRequired]
- err.webhook.url_required   [ErrWebhookUrlRequired]

## Agent (18)
- err.agent.not_found   [ErrAgentNotFound]
- err.agent.list_failed   [ErrAgentListFailed]
- err.agent.status_failed   [ErrAgentStatusFailed]
- err.agent.token_failed   [ErrAgentTokenFailed]
- err.agent.deploy_failed   [ErrAgentDeployFailed]
- err.agent.deploy_ssh_required   [ErrAgentDeploySSHRequired]
- err.agent.deploy_host_key_required   [ErrAgentDeployHostKeyRequired]
- err.agent.deploy_invalid_method   [ErrAgentDeployInvalidMethod]
- err.agent.deploy_ssh_key_invalid   [ErrAgentDeploySSHKeyInvalid]
- err.agent.deploy_host_key_mismatch   [ErrAgentDeployHostKeyMismatch]
- err.agent.deploy_ssh_connect_failed   [ErrAgentDeploySSHConnectFailed]
- err.agent.deploy_os_detect_failed   [ErrAgentDeployOSDetectFailed]
- err.agent.deploy_command_failed   [ErrAgentDeployCommandFailed]
- err.agent.install_token_failed   [ErrAgentInstallTokenFailed]
- err.agent.update_failed   [ErrAgentUpdateFailed]
- err.agent.logs_failed   [ErrAgentLogsFailed]
- msg.agent.token_regenerated   [MsgAgentTokenRegenerated]
- msg.agent.deployed   [MsgAgentDeployed]

## Settings (21)
- err.settings.failed   [ErrSettingsFailed]
- err.settings.update_failed   [ErrSettingsUpdateFailed]
- err.settings.no_settings   [ErrSettingsNoSettings]
- err.settings.tls_invalid   [ErrSettingsTLSInvalid]
- err.settings.tls_pair_required   [ErrSettingsTLSPairReq]
- err.settings.tls_regenerate   [ErrSettingsTLSRegenerate]
- err.settings.tls_regen_user_cert   [ErrSettingsTLSRegenUserCert]
- err.settings.backup_key_invalid   [ErrSettingsBackupKeyInvalid]
- err.settings.compose_project_missing   [ErrSettingsComposeProjectMissing]
- err.settings.idle_timeout_invalid   [ErrSettingsIdleTimeoutInvalid]
- msg.settings.updated   [MsgSettingsUpdated]
- msg.agent_settings.updated   [MsgAgentSettingsUpdated]
- err.agent_settings.invalid   [ErrAgentSettingsInvalid]
- err.retention_settings.invalid   [ErrRetentionSettingsInvalid]
- msg.retention_settings.updated   [MsgRetentionSettingsUpdated]
- err.toast_settings.invalid   [ErrToastSettingsInvalid]
- msg.toast_settings.updated   [MsgToastSettingsUpdated]
- err.appearance_settings.invalid   [ErrAppearanceSettingsInvalid]
- msg.appearance_settings.updated   [MsgAppearanceSettingsUpdated]
- msg.audit_log.purged   [MsgAuditLogPurged]
- msg.activity_log.purged   [MsgActivityLogPurged]

## Users (11)
- err.user.not_found   [ErrUserNotFound]
- err.profile.identity_provider_locked   [ErrProfileIdentityProviderLocked]
- err.user.list_failed   [ErrUserListFailed]
- err.user.create_failed   [ErrUserCreateFailed]
- err.user.create_no_default_role   [ErrUserCreateNoDefault]
- err.user.update_failed   [ErrUserUpdateFailed]
- err.user.remove_failed   [ErrUserRemoveFailed]
- err.user.self_delete   [ErrUserSelfDelete]
- err.user.password_required   [ErrUserPasswordRequired]
- err.user.password_incorrect   [ErrUserPasswordIncorrect]
- msg.user.password_updated   [MsgUserPasswordUpdated]

## Activity (1)
- err.activity.list_failed   [ErrActivityListFailed]

## Audit (16)
- err.audit.list_failed   [ErrAuditListFailed]
- err.audit.forwarder_list_failed   [ErrAuditForwarderListFailed]
- err.audit.forwarder_save_failed   [ErrAuditForwarderSaveFailed]
- err.audit.forwarder_test_failed   [ErrAuditForwarderTestFailed]
- err.audit.forwarder_history_failed   [ErrAuditForwarderHistoryFailed]
- err.audit.forwarder_unknown   [ErrAuditForwarderUnknown]
- err.audit.forwarder_type_required   [ErrAuditForwarderTypeRequired]
- err.audit.forwarder_delete_failed   [ErrAuditForwarderDeleteFailed]
- err.audit.forwarder_not_configured   [ErrAuditForwarderNotConfigured]
- err.audit.forwarder_field_required   [ErrAuditForwarderFieldRequired]
- err.audit.forwarder_name_required   [ErrAuditForwarderNameRequired]
- err.audit.forwarder_create_failed   [ErrAuditForwarderCreateFailed]
- msg.audit.forwarder_saved   [MsgAuditForwarderSaved]
- msg.audit.forwarder_tested   [MsgAuditForwarderTested]
- msg.audit.forwarder_deleted   [MsgAuditForwarderDeleted]
- msg.audit.forwarder_created   [MsgAuditForwarderCreated]

## Notifications (8)
- err.notification.not_found   [ErrNotificationNotFound]
- err.notification.list_failed   [ErrNotificationListFailed]
- err.notification.create_failed   [ErrNotificationCreateFailed]
- err.notification.update_failed   [ErrNotificationUpdateFailed]
- err.notification.remove_failed   [ErrNotificationRemoveFailed]
- err.notification.name_required   [ErrNotificationNameRequired]
- err.notification.type_required   [ErrNotificationTypeRequired]
- err.notification.invalid_type   [ErrNotificationInvalidType]

## In-App Notifications (5)
- err.in_app_notification.not_found   [ErrInAppNotificationNotFound]
- err.in_app_notification.list_failed   [ErrInAppNotificationListFailed]
- err.in_app_notification.read_failed   [ErrInAppNotificationReadFailed]
- err.in_app_notification.delete_failed   [ErrInAppNotificationDeleteFailed]
- err.in_app_notification.count_failed   [ErrInAppNotificationCountFailed]

## Registry (8)
- err.registry.failed   [ErrRegistryFailed]
- err.registry.not_found   [ErrRegistryNotFound]
- err.registry.list_failed   [ErrRegistryListFailed]
- err.registry.create_failed   [ErrRegistryCreateFailed]
- err.registry.update_failed   [ErrRegistryUpdateFailed]
- err.registry.remove_failed   [ErrRegistryRemoveFailed]
- err.registry.name_required   [ErrRegistryNameRequired]
- err.registry.url_required   [ErrRegistryUrlRequired]

## Scans (27)
- err.scan.failed   [ErrScanFailed]
- err.scan.not_found   [ErrScanNotFound]
- err.scan.list_failed   [ErrScanListFailed]
- err.scan.create_failed   [ErrScanCreateFailed]
- err.scan.image_required   [ErrScanImageRequired]
- err.scan.delete_failed   [ErrScanDeleteFailed]
- err.scan.summary_failed   [ErrScanSummaryFailed]
- err.scan.scanner_not_available   [ErrScannerNotAvailable]
- err.scan.scanner_invalid   [ErrScannerInvalid]
- err.scan.already_running   [ErrScanAlreadyRunning]
- msg.scan.started   [MsgScanStarted]
- msg.scan.deleted   [MsgScanDeleted]
- err.scanner.settings_invalid   [ErrScannerSettingsInvalid]
- msg.scanner.settings_updated   [MsgScannerSettingsUpdated]
- err.scanner.adapter_unknown   [ErrScannerAdapterUnknown]
- err.scanner.not_configured   [ErrScannerNotConfigured]
- err.scanner.none_configured   [ErrNoScannerConfigured]
- err.scanner.name_required   [ErrScannerNameRequired]
- err.scanner.field_required   [ErrScannerFieldRequired]
- err.scanner.list_failed   [ErrScannerListFailed]
- err.scanner.create_failed   [ErrScannerCreateFailed]
- err.scanner.save_failed   [ErrScannerSaveFailed]
- err.scanner.delete_failed   [ErrScannerDeleteFailed]
- err.scanner.test_failed   [ErrScannerTestFailed]
- msg.scanner.saved   [MsgScannerSaved]
- msg.scanner.deleted   [MsgScannerDeleted]
- msg.scanner.tested   [MsgScannerTested]

## Updates (6)
- err.update.check_failed   [ErrUpdateCheckFailed]
- err.update.not_found   [ErrUpdateNotFound]
- err.update.list_failed   [ErrUpdateListFailed]
- err.update.create_failed   [ErrUpdateCreateFailed]
- err.update.remove_failed   [ErrUpdateRemoveFailed]
- err.update.name_required   [ErrUpdateNameRequired]

## Plugins (6)
- err.plugin.list_failed   [ErrPluginListFailed]
- err.plugin.not_found   [ErrPluginNotFound]
- err.plugin.install_failed   [ErrPluginInstallFailed]
- err.plugin.remove_failed   [ErrPluginRemoveFailed]
- err.plugin.name_required   [ErrPluginNameRequired]
- err.plugin.source_required   [ErrPluginSourceRequired]

## Schedules (9)
- err.schedule.list_failed   [ErrScheduleListFailed]
- err.schedule.not_found   [ErrScheduleNotFound]
- err.schedule.create_failed   [ErrScheduleCreateFailed]
- err.schedule.update_failed   [ErrScheduleUpdateFailed]
- err.schedule.remove_failed   [ErrScheduleRemoveFailed]
- err.schedule.name_required   [ErrScheduleNameRequired]
- err.schedule.cron_required   [ErrScheduleCronRequired]
- err.schedule.action_required   [ErrScheduleActionRequired]
- err.schedule.target_required   [ErrScheduleTargetRequired]

## Alerts (13)
- err.alert.list_failed   [ErrAlertListFailed]
- err.alert.not_found   [ErrAlertNotFound]
- err.alert.create_failed   [ErrAlertCreateFailed]
- err.alert.update_failed   [ErrAlertUpdateFailed]
- err.alert.remove_failed   [ErrAlertRemoveFailed]
- err.alert.name_required   [ErrAlertNameRequired]
- err.alert.type_required   [ErrAlertTypeRequired]
- err.alert.destination_required   [ErrAlertDestinationRequired]
- err.alert.condition_unknown   [ErrAlertConditionUnknown]
- err.alert.condition_invalid   [ErrAlertConditionInvalid]
- err.alert.test_failed   [ErrAlertTestFailed]
- err.alert.event_list_failed   [ErrAlertEventListFailed]
- msg.alert.test_sent   [MsgAlertTestSent]

## Metrics (9)
- err.metrics.failed   [ErrMetricsFailed]
- err.metrics.host_info_failed   [ErrMetricsHostInfoFailed]
- err.metrics.stats_failed   [ErrMetricsStatsFailed]
- err.metrics.container_required   [ErrMetricsContainerRequired]
- err.metrics.container_not_found   [ErrMetricsContainerNotFound]
- err.metrics.container_not_running   [ErrMetricsContainerNotRunning]
- err.metrics.stream_not_supported   [ErrMetricsStreamNotSupported]
- err.metrics.history_failed   [ErrMetricsHistoryFailed]
- err.metrics.range_invalid   [ErrMetricsRangeInvalid]

## Workflows (12)
- err.workflow.not_found   [ErrWorkflowNotFound]
- err.workflow.list_failed   [ErrWorkflowListFailed]
- err.workflow.create_failed   [ErrWorkflowCreateFailed]
- err.workflow.update_failed   [ErrWorkflowUpdateFailed]
- err.workflow.remove_failed   [ErrWorkflowRemoveFailed]
- err.workflow.run_failed   [ErrWorkflowRunFailed]
- err.workflow.name_required   [ErrWorkflowNameRequired]
- err.workflow.link_message_fetch_failed   [ErrLinkMessageFetchFailed]
- err.workflow.link_output_list_failed   [ErrLinkOutputListFailed]
- err.workflow_node.list_failed   [ErrWorkflowNodeListFailed]
- err.workflow_node.update_failed   [ErrWorkflowNodeUpdateFailed]
- err.workflow_node.key_required   [ErrWorkflowNodeKeyRequired]

## App Store (5)
- err.appstore.list_failed   [ErrAppStoreListFailed]
- err.appstore.install_failed   [ErrAppStoreInstallFailed]
- err.appstore.uninstall_failed   [ErrAppStoreUninstallFailed]
- err.appstore.not_found   [ErrAppStoreNotFound]
- err.appstore.slug_required   [ErrAppStoreSlugRequired]

## RBAC (1)
- err.rbac.permission_denied   [ErrRBACPermissionDenied]

## System (12)
- err.system.restart_not_scheduled   [ErrSystemRestartNotScheduled]
- err.system.restart_inspect_failed   [ErrSystemRestartInspectFailed]
- err.system.restart_not_local   [ErrSystemRestartNotLocal]
- err.system.self_update_failed   [ErrSystemSelfUpdateFailed]
- err.system.self_update_not_local   [ErrSystemSelfUpdateNotLocal]
- err.system.reset_failed   [ErrSystemResetFailed]
- err.system.reset_not_local   [ErrSystemResetNotLocal]
- err.system.reset_confirm_mismatch   [ErrSystemResetConfirmMismatch]
- msg.system.restart_scheduled   [MsgSystemRestartScheduled]
- msg.system.self_update_scheduled   [MsgSystemSelfUpdateScheduled]
- msg.agent.update_triggered   [MsgAgentUpdateTriggered]
- msg.system.reset_scheduled   [MsgSystemResetScheduled]

## Roles (11)
- err.role.not_found   [ErrRoleNotFound]
- err.role.list_failed   [ErrRoleListFailed]
- err.role.create_failed   [ErrRoleCreateFailed]
- err.role.update_failed   [ErrRoleUpdateFailed]
- err.role.remove_failed   [ErrRoleRemoveFailed]
- err.role.name_required   [ErrRoleNameRequired]
- err.role.name_taken   [ErrRoleNameTaken]
- err.role.system_locked   [ErrRoleSystemLocked]
- msg.role.created   [MsgRoleCreated]
- msg.role.updated   [MsgRoleUpdated]
- msg.role.removed   [MsgRoleRemoved]

## Groups (16)
- err.group.not_found   [ErrGroupNotFound]
- err.group.list_failed   [ErrGroupListFailed]
- err.group.create_failed   [ErrGroupCreateFailed]
- err.group.update_failed   [ErrGroupUpdateFailed]
- err.group.remove_failed   [ErrGroupRemoveFailed]
- err.group.name_required   [ErrGroupNameRequired]
- err.group.name_taken   [ErrGroupNameTaken]
- err.group.member_exists   [ErrGroupMemberExists]
- err.group.system_locked   [ErrGroupSystemLocked]
- msg.group.created   [MsgGroupCreated]
- msg.group.updated   [MsgGroupUpdated]
- msg.group.removed   [MsgGroupRemoved]
- msg.group.member_added   [MsgGroupMemberAdded]
- msg.group.member_removed   [MsgGroupMemberRemoved]
- msg.group.role_assigned   [MsgGroupRoleAssigned]
- msg.group.role_unassigned   [MsgGroupRoleUnassigned]

## API Keys (22)
- err.api_key.not_found   [ErrAPIKeyNotFound]
- err.api_key.list_failed   [ErrAPIKeyListFailed]
- err.api_key.create_failed   [ErrAPIKeyCreateFailed]
- err.api_key.revoke_failed   [ErrAPIKeyRevokeFailed]
- err.api_key.name_required   [ErrAPIKeyNameRequired]
- err.api_key.invalid   [ErrAPIKeyInvalid]
- err.api_key.revoked   [ErrAPIKeyRevoked]
- err.api_key.expired   [ErrAPIKeyExpired]
- msg.api_key.created   [MsgAPIKeyCreated]
- msg.api_key.revoked   [MsgAPIKeyRevoked]
- err.api_key.grants_required   [ErrAPIKeyGrantsRequired]
- err.api_key.grant_invalid   [ErrAPIKeyGrantInvalid]
- err.api_key.grant_exceeds_caller   [ErrAPIKeyGrantExceedsCaller]
- err.api_key.update_failed   [ErrAPIKeyUpdateFailed]
- err.api_key.delete_failed   [ErrAPIKeyDeleteFailed]
- err.api_key.forbidden   [ErrAPIKeyForbidden]
- msg.api_key.updated   [MsgAPIKeyUpdated]
- msg.api_key.deleted   [MsgAPIKeyDeleted]
- err.api_key.usage_failed   [ErrAPIKeyUsageFailed]
- err.api_key.range_invalid   [ErrAPIKeyRangeInvalid]
- err.api_key.usage_clear_failed   [ErrAPIKeyUsageClearFailed]
- msg.api_key.usage_cleared   [MsgAPIKeyUsageCleared]

## User Roles (6)
- err.user_role.assign_failed   [ErrUserRoleAssignFailed]
- err.user_role.unassign_failed   [ErrUserRoleUnassignFailed]
- err.user_role.list_failed   [ErrUserRoleListFailed]
- err.user_role.exists   [ErrUserRoleExists]
- msg.user_role.assigned   [MsgUserRoleAssigned]
- msg.user_role.unassigned   [MsgUserRoleUnassigned]

## User Groups (1)
- err.user_group.list_failed   [ErrUserGroupListFailed]

## Docker Info (1)
- err.docker_info.failed   [ErrDockerInfoFailed]

## Widgets (10)
- err.widget.list_failed   [ErrWidgetListFailed]
- err.widget.not_found   [ErrWidgetNotFound]
- err.widget.install_failed   [ErrWidgetInstallFailed]
- err.widget.uninstall_failed   [ErrWidgetUninstallFailed]
- err.widget.update_failed   [ErrWidgetUpdateFailed]
- err.widget.key_required   [ErrWidgetKeyRequired]
- err.widget.key_exists   [ErrWidgetKeyExists]
- err.widget.builtin_cannot_delete   [ErrWidgetBuiltinDelete]
- msg.widget.installed   [MsgWidgetInstalled]
- msg.widget.uninstalled   [MsgWidgetUninstalled]

## Custom Nodes (11)
- err.custom_node.list_failed   [ErrCustomNodeListFailed]
- err.custom_node.not_found   [ErrCustomNodeNotFound]
- err.custom_node.key_required   [ErrCustomNodeKeyRequired]
- err.custom_node.key_exists   [ErrCustomNodeKeyExists]
- err.custom_node.create_failed   [ErrCustomNodeCreateFailed]
- err.custom_node.update_failed   [ErrCustomNodeUpdateFailed]
- err.custom_node.delete_failed   [ErrCustomNodeDeleteFailed]
- err.custom_node.code_required   [ErrCustomNodeCodeRequired]
- msg.custom_node.created   [MsgCustomNodeCreated]
- msg.custom_node.updated   [MsgCustomNodeUpdated]
- msg.custom_node.deleted   [MsgCustomNodeDeleted]

## Identity Providers (25)
- err.identity.not_found   [ErrIdentityNotFound]
- err.identity.list_failed   [ErrIdentityListFailed]
- err.identity.create_failed   [ErrIdentityCreateFailed]
- err.identity.update_failed   [ErrIdentityUpdateFailed]
- err.identity.remove_failed   [ErrIdentityRemoveFailed]
- err.identity.name_required   [ErrIdentityNameRequired]
- err.identity.client_id_required   [ErrIdentityClientRequired]
- err.identity.client_secret_required   [ErrIdentitySecretRequired]
- err.identity.tenant_id_required   [ErrIdentityTenantRequired]
- err.identity.issuer_url_required   [ErrIdentityIssuerRequired]
- err.identity.metadata_url_required   [ErrIdentityMetadataRequired]
- err.identity.invalid_type   [ErrIdentityInvalidType]
- err.identity.disabled   [ErrIdentityDisabled]
- err.oidc.state_failed   [ErrOIDCStateFailed]
- err.oidc.state_invalid   [ErrOIDCStateInvalid]
- err.oidc.state_expired   [ErrOIDCStateExpired]
- err.oidc.exchange_failed   [ErrOIDCExchangeFailed]
- err.oidc.userinfo_failed   [ErrOIDCUserInfoFailed]
- err.oidc.provision_failed   [ErrOIDCProvisionFailed]
- err.identity.fetch_groups_failed   [ErrIdentityFetchGroupsFailed]
- err.identity.test_failed   [ErrIdentityTestFailed]
- msg.identity.created   [MsgIdentityCreated]
- msg.identity.updated   [MsgIdentityUpdated]
- msg.identity.removed   [MsgIdentityRemoved]
- msg.identity.test_success   [MsgIdentityTestSuccess]

## Email Servers (0)

## Communication Channels (23)
- err.comm_channel.not_found   [ErrCommChannelNotFound]
- err.comm_channel.list_failed   [ErrCommChannelListFailed]
- err.comm_channel.create_failed   [ErrCommChannelCreateFailed]
- err.comm_channel.update_failed   [ErrCommChannelUpdateFailed]
- err.comm_channel.remove_failed   [ErrCommChannelRemoveFailed]
- err.comm_channel.name_required   [ErrCommChannelNameRequired]
- err.comm_channel.type_invalid   [ErrCommChannelTypeInvalid]
- err.comm_channel.webhook_required   [ErrCommChannelWebhookRequired]
- err.comm_channel.server_required   [ErrCommChannelServerRequired]
- err.comm_channel.token_required   [ErrCommChannelTokenRequired]
- err.comm_channel.topic_required   [ErrCommChannelTopicRequired]
- err.comm_channel.chat_id_required   [ErrCommChannelChatIDRequired]
- err.comm_channel.phone_required   [ErrCommChannelPhoneRequired]
- err.comm_channel.sender_required   [ErrCommChannelSenderRequired]
- err.comm_channel.recipients_required   [ErrCommChannelRecipientsRequired]
- err.comm_channel.priority_invalid   [ErrCommChannelPriorityInvalid]
- err.comm_channel.telegram_admin_required   [ErrCommChannelTelegramAdminRequired]
- err.comm_channel.test_failed   [ErrCommChannelTestFailed]
- msg.comm_channel.created   [MsgCommChannelCreated]
- msg.comm_channel.updated   [MsgCommChannelUpdated]
- msg.comm_channel.removed   [MsgCommChannelRemoved]
- msg.comm_channel.default_set   [MsgCommChannelDefaultSet]
- msg.comm_channel.test_sent   [MsgCommChannelTestSent]

## Password reset / policy (13)
- err.password_reset.invalid   [ErrPasswordResetInvalid]
- err.password_reset.expired   [ErrPasswordResetExpired]
- err.password_reset.consumed   [ErrPasswordResetConsumed]
- err.password_reset.failed   [ErrPasswordResetFailed]
- err.password.too_short   [ErrPasswordTooShort]
- err.password.too_weak   [ErrPasswordTooWeak]
- err.password.reused   [ErrPasswordReused]
- err.password.no_upper   [ErrPasswordNoUpper]
- err.password.no_lower   [ErrPasswordNoLower]
- err.password.no_digit   [ErrPasswordNoDigit]
- err.password.no_symbol   [ErrPasswordNoSymbol]
- msg.password_reset.sent   [MsgPasswordResetSent]
- msg.password_reset.consumed   [MsgPasswordResetConsumed]

## SCIM (9)
- err.scim.filter_invalid   [ErrSCIMFilterInvalid]
- err.scim.token_invalid   [ErrSCIMTokenInvalid]
- err.scim.not_found   [ErrSCIMNotFound]
- err.scim.create_failed   [ErrSCIMCreateFailed]
- err.scim.update_failed   [ErrSCIMUpdateFailed]
- err.scim.delete_failed   [ErrSCIMDeleteFailed]
- msg.scim.endpoint_created   [MsgSCIMEndpointCreated]
- msg.scim.endpoint_updated   [MsgSCIMEndpointUpdated]
- msg.scim.endpoint_removed   [MsgSCIMEndpointRemoved]

## Plugin install / update (11)
- err.plugin.slug_required   [ErrPluginSlugRequired]
- err.plugin.url_or_tarball_required   [ErrPluginURLOrTarballRequired]
- err.plugin.update_no_catalog_entry   [ErrPluginUpdateNoCatalogEntry]
- err.plugin.update_failed   [ErrPluginUpdateFailed]
- err.plugin.invalid_tarball   [ErrPluginInvalidTarball]
- err.plugin.signature_invalid   [ErrPluginSignatureInvalid]
- err.plugin.not_installed   [ErrPluginNotInstalled]
- err.plugin.already_installed   [ErrPluginAlreadyInstalled]
- msg.plugin.installed   [MsgPluginInstalled]
- msg.plugin.updated   [MsgPluginUpdated]
- msg.plugin.uninstalled   [MsgPluginUninstalled]

## Plugin install flow (additional) (5)
- err.plugin.values_required   [ErrPluginValuesRequired]
- err.plugin.manifest_invalid   [ErrPluginManifestInvalid]
- err.plugin.permission_in_use   [ErrPluginPermissionInUse]
- err.plugin.uninstall_blocked   [ErrPluginUninstallBlocked]
- err.platform.unsupported   [ErrPlatformUnsupported]

## SCIM (additional) (3)
- err.scim.endpoint_not_found   [ErrSCIMEndpointNotFound]
- err.scim.endpoint_name_taken   [ErrSCIMEndpointNameTaken]
- msg.scim.synced   [MsgSCIMSynced]

## User account lifecycle (14)
- err.user.already_disabled   [ErrUserAlreadyDisabled]
- err.user.not_disabled   [ErrUserNotDisabled]
- err.user.last_admin   [ErrUserLastAdmin]
- err.user.identity_provider_locked   [ErrUserIdentityProviderLocked]
- err.user.session_list_failed   [ErrUserSessionListFailed]
- err.user.session_revoke_failed   [ErrUserSessionRevokeFailed]
- err.user.password_set_failed   [ErrUserPasswordSetFailed]
- msg.user.disabled   [MsgUserDisabled]
- msg.user.enabled   [MsgUserEnabled]
- msg.user.unlocked   [MsgUserUnlocked]
- msg.user.force_reset   [MsgUserForceReset]
- msg.user.password_admin_set   [MsgUserPasswordAdminSet]
- msg.user.sessions_revoked   [MsgUserSessionsRevoked]
- msg.user.session_revoked   [MsgUserSessionRevoked]

## RBAC policies (24)
- err.rbac.policy_not_found   [ErrRBACPolicyNotFound]
- err.rbac.policy_invalid   [ErrRBACPolicyInvalid]
- err.rbac.policy_create_failed   [ErrRBACPolicyCreateFailed]
- err.rbac.policy_update_failed   [ErrRBACPolicyUpdateFailed]
- err.rbac.policy_delete_failed   [ErrRBACPolicyDeleteFailed]
- err.rbac.policy_expression_invalid   [ErrRBACPolicyExpressionInvalid]
- err.rbac.policy_expression_timeout   [ErrRBACPolicyExpressionTimeout]
- err.rbac.test_failed   [ErrRBACTestFailed]
- err.rbac.policy_timeout   [ErrRBACPolicyTimeout]
- msg.rbac.policy_created   [MsgRBACPolicyCreated]
- msg.rbac.policy_updated   [MsgRBACPolicyUpdated]
- msg.rbac.policy_removed   [MsgRBACPolicyRemoved]
- msg.rbac.policy_enabled   [MsgRBACPolicyEnabled]
- msg.rbac.policy_disabled   [MsgRBACPolicyDisabled]
- err.ai_provider.list_failed   [ErrAIProviderListFailed]
- err.ai_provider.not_found   [ErrAIProviderNotFound]
- err.ai_provider.name_required   [ErrAIProviderNameRequired]
- err.ai_provider.type_required   [ErrAIProviderTypeRequired]
- err.ai_provider.create_failed   [ErrAIProviderCreateFailed]
- err.ai_provider.update_failed   [ErrAIProviderUpdateFailed]
- err.ai_provider.remove_failed   [ErrAIProviderRemoveFailed]
- err.ai_provider.test_failed   [ErrAIProviderTestFailed]
- msg.ai_provider.default_set   [MsgAIProviderDefaultSet]
- err.ai.disabled   [ErrAIDisabled]

## MFA (8)
- err.mfa.status_failed   [ErrMFAStatusFailed]
- err.mfa.enroll_failed   [ErrMFAEnrollFailed]
- err.mfa.already_enabled   [ErrMFAAlreadyEnabled]
- err.mfa.not_enrolled   [ErrMFANotEnrolled]
- err.mfa.invalid_code   [ErrMFAInvalidCode]
- err.mfa.disable_failed   [ErrMFADisableFailed]
- err.mfa.challenge_invalid   [ErrMFAChallengeInvalid]
- err.mfa.required   [ErrMFARequired]

## License (28)
- err.license.read_failed   [ErrLicenseReadFailed]
- err.license.not_activated   [ErrLicenseNotActivated]
- err.license.key_required   [ErrLicenseKeyRequired]
- err.license.key_unknown   [ErrLicenseKeyUnknown]
- err.license.seat_limit   [ErrLicenseSeatLimit]
- err.license.pending_approval   [ErrLicensePendingApproval]
- err.license.not_started   [ErrLicenseNotStarted]
- err.license.expired   [ErrLicenseExpired]
- err.license.suspended   [ErrLicenseSuspended]
- err.license.revoked   [ErrLicenseRevoked]
- err.license.server_not_configured   [ErrLicenseServerNotConfigured]
- err.license.server_unavailable   [ErrLicenseServerUnavailable]
- err.license.server_rejected   [ErrLicenseServerRejected]
- err.license.request_rejected   [ErrLicenseRequestRejected]
- err.license.request_failed   [ErrLicenseRequestFailed]
- err.license.deactivate_failed   [ErrLicenseDeactivateFailed]
- err.license.notice_invalid   [ErrLicenseNoticeInvalid]
- msg.license.deactivated   [MsgLicenseDeactivated]
- msg.license.refreshed   [MsgLicenseRefreshed]
- msg.license.notice_recorded   [MsgLicenseNoticeRecorded]
- err.compute.agent_required   [ErrComputeAgentRequired]
- err.compute.agent_offline   [ErrComputeAgentOffline]
- err.compute.agent_too_old   [ErrComputeAgentTooOld]
- err.compute.unsupported_os   [ErrComputeUnsupportedOS]
- err.compute.console_ticket_invalid   [ErrConsoleTicketInvalid]
- err.compute.console_too_many_viewers   [ErrConsoleTooManyViewers]
- err.compute.console_unavailable   [ErrConsoleUnavailable]
- err.compute.console_kind_invalid   [ErrConsoleKindInvalid]


## Related

- [Conventions](./README.md): the envelope these codes travel in
- [Permissions](./permissions.md): what produces a 403
