package util

import (
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

// Claims adalah isi JWT access token yang diterbitkan saat login.
type Claims struct {
	UserID uuid.UUID `json:"user_id"`
	Email  string    `json:"email"`
	Roles  []string  `json:"roles"`
	jwt.RegisteredClaims
}

// GenerateToken membuat JWT access token HS256 dengan masa berlaku ttl.
func GenerateToken(secret string, ttl time.Duration, userID uuid.UUID, email string, roles []string) (token string, expiresAt time.Time, err error) {
	expiresAt = time.Now().Add(ttl)

	claims := Claims{
		UserID: userID,
		Email:  email,
		Roles:  roles,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(expiresAt),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
		},
	}

	signed, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte(secret))
	if err != nil {
		return "", time.Time{}, fmt.Errorf("generate token: %w", err)
	}
	return signed, expiresAt, nil
}

// ParseToken memvalidasi dan mem-parse JWT access token.
func ParseToken(secret, tokenString string) (*Claims, error) {
	claims := &Claims{}
	token, err := jwt.ParseWithClaims(tokenString, claims, func(t *jwt.Token) (interface{}, error) {
		return []byte(secret), nil
	})
	if err != nil {
		return nil, fmt.Errorf("parse token: %w", err)
	}
	if !token.Valid {
		return nil, fmt.Errorf("parse token: token tidak valid")
	}
	return claims, nil
}
