package services

import (
	"context"
	"fmt"
	"math"
	"nfl-app-go/database"
	"nfl-app-go/models"
	"sort"
)

// AnalyticsService handles all analytics and statistics functionality
// This service provides insights into pick performance, user statistics,
// team performance, and game analysis.
type AnalyticsService struct {
	pickRepo *database.MongoPickRepository
	gameRepo *database.MongoGameRepository
	userRepo *database.MongoUserRepository
}

// NewAnalyticsService creates a new analytics service
func NewAnalyticsService(
	pickRepo *database.MongoPickRepository,
	gameRepo *database.MongoGameRepository,
	userRepo *database.MongoUserRepository,
) *AnalyticsService {
	return &AnalyticsService{
		pickRepo: pickRepo,
		gameRepo: gameRepo,
		userRepo: userRepo,
	}
}

// UserPerformanceStats represents comprehensive user performance statistics
type UserPerformanceStats struct {
	UserID              int                              `json:"user_id"`
	UserName            string                           `json:"user_name"`
	Season              int                              `json:"season"`
	TotalPicks          int                              `json:"total_picks"`
	WinRate             float64                          `json:"win_rate"`
	PushRate            float64                          `json:"push_rate"`
	ResultBreakdown     map[models.PickResult]int        `json:"result_breakdown"`
	TypeBreakdown       map[models.PickType]int          `json:"type_breakdown"`
	TypePerformance     map[models.PickType]float64      `json:"type_performance"`
	WeeklyPerformance   []WeeklyStats                    `json:"weekly_performance"`
	TeamPerformance     map[string]TeamPickStats         `json:"team_performance"`
	OverUnderAnalysis   OverUnderAnalysis                `json:"over_under_analysis"`
	SpreadAnalysis      SpreadAnalysis                   `json:"spread_analysis"`
	ParlayPerformance   ParlayPerformanceStats           `json:"parlay_performance"`
	RecentForm          RecentFormStats                  `json:"recent_form"`
}

// WeeklyStats represents performance for a specific week
type WeeklyStats struct {
	Week         int     `json:"week"`
	TotalPicks   int     `json:"total_picks"`
	Wins         int     `json:"wins"`
	Losses       int     `json:"losses"`
	Pushes       int     `json:"pushes"`
	WinRate      float64 `json:"win_rate"`
}

// TeamPickStats represents performance when picking for/against specific teams
type TeamPickStats struct {
	TeamAbbr    string  `json:"team_abbr"`
	PicksFor    int     `json:"picks_for"`    // Picks selecting this team
	PicksAgainst int    `json:"picks_against"` // Picks against this team (spread)
	WinsFor     int     `json:"wins_for"`
	WinsAgainst int     `json:"wins_against"`
	WinRateFor  float64 `json:"win_rate_for"`
	WinRateAgainst float64 `json:"win_rate_against"`
}

// OverUnderAnalysis represents over/under betting analysis
type OverUnderAnalysis struct {
	TotalOverPicks   int     `json:"total_over_picks"`
	TotalUnderPicks  int     `json:"total_under_picks"`
	OverWinRate      float64 `json:"over_win_rate"`
	UnderWinRate     float64 `json:"under_win_rate"`
	AverageTotal     float64 `json:"average_total"`
	HighestTotal     float64 `json:"highest_total"`
	LowestTotal      float64 `json:"lowest_total"`
}

// SpreadAnalysis represents spread betting analysis  
type SpreadAnalysis struct {
	FavoriteWinRate   float64 `json:"favorite_win_rate"`   // Win rate when picking favorites
	UnderdogWinRate   float64 `json:"underdog_win_rate"`   // Win rate when picking underdogs  
	HomeWinRate       float64 `json:"home_win_rate"`       // Win rate when picking home teams
	AwayWinRate       float64 `json:"away_win_rate"`       // Win rate when picking away teams
	AverageSpread     float64 `json:"average_spread"`      // Average spread of games picked
	LargeSpreadWinRate float64 `json:"large_spread_win_rate"` // Win rate on spreads > 7 points
}

// ParlayPerformanceStats represents parlay performance statistics
type ParlayPerformanceStats struct {
	TotalParlayPoints    int                         `json:"total_parlay_points"`
	WeeklyParlayAverage  float64                     `json:"weekly_parlay_average"`
	BestWeek            int                         `json:"best_week"`
	BestWeekPoints      int                         `json:"best_week_points"`
	CategoryBreakdown   map[string]int              `json:"category_breakdown"`
	ParlayHitRate       map[int]float64             `json:"parlay_hit_rate"` // hit rate by parlay size
}

// RecentFormStats represents recent performance trends
type RecentFormStats struct {
	Last5WinRate    float64   `json:"last_5_win_rate"`
	Last10WinRate   float64   `json:"last_10_win_rate"`
	Trend           string    `json:"trend"`        // "improving", "declining", "stable"
	HotStreak       int       `json:"hot_streak"`   // Current winning streak
	ColdStreak      int       `json:"cold_streak"`  // Current losing streak
}

// GetUserPerformanceStats returns comprehensive performance statistics for a user
func (s *AnalyticsService) GetUserPerformanceStats(ctx context.Context, userID, season int) (*UserPerformanceStats, error) {
	// Get user information
	user, err := s.userRepo.FindByID(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to get user: %w", err)
	}

	// Get all picks for the user this season
	picks, err := s.pickRepo.GetUserPicksBySeason(ctx, userID, season)
	if err != nil {
		return nil, fmt.Errorf("failed to get user picks: %w", err)
	}

	if len(picks) == 0 {
		return &UserPerformanceStats{
			UserID:   userID,
			UserName: user.Name,
			Season:   season,
		}, nil
	}

	// Get games for context
	games, err := s.gameRepo.GetGamesBySeason(season)  
	if err != nil {
		return nil, fmt.Errorf("failed to get games: %w", err)
	}

	gameMap := make(map[int]*models.Game)
	for _, game := range games {
		gameMap[game.ID] = game
	}

	stats := &UserPerformanceStats{
		UserID:          userID,
		UserName:        user.Name,
		Season:          season,
		TotalPicks:      len(picks),
		ResultBreakdown: make(map[models.PickResult]int),
		TypeBreakdown:   make(map[models.PickType]int),
		TypePerformance: make(map[models.PickType]float64),
		TeamPerformance: make(map[string]TeamPickStats),
	}

	// Analyze picks
	s.analyzePickResults(picks, gameMap, stats)
	s.analyzeWeeklyPerformance(picks, stats)
	s.analyzeTeamPerformance(picks, gameMap, stats)
	s.analyzeOverUnderPerformance(picks, gameMap, stats)
	s.analyzeSpreadPerformance(picks, gameMap, stats)
	s.analyzeRecentForm(picks, stats)

	return stats, nil
}

// analyzePickResults analyzes overall pick results and calculates basic statistics
func (s *AnalyticsService) analyzePickResults(picks []models.Pick, gameMap map[int]*models.Game, stats *UserPerformanceStats) {
	wins := 0
	pushes := 0
	
	for _, pick := range picks {
		stats.ResultBreakdown[pick.Result]++
		stats.TypeBreakdown[pick.PickType]++
		
		if pick.Result == models.PickResultWin {
			wins++
		} else if pick.Result == models.PickResultPush {
			pushes++
		}
	}
	
	// Calculate overall rates
	if stats.TotalPicks > 0 {
		stats.WinRate = float64(wins) / float64(stats.TotalPicks)
		stats.PushRate = float64(pushes) / float64(stats.TotalPicks)
	}
	
	// Calculate performance by pick type
	for pickType := range stats.TypeBreakdown {
		typeWins := 0
		typeTotal := 0
		
		for _, pick := range picks {
			if pick.PickType == pickType {
				typeTotal++
				if pick.Result == models.PickResultWin {
					typeWins++
				}
			}
		}
		
		if typeTotal > 0 {
			stats.TypePerformance[pickType] = float64(typeWins) / float64(typeTotal)
		}
	}
}

// analyzeWeeklyPerformance breaks down performance by week
func (s *AnalyticsService) analyzeWeeklyPerformance(picks []models.Pick, stats *UserPerformanceStats) {
	weeklyMap := make(map[int]*WeeklyStats)
	
	for _, pick := range picks {
		if weeklyMap[pick.Week] == nil {
			weeklyMap[pick.Week] = &WeeklyStats{Week: pick.Week}
		}
		
		ws := weeklyMap[pick.Week]
		ws.TotalPicks++
		
		switch pick.Result {
		case models.PickResultWin:
			ws.Wins++
		case models.PickResultLoss:
			ws.Losses++
		case models.PickResultPush:
			ws.Pushes++
		}
	}
	
	// Calculate win rates and convert to slice
	for _, ws := range weeklyMap {
		if ws.TotalPicks > 0 {
			ws.WinRate = float64(ws.Wins) / float64(ws.TotalPicks)
		}
		stats.WeeklyPerformance = append(stats.WeeklyPerformance, *ws)
	}
	
	// Sort by week
	sort.Slice(stats.WeeklyPerformance, func(i, j int) bool {
		return stats.WeeklyPerformance[i].Week < stats.WeeklyPerformance[j].Week
	})
}

// analyzeTeamPerformance analyzes performance when picking for/against teams
func (s *AnalyticsService) analyzeTeamPerformance(picks []models.Pick, gameMap map[int]*models.Game, stats *UserPerformanceStats) {
	for _, pick := range picks {
		game := gameMap[pick.GameID]
		if game == nil {
			continue
		}
		
		// Determine which team was picked and which was picked against
		var pickedTeam, opposingTeam string
		if pick.PickType == models.PickTypeSpread || pick.PickType == models.PickTypeMoneyline {
			// For spread/moneyline, determine based on TeamID
			homeTeamID := s.getTeamIDFromAbbreviation(game.Home)
			if pick.TeamID == homeTeamID {
				pickedTeam = game.Home
				opposingTeam = game.Away
			} else {
				pickedTeam = game.Away
				opposingTeam = game.Home
			}
			
			// Update picked team stats
			if stats.TeamPerformance[pickedTeam].TeamAbbr == "" {
				stats.TeamPerformance[pickedTeam] = TeamPickStats{TeamAbbr: pickedTeam}
			}
			teamStats := stats.TeamPerformance[pickedTeam]
			teamStats.PicksFor++
			if pick.Result == models.PickResultWin {
				teamStats.WinsFor++
			}
			stats.TeamPerformance[pickedTeam] = teamStats
			
			// Update opposing team stats  
			if stats.TeamPerformance[opposingTeam].TeamAbbr == "" {
				stats.TeamPerformance[opposingTeam] = TeamPickStats{TeamAbbr: opposingTeam}
			}
			oppStats := stats.TeamPerformance[opposingTeam] 
			oppStats.PicksAgainst++
			if pick.Result == models.PickResultWin {
				oppStats.WinsAgainst++
			}
			stats.TeamPerformance[opposingTeam] = oppStats
		}
	}
	
	// Calculate win rates
	for team, teamStats := range stats.TeamPerformance {
		if teamStats.PicksFor > 0 {
			teamStats.WinRateFor = float64(teamStats.WinsFor) / float64(teamStats.PicksFor)
		}
		if teamStats.PicksAgainst > 0 {
			teamStats.WinRateAgainst = float64(teamStats.WinsAgainst) / float64(teamStats.PicksAgainst)
		}
		stats.TeamPerformance[team] = teamStats
	}
}

// analyzeOverUnderPerformance analyzes over/under betting patterns
func (s *AnalyticsService) analyzeOverUnderPerformance(picks []models.Pick, gameMap map[int]*models.Game, stats *UserPerformanceStats) {
	analysis := OverUnderAnalysis{}
	
	overWins := 0
	underWins := 0
	var totalSum float64
	var minTotal, maxTotal float64 = math.MaxFloat64, 0
	
	for _, pick := range picks {
		if pick.PickType != models.PickTypeOverUnder {
			continue
		}
		
		game := gameMap[pick.GameID]
		if game == nil || !game.HasOdds() {
			continue
		}
		
		totalSum += game.Odds.OU
		if game.Odds.OU < minTotal {
			minTotal = game.Odds.OU
		}
		if game.Odds.OU > maxTotal {
			maxTotal = game.Odds.OU
		}
		
		if pick.TeamID == 99 { // Over
			analysis.TotalOverPicks++
			if pick.Result == models.PickResultWin {
				overWins++
			}
		} else if pick.TeamID == 98 { // Under
			analysis.TotalUnderPicks++
			if pick.Result == models.PickResultWin {
				underWins++
			}
		}
	}
	
	totalOUPicks := analysis.TotalOverPicks + analysis.TotalUnderPicks
	if totalOUPicks > 0 {
		analysis.AverageTotal = totalSum / float64(totalOUPicks)
		analysis.HighestTotal = maxTotal
		analysis.LowestTotal = minTotal
	}
	
	if analysis.TotalOverPicks > 0 {
		analysis.OverWinRate = float64(overWins) / float64(analysis.TotalOverPicks)
	}
	if analysis.TotalUnderPicks > 0 {
		analysis.UnderWinRate = float64(underWins) / float64(analysis.TotalUnderPicks)
	}
	
	stats.OverUnderAnalysis = analysis
}

// analyzeSpreadPerformance analyzes spread betting patterns
func (s *AnalyticsService) analyzeSpreadPerformance(picks []models.Pick, gameMap map[int]*models.Game, stats *UserPerformanceStats) {
	analysis := SpreadAnalysis{}
	
	var favoriteWins, favoriteTotal int
	var underdogWins, underdogTotal int
	var homeWins, homeTotal int
	var awayWins, awayTotal int
	var largeSpreadWins, largeSpreadTotal int
	var spreadSum float64
	var spreadCount int
	
	for _, pick := range picks {
		if pick.PickType != models.PickTypeSpread {
			continue
		}
		
		game := gameMap[pick.GameID]
		if game == nil || !game.HasOdds() {
			continue
		}
		
		spreadSum += math.Abs(game.Odds.Spread)
		spreadCount++
		
		// Determine if pick was home/away
		homeTeamID := s.getTeamIDFromAbbreviation(game.Home)
		isHomePick := pick.TeamID == homeTeamID
		
		if isHomePick {
			homeTotal++
			if pick.Result == models.PickResultWin {
				homeWins++
			}
		} else {
			awayTotal++
			if pick.Result == models.PickResultWin {
				awayWins++
			}
		}
		
		// Determine if favorite/underdog (spread < 0 means home favored)
		isFavoriteHome := game.Odds.Spread < 0
		isPickingFavorite := (isFavoriteHome && isHomePick) || (!isFavoriteHome && !isHomePick)
		
		if isPickingFavorite {
			favoriteTotal++
			if pick.Result == models.PickResultWin {
				favoriteWins++
			}
		} else {
			underdogTotal++
			if pick.Result == models.PickResultWin {
				underdogWins++
			}
		}
		
		// Large spread analysis (>7 points)
		if math.Abs(game.Odds.Spread) > 7.0 {
			largeSpreadTotal++
			if pick.Result == models.PickResultWin {
				largeSpreadWins++
			}
		}
	}
	
	// Calculate rates
	if favoriteTotal > 0 {
		analysis.FavoriteWinRate = float64(favoriteWins) / float64(favoriteTotal)
	}
	if underdogTotal > 0 {
		analysis.UnderdogWinRate = float64(underdogWins) / float64(underdogTotal)
	}
	if homeTotal > 0 {
		analysis.HomeWinRate = float64(homeWins) / float64(homeTotal)
	}
	if awayTotal > 0 {
		analysis.AwayWinRate = float64(awayWins) / float64(awayTotal)
	}
	if largeSpreadTotal > 0 {
		analysis.LargeSpreadWinRate = float64(largeSpreadWins) / float64(largeSpreadTotal)
	}
	if spreadCount > 0 {
		analysis.AverageSpread = spreadSum / float64(spreadCount)
	}
	
	stats.SpreadAnalysis = analysis
}

// analyzeRecentForm analyzes recent performance trends
func (s *AnalyticsService) analyzeRecentForm(picks []models.Pick, stats *UserPerformanceStats) {
	// Sort picks by date (assuming they're chronological by Week)
	sort.Slice(picks, func(i, j int) bool {
		return picks[i].Week < picks[j].Week
	})
	
	form := RecentFormStats{}
	
	// Calculate last 5 and last 10 win rates
	if len(picks) >= 5 {
		last5 := picks[len(picks)-5:]
		wins5 := 0
		for _, pick := range last5 {
			if pick.Result == models.PickResultWin {
				wins5++
			}
		}
		form.Last5WinRate = float64(wins5) / 5.0
	}
	
	if len(picks) >= 10 {
		last10 := picks[len(picks)-10:]
		wins10 := 0
		for _, pick := range last10 {
			if pick.Result == models.PickResultWin {
				wins10++
			}
		}
		form.Last10WinRate = float64(wins10) / 10.0
	}
	
	// Calculate current streaks
	currentStreak := 0
	lastResult := models.PickResultPending
	
	for i := len(picks) - 1; i >= 0; i-- {
		pick := picks[i]
		if pick.Result == models.PickResultPending || pick.Result == models.PickResultPush {
			continue // Skip pending/push picks for streak calculation
		}
		
		if lastResult == models.PickResultPending {
			lastResult = pick.Result
			currentStreak = 1
		} else if pick.Result == lastResult {
			currentStreak++
		} else {
			break
		}
	}
	
	if lastResult == models.PickResultWin {
		form.HotStreak = currentStreak
	} else if lastResult == models.PickResultLoss {
		form.ColdStreak = currentStreak
	}
	
	// Determine trend
	if len(picks) >= 10 {
		if form.Last5WinRate > form.Last10WinRate {
			form.Trend = "improving"
		} else if form.Last5WinRate < form.Last10WinRate {
			form.Trend = "declining"
		} else {
			form.Trend = "stable"
		}
	}
	
	stats.RecentForm = form
}

// getTeamIDFromAbbreviation maps team abbreviation to ESPN team ID
func (s *AnalyticsService) getTeamIDFromAbbreviation(abbr string) int {
	teamMap := map[string]int{
		"ARI": 22, "ATL": 1,  "BAL": 33, "BUF": 2,  "CAR": 29, "CHI": 3,
		"CIN": 4,  "CLE": 5,  "DAL": 6,  "DEN": 7,  "DET": 8,  "GB":  9,
		"HOU": 34, "IND": 11, "JAX": 30, "KC":  12, "LV":  13, "LAC": 24,
		"LAR": 14, "MIA": 15, "MIN": 16, "NE":  17, "NO":  18, "NYG": 19,
		"NYJ": 20, "PHI": 21, "PIT": 23, "SF":  25, "SEA": 26, "TB":  27,
		"TEN": 10, "WSH": 28,
	}
	
	if id, exists := teamMap[abbr]; exists {
		return id
	}
	return 0
}

// GetLeagueStats returns overall league statistics for a season
func (s *AnalyticsService) GetLeagueStats(ctx context.Context, season int) (*LeagueStats, error) {
	// Get all picks for the season
	picks, err := s.pickRepo.GetPicksBySeason(ctx, season)
	if err != nil {
		return nil, fmt.Errorf("failed to get picks: %w", err)
	}

	stats := &LeagueStats{
		Season:          season,
		TotalPicks:      len(picks),
		ResultBreakdown: make(map[models.PickResult]int),
		TypeBreakdown:   make(map[models.PickType]int),
		WeeklyStats:     make(map[int]WeekStats),
	}

	// Analyze all picks
	for _, pick := range picks {
		stats.ResultBreakdown[pick.Result]++
		stats.TypeBreakdown[pick.PickType]++
		
		// Weekly stats
		if stats.WeeklyStats[pick.Week].Week == 0 {
			stats.WeeklyStats[pick.Week] = WeekStats{Week: pick.Week}
		}
		weekStats := stats.WeeklyStats[pick.Week]
		weekStats.TotalPicks++
		weekStats.Results[pick.Result]++
		stats.WeeklyStats[pick.Week] = weekStats
	}

	// Calculate overall win rate
	if wins, exists := stats.ResultBreakdown[models.PickResultWin]; exists && stats.TotalPicks > 0 {
		stats.OverallWinRate = float64(wins) / float64(stats.TotalPicks)
	}

	return stats, nil
}

// LeagueStats represents league-wide statistics
type LeagueStats struct {
	Season          int                             `json:"season"`
	TotalPicks      int                             `json:"total_picks"`
	OverallWinRate  float64                         `json:"overall_win_rate"`
	ResultBreakdown map[models.PickResult]int       `json:"result_breakdown"`
	TypeBreakdown   map[models.PickType]int         `json:"type_breakdown"`
	WeeklyStats     map[int]WeekStats               `json:"weekly_stats"`
}

// WeekStats represents statistics for a specific week league-wide
type WeekStats struct {
	Week       int                           `json:"week"`
	TotalPicks int                           `json:"total_picks"`
	Results    map[models.PickResult]int     `json:"results"`
}

// GetGameAnalytics returns detailed analytics for a specific game
func (s *AnalyticsService) GetGameAnalytics(ctx context.Context, gameID int) (*GameAnalytics, error) {
	// Get game information
	game, err := s.gameRepo.FindByESPNID(ctx, gameID)
	if err != nil {
		return nil, fmt.Errorf("failed to get game: %w", err)
	}

	// Get all picks for this game
	picks, err := s.pickRepo.GetPicksByGameID(ctx, gameID)
	if err != nil {
		return nil, fmt.Errorf("failed to get picks: %w", err)
	}

	analytics := &GameAnalytics{
		GameID:        gameID,
		Game:          game,
		TotalPicks:    len(picks),
		PicksByType:   make(map[models.PickType]int),
		PicksByResult: make(map[models.PickResult]int),
		UserPicks:     make([]UserGamePick, 0),
	}

	// Analyze picks
	for _, pick := range picks {
		analytics.PicksByType[pick.PickType]++
		analytics.PicksByResult[pick.Result]++
		
		// Get user info
		user, err := s.userRepo.FindByID(ctx, pick.UserID)
		if err == nil {
			userPick := UserGamePick{
				UserID:     pick.UserID,
				UserName:   user.Name,
				PickType:   pick.PickType,
				TeamID:     pick.TeamID,
				Result:     pick.Result,
				TeamName:   pick.TeamName,
			}
			analytics.UserPicks = append(analytics.UserPicks, userPick)
		}
	}

	// Calculate public pick percentages
	if analytics.TotalPicks > 0 {
		analytics.PublicPickPercentages = make(map[string]float64)
		
		for pickType, count := range analytics.PicksByType {
			percentage := float64(count) / float64(analytics.TotalPicks) * 100
			analytics.PublicPickPercentages[string(pickType)] = percentage
		}
	}

	return analytics, nil
}

// GameAnalytics represents detailed analytics for a specific game
type GameAnalytics struct {
	GameID                int                         `json:"game_id"`
	Game                  *models.Game                `json:"game"`
	TotalPicks            int                         `json:"total_picks"`
	PicksByType           map[models.PickType]int     `json:"picks_by_type"`
	PicksByResult         map[models.PickResult]int   `json:"picks_by_result"`
	PublicPickPercentages map[string]float64          `json:"public_pick_percentages"`
	UserPicks             []UserGamePick              `json:"user_picks"`
}

// UserGamePick represents a user's pick on a specific game
type UserGamePick struct {
	UserID   int                `json:"user_id"`
	UserName string             `json:"user_name"`
	PickType models.PickType    `json:"pick_type"`
	TeamID   int                `json:"team_id"`
	TeamName string             `json:"team_name"`
	Result   models.PickResult  `json:"result"`
}