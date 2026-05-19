package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/gorilla/websocket"
	grpcsvc "github.com/mangahub/mangahub/internal/grpc"
)

const (
	defaultHTTPURL  = "http://10.238.30.205:8080"
	defaultTCPAddr  = "10.238.30.205:9090"
	defaultUDPAddr  = "10.238.30.205:9091"
	defaultGRPCAddr = "10.238.30.205:9092"
	defaultWSURL    = "ws://10.238.30.205:9093/ws"
)

type session struct {
	Token    string `json:"token"`
	UserID   string `json:"user_id"`
	Username string `json:"username"`
}

func sessionPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".mangahub-session.json")
}

func saveSession(s *session) error {
	b, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(sessionPath(), b, 0600)
}

func loadSession() (*session, error) {
	b, err := os.ReadFile(sessionPath())
	if err != nil {
		return nil, err
	}
	s := &session{}
	if err := json.Unmarshal(b, s); err != nil {
		return nil, err
	}
	return s, nil
}

func clearSession() {
	os.Remove(sessionPath())
}

func httpJSON(method, urlStr string, body interface{}, token string, out interface{}) error {
	var reqBody io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return err
		}
		reqBody = bytes.NewReader(b)
	}
	req, err := http.NewRequest(method, urlStr, reqBody)
	if err != nil {
		return err
	}
	if reqBody != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 400 {
		return fmt.Errorf("http %d: %s", resp.StatusCode, string(data))
	}
	if out != nil && len(data) > 0 {
		return json.Unmarshal(data, out)
	}
	return nil
}

func usage() {
	fmt.Print(`MangaHub CLI

Usage:
  mangahub <command> [args]

Authentication:
  register <username> <email> <password>
  login    <username> <password>
  logout
  status

Manga:
  search   <query> [--genre G] [--status S]
  info     <manga-id>
  list

Library:
  library                          show user library
  library:add    <manga-id> [status] [chapter]
  library:remove <manga-id>
  progress       <manga-id> <chapter>
  rating         <manga-id> <rating 0-10>

Network protocols:
  tcp:sync                         connect to TCP sync server, listen for updates
  udp:subscribe [genres,...]       subscribe to UDP notifications
  udp:notify <manga-id> <message>  trigger an admin UDP notification
  grpc:get      <manga-id>         get manga via gRPC
  grpc:search   <query>            search via gRPC
  grpc:progress <manga-id> <ch>    update progress via gRPC
  chat [room]                      open WebSocket chat (default room: general)

Other:
  health                           hit HTTP /health endpoint
  version
`)
}

func main() {
	if len(os.Args) < 2 {
		usage()
		return
	}
	cmd := os.Args[1]
	args := os.Args[2:]

	switch cmd {
	case "help", "-h", "--help":
		usage()
	case "version":
		fmt.Println("mangahub-cli 1.0.0")
	case "register":
		cmdRegister(args)
	case "login":
		cmdLogin(args)
	case "logout":
		clearSession()
		fmt.Println("✓ Logged out")
	case "status":
		cmdStatus()
	case "search":
		cmdSearch(args)
	case "info":
		cmdInfo(args)
	case "list":
		cmdList()
	case "library":
		cmdLibrary(args)
	case "library:add":
		cmdLibraryAdd(args)
	case "library:remove":
		cmdLibraryRemove(args)
	case "progress":
		cmdProgress(args)
	case "rating":
		cmdRating(args)
	case "tcp:sync":
		cmdTCPSync(args)
	case "udp:subscribe":
		cmdUDPSubscribe(args)
	case "udp:notify":
		cmdUDPNotify(args)
	case "grpc:get":
		cmdGRPCGet(args)
	case "grpc:search":
		cmdGRPCSearch(args)
	case "grpc:progress":
		cmdGRPCProgress(args)
	case "chat":
		cmdChat(args)
	case "health":
		cmdHealth()
	default:
		fmt.Printf("Unknown command: %s\n\n", cmd)
		usage()
		os.Exit(1)
	}
}

func cmdRegister(args []string) {
	if len(args) < 3 {
		fmt.Println("usage: register <username> <email> <password>")
		os.Exit(1)
	}
	body := map[string]string{
		"username": args[0],
		"email":    args[1],
		"password": args[2],
	}
	var resp map[string]interface{}
	if err := httpJSON("POST", defaultHTTPURL+"/auth/register", body, "", &resp); err != nil {
		fmt.Println("✗", err)
		os.Exit(1)
	}
	fmt.Println("✓ Account created")
	pretty(resp)
}

func cmdLogin(args []string) {
	if len(args) < 2 {
		fmt.Println("usage: login <username> <password>")
		os.Exit(1)
	}
	body := map[string]string{
		"username": args[0],
		"password": args[1],
	}
	var resp session
	if err := httpJSON("POST", defaultHTTPURL+"/auth/login", body, "", &resp); err != nil {
		fmt.Println("✗", err)
		os.Exit(1)
	}
	if err := saveSession(&resp); err != nil {
		fmt.Println("✗ save session:", err)
		os.Exit(1)
	}
	fmt.Printf("✓ Logged in as %s (id=%s)\n", resp.Username, resp.UserID)
}

func cmdStatus() {
	s, err := loadSession()
	if err != nil {
		fmt.Println("Not logged in")
		return
	}
	fmt.Printf("Logged in as %s (id=%s)\n", s.Username, s.UserID)
}

func cmdSearch(args []string) {
	if len(args) < 1 {
		fmt.Println("usage: search <query> [--genre G] [--status S]")
		os.Exit(1)
	}
	q := args[0]
	genre := ""
	status := ""
	for i := 1; i < len(args)-1; i++ {
		if args[i] == "--genre" {
			genre = args[i+1]
		}
		if args[i] == "--status" {
			status = args[i+1]
		}
	}
	u, _ := url.Parse(defaultHTTPURL + "/manga")
	qs := u.Query()
	qs.Set("q", q)
	if genre != "" {
		qs.Set("genre", genre)
	}
	if status != "" {
		qs.Set("status", status)
	}
	u.RawQuery = qs.Encode()

	var resp map[string]interface{}
	if err := httpJSON("GET", u.String(), nil, "", &resp); err != nil {
		fmt.Println("✗", err)
		os.Exit(1)
	}
	pretty(resp)
}

func cmdInfo(args []string) {
	if len(args) < 1 {
		fmt.Println("usage: info <manga-id>")
		os.Exit(1)
	}
	tok := ""
	if s, err := loadSession(); err == nil {
		tok = s.Token
	}
	var resp map[string]interface{}
	if err := httpJSON("GET", defaultHTTPURL+"/manga/"+args[0], nil, tok, &resp); err != nil {
		fmt.Println("✗", err)
		os.Exit(1)
	}
	pretty(resp)
}

func cmdList() {
	var resp map[string]interface{}
	if err := httpJSON("GET", defaultHTTPURL+"/manga?limit=50", nil, "", &resp); err != nil {
		fmt.Println("✗", err)
		os.Exit(1)
	}
	pretty(resp)
}

func cmdLibrary(args []string) {
	s, err := loadSession()
	if err != nil {
		fmt.Println("✗ not logged in. run: mangahub login <u> <p>")
		os.Exit(1)
	}
	u, _ := url.Parse(defaultHTTPURL + "/users/library")
	if len(args) > 0 && strings.HasPrefix(args[0], "--status=") {
		qs := u.Query()
		qs.Set("status", strings.TrimPrefix(args[0], "--status="))
		u.RawQuery = qs.Encode()
	}
	var resp map[string]interface{}
	if err := httpJSON("GET", u.String(), nil, s.Token, &resp); err != nil {
		fmt.Println("✗", err)
		os.Exit(1)
	}
	pretty(resp)
}

func cmdLibraryAdd(args []string) {
	if len(args) < 1 {
		fmt.Println("usage: library:add <manga-id> [status] [chapter]")
		os.Exit(1)
	}
	s, err := loadSession()
	if err != nil {
		fmt.Println("✗ not logged in")
		os.Exit(1)
	}
	body := map[string]interface{}{"manga_id": args[0]}
	if len(args) >= 2 {
		body["status"] = args[1]
	}
	if len(args) >= 3 {
		var ch int
		fmt.Sscanf(args[2], "%d", &ch)
		body["chapter"] = ch
	}
	var resp map[string]interface{}
	if err := httpJSON("POST", defaultHTTPURL+"/users/library", body, s.Token, &resp); err != nil {
		fmt.Println("✗", err)
		os.Exit(1)
	}
	fmt.Println("✓ Added to library")
}

func cmdLibraryRemove(args []string) {
	if len(args) < 1 {
		fmt.Println("usage: library:remove <manga-id>")
		os.Exit(1)
	}
	s, err := loadSession()
	if err != nil {
		fmt.Println("✗ not logged in")
		os.Exit(1)
	}
	if err := httpJSON("DELETE", defaultHTTPURL+"/users/library/"+args[0], nil, s.Token, nil); err != nil {
		fmt.Println("✗", err)
		os.Exit(1)
	}
	fmt.Println("✓ Removed")
}

func cmdProgress(args []string) {
	if len(args) < 2 {
		fmt.Println("usage: progress <manga-id> <chapter>")
		os.Exit(1)
	}
	s, err := loadSession()
	if err != nil {
		fmt.Println("✗ not logged in")
		os.Exit(1)
	}
	var ch int
	fmt.Sscanf(args[1], "%d", &ch)
	body := map[string]interface{}{"manga_id": args[0], "chapter": ch}
	var resp map[string]interface{}
	if err := httpJSON("PUT", defaultHTTPURL+"/users/progress", body, s.Token, &resp); err != nil {
		fmt.Println("✗", err)
		os.Exit(1)
	}
	fmt.Printf("✓ Progress updated to chapter %d\n", ch)
}

func cmdRating(args []string) {
	if len(args) < 2 {
		fmt.Println("usage: rating <manga-id> <0-10>")
		os.Exit(1)
	}
	s, err := loadSession()
	if err != nil {
		fmt.Println("✗ not logged in")
		os.Exit(1)
	}
	var r int
	fmt.Sscanf(args[1], "%d", &r)
	body := map[string]interface{}{"manga_id": args[0], "rating": r}
	if err := httpJSON("PUT", defaultHTTPURL+"/users/rating", body, s.Token, nil); err != nil {
		fmt.Println("✗", err)
		os.Exit(1)
	}
	fmt.Println("✓ Rating updated")
}

func cmdTCPSync(args []string) {
	s, err := loadSession()
	if err != nil {
		fmt.Println("✗ not logged in")
		os.Exit(1)
	}
	addr := defaultTCPAddr
	fs := flag.NewFlagSet("tcp:sync", flag.ContinueOnError)
	fs.StringVar(&addr, "addr", addr, "tcp server address")
	fs.Parse(args)

	conn, err := net.Dial("tcp", addr)
	if err != nil {
		fmt.Println("✗ dial tcp:", err)
		os.Exit(1)
	}
	defer conn.Close()
	fmt.Printf("✓ Connected to TCP sync server at %s\n", addr)

	authMsg, _ := json.Marshal(map[string]string{
		"type":  "auth",
		"token": s.Token,
	})
	conn.Write(append(authMsg, '\n'))

	go func() {
		ticker := time.NewTicker(20 * time.Second)
		defer ticker.Stop()
		for range ticker.C {
			ping, _ := json.Marshal(map[string]string{"type": "ping"})
			conn.Write(append(ping, '\n'))
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-stop
		conn.Close()
		os.Exit(0)
	}()

	reader := bufio.NewReader(conn)
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			fmt.Println("✗ disconnected:", err)
			return
		}
		fmt.Print("[TCP] ", line)
	}
}

func cmdUDPSubscribe(args []string) {
	addr := defaultUDPAddr
	udpAddr, err := net.ResolveUDPAddr("udp", addr)
	if err != nil {
		fmt.Println("✗ resolve:", err)
		os.Exit(1)
	}
	conn, err := net.DialUDP("udp", nil, udpAddr)
	if err != nil {
		fmt.Println("✗ dial:", err)
		os.Exit(1)
	}
	defer conn.Close()

	genres := []string{}
	if len(args) > 0 {
		genres = strings.Split(args[0], ",")
	}
	regMsg, _ := json.Marshal(map[string]interface{}{
		"type":   "register",
		"genres": genres,
	})
	conn.Write(regMsg)
	fmt.Printf("✓ Subscribed to UDP notifications at %s\n", addr)

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-stop
		unreg, _ := json.Marshal(map[string]string{"type": "unregister"})
		conn.Write(unreg)
		conn.Close()
		os.Exit(0)
	}()

	buf := make([]byte, 4096)
	for {
		n, err := conn.Read(buf)
		if err != nil {
			fmt.Println("✗ read:", err)
			return
		}
		fmt.Printf("[UDP] %s\n", string(buf[:n]))
	}
}

func cmdUDPNotify(args []string) {
	if len(args) < 2 {
		fmt.Println("usage: udp:notify <manga-id> <message>")
		os.Exit(1)
	}
	body := map[string]string{
		"manga_id": args[0],
		"message":  strings.Join(args[1:], " "),
	}
	if err := httpJSON("POST", defaultHTTPURL+"/admin/notify", body, "", nil); err != nil {
		fmt.Println("✗", err)
		os.Exit(1)
	}
	fmt.Println("✓ Notification broadcasted")
}

func cmdGRPCGet(args []string) {
	if len(args) < 1 {
		fmt.Println("usage: grpc:get <manga-id>")
		os.Exit(1)
	}
	c, err := grpcsvc.NewClient(defaultGRPCAddr)
	if err != nil {
		fmt.Println("✗", err)
		os.Exit(1)
	}
	defer c.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	out, err := c.GetManga(ctx, args[0])
	if err != nil {
		fmt.Println("✗", err)
		os.Exit(1)
	}
	pretty(out)
}

func cmdGRPCSearch(args []string) {
	if len(args) < 1 {
		fmt.Println("usage: grpc:search <query>")
		os.Exit(1)
	}
	c, err := grpcsvc.NewClient(defaultGRPCAddr)
	if err != nil {
		fmt.Println("✗", err)
		os.Exit(1)
	}
	defer c.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	out, err := c.SearchManga(ctx, &grpcsvc.SearchRequest{Query: args[0], Limit: 20})
	if err != nil {
		fmt.Println("✗", err)
		os.Exit(1)
	}
	pretty(out)
}

func cmdGRPCProgress(args []string) {
	if len(args) < 2 {
		fmt.Println("usage: grpc:progress <manga-id> <chapter>")
		os.Exit(1)
	}
	s, err := loadSession()
	if err != nil {
		fmt.Println("✗ not logged in")
		os.Exit(1)
	}
	c, err := grpcsvc.NewClient(defaultGRPCAddr)
	if err != nil {
		fmt.Println("✗", err)
		os.Exit(1)
	}
	defer c.Close()
	var ch int
	fmt.Sscanf(args[1], "%d", &ch)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	out, err := c.UpdateProgress(ctx, s.UserID, args[0], ch)
	if err != nil {
		fmt.Println("✗", err)
		os.Exit(1)
	}
	pretty(out)
}

func cmdChat(args []string) {
	s, err := loadSession()
	if err != nil {
		fmt.Println("✗ not logged in")
		os.Exit(1)
	}
	room := "general"
	if len(args) > 0 {
		room = args[0]
	}
	u, _ := url.Parse(defaultWSURL)
	qs := u.Query()
	qs.Set("token", s.Token)
	qs.Set("room", room)
	u.RawQuery = qs.Encode()

	conn, _, err := websocket.DefaultDialer.Dial(u.String(), nil)
	if err != nil {
		fmt.Println("✗ dial ws:", err)
		os.Exit(1)
	}
	defer conn.Close()
	fmt.Printf("✓ Connected to chat room '%s' as %s\n", room, s.Username)
	fmt.Println("Type a message and press Enter. Use /quit to leave.")

	go func() {
		for {
			_, data, err := conn.ReadMessage()
			if err != nil {
				return
			}
			var msg map[string]interface{}
			if err := json.Unmarshal(data, &msg); err == nil {
				t, _ := msg["type"].(string)
				switch t {
				case "message":
					fmt.Printf("[%s] %s: %s\n", room, msg["username"], msg["message"])
				case "history":
					fmt.Printf("(history) %s: %s\n", msg["username"], msg["message"])
				case "join":
					fmt.Printf("--> %s joined\n", msg["username"])
				case "leave":
					fmt.Printf("<-- %s left\n", msg["username"])
				default:
					fmt.Printf("[%s] %s\n", t, string(data))
				}
			} else {
				fmt.Println(string(data))
			}
		}
	}()

	scanner := bufio.NewScanner(os.Stdin)
	for scanner.Scan() {
		text := strings.TrimSpace(scanner.Text())
		if text == "" {
			continue
		}
		if text == "/quit" {
			conn.WriteMessage(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""))
			return
		}
		payload, _ := json.Marshal(map[string]string{"message": text})
		if err := conn.WriteMessage(websocket.TextMessage, payload); err != nil {
			fmt.Println("✗ send:", err)
			return
		}
	}
}

func cmdHealth() {
	resp, err := http.Get(defaultHTTPURL + "/health")
	if err != nil {
		fmt.Println("✗ HTTP API unreachable:", err)
		os.Exit(1)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	fmt.Printf("HTTP API: %s %s\n", resp.Status, string(body))
}

func pretty(v interface{}) {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		fmt.Println(v)
		return
	}
	fmt.Println(string(b))
}
