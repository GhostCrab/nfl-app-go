package main

import (
	"context"
	"log"
	"math/rand"
	"nfl-app-go/database"
	"nfl-app-go/models"
	"os"
	"time"

	"go.mongodb.org/mongo-driver/bson"
)

func getEnvTest(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

func main() {
	// Connect to database
	dbConfig := database.Config{
		Host:     getEnvTest("DB_HOST", "p5server"),
		Port:     getEnvTest("DB_PORT", "27017"),
		Username: getEnvTest("DB_USERNAME", "nflapp"),
		Password: getEnvTest("DB_PASSWORD", ""),
		Database: getEnvTest("DB_NAME", "nfl_app"),
	}

	db, err := database.NewMongoConnection(dbConfig)
	if err != nil {
		log.Fatalf("Database connection failed: %v", err)
	}
	defer db.Close()

	collection := db.GetCollection("games")
	
	// Find the ARI vs BUF game
	ctx := context.Background()
	filter := bson.M{
		"$or": []bson.M{
			{"away": "ARI", "home": "BUF"},
			{"away": "BUF", "home": "ARI"},
		},
	}

	var game models.Game
	err = collection.FindOne(ctx, filter).Decode(&game)
	if err != nil {
		log.Fatalf("Could not find ARI vs BUF game: %v", err)
	}

	log.Printf("Found game: %s vs %s (ID: %d)", game.Away, game.Home, game.ID)

	// Randomize the game state
	rand.Seed(time.Now().UnixNano())
	
	states := []models.GameState{
		models.GameStateScheduled,
		models.GameStateInPlay,
		models.GameStateCompleted,
	}
	
	newState := states[rand.Intn(len(states))]
	newHomeScore := rand.Intn(50)
	newAwayScore := rand.Intn(50)
	newQuarter := rand.Intn(4) + 1

	log.Printf("Updating game to: State=%s, HomeScore=%d, AwayScore=%d, Quarter=%d", 
		newState, newHomeScore, newAwayScore, newQuarter)

	// Update the game in database
	update := bson.M{
		"$set": bson.M{
			"state":     newState,
			"homeScore": newHomeScore,
			"awayScore": newAwayScore,
			"quarter":   newQuarter,
		},
	}

	result, err := collection.UpdateOne(ctx, bson.M{"id": game.ID}, update)
	if err != nil {
		log.Fatalf("Failed to update game: %v", err)
	}

	log.Printf("Successfully updated game. Modified count: %d", result.ModifiedCount)
	log.Println("Check your browser - the game should update automatically!")
}