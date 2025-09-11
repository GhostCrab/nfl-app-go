package models

import (
	"fmt"
	"math/rand"
	"time"
)

// PickVisibility manages when picks become visible to other users
type PickVisibility struct {
	GameID         int          `json:"game_id"`
	GameDate       time.Time    `json:"game_date"`
	GameState      GameState    `json:"game_state"`
	Weekday        time.Weekday `json:"weekday"`
	IsThanksgiving bool         `json:"is_thanksgiving"`
	VisibleAt      time.Time    `json:"visible_at"`
	IsVisible      bool         `json:"is_visible"`
	VisibilityRule string       `json:"visibility_rule"`
}

// VisibilityRule constants
const (
	VisibilityRuleThursday5PM      = "thursday_5pm_pt"
	VisibilityRuleThanksgiving10AM = "thanksgiving_10am_pt"
	VisibilityRuleWeekend10AM      = "weekend_10am_pt"
	VisibilityRuleGameInProgress   = "game_in_progress"
	VisibilityRuleGameCompleted    = "game_completed"
	VisibilityRuleAlwaysVisible    = "always_visible"
)

// PickVisibilityService calculates when picks should become visible
type PickVisibilityService struct {
	debugDateTime *time.Time // For testing - overrides current time
}

// NewPickVisibilityService creates a new visibility service
func NewPickVisibilityService() *PickVisibilityService {
	return &PickVisibilityService{}
}

// SetDebugDateTime sets a fake current time for testing
func (s *PickVisibilityService) SetDebugDateTime(debugTime time.Time) {
	s.debugDateTime = &debugTime
}

// ClearDebugDateTime removes debug time override
func (s *PickVisibilityService) ClearDebugDateTime() {
	s.debugDateTime = nil
}

// GetCurrentTime returns debug time if set, otherwise actual current time in Pacific timezone
func (s *PickVisibilityService) GetCurrentTime() time.Time {
	if s.debugDateTime != nil {
		return *s.debugDateTime
	}

	// Always return time in Pacific timezone to match game times
	loc, err := time.LoadLocation("America/Los_Angeles")
	if err != nil {
		// Fallback to manual UTC offset if timezone loading fails
		return time.Now().Add(-8 * time.Hour) // Use PST as default
	}

	return time.Now().In(loc)
}

// generateInProgressGameState creates realistic in-progress game data for debug mode
func (s *PickVisibilityService) generateInProgressGameState(game Game, debugTime time.Time) Game {
	// Create a copy of the game to modify
	demoGame := game

	// Calculate how long the game has been "running"
	gameStartTime := game.PacificTime()
	elapsedMinutes := int(debugTime.Sub(gameStartTime).Minutes())

	// Simulate game progression (NFL games are ~180 minutes, 4 quarters of 15 minutes each)
	if elapsedMinutes > 180 {
		// Game should be completed
		demoGame.State = GameStateCompleted
		// Generate final scores (14-35 points typical range)
		demoGame.AwayScore = 14 + rand.Intn(22) // 14-35
		demoGame.HomeScore = 14 + rand.Intn(22) // 14-35
	} else {
		// Game is in progress
		demoGame.State = GameStateInPlay

		// Calculate quarter and clock time
		quarter := (elapsedMinutes / 15) + 1
		if quarter > 4 {
			quarter = 4 // Keep it in 4th quarter for overtime scenarios
		}

		minutesInQuarter := elapsedMinutes % 15
		clockMinutes := 15 - minutesInQuarter
		clockSeconds := rand.Intn(60)

		// Generate realistic scores based on game progress
		scoringFactor := float64(elapsedMinutes) / 180.0
		baseScore := int(scoringFactor * 24) // Scale up to ~24 points max

		demoGame.AwayScore = baseScore + rand.Intn(7) // Add some randomness
		demoGame.HomeScore = baseScore + rand.Intn(7)

		// Create realistic game status
		demoGame.Status = &GameStatus{
			DisplayClock: fmt.Sprintf("%d:%02d", clockMinutes, clockSeconds),
			HomeTimeouts: 3 - rand.Intn(3), // 0-3 timeouts remaining
			AwayTimeouts: 3 - rand.Intn(3),
			IsRedZone:    rand.Float32() < 0.2, // 20% chance of being in red zone
		}

		// Add possession info sometimes
		if rand.Float32() < 0.7 { // 70% chance of having possession info
			if rand.Float32() < 0.5 {
				demoGame.Status.Possession = demoGame.Away
			} else {
				demoGame.Status.Possession = demoGame.Home
			}

			// Add down and distance info
			demoGame.Status.Down = rand.Intn(4) + 1       // 1st-4th down
			demoGame.Status.Distance = rand.Intn(15) + 1  // 1-15 yards
			demoGame.Status.YardLine = rand.Intn(80) + 10 // 10-89 yard line
			demoGame.Status.ShortDownDistanceText = fmt.Sprintf("%s & %d",
				[]string{"1st", "2nd", "3rd", "4th"}[demoGame.Status.Down-1],
				demoGame.Status.Distance)
		}
	}

	return demoGame
}

// CalculateVisibility determines when a game's picks become visible
func (s *PickVisibilityService) CalculateVisibility(game Game) PickVisibility {
	return s.CalculateVisibilityWithSeason(game, game.Season)
}

// CalculateVisibilityWithSeason determines when a game's picks become visible with season context
func (s *PickVisibilityService) CalculateVisibilityWithSeason(game Game, season int) PickVisibility {
	// Convert game time to Pacific timezone
	pacificTime := game.PacificTime()
	gameDate := pacificTime
	weekday := gameDate.Weekday()

	// Check if it's Thanksgiving week
	thanksgivingDate := GetThanksgivingDate(game.Season)
	isThanksgiving := isSameWeek(gameDate, thanksgivingDate) && weekday == time.Thursday

	var visibleAt time.Time
	var rule string

	// Calculate effective game state based on debug time (if active) or actual state
	currentTime := s.GetCurrentTime()
	effectiveGame := game

	// If debug time is active, generate realistic in-progress game state
	if s.debugDateTime != nil {
		gameStartTime := gameDate

		// Only generate in-progress state if game has actually started
		if (currentTime.After(gameStartTime) || currentTime.Equal(gameStartTime)) && currentTime.Before(gameStartTime.Add(4*time.Hour)) {
			effectiveGame = s.generateInProgressGameState(game, currentTime)
		}
	}

	effectiveState := effectiveGame.State

	// Determine visibility rule based on effective game state and special circumstances
	switch {
	case effectiveState == GameStateInPlay:
		// In-progress games are always visible
		visibleAt = gameDate.Add(-24 * time.Hour) // Set to past time
		rule = VisibilityRuleGameInProgress

	case effectiveState == GameStateCompleted:
		// Completed games are always visible
		visibleAt = gameDate.Add(-24 * time.Hour) // Set to past time
		rule = VisibilityRuleGameCompleted

	case weekday == time.Thursday && isThanksgiving:
		// Thanksgiving Thursday games visible at 10:00 AM PT
		visibleAt = time.Date(gameDate.Year(), gameDate.Month(), gameDate.Day(), 10, 0, 0, 0, gameDate.Location())
		rule = VisibilityRuleThanksgiving10AM

	case weekday == time.Thursday:
		// Regular Thursday games visible at 5:00 PM PT
		visibleAt = time.Date(gameDate.Year(), gameDate.Month(), gameDate.Day(), 17, 0, 0, 0, gameDate.Location())
		rule = VisibilityRuleThursday5PM

	case weekday == time.Friday:
		// Friday games become visible at 10:00 AM PT on Saturday
		saturday := gameDate.AddDate(0, 0, 1) // Next day (Saturday)
		visibleAt = time.Date(saturday.Year(), saturday.Month(), saturday.Day(), 10, 0, 0, 0, gameDate.Location())
		rule = VisibilityRuleWeekend10AM

	case weekday == time.Saturday:
		// Saturday games become visible at 10:00 AM PT on Saturday
		visibleAt = time.Date(gameDate.Year(), gameDate.Month(), gameDate.Day(), 10, 0, 0, 0, gameDate.Location())
		rule = VisibilityRuleWeekend10AM

	case weekday == time.Sunday:
		// Sunday games become visible at 10:00 AM PT on Sunday (same day)
		visibleAt = time.Date(gameDate.Year(), gameDate.Month(), gameDate.Day(), 10, 0, 0, 0, gameDate.Location())
		rule = VisibilityRuleWeekend10AM

	case weekday == time.Monday:
		if IsModernSeason(season) {
			// Modern seasons (2025+): Monday games become visible when the first Monday game kicks off
			visibleAt = gameDate
			rule = VisibilityRuleWeekend10AM // Could add new rule like "monday_kickoff" but reusing existing
		} else {
			// Legacy seasons: Monday games become visible at 10:00 AM PT on the Sunday before
			sunday := gameDate.AddDate(0, 0, -1) // Previous day (Sunday)
			visibleAt = time.Date(sunday.Year(), sunday.Month(), sunday.Day(), 10, 0, 0, 0, gameDate.Location())
			rule = VisibilityRuleWeekend10AM
		}

	default:
		// Default: games become visible at 10:00 AM PT on Saturday before
		saturday := getLastSaturday(gameDate)
		visibleAt = time.Date(saturday.Year(), saturday.Month(), saturday.Day(), 10, 0, 0, 0, gameDate.Location())
		rule = VisibilityRuleWeekend10AM
	}

	// Check if picks are currently visible
	isVisible := currentTime.After(visibleAt) || currentTime.Equal(visibleAt)

	return PickVisibility{
		GameID:         game.ID,
		GameDate:       gameDate,
		GameState:      game.State,
		Weekday:        weekday,
		IsThanksgiving: isThanksgiving,
		VisibleAt:      visibleAt,
		IsVisible:      isVisible,
		VisibilityRule: rule,
	}
}

// IsPickVisibleToUser determines if a pick should be visible to a specific user
func (s *PickVisibilityService) IsPickVisibleToUser(pick Pick, game Game, viewingUserID int) bool {
	// User's own picks are always visible
	if pick.UserID == viewingUserID {
		return true
	}

	// Calculate visibility for this game
	visibility := s.CalculateVisibility(game)
	return visibility.IsVisible
}

// GetVisiblePicksForUser filters picks based on what the user should see
func (s *PickVisibilityService) GetVisiblePicksForUser(picks []Pick, games []Game, viewingUserID int) []Pick {
	// Create game lookup map
	gameMap := make(map[int]Game)
	for _, game := range games {
		gameMap[game.ID] = game
	}

	var visiblePicks []Pick
	for _, pick := range picks {
		if game, exists := gameMap[pick.GameID]; exists {
			if s.IsPickVisibleToUser(pick, game, viewingUserID) {
				visiblePicks = append(visiblePicks, pick)
			}
		}
	}

	return visiblePicks
}

// GetHiddenPickCounts returns counts of hidden picks grouped by day
func (s *PickVisibilityService) GetHiddenPickCounts(picks []Pick, games []Game, viewingUserID int) map[string]int {
	// Create game lookup map
	gameMap := make(map[int]Game)
	for _, game := range games {
		gameMap[game.ID] = game
	}

	// Determine if this is a modern season by checking the first game
	isModern := false
	if len(games) > 0 {
		isModern = IsModernSeason(games[0].Season)
	}

	var counts map[string]int
	if isModern {
		// Modern seasons: separate Sunday and Monday
		counts = map[string]int{
			"Thursday": 0,
			"Friday":   0,
			"Saturday": 0,
			"Sunday":   0,
			"Monday":   0,
		}
	} else {
		// Legacy seasons: group Sunday/Monday
		counts = map[string]int{
			"Thursday":      0,
			"Friday":        0,
			"Saturday":      0,
			"Sunday/Monday": 0,
		}
	}

	for _, pick := range picks {
		// Skip user's own picks
		if pick.UserID == viewingUserID {
			continue
		}

		if game, exists := gameMap[pick.GameID]; exists {
			visibility := s.CalculateVisibilityWithSeason(game, game.Season)
			if !visibility.IsVisible {
				// Count hidden pick by day
				if isModern {
					// Modern seasons: separate counting
					switch visibility.Weekday {
					case time.Thursday:
						counts["Thursday"]++
					case time.Friday:
						counts["Friday"]++
					case time.Saturday:
						counts["Saturday"]++
					case time.Sunday:
						counts["Sunday"]++
					case time.Monday:
						counts["Monday"]++
					}
				} else {
					// Legacy seasons: grouped counting
					switch visibility.Weekday {
					case time.Thursday:
						counts["Thursday"]++
					case time.Friday:
						counts["Friday"]++
					case time.Saturday:
						counts["Saturday"]++
					case time.Sunday, time.Monday:
						counts["Sunday/Monday"]++
					}
				}
			}
		}
	}

	return counts
}

// Helper functions
func isSameWeek(date1, date2 time.Time) bool {
	year1, week1 := date1.ISOWeek()
	year2, week2 := date2.ISOWeek()
	return year1 == year2 && week1 == week2
}

func getLastSaturday(date time.Time) time.Time {
	daysUntilSaturday := int(date.Weekday() - time.Saturday)
	if daysUntilSaturday <= 0 {
		daysUntilSaturday += 7
	}
	return date.AddDate(0, 0, -daysUntilSaturday)
}
