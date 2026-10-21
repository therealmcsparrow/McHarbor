# Copyright (c) 2026 McSparrow. All rights reserved.
# McHarbor is licensed under the McHarbor License. See LICENSE for details.
#
# Shared plumbing for add-dev-storage-and-catalogs.sh and
# remove-dev-storage-and-catalogs.sh. Sourced, never executed.

# Git Bash on Windows rewrites /app/... into a host path before docker
# ever sees it, which turns every sqlite3 call into
# "unable to open database P:/Programs/Git/app/data/mcharbor.db".
export MSYS_NO_PATHCONV=1
export MSYS2_ARG_CONV_EXCL='*'

# --- knobs ------------------------------------------------------------
SERVICE="${MCHARBOR_SERVICE:-mcharbor}"
DB="${MCHARBOR_DB:-/app/data/mcharbor.db}"

BACKUP_PATH="${MCHARBOR_DEV_BACKUP_PATH:-/mnt/backup}"
APP_CATALOG_PATH="${MCHARBOR_DEV_APP_CATALOG_PATH:-/mnt/app-catalog}"
PLUGIN_CATALOG_PATH="${MCHARBOR_DEV_PLUGIN_CATALOG_PATH:-/mnt/plugin-catalog}"

# Names shown in Settings -> Storage / Settings -> Catalogs. They match
# what migration 051 and catalogs.SeedLocalCatalogs would have written,
# so a row this script adds is indistinguishable from a seeded one.
BACKUP_NAME="Local storage"
APP_CATALOG_NAME="Local app catalog"
PLUGIN_CATALOG_NAME="Local dev catalog"

DRY_RUN=0
ASSUME_YES=0

# --- output -----------------------------------------------------------
if [ -t 1 ]; then
  C_RED=$'\033[31m'; C_GREEN=$'\033[32m'; C_BLUE=$'\033[34m'
  C_YELLOW=$'\033[33m'; C_DIM=$'\033[2m'; C_OFF=$'\033[0m'
else
  C_RED=''; C_GREEN=''; C_BLUE=''; C_YELLOW=''; C_DIM=''; C_OFF=''
fi

info()  { printf '%s%s%s\n' "$C_BLUE"   "$*" "$C_OFF"; }
ok()    { printf '%s%s%s\n' "$C_GREEN"  "$*" "$C_OFF"; }
warn()  { printf '%s%s%s\n' "$C_YELLOW" "$*" "$C_OFF" >&2; }
err()   { printf '%s%s%s\n' "$C_RED"    "$*" "$C_OFF" >&2; }
dim()   { printf '%s%s%s\n' "$C_DIM"    "$*" "$C_OFF"; }

# --- repo root --------------------------------------------------------
# Both scripts run docker compose, which needs the directory holding
# docker-compose.yml regardless of where the operator invoked them from.
repo_root() {
  local here
  here="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/../.." && pwd)"
  printf '%s\n' "$here"
}

# --- preflight --------------------------------------------------------
require_container() {
  if ! command -v docker >/dev/null 2>&1; then
    err "docker not found on PATH."
    exit 1
  fi
  if ! docker compose ps --status running --services 2>/dev/null | grep -qx "$SERVICE"; then
    err "compose service '$SERVICE' is not running."
    err "Start it first:  docker compose up -d"
    err "Or point the script elsewhere:  MCHARBOR_SERVICE=<name> $0"
    exit 1
  fi
  if ! docker compose exec -T "$SERVICE" sh -c 'command -v sqlite3 >/dev/null'; then
    err "sqlite3 is missing inside '$SERVICE'. Rebuild the image (docker/Dockerfile installs it)."
    exit 1
  fi
  if ! docker compose exec -T "$SERVICE" sh -c "[ -f '$DB' ]"; then
    err "database not found at $DB inside '$SERVICE'."
    err "Override with MCHARBOR_DB=<path>."
    exit 1
  fi
}

# sql runs a statement against the container DB. Every write goes
# through here so --dry-run has exactly one place to intercept.
sql() {
  docker compose exec -T "$SERVICE" sqlite3 "$DB" "$1"
}

sql_write() {
  if [ "$DRY_RUN" -eq 1 ]; then
    dim "  [dry-run] $(printf '%s' "$1" | tr '\n' ' ' | tr -s ' ')"
    return 0
  fi
  sql "$1"
}

# path_in_container reports whether a path exists inside the container
# and is a directory. A catalog row pointing at a path the backend
# cannot stat renders as a broken source in the UI, so both scripts
# check before writing and warn loudly rather than failing.
path_in_container() {
  docker compose exec -T "$SERVICE" sh -c "[ -d '$1' ]"
}

# now_rfc3339 matches the format the Go services write
# (time.Time.Format(time.RFC3339)) so hand-written rows parse the same
# way the app's own inserts do.
NOW_SQL="strftime('%Y-%m-%dT%H:%M:%SZ','now')"

# sq escapes a value for a single-quoted SQL literal.
sq() { printf "%s" "$1" | sed "s/'/''/g"; }

print_state() {
  echo
  info "storage_locations (local):"
  docker compose exec -T "$SERVICE" sqlite3 -header -column "$DB" \
    "SELECT id, name, base_path, enabled FROM storage_locations WHERE location_type = 'local';" \
    || true
  info "settings_catalogs (local paths):"
  docker compose exec -T "$SERVICE" sqlite3 -header -column "$DB" \
    "SELECT id, name, kind, url, enabled FROM settings_catalogs WHERE url LIKE '/%' ORDER BY kind, name;" \
    || true
}

reseed_notice() {
  cat <<'NOTE'

Note: this only changes the current database.
  - migration 051_protect_local_storage.sql recreates the /mnt/backup
    local storage row on a fresh data volume;
  - catalogs.SeedLocalCatalogs re-registers /mnt/app-catalog and
    /mnt/plugin-catalog on EVERY boot whenever those paths exist in the
    container and hold at least one .json file.
For a genuinely clean production image, drop the matching bind mounts
from docker-compose.yml as well.
NOTE
}
