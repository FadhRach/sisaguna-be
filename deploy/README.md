# Deploy SisaGuna ke VPS (Docker Compose)

Satu VPS menjalankan lima container: `caddy` (HTTPS), `api`, `postgres` (PostGIS), `backup`, dan dua tugas sekali jalan (`migrate`, `media-init`).
Semua perintah dijalankan dari **root repo** di VPS.

Sudah diuji di mesin lokal dengan stack yang sama (Caddy HTTPS, upload, foto publik lewat Caddy, folder privat tidak terjangkau, WSS,
backup, uji pulih, sinkron ke endpoint S3). Yang tidak bisa diuji di luar VPS: sertifikat Let's Encrypt asli dan R2 sungguhan.

## 1. Persiapan (sekali)

| Kebutuhan | Catatan |
| --- | --- |
| VPS Ubuntu 22.04/24.04, minimal 1 vCPU / 1 GB RAM / 20 GB disk | Foto disimpan di disk VPS; pantau ruang disk |
| Domain, mis. `api.sisaguna.id` | Buat record **A** (dan **AAAA** kalau punya IPv6) ke IP VPS sebelum menjalankan Caddy |
| Port 80 dan 443 terbuka | Caddy butuh keduanya untuk sertifikat Let's Encrypt |
| (Disarankan) bucket Cloudflare R2 privat + API token Object Read & Write | Tanpa R2, backup database hanya tersimpan di disk VPS yang sama |

```bash
# Docker + compose plugin
curl -fsSL https://get.docker.com | sh
# Firewall: hanya SSH, HTTP, HTTPS. Postgres tidak punya port publik sama sekali.
ufw allow OpenSSH && ufw allow 80/tcp && ufw allow 443/tcp && ufw enable
```

## 2. Konfigurasi

```bash
git clone <url-repo> /opt/sisaguna && cd /opt/sisaguna
cp .env.example .env
```

Isi `.env` (yang dipakai produksi):

| Variabel | Isi |
| --- | --- |
| `DOMAIN` | `api.sisaguna.id` |
| `PUBLIC_BASE_URL` | `https://api.sisaguna.id` (dipakai menyusun URL foto yang dikirim ke aplikasi) |
| `POSTGRES_USER`, `POSTGRES_DB` | bebas, mis. `sisaguna` |
| `POSTGRES_PASSWORD` | acak panjang: `openssl rand -hex 24` |
| `JWT_SECRET` | `openssl rand -base64 48` (minimal 32 byte) |
| `ADMIN_EMAIL`, `ADMIN_PASSWORD`, `ADMIN_NAME` | akun admin pertama (dibuat seeder) |
| `PARTNERS_MODE` | `dummy` untuk menguji app dengan kurir (GoSend, Grab, SPX, Lalamove) dan pembayaran QRIS/e-wallet **tiruan**; kosong/`off` = hanya tunai + pickup (default produksi). **Jangan biarkan `dummy` menyala saat ada penjual dan pembeli sungguhan**: pesanan non-tunai tampil `paid` padahal tidak ada uang yang masuk |
| `MERCHANT_AUTO_APPROVE` | `true` = toko baru langsung `approved` (hanya VPS uji). Kosong/`false` (default) = menunggu admin; admin mendapat notifikasi "Toko baru menunggu persetujuan" |
| `EMAIL_MODE`, `SMTP_HOST`, `SMTP_PORT`, `SMTP_USER`, `SMTP_PASSWORD`, `SMTP_FROM` | email kode lupa password. Tanpa `SMTP_HOST` di produksi, `EMAIL_MODE` jadi `off`: endpoint tetap menjawab 202 tetapi tidak ada email terkirim. Untuk uji tanpa SMTP: `EMAIL_MODE=log` mencetak kode ke `docker compose logs api` (jangan dipakai bersama pengguna sungguhan) |
| `FCM_CREDENTIALS_FILE`, `FCM_PROJECT_ID` | push FCM. Taruh JSON service account Firebase di `/srv/sisaguna/secrets/fcm.json` (`chmod 600`, bisa dibaca user API) lalu isi `FCM_CREDENTIALS_FILE=/run/secrets/fcm.json`. Kosong = push mati |
| `R2_ACCOUNT_ID`, `R2_ACCESS_KEY_ID`, `R2_SECRET_ACCESS_KEY`, `R2_BUCKET` | opsional tapi disarankan. Kalau keempatnya terisi: foto disalin ke R2 tiap menit dan backup harian dikirim ke R2 |

`DATABASE_URL` dan `POSTGRES_PORT` di `.env.example` hanya untuk development lokal; compose produksi menyusun sendiri alamat Postgres di jaringan Docker.

## 3. Menjalankan

```bash
docker compose -f deploy/docker-compose.prod.yml --env-file .env up -d --build
docker compose -f deploy/docker-compose.prod.yml --env-file .env ps        # semua Up; migrate dan media-init Exited (0)

# Data awal produksi: akun admin + voucher umum. Aman diulang (idempoten).
docker compose -f deploy/docker-compose.prod.yml --env-file .env run --rm --entrypoint /seed api -env prod
```

Untuk mempersingkat perintah: `alias dc='docker compose -f deploy/docker-compose.prod.yml --env-file .env'`.

## 4. Verifikasi (sebelum memberi tahu developer mobile)

```bash
curl https://api.sisaguna.id/healthz                    # {"data":{"status":"ok"}}  (Postgres + folder media bisa ditulis)
ss -tlnp | grep 5432                                    # harus kosong: Postgres tidak terbuka ke luar
dc logs api | tail                                      # JSON per baris; cari "server started"
dc run --rm backup now                                  # backup pertama; cek file muncul di R2 (atau /srv/sisaguna/backups)
dc run --rm backup restore-test                         # harus berakhir "UJI PULIH BERHASIL"
```

Lalu login sebagai admin lewat `POST /api/v1/auth/login` dan coba `GET /api/v1/admin/merchants`. Untuk uji menyeluruh,
impor `apiendpoints/SisaGuna.postman_collection.json`, ubah variabel `baseUrl` ke domain, dan jalankan folder `★ Alur Cepat MVP`
(butuh data contoh; di produksi buat data lewat aplikasi, jangan jalankan seed dev).

## 5. Operasional

| Tugas | Perintah |
| --- | --- |
| Update versi | `git pull && dc up -d --build` (migrasi database berjalan otomatis lewat container `migrate`) |
| Log | `dc logs -f api` (field `request_id`, `user_id`, `route`; token, password, dan isi chat tidak pernah di-log) |
| Backup manual | `dc run --rm backup now` |
| Uji pulih (lakukan tiap bulan) | `dc run --rm backup restore-test` |
| Restart | `dc restart api` |

**Backup otomatis:** container `backup` menjalankan `pg_dump` tiap hari 02:00 WIB. Disimpan 14 hari harian dan 8 minggu mingguan
(`db/daily/YYYY-MM-DD.dump`, `db/weekly/YYYY-Www.dump`). Foto disalin ke R2 oleh API dengan key yang sama dengan path lokal
(`media/public/ab/<sha>.jpg`).

**Monitoring uptime:** daftarkan `https://api.sisaguna.id/healthz` di UptimeRobot atau layanan sejenis (interval 1–5 menit, alert ke email/WhatsApp).
`/healthz` menjawab 503 kalau Postgres mati atau folder media tidak bisa ditulis.

## 5b. Data dummy di VPS uji

Hanya untuk VPS yang dipakai menguji app (akun dummy berpassword umum). Dari root repo:

```bash
deploy/demo.sh refresh   # isi/segarkan dummy tanpa menghapus apa pun (waktu pickup + stok kembali ke awal)
deploy/demo.sh reset     # HAPUS seluruh database, migrasi ulang, isi dummy dari nol (minta ketik "reset")
```

`make reset-dev` hanya untuk Mac (compose dev); di VPS selalu pakai `deploy/demo.sh`. Seed `dev` ditolak oleh API produksi kecuali
`ALLOW_DEMO_SEED=true`, yang diatur skrip hanya untuk perintah itu. Untuk VPS sungguhan jangan pernah menjalankannya.

## 6. Pemulihan dari nol (VPS baru atau data rusak)

1. Pasang Docker, clone repo, isi `.env` yang **sama** (terutama `JWT_SECRET`, supaya sesi pengguna tetap berlaku, dan `POSTGRES_PASSWORD`).
2. `dc up -d postgres` lalu tunggu healthy.
3. Pulihkan database dari cadangan: `dc stop api` lalu
   `dc run --rm -e CONFIRM_RESTORE=yes backup restore 2026-10-17` (tanggal cadangan; mingguan: `2026-W42`). Perintah ini **menghapus** database lalu mengisinya ulang.
4. Pulihkan foto dari R2 ke folder media (rclone dengan konfigurasi yang sama dengan R2 di `.env`):
   `rclone sync r2:<bucket>/media /srv/sisaguna/media` (key bucket sama persis dengan path lokal).
5. `dc up -d --build` dan jalankan verifikasi di bagian 4.

## 7. Catatan keamanan

- Postgres tidak punya `ports` di compose: hanya terjangkau dari jaringan Docker. Jangan menambahkannya.
- Folder `media/private` (foto komplain) tidak di-mount ke Caddy; hanya API yang menyajikannya setelah cek kepemilikan.
- Bucket R2 dibuat **privat**; aplikasi tidak pernah menerima URL R2.
- Token WebSocket bukan access token: tiket sekali pakai 30 detik. Karena tiket disimpan di memori API, jalankan **satu** instance `api`.
- Backup lokal (tanpa R2) berada di disk yang sama dengan database. Anggap itu cadangan sementara, bukan pengganti R2.
