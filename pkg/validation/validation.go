package validation

import (
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
)

var (
	// DatabaseNameRegex allows alphanumeric, underscore, hyphen (MySQL, PostgreSQL, MongoDB compatible)
	DatabaseNameRegex = regexp.MustCompile(`^[a-zA-Z0-9_-]+$`)

	// TableNameRegex allows alphanumeric, underscore, hyphen, dot (for schema.table)
	TableNameRegex = regexp.MustCompile(`^[a-zA-Z0-9_.-]+$`)

	// BackupIDRegex matches the expected backup ID format
	BackupIDRegex = regexp.MustCompile(`^backup-\d{4}-\d{2}-\d{2}-\d{2}-\d{2}-\d{2}-[a-f0-9]{8}$`)
)

// ValidateDatabaseName validates a database name for security
func ValidateDatabaseName(name string) error {
	if name == "" {
		return fmt.Errorf("database name cannot be empty")
	}

	if len(name) > 64 {
		return fmt.Errorf("database name too long (max 64 characters)")
	}

	// Reject names starting with dash (could be interpreted as flags)
	if strings.HasPrefix(name, "-") {
		return fmt.Errorf("database name cannot start with dash")
	}

	// Reject names starting with dot (hidden files, path traversal)
	if strings.HasPrefix(name, ".") {
		return fmt.Errorf("database name cannot start with dot")
	}

	if !DatabaseNameRegex.MatchString(name) {
		return fmt.Errorf("database name contains invalid characters (only alphanumeric, underscore, and hyphen allowed)")
	}

	return nil
}

// ValidateTableName validates a table name for security
func ValidateTableName(name string) error {
	if name == "" {
		return fmt.Errorf("table name cannot be empty")
	}

	if len(name) > 128 {
		return fmt.Errorf("table name too long (max 128 characters)")
	}

	// Reject names starting with dash
	if strings.HasPrefix(name, "-") {
		return fmt.Errorf("table name cannot start with dash")
	}

	// Reject names starting with dot
	if strings.HasPrefix(name, ".") {
		return fmt.Errorf("table name cannot start with dot")
	}

	if !TableNameRegex.MatchString(name) {
		return fmt.Errorf("table name contains invalid characters")
	}

	return nil
}

// ValidateBackupID validates a backup ID to prevent path traversal
func ValidateBackupID(id string) error {
	if id == "" {
		return fmt.Errorf("backup ID cannot be empty")
	}

	// Check for path traversal attempts
	if strings.Contains(id, "..") {
		return fmt.Errorf("backup ID cannot contain '..'")
	}

	if strings.Contains(id, "/") || strings.Contains(id, "\\") {
		return fmt.Errorf("backup ID cannot contain path separators")
	}

	// Validate format (optional but recommended)
	if !BackupIDRegex.MatchString(id) {
		return fmt.Errorf("backup ID has invalid format")
	}

	return nil
}

// SanitizePath cleans and validates a file path
func SanitizePath(path string, baseDir string) (string, error) {
	// Clean the path to resolve .. and .
	cleaned := filepath.Clean(path)

	// If baseDir is provided, ensure the path is within it
	if baseDir != "" {
		absBase, err := filepath.Abs(baseDir)
		if err != nil {
			return "", fmt.Errorf("invalid base directory: %w", err)
		}

		absPath, err := filepath.Abs(cleaned)
		if err != nil {
			return "", fmt.Errorf("invalid path: %w", err)
		}

		// Ensure the absolute path is within the base directory
		relPath, err := filepath.Rel(absBase, absPath)
		if err != nil || strings.HasPrefix(relPath, "..") {
			return "", fmt.Errorf("path traversal detected: path must be within %s", baseDir)
		}
	}

	return cleaned, nil
}

// ValidatePort validates a port number
func ValidatePort(port int) error {
	if port < 1 || port > 65535 {
		return fmt.Errorf("port must be between 1 and 65535, got %d", port)
	}
	return nil
}

// ValidateCompressionLevel validates compression level
func ValidateCompressionLevel(level int) error {
	if level < -1 || level > 9 {
		return fmt.Errorf("compression level must be between -1 and 9, got %d", level)
	}
	return nil
}

// ValidateEncryptionKeyLength validates encryption key minimum length
func ValidateEncryptionKeyLength(key []byte, minLength int) error {
	if len(key) < minLength {
		return fmt.Errorf("encryption key too short: minimum %d bytes required, got %d", minLength, len(key))
	}
	return nil
}
