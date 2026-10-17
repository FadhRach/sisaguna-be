// Package auth berisi JWT access token, token opaque (refresh), dan hashing password.
package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
)

type Claims struct {
	IsAdmin bool `json:"adm,omitempty"`
	jwt.RegisteredClaims
}

func (c *Claims) UserID() (uuid.UUID, error) { return uuid.Parse(c.Subject) }

type Tokens struct {
	secret []byte
	ttl    time.Duration
}

func NewTokens(secret string, ttl time.Duration) *Tokens {
	return &Tokens{secret: []byte(secret), ttl: ttl}
}

// Issue membuat access token HS256.
func (t *Tokens) Issue(userID uuid.UUID, isAdmin bool) (string, time.Time, error) {
	exp := time.Now().Add(t.ttl)
	claims := Claims{
		IsAdmin:          isAdmin,
		RegisteredClaims: jwt.RegisteredClaims{Subject: userID.String(), ExpiresAt: jwt.NewNumericDate(exp), IssuedAt: jwt.NewNumericDate(time.Now())},
	}
	s, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(t.secret)
	if err != nil {
		return "", time.Time{}, fmt.Errorf("auth issue token: %w", err)
	}
	return s, exp, nil
}

// Parse memvalidasi token; hanya HS256 yang diterima.
func (t *Tokens) Parse(s string) (*Claims, error) {
	claims := &Claims{}
	_, err := jwt.ParseWithClaims(s, claims, func(*jwt.Token) (any, error) { return t.secret, nil },
		jwt.WithValidMethods([]string{"HS256"}), jwt.WithExpirationRequired())
	if err != nil {
		return nil, err
	}
	return claims, nil
}

// NewOpaqueToken membuat refresh token acak. Bukan JWT supaya bisa dicabut lewat DB.
func NewOpaqueToken() (string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("auth opaque token: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}

// HashToken: SHA-256 hex, yang disimpan di DB (token asli tidak pernah disimpan).
func HashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

// Passwords membungkus bcrypt plus hash dummy untuk menyamakan waktu respons
// login saat email tidak ditemukan.
type Passwords struct{ dummy []byte }

func NewPasswords() (*Passwords, error) {
	d, err := bcrypt.GenerateFromPassword([]byte("dummy-password-for-timing"), bcrypt.DefaultCost)
	if err != nil {
		return nil, fmt.Errorf("auth dummy hash: %w", err)
	}
	return &Passwords{dummy: d}, nil
}

func (p *Passwords) Hash(password string) (string, error) {
	b, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	return string(b), err
}

func (p *Passwords) Verify(hash, password string) bool {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)) == nil
}

// VerifyDummy memakai waktu yang sama dengan Verify; selalu false.
func (p *Passwords) VerifyDummy(password string) bool {
	_ = bcrypt.CompareHashAndPassword(p.dummy, []byte(password))
	return false
}
