package services

import (
	"context"
	"nfl-app-go/database"
	"nfl-app-go/models"
)

// UserService defines interface for user-related operations
type UserService interface {
	GetAllUsers() ([]models.User, error)
	GetUserByID(ctx context.Context, userID int) (*models.User, error)
}

// DatabaseUserService implements UserService using MongoDB
type DatabaseUserService struct {
	userRepo *database.MongoUserRepository
}

// NewDatabaseUserService creates a new database user service
func NewDatabaseUserService(userRepo *database.MongoUserRepository) *DatabaseUserService {
	return &DatabaseUserService{
		userRepo: userRepo,
	}
}

// GetAllUsers returns all users
func (s *DatabaseUserService) GetAllUsers() ([]models.User, error) {
	return s.userRepo.GetAllUsers()
}

// GetUserByID returns a user by ID
func (s *DatabaseUserService) GetUserByID(ctx context.Context, userID int) (*models.User, error) {
	return s.userRepo.GetUserByID(userID)
}