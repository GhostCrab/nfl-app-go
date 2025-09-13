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

	// Debug week 16 2023
	season := 2023
	week := 16

	fmt.Printf("=== DEBUG WEEK %d SEASON %d ===\n", week, season)

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

	fmt.Printf("\nGames in week %d:\n", week)
	for _, game := range games {
		fmt.Printf("Game %d: %s vs %s on %s, State: %s\n", 
			game.ID, game.Away, game.Home, game.GetGameDayName(), game.State)
	}

	// For each user, show detailed breakdown
	for _, userID := range userIDs {
		fmt.Printf("\n=== USER %d ===\n", userID)
		
		// Get user's picks for this week
		picks, err := pickRepo.GetUserPicksForWeek(ctx, userID, season, week)
		if err != nil {
			log.Printf("Failed to get picks for user %d: %v", userID, err)
			continue
		}

		fmt.Printf("User %d has %d picks:\n", userID, len(picks))
		
		// Show all picks with results
		winCount := 0
		lossCount := 0
		pushCount := 0
		pendingCount := 0
		
		// Group picks by day for parlay analysis
		picksByDay := make(map[string][]models.Pick)
		
		for _, pick := range picks {
			// Find the game
			var game *models.Game
			for _, g := range games {
				if g.ID == pick.GameID {
					game = g
					break
				}
			}
			
			gameDay := "unknown"
			if game != nil {
				gameDay = game.GetGameDayName()
			}
			
			picksByDay[gameDay] = append(picksByDay[gameDay], pick)
			
			fmt.Printf("  Pick: Game %d (Team %d) on %s - Result: %s\n", 
				pick.GameID, pick.TeamID, gameDay, pick.Result)
			
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

		fmt.Printf("  Summary: %d wins, %d losses, %d pushes, %d pending\n", 
			winCount, lossCount, pushCount, pendingCount)

		// Analyze parlay scoring by day
		fmt.Printf("  Parlay analysis by day:\n")
		for dayName, dayPicks := range picksByDay {
			allWins := true
			for _, pick := range dayPicks {
				if pick.Result != models.PickResultWin {
					allWins = false
					break
				}
			}
			
			expectedPoints := 0
			if allWins {
				expectedPoints = len(dayPicks)
			}
			
			fmt.Printf("    %s: %d picks, all wins: %t, expected points: %d\n", 
				dayName, len(dayPicks), allWins, expectedPoints)
		}

		// Calculate parlay scores using service
		scores, err := parlayService.CalculateUserParlayScore(ctx, userID, season, week)
		if err != nil {
			log.Printf("Failed to calculate parlay scores for user %d: %v", userID, err)
			continue
		}

		totalParlayPoints := 0
		fmt.Printf("  Calculated parlay scores:\n")
		for category, points := range scores {
			totalParlayPoints += points
			fmt.Printf("    %s: %d points\n", category, points)
		}
		
		fmt.Printf("  Total calculated points: %d\n", totalParlayPoints)

		// Get stored parlay score from database
		storedScore, err := parlayRepo.GetUserParlayScore(ctx, userID, season, week)
		if err != nil {
			log.Printf("Failed to get stored score for user %d: %v", userID, err)
		} else {
			fmt.Printf("  Stored database score: %d\n", storedScore)
		}
	}
}