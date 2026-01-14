package commands

import (
	"encoding/json"
	"fmt"
	"os"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"
	"github.com/sanskarpan/db-backup/internal/backup"
	"github.com/sanskarpan/db-backup/internal/database"
	"github.com/sanskarpan/db-backup/internal/scheduler"
)

var scheduleCmd = &cobra.Command{
	Use:   "schedule",
	Short: "Manage backup schedules",
	Long:  `Create, list, and manage scheduled backup jobs`,
}

var scheduleAddCmd = &cobra.Command{
	Use:   "add",
	Short: "Add a new backup schedule",
	Long:  `Add a new scheduled backup job with cron expression`,
	Example: `  db-backup schedule add --id daily-mysql --name "Daily MySQL Backup" \
    --cron "0 2 * * *" --type mysql --host localhost --database mydb

  db-backup schedule add --id hourly-postgres --name "Hourly PostgreSQL" \
    --cron "0 * * * *" --type postgres --host localhost --all-databases`,
	RunE: runScheduleAdd,
}

var scheduleListCmd = &cobra.Command{
	Use:   "list",
	Short: "List all backup schedules",
	Long:  `List all scheduled backup jobs`,
	RunE:  runScheduleList,
}

var scheduleRemoveCmd = &cobra.Command{
	Use:   "remove [schedule-id]",
	Short: "Remove a backup schedule",
	Long:  `Remove a scheduled backup job by ID`,
	Args:  cobra.ExactArgs(1),
	RunE:  runScheduleRemove,
}

var scheduleEnableCmd = &cobra.Command{
	Use:   "enable [schedule-id]",
	Short: "Enable a backup schedule",
	Long:  `Enable a scheduled backup job`,
	Args:  cobra.ExactArgs(1),
	RunE:  runScheduleEnable,
}

var scheduleDisableCmd = &cobra.Command{
	Use:   "disable [schedule-id]",
	Short: "Disable a backup schedule",
	Long:  `Disable a scheduled backup job`,
	Args:  cobra.ExactArgs(1),
	RunE:  runScheduleDisable,
}

var scheduleRunCmd = &cobra.Command{
	Use:   "run [schedule-id]",
	Short: "Run a schedule immediately",
	Long:  `Execute a scheduled backup job immediately (outside its normal schedule)`,
	Args:  cobra.ExactArgs(1),
	RunE:  runScheduleRun,
}

type ScheduleAddOptions struct {
	ID              string
	Name            string
	Cron            string
	Type            string
	Host            string
	Port            int
	User            string
	Database        string
	Databases       []string
	AllDatabases    bool
	Compression     string
	Encrypt         bool
	Storage         string
	Retries         int
	RetryDelay      time.Duration
	Timeout         time.Duration
	Tags            map[string]string
}

type ScheduleListOptions struct {
	Format string
}

func init() {
	rootCmd.AddCommand(scheduleCmd)
	scheduleCmd.AddCommand(scheduleAddCmd)
	scheduleCmd.AddCommand(scheduleListCmd)
	scheduleCmd.AddCommand(scheduleRemoveCmd)
	scheduleCmd.AddCommand(scheduleEnableCmd)
	scheduleCmd.AddCommand(scheduleDisableCmd)
	scheduleCmd.AddCommand(scheduleRunCmd)

	// Add flags
	scheduleAddCmd.Flags().String("id", "", "Schedule ID (required)")
	scheduleAddCmd.Flags().String("name", "", "Schedule name")
	scheduleAddCmd.Flags().String("cron", "", "Cron expression (required)")
	scheduleAddCmd.Flags().String("type", "", "Database type (mysql, postgres, mongodb, sqlite)")
	scheduleAddCmd.Flags().String("host", "localhost", "Database host")
	scheduleAddCmd.Flags().Int("port", 0, "Database port")
	scheduleAddCmd.Flags().String("user", "", "Database user")
	scheduleAddCmd.Flags().String("database", "", "Database name")
	scheduleAddCmd.Flags().StringSlice("databases", nil, "Multiple databases")
	scheduleAddCmd.Flags().Bool("all-databases", false, "Backup all databases")
	scheduleAddCmd.Flags().String("compression", "zstd", "Compression type")
	scheduleAddCmd.Flags().Bool("encrypt", false, "Encrypt backup")
	scheduleAddCmd.Flags().String("storage", "local", "Storage provider")
	scheduleAddCmd.Flags().Int("retries", 3, "Number of retries on failure")
	scheduleAddCmd.Flags().Duration("retry-delay", 5*time.Minute, "Delay between retries")
	scheduleAddCmd.Flags().Duration("timeout", 24*time.Hour, "Job timeout")

	scheduleAddCmd.MarkFlagRequired("id")
	scheduleAddCmd.MarkFlagRequired("cron")
	scheduleAddCmd.MarkFlagRequired("type")

	scheduleListCmd.Flags().String("format", "table", "Output format (table, json)")
}

func runScheduleAdd(cmd *cobra.Command, args []string) error {
	// Parse options
	opts := &ScheduleAddOptions{
		ID:           cmd.Flag("id").Value.String(),
		Name:         cmd.Flag("name").Value.String(),
		Cron:         cmd.Flag("cron").Value.String(),
		Type:         cmd.Flag("type").Value.String(),
		Host:         cmd.Flag("host").Value.String(),
		User:         cmd.Flag("user").Value.String(),
		Database:     cmd.Flag("database").Value.String(),
		Compression:  cmd.Flag("compression").Value.String(),
		Storage:      cmd.Flag("storage").Value.String(),
	}

	port, _ := cmd.Flags().GetInt("port")
	opts.Port = port

	databases, _ := cmd.Flags().GetStringSlice("databases")
	opts.Databases = databases

	allDbs, _ := cmd.Flags().GetBool("all-databases")
	opts.AllDatabases = allDbs

	encrypt, _ := cmd.Flags().GetBool("encrypt")
	opts.Encrypt = encrypt

	retries, _ := cmd.Flags().GetInt("retries")
	opts.Retries = retries

	retryDelay, _ := cmd.Flags().GetDuration("retry-delay")
	opts.RetryDelay = retryDelay

	timeout, _ := cmd.Flags().GetDuration("timeout")
	opts.Timeout = timeout

	// Set default name if not provided
	if opts.Name == "" {
		opts.Name = opts.ID
	}

	// Create backup options
	dbType, err := parseDatabaseType(opts.Type)
	if err != nil {
		return err
	}

	backupOpts := &backup.CreateOptions{
		DatabaseType: dbType,
		Host:         opts.Host,
		Port:         opts.Port,
		Username:     opts.User,
		Database:     opts.Database,
		Databases:    opts.Databases,
		AllDatabases: opts.AllDatabases,
		Compression:  database.CompressionType(opts.Compression),
		Encrypt:      opts.Encrypt,
	}

	// Get password from environment
	backupOpts.Password = os.Getenv("DB_PASSWORD")

	// Create scheduled job
	job := &scheduler.ScheduledJob{
		ID:          opts.ID,
		Name:        opts.Name,
		Schedule:    opts.Cron,
		BackupOpts:  backupOpts,
		Enabled:     true,
		Retries:     opts.Retries,
		RetryDelay:  opts.RetryDelay,
		Timeout:     opts.Timeout,
		Tags:        opts.Tags,
	}

	// Initialize scheduler
	cfg := GetConfig()
	log := GetLogger()
	engine := backup.NewEngine(&backup.Config{
		TempDirectory:      cfg.Backup.TempDirectory,
		ParallelOperations: cfg.Backup.ParallelOperations,
		DefaultCompression: cfg.Backup.DefaultCompression,
		EnableEncryption:   cfg.Backup.Encryption.Enabled,
		EncryptionKey:      "",
	})

	sched, err := scheduler.NewScheduler(engine, log, nil)
	if err != nil {
		return fmt.Errorf("failed to create scheduler: %w", err)
	}

	// Add job
	if err := sched.AddJob(job); err != nil {
		return fmt.Errorf("failed to add schedule: %w", err)
	}

	fmt.Printf("✓ Schedule '%s' added successfully\n", opts.ID)
	fmt.Printf("  Next run: %s\n", job.NextRun.Format(time.RFC3339))

	return nil
}

func runScheduleList(cmd *cobra.Command, args []string) error {
	format, _ := cmd.Flags().GetString("format")

	// Initialize scheduler
	cfg := GetConfig()
	log := GetLogger()
	engine := backup.NewEngine(&backup.Config{
		TempDirectory:      cfg.Backup.TempDirectory,
		ParallelOperations: cfg.Backup.ParallelOperations,
		DefaultCompression: cfg.Backup.DefaultCompression,
		EnableEncryption:   cfg.Backup.Encryption.Enabled,
		EncryptionKey:      "",
	})

	sched, err := scheduler.NewScheduler(engine, log, nil)
	if err != nil {
		return fmt.Errorf("failed to create scheduler: %w", err)
	}

	jobs := sched.ListJobs()

	if format == "json" {
		data, err := json.MarshalIndent(jobs, "", "  ")
		if err != nil {
			return fmt.Errorf("failed to marshal jobs: %w", err)
		}
		fmt.Println(string(data))
		return nil
	}

	// Table format
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "ID\tNAME\tSCHEDULE\tENABLED\tLAST RUN\tNEXT RUN\tRUN COUNT\tFAIL COUNT")
	fmt.Fprintln(w, "---\t----\t--------\t-------\t--------\t--------\t---------\t----------")

	for _, job := range jobs {
		enabled := "No"
		if job.Enabled {
			enabled = "Yes"
		}

		lastRun := "Never"
		if !job.LastRun.IsZero() {
			lastRun = job.LastRun.Format("2006-01-02 15:04")
		}

		nextRun := "N/A"
		if job.Enabled && !job.NextRun.IsZero() {
			nextRun = job.NextRun.Format("2006-01-02 15:04")
		}

		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\t%d\t%d\n",
			job.ID, job.Name, job.Schedule, enabled, lastRun, nextRun, job.RunCount, job.FailCount)
	}

	w.Flush()
	return nil
}

func runScheduleRemove(cmd *cobra.Command, args []string) error {
	scheduleID := args[0]

	// Initialize scheduler
	cfg := GetConfig()
	log := GetLogger()
	engine := backup.NewEngine(&backup.Config{
		TempDirectory:      cfg.Backup.TempDirectory,
		ParallelOperations: cfg.Backup.ParallelOperations,
		DefaultCompression: cfg.Backup.DefaultCompression,
		EnableEncryption:   cfg.Backup.Encryption.Enabled,
		EncryptionKey:      "",
	})

	sched, err := scheduler.NewScheduler(engine, log, nil)
	if err != nil {
		return fmt.Errorf("failed to create scheduler: %w", err)
	}

	if err := sched.RemoveJob(scheduleID); err != nil {
		return fmt.Errorf("failed to remove schedule: %w", err)
	}

	fmt.Printf("✓ Schedule '%s' removed successfully\n", scheduleID)
	return nil
}

func runScheduleEnable(cmd *cobra.Command, args []string) error {
	scheduleID := args[0]

	// Initialize scheduler
	cfg := GetConfig()
	log := GetLogger()
	engine := backup.NewEngine(&backup.Config{
		TempDirectory:      cfg.Backup.TempDirectory,
		ParallelOperations: cfg.Backup.ParallelOperations,
		DefaultCompression: cfg.Backup.DefaultCompression,
		EnableEncryption:   cfg.Backup.Encryption.Enabled,
		EncryptionKey:      "",
	})

	sched, err := scheduler.NewScheduler(engine, log, nil)
	if err != nil {
		return fmt.Errorf("failed to create scheduler: %w", err)
	}

	if err := sched.EnableJob(scheduleID); err != nil {
		return fmt.Errorf("failed to enable schedule: %w", err)
	}

	fmt.Printf("✓ Schedule '%s' enabled successfully\n", scheduleID)
	return nil
}

func runScheduleDisable(cmd *cobra.Command, args []string) error {
	scheduleID := args[0]

	// Initialize scheduler
	cfg := GetConfig()
	log := GetLogger()
	engine := backup.NewEngine(&backup.Config{
		TempDirectory:      cfg.Backup.TempDirectory,
		ParallelOperations: cfg.Backup.ParallelOperations,
		DefaultCompression: cfg.Backup.DefaultCompression,
		EnableEncryption:   cfg.Backup.Encryption.Enabled,
		EncryptionKey:      "",
	})

	sched, err := scheduler.NewScheduler(engine, log, nil)
	if err != nil {
		return fmt.Errorf("failed to create scheduler: %w", err)
	}

	if err := sched.DisableJob(scheduleID); err != nil {
		return fmt.Errorf("failed to disable schedule: %w", err)
	}

	fmt.Printf("✓ Schedule '%s' disabled successfully\n", scheduleID)
	return nil
}

func runScheduleRun(cmd *cobra.Command, args []string) error {
	scheduleID := args[0]

	// Initialize scheduler
	cfg := GetConfig()
	log := GetLogger()
	engine := backup.NewEngine(&backup.Config{
		TempDirectory:      cfg.Backup.TempDirectory,
		ParallelOperations: cfg.Backup.ParallelOperations,
		DefaultCompression: cfg.Backup.DefaultCompression,
		EnableEncryption:   cfg.Backup.Encryption.Enabled,
		EncryptionKey:      "",
	})

	sched, err := scheduler.NewScheduler(engine, log, nil)
	if err != nil {
		return fmt.Errorf("failed to create scheduler: %w", err)
	}

	fmt.Printf("Running schedule '%s'...\n", scheduleID)

	result, err := sched.RunJobNow(scheduleID)
	if err != nil {
		return fmt.Errorf("failed to run schedule: %w", err)
	}

	if result.Success {
		fmt.Printf("✓ Backup completed successfully\n")
		fmt.Printf("  Backup ID: %s\n", result.BackupID)
		fmt.Printf("  Duration: %v\n", result.Duration)
	} else {
		fmt.Printf("✗ Backup failed: %v\n", result.Error)
		fmt.Printf("  Duration: %v\n", result.Duration)
		fmt.Printf("  Retries: %d\n", result.RetryCount)
	}

	return nil
}

