package services

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"nfl-app-go/database"
	"nfl-app-go/models"
	"path/filepath"
	"time"
)

// LegacyGameData represents the structure from legacy JSON files
type LegacyGameData struct {
	ID        int     `json:"id"`
	Date      string  `json:"date"`
	Week      int     `json:"week"`
	Away      string  `json:"away"`
	Home      string  `json:"home"`
	State     int     `json:"state"`
	AwayScore int     `json:"awayScore"`
	HomeScore int     `json:"homeScore"`
	Quarter   int     `json:"quarter"`
	Odds      *LegacyOdds `json:"odds,omitempty"`
}

// LegacyOdds represents odds structure from legacy data
type LegacyOdds struct {
	Spread float64 `json:"spread"`
	OU     float64 `json:"ou"`
}

// LegacyPickData represents pick structure from legacy JSON files
type LegacyPickData struct {
	User int `json:"user"`
	Game int `json:"game"`
	Team int `json:"team"`
}

// LegacyImportService handles importing legacy data
type LegacyImportService struct {
	gameRepo *database.MongoGameRepository
	userRepo *database.MongoUserRepository
	pickRepo *database.MongoPickRepository
}

// NewLegacyImportService creates a new legacy import service
func NewLegacyImportService(gameRepo *database.MongoGameRepository, userRepo *database.MongoUserRepository, pickRepo *database.MongoPickRepository) *LegacyImportService {
	return &LegacyImportService{
		gameRepo: gameRepo,
		userRepo: userRepo,
		pickRepo: pickRepo,
	}
}

// ImportAllLegacyData imports both games and picks from legacy-dbs directory
func (s *LegacyImportService) ImportAllLegacyData(legacyDbsPath string) error {
	log.Println("LegacyImport: Starting legacy data import...")
	
	// Import games for each season
	seasons := []int{2023, 2024, 2025}
	
	for _, season := range seasons {
		log.Printf("LegacyImport: Importing %d season data...", season)
		
		// Import games
		gamesFile := filepath.Join(legacyDbsPath, fmt.Sprintf("%d_games", season))
		if err := s.ImportGames(gamesFile, season); err != nil {
			log.Printf("LegacyImport: Error importing %d games: %v", season, err)
			return err
		}
		
		// Import picks
		picksFile := filepath.Join(legacyDbsPath, fmt.Sprintf("%d_picks", season))
		if err := s.ImportPicks(picksFile, season); err != nil {
			log.Printf("LegacyImport: Error importing %d picks: %v", season, err)
			return err
		}
	}
	
	log.Println("LegacyImport: Successfully imported all legacy data!")
	return nil
}

// ImportGames imports games from a legacy games JSON file
func (s *LegacyImportService) ImportGames(filePath string, season int) error {
	log.Printf("LegacyImport: Reading games file: %s", filePath)
	
	// Read the JSON file
	data, err := ioutil.ReadFile(filePath)
	if err != nil {
		return fmt.Errorf("failed to read games file %s: %w", filePath, err)
	}
	
	// Parse legacy games data
	var legacyGames []LegacyGameData
	if err := json.Unmarshal(data, &legacyGames); err != nil {
		return fmt.Errorf("failed to parse games JSON: %w", err)
	}
	
	log.Printf("LegacyImport: Found %d games in %s", len(legacyGames), filePath)
	
	// Convert to our Game model
	games := make([]*models.Game, 0, len(legacyGames))
	for _, legacyGame := range legacyGames {
		game, err := s.convertLegacyGame(legacyGame, season)
		if err != nil {
			log.Printf("LegacyImport: Warning - skipping game %d: %v", legacyGame.ID, err)
			continue
		}
		games = append(games, game)
	}
	
	log.Printf("LegacyImport: Converted %d games for %d season", len(games), season)
	
	// Bulk insert games
	if err := s.gameRepo.BulkUpsertGames(games); err != nil {
		return fmt.Errorf("failed to insert games: %w", err)
	}
	
	log.Printf("LegacyImport: Successfully imported %d games for %d season", len(games), season)
	return nil
}

// convertLegacyGame converts a legacy game to our Game model
func (s *LegacyImportService) convertLegacyGame(legacy LegacyGameData, season int) (*models.Game, error) {
	// Parse the date - legacy format is "2023-09-08T00:20Z"
	gameDate, err := time.Parse("2006-01-02T15:04Z", legacy.Date)
	if err != nil {
		return nil, fmt.Errorf("failed to parse date '%s': %w", legacy.Date, err)
	}
	
	// Convert legacy state to our GameState
	var gameState models.GameState
	switch legacy.State {
	case 1:
		gameState = models.GameStateScheduled
	case 2:
		gameState = models.GameStateInPlay
	case 4:
		gameState = models.GameStateCompleted
	default:
		gameState = models.GameStateScheduled
	}
	
	// Create the game
	game := &models.Game{
		ID:        legacy.ID,
		Season:    season,
		Date:      gameDate,
		Week:      legacy.Week,
		Away:      legacy.Away,
		Home:      legacy.Home,
		State:     gameState,
		AwayScore: legacy.AwayScore,
		HomeScore: legacy.HomeScore,
		Quarter:   legacy.Quarter,
	}
	
	// Add odds if available
	if legacy.Odds != nil {
		game.Odds = &models.Odds{
			Spread: legacy.Odds.Spread,
			OU:     legacy.Odds.OU,
		}
	}
	
	return game, nil
}

// ImportPicks imports picks from a legacy picks JSON file and stores them with season tracking
func (s *LegacyImportService) ImportPicks(filePath string, season int) error {
	ctx := context.Background()
	log.Printf("LegacyImport: Reading picks file: %s", filePath)
	
	// Read the JSON file
	data, err := ioutil.ReadFile(filePath)
	if err != nil {
		return fmt.Errorf("failed to read picks file %s: %w", filePath, err)
	}
	
	// Parse legacy picks data
	var legacyPicks []LegacyPickData
	if err := json.Unmarshal(data, &legacyPicks); err != nil {
		return fmt.Errorf("failed to parse picks JSON: %w", err)
	}
	
	log.Printf("LegacyImport: Found %d picks in %s", len(legacyPicks), filePath)
	
	// Check if picks for this season already exist using simple count method
	// We'll create a helper method to count by season
	existingPicks, err := s.pickRepo.FindBySeason(ctx, season)
	if err != nil {
		log.Printf("Warning: failed to check existing picks: %v", err)
	} else if len(existingPicks) > 0 {
		log.Printf("LegacyImport: Found %d existing picks for season %d. Skipping import to avoid duplicates.", len(existingPicks), season)
		return nil
	}
	
	// Convert legacy picks to Pick models with season tracking
	var picks []*models.Pick
	gameWeekMap := make(map[int]int) // Cache game ID to week mapping
	
	userPickCount := make(map[int]int)
	gamePickCount := make(map[int]int)
	pickTypeCount := make(map[models.PickType]int)
	
	for _, legacyPick := range legacyPicks {
		// Get week number from game data (we need to look up the game)
		week, exists := gameWeekMap[legacyPick.Game]
		if !exists {
			// Look up game to get week
			game, err := s.gameRepo.FindByESPNID(ctx, legacyPick.Game)
			if err != nil {
				log.Printf("Warning: failed to find game %d: %v", legacyPick.Game, err)
				continue
			}
			if game == nil {
				log.Printf("Warning: game %d not found, skipping pick", legacyPick.Game)
				continue
			}
			week = game.Week
			gameWeekMap[legacyPick.Game] = week
		}
		
		// Create Pick using the model's helper function (includes season tracking)
		pick := models.CreatePickFromLegacyData(
			legacyPick.User,
			legacyPick.Game,
			legacyPick.Team,
			season, // NEW: Season tracking
			week,
		)
		
		picks = append(picks, pick)
		
		// Update statistics
		userPickCount[legacyPick.User]++
		gamePickCount[legacyPick.Game]++
		pickTypeCount[pick.PickType]++
	}
	
	if len(picks) == 0 {
		log.Printf("LegacyImport: No valid picks to import for season %d", season)
		return nil
	}
	
	// Store picks in database using batch insert
	log.Printf("LegacyImport: Storing %d picks in database for season %d...", len(picks), season)
	if err := s.pickRepo.CreateMany(ctx, picks); err != nil {
		return fmt.Errorf("failed to store picks: %w", err)
	}
	
	// Log import statistics
	log.Printf("LegacyImport: Successfully imported %d picks for %d season", len(picks), season)
	log.Printf("  - Users with picks: %d", len(userPickCount))
	log.Printf("  - Games with picks: %d", len(gamePickCount))
	log.Printf("  - Spread picks: %d", pickTypeCount[models.PickTypeSpread])
	log.Printf("  - Over/Under picks: %d", pickTypeCount[models.PickTypeOverUnder])
	
	// Log user pick counts
	for userID, count := range userPickCount {
		log.Printf("  - User %d: %d picks", userID, count)
	}
	
	return nil
}

// GetImportSummary returns a summary of what would be imported
func (s *LegacyImportService) GetImportSummary(legacyDbsPath string) error {
	log.Println("LegacyImport: Analyzing legacy data files...")
	
	seasons := []int{2023, 2024, 2025}
	totalGames := 0
	totalPicks := 0
	
	for _, season := range seasons {
		// Analyze games file
		gamesFile := filepath.Join(legacyDbsPath, fmt.Sprintf("%d_games", season))
		gamesData, err := ioutil.ReadFile(gamesFile)
		if err != nil {
			log.Printf("LegacyImport: Cannot read %s: %v", gamesFile, err)
			continue
		}
		
		var legacyGames []LegacyGameData
		if err := json.Unmarshal(gamesData, &legacyGames); err != nil {
			log.Printf("LegacyImport: Cannot parse %s: %v", gamesFile, err)
			continue
		}
		
		// Analyze picks file
		picksFile := filepath.Join(legacyDbsPath, fmt.Sprintf("%d_picks", season))
		picksData, err := ioutil.ReadFile(picksFile)
		if err != nil {
			log.Printf("LegacyImport: Cannot read %s: %v", picksFile, err)
			continue
		}
		
		var legacyPicks []LegacyPickData
		if err := json.Unmarshal(picksData, &legacyPicks); err != nil {
			log.Printf("LegacyImport: Cannot parse %s: %v", picksFile, err)
			continue
		}
		
		log.Printf("LegacyImport: %d Season Summary:", season)
		log.Printf("  - Games: %d", len(legacyGames))
		log.Printf("  - Picks: %d", len(legacyPicks))
		
		// Analyze date range
		if len(legacyGames) > 0 {
			log.Printf("  - First game: %s", legacyGames[0].Date)
			log.Printf("  - Last game: %s", legacyGames[len(legacyGames)-1].Date)
		}
		
		totalGames += len(legacyGames)
		totalPicks += len(legacyPicks)
	}
	
	log.Printf("LegacyImport: Total Summary:")
	log.Printf("  - Total games across all seasons: %d", totalGames)
	log.Printf("  - Total picks across all seasons: %d", totalPicks)
	
	return nil
}