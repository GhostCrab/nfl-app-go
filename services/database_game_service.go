package services

import (
	"log"
	"math/rand"
	"nfl-app-go/database"
	"nfl-app-go/models"
	"sort"
	"time"
)

type DatabaseGameService struct {
	gameRepo *database.MongoGameRepository
}

func NewDatabaseGameService(gameRepo *database.MongoGameRepository) GameService {
	return &DatabaseGameService{
		gameRepo: gameRepo,
	}
}

func (s *DatabaseGameService) GetGames() ([]models.Game, error) {
	log.Printf("DatabaseGameService: Fetching games from database")
	
	gamePointers, err := s.gameRepo.GetAllGames()
	if err != nil {
		return nil, err
	}

	// Convert []*models.Game to []models.Game
	games := make([]models.Game, len(gamePointers))
	for i, gamePtr := range gamePointers {
		games[i] = *gamePtr
	}

	// Debug: Log first few games before sorting
	if len(games) > 0 {
		log.Printf("Before sorting - First game: %s vs %s, State: %s, Date: %s", 
			games[0].Away, games[0].Home, games[0].State, games[0].Date.Format("2006-01-02 15:04:05"))
	}

	// Sort games: in-progress first, then upcoming, then completed, all by time
	sort.Slice(games, func(i, j int) bool {
		gameI, gameJ := games[i], games[j]
		
		// Priority order: in_play > scheduled > completed
		statusOrder := map[models.GameState]int{
			models.GameStateInPlay:    1,
			models.GameStateScheduled: 2,
			models.GameStateCompleted: 3,
			models.GameStatePostponed: 4,
		}
		
		orderI, orderJ := statusOrder[gameI.State], statusOrder[gameJ.State]
		
		if orderI != orderJ {
			return orderI < orderJ
		}
		
		// Within same status, sort by date/time
		return gameI.Date.Before(gameJ.Date)
	})

	// Debug: Log first few games after sorting
	if len(games) > 0 {
		log.Printf("After sorting - First game: %s vs %s, State: %s, Date: %s", 
			games[0].Away, games[0].Home, games[0].State, games[0].Date.Format("2006-01-02 15:04:05"))
	}

	log.Printf("DatabaseGameService: Retrieved %d games from database", len(games))
	
	// Apply demo modifications to first 3 games for live simulation
	demoGames := s.applyDemoEffects(games)
	
	return demoGames, nil
}

func (s *DatabaseGameService) GetGamesByWeek(week, season int) ([]*models.Game, error) {
	log.Printf("DatabaseGameService: Fetching games from database for week %d, season %d", week, season)
	
	games, err := s.gameRepo.GetGamesByWeekSeason(week, season)
	if err != nil {
		return nil, err
	}

	log.Printf("DatabaseGameService: Retrieved %d games from database for week %d", len(games), week)
	return games, nil
}

func (s *DatabaseGameService) HealthCheck() bool {
	// Test database connectivity
	_, err := s.gameRepo.GetAllGames()
	return err == nil
}

// applyDemoEffects modifies games for live demo simulation (does not affect database)
func (s *DatabaseGameService) applyDemoEffects(games []models.Game) []models.Game {
	if len(games) == 0 {
		return games
	}

	// Create a copy to avoid modifying the original slice
	demoGames := make([]models.Game, len(games))
	copy(demoGames, games)

	// Make first 3 games appear live for demo
	count := 0
	for i := range demoGames {
		if count >= 3 {
			break
		}
		
		// Only modify completed or scheduled games to avoid overwriting real live games
		if demoGames[i].State == models.GameStateCompleted || demoGames[i].State == models.GameStateScheduled {
			demoGames[i] = s.makeGameLive(demoGames[i])
			count++
		}
	}

	return demoGames
}

// makeGameLive converts a game to appear as if it's currently live
func (s *DatabaseGameService) makeGameLive(game models.Game) models.Game {
	// Seed random with game ID for consistent results that change every 30 seconds
	rand.Seed(int64(game.ID) + time.Now().Unix()/30)

	// Make it look live
	game.State = models.GameStateInPlay
	game.Quarter = rand.Intn(4) + 1

	// Add some realistic scores
	game.HomeScore = rand.Intn(35)
	game.AwayScore = rand.Intn(35)

	// Occasionally make it close (30% chance)
	if rand.Float32() < 0.3 {
		diff := rand.Intn(7) // 0-6 point difference
		if game.HomeScore > game.AwayScore {
			game.AwayScore = game.HomeScore - diff
		} else {
			game.HomeScore = game.AwayScore - diff
		}
	}

	return game
}