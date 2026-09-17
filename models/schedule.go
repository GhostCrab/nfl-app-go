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

type weekWindow struct {
	week  int
	start time.Time // first kickoff of that week
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
	firstKickoff := make(map[int]time.Time)
	for i := range games {
		g := &games[i]
		if g.Week <= 0 {
			continue
		}
		if existing, ok := firstKickoff[g.Week]; !ok || g.Date.Before(existing) {
			firstKickoff[g.Week] = g.Date
		}
	}

	if len(firstKickoff) == 0 {
		return
	}

	windows := make([]weekWindow, 0, len(firstKickoff))
	for week, start := range firstKickoff {
		windows = append(windows, weekWindow{week: week, start: start})
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

// weekFromSchedule resolves a date to a week using indexed kickoff times.
// A week owns the span from its first kickoff until the next week's first
// kickoff, so it stays current through its own final game. Dates before the
// season resolve to the first week, dates after it to the last.
func weekFromSchedule(season int, date time.Time) (int, bool) {
	scheduleMu.RLock()
	windows := scheduleIndex[season]
	scheduleMu.RUnlock()

	if len(windows) == 0 {
		return 0, false
	}

	if date.Before(windows[0].start) {
		return windows[0].week, true
	}

	// Last window whose start is at or before the date
	current := windows[0].week
	for _, w := range windows {
		if w.start.After(date) {
			break
		}
		current = w.week
	}
	return current, true
}
