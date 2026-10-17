# Build stage
FROM golang:1.27-alpine AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /out/api ./cmd/api \
 && CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /out/seed ./cmd/seed

# Final stage: binary statis (semua dependency pure Go), tanpa shell.
# Seed dijalankan lewat: docker compose run --rm --entrypoint /seed api -env prod
FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=builder /out/api /api
COPY --from=builder /out/seed /seed
EXPOSE 3030
ENTRYPOINT ["/api"]
