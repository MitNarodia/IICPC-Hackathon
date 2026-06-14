// Package leaderboard turns the stream of Score events into a live, ranked board
// and pushes it to browsers. This file is the WebSocket fan-out hub.
//
// Design: clients subscribe to a "room" named by run_id. When the ranking
// service recomputes a board, it Broadcasts a JSON snapshot to that room. Each
// client has its own buffered send channel and a dedicated writer goroutine, so
// one slow browser can never block the others (if its buffer fills, we drop the
// client rather than stalling the hub — back-pressure isolation).
package leaderboard

import (
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

const (
	clientSendBuffer = 16
	writeWait        = 10 * time.Second
	pongWait         = 60 * time.Second
	pingPeriod       = (pongWait * 9) / 10
)

// Hub keeps the set of connected clients grouped by run room.
type Hub struct {
	mu    sync.RWMutex
	rooms map[string]map[*Client]struct{}
}

// NewHub builds an empty hub.
func NewHub() *Hub { return &Hub{rooms: make(map[string]map[*Client]struct{})} }

// Client is one WebSocket subscriber pinned to a single run room.
type Client struct {
	hub  *Hub
	run  string
	conn *websocket.Conn
	send chan []byte
}

// register adds a client to its room.
func (h *Hub) register(c *Client) {
	h.mu.Lock()
	defer h.mu.Unlock()
	room := h.rooms[c.run]
	if room == nil {
		room = make(map[*Client]struct{})
		h.rooms[c.run] = room
	}
	room[c] = struct{}{}
}

// unregister removes a client and tidies empty rooms.
func (h *Hub) unregister(c *Client) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if room := h.rooms[c.run]; room != nil {
		if _, ok := room[c]; ok {
			delete(room, c)
			close(c.send)
		}
		if len(room) == 0 {
			delete(h.rooms, c.run)
		}
	}
}

// Broadcast sends payload to every client subscribed to run. A client whose
// buffer is full is dropped (its writer will exit and unregister it).
func (h *Hub) Broadcast(run string, payload []byte) {
	h.mu.RLock()
	room := h.rooms[run]
	clients := make([]*Client, 0, len(room))
	for c := range room {
		clients = append(clients, c)
	}
	h.mu.RUnlock()

	for _, c := range clients {
		select {
		case c.send <- payload:
		default:
			// Slow consumer: drop it rather than block the whole room.
			h.unregister(c)
			_ = c.conn.Close()
		}
	}
}

// RoomSize reports how many clients watch a run (used by /metrics).
func (h *Hub) RoomSize(run string) int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.rooms[run])
}

// writePump owns all writes to the socket: queued broadcasts plus periodic
// pings to keep the connection alive and detect dead peers.
func (c *Client) writePump() {
	ticker := time.NewTicker(pingPeriod)
	defer func() {
		ticker.Stop()
		_ = c.conn.Close()
	}()
	for {
		select {
		case msg, ok := <-c.send:
			_ = c.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if !ok { // hub closed the channel
				_ = c.conn.WriteMessage(websocket.CloseMessage, nil)
				return
			}
			if err := c.conn.WriteMessage(websocket.TextMessage, msg); err != nil {
				return
			}
		case <-ticker.C:
			_ = c.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if err := c.conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}

// readPump drains incoming frames (we don't expect client messages) and handles
// pong/health so the connection's liveness is tracked. It exits on any error,
// which triggers unregistration.
func (c *Client) readPump() {
	defer func() {
		c.hub.unregister(c)
		_ = c.conn.Close()
	}()
	c.conn.SetReadLimit(1 << 16)
	_ = c.conn.SetReadDeadline(time.Now().Add(pongWait))
	c.conn.SetPongHandler(func(string) error {
		return c.conn.SetReadDeadline(time.Now().Add(pongWait))
	})
	for {
		if _, _, err := c.conn.ReadMessage(); err != nil {
			return
		}
	}
}
