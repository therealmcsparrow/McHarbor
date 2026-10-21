#!/usr/bin/env bash
# Copyright (c) 2026 McSparrow. All rights reserved.
# McHarbor is licensed under the McHarbor License. See LICENSE for details.
#
# Development helper — registers the three local bind mounts with the
# running McHarbor container:
#
#   /mnt/backup          -> storage_locations   (location_type = local)
#   /mnt/app-catalog     -> settings_catalogs   (kind = docker_app)
#   /mnt/plugin-catalog  -> settings_catalogs   (kind = plugin)
#
# Why SQL and not the REST API: storage locations of type "local" are
# protected by the service layer (ErrLocalStorageProtected in
# src/backend/modules/storage_locations/service.go), so the API can
# neither create the counterpart nor let the remove script take it back
# out. Writing both sides through sqlite3 keeps the pair symmetrical.
#
# Idempotent — re-running is a no-op. Undo with
# scripts/development/remove-dev-storage-and-catalogs.sh.
#
# Usage:
#   bash scripts/development/add-dev-storage-and-catalogs.sh [--dry-run]
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
  sed -n '4,30p' "${BASH_SOURCE[0]}" | sed 's/^# \{0,1\}//'
}

while [ $# -gt 0 ]; do
  case "$1" in
    --dry-run) DRY_RUN=1 ;;
    -h|--help) usage; exit 0 ;;
    *) err "unknown argument: $1"; usage >&2; exit 2 ;;
  esac
  shift
done

cd "$(repo_root)"
require_container

[ "$DRY_RUN" -eq 1 ] && warn "dry run — no rows will be written"

# --- host side --------------------------------------------------------
# The compose file binds ./mnt/<dir> unless MCHARBOR_LOCAL_*_PATH says
# otherwise. Docker would create a missing source dir as root-owned, so
# create it here instead, owned by the invoking user.
info "Ensuring host bind-mount directories exist..."
for d in mnt/backup mnt/app-catalog mnt/plugin-catalog; do
  if [ -d "$d" ]; then
    dim "  present  ./$d"
  elif [ "$DRY_RUN" -eq 1 ]; then
    dim "  [dry-run] mkdir -p ./$d"
  else
    mkdir -p "$d"
    ok "  created  ./$d"
  fi
done

# --- container side ---------------------------------------------------
info "Verifying the mounts inside '$SERVICE'..."
missing=0
for p in "$BACKUP_PATH" "$APP_CATALOG_PATH" "$PLUGIN_CATALOG_PATH"; do
  if path_in_container "$p"; then
    dim "  present  $p"
  else
    warn "  MISSING  $p — the row will be written but reads as a broken source"
    warn "           until docker-compose.yml mounts it and the container is recreated."
    missing=$((missing + 1))
  fi
done
[ "$missing" -gt 0 ] && warn "$missing mount(s) missing inside the container."

# --- storage location -------------------------------------------------
# Keyed on (location_type, base_path) rather than id: the row may
# already exist as 'default-local-backup' from migration 050/051, and a
# second row on the same path would show up twice in the backup picker.
info "Registering local storage location $BACKUP_PATH ..."
if [ "$(sql "SELECT COUNT(*) FROM storage_locations WHERE location_type = 'local' AND base_path = '$(sq "$BACKUP_PATH")';")" != "0" ]; then
  dim "  already registered — skipped"
else
  sql_write "INSERT INTO storage_locations
      (id, name, location_type, enabled, base_path, passive_mode, config, created_at, updated_at)
    VALUES
      ('default-local-backup', '$(sq "$BACKUP_NAME")', 'local', 1, '$(sq "$BACKUP_PATH")', 1, '{}', $NOW_SQL, $NOW_SQL)
    ON CONFLICT (id) DO NOTHING;"
  ok "  added '$BACKUP_NAME' -> $BACKUP_PATH"
fi

# --- catalogs ---------------------------------------------------------
# settings_catalogs carries UNIQUE(kind, url), so the guard below is
# also what keeps catalogs.SeedLocalCatalogs from colliding on boot.
add_catalog() {
  local id="$1" name="$2" kind="$3" url="$4"
  if [ "$(sql "SELECT COUNT(*) FROM settings_catalogs WHERE kind = '$(sq "$kind")' AND url = '$(sq "$url")';")" != "0" ]; then
    dim "  already registered — skipped ($kind $url)"
    return
  fi
  sql_write "INSERT INTO settings_catalogs
      (id, name, url, kind, enabled, readonly, created_at, updated_at)
    VALUES
      ('$(sq "$id")', '$(sq "$name")', '$(sq "$url")', '$(sq "$kind")', 1, 0, $NOW_SQL, $NOW_SQL)
    ON CONFLICT (id) DO NOTHING;"
  ok "  added '$name' ($kind) -> $url"
}

info "Registering local catalogs..."
add_catalog "dev-local-app-catalog"    "$APP_CATALOG_NAME"    "docker_app" "$APP_CATALOG_PATH"
add_catalog "dev-local-plugin-catalog" "$PLUGIN_CATALOG_NAME" "plugin"     "$PLUGIN_CATALOG_PATH"

print_state

echo
ok "Done. Reload Settings -> Storage and Settings -> Catalogs in the UI."
reseed_notice
