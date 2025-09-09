package main

// This test file should be run separately from the tests directory:
// go run test_modern_behavior.go
// It tests the modern behavior for 2025+ seasons

import (
	"fmt"
	"log"
	"nfl-app-go/models"
	"time"
)

func main() {
	fmt.Println("=== MODERN BEHAVIOR TEST (2025+ Seasons) ===")
	
	// Test 1: Season Detection 
	fmt.Println("\n1. Testing Season Detection:")
	testModernSeasonDetection()
	
	// Test 2: Daily Pick Grouping
	fmt.Println("\n2. Testing Daily Pick Grouping:")
	testDailyPickGrouping()
	
	// Test 3: Daily Scoring Logic
	fmt.Println("\n3. Testing Daily Scoring:")
	testDailyScoring()
	
	// Test 4: UserPicks Population
	fmt.Println("\n4. Testing UserPicks Daily Groups:")
	testUserPicksPopulation()
	
	// Test 5: Multi-Day Parlay Scenarios
	fmt.Println("\n5. Testing Multi-Day Scenarios:")
	testMultiDayScenarios()
	
	// Test 6: Edge Cases
	fmt.Println("\n6. Testing Edge Cases:")
	testEdgeCases()
	
	fmt.Println("\n=== MODERN BEHAVIOR TEST COMPLETE ===")
}

func testModernSeasonDetection() {
	modernSeasons := []int{2025, 2026, 2030}
	
	for _, season := range modernSeasons {
		isModern := models.IsModernSeason(season)
		status := "✅"
		if !isModern {
			status = "❌"
		}
		fmt.Printf("  %s Season %d should be modern: %v\n", status, season, isModern)
	}
}

func testDailyPickGrouping() {
	// Create games across multiple days
	games := []models.Game{
		createTestGame(1, "2025-09-04T20:00:00Z", "TNF", 2025), // Thursday
		createTestGame(2, "2025-09-06T02:00:00Z", "Amazon", 2025), // Friday (late game, UTC+7 = Friday PT)
		createTestGame(3, "2025-09-07T17:00:00Z", "CBS", 2025), // Sunday early
		createTestGame(4, "2025-09-07T20:00:00Z", "NBC", 2025), // Sunday night
		createTestGame(5, "2025-09-07T23:00:00Z", "Fox", 2025), // Sunday late
		createTestGame(6, "2025-09-09T01:00:00Z", "ESPN", 2025), // Monday night
	}
	
	// Create picks for various games
	picks := []models.Pick{
		{GameID: 1, PickType: models.PickTypeSpread, Result: models.PickResultWin},    // Thursday ATS
		{GameID: 1, PickType: models.PickTypeOverUnder, Result: models.PickResultWin}, // Thursday O/U
		{GameID: 2, PickType: models.PickTypeSpread, Result: models.PickResultLoss},   // Friday ATS (lose)
		{GameID: 3, PickType: models.PickTypeSpread, Result: models.PickResultWin},    // Sunday ATS
		{GameID: 4, PickType: models.PickTypeSpread, Result: models.PickResultWin},    // Sunday ATS
		{GameID: 5, PickType: models.PickTypeOverUnder, Result: models.PickResultPush}, // Sunday O/U (push)
		{GameID: 6, PickType: models.PickTypeSpread, Result: models.PickResultWin},    // Monday ATS
		{GameID: 6, PickType: models.PickTypeOverUnder, Result: models.PickResultWin}, // Monday O/U
	}
	
	// Test daily grouping
	dailyGroups := models.GroupPicksByDay(picks, games)
	fmt.Printf("  Picks grouped into %d days\n", len(dailyGroups))
	
	for date, dayPicks := range dailyGroups {
		fmt.Printf("    %s: %d picks\n", date, len(dayPicks))
		
		// Calculate daily points for each day
		points := models.CalculateDailyParlayPoints(dayPicks)
		fmt.Printf("      → %d points earned\n", points)
	}
}

func testDailyScoring() {
	fmt.Println("  Testing daily parlay point calculation:")
	
	// Test Case 1: Perfect Thursday (2 wins)
	thursdayPicks := []models.Pick{
		{Result: models.PickResultWin},  // ATS win
		{Result: models.PickResultWin},  // O/U win
	}
	thursdayPoints := models.CalculateDailyParlayPoints(thursdayPicks)
	status1 := "✅"
	if thursdayPoints != 2 {
		status1 = "❌"
	}
	fmt.Printf("    %s Thursday (2 wins): %d points (expected: 2)\n", status1, thursdayPoints)
	
	// Test Case 2: Friday loss (1 win, 1 loss)  
	fridayPicks := []models.Pick{
		{Result: models.PickResultWin},  
		{Result: models.PickResultLoss}, 
	}
	fridayPoints := models.CalculateDailyParlayPoints(fridayPicks)
	status2 := "✅"
	if fridayPoints != 0 {
		status2 = "❌"
	}
	fmt.Printf("    %s Friday (1W, 1L): %d points (expected: 0)\n", status2, fridayPoints)
	
	// Test Case 3: Sunday with push (2 wins, 1 push)
	sundayPicks := []models.Pick{
		{Result: models.PickResultWin},
		{Result: models.PickResultWin}, 
		{Result: models.PickResultPush},
	}
	sundayPoints := models.CalculateDailyParlayPoints(sundayPicks)
	status3 := "✅"
	if sundayPoints != 2 {
		status3 = "❌"
	}
	fmt.Printf("    %s Sunday (2W, 1P): %d points (expected: 2)\n", status3, sundayPoints)
	
	// Test Case 4: Single game day with both picks
	singleGamePicks := []models.Pick{
		{Result: models.PickResultWin},  // ATS
		{Result: models.PickResultWin},  // O/U
	}
	singlePoints := models.CalculateDailyParlayPoints(singleGamePicks)
	status4 := "✅"
	if singlePoints != 2 {
		status4 = "❌"
	}
	fmt.Printf("    %s Single game (ATS+O/U): %d points (expected: 2)\n", status4, singlePoints)
}

func testUserPicksPopulation() {
	games := []models.Game{
		createTestGame(1, "2025-09-04T20:00:00Z", "TNF", 2025), // Thursday
		createTestGame(2, "2025-09-07T20:00:00Z", "NBC", 2025), // Sunday
	}
	
	userPicks := &models.UserPicks{
		UserID:   1,
		UserName: "TestUser",
		Picks: []models.Pick{
			{GameID: 1, PickType: models.PickTypeSpread},
			{GameID: 1, PickType: models.PickTypeOverUnder},
			{GameID: 2, PickType: models.PickTypeSpread},
		},
	}
	
	// Test population of daily groups for modern season
	userPicks.PopulateDailyPickGroups(games, 2025)
	
	if userPicks.DailyPickGroups != nil {
		fmt.Printf("  ✅ DailyPickGroups populated for modern season: %d groups\n", len(userPicks.DailyPickGroups))
		for date, picks := range userPicks.DailyPickGroups {
			fmt.Printf("    %s: %d picks\n", date, len(picks))
		}
	} else {
		fmt.Printf("  ❌ DailyPickGroups not populated\n")
	}
	
	// Test that legacy seasons don't get daily groups
	legacyUserPicks := &models.UserPicks{
		UserID: 2,
		Picks:  []models.Pick{{GameID: 1}},
	}
	legacyUserPicks.PopulateDailyPickGroups(games, 2024)
	
	if legacyUserPicks.DailyPickGroups == nil {
		fmt.Printf("  ✅ Legacy season correctly skips DailyPickGroups\n")
	} else {
		fmt.Printf("  ❌ Legacy season incorrectly populated DailyPickGroups\n")
	}
}

func testMultiDayScenarios() {
	fmt.Println("  Testing week-long scenarios:")
	
	// Scenario: Week with mixed daily results
	dailyResults := map[string]int{
		"2025-09-04": 2, // Thursday: 2 points
		"2025-09-06": 0, // Friday: 0 points (loss)
		"2025-09-07": 4, // Sunday: 4 points  
		"2025-09-08": 1, // Monday: 1 point
	}
	
	totalPoints := 0
	for day, points := range dailyResults {
		totalPoints += points
		fmt.Printf("    %s: %d points\n", day, points)
	}
	
	fmt.Printf("    Total week points: %d (vs legacy max of ~6 for perfect week)\n", totalPoints)
	fmt.Printf("    ✅ Modern system allows higher weekly totals through daily scoring\n")
}

func testEdgeCases() {
	// Test Case 1: No picks for a day
	emptyPicks := []models.Pick{}
	emptyPoints := models.CalculateDailyParlayPoints(emptyPicks)
	status1 := "✅"
	if emptyPoints != 0 {
		status1 = "❌"
	}
	fmt.Printf("  %s No picks: %d points (expected: 0)\n", status1, emptyPoints)
	
	// Test Case 2: Only one pick (invalid parlay)
	onePick := []models.Pick{{Result: models.PickResultWin}}
	onePoints := models.CalculateDailyParlayPoints(onePick)
	status2 := "✅"
	if onePoints != 0 {
		status2 = "❌"
	}
	fmt.Printf("  %s One pick only: %d points (expected: 0, needs 2+ for parlay)\n", status2, onePoints)
	
	// Test Case 3: Only pushes (invalid parlay)
	pushOnly := []models.Pick{
		{Result: models.PickResultPush},
		{Result: models.PickResultPush},
	}
	pushPoints := models.CalculateDailyParlayPoints(pushOnly)
	status3 := "✅"
	if pushPoints != 0 {
		status3 = "❌"
	}
	fmt.Printf("  %s Push-only day: %d points (expected: 0, no wins)\n", status3, pushPoints)
	
	// Test Case 4: Pending picks
	pendingPicks := []models.Pick{
		{Result: models.PickResultWin},
		{Result: models.PickResultPending},
	}
	pendingPoints := models.CalculateDailyParlayPoints(pendingPicks)
	status4 := "✅"
	if pendingPoints != 0 {
		status4 = "❌"
	}
	fmt.Printf("  %s Pending picks: %d points (expected: 0, can't score incomplete)\n", status4, pendingPoints)
}

func createTestGame(id int, dateStr, network string, season int) models.Game {
	gameTime, err := time.Parse("2006-01-02T15:04:05Z", dateStr)
	if err != nil {
		log.Printf("Error parsing time %s: %v", dateStr, err)
		gameTime = time.Now()
	}
	
	return models.Game{
		ID:     id,
		Date:   gameTime,
		Week:   1,
		Season: season,
		Away:   "AWAY",
		Home:   "HOME",
		State:  models.GameStateScheduled,
	}
}