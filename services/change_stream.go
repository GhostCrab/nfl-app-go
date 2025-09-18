package services

import (
	"context"
	"fmt"
	"nfl-app-go/database"
	"nfl-app-go/logging"
	"strconv"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// ChangeEvent represents a database change event with context
type ChangeEvent struct {
	Collection    string                 `json:"collection"`
	Operation     string                 `json:"operation"`
	Season        int                    `json:"season"`
	Week          int                    `json:"week"`
	GameID        string                 `json:"gameId,omitempty"`
	UserID        int                    `json:"userId,omitempty"`
	UpdatedFields map[string]interface{} `json:"updatedFields,omitempty"` // Fields that changed in update operations
}

// ChangeStreamWatcher watches MongoDB changes and triggers callbacks
type ChangeStreamWatcher struct {
	db       *database.MongoDB
	onUpdate func(event ChangeEvent)
	restart  chan bool // Channel to force restart of change streams
}

// NewChangeStreamWatcher creates a new change stream watcher
func NewChangeStreamWatcher(db *database.MongoDB, onUpdate func(event ChangeEvent)) *ChangeStreamWatcher {
	return &ChangeStreamWatcher{
		db:       db,
		onUpdate: onUpdate,
		restart:  make(chan bool, 1),
	}
}

// ForceRestart forces the change streams to reconnect (useful for config changes)
func (w *ChangeStreamWatcher) ForceRestart() {
	select {
	case w.restart <- true:
		logger := logging.WithPrefix("ChangeStream")
		logger.Info("Force restart requested")
	default:
		// Channel is full, restart already pending
	}
}

// StartWatching begins watching for changes in games, picks, and parlay_scores collections
func (w *ChangeStreamWatcher) StartWatching() {
	// Start watching games collection
	go w.watchCollection("games")
	// Start watching picks collection  
	go w.watchCollection("picks")
	// Start watching parlay scores collection
	go w.watchCollection("parlay_scores")
}

// watchCollection watches a specific collection for changes
func (w *ChangeStreamWatcher) watchCollection(collectionName string) {
	logger := logging.WithPrefix("ChangeStream")
	logger.Infof("Starting to watch %s collection for changes", collectionName)
	
	collection := w.db.GetCollection(collectionName)
	
	// Create pipeline to filter meaningful changes only
	var pipeline mongo.Pipeline
	
	if collectionName == "games" {
		// Only trigger change events for meaningful game field changes
		pipeline = mongo.Pipeline{
			{
				{"$match", bson.M{
					"$or": []bson.M{
						// New games
						{"operationType": "insert"},
						// Updates to meaningful fields only
						{
							"operationType": "update",
							"$or": []bson.M{
								{"updateDescription.updatedFields.state": bson.M{"$exists": true}},
								{"updateDescription.updatedFields.awayScore": bson.M{"$exists": true}},
								{"updateDescription.updatedFields.homeScore": bson.M{"$exists": true}},
								{"updateDescription.updatedFields.quarter": bson.M{"$exists": true}},
								{"updateDescription.updatedFields.status": bson.M{"$exists": true}},
							},
						},
					},
				}},
			},
		}
		logger.Debug("Using filtered pipeline for games collection (meaningful changes only)")
	} else {
		// For other collections, watch all changes
		pipeline = mongo.Pipeline{}
		logger.Debugf("Using default pipeline for %s collection", collectionName)
	}
	
	// Watch for changes with error handling and auto-reconnect
	for {
		ctx := context.Background()
		
		// Configure change stream options to include full document on updates
		opts := options.ChangeStream()
		opts.SetFullDocument(options.UpdateLookup) // Include full document for update operations on all collections
		
		changeStream, err := collection.Watch(ctx, pipeline, opts)
		if err != nil {
			logger.Errorf("Error creating change stream for %s: %v", collectionName, err)
			time.Sleep(5 * time.Second) // Wait before retrying
			continue
		}

		logger.Infof("Successfully connected to %s collection", collectionName)

		// Process change events
		for changeStream.Next(ctx) {
			// Log raw change stream cursor information before decode
			logger.Debugf("Processing change event from %s collection", collectionName)

			var event bson.M
			if err := changeStream.Decode(&event); err != nil {
				logger.Errorf("Error decoding change event from %s: %v", collectionName, err)
				// Log additional context about the failed decode
				logger.Errorf("Failed to decode change event - context: %+v", ctx)
				streamID := changeStream.ID()
				logger.Errorf("Change stream ID: %v", streamID)
				continue
			}

			// Log the successfully decoded event structure for debugging
			logger.Debugf("Successfully decoded change event from %s: %+v", collectionName, event)

			// Extract operation type
			operationType, ok := event["operationType"].(string)
			if !ok {
				continue
			}

			
			// Debug logging for games collection to identify noise
			if collectionName == "games" {
				if operationType == "update" {
					if updateDesc, ok := event["updateDescription"].(bson.M); ok {
						if _, ok := updateDesc["updatedFields"].(bson.M); ok {
							// Updated fields extracted for change event processing
						}
					}
				} else if operationType == "replace" {
					if fullDoc, ok := event["fullDocument"].(bson.M); ok {
						if gameState, ok := fullDoc["state"].(string); ok {
							if gameID, ok := fullDoc["id"].(int32); ok {
								logger.Debugf("Game %d replace operation, state=%s", gameID, gameState)
							}
						}
					}
				}
			}

			// Create change event with extracted information
			changeEvent := w.extractChangeInfo(event, collectionName, operationType)
			
			// For update operations, extract the updated fields for more detailed SSE events
			if operationType == "update" && collectionName == "games" {
				if updateDesc, ok := event["updateDescription"].(bson.M); ok {
					if updatedFields, ok := updateDesc["updatedFields"].(bson.M); ok {
						changeEvent.UpdatedFields = make(map[string]interface{})
						for key, value := range updatedFields {
							changeEvent.UpdatedFields[key] = value
						}
					}
				}
			}

			// Log concise change summary for monitoring
			if changeEvent.Collection == "games" && changeEvent.GameID != "" {
				fieldNames := make([]string, 0)
				if changeEvent.UpdatedFields != nil {
					for field := range changeEvent.UpdatedFields {
						fieldNames = append(fieldNames, field)
					}
				}
				
				// Get human-readable game description from the document
				gameDesc := fmt.Sprintf("Game %s", changeEvent.GameID) // fallback
				if fullDoc, ok := event["fullDocument"].(bson.M); ok {
					if week, hasWeek := fullDoc["week"].(int32); hasWeek {
						if away, hasAway := fullDoc["away"].(string); hasAway {
							if home, hasHome := fullDoc["home"].(string); hasHome {
								gameDesc = fmt.Sprintf("WEEK %d %s @ %s", week, away, home)
							}
						}
					}
				}
				
				logger.Debugf("%s updated - fields: %v", gameDesc, fieldNames)
			}
			
			// Trigger update callback
			if w.onUpdate != nil {
				w.onUpdate(changeEvent)
			}
		}

		// Handle stream errors
		if err := changeStream.Err(); err != nil {
			logger.Errorf("Change stream error for %s: %v", collectionName, err)
		}

		changeStream.Close(ctx)
		logger.Warnf("Connection to %s closed, attempting to reconnect in 5 seconds...", collectionName)
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
		// With UpdateLookup enabled, fullDocument should always be available for updates
		if fullDoc, ok := event["fullDocument"].(bson.M); ok {
			doc = fullDoc
		} else {
			// This should rarely happen now that UpdateLookup is enabled for all collections
			debugLogger := logging.WithPrefix("ChangeStream")
			debugLogger.Warnf("Missing fullDocument for %s update operation, this is unexpected", collection)
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
			} else if gameID, ok := doc["id"].(int32); ok {
				changeEvent.GameID = fmt.Sprintf("%d", gameID)
			}
		} else if collection == "picks" {
			// Extract season, week, user_id, and game_id from picks document
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
			} else if gameID, ok := doc["game_id"].(int32); ok {
				changeEvent.GameID = fmt.Sprintf("%d", gameID)
			}
		} else if collection == "parlay_scores" {
			// For parlay_scores, extract season and user_id from document root
			if season, ok := doc["season"].(int32); ok {
				changeEvent.Season = int(season)
			}
			if userID, ok := doc["user_id"].(int32); ok {
				changeEvent.UserID = int(userID)
			}

			// For updates, extract week from the updated week_scores keys in updateDescription
			if operation == "update" {
				if updateDesc, ok := event["updateDescription"].(bson.M); ok {
					if updatedFields, ok := updateDesc["updatedFields"].(bson.M); ok {
						if weekScores, ok := updatedFields["week_scores"].(bson.M); ok {
							// Extract week numbers from week_scores keys (e.g., "1", "2")
							for weekKey := range weekScores {
								if weekNum, err := strconv.Atoi(weekKey); err == nil {
									changeEvent.Week = weekNum
									break // Use first week found
								}
							}
						}
					}
				}
			}
			// Note: parlay_scores documents don't have a direct week field at root level
		}
	}

	return changeEvent
}