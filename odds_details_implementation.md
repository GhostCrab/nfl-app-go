# Odds Details Dashboard Implementation

## Overview
Creating a comprehensive weekly odds tracking dashboard that displays real-time odds data from ESPN API (provider 58 - ESPN BET) and compares them with locked-in database odds.

## Requirements
1. Display comprehensive odds data for all games in the current week
2. Allow switching between weeks
3. Use in-memory caching (2-hour expiration)
4. Compare live ESPN odds vs database "locked-in" odds (locked at 10pm PT Wednesday)
5. No database storage of API data - all odds fetched on-demand
6. Fetch and display data from ALL Section 7 ESPN API endpoints:
   - Current odds (spread, over/under)
   - Opening odds and line movement tracking (pre-kickoff only)
   - Odds history showing movement over time (pre-kickoff)
   - Team historical performance against spreads/totals
7. Hide all provider information from end users (internal only)

## Technical Design

### Data Flow
```
User Request → Handler (Check Cache) → Fetch Current Week Games from DB
                  ↓
            Cache Valid? (< 2 hours old)
                  ↓ No
            Fetch Live Odds from ESPN API (Provider 58)
                  ↓
            Compare with DB Locked-In Odds
                  ↓
            Store in Memory Cache
                  ↓
            Render HTML Dashboard
```

### Key Components

#### 1. In-Memory Cache Structure
- Cache Key: Week number + Season year
- Cache Value: OddsData structure with games, odds, timestamp
- Expiration: 2 hours
- Thread-safe with mutex

#### 2. ESPN API Integration
- Endpoint: `https://sports.core.api.espn.com/v2/sports/football/leagues/nfl/events/{gameID}/competitions/{gameID}/odds/58`
- Provider: 58 (ESPN BET)
- Data: Spread, Over/Under, Open/Current odds

#### 3. Database Odds Comparison
- Fetch game odds from MongoDB (locked in at 10pm PT Wednesday)
- Compare:
  - DB Spread vs Live Spread
  - DB Over/Under vs Live Over/Under
  - Show differences if past lock time

#### 4. Lock Time Logic
- Lock Time: Wednesday 10pm Pacific Time
- Display comparison only if current time > lock time for the week
- Otherwise, show live odds only

## Implementation Tasks

### Task 1: Create odds_detail_handler.go
**Status:** Pending
**Details:**
- OddsDetailHandler struct with cache, mutex, ESPN service, game repo
- Data structures for cache entries
- Handler methods for GET requests

### Task 2: Implement In-Memory Cache
**Status:** Pending
**Details:**
- Thread-safe map with mutex
- 2-hour expiration check
- Cache invalidation logic

### Task 3: Add ESPN API Method
**Status:** Pending
**Details:**
- GetDetailedOdds(gameID) method in espn.go
- Parse full odds response (spread, OU, open, current)
- Handle provider 58 specifically

### Task 4: Implement Weekly Odds Fetching
**Status:** Pending
**Details:**
- Get current week using models.GetNFLWeekForDate()
- Fetch all games for week from database
- Fetch live odds for each game from ESPN
- Handle errors gracefully

### Task 5: Add Locked-In Odds Comparison
**Status:** Pending
**Details:**
- Calculate Wednesday 10pm PT for the displayed week
- Compare DB odds vs live odds if past lock time
- Calculate deltas (spread movement, OU movement)
- Flag significant line movements

### Task 6: Create HTML Template
**Status:** Pending
**Details:**
- Weekly odds table with game info
- Live odds display
- Locked-in odds comparison (if applicable)
- Line movement indicators
- Week selector/navigation

### Task 7: Register Routes
**Status:** Pending
**Details:**
- GET /odds-details (current week)
- GET /odds-details?week=X (specific week)
- Register in main.go

## Data Structures

### OddsCache
```go
type OddsCache struct {
    data      map[string]*CachedOddsData
    mutex     sync.RWMutex
}

type CachedOddsData struct {
    Week      int
    Season    int
    Games     []GameOddsInfo
    Timestamp time.Time
}

type GameOddsInfo struct {
    Game          models.Game
    LiveOdds      *DetailedOdds
    LockedOdds    *models.Odds  // From DB
    IsLocked      bool
    SpreadDelta   float64
    OUDelta       float64
}

type DetailedOdds struct {
    Spread        float64
    OverUnder     float64
    OpenSpread    float64
    OpenOverUnder float64
    Details       string
}
```

## ESPN API Response Structure (Provider 58)
Based on test call to game 401772941:
```json
{
  "provider": {"id": "58", "name": "ESPN BET", "priority": 1},
  "details": "PIT -5.5",
  "spread": -5.5,
  "overUnder": 42.5,
  "overOdds": -115,
  "underOdds": -105,
  "awayTeamOdds": {
    "moneyLine": -260,
    "spreadOdds": -110,
    "open": {"moneyLine": {...}, "pointSpread": {...}},
    "current": {"moneyLine": {...}, "pointSpread": {...}}
  },
  "homeTeamOdds": {
    "moneyLine": 215,
    "spreadOdds": -110,
    "open": {...},
    "current": {...}
  },
  "open": {
    "total": {"alternateDisplayValue": "42.5"}
  },
  "current": {
    "total": {"alternateDisplayValue": "42.5"}
  }
}
```

## UI Features
- Week navigation (prev/next buttons)
- Current week indicator
- Games sorted by date
- Color-coded line movements:
  - Green: Favorable movement
  - Red: Unfavorable movement
  - Gray: No movement / not locked yet
- Refresh timestamp display
- Manual refresh button

## Lock Time Calculation
```
Week Start Date → Wednesday of that week → 10:00 PM Pacific
```

Example:
- Week 1 starts Sept 5, 2024
- Lock time: Wednesday Sept 11, 2024 10:00 PM PT
- Show comparison only if current time > lock time

## Error Handling
- If ESPN API fails for a game, show "N/A" for live odds
- If no games in week, show friendly message
- Cache failures don't block page load
- Graceful degradation

## Implementation Summary

### Completed Tasks
1. ✅ Created comprehensive documentation
2. ✅ Implemented handler with in-memory cache (2-hour expiration)
3. ✅ Added ESPN API integration for all Section 7 endpoints:
   - Current/Opening odds from provider 58 (ESPN BET)
   - Odds movement/history endpoint
   - Team ATS (Against The Spread) records via odds-records endpoint
4. ✅ Built HTML template with comprehensive odds dashboard
5. ✅ Registered routes in main.go
6. ✅ Added locked-in odds comparison logic

### Files Created/Modified

#### New Files:
- `handlers/odds_detail_handler.go` - Handler with caching and data aggregation
- `templates/odds_details.html` - Comprehensive odds dashboard UI
- `odds_details_implementation.md` - This documentation file

#### Modified Files:
- `services/espn.go` - Added comprehensive odds fetching methods:
  - `GetComprehensiveOdds()` - Main method hitting multiple endpoints
  - `fetchCurrentAndOpenOdds()` - Fetches current and opening odds
  - `fetchOddsMovement()` - Fetches odds history (if available)
  - `fetchTeamATSStats()` - Fetches team ATS records
  - `GetESPNTeamID()` - Helper to convert team abbreviations to ESPN IDs
- `main.go` - Registered odds details handler and route

### ESPN API Endpoints Used

1. **Current/Opening Odds:**
   ```
   GET /v2/sports/football/leagues/nfl/events/{gameID}/competitions/{gameID}/odds/58
   ```
   - Provider 58 = ESPN BET
   - Returns current spread, over/under, opening odds

2. **Odds Movement/History:**
   ```
   GET /v2/sports/football/leagues/nfl/events/{gameID}/competitions/{gameID}/odds/58/history/0/movement
   ```
   - Returns historical odds snapshots (may be empty for some games)

3. **Team ATS Records:**
   ```
   GET /v2/sports/football/leagues/nfl/seasons/{season}/types/0/teams/{teamID}/odds-records
   ```
   - Returns comprehensive betting records including ATS performance
   - Includes overall, home/away, favorite/underdog breakdowns

### Features Implemented

**Dashboard Features:**
- Week navigation (previous/next, dropdown selector)
- Keyboard shortcuts (←/→ for week navigation, R for refresh)
- Cache information display (last updated, expiration time)
- Lock time indicator (Wed 10pm PT)

**Per-Game Display:**
- Current spread with line movement indicator
- Current over/under with line movement indicator
- Opening odds comparison
- Locked odds comparison (if past lock time)
- Team season ATS records with percentages
- Error handling for failed API calls

**Data Management:**
- In-memory caching with 2-hour expiration
- Thread-safe cache access with mutex
- Graceful degradation when endpoints fail
- No database storage of API data

### Access
- **URL:** `/odds-details`
- **Query Parameters:**
  - `week` - Week number (1-18), defaults to current week
  - `season` - Season year, defaults to 2025

### Notes
- Provider information (ESPN BET) is not exposed to end users
- Odds movement history endpoint may return empty data for some games
- ATS statistics use season-wide records (type 0)
- Lock time calculated as Wednesday 10pm PT of game week
- Cache expires after 2 hours to keep data relatively fresh

## Testing
To test the implementation:
1. Navigate to `/odds-details` in browser
2. Test week navigation using buttons or keyboard arrows
3. Verify odds data displays correctly
4. Check ATS records for teams
5. Compare locked vs current odds (if past Wednesday lock time)
6. Test cache by refreshing before 2-hour expiration
