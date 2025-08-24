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
type PickService struct {
	pickRepo *database.MongoPickRepository
	gameRepo *database.MongoGameRepository
	userRepo *database.MongoUserRepository
}

// NewPickService creates a new pick service
func NewPickService(pickRepo *database.MongoPickRepository, gameRepo *database.MongoGameRepository, userRepo *database.MongoUserRepository) *PickService {
	return &PickService{
		pickRepo: pickRepo,
		gameRepo: gameRepo,
		userRepo: userRepo,
	}
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
	
	// Get user's record for the season
	record, err := s.pickRepo.GetUserRecord(ctx, userID, season)
	if err != nil {
		return nil, fmt.Errorf("failed to get user record: %w", err)
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
		
		switch pick.PickType {
		case models.PickTypeSpread:
			userPicks.SpreadPicks = append(userPicks.SpreadPicks, *pick)
		case models.PickTypeOverUnder:
			userPicks.OverUnderPicks = append(userPicks.OverUnderPicks, *pick)
		}
		
		// Categorize bonus picks based on game day (this would need game data to determine)
		// For now, we'll leave bonus picks empty as we need to implement day-of-week logic
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
	
	// Group picks by user
	picksByUser := make(map[int][]*models.Pick)
	for _, pick := range allPicks {
		picksByUser[pick.UserID] = append(picksByUser[pick.UserID], pick)
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
		
		// Get user's record for the season
		record, err := s.pickRepo.GetUserRecord(ctx, user.ID, season)
		if err != nil {
			log.Printf("Warning: failed to get record for user %d: %v", user.ID, err)
			record = &models.UserRecord{} // Empty record on error
		}
		
		result = append(result, &models.UserPicks{
			UserID:   user.ID,
			UserName: user.Name,
			Picks:    picks,
			Record:   *record,
		})
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
		teamName := GetTeamName(teamAbbr)
		
		// Add spread information if available
		if game.Odds != nil && game.Odds.Spread != 0 {
			// Determine if this team is home or away to show correct spread
			if teamAbbr == game.Home {
				// Home team spread
				if game.Odds.Spread > 0 {
					pick.TeamName = fmt.Sprintf("%s +%.1f", teamName, game.Odds.Spread)
				} else {
					pick.TeamName = fmt.Sprintf("%s %.1f", teamName, game.Odds.Spread)
				}
			} else {
				// Away team spread (inverse of home spread)
				awaySpread := -game.Odds.Spread
				if awaySpread > 0 {
					pick.TeamName = fmt.Sprintf("%s +%.1f", teamName, awaySpread)
				} else {
					pick.TeamName = fmt.Sprintf("%s %.1f", teamName, awaySpread)
				}
			}
		} else {
			pick.TeamName = teamName
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