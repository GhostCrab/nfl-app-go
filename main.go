package main

import (
	"html/template"
	"log"
	"net/http"
	"nfl-app-go/database"
	"nfl-app-go/handlers"
	"nfl-app-go/middleware"
	"nfl-app-go/services"
	"os"

	"github.com/gorilla/mux"
	"github.com/joho/godotenv"
)

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

func main() {
	// Set log format with milliseconds for timing analysis
	log.SetFlags(log.LstdFlags | log.Lmicroseconds)
	
	// Load .env file
	if err := godotenv.Load(); err != nil {
		log.Printf("Warning: Could not load .env file: %v", err)
	}
	
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
		
		// Parse templates with custom functions
		templateFuncs := template.FuncMap{
			"add": func(a, b int) int {
				return a + b
			},
			"sub": func(a, b int) int {
				return a - b
			},
			"minus": func(a, b int) int {
				return a - b
			},
			"plus": func(a, b int) int {
				return a + b
			},
			"float64": func(i int) float64 {
				return float64(i)
			},
		}
		
		templates, err := template.New("").Funcs(templateFuncs).ParseGlob("templates/*.html")
		if err != nil {
			log.Fatal("Error parsing templates:", err)
		}

		// Create demo service as fallback
		gameService := services.NewDemoGameService()
		gameHandler := handlers.NewGameHandler(templates, gameService)
		// Note: Demo mode doesn't support parlay scoring
		
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

	// Create database repositories
	gameRepo := database.NewMongoGameRepository(db)
	userRepo := database.NewMongoUserRepository(db)
	parlayRepo := database.NewMongoParlayRepository(db)

	// Create ESPN service and data loader
	espnService := services.NewESPNService()
	dataLoader := services.NewDataLoader(espnService, gameRepo)

	// Check if we have games for 2025 season, if not load them
	currentSeason := 2025
	existingGames, err := gameRepo.GetGamesBySeason(currentSeason)
	if err != nil || len(existingGames) == 0 {
		log.Printf("No games found for %d season, loading from ESPN API...", currentSeason)
		if err := dataLoader.LoadGameData(currentSeason); err != nil {
			log.Printf("Failed to load game data for %d: %v", currentSeason, err)
		}
	} else {
		log.Printf("Found %d existing games for %d season", len(existingGames), currentSeason)
	}

	// Seed users if needed
	userSeeder := services.NewUserSeeder(userRepo)
	if err := userSeeder.SeedUsers(); err != nil {
		log.Printf("Failed to seed users: %v", err)
	}

	// Parse templates with custom functions
	templateFuncs := template.FuncMap{
		"add": func(a, b int) int {
			return a + b
		},
		"sub": func(a, b int) int {
			return a - b
		},
		"minus": func(a, b int) int {
			return a - b
		},
		"plus": func(a, b int) int {
			return a + b
		},
		"float64": func(i int) float64 {
			return float64(i)
		},
	}
	
	templates, err := template.New("").Funcs(templateFuncs).ParseGlob("templates/*.html")
	if err != nil {
		log.Fatal("Error parsing templates:", err)
	}

	// Create email service
	emailConfig := services.EmailConfig{
		SMTPHost:     getEnv("SMTP_HOST", ""),
		SMTPPort:     getEnv("SMTP_PORT", "587"),
		SMTPUsername: getEnv("SMTP_USERNAME", ""),
		SMTPPassword: getEnv("SMTP_PASSWORD", ""),
		FromEmail:    getEnv("FROM_EMAIL", ""),
		FromName:     getEnv("FROM_NAME", "NFL Games"),
	}
	emailService := services.NewEmailService(emailConfig)

	// Test email configuration if provided
	if emailService.IsConfigured() {
		log.Println("Email service configured, testing connection...")
		if err := emailService.TestConnection(); err != nil {
			log.Printf("Email service test failed: %v", err)
			log.Println("Password reset emails will use development mode (show link directly)")
		} else {
			log.Println("Email service test successful")
		}
	} else {
		log.Println("Email service not configured - using development mode for password resets")
	}

	// Create services
	jwtSecret := getEnv("JWT_SECRET", "your-secret-key-change-in-production")
	authService := services.NewAuthService(userRepo, jwtSecret)
	gameService := services.NewDatabaseGameService(gameRepo)
	pickRepo := database.NewMongoPickRepository(db)
	pickService := services.NewPickService(pickRepo, gameRepo, userRepo, parlayRepo)
	
	// Create middleware
	authMiddleware := middleware.NewAuthMiddleware(authService)
	
	// Create handlers
	gameHandler := handlers.NewGameHandler(templates, gameService)
	authHandler := handlers.NewAuthHandler(templates, authService, emailService)
	
	// Wire up pick service to game handler
	gameHandler.SetPickService(pickService)
	
	// Start background ESPN API updater
	backgroundUpdater := services.NewBackgroundUpdater(espnService, gameRepo, pickService, currentSeason)
	backgroundUpdater.Start()
	defer backgroundUpdater.Stop()
	
	// Start change stream watcher for real-time updates
	changeWatcher := services.NewChangeStreamWatcher(db, gameHandler.HandleDatabaseChange)
	changeWatcher.StartWatching()

	// Setup routes
	r := mux.NewRouter()
	
	// Add security middleware
	r.Use(middleware.SecurityMiddleware)
	
	// Add no-cache middleware for development only
	isDevelopment := getEnv("ENVIRONMENT", "development") == "development"
	log.Printf("Server starting in %s mode (isDevelopment: %t)", getEnv("ENVIRONMENT", "development"), isDevelopment)
	if isDevelopment {
		r.Use(func(next http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
				w.Header().Set("Pragma", "no-cache")
				w.Header().Set("Expires", "0")
				next.ServeHTTP(w, r)
			})
		})
	}
	
	// Static files
	log.Printf("Setting up static file server for /static/ directory")
	r.PathPrefix("/static/").Handler(http.StripPrefix("/static/", http.FileServer(http.Dir("static/"))))
	
	// Auth routes (public)
	r.HandleFunc("/login", authHandler.LoginPage).Methods("GET")
	r.HandleFunc("/login", authHandler.Login).Methods("POST")
	r.HandleFunc("/logout", authHandler.Logout).Methods("GET", "POST")
	r.HandleFunc("/api/login", authHandler.LoginAPI).Methods("POST")
	
	// Password reset routes (public)
	r.HandleFunc("/forgot-password", authHandler.ForgotPasswordPage).Methods("GET")
	r.HandleFunc("/forgot-password", authHandler.ForgotPassword).Methods("POST")
	r.HandleFunc("/reset-password", authHandler.ResetPasswordPage).Methods("GET")
	r.HandleFunc("/reset-password", authHandler.ResetPassword).Methods("POST")
	
	// Game routes (with optional auth to show user info)
	r.Handle("/", authMiddleware.OptionalAuth(http.HandlerFunc(gameHandler.GetGames))).Methods("GET")
	r.Handle("/games", authMiddleware.OptionalAuth(http.HandlerFunc(gameHandler.GetGames))).Methods("GET")
	r.Handle("/events", authMiddleware.OptionalAuth(http.HandlerFunc(gameHandler.SSEHandler))).Methods("GET")
	r.Handle("/api/games", authMiddleware.OptionalAuth(http.HandlerFunc(gameHandler.GetGamesAPI))).Methods("GET")
	r.Handle("/api/dashboard", authMiddleware.OptionalAuth(http.HandlerFunc(gameHandler.GetDashboardDataAPI))).Methods("GET")
	
	// Protected API routes
	apiRouter := r.PathPrefix("/api").Subrouter()
	apiRouter.Use(authMiddleware.RequireAuth)
	apiRouter.HandleFunc("/me", authHandler.Me).Methods("GET")

	// Server configuration
	useTLS := getEnv("USE_TLS", "true") == "true"
	serverPort := getEnv("SERVER_PORT", "8080")
	behindProxy := getEnv("BEHIND_PROXY", "false") == "true"
	
	if !emailService.IsConfigured() {
		log.Println("")
		log.Println("📧 EMAIL CONFIGURATION:")
		log.Println("To enable real password reset emails, set these environment variables:")
		log.Println("  SMTP_HOST=smtp.gmail.com (for Gmail)")
		log.Println("  SMTP_USERNAME=your-email@gmail.com")
		log.Println("  SMTP_PASSWORD=your-app-password")
		log.Println("  FROM_EMAIL=your-email@gmail.com")
		log.Println("  FROM_NAME=\"NFL Games\"")
		log.Println("")
	}
	
	// Start server
	serverAddr := "localhost:" + serverPort
	
	if behindProxy {
		log.Printf("Server starting on %s (HTTP - behind proxy/tunnel)", serverAddr)
		log.Println("⚡ Configured for Cloudflare Tunnel or reverse proxy")
		log.Println("Default password for all users: password123")
		log.Fatal(http.ListenAndServe(serverAddr, r))
	} else if useTLS {
		log.Printf("Server starting on %s (HTTPS)", serverAddr)
		log.Printf("Visit: https://localhost:%s", serverPort)
		log.Printf("Login page: https://localhost:%s/login", serverPort)
		log.Println("Default password for all users: password123")
		log.Println("⚠️  Using self-signed certificate - browser will show security warning")
		
		// Check if certificate files exist
		if _, err := os.Stat("server.crt"); os.IsNotExist(err) {
			log.Fatal("server.crt not found. Set USE_TLS=false or generate certificates.")
		}
		if _, err := os.Stat("server.key"); os.IsNotExist(err) {
			log.Fatal("server.key not found. Set USE_TLS=false or generate certificates.")
		}
		
		log.Fatal(http.ListenAndServeTLS(serverAddr, "server.crt", "server.key", r))
	} else {
		log.Printf("Server starting on %s (HTTP)", serverAddr)
		log.Printf("Visit: http://localhost:%s", serverPort)
		log.Printf("Login page: http://localhost:%s/login", serverPort)
		log.Println("Default password for all users: password123")
		log.Println("⚠️  HTTP mode - use only behind HTTPS proxy/tunnel")
		log.Fatal(http.ListenAndServe(serverAddr, r))
	}
}
