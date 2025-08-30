package main

import (
	"context"
	"fmt"
	"log"
	"math/rand"
	"nfl-app-go/database"
	"nfl-app-go/models"
	"os"
	"time"

	"github.com/joho/godotenv"
)

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

func main() {
	// Set log format
	log.SetFlags(log.LstdFlags | log.Lmicroseconds)
	
	// Load .env file
	if err := godotenv.Load(); err != nil {
		log.Printf("Warning: Could not load .env file: %v", err)
	}
	
	// Initialize MongoDB connection
	dbConfig := database.Config{
		Host:     getEnv("DB_HOST", "p5server"),
		Port:     getEnv("DB_PORT", "27017"),
		Username: getEnv("DB_USERNAME", "nflapp"),
		Password: getEnv("DB_PASSWORD", ""),
		Database: getEnv("DB_NAME", "nfl_app"),
	}
	
	log.Printf("Connecting to MongoDB...")
	db, err := database.NewMongoConnection(dbConfig)
	if err != nil {
		log.Fatalf("Database connection failed: %v", err)
	}
	defer db.Close()
	
	// Test connection
	if err := db.TestConnection(); err != nil {
		log.Fatalf("Database test failed: %v", err)
	}
	
	// Create repositories
	gameRepo := database.NewMongoGameRepository(db)
	pickRepo := database.NewMongoPickRepository(db)
	
	// Get all games for 2025 season
	games, err := gameRepo.GetGamesBySeason(2025)
	if err != nil {
		log.Fatalf("Failed to get games: %v", err)
	}
	
	if len(games) == 0 {
		log.Fatalf("No games found for 2025 season. Please run data loader first.")
	}
	
	log.Printf("Found %d games for 2025 season", len(games))
	
	// List of users to create picks for
	users := []struct {
		ID   int
		Name string
	}{
		{0, "ANDREW"},
		{1, "BARDIA"},
		{2, "COOPER"},
		{3, "MICAH"},
		{4, "RYAN"},
		{5, "TJ"},
		{6, "BRAD"},
	}
	
	// ESPN team ID mapping for team picks
	teamIDMap := map[string]int{
		"ATL": 1, "BUF": 2, "CHI": 3, "CIN": 4, "CLE": 5, "DAL": 6, "DEN": 7, "DET": 8,
		"GB": 9, "TEN": 10, "IND": 11, "KC": 12, "LV": 13, "LAR": 14, "MIA": 15, "MIN": 16,
		"NE": 17, "NO": 18, "NYG": 19, "NYJ": 20, "PHI": 21, "ARI": 22, "PIT": 23, "LAC": 24,
		"SF": 25, "SEA": 26, "TB": 27, "WSH": 28, "CAR": 29, "JAX": 30, "BAL": 33, "HOU": 34,
	}
	
	// Seed random number generator
	rand.Seed(time.Now().UnixNano())
	
	ctx := context.Background()
	totalPicksCreated := 0
	
	// Group games by week for easier processing
	gamesByWeek := make(map[int][]models.Game)
	for _, game := range games {
		gamesByWeek[game.Week] = append(gamesByWeek[game.Week], *game)
	}
	
	// Create picks for each user
	for _, user := range users {
		log.Printf("Creating picks for user %s (ID: %d)", user.Name, user.ID)
		userPicksCreated := 0
		
		// Create picks for each week
		for week := 1; week <= 18; week++ {
			weekGames := gamesByWeek[week]
			if len(weekGames) == 0 {
				continue
			}
			
			// Create 4-6 picks per week
			numPicks := 4 + rand.Intn(3) // 4, 5, or 6 picks
			if numPicks > len(weekGames) {
				numPicks = len(weekGames)
			}
			
			// Randomly select games for this week
			selectedGames := make([]models.Game, len(weekGames))
			copy(selectedGames, weekGames)
			rand.Shuffle(len(selectedGames), func(i, j int) {
				selectedGames[i], selectedGames[j] = selectedGames[j], selectedGames[i]
			})
			selectedGames = selectedGames[:numPicks]
			
			var weekPicks []*models.Pick
			
			for _, game := range selectedGames {
				// Randomly choose pick type (70% spread, 30% over/under)
				isOverUnder := rand.Float64() < 0.3
				
				var pick *models.Pick
				
				if isOverUnder {
					// Over/Under pick
					isOver := rand.Float64() < 0.5
					var teamID int
					var teamName string
					
					if isOver {
						teamID = 99 // Over
						teamName = "Over"
					} else {
						teamID = 98 // Under
						teamName = "Under"
					}
					
					pick = models.CreatePickFromLegacyData(user.ID, game.ID, teamID, 2025, week)
					pick.TeamName = teamName
					pick.PickType = models.PickTypeOverUnder
					
					if game.HasOdds() {
						pick.PickDescription = fmt.Sprintf("%s @ %s - %s %.1f", game.Away, game.Home, teamName, game.Odds.OU)
					} else {
						pick.PickDescription = fmt.Sprintf("%s @ %s - %s", game.Away, game.Home, teamName)
					}
				} else {
					// Spread pick
					isHomeTeam := rand.Float64() < 0.5
					var teamName string
					var teamID int
					
					if isHomeTeam {
						teamName = game.Home
						if id, exists := teamIDMap[game.Home]; exists {
							teamID = id
						}
					} else {
						teamName = game.Away
						if id, exists := teamIDMap[game.Away]; exists {
							teamID = id
						}
					}
					
					pick = models.CreatePickFromLegacyData(user.ID, game.ID, teamID, 2025, week)
					pick.TeamName = teamName
					pick.PickType = models.PickTypeSpread
					
					if game.HasOdds() {
						var spreadDesc string
						if teamName == game.Home {
							spreadDesc = game.FormatHomeSpread()
						} else {
							spreadDesc = game.FormatAwaySpread()
						}
						pick.PickDescription = fmt.Sprintf("%s @ %s - %s %s", game.Away, game.Home, teamName, spreadDesc)
					} else {
						pick.PickDescription = fmt.Sprintf("%s @ %s - %s", game.Away, game.Home, teamName)
					}
				}
				
				// Simulate pick results for completed games
				if game.State == models.GameStateCompleted {
					// Random result with slight bias toward wins
					resultRand := rand.Float64()
					if resultRand < 0.45 {
						pick.Result = models.PickResultWin
					} else if resultRand < 0.85 {
						pick.Result = models.PickResultLoss
					} else {
						pick.Result = models.PickResultPush
					}
				} else {
					pick.Result = models.PickResultPending
				}
				
				weekPicks = append(weekPicks, pick)
			}
			
			// Save picks for this week
			if len(weekPicks) > 0 {
				err := pickRepo.CreateMany(ctx, weekPicks)
				if err != nil {
					log.Printf("Failed to create picks for user %s, week %d: %v", user.Name, week, err)
					continue
				}
				
				userPicksCreated += len(weekPicks)
				log.Printf("  Week %d: Created %d picks", week, len(weekPicks))
			}
		}
		
		totalPicksCreated += userPicksCreated
		log.Printf("User %s: Created %d total picks", user.Name, userPicksCreated)
	}
	
	log.Printf("✅ Successfully created %d total test picks for %d users", totalPicksCreated, len(users))
	log.Printf("Test picks populate complete!")
}