package main

import (
	"fmt"
	"html/template"
	"log"
	"net/http"
	"nfl-app-go/database"
	"nfl-app-go/handlers"
	"nfl-app-go/middleware"
	"nfl-app-go/models"
	"nfl-app-go/services"
	"os"
	"regexp"
	"sort"
	"strings"

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
			"sub": func(a, b float64) float64 {
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
			"sequence": func(start, end int) []int {
				result := make([]int, end-start+1)
				for i := range result {
					result[i] = start + i
				}
				return result
			},
			"sortUsersByScore": func(userPicks []*models.UserPicks) []*models.UserPicks {
				if len(userPicks) == 0 {
					return userPicks
				}
				// Create a copy to avoid modifying original slice
				sorted := make([]*models.UserPicks, len(userPicks))
				copy(sorted, userPicks)
				
				// Sort by parlay points (descending - highest first)
				sort.Slice(sorted, func(i, j int) bool {
					scoreI := sorted[i].Record.ParlayPoints
					scoreJ := sorted[j].Record.ParlayPoints
					return scoreI > scoreJ
				})
				
				return sorted
			},
			"projectFinalScore": func(homeScore, awayScore, quarter int, timeLeft string) float64 {
				// Parse time left (e.g., "12:34")
				var minutes, seconds int
				if timeLeft == "Halftime" || timeLeft == "" {
					minutes = 0
					seconds = 0
				} else {
					fmt.Sscanf(timeLeft, "%d:%d", &minutes, &seconds)
				}
				
				// Calculate elapsed time in minutes
				var elapsedMinutes float64
				switch quarter {
				case 1:
					elapsedMinutes = 15 - float64(minutes) - float64(seconds)/60
				case 2:
					elapsedMinutes = 30 - float64(minutes) - float64(seconds)/60
				case 3:
					elapsedMinutes = 45 - float64(minutes) - float64(seconds)/60
				case 4:
					elapsedMinutes = 60 - float64(minutes) - float64(seconds)/60
				case 6: // Halftime
					elapsedMinutes = 30
				default:
					elapsedMinutes = 60 // Overtime or unknown, assume full game
				}
				
				// Avoid division by zero
				if elapsedMinutes <= 0 {
					elapsedMinutes = 1
				}
				
				// Calculate current total and project to 60 minutes
				currentTotal := float64(homeScore + awayScore)
				projectedTotal := (currentTotal / elapsedMinutes) * 60
				
				return projectedTotal
			},
			"findGameByID": func(games []models.Game, gameID int) *models.Game {
				for _, game := range games {
					if game.ID == gameID {
						return &game
					}
				}
				return nil
			},
			"isSpreadPickWinning": func(pick models.Pick, game models.Game) bool {
				if pick.TeamID == 98 || pick.TeamID == 99 {
					return false // This is an O/U pick, not spread
				}
				// Simplified spread check
				return game.HomeScore > game.AwayScore
			},
			"isPickedTeamCovering": func(pick models.Pick, game models.Game) string {
				if !pick.IsSpreadPick() || !game.HasOdds() {
					return "neutral"
				}
				
				teamName := pick.TeamName
				
				// Check if picked team name contains home or away team abbreviation
				isHomeTeamPick := false
				isAwayTeamPick := false
				
				// Simple matching - check if team abbreviation is in the name
				if len(teamName) > 0 && len(game.Home) > 0 && len(game.Away) > 0 {
					// Try to match by checking if abbreviation is in the name
					homeTeamLower := strings.ToLower(game.Home)
					awayTeamLower := strings.ToLower(game.Away) 
					teamNameLower := strings.ToLower(teamName)
					
					// Check various ways the team might be referenced
					if strings.Contains(teamNameLower, homeTeamLower) {
						isHomeTeamPick = true
					} else if strings.Contains(teamNameLower, awayTeamLower) {
						isAwayTeamPick = true
					} else {
						// Fallback: if we can't determine, assume it's away team (common pattern)
						isAwayTeamPick = true
					}
				}
				
				// Calculate spread coverage
				scoreDiff := game.HomeScore - game.AwayScore
				spread := game.Odds.Spread
				adjustedDiff := float64(scoreDiff) + spread
				
				if isHomeTeamPick {
					if adjustedDiff > 0 {
						return "covering" 
					} else if adjustedDiff < 0 {
						return "not-covering"
					} else {
						return "push"
					}
				} else if isAwayTeamPick {
					if adjustedDiff < 0 {
						return "covering"
					} else if adjustedDiff > 0 {
						return "not-covering" 
					} else {
						return "push"
					}
				}
				
				return "neutral"
			},
			"dict": func(values ...interface{}) (map[string]interface{}, error) {
				if len(values)%2 != 0 {
					return nil, fmt.Errorf("dict: number of arguments must be even")
				}
				result := make(map[string]interface{})
				for i := 0; i < len(values); i += 2 {
					key, ok := values[i].(string)
					if !ok {
						return nil, fmt.Errorf("dict: key must be string, got %T", values[i])
					}
					result[key] = values[i+1]
				}
				return result, nil
			},
			"getResultClass": func(pick models.Pick, game *models.Game) string {
				baseClass := pick.GetResultClass()
				
				// Add state-specific classes for pending picks
				if baseClass == "pick-class" && game != nil {
					if game.State == models.GameStateInPlay {
						return baseClass + " in-progress"
					} else if game.State == models.GameStateScheduled {
						return baseClass + " pending"
					}
				}
				
				return baseClass
			},
			"isOverUnder": func(pick models.Pick) bool {
				return pick.IsOverUnder()
			},
			"isSpreadPick": func(pick models.Pick) bool {
				return pick.IsSpreadPick()
			},
			"lower": func(s string) string {
				return strings.ToLower(s)
			},
			"contains": func(s, substr string) bool {
				return strings.Contains(s, substr)
			},
			"regexReplace": func(input, pattern, replacement string) string {
				re := regexp.MustCompile(pattern)
				return re.ReplaceAllString(input, replacement)
			},
			"split": func(s, sep string) []string {
				return strings.Split(s, sep)
			},
			"getPickTeamAbbr": func(pick models.Pick, game *models.Game, pickDesc string) string {
				if pick.IsOverUnder() {
					if strings.Contains(pickDesc, "Over") {
						return "OVR"
					} else {
						return "UND"
					}
				}
				// For spread picks, return the team abbreviation
				if game != nil && strings.Contains(pick.TeamName, game.Home) {
					return game.Home
				} else if game != nil && strings.Contains(pick.TeamName, game.Away) {
					return game.Away
				}
				return pick.TeamName
			},
			"getPickTeamIcon": func(teamAbbr string) string {
				if teamAbbr == "OVR" {
					return "https://api.iconify.design/mdi/chevron-double-up.svg"
				}
				if teamAbbr == "UND" {
					return "https://api.iconify.design/mdi/chevron-double-down.svg"
				}
				if teamAbbr == "" {
					return ""
				}
				teamLower := strings.ToLower(teamAbbr)
				return fmt.Sprintf("https://a.espncdn.com/combiner/i?img=/i/teamlogos/nfl/500/scoreboard/%s.png", teamLower)
			},
			"getPickValue": func(pick models.Pick, game *models.Game, pickDesc string) string {
				if pick.IsOverUnder() && game != nil && game.HasOdds() {
					return fmt.Sprintf("%.1f", game.Odds.OU)
				}
				// For spread picks
				if game != nil && game.HasOdds() {
					if strings.Contains(pick.TeamName, game.Home) {
						return game.FormatHomeSpread()
					} else if strings.Contains(pick.TeamName, game.Away) {
						return game.FormatAwaySpread()
					}
				}
				return string(pick.PickType)
			},
			"getTeamMascotName": func(abbr string) string {
				mascotMap := map[string]string{
					"ARI": "CARDINALS", "ATL": "FALCONS", "BAL": "RAVENS", "BUF": "BILLS",
					"CAR": "PANTHERS", "CHI": "BEARS", "CIN": "BENGALS", "CLE": "BROWNS",
					"DAL": "COWBOYS", "DEN": "BRONCOS", "DET": "LIONS", "GB": "PACKERS",
					"HOU": "TEXANS", "IND": "COLTS", "JAX": "JAGUARS", "KC": "CHIEFS",
					"LV": "RAIDERS", "LAC": "CHARGERS", "LAR": "RAMS", "MIA": "DOLPHINS",
					"MIN": "VIKINGS", "NE": "PATRIOTS", "NO": "SAINTS", "NYG": "GIANTS",
					"NYJ": "JETS", "PHI": "EAGLES", "PIT": "STEELERS", "SF": "49ERS",
					"SEA": "SEAHAWKS", "TB": "BUCCANEERS", "TEN": "TITANS", "WSH": "COMMANDERS",
					"OVR": "OVR", "UND": "UND", // Keep O/U abbreviations as-is
				}
				if mascot, exists := mascotMap[abbr]; exists {
					return mascot
				}
				return abbr // Fallback to abbreviation if not found
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
		log.Println("Server starting on 0.0.0.0:8080 (available on LAN)")
		log.Println("Visit: http://localhost:8080 or http://[your-pi-ip]:8080")
		log.Fatal(http.ListenAndServe("0.0.0.0:8080", r))
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
		"sub": func(a, b float64) float64 {
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
		"sequence": func(start, end int) []int {
			result := make([]int, end-start+1)
			for i := range result {
				result[i] = start + i
			}
			return result
		},
		"ceil": func(f float64) int {
			return int(f + 0.999999) // Ceiling function
		},
		"projectFinalScore": func(homeScore, awayScore, quarter int, timeLeft string) float64 {
			// Parse time left (e.g., "12:34")
			var minutes, seconds int
			if timeLeft == "Halftime" || timeLeft == "" {
				minutes = 0
				seconds = 0
			} else {
				fmt.Sscanf(timeLeft, "%d:%d", &minutes, &seconds)
			}
			
			// Calculate elapsed time in minutes
			var elapsedMinutes float64
			switch quarter {
			case 1:
				elapsedMinutes = 15 - float64(minutes) - float64(seconds)/60
			case 2:
				elapsedMinutes = 30 - float64(minutes) - float64(seconds)/60
			case 3:
				elapsedMinutes = 45 - float64(minutes) - float64(seconds)/60
			case 4:
				elapsedMinutes = 60 - float64(minutes) - float64(seconds)/60
			case 6: // Halftime
				elapsedMinutes = 30
			default:
				elapsedMinutes = 60 // Overtime or unknown, assume full game
			}
			
			// Avoid division by zero
			if elapsedMinutes <= 0 {
				elapsedMinutes = 1
			}
			
			// Calculate current total and project to 60 minutes
			currentTotal := float64(homeScore + awayScore)
			projectedTotal := (currentTotal / elapsedMinutes) * 60
			
			return projectedTotal
		},
		"findGameByID": func(games []models.Game, gameID int) *models.Game {
			for _, game := range games {
				if game.ID == gameID {
					return &game
				}
			}
			return nil
		},
		"isPickedTeamCovering": func(pick models.Pick, game models.Game) string {
			if !pick.IsSpreadPick() || !game.HasOdds() {
				return "neutral"
			}
			
			teamName := pick.TeamName
			
			// Check if picked team name contains home or away team abbreviation
			isHomeTeamPick := false
			isAwayTeamPick := false
			
			// Simple matching - check if team abbreviation is in the name
			if len(teamName) > 0 && len(game.Home) > 0 && len(game.Away) > 0 {
				// Try to match by checking if abbreviation is in the name
				homeTeamLower := strings.ToLower(game.Home)
				awayTeamLower := strings.ToLower(game.Away) 
				teamNameLower := strings.ToLower(teamName)
				
				// Check various ways the team might be referenced
				if strings.Contains(teamNameLower, homeTeamLower) {
					isHomeTeamPick = true
				} else if strings.Contains(teamNameLower, awayTeamLower) {
					isAwayTeamPick = true
				} else {
					// Fallback: if we can't determine, assume it's away team (common pattern)
					isAwayTeamPick = true
				}
			}
			
			// Calculate spread coverage
			scoreDiff := game.HomeScore - game.AwayScore
			spread := game.Odds.Spread
			adjustedDiff := float64(scoreDiff) + spread
			
			if isHomeTeamPick {
				if adjustedDiff > 0 {
					return "covering" 
				} else if adjustedDiff < 0 {
					return "not-covering"
				} else {
					return "push"
				}
			} else if isAwayTeamPick {
				if adjustedDiff < 0 {
					return "covering"
				} else if adjustedDiff > 0 {
					return "not-covering" 
				} else {
					return "push"
				}
			}
			
			return "neutral"
		},
		"isSpreadPickWinning": func(pick models.Pick, game models.Game) bool {
			if pick.TeamID == 98 || pick.TeamID == 99 {
				return false // This is an O/U pick, not spread
			}
			// For spread picks, we need to determine which team and check if they're covering
			// This is a simplified version - you'd need actual team mapping
			homeScore := game.HomeScore
			awayScore := game.AwayScore
			if game.HasOdds() {
				// Simple logic: if away team picked and away team leading by more than spread
				// Or if home team picked and home team leading by more than spread
				// This is simplified - real implementation needs team ID mapping
				scoreDiff := homeScore - awayScore
				return scoreDiff > 0 // Simplified for now
			}
			return homeScore > awayScore
		},
		"dict": func(values ...interface{}) (map[string]interface{}, error) {
			if len(values)%2 != 0 {
				return nil, fmt.Errorf("dict: number of arguments must be even")
			}
			result := make(map[string]interface{})
			for i := 0; i < len(values); i += 2 {
				key, ok := values[i].(string)
				if !ok {
					return nil, fmt.Errorf("dict: key must be string, got %T", values[i])
				}
				result[key] = values[i+1]
			}
			return result, nil
		},
		"getResultClass": func(pick models.Pick, game *models.Game) string {
			baseClass := pick.GetResultClass()
			
			// Add state-specific classes for pending picks
			if baseClass == "pick-class" && game != nil {
				if game.State == models.GameStateInPlay {
					return baseClass + " in-progress"
				} else if game.State == models.GameStateScheduled {
					return baseClass + " pending"
				}
			}
			
			return baseClass
		},
		"isOverUnder": func(pick models.Pick) bool {
			return pick.IsOverUnder()
		},
		"isSpreadPick": func(pick models.Pick) bool {
			return pick.IsSpreadPick()
		},
		"lower": func(s string) string {
			return strings.ToLower(s)
		},
		"contains": func(s, substr string) bool {
			return strings.Contains(s, substr)
		},
		"regexReplace": func(input, pattern, replacement string) string {
			re := regexp.MustCompile(pattern)
			return re.ReplaceAllString(input, replacement)
		},
		"split": func(s, sep string) []string {
			return strings.Split(s, sep)
		},
		"getPickTeamAbbr": func(pick models.Pick, game *models.Game, pickDesc string) string {
			if pick.IsOverUnder() {
				if strings.Contains(pickDesc, "Over") {
					return "OVR"
				} else {
					return "UND"
				}
			}
			// For spread picks, return the team abbreviation
			if game != nil && strings.Contains(pick.TeamName, game.Home) {
				return game.Home
			} else if game != nil && strings.Contains(pick.TeamName, game.Away) {
				return game.Away
			}
			return pick.TeamName
		},
		"getPickTeamIcon": func(teamAbbr string) string {
			if teamAbbr == "OVR" {
				return "https://api.iconify.design/mdi/chevron-double-up.svg"
			}
			if teamAbbr == "UND" {
				return "https://api.iconify.design/mdi/chevron-double-down.svg"
			}
			if teamAbbr == "" {
				return ""
			}
			teamLower := strings.ToLower(teamAbbr)
			return fmt.Sprintf("https://a.espncdn.com/combiner/i?img=/i/teamlogos/nfl/500/scoreboard/%s.png", teamLower)
		},
		"getPickValue": func(pick models.Pick, game *models.Game, pickDesc string) string {
			if pick.IsOverUnder() && game != nil && game.HasOdds() {
				return fmt.Sprintf("%.1f", game.Odds.OU)
			}
			// For spread picks
			if game != nil && game.HasOdds() {
				if strings.Contains(pick.TeamName, game.Home) {
					return game.FormatHomeSpread()
				} else if strings.Contains(pick.TeamName, game.Away) {
					return game.FormatAwaySpread()
				}
			}
			return string(pick.PickType)
		},
		"getTeamMascotName": func(abbr string) string {
			mascotMap := map[string]string{
				"ARI": "CARDINALS", "ATL": "FALCONS", "BAL": "RAVENS", "BUF": "BILLS",
				"CAR": "PANTHERS", "CHI": "BEARS", "CIN": "BENGALS", "CLE": "BROWNS",
				"DAL": "COWBOYS", "DEN": "BRONCOS", "DET": "LIONS", "GB": "PACKERS",
				"HOU": "TEXANS", "IND": "COLTS", "JAX": "JAGUARS", "KC": "CHIEFS",
				"LV": "RAIDERS", "LAC": "CHARGERS", "LAR": "RAMS", "MIA": "DOLPHINS",
				"MIN": "VIKINGS", "NE": "PATRIOTS", "NO": "SAINTS", "NYG": "GIANTS",
				"NYJ": "JETS", "PHI": "EAGLES", "PIT": "STEELERS", "SF": "49ERS",
				"SEA": "SEAHAWKS", "TB": "BUCCANEERS", "TEN": "TITANS", "WSH": "COMMANDERS",
				"OVR": "OVR", "UND": "UND", // Keep O/U abbreviations as-is
			}
			if mascot, exists := mascotMap[abbr]; exists {
				return mascot
			}
			return abbr // Fallback to abbreviation if not found
		},
		"sortUsersByScore": func(userPicks []*models.UserPicks) []*models.UserPicks {
			if len(userPicks) == 0 {
				return userPicks
			}
			// Create a copy to avoid modifying original slice
			sorted := make([]*models.UserPicks, len(userPicks))
			copy(sorted, userPicks)
			
			// Sort by parlay points (descending - highest first)
			sort.Slice(sorted, func(i, j int) bool {
				scoreI := sorted[i].Record.ParlayPoints
				scoreJ := sorted[j].Record.ParlayPoints
				return scoreI > scoreJ
			})
			
			return sorted
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
	serverAddr := "0.0.0.0:" + serverPort
	
	if behindProxy {
		log.Printf("Server starting on %s (HTTP - behind proxy/tunnel)", serverAddr)
		log.Println("⚡ Configured for Cloudflare Tunnel or reverse proxy")
		log.Println("Default password for all users: password123")
		log.Fatal(http.ListenAndServe(serverAddr, r))
	} else if useTLS {
		log.Printf("Server starting on %s (HTTPS - available on LAN)", serverAddr)
		log.Printf("Visit: https://localhost:%s or https://[your-pi-ip]:%s", serverPort, serverPort)
		log.Printf("Login page: https://localhost:%s/login or https://[your-pi-ip]:%s/login", serverPort, serverPort)
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
		log.Printf("Server starting on %s (HTTP - available on LAN)", serverAddr)
		log.Printf("Visit: http://localhost:%s or http://[your-pi-ip]:%s", serverPort, serverPort)
		log.Printf("Login page: http://localhost:%s/login or http://[your-pi-ip]:%s/login", serverPort, serverPort)
		log.Println("Default password for all users: password123")
		log.Println("⚠️  HTTP mode - use only behind HTTPS proxy/tunnel")
		log.Fatal(http.ListenAndServe(serverAddr, r))
	}
}
