package database

import (
	"context"
	"fmt"
	"nfl-app-go/logging"
	"time"

	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

type Config struct {
	Host     string
	Port     string
	Username string
	Password string
	Database string
}

type MongoDB struct {
	client   *mongo.Client
	database *mongo.Database
}

func NewMongoConnection(config Config) (*MongoDB, error) {
	logger := logging.WithPrefix("MongoDB")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	var uri string
	if config.Username != "" && config.Password != "" {
		uri = fmt.Sprintf("mongodb://%s:%s@%s:%s/%s?authSource=%s",
			config.Username, config.Password, config.Host, config.Port, config.Database, config.Database)
		logger.Infof("Connecting with authentication as user: %s", config.Username)
	} else {
		uri = fmt.Sprintf("mongodb://%s:%s/%s",
			config.Host, config.Port, config.Database)
		logger.Info("Connecting without authentication")
	}

	logger.Debugf("Connection URI: %s", uri)

	client, err := mongo.Connect(ctx, options.Client().ApplyURI(uri))
	if err != nil {
		logger.Errorf("Failed to connect: %v", err)
		return nil, fmt.Errorf("failed to connect to MongoDB: %w", err)
	}

	// Verify connection
	if err := client.Ping(ctx, nil); err != nil {
		logger.Errorf("Failed to ping: %v", err)
		return nil, fmt.Errorf("failed to ping MongoDB: %w", err)
	}

	database := client.Database(config.Database)
	logger.Infof("Successfully connected to %s:%s database=%s", config.Host, config.Port, config.Database)

	return &MongoDB{
		client:   client,
		database: database,
	}, nil
}

func (m *MongoDB) Close() error {
	logger := logging.WithPrefix("MongoDB")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err := m.client.Disconnect(ctx)
	if err != nil {
		logger.Errorf("Error disconnecting: %v", err)
	} else {
		logger.Info("Connection closed successfully")
	}
	return err
}

func (m *MongoDB) TestConnection() error {
	logger := logging.WithPrefix("MongoDB")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err := m.client.Ping(ctx, nil)
	if err != nil {
		logger.Errorf("Ping test failed: %v", err)
		return fmt.Errorf("MongoDB ping failed: %w", err)
	}

	logger.Debug("Ping test successful")
	return nil
}

func (m *MongoDB) GetCollection(name string) *mongo.Collection {
	return m.database.Collection(name)
}
