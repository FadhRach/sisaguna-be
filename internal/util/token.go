package util

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
)

// GenerateOpaqueToken membuat token acak (dipakai untuk refresh token).
// Beda dengan access token, refresh token bukan JWT — supaya bisa direvoke
// langsung lewat DB saat logout, tanpa perlu mekanisme blacklist JWT.
func GenerateOpaqueToken() (string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("generate opaque token: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}

// HashToken menghasilkan SHA-256 hash dari token untuk disimpan di database,
// supaya token asli tidak pernah ada di storage (prinsip sama seperti password).
func HashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}
