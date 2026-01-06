// Package parser extracts show, season, and episode information from file paths and torrent names
package parser

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// VideoInfo contains parsed metadata from a file path
type VideoInfo struct {
	ShowName      string
	SeasonNumber  int
	EpisodeNumber int
}

var (
	// Common season/episode patterns
	// S01E01, S1E1, s01e01
	sXXeXXPattern = regexp.MustCompile(`(?i)[sS](\d{1,2})[eE](\d{1,3})`)

	// 10x05 format (SeasonxEpisode)
	seXepPattern = regexp.MustCompile(`(?i)(\d{1,2})x(\d{1,3})`)

	// Season 1, season.1, Season.01
	seasonPattern = regexp.MustCompile(`(?i)season[\s._-]*(\d{1,2})`)

	// Episode 1, episode.1, ep01, e01 (but not embedded in other numbers)
	episodePattern = regexp.MustCompile(`(?i)(?:episode|ep|e)[\s._-]+(\d{1,3})\b`)

	// Folder pattern: /Season 1/, /S01/
	folderSeasonPattern = regexp.MustCompile(`(?i)(?:season|s)[\s._-]*(\d{1,2})`)

	// Common quality/release tags to remove from show names
	qualityTags = regexp.MustCompile(`(?i)[\[\(]?((?:720|1080|2160)p?|web-?dl|bluray|brrip|webrip|hdtv|x264|x265|hevc|aac|ac3|5\.1|dts|h\.264|h\.265)[\]\)]?`)
)

// ParseVideoInfo extracts show name, season, and episode from file path and torrent name
func ParseVideoInfo(torrentName, filePath string) VideoInfo {
	info := VideoInfo{
		ShowName:      torrentName,
		SeasonNumber:  0, // Default to 0 (Specials/Unknown)
		EpisodeNumber: 0,
	}

	// Try to extract season/episode from file path first (more reliable)
	fileName := filepath.Base(filePath)
	dirPath := filepath.Dir(filePath)

	// Check if this is an Extra/Special by looking for:
	// - Special folders: /extras/, /specials/, /deleted scenes/, /bloopers/, /gag reel/
	// - Special keywords in filename: extra, special, bonus, deleted scene, blooper, gag reel, behind the scenes, featurette
	lowerPath := strings.ToLower(filePath)
	lowerFileName := strings.ToLower(fileName)

	// Known special episode titles that should be marked as extras
	knownSpecials := []string{
		"faces of scranton",
		"the accountants",
		"the mentor",
		"blackmail",
		"subtle sexuality",
		"the 3rd floor",
		"the podcast",
	}

	isKnownSpecial := false
	for _, special := range knownSpecials {
		if strings.Contains(lowerFileName, special) {
			isKnownSpecial = true
			break
		}
	}

	isExtra := isKnownSpecial ||
	           strings.Contains(lowerPath, "/extras/") ||
	           strings.Contains(lowerPath, "/specials/") ||
	           strings.Contains(lowerPath, "/deleted scenes/") ||
	           strings.Contains(lowerPath, "/bloopers/") ||
	           strings.Contains(lowerPath, "/blooper/") ||
	           strings.Contains(lowerPath, "/gag reel/") ||
	           strings.Contains(lowerFileName, "extra") ||
	           strings.Contains(lowerFileName, "special") ||
	           strings.Contains(lowerFileName, "bonus") ||
	           strings.Contains(lowerFileName, "deleted scene") ||
	           strings.Contains(lowerFileName, "blooper") ||
	           strings.Contains(lowerFileName, "gag reel") ||
	           strings.Contains(lowerFileName, "behind the scenes") ||
	           strings.Contains(lowerFileName, "featurette")

	if isExtra {
		// Mark as Season 0 (Specials/Extras)
		info.SeasonNumber = 0
		// Try to extract episode number from "Extra N" or "Special N" pattern
		extraPattern := regexp.MustCompile(`(?i)(?:extra|special|bonus)[s]?\s+(\d+)`)
		if matches := extraPattern.FindStringSubmatch(fileName); len(matches) >= 2 {
			if episode, err := strconv.Atoi(matches[1]); err == nil {
				info.EpisodeNumber = episode
			}
		}
		// If no episode number found, it will default to 0
	}

	// Try SxxExx pattern (most common) - but only if not already marked as extra
	if !isExtra {
		if matches := sXXeXXPattern.FindStringSubmatch(fileName); len(matches) >= 3 {
			if season, err := strconv.Atoi(matches[1]); err == nil {
				info.SeasonNumber = season
			}
			if episode, err := strconv.Atoi(matches[2]); err == nil {
				info.EpisodeNumber = episode
			}
		}
	}

	// Try to get season from immediate parent folder first (most reliable for multi-season packs)
	// Look for /Season X/ or /SXX/ pattern in the full path
	if !isExtra && info.SeasonNumber == 0 {
		immediateFolder := filepath.Base(dirPath)
		folderPattern := regexp.MustCompile(`^(?:season[\s._-]*|s)0*(\d{1,2})$`)
		if matches := folderPattern.FindStringSubmatch(strings.ToLower(immediateFolder)); len(matches) >= 2 {
			if season, err := strconv.Atoi(matches[1]); err == nil && season > 0 && season < 100 {
				info.SeasonNumber = season
			}
		}
	}

	// Try XxY format if SxxExx didn't match (e.g., 10x05) - skip for extras
	if !isExtra && info.EpisodeNumber == 0 {
		if matches := seXepPattern.FindStringSubmatch(fileName); len(matches) >= 3 {
			if season, err := strconv.Atoi(matches[1]); err == nil {
				// Sanity check: season should be reasonable (1-99)
				// Only set season if we don't already have one
				if info.SeasonNumber == 0 && season >= 1 && season <= 99 {
					info.SeasonNumber = season
				}
			}
			if episode, err := strconv.Atoi(matches[2]); err == nil {
				info.EpisodeNumber = episode
			}
		}
	}

	// If no season found, try folder structure in full path - skip for extras
	if !isExtra && info.SeasonNumber == 0 {
		if matches := folderSeasonPattern.FindStringSubmatch(dirPath); len(matches) >= 2 {
			if season, err := strconv.Atoi(matches[1]); err == nil {
				info.SeasonNumber = season
			}
		}
	}

	// If still no season, try "Season X" pattern in filename - skip for extras
	if !isExtra && info.SeasonNumber == 0 {
		if matches := seasonPattern.FindStringSubmatch(fileName); len(matches) >= 2 {
			if season, err := strconv.Atoi(matches[1]); err == nil {
				info.SeasonNumber = season
			}
		}
	}

	// If no episode found, try episode pattern (but avoid matching years like 2020) - skip for extras
	if !isExtra && info.EpisodeNumber == 0 {
		if matches := episodePattern.FindStringSubmatch(fileName); len(matches) >= 2 {
			if episode, err := strconv.Atoi(matches[1]); err == nil {
				// Ignore if it looks like a year (4 digits >= 1900)
				if episode < 1900 {
					info.EpisodeNumber = episode
				}
			}
		}
	}

	// If no season info was found, treat as extra/special
	// This catches files that don't have clear season numbering
	if info.SeasonNumber == 0 && !isExtra {
		// Mark as Season 0 (Specials/Extras) and reset episode to 0
		// since we can't reliably determine episode numbers without season context
		info.EpisodeNumber = 0
	}

	// Extract show name from torrent name using LLM for better parsing
	info.ShowName = extractShowNameWithLLM(torrentName, filePath)

	return info
}

// extractShowName cleans up the torrent name to get a proper show name
func extractShowName(torrentName, filePath string) string {
	showName := torrentName

	// Try to extract show name from file path if it looks better
	// Example: "Show.Name.S01E01.720p.WEB-DL.mkv" -> "Show Name"
	fileName := filepath.Base(filePath)

	// Remove extension
	fileName = strings.TrimSuffix(fileName, filepath.Ext(fileName))

	// Try to find where season info starts
	if idx := sXXeXXPattern.FindStringIndex(fileName); idx != nil {
		// Everything before SxxExx is likely the show name
		possibleShowName := fileName[:idx[0]]
		possibleShowName = cleanShowName(possibleShowName)
		if len(possibleShowName) > 3 { // Sanity check
			showName = possibleShowName
		}
	} else if idx := seasonPattern.FindStringIndex(fileName); idx != nil {
		possibleShowName := fileName[:idx[0]]
		possibleShowName = cleanShowName(possibleShowName)
		if len(possibleShowName) > 3 {
			showName = possibleShowName
		}
	}

	// If we couldn't extract from filename, clean the torrent name
	if showName == torrentName {
		showName = cleanShowName(torrentName)
	}

	return showName
}

// cleanShowName removes quality tags and cleans up the show name
func cleanShowName(name string) string {
	// Remove quality tags
	name = qualityTags.ReplaceAllString(name, "")

	// Remove season/episode info (S01E01, S01, etc.)
	name = sXXeXXPattern.ReplaceAllString(name, "")
	name = seasonPattern.ReplaceAllString(name, "")
	// Also remove standalone Sxx patterns (like S01, S1)
	name = regexp.MustCompile(`(?i)\bS\d{1,2}\b`).ReplaceAllString(name, "")

	// Replace dots, underscores, and dashes with spaces
	name = strings.ReplaceAll(name, ".", " ")
	name = strings.ReplaceAll(name, "_", " ")
	name = strings.ReplaceAll(name, "-", " ")

	// Remove multiple spaces
	name = regexp.MustCompile(`\s+`).ReplaceAllString(name, " ")

	// Remove common year patterns (both at the end and in parentheses anywhere)
	name = regexp.MustCompile(`\(\d{4}\)`).ReplaceAllString(name, "")
	name = regexp.MustCompile(`\s*\d{4}$`).ReplaceAllString(name, "")

	// Remove multiple spaces again after year removal
	name = regexp.MustCompile(`\s+`).ReplaceAllString(name, " ")

	// Trim spaces
	name = strings.TrimSpace(name)

	return name
}

// extractShowNameWithLLM uses Claude Haiku to intelligently extract show names
func extractShowNameWithLLM(torrentName, filePath string) string {
	// Try LLM extraction first if API key is available
	if apiKey := os.Getenv("ANTHROPIC_API_KEY"); apiKey != "" {
		if showName := callClaudeForShowName(torrentName, filePath, apiKey); showName != "" {
			return showName
		}
	}

	// Fall back to regex-based extraction
	return extractShowName(torrentName, filePath)
}

type claudeMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type claudeRequest struct {
	Model     string          `json:"model"`
	MaxTokens int             `json:"max_tokens"`
	Messages  []claudeMessage `json:"messages"`
}

type claudeResponse struct {
	Content []struct {
		Text string `json:"text"`
	} `json:"content"`
}

// callClaudeForShowName calls Claude Haiku API to extract clean show name
func callClaudeForShowName(torrentName, filePath, apiKey string) string {
	prompt := fmt.Sprintf(`Extract the TV show name from this torrent/file information. Return ONLY the clean show name without any metadata.

Torrent name: %s
File path: %s

Rules:
- Remove metadata like "5 Seasons", "Complete", "Season 1-5", etc.
- Remove quality info like "720p", "WEB-DL", "DVDRip", etc.
- Remove year if present
- Return just the show name, properly capitalized
- If it's "Futurama 5 Seasons", return "Futurama"
- If it's "The Simpsons Complete", return "The Simpsons"

Return ONLY the show name, nothing else:`, torrentName, filePath)

	reqBody := claudeRequest{
		Model:     "claude-haiku-4-20250129",
		MaxTokens: 50,
		Messages: []claudeMessage{
			{
				Role:    "user",
				Content: prompt,
			},
		},
	}

	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return ""
	}

	req, err := http.NewRequest("POST", "https://api.anthropic.com/v1/messages", bytes.NewBuffer(jsonData))
	if err != nil {
		return ""
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-api-key", apiKey)
	req.Header.Set("anthropic-version", "2023-06-01")

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return ""
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return ""
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return ""
	}

	var claudeResp claudeResponse
	if err := json.Unmarshal(body, &claudeResp); err != nil {
		return ""
	}

	if len(claudeResp.Content) > 0 {
		showName := strings.TrimSpace(claudeResp.Content[0].Text)
		// Basic validation - make sure it's not empty and not too long
		if len(showName) > 0 && len(showName) < 100 {
			return showName
		}
	}

	return ""
}
