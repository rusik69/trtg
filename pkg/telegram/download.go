// Package telegram handles downloading videos from Telegram
package telegram

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

// Downloader handles downloading files from Telegram
type Downloader struct {
	bot    *tgbotapi.BotAPI
	chatID int64
}

// NewDownloader creates a new Telegram downloader
func NewDownloader(token string, chatID int64, apiURL string) (*Downloader, error) {
	apiURL = strings.TrimSuffix(apiURL, "/")
	bot, err := tgbotapi.NewBotAPIWithAPIEndpoint(token, apiURL+"/bot%s/%s")
	if err != nil {
		return nil, fmt.Errorf("failed to create Telegram bot: %w", err)
	}

	bot.Client = &http.Client{
		Timeout: 1 * time.Hour,
	}

	return &Downloader{
		bot:    bot,
		chatID: chatID,
	}, nil
}

// DownloadFile downloads a file from Telegram by file ID
func (d *Downloader) DownloadFile(fileID string, savePath string) error {
	file, err := d.bot.GetFile(tgbotapi.FileConfig{FileID: fileID})
	if err != nil {
		return fmt.Errorf("failed to get file info: %w", err)
	}

	// Create directory if needed
	if err := os.MkdirAll(filepath.Dir(savePath), 0755); err != nil {
		return fmt.Errorf("failed to create directory: %w", err)
	}

	// Download file
	fileURL := file.Link(d.bot.Token)
	resp, err := http.Get(fileURL)
	if err != nil {
		return fmt.Errorf("failed to download file: %w", err)
	}
	defer resp.Body.Close()

	// Create output file
	out, err := os.Create(savePath)
	if err != nil {
		return fmt.Errorf("failed to create file: %w", err)
	}
	defer out.Close()

	// Copy content
	_, err = io.Copy(out, resp.Body)
	if err != nil {
		return fmt.Errorf("failed to save file: %w", err)
	}

	return nil
}

// GetMessages gets messages from the chat
func (d *Downloader) GetMessages(limit int) ([]tgbotapi.Message, error) {
	config := tgbotapi.NewUpdate(0)
	config.Timeout = 60
	updates := d.bot.GetUpdatesChan(config)

	var messages []tgbotapi.Message
	seen := make(map[int]bool)

	for update := range updates {
		if update.Message != nil && update.Message.Chat.ID == d.chatID {
			if !seen[update.Message.MessageID] {
				messages = append(messages, *update.Message)
				seen[update.Message.MessageID] = true
				if len(messages) >= limit {
					break
				}
			}
		}
	}

	return messages, nil
}
