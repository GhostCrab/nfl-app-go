# NFL Parlay Club Scoring Rules

## Overview
The NFL Parlay Club is a season-long competition where players make weekly picks and earn points based on successful parlays.

## Basic Rules

### Weekly Pick Requirements
- Each player must make **at least 2 picks each week**
- Picks can be:
  - Team to cover the spread in a single game
  - Game total to go over/under the point total
- Players can make **more than 2 picks** if they choose

### Scoring System

#### All Picks Win
- If **all** of a player's picks win for a week, they are awarded points equal to the number of picks made
- 2 picks win = **+2 points**
- 3 picks win = **+3 points**  
- 4 picks win = **+4 points**
- etc.

#### Any Pick Loses
- If **any single pick loses**, **no points** are awarded to the player for that week

#### Push Handling
- If a pick **pushes** (ties), that pick is **not counted** towards the player's parlay for that week
- Examples:
  - 2 picks made, 1 push + 1 win = **+1 point** (only the winning pick counts)
  - 5 picks made, 2 push + 3 win = **+3 points** (only the 3 winning picks count)
  - 3 picks made, 1 push + 1 win + 1 loss = **0 points** (any loss = no points)

### Bonus Weeks

#### Separate Scoring
- **Bonus weeks are tallied separately** from the rest of the actual week
- A player can fail their bonus week parlay but still earn points from their regular weekend parlay
- Example: 
  - Thursday (bonus): 3 picks, all lose = **0 points**
  - Saturday/Sunday: 3 picks, all win = **+3 points**
  - Total for week = **+3 points**

#### Bonus Week Evolution by Season
- **2023**: Opening Thursday, Thanksgiving Thursday only
- **2024**: Added Thanksgiving Friday 
- **2025**: Added Opening Friday

### Season Scoring

#### Running Tally
- Points are **tallied each week** 
- The "Club Score" section displays the **running total** over the season
- **At the end of the season, the player with the most points wins**

#### Display Format
- Show total points accumulated up to the displayed week
- If a player won points in the currently displayed week, show **(+#)** next to their total
- Example: `John Smith: 24 (+3)` means 24 total points, earned 3 this week

## Implementation Notes

### Technical Considerations
- Handle pushes correctly by excluding them from parlay calculations
- Bonus weeks must be scored independently from regular weeks
- Club score display needs proper caching that updates when game results change
- Need to track weekly point awards for the (+#) display feature
- System must recalculate scores when game results are updated

### Edge Cases
- Minimum 2 picks required - what happens if player makes only 1 pick?
- Games postponed or cancelled - how do those picks get handled?
- Late score corrections by NFL - system should recalculate automatically