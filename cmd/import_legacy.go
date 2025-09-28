package main

import (
	"log"
	"nfl-app-go/database"
	"nfl-app-go/services"
	"os"
	"path/filepath"
	"strings"

	"github.com/joho/godotenv"
)

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

// getEnvWithDevelOverride gets environment variable with development port override
func getEnvWithDevelOverride(key, develKey, defaultValue string) string {
	// Check if we're in development mode
	environment := getEnv("ENVIRONMENT", "development")
	isDevelopment := strings.ToLower(environment) == "development"

	// Get the base value
	value := getEnv(key, defaultValue)

	// Override with development value if in development mode
	if isDevelopment {
		if develValue := getEnv(develKey, ""); develValue != "" {
			value = develValue
		}
	}

	return value
}

func main() {
	log.Println("=== NFL Legacy Data Import ===")
	
	// Load .env file
	if err := godotenv.Load(); err != nil {
		log.Printf("Warning: Could not load .env file: %v", err)
	}

	// Initialize MongoDB connection
	dbConfig := database.Config{
		Host:     getEnv("DB_HOST", "p5server"),
		Port:     getEnvWithDevelOverride("DB_PORT", "DEVEL_DB_PORT", "27017"),
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
	gameRepo := database.NewMongoGameRepository(db)
	userRepo := database.NewMongoUserRepository(db)
	weeklyPicksRepo := database.NewMongoWeeklyPicksRepository(db)

	// Create import service (now uses WeeklyPicksRepository)
	importService := services.NewLegacyImportService(gameRepo, userRepo, weeklyPicksRepo)

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
	
	log.Printf("Using legacy-dbs directory: %s", legacyDbsPath)

	// First, show what we would import
	log.Println("\n--- ANALYSIS PHASE ---")
	if err := importService.GetImportSummary(legacyDbsPath); err != nil {
		log.Fatalf("Failed to analyze legacy data: %v", err)
	}

	// Ask for confirmation
	log.Println("\n--- IMPORT PHASE ---")
	log.Println("Ready to import legacy data. This will:")
	log.Println("  1. Import all games from 2023 and 2024 seasons")
	log.Println("  2. Import all user picks WITH SEASON TRACKING")
	log.Println("  3. Preserve existing 2025 data")
	log.Println()
	
	// For automation, we'll proceed automatically
	// In production, you might want to add a confirmation prompt
	
	// Import all legacy data
	if err := importService.ImportAllLegacyData(legacyDbsPath); err != nil {
		log.Fatalf("Failed to import legacy data: %v", err)
	}

	log.Println("\n=== IMPORT COMPLETE ===")
	log.Println("Legacy data has been successfully imported into MongoDB!")
	log.Println("You can now access historical games from 2023 and 2024 seasons.")
}