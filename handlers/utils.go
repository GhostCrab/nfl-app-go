package handlers

import (
	"nfl-app-go/models"
)


// filterGamesByWeek filters games to only include those from the specified week
func filterGamesByWeek(games []models.Game, week int) []models.Game {
	filtered := make([]models.Game, 0)
	for _, game := range games {
		if game.Week == week {
			filtered = append(filtered, game)
		}
	}
	return filtered
}