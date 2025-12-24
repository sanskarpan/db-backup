// Package database provides database driver interfaces and implementations
package database

import (
	"context"
	"io"
	"time"

	"github.com/sanskarpan/db-backup/internal/types"
)

// DatabaseType represents the type of database
type DatabaseType string

const (
	DatabaseTypeMySQL      DatabaseType = "mysql"
	DatabaseTypePostgreSQL DatabaseType = "postgres"
	DatabaseTypeMongoDB    DatabaseType = "mongodb"
	DatabaseTypeSQLite     DatabaseType = "sqlite"
)

// Driver interface that all database drivers must implement
type Driver interface {
	// Connection management
	Connect(ctx context.Context, config *ConnectionConfig) error
	Disconnect() error
	Ping(ctx context.Context) error

	// Backup operations
	Backup(ctx context.Context, opts *BackupOptions) (*BackupResult, error)
	StreamBackup(ctx context.Context, opts *BackupOptions, writer io.Writer) error
	GetBackupSize(ctx context.Context, opts *BackupOptions) (int64, error)

	// Restore operations
	Restore(ctx context.Context, opts *RestoreOptions) (*RestoreResult, error)
	StreamRestore(ctx context.Context, opts *RestoreOptions, reader io.Reader) error
	ValidateRestore(ctx context.Context, opts *RestoreOptions) error

	// Metadata
	GetDatabases(ctx context.Context) ([]string, error)
	GetTables(ctx context.Context, database string) ([]string, error)
	GetTableSize(ctx context.Context, database, table string) (int64, error)
	GetVersion(ctx context.Context) (string, error)

	// Utility
	GetType() DatabaseType
	SupportsIncremental() bool
	SupportsPITR() bool
}

// ConnectionConfig holds database connection configuration
type ConnectionConfig struct {
	Type              DatabaseType
	Host              string
	Port              int
	Username          string
	Password          string
	Database          string
	SSLMode           string
	ConnectionString  string
	Options           map[string]string
	ConnectionTimeout time.Duration
	MaxConnections    int
}

// BackupOptions holds backup operation options
type BackupOptions struct {
	Database         string
	Databases        []string
	AllDatabases     bool
	Tables           []string
	ExcludeTables    []string
	Incremental      bool
	ConsistentBackup bool
	OutputPath       string
	Compression      CompressionType
	Parallel         int
	ChunkSize        int64
	Metadata         map[string]string
}

// RestoreOptions holds restore operation options
type RestoreOptions struct {
	Database       string
	SourceBackup   string
	Tables         []string
	ExcludeTables  []string
	PointInTime    *time.Time
	SkipValidation bool
	Parallel       int
	DropExisting   bool
	Metadata       map[string]string
}

// BackupResult contains the result of a backup operation
type BackupResult struct {
	ID              string
	StartTime       time.Time
	EndTime         time.Time
	Duration        time.Duration
	Size            int64
	CompressedSize  int64
	DatabaseVersion string
	Tables          []TableInfo
	Checksum        string
	Metadata        map[string]string
	Status          BackupStatus
	Error           error
}

// RestoreResult contains the result of a restore operation
type RestoreResult struct {
	StartTime      time.Time
	EndTime        time.Time
	Duration       time.Duration
	RestoredTables []string
	RowsRestored   int64
	Status         RestoreStatus
	Error          error
}

// TableInfo contains information about a table
type TableInfo struct {
	Name      string
	RowCount  int64
	DataSize  int64
	IndexSize int64
}

// BackupStatus represents the status of a backup
type BackupStatus string

const (
	BackupStatusPending    BackupStatus = "pending"
	BackupStatusInProgress BackupStatus = "in_progress"
	BackupStatusSuccess    BackupStatus = "success"
	BackupStatusFailed     BackupStatus = "failed"
)

// RestoreStatus represents the status of a restore
type RestoreStatus string

const (
	RestoreStatusPending    RestoreStatus = "pending"
	RestoreStatusInProgress RestoreStatus = "in_progress"
	RestoreStatusSuccess    RestoreStatus = "success"
	RestoreStatusFailed     RestoreStatus = "failed"
)

// CompressionType is an alias for types.CompressionType for backward compatibility
type CompressionType = types.CompressionType

// Re-export compression type constants
const (
	CompressionNone = types.CompressionNone
	CompressionGzip = types.CompressionGzip
	CompressionZstd = types.CompressionZstd
	CompressionLZ4  = types.CompressionLZ4
)

// DriverError represents a driver-specific error
type DriverError struct {
	Type    DatabaseType
	Message string
	Err     error
}

func (e *DriverError) Error() string {
	if e.Err != nil {
		return string(e.Type) + ": " + e.Message + ": " + e.Err.Error()
	}
	return string(e.Type) + ": " + e.Message
}

func (e *DriverError) Unwrap() error {
	return e.Err
}
