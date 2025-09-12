package interfaces

import (
	"nfl-app-go/services"
)

// Interface compliance checks - these will fail to compile if services don't implement interfaces
var (
	// Verify GameService implementations
	_ GameService = (*services.DemoGameService)(nil)
	_ GameService = (*services.DatabaseGameService)(nil)
	
	// Verify PickService implementation  
	_ PickService = (*services.PickService)(nil)
	
	// Verify specialized service implementations
	_ ParlayService = (*services.ParlayService)(nil)
	_ ResultCalculationService = (*services.ResultCalculationService)(nil)
	_ AnalyticsService = (*services.AnalyticsService)(nil)
	
	// Verify other service implementations
	_ AuthService = (*services.AuthService)(nil)
	_ EmailService = (*services.EmailService)(nil)
	_ PickVisibilityService = (*services.PickVisibilityService)(nil)
	_ UserService = (*services.DatabaseUserService)(nil)
	
	// Verify handlers that implement SSEBroadcaster
	// Note: This will be updated when we refactor handlers
)