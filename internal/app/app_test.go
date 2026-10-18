package app

import (
	"bytes"
	"context"
	"encoding/json"
	"image"
	"image/color"
	"image/png"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/FadhRach/sisaguna-be/internal/config"
	"github.com/FadhRach/sisaguna-be/internal/testutil"
)

type client struct {
	t     *testing.T
	h     http.Handler
	token string
}

type resp struct {
	Status int
	Body   map[string]any
}

func (r resp) data() map[string]any { d, _ := r.Body["data"].(map[string]any); return d }
func (r resp) list() []any          { d, _ := r.Body["data"].([]any); return d }
func (r resp) errCode() string {
	e, _ := r.Body["error"].(map[string]any)
	c, _ := e["code"].(string)
	return c
}
func (r resp) errField(k string) any { e, _ := r.Body["error"].(map[string]any); return e[k] }

func (c client) do(method, path string, body any, headers ...string) resp {
	c.t.Helper()
	var rd *bytes.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		rd = bytes.NewReader(b)
	} else {
		rd = bytes.NewReader(nil)
	}
	req := httptest.NewRequest(method, path, rd)
	req.Header.Set("Content-Type", "application/json")
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}
	for i := 0; i+1 < len(headers); i += 2 {
		req.Header.Set(headers[i], headers[i+1])
	}
	return c.serve(req)
}

func (c client) serve(req *http.Request) resp {
	rec := httptest.NewRecorder()
	c.h.ServeHTTP(rec, req)
	out := resp{Status: rec.Code, Body: map[string]any{}}
	_ = json.Unmarshal(rec.Body.Bytes(), &out.Body)
	return out
}

func (c client) upload(purpose string) resp {
	c.t.Helper()
	img := image.NewNRGBA(image.Rect(0, 0, 300, 200))
	for i := range img.Pix {
		img.Pix[i] = 180
	}
	img.Set(1, 1, color.White)
	var png_ bytes.Buffer
	_ = png.Encode(&png_, img)
	var body bytes.Buffer
	w := multipart.NewWriter(&body)
	_ = w.WriteField("purpose", purpose)
	fw, _ := w.CreateFormFile("file", "x.png")
	_, _ = fw.Write(png_.Bytes())
	_ = w.Close()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/media", &body)
	req.Header.Set("Content-Type", w.FormDataContentType())
	req.Header.Set("Authorization", "Bearer "+c.token)
	return c.serve(req)
}

func TestMVPJourney(t *testing.T) {
	pool := testutil.NewPool(t)
	a, err := New(&config.Config{AppEnv: "development", JWTSecret: strings.Repeat("s", 40), AccessTTL: time.Minute, RefreshTTL: time.Hour,
		PublicBaseURL: "http://api.test", MediaRoot: t.TempDir(), TrustedProxies: []string{"127.0.0.1"}, LoginRate: 100, RegisterRate: 100}, pool)
	if err != nil {
		t.Fatal(err)
	}
	anon := client{t: t, h: a.Router}
	register := func(name string) client {
		r := anon.do("POST", "/api/v1/auth/register", map[string]any{"name": name, "email": strings.ToLower(name) + "@e2e.test", "phone": "81234567890", "password": "Password123!", "account_type": "buyer"})
		if r.Status != 201 {
			t.Fatalf("register %s: %d %v", name, r.Status, r.Body)
		}
		return client{t: t, h: a.Router, token: r.data()["access_token"].(string)}
	}
	seller, buyer, adminUser := register("Seller"), register("Buyer"), register("Admin")
	if _, err := pool.Exec(context.Background(), `UPDATE users SET is_admin=true WHERE email='admin@e2e.test'`); err != nil {
		t.Fatal(err)
	}
	// is_admin dibaca dari token, jadi login ulang setelah dipromosikan
	adminUser.token = anon.do("POST", "/api/v1/auth/login", map[string]any{"email": "admin@e2e.test", "password": "Password123!"}).data()["access_token"].(string)

	// 1. mitra mendaftarkan toko (pending) dan belum bisa posting
	m := seller.do("POST", "/api/v1/merchants", map[string]any{"name": "Warung E2E", "address": "Jl. Senopati 1", "lat": -6.2433, "lng": 106.7989, "allergens": []string{"peanut"}})
	if m.Status != 201 || m.data()["status"] != "pending" {
		t.Fatalf("create merchant: %d %v", m.Status, m.Body)
	}
	merchantID := m.data()["id"].(string)
	if r := seller.do("POST", "/api/v1/merchants", map[string]any{"name": "Dua", "address": "x", "lat": 1, "lng": 1}); r.Status != 409 || r.errCode() != "merchant_exists" {
		t.Errorf("second merchant: %d %s", r.Status, r.errCode())
	}
	photo := seller.upload("listing")
	if photo.Status != 201 {
		t.Fatalf("upload: %d %v", photo.Status, photo.Body)
	}
	photoID := photo.data()["id"].(string)
	made := time.Now().Add(-time.Hour).UTC().Format(time.RFC3339)
	listingBody := map[string]any{
		"title": "Nasi Ayam Bakar", "tier": "human", "price": 12000, "price_original": 25000, "stock": 5, "pickup_hours": 2,
		"made_at": made, "storage": "room_temp", "safety_checks": []string{"untouched", "sensory_ok", "stored_right", "packed_clean"},
		"photo_media_ids": []string{photoID},
	}
	if r := seller.do("POST", "/api/v1/listings", listingBody, "Idempotency-Key", "l-1"); r.Status != 403 || r.errCode() != "merchant_not_approved" {
		t.Errorf("pending posting: %d %s", r.Status, r.errCode())
	}

	// 2. hanya admin yang bisa menyetujui
	if r := buyer.do("POST", "/api/v1/admin/merchants/"+merchantID+"/approve", nil); r.Status != 403 {
		t.Errorf("non-admin approve: %d", r.Status)
	}
	if r := adminUser.do("GET", "/api/v1/admin/merchants?status=pending", nil); r.Status != 200 || len(r.list()) != 1 {
		t.Errorf("admin list: %d %v", r.Status, r.Body)
	}
	if r := adminUser.do("POST", "/api/v1/admin/merchants/"+merchantID+"/approve", nil); r.Status != 204 {
		t.Fatalf("approve: %d %v", r.Status, r.Body)
	}
	if n := seller.do("GET", "/api/v1/me/notifications", nil).list(); len(n) != 1 || n[0].(map[string]any)["type"] != "merchant" {
		t.Errorf("owner must be notified: %v", n)
	}

	// 3. posting listing: idempotent, aturan kelayakan pangan
	if r := seller.do("POST", "/api/v1/listings", listingBody); r.Status != 400 || r.errCode() != "idempotency_key_required" {
		t.Errorf("missing key: %d %s", r.Status, r.errCode())
	}
	first := seller.do("POST", "/api/v1/listings", listingBody, "Idempotency-Key", "l-1")
	if first.Status != 201 {
		t.Fatalf("create listing: %d %v", first.Status, first.Body)
	}
	listingID := first.data()["id"].(string)
	if again := seller.do("POST", "/api/v1/listings", listingBody, "Idempotency-Key", "l-1"); again.Status != 200 || again.data()["id"] != listingID {
		t.Errorf("repeat: %d %v", again.Status, again.data()["id"])
	}
	stale := map[string]any{}
	for k, v := range listingBody {
		stale[k] = v
	}
	stale["made_at"] = time.Now().Add(-6 * time.Hour).UTC().Format(time.RFC3339)
	stale["photo_media_ids"] = []string{seller.upload("listing").data()["id"].(string)}
	if r := seller.do("POST", "/api/v1/listings", stale, "Idempotency-Key", "l-2"); r.Status != 422 || r.errCode() != "past_safe_window" || r.errField("suggested_tier") != "animal_feed" {
		t.Errorf("past safe window: %d %v", r.Status, r.Body)
	}
	if r := seller.do("POST", "/api/v1/listings", map[string]any{"title": "x"}, "Idempotency-Key", "l-3"); r.Status != 400 || r.errCode() != "validation_failed" {
		t.Errorf("validation: %d %s", r.Status, r.errCode())
	}

	// 4. pembeli menemukan listing
	home := buyer.do("GET", "/api/v1/home?lat=-6.24&lng=106.80&tab=human", nil)
	nearby, _ := home.data()["nearby"].([]any)
	if home.Status != 200 || len(nearby) != 1 {
		t.Fatalf("home: %d %v", home.Status, home.Body)
	}
	card := nearby[0].(map[string]any)
	if card["id"] != listingID || card["distance_km"] == nil || !strings.HasPrefix(card["cover_url"].(string), "http://api.test/media/") {
		t.Errorf("card: %v", card)
	}
	if r := anon.do("GET", "/api/v1/listings/"+listingID, nil); r.Status != 200 || r.data()["safety"].(map[string]any)["level"] != "safe" {
		t.Errorf("public detail: %d %v", r.Status, r.data()["safety"])
	}
	if r := anon.do("GET", "/api/v1/merchants/"+merchantID, nil); r.Status != 200 || len(r.data()["listings"].([]any)) != 1 {
		t.Errorf("merchant page: %d %v", r.Status, r.Body)
	}

	// 5. checkout
	cart := map[string]any{"merchant_id": merchantID, "lines": []map[string]any{{"listing_id": listingID, "quantity": 2}}}
	if q := buyer.do("POST", "/api/v1/checkout/quote", cart); q.Status != 200 || q.data()["total"] != float64(25000) { // 24.000 + 1.000
		t.Fatalf("quote: %d %v", q.Status, q.Body)
	}
	orderBody := map[string]any{"merchant_id": merchantID, "lines": cart["lines"], "payment_kind": "cash", "expected_total": 25000}
	if r := buyer.do("POST", "/api/v1/orders", orderBody); r.Status != 400 || r.errCode() != "idempotency_key_required" {
		t.Errorf("order without key: %d %s", r.Status, r.errCode())
	}
	stalePrice := map[string]any{"merchant_id": merchantID, "lines": cart["lines"], "payment_kind": "cash", "expected_total": 1}
	if r := buyer.do("POST", "/api/v1/orders", stalePrice, "Idempotency-Key", "o-0"); r.Status != 409 || r.errCode() != "price_changed" || r.errField("total") != float64(25000) {
		t.Errorf("price_changed: %d %v", r.Status, r.Body)
	}
	if r := buyer.do("POST", "/api/v1/orders", map[string]any{"merchant_id": merchantID, "lines": cart["lines"], "payment_kind": "qris", "expected_total": 25000}, "Idempotency-Key", "o-q"); r.Status != 422 || r.errCode() != "payment_method_unavailable" {
		t.Errorf("qris: %d %s", r.Status, r.errCode())
	}
	placed := buyer.do("POST", "/api/v1/orders", orderBody, "Idempotency-Key", "o-1")
	if placed.Status != 201 {
		t.Fatalf("place: %d %v", placed.Status, placed.Body)
	}
	orderID := placed.data()["id"].(string)
	code := placed.data()["code"].(string)
	if !strings.HasPrefix(code, "SG-") || placed.data()["status"] != "ready" {
		t.Errorf("order: %v", placed.data())
	}
	if r := buyer.do("POST", "/api/v1/orders", orderBody, "Idempotency-Key", "o-1"); r.Status != 200 || r.data()["id"] != orderID {
		t.Errorf("order repeat: %d", r.Status)
	}
	if d := anon.do("GET", "/api/v1/listings/"+listingID, nil).data(); d["stock"] != float64(3) {
		t.Errorf("stock after order = %v", d["stock"])
	}

	// 6. mitra: pesanan masuk (tanpa kode), salah kode, lalu serahkan
	inbox := seller.do("GET", "/api/v1/me/merchant/orders?status=ready", nil)
	if inbox.Status != 200 || len(inbox.list()) != 1 {
		t.Fatalf("inbox: %d %v", inbox.Status, inbox.Body)
	}
	if _, leaked := inbox.list()[0].(map[string]any)["code"]; leaked {
		t.Error("merchant inbox must not reveal the pickup code")
	}
	wrong := "SG-0000"
	if code == wrong {
		wrong = "SG-0001"
	}
	if r := seller.do("POST", "/api/v1/me/merchant/orders/"+orderID+"/confirm", map[string]any{"code": wrong}); r.Status != 422 || r.errCode() != "wrong_code" || r.errField("attempts_left") != float64(4) {
		t.Errorf("wrong code: %d %v", r.Status, r.Body)
	}
	if r := buyer.do("POST", "/api/v1/me/merchant/orders/"+orderID+"/confirm", map[string]any{"code": code}); r.Status != 404 {
		t.Errorf("buyer confirming: %d", r.Status)
	}
	done := seller.do("POST", "/api/v1/me/merchant/orders/"+orderID+"/confirm", map[string]any{"code": strings.ToLower(code)})
	if done.Status != 200 || done.data()["status"] != "completed" || done.data()["merchant_payout"] != float64(21600) {
		t.Fatalf("confirm: %d %v", done.Status, done.Body)
	}
	dash := seller.do("GET", "/api/v1/me/merchant/dashboard", nil).data()
	if dash["sales_today"] != float64(24000) || dash["net_today"] != float64(21600) || dash["completed_today"] != float64(1) {
		t.Errorf("dashboard: %v", dash)
	}

	// 7. pembeli: riwayat dan notifikasi
	hist := buyer.do("GET", "/api/v1/me/orders?tab=history", nil)
	if hist.Status != 200 || len(hist.list()) != 1 {
		t.Errorf("history: %d %v", hist.Status, hist.Body)
	}
	notes := buyer.do("GET", "/api/v1/me/notifications?limit=1", nil)
	if notes.Status != 200 || len(notes.list()) != 1 || notes.Body["meta"].(map[string]any)["next_cursor"] == nil {
		t.Errorf("notifications paging: %v", notes.Body)
	}
	if me := buyer.do("GET", "/api/v1/me", nil).data(); me["unread_notifications"] != float64(2) {
		t.Errorf("unread = %v", me["unread_notifications"])
	}
	if r := buyer.do("POST", "/api/v1/me/notifications/read-all", nil); r.Status != 204 {
		t.Errorf("read-all: %d", r.Status)
	}
	if me := buyer.do("GET", "/api/v1/me", nil).data(); me["unread_notifications"] != float64(0) {
		t.Errorf("unread after read-all = %v", me["unread_notifications"])
	}

	// 8. voucher: klaim dan daftar
	pool.Exec(context.Background(), `INSERT INTO vouchers (code,title,flat_off,expires_at) VALUES ('HEMAT2K','Hemat',2000,now()+interval '1 day')`)
	if r := buyer.do("POST", "/api/v1/vouchers/hemat2k/claim", nil); r.Status != 204 {
		t.Errorf("claim: %d %v", r.Status, r.Body)
	}
	if r := buyer.do("POST", "/api/v1/vouchers/HEMAT2K/claim", nil); r.Status != 409 || r.errCode() != "voucher_already_claimed" {
		t.Errorf("claim twice: %d %s", r.Status, r.errCode())
	}
	if r := buyer.do("POST", "/api/v1/vouchers/NOPE/claim", nil); r.Status != 404 {
		t.Errorf("unknown voucher: %d", r.Status)
	}
	vs := buyer.do("GET", "/api/v1/me/vouchers", nil).list()
	if len(vs) != 1 || vs[0].(map[string]any)["status"] != "claimed" {
		t.Errorf("vouchers: %v", vs)
	}
	// 9. sesudah selesai: ulasan, komplain (foto privat), dampak, favorit, preferensi
	if d := buyer.do("GET", "/api/v1/orders/"+orderID, nil).data(); d["reviewed"] != false || d["complaint_status"] != nil {
		t.Errorf("flags sebelum ulasan: %v %v", d["reviewed"], d["complaint_status"])
	}
	reviewPhoto := buyer.upload("review").data()["id"].(string)
	rev := buyer.do("POST", "/api/v1/orders/"+orderID+"/review", map[string]any{"stars": 5, "tags": []string{"Porsi pas"}, "comment": "Mantap", "media_ids": []string{reviewPhoto}})
	if rev.Status != 201 {
		t.Fatalf("review: %d %v", rev.Status, rev.Body)
	}
	if r := buyer.do("POST", "/api/v1/orders/"+orderID+"/review", map[string]any{"stars": 1}); r.Status != 409 || r.errCode() != "already_reviewed" {
		t.Errorf("review twice: %d %s", r.Status, r.errCode())
	}
	if r := seller.do("POST", "/api/v1/orders/"+orderID+"/review", map[string]any{"stars": 1}); r.Status != 404 {
		t.Errorf("merchant reviewing own order: %d", r.Status)
	}
	if r := anon.do("GET", "/api/v1/merchants/"+merchantID+"/reviews", nil); r.Status != 200 || len(r.list()) != 1 || r.list()[0].(map[string]any)["buyer_name"] != "Buyer" {
		t.Errorf("reviews: %d %v", r.Status, r.Body)
	}
	if r := anon.do("GET", "/api/v1/merchants/"+merchantID, nil); r.data()["rating_avg"] != float64(5) || r.data()["rating_count"] != float64(1) || r.data()["is_saved"] != nil {
		t.Errorf("merchant rating / anonymous is_saved: %v", r.data())
	}

	proof := buyer.upload("complaint").data()
	comp := buyer.do("POST", "/api/v1/orders/"+orderID+"/complaint", map[string]any{"reason": "not_fresh", "detail": "Ayamnya terasa tidak segar.", "media_ids": []string{proof["id"].(string)}})
	if comp.Status != 201 {
		t.Fatalf("complaint: %d %v", comp.Status, comp.Body)
	}
	for name, who := range map[string]client{"pembeli": buyer, "pemilik toko": seller, "admin": adminUser} {
		if r := who.do("GET", "/api/v1/media/private/"+proof["id"].(string), nil); r.Status != 200 {
			t.Errorf("foto komplain oleh %s: %d", name, r.Status)
		}
	}
	if r := anon.do("GET", "/api/v1/media/private/"+proof["id"].(string), nil); r.Status != 401 {
		t.Errorf("private media anonymous: %d", r.Status)
	}
	if r := adminUser.do("GET", "/api/v1/admin/complaints?status=submitted", nil); r.Status != 200 || len(r.list()) != 1 {
		t.Fatalf("admin complaints: %d %v", r.Status, r.Body)
	}
	if r := buyer.do("GET", "/api/v1/admin/complaints", nil); r.Status != 403 {
		t.Errorf("non-admin complaints: %d", r.Status)
	}
	complaintID := comp.data()["id"].(string)
	if r := adminUser.do("PATCH", "/api/v1/admin/complaints/"+complaintID, map[string]any{"status": "resolved", "resolution_note": "Toko sudah dihubungi."}); r.Status != 204 {
		t.Errorf("resolve complaint: %d %v", r.Status, r.Body)
	}
	if d := buyer.do("GET", "/api/v1/orders/"+orderID, nil).data(); d["reviewed"] != true || d["complaint_status"] != "resolved" {
		t.Errorf("flags sesudah: %v %v", d["reviewed"], d["complaint_status"])
	}

	// dampak: 2 porsi, hemat (25.000-12.000) x 2
	if imp := buyer.do("GET", "/api/v1/me/impact", nil).data(); imp["completed_orders"] != float64(1) || imp["portions"] != float64(2) || imp["saved_rupiah"] != float64(26000) {
		t.Errorf("impact: %v", imp)
	}

	// favorit
	if r := buyer.do("PUT", "/api/v1/me/saved-merchants/"+merchantID, nil); r.Status != 204 {
		t.Errorf("save: %d", r.Status)
	}
	if saved := buyer.do("GET", "/api/v1/me/saved-merchants?lat=-6.24&lng=106.80", nil).list(); len(saved) != 1 || saved[0].(map[string]any)["distance_km"] == nil {
		t.Errorf("saved list: %v", saved)
	}
	if r := buyer.do("GET", "/api/v1/merchants/"+merchantID, nil); r.data()["is_saved"] != true {
		t.Errorf("is_saved for buyer: %v", r.data()["is_saved"])
	}
	if r := buyer.do("DELETE", "/api/v1/me/saved-merchants/"+merchantID, nil); r.Status != 204 {
		t.Errorf("unsave: %d", r.Status)
	}

	// preferensi, hapus + pulihkan notifikasi
	if r := buyer.do("PUT", "/api/v1/me/notification-prefs", map[string]any{"enabled": true}); r.Status != 400 {
		t.Errorf("prefs harus lengkap: %d", r.Status)
	}
	prefs := map[string]any{"enabled": true, "pickup": true, "order": true, "merchant": true, "promo": true, "impact": false}
	if r := buyer.do("PUT", "/api/v1/me/notification-prefs", prefs); r.Status != 200 || r.data()["promo"] != true || r.data()["impact"] != false {
		t.Errorf("set prefs: %d %v", r.Status, r.Body)
	}
	noteID := buyer.do("GET", "/api/v1/me/notifications", nil).list()[0].(map[string]any)["id"].(string)
	if r := buyer.do("DELETE", "/api/v1/me/notifications/"+noteID, nil); r.Status != 204 {
		t.Errorf("delete notification: %d", r.Status)
	}
	if r := buyer.do("POST", "/api/v1/me/notifications/"+noteID+"/restore", nil); r.Status != 204 {
		t.Errorf("restore notification: %d", r.Status)
	}

	// 10. chat: pembeli memulai, mitra membalas, tanda baca
	conv := buyer.do("POST", "/api/v1/conversations", map[string]any{"merchant_id": merchantID})
	if conv.Status != 200 {
		t.Fatalf("start chat: %d %v", conv.Status, conv.Body)
	}
	convID := conv.data()["id"].(string)
	if r := seller.do("POST", "/api/v1/conversations", map[string]any{"merchant_id": merchantID}); r.Status != 403 || r.errCode() != "own_merchant" {
		t.Errorf("chat with own store: %d %s", r.Status, r.errCode())
	}
	msg := map[string]any{"client_id": "7b1f5d1e-5c0e-4f57-9a35-2b9f2b9a0001", "body": "Masih ada ayam bakar?", "listing_id": listingID}
	if r := buyer.do("POST", "/api/v1/conversations/"+convID+"/messages", msg); r.Status != 201 {
		t.Fatalf("send: %d %v", r.Status, r.Body)
	}
	if r := buyer.do("POST", "/api/v1/conversations/"+convID+"/messages", msg); r.Status != 200 {
		t.Errorf("resend same client_id: %d", r.Status)
	}
	if box := seller.do("GET", "/api/v1/conversations?as=merchant", nil).list(); len(box) != 1 || box[0].(map[string]any)["unread"] != float64(1) {
		t.Errorf("seller inbox: %v", box)
	}
	msgs := seller.do("GET", "/api/v1/conversations/"+convID+"/messages", nil)
	if len(msgs.list()) != 1 {
		t.Fatalf("messages: %v", msgs.Body)
	}
	lastID := msgs.list()[0].(map[string]any)["id"]
	if r := seller.do("POST", "/api/v1/conversations/"+convID+"/read", map[string]any{"last_read_id": lastID}); r.Status != 200 {
		t.Errorf("mark read: %d %v", r.Status, r.Body)
	}
	if box := seller.do("GET", "/api/v1/conversations?as=merchant", nil).list(); box[0].(map[string]any)["unread"] != float64(0) {
		t.Errorf("unread after read: %v", box)
	}
	if r := adminUser.do("GET", "/api/v1/conversations/"+convID+"/messages", nil); r.Status != 404 {
		t.Errorf("outsider reading chat: %d", r.Status)
	}
	if r := buyer.do("POST", "/api/v1/ws/ticket", nil); r.Status != 200 || r.data()["ticket"] == "" {
		t.Errorf("ws ticket: %d %v", r.Status, r.Body)
	}
}
