// Package web provides the web interface for viewing and streaming videos
package web

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/rusik69/yttg/pkg/database"
	"github.com/rusik69/yttg/pkg/telegram"
)

// Server handles HTTP requests for the web interface
type Server struct {
	db          *database.DB
	downloadDir string
	uploader    *telegram.Uploader
	mux         *http.ServeMux
	username    string
	password    string
	sessions    map[string]time.Time
	sessionsMu  sync.RWMutex
}

// NewServer creates a new web server
func NewServer(db *database.DB, downloadDir, botToken string, chatID int64, apiURL, username, password string) *Server {
	s := &Server{
		db:          db,
		downloadDir: downloadDir,
		mux:         http.NewServeMux(),
		username:    username,
		password:    password,
		sessions:    make(map[string]time.Time),
	}

	// Initialize Telegram uploader for downloading videos
	if botToken != "" && chatID != 0 && apiURL != "" {
		uploader, err := telegram.NewUploader(botToken, chatID, apiURL)
		if err != nil {
			log.Printf("Warning: Failed to initialize Telegram uploader: %v", err)
		} else {
			s.uploader = uploader
		}
	}

	// Ensure download directory exists
	if err := os.MkdirAll(downloadDir, 0755); err != nil {
		log.Printf("Warning: Failed to create download directory: %v", err)
	}

	// Setup routes
	s.mux.HandleFunc("/login", s.handleLogin)
	s.mux.HandleFunc("/logout", s.handleLogout)
	s.mux.HandleFunc("/", s.requireAuth(s.handleIndex))
	s.mux.HandleFunc("/channel/", s.requireAuth(s.handleChannel))
	s.mux.HandleFunc("/api/channels", s.requireAuth(s.handleAPIChannels))
	s.mux.HandleFunc("/api/channel/", s.requireAuth(s.handleAPIChannel))
	s.mux.HandleFunc("/api/download/", s.requireAuth(s.handleAPIDownload))
	s.mux.HandleFunc("/api/stream/", s.requireAuth(s.handleAPIStream))
	s.mux.HandleFunc("/static/", s.handleStatic)

	// Clean up expired sessions periodically
	go s.cleanupSessions()

	return s
}

// ServeHTTP implements http.Handler
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.mux.ServeHTTP(w, r)
}

// handleIndex shows the channel list page
func (s *Server) handleIndex(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}

	tmpl := `<!DOCTYPE html>
<html>
<head>
	<title>Video Channels</title>
	<meta charset="utf-8">
	<meta name="viewport" content="width=device-width, initial-scale=1">
	<style>
		* { margin: 0; padding: 0; box-sizing: border-box; }
		body { font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif; background: #1a1a1a; color: #fff; padding: 20px; }
		.container { max-width: 1200px; margin: 0 auto; }
		.header { display: flex; justify-content: space-between; align-items: center; margin-bottom: 30px; }
		h1 { margin: 0; }
		.logout-btn { background: #dc3545; color: white; border: none; padding: 10px 20px; border-radius: 4px; cursor: pointer; text-decoration: none; display: inline-block; }
		.logout-btn:hover { background: #c82333; }
		.channels { display: grid; grid-template-columns: repeat(auto-fill, minmax(300px, 1fr)); gap: 20px; }
		.channel-card { background: #2a2a2a; border-radius: 8px; padding: 20px; cursor: pointer; transition: transform 0.2s, background 0.2s; }
		.channel-card:hover { transform: translateY(-2px); background: #3a3a3a; }
		.channel-name { font-size: 18px; font-weight: bold; margin-bottom: 10px; }
		.channel-info { color: #aaa; font-size: 14px; }
		a { text-decoration: none; color: inherit; }
	</style>
</head>
<body>
	<div class="container">
		<div class="header">
			<h1>Video Channels</h1>
			<a href="/logout" class="logout-btn">Logout</a>
		</div>
		<div class="channels" id="channels"></div>
	</div>
	<script>
		fetch('/api/channels')
			.then(r => r.json())
			.then(channels => {
				const container = document.getElementById('channels');
				channels.forEach(ch => {
					const card = document.createElement('a');
					card.href = '/channel/' + encodeURIComponent(ch.name);
					card.className = 'channel-card';
					card.innerHTML = '<div class="channel-name">' + escapeHtml(ch.name) + '</div><div class="channel-info">' + ch.videoCount + ' videos</div>';
					container.appendChild(card);
				});
			});
		function escapeHtml(text) {
			const div = document.createElement('div');
			div.textContent = text;
			return div.innerHTML;
		}
	</script>
</body>
</html>`

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	fmt.Fprint(w, tmpl)
}

// handleChannel shows videos for a specific channel
func (s *Server) handleChannel(w http.ResponseWriter, r *http.Request) {
	channelName := strings.TrimPrefix(r.URL.Path, "/channel/")
	if channelName == "" {
		http.Redirect(w, r, "/", http.StatusFound)
		return
	}

	tmpl := `<!DOCTYPE html>
<html>
<head>
	<title>{{.ChannelName}} - Videos</title>
	<meta charset="utf-8">
	<meta name="viewport" content="width=device-width, initial-scale=1">
	<style>
		* { margin: 0; padding: 0; box-sizing: border-box; }
		body { font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif; background: #1a1a1a; color: #fff; padding: 20px; }
		.container { max-width: 1400px; margin: 0 auto; }
		.header { margin-bottom: 30px; }
		.back-link { color: #4a9eff; text-decoration: none; margin-bottom: 20px; display: inline-block; }
		h1 { margin-bottom: 10px; }
		.videos { display: grid; grid-template-columns: repeat(auto-fill, minmax(300px, 1fr)); gap: 20px; }
		.video-card { background: #2a2a2a; border-radius: 8px; padding: 15px; transition: transform 0.2s, background 0.2s; }
		.video-card:hover { transform: translateY(-2px); background: #3a3a3a; }
		.video-title { font-size: 16px; margin-bottom: 10px; }
		.video-info { color: #aaa; font-size: 12px; margin-bottom: 10px; }
		.download-btn { background: #4a9eff; color: white; border: none; padding: 8px 16px; border-radius: 4px; cursor: pointer; margin-right: 10px; }
		.download-btn:hover { background: #5aaeff; }
		.download-btn:disabled { background: #555; cursor: not-allowed; }
		.play-btn { background: #28a745; color: white; border: none; padding: 8px 16px; border-radius: 4px; cursor: pointer; }
		.play-btn:hover { background: #34ce57; }
		.video-player { display: none; position: fixed; top: 0; left: 0; width: 100%; height: 100%; background: rgba(0,0,0,0.95); z-index: 1000; }
		.video-player.active { display: flex; align-items: center; justify-content: center; }
		.video-player video { max-width: 100%; max-height: 100%; }
		.close-btn { position: absolute; top: 20px; right: 20px; background: #dc3545; color: white; border: none; padding: 10px 20px; border-radius: 4px; cursor: pointer; font-size: 18px; }
		.close-btn:hover { background: #c82333; }
	</style>
</head>
<body>
	<div class="container">
		<div class="header">
			<div>
				<a href="/" class="back-link">← Back to Channels</a>
				<h1 id="channelName"></h1>
			</div>
			<a href="/logout" class="logout-btn">Logout</a>
		</div>
		<div class="videos" id="videos"></div>
	</div>
	<div class="video-player" id="videoPlayer">
		<button class="close-btn" onclick="closePlayer()">×</button>
		<video id="videoElement" controls autoplay></video>
	</div>
	<script>
		const channelName = decodeURIComponent('{{.ChannelName}}');
		document.getElementById('channelName').textContent = channelName;
		
		fetch('/api/channel/' + encodeURIComponent(channelName))
			.then(r => r.json())
			.then(data => {
				const container = document.getElementById('videos');
				data.videos.forEach(video => {
					const card = document.createElement('div');
					card.className = 'video-card';
					const hasTelegram = video.localPath !== undefined && video.localPath !== '' && video.localPath.startsWith('telegram://');
					const playBtn = '<button class="play-btn" onclick="playVideo(' + video.id + ')">Play</button>';
					card.innerHTML = '<div class="video-title">' + escapeHtml(video.title) + '</div><div class="video-info">Downloaded: ' + video.downloadedAt + '</div>' + playBtn;
					container.appendChild(card);
				});
			});
		
		function downloadVideo(videoId, btn) {
			btn.disabled = true;
			btn.textContent = 'Downloading...';
			fetch('/api/download/' + videoId, { method: 'POST' })
				.then(r => r.json())
				.then(data => {
					if (data.error) {
						alert('Error: ' + data.error);
						btn.disabled = false;
						btn.textContent = 'Download';
					} else {
						location.reload();
					}
				});
		}
		
		function playVideo(videoId) {
			const player = document.getElementById('videoPlayer');
			const video = document.getElementById('videoElement');
			video.src = '/api/stream/' + videoId;
			player.classList.add('active');
			video.play();
		}
		
		function closePlayer() {
			const player = document.getElementById('videoPlayer');
			const video = document.getElementById('videoElement');
			player.classList.remove('active');
			video.pause();
			video.src = '';
		}
		
		function escapeHtml(text) {
			const div = document.createElement('div');
			div.textContent = text;
			return div.innerHTML;
		}
	</script>
</body>
</html>`

	t, _ := template.New("channel").Parse(tmpl)
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	t.Execute(w, map[string]string{"ChannelName": channelName})
}

// handleAPIChannels returns list of channels
func (s *Server) handleAPIChannels(w http.ResponseWriter, r *http.Request) {
	videos, err := s.db.GetAllVideos()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Extract channel names from video titles (torrent names)
	// Count all videos that have been uploaded to Telegram
	channelMap := make(map[string]int)
	for _, video := range videos {
		// Channel name is the torrent title (stored in Title field)
		channelName := video.Title
		if channelName == "" {
			continue
		}

		// Only count videos that have been uploaded (have uploaded_at timestamp)
		if video.UploadedAt != nil {
			channelMap[channelName]++
		}
	}

	type Channel struct {
		Name       string `json:"name"`
		VideoCount int    `json:"videoCount"`
	}

	channels := make([]Channel, 0, len(channelMap))
	for name, count := range channelMap {
		channels = append(channels, Channel{Name: name, VideoCount: count})
	}

	sort.Slice(channels, func(i, j int) bool {
		return channels[i].Name < channels[j].Name
	})

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(channels)
}

// handleAPIChannel returns videos for a specific channel
func (s *Server) handleAPIChannel(w http.ResponseWriter, r *http.Request) {
	channelNameEncoded := strings.TrimPrefix(r.URL.Path, "/api/channel/")
	if channelNameEncoded == "" {
		http.Error(w, "Channel name required", http.StatusBadRequest)
		return
	}

	// URL decode the channel name
	channelName, err := url.QueryUnescape(channelNameEncoded)
	if err != nil {
		// If decoding fails, use the original
		channelName = channelNameEncoded
	}

	videos, err := s.db.GetAllVideos()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	type Video struct {
		ID           int64  `json:"id"`
		Title        string `json:"title"`
		FilePath     string `json:"filePath"`
		DownloadedAt string `json:"downloadedAt"`
		LocalPath    string `json:"localPath,omitempty"`
	}

	result := struct {
		ChannelName string  `json:"channelName"`
		Videos      []Video `json:"videos"`
	}{
		ChannelName: channelName,
		Videos:      []Video{},
	}

	for _, video := range videos {
		// Match channel name (torrent title stored in video.Title)
		// Only show videos that have been uploaded to Telegram
		if video.Title == channelName && video.UploadedAt != nil {
			// Extract a better title from the file path
			videoTitle := filepath.Base(video.FilePath)
			if videoTitle == "" || videoTitle == "." {
				videoTitle = fmt.Sprintf("Video %d", video.ID)
			}

			v := Video{
				ID:           video.ID,
				Title:        videoTitle,
				FilePath:     video.FilePath,
				DownloadedAt: video.DownloadedAt.Format(time.RFC3339),
			}

			// If Telegram file ID exists, mark as downloadable
			if video.TelegramFileID != "" {
				v.LocalPath = "telegram://" + video.TelegramFileID // Use special prefix to indicate Telegram source
			}

			result.Videos = append(result.Videos, v)
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(result)
}

// handleAPIDownload downloads a video from Telegram (triggers download for streaming)
func (s *Server) handleAPIDownload(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	videoIDStr := strings.TrimPrefix(r.URL.Path, "/api/download/")
	if videoIDStr == "" {
		json.NewEncoder(w).Encode(map[string]string{"error": "Video ID required"})
		return
	}

	// Get video from database
	videos, err := s.db.GetAllVideos()
	if err != nil {
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}

	var video *database.Video
	for _, v := range videos {
		if v.ID == parseVideoID(videoIDStr) {
			video = &v
			break
		}
	}

	if video == nil {
		json.NewEncoder(w).Encode(map[string]string{"error": "Video not found"})
		return
	}

	if video.TelegramFileID == "" {
		json.NewEncoder(w).Encode(map[string]string{"error": "Telegram file ID not available"})
		return
	}

	// Check if already cached
	localPath := filepath.Join(s.downloadDir, fmt.Sprintf("cache_%d_%s", video.ID, filepath.Base(video.FilePath)))
	if _, err := os.Stat(localPath); err == nil {
		json.NewEncoder(w).Encode(map[string]string{"status": "ok", "path": localPath})
		return
	}

	// Download will happen when streaming, so just return success
	json.NewEncoder(w).Encode(map[string]string{"status": "ok", "message": "Video will be downloaded when streaming"})
}

// handleAPIStream streams a video file, downloading from Telegram if needed
func (s *Server) handleAPIStream(w http.ResponseWriter, r *http.Request) {
	videoIDStr := strings.TrimPrefix(r.URL.Path, "/api/stream/")
	if videoIDStr == "" {
		http.Error(w, "Video ID required", http.StatusBadRequest)
		return
	}

	// Get video from database
	videos, err := s.db.GetAllVideos()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	var video *database.Video
	for _, v := range videos {
		if v.ID == parseVideoID(videoIDStr) {
			video = &v
			break
		}
	}

	if video == nil {
		http.Error(w, "Video not found", http.StatusNotFound)
		return
	}

	if video.TelegramFileID == "" {
		http.Error(w, "Video file ID not available", http.StatusNotFound)
		return
	}

	// Download from Telegram if not already cached locally
	localPath := filepath.Join(s.downloadDir, fmt.Sprintf("cache_%d_%s", video.ID, filepath.Base(video.FilePath)))

	// Check if already downloaded
	if _, err := os.Stat(localPath); os.IsNotExist(err) {
		// Download from Telegram
		if s.uploader == nil {
			http.Error(w, "Telegram downloader not configured", http.StatusInternalServerError)
			return
		}

		// Create downloader from uploader's bot
		downloader, err := telegram.NewDownloaderFromUploader(s.uploader)
		if err != nil {
			http.Error(w, fmt.Sprintf("Failed to create downloader: %v", err), http.StatusInternalServerError)
			return
		}

		// Ensure directory exists
		if err := os.MkdirAll(filepath.Dir(localPath), 0755); err != nil {
			http.Error(w, fmt.Sprintf("Failed to create cache directory: %v", err), http.StatusInternalServerError)
			return
		}

		// Download file
		if err := downloader.DownloadFile(video.TelegramFileID, localPath); err != nil {
			http.Error(w, fmt.Sprintf("Failed to download from Telegram: %v", err), http.StatusInternalServerError)
			return
		}
	}

	// Set headers for video streaming
	w.Header().Set("Content-Type", "video/mp4")
	w.Header().Set("Accept-Ranges", "bytes")
	w.Header().Set("Cache-Control", "public, max-age=3600")

	// Serve video with proper headers for streaming
	http.ServeFile(w, r, localPath)
}

// handleStatic serves static files
func (s *Server) handleStatic(w http.ResponseWriter, r *http.Request) {
	http.NotFound(w, r)
}

// parseVideoID parses video ID from string
func parseVideoID(s string) int64 {
	id, _ := strconv.ParseInt(s, 10, 64)
	return id
}

// requireAuth wraps a handler to require authentication
func (s *Server) requireAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		sessionID := s.getSessionID(r)
		if !s.isValidSession(sessionID) {
			http.Redirect(w, r, "/login?redirect="+r.URL.Path, http.StatusFound)
			return
		}
		next(w, r)
	}
}

// handleLogin handles login requests
func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	if r.Method == "GET" {
		// Show login page
		tmpl := `<!DOCTYPE html>
<html>
<head>
	<title>Login</title>
	<meta charset="utf-8">
	<meta name="viewport" content="width=device-width, initial-scale=1">
	<style>
		* { margin: 0; padding: 0; box-sizing: border-box; }
		body { font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif; background: #1a1a1a; color: #fff; display: flex; align-items: center; justify-content: center; min-height: 100vh; }
		.login-container { background: #2a2a2a; border-radius: 8px; padding: 40px; width: 100%; max-width: 400px; }
		h1 { margin-bottom: 30px; text-align: center; }
		.form-group { margin-bottom: 20px; }
		label { display: block; margin-bottom: 8px; }
		input { width: 100%; padding: 12px; border: 1px solid #444; border-radius: 4px; background: #1a1a1a; color: #fff; font-size: 16px; }
		input:focus { outline: none; border-color: #4a9eff; }
		button { width: 100%; padding: 12px; background: #4a9eff; color: white; border: none; border-radius: 4px; cursor: pointer; font-size: 16px; }
		button:hover { background: #5aaeff; }
		.error { color: #dc3545; margin-top: 10px; text-align: center; }
	</style>
</head>
<body>
	<div class="login-container">
		<h1>Login</h1>
		<form method="POST" action="/login">
			<input type="hidden" name="redirect" value="{{.Redirect}}">
			<div class="form-group">
				<label for="username">Username</label>
				<input type="text" id="username" name="username" required autofocus>
			</div>
			<div class="form-group">
				<label for="password">Password</label>
				<input type="password" id="password" name="password" required>
			</div>
			<button type="submit">Login</button>
			{{if .Error}}<div class="error">{{.Error}}</div>{{end}}
		</form>
	</div>
</body>
</html>`

		redirect := r.URL.Query().Get("redirect")
		if redirect == "" {
			redirect = "/"
		}

		t, _ := template.New("login").Parse(tmpl)
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		t.Execute(w, map[string]string{
			"Redirect": redirect,
			"Error":    r.URL.Query().Get("error"),
		})
		return
	}

	// Handle POST login
	if r.Method == "POST" {
		username := r.FormValue("username")
		password := r.FormValue("password")
		redirect := r.FormValue("redirect")
		if redirect == "" {
			redirect = "/"
		}

		if username == s.username && password == s.password {
			// Create session
			sessionID := s.createSession()
			http.SetCookie(w, &http.Cookie{
				Name:     "session",
				Value:    sessionID,
				Path:     "/",
				MaxAge:   86400, // 24 hours
				HttpOnly: true,
				Secure:   false, // Set to true if using HTTPS
			})
			http.Redirect(w, r, redirect, http.StatusFound)
			return
		}

		// Invalid credentials
		http.Redirect(w, r, "/login?redirect="+redirect+"&error=Invalid+username+or+password", http.StatusFound)
		return
	}

	http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
}

// handleLogout handles logout requests
func (s *Server) handleLogout(w http.ResponseWriter, r *http.Request) {
	sessionID := s.getSessionID(r)
	if sessionID != "" {
		s.deleteSession(sessionID)
	}
	http.SetCookie(w, &http.Cookie{
		Name:     "session",
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
	})
	http.Redirect(w, r, "/login", http.StatusFound)
}

// getSessionID retrieves session ID from cookie
func (s *Server) getSessionID(r *http.Request) string {
	cookie, err := r.Cookie("session")
	if err != nil {
		return ""
	}
	return cookie.Value
}

// isValidSession checks if a session is valid
func (s *Server) isValidSession(sessionID string) bool {
	if sessionID == "" {
		return false
	}
	s.sessionsMu.RLock()
	defer s.sessionsMu.RUnlock()
	expiry, exists := s.sessions[sessionID]
	return exists && time.Now().Before(expiry)
}

// createSession creates a new session
func (s *Server) createSession() string {
	sessionID := generateSessionID()
	s.sessionsMu.Lock()
	defer s.sessionsMu.Unlock()
	s.sessions[sessionID] = time.Now().Add(24 * time.Hour)
	return sessionID
}

// deleteSession deletes a session
func (s *Server) deleteSession(sessionID string) {
	s.sessionsMu.Lock()
	defer s.sessionsMu.Unlock()
	delete(s.sessions, sessionID)
}

// cleanupSessions removes expired sessions periodically
func (s *Server) cleanupSessions() {
	ticker := time.NewTicker(1 * time.Hour)
	defer ticker.Stop()
	for range ticker.C {
		s.sessionsMu.Lock()
		now := time.Now()
		for sessionID, expiry := range s.sessions {
			if now.After(expiry) {
				delete(s.sessions, sessionID)
			}
		}
		s.sessionsMu.Unlock()
	}
}

// generateSessionID generates a random session ID
func generateSessionID() string {
	b := make([]byte, 32)
	rand.Read(b)
	return base64.URLEncoding.EncodeToString(b)
}
