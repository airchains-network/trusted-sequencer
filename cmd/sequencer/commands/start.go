package commands

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/airchains-network/trusted-sequencer/config"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

// StartCmd represents the start command
var StartCmd = &cobra.Command{
	Use:   "start",
	Short: "Start the trusted sequencer",
	Long: `Start the trusted sequencer with the configuration from ~/.trusted-sequencer/config.toml.
The sequencer will process transactions and submit them to the configured data availability layer.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return startCommand()
	},
}

func startCommand() error {
	log := logrus.New()
	log.SetFormatter(&logrus.TextFormatter{
		FullTimestamp:   true,
		TimestampFormat: "2006-01-02 15:04:05",
		ForceColors:     true,
	})
	log.SetLevel(logrus.InfoLevel)

	// Get user's home directory
	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("failed to get home directory: %v", err)
	}

	// Load configuration
	configPath := filepath.Join(home, ".trusted-sequencer", "config.toml")
	cfg, err := config.LoadConfig(configPath)
	if err != nil {
		return fmt.Errorf("failed to load config: %v", err)
	}

	// Check if genesis.json exists
	genesisPath := filepath.Join(home, ".trusted-sequencer", "genesis.json")
	if _, err := os.Stat(genesisPath); os.IsNotExist(err) {
		return fmt.Errorf("genesis.json not found at %s", genesisPath)
	}

	// Check if wallet exists
	keysDir := filepath.Join(home, ".trusted-sequencer", "keys")
	if _, err := os.Stat(keysDir); os.IsNotExist(err) {
		return fmt.Errorf("no wallet found. Please create a wallet using 'trusted-sequencer create-account <name>'")
	}

	// Check if keys directory is empty
	entries, err := os.ReadDir(keysDir)
	if err != nil {
		return fmt.Errorf("failed to read keys directory: %v", err)
	}
	if len(entries) == 0 {
		return fmt.Errorf("no wallet found. Please create a wallet using 'trusted-sequencer create-account <name>'")
	}

	// TODO: Implement the actual sequencer start logic here
	log.Info("Starting trusted sequencer...")
	log.Infof("DA Layer: %s", cfg.DA.Type)
	log.Infof("Node Address: %s", cfg.DA.NodeAddr)
	log.Infof("Namespace: %s", cfg.DA.Namespace)
	log.Infof("Rollup ID: %s", cfg.Rollup.RollupID)
	log.Infof("Junction API: %s", cfg.Junction.API)
	log.Infof("Junction RPC: %s", cfg.Junction.RPC)

	return nil
}
