package database

import (
	"context"
	"fmt"
	"log"
	"nfl-app-go/models"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

type MongoGameRepository struct {
	collection *mongo.Collection
}

func NewMongoGameRepository(db *MongoDB) *MongoGameRepository {
	collection := db.GetCollection("games")
	
	// Create compound index on game ID and season for faster upserts across seasons
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	
	indexModel := mongo.IndexModel{
		Keys: bson.D{{"id", 1}, {"season", 1}},
		Options: options.Index().SetUnique(true),
	}
	
	_, err := collection.Indexes().CreateOne(ctx, indexModel)
	if err != nil {
		log.Printf("Failed to create index on games collection: %v", err)
	}

	return &MongoGameRepository{
		collection: collection,
	}
}

func (r *MongoGameRepository) UpsertGame(game *models.Game) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Use both ID and season for unique identification across seasons
	filter := bson.M{"id": game.ID, "season": game.Season}
	
	// Use ReplaceOne with upsert option
	opts := options.Replace().SetUpsert(true)
	
	_, err := r.collection.ReplaceOne(ctx, filter, game, opts)
	if err != nil {
		return fmt.Errorf("failed to upsert game %d: %w", game.ID, err)
	}

	return nil
}

func (r *MongoGameRepository) GetAllGames() ([]*models.Game, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	cursor, err := r.collection.Find(ctx, bson.M{})
	if err != nil {
		return nil, fmt.Errorf("failed to find games: %w", err)
	}
	defer cursor.Close(ctx)

	var games []*models.Game
	if err := cursor.All(ctx, &games); err != nil {
		return nil, fmt.Errorf("failed to decode games: %w", err)
	}

	return games, nil
}

// GetGamesBySeason gets all games for a specific season
func (r *MongoGameRepository) GetGamesBySeason(season int) ([]*models.Game, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	filter := bson.M{"season": season}
	cursor, err := r.collection.Find(ctx, filter)
	if err != nil {
		return nil, fmt.Errorf("failed to find games for season %d: %w", season, err)
	}
	defer cursor.Close(ctx)

	var games []*models.Game
	if err := cursor.All(ctx, &games); err != nil {
		return nil, fmt.Errorf("failed to decode games: %w", err)
	}

	return games, nil
}

// FindByID gets a game by its ESPN ID (legacy method name for compatibility)
func (r *MongoGameRepository) FindByESPNID(ctx context.Context, espnID int) (*models.Game, error) {
	filter := bson.M{"id": espnID}
	
	var game models.Game
	err := r.collection.FindOne(ctx, filter).Decode(&game)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return nil, nil // Not found
		}
		return nil, fmt.Errorf("failed to find game by ESPN ID %d: %w", espnID, err)
	}
	
	return &game, nil
}

func (r *MongoGameRepository) GetGamesByWeekSeason(week, season int) ([]*models.Game, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	filter := bson.M{
		"week":   week,
		"season": season,
	}

	cursor, err := r.collection.Find(ctx, filter)
	if err != nil {
		return nil, fmt.Errorf("failed to find games for week %d season %d: %w", week, season, err)
	}
	defer cursor.Close(ctx)

	var games []*models.Game
	if err := cursor.All(ctx, &games); err != nil {
		return nil, fmt.Errorf("failed to decode games: %w", err)
	}

	return games, nil
}

func (r *MongoGameRepository) BulkUpsertGames(games []*models.Game) error {
	if len(games) == 0 {
		return nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	log.Printf("Upserting %d games in MongoDB", len(games))
	log.Printf("Collection name: %s, Database: %s", r.collection.Name(), r.collection.Database().Name())

	// MongoDB bulk write operations
	var operations []mongo.WriteModel
	
	for _, game := range games {
		// Use both ID and season for unique identification across seasons  
		filter := bson.M{"id": game.ID, "season": game.Season}
		replacement := game
		
		operation := mongo.NewReplaceOneModel().
			SetFilter(filter).
			SetReplacement(replacement).
			SetUpsert(true)
		
		operations = append(operations, operation)
	}

	opts := options.BulkWrite().SetOrdered(false)
	log.Printf("Executing bulk write with %d operations", len(operations))
	result, err := r.collection.BulkWrite(ctx, operations, opts)
	if err != nil {
		log.Printf("Bulk write error details: %v", err)
		log.Printf("Error type: %T", err)
		return fmt.Errorf("bulk upsert failed: %w", err)
	}

	log.Printf("Successfully processed %d games: %d upserted, %d modified", 
		len(games), result.UpsertedCount, result.ModifiedCount)
	
	return nil
}

func (r *MongoGameRepository) ClearAllGames() error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	result, err := r.collection.DeleteMany(ctx, bson.M{})
	if err != nil {
		return fmt.Errorf("failed to clear games collection: %w", err)
	}

	log.Printf("Cleared %d games from MongoDB", result.DeletedCount)
	return nil
}