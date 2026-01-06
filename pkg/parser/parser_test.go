package parser

import (
	"testing"
)

func TestParseVideoInfo(t *testing.T) {
	tests := []struct {
		torrentName     string
		filePath        string
		expectedShow    string
		expectedSeason  int
		expectedEpisode int
	}{
		{
			torrentName:     "Breaking Bad Season 1",
			filePath:        "Breaking.Bad.S01E01.720p.WEB-DL.mkv",
			expectedShow:    "Breaking Bad",
			expectedSeason:  1,
			expectedEpisode: 1,
		},
		{
			torrentName:     "Game of Thrones",
			filePath:        "Season 3/Game.of.Thrones.S03E05.1080p.mkv",
			expectedShow:    "Game of Thrones",
			expectedSeason:  3,
			expectedEpisode: 5,
		},
		{
			torrentName:     "The Office",
			filePath:        "The.Office.s02e10.HDTV.x264.mp4",
			expectedShow:    "The Office",
			expectedSeason:  2,
			expectedEpisode: 10,
		},
		{
			torrentName:     "Stranger Things S01",
			filePath:        "Season 1/Episode 01.mkv",
			expectedShow:    "Stranger Things",
			expectedSeason:  1,
			expectedEpisode: 1,
		},
		{
			torrentName:     "Movie Collection",
			filePath:        "some.movie.2020.1080p.mkv",
			expectedShow:    "Movie Collection",
			expectedSeason:  0,
			expectedEpisode: 0,
		},
		{
			torrentName:     "The Office",
			filePath:        "Season 2/Faces of Scranton.mkv",
			expectedShow:    "The Office",
			expectedSeason:  0, // Should be marked as extra
			expectedEpisode: 0,
		},
		{
			torrentName:     "The Office",
			filePath:        "Season 2/Deleted Scenes/Scene 01.mkv",
			expectedShow:    "The Office",
			expectedSeason:  0, // Should be marked as extra
			expectedEpisode: 0,
		},
		{
			torrentName:     "Random Show",
			filePath:        "Random Video File.mkv",
			expectedShow:    "Random Show",
			expectedSeason:  0, // No season info, treat as extra
			expectedEpisode: 0,
		},
		{
			torrentName:     "The Office",
			filePath:        "Season 3/Bloopers/Season 3 Bloopers.mkv",
			expectedShow:    "The Office",
			expectedSeason:  0, // Bloopers folder, treat as extra
			expectedEpisode: 0,
		},
		{
			torrentName:     "Parks and Rec",
			filePath:        "Season 2/Blooper Reel.mkv",
			expectedShow:    "Parks and Rec",
			expectedSeason:  0, // Blooper in filename, treat as extra
			expectedEpisode: 0,
		},
		{
			torrentName:     "Community",
			filePath:        "Season 1/Gag Reel/Community S01 Gag Reel.mkv",
			expectedShow:    "Community",
			expectedSeason:  0, // Gag Reel folder, treat as extra
			expectedEpisode: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.filePath, func(t *testing.T) {
			result := ParseVideoInfo(tt.torrentName, tt.filePath)

			if result.ShowName != tt.expectedShow {
				t.Errorf("ShowName = %q, want %q", result.ShowName, tt.expectedShow)
			}
			if result.SeasonNumber != tt.expectedSeason {
				t.Errorf("SeasonNumber = %d, want %d", result.SeasonNumber, tt.expectedSeason)
			}
			if result.EpisodeNumber != tt.expectedEpisode {
				t.Errorf("EpisodeNumber = %d, want %d", result.EpisodeNumber, tt.expectedEpisode)
			}
		})
	}
}

func TestCleanShowName(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{
			input:    "Breaking.Bad.S01E01.720p.WEB-DL.x264",
			expected: "Breaking Bad",
		},
		{
			input:    "Game_of_Thrones_Season_3_1080p",
			expected: "Game of Thrones",
		},
		{
			input:    "The.Office.(2005).720p.BluRay",
			expected: "The Office",
		},
		{
			input:    "Stranger-Things-S01-HEVC-x265",
			expected: "Stranger Things",
		},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := cleanShowName(tt.input)
			if result != tt.expected {
				t.Errorf("cleanShowName(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}
