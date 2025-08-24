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
	log.Println("=== DEBUG: Team ID to Team Mapping ===")
	
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

	gameRepo := database.NewMongoGameRepository(db)
	ctx := context.Background()
	
	// Get game 401547353 (DET @ KC) from debug output
	game, err := gameRepo.FindByESPNID(ctx, 401547353)
	if err != nil {
		log.Printf("Error getting game: %v", err)
	} else {
		log.Printf("Game 401547353: %s @ %s", game.Away, game.Home)
		log.Printf("Away team: %s, Home team: %s", game.Away, game.Home)
	}
	
	// Check a few other games
	gameIDs := []int{401547403, 401547401, 401547396}
	for _, gameID := range gameIDs {
		game, err := gameRepo.FindByESPNID(ctx, gameID)
		if err != nil {
			log.Printf("Error getting game %d: %v", gameID, err)
		} else {
			log.Printf("Game %d: %s @ %s", gameID, game.Away, game.Home)
		}
	}
}