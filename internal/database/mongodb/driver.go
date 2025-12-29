// Package mongodb provides MongoDB database driver implementation
package mongodb

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"github.com/sanskarpan/db-backup/internal/database"
	pkgErrors "github.com/sanskarpan/db-backup/pkg/errors"
	"github.com/sanskarpan/db-backup/pkg/utils"
	"github.com/sanskarpan/db-backup/pkg/validation"
)

// MongoDBDriver implements the database.Driver interface for MongoDB
type MongoDBDriver struct {
	client *mongo.Client
	config *database.ConnectionConfig
}

func init() {
	database.RegisterDriver(database.DatabaseTypeMongoDB, func() database.Driver {
		return NewMongoDBDriver()
	})
}

// NewMongoDBDriver creates a new MongoDB driver instance
func NewMongoDBDriver() *MongoDBDriver {
	return &MongoDBDriver{}
}

// Connect establishes a connection to the MongoDB database
func (d *MongoDBDriver) Connect(ctx context.Context, config *database.ConnectionConfig) error {
	// Build connection string
	connectionString := d.buildConnectionString(config)

	// Set client options
	clientOpts := options.Client().
		ApplyURI(connectionString).
		SetMaxPoolSize(uint64(config.MaxConnections))

	// Connect to MongoDB
	client, err := mongo.Connect(ctx, clientOpts)
	if err != nil {
		return pkgErrors.ErrDatabaseConnection(err)
	}

	// Test connection
	if err := client.Ping(ctx, nil); err != nil {
		client.Disconnect(ctx)
		return pkgErrors.ErrDatabaseConnection(err)
	}

	d.client = client
	d.config = config
	return nil
}

// Disconnect closes the database connection
func (d *MongoDBDriver) Disconnect() error {
	if d.client != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		return d.client.Disconnect(ctx)
	}
	return nil
}

// Ping tests the database connection
func (d *MongoDBDriver) Ping(ctx context.Context) error {
	if d.client == nil {
		return pkgErrors.New(pkgErrors.ErrorTypeDatabase, "not connected to database")
	}
	return d.client.Ping(ctx, nil)
}

// Backup creates a backup of the MongoDB database
func (d *MongoDBDriver) Backup(ctx context.Context, opts *database.BackupOptions) (*database.BackupResult, error) {
	result := &database.BackupResult{
		ID:        utils.GenerateBackupID(),
		StartTime: time.Now(),
		Metadata:  opts.Metadata,
		Status:    database.BackupStatusInProgress,
	}

	// Build mongodump command
	args, err := d.buildMongoDumpArgs(opts)
	if err != nil {
		result.Status = database.BackupStatusFailed
		result.Error = err
		return result, pkgErrors.ErrDatabaseBackup(err)
	}

	// Create mongodump command
	cmd := exec.CommandContext(ctx, "mongodump", args...)

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
		return result, pkgErrors.ErrDatabaseBackup(err).WithMetadata("command", "mongodump")
	}

	// Read stderr
	stderrOutput, _ := io.ReadAll(stderrPipe)

	// Wait for command to complete
	if err := cmd.Wait(); err != nil {
		result.Status = database.BackupStatusFailed
		result.Error = err
		return result, pkgErrors.ErrDatabaseBackup(err).WithMetadata("stderr", string(stderrOutput))
	}

	// Calculate backup size
	totalSize, err := dirSize(opts.OutputPath)
	if err != nil {
		result.Status = database.BackupStatusFailed
		result.Error = err
		return result, err
	}

	// Get database version
	version, _ := d.GetVersion(ctx)

	// Complete result
	result.EndTime = time.Now()
	result.Duration = result.EndTime.Sub(result.StartTime)
	result.Size = totalSize
	result.DatabaseVersion = version
	result.Status = database.BackupStatusSuccess

	return result, nil
}

// StreamBackup streams a backup to the provided writer
func (d *MongoDBDriver) StreamBackup(ctx context.Context, opts *database.BackupOptions, writer io.Writer) error {
	// MongoDB's mongodump doesn't support direct streaming to stdout
	// We need to dump to a temp directory and then tar it
	return pkgErrors.New(pkgErrors.ErrorTypeDatabase, "MongoDB streaming backup not implemented - use file-based backup")
}

// GetBackupSize estimates the size of a backup
func (d *MongoDBDriver) GetBackupSize(ctx context.Context, opts *database.BackupOptions) (int64, error) {
	var totalSize int64

	if opts.Database != "" {
		// Get database stats
		var result struct {
			DataSize int64 `bson:"dataSize"`
		}

		db := d.client.Database(opts.Database)
		err := db.RunCommand(ctx, map[string]interface{}{"dbStats": 1}).Decode(&result)
		if err != nil {
			return 0, err
		}

		totalSize = result.DataSize
	}

	return totalSize, nil
}

// Restore restores a MongoDB database from backup
func (d *MongoDBDriver) Restore(ctx context.Context, opts *database.RestoreOptions) (*database.RestoreResult, error) {
	result := &database.RestoreResult{
		StartTime: time.Now(),
		Status:    database.RestoreStatusInProgress,
	}

	// Validate backup directory exists
	if _, err := os.Stat(opts.SourceBackup); os.IsNotExist(err) {
		result.Status = database.RestoreStatusFailed
		result.Error = err
		return result, pkgErrors.ErrDatabaseRestore(err).WithMetadata("backup_path", opts.SourceBackup)
	}

	// Build mongorestore command
	args, err := d.buildMongoRestoreArgs(opts)
	if err != nil {
		result.Status = database.RestoreStatusFailed
		result.Error = err
		return result, pkgErrors.ErrDatabaseRestore(err)
	}

	// Create command
	cmd := exec.CommandContext(ctx, "mongorestore", args...)

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
func (d *MongoDBDriver) StreamRestore(ctx context.Context, opts *database.RestoreOptions, reader io.Reader) error {
	// MongoDB's mongorestore doesn't support streaming from stdin
	return pkgErrors.New(pkgErrors.ErrorTypeDatabase, "MongoDB streaming restore not implemented - use file-based restore")
}

// ValidateRestore validates that a restore can be performed
func (d *MongoDBDriver) ValidateRestore(ctx context.Context, opts *database.RestoreOptions) error {
	// Check if backup directory exists
	if _, err := os.Stat(opts.SourceBackup); os.IsNotExist(err) {
		return pkgErrors.ErrValidationFailed(fmt.Sprintf("backup directory not found: %s", opts.SourceBackup))
	}

	// Check database connection
	if err := d.Ping(ctx); err != nil {
		return pkgErrors.ErrValidationFailed("database connection failed")
	}

	return nil
}

// GetDatabases returns list of databases
func (d *MongoDBDriver) GetDatabases(ctx context.Context) ([]string, error) {
	databases, err := d.client.ListDatabaseNames(ctx, map[string]interface{}{})
	if err != nil {
		return nil, err
	}

	// Filter out system databases
	var userDatabases []string
	for _, db := range databases {
		if db != "admin" && db != "local" && db != "config" {
			userDatabases = append(userDatabases, db)
		}
	}

	return userDatabases, nil
}

// GetTables returns list of collections in a database
func (d *MongoDBDriver) GetTables(ctx context.Context, database string) ([]string, error) {
	db := d.client.Database(database)
	collections, err := db.ListCollectionNames(ctx, map[string]interface{}{})
	if err != nil {
		return nil, err
	}

	return collections, nil
}

// GetTableSize returns the size of a collection
func (d *MongoDBDriver) GetTableSize(ctx context.Context, database, table string) (int64, error) {
	var result struct {
		Size int64 `bson:"size"`
	}

	db := d.client.Database(database)
	err := db.RunCommand(ctx, map[string]interface{}{"collStats": table}).Decode(&result)
	if err != nil {
		return 0, err
	}

	return result.Size, nil
}

// GetVersion returns the MongoDB server version
func (d *MongoDBDriver) GetVersion(ctx context.Context) (string, error) {
	var result struct {
		Version string `bson:"version"`
	}

	err := d.client.Database("admin").RunCommand(ctx, map[string]interface{}{"buildInfo": 1}).Decode(&result)
	if err != nil {
		return "", err
	}

	return result.Version, nil
}

// GetType returns the database type
func (d *MongoDBDriver) GetType() database.DatabaseType {
	return database.DatabaseTypeMongoDB
}

// SupportsIncremental returns whether incremental backups are supported
func (d *MongoDBDriver) SupportsIncremental() bool {
	return true // MongoDB supports incremental backups via oplog
}

// SupportsPITR returns whether point-in-time recovery is supported
func (d *MongoDBDriver) SupportsPITR() bool {
	return true // MongoDB supports PITR via oplog replay
}

// buildConnectionString builds a MongoDB connection string
func (d *MongoDBDriver) buildConnectionString(config *database.ConnectionConfig) string {
	if config.ConnectionString != "" {
		return config.ConnectionString
	}

	// mongodb://[username:password@]host[:port][/database]
	auth := ""
	if config.Username != "" && config.Password != "" {
		auth = fmt.Sprintf("%s:%s@", config.Username, config.Password)
	}

	database := ""
	if config.Database != "" {
		database = "/" + config.Database
	}

	return fmt.Sprintf("mongodb://%s%s:%d%s",
		auth,
		config.Host,
		config.Port,
		database,
	)
}

// buildMongoDumpArgs builds mongodump command arguments
func (d *MongoDBDriver) buildMongoDumpArgs(opts *database.BackupOptions) ([]string, error) {
	args := []string{
		"--host", d.config.Host,
		"--port", fmt.Sprintf("%d", d.config.Port),
		"--out", opts.OutputPath,
		"--gzip", // Always use gzip compression
	}

	if d.config.Username != "" {
		args = append(args, "--username", d.config.Username)
	}

	if d.config.Password != "" {
		args = append(args, "--password", d.config.Password)
	}

	// Validate and add database name
	if opts.Database != "" {
		if err := validation.ValidateDatabaseName(opts.Database); err != nil {
			return nil, fmt.Errorf("invalid database name %q: %w", opts.Database, err)
		}
		args = append(args, "--db", opts.Database)
	}

	// Collection selection (MongoDB calls them collections, but we use Tables for consistency)
	if len(opts.Tables) > 0 {
		for _, collection := range opts.Tables {
			// Validate collection name (use table name validation)
			if err := validation.ValidateTableName(collection); err != nil {
				return nil, fmt.Errorf("invalid collection name %q: %w", collection, err)
			}
			args = append(args, "--collection", collection)
		}
	}

	// Add oplog for point-in-time consistency
	if opts.ConsistentBackup {
		args = append(args, "--oplog")
	}

	if opts.Parallel > 1 {
		args = append(args, "--numParallelCollections", fmt.Sprintf("%d", opts.Parallel))
	}

	return args, nil
}

// buildMongoRestoreArgs builds mongorestore command arguments
func (d *MongoDBDriver) buildMongoRestoreArgs(opts *database.RestoreOptions) ([]string, error) {
	args := []string{
		"--host", d.config.Host,
		"--port", fmt.Sprintf("%d", d.config.Port),
		"--gzip",
	}

	if d.config.Username != "" {
		args = append(args, "--username", d.config.Username)
	}

	if d.config.Password != "" {
		args = append(args, "--password", d.config.Password)
	}

	// Validate and add database name
	if opts.Database != "" {
		if err := validation.ValidateDatabaseName(opts.Database); err != nil {
			return nil, fmt.Errorf("invalid database name %q: %w", opts.Database, err)
		}
		args = append(args, "--db", opts.Database)
	}

	if opts.DropExisting {
		args = append(args, "--drop")
	}

	if opts.Parallel > 1 {
		args = append(args, "--numParallelCollections", fmt.Sprintf("%d", opts.Parallel))
	}

	// Restore from oplog for PITR
	if opts.PointInTime != nil {
		args = append(args,
			"--oplogReplay",
			"--oplogLimit", fmt.Sprintf("%d:%d",
				opts.PointInTime.Unix(),
				0,
			),
		)
	}

	args = append(args, opts.SourceBackup)

	return args, nil
}

// dirSize calculates the total size of a directory
func dirSize(path string) (int64, error) {
	var size int64

	err := filepath.Walk(path, func(_ string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() {
			size += info.Size()
		}
		return nil
	})

	return size, err
}
