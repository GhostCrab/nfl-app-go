package services

import (
	"context"
	"fmt"
	"log"
	"nfl-app-go/models"
	"nfl-app-go/database"
)

// PickServiceInterface defines the methods needed from PickService for result updates
type PickServiceInterface interface {
	UpdatePickResultsByGame(ctx context.Context, season, week, gameID int, pickResults map[int]models.PickResult) error
	UpdateIndividualPickResults(ctx context.Context, season, week, gameID int, pickUpdates []database.PickUpdate) error
}

// ResultCalculationService handles all game result calculations separated from PickService
// This service is responsible for calculating pick results based on game outcomes
// and determining spread, over/under, and moneyline results.
type ResultCalculationService struct {
	weeklyPicksRepo *database.MongoWeeklyPicksRepository
	gameRepo        *database.MongoGameRepository
	pickService     PickServiceInterface
}

// NewResultCalculationService creates a new result calculation service
func NewResultCalculationService(
	weeklyPicksRepo *database.MongoWeeklyPicksRepository,
	gameRepo *database.MongoGameRepository,
	pickService PickServiceInterface,
) *ResultCalculationService {
	return &ResultCalculationService{
		weeklyPicksRepo: weeklyPicksRepo,
		gameRepo:        gameRepo,
		pickService:     pickService,
	}
}

// ProcessGameCompletion processes all picks for a completed game and calculates results
func (s *ResultCalculationService) ProcessGameCompletion(ctx context.Context, game *models.Game) error {
	if !game.IsCompleted() {
		return fmt.Errorf("game %d is not completed yet", game.ID)
	}

	log.Printf("Processing game completion for game %d: %s @ %s (Final: %d-%d)", 
		game.ID, game.Away, game.Home, game.AwayScore, game.HomeScore)

	// Get all picks for this game from WeeklyPicks documents
	picks, err := s.getPicksByGameID(ctx, game.ID)
	if err != nil {
		return fmt.Errorf("failed to get picks for game %d: %w", game.ID, err)
	}

	if len(picks) == 0 {
		log.Printf("No picks found for game %d", game.ID)
		return nil
	}

	log.Printf("Processing %d picks for game %d", len(picks), game.ID)

	// Calculate results for each pick individually
	var pickUpdates []database.PickUpdate
	for _, pick := range picks {
		result := s.CalculatePickResult(&pick, game)

		pickUpdate := database.PickUpdate{
			UserID:   pick.UserID,
			PickType: string(pick.PickType),
			Result:   result,
		}
		pickUpdates = append(pickUpdates, pickUpdate)

		log.Printf("Calculated result for user %d game %d pick %s: %s -> %s",
			pick.UserID, pick.GameID, pick.PickType, pick.PickDescription, result)
	}

	// Update all pick results individually
	if len(pickUpdates) > 0 {
		if err := s.pickService.UpdateIndividualPickResults(ctx, game.Season, game.Week, game.ID, pickUpdates); err != nil {
			return fmt.Errorf("failed to update pick results: %w", err)
		}
		log.Printf("Updated %d individual pick results for game %d", len(pickUpdates), game.ID)
	}

	return nil
}

// CalculatePickResult determines the result of a pick based on the completed game
func (s *ResultCalculationService) CalculatePickResult(pick *models.Pick, game *models.Game) models.PickResult {
	if !game.IsCompleted() {
		return models.PickResultPending
	}

	switch pick.PickType {
	case models.PickTypeSpread:
		return s.calculateSpreadResult(pick, game)
	case models.PickTypeOverUnder:
		return s.calculateOverUnderResult(pick, game)
	case models.PickTypeMoneyline:
		return s.calculateMoneylineResult(pick, game)
	default:
		log.Printf("Unknown pick type: %s", pick.PickType)
		return models.PickResultPending
	}
}

// calculateSpreadResult determines the result of a spread pick
func (s *ResultCalculationService) calculateSpreadResult(pick *models.Pick, game *models.Game) models.PickResult {
	if !game.HasOdds() {
		log.Printf("Game %d has no odds for spread calculation", game.ID)
		return models.PickResultPending
	}

	// Determine which team was picked based on TeamID
	var isHomePick bool
	
	// Get team abbreviations from game data to match with pick
	homeTeamID := s.getTeamIDFromAbbreviation(game.Home)
	awayTeamID := s.getTeamIDFromAbbreviation(game.Away)
	
	if pick.TeamID == homeTeamID {
		isHomePick = true
	} else if pick.TeamID == awayTeamID {
		isHomePick = false
	} else {
		log.Printf("Could not determine team for pick (User: %d, Game: %d, TeamID: %d, Home: %s/%d, Away: %s/%d)",
			pick.UserID, pick.GameID, pick.TeamID, game.Home, homeTeamID, game.Away, awayTeamID)
		return models.PickResultPending
	}

	// Calculate spread result
	scoreDiff := game.HomeScore - game.AwayScore // Positive = home winning
	spreadDiff := float64(scoreDiff) + game.Odds.Spread // Apply spread
	
	if isHomePick {
		// Home team pick - need home team to cover
		if spreadDiff > 0 {
			return models.PickResultWin
		} else if spreadDiff < 0 {
			return models.PickResultLoss
		} else {
			return models.PickResultPush
		}
	} else {
		// Away team pick - need away team to cover
		if spreadDiff < 0 {
			return models.PickResultWin
		} else if spreadDiff > 0 {
			return models.PickResultLoss
		} else {
			return models.PickResultPush
		}
	}
}

// calculateOverUnderResult determines the result of an over/under pick
func (s *ResultCalculationService) calculateOverUnderResult(pick *models.Pick, game *models.Game) models.PickResult {
	if !game.HasOdds() {
		log.Printf("Game %d has no odds for over/under calculation", game.ID)
		return models.PickResultPending
	}

	totalScore := float64(game.HomeScore + game.AwayScore)
	overUnderLine := game.Odds.OU
	
	// TeamID 99 = Over, TeamID 98 = Under (legacy convention)
	isOverPick := pick.TeamID == 99
	
	if isOverPick {
		// Over pick
		if totalScore > overUnderLine {
			return models.PickResultWin
		} else if totalScore < overUnderLine {
			return models.PickResultLoss
		} else {
			return models.PickResultPush
		}
	} else {
		// Under pick
		if totalScore < overUnderLine {
			return models.PickResultWin
		} else if totalScore > overUnderLine {
			return models.PickResultLoss
		} else {
			return models.PickResultPush
		}
	}
}

// calculateMoneylineResult determines the result of a moneyline pick
func (s *ResultCalculationService) calculateMoneylineResult(pick *models.Pick, game *models.Game) models.PickResult {
	winner := game.Winner()
	
	if winner == "" {
		// Tie game - moneyline bets typically push
		return models.PickResultPush
	}
	
	// Determine which team was picked
	homeTeamID := s.getTeamIDFromAbbreviation(game.Home)
	awayTeamID := s.getTeamIDFromAbbreviation(game.Away)
	
	var pickedTeam string
	if pick.TeamID == homeTeamID {
		pickedTeam = game.Home
	} else if pick.TeamID == awayTeamID {
		pickedTeam = game.Away
	} else {
		log.Printf("Could not determine team for moneyline pick (User: %d, Game: %d, TeamID: %d)", pick.UserID, pick.GameID, pick.TeamID)
		return models.PickResultPending
	}
	
	if pickedTeam == winner {
		return models.PickResultWin
	} else {
		return models.PickResultLoss
	}
}

// getTeamIDFromAbbreviation maps team abbreviation to ESPN team ID
// This is a simplified mapping - in a real system this would come from a database
func (s *ResultCalculationService) getTeamIDFromAbbreviation(abbr string) int {
	// ESPN team ID mapping (simplified version)
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
	
	log.Printf("Unknown team abbreviation: %s", abbr)
	return 0
}

// GetPickStatisticsForGame returns statistics for picks on a specific game
func (s *ResultCalculationService) GetPickStatisticsForGame(ctx context.Context, gameID int) (GamePickStats, error) {
	picks, err := s.getPicksByGameID(ctx, gameID)
	if err != nil {
		return GamePickStats{}, fmt.Errorf("failed to get picks: %w", err)
	}

	stats := GamePickStats{
		GameID:     gameID,
		TotalPicks: len(picks),
		Results:    make(map[models.PickResult]int),
		PickTypes:  make(map[models.PickType]int),
	}

	// Count results and pick types
	for _, pick := range picks {
		stats.Results[pick.Result]++
		stats.PickTypes[pick.PickType]++
	}

	return stats, nil
}

// GamePickStats represents statistics for picks on a specific game
type GamePickStats struct {
	GameID     int                               `json:"game_id"`
	TotalPicks int                               `json:"total_picks"`
	Results    map[models.PickResult]int         `json:"results"`
	PickTypes  map[models.PickType]int           `json:"pick_types"`
}

// ValidatePickAgainstGame validates that a pick is valid for the given game
func (s *ResultCalculationService) ValidatePickAgainstGame(pick *models.Pick, game *models.Game) error {
	// Check if game allows picks (not completed/in progress)
	if game.IsCompleted() || game.IsInProgress() {
		return fmt.Errorf("cannot place pick on game that has started or completed")
	}

	// Validate spread picks
	if pick.PickType == models.PickTypeSpread {
		if !game.HasOdds() {
			return fmt.Errorf("game has no spread odds available")
		}
		
		homeTeamID := s.getTeamIDFromAbbreviation(game.Home)
		awayTeamID := s.getTeamIDFromAbbreviation(game.Away)
		
		if pick.TeamID != homeTeamID && pick.TeamID != awayTeamID {
			return fmt.Errorf("invalid team ID %d for spread pick (valid: %d, %d)", 
				pick.TeamID, homeTeamID, awayTeamID)
		}
	}

	// Validate over/under picks  
	if pick.PickType == models.PickTypeOverUnder {
		if !game.HasOdds() {
			return fmt.Errorf("game has no over/under odds available")
		}
		
		if pick.TeamID != 98 && pick.TeamID != 99 {
			return fmt.Errorf("invalid team ID %d for over/under pick (must be 98=Under or 99=Over)", pick.TeamID)
		}
	}

	return nil
}

// ProcessAllCompletedGames processes results for all completed games that haven't been processed
func (s *ResultCalculationService) ProcessAllCompletedGames(ctx context.Context, season int) error {
	log.Printf("Processing all completed games for season %d", season)

	// Get all games for the season
	games, err := s.gameRepo.GetGamesBySeason(season)
	if err != nil {
		return fmt.Errorf("failed to get games: %w", err)
	}

	processedCount := 0
	errorCount := 0

	for _, game := range games {
		if !game.IsCompleted() {
			continue
		}

		// Check if picks for this game have already been processed
		picks, err := s.getPicksByGameID(ctx, game.ID)
		if err != nil {
			log.Printf("Failed to get picks for game %d: %v", game.ID, err)
			errorCount++
			continue
		}

		// Check if any picks still have pending results
		hasPendingResults := false
		for _, pick := range picks {
			if pick.Result == models.PickResultPending {
				hasPendingResults = true
				break
			}
		}

		if !hasPendingResults && len(picks) > 0 {
			// All picks already processed
			continue
		}

		// Process this game
		err = s.ProcessGameCompletion(ctx, game)
		if err != nil {
			log.Printf("Failed to process game %d: %v", game.ID, err)
			errorCount++
			continue
		}

		processedCount++
	}

	log.Printf("Processed %d completed games (%d errors)", processedCount, errorCount)
	return nil
}

// Helper method to work with WeeklyPicks documents

// getPicksByGameID extracts all picks for a specific game from WeeklyPicks documents
func (s *ResultCalculationService) getPicksByGameID(ctx context.Context, gameID int) ([]models.Pick, error) {
	// We need to find which season and week this game belongs to first
	game, err := s.gameRepo.FindByESPNID(ctx, gameID)
	if err != nil {
		return nil, fmt.Errorf("failed to get game: %w", err)
	}

	// Get all weekly picks for the game's season and week
	weeklyPicksList, err := s.weeklyPicksRepo.FindAllByWeek(ctx, game.Season, game.Week)
	if err != nil {
		return nil, fmt.Errorf("failed to get weekly picks for game: %w", err)
	}

	var picks []models.Pick
	for _, weeklyPicks := range weeklyPicksList {
		for _, pick := range weeklyPicks.Picks {
			if pick.GameID == gameID {
				// CRITICAL: Ensure UserID is set from the parent WeeklyPicks document
				pick.UserID = weeklyPicks.UserID
				picks = append(picks, pick)
			}
		}
	}

	return picks, nil
}