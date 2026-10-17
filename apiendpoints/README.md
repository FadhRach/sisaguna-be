# Panduan Tes Manual API SisaGuna

Folder ini berisi semua yang dibutuhkan untuk mencoba API secara manual:

| File | Isi |
| --- | --- |
| `SisaGuna.postman_collection.json` | Koleksi Postman (108 request, berurutan per peran, token tersimpan otomatis) |
| `auth.http`, `me.http`, `catalog.http`, `checkout.http`, `mitra.http`, `admin.http` | Versi ringkas untuk VS Code REST Client (`humao.rest-client`) |

## 1. Menyalakan backend di Mac (sekali jalan)

```bash
# 0. Pastikan Docker/OrbStack jalan: `docker info` tidak error.
cd SisaGuna-be

# 1. Siapkan .env (sekali saja). Isi JWT_SECRET (>= 32 byte), ADMIN_EMAIL, ADMIN_PASSWORD, password Postgres.
cp .env.example .env
#    JWT_SECRET:  openssl rand -base64 48
#    Kalau port 5432 dipakai Postgres lain di Mac, isi POSTGRES_PORT=5434 dan samakan port di DATABASE_URL.

# 2. Database bersih + migrasi + data contoh (hapus semua data lokal lalu isi ulang)
make reset-dev

# 3. Jalankan server
go run ./cmd/api

# 4. Cek (di terminal lain)
curl localhost:3030/healthz        # {"data":{"status":"ok"}}
```

**Penting:** data contoh memakai jam relatif terhadap saat seed dijalankan (jendela pickup 2 sampai 10 jam, makanan siap santap
batas aman 4 jam). Kalau sudah lebih dari beberapa jam sejak `make reset-dev`, listing contoh sudah kedaluwarsa dan feed kosong.
Jalankan lagi `make reset-dev` dan restart server kapan pun mau mulai dari awal.

## 2. Akun dan data contoh

Semua password akun contoh: `Password123!`. Admin memakai `ADMIN_EMAIL` dan `ADMIN_PASSWORD` dari `.env`.

Data contoh = **salinan persis dummy app Android** (`SisaGuna/` di repo ini): 13 toko, 39 listing, 59 ulasan, 4 voucher, 8 notifikasi,
9 pesan chat, 4 pesanan Budi + 4 pesanan masuk ke Dapur Budi, foto asli 45 gambar. Disalin otomatis dari kode Kotlin oleh
`scripts/extract_android_seed.py` (`make sync-android-seed` mengulangnya kalau tim Android mengubah dummy).

| Akun | Peran | Catatan |
| --- | --- | --- |
| `budi.santoso@gmail.com` | Pembeli **dan** pemilik toko **Dapur Budi** (m0) | Akun utama app: 2 alamat (Rumah Tangerang, Kantor Kebon Jeruk), 4 pesanan, 4 toko favorit, voucher `HEMAT5K` sudah diklaim, GoPay tertaut, impact 34 porsi |
| `owner-m1@example.com` | Mitra: **Warung Bu Sari** (m1, Kemang) | Dipakai sebagai peran `seller` di Postman. Pemilik toko lain: `owner-m2` … `owner-m12@example.com` |
| `wati@example.com` | Mitra: **Dapur Nusantara** (**pending**) | Khusus dev, untuk tes persetujuan admin (tidak ada di app Android) |
| Penulis ulasan/chat (mis. `dinda-p@example.com`, `clara-t@example.com`) | Pembeli | Satu akun per nama di dummy |
| `ADMIN_EMAIL` | Admin | Menyetujui/menolak toko, menangani komplain, mengelola voucher |

Voucher (dari dummy): `HEMAT5K` (Rp5.000, min Rp15.000, sudah diklaim Budi), `SELAMAT20` (20% maks Rp8.000), `PERTAMA` (50% maks Rp10.000, min Rp10.000),
`PAKAN10` (10% maks Rp5.000, min Rp10.000).

Lokasi: toko memakai koordinat **nyata** per daerah, jarak dihitung asli dari `lat`/`lng` yang dikirim. Alamat "Rumah" Budi
(`-6.2275, 106.6544`, Tangerang) berjarak 0 km dari Dapur Budi dan ±20 km dari toko Jakarta. Pengiriman maksimal 15 km, jadi untuk mencoba
kurir ke toko Jakarta pakai alamat **Kantor** (`-6.1931, 106.7638`).

## 3. Postman

1. Postman → Import → `SisaGuna.postman_collection.json`.
2. Buka tab **Variables** koleksi, isi `admin_password` dengan `ADMIN_PASSWORD` dari `.env`. `admin_email` default
   `fadhlannur007@gmail.com`; ganti kalau `ADMIN_EMAIL` kamu berbeda. `baseUrl` default `http://localhost:3030`.
3. Mulai dari folder **★ Alur Cepat MVP**: klik kanan folder → *Run folder*. Isinya jalur utama dari login sampai pesanan
   selesai dengan 16 pengecekan otomatis. Semua hijau berarti MVP berfungsi.
4. Lalu jelajahi folder bernomor satu per satu. Jalankan `01 · Login` dulu; token tiap peran tersimpan terpisah
   (`buyer_token`, `seller_token`, `admin_token`, `pending_token`, `new_token`) sehingga kamu bisa berpindah peran tanpa login ulang.
5. Request berlabel "pilih file" (upload foto) membutuhkan file gambar: di tab *Body* pilih kolom `file` → *Select Files*.
6. Access token berlaku 15 menit. Kalau 401, jalankan lagi `01 · Login` (atau `Refresh token`).

Tanpa Postman, jalur yang sama bisa dijalankan dari terminal:

```bash
npx newman run apiendpoints/SisaGuna.postman_collection.json \
  --folder "★ Alur Cepat MVP (jalankan dengan Runner)" --env-var "admin_password=$ADMIN_PASSWORD"
```

## 4. Skenario uji, langkah demi langkah

### A. Pembeli memesan (folder 04, 05, 06)
1. `04 · Home tab makanan` → listing terdekat tampil dengan `distance_km` (hitungan server). `listing_id` dan `merchant_id` tersimpan.
2. `Detail listing` → ada `safety` (level, headline, advice), foto, toko, `more_from_merchant`.
3. `05 · Klaim SELAMAT20` → 204. Klaim lagi → 409.
4. `06 · Quote dengan voucher` → rincian: subtotal, `voucher_discount`, `service_fee`, `total`. Tidak ada data yang berubah.
5. `Buat pesanan` → 201, status `ready`, ada `code` (mis. `SG-4821`), `pickup_by` ± 45 menit. Stok listing berkurang.
6. `Ulangi request yang sama` → 200 dan order yang sama (Idempotency-Key sama), stok tidak berkurang lagi.
7. `Harga berubah` → 409 `price_changed` berisi angka baru.
8. `Batalkan pesanan` → stok dan voucher kembali. Batal lagi → 409 `invalid_order_state`.

### A2. Kurir dan pembayaran tiruan (folder 16, hanya kalau `PARTNERS_MODE=dummy`, default di development)
Kurir (GoSend, GrabExpress, SPX Instant, Lalamove) dan pembayaran non-tunai (QRIS, GoPay, OVO, DANA) **belum terhubung ke mitra sungguhan**.
Rumus ongkir dan promonya sama dengan app Android (tiruan dari API promo kurir) dan pembayaran selalu sukses tanpa uang bergerak.
1. `Alamat saya` menyimpan id Rumah dan Kantor. `Quote pengiriman` → `delivery.options` (4 kurir, termurah dulu), `delivery.selected`, `delivery_fee`, `total`.
2. `Pesan kirim + bayar QRIS` → 201, `payment {status: "paid", provider: "dummy"}`, `fulfillment: delivery`, tanpa `pickup_by`. Komisi toko tidak dikenakan pada ongkir.
3. `Batalkan` → `payment.status: refunded`. `e-wallet belum ditautkan` → 422 `payment_method_not_linked`; tautkan lewat `POST /me/payment-methods`.
4. Alasan pengiriman ditolak (422 `delivery_unavailable`, field `reason`): `too_far` (> 15 km), `merchant_pickup_only` (mis. Kompos Kita), `address_required`, `disabled` (mode `off`).
5. Mode `off` (default produksi): hanya pickup dan tunai; QRIS/e-wallet → 422 `payment_method_unavailable`. Ubah dengan `PARTNERS_MODE` di `.env`.

### B. Mitra menyerahkan pesanan (folder 07)
1. Jalankan `Buat pesanan kedua` (folder 06) dulu, lalu `07 · Pesanan masuk` → pesanan tampil **tanpa** kode pickup.
2. `Konfirmasi dengan kode salah` → 422 `wrong_code` dengan `attempts_left`. Ulangi 5 kali → 429 `too_many_attempts`
   (terkunci 10 menit untuk order itu).
3. `Konfirmasi dengan kode benar` (kode boleh `1234`, `sg-1234`, atau `SG-1234`) → `completed`, `payment_status: paid`.
4. `Dashboard` → `sales_today`, `commission_today`, `net_today`, `completed_today` bertambah.
5. `Mitra membatalkan` butuh alasan; tanpa alasan → 400.

### C. Mitra memposting listing (folder 08)
1. `Upload foto listing` (pilih file) → `listing_media_id` tersimpan.
2. `Buat listing makanan` → 201. Perhatikan `pickup_end`: yang lebih awal antara +2 jam dan batas aman (suhu ruang 4 jam dari `made_at`).
3. `Ulangi request` → 200 dengan listing yang sama.
4. `Makanan lewat batas aman` → 422 `past_safe_window` + `suggested_tier: animal_feed`.
5. `Checklist tidak lengkap` → 422 `checklist_incomplete` + daftar `missing`.
6. Ubah stok/harga, jeda, aktifkan, hapus. Cek `Katalog saya` untuk status turunan (`active`, `sold_out`, `expired`, `paused`).
7. Login sebagai `wati@example.com` (toko pending) lalu posting → 403 `merchant_not_approved`.

### D. Mitra baru disetujui admin (folder 09, 10)
Jalankan folder 09 berurutan: register → daftar toko (pending) → admin melihat antrean → non-admin ditolak (403) → admin menyetujui →
pemilik menerima notifikasi tipe `merchant` → toko tampil di `GET /merchants/:id` (sebelumnya 404).

### E. Aturan otomatis di server (scheduler tiap menit)
- **No-show:** order `ready` yang lewat `pickup_by + 15 menit` menjadi `no_show`, stok kembali, voucher **tidak** kembali.
  Untuk mencobanya tanpa menunggu:
  ```bash
  docker exec sisaguna-be-postgres-1 psql -U sisaguna -d sisaguna -c \
    "UPDATE orders SET pickup_by = now() - interval '16 minutes' WHERE id = '<order_id>'"
  ```
  Tunggu sampai satu menit, lalu `GET /orders/<order_id>` → `no_show`.
- **Reminder:** set `pickup_by = now() + interval '10 minutes'`; dalam semenit pembeli menerima notifikasi tipe `pickup` (sekali saja).
- **Foto yatim:** foto yang diunggah tapi tidak dipakai lebih dari 24 jam dihapus otomatis.

### F. Ulasan dan komplain (folder 12)
Jalankan folder 06 dan 07 dulu (butuh `order_id_2` yang sudah `completed`).
1. `Beri ulasan` → 201. Rating toko (`rating_avg`, `rating_count`) ikut berubah di `GET /merchants/:id`. Ulasan kedua → 409 `already_reviewed`.
2. `Ulasan toko (publik)` → hanya nama depan pembeli yang tampil.
3. `Kirim komplain` → 201, status `submitted`; mitra menerima notifikasi. Komplain kedua → 409 `already_complained`.
4. Admin: `daftar komplain` → `reviewing` → `resolved` (catatan wajib untuk status akhir). Pembeli mendapat notifikasi, dan `GET /orders/:id`
   menampilkan `reviewed: true` dan `complaint_status: resolved`.
5. Batas waktu: ulasan 7 hari, komplain 24 jam setelah `completed_at` (409 `review_window_closed` / `complaint_window_closed`).
6. Foto komplain bersifat privat: hanya pembeli, pemilik toko yang dikomplain, dan admin yang bisa membuka `photo_urls` (orang lain 404).

### G. Dampak, favorit, preferensi notifikasi (folder 13)
- `GET /me/impact`: `completed_orders`, `portions`, `feed_compost_kg`, `saved_rupiah` dari pesanan `completed`. Angka karbon sengaja belum ada.
- `PUT/DELETE /me/saved-merchants/:id` idempoten. `GET /merchants/:id` dengan token menambah `is_saved`; tanpa token field itu tidak ada.
- `PUT /me/notification-prefs` mengganti semua saklar (semua field wajib). Notifikasi tipe yang dimatikan tidak lagi disimpan.
- `DELETE /me/notifications/:id` = soft delete, `POST .../restore` membatalkannya.

### H. Chat dan realtime (folder 14)
1. Pembeli `POST /conversations {merchant_id}` → `conversation_id`. Mitra tidak bisa chat tokonya sendiri (403); toko pending 404.
2. Kirim pesan dengan `client_id` (UUID buatan HP). Kirim ulang `client_id` yang sama → 200 dan pesan yang sama (tidak dobel).
3. Kotak masuk mitra `?as=merchant` menampilkan `unread`. `POST .../read {last_read_id}` menurunkannya ke 0; tanda baca hanya maju.
4. `GET .../messages` urut naik. `?before_id=` untuk scroll ke atas, `?after_id=` untuk sinkron setelah reconnect.
5. **WebSocket di Postman:** jalankan `Tiket WebSocket` (berlaku 30 detik, sekali pakai) → New → WebSocket →
   `ws://localhost:3030/ws?ticket=<ticket>` → Connect. Dari akun lain kirim pesan chat atau buat pesanan: event
   `message.created`, `order.created`, `order.updated`, `notification.created` masuk sebagai JSON `{"type","at","data"}`.
   - Berlangganan stok produk: kirim `{"type":"subscribe","topic":"listing:<listing_id>"}`, lalu buat pesanan untuk listing itu → `listing.stock_changed`.
   - Indikator mengetik: kirim `{"type":"typing","conversation_id":"<id>"}` → lawan bicara menerima `typing`.
   - Batas: 5 koneksi per akun (yang tertua ditutup), 20 topic, pesan masuk maks 4 KB.
   - Alternatif tanpa Postman: `websocat "ws://localhost:3030/ws?ticket=<ticket>"`.

### I. Admin voucher, video listing, token perangkat (folder 15, 08)
- Admin `POST /admin/vouchers`: tepat satu `percent_off` atau `flat_off`; `max_discount` hanya untuk persen; kode huruf besar/angka 4–20 karakter.
  Kode ganda → 409 `voucher_code_taken`. `PATCH /admin/vouchers/:code` hanya mengubah judul, deskripsi, min_spend, masa berlaku, kuota, status aktif, syarat, warna.
  Voucher yang dinonaktifkan tidak bisa diklaim dan tidak berlaku di checkout.
- Video listing: `POST /media` dengan file MP4 ≤ 30 detik dan ≤ 30 MB (`purpose=listing`) → `kind: video`, `duration_ms`. Kirim `id`-nya sebagai `video_media_id`
  saat membuat listing; `GET /listings/:id` menampilkan `video_url`.
- `PUT /me/devices` menyimpan token FCM (push sungguhan menunggu Fase 2), `DELETE /me/devices/:token` saat logout.
- Batas tulis: semua request POST/PUT/PATCH/DELETE dibatasi 120 per menit per akun (429 `rate_limited`).

Panduan khusus developer mobile: `MOBILE.md`. Panduan deploy dan backup di VPS: `../deploy/README.md`.

## 5. Rumus yang bisa kamu cek sendiri

| Aturan | Rumus |
| --- | --- |
| Biaya layanan (dari `subtotal - diskon voucher`) | 0 kalau 0; Rp1.000 di bawah 25.000; Rp2.000 di bawah 75.000; selebihnya Rp3.000 |
| Komisi | 10% dibulatkan ke bawah |
| Voucher umum (toko = null) | Ditanggung SisaGuna: komisi dihitung dari `subtotal` |
| Voucher toko (`merchant_id` terisi) | Ditanggung mitra: komisi dari `subtotal - diskon` |
| `pickup_by` | yang lebih awal antara sekarang + 45 menit dan `pickup_end` paling awal di keranjang |
| Batal oleh pembeli | hanya saat `ready` dan sebelum `pickup_by` |
| Jam aman makanan siap santap | suhu ruang 4 jam, kering 72, kulkas 48, freezer 168 |

Contoh: 2 × Rp8.000 (Nasi Kuning) dengan `HEMAT5K` → subtotal 16.000, diskon 5.000, biaya layanan 1.000, **total 12.000**.
Ongkir (kurir tiruan): `fee = round500((tarif dasar + tarif/km × max(km, 0,5)) × faktor kecepatan)`; GoSend 6.000 + 2.500/km, Grab 6.500 + 2.400/km, SPX 5.000 + 2.600/km, Lalamove 8.000 + 2.200/km; Prioritas ×1,35, Standar ×1, Hemat ×0,75; ongkir tidak dikenai komisi.

## 6. Format respons

Sukses: `{"data": ..., "meta": ...}`. Gagal: `{"error": {"code": "snake_case", "message": "...", ...}}` (field tambahan seperti
`suggested_tier`, `attempts_left`, `total` ada di dalam objek `error`). Uang: integer Rupiah. Waktu: RFC 3339 UTC.
Daftar bertambah (order, notifikasi) memakai `?cursor=` dari `meta.next_cursor`; daftar terbatas memakai `?page=&limit=`.
`POST /listings` dan `POST /orders` wajib header `Idempotency-Key`.

### Kode error yang akan kamu temui

| HTTP | `error.code` | Arti |
| --- | --- | --- |
| 400 | `validation_failed` | Isi tidak valid; pesan menyebut nama field |
| 400 | `idempotency_key_required` | Header `Idempotency-Key` belum dikirim |
| 400 | `invalid_cart` | Keranjang campur toko, ada item dobel, atau listing tidak ada |
| 400 | `voucher_*` validasi (`validation_failed` + `detail`), `video_not_allowed`, `invalid_media` / `invalid_listing` | Foto bukan milikmu/sudah dipakai/jenis salah; produk chat bukan milik toko itu |
| 401 | `unauthorized` / `invalid_credentials` / `invalid_refresh_token` / `invalid_ticket` | Token bermasalah, login salah, atau tiket WebSocket habis/terpakai |
| 403 | `merchant_not_approved` / `merchant_required` / `own_merchant` (juga chat ke toko sendiri) / `forbidden` | Peran tidak mengizinkan |
| 404 | `*_not_found` (termasuk `conversation_not_found`, `complaint_not_found`), `merchant_unavailable`, `voucher_unavailable` | Tidak ada, atau bukan milikmu |
| 422 | `payment_method_not_linked`, `payment_method_fixed`, `delivery_unavailable` (+ `reason`) | E-wallet belum ditautkan; QRIS/tunai tidak bisa dilepas; pengiriman ditolak |
| 409 | `price_changed`, `out_of_stock`, `listing_unavailable`, `invalid_order_state`, `cancel_window_closed`, `email_taken`, `merchant_exists`, `voucher_already_claimed`, `already_reviewed`, `already_complained`, `order_not_completed`, `review_window_closed`, `complaint_window_closed`, `complaint_closed` | Konflik status/stok/batas waktu |
| 413 / 415 | `payload_too_large`, `file_too_large`, `unsupported_media_type` | Body > 1 MB, foto > 8 MB, bukan JPEG/PNG/WebP |
| 422 | `video_too_long`, `corrupt_video`, `past_safe_window`, `checklist_incomplete`, `invalid_price`, `made_at_required`, `payment_method_unavailable`, `delivery_unavailable`, `voucher_*`, `wrong_code` | Aturan bisnis menolak |
| 429 | `rate_limited`, `too_many_attempts` | Terlalu sering (chat 20 pesan/menit per akun) |

## 7. curl cepat

```bash
B=localhost:3030/api/v1
TOKEN=$(curl -s -H 'Content-Type: application/json' -d '{"email":"budi.santoso@gmail.com","password":"Password123!"}' $B/auth/login | python3 -c 'import sys,json;print(json.load(sys.stdin)["data"]["access_token"])')
curl -s "$B/home?lat=-6.2433&lng=106.7989" | python3 -m json.tool | head -40
curl -s -H "Authorization: Bearer $TOKEN" $B/me/orders?tab=history
```

## 8. Masalah umum

| Gejala | Penyebab / solusi |
| --- | --- |
| `connection refused` ke Postgres | Docker belum jalan, atau port bentrok. Cek `docker compose ps`; ubah `POSTGRES_PORT` dan `DATABASE_URL` di `.env` |
| `/home` kosong | Data contoh sudah kedaluwarsa. `make reset-dev` lalu restart server |
| 401 di semua request | Access token 15 menit habis. Jalankan `01 · Login` lagi |
| 429 saat login berulang | Rate limit per IP. Di development default 60/menit; di produksi 5/menit (`LOGIN_RATE_PER_MIN`) |
| Login admin gagal | `admin_password` di koleksi harus sama dengan `ADMIN_PASSWORD` di `.env` saat `make reset-dev` dijalankan |
| Foto tidak tampil di browser | Buka `url` dari respons (mis. `http://localhost:3030/media/ab/....jpg`). Di produksi disajikan Caddy |
