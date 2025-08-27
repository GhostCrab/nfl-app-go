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

type MongoParlayRepository struct {
	collection *mongo.Collection
}

func NewMongoParlayRepository(db *MongoDB) *MongoParlayRepository {
	collection := db.GetCollection("parlay_scores")
	
	// Create compound index on user_id, season, week for faster lookups
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	
	indexModel := mongo.IndexModel{
		Keys: bson.D{{"user_id", 1}, {"season", 1}, {"week", 1}},
		Options: options.Index().SetUnique(true),
	}
	
	_, err := collection.Indexes().CreateOne(ctx, indexModel)
	if err != nil {
		log.Printf("Failed to create index on parlay_scores collection: %v", err)
	}

	return &MongoParlayRepository{
		collection: collection,
	}
}

// UpsertParlayScore creates or updates a parlay score entry
func (r *MongoParlayRepository) UpsertParlayScore(ctx context.Context, score *models.ParlayScore) error {
	filter := bson.M{
		"user_id": score.UserID,
		"season":  score.Season,
		"week":    score.Week,
	}
	
	// Update timestamp
	score.UpdatedAt = time.Now()
	score.CalculateTotal()
	
	opts := options.Replace().SetUpsert(true)
	_, err := r.collection.ReplaceOne(ctx, filter, score, opts)
	if err != nil {
		return fmt.Errorf("failed to upsert parlay score: %w", err)
	}
	
	return nil
}

// GetUserParlayScore retrieves a user's parlay score for a specific week
func (r *MongoParlayRepository) GetUserParlayScore(ctx context.Context, userID, season, week int) (*models.ParlayScore, error) {
	filter := bson.M{
		"user_id": userID,
		"season":  season,
		"week":    week,
	}
	
	var score models.ParlayScore
	err := r.collection.FindOne(ctx, filter).Decode(&score)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return nil, nil // No score found
		}
		return nil, fmt.Errorf("failed to get parlay score: %w", err)
	}
	
	return &score, nil
}

// GetUserSeasonTotal calculates a user's total parlay points for a season
func (r *MongoParlayRepository) GetUserSeasonTotal(ctx context.Context, userID, season int) (int, error) {
	pipeline := mongo.Pipeline{
		{{"$match", bson.D{
			{Key: "user_id", Value: userID},
			{Key: "season", Value: season},
		}}},
		{{"$group", bson.D{
			{Key: "_id", Value: nil},
			{Key: "total", Value: bson.D{{"$sum", "$total_points"}}},
		}}},
	}
	
	cursor, err := r.collection.Aggregate(ctx, pipeline)
	if err != nil {
		return 0, fmt.Errorf("failed to aggregate season total: %w", err)
	}
	defer cursor.Close(ctx)
	
	if cursor.Next(ctx) {
		var result struct {
			Total int `bson:"total"`
		}
		if err := cursor.Decode(&result); err != nil {
			return 0, fmt.Errorf("failed to decode season total: %w", err)
		}
		return result.Total, nil
	}
	
	return 0, nil // No scores found
}

// GetAllUsersSeasonTotals gets parlay totals for all users in a season
func (r *MongoParlayRepository) GetAllUsersSeasonTotals(ctx context.Context, season int) (map[int]int, error) {
	pipeline := mongo.Pipeline{
		{{"$match", bson.D{
			{Key: "season", Value: season},
		}}},
		{{"$group", bson.D{
			{Key: "_id", Value: "$user_id"},
			{Key: "total", Value: bson.D{{"$sum", "$total_points"}}},
		}}},
	}
	
	cursor, err := r.collection.Aggregate(ctx, pipeline)
	if err != nil {
		return nil, fmt.Errorf("failed to aggregate all users season totals: %w", err)
	}
	defer cursor.Close(ctx)
	
	totals := make(map[int]int)
	for cursor.Next(ctx) {
		var result struct {
			UserID int `bson:"_id"`
			Total  int `bson:"total"`
		}
		if err := cursor.Decode(&result); err != nil {
			return nil, fmt.Errorf("failed to decode user season total: %w", err)
		}
		totals[result.UserID] = result.Total
	}
	
	return totals, nil
}

// DeleteUserWeekScore deletes a user's parlay score for a specific week
func (r *MongoParlayRepository) DeleteUserWeekScore(ctx context.Context, userID, season, week int) error {
	filter := bson.M{
		"user_id": userID,
		"season":  season,
		"week":    week,
	}
	
	_, err := r.collection.DeleteOne(ctx, filter)
	if err != nil {
		return fmt.Errorf("failed to delete parlay score: %w", err)
	}
	
	return nil
}

// GetWeekScores gets all users' parlay scores for a specific week
func (r *MongoParlayRepository) GetWeekScores(ctx context.Context, season, week int) ([]*models.ParlayScore, error) {
	filter := bson.M{
		"season": season,
		"week":   week,
	}
	
	cursor, err := r.collection.Find(ctx, filter)
	if err != nil {
		return nil, fmt.Errorf("failed to find week scores: %w", err)
	}
	defer cursor.Close(ctx)
	
	var scores []*models.ParlayScore
	for cursor.Next(ctx) {
		var score models.ParlayScore
		if err := cursor.Decode(&score); err != nil {
			return nil, fmt.Errorf("failed to decode parlay score: %w", err)
		}
		scores = append(scores, &score)
	}
	
	return scores, nil
}

// GetUserCumulativeScoreUpToWeek calculates a user's cumulative parlay points up to and including a specific week
func (r *MongoParlayRepository) GetUserCumulativeScoreUpToWeek(ctx context.Context, userID, season, week int) (int, error) {
	pipeline := mongo.Pipeline{
		{{"$match", bson.D{
			{Key: "user_id", Value: userID},
			{Key: "season", Value: season},
			{Key: "week", Value: bson.D{{"$lte", week}}}, // Less than or equal to the target week
		}}},
		{{"$group", bson.D{
			{Key: "_id", Value: nil},
			{Key: "total", Value: bson.D{{"$sum", "$total_points"}}},
		}}},
	}
	
	cursor, err := r.collection.Aggregate(ctx, pipeline)
	if err != nil {
		return 0, fmt.Errorf("failed to aggregate cumulative score: %w", err)
	}
	defer cursor.Close(ctx)
	
	if cursor.Next(ctx) {
		var result struct {
			Total int `bson:"total"`
		}
		if err := cursor.Decode(&result); err != nil {
			return 0, fmt.Errorf("failed to decode cumulative score: %w", err)
		}
		return result.Total, nil
	}
	
	return 0, nil // No scores found
}

// GetAllUsersCumulativeScoresUpToWeek gets cumulative parlay totals for all users up to a specific week
func (r *MongoParlayRepository) GetAllUsersCumulativeScoresUpToWeek(ctx context.Context, season, week int) (map[int]int, error) {
	pipeline := mongo.Pipeline{
		{{"$match", bson.D{
			{Key: "season", Value: season},
			{Key: "week", Value: bson.D{{"$lte", week}}}, // Less than or equal to the target week
		}}},
		{{"$group", bson.D{
			{Key: "_id", Value: "$user_id"},
			{Key: "total", Value: bson.D{{"$sum", "$total_points"}}},
		}}},
	}
	
	cursor, err := r.collection.Aggregate(ctx, pipeline)
	if err != nil {
		return nil, fmt.Errorf("failed to aggregate all users cumulative scores: %w", err)
	}
	defer cursor.Close(ctx)
	
	totals := make(map[int]int)
	for cursor.Next(ctx) {
		var result struct {
			UserID int `bson:"_id"`
			Total  int `bson:"total"`
		}
		if err := cursor.Decode(&result); err != nil {
			return nil, fmt.Errorf("failed to decode user cumulative score: %w", err)
		}
		totals[result.UserID] = result.Total
	}
	
	return totals, nil
}