#!/bin/sh
# Pulihkan database dari cadangan: restore <YYYY-MM-DD | YYYY-Www>. MENGHAPUS database $PGDATABASE lalu mengisinya ulang.
# Hentikan api dulu (docker compose stop api) dan set CONFIRM_RESTORE=yes.
set -eu
dest="${BACKUP_DEST:?BACKUP_DEST kosong}"
name="${1:?pakai: restore <YYYY-MM-DD | YYYY-Www>}"
case "$name" in *W*) dir=weekly ;; *) dir=daily ;; esac

if [ "${CONFIRM_RESTORE:-}" != yes ]; then
  echo "Ini MENGHAPUS database $PGDATABASE dan menggantinya dengan cadangan $dir/$name."
  echo "Ulangi dengan: docker compose ... run --rm -e CONFIRM_RESTORE=yes backup restore $name"
  exit 1
fi

tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT
rclone copyto "$dest/db/$dir/$name.dump" "$tmp/r.dump"
pg_restore --list "$tmp/r.dump" > /dev/null

psql -d postgres -qc "DROP DATABASE IF EXISTS \"$PGDATABASE\" WITH (FORCE)"
psql -d postgres -qc "CREATE DATABASE \"$PGDATABASE\""
pg_restore --no-owner -d "$PGDATABASE" "$tmp/r.dump" || echo "pg_restore memberi peringatan; periksa hasil di bawah"
echo "pengguna di database pulih: $(psql -d "$PGDATABASE" -tAc 'SELECT count(*) FROM users')"
echo "SELESAI. Jalankan lagi: docker compose ... up -d api"
