package database

import (
	"context"
	"fmt"
	"log"
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
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	var uri string
	if config.Username != "" && config.Password != "" {
		uri = fmt.Sprintf("mongodb://%s:%s@%s:%s/%s?authSource=%s",
			config.Username, config.Password, config.Host, config.Port, config.Database, config.Database)
		log.Printf("Connecting to MongoDB with authentication as user: %s", config.Username)
	} else {
		uri = fmt.Sprintf("mongodb://%s:%s/%s",
			config.Host, config.Port, config.Database)
		log.Printf("Connecting to MongoDB without authentication")
	}
	
	log.Printf("MongoDB URI: %s", uri)

	client, err := mongo.Connect(ctx, options.Client().ApplyURI(uri))
	if err != nil {
		return nil, fmt.Errorf("failed to connect to MongoDB: %w", err)
	}

	// Verify connection
	if err := client.Ping(ctx, nil); err != nil {
		return nil, fmt.Errorf("failed to ping MongoDB: %w", err)
	}

	database := client.Database(config.Database)
	log.Printf("Successfully connected to MongoDB at %s:%s", config.Host, config.Port)

	return &MongoDB{
		client:   client,
		database: database,
	}, nil
}

func (m *MongoDB) Close() error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return m.client.Disconnect(ctx)
}

func (m *MongoDB) TestConnection() error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err := m.client.Ping(ctx, nil)
	if err != nil {
		return fmt.Errorf("MongoDB ping failed: %w", err)
	}
	
	log.Println("MongoDB connection test successful")
	return nil
}

func (m *MongoDB) GetCollection(name string) *mongo.Collection {
	return m.database.Collection(name)
}