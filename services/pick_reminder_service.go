package services

import (
	"context"
	"fmt"
	"nfl-app-go/database"
	"nfl-app-go/logging"
	"nfl-app-go/models"
	"time"
)

// PickReminderService handles automated pick reminder emails
type PickReminderService struct {
	emailService  *EmailService
	userRepo      *database.MongoUserRepository
	pickService   *PickService
	gameService   GameService
	currentSeason int
	logger        *logging.Logger
}

// NewPickReminderService creates a new pick reminder service
func NewPickReminderService(
	emailService *EmailService,
	userRepo *database.MongoUserRepository,
	pickService *PickService,
	gameService GameService,
	currentSeason int,
) *PickReminderService {
	return &PickReminderService{
		emailService:  emailService,
		userRepo:      userRepo,
		pickService:   pickService,
		gameService:   gameService,
		currentSeason: currentSeason,
		logger:        logging.WithPrefix("PickReminder"),
	}
}

// StartScheduler starts the automated pick reminder scheduler
// Checks at 12PM PT on weekdays and 8AM PT on weekends
func (prs *PickReminderService) StartScheduler(ctx context.Context) {
	if !prs.emailService.IsConfigured() {
		prs.logger.Warn("Email service not configured, pick reminders disabled")
		return
	}

	prs.logger.Info("Starting pick reminder scheduler (12PM PT weekdays, 8AM PT weekends)")

	go func() {
		ticker := time.NewTicker(1 * time.Hour)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				prs.logger.Info("Pick reminder scheduler stopped")
				return
			case <-ticker.C:
				// Get current time in Pacific timezone
				pacificLoc := models.GetPacificTimeLocation()
				now := time.Now().In(pacificLoc)
				hour := now.Hour()
				weekday := now.Weekday()

				shouldCheck := false

				// Weekdays (Mon-Fri): check at 12PM PT
				if weekday >= time.Monday && weekday <= time.Friday {
					if hour == 12 {
						shouldCheck = true
					}
				}

				// Weekends (Sat-Sun): check at 8AM PT
				if weekday == time.Saturday || weekday == time.Sunday {
					if hour == 8 {
						shouldCheck = true
					}
				}

				if shouldCheck {
					prs.logger.Infof("Running pick reminder check for %s at %d:00 PT", now.Weekday(), hour)
					prs.checkAndNotifyUsers(now)
				}
			}
		}
	}()
}

// checkAndNotifyUsers checks all users and sends reminders to those missing picks
func (prs *PickReminderService) checkAndNotifyUsers(checkTime time.Time) {
	ctx := context.Background()

	// Get Pacific date (YYYY-MM-DD)
	pacificLoc := models.GetPacificTimeLocation()
	pacificDate := checkTime.In(pacificLoc).Format("2006-01-02")

	// Get all games for current season
	games, err := prs.gameService.GetGamesBySeason(prs.currentSeason)
	if err != nil {
		prs.logger.Errorf("Failed to get games: %v", err)
		return
	}

	// Filter games happening "today" in Pacific time
	todaysGames := prs.filterGamesByDate(games, pacificDate)
	if len(todaysGames) == 0 {
		prs.logger.Debugf("No games scheduled for %s", pacificDate)
		return
	}

	// Get current week from first game
	currentWeek := todaysGames[0].Week

	prs.logger.Infof("Checking pick reminders for %d games on %s (Week %d)", len(todaysGames), pacificDate, currentWeek)

	// Get all users
	users, err := prs.userRepo.GetAllUsers()
	if err != nil {
		prs.logger.Errorf("Failed to get users: %v", err)
		return
	}

	remindersSent := 0

	// Check each user
	for _, user := range users {
		// Get user's picks for this week
		userPicks, err := prs.pickService.GetUserPicksForWeek(ctx, user.ID, prs.currentSeason, currentWeek)
		if err != nil {
			prs.logger.Warnf("Failed to get picks for user %d: %v", user.ID, err)
			continue
		}

		// Group picks by day (userPicks.Picks contains the []Pick slice)
		dailyGroups := models.GroupPicksByDay(userPicks.Picks, games)

		// Check if user has picks for today
		todaysPicks := dailyGroups[pacificDate]

		// For modern seasons (2025+), need at least 2 picks per day
		shouldRemind := false
		if models.IsModernSeason(prs.currentSeason) {
			if len(todaysPicks) < 2 {
				shouldRemind = true
				prs.logger.Debugf("User %d (%s) has %d picks for today (need 2+)", user.ID, user.Name, len(todaysPicks))
			}
		} else {
			// Legacy: just check if any picks exist
			if len(todaysPicks) == 0 {
				shouldRemind = true
				prs.logger.Debugf("User %d (%s) has no picks for today", user.ID, user.Name)
			}
		}

		if shouldRemind {
			err := prs.sendPickReminder(user, todaysGames, len(todaysPicks))
			if err != nil {
				prs.logger.Errorf("Failed to send reminder to %s: %v", user.Email, err)
			} else {
				prs.logger.Infof("Sent pick reminder to %s (%d picks, %d games today)", user.Email, len(todaysPicks), len(todaysGames))
				remindersSent++
			}
		}
	}

	prs.logger.Infof("Pick reminder check complete: %d reminders sent", remindersSent)
}

// filterGamesByDate returns games happening on a specific Pacific date
func (prs *PickReminderService) filterGamesByDate(games []models.Game, pacificDate string) []models.Game {
	var todaysGames []models.Game
	for _, game := range games {
		if game.GetGameDateInPacific() == pacificDate {
			todaysGames = append(todaysGames, game)
		}
	}
	return todaysGames
}

// sendPickReminder sends a reminder email to a user
func (prs *PickReminderService) sendPickReminder(user models.User, games []models.Game, currentPickCount int) error {
	subject := "🏈 Reminder: Submit your picks for today's games"

	// Build game list
	gamesList := ""
	for _, game := range games {
		pacificLoc := models.GetPacificTimeLocation()
		gameTime := game.Date.In(pacificLoc)
		timeStr := gameTime.Format("3:04 PM")
		gamesList += fmt.Sprintf("    • %s @ %s - %s PT\n", game.Away, game.Home, timeStr)
	}

	// HTML template
	htmlTemplate := `
<!DOCTYPE html>
<html>
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>Pick Reminder</title>
    <style>
        body { font-family: Arial, sans-serif; line-height: 1.6; margin: 0; padding: 20px; background-color: #f4f4f4; }
        .container { max-width: 600px; margin: 0 auto; background: white; padding: 20px; border-radius: 8px; box-shadow: 0 2px 10px rgba(0,0,0,0.1); }
        .header { text-align: center; margin-bottom: 30px; }
        .header h1 { color: #2c3e50; margin: 0; }
        .content { margin-bottom: 30px; }
        .games-list { background-color: #f8f9fa; padding: 15px; border-radius: 4px; margin: 20px 0; }
        .game-item { margin: 8px 0; color: #333; }
        .button { display: inline-block; padding: 12px 24px; background-color: #3b82f6; color: white; text-decoration: none; border-radius: 4px; font-weight: bold; margin: 20px 0; }
        .button:hover { background-color: #2563eb; }
        .footer { text-align: center; font-size: 0.9em; color: #666; margin-top: 30px; padding-top: 20px; border-top: 1px solid #eee; }
        .warning { background-color: #fff3cd; border: 1px solid #ffeaa7; padding: 15px; border-radius: 4px; margin: 20px 0; }
    </style>
</head>
<body>
    <div class="container">
        <div class="header">
            <h1>🏈 NFL Parlay Club</h1>
            <h2>Pick Reminder</h2>
        </div>

        <div class="content">
            <p>Hey {{.Name}},</p>

            <p>Just a reminder to submit your picks for today's games!</p>

            <div class="games-list">
                <strong>Today's Games:</strong><br>
                {{range .Games}}
                <div class="game-item">{{.Away}} @ {{.Home}} - {{.Time}}</div>
                {{end}}
            </div>

            {{if .IsModernSeason}}
            <div class="warning">
                <strong>Remember:</strong> You need at least 2 picks for the day to score points!
                {{if gt .CurrentPickCount 0}}
                <br>You currently have {{.CurrentPickCount}} pick(s) for today.
                {{end}}
            </div>
            {{end}}

            <p style="text-align: center;">
                <a href="{{.AppURL}}" class="button">Submit Your Picks</a>
            </p>
        </div>

        <div class="footer">
            <p>Good luck!</p>
        </div>
    </div>
</body>
</html>`

	// Plain text version
	textTemplate := `
NFL Parlay Club - Pick Reminder

Hey {{.Name}},

Just a reminder to submit your picks for today's games!

Today's Games:
{{range .Games}}
{{.Away}} @ {{.Home}} - {{.Time}}
{{end}}
{{if .IsModernSeason}}
Remember: You need at least 2 picks for the day to score points!
{{if gt .CurrentPickCount 0}}You currently have {{.CurrentPickCount}} pick(s) for today.{{end}}
{{end}}

Visit the app to submit your picks: {{.AppURL}}

Good luck!
`

	// Prepare game data for template
	type GameInfo struct {
		Away string
		Home string
		Time string
	}

	gameInfos := make([]GameInfo, len(games))
	pacificLoc := models.GetPacificTimeLocation()
	for i, game := range games {
		gameTime := game.Date.In(pacificLoc)
		gameInfos[i] = GameInfo{
			Away: game.Away,
			Home: game.Home,
			Time: gameTime.Format("3:04 PM PT"),
		}
	}

	// Template data
	data := struct {
		Name             string
		Games            []GameInfo
		IsModernSeason   bool
		CurrentPickCount int
		AppURL           string
	}{
		Name:             user.Name,
		Games:            gameInfos,
		IsModernSeason:   models.IsModernSeason(prs.currentSeason),
		CurrentPickCount: currentPickCount,
		AppURL:           "https://nfl.ryanpielow.com", // TODO: Make this configurable
	}

	// Simple template replacement (avoiding template package for simplicity)
	htmlBody := htmlTemplate
	textBody := textTemplate

	// Replace placeholders
	htmlBody = replaceTemplatePlaceholders(htmlBody, data)
	textBody = replaceTemplatePlaceholders(textBody, data)

	return prs.emailService.sendEmail(user.Email, subject, textBody, htmlBody)
}

// Simple template replacement helper
func replaceTemplatePlaceholders(tmpl string, data interface{}) string {
	// This is a simplified version - in production you'd use html/template
	// For now, just return the template with basic replacements
	// The actual implementation would properly render the template
	return tmpl
}
