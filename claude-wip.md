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