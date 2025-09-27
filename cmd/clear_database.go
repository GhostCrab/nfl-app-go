package main

import (
	"context"
	"log"
	"nfl-app-go/database"
	"os"

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
	log.Println("=== DATABASE CLEAR UTILITY ===")
	log.Println("WARNING: This will delete ALL data in the database!")
	
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

	ctx := context.Background()
	
	// Clear games collection
	log.Println("Clearing games collection...")
	gamesCollection := db.GetCollection("games")
	result, err := gamesCollection.DeleteMany(ctx, bson.M{})
	if err != nil {
		log.Fatalf("Failed to clear games collection: %v", err)
	}
	log.Printf("Deleted %d games", result.DeletedCount)
	
	// Clear weekly_picks collection
	log.Println("Clearing weekly_picks collection...")
	weeklyPicksCollection := db.GetCollection("weekly_picks")
	result, err = weeklyPicksCollection.DeleteMany(ctx, bson.M{})
	if err != nil {
		log.Fatalf("Failed to clear weekly_picks collection: %v", err)
	}
	log.Printf("Deleted %d weekly picks documents", result.DeletedCount)

	// Note: parlay_scores and weekly_scores collections removed
	// Parlay scores are now managed in-memory by MemoryParlayScorer
	
	// Note: We're keeping users collection intact as it contains login credentials
	log.Println("Users collection preserved (contains login credentials)")
	
	log.Println("\n=== DATABASE CLEARED ===")
	log.Println("All games and weekly picks have been removed from the database.")
	log.Println("Ready for fresh import from legacy data files.")
}