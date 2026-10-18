# SisaGuna Backend

REST API dan server realtime untuk SisaGuna, marketplace makanan berlebih dari UMKM kuliner.
Go (Gin) + Postgres/PostGIS, dijalankan di satu VPS lewat Docker Compose. Aturan proyek dan
status milestone ada di [`CLAUDE.md`](./CLAUDE.md); kontrak API di [`api/openapi.yaml`](./api/openapi.yaml).

## Prasyarat

Go 1.27+, Docker, [`migrate`](https://github.com/golang-migrate/migrate) CLI, dan `sqlc`
(`go install github.com/sqlc-dev/sqlc/cmd/sqlc@latest`).

## Jalankan lokal

```bash
cp .env.example .env          # isi POSTGRES_PASSWORD, DATABASE_URL, JWT_SECRET (>= 32 byte), ADMIN_EMAIL, ADMIN_PASSWORD
docker compose up -d postgres # kalau port 5432 bentrok, set POSTGRES_PORT=5434 dan sesuaikan DATABASE_URL
make migrate-up
make seed-dev                 # data contoh; semua akun dev berpassword Password123!
go run ./cmd/api              # http://localhost:3030/healthz
```

Data dev = salinan dummy app Android (`SisaGuna/`): `budi.santoso@gmail.com` (pembeli sekaligus pemilik Dapur Budi),
`owner-m1@example.com` (Warung Bu Sari), `wati@example.com` (toko `pending`), admin dari `ADMIN_EMAIL`; password `Password123!`.
Request contoh ada di `apiendpoints/*.http`.

## Data dummy (seed)

`cmd/seed` adalah program kecil (bukan bagian API) yang mengisi database. Dua mode:

| Mode | Isi | Dipakai untuk |
| --- | --- | --- |
| `-env prod` | akun admin dari `ADMIN_EMAIL`/`ADMIN_PASSWORD` + 2 voucher umum | wajib sekali di server sungguhan |
| `-env dev` | salinan dummy app Android: 13 toko, 39 listing, 59 ulasan, 4 voucher, pesanan, chat, notifikasi, 45 foto | uji lokal dan VPS uji |

Isinya ada di `cmd/seed/data/*.json` (dihasilkan otomatis dari kode Kotlin oleh `scripts/extract_android_seed.py`) dan foto di `cmd/seed/img/`;
keduanya ikut di-embed ke binary. Mengambil ulang dari app Android: `make sync-android-seed`. Menjalankan seed berulang aman:
waktu pickup, umur pesanan, dan stok dikembalikan ke keadaan awal tanpa menghapus data lain.

| Tujuan | Lokal (Mac) | VPS uji (Docker) |
| --- | --- | --- |
| Segarkan dummy (jendela pickup + stok), tanpa hapus data | `make seed-dev` | `deploy/demo.sh refresh` |
| Mulai dari nol (hapus semua data) | `make reset-dev` | `deploy/demo.sh reset` (minta konfirmasi, menghapus database; foto tidak dihapus) |

`make reset-dev` hanya untuk lokal (menghapus volume Postgres compose dev), jangan dijalankan di VPS. Di VPS pakai `deploy/demo.sh`
dari root repo; skrip itu menyalakan `ALLOW_DEMO_SEED=true` hanya untuk satu perintah, karena seed `dev` otomatis ditolak
kalau `APP_ENV=production`. **Seed dev membuat akun berpassword umum `Password123!`, jadi hanya untuk VPS uji.**
Data dummy kedaluwarsa dalam hitungan jam (jendela pickup 1–10 jam): ulangi `refresh` sebelum sesi uji.

## Tes manual (Postman)

Panduan lengkap langkah demi langkah ada di [`apiendpoints/README.md`](./apiendpoints/README.md). Singkatnya:
`make reset-dev` → `go run ./cmd/api` → import `apiendpoints/SisaGuna.postman_collection.json` → jalankan folder
**★ Alur Cepat MVP**.

## Test

```bash
gofmt -l . && go vet ./...
go test ./...                 # integrasi memakai testcontainers (Docker harus jalan)
```

## Deploy ke VPS

Panduan lengkap (persiapan VPS, `.env`, verifikasi, backup, pemulihan): [`deploy/README.md`](./deploy/README.md). Ringkasnya:

```bash
cp .env.example .env          # isi DOMAIN, PUBLIC_BASE_URL, POSTGRES_*, JWT_SECRET, ADMIN_*, R2_* (password acak: openssl rand -hex 24)
docker compose -f deploy/docker-compose.prod.yml --env-file .env up -d --build
docker compose -f deploy/docker-compose.prod.yml --env-file .env run --rm --entrypoint /seed api -env prod   # admin + voucher awal
```

Postgres di compose produksi tidak punya `ports`. Migrasi berjalan otomatis lewat service `migrate` sebelum `api` start.
Backup harian dan salinan foto ke R2 dijelaskan di `deploy/README.md`. Developer mobile: [`apiendpoints/MOBILE.md`](./apiendpoints/MOBILE.md).

## Status pengujian dan risiko sebelum ke VPS

**Sudah diuji di mesin lokal** (Mac, Docker/OrbStack):

| Yang diuji | Cara | Hasil |
| --- | --- | --- |
| Logika bisnis, database, API | `go test ./...` dengan Postgres+PostGIS asli (testcontainers), termasuk balapan 10 pembeli rebutan stok, kunci kode pickup, WebSocket, upload foto/video, sinkron ke server S3 tiruan, paritas aturan Kotlin, paritas seed dengan dummy Android | lulus |
| Seluruh endpoint lewat HTTP | Postman collection (191 request) dijalankan dengan Newman ke server dan database lokal. Request upload butuh file asli, jadi lewat Newman penuh 14 request upload dan turunannya gagal (bukan bug); upload diuji lewat `go test` dan curl, dan folder "Alur Cepat MVP" lulus penuh | 177 lulus, 14 gagal karena tanpa file upload |
| Stack produksi (`deploy/docker-compose.prod.yml`) | dibangun dan dijalankan penuh di Mac: Caddy HTTPS, API, Postgres tanpa port publik, migrasi otomatis, folder media, foto publik lewat Caddy, folder privat tidak terjangkau, WebSocket lewat TLS, backup, uji pulih, restore, `deploy/demo.sh reset/refresh` | lulus |

**Belum diuji, risikonya ada di sini:**

| # | Risiko | Akibat kalau terjadi | Yang perlu dilakukan |
| --- | --- | --- | --- |
| 1 | Belum pernah jalan di VPS sungguhan: sertifikat Let's Encrypt asli, DNS, firewall | HTTPS gagal terbit, app tidak bisa konek | Ikuti `deploy/README.md`, cek `https://domain/healthz`, buka port 80 dan 443 |
| 2 | Cloudflare R2 sungguhan belum dicoba (hanya server S3 tiruan) | Foto/backup tidak tersalin ke R2 | Isi `R2_*`, jalankan `backup now`, cek objek muncul di bucket |
| 3 | `PARTNERS_MODE=dummy` membuat pembayaran non-tunai selalu "sukses" tanpa uang masuk dan kurir hanya hitungan rumus | Penjual melihat pesanan `paid` yang tidak pernah dibayar | Nyalakan hanya di VPS uji. Produksi sungguhan: `off` (default) sampai gateway dan kurir terhubung |
| 4 | Seed dummy membuat akun berpassword umum `Password123!` | Siapa pun bisa login | Jangan jalankan `deploy/demo.sh` di VPS yang dipakai pengguna sungguhan |
| 5 | App Android belum diintegrasikan dengan API ini (belum ada kode jaringan) | Perbedaan perilaku baru ketahuan saat integrasi | Baca `apiendpoints/ANDROID-SYNC.md`: tombol "Sudah saya ambil" pembeli, ulasan tanpa pesanan, batas pembatalan, form daftar mitra |
| 6 | Tiket WebSocket disimpan di memori satu proses | Lebih dari satu instance `api` membuat koneksi WebSocket gagal acak | Jalankan hanya satu container `api` |
| 7 | `/home` mengurutkan sampai 500 listing aktif per tab di memori (`listing/sort.go`) | Lambat dan "terdekat" tidak akurat kalau listing aktif > 500 | Cukup untuk uji; pindahkan ke SQL bila katalog membesar |
| 8 | Backup tanpa R2 tersimpan di disk yang sama dengan database | Disk rusak = data dan backup hilang | Isi R2 sebelum dipakai pengguna sungguhan; jalankan `backup restore-test` tiap bulan |
| 9 | Foto dummy dari Wikimedia, atribusi tidak lengkap | Masalah lisensi bila dipakai publik | Ganti dengan foto milik sendiri sebelum rilis; hanya untuk demo |
| 10 | Belum diuji beban (banyak pengguna bersamaan) dan belum ada monitoring selain `/healthz` | Masalah performa baru terlihat setelah dipakai | Daftarkan `/healthz` di UptimeRobot, pantau `docker compose logs -f api` |
| 11 | Kurir dan payment gateway sungguhan belum ada (tiruan) | Pembayaran non-tunai tidak memindahkan uang | Fase berikutnya |
| 12 | Push FCM dan email SMTP lulus tes dengan server tiruan, belum pernah dicoba dengan kredensial Firebase dan SMTP sungguhan | Push/email tidak sampai di HP | Isi `FCM_CREDENTIALS_FILE` dan `SMTP_*` di VPS, minta dev Android mendaftarkan token lewat `PUT /me/devices`, matikan app lalu picu notifikasi |
| 13 | Cek sesi (user dihapus, password diganti, admin dicabut) di-cache 10 detik | Akses masih bisa jalan sampai 10 detik setelah dicabut | Cukup; turunkan `sessionCacheTTL` di `internal/app/app.go` bila perlu |
| 14 | File biner `seed` 18 MB pernah ter-commit dan masih ada di riwayat git | Clone penuh tetap mengunduhnya | Bersihkan riwayat (`git filter-repo`) hanya jika ukuran clone mengganggu; itu menulis ulang riwayat |

Urutan aman menuju VPS: (1) VPS uji dengan `PARTNERS_MODE=dummy` dan `deploy/demo.sh reset`, (2) integrasi dan uji dengan app Android,
(3) baru VPS/konfigurasi produksi: `PARTNERS_MODE` kosong, tanpa seed dev, `R2_*` terisi, `backup restore-test` hijau.
