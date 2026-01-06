// Package main provides a tool to re-parse all videos in the database
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
	dryRun := flag.Bool("dry-run", false, "Show what would be updated without making changes")
	flag.Parse()

	// Get database URL from flag or environment
	connURL := *dbURL
	if connURL == "" {
		connURL = os.Getenv("DATABASE_URL")
	}
	if connURL == "" {
		log.Fatal("DATABASE_URL environment variable or -db flag is required")
	}

	// Connect to database
	db, err := database.New(connURL)
	if err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}
	defer db.Close()

	// Get all videos
	videos, err := db.GetAllVideos()
	if err != nil {
		log.Fatalf("Failed to get videos: %v", err)
	}

	fmt.Printf("Found %d videos in database\n", len(videos))
	if *dryRun {
		fmt.Println("DRY RUN MODE - no changes will be made")
	}
	fmt.Println()

	updatedCount := 0
	unchangedCount := 0
	seasonStats := make(map[int]int)

	for i, video := range videos {
		// Re-parse the video info
		info := parser.ParseVideoInfo(video.Title, video.FilePath)

		// Check if anything changed
		changed := info.ShowName != video.ShowName ||
			info.SeasonNumber != video.SeasonNumber ||
			info.EpisodeNumber != video.EpisodeNumber

		if changed {
			fmt.Printf("[%d/%d] UPDATE: %s\n", i+1, len(videos), video.FilePath)
			fmt.Printf("  Old: Show='%s', Season=%d, Episode=%d\n",
				video.ShowName, video.SeasonNumber, video.EpisodeNumber)
			fmt.Printf("  New: Show='%s', Season=%d, Episode=%d\n",
				info.ShowName, info.SeasonNumber, info.EpisodeNumber)

			if !*dryRun {
				err := db.UpdateVideoInfo(video.ID, info.ShowName, info.SeasonNumber, info.EpisodeNumber)
				if err != nil {
					log.Printf("  ERROR: Failed to update: %v", err)
					continue
				}
				fmt.Println("  ✓ Updated")
			} else {
				fmt.Println("  (would update)")
			}
			updatedCount++
		} else {
			unchangedCount++
		}

		seasonStats[info.SeasonNumber]++
	}

	fmt.Println()
	fmt.Println("=== Summary ===")
	fmt.Printf("Total videos: %d\n", len(videos))
	fmt.Printf("Updated: %d\n", updatedCount)
	fmt.Printf("Unchanged: %d\n", unchangedCount)
	fmt.Println()

	fmt.Println("=== Season Distribution (after re-parsing) ===")
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

	if *dryRun {
		fmt.Println()
		fmt.Println("NOTE: This was a dry run. Run without -dry-run to apply changes.")
	}
}
