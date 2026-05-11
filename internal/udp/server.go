package udp

import (
	"encoding/json"
	"fmt"
	"log"
	"net"
	"strings"
	"sync"
	"time"

	"github.com/mangahub/mangahub/pkg/models"
	"github.com/mangahub/mangahub/pkg/protocol"
)

type Server struct {
	addr    string
	conn    *net.UDPConn
	broker  *protocol.Broker
	mu      sync.RWMutex
	clients map[string]clientInfo
	stop    chan struct{}
}

type clientInfo struct {
	addr       *net.UDPAddr
	registered time.Time
	genres     []string
}

func NewServer(addr string) *Server {
	return &Server{
		addr:    addr,
		broker:  protocol.Default(),
		clients: make(map[string]clientInfo),
		stop:    make(chan struct{}),
	}
}

func (s *Server) Start() error {
	udpAddr, err := net.ResolveUDPAddr("udp", s.addr)
	if err != nil {
		return fmt.Errorf("resolve udp %s: %w", s.addr, err)
	}
	conn, err := net.ListenUDP("udp", udpAddr)
	if err != nil {
		return fmt.Errorf("listen udp: %w", err)
	}
	s.conn = conn
	log.Printf("[UDP] Listening on %s", s.addr)

	go s.broadcastLoop()
	return s.readLoop()
}

func (s *Server) Stop() {
	close(s.stop)
	if s.conn != nil {
		s.conn.Close()
	}
}

func (s *Server) readLoop() error {
	buf := make([]byte, 4096)
	for {
		n, addr, err := s.conn.ReadFromUDP(buf)
		if err != nil {
			select {
			case <-s.stop:
				return nil
			default:
				log.Printf("[UDP] read error: %v", err)
				continue
			}
		}
		s.handlePacket(buf[:n], addr)
	}
}

func (s *Server) handlePacket(data []byte, addr *net.UDPAddr) {
	var msg map[string]interface{}
	if err := json.Unmarshal(data, &msg); err != nil {
		log.Printf("[UDP] invalid packet from %s: %v", addr, err)
		return
	}
	t, _ := msg["type"].(string)
	switch t {
	case "register":
		s.registerClient(addr, msg)
	case "unregister":
		s.unregisterClient(addr)
	case "ping":
		s.sendTo(addr, map[string]interface{}{"type": "pong", "timestamp": time.Now().Unix()})
	default:
		s.sendTo(addr, map[string]string{"type": "error", "error": "unknown type"})
	}
}

func (s *Server) registerClient(addr *net.UDPAddr, msg map[string]interface{}) {
	key := addr.String()
	genres := []string{}
	if g, ok := msg["genres"].([]interface{}); ok {
		for _, x := range g {
			if str, ok := x.(string); ok {
				genres = append(genres, strings.ToLower(str))
			}
		}
	}
	s.mu.Lock()
	s.clients[key] = clientInfo{
		addr:       addr,
		registered: time.Now(),
		genres:     genres,
	}
	count := len(s.clients)
	s.mu.Unlock()
	log.Printf("[UDP] registered %s (total=%d)", key, count)
	s.sendTo(addr, map[string]interface{}{
		"type":      "registered",
		"timestamp": time.Now().Unix(),
		"genres":    genres,
	})
}

func (s *Server) unregisterClient(addr *net.UDPAddr) {
	key := addr.String()
	s.mu.Lock()
	delete(s.clients, key)
	s.mu.Unlock()
	log.Printf("[UDP] unregistered %s", key)
}

func (s *Server) sendTo(addr *net.UDPAddr, payload interface{}) {
	b, err := json.Marshal(payload)
	if err != nil {
		return
	}
	if _, err := s.conn.WriteToUDP(b, addr); err != nil {
		log.Printf("[UDP] write to %s failed: %v", addr, err)
	}
}

func (s *Server) broadcastLoop() {
	ch := s.broker.SubscribeNotification()
	defer s.broker.UnsubscribeNotification(ch)
	for {
		select {
		case <-s.stop:
			return
		case n, ok := <-ch:
			if !ok {
				return
			}
			s.broadcast(n)
		}
	}
}

func (s *Server) broadcast(n models.Notification) {
	b, err := json.Marshal(n)
	if err != nil {
		return
	}
	s.mu.RLock()
	targets := make([]*net.UDPAddr, 0, len(s.clients))
	for _, c := range s.clients {
		targets = append(targets, c.addr)
	}
	s.mu.RUnlock()
	for _, addr := range targets {
		if _, err := s.conn.WriteToUDP(b, addr); err != nil {
			log.Printf("[UDP] broadcast to %s failed: %v", addr, err)
		}
	}
	log.Printf("[UDP] broadcasted notification to %d clients", len(targets))
}
