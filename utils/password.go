package utils

import "golang.org/x/crypto/bcrypt"

// HashPassword bikin bcrypt hash dari password plain text.
func HashPassword(password string) (string, error) {
	bytes, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	return string(bytes), err
}

// ComparePassword balikin error kalau password tidak cocok dengan hash.
func ComparePassword(hash, password string) error {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(password))
}
