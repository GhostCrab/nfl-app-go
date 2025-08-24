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

// GetAwayTeamIconURL returns the Google Static CDN URL for the away team's logo
func (g *Game) GetAwayTeamIconURL() string {
	// Import would be needed at package level, but for now we'll hardcode the mapping
	teams := map[string]string{
		"BUF": "https://ssl.gstatic.com/onebox/media/sports/logos/_RMCkIDTISqCPcSoEvRDhg_48x48.png",
		"MIA": "https://ssl.gstatic.com/onebox/media/sports/logos/1ysKnl7VwOQO8g94gbjKdQ_48x48.png",
		"NE":  "https://ssl.gstatic.com/onebox/media/sports/logos/z89hPEH9DZbpIYmF72gSaw_48x48.png",
		"NYJ": "https://ssl.gstatic.com/onebox/media/sports/logos/T4TxwDGkrCfTrL6Flg9ktQ_48x48.png",
		"BAL": "https://ssl.gstatic.com/onebox/media/sports/logos/1vlEqqoyb9uTqBYiBeNH-w_48x48.png",
		"CIN": "https://ssl.gstatic.com/onebox/media/sports/logos/wDDRqMa40nidAOA5883Vmw_48x48.png",
		"CLE": "https://ssl.gstatic.com/onebox/media/sports/logos/bTzlW33n9s53DxRzmlZXyg_48x48.png",
		"PIT": "https://ssl.gstatic.com/onebox/media/sports/logos/mdUFLAswQ4jZ6V7jXqaxig_48x48.png",
		"HOU": "https://ssl.gstatic.com/onebox/media/sports/logos/sSUn9HRpYLQtEFF2aG9T8Q_48x48.png",
		"IND": "https://ssl.gstatic.com/onebox/media/sports/logos/zOE7BhKadEjaSrrFjcnR4w_48x48.png",
		"JAX": "https://ssl.gstatic.com/onebox/media/sports/logos/HLfqVCxzVx5CUDQ07GLeWg_48x48.png",
		"TEN": "https://ssl.gstatic.com/onebox/media/sports/logos/9J9dhhLeSa3syZ1bWXRjaw_48x48.png",
		"DEN": "https://ssl.gstatic.com/onebox/media/sports/logos/ZktET_o_WU6Mm1sJzJLZhQ_48x48.png",
		"KC":  "https://ssl.gstatic.com/onebox/media/sports/logos/5N0l1KbG1BHPyP8_S7SOXg_48x48.png",
		"LV":  "https://ssl.gstatic.com/onebox/media/sports/logos/QysqoqJQsTbiJl8sPL12Yg_48x48.png",
		"LAC": "https://ssl.gstatic.com/onebox/media/sports/logos/EAQRZu91bwn1l8brW9HWBQ_48x48.png",
		"DAL": "https://ssl.gstatic.com/onebox/media/sports/logos/-zeHm0cuBjZXc2HRxRAI0g_48x48.png",
		"NYG": "https://ssl.gstatic.com/onebox/media/sports/logos/q8qdTYh-OWR5uO_QZxFENw_48x48.png",
		"PHI": "https://ssl.gstatic.com/onebox/media/sports/logos/s4ab0JjXpDOespDSf9Z14Q_48x48.png",
		"WAS": "https://ssl.gstatic.com/onebox/media/sports/logos/o0CCwss-QfFnJaVdGIHFmQ_48x48.png",
		"CHI": "https://ssl.gstatic.com/onebox/media/sports/logos/7uaGv3B13mXyBhHcTysHcA_48x48.png",
		"DET": "https://ssl.gstatic.com/onebox/media/sports/logos/WE1l856fyyHh6eAbbb8hQQ_48x48.png",
		"GB":  "https://ssl.gstatic.com/onebox/media/sports/logos/IlA4VGrUHzSVLCOcHsRKgg_48x48.png",
		"MIN": "https://ssl.gstatic.com/onebox/media/sports/logos/Vmg4u0BSYZ-1Mc-5uyvxHg_48x48.png",
		"ATL": "https://ssl.gstatic.com/onebox/media/sports/logos/QNdwQPxtIRYUhnMBYq-bSA_48x48.png",
		"CAR": "https://ssl.gstatic.com/onebox/media/sports/logos/HsLg5tW_S7566EbsMPlcVQ_48x48.png",
		"NO":  "https://ssl.gstatic.com/onebox/media/sports/logos/AC5-UEeN3V_fjkdFXtHWfQ_48x48.png",
		"TB":  "https://ssl.gstatic.com/onebox/media/sports/logos/efP_3b5BgkGE-HMCHx4huQ_48x48.png",
		"ARI": "https://ssl.gstatic.com/onebox/media/sports/logos/5Mh3xcc8uAsxAi3WZvfEyQ_48x48.png",
		"LAR": "https://ssl.gstatic.com/onebox/media/sports/logos/CXW68CjwPIaUurbvSUSyJw_48x48.png",
		"SF":  "https://ssl.gstatic.com/onebox/media/sports/logos/ku3s7M4k5KMagYcFTCie_g_48x48.png",
		"SEA": "https://ssl.gstatic.com/onebox/media/sports/logos/iVPY42GLuHmD05DiOvNSVg_48x48.png",
	}
	
	if iconURL, exists := teams[g.Away]; exists {
		return iconURL
	}
	return ""
}

// GetHomeTeamIconURL returns the Google Static CDN URL for the home team's logo
func (g *Game) GetHomeTeamIconURL() string {
	// Use same team mapping as away team
	teams := map[string]string{
		"BUF": "https://ssl.gstatic.com/onebox/media/sports/logos/_RMCkIDTISqCPcSoEvRDhg_48x48.png",
		"MIA": "https://ssl.gstatic.com/onebox/media/sports/logos/1ysKnl7VwOQO8g94gbjKdQ_48x48.png",
		"NE":  "https://ssl.gstatic.com/onebox/media/sports/logos/z89hPEH9DZbpIYmF72gSaw_48x48.png",
		"NYJ": "https://ssl.gstatic.com/onebox/media/sports/logos/T4TxwDGkrCfTrL6Flg9ktQ_48x48.png",
		"BAL": "https://ssl.gstatic.com/onebox/media/sports/logos/1vlEqqoyb9uTqBYiBeNH-w_48x48.png",
		"CIN": "https://ssl.gstatic.com/onebox/media/sports/logos/wDDRqMa40nidAOA5883Vmw_48x48.png",
		"CLE": "https://ssl.gstatic.com/onebox/media/sports/logos/bTzlW33n9s53DxRzmlZXyg_48x48.png",
		"PIT": "https://ssl.gstatic.com/onebox/media/sports/logos/mdUFLAswQ4jZ6V7jXqaxig_48x48.png",
		"HOU": "https://ssl.gstatic.com/onebox/media/sports/logos/sSUn9HRpYLQtEFF2aG9T8Q_48x48.png",
		"IND": "https://ssl.gstatic.com/onebox/media/sports/logos/zOE7BhKadEjaSrrFjcnR4w_48x48.png",
		"JAX": "https://ssl.gstatic.com/onebox/media/sports/logos/HLfqVCxzVx5CUDQ07GLeWg_48x48.png",
		"TEN": "https://ssl.gstatic.com/onebox/media/sports/logos/9J9dhhLeSa3syZ1bWXRjaw_48x48.png",
		"DEN": "https://ssl.gstatic.com/onebox/media/sports/logos/ZktET_o_WU6Mm1sJzJLZhQ_48x48.png",
		"KC":  "https://ssl.gstatic.com/onebox/media/sports/logos/5N0l1KbG1BHPyP8_S7SOXg_48x48.png",
		"LV":  "https://ssl.gstatic.com/onebox/media/sports/logos/QysqoqJQsTbiJl8sPL12Yg_48x48.png",
		"LAC": "https://ssl.gstatic.com/onebox/media/sports/logos/EAQRZu91bwn1l8brW9HWBQ_48x48.png",
		"DAL": "https://ssl.gstatic.com/onebox/media/sports/logos/-zeHm0cuBjZXc2HRxRAI0g_48x48.png",
		"NYG": "https://ssl.gstatic.com/onebox/media/sports/logos/q8qdTYh-OWR5uO_QZxFENw_48x48.png",
		"PHI": "https://ssl.gstatic.com/onebox/media/sports/logos/s4ab0JjXpDOespDSf9Z14Q_48x48.png",
		"WAS": "https://ssl.gstatic.com/onebox/media/sports/logos/o0CCwss-QfFnJaVdGIHFmQ_48x48.png",
		"CHI": "https://ssl.gstatic.com/onebox/media/sports/logos/7uaGv3B13mXyBhHcTysHcA_48x48.png",
		"DET": "https://ssl.gstatic.com/onebox/media/sports/logos/WE1l856fyyHh6eAbbb8hQQ_48x48.png",
		"GB":  "https://ssl.gstatic.com/onebox/media/sports/logos/IlA4VGrUHzSVLCOcHsRKgg_48x48.png",
		"MIN": "https://ssl.gstatic.com/onebox/media/sports/logos/Vmg4u0BSYZ-1Mc-5uyvxHg_48x48.png",
		"ATL": "https://ssl.gstatic.com/onebox/media/sports/logos/QNdwQPxtIRYUhnMBYq-bSA_48x48.png",
		"CAR": "https://ssl.gstatic.com/onebox/media/sports/logos/HsLg5tW_S7566EbsMPlcVQ_48x48.png",
		"NO":  "https://ssl.gstatic.com/onebox/media/sports/logos/AC5-UEeN3V_fjkdFXtHWfQ_48x48.png",
		"TB":  "https://ssl.gstatic.com/onebox/media/sports/logos/efP_3b5BgkGE-HMCHx4huQ_48x48.png",
		"ARI": "https://ssl.gstatic.com/onebox/media/sports/logos/5Mh3xcc8uAsxAi3WZvfEyQ_48x48.png",
		"LAR": "https://ssl.gstatic.com/onebox/media/sports/logos/CXW68CjwPIaUurbvSUSyJw_48x48.png",
		"SF":  "https://ssl.gstatic.com/onebox/media/sports/logos/ku3s7M4k5KMagYcFTCie_g_48x48.png",
		"SEA": "https://ssl.gstatic.com/onebox/media/sports/logos/iVPY42GLuHmD05DiOvNSVg_48x48.png",
	}
	
	if iconURL, exists := teams[g.Home]; exists {
		return iconURL
	}
	return ""
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
	return pacificTime.Format("Mon 3:04 PM")
}