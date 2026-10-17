#!/bin/sh
# pg_dump format custom -> $BACKUP_DEST/db/daily/YYYY-MM-DD.dump; Minggu juga disalin ke db/weekly/.
# Simpan 14 hari harian dan 8 minggu mingguan. Koneksi lewat PGHOST/PGUSER/PGPASSWORD/PGDATABASE.
set -eu
dest="${BACKUP_DEST:?BACKUP_DEST kosong}"
day=$(date -u +%F)
tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT

pg_dump -Fc --no-owner -f "$tmp/$day.dump"
pg_restore --list "$tmp/$day.dump" > /dev/null # dump rusak -> gagal sekarang, bukan saat darurat

rclone copyto "$tmp/$day.dump" "$dest/db/daily/$day.dump"
if [ "$(date -u +%u)" = 7 ]; then
  rclone copyto "$tmp/$day.dump" "$dest/db/weekly/$(date -u +%G-W%V).dump"
fi
rclone delete --min-age 14d "$dest/db/daily" 2> /dev/null || true
rclone delete --min-age 56d "$dest/db/weekly" 2> /dev/null || true
echo "cadangan $day selesai: $(du -h "$tmp/$day.dump" | cut -f1)"
