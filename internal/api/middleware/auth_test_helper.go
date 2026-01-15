package middleware

import (
	"time"

	"github.com/sanskarpan/db-backup/internal/auth"
)

// Helper function for tests to create a token service
func createTestTokenService() *auth.TokenService {
	secret := "test-secret-must-be-at-least-32-characters-long-for-security"
	return auth.NewTokenService(secret, 1*time.Hour)
}
