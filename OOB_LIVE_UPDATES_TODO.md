# OOB Live Updates TODO & Discovery Log

## Critical Issues Identified

### 1. **FIXED: SSE Pick Updates Data Processing Inconsistency**
**Status**: ✅ FIXED (2025-09-27)
**Discovery Date**: 2025-01-28
**File**: `handlers/sse_handler.go:BroadcastPickUpdate()` (lines ~243-330)

**Problem**: SSE pick updates used completely different data processing logic than initial dashboard load, causing:
- Missing users when someone updates picks
- No daily grouping for 2025+ seasons in SSE updates
- Inconsistent user ordering and display structure
- Template rendering differences between initial load vs live updates

**Root Cause**:
- **Initial Load**: Calls `GetAllUserPicksForWeek()` → gets all users' picks
- **SSE Updates**: Calls `GetUserPicksForWeek()` → gets only ONE user's picks

**Solution Implemented**: Completely rewrote `BroadcastPickUpdate()` to match dashboard logic exactly:

```go
// CRITICAL FIX: Get ALL users' picks (same as dashboard initial load)
allUserPicks, err := h.pickService.GetAllUserPicksForWeek(context.Background(), season, week)

// CRITICAL FIX: Get all users to ensure empty entries exist (matches dashboard logic)
users, err := h.userRepo.GetAllUsers()

// CRITICAL FIX: Ensure all users have a pick entry, even if empty (matches dashboard logic)
userPicksMap := make(map[int]*models.UserPicks)
for _, up := range allUserPicks {
    userPicksMap[up.UserID] = up
}

// Add empty pick entries for users who don't have picks this week
for _, u := range users {
    if _, exists := userPicksMap[u.ID]; !exists {
        emptyUserPicks := &models.UserPicks{
            UserID:   u.ID,
            UserName: u.Name,
            Picks:    []models.Pick{},
            Record: models.UserRecord{
                Wins:   0,
                Losses: 0,
                Pushes: 0,
            },
        }
        allUserPicks = append(allUserPicks, emptyUserPicks)
        userPicksMap[u.ID] = emptyUserPicks
    }
}

// CRITICAL FIX: Populate DailyPickGroups for ALL modern seasons (matches dashboard logic)
up.PopulateDailyPickGroups(weekGames, season)
```

**Additional Changes**:
- Added `userRepo *database.MongoUserRepository` to SSEHandler struct
- Updated `SetServices()` method to accept userRepo parameter
- Updated `main.go` to pass userRepo to SSE handler: `sseHandler.SetServices(pickService, authService, visibilityService, memoryScorer, userRepo)`

**Result**: SSE pick updates now use identical data processing logic as dashboard initial loads. All users remain visible during live updates, daily grouping works correctly, and template consistency is maintained between initial loads and SSE updates.

---

## Live Update System Architecture

### Current SSE + HTMX OOB Setup ✅
- **SSE Handler**: Comprehensive OOB updates for game states, pick updates, scores
- **Dashboard Template**: Complete SSE listener setup with multiple event types
- **HTMX Integration**: Proper SSE extension usage with `hx-swap-oob="true"`
- **Change Streams**: MongoDB triggers → SSE broadcasts automatically
- **Message Sequencing**: Atomic counters prevent race conditions
- **Connection Resilience**: Auto-reconnect and refresh on gaps

### Key Strengths ✅
1. Proper HTMX OOB usage for targeted element updates
2. User context and security filtering for pick visibility
3. Message ordering with atomic IDs
4. Connection resilience with gap detection
5. Filtered change streams for performance

---

## Implementation Status

### ✅ Completed Live Update Fixes (2025-01-28, 2025-09-27)
- [x] **Analyzed current SSE + HTMX OOB architecture** - Found sophisticated system already in place
- [x] **Fixed pick enrichment for completed/in-progress games** - Added automatic result calculation
- [x] **Fixed multiple SSE events on pick submission** - Ensured single event per submission
- [x] **Built complete club score live updates pipeline** - Pick changes → MemoryParlayScorer → SSE broadcast
- [x] **Fixed MemoryParlayScorer not updating on individual pick changes** - Added immediate recalculation
- [x] **Optimized SSE broadcasting** - Only when scores actually change for completed games
- [x] **COMPLETED: Fixed SSE BroadcastPickUpdate() data processing inconsistency** - Now matches dashboard logic exactly
- [x] **COMPLETED: Fixed pick live-expansion SSE updates** - Hidden class removal, content updates, consolidated events
- [x] **COMPLETED: Fixed pick result calculation on game state transitions** - Pick styling now reflects current game state

### 🔴 Critical Fixes Still Needed
- [x] **COMPLETED: Fix SSE BroadcastPickUpdate() to match dashboard data processing**
  - ✅ Use GetAllUserPicksForWeek() instead of single user
  - ✅ Add PopulateDailyPickGroups() for modern seasons
  - ✅ Ensure all users have pick entries
  - ✅ Match exact filtering and enrichment logic

### 9. **FIXED: Pick Live-Expansion SSE Updates**
**Status**: ✅ FIXED (2025-09-27)
**Discovery Date**: 2025-09-27
**Files**:
- `templates/dashboard.html:pick-item-update` template (lines ~347-531)
- `handlers/sse_handler.go:broadcastPickUpdatesForGame()` (lines ~566-618)

**Problems Fixed**:
1. **Hidden class not removed** when games transition to in_play
2. **Live-expansion content not updating** during game progress
3. **Over-granular SSE events** - clock, possession, status sent separately
4. **Pick-item styling not updating** on game state transitions

**Root Cause**: Pick updates were using over-granular approach with separate events for clock, possession, and status updates, and the live-expansion div didn't have `hx-swap-oob="true"` like game templates.

**Solution Implemented**:

**1. Fixed Template to Match Game Pattern**:
```html
<!-- CRITICAL FIX: Always present pick expansion with OOB swap, visibility controlled by state -->
<div class="live-pick-expansion {{if ne $game.State "in_play"}}hidden{{end}}"
     id="live-expansion-{{$pick.UserID}}-{{$pick.GameID}}-{{$pick.TeamID}}"
     hx-swap-oob="true">
```

**2. Consolidated SSE Updates**:
- **Before**: Separate events for `game-clock-update`, `possession-update`, `pick-status-update`
- **After**: Single `pick-item-updated` event with complete pick-item AND live-expansion update

**3. Updated SSE Handler**:
```go
// CRITICAL: Now uses consolidated approach like game updates - single broadcast with all pick updates
func (h *SSEHandler) broadcastPickUpdatesForGame(game *models.Game) {
    // Collect all picks for this game into a single consolidated update
    htmlBuffer := &strings.Builder{}

    for _, userPicks := range allUserPicks {
        for _, pick := range userPicks.Picks {
            if pick.GameID == game.ID {
                // Render the pick-item-update template (includes both pick-item AND live-expansion with OOB)
                h.templates.ExecuteTemplate(htmlBuffer, "pick-item-update", templateData)
            }
        }
    }

    // Send single consolidated update with all pick updates for this game
    h.BroadcastToAllClients("pick-item-updated", htmlBuffer.String())
}
```

**4. Template Listener Simplification**:
```html
<!-- Consolidated pick update listener - handles both pick-item AND live-expansion updates -->
<div sse-swap="pick-item-updated" hx-swap="none"></div>
```

**Result**: Pick live-expansions now work exactly like game live-expansions:
- ✅ **Hidden class properly removed/added** on game state transitions
- ✅ **Live content updates** during game progress (clock, possession, red zone, O/U projection)
- ✅ **Pick styling updates** on game state changes (winning/losing classes)
- ✅ **Single consolidated SSE event** instead of multiple granular updates
- ✅ **Consistent behavior** with game row updates

### 10. **FIXED: Pick Result Calculation on Game State Transitions**
**Status**: ✅ FIXED (2025-09-27)
**Discovery Date**: 2025-09-27
**Files**:
- `handlers/sse_handler.go:handleGameCompletion()` (lines ~796-863)
- `services/pick_service.go:ResetPickResultsForGame()` (lines ~1137-1170)

**Problem**: Green-pick-class styling persisted on in-progress picks even after fresh reload when games transitioned from completed back to in-progress. This was caused by **pick.Result fields not being recalculated** when game states changed.

**Root Cause**: The SSE `handleGameCompletion` method only handled **in-memory parlay score recalculation** but missed the crucial step of **updating pick results in the database** when game states changed.

**Solution Implemented**:

**1. Enhanced SSE Game State Handler**:
```go
// CRITICAL: Update pick results in database based on new game state
if game.IsCompleted() {
    // Game completed - calculate pick results
    if err := h.pickService.ProcessGameCompletion(ctx, game); err != nil {
        logger.Errorf("Failed to process game completion for game %s: %v", gameID, err)
    }
} else {
    // Game transitioned to non-completed state (in_play or scheduled) - reset pick results to pending
    if err := h.pickService.ResetPickResultsForGame(ctx, game); err != nil {
        logger.Errorf("Failed to reset pick results for game %s: %v", gameID, err)
    }
}
```

**2. New ResetPickResultsForGame Method**:
```go
// ResetPickResultsForGame resets all pick results for a specific game back to pending
// Used when games transition from completed back to in_play or scheduled states
func (s *PickService) ResetPickResultsForGame(ctx context.Context, game *models.Game) error {
    // Get all picks for this week to find picks for this specific game
    allPicks, err := s.GetAllPicksForWeek(ctx, game.Season, game.Week)

    // Filter picks for this specific game and collect users who need updates
    pickResults := make(map[int]models.PickResult)
    for _, pick := range allPicks {
        if pick.GameID == game.ID && pick.Result != models.PickResultPending {
            pickResults[pick.UserID] = models.PickResultPending
        }
    }

    // Update all pick results in one batch operation
    return s.UpdatePickResultsByGame(ctx, game.Season, game.Week, game.ID, pickResults)
}
```

**Result**: Pick styling now correctly reflects game state transitions:
- ✅ **Game completed → in_progress**: Green styling removed, picks show as in-progress
- ✅ **Game in_progress → completed**: Pick results calculated, proper win/loss styling applied
- ✅ **Game scheduled → in_progress**: Picks remain pending (neutral styling)
- ✅ **Database pick.Result fields** always match current game state
- ✅ **Template getResultClass function** gets correct data for styling decisions

**Note**: This fix specifically addresses **pick result database updates** and does not touch club score storage (which remains in-memory only as intended).

### 10a. **CRITICAL FIX: Multiple Picks Per Game Database Update Bug**
**Status**: ✅ FIXED (2025-09-27)
**Discovery**: While testing fix #10, ATS picks weren't getting updated but O/U picks were.
**File**: `database/mongo_weekly_picks_repository.go:UpdatePickResults()` (line ~247)

**Root Cause**: Database method had a `break` statement that only updated the **first pick found** for each game, so users with both ATS + O/U picks for the same game would only get one pick updated.

```go
// BEFORE (BROKEN):
for i := range weeklyPicks.Picks {
    if weeklyPicks.Picks[i].GameID == gameID {
        weeklyPicks.Picks[i].Result = newResult
        break  // ⚠️ Only updates first pick found!
    }
}

// AFTER (FIXED):
for i := range weeklyPicks.Picks {
    if weeklyPicks.Picks[i].GameID == gameID {
        weeklyPicks.Picks[i].Result = newResult
        // CRITICAL FIX: Don't break - user may have multiple picks (ATS + O/U) for same game
    }
}
```

**Result**: Now **both ATS and O/U picks** get updated when game states change, fixing the inconsistent styling issue.

### 10b. **CRITICAL FIX: Individual Pick Result Calculation Bug**
**Status**: ✅ FIXED (2025-09-27)
**Discovery**: All picks for completed games were getting same result (all wins) instead of individual calculation.
**Files**:
- `services/pick_service.go:ProcessGameCompletion()` (lines ~425-447)
- `services/pick_service.go:ResetPickResultsForGame()` (lines ~1154-1178)

**Root Cause**: Both methods were using `UpdatePickResults(map[int]models.PickResult)` which applied **one result per user**, but each pick (ATS vs O/U) should be calculated **individually**.

**Example Issue**:
- User has Under pick → Game goes Over → Pick should be Loss
- User has ATS pick → Team covers → Pick should be Win
- **Before**: Both picks got same result (both Win or both Loss)
- **After**: Each pick calculated individually

**Solution**: Switched to `UpdateIndividualPickResults([]PickUpdate)` method:

```go
// BEFORE (BROKEN):
pickResults := make(map[int]models.PickResult)
for _, pick := range picks {
    result := s.resultCalcService.CalculatePickResult(&pick, game)
    pickResults[pick.UserID] = result  // ⚠️ One result per user
}
s.weeklyPicksRepo.UpdatePickResults(ctx, season, week, gameID, pickResults)

// AFTER (FIXED):
var pickUpdates []database.PickUpdate
for _, pick := range picks {
    result := s.resultCalcService.CalculatePickResult(&pick, game)
    pickUpdate := database.PickUpdate{
        UserID:   pick.UserID,
        PickType: string(pick.PickType),  // ✅ Individual pick type
        Result:   result,                 // ✅ Individual result
    }
    pickUpdates = append(pickUpdates, pickUpdate)
}
s.weeklyPicksRepo.UpdateIndividualPickResults(ctx, season, week, gameID, pickUpdates)
```

**Result**: Each pick now gets its **correct individual result** based on its specific pick type and game outcome.

---

### 🟡 Enhancement Opportunities
- [ ] Game grid updates - Currently uses complete refresh for game state changes
- [ ] Error handling - More granular SSE error recovery


### 2. **FIXED: Pick Enrichment for Completed/In-Progress Games**
**Status**: ✅ FIXED
**Discovery Date**: 2025-01-28
**File**: `services/pick_service.go:UpdateUserPicksForScheduledGames()` (lines ~818-832)

**Problem**: When picks were updated for games that were already completed or in-progress, the pick results weren't being calculated, leaving them in "pending" state even though the game outcome was known.

**Root Cause**: The pick update pipeline (`UpdateUserPicksForScheduledGames`) was missing the enrichment step that `full_database_refresh.go` includes for ingested picks.

**Solution Implemented**:
```go
// CRITICAL: Trigger pick enrichment for any updated picks on completed/in-progress games
// This ensures pick results are calculated when picks are updated for games that are already done
if s.resultCalcService != nil {
    for _, pick := range newPicksSlice {
        if game, exists := gameMap[pick.GameID]; exists {
            if game.State == models.GameStateCompleted || game.State == models.GameStateInPlay {
                logger.Debugf("Triggering pick enrichment for updated pick on %s game %d", game.State, pick.GameID)
                if err := s.resultCalcService.ProcessGameCompletion(ctx, &game); err != nil {
                    logger.Warnf("Failed to process completed game %d after pick update: %v", pick.GameID, err)
                    // Continue with other picks rather than failing completely
                }
            }
        }
    }
}
```

**Matches Pattern From**: `services/legacy_import.go` line 289-297 where similar enrichment is triggered after importing picks.

### 3. **FIXED: Multiple SSE Events on Pick Submission**
**Status**: ✅ FIXED
**Discovery Date**: 2025-01-28
**File**: `services/pick_service.go:UpdateUserPicksForScheduledGames()` (lines ~810-832)

**Problem**: Pick submission was generating multiple SSE events instead of one bulk update, causing excessive SSE traffic.

**Root Cause**: The pick enrichment fix was calling `ProcessGameCompletion()` which triggered additional database operations via `UpdateIndividualPickResults()`, each generating separate change stream events.

**Flow That Was Broken**:
1. ✅ `Upsert()` for main pick submission → 1 SSE event
2. ❌ `ProcessGameCompletion()` → calls `UpdateIndividualPickResults()` → additional SSE events
3. ❌ Each result update → separate MongoDB changes → more SSE events

**Solution Implemented**: Calculate pick results **in-memory** before the main database operation:
```go
// CRITICAL: Calculate pick results in-memory for completed/in-progress games BEFORE database update
// This ensures pick results are calculated but included in the single upsert operation
if s.resultCalcService != nil {
    for i := range newPicksSlice {
        pick := &newPicksSlice[i]
        if game, exists := gameMap[pick.GameID]; exists {
            if game.State == models.GameStateCompleted || game.State == models.GameStateInPlay {
                result := s.resultCalcService.CalculatePickResult(pick, &game)
                pick.Result = result
            }
        }
    }
}
// Replace picks for scheduled games (now with calculated results)
existingWeeklyPicks.ReplacePicksForScheduledGames(newPicksSlice, scheduledGameIDs)
// Single upsert operation - this will trigger only ONE change stream event!
```

**Result**: Pick submission now triggers exactly **one SSE event** while still calculating results for completed games.

### 4. **FIXED: Club Score Live Updates Pipeline**
**Status**: ✅ FIXED
**Discovery Date**: 2025-01-28
**Files**:
- `services/parlay_service.go` - Added SSE broadcasting after scoring
- `handlers/sse_handler.go:BroadcastParlayScoreUpdate()` - Rebuilt from scratch
- `main.go` - Wired up parlay service broadcaster

**Problem**: Club scores weren't updating live after pick changes. The complete pipeline was:
1. ✅ Pick update → Pick results calculated
2. ✅ MemoryParlayScorer notified via `checkAndTriggerScoring()`
3. ❌ **MISSING**: ParlayService didn't notify SSE handler after scoring
4. ❌ **BROKEN**: `BroadcastParlayScoreUpdate()` used outdated cumulative scoring logic

**Root Cause**:
- ParlayService had no SSE broadcaster reference
- BroadcastParlayScoreUpdate() had TODO comment about cumulative scoring and was out of sync

**Solution Implemented**:

**1. Added SSE Broadcasting to ParlayService:**
```go
type ParlayScoreBroadcaster interface {
    BroadcastParlayScoreUpdate(season, week int)
}

type ParlayService struct {
    // ... existing fields
    broadcaster ParlayScoreBroadcaster
}

func (s *ParlayService) SetBroadcaster(broadcaster ParlayScoreBroadcaster) {
    s.broadcaster = broadcaster
}

// In ProcessParlayCategory():
if s.broadcaster != nil {
    s.broadcaster.BroadcastParlayScoreUpdate(season, week)
}
```

**2. Rebuilt BroadcastParlayScoreUpdate() to match current MemoryParlayScorer logic:**
```go
// Populate parlay scores using the same logic as PickService (CRITICAL for consistency)
for _, up := range userPicks {
    // Get season total up to current week for ParlayPoints (cumulative)
    seasonTotal := h.memoryScorer.GetUserSeasonTotal(season, week, up.UserID)
    up.Record.ParlayPoints = seasonTotal

    // Get current week's points for WeeklyPoints display
    if weekParlayScore, exists := h.memoryScorer.GetUserScore(season, week, up.UserID); exists {
        up.Record.WeeklyPoints = weekParlayScore.TotalPoints
    } else {
        up.Record.WeeklyPoints = 0
    }
}
```

**3. Wired up in main.go:**
```go
parlayService.SetBroadcaster(sseHandler)
```

**Complete Pipeline Now Working**:
1. ✅ Pick submission → Pick results calculated
2. ✅ `checkAndTriggerScoring()` calls `ProcessParlayCategory()`
3. ✅ `ProcessParlayCategory()` calls `broadcaster.BroadcastParlayScoreUpdate()`
4. ✅ `BroadcastParlayScoreUpdate()` renders updated club scores with correct cumulative logic
5. ✅ SSE sends `parlay-scores-updated` event with OOB swap to clients
6. ✅ Template `club-scores` div updates live

**Result**: Club scores now update in real-time after pick submissions that change parlay results.

### 5. **FIXED: MemoryParlayScorer Not Updating on Individual Pick Changes**
**Status**: ✅ FIXED
**Discovery Date**: 2025-01-28
**File**: `services/pick_service.go:UpdateUserPicksForScheduledGames()` (lines ~837-866)

**Problem**: MemoryParlayScorer wasn't updating when individual picks changed for completed games. Club scores only updated on service restart, not on pick changes or even page refresh.

**Root Cause**: `checkAndTriggerScoring()` only triggers MemoryParlayScorer when **ALL games in a parlay category are completed**. When you update picks for individual completed games, other games in the same category are still scheduled/in-progress, so:
- `allCompleted = false` for the category
- MemoryParlayScorer never gets `RecalculateUserScore()` called
- Club scores remain stale until service restart

**Example Scenario**:
- Change pick for completed SEA @ ARI game (Sunday category)
- Other Sunday games still pending → Category not "complete"
- No MemoryParlayScorer recalculation triggered
- Club scores don't update

**Solution Implemented**: Added immediate MemoryParlayScorer recalculation for completed game pick changes:

```go
// CRITICAL: Immediately trigger MemoryParlayScorer recalculation if any picks changed for completed games
// This ensures club scores update even when not all category games are complete
if s.memoryScorer != nil {
    hasCompletedGamePicks := false
    for _, pick := range newPicksSlice {
        if game, exists := gameMap[pick.GameID]; exists {
            if game.State == models.GameStateCompleted {
                hasCompletedGamePicks = true
                break
            }
        }
    }

    if hasCompletedGamePicks {
        logger.Infof("Recalculating MemoryParlayScorer for user %d due to completed game pick changes", userID)
        _, err := s.memoryScorer.RecalculateUserScore(ctx, season, week, userID)
        if err != nil {
            logger.Warnf("Failed to recalculate memory parlay score for user %d: %v", userID, err)
        } else {
            // Trigger SSE broadcast of updated club scores using ParlayService
            if s.parlayService != nil {
                s.parlayService.TriggerScoreBroadcast(season, week)
            }
        }
    }
}
```

**Added to ParlayService**: `TriggerScoreBroadcast()` method for manual SSE triggering.

**New Flow**:
1. ✅ Pick submission with completed game changes
2. ✅ Pick results calculated in-memory
3. ✅ **NEW**: Immediate `RecalculateUserScore()` for affected user
4. ✅ **NEW**: Immediate `TriggerScoreBroadcast()` → SSE update
5. ✅ Club scores update live immediately
6. ✅ Also runs normal `checkAndTriggerScoring()` for category completion

**Result**: Club scores now update immediately when picks change for completed games, regardless of whether the entire parlay category is complete.

### 6. **OPTIMIZED: SSE Club Score Broadcasting - Only When Needed**
**Status**: ✅ OPTIMIZED
**Discovery Date**: 2025-01-28
**File**: `services/pick_service.go:UpdateUserPicksForScheduledGames()` (lines ~837-891)

**Problems Found**:
1. **SSE events triggering for ALL pick changes** (should only be completed games)
2. **SSE events when no actual score change** (losing parlay → losing parlay)

**Root Cause**: Previous fix was too broad - triggering SSE for any pick changes, and not checking if scores actually changed.

**Optimizations Implemented**:

**1. Only Trigger for Completed Games** (already working):
```go
// Only process if picks changed for completed games
for _, pick := range newPicksSlice {
    if game, exists := gameMap[pick.GameID]; exists {
        if game.State == models.GameStateCompleted {
            hasCompletedGamePicks = true
            break
        }
    }
}
```

**2. Only Broadcast When Scores Actually Change**:
```go
// Get the current score before recalculation to detect changes
var beforeScore *models.ParlayScore
if currentScore, exists := s.memoryScorer.GetUserScore(season, week, userID); exists {
    beforeScore = currentScore
}

_, err := s.memoryScorer.RecalculateUserScore(ctx, season, week, userID)

// Only broadcast SSE if the score actually changed
var afterScore *models.ParlayScore
if newScore, exists := s.memoryScorer.GetUserScore(season, week, userID); exists {
    afterScore = newScore
}

// Compare scores to detect actual changes
scoreChanged := false
if beforeScore == nil && afterScore != nil {
    scoreChanged = true
} else if beforeScore != nil && afterScore == nil {
    scoreChanged = true
} else if beforeScore != nil && afterScore != nil {
    scoreChanged = beforeScore.TotalPoints != afterScore.TotalPoints
}

if scoreChanged {
    logger.Infof("Club score changed for user %d: %v → %v, broadcasting SSE update",
        userID, getScorePoints(beforeScore), getScorePoints(afterScore))
    s.parlayService.TriggerScoreBroadcast(season, week)
} else {
    logger.Debugf("Club score unchanged for user %d (%v), skipping SSE broadcast",
        userID, getScorePoints(afterScore))
}
```

**Optimized Behavior**:
- ✅ **Only recalculates** when picks change for completed games
- ✅ **Only broadcasts SSE** when TotalPoints actually change
- ✅ **Skips SSE** when losing parlay → losing parlay (same points)
- ✅ **Detailed logging** showing before/after scores and decision

**Result**: Eliminates unnecessary SSE traffic while maintaining real-time updates when scores actually change.

### 7. **FIXED: Replaced All Trash getCurrentWeek Functions**
**Status**: ✅ FIXED
**Discovery Date**: 2025-01-28
**Files**:
- `services/mock_background_updater.go`
- `handlers/game_display_handler.go`
- `handlers/pick_management_handler.go`

**Problem**: Found 3 different `getCurrentWeek()` functions throughout the codebase, all using different (incorrect) logic for determining the current NFL week. Mock background updater was updating week 3 instead of week 4.

**Root Cause**: Each function had custom hardcoded date logic that was:
- **Inconsistent** between different parts of the codebase
- **Incorrect** for current dates (Sept 27 → Week 3 instead of Week 4)
- **Unreliable** with seasonal timing variations

**Solution**: Replaced all 3 functions to use the proper `models.GetNFLWeekForDate()`:

**Before (Mock Background Updater)**:
```go
// Hardcoded September date logic
if month == 9 {
    if day < 8 { return 1 }
    else if day < 15 { return 2 }
    else { return 3 }  // ❌ Sept 27 → Week 3
}
```

**Before (Game Display Handler)**: 55-line function with complex game timing analysis

**Before (Pick Management Handler)**: Game completion-based week detection

**After (All Functions)**:
```go
// Use the proper week calculation that accounts for NFL season timing
if len(games) > 0 {
    return models.GetNFLWeekForDate(time.Now(), games[0].Season)
}
return models.GetNFLWeekForDate(time.Now(), time.Now().Year())
```

**`GetNFLWeekForDate()` Advantages**:
- ✅ **Accurate**: Calculates based on actual NFL season start (Thursday after Labor Day)
- ✅ **Consistent**: Single source of truth for week calculation
- ✅ **Reliable**: Handles seasonal variations properly
- ✅ **Current**: September 27, 2025 correctly returns Week 4

**Result**: All week detection now uses proper NFL calendar logic. Mock background updater will target correct current week games.

### 8. **ENHANCED: VisibilityTimerService Live Updates**
**Status**: ✅ ENHANCED
**Discovery Date**: 2025-01-28
**Files**:
- `services/visibility_timer_service.go` - Added pick container refresh capability
- `handlers/sse_handler.go:BroadcastPickContainerRefresh()` - New method for visibility updates
- `main.go` - Wired up pick refresh handler
- `templates/dashboard.html` - Added pick-container-refresh listener

**Enhancement**: VisibilityTimerService was working well with live ticker and cutoff detection, but needed to trigger proper SSE pick container updates when visibility changes occur.

**What Was Already Working**:
- ✅ Live ticker running every minute
- ✅ Proper cutoff time detection via `ShouldTriggerVisibilityUpdate()`
- ✅ SSE broadcast capability wired up
- ✅ Template listener for `visibility-change` events

**What Was Enhanced**:

**1. Added Heavy-Handed Pick Container Refresh**:
```go
// New capability for full pick container OOB updates
type VisibilityTimerService struct {
    // ... existing fields
    pickRefreshHandler func(season, week int) // New handler for generating full pick container updates
}

func (s *VisibilityTimerService) SetPickRefreshHandler(handler func(season, week int)) {
    s.pickRefreshHandler = handler
}
```

**2. Enhanced Trigger Logic**:
```go
if pickHandler != nil {
    // Heavy-handed full container update
    pickHandler(s.currentSeason, week)
} else {
    // Fallback to dashboard reload
    s.broadcastHandler("visibility-change", "")
}
```

**3. New SSE Handler Method**:
```go
func (h *SSEHandler) BroadcastPickContainerRefresh(season, week int) {
    // Get all user picks with updated visibility filtering
    userPicks, err := h.pickService.GetAllUserPicksForWeek(ctx, season, week)
    // Apply visibility filtering for current time (shows newly visible picks)
    filteredUserPicks, err := h.visibilityService.FilterVisibleUserPicks(ctx, userPicks, season, week, -1)
    // Render complete pick-container with OOB swap
    oobHTML := fmt.Sprintf(`<div class="pick-container" id="pick-container" hx-swap-oob="true">%s</div>`, htmlBuffer.String())
    h.BroadcastToAllClients("pick-container-refresh", oobHTML)
}
```

**4. Template Integration**:
```html
<!-- Pick container refresh for visibility cutoffs -->
<div sse-swap="pick-container-refresh" hx-swap="none"></div>
```

**Complete Flow Now**:
1. ✅ **Timer ticks** every minute
2. ✅ **Detects cutoff** via `ShouldTriggerVisibilityUpdate()`
3. ✅ **Triggers heavy-handed refresh** via `BroadcastPickContainerRefresh()`
4. ✅ **Fetches all picks** with current visibility filtering
5. ✅ **Renders complete pick-container** with updated visibility
6. ✅ **Broadcasts OOB swap** to all connected clients
7. ✅ **Template receives** `pick-container-refresh` event and updates

**Result**: When pick visibility cutoffs are reached, every connected display receives a complete pick-container refresh showing newly visible picks in real-time.

---

## Key Discoveries & Lessons Learned

### Architecture Insights
- **Sophisticated Foundation**: System has mature SSE + HTMX OOB architecture already in place
- **Change Streams**: MongoDB change streams automatically trigger SSE broadcasts with atomic message IDs
- **Message Sequencing**: Prevents race conditions with gap detection and auto-refresh
- **Template Integration**: HTMX SSE extension properly integrated with `hx-swap-oob="true"`

### Critical Failure Patterns Discovered
1. **Data Processing Inconsistency**: Initial loads vs SSE updates using different logic
2. **Missing Template Functions**: SSE updates skip critical enrichment steps like `PopulateDailyPickGroups()`
3. **Scope Mismatch**: SSE sends single user data to multi-user templates
4. **Category Completion Dependency**: Scoring triggers only when ALL games in category complete
5. **Unnecessary SSE Events**: Broadcasting when no actual data changes occur

### Root Cause Categories
- **Pipeline Gaps**: Missing connections between services (ParlayService → SSE)
- **Logic Inconsistency**: Different data processing for same templates
- **Outdated Code**: SSE functions not updated when scoring logic evolved
- **Over-Broad Triggers**: SSE events firing for irrelevant changes

### Best Practices Established
- **Match Processing Logic**: Initial loads and SSE updates must use identical data processing
- **Call Same Enrichment**: Use same functions (`PopulateDailyPickGroups`, `EnrichPickWithGameData`)
- **Same Data Structures**: Ensure template compatibility between initial and live updates
- **Immediate Recalculation**: Don't wait for category completion for individual score changes
- **Change Detection**: Only broadcast SSE when display data actually changes
- **Interface Consistency**: Use broadcaster interfaces for clean service separation

### Performance Optimizations Applied
- **Single Database Operations**: In-memory result calculation before database writes
- **Conditional Broadcasting**: Only send SSE when scores change
- **Targeted Updates**: Only process completed games for score recalculation
- **Filtered Triggers**: Distinguish between different types of pick changes

---

## File References

### Key Files
- `handlers/sse_handler.go` - SSE broadcasting logic (NEEDS FIX)
- `handlers/game_display_handler.go` - Dashboard initial load logic (REFERENCE)
- `templates/dashboard.html` - SSE listeners and OOB setup
- `services/change_stream.go` - MongoDB change detection

### Critical Functions
- `BroadcastPickUpdate()` - BROKEN, needs to match dashboard logic
- `GetAllUserPicksForWeek()` - Correct function for getting all users
- `PopulateDailyPickGroups()` - MISSING from SSE updates
- `FilterVisibleUserPicks()` - Security filtering (used correctly)

---

*Last Updated: 2025-01-28*
*Next Review: After implementing SSE pick update fixes*