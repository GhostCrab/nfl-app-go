package services

import (
	"context"
	"fmt"
	"log"
	"nfl-app-go/database"
	"nfl-app-go/models"
	"time"
)

// WeekCategory represents a specific parlay category within a week
type WeekCategory struct {
	Week     int
	Category models.ParlayCategory
}

// BackgroundUpdater handles automatic ESPN API polling and game updates
type BackgroundUpdater struct {
	espnService         *ESPNService
	gameRepo            *database.MongoGameRepository
	pickService         *PickService
	parlayService       *ParlayService // New specialized service for parlay operations
	currentSeason       int
	ticker              *time.Ticker
	stopChan            chan bool
	running             bool
	lastUpdateType      string                // Track what type of update we last do
	lastOddsUpdate      time.Time             // Track when we last fetched odds
	processedWeeks      map[int]bool          // Track which weeks have already been scored
	processedCategories map[WeekCategory]bool // Track which categories have already been scored
}

// NewBackgroundUpdater creates a new background updater service
func NewBackgroundUpdater(espnService *ESPNService, gameRepo *database.MongoGameRepository, pickService *PickService, parlayService *ParlayService, currentSeason int) *BackgroundUpdater {
	return &BackgroundUpdater{
		espnService:         espnService,
		gameRepo:            gameRepo,
		pickService:         pickService,
		parlayService:       parlayService,
		currentSeason:       currentSeason,
		stopChan:            make(chan bool),
		running:             false,
		processedWeeks:      make(map[int]bool),
		processedCategories: make(map[WeekCategory]bool),
	}
}

// Start begins the background updating process
func (bu *BackgroundUpdater) Start() {
	if bu.running {
		log.Println("BackgroundUpdater: Already running")
		return
	}

	log.Println("BackgroundUpdater: Starting intelligent background ESPN API polling")
	bu.running = true

	// Do an initial update
	go bu.updateGames()

	// Start the intelligent scheduler
	go bu.intelligentScheduler()
}

// Stop halts the background updating process
func (bu *BackgroundUpdater) Stop() {
	if !bu.running {
		return
	}

	log.Println("BackgroundUpdater: Stopping...")
	bu.running = false

	if bu.ticker != nil {
		bu.ticker.Stop()
	}

	close(bu.stopChan)
}

// updateGames fetches latest data from ESPN and updates the database
func (bu *BackgroundUpdater) updateGames() {
	ctx := context.Background()
	startTime := time.Now()

	log.Printf("BackgroundUpdater: Starting ESPN API update for season %d", bu.currentSeason)

	// Fetch games from ESPN
	games, err := bu.espnService.GetScoreboardForYear(bu.currentSeason)
	if err != nil {
		log.Printf("BackgroundUpdater: Failed to fetch ESPN data: %v", err)
		return
	}

	if len(games) == 0 {
		log.Printf("BackgroundUpdater: No games received from ESPN API")
		return
	}

	// Note: Scoreboard API doesn't include odds data, so don't try to enrich here
	// Odds enrichment will be handled separately after database update

	// Get existing games from database for comparison
	existingGames, err := bu.gameRepo.GetGamesBySeason(bu.currentSeason)
	if err != nil {
		log.Printf("BackgroundUpdater: Failed to get existing games: %v", err)
		return
	}

	// Create map of existing games for quick lookup
	existingGameMap := make(map[int]models.Game)
	for _, game := range existingGames {
		existingGameMap[game.ID] = *game
	}

	// Always write all games to database - let MongoDB change streams detect actual changes
	var gamesToUpdate []*models.Game

	// Track state changes and preserve existing odds data
	var stateChanges []string
	for i := range games {
		if existing, exists := existingGameMap[games[i].ID]; exists {
			// Preserve odds data
			if existing.Odds != nil {
				games[i].Odds = existing.Odds
			}

			// Track state transitions for debugging
			if existing.State != games[i].State {
				stateChanges = append(stateChanges, fmt.Sprintf("Game %d (%s vs %s): %s → %s",
					games[i].ID, games[i].Away, games[i].Home, existing.State, games[i].State))
			}
		}

		// Removed verbose live game logging for cleaner monitoring

		gamesToUpdate = append(gamesToUpdate, &games[i])
	}

	// Log state changes to help debug game transitions
	if len(stateChanges) > 0 {
		log.Printf("BackgroundUpdater: Game state changes detected:")
		for _, change := range stateChanges {
			log.Printf("  %s", change)
		}
	}

	// Write all games to database - MongoDB change stream will detect actual document changes
	// Removed verbose "Writing X games to database" log for cleaner monitoring
	err = bu.gameRepo.BulkUpsertGames(gamesToUpdate)
	if err != nil {
		log.Printf("BackgroundUpdater: Failed to update games in database: %v", err)
		return
	}

	// Check for completed parlay categories and trigger immediate scoring
	completedCategories := bu.checkForCompletedParlayCategories(games)

	// Process parlay scoring for completed categories
	for weekCategory := range completedCategories {
		// Skip if this category has already been processed
		if bu.processedCategories[weekCategory] {
			continue
		}

		if !models.IsModernSeason(bu.currentSeason) {
			// LEGACY: 2023-2024 use category-based scoring
			log.Printf("BackgroundUpdater: LEGACY parlay category completed for Season %d Week %d Category %s, triggering immediate scoring",
				bu.currentSeason, weekCategory.Week, weekCategory.Category)
			if err := bu.parlayService.ProcessParlayCategory(ctx, bu.currentSeason, weekCategory.Week, weekCategory.Category); err != nil {
				log.Printf("BackgroundUpdater: Failed to process LEGACY parlay scoring for Season %d Week %d Category %s: %v",
					bu.currentSeason, weekCategory.Week, weekCategory.Category, err)
			} else {
				// Mark category as processed
				bu.processedCategories[weekCategory] = true
			}
		} else {
			// MODERN: 2025+ use daily parlay scoring for completed days
			dayName := bu.categoryToDayName(weekCategory.Category)
			log.Printf("BackgroundUpdater: MODERN daily parlay completed for Season %d Week %d Day %s, triggering immediate daily scoring",
				bu.currentSeason, weekCategory.Week, dayName)
			if err := bu.parlayService.ProcessDailyParlayScoring(ctx, bu.currentSeason, weekCategory.Week); err != nil {
				log.Printf("BackgroundUpdater: Failed to process MODERN daily parlay scoring for Season %d Week %d Day %s: %v",
					bu.currentSeason, weekCategory.Week, dayName, err)
			} else {
				// Mark category as processed
				bu.processedCategories[weekCategory] = true
			}
		}
	}

	// Check for completed weeks and trigger appropriate scoring system
	completedWeeks := bu.checkForCompletedWeeks(games)

	// Process week completion with season-appropriate scoring
	for _, week := range completedWeeks {
		// Skip if this week has already been processed in this session
		if bu.processedWeeks[week] {
			continue
		}

		// Check if parlay scores already exist in database for this week
		// If scores exist, skip processing to avoid duplicate scoring
		hasExistingScores, err := bu.parlayService.CheckWeekHasParlayScores(ctx, bu.currentSeason, week)
		if err != nil {
			log.Printf("BackgroundUpdater: Error checking existing scores for Season %d Week %d: %v", bu.currentSeason, week, err)
			continue
		}
		if hasExistingScores {
			log.Printf("BackgroundUpdater: Season %d Week %d already has parlay scores, skipping automatic scoring", bu.currentSeason, week)
			bu.processedWeeks[week] = true // Mark as processed to avoid repeated checks
			continue
		}

		if models.IsModernSeason(bu.currentSeason) {
			// MODERN: 2025+ use daily parlay scoring
			log.Printf("BackgroundUpdater: All games complete for Season %d Week %d, triggering MODERN daily parlay scoring", bu.currentSeason, week)
			if err := bu.parlayService.ProcessDailyParlayScoring(ctx, bu.currentSeason, week); err != nil {
				log.Printf("BackgroundUpdater: Failed to process MODERN daily parlay scoring for Season %d Week %d: %v", bu.currentSeason, week, err)
			} else {
				// Mark week as processed
				bu.processedWeeks[week] = true
			}
		} else {
			// LEGACY: 2023-2024 use traditional week-based scoring
			log.Printf("BackgroundUpdater: All games complete for Season %d Week %d, triggering LEGACY week parlay scoring", bu.currentSeason, week)
			if err := bu.parlayService.ProcessWeekParlayScoring(ctx, bu.currentSeason, week); err != nil {
				log.Printf("BackgroundUpdater: Failed to process LEGACY parlay scoring for Season %d Week %d: %v", bu.currentSeason, week, err)
			} else {
				// Mark week as processed
				bu.processedWeeks[week] = true
			}
		}
	}

	// Separate step: Enrich games with odds data (independent of scoreboard updates)
	// Only run odds enrichment every 30 minutes to avoid excessive API calls
	// if time.Since(bu.lastOddsUpdate) >= 30*time.Minute {
	// 	bu.enrichOddsForMissingGames()
	// 	bu.lastOddsUpdate = time.Now()
	// }

	duration := time.Since(startTime)
	log.Printf("BackgroundUpdater: Update completed in %v - %d games processed, %d weeks completed",
		duration, len(games), len(completedWeeks))
}

// checkForCompletedWeeks returns weeks that are now fully completed
func (bu *BackgroundUpdater) checkForCompletedWeeks(games []models.Game) []int {
	// Group games by week
	weekGames := make(map[int][]models.Game)
	for _, game := range games {
		weekGames[game.Week] = append(weekGames[game.Week], game)
	}

	var completedWeeks []int
	for week, gamesInWeek := range weekGames {
		if len(gamesInWeek) == 0 {
			continue
		}

		// Check if all games in this week are completed
		allCompleted := true
		for _, game := range gamesInWeek {
			if !game.IsCompleted() {
				allCompleted = false
				break
			}
		}

		if allCompleted {
			completedWeeks = append(completedWeeks, week)
		}
	}

	return completedWeeks
}

// checkForCompletedParlayCategories returns parlay categories that are now fully completed
func (bu *BackgroundUpdater) checkForCompletedParlayCategories(games []models.Game) map[WeekCategory]bool {
	// Group games by week and category
	weekCategoryGames := make(map[WeekCategory][]models.Game)

	for _, game := range games {
		// Determine parlay category based on game date
		category := models.CategorizeGameByDate(game.Date, bu.currentSeason, game.Week)
		weekCategory := WeekCategory{Week: game.Week, Category: category}
		weekCategoryGames[weekCategory] = append(weekCategoryGames[weekCategory], game)
	}

	completedCategories := make(map[WeekCategory]bool)

	for weekCategory, gamesInCategory := range weekCategoryGames {
		if len(gamesInCategory) == 0 {
			continue
		}

		// Check if all games in this category are completed
		allCompleted := true
		for _, game := range gamesInCategory {
			if !game.IsCompleted() {
				allCompleted = false
				break
			}
		}

		if allCompleted {
			completedCategories[weekCategory] = true
		}
	}

	return completedCategories
}

// categoryToDayName maps parlay categories to day names for modern scoring
func (bu *BackgroundUpdater) categoryToDayName(category models.ParlayCategory) string {
	switch category {
	case models.ParlayBonusThursday:
		return "Thursday"
	case models.ParlayBonusFriday:
		return "Friday"
	case models.ParlayRegular:
		return "Sunday/Monday" // Regular category includes both weekend days
	default:
		return string(category)
	}
}

// getUpdateInterval returns the appropriate update interval based on the time of year
// intelligentScheduler manages intelligent polling based on game urgency
func (bu *BackgroundUpdater) intelligentScheduler() {
	for {
		select {
		case <-bu.stopChan:
			log.Println("BackgroundUpdater: Stopping intelligent scheduler")
			return
		default:
			// Determine next update interval based on game states
			interval, updateType := bu.getIntelligentUpdateInterval()

			if updateType != bu.lastUpdateType {
				log.Printf("BackgroundUpdater: Switching to %s polling (interval: %v)", updateType, interval)
				bu.lastUpdateType = updateType
			}

			// Wait for the calculated interval
			timer := time.NewTimer(interval)

			select {
			case <-timer.C:
				go bu.updateGames()
			case <-bu.stopChan:
				timer.Stop()
				log.Println("BackgroundUpdater: Stopping intelligent scheduler")
				return
			}
		}
	}
}

// getIntelligentUpdateInterval calculates update interval based on current game states
func (bu *BackgroundUpdater) getIntelligentUpdateInterval() (time.Duration, string) {
	// Get current games from database to analyze their states
	games, err := bu.gameRepo.GetGamesBySeason(bu.currentSeason)
	if err != nil {
		log.Printf("BackgroundUpdater: Error fetching games for intelligent scheduling: %v", err)
		// Fallback to 2-minute intervals if we can't determine game states
		return 2 * time.Minute, "fallback"
	}

	// Check for games in progress (highest priority - 5 seconds)
	for _, game := range games {
		if game.State == models.GameStateInPlay {
			return 5 * time.Second, "live-games"
		}
	}

	// During NFL season (September through February): poll every 2 minutes
	now := time.Now()
	month := now.Month()
	if month >= 9 || month <= 2 {
		return 2 * time.Minute, "nfl-season"
	}

	// Off-season: poll every 30 minutes
	return 30 * time.Minute, "off-season"
}

// getCurrentNFLWeek determines current NFL week based on date
func (bu *BackgroundUpdater) getCurrentNFLWeek(now time.Time) int {
	// Simple approximation: NFL season starts around September 5th, week 1
	// Each week is 7 days, so we can estimate current week
	year := now.Year()
	if now.Month() < 9 {
		year-- // If before September, we're in previous year's season
	}

	// Approximate NFL season start (first Thursday in September)
	seasonStart := time.Date(year, 9, 5, 0, 0, 0, 0, time.UTC)
	// Adjust to first Thursday
	for seasonStart.Weekday() != time.Thursday {
		seasonStart = seasonStart.AddDate(0, 0, 1)
	}

	daysSinceStart := int(now.Sub(seasonStart).Hours() / 24)
	week := (daysSinceStart / 7) + 1

	// NFL regular season is weeks 1-18
	if week < 1 {
		week = 1
	} else if week > 18 {
		week = 18
	}

	return week
}

func (bu *BackgroundUpdater) getUpdateInterval() time.Duration {
	now := time.Now()
	month := now.Month()

	// NFL season runs roughly September through February
	if month >= 9 || month <= 2 {
		// During season: poll every 2 minutes
		return 2 * time.Minute
	} else {
		// Off-season: poll every 30 minutes
		return 30 * time.Minute
	}
}

// isAfterOddsCutoff checks if current time is after Wednesday 10 PM Pacific for the given week
func (bu *BackgroundUpdater) isAfterOddsCutoff(week int) bool {
	now := time.Now()

	// Load Pacific timezone
	pacificLoc, err := time.LoadLocation("America/Los_Angeles")
	if err != nil {
		log.Printf("BackgroundUpdater: Failed to load Pacific timezone: %v", err)
		// Fallback to UTC-8 (PST)
		pacificLoc = time.FixedZone("PST", -8*3600)
	}

	nowPacific := now.In(pacificLoc)

	// Calculate the Wednesday cutoff for the specified week
	year := nowPacific.Year()
	if nowPacific.Month() < 9 {
		year-- // If before September, we're in previous year's season
	}

	// NFL 2025 season opener is September 5, 2025 (which is actually a Friday)
	// Week 1 runs Tuesday Sept 3 - Monday Sept 9
	seasonOpener := time.Date(year, 9, 5, 0, 0, 0, 0, pacificLoc)

	// Find what day of week the season opener falls on
	openerWeekday := seasonOpener.Weekday()

	// Calculate days back to Tuesday of that week
	daysBackToTuesday := int((openerWeekday - time.Tuesday + 7) % 7)
	week1Tuesday := seasonOpener.AddDate(0, 0, -daysBackToTuesday)

	// Calculate the Tuesday that starts the given week
	weekStartTuesday := week1Tuesday.AddDate(0, 0, (week-1)*7)

	// Wednesday is 1 day after Tuesday
	wednesday := weekStartTuesday.AddDate(0, 0, 1)

	// Set cutoff time to 10 PM Pacific on Wednesday
	cutoffTime := time.Date(wednesday.Year(), wednesday.Month(), wednesday.Day(), 22, 0, 0, 0, pacificLoc)

	return nowPacific.After(cutoffTime)
}

// enrichOddsForMissingGames checks database for games without odds and enriches them
// Only updates odds if we haven't passed the Wednesday 10 PM Pacific cutoff for that week
func (bu *BackgroundUpdater) enrichOddsForMissingGames() {
	// Get all games from database for current season
	dbGames, err := bu.gameRepo.GetGamesBySeason(bu.currentSeason)
	if err != nil {
		log.Printf("BackgroundUpdater: Failed to get games for odds enrichment: %v", err)
		return
	}

	// Find scheduled games without odds, but only if we haven't passed the cutoff
	var gamesNeedingOdds []models.Game
	var gamesSkippedByCutoff []models.Game

	for _, game := range dbGames {
		if game.Odds == nil && game.State == models.GameStateScheduled {
			// Check if odds cutoff has passed for this game's week
			if bu.isAfterOddsCutoff(game.Week) {
				gamesSkippedByCutoff = append(gamesSkippedByCutoff, *game)
				continue // Skip this game - odds are locked for this week
			}
			gamesNeedingOdds = append(gamesNeedingOdds, *game)
		}
	}

	// Log cutoff information
	if len(gamesSkippedByCutoff) > 0 {
		log.Printf("BackgroundUpdater: ODDS CUTOFF - Skipped %d games past Wednesday 10 PM Pacific deadline:", len(gamesSkippedByCutoff))
		for _, game := range gamesSkippedByCutoff {
			log.Printf("  Week %d: Game %d (%s vs %s) - odds locked",
				game.Week, game.ID, game.Away, game.Home)
		}
	}

	if len(gamesNeedingOdds) == 0 {
		if len(gamesSkippedByCutoff) > 0 {
			log.Printf("BackgroundUpdater: No odds updates needed - all eligible games past cutoff")
		}
		return // No games need odds or all are past cutoff
	}

	log.Printf("BackgroundUpdater: Found %d games eligible for odds enrichment (before cutoff)", len(gamesNeedingOdds))
	for _, game := range gamesNeedingOdds {
		log.Printf("  Week %d: Game %d (%s vs %s) - attempting odds fetch",
			game.Week, game.ID, game.Away, game.Home)
	}

	// Enrich with odds for games that haven't passed cutoff
	enrichedGames := bu.espnService.EnrichGamesWithOdds(gamesNeedingOdds)

	// Only update games that actually got odds
	var gamesToUpdate []*models.Game
	oddsAdded := 0
	for i, game := range enrichedGames {
		if game.Odds != nil && i < len(gamesNeedingOdds) && gamesNeedingOdds[i].Odds == nil {
			gamesToUpdate = append(gamesToUpdate, &game)
			oddsAdded++

			// Log the detailed odds update with before/after values and game timing
			pacificTime, _ := time.LoadLocation("America/Los_Angeles")
			gameTimePacific := game.Date.In(pacificTime)

			log.Printf("BackgroundUpdater: ODDS UPDATE for Game %d (Week %d: %s vs %s)",
				game.ID, game.Week, game.Away, game.Home)
			log.Printf("  Game Time: %s Pacific", gameTimePacific.Format("Mon 1/2/2006 3:04 PM MST"))
			log.Printf("  BEFORE: Odds = nil (no odds available)")
			log.Printf("  AFTER:  Odds = Spread: %.1f, O/U: %.1f",
				game.Odds.Spread, game.Odds.OU)
			log.Printf("  STATUS: SUCCESS - Added new odds to database")
		}
	}

	// SANITY CHECK: Double-check cutoff times before database update
	// (Defense in depth - protects against future code changes)
	if len(gamesToUpdate) > 0 {
		var safeToUpdate []*models.Game
		var blockedByPolicy []models.Game

		for _, game := range gamesToUpdate {
			if bu.isAfterOddsCutoff(game.Week) {
				// Block this odds update - past cutoff
				blockedByPolicy = append(blockedByPolicy, *game)
				log.Printf("BackgroundUpdater: 🚫 SANITY CHECK BLOCK - Game %d (%s vs %s) Week %d odds update blocked (past Wednesday 10 PM Pacific cutoff)",
					game.ID, game.Away, game.Home, game.Week)
			} else {
				// Safe to update - before cutoff
				safeToUpdate = append(safeToUpdate, game)
			}
		}

		if len(blockedByPolicy) > 0 {
			log.Printf("BackgroundUpdater: 🚫 SANITY CHECK blocked %d games from odds updates due to cutoff policy", len(blockedByPolicy))
		}

		if len(safeToUpdate) > 0 {
			err = bu.gameRepo.BulkUpsertGames(safeToUpdate)
			if err != nil {
				log.Printf("BackgroundUpdater: Failed to update games with odds: %v", err)
				return
			}
			log.Printf("BackgroundUpdater: Successfully added odds to %d games", len(safeToUpdate))
		} else {
			log.Printf("BackgroundUpdater: No odds updates processed - all games blocked by sanity check")
		}
	}

	// Enhanced summary logging for odds enrichment process
	totalAttempted := len(gamesNeedingOdds)
	totalSuccessful := oddsAdded
	totalFailed := totalAttempted - totalSuccessful

	if totalAttempted > 0 {
		log.Printf("BackgroundUpdater: ODDS ENRICHMENT SUMMARY:")
		log.Printf("  Total games needing odds: %d", totalAttempted)
		log.Printf("  Successful odds fetches: %d (%.1f%%)", totalSuccessful, float64(totalSuccessful)/float64(totalAttempted)*100)
		log.Printf("  Failed odds fetches: %d (%.1f%%)", totalFailed, float64(totalFailed)/float64(totalAttempted)*100)

		// Group failed games by week for better analysis
		failedByWeek := make(map[int]int)
		for i := totalSuccessful; i < totalAttempted; i++ {
			if i < len(gamesNeedingOdds) {
				failedByWeek[gamesNeedingOdds[i].Week]++
			}
		}

		if len(failedByWeek) > 0 {
			log.Printf("  Failed odds by week:")
			for week, count := range failedByWeek {
				log.Printf("    Week %d: %d failed", week, count)
			}
		}
	}
}

// IsRunning returns whether the background updater is currently running
func (bu *BackgroundUpdater) IsRunning() bool {
	return bu.running
}
