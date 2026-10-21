#!/usr/bin/env bash
# Copyright (c) 2026 McSparrow. All rights reserved.
# McHarbor is licensed under the McHarbor License. See LICENSE for details.
#
# Development helper — the inverse of
# add-dev-storage-and-catalogs.sh. Removes the local bind-mount rows so
# a database promoted toward production carries no dev-only sources:
#
#   /mnt/backup          storage_locations   (location_type = local)
#   /mnt/app-catalog     settings_catalogs   (kind = docker_app)
#   /mnt/plugin-catalog  settings_catalogs   (kind = plugin)
#
# The local storage location cannot be deleted through the REST API
# (ErrLocalStorageProtected), so this works on the container's SQLite
# database directly. PRAGMA foreign_keys is turned ON explicitly — the
# sqlite3 CLI defaults it OFF, and without it the ON DELETE CASCADE /
# SET NULL rules on container_backup_plan_storage_locations,
# container_backup_plans and container_backup_run_destinations do not
# fire, leaving orphan rows behind.
#
# Idempotent — re-running is a no-op.
#
# Usage:
#   bash scripts/development/remove-dev-storage-and-catalogs.sh [--dry-run] [--yes]
#
# Environment overrides:
#   MCHARBOR_SERVICE                  compose service name (default: mcharbor)
#   MCHARBOR_DB                       db path in container (default: /app/data/mcharbor.db)
#   MCHARBOR_DEV_BACKUP_PATH          default: /mnt/backup
#   MCHARBOR_DEV_APP_CATALOG_PATH     default: /mnt/app-catalog
#   MCHARBOR_DEV_PLUGIN_CATALOG_PATH  default: /mnt/plugin-catalog

set -euo pipefail

# shellcheck source=scripts/development/dev-mounts-common.sh
source "$(dirname -- "${BASH_SOURCE[0]}")/dev-mounts-common.sh"

usage() {
  sed -n '4,32p' "${BASH_SOURCE[0]}" | sed 's/^# \{0,1\}//'
}

while [ $# -gt 0 ]; do
  case "$1" in
    --dry-run) DRY_RUN=1 ;;
    -y|--yes)  ASSUME_YES=1 ;;
    -h|--help) usage; exit 0 ;;
    *) err "unknown argument: $1"; usage >&2; exit 2 ;;
  esac
  shift
done

cd "$(repo_root)"
require_container

[ "$DRY_RUN" -eq 1 ] && warn "dry run — no rows will be deleted"

backup_sql_path="$(sq "$BACKUP_PATH")"
app_sql_path="$(sq "$APP_CATALOG_PATH")"
plugin_sql_path="$(sq "$PLUGIN_CATALOG_PATH")"

# --- what is about to go ---------------------------------------------
loc_ids="$(sql "SELECT id FROM storage_locations WHERE location_type = 'local' AND base_path = '$backup_sql_path';")"
cat_count="$(sql "SELECT COUNT(*) FROM settings_catalogs
  WHERE (kind = 'docker_app' AND url = '$app_sql_path')
     OR (kind = 'plugin'     AND url = '$plugin_sql_path');")"

if [ -z "$loc_ids" ] && [ "$cat_count" = "0" ]; then
  ok "Nothing to remove — the database is already clean."
  exit 0
fi

info "Planned removals:"
if [ -n "$loc_ids" ]; then
  while IFS= read -r id; do
    [ -z "$id" ] && continue
    plans="$(sql "SELECT COUNT(*) FROM container_backup_plan_storage_locations WHERE storage_location_id = '$(sq "$id")';")"
    plan_default="$(sql "SELECT COUNT(*) FROM container_backup_plans WHERE storage_location_id = '$(sq "$id")';")"
    runs="$(sql "SELECT COUNT(*) FROM container_backup_run_destinations WHERE storage_location_id = '$(sq "$id")';")"
    dim "  storage_location $id ($BACKUP_PATH)"
    dim "    $plans plan link(s) cascade-deleted, $plan_default plan default(s) and $runs run destination(s) set to NULL"
    if [ "$plans" != "0" ] || [ "$plan_default" != "0" ]; then
      warn "    backup plans reference this location — they will lose their destination."
    fi
  done <<< "$loc_ids"
fi
if [ "$cat_count" != "0" ]; then
  docker compose exec -T "$SERVICE" sqlite3 -column "$DB" \
    "SELECT '  catalog ' || id || ' (' || kind || ') ' || url FROM settings_catalogs
      WHERE (kind = 'docker_app' AND url = '$app_sql_path')
         OR (kind = 'plugin'     AND url = '$plugin_sql_path');"
fi

# --- confirm ----------------------------------------------------------
if [ "$DRY_RUN" -eq 0 ] && [ "$ASSUME_YES" -eq 0 ]; then
  printf '%sProceed? [y/N] %s' "$C_YELLOW" "$C_OFF"
  read -r reply < /dev/tty
  case "$reply" in
    y|Y|yes|YES) ;;
    *) err "Aborted."; exit 1 ;;
  esac
fi

# --- delete -----------------------------------------------------------
# One statement so the pragma and the deletes share a connection; the
# pragma is per-connection and is lost between exec invocations.
info "Removing rows..."
sql_write "PRAGMA foreign_keys = ON;
  DELETE FROM storage_locations
    WHERE location_type = 'local' AND base_path = '$backup_sql_path';
  DELETE FROM settings_catalogs
    WHERE (kind = 'docker_app' AND url = '$app_sql_path')
       OR (kind = 'plugin'     AND url = '$plugin_sql_path');"
if [ "$DRY_RUN" -eq 0 ]; then ok "  removed"; fi

print_state

echo
ok "Done. Reload Settings -> Storage and Settings -> Catalogs in the UI."
reseed_notice
