package models

import (
	"sort"
	"sync"
	"time"
)

// The NFL no longer keeps a predictable season shape: openers have moved off
// Thursday (2026 opens on a Wednesday) and midweek holiday games are routine
// (Christmas 2024, Thanksgiving Eve 2026). Deriving week numbers from a calendar
// rule therefore drifts every few seasons and needs hand-patching.
//
// Instead we index the real schedule once it is loaded, and answer week lookups
// from actual kickoff times. The calendar heuristic remains only as a fallback
// for contexts with no schedule available (demo mode, tests, pre-load startup).
//
// Two different questions get asked of this index, and they have different
// answers:
//
//   - "which week does this date belong to?" - used to classify a game.
//     Answered by weekFromSchedule, which maps a date into the week whose games
//     surround it.
//   - "which week is the league on right now?" - used to pick the default view.
//     Answered by CurrentWeek, which rolls forward as soon as a week's games
//     finish, so people see the week they are picking rather than the one that
//     just ended.

// typicalGameDuration is how long after kickoff a game is assumed complete. Used
// only to decide when a week is over for CurrentWeek.
const typicalGameDuration = 4 * time.Hour

type weekWindow struct {
	week  int
	start time.Time // first kickoff of that week
	last  time.Time // last kickoff of that week
}

var (
	scheduleMu sync.RWMutex
	// season -> week windows, ascending by start
	scheduleIndex = make(map[int][]weekWindow)
)

// RegisterSchedule indexes a season's games so week lookups use real kickoff
// times rather than the calendar heuristic. Safe to call repeatedly; each call
// replaces that season's index. Games with week <= 0 are ignored.
func RegisterSchedule(season int, games []Game) {
	type span struct{ first, last time.Time }
	spans := make(map[int]span)

	for i := range games {
		g := &games[i]
		if g.Week <= 0 {
			continue
		}
		s, ok := spans[g.Week]
		if !ok {
			spans[g.Week] = span{first: g.Date, last: g.Date}
			continue
		}
		if g.Date.Before(s.first) {
			s.first = g.Date
		}
		if g.Date.After(s.last) {
			s.last = g.Date
		}
		spans[g.Week] = s
	}

	if len(spans) == 0 {
		return
	}

	windows := make([]weekWindow, 0, len(spans))
	for week, s := range spans {
		windows = append(windows, weekWindow{week: week, start: s.first, last: s.last})
	}
	sort.Slice(windows, func(i, j int) bool { return windows[i].start.Before(windows[j].start) })

	scheduleMu.Lock()
	scheduleIndex[season] = windows
	scheduleMu.Unlock()
}

// RegisterScheduleFromPtrs indexes a season's games when held as pointers,
// which is what the repository layer returns.
func RegisterScheduleFromPtrs(season int, games []*Game) {
	flat := make([]Game, 0, len(games))
	for _, g := range games {
		if g != nil {
			flat = append(flat, *g)
		}
	}
	RegisterSchedule(season, flat)
}

// HasSchedule reports whether a season's real schedule has been indexed.
func HasSchedule(season int) bool {
	scheduleMu.RLock()
	defer scheduleMu.RUnlock()
	return len(scheduleIndex[season]) > 0
}

func windowsFor(season int) []weekWindow {
	scheduleMu.RLock()
	defer scheduleMu.RUnlock()
	return scheduleIndex[season]
}

// weekFromSchedule resolves a date to the week whose games surround it. A week
// owns the span from its first kickoff until the next week's first kickoff.
// Dates before the season resolve to the first week, dates after it to the last.
//
// This answers "which week is this game in", so it deliberately does not roll
// forward early - use CurrentWeek for the default view instead.
func weekFromSchedule(season int, date time.Time) (int, bool) {
	windows := windowsFor(season)
	if len(windows) == 0 {
		return 0, false
	}

	if date.Before(windows[0].start) {
		return windows[0].week, true
	}

	current := windows[0].week
	for _, w := range windows {
		if w.start.After(date) {
			break
		}
		current = w.week
	}
	return current, true
}

// CurrentWeek returns the week the league is on at the given time: the earliest
// week that has not finished yet. It advances as soon as a week's last game
// wraps up, so during the gap between weeks people see the week they are about
// to pick rather than the one that just ended.
//
// Returns ok=false when the season has no indexed schedule.
func CurrentWeek(season int, now time.Time) (int, bool) {
	windows := windowsFor(season)
	if len(windows) == 0 {
		return 0, false
	}

	for _, w := range windows {
		if now.Before(w.last.Add(typicalGameDuration)) {
			return w.week, true
		}
	}

	// Season is over - stay on the final week.
	return windows[len(windows)-1].week, true
}

// CurrentWeekOrFallback is CurrentWeek with the calendar heuristic as a backstop
// for when no schedule has been indexed.
func CurrentWeekOrFallback(season int, now time.Time) int {
	if week, ok := CurrentWeek(season, now); ok {
		return week
	}
	return nflWeekFromCalendar(now, season)
}
