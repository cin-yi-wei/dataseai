#!/bin/bash
# Daily backup of dataseai SQLite to GCS.
# Online .backup (no service downtime) → gzip → upload → prune > 30 days.
# Run via cron on the host that has /opt/dataseai/data/dataseai.db.
set -euo pipefail

DB_PATH="${DB_PATH:-/opt/dataseai/data/dataseai.db}"
BUCKET="${BUCKET:-gs://conray-dataseai-backups}"
KEEP_DAYS="${KEEP_DAYS:-30}"

TS=$(date -u +%Y%m%d-%H%M%S)
TMP=$(mktemp -d)
trap 'rm -rf "$TMP"' EXIT

sqlite3 "$DB_PATH" ".backup '$TMP/dataseai.db'"
gzip "$TMP/dataseai.db"
gsutil -q cp "$TMP/dataseai.db.gz" "$BUCKET/dataseai-$TS.db.gz"

CUTOFF=$(date -u -d "$KEEP_DAYS days ago" +%Y%m%d)
gsutil ls "$BUCKET/dataseai-"*".db.gz" 2>/dev/null | while read -r obj; do
  d=$(echo "$obj" | grep -oE 'dataseai-[0-9]{8}' | grep -oE '[0-9]{8}')
  if [ -n "$d" ] && [ "$d" -lt "$CUTOFF" ]; then
    gsutil -q rm "$obj"
    echo "pruned $obj"
  fi
done

echo "$(date -Iseconds) backup ok: dataseai-$TS.db.gz"
