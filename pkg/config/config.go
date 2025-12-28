// Package config handles configuration and torrent list reading
package config

import (
	"bufio"
	"fmt"
	"os"
	"strings"
)

// Config holds the application configuration
type Config struct {
	TorrentsFile   string
	DatabaseURL    string // PostgreSQL connection URL
	DownloadDir    string
	TelegramToken  string
	TelegramChatID int64
	TelegramAPIURL string
	WebUsername    string
	WebPassword    string
	TRTGAPIURL     string // URL for trtg download API
}

// NewConfig creates a new configuration from environment variables
// If skipTelegram is true, Telegram-related variables are optional
func NewConfig(skipTelegram bool) (*Config, error) {
	token := os.Getenv("TELEGRAM_BOT_TOKEN")
	if token == "" && !skipTelegram {
		return nil, fmt.Errorf("TELEGRAM_BOT_TOKEN environment variable is required")
	}

	chatIDStr := os.Getenv("TELEGRAM_CHAT_ID")
	var chatID int64
	if chatIDStr != "" {
		if _, err := fmt.Sscanf(chatIDStr, "%d", &chatID); err != nil {
			return nil, fmt.Errorf("invalid TELEGRAM_CHAT_ID: %w", err)
		}
	} else if !skipTelegram {
		return nil, fmt.Errorf("TELEGRAM_CHAT_ID environment variable is required")
	}

	apiURL := os.Getenv("TELEGRAM_API_URL")
	if apiURL == "" && !skipTelegram {
		return nil, fmt.Errorf("TELEGRAM_API_URL environment variable is required (Local Bot API Server URL)")
	}
	if apiURL == "" {
		apiURL = "http://localhost:8081" // Default for dry-run
	}

	torrentsFile := os.Getenv("TORRENTS_FILE")
	if torrentsFile == "" {
		torrentsFile = "torrents.txt"
	}

	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		// Default PostgreSQL connection string (use 127.0.0.1 for IPv4 when using network_mode: host)
		dbURL = "postgres://trtg:trtg@127.0.0.1:5432/trtg?sslmode=disable"
	}

	downloadDir := os.Getenv("DOWNLOAD_DIR")
	if downloadDir == "" {
		downloadDir = "downloads"
	}

	webUsername := os.Getenv("WEB_USERNAME")
	if webUsername == "" {
		webUsername = "admin" // Default username
	}

	webPassword := os.Getenv("WEB_PASSWORD")
	if webPassword == "" {
		webPassword = "admin" // Default password (should be changed!)
	}

	trtgAPIURL := os.Getenv("TRTG_API_URL")
	if trtgAPIURL == "" {
		trtgAPIURL = "http://localhost:8082" // Default trtg download API URL
	}

	return &Config{
		TorrentsFile:   torrentsFile,
		DatabaseURL:    dbURL,
		DownloadDir:    downloadDir,
		TelegramToken:  token,
		TelegramChatID: chatID,
		TelegramAPIURL: apiURL,
		WebUsername:    webUsername,
		WebPassword:    webPassword,
		TRTGAPIURL:     trtgAPIURL,
	}, nil
}

// ReadTorrents reads torrent file paths or magnet links from a file
func ReadTorrents(filename string) ([]string, error) {
	file, err := os.Open(filename)
	if err != nil {
		return nil, fmt.Errorf("failed to open torrents file: %w", err)
	}
	defer file.Close()

	var torrents []string
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		// Skip empty lines and comments
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		torrents = append(torrents, line)
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("failed to read torrents file: %w", err)
	}

	return torrents, nil
}
