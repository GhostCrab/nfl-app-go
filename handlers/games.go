package handlers

import (
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
type GameHandler struct {
	templates   *template.Template
	gameService services.GameService
	pickService *services.PickService
	sseClients  map[chan string]bool
	authService *services.AuthService
	dataLoader  *services.DataLoader
}

// NewGameHandler creates a new game handler
func NewGameHandler(templates *template.Template, gameService services.GameService) *GameHandler {
	return &GameHandler{
		templates:   templates,
		gameService: gameService,
		sseClients:  make(map[chan string]bool),
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
}

// SSEHandler handles Server-Sent Events for real-time game updates
func (h *GameHandler) SSEHandler(w http.ResponseWriter, r *http.Request) {
	log.Printf("SSE: New client connected from %s", r.RemoteAddr)

	// Set SSE headers
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	// Create client channel
	clientChan := make(chan string, 10)
	h.sseClients[clientChan] = true

	// Send initial connection confirmation
	fmt.Fprintf(w, "event: connected\n")
	fmt.Fprintf(w, "data: SSE connection established\n\n")
	w.(http.Flusher).Flush()

	// Handle client disconnect
	defer func() {
		delete(h.sseClients, clientChan)
		close(clientChan)
		log.Printf("SSE: Client disconnected from %s", r.RemoteAddr)
	}()

	// Keep connection alive and send updates
	for {
		select {
		case message := <-clientChan:
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
	log.Printf("SSE: Processing change event: Collection=%s, Operation=%s, Season=%d, Week=%d", 
		event.Collection, event.Operation, event.Season, event.Week)

	// Create a structured update event
	updateEvent := struct {
		Type       string `json:"type"`
		Collection string `json:"collection"`
		Operation  string `json:"operation"`
		Season     int    `json:"season"`
		Week       int    `json:"week"`
		GameID     string `json:"gameId,omitempty"`
		UserID     int    `json:"userId,omitempty"`
		Timestamp  int64  `json:"timestamp"`
	}{
		Type:       "databaseChange",
		Collection: event.Collection,
		Operation:  event.Operation,
		Season:     event.Season,
		Week:       event.Week,
		GameID:     event.GameID,
		UserID:     event.UserID,
		Timestamp:  time.Now().UnixMilli(),
	}

	// Convert to JSON
	updateJSON, err := json.Marshal(updateEvent)
	if err != nil {
		log.Printf("SSE: Error marshaling update event: %v", err)
		return
	}

	// Send structured update to all connected clients
	h.broadcastStructuredUpdate("databaseChange", string(updateJSON))
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
	for clientChan := range h.sseClients {
		select {
		case clientChan <- fmt.Sprintf("%s:%s", eventType, data):
		default:
			// Client channel is full, skip
		}
	}
	
	log.Printf("SSE: Broadcasted %s update to %d clients", eventType, len(h.sseClients))
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



