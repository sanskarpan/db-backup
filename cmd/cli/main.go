// Package main is the entry point for the CLI application
package main

import (
	"github.com/sanskarpan/db-backup/cmd/cli/commands"

	// Register database drivers
	_ "github.com/sanskarpan/db-backup/internal/database/mongodb"
	_ "github.com/sanskarpan/db-backup/internal/database/mysql"
	_ "github.com/sanskarpan/db-backup/internal/database/postgres"
	_ "github.com/sanskarpan/db-backup/internal/database/sqlite"
)

func main() {
	commands.Execute()
}
