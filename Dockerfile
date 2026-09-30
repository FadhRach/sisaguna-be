# Build stage
FROM golang:1.27-alpine AS builder

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /app/bin/sisaguna-be .

# Final stage — binary static (semua dependency pure Go, tidak butuh CGO),
# jadi bisa pakai base image minimal tanpa shell/package manager.
FROM gcr.io/distroless/static-debian12:nonroot

COPY --from=builder /app/bin/sisaguna-be /sisaguna-be

EXPOSE 3030

ENTRYPOINT ["/sisaguna-be"]
