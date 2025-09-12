package database

import (
	"context"
	"fmt"
	"nfl-app-go/models"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// GetPicksByGameID retrieves all picks for a specific game
// This method supports result calculation service operations
func (r *MongoPickRepository) GetPicksByGameID(ctx context.Context, gameID int) ([]models.Pick, error) {
	ctx, cancel := WithMediumTimeout()
	defer cancel()

	filter := bson.M{"game_id": gameID}
	
	// Sort by user_id for consistent ordering
	opts := options.Find().SetSort(bson.D{{Key: "user_id", Value: 1}})
	
	cursor, err := r.collection.Find(ctx, filter, opts)
	if err != nil {
		return nil, fmt.Errorf("failed to find picks by game ID %d: %w", gameID, err)
	}
	defer cursor.Close(ctx)
	
	var picks []models.Pick
	for cursor.Next(ctx) {
		var pick models.Pick
		if err := cursor.Decode(&pick); err != nil {
			return nil, fmt.Errorf("failed to decode pick: %w", err)
		}
		picks = append(picks, pick)
	}
	
	return picks, nil
}

// UpdatePickResult updates the result of a specific pick (alias for UpdateResult)
// This method supports result calculation service operations  
func (r *MongoPickRepository) UpdatePickResult(ctx context.Context, pickID primitive.ObjectID, result models.PickResult) error {
	return r.UpdateResult(ctx, pickID, result)
}

// GetUserPicksBySeason gets all picks for a user in a specific season
// This method supports analytics service operations
func (r *MongoPickRepository) GetUserPicksBySeason(ctx context.Context, userID, season int) ([]models.Pick, error) {
	ctx, cancel := WithMediumTimeout()
	defer cancel()

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
		return nil, fmt.Errorf("failed to find picks by user %d and season %d: %w", userID, season, err)
	}
	defer cursor.Close(ctx)
	
	var picks []models.Pick
	for cursor.Next(ctx) {
		var pick models.Pick
		if err := cursor.Decode(&pick); err != nil {
			return nil, fmt.Errorf("failed to decode pick: %w", err)
		}
		picks = append(picks, pick)
	}
	
	return picks, nil
}

// GetPicksBySeason gets all picks for a season (for analytics)
// This method supports analytics service operations
func (r *MongoPickRepository) GetPicksBySeason(ctx context.Context, season int) ([]models.Pick, error) {
	ctx, cancel := WithLongTimeout() // Longer timeout for potentially large result set
	defer cancel()

	filter := bson.M{"season": season}
	
	// Sort by week, user_id, game_id for consistent ordering
	opts := options.Find().SetSort(bson.D{
		{Key: "week", Value: 1},
		{Key: "user_id", Value: 1},
		{Key: "game_id", Value: 1},
	})
	
	cursor, err := r.collection.Find(ctx, filter, opts)
	if err != nil {
		return nil, fmt.Errorf("failed to find picks by season %d: %w", season, err)
	}
	defer cursor.Close(ctx)
	
	var picks []models.Pick
	for cursor.Next(ctx) {
		var pick models.Pick
		if err := cursor.Decode(&pick); err != nil {
			return nil, fmt.Errorf("failed to decode pick: %w", err)
		}
		picks = append(picks, pick)
	}
	
	return picks, nil
}

// GetUniqueUserIDsForWeek gets all unique user IDs who made picks in a specific week
// This method supports parlay service operations
func (r *MongoPickRepository) GetUniqueUserIDsForWeek(ctx context.Context, season, week int) ([]int, error) {
	ctx, cancel := WithMediumTimeout()
	defer cancel()

	pipeline := mongo.Pipeline{
		{{"$match", bson.D{
			{Key: "season", Value: season},
			{Key: "week", Value: week},
		}}},
		{{"$group", bson.D{
			{Key: "_id", Value: "$user_id"},
		}}},
		{{"$sort", bson.D{
			{Key: "_id", Value: 1},
		}}},
	}
	
	cursor, err := r.collection.Aggregate(ctx, pipeline)
	if err != nil {
		return nil, fmt.Errorf("failed to aggregate unique user IDs for season %d, week %d: %w", season, week, err)
	}
	defer cursor.Close(ctx)
	
	var userIDs []int
	for cursor.Next(ctx) {
		var result struct {
			ID int `bson:"_id"`
		}
		if err := cursor.Decode(&result); err != nil {
			return nil, fmt.Errorf("failed to decode user ID: %w", err)
		}
		userIDs = append(userIDs, result.ID)
	}
	
	return userIDs, nil
}

// GetUserPicksForWeek gets picks for a specific user and week
// This method supports parlay service and general user pick retrieval
func (r *MongoPickRepository) GetUserPicksForWeek(ctx context.Context, userID, season, week int) ([]models.Pick, error) {
	ctx, cancel := WithMediumTimeout()
	defer cancel()

	filter := bson.M{
		"user_id": userID,
		"season":  season,
		"week":    week,
	}
	
	// Sort by game_id for consistent ordering
	opts := options.Find().SetSort(bson.D{{Key: "game_id", Value: 1}})
	
	cursor, err := r.collection.Find(ctx, filter, opts)
	if err != nil {
		return nil, fmt.Errorf("failed to find picks for user %d, season %d, week %d: %w", userID, season, week, err)
	}
	defer cursor.Close(ctx)
	
	var picks []models.Pick
	for cursor.Next(ctx) {
		var pick models.Pick
		if err := cursor.Decode(&pick); err != nil {
			return nil, fmt.Errorf("failed to decode pick: %w", err)
		}
		picks = append(picks, pick)
	}
	
	return picks, nil
}