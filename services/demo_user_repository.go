package services

import (
	"errors"
	"nfl-app-go/models"
	"time"
)

// DemoUserRepository implements UserRepository in memory for testing
type DemoUserRepository struct {
	users map[string]*models.User
	nextID int
}

// NewDemoUserRepository creates a new in-memory user repository
func NewDemoUserRepository() *DemoUserRepository {
	repo := &DemoUserRepository{
		users:  make(map[string]*models.User),
		nextID: 0,
	}
	
	// Seed with default users
	repo.seedUsers()
	return repo
}

// seedUsers creates the initial users
func (r *DemoUserRepository) seedUsers() {
	users := []struct {
		ID       int
		Name     string
		Email    string
		Password string
	}{
		{0, "ANDREW", "ackilpatrick@gmail.com", "password123"},
		{1, "BARDIA", "bbakhtari@gmail.com", "password123"},
		{2, "COOPER", "cooper.kocsis@mattel.com", "password123"},
		{3, "MICAH", "micahgoldman@gmail.com", "password123"},
		{4, "RYAN", "ryan.pielow@gmail.com", "password123"},
		{5, "TJ", "tyerke@yahoo.com", "password123"},
		{6, "BRAD", "bradvassar@gmail.com", "password123"},
	}

	for _, userData := range users {
		user := &models.User{
			ID:        userData.ID,
			Name:      userData.Name,
			Email:     userData.Email,
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		}

		// Hash the password
		user.HashPassword(userData.Password)
		
		r.users[userData.Email] = user
		if userData.ID >= r.nextID {
			r.nextID = userData.ID + 1
		}
	}
}

// GetUserByEmail retrieves a user by their email address
func (r *DemoUserRepository) GetUserByEmail(email string) (*models.User, error) {
	if user, exists := r.users[email]; exists {
		return user, nil
	}
	return nil, errors.New("user not found")
}

// GetUserByID retrieves a user by their ID
func (r *DemoUserRepository) GetUserByID(id int) (*models.User, error) {
	for _, user := range r.users {
		if user.ID == id {
			return user, nil
		}
	}
	return nil, errors.New("user not found")
}

// CreateUser creates a new user
func (r *DemoUserRepository) CreateUser(user *models.User) error {
	if _, exists := r.users[user.Email]; exists {
		return errors.New("user already exists")
	}
	
	user.ID = r.nextID
	r.nextID++
	user.CreatedAt = time.Now()
	user.UpdatedAt = time.Now()
	
	r.users[user.Email] = user
	return nil
}

// GetUserByResetToken retrieves a user by their password reset token
func (r *DemoUserRepository) GetUserByResetToken(token string) (*models.User, error) {
	for _, user := range r.users {
		if user.ResetToken == token {
			return user, nil
		}
	}
	return nil, errors.New("user not found")
}

// UpdateUser updates an existing user
func (r *DemoUserRepository) UpdateUser(user *models.User) error {
	if _, exists := r.users[user.Email]; !exists {
		return errors.New("user not found")
	}
	
	user.UpdatedAt = time.Now()
	r.users[user.Email] = user
	return nil
}