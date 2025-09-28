package main

import (
	"context"
	"log"
	"nfl-app-go/database"
	"os"
	"strings"
	"time"

	"github.com/joho/godotenv"
	"go.mongodb.org/mongo-driver/bson"
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
	log.Println("=== Fix MongoDB Indexes ===")
	
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

	// Get the games collection
	collection := db.GetCollection("games")
	
	// List existing indexes
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	
	log.Println("Current indexes:")
	cursor, err := collection.Indexes().List(ctx)
	if err != nil {
		log.Fatalf("Failed to list indexes: %v", err)
	}
	
	var indexes []bson.M
	if err = cursor.All(ctx, &indexes); err != nil {
		log.Fatalf("Failed to read indexes: %v", err)
	}
	
	for _, index := range indexes {
		log.Printf("  Index: %v", index["name"])
	}
	
	// Drop the old unique index on 'id' if it exists
	log.Println("Dropping old 'id_1' index if it exists...")
	_, err = collection.Indexes().DropOne(ctx, "id_1")
	if err != nil {
		log.Printf("Note: Could not drop 'id_1' index (may not exist): %v", err)
	} else {
		log.Println("Successfully dropped old 'id_1' index")
	}
	
	log.Println("Index cleanup complete!")
	log.Println("The new compound index (id + season) will be created when you run the import again.")
}