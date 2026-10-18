import json, re, uuid, sys

NS = uuid.UUID("5a5a5a5a-1111-4222-8333-444455556666")
seed = lambda n: str(uuid.uuid5(NS, n))

ROLE_VAR = {"buyer": "buyer_token", "seller": "seller_token", "admin": "admin_token", "pending": "pending_token", "new": "new_token"}


def tests(status=None, extra=""):
    lines = []
    if status:
        lines.append(f"pm.test('status {status}', () => pm.response.to.have.status({status}));")
    if extra:
        lines.append(extra)
    return lines


def R(name, method, path, role=None, body=None, status=None, save="", pre="", desc="", headers=None, form=None):
    url_path = path.split("?")[0]
    req = {
        "method": method,
        "header": [{"key": k, "value": v} for k, v in (headers or {}).items()],
        "url": {
            "raw": "{{baseUrl}}" + path,
            "host": ["{{baseUrl}}"],
            "path": [p for p in url_path.split("/") if p],
        },
    }
    if "?" in path:
        req["url"]["query"] = [{"key": k, "value": v} for k, v in (q.split("=", 1) for q in path.split("?")[1].split("&"))]
    if body is not None:
        req["header"].append({"key": "Content-Type", "value": "application/json"})
        raw = json.dumps(body, indent=2, ensure_ascii=False)
        raw = re.sub(r'"@@(\{\{\w+\}\})@@"', r"\1", raw)  # angka tanpa tanda kutip
        req["body"] = {"mode": "raw", "raw": raw, "options": {"raw": {"language": "json"}}}
    if form is not None:
        req["body"] = {"mode": "formdata", "formdata": form}
    if role:
        req["auth"] = {"type": "bearer", "bearer": [{"key": "token", "value": "{{" + ROLE_VAR[role] + "}}", "type": "string"}]}
    else:
        req["auth"] = {"type": "noauth"}
    if desc:
        req["description"] = desc
    item = {"name": name, "request": req}
    events = []
    if pre:
        events.append({"listen": "prerequest", "script": {"type": "text/javascript", "exec": pre.strip().split("\n")}})
    t = tests(status, save.strip())
    if t:
        events.append({"listen": "test", "script": {"type": "text/javascript", "exec": t if isinstance(t, list) else [t]}})
    if events:
        item["event"] = events
    return item


def set_vars(**kv):
    """JS yang menyimpan nilai dari respons: nilai adalah ekspresi JS atas d (= response.data)."""
    out = ["const d = (pm.response.json() || {}).data;", "if (d) {"]
    for k, expr in kv.items():
        out.append(f"  try {{ const v = {expr}; if (v !== undefined && v !== null) pm.collectionVariables.set('{k}', v); }} catch (e) {{}}")
    out.append("}")
    return "\n".join(out)


def login_save(prefix):
    return set_vars(**{f"{prefix}_token": "d.access_token", f"{prefix}_refresh": "d.refresh_token"})


def file_field(key="file"):
    return {"key": key, "type": "file", "src": []}


def txt(k, v):
    return {"key": k, "value": v, "type": "text"}


def login(name, who, email, password, status=200):
    return R(name, "POST", "/api/v1/auth/login", body={"email": email, "password": password}, status=status, save=login_save(who),
             desc="Token tersimpan otomatis di variabel koleksi dan dipakai request lain.")


# -------------------------------------------------------------------------------------------------

# {{$guid}} Postman di-cache per run, jadi Idempotency-Key dibuat sendiri dengan JS.
new_idem = "pm.collectionVariables.set('{v}', 'pm-' + Date.now() + '-' + Math.random().toString(36).slice(2, 10));"
lat_lng = "lat={{lat}}&lng={{lng}}"

# "Nasi Kuning Sisa Katering" milik Warung Bu Sari (dipegang akun mitra `seller`): 2 x Rp8.000 memenuhi minimum voucher HEMAT5K,
# dan alur pesan -> konfirmasi mitra konsisten.
PICK = "([].concat(d.nearby, d.deals, d.popular).find(c => c.title === 'Nasi Kuning Sisa Katering') || d.nearby[0] || {})"
folders = []

folders.append(("00 · Cek Server", [
    R("Healthcheck", "GET", "/healthz", status=200, desc="Mengecek koneksi Postgres dan folder media bisa ditulis."),
]))

folders.append(("01 · Login (jalankan dulu)", [
    login("Login pembeli (Budi Santoso)", "buyer", "budi.santoso@gmail.com", "{{seed_password}}"),
    login("Login mitra (pemilik Warung Bu Sari, toko approved)", "seller", "owner-m1@example.com", "{{seed_password}}"),
    login("Login admin", "admin", "{{admin_email}}", "{{admin_password}}"),
    login("Login mitra toko pending (Wati)", "pending", "wati@example.com", "{{seed_password}}"),
    R("Login salah (401 invalid_credentials)", "POST", "/api/v1/auth/login", body={"email": "budi.santoso@gmail.com", "password": "salah-salah"}, status=401,
      desc="Pesan sama untuk email tak terdaftar dan password salah."),
]))

folders.append(("02 · Akun & Profil (pembeli)", [
    R("Profil saya", "GET", "/api/v1/me", "buyer", status=200),
    R("Ubah nama", "PATCH", "/api/v1/me", "buyer", body={"name": "Clara Tan"}, status=200),
    R("Ubah telepon tidak valid (400 validation_failed)", "PATCH", "/api/v1/me", "buyer", body={"phone": "+6281234"}, status=400),
    R("Register akun baru", "POST", "/api/v1/auth/register",
      body={"name": "Budi Baru", "email": "budi{{$timestamp}}@example.com", "phone": "81234567890", "password": "Password123!", "account_type": "buyer"},
      status=201, save=login_save("new"), desc="Email unik tiap kali dikirim. Token tersimpan sebagai new_token."),
    R("Refresh token (rotasi)", "POST", "/api/v1/auth/refresh", body={"refresh_token": "{{buyer_refresh}}"}, status=200,
      save=login_save("buyer"), desc="Refresh token lama hangus. Kirim token lama lagi = seluruh sesi dicabut (login ulang)."),
    R("Ganti password", "POST", "/api/v1/auth/password", "new", body={"current": "Password123!", "new": "PasswordBaru123!"}, status=200,
      desc="Dijalankan pada akun baru (new_token) supaya akun seed aman."),
    R("Lupa password (202, jawaban sama untuk email tak terdaftar)", "POST", "/api/v1/auth/forgot-password",
      body={"email": "tidak-terdaftar@example.com"}, status=202,
      desc="Kode 6 digit dikirim ke email (EMAIL_MODE=log: tercetak di log server). Respons tidak membocorkan apakah email terdaftar."),
    R("Reset password dengan kode salah (400 invalid_reset_code)", "POST", "/api/v1/auth/reset-password",
      body={"email": "tidak-terdaftar@example.com", "code": "000000", "new_password": "PasswordBaru123!"}, status=400),
    R("Logout", "POST", "/api/v1/auth/logout", "buyer", body={"refresh_token": "{{buyer_refresh}}"}, status=204,
      desc="Setelah ini jalankan lagi 'Login pembeli' di folder 01."),
]))

folders.append(("03 · Alamat & Foto", [
    R("Daftar alamat", "GET", "/api/v1/me/addresses", "buyer", status=200),
    R("Tambah alamat", "POST", "/api/v1/me/addresses", "buyer", status=201,
      body={"label": "Kos", "full_address": "Jl. Tebet Barat IV No. 9, Jakarta Selatan", "note": "Pagar hitam", "lat": -6.226, "lng": 106.853},
      save=set_vars(address_id="d.id")),
    R("Ubah alamat", "PATCH", "/api/v1/me/addresses/{{address_id}}", "buyer", body={"note": "Lantai 2"}, status=200),
    R("Pilih alamat aktif", "PUT", "/api/v1/me/selected-address", "buyer", body={"address_id": "{{address_id}}"}, status=204),
    R("Pakai lokasi GPS (null)", "PUT", "/api/v1/me/selected-address", "buyer", body={"address_id": None}, status=204),
    R("Hapus alamat", "DELETE", "/api/v1/me/addresses/{{address_id}}", "buyer", status=204),
    R("Upload foto profil", "POST", "/api/v1/media", "buyer", status=201,
      form=[txt("purpose", "avatar"), file_field()], save=set_vars(media_id="d.id"),
      desc="Pilih file JPEG/PNG/WebP (maks 8 MB) di kolom file. Hasil: url absolut yang bisa dibuka di browser."),
    R("Pasang foto profil", "PATCH", "/api/v1/me", "buyer", body={"photo_media_id": "{{media_id}}"}, status=200),
    R("Hapus foto profil", "PATCH", "/api/v1/me", "buyer", body={"photo_media_id": None}, status=200),
    R("Upload foto komplain (privat)", "POST", "/api/v1/media", "buyer", status=201,
      form=[txt("purpose", "complaint"), file_field()], save=set_vars(private_media_id="d.id")),
    R("Ambil file privat (pemilik)", "GET", "/api/v1/media/private/{{private_media_id}}", "buyer", status=200),
    R("Ambil file privat (orang lain → 404)", "GET", "/api/v1/media/private/{{private_media_id}}", "seller", status=404),
]))

folders.append(("04 · Jelajah (publik, tanpa login)", [
    R("Home tab makanan", "GET", "/api/v1/home?" + lat_lng + "&tab=human", status=200,
      save=set_vars(listing_id=PICK + ".id", merchant_id=PICK + ".merchant.id"),
      desc="Rail nearby/deals/popular + offer_merchants. Menyimpan listing_id dan merchant_id pertama untuk request berikutnya."),
    R("Home tab pakan & kompos", "GET", "/api/v1/home?" + lat_lng + "&tab=farm", status=200),
    R("Home dengan pencarian (q=roti)", "GET", "/api/v1/home?" + lat_lng + "&q=roti", status=200,
      desc="other_tab_match_count = jumlah hasil serupa di tab lain."),
    R("Daftar listing (halaman 1)", "GET", "/api/v1/listings?" + lat_lng + "&page=1&limit=5", status=200),
    R("Listing terdekat, radius 3 km", "GET", "/api/v1/listings?" + lat_lng + "&sort=nearest&max_km=3", status=200),
    R("Listing termurah", "GET", "/api/v1/listings?sort=cheapest", status=200),
    R("Listing diskon terbesar", "GET", "/api/v1/listings?sort=biggest_discount", status=200),
    R("Hanya yang gratis", "GET", "/api/v1/listings?free=true", status=200),
    R("Hanya pakan ternak", "GET", "/api/v1/listings?tier=animal_feed", status=200),
    R("Cari 'ayam'", "GET", "/api/v1/listings?q=ayam", status=200),
    R("Detail listing", "GET", "/api/v1/listings/{{listing_id}}?" + lat_lng, status=200,
      desc="Berisi status turunan, hasil safety (level/headline/advice), toko, dan more_from_merchant."),
    R("Halaman toko", "GET", "/api/v1/merchants/{{merchant_id}}?" + lat_lng, status=200),
    R("Toko pending tidak tampil publik (404)", "GET", "/api/v1/merchants/{{seed_merchant_pending}}", status=404),
]))

folders.append(("05 · Voucher", [
    R("Voucher saya & yang tersedia", "GET", "/api/v1/me/vouchers", "buyer", status=200,
      desc="status: available (belum diklaim), claimed, used."),
    R("Klaim SELAMAT20", "POST", "/api/v1/vouchers/SELAMAT20/claim", "buyer",
      save="pm.test('204 atau 409 (sudah diklaim)', () => pm.expect([204, 409]).to.include(pm.response.code));"),
    R("Klaim voucher yang sama lagi (409)", "POST", "/api/v1/vouchers/SELAMAT20/claim", "buyer", status=409),
    R("Klaim voucher tidak ada (404)", "POST", "/api/v1/vouchers/TIDAKADA/claim", "buyer", status=404),
]))

cart = {"merchant_id": "{{merchant_id}}", "lines": [{"listing_id": "{{listing_id}}", "quantity": 2}]}
order_body = lambda **kw: {**cart, "payment_kind": "cash", "note": "Tanpa sambal", "expected_total": "@@{{expected_total}}@@", **kw}
folders.append(("06 · Checkout & Pesanan (pembeli)", [
    R("Quote (rincian harga, tidak mengubah data)", "POST", "/api/v1/checkout/quote", "buyer", body=cart, status=200,
      save=set_vars(expected_total="d.total"), desc="Menyimpan total sebagai expected_total untuk request berikutnya."),
    R("Quote dengan voucher", "POST", "/api/v1/checkout/quote", "buyer", body={**cart, "voucher_code": "HEMAT5K"}, status=None,
      desc="HEMAT5K sudah diklaim di data awal (potongan Rp5.000, min. belanja Rp15.000). Voucher yang belum diklaim: 422 voucher_not_claimed."),
    R("Buat pesanan", "POST", "/api/v1/orders", "buyer", body=order_body(expected_total="@@{{expected_total}}@@"), status=201,
      headers={"Idempotency-Key": "{{idem_order}}"}, pre=new_idem.format(v="idem_order"),
      save=set_vars(order_id="d.id", order_code="d.code"),
      desc="expected_total dikirim sebagai string template; Postman menggantinya dengan angka. Idempotency-Key baru di setiap kirim."),
    R("Ulangi request yang sama (200, order yang sama)", "POST", "/api/v1/orders", "buyer", body=order_body(expected_total="@@{{expected_total}}@@"), status=200,
      headers={"Idempotency-Key": "{{idem_order}}"}, desc="Key sama dengan 'Buat pesanan' → order lama dikembalikan, stok tidak berkurang lagi."),
    R("Key sama, isi beda (422 idempotency_key_reused)", "POST", "/api/v1/orders", "buyer",
      body=order_body(lines=[{"listing_id": "{{listing_id}}", "quantity": 1}]), status=422, headers={"Idempotency-Key": "{{idem_order}}"},
      desc="Idempotency-Key yang sama dengan isi request berbeda ditolak, bukan mengembalikan order lama."),
    R("Tanpa expected_total (400 validation_failed)", "POST", "/api/v1/orders", "buyer",
      body={k: v for k, v in order_body().items() if k != "expected_total"}, status=400, headers={"Idempotency-Key": "{{idem_bad}}"},
      pre=new_idem.format(v="idem_bad"), desc="expected_total wajib: server membandingkan dengan harga hitungannya."),
    R("Tanpa Idempotency-Key (400)", "POST", "/api/v1/orders", "buyer", body=order_body(), status=400),
    R("Harga berubah (409 price_changed)", "POST", "/api/v1/orders", "buyer", body=order_body(expected_total=1), status=409,
      headers={"Idempotency-Key": "{{idem_bad}}"}, pre=new_idem.format(v="idem_bad"),
      desc="Respons error berisi total/service_fee terbaru."),
    R("Detail pesanan (pembeli melihat kode)", "GET", "/api/v1/orders/{{order_id}}", "buyer", status=200),
    R("Pesanan berjalan", "GET", "/api/v1/me/orders?tab=ongoing", "buyer", status=200),
    R("Batalkan pesanan", "POST", "/api/v1/orders/{{order_id}}/cancel", "buyer", body={"reason": "Berubah pikiran"}, status=200,
      desc="Hanya saat ready dan sebelum pickup_by. Stok dan voucher dikembalikan."),
    R("Batalkan lagi (409 invalid_order_state)", "POST", "/api/v1/orders/{{order_id}}/cancel", "buyer", status=409),
    R("Buat pesanan kedua (untuk tes mitra)", "POST", "/api/v1/orders", "buyer", body=order_body(), status=201,
      headers={"Idempotency-Key": "{{idem_order2}}"}, pre=new_idem.format(v="idem_order2"),
      save=set_vars(order_id_2="d.id", order_code_2="d.code")),
    R("Riwayat (semua)", "GET", "/api/v1/me/orders?tab=history&filter=all", "buyer", status=200),
    R("Riwayat batal", "GET", "/api/v1/me/orders?tab=history&filter=cancelled&limit=1", "buyer", status=200,
      desc="Cek meta.next_cursor lalu kirim sebagai ?cursor= untuk halaman berikutnya."),
]))

folders.append(("07 · Mitra: Pesanan & Dashboard (Warung Bu Sari)", [
    R("Dashboard hari ini", "GET", "/api/v1/me/merchant/dashboard", "seller", status=200),
    R("Pesanan masuk (menunggu)", "GET", "/api/v1/me/merchant/orders?status=ready", "seller", status=200,
      desc="Mitra tidak melihat kode pickup; kode diketik dari layar pembeli."),
    R("Konfirmasi dengan kode salah (422 wrong_code)", "POST", "/api/v1/me/merchant/orders/{{order_id_2}}/confirm", "seller", body={"code": "0000"}, status=422,
      desc="Respons error berisi attempts_left. 5 kode salah dalam 10 menit mengunci order itu (429 too_many_attempts)."),
    R("Konfirmasi dengan kode benar → selesai", "POST", "/api/v1/me/merchant/orders/{{order_id_2}}/confirm", "seller", body={"code": "{{order_code_2}}"}, status=200,
      desc="Kode boleh 4 digit, 'sg-1234', atau 'SG-1234'. Status jadi completed, pembayaran tunai paid."),
    R("Pesanan selesai", "GET", "/api/v1/me/merchant/orders?status=done", "seller", status=200),
    R("Dashboard setelah selesai", "GET", "/api/v1/me/merchant/dashboard", "seller", status=200),
    R("Buat pesanan ketiga (untuk batal mitra)", "POST", "/api/v1/orders", "buyer", body=order_body(), status=201,
      headers={"Idempotency-Key": "{{idem_order3}}"}, pre=new_idem.format(v="idem_order3"), save=set_vars(order_id_3="d.id")),
    R("Mitra membatalkan (alasan wajib)", "POST", "/api/v1/me/merchant/orders/{{order_id_3}}/cancel", "seller", body={"reason": "Bahan habis"}, status=200),
    R("Mitra membatalkan tanpa alasan (400)", "POST", "/api/v1/me/merchant/orders/{{order_id_3}}/cancel", "seller", body={}, status=400),
]))

folders.append(("08 · Mitra: Toko & Listing (Warung Bu Sari)", [
    R("Toko saya", "GET", "/api/v1/me/merchant", "seller", status=200),
    R("Ubah penawaran hari ini", "PATCH", "/api/v1/me/merchant", "seller", body={"todays_offer": "Es teler beli 2 gratis 1", "open_hours": "07.00 - 21.00"}, status=200),
    R("Hapus penawaran", "PATCH", "/api/v1/me/merchant", "seller", body={"todays_offer": ""}, status=200),
    R("Katalog saya", "GET", "/api/v1/me/merchant/listings?page=1&limit=20", "seller", status=200,
      desc="Status turunan: active, sold_out, expired, paused."),
    R("Upload foto listing", "POST", "/api/v1/media", "seller", status=201, form=[txt("purpose", "listing"), file_field()],
      save=set_vars(listing_media_id="d.id"), desc="Pilih file di kolom file. Satu foto hanya bisa dipakai satu listing."),
    R("Upload video listing (MP4 maks 30 detik / 30 MB)", "POST", "/api/v1/media", "seller", status=None, form=[txt("purpose", "listing"), file_field()],
      desc="Opsional. Pilih file MP4 di kolom file; hasilnya kind=video + duration_ms. Pakai id-nya sebagai video_media_id saat membuat listing. Lebih dari 30 detik: 422 video_too_long."),
    R("Buat listing makanan (human)", "POST", "/api/v1/listings", "seller", status=201,
      headers={"Idempotency-Key": "{{idem_listing}}"}, pre=new_idem.format(v="idem_listing") + "\npm.collectionVariables.set('made_at', new Date(Date.now() - 3600*1000).toISOString());",
      body={"title": "Nasi Goreng Spesial", "description": "Sisa katering siang", "tier": "human", "price": 15000, "price_original": 30000, "stock": 6,
            "made_at": "{{made_at}}", "storage": "room_temp", "safety_checks": ["untouched", "sensory_ok", "stored_right", "packed_clean"],
            "allergens": ["egg"], "pickup_hours": 2, "photo_media_ids": ["{{listing_media_id}}"]},
      save=set_vars(my_listing_id="d.id"), desc="made_at dibuat otomatis 1 jam lalu. pickup_end = yang lebih awal antara +2 jam dan batas aman (suhu ruang 4 jam)."),
    R("Ulangi request (200, listing yang sama)", "POST", "/api/v1/listings", "seller", status=200, headers={"Idempotency-Key": "{{idem_listing}}"},
      body={"title": "Nasi Goreng Spesial", "tier": "human", "price": 15000, "stock": 6, "made_at": "{{made_at}}", "storage": "room_temp",
            "safety_checks": ["untouched", "sensory_ok", "stored_right", "packed_clean"], "pickup_hours": 2, "photo_media_ids": ["{{listing_media_id}}"]}),
    R("Makanan lewat batas aman (422 past_safe_window)", "POST", "/api/v1/listings", "seller", status=422,
      headers={"Idempotency-Key": "{{idem_stale}}"}, pre=new_idem.format(v="idem_stale") + "\npm.collectionVariables.set('stale_made_at', new Date(Date.now() - 6*3600*1000).toISOString());",
      body={"title": "Nasi Kemarin", "tier": "human", "price": 5000, "stock": 2, "made_at": "{{stale_made_at}}", "storage": "room_temp",
            "safety_checks": ["untouched", "sensory_ok", "stored_right", "packed_clean"], "pickup_hours": 2, "photo_media_ids": ["{{listing_media_id}}"]},
      desc="Error berisi suggested_tier: animal_feed. Foto tidak terpakai karena listing ditolak."),
    R("Checklist tidak lengkap (422 checklist_incomplete)", "POST", "/api/v1/listings", "seller", status=422,
      headers={"Idempotency-Key": "{{idem_stale}}"},
      body={"title": "Nasi Tanpa Cek", "tier": "human", "price": 5000, "stock": 2, "made_at": "{{made_at}}", "safety_checks": ["untouched"], "pickup_hours": 2,
            "photo_media_ids": ["{{listing_media_id}}"]}, desc="Error berisi daftar checklist yang kurang (missing)."),
    R("Ubah stok & harga", "PATCH", "/api/v1/listings/{{my_listing_id}}", "seller", body={"stock": 3, "price": 14000}, status=200),
    R("Jeda listing", "PATCH", "/api/v1/listings/{{my_listing_id}}", "seller", body={"status": "paused"}, status=200),
    R("Aktifkan lagi", "PATCH", "/api/v1/listings/{{my_listing_id}}", "seller", body={"status": "active", "pickup_hours": 4}, status=200),
    R("Hapus listing (soft delete)", "DELETE", "/api/v1/listings/{{my_listing_id}}", "seller", status=204),
    R("Toko pending belum bisa posting (403)", "POST", "/api/v1/listings", "pending", status=403,
      headers={"Idempotency-Key": "{{idem_stale}}"},
      body={"title": "Coba Posting", "tier": "compost", "is_free": True, "stock": 1, "pickup_hours": 2, "photo_media_ids": ["{{listing_media_id}}"]}),
]))

folders.append(("09 · Daftar Mitra Baru → Disetujui Admin", [
    R("1. Register akun baru", "POST", "/api/v1/auth/register",
      body={"name": "Mitra Baru", "email": "mitra{{$timestamp}}@example.com", "phone": "81234567890", "password": "Password123!", "account_type": "merchant"},
      status=201, save=login_save("new")),
    R("2. Daftarkan toko (status pending)", "POST", "/api/v1/merchants", "new", status=201,
      body={"name": "Dapur Mitra Baru", "about": "Masakan rumahan", "address": "Jl. Cilandak KKO No. 1, Jakarta Selatan", "lat": -6.2905, "lng": 106.8004,
            "location_label": "Cilandak", "open_hours": "08.00 - 18.00", "halal": "unverified", "allergens": ["peanut"], "prep_minutes": 20},
      save=set_vars(new_merchant_id="d.id")),
    R("3. Daftar toko kedua ditolak (409)", "POST", "/api/v1/merchants", "new", status=409, body={"name": "Toko Dua", "address": "x", "lat": 1, "lng": 1}),
    R("4. Admin melihat antrean pending", "GET", "/api/v1/admin/merchants?status=pending", "admin", status=200),
    R("5. Non-admin tidak boleh menyetujui (403)", "POST", "/api/v1/admin/merchants/{{new_merchant_id}}/approve", "buyer", status=403),
    R("6. Admin menyetujui", "POST", "/api/v1/admin/merchants/{{new_merchant_id}}/approve", "admin", status=204),
    R("7. Pemilik menerima notifikasi", "GET", "/api/v1/me/notifications", "new", status=200),
    R("8. Toko kini tampil publik", "GET", "/api/v1/merchants/{{new_merchant_id}}", status=200),
]))

folders.append(("10 · Admin", [
    R("Semua toko", "GET", "/api/v1/admin/merchants", "admin", status=200),
    R("Antrean pending", "GET", "/api/v1/admin/merchants?status=pending&page=1&limit=10", "admin", status=200),
    R("Setujui toko seed (Dapur Nusantara)", "POST", "/api/v1/admin/merchants/{{seed_merchant_pending}}/approve", "admin", status=204),
    R("Tolak toko seed", "POST", "/api/v1/admin/merchants/{{seed_merchant_pending}}/reject", "admin", status=204),
]))

folders.append(("11 · Notifikasi", [
    R("Notifikasi pembeli", "GET", "/api/v1/me/notifications?limit=5", "buyer", status=200,
      save=set_vars(notification_id="d[0].id"), desc="meta.next_cursor dipakai sebagai ?cursor= untuk halaman berikutnya."),
    R("Filter tipe order", "GET", "/api/v1/me/notifications?type=order", "buyer", status=200),
    R("Tandai dibaca", "PATCH", "/api/v1/me/notifications/{{notification_id}}", "buyer", body={"read": True}, status=204),
    R("Tandai semua dibaca", "POST", "/api/v1/me/notifications/read-all", "buyer", status=204),
    R("Notifikasi mitra", "GET", "/api/v1/me/notifications", "seller", status=200),
]))


class _Js(str):
    def format(self, v):
        return self.replace("@V@", v)


new_uuid = _Js("pm.collectionVariables.set('@V@', 'xxxxxxxx-xxxx-4xxx-yxxx-xxxxxxxxxxxx'.replace(/[xy]/g, c => { const r = Math.random() * 16 | 0; return (c === 'x' ? r : (r & 0x3 | 0x8)).toString(16); }));")

folders.append(("12 · Ulasan & Komplain (setelah pesanan selesai)", [
    R("Detail pesanan selesai (reviewed=false)", "GET", "/api/v1/orders/{{order_id_2}}", "buyer", status=200,
      desc="Memakai order_id_2 dari folder 07 (sudah dikonfirmasi mitra). Jalankan folder 06 dan 07 dulu."),
    R("Beri ulasan", "POST", "/api/v1/orders/{{order_id_2}}/review", "buyer", status=201,
      body={"stars": 5, "tags": ["Porsi pas", "Cepat"], "comment": "Enak dan masih hangat."},
      desc="Sekali per order, maksimal 7 hari setelah selesai. Rating toko dihitung ulang di transaksi yang sama. media_ids (maks 3) berisi id hasil upload purpose=review."),
    R("Ulasan kedua ditolak (409 already_reviewed)", "POST", "/api/v1/orders/{{order_id_2}}/review", "buyer", body={"stars": 1}, status=409),
    R("Bintang di luar 1–5 (400)", "POST", "/api/v1/orders/{{order_id_2}}/review", "buyer", body={"stars": 9}, status=400),
    R("Mitra tidak bisa menilai order sendiri (404)", "POST", "/api/v1/orders/{{order_id_2}}/review", "seller", body={"stars": 5}, status=404),
    R("Pesanan yang sudah dibatalkan tidak bisa diulas (409)", "POST", "/api/v1/orders/{{order_id}}/review", "buyer", body={"stars": 5}, status=409,
      desc="order_id adalah pesanan yang dibatalkan di folder 06."),
    R("Ulasan toko (publik)", "GET", "/api/v1/merchants/{{merchant_id}}/reviews?limit=10", status=200,
      desc="Hanya nama depan pembeli yang tampil. Cursor: meta.next_cursor."),
    R("Kirim komplain", "POST", "/api/v1/orders/{{order_id_2}}/complaint", "buyer", status=201,
      body={"reason": "not_fresh", "detail": "Ayamnya terasa kurang segar saat dimakan."}, save=set_vars(complaint_id="d.id"),
      desc="Maksimal 24 jam setelah selesai, sekali per order. Foto privat (maks 3): upload dengan purpose=complaint lalu isi media_ids."),
    R("Komplain kedua ditolak (409 already_complained)", "POST", "/api/v1/orders/{{order_id_2}}/complaint", "buyer",
      body={"reason": "other", "detail": "Komplain kedua untuk pesanan yang sama."}, status=409),
    R("Detail komplain terlalu pendek (400)", "POST", "/api/v1/orders/{{order_id_2}}/complaint", "buyer", body={"reason": "other", "detail": "buruk"}, status=400),
    R("Mitra melihat notifikasi komplain", "GET", "/api/v1/me/notifications?limit=3", "seller", status=200),
    R("Admin: daftar komplain", "GET", "/api/v1/admin/complaints?status=submitted", "admin", status=200),
    R("Pembeli tidak boleh membuka daftar admin (403)", "GET", "/api/v1/admin/complaints", "buyer", status=403),
    R("Admin: tandai sedang ditinjau", "PATCH", "/api/v1/admin/complaints/{{complaint_id}}", "admin", body={"status": "reviewing"}, status=204),
    R("Admin: selesai tanpa catatan (400)", "PATCH", "/api/v1/admin/complaints/{{complaint_id}}", "admin", body={"status": "resolved"}, status=400),
    R("Admin: selesai dengan catatan", "PATCH", "/api/v1/admin/complaints/{{complaint_id}}", "admin",
      body={"status": "resolved", "resolution_note": "Toko sudah dihubungi dan berjanji memperbaiki."}, status=204,
      desc="Pembeli mendapat notifikasi. Fase 1 tunai: tidak ada refund otomatis."),
    R("Admin: ubah komplain yang sudah ditutup (409)", "PATCH", "/api/v1/admin/complaints/{{complaint_id}}", "admin", body={"status": "reviewing"}, status=409),
    R("Detail pesanan (reviewed=true, complaint_status=resolved)", "GET", "/api/v1/orders/{{order_id_2}}", "buyer", status=200,
      save="const d = pm.response.json().data;\npm.test('flags', () => { pm.expect(d.reviewed).to.eql(true); pm.expect(d.complaint_status).to.eql('resolved'); });"),
]))

folders.append(("13 · Dampak, Toko Favorit, Preferensi Notifikasi", [
    R("Dampak saya", "GET", "/api/v1/me/impact", "buyer", status=200,
      desc="completed_orders, portions, feed_compost_kg, saved_rupiah dari pesanan selesai. Angka karbon sengaja belum ada."),
    R("Simpan toko favorit", "PUT", "/api/v1/me/saved-merchants/{{merchant_id}}", "buyer", status=204, desc="Idempoten; dipakai juga untuk tombol Batalkan."),
    R("Simpan lagi (tetap 204)", "PUT", "/api/v1/me/saved-merchants/{{merchant_id}}", "buyer", status=204),
    R("Daftar toko favorit", "GET", "/api/v1/me/saved-merchants?" + lat_lng, "buyer", status=200),
    R("Halaman toko dengan login (is_saved=true)", "GET", "/api/v1/merchants/{{merchant_id}}", "buyer", status=200,
      save="pm.test('is_saved', () => pm.expect(pm.response.json().data.is_saved).to.eql(true));"),
    R("Halaman toko tanpa login (tanpa is_saved)", "GET", "/api/v1/merchants/{{merchant_id}}", status=200,
      save="pm.test('anonim', () => pm.expect(pm.response.json().data.is_saved).to.be.undefined);"),
    R("Hapus dari favorit", "DELETE", "/api/v1/me/saved-merchants/{{merchant_id}}", "buyer", status=204),
    R("Toko pending tidak bisa difavoritkan (404)", "PUT", "/api/v1/me/saved-merchants/{{seed_merchant_pending}}", "buyer", status=404,
      desc="Jalankan sebelum folder 10 menyetujui toko ini."),
    R("Preferensi notifikasi", "GET", "/api/v1/me/notification-prefs", "buyer", status=200),
    R("Ubah preferensi (matikan impact, nyalakan promo)", "PUT", "/api/v1/me/notification-prefs", "buyer", status=200,
      body={"enabled": True, "pickup": True, "order": True, "merchant": True, "promo": True, "impact": False},
      desc="PUT mengganti semua saklar, jadi semua field wajib. enabled=false mematikan semua notifikasi baru."),
    R("Preferensi tidak lengkap (400)", "PUT", "/api/v1/me/notification-prefs", "buyer", body={"enabled": True}, status=400),
    R("Ambil satu notifikasi", "GET", "/api/v1/me/notifications?limit=1", "buyer", status=200, save=set_vars(notification_id="d[0].id")),
    R("Hapus notifikasi (soft delete)", "DELETE", "/api/v1/me/notifications/{{notification_id}}", "buyer", status=204),
    R("Pulihkan notifikasi (Batalkan)", "POST", "/api/v1/me/notifications/{{notification_id}}/restore", "buyer", status=204),
    R("Pulihkan lagi (404, belum terhapus)", "POST", "/api/v1/me/notifications/{{notification_id}}/restore", "buyer", status=404),
]))

folders.append(("14 · Chat & Realtime", [
    R("Pembeli memulai percakapan dengan Warung Bu Sari", "POST", "/api/v1/conversations", "buyer", body={"merchant_id": "{{seed_merchant_m1}}"}, status=200,
      save=set_vars(conversation_id="d.id"), desc="Mengambil percakapan yang ada atau membuat baru. Hanya toko approved."),
    R("Mitra tidak bisa chat tokonya sendiri (403)", "POST", "/api/v1/conversations", "seller", body={"merchant_id": "{{seed_merchant_m1}}"}, status=403),
    R("Toko pending tidak bisa dichat (404)", "POST", "/api/v1/conversations", "buyer", body={"merchant_id": "{{seed_merchant_pending}}"}, status=404,
      desc="Jalankan sebelum folder 10 menyetujui toko ini."),
    R("Pembeli kirim pesan", "POST", "/api/v1/conversations/{{conversation_id}}/messages", "buyer", status=201,
      pre=new_uuid.format(v="client_id"),
      body={"client_id": "{{client_id}}", "body": "Kak, nasi ayamnya masih ada?", "listing_id": "{{listing_id}}"},
      save=set_vars(last_message_id="d.id"),
      desc="client_id dibuat HP; kirim ulang dengan client_id yang sama tidak membuat pesan ganda. listing_id (opsional) harus produk toko ini."),
    R("Kirim ulang client_id sama (200, pesan sama)", "POST", "/api/v1/conversations/{{conversation_id}}/messages", "buyer", status=200,
      body={"client_id": "{{client_id}}", "body": "Kak, nasi ayamnya masih ada?", "listing_id": "{{listing_id}}"}),
    R("Pesan kosong (400)", "POST", "/api/v1/conversations/{{conversation_id}}/messages", "buyer", pre=new_uuid.format(v="client_id_2"),
      body={"client_id": "{{client_id_2}}", "body": "   "}, status=400),
    R("Mitra: kotak masuk (ada unread)", "GET", "/api/v1/conversations?as=merchant", "seller", status=200,
      desc="Hanya percakapan yang sudah berisi pesan. unread = pesan lawan setelah last_read_id."),
    R("Mitra: baca pesan", "GET", "/api/v1/conversations/{{conversation_id}}/messages", "seller", status=200,
      desc="Urut naik. ?before_id= untuk scroll ke atas, ?after_id= untuk sinkron setelah reconnect. meta.peer_last_read_id = tanda baca lawan."),
    R("Mitra membalas", "POST", "/api/v1/conversations/{{conversation_id}}/messages", "seller", pre=new_uuid.format(v="client_id_2"),
      body={"client_id": "{{client_id_2}}", "body": "Masih ada kak, silakan dipesan."}, status=201),
    R("Mitra: tandai dibaca", "POST", "/api/v1/conversations/{{conversation_id}}/read", "seller", body={"last_read_id": "@@{{last_message_id}}@@"}, status=200,
      desc="Penanda hanya maju dan tidak melewati pesan terakhir. Pembeli diberi tahu lewat event conversation.read."),
    R("Pembeli: kotak masuk (1 balasan belum dibaca)", "GET", "/api/v1/conversations?as=buyer", "buyer", status=200),
    R("Pembeli: sinkron pesan setelah id", "GET", "/api/v1/conversations/{{conversation_id}}/messages?after_id={{last_message_id}}", "buyer", status=200),
    R("Orang luar tidak bisa membaca (404)", "GET", "/api/v1/conversations/{{conversation_id}}/messages", "admin", status=404),
    R("Tiket WebSocket (berlaku 30 detik, sekali pakai)", "POST", "/api/v1/ws/ticket", "buyer", status=200, save=set_vars(ws_ticket="d.ticket"),
      desc="Sambungkan: New → WebSocket di Postman, URL ws://localhost:3030/ws?ticket={{ws_ticket}}. Lalu kirim pesan lewat folder ini dari akun lain: event message.created masuk. Tiket yang sama tidak bisa dipakai dua kali."),
]))


folders.append(("15 · Admin Voucher & Token Perangkat", [
    R("Admin buat voucher persen", "POST", "/api/v1/admin/vouchers", "admin", status=201,
      pre="pm.collectionVariables.set('voucher_code', 'PM' + Math.random().toString(36).slice(2, 8).toUpperCase());\npm.collectionVariables.set('voucher_expires', new Date(Date.now() + 7*24*3600*1000).toISOString());",
      body={"code": "{{voucher_code}}", "title": "Hemat 15% (uji)", "description": "Voucher dari Postman", "percent_off": 15, "max_discount": 10000,
            "min_spend": 20000, "quota": 5, "expires_at": "{{voucher_expires}}", "terms": ["Tunai saja"], "accent_hex": "#55B931"},
      desc="Tepat satu dari percent_off / flat_off. max_discount hanya untuk persen. merchant_id dan applies_to_tier opsional (voucher toko = ditanggung mitra)."),
    R("Kode sama lagi (409 voucher_code_taken)", "POST", "/api/v1/admin/vouchers", "admin", status=409,
      body={"code": "{{voucher_code}}", "title": "Dobel", "flat_off": 1000, "expires_at": "{{voucher_expires}}"}),
    R("Persen dan flat sekaligus (400)", "POST", "/api/v1/admin/vouchers", "admin", status=400,
      body={"code": "SALAHDUA", "title": "Salah", "percent_off": 10, "flat_off": 1000, "expires_at": "{{voucher_expires}}"}),
    R("Non-admin ditolak (403)", "POST", "/api/v1/admin/vouchers", "buyer", status=403,
      body={"code": "NOPE1234", "title": "x", "flat_off": 1000, "expires_at": "{{voucher_expires}}"}),
    R("Admin daftar voucher", "GET", "/api/v1/admin/vouchers?page=1&limit=10", "admin", status=200),
    R("Pembeli klaim voucher baru", "POST", "/api/v1/vouchers/{{voucher_code}}/claim", "buyer", status=204),
    R("Admin ubah judul & kuota", "PATCH", "/api/v1/admin/vouchers/{{voucher_code}}", "admin", status=200,
      body={"title": "Hemat 15% (diubah)", "quota": 10}, desc="Jenis dan besar diskon tidak bisa diubah setelah dibuat. Kuota tidak boleh di bawah jumlah terpakai."),
    R("Admin nonaktifkan", "PATCH", "/api/v1/admin/vouchers/{{voucher_code}}", "admin", body={"is_active": False}, status=200),
    R("Klaim voucher nonaktif oleh akun lain (404)", "POST", "/api/v1/vouchers/{{voucher_code}}/claim", "seller", status=404),
    R("Daftarkan token FCM", "PUT", "/api/v1/me/devices", "buyer", status=204,
      body={"token": "postman-token-postman-token-0001", "platform": "android"}, desc="Idempoten. Pengiriman push belum aktif (Fase 2)."),
    R("Hapus token FCM (saat logout)", "DELETE", "/api/v1/me/devices/postman-token-postman-token-0001", "buyer", status=204),
]))


addr_save = "const d = (pm.response.json() || {}).data || [];\nconst k = d.find(a => a.label === 'Kantor'), r = d.find(a => a.label === 'Rumah');\nif (k) pm.collectionVariables.set('address_kantor_id', k.id);\nif (r) pm.collectionVariables.set('address_rumah_id', r.id);"
ship = lambda **kw: {**cart, "fulfillment": "delivery", "courier": "gosend", "speed": "standard", "address_id": "{{address_kantor_id}}", **kw}
folders.append(("16 · Pengiriman & Pembayaran Tiruan (PARTNERS_MODE=dummy)", [
    R("Alamat saya (Rumah + Kantor)", "GET", "/api/v1/me/addresses", "buyer", status=200, save=addr_save,
      desc="Dari data awal: Rumah (Tangerang) dan Kantor (Kebon Jeruk). Pengiriman dibatasi 15 km dari toko: dari Kantor ke Warung Bu Sari (Kemang) ±12 km masih bisa."),
    R("Quote pengiriman GoSend Standar", "POST", "/api/v1/checkout/quote", "buyer", body=ship(), status=200,
      save=set_vars(expected_total_ship="d.total") + "\npm.test('ada 4 kurir', () => pm.expect(pm.response.json().data.delivery.options.length).to.eql(4));",
      desc="delivery.options = 4 kurir termurah dulu; delivery.selected = pilihan; total = makanan + biaya layanan + ongkir. Tarif dan promo kurir memakai rumus TIRUAN yang sama dengan app Android."),
    R("Quote kecepatan Hemat (lebih murah, lebih lama)", "POST", "/api/v1/checkout/quote", "buyer", body=ship(speed="saver", courier="grab"), status=200),
    R("Quote pengiriman dari alamat Rumah terlalu jauh (422)", "POST", "/api/v1/checkout/quote", "buyer", body=ship(address_id="{{address_rumah_id}}"), status=422,
      desc="error.reason = too_far (maks 15 km). Alasan lain: merchant_pickup_only, address_required, disabled."),
    R("Pesan kirim + bayar QRIS (tiruan)", "POST", "/api/v1/orders", "buyer", status=201,
      body={**ship(), "payment_kind": "qris", "expected_total": "@@{{expected_total_ship}}@@"}, headers={"Idempotency-Key": "{{idem_ship}}"},
      pre=new_idem.format(v="idem_ship"), save=set_vars(order_id_ship="d.id") + "\nconst o = pm.response.json().data;\npm.test('dibayar tiruan', () => { pm.expect(o.payment.status).to.eql('paid'); pm.expect(o.payment.provider).to.eql('dummy'); pm.expect(o.fulfillment).to.eql('delivery'); });",
      desc="Tidak ada uang yang bergerak: pembayaran non-tunai langsung berstatus paid dengan provider 'dummy'. Order pengiriman tidak punya pickup_by."),
    R("Detail pesanan kirim", "GET", "/api/v1/orders/{{order_id_ship}}", "buyer", status=200,
      desc="delivery {courier, speed, fee, discount, payable, eta_min, eta_max}, delivery_address, can_cancel."),
    R("Mitra melihat pesanan kirim", "GET", "/api/v1/me/merchant/orders?status=ready", "seller", status=200),
    R("Batalkan -> pembayaran dikembalikan (refunded)", "POST", "/api/v1/orders/{{order_id_ship}}/cancel", "buyer", body={"reason": "Uji refund"}, status=200,
      save="pm.test('refunded', () => pm.expect(pm.response.json().data.payment.status).to.eql('refunded'));"),
    R("Quote keranjang tunai (untuk pesanan OVO)", "POST", "/api/v1/checkout/quote", "buyer", body=cart, status=200,
      save=set_vars(expected_total_cart="d.total")),
    R("e-wallet OVO belum ditautkan (422)", "POST", "/api/v1/orders", "buyer", status=422, headers={"Idempotency-Key": "{{idem_ship2}}"}, pre=new_idem.format(v="idem_ship2"),
      body={**cart, "payment_kind": "ovo", "expected_total": "@@{{expected_total_cart}}@@"}, desc="error.code = payment_method_not_linked. QRIS dan tunai selalu tersedia; GoPay/OVO/DANA harus ditautkan dulu."),
    R("Metode pembayaran saya", "GET", "/api/v1/me/payment-methods", "buyer", status=200),
    R("Tautkan OVO", "POST", "/api/v1/me/payment-methods", "buyer", body={"kind": "ovo", "phone": "081298765432"}, status=201,
      desc="Tiruan: tidak ada verifikasi ke OVO. Nomor disamarkan 0812 •••• 5432. Respons = daftar metode terbaru."),
    R("Jadikan OVO metode utama", "PUT", "/api/v1/me/payment-methods/default", "buyer", body={"kind": "ovo"}, status=200),
    R("Pesan dengan OVO (setelah ditautkan)", "POST", "/api/v1/orders", "buyer", status=201, headers={"Idempotency-Key": "{{idem_ship2}}"},
      body={**cart, "payment_kind": "ovo", "expected_total": "@@{{expected_total_cart}}@@"}, save=set_vars(order_id_ovo="d.id")),
    R("Lepas OVO (default kembali ke QRIS)", "DELETE", "/api/v1/me/payment-methods/ovo", "buyer", status=200,
      save="pm.test('QRIS default', () => pm.expect(pm.response.json().data.find(m => m.is_default).kind).to.eql('qris'));"),
    R("QRIS tidak bisa dilepas (422)", "DELETE", "/api/v1/me/payment-methods/qris", "buyer", status=422),
]))

# Alur cepat: seluruh jalur utama MVP dengan akun seed, tanpa upload file; cocok dijalankan dengan Collection Runner.
fast = [
    login("1. Login pembeli", "buyer", "budi.santoso@gmail.com", "{{seed_password}}"),
    login("2. Login mitra", "seller", "owner-m1@example.com", "{{seed_password}}"),
    R("3. Home → ambil listing pertama", "GET", "/api/v1/home?" + lat_lng + "&tab=human", status=200,
      save=set_vars(listing_id=PICK + ".id", merchant_id=PICK + ".merchant.id") +
      "\npm.test('ada listing di feed', () => pm.expect(pm.collectionVariables.get('listing_id')).to.be.a('string'));"),
    R("4. Klaim voucher SELAMAT20", "POST", "/api/v1/vouchers/SELAMAT20/claim", "buyer",
      save="pm.test('204 atau 409', () => pm.expect([204, 409]).to.include(pm.response.code));"),
    R("5. Quote dengan voucher", "POST", "/api/v1/checkout/quote", "buyer", body={**cart, "voucher_code": "HEMAT5K"}, status=200,
      save=set_vars(expected_total="d.total")),
    R("6. Buat pesanan", "POST", "/api/v1/orders", "buyer", body=order_body(voucher_code="HEMAT5K", expected_total="@@{{expected_total}}@@"), status=201,
      headers={"Idempotency-Key": "{{idem_order}}"}, pre=new_idem.format(v="idem_order"),
      save=set_vars(order_id="d.id", order_code="d.code") + "\npm.test('kode pickup', () => pm.expect(pm.collectionVariables.get('order_code')).to.match(/^SG-\\d{4}$/));"),
    R("7. Mitra melihat pesanan masuk", "GET", "/api/v1/me/merchant/orders?status=ready", "seller", status=200,
      save="pm.test('pesanan ada', () => pm.expect(pm.response.json().data.length).to.be.above(0));"),
    R("8. Kode salah ditolak", "POST", "/api/v1/me/merchant/orders/{{order_id}}/confirm", "seller", body={"code": "0000"}, status=422),
    R("9. Kode benar → selesai", "POST", "/api/v1/me/merchant/orders/{{order_id}}/confirm", "seller", body={"code": "{{order_code}}"}, status=200,
      save="pm.test('completed', () => pm.expect(pm.response.json().data.status).to.eql('completed'));"),
    R("10. Dashboard mitra", "GET", "/api/v1/me/merchant/dashboard", "seller", status=200),
    R("11. Riwayat pembeli", "GET", "/api/v1/me/orders?tab=history&filter=completed", "buyer", status=200),
    R("12. Notifikasi pembeli", "GET", "/api/v1/me/notifications", "buyer", status=200),
    R("13. Beri ulasan", "POST", "/api/v1/orders/{{order_id}}/review", "buyer", body={"stars": 5, "tags": ["Porsi pas"], "comment": "Enak!"}, status=201),
    R("14. Dampak pembeli", "GET", "/api/v1/me/impact", "buyer", status=200,
      save="pm.test('ada order selesai', () => pm.expect(pm.response.json().data.completed_orders).to.be.above(0));"),
    R("15. Mulai chat dengan toko", "POST", "/api/v1/conversations", "buyer", body={"merchant_id": "{{seed_merchant_m1}}"}, status=200,
      save=set_vars(conversation_id="d.id")),
    R("16. Kirim pesan chat", "POST", "/api/v1/conversations/{{conversation_id}}/messages", "buyer", pre=new_uuid.format(v="client_id"),
      body={"client_id": "{{client_id}}", "body": "Terima kasih, enak sekali!"}, status=201),
    R("17. Mitra melihat chat masuk", "GET", "/api/v1/conversations?as=merchant", "seller", status=200,
      save="pm.test('ada percakapan', () => pm.expect(pm.response.json().data.length).to.be.above(0));"),
]
folders.insert(0, ("★ Alur Cepat MVP (jalankan dengan Runner)", fast))

variables = {
    "baseUrl": "http://localhost:3030",
    "seed_password": "Password123!",
    "admin_email": "fadhlannur007@gmail.com",
    "admin_password": "ISI_DARI_.env_ADMIN_PASSWORD",
    "lat": "-6.2275",
    "lng": "106.6544",
    "merchant_id": seed("merchant:m1"),
    "listing_id": seed("listing:l1"),
    "seed_merchant_pending": seed("merchant:m13"),
    "seed_merchant_m1": seed("merchant:m1"),
}
for k in ["buyer_token", "buyer_refresh", "seller_token", "seller_refresh", "admin_token", "pending_token", "new_token", "new_refresh", "pending_refresh", "admin_refresh",
          "expected_total", "order_id", "order_code", "order_id_2", "order_code_2", "order_id_3", "media_id", "private_media_id", "address_id",
          "listing_media_id", "my_listing_id", "new_merchant_id", "notification_id", "idem_order", "idem_order2", "idem_order3", "idem_bad", "idem_listing",
          "idem_stale", "made_at", "stale_made_at", "address_kantor_id", "address_rumah_id", "expected_total_ship", "expected_total_cart", "order_id_ship", "order_id_ovo", "idem_ship", "idem_ship2", "voucher_code", "voucher_expires", "complaint_id", "conversation_id", "client_id", "client_id_2", "last_message_id", "ws_ticket"]:
    variables.setdefault(k, "")

col = {
    "info": {"name": "SisaGuna API (MVP)", "schema": "https://schema.getpostman.com/json/collection/v2.1.0/collection.json",
             "description": "Baca apiendpoints/README.md untuk panduan lengkap. Jalankan 'make reset-dev' sebelum mulai, lalu 'go run ./cmd/api'."},
    "variable": [{"key": k, "value": v} for k, v in variables.items()],
    "item": [{"name": n, "item": items} for n, items in folders],
}
json.dump(col, open(sys.argv[1], "w"), indent=2, ensure_ascii=False)
print("ok", sum(len(i) for _, i in folders), "requests")
