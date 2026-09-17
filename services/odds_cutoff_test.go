package services

import (
	"testing"
	"time"

	"nfl-app-go/logging"
	"nfl-app-go/models"
)

// buildGames turns (week, RFC3339 kickoff) pairs into games.
func buildGames(t *testing.T, entries map[int][]string) []*models.Game {
	t.Helper()
	var games []*models.Game
	id := 1
	for week, kickoffs := range entries {
		for _, k := range kickoffs {
			ts, err := time.Parse(time.RFC3339, k)
			if err != nil {
				t.Fatalf("bad kickoff %q: %v", k, err)
			}
			games = append(games, &models.Game{ID: id, Week: week, Season: 2026, Date: ts})
			id++
		}
	}
	return games
}

func testUpdater() *BackgroundUpdater {
	return &BackgroundUpdater{currentSeason: 2026, logger: logging.WithPrefix("test")}
}

// Real 2026 kickoffs pulled from the games collection. Week 1 opens on a
// Wednesday; every other week opens Thursday.
func schedule2026(t *testing.T) []*models.Game {
	return buildGames(t, map[int][]string{
		1: {"2026-09-10T00:20:00Z", "2026-09-13T17:00:00Z", "2026-09-15T00:15:00Z"},
		2: {"2026-09-18T00:15:00Z", "2026-09-20T17:00:00Z", "2026-09-22T00:15:00Z"},
		3: {"2026-09-25T00:15:00Z", "2026-09-27T17:00:00Z", "2026-09-29T00:15:00Z"},
		4: {"2026-10-02T00:15:00Z", "2026-10-04T17:00:00Z", "2026-10-06T00:15:00Z"},
	})
}

func TestOddsCutoffForWeek(t *testing.T) {
	bu := testUpdater()
	games := schedule2026(t)
	pacific, err := time.LoadLocation("America/Los_Angeles")
	if err != nil {
		t.Fatalf("load pacific: %v", err)
	}

	tests := []struct {
		week int
		want string // Pacific wall-clock
		why  string
	}{
		{1, "2026-09-08 23:59:59", "Wednesday opener locks Tuesday midnight"},
		{2, "2026-09-16 22:00:00", "Wednesday 10 PM before Thursday opener"},
		{3, "2026-09-23 22:00:00", "Wednesday 10 PM before Thursday opener"},
		{4, "2026-09-30 22:00:00", "Wednesday 10 PM before Thursday opener"},
	}

	for _, tc := range tests {
		got, ok := bu.oddsCutoffForWeek(tc.week, games)
		if !ok {
			t.Errorf("week %d: expected a cutoff, got none", tc.week)
			continue
		}
		if g := got.In(pacific).Format("2006-01-02 15:04:05"); g != tc.want {
			t.Errorf("week %d: cutoff = %s, want %s (%s)", tc.week, g, tc.want, tc.why)
		}
	}
}

// The cutoff must never land after the week's first kickoff, or odds would stay
// editable into a live game. This is what the Wednesday special case protects.
func TestOddsCutoffAlwaysPrecedesFirstKickoff(t *testing.T) {
	bu := testUpdater()
	games := schedule2026(t)

	for week := 1; week <= 4; week++ {
		cutoff, ok := bu.oddsCutoffForWeek(week, games)
		if !ok {
			t.Fatalf("week %d: no cutoff", week)
		}

		var first time.Time
		for _, g := range games {
			if g.Week == week && (first.IsZero() || g.Date.Before(first)) {
				first = g.Date
			}
		}
		if !cutoff.Before(first) {
			t.Errorf("week %d: cutoff %s is not before first kickoff %s",
				week, cutoff, first)
		}
	}
}

func TestIsAfterOddsCutoff(t *testing.T) {
	bu := testUpdater()
	games := schedule2026(t)

	// A week with no scheduled games must not be treated as locked.
	if bu.isAfterOddsCutoff(17, games) {
		t.Error("week with no games should not be locked")
	}

	// Sanity: weeks whose cutoff is in the far future are not locked yet.
	if bu.isAfterOddsCutoff(4, games) && time.Now().Before(time.Date(2026, 9, 30, 22, 0, 0, 0, time.UTC)) {
		t.Error("week 4 locked before its cutoff")
	}
}

// 2026 has a second Wednesday kickoff: Thanksgiving Eve in week 12. The lock is
// driven by the schedule, not a hardcoded week number, so it is handled the same
// way as the season opener without any extra special-casing.
func TestOddsCutoffHandlesThanksgivingEveWednesday(t *testing.T) {
	bu := testUpdater()
	games := buildGames(t, map[int][]string{
		11: {"2026-11-20T01:15:00Z"},
		12: {"2026-11-26T01:00:00Z", "2026-11-26T18:00:00Z", "2026-11-27T20:00:00Z"},
		13: {"2026-12-04T01:15:00Z"},
	})
	pacific, err := time.LoadLocation("America/Los_Angeles")
	if err != nil {
		t.Fatalf("load pacific: %v", err)
	}

	got, ok := bu.oddsCutoffForWeek(12, games)
	if !ok {
		t.Fatal("expected a cutoff for week 12")
	}
	const want = "2026-11-24 23:59:59"
	if g := got.In(pacific).Format("2006-01-02 15:04:05"); g != want {
		t.Errorf("week 12 cutoff = %s, want %s (Tuesday before the Wednesday game)", g, want)
	}
}
