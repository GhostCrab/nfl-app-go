# Old Picks Collection Cleanup TODO

After dropping the `picks` collection from MongoDB, the following files still reference the old picks system and need to be cleaned up or updated to use the new WeeklyPicks model.

## Files with Direct Collection Access (HIGH PRIORITY - WILL BREAK)

These files directly access the dropped "picks" collection and will fail at runtime:

### 1. `/cmd/clear_database.go`
- **Line:** `picksCollection := db.GetCollection("picks")`
- **Usage:** Database cleanup utility
- **Action Needed:** Update to use "weekly_picks" collection or remove picks cleanup entirely

### 2. `/database/mongo_pick_repository.go`
- **Line:** `collection := db.GetCollection("picks")`
- **Usage:** Core repository implementation for old individual picks
- **Action Needed:** **DELETE ENTIRE FILE** - no longer needed with WeeklyPicks model

### 3. `/scripts/simple_scoring_check.go`
- **Line:** `picksCollection := db.GetCollection("picks")`
- **Usage:** Scoring validation script
- **Action Needed:** Update to use WeeklyPicksRepository methods

### 4. `/scripts/full_database_refresh.go`
- **Line:** `picksCollection := db.GetCollection("picks")`
- **Usage:** Database refresh utility
- **Action Needed:** Update to use WeeklyPicksRepository methods

## Files Using Old PickRepository Interface (MEDIUM PRIORITY)

These files use the old PickRepository interface and should be updated to use WeeklyPicksRepository:

### Main Application Dependencies
- **`/main.go`** (Lines 141, 143, 144, 145)
  - Creates `pickRepo := database.NewMongoPickRepository(db)`
  - Passes to `parlayService`, `resultCalcService`, `analyticsService`
  - **Action:** Remove pickRepo creation, update service constructors

### Services Needing Updates
- **`/services/analytics_service.go`**
  - Uses old PickRepository
  - **Action:** Update to use WeeklyPicksRepository

- **`/services/parlay_service.go`**
  - Uses old PickRepository
  - **Action:** Update to use WeeklyPicksRepository

- **`/services/result_calculation_service.go`**
  - Uses old PickRepository
  - **Action:** Update to use WeeklyPicksRepository

- **`/services/legacy_import.go`**
  - Uses old PickRepository
  - **Action:** Review if still needed or update for WeeklyPicks

### Debug/Utility Commands
- **`/cmd/debug_game_ids.go`**
- **`/cmd/debug_pick_results.go`**
- **`/cmd/debug_picks_week8.go`**
- **`/cmd/import_legacy.go`**
- **`/cmd/populate_test_picks.go`**
- **`/cmd/test_pick_enrichment.go`**
- **`/cmd/test_picks.go`**
- **Action:** Update to use WeeklyPicksRepository or mark as deprecated

### Script Files
- **`/scripts/debug_parlay_scoring.go`**
- **`/scripts/debug_week16_scoring.go`**
- **`/scripts/debug_week2_thursday.go`**
- **`/scripts/recalculate_parlay_scoring.go`**
- **Action:** Update to use WeeklyPicksRepository

## Files to Remove (CLEANUP)

### 1. `/database/mongo_pick_repository.go`
- **Status:** No longer needed
- **Action:** DELETE - replaced by WeeklyPicksRepository

### 2. `/database/mongo_pick_repository_extensions.go`
- **Status:** Extensions for old repository
- **Action:** DELETE or migrate useful methods to WeeklyPicksRepository

## SSE Handler Already Updated

### ✅ `/handlers/sse_handler.go`
- **Status:** ALREADY HANDLES BOTH
- **Line:** `if (event.Collection == "picks" || event.Collection == "weekly_picks")`
- **Action:** Can remove "picks" reference after cleanup complete

## Cleanup Strategy

### Phase 1: Immediate Fixes (Prevent Runtime Errors)
1. Update or remove direct collection access in cmd/ and scripts/
2. Remove mongo_pick_repository.go file
3. Update main.go to not create pickRepo

### Phase 2: Service Layer Updates
1. Update AnalyticsService to use WeeklyPicksRepository
2. Update ParlayService to use WeeklyPicksRepository
3. Update ResultCalculationService to use WeeklyPicksRepository

### Phase 3: Debug/Utility Updates
1. Update or deprecate debug commands
2. Update or deprecate script files
3. Clean up SSE handler to only watch weekly_picks

### Phase 4: Final Cleanup
1. Remove any remaining old pick model references
2. Update documentation
3. Remove this TODO file

## Priority Order

1. **CRITICAL:** Fix files that access "picks" collection directly
2. **HIGH:** Update main.go and service constructors
3. **MEDIUM:** Update analytics, parlay, and result calculation services
4. **LOW:** Update debug commands and scripts
5. **CLEANUP:** Remove old repository files

## Current Status: 🔴 BROKEN
The application will fail at startup due to missing "picks" collection references in critical files.