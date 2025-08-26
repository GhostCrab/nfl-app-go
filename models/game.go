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

// Game represents an NFL game with scores and metadata
type Game struct {
	ID        int       `json:"id" bson:"id"`
	Season    int       `json:"season" bson:"season"`
	Date      time.Time `json:"date" bson:"date"`
	Week      int       `json:"week" bson:"week"`
	Away      string    `json:"away" bson:"away"`
	Home      string    `json:"home" bson:"home"`
	State     GameState `json:"state" bson:"state"`
	AwayScore int       `json:"awayScore" bson:"awayScore"`
	HomeScore int       `json:"homeScore" bson:"homeScore"`
	Quarter   int       `json:"quarter" bson:"quarter"`
	Odds      *Odds     `json:"odds,omitempty" bson:"odds,omitempty"` // Betting odds (nil if not available)
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

// getTeamIconURL returns the ESPN logo URL for a given team abbreviation
func getTeamIconURL(teamAbbr string) string {
	if teamAbbr == "" {
		return ""
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