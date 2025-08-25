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