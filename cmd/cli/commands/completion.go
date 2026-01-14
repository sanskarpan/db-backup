package commands

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"

	"github.com/spf13/cobra"
)

// completionCmd represents the completion command
var completionCmd = &cobra.Command{
	Use:   "completion [bash|zsh|fish|powershell]",
	Short: "Generate shell completion scripts",
	Long: `Generate completion scripts for your shell.

To load completions:

Bash:
  $ source <(db-backup completion bash)

  # To load completions for each session, execute once:
  # Linux:
  $ db-backup completion bash > /etc/bash_completion.d/db-backup
  # macOS:
  $ db-backup completion bash > /usr/local/etc/bash_completion.d/db-backup

Zsh:
  # If shell completion is not already enabled in your environment,
  # you will need to enable it.  You can execute the following once:

  $ echo "autoload -U compinit; compinit" >> ~/.zshrc

  # To load completions for each session, execute once:
  $ db-backup completion zsh > "${fpath[1]}/_db-backup"

  # You will need to start a new shell for this setup to take effect.

  # For oh-my-zsh users:
  $ db-backup completion zsh > ~/.oh-my-zsh/completions/_db-backup

Fish:
  $ db-backup completion fish | source

  # To load completions for each session, execute once:
  $ db-backup completion fish > ~/.config/fish/completions/db-backup.fish

PowerShell:
  PS> db-backup completion powershell | Out-String | Invoke-Expression

  # To load completions for every new session, run:
  PS> db-backup completion powershell > db-backup.ps1
  # and source this file from your PowerShell profile.
`,
	DisableFlagsInUseLine: true,
	ValidArgs:             []string{"bash", "zsh", "fish", "powershell"},
	Args:                  cobra.ExactValidArgs(1),
	RunE:                  runCompletion,
}

func init() {
	rootCmd.AddCommand(completionCmd)

	completionCmd.Flags().Bool("install", false, "Install completion script to system location")
	completionCmd.Flags().String("output", "", "Output file (default: stdout)")
}

func runCompletion(cmd *cobra.Command, args []string) error {
	shell := args[0]

	install, _ := cmd.Flags().GetBool("install")
	output, _ := cmd.Flags().GetString("output")

	if install {
		return installCompletion(shell)
	}

	if output != "" {
		return generateCompletionToFile(cmd, shell, output)
	}

	return generateCompletion(cmd, shell)
}

func generateCompletion(cmd *cobra.Command, shell string) error {
	switch shell {
	case "bash":
		return cmd.Root().GenBashCompletion(os.Stdout)
	case "zsh":
		return cmd.Root().GenZshCompletion(os.Stdout)
	case "fish":
		return cmd.Root().GenFishCompletion(os.Stdout, true)
	case "powershell":
		return cmd.Root().GenPowerShellCompletion(os.Stdout)
	default:
		return fmt.Errorf("unsupported shell: %s", shell)
	}
}

func generateCompletionToFile(cmd *cobra.Command, shell, output string) error {
	file, err := os.Create(output)
	if err != nil {
		return fmt.Errorf("failed to create output file: %w", err)
	}
	defer file.Close()

	switch shell {
	case "bash":
		return cmd.Root().GenBashCompletion(file)
	case "zsh":
		return cmd.Root().GenZshCompletion(file)
	case "fish":
		return cmd.Root().GenFishCompletion(file, true)
	case "powershell":
		return cmd.Root().GenPowerShellCompletion(file)
	default:
		return fmt.Errorf("unsupported shell: %s", shell)
	}
}

func installCompletion(shell string) error {
	var installPath string
	var content []byte

	switch shell {
	case "bash":
		installPath = getBashCompletionPath()
	case "zsh":
		installPath = getZshCompletionPath()
	case "fish":
		installPath = getFishCompletionPath()
	case "powershell":
		installPath = getPowerShellCompletionPath()
	default:
		return fmt.Errorf("unsupported shell: %s", shell)
	}

	// Read the completion script from embedded files or generate it
	if installPath == "" {
		return fmt.Errorf("could not determine installation path for %s", shell)
	}

	// Ensure directory exists
	dir := filepath.Dir(installPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create directory %s: %w", dir, err)
	}

	// Generate completion script
	file, err := os.Create(installPath)
	if err != nil {
		return fmt.Errorf("failed to create file %s: %w", installPath, err)
	}
	defer file.Close()

	var genErr error
	switch shell {
	case "bash":
		genErr = rootCmd.GenBashCompletion(file)
	case "zsh":
		genErr = rootCmd.GenZshCompletion(file)
	case "fish":
		genErr = rootCmd.GenFishCompletion(file, true)
	case "powershell":
		genErr = rootCmd.GenPowerShellCompletion(file)
	}

	if genErr != nil {
		return fmt.Errorf("failed to generate completion: %w", genErr)
	}

	fmt.Printf("Completion script installed to: %s\n", installPath)
	fmt.Println("\nPlease restart your shell or source the completion file.")

	if shell == "bash" {
		fmt.Printf("\nRun: source %s\n", installPath)
	} else if shell == "zsh" {
		fmt.Println("\nRun: exec zsh")
	} else if shell == "fish" {
		fmt.Println("\nRun: exec fish")
	} else if shell == "powershell" {
		fmt.Printf("\nRun: . %s\n", installPath)
	}

	return nil
}

func getBashCompletionPath() string {
	if runtime.GOOS == "darwin" {
		// macOS with Homebrew
		if _, err := os.Stat("/usr/local/etc/bash_completion.d"); err == nil {
			return "/usr/local/etc/bash_completion.d/db-backup"
		}
		if _, err := os.Stat("/opt/homebrew/etc/bash_completion.d"); err == nil {
			return "/opt/homebrew/etc/bash_completion.d/db-backup"
		}
	}

	// Linux
	if _, err := os.Stat("/etc/bash_completion.d"); err == nil {
		return "/etc/bash_completion.d/db-backup"
	}

	// User-local fallback
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".bash_completion.d", "db-backup")
}

func getZshCompletionPath() string {
	// oh-my-zsh
	home, _ := os.UserHomeDir()
	ohmyzsh := filepath.Join(home, ".oh-my-zsh", "completions", "_db-backup")
	if _, err := os.Stat(filepath.Dir(ohmyzsh)); err == nil {
		return ohmyzsh
	}

	// Standard zsh
	if runtime.GOOS == "darwin" {
		// macOS with Homebrew
		if _, err := os.Stat("/usr/local/share/zsh/site-functions"); err == nil {
			return "/usr/local/share/zsh/site-functions/_db-backup"
		}
		if _, err := os.Stat("/opt/homebrew/share/zsh/site-functions"); err == nil {
			return "/opt/homebrew/share/zsh/site-functions/_db-backup"
		}
	}

	// User-local fallback
	return filepath.Join(home, ".zfunc", "_db-backup")
}

func getFishCompletionPath() string {
	home, _ := os.UserHomeDir()

	// User config
	userConfig := filepath.Join(home, ".config", "fish", "completions", "db-backup.fish")
	if _, err := os.Stat(filepath.Dir(userConfig)); err == nil {
		return userConfig
	}

	// System-wide (macOS with Homebrew)
	if runtime.GOOS == "darwin" {
		if _, err := os.Stat("/usr/local/share/fish/vendor_completions.d"); err == nil {
			return "/usr/local/share/fish/vendor_completions.d/db-backup.fish"
		}
		if _, err := os.Stat("/opt/homebrew/share/fish/vendor_completions.d"); err == nil {
			return "/opt/homebrew/share/fish/vendor_completions.d/db-backup.fish"
		}
	}

	// Create user config dir and return path
	os.MkdirAll(filepath.Dir(userConfig), 0755)
	return userConfig
}

func getPowerShellCompletionPath() string {
	if runtime.GOOS != "windows" {
		return ""
	}

	// PowerShell profile directory
	userProfile := os.Getenv("USERPROFILE")
	if userProfile == "" {
		return ""
	}

	// Documents\PowerShell\Modules (PowerShell 7+)
	psModules := filepath.Join(userProfile, "Documents", "PowerShell", "Modules", "DbBackupCompletion")
	if _, err := os.Stat(filepath.Dir(psModules)); err == nil {
		return filepath.Join(psModules, "db-backup.ps1")
	}

	// Documents\WindowsPowerShell\Modules (Windows PowerShell 5.1)
	wsModules := filepath.Join(userProfile, "Documents", "WindowsPowerShell", "Modules", "DbBackupCompletion")
	return filepath.Join(wsModules, "db-backup.ps1")
}

// ============================================================================
// Custom Completion Functions
// ============================================================================

// These functions are called by Cobra's completion system

func init() {
	// Register custom completion functions for flags

	// Database completion
	registerDatabaseCompletion := func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		databases, err := getDatabases(cmd.Context())
		if err != nil {
			return nil, cobra.ShellCompDirectiveError
		}
		return databases, cobra.ShellCompDirectiveNoFileComp
	}

	// Backup completion
	registerBackupCompletion := func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		backups, err := getBackups(cmd.Context(), "")
		if err != nil {
			return nil, cobra.ShellCompDirectiveError
		}
		return backups, cobra.ShellCompDirectiveNoFileComp
	}

	// Provider completion
	registerProviderCompletion := func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		providers := []string{"local", "s3", "gcs", "azure", "minio", "wasabi", "backblaze", "digitalocean"}
		return providers, cobra.ShellCompDirectiveNoFileComp
	}

	// Database type completion
	registerTypeCompletion := func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		types := []string{"postgres", "mysql", "mongodb", "redis", "sqlite", "cassandra", "dynamodb", "elasticsearch", "influxdb", "timescaledb"}
		return types, cobra.ShellCompDirectiveNoFileComp
	}

	// Register these completions for relevant commands
	// This will be done in each command's init function
}

// Helper functions to get dynamic data
func getDatabases(ctx context.Context) ([]string, error) {
	// TODO: Implement actual database listing
	// For now, return empty list
	return []string{}, nil
}

func getBackups(ctx context.Context, database string) ([]string, error) {
	// TODO: Implement actual backup listing
	// For now, return empty list
	return []string{}, nil
}

func getSchedules(ctx context.Context) ([]string, error) {
	// TODO: Implement actual schedule listing
	return []string{}, nil
}

func getRetentionPolicies(ctx context.Context) ([]string, error) {
	// TODO: Implement actual retention policy listing
	return []string{}, nil
}

func getAlerts(ctx context.Context) ([]string, error) {
	// TODO: Implement actual alert listing
	return []string{}, nil
}

func getTags(ctx context.Context) ([]string, error) {
	// TODO: Implement actual tag listing
	return []string{}, nil
}
