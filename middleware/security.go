package middleware

import (
	"net/http"
	"os"
)

// SecurityMiddleware adds security headers to all responses
func SecurityMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		behindProxy := os.Getenv("BEHIND_PROXY") == "true"
		
		// Only set HSTS if we're handling TLS directly or behind a proxy that handles HTTPS
		if !behindProxy || r.Header.Get("X-Forwarded-Proto") == "https" || r.Header.Get("CF-Visitor") != "" {
			// Strict Transport Security - force HTTPS for 1 year
			w.Header().Set("Strict-Transport-Security", "max-age=31536000; includeSubDomains")
		}
		
		// Prevent clickjacking
		w.Header().Set("X-Frame-Options", "DENY")
		
		// Prevent MIME type sniffing
		w.Header().Set("X-Content-Type-Options", "nosniff")
		
		// XSS protection
		w.Header().Set("X-XSS-Protection", "1; mode=block")
		
		// Referrer policy
		w.Header().Set("Referrer-Policy", "strict-origin-when-cross-origin")
		
		// Content Security Policy - allow external resources we need
		csp := "default-src 'self'; " +
			"script-src 'self' 'unsafe-inline' https://unpkg.com; " +
			"style-src 'self' 'unsafe-inline' https://fonts.googleapis.com https://cdn.jsdelivr.net https://cdnjs.cloudflare.com; " +
			"font-src 'self' https://fonts.gstatic.com https://cdn.jsdelivr.net https://cdnjs.cloudflare.com; " +
			"img-src 'self' data: https://ssl.gstatic.com https://a.espncdn.com; " +
			"connect-src 'self'"
		w.Header().Set("Content-Security-Policy", csp)
		
		next.ServeHTTP(w, r)
	})
}