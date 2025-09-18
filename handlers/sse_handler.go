package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"html/template"
	"net/http"
	"nfl-app-go/database"
	"nfl-app-go/logging"
	"nfl-app-go/middleware"
	"nfl-app-go/models"
	"nfl-app-go/services"
	"strconv"
	"strings"
	"sync/atomic"
	"time"
)

// Note: SSEClient is already defined in games.go, but we'll use the same structure

// SSEHandler handles all Server-Sent Events functionality
type SSEHandler struct {
	templates         *template.Template
	gameService       services.GameService
	pickService       *services.PickService
	authService       *services.AuthService
	visibilityService *services.PickVisibilityService
	parlayRepo        *database.MongoParlayRepository
	sseClients        map[*SSEClient]bool
	messageCounter    uint64       // Atomic counter for message sequencing
	heartbeatTicker   *time.Ticker // Heartbeat timer
	stopHeartbeat     chan bool    // Channel to stop heartbeat
}

// NewSSEHandler creates a new SSE handler
func NewSSEHandler(templates *template.Template, gameService services.GameService) *SSEHandler {
	handler := &SSEHandler{
		templates:     templates,
		gameService:   gameService,
		sseClients:    make(map[*SSEClient]bool),
		stopHeartbeat: make(chan bool),
	}

	// Start heartbeat goroutine
	handler.startHeartbeat()

	return handler
}

// SetServices sets the required services
func (h *SSEHandler) SetServices(pickService *services.PickService, authService *services.AuthService, visibilityService *services.PickVisibilityService, parlayRepo *database.MongoParlayRepository) {
	h.pickService = pickService
	h.authService = authService
	h.visibilityService = visibilityService
	h.parlayRepo = parlayRepo
}

// GetClients returns the SSE clients for broadcasting (for use by other handlers)
func (h *SSEHandler) GetClients() map[*SSEClient]bool {
	return h.sseClients
}

// SSEHandler handles Server-Sent Events for real-time game updates
func (h *SSEHandler) Handle(w http.ResponseWriter, r *http.Request) {
	// Get user from context (if authenticated)
	user := middleware.GetUserFromContext(r)
	userID := 0 // Default to user 0 if not authenticated
	if user != nil {
		userID = user.ID
	}

	logger := logging.WithPrefix("SSE")
	logger.Infof("New client connected from %s (UserID: %d)", r.RemoteAddr, userID)

	// Set SSE headers
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Headers", "Cache-Control")

	// Create a new client
	client := &SSEClient{
		Channel: make(chan string, 100), // Buffer for messages
		UserID:  userID,
	}

	// Add client to the map
	h.sseClients[client] = true
	defer func() {
		delete(h.sseClients, client)
		close(client.Channel)
		logger.Infof("Client disconnected (UserID: %d)", userID)
	}()

	// Flusher for real-time streaming
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "SSE not supported", http.StatusInternalServerError)
		return
	}

	// Send initial connection message
	fmt.Fprintf(w, "data: {\"type\": \"connected\", \"message\": \"SSE connection established\"}\n\n")
	flusher.Flush()

	// Listen for messages
	for {
		select {
		case message, ok := <-client.Channel:
			if !ok {
				return
			}
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
			flusher.Flush()

		case <-r.Context().Done():
			logger.Debugf("Client context cancelled (UserID: %d)", userID)
			return
		}
	}
}

// HandleDatabaseChange processes database change events for SSE broadcasting
func (h *SSEHandler) HandleDatabaseChange(event services.ChangeEvent) {
	logger := logging.WithPrefix("SSE:DBChange")
	logger.Debugf("%s collection, GameID: %s, Season: %d, Week: %d, Operation: %s",
		event.Collection, event.GameID, event.Season, event.Week, event.Operation)

	// Handle game collection changes
	if event.Collection == "games" && event.GameID != "" {
		h.BroadcastGameUpdate(event.GameID, event.Season, event.Week)
	}

	// Handle pick collection changes
	if event.Collection == "picks" && event.UserID > 0 {
		h.BroadcastPickUpdate(event.UserID, event.Season, event.Week)
	}

	// Handle parlay score collection changes
	if event.Collection == "parlay_scores" {
		h.BroadcastParlayScoreUpdate(event.Season, event.Week)
	}
}

// BroadcastUpdate sends game updates to all connected SSE clients (legacy method)
func (h *SSEHandler) BroadcastUpdate() {
	games, err := h.gameService.GetGames()
	if err != nil {
		logger := logging.WithPrefix("SSE")
		logger.Errorf("Error fetching games for broadcast: %v", err)
		return
	}

	// Render games HTML
	data := struct {
		Games []models.Game
	}{
		Games: games,
	}

	// Execute the template
	htmlBuffer := &strings.Builder{}
	if err := h.templates.ExecuteTemplate(htmlBuffer, "games-container", data); err != nil {
		logger := logging.WithPrefix("SSE")
		logger.Errorf("Error rendering games template: %v", err)
		return
	}

	// Create SSE message
	message := map[string]interface{}{
		"type":    "games-update",
		"html":    htmlBuffer.String(),
		"message": "Games updated",
	}

	messageData, err := json.Marshal(message)
	if err != nil {
		logger := logging.WithPrefix("SSE")
		logger.Errorf("Error marshaling message: %v", err)
		return
	}

	// Broadcast to all clients
	h.broadcastToAllClients(string(messageData))
}

// BroadcastStructuredUpdate sends structured updates to all SSE clients
func (h *SSEHandler) BroadcastStructuredUpdate(eventType, data string) {
	message := map[string]interface{}{
		"type": eventType,
		"data": data,
	}

	messageData, err := json.Marshal(message)
	if err != nil {
		logger := logging.WithPrefix("SSE")
		logger.Errorf("Error marshaling structured message: %v", err)
		return
	}

	h.broadcastToAllClients(string(messageData))
}

// broadcastToAllClients sends a message to all connected SSE clients
func (h *SSEHandler) broadcastToAllClients(messageData string) {
	h.broadcastToAllClientsWithID(messageData)
}

// broadcastToAllClientsWithID sends a message with sequence ID to all connected SSE clients
func (h *SSEHandler) broadcastToAllClientsWithID(messageData string) {
	clientCount := len(h.sseClients)
	if clientCount == 0 {
		return
	}

	// Generate next message ID
	msgID := atomic.AddUint64(&h.messageCounter, 1)

	// Add message ID to the SSE message
	messageWithID := fmt.Sprintf("id: %d\n%s", msgID, messageData)

	sentCount := 0
	for client := range h.sseClients {
		select {
		case client.Channel <- messageWithID:
			sentCount++
		default:
			logger := logging.WithPrefix("SSE")
			logger.Warnf("Client channel full, skipping message")
		}
	}

	// log.Printf("SSE: Broadcasted message %d to %d/%d clients", msgID, sentCount, clientCount)
}

// sendMessageToClient sends a message with sequence ID to a specific client
func (h *SSEHandler) sendMessageToClient(client *SSEClient, messageData string) bool {
	// Generate next message ID
	msgID := atomic.AddUint64(&h.messageCounter, 1)

	// Add message ID to the SSE message
	messageWithID := fmt.Sprintf("id: %d\n%s", msgID, messageData)

	select {
	case client.Channel <- messageWithID:
		return true
	default:
		return false
	}
}

// startHeartbeat starts sending periodic heartbeat messages
func (h *SSEHandler) startHeartbeat() {
	h.heartbeatTicker = time.NewTicker(30 * time.Second) // Heartbeat every 30 seconds

	go func() {
		for {
			select {
			case <-h.heartbeatTicker.C:
				h.sendHeartbeat()
			case <-h.stopHeartbeat:
				h.heartbeatTicker.Stop()
				return
			}
		}
	}()
}

// sendHeartbeat sends a heartbeat message to all connected clients
func (h *SSEHandler) sendHeartbeat() {
	if len(h.sseClients) == 0 {
		return
	}

	heartbeatMessage := map[string]any{
		"type":      "heartbeat",
		"timestamp": time.Now().Unix(),
		"message":   "keepalive",
	}

	messageData, err := json.Marshal(heartbeatMessage)
	if err != nil {
		logger := logging.WithPrefix("SSE:Heartbeat")
		logger.Errorf("Error marshaling heartbeat message: %v", err)
		return
	}

	h.broadcastToAllClients(string(messageData))

	logger := logging.WithPrefix("SSE:Heartbeat")
	logger.Debugf("Sent heartbeat to %d clients", len(h.sseClients))
}

// Stop stops the heartbeat timer (cleanup method)
func (h *SSEHandler) Stop() {
	if h.stopHeartbeat != nil {
		close(h.stopHeartbeat)
	}
	if h.heartbeatTicker != nil {
		h.heartbeatTicker.Stop()
	}
}

// broadcastPickUpdate broadcasts pick updates for a specific user with proper visibility filtering
func (h *SSEHandler) BroadcastPickUpdate(userID, season, week int) {
	if h.pickService == nil {
		logger := logging.WithPrefix("SSE")
		logger.Warn("Pick service not available for pick update broadcast")
		return
	}

	if h.visibilityService == nil {
		logger := logging.WithPrefix("SSE")
		logger.Warn("Visibility service not available for pick update broadcast")
		return
	}

	logger := logging.WithPrefix("SSE:PickUpdate")
	logger.Debugf("Broadcasting pick update for user %d, season %d, week %d", userID, season, week)

	// Get current games for the season and week
	games, err := h.gameService.GetGamesBySeason(season)
	if err != nil {
		logger.Errorf("Error fetching games for pick update: %v", err)
		return
	}

	// Filter games by week
	var weekGames []models.Game
	for _, game := range games {
		if game.Week == week {
			weekGames = append(weekGames, game)
		}
	}

	// Get the updated user's picks
	userPicksData, err := h.pickService.GetUserPicksForWeek(context.Background(), userID, season, week)
	if err != nil {
		logger.Errorf("Error fetching user picks for broadcast: %v", err)
		return
	}

	// Convert single user picks to array for template compatibility
	var allUserPicks []*models.UserPicks
	if userPicksData != nil {
		allUserPicks = []*models.UserPicks{userPicksData}
	}

	// Enrich picks with display fields before rendering (CRITICAL for SSE updates)
	enrichLogger := logging.WithPrefix("SSE:PickEnrich")
	for _, up := range allUserPicks {
		enrichLogger.Debugf("Enriching %d picks for user %s", len(up.Picks), up.UserName)
		for i := range up.Picks {
			pick := &up.Picks[i]

			if err := h.pickService.EnrichPickWithGameData(pick); err != nil {
				enrichLogger.Errorf("Failed to enrich pick for Game %d, User %d: %v", pick.GameID, pick.UserID, err)
				continue
			}
		}

		// Populate DailyPickGroups for modern seasons (2025+)
		up.PopulateDailyPickGroups(weekGames, season)
	}

	// CRITICAL SECURITY FIX: Broadcast different content to each client based on their viewing permissions
	clientCount := len(h.sseClients)
	if clientCount == 0 {
		logger.Debug("No SSE clients connected, skipping broadcast")
		return
	}

	sentCount := 0
	for client := range h.sseClients {
		// Apply visibility filtering for this specific viewing user
		viewingUserID := client.UserID // UserID 0 means unauthenticated

		// Filter picks based on what this specific viewer is allowed to see
		filteredPicks, err := h.visibilityService.FilterVisibleUserPicks(
			context.Background(),
			allUserPicks,
			season,
			week,
			viewingUserID,
		)
		if err != nil {
			logger.Errorf("Error filtering picks for viewing user %d: %v", viewingUserID, err)
			continue
		}

		// Skip if no picks are visible to this user
		if len(filteredPicks) == 0 {
			logger.Debugf("No picks visible to viewing user %d, skipping", viewingUserID)
			continue
		}

		// Render template with filtered picks for this specific viewer
		htmlBuffer := &strings.Builder{}
		for _, up := range filteredPicks {
			templateData := map[string]interface{}{
				"UserPicks":     up,
				"Games":         weekGames,
				"IsCurrentUser": viewingUserID == userID, // True if viewing their own picks
				"IsFirst":       false,
				"Season":        season,
				"Week":          week,
			}

			if err := h.templates.ExecuteTemplate(htmlBuffer, "sse-user-picks-block", templateData); err != nil {
				logger.Errorf("Error rendering user picks template for viewer %d: %v", viewingUserID, err)
				continue
			}
		}

		// Send personalized content to this specific client
		htmlContent := htmlBuffer.String()
		message := fmt.Sprintf("user-picks-updated-sse-handler:%s", htmlContent)

		// Send to this specific client only
		if h.sendMessageToClient(client, message) {
			sentCount++
			logger.Debugf("Sent filtered pick update to user %d (viewing user %d's picks)", viewingUserID, userID)
		} else {
			logger.Warnf("Client channel full for user %d, skipping pick update", viewingUserID)
		}
	}

	logger.Infof("Broadcasted filtered pick updates to %d/%d clients for user %d's picks", sentCount, clientCount, userID)
}

// broadcastGameUpdate broadcasts game updates for a specific game
func (h *SSEHandler) BroadcastGameUpdate(gameIDStr string, season, week int) {
	gameID, err := strconv.Atoi(gameIDStr)
	if err != nil {
		logger := logging.WithPrefix("SSE")
		logger.Errorf("Invalid game ID: %s", gameIDStr)
		return
	}

	// Get the specific game
	game, err := h.gameService.GetGameByID(gameID)
	if err != nil {
		logger := logging.WithPrefix("SSE")
		logger.Errorf("Error fetching game %d: %v", gameID, err)
		return
	}

	// Broadcast different types of game updates
	h.broadcastGameStatusHTML(game)
	h.broadcastGameScoresHTML(game)
	h.broadcastPickUpdatesHTML(game)
}

// broadcastGameStatusHTML broadcasts game status updates
func (h *SSEHandler) broadcastGameStatusHTML(game *models.Game) {
	if game == nil {
		return
	}

	// Create targeted game status update using HTMX hx-swap-oob pattern
	htmlBuffer := &strings.Builder{}

	// Create proper hx-swap-oob update for game status
	fmt.Fprintf(htmlBuffer, `<div class="game-status-section" id="game-status-%d-%d-%d" hx-swap-oob="true">`, game.ID, game.Season, game.Week)

	if game.State == "scheduled" {
		fmt.Fprintf(htmlBuffer, `%s`, game.Date.Format("3:04 PM"))
	} else if game.State == "in_play" {
		fmt.Fprintf(htmlBuffer, `<span class="live-indicator">LIVE</span>`)
		if game.Quarter > 0 {
			if game.Quarter == 5 {
				htmlBuffer.WriteString(`<br>OT`)
			} else if game.Quarter == 6 {
				htmlBuffer.WriteString(`<br>2OT`)
			} else {
				fmt.Fprintf(htmlBuffer, `<br>Q%d`, game.Quarter)
			}
		}
	} else if game.State == "completed" {
		htmlBuffer.WriteString(`FINAL`)
	} else {
		htmlBuffer.WriteString(`@`)
	}

	htmlBuffer.WriteString(`</div>`)

	// Create SSE message with hx-swap-oob content
	// Use dashboard-update type which is configured in frontend template
	message := map[string]interface{}{
		"type": "dashboard-update",
		"html": htmlBuffer.String(),
	}

	messageData, err := json.Marshal(message)
	if err != nil {
		logger := logging.WithPrefix("SSE")
		logger.Errorf("Error marshaling game status message: %v", err)
		return
	}

	h.broadcastToAllClients(string(messageData))
	logger := logging.WithPrefix("SSE")
	logger.Debugf("Broadcasted game status update for game %d", game.ID)
}

// broadcastGameScoresHTML broadcasts game score updates
func (h *SSEHandler) broadcastGameScoresHTML(game *models.Game) {
	if game == nil {
		return
	}

	// Create targeted game scores update using HTMX hx-swap-oob pattern
	htmlBuffer := &strings.Builder{}

	// Create proper hx-swap-oob update for game scores
	fmt.Fprintf(htmlBuffer, `<span class="team-score" id="away-score-%d-%d-%d" hx-swap-oob="true">%d</span>`, game.ID, game.Season, game.Week, game.AwayScore)
	fmt.Fprintf(htmlBuffer, `<span class="team-score" id="home-score-%d-%d-%d" hx-swap-oob="true">%d</span>`, game.ID, game.Season, game.Week, game.HomeScore)

	// Create SSE message with hx-swap-oob content
	// Use dashboard-update type which is configured in frontend template
	message := map[string]interface{}{
		"type": "dashboard-update",
		"html": htmlBuffer.String(),
	}

	messageData, err := json.Marshal(message)
	if err != nil {
		logger := logging.WithPrefix("SSE")
		logger.Errorf("Error marshaling game scores message: %v", err)
		return
	}

	h.broadcastToAllClients(string(messageData))
	logger := logging.WithPrefix("SSE")
	logger.Debugf("Broadcasted game scores update for game %d (%d-%d)", game.ID, game.AwayScore, game.HomeScore)
}

// broadcastPickUpdatesHTML broadcasts pick-related updates for a game
func (h *SSEHandler) broadcastPickUpdatesHTML(game *models.Game) {
	if game == nil || h.pickService == nil {
		return
	}

	// For now, we'll broadcast to all users since determining affected users
	// would require getting all user picks which isn't directly supported
	// by the current service interface. This is a simplification for the refactor.
	affectedUsers := make(map[int]bool)

	// Get all connected users from SSE clients
	for client := range h.sseClients {
		if client.UserID > 0 {
			affectedUsers[client.UserID] = true
		}
	}

	// If no users are affected by this game, skip the broadcast
	if len(affectedUsers) == 0 {
		return
	}

	logger := logging.WithPrefix("SSE")
	logger.Debugf("Game %d update affects %d users, broadcasting pick updates", game.ID, len(affectedUsers))

	// Broadcast pick update for each affected user
	for userID := range affectedUsers {
		h.BroadcastPickUpdate(userID, game.Season, game.Week)
	}
}

// BroadcastParlayScoreUpdate broadcasts parlay score updates for a specific season/week
func (h *SSEHandler) BroadcastParlayScoreUpdate(season, week int) {
	logger := logging.WithPrefix("SSE")
	logger.Debugf("Broadcasting parlay score update for season %d, week %d", season, week)

	// Get updated user picks data to render club scores
	if h.pickService != nil {
		ctx := context.Background()
		userPicks, err := h.pickService.GetAllUserPicksForWeek(ctx, season, week)
		if err != nil {
			logger.Errorf("Failed to get user picks for club score update: %v", err)
			return
		}

		// Populate parlay scores for each user - CRITICAL for club scores display
		if h.parlayRepo != nil {
			for _, up := range userPicks {
				// Get user's cumulative parlay score up to this week
				cumulativeScore, err := h.parlayRepo.GetUserCumulativeScoreUpToWeek(ctx, up.UserID, season, week)
				if err != nil {
					logger.Warnf("Failed to get cumulative score for user %d: %v", up.UserID, err)
					continue
				}

				// Get this week's specific score for weekly gain display
				weekScore, err := h.parlayRepo.GetUserParlayScore(ctx, up.UserID, season, week)
				if err != nil {
					logger.Warnf("Failed to get week score for user %d: %v", up.UserID, err)
				}

				weeklyPoints := 0
				if weekScore != nil {
					weeklyPoints = weekScore.TotalPoints
				}

				// Populate the Record field with parlay scoring data
				up.Record.ParlayPoints = cumulativeScore
				up.Record.WeeklyPoints = weeklyPoints
			}
		}

		// Create template data
		templateData := map[string]interface{}{
			"UserPicks": userPicks,
		}

		// Render the club-scores-content template
		htmlBuffer := &strings.Builder{}
		if err := h.templates.ExecuteTemplate(htmlBuffer, "club-scores-content", templateData); err != nil {
			logger.Errorf("Error rendering club-scores template: %v", err)
			return
		}

		// Create hx-swap-oob update for club-scores div
		oobHTML := fmt.Sprintf(`<div class="club-scores" id="club-scores" hx-swap-oob="true">%s</div>`, htmlBuffer.String())

		// Send the HTML update via SSE
		message := fmt.Sprintf("parlay-scores-updated:%s", oobHTML)
		h.broadcastToAllClients(message)

		logger.Infof("Broadcasted club scores HTML update to %d clients", len(h.sseClients))
	} else {
		logger.Warn("Pick service not available for parlay score HTML broadcast")
	}
}
