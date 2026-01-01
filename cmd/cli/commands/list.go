package commands

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/sanskarpan/db-backup/internal/models"
	"github.com/sanskarpan/db-backup/internal/repository"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

// ListOptions holds options for the list command
type ListOptions struct {
	Database string
	Type     string
	Storage  string
	From     string
	To       string
	Tags     []string
	Format   string
	Limit    int
	Sort     string
	Order    string
}

// listCmd represents the list command
var listCmd = &cobra.Command{
	Use:   "list",
	Short: "List available backups",
	Long: `List all available backups with optional filtering and sorting.

The list command displays backup metadata including size, creation time,
database type, and storage location. Results can be filtered by various
criteria and formatted as table, JSON, or YAML.

Examples:
  # List all backups
  db-backup list

  # List MySQL backups from last week
  db-backup list --type mysql \\
    --from "2024-12-20T00:00:00Z"

  # List backups with specific tags
  db-backup list --tags "env=production,app=api"

  # List in JSON format
  db-backup list --format json

  # List and sort by size
  db-backup list --sort size --order desc --limit 10`,
	RunE: runList,
}

func init() {
	rootCmd.AddCommand(listCmd)

	// Filter flags
	listCmd.Flags().String("database", "", "filter by database name")
	listCmd.Flags().String("type", "", "filter by database type")
	listCmd.Flags().String("storage", "", "filter by storage provider")
	listCmd.Flags().String("from", "", "start date (RFC3339)")
	listCmd.Flags().String("to", "", "end date (RFC3339)")
	listCmd.Flags().StringSlice("tags", nil, "filter by tags")

	// Output flags
	listCmd.Flags().String("format", "table", "output format (table|json|yaml)")
	listCmd.Flags().Int("limit", 50, "limit results")
	listCmd.Flags().String("sort", "date", "sort by (date|size|name)")
	listCmd.Flags().String("order", "desc", "sort order (asc|desc)")
}

func runList(cmd *cobra.Command, args []string) error {
	opts := &ListOptions{}

	// Parse flags
	opts.Database, _ = cmd.Flags().GetString("database")
	opts.Type, _ = cmd.Flags().GetString("type")
	opts.Storage, _ = cmd.Flags().GetString("storage")
	opts.From, _ = cmd.Flags().GetString("from")
	opts.To, _ = cmd.Flags().GetString("to")
	opts.Tags, _ = cmd.Flags().GetStringSlice("tags")
	opts.Format, _ = cmd.Flags().GetString("format")
	opts.Limit, _ = cmd.Flags().GetInt("limit")
	opts.Sort, _ = cmd.Flags().GetString("sort")
	opts.Order, _ = cmd.Flags().GetString("order")

	// Get logger and config
	log := GetLogger()
	cfg := GetConfig()

	ctx := context.Background()

	log.Info("Listing backups", map[string]interface{}{
		"database": opts.Database,
		"type":     opts.Type,
		"format":   opts.Format,
		"limit":    opts.Limit,
	})

	// Create repository
	repo, err := repository.NewFileRepository(cfg.Backup.MetadataDirectory)
	if err != nil {
		return fmt.Errorf("failed to create repository: %w", err)
	}

	// Build filter
	filter := &repository.ListFilter{
		Database:     opts.Database,
		DatabaseType: opts.Type,
		StorageType:  opts.Storage,
		Limit:        opts.Limit,
		SortBy:       opts.Sort,
		SortOrder:    opts.Order,
	}

	// Parse time filters
	if opts.From != "" {
		fromTime, err := time.Parse(time.RFC3339, opts.From)
		if err != nil {
			return fmt.Errorf("invalid from date format (use RFC3339): %w", err)
		}
		filter.From = &fromTime
	}

	if opts.To != "" {
		toTime, err := time.Parse(time.RFC3339, opts.To)
		if err != nil {
			return fmt.Errorf("invalid to date format (use RFC3339): %w", err)
		}
		filter.To = &toTime
	}

	// Parse tags
	if len(opts.Tags) > 0 {
		filter.Tags = parseTags(opts.Tags)
	}

	// List backups
	backups, err := repo.List(ctx, filter)
	if err != nil {
		return fmt.Errorf("failed to list backups: %w", err)
	}

	// Display results based on format
	switch strings.ToLower(opts.Format) {
	case "json":
		return printJSON(backups)
	case "yaml", "yml":
		return printYAML(backups)
	default:
		return printTable(backups)
	}
}

func printTable(backups []*models.BackupMetadata) error {
	if len(backups) == 0 {
		fmt.Println("No backups found.")
		return nil
	}

	fmt.Println("Available backups:")
	fmt.Println()
	fmt.Println("ID                                     DATABASE       TYPE       SIZE        DATE                  STATUS")
	fmt.Println("────────────────────────────────────────────────────────────────────────────────────────────────────────────────")

	for _, b := range backups {
		fmt.Printf("%-38s %-14s %-10s %-11s %-21s %s\n",
			truncate(b.ID, 38),
			truncate(b.Database, 14),
			string(b.DatabaseType),
			formatBytes(b.Size),
			b.StartTime.Format("2006-01-02 15:04:05"),
			string(b.Status),
		)
	}

	fmt.Println()
	fmt.Printf("Total: %d backup(s)\n", len(backups))
	return nil
}

func printJSON(backups []*models.BackupMetadata) error {
	data, err := json.MarshalIndent(backups, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal JSON: %w", err)
	}
	fmt.Println(string(data))
	return nil
}

func printYAML(backups []*models.BackupMetadata) error {
	data, err := yaml.Marshal(backups)
	if err != nil {
		return fmt.Errorf("failed to marshal YAML: %w", err)
	}
	fmt.Println(string(data))
	return nil
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}
