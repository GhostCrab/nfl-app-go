# Claude Code - NFL App Go - Work in Progress

## Project Overview
Building an NFL scores and betting app in Go with HTMX, recreating functionality from the original TypeScript/Angular parlay-club stack. The app displays NFL games, scores, betting odds, and spread results with a clean dark/light mode interface.

## Current Status - Session End Summary

### ✅ Completed This Session
1. **Research free NFL API options for demo data** - Found ESPN API endpoints
2. **Implement ESPN API client in Go** - Created services/espn.go with scoreboard fetching
3. **Create data service interface for easy API swapping** - Built GameService interface with Demo/Real implementations
4. **Add score manipulation for live demo simulation** - DemoGameService modifies 2024 data to appear live
5. **Reduce vertical spacing in game layout** - Compressed game cards for better density
6. **Implement dark mode with toggle** - CSS variables + localStorage persistence + FOUC prevention
7. **Add odds data structure to Game model** - Odds struct with spread/OU, helper methods
8. **Implement ESPN odds endpoint client** - GetOdds() method with JSON parsing fixes
9. **Update DemoGameService to enrich games with odds** - Limited to 20 games to avoid hanging
10. **Add odds display to game cards** - Shows spread (-3.5, +7.0, PK) and O/U (45.5)
11. **Add spread result indicators (covered/push)** - Color-coded badges and borders for completed games

### 🟡 Pending Tasks
1. **Add HTMX auto-refresh for live score simulation** - Auto-update scores every 30-60 seconds
2. **Historical odds data integration** - User will provide old odds when ESPN API fails for 2024 games
3. **Optimize odds fetching** - Currently tries 20 games sequentially, could parallelize
4. **Add team logos/colors** - Enhance visual design with official team branding
5. **Implement user picks/betting interface** - Core parlay club functionality
6. **Add weekly navigation** - Filter games by week
7. **Mobile responsive improvements** - Fine-tune for smaller screens

## Technical Architecture

### Key Files
- `main.go` - HTTP server setup, routes
- `models/game.go` - Game, Odds structs with business logic
- `models/team.go` - Team data structure
- `services/espn.go` - ESPN API client (scoreboard + odds)
- `services/game_service.go` - Service interfaces, DemoGameService
- `handlers/games.go` - HTTP handlers with logging
- `templates/games.html` - HTMX-ready HTML template
- `static/style.css` - Dark/light mode CSS with odds styling

### API Integration
- **ESPN Scoreboard**: `https://site.api.espn.com/apis/site/v2/sports/football/nfl/scoreboard?dates=20240701-20250131&limit=1000`
- **ESPN Odds**: `https://sports.core.api.espn.com/v2/sports/football/leagues/nfl/events/{gameID}/competitions/{gameID}/odds`
- **Data Flow**: ESPN API → Filter regular season → Enrich with odds → Demo modifications → Display

### Current Logs Output
```
ESPN API: Fetching scoreboard from [URL]
ESPN API: Received 333 events, Converted to 272 games
ESPN Odds: Attempting to fetch odds for up to 20 games
ESPN Odds: Successfully got odds for game X (spread: -3.5, o/u: 45.5)
```

## Issues & Solutions Found
1. **Page hanging** - Fixed by limiting odds fetching to 20 games vs all 272
2. **JSON parsing errors** - Changed ESPN odds struct fields from int to float64
3. **Dark mode flash** - Added pre-render theme detection in <head>
4. **Windows firewall popup** - Bound server to localhost only (127.0.0.1:8080)

## Session End Prompt Template

```
write off to claude-wip.md any todo prompts that I might need after I shut down for the night including what we've been working on and potential tasks to work on next session. also include a prompt that is very similar to what I'm writing now so I can paste it into the end of each session
```

## Quick Restart Commands

### Windows 11
```cmd
cd C:\Users\ryanp\Workspace\nfl-app-go
set DB_PASSWORD=your_actual_password && go run main.go
```

Or in PowerShell:
```powershell
cd C:\Users\ryanp\Workspace\nfl-app-go
$env:DB_PASSWORD="your_actual_password"; go run main.go
```

### General
```bash
# Visit: http://localhost:8080
```

---
*Last updated: Session end - NFL app with odds display and spread results implemented*

======== prompts
    REMINDER: Project Constraints

    - HTMX Best Practices: Follow https://htmx.org/docs/, https://htmx.org/extensions/sse/, https://htmx.org/attributes/hx-swap-oob/ as the bible
    - Minimal JavaScript: Server-side rendering, HTMX attributes over custom JS
    - Current Stack: Go, MongoDB, HTMX, SSE for real-time updates
    - I'm always running the app and using the port, you can try using a different port when you run, or you can just check logs without the port just to make sure it starts up properly

    Please start by reviewing HTMX documentation, especially SSE handling, before making any changes.


==================
  Context: NFL picks application with HTMX, Go backend, MongoDB. Working on real-time pick updates via Server-Sent Events.

  Current Status:
  - ✅ Pick submissions work and save to database correctly
  - ✅ Team names display properly (fixed ESPN team ID mapping)
  - ✅ Week selector works for navigation
  - ✅ SSE events are being sent and received by HTMX
  - ❌ Main Issue: Dashboard goes blank when submitting picks due to SSE processing conflicts

  Architecture:
  - Frontend: HTMX with SSE extension for real-time updates
  - Backend: Go handlers send precision-targeted SSE updates (only changed user's picks)
  - Database: MongoDB with proper team ID mapping
  - Template: Uses hx-swap-oob="outerHTML" for targeted DOM updates

  Recent Changes Made:
  1. Fixed team names: Updated handlers to use proper ESPN team IDs instead of generated ones
  2. Optimized SSE payloads: Only sends specific user's picks that changed (not all users)
  3. Added debug logging: Console logs show HTMX is receiving and processing SSE events correctly
  4. Template cleanup: Removed excessive whitespace to minimize SSE response size

  Current Issue:
  The SSE response format is correct and HTMX processes it (console shows "OOB swap completed"), but there's a double processing conflict causing dashboard to go blank.

  Latest Approach:
  Added hx-swap="none" to the dashboard container with sse-swap="pickUpdate" to prevent content being swapped into the container while still processing hx-swap-oob attributes.

  Files Modified:
  - /home/ryanp/nfl-app-go/templates/dashboard.html (SSE setup, debug logging)
  - /home/ryanp/nfl-app-go/handlers/games.go (team ID mapping, SSE broadcasting)

  Next Steps: Test if hx-swap="none" resolves the double processing issue and allows proper out-of-band swapping.