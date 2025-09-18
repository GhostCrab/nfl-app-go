package handlers

import (
	"encoding/json"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"nfl-app-go/middleware"
	"nfl-app-go/models"
	"nfl-app-go/services"
	"os"
	"time"
)

// AuthHandler handles authentication-related HTTP requests
type AuthHandler struct {
	templates    *template.Template
	authService  *services.AuthService
	emailService *services.EmailService
}

// NewAuthHandler creates a new authentication handler
func NewAuthHandler(templates *template.Template, authService *services.AuthService, emailService *services.EmailService) *AuthHandler {
	return &AuthHandler{
		templates:    templates,
		authService:  authService,
		emailService: emailService,
	}
}

// LoginPage displays the login form
func (h *AuthHandler) LoginPage(w http.ResponseWriter, r *http.Request) {
	// If already authenticated, redirect to home
	if middleware.IsAuthenticated(r) {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}

	data := struct {
		Title string
		Error string
		Success string
	}{
		Title: "Login - NFL Games",
		Error: r.URL.Query().Get("error"),
		Success: r.URL.Query().Get("success"),
	}

	err := h.templates.ExecuteTemplate(w, "login.html", data)
	if err != nil {
		log.Printf("AuthHandler: Template error: %v", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
}

// Login handles the login form submission
func (h *AuthHandler) Login(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Parse form data
	email := r.FormValue("email")
	password := r.FormValue("password")

	if email == "" || password == "" {
		http.Redirect(w, r, "/login?error=Please provide both email and password", http.StatusSeeOther)
		return
	}

	// Authenticate user
	authResponse, err := h.authService.Login(email, password)
	if err != nil {
		log.Printf("AuthHandler: Login failed for %s: %v", email, err)
		http.Redirect(w, r, "/login?error=Invalid email or password", http.StatusSeeOther)
		return
	}

	// Set auth cookie
	h.setAuthCookie(w, authResponse.Token)

	log.Printf("AuthHandler: User %s (%s) logged in successfully", authResponse.User.Name, authResponse.User.Email)
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

// LoginAPI handles JSON login requests
func (h *AuthHandler) LoginAPI(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var loginReq models.LoginRequest
	if err := json.NewDecoder(r.Body).Decode(&loginReq); err != nil {
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}

	if loginReq.Email == "" || loginReq.Password == "" {
		http.Error(w, "Email and password are required", http.StatusBadRequest)
		return
	}

	// Authenticate user
	authResponse, err := h.authService.Login(loginReq.Email, loginReq.Password)
	if err != nil {
		log.Printf("AuthHandler: API login failed for %s: %v", loginReq.Email, err)
		http.Error(w, "Invalid email or password", http.StatusUnauthorized)
		return
	}

	log.Printf("AuthHandler: User %s (%s) logged in via API", authResponse.User.Name, authResponse.User.Email)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(authResponse)
}

// Logout handles user logout
func (h *AuthHandler) Logout(w http.ResponseWriter, r *http.Request) {
	// Check if we're behind a proxy (Cloudflare Tunnel)
	behindProxy := os.Getenv("BEHIND_PROXY") == "true"
	secure := !behindProxy // Secure = false if behind proxy, true if direct HTTPS
	
	// Clear auth cookie
	http.SetCookie(w, &http.Cookie{
		Name:     "auth_token",
		Value:    "",
		Path:     "/",
		Expires:  time.Unix(0, 0),
		HttpOnly: true,
		Secure:   secure, // Adapt based on proxy configuration
		SameSite: http.SameSiteStrictMode,
	})

	log.Printf("AuthHandler: User logged out from %s", r.RemoteAddr)
	http.Redirect(w, r, "/login", http.StatusSeeOther)
}

// ForgotPasswordPage displays the forgot password form
func (h *AuthHandler) ForgotPasswordPage(w http.ResponseWriter, r *http.Request) {
	success := r.URL.Query().Get("success")
	resetUrl := r.URL.Query().Get("resetUrl")
	
	// Handle development reset URL display
	var successHTML template.HTML
	if success == "generated" && resetUrl != "" {
		successHTML = template.HTML(fmt.Sprintf("Reset link generated! For development: <a href='%s'>Click here to reset password</a>", resetUrl))
	} else if success != "" {
		successHTML = template.HTML(success)
	}
	
	data := struct {
		Title string
		Error string
		Success template.HTML
	}{
		Title: "Forgot Password - NFL Games",
		Error: r.URL.Query().Get("error"),
		Success: successHTML,
	}

	err := h.templates.ExecuteTemplate(w, "forgot-password.html", data)
	if err != nil {
		log.Printf("AuthHandler: Template error: %v", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
}

// ForgotPassword handles the forgot password form submission
func (h *AuthHandler) ForgotPassword(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	email := r.FormValue("email")
	if email == "" {
		http.Redirect(w, r, "/forgot-password?error=Please provide your email address", http.StatusSeeOther)
		return
	}

	// Request password reset
	token, err := h.authService.RequestPasswordReset(email)
	if err != nil {
		log.Printf("AuthHandler: Password reset request failed for %s: %v", email, err)
		// Always show success message to avoid revealing which emails exist
		http.Redirect(w, r, "/forgot-password?success=If the email exists, a reset link has been sent", http.StatusSeeOther)
		return
	}

	// Try to get user for email sending
	user, userErr := h.authService.GetUserByEmail(email)
	
	// If email service is configured and user exists and token was generated, send email
	if h.emailService.IsConfigured() && userErr == nil && token != "" {
		baseURL := fmt.Sprintf("http://%s", r.Host)
		err := h.emailService.SendPasswordResetEmail(user.Email, user.Name, token, baseURL)
		if err != nil {
			log.Printf("AuthHandler: Failed to send password reset email to %s: %v", email, err)
			// Fall back to development mode
			resetURL := fmt.Sprintf("%s/reset-password?token=%s", baseURL, token)
			log.Printf("AuthHandler: Falling back to development mode. Reset URL: %s", resetURL)
			http.Redirect(w, r, fmt.Sprintf("/forgot-password?success=generated&resetUrl=%s", resetURL), http.StatusSeeOther)
			return
		}
		
		log.Printf("AuthHandler: Password reset email sent to %s", email)
		http.Redirect(w, r, "/forgot-password?success=A password reset link has been sent to your email address", http.StatusSeeOther)
		return
	}

	// Check if user exists and token was generated
	if token == "" {
		// User not found, but don't reveal this for security - show success message
		log.Printf("AuthHandler: Password reset requested for non-existent email %s", email)
		http.Redirect(w, r, "/forgot-password?success=If the email exists, a reset link has been sent", http.StatusSeeOther)
		return
	}
	
	// Fall back to development mode (email not configured)
	resetURL := fmt.Sprintf("http://%s/reset-password?token=%s", r.Host, token)
	log.Printf("AuthHandler: Email service not configured. Reset URL: %s", resetURL)
	http.Redirect(w, r, fmt.Sprintf("/forgot-password?success=generated&resetUrl=%s", resetURL), http.StatusSeeOther)
}

// ResetPasswordPage displays the reset password form
func (h *AuthHandler) ResetPasswordPage(w http.ResponseWriter, r *http.Request) {
	token := r.URL.Query().Get("token")
	if token == "" {
		http.Redirect(w, r, "/login?error=Invalid reset link", http.StatusSeeOther)
		return
	}

	data := struct {
		Title string
		Token string
		Error string
	}{
		Title: "Reset Password - NFL Games",
		Token: token,
		Error: r.URL.Query().Get("error"),
	}

	err := h.templates.ExecuteTemplate(w, "reset-password.html", data)
	if err != nil {
		log.Printf("AuthHandler: Template error: %v", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
}

// ResetPassword handles the reset password form submission
func (h *AuthHandler) ResetPassword(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	token := r.FormValue("token")
	newPassword := r.FormValue("newPassword")
	confirmPassword := r.FormValue("confirmPassword")

	if token == "" {
		http.Redirect(w, r, "/login?error=Invalid reset link", http.StatusSeeOther)
		return
	}

	if newPassword == "" || confirmPassword == "" {
		http.Redirect(w, r, fmt.Sprintf("/reset-password?token=%s&error=Please fill in all fields", token), http.StatusSeeOther)
		return
	}

	if newPassword != confirmPassword {
		http.Redirect(w, r, fmt.Sprintf("/reset-password?token=%s&error=Passwords do not match", token), http.StatusSeeOther)
		return
	}

	// Reset password
	err := h.authService.ResetPassword(token, newPassword)
	if err != nil {
		log.Printf("AuthHandler: Password reset failed: %v", err)
		http.Redirect(w, r, fmt.Sprintf("/reset-password?token=%s&error=%s", token, err.Error()), http.StatusSeeOther)
		return
	}

	log.Printf("AuthHandler: Password successfully reset using token %s", token[:8]+"...")
	http.Redirect(w, r, "/login?success=Password reset successful! Please log in with your new password", http.StatusSeeOther)
}

// Me returns the current user's information
func (h *AuthHandler) Me(w http.ResponseWriter, r *http.Request) {
	user := middleware.GetUserFromContext(r)
	if user == nil {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(user.ToSafeUser())
}

// setAuthCookie sets the authentication cookie
func (h *AuthHandler) setAuthCookie(w http.ResponseWriter, token string) {
	// Check if we're behind a proxy (Cloudflare Tunnel)
	behindProxy := os.Getenv("BEHIND_PROXY") == "true"
	secure := !behindProxy // Secure = false if behind proxy, true if direct HTTPS
	
	http.SetCookie(w, &http.Cookie{
		Name:     "auth_token",
		Value:    token,
		Path:     "/",
		Expires:  time.Now().Add(24 * 180 * time.Hour), // 6 months (180 days)
		HttpOnly: true,
		Secure:   secure, // Adapt based on proxy configuration
		SameSite: http.SameSiteStrictMode,
	})
}