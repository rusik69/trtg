// Package database handles PostgreSQL operations for tracking downloaded files
package database

import (
	"database/sql"
	"fmt"
	"strings"
	"time"

	_ "github.com/lib/pq"
)

// Video represents a downloaded file record
// Note: Field names kept for backward compatibility with existing database schema
type Video struct {
	ID               int64
	VideoID          string // Used as torrent URL/ID
	ChannelURL       string // Used as torrent URL
	Title            string
	FilePath         string
	DownloadedAt     time.Time
	UploadedAt       *time.Time
	TelegramFileID   string // Telegram file ID for downloading
	TelegramFilePath string // Telegram file path for downloading (for large files)
	ShowName         string // Parsed show name
	SeasonNumber     int    // Season number (0 for specials/unknown)
	EpisodeNumber    int    // Episode number (0 if unknown)
}

// DB wraps the PostgreSQL database connection
type DB struct {
	conn *sql.DB
}

// New creates a new database connection and initializes the schema
// dbURL should be a PostgreSQL connection string, e.g.:
// postgres://user:password@host:port/dbname?sslmode=disable
func New(dbURL string) (*DB, error) {
	conn, err := sql.Open("postgres", dbURL)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	// Set connection pool settings
	conn.SetMaxOpenConns(25)
	conn.SetMaxIdleConns(5)
	conn.SetConnMaxLifetime(5 * time.Minute)

	// Test connection with retries (for container startup)
	var pingErr error
	maxRetries := 30
	retryDelay := 2 * time.Second
	for i := 0; i < maxRetries; i++ {
		pingErr = conn.Ping()
		if pingErr == nil {
			break
		}
		if i < maxRetries-1 {
			time.Sleep(retryDelay)
		}
	}
	if pingErr != nil {
		conn.Close()
		return nil, fmt.Errorf("failed to ping database after %d retries: %w", maxRetries, pingErr)
	}

	db := &DB{conn: conn}
	if err := db.initSchema(); err != nil {
		conn.Close()
		return nil, err
	}

	return db, nil
}

// initSchema creates the necessary tables if they don't exist and migrates existing schema
func (db *DB) initSchema() error {
	// Create table if it doesn't exist
	schema := `
	CREATE TABLE IF NOT EXISTS videos (
		id SERIAL PRIMARY KEY,
		video_id TEXT NOT NULL,
		channel_url TEXT NOT NULL,
		title TEXT NOT NULL,
		file_path TEXT NOT NULL,
		downloaded_at TIMESTAMP NOT NULL,
		uploaded_at TIMESTAMP,
		telegram_file_id TEXT,
		telegram_file_path TEXT,
		UNIQUE(video_id, file_path)
	);
	CREATE INDEX IF NOT EXISTS idx_videos_video_id ON videos(video_id);
	CREATE INDEX IF NOT EXISTS idx_videos_channel_url ON videos(channel_url);
	CREATE INDEX IF NOT EXISTS idx_videos_file_path ON videos(file_path);
	CREATE INDEX IF NOT EXISTS idx_videos_telegram_file_id ON videos(telegram_file_id);
	-- Note: Table name and column names kept for backward compatibility
	-- video_id stores torrent URL/ID, channel_url stores torrent URL
	-- telegram_file_id stores Telegram file ID for downloading
	-- UNIQUE constraint on (video_id, file_path) ensures each file is tracked individually
	`

	if _, err := db.conn.Exec(schema); err != nil {
		return fmt.Errorf("failed to initialize database schema: %w", err)
	}

	// Add telegram_file_path column if it doesn't exist (for large files)
	_, _ = db.conn.Exec("ALTER TABLE videos ADD COLUMN IF NOT EXISTS telegram_file_path TEXT")

	// Add season-related columns if they don't exist
	_, _ = db.conn.Exec("ALTER TABLE videos ADD COLUMN IF NOT EXISTS show_name TEXT")
	_, _ = db.conn.Exec("ALTER TABLE videos ADD COLUMN IF NOT EXISTS season_number INTEGER DEFAULT 0")
	_, _ = db.conn.Exec("ALTER TABLE videos ADD COLUMN IF NOT EXISTS episode_number INTEGER DEFAULT 0")

	// Create indexes for season queries
	_, _ = db.conn.Exec("CREATE INDEX IF NOT EXISTS idx_videos_show_name ON videos(show_name)")
	_, _ = db.conn.Exec("CREATE INDEX IF NOT EXISTS idx_videos_season ON videos(show_name, season_number)")

	// Migrate existing VARCHAR(255) columns to TEXT if they exist
	migrations := []string{
		"ALTER TABLE videos ALTER COLUMN video_id TYPE TEXT",
		"ALTER TABLE videos ALTER COLUMN channel_url TYPE TEXT",
		"ALTER TABLE videos ALTER COLUMN title TYPE TEXT",
		"ALTER TABLE videos ALTER COLUMN telegram_file_id TYPE TEXT",
	}

	for _, migration := range migrations {
		// Ignore errors if column doesn't exist or is already TEXT
		db.conn.Exec(migration)
	}

	return nil
}

// Close closes the database connection
func (db *DB) Close() error {
	return db.conn.Close()
}

// Ping checks if the database connection is still alive
func (db *DB) Ping() error {
	return db.conn.Ping()
}

// IsVideoDownloaded checks if a file/torrent has already been downloaded
// If filePath is provided, checks for that specific file; otherwise checks if any file from the torrent exists
func (db *DB) IsVideoDownloaded(videoID, filePath string) (bool, error) {
	if filePath != "" {
		// Check for specific file
		var count int
		err := db.conn.QueryRow("SELECT COUNT(*) FROM videos WHERE video_id = $1 AND file_path = $2", videoID, filePath).Scan(&count)
		if err != nil {
			return false, fmt.Errorf("failed to check file: %w", err)
		}
		return count > 0, nil
	}
	// Check if any file from this torrent exists
	var count int
	err := db.conn.QueryRow("SELECT COUNT(*) FROM videos WHERE video_id = $1", videoID).Scan(&count)
	if err != nil {
		return false, fmt.Errorf("failed to check torrent: %w", err)
	}
	return count > 0, nil
}

// AddVideo adds a new file/torrent record to the database
// If the video already exists (duplicate key), it's not an error - just skip
func (db *DB) AddVideo(videoID, channelURL, title, filePath, showName string, seasonNumber, episodeNumber int) error {
	_, err := db.conn.Exec(
		"INSERT INTO videos (video_id, channel_url, title, file_path, downloaded_at, show_name, season_number, episode_number) VALUES ($1, $2, $3, $4, $5, $6, $7, $8)",
		videoID, channelURL, title, filePath, time.Now(), showName, seasonNumber, episodeNumber,
	)
	if err != nil {
		// Check if it's a duplicate key error (UNIQUE constraint violation)
		// PostgreSQL returns "duplicate key value violates unique constraint" error
		if strings.Contains(err.Error(), "unique constraint") || strings.Contains(err.Error(), "duplicate key") {
			// Already exists, not an error
			return nil
		}
		return fmt.Errorf("failed to add file: %w", err)
	}
	return nil
}

// UpdateTelegramFileID updates the Telegram file ID for a video
func (db *DB) UpdateTelegramFileID(videoID, filePath, telegramFileID string) error {
	_, err := db.conn.Exec(
		"UPDATE videos SET telegram_file_id = $1 WHERE video_id = $2 AND file_path = $3",
		telegramFileID, videoID, filePath,
	)
	if err != nil {
		return fmt.Errorf("failed to update Telegram file ID: %w", err)
	}
	return nil
}

// UpdateTelegramFileInfo updates both Telegram file ID and file path for a video
func (db *DB) UpdateTelegramFileInfo(videoID, filePath, telegramFileID, telegramFilePath string) error {
	_, err := db.conn.Exec(
		"UPDATE videos SET telegram_file_id = $1, telegram_file_path = $2 WHERE video_id = $3 AND file_path = $4",
		telegramFileID, telegramFilePath, videoID, filePath,
	)
	if err != nil {
		return fmt.Errorf("failed to update Telegram file info: %w", err)
	}
	return nil
}

// MarkUploaded marks a file/torrent as uploaded to Telegram
// If filePath is provided, marks that specific file; otherwise marks all files from the torrent
func (db *DB) MarkUploaded(videoID, filePath string) error {
	if filePath != "" {
		// Mark specific file
		_, err := db.conn.Exec(
			"UPDATE videos SET uploaded_at = $1 WHERE video_id = $2 AND file_path = $3",
			time.Now(), videoID, filePath,
		)
		if err != nil {
			return fmt.Errorf("failed to mark file as uploaded: %w", err)
		}
		return nil
	}
	// Mark all files from this torrent
	_, err := db.conn.Exec(
		"UPDATE videos SET uploaded_at = $1 WHERE video_id = $2",
		time.Now(), videoID,
	)
	if err != nil {
		return fmt.Errorf("failed to mark torrent as uploaded: %w", err)
	}
	return nil
}

// GetPendingUploads returns files/torrents that have been downloaded but not uploaded
func (db *DB) GetPendingUploads() ([]Video, error) {
	rows, err := db.conn.Query(
		"SELECT id, video_id, channel_url, title, file_path, downloaded_at FROM videos WHERE uploaded_at IS NULL",
	)
	if err != nil {
		return nil, fmt.Errorf("failed to query pending uploads: %w", err)
	}
	defer rows.Close()

	var videos []Video
	for rows.Next() {
		var v Video
		if err := rows.Scan(&v.ID, &v.VideoID, &v.ChannelURL, &v.Title, &v.FilePath, &v.DownloadedAt); err != nil {
			return nil, fmt.Errorf("failed to scan file row: %w", err)
		}
		videos = append(videos, v)
	}

	return videos, rows.Err()
}

// GetAllVideos returns all downloaded files/torrents
func (db *DB) GetAllVideos() ([]Video, error) {
	rows, err := db.conn.Query(
		"SELECT id, video_id, channel_url, title, file_path, downloaded_at, uploaded_at, telegram_file_id, telegram_file_path, COALESCE(show_name, ''), COALESCE(season_number, 0), COALESCE(episode_number, 0) FROM videos ORDER BY downloaded_at DESC",
	)
	if err != nil {
		return nil, fmt.Errorf("failed to query files: %w", err)
	}
	defer rows.Close()

	var videos []Video
	for rows.Next() {
		var v Video
		var uploadedAt sql.NullTime
		var telegramFileID sql.NullString
		var telegramFilePath sql.NullString
		if err := rows.Scan(&v.ID, &v.VideoID, &v.ChannelURL, &v.Title, &v.FilePath, &v.DownloadedAt, &uploadedAt, &telegramFileID, &telegramFilePath, &v.ShowName, &v.SeasonNumber, &v.EpisodeNumber); err != nil {
			return nil, fmt.Errorf("failed to scan file row: %w", err)
		}
		if uploadedAt.Valid {
			v.UploadedAt = &uploadedAt.Time
		}
		if telegramFileID.Valid {
			v.TelegramFileID = telegramFileID.String
		}
		if telegramFilePath.Valid {
			v.TelegramFilePath = telegramFilePath.String
		}
		videos = append(videos, v)
	}

	return videos, rows.Err()
}

// GetVideoByID returns a video by its database ID
func (db *DB) GetVideoByID(id int64) (*Video, error) {
	var v Video
	var uploadedAt sql.NullTime
	var telegramFileID sql.NullString
	var telegramFilePath sql.NullString

	err := db.conn.QueryRow(
		"SELECT id, video_id, channel_url, title, file_path, downloaded_at, uploaded_at, telegram_file_id, telegram_file_path, COALESCE(show_name, ''), COALESCE(season_number, 0), COALESCE(episode_number, 0) FROM videos WHERE id = $1",
		id,
	).Scan(&v.ID, &v.VideoID, &v.ChannelURL, &v.Title, &v.FilePath, &v.DownloadedAt, &uploadedAt, &telegramFileID, &telegramFilePath, &v.ShowName, &v.SeasonNumber, &v.EpisodeNumber)

	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("video not found")
		}
		return nil, fmt.Errorf("failed to get video: %w", err)
	}

	if uploadedAt.Valid {
		v.UploadedAt = &uploadedAt.Time
	}
	if telegramFileID.Valid {
		v.TelegramFileID = telegramFileID.String
	}
	if telegramFilePath.Valid {
		v.TelegramFilePath = telegramFilePath.String
	}

	return &v, nil
}

// Show represents a TV show with its season count
type Show struct {
	Name         string `json:"name"`
	SeasonCount  int    `json:"seasonCount"`
	EpisodeCount int    `json:"episodeCount"`
}

// Season represents a season with episode count
type Season struct {
	SeasonNumber int `json:"seasonNumber"`
	EpisodeCount int `json:"episodeCount"`
}

// GetAllShows returns all unique shows with their season and episode counts
// Groups by show_name (parsed from torrents), falling back to title if show_name is not available
func (db *DB) GetAllShows() ([]Show, error) {
	rows, err := db.conn.Query(`
		SELECT
			COALESCE(NULLIF(show_name, ''), title) as show_name,
			COUNT(DISTINCT season_number) as season_count,
			COUNT(*) as episode_count
		FROM videos
		WHERE uploaded_at IS NOT NULL
		GROUP BY COALESCE(NULLIF(show_name, ''), title)
		ORDER BY show_name
	`)
	if err != nil {
		return nil, fmt.Errorf("failed to query shows: %w", err)
	}
	defer rows.Close()

	var shows []Show
	for rows.Next() {
		var s Show
		if err := rows.Scan(&s.Name, &s.SeasonCount, &s.EpisodeCount); err != nil {
			return nil, fmt.Errorf("failed to scan show row: %w", err)
		}
		shows = append(shows, s)
	}

	return shows, rows.Err()
}

// GetSeasonsByShow returns all seasons for a specific show
func (db *DB) GetSeasonsByShow(showName string) ([]Season, error) {
	rows, err := db.conn.Query(`
		SELECT
			season_number,
			COUNT(*) as episode_count
		FROM videos
		WHERE COALESCE(NULLIF(show_name, ''), title) = $1
		GROUP BY season_number
		ORDER BY season_number
	`, showName)
	if err != nil {
		return nil, fmt.Errorf("failed to query seasons: %w", err)
	}
	defer rows.Close()

	var seasons []Season
	for rows.Next() {
		var s Season
		if err := rows.Scan(&s.SeasonNumber, &s.EpisodeCount); err != nil {
			return nil, fmt.Errorf("failed to scan season row: %w", err)
		}
		seasons = append(seasons, s)
	}

	return seasons, rows.Err()
}

// GetEpisodesByShowAndSeason returns all episodes for a specific show and season
func (db *DB) GetEpisodesByShowAndSeason(showName string, seasonNumber int) ([]Video, error) {
	rows, err := db.conn.Query(`
		SELECT
			id, video_id, channel_url, title, file_path, downloaded_at,
			uploaded_at, telegram_file_id, telegram_file_path,
			COALESCE(show_name, ''), COALESCE(season_number, 0), COALESCE(episode_number, 0)
		FROM videos
		WHERE COALESCE(NULLIF(show_name, ''), title) = $1 AND season_number = $2
		ORDER BY episode_number, file_path
	`, showName, seasonNumber)
	if err != nil {
		return nil, fmt.Errorf("failed to query episodes: %w", err)
	}
	defer rows.Close()

	var videos []Video
	for rows.Next() {
		var v Video
		var uploadedAt sql.NullTime
		var telegramFileID sql.NullString
		var telegramFilePath sql.NullString
		if err := rows.Scan(&v.ID, &v.VideoID, &v.ChannelURL, &v.Title, &v.FilePath, &v.DownloadedAt, &uploadedAt, &telegramFileID, &telegramFilePath, &v.ShowName, &v.SeasonNumber, &v.EpisodeNumber); err != nil {
			return nil, fmt.Errorf("failed to scan episode row: %w", err)
		}
		if uploadedAt.Valid {
			v.UploadedAt = &uploadedAt.Time
		}
		if telegramFileID.Valid {
			v.TelegramFileID = telegramFileID.String
		}
		if telegramFilePath.Valid {
			v.TelegramFilePath = telegramFilePath.String
		}
		videos = append(videos, v)
	}

	return videos, rows.Err()
}

// UpdateVideoInfo updates the show name, season, and episode information for a video
func (db *DB) UpdateVideoInfo(id int64, showName string, seasonNumber, episodeNumber int) error {
	_, err := db.conn.Exec(
		"UPDATE videos SET show_name = $1, season_number = $2, episode_number = $3 WHERE id = $4",
		showName, seasonNumber, episodeNumber, id,
	)
	if err != nil {
		return fmt.Errorf("failed to update video info: %w", err)
	}
	return nil
}

// UpdateVideoTelegramInfo updates the Telegram file ID and path for a video by ID
func (db *DB) UpdateVideoTelegramInfo(id int64, telegramFileID, telegramFilePath string) error {
	_, err := db.conn.Exec(
		"UPDATE videos SET telegram_file_id = $1, telegram_file_path = $2 WHERE id = $3",
		telegramFileID, telegramFilePath, id,
	)
	if err != nil {
		return fmt.Errorf("failed to update video Telegram info: %w", err)
	}
	return nil
}
