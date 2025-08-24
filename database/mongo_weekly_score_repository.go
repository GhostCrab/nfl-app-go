package database

import (
	"context"
	"fmt"
	"log"
	"time"

	"nfl-app-go/models"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// MongoWeeklyScoreRepository implements WeeklyScoreRepository using MongoDB
type MongoWeeklyScoreRepository struct {
	collection *mongo.Collection
}

// NewMongoWeeklyScoreRepository creates a new MongoDB weekly score repository
func NewMongoWeeklyScoreRepository(db *MongoDB) *MongoWeeklyScoreRepository {
	return &MongoWeeklyScoreRepository{
		collection: db.GetCollection("weekly_scores"),
	}
}

// FindByUserSeasonWeek finds all weekly scores for a user in a specific season and week
func (r *MongoWeeklyScoreRepository) FindByUserSeasonWeek(ctx context.Context, userID, season, week int) ([]*models.WeeklyScore, error) {
	filter := bson.M{
		"user_id": userID,
		"season":  season,
		"week":    week,
	}

	cursor, err := r.collection.Find(ctx, filter)
	if err != nil {
		return nil, fmt.Errorf("failed to find weekly scores: %w", err)
	}
	defer cursor.Close(ctx)

	var weeklyScores []*models.WeeklyScore
	if err := cursor.All(ctx, &weeklyScores); err != nil {
		return nil, fmt.Errorf("failed to decode weekly scores: %w", err)
	}

	return weeklyScores, nil
}

// FindByUserSeason finds all weekly scores for a user in a specific season
func (r *MongoWeeklyScoreRepository) FindByUserSeason(ctx context.Context, userID, season int) ([]*models.WeeklyScore, error) {
	filter := bson.M{
		"user_id": userID,
		"season":  season,
	}

	cursor, err := r.collection.Find(ctx, filter, options.Find().SetSort(bson.D{{Key: "week", Value: 1}}))
	if err != nil {
		return nil, fmt.Errorf("failed to find weekly scores: %w", err)
	}
	defer cursor.Close(ctx)

	var weeklyScores []*models.WeeklyScore
	if err := cursor.All(ctx, &weeklyScores); err != nil {
		return nil, fmt.Errorf("failed to decode weekly scores: %w", err)
	}

	return weeklyScores, nil
}

// FindBySeasonWeek finds all weekly scores for a specific season and week
func (r *MongoWeeklyScoreRepository) FindBySeasonWeek(ctx context.Context, season, week int) ([]*models.WeeklyScore, error) {
	filter := bson.M{
		"season": season,
		"week":   week,
	}

	cursor, err := r.collection.Find(ctx, filter)
	if err != nil {
		return nil, fmt.Errorf("failed to find weekly scores: %w", err)
	}
	defer cursor.Close(ctx)

	var weeklyScores []*models.WeeklyScore
	if err := cursor.All(ctx, &weeklyScores); err != nil {
		return nil, fmt.Errorf("failed to decode weekly scores: %w", err)
	}

	return weeklyScores, nil
}

// Upsert creates or updates a weekly score record
func (r *MongoWeeklyScoreRepository) Upsert(ctx context.Context, weeklyScore *models.WeeklyScore) error {
	filter := bson.M{
		"user_id":  weeklyScore.UserID,
		"season":   weeklyScore.Season,
		"week":     weeklyScore.Week,
		"day_type": weeklyScore.DayType,
	}

	update := bson.M{
		"$set": bson.M{
			"total_picks":   weeklyScore.TotalPicks,
			"winning_picks": weeklyScore.WinningPicks,
			"losing_picks":  weeklyScore.LosingPicks,
			"push_picks":    weeklyScore.PushPicks,
			"pending_picks": weeklyScore.PendingPicks,
			"points":        weeklyScore.Points,
			"is_complete":   weeklyScore.IsComplete,
			"updated_at":    weeklyScore.UpdatedAt,
		},
		"$setOnInsert": bson.M{
			"user_id":    weeklyScore.UserID,
			"season":     weeklyScore.Season,
			"week":       weeklyScore.Week,
			"day_type":   weeklyScore.DayType,
			"created_at": weeklyScore.CreatedAt,
		},
	}

	opts := options.Update().SetUpsert(true)
	result, err := r.collection.UpdateOne(ctx, filter, update, opts)
	if err != nil {
		return fmt.Errorf("failed to upsert weekly score: %w", err)
	}

	// If this was an insert, set the ID
	if result.UpsertedID != nil {
		if oid, ok := result.UpsertedID.(primitive.ObjectID); ok {
			weeklyScore.ID = oid.Hex()
		}
	}

	log.Printf("Upserted weekly score for user %d, season %d, week %d, dayType %s: %d points",
		weeklyScore.UserID, weeklyScore.Season, weeklyScore.Week, weeklyScore.DayType, weeklyScore.Points)

	return nil
}

// GetSeasonLeaderboard gets season scores for all users, ordered by total points
func (r *MongoWeeklyScoreRepository) GetSeasonLeaderboard(ctx context.Context, season int) ([]*models.SeasonScore, error) {
	pipeline := mongo.Pipeline{
		{{"$match", bson.D{{Key: "season", Value: season}}}},
		{{"$group", bson.D{
			{Key: "_id", Value: "$user_id"},
			{Key: "total_points", Value: bson.D{{Key: "$sum", Value: "$points"}}},
			{Key: "weeks", Value: bson.D{{Key: "$addToSet", Value: "$week"}}},
		}}},
		{{"$sort", bson.D{{Key: "total_points", Value: -1}}}},
	}

	cursor, err := r.collection.Aggregate(ctx, pipeline)
	if err != nil {
		return nil, fmt.Errorf("failed to get season leaderboard: %w", err)
	}
	defer cursor.Close(ctx)

	var results []struct {
		ID          int   `bson:"_id"`
		TotalPoints int   `bson:"total_points"`
		Weeks       []int `bson:"weeks"`
	}

	if err := cursor.All(ctx, &results); err != nil {
		return nil, fmt.Errorf("failed to decode leaderboard results: %w", err)
	}

	seasonScores := make([]*models.SeasonScore, len(results))
	for i, result := range results {
		seasonScores[i] = &models.SeasonScore{
			UserID:      result.ID,
			Season:      season,
			TotalPoints: result.TotalPoints,
			UpdatedAt:   time.Now(),
		}
	}

	return seasonScores, nil
}