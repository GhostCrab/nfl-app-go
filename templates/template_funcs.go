package templates

import (
	"encoding/json"
	"fmt"
	"html/template"
	"log"
	"nfl-app-go/models"
	"regexp"
	"sort"
	"strings"
	"time"
)

// GetTemplateFuncs returns the template function map for HTML templates
func GetTemplateFuncs() template.FuncMap {
	return template.FuncMap{
		// Basic math functions
		"add":     func(a, b int) int { return a + b },
		"sub":     func(a, b float64) float64 { return a - b },
		"minus":   func(a, b int) int { return a - b },
		"plus":    func(a, b int) int { return a + b },
		"mul":     func(a, b float64) float64 { return a * b },
		"float64": func(i int) float64 { return float64(i) },
		"ceil":    func(f float64) int { return int(f + 0.999999) },

		// Utility functions
		"sequence": func(start, end int) []int {
			result := make([]int, end-start+1)
			for i := range result {
				result[i] = start + i
			}
			return result
		},
		"seq": func(start, end int) []int {
			result := make([]int, end-start+1)
			for i := range result {
				result[i] = start + i
			}
			return result
		},
		"slice": func(items ...string) []string {
			return items
		},

		// String functions
		"lower":    strings.ToLower,
		"contains": strings.Contains,
		"split":    strings.Split,
		"regexReplace": func(input, pattern, replacement string) string {
			re := regexp.MustCompile(pattern)
			return re.ReplaceAllString(input, replacement)
		},

		// JSON and data functions
		"toJSON": func(v interface{}) template.JS {
			data, _ := json.Marshal(v)
			return template.JS(data)
		},
		"dict": func(values ...interface{}) (map[string]interface{}, error) {
			if len(values)%2 != 0 {
				return nil, fmt.Errorf("dict: number of arguments must be even")
			}
			result := make(map[string]interface{})
			for i := 0; i < len(values); i += 2 {
				key, ok := values[i].(string)
				if !ok {
					return nil, fmt.Errorf("dict: key must be string, got %T", values[i])
				}
				result[key] = values[i+1]
			}
			return result, nil
		},

		// Debug functions
		"debugLog": func(msg string) string {
			log.Printf("TEMPLATE DEBUG: %s", msg)
			return ""
		},

		// Game and scoring functions
		"projectFinalScore":     projectFinalScore,
		"findGameByID":          findGameByID,
		"isPickedTeamCovering":  isPickedTeamCovering,
		"isSpreadPickWinning":   isSpreadPickWinning,
		"calculateSpreadResult": calculateSpreadResult,

		// Pick functions
		"isOverUnder":           func(pick models.Pick) bool { return pick.IsOverUnder() },
		"isSpreadPick":          func(pick models.Pick) bool { return pick.IsSpreadPick() },
		"getResultClass":        getResultClass,
		"getPickTeamAbbr":       getPickTeamAbbr,
		"getPickTeamIcon":       getPickTeamIcon,
		"getPickValue":          getPickValue,
		"sortPicksByGameTime":   sortPicksByGameTime,

		// Team functions
		"getTeamMascotName": getTeamMascotName,

		// User sorting functions
		"sortUsersByScore":          sortUsersByScore,
		"sortUsersWithCurrentFirst": sortUsersWithCurrentFirst,

		// Date and time functions
		"getDayNameFromDate": getDayNameFromDate,
		"formatAwaySpread":   formatAwaySpread,
		"formatHomeSpread":   formatHomeSpread,

		// Modern season functions
		"hasDailyGroups": hasDailyGroups,
	}
}

// projectFinalScore calculates projected final score based on current game state
func projectFinalScore(homeScore, awayScore, quarter int, timeLeft string) float64 {
	// Parse time left (e.g., "12:34")
	var minutes, seconds int
	if timeLeft == "Halftime" || timeLeft == "" {
		minutes = 0
		seconds = 0
	} else {
		fmt.Sscanf(timeLeft, "%d:%d", &minutes, &seconds)
	}

	// Calculate elapsed time in minutes
	var elapsedMinutes float64
	switch quarter {
	case 1:
		elapsedMinutes = 15 - float64(minutes) - float64(seconds)/60
	case 2:
		elapsedMinutes = 30 - float64(minutes) - float64(seconds)/60
	case 3:
		elapsedMinutes = 45 - float64(minutes) - float64(seconds)/60
	case 4:
		elapsedMinutes = 60 - float64(minutes) - float64(seconds)/60
	case 6: // Halftime
		elapsedMinutes = 30
	default:
		elapsedMinutes = 60 // Overtime or unknown, assume full game
	}

	// Avoid division by zero
	if elapsedMinutes <= 0 {
		elapsedMinutes = 1
	}

	// Calculate current total and project to 60 minutes
	currentTotal := float64(homeScore + awayScore)
	projectedTotal := (currentTotal / elapsedMinutes) * 60

	return projectedTotal
}

// findGameByID finds a game by its ID from a slice of games
func findGameByID(games []models.Game, gameID int) *models.Game {
	for _, game := range games {
		if game.ID == gameID {
			return &game
		}
	}
	return nil
}

// isPickedTeamCovering determines if the picked team is covering the spread
func isPickedTeamCovering(pick models.Pick, game models.Game) string {
	if !pick.IsSpreadPick() || !game.HasOdds() {
		return "neutral"
	}

	teamName := pick.TeamName

	// Check if picked team name contains home or away team abbreviation
	isHomeTeamPick := false
	isAwayTeamPick := false

	// Simple matching - check if team abbreviation is in the name
	if len(teamName) > 0 && len(game.Home) > 0 && len(game.Away) > 0 {
		// Try to match by checking if abbreviation is in the name
		homeTeamLower := strings.ToLower(game.Home)
		awayTeamLower := strings.ToLower(game.Away)
		teamNameLower := strings.ToLower(teamName)

		// Check various ways the team might be referenced
		if strings.Contains(teamNameLower, homeTeamLower) {
			isHomeTeamPick = true
		} else if strings.Contains(teamNameLower, awayTeamLower) {
			isAwayTeamPick = true
		} else {
			// Fallback: if we can't determine, assume it's away team (common pattern)
			isAwayTeamPick = true
		}
	}

	// Calculate spread coverage
	scoreDiff := game.HomeScore - game.AwayScore
	spread := game.Odds.Spread
	adjustedDiff := float64(scoreDiff) + spread

	if isHomeTeamPick {
		if adjustedDiff > 0 {
			return "covering"
		} else if adjustedDiff < 0 {
			return "not-covering"
		} else {
			return "push"
		}
	} else if isAwayTeamPick {
		if adjustedDiff < 0 {
			return "covering"
		} else if adjustedDiff > 0 {
			return "not-covering"
		} else {
			return "push"
		}
	}

	return "neutral"
}

// isSpreadPickWinning determines if a spread pick is winning (simplified)
func isSpreadPickWinning(pick models.Pick, game models.Game) bool {
	if pick.TeamID == 98 || pick.TeamID == 99 {
		return false // This is an O/U pick, not spread
	}
	// For spread picks, we need to determine which team and check if they're covering
	// This is a simplified version - you'd need actual team mapping
	homeScore := game.HomeScore
	awayScore := game.AwayScore
	if game.HasOdds() {
		// Simple logic: if away team picked and away team leading by more than spread
		// Or if home team picked and home team leading by more than spread
		// This is simplified - real implementation needs team ID mapping
		scoreDiff := homeScore - awayScore
		return scoreDiff > 0 // Simplified for now
	}
	return homeScore > awayScore
}

// calculateSpreadResult calculates the spread result
func calculateSpreadResult(homeScore, awayScore int, spread float64) string {
	scoreDiff := float64(homeScore - awayScore)
	spreadDiff := scoreDiff + spread

	if spreadDiff > 0 {
		return "home-covered"
	} else if spreadDiff < 0 {
		return "away-covered"
	} else {
		return "push"
	}
}

// getResultClass returns CSS class for pick result
func getResultClass(pick models.Pick, game *models.Game) string {
	baseClass := pick.GetResultClass()

	// Add state-specific classes for pending picks
	if baseClass == "pick-class" && game != nil {
		if game.State == models.GameStateInPlay {
			return baseClass + " in-progress"
		} else if game.State == models.GameStateScheduled {
			return baseClass + " pending"
		}
	}

	return baseClass
}

// getPickTeamAbbr returns the team abbreviation for a pick
func getPickTeamAbbr(pick models.Pick, game *models.Game, pickDesc string) string {
	if pick.IsOverUnder() {
		// Use TeamID directly for reliable over/under detection
		if pick.TeamID == 99 {
			return "OVR"
		} else if pick.TeamID == 98 {
			return "UND"
		}
		// Fallback: check TeamName if TeamID isn't set correctly
		if strings.Contains(pick.TeamName, "Over") {
			return "OVR"
		} else {
			return "UND"
		}
	}
	// For spread picks, return the team abbreviation
	if game != nil && strings.Contains(pick.TeamName, game.Home) {
		return game.Home
	} else if game != nil && strings.Contains(pick.TeamName, game.Away) {
		return game.Away
	}
	return pick.TeamName
}

// getPickTeamIcon returns the team icon URL
func getPickTeamIcon(teamAbbr string) string {
	if teamAbbr == "OVR" {
		return "https://api.iconify.design/mdi/chevron-double-up.svg"
	}
	if teamAbbr == "UND" {
		return "https://api.iconify.design/mdi/chevron-double-down.svg"
	}
	if teamAbbr == "" {
		return ""
	}
	teamLower := strings.ToLower(teamAbbr)
	return fmt.Sprintf("https://a.espncdn.com/combiner/i?img=/i/teamlogos/nfl/500/scoreboard/%s.png", teamLower)
}

// getPickValue returns the display value for a pick
func getPickValue(pick models.Pick, game *models.Game, pickDesc string) string {
	if pick.IsOverUnder() && game != nil && game.HasOdds() {
		return fmt.Sprintf("%.1f", game.Odds.OU)
	}
	// For spread picks
	if game != nil && game.HasOdds() {
		if strings.Contains(pick.TeamName, game.Home) {
			return game.FormatHomeSpread()
		} else if strings.Contains(pick.TeamName, game.Away) {
			return game.FormatAwaySpread()
		}
	}
	return string(pick.PickType)
}

// getTeamMascotName returns the full mascot name for a team abbreviation
func getTeamMascotName(abbr string) string {
	mascotMap := map[string]string{
		"ARI": "CARDINALS", "ATL": "FALCONS", "BAL": "RAVENS", "BUF": "BILLS",
		"CAR": "PANTHERS", "CHI": "BEARS", "CIN": "BENGALS", "CLE": "BROWNS",
		"DAL": "COWBOYS", "DEN": "BRONCOS", "DET": "LIONS", "GB": "PACKERS",
		"HOU": "TEXANS", "IND": "COLTS", "JAX": "JAGUARS", "KC": "CHIEFS",
		"LV": "RAIDERS", "LAC": "CHARGERS", "LAR": "RAMS", "MIA": "DOLPHINS",
		"MIN": "VIKINGS", "NE": "PATRIOTS", "NO": "SAINTS", "NYG": "GIANTS",
		"NYJ": "JETS", "PHI": "EAGLES", "PIT": "STEELERS", "SF": "49ERS",
		"SEA": "SEAHAWKS", "TB": "BUCCANEERS", "TEN": "TITANS", "WSH": "COMMANDERS",
		"OVR": "OVER", "UND": "UNDER", // Full names for O/U picks on desktop
	}
	if mascot, exists := mascotMap[abbr]; exists {
		return mascot
	}
	return abbr // Fallback to abbreviation if not found
}

// sortPicksByGameTime sorts picks by their associated game start time
func sortPicksByGameTime(picks []models.Pick, games []models.Game) []models.Pick {
	if len(picks) == 0 || len(games) == 0 {
		return picks
	}

	// Create a map of gameID to game for quick lookup
	gameMap := make(map[int]models.Game)
	for _, game := range games {
		gameMap[game.ID] = game
	}

	// Create a copy to avoid modifying original slice
	sortedPicks := make([]models.Pick, len(picks))
	copy(sortedPicks, picks)

	// Sort picks by their game's start time
	sort.Slice(sortedPicks, func(i, j int) bool {
		gameI, existsI := gameMap[sortedPicks[i].GameID]
		gameJ, existsJ := gameMap[sortedPicks[j].GameID]

		// If either game doesn't exist, maintain original order
		if !existsI || !existsJ {
			return i < j
		}

		// Primary sort: by game date/time
		if gameI.Date.Unix() != gameJ.Date.Unix() {
			return gameI.Date.Before(gameJ.Date)
		}

		// Secondary sort: alphabetically by home team name for games at same time
		return gameI.Home < gameJ.Home
	})

	return sortedPicks
}

// sortUsersByScore sorts users by their parlay points (descending)
func sortUsersByScore(userPicks []*models.UserPicks) []*models.UserPicks {
	if len(userPicks) == 0 {
		return userPicks
	}
	// Create a copy to avoid modifying original slice
	sorted := make([]*models.UserPicks, len(userPicks))
	copy(sorted, userPicks)

	// Sort by parlay points (descending - highest first)
	sort.Slice(sorted, func(i, j int) bool {
		scoreI := sorted[i].Record.ParlayPoints
		scoreJ := sorted[j].Record.ParlayPoints
		return scoreI > scoreJ
	})

	return sorted
}

// sortUsersWithCurrentFirst sorts users with current user first, others alphabetically
func sortUsersWithCurrentFirst(userPicks []*models.UserPicks, currentUserName string) []*models.UserPicks {
	log.Printf("TEMPLATE DEBUG: sortUsersWithCurrentFirst called - input count: %d, currentUser: %s", len(userPicks), currentUserName)

	if len(userPicks) == 0 || currentUserName == "" {
		return userPicks
	}
	// Create a copy to avoid modifying original slice
	sorted := make([]*models.UserPicks, len(userPicks))
	copy(sorted, userPicks)

	// Debug input data
	for i, up := range userPicks {
		log.Printf("TEMPLATE DEBUG: Input user %d: %s (picks: %d)", i, up.UserName, len(up.Picks))
	}

	// Sort so current user appears first
	sort.Slice(sorted, func(i, j int) bool {
		isCurrent_i := sorted[i].UserName == currentUserName
		isCurrent_j := sorted[j].UserName == currentUserName

		// Current user should come first
		if isCurrent_i && !isCurrent_j {
			return true
		}
		if !isCurrent_i && isCurrent_j {
			return false
		}

		// For non-current users, maintain alphabetical order
		return sorted[i].UserName < sorted[j].UserName
	})

	// Debug output data
	for i, up := range sorted {
		log.Printf("TEMPLATE DEBUG: Output user %d: %s (picks: %d)", i, up.UserName, len(up.Picks))
	}

	return sorted
}

// getDayNameFromDate returns uppercase day name from date string in Pacific timezone
func getDayNameFromDate(dateStr string) string {
	// Parse date string in format "2025-09-07" directly in Pacific timezone
	// Since the date string already represents the Pacific timezone date
	pacificLoc := models.GetPacificTimeLocation()
	parsedTime, err := time.ParseInLocation("2006-01-02", dateStr, pacificLoc)
	if err != nil {
		log.Printf("Error parsing date %s: %v", dateStr, err)
		return "UNKNOWN"
	}
	// Get day name (already in Pacific timezone)
	dayName := parsedTime.Format("Monday")
	return strings.ToUpper(dayName)
}

// formatAwaySpread formats the spread for the away team
func formatAwaySpread(odds *models.Odds) string {
	if odds == nil {
		return ""
	}
	// Away team gets opposite of the spread
	awaySpread := -odds.Spread
	if awaySpread > 0 {
		return fmt.Sprintf("+%.1f", awaySpread)
	} else if awaySpread < 0 {
		return fmt.Sprintf("%.1f", awaySpread)
	} else {
		return "PK" // Pick 'em
	}
}

// formatHomeSpread formats the spread for the home team
func formatHomeSpread(odds *models.Odds) string {
	if odds == nil {
		return ""
	}
	if odds.Spread > 0 {
		return fmt.Sprintf("+%.1f", odds.Spread)
	} else if odds.Spread < 0 {
		return fmt.Sprintf("%.1f", odds.Spread)
	} else {
		return "PK" // Pick 'em
	}
}

// hasDailyGroups checks if user has daily pick groups (modern seasons)
func hasDailyGroups(userPicks *models.UserPicks) bool {
	if userPicks == nil {
		log.Printf("TEMPLATE DEBUG: hasDailyGroups - userPicks is nil")
		return false
	}
	if userPicks.DailyPickGroups == nil {
		log.Printf("TEMPLATE DEBUG: hasDailyGroups - User %s DailyPickGroups is nil", userPicks.UserName)
		return false
	}
	hasGroups := len(userPicks.DailyPickGroups) > 0
	log.Printf("TEMPLATE DEBUG: hasDailyGroups - User %s has %d daily groups, returning %v", userPicks.UserName, len(userPicks.DailyPickGroups), hasGroups)
	return hasGroups
}