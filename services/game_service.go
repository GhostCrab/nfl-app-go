package services

import (
	"log"
	"math/rand"
	"nfl-app-go/models"
	"time"
)

// GameService interface defines methods for getting game data
type GameService interface {
	GetGames() ([]models.Game, error)
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

// GetGames returns games modified for demo purposes
func (d *DemoGameService) GetGames() ([]models.Game, error) {
	log.Println("DemoGameService: Fetching games for 2024")
	games, err := d.espnService.GetScoreboardForYear(2024)
	if err != nil {
		log.Printf("DemoGameService: ESPN API failed, using sample data: %v", err)
		return d.getSampleGames(), nil
	}

	log.Printf("DemoGameService: Got %d games from ESPN", len(games))
	
	// Try to enrich a limited number of games with odds to avoid hanging
	enrichedGames := d.espnService.EnrichGamesWithOddsLimited(games, 20) // Only try first 20 games
	
	demoGames := d.makeDemoGames(enrichedGames)
	log.Printf("DemoGameService: Returning %d demo games", len(demoGames))
	return demoGames, nil
}

// HealthCheck verifies the underlying service is accessible
func (d *DemoGameService) HealthCheck() bool {
	return d.espnService.HealthCheck()
}

// makeDemoGames modifies real games to create live demo scenarios
func (d *DemoGameService) makeDemoGames(games []models.Game) []models.Game {
	if len(games) == 0 {
		return d.getSampleGames()
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
func (d *DemoGameService) getSampleGames() []models.Game {
	now := time.Now()
	
	return []models.Game{
		{
			ID:        1,
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