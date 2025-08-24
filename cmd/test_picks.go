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
	log.Println("=== NFL Picks Database Test ===")
	
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
	
	log.Printf("Connecting to MongoDB: %s:%s/%s", dbConfig.Host, dbConfig.Port, dbConfig.Database)
	
	db, err := database.NewMongoConnection(dbConfig)
	if err != nil {
		log.Fatalf("Failed to connect to MongoDB: %v", err)
	}
	defer db.Close()

	// Test the connection
	if err := db.TestConnection(); err != nil {
		log.Fatalf("MongoDB connection test failed: %v", err)
	}
	
	log.Println("MongoDB connection successful")

	// Create repositories
	pickRepo := database.NewMongoPickRepository(db)
	gameRepo := database.NewMongoGameRepository(db)
	userRepo := database.NewMongoUserRepository(db)
	
	// Create pick service
	pickService := services.NewPickService(pickRepo, gameRepo, userRepo)

	ctx := context.Background()
	
	// Test 1: Check total picks count
	totalPicks, err := pickRepo.Count(ctx)
	if err != nil {
		log.Printf("Error counting picks: %v", err)
	} else {
		log.Printf("Total picks in database: %d", totalPicks)
	}
	
	// Test 2: Check picks by season
	seasons := []int{2023, 2024, 2025}
	for _, season := range seasons {
		picks, err := pickRepo.FindBySeason(ctx, season)
		if err != nil {
			log.Printf("Error finding picks for season %d: %v", season, err)
		} else {
			log.Printf("Season %d: %d picks found", season, len(picks))
			if len(picks) > 0 {
				log.Printf("  Sample pick: User %d, Game %d, Team %d, Type %s, Week %d", 
					picks[0].UserID, picks[0].GameID, picks[0].TeamID, picks[0].PickType, picks[0].Week)
			}
		}
	}
	
	// Test 3: Check users in database
	users, err := userRepo.GetAllUsers()
	if err != nil {
		log.Printf("Error getting users: %v", err)
	} else {
		log.Printf("Users in database: %d", len(users))
		for _, user := range users {
			log.Printf("  User ID %d: %s (%s)", user.ID, user.Name, user.Email)
		}
	}
	
	// Test 4: Check specific week picks (2023 week 1)
	log.Println("Testing specific week: 2023, Week 1")
	userPicks, err := pickService.GetAllUserPicksForWeek(ctx, 2023, 1)
	if err != nil {
		log.Printf("Error getting week picks: %v", err)
	} else {
		log.Printf("Week picks found: %d users", len(userPicks))
		for _, up := range userPicks {
			log.Printf("  User %s (ID %d): %d picks, Record: %s", up.UserName, up.UserID, len(up.Picks), up.Record.String())
		}
	}
	
	// Test 5: Check games for week 1 of 2023
	games, err := gameRepo.GetGamesByWeekSeason(1, 2023)
	if err != nil {
		log.Printf("Error getting games: %v", err)
	} else {
		log.Printf("Games for 2023 Week 1: %d", len(games))
	}
}