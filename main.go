package main

import (
	"html/template"
	"log"
	"net/http"
	"nfl-app-go/database"
	"nfl-app-go/handlers"
	"nfl-app-go/services"
	"os"

	"github.com/gorilla/mux"
)

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

func main() {
	// Initialize MongoDB connection
	dbConfig := database.Config{
		Host:     getEnv("DB_HOST", "p5server"),
		Port:     getEnv("DB_PORT", "27017"),
		Username: getEnv("DB_USERNAME", "nflapp"),
		Password: getEnv("DB_PASSWORD", ""),
		Database: getEnv("DB_NAME", "nfl_app"),
	}
	
	log.Printf("DB Config - Host: %s, Port: %s, Username: %s, Database: %s, Password set: %t", 
		dbConfig.Host, dbConfig.Port, dbConfig.Username, dbConfig.Database, dbConfig.Password != "")

	db, err := database.NewMongoConnection(dbConfig)
	if err != nil {
		log.Printf("Database connection failed: %v", err)
		log.Println("Continuing without database connection...")
		
		// Parse templates
		templates, err := template.ParseGlob("templates/*.html")
		if err != nil {
			log.Fatal("Error parsing templates:", err)
		}

		// Create demo service as fallback
		gameService := services.NewDemoGameService()
		gameHandler := handlers.NewGameHandler(templates, gameService)
		
		// Setup routes without database
		r := mux.NewRouter()
		r.PathPrefix("/static/").Handler(http.StripPrefix("/static/", http.FileServer(http.Dir("static/"))))
		r.HandleFunc("/", gameHandler.GetGames).Methods("GET")
		r.HandleFunc("/games", gameHandler.GetGames).Methods("GET")

		// Start server
		log.Println("Server starting on 127.0.0.1:8080")
		log.Println("Visit: http://localhost:8080")
		log.Fatal(http.ListenAndServe("127.0.0.1:8080", r))
		return
	}
	
	defer db.Close()
	
	// Test the connection
	if err := db.TestConnection(); err != nil {
		log.Printf("Database test failed: %v", err)
	}

	// Create database repository
	gameRepo := database.NewMongoGameRepository(db)

	// Create ESPN service and data loader
	espnService := services.NewESPNService()
	dataLoader := services.NewDataLoader(espnService, gameRepo)

	// Clear existing games collection to get fresh data with corrected date parsing
	log.Println("Clearing existing games collection...")
	if err := gameRepo.ClearAllGames(); err != nil {
		log.Printf("Failed to clear games collection: %v", err)
	}

	// Load game data on startup
	log.Println("Loading game data from ESPN API...")
	if err := dataLoader.LoadGameData(2024); err != nil {
		log.Printf("Failed to load game data: %v", err)
	}

	// Parse templates
	templates, err := template.ParseGlob("templates/*.html")
	if err != nil {
		log.Fatal("Error parsing templates:", err)
	}

	// Create database-backed service
	gameService := services.NewDatabaseGameService(gameRepo)
	
	// Create handlers
	gameHandler := handlers.NewGameHandler(templates, gameService)
	
	// Start change stream watcher for real-time updates
	changeWatcher := services.NewChangeStreamWatcher(db, gameHandler.BroadcastUpdate)
	changeWatcher.StartWatching()

	// Setup routes
	r := mux.NewRouter()
	
	// Static files
	r.PathPrefix("/static/").Handler(http.StripPrefix("/static/", http.FileServer(http.Dir("static/"))))
	
	// Game routes
	r.HandleFunc("/", gameHandler.GetGames).Methods("GET")
	r.HandleFunc("/games", gameHandler.GetGames).Methods("GET")
	r.HandleFunc("/events", gameHandler.SSEHandler).Methods("GET")

	// Start server
	log.Println("Server starting on 127.0.0.1:8080")
	log.Println("Visit: http://localhost:8080")
	log.Fatal(http.ListenAndServe("127.0.0.1:8080", r))
}