package database

import (
	"context"
	"fmt"
	"log"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"

	"nfl-app-go/models"
)

// MongoPickRepository implements PickRepository for MongoDB
type MongoPickRepository struct {
	collection *mongo.Collection
	database   *mongo.Database
}

// NewMongoPickRepository creates a new MongoDB pick repository
func NewMongoPickRepository(db *MongoDB) *MongoPickRepository {
	collection := db.GetCollection("picks")
	
	// Create indexes for efficient querying
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	
	// Create compound indexes for common queries
	indexes := []mongo.IndexModel{
		{
			Keys: bson.D{
				{Key: "season", Value: 1},
				{Key: "week", Value: 1},
			},
		},
		{
			Keys: bson.D{
				{Key: "user_id", Value: 1},
				{Key: "season", Value: 1},
			},
		},
		{
			Keys: bson.D{
				{Key: "game_id", Value: 1},
			},
		},
		{
			Keys: bson.D{
				{Key: "user_id", Value: 1},
				{Key: "season", Value: 1},
				{Key: "week", Value: 1},
			},
		},
	}
	
	_, err := collection.Indexes().CreateMany(ctx, indexes)
	if err != nil {
		log.Printf("Warning: Could not create pick indexes: %v", err)
	}
	
	return &MongoPickRepository{
		collection: collection,
		database:   db.database,
	}
}

// Create inserts a new pick
func (r *MongoPickRepository) Create(ctx context.Context, pick *models.Pick) error {
	result, err := r.collection.InsertOne(ctx, pick)
	if err != nil {
		return fmt.Errorf("failed to create pick: %w", err)
	}

	// Note: Pick model no longer has ID field - using WeeklyPicks storage instead
	_ = result // Suppress unused variable warning
	return nil
}

// CreateMany inserts multiple picks in batch (useful for legacy import)
func (r *MongoPickRepository) CreateMany(ctx context.Context, picks []*models.Pick) error {
	if len(picks) == 0 {
		return nil
	}

	// Convert to interface slice for MongoDB
	docs := make([]interface{}, len(picks))

	for i, pick := range picks {
		// Note: Timestamps now managed at WeeklyPicks document level
		docs[i] = pick
	}
	
	_, err := r.collection.InsertMany(ctx, docs)
	if err != nil {
		return fmt.Errorf("failed to create picks batch: %w", err)
	}
	
	return nil
}

// FindByID retrieves a pick by its ID
func (r *MongoPickRepository) FindByID(ctx context.Context, id primitive.ObjectID) (*models.Pick, error) {
	var pick models.Pick
	err := r.collection.FindOne(ctx, bson.M{"_id": id}).Decode(&pick)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to find pick by ID: %w", err)
	}
	return &pick, nil
}

// FindByUserAndWeek retrieves all picks for a user in a specific season/week
func (r *MongoPickRepository) FindByUserAndWeek(ctx context.Context, userID, season, week int) ([]*models.Pick, error) {
	filter := bson.M{
		"user_id": userID,
		"season":  season,
		"week":    week,
	}
	
	cursor, err := r.collection.Find(ctx, filter)
	if err != nil {
		return nil, fmt.Errorf("failed to find picks by user and week: %w", err)
	}
	defer cursor.Close(ctx)
	
	var picks []*models.Pick
	for cursor.Next(ctx) {
		var pick models.Pick
		if err := cursor.Decode(&pick); err != nil {
			return nil, fmt.Errorf("failed to decode pick: %w", err)
		}
		picks = append(picks, &pick)
	}
	
	return picks, nil
}

// FindByWeek retrieves all picks for a specific season/week
func (r *MongoPickRepository) FindByWeek(ctx context.Context, season, week int) ([]*models.Pick, error) {
	filter := bson.M{
		"season": season,
		"week":   week,
	}
	
	cursor, err := r.collection.Find(ctx, filter)
	if err != nil {
		return nil, fmt.Errorf("failed to find picks by week: %w", err)
	}
	defer cursor.Close(ctx)
	
	var picks []*models.Pick
	for cursor.Next(ctx) {
		var pick models.Pick
		if err := cursor.Decode(&pick); err != nil {
			return nil, fmt.Errorf("failed to decode pick: %w", err)
		}
		picks = append(picks, &pick)
	}
	
	return picks, nil
}

// FindByGame retrieves all picks for a specific game
func (r *MongoPickRepository) FindByGame(ctx context.Context, gameID int) ([]*models.Pick, error) {
	filter := bson.M{"game_id": gameID}
	
	cursor, err := r.collection.Find(ctx, filter)
	if err != nil {
		return nil, fmt.Errorf("failed to find picks by game: %w", err)
	}
	defer cursor.Close(ctx)
	
	var picks []*models.Pick
	for cursor.Next(ctx) {
		var pick models.Pick
		if err := cursor.Decode(&pick); err != nil {
			return nil, fmt.Errorf("failed to decode pick: %w", err)
		}
		picks = append(picks, &pick)
	}
	
	return picks, nil
}

// FindByUserAndSeason retrieves all picks for a user in a specific season
func (r *MongoPickRepository) FindByUserAndSeason(ctx context.Context, userID, season int) ([]*models.Pick, error) {
	filter := bson.M{
		"user_id": userID,
		"season":  season,
	}
	
	// Sort by week, then by game_id for consistent ordering
	opts := options.Find().SetSort(bson.D{
		{Key: "week", Value: 1},
		{Key: "game_id", Value: 1},
	})
	
	cursor, err := r.collection.Find(ctx, filter, opts)
	if err != nil {
		return nil, fmt.Errorf("failed to find picks by user and season: %w", err)
	}
	defer cursor.Close(ctx)
	
	var picks []*models.Pick
	for cursor.Next(ctx) {
		var pick models.Pick
		if err := cursor.Decode(&pick); err != nil {
			return nil, fmt.Errorf("failed to decode pick: %w", err)
		}
		picks = append(picks, &pick)
	}
	
	return picks, nil
}

// Update modifies an existing pick
func (r *MongoPickRepository) Update(ctx context.Context, pick *models.Pick) error {
	// Note: Pick model no longer has ID field - this method may not work with new model
	// Consider using WeeklyPicksRepository.Upsert instead

	filter := bson.M{"user_id": pick.UserID, "game_id": pick.GameID}
	update := bson.M{"$set": pick}

	result, err := r.collection.UpdateOne(ctx, filter, update)
	if err != nil {
		return fmt.Errorf("failed to update pick: %w", err)
	}

	if result.MatchedCount == 0 {
		return fmt.Errorf("pick not found")
	}
	
	return nil
}

// UpdateResult updates the result of a pick (for game completion processing)
func (r *MongoPickRepository) UpdateResult(ctx context.Context, id primitive.ObjectID, result models.PickResult) error {
	filter := bson.M{"_id": id}
	update := bson.M{
		"$set": bson.M{
			"result":     result,
			"updated_at": time.Now(),
		},
	}
	
	updateResult, err := r.collection.UpdateOne(ctx, filter, update)
	if err != nil {
		return fmt.Errorf("failed to update pick result: %w", err)
	}
	
	if updateResult.MatchedCount == 0 {
		return fmt.Errorf("pick not found")
	}
	
	return nil
}

// Delete removes a pick
func (r *MongoPickRepository) Delete(ctx context.Context, id primitive.ObjectID) error {
	result, err := r.collection.DeleteOne(ctx, bson.M{"_id": id})
	if err != nil {
		return fmt.Errorf("failed to delete pick: %w", err)
	}
	
	if result.DeletedCount == 0 {
		return fmt.Errorf("pick not found")
	}
	
	return nil
}

// DeleteByUserAndWeek removes all picks for a user in a specific season/week
func (r *MongoPickRepository) DeleteByUserAndWeek(ctx context.Context, userID, season, week int) error {
	filter := bson.M{
		"user_id": userID,
		"season":  season,
		"week":    week,
	}
	
	result, err := r.collection.DeleteMany(ctx, filter)
	if err != nil {
		return fmt.Errorf("failed to delete picks by user and week: %w", err)
	}
	
	log.Printf("Deleted %d picks for user %d, season %d, week %d", result.DeletedCount, userID, season, week)
	return nil
}

// DeleteByUserGameAndWeek removes picks for a specific user, game, and week
func (r *MongoPickRepository) DeleteByUserGameAndWeek(ctx context.Context, userID, gameID, season, week int) error {
	filter := bson.M{
		"user_id": userID,
		"game_id": gameID,
		"season":  season,
		"week":    week,
	}
	
	result, err := r.collection.DeleteMany(ctx, filter)
	if err != nil {
		return fmt.Errorf("failed to delete picks by user, game and week: %w", err)
	}
	
	log.Printf("Deleted %d picks for user %d, game %d, season %d, week %d", result.DeletedCount, userID, gameID, season, week)
	return nil
}

// GetUserRecord calculates a user's win-loss record for a season
func (r *MongoPickRepository) GetUserRecord(ctx context.Context, userID, season int) (*models.UserRecord, error) {
	pipeline := mongo.Pipeline{
		{{"$match", bson.D{
			{Key: "user_id", Value: userID},
			{Key: "season", Value: season},
		}}},
		{{"$group", bson.D{
			{Key: "_id", Value: nil},
			{Key: "wins", Value: bson.D{
				{Key: "$sum", Value: bson.D{
					{Key: "$cond", Value: bson.A{
						bson.D{{Key: "$eq", Value: bson.A{"$result", models.PickResultWin}}},
						1,
						0,
					}},
				}},
			}},
			{Key: "losses", Value: bson.D{
				{Key: "$sum", Value: bson.D{
					{Key: "$cond", Value: bson.A{
						bson.D{{Key: "$eq", Value: bson.A{"$result", models.PickResultLoss}}},
						1,
						0,
					}},
				}},
			}},
			{Key: "pushes", Value: bson.D{
				{Key: "$sum", Value: bson.D{
					{Key: "$cond", Value: bson.A{
						bson.D{{Key: "$eq", Value: bson.A{"$result", models.PickResultPush}}},
						1,
						0,
					}},
				}},
			}},
		}}},
	}
	
	cursor, err := r.collection.Aggregate(ctx, pipeline)
	if err != nil {
		return nil, fmt.Errorf("failed to aggregate user record: %w", err)
	}
	defer cursor.Close(ctx)
	
	if cursor.Next(ctx) {
		var result struct {
			Wins   int `bson:"wins"`
			Losses int `bson:"losses"`
			Pushes int `bson:"pushes"`
		}
		if err := cursor.Decode(&result); err != nil {
			return nil, fmt.Errorf("failed to decode user record: %w", err)
		}
		
		return &models.UserRecord{
			Wins:   result.Wins,
			Losses: result.Losses,
			Pushes: result.Pushes,
		}, nil
	}
	
	// No picks found, return empty record
	return &models.UserRecord{
		Wins:   0,
		Losses: 0,
		Pushes: 0,
	}, nil
}

// Count returns the total number of picks
func (r *MongoPickRepository) Count(ctx context.Context) (int64, error) {
	return r.collection.CountDocuments(ctx, bson.M{})
}

// CountBySeasonAndWeek returns the number of picks for a specific season/week
func (r *MongoPickRepository) CountBySeasonAndWeek(ctx context.Context, season, week int) (int64, error) {
	filter := bson.M{
		"season": season,
		"week":   week,
	}
	return r.collection.CountDocuments(ctx, filter)
}

// FindBySeason retrieves all picks for a specific season
func (r *MongoPickRepository) FindBySeason(ctx context.Context, season int) ([]*models.Pick, error) {
	filter := bson.M{"season": season}

	// Limit to first 10 for duplicate check (don't need to load all)
	opts := options.Find().SetLimit(10)

	cursor, err := r.collection.Find(ctx, filter, opts)
	if err != nil {
		return nil, fmt.Errorf("failed to find picks by season: %w", err)
	}
	defer cursor.Close(ctx)

	var picks []*models.Pick
	for cursor.Next(ctx) {
		var pick models.Pick
		if err := cursor.Decode(&pick); err != nil {
			return nil, fmt.Errorf("failed to decode pick: %w", err)
		}
		picks = append(picks, &pick)
	}

	return picks, nil
}

// BulkWrite performs multiple pick operations in a single database transaction
// This reduces change stream events and improves performance
func (r *MongoPickRepository) BulkWrite(ctx context.Context, operations []mongo.WriteModel) error {
	if len(operations) == 0 {
		return nil // No operations to perform
	}

	opts := options.BulkWrite().SetOrdered(true) // Ordered execution: deletes first, then inserts

	result, err := r.collection.BulkWrite(ctx, operations, opts)
	if err != nil {
		return fmt.Errorf("bulk write failed: %w", err)
	}

	// Log the results for debugging
	if result.DeletedCount > 0 || result.InsertedCount > 0 {
		fmt.Printf("Bulk pick operation completed: %d deleted, %d inserted\n",
			result.DeletedCount, result.InsertedCount)
	}

	return nil
}

