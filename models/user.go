package models

import (
	"crypto/rand"
	"encoding/hex"
	"time"
	"golang.org/x/crypto/bcrypt"
)

// User represents a user in the system
type User struct {
	ID                int       `json:"id" bson:"_id"`
	Name              string    `json:"name" bson:"name"`
	Email             string    `json:"email" bson:"email"`
	Password          string    `json:"-" bson:"password"` // Never serialize password in JSON
	ResetToken        string    `json:"-" bson:"resetToken,omitempty"` // Password reset token
	ResetTokenExpiry  *time.Time `json:"-" bson:"resetTokenExpiry,omitempty"` // Token expiry
	CreatedAt         time.Time `json:"createdAt" bson:"createdAt"`
	UpdatedAt         time.Time `json:"updatedAt" bson:"updatedAt"`
}

// LoginRequest represents login form data
type LoginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

// AuthResponse represents the response after successful authentication
type AuthResponse struct {
	User  User   `json:"user"`
	Token string `json:"token"`
}

// PasswordResetRequest represents a password reset request
type PasswordResetRequest struct {
	Email string `json:"email"`
}

// PasswordResetForm represents the password reset form data
type PasswordResetForm struct {
	Token           string `json:"token"`
	NewPassword     string `json:"newPassword"`
	ConfirmPassword string `json:"confirmPassword"`
}

// HashPassword hashes the user's password using bcrypt
func (u *User) HashPassword(password string) error {
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	u.Password = string(hashedPassword)
	return nil
}

// CheckPassword verifies the provided password against the stored hash
func (u *User) CheckPassword(password string) bool {
	err := bcrypt.CompareHashAndPassword([]byte(u.Password), []byte(password))
	return err == nil
}

// ToSafeUser returns a copy of the user without sensitive fields
func (u *User) ToSafeUser() User {
	return User{
		ID:        u.ID,
		Name:      u.Name,
		Email:     u.Email,
		CreatedAt: u.CreatedAt,
		UpdatedAt: u.UpdatedAt,
		// Password, ResetToken, ResetTokenExpiry intentionally omitted
	}
}

// GenerateResetToken generates a new password reset token
func (u *User) GenerateResetToken() error {
	// Generate 32 random bytes
	bytes := make([]byte, 32)
	if _, err := rand.Read(bytes); err != nil {
		return err
	}
	
	u.ResetToken = hex.EncodeToString(bytes)
	expiry := time.Now().Add(24 * time.Hour) // Token expires in 24 hours
	u.ResetTokenExpiry = &expiry
	u.UpdatedAt = time.Now()
	
	return nil
}

// IsResetTokenValid checks if the reset token is valid and not expired
func (u *User) IsResetTokenValid(token string) bool {
	if u.ResetToken == "" || u.ResetTokenExpiry == nil {
		return false
	}
	
	if u.ResetToken != token {
		return false
	}
	
	return time.Now().Before(*u.ResetTokenExpiry)
}

// ClearResetToken clears the password reset token
func (u *User) ClearResetToken() {
	u.ResetToken = ""
	u.ResetTokenExpiry = nil
	u.UpdatedAt = time.Now()
}