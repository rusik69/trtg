// Package main provides the entry point for the trtg-web web interface
package main

import (
	"flag"
	"log"
	"net/http"

	"github.com/rusik69/trtg/pkg/cleanup"
	"github.com/rusik69/trtg/pkg/config"
	"github.com/rusik69/trtg/pkg/database"
	"github.com/rusik69/trtg/pkg/web"
)

func main() {
	dbURL := flag.String("db", "", "PostgreSQL connection URL (overrides DATABASE_URL env)")
	port := flag.String("port", "8080", "Port to listen on")
	downloadDir := flag.String("download-dir", "", "Download directory for videos (overrides DOWNLOAD_DIR env)")
	flag.Parse()

	// Web interface no longer needs Telegram credentials - it uses trtg API instead
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
	server := web.NewServer(db, cfg.DownloadDir, cfg.TRTGAPIURL, cfg.WebUsername, cfg.WebPassword, cfg.TelegramToken, cfg.TelegramChatID, cfg.TelegramAPIURL)

	// Start cleanup service for telegram-bot-api storage
	// Scans /var/lib/telegram-bot-api and cleans up old files to keep storage under limits
	cleanupSvc := cleanup.NewService("/var/lib/telegram-bot-api")
	cleanupSvc.Start()
	log.Printf("Started telegram-bot-api storage cleanup service (max: %d GB, %d files)", cleanup.MaxStorageGB, cleanup.MaxFiles)

	log.Printf("Starting web server on port %s", *port)
	if err := http.ListenAndServe(":"+*port, server); err != nil {
		log.Fatalf("Failed to start web server: %v", err)
	}
}
