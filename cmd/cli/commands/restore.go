package commands

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/sanskarpan/db-backup/internal/repository"
	"github.com/sanskarpan/db-backup/internal/restore"
	"github.com/spf13/cobra"
)

// RestoreOptions holds options for the restore command
type RestoreOptions struct {
	BackupID string

	// Target database
	TargetType     string
	TargetHost     string
	TargetPort     int
	TargetUser     string
	TargetPassword string
	TargetDatabase string

	// Restore options
	PointInTime   string
	Tables        []string
	ExcludeTables []string

	// Decryption
	Decrypt       bool
	DecryptionKey string

	// Download options
	DownloadOnly bool
	DownloadPath string

	// Flags
	SkipValidation bool
	Force          bool
}

// restoreCmd represents the restore command
var restoreCmd = &cobra.Command{
	Use:   "restore [backup-id]",
	Short: "Restore from a backup",
	Long: `Restore a database from a previously created backup.

The restore command can restore full databases or specific tables,
and supports point-in-time recovery for compatible database types.

Examples:
  # Restore from backup ID
  db-backup restore backup-123-456 --target-host localhost

  # Point-in-time restore for PostgreSQL
  db-backup restore backup-123-456 \\
    --point-in-time "2025-01-01T12:00:00Z"

  # Restore specific tables
  db-backup restore backup-123-456 \\
    --tables users,orders --target-database newdb

  # Restore encrypted backup
  db-backup restore backup-123-456 \\
    --decrypt --decryption-key /path/to/key

  # Download backup without restoring
  db-backup restore backup-123-456 \\
    --download-only --download-path /tmp/backup.sql`,
	Args: cobra.ExactArgs(1),
	RunE: runRestore,
}

func init() {
	rootCmd.AddCommand(restoreCmd)

	// Target database flags
	restoreCmd.Flags().String("target-type", "", "target database type")
	restoreCmd.Flags().String("target-host", "localhost", "target host")
	restoreCmd.Flags().Int("target-port", 0, "target port")
	restoreCmd.Flags().String("target-user", "", "target user")
	restoreCmd.Flags().String("target-password", "", "target password")
	restoreCmd.Flags().String("target-database", "", "target database name")

	// Restore options
	restoreCmd.Flags().String("point-in-time", "", "restore to specific timestamp (RFC3339)")
	restoreCmd.Flags().StringSlice("tables", nil, "restore specific tables only")
	restoreCmd.Flags().StringSlice("exclude-tables", nil, "exclude tables from restore")

	// Decryption
	restoreCmd.Flags().Bool("decrypt", false, "decrypt backup")
	restoreCmd.Flags().String("decryption-key", "", "decryption key or key file")

	// Download options
	restoreCmd.Flags().Bool("download-only", false, "download backup without restoring")
	restoreCmd.Flags().String("download-path", "", "download destination path")

	// Other flags
	restoreCmd.Flags().Bool("skip-validation", false, "skip pre-restore validation")
	restoreCmd.Flags().Bool("force", false, "force restore without confirmation")
}

func runRestore(cmd *cobra.Command, args []string) error {
	opts := &RestoreOptions{
		BackupID: args[0],
	}

	// Parse flags
	opts.TargetType, _ = cmd.Flags().GetString("target-type")
	opts.TargetHost, _ = cmd.Flags().GetString("target-host")
	opts.TargetPort, _ = cmd.Flags().GetInt("target-port")
	opts.TargetUser, _ = cmd.Flags().GetString("target-user")
	opts.TargetPassword, _ = cmd.Flags().GetString("target-password")
	opts.TargetDatabase, _ = cmd.Flags().GetString("target-database")

	opts.PointInTime, _ = cmd.Flags().GetString("point-in-time")
	opts.Tables, _ = cmd.Flags().GetStringSlice("tables")
	opts.ExcludeTables, _ = cmd.Flags().GetStringSlice("exclude-tables")

	opts.Decrypt, _ = cmd.Flags().GetBool("decrypt")
	opts.DecryptionKey, _ = cmd.Flags().GetString("decryption-key")

	opts.DownloadOnly, _ = cmd.Flags().GetBool("download-only")
	opts.DownloadPath, _ = cmd.Flags().GetString("download-path")

	opts.SkipValidation, _ = cmd.Flags().GetBool("skip-validation")
	opts.Force, _ = cmd.Flags().GetBool("force")

	// Get logger and config
	log := GetLogger()
	cfg := GetConfig()

	ctx := context.Background()

	log.Info("Starting restore operation", map[string]interface{}{
		"backup_id":     opts.BackupID,
		"target_host":   opts.TargetHost,
		"download_only": opts.DownloadOnly,
	})

	// Create repository
	repo, err := repository.NewFileRepository(cfg.Backup.MetadataDirectory)
	if err != nil {
		return fmt.Errorf("failed to create repository: %w", err)
	}

	// Get backup metadata
	metadata, err := repo.Get(ctx, opts.BackupID)
	if err != nil {
		return fmt.Errorf("backup not found: %w", err)
	}

	// Display backup information
	fmt.Println("Backup Information:")
	fmt.Printf("  ID:              %s\n", metadata.ID)
	fmt.Printf("  Name:            %s\n", metadata.Name)
	fmt.Printf("  Database:        %s\n", metadata.Database)
	fmt.Printf("  Type:            %s\n", metadata.DatabaseType)
	fmt.Printf("  Size:            %s\n", formatBytes(metadata.Size))
	fmt.Printf("  Created:         %s\n", metadata.StartTime.Format("2006-01-02 15:04:05"))
	fmt.Printf("  Tables:          %d\n", len(metadata.Tables))
	fmt.Println()

	// Handle download-only mode
	if opts.DownloadOnly {
		downloadPath := opts.DownloadPath
		if downloadPath == "" {
			downloadPath = fmt.Sprintf("./%s", metadata.Name)
		}

		fmt.Printf("Downloading backup to: %s\n", downloadPath)

		// Copy backup file
		if err := copyFile(metadata.BackupPath, downloadPath); err != nil {
			return fmt.Errorf("failed to download backup: %w", err)
		}

		fmt.Println("✓ Download completed successfully!")
		return nil
	}

	// Confirmation prompt (unless --force)
	if !opts.Force {
		fmt.Println("⚠️  WARNING: This will restore data to the target database.")
		fmt.Println("   All existing data in the target database may be overwritten.")
		fmt.Print("\nDo you want to continue? (yes/no): ")

		reader := bufio.NewReader(os.Stdin)
		response, _ := reader.ReadString('\n')
		response = strings.TrimSpace(strings.ToLower(response))

		if response != "yes" && response != "y" {
			fmt.Println("Restore cancelled.")
			return nil
		}
	}

	// Create restore engine
	engineCfg := &restore.Config{
		TempDirectory: cfg.Backup.TempDirectory,
		ValidateFirst: !opts.SkipValidation,
	}
	engine := restore.NewEngine(engineCfg)

	// Parse point-in-time if provided
	var pitTime *time.Time
	if opts.PointInTime != "" {
		t, err := time.Parse(time.RFC3339, opts.PointInTime)
		if err != nil {
			return fmt.Errorf("invalid point-in-time format (use RFC3339): %w", err)
		}
		pitTime = &t
	}

	// Set defaults for target if not specified
	targetHost := opts.TargetHost
	targetPort := opts.TargetPort
	if targetPort == 0 {
		targetPort = getPort(string(metadata.DatabaseType), 0)
	}

	// Create restore options
	restoreOpts := &restore.RestoreOptions{
		BackupID:       opts.BackupID,
		TargetHost:     targetHost,
		TargetPort:     targetPort,
		TargetUsername: opts.TargetUser,
		TargetPassword: opts.TargetPassword,
		TargetDatabase: opts.TargetDatabase,
		PointInTime:    pitTime,
		Tables:         opts.Tables,
		ExcludeTables:  opts.ExcludeTables,
		Decrypt:        opts.Decrypt,
		DecryptionKey:  opts.DecryptionKey,
		SkipValidation: opts.SkipValidation,
		Force:          opts.Force,
		ProgressCallback: func(progress restore.Progress) {
			fmt.Printf("\r[%s] %.1f%% - %s", progress.Stage, progress.Percentage, progress.Message)
		},
	}

	// Perform restore
	fmt.Println("Starting restore...")
	startTime := time.Now()

	result, err := engine.RestoreBackup(ctx, metadata, restoreOpts)
	if err != nil {
		log.Error("Restore failed", err)
		return fmt.Errorf("restore failed: %w", err)
	}

	duration := time.Since(startTime)

	fmt.Println() // New line after progress
	fmt.Println("✓ Restore completed successfully!")
	fmt.Printf("\n")
	fmt.Printf("  Backup ID:       %s\n", result.BackupID)
	fmt.Printf("  Tables Restored: %d\n", len(result.RestoredTables))
	if result.RowsRestored > 0 {
		fmt.Printf("  Rows Restored:   %d\n", result.RowsRestored)
	}
	fmt.Printf("  Duration:        %s\n", duration.Round(time.Second))
	fmt.Printf("  Status:          %s\n", result.Status)

	log.Info("Restore completed", map[string]interface{}{
		"backup_id": result.BackupID,
		"duration":  duration.Seconds(),
		"status":    result.Status,
	})

	return nil
}

func copyFile(src, dst string) error {
	input, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	return os.WriteFile(dst, input, 0644)
}
