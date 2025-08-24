package services

import (
	"context"
	"fmt"
	"log"
	"time"

	"nfl-app-go/database"
	"nfl-app-go/models"
)

// ScoringService handles parlay club scoring logic
type ScoringService struct {
	pickRepo         *database.MongoPickRepository
	gameRepo         *database.MongoGameRepository
	weeklyScoreRepo  *database.MongoWeeklyScoreRepository
}

// NewScoringService creates a new scoring service
func NewScoringService(pickRepo *database.MongoPickRepository, gameRepo *database.MongoGameRepository, weeklyScoreRepo *database.MongoWeeklyScoreRepository) *ScoringService {
	return &ScoringService{
		pickRepo:        pickRepo,
		gameRepo:        gameRepo,
		weeklyScoreRepo: weeklyScoreRepo,
	}
}

// DayType constants
const (
	DayTypeRegular       = "regular"
	DayTypeBonusThursday = "bonus_thursday"
	DayTypeBonusFriday   = "bonus_friday"
)

// GetDayType determines what type of day a pick belongs to based on game date and season rules
func (s *ScoringService) GetDayType(gameDate time.Time, week, season int) string {
	dayOfWeek := gameDate.Weekday()
	
	// Check if this is a bonus week for the season
	isBonusWeek := s.IsBonusWeek(week, season)
	if !isBonusWeek {
		return DayTypeRegular
	}
	
	// In bonus weeks, Thursday and Friday games are separate scoring categories
	switch dayOfWeek {
	case time.Thursday:
		if s.ShouldShowBonusThursday(week, season) {
			return DayTypeBonusThursday
		}
	case time.Friday:
		if s.ShouldShowBonusFriday(week, season) {
			return DayTypeBonusFriday
		}
	}
	
	return DayTypeRegular
}

// IsBonusWeek checks if a week has bonus games for the given season
func (s *ScoringService) IsBonusWeek(week, season int) bool {
	// Week 1 (opening week) is always bonus
	if week == 1 {
		return true
	}
	
	// Thanksgiving week (varies by season)
	thanksgivingWeek := s.GetThanksgivingWeek(season)
	return week == thanksgivingWeek
}

// ShouldShowBonusThursday checks if Thursday should be a bonus day
func (s *ScoringService) ShouldShowBonusThursday(week, season int) bool {
	// All seasons have bonus Thursday for opening and thanksgiving weeks
	return s.IsBonusWeek(week, season)
}

// ShouldShowBonusFriday checks if Friday should be a bonus day
func (s *ScoringService) ShouldShowBonusFriday(week, season int) bool {
	if !s.IsBonusWeek(week, season) {
		return false
	}
	
	if season >= 2025 {
		// 2025+: Friday bonus for both opening week and thanksgiving week
		return true
	} else if season >= 2024 {
		// 2024: Friday bonus only for thanksgiving week
		thanksgivingWeek := s.GetThanksgivingWeek(season)
		return week == thanksgivingWeek
	}
	
	// 2023: No Friday bonus games
	return false
}

// GetThanksgivingWeek calculates thanksgiving week for a season (simplified version)
func (s *ScoringService) GetThanksgivingWeek(season int) int {
	// Known values - could be enhanced with calculation later
	knownThanksgivingWeeks := map[int]int{
		2023: 12,
		2024: 13,
		2025: 13,
	}
	
	if week, exists := knownThanksgivingWeeks[season]; exists {
		return week
	}
	
	// Default fallback
	return 13
}

// RecalculateWeeklyScores recalculates all scores for a specific week
func (s *ScoringService) RecalculateWeeklyScores(ctx context.Context, season, week int) error {
	log.Printf("ScoringService: Recalculating scores for season %d, week %d", season, week)
	
	// Get all picks for the week
	allPicks, err := s.pickRepo.FindByWeek(ctx, season, week)
	if err != nil {
		return fmt.Errorf("failed to get picks for week: %w", err)
	}
	
	// Get all games for the week to determine day types
	games, err := s.gameRepo.GetGamesByWeekSeason(week, season)
	if err != nil {
		return fmt.Errorf("failed to get games for week: %w", err)
	}
	
	// Create game ID to game mapping
	gameMap := make(map[int]*models.Game)
	for _, game := range games {
		gameMap[game.ID] = game
	}
	
	// Group picks by user and day type
	userDayPicks := make(map[int]map[string][]*models.Pick)
	
	for _, pick := range allPicks {
		if pick.UserID < 0 {
			continue // Skip invalid user IDs (negative values only)
		}
		
		// Get game to determine day type
		game, exists := gameMap[pick.GameID]
		if !exists {
			log.Printf("Warning: Pick references unknown game ID %d", pick.GameID)
			continue
		}
		
		dayType := s.GetDayType(game.Date, week, season)
		
		if userDayPicks[pick.UserID] == nil {
			userDayPicks[pick.UserID] = make(map[string][]*models.Pick)
		}
		userDayPicks[pick.UserID][dayType] = append(userDayPicks[pick.UserID][dayType], pick)
	}
	
	// Calculate and save weekly scores for each user/day combination
	for userID, dayTypePicks := range userDayPicks {
		for dayType, picks := range dayTypePicks {
			if len(picks) == 0 {
				continue
			}
			
			// Create or update weekly score
			weeklyScore := &models.WeeklyScore{
				UserID:    userID,
				Season:    season,
				Week:      week,
				DayType:   dayType,
				CreatedAt: time.Now(),
			}
			
			weeklyScore.UpdateFromPicks(picks)
			
			if err := s.weeklyScoreRepo.Upsert(ctx, weeklyScore); err != nil {
				log.Printf("Error saving weekly score for user %d, week %d, dayType %s: %v", userID, week, dayType, err)
				continue
			}
			
			log.Printf("Updated score: User %d, Week %d, DayType %s, Points: %d", userID, week, dayType, weeklyScore.Points)
		}
	}
	
	return nil
}

// GetUserSeasonScore gets a user's total season score with weekly breakdown
func (s *ScoringService) GetUserSeasonScore(ctx context.Context, userID, season int) (*models.SeasonScore, error) {
	weeklyScores, err := s.weeklyScoreRepo.FindByUserSeason(ctx, userID, season)
	if err != nil {
		return nil, fmt.Errorf("failed to get user weekly scores: %w", err)
	}
	
	seasonScore := &models.SeasonScore{
		UserID:          userID,
		Season:          season,
		WeeklyBreakdown: make(map[int]models.WeekSummary),
		UpdatedAt:       time.Now(),
	}
	
	// Group by week
	weeklyGrouped := make(map[int][]*models.WeeklyScore)
	for _, ws := range weeklyScores {
		weeklyGrouped[ws.Week] = append(weeklyGrouped[ws.Week], ws)
	}
	
	// Calculate weekly summaries
	for week, weekScores := range weeklyGrouped {
		summary := models.WeekSummary{Week: week}
		
		for _, ws := range weekScores {
			switch ws.DayType {
			case DayTypeRegular:
				summary.RegularPoints = ws.Points
			case DayTypeBonusThursday:
				summary.BonusThurPoints = ws.Points
			case DayTypeBonusFriday:
				summary.BonusFriPoints = ws.Points
			}
		}
		
		summary.WeekTotal = summary.GetWeekTotal()
		seasonScore.WeeklyBreakdown[week] = summary
		seasonScore.TotalPoints += summary.WeekTotal
	}
	
	return seasonScore, nil
}

// GetWeeklyPointsForUser returns how many points a user earned in a specific week
func (s *ScoringService) GetWeeklyPointsForUser(ctx context.Context, userID, season, week int) (int, error) {
	weeklyScores, err := s.weeklyScoreRepo.FindByUserSeasonWeek(ctx, userID, season, week)
	if err != nil {
		return 0, fmt.Errorf("failed to get weekly scores: %w", err)
	}
	
	totalPoints := 0
	for _, ws := range weeklyScores {
		totalPoints += ws.Points
	}
	
	return totalPoints, nil
}