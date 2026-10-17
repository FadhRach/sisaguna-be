// Package chat: percakapan pembeli dengan toko. Pembeli memulai, mitra hanya membalas.
// Pesan baru dan tanda baca dikirim sebagai event realtime dari transaksi yang sama dengan datanya.
package chat

import (
	"context"
	"errors"
	"fmt"
	"slices"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/FadhRach/sisaguna-be/db/sqlc"
	"github.com/FadhRach/sisaguna-be/internal/platform/database"
	"github.com/FadhRach/sisaguna-be/internal/platform/events"
	"github.com/FadhRach/sisaguna-be/internal/platform/httpx"
)

var (
	ErrNotFound         = errors.New("percakapan tidak ditemukan")
	ErrMerchantNotFound = errors.New("toko tidak ditemukan")
	ErrOwnMerchant      = errors.New("tidak bisa chat dengan tokomu sendiri")
	ErrInvalidListing   = errors.New("produk bukan milik toko ini")
)

const defaultMessages = 30

type Service struct {
	pool *pgxpool.Pool
	q    *sqlc.Queries
}

func NewService(pool *pgxpool.Pool) *Service { return &Service{pool: pool, q: sqlc.New(pool)} }

// Start mengambil percakapan yang sudah ada atau membuat baru. Hanya ke toko approved milik orang lain.
func (s *Service) Start(ctx context.Context, buyer, merchantID uuid.UUID) (*ConversationDTO, error) {
	m, err := s.q.GetMerchantBrief(ctx, merchantID)
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && m.Status != sqlc.MerchantStatusApproved) {
		return nil, ErrMerchantNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("chat service start: %w", err)
	}
	if m.OwnerID == buyer {
		return nil, ErrOwnMerchant
	}
	id, err := s.q.GetOrCreateConversation(ctx, sqlc.GetOrCreateConversationParams{BuyerID: buyer, MerchantID: merchantID})
	if err != nil {
		return nil, fmt.Errorf("chat service start: %w", err)
	}
	return &ConversationDTO{ID: id.String(), Counterpart: PartyDTO{ID: m.ID.String(), Name: m.Name}}, nil
}

// Inbox: percakapan yang sudah berisi pesan, terbaru dulu, dengan cursor.
func (s *Service) Inbox(ctx context.Context, user uuid.UUID, as string, cur *httpx.Cursor, limit int) ([]ConversationDTO, string, error) {
	p := sqlc.ListConversationsParams{Side: sqlc.ChatSide(as), UserID: user, Lim: int32(limit + 1)}
	if cur != nil {
		p.CurAt = pgtype.Timestamptz{Time: cur.At, Valid: true}
		p.CurID = uuid.NullUUID{UUID: cur.ID, Valid: true}
	}
	rows, err := s.q.ListConversations(ctx, p)
	if err != nil {
		return nil, "", fmt.Errorf("chat service inbox: %w", err)
	}
	next := ""
	if len(rows) > limit {
		rows = rows[:limit]
		last := rows[len(rows)-1]
		next = httpx.EncodeCursor(last.LastMessageAt.Time, last.ID)
	}
	out := make([]ConversationDTO, 0, len(rows))
	for _, r := range rows {
		other := PartyDTO{ID: r.MerchantID.String(), Name: r.MerchantName}
		if as == "merchant" {
			other = PartyDTO{ID: r.BuyerID.String(), Name: r.BuyerName}
		}
		out = append(out, ConversationDTO{ID: r.ID.String(), Counterpart: other, Unread: r.Unread,
			LastMessage: &LastMessageDTO{Body: r.LastBody, Side: string(r.LastSide), At: r.LastMessageAt.Time.UTC()}})
	}
	return out, next, nil
}

// Messages mengembalikan pesan urut naik dan id terakhir yang sudah dibaca lawan bicara (untuk tanda dibaca).
func (s *Service) Messages(ctx context.Context, user, convID uuid.UUID, q MessagesQuery) ([]MessageDTO, int64, error) {
	p, side, err := s.access(ctx, s.q, user, convID)
	if err != nil {
		return nil, 0, err
	}
	limit := q.Limit
	if limit == 0 {
		limit = defaultMessages
	}
	params := sqlc.ListMessagesParams{ConversationID: convID, Lim: int32(limit)}
	if q.BeforeID != nil {
		params.BeforeID = pgtype.Int8{Int64: *q.BeforeID, Valid: true}
	}
	if q.AfterID != nil {
		params.AfterID = pgtype.Int8{Int64: *q.AfterID, Valid: true}
	}
	rows, err := s.q.ListMessages(ctx, params)
	if err != nil {
		return nil, 0, fmt.Errorf("chat service messages: %w", err)
	}
	if q.AfterID == nil {
		slices.Reverse(rows) // query terbaru-dulu, klien menerima urut naik
	}
	out := make([]MessageDTO, 0, len(rows))
	for _, m := range rows {
		out = append(out, toMessage(m))
	}
	peerRead := p.MerchantLastReadID
	if side == sqlc.ChatSideMerchant {
		peerRead = p.BuyerLastReadID
	}
	return out, peerRead, nil
}

// Send idempoten lewat client_id: kirim ulang mengembalikan pesan yang sama (created=false) tanpa event baru.
func (s *Service) Send(ctx context.Context, user, convID uuid.UUID, in SendRequest) (msg *MessageDTO, created bool, err error) {
	err = database.InTx(ctx, s.pool, func(tx pgx.Tx) error {
		q := s.q.WithTx(tx)
		p, side, err := s.access(ctx, q, user, convID)
		if err != nil {
			return err
		}
		listing := uuid.NullUUID{}
		if in.ListingID != nil {
			ok, err := q.ListingBelongsToMerchant(ctx, sqlc.ListingBelongsToMerchantParams{ID: *in.ListingID, MerchantID: p.MerchantID})
			if err != nil {
				return err
			}
			if !ok {
				return ErrInvalidListing
			}
			listing = uuid.NullUUID{UUID: *in.ListingID, Valid: true}
		}
		row, err := q.InsertMessage(ctx, sqlc.InsertMessageParams{ConversationID: convID, SenderSide: side, SenderID: user,
			Body: in.Body, ListingID: listing, ClientID: in.ClientID})
		if errors.Is(err, pgx.ErrNoRows) { // client_id sudah pernah dikirim
			old, err := q.GetMessageByClientID(ctx, sqlc.GetMessageByClientIDParams{ConversationID: convID, ClientID: in.ClientID})
			if err != nil {
				return err
			}
			d := toMessage(old)
			msg = &d
			return nil
		}
		if err != nil {
			return err
		}
		if err := q.TouchConversation(ctx, sqlc.TouchConversationParams{ID: convID, MessageID: pgtype.Int8{Int64: row.ID, Valid: true}, At: row.CreatedAt}); err != nil {
			return err
		}
		d := toMessage(row)
		msg, created = &d, true
		// ke kedua pihak: lawan bicara menerima pesan, perangkat lain si pengirim ikut tersinkron
		return events.Emit(ctx, q, "message.created", d, []uuid.UUID{p.BuyerID, p.OwnerID}, "")
	})
	if err != nil {
		return nil, false, wrap("chat service send", err)
	}
	return msg, created, nil
}

// MarkRead memajukan penanda baca (tidak pernah mundur, tidak melewati pesan terakhir) dan memberi tahu lawan bicara.
func (s *Service) MarkRead(ctx context.Context, user, convID uuid.UUID, lastRead int64) (int64, error) {
	var final int64
	err := database.InTx(ctx, s.pool, func(tx pgx.Tx) error {
		q := s.q.WithTx(tx)
		p, side, err := s.access(ctx, q, user, convID)
		if err != nil {
			return err
		}
		before, peer := p.BuyerLastReadID, p.OwnerID
		if side == sqlc.ChatSideMerchant {
			before, peer = p.MerchantLastReadID, p.BuyerID
		}
		final, err = q.MarkConversationRead(ctx, sqlc.MarkConversationReadParams{ID: convID, Side: side, LastReadID: lastRead})
		if err != nil {
			return err
		}
		if final == before {
			return nil
		}
		return events.Emit(ctx, q, "conversation.read", map[string]any{"conversation_id": convID, "last_read_id": final}, []uuid.UUID{peer}, "")
	})
	return final, wrap("chat service mark read", err)
}

// Peer dipakai hub WebSocket untuk meneruskan "typing": mengembalikan user id lawan bicara,
// atau ok=false kalau user bukan peserta percakapan.
func (s *Service) Peer(ctx context.Context, user, convID uuid.UUID) (uuid.UUID, bool) {
	p, side, err := s.access(ctx, s.q, user, convID)
	if err != nil {
		return uuid.Nil, false
	}
	if side == sqlc.ChatSideBuyer {
		return p.OwnerID, true
	}
	return p.BuyerID, true
}

// access memastikan user adalah pembeli atau pemilik toko di percakapan ini. Orang lain dijawab "tidak ditemukan".
func (s *Service) access(ctx context.Context, q *sqlc.Queries, user, convID uuid.UUID) (sqlc.GetConversationPartiesRow, sqlc.ChatSide, error) {
	p, err := q.GetConversationParties(ctx, convID)
	if errors.Is(err, pgx.ErrNoRows) {
		return p, "", ErrNotFound
	}
	if err != nil {
		return p, "", fmt.Errorf("chat access: %w", err)
	}
	switch user {
	case p.BuyerID:
		return p, sqlc.ChatSideBuyer, nil
	case p.OwnerID:
		return p, sqlc.ChatSideMerchant, nil
	}
	return p, "", ErrNotFound
}

func toMessage(m sqlc.Message) MessageDTO {
	d := MessageDTO{ID: m.ID, ConversationID: m.ConversationID.String(), SenderSide: string(m.SenderSide), Body: m.Body,
		ClientID: m.ClientID.String(), CreatedAt: m.CreatedAt.Time.UTC()}
	if m.ListingID.Valid {
		l := m.ListingID.UUID.String()
		d.ListingID = &l
	}
	return d
}

func wrap(ctx string, err error) error {
	if err == nil {
		return nil
	}
	for _, known := range []error{ErrNotFound, ErrMerchantNotFound, ErrOwnMerchant, ErrInvalidListing} {
		if errors.Is(err, known) {
			return err
		}
	}
	return fmt.Errorf("%s: %w", ctx, err)
}
