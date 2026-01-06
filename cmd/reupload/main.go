// Package main provides a tool to re-upload broken videos to Telegram
package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/rusik69/trtg/pkg/database"
	"github.com/rusik69/trtg/pkg/telegram"
	"github.com/rusik69/trtg/pkg/torrent"
)

func main() {
	videoID := flag.Int64("video-id", 0, "Video ID to re-upload (required)")
	dbURL := flag.String("db", "", "PostgreSQL connection URL (or use DATABASE_URL env)")
	downloadDir := flag.String("download-dir", "/tmp", "Download directory")
	flag.Parse()

	if *videoID == 0 {
		log.Fatal("Error: -video-id is required")
	}

	// Get database URL from flag or environment
	connURL := *dbURL
	if connURL == "" {
		connURL = os.Getenv("DATABASE_URL")
	}
	if connURL == "" {
		log.Fatal("DATABASE_URL environment variable or -db flag is required")
	}

	// Connect to database
	db, err := database.New(connURL)
	if err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}
	defer db.Close()

	// Get video info
	video, err := db.GetVideoByID(*videoID)
	if err != nil {
		log.Fatalf("Failed to get video %d: %v", *videoID, err)
	}

	log.Printf("Found video: %s S%02dE%02d - %s", video.ShowName, video.SeasonNumber, video.EpisodeNumber, video.FilePath)

	// Check if we have Telegram credentials
	token := os.Getenv("TELEGRAM_BOT_TOKEN")
	chatID := os.Getenv("TELEGRAM_CHAT_ID")
	apiURL := os.Getenv("TELEGRAM_API_URL")

	if token == "" || chatID == "" {
		log.Fatal("TELEGRAM_BOT_TOKEN and TELEGRAM_CHAT_ID environment variables are required")
	}
	if apiURL == "" {
		apiURL = "https://api.telegram.org"
	}

	log.Printf("Using Telegram API URL: %s", apiURL)

	// Initialize torrent downloader
	downloader, err := torrent.NewDownloader(*downloadDir)
	if err != nil {
		log.Fatalf("Failed to initialize torrent downloader: %v", err)
	}
	defer downloader.Close()

	// Initialize Telegram uploader
	uploader, err := telegram.NewUploader(token, chatID, apiURL)
	if err != nil {
		log.Fatalf("Failed to initialize Telegram uploader: %v", err)
	}

	// Download the specific file from torrent
	log.Printf("Downloading from torrent: %s", video.VideoID)
	log.Printf("Looking for file: %s", video.FilePath)

	// Add the torrent
	t, err := downloader.Client.AddMagnet(video.VideoID)
	if err != nil {
		log.Fatalf("Failed to add torrent: %v", err)
	}

	// Wait for metadata
	log.Printf("Waiting for torrent metadata...")
	<-t.GotInfo()

	// Find the specific file
	files := t.Files()
	var targetFile *torrent.File
	foundIdx := -1
	for i, file := range files {
		if file.Path() == video.FilePath {
			targetFile = &file
			foundIdx = i
			break
		}
	}

	if targetFile == nil {
		log.Fatalf("File not found in torrent: %s", video.FilePath)
	}

	log.Printf("Found file: %s (%.2f MB)", targetFile.Path(), float64(targetFile.Length())/1024/1024)

	// Download only this file
	targetFile.Download()

	// Wait for download to complete
	log.Printf("Downloading file...")
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for {
		completed := targetFile.BytesCompleted()
		total := targetFile.Length()

		if completed >= total {
			break
		}

		progress := float64(completed) / float64(total) * 100
		log.Printf("Progress: %.1f%% (%d / %d bytes)", progress, completed, total)
		<-ticker.C
	}

	// Get the downloaded file path
	downloadedPath := fmt.Sprintf("%s/%s", downloader.DownloadDir, targetFile.Path())
	log.Printf("Download complete: %s", downloadedPath)

	// Upload to Telegram
	log.Printf("Uploading to Telegram...")
	fileID, filePath, err := uploader.UploadVideo(downloadedPath, video.Title)
	if err != nil {
		log.Fatalf("Failed to upload to Telegram: %v", err)
	}

	log.Printf("Upload successful!")
	log.Printf("  New Telegram file ID: %s", fileID)
	log.Printf("  New Telegram file path: %s", filePath)

	// Update database with new file_id
	if err := db.UpdateVideoTelegramInfo(*videoID, fileID, filePath); err != nil {
		log.Fatalf("Failed to update database: %v", err)
	}

	log.Printf("Database updated successfully!")
	log.Printf("Video %d is now fixed and ready to play", *videoID)

	// Clean up downloaded file
	os.RemoveAll(downloadedPath)
}
