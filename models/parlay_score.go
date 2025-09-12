package models

import (
	"time"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

// ParlayScore represents a user's parlay scoring for a specific week
type ParlayScore struct {
	ID          primitive.ObjectID `bson:"_id,omitempty" json:"id"`
	UserID      int               `bson:"user_id" json:"user_id"`
	Season      int               `bson:"season" json:"season"`
	Week        int               `bson:"week" json:"week"`
	RegularPoints      int       `bson:"regular_points" json:"regular_points"`           // Points from weekend games
	BonusThursdayPoints int      `bson:"bonus_thursday_points" json:"bonus_thursday_points"` // Points from Thursday games
	BonusFridayPoints   int      `bson:"bonus_friday_points" json:"bonus_friday_points"`     // Points from Friday games (2024+)
	TotalPoints         int      `bson:"total_points" json:"total_points"`                   // Sum of all points for the week
	CreatedAt    time.Time        `bson:"created_at" json:"created_at"`
	UpdatedAt    time.Time        `bson:"updated_at" json:"updated_at"`
}

// CalculateTotal updates the total points from individual categories
func (ps *ParlayScore) CalculateTotal() {
	ps.TotalPoints = ps.RegularPoints + ps.BonusThursdayPoints + ps.BonusFridayPoints
}


// ParlaySeasonRecord represents a user's parlay performance for an entire season
type ParlaySeasonRecord struct {
	ID          primitive.ObjectID           `bson:"_id,omitempty" json:"id"`
	UserID      int                         `bson:"user_id" json:"user_id"`
	Season      int                         `bson:"season" json:"season"`
	WeekScores  map[int]ParlayWeekScore     `bson:"week_scores" json:"week_scores"`
	TotalPoints int                         `bson:"total_points" json:"total_points"`
	CreatedAt   time.Time                   `bson:"created_at" json:"created_at"`
	UpdatedAt   time.Time                   `bson:"updated_at" json:"updated_at"`
}

// RecalculateTotals recalculates season totals from weekly scores
func (r *ParlaySeasonRecord) RecalculateTotals() {
	total := 0
	for _, week := range r.WeekScores {
		total += week.TotalPoints
	}
	r.TotalPoints = total
	r.UpdatedAt = time.Now()
}

// ParlayWeekScore represents parlay scores for a specific week
type ParlayWeekScore struct {
	Week               int            `bson:"week" json:"week"`
	ThursdayPoints     int            `bson:"thursday_points" json:"thursday_points"`
	FridayPoints       int            `bson:"friday_points" json:"friday_points"`
	SundayMondayPoints int            `bson:"sunday_monday_points" json:"sunday_monday_points"`
	DailyScores        map[string]int `bson:"daily_scores,omitempty" json:"daily_scores,omitempty"` // For modern seasons
	TotalPoints        int            `bson:"total_points" json:"total_points"`
}

// CreateParlayScore creates a new parlay score entry
func CreateParlayScore(userID, season, week int, scores map[ParlayCategory]int) *ParlayScore {
	now := time.Now()
	ps := &ParlayScore{
		UserID:    userID,
		Season:    season,
		Week:      week,
		RegularPoints:       scores[ParlayRegular],
		BonusThursdayPoints: scores[ParlayBonusThursday],
		BonusFridayPoints:   scores[ParlayBonusFriday],
		CreatedAt: now,
		UpdatedAt: now,
	}
	ps.CalculateTotal()
	return ps
}