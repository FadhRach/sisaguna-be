# SisaGuna Backend

REST API untuk SisaGuna — marketplace redistribusi makanan berbasis lokasi. Ditulis
di Go (Fiber), deploy sebagai container Docker ke server AWS, database Postgres +
PostGIS self-hosted.

Detail arsitektur, batasan kritis, dan skema database lengkap ada di
[`CLAUDE.md`](./CLAUDE.md). README ini fokus ke cara menjalankan & test proyeknya.

## Prasyarat

- Docker + Docker Compose
- Go 1.27+ (opsional, cuma dibutuhkan buat jalan lokal tanpa Docker)
- [`golang-migrate` CLI](https://github.com/golang-migrate/migrate) untuk menjalankan migrasi
- (Opsional) Extension **REST Client** di VS Code untuk test API lewat folder `apiendpoints/`

## Setup Awal

```bash
git clone <repo-ini>
cd SisaGuna-be
cp .env.example .env
```

Isi `.env` — minimal `POSTGRES_USER`, `POSTGRES_PASSWORD`, `POSTGRES_DB`, `DATABASE_URL`,
dan `JWT_SECRET` wajib diisi (lihat tabel di CLAUDE.md untuk penjelasan tiap variabel).

## Menjalankan dengan Docker (direkomendasikan)

```bash
docker compose up --build
```

Ini menjalankan dua service: `app` (backend Go) dan `postgres` (image `postgis/postgis`,
extension PostGIS sudah bawaan, tidak perlu install manual). Server jalan di
`http://localhost:$PORT` (default `3030`), semua endpoint di bawah prefix `/api/v1`.

Kalau kamu sudah punya Postgres lokal sendiri yang jalan di port 5432, ganti port
mapping service `postgres` di `docker-compose.yml` (mis. `"5433:5432"`) supaya tidak
bentrok.

## Menjalankan Migrasi

Postgres di docker-compose port-nya di-map ke host, jadi `migrate` CLI dari host bisa
langsung connect:

```bash
migrate -database "$DATABASE_URL" -path database/migrations up
```

## Menjalankan Secara Lokal (tanpa Docker)

Butuh Postgres + PostGIS sendiri yang sudah jalan, lalu:

```bash
go run ./main.go
```

## Testing API

Folder [`apiendpoints/`](./apiendpoints) berisi file `.http` siap pakai (extension
REST Client di VS Code) untuk semua endpoint yang sudah dibangun — register,
login, refresh, logout, get/update profil. Lihat `apiendpoints/README.md` untuk
detail cara pakainya.

Untuk verifikasi cepat lewat terminal sebelum commit:

```bash
gofmt -l .
go vet ./...
go build ./...
go test ./...
```

## Deploy ke AWS

Mekanisme orkestrasi (ECS, EC2, Lightsail, dst) belum ditentukan di sesi ini — bagian
ini sengaja digeneralisasi, bukan diasumsikan. Alur dasarnya:

```bash
docker build -t sisaguna-be .
# push ke registry (ECR atau lainnya), lalu jalankan di server AWS yang sudah ada
docker run -p 3030:3030 --env-file .env sisaguna-be
```

Pastikan `DATABASE_URL` di environment production diarahkan ke Postgres yang benar
di infrastruktur AWS kamu (bisa container terpisah, bisa RDS, dll — tidak diasumsikan
di sini).

## Local vs Production Database

`DATABASE_URL` adalah satu-satunya env var yang dibaca aplikasi untuk koneksi DB —
tidak ada kode yang hardcode ke provider tertentu. Ganti connection string-nya sesuai
environment, tidak ada perubahan kode yang dibutuhkan:

| Environment | `DATABASE_URL` mengarah ke |
| --- | --- |
| Docker Compose (dev) | Service `postgres` di network internal (`postgres:5432`) — diatur otomatis lewat `docker-compose.yml` |
| Lokal tanpa Docker | Postgres yang kamu jalankan sendiri, biasanya `localhost:5432` |
| Production (AWS) | Tergantung setup — container Postgres lain, RDS, dst |

## Follow-up yang belum diputuskan

Dicatat di sini biar tidak lupa, bukan bagian dari setup yang sudah selesai:

- **Mekanisme pemicu cron** (`/internal/cron/*`, lihat CLAUDE.md batasan kritis #6) —
  belum ada cron job berjalan di sisi AWS (dulu pakai Vercel Cron). Kandidat: sidecar
  container penjadwal (mis. Ofelia) yang `curl` endpoint-nya, AWS EventBridge, atau
  ticker di dalam aplikasi sendiri.
- **Penyimpanan foto listing** — dulu pakai Vercel Blob (`BLOB_READ_WRITE_TOKEN`), sudah
  tidak relevan di AWS. Kemungkinan besar S3, belum diputuskan.
