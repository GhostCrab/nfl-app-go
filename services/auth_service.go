package services

import (
	"errors"
	"nfl-app-go/models"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// AuthService handles authentication operations
type AuthService struct {
	userRepo   UserRepository
	jwtSecret  []byte
	tokenExpiry time.Duration
}

// UserRepository interface for user data operations
type UserRepository interface {
	GetUserByEmail(email string) (*models.User, error)
	GetUserByID(id int) (*models.User, error)
	GetUserByResetToken(token string) (*models.User, error)
	CreateUser(user *models.User) error
	UpdateUser(user *models.User) error
}

// JWTClaims represents the claims in our JWT token
type JWTClaims struct {
	UserID int    `json:"user_id"`
	Email  string `json:"email"`
	Name   string `json:"name"`
	jwt.RegisteredClaims
}

// NewAuthService creates a new authentication service
func NewAuthService(userRepo UserRepository, jwtSecret string) *AuthService {
	return &AuthService{
		userRepo:    userRepo,
		jwtSecret:   []byte(jwtSecret),
		tokenExpiry: 24 * 30 * 6 * time.Hour, // Token expires in 6 months
	}
}

// Login authenticates a user and returns a JWT token
func (a *AuthService) Login(email, password string) (*models.AuthResponse, error) {
	// Get user by email
	user, err := a.userRepo.GetUserByEmail(email)
	if err != nil {
		return nil, errors.New("invalid email or password")
	}

	// Check password
	if !user.CheckPassword(password) {
		return nil, errors.New("invalid email or password")
	}

	// Generate JWT token
	token, err := a.GenerateToken(user)
	if err != nil {
		return nil, errors.New("failed to generate token")
	}

	return &models.AuthResponse{
		User:  user.ToSafeUser(),
		Token: token,
	}, nil
}

// GenerateToken creates a new JWT token for the user
func (a *AuthService) GenerateToken(user *models.User) (string, error) {
	claims := JWTClaims{
		UserID: user.ID,
		Email:  user.Email,
		Name:   user.Name,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(a.tokenExpiry)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			NotBefore: jwt.NewNumericDate(time.Now()),
			Issuer:    "nfl-app-go",
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString(a.jwtSecret)
}

// ValidateToken validates a JWT token and returns the claims
func (a *AuthService) ValidateToken(tokenString string) (*JWTClaims, error) {
	token, err := jwt.ParseWithClaims(tokenString, &JWTClaims{}, func(token *jwt.Token) (interface{}, error) {
		// Verify the signing method
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, errors.New("invalid token signing method")
		}
		return a.jwtSecret, nil
	})

	if err != nil {
		return nil, err
	}

	if claims, ok := token.Claims.(*JWTClaims); ok && token.Valid {
		return claims, nil
	}

	return nil, errors.New("invalid token")
}

// GetUserFromToken validates token and returns the user
func (a *AuthService) GetUserFromToken(tokenString string) (*models.User, error) {
	claims, err := a.ValidateToken(tokenString)
	if err != nil {
		return nil, err
	}

	user, err := a.userRepo.GetUserByID(claims.UserID)
	if err != nil {
		return nil, errors.New("user not found")
	}

	return user, nil
}

// GetUserByEmail returns a user by email address
func (a *AuthService) GetUserByEmail(email string) (*models.User, error) {
	return a.userRepo.GetUserByEmail(email)
}

// RequestPasswordReset generates a password reset token for the user
func (a *AuthService) RequestPasswordReset(email string) (string, error) {
	user, err := a.userRepo.GetUserByEmail(email)
	if err != nil {
		// User not found - return empty token but no error (security)
		return "", nil
	}

	// Generate reset token
	if err := user.GenerateResetToken(); err != nil {
		return "", errors.New("failed to generate reset token")
	}

	// Save user with reset token
	if err := a.userRepo.UpdateUser(user); err != nil {
		return "", errors.New("failed to save reset token")
	}

	return user.ResetToken, nil
}

// ResetPassword resets the user's password using a valid reset token
func (a *AuthService) ResetPassword(token, newPassword string) error {
	if newPassword == "" {
		return errors.New("password cannot be empty")
	}

	if len(newPassword) < 6 {
		return errors.New("password must be at least 6 characters long")
	}

	// Find user by reset token
	user, err := a.userRepo.GetUserByResetToken(token)
	if err != nil {
		return errors.New("invalid or expired reset token")
	}

	// Validate token
	if !user.IsResetTokenValid(token) {
		return errors.New("invalid or expired reset token")
	}

	// Update password
	if err := user.HashPassword(newPassword); err != nil {
		return errors.New("failed to hash password")
	}

	// Clear reset token
	user.ClearResetToken()

	// Save user
	if err := a.userRepo.UpdateUser(user); err != nil {
		return errors.New("failed to update password")
	}

	return nil
}