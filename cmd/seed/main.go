package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"

	"github.com/mangahub/mangahub/internal/config"
	"github.com/mangahub/mangahub/internal/database"
	"github.com/mangahub/mangahub/internal/manga"
	"github.com/mangahub/mangahub/pkg/models"
)

func main() {
	cfgPath := flag.String("config", "config.yaml", "path to config file")
	dataPath := flag.String("data", "data/manga.json", "path to manga JSON file")
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

	raw, err := os.ReadFile(*dataPath)
	if err != nil {
		log.Fatalf("read data file: %v", err)
	}

	var entries []*models.Manga
	if err := json.Unmarshal(raw, &entries); err != nil {
		log.Fatalf("parse data file: %v", err)
	}

	repo := manga.NewRepository(db)
	count := 0
	for _, m := range entries {
		if err := repo.Upsert(m); err != nil {
			log.Printf("upsert %s failed: %v", m.ID, err)
			continue
		}
		count++
	}
	fmt.Printf("Seeded %d manga entries into %s\n", count, cfg.Database.Path)
}
