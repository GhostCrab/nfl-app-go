# Missing Repository Methods and Required Refactoring

## Overview

During the enterprise-level refactoring process, several missing methods and structural issues have been identified. This document outlines what needs to be implemented to complete the service decomposition.

## Missing Repository Methods

### MongoPickRepository Missing Methods
The following methods are referenced in the new service layer but don't exist:

```go
// In mongo_pick_repository.go - MISSING METHODS:

// GetPicksByGameID retrieves all picks for a specific game
func (r *MongoPickRepository) GetPicksByGameID(ctx context.Context, gameID int) ([]models.Pick, error)

// UpdatePickResult updates the result of a specific pick
func (r *MongoPickRepository) UpdatePickResult(ctx context.Context, pickID primitive.ObjectID, result models.PickResult) error

// GetUserPicksBySeason gets all picks for a user in a season
func (r *MongoPickRepository) GetUserPicksBySeason(ctx context.Context, userID, season int) ([]models.Pick, error)

// GetPicksBySeason gets all picks for a season
func (r *MongoPickRepository) GetPicksBySeason(ctx context.Context, season int) ([]models.Pick, error)

// GetUniqueUserIDsForWeek gets all unique user IDs who made picks in a week
func (r *MongoPickRepository) GetUniqueUserIDsForWeek(ctx context.Context, season, week int) ([]int, error)

// GetUserPicksForWeek gets picks for a specific user and week
func (r *MongoPickRepository) GetUserPicksForWeek(ctx context.Context, userID, season, week int) ([]models.Pick, error)
```

### MongoUserRepository Missing Methods
```go
// In mongo_user_repository.go - MISSING METHODS:

// FindByID finds a user by their ID
func (r *MongoUserRepository) FindByID(ctx context.Context, userID int) (*models.User, error)
```

### MongoParlayRepository Missing Methods
```go
// In mongo_parlay_repository.go - MISSING METHODS:

// GetUserSeasonRecord gets a user's parlay record for a season
func (r *MongoParlayRepository) GetUserSeasonRecord(ctx context.Context, userID, season int) (*models.ParlaySeasonRecord, error)

// UpsertUserSeasonRecord creates or updates a user's season record
func (r *MongoParlayRepository) UpsertUserSeasonRecord(ctx context.Context, record *models.ParlaySeasonRecord) error

// CountUsersWithScoresForWeek counts users who have scores for a specific week
func (r *MongoParlayRepository) CountUsersWithScoresForWeek(ctx context.Context, season, week int) (int, error)
```

## Missing Model Structures

### Parlay Models
These models are referenced but don't exist:

```go
// In models/parlay_score.go - MISSING STRUCTURES:

// ParlayCategory represents different parlay betting categories
type ParlayCategory string

const (
    ParlayCategoryThursday      ParlayCategory = "thursday"
    ParlayCategoryFriday        ParlayCategory = "friday"  
    ParlayCategorySundayMonday  ParlayCategory = "sunday_monday"
)

// ParlaySeasonRecord represents a user's parlay performance for an entire season
type ParlaySeasonRecord struct {
    ID          primitive.ObjectID           `bson:"_id,omitempty" json:"id"`
    UserID      int                         `bson:"user_id" json:"user_id"`
    Season      int                         `bson:"season" json:"season"`
    WeekScores  map[int]ParlayWeekScore     `bson:"week_scores" json:"week_scores"`
    TotalPoints int                         `bson:"total_points" json:"total_points"`
    CreatedAt   time.Time                   `bson:"created_at" json:"created_at"`
    UpdatedAt   time.Time                   `bson:"updated_at" json:"updated_at"`
}

// RecalculateTotals recalculates season totals from weekly scores
func (r *ParlaySeasonRecord) RecalculateTotals() {
    total := 0
    for _, week := range r.WeekScores {
        total += week.TotalPoints
    }
    r.TotalPoints = total
    r.UpdatedAt = time.Now()
}

// ParlayWeekScore represents parlay scores for a specific week
type ParlayWeekScore struct {
    Week               int            `bson:"week" json:"week"`
    ThursdayPoints     int            `bson:"thursday_points" json:"thursday_points"`
    FridayPoints       int            `bson:"friday_points" json:"friday_points"`
    SundayMondayPoints int            `bson:"sunday_monday_points" json:"sunday_monday_points"`
    DailyScores        map[string]int `bson:"daily_scores,omitempty" json:"daily_scores,omitempty"` // For modern seasons
    TotalPoints        int            `bson:"total_points" json:"total_points"`
}
```

## Current Architecture Problems

### 1. Service Layer Issues

#### PickService (1,128 lines) - BLOATED
**Current responsibilities:**
- Pick creation/management ✓
- Result calculations ❌ (should be separate service)
- Parlay scoring ❌ (should be separate service)  
- Analytics ❌ (should be separate service)
- Broadcasting ❌ (should be separate service)
- Team ID mapping ❌ (should be utility or separate service)

**Refactoring Status:**
- ✅ **ParlayService** created (handles parlay scoring)
- ✅ **ResultCalculationService** created (handles pick result calculations) 
- ✅ **AnalyticsService** created (handles statistics and analytics)
- ❌ **Broadcasting** - needs extraction to notification service
- ❌ **Team mapping** - needs utility service

#### BackgroundUpdater (561 lines) - LARGE
**Current responsibilities:**
- ESPN API updates ✓
- Game state monitoring ✓ 
- Pick result processing ❌ (should delegate to ResultCalculationService)
- Parlay score processing ❌ (should delegate to ParlayService)

### 2. Handler Layer Issues

#### GameHandler (1,687 lines) - MASSIVE
**Current responsibilities:**
- HTTP game endpoints ✓
- SSE broadcasting ❌ (partially extracted to sse_handler.go)
- Pick submission ❌ (should be separate handler)
- Demo game logic ❌ (should be separate service)
- Database change handling ❌ (should be middleware)

**Refactoring Status:**
- ✅ **SSEHandler** partially created 
- ❌ **PickHandler** - needs creation for pick-related endpoints
- ❌ **DemoService** - needs extraction for demo game logic

### 3. Database Layer Issues

#### Context Timeout Duplication
- ✅ **Database utilities** created with standardized timeouts
- ❌ **Repository refactoring** - need to update all repositories to use new utilities

## Implementation Priority

### Phase 1: Complete Repository Methods (HIGH PRIORITY)
1. Add missing methods to MongoPickRepository
2. Add missing methods to MongoUserRepository  
3. Add missing methods to MongoParlayRepository
4. Create missing parlay model structures

### Phase 2: Service Integration (HIGH PRIORITY)
1. Update BackgroundUpdater to use new services
2. Update original PickService to delegate to new services
3. Create team mapping utility service
4. Create notification service for broadcasting

### Phase 3: Handler Decomposition (MEDIUM PRIORITY)
1. Create PickHandler for pick-related endpoints
2. Extract demo logic to DemoService
3. Create middleware for database change handling
4. Update routing to use new handlers

### Phase 4: Repository Modernization (LOW PRIORITY)
1. Update all repositories to use database utilities
2. Implement proper error handling patterns
3. Add connection pooling management
4. Implement transaction support

## Service Dependencies After Refactoring

```
┌─────────────────┐    ┌────────────────────┐    ┌──────────────────┐
│   PickService   │───▶│ ResultCalculation  │───▶│  MongoPickRepo   │
│   (Core CRUD)   │    │     Service        │    │                  │
└─────────────────┘    └────────────────────┘    └──────────────────┘
         │                        │                        │
         │              ┌─────────▼──────────┐             │
         │              │   ParlayService    │             │
         │              │  (Scoring Logic)   │             │
         │              └─────────┬──────────┘             │
         │                        │                        │
         ▼                        ▼                        ▼
┌─────────────────┐    ┌────────────────────┐    ┌──────────────────┐
│ AnalyticsService│    │ MongoParlayRepo    │    │  MongoGameRepo   │
│  (Statistics)   │    │                    │    │                  │
└─────────────────┘    └────────────────────┘    └──────────────────┘
         │                        │                        │
         ▼                        ▼                        ▼
┌─────────────────┐    ┌────────────────────┐    ┌──────────────────┐
│  MongoUserRepo  │    │   TeamService      │    │   ESPNService    │
│                 │    │   (ID Mapping)     │    │                  │
└─────────────────┘    └────────────────────┘    └──────────────────┘
```

## Benefits of Completed Refactoring

### Developer Experience
- **Reduced Complexity**: Each service has single responsibility
- **Easier Testing**: Services can be unit tested independently  
- **Better Organization**: Clear separation of concerns
- **Faster Development**: Smaller, focused files are easier to modify

### System Architecture
- **Scalability**: Services can be scaled independently
- **Maintainability**: Changes isolated to specific domains
- **Testability**: Mock interfaces for testing
- **Documentation**: Self-documenting service boundaries

### Code Quality Metrics (Expected)
- **File Size**: No file > 500 lines (currently: games.go = 1,687 lines)
- **Method Count**: No service > 15 methods (currently: PickService = 32 methods)
- **Cyclomatic Complexity**: Reduced from high values to < 10 per method
- **Test Coverage**: Can achieve > 80% with focused services

## Risk Assessment

### Low Risk (Safe to implement immediately)
- ✅ Template function extraction (COMPLETED)
- ✅ Database utility creation (COMPLETED)
- ✅ Service creation (COMPLETED - needs repository methods)
- ❌ Repository method additions (straightforward CRUD operations)

### Medium Risk (Requires careful testing)
- Service integration and delegation
- Handler decomposition  
- Routing updates

### High Risk (Requires production planning)
- Database schema changes (if any)
- Live service replacement
- Authentication/authorization changes

## Next Steps

1. **Implement missing repository methods** (enables new services to function)
2. **Create missing model structures** (supports parlay functionality)
3. **Update BackgroundUpdater** to delegate to new services  
4. **Test service integration** with existing functionality
5. **Gradually migrate handlers** to use new service architecture
6. **Monitor performance** and adjust as needed

This refactoring transforms the codebase from a monolithic structure to a clean, enterprise-level architecture following Domain-Driven Design principles and Go best practices.