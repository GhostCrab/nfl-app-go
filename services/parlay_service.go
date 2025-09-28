package services

import (
	"context"
	"fmt"
	"nfl-app-go/database"
	"nfl-app-go/logging"
	"nfl-app-go/models"
	"runtime"
)

// ParlayScoreBroadcaster interface for broadcasting parlay score updates via SSE
type ParlayScoreBroadcaster interface {
	BroadcastParlayScoreUpdate(season, week int)
}

// ParlayService handles all parlay scoring logic separated from PickService
// This service is responsible for calculating weekly and daily parlay scores
// and updating user parlay records.
type ParlayService struct {
	weeklyPicksRepo *database.MongoWeeklyPicksRepository
	gameRepo        *database.MongoGameRepository
	memoryScorer    *MemoryParlayScorer
	broadcaster     ParlayScoreBroadcaster
}

// NewParlayService creates a new parlay service instance
func NewParlayService(
	weeklyPicksRepo *database.MongoWeeklyPicksRepository,
	gameRepo *database.MongoGameRepository,
) *ParlayService {
	service := &ParlayService{
		weeklyPicksRepo: weeklyPicksRepo,
		gameRepo:        gameRepo,
	}
	return service
}

// SetMemoryScorer sets the memory scorer for club score tracking
func (s *ParlayService) SetMemoryScorer(memoryScorer *MemoryParlayScorer) {
	s.memoryScorer = memoryScorer
}

// SetBroadcaster sets the SSE broadcaster for real-time parlay score updates
func (s *ParlayService) SetBroadcaster(broadcaster ParlayScoreBroadcaster) {
	s.broadcaster = broadcaster
}

// TriggerScoreBroadcast manually triggers a parlay score broadcast for immediate updates
func (s *ParlayService) TriggerScoreBroadcast(season, week int) {
	if s.broadcaster != nil {
		s.broadcaster.BroadcastParlayScoreUpdate(season, week)
	}
}

// CalculateUserParlayScore calculates parlay scores for a specific user and week
// Uses legacy logic for pre-2025 seasons and daily logic for modern seasons (2025+)
func (s *ParlayService) CalculateUserParlayScore(ctx context.Context, userID, season, week int) (map[models.ParlayCategory]int, error) {
	// For modern seasons (2025+), convert daily scores to category format
	if models.IsModernSeason(season) {
		return s.calculateModernSeasonParlayScore(ctx, userID, season, week)
	}

	// Legacy seasons: use the original categorization logic
	// Get user's picks for the week from WeeklyPicks document
	userPicks, err := s.getUserPicksForWeek(ctx, userID, season, week)
	if err != nil {
		return nil, fmt.Errorf("failed to get user picks: %w", err)
	}

	if len(userPicks) == 0 {
		logging.Infof("No picks found for user %d, season %d, week %d", userID, season, week)
		return map[models.ParlayCategory]int{}, nil
	}

	// Get game information for categorization using the original legacy logic
	gameInfoMap, err := s.getGameInfoForWeek(ctx, season, week)
	if err != nil {
		return nil, fmt.Errorf("failed to get game info: %w", err)
	}

	// Categorize picks by parlay type using the ORIGINAL legacy logic
	categories := models.CategorizePicksByGame(userPicks, gameInfoMap)

	// Calculate points for each category using the ORIGINAL legacy logic
	scores := make(map[models.ParlayCategory]int)
	for category, picks := range categories {
		scores[category] = models.CalculateParlayPoints(picks)
	}

	return scores, nil
}

// calculateModernSeasonParlayScore calculates parlay scores for modern seasons (2025+)
// Maps daily scores to category format for compatibility with existing interfaces
func (s *ParlayService) calculateModernSeasonParlayScore(ctx context.Context, userID, season, week int) (map[models.ParlayCategory]int, error) {
	// Get all games for the week
	games, err := s.gameRepo.GetGamesByWeekSeason(week, season)
	if err != nil {
		return nil, fmt.Errorf("failed to get games: %w", err)
	}

	// Convert to slice of models.Game (not pointers)
	gameSlice := make([]models.Game, len(games))
	for i, game := range games {
		gameSlice[i] = *game
	}

	// Calculate daily scores using modern logic
	dailyScores, err := s.CalculateUserDailyParlayScores(ctx, userID, season, week, gameSlice)
	if err != nil {
		return nil, fmt.Errorf("failed to calculate daily scores: %w", err)
	}

	// Map daily scores to parlay categories for compatibility
	// In modern seasons, each day is its own "category"
	scores := make(map[models.ParlayCategory]int)

	// Thursday maps to bonus Thursday category
	if thursdayPoints, exists := dailyScores["Thursday"]; exists {
		scores[models.ParlayBonusThursday] = thursdayPoints
	}

	// Friday maps to bonus Friday category
	if fridayPoints, exists := dailyScores["Friday"]; exists {
		scores[models.ParlayBonusFriday] = fridayPoints
	}

	// Weekend days (Saturday, Sunday, Monday) map to regular category
	weekendTotal := 0
	for _, day := range []string{"Saturday", "Sunday", "Monday"} {
		if points, exists := dailyScores[day]; exists {
			weekendTotal += points
		}
	}
	scores[models.ParlayRegular] = weekendTotal

	return scores, nil
}

// getGameInfoForWeek retrieves game date information for categorizing picks using original legacy logic
func (s *ParlayService) getGameInfoForWeek(ctx context.Context, season, week int) (map[int]models.GameDayInfo, error) {
	// Get all games for the week
	games, err := s.gameRepo.GetGamesByWeekSeason(week, season)
	if err != nil {
		return nil, fmt.Errorf("failed to get games for week: %w", err)
	}

	gameInfoMap := make(map[int]models.GameDayInfo)
	for _, game := range games {
		// Use the ORIGINAL legacy categorization logic
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

// ProcessWeekParlayScoring processes parlay scoring for all users in a week
func (s *ParlayService) ProcessWeekParlayScoring(ctx context.Context, season, week int) error {
	logging.Infof("Processing parlay scoring for season %d, week %d", season, week)

	// Get all unique user IDs who made picks this week from WeeklyPicks documents
	userIDs, err := s.getUniqueUserIDsForWeek(ctx, season, week)
	if err != nil {
		return fmt.Errorf("failed to get user IDs: %w", err)
	}

	logging.Infof("Found %d users with picks for week %d", len(userIDs), week)

	// Process each user's parlay scores
	for _, userID := range userIDs {
		scores, err := s.CalculateUserParlayScore(ctx, userID, season, week)
		if err != nil {
			logging.Errorf("Failed to calculate parlay scores for user %d: %v", userID, err)
			continue
		}

		// Update the user's parlay record
		err = s.UpdateUserParlayRecord(ctx, userID, season, week, scores)
		if err != nil {
			logging.Errorf("Failed to update parlay record for user %d: %v", userID, err)
			continue
		}

		logging.Infof("Updated parlay scores for user %d: %+v", userID, scores)
	}

	return nil
}

// UpdateUserParlayRecord updates a user's parlay scores in memory
func (s *ParlayService) UpdateUserParlayRecord(ctx context.Context, userID, season, week int, weeklyScores map[models.ParlayCategory]int) error {
	// Check for invalid data
	if season == 0 || week == 0 {
		logging.Errorf("Invalid ParlayService.UpdateUserParlayRecord params - Season=%d, Week=%d, UserID=%d",
			season, week, userID)

		// Print stack trace to identify caller
		buf := make([]byte, 4096)
		n := runtime.Stack(buf, false)
		logging.Errorf("ParlayService.UpdateUserParlayRecord stack trace:\n%s", buf[:n])
	}

	// Use MemoryParlayScorer to recalculate and store scores
	if s.memoryScorer != nil {
		_, err := s.memoryScorer.RecalculateUserScore(ctx, season, week, userID)
		if err != nil {
			return fmt.Errorf("failed to update user parlay record in memory: %w", err)
		}
	}

	return nil
}

// ProcessParlayCategory processes scoring for a specific parlay category
func (s *ParlayService) ProcessParlayCategory(ctx context.Context, season, week int, category models.ParlayCategory) error {
	// Get all users with picks for this week
	userIDs, err := s.getUniqueUserIDsForWeek(ctx, season, week)
	if err != nil {
		return fmt.Errorf("failed to get user IDs: %w", err)
	}

	// Process each user for this specific category
	for _, userID := range userIDs {
		points, err := s.CalculateUserParlayCategoryScore(ctx, userID, season, week, category)
		if err != nil {
			logging.Errorf("Failed to calculate %s score for user %d: %v", category, userID, err)
			continue
		}

		err = s.UpdateUserParlayCategoryRecord(ctx, userID, season, week, category, points)
		if err != nil {
			logging.Errorf("Failed to update %s record for user %d: %v", category, userID, err)
			continue
		}

		if points > 0 {
			logging.Infof("User %d scored %d points in %s category", userID, points, category)
		}
	}

	// CRITICAL: Broadcast parlay score updates via SSE after category scoring completes
	// This triggers real-time club score updates for all connected clients
	if s.broadcaster != nil {
		logging.Infof("Broadcasting parlay score updates for season %d, week %d after %s category completion", season, week, category)
		s.broadcaster.BroadcastParlayScoreUpdate(season, week)
	} else {
		logging.Warn("No broadcaster set for parlay score updates")
	}

	return nil
}

// CalculateUserParlayCategoryScore calculates score for specific parlay category
func (s *ParlayService) CalculateUserParlayCategoryScore(ctx context.Context, userID, season, week int, category models.ParlayCategory) (int, error) {
	// Get all scores and return just the requested category
	scores, err := s.CalculateUserParlayScore(ctx, userID, season, week)
	if err != nil {
		return 0, err
	}

	return scores[category], nil
}

// UpdateUserParlayCategoryRecord updates a specific category score for a user
func (s *ParlayService) UpdateUserParlayCategoryRecord(ctx context.Context, userID, season, week int, category models.ParlayCategory, points int) error {
	// For now, delegate to the full record update method
	// This could be optimized later to only update specific fields
	currentScores, err := s.CalculateUserParlayScore(ctx, userID, season, week)
	if err != nil {
		return err
	}

	currentScores[category] = points
	return s.UpdateUserParlayRecord(ctx, userID, season, week, currentScores)
}

// CheckWeekHasParlayScores checks if parlay scores have been calculated for a week
func (s *ParlayService) CheckWeekHasParlayScores(ctx context.Context, season, week int) (bool, error) {
	// Check MemoryParlayScorer for existing scores
	if s.memoryScorer != nil {
		scores := s.memoryScorer.GetWeekScores(season, week)
		return len(scores) > 0, nil
	}
	return false, nil
}

// ProcessDailyParlayScoring processes daily parlay scoring (for modern seasons)
func (s *ParlayService) ProcessDailyParlayScoring(ctx context.Context, season, week int) error {
	// Check for invalid data
	if season == 0 || week == 0 {
		logging.Errorf("Invalid ProcessDailyParlayScoring params - Season=%d, Week=%d", season, week)

		// Print stack trace to identify caller
		buf := make([]byte, 4096)
		n := runtime.Stack(buf, false)
		logging.Errorf("ProcessDailyParlayScoring stack trace:\n%s", buf[:n])
	}

	logging.Infof("Processing daily parlay scoring for season %d, week %d", season, week)

	// Get all games for the week
	games, err := s.gameRepo.GetGamesByWeekSeason(week, season)
	if err != nil {
		return fmt.Errorf("failed to get games: %w", err)
	}

	// Convert to slice of models.Game (not pointers)
	gameSlice := make([]models.Game, len(games))
	for i, game := range games {
		gameSlice[i] = *game
	}

	// Get all users with picks for this week
	userIDs, err := s.getUniqueUserIDsForWeek(ctx, season, week)
	if err != nil {
		return fmt.Errorf("failed to get user IDs: %w", err)
	}

	// Process each user's daily scores
	for _, userID := range userIDs {
		dailyScores, err := s.CalculateUserDailyParlayScores(ctx, userID, season, week, gameSlice)
		if err != nil {
			logging.Errorf("Failed to calculate daily scores for user %d: %v", userID, err)
			continue
		}

		err = s.UpdateUserDailyParlayRecord(ctx, userID, season, week, dailyScores)
		if err != nil {
			logging.Errorf("Failed to update daily record for user %d: %v", userID, err)
			continue
		}

		logging.Infof("Updated daily parlay scores for user %d: %+v", userID, dailyScores)
	}

	return nil
}

// CalculateUserDailyParlayScores calculates daily parlay scores for modern seasons
func (s *ParlayService) CalculateUserDailyParlayScores(ctx context.Context, userID, season, week int, games []models.Game) (map[string]int, error) {
	// Get user's picks for the week from WeeklyPicks document
	userPicks, err := s.getUserPicksForWeek(ctx, userID, season, week)
	if err != nil {
		return nil, fmt.Errorf("failed to get user picks: %w", err)
	}

	// Group games by day
	dayGroups := models.GroupGamesByDayName(games)

	// Calculate scores for each day
	dailyScores := make(map[string]int)

	for dayName, dayGames := range dayGroups {
		// Get picks for games on this day
		dayPicks := s.getPicksForGames(userPicks, dayGames)

		// Calculate parlay score for this day
		dailyScores[dayName] = s.calculateDayParlayScore(dayPicks)
	}

	return dailyScores, nil
}

// getPicksForGames filters picks to only include those for specified games
func (s *ParlayService) getPicksForGames(picks []models.Pick, games []models.Game) []models.Pick {
	gameIDs := make(map[int]bool)
	for _, game := range games {
		gameIDs[game.ID] = true
	}

	var filtered []models.Pick
	for _, pick := range picks {
		if gameIDs[pick.GameID] {
			filtered = append(filtered, pick)
		}
	}

	return filtered
}

// calculateDayParlayScore calculates the parlay score for picks on a specific day
func (s *ParlayService) calculateDayParlayScore(picks []models.Pick) int {
	if len(picks) < 2 {
		return 0
	}

	points := 0

	// All picks must be winners for parlay to pay out
	for _, pick := range picks {
		if pick.Result == models.PickResultLoss || pick.Result == models.PickResultPending {
			return 0
		}

		if pick.Result == models.PickResultWin {
			points += 1
		}
	}

	return points
}

// UpdateUserDailyParlayRecord updates user's daily parlay scores
func (s *ParlayService) UpdateUserDailyParlayRecord(ctx context.Context, userID, season, week int, dailyScores map[string]int) error {
	// Use MemoryParlayScorer to recalculate and store scores
	if s.memoryScorer != nil {
		_, err := s.memoryScorer.RecalculateUserScore(ctx, season, week, userID)
		if err != nil {
			return fmt.Errorf("failed to update user daily parlay record in memory: %w", err)
		}
	}

	return nil
}

// Helper methods to work with WeeklyPicks documents

// getUserPicksForWeek gets all picks for a specific user and week from WeeklyPicks document
func (s *ParlayService) getUserPicksForWeek(ctx context.Context, userID, season, week int) ([]models.Pick, error) {
	weeklyPicks, err := s.weeklyPicksRepo.FindByUserAndWeek(ctx, userID, season, week)
	if err != nil {
		return nil, fmt.Errorf("failed to get weekly picks: %w", err)
	}

	if weeklyPicks == nil {
		return []models.Pick{}, nil // No picks found for this user/week
	}

	return weeklyPicks.Picks, nil
}

// getUniqueUserIDsForWeek gets all unique user IDs who made picks in a specific week
func (s *ParlayService) getUniqueUserIDsForWeek(ctx context.Context, season, week int) ([]int, error) {
	weeklyPicksList, err := s.weeklyPicksRepo.FindAllByWeek(ctx, season, week)
	if err != nil {
		return nil, fmt.Errorf("failed to get weekly picks: %w", err)
	}

	// Extract unique user IDs
	userIDSet := make(map[int]bool)
	for _, weeklyPicks := range weeklyPicksList {
		userIDSet[weeklyPicks.UserID] = true
	}

	// Convert to slice
	var userIDs []int
	for userID := range userIDSet {
		userIDs = append(userIDs, userID)
	}

	return userIDs, nil
}
