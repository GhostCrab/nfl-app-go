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
// Returns map of parlay category to points earned
func (s *ParlayService) CalculateUserParlayScore(ctx context.Context, userID, season, week int) (map[models.ParlayCategory]int, error) {
	// Get user's picks for the week
	userPicks, err := s.pickRepo.GetUserPicksForWeek(ctx, userID, season, week)
	if err != nil {
		return nil, fmt.Errorf("failed to get user picks: %w", err)
	}

	if len(userPicks) == 0 {
		log.Printf("No picks found for user %d, season %d, week %d", userID, season, week)
		return map[models.ParlayCategory]int{}, nil
	}

	// Get games for the week to validate completion
	games, err := s.gameRepo.GetGamesByWeekSeason(week, season) 
	if err != nil {
		return nil, fmt.Errorf("failed to get games: %w", err)
	}

	// Create game map for quick lookup
	gameMap := make(map[int]models.Game)
	for _, game := range games {
		gameMap[game.ID] = *game
	}

	// Calculate scores for each parlay category
	scores := make(map[models.ParlayCategory]int)
	
	// Calculate Thursday category
	scores[models.ParlayBonusThursday] = s.calculateCategoryScore(userPicks, gameMap, models.ParlayBonusThursday)
	
	// Calculate Friday category  
	scores[models.ParlayBonusFriday] = s.calculateCategoryScore(userPicks, gameMap, models.ParlayBonusFriday)
	
	// Calculate SundayMonday category
	scores[models.ParlayRegular] = s.calculateCategoryScore(userPicks, gameMap, models.ParlayRegular)

	return scores, nil
}

// calculateCategoryScore calculates the parlay score for a specific category
func (s *ParlayService) calculateCategoryScore(picks []models.Pick, gameMap map[int]models.Game, category models.ParlayCategory) int {
	categoryPicks := s.filterPicksByCategory(picks, gameMap, category)
	
	if len(categoryPicks) == 0 {
		return 0
	}

	// All picks must be winners for parlay to pay out
	allWinners := true
	for _, pick := range categoryPicks {
		if pick.Result != models.PickResultWin {
			allWinners = false
			break
		}
	}

	if !allWinners {
		return 0
	}

	// Calculate parlay payout based on number of picks
	return s.calculateParlayPayout(len(categoryPicks))
}

// filterPicksByCategory filters picks based on the parlay category
func (s *ParlayService) filterPicksByCategory(picks []models.Pick, gameMap map[int]models.Game, category models.ParlayCategory) []models.Pick {
	var filtered []models.Pick
	
	for _, pick := range picks {
		game, exists := gameMap[pick.GameID]
		if !exists {
			continue
		}

		dayName := game.GetGameDayName()
		
		switch category {
		case models.ParlayBonusThursday:
			if dayName == "Thursday" {
				filtered = append(filtered, pick)
			}
		case models.ParlayBonusFriday:
			if dayName == "Friday" {
				filtered = append(filtered, pick)
			}
		case models.ParlayRegular:
			if dayName == "Sunday" || dayName == "Monday" {
				filtered = append(filtered, pick)
			}
		}
	}
	
	return filtered
}

// calculateParlayPayout calculates points based on number of picks in parlay
func (s *ParlayService) calculateParlayPayout(numPicks int) int {
	switch numPicks {
	case 1:
		return 1 // Single pick = 1 point
	case 2:
		return 3 // 2-pick parlay = 3 points  
	case 3:
		return 7 // 3-pick parlay = 7 points
	case 4:
		return 15 // 4-pick parlay = 15 points
	case 5:
		return 31 // 5-pick parlay = 31 points
	default:
		// For 6+ picks, continue exponential growth pattern
		// Formula: (2^n) - 1 where n is number of picks
		result := 1
		for i := 0; i < numPicks; i++ {
			result *= 2
		}
		return result - 1
	}
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

	// Calculate payout based on number of picks
	return s.calculateParlayPayout(len(picks))
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