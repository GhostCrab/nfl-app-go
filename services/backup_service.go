package services

import (
	"context"
	"encoding/json"
	"fmt"
	"nfl-app-go/database"
	"nfl-app-go/logging"
	"os"
	"path/filepath"
	"time"

	"go.mongodb.org/mongo-driver/bson"
)

type BackupService struct {
	db           *database.MongoDB
	backupDir    string
	logger       *logging.Logger
	collections  []string
}

type BackupConfig struct {
	BackupDir   string
	Collections []string
}

func NewBackupService(db *database.MongoDB, config BackupConfig) *BackupService {
	logger := logging.WithPrefix("BackupService")

	// Default collections if none specified
	collections := config.Collections
	if len(collections) == 0 {
		collections = []string{"picks", "games"}
	}

	return &BackupService{
		db:          db,
		backupDir:   config.BackupDir,
		logger:      logger,
		collections: collections,
	}
}

func (bs *BackupService) CreateBackup() error {
	timestamp := time.Now().Format("2006-01-02_15-04-05")
	backupPath := filepath.Join(bs.backupDir, fmt.Sprintf("backup_%s", timestamp))

	bs.logger.Infof("Starting backup to %s", backupPath)

	// Create backup directory
	if err := os.MkdirAll(backupPath, 0755); err != nil {
		return fmt.Errorf("failed to create backup directory: %w", err)
	}

	// Backup each collection
	for _, collectionName := range bs.collections {
		if err := bs.backupCollection(collectionName, backupPath); err != nil {
			bs.logger.Errorf("Failed to backup collection %s: %v", collectionName, err)
			return fmt.Errorf("failed to backup collection %s: %w", collectionName, err)
		}
		bs.logger.Infof("Successfully backed up collection: %s", collectionName)
	}

	// Create backup metadata
	if err := bs.createBackupMetadata(backupPath, timestamp); err != nil {
		bs.logger.Warnf("Failed to create backup metadata: %v", err)
	}

	bs.logger.Infof("Backup completed successfully at %s", backupPath)
	return nil
}

func (bs *BackupService) backupCollection(collectionName string, backupPath string) error {
	collection := bs.db.GetCollection(collectionName)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()

	// Find all documents
	cursor, err := collection.Find(ctx, bson.M{})
	if err != nil {
		return fmt.Errorf("failed to find documents: %w", err)
	}
	defer cursor.Close(ctx)

	// Create output file
	outputFile := filepath.Join(backupPath, fmt.Sprintf("%s.json", collectionName))
	file, err := os.Create(outputFile)
	if err != nil {
		return fmt.Errorf("failed to create output file: %w", err)
	}
	defer file.Close()

	// Write documents as JSON lines
	encoder := json.NewEncoder(file)
	documentCount := 0

	for cursor.Next(ctx) {
		var document bson.M
		if err := cursor.Decode(&document); err != nil {
			return fmt.Errorf("failed to decode document: %w", err)
		}

		if err := encoder.Encode(document); err != nil {
			return fmt.Errorf("failed to encode document: %w", err)
		}
		documentCount++
	}

	if err := cursor.Err(); err != nil {
		return fmt.Errorf("cursor error: %w", err)
	}

	bs.logger.Infof("Backed up %d documents from collection %s", documentCount, collectionName)
	return nil
}

func (bs *BackupService) createBackupMetadata(backupPath string, timestamp string) error {
	metadata := map[string]interface{}{
		"timestamp":   timestamp,
		"created_at":  time.Now().UTC().Format(time.RFC3339),
		"collections": bs.collections,
		"version":     "1.0",
	}

	metadataFile := filepath.Join(backupPath, "metadata.json")
	file, err := os.Create(metadataFile)
	if err != nil {
		return err
	}
	defer file.Close()

	encoder := json.NewEncoder(file)
	encoder.SetIndent("", "  ")
	return encoder.Encode(metadata)
}

func (bs *BackupService) CleanupOldBackups(retentionDays int) error {
	if retentionDays <= 0 {
		bs.logger.Info("Backup cleanup disabled (retention days <= 0)")
		return nil
	}

	bs.logger.Infof("Cleaning up backups older than %d days", retentionDays)

	cutoffTime := time.Now().AddDate(0, 0, -retentionDays)

	entries, err := os.ReadDir(bs.backupDir)
	if err != nil {
		return fmt.Errorf("failed to read backup directory: %w", err)
	}

	deletedCount := 0
	for _, entry := range entries {
		if !entry.IsDir() || !isBackupDir(entry.Name()) {
			continue
		}

		info, err := entry.Info()
		if err != nil {
			bs.logger.Warnf("Failed to get info for %s: %v", entry.Name(), err)
			continue
		}

		if info.ModTime().Before(cutoffTime) {
			backupPath := filepath.Join(bs.backupDir, entry.Name())
			if err := os.RemoveAll(backupPath); err != nil {
				bs.logger.Warnf("Failed to remove old backup %s: %v", backupPath, err)
			} else {
				bs.logger.Infof("Removed old backup: %s", entry.Name())
				deletedCount++
			}
		}
	}

	bs.logger.Infof("Cleanup completed. Removed %d old backups", deletedCount)
	return nil
}

func isBackupDir(name string) bool {
	// Check if directory name matches backup_YYYY-MM-DD_HH-MM-SS pattern
	return len(name) > 7 && name[:7] == "backup_"
}

// StartScheduler starts the backup scheduler that runs nightly backups
func (bs *BackupService) StartScheduler(ctx context.Context, backupTime string, retentionDays int) {
	bs.logger.Infof("Starting backup scheduler. Daily backup at %s, retention: %d days", backupTime, retentionDays)

	go func() {
		ticker := time.NewTicker(1 * time.Hour) // Check every hour
		defer ticker.Stop()

		var lastBackupDate string

		for {
			select {
			case <-ctx.Done():
				bs.logger.Info("Backup scheduler stopped")
				return
			case <-ticker.C:
				now := time.Now()
				currentDate := now.Format("2006-01-02")
				currentTime := now.Format("15:04")

				// Check if it's time for backup and we haven't done it today
				if currentTime >= backupTime && lastBackupDate != currentDate {
					bs.logger.Info("Starting scheduled backup")

					if err := bs.CreateBackup(); err != nil {
						bs.logger.Errorf("Scheduled backup failed: %v", err)
					} else {
						lastBackupDate = currentDate

						// Run cleanup after successful backup
						if err := bs.CleanupOldBackups(retentionDays); err != nil {
							bs.logger.Errorf("Backup cleanup failed: %v", err)
						}
					}
				}
			}
		}
	}()
}

// RestoreBackup restores a specific backup by timestamp
func (bs *BackupService) RestoreBackup(timestamp string, collections []string) error {
	backupPath := filepath.Join(bs.backupDir, fmt.Sprintf("backup_%s", timestamp))

	// Check if backup exists
	if _, err := os.Stat(backupPath); os.IsNotExist(err) {
		return fmt.Errorf("backup not found: %s", backupPath)
	}

	// Load and validate metadata
	metadata, err := bs.loadBackupMetadata(backupPath)
	if err != nil {
		bs.logger.Warnf("Could not load backup metadata: %v", err)
	} else {
		bs.logger.Infof("Restoring backup from %s", metadata["created_at"])
	}

	// Default to all collections if none specified
	if len(collections) == 0 {
		collections = bs.collections
	}

	bs.logger.Infof("Starting restore from %s", backupPath)

	// Restore each collection
	for _, collectionName := range collections {
		if err := bs.restoreCollection(collectionName, backupPath); err != nil {
			bs.logger.Errorf("Failed to restore collection %s: %v", collectionName, err)
			return fmt.Errorf("failed to restore collection %s: %w", collectionName, err)
		}
		bs.logger.Infof("Successfully restored collection: %s", collectionName)
	}

	bs.logger.Infof("Restore completed successfully from %s", backupPath)
	return nil
}

func (bs *BackupService) restoreCollection(collectionName string, backupPath string) error {
	collection := bs.db.GetCollection(collectionName)
	backupFile := filepath.Join(backupPath, fmt.Sprintf("%s.json", collectionName))

	// Check if backup file exists
	if _, err := os.Stat(backupFile); os.IsNotExist(err) {
		return fmt.Errorf("backup file not found: %s", backupFile)
	}

	// Open backup file
	file, err := os.Open(backupFile)
	if err != nil {
		return fmt.Errorf("failed to open backup file: %w", err)
	}
	defer file.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()

	// Clear existing collection (WARNING: This deletes all current data!)
	bs.logger.Warnf("CLEARING collection %s before restore", collectionName)
	if _, err := collection.DeleteMany(ctx, bson.M{}); err != nil {
		return fmt.Errorf("failed to clear collection: %w", err)
	}

	// Read and insert documents
	decoder := json.NewDecoder(file)
	documentCount := 0
	var documents []interface{}

	for {
		var document bson.M
		if err := decoder.Decode(&document); err != nil {
			if err.Error() == "EOF" {
				break
			}
			return fmt.Errorf("failed to decode document: %w", err)
		}
		documents = append(documents, document)
		documentCount++

		// Insert in batches of 1000
		if len(documents) >= 1000 {
			if _, err := collection.InsertMany(ctx, documents); err != nil {
				return fmt.Errorf("failed to insert batch: %w", err)
			}
			documents = documents[:0] // Clear slice
		}
	}

	// Insert remaining documents
	if len(documents) > 0 {
		if _, err := collection.InsertMany(ctx, documents); err != nil {
			return fmt.Errorf("failed to insert final batch: %w", err)
		}
	}

	bs.logger.Infof("Restored %d documents to collection %s", documentCount, collectionName)
	return nil
}

func (bs *BackupService) loadBackupMetadata(backupPath string) (map[string]interface{}, error) {
	metadataFile := filepath.Join(backupPath, "metadata.json")
	file, err := os.Open(metadataFile)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	var metadata map[string]interface{}
	decoder := json.NewDecoder(file)
	if err := decoder.Decode(&metadata); err != nil {
		return nil, err
	}

	return metadata, nil
}

// ListBackups returns available backups with their metadata
func (bs *BackupService) ListBackups() ([]BackupInfo, error) {
	entries, err := os.ReadDir(bs.backupDir)
	if err != nil {
		return nil, fmt.Errorf("failed to read backup directory: %w", err)
	}

	var backups []BackupInfo
	for _, entry := range entries {
		if !entry.IsDir() || !isBackupDir(entry.Name()) {
			continue
		}

		info, err := entry.Info()
		if err != nil {
			bs.logger.Warnf("Failed to get info for %s: %v", entry.Name(), err)
			continue
		}

		// Extract timestamp from directory name (backup_YYYY-MM-DD_HH-MM-SS)
		timestamp := entry.Name()[7:] // Remove "backup_" prefix

		backupPath := filepath.Join(bs.backupDir, entry.Name())
		metadata, err := bs.loadBackupMetadata(backupPath)

		backup := BackupInfo{
			Timestamp:   timestamp,
			CreatedAt:   info.ModTime(),
			Size:        bs.calculateBackupSize(backupPath),
			Collections: bs.collections, // Default to configured collections
		}

		if err == nil {
			if collections, ok := metadata["collections"].([]interface{}); ok {
				backup.Collections = make([]string, len(collections))
				for i, col := range collections {
					if colStr, ok := col.(string); ok {
						backup.Collections[i] = colStr
					}
				}
			}
		}

		backups = append(backups, backup)
	}

	return backups, nil
}

func (bs *BackupService) calculateBackupSize(backupPath string) int64 {
	var totalSize int64

	err := filepath.Walk(backupPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() {
			totalSize += info.Size()
		}
		return nil
	})

	if err != nil {
		bs.logger.Warnf("Failed to calculate backup size for %s: %v", backupPath, err)
		return 0
	}

	return totalSize
}

type BackupInfo struct {
	Timestamp   string    `json:"timestamp"`
	CreatedAt   time.Time `json:"created_at"`
	Size        int64     `json:"size"`
	Collections []string  `json:"collections"`
}