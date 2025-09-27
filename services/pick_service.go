package services

import (
	"context"
	"fmt"
	"runtime"
	"sort"
	"time"

	"go.mongodb.org/mongo-driver/bson/primitive"

	"nfl-app-go/database"
	"nfl-app-go/logging"
	"nfl-app-go/models"
)

// PickService handles business logic for picks
// SSEBroadcaster interface for sending SSE updates
type SSEBroadcaster interface {
	BroadcastToAllClients(eventType, data string)
}

type PickService struct {
	weeklyPicksRepo *database.MongoWeeklyPicksRepository
	gameRepo        *database.MongoGameRepository
	userRepo        *database.MongoUserRepository
	parlayRepo      *database.MongoParlayRepository
	broadcaster     SSEBroadcaster

	// Specialized services for delegation
	parlayService     *ParlayService
	resultCalcService *ResultCalculationService
	analyticsService  *AnalyticsService
}

// NewPickService creates a new pick service
func NewPickService(weeklyPicksRepo *database.MongoWeeklyPicksRepository, gameRepo *database.MongoGameRepository, userRepo *database.MongoUserRepository, parlayRepo *database.MongoParlayRepository) *PickService {
	return &PickService{
		weeklyPicksRepo: weeklyPicksRepo,
		gameRepo:        gameRepo,
		userRepo:        userRepo,
		parlayRepo:      parlayRepo,
	}
}

// SetBroadcaster sets the SSE broadcaster for real-time updates
func (s *PickService) SetBroadcaster(broadcaster SSEBroadcaster) {
	s.broadcaster = broadcaster
}

// SetSpecializedServices sets the specialized services for delegation
func (s *PickService) SetSpecializedServices(parlayService *ParlayService, resultCalcService *ResultCalculationService, analyticsService *AnalyticsService) {
	s.parlayService = parlayService
	s.resultCalcService = resultCalcService
	s.analyticsService = analyticsService
}

// CreatePick creates a new pick with validation
func (s *PickService) CreatePick(ctx context.Context, userID, gameID, teamID, season, week int) (*models.Pick, error) {
	logger := logging.WithPrefix("PickService")
	// Validate game exists
	game, err := s.gameRepo.FindByESPNID(ctx, gameID)
	if err != nil {
		return nil, fmt.Errorf("failed to validate game: %w", err)
	}
	if game == nil {
		return nil, fmt.Errorf("game %d not found", gameID)
	}

	// Validate user exists
	user, err := s.userRepo.GetUserByID(userID)
	if err != nil {
		return nil, fmt.Errorf("failed to validate user: %w", err)
	}
	if user == nil {
		return nil, fmt.Errorf("user %d not found", userID)
	}

	// Create pick using the model's helper function
	pick := models.CreatePickFromLegacyData(userID, gameID, teamID, season, week)

	// TODO: Store in database using WeeklyPicks repository
	// For now, just return the created pick without storing
	// if err := s.weeklyPicksRepo.Create(ctx, pick); err != nil {
	// 	return nil, fmt.Errorf("failed to create pick: %w", err)
	// }

	logger.Infof("Created pick: User %d, Game %d, Team %d, Season %d, Week %d",
		userID, gameID, teamID, season, week)

	return pick, nil
}

// GetUserPicksForWeek retrieves a user's picks for a specific week with organized structure
func (s *PickService) GetUserPicksForWeek(ctx context.Context, userID, season, week int) (*models.UserPicks, error) {
	logger := logging.WithPrefix("PickService")

	// Get weekly picks document
	weeklyPicks, err := s.weeklyPicksRepo.FindByUserAndWeek(ctx, userID, season, week)
	if err != nil {
		return nil, fmt.Errorf("failed to get weekly picks: %w", err)
	}

	// If no picks found, return empty UserPicks
	if weeklyPicks == nil {
		// Get user info for empty response
		user, err := s.userRepo.GetUserByID(userID)
		if err != nil {
			return nil, fmt.Errorf("failed to get user info: %w", err)
		}
		if user == nil {
			return nil, fmt.Errorf("user not found")
		}

		return &models.UserPicks{
			UserID:   userID,
			UserName: user.Name,
			Picks:    []models.Pick{},
			Record:   models.UserRecord{},
		}, nil
	}

	// Get user info
	user, err := s.userRepo.GetUserByID(userID)
	if err != nil {
		return nil, fmt.Errorf("failed to get user info: %w", err)
	}
	if user == nil {
		return nil, fmt.Errorf("user not found")
	}

	// Convert WeeklyPicks to UserPicks
	userPicks := weeklyPicks.ToUserPicks()
	userPicks.UserName = user.Name

	// TODO: Calculate user record for the season (will need to aggregate across all weeks)
	// For now, return empty record
	userPicks.Record = models.UserRecord{}

	// Get game information for categorizing picks by day
	gameInfoMap, err := s.getGameInfoForWeek(ctx, season, week)
	if err != nil {
		logger.Warnf("Failed to get game info for pick categorization: %v", err)
		gameInfoMap = make(map[int]models.GameDayInfo) // Empty map as fallback
	}

	// Enrich picks with game data and categorize them
	for i := range userPicks.Picks {
		pick := &userPicks.Picks[i]

		// IMPORTANT: Enrich pick with game data to populate TeamName and other display fields
		// This fixes the issue where picks show help.svg on first load due to empty TeamName
		if err := s.EnrichPickWithGameData(pick); err != nil {
			logger.Warnf("Failed to enrich pick for game %d: %v", pick.GameID, err)
			// Continue with unenriched pick rather than failing completely
		}

		// Categorize by pick type
		switch pick.PickType {
		case models.PickTypeSpread:
			userPicks.SpreadPicks = append(userPicks.SpreadPicks, *pick)
		case models.PickTypeOverUnder:
			userPicks.OverUnderPicks = append(userPicks.OverUnderPicks, *pick)
		}

		// Categorize by game day for bonus weeks
		if gameInfo, exists := gameInfoMap[pick.GameID]; exists {
			switch gameInfo.Category {
			case models.ParlayBonusThursday:
				userPicks.BonusThursdayPicks = append(userPicks.BonusThursdayPicks, *pick)
			case models.ParlayBonusFriday:
				userPicks.BonusFridayPicks = append(userPicks.BonusFridayPicks, *pick)
			}
		}
	}

	logger.Debugf("Retrieved %d picks for user %d, season %d, week %d", len(userPicks.Picks), userID, season, week)

	// Get games for this week to support daily grouping and sorting
	games, err := s.gameRepo.GetGamesByWeekSeason(week, season)
	if err != nil {
		logger.Warnf("Failed to get games for sorting: %v", err)
	} else {
		// Convert to slice and create game lookup map for sorting
		gameSlice := make([]models.Game, len(games))
		gameMap := make(map[int]models.Game)
		for i, game := range games {
			gameSlice[i] = *game
			gameMap[game.ID] = *game
		}

		// Sort all picks by game start time
		s.sortPicksByGameTime(userPicks.Picks, gameMap)
		s.sortPicksByGameTime(userPicks.SpreadPicks, gameMap)
		s.sortPicksByGameTime(userPicks.OverUnderPicks, gameMap)
		s.sortPicksByGameTime(userPicks.BonusThursdayPicks, gameMap)
		s.sortPicksByGameTime(userPicks.BonusFridayPicks, gameMap)

		// MODERN: Populate daily pick groups for 2025+ seasons
		if models.IsModernSeason(season) {
			userPicks.PopulateDailyPickGroups(gameSlice, season)

			// Sort picks within each daily group by game start time
			for date, dayPicks := range userPicks.DailyPickGroups {
				s.sortPicksByGameTime(dayPicks, gameMap)
				userPicks.DailyPickGroups[date] = dayPicks
			}
		}
	}

	return userPicks, nil
}

// GetAllUserPicksForWeek retrieves all users' picks for a specific week
func (s *PickService) GetAllUserPicksForWeek(ctx context.Context, season, week int) ([]*models.UserPicks, error) {
	logger := logging.WithPrefix("PickService")

	// Check for invalid data and print stack trace
	if season == 0 || week == 0 {
		logger.Errorf("Invalid GetAllUserPicksForWeek params - Season=%d, Week=%d", season, week)

		// Print stack trace to identify caller
		buf := make([]byte, 4096)
		n := runtime.Stack(buf, false)
		logger.Errorf("GetAllUserPicksForWeek stack trace:\n%s", buf[:n])
	}

	// Get all weekly picks for the week using new repository
	allWeeklyPicks, err := s.weeklyPicksRepo.FindAllByWeek(ctx, season, week)
	if err != nil {
		return nil, fmt.Errorf("failed to get weekly picks: %w", err)
	}

	logger.Infof("Found %d users with picks for season %d, week %d", len(allWeeklyPicks), season, week)

	var userPicksList []*models.UserPicks
	for _, weeklyPicks := range allWeeklyPicks {
		// Get user info
		user, err := s.userRepo.GetUserByID(weeklyPicks.UserID)
		if err != nil {
			logger.Errorf("Failed to get user info for user %d: %v", weeklyPicks.UserID, err)
			continue
		}
		if user == nil {
			logger.Warnf("User %d not found, skipping", weeklyPicks.UserID)
			continue
		}

		// Convert to UserPicks
		userPicks := weeklyPicks.ToUserPicks()
		userPicks.UserName = user.Name

		// TODO: Calculate user record for the season (will need to aggregate across all weeks)
		userPicks.Record = models.UserRecord{}

		userPicksList = append(userPicksList, userPicks)
		logger.Debugf("User %s has %d picks", user.Name, len(userPicks.Picks))
	}

	// Get game information for categorizing picks by day
	gameInfoMap, err := s.getGameInfoForWeek(ctx, season, week)
	if err != nil {
		logger.Warnf("Failed to get game info for pick categorization: %v", err)
		gameInfoMap = make(map[int]models.GameDayInfo) // Empty map as fallback
	}

	// Categorize picks for each user
	for _, userPicksData := range userPicksList {
		// Get cumulative parlay points up to the current week (not entire season)
		cumulativeParlayTotal, err := s.parlayRepo.GetUserCumulativeScoreUpToWeek(ctx, userPicksData.UserID, season, week)
		if err != nil {
			logger.Warnf("Failed to get cumulative parlay total for user %d season %d week %d: %v", userPicksData.UserID, season, week, err)
		} else {
			userPicksData.Record.ParlayPoints = cumulativeParlayTotal
		}

		// Get parlay points for this specific week
		weekParlayScore, err := s.parlayRepo.GetUserParlayScore(ctx, userPicksData.UserID, season, week)
		if err != nil {
			logger.Warnf("Failed to get weekly parlay score for user %d week %d: %v", userPicksData.UserID, week, err)
		} else if weekParlayScore != nil {
			userPicksData.Record.WeeklyPoints = weekParlayScore.TotalPoints
		}

		// Enrich picks and categorize them by game day for bonus weeks
		for i := range userPicksData.Picks {
			pick := &userPicksData.Picks[i]

			// IMPORTANT: Enrich pick with game data to populate TeamName and other display fields
			// This fixes the issue where picks show help.svg on first load due to empty TeamName
			if err := s.EnrichPickWithGameData(pick); err != nil {
				logger.Warnf("Failed to enrich pick for game %d: %v", pick.GameID, err)
				// Continue with unenriched pick rather than failing completely
			}

			if gameInfo, exists := gameInfoMap[pick.GameID]; exists {
				switch gameInfo.Category {
				case models.ParlayBonusThursday:
					userPicksData.BonusThursdayPicks = append(userPicksData.BonusThursdayPicks, *pick)
				case models.ParlayBonusFriday:
					userPicksData.BonusFridayPicks = append(userPicksData.BonusFridayPicks, *pick)
				}
			}

			// Also categorize by pick type
			switch pick.PickType {
			case models.PickTypeSpread:
				userPicksData.SpreadPicks = append(userPicksData.SpreadPicks, *pick)
			case models.PickTypeOverUnder:
				userPicksData.OverUnderPicks = append(userPicksData.OverUnderPicks, *pick)
			}
		}
	}

	// Get games for this week to support daily grouping and sorting
	games, err := s.gameRepo.GetGamesByWeekSeason(week, season)
	if err != nil {
		logger.Warnf("Failed to get games for sorting: %v", err)
	} else {
		// Convert to slice and create game lookup map for sorting
		gameSlice := make([]models.Game, len(games))
		gameMap := make(map[int]models.Game)
		for i, game := range games {
			gameSlice[i] = *game
			gameMap[game.ID] = *game
		}

		// Sort all picks by game start time for each user
		for _, userPicks := range userPicksList {
			s.sortPicksByGameTime(userPicks.Picks, gameMap)
			s.sortPicksByGameTime(userPicks.SpreadPicks, gameMap)
			s.sortPicksByGameTime(userPicks.OverUnderPicks, gameMap)
		}

		// MODERN: Populate daily pick groups for 2025+ seasons
		if models.IsModernSeason(season) {
			// Populate daily groups for each user
			for _, userPicks := range userPicksList {
				logger.Debugf("BEFORE PopulateDailyPickGroups - User %s has %d picks, DailyPickGroups: %v", userPicks.UserName, len(userPicks.Picks), userPicks.DailyPickGroups != nil)
				userPicks.PopulateDailyPickGroups(gameSlice, season)
				logger.Debugf("AFTER PopulateDailyPickGroups - User %s DailyPickGroups has %d groups", userPicks.UserName, len(userPicks.DailyPickGroups))

				// Sort picks within each daily group by game start time
				for date, dayPicks := range userPicks.DailyPickGroups {
					s.sortPicksByGameTime(dayPicks, gameMap)
					userPicks.DailyPickGroups[date] = dayPicks
					logger.Debugf("User %s - Date %s has %d picks (sorted)", userPicks.UserName, date, len(dayPicks))
				}
			}
		}
	}

	// Sort by user name
	sort.Slice(userPicksList, func(i, j int) bool {
		return userPicksList[i].UserName < userPicksList[j].UserName
	})

	return userPicksList, nil
}

// UpdatePickResult updates the result of a pick (called when games are completed)
// NOTE: This method is legacy and may not work with new WeeklyPicks model
func (s *PickService) UpdatePickResult(ctx context.Context, pickID primitive.ObjectID, result models.PickResult) error {
	// TODO: Implement with WeeklyPicks repository
	return fmt.Errorf("UpdatePickResult not implemented with WeeklyPicks model - use UpdatePickResultsByGame instead")
}

// UpdatePickResultsByGame updates pick results for all users who have picks on a specific game
func (s *PickService) UpdatePickResultsByGame(ctx context.Context, season, week, gameID int, pickResults map[int]models.PickResult) error {
	logger := logging.WithPrefix("PickService")

	// Use the WeeklyPicksRepository's UpdatePickResults method
	if err := s.weeklyPicksRepo.UpdatePickResults(ctx, season, week, gameID, pickResults); err != nil {
		return fmt.Errorf("failed to update pick results for game %d: %w", gameID, err)
	}

	logger.Infof("Updated pick results for game %d: %d users affected", gameID, len(pickResults))
	return nil
}

// ProcessGameCompletion processes all picks for a completed game and calculates results
func (s *PickService) ProcessGameCompletion(ctx context.Context, game *models.Game) error {
	logger := logging.WithPrefix("PickService")
	if !game.IsCompleted() {
		return fmt.Errorf("game %d is not completed", game.ID)
	}

	// Get all picks for this game using new method
	allPicks, err := s.GetAllPicksForWeek(ctx, game.Season, game.Week)
	if err != nil {
		return fmt.Errorf("failed to get picks for week %d: %w", game.Week, err)
	}

	// Filter picks for this specific game
	var picks []models.Pick
	for _, pick := range allPicks {
		if pick.GameID == game.ID {
			picks = append(picks, pick)
		}
	}

	logger.Infof("Processing %d picks for completed game %d", len(picks), game.ID)

	// Calculate results for all picks and collect them by UserID
	pickResults := make(map[int]models.PickResult)
	for _, pick := range picks {
		// Calculate pick result using specialized service
		result := s.resultCalcService.CalculatePickResult(&pick, game)

		// Store result mapped by UserID
		pickResults[pick.UserID] = result

		logger.Infof("Calculated pick result for user %d, game %d: %s", pick.UserID, pick.GameID, result)
	}

	// Update all pick results in one batch operation
	if len(pickResults) > 0 {
		if err := s.UpdatePickResultsByGame(ctx, game.Season, game.Week, game.ID, pickResults); err != nil {
			return fmt.Errorf("failed to update pick results: %w", err)
		}
	}

	return nil
}

// GetPickStats returns statistics about picks in the system
func (s *PickService) GetPickStats(ctx context.Context) (map[string]interface{}, error) {
	totalWeeklyPicksDocuments, err := s.weeklyPicksRepo.Count(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to count weekly picks documents: %w", err)
	}

	stats := map[string]interface{}{
		"total_weekly_picks_documents": totalWeeklyPicksDocuments,
		"last_updated":                 time.Now(),
	}

	// Add season-specific stats
	seasons := []int{2023, 2024, 2025}
	for _, season := range seasons {
		// This would require additional aggregation queries to get detailed stats per season
		stats[fmt.Sprintf("season_%d", season)] = fmt.Sprintf("Season %d data", season)
	}

	return stats, nil
}

// EnrichPickWithGameData populates the display fields of a pick with game information
func (s *PickService) EnrichPickWithGameData(pick *models.Pick) error {
	logger := logging.WithPrefix("PickService")
	// Get the game information
	game, err := s.gameRepo.FindByESPNID(context.Background(), pick.GameID)
	if err != nil {
		return fmt.Errorf("failed to find game %d: %w", pick.GameID, err)
	}

	if game == nil {
		return fmt.Errorf("game %d not found", pick.GameID)
	}

	// Set game description with status
	gameStatus := s.getGameStatusDescription(game)
	pick.GameDescription = fmt.Sprintf("%s @ %s (%s)", game.Away, game.Home, gameStatus)

	// Determine team name based on pick type and team ID

	switch pick.PickType {
	case models.PickTypeOverUnder:
		if pick.TeamID == 98 {
			pick.TeamName = "Under"
			if game.Odds != nil && game.Odds.OU > 0 {
				pick.TeamName = fmt.Sprintf("Under %.1f", game.Odds.OU)
			}
		} else if pick.TeamID == 99 {
			pick.TeamName = "Over"
			if game.Odds != nil && game.Odds.OU > 0 {
				pick.TeamName = fmt.Sprintf("Over %.1f", game.Odds.OU)
			}
		} else {
		}
	case models.PickTypeSpread:
		// Get team abbreviation from ID mapping
		teamAbbr := s.getTeamNameFromID(pick.TeamID, game)

		// Add spread information if available
		hasOdds := game.Odds != nil
		hasSpread := hasOdds && game.Odds.Spread != 0
		if hasOdds {
		}

		if hasSpread {
			// Determine if this team is home or away to show correct spread
			isHome := teamAbbr == game.Home

			if isHome {
				// Home team spread
				if game.Odds.Spread > 0 {
					pick.TeamName = fmt.Sprintf("%s +%.1f", teamAbbr, game.Odds.Spread)
				} else {
					pick.TeamName = fmt.Sprintf("%s %.1f", teamAbbr, game.Odds.Spread)
				}
			} else {
				// Away team spread (inverse of home spread)
				awaySpread := -game.Odds.Spread
				if awaySpread > 0 {
					pick.TeamName = fmt.Sprintf("%s +%.1f", teamAbbr, awaySpread)
				} else {
					pick.TeamName = fmt.Sprintf("%s %.1f", teamAbbr, awaySpread)
				}
			}
		} else {
			pick.TeamName = teamAbbr
		}
	default:
	}

	// Update pick result based on game status
	if game.State == models.GameStateCompleted {
		calculatedResult := s.calculatePickResult(pick, game)
		if calculatedResult != pick.Result {
			// TODO: Update result in WeeklyPicks repository
			logger.Infof("Pick result calculated: %s -> %s (not updated in DB yet)", pick.Result, calculatedResult)
			pick.Result = calculatedResult
		}
	} else if game.State == models.GameStateInPlay {
		pick.Result = models.PickResultPending
	} else {
		pick.Result = models.PickResultPending
	}

	// Set complete pick description
	pick.PickDescription = fmt.Sprintf("%s - %s", pick.GameDescription, pick.TeamName)

	return nil
}

// getTeamNameFromID attempts to map team ID to team abbreviation
// This is a heuristic based on common ESPN team ID patterns
func (s *PickService) getTeamNameFromID(teamID int, game *models.Game) string {
	logger := logging.WithPrefix("PickService")
	// Team ID mappings from legacy system - the definitive mapping
	espnTeamMap := map[int]string{
		0: "PSH", 1: "ATL", 2: "BUF", 3: "CHI", 4: "CIN", 5: "CLE", 6: "DAL", 7: "DEN", 8: "DET",
		9: "GB", 10: "TEN", 11: "IND", 12: "KC", 13: "LV", 14: "LAR", 15: "MIA", 16: "MIN",
		17: "NE", 18: "NO", 19: "NYG", 20: "NYJ", 21: "PHI", 22: "ARI", 23: "PIT", 24: "LAC",
		25: "SF", 26: "SEA", 27: "TB", 28: "WSH", 29: "CAR", 30: "JAX", 33: "BAL", 34: "HOU",
		// Over/Under special cases
		98: "UND", 99: "OVR",
	}

	if abbr, exists := espnTeamMap[teamID]; exists {
		// Verify this team is actually in the game
		if abbr == game.Away || abbr == game.Home {
			return abbr
		}

		// Special case for Washington - handle both WAS and WSH
		if abbr == "WSH" && (game.Away == "WAS" || game.Home == "WAS") {
			return "WAS"
		}
		if abbr == "WAS" && (game.Away == "WSH" || game.Home == "WSH") {
			return "WSH"
		}
	} else {
	}

	// Debug logging for unmatched teams
	if teamID == 28 { // Washington
		logger.Warnf("Washington team ID 28 not matched - Game: %s @ %s, Expected: WSH", game.Away, game.Home)
	}

	// Fallback: return the team ID as string
	fallback := fmt.Sprintf("Team%d", teamID)
	return fallback
}

// getGameStatusDescription returns a human-readable game status
func (s *PickService) getGameStatusDescription(game *models.Game) string {
	switch game.State {
	case models.GameStateScheduled:
		return "Scheduled"
	case models.GameStateInPlay:
		if game.Quarter > 0 {
			return fmt.Sprintf("Q%d", game.Quarter)
		}
		return "Live"
	case models.GameStateCompleted:
		return "Final"
	case models.GameStatePostponed:
		return "Postponed"
	default:
		return "Unknown"
	}
}

// calculatePickResult determines if a pick won, lost, or pushed based on game outcome
func (s *PickService) calculatePickResult(pick *models.Pick, game *models.Game) models.PickResult {
	// Delegate to specialized ResultCalculationService
	return s.resultCalcService.CalculatePickResult(pick, game)
}

// CalculateUserParlayScore calculates parlay club points for a user in a specific week
// Handles bonus weeks separately from regular weekend picks
func (s *PickService) CalculateUserParlayScore(ctx context.Context, userID, season, week int) (map[models.ParlayCategory]int, error) {
	// Delegate to specialized ParlayService
	return s.parlayService.CalculateUserParlayScore(ctx, userID, season, week)
}

// CalculateAllUsersParlayScores calculates parlay scores for all users in a week
func (s *PickService) CalculateAllUsersParlayScores(ctx context.Context, season, week int) (map[int]map[models.ParlayCategory]int, error) {
	logger := logging.WithPrefix("PickService")
	// Get all users
	users, err := s.userRepo.GetAllUsers()
	if err != nil {
		return nil, fmt.Errorf("failed to get users: %w", err)
	}

	result := make(map[int]map[models.ParlayCategory]int)

	for _, user := range users {
		scores, err := s.CalculateUserParlayScore(ctx, user.ID, season, week)
		if err != nil {
			logger.Warnf("Failed to calculate parlay score for user %d: %v", user.ID, err)
			continue
		}
		result[user.ID] = scores
	}

	return result, nil
}

// UpdateUserParlayRecord updates a user's parlay record with weekly points
func (s *PickService) UpdateUserParlayRecord(ctx context.Context, userID, season, week int, weeklyScores map[models.ParlayCategory]int) error {
	logger := logging.WithPrefix("PickService")
	// Create parlay score entry
	parlayScore := models.CreateParlayScore(userID, season, week, weeklyScores)

	// DEBUG: Log before saving parlay score
	logger.Debugf("UpdateUserParlayRecord about to save - UserID=%d, Season=%d, Week=%d",
		userID, season, week)

	// Save to database
	if err := s.parlayRepo.UpsertParlayScore(ctx, parlayScore); err != nil {
		return fmt.Errorf("failed to save parlay score: %w", err)
	}

	logger.Infof("User %d earned %d parlay points in week %d (Regular: %d, Thu: %d, Fri: %d)",
		userID, parlayScore.TotalPoints, week,
		weeklyScores[models.ParlayRegular],
		weeklyScores[models.ParlayBonusThursday],
		weeklyScores[models.ParlayBonusFriday])

	return nil
}

// getGameInfoForWeek retrieves game date information for categorizing picks
func (s *PickService) getGameInfoForWeek(ctx context.Context, season, week int) (map[int]models.GameDayInfo, error) {
	// Get all games for the week
	games, err := s.gameRepo.FindByWeek(ctx, season, week)
	if err != nil {
		return nil, fmt.Errorf("failed to get games for week: %w", err)
	}

	gameInfoMap := make(map[int]models.GameDayInfo)
	for _, game := range games {
		category := models.CategorizeGameByDate(game.Date, season, week)
		gameInfoMap[game.ID] = models.GameDayInfo{
			GameID:   game.ID,
			GameDate: game.Date,
			Weekday:  game.Date.Weekday(),
			Category: category,
		}
	}

	return gameInfoMap, nil
}

// ProcessWeekParlayScoring calculates and saves parlay scores for all users when a week is complete
func (s *PickService) ProcessWeekParlayScoring(ctx context.Context, season, week int) error {
	// Delegate to specialized ParlayService
	return s.parlayService.ProcessWeekParlayScoring(ctx, season, week)
}

// ProcessParlayCategory calculates and saves parlay scores for a specific category when completed
func (s *PickService) ProcessParlayCategory(ctx context.Context, season, week int, category models.ParlayCategory) error {
	// Delegate to specialized ParlayService
	return s.parlayService.ProcessParlayCategory(ctx, season, week, category)
}

// CalculateUserParlayCategoryScore calculates points for a specific parlay category
func (s *PickService) CalculateUserParlayCategoryScore(ctx context.Context, userID, season, week int, category models.ParlayCategory) (int, error) {
	// Get all user's picks for the week
	userPicks, err := s.GetUserPicksForWeek(ctx, userID, season, week)
	if err != nil {
		return 0, fmt.Errorf("failed to get user picks: %w", err)
	}

	// Get game information for categorization
	gameInfoMap, err := s.getGameInfoForWeek(ctx, season, week)
	if err != nil {
		return 0, fmt.Errorf("failed to get game info: %w", err)
	}

	// Categorize picks by parlay type
	categories := models.CategorizePicksByGame(userPicks.Picks, gameInfoMap)

	// Get picks for the specific category
	categoryPicks, exists := categories[category]
	if !exists || len(categoryPicks) == 0 {
		return 0, nil // No picks in this category
	}

	// Calculate points for this category
	return models.CalculateParlayPoints(categoryPicks), nil
}

// UpdateUserParlayCategoryRecord updates a specific category score in the user's parlay record
func (s *PickService) UpdateUserParlayCategoryRecord(ctx context.Context, userID, season, week int, category models.ParlayCategory, points int) error {
	logger := logging.WithPrefix("PickService")
	// Get existing parlay score or create new one
	existingScore, err := s.parlayRepo.GetUserParlayScore(ctx, userID, season, week)
	if err != nil && err.Error() != "parlay score not found" {
		return fmt.Errorf("failed to get existing parlay score: %w", err)
	}

	var parlayScore *models.ParlayScore
	if existingScore != nil {
		parlayScore = existingScore
	} else {
		// Create new parlay score entry
		parlayScore = &models.ParlayScore{
			UserID:    userID,
			Season:    season,
			Week:      week,
			CreatedAt: time.Now(),
		}
	}

	// Update the specific category
	switch category {
	case models.ParlayRegular:
		parlayScore.RegularPoints = points
	case models.ParlayBonusThursday:
		parlayScore.BonusThursdayPoints = points
	case models.ParlayBonusFriday:
		parlayScore.BonusFridayPoints = points
	}

	// Recalculate total and update timestamp
	parlayScore.CalculateTotal()
	parlayScore.UpdatedAt = time.Now()

	// DEBUG: Log before saving parlay score in ProcessParlayCategory
	logger.Debugf("ProcessParlayCategory about to save - UserID=%d, Season=%d, Week=%d",
		userID, season, week)

	// Save to database
	if err := s.parlayRepo.UpsertParlayScore(ctx, parlayScore); err != nil {
		return fmt.Errorf("failed to save parlay score: %w", err)
	}

	// Don't broadcast here - let the caller broadcast once after all users are processed

	return nil
}

// ReplaceUserPicksForWeek clears existing picks and creates new ones for a user/week
func (s *PickService) ReplaceUserPicksForWeek(ctx context.Context, userID, season, week int, picks []*models.Pick) error {
	logger := logging.WithPrefix("PickService")
	// Create new weekly picks document
	weeklyPicks := models.NewWeeklyPicks(userID, season, week)
	for _, pick := range picks {
		weeklyPicks.Picks = append(weeklyPicks.Picks, *pick)
	}

	// Upsert the weekly picks (replaces existing)
	if err := s.weeklyPicksRepo.Upsert(ctx, weeklyPicks); err != nil {
		return fmt.Errorf("failed to replace weekly picks: %w", err)
	}

	logger.Infof("Replaced picks for user %d, season %d, week %d: %d picks", userID, season, week, len(picks))
	return nil
}

// UpdateUserPicksForScheduledGames updates picks only for scheduled games, preserving existing picks for completed games
// NEW: Uses WeeklyPicks model for single document update instead of multiple pick documents
func (s *PickService) UpdateUserPicksForScheduledGames(ctx context.Context, userID, season, week int, newPicks []*models.Pick, gameMap map[int]models.Game) error {
	logger := logging.WithPrefix("PickService")

	// Get existing weekly picks document
	existingWeeklyPicks, err := s.weeklyPicksRepo.FindByUserAndWeek(ctx, userID, season, week)
	if err != nil {
		return fmt.Errorf("failed to get existing weekly picks: %w", err)
	}

	// If no existing picks, create new document
	if existingWeeklyPicks == nil {
		existingWeeklyPicks = models.NewWeeklyPicks(userID, season, week)
	}

	// Convert newPicks from []*models.Pick to []models.Pick
	newPicksSlice := make([]models.Pick, len(newPicks))
	for i, pick := range newPicks {
		newPicksSlice[i] = *pick
	}

	// Separate existing picks by game state
	var picksToKeep []models.Pick
	scheduledGameIDs := make(map[int]bool)

	for _, pick := range existingWeeklyPicks.Picks {
		if game, exists := gameMap[pick.GameID]; exists {
			if game.State != models.GameStateScheduled {
				// Keep existing picks for completed/in-progress games
				picksToKeep = append(picksToKeep, pick)
				logger.Debugf("Preserving existing pick for completed/in-progress game %d (state: %s)", pick.GameID, game.State)
			} else {
				// Mark scheduled games for replacement
				scheduledGameIDs[pick.GameID] = true
			}
		}
	}

	// Mark new picks' games for update
	for _, pick := range newPicksSlice {
		scheduledGameIDs[pick.GameID] = true
	}

	// Replace picks for scheduled games
	existingWeeklyPicks.ReplacePicksForScheduledGames(newPicksSlice, scheduledGameIDs)

	// Single upsert operation - this will trigger only ONE change stream event!
	if err := s.weeklyPicksRepo.Upsert(ctx, existingWeeklyPicks); err != nil {
		return fmt.Errorf("failed to upsert weekly picks: %w", err)
	}

	logger.Infof("Updated picks for user %d, season %d, week %d: kept %d existing picks, created %d new picks",
		userID, season, week, len(picksToKeep), len(newPicksSlice))

	// Trigger scoring for any categories that might now be complete due to pick updates
	s.checkAndTriggerScoring(ctx, season, week, gameMap)

	return nil
}

// GetAllPicksForWeek returns individual Pick objects for compatibility with existing code
// This method maintains backward compatibility while using WeeklyPicks storage internally
func (s *PickService) GetAllPicksForWeek(ctx context.Context, season, week int) ([]models.Pick, error) {
	// Get all weekly picks documents
	allWeeklyPicks, err := s.weeklyPicksRepo.FindAllByWeek(ctx, season, week)
	if err != nil {
		return nil, fmt.Errorf("failed to get weekly picks: %w", err)
	}

	// Convert to individual picks with UserID populated
	var allPicks []models.Pick
	for _, weeklyPicks := range allWeeklyPicks {
		individualPicks := weeklyPicks.ToIndividualPicks()
		allPicks = append(allPicks, individualPicks...)
	}

	return allPicks, nil
}

// GetPicksForAnalytics retrieves picks for analytics calculations with enriched team names
func (s *PickService) GetPicksForAnalytics(ctx context.Context, season int, week *int, allSeasons bool) ([]models.Pick, error) {
	var picks []*models.Pick

	if allSeasons {
		// Get all weekly picks for season and convert to individual picks
		allWeeklyPicks, err := s.weeklyPicksRepo.FindBySeason(ctx, season)
		if err != nil {
			return nil, err
		}
		// Convert to individual picks
		var pickPtrs []*models.Pick
		for _, weeklyPicks := range allWeeklyPicks {
			individualPicks := weeklyPicks.ToIndividualPicks()
			for i := range individualPicks {
				pickPtrs = append(pickPtrs, &individualPicks[i])
			}
		}
		picks = pickPtrs
	} else if week != nil {
		// Get weekly picks for specific week and convert to individual picks
		allWeeklyPicks, err := s.weeklyPicksRepo.FindAllByWeek(ctx, season, *week)
		if err != nil {
			return nil, err
		}
		// Convert to individual picks
		var pickPtrs []*models.Pick
		for _, weeklyPicks := range allWeeklyPicks {
			individualPicks := weeklyPicks.ToIndividualPicks()
			for i := range individualPicks {
				pickPtrs = append(pickPtrs, &individualPicks[i])
			}
		}
		picks = pickPtrs
	} else {
		// Get all weekly picks for entire season and convert to individual picks
		allWeeklyPicks, err := s.weeklyPicksRepo.FindBySeason(ctx, season)
		if err != nil {
			return nil, err
		}
		// Convert to individual picks
		var pickPtrs []*models.Pick
		for _, weeklyPicks := range allWeeklyPicks {
			individualPicks := weeklyPicks.ToIndividualPicks()
			for i := range individualPicks {
				pickPtrs = append(pickPtrs, &individualPicks[i])
			}
		}
		picks = pickPtrs
	}

	// Enrich picks with team names for analytics
	enrichedPicks := make([]models.Pick, len(picks))
	for i, pick := range picks {
		enrichedPick := *pick

		// Get game data to populate team names
		game, err := s.gameRepo.FindByESPNID(ctx, pick.GameID)
		if err == nil && game != nil {
			s.populateTeamName(&enrichedPick, game)
		}

		enrichedPicks[i] = enrichedPick
	}

	return enrichedPicks, nil
}

// populateTeamName populates the TeamName field based on TeamID and game data
func (s *PickService) populateTeamName(pick *models.Pick, game *models.Game) {
	switch pick.PickType {
	case models.PickTypeOverUnder:
		if pick.TeamID == 98 {
			pick.TeamName = "UND" // Use consistent abbreviation for analytics
		} else if pick.TeamID == 99 {
			pick.TeamName = "OVR" // Use consistent abbreviation for analytics
		}
	case models.PickTypeSpread:
		// Get team abbreviation from ID mapping
		teamAbbr := s.getTeamNameFromID(pick.TeamID, game)
		pick.TeamName = teamAbbr // Use clean abbreviation for analytics
	}
}


// checkAndTriggerScoring checks if any parlay categories are now complete and triggers scoring
// Only triggers scoring for categories with completed games - not for pending games
func (s *PickService) checkAndTriggerScoring(ctx context.Context, season, week int, gameMap map[int]models.Game) {
	logger := logging.WithPrefix("PickService")
	// Group games by category for this week
	weekCategoryGames := make(map[models.ParlayCategory][]models.Game)

	for _, game := range gameMap {
		if game.Week == week {
			category := models.CategorizeGameByDate(game.Date, season, game.Week)
			weekCategoryGames[category] = append(weekCategoryGames[category], game)
		}
	}

	// Check each category for completion
	for category, games := range weekCategoryGames {
		// Skip categories that have any pending games - no need to trigger scoring
		hasCompletedGames := false
		allCompleted := true
		for _, game := range games {
			if game.IsCompleted() {
				hasCompletedGames = true
			} else {
				allCompleted = false
			}
		}

		// Only trigger scoring if:
		// 1. Category has some completed games
		// 2. ALL games in the category are completed (no pending games)
		// 3. There are games in the category
		if hasCompletedGames && allCompleted && len(games) > 0 {
			logger.Infof("Parlay category %s completed for Week %d, triggering scoring", category, week)
			if err := s.ProcessParlayCategory(ctx, season, week, category); err != nil {
				logger.Errorf("Failed to process parlay category %s for Week %d: %v", category, week, err)
			}
		}
	}
}

// ===== MODERN SCORING: 2025+ Daily Parlay System =====

// ProcessDailyParlayScoring calculates and saves daily parlay scores for modern seasons (2025+)
func (s *PickService) ProcessDailyParlayScoring(ctx context.Context, season, week int) error {
	// Delegate to specialized ParlayService
	return s.parlayService.ProcessDailyParlayScoring(ctx, season, week)
}

// CalculateUserDailyParlayScores calculates points for each day for a user in modern seasons
func (s *PickService) CalculateUserDailyParlayScores(ctx context.Context, userID, season, week int, games []models.Game) (map[string]int, error) {
	logger := logging.WithPrefix("PickService")
	// Get user's picks for the week
	userPicks, err := s.GetUserPicksForWeek(ctx, userID, season, week)
	if err != nil {
		return nil, fmt.Errorf("failed to get user picks: %w", err)
	}

	// Group picks by Pacific timezone date
	dailyPickGroups := models.GroupPicksByDay(userPicks.Picks, games)

	// Calculate points for each day
	dailyScores := make(map[string]int)
	for date, picks := range dailyPickGroups {
		points := models.CalculateDailyParlayPoints(picks)
		if points > 0 {
			dailyScores[date] = points
		}

		logger.Debugf("User %d, Date %s: %d picks → %d points", userID, date, len(picks), points)
	}

	return dailyScores, nil
}

// UpdateUserDailyParlayRecord updates a user's daily parlay record in the database
func (s *PickService) UpdateUserDailyParlayRecord(ctx context.Context, userID, season, week int, dailyScores map[string]int) error {
	logger := logging.WithPrefix("PickService")
	// Calculate total points for the week
	totalPoints := 0
	logger.Debugf("User %d daily scores before save: %+v", userID, dailyScores)
	for date, points := range dailyScores {
		logger.Debugf("User %d adding %d points from date %s", userID, points, date)
		totalPoints += points
	}
	logger.Debugf("User %d calculated total points: %d", userID, totalPoints)

	// For now, store in the existing ParlayScore structure with total points
	// Future enhancement: could add a DailyParlayScore model for more granular storage
	parlayScore := &models.ParlayScore{
		UserID:      userID,
		Season:      season,
		Week:        week,
		TotalPoints: totalPoints,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
		// MODERN: Store daily breakdown as JSON or separate table
		// For now, just store total in existing structure
	}

	// DEBUG: Log before saving daily parlay score
	logger.Debugf("ProcessUserDailyParlayScoring about to save - UserID=%d, Season=%d, Week=%d",
		userID, season, week)

	// Save to database
	if err := s.parlayRepo.UpsertParlayScore(ctx, parlayScore); err != nil {
		return fmt.Errorf("failed to save daily parlay scores: %w", err)
	}

	return nil
}

// CheckWeekHasParlayScores checks if parlay scores already exist for a given week
func (s *PickService) CheckWeekHasParlayScores(ctx context.Context, season, week int) (bool, error) {
	scores, err := s.parlayRepo.GetWeekScores(ctx, season, week)
	if err != nil {
		return false, fmt.Errorf("failed to check week scores: %w", err)
	}

	// Return true if any scores exist for this week
	return len(scores) > 0, nil
}

// sortPicksByGameTime sorts picks by their corresponding game start times
// Uses the same sorting logic as sortGamesByKickoffTime for consistency
func (s *PickService) sortPicksByGameTime(picks []models.Pick, gameMap map[int]models.Game) {
	sort.Slice(picks, func(i, j int) bool {
		gameI, existsI := gameMap[picks[i].GameID]
		gameJ, existsJ := gameMap[picks[j].GameID]

		// If either game doesn't exist, maintain original order
		if !existsI || !existsJ {
			return i < j
		}

		// Primary sort: by game date/time
		if gameI.Date.Unix() != gameJ.Date.Unix() {
			return gameI.Date.Before(gameJ.Date)
		}

		// Secondary sort: alphabetically by home team name for same kickoff time
		return gameI.Home < gameJ.Home
	})
}
