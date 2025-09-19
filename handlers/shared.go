package handlers

import (
	"nfl-app-go/models"
	"sort"
)

// sortGamesByKickoffTime sorts games chronologically by kickoff time
// Secondary sort: alphabetically by home team name for games at same time
func sortGamesByKickoffTime(games []models.Game) {
	sort.Slice(games, func(i, j int) bool {
		// Primary sort: by game date/time
		if games[i].Date.Unix() != games[j].Date.Unix() {
			return games[i].Date.Before(games[j].Date)
		}
		// Secondary sort: alphabetically by home team name for same kickoff time
		return games[i].Home < games[j].Home
	})
}