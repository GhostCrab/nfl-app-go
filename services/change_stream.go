package services

import (
	"context"
	"log"
	"nfl-app-go/database"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
)

// ChangeEvent represents a database change event with context
type ChangeEvent struct {
	Collection string
	Operation  string
	Season     int
	Week       int
	GameID     string
	UserID     int
}

// ChangeStreamWatcher watches MongoDB changes and triggers callbacks
type ChangeStreamWatcher struct {
	db       *database.MongoDB
	onUpdate func(event ChangeEvent)
}

// NewChangeStreamWatcher creates a new change stream watcher
func NewChangeStreamWatcher(db *database.MongoDB, onUpdate func(event ChangeEvent)) *ChangeStreamWatcher {
	return &ChangeStreamWatcher{
		db:       db,
		onUpdate: onUpdate,
	}
}

// StartWatching begins watching for changes in games and picks collections
func (w *ChangeStreamWatcher) StartWatching() {
	// Start watching games collection
	go w.watchCollection("games")
	// Start watching picks collection  
	go w.watchCollection("picks")
}

// watchCollection watches a specific collection for changes
func (w *ChangeStreamWatcher) watchCollection(collectionName string) {
	log.Printf("ChangeStream: Starting to watch %s collection for changes", collectionName)
	
	collection := w.db.GetCollection(collectionName)
	
	// Create pipeline to watch for all operations
	pipeline := mongo.Pipeline{}
	
	// Watch for changes with error handling and auto-reconnect
	for {
		ctx := context.Background()
		changeStream, err := collection.Watch(ctx, pipeline)
		if err != nil {
			log.Printf("ChangeStream: Error creating change stream for %s: %v", collectionName, err)
			time.Sleep(5 * time.Second) // Wait before retrying
			continue
		}

		log.Printf("ChangeStream: Successfully connected to %s collection", collectionName)

		// Process change events
		for changeStream.Next(ctx) {
			var event bson.M
			if err := changeStream.Decode(&event); err != nil {
				log.Printf("ChangeStream: Error decoding change event from %s: %v", collectionName, err)
				continue
			}

			// Extract operation type
			operationType, ok := event["operationType"].(string)
			if !ok {
				continue
			}

			log.Printf("ChangeStream: Detected %s operation on %s collection", operationType, collectionName)

			// Create change event with extracted information
			changeEvent := w.extractChangeInfo(event, collectionName, operationType)

			// Trigger update callback
			if w.onUpdate != nil {
				w.onUpdate(changeEvent)
			}
		}

		// Handle stream errors
		if err := changeStream.Err(); err != nil {
			log.Printf("ChangeStream: Change stream error for %s: %v", collectionName, err)
		}

		changeStream.Close(ctx)
		log.Printf("ChangeStream: Connection to %s closed, attempting to reconnect in 5 seconds...", collectionName)
		time.Sleep(5 * time.Second)
	}
}

// extractChangeInfo extracts relevant information from a change stream event
func (w *ChangeStreamWatcher) extractChangeInfo(event bson.M, collection, operation string) ChangeEvent {
	changeEvent := ChangeEvent{
		Collection: collection,
		Operation:  operation,
	}

	// Extract document information based on operation type
	var doc bson.M
	if operation == "insert" || operation == "replace" {
		if fullDoc, ok := event["fullDocument"].(bson.M); ok {
			doc = fullDoc
		}
	} else if operation == "update" {
		if fullDoc, ok := event["fullDocument"].(bson.M); ok {
			doc = fullDoc
		}
	} else if operation == "delete" {
		if docKey, ok := event["documentKey"].(bson.M); ok {
			doc = docKey
		}
	}

	if doc != nil {
		// Extract common fields based on collection
		if collection == "games" {
			if season, ok := doc["season"].(int32); ok {
				changeEvent.Season = int(season)
			}
			if week, ok := doc["week"].(int32); ok {
				changeEvent.Week = int(week)
			}
			if gameID, ok := doc["_id"].(string); ok {
				changeEvent.GameID = gameID
			} else if gameID, ok := doc["id"].(string); ok {
				changeEvent.GameID = gameID
			}
		} else if collection == "picks" {
			if season, ok := doc["season"].(int32); ok {
				changeEvent.Season = int(season)
			}
			if week, ok := doc["week"].(int32); ok {
				changeEvent.Week = int(week)
			}
			if userID, ok := doc["user_id"].(int32); ok {
				changeEvent.UserID = int(userID)
			}
			if gameID, ok := doc["game_id"].(string); ok {
				changeEvent.GameID = gameID
			}
		}
	}

	return changeEvent
}