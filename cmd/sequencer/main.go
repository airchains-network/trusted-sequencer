package main

import (
	"os"

	"github.com/airchains-network/trusted-sequencer/cmd/sequencer/commands"
	"github.com/spf13/cobra"
)

func main() {
	// Create root command
	rootCmd := &cobra.Command{
		Use:   "trusted-sequencer",
		Short: "A trusted sequencer implementation for rollups",
		Long: `A trusted sequencer implementation for rollups that ensures transaction ordering and data availability.
It supports integration with various data availability layers like Avail and Celestia.`,
	}

	// Add commands
	rootCmd.AddCommand(commands.InitCmd)
	rootCmd.AddCommand(commands.StartCmd)

	// Execute root command
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}
