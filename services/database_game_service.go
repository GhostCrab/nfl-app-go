package services

import (
	"fmt"
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
	return s.GetGamesBySeason(2025)
}

func (s *DatabaseGameService) GetGamesBySeason(season int) ([]models.Game, error) {
	
	gamePointers, err := s.gameRepo.GetGamesBySeason(season)
	if err != nil {
		return nil, err
	}

	// Convert []*models.Game to []models.Game
	games := make([]models.Game, len(gamePointers))
	for i, gamePtr := range gamePointers {
		games[i] = *gamePtr
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

	
	// Apply demo modifications to first 3 games for live simulation
	// demoGames := s.applyDemoEffects(games) // DISABLED for analytics
	
	return games, nil
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

	// Make all Week 1 games appear live for demo
	for i := range demoGames {
		// Only modify Week 1 games that are completed or scheduled
		if demoGames[i].Week == 1 && (demoGames[i].State == models.GameStateCompleted || demoGames[i].State == models.GameStateScheduled) {
			demoGames[i] = s.makeGameLive(demoGames[i])
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

	// Add realistic live game status data
	s.addLiveStatusData(&game)

	return game
}

// addLiveStatusData adds realistic possession and field position data to a live game
func (s *DatabaseGameService) addLiveStatusData(game *models.Game) {
	// Use the same seed as makeGameLive for consistency
	rand.Seed(int64(game.ID) + time.Now().Unix()/30)

	// Generate realistic game clock
	var displayClock string
	if game.Quarter == 6 {
		displayClock = "Halftime"
	} else {
		minutes := rand.Intn(15) // 0-14 minutes
		seconds := rand.Intn(60) // 0-59 seconds
		displayClock = fmt.Sprintf("%d:%02d", minutes, seconds)
	}

	// Pick which team has possession (favor the team that's behind slightly)
	var possessionTeam string
	if game.HomeScore < game.AwayScore && rand.Float32() < 0.6 {
		possessionTeam = game.Home
	} else if game.AwayScore < game.HomeScore && rand.Float32() < 0.6 {
		possessionTeam = game.Away
	} else {
		// Random possession
		if rand.Float32() < 0.5 {
			possessionTeam = game.Home
		} else {
			possessionTeam = game.Away
		}
	}

	// Generate field position (1-99 yard line)
	yardLine := rand.Intn(99) + 1
	
	// Generate down and distance
	down := rand.Intn(4) + 1 // 1st, 2nd, 3rd, or 4th down
	distance := rand.Intn(20) + 1 // 1-20 yards to go
	
	// Make realistic adjustments
	if down == 1 {
		distance = 10 // 1st and 10 is most common
	} else if down == 4 {
		// 4th down usually has shorter distance
		if distance > 10 {
			distance = rand.Intn(5) + 1 // 1-5 yards on 4th down
		}
	}

	// Generate possession text (field position)
	var possessionText string
	var oppTeam string
	if possessionTeam == game.Home {
		oppTeam = game.Away
	} else {
		oppTeam = game.Home
	}

	if yardLine <= 50 {
		// Own territory
		possessionText = fmt.Sprintf("%s %d", possessionTeam, yardLine)
	} else {
		// Opponent's territory
		oppYardLine := 100 - yardLine
		possessionText = fmt.Sprintf("%s %d", oppTeam, oppYardLine)
	}

	// Generate down/distance text
	downDistanceText := fmt.Sprintf("%s down and %d", s.getOrdinalNumber(down), distance)
	shortDownDistanceText := fmt.Sprintf("%s & %d", s.getShortOrdinal(down), distance)

	// Determine if in red zone (opponent's 20 yard line or closer)
	isRedZone := yardLine > 80

	// Generate timeouts (start with 3, sometimes use them)
	homeTimeouts := 3 - rand.Intn(2) // 2-3 timeouts
	awayTimeouts := 3 - rand.Intn(2) // 2-3 timeouts

	// Set the status data
	game.SetStatus(
		displayClock,
		possessionTeam,
		possessionText,
		downDistanceText,
		shortDownDistanceText,
		down,
		yardLine,
		distance,
		homeTimeouts,
		awayTimeouts,
		isRedZone,
	)
}

// Helper functions for formatting
func (s *DatabaseGameService) getOrdinalNumber(n int) string {
	switch n {
	case 1:
		return "1st"
	case 2:
		return "2nd"
	case 3:
		return "3rd"
	case 4:
		return "4th"
	default:
		return fmt.Sprintf("%dth", n)
	}
}

func (s *DatabaseGameService) getShortOrdinal(n int) string {
	switch n {
	case 1:
		return "1st"
	case 2:
		return "2nd" 
	case 3:
		return "3rd"
	case 4:
		return "4th"
	default:
		return fmt.Sprintf("%d", n)
	}
}