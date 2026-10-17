package realtime

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"github.com/coder/websocket"
	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"

	"github.com/FadhRach/sisaguna-be/internal/platform/httpx"
)

// Routes: POST /api/v1/ws/ticket (login) dan GET /ws?ticket= (tiket sekali pakai, bukan token di URL).
func (h *Hub) Routes(router *gin.Engine, api *gin.RouterGroup, requireAuth, ticketLimit gin.HandlerFunc) {
	api.POST("/ws/ticket", requireAuth, ticketLimit, h.issueTicket)
	router.GET("/ws", h.serve)
}

func (h *Hub) issueTicket(c *gin.Context) {
	httpx.OK(c, http.StatusOK, gin.H{"ticket": h.tickets.issue(httpx.UserID(c)), "expires_in": int(ticketTTL.Seconds())})
}

func (h *Hub) serve(c *gin.Context) {
	user, ok := h.tickets.redeem(c.Query("ticket"))
	if !ok {
		httpx.Error(c, http.StatusUnauthorized, "invalid_ticket", "tiket tidak valid atau kedaluwarsa")
		return
	}
	conn, err := websocket.Accept(c.Writer, c.Request, nil)
	if err != nil {
		return // Accept sudah menulis responsnya
	}
	conn.SetReadLimit(maxInbound)
	ctx, cancel := context.WithCancel(c.Request.Context())
	cl := newClient(h, user, conn, cancel)
	h.add(cl)
	defer func() {
		h.remove(cl)
		cl.kill()
	}()
	go cl.writeLoop(ctx)
	cl.readLoop(ctx)
}

// Run mendengarkan channel sg_events lewat satu koneksi khusus (di luar pool) dan menyambung ulang
// dengan jeda 1, 2, 4, ... maks 30 detik. Event yang terjadi saat terputus tidak dikirim ulang.
func (h *Hub) Run(ctx context.Context, cfg *pgx.ConnConfig) {
	wait := time.Second
	for ctx.Err() == nil {
		started := time.Now()
		err := h.listen(ctx, cfg)
		if ctx.Err() != nil {
			return
		}
		if time.Since(started) > time.Minute {
			wait = time.Second // koneksi sempat sehat
		}
		slog.Error("realtime listener terputus", "err", err, "retry_in", wait.String())
		select {
		case <-ctx.Done():
			return
		case <-time.After(wait):
		}
		wait = min(wait*2, 30*time.Second)
	}
}

func (h *Hub) listen(ctx context.Context, cfg *pgx.ConnConfig) error {
	conn, err := pgx.ConnectConfig(ctx, cfg)
	if err != nil {
		return err
	}
	defer conn.Close(context.Background())
	if _, err := conn.Exec(ctx, "LISTEN sg_events"); err != nil {
		return err
	}
	for {
		n, err := conn.WaitForNotification(ctx)
		if err != nil {
			return err
		}
		h.dispatch(n.Payload)
	}
}
