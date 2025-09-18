// DEPRECATED: This file is deprecated as of the parlay scoring refactor.
// Parlay scores are now calculated in-memory using MemoryParlayScorer.
// This file is kept only for legacy compatibility and will be removed in a future cleanup.

package database

import (
	"context"
	"fmt"
	"log"
	"nfl-app-go/models"
	"runtime"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// GetUserSeasonRecord retrieves a user's season record for parlay scores
// This method supports parlay service analytics operations
func (r *MongoParlayRepository) GetUserSeasonRecord(ctx context.Context, userID, season int) (*models.ParlaySeasonRecord, error) {
	ctx, cancel := WithMediumTimeout()
	defer cancel()

	var record models.ParlaySeasonRecord
	filter := bson.M{
		"user_id": userID,
		"season":  season,
	}
	
	err := r.collection.FindOne(ctx, filter).Decode(&record)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return nil, nil // No record found, not an error
		}
		return nil, fmt.Errorf("failed to find parlay season record for user %d, season %d: %w", userID, season, err)
	}

	return &record, nil
}

// UpsertUserSeasonRecord creates or updates a user's season record for parlay scores
// This method supports parlay service scoring operations
func (r *MongoParlayRepository) UpsertUserSeasonRecord(ctx context.Context, record *models.ParlaySeasonRecord) error {
	// DEBUG: Log season record operations
	log.Printf("PARLAY_DEBUG: UpsertUserSeasonRecord called - UserID=%d, Season=%d",
		record.UserID, record.Season)

	// Check for invalid data
	if record.Season == 0 {
		log.Printf("PARLAY_ERROR: Invalid UpsertUserSeasonRecord data - Season=%d, UserID=%d",
			record.Season, record.UserID)

		// Print stack trace to identify caller
		buf := make([]byte, 4096)
		n := runtime.Stack(buf, false)
		log.Printf("PARLAY_ERROR: UpsertUserSeasonRecord stack trace:\n%s", buf[:n])
	}

	ctx, cancel := WithMediumTimeout()
	defer cancel()

	filter := bson.M{
		"user_id": record.UserID,
		"season":  record.Season,
	}
	
	// Update timestamps
	now := time.Now()
	if record.CreatedAt.IsZero() {
		record.CreatedAt = now
	}
	record.UpdatedAt = now
	
	// Upsert operation - update if exists, insert if not
	update := bson.M{"$set": record}
	opts := options.Update().SetUpsert(true)
	
	result, err := r.collection.UpdateOne(ctx, filter, update, opts)
	if err != nil {
		return fmt.Errorf("failed to upsert parlay season record for user %d, season %d: %w", record.UserID, record.Season, err)
	}
	
	// Set the ID if this was an insert
	if result.UpsertedCount > 0 && result.UpsertedID != nil {
		if oid, ok := result.UpsertedID.(primitive.ObjectID); ok {
			record.ID = oid
		}
	}
	
	return nil
}

// CountUsersWithScoresForWeek counts how many users have parlay scores for a specific week
// This method supports parlay service analytics operations
func (r *MongoParlayRepository) CountUsersWithScoresForWeek(ctx context.Context, season, week int) (int64, error) {
	ctx, cancel := WithMediumTimeout()
	defer cancel()

	filter := bson.M{
		"season": season,
		"week":   week,
	}
	
	count, err := r.collection.CountDocuments(ctx, filter)
	if err != nil {
		return 0, fmt.Errorf("failed to count users with parlay scores for season %d, week %d: %w", season, week, err)
	}
	
	return count, nil
}

// GetWeeklyScoresForSeason retrieves all weekly parlay scores for a season
// This method supports parlay service reporting operations
func (r *MongoParlayRepository) GetWeeklyScoresForSeason(ctx context.Context, season int) ([]models.ParlayScore, error) {
	ctx, cancel := WithLongTimeout() // Potentially large result set
	defer cancel()

	filter := bson.M{"season": season}
	
	// Sort by week, then by total points descending for leaderboard order
	opts := options.Find().SetSort(bson.D{
		{Key: "week", Value: 1},
		{Key: "total_points", Value: -1},
		{Key: "user_id", Value: 1},
	})
	
	cursor, err := r.collection.Find(ctx, filter, opts)
	if err != nil {
		return nil, fmt.Errorf("failed to find weekly parlay scores for season %d: %w", season, err)
	}
	defer cursor.Close(ctx)
	
	var scores []models.ParlayScore
	for cursor.Next(ctx) {
		var score models.ParlayScore
		if err := cursor.Decode(&score); err != nil {
			return nil, fmt.Errorf("failed to decode parlay score: %w", err)
		}
		scores = append(scores, score)
	}
	
	return scores, nil
}

// GetTopScorersForWeek retrieves the top parlay scorers for a specific week
// This method supports parlay service leaderboard operations
func (r *MongoParlayRepository) GetTopScorersForWeek(ctx context.Context, season, week, limit int) ([]models.ParlayScore, error) {
	ctx, cancel := WithMediumTimeout()
	defer cancel()

	filter := bson.M{
		"season": season,
		"week":   week,
	}
	
	// Sort by total points descending for leaderboard
	opts := options.Find().
		SetSort(bson.D{{Key: "total_points", Value: -1}}).
		SetLimit(int64(limit))
	
	cursor, err := r.collection.Find(ctx, filter, opts)
	if err != nil {
		return nil, fmt.Errorf("failed to find top parlay scorers for season %d, week %d: %w", season, week, err)
	}
	defer cursor.Close(ctx)
	
	var scores []models.ParlayScore
	for cursor.Next(ctx) {
		var score models.ParlayScore
		if err := cursor.Decode(&score); err != nil {
			return nil, fmt.Errorf("failed to decode parlay score: %w", err)
		}
		scores = append(scores, score)
	}
	
	return scores, nil
}

// GetUserWeeklyScoresForSeason retrieves all weekly parlay scores for a specific user and season
// This method supports parlay service user analytics operations
func (r *MongoParlayRepository) GetUserWeeklyScoresForSeason(ctx context.Context, userID, season int) ([]models.ParlayScore, error) {
	ctx, cancel := WithMediumTimeout()
	defer cancel()

	filter := bson.M{
		"user_id": userID,
		"season":  season,
	}
	
	// Sort by week for chronological order
	opts := options.Find().SetSort(bson.D{{Key: "week", Value: 1}})
	
	cursor, err := r.collection.Find(ctx, filter, opts)
	if err != nil {
		return nil, fmt.Errorf("failed to find weekly parlay scores for user %d, season %d: %w", userID, season, err)
	}
	defer cursor.Close(ctx)
	
	var scores []models.ParlayScore
	for cursor.Next(ctx) {
		var score models.ParlayScore
		if err := cursor.Decode(&score); err != nil {
			return nil, fmt.Errorf("failed to decode parlay score: %w", err)
		}
		scores = append(scores, score)
	}
	
	return scores, nil
}

// UpdateWeekScore updates a specific week's score in a user's season record
// This method supports parlay service score management operations
func (r *MongoParlayRepository) UpdateWeekScore(ctx context.Context, userID, season, week int, weekScore models.ParlayWeekScore) error {
	// DEBUG: Log every week score update to identify bad data sources
	log.Printf("PARLAY_DEBUG: UpdateWeekScore called - UserID=%d, Season=%d, Week=%d",
		userID, season, week)

	// Check for invalid data
	if season == 0 || week == 0 {
		log.Printf("PARLAY_ERROR: Invalid UpdateWeekScore data - Season=%d, Week=%d, UserID=%d",
			season, week, userID)

		// Print stack trace to identify caller
		buf := make([]byte, 4096)
		n := runtime.Stack(buf, false)
		log.Printf("PARLAY_ERROR: UpdateWeekScore stack trace:\n%s", buf[:n])
	}

	ctx, cancel := WithMediumTimeout()
	defer cancel()

	filter := bson.M{
		"user_id": userID,
		"season":  season,
	}
	
	// Use dot notation to update the specific week in the week_scores map
	update := bson.M{
		"$set": bson.M{
			fmt.Sprintf("week_scores.%d", week): weekScore,
			"updated_at": time.Now(),
		},
	}
	
	result, err := r.collection.UpdateOne(ctx, filter, update)
	if err != nil {
		return fmt.Errorf("failed to update week score for user %d, season %d, week %d: %w", userID, season, week, err)
	}
	
	if result.MatchedCount == 0 {
		return fmt.Errorf("no parlay season record found for user %d, season %d", userID, season)
	}
	
	return nil
}