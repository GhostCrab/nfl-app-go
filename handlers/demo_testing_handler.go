package handlers

import (
	"context"
	"fmt"
	"html/template"
	"log"
	"math/rand"
	"net/http"
	"nfl-app-go/database"
	"nfl-app-go/models"
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
	parlayRepo    *database.MongoParlayRepository
}

// NewDemoTestingHandler creates a new demo testing handler
func NewDemoTestingHandler(
	templates *template.Template,
	gameService services.GameService,
	sseHandler *SSEHandler,
	parlayService *services.ParlayService,
	parlayRepo *database.MongoParlayRepository,
) *DemoTestingHandler {
	return &DemoTestingHandler{
		templates:     templates,
		gameService:   gameService,
		sseHandler:    sseHandler,
		parlayService: parlayService,
		parlayRepo:    parlayRepo,
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

	log.Printf("Modifying parlay scores database for season %d, week %d", currentSeason, currentWeek)

	if h.parlayService == nil {
		log.Println("Parlay service not available")
		http.Error(w, "Parlay service not available", http.StatusInternalServerError)
		return
	}

	// Generate random dummy data for users 1-6 (assuming these users exist)
	ctx := context.Background()
	updatedUsers := []int{}

	for userID := 1; userID <= 6; userID++ {
		// Create random parlay score
		parlayScore := &models.ParlayScore{
			UserID:              userID,
			Season:              currentSeason,
			Week:                currentWeek,
			RegularPoints:       rand.Intn(5),    // 0-4 points
			BonusThursdayPoints: rand.Intn(3),    // 0-2 points
			BonusFridayPoints:   rand.Intn(3),    // 0-2 points
			CreatedAt:           time.Now(),
			UpdatedAt:           time.Now(),
		}

		// Calculate total points
		parlayScore.CalculateTotal()

		// Insert/update the score - this should trigger MongoDB change stream
		err := h.parlayRepo.UpsertParlayScore(ctx, parlayScore)
		if err != nil {
			log.Printf("Failed to update parlay score for user %d: %v", userID, err)
			continue
		}

		updatedUsers = append(updatedUsers, userID)
		log.Printf("Updated parlay score for user %d: %d total points", userID, parlayScore.TotalPoints)
	}

	// Return success response with details
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	response := fmt.Sprintf(`{"success": true, "message": "Database parlay scores updated for %d users", "season": %d, "week": %d, "count": %d}`,
		len(updatedUsers), currentSeason, currentWeek, len(updatedUsers))
	w.Write([]byte(response))
}