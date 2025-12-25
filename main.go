// Package main provides the entry point for the yttg application
package main

import (
	"flag"
	"fmt"
	"log"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	"github.com/rusik69/yttg/pkg/config"
	"github.com/rusik69/yttg/pkg/database"
	"github.com/rusik69/yttg/pkg/telegram"
	"github.com/rusik69/yttg/pkg/torrent"
)

func main() {
	torrentsFile := flag.String("torrents", "", "Path to torrents file (overrides TORRENTS_FILE env)")
	dbURL := flag.String("db", "", "PostgreSQL connection URL (overrides DATABASE_URL env)")
	downloadDir := flag.String("download-dir", "", "Download directory (overrides DOWNLOAD_DIR env)")
	dryRun := flag.Bool("dry-run", false, "Only list torrents without downloading or uploading")
	cleanup := flag.Bool("cleanup", true, "Delete files after download/upload to free disk space")
	flag.Parse()

	cfg, err := config.NewConfig(*dryRun)
	if err != nil {
		log.Fatalf("Failed to load configuration: %v", err)
	}

	if *torrentsFile != "" {
		cfg.TorrentsFile = *torrentsFile
	}
	if *dbURL != "" {
		cfg.DatabaseURL = *dbURL
	}
	if *downloadDir != "" {
		cfg.DownloadDir = *downloadDir
	}

	torrents, err := config.ReadTorrents(cfg.TorrentsFile)
	if err != nil {
		log.Fatalf("Failed to read torrents: %v", err)
	}

	log.Printf("Loaded %d torrents from %s", len(torrents), cfg.TorrentsFile)

	db, err := database.New(cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("Failed to initialize database: %v", err)
	}
	defer db.Close()

	downloader, err := torrent.NewDownloader(cfg.DownloadDir)
	if err != nil {
		log.Fatalf("Failed to initialize torrent downloader: %v", err)
	}
	defer downloader.Close()

	var uploader *telegram.Uploader
	if !*dryRun {
		log.Printf("Initializing Telegram uploader with API URL: %s", cfg.TelegramAPIURL)

		// Wait for telegram-bot-api to be ready
		log.Printf("Waiting for Telegram Bot API Server to be ready...")
		client := &http.Client{Timeout: 5 * time.Second}
		maxRetries := 30
		for i := 0; i < maxRetries; i++ {
			resp, err := client.Get(cfg.TelegramAPIURL + "/health")
			if err == nil {
				resp.Body.Close()
				if resp.StatusCode == 200 || resp.StatusCode == 404 {
					log.Printf("Telegram Bot API Server HTTP endpoint is ready, waiting additional 10s for full initialization...")
					time.Sleep(10 * time.Second)
					break
				}
			}
			if i < maxRetries-1 {
				log.Printf("Waiting for Telegram Bot API Server... (attempt %d/%d)", i+1, maxRetries)
				time.Sleep(2 * time.Second)
			} else {
				log.Printf("Warning: Telegram Bot API Server may not be ready, continuing anyway...")
			}
		}

		var err error
		uploader, err = telegram.NewUploader(cfg.TelegramToken, cfg.TelegramChatID, cfg.TelegramAPIURL)
		if err != nil {
			log.Fatalf("Failed to initialize Telegram uploader: %v", err)
		}
		log.Printf("Telegram uploader initialized, testing connection...")

		// Test connection with retries
		var botName string
		var connErr error
		connMaxRetries := 10
		for i := 0; i < connMaxRetries; i++ {
			log.Printf("Attempting to connect to Telegram Bot API (attempt %d/%d)...", i+1, connMaxRetries)
			botName, connErr = uploader.GetMe()
			if connErr == nil {
				break
			}
			if i < connMaxRetries-1 {
				waitTime := time.Duration(i+1) * 3 * time.Second
				log.Printf("Connection attempt %d/%d failed: %v, retrying in %v...", i+1, connMaxRetries, connErr, waitTime)
				time.Sleep(waitTime)
			}
		}
		if connErr != nil {
			log.Fatalf("Failed to connect to Telegram after %d attempts (API URL: %s): %v", connMaxRetries, cfg.TelegramAPIURL, connErr)
		}
		log.Printf("Connected to Telegram as @%s (Local Bot API Server, 2GB limit)", botName)
	} else {
		log.Printf("Dry-run mode: Skipping Telegram initialization")
	}

	totalDownloaded := 0
	totalUploaded := 0
	totalSkipped := 0
	totalSkippedFiles := 0

	for _, torrentURL := range torrents {
		// Check database connection before processing each torrent
		if err := db.Ping(); err != nil {
			log.Fatalf("Database connection lost: %v. Stopping downloads.", err)
		}

		// Use torrent URL as ID for database tracking
		torrentID := torrentURL

		// Get torrent info to check file sizes before download
		info, totalSize, files, err := downloader.GetTorrentInfo(torrentURL)
		if err != nil {
			log.Printf("Warning: Failed to get info for %s: %v", torrentURL, err)
			continue
		}

		totalSizeGB := float64(totalSize) / (1024 * 1024 * 1024)
		maxFileSize := int64(2 * 1024 * 1024 * 1024)

		// Filter to only video files <= 2GB
		videoExts := map[string]bool{
			".mp4": true, ".avi": true, ".mkv": true, ".mov": true, ".wmv": true,
			".flv": true, ".webm": true, ".m4v": true, ".mpg": true, ".mpeg": true,
			".3gp": true, ".ogv": true, ".ts": true, ".m2ts": true, ".mts": true,
		}

		var filesToProcess []torrent.FileInfo
		for _, fileInfo := range files {
			ext := strings.ToLower(filepath.Ext(fileInfo.Path))
			if videoExts[ext] && fileInfo.Size <= maxFileSize {
				// Check database connection before checking if file is downloaded
				if err := db.Ping(); err != nil {
					log.Fatalf("Database connection lost: %v. Stopping downloads.", err)
				}

				// Check if this specific file has already been downloaded
				downloaded, err := db.IsVideoDownloaded(torrentID, fileInfo.Path)
				if err != nil {
					log.Fatalf("Failed to check file %s (database connection may be lost): %v. Stopping downloads.", fileInfo.Path, err)
				}
				if downloaded {
					log.Printf("Skipping already downloaded file: %s", fileInfo.Path)
					totalSkippedFiles++
					continue
				}
				filesToProcess = append(filesToProcess, fileInfo)
			} else {
				totalSkippedFiles++
			}
		}

		// Count files that will be downloaded vs skipped
		var filesToDownload, filesToSkip int
		for _, file := range files {
			if file.Size > maxFileSize {
				filesToSkip++
			} else {
				filesToDownload++
			}
		}

		if *dryRun {
			fmt.Printf("Would download: %s - %s (total: %.2f GB, %d files <= 2GB, %d files > 2GB will be skipped)\n",
				torrentURL, info, totalSizeGB, filesToDownload, filesToSkip)
			continue
		}

		if len(filesToProcess) == 0 {
			log.Printf("Skipping torrent %s (%s): no new video files <= 2GB to download", torrentURL, info)
			continue
		}

		// Process torrent: download -> upload -> delete for each file
		log.Printf("Processing torrent: %s (%s, total: %.2f GB, %d files to download, %d files already downloaded or skipped)",
			torrentURL, info, totalSizeGB, len(filesToProcess), len(files)-len(filesToProcess))

		// Add torrent once for processing all files
		t, err := downloader.GetOrAddTorrent(torrentURL)
		if err != nil {
			log.Printf("Warning: Failed to add torrent %s: %v", torrentURL, err)
			continue
		}

		// Wait for metadata
		<-t.GotInfo()

		// Process each file: download -> upload -> delete
		for i, fileInfo := range filesToProcess {
			// Check database connection before downloading each file
			if err := db.Ping(); err != nil {
				log.Fatalf("Database connection lost: %v. Stopping downloads.", err)
			}

			// Download this file
			filePath, err := downloader.DownloadSingleFile(t, fileInfo.Path, info, i, len(filesToProcess))
			if err != nil {
				log.Printf("Warning: Failed to download file %s: %v", fileInfo.Path, err)
				continue
			}

			// Add file to database immediately after download to prevent re-downloading
			// Use file path within torrent as the identifier
			if err := db.AddVideo(torrentID, torrentURL, info, fileInfo.Path); err != nil {
				log.Fatalf("Failed to add file to database (connection may be lost): %v. Stopping downloads.", err)
			}

			// Upload to Telegram
			fileName := filepath.Base(filePath)
			title := fmt.Sprintf("%s - %s", info, fileName)

			log.Printf("Uploading to Telegram: %s", title)
			telegramFileID, err := uploader.UploadDocument(filePath, title)
			if err != nil {
				log.Printf("Warning: Failed to upload file %s: %v", filePath, err)
				// Continue with next file even if upload fails
				continue
			}
			log.Printf("Uploaded: %s (File ID: %s)", title, telegramFileID)
			totalUploaded++

			// Store Telegram file ID in database
			// Check connection first
			if err := db.Ping(); err != nil {
				log.Fatalf("Database connection lost: %v. Stopping downloads.", err)
			}
			if err := db.UpdateTelegramFileID(torrentID, fileInfo.Path, telegramFileID); err != nil {
				log.Fatalf("Failed to store Telegram file ID (database connection may be lost): %v. Stopping downloads.", err)
			}

			// Mark file as uploaded in database
			// Check connection first
			if err := db.Ping(); err != nil {
				log.Fatalf("Database connection lost: %v. Stopping downloads.", err)
			}
			if err := db.MarkUploaded(torrentID, fileInfo.Path); err != nil {
				log.Printf("Warning: Failed to mark file as uploaded: %v", err)
				// Don't fatal here as upload already succeeded, just log warning
			}

			// Delete file immediately after upload
			if *cleanup {
				if err := downloader.CleanupFile(filePath); err != nil {
					log.Printf("Warning: Failed to cleanup file %s: %v", filePath, err)
				} else {
					log.Printf("Cleaned up: %s", filePath)
				}
			}

			totalDownloaded++
		}

		// Stop torrent to prevent seeding
		if err := downloader.StopTorrent(torrentURL); err != nil {
			log.Printf("Warning: Failed to stop torrent %s: %v", torrentURL, err)
		}
	}

	// Summary
	log.Printf("=== Summary ===")
	log.Printf("Torrents downloaded: %d", totalDownloaded)
	log.Printf("Files uploaded: %d", totalUploaded)
	log.Printf("Torrents skipped (already downloaded): %d", totalSkipped)
	log.Printf("Files skipped (exceeds 2GB per-file limit): %d", totalSkippedFiles)

	log.Printf("Done!")
}
