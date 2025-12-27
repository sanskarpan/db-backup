// Package mysql provides MySQL database driver implementation
package mysql

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	sql "database/sql"
	"time"

	_ "github.com/go-sql-driver/mysql"
	"github.com/sanskarpan/db-backup/internal/database"
	pkgErrors "github.com/sanskarpan/db-backup/pkg/errors"
	"github.com/sanskarpan/db-backup/pkg/utils"
	"github.com/sanskarpan/db-backup/pkg/validation"
)

// MySQLDriver implements the database.Driver interface for MySQL
type MySQLDriver struct {
	db     *sql.DB
	config *database.ConnectionConfig
}

func init() {
	database.RegisterDriver(database.DatabaseTypeMySQL, func() database.Driver {
		return NewMySQLDriver()
	})
}

// NewMySQLDriver creates a new MySQL driver instance
func NewMySQLDriver() *MySQLDriver {
	return &MySQLDriver{}
}

// Connect establishes a connection to the MySQL database
func (d *MySQLDriver) Connect(ctx context.Context, config *database.ConnectionConfig) error {
	// Build DSN (Data Source Name)
	dsn := d.buildDSN(config)

	// Open database connection
	db, err := sql.Open("mysql", dsn)
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
func (d *MySQLDriver) Disconnect() error {
	if d.db != nil {
		return d.db.Close()
	}
	return nil
}

// Ping tests the database connection
func (d *MySQLDriver) Ping(ctx context.Context) error {
	if d.db == nil {
		return pkgErrors.New(pkgErrors.ErrorTypeDatabase, "not connected to database")
	}
	return d.db.PingContext(ctx)
}

// Backup creates a backup of the MySQL database
func (d *MySQLDriver) Backup(ctx context.Context, opts *database.BackupOptions) (*database.BackupResult, error) {
	result := &database.BackupResult{
		ID:        utils.GenerateBackupID(),
		StartTime: time.Now(),
		Metadata:  opts.Metadata,
		Status:    database.BackupStatusInProgress,
	}

	// Build mysqldump command
	args, err := d.buildMySQLDumpArgs(opts)
	if err != nil {
		result.Status = database.BackupStatusFailed
		result.Error = err
		return result, pkgErrors.ErrDatabaseBackup(err)
	}

	// Create mysqldump command
	cmd := exec.CommandContext(ctx, "mysqldump", args...)

	// Set password via environment variable for security
	cmd.Env = append(os.Environ(), fmt.Sprintf("MYSQL_PWD=%s", d.config.Password))

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
		return result, pkgErrors.ErrDatabaseBackup(err).WithMetadata("command", "mysqldump")
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
func (d *MySQLDriver) StreamBackup(ctx context.Context, opts *database.BackupOptions, writer io.Writer) error {
	args, err := d.buildMySQLDumpArgs(opts)
	if err != nil {
		return err
	}

	cmd := exec.CommandContext(ctx, "mysqldump", args...)
	cmd.Env = append(os.Environ(), fmt.Sprintf("MYSQL_PWD=%s", d.config.Password))
	cmd.Stdout = writer

	return cmd.Run()
}

// GetBackupSize estimates the size of a backup
func (d *MySQLDriver) GetBackupSize(ctx context.Context, opts *database.BackupOptions) (int64, error) {
	var totalSize int64

	if opts.Database != "" {
		// Get size of specific database
		query := `SELECT SUM(data_length + index_length)
				  FROM information_schema.TABLES
				  WHERE table_schema = ?`
		if err := d.db.QueryRowContext(ctx, query, opts.Database).Scan(&totalSize); err != nil {
			return 0, err
		}
	}

	return totalSize, nil
}

// Restore restores a MySQL database from backup
func (d *MySQLDriver) Restore(ctx context.Context, opts *database.RestoreOptions) (*database.RestoreResult, error) {
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

	// Validate database name if provided
	if opts.Database != "" {
		if err := validation.ValidateDatabaseName(opts.Database); err != nil {
			result.Status = database.RestoreStatusFailed
			result.Error = err
			return result, pkgErrors.ErrDatabaseRestore(err)
		}
	}

	// Build mysql command
	args := []string{
		fmt.Sprintf("--host=%s", d.config.Host),
		fmt.Sprintf("--port=%d", d.config.Port),
		fmt.Sprintf("--user=%s", d.config.Username),
	}

	if opts.Database != "" {
		args = append(args, opts.Database)
	}

	// Create command
	cmd := exec.CommandContext(ctx, "mysql", args...)
	cmd.Env = append(os.Environ(), fmt.Sprintf("MYSQL_PWD=%s", d.config.Password))

	// Open backup file
	backupFile, err := os.Open(opts.SourceBackup)
	if err != nil {
		result.Status = database.RestoreStatusFailed
		result.Error = err
		return result, pkgErrors.ErrDatabaseRestore(err)
	}
	defer backupFile.Close()

	// Set stdin to backup file
	cmd.Stdin = backupFile

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
func (d *MySQLDriver) StreamRestore(ctx context.Context, opts *database.RestoreOptions, reader io.Reader) error {
	args := []string{
		fmt.Sprintf("--host=%s", d.config.Host),
		fmt.Sprintf("--port=%d", d.config.Port),
		fmt.Sprintf("--user=%s", d.config.Username),
	}

	if opts.Database != "" {
		args = append(args, opts.Database)
	}

	cmd := exec.CommandContext(ctx, "mysql", args...)
	cmd.Env = append(os.Environ(), fmt.Sprintf("MYSQL_PWD=%s", d.config.Password))
	cmd.Stdin = reader

	return cmd.Run()
}

// ValidateRestore validates that a restore can be performed
func (d *MySQLDriver) ValidateRestore(ctx context.Context, opts *database.RestoreOptions) error {
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
func (d *MySQLDriver) GetDatabases(ctx context.Context) ([]string, error) {
	rows, err := d.db.QueryContext(ctx, "SHOW DATABASES")
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
		// Skip system databases
		if dbName != "information_schema" && dbName != "mysql" && dbName != "performance_schema" && dbName != "sys" {
			databases = append(databases, dbName)
		}
	}

	return databases, nil
}

// GetTables returns list of tables in a database
func (d *MySQLDriver) GetTables(ctx context.Context, database string) ([]string, error) {
	query := "SHOW TABLES FROM " + database
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
func (d *MySQLDriver) GetTableSize(ctx context.Context, database, table string) (int64, error) {
	query := `SELECT data_length + index_length
			  FROM information_schema.TABLES
			  WHERE table_schema = ? AND table_name = ?`

	var size int64
	err := d.db.QueryRowContext(ctx, query, database, table).Scan(&size)
	return size, err
}

// GetVersion returns the MySQL server version
func (d *MySQLDriver) GetVersion(ctx context.Context) (string, error) {
	var version string
	err := d.db.QueryRowContext(ctx, "SELECT VERSION()").Scan(&version)
	return version, err
}

// GetType returns the database type
func (d *MySQLDriver) GetType() database.DatabaseType {
	return database.DatabaseTypeMySQL
}

// SupportsIncremental returns whether incremental backups are supported
func (d *MySQLDriver) SupportsIncremental() bool {
	return true // MySQL supports incremental backups via binary logs
}

// SupportsPITR returns whether point-in-time recovery is supported
func (d *MySQLDriver) SupportsPITR() bool {
	return true // MySQL supports PITR via binary logs
}

// buildDSN builds a MySQL DSN connection string
func (d *MySQLDriver) buildDSN(config *database.ConnectionConfig) string {
	if config.ConnectionString != "" {
		return config.ConnectionString
	}

	dsn := fmt.Sprintf("%s:%s@tcp(%s:%d)/%s?parseTime=true&timeout=%s",
		config.Username,
		config.Password,
		config.Host,
		config.Port,
		config.Database,
		config.ConnectionTimeout.String(),
	)

	if config.Options != nil {
		for k, v := range config.Options {
			dsn += fmt.Sprintf("&%s=%s", k, v)
		}
	}

	return dsn
}

// buildMySQLDumpArgs builds mysqldump command arguments
func (d *MySQLDriver) buildMySQLDumpArgs(opts *database.BackupOptions) ([]string, error) {
	args := []string{
		fmt.Sprintf("--host=%s", d.config.Host),
		fmt.Sprintf("--port=%d", d.config.Port),
		fmt.Sprintf("--user=%s", d.config.Username),
		"--single-transaction",  // Consistent snapshot
		"--routines",             // Include stored procedures
		"--triggers",             // Include triggers
		"--events",               // Include events
		"--skip-lock-tables",     // Don't lock tables
	}

	// Database selection
	if opts.AllDatabases {
		args = append(args, "--all-databases")
	} else if len(opts.Databases) > 0 {
		// Validate all database names
		for _, db := range opts.Databases {
			if err := validation.ValidateDatabaseName(db); err != nil {
				return nil, fmt.Errorf("invalid database name %q: %w", db, err)
			}
		}
		args = append(args, "--databases")
		args = append(args, opts.Databases...)
	} else if opts.Database != "" {
		// Validate single database name
		if err := validation.ValidateDatabaseName(opts.Database); err != nil {
			return nil, fmt.Errorf("invalid database name %q: %w", opts.Database, err)
		}
		args = append(args, opts.Database)
	}

	// Table selection
	if len(opts.Tables) > 0 && opts.Database != "" {
		// Validate all table names
		for _, table := range opts.Tables {
			if err := validation.ValidateTableName(table); err != nil {
				return nil, fmt.Errorf("invalid table name %q: %w", table, err)
			}
		}
		args = append(args, opts.Tables...)
	}

	// Exclude tables
	for _, table := range opts.ExcludeTables {
		if err := validation.ValidateTableName(table); err != nil {
			return nil, fmt.Errorf("invalid excluded table name %q: %w", table, err)
		}
		args = append(args, fmt.Sprintf("--ignore-table=%s.%s", opts.Database, table))
	}

	return args, nil
}

// getTableInfo retrieves information about tables
func (d *MySQLDriver) getTableInfo(ctx context.Context, dbName string) ([]database.TableInfo, error) {
	query := `SELECT table_name, table_rows, data_length, index_length
			  FROM information_schema.TABLES
			  WHERE table_schema = ?`

	rows, err := d.db.QueryContext(ctx, query, dbName)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var tables []database.TableInfo
	for rows.Next() {
		var info database.TableInfo
		if err := rows.Scan(&info.Name, &info.RowCount, &info.DataSize, &info.IndexSize); err != nil {
			return nil, err
		}
		tables = append(tables, info)
	}

	return tables, nil
}
