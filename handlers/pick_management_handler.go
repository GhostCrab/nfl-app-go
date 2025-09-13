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
	
	// Sort games chronologically (same as game display logic)
	sort.Slice(games, func(i, j int) bool {
		// Primary sort: by game date/time
		if games[i].Date.Unix() != games[j].Date.Unix() {
			return games[i].Date.Before(games[j].Date)
		}
		// Secondary sort: alphabetically by home team name for same kickoff time
		return games[i].Home < games[j].Home
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
	
	// Build PickState structure expected by template: map[gameID]map[teamID]bool
	pickState := make(map[int]map[int]bool)
	for _, game := range games {
		gameState := make(map[int]bool)
		
		// Initialize all options as false
		gameState[1] = false  // away team
		gameState[2] = false  // home team  
		gameState[98] = false // under
		gameState[99] = false // over
		
		// Set existing picks to true
		if existingPick, exists := picksByGame[game.ID]; exists {
			// For Over/Under picks, use the TeamID directly (98/99)
			if existingPick.TeamID == 98 || existingPick.TeamID == 99 {
				gameState[existingPick.TeamID] = true
			} else {
				// For spread picks, convert ESPN team ID back to positional key (1=away, 2=home)
				awayTeamID := h.getTeamIDFromAbbreviation(game.Away)
				homeTeamID := h.getTeamIDFromAbbreviation(game.Home)
				
				if existingPick.TeamID == awayTeamID {
					gameState[1] = true // away team
				} else if existingPick.TeamID == homeTeamID {
					gameState[2] = true // home team
				}
			}
		}
		
		pickState[game.ID] = gameState
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
		PickState      map[int]map[int]bool // Proper structure for template
	}{
		User:           user,
		Games:          games,
		ExistingPicks:  picksByGame,
		CurrentSeason:  season,
		CurrentWeek:    week,
		CanSubmitPicks: canSubmitPicks,
		PickState:      pickState,
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
	
	// Parse picks from form (format: pick-{gameID}-{type} = "1")
	var picks []*models.Pick
	
	for fieldName, fieldValues := range r.Form {
		// Skip non-pick fields and fields without "pick-" prefix
		if !strings.HasPrefix(fieldName, "pick-") {
			continue
		}
		
		// Skip if no value provided or checkbox not checked
		if len(fieldValues) == 0 || fieldValues[0] != "1" {
			continue
		}
		
		// Parse field name: pick-{gameID}-{type}
		parts := strings.Split(fieldName, "-")
		if len(parts) != 3 {
			log.Printf("Invalid pick field format: %s", fieldName)
			continue
		}
		
		// Extract game ID
		gameID, err := strconv.Atoi(parts[1])
		if err != nil {
			log.Printf("Invalid game ID in field %s: %s", fieldName, parts[1])
			continue
		}
		
		// Validate that game exists and get game data
		game, exists := gameMap[gameID]
		if !exists {
			log.Printf("Pick submitted for non-existent game: %d", gameID)
			continue
		}
		
		// Extract team ID/type (away, home, 98, 99) - now with proper ESPN team IDs
		var teamID int
		switch parts[2] {
		case "away":
			teamID = h.getTeamIDFromAbbreviation(game.Away)
		case "home":
			teamID = h.getTeamIDFromAbbreviation(game.Home)
		case "98": // Under
			teamID = 98
		case "99": // Over
			teamID = 99
		default:
			log.Printf("Invalid pick type in field %s: %s", fieldName, parts[2])
			continue
		}
		
		// Determine pick type and team name based on team ID
		var pickType models.PickType
		var teamName string
		
		// Handle Over/Under picks (still use 98/99)
		if teamID == 98 {
			pickType = models.PickTypeOverUnder
			if game.HasOdds() {
				teamName = fmt.Sprintf("Under %.1f", game.Odds.OU)
			} else {
				teamName = "Under"
			}
		} else if teamID == 99 {
			pickType = models.PickTypeOverUnder
			if game.HasOdds() {
				teamName = fmt.Sprintf("Over %.1f", game.Odds.OU)
			} else {
				teamName = "Over"
			}
		} else {
			// Handle spread picks (now using actual ESPN team IDs)
			pickType = models.PickTypeSpread
			
			// Determine team name by comparing ESPN team ID with away/home teams
			awayTeamID := h.getTeamIDFromAbbreviation(game.Away)
			homeTeamID := h.getTeamIDFromAbbreviation(game.Home)
			
			if teamID == awayTeamID {
				teamName = game.Away
			} else if teamID == homeTeamID {
				teamName = game.Home
			} else {
				log.Printf("TeamID %d doesn't match away team %s (ID: %d) or home team %s (ID: %d) for game %d", 
					teamID, game.Away, awayTeamID, game.Home, homeTeamID, gameID)
				continue
			}
		}
		
		// Create pick with proper TeamName
		pick := &models.Pick{
			UserID:          user.ID,
			GameID:          gameID,
			TeamID:          teamID,
			PickType:        pickType,
			Season:          season,
			Week:            week,
			Result:          models.PickResultPending,
			TeamName:        teamName,
			GameDescription: fmt.Sprintf("%s @ %s", game.Away, game.Home),
		}
		
		// Debug logging for pick creation
		log.Printf("SUBMIT_PICKS DEBUG: Created pick - GameID=%d, TeamID=%d, TeamName='%s', PickType='%s', GameDescription='%s'", 
			pick.GameID, pick.TeamID, pick.TeamName, pick.PickType, pick.GameDescription)
		
		picks = append(picks, pick)
	}
	
	log.Printf("Processed %d picks from form submission", len(picks))
	
	// CRITICAL FIX: Use UpdateUserPicksForScheduledGames to preserve existing picks for completed games
	// instead of ReplaceUserPicksForWeek which deletes all picks for the week
	ctx := context.Background()
	if err := h.pickService.UpdateUserPicksForScheduledGames(ctx, user.ID, season, week, picks, gameMap); err != nil {
		log.Printf("Error updating picks for user %d: %v", user.ID, err)
		http.Error(w, "Error saving picks", http.StatusInternalServerError)
		return
	}
	
	log.Printf("Successfully saved %d picks for user %d", len(picks), user.ID)
	
	// Broadcast pick update via SSE
	h.broadcastPickUpdate(user.ID, season, week)
	
	// For HTMX requests, return success message instead of redirect
	if r.Header.Get("HX-Request") == "true" {
		w.Header().Set("Content-Type", "text/html")
		fmt.Fprintf(w, `<div class="success-message">✓ Successfully saved %d picks!</div>`, len(picks))
		return
	}
	
	// For regular requests, redirect back to pick picker with success message  
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

// getTeamIDFromAbbreviation maps team abbreviation to ESPN team ID
func (h *PickManagementHandler) getTeamIDFromAbbreviation(abbr string) int {
	// ESPN team ID mapping (matches the one used in PickService)
	teamMap := map[string]int{
		"ARI": 22, "ATL": 1,  "BAL": 33, "BUF": 2,  "CAR": 29, "CHI": 3,
		"CIN": 4,  "CLE": 5,  "DAL": 6,  "DEN": 7,  "DET": 8,  "GB":  9,
		"HOU": 34, "IND": 11, "JAX": 30, "KC":  12, "LV":  13, "LAC": 24,
		"LAR": 14, "MIA": 15, "MIN": 16, "NE":  17, "NO":  18, "NYG": 19,
		"NYJ": 20, "PHI": 21, "PIT": 23, "SF":  25, "SEA": 26, "TB":  27,
		"TEN": 10, "WSH": 28, "WAS": 28, // Handle both WSH and WAS for Washington
	}
	
	if id, exists := teamMap[abbr]; exists {
		return id
	}
	
	log.Printf("Unknown team abbreviation: %s", abbr)
	return 0 // Fallback for unknown teams
}