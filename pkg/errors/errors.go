// Package errors provides custom error types and utilities for the backup utility
package errors

import (
	"errors"
	"fmt"
	"runtime"
)

// ErrorType represents the category of error
type ErrorType string

const (
	// ErrorTypeDatabase represents database-related errors
	ErrorTypeDatabase ErrorType = "DATABASE"
	// ErrorTypeStorage represents storage-related errors
	ErrorTypeStorage ErrorType = "STORAGE"
	// ErrorTypeCompression represents compression-related errors
	ErrorTypeCompression ErrorType = "COMPRESSION"
	// ErrorTypeEncryption represents encryption-related errors
	ErrorTypeEncryption ErrorType = "ENCRYPTION"
	// ErrorTypeValidation represents validation errors
	ErrorTypeValidation ErrorType = "VALIDATION"
	// ErrorTypeConfiguration represents configuration errors
	ErrorTypeConfiguration ErrorType = "CONFIGURATION"
	// ErrorTypeNetwork represents network-related errors
	ErrorTypeNetwork ErrorType = "NETWORK"
	// ErrorTypeRetention represents retention policy errors
	ErrorTypeRetention ErrorType = "RETENTION"
	// ErrorTypeOperation represents general operation errors
	ErrorTypeOperation ErrorType = "OPERATION"
	// ErrorTypeNotFound represents resource not found errors
	ErrorTypeNotFound ErrorType = "NOT_FOUND"
	// ErrorTypeInternal represents internal/unknown errors
	ErrorTypeInternal ErrorType = "INTERNAL"
)

// BackupError represents a structured error with context
type BackupError struct {
	Type       ErrorType
	Message    string
	Err        error
	StackTrace string
	Metadata   map[string]interface{}
}

// Error implements the error interface
func (e *BackupError) Error() string {
	if e.Err != nil {
		return fmt.Sprintf("[%s] %s: %v", e.Type, e.Message, e.Err)
	}
	return fmt.Sprintf("[%s] %s", e.Type, e.Message)
}

// Unwrap returns the wrapped error
func (e *BackupError) Unwrap() error {
	return e.Err
}

// New creates a new BackupError
func New(errType ErrorType, message string) *BackupError {
	return &BackupError{
		Type:       errType,
		Message:    message,
		StackTrace: captureStackTrace(),
		Metadata:   make(map[string]interface{}),
	}
}

// Wrap wraps an existing error with additional context
func Wrap(err error, errType ErrorType, message string) *BackupError {
	if err == nil {
		return nil
	}

	// If already a BackupError, preserve the original type unless specified
	var backupErr *BackupError
	if errors.As(err, &backupErr) {
		if errType == "" {
			errType = backupErr.Type
		}
	}

	return &BackupError{
		Type:       errType,
		Message:    message,
		Err:        err,
		StackTrace: captureStackTrace(),
		Metadata:   make(map[string]interface{}),
	}
}

// WithMetadata adds metadata to the error
func (e *BackupError) WithMetadata(key string, value interface{}) *BackupError {
	if e.Metadata == nil {
		e.Metadata = make(map[string]interface{})
	}
	e.Metadata[key] = value
	return e
}

// Is checks if the error matches the target error type
func (e *BackupError) Is(target error) bool {
	t, ok := target.(*BackupError)
	if !ok {
		return false
	}
	return e.Type == t.Type
}

// captureStackTrace captures the current stack trace
func captureStackTrace() string {
	const depth = 32
	var pcs [depth]uintptr
	n := runtime.Callers(3, pcs[:])
	frames := runtime.CallersFrames(pcs[:n])

	stackTrace := ""
	for {
		frame, more := frames.Next()
		stackTrace += fmt.Sprintf("%s\n\t%s:%d\n", frame.Function, frame.File, frame.Line)
		if !more {
			break
		}
	}
	return stackTrace
}

// Common error constructors

// ErrDatabaseConnection creates a database connection error
func ErrDatabaseConnection(err error) *BackupError {
	return Wrap(err, ErrorTypeDatabase, "failed to connect to database")
}

// ErrDatabaseBackup creates a database backup error
func ErrDatabaseBackup(err error) *BackupError {
	return Wrap(err, ErrorTypeDatabase, "failed to create database backup")
}

// ErrDatabaseRestore creates a database restore error
func ErrDatabaseRestore(err error) *BackupError {
	return Wrap(err, ErrorTypeDatabase, "failed to restore database")
}

// ErrStorageUpload creates a storage upload error
func ErrStorageUpload(err error) *BackupError {
	return Wrap(err, ErrorTypeStorage, "failed to upload to storage")
}

// ErrStorageDownload creates a storage download error
func ErrStorageDownload(err error) *BackupError {
	return Wrap(err, ErrorTypeStorage, "failed to download from storage")
}

// ErrCompressionFailed creates a compression error
func ErrCompressionFailed(err error) *BackupError {
	return Wrap(err, ErrorTypeCompression, "failed to compress data")
}

// ErrDecompressionFailed creates a decompression error
func ErrDecompressionFailed(err error) *BackupError {
	return Wrap(err, ErrorTypeCompression, "failed to decompress data")
}

// ErrEncryptionFailed creates an encryption error
func ErrEncryptionFailed(err error) *BackupError {
	return Wrap(err, ErrorTypeEncryption, "failed to encrypt data")
}

// ErrDecryptionFailed creates a decryption error
func ErrDecryptionFailed(err error) *BackupError {
	return Wrap(err, ErrorTypeEncryption, "failed to decrypt data")
}

// ErrValidationFailed creates a validation error
func ErrValidationFailed(message string) *BackupError {
	return New(ErrorTypeValidation, message)
}

// ErrConfigInvalid creates a configuration error
func ErrConfigInvalid(err error) *BackupError {
	return Wrap(err, ErrorTypeConfiguration, "invalid configuration")
}

// ErrRetentionPolicy creates a retention policy error
func ErrRetentionPolicy(err error) *BackupError {
	return Wrap(err, ErrorTypeRetention, "retention policy error")
}

// ErrOperationFailed creates a general operation error
func ErrOperationFailed(message string) *BackupError {
	return New(ErrorTypeOperation, message)
}

// ErrNotFound creates a not found error
func ErrNotFound(message string) *BackupError {
	return New(ErrorTypeNotFound, message)
}

// IsRetryable checks if an error is retryable
func IsRetryable(err error) bool {
	var backupErr *BackupError
	if errors.As(err, &backupErr) {
		// Network and some storage errors are typically retryable
		return backupErr.Type == ErrorTypeNetwork || backupErr.Type == ErrorTypeStorage
	}
	return false
}
