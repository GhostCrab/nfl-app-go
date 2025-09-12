package interfaces

import (
	"context"
	"nfl-app-go/models"
)

// GameServiceInterface defines the core game data operations used by handlers
// This replaces the concrete services.GameService dependency
type GameServiceInterface interface {
	GetGames() ([]models.Game, error)
	GetGamesBySeason(season int) ([]models.Game, error) 
	GetGameByID(gameID int) (*models.Game, error)
	HealthCheck() bool
}

// PickServiceInterface defines the core pick operations used by handlers  
// This replaces the concrete *services.PickService dependency
type PickServiceInterface interface {
	GetUserPicksForWeek(ctx context.Context, userID, season, week int) (*models.UserPicks, error)
	GetAllUserPicksForWeek(ctx context.Context, season, week int) ([]*models.UserPicks, error)
	ReplaceUserPicksForWeek(ctx context.Context, userID, season, week int, picks []*models.Pick) error
	GetPicksForAnalytics(ctx context.Context, season int, week *int, allSeasons bool) ([]models.Pick, error)
}

// AuthServiceInterface defines authentication operations used by handlers
// This replaces the concrete *services.AuthService dependency
type AuthServiceInterface interface {
	ValidateToken(tokenString string) (*models.User, error)
	GetCurrentUser(ctx context.Context) (*models.User, error)
}

// UserServiceInterface defines user operations used by handlers
// This abstracts user data access
type UserServiceInterface interface {
	GetAllUsers() ([]models.User, error)
	GetUserByID(userID int) (*models.User, error)
}

// VisibilityServiceInterface defines pick visibility operations
// This replaces the concrete *services.PickVisibilityService dependency
type VisibilityServiceInterface interface {
	FilterVisibleUserPicks(ctx context.Context, userPicks []*models.UserPicks, viewingUserID int, season, week int) ([]*models.UserPicks, error)
	SetDebugDateTime(debugTime interface{}) // Using interface{} to avoid time import issues
}