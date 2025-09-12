package handlers

import (
	"encoding/json"
	"html/template"
	"log"
	"net/http"
	"nfl-app-go/middleware"
	"nfl-app-go/models"
	"nfl-app-go/services"
	"time"
)

// DashboardHandler handles dashboard and user statistics functionality
// This handler provides comprehensive user performance data and analytics
type DashboardHandler struct {
	templates         *template.Template
	gameService       services.GameService
	pickService       *services.PickService
	authService       *services.AuthService
	visibilityService *services.PickVisibilityService
	analyticsService  *services.AnalyticsService
	dataLoader        *services.DataLoader
}

// NewDashboardHandler creates a new dashboard handler
func NewDashboardHandler(
	templates *template.Template,
	gameService services.GameService,
	pickService *services.PickService,
	authService *services.AuthService,
	visibilityService *services.PickVisibilityService,
	analyticsService *services.AnalyticsService,
	dataLoader *services.DataLoader,
) *DashboardHandler {
	return &DashboardHandler{
		templates:         templates,
		gameService:       gameService,
		pickService:       pickService,
		authService:       authService,
		visibilityService: visibilityService,
		analyticsService:  analyticsService,
		dataLoader:        dataLoader,
	}
}

// GetDashboardDataAPI provides basic dashboard data via JSON API
// This is a simplified version for the integration phase
func (h *DashboardHandler) GetDashboardDataAPI(w http.ResponseWriter, r *http.Request) {
	log.Println("GetDashboardDataAPI called - using simplified version")
	
	// Set JSON content type
	w.Header().Set("Content-Type", "application/json")
	
	// Extract user from context (optional for dashboard data)
	var viewingUser *models.User
	if user, ok := r.Context().Value(middleware.UserKey).(*models.User); ok {
		viewingUser = user
	}
	
	// Create basic dashboard response
	dashboardData := struct {
		Message   string       `json:"message"`
		User      *models.User `json:"user,omitempty"`
		Timestamp string       `json:"timestamp"`
	}{
		Message:   "Dashboard API - Integration Phase (Simplified)",
		User:      viewingUser,
		Timestamp: time.Now().Format(time.RFC3339),
	}
	
	// Encode and send JSON response
	if err := json.NewEncoder(w).Encode(dashboardData); err != nil {
		log.Printf("Error encoding dashboard API response: %v", err)
		http.Error(w, `{"error":"Error encoding response"}`, http.StatusInternalServerError)
	}
}