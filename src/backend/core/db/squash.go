// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package db

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"strings"
)

// The migration squash.
//
// Everything written after the 2.0.8 release has been merged twice: on
// 2026-09-01 (77 files into fourteen) and on 2026-09-25 (those fourteen plus
// the 66 written since into seventeen, 065 through 081), so that each table is
// created in its final shape in one place. The merged set produces the same
// schema and the same seeded rows as the files it replaced; that was verified
// by building a database from each set — fresh, and upgraded from a populated
// 2.0.8 database — and diffing tables, columns, keys, checks, indexes and data.
//
// The problem this file solves is that `schema_migrations` keys on *filename*.
// A database that already applied the retired files has no record of the
// merged names, so without intervention every one of them would run again on
// the next boot and die on `duplicate column name`.
//
// Three populations, three outcomes:
//
//   - A 2.0.8 install, or a fresh one. None of the retired names are present,
//     so nothing happens here and the merged files run normally.
//   - An install at the pre-squash head. The sentinel is present; the merged
//     names are recorded as applied and none of them run. The schema is
//     already what they would produce.
//   - An install stopped part-way through the retired range, including one
//     that stopped at the head of the first squash. There is no per-file
//     answer — each merged file covers several originals — so startup fails
//     with an instruction rather than a guess.
//
// The retired names stay in the ledger; the schema health check skips them
// through RetiredMigrationNames. Once no installation predates the squash,
// this file can be deleted.

// squashSentinel is the last migration of the retired range. Its presence
// means the database reached the pre-squash head.
const squashSentinel = "144_table_views.sql"

// squashedMigrations are the retired filenames: the 2026-09-01 originals
// first, then everything that squash produced or that was written after it.
var squashedMigrations = []string{
	"052_storage_location_config.sql",
	"066_mfa_ldap_exec_history_ai.sql", "067_identity_provider_ldap.sql",
	"070_rbac_v2.sql", "071_owner_backfill.sql", "072_login_lockout_policy.sql",
	"073_password_history.sql", "074_rbac_v2_rollout.sql", "075_rbac_policies.sql",
	"076_rbac_policy_seeds.sql", "077_scim_endpoints.sql", "078_scim_sync.sql",
	"079_scim_force_sync.sql", "083_settings_cleanup_null_ids.sql",
	"084_plugin_ui_hooks.sql", "085_security_settings.sql",
	"086_appstore_plugins_installed_by.sql", "087_appstore_plugins_installed_by_fk.sql",
	"088_environments_single_default.sql", "089_ai_providers.sql",
	"090_drop_ai_providers.sql", "091_ai_provider_connections.sql",
	"092_communication_channels_config.sql", "093_drop_notification_channels.sql",
	"094_email_servers_config.sql", "095_catalogs.sql",
	"096_container_backup_snapshot_configs.sql", "097_password_history_size_setting.sql",
	"098_plugin_nodes_metadata.sql", "099_catalogs_plugin_default_github.sql",
	"102_role_presets.sql", "103_elevations.sql", "104_plugin_contribution_tables.sql",
	"105_rbac_policies_origin.sql", "106_external_app_catalog.sql",
	"107_builtin_catalog_urls.sql", "108_ai_assistant_usage.sql",
	"109_settings_self_update_id.sql", "110_audit_log_severity.sql",
	"111_ai_assistant_action_log.sql", "112_audit_export_siem.sql",
	"113_plugin_themes.sql", "114_audit_log_state_diff.sql", "115_audit_log_worm.sql",
	"116_audit_log_hash_chain.sql", "117_audit_retention_presets.sql",
	"118_audit_syslog_settings.sql", "119_audit_syslog_tls.sql",
	"120_audit_clock_drift.sql", "121_audit_action_catalog.sql",
	"122_audit_runtime_flags.sql", "123_audit_clock_reference_url.sql",
	"124_audit_coverage_map.sql", "125_identity_provider_config.sql",
	"126_plugin_config.sql", "127_container_health_history.sql",
	"128_container_uptime_daily.sql", "129_metrics_history.sql",
	"130_lifecycle_timestamp_format.sql", "131_lifecycle_timestamp_sweep.sql",
	"132_username_case_insensitive.sql", "133_username_index_binary.sql",
	"134_container_backup_runs_container_name.sql", "135_rbac_route_gap_grants.sql",
	"136_plugin_notification_adapters.sql", "137_drop_email_servers.sql",
	"138_swarm_role_grants.sql", "139_stacks_orchestrator.sql",
	"140_container_backup_run_items.sql", "141_backups_restore_permissions.sql",
	"142_container_backup_selections.sql", "143_audit_log_forwarders.sql",
	"144_audit_forwarder_actions.sql", "145_audit_log_forwarder_instances.sql",
	"146_audit_forwarder_actions_fix.sql", "147_kubernetes_to_plugin.sql",
	"148_drop_orchestrator_type.sql",

	"065_storage_location_config.sql", "066_auth_users_sessions.sql",
	"067_identity_providers.sql", "068_rbac.sql", "069_scim.sql", "070_plugins.sql",
	"071_ai_assistant.sql", "072_communications.sql", "073_audit.sql",
	"074_container_backups.sql", "075_observability.sql", "076_stacks_environments.sql",
	"077_kubernetes_to_plugin.sql", "078_settings.sql",
	"079_alerts_expansion.sql", "080_scanner_plugins.sql",
	"081_notification_error_level_and_catalog_defaults.sql", "082_host_gpu_metrics.sql",
	"083_container_identity_key.sql", "084_api_key_rbac.sql",
	"085_api_key_usage_history.sql", "086_repair_notification_reads_fk.sql",
	"087_password_reset_email.sql", "088_plugin_action_audit_catalog.sql",
	"089_notification_recipient_override.sql", "090_drop_widget_node_catalogs.sql",
	"091_session_idle_timeout.sql", "092_user_toast_preferences.sql",
	"093_user_appearance_preferences.sql", "094_swarm_to_plugin.sql",
	"095_plugin_widget_settings.sql", "096_plugin_preset_grants.sql",
	"097_agent_logs.sql", "098_permission_catalog_hygiene.sql", "099_fk_indexes.sql",
	"100_license.sql", "101_license_contact.sql", "102_license_key.sql",
	"103_appstore_registry_meta.sql", "104_appstore_installed_durable.sql",
	"105_appstore_catalog_latest.sql", "106_appstore_plugin_dependencies.sql",
	"107_ai_oauth_tokens.sql", "108_drop_legacy_notifications.sql",
	"109_ai_provider_oauth.sql", "110_appstore_plugin_signed.sql",
	"111_agent_health_history.sql", "112_stack_env_encryption.sql", "113_secrets.sql",
	"114_secrets_role_presets.sql", "115_module_secret_ids.sql", "116_sudo_windows.sql",
	"117_stack_secret_file_mode.sql", "118_fk_indexes_secrets.sql",
	"119_drop_ai_assistant_legacy.sql", "120_gitops_last_seen_commit.sql",
	"121_secret_apply.sql", "122_secrets_apply_preset.sql",
	"123_environment_connection_migration.sql", "124_environment_migration_failsafe.sql",
	"125_agent_install_kind.sql", "126_plugin_menu_priority.sql",
	"127_plugin_catalogs.sql", "128_drop_seeded_vm_image_mount.sql",
	"129_secrets_group.sql", "130_secrets_custom_kind.sql",
	"131_secrets_application_scope.sql", "132_agent_deploy_ssh.sql",
	"133_host_update_schedule.sql", "134_plugin_domain_audit_catalog.sql",
	"135_legacy_addon_audit_actions.sql", "136_audit_environment_name.sql",
	"137_plugin_built_for.sql", "138_clock_reference_url_cloudflare.sql",
	"139_plugin_build.sql", "140_plugin_migrations.sql",
	"141_lifecycle_events_guest_subjects.sql", "142_host_update_new_packages.sql",
	"143_backup_log_provider_rows.sql", squashSentinel,
}

// squashReplacements are the seventeen files that replaced them.
var squashReplacements = []string{
	"065_settings.sql",
	"066_accounts.sql",
	"067_rbac.sql",
	"068_backups_storage.sql",
	"069_secrets.sql",
	"070_stacks_environments_gitops.sql",
	"071_kubernetes_swarm_to_plugins.sql",
	"072_audit.sql",
	"073_identity.sql",
	"074_plugins_appstore.sql",
	"075_ai.sql",
	"076_communications_notifications.sql",
	"077_observability_history.sql",
	"078_alerts.sql",
	"079_scanners.sql",
	"080_api_keys.sql",
	"081_license.sql",
}

// RetiredMigrationNames returns the filenames the squash replaced, so the
// schema health check does not report them as applied-but-unknown.
func RetiredMigrationNames() []string {
	return append([]string(nil), squashedMigrations...)
}

// reconcileSquashedMigrations records the merged filenames as applied on a
// database that already ran the originals, so they are skipped rather than
// re-run. It is a no-op on 2.0.8 and fresh installs.
func reconcileSquashedMigrations(ctx context.Context, db *sql.DB, driver Driver) error {
	applied, err := appliedSquashedCount(ctx, db, driver)
	if err != nil {
		return err
	}
	if applied == 0 {
		return nil // 2.0.8 or fresh: the merged files are genuinely new.
	}

	atHead, err := migrationApplied(ctx, db, driver, squashSentinel)
	if err != nil {
		return err
	}
	if !atHead {
		return fmt.Errorf(
			"this database is part-way through the migrations retired by the "+
				"migration squash (%d of %d applied, but %s is not). The merged "+
				"files each cover several of the originals, so there is no safe "+
				"way to apply only the missing part. Recreate the database, or "+
				"restore a backup taken at or after %s",
			applied, len(squashedMigrations), squashSentinel, squashSentinel)
	}

	// At the pre-squash head: the schema already matches what the merged files
	// produce, so claim their names without running them.
	var claimed []string
	for _, name := range squashReplacements {
		already, err := migrationApplied(ctx, db, driver, name)
		if err != nil {
			return err
		}
		if already {
			continue
		}
		if _, err := db.ExecContext(ctx, migrationRecordSQL(driver), name); err != nil {
			return fmt.Errorf("recording squashed migration %s: %w", name, err)
		}
		claimed = append(claimed, name)
	}
	if len(claimed) > 0 {
		slog.Info("migration squash reconciled",
			"retired", len(squashedMigrations),
			"claimed", len(claimed),
			"names", strings.Join(claimed, ","))
	}
	return nil
}

func appliedSquashedCount(ctx context.Context, db *sql.DB, driver Driver) (int, error) {
	count := 0
	for _, name := range squashedMigrations {
		ok, err := migrationApplied(ctx, db, driver, name)
		if err != nil {
			return 0, err
		}
		if ok {
			count++
		}
	}
	return count, nil
}

func migrationApplied(ctx context.Context, db *sql.DB, driver Driver, name string) (bool, error) {
	var n int
	if err := db.QueryRowContext(ctx, migrationLookupSQL(driver), name).Scan(&n); err != nil {
		return false, fmt.Errorf("checking migration %s: %w", name, err)
	}
	return n > 0, nil
}
