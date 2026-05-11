package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/mangahub/mangahub/internal/config"
	"github.com/mangahub/mangahub/internal/database"
	grpcsvc "github.com/mangahub/mangahub/internal/grpc"
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

	addr := fmt.Sprintf("%s:%d", cfg.Server.Host, cfg.Server.GRPCPort)
	srv := grpcsvc.NewServer(addr, db)

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-stop
		srv.Stop()
	}()

	if err := srv.Start(); err != nil {
		log.Fatalf("grpc server: %v", err)
	}
}
