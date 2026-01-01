package commands

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/sanskarpan/db-backup/internal/backup"
	"github.com/sanskarpan/db-backup/internal/config"
	"github.com/sanskarpan/db-backup/internal/database"
	"github.com/sanskarpan/db-backup/internal/repository"
	"github.com/spf13/cobra"
)

// BackupOptions holds options for the backup command
type BackupOptions struct {
	// Database connection
	Type     string
	Host     string
	Port     int
	User     string
	Password string
	Database string

	// Multiple databases
	Databases    []string
	AllDatabases bool
	Tables       []string
	ExcludeTables []string

	// Backup options
	Compression      string
	CompressionLevel int
	Encrypt          bool
	EncryptionKey    string

	// Storage options
	Storage     string
	StoragePath string

	// Metadata
	Name string
	Tags []string

	// Flags
	Notify bool
	DryRun bool
}

// backupCmd represents the backup command
var backupCmd = &cobra.Command{
	Use:   "backup",
	Short: "Create a database backup",
	Long: `Create a backup of a database with optional compression and encryption.

The backup command supports multiple database types and can upload backups
to various storage providers including local filesystem, AWS S3, Google Cloud
Storage, and Azure Blob Storage.

Examples:
  # Basic MySQL backup
  db-backup backup --type mysql --host localhost --user root \\
    --password secret --database mydb

  # PostgreSQL backup with compression
  db-backup backup --type postgres --host localhost \\
    --database mydb --compression zstd

  # MongoDB backup with encryption
  db-backup backup --type mongodb --host localhost \\
    --database mydb --encrypt --encryption-key /path/to/key

  # Backup all MySQL databases to S3
  db-backup backup --type mysql --host localhost \\
    --all-databases --compression gzip --storage s3

  # Backup specific tables
  db-backup backup --type mysql --host localhost \\
    --database mydb --tables users,orders,products`,
	RunE: runBackup,
}

func init() {
	rootCmd.AddCommand(backupCmd)

	// Database connection flags
	backupCmd.Flags().StringP("type", "t", "", "database type (mysql|postgres|mongodb|sqlite)")
	backupCmd.Flags().StringP("host", "h", "localhost", "database host")
	backupCmd.Flags().IntP("port", "P", 0, "database port")
	backupCmd.Flags().StringP("user", "u", "", "database user")
	backupCmd.Flags().StringP("password", "p", "", "database password")
	backupCmd.Flags().StringP("database", "d", "", "database name")

	// Multiple databases
	backupCmd.Flags().StringSlice("databases", nil, "multiple databases (comma-separated)")
	backupCmd.Flags().Bool("all-databases", false, "backup all databases")
	backupCmd.Flags().StringSlice("tables", nil, "specific tables to backup")
	backupCmd.Flags().StringSlice("exclude-tables", nil, "tables to exclude from backup")

	// Compression flags
	backupCmd.Flags().String("compression", "", "compression type (gzip|zstd|lz4|none)")
	backupCmd.Flags().Int("compress-level", 0, "compression level (1-9)")

	// Encryption flags
	backupCmd.Flags().Bool("encrypt", false, "enable encryption")
	backupCmd.Flags().String("encryption-key", "", "encryption key or key file path")

	// Storage flags
	backupCmd.Flags().String("storage", "", "storage provider (s3|gcs|azure|local)")
	backupCmd.Flags().String("storage-path", "", "custom storage path")

	// Metadata flags
	backupCmd.Flags().String("name", "", "backup name (auto-generated if not provided)")
	backupCmd.Flags().StringSlice("tags", nil, "tags for backup (key=value)")

	// Other flags
	backupCmd.Flags().Bool("notify", false, "send notifications")
	backupCmd.Flags().Bool("dry-run", false, "simulate backup without execution")

	// Mark required flags
	backupCmd.MarkFlagRequired("type")
}

func runBackup(cmd *cobra.Command, args []string) error {
	// Parse flags
	opts := &BackupOptions{}

	// Database connection
	opts.Type, _ = cmd.Flags().GetString("type")
	opts.Host, _ = cmd.Flags().GetString("host")
	opts.Port, _ = cmd.Flags().GetInt("port")
	opts.User, _ = cmd.Flags().GetString("user")
	opts.Password, _ = cmd.Flags().GetString("password")
	opts.Database, _ = cmd.Flags().GetString("database")

	// Multiple databases
	opts.Databases, _ = cmd.Flags().GetStringSlice("databases")
	opts.AllDatabases, _ = cmd.Flags().GetBool("all-databases")
	opts.Tables, _ = cmd.Flags().GetStringSlice("tables")
	opts.ExcludeTables, _ = cmd.Flags().GetStringSlice("exclude-tables")

	// Compression
	opts.Compression, _ = cmd.Flags().GetString("compression")
	opts.CompressionLevel, _ = cmd.Flags().GetInt("compress-level")

	// Encryption
	opts.Encrypt, _ = cmd.Flags().GetBool("encrypt")
	opts.EncryptionKey, _ = cmd.Flags().GetString("encryption-key")

	// Storage
	opts.Storage, _ = cmd.Flags().GetString("storage")
	opts.StoragePath, _ = cmd.Flags().GetString("storage-path")

	// Metadata
	opts.Name, _ = cmd.Flags().GetString("name")
	opts.Tags, _ = cmd.Flags().GetStringSlice("tags")

	// Flags
	opts.Notify, _ = cmd.Flags().GetBool("notify")
	opts.DryRun, _ = cmd.Flags().GetBool("dry-run")

	// Validate options
	if err := validateBackupOptions(opts); err != nil {
		return err
	}

	// Get logger and config
	log := GetLogger()
	cfg := GetConfig()

	ctx := context.Background()

	log.Info("Starting backup operation", map[string]interface{}{
		"type":     opts.Type,
		"host":     opts.Host,
		"database": opts.Database,
		"dry_run":  opts.DryRun,
	})

	if opts.DryRun {
		fmt.Println("✓ Dry run mode - showing what would be backed up:")
		fmt.Printf("  Database Type: %s\n", opts.Type)
		fmt.Printf("  Host: %s:%d\n", opts.Host, getPort(opts.Type, opts.Port))
		fmt.Printf("  Database: %s\n", opts.Database)
		fmt.Printf("  Compression: %s\n", getCompression(opts.Compression, cfg))
		if opts.Encrypt {
			fmt.Printf("  Encryption: enabled\n")
		}
		log.Info("Dry run mode - no actual backup performed")
		return nil
	}

	// Create backup engine
	engineCfg := &backup.Config{
		TempDirectory:      cfg.Backup.TempDirectory,
		ParallelOperations: cfg.Backup.ParallelOperations,
		DefaultCompression: cfg.Backup.DefaultCompression,
		EnableEncryption:   opts.Encrypt,
		EncryptionKey:      opts.EncryptionKey,
	}
	engine := backup.NewEngine(engineCfg)

	// Create repository
	repo, err := repository.NewFileRepository(cfg.Backup.MetadataDirectory)
	if err != nil {
		return fmt.Errorf("failed to create repository: %w", err)
	}

	// Parse database type
	dbType, err := parseDatabaseType(opts.Type)
	if err != nil {
		return err
	}

	// Get port (use default if not specified)
	port := getPort(opts.Type, opts.Port)

	// Parse compression type
	compression := parseCompressionType(getCompression(opts.Compression, cfg))

	// Parse tags
	tags := parseTags(opts.Tags)

	// Create backup options
	backupOpts := &backup.CreateOptions{
		DatabaseType:     dbType,
		Host:             opts.Host,
		Port:             port,
		Username:         opts.User,
		Password:         opts.Password,
		Database:         opts.Database,
		Databases:        opts.Databases,
		AllDatabases:     opts.AllDatabases,
		Tables:           opts.Tables,
		ExcludeTables:    opts.ExcludeTables,
		Compression:      compression,
		CompressionLevel: opts.CompressionLevel,
		Encrypt:          opts.Encrypt,
		EncryptionKey:    opts.EncryptionKey,
		Name:             opts.Name,
		Tags:             tags,
		ProgressCallback: func(progress backup.Progress) {
			fmt.Printf("\r[%s] %.1f%% - %s", progress.Stage, progress.Percentage, progress.Message)
		},
	}

	// Create backup
	fmt.Println("Creating backup...")
	startTime := time.Now()

	metadata, err := engine.CreateBackup(ctx, backupOpts)
	if err != nil {
		log.Error("Backup failed", err)
		return fmt.Errorf("backup failed: %w", err)
	}

	// Save metadata to repository
	if err := repo.Save(ctx, metadata); err != nil {
		log.Error("Failed to save metadata", err)
		return fmt.Errorf("failed to save metadata: %w", err)
	}

	duration := time.Since(startTime)

	fmt.Println() // New line after progress
	fmt.Println("✓ Backup completed successfully!")
	fmt.Printf("\n")
	fmt.Printf("  Backup ID:       %s\n", metadata.ID)
	fmt.Printf("  Name:            %s\n", metadata.Name)
	fmt.Printf("  Database:        %s\n", metadata.Database)
	fmt.Printf("  Size:            %s\n", formatBytes(metadata.Size))
	if metadata.CompressedSize > 0 && metadata.CompressedSize != metadata.Size {
		ratio := float64(metadata.Size-metadata.CompressedSize) / float64(metadata.Size) * 100
		fmt.Printf("  Compressed Size: %s (%.1f%% reduction)\n", formatBytes(metadata.CompressedSize), ratio)
	}
	fmt.Printf("  Tables:          %d\n", len(metadata.Tables))
	fmt.Printf("  Duration:        %s\n", duration.Round(time.Second))
	fmt.Printf("  Location:        %s\n", metadata.BackupPath)
	fmt.Printf("  Checksum:        %s\n", metadata.Checksum[:16]+"...")

	log.Info("Backup completed", map[string]interface{}{
		"backup_id": metadata.ID,
		"size":      metadata.Size,
		"duration":  duration.Seconds(),
	})

	return nil
}

func validateBackupOptions(opts *BackupOptions) error {
	// Validate database type
	validTypes := map[string]bool{
		"mysql":    true,
		"postgres": true,
		"mongodb":  true,
		"sqlite":   true,
	}
	if !validTypes[opts.Type] {
		return fmt.Errorf("invalid database type: %s (must be mysql|postgres|mongodb|sqlite)", opts.Type)
	}

	// For SQLite, database is a file path
	if opts.Type == "sqlite" {
		if opts.Database == "" {
			return fmt.Errorf("database file path is required for SQLite")
		}
		return nil
	}

	// Validate database connection options
	if !opts.AllDatabases && opts.Database == "" && len(opts.Databases) == 0 {
		return fmt.Errorf("database name is required (or use --all-databases)")
	}

	// Validate encryption options
	if opts.Encrypt && opts.EncryptionKey == "" {
		return fmt.Errorf("encryption key is required when encryption is enabled")
	}

	// Validate compression type
	if opts.Compression != "" {
		validCompression := map[string]bool{
			"gzip": true,
			"zstd": true,
			"lz4":  true,
			"none": true,
		}
		if !validCompression[opts.Compression] {
			return fmt.Errorf("invalid compression type: %s (must be gzip|zstd|lz4|none)", opts.Compression)
		}
	}

	// Validate compression level
	if opts.CompressionLevel != 0 && (opts.CompressionLevel < 1 || opts.CompressionLevel > 9) {
		return fmt.Errorf("compression level must be between 1 and 9")
	}

	return nil
}

// Helper functions

func parseDatabaseType(typeStr string) (database.DatabaseType, error) {
	switch strings.ToLower(typeStr) {
	case "mysql":
		return database.DatabaseTypeMySQL, nil
	case "postgres", "postgresql":
		return database.DatabaseTypePostgreSQL, nil
	case "mongodb", "mongo":
		return database.DatabaseTypeMongoDB, nil
	case "sqlite":
		return database.DatabaseTypeSQLite, nil
	default:
		return "", fmt.Errorf("unsupported database type: %s", typeStr)
	}
}

func parseCompressionType(typeStr string) database.CompressionType {
	switch strings.ToLower(typeStr) {
	case "gzip", "gz":
		return database.CompressionGzip
	case "zstd":
		return database.CompressionZstd
	case "lz4":
		return database.CompressionLZ4
	case "none", "":
		return database.CompressionNone
	default:
		return database.CompressionNone
	}
}

func getPort(dbType string, port int) int {
	if port != 0 {
		return port
	}

	// Return default ports
	switch strings.ToLower(dbType) {
	case "mysql":
		return 3306
	case "postgres", "postgresql":
		return 5432
	case "mongodb", "mongo":
		return 27017
	default:
		return 0
	}
}

func getCompression(compression string, cfg *config.Config) string {
	if compression != "" {
		return compression
	}
	if cfg.Backup.DefaultCompression != "" {
		return cfg.Backup.DefaultCompression
	}
	return "gzip"
}

func parseTags(tagStrings []string) map[string]string {
	tags := make(map[string]string)
	for _, tag := range tagStrings {
		parts := strings.SplitN(tag, "=", 2)
		if len(parts) == 2 {
			tags[parts[0]] = parts[1]
		}
	}
	return tags
}

func formatBytes(bytes int64) string {
	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%d B", bytes)
	}
	div, exp := int64(unit), 0
	for n := bytes / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(bytes)/float64(div), "KMGTPE"[exp])
}
