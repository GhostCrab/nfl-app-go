package main

import (
	"fmt"
	"log"
	"nfl-app-go/config"
	"nfl-app-go/database"
	"nfl-app-go/logging"
	"nfl-app-go/services"
	"time"
)

func main() {
	fmt.Println("💾 NFL App Manual Backup Tool")
	fmt.Println("============================")

	// Load configuration
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("Failed to load configuration: %v", err)
	}

	// Configure logging
	logging.Configure(cfg.ToLoggingConfig())

	// Connect to database
	dbConfig := cfg.ToDatabaseConfig()
	db, err := database.NewMongoConnection(dbConfig)
	if err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}
	defer db.Close()

	// Create backup service
	backupConfig := services.BackupConfig{
		BackupDir:   cfg.GetBackupDir(),
		Collections: []string{"weekly_picks", "games"},
	}
	backupService := services.NewBackupService(db, backupConfig)

	// Perform manual backup
	fmt.Printf("🔄 Starting manual backup...\n")
	fmt.Printf("📁 Backup directory: %s\n", cfg.GetBackupDir())
	fmt.Printf("📊 Collections: weekly_picks, games\n")
	fmt.Printf("⏰ Started at: %s\n", time.Now().Format("2006-01-02 15:04:05"))

	startTime := time.Now()

	if err := backupService.CreateBackup(); err != nil {
		log.Fatalf("Manual backup failed: %v", err)
	}

	duration := time.Since(startTime)

	fmt.Println("\n✅ Manual backup completed successfully!")
	fmt.Printf("⏱️  Duration: %v\n", duration.Round(time.Second))
	fmt.Printf("🕐 Completed at: %s\n", time.Now().Format("2006-01-02 15:04:05"))
	fmt.Printf("📁 Check backup directory: %s\n", cfg.GetBackupDir())
}