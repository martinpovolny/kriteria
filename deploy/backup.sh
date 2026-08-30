#!/usr/bin/env bash
# Nightly SQLite backup for the kriteria production database.
#
# Uses `sqlite3 .backup`, which is safe to run against a live database in
# WAL mode (it takes a read lock, not an exclusive one — the running server
# is never interrupted).
#
# Retention: daily backups for DAILY_RETENTION_DAYS, then thinned to one
# snapshot per ISO week (the oldest one in each week) kept forever.
set -euo pipefail

DEPLOY_DIR="${DEPLOY_DIR:-$HOME/Projects/kriteria}"
DB_PATH="${DB_PATH:-$DEPLOY_DIR/data/kriteria.db}"
BACKUP_DIR="${BACKUP_DIR:-$DEPLOY_DIR/data/backups}"
DAILY_RETENTION_DAYS="${DAILY_RETENTION_DAYS:-14}"

mkdir -p "$BACKUP_DIR"

stamp="$(date +%Y%m%d-%H%M%S)"
dest="$BACKUP_DIR/kriteria-$stamp.db"
tmp="$dest.tmp"

sqlite3 "$DB_PATH" ".backup '$tmp'"
mv "$tmp" "$dest"

echo "backed up $DB_PATH -> $dest"

# --- Thin backups older than DAILY_RETENTION_DAYS to one-per-ISO-week ---
# Among files past the daily window, keep only the earliest file of each
# ISO year-week and delete the rest. Idempotent: re-running never removes
# the already-kept weekly snapshot, and thinning happens gradually as each
# day's file ages past the window.
keep_week=""
while IFS= read -r f; do
    [ -z "$f" ] && continue
    base="$(basename "$f")"
    datepart="${base#kriteria-}"
    datepart="${datepart%%-*}" # YYYYMMDD
    week="$(date -d "$datepart" +%G-%V)"
    if [ "$week" = "$keep_week" ]; then
        rm -f "$f"
    else
        keep_week="$week"
    fi
done < <(find "$BACKUP_DIR" -name 'kriteria-*.db' -mtime "+$DAILY_RETENTION_DAYS" | sort)
