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
//	go run scripts/force_update_odds.go -season 2026 -week 2 -skip-weekday Thursday
//	go run scripts/force_update_odds.go -season 2026 -week 2 -skip-weekday Thursday -apply
package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"sort"
	"strconv"
	"strings"

	"nfl-app-go/config"
	"nfl-app-go/database"
	"nfl-app-go/logging"
	"nfl-app-go/models"
	"nfl-app-go/services"
)

// parseExcludedIDs turns "123,456" into a lookup set.
func parseExcludedIDs(raw string) (map[int]bool, error) {
	out := map[int]bool{}
	for _, part := range strings.Split(raw, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		id, err := strconv.Atoi(part)
		if err != nil {
			return nil, fmt.Errorf("invalid game id %q", part)
		}
		out[id] = true
	}
	return out, nil
}

// parseSkippedWeekdays turns "Thursday,Monday" into a lookup set. Day names are
// matched case-insensitively against Pacific-time kickoff, which is how the
// league reckons game days.
func parseSkippedWeekdays(raw string) (map[string]bool, error) {
	valid := map[string]bool{
		"sunday": true, "monday": true, "tuesday": true, "wednesday": true,
		"thursday": true, "friday": true, "saturday": true,
	}
	out := map[string]bool{}
	for _, part := range strings.Split(raw, ",") {
		part = strings.ToLower(strings.TrimSpace(part))
		if part == "" {
			continue
		}
		if !valid[part] {
			return nil, fmt.Errorf("invalid weekday %q", part)
		}
		out[part] = true
	}
	return out, nil
}

func titleCase(s string) string {
	if s == "" {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}

func main() {
	season := flag.Int("season", 0, "season year (required), e.g. 2026")
	week := flag.Int("week", 0, "week number (required), e.g. 2")
	apply := flag.Bool("apply", false, "write the changes; omit for a dry run")
	exclude := flag.String("exclude", "", "comma-separated ESPN game IDs to leave untouched")
	skipWeekday := flag.String("skip-weekday", "", "comma-separated weekday names to leave untouched (Pacific kickoff), e.g. Thursday")
	flag.Parse()

	if *season == 0 || *week == 0 {
		fmt.Fprintln(os.Stderr, "usage: force_update_odds -season YYYY -week N [-skip-weekday Thursday] [-exclude ID,ID] [-apply]")
		flag.PrintDefaults()
		os.Exit(2)
	}

	excludedIDs, err := parseExcludedIDs(*exclude)
	if err != nil {
		log.Fatalf("bad -exclude: %v", err)
	}
	skippedDays, err := parseSkippedWeekdays(*skipWeekday)
	if err != nil {
		log.Fatalf("bad -skip-weekday: %v", err)
	}

	fmt.Printf("Odds refresh - season %d week %d\n", *season, *week)
	if *apply {
		fmt.Println("mode: APPLY (database will be written)")
	} else {
		fmt.Println("mode: DRY RUN (no writes; re-run with -apply to commit)")
	}
	if len(skippedDays) > 0 {
		days := make([]string, 0, len(skippedDays))
		for d := range skippedDays {
			days = append(days, titleCase(d))
		}
		sort.Strings(days)
		fmt.Printf("holding: %s games (left at current lines)\n", strings.Join(days, ", "))
	}
	if len(excludedIDs) > 0 {
		fmt.Printf("holding: %d game(s) by ID\n", len(excludedIDs))
	}
	fmt.Println(strings.Repeat("=", 92))

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
	pacific := models.GetPacificTimeLocation()

	stored, err := gameRepo.GetGamesByWeekSeason(*week, *season)
	if err != nil {
		log.Fatalf("Failed to load games: %v", err)
	}
	if len(stored) == 0 {
		log.Fatalf("No games found for season %d week %d", *season, *week)
	}

	// Split held-back games out before hitting ESPN so they are never touched.
	var candidates []models.Game
	var held []models.Game
	for _, g := range stored {
		day := strings.ToLower(g.Date.In(pacific).Weekday().String())
		if excludedIDs[g.ID] || skippedDays[day] {
			held = append(held, *g)
			continue
		}
		candidates = append(candidates, *g)
	}
	sort.Slice(candidates, func(i, j int) bool { return candidates[i].Date.Before(candidates[j].Date) })
	sort.Slice(held, func(i, j int) bool { return held[i].Date.Before(held[j].Date) })

	before := make(map[int]*models.Odds, len(candidates))
	for i := range candidates {
		before[candidates[i].ID] = candidates[i].Odds
	}

	if len(held) > 0 {
		fmt.Println("HELD BACK (not fetched, not written):")
		for _, g := range held {
			line := "no odds"
			if g.Odds != nil {
				line = fmt.Sprintf("spread %.1f, O/U %.1f", g.Odds.Spread, g.Odds.OU)
			}
			fmt.Printf("  %-9d %-12s %-18s %s\n", g.ID, fmt.Sprintf("%s@%s", g.Away, g.Home),
				g.Date.In(pacific).Format("Mon 3:04 PM"), line)
		}
		fmt.Println()
	}

	if len(candidates) == 0 {
		fmt.Println("Every game was held back - nothing to do.")
		return
	}

	fmt.Printf("Fetching current odds for %d games from ESPN...\n\n", len(candidates))
	enriched := espnService.EnrichGamesWithOdds(candidates)

	fmt.Printf("%-9s %-12s %-11s %-22s %-22s %s\n", "ID", "MATCHUP", "KICKOFF", "SPREAD (old -> new)", "O/U (old -> new)", "STATUS")
	fmt.Println(strings.Repeat("-", 92))

	var toWrite []*models.Game
	changed, unchanged, missing, started := 0, 0, 0, 0

	for i := range enriched {
		g := enriched[i]
		matchup := fmt.Sprintf("%s@%s", g.Away, g.Home)
		kickoff := g.Date.In(pacific).Format("Mon 3:04PM")
		old := before[g.ID]

		switch {
		case g.Odds == nil:
			fmt.Printf("%-9d %-12s %-11s %-22s %-22s %s\n", g.ID, matchup, kickoff, "-", "-", "no odds returned")
			missing++
			continue
		case g.State != models.GameStateScheduled:
			// Never rewrite a line for a game already under way.
			fmt.Printf("%-9d %-12s %-11s %-22s %-22s %s\n", g.ID, matchup, kickoff, "-", "-",
				fmt.Sprintf("skipped (state=%s)", g.State))
			started++
			continue
		case old != nil && old.Spread == g.Odds.Spread && old.OU == g.Odds.OU:
			fmt.Printf("%-9d %-12s %-11s %-22s %-22s %s\n", g.ID, matchup, kickoff,
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
		fmt.Printf("%-9d %-12s %-11s %-22s %-22s %s\n", g.ID, matchup, kickoff,
			fmt.Sprintf("%s -> %.1f", oldSpread, g.Odds.Spread),
			fmt.Sprintf("%s -> %.1f", oldOU, g.Odds.OU), "WILL UPDATE")

		gameCopy := enriched[i]
		toWrite = append(toWrite, &gameCopy)
		changed++
	}

	fmt.Println(strings.Repeat("-", 92))
	fmt.Printf("%d to update, %d unchanged, %d already started, %d without odds, %d held back\n",
		changed, unchanged, started, missing, len(held))

	if len(toWrite) == 0 {
		fmt.Println("\nNothing to do.")
		return
	}

	if !*apply {
		fmt.Printf("\nDry run - nothing written. To commit, add -apply to the same command.\n")
		return
	}

	if err := gameRepo.BulkUpsertGames(toWrite); err != nil {
		log.Fatalf("Failed to write odds: %v", err)
	}
	fmt.Printf("\nUpdated odds for %d games in season %d week %d.\n", len(toWrite), *season, *week)
	fmt.Println("Picks for these games will now grade against the new lines.")
}
