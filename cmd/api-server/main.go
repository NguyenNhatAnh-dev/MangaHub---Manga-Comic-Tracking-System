package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/mangahub/mangahub/internal/config"
	"github.com/mangahub/mangahub/internal/database"
	grpcsvc "github.com/mangahub/mangahub/internal/grpc"
	"github.com/mangahub/mangahub/internal/httpapi"
	"github.com/mangahub/mangahub/internal/tcp"
	"github.com/mangahub/mangahub/internal/udp"
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

	api := httpapi.NewServer(db, cfg.Auth.JWTSecret)
	httpAddr := fmt.Sprintf("%s:%d", cfg.Server.Host, cfg.Server.HTTPPort)
	httpSrv := &http.Server{
		Addr:         httpAddr,
		Handler:      api.Handler(),
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 15 * time.Second,
	}
	go func() {
		log.Printf("[HTTP] Listening on %s", httpAddr)
		if err := httpSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("http server: %v", err)
		}
	}()

	tcpAddr := fmt.Sprintf("%s:%d", cfg.Server.Host, cfg.Server.TCPPort)
	tcpSrv := tcp.NewServer(tcpAddr, cfg.Auth.JWTSecret)
	go func() {
		if err := tcpSrv.Start(); err != nil {
			log.Printf("tcp server: %v", err)
		}
	}()

	udpAddr := fmt.Sprintf("%s:%d", cfg.Server.Host, cfg.Server.UDPPort)
	udpSrv := udp.NewServer(udpAddr)
	go func() {
		if err := udpSrv.Start(); err != nil {
			log.Printf("udp server: %v", err)
		}
	}()

	grpcAddr := fmt.Sprintf("%s:%d", cfg.Server.Host, cfg.Server.GRPCPort)
	grpcSrv := grpcsvc.NewServer(grpcAddr, db)
	go func() {
		if err := grpcSrv.Start(); err != nil {
			log.Printf("grpc server: %v", err)
		}
	}()

	wsAddr := fmt.Sprintf("%s:%d", cfg.Server.Host, cfg.Server.WebSocketPort)
	hub := wsserver.NewHub(db, cfg.Auth.JWTSecret)
	wsMux := http.NewServeMux()
	wsMux.Handle("/ws", hub)
	wsMux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status":"ok"}`))
	})
	wsHTTPSrv := &http.Server{Addr: wsAddr, Handler: wsMux}
	go func() {
		log.Printf("[WS] Listening on %s/ws", wsAddr)
		if err := wsHTTPSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Printf("ws server: %v", err)
		}
	}()

	log.Println("=========================================")
	log.Println("MangaHub all-in-one server is running")
	log.Printf(" HTTP API : http://%s", httpAddr)
	log.Printf(" TCP Sync : tcp://%s", tcpAddr)
	log.Printf(" UDP Notif: udp://%s", udpAddr)
	log.Printf(" gRPC     : grpc://%s", grpcAddr)
	log.Printf(" WebSocket: ws://%s/ws", wsAddr)
	log.Println("Press Ctrl+C to stop")
	log.Println("=========================================")

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	<-stop

	log.Println("Shutting down...")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	httpSrv.Shutdown(ctx)
	wsHTTPSrv.Shutdown(ctx)
	tcpSrv.Stop()
	udpSrv.Stop()
	grpcSrv.Stop()
	log.Println("Shutdown complete")
}
