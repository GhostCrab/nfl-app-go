package middleware

import (
	"context"
	"net/http"
	"nfl-app-go/models"
	"nfl-app-go/services"
	"strings"
)

// UserContextKey is the key used to store user in request context
type UserContextKey string

const UserKey UserContextKey = "user"

// AuthMiddleware handles JWT authentication
type AuthMiddleware struct {
	authService *services.AuthService
}

// NewAuthMiddleware creates a new authentication middleware
func NewAuthMiddleware(authService *services.AuthService) *AuthMiddleware {
	return &AuthMiddleware{
		authService: authService,
	}
}

// RequireAuth middleware that requires authentication
func (m *AuthMiddleware) RequireAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user, err := m.getUserFromRequest(r)
		if err != nil {
			// Redirect to login page for web requests
			if strings.Contains(r.Header.Get("Accept"), "text/html") {
				http.Redirect(w, r, "/login", http.StatusSeeOther)
				return
			}
			// Return 401 for API requests
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}

		// Add user to request context
		ctx := context.WithValue(r.Context(), UserKey, user)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// OptionalAuth middleware that optionally adds user to context if authenticated
func (m *AuthMiddleware) OptionalAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user, _ := m.getUserFromRequest(r)
		if user != nil {
			// Add user to request context
			ctx := context.WithValue(r.Context(), UserKey, user)
			r = r.WithContext(ctx)
		}
		next.ServeHTTP(w, r)
	})
}

// getUserFromRequest extracts and validates user from request
func (m *AuthMiddleware) getUserFromRequest(r *http.Request) (*models.User, error) {
	// Try to get token from Authorization header
	authHeader := r.Header.Get("Authorization")
	if authHeader != "" {
		// Expected format: "Bearer <token>"
		parts := strings.Split(authHeader, " ")
		if len(parts) == 2 && parts[0] == "Bearer" {
			return m.authService.GetUserFromToken(parts[1])
		}
	}

	// Try to get token from cookie
	cookie, err := r.Cookie("auth_token")
	if err == nil && cookie.Value != "" {
		return m.authService.GetUserFromToken(cookie.Value)
	}

	return nil, http.ErrNoCookie
}

// GetUserFromContext retrieves the authenticated user from request context
func GetUserFromContext(r *http.Request) *models.User {
	if user, ok := r.Context().Value(UserKey).(*models.User); ok {
		return user
	}
	return nil
}

// IsAuthenticated checks if the request has an authenticated user
func IsAuthenticated(r *http.Request) bool {
	return GetUserFromContext(r) != nil
}