// Package main provides diagnostic tool for checking parser
package main

import (
	"flag"
	"fmt"
	"log"
	"os"

	"github.com/rusik69/trtg/pkg/database"
	"github.com/rusik69/trtg/pkg/parser"
)

func main() {
	dbURL := flag.String("db", "", "PostgreSQL connection URL (or use DATABASE_URL env)")
	sampleSize := flag.Int("samples", 20, "Number of uncategorized samples to show")
	flag.Parse()

	// Get database URL from flag or environment
	connURL := *dbURL
	if connURL == "" {
		connURL = os.Getenv("DATABASE_URL")
	}
	if connURL == "" {
		connURL = "postgres://trtg:trtg@127.0.0.1:5432/trtg?sslmode=disable"
	}

	db, err := database.New(connURL)
	if err != nil {
		log.Fatalf("Failed to initialize database: %v", err)
	}
	defer db.Close()

	videos, err := db.GetAllVideos()
	if err != nil {
		log.Fatalf("Failed to get videos: %v", err)
	}

	fmt.Printf("Total videos in database: %d\n\n", len(videos))

	// Group by season (current state)
	seasonStats := make(map[int]int)
	var uncategorized []database.Video

	for _, video := range videos {
		if video.SeasonNumber == 0 {
			uncategorized = append(uncategorized, video)
		}
		seasonStats[video.SeasonNumber]++
	}

	fmt.Println("=== Current Season Distribution ===")
	for season := 0; season <= 40; season++ {
		count := seasonStats[season]
		if count > 0 {
			label := fmt.Sprintf("Season %d", season)
			if season == 0 {
				label = "Uncategorized (Season 0)"
			}
			fmt.Printf("%s: %d episodes\n", label, count)
		}
	}

	fmt.Printf("\n=== Sample Uncategorized Files (showing %d) ===\n", *sampleSize)
	limit := *sampleSize
	if len(uncategorized) < limit {
		limit = len(uncategorized)
	}

	fixableCount := 0
	for i := 0; i < limit; i++ {
		video := uncategorized[i]
		fmt.Printf("\n[%d] File: %s\n", i+1, video.FilePath)
		fmt.Printf("    Title: %s\n", video.Title)
		fmt.Printf("    Current: Show='%s', Season=%d, Episode=%d\n",
			video.ShowName, video.SeasonNumber, video.EpisodeNumber)

		// Try parsing with improved parser
		info := parser.ParseVideoInfo(video.Title, video.FilePath)
		fmt.Printf("    Re-parsed: Show='%s', Season=%d, Episode=%d\n",
			info.ShowName, info.SeasonNumber, info.EpisodeNumber)

		if info.SeasonNumber > 0 {
			fmt.Printf("    ✓ Can be fixed!\n")
			fixableCount++
		} else {
			fmt.Printf("    ✗ Still uncategorized\n")
		}
	}

	fmt.Printf("\n=== Summary ===\n")
	fmt.Printf("Total uncategorized: %d\n", len(uncategorized))
	fmt.Printf("Fixable (from samples): %d/%d\n", fixableCount, limit)
	fmt.Printf("\nTo fix all videos, run: reparse -db=\"%s\"\n", connURL)
	fmt.Printf("To preview changes, run: reparse -db=\"%s\" -dry-run\n", connURL)
}
