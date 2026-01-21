// Package telegram handles uploading videos to Telegram
package telegram

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
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
	apiURL string // Store API URL for file downloads
}

// NewUploader creates a new Telegram uploader using Local Bot API Server
func NewUploader(token string, chatID int64, apiURL string) (*Uploader, error) {
	if apiURL == "" {
		return nil, fmt.Errorf("apiURL is required - Local Bot API Server must be configured")
	}

	apiURL = strings.TrimSuffix(apiURL, "/")
	bot, err := tgbotapi.NewBotAPIWithAPIEndpoint(token, apiURL+"/bot%s/%s")
	if err != nil {
		return nil, fmt.Errorf("failed to create Telegram bot: %w", err)
	}

	bot.Client = &http.Client{Timeout: 1 * time.Hour}

	return &Uploader{
		bot:    bot,
		chatID: chatID,
		apiURL: apiURL,
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

// UploadResult contains both file ID and file path from upload
type UploadResult struct {
	FileID    string
	FilePath  string // File path on Telegram server (for Local Bot API)
	MessageID int    // Telegram message ID for deleting messages
}

// UploadDocument uploads a file as document to Telegram and returns the file ID and path
func (u *Uploader) UploadDocument(filePath, title string) (string, error) {
	result, err := u.UploadDocumentWithPath(filePath, title)
	if err != nil {
		return "", err
	}
	return result.FileID, nil
}

// UploadDocumentWithPath uploads a file and returns both file ID and file path
func (u *Uploader) UploadDocumentWithPath(filePath, title string) (*UploadResult, error) {
	fileInfo, err := os.Stat(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to stat file: %w", err)
	}

	if fileInfo.Size() > MaxFileSize {
		return nil, fmt.Errorf("file too large for Telegram upload (max 2GB): %d bytes", fileInfo.Size())
	}

	doc := tgbotapi.NewDocument(u.chatID, tgbotapi.FilePath(filePath))
	doc.Caption = title

	msg, err := u.bot.Send(doc)
	if err != nil {
		return nil, fmt.Errorf("failed to upload document: %w", err)
	}

	// Log the complete response for debugging
	log.Printf("Telegram upload response: MessageID=%d, ChatID=%d", msg.MessageID, msg.Chat.ID)
	if msg.Document != nil {
		log.Printf("  Document: FileID=%s, FileUniqueID=%s, FileName=%s, FileSize=%d, MimeType=%s",
			msg.Document.FileID,
			msg.Document.FileUniqueID,
			msg.Document.FileName,
			msg.Document.FileSize,
			msg.Document.MimeType)
	}

	// Extract file ID from the message
	if msg.Document != nil {
		result := &UploadResult{
			FileID:    msg.Document.FileID,
			MessageID: msg.MessageID,
		}

		// Try to get the file path for the uploaded document
		// This is needed for downloading the file later via Local Bot API
		filePath, err := u.getFilePath(msg.Document.FileID, msg.Document.FileUniqueID)
		if err != nil {
			// Log warning but don't fail - we still have FileID which can be used for cloud downloads
			log.Printf("Warning: Could not get file path for uploaded document (FileID: %s): %v", result.FileID, err)
			log.Printf("File will still be accessible via Telegram cloud, but local path download may fail")
		} else {
			result.FilePath = filePath
			log.Printf("File uploaded successfully with FileID: %s, FilePath: %s, MessageID: %d", result.FileID, result.FilePath, result.MessageID)
		}

		return result, nil
	}

	return nil, fmt.Errorf("no file ID in response")
}

// SendMessage sends a text message to the chat
func (u *Uploader) SendMessage(text string) error {
	msg := tgbotapi.NewMessage(u.chatID, text)
	_, err := u.bot.Send(msg)
	return err
}

// DeleteMessage deletes a message from Telegram by message ID
func (u *Uploader) DeleteMessage(messageID int) error {
	deleteMsg := tgbotapi.NewDeleteMessage(u.chatID, messageID)
	_, err := u.bot.Send(deleteMsg)
	if err != nil {
		return fmt.Errorf("failed to delete message %d: %w", messageID, err)
	}
	return nil
}

// GetMe returns information about the bot
func (u *Uploader) GetMe() (string, error) {
	user, err := u.bot.GetMe()
	if err != nil {
		return "", err
	}
	return user.UserName, nil
}

// getFilePath gets the file path from Local Bot API Server
func (u *Uploader) getFilePath(fileID, fileUniqueID string) (string, error) {
	log.Printf("Getting file path for FileID: %s, FileUniqueID: %s", fileID, fileUniqueID)

	// Try GetFile API
	file, err := u.bot.GetFile(tgbotapi.FileConfig{FileID: fileID})
	if err == nil && file.FilePath != "" {
		normalized := u.normalizePath(file.FilePath)
		log.Printf("Got file path from GetFile API: %s (normalized: %s)", file.FilePath, normalized)
		return normalized, nil
	}
	log.Printf("GetFile API failed or returned empty path: %v", err)

	// Try direct HTTP API call
	apiURL := strings.TrimSuffix(u.apiURL, "/")
	getFileURL := fmt.Sprintf("%s/bot%s/getFile?file_id=%s", apiURL, u.bot.Token, fileID)
	resp, err := http.Get(getFileURL)
	if err == nil {
		defer resp.Body.Close()
		log.Printf("Direct HTTP getFile returned status: %d", resp.StatusCode)
		if resp.StatusCode == http.StatusOK {
			var result struct {
				OK     bool `json:"ok"`
				Result struct {
					FilePath string `json:"file_path"`
				} `json:"result"`
			}
			if json.NewDecoder(resp.Body).Decode(&result) == nil && result.OK && result.Result.FilePath != "" {
				normalized := u.normalizePath(result.Result.FilePath)
				log.Printf("Got file path from HTTP API: %s (normalized: %s)", result.Result.FilePath, normalized)
				return normalized, nil
			}
		} else if resp.StatusCode == 400 {
			// File is too big or getFile not working - try path patterns
			log.Printf("getFile returned 400, trying path patterns")
			return u.findFilePathByPatterns(fileID, fileUniqueID)
		}
	} else {
		log.Printf("HTTP getFile request failed: %v", err)
	}

	// Last resort: try path patterns anyway
	log.Printf("All getFile methods failed, trying path patterns as last resort")
	return u.findFilePathByPatterns(fileID, fileUniqueID)
}

// normalizePath converts absolute paths to relative paths for HTTP access
func (u *Uploader) normalizePath(path string) string {
	// Handle absolute filesystem paths first
	storageRoots := []string{"/var/lib/telegram-bot-api", "/app/telegram-bot-api-data"}
	for _, root := range storageRoots {
		if strings.HasPrefix(path, root+"/") {
			path = strings.TrimPrefix(path, root+"/")
			log.Printf("Stripped storage root, path is now: %s", path)
			break
		}
	}

	// If path starts with bot token (e.g., "1234567890:ABC.../documents/file.mp4"),
	// strip the token prefix since the download URL will add "bot{token}/" prefix
	if strings.Contains(path, ":") && strings.Contains(path, "/") {
		// Format: "TOKEN/documents/file.mp4" -> "documents/file.mp4"
		parts := strings.SplitN(path, "/", 2)
		if len(parts) == 2 && strings.Contains(parts[0], ":") {
			log.Printf("Stripping token prefix from path: %s -> %s", path, parts[1])
			return parts[1]
		}
	}

	return path
}

// findFilePathByPatterns tries common path patterns when getFile fails
func (u *Uploader) findFilePathByPatterns(fileID, fileUniqueID string) (string, error) {
	apiURL := strings.TrimSuffix(u.apiURL, "/")
	fileIDParts := strings.Split(fileID, "_")
	lastSegment := ""
	if len(fileIDParts) > 0 {
		lastSegment = fileIDParts[len(fileIDParts)-1]
	}

	// For Local Bot API Server, try these patterns in order
	possiblePaths := []string{
		// Direct file ID (most common for local API)
		fileID,
		// Documents folder patterns
		fmt.Sprintf("documents/%s", fileID),
		fmt.Sprintf("documents/document_%s", fileID),
		// Files folder patterns
		fmt.Sprintf("files/%s", fileID),
		// Last segment patterns
		fmt.Sprintf("documents/%s", lastSegment),
		fmt.Sprintf("files/%s", lastSegment),
		// Videos folder patterns (for video files)
		fmt.Sprintf("videos/%s", fileID),
		fmt.Sprintf("videos/video_%s", fileID),
	}
	if fileUniqueID != "" {
		possiblePaths = append(possiblePaths,
			fileUniqueID,
			fmt.Sprintf("documents/%s", fileUniqueID),
			fmt.Sprintf("files/%s", fileUniqueID),
			fmt.Sprintf("videos/%s", fileUniqueID),
		)
	}

	client := &http.Client{Timeout: 10 * time.Second}
	log.Printf("Trying %d possible path patterns for FileID: %s", len(possiblePaths), fileID)

	for i, path := range possiblePaths {
		if path == "" {
			continue
		}
		url := fmt.Sprintf("%s/file/bot%s/%s", apiURL, u.bot.Token, path)
		log.Printf("  [%d/%d] Trying pattern: %s", i+1, len(possiblePaths), path)
		req, _ := http.NewRequest("HEAD", url, nil)
		resp, err := client.Do(req)
		if err == nil && resp != nil {
			resp.Body.Close()
			log.Printf("    -> HTTP %d", resp.StatusCode)
			if resp.StatusCode == http.StatusOK {
				log.Printf("Found working file path pattern: %s", path)
				return path, nil
			}
		} else {
			log.Printf("    -> Request failed: %v", err)
		}
	}

	log.Printf("None of the %d patterns worked for FileID: %s", len(possiblePaths), fileID)
	return "", fmt.Errorf("could not find file path for %s", fileID)
}

// verifyFilePath verifies that a file path is accessible via local filesystem
func (u *Uploader) verifyFilePath(fileID, filePath string) error {
	// With --local mode, verify file exists in local storage
	localStoragePath := filepath.Join("/var/lib/telegram-bot-api", u.bot.Token, filePath)
	log.Printf("Verifying file exists at: %s", localStoragePath)

	fileInfo, err := os.Stat(localStoragePath)
	if err != nil {
		return fmt.Errorf("file not accessible: %w", err)
	}

	if fileInfo.Size() == 0 {
		return fmt.Errorf("file is empty")
	}

	log.Printf("File verified: %d bytes at %s", fileInfo.Size(), localStoragePath)
	return nil
}

// NewDownloaderFromUploader creates a downloader from an existing uploader
func NewDownloaderFromUploader(uploader *Uploader) (*Downloader, error) {
	return &Downloader{
		bot:    uploader.bot,
		chatID: uploader.chatID,
		apiURL: uploader.apiURL,
	}, nil
}
