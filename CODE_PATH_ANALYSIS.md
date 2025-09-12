# Complete Code Path Analysis

## Entry Points and Flow Analysis

### 1. Main Application Entry Point: `main.go`

#### Flow A: Database Connection Success
```
main() → Database Connection → Service Initialization → HTTP Server Start
├── MongoDB Connection (database.NewMongoConnection)
├── Template Loading (templates.GetTemplateFuncs) ✅ REFACTORED
├── Service Creation:
│   ├── ESPNService (services.NewESPNService)
│   ├── DataLoader (services.NewDataLoader) 
│   ├── AuthService (services.NewAuthService)
│   ├── GameService (services.NewDatabaseGameService)
│   ├── PickService (services.NewPickService) ⚠️  NEEDS DECOMPOSITION
│   ├── VisibilityService (services.NewPickVisibilityService)
│   └── EmailService (services.NewEmailService)
├── Handler Creation:
│   ├── GameHandler (handlers.NewGameHandler) ⚠️  OVERSIZED (1,687 lines)
│   ├── AuthHandler (handlers.NewAuthHandler)
│   └── AnalyticsHandler (handlers.NewAnalyticsHandler)
├── Background Services:
│   ├── BackgroundUpdater (services.NewBackgroundUpdater) ⚠️  LARGE (561 lines)
│   ├── ChangeStreamWatcher (services.NewChangeStreamWatcher)
│   └── VisibilityTimerService (services.NewVisibilityTimerService)
└── HTTP Routes Setup → Server Start (port 8080/configured)
```

#### Flow B: Database Connection Failure  
```
main() → Database Connection FAILED → Fallback Mode
├── Demo Services (services.NewDemoGameService)
├── Limited Template Loading
├── Basic Routes (/games, /games/refresh)
└── HTTP Server Start (fallback mode)
```

### 2. HTTP Request Flow Analysis

#### GET / or /games (Dashboard)
```
HTTP Request → AuthMiddleware.OptionalAuth → GameHandler.GetGames
├── Debug DateTime Handling (if ?datetime param)
├── getCurrentWeek() calculation ⚠️  FIXED timezone issue
├── Game Data Retrieval:
│   ├── gameService.GetGames()
│   └── sortGamesByKickoffTime()
├── User Pick Data (if authenticated):
│   ├── pickService.GetUserPicksForWeek() ⚠️  COMPLEX METHOD
│   ├── visibilityService.FilterVisibleUserPicks() 
│   └── personalizeUserPicksForViewer()
└── Template Rendering → HTML Response
```

#### POST /submit-picks (Pick Submission)
```
HTTP Request → AuthMiddleware.RequireAuth → GameHandler.SubmitPicks
├── Form Parsing & Validation
├── Pick Processing Loop:
│   ├── pickService.CreatePick() for each pick
│   ├── Game validation against gameMap
│   └── Pick result enrichment
├── Parlay Score Processing:
│   ├── pickService.checkAndTriggerScoring() ⚠️  SHOULD USE ParlayService
│   └── pickService.ProcessWeekParlayScoring() ⚠️  SHOULD USE ParlayService  
├── SSE Broadcasting:
│   ├── broadcastPickUpdate() ⚠️  SHOULD USE SSEHandler
│   └── broadcastToAllClients()
└── HTTP Redirect Response
```

#### GET /events (Server-Sent Events)
```
HTTP Request → AuthMiddleware.OptionalAuth → GameHandler.SSEHandler
├── User Context Extraction
├── SSE Client Creation & Registration
├── Connection Management:
│   ├── sseClients map management ⚠️  SHOULD BE IN SSEHandler
│   └── Message Broadcasting Loop
└── Real-time Event Stream
```

### 3. Background Service Flows

#### ESPN API Updates (BackgroundUpdater)
```
BackgroundUpdater.Start() → Scheduled Updates (every 30 seconds)
├── ESPN API Fetch (espnService.FetchGames)
├── Game Data Processing:
│   ├── Data comparison with existing games
│   ├── BulkUpsertGames() to MongoDB
│   └── Change detection
├── Pick Result Processing (if games completed):
│   ├── pickService.ProcessGameCompletion() ⚠️  SHOULD USE ResultCalculationService
│   └── pickService.ProcessWeekParlayScoring() ⚠️  SHOULD USE ParlayService
└── SSE Broadcasting of Updates
```

#### Database Change Stream (ChangeStreamWatcher)
```
ChangeStreamWatcher.StartWatching() → MongoDB Change Stream
├── Change Event Detection
├── Event Processing:
│   ├── Game collection changes
│   └── Pick collection changes  
├── Handler Callback:
│   └── gameHandler.HandleDatabaseChange() ⚠️  SHOULD USE SSEHandler
└── SSE Broadcasting
```

#### Pick Visibility Timer (VisibilityTimerService)
```
VisibilityTimerService.Start() → Scheduled Visibility Updates
├── Upcoming Games Analysis
├── Pick Visibility Rule Calculation:
│   ├── Thursday 5pm PT rule
│   ├── Thanksgiving 10am PT rule  
│   ├── Weekend 10am PT rule
│   └── Modern season daily rules
├── Automatic Visibility Updates
└── SSE Broadcasting of Changes
```

### 4. Data Flow Analysis

#### Game Data Pipeline
```
ESPN API → ESPNService.FetchGames() → DataLoader → GameRepository
├── Raw ESPN JSON Processing
├── Game Model Transformation
├── Odds Data Extraction (spread, over/under)
├── Game Status Processing (live updates)
└── MongoDB Storage (BulkUpsertGames)
```

#### Pick Data Pipeline  
```
User Form → PickService.CreatePick() → Pick Validation → Pick Repository
├── Team ID Validation against Game
├── Pick Type Determination (spread/over-under)
├── Season/Week Assignment  
├── Pick Enrichment (team names, descriptions)
└── MongoDB Storage
```

#### Result Calculation Pipeline
```
Game Completion → BackgroundUpdater → Pick Result Processing
├── Game State Detection (completed)
├── Pick Retrieval for Game
├── Result Calculation:
│   ├── Spread Result Calculation ⚠️  SHOULD USE ResultCalculationService
│   ├── Over/Under Result Calculation ⚠️  SHOULD USE ResultCalculationService
│   └── Push/Win/Loss Determination
├── Pick Result Updates
└── Parlay Score Recalculation ⚠️  SHOULD USE ParlayService
```

## Critical Code Paths by Feature

### Feature: Pick Submission
**Current Path:**
```
GameHandler.SubmitPicks() → PickService.CreatePick() → MongoDB → SSE Broadcast
```
**Problems:**
- Handler doing business logic
- Pick service too large
- Direct SSE coupling

**Recommended Path:**
```
PickHandler.SubmitPicks() → PickService.CreatePick() → ResultCalculationService.ValidatePickAgainstGame() → NotificationService.BroadcastPickUpdate()
```

### Feature: Game Result Processing  
**Current Path:**
```
BackgroundUpdater → PickService.ProcessGameCompletion() → Parlay Calculation → SSE
```
**Problems:**
- Background updater doing business logic
- Pick service overloaded
- Tight coupling

**Recommended Path:**  
```
BackgroundUpdater → ResultCalculationService.ProcessGameCompletion() → ParlayService.ProcessWeekParlayScoring() → NotificationService.BroadcastUpdate()
```

### Feature: Analytics & Statistics
**Current Path:**
```
AnalyticsHandler → PickService.GetPickStats() → Manual aggregations
```
**Problems:**
- Analytics mixed with pick CRUD
- No comprehensive statistics
- Ad-hoc calculations

**Recommended Path:**
```
AnalyticsHandler → AnalyticsService.GetUserPerformanceStats() → Structured analytics
```

## Database Access Patterns

### Read Patterns
```
Games Collection:
├── GetGamesBySeason() - Dashboard, Analytics (FREQUENT)
├── GetGamesByWeekSeason() - Pick submission, Scoring (FREQUENT)
├── FindByESPNID() - ESPN updates (FREQUENT)
└── GetAllGames() - Less common

Picks Collection:  
├── GetUserPicksForWeek() - Dashboard (FREQUENT) ⚠️  MISSING METHOD
├── GetPicksByGameID() - Result processing (FREQUENT) ⚠️  MISSING METHOD
├── GetUserPicksBySeason() - Analytics (FREQUENT) ⚠️  MISSING METHOD
└── CreateMany() - Bulk operations, imports

Users Collection:
├── FindByID() - Authentication, Analytics ⚠️  MISSING METHOD  
├── FindByEmail() - Login (FREQUENT)
└── GetAllUsers() - Admin, Analytics
```

### Write Patterns
```
Games Collection:
├── BulkUpsertGames() - ESPN updates (every 30s)
└── UpsertGame() - Individual updates

Picks Collection:
├── Create() - Pick submission (USER DRIVEN)
├── UpdatePickResult() - Result processing ⚠️  MISSING METHOD
└── BulkUpdateResults() - Batch processing

Parlay Collection:
├── UpsertUserSeasonRecord() - Scoring updates ⚠️  MISSING METHOD
└── GetUserSeasonRecord() - Score retrieval ⚠️  MISSING METHOD
```

## Service Dependencies (Current vs Recommended)

### Current Dependencies (Problematic)
```
GameHandler ──┬── PickService (1,128 lines) ──┬── GameRepository
              │                               ├── PickRepository
              │                               ├── UserRepository  
              │                               └── ParlayRepository
              ├── AuthService
              ├── GameService
              ├── VisibilityService
              └── Direct SSE Management ❌
```

### Recommended Dependencies (Clean)
```
PickHandler ─── PickService (Core CRUD only) ─── PickRepository
             │
             ├─ ResultCalculationService ──────── GameRepository
             │
             ├─ ParlayService ─────────────────── ParlayRepository  
             │
             ├─ AnalyticsService ──────────────── All Repositories
             │
             └─ NotificationService (SSE) ────── SSEHandler
```

## Critical Issues Found

### 1. **Single Responsibility Principle Violations**
- ❌ `PickService`: 32 methods, 1,128 lines (pick CRUD + results + parlay + analytics)
- ❌ `GameHandler`: 27 methods, 1,687 lines (HTTP + SSE + business logic)
- ❌ `BackgroundUpdater`: 561 lines (API updates + business logic)

### 2. **Missing Repository Methods** 
- ❌ 8+ critical repository methods missing
- ❌ Parlay models don't exist
- ❌ Analytics queries not optimized

### 3. **Tight Coupling Issues**
- ❌ Handlers directly managing SSE connections
- ❌ Background services doing business logic
- ❌ Services directly calling other services' internal methods

### 4. **Database Inefficiencies**
- ❌ 20+ duplicate timeout patterns
- ❌ No connection pooling optimization
- ❌ Inconsistent error handling

## Validation of Critical User Journeys

### Journey 1: User Makes Pick ✅ WORKS
1. User visits dashboard → ✅ Authentication works
2. User sees games and picks → ✅ Visibility rules work  
3. User submits picks → ✅ Validation works
4. Other users see updates → ✅ SSE broadcasting works
**Status: FUNCTIONAL but needs refactoring**

### Journey 2: Game Completes → Results Calculated ✅ WORKS
1. ESPN API updates game → ✅ Background updater works
2. Game marked complete → ✅ Change stream detects
3. Pick results calculated → ✅ Result logic works
4. Parlay scores updated → ✅ Scoring logic works
5. Users see updates → ✅ SSE broadcasting works
**Status: FUNCTIONAL but needs decomposition**

### Journey 3: User Views Analytics ⚠️ LIMITED
1. User visits analytics page → ✅ Works
2. User sees basic stats → ✅ Limited stats available
3. Advanced analytics → ❌ Not comprehensive
**Status: NEEDS ENHANCEMENT with AnalyticsService**

## Conclusion

The codebase is **FUNCTIONAL** but suffers from severe architectural debt:

### ✅ **Working Well:**
- Core pick submission and validation
- Game data updates from ESPN
- Real-time SSE broadcasting  
- Pick visibility rules
- Basic parlay scoring

### ❌ **Major Problems:**
- Massive files violating SRP (1,000+ lines)
- Missing repository methods blocking service decomposition
- Tight coupling preventing independent testing
- Limited analytics and reporting capabilities

### 🎯 **Priority Actions:**
1. **Implement missing repository methods** (enables service decomposition)
2. **Complete service separation** (ParlayService, ResultCalculationService, AnalyticsService)  
3. **Extract handlers** (PickHandler, SSEHandler completion)
4. **Add comprehensive error handling and logging**

The refactoring work done so far creates the foundation for a clean architecture, but requires the missing repository methods to be functional.