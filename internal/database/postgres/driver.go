// Package postgres provides PostgreSQL database driver implementation
package postgres

import (
	"context"
	"database/sql"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"time"

	_ "github.com/lib/pq"
	"github.com/sanskarpan/db-backup/internal/database"
	pkgErrors "github.com/sanskarpan/db-backup/pkg/errors"
	"github.com/sanskarpan/db-backup/pkg/utils"
	"github.com/sanskarpan/db-backup/pkg/validation"
)

// PostgreSQLDriver implements the database.Driver interface for PostgreSQL
type PostgreSQLDriver struct {
	db     *sql.DB
	config *database.ConnectionConfig
}

func init() {
	database.RegisterDriver(database.DatabaseTypePostgreSQL, func() database.Driver {
		return NewPostgreSQLDriver()
	})
}

// NewPostgreSQLDriver creates a new PostgreSQL driver instance
func NewPostgreSQLDriver() *PostgreSQLDriver {
	return &PostgreSQLDriver{}
}

// Connect establishes a connection to the PostgreSQL database
func (d *PostgreSQLDriver) Connect(ctx context.Context, config *database.ConnectionConfig) error {
	// Build connection string
	connStr := d.buildConnectionString(config)

	// Open database connection
	db, err := sql.Open("postgres", connStr)
	if err != nil {
		return pkgErrors.ErrDatabaseConnection(err)
	}

	// Set connection pool settings
	if config.MaxConnections > 0 {
		db.SetMaxOpenConns(config.MaxConnections)
		db.SetMaxIdleConns(config.MaxConnections / 2)
	} else {
		db.SetMaxOpenConns(25)
		db.SetMaxIdleConns(10)
	}
	db.SetConnMaxLifetime(time.Hour)

	// Test connection
	if err := db.PingContext(ctx); err != nil {
		db.Close()
		return pkgErrors.ErrDatabaseConnection(err)
	}

	d.db = db
	d.config = config
	return nil
}

// Disconnect closes the database connection
func (d *PostgreSQLDriver) Disconnect() error {
	if d.db != nil {
		return d.db.Close()
	}
	return nil
}

// Ping tests the database connection
func (d *PostgreSQLDriver) Ping(ctx context.Context) error {
	if d.db == nil {
		return pkgErrors.New(pkgErrors.ErrorTypeDatabase, "not connected to database")
	}
	return d.db.PingContext(ctx)
}

// Backup creates a backup of the PostgreSQL database
func (d *PostgreSQLDriver) Backup(ctx context.Context, opts *database.BackupOptions) (*database.BackupResult, error) {
	result := &database.BackupResult{
		ID:        utils.GenerateBackupID(),
		StartTime: time.Now(),
		Metadata:  opts.Metadata,
		Status:    database.BackupStatusInProgress,
	}

	// Build pg_dump command
	args, err := d.buildPgDumpArgs(opts)
	if err != nil {
		result.Status = database.BackupStatusFailed
		result.Error = err
		return result, pkgErrors.ErrDatabaseBackup(err)
	}

	// Create pg_dump command
	cmd := exec.CommandContext(ctx, "pg_dump", args...)

	// Set password via environment variable
	cmd.Env = append(os.Environ(), fmt.Sprintf("PGPASSWORD=%s", d.config.Password))

	// Create output file
	outputFile, err := os.Create(opts.OutputPath)
	if err != nil {
		result.Status = database.BackupStatusFailed
		result.Error = err
		return result, pkgErrors.ErrDatabaseBackup(err).WithMetadata("output_path", opts.OutputPath)
	}
	defer outputFile.Close()

	// Set command output to file
	cmd.Stdout = outputFile

	// Capture stderr for errors
	stderrPipe, err := cmd.StderrPipe()
	if err != nil {
		result.Status = database.BackupStatusFailed
		result.Error = err
		return result, pkgErrors.ErrDatabaseBackup(err)
	}

	// Start command
	if err := cmd.Start(); err != nil {
		result.Status = database.BackupStatusFailed
		result.Error = err
		return result, pkgErrors.ErrDatabaseBackup(err).WithMetadata("command", "pg_dump")
	}

	// Read stderr
	stderrOutput, _ := io.ReadAll(stderrPipe)

	// Wait for command to complete
	if err := cmd.Wait(); err != nil {
		result.Status = database.BackupStatusFailed
		result.Error = err
		return result, pkgErrors.ErrDatabaseBackup(err).WithMetadata("stderr", string(stderrOutput))
	}

	// Get file info
	fileInfo, err := outputFile.Stat()
	if err != nil {
		result.Status = database.BackupStatusFailed
		result.Error = err
		return result, err
	}

	// Get database version
	version, _ := d.GetVersion(ctx)

	// Get table information
	tables, _ := d.getTableInfo(ctx, opts.Database)

	// Complete result
	result.EndTime = time.Now()
	result.Duration = result.EndTime.Sub(result.StartTime)
	result.Size = fileInfo.Size()
	result.DatabaseVersion = version
	result.Tables = tables
	result.Status = database.BackupStatusSuccess

	return result, nil
}

// StreamBackup streams a backup to the provided writer
func (d *PostgreSQLDriver) StreamBackup(ctx context.Context, opts *database.BackupOptions, writer io.Writer) error {
	args, err := d.buildPgDumpArgs(opts)
	if err != nil {
		return pkgErrors.ErrDatabaseBackup(err)
	}

	cmd := exec.CommandContext(ctx, "pg_dump", args...)
	cmd.Env = append(os.Environ(), fmt.Sprintf("PGPASSWORD=%s", d.config.Password))
	cmd.Stdout = writer

	return cmd.Run()
}

// GetBackupSize estimates the size of a backup
func (d *PostgreSQLDriver) GetBackupSize(ctx context.Context, opts *database.BackupOptions) (int64, error) {
	var totalSize int64

	if opts.Database != "" {
		query := `SELECT pg_database_size($1)`
		if err := d.db.QueryRowContext(ctx, query, opts.Database).Scan(&totalSize); err != nil {
			return 0, err
		}
	}

	return totalSize, nil
}

// Restore restores a PostgreSQL database from backup
func (d *PostgreSQLDriver) Restore(ctx context.Context, opts *database.RestoreOptions) (*database.RestoreResult, error) {
	result := &database.RestoreResult{
		StartTime: time.Now(),
		Status:    database.RestoreStatusInProgress,
	}

	// Validate backup file exists
	if _, err := os.Stat(opts.SourceBackup); os.IsNotExist(err) {
		result.Status = database.RestoreStatusFailed
		result.Error = err
		return result, pkgErrors.ErrDatabaseRestore(err).WithMetadata("backup_file", opts.SourceBackup)
	}

	// Build pg_restore or psql command
	var args []string
	var err error
	cmdName := "pg_restore"

	// Check if this is a custom format backup or SQL dump
	if strings.HasSuffix(opts.SourceBackup, ".sql") {
		cmdName = "psql"
		args, err = d.buildPsqlArgs(opts)
	} else {
		args, err = d.buildRestoreArgs(opts)
	}

	if err != nil {
		result.Status = database.RestoreStatusFailed
		result.Error = err
		return result, pkgErrors.ErrDatabaseRestore(err)
	}

	// Create command
	cmd := exec.CommandContext(ctx, cmdName, args...)
	cmd.Env = append(os.Environ(), fmt.Sprintf("PGPASSWORD=%s", d.config.Password))

	// For SQL dumps, read from file
	if cmdName == "psql" {
		backupFile, err := os.Open(opts.SourceBackup)
		if err != nil {
			result.Status = database.RestoreStatusFailed
			result.Error = err
			return result, pkgErrors.ErrDatabaseRestore(err)
		}
		defer backupFile.Close()
		cmd.Stdin = backupFile
	}

	// Capture stderr
	stderrPipe, _ := cmd.StderrPipe()

	// Run command
	if err := cmd.Start(); err != nil {
		result.Status = database.RestoreStatusFailed
		result.Error = err
		return result, pkgErrors.ErrDatabaseRestore(err)
	}

	stderrOutput, _ := io.ReadAll(stderrPipe)

	if err := cmd.Wait(); err != nil {
		result.Status = database.RestoreStatusFailed
		result.Error = err
		return result, pkgErrors.ErrDatabaseRestore(err).WithMetadata("stderr", string(stderrOutput))
	}

	result.EndTime = time.Now()
	result.Duration = result.EndTime.Sub(result.StartTime)
	result.Status = database.RestoreStatusSuccess

	return result, nil
}

// StreamRestore restores from a reader
func (d *PostgreSQLDriver) StreamRestore(ctx context.Context, opts *database.RestoreOptions, reader io.Reader) error {
	args, err := d.buildPsqlArgs(opts)
	if err != nil {
		return pkgErrors.ErrDatabaseRestore(err)
	}

	cmd := exec.CommandContext(ctx, "psql", args...)
	cmd.Env = append(os.Environ(), fmt.Sprintf("PGPASSWORD=%s", d.config.Password))
	cmd.Stdin = reader

	return cmd.Run()
}

// ValidateRestore validates that a restore can be performed
func (d *PostgreSQLDriver) ValidateRestore(ctx context.Context, opts *database.RestoreOptions) error {
	// Check if backup file exists
	if _, err := os.Stat(opts.SourceBackup); os.IsNotExist(err) {
		return pkgErrors.ErrValidationFailed(fmt.Sprintf("backup file not found: %s", opts.SourceBackup))
	}

	// Check database connection
	if err := d.Ping(ctx); err != nil {
		return pkgErrors.ErrValidationFailed("database connection failed")
	}

	return nil
}

// GetDatabases returns list of databases
func (d *PostgreSQLDriver) GetDatabases(ctx context.Context) ([]string, error) {
	query := `SELECT datname FROM pg_database WHERE datistemplate = false AND datname != 'postgres'`
	rows, err := d.db.QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var databases []string
	for rows.Next() {
		var dbName string
		if err := rows.Scan(&dbName); err != nil {
			return nil, err
		}
		databases = append(databases, dbName)
	}

	return databases, nil
}

// GetTables returns list of tables in a database
func (d *PostgreSQLDriver) GetTables(ctx context.Context, database string) ([]string, error) {
	query := `SELECT tablename FROM pg_tables WHERE schemaname = 'public'`
	rows, err := d.db.QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var tables []string
	for rows.Next() {
		var tableName string
		if err := rows.Scan(&tableName); err != nil {
			return nil, err
		}
		tables = append(tables, tableName)
	}

	return tables, nil
}

// GetTableSize returns the size of a table
func (d *PostgreSQLDriver) GetTableSize(ctx context.Context, database, table string) (int64, error) {
	query := `SELECT pg_total_relation_size($1)`

	var size int64
	err := d.db.QueryRowContext(ctx, query, table).Scan(&size)
	return size, err
}

// GetVersion returns the PostgreSQL server version
func (d *PostgreSQLDriver) GetVersion(ctx context.Context) (string, error) {
	var version string
	err := d.db.QueryRowContext(ctx, "SELECT version()").Scan(&version)
	return version, err
}

// GetType returns the database type
func (d *PostgreSQLDriver) GetType() database.DatabaseType {
	return database.DatabaseTypePostgreSQL
}

// SupportsIncremental returns whether incremental backups are supported
func (d *PostgreSQLDriver) SupportsIncremental() bool {
	return true // PostgreSQL supports incremental backups via WAL
}

// SupportsPITR returns whether point-in-time recovery is supported
func (d *PostgreSQLDriver) SupportsPITR() bool {
	return true // PostgreSQL supports PITR via WAL
}

// buildConnectionString builds a PostgreSQL connection string
func (d *PostgreSQLDriver) buildConnectionString(config *database.ConnectionConfig) string {
	if config.ConnectionString != "" {
		return config.ConnectionString
	}

	sslMode := config.SSLMode
	if sslMode == "" {
		sslMode = "disable"
	}

	return fmt.Sprintf("host=%s port=%d user=%s password=%s dbname=%s sslmode=%s connect_timeout=%d",
		config.Host,
		config.Port,
		config.Username,
		config.Password,
		config.Database,
		sslMode,
		int(config.ConnectionTimeout.Seconds()),
	)
}

// buildPgDumpArgs builds pg_dump command arguments
func (d *PostgreSQLDriver) buildPgDumpArgs(opts *database.BackupOptions) ([]string, error) {
	args := []string{
		"-h", d.config.Host,
		"-p", fmt.Sprintf("%d", d.config.Port),
		"-U", d.config.Username,
		"-F", "c", // Custom format for better compression and parallel restore
		"-v",      // Verbose
		"--no-owner",
		"--no-acl",
	}

	if opts.Parallel > 1 {
		args = append(args, "-j", fmt.Sprintf("%d", opts.Parallel))
	}

	if opts.ConsistentBackup {
		args = append(args, "--serializable-deferrable")
	}

	// Table selection
	if len(opts.Tables) > 0 {
		for _, table := range opts.Tables {
			// Validate table name to prevent command injection
			if err := validation.ValidateTableName(table); err != nil {
				return nil, fmt.Errorf("invalid table name %q: %w", table, err)
			}
			args = append(args, "-t", table)
		}
	}

	// Exclude tables
	if len(opts.ExcludeTables) > 0 {
		for _, table := range opts.ExcludeTables {
			// Validate excluded table name
			if err := validation.ValidateTableName(table); err != nil {
				return nil, fmt.Errorf("invalid excluded table name %q: %w", table, err)
			}
			args = append(args, "-T", table)
		}
	}

	// Validate and add database name
	if opts.Database != "" {
		if err := validation.ValidateDatabaseName(opts.Database); err != nil {
			return nil, fmt.Errorf("invalid database name %q: %w", opts.Database, err)
		}
		args = append(args, opts.Database)
	}

	return args, nil
}

// buildRestoreArgs builds pg_restore command arguments
func (d *PostgreSQLDriver) buildRestoreArgs(opts *database.RestoreOptions) ([]string, error) {
	// Validate database name if provided
	if opts.Database != "" {
		if err := validation.ValidateDatabaseName(opts.Database); err != nil {
			return nil, fmt.Errorf("invalid database name %q: %w", opts.Database, err)
		}
	}

	args := []string{
		"-h", d.config.Host,
		"-p", fmt.Sprintf("%d", d.config.Port),
		"-U", d.config.Username,
		"-d", opts.Database,
		"-v",
		"--no-owner",
		"--no-acl",
	}

	if opts.Parallel > 1 {
		args = append(args, "-j", fmt.Sprintf("%d", opts.Parallel))
	}

	if opts.DropExisting {
		args = append(args, "--clean")
	}

	if len(opts.Tables) > 0 {
		for _, table := range opts.Tables {
			// Validate table name
			if err := validation.ValidateTableName(table); err != nil {
				return nil, fmt.Errorf("invalid table name %q: %w", table, err)
			}
			args = append(args, "-t", table)
		}
	}

	args = append(args, opts.SourceBackup)

	return args, nil
}

// buildPsqlArgs builds psql command arguments
func (d *PostgreSQLDriver) buildPsqlArgs(opts *database.RestoreOptions) ([]string, error) {
	// Validate database name if provided
	if opts.Database != "" {
		if err := validation.ValidateDatabaseName(opts.Database); err != nil {
			return nil, fmt.Errorf("invalid database name %q: %w", opts.Database, err)
		}
	}

	args := []string{
		"-h", d.config.Host,
		"-p", fmt.Sprintf("%d", d.config.Port),
		"-U", d.config.Username,
		"-d", opts.Database,
	}

	return args, nil
}

// getTableInfo retrieves information about tables
func (d *PostgreSQLDriver) getTableInfo(ctx context.Context, dbName string) ([]database.TableInfo, error) {
	query := `
		SELECT
			tablename,
			pg_total_relation_size(schemaname||'.'||tablename) as total_size,
			pg_relation_size(schemaname||'.'||tablename) as data_size,
			pg_indexes_size(schemaname||'.'||tablename) as index_size
		FROM pg_tables
		WHERE schemaname = 'public'
	`

	rows, err := d.db.QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var tables []database.TableInfo
	for rows.Next() {
		var info database.TableInfo
		var totalSize int64
		if err := rows.Scan(&info.Name, &totalSize, &info.DataSize, &info.IndexSize); err != nil {
			return nil, err
		}
		// Note: PostgreSQL doesn't have easy row count, would need to query each table
		tables = append(tables, info)
	}

	return tables, nil
}

