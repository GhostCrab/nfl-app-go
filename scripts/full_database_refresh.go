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
	log.Println("  1. Clear all games and picks from MongoDB")
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
	
	// Clear picks collection
	log.Println("Clearing picks collection...")
	picksCollection := db.GetCollection("picks")
	result, err = picksCollection.DeleteMany(ctx, bson.M{})
	if err != nil {
		log.Fatalf("Failed to clear picks collection: %v", err)
	}
	log.Printf("✓ Deleted %d picks", result.DeletedCount)
	
	// Clear parlay scores collection
	log.Println("Clearing parlay scores collection...")
	parlayCollection := db.GetCollection("parlay_scores")
	result, err = parlayCollection.DeleteMany(ctx, bson.M{})
	if err != nil {
		log.Fatalf("Failed to clear parlay scores collection: %v", err)
	}
	log.Printf("✓ Deleted %d parlay scores", result.DeletedCount)
	
	log.Println("✓ Database cleared successfully")

	// STEP 2: Re-import legacy data
	log.Println("\n" + strings.Repeat("=", 50))
	log.Println("STEP 2: IMPORTING LEGACY DATA")
	log.Println(strings.Repeat("=", 50))

	// Create repositories
	gameRepo := database.NewMongoGameRepository(db)
	userRepo := database.NewMongoUserRepository(db)
	pickRepo := database.NewMongoPickRepository(db)

	// Create import service
	importService := services.NewLegacyImportService(gameRepo, userRepo, pickRepo)

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
	log.Println("✓ Legacy data imported successfully")

	// STEP 3: Calculate pick results for completed games
	log.Println("\n" + strings.Repeat("=", 50))
	log.Println("STEP 3: CALCULATING PICK RESULTS")
	log.Println(strings.Repeat("=", 50))

	// Create result calculation service
	resultCalcService := services.NewResultCalculationService(pickRepo, gameRepo)

	// Process all completed games for each season
	for _, season := range []int{2023, 2024, 2025} {
		log.Printf("Processing pick results for season %d...", season)
		if err := resultCalcService.ProcessAllCompletedGames(ctx, season); err != nil {
			log.Printf("Failed to process completed games for season %d: %v", season, err)
			continue
		}
		log.Printf("✓ Completed pick result processing for season %d", season)
	}

	// STEP 4: Recalculate all parlay scores
	log.Println("\n" + strings.Repeat("=", 50))
	log.Println("STEP 4: RECALCULATING PARLAY SCORES")
	log.Println(strings.Repeat("=", 50))

	// Create parlay repository and specialized services
	parlayRepo := database.NewMongoParlayRepository(db)
	parlayService := services.NewParlayService(pickRepo, gameRepo, parlayRepo)
	pickService := services.NewPickService(pickRepo, gameRepo, userRepo, parlayRepo)
	
	// Set specialized services on pick service
	pickService.SetSpecializedServices(parlayService, nil, nil)
	
	// Clear existing parlay scores to avoid double-counting from previous runs
	log.Println("Clearing existing parlay scores...")
	result, err = parlayCollection.DeleteMany(ctx, bson.M{})
	if err != nil {
		log.Fatalf("Failed to clear parlay scores collection: %v", err)
	}
	log.Printf("✓ Deleted %d existing parlay score records", result.DeletedCount)

	// Recalculate all seasons
	seasons := []int{2023, 2024, 2025}
	totalWeeksProcessed := 0
	totalPointsAwarded := 0

	for _, season := range seasons {
		log.Printf("\n--- SEASON %d ---", season)
		
		seasonWeeks := 0
		seasonPoints := 0
		
		// Process weeks 1-18 for each season
		for week := 1; week <= 18; week++ {
			// Check if there are picks for this week
			allPicks, err := pickRepo.FindByWeek(ctx, season, week)
			if err != nil {
				log.Printf("Error checking picks for S%d W%d: %v", season, week, err)
				continue
			}
			
			if len(allPicks) == 0 {
				continue // Skip weeks with no picks
			}
			
			// Process parlay scoring
			if err := pickService.ProcessWeekParlayScoring(ctx, season, week); err != nil {
				log.Printf("Failed to process parlay scoring for S%d W%d: %v", season, week, err)
				continue
			}
			
			// Get scores for summary
			scores, err := parlayRepo.GetWeekScores(ctx, season, week)
			if err != nil {
				log.Printf("Warning: couldn't fetch scores for S%d W%d: %v", season, week, err)
				continue
			}
			
			weekTotal := 0
			usersWithPoints := 0
			for _, score := range scores {
				weekTotal += score.TotalPoints
				if score.TotalPoints > 0 {
					usersWithPoints++
				}
			}
			
			log.Printf("Week %2d: %3d points awarded to %d users", week, weekTotal, usersWithPoints)
			seasonWeeks++
			seasonPoints += weekTotal
		}
		
		log.Printf("Season %d complete: %d weeks, %d total points", season, seasonWeeks, seasonPoints)
		totalWeeksProcessed += seasonWeeks
		totalPointsAwarded += seasonPoints
	}

	// STEP 5: Final verification
	log.Println("\n" + strings.Repeat("=", 50))
	log.Println("STEP 5: VERIFICATION")
	log.Println(strings.Repeat("=", 50))

	// Count final totals
	gamesCount, err := gamesCollection.CountDocuments(ctx, bson.M{})
	if err != nil {
		log.Printf("Warning: couldn't count games: %v", err)
	} else {
		log.Printf("✓ Total games in database: %d", gamesCount)
	}

	picksCount, err := picksCollection.CountDocuments(ctx, bson.M{})
	if err != nil {
		log.Printf("Warning: couldn't count picks: %v", err)
	} else {
		log.Printf("✓ Total picks in database: %d", picksCount)
	}

	parlayCount, err := parlayCollection.CountDocuments(ctx, bson.M{})
	if err != nil {
		log.Printf("Warning: couldn't count parlay scores: %v", err)
	} else {
		log.Printf("✓ Total parlay score records: %d", parlayCount)
	}

	// Final summary
	log.Println("\n" + strings.Repeat("=", 60))
	log.Println("FULL DATABASE REFRESH COMPLETE!")
	log.Println(strings.Repeat("=", 60))
	log.Printf("Weeks processed: %d", totalWeeksProcessed)
	log.Printf("Total points awarded: %d", totalPointsAwarded)
	log.Println("Database is now fully refreshed with corrected scoring logic!")
	log.Println(strings.Repeat("=", 60))
}