#!/usr/bin/env python3
"""Menyalin data dummy app Android (Fake*Repository) menjadi cmd/seed/data/*.json.

Dibaca langsung dari kode Kotlin, bukan diketik ulang, supaya seed backend selalu sama dengan dummy
yang dilihat tim Android. Waktu disimpan sebagai selisih menit dari "sekarang" ({"min": -55}).
Gagal keras (exit 1) kalau jumlah item tidak sesuai harapan, artinya bentuk kode Kotlin berubah.

Pakai: python3 scripts/extract_android_seed.py [folder SisaGuna] (default: ./SisaGuna)
"""
import json
import re
import sys
from pathlib import Path

ANDROID = Path(sys.argv[1] if len(sys.argv) > 1 else "SisaGuna")
SRC = ANDROID / "app/src/main/java/com/sisaguna/android/data/repository"
IMG = ANDROID / "app/src/main/assets/img"
OUT = Path(__file__).resolve().parent.parent / "cmd/seed/data"

EXPECTED = {"merchants": 13, "listings": 39, "reviews": 59, "vouchers": 4, "notifications": 8, "chat": 9,
            "orders": 4, "incoming": 4, "addresses": 2, "payment_methods": 3, "saved": 4}


def fail(msg):
    print("GAGAL:", msg, file=sys.stderr)
    sys.exit(1)


# ---------- parser Kotlin minimal: panggilan fungsi, argumen bernama, literal ----------

def split_top(s):
    """Memecah argumen di koma tingkat atas (abaikan koma di dalam kurung dan string)."""
    parts, depth, cur, i, in_str = [], 0, [], 0, False
    while i < len(s):
        c = s[i]
        if in_str:
            cur.append(c)
            if c == "\\":
                cur.append(s[i + 1])
                i += 1
            elif c == '"':
                in_str = False
        elif c == '"':
            in_str = True
            cur.append(c)
        elif c in "([{":
            depth += 1
            cur.append(c)
        elif c in ")]}":
            depth -= 1
            cur.append(c)
        elif c == "," and depth == 0:
            parts.append("".join(cur).strip())
            cur = []
        else:
            cur.append(c)
        i += 1
    tail = "".join(cur).strip()
    if tail:
        parts.append(tail)
    return parts


def calls(src, name):
    """Isi kurung dari setiap `name(...)` (bukan didahului huruf, titik, atau 'fun ')."""
    out = []
    for m in re.finditer(r"(?<![\w.])" + re.escape(name) + r"\(", src):
        if src[max(0, m.start() - 4):m.start()] == "fun ":
            continue
        i, depth, in_str = m.end(), 1, False
        while depth:
            c = src[i]
            if in_str:
                if c == "\\":
                    i += 1
                elif c == '"':
                    in_str = False
            elif c == '"':
                in_str = True
            elif c == "(":
                depth += 1
            elif c == ")":
                depth -= 1
            i += 1
        body = src[m.end():i - 1]
        if not re.search(r"\bval \w+\s*:", body):  # lewati deklarasi data class
            out.append(body)
    return out


def unescape(body):
    out, i = [], 0
    while i < len(body):
        c = body[i]
        if c == "\\":
            n = body[i + 1]
            if n == "u":
                out.append(chr(int(body[i + 2:i + 6], 16)))
                i += 6
                continue
            out.append({"n": "\n", "t": "\t", '"': '"', "\\": "\\", "$": "$"}[n])
            i += 2
            continue
        if c == "$":
            fail("string template ($) tidak didukung: " + body[:40])
        out.append(c)
        i += 1
    return "".join(out)


def ev(expr):
    e = expr.strip()
    if e.startswith('"') and e.endswith('"') and e.count('"') >= 2 and not re.search(r'"\s*[+]\s*"', e):
        return unescape(e[1:-1])
    if e == "null":
        return None
    if e in ("true", "false"):
        return e == "true"
    if e == "ListingRepository.MY_MERCHANT_ID" or e == "myStore":
        return "m0"
    m = re.fullmatch(r'seedImage\("([\w]+)"\)', e)
    if m:
        return m.group(1)
    if e in ("emptySet()", "emptyList()"):
        return []
    m = re.fullmatch(r'seedMerchant\("(\w+)"\)', e)
    if m:
        return m.group(1)
    m = re.fullmatch(r"OrderLine\((.*)\)", e, re.S)
    if m:
        return args(m.group(1), ["listing_id", "title", "tier", "image", "quantity", "unit_price", "unit_original_price"])
    m = re.fullmatch(r"OrderRating\((.*)\)", e, re.S)
    if m:
        return args(m.group(1), ["stars", "tags", "comment"])
    m = re.fullmatch(r"(?:setOf|listOf|linkedSetOf)\((.*)\)", e, re.S)
    if m:
        return [ev(p) for p in split_top(m.group(1))]
    m = re.fullmatch(r"(?:ago|inMin|inMinutes)\((.+?)(?:,\s*now)?\)", e)
    if m:
        v = num(m.group(1))
        return {"min": -v if e.startswith("ago") else v}
    m = re.fullmatch(r"daysAgo\((.+)\)", e)
    if m:
        return {"min": -num(m.group(1)) * 1440}
    m = re.fullmatch(r"now\.plus\((.+?),\s*ChronoUnit\.DAYS\)", e)
    if m:
        return {"min": num(m.group(1)) * 1440}
    m = re.fullmatch(r"YearMonth\.of\((\d+),\s*(\d+)\)", e)
    if m:
        return f"{m.group(1)}-{int(m.group(2)):02d}"
    if re.fullmatch(r"0x[0-9A-Fa-f]+", e):
        return int(e, 16)
    if re.fullmatch(r"[\d_.\s*+\-()]+", e):
        return num(e)
    m = re.fullmatch(r"(?:[A-Za-z]+\.)?([A-Z][A-Z_]+)", e)  # StorageMethod.SHELF, EGG, ANIMAL_FEED
    if m:
        return m.group(1).lower()
    fail("ekspresi tidak dikenal: " + e[:80])


def num(s):
    s = s.replace("_", "")
    if not re.fullmatch(r"[\d.\s*+\-()]+", s):
        fail("angka tidak valid: " + s)
    v = eval(s)  # hanya digit dan operator (divalidasi di atas)
    return int(v) if float(v).is_integer() and "." not in s else v


def args(argstr, positional):
    """Argumen -> dict. `positional` memberi nama untuk argumen tanpa nama (urut)."""
    out, pos = {}, 0
    for a in split_top(argstr):
        m = re.match(r"^([A-Za-z_]\w*)\s*=\s*(?!=)(.*)$", a, re.S)
        if m:
            out[m.group(1)] = ev(m.group(2))
        else:
            out[positional[pos]] = ev(a)
            pos += 1
    return out


def read(name):
    return (SRC / name).read_text()


# ---------- ekstraksi per sumber ----------

def merchants():
    pos = ["id", "name"]
    res = []
    for a in calls(read("FakeListingRepository.kt"), "Merchant"):
        d = args(a, pos)
        res.append({
            "id": d["id"], "name": d["name"], "verified": d["isVerified"], "status": d["status"].lower(), "location": d["location"],
            "rating": d.get("rating"), "rating_count": d.get("ratingCount", 0), "banner": d.get("bannerUrl", ""), "photos": d.get("photos", []),
            "about": d.get("about", ""), "address": d.get("address", ""), "open_hours": d.get("openHours", ""), "halal": d.get("halal", "unverified"),
            "allergens": d.get("allergens", []), "todays_offer": d.get("todaysOffer"), "delivery_available": d.get("deliveryAvailable", True),
            "prep_minutes": d.get("prepMinutes", 15), "distance_km": d.get("distanceKm"), "total_rescued": d.get("totalRescued", 0),
        })
    return res


def listings(merchant_halal):
    src = read("FakeListingRepository.kt")
    # urutan di file = urutan daftar; gabungkan human() dan byWeight() menurut posisi
    found = []
    for name in ("human", "byWeight"):
        for m in re.finditer(r"(?<![\w.])" + name + r"\(\n", src):
            if src[max(0, m.start() - 4):m.start()] != "fun ":  # lewati definisi fungsi
                found.append((m.start(), name))
    found.sort()
    bodies = {}
    for name in ("human", "byWeight"):
        for a in calls(src, name):
            bodies.setdefault(name, []).append(a)
    idx = {"human": 0, "byWeight": 0}
    res = []
    for _, name in found:
        a = bodies[name][idx[name]]
        idx[name] += 1
        if name == "human":
            d = args(a, ["id", "merchantId", "title", "original", "discounted", "image", "madeHoursAgo", "storage", "pickupEndIn",
                         "description", "stock", "allergens"])
            res.append({"id": d["id"], "merchant_id": d["merchantId"], "title": d["title"], "tier": "human", "original": d["original"],
                        "price": d["discounted"], "is_free": d["discounted"] is None, "image": d["image"], "gallery": d.get("gallery", []),
                        "made_hours_ago": d["madeHoursAgo"], "storage": d["storage"], "pickup_end_in_h": d["pickupEndIn"], "pickup_start_in_h": -0.5,
                        "description": d["description"], "stock": d["stock"], "allergens": d["allergens"], "sold": d.get("sold", 0),
                        "halal": d.get("halal") or merchant_halal[d["merchantId"]], "safety_checks": ["untouched", "sensory_ok", "stored_right", "packed_clean", "hygiene"]})
        else:
            d = args(a, ["id", "merchantId", "title", "tier", "original", "price", "image", "pickupEndIn", "description", "stock"])
            res.append({"id": d["id"], "merchant_id": d["merchantId"], "title": d["title"], "tier": d["tier"], "original": d["original"],
                        "price": d["price"], "is_free": d["price"] is None, "image": d["image"], "gallery": d.get("gallery", []),
                        "made_hours_ago": 5, "storage": "room_temp", "pickup_end_in_h": d["pickupEndIn"], "pickup_start_in_h": -3,
                        "description": d["description"], "stock": d["stock"], "allergens": d.get("allergens", []), "sold": d.get("sold", 0),
                        "halal": "other", "safety_checks": ["untouched", "sensory_ok", "stored_right", "packed_clean"]})
    return res


def reviews():
    res = []
    for a in calls(read("ReviewRepository.kt"), "Review"):
        d = args(a, ["id", "merchantId", "author", "stars", "comment", "createdAt", "photos", "itemTitle", "tags"])
        res.append({"id": d["id"], "merchant_id": d["merchantId"], "author": d["author"], "stars": d["stars"], "comment": d["comment"],
                    "at": d["createdAt"], "photos": d["photos"], "item_title": d["itemTitle"], "tags": d.get("tags", [])})
    return res


def vouchers():
    res = []
    for a in calls(read("VoucherRepository.kt"), "Voucher"):
        if "code" in a.split("\n")[0]:  # deklarasi data class
            continue
        d = args(a, ["code", "title", "description"])
        res.append({"code": d["code"], "title": d["title"], "description": d["description"], "percent_off": d.get("percentOff"),
                    "flat_off": d.get("flatOff"), "max_discount": d.get("maxDiscount"), "min_spend": d.get("minSpend", 0),
                    "expires": d["expiresAt"], "image": d["imageUrl"], "accent": d["accent"], "terms": d.get("terms", []),
                    "applies_to": d.get("appliesTo", "Semua makanan siap santap")})
    claimed = re.search(r'MutableStateFlow\(setOf\(((?:"[A-Z0-9]+",?\s*)+)\)\)', read("VoucherRepository.kt"))
    return res, re.findall(r'"([A-Z0-9]+)"', claimed.group(1)) if claimed else []


def notifications():
    res = []
    for a in calls(read("NotificationRepository.kt"), "AppNotification"):
        if "val id" in a:
            continue
        d = args(a, ["id", "type", "title", "body", "createdAt"])
        res.append({"id": d["id"], "type": d["type"], "title": d["title"], "body": d["body"], "at": d["createdAt"], "is_read": d.get("isRead", False)})
    return res


def chat():
    res = []
    for a in calls(read("ChatRepository.kt"), "ChatMessage"):
        if "me${" in a or "st${" in a or "sr${" in a or "by${" in a or "val id" in a:
            continue
        d = args(a, ["id", "merchantId", "fromBuyer", "text", "sentAt"])
        res.append({"id": d["id"], "merchant_id": d["merchantId"], "from_buyer": d["fromBuyer"], "text": d["text"], "at": d["sentAt"],
                    "listing_id": d.get("listingId"), "is_read": d.get("isRead", True), "buyer": d.get("buyer", "me")})
    return res


def incoming():
    res = []
    for a in calls(read("IncomingOrderRepository.kt"), "IncomingOrder"):
        if "val id" in a:
            continue
        d = args(a, ["id", "buyer", "code", "item_title", "quantity", "unit", "total", "fulfillment", "courier_label", "created", "pickup_by", "status"])
        d["status"] = d.get("status", "waiting")
        res.append(d)
    return res


def orders():
    res = []
    for a in calls(read("CommerceRepositories.kt"), "Order"):
        if 'id = "' not in a:
            continue
        d = args(a, [])
        res.append({"id": d["id"], "code": d["pickupCode"], "merchant_id": d["merchant"], "lines": d["lines"], "payment": d["payment"],
                    "note": d["note"], "status": d["status"], "created": d["createdAt"], "pickup_end": d["pickupEnd"],
                    "completed": d.get("completedAt"), "rating": d.get("rating")})
    return res


def addresses():
    return [dict(zip(["id", "label", "full_address", "lat", "lng"], [ev(x) for x in split_top(a)[:5]]),
                 note=(args(a, ["a", "b", "c", "d", "e"]).get("note", "")))
            for a in calls(read("CommerceRepositories.kt"), "Address") if a.startswith('"a')]


def payment_methods():
    res = []
    for a in calls(read("CommerceRepositories.kt"), "PaymentMethod"):
        if not a.startswith('"p-') or "${" in a:  # lewati yang dibuat dinamis (link e-wallet)
            continue
        d = args(a, ["id", "kind", "label", "detail"])
        res.append(d)
    return res


def saved():
    m = re.search(r"linkedSetOf\((.*?)\)", read("SavedMerchantRepository.kt"))
    return [ev(x) for x in split_top(m.group(1))]


def profile():
    p = read("ProfileRepository.kt")
    prof = args(calls(p, "UserProfile")[0].replace("name =", "name ="), [])
    impact = args(calls(p, "ImpactStats")[0], [])
    return {"name": prof["name"], "email": prof["email"], "phone": prof["phone"], "location": prof["location"], "member_since": prof["memberSince"],
            "impact": {"portions": impact["portions"], "compost_kg": impact["compostKg"], "carbon_kg": impact["carbonKg"], "saved_rupiah": impact["savedRupiah"]}}


def main():
    if not SRC.is_dir():
        fail(f"folder tidak ditemukan: {SRC}")
    ms = merchants()
    data = {"merchants": ms, "listings": listings({m["id"]: m["halal"] for m in ms}), "reviews": reviews(), "notifications": notifications(),
            "chat": chat(), "orders": orders(), "incoming": incoming(), "addresses": addresses(), "payment_methods": payment_methods(), "saved": saved()}
    data["vouchers"], claimed = vouchers()
    for k, want in EXPECTED.items():
        if len(data[k]) != want:
            fail(f"{k}: dapat {len(data[k])}, seharusnya {want}")
    # semua gambar yang dirujuk harus ada
    used = set()
    for m in ms:
        used |= {m["banner"], *m["photos"]}
    for l in data["listings"]:
        used |= {l["image"], *l["gallery"]}
    for r in data["reviews"]:
        used |= set(r["photos"])
    for v in data["vouchers"]:
        used.add(v["image"])
    for o in data["orders"]:
        used |= {ln["image"] for ln in o["lines"]}
    missing = sorted(u for u in used if not (IMG / f"{u}.jpg").exists())
    if missing:
        fail("gambar tidak ada di assets/img: " + ", ".join(missing))
    ids = [l["merchant_id"] for l in data["listings"]]
    if set(ids) - {m["id"] for m in ms}:
        fail("listing merujuk toko yang tidak ada")
    data["claimed_vouchers"] = claimed
    data["profile"] = profile()
    data["images"] = sorted(p.stem for p in IMG.glob("*.jpg"))
    OUT.mkdir(parents=True, exist_ok=True)
    for k, v in data.items():
        (OUT / f"{k}.json").write_text(json.dumps(v, indent=1, ensure_ascii=False) + "\n")
    print("ok:", {k: (len(v) if isinstance(v, list) else "obj") for k, v in data.items()})


main()
