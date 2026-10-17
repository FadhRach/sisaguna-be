package realtime

import (
	"context"
	"encoding/json"
	"sync"
	"time"

	"github.com/coder/websocket"
	"github.com/google/uuid"
)

const (
	pingEvery    = 30 * time.Second
	pongTimeout  = 60 * time.Second
	writeTimeout = 5 * time.Second
	typingGap    = time.Second
)

type client struct {
	hub    *Hub
	user   uuid.UUID
	conn   *websocket.Conn
	send   chan []byte
	cancel context.CancelFunc
	once   sync.Once

	topics     map[string]struct{} // dijaga Hub.mu
	lastTyping time.Time
}

func newClient(h *Hub, user uuid.UUID, conn *websocket.Conn, cancel context.CancelFunc) *client {
	return &client{hub: h, user: user, conn: conn, send: make(chan []byte, sendBuffer), cancel: cancel, topics: map[string]struct{}{}}
}

// kill menutup koneksi seketika (tanpa menunggu handshake close).
func (c *client) kill() {
	c.once.Do(func() {
		c.cancel()
		_ = c.conn.CloseNow()
	})
}

// enqueue tidak pernah menunggu: klien yang buffer-nya penuh (sinyal buruk) diputus supaya tidak memperlambat yang lain.
func (c *client) enqueue(msg []byte) {
	select {
	case c.send <- msg:
	default:
		c.kill()
	}
}

func (c *client) writeLoop(ctx context.Context) {
	tick := time.NewTicker(pingEvery)
	defer tick.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case msg := <-c.send:
			wctx, cancel := context.WithTimeout(ctx, writeTimeout)
			err := c.conn.Write(wctx, websocket.MessageText, msg)
			cancel()
			if err != nil {
				c.kill()
				return
			}
		case <-tick.C:
			pctx, cancel := context.WithTimeout(ctx, pongTimeout)
			err := c.conn.Ping(pctx)
			cancel()
			if err != nil {
				c.kill()
				return
			}
		}
	}
}

// command adalah pesan HP ke server: subscribe, unsubscribe, typing.
type command struct {
	Type           string `json:"type"`
	Topic          string `json:"topic"`
	ConversationID string `json:"conversation_id"`
}

func (c *client) readLoop(ctx context.Context) {
	for {
		_, data, err := c.conn.Read(ctx)
		if err != nil {
			return
		}
		var cmd command
		if json.Unmarshal(data, &cmd) != nil {
			continue
		}
		switch cmd.Type {
		case "subscribe":
			if !c.hub.subscribe(c, cmd.Topic) {
				c.enqueue([]byte(`{"type":"error","data":{"code":"invalid_topic"}}`))
			}
		case "unsubscribe":
			c.hub.unsubscribe(c, cmd.Topic)
		case "typing":
			conv, err := uuid.Parse(cmd.ConversationID)
			if err != nil || time.Since(c.lastTyping) < typingGap {
				continue
			}
			c.lastTyping = time.Now()
			c.hub.typing(ctx, c.user, conv)
		}
	}
}
