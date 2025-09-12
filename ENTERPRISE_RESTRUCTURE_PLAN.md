# Enterprise-Level Codebase Restructuring Plan

## Current Architecture Problems

### 1. **Massive Service Files**
- `pick_service.go`: 1,128 lines, 32 methods (violates SRP)
- `background_updater.go`: 561 lines 
- `games.go`: 1,687 lines, 27 methods

### 2. **Database Layer Issues**
- 20+ instances of duplicated `context.WithTimeout` pattern
- No central connection management
- Inconsistent error handling patterns

### 3. **Mixed Responsibilities**
- Pick service handles: creation, scoring, analytics, broadcasting, team mapping
- Game handler handles: HTTP, SSE, picks, demos, database changes

### 4. **Missing Enterprise Patterns**
- No dependency injection container
- No interface segregation 
- No proper layered architecture
- Limited error handling standardization

## Target Enterprise Architecture

```
nfl-app-go/
├── cmd/                          # Application entrypoints
│   ├── server/                   # Main web server
│   └── tools/                    # Administrative tools
├── internal/                     # Private application code
│   ├── api/                      # API layer (HTTP handlers)
│   │   ├── handlers/             # HTTP request handlers
│   │   ├── middleware/           # HTTP middleware
│   │   └── routes/               # Route definitions
│   ├── core/                     # Core business logic
│   │   ├── domain/               # Domain models and interfaces
│   │   ├── ports/                # Interface definitions (dependency inversion)
│   │   └── services/             # Core business services
│   ├── infrastructure/           # External concerns
│   │   ├── database/             # Database implementations
│   │   ├── external/             # External API clients (ESPN)
│   │   ├── notification/         # SSE, email services
│   │   └── templates/            # Template utilities
│   ├── config/                   # Configuration management
│   └── shared/                   # Shared utilities and types
├── pkg/                          # Public packages (if any)
├── web/                          # Web assets
│   ├── static/                   # Static files
│   └── templates/                # HTML templates
├── scripts/                      # Build and deployment scripts
├── deployments/                  # Deployment configurations
├── docs/                         # Documentation
└── tests/                        # Test utilities and data
```

## Phase 1: Database Layer Refactoring

### Issues Found
- **20+ duplicate timeout patterns** across repositories
- **Inconsistent error handling** 
- **No connection pooling management**
- **Mixed timeout durations** (5s, 10s, 30s randomly chosen)

### Solutions Implemented
✅ Created `database/utils.go` with standardized timeout patterns:
```go
const (
    ShortTimeout     = 5 * time.Second   // CRUD operations
    MediumTimeout    = 10 * time.Second  // Queries
    LongTimeout      = 30 * time.Second  // Bulk operations
    VeryLongTimeout  = 60 * time.Second  // Migrations
)
```

### Next Steps
1. **Repository Interface Standardization**
2. **Connection Pool Management**
3. **Error Handling Middleware**
4. **Transaction Support**

## Phase 2: Service Layer Decomposition

### Current `PickService` Responsibilities (1,128 lines, 32 methods):
- ✋ **Pick Management**: Create, update, validate picks
- ✋ **Result Calculation**: Spread, over/under calculations
- ✋ **Parlay Scoring**: Weekly and daily scoring logic
- ✋ **Analytics**: Statistics and reporting
- ✋ **Broadcasting**: SSE notifications
- ✋ **Team Mapping**: ESPN ID to team name conversion

### Proposed Service Split:
```go
// Core domain services
type PickService interface {
    CreatePick(ctx context.Context, req CreatePickRequest) (*Pick, error)
    GetUserPicks(ctx context.Context, userID, season, week int) (*UserPicks, error)
    UpdatePick(ctx context.Context, pickID string, req UpdatePickRequest) error
}

type GameResultService interface {
    CalculatePickResults(ctx context.Context, game *Game, picks []Pick) error
    ProcessGameCompletion(ctx context.Context, gameID int) error
}

type ParlayService interface {
    CalculateWeeklyScores(ctx context.Context, season, week int) error
    CalculateDailyScores(ctx context.Context, season, week int) error
    GetUserParlayScores(ctx context.Context, userID, season int) (ParlayScores, error)
}

type PickAnalyticsService interface {
    GetPickStatistics(ctx context.Context, filters StatFilters) (PickStats, error)
    GetUserPerformance(ctx context.Context, userID, season int) (Performance, error)
}
```

## Phase 3: Handler Layer Restructuring

### Current `games.go` Issues (1,687 lines, 27 methods):
- Mixed HTTP handling and business logic
- SSE broadcasting embedded in handler
- Demo game simulation logic
- Direct database access

### Proposed Handler Structure:
```go
internal/
├── api/
│   ├── handlers/
│   │   ├── game_handlers.go      # Game display endpoints
│   │   ├── pick_handlers.go      # Pick submission endpoints  
│   │   ├── auth_handlers.go      # Authentication endpoints
│   │   ├── analytics_handlers.go # Analytics endpoints
│   │   └── sse_handlers.go       # Server-sent events
│   ├── middleware/
│   │   ├── auth.go              # Authentication middleware
│   │   ├── logging.go           # Request logging
│   │   └── validation.go        # Request validation
│   └── routes/
│       └── routes.go            # Route definitions
```

## Phase 4: Domain Layer Creation

### Core Domain Models:
```go
internal/core/domain/
├── entities/
│   ├── game.go                  # Game aggregate root
│   ├── pick.go                  # Pick aggregate root  
│   ├── user.go                  # User aggregate root
│   └── parlay.go               # Parlay aggregate root
├── valueobjects/
│   ├── odds.go                 # Odds value object
│   ├── score.go                # Score value object
│   └── timeperiod.go           # Game time value object
└── repositories/
    ├── game_repository.go      # Game repository interface
    ├── pick_repository.go      # Pick repository interface
    └── user_repository.go      # User repository interface
```

## Phase 5: Infrastructure Layer Organization

### External Service Management:
```go
internal/infrastructure/
├── database/
│   ├── mongodb/
│   │   ├── connection.go       # Connection management
│   │   ├── game_repository.go  # Game repo implementation
│   │   └── pick_repository.go  # Pick repo implementation
│   └── repositories.go         # Repository factory
├── external/
│   ├── espn/
│   │   ├── client.go          # ESPN API client
│   │   └── models.go          # ESPN-specific models
│   └── providers.go           # External service factory
├── notification/
│   ├── sse/
│   │   ├── hub.go             # SSE connection hub
│   │   └── broadcaster.go     # SSE broadcasting logic
│   └── email/
│       └── service.go         # Email service
└── templates/
    └── functions.go           # Template function utilities
```

## Implementation Roadmap

### Phase 1: Foundation (Week 1)
- [x] Extract template functions (COMPLETED)
- [x] Create database utilities (COMPLETED)
- [x] Audit existing code (IN PROGRESS)
- [ ] Standardize error handling
- [ ] Create dependency injection framework

### Phase 2: Service Decomposition (Week 2)
- [ ] Split PickService into 4 focused services
- [ ] Extract business logic from handlers
- [ ] Create service interfaces and implementations
- [ ] Implement proper dependency injection

### Phase 3: Handler Restructuring (Week 3)
- [ ] Break down games.go handler
- [ ] Create specialized handlers
- [ ] Implement middleware chain
- [ ] Standardize HTTP response patterns

### Phase 4: Infrastructure Improvement (Week 4)
- [ ] Improve database layer with connection pooling
- [ ] Implement proper transaction support
- [ ] Create external service abstraction layer
- [ ] Standardize configuration management

## Metrics and Success Criteria

### Code Quality Metrics:
- **Cyclomatic Complexity**: Reduce from current high values to < 10 per method
- **File Size**: No file > 500 lines
- **Method Count**: No class > 15 methods
- **Test Coverage**: Achieve > 80% coverage

### Architecture Metrics:
- **Dependency Direction**: All dependencies point inward to domain
- **Interface Segregation**: Each interface serves single purpose
- **Single Responsibility**: Each class/service has one reason to change
- **Separation of Concerns**: Clear boundaries between layers

## Risk Assessment

### Low Risk Changes:
- Template function extraction ✅ 
- Database utility creation ✅
- Documentation improvements
- Test addition

### Medium Risk Changes:
- Service decomposition (requires careful interface design)
- Handler restructuring (affects HTTP API)
- Dependency injection introduction

### High Risk Changes:
- Database schema modifications
- External API integration changes
- Authentication/authorization changes

## Benefits Expected

### Developer Experience:
- **Faster Navigation**: Smaller, focused files
- **Easier Testing**: Single-purpose services
- **Better Onboarding**: Clear architectural boundaries
- **Reduced Bugs**: Single Responsibility Principle

### System Performance:
- **Better Resource Usage**: Proper connection pooling
- **Improved Caching**: Layered caching strategies
- **Better Monitoring**: Centralized logging and metrics
- **Easier Scaling**: Horizontal scaling preparation

### Maintenance Benefits:
- **Easier Debugging**: Clear error boundaries
- **Simpler Deployments**: Service isolation
- **Better Testing**: Unit test focused services
- **Documentation**: Self-documenting architecture

This restructuring plan transforms the current monolithic approach into a clean, enterprise-level architecture following Domain-Driven Design principles and Go best practices.