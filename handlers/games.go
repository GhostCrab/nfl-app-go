package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"html/template"
	"log"
	"math/rand"
	"net/http"
	"nfl-app-go/middleware"
	"nfl-app-go/models"
	"nfl-app-go/services"
	"sort"
	"strconv"
	"strings"
	"time"
)

// sortGamesByKickoffTime sorts games chronologically by kickoff time
// Secondary sort: alphabetically by home team name for games at same time
func sortGamesByKickoffTime(games []models.Game) {
	sort.Slice(games, func(i, j int) bool {
		// Primary sort: by game date/time
		if games[i].Date.Unix() != games[j].Date.Unix() {
			return games[i].Date.Before(games[j].Date)
		}
		// Secondary sort: alphabetically by home team name for same kickoff time
		return games[i].Home < games[j].Home
	})
}

// GameHandler handles game-related HTTP requests
// SSEClient represents a connected SSE client with user context
type SSEClient struct {
	Channel chan string
	UserID  int
}

type GameHandler struct {
	templates         *template.Template
	gameService       services.GameService
	pickService       *services.PickService
	sseClients        map[*SSEClient]bool
	authService       *services.AuthService
	dataLoader        *services.DataLoader
	visibilityService *services.PickVisibilityService
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

// SetVisibilityService sets the pick visibility service
func (h *GameHandler) SetVisibilityService(visibilityService *services.PickVisibilityService) {
	h.visibilityService = visibilityService
}

// GetGames handles GET / and /games - displays dashboard
func (h *GameHandler) GetGames(w http.ResponseWriter, r *http.Request) {
	log.Printf("HTTP: %s %s from %s", r.Method, r.URL.Path, r.RemoteAddr)

	// Handle debug datetime parameter for testing visibility
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

				// Check if we should enable demo games for games within 60 minutes
				h.applyDebugGameStates(debugTime)
			} else {
				log.Printf("DEBUG: Invalid datetime format: %s (use YYYY-MM-DDTHH:MM)", debugDateTimeStr)
			}
		}
	} else if debugDateTimeStr == "" && h.visibilityService != nil {
		// Clear debug time if no parameter provided
		h.visibilityService.ClearDebugDateTime()
	}

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
	if gameServiceWithSeason, ok := h.gameService.(interface {
		GetGamesBySeason(int) ([]models.Game, error)
	}); ok {
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

	// Sort games chronologically by kickoff time, then by home team name
	sortGamesByKickoffTime(games)

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

		// Apply pick visibility filtering for security
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

		// Apply demo effects to picks for Week 1 games (but avoid "pending" string)
		// userPicks = h.applyDemoEffectsToPicksForWeek1(userPicks) // DISABLED for analytics

		// Ensure all users have a pick entry, even if empty
		userPicksMap := make(map[int]*models.UserPicks)
		for _, up := range userPicks {
			userPicksMap[up.UserID] = up
		}

		// Add empty pick entries for users who don't have picks this week
		for _, user := range users {
			if _, exists := userPicksMap[user.ID]; !exists {
				userPicks = append(userPicks, &models.UserPicks{
					UserID:             user.ID,
					UserName:           user.Name,
					Picks:              []models.Pick{},
					BonusThursdayPicks: []models.Pick{},
					BonusFridayPicks:   []models.Pick{},
					Record:             models.UserRecord{},
				})
			}
		}
	} else {
		log.Printf("GameHandler: WARNING - No pick service available")
	}

	// Populate DailyPickGroups for modern seasons (2025+) before rendering template
	for _, userPicks := range userPicks {
		userPicks.PopulateDailyPickGroups(games, season)
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

// RefreshGames handles game refresh requests from SSE events
func (h *GameHandler) RefreshGames(w http.ResponseWriter, r *http.Request) {
	log.Printf("HTTP: SSE Game refresh request from %s", r.RemoteAddr)

	// Extract season and week from query parameters
	seasonStr := r.URL.Query().Get("season")
	weekStr := r.URL.Query().Get("week")

	season := 2025 // Default season
	week := 1      // Default week

	if seasonStr != "" {
		if parsedSeason, err := strconv.Atoi(seasonStr); err == nil {
			season = parsedSeason
		}
	}

	if weekStr != "" {
		if parsedWeek, err := strconv.Atoi(weekStr); err == nil {
			week = parsedWeek
		}
	}

	// Fetch games for the specific season/week
	var games []models.Game
	var err error

	if gameServiceWithSeason, ok := h.gameService.(interface {
		GetGamesBySeason(int) ([]models.Game, error)
	}); ok {
		allGames, err := gameServiceWithSeason.GetGamesBySeason(season)
		if err != nil {
			log.Printf("RefreshGames: Error fetching games: %v", err)
			http.Error(w, "Error fetching games", http.StatusInternalServerError)
			return
		}

		// Filter to specific week
		for _, game := range allGames {
			if game.Week == week {
				games = append(games, game)
			}
		}
	} else {
		games, err = h.gameService.GetGames()
		if err != nil {
			log.Printf("RefreshGames: Error fetching games: %v", err)
			http.Error(w, "Error fetching games", http.StatusInternalServerError)
			return
		}
	}

	// Create template data
	data := struct {
		Games []models.Game
	}{
		Games: games,
	}

	// Return only the game-grid template (games container content)
	err = h.templates.ExecuteTemplate(w, "game-grid", data)
	if err != nil {
		log.Printf("RefreshGames: Template error: %v", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	log.Printf("HTTP: Successfully refreshed games for season %d, week %d (%d games)", season, week, len(games))
}

// TestGameUpdate simulates a game update for testing SSE functionality
func (h *GameHandler) TestGameUpdate(w http.ResponseWriter, r *http.Request) {
	log.Printf("HTTP: Test game update request from %s", r.RemoteAddr)

	// Get test parameters
	gameIDStr := r.URL.Query().Get("gameId")
	testType := r.URL.Query().Get("type") // "score", "state", "live"

	if gameIDStr == "" {
		http.Error(w, "gameId parameter required", http.StatusBadRequest)
		return
	}

	gameID, err := strconv.Atoi(gameIDStr)
	if err != nil {
		http.Error(w, "Invalid gameId", http.StatusBadRequest)
		return
	}

	// Default test type
	if testType == "" {
		testType = "score"
	}

	// Simulate different types of game updates
	switch testType {
	case "score":
		// Simulate score update
		log.Printf("TestGameUpdate: Simulating score update for game %d", gameID)
		h.simulateScoreUpdate(gameID)
	case "state":
		// Simulate state change (scheduled -> in_play -> completed)
		log.Printf("TestGameUpdate: Simulating state change for game %d", gameID)
		h.simulateStateChange(gameID)
	case "live":
		// Simulate live game updates
		log.Printf("TestGameUpdate: Simulating live updates for game %d", gameID)
		h.simulateLiveUpdate(gameID)
	default:
		http.Error(w, "Invalid test type. Use: score, state, or live", http.StatusBadRequest)
		return
	}

	// Trigger SSE update
	h.BroadcastStructuredUpdate("gameUpdate", fmt.Sprintf(`{"type":"testUpdate","gameId":%d,"testType":"%s","timestamp":%d}`, gameID, testType, time.Now().UnixMilli()))

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(fmt.Sprintf(`{"success":true,"message":"Test %s update triggered for game %d","gameId":%d,"type":"%s"}`, testType, gameID, gameID, testType)))
}

// Helper methods for test game updates
func (h *GameHandler) simulateScoreUpdate(gameID int) {
	// This would update the game in database with new scores
	// For now, just log the simulation
	log.Printf("SIMULATION: Game %d - Updated scores (Home: %d, Away: %d)", gameID, 14+rand.Intn(21), 10+rand.Intn(21))
}

func (h *GameHandler) simulateStateChange(gameID int) {
	// This would change game state in database
	log.Printf("SIMULATION: Game %d - State changed to in_play", gameID)
}

func (h *GameHandler) simulateLiveUpdate(gameID int) {
	// This would update live game data (clock, possession, etc.)
	quarter := rand.Intn(4) + 1
	clockMinutes := rand.Intn(15)
	clockSeconds := rand.Intn(60)
	log.Printf("SIMULATION: Game %d - Live update (Q%d %d:%02d)", gameID, quarter, clockMinutes, clockSeconds)
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
	// Skip pick changes - these are now handled directly by SubmitPicks handler
	if event.Collection == "picks" {
		return
	}

	// For game collection changes, send targeted HTML updates
	if event.Collection == "games" && event.GameID != "" {
		h.broadcastGameUpdate(event.GameID, event.Season, event.Week)
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
	h.BroadcastStructuredUpdate("gameUpdate", htmlContent)
}

// BroadcastStructuredUpdate sends a structured update to all SSE clients
func (h *GameHandler) BroadcastStructuredUpdate(eventType, data string) {
	for client := range h.sseClients {
		select {
		case client.Channel <- fmt.Sprintf("%s:%s", eventType, data):
		default:
			// Client channel is full, skip
		}
	}

	// log.Printf("SSE: Broadcasted %s update to %d clients", eventType, len(h.sseClients))
}

// getCurrentWeek determines the current NFL week based on game dates and season state
func (h *GameHandler) getCurrentWeek(games []models.Game) int {
	// Use Pacific time for current time since NFL games are scheduled in Pacific time
	// and users expect week transitions to happen based on Pacific time
	pacificLoc, err := time.LoadLocation("America/Los_Angeles")
	if err != nil {
		// Fallback to UTC-8 offset if timezone loading fails
		pacificLoc = time.FixedZone("PST", -8*3600)
	}
	now := time.Now().In(pacificLoc)

	log.Printf("getCurrentWeek DEBUG: Current Pacific time: %v", now.Format("Mon Jan 2, 2006 15:04:05 MST"))

	if len(games) == 0 {
		return 1
	}

	// Find earliest and latest game dates across all weeks (using Pacific time)
	var earliestGame, latestGame time.Time
	weekGames := make(map[int][]models.Game)

	for _, game := range games {
		// Use Pacific time for game date comparisons
		gamePacificTime := game.PacificTime()
		if earliestGame.IsZero() || gamePacificTime.Before(earliestGame) {
			earliestGame = gamePacificTime
		}
		if latestGame.IsZero() || gamePacificTime.After(latestGame) {
			latestGame = gamePacificTime
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

		// Find earliest game in this week (using Pacific time)
		var earliestInWeek time.Time
		for _, game := range weekGamesList {
			gamePacificTime := game.PacificTime()
			if earliestInWeek.IsZero() || gamePacificTime.Before(earliestInWeek) {
				earliestInWeek = gamePacificTime
			}
		}

		// Check if we're within the relevant time window for this week:
		// - 3 days before the first game: show this week for preparation
		// - During the week (until 2 days after the last game): show this week
		// - After that: move to next week
		threeDaysBefore := earliestInWeek.Add(-3 * 24 * time.Hour)

		// Find the latest game in this week to determine when the week "ends"
		var latestInWeek time.Time
		for _, game := range weekGamesList {
			gamePacificTime := game.PacificTime()
			if latestInWeek.IsZero() || gamePacificTime.After(latestInWeek) {
				latestInWeek = gamePacificTime
			}
		}

		// Week window: 3 days before first game until 2 days after last game
		weekEndTime := latestInWeek.Add(2 * 24 * time.Hour)

		log.Printf("getCurrentWeek DEBUG: Week %d - earliest: %v, latest: %v, current: %v",
			week, earliestInWeek.Format("Mon Jan 2 15:04 MST"), latestInWeek.Format("Mon Jan 2 15:04 MST"), now.Format("Mon Jan 2 15:04 MST"))
		log.Printf("getCurrentWeek DEBUG: Week %d - 3daysBefore: %v, weekEnd: %v",
			week, threeDaysBefore.Format("Mon Jan 2 15:04 MST"), weekEndTime.Format("Mon Jan 2 15:04 MST"))
		log.Printf("getCurrentWeek DEBUG: Week %d - inWindow: %v",
			week, now.After(threeDaysBefore) && now.Before(weekEndTime))

		if now.After(threeDaysBefore) && now.Before(weekEndTime) {
			log.Printf("getCurrentWeek: Current time within Week %d window (games: %v to %v), showing Week %d",
				week, earliestInWeek.Format("Jan 2 15:04"), latestInWeek.Format("Jan 2 15:04"), week)
			return week
		}
	}

	// Fallback: find the week with games closest to current time
	currentWeek := 1
	minTimeDiff := time.Duration(999999999999999) // Very large duration

	for week, weekGamesList := range weekGames {
		for _, game := range weekGamesList {
			// Use Pacific time for game date comparison
			gamePacificTime := game.PacificTime()
			timeDiff := gamePacificTime.Sub(now)
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
	if gameServiceWithSeason, ok := h.gameService.(interface {
		GetGamesBySeason(int) ([]models.Game, error)
	}); ok {
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
	if gameServiceWithSeason, ok := h.gameService.(interface {
		GetGamesBySeason(int) ([]models.Game, error)
	}); ok {
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

	// Sort games chronologically by kickoff time, then by home team name
	sortGamesByKickoffTime(games)

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

		// Apply pick visibility filtering for security
		if h.visibilityService != nil {
			viewingUserID := -1 // Default for anonymous users
			if user != nil {
				viewingUserID = user.ID
			}

			filteredUserPicks, err := h.visibilityService.FilterVisibleUserPicks(r.Context(), userPicks, season, currentWeek, viewingUserID)
			if err != nil {
				log.Printf("API: Warning - failed to filter pick visibility: %v", err)
			} else {
				userPicks = filteredUserPicks
				log.Printf("API: Applied pick visibility filtering for user ID %d", viewingUserID)
			}
		}

		// Apply demo effects to picks for Week 1 games (but avoid "pending" string)
		// userPicks = h.applyDemoEffectsToPicksForWeek1(userPicks) // DISABLED for analytics
	} else {
		log.Printf("API: WARNING - No pick service available")
	}

	// Create response data structure
	data := struct {
		Games       []models.Game       `json:"games"`
		UserPicks   []*models.UserPicks `json:"userPicks"`
		Users       []*models.User      `json:"users"`
		User        *models.User        `json:"user"`
		CurrentWeek int                 `json:"currentWeek"`
		Season      int                 `json:"season"`
		Weeks       []int               `json:"weeks"`
		Title       string              `json:"title"`
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
	if gameServiceWithSeason, ok := h.gameService.(interface {
		GetGamesBySeason(int) ([]models.Game, error)
	}); ok {
		allGames, err = gameServiceWithSeason.GetGamesBySeason(season)
	} else {
		allGames, err = h.gameService.GetGames()
	}

	if err != nil {
		log.Printf("Error getting games for week %d, season %d: %v", week, season, err)
		http.Error(w, "Failed to load games", http.StatusInternalServerError)
		return
	}

	// Filter games by week and season - show all games but will disable non-scheduled ones
	var availableGames []models.Game
	for _, game := range allGames {
		if game.Week == week && game.Season == season {
			availableGames = append(availableGames, game)
		}
	}

	// Sort games chronologically by kickoff time, then by home team name
	sortGamesByKickoffTime(availableGames)

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
	pickState := make(map[int]map[int]bool) // pickState[gameID][templateKey] = selected
	if userPicks != nil {
		// Combine all pick types
		allPicks := append(userPicks.Picks, userPicks.SpreadPicks...)
		allPicks = append(allPicks, userPicks.OverUnderPicks...)
		allPicks = append(allPicks, userPicks.BonusThursdayPicks...)
		allPicks = append(allPicks, userPicks.BonusFridayPicks...)

		// Create game lookup for team mapping
		gameMap := make(map[int]models.Game)
		for _, game := range availableGames {
			gameMap[game.ID] = game
		}

		for _, pick := range allPicks {
			if pickState[pick.GameID] == nil {
				pickState[pick.GameID] = make(map[int]bool)
			}

			// Map database TeamID to template key
			var templateKey int
			if pick.TeamID == 98 {
				templateKey = 98 // Under
			} else if pick.TeamID == 99 {
				templateKey = 99 // Over
			} else if game, exists := gameMap[pick.GameID]; exists {
				// For spread picks, determine if it's away (1) or home (2)
				awayTeamID := h.getESPNTeamID(game.Away)
				homeTeamID := h.getESPNTeamID(game.Home)

				if pick.TeamID == awayTeamID {
					templateKey = 1 // Away
				} else if pick.TeamID == homeTeamID {
					templateKey = 2 // Home
				} else {
					// Fallback: try to match by team name
					if strings.Contains(strings.ToLower(pick.TeamName), strings.ToLower(game.Away)) {
						templateKey = 1 // Away
					} else if strings.Contains(strings.ToLower(pick.TeamName), strings.ToLower(game.Home)) {
						templateKey = 2 // Home
					} else {
						log.Printf("PICK-PICKER: Warning - Could not map pick TeamID %d to template key for game %d", pick.TeamID, pick.GameID)
						continue
					}
				}
			} else {
				log.Printf("PICK-PICKER: Warning - Game %d not found for pick TeamID %d", pick.GameID, pick.TeamID)
				continue
			}

			pickState[pick.GameID][templateKey] = true
			log.Printf("PICK-PICKER: Mapped pick GameID=%d, TeamID=%d, TeamName=%s -> templateKey=%d", pick.GameID, pick.TeamID, pick.TeamName, templateKey)
		}
	}

	data := struct {
		Games         []models.Game
		Week          int
		CurrentWeek   int
		Season        int
		CurrentSeason int
		User          *models.User
		PickState     map[int]map[int]bool
	}{
		Games:         availableGames,
		Week:          week,
		CurrentWeek:   week,
		Season:        season,
		CurrentSeason: season,
		User:          user,
		PickState:     pickState,
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
	if gameServiceWithSeason, ok := h.gameService.(interface {
		GetGamesBySeason(int) ([]models.Game, error)
	}); ok {
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

					// Skip picks for games that have already started or completed
					// (preserve existing picks by not including them in the replacement)
					if game.State != models.GameStateScheduled {
						log.Printf("Skipping pick for Game %d - game state is %s (not scheduled), will preserve existing picks", gameID, game.State)
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

					// DEBUG: Log pick creation details
					log.Printf("SUBMIT_PICKS DEBUG: Created pick - GameID=%d, TeamID=%d, TeamName='%s', PickType='%s', PickDescription='%s'", 
						pick.GameID, pick.TeamID, pick.TeamName, pick.PickType, pick.PickDescription)

					picks = append(picks, pick)
				}
			}
		}
	}

	log.Printf("User %d submitting %d picks for week %d, season %d", user.ID, len(picks), week, season)

	// Update picks via pick service (preserve existing picks for completed games)
	if h.pickService != nil {
		err := h.pickService.UpdateUserPicksForScheduledGames(context.Background(), user.ID, season, week, picks, gameMap)
		if err != nil {
			log.Printf("Error updating user picks: %v", err)
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

// broadcastPickUpdate sends targeted OOB updates for the specific user who updated picks
func (h *GameHandler) broadcastPickUpdate(userID, season, week int) {
	// Get fresh data for the updated week
	var games []models.Game
	var err error

	// Use GameService interface method that supports seasons
	if gameServiceWithSeason, ok := h.gameService.(interface {
		GetGamesBySeason(int) ([]models.Game, error)
	}); ok {
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

		// Apply pick visibility filtering for security
		if h.visibilityService != nil {
			filteredUserPicks, err := h.visibilityService.FilterVisibleUserPicks(context.Background(), allUserPicks, season, week, client.UserID)
			if err != nil {
				log.Printf("SSE: Warning - failed to filter pick visibility for user %d: %v", client.UserID, err)
			} else {
				allUserPicks = filteredUserPicks
				log.Printf("SSE: Applied pick visibility filtering for user ID %d", client.UserID)
			}
		}

		// Render targeted OOB update for just the updated user's picks
		var htmlContent strings.Builder
		isCurrentUser := (client.UserID == userID)

		// Enrich picks with display fields before template rendering (CRITICAL for SSE updates)
		log.Printf("SSE: Enriching %d picks for user %s", len(updatedUserPicks.Picks), updatedUser.Name)
		for i := range updatedUserPicks.Picks {
			pick := &updatedUserPicks.Picks[i]
			log.Printf("SSE: BEFORE enrichment - Pick GameID=%d, TeamName='%s', PickType='%s'", pick.GameID, pick.TeamName, pick.PickType)

			if err := h.pickService.EnrichPickWithGameData(pick); err != nil {
				log.Printf("SSE: Failed to enrich pick for Game %d, User %d: %v", pick.GameID, pick.UserID, err)
				continue
			}

			log.Printf("SSE: AFTER enrichment - Pick GameID=%d, TeamName='%s', PickType='%s'", pick.GameID, pick.TeamName, pick.PickType)
		}

		// Populate DailyPickGroups for modern seasons (2025+)
		updatedUserPicks.PopulateDailyPickGroups(games, season)

		templateData := map[string]interface{}{
			"UserPicks":     updatedUserPicks,
			"Games":         games,
			"IsCurrentUser": isCurrentUser,
			"IsFirst":       false,  // Not needed for OOB updates
			"Season":        season, // Add season for template logic
		}

		if err := h.templates.ExecuteTemplate(&htmlContent, "sse-user-picks-block", templateData); err != nil {
			log.Printf("SSE: Template error for user picks block: %v", err)
			continue
		}

		// Get the rendered OOB content with hx-swap-oob="true" already in template
		finalContent := strings.TrimSpace(htmlContent.String())

		// Debug the rendered content (much smaller now!)
		log.Printf("SSE: Rendered user %d OOB picks content length: %d characters", userID, len(finalContent))
		if len(finalContent) > 0 && len(finalContent) <= 200 {
			log.Printf("SSE: OOB Content preview: %s", finalContent)
		} else if len(finalContent) > 200 {
			log.Printf("SSE: OOB Content preview: %s...", finalContent[:200])
		}

		// Send user-picks-updated event with OOB content
		select {
		case client.Channel <- fmt.Sprintf("user-picks-updated:%s", finalContent):
		default:
			// Client channel is full, skip
		}
	}

	log.Printf("SSE: Sent targeted OOB pick updates for user %d to %d connected clients", userID, len(h.sseClients))
}

// broadcastGameUpdate sends targeted game HTML updates to all connected SSE clients
func (h *GameHandler) broadcastGameUpdate(gameID string, season, week int) {
	log.Printf("SSE: Broadcasting game update for gameID %s", gameID)

	// Get the updated game from database
	if h.gameService == nil {
		log.Printf("SSE: GameService not available")
		return
	}

	// Convert gameID string to int and fetch only the specific game
	gameIDInt, err := strconv.Atoi(gameID)
	if err != nil {
		log.Printf("SSE: Invalid game ID format %s: %v", gameID, err)
		return
	}

	updatedGame, err := h.gameService.GetGameByID(gameIDInt)
	if err != nil {
		log.Printf("SSE: Error fetching game %d: %v", gameIDInt, err)
		return
	}

	// Send targeted updates for live game elements
	if updatedGame.State == models.GameStateInPlay {
		h.broadcastGameStatusHTML(updatedGame)
		h.broadcastGameScoresHTML(updatedGame)
		// Also update pick items to reflect current scores and spread results
		h.broadcastPickUpdatesHTML(updatedGame)
	}

	// Also trigger pick updates when games complete (final results)
	if updatedGame.State == models.GameStateCompleted {
		h.broadcastGameStatusHTML(updatedGame) // Update game status to remove live expansion
		h.broadcastPickUpdatesHTML(updatedGame)
	}
}

// broadcastGameStatusHTML sends updated game status HTML via SSE using templates
func (h *GameHandler) broadcastGameStatusHTML(game *models.Game) {
	var statusHTML strings.Builder

	// Render game clock using template
	if err := h.templates.ExecuteTemplate(&statusHTML, "sse-game-clock", game); err != nil {
		log.Printf("SSE: Template error for game clock: %v", err)
		return
	}

	// Render possession info using template
	var possessionHTML strings.Builder
	if err := h.templates.ExecuteTemplate(&possessionHTML, "sse-possession-info", game); err != nil {
		log.Printf("SSE: Template error for possession info: %v", err)
	} else {
		statusHTML.WriteString(possessionHTML.String())
	}

	// Broadcast to all connected clients
	for client := range h.sseClients {
		select {
		case client.Channel <- fmt.Sprintf("gameStatus:%s", statusHTML.String()):
		default:
			// Client channel full, skip
		}
	}

}

// broadcastGameScoresHTML sends updated game scores HTML via SSE
func (h *GameHandler) broadcastGameScoresHTML(game *models.Game) {
	// Create HTML for score updates with hx-swap-oob targeting
	awayScoreHTML := fmt.Sprintf(`<span class="team-score" id="away-score-%d" hx-swap-oob="true">%d</span>`,
		game.ID, game.AwayScore)
	homeScoreHTML := fmt.Sprintf(`<span class="team-score" id="home-score-%d" hx-swap-oob="true">%d</span>`,
		game.ID, game.HomeScore)

	scoresHTML := awayScoreHTML + homeScoreHTML

	// Broadcast to all connected clients
	for client := range h.sseClients {
		select {
		case client.Channel <- fmt.Sprintf("gameScores:%s", scoresHTML):
		default:
			// Client channel full, skip
		}
	}

}

// broadcastPickUpdatesHTML sends updated pick item HTML via SSE for a specific game
func (h *GameHandler) broadcastPickUpdatesHTML(game *models.Game) {
	// Get current season (2025 for now, but should be dynamic)
	season := 2025

	// Get all user picks for current week
	gameWeek := game.Week
	if gameWeek <= 0 {
		log.Printf("SSE: Game %d has invalid week %d, skipping pick updates", game.ID, gameWeek)
		return
	}

	allUserPicks, err := h.pickService.GetAllUserPicksForWeek(context.Background(), season, gameWeek)
	if err != nil {
		log.Printf("SSE: Error fetching user picks for week %d: %v", gameWeek, err)
		return
	}

	// Filter to only picks for this specific game
	games := []models.Game{*game}

	var pickUpdates []string

	// Generate updated pick item HTML for each user who has a pick for this game
	for _, userPicks := range allUserPicks {
		// Process only the main Picks slice - it contains all enriched picks
		// The categorized slices (SpreadPicks, OverUnderPicks, etc.) are copies that may not be enriched
		for _, pick := range userPicks.Picks {
			if pick.GameID == game.ID {
				// Log pick state before enrichment
				log.Printf("SSE DEBUG: BEFORE enrichment - Game %d, User %d, TeamName='%s', PickType='%s', TeamID=%d, GameDescription='%s'",
					pick.GameID, pick.UserID, pick.TeamName, pick.PickType, pick.TeamID, pick.GameDescription)

				// Ensure pick is enriched with display fields before rendering
				if err := h.pickService.EnrichPickWithGameData(&pick); err != nil {
					log.Printf("SSE: Failed to enrich pick for Game %d, User %d: %v", pick.GameID, pick.UserID, err)
					continue
				}

				// Log pick state after enrichment
				log.Printf("SSE DEBUG: AFTER enrichment - Game %d, User %d, TeamName='%s', PickType='%s', TeamID=%d, GameDescription='%s'",
					pick.GameID, pick.UserID, pick.TeamName, pick.PickType, pick.TeamID, pick.GameDescription)

				// Create template data for this specific pick
				templateData := struct {
					Pick          models.Pick
					Games         []models.Game
					IsCurrentUser bool
				}{
					Pick:          pick,
					Games:         games,
					IsCurrentUser: false, // SSE updates don't need current user context
				}

				// Log template data being passed
				log.Printf("SSE DEBUG: Template data - Pick.TeamName='%s', Pick.PickType='%s', Pick.TeamID=%d",
					templateData.Pick.TeamName, templateData.Pick.PickType, templateData.Pick.TeamID)

				// Render the pick item using the unified-pick-item template
				var pickHTML strings.Builder
				if err := h.templates.ExecuteTemplate(&pickHTML, "unified-pick-item", templateData); err != nil {
					log.Printf("SSE: Template error for pick item (Game %d, User %d): %v", pick.GameID, pick.UserID, err)
					continue
				}

				// Add hx-swap-oob attribute to the rendered HTML for HTMX out-of-band swapping
				pickHTMLWithOOB := strings.Replace(pickHTML.String(),
					fmt.Sprintf(`id="pick-item-%d-%d"`, pick.GameID, pick.UserID),
					fmt.Sprintf(`id="pick-item-%d-%d" hx-swap-oob="true"`, pick.GameID, pick.UserID), 1)

				pickUpdates = append(pickUpdates, pickHTMLWithOOB)
			}
		}
	}

	if len(pickUpdates) == 0 {
		log.Printf("SSE: No pick updates to send for game %d", game.ID)
		return
	}

	// Combine all pick updates into a single SSE event
	combinedHTML := strings.Join(pickUpdates, "")

	// Broadcast to all connected clients
	for client := range h.sseClients {
		select {
		case client.Channel <- fmt.Sprintf("pickUpdates:%s", combinedHTML):
		default:
			// Client channel full, skip
		}
	}

	log.Printf("SSE: Sent pick updates for game %d to %d clients (%d pick items updated)",
		game.ID, len(h.sseClients), len(pickUpdates))
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

// applyDebugGameStates applies demo game states for testing based on debug datetime
func (h *GameHandler) applyDebugGameStates(debugTime time.Time) {
	// Get games to check which ones should be modified
	var games []models.Game
	var err error

	if gameServiceWithSeason, ok := h.gameService.(interface {
		GetGamesBySeason(int) ([]models.Game, error)
	}); ok {
		games, err = gameServiceWithSeason.GetGamesBySeason(2025)
	} else {
		games, err = h.gameService.GetGames()
	}

	if err != nil {
		log.Printf("DEBUG: Error fetching games for demo state application: %v", err)
		return
	}

	demoGamesCount := 0
	completedGamesCount := 0

	for _, game := range games {
		gameTime := game.PacificTime()
		timeDiff := debugTime.Sub(gameTime)

		// Games within 60 minutes of debug time should be "in-progress"
		if timeDiff > -60*time.Minute && timeDiff < 60*time.Minute {
			demoGamesCount++
		}

		// Games before debug time should be "completed"
		if timeDiff > 60*time.Minute {
			completedGamesCount++
		}
	}

	log.Printf("DEBUG: Demo time analysis - %d games would be in-progress, %d would be completed (debug time: %s)",
		demoGamesCount, completedGamesCount, debugTime.Format("2006-01-02 15:04:05 MST"))

	// Note: Actual game state modification would require a different game service implementation
	// that can dynamically alter game states. For now, we just log what would happen.
}
