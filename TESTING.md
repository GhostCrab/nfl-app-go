# NFL App Testing Guide

This document provides comprehensive testing instructions for the NFL Parlay Club application, including pick visibility, real-time SSE updates, and background polling systems.

## Table of Contents

1. [Setup for Testing](#setup-for-testing)
2. [API Endpoints](#api-endpoints)
3. [Command-Line Utilities](#command-line-utilities)
4. [Pick Visibility System](#pick-visibility-system)
5. [Real-Time SSE Updates](#real-time-sse-updates)
6. [Intelligent Background Polling](#intelligent-background-polling)
7. [Test Endpoints](#test-endpoints)
8. [Debug Utilities](#debug-utilities)
9. [Advanced Testing Scenarios](#advanced-testing-scenarios)
10. [Troubleshooting](#troubleshooting)

## Setup for Testing

### 1. Populate Test Data

First, populate the 2025 season with test picks for all users:

```bash
go run cmd/populate_test_picks.go
```

This creates 4-6 realistic picks per user per week for all 7 users (617 total picks).

### 2. Start the Server

```bash
go run main.go
```

The server will start:
- Visibility timer service
- Intelligent background polling
- SSE event streaming
- Change stream watchers

Look for these startup messages:
```
VisibilityTimerService: Started monitoring pick visibility changes
BackgroundUpdater: Starting intelligent background ESPN API polling
ChangeStream: Successfully connected to games collection
ChangeStream: Successfully connected to picks collection
```

## API Endpoints

### Authentication APIs
```bash
# Login API endpoint
curl -X POST "http://localhost:8080/api/login" \
  -H "Content-Type: application/json" \
  -d '{"email":"user@example.com","password":"password123"}'
```

### Game Data APIs
```bash
# Get games data (JSON)
curl "http://localhost:8080/api/games?season=2025&week=13"

# Get complete dashboard data (JSON)
curl "http://localhost:8080/api/dashboard?season=2025&week=13"

# Manual game refresh (HTML)
curl "http://localhost:8080/games/refresh?season=2025&week=13"
```

## Command-Line Utilities

### Data Management Scripts

#### 1. Populate Test Data
```bash
# Create test picks for all users (617 total picks)
go run cmd/populate_test_picks.go
```

#### 2. Clear Database
```bash
# WARNING: Deletes ALL data in MongoDB
go run cmd/clear_database.go
```

#### 3. Import Legacy Data
```bash
# Import historical data from JSON files
go run cmd/import_legacy.go
```

#### 4. Full Database Refresh
```bash
# Complete database refresh with legacy import
go run scripts/full_database_refresh.go
```

#### 5. Recalculate Parlay Scoring
```bash
# Recalculate all parlay scores across all seasons
go run scripts/recalculate_parlay_scoring.go
```

### Debug and Analysis Scripts

#### 6. Debug Pick Results
```bash
# Analyze pick result calculations for specific scenarios
go run cmd/debug_pick_results.go
```

#### 7. Debug Specific Games
```bash
# Debug Cleveland vs Pittsburgh game data
go run cmd/debug_cle_pit.go

# Debug Week 8 picks specifically
go run cmd/debug_picks_week8.go
```

#### 8. Debug IDs and Mappings
```bash
# Debug game ID mappings
go run cmd/debug_game_ids.go

# Debug team ID mappings
go run cmd/debug_team_ids.go
```

#### 9. Test Pick Enrichment
```bash
# Test pick data enrichment processes
go run cmd/test_pick_enrichment.go
```

#### 10. Fix Database Indexes
```bash
# Repair or optimize MongoDB indexes
go run cmd/fix_indexes.go
```

## Pick Visibility System

### Overview

The pick visibility system implements these rules:
- **Own picks**: Always visible to the user
- **Thursday games**: Visible to all at 5:00 PM PT (10:00 AM PT on Thanksgiving)
- **Friday games**: Visible at 10:00 AM PT on Saturday
- **Saturday games**: Visible at 10:00 AM PT on Saturday  
- **Sunday/Monday games**: Visible at 10:00 AM PT on Sunday
- **In-progress/Completed games**: Always visible to everyone
- **Hidden picks**: Show day-grouped counts without revealing actual picks

### Testing Scenarios

#### 1. Basic Visibility Testing

Test different times using the debug datetime parameter (all times Pacific):

```bash
# Thursday at 4:00 PM PT (before 5:00 PM threshold)
http://localhost:8080/?datetime=2025-09-11T16:00

# Thursday at 6:00 PM PT (after 5:00 PM threshold)
http://localhost:8080/?datetime=2025-09-11T18:00

# Saturday at 9:00 AM PT (before 10:00 AM threshold)
http://localhost:8080/?datetime=2025-09-13T09:00

# Saturday at 11:00 AM PT (after 10:00 AM threshold)
http://localhost:8080/?datetime=2025-09-13T11:00
```

#### 2. Thanksgiving Special Rules

```bash
# Thanksgiving Thursday at 9:00 AM PT (before 10:00 AM threshold)
http://localhost:8080/?datetime=2025-11-27T09:00

# Thanksgiving Thursday at 11:00 AM PT (after 10:00 AM threshold)  
http://localhost:8080/?datetime=2025-11-27T11:00
```

#### 3. Sunday/Monday Visibility

```bash
# Sunday at 9:00 AM PT (before threshold - Monday picks still hidden)
http://localhost:8080/?datetime=2025-11-30T09:00

# Sunday at 11:00 AM PT (after threshold - both Sunday and Monday picks visible)
http://localhost:8080/?datetime=2025-11-30T11:00
```

#### 4. In-Progress Game Testing

```bash
# Set time to during a game to trigger demo "in-progress" state
# Example: Friday game at 12:00 PM, test at 12:01 PM (should be visible due to in-progress override)
http://localhost:8080/?datetime=2025-11-28T12:01
```

## Real-Time SSE Updates

### Overview

The application uses Server-Sent Events (SSE) for real-time updates:
- **Pick updates**: When users submit picks
- **Game updates**: When game scores/states change  
- **Visibility changes**: When pick visibility thresholds are reached
- **Week filtering**: Only processes events for the currently viewed week

### SSE Event Types

1. **`user-picks-updated`** - Triggers when picks are submitted
2. **`gameUpdate`** - Triggers when game data changes
3. **`gameScoreUpdate`** - Triggers for individual game score updates
4. **`visibility-change`** - Triggers when pick visibility changes
5. **`dashboard-update`** - General dashboard refresh

### Testing Real-Time Updates

#### 1. Multi-Browser Pick Updates

1. Open the dashboard in **2 browsers**
2. Log in as different users in each browser
3. Submit picks in one browser
4. **Both browsers should update immediately** showing the new picks

#### 2. Game Update Testing

1. Open dashboard in **2 browsers** 
2. Navigate to the same week (e.g., `?week=13&season=2025`)
3. Trigger a test game update (see [Test Endpoints](#test-endpoints))
4. **Both browsers should refresh games automatically**

#### 3. Week Filtering

1. Open dashboard on **different weeks** in different tabs:
   - Tab 1: `?week=12&season=2025`  
   - Tab 2: `?week=13&season=2025`
2. Trigger game update for week 13
3. **Only Tab 2 should update** (week filtering working)

## Intelligent Background Polling

### Polling Intervals

The background updater uses intelligent scheduling:

- **Live games**: 5 seconds (highest priority)
- **Games starting soon** (within 2 hours): 30 seconds  
- **Current week games**: 30 minutes
- **Next week games**: 6 hours
- **Future weeks**: 24 hours

### Monitoring Polling

Check server logs for polling status changes:

```bash
# When games go live
BackgroundUpdater: Switching to live-games polling (interval: 5s)

# When games end
BackgroundUpdater: Switching to current-week polling (interval: 30m0s)

# Off-season
BackgroundUpdater: Switching to future-weeks polling (interval: 24h0m0s)
```

## Test Endpoints

### 1. Game Update Testing

**Test game score updates:**
```bash
curl -X POST "http://localhost:8080/games/test-update?gameId=401671686&type=score"
```

**Test game state changes:**
```bash  
curl -X POST "http://localhost:8080/games/test-update?gameId=401671686&type=state"
```

**Test live game updates:**
```bash
curl -X POST "http://localhost:8080/games/test-update?gameId=401671686&type=live"
```

Expected response:
```json
{
  "success": true,
  "message": "Test score update triggered for game 401671686",
  "gameId": 401671686,
  "type": "score"
}
```

### 2. Manual Game Refresh

**Refresh games for specific week:**
```bash
curl "http://localhost:8080/games/refresh?season=2025&week=13"
```

### 3. SSE Connection Testing

**Connect to SSE stream:**
```bash
curl -N -H "Accept: text/event-stream" "http://localhost:8080/events"
```

Expected output:
```
data: SSE connection established for user 0

event: gameUpdate
data: {"type":"databaseChange","collection":"games","operation":"update","season":2025,"week":13}
```

## Debug Utilities

### Debug Parameters

#### Debug DateTime Parameter
Control time-based features for testing:
```bash
# Test different times (Pacific timezone)
http://localhost:8080/?datetime=2025-09-11T16:00

# Test with specific week
http://localhost:8080/?week=13&season=2025&datetime=2025-11-28T12:01
```

The debug parameter enables:
- Pick visibility testing
- In-progress game simulation
- Realistic game state generation
- Thanksgiving special rules testing

#### URL Parameters
```bash
# Navigation parameters
?week=13&season=2025        # Navigate to specific week
?debug=true                 # Enable debug mode
?datetime=2025-11-28T12:01  # Override current time (Pacific)

# Combined examples
?week=13&season=2025&datetime=2025-11-28T12:01&debug=true
```

### Development Features

#### Environment Variables
```bash
# Set development mode for detailed logging
ENVIRONMENT=development

# Database configuration
DB_HOST=p5server
DB_PORT=27017
DB_USERNAME=nflapp
DB_PASSWORD=password

# Email configuration (for testing password resets)
SMTP_HOST=smtp.gmail.com
SMTP_PORT=587
SMTP_EMAIL=test@example.com
SMTP_PASSWORD=app-password
```

#### Logging Levels
The application provides extensive logging:
```bash
# Server startup
2025/08/30 00:04:43 Server starting in development mode (isDevelopment: true)

# SSE connections
SSE: New client connected from 127.0.0.1:12345 (UserID: 1)
SSE: Broadcasted gameUpdate to 2 clients

# Background polling
BackgroundUpdater: Switching to live-games polling (interval: 5s)

# Pick visibility
DEBUG: Set debug datetime to 2025-09-11 16:00:00 PDT
VisibilityTimerService: Next visibility change at 2025-09-11 17:00:00 PDT

# Game state simulation  
SIMULATION: Game 123 - Updated scores (Home: 21, Away: 14)
```

### Test Data Management

#### Default Users
The application includes 7 test users:
- **CLARK** (ID: 1) - password123
- **RANDY** (ID: 2) - password123
- **BRAD** (ID: 3) - password123
- **MIKE** (ID: 4) - password123
- **JASON** (ID: 5) - password123
- **KEVIN** (ID: 6) - password123
- **RYAN** (ID: 7) - password123

#### Test Pick Distribution
When using `cmd/populate_test_picks.go`:
- 617 total picks across all users
- 4-6 picks per user per week
- Realistic team selections
- Proper distribution across pick types (spread, over/under, bonus)

### Database Testing

#### MongoDB Collections
```bash
# View collections
mongo nfl_app --eval "db.getCollectionNames()"

# Count documents
mongo nfl_app --eval "db.games.count()"
mongo nfl_app --eval "db.picks.count()"
mongo nfl_app --eval "db.users.count()"

# Sample data
mongo nfl_app --eval "db.games.findOne()"
mongo nfl_app --eval "db.picks.findOne()"
```

#### Legacy Data Structure
The application supports importing from legacy JSON files:
```
legacy-dbs/
├── 2023_games/     # Historical game data
├── 2023_picks/     # Historical pick data
├── 2024_games/     # Recent game data
└── 2024_picks/     # Recent pick data
```

### Complete Testing Workflows

#### Full System Test (Recommended)
```bash
# 1. Clean slate
go run cmd/clear_database.go

# 2. Import fresh data
go run cmd/import_legacy.go

# 3. Populate test picks
go run cmd/populate_test_picks.go

# 4. Start server
go run main.go

# 5. Test in browser
# - Open http://localhost:8080/?week=13&season=2025
# - Log in as different users
# - Test pick submissions
# - Test SSE updates
```

#### Quick SSE Test
```bash
# Terminal 1: Start server
go run main.go

# Terminal 2: Connect to SSE
curl -N -H "Accept: text/event-stream" "http://localhost:8080/events"

# Terminal 3: Trigger update
curl -X POST "http://localhost:8080/games/test-update?gameId=123&type=score"

# Should see SSE event in Terminal 2
```

#### Pick Visibility Test Sequence
```bash
# 1. Populate data
go run cmd/populate_test_picks.go

# 2. Test Thursday before 5 PM PT
curl "http://localhost:8080/?datetime=2025-09-11T16:00"

# 3. Test Thursday after 5 PM PT  
curl "http://localhost:8080/?datetime=2025-09-11T18:00"

# 4. Test weekend visibility
curl "http://localhost:8080/?datetime=2025-09-13T11:00"
```

## Advanced Testing Scenarios

### 1. Complete SSE Workflow

1. **Setup**: Open 2 browsers to same week
2. **Connect**: Both establish SSE connections
3. **Trigger**: `curl -X POST "http://localhost:8080/games/test-update?gameId=123&type=score"`
4. **Observe**: Both browsers receive update and refresh games
5. **Verify**: Check browser dev tools for SSE messages

### 2. Week Filtering Validation

1. **Setup**: Open browsers to different weeks
   - Browser A: `http://localhost:8080/?week=12&season=2025`
   - Browser B: `http://localhost:8080/?week=13&season=2025`
2. **Trigger**: Update for week 13 games
3. **Expected**: Only Browser B updates, Browser A filters out the event
4. **Console**: Check for "SSE: Filtering out gameUpdate" messages

### 3. Pick Visibility Real-Time

1. **Setup**: Set debug time just before threshold
   ```
   http://localhost:8080/?datetime=2025-09-13T09:59
   ```
2. **Wait**: Let server time advance or manually advance debug time
3. **Observe**: Picks automatically become visible via SSE updates
4. **Verify**: No manual refresh needed

### 4. Intelligent Polling Behavior

1. **Monitor logs** during different times:
   - Off-season: 24-hour intervals
   - Regular season: 30-minute intervals  
   - Game day: 30-second intervals
   - Live games: 5-second intervals

2. **Simulate game states** to trigger polling changes:
   ```bash
   # This should switch to live-games polling (5s)
   curl -X POST "http://localhost:8080/games/test-update?gameId=123&type=state"
   ```

### 5. Multi-User Pick Testing

1. **Create test scenario**:
   - User A: Has Thursday picks
   - User B: Has Sunday picks
   - Current time: Thursday 4 PM PT

2. **Expected behavior**:
   - User A sees own Thursday picks
   - User B sees "Thursday: 1 pick" (hidden)
   - User B sees own Sunday picks
   - User A sees "Sunday/Monday: 2 picks" (hidden)

## Troubleshooting

### No Picks Showing
```bash
# Repopulate test data
go run cmd/populate_test_picks.go

# Check database connection
2025/08/30 00:04:43 Successfully connected to MongoDB at p5server:27017
```

### Debug Time Not Working
- Use format: `YYYY-MM-DDTHH:MM` (24-hour time)
- Time is interpreted as Pacific timezone
- Check server logs: `DEBUG: Set debug datetime to 2025-09-11 16:00:00 PDT`

### SSE Updates Not Working

**Check SSE connection:**
```bash
# Browser dev tools -> Network -> Look for /events connection
# Should show "EventStream" type with persistent connection
```

**Check server logs:**
```bash
SSE: New client connected from 127.0.0.1:12345 (UserID: 1)
SSE: Broadcasted gameUpdate to 2 clients
```

**Check HTMX extensions:**
```html
<!-- Verify these are loaded -->
<script src="https://unpkg.com/htmx.org@1.9.10"></script>
<script src="https://unpkg.com/htmx.org@1.9.10/dist/ext/sse.js"></script>
```

### Week Filtering Not Working

**Check browser console:**
```javascript
// Should see filtering messages
SSE: Filtering out gameUpdate for season 2025, week 12 (current: season 2025, week 13)
SSE: Processing gameUpdate for season 2025, week 13 (matches current view)
```

**Check data attributes:**
```html
<!-- Verify these are set correctly -->
<div sse-swap="gameUpdate" data-current-season="2025" data-current-week="13">
```

### Background Polling Issues

**Check polling status:**
```bash
# Look for polling type changes
BackgroundUpdater: Switching to live-games polling (interval: 5s)
BackgroundUpdater: Error fetching games for intelligent scheduling: connection timeout
```

**Check game state detection:**
```bash
# Verify games are properly classified
BackgroundUpdater: Found 2 in-progress games, switching to live polling
```

### Game Update Simulation

**Verify test endpoints:**
```bash
# Should return JSON success response
curl -X POST "http://localhost:8080/games/test-update?gameId=123&type=score"
```

**Check simulation logs:**
```bash
SIMULATION: Game 123 - Updated scores (Home: 21, Away: 14)
SSE: Broadcasted gameUpdate to 3 clients
```

## Production Considerations

### Security
- Remove or restrict debug datetime parameter access
- Implement rate limiting for test endpoints
- Secure SSE connections with proper authentication

### Performance
- Monitor SSE client connections (avoid memory leaks)
- Consider scaling for high-frequency live game updates
- Monitor background polling impact on ESPN API limits

### Monitoring
- Track SSE connection counts
- Monitor polling frequency changes
- Alert on failed game updates or SSE disconnections

### Timezone Handling
- Ensure proper Pacific timezone conversion for all users
- Test pick visibility across different user timezones
- Verify Thanksgiving detection works correctly each year