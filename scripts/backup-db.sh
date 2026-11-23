#!/usr/bin/env bash
# Safe SQLite backup for WAL-mode databases (never use raw cp on an active WAL DB).
#
# Usage:
#   ./scripts/backup-db.sh [db_path] [output_dir]
#   ./scripts/backup-db.sh --docker [output_dir] [volume_name]
set -euo pipefail

timestamp="$(date +%Y%m%d-%H%M%S)"
filename="frequencia-${timestamp}.db"

if [[ "${1:-}" == "--help" || "${1:-}" == "-h" ]]; then
  sed -n '2,6p' "$0" | sed 's/^# \{0,1\}//'
  exit 0
fi

if [[ "${1:-}" == "--docker" ]]; then
  out_dir="${2:-./backups}"
  volume="${3:-$(basename "$PWD")_frequencia_data}"
  mkdir -p "$out_dir"
  abs_out="$(cd "$out_dir" && pwd)"

  docker run --rm \
    -v "${volume}:/data" \
    -v "${abs_out}:/backups" \
    keinos/sqlite3:latest \
    sqlite3 /data/frequencia.db ".timeout 5000" ".backup '/backups/${filename}'"

  dest="${abs_out}/${filename}"
else
  db_path="${1:-./data/frequencia.db}"
  out_dir="${2:-./backups}"
  if [[ ! -f "$db_path" ]]; then
    echo "error: database file not found: $db_path" >&2
    exit 1
  fi
  mkdir -p "$out_dir"
  dest="${out_dir}/${filename}"
  sqlite3 "$db_path" ".timeout 5000" ".backup '${dest}'"
fi

if [[ ! -s "$dest" ]]; then
  echo "error: backup file is empty: $dest" >&2
  exit 1
fi

echo "backup created: ${dest} ($(du -h "$dest" | cut -f1))"
