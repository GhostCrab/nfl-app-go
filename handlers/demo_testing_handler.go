package handlers

import (
	"fmt"
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
	templates     *template.Template
	gameService   services.GameService
	sseHandler    *SSEHandler // Reference to SSE handler for broadcasts
	parlayService *services.ParlayService
}

// NewDemoTestingHandler creates a new demo testing handler
func NewDemoTestingHandler(
	templates *template.Template,
	gameService services.GameService,
	sseHandler *SSEHandler,
	parlayService *services.ParlayService,
) *DemoTestingHandler {
	return &DemoTestingHandler{
		templates:     templates,
		gameService:   gameService,
		sseHandler:    sseHandler,
		parlayService: parlayService,
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

// TestClubScoreUpdate handles testing club score update functionality
// This endpoint triggers simulated club score updates for demonstration purposes
func (h *DemoTestingHandler) TestClubScoreUpdate(w http.ResponseWriter, r *http.Request) {
	log.Println("TestClubScoreUpdate called")

	// Get season and week from query parameters, with defaults
	seasonStr := r.URL.Query().Get("season")
	weekStr := r.URL.Query().Get("week")

	currentSeason := time.Now().Year()
	currentWeek := 1

	if seasonStr != "" {
		if season, err := strconv.Atoi(seasonStr); err == nil {
			currentSeason = season
		}
	}

	if weekStr != "" {
		if week, err := strconv.Atoi(weekStr); err == nil {
			currentWeek = week
		}
	}

	log.Printf("Triggering club score update for season %d, week %d", currentSeason, currentWeek)

	// Simulate parlay score update broadcast
	if h.sseHandler != nil {
		h.sseHandler.BroadcastParlayScoreUpdate(currentSeason, currentWeek)
	}

	// Return success response
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(`{"success": true, "message": "Club score update triggered"}`))
}

// TestDatabaseParlayUpdate handles testing database parlay score modifications
// This endpoint modifies the parlay_scores collection to trigger real change stream events
func (h *DemoTestingHandler) TestDatabaseParlayUpdate(w http.ResponseWriter, r *http.Request) {
	log.Println("TestDatabaseParlayUpdate called")

	// Get season and week from query parameters, with defaults
	seasonStr := r.URL.Query().Get("season")
	weekStr := r.URL.Query().Get("week")

	currentSeason := time.Now().Year()
	currentWeek := 1

	if seasonStr != "" {
		if season, err := strconv.Atoi(seasonStr); err == nil {
			currentSeason = season
		}
	}

	if weekStr != "" {
		if week, err := strconv.Atoi(weekStr); err == nil {
			currentWeek = week
		}
	}

	log.Printf("DEPRECATED: TestDatabaseParlayUpdate called for season %d, week %d", currentSeason, currentWeek)
	log.Printf("This endpoint is deprecated as parlay scores are now calculated in-memory, not stored in database")

	// Since parlay scores are now calculated in-memory from picks and games,
	// trigger a parlay score recalculation instead
	if h.sseHandler != nil {
		h.sseHandler.BroadcastParlayScoreUpdate(currentSeason, currentWeek)
		log.Printf("Triggered parlay score broadcast for season %d, week %d", currentSeason, currentWeek)
	}

	// Return deprecation notice
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	response := fmt.Sprintf(`{"success": true, "message": "DEPRECATED: Database parlay scores are no longer used. Triggered in-memory score broadcast instead.", "season": %d, "week": %d}`,
		currentSeason, currentWeek)
	w.Write([]byte(response))
}