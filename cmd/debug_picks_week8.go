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
	log.Println("=== DEBUG: Week 8 2023 Picks ===")
	
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
	pickService := services.NewPickService(pickRepo, gameRepo, userRepo)

	ctx := context.Background()
	
	// Test different weeks that are causing issues
	testWeeks := []int{8, 9, 10, 11}
	
	for _, week := range testWeeks {
		log.Printf("\n=== TESTING Week %d, 2023 ===", week)
		
		// Test raw repository call
		picks, err := pickRepo.FindByWeek(ctx, 2023, week)
		if err != nil {
			log.Printf("ERROR - Repository FindByWeek failed: %v", err)
			continue
		}
		log.Printf("Repository returned %d raw picks", len(picks))
		
		// Test service call
		userPicks, err := pickService.GetAllUserPicksForWeek(ctx, 2023, week)
		if err != nil {
			log.Printf("ERROR - Service GetAllUserPicksForWeek failed: %v", err)
			continue
		}
		log.Printf("Service returned %d user pick objects", len(userPicks))
		
		// Show sample data
		if len(picks) > 0 {
			log.Printf("Sample pick: UserID=%d, GameID=%d, TeamID=%d", picks[0].UserID, picks[0].GameID, picks[0].TeamID)
		}
		if len(userPicks) > 0 {
			log.Printf("Sample user picks: UserID=%d (%s) with %d picks", userPicks[0].UserID, userPicks[0].UserName, len(userPicks[0].Picks))
		}
	}
}