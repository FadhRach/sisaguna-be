// Package idempotency: Idempotency-Key yang dipakai ulang harus membawa isi request yang sama. Isi berbeda dengan key
// yang sama berarti bug klien atau percobaan menyusupkan request lain, bukan retry, sehingga ditolak.
package idempotency

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
)

var ErrKeyReused = errors.New("Idempotency-Key ini sudah dipakai untuk permintaan yang berbeda, buat key baru")

// Hash adalah SHA-256 dari JSON v. Struct menghasilkan urutan field yang tetap; slice yang urutannya tidak berarti
// harus diurutkan dulu oleh pemanggil.
func Hash(v any) string {
	b, _ := json.Marshal(v) // tipe request hanya berisi string/angka/slice, tidak bisa gagal
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

// Check membandingkan hash tersimpan dengan hash request. Baris lama tanpa hash (stored == "") dianggap cocok.
func Check(stored, current string) error {
	if stored != "" && stored != current {
		return ErrKeyReused
	}
	return nil
}
