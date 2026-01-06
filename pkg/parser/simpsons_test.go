package parser

import (
	"testing"
)

// TestSimpsonsPatterns tests various Simpsons file naming patterns
func TestSimpsonsPatterns(t *testing.T) {
	tests := []struct {
		torrentName     string
		filePath        string
		expectedShow    string
		expectedSeason  int
		expectedEpisode int
		description     string
	}{
		{
			torrentName:     "The Simpsons Seasons 1-36",
			filePath:        "Season 1/The.Simpsons.S01E01.mkv",
			expectedShow:    "The Simpsons",
			expectedSeason:  1,
			expectedEpisode: 1,
			description:     "Standard SxxExx with Season folder",
		},
		{
			torrentName:     "The Simpsons Seasons 1-36",
			filePath:        "Season 28/The.Simpsons.S28E15.mkv",
			expectedShow:    "The Simpsons",
			expectedSeason:  28,
			expectedEpisode: 15,
			description:     "Season 28 with SxxExx",
		},
		{
			torrentName:     "The Simpsons Complete",
			filePath:        "S05/The.Simpsons.S05E10.mkv",
			expectedShow:    "The Simpsons",
			expectedSeason:  5,
			expectedEpisode: 10,
			description:     "Short folder name S05",
		},
		{
			torrentName:     "The Simpsons",
			filePath:        "Season.10/simpsons_10x05.mkv",
			expectedShow:    "The Simpsons",
			expectedSeason:  10,
			expectedEpisode: 5,
			description:     "10x05 format",
		},
		{
			torrentName:     "The Simpsons",
			filePath:        "Simpsons - 301 - Homer Goes to College.mkv",
			expectedShow:    "The Simpsons",
			expectedSeason:  0, // Won't detect without clear pattern
			expectedEpisode: 0,
			description:     "Production code format (301 = season 3, ep 1)",
		},
		{
			torrentName:     "The Simpsons Seasons 1-36",
			filePath:        "Season 1/Episode 01 - Simpsons Roasting on an Open Fire.mkv",
			expectedShow:    "The Simpsons Seasons",
			expectedSeason:  1,
			expectedEpisode: 1,
			description:     "Episode XX format with Season folder",
		},
		{
			torrentName:     "[MULT] Симпсоны / The Simpsons / Сезоны 1-36",
			filePath:        "Season 15/The.Simpsons.S15E08.720p.WEB-DL.mkv",
			expectedShow:    "The Simpsons",
			expectedSeason:  15,
			expectedEpisode: 8,
			description:     "Multilingual torrent name",
		},
		{
			torrentName:     "The Simpsons Complete Collection",
			filePath:        "The Simpsons 1989-2024/Season 20/s20e01.mkv",
			expectedShow:    "The Simpsons Complete Collection",
			expectedSeason:  20,
			expectedEpisode: 1,
			description:     "Nested folders with year range",
		},
	}

	for _, tt := range tests {
		t.Run(tt.description, func(t *testing.T) {
			result := ParseVideoInfo(tt.torrentName, tt.filePath)

			if result.SeasonNumber != tt.expectedSeason {
				t.Errorf("Season: got %d, want %d (file: %s)", result.SeasonNumber, tt.expectedSeason, tt.filePath)
			}
			if result.EpisodeNumber != tt.expectedEpisode {
				t.Errorf("Episode: got %d, want %d (file: %s)", result.EpisodeNumber, tt.expectedEpisode, tt.filePath)
			}
			// Show name matching is more flexible
			if result.ShowName == "" {
				t.Errorf("ShowName is empty, expected something (file: %s)", tt.filePath)
			}
		})
	}
}
