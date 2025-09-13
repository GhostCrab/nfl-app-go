package main

import (
	"context"
	"fmt"
	"log"
	"nfl-app-go/database"
	"nfl-app-go/models"
	"nfl-app-go/services"
	"os"

	"github.com/joho/godotenv"
)

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

func main() {
	// Load .env file
	if err := godotenv.Load(); err != nil {
		log.Printf("Warning: Could not load .env file: %v", err)
	}

	// Initialize MongoDB connection
	dbConfig := database.Config{
		Host:     getEnv("DB_HOST", "p5server"),
		Port:     getEnv("DB_PORT", "27017"),
		Username: getEnv("DB_USERNAME", "nflapp"),
		Password: getEnv("DB_PASSWORD", ""),
		Database: getEnv("DB_NAME", "nfl_app"),
	}
	
	db, err := database.NewMongoConnection(dbConfig)
	if err != nil {
		log.Fatalf("Failed to connect to MongoDB: %v", err)
	}
	defer db.Close()

	// Create repositories
	ctx := context.Background()

	pickRepo := database.NewMongoPickRepository(db)
	gameRepo := database.NewMongoGameRepository(db)
	parlayRepo := database.NewMongoParlayRepository(db)

	// Create parlay service
	parlayService := services.NewParlayService(pickRepo, gameRepo, parlayRepo)

	// Debug week 2 2025 Thursday
	season := 2025
	week := 2

	fmt.Printf("=== DEBUG WEEK %d SEASON %d THURSDAY ===\n", week, season)

	// Get all users with picks this week
	userIDs, err := pickRepo.GetUniqueUserIDsForWeek(ctx, season, week)
	if err != nil {
		log.Fatalf("Failed to get user IDs: %v", err)
	}

	fmt.Printf("Users with picks: %v\n", userIDs)

	// Get all games for the week
	games, err := gameRepo.GetGamesByWeekSeason(week, season)
	if err != nil {
		log.Fatalf("Failed to get games: %v", err)
	}

	// Find Thursday games and show them
	var thursdayGames []models.Game
	fmt.Printf("\nGames in week %d:\n", week)
	for _, game := range games {
		dayName := game.GetGameDayName()
		fmt.Printf("Game %d: %s vs %s on %s, State: %s\n", 
			game.ID, game.Away, game.Home, dayName, game.State)
		
		if dayName == "Thursday" {
			thursdayGames = append(thursdayGames, *game)
		}
	}

	fmt.Printf("\nFound %d Thursday games\n", len(thursdayGames))

	// User names for reference
	userNames := map[int]string{
		0: "Ryan", 1: "Andrew", 2: "Bardia", 3: "TJ", 4: "Micah", 5: "Brad", 6: "Grant",
	}

	// For each user, show detailed Thursday breakdown
	for _, userID := range userIDs {
		userName := userNames[userID]
		if userName == "" {
			userName = fmt.Sprintf("User%d", userID)
		}
		
		fmt.Printf("\n=== %s (User %d) ===\n", userName, userID)
		
		// Get user's picks for this week
		picks, err := pickRepo.GetUserPicksForWeek(ctx, userID, season, week)
		if err != nil {
			log.Printf("Failed to get picks for user %d: %v", userID, err)
			continue
		}

		// Filter for Thursday picks
		var thursdayPicks []models.Pick
		for _, pick := range picks {
			for _, game := range thursdayGames {
				if pick.GameID == game.ID {
					thursdayPicks = append(thursdayPicks, pick)
				}
			}
		}

		fmt.Printf("User %d has %d Thursday picks:\n", userID, len(thursdayPicks))
		
		if len(thursdayPicks) == 0 {
			fmt.Printf("  No Thursday picks\n")
			continue
		}

		// Show all Thursday picks with results
		winCount := 0
		lossCount := 0
		pushCount := 0
		pendingCount := 0
		
		for _, pick := range thursdayPicks {
			fmt.Printf("  Pick: Game %d (Team %d) - Result: %s\n", 
				pick.GameID, pick.TeamID, pick.Result)
			
			switch pick.Result {
			case models.PickResultWin:
				winCount++
			case models.PickResultLoss:
				lossCount++
			case models.PickResultPush:
				pushCount++
			case models.PickResultPending:
				pendingCount++
			}
		}

		fmt.Printf("  Thursday Summary: %d wins, %d losses, %d pushes, %d pending\n", 
			winCount, lossCount, pushCount, pendingCount)

		// Calculate expected points (must win ALL picks excluding pushes)
		expectedPoints := 0
		if lossCount == 0 && pendingCount == 0 && winCount > 0 {
			expectedPoints = winCount
		}
		fmt.Printf("  Expected Thursday Points: %d\n", expectedPoints)

		// Calculate parlay scores using service for this specific week
		scores, err := parlayService.CalculateUserParlayScore(ctx, userID, season, week)
		if err != nil {
			log.Printf("Failed to calculate parlay scores for user %d: %v", userID, err)
			continue
		}

		// Show Thursday category specifically
		thursdayPoints := scores[models.ParlayBonusThursday]
		fmt.Printf("  Calculated Thursday parlay score: %d\n", thursdayPoints)

		// Get stored parlay score from database
		storedScore, err := parlayRepo.GetUserParlayScore(ctx, userID, season, week)
		if err != nil {
			log.Printf("Failed to get stored score for user %d: %v", userID, err)
		} else {
			fmt.Printf("  Stored database score (total week): %d\n", storedScore)
		}
	}
}