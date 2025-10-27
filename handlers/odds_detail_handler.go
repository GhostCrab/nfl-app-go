package handlers

import (
	"context"
	"fmt"
	"html/template"
	"net/http"
	"nfl-app-go/logging"
	"nfl-app-go/middleware"
	"nfl-app-go/models"
	"nfl-app-go/services"
	"sort"
	"strconv"
	"sync"
	"time"
)

// OddsDetailHandler handles odds details dashboard requests
type OddsDetailHandler struct {
	templates        *template.Template
	espnService      *services.ESPNService
	gameService      services.GameService
	analyticsService *services.AnalyticsService
	logger           *logging.Logger
	cache            *OddsCache
}

// OddsCache stores fetched odds data with expiration
type OddsCache struct {
	data  map[string]*CachedOddsData
	mutex sync.RWMutex
}

// CachedOddsData represents cached odds information for a week
type CachedOddsData struct {
	Week      int
	Season    int
	Games     []GameOddsInfo
	Timestamp time.Time
}

// GameOddsInfo contains all odds information for a single game
type GameOddsInfo struct {
	Game              models.Game
	ComprehensiveOdds *services.ComprehensiveOddsData
	LockedOdds        *models.Odds // From database (locked Wed 10pm PT)
	IsLocked          bool         // Whether we're past the lock time
	SpreadDelta       float64      // Difference between current and locked spread
	OUDelta           float64      // Difference between current and locked OU
	SpreadMovement    float64      // Difference between opening and current spread
	OUMovement        float64      // Difference between opening and current OU
	FetchError        string       // Error message if live odds fetch failed
	ATSRecords        *TeamRecords // ATS records from analytics service
	OURecords         *TeamRecords // O/U records from analytics service
}

// TeamRecords holds record information for both teams
type TeamRecords struct {
	HomeRecord   string
	HomeWinPct   float64
	HomeWins     int
	HomeLosses   int
	HomePushes   int
	AwayRecord   string
	AwayWinPct   float64
	AwayWins     int
	AwayLosses   int
	AwayPushes   int
}

// RecommendedPick represents a suggested pick with analysis
type RecommendedPick struct {
	GameID      int
	GameDesc    string
	GameTime    time.Time
	PickType    string // "ATS" or "O/U"
	PickTeam    string // Team abbr for ATS, "Over" or "Under" for O/U
	PickValue   string // Spread value or O/U value
	Score       int    // Confidence score
	Explanation string
}

// DailyPicks holds recommended picks grouped by day
type DailyPicks struct {
	Date  string
	Picks []RecommendedPick
}

// NewOddsDetailHandler creates a new odds detail handler
func NewOddsDetailHandler(templates *template.Template, espnService *services.ESPNService, gameService services.GameService, analyticsService *services.AnalyticsService) *OddsDetailHandler {
	return &OddsDetailHandler{
		templates:        templates,
		espnService:      espnService,
		gameService:      gameService,
		analyticsService: analyticsService,
		logger:           logging.WithPrefix("OddsDetail"),
		cache: &OddsCache{
			data: make(map[string]*CachedOddsData),
		},
	}
}

// ShowOddsDetails displays the odds details dashboard
func (h *OddsDetailHandler) ShowOddsDetails(w http.ResponseWriter, r *http.Request) {
	h.logger.Debugf("HTTP: %s %s from %s", r.Method, r.URL.Path, r.RemoteAddr)

	// Get user from context
	user := middleware.GetUserFromContext(r)

	// Parse query parameters
	weekStr := r.URL.Query().Get("week")
	seasonStr := r.URL.Query().Get("season")

	// Default to current week/season
	now := time.Now()
	season := 2025
	if seasonStr != "" {
		if parsedSeason, err := strconv.Atoi(seasonStr); err == nil {
			season = parsedSeason
		}
	}

	var week int
	if weekStr != "" {
		if parsedWeek, err := strconv.Atoi(weekStr); err == nil {
			week = parsedWeek
		}
	} else {
		// Get current week using models function
		week = models.GetNFLWeekForDate(now, season)
	}

	// Get odds data (from cache or fetch fresh)
	oddsData, err := h.GetOddsData(r.Context(), season, week)
	if err != nil {
		h.logger.Errorf("Error getting odds data: %v", err)
		http.Error(w, "Error loading odds data", http.StatusInternalServerError)
		return
	}

	// Calculate lock time for this week
	lockTime := h.calculateLockTime(season, week)

	// Generate recommended picks
	dailyPicks := h.generateDailyPicks(oddsData.Games)

	// Template data
	data := struct {
		Title        string
		User         *models.User
		Week         int
		Season       int
		Games        []GameOddsInfo
		DailyPicks   []DailyPicks
		LockTime     time.Time
		IsLocked     bool
		LastUpdated  time.Time
		CacheExpires time.Time
	}{
		Title:        fmt.Sprintf("Odds Details - Week %d", week),
		User:         user,
		Week:         week,
		Season:       season,
		Games:        oddsData.Games,
		DailyPicks:   dailyPicks,
		LockTime:     lockTime,
		IsLocked:     now.After(lockTime),
		LastUpdated:  oddsData.Timestamp,
		CacheExpires: oddsData.Timestamp.Add(2 * time.Hour),
	}

	// Render template
	err = h.templates.ExecuteTemplate(w, "odds_details.html", data)
	if err != nil {
		h.logger.Errorf("Template error: %v", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	h.logger.Debugf("Successfully served %s %s", r.Method, r.URL.Path)
}

// GetOddsData retrieves odds data from cache or fetches fresh data
func (h *OddsDetailHandler) GetOddsData(ctx context.Context, season int, week int) (*CachedOddsData, error) {
	cacheKey := fmt.Sprintf("%d-%d", season, week)

	// Check cache first
	h.cache.mutex.RLock()
	cached, exists := h.cache.data[cacheKey]
	h.cache.mutex.RUnlock()

	// Return cached data if still valid (< 2 hours old)
	if exists && time.Since(cached.Timestamp) < 2*time.Hour {
		h.logger.Infof("Returning cached odds data for week %d (age: %v)", week, time.Since(cached.Timestamp))
		return cached, nil
	}

	// Cache miss or expired - fetch fresh data
	h.logger.Infof("Cache miss/expired for week %d - fetching fresh odds data", week)

	// Get games for this week from database
	games, err := h.getGamesForWeek(season, week)
	if err != nil {
		return nil, fmt.Errorf("failed to get games: %w", err)
	}

	if len(games) == 0 {
		h.logger.Warnf("No games found for season %d week %d", season, week)
	}

	// Get ALL games for the season (for ATS/O/U calculations)
	allSeasonGames, err := h.getGamesForSeason(season)
	if err != nil {
		h.logger.Warnf("Failed to get all season games for records calculation: %v", err)
		allSeasonGames = games // Fallback to just this week's games
	}

	// Calculate lock time
	lockTime := h.calculateLockTime(season, week)
	isLocked := time.Now().After(lockTime)

	// Fetch comprehensive odds for each game
	gamesOdds := make([]GameOddsInfo, len(games))
	for i, game := range games {
		gameInfo := GameOddsInfo{
			Game:       game,
			LockedOdds: game.Odds, // Database odds (locked)
			IsLocked:   isLocked,
		}

		// Get ESPN team IDs for ATS lookups
		homeTeamID := services.GetESPNTeamID(game.Home)
		awayTeamID := services.GetESPNTeamID(game.Away)

		// Fetch comprehensive odds from ESPN
		comprehensiveOdds, err := h.espnService.GetComprehensiveOdds(game.ID, homeTeamID, awayTeamID, season)
		if err != nil {
			h.logger.Warnf("Failed to fetch comprehensive odds for game %d: %v", game.ID, err)
			gameInfo.FetchError = err.Error()
		} else {
			gameInfo.ComprehensiveOdds = comprehensiveOdds

			// Calculate line movement (opening vs current)
			if comprehensiveOdds.CurrentOdds != nil && comprehensiveOdds.OpeningOdds != nil {
				gameInfo.SpreadMovement = comprehensiveOdds.CurrentOdds.Spread - comprehensiveOdds.OpeningOdds.Spread
				gameInfo.OUMovement = comprehensiveOdds.CurrentOdds.OverUnder - comprehensiveOdds.OpeningOdds.OverUnder
			}

			// Calculate deltas vs locked odds if applicable
			if game.HasOdds() && isLocked && comprehensiveOdds.CurrentOdds != nil {
				gameInfo.SpreadDelta = comprehensiveOdds.CurrentOdds.Spread - game.Odds.Spread
				gameInfo.OUDelta = comprehensiveOdds.CurrentOdds.OverUnder - game.Odds.OU
			}
		}

		// Calculate ATS and O/U records from all season games
		gameInfo.ATSRecords = h.calculateATSRecords(ctx, allSeasonGames, game.Home, game.Away)
		gameInfo.OURecords = h.calculateOURecords(ctx, allSeasonGames, game.Home, game.Away)

		gamesOdds[i] = gameInfo
	}

	// Sort games by date
	sort.Slice(gamesOdds, func(i, j int) bool {
		return gamesOdds[i].Game.Date.Before(gamesOdds[j].Game.Date)
	})

	// Create cached data
	cachedData := &CachedOddsData{
		Week:      week,
		Season:    season,
		Games:     gamesOdds,
		Timestamp: time.Now(),
	}

	// Store in cache
	h.cache.mutex.Lock()
	h.cache.data[cacheKey] = cachedData
	h.cache.mutex.Unlock()

	h.logger.Infof("Cached fresh odds data for week %d (%d games)", week, len(gamesOdds))

	return cachedData, nil
}

// getGamesForSeason fetches all games for a specific season from the database
func (h *OddsDetailHandler) getGamesForSeason(season int) ([]models.Game, error) {
	if gameServiceWithSeason, ok := h.gameService.(interface{ GetGamesBySeason(int) ([]models.Game, error) }); ok {
		return gameServiceWithSeason.GetGamesBySeason(season)
	}
	return nil, fmt.Errorf("game service does not support season queries")
}

// getGamesForWeek fetches games for a specific week/season from the database
func (h *OddsDetailHandler) getGamesForWeek(season int, week int) ([]models.Game, error) {
	// Try to use the season-specific method if available
	if gameServiceWithWeek, ok := h.gameService.(interface {
		GetGamesByWeekSeason(week int, season int) ([]models.Game, error)
	}); ok {
		return gameServiceWithWeek.GetGamesByWeekSeason(week, season)
	}

	// Fallback: get all games and filter
	if gameServiceWithSeason, ok := h.gameService.(interface{ GetGamesBySeason(int) ([]models.Game, error) }); ok {
		games, err := gameServiceWithSeason.GetGamesBySeason(season)
		if err != nil {
			return nil, err
		}

		// Filter by week
		var filtered []models.Game
		for _, game := range games {
			if game.Week == week {
				filtered = append(filtered, game)
			}
		}
		return filtered, nil
	}

	return nil, fmt.Errorf("game service does not support week/season queries")
}

// calculateLockTime calculates the Wednesday 10pm PT lock time for a given week
func (h *OddsDetailHandler) calculateLockTime(season int, week int) time.Time {
	// Get first game of the week to determine week start
	games, err := h.getGamesForWeek(season, week)
	if err != nil || len(games) == 0 {
		// Fallback: return a time in the past
		return time.Now().Add(-24 * time.Hour)
	}

	// Find earliest game in the week
	var weekStart time.Time
	for i, game := range games {
		if i == 0 || game.Date.Before(weekStart) {
			weekStart = game.Date
		}
	}

	// Find the Wednesday before or of the week
	// Go back to the start of the week (Sunday)
	weekday := weekStart.Weekday()
	daysToSunday := int(weekday)
	sunday := weekStart.AddDate(0, 0, -daysToSunday)

	// Wednesday is 3 days after Sunday
	wednesday := sunday.AddDate(0, 0, 3)

	// Set to 10pm Pacific Time
	loc, err := time.LoadLocation("America/Los_Angeles")
	if err != nil {
		h.logger.Errorf("Failed to load Pacific timezone: %v", err)
		loc = time.UTC
	}

	lockTime := time.Date(
		wednesday.Year(),
		wednesday.Month(),
		wednesday.Day(),
		22, 0, 0, 0, // 10pm
		loc,
	)

	return lockTime
}

// calculateATSRecords calculates ATS records for both teams from completed games
func (h *OddsDetailHandler) calculateATSRecords(ctx context.Context, allGames []models.Game, homeTeam, awayTeam string) *TeamRecords {
	records := &TeamRecords{}

	homeWins, homeLosses, homePushes := 0, 0, 0
	awayWins, awayLosses, awayPushes := 0, 0, 0

	for _, game := range allGames {
		if !game.IsCompleted() || !game.HasOdds() {
			continue
		}

		spreadResult := game.SpreadResult()

		// Check home team
		if game.Home == homeTeam {
			if spreadResult == "home-covered" {
				homeWins++
			} else if spreadResult == "away-covered" {
				homeLosses++
			} else if spreadResult == "push" {
				homePushes++
			}
		} else if game.Away == homeTeam {
			if spreadResult == "away-covered" {
				homeWins++
			} else if spreadResult == "home-covered" {
				homeLosses++
			} else if spreadResult == "push" {
				homePushes++
			}
		}

		// Check away team
		if game.Home == awayTeam {
			if spreadResult == "home-covered" {
				awayWins++
			} else if spreadResult == "away-covered" {
				awayLosses++
			} else if spreadResult == "push" {
				awayPushes++
			}
		} else if game.Away == awayTeam {
			if spreadResult == "away-covered" {
				awayWins++
			} else if spreadResult == "home-covered" {
				awayLosses++
			} else if spreadResult == "push" {
				awayPushes++
			}
		}
	}

	records.HomeRecord = fmt.Sprintf("%d-%d-%d", homeWins, homeLosses, homePushes)
	records.HomeWins = homeWins
	records.HomeLosses = homeLosses
	records.HomePushes = homePushes
	if homeWins+homeLosses > 0 {
		records.HomeWinPct = float64(homeWins) / float64(homeWins+homeLosses) * 100
	}

	records.AwayRecord = fmt.Sprintf("%d-%d-%d", awayWins, awayLosses, awayPushes)
	records.AwayWins = awayWins
	records.AwayLosses = awayLosses
	records.AwayPushes = awayPushes
	if awayWins+awayLosses > 0 {
		records.AwayWinPct = float64(awayWins) / float64(awayWins+awayLosses) * 100
	}

	return records
}

// calculateOURecords calculates O/U records for both teams from completed games
func (h *OddsDetailHandler) calculateOURecords(ctx context.Context, allGames []models.Game, homeTeam, awayTeam string) *TeamRecords {
	records := &TeamRecords{}

	homeOvers, homeUnders, homePushes := 0, 0, 0
	awayOvers, awayUnders, awayPushes := 0, 0, 0

	for _, game := range allGames {
		if !game.IsCompleted() || !game.HasOdds() {
			continue
		}

		totalPoints := float64(game.HomeScore + game.AwayScore)

		var isOver, isUnder, isPush bool
		if totalPoints > game.Odds.OU {
			isOver = true
		} else if totalPoints < game.Odds.OU {
			isUnder = true
		} else {
			isPush = true
		}

		// Check home team
		if game.Home == homeTeam || game.Away == homeTeam {
			if isOver {
				homeOvers++
			} else if isUnder {
				homeUnders++
			} else if isPush {
				homePushes++
			}
		}

		// Check away team
		if game.Home == awayTeam || game.Away == awayTeam {
			if isOver {
				awayOvers++
			} else if isUnder {
				awayUnders++
			} else if isPush {
				awayPushes++
			}
		}
	}

	records.HomeRecord = fmt.Sprintf("%d-%d-%d", homeOvers, homeUnders, homePushes)
	records.HomeWins = homeOvers
	records.HomeLosses = homeUnders
	records.HomePushes = homePushes
	if homeOvers+homeUnders > 0 {
		records.HomeWinPct = float64(homeOvers) / float64(homeOvers+homeUnders) * 100
	}

	records.AwayRecord = fmt.Sprintf("%d-%d-%d", awayOvers, awayUnders, awayPushes)
	records.AwayWins = awayOvers
	records.AwayLosses = awayUnders
	records.AwayPushes = awayPushes
	if awayOvers+awayUnders > 0 {
		records.AwayWinPct = float64(awayOvers) / float64(awayOvers+awayUnders) * 100
	}

	return records
}

// generateDailyPicks generates recommended picks grouped by day
func (h *OddsDetailHandler) generateDailyPicks(games []GameOddsInfo) []DailyPicks {
	// Get Pacific timezone
	pacificLoc, err := time.LoadLocation("America/Los_Angeles")
	if err != nil {
		h.logger.Errorf("Failed to load Pacific timezone: %v", err)
		pacificLoc = time.UTC
	}

	// Group games by day (in Pacific time)
	gamesByDay := make(map[string][]GameOddsInfo)
	for _, game := range games {
		if game.Game.IsCompleted() {
			continue // Skip completed games
		}
		// Convert to Pacific time for grouping
		pacificTime := game.Game.Date.In(pacificLoc)
		dayKey := pacificTime.Format("Monday, Jan 2")
		gamesByDay[dayKey] = append(gamesByDay[dayKey], game)
	}

	// Generate picks for each day
	var dailyPicks []DailyPicks
	for day, dayGames := range gamesByDay {
		picksNeeded := 2
		if len(dayGames) >= 2 {
			picksNeeded = 4
		}

		picks := h.calculateBestPicks(dayGames, picksNeeded)
		if len(picks) > 0 {
			dailyPicks = append(dailyPicks, DailyPicks{
				Date:  day,
				Picks: picks,
			})
		}
	}

	// Sort by date
	sort.Slice(dailyPicks, func(i, j int) bool {
		// Parse dates for comparison
		ti, _ := time.Parse("Monday, Jan 2", dailyPicks[i].Date)
		tj, _ := time.Parse("Monday, Jan 2", dailyPicks[j].Date)
		return ti.Before(tj)
	})

	return dailyPicks
}

// calculateBestPicks calculates the best picks based on ATS/O/U analysis
func (h *OddsDetailHandler) calculateBestPicks(games []GameOddsInfo, limit int) []RecommendedPick {
	var allPicks []RecommendedPick

	for _, game := range games {
		if !game.Game.HasOdds() || game.ATSRecords == nil || game.OURecords == nil {
			continue
		}

		// Check for line movement
		var spreadMovement string
		if game.IsLocked && game.LockedOdds != nil && game.ComprehensiveOdds != nil && game.ComprehensiveOdds.CurrentOdds != nil {
			if game.SpreadDelta != 0 {
				if game.SpreadDelta > 0 {
					spreadMovement = fmt.Sprintf(" (Line moved from %.1f to %.1f)", game.LockedOdds.Spread, game.ComprehensiveOdds.CurrentOdds.Spread)
				} else {
					spreadMovement = fmt.Sprintf(" (Line moved from %.1f to %.1f)", game.LockedOdds.Spread, game.ComprehensiveOdds.CurrentOdds.Spread)
				}
			}
		}

		var ouMovement string
		if game.IsLocked && game.LockedOdds != nil && game.ComprehensiveOdds != nil && game.ComprehensiveOdds.CurrentOdds != nil {
			if game.OUDelta != 0 {
				ouMovement = fmt.Sprintf(" (Line moved from %.1f to %.1f)", game.LockedOdds.OU, game.ComprehensiveOdds.CurrentOdds.OverUnder)
			}
		}

		// Calculate ATS picks for both teams
		homeATSScore := game.ATSRecords.HomeWins + game.ATSRecords.AwayLosses -
			game.ATSRecords.HomeLosses - game.ATSRecords.AwayWins
		awayATSScore := game.ATSRecords.AwayWins + game.ATSRecords.HomeLosses -
			game.ATSRecords.AwayLosses - game.ATSRecords.HomeWins

		if homeATSScore > 0 {
			explanation := fmt.Sprintf("%s ATS: %s, %s ATS: %s",
				game.Game.Home, game.ATSRecords.HomeRecord,
				game.Game.Away, game.ATSRecords.AwayRecord)
			if spreadMovement != "" {
				explanation += spreadMovement
			}

			allPicks = append(allPicks, RecommendedPick{
				GameID:      game.Game.ID,
				GameDesc:    fmt.Sprintf("%s @ %s", game.Game.Away, game.Game.Home),
				GameTime:    game.Game.Date,
				PickType:    "ATS",
				PickTeam:    game.Game.Home,
				PickValue:   game.Game.FormatHomeSpread(),
				Score:       homeATSScore,
				Explanation: explanation,
			})
		}

		if awayATSScore > 0 {
			explanation := fmt.Sprintf("%s ATS: %s, %s ATS: %s",
				game.Game.Away, game.ATSRecords.AwayRecord,
				game.Game.Home, game.ATSRecords.HomeRecord)
			if spreadMovement != "" {
				explanation += spreadMovement
			}

			allPicks = append(allPicks, RecommendedPick{
				GameID:      game.Game.ID,
				GameDesc:    fmt.Sprintf("%s @ %s", game.Game.Away, game.Game.Home),
				GameTime:    game.Game.Date,
				PickType:    "ATS",
				PickTeam:    game.Game.Away,
				PickValue:   game.Game.FormatAwaySpread(),
				Score:       awayATSScore,
				Explanation: explanation,
			})
		}

		// Calculate O/U picks
		overScore := game.OURecords.HomeWins + game.OURecords.AwayWins -
			game.OURecords.HomeLosses - game.OURecords.AwayLosses
		underScore := game.OURecords.HomeLosses + game.OURecords.AwayLosses -
			game.OURecords.HomeWins - game.OURecords.AwayWins

		if overScore > 0 {
			explanation := fmt.Sprintf("%s O/U: %s, %s O/U: %s",
				game.Game.Home, game.OURecords.HomeRecord,
				game.Game.Away, game.OURecords.AwayRecord)
			if ouMovement != "" {
				explanation += ouMovement
			}

			allPicks = append(allPicks, RecommendedPick{
				GameID:      game.Game.ID,
				GameDesc:    fmt.Sprintf("%s @ %s", game.Game.Away, game.Game.Home),
				GameTime:    game.Game.Date,
				PickType:    "O/U",
				PickTeam:    "Over",
				PickValue:   fmt.Sprintf("%.1f", game.Game.Odds.OU),
				Score:       overScore,
				Explanation: explanation,
			})
		}

		if underScore > 0 {
			explanation := fmt.Sprintf("%s O/U: %s, %s O/U: %s",
				game.Game.Home, game.OURecords.HomeRecord,
				game.Game.Away, game.OURecords.AwayRecord)
			if ouMovement != "" {
				explanation += ouMovement
			}

			allPicks = append(allPicks, RecommendedPick{
				GameID:      game.Game.ID,
				GameDesc:    fmt.Sprintf("%s @ %s", game.Game.Away, game.Game.Home),
				GameTime:    game.Game.Date,
				PickType:    "O/U",
				PickTeam:    "Under",
				PickValue:   fmt.Sprintf("%.1f", game.Game.Odds.OU),
				Score:       underScore,
				Explanation: explanation,
			})
		}
	}

	// Sort by score (descending)
	sort.Slice(allPicks, func(i, j int) bool {
		return allPicks[i].Score > allPicks[j].Score
	})

	// Return top picks up to limit
	if len(allPicks) > limit {
		return allPicks[:limit]
	}
	return allPicks
}
