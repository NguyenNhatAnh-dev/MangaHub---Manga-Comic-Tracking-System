package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/mangahub/mangahub/internal/config"
	"github.com/mangahub/mangahub/internal/tcp"
)

func main() {
	cfgPath := flag.String("config", "config.yaml", "path to config file")
	flag.Parse()
	cfg, err := config.Load(*cfgPath)
	if err != nil {
		log.Fatalf("load config: %v", err)
	}
	addr := fmt.Sprintf("%s:%d", cfg.Server.Host, cfg.Server.TCPPort)
	srv := tcp.NewServer(addr, cfg.Auth.JWTSecret)

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-stop
		srv.Stop()
	}()

	if err := srv.Start(); err != nil {
		log.Fatalf("tcp server: %v", err)
	}
}
