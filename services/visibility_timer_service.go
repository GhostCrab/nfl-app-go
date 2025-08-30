package services

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"
)

// VisibilityTimerService manages automatic pick visibility updates
type VisibilityTimerService struct {
	visibilityService *PickVisibilityService
	broadcastHandler  func(eventType, data string) // Handler for SSE broadcasts
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

// Start begins monitoring for pick visibility changes
func (s *VisibilityTimerService) Start() {
	s.mu.Lock()
	defer s.mu.Unlock()
	
	if s.ticker != nil {
		log.Printf("VisibilityTimerService: Already running")
		return
	}
	
	// Check every minute for visibility changes
	s.ticker = time.NewTicker(1 * time.Minute)
	s.wg.Add(1)
	
	go s.run()
	log.Printf("VisibilityTimerService: Started monitoring pick visibility changes")
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
	log.Printf("VisibilityTimerService: Stopped monitoring pick visibility changes")
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
			log.Printf("VisibilityTimerService: Error checking visibility for week %d: %v", week, err)
			continue
		}
		
		if shouldTrigger {
			log.Printf("VisibilityTimerService: Visibility changed for week %d, triggering update", week)
			s.triggerVisibilityUpdate(week)
		}
	}
	
	// Update last check time
	s.mu.Lock()
	s.lastCheck = currentTime
	s.mu.Unlock()
}

// triggerVisibilityUpdate sends an SSE event to update pick visibility
func (s *VisibilityTimerService) triggerVisibilityUpdate(week int) {
	if s.broadcastHandler == nil {
		log.Printf("VisibilityTimerService: No broadcast handler configured")
		return
	}
	
	// Create visibility update event JSON
	eventJSON := fmt.Sprintf(`{"type":"visibility-change","season":%d,"week":%d,"message":"Pick visibility has changed, refreshing picks","timestamp":%d}`,
		s.currentSeason, week, time.Now().UnixMilli())
	
	// Broadcast the visibility change event
	s.broadcastHandler("visibility-change", eventJSON)
	
	log.Printf("VisibilityTimerService: Sent visibility change event for week %d", week)
}

// GetNextScheduledUpdate returns when the next visibility change will occur
func (s *VisibilityTimerService) GetNextScheduledUpdate(ctx context.Context) (*time.Time, error) {
	// Check all weeks for the next visibility change
	var nextUpdate *time.Time
	
	for week := 1; week <= 18; week++ {
		weekNext, err := s.visibilityService.GetNextVisibilityChange(ctx, s.currentSeason, week)
		if err != nil {
			log.Printf("VisibilityTimerService: Error getting next visibility change for week %d: %v", week, err)
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
		log.Printf("VisibilityTimerService: Error getting upcoming changes: %v", err)
		return
	}
	
	if nextUpdate == nil {
		log.Printf("VisibilityTimerService: No upcoming visibility changes scheduled")
		return
	}
	
	currentTime := s.visibilityService.GetCurrentTime()
	timeUntil := nextUpdate.Sub(currentTime)
	
	log.Printf("VisibilityTimerService: Next visibility change at %s (in %s)",
		nextUpdate.Format("2006-01-02 15:04:05 MST"), 
		timeUntil.Round(time.Minute))
}