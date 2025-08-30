# Pick Visibility System - Testing Guide

This document explains how to test the new pick visibility system that controls when users can see each other's picks.

## Overview

The pick visibility system implements these rules:
- **Own picks**: Always visible to the user
- **Thursday games**: Visible to all at 5:00 PM PT (10:00 AM PT on Thanksgiving)
- **Friday/Saturday/Sunday/Monday games**: Visible to all at 10:00 AM PT on Saturday
- **In-progress/Completed games**: Always visible to everyone
- **Hidden picks**: Show day-grouped counts without revealing actual picks

## Setup for Testing

### 1. Populate Test Data

First, populate the 2025 season with test picks for all users:

```bash
go run cmd/populate_test_picks.go
```

This creates 4-6 realistic picks per user per week for all 7 users.

### 2. Start the Server

```bash
go run main.go
```

The server will start the visibility timer service and log upcoming visibility changes.

## Testing Scenarios

### 1. Basic Visibility Testing

**Test different times using the debug datetime parameter:**

```bash
# Test Thursday at 4:00 PM PT (before 5:00 PM threshold)
http://localhost:8080/?datetime=2025-09-11T16:00

# Test Thursday at 6:00 PM PT (after 5:00 PM threshold)
http://localhost:8080/?datetime=2025-09-11T18:00

# Test Saturday at 9:00 AM PT (before 10:00 AM threshold)
http://localhost:8080/?datetime=2025-09-13T09:00

# Test Saturday at 11:00 AM PT (after 10:00 AM threshold)
http://localhost:8080/?datetime=2025-09-13T11:00
```

### 2. Thanksgiving Special Rules

```bash
# Thanksgiving Thursday at 9:00 AM PT (before 10:00 AM threshold)
http://localhost:8080/?datetime=2025-11-27T09:00

# Thanksgiving Thursday at 11:00 AM PT (after 10:00 AM threshold)  
http://localhost:8080/?datetime=2025-11-27T11:00
```

### 3. In-Progress Game Testing

```bash
# Set time to within 60 minutes of a game start time to trigger demo "in-progress" state
# Example: If game starts at 1:00 PM, test at 1:30 PM
http://localhost:8080/?datetime=2025-09-14T13:30
```

### 4. Real-Time Updates

The system automatically checks for visibility changes every minute. When picks become visible:
- SSE event `visibility-change` is sent to all connected clients
- Clients automatically refresh their pick displays
- No manual refresh needed

## What to Look For

### 1. Pick Display Changes

**When picks are hidden:**
- Other users' picks sections show "HIDDEN PICKS" instead of actual picks
- Day-grouped counts display: "Thursday: 2 picks", "Sunday/Monday: 3 picks"
- Note explains "Picks will be revealed based on game schedule"

**When picks become visible:**
- Hidden picks are replaced with actual pick details
- Page updates automatically via SSE when visibility changes
- Real-time updates work without page refresh

### 2. Debug Information

Check server logs for:
```
DEBUG: Set debug datetime to 2025-09-11 16:00:00 PDT Pacific
VisibilityTimerService: Next visibility change at 2025-09-11 17:00:00 PDT (in 1h0m)
DEBUG: Demo time analysis - 3 games would be in-progress, 5 would be completed
```

### 3. User Experience

**As authenticated user:**
- Always see your own picks regardless of time
- See other users' picks only when appropriate
- Hidden pick counts give preview of what's coming
- No disruption when visibility changes (smooth SSE updates)

**As unauthenticated user:**
- Prompted to log in
- Can see games but no pick information

## Expected Behavior Examples

### Scenario: Thursday 4:00 PM PT

- **Your picks**: Fully visible with all details
- **Others' Thursday picks**: Hidden (show "Thursday: 2 picks") 
- **Others' weekend picks**: Hidden (show "Sunday/Monday: 3 picks")
- **Completed game picks**: Visible regardless of day

### Scenario: Thursday 6:00 PM PT

- **Your picks**: Fully visible
- **Others' Thursday picks**: Now visible with full details
- **Others' weekend picks**: Still hidden until Saturday 10 AM
- **Completed game picks**: Visible

### Scenario: Saturday 11:00 AM PT  

- **All picks**: Visible for all users (weekend threshold passed)
- **Hidden sections**: Gone (replaced with actual picks)
- **Real-time**: Any new picks immediately visible to all users

## Advanced Testing

### 1. Multiple Users

Log in as different users to verify:
- Each user always sees their own picks
- Visibility rules apply consistently across users
- Hidden counts are accurate per user

### 2. SSE Real-Time Updates

1. Open dashboard in multiple browser tabs
2. Set debug time just before a visibility threshold
3. Wait for the threshold to pass (or advance debug time)
4. Verify all tabs update simultaneously via SSE

### 3. Edge Cases

Test boundary conditions:
- Exactly at visibility threshold times
- Games that start before normal visibility rules
- Thanksgiving vs regular Thursday behavior
- Mixed week scenarios (some picks visible, some hidden)

## Troubleshooting

### No Picks Showing
- Run the populate script: `go run cmd/populate_test_picks.go`
- Check you're logged in as a valid user
- Verify database connection

### Debug Time Not Working
- Use format: `YYYY-MM-DDTHH:MM` (24-hour time)
- Time is interpreted as Pacific timezone
- Check server logs for debug confirmation

### SSE Updates Not Working
- Check browser dev tools network tab for `/events` connection
- Verify HTMX extensions are loaded
- Check server logs for SSE client connections

### Visibility Rules Not Applied
- Check server logs for visibility service initialization
- Verify game data has proper dates and states
- Check visibility calculations in debug logs

## Production Considerations

When deploying to production:
- Remove or restrict debug datetime parameter access
- Consider rate limiting for visibility change broadcasts
- Monitor performance impact of minute-by-minute visibility checks
- Ensure proper timezone handling for users in different regions