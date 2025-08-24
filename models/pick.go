package models

import (
	"fmt"
	"time"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

// Pick represents a user's prediction for a game
// Compatible with legacy import format but includes season tracking for multi-year storage
type Pick struct {
	ID        primitive.ObjectID `bson:"_id,omitempty" json:"id"`
	UserID    int               `bson:"user_id" json:"user_id"`         // Maps to legacy "user" field
	GameID    int               `bson:"game_id" json:"game_id"`         // Maps to legacy "game" field (ESPN game ID)
	TeamID    int               `bson:"team_id" json:"team_id"`         // Maps to legacy "team" field
	PickType  PickType          `bson:"pick_type" json:"pick_type"`     // Derived from team_id (98/99 = over/under, others = spread)
	Season    int               `bson:"season" json:"season"`           // NEW: Season tracking for multi-year storage
	Week      int               `bson:"week" json:"week"`               // Derived from game data during import
	Result    PickResult        `bson:"result" json:"result"`           // Calculated after game completion
	CreatedAt time.Time         `bson:"created_at" json:"created_at"`   // Import timestamp for legacy data
	UpdatedAt time.Time         `bson:"updated_at" json:"updated_at"`   // Last result update
	
	// Display fields (populated by service layer for UI)
	GameDescription string `bson:"-" json:"game_description"` // "DET @ KC" 
	TeamName        string `bson:"-" json:"team_name"`        // "Detroit Lions" or "Over 45.5"
	PickDescription string `bson:"-" json:"pick_description"` // "DET @ KC - Detroit Lions (spread)"
}

// PickType represents the type of pick (spread, over/under, etc.)
type PickType string

const (
	PickTypeSpread    PickType = "spread"
	PickTypeOverUnder PickType = "over_under"
	PickTypeMoneyline PickType = "moneyline"
)

// PickResult represents the outcome of a pick
type PickResult string

const (
	PickResultPending PickResult = "pending"
	PickResultWin     PickResult = "win"
	PickResultLoss    PickResult = "loss"
	PickResultPush    PickResult = "push"
)

// IsOverUnder returns true if this pick is an over/under bet
func (p *Pick) IsOverUnder() bool {
	return p.PickType == PickTypeOverUnder
}

// IsSpreadPick returns true if this pick is a spread bet
func (p *Pick) IsSpreadPick() bool {
	return p.PickType == PickTypeSpread
}

// IsCompleted returns true if the pick has a final result
func (p *Pick) IsCompleted() bool {
	return p.Result != PickResultPending
}

// GetResultClass returns CSS class based on pick result
func (p *Pick) GetResultClass() string {
	switch p.Result {
	case PickResultWin:
		return "green-pick-class"
	case PickResultLoss:
		return "red-pick-class"
	case PickResultPush:
		return "yellow-pick-class"
	default:
		return "pick-class"
	}
}

// UserPicks represents a user's picks for a specific week
type UserPicks struct {
	UserID       int    `json:"user_id"`
	UserName     string `json:"user_name"`
	Picks        []Pick `json:"picks"`
	SpreadPicks  []Pick `json:"spread_picks"`
	OverUnderPicks []Pick `json:"over_under_picks"`
	BonusThursdayPicks []Pick `json:"bonus_thursday_picks"`
	BonusFridayPicks   []Pick `json:"bonus_friday_picks"`
	Record       UserRecord `json:"record"`
}

// UserRecord represents a user's win-loss record
type UserRecord struct {
	Wins   int `json:"wins"`
	Losses int `json:"losses"`
	Pushes int `json:"pushes"`
}

// String returns the record in "W-L-P" format
func (r *UserRecord) String() string {
	return fmt.Sprintf("%d-%d-%d", r.Wins, r.Losses, r.Pushes)
}

// GetWinPercentage calculates win percentage (pushes count as 0.5)
func (r *UserRecord) GetWinPercentage() float64 {
	total := r.Wins + r.Losses + r.Pushes
	if total == 0 {
		return 0.0
	}
	return (float64(r.Wins) + float64(r.Pushes)*0.5) / float64(total)
}

// DeterminePickTypeFromLegacyTeamID determines pick type from legacy team ID
// Legacy format: team IDs 98/99 represent over/under, actual team IDs represent spread picks
func DeterminePickTypeFromLegacyTeamID(teamID int) PickType {
	if teamID == 98 || teamID == 99 {
		return PickTypeOverUnder
	}
	return PickTypeSpread
}

// IsLegacyOverUnderPick returns true if the team ID represents an over/under pick
func IsLegacyOverUnderPick(teamID int) bool {
	return teamID == 98 || teamID == 99
}

// CreatePickFromLegacyData creates a Pick from legacy import data
// Season and week must be provided since legacy data doesn't include them
func CreatePickFromLegacyData(userID, gameID, teamID, season, week int) *Pick {
	now := time.Now()
	return &Pick{
		UserID:    userID,
		GameID:    gameID,
		TeamID:    teamID,
		PickType:  DeterminePickTypeFromLegacyTeamID(teamID),
		Season:    season, // NEW: Season tracking for multi-year storage
		Week:      week,
		Result:    PickResultPending,
		CreatedAt: now,
		UpdatedAt: now,
	}
}