// Package payment: pembayaran non-tunai TIRUAN. Tidak ada uang yang berpindah; setiap tagihan langsung dianggap
// berhasil. shortcut: ganti Charge dengan panggilan payment gateway sungguhan (QRIS/e-wallet) saat kemitraan terhubung;
// pemanggil (package order) hanya perlu tahu hasilnya paid atau gagal.
package payment

import (
	"fmt"
	"strings"
)

// Provider menandai asal pembayaran di respons API supaya app tahu ini bukan transaksi sungguhan.
const Provider = "dummy"

// Linkable: e-wallet yang harus ditautkan dulu oleh pengguna. QRIS dan tunai selalu tersedia.
func Linkable(kind string) bool { return kind == "gopay" || kind == "ovo" || kind == "dana" }

// Charge menagih amount rupiah dengan metode kind. Tiruan: selalu berhasil.
func Charge(kind string, amount int) error { return nil }

// MaskPhone menyamarkan nomor e-wallet seperti app Android: "0812 •••• 7890" (8 digit atau lebih), selain itu apa adanya.
func MaskPhone(phone string) string {
	var d strings.Builder
	for _, r := range phone {
		if r >= '0' && r <= '9' {
			d.WriteRune(r)
		}
	}
	digits := d.String()
	if len(digits) >= 8 {
		return fmt.Sprintf("%s •••• %s", digits[:4], digits[len(digits)-4:])
	}
	return digits
}
