package handlers

import (
	"html/template"
	"log"
	"net/http"
	"nfl-app-go/services"
	"strconv"
	"time"
)

// DemoTestingHandler handles demo and testing functionality
// This handler provides game simulation and testing endpoints
type DemoTestingHandler struct {
	templates   *template.Template
	gameService services.GameService
	sseHandler  *SSEHandler // Reference to SSE handler for broadcasts
}

// NewDemoTestingHandler creates a new demo testing handler
func NewDemoTestingHandler(
	templates *template.Template,
	gameService services.GameService,
	sseHandler *SSEHandler,
) *DemoTestingHandler {
	return &DemoTestingHandler{
		templates:   templates,
		gameService: gameService,
		sseHandler:  sseHandler,
	}
}

// TestGameUpdate handles testing game update functionality
// This endpoint triggers simulated game updates for demonstration purposes
func (h *DemoTestingHandler) TestGameUpdate(w http.ResponseWriter, r *http.Request) {
	log.Println("TestGameUpdate called")
	
	// Parse game ID from query parameter
	gameIDStr := r.URL.Query().Get("game_id")
	if gameIDStr == "" {
		http.Error(w, "Missing game_id parameter", http.StatusBadRequest)
		return
	}
	
	gameID, err := strconv.Atoi(gameIDStr)
	if err != nil {
		log.Printf("Invalid game ID: %s", gameIDStr)
		http.Error(w, "Invalid game_id parameter", http.StatusBadRequest)
		return
	}
	
	// Determine update type from query parameter
	updateType := r.URL.Query().Get("type")
	if updateType == "" {
		updateType = "score" // Default to score update
	}
	
	log.Printf("Triggering %s update for game %d", updateType, gameID)
	
	// Perform the appropriate simulation based on type
	switch updateType {
	case "score":
		h.simulateScoreUpdate(gameID)
	case "state":
		h.simulateStateChange(gameID)
	case "live":
		h.simulateLiveUpdate(gameID)
	default:
		log.Printf("Unknown update type: %s, defaulting to score", updateType)
		h.simulateScoreUpdate(gameID)
	}
	
	// Return success response
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(`{"success": true, "message": "Update triggered"}`))
}

// simulateScoreUpdate simulates a score change for demo purposes
func (h *DemoTestingHandler) simulateScoreUpdate(gameID int) {
	log.Printf("Simulating score update for game %d", gameID)
	
	if h.sseHandler != nil {
		// Convert gameID to string and use current season/week
		gameIDStr := strconv.Itoa(gameID)
		currentSeason := time.Now().Year()
		currentWeek := 1 // Default week for demo
		
		h.sseHandler.BroadcastGameUpdate(gameIDStr, currentSeason, currentWeek)
	}
}

// simulateStateChange simulates a game state change for demo purposes
func (h *DemoTestingHandler) simulateStateChange(gameID int) {
	log.Printf("Simulating state change for game %d", gameID)
	
	if h.sseHandler != nil {
		// Convert gameID to string and use current season/week
		gameIDStr := strconv.Itoa(gameID)
		currentSeason := time.Now().Year()
		currentWeek := 1 // Default week for demo
		
		h.sseHandler.BroadcastGameUpdate(gameIDStr, currentSeason, currentWeek)
	}
}

// simulateLiveUpdate simulates comprehensive live game updates
func (h *DemoTestingHandler) simulateLiveUpdate(gameID int) {
	log.Printf("Simulating live update for game %d", gameID)
	
	if h.sseHandler != nil {
		// Convert gameID to string and use current season/week
		gameIDStr := strconv.Itoa(gameID)
		currentSeason := time.Now().Year()
		currentWeek := 1 // Default week for demo
		
		h.sseHandler.BroadcastGameUpdate(gameIDStr, currentSeason, currentWeek)
	}
}