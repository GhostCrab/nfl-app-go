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
	log.Println("=== SIMPLE SCORING CHECK ===")

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

	if err := db.TestConnection(); err != nil {
		log.Fatalf("MongoDB connection test failed: %v", err)
	}

	ctx := context.Background()

	// Check weekly_picks collection - specifically look at results distribution
	weeklyPicksCollection := db.GetCollection("weekly_picks")

	// Count total weekly picks documents
	totalWeeklyDocs, err := weeklyPicksCollection.CountDocuments(ctx, bson.M{})
	if err != nil {
		log.Fatalf("Failed to count weekly picks documents: %v", err)
	}
	log.Printf("Total weekly picks documents: %d", totalWeeklyDocs)

	// Count total individual picks and their results distribution using aggregation
	pipeline := []bson.M{
		{"$unwind": "$picks"},                                    // Flatten picks array
		{"$group": bson.M{
			"_id":   "$picks.result",
			"count": bson.M{"$sum": 1},
		}},
	}

	cursor, err := weeklyPicksCollection.Aggregate(ctx, pipeline)
	if err != nil {
		log.Fatalf("Failed to aggregate picks by result: %v", err)
	}
	defer cursor.Close(ctx)

	log.Println("\nPick results distribution:")
	totalIndividualPicks := 0
	for cursor.Next(ctx) {
		var result struct {
			ID    string `bson:"_id"`
			Count int    `bson:"count"`
		}
		if err := cursor.Decode(&result); err != nil {
			log.Printf("Error decoding result: %v", err)
			continue
		}
		log.Printf("  %s: %d picks", result.ID, result.Count)
		totalIndividualPicks += result.Count
	}
	log.Printf("Total individual picks: %d", totalIndividualPicks)

	// Sample a few picks from 2025 Week 4 to see their structure
	log.Println("\nSample picks from 2025 Week 4:")
	sampleCursor, err := weeklyPicksCollection.Find(ctx, bson.M{
		"season": 2025,
		"week":   4,
	}, nil)
	if err != nil {
		log.Fatalf("Failed to find sample weekly picks: %v", err)
	}
	defer sampleCursor.Close(ctx)

	count := 0
	for sampleCursor.Next(ctx) && count < 3 {
		var weeklyPicks bson.M
		if err := sampleCursor.Decode(&weeklyPicks); err != nil {
			log.Printf("Error decoding weekly picks: %v", err)
			continue
		}

		picks, ok := weeklyPicks["picks"].(bson.A)
		if !ok {
			log.Printf("Error: picks field is not a bson.A array")
			continue
		}

		log.Printf("  User %v (Season %v, Week %v) has %d picks:",
			weeklyPicks["user_id"], weeklyPicks["season"], weeklyPicks["week"], len(picks))

		// Show first few picks for this user
		for i, pickInterface := range picks {
			if i >= 3 { // Limit to first 3 picks per user
				break
			}
			if pick, ok := pickInterface.(bson.M); ok {
				log.Printf("    Pick %d: Game %v, Team %v, Type %v, Result: %v",
					i+1, pick["game_id"], pick["team_id"], pick["pick_type"], pick["result"])
			}
		}
		count++
	}

	// Check games collection - see if scores are populated
	gamesCollection := db.GetCollection("games")
	
	log.Println("\nSample games from 2025 Week 4:")
	gamesCursor, err := gamesCollection.Find(ctx, bson.M{
		"season": 2025,
		"week":   4,
	}, nil)
	if err != nil {
		log.Fatalf("Failed to find sample games: %v", err)
	}
	defer gamesCursor.Close(ctx)

	gameCount := 0
	for gamesCursor.Next(ctx) && gameCount < 5 {
		var game bson.M
		if err := gamesCursor.Decode(&game); err != nil {
			log.Printf("Error decoding game: %v", err)
			continue
		}
		log.Printf("  Game %v: %v vs %v - Home: %v, Away: %v", 
			game["id"], game["away_team_id"], game["home_team_id"], 
			game["home_score"], game["away_score"])
		gameCount++
	}

	// Check parlay scores collection
	parlayCollection := db.GetCollection("parlay_scores")
	
	parlayCount, err := parlayCollection.CountDocuments(ctx, bson.M{})
	if err != nil {
		log.Printf("Error counting parlay scores: %v", err)
	} else {
		log.Printf("\nTotal parlay score records: %d", parlayCount)
	}

	// Sample parlay scores
	log.Println("\nSample parlay scores:")
	parlayCursor, err := parlayCollection.Find(ctx, bson.M{}, nil)
	if err != nil {
		log.Printf("Error finding parlay scores: %v", err)
	} else {
		defer parlayCursor.Close(ctx)
		
		parlayCount := 0
		for parlayCursor.Next(ctx) && parlayCount < 10 {
			var parlay bson.M
			if err := parlayCursor.Decode(&parlay); err != nil {
				log.Printf("Error decoding parlay: %v", err)
				continue
			}
			log.Printf("  User %v S%v W%v: Reg=%v, Thu=%v, Fri=%v, Total=%v", 
				parlay["user_id"], parlay["season"], parlay["week"],
				parlay["regular_points"], parlay["bonus_thursday_points"], 
				parlay["bonus_friday_points"], parlay["total_points"])
			parlayCount++
		}
	}

	log.Println("\n=== CHECK COMPLETE ===")
}