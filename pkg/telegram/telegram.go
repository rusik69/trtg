// Package telegram handles uploading videos to Telegram
package telegram

import (
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

const (
	// MaxFileSize is the maximum file size for Local Bot API Server (2GB)
	MaxFileSize = 2 * 1024 * 1024 * 1024
)

// Uploader handles Telegram video uploads
type Uploader struct {
	bot    *tgbotapi.BotAPI
	chatID int64
}

// NewUploader creates a new Telegram uploader using Local Bot API Server
func NewUploader(token string, chatID int64, apiURL string) (*Uploader, error) {
	apiURL = strings.TrimSuffix(apiURL, "/")
	fmt.Printf("Creating bot with endpoint: %s\n", apiURL+"/bot%s/%s")

	bot, err := tgbotapi.NewBotAPIWithAPIEndpoint(token, apiURL+"/bot%s/%s")
	if err != nil {
		return nil, fmt.Errorf("failed to create Telegram bot: %w", err)
	}
	fmt.Printf("Bot created, setting HTTP client timeout...\n")

	// Configure HTTP client with longer timeout for large file uploads (2GB max)
	// Use 1 hour timeout to handle large files
	bot.Client = &http.Client{
		Timeout: 1 * time.Hour,
	}
	fmt.Printf("HTTP client configured with 1h timeout for large file uploads\n")

	return &Uploader{
		bot:    bot,
		chatID: chatID,
	}, nil
}

// GetMaxFileSize returns the maximum file size for uploads (2GB)
func (u *Uploader) GetMaxFileSize() int64 {
	return MaxFileSize
}

// UploadVideo uploads a video file to Telegram
func (u *Uploader) UploadVideo(filePath, title string) error {
	fileInfo, err := os.Stat(filePath)
	if err != nil {
		return fmt.Errorf("failed to stat file: %w", err)
	}

	if fileInfo.Size() > MaxFileSize {
		return fmt.Errorf("file too large for Telegram upload (max 2GB): %d bytes", fileInfo.Size())
	}

	video := tgbotapi.NewVideo(u.chatID, tgbotapi.FilePath(filePath))
	video.Caption = title
	video.SupportsStreaming = true

	_, err = u.bot.Send(video)
	if err != nil {
		return fmt.Errorf("failed to upload video: %w", err)
	}

	return nil
}

// UploadDocument uploads a file as document to Telegram and returns the file ID
func (u *Uploader) UploadDocument(filePath, title string) (string, error) {
	fileInfo, err := os.Stat(filePath)
	if err != nil {
		return "", fmt.Errorf("failed to stat file: %w", err)
	}

	if fileInfo.Size() > MaxFileSize {
		return "", fmt.Errorf("file too large for Telegram upload (max 2GB): %d bytes", fileInfo.Size())
	}

	doc := tgbotapi.NewDocument(u.chatID, tgbotapi.FilePath(filePath))
	doc.Caption = title

	msg, err := u.bot.Send(doc)
	if err != nil {
		return "", fmt.Errorf("failed to upload document: %w", err)
	}

	// Extract file ID from the message
	if msg.Document != nil {
		return msg.Document.FileID, nil
	}

	return "", fmt.Errorf("no file ID in response")
}

// SendMessage sends a text message to the chat
func (u *Uploader) SendMessage(text string) error {
	msg := tgbotapi.NewMessage(u.chatID, text)
	_, err := u.bot.Send(msg)
	return err
}

// GetMe returns information about the bot
func (u *Uploader) GetMe() (string, error) {
	user, err := u.bot.GetMe()
	if err != nil {
		return "", err
	}
	return user.UserName, nil
}

// NewDownloaderFromUploader creates a downloader from an existing uploader
func NewDownloaderFromUploader(uploader *Uploader) (*Downloader, error) {
	return &Downloader{
		bot:    uploader.bot,
		chatID: uploader.chatID,
	}, nil
}
