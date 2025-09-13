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

    I noticed the conversation compacted, let me remind you of these very important guidelines for this project:

    - HTMX Best Practices: Follow https://htmx.org/docs/, https://htmx.org/extensions/sse/, https://htmx.org/attributes/hx-swap-oob/ as the bible
    - Minimal JavaScript: Server-side rendering, HTMX attributes over custom JS
    - Current Stack: Go, MongoDB, HTMX, SSE for real-time updates
    - you're allowed to "go run ." to check compilation, but any functionality will be tested by me
    - IMPORTANT: When searching for code, always use grep -r "pattern" . or the Grep tool without path restrictions to search the entire codebase first. Don't waste time with complex searches or specific paths - just search everything immediately.
    - IMPORTANT: I've seen countless times where claude uses `Search(pattern: "<SEARCH_TERM>", output_mode: "content")` and it always fails. just use `Bash(grep -r "<SEARCH_TERM>" .)` instead
    
    Please start by reviewing HTMX documentation, especially SSE handling, before making any changes.


    - I'm always running the app and using the port, you can try using a different port when you run, or you can just check logs without the port just to make sure it starts up properly

========================
so far claude code has written 99% of the codebase in this project. There have been many instances where you have re-written or duplicated code to fix issues that have arisen but the old problematic code has not been removed. I would like to begin a full audit of the codebase starting from ingres points and main functions that process code that is being used and remove unused data paths or functions and properly identify and document used code. I have made a commit at the current stage because I anticipate a lot of refacotring and removing of code. Just go ahead and start auditing the code from the ingress points and start to refactor and break out code in logical blocks to new files to make it easier to read and update for a new engineer.

ok seems to still work. go further beyond the main.go file to identify duplicated code and redundant/vestigial code paths. I want a full audit of the ENTIRE code base. REPEAT: ENTIER CODE BASE. audit the whole thing. compare file layouts to enterprise level golang codebases and reconfigure the current codebase to be matching that level of granularity. follow every code path and hilight them as real and prune unused ones. make sure every function and structure is properly documented in code to show what it does and how it's used. go all out and dont stop until  you're out of tokens. Refrence/update ENTERPRISE_RESTRUCTURE_PLAN.md to see what other conversations have done so far and use it as a live checklist to track what is yet to be done.

review CODE_PATH_ANALYSIS.md, CODEBASE_AUDIT_RESULTS.md, ENTERPRISE_RESTRUCTURE_PLAN.md, FINAL_COMPREHENSIVE_AUDIT_REPORT.md, and MISSING_METHODS_AND_REFACTORING.md. Some of those documents may be a little out of date. update and consolidate where needed to keep track of the progress of the current refactor.

review and keep up to date CONSOLIDATED_REFACTOR_STATUS.md for current task tracking

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


=====================================
> I have a major change to how scores are tallied and picks are displayed. It will only apply to the 2025 season, so we will need to retain the current display templating and scoring management for the 2023 and 2024 seasons. I would like to re-use and not have duplicate 
  code as much as possible, but at the same time, sequester legacy-specific templating and code in its own files or other places so it's easy to see the "modern" code on it's own.\
  Here's the change:\
  currently, scoring is done mostly ona weekly basis. bets are made for the week and once the games are finished for the week, a user's bets for the week are tallied and if all bets win, the user gets points, if a single bet loses, the user gets no points. There are 
  special circumstances for push bets. There are a couple weeks (first week and thanksgiving week) where we have "bonus" days that are treated like their own "week" for scoring purposes. Wining a parlay on those single days wins points and arent affected by the games in 
  the rest of the week. The new scoring/display change is this: There are no more 'bonus' days for the 2025 (and future) seasons. Instead, every day will be scored individually, as if every day is a 'bonus' day. "weeks" for scoring purposes dont exist anymore and are only 
  used as a bucket for display purposes. An example is week 1 of this season there is 1 game thursday, 1 game friday, many games sunday, and 1 game monday. Users should make 2 picks for each of those days (more on sunday if they would like) and if they lose picks on friday
   and monday, for example, but hit 2 pick parlays on thursday and sunday, they would have gotten 4 points for that week. Right now we have special code for displaying "bonus thursday" and "bonus friday" in the pick list. The new paradigm should have sections for each day 
  that there are picks. The picker modal should have dividers or some way to distinguish individual days so it's easier to make picks for that week. In code, attempt to template and re-use code as much as possible by bucketing days and then running the same code for each 
  days. There's probably a lot of code for special bonus thursday and friday games that should be stripped out since all days with games should be treated the same way. Ask me more questions about code architecture or game rules if you have any.

● This is a significant architectural change! I have several questions to make sure I understand the requirements correctly and can design the best approach:

  Scoring & Game Rules Questions:

  1. Daily Parlay Rules: For each day, users still need a minimum of 2 picks to form a valid parlay? And they can make more than 2 picks per day if they want?
  2. Daily Scoring: If a user makes 3 picks on Sunday and 2 win + 1 loses, they get 0 points for Sunday? (Any loss = 0 points for that day?)
  3. Push Handling: Do pushes still not count toward the minimum 2 picks but don't cause failure? (e.g., 1 win + 1 push = 1 point for that day?)
  4. Pick Limits: Is there a maximum number of picks per day, or can users pick every game on Sunday if they want?

  Technical Architecture Questions:

  5. Season Detection: Should I add a season check everywhere to determine legacy vs modern behavior? Like if (season >= 2025) { /* modern */ } else { /* legacy */ }?
  6. Database Changes: Do we need to modify the Pick model to store a "day" field, or can we derive the day from the game date when needed?
  7. Template Strategy: Would you prefer:
    - Option A: Keep existing templates, add new 2025-specific templates
    - Option B: Refactor templates to be parameterized (pass in grouping strategy)
    - Option C: Create a template service that chooses the right template based on season
  8. Legacy Code Location: Where should legacy-specific code live?
    - Option A: legacy/ directory with 2023/2024 handlers
    - Option B: Same files but with clear // LEGACY sections
    - Option C: Separate service classes like LegacyPickService vs ModernPickService

  Display Questions:

  9. Day Grouping: For the picker modal, should days be grouped like "Thursday Games", "Friday Games", "Sunday Games", "Monday Games" based on actual game dates?
  10. Week Display: In the dashboard, should we still show "Week X" as the main container, then have day sections within each week?

  Let me know your preferences and I'll create a clean architecture that maximizes code reuse while keeping legacy code clearly separated!

> 1. 2 pick minimum is still enforced. if it's a 1-game-day, the user must make an ats and o/u pick on that single game for a valid parlay. Users can make more than 2 picks if there are multiple games in a day.\
  2. correct, if any single pick fails, 0 points are awarded for the day.\
  3. pushes are scratched for scoring purposes but do not invalidate parlays if it brings the player below 2 bets. If they make 2 picks and 1 wins and 1 pushes, they get 1 point. 5 picks and 2 of them push and the others win, they get 3 points, etc.\
  4. There is no limit to the number of picks a user can make\
  5. if its required, yes, you will need to have checks for legacy vs modern behavior. Pick modal probably doesnt need it since we cant make picks in the past anyways. The main parts I would expect different behavior are on the pick list display and the scoring logic.\
  6. I would expect it would be possible to derive the day from the game's date field and picks are associated with games. Make sure game days are based on their start time in pacific time since UTC might shift it to another day.\
  7. I'm not sure but option b sounds interesting\
  8. I've been using claude code for 99% of dev on this project, which option would make it clearer to you that this code should not be tampered with when working on new features.

● Based on your answers, I think Option B (parameterized templates) + Option B (clear // LEGACY sections) will work best for Claude Code development.

● Perfect! For Claude Code development, clear // LEGACY sections will make it obvious what code shouldn't be touched. Let me analyze the current codebase first:

● Search(pattern: "BonusThursday|BonusFriday|bonus", glob: "**/*.go")
  ⎿  Found 0 files 

● Search(pattern: "BonusThursday|BonusFriday", glob: "**/*.html")
  ⎿  Found 0 files 

● Search(pattern: "Bonus.*Pick|Thursday.*Pick|Friday.*Pick", glob: "**/*")
  ⎿  Found 0 files 

● Search(pattern: "BONUS.*THURSDAY|BONUS.*FRIDAY", glob: "templates/*.html", output_mode: "content")
  ⎿  Found 0 lines 

● Let me check the template for the current bonus day structure:

● Read(templates/dashboard.html)
  ⎿  Read 50 lines (ctrl+r to expand)

● Perfect! Now let me check the UserPicks model to understand the current structure:

● Search(pattern: "BonusThursdayPicks|BonusFridayPicks", path: "models", output_mode: "content")
  ⎿  Found 0 lines 

● Read(models/pick.go)
  ⎿  Read 40 lines (ctrl+r to expand)

● Let me find the UserPicks struct:

● Search(pattern: "type.*UserPicks.*struct", path: "models", output_mode: "content")
  ⎿  Found 0 lines 

● Bash(find /home/ryanp/nfl-app-go -name "*.go" -exec grep -l "UserPicks" {} \;)
  ⎿  Wi-Fi is currently blocked by rfkill.          
     Use raspi-config to set the country before use.
     … +10 lines (ctrl+r to expand)

● Search(pattern: "UserPicks.*struct", path: "models/pick.go", output_mode: "content")
  ⎿  Found 0 lines 

● Search(pattern: "type.*UserPicks", path: "models/pick.go", output_mode: "content")
  ⎿  Found 0 lines 

● Bash(grep -n "UserPicks" /home/ryanp/nfl-app-go/models/pick.go)
  ⎿  Wi-Fi is currently blocked by rfkill.          
     Use raspi-config to set the country before use.
     … +2 lines (ctrl+r to expand)

● Read(models/pick.go)
  ⎿  Read 25 lines (ctrl+r to expand)

● Excellent! Now let me check how the current scoring logic works:

● Search(pattern: "CalculateParlayPoints|ProcessWeekParlayScoring|ProcessParlayCategory", path: "services", output_mode: "content")
  ⎿  Found 0 lines 

● Bash(grep -n "ProcessWeekParlayScoring" /home/ryanp/nfl-app-go/services/pick_service.go)
  ⎿  Wi-Fi is currently blocked by rfkill.          
     Use raspi-config to set the country before use.
     … +2 lines (ctrl+r to expand)

● Read(services/pick_service.go)
  ⎿  Read 50 lines (ctrl+r to expand)

● Perfect! I've analyzed the codebase. Here's my architectural design:

  Architecture Plan: Legacy vs Modern Daily Scoring

  Current Legacy System (2023-2024):

  - UserPicks.BonusThursdayPicks[] and BonusFridayPicks[] for special days
  - UserPicks.Picks[] for regular week picks
  - Weekly parlay scoring with ProcessWeekParlayScoring()
  - Templates show "BONUS THURSDAY", "BONUS FRIDAY", "REMAINING PICKS"

  New Modern System (2025+):

  - Daily Parlay Scoring: Every day is independent
  - Pacific Time Grouping: Games grouped by PT date (not UTC)
  - No Special Bonus Logic: All days treated equally
  - Template Parameterization: Same template, different grouping data

  Architecture Strategy:

  1. Add Season Detection Helper:
  // In models/game.go or new file
  func IsModernSeason(season int) bool { return season >= 2025 }
  2. Create Day Grouping Logic:
  // New function to group games/picks by Pacific Time date
  func GroupGamesByDay(games []Game) map[string][]Game
  func GroupPicksByDay(picks []Pick, games []Game) map[string][]Pick
  3. Template Strategy:
    - Keep existing templates as // LEGACY: 2023-2024 bonus day logic
    - Add parameterized template data for modern daily grouping
    - Templates receive either legacy bonus structure OR modern daily structure
  4. Scoring Logic:
    - Keep existing ProcessWeekParlayScoring() as // LEGACY
    - Add new ProcessDailyParlayScoring() for modern seasons
  5. UserPicks Structure:
    - Keep BonusThursdayPicks, BonusFridayPicks for legacy
    - Add new DailyPickGroups map[string][]Pick for modern seasons

  Does this architecture sound good? Should I start implementing the day-based game grouping logic first?
