package commands

import (
	"fmt"
	"runtime"

	"github.com/spf13/cobra"
)

var (
	// Version information (set via ldflags during build)
	Version   = "dev"
	BuildTime = "unknown"
	GitCommit = "unknown"
)

// versionCmd represents the version command
var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Show version information",
	Long:  `Display detailed version information including build time and git commit.`,
	Run:   runVersion,
}

func init() {
	rootCmd.AddCommand(versionCmd)

	// Add flags
	versionCmd.Flags().BoolP("short", "s", false, "print version number only")
}

func runVersion(cmd *cobra.Command, args []string) {
	short, _ := cmd.Flags().GetBool("short")

	if short {
		fmt.Println(Version)
		return
	}

	fmt.Printf("DB Backup Utility\n")
	fmt.Printf("  Version:      %s\n", Version)
	fmt.Printf("  Build Time:   %s\n", BuildTime)
	fmt.Printf("  Git Commit:   %s\n", GitCommit)
	fmt.Printf("  Go Version:   %s\n", runtime.Version())
	fmt.Printf("  OS/Arch:      %s/%s\n", runtime.GOOS, runtime.GOARCH)
}
