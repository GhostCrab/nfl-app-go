package services

import (
	"fmt"
	"strings"
)

// TeamData holds team information including logo URLs
type TeamData struct {
	Name string
	City string
}

// GetTeamIconURL returns the ESPN logo URL for a given team abbreviation
func GetTeamIconURL(teamAbbr string) string {
	if teamAbbr == "" {
		return ""
	}
	// Convert to lowercase for ESPN API
	teamLower := strings.ToLower(teamAbbr)
	return fmt.Sprintf("https://a.espncdn.com/combiner/i?img=/i/teamlogos/nfl/500/scoreboard/%s.png", teamLower)
}

// GetTeamData returns team information for all NFL teams
func GetTeamData() map[string]TeamData {
	return map[string]TeamData{
		// AFC East
		"BUF": {Name: "Bills", City: "Buffalo"},
		"MIA": {Name: "Dolphins", City: "Miami"},
		"NE":  {Name: "Patriots", City: "New England"},
		"NYJ": {Name: "Jets", City: "New York"},
		
		// AFC North
		"BAL": {Name: "Ravens", City: "Baltimore"},
		"CIN": {Name: "Bengals", City: "Cincinnati"},
		"CLE": {Name: "Browns", City: "Cleveland"},
		"PIT": {Name: "Steelers", City: "Pittsburgh"},
		
		// AFC South
		"HOU": {Name: "Texans", City: "Houston"},
		"IND": {Name: "Colts", City: "Indianapolis"},
		"JAX": {Name: "Jaguars", City: "Jacksonville"},
		"TEN": {Name: "Titans", City: "Tennessee"},
		
		// AFC West
		"DEN": {Name: "Broncos", City: "Denver"},
		"KC":  {Name: "Chiefs", City: "Kansas City"},
		"LV":  {Name: "Raiders", City: "Las Vegas"},
		"LAC": {Name: "Chargers", City: "Los Angeles"},
		
		// NFC East
		"DAL": {Name: "Cowboys", City: "Dallas"},
		"NYG": {Name: "Giants", City: "New York"},
		"PHI": {Name: "Eagles", City: "Philadelphia"},
		"WAS": {Name: "Commanders", City: "Washington"},
		"WSH": {Name: "Commanders", City: "Washington"}, // Alternative abbreviation
		
		// NFC North
		"CHI": {Name: "Bears", City: "Chicago"},
		"DET": {Name: "Lions", City: "Detroit"},
		"GB":  {Name: "Packers", City: "Green Bay"},
		"MIN": {Name: "Vikings", City: "Minnesota"},
		
		// NFC South
		"ATL": {Name: "Falcons", City: "Atlanta"},
		"CAR": {Name: "Panthers", City: "Carolina"},
		"NO":  {Name: "Saints", City: "New Orleans"},
		"TB":  {Name: "Buccaneers", City: "Tampa Bay"},
		
		// NFC West
		"ARI": {Name: "Cardinals", City: "Arizona"},
		"LAR": {Name: "Rams", City: "Los Angeles"},
		"SF":  {Name: "49ers", City: "San Francisco"},
		"SEA": {Name: "Seahawks", City: "Seattle"},
	}
}

// GetTeamName returns the full team name for a given team abbreviation
func GetTeamName(teamAbbr string) string {
	teams := GetTeamData()
	if team, exists := teams[teamAbbr]; exists {
		return team.City + " " + team.Name
	}
	return teamAbbr // Return abbreviation if team not found
}