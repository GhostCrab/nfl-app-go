package services

import (
	"context"
	"log"
	"nfl-app-go/models"
	"time"
)

// PickVisibilityService manages pick visibility rules and timing
type PickVisibilityService struct {
	visibilityModel *models.PickVisibilityService
	gameService     GameService
}

// NewPickVisibilityService creates a new pick visibility service
func NewPickVisibilityService(gameService GameService) *PickVisibilityService {
	return &PickVisibilityService{
		visibilityModel: models.NewPickVisibilityService(),
		gameService:     gameService,
	}
}

// SetDebugDateTime sets debug time for testing visibility rules
func (s *PickVisibilityService) SetDebugDateTime(debugTime time.Time) {
	s.visibilityModel.SetDebugDateTime(debugTime)
	log.Printf("PickVisibilityService: Debug time set to %v", debugTime.Format("2006-01-02 15:04:05 MST"))
}

// ClearDebugDateTime removes debug time override
func (s *PickVisibilityService) ClearDebugDateTime() {
	s.visibilityModel.ClearDebugDateTime()
	log.Printf("PickVisibilityService: Debug time cleared, using real time")
}

// GetCurrentTime returns the current effective time (debug or real)
func (s *PickVisibilityService) GetCurrentTime() time.Time {
	return s.visibilityModel.GetCurrentTime()
}

// FilterVisibleUserPicks filters user picks based on visibility rules for the viewing user
func (s *PickVisibilityService) FilterVisibleUserPicks(ctx context.Context, userPicks []*models.UserPicks, season, week int, viewingUserID int) ([]*models.UserPicks, error) {
	// Get games for the week to determine visibility
	var games []models.Game
	var err error
	
	if gameServiceWithSeason, ok := s.gameService.(interface{ GetGamesBySeason(int) ([]models.Game, error) }); ok {
		games, err = gameServiceWithSeason.GetGamesBySeason(season)
		if err != nil {
			return nil, err
		}
		
		// Filter to this week's games
		weekGames := make([]models.Game, 0)
		for _, game := range games {
			if game.Week == week {
				weekGames = append(weekGames, game)
			}
		}
		games = weekGames
	} else {
		games, err = s.gameService.GetGames()
		if err != nil {
			return nil, err
		}
	}
	
	// Filter picks for each user
	filteredUserPicks := make([]*models.UserPicks, 0, len(userPicks))
	
	for _, userPicksEntry := range userPicks {
		if userPicksEntry == nil {
			continue
		}
		
		// Create filtered copy
		filteredEntry := &models.UserPicks{
			UserID:   userPicksEntry.UserID,
			UserName: userPicksEntry.UserName,
			Record:   userPicksEntry.Record,
		}
		
		// Filter each pick category
		filteredEntry.Picks = s.visibilityModel.GetVisiblePicksForUser(userPicksEntry.Picks, games, viewingUserID)
		filteredEntry.SpreadPicks = s.visibilityModel.GetVisiblePicksForUser(userPicksEntry.SpreadPicks, games, viewingUserID)
		filteredEntry.OverUnderPicks = s.visibilityModel.GetVisiblePicksForUser(userPicksEntry.OverUnderPicks, games, viewingUserID)
		filteredEntry.BonusThursdayPicks = s.visibilityModel.GetVisiblePicksForUser(userPicksEntry.BonusThursdayPicks, games, viewingUserID)
		filteredEntry.BonusFridayPicks = s.visibilityModel.GetVisiblePicksForUser(userPicksEntry.BonusFridayPicks, games, viewingUserID)
		
		// Get hidden pick counts for this user (use only the main Picks array to avoid duplicates)
		hiddenCounts := s.visibilityModel.GetHiddenPickCounts(userPicksEntry.Picks, games, viewingUserID)
		
		// Add hidden count metadata (we'll extend UserPicks model for this)
		if hasHiddenPicks(hiddenCounts) {
			// Store hidden counts in a way the template can access
			// For now, we'll add them as a special field extension
			filteredEntry.HiddenPickCounts = hiddenCounts
		}
		
		filteredUserPicks = append(filteredUserPicks, filteredEntry)
	}
	
	return filteredUserPicks, nil
}

// GetVisibilityStatus returns visibility information for all games in a week
func (s *PickVisibilityService) GetVisibilityStatus(ctx context.Context, season, week int) (map[int]models.PickVisibility, error) {
	// Get games for the week
	var games []models.Game
	var err error
	
	if gameServiceWithSeason, ok := s.gameService.(interface{ GetGamesBySeason(int) ([]models.Game, error) }); ok {
		games, err = gameServiceWithSeason.GetGamesBySeason(season)
		if err != nil {
			return nil, err
		}
		
		// Filter to this week's games
		weekGames := make([]models.Game, 0)
		for _, game := range games {
			if game.Week == week {
				weekGames = append(weekGames, game)
			}
		}
		games = weekGames
	} else {
		games, err = s.gameService.GetGames()
		if err != nil {
			return nil, err
		}
	}
	
	// Calculate visibility for each game
	visibilityMap := make(map[int]models.PickVisibility)
	for _, game := range games {
		visibility := s.visibilityModel.CalculateVisibility(game)
		visibilityMap[game.ID] = visibility
	}
	
	return visibilityMap, nil
}

// GetNextVisibilityChange returns the next time pick visibility will change
func (s *PickVisibilityService) GetNextVisibilityChange(ctx context.Context, season, week int) (*time.Time, error) {
	visibilityMap, err := s.GetVisibilityStatus(ctx, season, week)
	if err != nil {
		return nil, err
	}
	
	currentTime := s.GetCurrentTime()
	var nextChange *time.Time
	
	for _, visibility := range visibilityMap {
		// Skip games that are already visible
		if visibility.IsVisible {
			continue
		}
		
		// Check if this visibility change is sooner than current next change
		if nextChange == nil || visibility.VisibleAt.Before(*nextChange) {
			nextChange = &visibility.VisibleAt
		}
	}
	
	// Only return future times
	if nextChange != nil && nextChange.Before(currentTime) {
		return nil, nil // No future visibility changes
	}
	
	return nextChange, nil
}

// ShouldTriggerVisibilityUpdate checks if visibility has changed since last check
func (s *PickVisibilityService) ShouldTriggerVisibilityUpdate(ctx context.Context, season, week int, lastCheck time.Time) (bool, error) {
	visibilityMap, err := s.GetVisibilityStatus(ctx, season, week)
	if err != nil {
		return false, err
	}
	
	currentTime := s.GetCurrentTime()
	
	// Check if any games became visible since last check
	for _, visibility := range visibilityMap {
		if visibility.VisibleAt.After(lastCheck) && visibility.VisibleAt.Before(currentTime) {
			return true, nil
		}
	}
	
	return false, nil
}

// Helper function to check if there are any hidden picks
func hasHiddenPicks(counts map[string]int) bool {
	for _, count := range counts {
		if count > 0 {
			return true
		}
	}
	return false
}