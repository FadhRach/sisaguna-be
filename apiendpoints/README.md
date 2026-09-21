# apiendpoints

Koleksi request manual buat test API SisaGuna, format `.http` (dipakai extension
**REST Client** di VS Code — `humao.rest-client`, gratis di marketplace).

## Cara pakai

1. Install extension "REST Client" di VS Code.
2. Jalankan backend lokal (`go run ./cmd/main.go`), pastikan `PORT` di `.env` cocok
   sama `@baseUrl` di file `.http` (default 3030).
3. Buka salah satu file `.http`, klik "Send Request" yang muncul di atas tiap blok
   request (dipisahkan `###`).
4. Beberapa request saling terhubung (mis. `login` dipakai `refresh`/`logout`) lewat
   variabel `{{login.response.body.$.data.access_token}}` — jalankan request `login`
   dulu sebelum request yang bergantung padanya.

Tidak pakai REST Client? Isi request yang sama bisa langsung disalin ke Postman/
Insomnia/curl — body JSON dan header-nya sudah generic.

## Isi

- `auth.http` — register, login, refresh, logout
- `users.http` — get/patch profil sendiri, lihat profil publik user lain
