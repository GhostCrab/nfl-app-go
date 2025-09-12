package database

import (
	"context"
	"time"
)

// ContextWithTimeout creates a context with timeout and cancel function
// This utility eliminates the repetitive pattern across all repositories
func ContextWithTimeout(timeout time.Duration) (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), timeout)
}

// Common timeout durations for database operations
const (
	// ShortTimeout for quick operations like create, update, delete single documents
	ShortTimeout = 5 * time.Second
	
	// MediumTimeout for queries that might return multiple documents or complex operations
	MediumTimeout = 10 * time.Second
	
	// LongTimeout for bulk operations, migrations, or complex aggregations
	LongTimeout = 30 * time.Second
	
	// VeryLongTimeout for maintenance operations like full data imports
	VeryLongTimeout = 60 * time.Second
)

// WithShortTimeout creates a context with ShortTimeout (5 seconds)
func WithShortTimeout() (context.Context, context.CancelFunc) {
	return ContextWithTimeout(ShortTimeout)
}

// WithMediumTimeout creates a context with MediumTimeout (10 seconds) 
func WithMediumTimeout() (context.Context, context.CancelFunc) {
	return ContextWithTimeout(MediumTimeout)
}

// WithLongTimeout creates a context with LongTimeout (30 seconds)
func WithLongTimeout() (context.Context, context.CancelFunc) {
	return ContextWithTimeout(LongTimeout)
}

// WithVeryLongTimeout creates a context with VeryLongTimeout (60 seconds)
func WithVeryLongTimeout() (context.Context, context.CancelFunc) {
	return ContextWithTimeout(VeryLongTimeout)
}