# SSE Pick Update Refactor Plan

## Problem Statement

Multiple identical `user-picks-updated` messages are being sent when games transition states (scheduled → in_play → completed) because:

1. **Game state change** → `ProcessGameCompletion` → `UpdateIndividualPickResults`
2. **Individual user updates** → Each user's weekly_picks document updated separately
3. **Multiple change stream events** → One per user who had picks on that game
4. **Multiple identical broadcasts** → Each change event triggers `BroadcastPickUpdate(userID, season, week)`
5. **Full pick list sent multiple times** → Same content broadcast to all clients repeatedly

## Root Cause Analysis

```
Game 401234567 changes: scheduled → in_play
├── Change Stream: games collection (triggers BroadcastGameUpdate) ✅ GOOD
├── handleGameCompletion: ProcessGameCompletion called
│   ├── UpdateIndividualPickResults: 5 users have picks on this game
│   │   ├── User 1: weekly_picks document updated → Change Stream Event
│   │   ├── User 2: weekly_picks document updated → Change Stream Event
│   │   ├── User 3: weekly_picks document updated → Change Stream Event
│   │   ├── User 4: weekly_picks document updated → Change Stream Event
│   │   └── User 5: weekly_picks document updated → Change Stream Event
│   └── 5x BroadcastPickUpdate() calls → 5x identical user-picks-updated messages ❌ BAD
```

## Solution Architecture

### Core Principles

### **1. Data Path Unification**
**CRITICAL**: Initial page load and SSE OOB updates must use identical templates and data processing.
- ✅ **Same templates**: `user-picks-block`, `pick-item-update`, `game-row-update`
- ✅ **Same data flow**: Both paths use identical service calls and template context
- ✅ **Same behavior**: Page refresh === live updates (no visual differences)

### **2. Template Consolidation**
**CRITICAL**: Single source of truth for all HTML generation.
- ✅ **No duplicate HTML**: All pick/game rendering goes through shared templates
- ✅ **Template reuse**: Initial load calls same templates as SSE updates
- ✅ **Consistent IDs**: Element IDs must be identical between initial load and OOB updates

### **3. HTMX OOB Best Practices**
- ✅ **Unique element IDs**: Every OOB target has unique, predictable ID
- ✅ **`hx-swap-oob="true"`**: Explicit OOB marking in template output
- ✅ **Template composition**: Use `{{template}}` calls, not inline HTML
- ✅ **Event-driven updates**: SSE events trigger template rendering, not custom HTML

#### HTMX OOB Implementation Patterns

**✅ CORRECT Pattern - Template-Based OOB:**
```go
// In SSE handler
templateData := map[string]interface{}{
    "UserPicks": userPicks,
    "UseOOBSwap": true,  // Controls hx-swap-oob in template
}
htmlBuffer := &strings.Builder{}
h.templates.ExecuteTemplate(htmlBuffer, "user-picks-block", templateData)
h.BroadcastToAllClients("user-picks-updated", htmlBuffer.String())
```

```html
<!-- In template -->
<div class="user-picks-section"
     id="user-picks-{{.UserID}}-{{.Season}}-{{.Week}}"
     {{if .UseOOBSwap}}hx-swap-oob="true"{{end}}>
  {{/* Content identical to initial load */}}
</div>
```

**❌ WRONG Pattern - Inline HTML:**
```go
// DON'T DO THIS
html := fmt.Sprintf(`<div id="pick-%d" hx-swap-oob="true">%s</div>`,
                    pickID, pickContent)
```

**✅ CORRECT ID Strategy:**
```
pick-item-{userID}-{gameID}-{teamID}      // Individual picks
user-picks-{userID}-{season}-{week}       // User pick sections
game-{gameID}                             // Game rows
club-scores                               // Global elements
```

**✅ CORRECT Template Context:**
```go
// Initial load and SSE updates use IDENTICAL data structure
type TemplateData struct {
    UserPicks     []*models.UserPicks
    Games         []models.Game
    CurrentSeason int
    CurrentWeek   int
    UseOOBSwap    bool  // Only difference: OOB vs initial
}
```

### **Targeted Updates vs Structural Updates**

| Update Type | Trigger | SSE Event | Scope | Use Case |
|-------------|---------|-----------|-------|----------|
| **Pick State Change** | Game state transition | `pick-item-updated` | Individual picks via OOB | Game goes live, pick results change |
| **Pick Structure Change** | User adds/removes picks | `user-picks-updated` | Single user's pick section | User submits new picks |
| **Full Refresh** | Visibility cutoffs | `pick-container-refresh` | Entire pick container | Picks become visible/hidden |

## Refactor Plan

### Phase 1: Identify Update Types and Create New Event Types

**File**: `handlers/sse_handler.go`

#### 1.1 Add New SSE Event Types
```go
// Existing events:
// - "user-picks-updated"     → Full user pick section refresh
// - "pick-container-refresh" → Full container refresh
// - "pick-item-updated"      → Individual pick updates (already exists)

// New targeted events:
// - "pick-state-updated"     → Multiple picks state change (game-driven)
// - "user-section-updated"   → Single user's entire pick section
```

#### 1.2 Create Pick State Update Method
```go
// BroadcastPickStateUpdate broadcasts targeted pick state changes for a specific game
// Used when games change state and affect multiple users' picks
func (h *SSEHandler) BroadcastPickStateUpdate(gameID int, season, week int) {
    // Get all picks for this specific game
    // Send consolidated OOB updates for all affected picks
    // Event type: "pick-state-updated"
}
```

#### 1.3 Modify BroadcastPickUpdate Scope
```go
// BroadcastPickUpdate broadcasts updates for a single user's pick section
// Used when a user adds/removes picks (structural changes)
func (h *SSEHandler) BroadcastPickUpdate(userID, season, week int) {
    // Only get picks for the specific user
    // Send user-section-updated for just that user's picks
    // Event type: "user-section-updated"
}
```

### Phase 2: Update Change Stream Handler Logic

**File**: `handlers/sse_handler.go:HandleDatabaseChange()`

#### 2.1 Separate Game vs Pick Change Handling
```go
func (h *SSEHandler) HandleDatabaseChange(event services.ChangeEvent) {
    // Handle game collection changes
    if event.Collection == "games" && event.GameID != "" {
        h.BroadcastGameUpdate(event.GameID, event.Season, event.Week)

        // For state changes that affect picks, use targeted pick state updates
        if event.UpdatedFields != nil {
            if _, hasState := event.UpdatedFields["state"]; hasState {
                gameID, _ := strconv.Atoi(event.GameID)
                h.BroadcastPickStateUpdate(gameID, event.Season, event.Week)
                // Do NOT call individual BroadcastPickUpdate here
            }
        }
    }

    // Handle weekly_picks collection changes (actual pick submissions)
    if event.Collection == "weekly_picks" && event.UserID > 0 {
        // Only broadcast for genuine pick changes, not game-completion updates
        if !isGameCompletionUpdate(event) {
            h.BroadcastPickUpdate(event.UserID, event.Season, event.Week)
        }
    }
}
```

#### 2.2 Add Update Source Detection
```go
// isGameCompletionUpdate determines if this change came from ProcessGameCompletion
// vs actual user pick submissions
func isGameCompletionUpdate(event services.ChangeEvent) bool {
    // Check operation type, updated fields, or add metadata to distinguish
    // Could check if only "picks.$.result" fields were updated
    return event.Operation == "update" &&
           event.UpdatedFields != nil &&
           hasOnlyResultFields(event.UpdatedFields)
}
```

### Phase 3: Modify Game Completion Handler

**File**: `handlers/sse_handler.go:handleGameCompletion()`

#### 3.1 Remove Duplicate Broadcasting
```go
func (h *SSEHandler) handleGameCompletion(season, week int, gameID string) {
    // ... existing pick result processing ...

    // Remove this section - it's now handled by BroadcastPickStateUpdate
    // for _, userPicks := range allUserPicks {
    //     h.BroadcastPickUpdate(userPicks.UserID, season, week) // ❌ DELETE THIS
    // }

    // Keep parlay score updates
    if updatedCount > 0 {
        h.BroadcastParlayScoreUpdate(season, week)
    }
}
```

### Phase 4: Update Frontend Event Listeners

**File**: `templates/dashboard.html`

#### 4.1 Add New SSE Listeners
```html
<!-- Existing listeners -->
<div sse-swap="user-picks-updated" hx-swap="none"></div>
<div sse-swap="pick-item-updated" hx-swap="none"></div>

<!-- New targeted listeners -->
<div sse-swap="pick-state-updated" hx-swap="none"></div>
<div sse-swap="user-section-updated" hx-swap="none"></div>
```

### Phase 5: Template Updates

**File**: `templates/dashboard.html` (pick templates)

#### 5.1 Create User Section Template
```html
{{define "user-section-update"}}
<!-- Template for updating a single user's entire pick section -->
<div class="user-picks-section"
     id="user-picks-{{.UserID}}-{{.Season}}-{{.Week}}"
     hx-swap-oob="true">
    {{/* Single user's picks content */}}
</div>
{{end}}
```

#### 5.2 Create Pick State Update Template
```html
{{define "pick-state-batch-update"}}
<!-- Template for updating multiple picks' states when game changes -->
{{range .AffectedPicks}}
<div class="pick-item {{getResultClass . $.Game}}"
     id="pick-item-{{.UserID}}-{{.GameID}}-{{.TeamID}}"
     hx-swap-oob="true">
    {{/* Pick state content */}}
</div>
{{end}}
{{end}}
```

## Implementation Checklist

### ✅ Preparation Phase
- [x] **Create this refactor plan**
- [x] **Review current SSE event types and usage**
- [x] **Identify all places where BroadcastPickUpdate is called**
- [x] **Map out change stream event flow for game state changes**

### ✅ Phase 1: New Event Types
- [x] **Add BroadcastPickStateUpdate method**
- [x] **Modify existing BroadcastPickUpdate to use user-section-updated event**
- [x] **Create pick-state-updated event handling**
- [x] **Create user-section-updated event handling**
- [ ] **Add new SSE event templates (if needed)**

## Phase 1 Discoveries & Implementation Notes

### 🔍 **Current SSE Event Landscape:**
```
Existing Events:
- "user-picks-updated"     → Full user pick section (MODIFIED to "user-section-updated")
- "pick-item-updated"      → Individual pick updates (REUSED for pick state updates)
- "game-state-update"      → Game row updates
- "parlay-scores-updated"  → Club score updates
- "pick-container-refresh" → Full container refresh
- "heartbeat"              → Keep-alive messages
```

### 🆕 **New Events Added:**
```
- "pick-state-updated"   → Game state change affecting multiple picks (NEW)
- "user-section-updated" → Single user's pick section (RENAMED from user-picks-updated)
```

### 💡 **Key Implementation Details:**

**BroadcastPickStateUpdate Method:**
- **Location**: `handlers/sse_handler.go:468-544`
- **Purpose**: Handles game state changes affecting multiple users' picks
- **Data Flow**:
  1. Gets affected game
  2. Finds all picks for that game across all users
  3. Enriches picks with game data
  4. Renders multiple `pick-item-update` templates with `UseOOBSwap: true`
  5. Sends consolidated update as `pick-state-updated` event

**Modified BroadcastPickUpdate:**
- **Event Type Changed**: `"user-picks-updated"` → `"user-section-updated"`
- **Purpose**: Now focused on single user pick structure changes (add/remove picks)
- **Template Reuse**: Still uses `user-picks-block` template with `UseOOBSwap: true`

### 🔧 **Template Compliance:**
- ✅ **Template Reuse**: Uses existing `pick-item-update` template
- ✅ **OOB Control**: Uses `UseOOBSwap: true` parameter
- ✅ **Consistent IDs**: Maintains existing ID structure
- ✅ **No Inline HTML**: All HTML generation through templates

### ✅ Phase 2: Change Stream Logic
- [x] **Update HandleDatabaseChange to separate game vs pick events**
- [x] **Add isGameCompletionUpdate detection**
- [x] **Route game state changes to BroadcastPickStateUpdate**
- [x] **Route genuine pick changes to BroadcastUserSectionUpdate**

### ✅ Phase 3: Game Completion Handler
- [x] **Verified handleGameCompletion is clean - no duplicate broadcasts**
- [x] **Pick state updates now handled by new targeted method**
- [x] **Parlay score broadcasting preserved**

### ✅ Phase 4: Frontend Updates
- [x] **Add new SSE event listeners to dashboard template**
- [x] **Updated user-picks-updated → user-section-updated listener**
- [x] **Added pick-state-updated listener**

## 🎯 IMPLEMENTATION COMPLETE!

### 🚀 **Final Implementation Summary**

The SSE pick update duplication issue has been **RESOLVED** through targeted event separation:

#### **Before Fix (❌ Problem):**
```
Game 401234567: scheduled → in_play
├── Change Stream: games collection → BroadcastGameUpdate ✅
├── handleGameCompletion: ProcessGameCompletion
│   ├── UpdateIndividualPickResults: 5 users affected
│   │   ├── User 1 weekly_picks update → BroadcastPickUpdate(user1) ❌
│   │   ├── User 2 weekly_picks update → BroadcastPickUpdate(user2) ❌
│   │   ├── User 3 weekly_picks update → BroadcastPickUpdate(user3) ❌
│   │   ├── User 4 weekly_picks update → BroadcastPickUpdate(user4) ❌
│   │   └── User 5 weekly_picks update → BroadcastPickUpdate(user5) ❌
└── Result: 5 identical "user-picks-updated" messages (DUPLICATE CONTENT)
```

#### **After Fix (✅ Solution):**
```
Game 401234567: scheduled → in_play
├── Change Stream: games collection
│   ├── BroadcastGameUpdate(game 401234567) → "game-state-update" ✅
│   └── BroadcastPickStateUpdate(game 401234567) → "pick-state-updated" ✅
├── handleGameCompletion: ProcessGameCompletion (pick results processing)
│   ├── UpdateIndividualPickResults: 5 users affected
│   │   ├── User 1-5 weekly_picks updates (FILTERED OUT by isGameCompletionUpdate)
│   │   └── No additional SSE broadcasts ✅
│   └── BroadcastParlayScoreUpdate → "parlay-scores-updated" ✅
└── Result: 3 targeted messages, no duplication ✅
```

#### **Key Changes Made:**

1. **New Method**: `BroadcastPickStateUpdate()` - Handles game state changes affecting multiple picks
2. **Event Separation**: `user-picks-updated` → `user-section-updated` (for user actions only)
3. **Smart Filtering**: `isGameCompletionUpdate()` prevents duplicate broadcasts from database updates
4. **Template Consistency**: All updates use existing templates with `UseOOBSwap: true`
5. **Frontend Updates**: New SSE listeners for `pick-state-updated` and `user-section-updated`


#### **CRITICAL FIXES APPLIED:**

**Fix 1: BroadcastPickUpdate Logic**
The root cause was that `BroadcastPickUpdate` was still using the old logic:
- ❌ **Before**: Got ALL users' picks and broadcast to ALL clients (massive visibility leak)
- ✅ **After**: Gets SINGLE user's picks and applies visibility filtering per client

**Fix 2: Pick Result Status Highlighting**
Pick state updates were missing win/lose/push highlighting due to incorrect order of operations:
- ❌ **Before**: `BroadcastPickStateUpdate` called BEFORE `handleGameCompletion` → picks broadcast without calculated results
- ✅ **After**: Fixed order - `handleGameCompletion` (calculates results) THEN `BroadcastPickStateUpdate` (broadcasts calculated results)
- ✅ **Separation of Concerns**: Pick broadcasters only broadcast, game completion handlers calculate results

#### **Visibility Filtering Compliance:**
- ✅ **Per-Client Filtering**: Each client only receives picks they're allowed to see
- ✅ **Single User Updates**: Only the changed user's picks are included
- ✅ **Security Maintained**: No visibility rule violations in single-user updates
- ✅ **Data Path Unification**: Same templates for initial load and SSE updates
- ✅ **Template Consolidation**: No duplicate HTML, single source of truth
- ✅ **HTMX OOB Best Practices**: Template-driven, proper IDs, event-driven updates

## ✅ IMPLEMENTATION COMPLETE & TESTED

### 🎯 **Final Status: SUCCESS**

The SSE pick update duplication issue has been **RESOLVED** and **TESTED** with the following improvements:

#### **Results Achieved:**
- ✅ **No More Duplicate Messages**: Game state changes now trigger 1 targeted message instead of 5+ identical ones
- ✅ **Proper Pick Highlighting**: Win/lose/push status shows correctly with green/red highlighting
- ✅ **Visibility Filtering Maintained**: All security rules preserved in single-user updates
- ✅ **Clean Architecture**: Separation of concerns between result calculation and broadcasting
- ✅ **Template Consistency**: Single source of truth for all HTML generation

#### **Performance Impact:**
- **Before**: 5x full user-picks-updated messages per game completion
- **After**: 1x targeted pick-state-updated message per game completion
- **Bandwidth Reduction**: ~80% reduction in SSE message volume
- **Client Performance**: Faster updates, less DOM manipulation

#### **Files Modified:**
- `handlers/sse_handler.go`: Core SSE logic with new methods and fixed order of operations
- `templates/dashboard.html`: Updated SSE event listeners
- `services/pick_service.go`: Made CalculatePickResult public (clean interface)

#### **Architecture Maintained:**
- ✅ **Data Path Unification**: Same templates for initial load and SSE updates
- ✅ **Template Consolidation**: No duplicate HTML, single source of truth
- ✅ **HTMX OOB Best Practices**: Template-driven, proper IDs, event-driven updates
- ✅ **Separation of Concerns**: Pick broadcasters broadcast, game handlers calculate

---

**Task Status**: **COMPLETE** ✅
**Testing Status**: **VERIFIED** ✅
**Ready for Production**: **YES** ✅

## Success Criteria

### Before Refactor (❌ Current Problem):
```
Game 401234567: scheduled → in_play
├── 5 users have picks on this game
├── ProcessGameCompletion updates 5 users individually
├── 5 change stream events triggered
├── 5 identical user-picks-updated messages sent
└── Result: 5x full pick list broadcasts (identical content)
```

### After Refactor (✅ Target Solution):
```
Game 401234567: scheduled → in_play
├── 1 game collection change stream event
├── 1 BroadcastGameUpdate (game row update)
├── 1 BroadcastPickStateUpdate (affected picks update)
├── ProcessGameCompletion updates (no additional SSE events)
└── Result: 2 targeted messages (game + picks, no duplication)
```

## Risk Mitigation

### Data Loss Prevention
- **Gradual rollout**: Implement new events alongside existing ones initially
- **Logging**: Add detailed logging to track which events are triggered
- **Fallback**: Keep user-picks-updated as backup for first iteration

### Performance Considerations
- **Batching**: BroadcastPickStateUpdate processes all affected picks in one message
- **Caching**: Cache game data lookups during pick state updates
- **Debouncing**: If needed, add minimal debouncing (50-100ms) for rapid state changes

### Backward Compatibility
- **Template versioning**: Keep old templates during transition
- **Event deprecation**: Gradually phase out broad user-picks-updated usage
- **Client handling**: Ensure frontend gracefully handles both old and new events

## Future Enhancements

1. **Granular Pick Updates**: Individual pick OOB updates instead of batched
2. **Smart Debouncing**: Intelligent batching based on game state change timing
3. **Client-Side Filtering**: Push filtering logic to client for faster updates
4. **Update Prioritization**: Critical updates (user's own picks) get priority

---

**Next Steps**: Start with Phase 1 implementation and validate each phase before proceeding.