# Sinkronisasi Backend ↔ App Android

Dokumen ini untuk tim Android: apa yang harus diganti saat `Fake*Repository` di `di/RepositoryModule.kt` diganti panggilan ke API,
field mana yang berubah nama, dan **titik-titik di app yang sengaja tidak diikuti backend**. Backend sudah disesuaikan dengan model dan
data dummy di folder `SisaGuna/` (commit `b354f19`): isi API di database dev = salinan dummy tersebut.

Pelengkap: `MOBILE.md` (alur token, WebSocket, contoh respons), `README.md` (skenario uji Postman), `../api/openapi.yaml` (kontrak).

## 1. Konvensi yang berubah dari Kotlin

| Hal | Kotlin sekarang | API |
| --- | --- | --- |
| Nama field | camelCase (`priceOriginal`) | snake_case (`price_original`). Pakai `@SerialName` atau `FieldNamingPolicy.LOWER_CASE_WITH_UNDERSCORES` |
| ID | String pendek (`"l1"`, `"m4"`, `"o2"`) | UUID string. Tetap `String` di Kotlin, jangan di-parse |
| Enum | `HUMAN`, `ANIMAL_FEED`, `READY` | huruf kecil: `human`, `animal_feed`, `ready` (`lowercase()` / `@SerialName`) |
| Waktu | `Instant` | RFC 3339 UTC (`2026-10-17T03:45:16Z`), `Instant.parse()` langsung |
| Uang | `Int` rupiah (`Long` untuk `savedRupiah`) | integer rupiah |
| Foto | `file:///android_asset/img/x.jpg` | URL absolut `https://…/media/ab/<hash>.jpg` (Coil `AsyncImage` tanpa perubahan) |
| Bentuk respons | objek langsung | `{"data": …, "meta": …}`; error `{"error": {"code", "message", …}}` |
| Jarak | `Listing.distanceKm` terisi dummy | `distance_km` dihitung server dari `lat`/`lng` yang **dikirim app** (null kalau tidak dikirim) |
| Sinkron | `fun addListing`, `fun cancel` … | semua jadi `suspend` mengembalikan `Result`; `StateFlow` tetap dipakai sebagai cache lokal yang diisi dari respons |

## 2. Repository → endpoint

| Interface (Kotlin) | Metode | Endpoint |
| --- | --- | --- |
| **Auth** (`SessionViewModel`, belum ada repository) | login, register, logout, refresh | `POST /auth/login`, `/auth/register`, `/auth/logout`, `/auth/refresh`. Simpan `access_token` (15 menit) + `refresh_token`; lihat `MOBILE.md` §2 |
| `ListingRepository` | `getHomeFeed(tier)` | `GET /home?lat&lng&tab&q&sort&free&max_km` → `nearby, deals, popular, animal_feed, compost, offer_merchants, human_count, farm_count, other_tab_match_count`. Toko tiap kartu ada di `card.merchant` (pengganti `merchantsById`) |
| | `getListingsByTier(tier)` | `GET /listings?tier&sort&q&free&max_km&lat&lng&page&limit` (`meta.total`). Sort: `relevant, recommended, rating, delivery_fee, delivery_time, nearest, price_low, discount, ending_soon` (= `CategorySort`) |
| | `getMerchants()` | `GET /merchants?lat&lng&available=true` → kartu toko (`available_count`, `next_pickup_end`, `is_saved`); toko milik sendiri tidak ikut |
| | `merchant(id)`, `getMerchantFeed(id)` | `GET /merchants/:id?lat&lng` → profil toko + `listings` + `rating_breakdown` + `is_saved` (kalau login) |
| | `getVisibleListings()` | tidak ada padanan satu-satu; pakai `GET /listings` (paging) |
| | `getListingDetail(id)` | `GET /listings/:id?lat&lng` → listing, `merchant`, `more_from_merchant` (terlaris dulu), `safety` (= `SafetyAssessment`), `photos`, `video_url` |
| | `addListing(listing)` | `POST /media` per foto/video (`purpose=listing`) lalu `POST /listings` + header `Idempotency-Key`. Server menghitung `pickup_end` dan menolak yang tak layak (`past_safe_window`, `checklist_incomplete`) |
| | `deleteListing(id)` | `DELETE /listings/:id` (soft delete) |
| | `reduceStock` | **hapus**: stok hanya berkurang di server saat checkout |
| | `MY_MERCHANT_ID = "m0"` | ambil id toko sendiri dari `GET /me/merchant` |
| `SavedMerchantRepository` | `savedIds`, `unsave`, `restore` | `GET /me/saved-merchants` (urut disimpan), `DELETE /me/saved-merchants/:id`, `PUT /me/saved-merchants/:id`. Server menambahkan di akhir; posisi lama pada "Batalkan" dipulihkan di sisi app |
| `ProfileRepository` | `profile`, `update` | `GET /me`, `PATCH /me {name,email,phone}` (`phone` digit tanpa 62/0) |
| | `setPhoto`, `setAvatar` | `POST /media (purpose=avatar)` lalu `PATCH /me {photo_media_id}`; avatar: `PATCH /me {avatar_key}` (8 kunci buah di app) |
| | `getImpact` | `GET /me/impact` → `completed_orders, portions, feed_compost_kg, saved_rupiah, carbon_kg (estimasi 0,9/porsi), tier` |
| | `getMyCatalog` | `GET /me/merchant/listings` (status turunan `active/sold_out/expired/paused`) |
| | `changePassword` | `POST /auth/password {current,new}` → 200, atau 400 `wrong_password` |
| `NotificationRepository` | `notifications`, filter | `GET /me/notifications?type&cursor&limit` (grup hari/relativeTime tetap dihitung di app) |
| | `markRead`, `markAllRead` | `PATCH /me/notifications/:id {read:true}`, `POST /me/notifications/read-all` |
| | `delete`, `restore` | `DELETE /me/notifications/:id`, `POST /me/notifications/:id/restore` |
| | `prefs`, `updatePrefs` | `GET/PUT /me/notification-prefs` (PUT mengirim keenam saklar) |
| | `push` | **hapus**: notifikasi dibuat server (pesanan, selesai, pickup 30 menit, toko favorit upload menu baru) + event WebSocket `notification.created` |
| `CartRepository` | semuanya | tetap lokal (keranjang satu toko); kirim isinya saat quote/checkout |
| `OrderRepository` | `orders` | `GET /me/orders?tab=ongoing\|history&filter=all\|completed\|cancelled&cursor` |
| | `place(request)` | `POST /checkout/quote` (rincian + penawaran kurir) lalu `POST /orders` + `Idempotency-Key` + `expected_total`; lihat §4 |
| | `cancel(id)` | `POST /orders/:id/cancel`; tampilkan tombol hanya kalau `can_cancel` |
| | `rate(id, rating)` | `POST /orders/:id/review {stars, tags, comment, media_ids}`; tombol hanya kalau `can_rate` |
| | `complain(id, …)` | `POST /media (purpose=complaint)` lalu `POST /orders/:id/complaint {reason, detail, media_ids}`; tombol hanya kalau `can_complain` |
| | `markPickedUp(id)` | **hapus** (lihat §5.1) |
| `AddressRepository` | list/upsert/delete | `GET/POST /me/addresses`, `PATCH/DELETE /me/addresses/:id` (maks 20) |
| | `select`, `selectCurrentLocation` | `PUT /me/selected-address {address_id}` (`null` = lokasi GPS; koordinat GPS tidak disimpan server, kirim sebagai `lat`/`lng`) |
| `PaymentMethodRepository` | list, `link`, `unlink`, `setDefault` | `GET /me/payment-methods`, `POST` (`{kind: gopay\|ovo\|dana, phone}`), `DELETE /:kind`, `PUT /default {kind}`; setiap aksi mengembalikan daftar terbaru. **Tiruan** |
| `VoucherRepository` | `vouchers`, `claimed` | `GET /me/vouchers` → tiap voucher punya `status` `available\|claimed\|used`, `applies_to`, `image_url`, `merchant_name`, `terms`, `accent_hex` |
| | `claim(code)` | `POST /vouchers/:code/claim` (204; 409 sudah diklaim; 404 tidak tersedia) |
| | `consume(code)` | **hapus**: voucher otomatis terpakai saat `POST /orders {voucher_code}` dan dikembalikan saat batal |
| `ReviewRepository` | `forMerchant(id)` | `GET /merchants/:id/reviews?cursor` (nama penulis "Nama I."); histogram bintang dari `rating_breakdown` toko |
| | `add(review)` | hanya lewat `POST /orders/:id/review` (lihat §5.2) |
| `ChatRepository` | `messages` (per toko) | inbox: `GET /conversations?as=buyer\|merchant`; riwayat: `GET /conversations/:id/messages?before_id\|after_id` |
| | `send(merchantId, text, listingId, reply)` | `POST /conversations {merchant_id}` (ambil/buat) lalu `POST /conversations/:id/messages {client_id, body, listing_id}`. Parameter `reply` dan `ChatReplies.kt` dihapus: balasan nyata dari toko lewat WebSocket |
| | `replyAsStore`, `markReadByStore` | sama (dengan token mitra); `POST /conversations/:id/read {last_read_id}` |
| | `typing` | event WebSocket `typing` (kirim `{"type":"typing","conversation_id"}`) |
| | `BUYER_ME`, thread mitra per nama pembeli | thread dikenali dari `conversation.id` (`counterpart.id` = id pembeli) |
| `IncomingOrderRepository` | `orders` | `GET /me/merchant/orders?status=ready\|done&cursor` (`ready` = WAITING, `done` = HANDED_OVER/NO_SHOW) |
| | `confirmPickup(id, code)` | `POST /me/merchant/orders/:id/confirm {code}` → 200; 422 `wrong_code` (+`attempts_left`); 429 `too_many_attempts` |
| (belum ada) | daftar toko | `POST /merchants` (nama, alamat, `lat`, `lng`, …) → toko `pending` sampai admin setuju; lihat §5.5 |
| (belum ada) | dashboard mitra | `GET /me/merchant/dashboard` (`sales_today, commission_today, net_today, completed_today, pending_orders, active_listings`, hari ini WIB). `MerchantHome` menghitung sendiri dari `IncomingOrder`; sebaiknya pakai ini |

## 3. Pemetaan model

**Listing** — `id`, `merchantId`→`merchant.id`, `title`, `tier`, `priceOriginal`→`price_original`, `priceDiscounted`→`price`, `isFree`→`is_free`,
`pickupEnd`→`pickup_end`, `pickupStart`→`pickup_start`, `imageUrl`→`cover_url` (kartu) / `photos[0].url` (detail), `gallery`→`photos[1..]`,
`distanceKm`→`distance_km`, `description`, `stock`, `unit` (`PORTION`→`portion`, `KILOGRAM`→`kg`), `halal`, `allergens`, `madeAt`→`made_at`, `storage`,
`safetyChecks`→`safety_checks`, `videoUrl`→`video_url`, `soldCount`→`sold_count`, `discountPercent`→`discount_pct` (rumus sama, bagi bulat).
Listing gratis boleh punya `price_original` (harga coret), `price` null.

**Merchant** — `isVerified`→`is_verified`, `location`→`location_label`, `rating`→`rating_avg`, `ratingCount`→`rating_count`, `bannerUrl`→`banner_url`,
`photos`→`photos[].url`, `about`, `address`, `openHours`→`open_hours`, `halal`, `allergens`, `todaysOffer`→`todays_offer`, `deliveryAvailable`→`delivery_available`,
`prepMinutes`→`prep_minutes`, `totalRescued`→`total_rescued`. Tambahan: `lat`, `lng`, `rating_breakdown`, `status`.

**Order** — `pickupCode`→`code` (hanya terlihat pembeli), `merchant` (`id,name,location_label,lat,lng`), `lines`→`items` (`listing_id,title,tier,unit,quantity,unit_price,unit_original_price,image_url`),
`payment`→`payment.kind` (+`status`, `provider`), `serviceFee`→`service_fee`, `voucherDiscount`→`voucher_discount`, `pickupBy`→`pickup_by` (null untuk pengiriman),
`delivery` (`courier, speed, fee, discount, payable, eta_min, eta_max`), `deliveryAddress`→`delivery_address`, `rating`, `complaint`, `canRate`/`canComplain`→`can_rate`/`can_complain`, tambahan `can_cancel`.
`total`, `subtotal`, `original_total`, `delivery_fee` dikirim server (jangan dihitung ulang di app); `savings` = `original_total - (subtotal - voucher_discount)`.

**Enum** — `OrderStatus`: `ready, completed, cancelled`, ditambah `no_show` ("Dibatalkan, tidak diambil") dan `expired`. `IncomingStatus`: `WAITING`=`ready`, `HANDED_OVER`=`completed`, `NO_SHOW`=`no_show`.
`ComplaintStatus`: `submitted, reviewing, resolved` + `rejected`. `MerchantStatus`: + `suspended`. `SafetyLevel`: `safe, caution, unsafe, not_for_humans`.
`Courier`: `gosend, grab, shopee, lalamove`. `DeliverySpeed`: `priority, standard, saver`. `PaymentKind`: `qris, gopay, ovo, dana, cash`. Selebihnya (tier, halal, allergen, storage, safety check, complaint reason, notification type) hanya huruf kecil.

## 4. Checkout

1. `POST /checkout/quote` dengan `{merchant_id, lines:[{listing_id, quantity}], voucher_code?, fulfillment: pickup|delivery, courier?, speed?, address_id?}`.
   Respons: `subtotal, voucher_discount, service_fee, delivery_fee, total, pickup_by, payment_kinds`, dan `delivery` (`available`, `reason`, `distance_km`, `address`, `options[]` urut ongkir, `selected`). Tampilkan `options` sebagai daftar kurir; `DeliveryPricing` lokal boleh dipakai untuk pratinjau, tetapi angka server yang berlaku.
2. `POST /orders` dengan isi sama + `payment_kind`, `note`, `expected_total` (total yang tampil) + header `Idempotency-Key`. Selisih harga → 409 `price_changed` berisi angka baru.
3. Pembayaran non-tunai **tiruan**: order langsung `payment.status = paid`, `provider = "dummy"`. Layar QRIS "Saya sudah bayar" cukup memanggil `POST /orders` setelah diketuk.
4. Total 0 selalu tunai. e-wallet harus ditautkan dulu (422 `payment_method_not_linked`). Pengiriman ditolak dengan 422 `delivery_unavailable` + `reason` (`too_far`, `merchant_pickup_only`, `address_required`, `disabled`).
5. Jarak pengiriman dihitung dari **alamat yang dipilih** ke toko (bukan `merchant.distanceKm`). Batas 15 km.

## 5. Perbedaan perilaku — app perlu menyesuaikan

1. **"Sudah saya ambil" (pembeli) dihapus.** Penyelesaian pesanan hanya oleh mitra yang mengetik kode pickup dari layar pembeli; kalau pembeli bisa menyelesaikan sendiri, kode tidak berarti. Event `order.created` ke mitra juga tidak membawa kode.
2. **Ulasan hanya dari pesanan selesai** (sekali per pesanan, maks 7 hari), bukan dari halaman toko tanpa pesanan. Pindahkan "Tulis ulasan" ke detail pesanan (`can_rate`). Halaman toko hanya menampilkan.
3. **Batal oleh pembeli** hanya saat `ready` dan sebelum `pickup_by` (pengiriman: selama `ready`). Pakai `can_cancel`; jangan tampilkan tombol kalau false. Mitra bisa batal dengan alasan wajib.
4. **Komplain** maks 24 jam setelah selesai (`can_complain`), sekali per pesanan. Status `rejected` baru; tambahkan labelnya.
5. **Daftar mitra**: register hanya membuat akun. Toko dibuat lewat `POST /merchants` (tambah form: nama, alamat, titik peta, jam buka, halal, alergen) dan baru bisa posting setelah admin menyetujui (`merchant.status_changed` / `GET /me/merchant`). Mode mitra aktif kalau `status = approved`.
6. **Register/login**: email valid, password ≥ 8 karakter, telepon 8–14 digit tanpa `+62`/`0` di depan (normalisasi `ProfileValidation.normalizePhone`), `account_type: buyer|merchant`. Login memakai **email** (kolom "email atau nomor HP" belum didukung). Rate limit login 5/menit/IP di produksi.
7. **Lokasi**: kirim `lat`/`lng` setiap memanggil `/home`, `/listings`, detail, `/merchants`, `/me/saved-merchants`. Tanpa itu `distance_km` null dan urutan "terdekat" tidak bermakna. Titik toko diisi saat `POST /merchants` / `PATCH /me/merchant`.
8. **Teks dari server tidak diterjemahkan** oleh `tr()`: notifikasi, `todays_offer`, voucher, `safety.headline/advice`. Teks `safety` sama persis dengan `FoodSafety.assess` (Indonesia).
9. **Nama penulis ulasan** dipendekkan server menjadi "Nama I." (privasi); "Dinda P." tetap sama.
10. **Chat mitra** dikenali dari `conversation.id`, bukan nama pembeli. Balasan otomatis (`ChatReplies`) dihapus; gunakan WebSocket untuk balasan nyata, `typing` untuk indikator.
11. **Upload**: foto/video diunggah dulu (`POST /media`, ≤ 8 MB foto, ≤ 30 MB / 30 detik video MP4), hasilnya `id` dipakai di body. Berat pakan/kompos dikirim sebagai kilogram bulat (`ceil`).
12. **Notifikasi** membawa `data.order_id` / `data.merchant_id` untuk deep link (belum dipakai di `NotificationsScreen`).
13. **Error**: petakan `error.code` ke pesan di app (daftar di `README.md` §6). `429` → minta coba lagi.
14. **Push (FCM)** belum aktif; daftarkan token lewat `PUT /me/devices` sekarang agar siap.
15. **Cleartext HTTP untuk uji lokal**: tambah `network_security_config` yang mengizinkan `10.0.2.2` (emulator → `http://10.0.2.2:3030`) di build debug. Produksi memakai HTTPS.
16. **Karbon** = estimasi `porsi × 0,9` (sama dengan app); faktor emisi belum diverifikasi.

## 6. Cara mengganti Fake → API

1. Tambah Retrofit/OkHttp + `kotlinx-serialization` (atau Moshi), dan interceptor: header `Authorization`, refresh otomatis saat 401 (satu refresh pada satu waktu), `Idempotency-Key` untuk `POST /orders` dan `POST /listings`.
2. Buat `Api*Repository` per interface (urutan yang disarankan: Auth → Listing → Voucher → Order/Cart/Address/Payment → Saved/Profile/Notification → Review → Chat + WebSocket → IncomingOrder).
3. Ganti `@Binds` satu per satu di `RepositoryModule.kt`; `StateFlow` tetap sebagai cache yang diisi dari hasil panggilan.
4. Jalankan backend lokal (`make reset-dev && go run ./cmd/api`), login `budi.santoso@gmail.com` / `Password123!`. Isinya sama dengan dummy: 13 toko, 39 listing, 4 pesanan Budi, voucher `HEMAT5K` terklaim, dst.

## 7. Dummy yang disalin dan perbedaannya

- **Disalin langsung dari kode Kotlin** (`make sync-android-seed`): merchant, listing (harga, stok, terjual, deskripsi, alergen, jam dibuat/penyimpanan, galeri), ulasan, voucher (teks syarat, warna), notifikasi, chat (+status baca), pesanan Budi, pesanan masuk Dapur Budi, alamat, metode bayar, toko favorit, profil, impact; 45 foto asli (`cmd/seed/img/`, lisensi lihat `CREDITS.txt`, demo saja).
- **Waktu** relatif terhadap saat seed dijalankan (jendela pickup 1–10 jam). `make seed-dev` menyegarkan waktu dan stok tanpa menghapus data; `make reset-dev` mengulang dari nol.
- **Beda dari dummy**: jarak dihitung dari koordinat nyata toko (bukan angka tetap); `rating`/`total_rescued` toko adalah angka dummy, ulasan baru mengubah rating menjadi rata-rata; `service_fee` pesanan seed 0 (seperti dummy); pesanan dengan rating (o2) muncul juga sebagai ulasan toko Warung Bu Sari; tiap ulasan dummy dibuatkan satu pesanan selesai atas nama penulisnya; ada satu toko tambahan `pending` (Dapur Nusantara) untuk uji admin.
- **Akun**: `budi.santoso@gmail.com` (pembeli + pemilik Dapur Budi), `owner-m1@example.com` … `owner-m12@example.com` (pemilik toko), `wati@example.com` (pending), semua `Password123!`; admin dari `.env`.
