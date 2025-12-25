package auth

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
)

// GenerateToken generates a random token for runner authentication
func GenerateToken() (string, error) {
	bytes := make([]byte, 32)
	if _, err := rand.Read(bytes); err != nil {
		return "", fmt.Errorf("failed to generate token: %w", err)
	}
	return hex.EncodeToString(bytes), nil
}

// ValidateToken validates a runner token (simple implementation)
// In production, this would check against a database or token store
func ValidateToken(token string) bool {
	// For now, accept any non-empty token
	// TODO: Implement proper token validation
	return len(token) > 0
}

