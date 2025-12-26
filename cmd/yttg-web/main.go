// Package main provides the entry point for the yttg-web web interface
package main

import (
	"flag"
	"log"
	"net/http"

	"github.com/rusik69/yttg/pkg/config"
	"github.com/rusik69/yttg/pkg/database"
	"github.com/rusik69/yttg/pkg/web"
)

func main() {
	dbURL := flag.String("db", "", "PostgreSQL connection URL (overrides DATABASE_URL env)")
	port := flag.String("port", "8080", "Port to listen on")
	downloadDir := flag.String("download-dir", "", "Download directory for videos (overrides DOWNLOAD_DIR env)")
	flag.Parse()

	// Web interface no longer needs Telegram credentials - it uses yttg API instead
	cfg, err := config.NewConfig(true) // Skip Telegram credentials
	if err != nil {
		log.Fatalf("Failed to load configuration: %v", err)
	}

	if *dbURL != "" {
		cfg.DatabaseURL = *dbURL
	}

	if *downloadDir != "" {
		cfg.DownloadDir = *downloadDir
	}

	// Initialize database
	db, err := database.New(cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("Failed to initialize database: %v", err)
	}
	defer db.Close()

	// Initialize web server
	server := web.NewServer(db, cfg.DownloadDir, cfg.YTTGAPIURL, cfg.WebUsername, cfg.WebPassword, cfg.TelegramToken, cfg.TelegramChatID, cfg.TelegramAPIURL)

	log.Printf("Starting web server on port %s", *port)
	if err := http.ListenAndServe(":"+*port, server); err != nil {
		log.Fatalf("Failed to start web server: %v", err)
	}
}
