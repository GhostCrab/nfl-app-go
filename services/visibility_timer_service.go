package services

import (
	"context"
	"nfl-app-go/logging"
	"sync"
	"time"
)

// VisibilityTimerService manages automatic pick visibility updates
type VisibilityTimerService struct {
	visibilityService *PickVisibilityService
	broadcastHandler  func(eventType, data string) // Handler for SSE broadcasts
	pickRefreshHandler func(season, week int) // Handler for generating full pick container updates
	currentSeason     int
	ticker            *time.Ticker
	stopChan          chan struct{}
	wg                sync.WaitGroup
	mu                sync.RWMutex
	lastCheck         time.Time
}

// NewVisibilityTimerService creates a new visibility timer service
func NewVisibilityTimerService(visibilityService *PickVisibilityService, broadcastHandler func(string, string), season int) *VisibilityTimerService {
	return &VisibilityTimerService{
		visibilityService: visibilityService,
		broadcastHandler:  broadcastHandler,
		currentSeason:     season,
		stopChan:          make(chan struct{}),
		lastCheck:         time.Now(),
	}
}

// SetPickRefreshHandler sets the handler for generating full pick container updates
func (s *VisibilityTimerService) SetPickRefreshHandler(handler func(season, week int)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.pickRefreshHandler = handler
}

// Start begins monitoring for pick visibility changes
func (s *VisibilityTimerService) Start() {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.ticker != nil {
		logging.Warn("VisibilityTimerService: Already running")
		return
	}

	// Check every minute for visibility changes
	s.ticker = time.NewTicker(1 * time.Minute)
	s.wg.Add(1)

	go s.run()
	logging.Info("VisibilityTimerService: Started monitoring pick visibility changes")
}

// Stop stops the visibility monitoring
func (s *VisibilityTimerService) Stop() {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.ticker == nil {
		return
	}

	s.ticker.Stop()
	close(s.stopChan)
	s.ticker = nil

	// Wait for goroutine to finish
	s.wg.Wait()
	logging.Info("VisibilityTimerService: Stopped monitoring pick visibility changes")
}

// run is the main monitoring loop
func (s *VisibilityTimerService) run() {
	defer s.wg.Done()

	ctx := context.Background()

	for {
		select {
		case <-s.ticker.C:
			s.checkForVisibilityChanges(ctx)
		case <-s.stopChan:
			return
		}
	}
}

// checkForVisibilityChanges checks if any picks have become visible since last check
func (s *VisibilityTimerService) checkForVisibilityChanges(ctx context.Context) {
	s.mu.RLock()
	lastCheck := s.lastCheck
	s.mu.RUnlock()

	currentTime := s.visibilityService.GetCurrentTime()

	// Check each week of the current season for visibility changes
	for week := 1; week <= 18; week++ {
		shouldTrigger, err := s.visibilityService.ShouldTriggerVisibilityUpdate(ctx, s.currentSeason, week, lastCheck)
		if err != nil {
			logging.Errorf("VisibilityTimerService: Error checking visibility for week %d: %v", week, err)
			continue
		}

		if shouldTrigger {
			logging.Infof("VisibilityTimerService: Visibility changed for week %d, triggering update", week)
			s.triggerVisibilityUpdate(week)
		}
	}

	// Update last check time
	s.mu.Lock()
	s.lastCheck = currentTime
	s.mu.Unlock()
}

// triggerVisibilityUpdate sends an SSE event to update pick visibility with full pick container refresh
func (s *VisibilityTimerService) triggerVisibilityUpdate(week int) {
	if s.broadcastHandler == nil {
		logging.Error("VisibilityTimerService: No broadcast handler configured")
		return
	}

	// Use pick refresh handler if available for heavy-handed full container update
	s.mu.RLock()
	pickHandler := s.pickRefreshHandler
	s.mu.RUnlock()

	if pickHandler != nil {
		logging.Infof("VisibilityTimerService: Triggering full pick container refresh for week %d", week)
		// This will generate and broadcast complete pick-container OOB updates
		pickHandler(s.currentSeason, week)
	} else {
		// Fallback to simple visibility change event (will trigger hx-get="/" reload)
		logging.Infof("VisibilityTimerService: Triggering dashboard reload for week %d visibility change", week)

		// Send empty data - the SSE listener will trigger hx-get="/" to reload dashboard
		s.broadcastHandler("visibility-change", "")
	}

	logging.Infof("VisibilityTimerService: Sent visibility change update for week %d", week)
}

// GetNextScheduledUpdate returns when the next visibility change will occur
func (s *VisibilityTimerService) GetNextScheduledUpdate(ctx context.Context) (*time.Time, error) {
	// Check all weeks for the next visibility change
	var nextUpdate *time.Time

	for week := 1; week <= 18; week++ {
		weekNext, err := s.visibilityService.GetNextVisibilityChange(ctx, s.currentSeason, week)
		if err != nil {
			logging.Errorf("VisibilityTimerService: Error getting next visibility change for week %d: %v", week, err)
			continue
		}

		if weekNext != nil {
			if nextUpdate == nil || weekNext.Before(*nextUpdate) {
				nextUpdate = weekNext
			}
		}
	}

	return nextUpdate, nil
}

// LogUpcomingChanges logs the next few visibility changes for debugging
func (s *VisibilityTimerService) LogUpcomingChanges(ctx context.Context) {
	nextUpdate, err := s.GetNextScheduledUpdate(ctx)
	if err != nil {
		logging.Errorf("VisibilityTimerService: Error getting upcoming changes: %v", err)
		return
	}

	if nextUpdate == nil {
		logging.Warn("VisibilityTimerService: No upcoming visibility changes scheduled")
		return
	}

	currentTime := s.visibilityService.GetCurrentTime()
	timeUntil := nextUpdate.Sub(currentTime)

	logging.Infof("VisibilityTimerService: Next visibility change at %s (in %s)",
		nextUpdate.Format("2006-01-02 15:04:05 MST"),
		timeUntil.Round(time.Minute))
}
