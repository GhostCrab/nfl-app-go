package main

import (
	"context"
	"log"
	"nfl-app-go/database"
	"nfl-app-go/services"
	"os"
	"path/filepath"
	"strings"

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
	log.Println("=== FULL DATABASE REFRESH ===")
	log.Println("This script will:")
	log.Println("  1. Clear all games and weekly_picks from MongoDB")
	log.Println("  2. Re-import all legacy data from JSON files")
	log.Println("  3. Calculate pick results for completed games")
	log.Println("  4. Recalculate all parlay scores")
	log.Println("  5. Verify the final results")
	log.Println()

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

	// Get current working directory
	cwd, err := os.Getwd()
	if err != nil {
		log.Fatalf("Failed to get current directory: %v", err)
	}

	// Path to legacy-dbs directory
	legacyDbsPath := filepath.Join(cwd, "legacy-dbs")

	// Check if legacy-dbs directory exists
	if _, err := os.Stat(legacyDbsPath); os.IsNotExist(err) {
		log.Fatalf("Legacy-dbs directory not found at: %s", legacyDbsPath)
	}

	// STEP 1: Clear existing data
	log.Println("\n" + strings.Repeat("=", 50))
	log.Println("STEP 1: CLEARING DATABASE")
	log.Println(strings.Repeat("=", 50))

	ctx := context.Background()

	// Clear games collection
	log.Println("Clearing games collection...")
	gamesCollection := db.GetCollection("games")
	result, err := gamesCollection.DeleteMany(ctx, bson.M{})
	if err != nil {
		log.Fatalf("Failed to clear games collection: %v", err)
	}
	log.Printf("✓ Deleted %d games", result.DeletedCount)

	// Clear weekly_picks collection
	log.Println("Clearing weekly_picks collection...")
	weeklyPicksCollection := db.GetCollection("weekly_picks")
	result, err = weeklyPicksCollection.DeleteMany(ctx, bson.M{})
	if err != nil {
		log.Fatalf("Failed to clear weekly_picks collection: %v", err)
	}
	log.Printf("✓ Deleted %d weekly picks documents", result.DeletedCount)


	log.Println("✓ Database cleared successfully")

	// STEP 2: Re-import legacy data
	log.Println("\n" + strings.Repeat("=", 50))
	log.Println("STEP 2: IMPORTING LEGACY DATA")
	log.Println(strings.Repeat("=", 50))

	// Create repositories for legacy import (now using WeeklyPicksRepository)
	gameRepo := database.NewMongoGameRepository(db)
	userRepo := database.NewMongoUserRepository(db)
	weeklyPicksRepo := database.NewMongoWeeklyPicksRepository(db)

	// Create pick service and result calculation service for proper pick enrichment
	pickService := services.NewPickService(weeklyPicksRepo, gameRepo, userRepo)
	resultCalcService := services.NewResultCalculationService(weeklyPicksRepo, gameRepo, pickService)

	// Create legacy import service with result calculation service for pick enrichment
	importService := services.NewLegacyImportService(gameRepo, userRepo, weeklyPicksRepo, resultCalcService)

	// Show what we're about to import
	log.Printf("Analyzing legacy data in: %s", legacyDbsPath)
	if err := importService.GetImportSummary(legacyDbsPath); err != nil {
		log.Fatalf("Failed to analyze legacy data: %v", err)
	}

	// Import all legacy data
	log.Println("Importing legacy data...")
	if err := importService.ImportAllLegacyData(legacyDbsPath); err != nil {
		log.Fatalf("Failed to import legacy data: %v", err)
	}
	log.Println("✓ Legacy data imported successfully with pick enrichment")

	// STEP 3: Final verification
	log.Println("\n" + strings.Repeat("=", 50))
	log.Println("STEP 3: VERIFICATION")
	log.Println(strings.Repeat("=", 50))

	// Count final totals
	gamesCount, err := gamesCollection.CountDocuments(ctx, bson.M{})
	if err != nil {
		log.Printf("Warning: couldn't count games: %v", err)
	} else {
		log.Printf("✓ Total games in database: %d", gamesCount)
	}

	weeklyPicksCount, err := weeklyPicksCollection.CountDocuments(ctx, bson.M{})
	if err != nil {
		log.Printf("Warning: couldn't count weekly picks: %v", err)
	} else {
		log.Printf("✓ Total weekly picks documents in database: %d", weeklyPicksCount)
	}


	// Final summary
	log.Println("\n" + strings.Repeat("=", 60))
	log.Println("FULL DATABASE REFRESH COMPLETE!")
	log.Println(strings.Repeat("=", 60))
	log.Println("Database is now fully refreshed with corrected pick result calculation!")
	log.Println("Note: Parlay scores are calculated in-memory on startup via MemoryParlayScorer")
	log.Println(strings.Repeat("=", 60))
}
