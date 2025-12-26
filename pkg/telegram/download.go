// Package telegram handles downloading videos from Telegram
package telegram

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
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
	apiURL string // Store API URL for custom endpoint file downloads
}

// NewDownloader creates a new Telegram downloader using Local Bot API Server
func NewDownloader(token string, chatID int64, apiURL string) (*Downloader, error) {
	if apiURL == "" {
		return nil, fmt.Errorf("apiURL is required - Local Bot API Server must be configured")
	}

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
		apiURL: apiURL,
	}, nil
}

// DownloadFile downloads a file from Telegram by file ID using the bot API directly
// If telegramFilePath is provided, it will be used directly (for large files where GetFile fails)
func (d *Downloader) DownloadFile(fileID string, savePath string) error {
	return d.DownloadFileWithPath(fileID, "", savePath)
}

// DownloadFileWithPath downloads a file from Telegram, using filePath if provided
func (d *Downloader) DownloadFileWithPath(fileID, telegramFilePath, savePath string) error {
	var filePath string
	var fileSize int64

	log.Printf("DownloadFileWithPath called: fileID=%s, telegramFilePath=%s, apiURL=%s", fileID, telegramFilePath, d.apiURL)

	// If file path was provided (stored during upload), use it directly
	if telegramFilePath != "" {
		filePath = telegramFilePath
		log.Printf("Using stored file path: %s", filePath)
	} else {
		// No file path stored - this should not happen if upload worked correctly
		// Try getFile API via Local Bot API Server as last resort, but it will likely fail for large files
		log.Printf("Warning: No file path stored for file ID %s, trying getFile API via Local Bot API Server (will likely fail for large files)", fileID)

		if d.apiURL == "" {
			return fmt.Errorf("apiURL is required - Local Bot API Server must be configured")
		}

		apiURL := strings.TrimSuffix(d.apiURL, "/")
		getFileURL := fmt.Sprintf("%s/bot%s/getFile?file_id=%s", apiURL, d.bot.Token, fileID)
		log.Printf("Calling getFile API: %s", getFileURL)

		httpClient := &http.Client{Timeout: 30 * time.Second}
		resp, httpErr := httpClient.Get(getFileURL)
		if httpErr == nil {
			defer resp.Body.Close()
			log.Printf("getFile API returned status: %d", resp.StatusCode)
			if resp.StatusCode == http.StatusOK {
				var result struct {
					OK     bool `json:"ok"`
					Result struct {
						FilePath string `json:"file_path"`
						FileSize int64  `json:"file_size"`
					} `json:"result"`
				}
				bodyBytes, _ := io.ReadAll(resp.Body)
				log.Printf("getFile API response body: %s", string(bodyBytes))

				if jsonErr := json.Unmarshal(bodyBytes, &result); jsonErr == nil && result.OK && result.Result.FilePath != "" {
					filePath = result.Result.FilePath
					fileSize = result.Result.FileSize
					log.Printf("Got file path from getFile API via Local Bot API Server: %s (size: %d)", filePath, fileSize)
				} else if jsonErr != nil {
					log.Printf("Failed to decode getFile response: %v", jsonErr)
				}
			}
		} else {
			log.Printf("getFile API request failed: %v", httpErr)
		}

		if filePath == "" {
			return fmt.Errorf("file path not available for file ID %s (file was likely uploaded without storing the path)", fileID)
		}
	}

	// Create directory if needed
	if err := os.MkdirAll(filepath.Dir(savePath), 0755); err != nil {
		return fmt.Errorf("failed to create directory: %w", err)
	}

	// Construct download URL using the file path we obtained
	apiURL := strings.TrimSuffix(d.apiURL, "/")
	downloadURL := fmt.Sprintf("%s/file/bot%s/%s", apiURL, d.bot.Token, filePath)
	log.Printf("Downloading from Local Bot API Server: %s", downloadURL)

	// Download the file
	resp, err := http.Get(downloadURL)
	if err != nil {
		return fmt.Errorf("failed to download file: %w", err)
	}
	defer resp.Body.Close()

	// If we got 404, the file was cleaned from cache - call getFile to re-fetch from Telegram cloud
	if resp.StatusCode == http.StatusNotFound {
		log.Printf("File not in local cache (404), calling getFile to re-fetch from Telegram cloud...")
		resp.Body.Close()

		// Call getFile API to make the local server fetch the file from Telegram cloud
		getFileURL := fmt.Sprintf("%s/bot%s/getFile?file_id=%s", apiURL, d.bot.Token, fileID)
		log.Printf("Calling getFile API to trigger cloud fetch: %s", getFileURL)

		httpClient := &http.Client{Timeout: 60 * time.Second}
		getResp, getErr := httpClient.Get(getFileURL)
		if getErr != nil {
			return fmt.Errorf("failed to call getFile API after 404: %w", getErr)
		}
		defer getResp.Body.Close()

		if getResp.StatusCode != http.StatusOK {
			bodyBytes, _ := io.ReadAll(getResp.Body)
			return fmt.Errorf("getFile API failed with status %d: %s", getResp.StatusCode, string(bodyBytes))
		}

		var result struct {
			OK     bool `json:"ok"`
			Result struct {
				FilePath string `json:"file_path"`
				FileSize int64  `json:"file_size"`
			} `json:"result"`
		}
		bodyBytes, _ := io.ReadAll(getResp.Body)
		if jsonErr := json.Unmarshal(bodyBytes, &result); jsonErr != nil || !result.OK || result.Result.FilePath == "" {
			return fmt.Errorf("getFile API returned invalid response: %s", string(bodyBytes))
		}

		// getFile fetched the file from Telegram cloud and saved it to disk
		// The response contains the absolute path where it was saved
		diskPath := result.Result.FilePath
		log.Printf("File re-fetched from Telegram cloud to disk: %s (size: %d bytes)", diskPath, result.Result.FileSize)

		// Copy directly from disk instead of trying HTTP download (HTTP endpoint returns 501)
		log.Printf("Copying file from disk: %s -> %s", diskPath, savePath)

		sourceFile, err := os.Open(diskPath)
		if err != nil {
			return fmt.Errorf("failed to open re-fetched file from disk: %w", err)
		}
		defer sourceFile.Close()

		// Create output file
		out, err := os.Create(savePath)
		if err != nil {
			sourceFile.Close()
			return fmt.Errorf("failed to create output file: %w", err)
		}
		defer out.Close()

		// Copy from disk to destination
		written, err := io.Copy(out, sourceFile)
		if err != nil {
			return fmt.Errorf("failed to copy re-fetched file: %w", err)
		}

		log.Printf("Successfully copied %d bytes from re-fetched file to %s", written, savePath)
		return nil
	}

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download failed with status: %d", resp.StatusCode)
	}

	// Create output file
	out, err := os.Create(savePath)
	if err != nil {
		return fmt.Errorf("failed to create output file: %w", err)
	}
	defer out.Close()

	// Copy with progress
	written, err := io.Copy(out, resp.Body)
	if err != nil {
		return fmt.Errorf("failed to save file: %w", err)
	}

	log.Printf("Successfully downloaded %d bytes to %s", written, savePath)
	return nil
}

// GetDownloadURL returns the download URL for a file, resolving the path if necessary
func (d *Downloader) GetDownloadURL(fileID, telegramFilePath string) (string, error) {
	var filePath string

	if telegramFilePath != "" {
		filePath = telegramFilePath
	} else {
		// Try getFile API via Local Bot API Server
		if d.apiURL == "" {
			return "", fmt.Errorf("apiURL is required")
		}

		apiURL := strings.TrimSuffix(d.apiURL, "/")
		getFileURL := fmt.Sprintf("%s/bot%s/getFile?file_id=%s", apiURL, d.bot.Token, fileID)

		httpClient := &http.Client{Timeout: 30 * time.Second}
		resp, err := httpClient.Get(getFileURL)
		if err != nil {
			return "", fmt.Errorf("failed to call getFile API: %w", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode == http.StatusOK {
			var result struct {
				OK     bool `json:"ok"`
				Result struct {
					FilePath string `json:"file_path"`
				} `json:"result"`
			}
			if json.NewDecoder(resp.Body).Decode(&result) == nil && result.OK && result.Result.FilePath != "" {
				filePath = result.Result.FilePath
			}
		}

		if filePath == "" {
			return "", fmt.Errorf("file path not available for file ID %s", fileID)
		}
	}

	apiURL := strings.TrimSuffix(d.apiURL, "/")
	return fmt.Sprintf("%s/file/bot%s/%s", apiURL, d.bot.Token, filePath), nil
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
