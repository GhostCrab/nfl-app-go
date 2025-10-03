package handlers

import (
	"encoding/json"
	"fmt"
	"html/template"
	"net/http"
	"nfl-app-go/config"
	"nfl-app-go/database"
	"nfl-app-go/logging"
	"nfl-app-go/middleware"
	"nfl-app-go/models"
	"nfl-app-go/services"
	"strconv"
	"time"
)

// GameDisplayHandler handles game listing and display functionality
// This handler renders the full dashboard view with games, picks, and user data
type GameDisplayHandler struct {
	templates         *template.Template
	gameService       services.GameService
	pickService       *services.PickService
	authService       *services.AuthService
	visibilityService *services.PickVisibilityService
	userRepo          *database.MongoUserRepository
	config            *config.Config
}

// NewGameDisplayHandler creates a new game display handler
func NewGameDisplayHandler(templates *template.Template, gameService services.GameService, cfg *config.Config) *GameDisplayHandler {
	return &GameDisplayHandler{
		templates:   templates,
		gameService: gameService,
		config:      cfg,
	}
}

// SetServices sets the optional services for full dashboard functionality
func (h *GameDisplayHandler) SetServices(pickService *services.PickService, authService *services.AuthService, visibilityService *services.PickVisibilityService, userRepo *database.MongoUserRepository) {
	h.pickService = pickService
	h.authService = authService
	h.visibilityService = visibilityService
	h.userRepo = userRepo
}

// GetGames handles the main games display page
// This endpoint renders the full dashboard with games, picks, and user data
func (h *GameDisplayHandler) GetGames(w http.ResponseWriter, r *http.Request) {
	logger := logging.WithPrefix("GameDisplay")
	logger.Debugf("HTTP: %s %s from %s", r.Method, r.URL.Path, r.RemoteAddr)

	// Parse query parameters for week filtering
	seasonStr := r.URL.Query().Get("season")
	weekStr := r.URL.Query().Get("week")

	// Handle debug datetime parameter for testing visibility (from original games.go)
	debugDateTimeStr := r.URL.Query().Get("datetime")
	if debugDateTimeStr != "" && h.visibilityService != nil {
		// Parse the datetime as Pacific time (not UTC)
		pacific, err := time.LoadLocation("America/Los_Angeles")
		if err != nil {
			logger.Debugf("Could not load Pacific timezone: %v", err)
		} else {
			if debugTime, err := time.ParseInLocation("2006-01-02T15:04", debugDateTimeStr, pacific); err == nil {
				h.visibilityService.SetDebugDateTime(debugTime)
				logger.Debugf("Set debug datetime to %v (parsed as Pacific time)", debugTime.Format("2006-01-02 15:04:05 MST"))
			}
		}
	}

	var season int
	var err error

	// Default to current season (2025)
	season = 2025
	if seasonStr != "" {
		if s, err := strconv.Atoi(seasonStr); err == nil && s >= 2020 && s <= 2030 {
			season = s
		}
	}

	// Get all games for season to determine current week
	allGames, err := h.gameService.GetGamesBySeason(season)
	if err != nil {
		logger.Errorf("Error fetching games for season %d: %v", season, err)
		http.Error(w, "Unable to fetch games", http.StatusInternalServerError)
		return
	}

	// Determine current week
	currentWeek := 0
	if weekStr != "" {
		if w, err := strconv.Atoi(weekStr); err == nil {
			currentWeek = w
		}
	}
	if currentWeek == 0 {
		currentWeek = h.getCurrentWeek(allGames)
		logger.Infof("Auto-detected current week: %d", currentWeek)
	}

	// Filter games by the determined current week
	filteredGames := make([]models.Game, 0)
	for _, game := range allGames {
		if game.Week == currentWeek {
			filteredGames = append(filteredGames, game)
		}
	}
	games := filteredGames

	// Sort games chronologically by kickoff time
	sortGamesByKickoffTime(games)

	// Get current user from context (if authenticated)
	user := middleware.GetUserFromContext(r)

	// Check if admin mode is enabled (only for user ID 4 - RYAN)
	isAdminMode := false
	if user != nil && user.ID == 4 && r.URL.Query().Get("admin") == "true" {
		isAdminMode = true
		logger.Infof("Admin mode enabled for user %s (ID: %d)", user.Name, user.ID)
	}

	// Generate week list (1-18 for regular season)
	weeks := generateWeekList()

	// Get all users
	var users []*models.User
	if h.userRepo != nil {
		allUsers, err := h.userRepo.GetAllUsers()
		if err != nil {
			logger.Errorf("Error getting users: %v", err)
		} else {
			// Convert slice to pointer slice
			users = make([]*models.User, len(allUsers))
			for i := range allUsers {
				users[i] = &allUsers[i]
			}
		}
	}

	// Get all user picks with proper visibility filtering (like original games.go)
	var userPicks []*models.UserPicks
	if h.pickService != nil {
		var err error
		userPicks, err = h.pickService.GetAllUserPicksForWeek(r.Context(), season, currentWeek)
		if err != nil {
			logger.Warnf("Failed to load picks for week %d, season %d: %v", currentWeek, season, err)
			userPicks = []*models.UserPicks{} // Empty picks on error
		}

		// Apply pick visibility filtering for security (critical for proper pick display)
		if h.visibilityService != nil {
			viewingUserID := -1 // Default for anonymous users
			if user != nil {
				viewingUserID = user.ID
			}

			filteredUserPicks, err := h.visibilityService.FilterVisibleUserPicks(r.Context(), userPicks, season, currentWeek, viewingUserID)
			if err != nil {
				logger.Warnf("Failed to filter pick visibility: %v", err)
			} else {
				userPicks = filteredUserPicks
				logger.Debugf("Applied pick visibility filtering for user ID %d", viewingUserID)
			}
		}

		// Ensure all users have a pick entry, even if empty
		userPicksMap := make(map[int]*models.UserPicks)
		for _, up := range userPicks {
			userPicksMap[up.UserID] = up
		}

		// Add empty pick entries for users who don't have picks this week
		// and populate their cumulative parlay scores
		for _, u := range users {
			if _, exists := userPicksMap[u.ID]; !exists {
				emptyPicks := &models.UserPicks{
					UserID:             u.ID,
					UserName:           u.Name,
					Picks:              []models.Pick{},
					BonusThursdayPicks: []models.Pick{},
					BonusFridayPicks:   []models.Pick{},
					Record:             models.UserRecord{},
				}
				userPicks = append(userPicks, emptyPicks)
			}
		}

		// Populate parlay scores for ALL users (including those with no picks)
		if err := h.pickService.PopulateParlayScores(r.Context(), userPicks, season, currentWeek); err != nil {
			logger.Warnf("Failed to populate parlay scores: %v", err)
		}

		// Populate daily pick groups for modern seasons (after visibility filtering)
		for _, up := range userPicks {
			up.PopulateDailyPickGroups(games, season)
		}
	} else {
		logger.Warn("No pick service available")
	}

	data := struct {
		Games             []models.Game
		Title             string
		User              *models.User
		Users             []*models.User
		UserPicks         []*models.UserPicks
		Weeks             []int
		CurrentWeek       int
		CurrentSeason     int
		DisplayIDTooltips bool
		IsAdminMode       bool
	}{
		Games:             games,
		Title:             fmt.Sprintf("PC '%d - Dashboard", season%100),
		User:              user,
		Users:             users,
		UserPicks:         userPicks,
		Weeks:             weeks,
		CurrentWeek:       currentWeek,
		CurrentSeason:     season,
		DisplayIDTooltips: h.config != nil && h.config.App.DisplayIDTooltips,
		IsAdminMode:       isAdminMode,
	}

	// Check if this is an HTMX request
	isHTMXRequest := r.Header.Get("HX-Request") == "true"

	var templateName string
	var contentType string

	if isHTMXRequest {
		// HTMX request - return only the dashboard content
		templateName = "dashboard-content"
		contentType = "text/html; charset=utf-8"
		logger.Infof("Serving HTMX partial content for week %d, season %d", currentWeek, season)
	} else {
		// Regular request - return full page
		templateName = "dashboard.html"
		contentType = "text/html; charset=utf-8"
		logger.Infof("Serving full dashboard page for week %d, season %d", currentWeek, season)
	}

	// Set content type
	w.Header().Set("Content-Type", contentType)

	// Execute the appropriate template
	err = h.templates.ExecuteTemplate(w, templateName, data)
	if err != nil {
		logger.Errorf("Template error (%s): %v", templateName, err)
		http.Error(w, "Error rendering page", http.StatusInternalServerError)
		return
	}

	logger.Debugf("Successfully served %s for season %d, week %d", templateName, season, currentWeek)
}

// RefreshGames handles HTMX refresh requests for the games list
// This endpoint returns updated game HTML fragments for real-time updates
func (h *GameDisplayHandler) RefreshGames(w http.ResponseWriter, r *http.Request) {
	logger := logging.WithPrefix("GameDisplay")
	logger.Debug("RefreshGames called")

	// Parse query parameters
	seasonStr := r.URL.Query().Get("season")
	weekStr := r.URL.Query().Get("week")

	var season, week int
	var err error

	// Parse season parameter
	if seasonStr != "" {
		season, err = strconv.Atoi(seasonStr)
		if err != nil {
			logger.Warnf("Invalid season in refresh: %s", seasonStr)
			http.Error(w, "Invalid season", http.StatusBadRequest)
			return
		}
	} else {
		season = time.Now().Year()
		if time.Now().Month() < time.March {
			season--
		}
	}

	// Parse week parameter
	if weekStr != "" {
		week, err = strconv.Atoi(weekStr)
		if err != nil {
			logger.Warnf("Invalid week in refresh: %s", weekStr)
			http.Error(w, "Invalid week", http.StatusBadRequest)
			return
		}
	} else {
		// Get current week
		allGames, err := h.gameService.GetGamesBySeason(season)
		if err != nil {
			logger.Errorf("Error getting games for current week calculation: %v", err)
			http.Error(w, "Error retrieving games", http.StatusInternalServerError)
			return
		}
		week = h.getCurrentWeek(allGames)
	}

	// Get fresh game data
	allGames, err := h.gameService.GetGamesBySeason(season)
	if err != nil {
		logger.Errorf("Error refreshing games for season %d: %v", season, err)
		http.Error(w, "Error retrieving games", http.StatusInternalServerError)
		return
	}

	// Filter games by week
	games := make([]models.Game, 0)
	for _, game := range allGames {
		if game.Week == week {
			games = append(games, game)
		}
	}

	// Sort games chronologically
	sortGamesByKickoffTime(games)

	// Return HTMX-compatible partial response
	data := struct {
		Games  []models.Game
		Season int
		Week   int
	}{
		Games:  games,
		Season: season,
		Week:   week,
	}

	// Render just the games list fragment for HTMX
	if err := h.templates.ExecuteTemplate(w, "games-list-fragment.html", data); err != nil {
		logger.Errorf("Error rendering games refresh template: %v", err)
		http.Error(w, "Error rendering games", http.StatusInternalServerError)
	}
}

// GetGamesAPI provides JSON API access to games data
// This endpoint is used by JavaScript clients and mobile applications
func (h *GameDisplayHandler) GetGamesAPI(w http.ResponseWriter, r *http.Request) {
	logger := logging.WithPrefix("GameDisplay")
	logger.Debug("GetGamesAPI called")

	// Set JSON content type
	w.Header().Set("Content-Type", "application/json")

	// Parse query parameters
	seasonStr := r.URL.Query().Get("season")
	weekStr := r.URL.Query().Get("week")

	var season, week int
	var err error

	// Parse season
	if seasonStr != "" {
		season, err = strconv.Atoi(seasonStr)
		if err != nil {
			logger.Warnf("Invalid season in API: %s", seasonStr)
			http.Error(w, `{"error":"Invalid season parameter"}`, http.StatusBadRequest)
			return
		}
	} else {
		season = time.Now().Year()
		if time.Now().Month() < time.March {
			season--
		}
	}

	// Parse week
	if weekStr != "" {
		week, err = strconv.Atoi(weekStr)
		if err != nil {
			logger.Warnf("Invalid week in API: %s", weekStr)
			http.Error(w, `{"error":"Invalid week parameter"}`, http.StatusBadRequest)
			return
		}
	} else {
		allGames, err := h.gameService.GetGamesBySeason(season)
		if err != nil {
			logger.Errorf("Error getting games for API current week: %v", err)
			http.Error(w, `{"error":"Error retrieving games"}`, http.StatusInternalServerError)
			return
		}
		week = h.getCurrentWeek(allGames)
	}

	// Get games data
	allGames, err := h.gameService.GetGamesBySeason(season)
	if err != nil {
		logger.Errorf("Error getting games for API season %d: %v", season, err)
		http.Error(w, `{"error":"Error retrieving games"}`, http.StatusInternalServerError)
		return
	}

	// Filter games by week
	games := make([]models.Game, 0)
	for _, game := range allGames {
		if game.Week == week {
			games = append(games, game)
		}
	}

	// Sort games chronologically
	sortGamesByKickoffTime(games)

	// Create API response
	response := struct {
		Games  []models.Game `json:"games"`
		Season int           `json:"season"`
		Week   int           `json:"week"`
		Count  int           `json:"count"`
	}{
		Games:  games,
		Season: season,
		Week:   week,
		Count:  len(games),
	}

	// Encode and send JSON response
	if err := json.NewEncoder(w).Encode(response); err != nil {
		logger.Errorf("Error encoding games API response: %v", err)
		http.Error(w, `{"error":"Error encoding response"}`, http.StatusInternalServerError)
	}
}

// getCurrentWeek determines the current NFL week using the proper GetNFLWeekForDate function
// This utility function helps determine which week to display by default
func (h *GameDisplayHandler) getCurrentWeek(games []models.Game) int {
	// Use the proper week calculation that accounts for NFL season timing
	if len(games) > 0 {
		// Use the season from the games
		return models.GetNFLWeekForDate(time.Now(), games[0].Season)
	}
	// Fallback to current year if no games available
	return models.GetNFLWeekForDate(time.Now(), time.Now().Year())
}

// generateWeekList creates a list of weeks 1-18 for template rendering
func generateWeekList() []int {
	weeks := make([]int, 18)
	for i := 0; i < 18; i++ {
		weeks[i] = i + 1
	}
	return weeks
}
