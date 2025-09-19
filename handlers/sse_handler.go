package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"html/template"
	"net/http"
	"nfl-app-go/logging"
	"nfl-app-go/middleware"
	"nfl-app-go/models"
	"nfl-app-go/services"
	"strconv"
	"strings"
	"sync/atomic"
	"time"
)

// SSEClient represents a connected SSE client with user context
type SSEClient struct {
	Channel chan string
	UserID  int
}

// SSEHandler handles all Server-Sent Events functionality
type SSEHandler struct {
	templates         *template.Template
	gameService       services.GameService
	pickService       *services.PickService
	authService       *services.AuthService
	visibilityService *services.PickVisibilityService
	memoryScorer      *services.MemoryParlayScorer
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
func (h *SSEHandler) SetServices(pickService *services.PickService, authService *services.AuthService, visibilityService *services.PickVisibilityService, memoryScorer *services.MemoryParlayScorer) {
	h.pickService = pickService
	h.authService = authService
	h.visibilityService = visibilityService
	h.memoryScorer = memoryScorer
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

	// Send initial connection message using proper SSE format
	fmt.Fprintf(w, "id: 0\nevent: connection\ndata: SSE connection established\n\n")
	flusher.Flush()

	// Listen for messages
	for {
		select {
		case message, ok := <-client.Channel:
			if !ok {
				return
			}
			// Message is already in proper SSE format (id:/event:/data:)
			// Just write it directly to the response
			fmt.Fprint(w, message)
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

	// Handle game completion events to trigger parlay score recalculation
	if event.Collection == "games" && h.memoryScorer != nil {
		// Check if this is a meaningful game state change that affects scoring
		if event.UpdatedFields != nil {
			if _, hasState := event.UpdatedFields["state"]; hasState {
				h.handleGameCompletion(event.Season, event.Week, event.GameID)
			}
		}
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
	// Create proper SSE format with event: and data: lines (no JSON)
	// HTMX SSE extension expects this exact format
	message := fmt.Sprintf("event: %s\ndata: %s\n\n", eventType, data)
	h.broadcastToAllClients(message)
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

		// Send personalized content to this specific client using proper SSE format
		htmlContent := htmlBuffer.String()

		// Create proper SSE message with HTML content (no JSON wrapping)
		// HTMX SSE extension expects event: and data: lines with raw HTML
		message := fmt.Sprintf("event: user-picks-updated\ndata: %s\n\n", htmlContent)

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

// BroadcastLivePickExpansionForUser broadcasts targeted updates for all of a user's live pick expansions for a game
func (h *SSEHandler) BroadcastLivePickExpansionForUser(game *models.Game, userID int) {
	if game == nil || game.State != "in_play" {
		return
	}

	logger := logging.WithPrefix("SSE:LivePickExpansion")

	// Get user's picks for this week to find picks for this game
	ctx := context.Background()
	userPicks, err := h.pickService.GetUserPicksForWeek(ctx, userID, game.Season, game.Week)
	if err != nil {
		logger.Errorf("Failed to get picks for user %d: %v", userID, err)
		return
	}

	if userPicks == nil || len(userPicks.Picks) == 0 {
		return // User has no picks this week
	}

	// Find picks for this specific game and broadcast updates for each
	for _, pick := range userPicks.Picks {
		if pick.GameID == game.ID {
			h.BroadcastLivePickExpansionForPick(game, &pick)
		}
	}
}

// BroadcastLivePickExpansionForPick broadcasts targeted update for a specific pick's live expansion
func (h *SSEHandler) BroadcastLivePickExpansionForPick(game *models.Game, pick *models.Pick) {
	if game == nil || game.State != "in_play" || pick == nil {
		return
	}

	logger := logging.WithPrefix("SSE:LivePickExpansion")

	// Create targeted live-pick-expansion update using pick ID
	htmlBuffer := &strings.Builder{}

	// Target the specific live-pick-expansion div for this pick
	fmt.Fprintf(htmlBuffer, `<div class="live-pick-expansion" id="live-expansion-%s" hx-swap-oob="true">`, pick.ID.Hex())

	// Game clock (pick-specific)
	fmt.Fprintf(htmlBuffer, `<span class="game-clock" id="game-time-%s">%s`, pick.ID.Hex(), game.GetGameClock())
	if game.Quarter == 5 {
		htmlBuffer.WriteString(` OT`)
	} else if game.Quarter == 6 {
		htmlBuffer.WriteString(` 2OT`)
	} else if game.Quarter > 0 {
		fmt.Fprintf(htmlBuffer, ` Q%d`, game.Quarter)
	}
	htmlBuffer.WriteString(`</span>`)

	// Possession info
	if game.HasStatus() {
		possessionStr := game.GetPossessionString()
		if possessionStr != "" {
			fmt.Fprintf(htmlBuffer, `<span class="status-divider"> || </span><span class="game-possession" id="possession-%s">%s</span>`, pick.ID.Hex(), possessionStr)
		}
	}

	htmlBuffer.WriteString(`</div>`)

	h.BroadcastStructuredUpdate("live-pick-expansion", htmlBuffer.String())
	logger.Debugf("Broadcasted live pick expansion update for pick %s (game %d)", pick.ID.Hex(), game.ID)
}

// BroadcastGameClockUpdate broadcasts targeted game clock updates
func (h *SSEHandler) BroadcastGameClockUpdate(game *models.Game) {
	if game == nil || game.State != "in_play" {
		return
	}

	// Create targeted game clock update
	clockText := game.GetGameClock()
	if game.Quarter == 5 {
		clockText += " OT"
	} else if game.Quarter == 6 {
		clockText += " 2OT"
	} else if game.Quarter > 0 {
		clockText += fmt.Sprintf(" Q%d", game.Quarter)
	}

	html := fmt.Sprintf(`<span class="status-item game-clock" id="game-time-%d-%d-%d" hx-swap-oob="true">%s</span>`, game.ID, game.Season, game.Week, clockText)
	h.BroadcastStructuredUpdate("game-clock-update", html)

	logger := logging.WithPrefix("SSE:GameClock")
	logger.Debugf("Broadcasted game clock update for game %d: %s", game.ID, clockText)
}

// BroadcastPossessionUpdate broadcasts targeted possession updates
func (h *SSEHandler) BroadcastPossessionUpdate(game *models.Game) {
	if game == nil || game.State != "in_play" || !game.HasStatus() {
		return
	}

	possessionStr := game.GetPossessionString()
	if possessionStr == "" {
		return
	}

	html := fmt.Sprintf(`<span class="game-possession" id="possession-%d" hx-swap-oob="true">%s</span>`, game.ID, possessionStr)
	h.BroadcastStructuredUpdate("possession-update", html)

	logger := logging.WithPrefix("SSE:Possession")
	logger.Debugf("Broadcasted possession update for game %d: %s", game.ID, possessionStr)
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

	// Consolidate game updates to reduce rapid-fire SSE messages
	h.broadcastConsolidatedGameUpdate(game)
}

// broadcastConsolidatedGameUpdate sends consolidated updates to reduce SSE message volume
func (h *SSEHandler) broadcastConsolidatedGameUpdate(game *models.Game) {
	if game == nil {
		return
	}

	logger := logging.WithPrefix("SSE")

	// Send game status and scores as single consolidated update
	htmlBuffer := &strings.Builder{}

	// Game status section
	fmt.Fprintf(htmlBuffer, `<div class="game-status-section" id="game-status-%d-%d-%d" hx-swap-oob="true">`, game.ID, game.Season, game.Week)
	if game.State == "scheduled" {
		fmt.Fprintf(htmlBuffer, `<div class="game-time">%s</div>`, game.FormatGameTime())
	} else if game.State == "in_play" || game.State == "completed" {
		htmlBuffer.WriteString(`<span class="live-indicator">LIVE</span><br>`)
		if game.Quarter == 5 {
			htmlBuffer.WriteString(`OT`)
		} else if game.Quarter == 6 {
			htmlBuffer.WriteString(`2OT`)
		} else {
			fmt.Fprintf(htmlBuffer, `Q%d`, game.Quarter)
		}
	}
	htmlBuffer.WriteString(`</div>`)

	// Team scores
	fmt.Fprintf(htmlBuffer, `<span class="team-score" id="away-score-%d-%d-%d" hx-swap-oob="true">%d</span>`,
		game.ID, game.Season, game.Week, game.AwayScore)
	fmt.Fprintf(htmlBuffer, `<span class="team-score" id="home-score-%d-%d-%d" hx-swap-oob="true">%d</span>`,
		game.ID, game.Season, game.Week, game.HomeScore)

	// Global game clock (if live)
	if game.State == "in_play" {
		clockText := game.GetGameClock()
		if game.Quarter == 5 {
			clockText += " OT"
		} else if game.Quarter == 6 {
			clockText += " 2OT"
		} else if game.Quarter > 0 {
			clockText += fmt.Sprintf(" Q%d", game.Quarter)
		}
		fmt.Fprintf(htmlBuffer, `<span class="status-item game-clock" id="game-time-%d-%d-%d" hx-swap-oob="true">%s</span>`,
			game.ID, game.Season, game.Week, clockText)

		// Global possession
		if game.HasStatus() {
			possessionStr := game.GetPossessionString()
			if possessionStr != "" {
				fmt.Fprintf(htmlBuffer, `<span class="game-possession" id="possession-%d" hx-swap-oob="true">%s</span>`,
					game.ID, possessionStr)
			}
		}
	}

	// Send consolidated update
	h.BroadcastStructuredUpdate("dashboard-update", htmlBuffer.String())
	logger.Debugf("Broadcasted consolidated game update for game %d", game.ID)

	// Send live pick expansion updates separately (but only for live games)
	if game.State == "in_play" {
		// Small delay to prevent overwhelming HTMX
		time.Sleep(50 * time.Millisecond)

		// Send targeted pick updates
		for client := range h.sseClients {
			if client.UserID > 0 {
				h.BroadcastLivePickExpansionForUser(game, client.UserID)
			}
		}
	}
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

	// Create proper SSE message with HTML content (no JSON wrapping)
	// HTMX SSE extension expects raw HTML with hx-swap-oob attributes
	h.BroadcastStructuredUpdate("dashboard-update", htmlBuffer.String())
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

	// Create proper SSE message with HTML content (no JSON wrapping)
	// HTMX SSE extension expects raw HTML with hx-swap-oob attributes
	h.BroadcastStructuredUpdate("dashboard-update", htmlBuffer.String())
	logger := logging.WithPrefix("SSE")
	logger.Debugf("Broadcasted game scores update for game %d (%d-%d)", game.ID, game.AwayScore, game.HomeScore)
}

// REMOVED: broadcastPickUpdatesHTML was causing massive 400+ line DOM updates
// Replaced with targeted updates: BroadcastLivePickExpansion, BroadcastGameClockUpdate, etc.

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
		if h.memoryScorer != nil {
			for _, up := range userPicks {
				// Get this week's specific score from memory scorer
				weekScore, exists := h.memoryScorer.GetUserScore(season, week, up.UserID)
				weeklyPoints := 0
				if exists && weekScore != nil {
					weeklyPoints = weekScore.TotalPoints
				}

				// For now, use weekly points as cumulative (will need separate cumulative calculation)
				// TODO: Implement proper cumulative scoring across multiple weeks
				cumulativeScore := weeklyPoints

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

// handleGameCompletion handles game completion events to recalculate parlay scores
func (h *SSEHandler) handleGameCompletion(season, week int, gameID string) {
	logger := logging.WithPrefix("SSE:GameCompletion")
	ctx := context.Background()

	if h.memoryScorer == nil || h.pickService == nil {
		logger.Warn("Memory scorer or pick service not available")
		return
	}

	logger.Infof("Game %s completed for season %d, week %d - recalculating parlay scores", gameID, season, week)

	// Get all users who made picks for this week
	allUserPicks, err := h.pickService.GetAllUserPicksForWeek(ctx, season, week)
	if err != nil {
		logger.Errorf("Failed to get user picks for week %d: %v", week, err)
		return
	}

	// Recalculate scores for each user who made picks this week
	updatedCount := 0
	for _, userPicks := range allUserPicks {
		_, err := h.memoryScorer.RecalculateUserScore(ctx, season, week, userPicks.UserID)
		if err != nil {
			logger.Errorf("Failed to recalculate score for user %d: %v", userPicks.UserID, err)
			continue
		}
		updatedCount++
	}

	logger.Infof("Recalculated parlay scores for %d users", updatedCount)

	// Broadcast the updated scores via SSE
	if updatedCount > 0 {
		h.BroadcastParlayScoreUpdate(season, week)
	}
}

// BroadcastPickStatusUpdate broadcasts targeted pick status updates when game results change
// This updates the current-value span and color class for specific picks
func (h *SSEHandler) BroadcastPickStatusUpdate(game *models.Game, pick *models.Pick) {
	logger := logging.WithPrefix("SSE")

	// Determine current status and color class
	var colorClass, statusValue string

	if game.State == "in_play" {
		// For spread picks
		if pick.TeamName == "OVER" || pick.TeamName == "UNDER" {
			// O/U pick logic
			currentTotal := game.HomeScore + game.AwayScore
			overUnder := int(game.Odds.OU)

			if pick.TeamName == "OVER" {
				if currentTotal > overUnder {
					colorClass = "pick-status-winning"
				} else if currentTotal == overUnder {
					colorClass = "pick-status-push"
				} else {
					colorClass = "pick-status-pace"
				}
			} else { // UNDER
				if currentTotal < overUnder {
					colorClass = "pick-status-winning"
				} else if currentTotal == overUnder {
					colorClass = "pick-status-push"
				} else {
					colorClass = "pick-status-pace"
				}
			}
			statusValue = fmt.Sprintf("%d", currentTotal)
		} else {
			// Spread pick logic - determine if picked team is covering
			var differential int
			if pick.TeamName == game.Away || strings.Contains(pick.TeamName, game.Away) {
				differential = game.AwayScore - game.HomeScore
			} else {
				differential = game.HomeScore - game.AwayScore
			}

			// Check if covering the spread (using same logic as template)
			if differential > 0 {
				colorClass = "pick-status-winning"
			} else if differential == 0 {
				colorClass = "pick-status-push"
			} else {
				colorClass = "pick-status-losing"
			}

			if differential == 0 {
				statusValue = "TIE"
			} else if differential > 0 {
				statusValue = fmt.Sprintf("+%d", differential)
			} else {
				statusValue = fmt.Sprintf("%d", differential)
			}
		}
	} else if game.State == "completed" {
		// Final status for completed games
		// Similar logic but with final determination
		// TODO: Implement final status logic when game is completed
		return // Skip for now, handle in-play updates first
	} else {
		// Game not started yet
		return
	}

	// Create targeted update HTML for the current-value span using pick ID
	pickStatusHTML := fmt.Sprintf(
		`<span class="current-value %s" id="pick-status-%s" hx-swap-oob="true">%s</span>`,
		colorClass, pick.ID.Hex(), statusValue,
	)

	h.BroadcastStructuredUpdate("pick-status-update", pickStatusHTML)
	logger.Debugf("Broadcasted pick status update for pick %s (game %d): %s (%s)",
		pick.ID.Hex(), game.ID, statusValue, colorClass)
}
