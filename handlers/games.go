package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"nfl-app-go/middleware"
	"nfl-app-go/models"
	"nfl-app-go/services"
	"strconv"
	"strings"
	"time"
)

// GameHandler handles game-related HTTP requests
// SSEClient represents a connected SSE client with user context
type SSEClient struct {
	Channel chan string
	UserID  int
}

type GameHandler struct {
	templates   *template.Template
	gameService services.GameService
	pickService *services.PickService
	sseClients  map[*SSEClient]bool
	authService *services.AuthService
	dataLoader  *services.DataLoader
}

// NewGameHandler creates a new game handler
func NewGameHandler(templates *template.Template, gameService services.GameService) *GameHandler {
	return &GameHandler{
		templates:   templates,
		gameService: gameService,
		sseClients:  make(map[*SSEClient]bool),
	}
}

// SetPickService sets the pick service for pick operations
func (h *GameHandler) SetPickService(pickService *services.PickService) {
	h.pickService = pickService
}

// SetAuthService sets the auth service for user operations
func (h *GameHandler) SetAuthService(authService *services.AuthService) {
	h.authService = authService
}

// GetGames handles GET / and /games - displays dashboard
func (h *GameHandler) GetGames(w http.ResponseWriter, r *http.Request) {
	log.Printf("HTTP: %s %s from %s", r.Method, r.URL.Path, r.RemoteAddr)
	
	// Get week filter from query params
	weekStr := r.URL.Query().Get("week")
	var selectedWeek int
	if weekStr != "" {
		if w, err := strconv.Atoi(weekStr); err == nil {
			selectedWeek = w
		}
	}
	
	// Get season from query params, default to 2025
	seasonStr := r.URL.Query().Get("season")
	season := 2025 // Default to current season
	if seasonStr != "" {
		if s, err := strconv.Atoi(seasonStr); err == nil && s >= 2020 && s <= 2030 {
			season = s
		}
	}
	
	var games []models.Game
	var err error
	
	// Use GameService interface method that supports seasons
	if gameServiceWithSeason, ok := h.gameService.(interface{ GetGamesBySeason(int) ([]models.Game, error) }); ok {
		games, err = gameServiceWithSeason.GetGamesBySeason(season)
	} else {
		games, err = h.gameService.GetGames()
	}
	
	if err != nil {
		log.Printf("GameHandler: Error fetching games for season %d: %v", season, err)
		http.Error(w, "Unable to fetch games", http.StatusInternalServerError)
		return
	}
	
	// Determine current week (use selected or auto-detect current week)
	currentWeek := selectedWeek
	if currentWeek == 0 {
		currentWeek = h.getCurrentWeek(games)
		log.Printf("GameHandler: Auto-detected current week: %d", currentWeek)
	}
	
	// Always filter games by the determined current week
	filteredGames := make([]models.Game, 0)
	for _, game := range games {
		if game.Week == currentWeek {
			filteredGames = append(filteredGames, game)
		}
	}
	games = filteredGames
	
	// Get current user from context (if authenticated)
	user := middleware.GetUserFromContext(r)
	
	// Generate week list (1-18 for regular season)
	weeks := make([]int, 18)
	for i := range weeks {
		weeks[i] = i + 1
	}
	
	
	// Get all users from database instead of mock users
	var users []*models.User
	// For now, skip trying to get users from authService since we need to access private userRepo field
	
	// Fallback to database users if authService users failed
	if len(users) == 0 {
		// Use actual database users (matching the imported picks)
		users = []*models.User{
			{ID: 0, Name: "ANDREW", Email: "ackilpatrick@gmail.com"},
			{ID: 1, Name: "BARDIA", Email: "bbakhtari@gmail.com"},
			{ID: 2, Name: "COOPER", Email: "cooper.kocsis@mattel.com"},
			{ID: 3, Name: "MICAH", Email: "micahgoldman@gmail.com"},
			{ID: 4, Name: "RYAN", Email: "ryan.pielow@gmail.com"},
			{ID: 5, Name: "TJ", Email: "tyerke@yahoo.com"},
			{ID: 6, Name: "BRAD", Email: "bradvassar@gmail.com"},
		}
	}
	
	// Load pick data for the current week if pick service is available
	var userPicks []*models.UserPicks
	if h.pickService != nil {
		var err error
		userPicks, err = h.pickService.GetAllUserPicksForWeek(r.Context(), season, currentWeek)
		if err != nil {
			log.Printf("GameHandler: Warning - failed to load picks for week %d, season %d: %v", currentWeek, season, err)
			userPicks = []*models.UserPicks{} // Empty picks on error
		}
		// Apply demo effects to picks for Week 1 games (but avoid "pending" string)
		userPicks = h.applyDemoEffectsToPicksForWeek1(userPicks)
		
		// Ensure all users have a pick entry, even if empty
		userPicksMap := make(map[int]*models.UserPicks)
		for _, up := range userPicks {
			userPicksMap[up.UserID] = up
		}
		
		// Add empty pick entries for users who don't have picks this week
		for _, user := range users {
			if _, exists := userPicksMap[user.ID]; !exists {
				userPicks = append(userPicks, &models.UserPicks{
					UserID:               user.ID,
					UserName:             user.Name,
					Picks:                []models.Pick{},
					BonusThursdayPicks:   []models.Pick{},
					BonusFridayPicks:     []models.Pick{},
					Record:               models.UserRecord{},
				})
			}
		}
	} else {
		log.Printf("GameHandler: WARNING - No pick service available")
	}
	
	
	
	data := struct {
		Games       []models.Game
		Title       string
		User        *models.User
		Users       []*models.User
		UserPicks   []*models.UserPicks
		Weeks       []int
		CurrentWeek int
		CurrentSeason int
	}{
		Games:         games,
		Title:         fmt.Sprintf("PC '%d - Dashboard", season%100), // Show last 2 digits of season
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
		log.Printf("HTTP: Serving full page for week %d, season %d", currentWeek, season)
	}
	
	// Set content type
	w.Header().Set("Content-Type", contentType)
	
	// Execute the appropriate template
	err = h.templates.ExecuteTemplate(w, templateName, data)
	if err != nil {
		log.Printf("GameHandler: Template error (%s): %v", templateName, err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	
	log.Printf("HTTP: Successfully served %s %s", r.Method, r.URL.Path)
	log.Printf("DEBUG: Template data - UserPicks count: %d, User: %v", len(userPicks), user)
}

// SSEHandler handles Server-Sent Events for real-time game updates
func (h *GameHandler) SSEHandler(w http.ResponseWriter, r *http.Request) {
	// Get user from context (if authenticated)
	user := middleware.GetUserFromContext(r)
	userID := 0 // Default to user 0 if not authenticated
	if user != nil {
		userID = user.ID
	}
	
	log.Printf("SSE: New client connected from %s (UserID: %d)", r.RemoteAddr, userID)

	// Set SSE headers
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	// Create client with user context
	client := &SSEClient{
		Channel: make(chan string, 10),
		UserID:  userID,
	}
	h.sseClients[client] = true

	// Send initial connection confirmation
	fmt.Fprintf(w, "event: connected\n")
	fmt.Fprintf(w, "data: SSE connection established for user %d\n\n", userID)
	w.(http.Flusher).Flush()

	// Handle client disconnect
	defer func() {
		delete(h.sseClients, client)
		close(client.Channel)
		log.Printf("SSE: Client disconnected from %s (UserID: %d)", r.RemoteAddr, userID)
	}()

	// Keep connection alive and send updates
	for {
		select {
		case message := <-client.Channel:
			// Parse message format: "eventType:data"
			parts := strings.SplitN(message, ":", 2)
			eventType := "gameUpdate" // default
			data := message
			
			if len(parts) == 2 {
				eventType = parts[0]
				data = parts[1]
			}
			
			fmt.Fprintf(w, "event: %s\n", eventType)
			// Split data into lines and prefix each with "data: "
			lines := strings.Split(data, "\n")
			for _, line := range lines {
				fmt.Fprintf(w, "data: %s\n", line)
			}
			fmt.Fprintf(w, "\n")
			w.(http.Flusher).Flush()
		case <-r.Context().Done():
			return
		case <-time.After(30 * time.Second):
			// Send keepalive
			fmt.Fprintf(w, "event: keepalive\n")
			fmt.Fprintf(w, "data: ping\n\n")
			w.(http.Flusher).Flush()
		}
	}
}

// HandleDatabaseChange processes database changes and sends appropriate updates to SSE clients
func (h *GameHandler) HandleDatabaseChange(event services.ChangeEvent) {
	log.Printf("SSE: Processing change event: Collection=%s, Operation=%s, Season=%d, Week=%d, UserID=%d", 
		event.Collection, event.Operation, event.Season, event.Week, event.UserID)

	// Skip pick changes - these are now handled directly by SubmitPicks handler
	if event.Collection == "picks" {
		log.Printf("SSE: Skipping pick change event (handled by SubmitPicks)")
		return
	}
	
	// For other collections (like games), send structured database change events
	if event.Collection == "games" {
		updateEvent := struct {
			Type       string `json:"type"`
			Collection string `json:"collection"`
			Operation  string `json:"operation"`
			Season     int    `json:"season"`
			Week       int    `json:"week"`
			GameID     string `json:"gameId,omitempty"`
			Timestamp  int64  `json:"timestamp"`
		}{
			Type:       "databaseChange",
			Collection: event.Collection,
			Operation:  event.Operation,
			Season:     event.Season,
			Week:       event.Week,
			GameID:     event.GameID,
			Timestamp:  time.Now().UnixMilli(),
		}

		// Convert to JSON
		updateJSON, err := json.Marshal(updateEvent)
		if err != nil {
			log.Printf("SSE: Error marshaling update event: %v", err)
			return
		}

		// Send structured update to all connected clients
		h.broadcastStructuredUpdate("gameUpdate", string(updateJSON))
	}
}

// BroadcastUpdate sends game updates to all connected SSE clients (legacy method)
func (h *GameHandler) BroadcastUpdate() {
	games, err := h.gameService.GetGames()
	if err != nil {
		log.Printf("SSE: Error fetching games for broadcast: %v", err)
		return
	}

	// Render games HTML
	data := struct {
		Games []models.Game
	}{
		Games: games,
	}

	// Use a buffer to capture template output
	var htmlBuffer strings.Builder
	err = h.templates.ExecuteTemplate(&htmlBuffer, "game-grid", data)
	if err != nil {
		log.Printf("SSE: Template error for broadcast: %v", err)
		return
	}

	htmlContent := htmlBuffer.String()
	log.Printf("SSE: Generated HTML content length: %d", len(htmlContent))
	
	if len(htmlContent) == 0 {
		log.Printf("SSE: Warning - HTML content is empty!")
		return
	}
	
	// Send HTML update to all connected clients
	h.broadcastStructuredUpdate("gameUpdate", htmlContent)
}

// broadcastStructuredUpdate sends a structured update to all SSE clients
func (h *GameHandler) broadcastStructuredUpdate(eventType, data string) {
	for client := range h.sseClients {
		select {
		case client.Channel <- fmt.Sprintf("%s:%s", eventType, data):
		default:
			// Client channel is full, skip
		}
	}
	
	log.Printf("SSE: Broadcasted %s update to %d clients", eventType, len(h.sseClients))
}

// broadcastToUser sends a structured update to a specific user
func (h *GameHandler) broadcastToUser(userID int, eventType, data string) {
	count := 0
	for client := range h.sseClients {
		if client.UserID == userID {
			select {
			case client.Channel <- fmt.Sprintf("%s:%s", eventType, data):
				count++
			default:
				// Client channel is full, skip
			}
		}
	}
	
	log.Printf("SSE: Broadcasted %s update to %d clients for user %d", eventType, count, userID)
}

// getCurrentWeek determines the current NFL week based on game dates and season state
func (h *GameHandler) getCurrentWeek(games []models.Game) int {
	now := time.Now()
	
	if len(games) == 0 {
		return 1
	}
	
	// Find earliest and latest game dates across all weeks
	var earliestGame, latestGame time.Time
	weekGames := make(map[int][]models.Game)
	
	for _, game := range games {
		if earliestGame.IsZero() || game.Date.Before(earliestGame) {
			earliestGame = game.Date
		}
		if latestGame.IsZero() || game.Date.After(latestGame) {
			latestGame = game.Date
		}
		
		weekGames[game.Week] = append(weekGames[game.Week], game)
	}
	
	// If current time is before the season starts, show Week 1
	if now.Before(earliestGame) {
		log.Printf("getCurrentWeek: Season hasn't started yet (earliest: %v), showing Week 1", earliestGame.Format("Jan 2, 2006"))
		return 1
	}
	
	// If current time is after the season ends, show Week 18
	if now.After(latestGame.Add(7 * 24 * time.Hour)) { // Add 7 days buffer after last game
		log.Printf("getCurrentWeek: Season has ended (latest: %v), showing Week 18", latestGame.Format("Jan 2, 2006"))
		return 18
	}
	
	// Find the current week by checking which week we're in
	// Look for the week where:
	// 1. Current time is after the week's first game started, OR
	// 2. Current time is within 3 days before the week's first game
	
	for week := 1; week <= 18; week++ {
		weekGamesList, exists := weekGames[week]
		if !exists || len(weekGamesList) == 0 {
			continue
		}
		
		// Find earliest game in this week
		var earliestInWeek time.Time
		for _, game := range weekGamesList {
			if earliestInWeek.IsZero() || game.Date.Before(earliestInWeek) {
				earliestInWeek = game.Date
			}
		}
		
		// If we're within 3 days before this week's first game, or after it started
		threeDaysBefore := earliestInWeek.Add(-3 * 24 * time.Hour)
		if now.After(threeDaysBefore) && now.Before(earliestInWeek.Add(7 * 24 * time.Hour)) {
			log.Printf("getCurrentWeek: Current time within Week %d window (games start: %v), showing Week %d", 
				week, earliestInWeek.Format("Jan 2, 2006 15:04"), week)
			return week
		}
	}
	
	// Fallback: find the week with games closest to current time
	currentWeek := 1
	minTimeDiff := time.Duration(999999999999999) // Very large duration
	
	for week, weekGamesList := range weekGames {
		for _, game := range weekGamesList {
			timeDiff := game.Date.Sub(now)
			if timeDiff < 0 {
				timeDiff = -timeDiff
			}
			
			if timeDiff < minTimeDiff {
				minTimeDiff = timeDiff
				currentWeek = week
			}
		}
	}
	
	log.Printf("getCurrentWeek: Using fallback logic, closest week: %d", currentWeek)
	return currentWeek
}

// applyDemoEffectsToPicksForWeek1 modifies picks for Week 1 games to appear as pending/in-progress
func (h *GameHandler) applyDemoEffectsToPicksForWeek1(userPicks []*models.UserPicks) []*models.UserPicks {
	if len(userPicks) == 0 {
		return userPicks
	}

	// Create a copy to avoid modifying original data
	demoUserPicks := make([]*models.UserPicks, len(userPicks))
	for i, userPicksPtr := range userPicks {
		if userPicksPtr == nil {
			continue
		}
		
		// Create a copy of the UserPicks struct
		userPicksCopy := *userPicksPtr
		
		// Apply demo effects to all pick categories
		userPicksCopy.Picks = h.applyDemoEffectsToPicks(userPicksCopy.Picks)
		userPicksCopy.SpreadPicks = h.applyDemoEffectsToPicks(userPicksCopy.SpreadPicks)
		userPicksCopy.OverUnderPicks = h.applyDemoEffectsToPicks(userPicksCopy.OverUnderPicks)
		userPicksCopy.BonusThursdayPicks = h.applyDemoEffectsToPicks(userPicksCopy.BonusThursdayPicks)
		userPicksCopy.BonusFridayPicks = h.applyDemoEffectsToPicks(userPicksCopy.BonusFridayPicks)
		
		demoUserPicks[i] = &userPicksCopy
	}
	
	return demoUserPicks
}

// applyDemoEffectsToPicks modifies individual picks for Week 1 games to appear pending
func (h *GameHandler) applyDemoEffectsToPicks(picks []models.Pick) []models.Pick {
	if len(picks) == 0 {
		return picks
	}
	
	demoPicks := make([]models.Pick, len(picks))
	for i, pick := range picks {
		// Copy the pick
		demoPicks[i] = pick
		
		// For Week 1 picks, make them appear as in-progress (use empty string to avoid "pending")
		if pick.Week == 1 && (pick.Result == models.PickResultWin || pick.Result == models.PickResultLoss || pick.Result == models.PickResultPush) {
			demoPicks[i].Result = models.PickResult("") // Empty string instead of "pending"
		}
	}
	
	return demoPicks
}

// GetGamesAPI handles GET /api/games - returns just the games grid HTML for AJAX requests
func (h *GameHandler) GetGamesAPI(w http.ResponseWriter, r *http.Request) {
	
	// Get week filter from query params
	weekStr := r.URL.Query().Get("week")
	var selectedWeek int
	if weekStr != "" {
		if w, err := strconv.Atoi(weekStr); err == nil {
			selectedWeek = w
		}
	}
	
	// Get season from query params, default to 2025
	seasonStr := r.URL.Query().Get("season")
	season := 2025
	if seasonStr != "" {
		if s, err := strconv.Atoi(seasonStr); err == nil && s >= 2020 && s <= 2030 {
			season = s
		}
	}
	
	var games []models.Game
	var err error
	
	// Use GameService interface method that supports seasons
	if gameServiceWithSeason, ok := h.gameService.(interface{ GetGamesBySeason(int) ([]models.Game, error) }); ok {
		games, err = gameServiceWithSeason.GetGamesBySeason(season)
	} else {
		games, err = h.gameService.GetGames()
	}
	
	if err != nil {
		log.Printf("API: Error fetching games for season %d: %v", season, err)
		http.Error(w, "Unable to fetch games", http.StatusInternalServerError)
		return
	}
	
	// Filter by week if specified
	if selectedWeek > 0 {
		filteredGames := make([]models.Game, 0)
		for _, game := range games {
			if game.Week == selectedWeek {
				filteredGames = append(filteredGames, game)
			}
		}
		games = filteredGames
	}
	
	log.Printf("API: Returning %d games for week %d, season %d", len(games), selectedWeek, season)
	
	// Return just the game grid template
	data := struct {
		Games []models.Game
	}{
		Games: games,
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	err = h.templates.ExecuteTemplate(w, "game-grid", data)
	if err != nil {
		log.Printf("API: Template error: %v", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
}

// GetDashboardDataAPI handles GET /api/dashboard - returns complete dashboard data including games and picks
func (h *GameHandler) GetDashboardDataAPI(w http.ResponseWriter, r *http.Request) {
	
	// Get week filter from query params
	weekStr := r.URL.Query().Get("week")
	var selectedWeek int
	if weekStr != "" {
		if w, err := strconv.Atoi(weekStr); err == nil {
			selectedWeek = w
		}
	}
	
	// Get season from query params, default to 2025
	seasonStr := r.URL.Query().Get("season")
	season := 2025
	if seasonStr != "" {
		if s, err := strconv.Atoi(seasonStr); err == nil && s >= 2020 && s <= 2030 {
			season = s
		}
	}
	
	var games []models.Game
	var err error
	
	// Use GameService interface method that supports seasons
	if gameServiceWithSeason, ok := h.gameService.(interface{ GetGamesBySeason(int) ([]models.Game, error) }); ok {
		games, err = gameServiceWithSeason.GetGamesBySeason(season)
	} else {
		games, err = h.gameService.GetGames()
	}
	
	if err != nil {
		log.Printf("API: Error fetching games for season %d: %v", season, err)
		http.Error(w, "Unable to fetch games", http.StatusInternalServerError)
		return
	}
	
	// Determine current week (use selected or auto-detect current week)
	currentWeek := selectedWeek
	if currentWeek == 0 {
		currentWeek = h.getCurrentWeek(games)
		log.Printf("API: Auto-detected current week: %d", currentWeek)
	}
	
	// Always filter games by the determined current week
	filteredGames := make([]models.Game, 0)
	for _, game := range games {
		if game.Week == currentWeek {
			filteredGames = append(filteredGames, game)
		}
	}
	games = filteredGames
	
	// Get current user from context (if authenticated)
	user := middleware.GetUserFromContext(r)
	
	// Generate week list (1-18 for regular season)
	weeks := make([]int, 18)
	for i := range weeks {
		weeks[i] = i + 1
	}
	
	// Get all users
	users := []*models.User{
		{ID: 0, Name: "ANDREW", Email: "ackilpatrick@gmail.com"},
		{ID: 1, Name: "BARDIA", Email: "bbakhtari@gmail.com"},
		{ID: 2, Name: "COOPER", Email: "cooper.kocsis@mattel.com"},
		{ID: 3, Name: "MICAH", Email: "micahgoldman@gmail.com"},
		{ID: 4, Name: "RYAN", Email: "ryan.pielow@gmail.com"},
		{ID: 5, Name: "TJ", Email: "tyerke@yahoo.com"},
		{ID: 6, Name: "BRAD", Email: "bradvassar@gmail.com"},
	}
	
	// Load pick data for the current week if pick service is available
	var userPicks []*models.UserPicks
	if h.pickService != nil {
		var err error
		userPicks, err = h.pickService.GetAllUserPicksForWeek(r.Context(), season, currentWeek)
		if err != nil {
			log.Printf("API: Warning - failed to load picks for week %d, season %d: %v", currentWeek, season, err)
			userPicks = []*models.UserPicks{} // Empty picks on error
		}
		// Apply demo effects to picks for Week 1 games (but avoid "pending" string)
		userPicks = h.applyDemoEffectsToPicksForWeek1(userPicks)
	} else {
		log.Printf("API: WARNING - No pick service available")
	}
	
	// Create response data structure
	data := struct {
		Games       []models.Game         `json:"games"`
		UserPicks   []*models.UserPicks   `json:"userPicks"`
		Users       []*models.User        `json:"users"`
		User        *models.User          `json:"user"`
		CurrentWeek int                   `json:"currentWeek"`
		Season      int                   `json:"season"`
		Weeks       []int                 `json:"weeks"`
		Title       string                `json:"title"`
	}{
		Games:       games,
		UserPicks:   userPicks,
		Users:       users,
		User:        user,
		CurrentWeek: currentWeek,
		Season:      season,
		Weeks:       weeks,
		Title:       fmt.Sprintf("PC '%d - Dashboard", season%100),
	}
	
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(data); err != nil {
		log.Printf("API: JSON encoding error: %v", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	
}

// ShowPickPicker displays the pick picker modal/overlay
func (h *GameHandler) ShowPickPicker(w http.ResponseWriter, r *http.Request) {
	log.Printf("PICK-PICKER: %s %s from %s", r.Method, r.URL.Path, r.RemoteAddr)
	log.Printf("PICK-PICKER: Query params: %v", r.URL.RawQuery)
	
	// Get week from query parameters
	weekParam := r.URL.Query().Get("week")
	week, err := strconv.Atoi(weekParam)
	if err != nil || week < 1 || week > 18 {
		week = h.getCurrentWeek([]models.Game{}) // Default to current week
	}
	
	// Get season from query parameters
	seasonParam := r.URL.Query().Get("season")
	season, err := strconv.Atoi(seasonParam)
	if err != nil || season < 2020 || season > 2030 {
		season = 2025 // Default to current season
	}
	
	// Get user from context (set by auth middleware)
	user := middleware.GetUserFromContext(r)
	
	// Get games for the season
	var allGames []models.Game
	
	// Use GameService interface method that supports seasons
	if gameServiceWithSeason, ok := h.gameService.(interface{ GetGamesBySeason(int) ([]models.Game, error) }); ok {
		allGames, err = gameServiceWithSeason.GetGamesBySeason(season)
	} else {
		allGames, err = h.gameService.GetGames()
	}
	
	if err != nil {
		log.Printf("Error getting games for week %d, season %d: %v", week, season, err)
		http.Error(w, "Failed to load games", http.StatusInternalServerError)
		return
	}
	
	// Filter games by week and season, and only show scheduled games (can't pick games that started)
	var availableGames []models.Game
	for _, game := range allGames {
		if game.Week == week && game.Season == season && game.State == models.GameStateScheduled {
			availableGames = append(availableGames, game)
		}
	}
	
	// Get current user picks for this week if user is authenticated
	var userPicks *models.UserPicks
	if user != nil && h.pickService != nil {
		picks, err := h.pickService.GetUserPicksForWeek(context.Background(), user.ID, season, week)
		if err != nil {
			log.Printf("Error getting user picks: %v", err)
			// Continue without error - user just won't see current picks
		} else {
			userPicks = picks
		}
	}
	
	// Create pick state map for template
	pickState := make(map[int]map[int]bool) // pickState[gameID][teamID] = selected
	if userPicks != nil {
		allPicks := append(userPicks.Picks, userPicks.SpreadPicks...)
		allPicks = append(allPicks, userPicks.OverUnderPicks...)
		for _, pick := range allPicks {
			if pickState[pick.GameID] == nil {
				pickState[pick.GameID] = make(map[int]bool)
			}
			pickState[pick.GameID][pick.TeamID] = true
		}
	}
	
	data := struct {
		Games     []models.Game
		Week      int
		Season    int
		User      *models.User
		PickState map[int]map[int]bool
	}{
		Games:     availableGames,
		Week:      week,
		Season:    season,
		User:      user,
		PickState: pickState,
	}
	
	// Set content type for HTML response
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	
	log.Printf("PICK-PICKER: Rendering template with %d games for week %d, season %d", len(availableGames), week, season)
	
	// Render pick picker template
	err = h.templates.ExecuteTemplate(w, "pick-picker", data)
	if err != nil {
		log.Printf("PICK-PICKER: Template error: %v", err)
		http.Error(w, "Template error", http.StatusInternalServerError)
		return
	}
	
	log.Printf("PICK-PICKER: Template rendered successfully")
}

// SubmitPicks handles pick submissions via HTMX form
func (h *GameHandler) SubmitPicks(w http.ResponseWriter, r *http.Request) {
	log.Printf("HTTP: %s %s from %s", r.Method, r.URL.Path, r.RemoteAddr)
	
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	
	// Get user from context
	user := middleware.GetUserFromContext(r)
	if user == nil {
		http.Error(w, "Authentication required", http.StatusUnauthorized)
		return
	}
	
	// Parse form data
	err := r.ParseForm()
	if err != nil {
		log.Printf("Error parsing form: %v", err)
		http.Error(w, "Invalid form data", http.StatusBadRequest)
		return
	}
	
	// Get week and season from form
	week, _ := strconv.Atoi(r.FormValue("week"))
	season, _ := strconv.Atoi(r.FormValue("season"))
	
	if week < 1 || week > 18 || season < 2020 || season > 2030 {
		http.Error(w, "Invalid week or season", http.StatusBadRequest)
		return
	}
	
	// Get games data to resolve team names
	var games []models.Game
	if gameServiceWithSeason, ok := h.gameService.(interface{ GetGamesBySeason(int) ([]models.Game, error) }); ok {
		games, err = gameServiceWithSeason.GetGamesBySeason(season)
	} else {
		games, err = h.gameService.GetGames()
	}
	
	if err != nil {
		log.Printf("Error fetching games for pick submission: %v", err)
		http.Error(w, "Failed to fetch games data", http.StatusInternalServerError)
		return
	}
	
	// Create game lookup map
	gameMap := make(map[int]models.Game)
	for _, game := range games {
		gameMap[game.ID] = game
	}

	// Parse picks from form data  
	// Checkbox form fields: "pick-{gameID}-{away/home/98/99}" = "1"
	var picks []*models.Pick
	
	for fieldName, values := range r.Form {
		if len(values) == 0 || values[0] != "1" {
			continue // Skip unchecked checkboxes
		}
		
		if strings.HasPrefix(fieldName, "pick-") {
			// Parse pick-{gameID}-{team} from the field name
			parts := strings.Split(fieldName, "-")
			if len(parts) == 3 {
				gameID, err1 := strconv.Atoi(parts[1])
				if err1 == nil {
					game, gameExists := gameMap[gameID]
					if !gameExists {
						log.Printf("Game %d not found for pick submission", gameID)
						continue
					}
					
					var teamName string
					var teamID int
					pickType := models.PickTypeSpread // Default to spread pick
					
					switch parts[2] {
					case "away":
						teamName = game.Away
						teamID = h.getESPNTeamID(game.Away) // Use actual ESPN team ID
					case "home":
						teamName = game.Home  
						teamID = h.getESPNTeamID(game.Home) // Use actual ESPN team ID
					case "98":
						teamName = "Under"
						teamID = 98
						pickType = models.PickTypeOverUnder
					case "99":
						teamName = "Over"
						teamID = 99
						pickType = models.PickTypeOverUnder
					default:
						continue // Skip unknown team types
					}
					
					// Create pick using model helper
					pick := models.CreatePickFromLegacyData(user.ID, gameID, teamID, season, week)
					pick.PickType = pickType
					pick.TeamName = teamName // Set the team name directly
					
					// Set pick description for template display
					if pickType == models.PickTypeOverUnder {
						pick.PickDescription = fmt.Sprintf("%s @ %s - %s", game.Away, game.Home, teamName)
					} else {
						// For spread picks, include spread info if available
						if game.HasOdds() {
							var spreadDesc string
							if teamName == game.Home {
								spreadDesc = game.FormatHomeSpread()
							} else {
								spreadDesc = game.FormatAwaySpread()
							}
							pick.PickDescription = fmt.Sprintf("%s @ %s - %s %s (spread)", game.Away, game.Home, teamName, spreadDesc)
						} else {
							pick.PickDescription = fmt.Sprintf("%s @ %s - %s (spread)", game.Away, game.Home, teamName)
						}
					}
					
					picks = append(picks, pick)
				}
			}
		}
	}
	
	log.Printf("User %d submitting %d picks for week %d, season %d", user.ID, len(picks), week, season)
	
	// Replace picks via pick service (clear existing and create new ones atomically)
	if h.pickService != nil {
		err := h.pickService.ReplaceUserPicksForWeek(context.Background(), user.ID, season, week, picks)
		if err != nil {
			log.Printf("Error replacing user picks: %v", err)
			http.Error(w, "Failed to submit picks", http.StatusInternalServerError)
			return
		}
		
		// Trigger single SSE update after all database operations complete
		h.broadcastPickUpdate(user.ID, season, week)
	}
	
	// Return success response for HTMX
	w.Header().Set("HX-Trigger", "picks-updated")
	w.WriteHeader(http.StatusOK)
	fmt.Fprint(w, "Picks submitted successfully!")
}

// broadcastPickUpdate sends personalized HTML updates to all connected SSE clients
func (h *GameHandler) broadcastPickUpdate(userID, season, week int) {
	// Get fresh data for the updated week
	var games []models.Game
	var err error
	
	// Use GameService interface method that supports seasons
	if gameServiceWithSeason, ok := h.gameService.(interface{ GetGamesBySeason(int) ([]models.Game, error) }); ok {
		games, err = gameServiceWithSeason.GetGamesBySeason(season)
	} else {
		games, err = h.gameService.GetGames()
	}
	
	if err != nil {
		log.Printf("SSE: Error fetching games for pick update broadcast: %v", err)
		return
	}
	
	// Filter games by week
	filteredGames := make([]models.Game, 0)
	for _, game := range games {
		if game.Week == week {
			filteredGames = append(filteredGames, game)
		}
	}
	games = filteredGames
	
	// Get picks for ONLY the user who updated their picks
	var updatedUserPicks *models.UserPicks
	if h.pickService != nil {
		userPicks, pickErr := h.pickService.GetUserPicksForWeek(context.Background(), userID, season, week)
		if pickErr != nil {
			log.Printf("SSE: Warning - failed to load picks for user %d, week %d, season %d: %v", userID, week, season, pickErr)
			return
		}
		updatedUserPicks = userPicks
	}
	
	// Generate weeks list
	weeks := make([]int, 18)
	for i := range weeks {
		weeks[i] = i + 1
	}
	
	// Get user info for the updated user
	users := []*models.User{
		{ID: 0, Name: "ANDREW", Email: "ackilpatrick@gmail.com"},
		{ID: 1, Name: "BARDIA", Email: "bbakhtari@gmail.com"},
		{ID: 2, Name: "COOPER", Email: "cooper.kocsis@mattel.com"},
		{ID: 3, Name: "MICAH", Email: "micahgoldman@gmail.com"},
		{ID: 4, Name: "RYAN", Email: "ryan.pielow@gmail.com"},
		{ID: 5, Name: "TJ", Email: "tyerke@yahoo.com"},
		{ID: 6, Name: "BRAD", Email: "bradvassar@gmail.com"},
	}
	
	var updatedUser *models.User
	for _, user := range users {
		if user.ID == userID {
			updatedUser = user
			break
		}
	}
	
	if updatedUser == nil {
		log.Printf("SSE: Could not find user info for updated user %d", userID)
		return
	}
	
	// Send targeted update to all connected clients
	for client := range h.sseClients {
		// Find viewing user info for this client
		var viewingUser *models.User
		for _, user := range users {
			if user.ID == client.UserID {
				viewingUser = user
				break
			}
		}
		
		if viewingUser == nil {
			log.Printf("SSE: Could not find user info for client UserID %d", client.UserID)
			continue
		}
		
		// Debug logging
		log.Printf("SSE: Broadcasting user %s picks update to client %d", updatedUser.Name, client.UserID)
		if updatedUserPicks != nil {
			log.Printf("SSE: Updated user %s has %d picks", updatedUserPicks.UserName, len(updatedUserPicks.Picks))
		}
		
		// Get all user picks for this week to render complete picks section
		allUserPicks, err := h.pickService.GetAllUserPicksForWeek(context.Background(), season, week)
		if err != nil {
			log.Printf("SSE: Error fetching all user picks for complete section render: %v", err)
			continue
		}
		
		// Render the complete picks section to maintain proper structure
		var htmlContent strings.Builder
		
		templateData := struct {
			Games     []models.Game
			UserPicks []*models.UserPicks
			User      *models.User
		}{
			Games:     games,
			UserPicks: allUserPicks,
			User:      viewingUser,
		}
		
		if err := h.templates.ExecuteTemplate(&htmlContent, "picks-section", templateData); err != nil {
			log.Printf("SSE: Template error for picks section: %v", err)
			continue
		}
		
		// Get the rendered content without OOB wrapper - let client-side handle the swap
		renderedContent := strings.TrimSpace(htmlContent.String())
		// Send plain content and let HTMX SSE listener handle the target
		finalContent := renderedContent
		
		// Debug the rendered content
		log.Printf("SSE: Rendered picks section content length: %d characters", len(finalContent))
		log.Printf("SSE: Content preview: %s", finalContent[:min(200, len(finalContent))])
		
		// Send user-picks-updated event
		select {
		case client.Channel <- fmt.Sprintf("user-picks-updated:%s", finalContent):
		default:
			// Client channel is full, skip
		}
	}
	
	log.Printf("SSE: Sent personalized pick updates to %d connected clients, triggered by user %d", len(h.sseClients), userID)
}

// personalizeUserPicksForViewer filters and modifies user picks based on what the viewing user should see
func (h *GameHandler) personalizeUserPicksForViewer(allUserPicks []*models.UserPicks, viewingUser *models.User, games []models.Game) []*models.UserPicks {
	// For now, return all picks (no filtering)
	// TODO: Add logic to hide picks for games that haven't started based on viewing user permissions
	return allUserPicks
}

// getESPNTeamID maps team abbreviations to ESPN team IDs
func (h *GameHandler) getESPNTeamID(teamAbbr string) int {
	// ESPN team ID mapping (matching the one used in pick service)
	teamIDMap := map[string]int{
		"ATL": 1, "BUF": 2, "CHI": 3, "CIN": 4, "CLE": 5, "DAL": 6, "DEN": 7, "DET": 8,
		"GB": 9, "TEN": 10, "IND": 11, "KC": 12, "LV": 13, "LAR": 14, "MIA": 15, "MIN": 16,
		"NE": 17, "NO": 18, "NYG": 19, "NYJ": 20, "PHI": 21, "ARI": 22, "PIT": 23, "LAC": 24,
		"SF": 25, "SEA": 26, "TB": 27, "WSH": 28, "CAR": 29, "JAX": 30, "BAL": 33, "HOU": 34,
	}
	
	if id, exists := teamIDMap[teamAbbr]; exists {
		return id
	}
	
	// Fallback: return 0 if team not found
	log.Printf("Warning: Unknown team abbreviation '%s', using teamID 0", teamAbbr)
	return 0
}



