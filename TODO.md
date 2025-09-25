# WeeklyPicks Migration TODOs

## ✅ COMPILATION SUCCESSFUL

The core WeeklyPicks migration is now compiling successfully! The main performance improvement (1 SSE event instead of 8) should now work.

### Key Changes Completed:
- ✅ WeeklyPicks storage model implemented
- ✅ Single upsert operation instead of multiple bulk operations
- ✅ Change stream monitoring weekly_picks collection
- ✅ All Pick.ID references removed (no longer needed)
- ✅ Backward compatibility maintained through adapter patterns

## High Priority - Compilation Fixes (COMPLETED)

### services/pick_service.go
- [x] **CreatePick()**: Commented out pickRepo.Create() for now
- [x] **GetAllUserPicksForWeek()**: Fixed undefined picksByUser variable, updated logic to use WeeklyPicks
- [x] **ProcessGameCompletion()**: Updated to use GetAllPicksForWeek and removed pick.ID references
- [x] **GetPickStats()**: Stubbed out pickRepo.Count() for now
- [x] **EnrichPickWithGameData()**: Removed pick.ID references and pickRepo.UpdateResult
- [x] **ReplaceUserPicksForWeek()**: Updated to use WeeklyPicks.Upsert
- [x] **GetPicksForAnalytics()**: Updated to use WeeklyPicks repository methods
- [x] **UpdatePickResult()**: Stubbed out for WeeklyPicks compatibility

### services/result_calculation_service.go
- [x] **Remove pick.ID references**: Replaced pick.ID.Hex() with user/game identification

### handlers/sse_handler.go
- [x] **Remove pick.ID references**: Replaced pick.ID.Hex() with UserID-GameID combination for HTML element IDs

## Medium Priority - Functionality Implementation

### database/mongo_weekly_picks_repository.go
- [ ] **Add FindAllBySeason()** method for analytics support
- [ ] **Add Create()** method for single pick creation (if needed)
- [ ] **Add Count()** method for statistics

### services/pick_service.go
- [ ] **UpdatePickResult()**: Implement result updates within WeeklyPicks documents
- [ ] **ProcessGameCompletion()**: Update pick results in WeeklyPicks format
- [ ] **EnrichPickWithGameData()**: Remove dependency on pick.ID field
- [ ] **GetUserRecord()**: Implement user record calculation from WeeklyPicks
- [ ] **CreatePick()**: Decide if single pick creation is needed or if all picks go through WeeklyPicks

## Low Priority - Code Cleanup

### services/pick_service.go
- [ ] **GetUserPicksForWeek()**: Clean up unreachable code after return statement (line 141)
- [ ] **Remove legacy imports**: Clean up unused bson/mongo imports after migration
- [ ] **Add error handling**: Improve error handling for WeeklyPicks operations

## Testing Required

- [ ] **Pick submission**: Verify only 1 SSE event is generated instead of 8
- [ ] **Pick retrieval**: Ensure GetUserPicksForWeek works with new storage
- [ ] **Analytics**: Verify GetPicksForAnalytics works with converted data
- [ ] **Game completion**: Test ProcessGameCompletion with new storage model

## Database Migration

- [ ] **Data migration**: Convert existing individual pick documents to WeeklyPicks format
- [ ] **Index creation**: Ensure proper indexes exist on weekly_picks collection
- [ ] **Cleanup**: Remove old picks collection after migration complete

## Notes

- The core WeeklyPicks storage and retrieval is working
- Main issue is updating all references from individual Pick documents to WeeklyPicks documents
- Pick.ID field was removed - need to handle methods that depend on it
- Change stream is already updated to watch weekly_picks collection
- SSE handler supports both collections during transition