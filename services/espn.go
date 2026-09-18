package services

import (
	"encoding/json"
	"fmt"
	"net/http"
	"nfl-app-go/logging"
	"nfl-app-go/models"
	"strconv"
	"strings"
	"time"
)

// ESPNService handles ESPN API interactions
type ESPNService struct {
	client  *http.Client
	baseURL string
	logger  *logging.Logger
}

// NewESPNService creates a new ESPN service
func NewESPNService() *ESPNService {
	logger := logging.WithPrefix("espn_service")

	return &ESPNService{
		client:  &http.Client{Timeout: 10 * time.Second},
		baseURL: "https://site.api.espn.com/apis/site/v2/sports/football/nfl/scoreboard",
		logger:  logger,
	}
}

// ESPN API response structures
type ESPNResponse struct {
	Events []ESPNEvent `json:"events"`
}

type ESPNEvent struct {
	ID           string            `json:"id"`
	Date         string            `json:"date"`
	Week         ESPNWeek          `json:"week"`
	Season       ESPNSeason        `json:"season"`
	Status       ESPNStatus        `json:"status"`
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
	ID       string   `json:"id"`
	HomeAway string   `json:"homeAway"`
	Score    string   `json:"score"`
	Team     ESPNTeam `json:"team"`
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
	LastPlay              ESPNLastPlay `json:"lastPlay,omitempty"`
	Down                  int          `json:"down,omitempty"`
	YardLine              int          `json:"yardLine,omitempty"`
	Distance              int          `json:"distance,omitempty"`
	IsRedZone             bool         `json:"isRedZone"`
	HomeTimeouts          int          `json:"homeTimeouts"`
	AwayTimeouts          int          `json:"awayTimeouts"`
	DownDistanceText      string       `json:"downDistanceText,omitempty"`
	ShortDownDistanceText string       `json:"shortDownDistanceText,omitempty"`
	PossessionText        string       `json:"possessionText,omitempty"`
	Possession            string       `json:"possession,omitempty"`
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
	MoneyLine  float64         `json:"moneyLine"`
	SpreadOdds float64         `json:"spreadOdds"`
	Team       ESPNOddsTeamRef `json:"team"`
}

type ESPNOddsTeamRef struct {
	Ref string `json:"$ref"`
}

// Detailed odds structures for individual provider endpoint
type ESPNDetailedOddsResponse struct {
	Provider     ESPNProvider          `json:"provider"`
	Details      string                `json:"details"`
	OverUnder    float64               `json:"overUnder"`
	Spread       float64               `json:"spread"`
	OverOdds     float64               `json:"overOdds"`
	UnderOdds    float64               `json:"underOdds"`
	HomeTeamOdds ESPNDetailedTeamOdds  `json:"homeTeamOdds"`
	AwayTeamOdds ESPNDetailedTeamOdds  `json:"awayTeamOdds"`
	Open         *ESPNDetailedOpenOdds `json:"open,omitempty"`
	Current      *ESPNDetailedOpenOdds `json:"current,omitempty"`
}

type ESPNDetailedTeamOdds struct {
	MoneyLine  float64 `json:"moneyLine"`
	SpreadOdds float64 `json:"spreadOdds"`
	Favorite   bool    `json:"favorite"`
	Underdog   bool    `json:"underdog"`
}

type ESPNDetailedOpenOdds struct {
	Total *ESPNOddsTotal `json:"total,omitempty"`
}

type ESPNOddsTotal struct {
	AlternateDisplayValue string  `json:"alternateDisplayValue,omitempty"`
	American              string  `json:"american,omitempty"`
	Value                 float64 `json:"value,omitempty"`
}

// GetScoreboard fetches current NFL scoreboard from ESPN
func (e *ESPNService) GetScoreboard() ([]models.Game, error) {
	return e.GetScoreboardForYear(time.Now().Year())
}

// RegularSeasonWeeks is the number of weeks in an NFL regular season.
const RegularSeasonWeeks = 18

// GetScoreboardForWeek fetches one week of the regular season.
//
// ESPN dropped support for the hyphenated "dates=START-END" range syntax this
// endpoint used to accept - every range form now returns HTTP 400, even a
// single-day range. Querying by season and week is the supported replacement
// and is also far cheaper than pulling the whole season for a live-score poll.
func (e *ESPNService) GetScoreboardForWeek(year, week int) ([]models.Game, error) {
	url := fmt.Sprintf("%s?dates=%d&seasontype=2&week=%d", e.baseURL, year, week)

	resp, err := e.client.Get(url)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch ESPN data for %d week %d: %w", year, week, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("ESPN API returned status %d for %d week %d", resp.StatusCode, year, week)
	}

	var espnResp ESPNResponse
	if err := json.NewDecoder(resp.Body).Decode(&espnResp); err != nil {
		return nil, fmt.Errorf("failed to decode ESPN response for %d week %d: %w", year, week, err)
	}

	return e.convertToGames(espnResp.Events), nil
}

// GetScoreboardForYear fetches the full regular season, week by week.
//
// A failed week is logged and skipped rather than failing the whole season, so
// one bad response cannot blank out an otherwise good sync. An error is only
// returned when nothing at all could be fetched.
func (e *ESPNService) GetScoreboardForYear(year int) ([]models.Game, error) {
	var all []models.Game
	var firstErr error
	failed := 0

	for week := 1; week <= RegularSeasonWeeks; week++ {
		games, err := e.GetScoreboardForWeek(year, week)
		if err != nil {
			failed++
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		all = append(all, games...)
	}

	if len(all) == 0 {
		if firstErr != nil {
			return nil, fmt.Errorf("no games fetched for %d (%d weeks failed): %w", year, failed, firstErr)
		}
		return nil, fmt.Errorf("no games returned for %d", year)
	}

	return all, nil
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
			e.logger.Errorf("Failed to parse date '%s' for game %s: %v", event.Date, event.ID, err)
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
	// e.logger.Debugf("Game %s (%s vs %s) parsed date from '%s' to '%s'",
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
			event.Status.DisplayClock,       // displayClock
			event.Status.Type.Name,          // statusName (e.g., "STATUS_HALFTIME")
			situation.Possession,            // possession
			situation.PossessionText,        // possessionText
			situation.DownDistanceText,      // downDistanceText
			situation.ShortDownDistanceText, // shortDownDistanceText
			situation.Down,                  // down
			situation.YardLine,              // yardLine
			situation.Distance,              // distance
			situation.HomeTimeouts,          // homeTimeouts
			situation.AwayTimeouts,          // awayTimeouts
			situation.IsRedZone,             // isRedZone
		)

		// Debug logging for halftime detection
		if event.ID == "401772510" {
			e.logger.Debugf("Game %s DEBUG - Period=%d, DisplayClock=%s, StatusName=%s, StatusDesc=%s",
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

			e.logger.Infof("Game %s live status - %s %s at %s",
				event.ID, situation.Possession, situation.ShortDownDistanceText, situation.PossessionText)
		}
	}

	return game
}

// getTeamAbbrFromID converts ESPN team ID to team abbreviation
func (e *ESPNService) getTeamAbbrFromID(teamIDStr string) string {
	// ESPN team ID mapping (reverse of GetESPNTeamID)
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
	e.logger.Warnf("Unknown ESPN team ID '%s'", teamIDStr)
	return ""
}

// GetESPNTeamID converts team abbreviation to ESPN team ID
func GetESPNTeamID(abbr string) int {
	teamIDMap := map[string]int{
		"ATL": 1, "BUF": 2, "CHI": 3, "CIN": 4, "CLE": 5, "DAL": 6, "DEN": 7, "DET": 8,
		"GB": 9, "TEN": 10, "IND": 11, "KC": 12, "LV": 13, "LAR": 14, "MIA": 15, "MIN": 16,
		"NE": 17, "NO": 18, "NYG": 19, "NYJ": 20, "PHI": 21, "ARI": 22, "PIT": 23, "LAC": 24,
		"SF": 25, "SEA": 26, "TB": 27, "WSH": 28, "CAR": 29, "JAX": 30, "BAL": 33, "HOU": 34,
	}

	if id, exists := teamIDMap[abbr]; exists {
		return id
	}

	return 0 // Unknown team
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

// https://sports.core.api.espn.com/v2/sports/football/leagues/nfl/events/401772941/competitions/401772941/odds
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

	e.logger.Infof("Starting odds enrichment for up to %d games", maxGames)

	count := 0
	successCount := 0
	failedCount := 0

	// First pass: prioritize scheduled games without odds
	for i := range enrichedGames {
		if count >= maxGames {
			break
		}

		if enrichedGames[i].State == models.GameStateScheduled {
			e.logger.Infof("Fetching odds for Game %d Week %d (%s vs %s)",
				enrichedGames[i].ID, enrichedGames[i].Week, enrichedGames[i].Away, enrichedGames[i].Home)

			if odds, err := e.GetOdds(enrichedGames[i].ID); err == nil {
				e.logger.Infof("SUCCESS - Game %d got odds: Spread=%.1f, O/U=%.1f",
					enrichedGames[i].ID, odds.Spread, odds.OU)
				enrichedGames[i].Odds = odds
				successCount++
			} else {
				e.logger.Errorf("FAILED - Game %d odds fetch error: %v", enrichedGames[i].ID, err)
				failedCount++
			}
			count++
		}
	}

	e.logger.Infof("Enrichment complete - %d attempts, %d successful, %d failed",
		count, successCount, failedCount)
	return enrichedGames
}

// ComprehensiveOddsData aggregates all odds information for a game
type ComprehensiveOddsData struct {
	CurrentOdds   *CurrentOdds
	OpeningOdds   *OpeningOdds
	OddsMovement  []OddsSnapshot
	HomeTeamStats *TeamOddsStats
	AwayTeamStats *TeamOddsStats
}

// CurrentOdds represents current betting lines
type CurrentOdds struct {
	Spread    float64
	OverUnder float64
	Details   string
}

// OpeningOdds represents opening betting lines
type OpeningOdds struct {
	Spread    float64
	OverUnder float64
}

// OddsSnapshot represents odds at a specific point in time
type OddsSnapshot struct {
	Timestamp time.Time
	Spread    float64
	OverUnder float64
}

// TeamOddsStats represents historical performance metrics for a team
type TeamOddsStats struct {
	TeamAbbr      string
	ATSRecord     string // e.g., "8-5-1" (wins-losses-pushes)
	ATSPercentage float64
	OURecord      string
	OUPercentage  float64
}

// ESPN Odds Records response structures (for /odds-records endpoint)
type ESPNOddsRecordsResponse struct {
	Count int                  `json:"count"`
	Items []ESPNOddsRecordItem `json:"items"`
}

type ESPNOddsRecordItem struct {
	Abbreviation     string               `json:"abbreviation"`
	DisplayName      string               `json:"displayName"`
	ShortDisplayName string               `json:"shortDisplayName"`
	Type             string               `json:"type"`
	Stats            []ESPNOddsRecordStat `json:"stats"`
}

type ESPNOddsRecordStat struct {
	DisplayName  string  `json:"displayName"`
	Abbreviation string  `json:"abbreviation"`
	Type         string  `json:"type"`
	Value        float64 `json:"value"`
	DisplayValue string  `json:"displayValue"`
}

// ESPN Odds History/Movement response structures
type ESPNOddsMovementResponse struct {
	Count int                    `json:"count"`
	Items []ESPNOddsMovementItem `json:"items"`
}

type ESPNOddsMovementItem struct {
	Timestamp string  `json:"timestamp"`
	Spread    float64 `json:"spread"`
	OverUnder float64 `json:"overUnder"`
}

// GetComprehensiveOdds fetches all odds data from ESPN API Section 7 endpoints
func (e *ESPNService) GetComprehensiveOdds(gameID int, homeTeamID, awayTeamID int, season int) (*ComprehensiveOddsData, error) {
	data := &ComprehensiveOddsData{}

	// 1. Fetch current/opening odds from provider 58
	currentOdds, openOdds, err := e.fetchCurrentAndOpenOdds(gameID)
	if err != nil {
		e.logger.Warnf("Failed to fetch current odds for game %d: %v", gameID, err)
	} else {
		data.CurrentOdds = currentOdds
		data.OpeningOdds = openOdds
	}

	// 3. Fetch team ATS records and stats for both teams
	homeStats, err := e.fetchTeamATSStats(homeTeamID, season)
	if err != nil {
		e.logger.Warnf("Failed to fetch home team ATS stats: %v", err)
	} else {
		data.HomeTeamStats = homeStats
	}

	awayStats, err := e.fetchTeamATSStats(awayTeamID, season)
	if err != nil {
		e.logger.Warnf("Failed to fetch away team ATS stats: %v", err)
	} else {
		data.AwayTeamStats = awayStats
	}

	return data, nil
}

// fetchCurrentAndOpenOdds fetches current and opening odds from provider 58
func (e *ESPNService) fetchCurrentAndOpenOdds(gameID int) (*CurrentOdds, *OpeningOdds, error) {
	url := fmt.Sprintf("https://sports.core.api.espn.com/v2/sports/football/leagues/nfl/events/%d/competitions/%d/odds/58", gameID, gameID)

	resp, err := e.client.Get(url)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to fetch odds: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, nil, fmt.Errorf("odds API returned status %d", resp.StatusCode)
	}

	var oddsResp ESPNDetailedOddsResponse
	if err := json.NewDecoder(resp.Body).Decode(&oddsResp); err != nil {
		return nil, nil, fmt.Errorf("failed to decode odds response: %w", err)
	}

	// Current odds
	current := &CurrentOdds{
		Spread:    oddsResp.Spread,
		OverUnder: oddsResp.OverUnder,
		Details:   oddsResp.Details,
	}

	// Opening odds
	opening := &OpeningOdds{
		Spread:    oddsResp.Spread, // Default to current if no open data
		OverUnder: oddsResp.OverUnder,
	}

	// Extract opening odds if available
	if oddsResp.Open != nil && oddsResp.Open.Total != nil {
		if oddsResp.Open.Total.Value > 0 {
			opening.OverUnder = oddsResp.Open.Total.Value
		} else if oddsResp.Open.Total.AlternateDisplayValue != "" {
			// Try parsing from alternate display value
			var parsedOU float64
			if _, err := fmt.Sscanf(oddsResp.Open.Total.AlternateDisplayValue, "%f", &parsedOU); err == nil {
				opening.OverUnder = parsedOU
			}
		}
	}

	// Extract opening spread from team odds if available
	if oddsResp.HomeTeamOdds.Favorite && oddsResp.AwayTeamOdds.Underdog {
		// Home team is favored, so spread should be negative
		opening.Spread = oddsResp.Spread
	} else if oddsResp.AwayTeamOdds.Favorite && oddsResp.HomeTeamOdds.Underdog {
		// Away team is favored, so spread should be positive
		opening.Spread = oddsResp.Spread
	}

	return current, opening, nil
}

// fetchTeamATSStats fetches Against-the-Spread records for a team using odds-records endpoint
func (e *ESPNService) fetchTeamATSStats(teamID int, season int) (*TeamOddsStats, error) {
	// Use type 0 for season-wide records
	url := fmt.Sprintf("https://sports.core.api.espn.com/v2/sports/football/leagues/nfl/seasons/%d/types/0/teams/%d/odds-records", season, teamID)

	resp, err := e.client.Get(url)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch odds records: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("odds records API returned status %d", resp.StatusCode)
	}

	var recordsResp ESPNOddsRecordsResponse
	if err := json.NewDecoder(resp.Body).Decode(&recordsResp); err != nil {
		return nil, fmt.Errorf("failed to decode odds records response: %w", err)
	}

	stats := &TeamOddsStats{}

	// Find ATS (spread) overall record and O/U records
	for _, item := range recordsResp.Items {
		if item.Type == "spreadOverall" {
			var wins, losses, pushes int
			for _, stat := range item.Stats {
				switch stat.Type {
				case "win":
					wins = int(stat.Value)
				case "loss":
					losses = int(stat.Value)
				case "push":
					pushes = int(stat.Value)
				}
			}

			stats.ATSRecord = fmt.Sprintf("%d-%d-%d", wins, losses, pushes)
			totalGames := wins + losses
			if totalGames > 0 {
				stats.ATSPercentage = float64(wins) / float64(totalGames) * 100
			}
		}

		// Track O/U records
		if item.Type == "overUnderOverall" {
			var overs, unders, pushes int
			for _, stat := range item.Stats {
				switch stat.Type {
				case "over":
					overs = int(stat.Value)
				case "under":
					unders = int(stat.Value)
				case "push":
					pushes = int(stat.Value)
				}
			}

			stats.OURecord = fmt.Sprintf("%d-%d-%d", overs, unders, pushes)
			totalGames := overs + unders
			if totalGames > 0 {
				stats.OUPercentage = float64(overs) / float64(totalGames) * 100
			}
		}
	}

	return stats, nil
}
