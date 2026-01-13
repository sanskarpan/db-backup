// Package commands implements all CLI commands
package commands

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/sanskarpan/db-backup/internal/config"
	"github.com/sanskarpan/db-backup/internal/logger"
)

var (
	cfgFile string
	cfg     *config.Config
	log     *logger.Logger
	verbose bool
)

// rootCmd represents the base command when called without any subcommands
var rootCmd = &cobra.Command{
	Use:   "db-backup",
	Short: "Enterprise-grade database backup and restore utility",
	Long: `DB Backup Utility is a comprehensive backup and restore solution for databases.

It supports multiple database types (MySQL, PostgreSQL, MongoDB, SQLite),
cloud storage providers (AWS S3, GCS, Azure), compression, encryption,
scheduling, and monitoring capabilities.

Examples:
  # Backup a MySQL database
  db-backup backup --type mysql --host localhost --user root --database mydb

  # List all backups
  db-backup list

  # Restore from a backup
  db-backup restore backup-123-456 --target-host localhost`,
	Version: GetVersion(),
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		// Initialize configuration
		var err error
		cfg, err = config.Load(cfgFile)
		if err != nil {
			return fmt.Errorf("failed to load configuration: %w", err)
		}

		// Override log level if verbose flag is set
		if verbose {
			cfg.Logging.Level = "debug"
		}

		// Initialize logger
		log = logger.New(cfg.Logging)
		log.Debug("Configuration loaded successfully", map[string]interface{}{
			"config_file": cfgFile,
			"verbose":     verbose,
		})

		return nil
	},
}

// Execute adds all child commands to the root command and sets flags appropriately.
func Execute() {
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

func init() {
	cobra.OnInitialize(initConfig)

	// Global flags
	rootCmd.PersistentFlags().StringVarP(&cfgFile, "config", "c", "", "config file path (default: ./config.yaml)")
	rootCmd.PersistentFlags().BoolVarP(&verbose, "verbose", "v", false, "verbose output (debug logging)")
	rootCmd.PersistentFlags().String("log-level", "", "log level (debug|info|warn|error)")
	rootCmd.PersistentFlags().String("log-file", "", "log file path")

	// Bind flags to viper
	viper.BindPFlag("logging.level", rootCmd.PersistentFlags().Lookup("log-level"))
	viper.BindPFlag("logging.file.path", rootCmd.PersistentFlags().Lookup("log-file"))
}

// initConfig reads in config file and ENV variables if set
func initConfig() {
	if cfgFile != "" {
		viper.SetConfigFile(cfgFile)
	}
}

// GetConfig returns the loaded configuration
func GetConfig() *config.Config {
	return cfg
}

// GetLogger returns the logger instance
func GetLogger() *logger.Logger {
	return log
}

// GetVersion returns version information
func GetVersion() string {
	// These will be set during build using ldflags
	var (
		Version   = "dev"
		BuildTime = "unknown"
		GitCommit = "unknown"
	)
	return fmt.Sprintf("%s (built: %s, commit: %s)", Version, BuildTime, GitCommit)
}
