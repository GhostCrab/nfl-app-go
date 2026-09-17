// force_update_odds is a one-off admin tool that refreshes odds for a single
// week, deliberately bypassing the Wednesday cutoff enforced by the background
// updater.
//
// Use it to repair a week whose odds went stale because of a cutoff bug. Note
// that stored spreads and totals grade already-submitted picks
// (result_calculation_service.go applies game.Odds.Spread), so refreshing a week
// that users have already picked will re-grade those picks against lines they
// never saw. It runs as a dry run unless -apply is passed.
//
//	go run scripts/force_update_odds.go -season 2026 -week 2
//	go run scripts/force_update_odds.go -season 2026 -week 2 -apply
package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"sort"
	"strings"

	"nfl-app-go/config"
	"nfl-app-go/database"
	"nfl-app-go/logging"
	"nfl-app-go/models"
	"nfl-app-go/services"
)

func main() {
	season := flag.Int("season", 0, "season year (required), e.g. 2026")
	week := flag.Int("week", 0, "week number (required), e.g. 2")
	apply := flag.Bool("apply", false, "write the changes; omit for a dry run")
	flag.Parse()

	if *season == 0 || *week == 0 {
		fmt.Fprintln(os.Stderr, "usage: force_update_odds -season YYYY -week N [-apply]")
		flag.PrintDefaults()
		os.Exit(2)
	}

	fmt.Printf("Odds refresh - season %d week %d\n", *season, *week)
	if *apply {
		fmt.Println("mode: APPLY (database will be written)")
	} else {
		fmt.Println("mode: DRY RUN (no writes; re-run with -apply to commit)")
	}
	fmt.Println(strings.Repeat("=", 78))

	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("Failed to load configuration: %v", err)
	}
	logging.Configure(cfg.ToLoggingConfig())

	db, err := database.NewMongoConnection(cfg.ToDatabaseConfig())
	if err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}
	defer db.Close()

	gameRepo := database.NewMongoGameRepository(db)
	espnService := services.NewESPNService()

	stored, err := gameRepo.GetGamesByWeekSeason(*week, *season)
	if err != nil {
		log.Fatalf("Failed to load games: %v", err)
	}
	if len(stored) == 0 {
		log.Fatalf("No games found for season %d week %d", *season, *week)
	}

	games := make([]models.Game, 0, len(stored))
	for _, g := range stored {
		games = append(games, *g)
	}
	sort.Slice(games, func(i, j int) bool { return games[i].Date.Before(games[j].Date) })

	before := make(map[int]*models.Odds, len(games))
	for i := range games {
		before[games[i].ID] = games[i].Odds
	}

	fmt.Printf("Fetching current odds for %d games from ESPN...\n\n", len(games))
	enriched := espnService.EnrichGamesWithOdds(games)

	fmt.Printf("%-12s %-22s %-22s %s\n", "MATCHUP", "SPREAD (old -> new)", "O/U (old -> new)", "STATUS")
	fmt.Println(strings.Repeat("-", 78))

	var toWrite []*models.Game
	changed, unchanged, missing, started := 0, 0, 0, 0

	for i := range enriched {
		g := enriched[i]
		matchup := fmt.Sprintf("%s@%s", g.Away, g.Home)
		old := before[g.ID]

		if g.Odds == nil {
			fmt.Printf("%-12s %-22s %-22s %s\n", matchup, "-", "-", "no odds returned")
			missing++
			continue
		}
		// Refuse to touch a game already under way - its odds are final.
		if g.State != models.GameStateScheduled {
			fmt.Printf("%-12s %-22s %-22s %s\n", matchup, "-", "-",
				fmt.Sprintf("skipped (state=%s)", g.State))
			started++
			continue
		}

		if old != nil && old.Spread == g.Odds.Spread && old.OU == g.Odds.OU {
			fmt.Printf("%-12s %-22s %-22s %s\n", matchup,
				fmt.Sprintf("%.1f (same)", old.Spread),
				fmt.Sprintf("%.1f (same)", old.OU), "unchanged")
			unchanged++
			continue
		}

		oldSpread, oldOU := "none", "none"
		if old != nil {
			oldSpread = fmt.Sprintf("%.1f", old.Spread)
			oldOU = fmt.Sprintf("%.1f", old.OU)
		}
		fmt.Printf("%-12s %-22s %-22s %s\n", matchup,
			fmt.Sprintf("%s -> %.1f", oldSpread, g.Odds.Spread),
			fmt.Sprintf("%s -> %.1f", oldOU, g.Odds.OU), "WILL UPDATE")

		gameCopy := enriched[i]
		toWrite = append(toWrite, &gameCopy)
		changed++
	}

	fmt.Println(strings.Repeat("-", 78))
	fmt.Printf("%d to update, %d unchanged, %d already started, %d without odds\n",
		changed, unchanged, started, missing)

	if len(toWrite) == 0 {
		fmt.Println("\nNothing to do.")
		return
	}

	if !*apply {
		fmt.Printf("\nDry run - nothing written. To commit:\n")
		fmt.Printf("  go run scripts/force_update_odds.go -season %d -week %d -apply\n", *season, *week)
		return
	}

	if err := gameRepo.BulkUpsertGames(toWrite); err != nil {
		log.Fatalf("Failed to write odds: %v", err)
	}
	fmt.Printf("\nUpdated odds for %d games in season %d week %d.\n", len(toWrite), *season, *week)
	fmt.Println("Picks for these games will now grade against the new lines.")
}
