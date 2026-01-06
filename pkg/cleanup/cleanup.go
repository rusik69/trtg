// Package cleanup manages telegram-bot-api local storage to prevent disk space issues
package cleanup

import (
	"fmt"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"sort"
	"time"
)

const (
	// MaxStorageGB is the maximum storage size in GB before cleanup triggers
	MaxStorageGB = 2
	// MaxFiles is the maximum number of files to keep
	MaxFiles = 5
	// CleanupIntervalMinutes is how often to run cleanup
	CleanupIntervalMinutes = 2
)

// FileInfo holds information about a file for cleanup
type FileInfo struct {
	Path    string
	ModTime time.Time
	Size    int64
}

// Service manages cleanup of telegram-bot-api storage
type Service struct {
	storagePath string
	maxBytes    int64
	maxFiles    int
	interval    time.Duration
}

// NewService creates a new cleanup service
func NewService(storagePath string) *Service {
	return &Service{
		storagePath: storagePath,
		maxBytes:    int64(MaxStorageGB * 1024 * 1024 * 1024), // Convert GB to bytes
		maxFiles:    MaxFiles,
		interval:    time.Duration(CleanupIntervalMinutes) * time.Minute,
	}
}

// Start begins the cleanup service in a goroutine
func (s *Service) Start() {
	log.Printf("Starting cleanup service for %s (max: %d GB, %d files, interval: %v)",
		s.storagePath, MaxStorageGB, MaxFiles, s.interval)

	go s.run()
}

// run is the main cleanup loop
func (s *Service) run() {
	// Run cleanup immediately on start
	s.cleanup()

	// Then run periodically
	ticker := time.NewTicker(s.interval)
	defer ticker.Stop()

	for range ticker.C {
		s.cleanup()
	}
}

// cleanup performs the actual cleanup operation
func (s *Service) cleanup() {
	// Check if storage path exists
	if _, err := os.Stat(s.storagePath); os.IsNotExist(err) {
		log.Printf("Cleanup: Storage path does not exist yet: %s", s.storagePath)
		return
	}

	// Find all files
	files, err := s.scanFiles()
	if err != nil {
		log.Printf("Cleanup: Error scanning files: %v", err)
		return
	}

	if len(files) == 0 {
		log.Printf("Cleanup: No files found in storage")
		return
	}

	// Calculate total size
	var totalSize int64
	for _, f := range files {
		totalSize += f.Size
	}

	totalSizeGB := float64(totalSize) / (1024 * 1024 * 1024)
	log.Printf("Cleanup: Found %d files, total size: %.2f GB", len(files), totalSizeGB)

	// Check if cleanup is needed
	needsCleanup := totalSize > s.maxBytes || len(files) > s.maxFiles

	if !needsCleanup {
		log.Printf("Cleanup: Storage within limits (%.2f/%.0f GB, %d/%d files), no action needed",
			totalSizeGB, float64(MaxStorageGB), len(files), MaxFiles)
		return
	}

	// Sort files by modification time (oldest first)
	sort.Slice(files, func(i, j int) bool {
		return files[i].ModTime.Before(files[j].ModTime)
	})

	// Delete oldest files until under limits
	deletedCount := 0
	var deletedSize int64

	for _, file := range files {
		// Check if we're now under limits
		if totalSize <= s.maxBytes && len(files)-deletedCount <= s.maxFiles {
			break
		}

		// Delete the file
		if err := os.Remove(file.Path); err != nil {
			log.Printf("Cleanup: Error deleting file %s: %v", file.Path, err)
			continue
		}

		deletedCount++
		deletedSize += file.Size
		totalSize -= file.Size

		log.Printf("Cleanup: Deleted %s (%.2f MB, modified: %s)",
			filepath.Base(file.Path),
			float64(file.Size)/(1024*1024),
			file.ModTime.Format("2006-01-02 15:04:05"))
	}

	if deletedCount > 0 {
		remainingGB := float64(totalSize) / (1024 * 1024 * 1024)
		deletedGB := float64(deletedSize) / (1024 * 1024 * 1024)
		log.Printf("Cleanup: Deleted %d files (%.2f GB freed), remaining: %d files (%.2f GB)",
			deletedCount, deletedGB, len(files)-deletedCount, remainingGB)
	}
}

// scanFiles recursively scans for all files in the storage path
func (s *Service) scanFiles() ([]FileInfo, error) {
	var files []FileInfo

	err := filepath.WalkDir(s.storagePath, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		// Skip directories
		if d.IsDir() {
			return nil
		}

		// Scan all files (including temp files, partial uploads, etc.)
		// Note: This includes video files (.mp4, .mkv, .avi, .mov) and
		// temporary upload files in the temp/ directory

		info, err := d.Info()
		if err != nil {
			log.Printf("Cleanup: Error getting file info for %s: %v", path, err)
			return nil // Skip this file but continue
		}

		files = append(files, FileInfo{
			Path:    path,
			ModTime: info.ModTime(),
			Size:    info.Size(),
		})

		return nil
	})

	if err != nil {
		return nil, fmt.Errorf("failed to walk directory: %w", err)
	}

	return files, nil
}
