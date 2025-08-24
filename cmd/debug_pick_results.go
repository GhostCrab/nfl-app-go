package main

import (
	"context"
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
	log.Println("=== DEBUG: Pick Results Calculation ===")
	
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

	// Create repositories and service
	pickRepo := database.NewMongoPickRepository(db)
	gameRepo := database.NewMongoGameRepository(db)
	userRepo := database.NewMongoUserRepository(db)
	_ = services.NewPickService(pickRepo, gameRepo, userRepo) // Not used in this debug

	ctx := context.Background()
	
	// Get a specific completed game to test with
	log.Println("Looking for a completed game...")
	games, err := gameRepo.GetGamesByWeekSeason(1, 2023)
	if err != nil {
		log.Printf("Error getting games: %v", err)
		return
	}
	
	var testGame *models.Game
	for i, game := range games {
		if game.State == models.GameStateCompleted && i < 3 { // Look at first few games
			testGame = game
			log.Printf("Found completed game: %s @ %s, Score: %d-%d, State: %s", 
				game.Away, game.Home, game.AwayScore, game.HomeScore, game.State)
			if game.Odds != nil {
				log.Printf("  Spread: %.1f, O/U: %.1f", game.Odds.Spread, game.Odds.OU)
			} else {
				log.Printf("  No odds data")
			}
			break
		}
	}
	
	if testGame == nil {
		log.Println("No completed games found")
		return
	}
	
	// Get picks for this game
	picks, err := pickRepo.FindByWeek(ctx, 2023, 1)
	if err != nil {
		log.Printf("Error getting picks: %v", err)
		return
	}
	
	log.Printf("Testing pick results for game %d...", testGame.ID)
	for _, pick := range picks {
		if pick.GameID == testGame.ID {
			log.Printf("Pick: User %d, Team %d, Type %s, Current Result: %s", 
				pick.UserID, pick.TeamID, pick.PickType, pick.Result)
			
			// Test manual calculation
			if pick.PickType == models.PickTypeSpread && testGame.Odds != nil {
				log.Printf("  Manual calculation debug:")
				log.Printf("    Game: %s @ %s, Score: %d-%d", testGame.Away, testGame.Home, testGame.AwayScore, testGame.HomeScore)
				log.Printf("    Spread: %.1f (negative means home favored)", testGame.Odds.Spread)
				log.Printf("    Point diff (home-away): %d", testGame.HomeScore - testGame.AwayScore)
			}
		}
	}
}