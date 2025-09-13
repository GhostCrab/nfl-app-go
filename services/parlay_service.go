package services

import (
	"context"
	"fmt"
	"log"
	"nfl-app-go/database"
	"nfl-app-go/models"
)

// ParlayService handles all parlay scoring logic separated from PickService
// This service is responsible for calculating weekly and daily parlay scores
// and updating user parlay records.
type ParlayService struct {
	pickRepo   *database.MongoPickRepository
	gameRepo   *database.MongoGameRepository  
	parlayRepo *database.MongoParlayRepository
}

// NewParlayService creates a new parlay service instance
func NewParlayService(
	pickRepo *database.MongoPickRepository,
	gameRepo *database.MongoGameRepository, 
	parlayRepo *database.MongoParlayRepository,
) *ParlayService {
	return &ParlayService{
		pickRepo:   pickRepo,
		gameRepo:   gameRepo,
		parlayRepo: parlayRepo,
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
	// Get user's picks for the week
	userPicks, err := s.pickRepo.GetUserPicksForWeek(ctx, userID, season, week)
	if err != nil {
		return nil, fmt.Errorf("failed to get user picks: %w", err)
	}

	if len(userPicks) == 0 {
		log.Printf("No picks found for user %d, season %d, week %d", userID, season, week)
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
	log.Printf("Processing parlay scoring for season %d, week %d", season, week)

	// Get all unique user IDs who made picks this week
	userIDs, err := s.pickRepo.GetUniqueUserIDsForWeek(ctx, season, week)
	if err != nil {
		return fmt.Errorf("failed to get user IDs: %w", err) 
	}

	log.Printf("Found %d users with picks for week %d", len(userIDs), week)

	// Process each user's parlay scores
	for _, userID := range userIDs {
		scores, err := s.CalculateUserParlayScore(ctx, userID, season, week)
		if err != nil {
			log.Printf("Failed to calculate parlay scores for user %d: %v", userID, err)
			continue
		}

		// Update the user's parlay record
		err = s.UpdateUserParlayRecord(ctx, userID, season, week, scores)
		if err != nil {
			log.Printf("Failed to update parlay record for user %d: %v", userID, err)
			continue
		}

		log.Printf("Updated parlay scores for user %d: %+v", userID, scores)
	}

	return nil
}

// UpdateUserParlayRecord updates a user's parlay scores in the database
func (s *ParlayService) UpdateUserParlayRecord(ctx context.Context, userID, season, week int, weeklyScores map[models.ParlayCategory]int) error {
	// Get or create the user's season record
	seasonRecord, err := s.parlayRepo.GetUserSeasonRecord(ctx, userID, season)
	if err != nil {
		return fmt.Errorf("failed to get season record: %w", err)
	}

	if seasonRecord == nil {
		// Create new season record
		seasonRecord = &models.ParlaySeasonRecord{
			UserID:     userID,
			Season:     season,
			WeekScores: make(map[int]models.ParlayWeekScore),
		}
	}

	// Update the week scores
	totalPoints := 0
	for _, points := range weeklyScores {
		totalPoints += points
	}

	weekScore := models.ParlayWeekScore{
		Week:            week,
		ThursdayPoints:  weeklyScores[models.ParlayBonusThursday],
		FridayPoints:    weeklyScores[models.ParlayBonusFriday],
		SundayMondayPoints: weeklyScores[models.ParlayRegular],
		TotalPoints:     totalPoints,
	}

	seasonRecord.WeekScores[week] = weekScore

	// Recalculate season totals
	seasonRecord.RecalculateTotals()

	// Save to database
	return s.parlayRepo.UpsertUserSeasonRecord(ctx, seasonRecord)
}

// ProcessParlayCategory processes scoring for a specific parlay category
func (s *ParlayService) ProcessParlayCategory(ctx context.Context, season, week int, category models.ParlayCategory) error {
	log.Printf("Processing parlay category %s for season %d, week %d", category, season, week)

	// Get all users with picks for this week
	userIDs, err := s.pickRepo.GetUniqueUserIDsForWeek(ctx, season, week)
	if err != nil {
		return fmt.Errorf("failed to get user IDs: %w", err)
	}

	// Process each user for this specific category
	for _, userID := range userIDs {
		points, err := s.CalculateUserParlayCategoryScore(ctx, userID, season, week, category)
		if err != nil {
			log.Printf("Failed to calculate %s score for user %d: %v", category, userID, err)
			continue
		}

		err = s.UpdateUserParlayCategoryRecord(ctx, userID, season, week, category, points)
		if err != nil {
			log.Printf("Failed to update %s record for user %d: %v", category, userID, err)
			continue
		}

		if points > 0 {
			log.Printf("User %d scored %d points in %s category", userID, points, category)
		}
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
	// Check if any users have parlay scores recorded for this week
	count, err := s.parlayRepo.CountUsersWithScoresForWeek(ctx, season, week)
	if err != nil {
		return false, fmt.Errorf("failed to check parlay scores: %w", err)
	}

	return count > 0, nil
}

// ProcessDailyParlayScoring processes daily parlay scoring (for modern seasons)
func (s *ParlayService) ProcessDailyParlayScoring(ctx context.Context, season, week int) error {
	log.Printf("Processing daily parlay scoring for season %d, week %d", season, week)

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
	userIDs, err := s.pickRepo.GetUniqueUserIDsForWeek(ctx, season, week)
	if err != nil {
		return fmt.Errorf("failed to get user IDs: %w", err)
	}

	// Process each user's daily scores
	for _, userID := range userIDs {
		dailyScores, err := s.CalculateUserDailyParlayScores(ctx, userID, season, week, gameSlice)
		if err != nil {
			log.Printf("Failed to calculate daily scores for user %d: %v", userID, err)
			continue
		}

		err = s.UpdateUserDailyParlayRecord(ctx, userID, season, week, dailyScores)
		if err != nil {
			log.Printf("Failed to update daily record for user %d: %v", userID, err)
			continue
		}

		log.Printf("Updated daily parlay scores for user %d: %+v", userID, dailyScores)
	}

	return nil
}

// CalculateUserDailyParlayScores calculates daily parlay scores for modern seasons
func (s *ParlayService) CalculateUserDailyParlayScores(ctx context.Context, userID, season, week int, games []models.Game) (map[string]int, error) {
	// Get user's picks for the week
	userPicks, err := s.pickRepo.GetUserPicksForWeek(ctx, userID, season, week)
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
	if len(picks) == 0 {
		return 0
	}

	// All picks must be winners for parlay to pay out
	for _, pick := range picks {
		if pick.Result != models.PickResultWin {
			return 0
		}
	}

	// Calculate payout using original legacy logic
	return len(picks)
}

// UpdateUserDailyParlayRecord updates user's daily parlay scores
func (s *ParlayService) UpdateUserDailyParlayRecord(ctx context.Context, userID, season, week int, dailyScores map[string]int) error {
	// Get or create user's season record
	seasonRecord, err := s.parlayRepo.GetUserSeasonRecord(ctx, userID, season)
	if err != nil {
		return fmt.Errorf("failed to get season record: %w", err)
	}

	if seasonRecord == nil {
		seasonRecord = &models.ParlaySeasonRecord{
			UserID:     userID,
			Season:     season,
			WeekScores: make(map[int]models.ParlayWeekScore),
		}
	}

	// Update week record with daily scores
	weekScore, exists := seasonRecord.WeekScores[week]
	if !exists {
		weekScore = models.ParlayWeekScore{
			Week: week,
		}
	}

	// Set daily scores
	weekScore.DailyScores = dailyScores
	
	// Calculate total from daily scores
	totalPoints := 0
	for _, points := range dailyScores {
		totalPoints += points
	}
	weekScore.TotalPoints = totalPoints

	seasonRecord.WeekScores[week] = weekScore
	
	// Recalculate season totals
	seasonRecord.RecalculateTotals()

	// Save to database
	return s.parlayRepo.UpsertUserSeasonRecord(ctx, seasonRecord)
}