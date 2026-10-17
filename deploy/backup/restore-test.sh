#!/bin/sh
# Uji pemulihan: unduh cadangan harian terbaru, pulihkan ke database sementara, bandingkan jumlah baris
# dengan database hidup, lalu hapus database sementara. Exit 0 hanya kalau hasilnya masuk akal.
set -eu
dest="${BACKUP_DEST:?BACKUP_DEST kosong}"
latest=$(rclone lsf "$dest/db/daily" 2> /dev/null | sort | tail -1)
[ -n "$latest" ] || { echo "GAGAL: belum ada cadangan di $dest/db/daily"; exit 1; }

tmp=$(mktemp -d)
scratch="restore_test_$(date +%s)"
cleanup() { psql -d postgres -qc "DROP DATABASE IF EXISTS $scratch" > /dev/null 2>&1 || true; rm -rf "$tmp"; }
trap cleanup EXIT

rclone copyto "$dest/db/daily/$latest" "$tmp/latest.dump"
psql -d postgres -qc "CREATE DATABASE $scratch"
# pg_restore bisa melaporkan error ringan (mis. ekstensi); kelayakan hasil dinilai dari verifikasi di bawah
pg_restore --no-owner -d "$scratch" "$tmp/latest.dump" 2> "$tmp/restore.log" || echo "pg_restore memberi peringatan (lihat ringkasan di bawah)"

count() { psql -d "$1" -tAc "SELECT count(*) FROM $2"; }
ok=1
echo "cadangan: $latest"
echo "PostGIS : $(psql -d "$scratch" -tAc 'SELECT PostGIS_Version()')"
printf '%-14s %10s %10s\n' tabel pulih hidup
for t in users merchants listings orders messages media; do
  restored=$(count "$scratch" "$t")
  live=$(count "$PGDATABASE" "$t")
  printf '%-14s %10s %10s\n' "$t" "$restored" "$live"
  # cadangan boleh lebih lama dari database hidup, tapi tidak boleh lebih banyak atau kosong padahal hidup berisi
  [ "$restored" -le "$live" ] || ok=0
  [ "$live" -eq 0 ] || [ "$restored" -gt 0 ] || ok=0
done
if [ "$ok" = 1 ]; then echo "UJI PULIH BERHASIL"; else echo "UJI PULIH GAGAL"; head -20 "$tmp/restore.log"; exit 1; fi
