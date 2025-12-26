package web

import (
	"log"
	"os"
	"path/filepath"
	"sort"
	"time"
)

// CleanupManager handles periodic cleanup of old files in the cache directory
type CleanupManager struct {
	baseDir  string
	maxSize  int64
	interval time.Duration
	done     chan struct{}
}

// NewCleanupManager creates a new cleanup manager
func NewCleanupManager(baseDir string, maxSize int64, interval time.Duration) *CleanupManager {
	return &CleanupManager{
		baseDir:  baseDir,
		maxSize:  maxSize,
		interval: interval,
		done:     make(chan struct{}),
	}
}

// Start begins the periodic cleanup routine
func (cm *CleanupManager) Start() {
	log.Printf("Starting cache cleanup manager. BaseDir: %s, MaxSize: %d bytes, Interval: %v", cm.baseDir, cm.maxSize, cm.interval)
	ticker := time.NewTicker(cm.interval)
	go func() {
		for {
			select {
			case <-ticker.C:
				cm.cleanup()
			case <-cm.done:
				ticker.Stop()
				return
			}
		}
	}()
}

// Stop stops the cleanup routine
func (cm *CleanupManager) Stop() {
	close(cm.done)
}

type fileInfo struct {
	path    string
	size    int64
	modTime time.Time
}

// cleanup deletes oldest files if total size exceeds limit
func (cm *CleanupManager) cleanup() {
	log.Println("Starting scheduled cache cleanup...")
	startTime := time.Now()

	var files []fileInfo
	totalSize := int64(0)

	err := filepath.Walk(cm.baseDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			log.Printf("Error accessing path %s: %v", path, err)
			return nil // Continue walking
		}

		if !info.IsDir() {
			files = append(files, fileInfo{
				path:    path,
				size:    info.Size(),
				modTime: info.ModTime(),
			})
			totalSize += info.Size()
		}
		return nil
	})

	if err != nil {
		log.Printf("Error during cleanup walk: %v", err)
		return
	}

	log.Printf("Total cache size: %d bytes (Limit: %d bytes)", totalSize, cm.maxSize)

	if totalSize <= cm.maxSize {
		log.Println("Cache size within limit. No cleanup needed.")
		return
	}

	// Sort files by modification time (oldest first)
	sort.Slice(files, func(i, j int) bool {
		return files[i].modTime.Before(files[j].modTime)
	})

	deletedCount := 0
	reclaimedSize := int64(0)

	for _, file := range files {
		if totalSize <= cm.maxSize {
			break
		}

		if err := os.Remove(file.path); err != nil {
			log.Printf("Failed to delete old file %s: %v", file.path, err)
		} else {
			log.Printf("Deleted old file: %s (Size: %d bytes, ModTime: %v)", file.path, file.size, file.modTime)
			deletedCount++
			reclaimedSize += file.size
			totalSize -= file.size
		}
	}

	duration := time.Since(startTime)
	log.Printf("Cleanup completed in %v. Deleted %d files, reclaimed %d bytes.", duration, deletedCount, reclaimedSize)
}
