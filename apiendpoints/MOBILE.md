# Panduan Integrasi untuk Developer Mobile (Android & iOS)

Dokumen ini cukup untuk mulai mengintegrasikan API tanpa menunggu backend. Kontrak mesin-baca ada di `api/openapi.yaml`,
koleksi Postman di `apiendpoints/SisaGuna.postman_collection.json`, dan skenario uji manual di `apiendpoints/README.md`.
Contoh respons di bawah diambil dari server sungguhan (daftar dipotong menjadi satu elemen).

## 1. Dasar

| Hal | Aturan |
| --- | --- |
| Base URL | `https://<domain>/api/v1` (lokal: `http://localhost:3030/api/v1`). WebSocket: `wss://<domain>/ws`. Health: `/healthz` |
| Format sukses | `{"data": ..., "meta": ...}` (`meta` hanya untuk daftar) |
| Format gagal | `{"error": {"code": "snake_case", "message": "...", ...field tambahan}}`. Tampilkan/ambil keputusan dari `code`, bukan `message` |
| Uang | Integer Rupiah (`12000`), tidak ada desimal. Muat di `Int` Kotlin/Swift |
| Waktu | RFC 3339 UTC (`2026-10-17T03:45:16Z`). Konversi ke WIB/WITA/WIT di HP |
| Enum | huruf kecil snake_case (`human`, `animal_feed`, `ready`, `no_show`) |
| ID | UUID string, kecuali `messages.id` yang berupa angka urut |
| Foto/video | Selalu URL absolut (`url`, `thumb_url`); tidak pernah path disk |
| Body JSON | Maks 1 MB. Field tak dikenal diabaikan; field wajib kosong → 400 `validation_failed` |
| Jarak | Kirim `lat` & `lng` pengguna; server menghitung `distance_km` (PostGIS, jarak lurus). Tanpa lat/lng nilainya `null` |

### Lokasi
Android/iOS hanya perlu izin lokasi **pembeli** untuk mengirim `lat`/`lng` ke `/home`, `/listings`, `/listings/:id`, `/merchants/:id`, `/me/saved-merchants`.
Kalau izin ditolak, kirim koordinat alamat tersimpan (`/me/addresses`) atau tidak kirim sama sekali. Titik toko diisi mitra saat `POST /merchants`
(`lat`, `lng`) dan bisa diperbarui lewat `PATCH /me/merchant`; listing otomatis ikut.

## 2. Autentikasi

1. `POST /auth/register` atau `/auth/login` → `access_token` (JWT, **15 menit**) dan `refresh_token` (opaque, 30 hari).
2. Kirim `Authorization: Bearer <access_token>` di semua endpoint "Login".
3. Saat menerima **401** `unauthorized`: panggil `POST /auth/refresh {"refresh_token": "..."}`, **simpan kedua token baru**, ulangi request sekali.
   Refresh token berotasi: yang lama langsung hangus. Memakai refresh token lama lagi dianggap pencurian dan mencabut seluruh sesi → arahkan ke layar login.
   Serialisasikan refresh (satu refresh pada satu waktu, request lain menunggu hasilnya) supaya dua request paralel tidak saling menghanguskan.
4. Logout: `POST /auth/logout {"refresh_token": "..."}` dan (kalau ada) `DELETE /me/devices/:token`.
5. `is_admin` dan peran mitra ditentukan server. Mode mitra aktif kalau `GET /me/merchant` mengembalikan toko dengan `status = approved`
   (dengarkan event `merchant.status_changed` atau cek ulang saat aplikasi dibuka).

Rate limit: login 5/menit/IP, register 3/menit/IP (produksi); semua request tulis 120/menit/akun; chat 20 pesan/menit; upload 30/jam. Lewat batas → 429 `rate_limited`.

**Contoh login**, HTTP 200
```json
{
  "data": {
    "access_token": "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJzdWIiOiJjZjNjNDdhMS1lYmE5LTViNWYtODE4NC1lYTRhYzBhNDJkYTEiLCJleHAiOjE3OTIyMTY4MDksImlhdCI6MTc5MjIxNTkwOX0.tZCDDbVyZhqAPdzfbxHn57oAcamM-7eA9jQ_WhupnLM",
    "refresh_token": "eedBb3n8S76JE7X1O-x_EfzrDsaLy22YG6IUhbF6wlw",
    "expires_at": "2026-10-17T06:00:09.322757Z",
    "user": {
      "id": "cf3c47a1-eba9-5b5f-8184-ea4ac0a42da1",
      "name": "Budi Santoso",
      "email": "budi.santoso@gmail.com",
      "phone": "81234567890",
      "avatar_key": null,
      "photo_url": null,
      "location_label": "Tangerang, Banten",
      "selected_address_id": "8f4b18de-f0fd-51a4-bd65-20d0eaba3723",
      "is_admin": false,
      "created_at": "2026-01-01T00:00:00Z"
    }
  }
}
```

(Respons tanpa token: lihat "Tanpa token" di bagian 7.)

## 3. Aturan yang harus diikuti klien

- **Idempotency-Key** wajib di `POST /orders` dan `POST /listings` (UUID baru **per niat pengguna**, bukan per percobaan: pakai ulang key yang sama saat retry
  karena timeout). Request ulang mengembalikan hasil yang sama (200, bukan 201) tanpa efek ganda.
- **Chat** memakai `client_id` (UUID dari HP) dengan cara yang sama.
- **Harga dihitung server.** Kirim `expected_total` di `POST /orders`; kalau beda → 409 `price_changed` berisi angka terbaru, perbarui ringkasan lalu minta konfirmasi lagi.
- **Stok** dijaga server. 409 `out_of_stock` / `listing_unavailable` → muat ulang detail. Berlangganan topic `listing:<id>` untuk stok realtime.
- **Pembayaran non-tunai dan kurir masih TIRUAN** (`PARTNERS_MODE=dummy`, default development): QRIS/GoPay/OVO/DANA selalu sukses (`payment.provider = "dummy"`), ongkir dari rumus tiruan yang sama dengan app Android. Mode `off` (default produksi): hanya tunai + pickup, selain itu 422 `payment_method_unavailable` / `delivery_unavailable`. Cek `quote.payment_kinds` dan `quote.delivery` untuk tahu apa yang ditawarkan.
- **Pengiriman**: kirim `fulfillment: "delivery"`, `courier`, `speed`, `address_id` di quote dan order. Jarak dari alamat ke toko, maks 15 km; 422 `delivery_unavailable` membawa `reason`. Total sudah termasuk ongkir (`delivery_fee`); komisi toko tidak dikenakan pada ongkir.
- **Kode pickup** (`code`, mis. `SG-4821`) hanya ada di respons pembeli. Mitra memasukkannya sendiri; boleh `4821`, `sg-4821`, atau `SG-4821`.
  Salah 5 kali dalam 10 menit mengunci order itu (429 `too_many_attempts`); respons salah berisi `attempts_left`.
- **Pembatalan pembeli** hanya saat `ready` dan sebelum `pickup_by` (409 `cancel_window_closed` setelahnya). No-show otomatis 15 menit setelah `pickup_by`.
- **Ulasan**: sekali per order selesai, maks 7 hari. **Komplain**: sekali, maks 24 jam. Gunakan `reviewed` dan `complaint_status` pada detail order untuk menampilkan tombol.
- **Pagination**: daftar yang terus bertambah (order, notifikasi, ulasan, inbox) memakai `?cursor=<meta.next_cursor>&limit=`; `next_cursor: null` berarti habis.
  Daftar terbatas (`/listings`, katalog mitra, admin) memakai `?page=&limit=` dan `meta.total`, `meta.total_pages`.

## 4. Upload foto dan video

`POST /media` (multipart) dengan field `purpose` (`listing|avatar|merchant|review|complaint`) dan `file`.
Foto JPEG/PNG/WebP maks 8 MB (di-resize ke 1600 px, EXIF/GPS dibuang). **Video MP4 maks 30 MB dan 30 detik, hanya `purpose=listing`**; metadata lokasi dibuang, video tidak di-transcode
(kompres di HP sebelum kirim). Jenis file dikenali dari isi, bukan ekstensi.

Alur: unggah dulu → dapat `id` → kirim `id` di request berikutnya (`photo_media_ids`, `video_media_id`, `photo_media_id`, `media_ids`). Satu file hanya bisa dipakai satu entitas;
file yang tidak dipakai dihapus otomatis setelah 24 jam. Foto `complaint` bersifat privat (URL-nya butuh header Authorization).

## 5. Realtime (WebSocket)

WebSocket hanya **memberi tahu**; data resmi selalu diambil lewat REST.

1. `POST /ws/ticket` → `{"ticket": "...", "expires_in": 30}`. Tiket sekali pakai.
2. Sambung ke `wss://<domain>/ws?ticket=<ticket>`. Satu koneksi untuk seluruh aplikasi (buka saat foreground, tutup saat background).
3. Server → HP: `{"type": "...", "at": "<waktu>", "data": {...}}`.
4. HP → server: `{"type":"subscribe","topic":"listing:<id>"}`, `unsubscribe`, `{"type":"typing","conversation_id":"<id>"}`. Topic lain → frame `error`.
5. Putus: sambung ulang dengan jeda 1, 2, 4 … maks 30 detik **dengan tiket baru**, lalu ambil ulang data layar yang terbuka. Event saat offline tidak dikirim ulang.
   Gunakan `GET /conversations/:id/messages?after_id=<id terakhir>` untuk mengejar chat.

| Event | Penerima | Data | Reaksi |
| --- | --- | --- | --- |
| `order.created` | pemilik toko | `order_id, item_count, total` | kartu pesanan masuk (kode pickup sengaja tidak dikirim ke mitra) |
| `order.updated` | pembeli & pemilik toko | `order_id, status` | muat ulang detail / daftar |
| `listing.stock_changed` | subscriber `listing:<id>` | `listing_id, stock` | nonaktifkan tombol beli kalau 0 |
| `message.created` | pembeli & pemilik toko (termasuk perangkat lain pengirim) | pesan lengkap | tambah ke chat, badge inbox |
| `conversation.read` | lawan bicara | `conversation_id, last_read_id` | tanda dibaca |
| `typing` | lawan bicara | `conversation_id` | indikator 3 detik |
| `notification.created` | pemilik | `notification_id, type, title` | badge lonceng |
| `merchant.status_changed` | pemilik toko | `merchant_id, status` | buka mode mitra |

Batas: 5 koneksi per akun (yang tertua ditutup), 20 topic, pesan masuk maks 4 KB, klien yang terlalu lambat diputus. Server mengirim ping tiap 30 detik (library WebSocket standar membalas otomatis).

## 6. Push notification (FCM)

Daftarkan token dengan `PUT /me/devices {"token": "...", "platform": "android|ios"}` setiap kali token berubah/aplikasi dibuka dan hapus saat logout.
**Pengiriman push belum aktif** (Fase 2); token sudah tersimpan. Sampai itu, gunakan WebSocket (foreground) dan `GET /me/notifications`.

## 7. Contoh respons

Diambil dari server sungguhan (data awal = dummy app Android). Daftar dipotong menjadi satu elemen.

**GET /home?lat=-6.1931&lng=106.7638&tab=human (login sebagai Budi)**, HTTP 200
```json
{
  "data": {
    "nearby": [
      {
        "id": "a02add60-f663-5310-9dea-42d1e7caa8d6",
        "title": "Mie Goreng Spesial",
        "tier": "human",
        "unit": "portion",
        "is_free": false,
        "price": 7000,
        "price_original": 20000,
        "discount_pct": 65,
        "stock": 5,
        "halal": "non_halal",
        "allergens": [
          "soy",
          "…"
        ],
        "pickup_start": "2026-10-17T05:14:57.052667Z",
        "pickup_end": "2026-10-17T07:44:57.052667Z",
        "distance_km": 0.8,
        "cover_url": "http://localhost:3030/media/87/87ccb3be0deeacaf3b799d6ea26d48df524af45d67bbfd956e83e5bf0a6f81a2.jpg",
        "thumb_url": "http://localhost:3030/media/87/87ccb3be0deeacaf3b799d6ea26d48df524af45d67bbfd956e83e5bf0a6f81a2_thumb.jpg",
        "merchant": {
          "id": "c18ef43c-8611-5aa9-9bb3-a85b8756c31c",
          "name": "Bakmi Ahong",
          "location_label": "Kebon Jeruk",
          "rating_avg": 4.6,
          "rating_count": 120,
          "is_verified": true
        },
        "sold_count": 40,
        "delivery": {
          "fee": 8000,
          "eta_min": 31,
          "eta_max": 41
        }
      },
      "…"
    ],
    "deals": [
      {
        "id": "a17f52a8-d27e-5829-b9ff-aa4a593e484d",
        "title": "Nasi Box Rapat Berlebih",
        "tier": "human",
        "unit": "portion",
        "is_free": true,
        "price": null,
        "price_original": 20000,
        "discount_pct": 100,
        "stock": 4,
        "halal": "unverified",
        "allergens": [
          "egg",
          "…"
        ],
        "pickup_start": "2026-10-17T05:14:57.052667Z",
        "pickup_end": "2026-10-17T06:44:57.052667Z",
        "distance_km": 10,
        "cover_url": "http://localhost:3030/media/c5/c5408dc2bb5d0a2f231accd3103fed04b2d4c887fc779790542177f477e7232c.jpg",
        "thumb_url": "http://localhost:3030/media/c5/c5408dc2bb5d0a2f231accd3103fed04b2d4c887fc779790542177f477e7232c_thumb.jpg",
        "merchant": {
          "id": "464dec96-2444-56f3-a8e2-ecc58cca25d5",
          "name": "Kantin Pak Budi",
          "location_label": "Cipete",
          "rating_avg": 4.5,
          "rating_count": 37,
          "is_verified": false
        },
        "sold_count": 22,
        "delivery": {
          "fee": 31000,
          "eta_min": 73,
          "eta_max": 83
        }
      },
      "…"
    ],
    "popular": [
      {
        "id": "7ad95a16-9f0e-5899-9094-6314f8b47fc8",
        "title": "Donat Glaze Sisa Etalase",
        "tier": "human",
        "unit": "portion",
        "is_free": false,
        "price": 3000,
        "price_original": 12000,
        "discount_pct": 75,
        "stock": 10,
        "halal": "halal_certified",
        "allergens": [
          "gluten",
          "…"
        ],
        "pickup_start": "2026-10-17T05:14:57.052667Z",
        "pickup_end": "2026-10-17T10:44:57.052667Z",
        "distance_km": 6.9,
        "cover_url": "http://localhost:3030/media/4a/4acc440d93c01df9f365c67f5b2e17d419b862a45a08c27d8f3dc7d6a1d777d9.jpg",
        "thumb_url": "http://localhost:3030/media/4a/4acc440d93c01df9f365c67f5b2e17d419b862a45a08c27d8f3dc7d6a1d777d9_thumb.jpg",
        "merchant": {
          "id": "0e1243c3-3316-5d5f-a75e-d79affb036af",
          "name": "Roti Segar Bakery",
          "location_label": "Blok M",
          "rating_avg": 4.7,
          "rating_count": 158,
          "is_verified": true
        },
        "sold_count": 95,
        "delivery": {
          "fee": 23000,
          "eta_min": 51,
          "eta_max": 61
        }
      },
      "…"
    ],
    "animal_feed": [
      {
        "id": "d5790ad3-428b-5cc7-961b-6f8c76b73119",
        "title": "Ampas Kelapa Parut",
        "tier": "animal_feed",
        "unit": "kg",
        "is_free": false,
        "price": 1500,
        "price_original": 4000,
        "discount_pct": 63,
        "stock": 10,
        "halal": "other",
        "allergens": [],
        "pickup_start": "2026-10-17T02:44:57.052667Z",
        "pickup_end": "2026-10-17T13:44:57.052667Z",
        "distance_km": 13.4,
        "cover_url": "http://localhost:3030/media/51/515f956c513d42df4b2192b092e7e7af82b82ae0b2081d8d97d5eca8bfef2155.jpg",
        "thumb_url": "http://localhost:3030/media/51/515f956c513d42df4b2192b092e7e7af82b82ae0b2081d8d97d5eca8bfef2155_thumb.jpg",
        "merchant": {
          "id": "8d105567-cd87-5730-a2a5-fb5c7345e3fd",
          "name": "Jajanan Pasar Bu Ning",
          "location_label": "Pasar Minggu",
          "rating_avg": 4.9,
          "rating_count": 240,
          "is_verified": true
        },
        "sold_count": 75,
        "delivery": {
          "fee": 39500,
          "eta_min": 77,
          "eta_max": 87
        }
      },
      "…"
    ],
    "compost": [
      {
        "id": "805cd3c5-6bf0-5e7c-8a6f-479e5d6992bd",
        "title": "Kulit Pisang untuk Kompos",
        "tier": "compost",
        "unit": "kg",
        "is_free": true,
        "price": null,
        "price_original": null,
        "discount_pct": 100,
        "stock": 8,
        "halal": "other",
        "allergens": [],
        "pickup_start": "2026-10-17T02:44:57.052667Z",
        "pickup_end": "2026-10-17T14:44:57.052667Z",
        "distance_km": 8.4,
        "cover_url": "http://localhost:3030/media/32/32642de098609b4608155fb107881ed0d17474013394c87441f8ce7361616c74.jpg",
        "thumb_url": "http://localhost:3030/media/32/32642de098609b4608155fb107881ed0d17474013394c87441f8ce7361616c74_thumb.jpg",
        "merchant": {
          "id": "2d386fd0-361a-5340-aca7-a326d88bb488",
          "name": "Segar Juice & Salad",
          "location_label": "Kuningan",
          "rating_avg": 4.4,
          "rating_count": 41,
          "is_verified": false
        },
        "sold_count": 30,
        "delivery": {
          "fee": 27000,
          "eta_min": 57,
          "eta_max": 67
        }
      },
      "…"
    ],
    "offer_merchants": [
      {
        "id": "0e1243c3-3316-5d5f-a75e-d79affb036af",
        "name": "Roti Segar Bakery",
        "location_label": "Blok M",
        "rating_avg": 4.7,
        "rating_count": 158,
        "is_verified": true,
        "todays_offer": "Semua roti & donat diskon 70% setelah jam 18.00",
        "distance_km": 6.9
      },
      "…"
    ],
    "human_count": 16,
    "farm_count": 12,
    "other_tab_match_count": 0
  }
}
```

**GET /listings/:id**, HTTP 200
```json
{
  "data": {
    "id": "5f8642b5-abd7-5d30-9671-c6a9f44b72fb",
    "title": "Nasi Kuning Sisa Katering",
    "description": "Nasi kuning dengan telur dadar iris, mie goreng, orek tempe, perkedel, dan kentang balado dari pesanan katering kantor siang ini. Dimasak jam 10 pagi, disimpan tertutup. Paling enak dihangatkan sebentar sebelum dimakan.",
    "tier": "human",
    "unit": "portion",
    "status": "active",
    "is_free": false,
    "price": 8000,
    "price_original": 25000,
    "stock": 6,
    "sold_count": 48,
    "halal": "halal_certified",
    "allergens": [
      "egg",
      "…"
    ],
    "made_at": "2026-10-17T04:14:57.052667Z",
    "storage": "room_temp",
    "safety_checks": [
      "untouched",
      "…"
    ],
    "pickup_start": "2026-10-17T05:14:57.052667Z",
    "pickup_end": "2026-10-17T07:44:57.052667Z",
    "distance_km": 9.4,
    "photos": [
      {
        "url": "http://localhost:3030/media/95/9587061f5d4c0a74105b4a4590d19b9f7f927fc1190b2e491b5c0625906fba03.jpg",
        "thumb_url": "http://localhost:3030/media/95/9587061f5d4c0a74105b4a4590d19b9f7f927fc1190b2e491b5c0625906fba03_thumb.jpg"
      },
      "…"
    ],
    "video_url": null,
    "safety": {
      "level": "safe",
      "hours_since_made": 1,
      "safe_until": "2026-10-17T15:14:57.052667+07:00",
      "passed_checks": [
        "untouched",
        "…"
      ],
      "missing_required": [],
      "headline": "Lolos cek kelayakan",
      "advice": "Checklist wajib lengkap dan masih dalam batas waktu aman.",
      "suggest_tier": null,
      "score": 5
    },
    "merchant": {
      "id": "3e752c07-24e1-5d5d-bd2d-5552249d0b1d",
      "name": "Warung Bu Sari",
      "location_label": "Kemang",
      "rating_avg": 4.9,
      "rating_count": 212,
      "is_verified": true,
      "about": "Warung masakan rumahan sejak 2009. Tiap sore sisa lauk katering kantor kami bagikan murah lewat SisaGuna.",
      "address": "Jl. Kemang Raya No. 12, Jakarta Selatan",
      "lat": -6.2615,
      "lng": 106.814,
      "open_hours": "07.00-21.00",
      "halal": "halal_certified",
      "allergens": [
        "egg",
        "…"
      ],
      "delivery_available": true,
      "prep_minutes": 10
    },
    "more_from_merchant": [
      {
        "id": "1e0a8992-6bb0-5292-9a2d-c31841564dab",
        "title": "Pisang Goreng Tepung",
        "tier": "human",
        "unit": "portion",
        "is_free": false,
        "price": 3000,
        "price_original": 10000,
        "discount_pct": 70,
        "stock": 8,
        "halal": "halal_certified",
        "allergens": [
          "gluten"
        ],
        "pickup_start": "2026-10-17T05:14:57.052667Z",
        "pickup_end": "2026-10-17T07:14:57.052667Z",
        "distance_km": 9.4,
        "cover_url": "http://localhost:3030/media/85/851acdfaf0eec5ae6c869ae3bea799b9fb0a4aa91b54b4e872075af94f7e6370.jpg",
        "thumb_url": "http://localhost:3030/media/85/851acdfaf0eec5ae6c869ae3bea799b9fb0a4aa91b54b4e872075af94f7e6370_thumb.jpg",
        "merchant": {
          "id": "3e752c07-24e1-5d5d-bd2d-5552249d0b1d",
          "name": "Warung Bu Sari",
          "location_label": "Kemang",
          "rating_avg": 4.9,
          "rating_count": 212,
          "is_verified": true
        },
        "sold_count": 39,
        "delivery": {
          "fee": 29500,
          "eta_min": 66,
          "eta_max": 76
        }
      },
      "…"
    ]
  }
}
```

**GET /merchants?available=true (rekomendasi penyedia)**, HTTP 200
```json
{
  "data": [
    {
      "id": "c18ef43c-8611-5aa9-9bb3-a85b8756c31c",
      "name": "Bakmi Ahong",
      "location_label": "Kebon Jeruk",
      "todays_offer": null,
      "rating_avg": 4.6,
      "rating_count": 120,
      "is_verified": true,
      "banner_url": "http://localhost:3030/media/87/87ccb3be0deeacaf3b799d6ea26d48df524af45d67bbfd956e83e5bf0a6f81a2.jpg",
      "available_count": 1,
      "next_pickup_end": "2026-10-17T07:44:57.052667Z",
      "distance_km": 0.8,
      "is_saved": false
    },
    "…"
  ]
}
```

**GET /merchants/:id (dengan token)**, HTTP 200
```json
{
  "data": {
    "id": "3e752c07-24e1-5d5d-bd2d-5552249d0b1d",
    "name": "Warung Bu Sari",
    "status": "approved",
    "is_verified": true,
    "about": "Warung masakan rumahan sejak 2009. Tiap sore sisa lauk katering kantor kami bagikan murah lewat SisaGuna.",
    "address": "Jl. Kemang Raya No. 12, Jakarta Selatan",
    "lat": -6.2615,
    "lng": 106.814,
    "location_label": "Kemang",
    "open_hours": "07.00-21.00",
    "halal": "halal_certified",
    "allergens": [
      "egg",
      "…"
    ],
    "todays_offer": "Nasi kuning + sayur sop mulai Rp 4.000 sampai jam 21.00",
    "delivery_available": true,
    "prep_minutes": 10,
    "rating_avg": 4.9,
    "rating_count": 212,
    "total_rescued": 1284,
    "distance_km": 9.4,
    "banner_url": "http://localhost:3030/media/c6/c6c97fa39372a260f4ace9990fa378a025617da3686fdc9df1ad4b54fb9c1ecb.jpg",
    "photos": [
      {
        "url": "http://localhost:3030/media/c6/c6c97fa39372a260f4ace9990fa378a025617da3686fdc9df1ad4b54fb9c1ecb.jpg",
        "thumb_url": "http://localhost:3030/media/c6/c6c97fa39372a260f4ace9990fa378a025617da3686fdc9df1ad4b54fb9c1ecb_thumb.jpg"
      },
      "…"
    ],
    "rating_breakdown": {
      "1": 0,
      "2": 0,
      "3": 0,
      "4": 2,
      "5": 6
    },
    "listings": [
      {
        "id": "5f8642b5-abd7-5d30-9671-c6a9f44b72fb",
        "title": "Nasi Kuning Sisa Katering",
        "tier": "human",
        "unit": "portion",
        "is_free": false,
        "price": 8000,
        "price_original": 25000,
        "discount_pct": 68,
        "stock": 6,
        "halal": "halal_certified",
        "allergens": [
          "egg",
          "…"
        ],
        "pickup_start": "2026-10-17T05:14:57.052667Z",
        "pickup_end": "2026-10-17T07:44:57.052667Z",
        "distance_km": 9.4,
        "cover_url": "http://localhost:3030/media/95/9587061f5d4c0a74105b4a4590d19b9f7f927fc1190b2e491b5c0625906fba03.jpg",
        "thumb_url": "http://localhost:3030/media/95/9587061f5d4c0a74105b4a4590d19b9f7f927fc1190b2e491b5c0625906fba03_thumb.jpg",
        "merchant": {
          "id": "3e752c07-24e1-5d5d-bd2d-5552249d0b1d",
          "name": "Warung Bu Sari",
          "location_label": "Kemang",
          "rating_avg": 4.9,
          "rating_count": 212,
          "is_verified": true
        },
        "sold_count": 48,
        "delivery": {
          "fee": 29500,
          "eta_min": 66,
          "eta_max": 76
        }
      },
      "…"
    ],
    "is_saved": true
  }
}
```

**POST /checkout/quote (pickup + voucher)**, HTTP 200
```json
{
  "data": {
    "lines": [
      {
        "listing_id": "5f8642b5-abd7-5d30-9671-c6a9f44b72fb",
        "title": "Nasi Kuning Sisa Katering",
        "quantity": 2,
        "unit_price": 8000,
        "unit_original_price": 25000,
        "line_total": 16000
      }
    ],
    "subtotal": 16000,
    "original_total": 50000,
    "voucher_code": "HEMAT5K",
    "voucher_discount": 5000,
    "service_fee": 1000,
    "delivery_fee": 0,
    "total": 12000,
    "fulfillment": "pickup",
    "pickup_by": "2026-10-17T06:30:09.582135Z",
    "pickup_end": "2026-10-17T07:44:57.052667Z",
    "payment_kinds": [
      "qris",
      "…"
    ],
    "delivery": {
      "available": false,
      "reason": "too_far",
      "distance_km": 18.1,
      "address": {
        "id": "8f4b18de-f0fd-51a4-bd65-20d0eaba3723",
        "label": "Rumah",
        "full_address": "Jl. Sutera Onyx XII No.30, Kunciran, Kec. Pinang, Kota Tangerang, Banten 15144",
        "note": "",
        "lat": -6.2275,
        "lng": 106.6544
      },
      "speed": "standard",
      "options": [],
      "selected": null
    }
  }
}
```

**POST /checkout/quote (pengiriman GoSend Standar, alamat Kantor)**, HTTP 200
```json
{
  "data": {
    "lines": [
      {
        "listing_id": "5f8642b5-abd7-5d30-9671-c6a9f44b72fb",
        "title": "Nasi Kuning Sisa Katering",
        "quantity": 2,
        "unit_price": 8000,
        "unit_original_price": 25000,
        "line_total": 16000
      }
    ],
    "subtotal": 16000,
    "original_total": 50000,
    "voucher_code": null,
    "voucher_discount": 0,
    "service_fee": 1000,
    "delivery_fee": 29500,
    "total": 46500,
    "fulfillment": "delivery",
    "pickup_by": null,
    "pickup_end": "2026-10-17T07:44:57.052667Z",
    "payment_kinds": [
      "qris",
      "…"
    ],
    "delivery": {
      "available": true,
      "reason": null,
      "distance_km": 9.4,
      "address": {
        "id": "03e1461f-6de5-5237-820c-d80c30d11258",
        "label": "Kantor",
        "full_address": "Business Park Kebon Jeruk Ruko AB-6, Meruya Utara, Kembangan, Jakarta Barat 11620",
        "note": "Lantai 2, sebelah lift",
        "lat": -6.1931,
        "lng": 106.7638
      },
      "speed": "standard",
      "options": [
        {
          "courier": "shopee",
          "courier_label": "SPX Instant",
          "speed": "standard",
          "speed_label": "Standar",
          "fee": 29500,
          "discount": 2000,
          "payable": 27500,
          "eta_min": 66,
          "eta_max": 76,
          "promo_label": "Potongan Rp 2.000",
          "promo_label_en": "Rp 2,000 off"
        },
        "…"
      ],
      "selected": {
        "courier": "gosend",
        "courier_label": "GoSend",
        "speed": "standard",
        "speed_label": "Standar",
        "fee": 29500,
        "discount": 0,
        "payable": 29500,
        "eta_min": 66,
        "eta_max": 76,
        "promo_label": null,
        "promo_label_en": null
      }
    }
  }
}
```

**422 delivery_unavailable**, HTTP 422
```json
{
  "error": {
    "code": "delivery_unavailable",
    "message": "pengantaran tidak tersedia untuk pesanan ini, gunakan ambil di toko",
    "reason": "too_far"
  }
}
```

**POST /orders (pengiriman, QRIS tiruan)**, HTTP 201
```json
{
  "data": {
    "id": "f0420978-4971-4998-927d-2c5dc44f5854",
    "code": "SG-0859",
    "status": "ready",
    "fulfillment": "delivery",
    "payment_kind": "qris",
    "payment_status": "paid",
    "note": "",
    "merchant": {
      "id": "3e752c07-24e1-5d5d-bd2d-5552249d0b1d",
      "name": "Warung Bu Sari",
      "location_label": "Kemang",
      "lat": -6.2615,
      "lng": 106.814
    },
    "items": [
      {
        "listing_id": "5f8642b5-abd7-5d30-9671-c6a9f44b72fb",
        "title": "Nasi Kuning Sisa Katering",
        "tier": "human",
        "unit": "portion",
        "quantity": 2,
        "unit_price": 8000,
        "unit_original_price": 25000,
        "image_url": "http://localhost:3030/media/95/9587061f5d4c0a74105b4a4590d19b9f7f927fc1190b2e491b5c0625906fba03.jpg",
        "thumb_url": "http://localhost:3030/media/95/9587061f5d4c0a74105b4a4590d19b9f7f927fc1190b2e491b5c0625906fba03_thumb.jpg"
      }
    ],
    "subtotal": 16000,
    "original_total": 50000,
    "voucher_code": null,
    "voucher_discount": 0,
    "service_fee": 1000,
    "delivery_fee": 29500,
    "total": 46500,
    "pickup_by": null,
    "pickup_end": "2026-10-17T07:44:57.052667Z",
    "created_at": "2026-10-17T05:45:09.593996Z",
    "completed_at": null,
    "cancelled_at": null,
    "cancelled_by": null,
    "cancel_reason": null,
    "reviewed": false,
    "complaint_status": null,
    "rating": null,
    "complaint": null,
    "delivery": {
      "courier": "gosend",
      "courier_label": "GoSend",
      "speed": "standard",
      "speed_label": "Standar",
      "fee": 29500,
      "discount": 0,
      "payable": 29500,
      "eta_min": 66,
      "eta_max": 76
    },
    "delivery_address": {
      "id": "03e1461f-6de5-5237-820c-d80c30d11258",
      "label": "Kantor",
      "full_address": "Business Park Kebon Jeruk Ruko AB-6, Meruya Utara, Kembangan, Jakarta Barat 11620",
      "note": "Lantai 2, sebelah lift",
      "lat": -6.1931,
      "lng": 106.7638
    },
    "payment": {
      "kind": "qris",
      "status": "paid",
      "provider": "dummy",
      "paid_at": "2026-10-17T05:45:09.593996Z"
    },
    "can_cancel": true,
    "can_rate": false,
    "can_complain": false
  }
}
```

**POST /orders (pickup + voucher, pembeli melihat kode)**, HTTP 201
```json
{
  "data": {
    "id": "c64eca20-bd9c-4188-a171-58d9f96953f3",
    "code": "SG-6240",
    "status": "ready",
    "fulfillment": "pickup",
    "payment_kind": "cash",
    "payment_status": "unpaid",
    "note": "",
    "merchant": {
      "id": "3e752c07-24e1-5d5d-bd2d-5552249d0b1d",
      "name": "Warung Bu Sari",
      "location_label": "Kemang",
      "lat": -6.2615,
      "lng": 106.814
    },
    "items": [
      {
        "listing_id": "5f8642b5-abd7-5d30-9671-c6a9f44b72fb",
        "title": "Nasi Kuning Sisa Katering",
        "tier": "human",
        "unit": "portion",
        "quantity": 2,
        "unit_price": 8000,
        "unit_original_price": 25000,
        "image_url": "http://localhost:3030/media/95/9587061f5d4c0a74105b4a4590d19b9f7f927fc1190b2e491b5c0625906fba03.jpg",
        "thumb_url": "http://localhost:3030/media/95/9587061f5d4c0a74105b4a4590d19b9f7f927fc1190b2e491b5c0625906fba03_thumb.jpg"
      }
    ],
    "subtotal": 16000,
    "original_total": 50000,
    "voucher_code": "HEMAT5K",
    "voucher_discount": 5000,
    "service_fee": 1000,
    "delivery_fee": 0,
    "total": 12000,
    "pickup_by": "2026-10-17T06:30:09.630136Z",
    "pickup_end": "2026-10-17T07:44:57.052667Z",
    "created_at": "2026-10-17T05:45:09.629673Z",
    "completed_at": null,
    "cancelled_at": null,
    "cancelled_by": null,
    "cancel_reason": null,
    "reviewed": false,
    "complaint_status": null,
    "rating": null,
    "complaint": null,
    "delivery": null,
    "delivery_address": null,
    "payment": {
      "kind": "cash",
      "status": "unpaid",
      "provider": null,
      "paid_at": null
    },
    "can_cancel": true,
    "can_rate": false,
    "can_complain": false
  }
}
```

**409 price_changed**, HTTP 409
```json
{
  "error": {
    "code": "price_changed",
    "delivery_fee": 0,
    "message": "harga berubah, periksa ringkasan terbaru",
    "service_fee": 1000,
    "subtotal": 16000,
    "total": 17000,
    "voucher_discount": 0
  }
}
```

**GET /me/merchant/orders (mitra: tanpa kode)**, HTTP 200
```json
{
  "data": [
    {
      "id": "c64eca20-bd9c-4188-a171-58d9f96953f3",
      "status": "ready",
      "fulfillment": "pickup",
      "payment_kind": "cash",
      "payment_status": "unpaid",
      "note": "",
      "merchant": {
        "id": "3e752c07-24e1-5d5d-bd2d-5552249d0b1d",
        "name": "Warung Bu Sari",
        "location_label": "Kemang",
        "lat": -6.2615,
        "lng": 106.814
      },
      "buyer_name": "Budi Santoso",
      "items": [
        {
          "listing_id": "5f8642b5-abd7-5d30-9671-c6a9f44b72fb",
          "title": "Nasi Kuning Sisa Katering",
          "tier": "human",
          "unit": "portion",
          "quantity": 2,
          "unit_price": 8000,
          "unit_original_price": 25000,
          "image_url": "http://localhost:3030/media/95/9587061f5d4c0a74105b4a4590d19b9f7f927fc1190b2e491b5c0625906fba03.jpg",
          "thumb_url": "http://localhost:3030/media/95/9587061f5d4c0a74105b4a4590d19b9f7f927fc1190b2e491b5c0625906fba03_thumb.jpg"
        }
      ],
      "subtotal": 16000,
      "original_total": 50000,
      "voucher_code": "HEMAT5K",
      "voucher_discount": 5000,
      "service_fee": 1000,
      "delivery_fee": 0,
      "total": 12000,
      "commission": 1600,
      "merchant_payout": 14400,
      "pickup_by": "2026-10-17T06:30:09.630136Z",
      "pickup_end": "2026-10-17T07:44:57.052667Z",
      "created_at": "2026-10-17T05:45:09.629673Z",
      "completed_at": null,
      "cancelled_at": null,
      "cancelled_by": null,
      "cancel_reason": null,
      "reviewed": false,
      "complaint_status": null,
      "rating": null,
      "complaint": null,
      "delivery": null,
      "delivery_address": null,
      "payment": {
        "kind": "cash",
        "status": "unpaid",
        "provider": null,
        "paid_at": null
      },
      "can_cancel": true,
      "can_rate": false,
      "can_complain": false
    },
    "…"
  ],
  "meta": {
    "next_cursor": null
  }
}
```

**Kode salah**, HTTP 422
```json
{
  "error": {
    "attempts_left": 4,
    "code": "wrong_code",
    "message": "kode pickup salah"
  }
}
```

**Konfirmasi berhasil**, HTTP 200
```json
{
  "data": {
    "id": "c64eca20-bd9c-4188-a171-58d9f96953f3",
    "status": "completed",
    "fulfillment": "pickup",
    "payment_kind": "cash",
    "payment_status": "paid",
    "note": "",
    "merchant": {
      "id": "3e752c07-24e1-5d5d-bd2d-5552249d0b1d",
      "name": "Warung Bu Sari",
      "location_label": "Kemang",
      "lat": -6.2615,
      "lng": 106.814
    },
    "buyer_name": "Budi Santoso",
    "items": [
      {
        "listing_id": "5f8642b5-abd7-5d30-9671-c6a9f44b72fb",
        "title": "Nasi Kuning Sisa Katering",
        "tier": "human",
        "unit": "portion",
        "quantity": 2,
        "unit_price": 8000,
        "unit_original_price": 25000,
        "image_url": "http://localhost:3030/media/95/9587061f5d4c0a74105b4a4590d19b9f7f927fc1190b2e491b5c0625906fba03.jpg",
        "thumb_url": "http://localhost:3030/media/95/9587061f5d4c0a74105b4a4590d19b9f7f927fc1190b2e491b5c0625906fba03_thumb.jpg"
      }
    ],
    "subtotal": 16000,
    "original_total": 50000,
    "voucher_code": "HEMAT5K",
    "voucher_discount": 5000,
    "service_fee": 1000,
    "delivery_fee": 0,
    "total": 12000,
    "commission": 1600,
    "merchant_payout": 14400,
    "pickup_by": "2026-10-17T06:30:09.630136Z",
    "pickup_end": "2026-10-17T07:44:57.052667Z",
    "created_at": "2026-10-17T05:45:09.629673Z",
    "completed_at": "2026-10-17T05:45:09.658501Z",
    "cancelled_at": null,
    "cancelled_by": null,
    "cancel_reason": null,
    "reviewed": false,
    "complaint_status": null,
    "rating": null,
    "complaint": null,
    "delivery": null,
    "delivery_address": null,
    "payment": {
      "kind": "cash",
      "status": "paid",
      "provider": null,
      "paid_at": "2026-10-17T05:45:09.658501Z"
    },
    "can_cancel": false,
    "can_rate": false,
    "can_complain": false
  }
}
```

**POST /orders/:id/review**, HTTP 201
```json
{
  "data": {
    "id": "098b97f6-f2eb-4e23-8d03-c237711c910e",
    "stars": 5,
    "tags": [
      "Porsi pas"
    ],
    "comment": "Enak!",
    "buyer_name": "",
    "photos": [],
    "created_at": "2026-10-17T05:45:09.667397Z"
  }
}
```

**GET /me/impact**, HTTP 200
```json
{
  "data": {
    "completed_orders": 3,
    "portions": 36,
    "feed_compost_kg": 3,
    "saved_rupiah": 284000,
    "carbon_kg": 32,
    "tier": "hero"
  }
}
```

**GET /me/payment-methods**, HTTP 200
```json
{
  "data": [
    {
      "kind": "qris",
      "label": "QRIS",
      "detail": "Bayar pakai aplikasi bank atau e-wallet apa pun",
      "is_default": true
    },
    "…"
  ]
}
```

**GET /me/vouchers**, HTTP 200
```json
{
  "data": [
    {
      "code": "SELAMAT20",
      "title": "Diskon 20%",
      "description": "Maks. Rp 8.000 · tanpa minimum",
      "percent_off": 20,
      "flat_off": null,
      "max_discount": 8000,
      "min_spend": 0,
      "merchant_id": null,
      "merchant_name": null,
      "applies_to_tier": null,
      "applies_to": "Roti, kue & dessert",
      "image_url": "http://localhost:3030/media/4a/4acc440d93c01df9f365c67f5b2e17d419b862a45a08c27d8f3dc7d6a1d777d9.jpg",
      "expires_at": "2026-10-24T05:44:57.052667Z",
      "terms": [
        "Diskon 20% dari subtotal, maksimal Rp 8.000.",
        "…"
      ],
      "accent_hex": "#C0306A",
      "status": "available"
    },
    "…"
  ]
}
```

**POST /conversations**, HTTP 200
```json
{
  "data": {
    "id": "5a8fc60b-fe23-5580-b091-9797a348541c",
    "counterpart": {
      "id": "3e752c07-24e1-5d5d-bd2d-5552249d0b1d",
      "name": "Warung Bu Sari"
    },
    "last_message": null,
    "unread": 0
  }
}
```

**POST /conversations/:id/messages**, HTTP 201
```json
{
  "data": {
    "id": 10,
    "conversation_id": "5a8fc60b-fe23-5580-b091-9797a348541c",
    "sender_side": "buyer",
    "body": "Masih ada?",
    "listing_id": null,
    "client_id": "2b9761a0-a0c7-4e51-8a78-a29364097449",
    "created_at": "2026-10-17T05:45:09.708735Z"
  }
}
```

**GET /conversations?as=merchant**, HTTP 200
```json
{
  "data": [
    {
      "id": "5a8fc60b-fe23-5580-b091-9797a348541c",
      "counterpart": {
        "id": "cf3c47a1-eba9-5b5f-8184-ea4ac0a42da1",
        "name": "Budi Santoso"
      },
      "last_message": {
        "body": "Masih ada?",
        "side": "buyer",
        "at": "2026-10-17T05:45:09.708735Z"
      },
      "unread": 1
    }
  ],
  "meta": {
    "next_cursor": null
  }
}
```

**GET /me/notifications**, HTTP 200
```json
{
  "data": [
    {
      "id": "b01e1785-f3c7-4860-911f-573861fec679",
      "type": "order",
      "title": "Pesanan selesai",
      "body": "Terima kasih sudah menyelamatkan Nasi Kuning Sisa Katering. Beri rating untuk Warung Bu Sari?",
      "data": {
        "order_id": "c64eca20-bd9c-4188-a171-58d9f96953f3"
      },
      "created_at": "2026-10-17T05:45:09.658501Z",
      "read_at": null
    },
    "…"
  ],
  "meta": {
    "next_cursor": "eyJhdCI6IjIwMjYtMTAtMTdUMDU6NDU6MDkuNjI5NjczWiIsImlkIjoiZTE5NDdmNTctNzExOS00NzI1LWEzM2UtNWMxMDAwNmU1N2EwIn0"
  }
}
```

**POST /ws/ticket**, HTTP 200
```json
{
  "data": {
    "expires_in": 30,
    "ticket": "1a15c3335c1b9155da8a0e23a701fd872a733a2e5e4545df"
  }
}
```

**GET /me/merchant**, HTTP 200
```json
{
  "data": {
    "id": "3e752c07-24e1-5d5d-bd2d-5552249d0b1d",
    "name": "Warung Bu Sari",
    "status": "approved",
    "is_verified": true,
    "about": "Warung masakan rumahan sejak 2009. Tiap sore sisa lauk katering kantor kami bagikan murah lewat SisaGuna.",
    "address": "Jl. Kemang Raya No. 12, Jakarta Selatan",
    "lat": -6.2615,
    "lng": 106.814,
    "location_label": "Kemang",
    "open_hours": "07.00-21.00",
    "halal": "halal_certified",
    "allergens": [
      "egg",
      "…"
    ],
    "todays_offer": "Nasi kuning + sayur sop mulai Rp 4.000 sampai jam 21.00",
    "delivery_available": true,
    "prep_minutes": 10,
    "rating_avg": 4.8,
    "rating_count": 9,
    "total_rescued": 1286,
    "banner_url": "http://localhost:3030/media/c6/c6c97fa39372a260f4ace9990fa378a025617da3686fdc9df1ad4b54fb9c1ecb.jpg",
    "photos": [
      {
        "url": "http://localhost:3030/media/c6/c6c97fa39372a260f4ace9990fa378a025617da3686fdc9df1ad4b54fb9c1ecb.jpg",
        "thumb_url": "http://localhost:3030/media/c6/c6c97fa39372a260f4ace9990fa378a025617da3686fdc9df1ad4b54fb9c1ecb_thumb.jpg"
      },
      "…"
    ]
  }
}
```

**400 validation_failed**, HTTP 400
```json
{
  "error": {
    "code": "validation_failed",
    "message": "kolom tidak valid: merchant_id, lines"
  }
}
```

**Tanpa token**, HTTP 401
```json
{
  "error": {
    "code": "unauthorized",
    "message": "header Authorization Bearer wajib diisi"
  }
}
```

## 8. Daftar endpoint

| Method | Path | Akses | Fungsi |
| --- | --- | --- | --- |
| POST | `/api/v1/auth/register` | Publik | Daftar + langsung login |
| POST | `/api/v1/auth/login` | Publik | Login |
| POST | `/api/v1/auth/refresh` | Publik | Tukar refresh token (rotasi) |
| POST | `/api/v1/auth/logout` | Login | Cabut refresh token |
| POST | `/api/v1/auth/password` | Login | Ganti password |
| GET | `/api/v1/me` | Login | Profil + jumlah notifikasi belum dibaca |
| PATCH | `/api/v1/me` | Login | Ubah profil / foto |
| GET | `/api/v1/me/impact` | Login | Statistik dampak |
| GET | `/api/v1/me/addresses` | Login | Daftar alamat |
| POST | `/api/v1/me/addresses` | Login | Tambah alamat (maks 20) |
| PATCH | `/api/v1/me/addresses/:id` | Login | Ubah alamat |
| DELETE | `/api/v1/me/addresses/:id` | Login | Hapus alamat |
| PUT | `/api/v1/me/selected-address` | Login | Pilih alamat aktif (null = GPS) |
| POST | `/api/v1/media` | Login | Upload foto / video (multipart) |
| GET | `/api/v1/media/private/:id` | Login | Foto komplain (pemilik, toko, admin) |
| GET | `/api/v1/home` | Publik | Beranda: rail nearby/deals/popular/animal_feed/compost + toko dengan penawaran |
| GET | `/api/v1/listings` | Publik | Daftar listing (filter, sort, page) |
| GET | `/api/v1/listings/:id` | Publik | Detail listing + penilaian keamanan pangan |
| POST | `/api/v1/listings` | Mitra | Buat listing (Idempotency-Key) |
| PATCH | `/api/v1/listings/:id` | Mitra | Ubah stok/harga/jeda |
| DELETE | `/api/v1/listings/:id` | Mitra | Hapus (soft delete) |
| POST | `/api/v1/merchants` | Login | Daftar toko (status pending) |
| GET | `/api/v1/merchants` | Publik* | Rekomendasi penyedia: `available_count`, `next_pickup_end`, `is_saved` |
| GET | `/api/v1/merchants/:id` | Publik* | Halaman toko (*token opsional → is_saved) |
| GET | `/api/v1/merchants/:id/reviews` | Publik | Ulasan toko (cursor) |
| GET | `/api/v1/me/merchant` | Login | Toko saya |
| PATCH | `/api/v1/me/merchant` | Login | Ubah toko |
| GET | `/api/v1/me/merchant/dashboard` | Mitra | Ringkasan hari ini |
| GET | `/api/v1/me/merchant/listings` | Mitra | Katalog saya |
| GET | `/api/v1/me/merchant/orders` | Mitra | Pesanan masuk |
| POST | `/api/v1/me/merchant/orders/:id/confirm` | Mitra | Serahkan pesanan dengan kode |
| POST | `/api/v1/me/merchant/orders/:id/cancel` | Mitra | Batalkan (alasan wajib) |
| POST | `/api/v1/checkout/quote` | Login | Rincian harga (tidak mengubah data) |
| POST | `/api/v1/orders` | Login | Buat pesanan (Idempotency-Key) |
| GET | `/api/v1/orders/:id` | Login | Detail pesanan |
| GET | `/api/v1/me/orders` | Login | Riwayat / berjalan (cursor) |
| POST | `/api/v1/orders/:id/cancel` | Login | Batalkan pesanan |
| POST | `/api/v1/orders/:id/review` | Login | Beri ulasan |
| POST | `/api/v1/orders/:id/complaint` | Login | Kirim komplain |
| GET | `/api/v1/me/vouchers` | Login | Voucher tersedia/diklaim/terpakai |
| POST | `/api/v1/vouchers/:code/claim` | Login | Klaim voucher |
| GET | `/api/v1/me/payment-methods` | Login | Metode pembayaran (QRIS, e-wallet tertaut, tunai) — tiruan |
| POST | `/api/v1/me/payment-methods` | Login | Tautkan GoPay/OVO/DANA (tiruan) |
| PUT | `/api/v1/me/payment-methods/default` | Login | Metode utama |
| DELETE | `/api/v1/me/payment-methods/:kind` | Login | Lepas e-wallet |
| GET | `/api/v1/me/saved-merchants` | Login | Toko favorit |
| PUT | `/api/v1/me/saved-merchants/:merchantId` | Login | Simpan toko |
| DELETE | `/api/v1/me/saved-merchants/:merchantId` | Login | Hapus favorit |
| GET | `/api/v1/me/notifications` | Login | Notifikasi (cursor, ?type=) |
| PATCH | `/api/v1/me/notifications/:id` | Login | Tandai dibaca |
| POST | `/api/v1/me/notifications/read-all` | Login | Tandai semua dibaca |
| DELETE | `/api/v1/me/notifications/:id` | Login | Hapus (soft) |
| POST | `/api/v1/me/notifications/:id/restore` | Login | Batalkan hapus |
| GET | `/api/v1/me/notification-prefs` | Login | Preferensi notifikasi |
| PUT | `/api/v1/me/notification-prefs` | Login | Ganti preferensi |
| PUT | `/api/v1/me/devices` | Login | Daftarkan token FCM |
| DELETE | `/api/v1/me/devices/:token` | Login | Hapus token FCM (saat logout) |
| GET | `/api/v1/conversations` | Login | Kotak masuk chat (?as=buyer|merchant) |
| POST | `/api/v1/conversations` | Login | Mulai chat dengan toko |
| GET | `/api/v1/conversations/:id/messages` | Login | Pesan (before_id / after_id) |
| POST | `/api/v1/conversations/:id/messages` | Login | Kirim pesan (client_id) |
| POST | `/api/v1/conversations/:id/read` | Login | Tandai dibaca |
| POST | `/api/v1/ws/ticket` | Login | Tiket WebSocket |
| GET | `/api/v1/admin/merchants` | Admin | Antrean toko |
| POST | `/api/v1/admin/merchants/:id/approve` | Admin | Setujui toko |
| POST | `/api/v1/admin/merchants/:id/reject` | Admin | Tolak toko |
| GET | `/api/v1/admin/complaints` | Admin | Daftar komplain |
| PATCH | `/api/v1/admin/complaints/:id` | Admin | Ubah status komplain |
| GET | `/api/v1/admin/vouchers` | Admin | Daftar voucher |
| POST | `/api/v1/admin/vouchers` | Admin | Buat voucher |
| PATCH | `/api/v1/admin/vouchers/:code` | Admin | Ubah voucher |

Kode error lengkap: bagian 6 di `apiendpoints/README.md`. Akun contoh (hanya development, setelah `make reset-dev`): `budi.santoso@gmail.com` (pembeli sekaligus pemilik Dapur Budi, akun utama app),
`owner-m1@example.com` (Warung Bu Sari, mitra approved), `wati@example.com` (mitra pending), password `Password123!`.
Pemetaan lengkap ke kode Kotlin dan perbedaan perilaku: `ANDROID-SYNC.md`.

## 9. Yang belum ada

Kemitraan kurir dan payment gateway sungguhan (saat ini tiruan), push FCM, lupa password lewat email, faktor emisi karbon resmi.
