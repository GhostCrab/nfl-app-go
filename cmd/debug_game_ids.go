package main

import (
	"context"
	"log"
	"nfl-app-go/database"
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
	log.Println("=== DEBUG: Game ID Analysis ===")
	
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
	pickRepo := database.NewMongoPickRepository(db)
	gameRepo := database.NewMongoGameRepository(db)
	userRepo := database.NewMongoUserRepository(db)
	
	// Create pick service
	pickService := services.NewPickService(pickRepo, gameRepo, userRepo)

	ctx := context.Background()
	
	// Check games for 2023 Week 1
	log.Println("=== GAMES for 2023 Week 1 ===")
	games, err := gameRepo.GetGamesByWeekSeason(1, 2023)
	if err != nil {
		log.Printf("Error getting games: %v", err)
	} else {
		log.Printf("Found %d games for 2023 Week 1:", len(games))
		for _, game := range games {
			log.Printf("  Game ID: %d, %s @ %s, Date: %v", 
				game.ID, game.Away, game.Home, game.Date.Format("2006-01-02"))
		}
	}
	
	// Check picks for 2023 Week 1
	log.Println("\n=== PICKS for 2023 Week 1 ===")
	picks, err := pickRepo.FindByWeek(ctx, 2023, 1)
	if err != nil {
		log.Printf("Error getting picks: %v", err)
	} else {
		log.Printf("Found %d picks for 2023 Week 1:", len(picks))
		gameIdMap := make(map[int]int) // Count how many times each game ID appears
		for _, pick := range picks {
			gameIdMap[pick.GameID]++
		}
		log.Printf("Game IDs in picks:")
		for gameId, count := range gameIdMap {
			log.Printf("  Game ID: %d (appears %d times)", gameId, count)
		}
	}
	
	// Test the service to see what it returns
	log.Println("\n=== PICK SERVICE RESULT ===")
	userPicks, err := pickService.GetAllUserPicksForWeek(ctx, 2023, 1)
	if err != nil {
		log.Printf("Error from pick service: %v", err)
	} else {
		log.Printf("Pick service returned %d users", len(userPicks))
		for _, up := range userPicks {
			log.Printf("User %s: %d picks", up.UserName, len(up.Picks))
			for i, pick := range up.Picks {
				if i < 3 { // Show first 3 picks
					log.Printf("  Pick %d: Game %d, Team %d, Type %s, Result %s", 
						i+1, pick.GameID, pick.TeamID, pick.PickType, pick.Result)
				}
			}
		}
	}
}