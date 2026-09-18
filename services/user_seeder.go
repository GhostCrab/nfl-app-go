package services

import (
	"nfl-app-go/logging"
	"nfl-app-go/models"
	"time"
)

// UserSeeder handles seeding the database with initial users
type UserSeeder struct {
	userRepo UserRepository
}

// NewUserSeeder creates a new user seeder
func NewUserSeeder(userRepo UserRepository) *UserSeeder {
	return &UserSeeder{
		userRepo: userRepo,
	}
}

// SeedUsers creates the initial users in the database
func (s *UserSeeder) SeedUsers() error {
	// Initial users from the legacy application
	users := []struct {
		ID       int
		Name     string
		Email    string
		Password string // Default password for all users
	}{
		{0, "ANDREW", "ackilpatrick@gmail.com", "password123"},
		{1, "BARDIA", "bbakhtari@gmail.com", "password123"},
		{2, "COOPER", "cooper.kocsis@mattel.com", "password123"},
		{3, "MICAH", "micahgoldman@gmail.com", "password123"},
		{4, "RYAN", "ryan.pielow@gmail.com", "password123"},
		{5, "TJ", "tyerke@yahoo.com", "password123"},
		{6, "BRAD", "bradvassar@gmail.com", "password123"},
	}

	var existingCount, createdCount int

	for _, userData := range users {
		// Check if user already exists
		existingUser, err := s.userRepo.GetUserByEmail(userData.Email)
		if err == nil && existingUser != nil {
			existingCount++
			continue
		}

		// Create new user
		user := &models.User{
			ID:        userData.ID,
			Name:      userData.Name,
			Email:     userData.Email,
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		}

		// Hash the password
		if err := user.HashPassword(userData.Password); err != nil {
			logging.Errorf("Failed to hash password for %s: %v", userData.Email, err)
			continue
		}

		// Create user in database
		if err := s.userRepo.CreateUser(user); err != nil {
			logging.Errorf("Failed to create user %s: %v", userData.Email, err)
			continue
		}

		logging.Infof("Created user %s (%s) with ID %d", userData.Name, userData.Email, userData.ID)
		createdCount++
	}

	if existingCount > 0 || createdCount > 0 {
		logging.Infof("Completed Seeding Users - %d existing, %d created", existingCount, createdCount)
	}
	return nil
}

