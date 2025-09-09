package main

// This test file should be run separately: go run test_legacy_behavior.go
// It tests the legacy behavior for 2023-2024 seasons

import (
	"fmt"
	"log"
	"nfl-app-go/models"
	"time"
)

func main() {
	fmt.Println("=== LEGACY BEHAVIOR TEST (2023-2024 Seasons) ===")
	
	// Test 1: Season Detection
	fmt.Println("\n1. Testing Season Detection:")
	testSeasonDetection()
	
	// Test 2: Game Grouping
	fmt.Println("\n2. Testing Game Grouping:")
	testGameGrouping()
	
	// Test 3: Pick Grouping
	fmt.Println("\n3. Testing Pick Grouping:")
	testPickGrouping()
	
	// Test 4: Legacy Scoring Logic
	fmt.Println("\n4. Testing Legacy Scoring:")
	testLegacyScoring()
	
	// Test 5: Pacific Timezone Handling
	fmt.Println("\n5. Testing Pacific Timezone:")
	testTimezoneHandling()
	
	fmt.Println("\n=== LEGACY BEHAVIOR TEST COMPLETE ===")
}

func testSeasonDetection() {
	testCases := []struct {
		season   int
		expected bool
		desc     string
	}{
		{2023, false, "2023 should be legacy"},
		{2024, false, "2024 should be legacy"},
		{2025, true, "2025 should be modern"},
		{2026, true, "2026 should be modern"},
	}
	
	for _, tc := range testCases {
		result := models.IsModernSeason(tc.season)
		status := "✅"
		if result != tc.expected {
			status = "❌"
		}
		fmt.Printf("  %s Season %d: %s (got: %v, expected: %v)\n", 
			status, tc.season, tc.desc, result, tc.expected)
	}
}

func testGameGrouping() {
	// Create test games for different days
	games := []models.Game{
		createTestGame(1, "2024-09-05T20:00:00Z", "TNF", "Thursday Night Football"),
		createTestGame(2, "2024-09-06T23:00:00Z", "Amazon", "Friday Night Football"),
		createTestGame(3, "2024-09-08T17:00:00Z", "CBS", "Sunday Early"),
		createTestGame(4, "2024-09-08T20:00:00Z", "NBC", "Sunday Night"),
		createTestGame(5, "2024-09-09T20:00:00Z", "ESPN", "Monday Night"),
	}
	
	fmt.Println("  Testing Pacific timezone game day detection:")
	for _, game := range games {
		dayName := game.GetGameDayName()
		dateInPacific := game.GetGameDateInPacific()
		fmt.Printf("    Game %d: %s -> Day: %s, Date: %s\n", 
			game.ID, game.Date.Format("2006-01-02 15:04 MST"), dayName, dateInPacific)
	}
	
	// Test grouping by day
	dayGroups := models.GroupGamesByDayName(games)
	fmt.Printf("  Games grouped by day: %d groups\n", len(dayGroups))
	for dayName, dayGames := range dayGroups {
		fmt.Printf("    %s: %d games\n", dayName, len(dayGames))
	}
}

func testPickGrouping() {
	// Create test games
	games := []models.Game{
		createTestGame(1, "2024-09-05T20:00:00Z", "TNF", "Thursday"),
		createTestGame(2, "2024-09-08T17:00:00Z", "CBS", "Sunday"),
		createTestGame(3, "2024-09-09T20:00:00Z", "ESPN", "Monday"),
	}
	
	// Create test picks
	picks := []models.Pick{
		{GameID: 1, PickType: models.PickTypeSpread, Result: models.PickResultWin},
		{GameID: 1, PickType: models.PickTypeOverUnder, Result: models.PickResultWin},
		{GameID: 2, PickType: models.PickTypeSpread, Result: models.PickResultWin},
		{GameID: 3, PickType: models.PickTypeSpread, Result: models.PickResultLoss},
	}
	
	// Test grouping picks by day
	pickGroups := models.GroupPicksByDay(picks, games)
	fmt.Printf("  Picks grouped by day: %d groups\n", len(pickGroups))
	for date, dayPicks := range pickGroups {
		fmt.Printf("    %s: %d picks\n", date, len(dayPicks))
	}
}

func testLegacyScoring() {
	fmt.Println("  Testing legacy weekly parlay scoring:")
	
	// Test Case 1: All wins (should get points)
	allWinsPicks := []models.Pick{
		{Result: models.PickResultWin},
		{Result: models.PickResultWin},
		{Result: models.PickResultWin},
	}
	points1 := models.CalculateParlayPoints(allWinsPicks)
	status1 := "✅"
	if points1 != 3 {
		status1 = "❌"
	}
	fmt.Printf("    %s All wins (3 picks): %d points (expected: 3)\n", status1, points1)
	
	// Test Case 2: One loss (should get 0 points)
	oneLossPicks := []models.Pick{
		{Result: models.PickResultWin},
		{Result: models.PickResultWin},
		{Result: models.PickResultLoss},
	}
	points2 := models.CalculateParlayPoints(oneLossPicks)
	status2 := "✅"
	if points2 != 0 {
		status2 = "❌"
	}
	fmt.Printf("    %s One loss (3 picks): %d points (expected: 0)\n", status2, points2)
	
	// Test Case 3: Push handling
	pushPicks := []models.Pick{
		{Result: models.PickResultWin},
		{Result: models.PickResultWin},
		{Result: models.PickResultPush},
	}
	points3 := models.CalculateParlayPoints(pushPicks)
	status3 := "✅"
	if points3 != 2 {
		status3 = "❌"
	}
	fmt.Printf("    %s Push handling (2W, 1P): %d points (expected: 2)\n", status3, points3)
	
	// Test modern daily scoring for comparison
	fmt.Println("  Testing modern daily parlay scoring:")
	dailyPoints := models.CalculateDailyParlayPoints(allWinsPicks)
	status4 := "✅"
	if dailyPoints != 3 {
		status4 = "❌"
	}
	fmt.Printf("    %s Daily scoring (3 wins): %d points (expected: 3)\n", status4, dailyPoints)
}

func testTimezoneHandling() {
	// Test UTC game time conversion to Pacific
	utcTime := time.Date(2024, 9, 8, 3, 0, 0, 0, time.UTC) // 3 AM UTC = 8 PM PDT previous day
	game := models.Game{
		ID:   1,
		Date: utcTime,
	}
	
	pacificDate := game.GetGameDateInPacific()
	dayName := game.GetGameDayName()
	
	fmt.Printf("  UTC Time: %s\n", utcTime.Format("2006-01-02 15:04:05 MST"))
	fmt.Printf("  Pacific Date: %s\n", pacificDate)
	fmt.Printf("  Day Name: %s\n", dayName)
	
	// Verify it correctly shifts to previous day in Pacific
	expectedDate := "2024-09-07" // Should be September 7th in Pacific
	status := "✅"
	if pacificDate != expectedDate {
		status = "❌"
	}
	fmt.Printf("  %s Timezone conversion (expected: %s, got: %s)\n", status, expectedDate, pacificDate)
}

func createTestGame(id int, dateStr, network, desc string) models.Game {
	gameTime, err := time.Parse("2006-01-02T15:04:05Z", dateStr)
	if err != nil {
		log.Printf("Error parsing time %s: %v", dateStr, err)
		gameTime = time.Now()
	}
	
	return models.Game{
		ID:     id,
		Date:   gameTime,
		Week:   1,
		Season: 2024,
		Away:   "AWAY",
		Home:   "HOME",
		State:  models.GameStateScheduled,
	}
}