// Package utils provides common utility functions
package utils

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"strings"
	"time"
)

// GenerateRandomString generates a random string of specified length
func GenerateRandomString(length int) (string, error) {
	bytes := make([]byte, length)
	if _, err := rand.Read(bytes); err != nil {
		return "", fmt.Errorf("failed to generate random string: %w", err)
	}
	return base64.URLEncoding.EncodeToString(bytes)[:length], nil
}

// GenerateID generates a unique ID
func GenerateID(prefix string) (string, error) {
	random, err := GenerateRandomString(16)
	if err != nil {
		return "", err
	}
	if prefix != "" {
		return fmt.Sprintf("%s_%s", prefix, random), nil
	}
	return random, nil
}

// GenerateBackupID generates a unique backup ID with timestamp
func GenerateBackupID() string {
	return fmt.Sprintf("backup-%s-%d",
		time.Now().Format("20060102-150405"),
		time.Now().UnixNano()%1000000,
	)
}

// Truncate truncates a string to a maximum length
func Truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}

// Contains checks if a string slice contains a string
func Contains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}

// RemoveDuplicates removes duplicate strings from a slice
func RemoveDuplicates(slice []string) []string {
	seen := make(map[string]bool)
	result := []string{}
	for _, item := range slice {
		if !seen[item] {
			seen[item] = true
			result = append(result, item)
		}
	}
	return result
}

// SplitAndTrim splits a string by separator and trims whitespace
func SplitAndTrim(s, sep string) []string {
	parts := strings.Split(s, sep)
	result := make([]string, 0, len(parts))
	for _, part := range parts {
		trimmed := strings.TrimSpace(part)
		if trimmed != "" {
			result = append(result, trimmed)
		}
	}
	return result
}

// MaskSensitive masks sensitive information (like passwords, tokens)
func MaskSensitive(s string) string {
	if len(s) <= 8 {
		return "****"
	}
	return s[:4] + "****" + s[len(s)-4:]
}
