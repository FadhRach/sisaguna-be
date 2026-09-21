# SisaGuna Backend

REST API untuk SisaGuna — marketplace redistribusi makanan berbasis lokasi. Ditulis
di Go, deploy serverless ke Vercel, database Postgres (Supabase + PostGIS).

Detail arsitektur, batasan kritis, dan skema database lengkap ada di
[`CLAUDE.md`](./CLAUDE.md). README ini fokus ke cara menjalankan & test proyeknya.

## Prasyarat

- Go 1.27+ (cek `go version`)
- Postgres (lokal, atau langsung pakai Supabase — lihat [Local vs Production Database](#local-vs-production-database))
- [`golang-migrate` CLI](https://github.com/golang-migrate/migrate) untuk menjalankan migrasi
- (Opsional) [Vercel CLI](https://vercel.com/docs/cli) kalau mau coba `vercel dev` / deploy
- (Opsional) Extension **REST Client** di VS Code untuk test API lewat folder `apiendpoints/`

## Setup Awal

```bash
git clone <repo-ini>
cd SisaGuna-be
go mod download
cp .env.example .env
```

Isi `.env` — minimal `DATABASE_URL`, `MIGRATION_DATABASE_URL`, dan `JWT_SECRET`
wajib diisi (lihat tabel di CLAUDE.md untuk penjelasan tiap variabel). Kalau pakai
Postgres lokal, isian paling sederhana:

```
DATABASE_URL=postgresql://postgres@localhost:5432/sisaguna?sslmode=disable
MIGRATION_DATABASE_URL=postgresql://postgres@localhost:5432/sisaguna?sslmode=disable
JWT_SECRET=ganti-dengan-random-string-yang-panjang
```

## Menjalankan Migrasi

```bash
createdb sisaguna   # sekali saja, kalau database belum ada
migrate -database "$MIGRATION_DATABASE_URL" -path database/migrations up
```

Tabel `listings` butuh extension **PostGIS**. Di macOS: `brew install postgis`
lalu `CREATE EXTENSION IF NOT EXISTS postgis;` akan jalan otomatis lewat migrasi.
Kalau belum sempat install PostGIS, migrasi `users`/`refresh_tokens` tetap bisa
jalan duluan (nomor `000001`/`000002`) — cukup untuk fitur auth & user yang
sudah dibangun.

## Menjalankan Secara Lokal

```bash
go run ./cmd/main.go
# atau, untuk meniru environment serverless Vercel:
vercel dev
```

Server jalan di `http://localhost:$PORT` (default `3030`), semua endpoint di
bawah prefix `/api/v1`.

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

## Deploy ke Vercel

```bash
vercel                 # link project & deploy preview
vercel env add ...     # set DATABASE_URL, JWT_SECRET, dst di environment Vercel
vercel --prod           # deploy production
```

Vercel otomatis mendeteksi tiap file di `/api` sebagai satu function terpisah
(bukan mode "Go Framework Preset" yang butuh server persisten) — lihat bagian
Architecture di CLAUDE.md untuk detail.

## Local vs Production Database

Boleh, dan disarankan: **Postgres lokal untuk development, Supabase Postgres untuk
production.** Kode aplikasi cuma baca `DATABASE_URL` dari environment variable —
tidak ada kode yang hardcode ke Supabase, jadi tinggal ganti connection string per
environment:

| Environment | `DATABASE_URL` mengarah ke |
| --- | --- |
| Lokal (`.env`) | Postgres lokal, port 5432 biasa |
| Production (env var Vercel) | Supabase Supavisor pooler, port 6543 |

Beberapa catatan:

- Konfigurasi `PreferSimpleProtocol`/`PrepareStmt:false` di `internal/db` (wajib
  untuk Supavisor transaction mode) tidak mengganggu Postgres biasa — sudah
  diverifikasi lewat test concurrent request saat awal development.
- File migrasi di `database/migrations` sama persis dipakai untuk lokal maupun
  Supabase, asal extension yang sama (`pgcrypto`, `postgis`) terpasang di keduanya.
- Dev di lokal jauh lebih cepat: tidak kena latency jaringan ke Supabase, tidak
  kena limit koneksi free tier, dan tidak kena auto-pause Supabase setelah 7 hari
  idle.
- Sebelum deploy, tetap jalankan migrasi yang sama ke Supabase (`MIGRATION_DATABASE_URL`
  di environment Vercel diarahkan ke koneksi direct/session mode Supabase, port 5432).
