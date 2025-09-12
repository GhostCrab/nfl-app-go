package database

import (
	"context"
	"fmt"
	"nfl-app-go/models"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
)

// FindByID retrieves a user by their ID with context support
// This method supports the new service architecture pattern
func (r *MongoUserRepository) FindByID(ctx context.Context, userID int) (*models.User, error) {
	ctx, cancel := WithShortTimeout()
	defer cancel()

	var user models.User
	filter := bson.M{"id": userID}
	
	err := r.collection.FindOne(ctx, filter).Decode(&user)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return nil, fmt.Errorf("user with ID %d not found", userID)
		}
		return nil, fmt.Errorf("failed to find user by ID %d: %w", userID, err)
	}

	return &user, nil
}