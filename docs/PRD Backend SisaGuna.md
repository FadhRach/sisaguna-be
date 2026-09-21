# PRD Backend SisaGuna

2026-09-20 · @Someone

Backend ini bisa jalan di Vercel, tapi hanya realistis dengan plan Pro karena kebutuhan cron per menit, dan angka emisi 2.324 di dek asli kemungkinan salah satuan.

## Ringkasan & Batasan MVP

Fokus backend adalah Tier 1 (konsumsi manusia) saja. Tier 2 (pakan ternak) dan Tier 3 (kompos) cukup disiapkan sebagai kategori di skema, belum butuh alur bisnis sendiri.

**Sengaja tidak dibangun dulu** (sesuai keputusan desain di dek):

- Logistik pengantaran (delivery fleet)
- Pembayaran di dalam aplikasi
- Deteksi otomatis berbasis kamera untuk verifikasi kelayakan makanan
- Gamifikasi, poin, badge
- Dukungan multi kota / multi bahasa

**Target operasional yang jadi acuan desain teknis:**

| Metrik | Target | Konsekuensi teknis |
| --- | --- | --- |
| Waktu posting mitra | < 30 detik | Form create listing wajib field minimal, tanpa langkah verifikasi manual di jalur kritis |
| Tingkat pengambilan berhasil | > 75% dari total posting | Status listing harus akurat real-time, bukan basi karena nunggu cron |
| Mitra aktif tahun pertama | 20 mitra, 1 area (kampus) | Query feed tidak perlu di-partition per kota dulu, tapi index GPS tetap wajib dari awal |

Batasan ini yang jadi dasar semua keputusan di bagian Risiko Arsitektur dan Pipeline di bawah.

### Fase 1: Prototype Cost-Free (Vercel Hobby)

Boleh mulai dari sini dulu. Cron 1x sehari di Hobby aman dipakai karena correctness feed sudah dijamin di level query (`pickup_deadline > now()`), bukan di cron — jadi "potong performa" di sini gak ngorbanin apa yang dilihat user, cuma bikin laporan status di database telat update sampai 1 hari.

**Setup gratis yang dipakai:**

- Vercel Hobby untuk backend Go
- Supabase free tier untuk Postgres (Supavisor pooler mode transaksi + PostGIS didukung, auto-pause kalau nganggur 7 hari)
- Vercel Blob atau Cloudflare R2 free tier untuk foto listing

**Fitur wajib vs ditunda:**

| Fitur | Fase 1 (Prototype) | Fase 2 |
| --- | --- | --- |
| Auth (register, login) | Ya, versi sederhana | Refresh token flow lengkap |
| Listings: create, feed, detail, cancel | Ya, wajib (core loop) | - |
| Reservations: create, confirm, complete | Ya, wajib (core loop) | Penanganan no\_show lebih detail |
| Reviews dua arah | Ya, wajib — ini yang bikin orang percaya ("kepercayaan" prioritas #2 di dek) | - |
| Cron expire-listings | Ya, cukup 1x/hari | Naik ke per menit kalau upgrade Pro |
| Impact / kalkulasi CO2 | Ditunda | Aktifkan setelah angka konversi diverifikasi (lihat Risiko Arsitektur poin 4) |
| Notifications otomatis (reminder) | Ditunda | Butuh Pro plan untuk presisi menit |

Tabel `impact_records` dan `notifications` tetap dibuat dari awal walau belum dipakai. Bikin tabel kosong itu gratis, dan ini menghindari migrasi database yang nyusahin nanti.

## Risiko Arsitektur: Go + Vercel + Pipeline

### 1. Cron di Vercel Hobby praktis tidak berguna untuk sweep kedaluwarsa

&#91;Certain\] Di plan Hobby, cron cuma boleh jalan maksimal 1 kali per hari, dan waktu eksekusinya meleset sampai hampir 1 jam dari jadwal ([sumber](https://vercel.com/docs/cron-jobs/usage-and-pricing)). Plan Pro baru bisa per menit.

Listing SisaGuna punya batas ambil dalam hitungan jam, bukan hari. Kalau job "tandai expired" cuma jalan sekali sehari, feed bakal penuh listing basi yang keliatan masih "available" sepanjang hari, langsung merusak metrik tingkat pengambilan berhasil di atas.

**Keputusan:** jangan jadikan cron sebagai satu-satunya sumber kebenaran status expired. Query feed dan detail listing wajib filter `WHERE status = 'available' AND pickup_deadline > now()` di level aplikasi, apa pun kondisi cron-nya. Cron (harian di Hobby, per menit kalau upgrade ke Pro) cukup jadi housekeeping: mengubah status jadi `expired` di database untuk kebutuhan laporan dan mematikan reminder notifikasi.

### 2. WebSocket native Vercel baru beta dan tidak cocok jadi tulang punggung feed real-time

&#91;Certain\] Vercel baru merilis dukungan WebSocket native sebagai public beta Juni 2026, dan koneksi terputus otomatis sekitar 5 menit lalu harus reconnect dari sisi client ([sumber](https://dev.classmethod.jp/en/articles/vercel-functions-websocket-public-beta-verification/)). Ini bukan koneksi persisten seperti server tradisional.

**Keputusan:** untuk discovery feed, pakai polling dengan cache pendek (misalnya 15-30 detik) di endpoint, bukan WebSocket. WebSocket beta boleh dipakai belakangan untuk hal ringan seperti notifikasi status reservasi, dengan logic reconnect wajib di Kotlin dan SwiftUI.

### 3. Koneksi database bisa habis kalau pakai driver Postgres biasa

&#91;Likely\] Setiap cold start function Vercel bisa membuka koneksi database baru. Di beban tinggi, Postgres biasa (max \~100 koneksi) gampang kehabisan slot koneksi karena banyak instance function jalan bersamaan.

**Keputusan:** pakai Postgres yang punya connection pooling bawaan untuk serverless (Neon atau Supabase, keduanya bisa dipasang sebagai Vercel integration), bukan Postgres self-hosted biasa tanpa pooler.

Sudah diputuskan pakai Supabase: connect lewat Supavisor pooler mode transaksi (port 6543, bukan direct port 5432), dan matikan prepared statement di driver Go (`pgx` perlu `default_query_exec_mode=simple_protocol` atau setara) karena mode transaksi Supavisor tidak mendukung prepared statement. Kalau ini kelewat, errornya baru muncul saat ada lebih dari satu request bersamaan, bukan saat testing sendirian — gampang lolos dari pengecekan awal.

### 4. Angka "2,324 kg CO2 per ton" di dek kemungkinan salah satuan

&#91;Likely\] Ini yang paling perlu dicek ke tim sebelum di-hardcode ke kalkulasi dampak. Faktor emisi sampah makanan yang dipublikasikan lembaga lingkungan (NSW EPA, UK DEFRA, EPA AS) umumnya ada di kisaran 0,6 sampai 2,5 **ton** CO2e per **ton** sampah makanan yang masuk TPA ([contoh sumber](https://www.epa.nsw.gov.au/sites/default/files/24p4522-emissions-impacts-from-landfilling-food-waste.pdf), menyebut "at least 2.1 tonnes CO2-e per tonne of food"). Artinya rasio kg-ke-kg-nya kira-kira 1:1 sampai 1:2,5, bukan 1:0,002 seperti yang tertulis di dek ("2,324 kg CO2 per satu **ton**").

Angka di dek kemungkinan aslinya bermaksud sekitar 2,324 kg CO2e per **kg** sampah makanan (atau setara 2,324 ton CO2e per ton), lalu salah tulis satuan. Kalau formula `co2_avoided_kg = weight_kg * 0.002324` ini langsung dipakai di `impact_records`, dampak yang ditampilkan ke mitra dan penerima bakal kurang sekitar 1000 kali dari yang seharusnya. Ini fatal karena dek sendiri bilang fitur ini akan jadi "bukti dampak yang bisa diaudit", jadi angkanya harus benar dari hari pertama, bukan hal yang bisa dibenerin belakangan setelah dipublikasikan ke mitra.

**Keputusan:** konfirmasi ke tim riset angka aslinya dari sumber Bappenas yang dikutip, jangan pakai 0,002324 sebagai konstanta konversi tanpa verifikasi ulang.

## Skema Database

Postgres dengan ekstensi PostGIS wajib dipasang, karena discovery feed butuh query "radius X km dari titik GPS user" yang jauh lebih cepat lewat index `GIST` dibanding hitung manual di kode Go.

### users

| Kolom | Tipe | Keterangan |
| --- | --- | --- |
| id | uuid | primary key |
| name | text |  |
| email | text | unique |
| phone | text |  |
| password\_hash | text |  |
| roles | text\[\] | `mitra_usaha`, `rumah_tangga`, `penerima`, `peternak`, `pengelola_kompos`, `admin`. Satu user bisa punya lebih dari satu role |
| business\_name | text | nullable, untuk role mitra\_usaha |
| avg\_rating | numeric(2,1) | cache, di-update dari tabel reviews |
| created\_at | timestamptz |  |

### listings

| Kolom | Tipe | Keterangan |
| --- | --- | --- |
| id | uuid | primary key |
| mitra\_id | uuid | FK ke users |
| title | text |  |
| description | text | nullable |
| tier | enum | `konsumsi_manusia`, `pakan_ternak`, `kompos` — sesuai tier di dek |
| quantity | numeric |  |
| unit | text | porsi, kg, dus, dst |
| price | numeric | default 0 untuk gratis |
| photo\_url | text | wajib diisi untuk tier konsumsi\_manusia |
| pickup\_location | geography(Point,4326) | kolom PostGIS, index GIST |
| pickup\_address | text |  |
| ready\_at | timestamptz |  |
| pickup\_deadline | timestamptz | batas ambil |
| status | enum | `available`, `reserved`, `completed`, `expired`, `cancelled` |
| created\_at | timestamptz |  |

### reservations

| Kolom | Tipe | Keterangan |
| --- | --- | --- |
| id | uuid | primary key |
| listing\_id | uuid | FK ke listings |
| penerima\_id | uuid | FK ke users |
| pickup\_slot\_start | timestamptz |  |
| pickup\_slot\_end | timestamptz |  |
| status | enum | `pending`, `confirmed`, `completed`, `no_show`, `cancelled` |
| created\_at | timestamptz |  |

### reviews

| Kolom | Tipe | Keterangan |
| --- | --- | --- |
| id | uuid | primary key |
| reservation\_id | uuid | FK ke reservations |
| reviewer\_id | uuid | FK ke users |
| reviewee\_id | uuid | FK ke users |
| direction | enum | `mitra_ke_penerima`, `penerima_ke_mitra` — penilaian dua arah sesuai dek |
| rating | smallint | 1 sampai 5 |
| comment | text | nullable |
| created\_at | timestamptz | unique constraint pada (reservation\_id, direction) supaya tidak double review |

### impact\_records

| Kolom | Tipe | Keterangan |
| --- | --- | --- |
| id | uuid | primary key |
| reservation\_id | uuid | FK ke reservations, unique |
| weight\_kg | numeric | diisi mitra saat konfirmasi pickup |
| co2\_avoided\_kg | numeric | weight\_kg dikali konstanta konversi — lihat catatan di Risiko Arsitektur poin 4 sebelum hardcode nilainya |
| verified\_at | timestamptz |  |

### notifications

| Kolom | Tipe | Keterangan |
| --- | --- | --- |
| id | uuid | primary key |
| user\_id | uuid | FK ke users |
| type | enum | `listing_baru_dekat`, `reminder_batas_ambil`, `reservasi_dikonfirmasi`, `review_baru` |
| payload | jsonb | data kontekstual, misal listing\_id |
| read\_at | timestamptz | nullable |
| created\_at | timestamptz |  |

## Daftar API Endpoint

Semua endpoint diawali `/api/v1` supaya versi bisa berubah tanpa merusak client Android/iOS lama.

### Auth

| Method | Path | Deskripsi |
| --- | --- | --- |
| POST | /auth/register | Daftar user baru, isi roles |
| POST | /auth/login | Login, kembalikan access + refresh token |
| POST | /auth/refresh | Perpanjang access token |
| POST | /auth/logout | Invalidasi refresh token |

### Users

| Method | Path | Deskripsi |
| --- | --- | --- |
| GET | /users/me | Profil user yang login |
| PATCH | /users/me | Update profil |
| GET | /users/:id | Profil publik (untuk lihat rating mitra sebelum pickup) |
| GET | /users/:id/reviews | Riwayat review user tersebut |

### Listings (Posting)

| Method | Path | Deskripsi |
| --- | --- | --- |
| POST | /listings | Mitra buat posting baru. Payload minimal supaya target di bawah 30 detik tercapai |
| GET | /listings/:id | Detail satu listing |
| PATCH | /listings/:id | Update atau batalkan listing |
| GET | /listings/feed | Discovery feed: query param `lat`, `lng`, `radius_km`, `tier`, `status`. Filter `pickup_deadline > now()` wajib di query ini, terlepas dari status cron |

### Reservations

| Method | Path | Deskripsi |
| --- | --- | --- |
| POST | /listings/:id/reservations | Penerima booking slot pengambilan |
| PATCH | /reservations/:id | Mitra konfirmasi/tolak, atau tandai selesai (isi weight\_kg di sini, trigger kalkulasi dampak) |
| GET | /reservations | List reservasi milik user (filter by status) |

### Reviews

| Method | Path | Deskripsi |
| --- | --- | --- |
| POST | /reservations/:id/reviews | Kirim review, arah otomatis dari role pengirim |

### Impact

| Method | Path | Deskripsi |
| --- | --- | --- |
| GET | /impact/summary | Total kg terselamatkan dan CO2 dicegah untuk user yang login |
| GET | /impact/global | Statistik agregat untuk landing page / laporan |

### Notifications

| Method | Path | Deskripsi |
| --- | --- | --- |
| GET | /notifications | List notifikasi user, paginated |
| PATCH | /notifications/:id/read | Tandai sudah dibaca |

### Internal (dipanggil cron, bukan oleh client)

| Method | Path | Deskripsi |
| --- | --- | --- |
| POST | /internal/cron/expire-listings | Sweep listings yang lewat pickup\_deadline, ubah status jadi expired |
| POST | /internal/cron/pickup-reminders | Kirim reminder ke reservasi yang mendekati pickup\_slot\_end |

Endpoint internal ini harus divalidasi pakai header secret (`CRON_SECRET`), bukan lewat auth token user biasa.

## Pipeline & Background Job

| Pipeline | Trigger | Fungsi | Catatan |
| --- | --- | --- | --- |
| Sweep kedaluwarsa listing | Cron: harian di Hobby, per menit kalau Pro | Ubah status listing jadi expired di database untuk laporan | Bukan sumber kebenaran real-time — feed tetap filter pickup\_deadline di level query (lihat Risiko Arsitektur) |
| Reminder batas ambil | Cron per menit (butuh Pro) | Push notification ke penerima menjelang pickup\_slot\_end | Kalau tetap di Hobby, fitur ini realistisnya jadi reminder H-1 hari, bukan H-30 menit |
| Kalkulasi dampak | Trigger saat reservation di-PATCH status jadi completed | Hitung co2\_avoided\_kg dari weight\_kg, tulis ke impact\_records | Jangan hardcode konstanta konversi sebelum angka di dek diverifikasi |
| Upload foto listing | Client minta presigned URL dari backend, upload langsung ke object storage | Hindari proxy file besar lewat Function Vercel (body request function ada batas ukuran) | Pakai Vercel Blob atau storage S3-compatible seperti Cloudflare R2 / Supabase Storage |
| Sinkron rating | Trigger saat review baru dibuat | Update kolom avg\_rating di users | Hindari hitung ulang AVG() penuh tiap kali feed di-load |

Urutan implementasi yang disarankan: bangun dulu jalur create listing → feed → reservation → complete (jalur inti yang divalidasi di dek lewat wawancara lapangan), baru tambahkan reminder dan sinkron rating.

## Strategi Konsistensi Data Lintas Android & iOS

"Data seragam kemanapun" itu masalah kontrak API, bukan cuma masalah database. Kalau Kotlin dan SwiftUI masing-masing nulis parsing sendiri dari dokumentasi endpoint yang ditulis manual, dua tim gampang beda interpretasi.

**Rekomendasi konkret:**

1. **Kontrak API-first pakai OpenAPI 3.0.** Definisikan semua endpoint di atas sebagai spec OpenAPI dulu, baru backend Go dan kedua client generate kode dari spec yang sama (`swift-openapi-generator` untuk SwiftUI, `openapi-generator` dengan target Kotlin/Retrofit untuk Android). Spec ini juga jadi satu-satunya sumber kebenaran nama field, bukan dokumen terpisah yang gampang basi.
2. **Format angka desimal sebagai string, bukan float JSON.** Field seperti quantity, price, weight\_kg, co2\_avoided\_kg dikirim sebagai string desimal. Kotlin (Double) dan Swift (Double) bisa membulatkan float dengan cara berbeda di kasus tertentu, dan untuk angka dampak yang "bisa diaudit" itu risiko yang tidak perlu diambil.
3. **Semua timestamp UTC ISO 8601, konversi ke waktu lokal di client.** Indonesia punya 3 zona waktu (WIB/WITA/WIT); jangan simpan atau kirim waktu lokal dari backend.
4. **Idempotency-Key header wajib di POST /listings dan POST /reservations.** Target posting di bawah 30 detik biasanya berarti UI yang agresif retry kalau koneksi lambat — tanpa idempotency key, retry bisa bikin listing atau reservasi dobel.
5. **Enum status (listing, reservation, notification) didefinisikan sekali di spec OpenAPI**, dipakai apa adanya oleh kedua client, bukan di-mapping ulang manual ke enum lokal masing-masing platform.
