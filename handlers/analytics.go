package handlers

import (
	"context"
	"encoding/json"
	"html/template"
	"net/http"
	"nfl-app-go/logging"
	"nfl-app-go/middleware"
	"nfl-app-go/models"
	"nfl-app-go/services"
	"sort"
	"strconv"
	"strings"
)

// AnalyticsHandler handles analytics-related HTTP requests
type AnalyticsHandler struct {
	templates       *template.Template
	gameService     services.GameService
	pickService     *services.PickService
	userService     services.UserService
	teamService     services.TeamService
}

// NewAnalyticsHandler creates a new analytics handler
func NewAnalyticsHandler(templates *template.Template, gameService services.GameService, pickService *services.PickService, userService services.UserService, teamService services.TeamService) *AnalyticsHandler {
	return &AnalyticsHandler{
		templates:   templates,
		gameService: gameService,
		pickService: pickService,
		userService: userService,
		teamService: teamService,
	}
}

// Analytics page data structures
type UserStats struct {
	UserID   int    `json:"user_id"`
	UserName string `json:"user_name"`
	
	// Parlay scoring
	TotalScore    int     `json:"total_score"`
	WeeklyScores  []int   `json:"weekly_scores"`  // Scores by week
	
	// Pick performance
	ATSRecord     Record  `json:"ats_record"`     // Against the spread
	OURecord      Record  `json:"ou_record"`      // Over/under
	TotalRecord   Record  `json:"total_record"`   // Combined
	
	// Point differentials
	ATSPoints     int     `json:"ats_points"`     // Point differential for ATS picks
	OUPoints      int     `json:"ou_points"`      // Point differential for OU picks
	TotalPoints   int     `json:"total_points"`   // Combined point differential
	
	// Team preferences
	MostPickedATS string  `json:"most_picked_ats"`
	MostPickedOU  string  `json:"most_picked_ou"`
	MostPickedTotal string `json:"most_picked_total"`
	
	// Performance by team
	TeamStats     map[string]TeamPickStats `json:"team_stats"`
}

type Record struct {
	Wins   int     `json:"wins"`
	Losses int     `json:"losses"`
	Pushes int     `json:"pushes"`
	Total  int     `json:"total"`
	WinPct float64 `json:"win_pct"`
}

type TeamPickStats struct {
	ATSRecord Record `json:"ats_record"`
	OURecord  Record `json:"ou_record"`
	Total     int    `json:"total"`
}

type TeamLeagueStats struct {
	TeamAbbr    string  `json:"team_abbr"`
	TeamName    string  `json:"team_name"`
	SURecord    Record  `json:"su_record"`      // Straight up record
	ATSRecord   Record  `json:"ats_record"`     // Against the spread
	OURecord    Record  `json:"ou_record"`      // Over/under when this team plays
	UpsetRecord Record  `json:"upset_record"`   // Performance in upset situations (5+ point spreads)
}

type LeagueStats struct {
	FavoredATS Record `json:"favored_ats"`  // How favorites perform ATS
	HomeATS    Record `json:"home_ats"`     // How home teams perform ATS
	FavoredSU  Record `json:"favored_su"`   // How favorites perform straight up
	UpsetSU    Record `json:"upset_su"`     // Upset frequency (5+ point spreads)
	OverUnder  Record `json:"over_under"`   // Over/under results
	ExtremeOVR Record `json:"extreme_ovr"`  // O/U for games with total >= 50
	ExtremeUND Record `json:"extreme_und"`  // O/U for games with total <= 40
}

type AnalyticsData struct {
	Season       int               `json:"season"`
	Week         *int              `json:"week,omitempty"`        // nil for season-wide
	AllSeasons   bool              `json:"all_seasons"`
	
	// User statistics
	UserStats    []UserStats       `json:"user_stats"`
	
	// NFL team performance
	TeamStats    []TeamLeagueStats `json:"team_stats"`
	LeagueStats  LeagueStats       `json:"league_stats"`
	
	// Available seasons for selector
	AvailableSeasons []int         `json:"available_seasons"`
}

// ShowAnalytics displays the analytics page
func (h *AnalyticsHandler) ShowAnalytics(w http.ResponseWriter, r *http.Request) {
	logger := logging.WithPrefix("Analytics")
	logger.Debugf("HTTP: %s %s from %s", r.Method, r.URL.Path, r.RemoteAddr)
	
	// Get user from context
	user := middleware.GetUserFromContext(r)
	
	// Parse query parameters
	seasonStr := r.URL.Query().Get("season")
	weekStr := r.URL.Query().Get("week")
	allSeasonsStr := r.URL.Query().Get("all_seasons")
	
	// Default to current season
	season := 2025
	if seasonStr != "" {
		if parsedSeason, err := strconv.Atoi(seasonStr); err == nil {
			season = parsedSeason
		}
	}
	
	var week *int
	if weekStr != "" {
		if parsedWeek, err := strconv.Atoi(weekStr); err == nil {
			week = &parsedWeek
		}
	}
	
	allSeasons := strings.ToLower(allSeasonsStr) == "true"
	
	// Get analytics data
	analyticsData, err := h.GetAnalyticsData(r.Context(), season, week, allSeasons)
	if err != nil {
		logger.Errorf("Error getting analytics data: %v", err)
		http.Error(w, "Error loading analytics data", http.StatusInternalServerError)
		return
	}
	
	// Template data
	data := struct {
		Title     string
		User      *models.User
		Analytics AnalyticsData
		Season    int
		Week      *int
	}{
		Title:     "Analytics",
		User:      user,
		Analytics: *analyticsData,
		Season:    season,
		Week:      week,
	}
	
	// Check if this is an HTMX request (partial update)
	if r.Header.Get("HX-Request") == "true" {
		// Return only the analytics content for HTMX updates
		err = h.templates.ExecuteTemplate(w, "analytics-content", data)
		if err != nil {
			logger.Errorf("Template error (HTMX): %v", err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	} else {
		// Return the full page
		err = h.templates.ExecuteTemplate(w, "analytics_htmx.html", data)
		if err != nil {
			logger.Errorf("Template error: %v", err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}
	
	logger.Debugf("Successfully served %s %s", r.Method, r.URL.Path)
}

// GetAnalyticsData retrieves and processes analytics data
func (h *AnalyticsHandler) GetAnalyticsData(ctx context.Context, season int, week *int, allSeasons bool) (*AnalyticsData, error) {
	logger := logging.WithPrefix("Analytics")
	logger.Debugf("Getting data for season=%d, week=%v, allSeasons=%t", season, week, allSeasons)
	
	// Get games
	var games []models.Game
	var err error
	
	if allSeasons {
		// Get games from all seasons
		// This would need to be implemented in the game service
		games, err = h.gameService.GetGames()
	} else if gameServiceWithSeason, ok := h.gameService.(interface{ GetGamesBySeason(int) ([]models.Game, error) }); ok {
		games, err = gameServiceWithSeason.GetGamesBySeason(season)
		if err != nil {
			return nil, err
		}
		
		// Filter by week if specified
		if week != nil {
			filteredGames := make([]models.Game, 0)
			for _, game := range games {
				if game.Week == *week {
					filteredGames = append(filteredGames, game)
				}
			}
			games = filteredGames
		}
	} else {
		games, err = h.gameService.GetGames()
	}
	
	if err != nil {
		return nil, err
	}
	
	logger.Debugf("Retrieved %d games for season %d", len(games), season)
	
	// Get picks for the same scope
	picks, err := h.pickService.GetPicksForAnalytics(ctx, season, week, allSeasons)
	if err != nil {
		return nil, err
	}
	
	logger.Debugf("Retrieved %d picks for season %d", len(picks), season)
	
	// Get users
	users, err := h.userService.GetAllUsers()
	if err != nil {
		return nil, err
	}
	
	// Process analytics data
	analyticsData := &AnalyticsData{
		Season:     season,
		Week:       week,
		AllSeasons: allSeasons,
		UserStats:  h.calculateUserStats(users, picks, games),
		TeamStats:  h.calculateTeamStats(games),
		LeagueStats: h.calculateLeagueStats(games),
		AvailableSeasons: []int{2023, 2024, 2025}, // Could be dynamic
	}
	
	return analyticsData, nil
}

// GetAnalyticsAPI returns analytics data as JSON
func (h *AnalyticsHandler) GetAnalyticsAPI(w http.ResponseWriter, r *http.Request) {
	logger := logging.WithPrefix("Analytics")
	logger.Debugf("Analytics API request from %s", r.RemoteAddr)
	
	// Parse parameters (same as ShowAnalytics)
	seasonStr := r.URL.Query().Get("season")
	weekStr := r.URL.Query().Get("week")
	allSeasonsStr := r.URL.Query().Get("all_seasons")
	
	season := 2025
	if seasonStr != "" {
		if parsedSeason, err := strconv.Atoi(seasonStr); err == nil {
			season = parsedSeason
		}
	}
	
	var week *int
	if weekStr != "" {
		if parsedWeek, err := strconv.Atoi(weekStr); err == nil {
			week = &parsedWeek
		}
	}
	
	allSeasons := strings.ToLower(allSeasonsStr) == "true"
	
	// Get analytics data
	analyticsData, err := h.GetAnalyticsData(r.Context(), season, week, allSeasons)
	if err != nil {
		logger.Errorf("API error getting analytics data: %v", err)
		http.Error(w, "Error loading analytics data", http.StatusInternalServerError)
		return
	}
	
	// Return JSON
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(analyticsData)
	
	logger.Debugf("Analytics API data returned for season %d", season)
}

// Helper functions for calculations - using proven dashboard approach
func (h *AnalyticsHandler) calculateUserStats(users []models.User, picks []models.Pick, games []models.Game) []UserStats {
	logger := logging.WithPrefix("Analytics")
	// Instead of manually processing picks, use the proven GetAllUserPicksForWeek approach
	// Get the pick service from analytics handler
	if h.pickService == nil {
		logger.Warn("No pick service available")
		return []UserStats{}
	}
	
	userStatsMap := make(map[int]*UserStats)
	
	// Initialize user stats
	for _, user := range users {
		userStatsMap[user.ID] = &UserStats{
			UserID:   user.ID,
			UserName: user.Name,
			TeamStats: make(map[string]TeamPickStats),
		}
	}
	
	// Determine which weeks to process based on available games
	weekSet := make(map[int]bool)
	seasonSet := make(map[int]bool)
	for _, game := range games {
		weekSet[game.Week] = true
		seasonSet[game.Season] = true
	}
	
	logger.Debugf("Processing weeks %v for seasons %v", getKeysFromIntMap(weekSet), getKeysFromIntMap(seasonSet))
	
	totalRecordsProcessed := 0
	
	// Process each season and get the FINAL week's cumulative totals
	for season := range seasonSet {
		// Get final week for this season to get cumulative totals
		maxWeek := 0
		for week := range weekSet {
			if week > maxWeek {
				maxWeek = week
			}
		}
		
		if maxWeek == 0 {
			continue
		}
		
		// Use the same method the dashboard uses - but only for the final week to get cumulative totals
		userPicks, err := h.pickService.GetAllUserPicksForWeek(context.Background(), season, maxWeek)
		if err != nil {
			logger.Errorf("Error getting picks for season %d week %d: %v", season, maxWeek, err)
			continue
		}
		
		// Get the cumulative totals from the final week (ParlayPoints is cumulative)
		for _, userPicksData := range userPicks {
			if userStats, exists := userStatsMap[userPicksData.UserID]; exists {
				record := userPicksData.Record
				
				// ParlayPoints is cumulative, so just use it from final week
				userStats.TotalScore = record.ParlayPoints
				
				// Now separate ATS vs O/U picks by examining actual picks across all weeks
				for week := range weekSet {
					weekUserPicks, err := h.pickService.GetUserPicksForWeek(context.Background(), userPicksData.UserID, season, week)
					if err != nil {
						continue
					}
					
					// Count ATS vs O/U picks from actual pick data
					for _, pick := range weekUserPicks.Picks {
						if pick.Result == models.PickResultPending {
							continue // Skip pending picks
						}
						
						if pick.PickType == models.PickTypeSpread {
							// ATS pick
							switch pick.Result {
							case models.PickResultWin:
								userStats.ATSRecord.Wins++
							case models.PickResultLoss:
								userStats.ATSRecord.Losses++
							case models.PickResultPush:
								userStats.ATSRecord.Pushes++
							}
							userStats.ATSRecord.Total++
						} else if pick.PickType == models.PickTypeOverUnder {
							// O/U pick
							switch pick.Result {
							case models.PickResultWin:
								userStats.OURecord.Wins++
							case models.PickResultLoss:
								userStats.OURecord.Losses++
							case models.PickResultPush:
								userStats.OURecord.Pushes++
							}
							userStats.OURecord.Total++
						}
					}
				}
				
				totalRecordsProcessed++
				logger.Debugf("User %s season %d: ATS %d-%d-%d, OU %d-%d-%d, Score: %d",
					userPicksData.UserName, season,
					userStats.ATSRecord.Wins, userStats.ATSRecord.Losses, userStats.ATSRecord.Pushes,
					userStats.OURecord.Wins, userStats.OURecord.Losses, userStats.OURecord.Pushes,
					userStats.TotalScore)
			}
		}
	}
	
	logger.Debugf("Processed %d user-week records using dashboard approach", totalRecordsProcessed)
	
	// Convert map to slice and finalize calculations
	var result []UserStats
	for _, stats := range userStatsMap {
		// Calculate win percentages
		if stats.ATSRecord.Total > 0 {
			stats.ATSRecord.WinPct = float64(stats.ATSRecord.Wins) / float64(stats.ATSRecord.Total)
		}
		if stats.OURecord.Total > 0 {
			stats.OURecord.WinPct = float64(stats.OURecord.Wins) / float64(stats.OURecord.Total)
		}
		
		// Calculate combined totals
		stats.TotalRecord.Wins = stats.ATSRecord.Wins + stats.OURecord.Wins
		stats.TotalRecord.Losses = stats.ATSRecord.Losses + stats.OURecord.Losses
		stats.TotalRecord.Pushes = stats.ATSRecord.Pushes + stats.OURecord.Pushes
		stats.TotalRecord.Total = stats.TotalRecord.Wins + stats.TotalRecord.Losses + stats.TotalRecord.Pushes
		if stats.TotalRecord.Total > 0 {
			stats.TotalRecord.WinPct = float64(stats.TotalRecord.Wins) / float64(stats.TotalRecord.Total)
		}
		
		logger.Debugf("User %s final - ATS: %d-%d-%d (%.1f%%), OU: %d-%d-%d (%.1f%%), Total: %d-%d-%d (%.1f%%), Score: %d", 
			stats.UserName,
			stats.ATSRecord.Wins, stats.ATSRecord.Losses, stats.ATSRecord.Pushes, stats.ATSRecord.WinPct*100,
			stats.OURecord.Wins, stats.OURecord.Losses, stats.OURecord.Pushes, stats.OURecord.WinPct*100,
			stats.TotalRecord.Wins, stats.TotalRecord.Losses, stats.TotalRecord.Pushes, stats.TotalRecord.WinPct*100,
			stats.TotalScore)
		
		result = append(result, *stats)
	}
	
	// Sort by total score (highest first)
	sort.Slice(result, func(i, j int) bool {
		return result[i].TotalScore > result[j].TotalScore
	})
	
	return result
}

// Helper function to get keys from int map
func getKeysFromIntMap(m map[int]bool) []int {
	keys := make([]int, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Ints(keys)
	return keys
}

func (h *AnalyticsHandler) calculateTeamStats(games []models.Game) []TeamLeagueStats {
	// Get all teams
	allTeams, err := h.teamService.GetAllTeams()
	if err != nil {
		return []TeamLeagueStats{}
	}
	
	// Initialize team stats map
	teamStatsMap := make(map[string]*TeamLeagueStats)
	for _, team := range allTeams {
		// Skip Over/Under "teams"
		if team.Abbr == "OVR" || team.Abbr == "UND" {
			continue
		}
		
		teamStatsMap[team.Abbr] = &TeamLeagueStats{
			TeamAbbr: team.Abbr,
			TeamName: team.DisplayName(),
		}
	}
	
	// Process each game
	for _, game := range games {
		if !game.IsCompleted() || !game.HasOdds() {
			continue
		}
		
		homeStats := teamStatsMap[game.Home]
		awayStats := teamStatsMap[game.Away]
		
		if homeStats == nil || awayStats == nil {
			continue
		}
		
		// Calculate results
		homeWon := game.HomeScore > game.AwayScore
		awayWon := game.AwayScore > game.HomeScore
		tie := game.HomeScore == game.AwayScore
		
		spreadResult := game.SpreadResult()
		totalPoints := game.HomeScore + game.AwayScore
		
		// Update straight up records
		if homeWon {
			homeStats.SURecord.Wins++
			awayStats.SURecord.Losses++
		} else if awayWon {
			homeStats.SURecord.Losses++
			awayStats.SURecord.Wins++
		} else if tie {
			homeStats.SURecord.Pushes++
			awayStats.SURecord.Pushes++
		}
		homeStats.SURecord.Total++
		awayStats.SURecord.Total++
		
		// Update ATS records
		if spreadResult == "home-covered" {
			homeStats.ATSRecord.Wins++
			awayStats.ATSRecord.Losses++
		} else if spreadResult == "away-covered" {
			homeStats.ATSRecord.Losses++
			awayStats.ATSRecord.Wins++
		} else if spreadResult == "push" {
			homeStats.ATSRecord.Pushes++
			awayStats.ATSRecord.Pushes++
		}
		homeStats.ATSRecord.Total++
		awayStats.ATSRecord.Total++
		
		// Update O/U records (both teams get same result)
		if float64(totalPoints) > game.Odds.OU {
			// Over hit
			homeStats.OURecord.Wins++
			awayStats.OURecord.Wins++
		} else if float64(totalPoints) < game.Odds.OU {
			// Under hit
			homeStats.OURecord.Losses++
			awayStats.OURecord.Losses++
		} else {
			// Push
			homeStats.OURecord.Pushes++
			awayStats.OURecord.Pushes++
		}
		homeStats.OURecord.Total++
		awayStats.OURecord.Total++
		
		// Update upset records (games with spreads >= 5 points)
		if game.Odds.Spread <= -5.0 || game.Odds.Spread >= 5.0 {
			// Home team heavily favored (negative spread) or away team heavily favored (positive spread)
			if homeWon && game.Odds.Spread <= -5.0 {
				// Favorite won (home)
				homeStats.UpsetRecord.Wins++
				awayStats.UpsetRecord.Losses++
			} else if awayWon && game.Odds.Spread >= 5.0 {
				// Favorite won (away)
				homeStats.UpsetRecord.Losses++
				awayStats.UpsetRecord.Wins++
			} else if awayWon && game.Odds.Spread <= -5.0 {
				// Upset (away dog won)
				homeStats.UpsetRecord.Losses++
				awayStats.UpsetRecord.Wins++
			} else if homeWon && game.Odds.Spread >= 5.0 {
				// Upset (home dog won)
				homeStats.UpsetRecord.Wins++
				awayStats.UpsetRecord.Losses++
			} else if tie {
				homeStats.UpsetRecord.Pushes++
				awayStats.UpsetRecord.Pushes++
			}
			homeStats.UpsetRecord.Total++
			awayStats.UpsetRecord.Total++
		}
	}
	
	// Calculate win percentages and convert to slice
	var result []TeamLeagueStats
	for _, stats := range teamStatsMap {
		// Calculate win percentages
		if stats.SURecord.Total > 0 {
			stats.SURecord.WinPct = float64(stats.SURecord.Wins) / float64(stats.SURecord.Total)
		}
		if stats.ATSRecord.Total > 0 {
			stats.ATSRecord.WinPct = float64(stats.ATSRecord.Wins) / float64(stats.ATSRecord.Total)
		}
		if stats.OURecord.Total > 0 {
			stats.OURecord.WinPct = float64(stats.OURecord.Wins) / float64(stats.OURecord.Total)
		}
		if stats.UpsetRecord.Total > 0 {
			stats.UpsetRecord.WinPct = float64(stats.UpsetRecord.Wins) / float64(stats.UpsetRecord.Total)
		}
		
		result = append(result, *stats)
	}
	
	// Sort by win percentage
	sort.Slice(result, func(i, j int) bool {
		return result[i].SURecord.WinPct > result[j].SURecord.WinPct
	})
	
	return result
}

func (h *AnalyticsHandler) calculateLeagueStats(games []models.Game) LeagueStats {
	var leagueStats LeagueStats
	
	// Process each game
	for _, game := range games {
		if !game.IsCompleted() || !game.HasOdds() {
			continue
		}
		
		// Calculate results
		homeWon := game.HomeScore > game.AwayScore
		awayWon := game.AwayScore > game.HomeScore
		tie := game.HomeScore == game.AwayScore
		
		spreadResult := game.SpreadResult()
		totalPoints := game.HomeScore + game.AwayScore
		
		// Determine favorite (negative spread means home favored)
		homeFavored := game.Odds.Spread < 0
		awayFavored := game.Odds.Spread > 0
		pickEm := game.Odds.Spread == 0
		
		// Update favored ATS stats
		if !pickEm {
			leagueStats.FavoredATS.Total++
			
			if homeFavored && spreadResult == "home-covered" {
				// Home favorite covered
				leagueStats.FavoredATS.Wins++
			} else if awayFavored && spreadResult == "away-covered" {
				// Away favorite covered
				leagueStats.FavoredATS.Wins++
			} else if spreadResult == "push" {
				leagueStats.FavoredATS.Pushes++
			} else {
				// Favorite didn't cover
				leagueStats.FavoredATS.Losses++
			}
		}
		
		// Update home ATS stats
		leagueStats.HomeATS.Total++
		if spreadResult == "home-covered" {
			leagueStats.HomeATS.Wins++
		} else if spreadResult == "away-covered" {
			leagueStats.HomeATS.Losses++
		} else if spreadResult == "push" {
			leagueStats.HomeATS.Pushes++
		}
		
		// Update favored SU (straight up) stats
		if !pickEm {
			leagueStats.FavoredSU.Total++
			
			if homeFavored && homeWon {
				// Home favorite won
				leagueStats.FavoredSU.Wins++
			} else if awayFavored && awayWon {
				// Away favorite won
				leagueStats.FavoredSU.Wins++
			} else if tie {
				leagueStats.FavoredSU.Pushes++
			} else {
				// Favorite lost (upset)
				leagueStats.FavoredSU.Losses++
			}
		}
		
		// Update upset SU stats (spreads >= 5 points)
		if game.Odds.Spread <= -5.0 || game.Odds.Spread >= 5.0 {
			leagueStats.UpsetSU.Total++
			
			// Check if underdog won
			if (game.Odds.Spread <= -5.0 && awayWon) || (game.Odds.Spread >= 5.0 && homeWon) {
				// Upset occurred
				leagueStats.UpsetSU.Wins++
			} else if tie {
				leagueStats.UpsetSU.Pushes++
			} else {
				// Favorite won
				leagueStats.UpsetSU.Losses++
			}
		}
		
		// Update over/under stats
		leagueStats.OverUnder.Total++
		if float64(totalPoints) > game.Odds.OU {
			// Over hit
			leagueStats.OverUnder.Wins++
		} else if float64(totalPoints) < game.Odds.OU {
			// Under hit
			leagueStats.OverUnder.Losses++
		} else {
			// Push
			leagueStats.OverUnder.Pushes++
		}
		
		// Update extreme totals stats
		if game.Odds.OU >= 50.0 {
			// High total game (50+)
			leagueStats.ExtremeOVR.Total++
			if float64(totalPoints) > game.Odds.OU {
				leagueStats.ExtremeOVR.Wins++
			} else if float64(totalPoints) < game.Odds.OU {
				leagueStats.ExtremeOVR.Losses++
			} else {
				leagueStats.ExtremeOVR.Pushes++
			}
		}
		
		if game.Odds.OU <= 40.0 {
			// Low total game (40 or less)
			leagueStats.ExtremeUND.Total++
			if float64(totalPoints) < game.Odds.OU {
				// Under hit in low total game
				leagueStats.ExtremeUND.Wins++
			} else if float64(totalPoints) > game.Odds.OU {
				// Over hit in low total game
				leagueStats.ExtremeUND.Losses++
			} else {
				leagueStats.ExtremeUND.Pushes++
			}
		}
	}
	
	// Calculate win percentages
	if leagueStats.FavoredATS.Total > 0 {
		leagueStats.FavoredATS.WinPct = float64(leagueStats.FavoredATS.Wins) / float64(leagueStats.FavoredATS.Total)
	}
	if leagueStats.HomeATS.Total > 0 {
		leagueStats.HomeATS.WinPct = float64(leagueStats.HomeATS.Wins) / float64(leagueStats.HomeATS.Total)
	}
	if leagueStats.FavoredSU.Total > 0 {
		leagueStats.FavoredSU.WinPct = float64(leagueStats.FavoredSU.Wins) / float64(leagueStats.FavoredSU.Total)
	}
	if leagueStats.UpsetSU.Total > 0 {
		leagueStats.UpsetSU.WinPct = float64(leagueStats.UpsetSU.Wins) / float64(leagueStats.UpsetSU.Total)
	}
	if leagueStats.OverUnder.Total > 0 {
		leagueStats.OverUnder.WinPct = float64(leagueStats.OverUnder.Wins) / float64(leagueStats.OverUnder.Total)
	}
	if leagueStats.ExtremeOVR.Total > 0 {
		leagueStats.ExtremeOVR.WinPct = float64(leagueStats.ExtremeOVR.Wins) / float64(leagueStats.ExtremeOVR.Total)
	}
	if leagueStats.ExtremeUND.Total > 0 {
		leagueStats.ExtremeUND.WinPct = float64(leagueStats.ExtremeUND.Wins) / float64(leagueStats.ExtremeUND.Total)
	}
	
	return leagueStats
}

// Helper functions
func (h *AnalyticsHandler) calculatePickResult(pick models.Pick, game models.Game) string {
	if !game.IsCompleted() || !game.HasOdds() {
		return "pending"
	}
	
	if pick.PickType == "spread" {
		// Get spread result from game
		spreadResult := game.SpreadResult()
		
		// Determine if the picked team covered
		var pickWon bool
		if pick.TeamName == game.Home {
			pickWon = (spreadResult == "home-covered")
		} else if pick.TeamName == game.Away {
			pickWon = (spreadResult == "away-covered")
		}
		
		if spreadResult == "push" {
			return "push"
		} else if pickWon {
			return "win"
		} else {
			return "loss"
		}
		
	} else if pick.PickType == "over_under" {
		// Calculate total points
		totalPoints := game.HomeScore + game.AwayScore
		
		// Compare with over/under line
		if float64(totalPoints) > game.Odds.OU {
			// Game went over
			if pick.TeamName == "OVR" || pick.TeamName == "Over" {
				return "win"
			} else if pick.TeamName == "UND" || pick.TeamName == "Under" {
				return "loss"
			}
		} else if float64(totalPoints) < game.Odds.OU {
			// Game went under
			if pick.TeamName == "UND" || pick.TeamName == "Under" {
				return "win"
			} else if pick.TeamName == "OVR" || pick.TeamName == "Over" {
				return "loss"
			}
		} else {
			// Exact total - push
			return "push"
		}
	}
	
	// Default fallback
	return "loss"
}

func (h *AnalyticsHandler) updateATSStats(record *Record, result string, game models.Game) {
	record.Total++
	switch result {
	case "win":
		record.Wins++
	case "loss":
		record.Losses++
	case "push":
		record.Pushes++
	}
	
	if record.Total > 0 {
		record.WinPct = float64(record.Wins) / float64(record.Total)
	}
}

func (h *AnalyticsHandler) updateOUStats(record *Record, result string, game models.Game) {
	record.Total++
	switch result {
	case "win":
		record.Wins++
	case "loss":
		record.Losses++
	case "push":
		record.Pushes++
	}
	
	if record.Total > 0 {
		record.WinPct = float64(record.Wins) / float64(record.Total)
	}
}