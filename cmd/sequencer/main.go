package main

import (
	"fmt"
	"os"

	"github.com/airchains-network/trusted-sequencer/cmd/sequencer/commands"
	"github.com/spf13/cobra"
)

var (
	// Version is the version of the build, set by -ldflags
	Version = "dev"
	// Commit is the git commit hash of the build, set by -ldflags
	Commit = "none"
	// BuildTime is the time the binary was built, set by -ldflags
	BuildTime = "unknown"
)

func main() {
	// Create root command
	rootCmd := &cobra.Command{
		Use:   "trusted-sequencer",
		Short: "A trusted sequencer implementation for rollups",
		Long: `A trusted sequencer implementation for rollups that ensures transaction ordering and data availability.
It supports integration with various data availability layers like Avail and Celestia.`,
		Version: fmt.Sprintf("%s (commit: %s) built at %s", Version, Commit, BuildTime),
	}

	// Add commands
	rootCmd.AddCommand(commands.InitCmd)
	rootCmd.AddCommand(commands.StartCmd)
	// Execute root command
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}
