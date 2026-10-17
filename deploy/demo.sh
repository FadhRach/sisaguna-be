#!/bin/sh
# Data dummy (salinan dummy app Android) di VPS UJI. JANGAN dipakai di VPS dengan pengguna sungguhan:
# semua akun dummy berpassword umum "Password123!".
#
#   deploy/demo.sh refresh   isi/segarkan dummy tanpa menghapus apa pun (waktu pickup + stok kembali ke awal)
#   deploy/demo.sh reset     HAPUS seluruh database, migrasi ulang, isi dummy dari nol (foto tidak dihapus)
#
# Jalankan dari root repo. Butuh .env produksi (atau ENV_FILE=path), dan PARTNERS_MODE=dummy kalau mau kurir/pembayaran tiruan.
set -eu
cd "$(dirname "$0")/.."
ENV_FILE="${ENV_FILE:-.env}"
dc() { docker compose -f deploy/docker-compose.prod.yml ${OVERRIDE:+-f "$OVERRIDE"} --env-file "$ENV_FILE" "$@"; }
seed() { dc run --rm -e ALLOW_DEMO_SEED=true --entrypoint /seed api -env "$1"; }
db() { # nilai variabel dari env file, mis. POSTGRES_USER
  sed -n "s/^$1=//p" "$ENV_FILE" | tail -1 | tr -d '"'
}

case "${1:-}" in
  refresh)
    seed dev
    ;;
  reset)
    if [ "${CONFIRM:-}" != reset ]; then
      printf 'Ini MENGHAPUS seluruh database %s. Ketik "reset" untuk lanjut: ' "$(db POSTGRES_DB)"
      read -r answer
      [ "$answer" = reset ] || { echo batal; exit 1; }
    fi
    dc stop api
    dc up -d postgres
    dc exec -T postgres sh -c "until pg_isready -h 127.0.0.1 -U '$(db POSTGRES_USER)' >/dev/null 2>&1; do sleep 1; done"
    dc exec -T postgres psql -U "$(db POSTGRES_USER)" -d postgres -qc "DROP DATABASE IF EXISTS \"$(db POSTGRES_DB)\" WITH (FORCE)"
    dc exec -T postgres psql -U "$(db POSTGRES_USER)" -d postgres -qc "CREATE DATABASE \"$(db POSTGRES_DB)\""
    dc run --rm migrate
    seed prod   # admin dari ADMIN_EMAIL
    seed dev
    dc up -d api
    ;;
  *)
    echo "pakai: deploy/demo.sh refresh | reset"
    exit 2
    ;;
esac
