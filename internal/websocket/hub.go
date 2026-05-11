package websocket

import (
	"database/sql"
	"encoding/json"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/mangahub/mangahub/internal/auth"
	"github.com/mangahub/mangahub/pkg/models"
)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin:     func(r *http.Request) bool { return true },
}

type Hub struct {
	db      *sql.DB
	authSvc *auth.Service
	mu      sync.RWMutex
	rooms   map[string]map[*Client]struct{}
}

type Client struct {
	hub      *Hub
	conn     *websocket.Conn
	userID   string
	username string
	room     string
	send     chan []byte
}

func NewHub(db *sql.DB, jwtSecret string) *Hub {
	return &Hub{
		db:      db,
		authSvc: auth.NewService(jwtSecret),
		rooms:   make(map[string]map[*Client]struct{}),
	}
}

func (h *Hub) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	token := r.URL.Query().Get("token")
	room := r.URL.Query().Get("room")
	if room == "" {
		room = "general"
	}
	claims, err := h.authSvc.ParseToken(token)
	if err != nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("[WS] upgrade error: %v", err)
		return
	}
	c := &Client{
		hub:      h,
		conn:     conn,
		userID:   claims.UserID,
		username: claims.Username,
		room:     room,
		send:     make(chan []byte, 32),
	}
	h.join(c)
	go c.writePump()
	c.readPump()
}

func (h *Hub) join(c *Client) {
	h.mu.Lock()
	if _, ok := h.rooms[c.room]; !ok {
		h.rooms[c.room] = make(map[*Client]struct{})
	}
	h.rooms[c.room][c] = struct{}{}
	count := len(h.rooms[c.room])
	h.mu.Unlock()
	log.Printf("[WS] %s joined %s (room size=%d)", c.username, c.room, count)

	history := h.recentMessages(c.room, 20)
	for _, m := range history {
		b, _ := json.Marshal(map[string]interface{}{
			"type":      "history",
			"username":  m.Username,
			"message":   m.Message,
			"room":      m.Room,
			"timestamp": m.Timestamp,
		})
		select {
		case c.send <- b:
		default:
		}
	}

	h.broadcastToRoom(c.room, map[string]interface{}{
		"type":      "join",
		"username":  c.username,
		"room":      c.room,
		"timestamp": time.Now().Unix(),
	})
}

func (h *Hub) leave(c *Client) {
	h.mu.Lock()
	if room, ok := h.rooms[c.room]; ok {
		delete(room, c)
		if len(room) == 0 {
			delete(h.rooms, c.room)
		}
	}
	h.mu.Unlock()
	close(c.send)
	c.conn.Close()
	log.Printf("[WS] %s left %s", c.username, c.room)
	h.broadcastToRoom(c.room, map[string]interface{}{
		"type":      "leave",
		"username":  c.username,
		"room":      c.room,
		"timestamp": time.Now().Unix(),
	})
}

func (h *Hub) broadcastToRoom(room string, payload interface{}) {
	b, err := json.Marshal(payload)
	if err != nil {
		return
	}
	h.mu.RLock()
	clients := h.rooms[room]
	targets := make([]*Client, 0, len(clients))
	for c := range clients {
		targets = append(targets, c)
	}
	h.mu.RUnlock()
	for _, c := range targets {
		select {
		case c.send <- b:
		default:
		}
	}
}

func (h *Hub) saveMessage(m models.ChatMessage) {
	_, err := h.db.Exec(
		"INSERT INTO chat_messages (user_id, username, room, message, timestamp) VALUES (?, ?, ?, ?, ?)",
		m.UserID, m.Username, m.Room, m.Message, m.Timestamp,
	)
	if err != nil {
		log.Printf("[WS] save message error: %v", err)
	}
}

func (h *Hub) recentMessages(room string, limit int) []models.ChatMessage {
	rows, err := h.db.Query(
		"SELECT user_id, username, room, message, timestamp FROM chat_messages WHERE room = ? ORDER BY id DESC LIMIT ?",
		room, limit,
	)
	if err != nil {
		return nil
	}
	defer rows.Close()
	out := []models.ChatMessage{}
	for rows.Next() {
		var m models.ChatMessage
		if err := rows.Scan(&m.UserID, &m.Username, &m.Room, &m.Message, &m.Timestamp); err == nil {
			out = append([]models.ChatMessage{m}, out...)
		}
	}
	return out
}

func (c *Client) readPump() {
	defer c.hub.leave(c)
	c.conn.SetReadLimit(4096)
	c.conn.SetReadDeadline(time.Now().Add(60 * time.Second))
	c.conn.SetPongHandler(func(string) error {
		c.conn.SetReadDeadline(time.Now().Add(60 * time.Second))
		return nil
	})
	for {
		_, data, err := c.conn.ReadMessage()
		if err != nil {
			return
		}
		var msg map[string]interface{}
		if err := json.Unmarshal(data, &msg); err != nil {
			continue
		}
		text, _ := msg["message"].(string)
		if text == "" {
			continue
		}
		if len(text) > 1000 {
			text = text[:1000]
		}
		now := time.Now().Unix()
		cm := models.ChatMessage{
			UserID:    c.userID,
			Username:  c.username,
			Message:   text,
			Room:      c.room,
			Timestamp: now,
		}
		c.hub.saveMessage(cm)
		c.hub.broadcastToRoom(c.room, map[string]interface{}{
			"type":      "message",
			"user_id":   cm.UserID,
			"username":  cm.Username,
			"message":   cm.Message,
			"room":      cm.Room,
			"timestamp": cm.Timestamp,
		})
	}
}

func (c *Client) writePump() {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case msg, ok := <-c.send:
			c.conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
			if !ok {
				c.conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}
			if err := c.conn.WriteMessage(websocket.TextMessage, msg); err != nil {
				return
			}
		case <-ticker.C:
			c.conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
			if err := c.conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}
