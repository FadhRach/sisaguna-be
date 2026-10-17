#!/bin/sh
# Mode: run (default, jadwal harian 02:00 WIB) | now (cadangan sekarang) | restore-test (uji pulih cadangan terbaru)
#       | restore <tanggal> (pulihkan database sungguhan; lihat restore.sh)
set -eu

if [ -z "${BACKUP_DEST:-}" ]; then
  if [ -n "${R2_BUCKET:-}" ] && [ -n "${R2_ACCOUNT_ID:-}" ] && [ -n "${R2_ACCESS_KEY_ID:-}" ] && [ -n "${R2_SECRET_ACCESS_KEY:-}" ]; then
    # NO_CHECK_BUCKET: token R2 yang dibatasi satu bucket tidak boleh CreateBucket
    export RCLONE_CONFIG_R2_TYPE=s3 RCLONE_CONFIG_R2_PROVIDER=Cloudflare RCLONE_CONFIG_R2_NO_CHECK_BUCKET=true
    export RCLONE_CONFIG_R2_ACCESS_KEY_ID="$R2_ACCESS_KEY_ID" RCLONE_CONFIG_R2_SECRET_ACCESS_KEY="$R2_SECRET_ACCESS_KEY"
    export RCLONE_CONFIG_R2_ENDPOINT="${R2_ENDPOINT:-https://${R2_ACCOUNT_ID}.r2.cloudflarestorage.com}"
    export BACKUP_DEST="r2:${R2_BUCKET}"
  else
    echo "PERINGATAN: R2 belum dikonfigurasi, cadangan disimpan di /backups (satu disk dengan server; isi R2_* untuk cadangan di luar VPS)"
    export BACKUP_DEST=/backups
  fi
fi

case "${1:-run}" in
  now) exec backup.sh ;;
  restore-test) exec restore-test.sh ;;
  restore) shift; exec restore.sh "$@" ;;
  run)
    [ "${RUN_NOW:-0}" = 1 ] && backup.sh
    while true; do
      # 19:00 UTC = 02:00 WIB
      now=$(date -u +%s)
      next=$(( now / 86400 * 86400 + 19 * 3600 ))
      [ "$next" -le "$now" ] && next=$(( next + 86400 ))
      echo "cadangan berikutnya dalam $(( next - now )) detik"
      sleep $(( next - now ))
      backup.sh || echo "CADANGAN GAGAL, coba lagi besok"
    done
    ;;
  *) echo "pakai: run | now | restore-test | restore <tanggal>"; exit 2 ;;
esac
