package models

import (
	"time"
)

// WeeklyScore represents a player's score for a specific week and day type
type WeeklyScore struct {
	ID           string    `json:"id" bson:"_id,omitempty"`
	UserID       int       `json:"user_id" bson:"user_id"`
	Season       int       `json:"season" bson:"season"`
	Week         int       `json:"week" bson:"week"`
	DayType      string    `json:"day_type" bson:"day_type"` // "regular", "bonus_thursday", "bonus_friday"
	TotalPicks   int       `json:"total_picks" bson:"total_picks"`
	WinningPicks int       `json:"winning_picks" bson:"winning_picks"`
	LosingPicks  int       `json:"losing_picks" bson:"losing_picks"`
	PushPicks    int       `json:"push_picks" bson:"push_picks"`
	PendingPicks int       `json:"pending_picks" bson:"pending_picks"`
	Points       int       `json:"points" bson:"points"`
	IsComplete   bool      `json:"is_complete" bson:"is_complete"` // All games finished
	CreatedAt    time.Time `json:"created_at" bson:"created_at"`
	UpdatedAt    time.Time `json:"updated_at" bson:"updated_at"`
}

// CalculatePoints determines points based on parlay rules
func (ws *WeeklyScore) CalculatePoints() int {
	// If any pick lost, no points
	if ws.LosingPicks > 0 {
		return 0
	}
	
	// If there are still pending picks, can't award points yet
	if ws.PendingPicks > 0 {
		return 0
	}
	
	// Points = number of winning picks (pushes don't count)
	return ws.WinningPicks
}

// UpdateFromPicks recalculates the weekly score based on current pick results
func (ws *WeeklyScore) UpdateFromPicks(picks []*Pick) {
	ws.TotalPicks = len(picks)
	ws.WinningPicks = 0
	ws.LosingPicks = 0
	ws.PushPicks = 0
	ws.PendingPicks = 0
	
	for _, pick := range picks {
		switch pick.Result {
		case "win":
			ws.WinningPicks++
		case "loss":
			ws.LosingPicks++
		case "push":
			ws.PushPicks++
		default: // pending or unknown
			ws.PendingPicks++
		}
	}
	
	ws.Points = ws.CalculatePoints()
	ws.IsComplete = ws.PendingPicks == 0
	ws.UpdatedAt = time.Now()
}

// SeasonScore represents a player's total score for a season
type SeasonScore struct {
	UserID      int                    `json:"user_id" bson:"user_id"`
	Season      int                    `json:"season" bson:"season"`
	TotalPoints int                    `json:"total_points" bson:"total_points"`
	WeeklyBreakdown map[int]WeekSummary `json:"weekly_breakdown" bson:"weekly_breakdown"`
	UpdatedAt   time.Time              `json:"updated_at" bson:"updated_at"`
}

// WeekSummary represents a summary of all scoring for a specific week
type WeekSummary struct {
	Week            int `json:"week" bson:"week"`
	RegularPoints   int `json:"regular_points" bson:"regular_points"`
	BonusThurPoints int `json:"bonus_thursday_points" bson:"bonus_thursday_points"`
	BonusFriPoints  int `json:"bonus_friday_points" bson:"bonus_friday_points"`
	WeekTotal       int `json:"week_total" bson:"week_total"`
}

// GetWeekTotal returns total points for the week
func (ws *WeekSummary) GetWeekTotal() int {
	return ws.RegularPoints + ws.BonusThurPoints + ws.BonusFriPoints
}