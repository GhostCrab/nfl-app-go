package main

import (
	"context"
	"log"
	"nfl-app-go/database"
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
	log.Println("=== DEBUG: CLE @ PIT Game Data ===")
	
	if err := godotenv.Load(); err != nil {
		log.Printf("Warning: Could not load .env file: %v", err)
	}

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

	gameRepo := database.NewMongoGameRepository(db)
	ctx := context.Background()
	
	// Look for CLE @ PIT game from 2024
	game, err := gameRepo.FindByESPNID(ctx, 401671631)
	if err != nil {
		log.Fatalf("Failed to find game: %v", err)
	}
	
	if game == nil {
		log.Println("Game 401671631 not found")
		return
	}
	
	log.Printf("Game: %s @ %s", game.Away, game.Home)
	log.Printf("State: %s", game.State)
	log.Printf("Scores: %s %d - %s %d", game.Away, game.AwayScore, game.Home, game.HomeScore)
	log.Printf("Odds: %+v", game.Odds)
	log.Printf("Week: %d, Season: %d", game.Week, game.Season)
	
	// Check a few more games from week 14, 2024
	log.Println("\n=== Checking other Week 14, 2024 games ===")
	games, err := gameRepo.GetGamesByWeekSeason(14, 2024)
	if err != nil {
		log.Printf("Failed to get week 14 games: %v", err)
		return
	}
	
	log.Printf("Found %d games for Week 14, 2024", len(games))
	for i, g := range games {
		if i >= 5 { // Just check first 5
			break
		}
		log.Printf("Game %d: %s @ %s, State: %s, Odds: %+v", g.ID, g.Away, g.Home, g.State, g.Odds)
	}
}