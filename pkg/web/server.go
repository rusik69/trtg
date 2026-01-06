// Package web provides the web interface for viewing and streaming videos
package web

import (
	"bytes"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"html/template"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/rusik69/trtg/pkg/database"
	"github.com/rusik69/trtg/pkg/telegram"
)

// Server handles HTTP requests for the web interface
type Server struct {
	db             *database.DB
	downloadDir    string
	trtgAPIURL     string // URL for trtg download API (fallback)
	downloader     *telegram.Downloader
	mux            *http.ServeMux
	username       string
	password       string
	sessions       map[string]time.Time
	sessionsMu     sync.RWMutex
	currentVideo   int64 // Track currently playing video for cleanup
	currentVideoMu sync.Mutex
	token          string // Telegram bot token for local file access
}

// NewServer creates a new web server
func NewServer(db *database.DB, downloadDir, trtgAPIURL, username, password, telegramToken string, telegramChatID int64, telegramAPIURL string) *Server {
	var downloader *telegram.Downloader
	if telegramToken != "" && telegramAPIURL != "" {
		var err error
		downloader, err = telegram.NewDownloader(telegramToken, telegramChatID, telegramAPIURL)
		if err != nil {
			log.Printf("Warning: Failed to initialize Telegram downloader for direct streaming: %v", err)
		} else {
			log.Printf("Initialized Telegram downloader for direct streaming (API URL: %s)", telegramAPIURL)
		}
	}

	s := &Server{
		db:          db,
		downloadDir: downloadDir,
		trtgAPIURL:  trtgAPIURL,
		downloader:  downloader,
		mux:         http.NewServeMux(),
		username:    username,
		password:    password,
		sessions:    make(map[string]time.Time),
		token:       telegramToken,
	}

	log.Printf("Initializing web server with trtg API URL: %s", trtgAPIURL)

	// Ensure download directory exists (for any temporary files if needed)
	if err := os.MkdirAll(downloadDir, 0755); err != nil {
		log.Printf("Warning: Failed to create download directory: %v", err)
	}

	// Setup routes
	s.mux.HandleFunc("/login", s.handleLogin)
	s.mux.HandleFunc("/logout", s.handleLogout)
	s.mux.HandleFunc("/", s.requireAuth(s.handleIndex))
	s.mux.HandleFunc("/channel/", s.requireAuth(s.handleChannel))
	s.mux.HandleFunc("/show/", s.requireAuth(s.handleShow))
	s.mux.HandleFunc("/api/channels", s.requireAuth(s.handleAPIChannels))
	s.mux.HandleFunc("/api/channel/", s.requireAuth(s.handleAPIChannel))
	s.mux.HandleFunc("/api/shows", s.requireAuth(s.handleAPIShows))
	s.mux.HandleFunc("/api/show/", s.requireAuth(s.handleAPIShow))
	s.mux.HandleFunc("/api/stream/", s.requireAuth(s.handleAPIStream))
	s.mux.HandleFunc("/api/status/", s.requireAuth(s.handleAPIStatus))
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
	<title>TV Shows</title>
	<meta charset="utf-8">
	<meta name="viewport" content="width=device-width, initial-scale=1">
	<style>
		* { margin: 0; padding: 0; box-sizing: border-box; }
		body { font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif; background: #1a1a1a; color: #fff; padding: 20px; }
		.container { max-width: 1200px; margin: 0 auto; }
		.header { display: flex; justify-content: space-between; align-items: center; margin-bottom: 30px; }
		h1 { margin: 0; }
		.view-toggle { display: flex; gap: 10px; }
		.view-btn { background: #4a9eff; color: white; border: none; padding: 10px 20px; border-radius: 4px; cursor: pointer; text-decoration: none; display: inline-block; }
		.view-btn:hover { background: #5aaeff; }
		.view-btn.active { background: #28a745; }
		.logout-btn { background: #dc3545; color: white; border: none; padding: 10px 20px; border-radius: 4px; cursor: pointer; text-decoration: none; display: inline-block; }
		.logout-btn:hover { background: #c82333; }
		.shows { display: grid; grid-template-columns: repeat(auto-fill, minmax(300px, 1fr)); gap: 20px; }
		.show-card { background: #2a2a2a; border-radius: 8px; padding: 20px; cursor: pointer; transition: transform 0.2s, background 0.2s; }
		.show-card:hover { transform: translateY(-2px); background: #3a3a3a; }
		.show-name { font-size: 18px; font-weight: bold; margin-bottom: 10px; }
		.show-info { color: #aaa; font-size: 14px; }
		a { text-decoration: none; color: inherit; }
	</style>
</head>
<body>
	<div class="container">
		<div class="header">
			<div>
				<h1>TV Shows</h1>
			</div>
			<div style="display: flex; gap: 10px; align-items: center;">
				<a href="/logout" class="logout-btn">Logout</a>
			</div>
		</div>
		<div class="shows" id="shows"></div>
	</div>
	<script>
		fetch('/api/shows')
			.then(r => r.json())
			.then(shows => {
				const container = document.getElementById('shows');
				shows.forEach(show => {
					const card = document.createElement('a');
					card.href = '/show/' + encodeURIComponent(show.name);
					card.className = 'show-card';
					const seasonText = show.seasonCount === 1 ? '1 season' : show.seasonCount + ' seasons';
					const episodeText = show.episodeCount === 1 ? '1 episode' : show.episodeCount + ' episodes';
					card.innerHTML = '<div class="show-name">' + escapeHtml(show.name) + '</div><div class="show-info">' + seasonText + ' • ' + episodeText + '</div>';
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
		.header { display: flex; justify-content: space-between; align-items: flex-start; margin-bottom: 30px; }
		.logout-btn { background: #dc3545; color: white; border: none; padding: 10px 20px; border-radius: 4px; cursor: pointer; text-decoration: none; display: inline-block; }
		.logout-btn:hover { background: #c82333; }
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
		.video-player { display: none; position: fixed; top: 0; left: 0; width: 100%; height: 100%; background: rgba(0,0,0,0.95); z-index: 1000; flex-direction: column; }
		.video-player.active { display: flex; align-items: center; justify-content: center; }
		.video-player video { max-width: 100%; max-height: 100%; }
		.close-btn { position: absolute; top: 20px; right: 20px; background: #dc3545; color: white; border: none; padding: 10px 20px; border-radius: 4px; cursor: pointer; font-size: 18px; z-index: 1002; }
		.close-btn:hover { background: #c82333; }
		.audio-track-selector { position: absolute; top: 20px; left: 20px; background: rgba(0,0,0,0.8); padding: 10px 15px; border-radius: 4px; z-index: 1002; display: none; }
		.audio-track-selector.active { display: block; }
		.audio-track-selector label { margin-right: 10px; font-size: 14px; }
		.audio-track-selector select { background: #2a2a2a; color: white; border: 1px solid #444; border-radius: 4px; padding: 5px 10px; font-size: 14px; cursor: pointer; }
		.audio-track-selector select:focus { outline: none; border-color: #4a9eff; }
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
		<div class="audio-track-selector" id="audioTrackSelector">
			<label for="audioTrackSelect">Audio Track:</label>
			<select id="audioTrackSelect" onchange="changeAudioTrack()"></select>
		</div>
		<video id="videoElement" controls autoplay></video>
	</div>
	<script>
		let currentVideoId = null;
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
			currentVideoId = videoId;
			
			const player = document.getElementById('videoPlayer');
			const video = document.getElementById('videoElement');
			
			// Clear any previous error state
			video.src = '';
			video.load();
			
			const statusMsg = document.createElement('div');
			statusMsg.style.cssText = 'position: absolute; top: 50%; left: 50%; transform: translate(-50%, -50%); color: white; font-size: 18px; z-index: 1001; background: rgba(0,0,0,0.8); padding: 20px; border-radius: 8px;';
			statusMsg.textContent = 'Loading video...';
			player.appendChild(statusMsg);
			player.classList.add('active');
			
			// Stream directly from trtg (which handles downloads on-demand)
			const streamUrl = '/api/stream/' + videoId;
			console.log('Starting stream from:', streamUrl);
			
			// Clear previous error handlers
			video.onerror = null;
			video.oncanplay = null;
			
			// Set up error handler before setting src
			video.onerror = function(e) {
				console.error('Video load error:', e, 'src:', video.src, 'error code:', video.error);
				let errorMsg = 'Failed to load video';
				if (video.error) {
					switch(video.error.code) {
						case 1: errorMsg = 'Video loading aborted'; break;
						case 2: errorMsg = 'Network error loading video'; break;
						case 3: errorMsg = 'Video decoding error'; break;
						case 4: errorMsg = 'Video format not supported'; break;
					}
				}
				statusMsg.textContent = 'Error: ' + errorMsg + '. Please try again.';
				player.appendChild(statusMsg);
			};
			
			// Set up success handler
			video.oncanplay = function() {
				console.log('Video can play, starting playback');
				statusMsg.remove();
				video.play().catch(err => {
					console.error('Play error:', err);
					statusMsg.textContent = 'Error playing video: ' + err.message;
					player.appendChild(statusMsg);
				});
			};

			// Set up loadedmetadata handler to detect audio tracks
			video.onloadedmetadata = function() {
				console.log('Video metadata loaded, detecting audio tracks');
				detectAudioTracks();
			};

			// Set the source and load (trtg will download on-demand)
			video.src = streamUrl;
			video.load();
		}
		
		function closePlayer() {
			currentVideoId = null;

			const player = document.getElementById('videoPlayer');
			const video = document.getElementById('videoElement');
			const audioTrackSelector = document.getElementById('audioTrackSelector');
			player.classList.remove('active');
			audioTrackSelector.classList.remove('active');
			video.pause();
			video.src = '';
		}

		function detectAudioTracks() {
			const video = document.getElementById('videoElement');
			const selector = document.getElementById('audioTrackSelector');
			const select = document.getElementById('audioTrackSelect');

			// Clear existing options
			select.innerHTML = '';

			// Check for audio tracks
			const audioTracks = video.audioTracks;
			if (audioTracks && audioTracks.length > 1) {
				// Add options for each audio track
				for (let i = 0; i < audioTracks.length; i++) {
					const option = document.createElement('option');
					option.value = i;
					option.textContent = audioTracks[i].label || audioTracks[i].language || 'Track ' + (i + 1);
					if (audioTracks[i].enabled) {
						option.selected = true;
					}
					select.appendChild(option);
				}
				selector.classList.add('active');
			} else {
				selector.classList.remove('active');
			}
		}

		function changeAudioTrack() {
			const video = document.getElementById('videoElement');
			const select = document.getElementById('audioTrackSelect');
			const selectedIndex = parseInt(select.value);

			const audioTracks = video.audioTracks;
			if (audioTracks) {
				for (let i = 0; i < audioTracks.length; i++) {
					audioTracks[i].enabled = (i === selectedIndex);
				}
			}
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

// handleShow shows seasons for a specific show or episodes if season is specified
func (s *Server) handleShow(w http.ResponseWriter, r *http.Request) {
	// Parse URL: /show/{showName} or /show/{showName}/season/{seasonNumber}
	pathParts := strings.Split(strings.TrimPrefix(r.URL.Path, "/show/"), "/")
	if len(pathParts) == 0 || pathParts[0] == "" {
		http.Redirect(w, r, "/", http.StatusFound)
		return
	}

	showName, err := url.QueryUnescape(pathParts[0])
	if err != nil {
		showName = pathParts[0]
	}

	// Check if we're viewing a specific season
	if len(pathParts) >= 3 && pathParts[1] == "season" {
		seasonNum, _ := strconv.Atoi(pathParts[2])
		s.handleSeasonView(w, r, showName, seasonNum)
		return
	}

	// Show seasons list
	tmpl := `<!DOCTYPE html>
<html>
<head>
	<title>{{.ShowName}} - Seasons</title>
	<meta charset="utf-8">
	<meta name="viewport" content="width=device-width, initial-scale=1">
	<style>
		* { margin: 0; padding: 0; box-sizing: border-box; }
		body { font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif; background: #1a1a1a; color: #fff; padding: 20px; }
		.container { max-width: 1200px; margin: 0 auto; }
		.header { display: flex; justify-content: space-between; align-items: flex-start; margin-bottom: 30px; }
		.logout-btn { background: #dc3545; color: white; border: none; padding: 10px 20px; border-radius: 4px; cursor: pointer; text-decoration: none; display: inline-block; }
		.logout-btn:hover { background: #c82333; }
		.back-link { color: #4a9eff; text-decoration: none; margin-bottom: 20px; display: inline-block; }
		h1 { margin-bottom: 10px; }
		.seasons { display: grid; grid-template-columns: repeat(auto-fill, minmax(250px, 1fr)); gap: 20px; }
		.season-card { background: #2a2a2a; border-radius: 8px; padding: 20px; cursor: pointer; transition: transform 0.2s, background 0.2s; text-decoration: none; color: inherit; display: block; }
		.season-card:hover { transform: translateY(-2px); background: #3a3a3a; }
		.season-name { font-size: 18px; font-weight: bold; margin-bottom: 10px; }
		.season-info { color: #aaa; font-size: 14px; }
	</style>
</head>
<body>
	<div class="container">
		<div class="header">
			<div>
				<a href="/" class="back-link">← Back to Shows</a>
				<h1 id="showName"></h1>
			</div>
			<a href="/logout" class="logout-btn">Logout</a>
		</div>
		<div class="seasons" id="seasons"></div>
	</div>
	<script>
		const showName = decodeURIComponent('{{.ShowName}}');
		document.getElementById('showName').textContent = showName;

		fetch('/api/show/' + encodeURIComponent(showName))
			.then(r => r.json())
			.then(data => {
				const container = document.getElementById('seasons');
				data.seasons.forEach(season => {
					const card = document.createElement('a');
					card.href = '/show/' + encodeURIComponent(showName) + '/season/' + season.seasonNumber;
					card.className = 'season-card';
					const seasonLabel = season.seasonNumber === 0 ? 'Specials' : 'Season ' + season.seasonNumber;
					const episodeText = season.episodeCount === 1 ? '1 episode' : season.episodeCount + ' episodes';
					card.innerHTML = '<div class="season-name">' + seasonLabel + '</div><div class="season-info">' + episodeText + '</div>';
					container.appendChild(card);
				});
			});
	</script>
</body>
</html>`

	t, _ := template.New("show").Parse(tmpl)
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	t.Execute(w, map[string]string{"ShowName": showName})
}

// handleSeasonView shows episodes for a specific season
func (s *Server) handleSeasonView(w http.ResponseWriter, r *http.Request, showName string, seasonNumber int) {
	tmpl := `<!DOCTYPE html>
<html>
<head>
	<title>{{.ShowName}} - {{.SeasonLabel}}</title>
	<meta charset="utf-8">
	<meta name="viewport" content="width=device-width, initial-scale=1">
	<style>
		* { margin: 0; padding: 0; box-sizing: border-box; }
		body { font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif; background: #1a1a1a; color: #fff; padding: 20px; }
		.container { max-width: 1400px; margin: 0 auto; }
		.header { display: flex; justify-content: space-between; align-items: flex-start; margin-bottom: 30px; }
		.logout-btn { background: #dc3545; color: white; border: none; padding: 10px 20px; border-radius: 4px; cursor: pointer; text-decoration: none; display: inline-block; }
		.logout-btn:hover { background: #c82333; }
		.back-link { color: #4a9eff; text-decoration: none; margin-bottom: 10px; display: inline-block; }
		h1 { margin-bottom: 5px; }
		h2 { margin-bottom: 10px; color: #aaa; font-weight: normal; font-size: 18px; }
		.videos { display: grid; grid-template-columns: repeat(auto-fill, minmax(300px, 1fr)); gap: 20px; }
		.video-card { background: #2a2a2a; border-radius: 8px; padding: 15px; transition: transform 0.2s, background 0.2s; }
		.video-card:hover { transform: translateY(-2px); background: #3a3a3a; }
		.video-title { font-size: 16px; margin-bottom: 10px; }
		.video-info { color: #aaa; font-size: 12px; margin-bottom: 10px; }
		.play-btn { background: #28a745; color: white; border: none; padding: 8px 16px; border-radius: 4px; cursor: pointer; }
		.play-btn:hover { background: #34ce57; }
		.video-player { display: none; position: fixed; top: 0; left: 0; width: 100%; height: 100%; background: rgba(0,0,0,0.95); z-index: 1000; flex-direction: column; }
		.video-player.active { display: flex; align-items: center; justify-content: center; }
		.video-player video { max-width: 100%; max-height: 100%; }
		.close-btn { position: absolute; top: 20px; right: 20px; background: #dc3545; color: white; border: none; padding: 10px 20px; border-radius: 4px; cursor: pointer; font-size: 18px; z-index: 1002; }
		.close-btn:hover { background: #c82333; }
		.audio-track-selector { position: absolute; top: 20px; left: 20px; background: rgba(0,0,0,0.8); padding: 10px 15px; border-radius: 4px; z-index: 1002; display: none; }
		.audio-track-selector.active { display: block; }
		.audio-track-selector label { margin-right: 10px; font-size: 14px; }
		.audio-track-selector select { background: #2a2a2a; color: white; border: 1px solid #444; border-radius: 4px; padding: 5px 10px; font-size: 14px; cursor: pointer; }
		.audio-track-selector select:focus { outline: none; border-color: #4a9eff; }
	</style>
</head>
<body>
	<div class="container">
		<div class="header">
			<div>
				<a href="/show/{{.ShowNameEncoded}}" class="back-link">← Back to {{.ShowName}}</a>
				<h1 id="showName"></h1>
				<h2 id="seasonLabel"></h2>
			</div>
			<a href="/logout" class="logout-btn">Logout</a>
		</div>
		<div class="videos" id="videos"></div>
	</div>
	<div class="video-player" id="videoPlayer">
		<button class="close-btn" onclick="closePlayer()">×</button>
		<div class="audio-track-selector" id="audioTrackSelector">
			<label for="audioTrackSelect">Audio Track:</label>
			<select id="audioTrackSelect" onchange="changeAudioTrack()"></select>
		</div>
		<video id="videoElement" controls autoplay></video>
	</div>
	<script>
		const showName = decodeURIComponent('{{.ShowName}}');
		const seasonNumber = {{.SeasonNumber}};
		const seasonLabel = seasonNumber === 0 ? 'Specials' : 'Season ' + seasonNumber;

		document.getElementById('showName').textContent = showName;
		document.getElementById('seasonLabel').textContent = seasonLabel;

		fetch('/api/show/' + encodeURIComponent(showName) + '/season/' + seasonNumber)
			.then(r => r.json())
			.then(data => {
				const container = document.getElementById('videos');
				data.episodes.forEach(video => {
					const card = document.createElement('div');
					card.className = 'video-card';
					const episodeLabel = video.episodeNumber > 0 ? 'E' + video.episodeNumber + ' - ' : '';
					const playBtn = '<button class="play-btn" onclick="playVideo(' + video.id + ')">Play</button>';
					card.innerHTML = '<div class="video-title">' + episodeLabel + escapeHtml(video.title) + '</div><div class="video-info">Downloaded: ' + video.downloadedAt + '</div>' + playBtn;
					container.appendChild(card);
				});
			});

		function playVideo(videoId) {
			const player = document.getElementById('videoPlayer');
			const video = document.getElementById('videoElement');

			video.src = '';
			video.load();

			const statusMsg = document.createElement('div');
			statusMsg.style.cssText = 'position: absolute; top: 50%; left: 50%; transform: translate(-50%, -50%); color: white; font-size: 18px; z-index: 1001; background: rgba(0,0,0,0.8); padding: 20px; border-radius: 8px;';
			statusMsg.textContent = 'Loading video...';
			player.appendChild(statusMsg);
			player.classList.add('active');

			const streamUrl = '/api/stream/' + videoId;

			video.onerror = function(e) {
				let errorMsg = 'Failed to load video';
				if (video.error) {
					switch(video.error.code) {
						case 1: errorMsg = 'Video loading aborted'; break;
						case 2: errorMsg = 'Network error loading video'; break;
						case 3: errorMsg = 'Video decoding error'; break;
						case 4: errorMsg = 'Video format not supported'; break;
					}
				}
				statusMsg.textContent = 'Error: ' + errorMsg + '. Please try again.';
			};

			video.oncanplay = function() {
				statusMsg.remove();
				video.play().catch(err => {
					statusMsg.textContent = 'Error playing video: ' + err.message;
					player.appendChild(statusMsg);
				});
			};

			// Set up loadedmetadata handler to detect audio tracks
			video.onloadedmetadata = function() {
				console.log('Video metadata loaded, detecting audio tracks');
				detectAudioTracks();
			};

			video.src = streamUrl;
			video.load();
		}

		function closePlayer() {
			const player = document.getElementById('videoPlayer');
			const video = document.getElementById('videoElement');
			const audioTrackSelector = document.getElementById('audioTrackSelector');
			player.classList.remove('active');
			audioTrackSelector.classList.remove('active');
			video.pause();
			video.src = '';
		}

		function detectAudioTracks() {
			const video = document.getElementById('videoElement');
			const selector = document.getElementById('audioTrackSelector');
			const select = document.getElementById('audioTrackSelect');

			// Clear existing options
			select.innerHTML = '';

			// Check for audio tracks
			const audioTracks = video.audioTracks;
			if (audioTracks && audioTracks.length > 1) {
				// Add options for each audio track
				for (let i = 0; i < audioTracks.length; i++) {
					const option = document.createElement('option');
					option.value = i;
					option.textContent = audioTracks[i].label || audioTracks[i].language || 'Track ' + (i + 1);
					if (audioTracks[i].enabled) {
						option.selected = true;
					}
					select.appendChild(option);
				}
				selector.classList.add('active');
			} else {
				selector.classList.remove('active');
			}
		}

		function changeAudioTrack() {
			const video = document.getElementById('videoElement');
			const select = document.getElementById('audioTrackSelect');
			const selectedIndex = parseInt(select.value);

			const audioTracks = video.audioTracks;
			if (audioTracks) {
				for (let i = 0; i < audioTracks.length; i++) {
					audioTracks[i].enabled = (i === selectedIndex);
				}
			}
		}

		function escapeHtml(text) {
			const div = document.createElement('div');
			div.textContent = text;
			return div.innerHTML;
		}
	</script>
</body>
</html>`

	seasonLabel := "Extras"
	if seasonNumber > 0 {
		seasonLabel = fmt.Sprintf("Season %d", seasonNumber)
	}

	t, _ := template.New("season").Parse(tmpl)
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	t.Execute(w, map[string]interface{}{
		"ShowName":        showName,
		"ShowNameEncoded": url.QueryEscape(showName),
		"SeasonLabel":     seasonLabel,
		"SeasonNumber":    seasonNumber,
	})
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

	log.Printf("handleAPIChannels: Found %d videos in DB", len(videos))
	for i, v := range videos {
		log.Printf("Video [%d]: ID=%d, Title='%s', UploadedAt=%v", i, v.ID, v.Title, v.UploadedAt)
	}
	log.Printf("handleAPIChannels: Built %d channels", len(channels))

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

// handleAPIShows returns list of all shows with season counts
func (s *Server) handleAPIShows(w http.ResponseWriter, r *http.Request) {
	shows, err := s.db.GetAllShows()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(shows)
}

// handleAPIShow returns seasons for a show or episodes if season is specified
func (s *Server) handleAPIShow(w http.ResponseWriter, r *http.Request) {
	// Parse URL: /api/show/{showName} or /api/show/{showName}/season/{seasonNumber}
	pathParts := strings.Split(strings.TrimPrefix(r.URL.Path, "/api/show/"), "/")
	if len(pathParts) == 0 || pathParts[0] == "" {
		http.Error(w, "Show name required", http.StatusBadRequest)
		return
	}

	showName, err := url.QueryUnescape(pathParts[0])
	if err != nil {
		showName = pathParts[0]
	}

	// Check if we're requesting a specific season
	if len(pathParts) >= 3 && pathParts[1] == "season" {
		seasonNum, _ := strconv.Atoi(pathParts[2])
		s.handleAPIEpisodes(w, r, showName, seasonNum)
		return
	}

	// Return seasons for the show
	seasons, err := s.db.GetSeasonsByShow(showName)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	result := struct {
		ShowName string            `json:"showName"`
		Seasons  []database.Season `json:"seasons"`
	}{
		ShowName: showName,
		Seasons:  seasons,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(result)
}

// handleAPIEpisodes returns episodes for a specific show and season
func (s *Server) handleAPIEpisodes(w http.ResponseWriter, r *http.Request, showName string, seasonNumber int) {
	episodes, err := s.db.GetEpisodesByShowAndSeason(showName, seasonNumber)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	type Episode struct {
		ID            int64  `json:"id"`
		Title         string `json:"title"`
		FilePath      string `json:"filePath"`
		EpisodeNumber int    `json:"episodeNumber"`
		DownloadedAt  string `json:"downloadedAt"`
	}

	result := struct {
		ShowName     string    `json:"showName"`
		SeasonNumber int       `json:"seasonNumber"`
		Episodes     []Episode `json:"episodes"`
	}{
		ShowName:     showName,
		SeasonNumber: seasonNumber,
		Episodes:     []Episode{},
	}

	for _, video := range episodes {
		// Extract a better title from the file path
		videoTitle := filepath.Base(video.FilePath)
		if videoTitle == "" || videoTitle == "." {
			videoTitle = fmt.Sprintf("Video %d", video.ID)
		}

		ep := Episode{
			ID:            video.ID,
			Title:         videoTitle,
			FilePath:      video.FilePath,
			EpisodeNumber: video.EpisodeNumber,
			DownloadedAt:  video.DownloadedAt.Format(time.RFC3339),
		}

		result.Episodes = append(result.Episodes, ep)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(result)
}

// handleAPIStream proxies video streaming requests to trtg download API
func (s *Server) handleAPIStream(w http.ResponseWriter, r *http.Request) {
	videoIDStr := strings.TrimPrefix(r.URL.Path, "/api/stream/")
	if videoIDStr == "" {
		http.Error(w, "Video ID required", http.StatusBadRequest)
		return
	}

	videoID := parseVideoID(videoIDStr)

	// Track current video for cleanup
	s.currentVideoMu.Lock()
	s.currentVideo = videoID
	s.currentVideoMu.Unlock()

	// Verify video exists in database
	videos, err := s.db.GetAllVideos()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	var video *database.Video
	for _, v := range videos {
		if v.ID == videoID {
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

	// Try to serve directly from local disk first (faster and more reliable)
	if s.token != "" {
		// Construct expected local path: /var/lib/telegram-bot-api/<TOKEN>/<path_from_db>
		// The path in DB is relative to the token directory (e.g., "documents/file.mp4")
		localPath := filepath.Join("/var/lib/telegram-bot-api", s.token, video.TelegramFilePath)

		log.Printf("Checking for local file at: %s", localPath)
		if _, err := os.Stat(localPath); err == nil {
			log.Printf("Serving video %d directly from local disk: %s", videoID, localPath)
			// Check if it needs transcoding for browser compatibility (check codecs, not just extension)
			if needsTranscodingByCodec(localPath) {
				log.Printf("File requires transcoding for browser compatibility (incompatible audio/video codec): %s", localPath)
				s.transcodeAndServe(w, r, localPath, videoID)
				return
			}
			http.ServeFile(w, r, localPath)
			return
		}
		log.Printf("Local file not found at %s (cleaned from cache), will re-download from Telegram", localPath)
	}

	// File not in cache - need to re-download from Telegram
	// Download to temporary file first, then stream it
	if s.downloader != nil {
		log.Printf("Re-downloading video %d from Telegram (not in cache)", videoID)

		// Create temporary file
		tmpFile, err := os.CreateTemp(s.downloadDir, fmt.Sprintf("stream-%d-*.mp4", videoID))
		if err != nil {
			log.Printf("Error creating temp file for video %d: %v", videoID, err)
			http.Error(w, "Failed to create temporary file", http.StatusInternalServerError)
			return
		}
		tmpPath := tmpFile.Name()
		tmpFile.Close()
		defer os.Remove(tmpPath) // Clean up after streaming

		// Download file from Telegram
		err = s.downloader.DownloadFileWithPath(video.TelegramFileID, video.TelegramFilePath, tmpPath)
		if err != nil {
			log.Printf("Error re-downloading video %d from Telegram: %v", videoID, err)
			http.Error(w, fmt.Sprintf("Failed to download video from Telegram: %v", err), http.StatusInternalServerError)
			return
		}

		log.Printf("Successfully re-downloaded video %d to %s, now streaming", videoID, tmpPath)
		// Check if it needs transcoding for browser compatibility (check codecs, not just extension)
		if needsTranscodingByCodec(tmpPath) {
			log.Printf("File requires transcoding for browser compatibility (incompatible audio/video codec): %s", tmpPath)
			s.transcodeAndServe(w, r, tmpPath, videoID)
			return
		}
		http.ServeFile(w, r, tmpPath)
		return
	}

	// No downloader configured - fall back to trtg proxy as last resort
	targetURL := fmt.Sprintf("%s/download/%d", strings.TrimSuffix(s.trtgAPIURL, "/"), videoID)
	log.Printf("No downloader configured, proxying stream request for video %d to trtg: %s", videoID, targetURL)

	// Create request
	req, err := http.NewRequest(r.Method, targetURL, nil)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to create request: %v", err), http.StatusInternalServerError)
		return
	}

	// Copy headers (especially Range header for video seeking)
	for key, values := range r.Header {
		for _, value := range values {
			req.Header.Add(key, value)
		}
	}

	// Make request (no timeout for streaming)
	client := &http.Client{Timeout: 0}
	resp, err := client.Do(req)
	if err != nil {
		log.Printf("Error proxying video %d: %v", videoID, err)
		http.Error(w, fmt.Sprintf("Failed to stream video: %v", err), http.StatusInternalServerError)
		return
	}
	defer resp.Body.Close()

	// Copy response headers
	for key, values := range resp.Header {
		for _, value := range values {
			w.Header().Add(key, value)
		}
	}

	// Set status code
	w.WriteHeader(resp.StatusCode)

	// Stream response body
	if _, err := io.Copy(w, resp.Body); err != nil {
		log.Printf("Error streaming response for video %d: %v", videoID, err)
	}
}

// handleAPIStatus checks if a video is ready for streaming (trtg handles downloads on-demand)
func (s *Server) handleAPIStatus(w http.ResponseWriter, r *http.Request) {
	videoIDStr := strings.TrimPrefix(r.URL.Path, "/api/status/")
	if videoIDStr == "" {
		json.NewEncoder(w).Encode(map[string]interface{}{"error": "Video ID required"})
		return
	}

	videoID := parseVideoID(videoIDStr)

	// Get video from database
	videos, err := s.db.GetAllVideos()
	if err != nil {
		json.NewEncoder(w).Encode(map[string]interface{}{"error": err.Error()})
		return
	}

	var video *database.Video
	for _, v := range videos {
		if v.ID == videoID {
			video = &v
			break
		}
	}

	if video == nil {
		json.NewEncoder(w).Encode(map[string]interface{}{"error": "Video not found"})
		return
	}

	if video.TelegramFileID == "" {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"status": "error",
			"ready":  false,
			"error":  "Telegram file ID not available",
		})
		return
	}

	// trtg handles downloads on-demand, so we can always return ready
	// The actual download will happen when streaming starts
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status": "ready",
		"ready":  true,
	})
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

// needsTranscodingByCodec checks if a video file needs transcoding by inspecting actual codecs
// This is more accurate than just checking file extensions, as MP4 files can contain
// incompatible codecs (like AC3 audio which most browsers don't support)
func needsTranscodingByCodec(filePath string) bool {
	// Run ffprobe to check video and audio codecs
	cmd := exec.Command("ffprobe",
		"-v", "error",
		"-select_streams", "v:0", // First video stream
		"-show_entries", "stream=codec_name",
		"-of", "default=noprint_wrappers=1:nokey=1",
		filePath,
	)
	videoCodecOut, err := cmd.Output()
	if err != nil {
		log.Printf("Warning: Failed to detect video codec for %s: %v, assuming needs transcoding", filePath, err)
		return true // Safe default if we can't detect
	}
	videoCodec := strings.TrimSpace(string(videoCodecOut))

	// Check audio codec
	cmd = exec.Command("ffprobe",
		"-v", "error",
		"-select_streams", "a:0", // First audio stream
		"-show_entries", "stream=codec_name",
		"-of", "default=noprint_wrappers=1:nokey=1",
		filePath,
	)
	audioCodecOut, err := cmd.Output()
	if err != nil {
		log.Printf("Warning: Failed to detect audio codec for %s: %v, assuming needs transcoding", filePath, err)
		return true // Safe default if we can't detect
	}
	audioCodec := strings.TrimSpace(string(audioCodecOut))

	log.Printf("Detected codecs for %s: video=%s, audio=%s", filepath.Base(filePath), videoCodec, audioCodec)

	// Check if video codec is browser-compatible
	// h264 (AVC), vp8, vp9, av1 are widely supported
	videoCompatible := false
	switch videoCodec {
	case "h264", "vp8", "vp9", "av1":
		videoCompatible = true
	}

	// Check if audio codec is browser-compatible
	// aac, mp3, opus, vorbis are widely supported
	// ac3 (Dolby Digital), dts, truehd, eac3 are NOT supported by most browsers
	audioCompatible := false
	switch audioCodec {
	case "aac", "mp3", "opus", "vorbis":
		audioCompatible = true
	case "ac3", "eac3", "dts", "truehd", "dts-hd":
		log.Printf("Incompatible audio codec detected: %s (not supported by browsers)", audioCodec)
		audioCompatible = false
	}

	// Need transcoding if either codec is incompatible
	needsTranscode := !videoCompatible || !audioCompatible
	if needsTranscode {
		log.Printf("File needs transcoding: video_compatible=%v, audio_compatible=%v", videoCompatible, audioCompatible)
	}
	return needsTranscode
}

// transcodeAndServe transcodes video files to MP4 and caches the result on disk
func (s *Server) transcodeAndServe(w http.ResponseWriter, r *http.Request, inputPath string, videoID int64) {
	// Check if transcoded version already exists in cache
	cachedPath := filepath.Join(s.downloadDir, fmt.Sprintf("transcoded-%d.mp4", videoID))

	if _, err := os.Stat(cachedPath); err == nil {
		log.Printf("Serving cached transcoded video %d from: %s", videoID, cachedPath)
		http.ServeFile(w, r, cachedPath)
		return
	}

	// Transcode to cache file
	log.Printf("Transcoding video %d to browser-compatible MP4: %s -> %s", videoID, inputPath, cachedPath)

	// Use ffmpeg to transcode to browser-compatible MP4 (H.264 video + AAC audio)
	// -c:v libx264: H.264 video codec (universally supported)
	// -preset veryfast: Fast encoding with reasonable quality
	// -crf 23: Constant quality (lower = better quality, 23 is good balance)
	// -map 0:v:0: Map first video stream
	// -map 0:a: Map ALL audio streams (important for multi-audio videos)
	// -c:a aac: AAC audio codec (universally supported)
	// -b:a 128k: 128kbps audio bitrate
	// -ac 2: Force stereo output (browser compatible)
	// -movflags +faststart: Enable streaming before full download
	// -max_muxing_queue_size 1024: Handle high bitrate streams
	cmd := exec.Command("ffmpeg",
		"-i", inputPath,
		"-map", "0:v:0", // Explicitly map first video stream
		"-map", "0:a",   // Map all audio streams
		"-c:v", "libx264",
		"-preset", "veryfast",
		"-crf", "23",
		"-c:a", "aac",
		"-b:a", "128k",
		"-ac", "2", // Force stereo output
		"-movflags", "+faststart",
		"-max_muxing_queue_size", "1024", // Handle complex files better
		"-y",
		cachedPath,
	)

	// Capture stderr for logging
	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	// Run ffmpeg and wait for completion
	if err := cmd.Run(); err != nil {
		log.Printf("ffmpeg error for video %d: %v\nStderr: %s", videoID, err, stderr.String())
		http.Error(w, "Failed to transcode video", http.StatusInternalServerError)
		return
	}

	log.Printf("Successfully transcoded video %d to cache", videoID)

	// Serve the transcoded file
	http.ServeFile(w, r, cachedPath)
}
