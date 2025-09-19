package interfaces

import (
	"context"
	"nfl-app-go/models"
	"time"
)

// GameService defines methods for game data operations
type GameService interface {
	GetGames() ([]models.Game, error)
	GetGamesBySeason(season int) ([]models.Game, error)
	GetGameByID(gameID int) (*models.Game, error)
	HealthCheck() bool
}

// PickService defines methods for pick management operations
type PickService interface {
	// Core pick operations
	CreatePick(ctx context.Context, userID, gameID, teamID, season, week int) (*models.Pick, error)
	GetUserPicksForWeek(ctx context.Context, userID, season, week int) (*models.UserPicks, error)
	GetAllUserPicksForWeek(ctx context.Context, season, week int) ([]*models.UserPicks, error)
	ReplaceUserPicksForWeek(ctx context.Context, userID, season, week int, picks []*models.Pick) error

	// Analytics and reporting
	GetPicksForAnalytics(ctx context.Context, season int, week *int, allSeasons bool) ([]models.Pick, error)
	GetPickStats(ctx context.Context) (map[string]interface{}, error)

	// Pick data enrichment
	EnrichPickWithGameData(pick *models.Pick) error

	// Delegation methods (calls specialized services)
	CalculateUserParlayScore(ctx context.Context, userID, season, week int) (map[models.ParlayCategory]int, error)
	ProcessWeekParlayScoring(ctx context.Context, season, week int) error
	ProcessParlayCategory(ctx context.Context, season, week int, category models.ParlayCategory) error
	ProcessDailyParlayScoring(ctx context.Context, season, week int) error
}

// ParlayService defines methods for parlay scoring operations
type ParlayService interface {
	CalculateUserParlayScore(ctx context.Context, userID, season, week int) (map[models.ParlayCategory]int, error)
	ProcessWeekParlayScoring(ctx context.Context, season, week int) error
	ProcessParlayCategory(ctx context.Context, season, week int, category models.ParlayCategory) error
	ProcessDailyParlayScoring(ctx context.Context, season, week int) error
	UpdateUserParlayRecord(ctx context.Context, userID, season, week int, weeklyScores map[models.ParlayCategory]int) error
	CheckWeekHasParlayScores(ctx context.Context, season, week int) (bool, error)
}

// ResultCalculationService defines methods for pick result calculations
type ResultCalculationService interface {
	ProcessGameCompletion(ctx context.Context, game *models.Game) error
	CalculatePickResult(pick *models.Pick, game *models.Game) models.PickResult
	ValidatePickAgainstGame(pick *models.Pick, game *models.Game) error
	ProcessAllCompletedGames(ctx context.Context, season int) error
}

// AnalyticsService defines methods for analytics and statistics
type AnalyticsService interface {
	// User performance analysis
	GetUserPerformanceStats(ctx context.Context, userID, season int) (*UserPerformanceStats, error)
	GetLeaderboard(ctx context.Context, season int, week *int) ([]LeaderboardEntry, error)

	// System analytics
	GetSystemStats(ctx context.Context) (map[string]interface{}, error)
	GetPickTypeDistribution(ctx context.Context, season int) (map[models.PickType]int, error)
}

// AuthService defines methods for authentication and authorization
type AuthService interface {
	// Authentication
	Login(username, password string) (*models.User, string, error)
	ValidateToken(tokenString string) (*models.User, error)
	GenerateToken(user *models.User) (string, error)

	// Password management
	ChangePassword(userID int, oldPassword, newPassword string) error
	RequestPasswordReset(email string) error
	ResetPassword(token, newPassword string) error

	// User management
	GetCurrentUser(ctx context.Context) (*models.User, error)
}

// EmailService defines methods for email operations
type EmailService interface {
	SendPasswordResetEmail(email, resetLink string) error
	SendWelcomeEmail(email, username string) error
	IsConfigured() bool
	TestConnection() error
}

// PickVisibilityService defines methods for managing pick visibility rules
type PickVisibilityService interface {
	FilterVisibleUserPicks(userPicks []*models.UserPicks, viewingUserID int, season, week int) ([]*models.UserPicks, error)
	SetDebugDateTime(debugTime *time.Time)
}

// UserService defines methods for user operations
type UserService interface {
	GetUserByID(userID int) (*models.User, error)
	GetAllUsers() ([]*models.User, error)
	CreateUser(user *models.User) error
	UpdateUser(user *models.User) error
	DeleteUser(userID int) error
}

// SSEBroadcaster defines methods for Server-Sent Events broadcasting
type SSEBroadcaster interface {
	BroadcastToAllClients(eventType, data string)
	AddClient(userID int) chan string
	RemoveClient(userID int, clientChan chan string)
	HandleDatabaseChange(changeEvent interface{})
}

// DataLoader defines methods for loading external data
type DataLoader interface {
	LoadGameData(season int) error
	RefreshCurrentWeek() error
}

// ===============================================
// Supporting types for analytics interfaces
// ===============================================

// UserPerformanceStats represents user performance analytics
type UserPerformanceStats struct {
	UserID        int                     `json:"user_id"`
	UserName      string                  `json:"user_name"`
	Season        int                     `json:"season"`
	TotalPicks    int                     `json:"total_picks"`
	WinRate       float64                 `json:"win_rate"`
	PushRate      float64                 `json:"push_rate"`
	TypeBreakdown map[models.PickType]int `json:"type_breakdown"`
	WeeklyStats   []WeeklyPerformance     `json:"weekly_stats"`
}

// WeeklyPerformance represents performance for a specific week
type WeeklyPerformance struct {
	Week         int     `json:"week"`
	Wins         int     `json:"wins"`
	Losses       int     `json:"losses"`
	Pushes       int     `json:"pushes"`
	WinRate      float64 `json:"win_rate"`
	ParlayPoints int     `json:"parlay_points"`
}

// LeaderboardEntry represents a user's position in rankings
type LeaderboardEntry struct {
	Rank         int     `json:"rank"`
	UserID       int     `json:"user_id"`
	UserName     string  `json:"user_name"`
	TotalPicks   int     `json:"total_picks"`
	Wins         int     `json:"wins"`
	Losses       int     `json:"losses"`
	Pushes       int     `json:"pushes"`
	WinRate      float64 `json:"win_rate"`
	ParlayPoints int     `json:"parlay_points"`
}
