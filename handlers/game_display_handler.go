package handlers

import (
	"encoding/json"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"nfl-app-go/database"
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
}

// NewGameDisplayHandler creates a new game display handler
func NewGameDisplayHandler(templates *template.Template, gameService services.GameService) *GameDisplayHandler {
	return &GameDisplayHandler{
		templates:   templates,
		gameService: gameService,
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
	log.Printf("HTTP: %s %s from %s", r.Method, r.URL.Path, r.RemoteAddr)

	// Parse query parameters for week filtering
	seasonStr := r.URL.Query().Get("season")
	weekStr := r.URL.Query().Get("week")
	
	// Handle debug datetime parameter for testing visibility (from original games.go)
	debugDateTimeStr := r.URL.Query().Get("datetime")
	if debugDateTimeStr != "" && h.visibilityService != nil {
		// Parse the datetime as Pacific time (not UTC)
		pacific, err := time.LoadLocation("America/Los_Angeles")
		if err != nil {
			log.Printf("DEBUG: Could not load Pacific timezone: %v", err)
		} else {
			if debugTime, err := time.ParseInLocation("2006-01-02T15:04", debugDateTimeStr, pacific); err == nil {
				h.visibilityService.SetDebugDateTime(debugTime)
				log.Printf("DEBUG: Set debug datetime to %v (parsed as Pacific time)", debugTime.Format("2006-01-02 15:04:05 MST"))
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
		log.Printf("GameHandler: Error fetching games for season %d: %v", season, err)
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
		log.Printf("GameHandler: Auto-detected current week: %d", currentWeek)
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
	
	// Generate week list (1-18 for regular season)
	weeks := generateWeekList()
	
	// Get all users
	var users []*models.User
	if h.userRepo != nil {
		allUsers, err := h.userRepo.GetAllUsers()
		if err != nil {
			log.Printf("GameHandler: Error getting users: %v", err)
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
			log.Printf("GameHandler: Warning - failed to load picks for week %d, season %d: %v", currentWeek, season, err)
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
				log.Printf("GameHandler: Warning - failed to filter pick visibility: %v", err)
			} else {
				userPicks = filteredUserPicks
				log.Printf("GameHandler: Applied pick visibility filtering for user ID %d", viewingUserID)
			}
		}

		// Ensure all users have a pick entry, even if empty
		userPicksMap := make(map[int]*models.UserPicks)
		for _, up := range userPicks {
			userPicksMap[up.UserID] = up
		}

		// Add empty pick entries for users who don't have picks this week
		for _, u := range users {
			if _, exists := userPicksMap[u.ID]; !exists {
				userPicks = append(userPicks, &models.UserPicks{
					UserID:             u.ID,
					UserName:           u.Name,
					Picks:              []models.Pick{},
					BonusThursdayPicks: []models.Pick{},
					BonusFridayPicks:   []models.Pick{},
					Record:             models.UserRecord{},
				})
			}
		}
		
		// Populate daily pick groups for modern seasons (after visibility filtering)
		for _, up := range userPicks {
			up.PopulateDailyPickGroups(games, season)
		}
	} else {
		log.Printf("GameHandler: WARNING - No pick service available")
	}
	
	data := struct {
		Games         []models.Game
		Title         string
		User          *models.User
		Users         []*models.User
		UserPicks     []*models.UserPicks
		Weeks         []int
		CurrentWeek   int
		CurrentSeason int
	}{
		Games:         games,
		Title:         fmt.Sprintf("PC '%d - Dashboard", season%100),
		User:          user,
		Users:         users,
		UserPicks:     userPicks,
		Weeks:         weeks,
		CurrentWeek:   currentWeek,
		CurrentSeason: season,
	}
	
	// Check if this is an HTMX request
	isHTMXRequest := r.Header.Get("HX-Request") == "true"
	
	var templateName string
	var contentType string
	
	if isHTMXRequest {
		// HTMX request - return only the dashboard content
		templateName = "dashboard-content"
		contentType = "text/html; charset=utf-8"
		log.Printf("HTTP: Serving HTMX partial content for week %d, season %d", currentWeek, season)
	} else {
		// Regular request - return full page
		templateName = "dashboard.html"
		contentType = "text/html; charset=utf-8"
		log.Printf("HTTP: Serving full dashboard page for week %d, season %d", currentWeek, season)
	}
	
	// Set content type
	w.Header().Set("Content-Type", contentType)
	
	// Execute the appropriate template
	err = h.templates.ExecuteTemplate(w, templateName, data)
	if err != nil {
		log.Printf("GameHandler: Template error (%s): %v", templateName, err)
		http.Error(w, "Error rendering page", http.StatusInternalServerError)
		return
	}
	
	log.Printf("HTTP: Successfully served %s for season %d, week %d", templateName, season, currentWeek)
}

// RefreshGames handles HTMX refresh requests for the games list
// This endpoint returns updated game HTML fragments for real-time updates
func (h *GameDisplayHandler) RefreshGames(w http.ResponseWriter, r *http.Request) {
	log.Println("RefreshGames called")
	
	// Parse query parameters
	seasonStr := r.URL.Query().Get("season")
	weekStr := r.URL.Query().Get("week")
	
	var season, week int
	var err error
	
	// Parse season parameter
	if seasonStr != "" {
		season, err = strconv.Atoi(seasonStr)
		if err != nil {
			log.Printf("Invalid season in refresh: %s", seasonStr)
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
			log.Printf("Invalid week in refresh: %s", weekStr)
			http.Error(w, "Invalid week", http.StatusBadRequest)
			return
		}
	} else {
		// Get current week
		allGames, err := h.gameService.GetGamesBySeason(season)
		if err != nil {
			log.Printf("Error getting games for current week calculation: %v", err)
			http.Error(w, "Error retrieving games", http.StatusInternalServerError)
			return
		}
		week = h.getCurrentWeek(allGames)
	}
	
	// Get fresh game data
	allGames, err := h.gameService.GetGamesBySeason(season)
	if err != nil {
		log.Printf("Error refreshing games for season %d: %v", season, err)
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
		log.Printf("Error rendering games refresh template: %v", err)
		http.Error(w, "Error rendering games", http.StatusInternalServerError)
	}
}

// GetGamesAPI provides JSON API access to games data
// This endpoint is used by JavaScript clients and mobile applications
func (h *GameDisplayHandler) GetGamesAPI(w http.ResponseWriter, r *http.Request) {
	log.Println("GetGamesAPI called")
	
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
			log.Printf("Invalid season in API: %s", seasonStr)
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
			log.Printf("Invalid week in API: %s", weekStr)
			http.Error(w, `{"error":"Invalid week parameter"}`, http.StatusBadRequest)
			return
		}
	} else {
		allGames, err := h.gameService.GetGamesBySeason(season)
		if err != nil {
			log.Printf("Error getting games for API current week: %v", err)
			http.Error(w, `{"error":"Error retrieving games"}`, http.StatusInternalServerError)
			return
		}
		week = h.getCurrentWeek(allGames)
	}
	
	// Get games data
	allGames, err := h.gameService.GetGamesBySeason(season)
	if err != nil {
		log.Printf("Error getting games for API season %d: %v", season, err)
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
		log.Printf("Error encoding games API response: %v", err)
		http.Error(w, `{"error":"Error encoding response"}`, http.StatusInternalServerError)
	}
}

// getCurrentWeek determines the current NFL week based on game dates
// This utility function helps determine which week to display by default
func (h *GameDisplayHandler) getCurrentWeek(games []models.Game) int {
	if len(games) == 0 {
		return 1 // Default to week 1 if no games
	}
	
	now := time.Now()
	
	// Group games by week
	weekGames := make(map[int][]models.Game)
	for _, game := range games {
		weekGames[game.Week] = append(weekGames[game.Week], game)
	}
	
	// Find the current week based on game timing
	for week := 1; week <= 18; week++ {
		gamesThisWeek := weekGames[week]
		if len(gamesThisWeek) == 0 {
			continue
		}
		
		// Check if any games this week are in progress or recently completed
		hasRecentActivity := false
		allCompleted := true
		
		for _, game := range gamesThisWeek {
			timeSinceKickoff := now.Sub(game.Date)
			
			// Game is recent if it's within 4 hours of kickoff (before or after)
			if timeSinceKickoff >= -4*time.Hour && timeSinceKickoff <= 4*time.Hour {
				hasRecentActivity = true
			}
			
			// Check if game is not completed
			if !game.IsCompleted() {
				allCompleted = false
			}
		}
		
		// If there's recent activity or not all games are completed, this is likely the current week
		if hasRecentActivity || !allCompleted {
			log.Printf("Determined current week as %d based on game activity", week)
			return week
		}
	}
	
	// Fallback: find the first week with incomplete games
	for week := 1; week <= 18; week++ {
		gamesThisWeek := weekGames[week]
		for _, game := range gamesThisWeek {
			if !game.IsCompleted() {
				log.Printf("Determined current week as %d based on incomplete games", week)
				return week
			}
		}
	}
	
	// Ultimate fallback: return week 1
	log.Println("Using fallback current week: 1")
	return 1
}

// generateWeekList creates a list of weeks 1-18 for template rendering
func generateWeekList() []int {
	weeks := make([]int, 18)
	for i := 0; i < 18; i++ {
		weeks[i] = i + 1
	}
	return weeks
}

