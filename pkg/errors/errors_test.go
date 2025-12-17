package errors

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNew(t *testing.T) {
	tests := []struct {
		name     string
		errType  ErrorType
		message  string
		expected string
	}{
		{
			name:     "database error",
			errType:  ErrorTypeDatabase,
			message:  "connection failed",
			expected: "[DATABASE] connection failed",
		},
		{
			name:     "storage error",
			errType:  ErrorTypeStorage,
			message:  "upload failed",
			expected: "[STORAGE] upload failed",
		},
		{
			name:     "validation error",
			errType:  ErrorTypeValidation,
			message:  "invalid input",
			expected: "[VALIDATION] invalid input",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := New(tt.errType, tt.message)

			assert.NotNil(t, err)
			assert.Equal(t, tt.errType, err.Type)
			assert.Equal(t, tt.message, err.Message)
			assert.Equal(t, tt.expected, err.Error())
			assert.NotEmpty(t, err.StackTrace)
			assert.NotNil(t, err.Metadata)
		})
	}
}

func TestWrap(t *testing.T) {
	t.Run("wrap standard error", func(t *testing.T) {
		originalErr := errors.New("original error")
		wrappedErr := Wrap(originalErr, ErrorTypeDatabase, "failed to connect")

		assert.NotNil(t, wrappedErr)
		assert.Equal(t, ErrorTypeDatabase, wrappedErr.Type)
		assert.Equal(t, "failed to connect", wrappedErr.Message)
		assert.Equal(t, originalErr, wrappedErr.Err)
		assert.Contains(t, wrappedErr.Error(), "failed to connect")
		assert.Contains(t, wrappedErr.Error(), "original error")
	})

	t.Run("wrap nil error", func(t *testing.T) {
		wrappedErr := Wrap(nil, ErrorTypeDatabase, "failed to connect")
		assert.Nil(t, wrappedErr)
	})

	t.Run("wrap BackupError preserves type", func(t *testing.T) {
		originalErr := New(ErrorTypeStorage, "original message")
		wrappedErr := Wrap(originalErr, "", "wrapped message")

		assert.NotNil(t, wrappedErr)
		assert.Equal(t, ErrorTypeStorage, wrappedErr.Type)
		assert.Equal(t, "wrapped message", wrappedErr.Message)
	})
}

func TestWithMetadata(t *testing.T) {
	err := New(ErrorTypeDatabase, "test error")
	err.WithMetadata("key1", "value1")
	err.WithMetadata("key2", 42)

	assert.Equal(t, "value1", err.Metadata["key1"])
	assert.Equal(t, 42, err.Metadata["key2"])
}

func TestIs(t *testing.T) {
	err1 := New(ErrorTypeDatabase, "error1")
	err2 := New(ErrorTypeDatabase, "error2")
	err3 := New(ErrorTypeStorage, "error3")

	assert.True(t, err1.Is(err2))
	assert.False(t, err1.Is(err3))
}

func TestUnwrap(t *testing.T) {
	originalErr := errors.New("original")
	wrappedErr := Wrap(originalErr, ErrorTypeDatabase, "wrapped")

	unwrapped := wrappedErr.Unwrap()
	assert.Equal(t, originalErr, unwrapped)
}

func TestErrorConstructors(t *testing.T) {
	testErr := errors.New("test error")

	t.Run("ErrDatabaseConnection", func(t *testing.T) {
		err := ErrDatabaseConnection(testErr)
		assert.NotNil(t, err)
		assert.Equal(t, ErrorTypeDatabase, err.Type)
		assert.Contains(t, err.Error(), "failed to connect to database")
	})

	t.Run("ErrDatabaseBackup", func(t *testing.T) {
		err := ErrDatabaseBackup(testErr)
		assert.NotNil(t, err)
		assert.Equal(t, ErrorTypeDatabase, err.Type)
		assert.Contains(t, err.Error(), "failed to create database backup")
	})

	t.Run("ErrDatabaseRestore", func(t *testing.T) {
		err := ErrDatabaseRestore(testErr)
		assert.NotNil(t, err)
		assert.Equal(t, ErrorTypeDatabase, err.Type)
		assert.Contains(t, err.Error(), "failed to restore database")
	})

	t.Run("ErrStorageUpload", func(t *testing.T) {
		err := ErrStorageUpload(testErr)
		assert.NotNil(t, err)
		assert.Equal(t, ErrorTypeStorage, err.Type)
		assert.Contains(t, err.Error(), "failed to upload to storage")
	})

	t.Run("ErrStorageDownload", func(t *testing.T) {
		err := ErrStorageDownload(testErr)
		assert.NotNil(t, err)
		assert.Equal(t, ErrorTypeStorage, err.Type)
		assert.Contains(t, err.Error(), "failed to download from storage")
	})

	t.Run("ErrCompressionFailed", func(t *testing.T) {
		err := ErrCompressionFailed(testErr)
		assert.NotNil(t, err)
		assert.Equal(t, ErrorTypeCompression, err.Type)
	})

	t.Run("ErrEncryptionFailed", func(t *testing.T) {
		err := ErrEncryptionFailed(testErr)
		assert.NotNil(t, err)
		assert.Equal(t, ErrorTypeEncryption, err.Type)
	})

	t.Run("ErrValidationFailed", func(t *testing.T) {
		err := ErrValidationFailed("invalid input")
		assert.NotNil(t, err)
		assert.Equal(t, ErrorTypeValidation, err.Type)
		assert.Equal(t, "invalid input", err.Message)
	})

	t.Run("ErrConfigInvalid", func(t *testing.T) {
		err := ErrConfigInvalid(testErr)
		assert.NotNil(t, err)
		assert.Equal(t, ErrorTypeConfiguration, err.Type)
	})
}

func TestIsRetryable(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		expected bool
	}{
		{
			name:     "network error is retryable",
			err:      New(ErrorTypeNetwork, "connection timeout"),
			expected: true,
		},
		{
			name:     "storage error is retryable",
			err:      New(ErrorTypeStorage, "upload failed"),
			expected: true,
		},
		{
			name:     "database error is not retryable",
			err:      New(ErrorTypeDatabase, "connection failed"),
			expected: false,
		},
		{
			name:     "validation error is not retryable",
			err:      New(ErrorTypeValidation, "invalid input"),
			expected: false,
		},
		{
			name:     "standard error is not retryable",
			err:      errors.New("standard error"),
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := IsRetryable(tt.err)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestStackTrace(t *testing.T) {
	err := New(ErrorTypeDatabase, "test error")

	// Stack trace should contain function names and file paths
	assert.NotEmpty(t, err.StackTrace)
	assert.Contains(t, err.StackTrace, "errors_test.go")
}

func BenchmarkNew(b *testing.B) {
	for i := 0; i < b.N; i++ {
		_ = New(ErrorTypeDatabase, "benchmark error")
	}
}

func BenchmarkWrap(b *testing.B) {
	err := errors.New("original error")
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = Wrap(err, ErrorTypeDatabase, "wrapped error")
	}
}
