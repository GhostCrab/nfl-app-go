package services

import (
	"fmt"
	"math/rand"
	"nfl-app-go/database"
	"nfl-app-go/logging"
	"nfl-app-go/models"
	"time"
)

// MockBackgroundUpdater generates fake game updates for testing SSE live updates
type MockBackgroundUpdater struct {
	gameRepo      *database.MongoGameRepository
	currentSeason int
	ticker        *time.Ticker
	stopChan      chan bool
	running       bool
	logger        *logging.Logger
	updateCount   int // Track number of updates for completion cycle
}

// NewMockBackgroundUpdater creates a new mock background updater
func NewMockBackgroundUpdater(gameRepo *database.MongoGameRepository, currentSeason int) *MockBackgroundUpdater {
	return &MockBackgroundUpdater{
		gameRepo:      gameRepo,
		currentSeason: currentSeason,
		stopChan:      make(chan bool),
		running:       false,
		logger:        logging.WithPrefix("MockUpdater"),
	}
}

// Start begins the mock updating process every 20 seconds
func (mu *MockBackgroundUpdater) Start() {
	if mu.running {
		mu.logger.Info("Already running")
		return
	}

	mu.logger.Info("Starting mock game updates every 10 seconds")
	mu.running = true

	// Start the ticker for 10-second intervals
	mu.ticker = time.NewTicker(10 * time.Second)

	go func() {
		for {
			select {
			case <-mu.ticker.C:
				mu.generateFakeUpdates()
			case <-mu.stopChan:
				mu.logger.Info("Stopping mock updater")
				return
			}
		}
	}()
}

// Stop halts the mock updating process
func (mu *MockBackgroundUpdater) Stop() {
	if !mu.running {
		return
	}

	mu.logger.Info("Stopping mock updater...")
	mu.running = false

	if mu.ticker != nil {
		mu.ticker.Stop()
	}

	close(mu.stopChan)
}

// generateFakeUpdates creates fake game data updates for the first 3 games of current week
func (mu *MockBackgroundUpdater) generateFakeUpdates() {
	mu.updateCount++
	isCompletionCycle := mu.updateCount%4 == 0

	// Get current week games
	currentWeek := mu.getCurrentWeek()
	games, err := mu.gameRepo.GetGamesByWeekSeason(currentWeek, mu.currentSeason)
	if err != nil {
		mu.logger.Errorf("Failed to get games for week %d: %v", currentWeek, err)
		return
	}

	if len(games) == 0 {
		mu.logger.Warnf("No games found for week %d", currentWeek)
		return
	}

	// Take first 3 games (or fewer if less than 3 exist)
	gamesToUpdate := games
	if len(games) > 3 {
		gamesToUpdate = games[:3]
	}

	cycleType := "in-progress"
	if isCompletionCycle {
		cycleType = "completion"
	}
	mu.logger.Infof("Generating %s updates (#%d) for %d games in week %d", cycleType, mu.updateCount, len(gamesToUpdate), currentWeek)

	// Generate fake updates for each game
	var updatedGames []*models.Game
	for _, game := range gamesToUpdate {
		var fakeGame *models.Game
		if isCompletionCycle {
			fakeGame = mu.generateCompletedGameData(game)
		} else {
			fakeGame = mu.generateInProgressGameData(game)
		}
		updatedGames = append(updatedGames, fakeGame)

		clock := ""
		possession := ""
		if fakeGame.HasStatus() {
			clock = fakeGame.Status.DisplayClock
			possession = fakeGame.Status.Possession
		}

		statusInfo := fmt.Sprintf("%s, %d:%d, Q%d", fakeGame.State, fakeGame.AwayScore, fakeGame.HomeScore, fakeGame.Quarter)
		if fakeGame.State == models.GameStateCompleted {
			totalPoints := fakeGame.HomeScore + fakeGame.AwayScore
			mu.logger.Infof("Mock update: Game %d (%s vs %s) - FINAL: %s wins %d-%d (Total: %d, Over hit)",
				fakeGame.ID, fakeGame.Away, fakeGame.Home, fakeGame.Home,
				fakeGame.HomeScore, fakeGame.AwayScore, totalPoints)
		} else {
			mu.logger.Infof("Mock update: Game %d (%s vs %s) - %s %s %s",
				fakeGame.ID, fakeGame.Away, fakeGame.Home, statusInfo, clock, possession)
		}
	}

	// Update games in database (this will trigger MongoDB change streams and SSE updates)
	err = mu.gameRepo.BulkUpsertGames(updatedGames)
	if err != nil {
		mu.logger.Errorf("Failed to update mock games: %v", err)
		return
	}

	mu.logger.Info("Mock game updates completed successfully")
}

// generateInProgressGameData creates realistic fake in-progress game data
func (mu *MockBackgroundUpdater) generateInProgressGameData(original *models.Game) *models.Game {
	// Create a copy of the original game
	game := *original

	// Set game to in progress
	game.State = models.GameStateInPlay

	// Generate random scores (0-35 range for realism)
	game.AwayScore = rand.Intn(36)
	game.HomeScore = rand.Intn(36)

	// Generate random quarter (1-4)
	game.Quarter = rand.Intn(4) + 1

	// Generate realistic game clock
	clock := mu.generateRandomClock()

	// Generate random possession (50/50 chance for either team)
	var possession string
	if rand.Intn(2) == 0 {
		possession = game.Away
	} else {
		possession = game.Home
	}

	// Set game status with live information
	game.SetStatus(
		clock,                    // displayClock
		"STATUS_IN_PROGRESS",     // statusName
		possession,               // possession
		possession + " 35",       // possessionText (fake field position)
		"1st & 10",              // downDistanceText
		"1st & 10",              // shortDownDistanceText
		1,                        // down
		35,                       // yardLine
		10,                       // distance
		3,                        // homeTimeouts
		3,                        // awayTimeouts
		false,                    // isRedZone
	)

	return &game
}

// generateCompletedGameData creates completed game with home team win and over hit
func (mu *MockBackgroundUpdater) generateCompletedGameData(original *models.Game) *models.Game {
	// Create a copy of the original game
	game := *original

	// Set game to completed
	game.State = models.GameStateCompleted

	// Home team wins with realistic scores that go over
	// Generate scores between 24-35 for home (winner) and 17-28 for away
	game.HomeScore = 24 + rand.Intn(12) // 24-35
	game.AwayScore = 17 + rand.Intn(12) // 17-28

	// Ensure home team wins
	if game.HomeScore <= game.AwayScore {
		game.HomeScore = game.AwayScore + 1 + rand.Intn(7) // Win by 1-7 points
	}

	// Ensure total goes over (assuming typical O/U around 45-50)
	totalPoints := game.HomeScore + game.AwayScore
	if totalPoints < 48 {
		// Boost scores to ensure over hits
		boost := (48 - totalPoints + rand.Intn(10)) / 2
		game.HomeScore += boost
		game.AwayScore += boost
		// Re-ensure home team still wins
		if game.HomeScore <= game.AwayScore {
			game.HomeScore = game.AwayScore + 1
		}
	}

	// Set final quarter
	game.Quarter = 4

	// Set final game status
	game.SetStatus(
		"0:00",                   // displayClock
		"STATUS_FINAL",           // statusName
		"",                       // possession (none in final)
		"",                       // possessionText
		"",                       // downDistanceText
		"",                       // shortDownDistanceText
		0,                        // down
		0,                        // yardLine
		0,                        // distance
		0,                        // homeTimeouts
		0,                        // awayTimeouts
		false,                    // isRedZone
	)

	return &game
}

// generateRandomClock creates realistic game clock times
func (mu *MockBackgroundUpdater) generateRandomClock() string {
	// Generate minutes (0-15 for each quarter)
	minutes := rand.Intn(16)

	// Generate seconds (0-59)
	seconds := rand.Intn(60)

	return fmt.Sprintf("%d:%02d", minutes, seconds)
}

// getCurrentWeek determines the current NFL week using the proper GetNFLWeekForDate function
func (mu *MockBackgroundUpdater) getCurrentWeek() int {
	return models.GetNFLWeekForDate(time.Now(), mu.currentSeason)
}

// IsRunning returns whether the mock updater is currently running
func (mu *MockBackgroundUpdater) IsRunning() bool {
	return mu.running
}