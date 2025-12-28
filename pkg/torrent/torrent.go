// Package torrent handles torrent downloading
package torrent

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/anacrolix/torrent"
	"github.com/anacrolix/torrent/storage"
)

// Downloader handles torrent downloads
type Downloader struct {
	client      *torrent.Client
	downloadDir string
}

// NewDownloader creates a new torrent downloader
func NewDownloader(downloadDir string) (*Downloader, error) {
	if err := os.MkdirAll(downloadDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create download directory: %w", err)
	}

	cfg := torrent.NewDefaultClientConfig()
	cfg.DataDir = downloadDir
	cfg.DefaultStorage = storage.NewMMap(downloadDir)
	// Disable uploading/seeding during download
	cfg.NoUpload = true
	cfg.DisableAggressiveUpload = true
	cfg.Seed = false

	client, err := torrent.NewClient(cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to create torrent client: %w", err)
	}

	return &Downloader{
		client:      client,
		downloadDir: downloadDir,
	}, nil
}

// StopTorrent stops and removes a torrent from the client to prevent seeding
func (d *Downloader) StopTorrent(torrentURL string) error {
	var t *torrent.Torrent
	var err error

	// Find the torrent in the client
	if strings.HasPrefix(torrentURL, "magnet:") {
		// For magnet links, we need to find by info hash
		// Extract info hash from magnet link
		parts := strings.Split(torrentURL, "btih:")
		if len(parts) < 2 {
			return fmt.Errorf("invalid magnet link")
		}
		infoHash := strings.Split(parts[1], "&")[0]

		// Find torrent by info hash
		for _, torrent := range d.client.Torrents() {
			if torrent.InfoHash().String() == infoHash {
				t = torrent
				break
			}
		}
	} else {
		// For .torrent files, try to find by name or add and then drop
		t, err = d.client.AddTorrentFromFile(torrentURL)
		if err != nil {
			return fmt.Errorf("failed to add torrent file: %w", err)
		}
	}

	if t == nil {
		// Torrent not found, might already be removed
		return nil
	}

	// Stop downloading and seeding
	t.Drop()
	return nil
}

// GetOrAddTorrent gets an existing torrent or adds a new one
func (d *Downloader) GetOrAddTorrent(torrentURL string) (*torrent.Torrent, error) {
	// Check if torrent already exists (for magnet links, check by info hash)
	if strings.HasPrefix(torrentURL, "magnet:") {
		parts := strings.Split(torrentURL, "btih:")
		if len(parts) >= 2 {
			infoHash := strings.Split(parts[1], "&")[0]
			for _, t := range d.client.Torrents() {
				if t.InfoHash().String() == infoHash {
					return t, nil
				}
			}
		}
		t, err := d.client.AddMagnet(torrentURL)
		if err != nil {
			return nil, fmt.Errorf("failed to add magnet link: %w", err)
		}
		return t, nil
	}

	// For .torrent files, try to find existing or add new
	// Note: anacrolix/torrent will return existing torrent if already added
	t, err := d.client.AddTorrentFromFile(torrentURL)
	if err != nil {
		return nil, fmt.Errorf("failed to add torrent file: %w", err)
	}
	return t, nil
}

// DownloadSingleFile downloads a single file from an already-added torrent
// Returns the path to the downloaded file
func (d *Downloader) DownloadSingleFile(t *torrent.Torrent, filePath, torrentName string, fileIndex, totalFiles int) (string, error) {
	// Find the file in the torrent
	var targetFile *torrent.File
	for _, file := range t.Files() {
		if file.Path() == filePath {
			targetFile = file
			break
		}
	}

	if targetFile == nil {
		return "", fmt.Errorf("file not found in torrent: %s", filePath)
	}

	fileSizeGB := float64(targetFile.Length()) / (1024 * 1024 * 1024)
	fmt.Printf("Downloading file %d/%d: %s (%.2f GB)\n", fileIndex+1, totalFiles, filePath, fileSizeGB)

	// Download this file
	targetFile.Download()

	// Wait for this file to complete
	ctx, cancel := context.WithTimeout(context.Background(), 24*time.Hour)
	ticker := time.NewTicker(30 * time.Second)
	var lastProgress int64
	var lastPercent float64

	fileDownloaded := false
	for {
		select {
		case <-ctx.Done():
			cancel()
			ticker.Stop()
			return "", fmt.Errorf("download timeout for file %s", filePath)
		case <-ticker.C:
			progress := targetFile.BytesCompleted()
			fileSize := targetFile.Length()

			if fileSize > 0 && progress >= fileSize {
				fileDownloaded = true
				cancel()
				ticker.Stop()
				break
			}

			if fileSize > 0 {
				percent := float64(progress) / float64(fileSize) * 100
				if percent-lastPercent >= 5.0 || (progress != lastProgress && progress-lastProgress > 10*1024*1024) {
					fmt.Printf("  Progress: %.1f%% (%d/%d bytes)\n", percent, progress, fileSize)
					lastPercent = percent
					lastProgress = progress
				}
			}
		}
		if fileDownloaded {
			break
		}
	}

	// Find the downloaded file path
	var downloadedPath string
	var found bool

	// Try path with torrent name prefix
	downloadedPath = filepath.Join(d.downloadDir, torrentName, filePath)
	if _, err := os.Stat(downloadedPath); err == nil {
		found = true
	} else {
		// Try path without torrent name
		downloadedPath = filepath.Join(d.downloadDir, filePath)
		if _, err := os.Stat(downloadedPath); err == nil {
			found = true
		} else if totalFiles == 1 {
			// Single file torrent
			downloadedPath = filepath.Join(d.downloadDir, torrentName)
			if _, err := os.Stat(downloadedPath); err == nil {
				found = true
			}
		}
	}

	if !found {
		return "", fmt.Errorf("file not found after download: %s", filePath)
	}

	return downloadedPath, nil
}

// DownloadTorrent downloads a torrent file or magnet link
// Returns the paths to the downloaded files and the torrent name
// Note: Caller should call StopTorrent after upload to prevent seeding
func (d *Downloader) DownloadTorrent(torrentURL string) ([]string, string, error) {
	var t *torrent.Torrent
	var err error

	// Check if it's a magnet link or a file path
	if strings.HasPrefix(torrentURL, "magnet:") {
		t, err = d.client.AddMagnet(torrentURL)
		if err != nil {
			return nil, "", fmt.Errorf("failed to add magnet link: %w", err)
		}
	} else {
		// Assume it's a .torrent file path
		t, err = d.client.AddTorrentFromFile(torrentURL)
		if err != nil {
			return nil, "", fmt.Errorf("failed to add torrent file: %w", err)
		}
	}

	// Wait for metadata
	<-t.GotInfo()

	torrentName := t.Name()
	if torrentName == "" {
		torrentName = "torrent"
	}

	fmt.Printf("Downloading torrent: %s\n", torrentName)
	fmt.Printf("Files in torrent: %d\n", len(t.Files()))

	// Download only video files <= 2GB
	maxFileSize := int64(2 * 1024 * 1024 * 1024)
	files := t.Files()
	var filesToDownload []*torrent.File
	var skippedCount int

	// Video file extensions
	videoExts := map[string]bool{
		".mp4": true, ".avi": true, ".mkv": true, ".mov": true, ".wmv": true,
		".flv": true, ".webm": true, ".m4v": true, ".mpg": true, ".mpeg": true,
		".3gp": true, ".ogv": true, ".ts": true, ".m2ts": true, ".mts": true,
	}

	for _, file := range files {
		filePath := file.Path()
		ext := strings.ToLower(filepath.Ext(filePath))

		// Skip non-video files
		if !videoExts[ext] {
			fmt.Printf("Skipping non-video file: %s\n", filePath)
			skippedCount++
			continue
		}

		if file.Length() > maxFileSize {
			fmt.Printf("Skipping file %s (%.2f GB > 2GB)\n", filePath, float64(file.Length())/(1024*1024*1024))
			skippedCount++
		} else {
			filesToDownload = append(filesToDownload, file)
		}
	}

	if len(filesToDownload) == 0 {
		return nil, "", fmt.Errorf("no video files to download (all files exceed 2GB limit or are not video files)")
	}

	fmt.Printf("Downloading %d files one by one (skipping %d files > 2GB)\n", len(filesToDownload), skippedCount)

	// Download files one by one sequentially
	var filePaths []string
	for i, file := range filesToDownload {
		fileSizeGB := float64(file.Length()) / (1024 * 1024 * 1024)
		fmt.Printf("Downloading file %d/%d: %s (%.2f GB)\n", i+1, len(filesToDownload), file.Path(), fileSizeGB)

		// Download this file
		file.Download()

		// Wait for this file to complete
		ctx, cancel := context.WithTimeout(context.Background(), 24*time.Hour)
		ticker := time.NewTicker(30 * time.Second) // Check every 30 seconds
		var lastProgress int64
		var lastPercent float64

		fileDownloaded := false
		for {
			select {
			case <-ctx.Done():
				cancel()
				ticker.Stop()
				return nil, "", fmt.Errorf("download timeout for file %s", file.Path())
			case <-ticker.C:
				progress := file.BytesCompleted()
				fileSize := file.Length()

				if fileSize > 0 && progress >= fileSize {
					// File download complete
					fileDownloaded = true
					cancel()
					ticker.Stop()
					break
				}

				// Only log if progress changed by at least 5% or significant bytes
				if fileSize > 0 {
					percent := float64(progress) / float64(fileSize) * 100
					if percent-lastPercent >= 5.0 || (progress != lastProgress && progress-lastProgress > 10*1024*1024) {
						fmt.Printf("  Progress: %.1f%% (%d/%d bytes)\n", percent, progress, fileSize)
						lastPercent = percent
						lastProgress = progress
					}
				}
			}
			if fileDownloaded {
				break
			}
		}

		// Verify file exists and add to list
		// anacrolix/torrent stores files in DataDir, and for multi-file torrents,
		// files are stored at DataDir/torrentName/file.Path()
		// But file.Path() might already include the torrent name, so we need to check both
		var filePath string
		var found bool

		// Try path with torrent name prefix (standard multi-file torrent structure)
		filePath = filepath.Join(d.downloadDir, torrentName, file.Path())
		if _, err := os.Stat(filePath); err == nil {
			found = true
		} else {
			// Try path without torrent name (if file.Path() already includes it)
			filePath = filepath.Join(d.downloadDir, file.Path())
			if _, err := os.Stat(filePath); err == nil {
				found = true
			} else if len(filesToDownload) == 1 {
				// Single file torrent - try just the torrent name
				filePath = filepath.Join(d.downloadDir, torrentName)
				if _, err := os.Stat(filePath); err == nil {
					found = true
				}
			}
		}

		if found {
			filePaths = append(filePaths, filePath)
			fmt.Printf("  Completed: %s\n", file.Path())
		} else {
			fmt.Printf("  Warning: File not found after download: %s (tried: %s)\n", file.Path(), filePath)
		}
	}

	if len(filePaths) == 0 {
		return nil, "", fmt.Errorf("no files found after download completion")
	}

	// Stop the torrent to prevent seeding (will be removed after upload)
	t.Drop()

	return filePaths, torrentName, nil
}

// FileInfo represents information about a file in a torrent
type FileInfo struct {
	Path string
	Size int64
}

// GetTorrentInfo gets information about a torrent without downloading
// Returns torrent name, total size, and list of files with their sizes
// Note: The torrent is removed after getting info to prevent seeding
func (d *Downloader) GetTorrentInfo(torrentURL string) (string, int64, []FileInfo, error) {
	var t *torrent.Torrent
	var err error

	if strings.HasPrefix(torrentURL, "magnet:") {
		t, err = d.client.AddMagnet(torrentURL)
		if err != nil {
			return "", 0, nil, fmt.Errorf("failed to add magnet link: %w", err)
		}
	} else {
		t, err = d.client.AddTorrentFromFile(torrentURL)
		if err != nil {
			return "", 0, nil, fmt.Errorf("failed to add torrent file: %w", err)
		}
	}

	// Wait for metadata
	<-t.GotInfo()

	name := t.Name()
	totalSize := t.Info().TotalLength()

	// Get file information
	var files []FileInfo
	for _, file := range t.Files() {
		files = append(files, FileInfo{
			Path: file.Path(),
			Size: file.Length(),
		})
	}

	// Remove torrent immediately after getting info to prevent seeding
	t.Drop()

	return name, totalSize, files, nil
}

// Close closes the torrent client
func (d *Downloader) Close() {
	d.client.Close()
}

// CleanupFile removes a downloaded file
func (d *Downloader) CleanupFile(filePath string) error {
	return os.RemoveAll(filePath)
}
