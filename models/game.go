package models

import (
	"fmt"
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
	Spread float64 `json:"spread"` // Point spread (negative = home team favored)
	OU     float64 `json:"ou"`     // Over/Under total points
}

// Game represents an NFL game with scores and metadata
type Game struct {
	ID        int       `json:"id"`
	Date      time.Time `json:"date"`
	Week      int       `json:"week"`
	Away      string    `json:"away"`
	Home      string    `json:"home"`
	State     GameState `json:"state"`
	AwayScore int       `json:"awayScore"`
	HomeScore int       `json:"homeScore"`
	Quarter   int       `json:"quarter"`
	Odds      *Odds     `json:"odds,omitempty"` // Betting odds (nil if not available)
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