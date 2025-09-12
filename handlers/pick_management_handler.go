package handlers

import (
	"context"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"nfl-app-go/middleware"
	"nfl-app-go/models"
	"nfl-app-go/services"
	"sort"
	"strconv"
	"strings"
	"time"
)

// PickManagementHandler handles pick submission and management functionality
// This handler is focused on user pick operations and the pick selection interface
type PickManagementHandler struct {
	templates         *template.Template
	gameService       services.GameService
	pickService       *services.PickService
	authService       *services.AuthService
	visibilityService *services.PickVisibilityService
	sseHandler        *SSEHandler // Reference to SSE handler for broadcasts
}

// NewPickManagementHandler creates a new pick management handler
func NewPickManagementHandler(
	templates *template.Template,
	gameService services.GameService,
	pickService *services.PickService,
	authService *services.AuthService,
	visibilityService *services.PickVisibilityService,
	sseHandler *SSEHandler,
) *PickManagementHandler {
	return &PickManagementHandler{
		templates:         templates,
		gameService:       gameService,
		pickService:       pickService,
		authService:       authService,
		visibilityService: visibilityService,
		sseHandler:        sseHandler,
	}
}

// ShowPickPicker displays the pick selection interface for a specific week
// This endpoint renders the interactive pick selection form with current games
func (h *PickManagementHandler) ShowPickPicker(w http.ResponseWriter, r *http.Request) {
	log.Println("ShowPickPicker called")
	
	// Extract user from context (set by auth middleware)
	user, ok := r.Context().Value(middleware.UserKey).(*models.User)
	if !ok {
		log.Println("No user in context for pick picker")
		http.Error(w, "Authentication required", http.StatusUnauthorized)
		return
	}
	
	log.Printf("ShowPickPicker for user %d (%s)", user.ID, user.Email)
	
	// Parse query parameters
	seasonStr := r.URL.Query().Get("season")
	weekStr := r.URL.Query().Get("week")
	
	var season, week int
	var err error
	
	// Default to current season
	if seasonStr == "" {
		season = time.Now().Year()
		if time.Now().Month() < time.March {
			season--
		}
	} else {
		season, err = strconv.Atoi(seasonStr)
		if err != nil {
			log.Printf("Invalid season parameter: %s", seasonStr)
			http.Error(w, "Invalid season parameter", http.StatusBadRequest)
			return
		}
	}
	
	// Determine current week if not specified
	if weekStr == "" {
		allGames, err := h.gameService.GetGamesBySeason(season)
		if err != nil {
			log.Printf("Error getting games for current week: %v", err)
			http.Error(w, "Error retrieving games", http.StatusInternalServerError)
			return
		}
		week = h.getCurrentWeek(allGames)
	} else {
		week, err = strconv.Atoi(weekStr)
		if err != nil {
			log.Printf("Invalid week parameter: %s", weekStr)
			http.Error(w, "Invalid week parameter", http.StatusBadRequest)
			return
		}
	}
	
	log.Printf("Loading pick picker for season %d, week %d", season, week)
	
	// Get games for the season and filter by week
	allGames, err := h.gameService.GetGamesBySeason(season)
	if err != nil {
		log.Printf("Error getting games for pick picker: %v", err)
		http.Error(w, "Error retrieving games", http.StatusInternalServerError)
		return
	}
	
	// Filter games by week
	games := make([]models.Game, 0)
	for _, game := range allGames {
		if game.Week == week {
			games = append(games, game)
		}
	}
	
	// Sort games chronologically
	sort.Slice(games, func(i, j int) bool {
		return games[i].Date.Before(games[j].Date)
	})
	
	// Get user's existing picks for this week
	ctx := context.Background()
	userPicksData, err := h.pickService.GetUserPicksForWeek(ctx, user.ID, season, week)
	if err != nil {
		log.Printf("Error getting existing picks: %v", err)
		// Continue without existing picks rather than failing
		userPicksData = &models.UserPicks{Picks: []models.Pick{}}
	}
	
	// Convert to []*models.Pick for compatibility
	existingPicks := make([]*models.Pick, len(userPicksData.Picks))
	for i := range userPicksData.Picks {
		existingPicks[i] = &userPicksData.Picks[i]
	}
	
	// Create a map for quick lookup of existing picks by game ID
	picksByGame := make(map[int]*models.Pick)
	for _, pick := range existingPicks {
		picksByGame[pick.GameID] = pick
	}
	
	// Check if picks can still be submitted (before kickoff)
	canSubmitPicks := h.canSubmitPicksForWeek(games)
	
	// Prepare template data
	data := struct {
		User           *models.User
		Games          []models.Game
		ExistingPicks  map[int]*models.Pick
		CurrentSeason  int
		CurrentWeek    int
		CanSubmitPicks bool
		PickState      map[int]*models.Pick // Alias for ExistingPicks to match template expectations
	}{
		User:           user,
		Games:          games,
		ExistingPicks:  picksByGame,
		CurrentSeason:  season,
		CurrentWeek:    week,
		CanSubmitPicks: canSubmitPicks,
		PickState:      picksByGame,
	}
	
	// Render the pick picker template
	if err := h.templates.ExecuteTemplate(w, "pick-picker", data); err != nil {
		log.Printf("Error rendering pick picker template: %v", err)
		http.Error(w, "Error rendering page", http.StatusInternalServerError)
	}
}

// SubmitPicks handles pick submission from the pick picker form
// This endpoint processes user picks and saves them to the database
func (h *PickManagementHandler) SubmitPicks(w http.ResponseWriter, r *http.Request) {
	log.Println("SubmitPicks called")
	
	// Extract user from context
	user, ok := r.Context().Value(middleware.UserKey).(*models.User)
	if !ok {
		log.Println("No user in context for pick submission")
		http.Error(w, "Authentication required", http.StatusUnauthorized)
		return
	}
	
	// Parse form data
	if err := r.ParseForm(); err != nil {
		log.Printf("Error parsing form data: %v", err)
		http.Error(w, "Invalid form data", http.StatusBadRequest)
		return
	}
	
	// Extract season and week
	seasonStr := r.FormValue("season")
	weekStr := r.FormValue("week")
	
	season, err := strconv.Atoi(seasonStr)
	if err != nil {
		log.Printf("Invalid season in form: %s", seasonStr)
		http.Error(w, "Invalid season", http.StatusBadRequest)
		return
	}
	
	week, err := strconv.Atoi(weekStr)
	if err != nil {
		log.Printf("Invalid week in form: %s", weekStr)
		http.Error(w, "Invalid week", http.StatusBadRequest)
		return
	}
	
	log.Printf("Processing picks submission for user %d, season %d, week %d", user.ID, season, week)
	
	// Get games for validation
	allGames, err := h.gameService.GetGamesBySeason(season)
	if err != nil {
		log.Printf("Error getting games for pick validation: %v", err)
		http.Error(w, "Error validating picks", http.StatusInternalServerError)
		return
	}
	
	// Filter games by week
	games := make([]models.Game, 0)
	for _, game := range allGames {
		if game.Week == week {
			games = append(games, game)
		}
	}
	
	// Check if picks can still be submitted
	if !h.canSubmitPicksForWeek(games) {
		log.Printf("Attempted to submit picks after deadline for week %d", week)
		http.Error(w, "Pick submission deadline has passed", http.StatusForbidden)
		return
	}
	
	// Create game lookup map
	gameMap := make(map[int]models.Game)
	for _, game := range games {
		gameMap[game.ID] = game
	}
	
	// Parse picks from form
	var picks []*models.Pick
	
	for gameIDStr, teamIDStr := range r.Form {
		if !strings.HasPrefix(gameIDStr, "pick_") {
			continue // Skip non-pick fields
		}
		
		// Extract game ID from form field name (pick_12345 -> 12345)
		gameIDStr = strings.TrimPrefix(gameIDStr, "pick_")
		gameID, err := strconv.Atoi(gameIDStr)
		if err != nil {
			log.Printf("Invalid game ID in form: %s", gameIDStr)
			continue
		}
		
		// Parse team ID
		if len(teamIDStr) == 0 {
			continue // Skip empty picks
		}
		
		teamID, err := strconv.Atoi(teamIDStr[0])
		if err != nil {
			log.Printf("Invalid team ID in form: %s", teamIDStr[0])
			continue
		}
		
		// Validate that game exists
		_, exists := gameMap[gameID]
		if !exists {
			log.Printf("Pick submitted for non-existent game: %d", gameID)
			continue
		}
		
		// Determine pick type based on team ID
		var pickType models.PickType
		switch teamID {
		case 98: // Over
			pickType = models.PickTypeOverUnder
		case 99: // Under  
			pickType = models.PickTypeOverUnder
		default: // Spread pick
			pickType = models.PickTypeSpread
		}
		
		// Create pick
		pick := &models.Pick{
			UserID:   user.ID,
			GameID:   gameID,
			TeamID:   teamID,
			PickType: pickType,
			Season:   season,
			Week:     week,
			Result:   models.PickResultPending,
		}
		
		picks = append(picks, pick)
	}
	
	log.Printf("Processed %d picks from form submission", len(picks))
	
	// Replace all picks for this week using the service method
	ctx := context.Background()
	if err := h.pickService.ReplaceUserPicksForWeek(ctx, user.ID, season, week, picks); err != nil {
		log.Printf("Error replacing picks for user %d: %v", user.ID, err)
		http.Error(w, "Error saving picks", http.StatusInternalServerError)
		return
	}
	
	log.Printf("Successfully saved %d picks for user %d", len(picks), user.ID)
	
	// Broadcast pick update via SSE
	h.broadcastPickUpdate(user.ID, season, week)
	
	// Redirect back to pick picker with success message
	redirectURL := fmt.Sprintf("/pick-picker?season=%d&week=%d&success=1", season, week)
	http.Redirect(w, r, redirectURL, http.StatusSeeOther)
}

// broadcastPickUpdate sends SSE notifications about pick updates
func (h *PickManagementHandler) broadcastPickUpdate(userID, season, week int) {
	if h.sseHandler == nil {
		return
	}
	
	// Create pick update event
	eventData := fmt.Sprintf(`{
		"user_id": %d,
		"season": %d,
		"week": %d,
		"timestamp": "%s"
	}`, userID, season, week, time.Now().Format(time.RFC3339))
	
	// Broadcast to all connected clients
	h.sseHandler.BroadcastStructuredUpdate("pick_update", eventData)
}

// canSubmitPicksForWeek checks if picks can still be submitted for the given games
func (h *PickManagementHandler) canSubmitPicksForWeek(games []models.Game) bool {
	now := time.Now()
	
	// Allow pick submission if any game hasn't started yet
	for _, game := range games {
		if game.Date.After(now) {
			return true
		}
	}
	
	return false
}

// getCurrentWeek determines the current NFL week based on game dates
func (h *PickManagementHandler) getCurrentWeek(games []models.Game) int {
	if len(games) == 0 {
		return 1
	}
	
	// Group games by week
	weekGames := make(map[int][]models.Game)
	for _, game := range games {
		weekGames[game.Week] = append(weekGames[game.Week], game)
	}
	
	// Find current week based on game timing
	for week := 1; week <= 18; week++ {
		gamesThisWeek := weekGames[week]
		if len(gamesThisWeek) == 0 {
			continue
		}
		
		// Check if any games this week are not completed
		for _, game := range gamesThisWeek {
			if !game.IsCompleted() {
				return week
			}
		}
	}
	
	return 1 // Fallback
}