package database

import (
	"context"
	"fmt"
	"nfl-app-go/logging"
	"nfl-app-go/models"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// MongoWeeklyPicksRepository handles weekly pick storage operations
type MongoWeeklyPicksRepository struct {
	collection *mongo.Collection
	database   *mongo.Database
}

// NewMongoWeeklyPicksRepository creates a new MongoDB weekly picks repository
func NewMongoWeeklyPicksRepository(db *MongoDB) *MongoWeeklyPicksRepository {
	collection := db.GetCollection("weekly_picks")

	// Create indexes for efficient querying
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Compound index on user_id, season, week (unique constraint)
	indexModel := mongo.IndexModel{
		Keys:    bson.D{{Key: "user_id", Value: 1}, {Key: "season", Value: 1}, {Key: "week", Value: 1}},
		Options: options.Index().SetUnique(true),
	}
	collection.Indexes().CreateOne(ctx, indexModel)

	// Index on season and week for efficient queries
	seasonWeekIndex := mongo.IndexModel{
		Keys: bson.D{{Key: "season", Value: 1}, {Key: "week", Value: 1}},
	}
	collection.Indexes().CreateOne(ctx, seasonWeekIndex)

	return &MongoWeeklyPicksRepository{
		collection: collection,
		database:   db.database,
	}
}

// Upsert creates or updates a WeeklyPicks document
func (r *MongoWeeklyPicksRepository) Upsert(ctx context.Context, weeklyPicks *models.WeeklyPicks) error {
	weeklyPicks.UpdatedAt = time.Now()
	if weeklyPicks.CreatedAt.IsZero() {
		weeklyPicks.CreatedAt = weeklyPicks.UpdatedAt
	}

	filter := bson.M{
		"user_id": weeklyPicks.UserID,
		"season":  weeklyPicks.Season,
		"week":    weeklyPicks.Week,
	}

	update := bson.M{
		"$set": bson.M{
			"user_id":    weeklyPicks.UserID,
			"season":     weeklyPicks.Season,
			"week":       weeklyPicks.Week,
			"picks":      weeklyPicks.Picks,
			"updated_at": weeklyPicks.UpdatedAt,
		},
		"$setOnInsert": bson.M{
			"created_at": weeklyPicks.CreatedAt,
		},
	}

	opts := options.Update().SetUpsert(true)
	result, err := r.collection.UpdateOne(ctx, filter, update, opts)
	if err != nil {
		return fmt.Errorf("failed to upsert weekly picks: %w", err)
	}

	// Set the ID if this was an insert
	if result.UpsertedID != nil {
		if oid, ok := result.UpsertedID.(primitive.ObjectID); ok {
			weeklyPicks.ID = oid
		}
	}

	return nil
}

// FindByUserAndWeek retrieves weekly picks for a specific user, season, and week
func (r *MongoWeeklyPicksRepository) FindByUserAndWeek(ctx context.Context, userID, season, week int) (*models.WeeklyPicks, error) {
	filter := bson.M{
		"user_id": userID,
		"season":  season,
		"week":    week,
	}

	var weeklyPicks models.WeeklyPicks
	err := r.collection.FindOne(ctx, filter).Decode(&weeklyPicks)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return nil, nil // No picks found, return nil (not an error)
		}
		return nil, fmt.Errorf("failed to find weekly picks: %w", err)
	}

	return &weeklyPicks, nil
}

// FindAllByWeek retrieves all users' weekly picks for a specific season and week
func (r *MongoWeeklyPicksRepository) FindAllByWeek(ctx context.Context, season, week int) ([]*models.WeeklyPicks, error) {
	filter := bson.M{
		"season": season,
		"week":   week,
	}

	cursor, err := r.collection.Find(ctx, filter)
	if err != nil {
		return nil, fmt.Errorf("failed to find weekly picks by week: %w", err)
	}
	defer cursor.Close(ctx)

	var weeklyPicksList []*models.WeeklyPicks
	for cursor.Next(ctx) {
		var weeklyPicks models.WeeklyPicks
		if err := cursor.Decode(&weeklyPicks); err != nil {
			return nil, fmt.Errorf("failed to decode weekly picks: %w", err)
		}
		weeklyPicksList = append(weeklyPicksList, &weeklyPicks)
	}

	return weeklyPicksList, nil
}

// FindBySeason retrieves all weekly picks for a specific season
func (r *MongoWeeklyPicksRepository) FindBySeason(ctx context.Context, season int) ([]*models.WeeklyPicks, error) {
	filter := bson.M{"season": season}

	cursor, err := r.collection.Find(ctx, filter)
	if err != nil {
		return nil, fmt.Errorf("failed to find weekly picks by season: %w", err)
	}
	defer cursor.Close(ctx)

	var weeklyPicksList []*models.WeeklyPicks
	for cursor.Next(ctx) {
		var weeklyPicks models.WeeklyPicks
		if err := cursor.Decode(&weeklyPicks); err != nil {
			return nil, fmt.Errorf("failed to decode weekly picks: %w", err)
		}
		weeklyPicksList = append(weeklyPicksList, &weeklyPicks)
	}

	return weeklyPicksList, nil
}

// FindByUserAndSeason retrieves all weekly picks for a user in a specific season
func (r *MongoWeeklyPicksRepository) FindByUserAndSeason(ctx context.Context, userID, season int) ([]*models.WeeklyPicks, error) {
	filter := bson.M{
		"user_id": userID,
		"season":  season,
	}

	cursor, err := r.collection.Find(ctx, filter)
	if err != nil {
		return nil, fmt.Errorf("failed to find weekly picks by user and season: %w", err)
	}
	defer cursor.Close(ctx)

	var weeklyPicksList []*models.WeeklyPicks
	for cursor.Next(ctx) {
		var weeklyPicks models.WeeklyPicks
		if err := cursor.Decode(&weeklyPicks); err != nil {
			return nil, fmt.Errorf("failed to decode weekly picks: %w", err)
		}
		weeklyPicksList = append(weeklyPicksList, &weeklyPicks)
	}

	return weeklyPicksList, nil
}

// Delete removes weekly picks for a specific user, season, and week
func (r *MongoWeeklyPicksRepository) Delete(ctx context.Context, userID, season, week int) error {
	filter := bson.M{
		"user_id": userID,
		"season":  season,
		"week":    week,
	}

	result, err := r.collection.DeleteOne(ctx, filter)
	if err != nil {
		return fmt.Errorf("failed to delete weekly picks: %w", err)
	}

	if result.DeletedCount == 0 {
		return fmt.Errorf("no weekly picks found to delete")
	}

	return nil
}

// Count returns the total number of weekly picks documents
func (r *MongoWeeklyPicksRepository) Count(ctx context.Context) (int64, error) {
	return r.collection.CountDocuments(ctx, bson.M{})
}

// CountBySeasonAndWeek returns the number of users who have submitted picks for a specific season and week
func (r *MongoWeeklyPicksRepository) CountBySeasonAndWeek(ctx context.Context, season, week int) (int64, error) {
	filter := bson.M{
		"season": season,
		"week":   week,
	}
	return r.collection.CountDocuments(ctx, filter)
}

// UpdatePickResults updates the results for specific picks within weekly pick documents
// This is used when games complete and pick results need to be calculated
func (r *MongoWeeklyPicksRepository) UpdatePickResults(ctx context.Context, season, week, gameID int, updates map[int]models.PickResult) error {
	// Update all weekly pick documents that have picks for this game
	filter := bson.M{
		"season":        season,
		"week":          week,
		"picks.game_id": gameID,
	}

	// Build update operations for each user
	var operations []mongo.WriteModel

	cursor, err := r.collection.Find(ctx, filter)
	if err != nil {
		return fmt.Errorf("failed to find weekly picks for result update: %w", err)
	}
	defer cursor.Close(ctx)

	for cursor.Next(ctx) {
		var weeklyPicks models.WeeklyPicks
		if err := cursor.Decode(&weeklyPicks); err != nil {
			continue
		}

		// Check if this user has a pick result to update
		if newResult, hasUpdate := updates[weeklyPicks.UserID]; hasUpdate {
			// Update the specific pick's result
			for i := range weeklyPicks.Picks {
				if weeklyPicks.Picks[i].GameID == gameID {
					weeklyPicks.Picks[i].Result = newResult
					break
				}
			}

			// Create update operation
			updateFilter := bson.M{
				"user_id": weeklyPicks.UserID,
				"season":  season,
				"week":    week,
			}
			updateDoc := bson.M{
				"$set": bson.M{
					"picks":      weeklyPicks.Picks,
					"updated_at": time.Now(),
				},
			}
			updateOp := mongo.NewUpdateOneModel().SetFilter(updateFilter).SetUpdate(updateDoc)
			operations = append(operations, updateOp)
		}
	}

	// Execute bulk update if we have operations
	if len(operations) > 0 {
		opts := options.BulkWrite().SetOrdered(false)
		_, err := r.collection.BulkWrite(ctx, operations, opts)
		if err != nil {
			return fmt.Errorf("failed to bulk update pick results: %w", err)
		}
	}

	return nil
}

// UpdateIndividualPickResults updates specific picks individually based on game, user, and pick type
// This handles the case where users have multiple picks per game (spread, over/under)
func (r *MongoWeeklyPicksRepository) UpdateIndividualPickResults(ctx context.Context, season, week, gameID int, pickUpdates []PickUpdate) error {
	if len(pickUpdates) == 0 {
		return nil
	}

	var operations []mongo.WriteModel

	for _, update := range pickUpdates {
		// Update specific pick using arrayFilters for precise targeting
		// This allows updating multiple picks in the same document for the same game
		filter := bson.M{
			"user_id": update.UserID,
			"season":  season,
			"week":    week,
		}

		updateDoc := bson.M{
			"$set": bson.M{
				"picks.$[elem].result": update.Result,
				"updated_at":           time.Now(),
			},
		}

		// ArrayFilters allow us to specify exactly which array element to update
		arrayFilters := options.ArrayFilters{
			Filters: []interface{}{
				bson.M{
					"elem.game_id":   gameID,
					"elem.pick_type": update.PickType,
				},
			},
		}

		logging.Debugf("Building update for game %d, user %d, type %s, result %s",
			gameID, update.UserID, update.PickType, update.Result)

		updateOp := mongo.NewUpdateOneModel().
			SetFilter(filter).
			SetUpdate(updateDoc).
			SetArrayFilters(arrayFilters)
		operations = append(operations, updateOp)
	}

	if len(operations) > 0 {
		opts := options.BulkWrite().SetOrdered(false)
		result, err := r.collection.BulkWrite(ctx, operations, opts)
		if err != nil {
			return fmt.Errorf("failed to bulk update individual pick results: %w", err)
		}

		logging.Infof("Bulk update result for game %d: ModifiedCount=%d, MatchedCount=%d, UpsertedCount=%d",
			gameID, result.ModifiedCount, result.MatchedCount, result.UpsertedCount)

		// Log individual operation results if any failed
		if result.ModifiedCount < int64(len(operations)) {
			logging.Warnf("Some updates failed for game %d: Expected %d, Modified %d",
				gameID, len(operations), result.ModifiedCount)
		}
	}

	return nil
}

// PickUpdate represents an individual pick result update
type PickUpdate struct {
	UserID   int
	PickType string
	Result   models.PickResult
}
