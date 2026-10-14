# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## What this is

NFL Parlay Club — a server-rendered Go web app (MongoDB + HTMX + SSE) for a pick'em pool with **daily parlay scoring**: each game day's picks must form a winning parlay independently, rather than scoring a week as a whole. Module name is `nfl-app-go`; `go.mod` targets Go 1.25.

## Working constraints

These come from the project owner and override default instincts:

- **HTMX is the bible.** Follow [htmx.org/docs](https://htmx.org/docs/), the [SSE extension](https://htmx.org/extensions/sse/), and [hx-swap-oob](https://htmx.org/attributes/hx-swap-oob/). Review the SSE docs before changing real-time behavior.
- **Minimal JavaScript.** Server-side rendering and HTMX attributes over custom JS.
- **The owner runs the app and owns the port.** You may `go build` / `go run .` to verify it compiles and starts; the owner tests actual functionality. If you need to run it, use a different port or just check that startup logs are clean.
- **Search the whole codebase, unrestricted.** `grep -r "pattern" .` from the repo root, no path narrowing on the first pass.

## Commands

```bash
go build -o nfl-app .          # build the server (the root package only)
go run .                       # run; requires MongoDB + .env
air                            # hot reload (builds to ./tmp/main, watches go/html/tmpl)
go test ./models/... ./services/...                           # the only packages with tests
go test ./models/ -run TestCurrentWeekRollsForwardWhenAWeekFinishes -v   # single test
```

**`go build ./...` and `go vet ./...` fail repo-wide, and not because of your change.** Three packages are permanently broken:

- `cmd/` and `scripts/` each hold several standalone `package main` tools in one directory, so `main`/`getEnv` collide. Build or run them one file at a time: `go run scripts/manual_backup.go`, `go build -o scripts/restore_backup scripts/restore_backup.go`.
- `interfaces/compliance.go` asserts service implementations with `var _ Iface = (*impl)(nil)` against signatures in `interfaces/` that have drifted from `services/`. The package does not compile. Nothing imports it — `main.go` wires concrete types directly — so it is dead weight, not a contract. Don't trust it as a description of a service's API; read the service.

Always scope builds and vets to `.` plus the packages you touched. `go vet` also emits pre-existing warnings (unkeyed `primitive.E` literals, IPv6 address formats in `email_service.go`) — don't read those as regressions.

## Architecture

### Startup and wiring

[main.go](main.go) is the single composition root: ~390 lines that load config, connect Mongo, construct every repository/service/handler, register routes, and start background goroutines. There is no DI container, and handlers receive concrete `*services.X` types.

Two things about this wiring are easy to break:

1. **Order-dependent two-phase construction.** Services with circular dependencies are constructed bare, then wired via setters: `pickService.SetSpecializedServices(parlayService, resultCalcService, analyticsService)`, `SetMemoryScorer`, `SetBroadcaster`, `sseHandler.SetServices(...)`. `MemoryParlayScorer` must be built *after* `pickService.SetSpecializedServices`, and the scorer must be set on both `parlayService` and `pickService`. Adding a service means finding the right slot in this sequence.
2. **Graceful degradation to demo mode.** If the Mongo connection fails, `main` doesn't exit — it parses templates, spins up `services.NewDemoGameService()`, serves only `/` and `/games`, and returns. Demo mode has no picks, auth, or parlay scoring. Changes to the normal path often need a matching look at this early-return block.

Templates are parsed once at startup from `templates/*.html` plus `templates/partials/*.html`, with helpers from [templates/template_funcs.go](templates/template_funcs.go). Because they're parsed at boot, template edits need a restart (`air` handles this — it watches `.html`).

Config is centralized in [config/config.go](config/config.go) (`.env` via godotenv) and exposed to consumers through `To*Config()` adapter methods rather than passing `*Config` around.

### Data model: one document per user-week

The central collection is `weekly_picks`, and the shape drives much of the code. One `WeeklyPicks` document per `(user_id, season, week)` holds all of that user's picks as an **embedded `[]Pick` array** — picks are not their own collection. See [models/pick.go](models/pick.go).

Consequences worth knowing before you touch pick code:

- A `Pick` carries denormalized `UserID`/`Season`/`Week` copied down from its parent document, plus `bson:"-"` display fields (`TeamName`, `PickDescription`) that services populate at read time via `EnrichPickWithGameData`. A pick read straight from Mongo is not display-ready.
- Updating one pick means a positional update into the array, not a document write. `ReplacePicksForScheduledGames` deliberately replaces only picks whose games haven't started, preserving locked ones.
- `TeamID` is overloaded: a real ESPN team ID means a spread pick, while **98/99 mean under/over**. `DeterminePickTypeFromLegacyTeamID` is the one place that decodes this.
- `LegacyPick` is a separate flat shape used only by the legacy importer.

### Scoring: two eras

`models.IsModernSeason(season)` (`season >= 2025`, in [models/game.go](models/game.go)) forks the scoring model, and both paths are live because historical seasons are queryable:

- **Modern (2025+): daily parlays.** Picks are grouped by the game's *Pacific-timezone date* (`GroupPicksByDay`), and each day scores independently via `CalculateDailyParlayPoints`. Rules: minimum 2 picks for the day, any loss zeroes the day, any pending pick zeroes the day, pushes neither score nor fail but do count toward the 2-pick minimum, and points equal the number of winning picks.
- **Legacy: three categories.** `ParlayCategory` splits a week into `regular` (Sat/Sun/Mon), `bonus_thursday`, and `bonus_friday` (2024+), scored by `CalculateParlayPoints` with the same win/loss/push logic. `ParlayScore` persists these as `regular_points` / `bonus_thursday_points` / `bonus_friday_points`.

`MemoryParlayScorer` ([services/memory_parlay_scorer.go](services/memory_parlay_scorer.go)) keeps the current season's scores in memory so live SSE updates don't hit Mongo per broadcast; `InitializeClubScores` warms it at boot. Score mutations should go through the scorer so in-memory and broadcast state stay aligned.

`PickService` is the god object (~1200 lines) and delegates to `ParlayService`, `ResultCalculationService`, and `AnalyticsService` through the setter-injected fields. New scoring logic belongs in the specialized service, not in `PickService`.

### Week resolution: schedule index, not arithmetic

Don't compute an NFL week from the calendar. At boot, `main` calls `models.RegisterScheduleFromPtrs(currentSeason, games)` to index real kickoff times, and `models.CurrentWeek` / `GetNFLWeekForDate` ([models/schedule.go](models/schedule.go)) resolve weeks from that index. `nflWeekFromCalendar` is only a fallback for seasons with no indexed schedule — openers and holiday games move year to year, which is what the calendar heuristic gets wrong. [models/schedule_test.go](models/schedule_test.go) pins this behavior (Wednesday openers, midweek holiday games, week rollover).

### Pick visibility

Opponents' picks are hidden until a per-game deadline, so the view layer can never just render all picks. [models/pick_visibility.go](models/pick_visibility.go) defines the rules (`thursday_5pm_pt`, `weekend_10am_pt`, `thanksgiving_10am_pt`, `wednesday_midnight_pt`, plus in-progress/completed), all evaluated in `America/Los_Angeles`. `services.PickVisibilityService.FilterVisibleUserPicks` is the filter handlers call; `VisibilityTimerService` fires at each upcoming deadline and triggers an SSE pick-container refresh so the UI reveals picks without a reload. Both services support `SetDebugDateTime` for testing a fake "now".

**Two different types share the name `PickVisibilityService`** — `models.PickVisibilityService` (rule evaluation, `NewPickVisibilityService()` with no args) and `services.PickVisibilityService` (the handler-facing filter, `NewPickVisibilityService(gameService)`). Check the import before calling either. Note also that `FilterVisibleUserPicks` takes `(ctx, userPicks, season, week, viewingUserID)` — the `interfaces/` declarations put `viewingUserID` third, and they're wrong.

### Real-time: SSE + out-of-band swaps

[handlers/sse_handler.go](handlers/sse_handler.go) is the only broadcast path. It keeps a client registry, a message-ID counter, a heartbeat, and renders **HTML fragments** (not JSON) that HTMX swaps in. Event names are a contract with the templates' `sse-swap` attributes:

| Event | Broadcast by | Consumed in |
|---|---|---|
| `pick-state-updated` | `BroadcastPickStateUpdate` | [templates/dashboard.html:53](templates/dashboard.html#L53) |
| `game-state-update` | `broadcastConsolidatedGameUpdate` | dashboard, `partials/games.html` |
| `pick-item-updated` | `broadcastPickUpdatesForGame` | dashboard |
| `parlay-scores-updated` | `BroadcastParlayScoreUpdate` | dashboard |
| `pick-container-refresh` | `BroadcastPickContainerRefresh` | dashboard (visibility reveals) |
| `visibility-change` | `VisibilityTimerService` | dashboard |
| `heartbeat` | `sendHeartbeat` | dashboard |

Renaming an event means editing both sides. Fragments pass through `compactHTMLForSSE` because SSE frames can't carry raw newlines.

**The `UseOOBSwap` flag.** Shared templates like `user-picks-block` are rendered both by normal navigation and by SSE. The template emits `hx-swap-oob="true"` only when the data dict sets `UseOOBSwap` (true from the SSE handler, false/absent from page handlers). Getting this backwards empties the picks container during week switching, because OOB markup collides with the `dashboard-content` swap target. See the comment at [templates/dashboard.html:208](templates/dashboard.html#L208).

Changes also arrive from Mongo itself: `services.ChangeStreamWatcher` watches collections and routes events to `sseHandler.HandleDatabaseChange`, so a write from any process — including the background updater or a CLI script — fans out to connected browsers.

### Background work

All started from `main` and all optional via env flags:

- **`BackgroundUpdater`** (`BACKGROUND_UPDATER_ENABLED`) polls ESPN on an *adaptive* interval chosen by `getIntelligentUpdateInterval`: 5s with live games, 10s within 15 min of kickoff, 30s pre-game, 15s when a game should have started but hasn't, 2 min on game day and in-season, 30 min off-season, with a 30-min full sync. It also enriches odds and triggers parlay scoring when a week or category completes. Odds stop updating after a per-week cutoff (`oddsCutoffForWeek`, pinned by [services/odds_cutoff_test.go](services/odds_cutoff_test.go)).
- **`MockBackgroundUpdater`** (`MOCK_UPDATER_ENABLED`) fabricates game updates every 10s for testing the live UI without ESPN.
- **`BackupService`** (`BACKUP_ENABLED`) dumps `weekly_picks` and `games` nightly at `BACKUP_TIME` with `BACKUP_RETENTION_DAYS` retention.
- **`PickReminderService`** emails users missing picks — 12 PM PT weekdays, 8 AM PT weekends.

### Auth and routing

JWT in an `auth_token` cookie, verified by [middleware/auth.go](middleware/auth.go). Routes are `gorilla/mux`, and the distinction matters: `OptionalAuth` wraps public pages that merely personalize when signed in (`/`, `/games`, `/events`, `/analytics`, `/odds-details`), while `RequireAuth` guards `/pick-picker`, `/submit-picks`, and the `/api` subrouter. Users are seeded on first run by `UserSeeder`.

## Season rollover

`CURRENT_SEASON` in `.env` drives most things, but three hardcoded season arrays must also be extended or the new year silently drops out of imports, scoring, and queries:

- `services/legacy_import.go` (two places)
- `services/memory_parlay_scorer.go`
- `services/pick_service.go`

The README's "Annual Season Updates" section has the full checklist and current line numbers.

## Agent skills

### Issue tracker

Issues and specs live as markdown files under `.scratch/<feature-slug>/` in this repo. See [docs/agents/issue-tracker.md](docs/agents/issue-tracker.md).

### Triage labels

The five canonical triage roles, used verbatim as `Status:` values. See [docs/agents/triage-labels.md](docs/agents/triage-labels.md).

### Domain docs

Single-context: `GLOSSARY.md` and `docs/adr/` at the repo root. See [docs/agents/domain.md](docs/agents/domain.md).
