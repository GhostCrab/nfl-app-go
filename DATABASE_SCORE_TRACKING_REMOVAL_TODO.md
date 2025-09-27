# Database Score Tracking Removal TODO

## Problem
The application is currently using **dual score tracking** - both in-memory (MemoryParlayScorer) and database (MongoParlayRepository). This creates:
- Performance overhead
- Potential inconsistencies
- Risk of reverting to database-only scoring
- Unnecessary database write operations

## Goal
Remove all database score tracking infrastructure and rely solely on in-memory scoring via MemoryParlayScorer.

## Files to Remove Database Score Tracking From

### 1. **CRITICAL - Services writing to database**
- `/services/pick_service.go` (lines 643, 758, 1054) - `UpsertParlayScore` calls
- `/services/parlay_service.go` (lines 224, 448) - `UpsertUserSeasonRecord` calls

### 2. **Database Infrastructure to Remove**
- `/database/mongo_parlay_repository.go` - **ENTIRE FILE** (already marked deprecated)
- `/database/mongo_parlay_repository_extensions.go` - **ENTIRE FILE**

### 3. **Main Application Dependencies**
- `/main.go` - Remove `parlayRepo := database.NewMongoParlayRepository(db)`
- Update service constructors to remove parlayRepo parameter

### 4. **Script References to Clean**
- `/scripts/full_database_refresh.go` - Remove parlay_scores clearing
- `/cmd/clear_database.go` - Remove parlay_scores clearing

### 5. **Handler References**
- `/handlers/demo_testing_handler.go` - Remove parlay_scores modification

## Migration Strategy

### Phase 1: Remove Active Database Writes
1. Remove `UpsertParlayScore` and `UpsertUserSeasonRecord` calls from services
2. Verify MemoryParlayScorer is handling all score updates

### Phase 2: Remove Service Dependencies
1. Update service constructors to not require MongoParlayRepository
2. Update main.go to not create parlayRepo

### Phase 3: Remove Database Infrastructure
1. Delete mongo_parlay_repository.go and extensions
2. Remove collection references from scripts

### Phase 4: Verify Memory-Only Operation
1. Test that scores work correctly with only MemoryParlayScorer
2. Verify no database score writes occur during game updates

## Expected Benefits
- **Performance**: No database writes for every score update
- **Simplicity**: Single source of truth for scores (memory)
- **Reliability**: Cannot accidentally revert to database scoring
- **Cleaner Architecture**: Remove deprecated/unused code

## Risk Mitigation
- MemoryParlayScorer already implemented and functional
- All score calculations happen in-memory on startup
- Scores are recalculated on game state changes via memory scorer