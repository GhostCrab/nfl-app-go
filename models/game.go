package models

import (
	"fmt"
	"strings"
	"time"
)

// GameState represents the current state of a game
type GameState string

const (
	GameStateScheduled GameState = "scheduled"
	GameStateInPlay    GameState = "in_play"
	GameStateCompleted GameState = "completed"
	GameStatePostponed GameState = "postponed"
)

// Odds represents betting odds for a game
type Odds struct {
	Spread float64 `json:"spread" bson:"spread"` // Point spread (negative = home team favored)
	OU     float64 `json:"ou" bson:"ou"`         // Over/Under total points
}

// GameStatus represents live game status information
type GameStatus struct {
	DisplayClock            string `json:"displayClock" bson:"displayClock"`                       // Game clock (e.g., "12:34", "0:00")
	Possession              string `json:"possession,omitempty" bson:"possession,omitempty"`               // Team abbreviation with ball
	PossessionText          string `json:"possessionText,omitempty" bson:"possessionText,omitempty"`       // Field position (e.g., "NYG 25")
	DownDistanceText        string `json:"downDistanceText,omitempty" bson:"downDistanceText,omitempty"`   // Full down/distance text
	ShortDownDistanceText   string `json:"shortDownDistanceText,omitempty" bson:"shortDownDistanceText,omitempty"` // Short down/distance (e.g., "1st & 10")
	Down                    int    `json:"down,omitempty" bson:"down,omitempty"`                           // Current down (1-4)
	YardLine                int    `json:"yardLine,omitempty" bson:"yardLine,omitempty"`                   // Yard line position
	Distance                int    `json:"distance,omitempty" bson:"distance,omitempty"`                   // Yards to go for first down
	IsRedZone               bool   `json:"isRedZone" bson:"isRedZone"`                             // Whether team is in red zone
	HomeTimeouts            int    `json:"homeTimeouts" bson:"homeTimeouts"`                       // Home team timeouts remaining
	AwayTimeouts            int    `json:"awayTimeouts" bson:"awayTimeouts"`                       // Away team timeouts remaining
}

// Game represents an NFL game with scores and metadata
type Game struct {
	ID        int          `json:"id" bson:"id"`
	Season    int          `json:"season" bson:"season"`
	Date      time.Time    `json:"date" bson:"date"`
	Week      int          `json:"week" bson:"week"`
	Away      string       `json:"away" bson:"away"`
	Home      string       `json:"home" bson:"home"`
	State     GameState    `json:"state" bson:"state"`
	AwayScore int          `json:"awayScore" bson:"awayScore"`
	HomeScore int          `json:"homeScore" bson:"homeScore"`
	Quarter   int          `json:"quarter" bson:"quarter"`
	Odds      *Odds        `json:"odds,omitempty" bson:"odds,omitempty"`     // Betting odds (nil if not available)
	Status    *GameStatus  `json:"status,omitempty" bson:"status,omitempty"` // Live game status (nil if not available)
}

// IsCompleted returns true if the game is finished
func (g *Game) IsCompleted() bool {
	return g.State == GameStateCompleted
}

// IsInProgress returns true if the game is currently being played
func (g *Game) IsInProgress() bool {
	return g.State == GameStateInPlay
}

// Winner returns the winning team abbreviation or empty string if tie/not completed
func (g *Game) Winner() string {
	if !g.IsCompleted() {
		return ""
	}
	if g.HomeScore > g.AwayScore {
		return g.Home
	} else if g.AwayScore > g.HomeScore {
		return g.Away
	}
	return "" // tie
}

// ScoreString returns a formatted score string
func (g *Game) ScoreString() string {
	if g.State == GameStateScheduled {
		return "vs"
	}
	return ""
}

// roundToHalf rounds a float to the nearest 0.5 increment
func roundToHalf(val float64) float64 {
	return float64(int(val*2+0.5)) / 2
}

// HasOdds returns true if betting odds are available
func (g *Game) HasOdds() bool {
	return g.Odds != nil
}

// SetOdds sets the betting odds for the game with sanitization
func (g *Game) SetOdds(spread, ou float64) {
	g.Odds = &Odds{
		Spread: roundToHalf(spread),
		OU:     roundToHalf(ou),
	}
}

// SpreadResult returns the result of the spread bet: "covered", "push", or empty if no odds/not completed
func (g *Game) SpreadResult() string {
	if !g.HasOdds() || !g.IsCompleted() {
		return ""
	}
	
	// Calculate the spread-adjusted score difference (home - away)
	scoreDiff := float64(g.HomeScore - g.AwayScore)
	spreadDiff := scoreDiff + g.Odds.Spread // Add spread to actual score difference
	
	if spreadDiff > 0 {
		return "home-covered" // Home team covered
	} else if spreadDiff < 0 {
		return "away-covered" // Away team covered  
	} else {
		return "push" // Exact spread - push
	}
}

// FormatSpread returns a formatted string for the spread
func (g *Game) FormatSpread() string {
	if !g.HasOdds() {
		return ""
	}
	
	if g.Odds.Spread > 0 {
		return fmt.Sprintf("+%.1f", g.Odds.Spread)
	} else if g.Odds.Spread < 0 {
		return fmt.Sprintf("%.1f", g.Odds.Spread)
	} else {
		return "PK" // Pick 'em
	}
}

// FormatAwaySpread returns the spread formatted for the away team
func (g *Game) FormatAwaySpread() string {
	if !g.HasOdds() {
		return ""
	}
	
	// Away team gets opposite of the spread
	awaySpread := -g.Odds.Spread
	if awaySpread > 0 {
		return fmt.Sprintf("+%.1f", awaySpread)
	} else if awaySpread < 0 {
		return fmt.Sprintf("%.1f", awaySpread)
	} else {
		return "PK" // Pick 'em
	}
}

// FormatHomeSpread returns the spread formatted for the home team
func (g *Game) FormatHomeSpread() string {
	if !g.HasOdds() {
		return ""
	}
	
	if g.Odds.Spread > 0 {
		return fmt.Sprintf("+%.1f", g.Odds.Spread)
	} else if g.Odds.Spread < 0 {
		return fmt.Sprintf("%.1f", g.Odds.Spread)
	} else {
		return "PK" // Pick 'em
	}
}

// getTeamIconURL returns the ESPN logo URL for a given team abbreviation or special icon for Over/Under
func getTeamIconURL(teamAbbr string) string {
	if teamAbbr == "" {
		return ""
	}
	// Special cases for Over/Under picks
	if teamAbbr == "OVR" {
		return "https://api.iconify.design/mdi/chevron-double-up.svg"
	}
	if teamAbbr == "UND" {
		return "https://api.iconify.design/mdi/chevron-double-down.svg"
	}
	// Convert to lowercase for ESPN API
	teamLower := strings.ToLower(teamAbbr)
	return fmt.Sprintf("https://a.espncdn.com/combiner/i?img=/i/teamlogos/nfl/500/scoreboard/%s.png", teamLower)
}

// GetAwayTeamIconURL returns the ESPN logo URL for the away team
func (g *Game) GetAwayTeamIconURL() string {
	return getTeamIconURL(g.Away)
}

// GetHomeTeamIconURL returns the ESPN logo URL for the home team
func (g *Game) GetHomeTeamIconURL() string {
	return getTeamIconURL(g.Home)
}

// PacificTime returns the game date converted to Pacific Time for display
func (g *Game) PacificTime() time.Time {
	// Load Pacific timezone (handles PST/PDT automatically)
	loc, err := time.LoadLocation("America/Los_Angeles")
	if err != nil {
		// Fallback to manual UTC offset if timezone loading fails
		// Use UTC-8 (PST) as default since most NFL season is in PST
		return g.Date.Add(-8 * time.Hour)
	}
	
	return g.Date.In(loc)
}

// FormatGameTime returns the game time formatted for Pacific timezone display
func (g *Game) FormatGameTime() string {
	pacificTime := g.PacificTime()
	return pacificTime.Format("1/2/06 3:04 PM")
}

// HasStatus returns true if live game status is available
func (g *Game) HasStatus() bool {
	return g.Status != nil
}

// SetStatus sets the live game status
func (g *Game) SetStatus(displayClock, possession, possessionText, downDistanceText, shortDownDistanceText string,
	down, yardLine, distance, homeTimeouts, awayTimeouts int, isRedZone bool) {
	g.Status = &GameStatus{
		DisplayClock:          displayClock,
		Possession:            possession,
		PossessionText:        possessionText,
		DownDistanceText:      downDistanceText,
		ShortDownDistanceText: shortDownDistanceText,
		Down:                  down,
		YardLine:              yardLine,
		Distance:              distance,
		IsRedZone:             isRedZone,
		HomeTimeouts:          homeTimeouts,
		AwayTimeouts:          awayTimeouts,
	}
}

// GetGameClock returns the display clock or empty string if not available
func (g *Game) GetGameClock() string {
	if g.HasStatus() {
		return g.Status.DisplayClock
	}
	return ""
}

// GetPossessionString returns formatted possession information like "NYG 1st & 10 at NYG 25"
func (g *Game) GetPossessionString() string {
	if !g.HasStatus() || g.Status.Possession == "" {
		return ""
	}
	
	parts := []string{}
	if g.Status.Possession != "" {
		parts = append(parts, g.Status.Possession)
	}
	if g.Status.ShortDownDistanceText != "" {
		parts = append(parts, g.Status.ShortDownDistanceText)
	}
	if g.Status.PossessionText != "" {
		parts = append(parts, fmt.Sprintf("at %s", g.Status.PossessionText))
	}
	
	if len(parts) > 0 {
		return strings.Join(parts, " ")
	}
	return ""
}

// GetLiveStatusString returns formatted live status like "Q1 12:34: NYG 1st & 10 at NYG 25"
func (g *Game) GetLiveStatusString() string {
	if !g.IsInProgress() {
		return ""
	}
	
	parts := []string{}
	
	// Quarter and clock
	quarterStr := fmt.Sprintf("Q%d", g.Quarter)
	if g.Quarter == 5 {
		quarterStr = "OT"
	} else if g.Quarter == 6 {
		quarterStr = "Halftime"
	}
	
	if g.HasStatus() && g.Status.DisplayClock != "" {
		if g.Status.DisplayClock == "0:00" {
			if g.Quarter == 2 {
				quarterStr = "Halftime"
			} else {
				quarterStr = fmt.Sprintf("End %s", quarterStr)
			}
		} else {
			quarterStr = fmt.Sprintf("%s %s", quarterStr, g.Status.DisplayClock)
		}
	}
	parts = append(parts, quarterStr)
	
	// Possession info
	possessionStr := g.GetPossessionString()
	if possessionStr != "" {
		parts = append(parts, possessionStr)
	}
	
	return strings.Join(parts, ": ")
}