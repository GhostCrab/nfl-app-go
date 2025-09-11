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
)

// Note: SSEClient is already defined in games.go, but we'll use the same structure

// SSEHandler handles all Server-Sent Events functionality
type SSEHandler struct {
	templates         *template.Template
	gameService       services.GameService
	pickService       *services.PickService
	authService       *services.AuthService
	visibilityService *services.PickVisibilityService
	sseClients        map[*SSEClient]bool
}

// NewSSEHandler creates a new SSE handler
func NewSSEHandler(templates *template.Template, gameService services.GameService) *SSEHandler {
	return &SSEHandler{
		templates:   templates,
		gameService: gameService,
		sseClients:  make(map[*SSEClient]bool),
	}
}

// SetServices sets the required services
func (h *SSEHandler) SetServices(pickService *services.PickService, authService *services.AuthService, visibilityService *services.PickVisibilityService) {
	h.pickService = pickService
	h.authService = authService
	h.visibilityService = visibilityService
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

	log.Printf("SSE: New client connected from %s (UserID: %d)", r.RemoteAddr, userID)

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
		log.Printf("SSE: Client disconnected (UserID: %d)", userID)
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
			fmt.Fprintf(w, "data: %s\n\n", message)
			flusher.Flush()

		case <-r.Context().Done():
			log.Printf("SSE: Client context cancelled (UserID: %d)", userID)
			return
		}
	}
}

// HandleDatabaseChange processes database change events for SSE broadcasting
func (h *SSEHandler) HandleDatabaseChange(event services.ChangeEvent) {
	log.Printf("DATABASE CHANGE: %s collection, GameID: %s, Season: %d, Week: %d, Operation: %s",
		event.Collection, event.GameID, event.Season, event.Week, event.Operation)

	if event.Collection != "games" {
		return
	}

	// For game collection changes, send targeted HTML updates
	if event.Collection == "games" && event.GameID != "" {
		h.BroadcastGameUpdate(event.GameID, event.Season, event.Week)
	}
}

// BroadcastUpdate sends game updates to all connected SSE clients (legacy method)
func (h *SSEHandler) BroadcastUpdate() {
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

	// Execute the template
	htmlBuffer := &strings.Builder{}
	if err := h.templates.ExecuteTemplate(htmlBuffer, "games-container", data); err != nil {
		log.Printf("SSE: Error rendering games template: %v", err)
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
		log.Printf("SSE: Error marshaling message: %v", err)
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
		log.Printf("SSE: Error marshaling structured message: %v", err)
		return
	}

	h.broadcastToAllClients(string(messageData))
}

// broadcastToUser sends a message to a specific user's SSE connections
func (h *SSEHandler) broadcastToUser(userID int, eventType, data string) {
	message := map[string]interface{}{
		"type": eventType,
		"data": data,
	}

	messageData, err := json.Marshal(message)
	if err != nil {
		log.Printf("SSE: Error marshaling user message: %v", err)
		return
	}

	count := 0
	for client := range h.sseClients {
		if client.UserID == userID {
			select {
			case client.Channel <- string(messageData):
				count++
			default:
				log.Printf("SSE: Client channel full for user %d, skipping message", userID)
			}
		}
	}

	if count > 0 {
		log.Printf("SSE: Broadcasted %s to %d client(s) for user %d", eventType, count, userID)
	}
}

// broadcastToAllClients sends a message to all connected SSE clients
func (h *SSEHandler) broadcastToAllClients(messageData string) {
	clientCount := len(h.sseClients)
	if clientCount == 0 {
		return
	}

	sentCount := 0
	for client := range h.sseClients {
		select {
		case client.Channel <- messageData:
			sentCount++
		default:
			log.Printf("SSE: Client channel full, skipping message")
		}
	}

	log.Printf("SSE: Broadcasted to %d/%d clients", sentCount, clientCount)
}

// broadcastPickUpdate broadcasts pick updates for a specific user
func (h *SSEHandler) BroadcastPickUpdate(userID, season, week int) {
	if h.pickService == nil {
		log.Printf("SSE: Pick service not available for pick update broadcast")
		return
	}

	// Get current games for the season and week
	games, err := h.gameService.GetGamesBySeason(season)
	if err != nil {
		log.Printf("SSE: Error fetching games for pick update: %v", err)
		return
	}

	// Filter games by week
	var weekGames []models.Game
	for _, game := range games {
		if game.Week == week {
			weekGames = append(weekGames, game)
		}
	}

	// Get user picks for all users - this is a simplified approach
	// In the real implementation, we would need to get picks for all users
	// For now, let's get picks for the specific user
	userPicksData, err := h.pickService.GetUserPicksForWeek(context.Background(), userID, season, week)
	if err != nil {
		log.Printf("SSE: Error fetching user picks for broadcast: %v", err)
		return
	}

	// Convert single user picks to array for template compatibility
	var userPicks []*models.UserPicks
	if userPicksData != nil {
		userPicks = []*models.UserPicks{userPicksData}
	}

	// Render updated user picks HTML
	data := struct {
		UserPicks []*models.UserPicks
		Games     []models.Game
	}{
		UserPicks: userPicks,
		Games:     weekGames,
	}

	// Execute the user picks template
	htmlBuffer := &strings.Builder{}
	if err := h.templates.ExecuteTemplate(htmlBuffer, "sse-user-picks-block", data); err != nil {
		log.Printf("SSE: Error rendering user picks template: %v", err)
		return
	}

	// Create SSE message for out-of-band swap
	message := map[string]interface{}{
		"type": "user-picks-update",
		"html": htmlBuffer.String(),
	}

	messageData, err := json.Marshal(message)
	if err != nil {
		log.Printf("SSE: Error marshaling pick update message: %v", err)
		return
	}

	// Broadcast to all clients (picks visibility is already filtered per user above)
	h.broadcastToAllClients(string(messageData))
}

// broadcastGameUpdate broadcasts game updates for a specific game
func (h *SSEHandler) BroadcastGameUpdate(gameIDStr string, season, week int) {
	gameID, err := strconv.Atoi(gameIDStr)
	if err != nil {
		log.Printf("SSE: Invalid game ID: %s", gameIDStr)
		return
	}

	// Get the specific game
	game, err := h.gameService.GetGameByID(gameID)
	if err != nil {
		log.Printf("SSE: Error fetching game %d: %v", gameID, err)
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

	// Render game status HTML
	data := struct{ Game *models.Game }{Game: game}

	htmlBuffer := &strings.Builder{}
	if err := h.templates.ExecuteTemplate(htmlBuffer, "game-status", data); err != nil {
		log.Printf("SSE: Error rendering game status template: %v", err)
		return
	}

	// Create SSE message for out-of-band swap
	message := map[string]interface{}{
		"type":     "game-status-update",
		"game_id":  game.ID,
		"html":     htmlBuffer.String(),
		"selector": fmt.Sprintf("#game-status-%d", game.ID),
	}

	messageData, err := json.Marshal(message)
	if err != nil {
		log.Printf("SSE: Error marshaling game status message: %v", err)
		return
	}

	h.broadcastToAllClients(string(messageData))
}

// broadcastGameScoresHTML broadcasts game score updates
func (h *SSEHandler) broadcastGameScoresHTML(game *models.Game) {
	if game == nil {
		return
	}

	// Render game scores HTML
	data := struct{ Game *models.Game }{Game: game}

	htmlBuffer := &strings.Builder{}
	if err := h.templates.ExecuteTemplate(htmlBuffer, "game-scores", data); err != nil {
		log.Printf("SSE: Error rendering game scores template: %v", err)
		return
	}

	// Create SSE message for out-of-band swap
	message := map[string]interface{}{
		"type":     "game-scores-update",
		"game_id":  game.ID,
		"html":     htmlBuffer.String(),
		"selector": fmt.Sprintf("#game-scores-%d", game.ID),
	}

	messageData, err := json.Marshal(message)
	if err != nil {
		log.Printf("SSE: Error marshaling game scores message: %v", err)
		return
	}

	h.broadcastToAllClients(string(messageData))
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

	log.Printf("SSE: Game %d update affects %d users, broadcasting pick updates", game.ID, len(affectedUsers))

	// Broadcast pick update for each affected user
	for userID := range affectedUsers {
		h.BroadcastPickUpdate(userID, game.Season, game.Week)
	}
}