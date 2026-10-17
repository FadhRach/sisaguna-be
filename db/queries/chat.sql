-- name: GetOrCreateConversation :one
-- UNIQUE (buyer_id, merchant_id): panggilan ulang mengembalikan percakapan yang sama.
INSERT INTO conversations (buyer_id, merchant_id) VALUES ($1, $2)
ON CONFLICT (buyer_id, merchant_id) DO UPDATE SET buyer_id = EXCLUDED.buyer_id
RETURNING id;

-- name: GetConversationParties :one
-- Pembeli dan pemilik toko dari satu percakapan; dipakai untuk cek akses dan tujuan event.
SELECT c.id, c.buyer_id, c.merchant_id, m.owner_id AS owner_id, m.name AS merchant_name,
       c.buyer_last_read_id, c.merchant_last_read_id, coalesce(c.last_message_id, 0)::bigint AS last_message_id
FROM conversations c JOIN merchants m ON m.id = c.merchant_id
WHERE c.id = $1;

-- name: ListConversations :many
-- side 'buyer': percakapan milik pembeli. side 'merchant': percakapan toko milik pemilik. Hanya yang sudah ada pesan.
SELECT c.id, c.merchant_id, m.name AS merchant_name, c.buyer_id, u.name AS buyer_name,
       c.last_message_at, msg.body AS last_body, msg.sender_side AS last_side,
       (SELECT count(*) FROM messages x WHERE x.conversation_id = c.id
          AND x.id > CASE WHEN @side::chat_side = 'buyer' THEN c.buyer_last_read_id ELSE c.merchant_last_read_id END
          AND x.sender_side <> @side::chat_side)::bigint AS unread
FROM conversations c
JOIN merchants m ON m.id = c.merchant_id
JOIN users u ON u.id = c.buyer_id
JOIN messages msg ON msg.id = c.last_message_id
WHERE c.last_message_id IS NOT NULL
  AND CASE WHEN @side::chat_side = 'buyer' THEN c.buyer_id = @user_id ELSE m.owner_id = @user_id END
  AND (sqlc.narg('cur_at')::timestamptz IS NULL OR (c.last_message_at, c.id) < (sqlc.narg('cur_at')::timestamptz, sqlc.narg('cur_id')::uuid))
ORDER BY c.last_message_at DESC, c.id DESC
LIMIT @lim;

-- name: InsertMessage :one
-- ErrNoRows = client_id sudah pernah dikirim (kirim ulang): ambil pesan lama dengan GetMessageByClientID.
INSERT INTO messages (conversation_id, sender_side, sender_id, body, listing_id, client_id)
VALUES (@conversation_id, @sender_side::chat_side, @sender_id, @body, sqlc.narg('listing_id'), @client_id)
ON CONFLICT (conversation_id, client_id) DO NOTHING
RETURNING *;

-- name: GetMessageByClientID :one
SELECT * FROM messages WHERE conversation_id = $1 AND client_id = $2;

-- name: TouchConversation :exec
UPDATE conversations SET last_message_id = @message_id, last_message_at = @at WHERE id = @id;

-- name: ListMessages :many
-- before_id / after_id memilih arah; tanpa keduanya = pesan terbaru. Hasil selalu dibalik jadi urut naik di service.
SELECT * FROM messages
WHERE conversation_id = @conversation_id
  AND (sqlc.narg('before_id')::bigint IS NULL OR id < sqlc.narg('before_id')::bigint)
  AND (sqlc.narg('after_id')::bigint IS NULL OR id > sqlc.narg('after_id')::bigint)
ORDER BY CASE WHEN sqlc.narg('after_id')::bigint IS NULL THEN -id ELSE id END
LIMIT @lim;

-- name: MarkConversationRead :one
-- Penanda tidak pernah mundur dan tidak melewati pesan terakhir. Mengembalikan nilai akhir.
UPDATE conversations SET
    buyer_last_read_id    = CASE WHEN @side::chat_side = 'buyer'
                                 THEN greatest(buyer_last_read_id, least(@last_read_id::bigint, coalesce(last_message_id, 0))) ELSE buyer_last_read_id END,
    merchant_last_read_id = CASE WHEN @side::chat_side = 'merchant'
                                 THEN greatest(merchant_last_read_id, least(@last_read_id::bigint, coalesce(last_message_id, 0))) ELSE merchant_last_read_id END
WHERE id = @id
RETURNING CASE WHEN @side::chat_side = 'buyer' THEN buyer_last_read_id ELSE merchant_last_read_id END::bigint AS last_read_id;

-- name: ListingBelongsToMerchant :one
SELECT EXISTS (SELECT 1 FROM listings WHERE id = $1 AND merchant_id = $2 AND status <> 'deleted')::bool;
