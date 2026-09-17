package models

import (
	"testing"
	"time"
)

func mustTime(t *testing.T, s string) time.Time {
	t.Helper()
	ts, err := time.Parse(time.RFC3339, s)
	if err != nil {
		t.Fatalf("bad time %q: %v", s, err)
	}
	return ts
}

func gamesAt(t *testing.T, season int, byWeek map[int]string) []Game {
	t.Helper()
	var games []Game
	id := 1
	for week, kickoff := range byWeek {
		games = append(games, Game{ID: id, Season: season, Week: week, Date: mustTime(t, kickoff)})
		id++
	}
	return games
}

// 2024 weeks 16-18. Week 17 opens on Christmas Day, a Wednesday - the case the
// calendar heuristic gets wrong (it reports week 16).
func TestWeekForDateHandlesMidweekHolidayGames(t *testing.T) {
	const season = 2024
	RegisterSchedule(season, gamesAt(t, season, map[int]string{
		16: "2024-12-20T01:15:00Z",
		17: "2024-12-25T18:00:00Z",
		18: "2025-01-04T21:30:00Z",
	}))

	christmas := mustTime(t, "2024-12-25T18:00:00Z")

	if got := GetNFLWeekForDate(christmas, season); got != 17 {
		t.Errorf("Christmas 2024 game: got week %d, want 17", got)
	}
	// Confirm this is a genuine fix, not a coincidence
	if got := nflWeekFromCalendar(christmas, season); got != 16 {
		t.Errorf("precondition: calendar heuristic should report 16, got %d", got)
	}
}

// 2026 opens on a Wednesday, so a Thursday-opener assumption misplaces week 1.
func TestWeekForDateHandlesWednesdayOpener(t *testing.T) {
	const season = 2026
	RegisterSchedule(season, gamesAt(t, season, map[int]string{
		1: "2026-09-10T00:20:00Z", // Wed Sep 9 PT
		2: "2026-09-18T00:15:00Z", // Thu Sep 17 PT
		3: "2026-09-25T00:15:00Z",
	}))

	tests := []struct {
		when string
		want int
		why  string
	}{
		{"2026-09-10T01:00:00Z", 1, "during the Wednesday opener"},
		{"2026-09-15T12:00:00Z", 1, "week 1 holds until week 2 kicks off"},
		{"2026-09-18T01:00:00Z", 2, "week 2 has started"},
		{"2026-08-01T00:00:00Z", 1, "before the season clamps to week 1"},
		{"2027-02-01T00:00:00Z", 3, "after the season clamps to the last week"},
	}
	for _, tc := range tests {
		if got := GetNFLWeekForDate(mustTime(t, tc.when), season); got != tc.want {
			t.Errorf("%s: got week %d, want %d (%s)", tc.when, got, tc.want, tc.why)
		}
	}
}

// With no schedule indexed the calendar heuristic must still answer, so demo
// mode and early startup keep working.
func TestFallsBackToCalendarWithoutSchedule(t *testing.T) {
	const season = 1999 // never registered

	if HasSchedule(season) {
		t.Fatalf("season %d should not be indexed", season)
	}
	if _, ok := weekFromSchedule(season, time.Now()); ok {
		t.Error("weekFromSchedule should report no schedule")
	}

	d := mustTime(t, "1999-09-23T00:00:00Z")
	if GetNFLWeekForDate(d, season) != nflWeekFromCalendar(d, season) {
		t.Error("expected fallback to the calendar heuristic")
	}
}

func TestRegisterScheduleIgnoresJunk(t *testing.T) {
	const season = 2031

	RegisterSchedule(season, []Game{{ID: 1, Season: season, Week: 0}})
	if HasSchedule(season) {
		t.Error("games with week <= 0 should not produce an index")
	}

	RegisterScheduleFromPtrs(season, []*Game{
		nil,
		{ID: 2, Season: season, Week: 1, Date: mustTime(t, "2031-09-11T00:20:00Z")},
	})
	if !HasSchedule(season) {
		t.Fatal("expected index from pointer slice")
	}
	if got := GetNFLWeekForDate(mustTime(t, "2031-09-12T00:00:00Z"), season); got != 1 {
		t.Errorf("got week %d, want 1", got)
	}
}
