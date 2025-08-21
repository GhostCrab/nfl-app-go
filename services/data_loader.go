package services

import (
	"log"
	"nfl-app-go/database"
	"nfl-app-go/models"
)

type DataLoader struct {
	espnService *ESPNService
	gameRepo    *database.MongoGameRepository
}

func NewDataLoader(espnService *ESPNService, gameRepo *database.MongoGameRepository) *DataLoader {
	return &DataLoader{
		espnService: espnService,
		gameRepo:    gameRepo,
	}
}

func (dl *DataLoader) LoadGameData(season int) error {
	log.Printf("DataLoader: Starting to load game data for season %d", season)

	// Fetch games from ESPN API
	games, err := dl.espnService.GetScoreboardForYear(season)
	if err != nil {
		return err
	}

	log.Printf("DataLoader: Fetched %d games from ESPN API", len(games))

	// Fetch odds for a subset of games (limit to avoid hanging)
	gamesWithOdds := dl.espnService.EnrichGamesWithOddsLimited(games, 20)
	log.Printf("DataLoader: Enhanced %d games with odds data", len(gamesWithOdds))

	// Convert []models.Game to []*models.Game for repository
	gamePointers := make([]*models.Game, len(gamesWithOdds))
	for i := range gamesWithOdds {
		gamePointers[i] = &gamesWithOdds[i]
	}

	// Store games in database
	err = dl.gameRepo.BulkUpsertGames(gamePointers)
	if err != nil {
		return err
	}

	log.Printf("DataLoader: Successfully loaded %d games into database", len(gamePointers))
	return nil
}