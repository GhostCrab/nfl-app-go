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
	log.Println("=== TEST: Pick Enrichment ===")
	
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
	parlayRepo := database.NewMongoParlayRepository(db)
	pickService := services.NewPickService(pickRepo, gameRepo, userRepo, parlayRepo)

	ctx := context.Background()
	
	log.Println("Testing enriched pick service for 2023 Week 1...")
	userPicks, err := pickService.GetAllUserPicksForWeek(ctx, 2023, 1)
	if err != nil {
		log.Printf("Error: %v", err)
	} else {
		log.Printf("Got %d users", len(userPicks))
		for _, up := range userPicks {
			if len(up.Picks) > 0 {
				log.Printf("User %s:", up.UserName)
				for i, pick := range up.Picks {
					if i < 2 { // Show first 2 picks
						log.Printf("  Pick: %s", pick.PickDescription)
					}
				}
				break // Just show one user for testing
			}
		}
	}
}