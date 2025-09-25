package models

import (
	"fmt"
	"time"

	"go.mongodb.org/mongo-driver/bson/primitive"
)

// WeeklyPicks represents all of a user's picks for a specific week (new storage model)
// This replaces individual pick documents to reduce database operations and change stream events
type WeeklyPicks struct {
	ID        primitive.ObjectID `bson:"_id,omitempty" json:"id"`
	UserID    int                `bson:"user_id" json:"user_id"`
	Season    int                `bson:"season" json:"season"`
	Week      int                `bson:"week" json:"week"`
	Picks     []Pick             `bson:"picks" json:"picks"` // All picks as embedded documents
	CreatedAt time.Time          `bson:"created_at" json:"created_at"`
	UpdatedAt time.Time          `bson:"updated_at" json:"updated_at"`

	// Computed fields (calculated when needed, not stored)
	Record   UserRecord `bson:"-" json:"record"`    // Win/loss record for display
	UserName string     `bson:"-" json:"user_name"` // Populated from user service
}

// Pick represents a user's prediction for a game
// This structure is used throughout the application for compatibility
// When stored in WeeklyPicks, UserID/Season/Week are at the document level
type Pick struct {
	// Core pick data
	GameID   int        `bson:"game_id" json:"game_id"`     // ESPN game ID
	TeamID   int        `bson:"team_id" json:"team_id"`     // ESPN team ID or 98/99 for over/under
	PickType PickType   `bson:"pick_type" json:"pick_type"` // Derived from team_id
	Result   PickResult `bson:"result" json:"result"`       // Calculated after game completion

	// Context fields (populated when returned from service layer)
	UserID int `bson:"user_id,omitempty" json:"user_id"` // From WeeklyPicks.UserID
	Season int `bson:"season,omitempty" json:"season"`   // From WeeklyPicks.Season
	Week   int `bson:"week,omitempty" json:"week"`       // From WeeklyPicks.Week

	// Display fields (populated by service layer for UI, not stored in embedded documents)
	GameDescription string `bson:"-" json:"game_description"` // "DET @ KC"
	TeamName        string `bson:"-" json:"team_name"`        // "Detroit Lions" or "Over 45.5"
	PickDescription string `bson:"-" json:"pick_description"` // "DET @ KC - Detroit Lions (spread)"
}

// LEGACY: Individual pick document structure (kept for backward compatibility during transition)
// This will be removed once migration is complete
type LegacyPick struct {
	ID        primitive.ObjectID `bson:"_id,omitempty" json:"id"`
	UserID    int                `bson:"user_id" json:"user_id"`       // Maps to legacy "user" field
	GameID    int                `bson:"game_id" json:"game_id"`       // Maps to legacy "game" field (ESPN game ID)
	TeamID    int                `bson:"team_id" json:"team_id"`       // Maps to legacy "team" field
	PickType  PickType           `bson:"pick_type" json:"pick_type"`   // Derived from team_id (98/99 = over/under, others = spread)
	Season    int                `bson:"season" json:"season"`         // NEW: Season tracking for multi-year storage
	Week      int                `bson:"week" json:"week"`             // Derived from game data during import
	Result    PickResult         `bson:"result" json:"result"`         // Calculated after game completion
	CreatedAt time.Time          `bson:"created_at" json:"created_at"` // Import timestamp for legacy data
	UpdatedAt time.Time          `bson:"updated_at" json:"updated_at"` // Last result update

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

// UserPicks represents a user's picks for display (converted from WeeklyPicks)
// This is now a view/DTO that gets populated from WeeklyPicks documents
type UserPicks struct {
	UserID         int    `json:"user_id"`
	UserName       string `json:"user_name"`
	Picks          []Pick `json:"picks"`
	SpreadPicks    []Pick `json:"spread_picks"`
	OverUnderPicks []Pick `json:"over_under_picks"`
	// LEGACY: 2023-2024 bonus day logic
	BonusThursdayPicks []Pick     `json:"bonus_thursday_picks"`
	BonusFridayPicks   []Pick     `json:"bonus_friday_picks"`
	Record             UserRecord `json:"record"`
	// Pick visibility metadata
	HiddenPickCounts map[string]int `json:"hidden_pick_counts,omitempty"` // Counts of hidden picks by day
	// MODERN: 2025+ daily grouping (populated for modern seasons)
	DailyPickGroups map[string][]Pick `json:"daily_pick_groups"` // Picks grouped by Pacific date (YYYY-MM-DD)
}

// UserRecord represents a user's win-loss record
type UserRecord struct {
	Wins   int `json:"wins"`
	Losses int `json:"losses"`
	Pushes int `json:"pushes"`
	// Parlay Club scoring
	ParlayPoints int `json:"parlay_points"` // Total points earned in parlay club
	WeeklyPoints int `json:"weekly_points"` // Points earned this specific week (for display)
}

// String returns the record in parlay points format, showing weekly bonus if applicable
func (r *UserRecord) String() string {
	if r.WeeklyPoints > 0 {
		return fmt.Sprintf("%d (+%d)", r.ParlayPoints, r.WeeklyPoints)
	}
	return fmt.Sprintf("%d", r.ParlayPoints)
}

// LegacyString returns the record in "W-L-P" format (for backwards compatibility)
func (r *UserRecord) LegacyString() string {
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
	return &Pick{
		GameID:   gameID,
		TeamID:   teamID,
		PickType: DeterminePickTypeFromLegacyTeamID(teamID),
		Result:   PickResultPending,
	}
}

// WeeklyPicks helper methods

// ToIndividualPicks converts WeeklyPicks to individual Pick objects with UserID populated
// This maintains compatibility with existing code that expects Pick objects with UserID
func (wp *WeeklyPicks) ToIndividualPicks() []Pick {
	individualPicks := make([]Pick, len(wp.Picks))
	for i, pick := range wp.Picks {
		individualPicks[i] = pick
		individualPicks[i].UserID = wp.UserID // Populate UserID from document level
	}
	return individualPicks
}

// ToUserPicks converts WeeklyPicks to UserPicks for display
func (wp *WeeklyPicks) ToUserPicks() *UserPicks {
	return &UserPicks{
		UserID:   wp.UserID,
		UserName: wp.UserName,
		Picks:    wp.ToIndividualPicks(), // Use individual picks with UserID populated
		Record:   wp.Record,
	}
}

// AddPick adds a new pick to the weekly picks
func (wp *WeeklyPicks) AddPick(pick Pick) {
	wp.Picks = append(wp.Picks, pick)
	wp.UpdatedAt = time.Now()
}

// RemovePick removes a pick for a specific game
func (wp *WeeklyPicks) RemovePick(gameID int) bool {
	for i, pick := range wp.Picks {
		if pick.GameID == gameID {
			wp.Picks = append(wp.Picks[:i], wp.Picks[i+1:]...)
			wp.UpdatedAt = time.Now()
			return true
		}
	}
	return false
}

// GetPickByGame returns the pick for a specific game, if it exists
func (wp *WeeklyPicks) GetPickByGame(gameID int) (*Pick, bool) {
	for i, pick := range wp.Picks {
		if pick.GameID == gameID {
			return &wp.Picks[i], true
		}
	}
	return nil, false
}

// ReplacePicksForScheduledGames replaces picks for specific games
func (wp *WeeklyPicks) ReplacePicksForScheduledGames(newPicks []Pick, scheduledGameIDs map[int]bool) {
	// Remove existing picks for scheduled games
	var keepPicks []Pick
	for _, pick := range wp.Picks {
		if !scheduledGameIDs[pick.GameID] {
			keepPicks = append(keepPicks, pick)
		}
	}

	// Add new picks
	wp.Picks = append(keepPicks, newPicks...)
	wp.UpdatedAt = time.Now()
}

// NewWeeklyPicks creates a new WeeklyPicks document
func NewWeeklyPicks(userID, season, week int) *WeeklyPicks {
	now := time.Now()
	return &WeeklyPicks{
		UserID:    userID,
		Season:    season,
		Week:      week,
		Picks:     make([]Pick, 0),
		CreatedAt: now,
		UpdatedAt: now,
	}
}

// CalculateParlayPoints calculates parlay club points for a set of picks
// Rules:
// - Minimum 2 games required for a valid parlay
// - Any loss = 0 points
// - Pushes don't count toward minimum but don't cause failure
// - Points = number of winning picks (if 2+ games and no losses)
func CalculateParlayPoints(picks []Pick) int {
	if len(picks) == 0 {
		return 0
	}

	winningPicks := 0
	pushPicks := 0

	for _, pick := range picks {
		switch pick.Result {
		case PickResultLoss:
			return 0 // Any loss = no points
		case PickResultWin:
			winningPicks++
		case PickResultPush:
			pushPicks++
			// Pushes are excluded from count but don't cause failure
			continue
		case PickResultPending:
			// Can't calculate points for incomplete parlays
			return 0
		}
	}

	// Parlay must have at least 2 games total (wins + pushes)
	// This ensures single-game "parlays" are invalid
	totalGames := winningPicks + pushPicks
	if totalGames < 2 {
		return 0 // Invalid parlay - need minimum 2 games
	}

	// Return points equal to the number of winning picks
	// Example: 2 games (1 win, 1 push) = 1 point
	// Example: 1 game (1 win) = 0 points (invalid parlay)
	return winningPicks
}

// GetPickGameDate returns the Pacific timezone date for a pick based on its associated game
func (p *Pick) GetPickGameDate(games []Game) string {
	for _, game := range games {
		if game.ID == p.GameID {
			return game.GetGameDateInPacific()
		}
	}
	// Fallback: return empty string if game not found
	return ""
}

// GroupPicksByDay groups picks by their associated game's Pacific timezone date
func GroupPicksByDay(picks []Pick, games []Game) map[string][]Pick {
	dayGroups := make(map[string][]Pick)

	for _, pick := range picks {
		dayKey := pick.GetPickGameDate(games)
		if dayKey != "" { // Only group if we found the game
			dayGroups[dayKey] = append(dayGroups[dayKey], pick)
		}
	}

	return dayGroups
}

// GroupPicksByDayName groups picks by their associated game's Pacific timezone day name
func GroupPicksByDayName(picks []Pick, games []Game) map[string][]Pick {
	dayGroups := make(map[string][]Pick)

	for _, pick := range picks {
		for _, game := range games {
			if game.ID == pick.GameID {
				dayName := game.GetGameDayName()
				dayGroups[dayName] = append(dayGroups[dayName], pick)
				break
			}
		}
	}

	return dayGroups
}

// CalculateDailyParlayPoints calculates points for picks on a specific day (modern scoring)
// Rules: minimum 2 picks per day, any loss = 0 points, pushes don't count but don't fail
func CalculateDailyParlayPoints(picks []Pick) int {
	if len(picks) == 0 {
		return 0
	}

	winningPicks := 0
	pushPicks := 0

	for _, pick := range picks {
		switch pick.Result {
		case PickResultLoss:
			return 0 // Any loss = 0 points for the day
		case PickResultWin:
			winningPicks++
		case PickResultPush:
			pushPicks++
			// Pushes don't count toward points but don't cause failure
			continue
		case PickResultPending:
			return 0 // Can't calculate points if any pick is pending
		}
	}

	// Daily parlay must have at least 2 games total (wins + pushes)
	totalGames := winningPicks + pushPicks
	if totalGames < 2 {
		return 0 // Invalid daily parlay - need minimum 2 games
	}

	// Return points equal to the number of winning picks
	// Example: Thursday with 2 games (1 win, 1 push) = 1 point
	// Example: Sunday with 5 games (3 wins, 1 push, 1 loss) = 0 points
	return winningPicks
}

// PopulateDailyPickGroups fills the DailyPickGroups field for modern seasons (2025+)
func (up *UserPicks) PopulateDailyPickGroups(games []Game, season int) {
	if !IsModernSeason(season) {
		return // Only populate for modern seasons
	}

	// Group all picks by Pacific timezone date
	up.DailyPickGroups = GroupPicksByDay(up.Picks, games)
}

// ParlayCategory represents different parlay scoring categories
type ParlayCategory string

const (
	ParlayRegular       ParlayCategory = "regular"        // Weekend games (Sat/Sun/Mon)
	ParlayBonusThursday ParlayCategory = "bonus_thursday" // Thursday games
	ParlayBonusFriday   ParlayCategory = "bonus_friday"   // Friday games (2024+)
)

// GameDayInfo contains information about when a game is played
type GameDayInfo struct {
	GameID   int
	GameDate time.Time
	Weekday  time.Weekday
	Category ParlayCategory
}

// GetThanksgivingDate returns the date of Thanksgiving for a given year (4th Thursday of November)
func GetThanksgivingDate(year int) time.Time {
	// Start with November 1st
	nov1 := time.Date(year, time.November, 1, 0, 0, 0, 0, time.UTC)

	// Find the first Thursday of November
	daysUntilFirstThursday := (int(time.Thursday) - int(nov1.Weekday()) + 7) % 7
	firstThursday := nov1.AddDate(0, 0, daysUntilFirstThursday)

	// The 4th Thursday is 3 weeks later
	thanksgiving := firstThursday.AddDate(0, 0, 21)

	return thanksgiving
}

// GetNFLWeekForDate calculates which NFL week a given date falls into
// NFL seasons typically start the first Thursday after Labor Day (first Monday of September)
func GetNFLWeekForDate(gameDate time.Time, season int) int {
	// Estimate season start - typically the Thursday after Labor Day
	// Labor Day is first Monday of September
	sep1 := time.Date(season, time.September, 1, 0, 0, 0, 0, time.UTC)
	daysUntilFirstMonday := (int(time.Monday) - int(sep1.Weekday()) + 7) % 7
	laborDay := sep1.AddDate(0, 0, daysUntilFirstMonday)

	// NFL season usually starts the Thursday after Labor Day
	seasonStart := laborDay.AddDate(0, 0, 3) // 3 days after Monday = Thursday

	// Calculate days since season start
	daysSinceStart := int(gameDate.Sub(seasonStart).Hours() / 24)

	// NFL weeks start on Thursday, so week = (days since start / 7) + 1
	week := (daysSinceStart / 7) + 1

	// Ensure week is at least 1
	if week < 1 {
		week = 1
	}

	return week
}

// GetThanksgivingWeek dynamically calculates which NFL week Thanksgiving falls in
func GetThanksgivingWeek(season int) int {
	thanksgiving := GetThanksgivingDate(season)
	return GetNFLWeekForDate(thanksgiving, season)
}

// CategorizeGameByDate determines the parlay category based on game date, season, and week
func CategorizeGameByDate(gameDate time.Time, season, week int) ParlayCategory {
	// Convert UTC time to US Eastern timezone for proper day-of-week categorization
	// NFL games are scheduled based on US time zones, not UTC
	eastern, _ := time.LoadLocation("America/New_York")
	gameTimeEastern := gameDate.In(eastern)
	weekday := gameTimeEastern.Weekday()

	// Dynamically calculate Thanksgiving week
	thanksgivingWeek := GetThanksgivingWeek(season)

	switch weekday {
	case time.Thursday:
		// Thursday bonus weeks: Week 1 (Opening) and Thanksgiving week
		if week == 1 || week == thanksgivingWeek {
			return ParlayBonusThursday
		}
		return ParlayRegular
	case time.Friday:
		// Friday bonus weeks by season:
		// 2023: Thanksgiving Friday only
		// 2024: Thanksgiving Friday only
		// 2025+: Week 1 (Opening Friday) and Thanksgiving Friday
		if season >= 2025 && (week == 1 || week == thanksgivingWeek) {
			return ParlayBonusFriday
		} else if (season == 2023 || season == 2024) && week == thanksgivingWeek {
			return ParlayBonusFriday
		}
		return ParlayRegular
	case time.Saturday, time.Sunday, time.Monday, time.Tuesday:
		return ParlayRegular
	default:
		// Wednesday and other days default to regular
		return ParlayRegular
	}
}

// CategorizePicksByGame separates picks into parlay categories based on their game dates
func CategorizePicksByGame(picks []Pick, gameInfoMap map[int]GameDayInfo) map[ParlayCategory][]Pick {
	categories := map[ParlayCategory][]Pick{
		ParlayRegular:       {},
		ParlayBonusThursday: {},
		ParlayBonusFriday:   {},
	}

	for _, pick := range picks {
		gameInfo, exists := gameInfoMap[pick.GameID]
		if !exists {
			// If we don't have game info, default to regular
			categories[ParlayRegular] = append(categories[ParlayRegular], pick)
			continue
		}

		categories[gameInfo.Category] = append(categories[gameInfo.Category], pick)
	}

	return categories
}
