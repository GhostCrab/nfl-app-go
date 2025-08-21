package services

import (
	"context"
	"log"
	"nfl-app-go/database"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
)

// ChangeStreamWatcher watches MongoDB changes and triggers callbacks
type ChangeStreamWatcher struct {
	db       *database.MongoDB
	onUpdate func()
}

// NewChangeStreamWatcher creates a new change stream watcher
func NewChangeStreamWatcher(db *database.MongoDB, onUpdate func()) *ChangeStreamWatcher {
	return &ChangeStreamWatcher{
		db:       db,
		onUpdate: onUpdate,
	}
}

// StartWatching begins watching for changes in the games collection
func (w *ChangeStreamWatcher) StartWatching() {
	go func() {
		log.Println("ChangeStream: Starting to watch games collection for changes")
		
		collection := w.db.GetCollection("games")
		
		// Create pipeline to watch for all operations on games collection
		pipeline := mongo.Pipeline{}
		
		// Watch for changes with error handling and auto-reconnect
		for {
			ctx := context.Background()
			changeStream, err := collection.Watch(ctx, pipeline)
			if err != nil {
				log.Printf("ChangeStream: Error creating change stream: %v", err)
				time.Sleep(5 * time.Second) // Wait before retrying
				continue
			}

			log.Println("ChangeStream: Successfully connected to games collection")

			// Process change events
			for changeStream.Next(ctx) {
				var event bson.M
				if err := changeStream.Decode(&event); err != nil {
					log.Printf("ChangeStream: Error decoding change event: %v", err)
					continue
				}

				// Log the type of operation
				operationType, ok := event["operationType"].(string)
				if ok {
					log.Printf("ChangeStream: Detected %s operation on games collection", operationType)
				}

				// Trigger update callback for any operation (insert, update, delete, replace)
				if w.onUpdate != nil {
					w.onUpdate()
				}
			}

			// Handle stream errors
			if err := changeStream.Err(); err != nil {
				log.Printf("ChangeStream: Change stream error: %v", err)
			}

			changeStream.Close(ctx)
			log.Println("ChangeStream: Connection closed, attempting to reconnect in 5 seconds...")
			time.Sleep(5 * time.Second)
		}
	}()
}