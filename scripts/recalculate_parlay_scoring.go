package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"strings"

	"nfl-app-go/database"
	"nfl-app-go/services"
	"github.com/joho/godotenv"
)

func main() {
	// Load .env file
	if err := godotenv.Load(); err != nil {
		log.Printf("Warning: Could not load .env file: %v", err)
	}

	// Get database config from environment
	getEnv := func(key, defaultValue string) string {
		if value := os.Getenv(key); value != "" {
			return value
		}
		return defaultValue
	}

	// Parse command line arguments
	if len(os.Args) < 2 {
		fmt.Println("Parlay Scoring Recalculator")
		fmt.Println("Usage:")
		fmt.Println("  go run scripts/recalculate_parlay_scoring.go all                    # Recalculate all seasons from 2023")
		fmt.Println("  go run scripts/recalculate_parlay_scoring.go <season>               # Recalculate entire season")
		fmt.Println("  go run scripts/recalculate_parlay_scoring.go <season> <week>        # Recalculate specific week")
		fmt.Println("")
		fmt.Println("Examples:")
		fmt.Println("  go run scripts/recalculate_parlay_scoring.go all")
		fmt.Println("  go run scripts/recalculate_parlay_scoring.go 2023")
		fmt.Println("  go run scripts/recalculate_parlay_scoring.go 2023 15")
		os.Exit(1)
	}

	// Initialize database connection
	dbConfig := database.Config{
		Host:     getEnv("DB_HOST", "p5server"),
		Port:     getEnv("DB_PORT", "27017"), 
		Username: getEnv("DB_USERNAME", "nflapp"),
		Password: getEnv("DB_PASSWORD", ""),
		Database: getEnv("DB_NAME", "nfl_app"),
	}
	
	db, err := database.NewMongoConnection(dbConfig)
	if err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}
	defer db.Close()

	// Initialize repositories
	gameRepo := database.NewMongoGameRepository(db)
	pickRepo := database.NewMongoPickRepository(db)
	userRepo := database.NewMongoUserRepository(db)
	parlayRepo := database.NewMongoParlayRepository(db)

	// Initialize services
	pickService := services.NewPickService(pickRepo, gameRepo, userRepo, parlayRepo)

	ctx := context.Background()

	// Handle different argument patterns
	arg1 := os.Args[1]
	
	if arg1 == "all" {
		// Recalculate all seasons from 2023
		recalculateAllSeasons(ctx, pickService, pickRepo, parlayRepo)
	} else {
		// Parse season
		var season int
		if _, err := fmt.Sscanf(arg1, "%d", &season); err != nil {
			log.Fatalf("Invalid season: %s", arg1)
		}
		
		if len(os.Args) == 3 {
			// Specific week
			var week int
			if _, err := fmt.Sscanf(os.Args[2], "%d", &week); err != nil {
				log.Fatalf("Invalid week: %s", os.Args[2])
			}
			recalculateWeek(ctx, pickService, pickRepo, parlayRepo, season, week)
		} else {
			// Entire season
			recalculateSeason(ctx, pickService, pickRepo, parlayRepo, season)
		}
	}
}

func recalculateAllSeasons(ctx context.Context, pickService *services.PickService, pickRepo *database.MongoPickRepository, parlayRepo *database.MongoParlayRepository) {
	fmt.Printf("Recalculating ALL parlay scores from 2023 onwards...\n")
	
	seasons := []int{2023, 2024, 2025}
	totalWeeksProcessed := 0
	totalPointsAwarded := 0

	for _, season := range seasons {
		fmt.Printf("\n=== SEASON %d ===\n", season)
		weekPoints := recalculateSeason(ctx, pickService, pickRepo, parlayRepo, season)
		totalWeeksProcessed += len(weekPoints)
		for _, points := range weekPoints {
			totalPointsAwarded += points
		}
	}

	fmt.Printf("\n" + strings.Repeat("=", 60) + "\n")
	fmt.Printf("RECALCULATION COMPLETE\n")
	fmt.Printf("Weeks processed: %d\n", totalWeeksProcessed)
	fmt.Printf("Total points awarded: %d\n", totalPointsAwarded)
	fmt.Printf(strings.Repeat("=", 60) + "\n")
}

func recalculateSeason(ctx context.Context, pickService *services.PickService, pickRepo *database.MongoPickRepository, parlayRepo *database.MongoParlayRepository, season int) []int {
	fmt.Printf("Recalculating Season %d...\n", season)
	
	var weekPoints []int
	
	// Process weeks 1-18 for the season
	for week := 1; week <= 18; week++ {
		points := recalculateWeek(ctx, pickService, pickRepo, parlayRepo, season, week)
		if points >= 0 { // Only add to results if week had picks
			weekPoints = append(weekPoints, points)
		}
	}
	
	totalPoints := 0
	for _, points := range weekPoints {
		totalPoints += points
	}
	
	fmt.Printf("Season %d complete: %d weeks processed, %d total points awarded\n", season, len(weekPoints), totalPoints)
	return weekPoints
}

func recalculateWeek(ctx context.Context, pickService *services.PickService, pickRepo *database.MongoPickRepository, parlayRepo *database.MongoParlayRepository, season, week int) int {
	fmt.Printf("Processing Season %d, Week %d...", season, week)
	
	// Check if there are any picks for this week first
	allPicks, err := pickRepo.FindByWeek(ctx, season, week)
	if err != nil {
		log.Printf("Error checking picks for S%d W%d: %v", season, week, err)
		return -1
	}
	
	if len(allPicks) == 0 {
		fmt.Printf(" no picks found, skipping\n")
		return -1
	}
	
	// Process parlay scoring
	if err := pickService.ProcessWeekParlayScoring(ctx, season, week); err != nil {
		log.Printf("Failed to process parlay scoring for S%d W%d: %v", season, week, err)
		return -1
	}
	
	// Get scores to show summary
	scores, err := parlayRepo.GetWeekScores(ctx, season, week)
	if err != nil {
		log.Printf("Warning: couldn't fetch scores for S%d W%d: %v", season, week, err)
		return -1
	}
	
	weekTotal := 0
	usersWithPoints := 0
	for _, score := range scores {
		weekTotal += score.TotalPoints
		if score.TotalPoints > 0 {
			usersWithPoints++
		}
	}
	
	fmt.Printf(" ✓ %d points awarded to %d users\n", weekTotal, usersWithPoints)
	return weekTotal
}