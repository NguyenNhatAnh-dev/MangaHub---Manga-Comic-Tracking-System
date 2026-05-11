package tcp

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net"
	"strings"
	"sync"
	"time"

	"github.com/mangahub/mangahub/internal/auth"
	"github.com/mangahub/mangahub/pkg/models"
	"github.com/mangahub/mangahub/pkg/protocol"
)

type Server struct {
	addr      string
	authSvc   *auth.Service
	broker    *protocol.Broker
	listener  net.Listener
	mu        sync.RWMutex
	clients   map[string]*client
	stop      chan struct{}
}

type client struct {
	id       string
	userID   string
	username string
	conn     net.Conn
	writer   *bufio.Writer
	mu       sync.Mutex
}

func NewServer(addr, jwtSecret string) *Server {
	return &Server{
		addr:    addr,
		authSvc: auth.NewService(jwtSecret),
		broker:  protocol.Default(),
		clients: make(map[string]*client),
		stop:    make(chan struct{}),
	}
}

func (s *Server) Start() error {
	ln, err := net.Listen("tcp", s.addr)
	if err != nil {
		return fmt.Errorf("listen tcp %s: %w", s.addr, err)
	}
	s.listener = ln
	log.Printf("[TCP] Listening on %s", s.addr)

	go s.broadcastLoop()

	for {
		conn, err := ln.Accept()
		if err != nil {
			select {
			case <-s.stop:
				return nil
			default:
				log.Printf("[TCP] accept error: %v", err)
				continue
			}
		}
		go s.handleConn(conn)
	}
}

func (s *Server) Stop() {
	close(s.stop)
	if s.listener != nil {
		s.listener.Close()
	}
	s.mu.Lock()
	for _, c := range s.clients {
		c.conn.Close()
	}
	s.clients = make(map[string]*client)
	s.mu.Unlock()
}

func (s *Server) handleConn(conn net.Conn) {
	defer conn.Close()
	reader := bufio.NewReader(conn)
	writer := bufio.NewWriter(conn)

	conn.SetReadDeadline(time.Now().Add(10 * time.Second))
	authLine, err := reader.ReadString('\n')
	if err != nil {
		log.Printf("[TCP] auth read error: %v", err)
		return
	}
	conn.SetReadDeadline(time.Time{})

	var authMsg struct {
		Type  string `json:"type"`
		Token string `json:"token"`
	}
	if err := json.Unmarshal([]byte(strings.TrimSpace(authLine)), &authMsg); err != nil {
		writeJSONLine(writer, map[string]string{"type": "error", "error": "invalid auth message"})
		return
	}

	claims, err := s.authSvc.ParseToken(authMsg.Token)
	if err != nil {
		writeJSONLine(writer, map[string]string{"type": "error", "error": "invalid token"})
		return
	}

	c := &client{
		id:       auth.GenerateID("tcp"),
		userID:   claims.UserID,
		username: claims.Username,
		conn:     conn,
		writer:   writer,
	}

	s.mu.Lock()
	s.clients[c.id] = c
	count := len(s.clients)
	s.mu.Unlock()

	log.Printf("[TCP] %s connected (user=%s, total=%d)", c.id, c.username, count)
	writeJSONLine(writer, map[string]interface{}{
		"type":       "welcome",
		"session_id": c.id,
		"username":   c.username,
	})

	defer func() {
		s.mu.Lock()
		delete(s.clients, c.id)
		s.mu.Unlock()
		log.Printf("[TCP] %s disconnected", c.id)
	}()

	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			return
		}
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var msg map[string]interface{}
		if err := json.Unmarshal([]byte(line), &msg); err != nil {
			c.send(map[string]string{"type": "error", "error": "invalid json"})
			continue
		}
		t, _ := msg["type"].(string)
		switch t {
		case "ping":
			c.send(map[string]interface{}{"type": "pong", "timestamp": time.Now().Unix()})
		case "progress":
			s.handleProgressFromClient(c, msg)
		default:
			c.send(map[string]string{"type": "error", "error": "unknown message type"})
		}
	}
}

func (s *Server) handleProgressFromClient(c *client, msg map[string]interface{}) {
	mangaID, _ := msg["manga_id"].(string)
	chapterFloat, _ := msg["chapter"].(float64)
	if mangaID == "" {
		c.send(map[string]string{"type": "error", "error": "manga_id required"})
		return
	}
	upd := models.ProgressUpdate{
		UserID:    c.userID,
		MangaID:   mangaID,
		Chapter:   int(chapterFloat),
		Timestamp: time.Now().Unix(),
	}
	s.broker.PublishProgress(upd)
	c.send(map[string]interface{}{"type": "ack", "manga_id": mangaID, "chapter": upd.Chapter})
}

func (s *Server) broadcastLoop() {
	ch := s.broker.SubscribeProgress()
	defer s.broker.UnsubscribeProgress(ch)
	for {
		select {
		case <-s.stop:
			return
		case upd, ok := <-ch:
			if !ok {
				return
			}
			s.broadcastProgress(upd)
		}
	}
}

func (s *Server) broadcastProgress(upd models.ProgressUpdate) {
	payload := map[string]interface{}{
		"type":      "progress_update",
		"user_id":   upd.UserID,
		"manga_id":  upd.MangaID,
		"chapter":   upd.Chapter,
		"timestamp": upd.Timestamp,
	}
	s.mu.RLock()
	targets := make([]*client, 0, len(s.clients))
	for _, c := range s.clients {
		if c.userID == upd.UserID {
			targets = append(targets, c)
		}
	}
	s.mu.RUnlock()
	for _, c := range targets {
		if err := c.send(payload); err != nil {
			log.Printf("[TCP] send to %s failed: %v", c.id, err)
		}
	}
}

func (c *client) send(payload interface{}) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	return writeJSONLine(c.writer, payload)
}

func writeJSONLine(w *bufio.Writer, payload interface{}) error {
	if w == nil {
		return errors.New("nil writer")
	}
	b, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	if _, err := w.Write(b); err != nil {
		return err
	}
	if err := w.WriteByte('\n'); err != nil {
		return err
	}
	return w.Flush()
}
