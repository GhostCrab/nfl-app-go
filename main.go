package main

import (
	"context"
	"html/template"
	"net/http"
	"nfl-app-go/database"
	"nfl-app-go/handlers"
	"nfl-app-go/logging"
	"nfl-app-go/middleware"
	"nfl-app-go/services"
	"nfl-app-go/templates"
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
	// Configure structured logging
	logging.Configure(logging.Config{
		Level:       getEnv("LOG_LEVEL", "info"),
		Output:      os.Stdout,
		Prefix:      "nfl-app",
		EnableColor: getEnv("LOG_COLOR", "true") != "false",
	})

	// Load .env file
	if err := godotenv.Load(); err != nil {
		logging.Warnf("Could not load .env file: %v", err)
	}

	// Initialize MongoDB connection
	dbConfig := database.Config{
		Host:     getEnv("DB_HOST", "p5server"),
		Port:     getEnv("DB_PORT", "27017"),
		Username: getEnv("DB_USERNAME", "nflapp"),
		Password: getEnv("DB_PASSWORD", ""),
		Database: getEnv("DB_NAME", "nfl_app"),
	}

	logging.Infof("DB Config - Host: %s, Port: %s, Username: %s, Database: %s, Password set: %t",
		dbConfig.Host, dbConfig.Port, dbConfig.Username, dbConfig.Database, dbConfig.Password != "")

	db, err := database.NewMongoConnection(dbConfig)
	if err != nil {
		logging.Errorf("Database connection failed: %v", err)
		logging.Warn("Continuing without database connection...")

		// Parse templates with custom functions
		templateFuncs := templates.GetTemplateFuncs()

		templates, err := template.New("").Funcs(templateFuncs).ParseGlob("templates/*.html")
		if err != nil {
			logging.Fatal("Error parsing templates:", err)
		}

		// Create demo service as fallback
		gameService := services.NewDemoGameService()
		gameDisplayHandler := handlers.NewGameDisplayHandler(templates, gameService)
		// Note: Demo mode doesn't support parlay scoring

		// Setup routes without database
		r := mux.NewRouter()
		r.PathPrefix("/static/").Handler(http.StripPrefix("/static/", http.FileServer(http.Dir("static/"))))
		r.HandleFunc("/", gameDisplayHandler.GetGames).Methods("GET")
		r.HandleFunc("/games", gameDisplayHandler.GetGames).Methods("GET")
		r.HandleFunc("/games/refresh", gameDisplayHandler.RefreshGames).Methods("GET")

		// Start server
		logging.Info("Server starting on 0.0.0.0:8080 (available on LAN)")
		logging.Info("Visit: http://localhost:8080 or http://[your-pi-ip]:8080")
		logging.Fatal(http.ListenAndServe("0.0.0.0:8080", r))
		return
	}

	defer db.Close()

	// Test the connection
	if err := db.TestConnection(); err != nil {
		logging.Errorf("Database test failed: %v", err)
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
		logging.Infof("No games found for %d season, loading from ESPN API...", currentSeason)
		if err := dataLoader.LoadGameData(currentSeason); err != nil {
			logging.Errorf("Failed to load game data for %d: %v", currentSeason, err)
		}
	} else {
		logging.Infof("Found %d existing games for %d season", len(existingGames), currentSeason)
	}

	// Seed users if needed
	userSeeder := services.NewUserSeeder(userRepo)
	if err := userSeeder.SeedUsers(); err != nil {
		logging.Errorf("Failed to seed users: %v", err)
	}

	// Parse templates with custom functions
	templateFuncs := templates.GetTemplateFuncs()

	templates, err := template.New("").Funcs(templateFuncs).ParseGlob("templates/*.html")
	if err != nil {
		logging.Fatal("Error parsing templates:", err)
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
		logging.Info("Email service configured, testing connection...")
		if err := emailService.TestConnection(); err != nil {
			logging.Errorf("Email service test failed: %v", err)
			logging.Info("Password reset emails will use development mode (show link directly)")
		} else {
			logging.Info("Email service test successful")
		}
	} else {
		logging.Info("Email service not configured - using development mode for password resets")
	}

	// Create services
	jwtSecret := getEnv("JWT_SECRET", "your-secret-key-change-in-production")
	authService := services.NewAuthService(userRepo, jwtSecret)
	gameService := services.NewDatabaseGameService(gameRepo)
	pickRepo := database.NewMongoPickRepository(db)
	pickService := services.NewPickService(pickRepo, gameRepo, userRepo, parlayRepo)
	parlayService := services.NewParlayService(pickRepo, gameRepo, parlayRepo)
	resultCalcService := services.NewResultCalculationService(pickRepo, gameRepo)
	analyticsService := services.NewAnalyticsService(pickRepo, gameRepo, userRepo)
	visibilityService := services.NewPickVisibilityService(gameService)

	// Create middleware
	authMiddleware := middleware.NewAuthMiddleware(authService)

	// Create new specialized handlers
	sseHandler := handlers.NewSSEHandler(templates, gameService)
	sseHandler.SetServices(pickService, authService, visibilityService)
	
	gameDisplayHandler := handlers.NewGameDisplayHandler(templates, gameService)
	gameDisplayHandler.SetServices(pickService, authService, visibilityService, userRepo)
	pickManagementHandler := handlers.NewPickManagementHandler(templates, gameService, pickService, authService, visibilityService, sseHandler)
	dashboardHandler := handlers.NewDashboardHandler(templates, gameService, pickService, authService, visibilityService, nil, dataLoader)
	demoTestingHandler := handlers.NewDemoTestingHandler(templates, gameService, sseHandler)
	
	// Legacy GameHandler removed - all functionality moved to specialized handlers
	
	// Create auth handler
	authHandler := handlers.NewAuthHandler(templates, authService, emailService)
	
	// Set up SSE broadcasting for pick service
	pickService.SetBroadcaster(sseHandler)
	
	// Set up specialized services for delegation
	pickService.SetSpecializedServices(parlayService, resultCalcService, analyticsService)

	// Start background ESPN API updater
	backgroundUpdater := services.NewBackgroundUpdater(espnService, gameRepo, pickService, parlayService, currentSeason)
	backgroundUpdater.Start()
	defer backgroundUpdater.Stop()

	// Start change stream watcher for real-time updates
	changeWatcher := services.NewChangeStreamWatcher(db, sseHandler.HandleDatabaseChange)
	changeWatcher.StartWatching()

	// Start visibility timer service for automatic pick visibility updates
	visibilityTimer := services.NewVisibilityTimerService(
		visibilityService, 
		func(eventType, data string) {
			// Broadcast visibility changes via SSE
			sseHandler.BroadcastStructuredUpdate(eventType, data)
		}, 
		currentSeason,
	)
	visibilityTimer.Start()
	defer visibilityTimer.Stop()
	
	// Log upcoming visibility changes for debugging
	visibilityTimer.LogUpcomingChanges(context.Background())

	// Setup routes
	r := mux.NewRouter()

	// Add security middleware
	r.Use(middleware.SecurityMiddleware)

	// Add no-cache middleware for development only
	isDevelopment := getEnv("ENVIRONMENT", "development") == "development"
	logging.Infof("Server starting in %s mode (isDevelopment: %t)", getEnv("ENVIRONMENT", "development"), isDevelopment)
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
	logging.Debug("Setting up static file server for /static/ directory")
	r.PathPrefix("/static/").Handler(http.StripPrefix("/static/", http.FileServer(http.Dir("static/"))))

	// Create additional services for analytics
	userService := services.NewDatabaseUserService(userRepo)
	teamService := services.NewStaticTeamService()
	
	// Create analytics handler
	analyticsHandler := handlers.NewAnalyticsHandler(templates, gameService, pickService, userService, teamService)

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

	// Game display routes (with optional auth to show user info)
	r.Handle("/", authMiddleware.OptionalAuth(http.HandlerFunc(gameDisplayHandler.GetGames))).Methods("GET")
	r.Handle("/games", authMiddleware.OptionalAuth(http.HandlerFunc(gameDisplayHandler.GetGames))).Methods("GET")
	r.Handle("/games/refresh", authMiddleware.OptionalAuth(http.HandlerFunc(gameDisplayHandler.RefreshGames))).Methods("GET")
	r.Handle("/api/games", authMiddleware.OptionalAuth(http.HandlerFunc(gameDisplayHandler.GetGamesAPI))).Methods("GET")
	
	// SSE and real-time updates
	r.Handle("/events", authMiddleware.OptionalAuth(http.HandlerFunc(sseHandler.Handle))).Methods("GET")
	
	// Demo and testing routes
	r.Handle("/games/test-update", authMiddleware.OptionalAuth(http.HandlerFunc(demoTestingHandler.TestGameUpdate))).Methods("POST")
	
	// Dashboard API routes
	r.Handle("/api/dashboard", authMiddleware.OptionalAuth(http.HandlerFunc(dashboardHandler.GetDashboardDataAPI))).Methods("GET")

	// Analytics routes
	r.Handle("/analytics", authMiddleware.OptionalAuth(http.HandlerFunc(analyticsHandler.ShowAnalytics))).Methods("GET")
	r.Handle("/api/analytics", authMiddleware.OptionalAuth(http.HandlerFunc(analyticsHandler.GetAnalyticsAPI))).Methods("GET")

	// Pick management routes (require authentication)
	r.Handle("/pick-picker", authMiddleware.RequireAuth(http.HandlerFunc(pickManagementHandler.ShowPickPicker))).Methods("GET")
	r.Handle("/submit-picks", authMiddleware.RequireAuth(http.HandlerFunc(pickManagementHandler.SubmitPicks))).Methods("POST")

	// Protected API routes
	apiRouter := r.PathPrefix("/api").Subrouter()
	apiRouter.Use(authMiddleware.RequireAuth)
	apiRouter.HandleFunc("/me", authHandler.Me).Methods("GET")

	// Server configuration
	useTLS := getEnv("USE_TLS", "true") == "true"
	serverPort := getEnv("SERVER_PORT", "8080")
	behindProxy := getEnv("BEHIND_PROXY", "false") == "true"

	if !emailService.IsConfigured() {
		logging.Info("")
		logging.Info("📧 EMAIL CONFIGURATION:")
		logging.Info("To enable real password reset emails, set these environment variables:")
		logging.Info("  SMTP_HOST=smtp.gmail.com (for Gmail)")
		logging.Info("  SMTP_USERNAME=your-email@gmail.com")
		logging.Info("  SMTP_PASSWORD=your-app-password")
		logging.Info("  FROM_EMAIL=your-email@gmail.com")
		logging.Info("  FROM_NAME=\"NFL Games\"")
		logging.Info("")
	}

	// Start server
	serverAddr := "0.0.0.0:" + serverPort

	if behindProxy {
		logging.Infof("Server starting on %s (HTTP - behind proxy/tunnel)", serverAddr)
		logging.Info("⚡ Configured for Cloudflare Tunnel or reverse proxy")
		logging.Info("Default password for all users: password123")
		logging.Fatal(http.ListenAndServe(serverAddr, r))
	} else if useTLS {
		logging.Infof("Server starting on %s (HTTPS - available on LAN)", serverAddr)
		logging.Infof("Visit: https://localhost:%s or https://[your-pi-ip]:%s", serverPort, serverPort)
		logging.Infof("Login page: https://localhost:%s/login or https://[your-pi-ip]:%s/login", serverPort, serverPort)
		logging.Info("Default password for all users: password123")
		logging.Info("⚠️  Using self-signed certificate - browser will show security warning")

		// Check if certificate files exist
		if _, err := os.Stat("server.crt"); os.IsNotExist(err) {
			logging.Fatal("server.crt not found. Set USE_TLS=false or generate certificates.")
		}
		if _, err := os.Stat("server.key"); os.IsNotExist(err) {
			logging.Fatal("server.key not found. Set USE_TLS=false or generate certificates.")
		}

		logging.Fatal(http.ListenAndServeTLS(serverAddr, "server.crt", "server.key", r))
	} else {
		logging.Infof("Server starting on %s (HTTP - available on LAN)", serverAddr)
		logging.Infof("Visit: http://localhost:%s or http://[your-pi-ip]:%s", serverPort, serverPort)
		logging.Infof("Login page: http://localhost:%s/login or http://[your-pi-ip]:%s/login", serverPort, serverPort)
		logging.Info("Default password for all users: password123")
		logging.Info("⚠️  HTTP mode - use only behind HTTPS proxy/tunnel")
		logging.Fatal(http.ListenAndServe(serverAddr, r))
	}
}
