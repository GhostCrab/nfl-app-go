package services

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"nfl-app-go/models"
	"strconv"
	"strings"
	"time"
)

// ESPNService handles ESPN API interactions
type ESPNService struct {
	client *http.Client
	baseURL string
}

// NewESPNService creates a new ESPN service
func NewESPNService() *ESPNService {
	return &ESPNService{
		client: &http.Client{Timeout: 10 * time.Second},
		baseURL: "https://site.api.espn.com/apis/site/v2/sports/football/nfl/scoreboard",
	}
}

// ESPN API response structures
type ESPNResponse struct {
	Events []ESPNEvent `json:"events"`
}

type ESPNEvent struct {
	ID           string         `json:"id"`
	Date         string         `json:"date"`
	Week         ESPNWeek       `json:"week"`
	Season       ESPNSeason     `json:"season"`
	Status       ESPNStatus     `json:"status"`
	Competitions []ESPNCompetition `json:"competitions"`
}

type ESPNSeason struct {
	Year int `json:"year"`
	Type int `json:"type"`
}

type ESPNWeek struct {
	Number int `json:"number"`
}

type ESPNStatus struct {
	Type         ESPNStatusType `json:"type"`
	Period       int            `json:"period"`
	DisplayClock string         `json:"displayClock,omitempty"`
	Clock        float64        `json:"clock,omitempty"` // Changed to float64 to handle ESPN's 0.0 values
}

type ESPNStatusType struct {
	Name        string `json:"name"`
	State       string `json:"state"`
	Completed   bool   `json:"completed"`
	Description string `json:"description"`
}

type ESPNCompetition struct {
	Competitors []ESPNCompetitor `json:"competitors"`
	Situation   *ESPNSituation   `json:"situation,omitempty"`
}

type ESPNCompetitor struct {
	ID         string    `json:"id"`
	HomeAway   string    `json:"homeAway"`
	Score      string    `json:"score"`
	Team       ESPNTeam  `json:"team"`
}

type ESPNTeam struct {
	ID           string `json:"id"`
	Abbreviation string `json:"abbreviation"`
	DisplayName  string `json:"displayName"`
	Location     string `json:"location"`
	Name         string `json:"name"`
}

// ESPNSituation represents live game situation data
type ESPNSituation struct {
	LastPlay               ESPNLastPlay `json:"lastPlay,omitempty"`
	Down                   int          `json:"down,omitempty"`
	YardLine               int          `json:"yardLine,omitempty"`
	Distance               int          `json:"distance,omitempty"`
	IsRedZone              bool         `json:"isRedZone"`
	HomeTimeouts           int          `json:"homeTimeouts"`
	AwayTimeouts           int          `json:"awayTimeouts"`
	DownDistanceText       string       `json:"downDistanceText,omitempty"`
	ShortDownDistanceText  string       `json:"shortDownDistanceText,omitempty"`
	PossessionText         string       `json:"possessionText,omitempty"`
	Possession             string       `json:"possession,omitempty"`
}

// ESPNLastPlay represents the last play in a game
type ESPNLastPlay struct {
	ID          string `json:"id,omitempty"`
	Text        string `json:"text,omitempty"`
	ScoreValue  int    `json:"scoreValue,omitempty"`
	StatYardage int    `json:"statYardage,omitempty"`
}

// ESPN Odds API response structures
type ESPNOddsResponse struct {
	Items []ESPNOddsItem `json:"items"`
}

type ESPNOddsItem struct {
	Provider     ESPNProvider `json:"provider"`
	Details      string       `json:"details"`
	OverOdds     float64      `json:"overOdds"`
	UnderOdds    float64      `json:"underOdds"`
	OverUnder    float64      `json:"overUnder"`
	Spread       float64      `json:"spread"`
	HomeTeamOdds ESPNTeamOdds `json:"homeTeamOdds"`
	AwayTeamOdds ESPNTeamOdds `json:"awayTeamOdds"`
}

type ESPNProvider struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type ESPNTeamOdds struct {
	MoneyLine  float64 `json:"moneyLine"`
	SpreadOdds float64 `json:"spreadOdds"`
	Team       ESPNOddsTeamRef `json:"team"`
}

type ESPNOddsTeamRef struct {
	Ref string `json:"$ref"`
}

// GetScoreboard fetches current NFL scoreboard from ESPN
func (e *ESPNService) GetScoreboard() ([]models.Game, error) {
	return e.GetScoreboardForYear(time.Now().Year())
}

// GetScoreboardForYear fetches NFL scoreboard for a specific year (regular season only)
// Uses date range from July to January to capture full season including Week 18
func (e *ESPNService) GetScoreboardForYear(year int) ([]models.Game, error) {
	// NFL season runs from July (year) to January (year+1) to capture Week 18
	startDate := fmt.Sprintf("%d0701", year)     // July 1st
	endDate := fmt.Sprintf("%d0131", year+1)     // January 31st next year
	url := fmt.Sprintf("%s?dates=%s-%s&limit=1000", e.baseURL, startDate, endDate)
	
	log.Printf("ESPN API: Fetching scoreboard from %s", url)
	resp, err := e.client.Get(url)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch ESPN data: %w", err)
	}
	defer resp.Body.Close()

	log.Printf("ESPN API: Scoreboard response status %d", resp.StatusCode)
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("ESPN API returned status %d", resp.StatusCode)
	}

	var espnResp ESPNResponse
	if err := json.NewDecoder(resp.Body).Decode(&espnResp); err != nil {
		return nil, fmt.Errorf("failed to decode ESPN response: %w", err)
	}

	log.Printf("ESPN API: Received %d events", len(espnResp.Events))
	games := e.convertToGames(espnResp.Events)
	log.Printf("ESPN API: Converted to %d games", len(games))
	return games, nil
}

// convertToGames converts ESPN events to our Game model
func (e *ESPNService) convertToGames(events []ESPNEvent) []models.Game {
	games := make([]models.Game, 0, len(events))

	for _, event := range events {
		// Only include regular season games (type 2)
		if event.Season.Type != 2 {
			continue
		}
		
		if len(event.Competitions) == 0 || len(event.Competitions[0].Competitors) < 2 {
			continue
		}

		game := e.convertEvent(event)
		games = append(games, game)
	}

	return games
}

// convertEvent converts a single ESPN event to our Game model
func (e *ESPNService) convertEvent(event ESPNEvent) models.Game {
	competition := event.Competitions[0]
	
	// Parse game ID
	gameID, _ := strconv.Atoi(event.ID)
	
	// Parse date - ESPN uses format like "2024-09-08T00:20Z"
	gameDate, err := time.Parse("2006-01-02T15:04Z", event.Date)
	if err != nil {
		// Try alternative format with seconds
		gameDate, err = time.Parse("2006-01-02T15:04:05Z", event.Date)
		if err != nil {
			log.Printf("ESPN API: Failed to parse date '%s' for game %s: %v", event.Date, event.ID, err)
			gameDate = time.Now() // Fallback to current time
		}
	}
	
	// Determine home/away teams and scores
	var homeTeam, awayTeam string
	var homeScore, awayScore int
	
	for _, competitor := range competition.Competitors {
		score, _ := strconv.Atoi(competitor.Score)
		
		if competitor.HomeAway == "home" {
			homeTeam = competitor.Team.Abbreviation
			homeScore = score
		} else {
			awayTeam = competitor.Team.Abbreviation
			awayScore = score
		}
	}
	
	// Convert status
	state := e.convertGameState(event.Status)
	
	// Debug log the parsing result
	// log.Printf("ESPN API: Game %s (%s vs %s) parsed date from '%s' to '%s'", 
	// 	event.ID, awayTeam, homeTeam, event.Date, gameDate.Format("2006-01-02 15:04:05"))
	
	game := models.Game{
		ID:        gameID,
		Season:    event.Season.Year,
		Date:      gameDate,
		Week:      event.Week.Number,
		Away:      awayTeam,
		Home:      homeTeam,
		State:     state,
		AwayScore: awayScore,
		HomeScore: homeScore,
		Quarter:   event.Status.Period,
	}
	
	// Add live status data if game is in progress and situation data is available
	if state == models.GameStateInPlay && competition.Situation != nil {
		situation := competition.Situation
		game.SetStatus(
			event.Status.DisplayClock,        // displayClock
			event.Status.Type.Name,           // statusName (e.g., "STATUS_HALFTIME")
			situation.Possession,             // possession
			situation.PossessionText,         // possessionText  
			situation.DownDistanceText,       // downDistanceText
			situation.ShortDownDistanceText,  // shortDownDistanceText
			situation.Down,                   // down
			situation.YardLine,               // yardLine
			situation.Distance,               // distance
			situation.HomeTimeouts,           // homeTimeouts
			situation.AwayTimeouts,           // awayTimeouts
			situation.IsRedZone,              // isRedZone
		)
		
		// Debug logging for halftime detection
		if event.ID == "401772510" {
			log.Printf("ESPN API: Game %s DEBUG - Period=%d, DisplayClock=%s, StatusName=%s, StatusDesc=%s", 
				event.ID, event.Status.Period, event.Status.DisplayClock, 
				event.Status.Type.Name, event.Status.Type.Description)
		}

		// Log possession data for debugging
		// Convert ESPN team ID to team abbreviation for possession
		if situation.Possession != "" {
			teamAbbr := e.getTeamAbbrFromID(situation.Possession)
			if teamAbbr != "" {
				// Update the possession field with team abbreviation instead of ID
				game.Status.Possession = teamAbbr
			}
			
			log.Printf("ESPN API: Game %s live status - %s %s at %s", 
				event.ID, situation.Possession, situation.ShortDownDistanceText, situation.PossessionText)
		}
	}
	
	return game
}

// getTeamAbbrFromID converts ESPN team ID to team abbreviation
func (e *ESPNService) getTeamAbbrFromID(teamIDStr string) string {
	// ESPN team ID mapping (reverse of getESPNTeamID)
	teamIDMap := map[string]string{
		"1": "ATL", "2": "BUF", "3": "CHI", "4": "CIN", "5": "CLE", "6": "DAL", "7": "DEN", "8": "DET",
		"9": "GB", "10": "TEN", "11": "IND", "12": "KC", "13": "LV", "14": "LAR", "15": "MIA", "16": "MIN",
		"17": "NE", "18": "NO", "19": "NYG", "20": "NYJ", "21": "PHI", "22": "ARI", "23": "PIT", "24": "LAC",
		"25": "SF", "26": "SEA", "27": "TB", "28": "WSH", "29": "CAR", "30": "JAX", "33": "BAL", "34": "HOU",
	}
	
	if abbr, exists := teamIDMap[teamIDStr]; exists {
		return abbr
	}
	
	// Fallback: return empty string if team not found
	log.Printf("Warning: Unknown ESPN team ID '%s'", teamIDStr)
	return ""
}

// convertGameState converts ESPN status to our GameState
func (e *ESPNService) convertGameState(status ESPNStatus) models.GameState {
	switch strings.ToLower(status.Type.State) {
	case "pre":
		return models.GameStateScheduled
	case "in":
		return models.GameStateInPlay
	case "post":
		return models.GameStateCompleted
	default:
		return models.GameStateScheduled
	}
}

// HealthCheck verifies ESPN API is accessible
func (e *ESPNService) HealthCheck() bool {
	req, err := http.NewRequest("HEAD", e.baseURL, nil)
	if err != nil {
		return false
	}

	resp, err := e.client.Do(req)
	if err != nil {
		return false
	}
	defer resp.Body.Close()

	return resp.StatusCode == http.StatusOK
}

// GetOdds fetches betting odds for a specific game from ESPN
func (e *ESPNService) GetOdds(gameID int) (*models.Odds, error) {
	url := fmt.Sprintf("https://sports.core.api.espn.com/v2/sports/football/leagues/nfl/events/%d/competitions/%d/odds", gameID, gameID)
	
	resp, err := e.client.Get(url)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch odds: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("odds API returned status %d", resp.StatusCode)
	}

	var oddsResp ESPNOddsResponse
	if err := json.NewDecoder(resp.Body).Decode(&oddsResp); err != nil {
		return nil, fmt.Errorf("failed to decode odds response: %w", err)
	}

	// Use the first available odds provider
	if len(oddsResp.Items) == 0 {
		return nil, fmt.Errorf("no odds available for game %d", gameID)
	}

	item := oddsResp.Items[0]
	return &models.Odds{
		Spread: item.Spread,
		OU:     item.OverUnder,
	}, nil
}

// EnrichGamesWithOdds attempts to add odds to games that don't have them
func (e *ESPNService) EnrichGamesWithOdds(games []models.Game) []models.Game {
	return e.EnrichGamesWithOddsLimited(games, len(games))
}

// EnrichGamesWithOddsLimited attempts to add odds to a limited number of games
func (e *ESPNService) EnrichGamesWithOddsLimited(games []models.Game, maxGames int) []models.Game {
	enrichedGames := make([]models.Game, len(games))
	copy(enrichedGames, games)

	log.Printf("ESPN Odds: Starting odds enrichment for up to %d games", maxGames)
	
	count := 0
	successCount := 0
	failedCount := 0
	
	// First pass: prioritize scheduled games without odds
	for i := range enrichedGames {
		if count >= maxGames {
			break
		}
		
		if !enrichedGames[i].HasOdds() && enrichedGames[i].State == models.GameStateScheduled {
			log.Printf("ESPN Odds: Fetching odds for Game %d (%s vs %s)", 
				enrichedGames[i].ID, enrichedGames[i].Away, enrichedGames[i].Home)
			
			if odds, err := e.GetOdds(enrichedGames[i].ID); err == nil {
				log.Printf("ESPN Odds: SUCCESS - Game %d got odds: Spread=%.1f, O/U=%.1f", 
					enrichedGames[i].ID, odds.Spread, odds.OU)
				enrichedGames[i].Odds = odds
				successCount++
			} else {
				log.Printf("ESPN Odds: FAILED - Game %d odds fetch error: %v", enrichedGames[i].ID, err)
				failedCount++
			}
			count++
		}
	}
	
	// Second pass: any remaining games without odds (if we haven't hit the limit)
	for i := range enrichedGames {
		if count >= maxGames {
			break
		}
		
		if !enrichedGames[i].HasOdds() {
			log.Printf("ESPN Odds: Fetching odds for Game %d (%s vs %s) [second pass]", 
				enrichedGames[i].ID, enrichedGames[i].Away, enrichedGames[i].Home)
			
			if odds, err := e.GetOdds(enrichedGames[i].ID); err == nil {
				log.Printf("ESPN Odds: SUCCESS - Game %d got odds: Spread=%.1f, O/U=%.1f", 
					enrichedGames[i].ID, odds.Spread, odds.OU)
				enrichedGames[i].Odds = odds
				successCount++
			} else {
				log.Printf("ESPN Odds: FAILED - Game %d odds fetch error: %v", enrichedGames[i].ID, err)
				failedCount++
			}
			count++
		}
	}

	log.Printf("ESPN Odds: Enrichment complete - %d attempts, %d successful, %d failed", 
		count, successCount, failedCount)
	return enrichedGames
}