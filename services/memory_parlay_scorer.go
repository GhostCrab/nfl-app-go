package services

import (
	"context"
	"fmt"
	"nfl-app-go/logging"
	"nfl-app-go/models"
	"sync"
	"time"
)

// WeeklyUserScores represents all users' scores for a specific week
type WeeklyUserScores struct {
	Season     int
	Week       int
	UserScores map[int]*models.ParlayScore // userID -> score
	UpdatedAt  time.Time
}

// MemoryParlayScorer manages parlay scores in memory
type MemoryParlayScorer struct {
	mu            sync.RWMutex
	weeklyScores  map[string]*WeeklyUserScores // "season-week" -> scores
	parlayService *ParlayService
	pickService   *PickService
	logger        *logging.Logger
}

// NewMemoryParlayScorer creates a new in-memory parlay scorer
func NewMemoryParlayScorer(parlayService *ParlayService, pickService *PickService) *MemoryParlayScorer {
	return &MemoryParlayScorer{
		weeklyScores:  make(map[string]*WeeklyUserScores),
		parlayService: parlayService,
		pickService:   pickService,
		logger:        logging.WithPrefix("MemoryParlayScorer"),
	}
}

// GetUserScore gets a user's parlay score for a specific week
func (mps *MemoryParlayScorer) GetUserScore(season, week, userID int) (*models.ParlayScore, bool) {
	mps.mu.RLock()
	defer mps.mu.RUnlock()

	key := fmt.Sprintf("%d-%d", season, week)
	weekScores, exists := mps.weeklyScores[key]
	if !exists {
		return nil, false
	}

	userScore, exists := weekScores.UserScores[userID]
	if !exists {
		return nil, false
	}

	// Return a copy to prevent external modifications
	scoreCopy := *userScore
	return &scoreCopy, true
}

// GetWeekScores gets all users' scores for a specific week
func (mps *MemoryParlayScorer) GetWeekScores(season, week int) []*models.ParlayScore {
	mps.mu.RLock()
	defer mps.mu.RUnlock()

	key := fmt.Sprintf("%d-%d", season, week)
	weekScores, exists := mps.weeklyScores[key]
	if !exists {
		return []*models.ParlayScore{}
	}

	scores := make([]*models.ParlayScore, 0, len(weekScores.UserScores))
	for _, score := range weekScores.UserScores {
		scoreCopy := *score
		scores = append(scores, &scoreCopy)
	}

	return scores
}

// CalculateAndStoreWeekScores calculates and stores scores for all users in a specific week
func (mps *MemoryParlayScorer) CalculateAndStoreWeekScores(ctx context.Context, season, week int) error {
	// Get all users who made picks for this week
	allUserScores, err := mps.pickService.CalculateAllUsersParlayScores(ctx, season, week)
	if err != nil {
		return fmt.Errorf("failed to calculate parlay scores: %w", err)
	}

	// Convert to ParlayScore models
	userScores := make(map[int]*models.ParlayScore)
	for userID, scores := range allUserScores {
		parlayScore := models.CreateParlayScore(userID, season, week, scores)
		userScores[userID] = parlayScore
	}

	// Store in memory
	mps.mu.Lock()
	defer mps.mu.Unlock()

	key := fmt.Sprintf("%d-%d", season, week)
	mps.weeklyScores[key] = &WeeklyUserScores{
		Season:     season,
		Week:       week,
		UserScores: userScores,
		UpdatedAt:  time.Now(),
	}

	return nil
}

// RecalculateUserScore recalculates and updates a specific user's score for a week
func (mps *MemoryParlayScorer) RecalculateUserScore(ctx context.Context, season, week, userID int) (*models.ParlayScore, error) {
	// Calculate fresh scores
	scores, err := mps.parlayService.CalculateUserParlayScore(ctx, userID, season, week)
	if err != nil {
		return nil, fmt.Errorf("failed to calculate user parlay score: %w", err)
	}

	parlayScore := models.CreateParlayScore(userID, season, week, scores)

	// Update in memory
	mps.mu.Lock()
	defer mps.mu.Unlock()

	key := fmt.Sprintf("%d-%d", season, week)
	weekScores, exists := mps.weeklyScores[key]
	if !exists {
		// Initialize week if it doesn't exist
		weekScores = &WeeklyUserScores{
			Season:     season,
			Week:       week,
			UserScores: make(map[int]*models.ParlayScore),
			UpdatedAt:  time.Now(),
		}
		mps.weeklyScores[key] = weekScores
	}

	weekScores.UserScores[userID] = parlayScore
	weekScores.UpdatedAt = time.Now()

	mps.logger.Debugf("Updated score for user %d: %d points (season %d, week %d)",
		userID, parlayScore.TotalPoints, season, week)

	return parlayScore, nil
}

// InitializeFromDatabase loads existing scores from database on startup
func (mps *MemoryParlayScorer) InitializeFromDatabase(ctx context.Context, currentSeason int) error {
	mps.logger.Info("Initializing parlay scores from database calculations")

	if mps.parlayService == nil {
		mps.logger.Error("ParlayService is nil, cannot initialize scores")
		return fmt.Errorf("parlayService is nil")
	}

	if mps.pickService == nil {
		mps.logger.Error("PickService is nil, cannot initialize scores")
		return fmt.Errorf("pickService is nil")
	}

	// For now, we'll calculate scores for the current season
	// In the future, you might want to calculate historical scores as needed
	for week := 1; week <= 18; week++ {
		err := mps.CalculateAndStoreWeekScores(ctx, currentSeason, week)
		if err != nil {
			mps.logger.Warnf("Failed to calculate scores for season %d, week %d: %v",
				currentSeason, week, err)
			continue
		}
	}

	mps.logger.Infof("Finished initializing parlay scores for season %d", currentSeason)
	return nil
}

// GetMemoryStats returns statistics about the in-memory data
func (mps *MemoryParlayScorer) GetMemoryStats() map[string]interface{} {
	mps.mu.RLock()
	defer mps.mu.RUnlock()

	totalUsers := 0
	for _, weekScores := range mps.weeklyScores {
		totalUsers += len(weekScores.UserScores)
	}

	return map[string]interface{}{
		"total_weeks_stored": len(mps.weeklyScores),
		"total_user_scores":  totalUsers,
		"weeks": func() []string {
			weeks := make([]string, 0, len(mps.weeklyScores))
			for key := range mps.weeklyScores {
				weeks = append(weeks, key)
			}
			return weeks
		}(),
	}
}
