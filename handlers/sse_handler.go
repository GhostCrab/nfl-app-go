package handlers

import (
	"context"
	"fmt"
	"html/template"
	"net/http"
	"nfl-app-go/config"
	"nfl-app-go/database"
	"nfl-app-go/logging"
	"nfl-app-go/middleware"
	"nfl-app-go/models"
	"nfl-app-go/services"
	"regexp"
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

// SSEMessage represents a queued SSE message with ordering and targeting info
type SSEMessage struct {
	EventType    string
	Data         string
	Reserved     uint64     // Pre-reserved message ID
	TargetClient *SSEClient // nil = broadcast to all clients
}

// SSEHandler handles all Server-Sent Events functionality
type SSEHandler struct {
	templates         *template.Template
	gameService       services.GameService
	pickService       *services.PickService
	authService       *services.AuthService
	visibilityService *services.PickVisibilityService
	memoryScorer      *services.MemoryParlayScorer
	userRepo          *database.MongoUserRepository // Added for BroadcastPickUpdate fix
	sseClients        map[*SSEClient]bool
	messageCounter    uint64          // Atomic counter for message sequencing
	messageQueue      chan SSEMessage // Ordered message queue
	heartbeatTicker   *time.Ticker    // Heartbeat timer
	stopHeartbeat     chan bool       // Channel to stop heartbeat
	stopMessageQueue  chan bool       // Channel to stop message processor
	config            *config.Config
}

// NewSSEHandler creates a new SSE handler
func NewSSEHandler(templates *template.Template, gameService services.GameService, cfg *config.Config) *SSEHandler {
	handler := &SSEHandler{
		templates:        templates,
		gameService:      gameService,
		sseClients:       make(map[*SSEClient]bool),
		messageQueue:     make(chan SSEMessage, 1000), // Buffered queue for message ordering
		stopHeartbeat:    make(chan bool),
		stopMessageQueue: make(chan bool),
		config:           cfg,
	}

	// Start message processor for ordered delivery
	go handler.messageProcessor()

	// Start heartbeat goroutine
	handler.startHeartbeat()

	return handler
}

// SetServices sets the required services
func (h *SSEHandler) SetServices(pickService *services.PickService, authService *services.AuthService, visibilityService *services.PickVisibilityService, memoryScorer *services.MemoryParlayScorer, userRepo *database.MongoUserRepository) {
	h.pickService = pickService
	h.authService = authService
	h.visibilityService = visibilityService
	h.memoryScorer = memoryScorer
	h.userRepo = userRepo
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
	fmt.Fprintf(w, "event: connection\ndata: SSE connection established\n\n")
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

	// Handle weekly_picks collection changes
	if event.Collection == "weekly_picks" && event.UserID > 0 {
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

// getNextMessageID returns the next atomic message ID for SSE ordering
func (h *SSEHandler) getNextMessageID() uint64 {
	return atomic.AddUint64(&h.messageCounter, 1)
}

// BroadcastToAllClients sends a message with sequence ID to all connected SSE clients via queue
func (h *SSEHandler) BroadcastToAllClients(eventType, data string) {
	if len(h.sseClients) == 0 {
		return
	}

	// Reserve message ID and queue for ordered delivery
	msgID := h.getNextMessageID()
	message := SSEMessage{
		EventType:    eventType,
		Data:         compactHTMLForSSE(data),
		Reserved:     msgID,
		TargetClient: nil, // nil = broadcast to all clients
	}

	// Send to queue for ordered processing
	select {
	case h.messageQueue <- message:
		// Message queued successfully
	default:
		logger := logging.WithPrefix("SSE")
		logger.Warnf("Message queue full, dropping broadcast message %d", msgID)
	}
}

// sendMessageToClient sends a message with sequence ID to a specific client
func (h *SSEHandler) sendMessageToClient(client *SSEClient, messageData string) bool {
	select {
	case client.Channel <- messageData:
		return true
	default:
		return false
	}
}

// messageProcessor processes queued messages in order to prevent race conditions
func (h *SSEHandler) messageProcessor() {
	logger := logging.WithPrefix("SSE:MessageProcessor")
	logger.Debug("Message processor started")

	for {
		select {
		case message := <-h.messageQueue:
			// Process message in order
			if message.TargetClient != nil {
				// Send to specific client
				messageData := fmt.Sprintf("id: %d\nevent: %s\ndata: %s\n\n", message.Reserved, message.EventType, message.Data)
				h.sendMessageToClient(message.TargetClient, messageData)
			} else {
				// Broadcast to all clients
				h.broadcastDirectly(message.EventType, message.Data, message.Reserved)
			}
		case <-h.stopMessageQueue:
			logger.Debug("Message processor stopped")
			return
		}
	}
}

// broadcastDirectly sends message directly to all clients with pre-reserved ID
func (h *SSEHandler) broadcastDirectly(eventType, data string, messageID uint64) {
	messageData := fmt.Sprintf("id: %d\nevent: %s\ndata: %s\n\n", messageID, eventType, data)

	logger := logging.WithPrefix("SSE")
	clientCount := len(h.sseClients)
	sentCount := 0

	for client := range h.sseClients {
		select {
		case client.Channel <- messageData:
			sentCount++
		default:
			logger.Warnf("Client channel full, skipping message %d", messageID)
		}
	}

	if clientCount > 0 {
		logger.Debugf("Broadcasted message %d (%s) to %d/%d clients", messageID, eventType, sentCount, clientCount)
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
	logger := logging.WithPrefix("SSE:Heartbeat")

	if len(h.sseClients) == 0 {
		return
	}

	h.BroadcastToAllClients("heartbeat", "keep-alive")

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

// broadcastPickUpdate broadcasts pick updates for ALL users with proper visibility filtering
// CRITICAL FIX: Now matches dashboard logic exactly - gets ALL users' picks to prevent user disappearing
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
	logger.Debugf("Broadcasting pick update triggered by user %d, season %d, week %d", userID, season, week)

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

	// CRITICAL FIX: Get ALL users' picks (same as dashboard initial load)
	allUserPicks, err := h.pickService.GetAllUserPicksForWeek(context.Background(), season, week)
	if err != nil {
		logger.Errorf("Error fetching all user picks for broadcast: %v", err)
		return
	}

	// CRITICAL FIX: Get all users to ensure empty entries exist (matches dashboard logic)
	users, err := h.userRepo.GetAllUsers()
	if err != nil {
		logger.Errorf("Error fetching users for pick update: %v", err)
		return
	}

	// CRITICAL FIX: Ensure all users have a pick entry, even if empty (matches dashboard logic)
	userPicksMap := make(map[int]*models.UserPicks)
	for _, up := range allUserPicks {
		userPicksMap[up.UserID] = up
	}

	// Add empty pick entries for users who don't have picks this week
	for _, u := range users {
		if _, exists := userPicksMap[u.ID]; !exists {
			emptyUserPicks := &models.UserPicks{
				UserID:   u.ID,
				UserName: u.Name,
				Picks:    []models.Pick{},
				Record: models.UserRecord{
					Wins:   0,
					Losses: 0,
					Pushes: 0,
				},
			}
			allUserPicks = append(allUserPicks, emptyUserPicks)
			userPicksMap[u.ID] = emptyUserPicks
		}
	}

	// CRITICAL FIX: Enrich picks with display fields and PopulateDailyPickGroups (matches dashboard logic)
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

		// CRITICAL FIX: Populate DailyPickGroups for ALL modern seasons (matches dashboard logic)
		up.PopulateDailyPickGroups(weekGames, season)
	}

	// CRITICAL SECURITY FIX: Broadcast different content to each client based on their viewing permissions
	clientCount := len(h.sseClients)
	if clientCount == 0 {
		logger.Debug("No SSE clients connected, skipping broadcast")
		return
	}

	// Process each client individually with their own message ID for ordering
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
				"UseOOBSwap":    true, // Enable OOB swapping for SSE updates
			}

			if err := h.templates.ExecuteTemplate(htmlBuffer, "user-picks-block", templateData); err != nil {
				logger.Errorf("Error rendering user picks template for viewer %d: %v", viewingUserID, err)
				continue
			}
		}

		// Send personalized content to this specific client via queue
		htmlContent := compactHTMLForSSE(htmlBuffer.String())

		// Reserve message ID and queue for ordered delivery
		msgID := h.getNextMessageID()
		message := SSEMessage{
			EventType:    "user-picks-updated",
			Data:         htmlContent,
			Reserved:     msgID,
			TargetClient: client, // Send to specific client only
		}

		// Send to queue for ordered processing
		select {
		case h.messageQueue <- message:
			sentCount++
			logger.Debugf("Queued filtered pick update for user %d (viewing user %d's picks)", viewingUserID, userID)
		default:
			logger.Warnf("Message queue full, dropping pick update for user %d", viewingUserID)
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
	fmt.Fprintf(htmlBuffer, `<div class="live-pick-expansion" id="live-expansion-%d-%d" hx-swap-oob="true">`, pick.UserID, pick.GameID)

	// Game clock (pick-specific)
	fmt.Fprintf(htmlBuffer, `<span class="game-clock" id="game-time-%d-%d">%s`, pick.UserID, pick.GameID, game.GetGameClock())
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
			fmt.Fprintf(htmlBuffer, `<span class="status-divider"> || </span><span class="game-possession" id="possession-%d-%d">%s</span>`, pick.UserID, pick.GameID, possessionStr)
		}
	}

	htmlBuffer.WriteString(`</div>`)

	h.BroadcastToAllClients("live-pick-expansion", htmlBuffer.String())
	logger.Debugf("Broadcasted live pick expansion update for pick %d-%d (game %d)", pick.UserID, pick.GameID, game.ID)
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
	h.BroadcastToAllClients("game-clock-update", html)

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
	h.BroadcastToAllClients("possession-update", html)

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

	// Use the complete game-row-update template for state transitions
	htmlBuffer := &strings.Builder{}
	templateData := struct {
		*models.Game
		DisplayIDTooltips bool
	}{
		Game:              game,
		DisplayIDTooltips: h.config != nil && h.config.App.DisplayIDTooltips,
	}
	if err := h.templates.ExecuteTemplate(htmlBuffer, "game-row-update", templateData); err != nil {
		logger.Errorf("Error rendering game-row-update template for game %d: %v", game.ID, err)
		return
	}

	// Send the complete game row update
	h.BroadcastToAllClients("game-state-update", htmlBuffer.String())
	logger.Debugf("Broadcasted complete game state update for game %d (state: %s)", game.ID, game.State)

	// Send pick item updates for all affected picks when game state changes
	h.broadcastPickUpdatesForGame(game)
}

// broadcastPickUpdatesForGame broadcasts consolidated pick item updates for all picks associated with a game
// CRITICAL: Now uses consolidated approach like game updates - single broadcast with all pick updates
func (h *SSEHandler) broadcastPickUpdatesForGame(game *models.Game) {
	if game == nil || h.pickService == nil {
		return
	}

	logger := logging.WithPrefix("SSE:PickGameUpdate")

	// Get all user picks for this week to find picks for this game
	ctx := context.Background()
	allUserPicks, err := h.pickService.GetAllUserPicksForWeek(ctx, game.Season, game.Week)
	if err != nil {
		logger.Errorf("Failed to get picks for game %d: %v", game.ID, err)
		return
	}

	// Get games array for template compatibility
	games, err := h.gameService.GetGamesBySeason(game.Season)
	if err != nil {
		logger.Errorf("Error fetching games for pick updates: %v", err)
		return
	}

	// Collect all picks for this game into a single consolidated update
	htmlBuffer := &strings.Builder{}
	pickCount := 0

	for _, userPicks := range allUserPicks {
		for _, pick := range userPicks.Picks {
			if pick.GameID == game.ID {
				// Create template data for each pick
				templateData := map[string]interface{}{
					"Pick":          &pick,
					"Games":         games,
					"IsCurrentUser": false, // Will be handled by client-side filtering
				}

				// Render the pick-item-update template (includes both pick-item AND live-expansion with OOB)
				if err := h.templates.ExecuteTemplate(htmlBuffer, "pick-item-update", templateData); err != nil {
					logger.Errorf("Error rendering pick-item-update for pick %d-%d: %v", pick.UserID, pick.GameID, err)
					continue
				}

				pickCount++
			}
		}
	}

	if pickCount > 0 {
		// Send single consolidated update with all pick updates for this game
		h.BroadcastToAllClients("pick-item-updated", htmlBuffer.String())
		logger.Debugf("Broadcasted consolidated pick updates for %d picks associated with game %d (state: %s)", pickCount, game.ID, game.State)
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
	h.BroadcastToAllClients("dashboard-update", htmlBuffer.String())
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
	h.BroadcastToAllClients("dashboard-update", htmlBuffer.String())
	logger := logging.WithPrefix("SSE")
	logger.Debugf("Broadcasted game scores update for game %d (%d-%d)", game.ID, game.AwayScore, game.HomeScore)
}

// BroadcastParlayScoreUpdate broadcasts parlay score updates for a specific season/week
// Rebuilt to match current cumulative scoring implementation using MemoryParlayScorer
func (h *SSEHandler) BroadcastParlayScoreUpdate(season, week int) {
	logger := logging.WithPrefix("SSE:ParlayScore")
	logger.Debugf("Broadcasting parlay score update for season %d, week %d", season, week)

	if h.pickService == nil {
		logger.Warn("Pick service not available for parlay score broadcast")
		return
	}

	if h.memoryScorer == nil {
		logger.Warn("Memory scorer not available for parlay score broadcast")
		return
	}

	ctx := context.Background()

	// Get all user picks for this week to build club scores display
	userPicks, err := h.pickService.GetAllUserPicksForWeek(ctx, season, week)
	if err != nil {
		logger.Errorf("Failed to get user picks for club score update: %v", err)
		return
	}

	// Populate parlay scores using the same logic as PickService (CRITICAL for consistency)
	for _, up := range userPicks {
		// Get season total up to current week for ParlayPoints (cumulative)
		seasonTotal := h.memoryScorer.GetUserSeasonTotal(season, week, up.UserID)
		up.Record.ParlayPoints = seasonTotal

		// Get current week's points for WeeklyPoints display
		if weekParlayScore, exists := h.memoryScorer.GetUserScore(season, week, up.UserID); exists {
			up.Record.WeeklyPoints = weekParlayScore.TotalPoints
		} else {
			up.Record.WeeklyPoints = 0
		}

		logger.Debugf("User %d scores - Season total: %d, Week %d: %d",
			up.UserID, up.Record.ParlayPoints, week, up.Record.WeeklyPoints)
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
	h.BroadcastToAllClients("parlay-scores-updated", oobHTML)

	logger.Infof("Broadcasted club scores update to %d clients for season %d, week %d", len(h.sseClients), season, week)
}

// BroadcastPickContainerRefresh broadcasts complete pick-container updates for visibility changes
// This is used by VisibilityTimerService when pick visibility cutoffs are reached
func (h *SSEHandler) BroadcastPickContainerRefresh(season, week int) {
	logger := logging.WithPrefix("SSE:PickVisibility")
	logger.Infof("Broadcasting full pick container refresh for season %d, week %d", season, week)

	if h.pickService == nil {
		logger.Warn("Pick service not available for pick container refresh")
		return
	}

	ctx := context.Background()

	// Get all user picks for the specified week with updated visibility
	userPicks, err := h.pickService.GetAllUserPicksForWeek(ctx, season, week)
	if err != nil {
		logger.Errorf("Failed to get user picks for visibility refresh: %v", err)
		return
	}

	// Apply pick visibility filtering for current time (this will show newly visible picks)
	if h.visibilityService != nil {
		// Use anonymous user ID (-1) to get the base visibility (could be enhanced per-user if needed)
		filteredUserPicks, err := h.visibilityService.FilterVisibleUserPicks(ctx, userPicks, season, week, -1)
		if err != nil {
			logger.Warnf("Failed to filter pick visibility for refresh: %v", err)
		} else {
			userPicks = filteredUserPicks
			logger.Debugf("Applied pick visibility filtering for season %d, week %d", season, week)
		}
	}

	// Create template data for pick container
	templateData := map[string]interface{}{
		"UserPicks": userPicks,
	}

	// Render the pick-container content with updated visibility
	htmlBuffer := &strings.Builder{}
	if err := h.templates.ExecuteTemplate(htmlBuffer, "picks-section", templateData); err != nil {
		logger.Errorf("Error rendering picks-section template for visibility refresh: %v", err)
		return
	}

	// Create hx-swap-oob update for pick-container div
	oobHTML := fmt.Sprintf(`<div class="pick-container" id="pick-container" hx-swap-oob="true">%s</div>`, htmlBuffer.String())

	// Send the HTML update via SSE
	h.BroadcastToAllClients("pick-container-refresh", oobHTML)

	logger.Infof("Broadcasted pick container refresh to %d clients for season %d, week %d", len(h.sseClients), season, week)
}

// handleGameCompletion handles game state change events to update pick results and recalculate parlay scores
func (h *SSEHandler) handleGameCompletion(season, week int, gameID string) {
	logger := logging.WithPrefix("SSE:GameStateChange")
	ctx := context.Background()

	if h.memoryScorer == nil || h.pickService == nil {
		logger.Warn("Memory scorer or pick service not available")
		return
	}

	// Get the current game to check its state
	gameIDInt, err := strconv.Atoi(gameID)
	if err != nil {
		logger.Errorf("Invalid game ID: %s", gameID)
		return
	}

	game, err := h.gameService.GetGameByID(gameIDInt)
	if err != nil {
		logger.Errorf("Failed to get game %s: %v", gameID, err)
		return
	}

	logger.Infof("Game %s state changed to %s for season %d, week %d - updating pick results", gameID, game.State, season, week)

	// CRITICAL: Update pick results in database based on new game state
	if game.IsCompleted() {
		// Game completed - calculate pick results
		logger.Infof("Processing game completion for game %s", gameID)
		if err := h.pickService.ProcessGameCompletion(ctx, game); err != nil {
			logger.Errorf("Failed to process game completion for game %s: %v", gameID, err)
		} else {
			logger.Infof("Successfully updated pick results for completed game %s", gameID)
		}
	} else {
		// Game transitioned to non-completed state (in_play or scheduled) - reset pick results to pending
		logger.Infof("Game %s transitioned to %s state - resetting pick results to pending", gameID, game.State)
		if err := h.pickService.ResetPickResultsForGame(ctx, game); err != nil {
			logger.Errorf("Failed to reset pick results for game %s: %v", gameID, err)
		} else {
			logger.Infof("Successfully reset pick results for game %s", gameID)
		}
	}

	// Recalculate in-memory parlay scores for each user who made picks this week
	allUserPicks, err := h.pickService.GetAllUserPicksForWeek(ctx, season, week)
	if err != nil {
		logger.Errorf("Failed to get user picks for week %d: %v", week, err)
		return
	}

	updatedCount := 0
	for _, userPicks := range allUserPicks {
		_, err := h.memoryScorer.RecalculateUserScore(ctx, season, week, userPicks.UserID)
		if err != nil {
			logger.Errorf("Failed to recalculate score for user %d: %v", userPicks.UserID, err)
			continue
		}
		updatedCount++
	}

	logger.Infof("Recalculated in-memory parlay scores for %d users", updatedCount)

	// Broadcast the updated scores via SSE (club scores - in-memory only)
	if updatedCount > 0 {
		h.BroadcastParlayScoreUpdate(season, week)
	}
}

// BroadcastPickItemUpdate broadcasts complete pick item updates for state transitions
func (h *SSEHandler) BroadcastPickItemUpdate(game *models.Game, pick *models.Pick, viewingUserID int) {
	if game == nil || pick == nil {
		return
	}

	logger := logging.WithPrefix("SSE:PickItem")

	// Get games array for template compatibility (unified-pick-item expects this)
	games, err := h.gameService.GetGamesBySeason(game.Season)
	if err != nil {
		logger.Errorf("Error fetching games for pick item update: %v", err)
		return
	}

	// Create template data for the pick item update
	templateData := map[string]interface{}{
		"Pick":          pick,
		"Games":         games,
		"IsCurrentUser": viewingUserID == pick.UserID,
	}

	// Render the pick-item-update template
	htmlBuffer := &strings.Builder{}
	if err := h.templates.ExecuteTemplate(htmlBuffer, "pick-item-update", templateData); err != nil {
		logger.Errorf("Error rendering pick-item-update template for pick %d-%d: %v", pick.UserID, pick.GameID, err)
		return
	}

	// Get message ID for consistent ordering
	msgID := h.getNextMessageID()

	// Send personalized update to specific client
	message := fmt.Sprintf("id: %d\nevent: pick-item-updated\ndata: %s\n\n", msgID, compactHTMLForSSE(htmlBuffer.String()))

	// Send to all clients (they'll filter based on visibility on their end)
	sentCount := 0
	for client := range h.sseClients {
		if h.sendMessageToClient(client, message) {
			sentCount++
		}
	}

	logger.Debugf("Broadcasted pick item update for pick %d-%d (game %d, state: %s) to %d clients",
		pick.UserID, pick.GameID, game.ID, game.State, sentCount)
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
		`<span class="current-value %s" id="pick-status-%d-%d" hx-swap-oob="true">%s</span>`,
		colorClass, pick.UserID, pick.GameID, statusValue,
	)

	h.BroadcastToAllClients("pick-status-update", pickStatusHTML)
	logger.Debugf("Broadcasted pick status update for pick %d-%d (game %d): %s (%s)",
		pick.UserID, pick.GameID, game.ID, statusValue, colorClass)
}

func compactHTMLForSSE(data string) string {
	// Remove newlines and extra spaces between tags, but preserve content spaces
	lines := strings.Split(data, "\n")
	var cleaned []string

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed != "" {
			cleaned = append(cleaned, trimmed)
		}
	}

	// Join with spaces to preserve readability
	joined := strings.Join(cleaned, " ")

	// Remove spaces between adjacent tags only
	re := regexp.MustCompile(`>\s+<`)
	return re.ReplaceAllString(joined, "><")
}
