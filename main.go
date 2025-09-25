package main

import (
	"context"
	"html/template"
	"net/http"
	"nfl-app-go/config"
	"nfl-app-go/database"
	"nfl-app-go/handlers"
	"nfl-app-go/logging"
	"nfl-app-go/middleware"
	"nfl-app-go/services"
	"nfl-app-go/templates"
	"os"

	"github.com/gorilla/mux"
)

func main() {
	// Load configuration from environment and .env file
	cfg, err := config.Load()
	if err != nil {
		// Use basic logging since structured logging isn't configured yet
		logging.Fatal("Failed to load configuration:", err)
	}

	// Configure structured logging using config
	logging.Configure(cfg.ToLoggingConfig())

	// Log the loaded configuration
	cfg.LogConfiguration()

	// Get database config from centralized config
	dbConfig := cfg.ToDatabaseConfig()

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
		// Parse partial templates
		templates, err = templates.ParseGlob("templates/partials/*.html")
		if err != nil {
			logging.Fatal("Error parsing templates:", err)
		}

		// Create demo service as fallback
		gameService := services.NewDemoGameService()
		gameDisplayHandler := handlers.NewGameDisplayHandler(templates, gameService, cfg)
		// Note: Demo mode doesn't support parlay scoring

		// Setup routes without database
		r := mux.NewRouter()
		r.PathPrefix("/static/").Handler(http.StripPrefix("/static/", http.FileServer(http.Dir("static/"))))
		r.HandleFunc("/", gameDisplayHandler.GetGames).Methods("GET")
		r.HandleFunc("/games", gameDisplayHandler.GetGames).Methods("GET")
		r.HandleFunc("/games/refresh", gameDisplayHandler.RefreshGames).Methods("GET")

		// Start server using centralized config
		demoServerAddr := cfg.GetServerAddress()
		logging.Infof("Demo server starting on %s (available on LAN)", demoServerAddr)
		logging.Infof("Visit: http://localhost:%s or http://[your-pi-ip]:%s", cfg.Server.Port, cfg.Server.Port)
		logging.Fatal(http.ListenAndServe(demoServerAddr, r))
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
	weeklyPicksRepo := database.NewMongoWeeklyPicksRepository(db)

	// Create ESPN service and data loader
	espnService := services.NewESPNService()
	dataLoader := services.NewDataLoader(espnService, gameRepo)

	// Check if we have games for current season, if not load them
	currentSeason := cfg.App.CurrentSeason
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
	// Parse partial templates
	templates, err = templates.ParseGlob("templates/partials/*.html")
	if err != nil {
		logging.Fatal("Error parsing templates:", err)
	}

	// Create email service using centralized config
	emailService := services.NewEmailService(cfg.ToEmailConfig())

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

	// Create services using centralized config
	authService := services.NewAuthService(userRepo, cfg.Auth.JWTSecret)
	gameService := services.NewDatabaseGameService(gameRepo)
	pickRepo := database.NewMongoPickRepository(db) // Keep for legacy compatibility during transition
	pickService := services.NewPickService(weeklyPicksRepo, gameRepo, userRepo, parlayRepo)
	parlayService := services.NewParlayService(pickRepo, gameRepo, parlayRepo)
	resultCalcService := services.NewResultCalculationService(pickRepo, gameRepo)
	analyticsService := services.NewAnalyticsService(pickRepo, gameRepo, userRepo)
	visibilityService := services.NewPickVisibilityService(gameService)

	// Set up specialized services for delegation BEFORE creating memory scorer
	pickService.SetSpecializedServices(parlayService, resultCalcService, analyticsService)

	// Create in-memory parlay scorer for real-time score management
	memoryScorer := services.NewMemoryParlayScorer(parlayService, pickService)
	ctx := context.Background()
	if err := memoryScorer.InitializeFromDatabase(ctx, currentSeason); err != nil {
		logging.Errorf("Failed to initialize memory scorer: %v", err)
	}

	// Create middleware
	authMiddleware := middleware.NewAuthMiddleware(authService)

	// Create new specialized handlers
	sseHandler := handlers.NewSSEHandler(templates, gameService, cfg)
	sseHandler.SetServices(pickService, authService, visibilityService, memoryScorer)

	gameDisplayHandler := handlers.NewGameDisplayHandler(templates, gameService, cfg)
	gameDisplayHandler.SetServices(pickService, authService, visibilityService, userRepo)
	pickManagementHandler := handlers.NewPickManagementHandler(templates, gameService, pickService, authService, visibilityService, sseHandler)
	dashboardHandler := handlers.NewDashboardHandler(templates, gameService, pickService, authService, visibilityService, nil, dataLoader)
	demoTestingHandler := handlers.NewDemoTestingHandler(templates, gameService, sseHandler, parlayService)

	// Create auth handler
	authHandler := handlers.NewAuthHandler(templates, authService, emailService)

	// Set up SSE broadcasting for pick service
	pickService.SetBroadcaster(sseHandler)

	// Start background ESPN API updater
	if cfg.IsBackgroundUpdaterEnabled() {
		backgroundUpdater := services.NewBackgroundUpdater(espnService, gameRepo, pickService, parlayService, currentSeason)
		backgroundUpdater.Start()
		defer backgroundUpdater.Stop()
	}

	// Start mock background updater for testing (if enabled)
	if cfg.IsMockUpdaterEnabled() {
		mockUpdater := services.NewMockBackgroundUpdater(gameRepo, currentSeason)
		mockUpdater.Start()
		defer mockUpdater.Stop()
		logging.Info("Mock background updater enabled - generating fake game updates every 10 seconds (every 4th update completes games)")
	}

	// Start change stream watcher for real-time updates
	changeWatcher := services.NewChangeStreamWatcher(db, sseHandler.HandleDatabaseChange)
	changeWatcher.StartWatching()

	// Start visibility timer service for automatic pick visibility updates
	visibilityTimer := services.NewVisibilityTimerService(
		visibilityService,
		func(eventType, data string) {
			// Broadcast visibility changes via SSE
			sseHandler.BroadcastToAllClients(eventType, data)
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
	isDevelopment := cfg.App.IsDevelopment
	logging.Infof("Server starting in %s mode (isDevelopment: %t)", cfg.Server.Environment, isDevelopment)
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
	r.Handle("/club-scores/test-update", authMiddleware.OptionalAuth(http.HandlerFunc(demoTestingHandler.TestClubScoreUpdate))).Methods("POST")
	r.Handle("/parlay-scores/test-db-update", authMiddleware.OptionalAuth(http.HandlerFunc(demoTestingHandler.TestDatabaseParlayUpdate))).Methods("POST")

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

	// Server configuration from centralized config
	useTLS := cfg.Server.UseTLS
	serverPort := cfg.Server.Port
	behindProxy := cfg.Server.BehindProxy

	if !cfg.IsEmailConfigured() {
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

	// Start server using centralized config
	serverAddr := cfg.GetServerAddress()

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
		if _, err := os.Stat(cfg.Server.CertFile); os.IsNotExist(err) {
			logging.Fatalf("%s not found. Set USE_TLS=false or generate certificates.", cfg.Server.CertFile)
		}
		if _, err := os.Stat(cfg.Server.KeyFile); os.IsNotExist(err) {
			logging.Fatalf("%s not found. Set USE_TLS=false or generate certificates.", cfg.Server.KeyFile)
		}

		logging.Fatal(http.ListenAndServeTLS(serverAddr, cfg.Server.CertFile, cfg.Server.KeyFile, r))
	} else {
		logging.Infof("Server starting on %s (HTTP - available on LAN)", serverAddr)
		logging.Infof("Visit: http://localhost:%s or http://[your-pi-ip]:%s", serverPort, serverPort)
		logging.Infof("Login page: http://localhost:%s/login or http://[your-pi-ip]:%s/login", serverPort, serverPort)
		logging.Info("Default password for all users: password123")
		logging.Info("⚠️  HTTP mode - use only behind HTTPS proxy/tunnel")
		logging.Fatal(http.ListenAndServe(serverAddr, r))
	}
}
