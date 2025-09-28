package main

import (
	"bufio"
	"fmt"
	"log"
	"nfl-app-go/config"
	"nfl-app-go/database"
	"nfl-app-go/logging"
	"nfl-app-go/services"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"
)

func main() {
	fmt.Println("🔄 NFL App Database Backup & Restore Tool")
	fmt.Println("==========================================")

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

	// Show main menu
	fmt.Println()
	fmt.Println("Options:")
	fmt.Println("  [1] Restore from existing backup")
	fmt.Println("  [2] Create new backup")
	fmt.Println("  [3] Exit")
	fmt.Println()

	// Get main menu selection
	reader := bufio.NewReader(os.Stdin)
	fmt.Print("Select an option [1-3]: ")

	input, err := reader.ReadString('\n')
	if err != nil {
		log.Fatalf("Failed to read input: %v", err)
	}

	choice, err := strconv.Atoi(strings.TrimSpace(input))
	if err != nil || choice < 1 || choice > 3 {
		log.Fatalf("Invalid selection. Please choose 1, 2, or 3")
	}

	switch choice {
	case 1:
		performRestore(backupService, cfg)
	case 2:
		performBackup(backupService, cfg)
	case 3:
		fmt.Println("👋 Goodbye!")
		os.Exit(0)
	}
}

func performBackup(backupService *services.BackupService, cfg *config.Config) {
	fmt.Println("\n💾 Creating New Backup")
	fmt.Println("=====================")
	fmt.Printf("📁 Backup directory: %s\n", cfg.GetBackupDir())
	fmt.Printf("📊 Collections: weekly_picks, games\n")
	fmt.Printf("⏰ Started at: %s\n", time.Now().Format("2006-01-02 15:04:05"))

	startTime := time.Now()

	if err := backupService.CreateBackup(); err != nil {
		log.Fatalf("Manual backup failed: %v", err)
	}

	duration := time.Since(startTime)

	fmt.Println("\n✅ Backup completed successfully!")
	fmt.Printf("⏱️  Duration: %v\n", duration.Round(time.Second))
	fmt.Printf("🕐 Completed at: %s\n", time.Now().Format("2006-01-02 15:04:05"))
	fmt.Printf("📁 Check backup directory: %s\n", cfg.GetBackupDir())
}

func performRestore(backupService *services.BackupService, cfg *config.Config) {
	fmt.Println("\n🔄 Restore from Backup")
	fmt.Println("=====================")

	// List available backups
	backups, err := backupService.ListBackups()
	if err != nil {
		log.Fatalf("Failed to list backups: %v", err)
	}

	if len(backups) == 0 {
		fmt.Printf("❌ No backups found in directory: %s\n", cfg.GetBackupDir())
		fmt.Println("   Make sure backups exist before attempting restore.")
		os.Exit(1)
	}

	// Sort backups by creation time (newest first)
	sort.Slice(backups, func(i, j int) bool {
		return backups[i].CreatedAt.After(backups[j].CreatedAt)
	})

	// Display available backups
	fmt.Printf("\n📦 Available Backups in %s:\n", cfg.GetBackupDir())
	fmt.Println("=" + strings.Repeat("=", 80))
	for i, backup := range backups {
		sizeKB := backup.Size / 1024
		fmt.Printf("[%d] %s\n", i+1, backup.Timestamp)
		fmt.Printf("    Created: %s\n", backup.CreatedAt.Format("2006-01-02 15:04:05"))
		fmt.Printf("    Size: %d KB\n", sizeKB)
		fmt.Printf("    Collections: %s\n", strings.Join(backup.Collections, ", "))
		if i < len(backups)-1 {
			fmt.Println()
		}
	}

	// Get user selection
	reader := bufio.NewReader(os.Stdin)
	fmt.Printf("\n🎯 Select backup to restore [1-%d]: ", len(backups))

	input, err := reader.ReadString('\n')
	if err != nil {
		log.Fatalf("Failed to read input: %v", err)
	}

	choice, err := strconv.Atoi(strings.TrimSpace(input))
	if err != nil || choice < 1 || choice > len(backups) {
		log.Fatalf("Invalid selection. Please choose a number between 1 and %d", len(backups))
	}

	selectedBackup := backups[choice-1]

	// Confirm collections to restore
	fmt.Printf("\n📋 Available collections in backup: %s\n", strings.Join(selectedBackup.Collections, ", "))
	fmt.Print("   Collections to restore (comma-separated, or 'all'): ")

	input, err = reader.ReadString('\n')
	if err != nil {
		log.Fatalf("Failed to read input: %v", err)
	}

	collectionsInput := strings.TrimSpace(input)
	var collectionsToRestore []string

	if collectionsInput == "all" || collectionsInput == "" {
		collectionsToRestore = selectedBackup.Collections
	} else {
		parts := strings.Split(collectionsInput, ",")
		for _, part := range parts {
			collection := strings.TrimSpace(part)
			if collection != "" {
				// Validate collection exists in backup
				found := false
				for _, available := range selectedBackup.Collections {
					if available == collection {
						found = true
						break
					}
				}
				if !found {
					log.Fatalf("Collection '%s' not found in backup. Available: %s",
						collection, strings.Join(selectedBackup.Collections, ", "))
				}
				collectionsToRestore = append(collectionsToRestore, collection)
			}
		}
	}

	// Final confirmation with warnings
	fmt.Println("\n⚠️  WARNING: DESTRUCTIVE OPERATION")
	fmt.Println("=" + strings.Repeat("=", 40))
	fmt.Printf("Backup: %s\n", selectedBackup.Timestamp)
	fmt.Printf("Created: %s\n", selectedBackup.CreatedAt.Format("2006-01-02 15:04:05"))
	fmt.Printf("Collections to restore: %s\n", strings.Join(collectionsToRestore, ", "))
	fmt.Println()
	fmt.Println("⚠️  This will PERMANENTLY DELETE all current data in the selected collections!")
	fmt.Println("⚠️  Make sure you have a recent backup of current data if needed!")
	fmt.Println()
	fmt.Print("Type 'YES' (all caps) to confirm restore: ")

	input, err = reader.ReadString('\n')
	if err != nil {
		log.Fatalf("Failed to read confirmation: %v", err)
	}

	confirmation := strings.TrimSpace(input)
	if confirmation != "YES" {
		fmt.Println("❌ Restore cancelled by user")
		os.Exit(0)
	}

	// Perform restore
	fmt.Printf("\n🔄 Starting restore of backup: %s\n", selectedBackup.Timestamp)
	fmt.Printf("⏰ Started at: %s\n", time.Now().Format("2006-01-02 15:04:05"))

	startTime := time.Now()

	if err := backupService.RestoreBackup(selectedBackup.Timestamp, collectionsToRestore); err != nil {
		log.Fatalf("Restore failed: %v", err)
	}

	duration := time.Since(startTime)

	fmt.Println("\n✅ Restore completed successfully!")
	fmt.Printf("⏱️  Duration: %v\n", duration.Round(time.Second))
	fmt.Printf("📊 Collections restored: %s\n", strings.Join(collectionsToRestore, ", "))
	fmt.Printf("🕐 Completed at: %s\n", time.Now().Format("2006-01-02 15:04:05"))

	// Additional safety reminder
	fmt.Println("\n💡 Remember to:")
	fmt.Println("   • Restart your application if it's currently running")
	fmt.Println("   • Verify the restored data looks correct")
	fmt.Println("   • Check application logs for any issues")
}