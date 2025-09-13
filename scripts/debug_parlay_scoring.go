package main

import (
	"context"
	"fmt"
	"log"
	"nfl-app-go/database"
	"nfl-app-go/models"
	"nfl-app-go/services"
	"os"
	"sort"
	"time"

	"github.com/joho/godotenv"
	"go.mongodb.org/mongo-driver/bson"
)

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

func main() {
	log.Println("=== PARLAY SCORING DEBUG ===")

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

	if err := db.TestConnection(); err != nil {
		log.Fatalf("MongoDB connection test failed: %v", err)
	}

	ctx := context.Background()

	// Create repositories
	gameRepo := database.NewMongoGameRepository(db)
	pickRepo := database.NewMongoPickRepository(db)
	parlayRepo := database.NewMongoParlayRepository(db)

	// Debug specific season/week - let's look at 2024 Week 1
	season := 2024
	week := 1

	log.Printf("\n=== DEBUGGING SEASON %d WEEK %d ===", season, week)

	// Get all games for this week
	games, err := gameRepo.GetByWeek(ctx, season, week)
	if err != nil {
		log.Fatalf("Failed to get games: %v", err)
	}

	log.Printf("Found %d games for Week %d", len(games), week)

	// Create game map for easy lookup
	gameMap := make(map[int]models.Game)
	for _, game := range games {
		gameMap[game.ID] = game
	}

	// Print game details with day categorization
	log.Println("\n--- GAMES AND CATEGORIZATION ---")
	for _, game := range games {
		dayName := game.GetGameDayName()
		category := models.ParlayCategory("")
		switch dayName {
		case "Thursday":
			category = models.ParlayThursday
		case "Friday":
			category = models.ParlayFriday
		default:
			category = models.ParlaySunMon
		}
		
		// Get game status and winner
		status := "Unknown"
		winner := "TBD"
		if game.HomeScore != nil && game.AwayScore != nil {
			if *game.HomeScore > *game.AwayScore {
				winner = fmt.Sprintf("Home (%d)", game.HomeTeamID)
			} else if *game.AwayScore > *game.HomeScore {
				winner = fmt.Sprintf("Away (%d)", game.AwayTeamID)
			} else {
				winner = "Tie"
			}
			status = "Final"
		}

		log.Printf("Game %d: %s (%s) - %s vs %s - Winner: %s", 
			game.ID, game.Date.Format("Jan 2 15:04"), category, 
			fmt.Sprintf("%d", game.AwayTeamID), fmt.Sprintf("%d", game.HomeTeamID), winner)
	}

	// Get all picks for this week
	allPicks, err := pickRepo.FindByWeek(ctx, season, week)
	if err != nil {
		log.Fatalf("Failed to get picks: %v", err)
	}

	log.Printf("\nFound %d picks for Week %d", len(allPicks), week)

	// Group picks by user
	userPicks := make(map[int][]models.Pick)
	for _, pick := range allPicks {
		userPicks[pick.UserID] = append(userPicks[pick.UserID], pick)
	}

	// Debug each user's picks and scoring
	log.Println("\n--- USER PICK ANALYSIS ---")
	for userID, picks := range userPicks {
		log.Printf("\nUser %d has %d picks:", userID, len(picks))
		
		// Sort picks by game ID for consistent output
		sort.Slice(picks, func(i, j int) bool {
			return picks[i].GameID < picks[j].GameID
		})

		// Categorize picks
		thursdayPicks := []models.Pick{}
		fridayPicks := []models.Pick{}
		sunmonPicks := []models.Pick{}

		for _, pick := range picks {
			game, exists := gameMap[pick.GameID]
			if !exists {
				log.Printf("  WARNING: Pick for unknown game %d", pick.GameID)
				continue
			}

			dayName := game.GetGameDayName()
			resultStr := string(pick.Result)
			if pick.Result == "" {
				resultStr = "PENDING"
			}

			log.Printf("  Game %d (%s): Team %d - %s", 
				pick.GameID, dayName, pick.TeamID, resultStr)

			switch dayName {
			case "Thursday":
				thursdayPicks = append(thursdayPicks, pick)
			case "Friday":
				fridayPicks = append(fridayPicks, pick)
			default:
				sunmonPicks = append(sunmonPicks, pick)
			}
		}

		// Calculate category scores manually
		log.Printf("  Thursday picks: %d", len(thursdayPicks))
		thursdayScore := calculateCategoryScore(thursdayPicks, "Thursday")
		log.Printf("  Thursday score: %d", thursdayScore)

		log.Printf("  Friday picks: %d", len(fridayPicks))
		fridayScore := calculateCategoryScore(fridayPicks, "Friday")
		log.Printf("  Friday score: %d", fridayScore)

		log.Printf("  Sunday/Monday picks: %d", len(sunmonPicks))
		sunmonScore := calculateCategoryScore(sunmonPicks, "Sunday/Monday")
		log.Printf("  Sunday/Monday score: %d", sunmonScore)

		totalScore := thursdayScore + fridayScore + sunmonScore
		log.Printf("  TOTAL SCORE: %d", totalScore)
	}

	// Also check what's in the parlay_scores collection
	log.Println("\n--- PARLAY SCORES COLLECTION ---")
	parlayScores, err := parlayRepo.GetWeekScores(ctx, season, week)
	if err != nil {
		log.Printf("Error getting parlay scores: %v", err)
	} else {
		log.Printf("Found %d parlay score records", len(parlayScores))
		for _, score := range parlayScores {
			log.Printf("User %d: Thursday=%d, Friday=%d, SunMon=%d, Total=%d", 
				score.UserID, score.ThursdayPoints, score.FridayPoints, 
				score.SundayMondayPoints, score.TotalPoints)
		}
	}

	log.Println("\n=== DEBUG COMPLETE ===")
}

func calculateCategoryScore(picks []models.Pick, category string) int {
	if len(picks) == 0 {
		return 0
	}

	// Check if all picks are winners
	allWinners := true
	winCount := 0
	lossCount := 0
	pendingCount := 0

	for _, pick := range picks {
		switch pick.Result {
		case models.PickResultWin:
			winCount++
		case models.PickResultLoss:
			lossCount++
			allWinners = false
		default:
			pendingCount++
			allWinners = false
		}
	}

	fmt.Printf("    %s: %d wins, %d losses, %d pending", category, winCount, lossCount, pendingCount)

	if !allWinners {
		return 0
	}

	// Calculate parlay payout based on number of picks
	return calculateParlayPayout(len(picks))
}

func calculateParlayPayout(numPicks int) int {
	switch numPicks {
	case 1:
		return 1
	case 2:
		return 3
	case 3:
		return 6
	case 4:
		return 10
	case 5:
		return 20
	case 6:
		return 40
	case 7:
		return 75
	case 8:
		return 150
	default:
		// For more than 8 picks, use exponential growth
		base := 150
		for i := 8; i < numPicks; i++ {
			base = base * 2
		}
		return base
	}
}