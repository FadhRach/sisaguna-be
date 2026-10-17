-- name: EnsurePaymentMethods :exec
-- QRIS dan tunai selalu ada; QRIS menjadi default sampai pengguna memilih lain.
INSERT INTO payment_methods (user_id, kind, detail, is_default) VALUES
    (@user_id, 'qris', 'Bayar pakai aplikasi bank atau e-wallet apa pun', true),
    (@user_id, 'cash', 'Tunai langsung ke penyedia', false)
ON CONFLICT DO NOTHING;

-- name: ListPaymentMethods :many
-- Urutan seperti app: QRIS, e-wallet sesuai urutan tautan, tunai terakhir.
SELECT kind, detail, is_default FROM payment_methods WHERE user_id = $1
ORDER BY CASE kind WHEN 'qris' THEN 0 WHEN 'cash' THEN 2 ELSE 1 END, created_at, kind;

-- name: LinkPaymentMethod :exec
INSERT INTO payment_methods (user_id, kind, detail) VALUES ($1, $2, $3)
ON CONFLICT (user_id, kind) DO UPDATE SET detail = EXCLUDED.detail;

-- name: UnlinkPaymentMethod :one
-- QRIS dan tunai tidak bisa dilepas (ErrNoRows). Mengembalikan apakah yang dilepas adalah default.
DELETE FROM payment_methods WHERE user_id = $1 AND kind = $2 AND kind NOT IN ('qris', 'cash') RETURNING is_default;

-- name: ClearDefaultPayment :exec
UPDATE payment_methods SET is_default = false WHERE user_id = $1;

-- name: SetDefaultPayment :execrows
UPDATE payment_methods SET is_default = true WHERE user_id = $1 AND kind = $2;

-- name: HasPaymentMethod :one
SELECT EXISTS (SELECT 1 FROM payment_methods WHERE user_id = $1 AND kind = $2)::bool;
