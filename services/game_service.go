package services

import (
	"fmt"
	"log"
	"math/rand"
	"nfl-app-go/models"
	"time"
)

// GameService interface defines methods for getting game data
type GameService interface {
	GetGames() ([]models.Game, error)
	GetGamesBySeason(season int) ([]models.Game, error)
	GetGameByID(gameID int) (*models.Game, error)
	HealthCheck() bool
}

// DemoGameService wraps ESPN data and manipulates it for live demos
type DemoGameService struct {
	espnService *ESPNService
}

// NewDemoGameService creates a new demo game service
func NewDemoGameService() *DemoGameService {
	return &DemoGameService{
		espnService: NewESPNService(),
	}
}

// GetGames returns games for current season (2025) modified for demo purposes
func (d *DemoGameService) GetGames() ([]models.Game, error) {
	return d.GetGamesBySeason(2025)
}

// GetGamesBySeason returns games for specified season modified for demo purposes
func (d *DemoGameService) GetGamesBySeason(season int) ([]models.Game, error) {
	log.Printf("DemoGameService: Fetching games for %d", season)
	games, err := d.espnService.GetScoreboardForYear(season)
	if err != nil {
		log.Printf("DemoGameService: ESPN API failed for %d, using sample data: %v", season, err)
		return d.getSampleGames(season), nil
	}

	log.Printf("DemoGameService: Got %d games from ESPN for %d", len(games), season)
	
	// Try to enrich a limited number of games with odds to avoid hanging
	enrichedGames := d.espnService.EnrichGamesWithOddsLimited(games, 20) // Only try first 20 games
	
	demoGames := d.makeDemoGames(enrichedGames)
	log.Printf("DemoGameService: Returning %d demo games for %d", len(demoGames), season)
	return demoGames, nil
}

// GetGameByID returns a specific game by ID for the current season (2025)
func (d *DemoGameService) GetGameByID(gameID int) (*models.Game, error) {
	games, err := d.GetGames()
	if err != nil {
		return nil, err
	}
	
	for _, game := range games {
		if game.ID == gameID {
			return &game, nil
		}
	}
	
	return nil, fmt.Errorf("game with ID %d not found", gameID)
}

// HealthCheck verifies the underlying service is accessible
func (d *DemoGameService) HealthCheck() bool {
	return d.espnService.HealthCheck()
}

// makeDemoGames modifies real games to create live demo scenarios
func (d *DemoGameService) makeDemoGames(games []models.Game) []models.Game {
	if len(games) == 0 {
		return d.getSampleGames(2025)
	}

	demoGames := make([]models.Game, len(games))
	copy(demoGames, games)

	// Modify some games to be "live" for demo
	for i := range demoGames {
		if i < 3 { // Make first 3 games interesting for demo
			demoGames[i] = d.makeGameLive(demoGames[i])
		}
	}

	return demoGames
}

// makeGameLive converts a game to appear as if it's currently live
func (d *DemoGameService) makeGameLive(game models.Game) models.Game {
	// Seed random with game ID for consistent results
	rand.Seed(int64(game.ID) + time.Now().Unix()/30) // Changes every 30 seconds

	// Make it look live
	game.State = models.GameStateInPlay
	game.Quarter = rand.Intn(4) + 1

	// Add some realistic scores
	game.HomeScore = rand.Intn(35)
	game.AwayScore = rand.Intn(35)

	// Occasionally make it close
	if rand.Float32() < 0.3 { // 30% chance
		diff := rand.Intn(7) // 0-6 point difference
		if game.HomeScore > game.AwayScore {
			game.AwayScore = game.HomeScore - diff
		} else {
			game.HomeScore = game.AwayScore - diff
		}
	}

	return game
}

// getSampleGames returns fallback sample data when ESPN is unavailable
func (d *DemoGameService) getSampleGames(season int) []models.Game {
	now := time.Now()
	
	return []models.Game{
		{
			ID:        1,
			Season:    season,
			Date:      now.AddDate(0, 0, -1),
			Week:      1,
			Away:      "KC",
			Home:      "DET",
			State:     models.GameStateCompleted,
			AwayScore: 21,
			HomeScore: 20,
			Quarter:   4,
		},
		{
			ID:        2,
			Season:    season,
			Date:      now,
			Week:      1,
			Away:      "GB",
			Home:      "CHI",
			State:     models.GameStateInPlay,
			AwayScore: 14,
			HomeScore: 7,
			Quarter:   3,
		},
		{
			ID:        3,
			Season:    season,
			Date:      now.AddDate(0, 0, 1),
			Week:      1,
			Away:      "DAL",
			Home:      "NYG",
			State:     models.GameStateScheduled,
			AwayScore: 0,
			HomeScore: 0,
			Quarter:   0,
		},
	}
}