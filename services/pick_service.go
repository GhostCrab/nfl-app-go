package services

import (
	"context"
	"fmt"
	"log"
	"sort"
	"time"

	"go.mongodb.org/mongo-driver/bson/primitive"

	"nfl-app-go/database"
	"nfl-app-go/models"
)

// PickService handles business logic for picks
// SSEBroadcaster interface for sending SSE updates
type SSEBroadcaster interface {
	BroadcastStructuredUpdate(eventType, data string)
}

type PickService struct {
	pickRepo    *database.MongoPickRepository
	gameRepo    *database.MongoGameRepository
	userRepo    *database.MongoUserRepository
	parlayRepo  *database.MongoParlayRepository
	broadcaster SSEBroadcaster
}

// NewPickService creates a new pick service
func NewPickService(pickRepo *database.MongoPickRepository, gameRepo *database.MongoGameRepository, userRepo *database.MongoUserRepository, parlayRepo *database.MongoParlayRepository) *PickService {
	return &PickService{
		pickRepo:   pickRepo,
		gameRepo:   gameRepo,
		userRepo:   userRepo,
		parlayRepo: parlayRepo,
	}
}

// SetBroadcaster sets the SSE broadcaster for real-time updates
func (s *PickService) SetBroadcaster(broadcaster SSEBroadcaster) {
	s.broadcaster = broadcaster
}

// CreatePick creates a new pick with validation
func (s *PickService) CreatePick(ctx context.Context, userID, gameID, teamID, season, week int) (*models.Pick, error) {
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
	
	// Store in database
	if err := s.pickRepo.Create(ctx, pick); err != nil {
		return nil, fmt.Errorf("failed to create pick: %w", err)
	}
	
	log.Printf("Created pick: User %d, Game %d, Team %d, Season %d, Week %d", 
		userID, gameID, teamID, season, week)
	
	return pick, nil
}

// GetUserPicksForWeek retrieves a user's picks for a specific week with organized structure
func (s *PickService) GetUserPicksForWeek(ctx context.Context, userID, season, week int) (*models.UserPicks, error) {
	picks, err := s.pickRepo.FindByUserAndWeek(ctx, userID, season, week)
	if err != nil {
		return nil, fmt.Errorf("failed to get user picks: %w", err)
	}
	
	// Get user info
	user, err := s.userRepo.GetUserByID(userID)
	if err != nil {
		return nil, fmt.Errorf("failed to get user info: %w", err)
	}
	if user == nil {
		return nil, fmt.Errorf("user not found")
	}
	
	// Get user's basic record for the season
	record, err := s.pickRepo.GetUserRecord(ctx, userID, season)
	if err != nil {
		return nil, fmt.Errorf("failed to get user record: %w", err)
	}
	
	// Get cumulative parlay points up to the current week (not entire season)
	cumulativeParlayTotal, err := s.parlayRepo.GetUserCumulativeScoreUpToWeek(ctx, userID, season, week)
	if err != nil {
		log.Printf("Warning: failed to get cumulative parlay total for user %d season %d week %d: %v", userID, season, week, err)
	} else {
		record.ParlayPoints = cumulativeParlayTotal
	}
	
	// Get parlay points for this specific week
	weekParlayScore, err := s.parlayRepo.GetUserParlayScore(ctx, userID, season, week)
	if err != nil {
		log.Printf("Warning: failed to get weekly parlay score for user %d week %d: %v", userID, week, err)
	} else if weekParlayScore != nil {
		record.WeeklyPoints = weekParlayScore.TotalPoints
	}
	
	// Get game information for categorizing picks by day
	gameInfoMap, err := s.getGameInfoForWeek(ctx, season, week)
	if err != nil {
		log.Printf("Warning: failed to get game info for pick categorization: %v", err)
		gameInfoMap = make(map[int]models.GameDayInfo) // Empty map as fallback
	}
	
	// Organize picks by type
	userPicks := &models.UserPicks{
		UserID:   userID,
		UserName: user.Name,
		Picks:    make([]models.Pick, len(picks)),
		Record:   *record,
	}
	
	// Convert picks and categorize
	for i, pick := range picks {
		userPicks.Picks[i] = *pick
		
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
	
	return userPicks, nil
}

// GetAllUserPicksForWeek retrieves all users' picks for a specific week
func (s *PickService) GetAllUserPicksForWeek(ctx context.Context, season, week int) ([]*models.UserPicks, error) {
	// Get all picks for the week
	allPicks, err := s.pickRepo.FindByWeek(ctx, season, week)
	if err != nil {
		return nil, fmt.Errorf("failed to get week picks: %w", err)
	}
	
	log.Printf("PickService: Found %d total picks for season %d, week %d", len(allPicks), season, week)
	
	// Group picks by user
	picksByUser := make(map[int][]*models.Pick)
	for _, pick := range allPicks {
		picksByUser[pick.UserID] = append(picksByUser[pick.UserID], pick)
		log.Printf("PickService: Assigning pick for game %d to user %d (team: %s)", pick.GameID, pick.UserID, pick.TeamName)
	}
	
	log.Printf("PickService: Grouped picks by user: %d users have picks", len(picksByUser))
	for userID, userPicks := range picksByUser {
		log.Printf("PickService: User %d has %d picks", userID, len(userPicks))
	}
	
	// Get game information for categorizing picks by day
	gameInfoMap, err := s.getGameInfoForWeek(ctx, season, week)
	if err != nil {
		log.Printf("Warning: failed to get game info for pick categorization: %v", err)
		gameInfoMap = make(map[int]models.GameDayInfo) // Empty map as fallback
	}
	
	// Get all users
	users, err := s.userRepo.GetAllUsers()
	if err != nil {
		return nil, fmt.Errorf("failed to get users: %w", err)
	}
	
	var result []*models.UserPicks
	for _, user := range users {
		userPicks, exists := picksByUser[user.ID]
		var picks []models.Pick
		if exists {
			picks = make([]models.Pick, len(userPicks))
			for i, pick := range userPicks {
				enrichedPick := *pick
				
				// Enrich pick with game information
				if err := s.enrichPickWithGameData(&enrichedPick); err != nil {
					log.Printf("Warning: failed to enrich pick for game %d: %v", pick.GameID, err)
				}
				
				picks[i] = enrichedPick
			}
		}
		
		// Get user's basic record for the season
		record, err := s.pickRepo.GetUserRecord(ctx, user.ID, season)
		if err != nil {
			log.Printf("Warning: failed to get record for user %d: %v", user.ID, err)
			record = &models.UserRecord{} // Empty record on error
		}
		
		// Get cumulative parlay points up to the current week (not entire season)
		cumulativeParlayTotal, err := s.parlayRepo.GetUserCumulativeScoreUpToWeek(ctx, user.ID, season, week)
		if err != nil {
			log.Printf("Warning: failed to get cumulative parlay total for user %d season %d week %d: %v", user.ID, season, week, err)
		} else {
			record.ParlayPoints = cumulativeParlayTotal
		}
		
		// Get parlay points for this specific week
		weekParlayScore, err := s.parlayRepo.GetUserParlayScore(ctx, user.ID, season, week)
		if err != nil {
			log.Printf("Warning: failed to get weekly parlay score for user %d week %d: %v", user.ID, week, err)
		} else if weekParlayScore != nil {
			record.WeeklyPoints = weekParlayScore.TotalPoints
		}
		
		// Create user picks with categorization
		userPicksData := &models.UserPicks{
			UserID:   user.ID,
			UserName: user.Name,
			Picks:    picks,
			Record:   *record,
		}
		
		// Categorize picks by game day for bonus weeks
		for _, pick := range picks {
			if gameInfo, exists := gameInfoMap[pick.GameID]; exists {
				switch gameInfo.Category {
				case models.ParlayBonusThursday:
					userPicksData.BonusThursdayPicks = append(userPicksData.BonusThursdayPicks, pick)
				case models.ParlayBonusFriday:
					userPicksData.BonusFridayPicks = append(userPicksData.BonusFridayPicks, pick)
				}
			}
			
			// Also categorize by pick type
			switch pick.PickType {
			case models.PickTypeSpread:
				userPicksData.SpreadPicks = append(userPicksData.SpreadPicks, pick)
			case models.PickTypeOverUnder:
				userPicksData.OverUnderPicks = append(userPicksData.OverUnderPicks, pick)
			}
		}
		
		result = append(result, userPicksData)
	}
	
	// Sort by user name
	sort.Slice(result, func(i, j int) bool {
		return result[i].UserName < result[j].UserName
	})
	
	return result, nil
}

// UpdatePickResult updates the result of a pick (called when games are completed)
func (s *PickService) UpdatePickResult(ctx context.Context, pickID primitive.ObjectID, result models.PickResult) error {
	return s.pickRepo.UpdateResult(ctx, pickID, result)
}

// ProcessGameCompletion processes all picks for a completed game and calculates results
func (s *PickService) ProcessGameCompletion(ctx context.Context, game *models.Game) error {
	if !game.IsCompleted() {
		return fmt.Errorf("game %d is not completed", game.ID)
	}
	
	// Get all picks for this game
	picks, err := s.pickRepo.FindByGame(ctx, game.ID)
	if err != nil {
		return fmt.Errorf("failed to get picks for game %d: %w", game.ID, err)
	}
	
	log.Printf("Processing %d picks for completed game %d", len(picks), game.ID)
	
	for _, pick := range picks {
		var result models.PickResult
		
		if pick.PickType == models.PickTypeSpread {
			// Process spread pick
			result = s.calculateSpreadResult(pick, game)
		} else if pick.PickType == models.PickTypeOverUnder {
			// Process over/under pick
			result = s.calculateOverUnderResult(pick, game)
		} else {
			log.Printf("Unknown pick type %s for pick %s", pick.PickType, pick.ID.Hex())
			continue
		}
		
		// Update the pick result
		if err := s.pickRepo.UpdateResult(ctx, pick.ID, result); err != nil {
			log.Printf("Failed to update pick %s result: %v", pick.ID.Hex(), err)
			continue
		}
		
		log.Printf("Updated pick %s result to %s", pick.ID.Hex(), result)
	}
	
	return nil
}

// calculateSpreadResult determines the result of a spread pick
func (s *PickService) calculateSpreadResult(pick *models.Pick, game *models.Game) models.PickResult {
	if game.Odds == nil || game.State != models.GameStateCompleted {
		return models.PickResultPending
	}
	// Note: Spread of 0 is valid (pick 'em game), so we don't filter it out
	
	teamAbbr := s.getTeamNameFromID(pick.TeamID, game)
	
	// Calculate actual point difference (home - away)
	pointDiff := float64(game.HomeScore - game.AwayScore)
	
	if teamAbbr == game.Home {
		// Picked home team
		adjustedDiff := pointDiff + game.Odds.Spread
		if adjustedDiff > 0 {
			return models.PickResultWin
		} else if adjustedDiff == 0 {
			return models.PickResultPush
		} else {
			return models.PickResultLoss
		}
	} else {
		// Picked away team
		adjustedDiff := -pointDiff - game.Odds.Spread
		if adjustedDiff > 0 {
			return models.PickResultWin
		} else if adjustedDiff == 0 {
			return models.PickResultPush
		} else {
			return models.PickResultLoss
		}
	}
}

// calculateOverUnderResult determines the result of an over/under pick
func (s *PickService) calculateOverUnderResult(pick *models.Pick, game *models.Game) models.PickResult {
	if game.Odds == nil || game.Odds.OU == 0 || game.State != models.GameStateCompleted {
		return models.PickResultPending
	}
	
	totalPoints := float64(game.HomeScore + game.AwayScore)
	
	if pick.TeamID == 99 { // Over
		if totalPoints > game.Odds.OU {
			return models.PickResultWin
		} else if totalPoints == game.Odds.OU {
			return models.PickResultPush
		} else {
			return models.PickResultLoss
		}
	} else { // Under
		if totalPoints < game.Odds.OU {
			return models.PickResultWin
		} else if totalPoints == game.Odds.OU {
			return models.PickResultPush
		} else {
			return models.PickResultLoss
		}
	}
}

// GetPickStats returns statistics about picks in the system
func (s *PickService) GetPickStats(ctx context.Context) (map[string]interface{}, error) {
	totalPicks, err := s.pickRepo.Count(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to count picks: %w", err)
	}
	
	stats := map[string]interface{}{
		"total_picks": totalPicks,
		"last_updated": time.Now(),
	}
	
	// Add season-specific stats
	seasons := []int{2023, 2024, 2025}
	for _, season := range seasons {
		// This would require additional aggregation queries to get detailed stats per season
		stats[fmt.Sprintf("season_%d", season)] = fmt.Sprintf("Season %d data", season)
	}
	
	return stats, nil
}

// enrichPickWithGameData populates the display fields of a pick with game information
func (s *PickService) enrichPickWithGameData(pick *models.Pick) error {
	// Get the game information
	game, err := s.gameRepo.FindByESPNID(context.Background(), pick.GameID)
	if err != nil {
		return fmt.Errorf("failed to find game %d: %w", pick.GameID, err)
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
		}
	case models.PickTypeSpread:
		// Get team abbreviation from ID mapping
		teamAbbr := s.getTeamNameFromID(pick.TeamID, game)
		
		// Add spread information if available
		if game.Odds != nil && game.Odds.Spread != 0 {
			// Determine if this team is home or away to show correct spread
			if teamAbbr == game.Home {
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
	}
	
	// Update pick result based on game status
	if game.State == models.GameStateCompleted {
		calculatedResult := s.calculatePickResult(pick, game)
		if calculatedResult != pick.Result {
			// Result has changed, update in database
			if err := s.pickRepo.UpdateResult(context.Background(), pick.ID, calculatedResult); err != nil {
				log.Printf("Warning: failed to update pick result for pick %s: %v", pick.ID.Hex(), err)
			} else {
				log.Printf("Updated pick result for pick %s: %s -> %s", pick.ID.Hex(), pick.Result, calculatedResult)
			}
			pick.Result = calculatedResult
		}
	} else if game.State == models.GameStateInPlay {
		pick.Result = models.PickResultPending // Could show "In Progress" 
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
	}
	
	// Debug logging for unmatched teams
	if teamID == 28 { // Washington
		log.Printf("Warning: Washington team ID 28 not matched - Game: %s @ %s, Expected: WSH", game.Away, game.Home)
	}
	
	// Fallback: return the team ID as string
	return fmt.Sprintf("Team%d", teamID)
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
	if game.State != models.GameStateCompleted {
		return models.PickResultPending
	}
	
	switch pick.PickType {
	case models.PickTypeSpread:
		return s.calculateSpreadResult(pick, game)
	case models.PickTypeOverUnder:
		return s.calculateOverUnderResult(pick, game)
	default:
		return models.PickResultPending
	}
}

// CalculateUserParlayScore calculates parlay club points for a user in a specific week
// Handles bonus weeks separately from regular weekend picks
func (s *PickService) CalculateUserParlayScore(ctx context.Context, userID, season, week int) (map[models.ParlayCategory]int, error) {
	// Get all user's picks for the week
	userPicks, err := s.GetUserPicksForWeek(ctx, userID, season, week)
	if err != nil {
		return nil, fmt.Errorf("failed to get user picks: %w", err)
	}
	
	// Get game information for categorization
	gameInfoMap, err := s.getGameInfoForWeek(ctx, season, week)
	if err != nil {
		return nil, fmt.Errorf("failed to get game info: %w", err)
	}
	
	// Categorize picks by parlay type
	categories := models.CategorizePicksByGame(userPicks.Picks, gameInfoMap)
	
	// Calculate points for each category
	scores := make(map[models.ParlayCategory]int)
	for category, picks := range categories {
		scores[category] = models.CalculateParlayPoints(picks)
	}
	
	return scores, nil
}

// CalculateAllUsersParlayScores calculates parlay scores for all users in a week
func (s *PickService) CalculateAllUsersParlayScores(ctx context.Context, season, week int) (map[int]map[models.ParlayCategory]int, error) {
	// Get all users
	users, err := s.userRepo.GetAllUsers()
	if err != nil {
		return nil, fmt.Errorf("failed to get users: %w", err)
	}
	
	result := make(map[int]map[models.ParlayCategory]int)
	
	for _, user := range users {
		scores, err := s.CalculateUserParlayScore(ctx, user.ID, season, week)
		if err != nil {
			log.Printf("Warning: failed to calculate parlay score for user %d: %v", user.ID, err)
			continue
		}
		result[user.ID] = scores
	}
	
	return result, nil
}

// UpdateUserParlayRecord updates a user's parlay record with weekly points
func (s *PickService) UpdateUserParlayRecord(ctx context.Context, userID, season, week int, weeklyScores map[models.ParlayCategory]int) error {
	// Create parlay score entry
	parlayScore := models.CreateParlayScore(userID, season, week, weeklyScores)
	
	// Save to database
	if err := s.parlayRepo.UpsertParlayScore(ctx, parlayScore); err != nil {
		return fmt.Errorf("failed to save parlay score: %w", err)
	}
	
	log.Printf("User %d earned %d parlay points in week %d (Regular: %d, Thu: %d, Fri: %d)", 
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
	log.Printf("Processing parlay scoring for Season %d, Week %d", season, week)
	
	// Calculate scores for all users
	allScores, err := s.CalculateAllUsersParlayScores(ctx, season, week)
	if err != nil {
		return fmt.Errorf("failed to calculate parlay scores: %w", err)
	}
	
	// Save each user's score
	for userID, weeklyScores := range allScores {
		if err := s.UpdateUserParlayRecord(ctx, userID, season, week, weeklyScores); err != nil {
			log.Printf("Warning: failed to save parlay score for user %d: %v", userID, err)
			continue
		}
	}
	
	log.Printf("Completed parlay scoring for Season %d, Week %d", season, week)
	return nil
}

// ProcessParlayCategory calculates and saves parlay scores for a specific category when completed
func (s *PickService) ProcessParlayCategory(ctx context.Context, season, week int, category models.ParlayCategory) error {
	log.Printf("Processing parlay scoring for Season %d, Week %d, Category %s", season, week, category)
	
	// Get all users
	users, err := s.userRepo.GetAllUsers()
	if err != nil {
		return fmt.Errorf("failed to get users: %w", err)
	}
	
	// Process each user
	for _, user := range users {
		// Calculate scores for this specific category
		categoryScore, err := s.CalculateUserParlayCategoryScore(ctx, user.ID, season, week, category)
		if err != nil {
			log.Printf("Warning: failed to calculate parlay category score for user %d: %v", user.ID, err)
			continue
		}
		
		// Update user's parlay record for this category
		if err := s.UpdateUserParlayCategoryRecord(ctx, user.ID, season, week, category, categoryScore); err != nil {
			log.Printf("Warning: failed to save parlay category score for user %d: %v", user.ID, err)
			continue
		}
		
		log.Printf("User %d earned %d points in %s category for Week %d", user.ID, categoryScore, category, week)
	}
	
	// Note: Club score updates will be visible on next page load/refresh
	// Real-time score updates would need OOB template content to work properly
	
	log.Printf("Completed parlay category scoring for Season %d, Week %d, Category %s", season, week, category)
	return nil
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
			UserID:  userID,
			Season:  season,
			Week:    week,
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
	
	// Save to database
	if err := s.parlayRepo.UpsertParlayScore(ctx, parlayScore); err != nil {
		return fmt.Errorf("failed to save parlay score: %w", err)
	}
	
	// Don't broadcast here - let the caller broadcast once after all users are processed
	
	return nil
}

// ReplaceUserPicksForWeek clears existing picks and creates new ones for a user/week
func (s *PickService) ReplaceUserPicksForWeek(ctx context.Context, userID, season, week int, picks []*models.Pick) error {
	// First, delete all existing picks for this user/season/week
	if err := s.pickRepo.DeleteByUserAndWeek(ctx, userID, season, week); err != nil {
		return fmt.Errorf("failed to clear existing picks: %w", err)
	}
	
	// Create new picks
	for _, pick := range picks {
		if err := s.pickRepo.Create(ctx, pick); err != nil {
			return fmt.Errorf("failed to create pick: %w", err)
		}
	}
	
	log.Printf("Replaced picks for user %d, season %d, week %d: %d picks", userID, season, week, len(picks))
	return nil
}

// UpdateUserPicksForScheduledGames updates picks only for scheduled games, preserving existing picks for completed games
func (s *PickService) UpdateUserPicksForScheduledGames(ctx context.Context, userID, season, week int, newPicks []*models.Pick, gameMap map[int]models.Game) error {
	// Get existing picks for this user/season/week
	existingPicks, err := s.pickRepo.FindByUserAndWeek(ctx, userID, season, week)
	if err != nil {
		return fmt.Errorf("failed to get existing picks: %w", err)
	}
	
	// Separate existing picks by game state
	picksToKeep := make([]*models.Pick, 0)
	gameIDsToUpdate := make(map[int]bool)
	
	for _, pick := range existingPicks {
		if game, exists := gameMap[pick.GameID]; exists {
			if game.State != models.GameStateScheduled {
				// Keep existing picks for completed/in-progress games
				picksToKeep = append(picksToKeep, pick)
				log.Printf("Preserving existing pick for completed/in-progress game %d (state: %s)", pick.GameID, game.State)
			} else {
				// Mark scheduled games for update
				gameIDsToUpdate[pick.GameID] = true
			}
		}
	}
	
	// Mark new picks' games for update
	for _, pick := range newPicks {
		gameIDsToUpdate[pick.GameID] = true
	}
	
	// Delete only picks for scheduled games that are being updated
	for gameID := range gameIDsToUpdate {
		if err := s.pickRepo.DeleteByUserGameAndWeek(ctx, userID, gameID, season, week); err != nil {
			return fmt.Errorf("failed to delete picks for game %d: %w", gameID, err)
		}
	}
	
	// Create new picks for scheduled games
	for _, pick := range newPicks {
		if err := s.pickRepo.Create(ctx, pick); err != nil {
			return fmt.Errorf("failed to create pick: %w", err)
		}
	}
	
	log.Printf("Updated picks for user %d, season %d, week %d: kept %d existing picks, created %d new picks", 
		userID, season, week, len(picksToKeep), len(newPicks))
	
	// Trigger scoring for any categories that might now be complete due to pick updates
	s.checkAndTriggerScoring(ctx, season, week, gameMap)
	
	return nil
}

// GetPicksForAnalytics retrieves picks for analytics calculations with enriched team names
func (s *PickService) GetPicksForAnalytics(ctx context.Context, season int, week *int, allSeasons bool) ([]models.Pick, error) {
	var picks []*models.Pick
	var err error
	
	if allSeasons {
		// Would need to implement getting picks from all seasons
		// For now, just return current season
		picks, err = s.pickRepo.FindBySeason(ctx, season)
		if err != nil {
			return nil, err
		}
	} else if week != nil {
		// Get picks for specific week
		picks, err = s.pickRepo.FindByWeek(ctx, season, *week)
		if err != nil {
			return nil, err
		}
	} else {
		// Get picks for entire season
		picks, err = s.pickRepo.FindBySeason(ctx, season)
		if err != nil {
			return nil, err
		}
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

// Helper function to convert []*Pick to []Pick
func convertPickPointers(picks []*models.Pick) []models.Pick {
	result := make([]models.Pick, len(picks))
	for i, pick := range picks {
		result[i] = *pick
	}
	return result
}

// checkAndTriggerScoring checks if any parlay categories are now complete and triggers scoring
// Only triggers scoring for categories with completed games - not for pending games
func (s *PickService) checkAndTriggerScoring(ctx context.Context, season, week int, gameMap map[int]models.Game) {
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
			log.Printf("PickService: Parlay category %s completed for Week %d, triggering scoring", category, week)
			if err := s.ProcessParlayCategory(ctx, season, week, category); err != nil {
				log.Printf("PickService: Failed to process parlay category %s for Week %d: %v", category, week, err)
			}
		}
	}
}

