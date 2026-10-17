.PHONY: sync-android-seed migrate-up migrate-down sqlc test seed-dev seed-prod reset-dev

# Program Go membaca .env sendiri (godotenv); hanya CLI migrate yang perlu DATABASE_URL.
LOAD_ENV = set -a && . ./.env && set +a &&

migrate-up:
	$(LOAD_ENV) migrate -path db/migrations -database "$$DATABASE_URL" up

migrate-down:
	$(LOAD_ENV) migrate -path db/migrations -database "$$DATABASE_URL" down 1

sqlc:
	sqlc generate

test:
	go test ./...

seed-dev:
	go run ./cmd/seed -env dev

seed-prod:
	go run ./cmd/seed -env prod

# Hapus semua data dev (volume Postgres), migrasi ulang, isi seed. Hanya untuk lokal.
reset-dev:
	docker compose down -v
	docker compose up -d --wait postgres
	$(MAKE) migrate-up seed-dev

# Salin ulang data dummy app Android (folder SisaGuna/) ke cmd/seed/data lalu muat ke DB dev. Gambar: salin manual dari
# SisaGuna/app/src/main/assets/img/*.jpg ke cmd/seed/img/ kalau ada gambar baru.
sync-android-seed:
	python3 scripts/extract_android_seed.py SisaGuna
	cp SisaGuna/app/src/main/assets/img/*.jpg SisaGuna/app/src/main/assets/img/CREDITS.txt cmd/seed/img/
	$(MAKE) seed-dev
