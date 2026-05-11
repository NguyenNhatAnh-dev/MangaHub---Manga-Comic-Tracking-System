package main

import (
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/mangahub/mangahub/internal/config"
	"github.com/mangahub/mangahub/internal/database"
	wsserver "github.com/mangahub/mangahub/internal/websocket"
)

func main() {
	cfgPath := flag.String("config", "config.yaml", "path to config file")
	flag.Parse()
	cfg, err := config.Load(*cfgPath)
	if err != nil {
		log.Fatalf("load config: %v", err)
	}
	db, err := database.Init(cfg.Database.Path)
	if err != nil {
		log.Fatalf("init db: %v", err)
	}
	defer db.Close()

	hub := wsserver.NewHub(db, cfg.Auth.JWTSecret)
	mux := http.NewServeMux()
	mux.Handle("/ws", hub)

	addr := fmt.Sprintf("%s:%d", cfg.Server.Host, cfg.Server.WebSocketPort)
	srv := &http.Server{Addr: addr, Handler: mux}

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-stop
		srv.Close()
	}()

	log.Printf("[WS] Listening on %s/ws", addr)
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatalf("ws server: %v", err)
	}
}
