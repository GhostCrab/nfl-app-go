package services

import (
	"context"
	"log"
	"nfl-app-go/database"
	"nfl-app-go/models"
	"time"
)

// BackgroundUpdater handles automatic ESPN API polling and game updates
type BackgroundUpdater struct {
	espnService  *ESPNService
	gameRepo     *database.MongoGameRepository
	pickService  *PickService
	currentSeason int
	ticker       *time.Ticker
	stopChan     chan bool
	running      bool
	lastUpdateType string // Track what type of update we last did
}

// NewBackgroundUpdater creates a new background updater service
func NewBackgroundUpdater(espnService *ESPNService, gameRepo *database.MongoGameRepository, pickService *PickService, currentSeason int) *BackgroundUpdater {
	return &BackgroundUpdater{
		espnService:   espnService,
		gameRepo:      gameRepo,
		pickService:   pickService,
		currentSeason: currentSeason,
		stopChan:      make(chan bool),
		running:       false,
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
	
	// Track changes for logging and parlay scoring
	var updatedGames []models.Game
	var gamesToUpdate []*models.Game // Only games that actually changed
	var completedWeeks []int // Weeks that became fully completed
	
	// Check each game for changes and collect only those that need updating
	for i := range games {
		gameChanged := false
		
		// Check if this game's data changed (excluding odds, since scoreboard API doesn't include odds)
		if existing, exists := existingGameMap[games[i].ID]; exists {
			if existing.State != games[i].State || 
			   existing.AwayScore != games[i].AwayScore || 
			   existing.HomeScore != games[i].HomeScore ||
			   existing.Quarter != games[i].Quarter {
				
				gameChanged = true
				updatedGames = append(updatedGames, games[i])
				log.Printf("BackgroundUpdater: GAME UPDATE for Game %d (%s vs %s)", 
					games[i].ID, games[i].Away, games[i].Home)
				log.Printf("  BEFORE: State=%s, Score=%d-%d, Quarter=%d", 
					existing.State, existing.AwayScore, existing.HomeScore, existing.Quarter)
				log.Printf("  AFTER:  State=%s, Score=%d-%d, Quarter=%d", 
					games[i].State, games[i].AwayScore, games[i].HomeScore, games[i].Quarter)
				
				// Log odds preservation
				if existing.Odds != nil {
					log.Printf("  ODDS: Preserved from database - Spread: %.1f, O/U: %.1f", 
						existing.Odds.Spread, existing.Odds.OU)
				} else {
					log.Printf("  ODDS: None available (will attempt enrichment separately)")
				}
			}
		} else {
			// New game that doesn't exist in database
			gameChanged = true
			updatedGames = append(updatedGames, games[i])
			log.Printf("BackgroundUpdater: NEW GAME detected - %d: %s vs %s", 
				games[i].ID, games[i].Away, games[i].Home)
			log.Printf("  State=%s, Score=%d-%d, Quarter=%d", 
				games[i].State, games[i].AwayScore, games[i].HomeScore, games[i].Quarter)
		}
		
		// Add to update list if changed, but preserve existing odds data
		if gameChanged {
			// Preserve odds data from database since scoreboard API doesn't include it
			if existing, exists := existingGameMap[games[i].ID]; exists && existing.Odds != nil {
				games[i].Odds = existing.Odds
			}
			gamesToUpdate = append(gamesToUpdate, &games[i])
		}
	}
	
	// Only update games that actually changed
	if len(gamesToUpdate) > 0 {
		log.Printf("BackgroundUpdater: Updating %d changed games in database", len(gamesToUpdate))
		err = bu.gameRepo.BulkUpsertGames(gamesToUpdate)
		if err != nil {
			log.Printf("BackgroundUpdater: Failed to update games in database: %v", err)
			return
		}
	} else {
		log.Printf("BackgroundUpdater: No games changed, skipping database update")
	}
	
	// Check for newly completed weeks and trigger parlay scoring
	if len(updatedGames) > 0 {
		completedWeeks = bu.checkForCompletedWeeks(games)
		
		// Process parlay scoring for completed weeks
		for _, week := range completedWeeks {
			log.Printf("BackgroundUpdater: All games complete for Season %d Week %d, triggering parlay scoring", bu.currentSeason, week)
			if err := bu.pickService.ProcessWeekParlayScoring(ctx, bu.currentSeason, week); err != nil {
				log.Printf("BackgroundUpdater: Failed to process parlay scoring for Season %d Week %d: %v", bu.currentSeason, week, err)
			}
		}
	}
	
	// Separate step: Enrich games with odds data (independent of scoreboard updates)
	bu.enrichOddsForMissingGames()
	
	duration := time.Since(startTime)
	if len(updatedGames) > 0 || len(completedWeeks) > 0 {
		log.Printf("BackgroundUpdater: Update completed in %v - %d games processed, %d updated, %d weeks completed", 
			duration, len(games), len(updatedGames), len(completedWeeks))
	} else {
		log.Printf("BackgroundUpdater: Update completed in %v - %d games processed, no changes detected", 
			duration, len(games))
	}
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
		// Fallback to 30-minute intervals if we can't determine game states
		return 30 * time.Minute, "fallback"
	}
	
	now := time.Now()
	currentWeek := bu.getCurrentNFLWeek(now)
	
	// Check for games in progress (highest priority - 5 seconds)
	for _, game := range games {
		if game.State == models.GameStateInPlay {
			return 5 * time.Second, "live-games"
		}
	}
	
	// Current week games (30 minutes) - but no need for frequent updates if games are starting soon
	// since odds don't change during game day
	hasCurrentWeekGames := false
	for _, game := range games {
		if game.Week == currentWeek {
			hasCurrentWeekGames = true
			break
		}
	}
	if hasCurrentWeekGames {
		return 30 * time.Minute, "current-week"
	}
	
	// Next week games (6 hours)
	hasNextWeekGames := false
	nextWeek := currentWeek + 1
	for _, game := range games {
		if game.Week == nextWeek {
			hasNextWeekGames = true
			break
		}
	}
	if hasNextWeekGames {
		return 6 * time.Hour, "next-week"
	}
	
	// Future weeks (24 hours)
	return 24 * time.Hour, "future-weeks"
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
			
			// Log the detailed odds update with before/after values
			log.Printf("BackgroundUpdater: ODDS UPDATE for Game %d (%s vs %s)", 
				game.ID, game.Away, game.Home)
			log.Printf("  BEFORE: Odds = nil (no odds available)")
			log.Printf("  AFTER:  Odds = Spread: %.1f, O/U: %.1f", 
				game.Odds.Spread, game.Odds.OU)
			log.Printf("  STATUS: SUCCESS - Added new odds to database")
		} else if i < len(gamesNeedingOdds) {
			// Log failed odds fetch attempts
			failedGame := gamesNeedingOdds[i]
			log.Printf("BackgroundUpdater: ODDS FETCH FAILED for Game %d (%s vs %s) - ESPN API returned no odds", 
				failedGame.ID, failedGame.Away, failedGame.Home)
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
}


// IsRunning returns whether the background updater is currently running
func (bu *BackgroundUpdater) IsRunning() bool {
	return bu.running
}