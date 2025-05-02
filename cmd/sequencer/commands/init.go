package commands

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/airchains-network/trusted-sequencer/config"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

// InitCmd represents the init command
var InitCmd = &cobra.Command{
	Use:   "init",
	Short: "Initialize the trusted sequencer",
	Long: `Initialize the trusted sequencer with the required configuration.
This command creates the necessary directories and configuration files.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return initCommand(cmd)
	},
}

func init() {
	// DA configuration flags
	InitCmd.Flags().String("da.type", "", "DA layer type (avail/celestia)")
	InitCmd.Flags().String("da.node-addr", "", "DA node address")
	InitCmd.Flags().String("da.auth-token", "", "DA auth token")
	InitCmd.Flags().String("da.namespace", "", "DA namespace")

	// Rollup configuration flags
	InitCmd.Flags().String("rollup.id", "", "Rollup ID")

	// General configuration flags
	InitCmd.Flags().String("geth.rpc-url", "http://127.0.0.1:8545", "Geth RPC URL")
	InitCmd.Flags().String("geth.ws-url", "ws://127.0.0.1:8546", "Geth WebSocket URL")
	InitCmd.Flags().String("rpc.port", ":11111", "RPC server port")
	InitCmd.Flags().String("ws.port", ":11112", "WebSocket server port")

	// Prover configuration flags
	InitCmd.Flags().String("prover.url", "", "Prover URL")

	// Junction configuration flags
	InitCmd.Flags().String("junction.api", "", "Junction API URL")
	InitCmd.Flags().String("junction.rpc", "", "Junction RPC URL")

	// Mark required flags
	InitCmd.MarkFlagRequired("da.type")
	InitCmd.MarkFlagRequired("da.node-addr")
	InitCmd.MarkFlagRequired("da.auth-token")
	InitCmd.MarkFlagRequired("da.namespace")
	InitCmd.MarkFlagRequired("rollup.id")
	InitCmd.MarkFlagRequired("prover.url")
	InitCmd.MarkFlagRequired("junction.api")
	InitCmd.MarkFlagRequired("junction.rpc")
}

func initCommand(cmd *cobra.Command) error {
	// Get flag values
	daType, _ := cmd.Flags().GetString("da.type")
	nodeAddr, _ := cmd.Flags().GetString("da.node-addr")
	authToken, _ := cmd.Flags().GetString("da.auth-token")
	namespace, _ := cmd.Flags().GetString("da.namespace")
	rollupID, _ := cmd.Flags().GetString("rollup.id")
	gethRPCURL, _ := cmd.Flags().GetString("geth.rpc-url")
	gethWSURL, _ := cmd.Flags().GetString("geth.ws-url")
	rpcPort, _ := cmd.Flags().GetString("rpc.port")
	wsPort, _ := cmd.Flags().GetString("ws.port")
	proverURL, _ := cmd.Flags().GetString("prover.url")
	junctionAPI, _ := cmd.Flags().GetString("junction.api")
	junctionRPC, _ := cmd.Flags().GetString("junction.rpc")

	log := logrus.New()
	log.SetFormatter(&logrus.TextFormatter{
		FullTimestamp:   true,
		TimestampFormat: "2006-01-02 15:04:05",
		ForceColors:     true,
	})
	log.SetLevel(logrus.InfoLevel)

	// Validate DA type
	if daType != "avail" && daType != "celestia" {
		return fmt.Errorf("invalid --da.type: %s. Must be either 'avail' or 'celestia'", daType)
	}

	// Get user's home directory
	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("failed to get home directory: %v", err)
	}

	// Create .trusted-sequencer directory
	sequencerDir := filepath.Join(home, ".trusted-sequencer")
	if err := os.MkdirAll(sequencerDir, 0755); err != nil {
		return fmt.Errorf("failed to create .trusted-sequencer directory: %v", err)
	}

	// Create data directories
	dataDir := filepath.Join(sequencerDir, "data")
	dirs := []string{
		filepath.Join(dataDir, "txn_db"),
		filepath.Join(dataDir, "batch_db"),
		filepath.Join(dataDir, "state_db"),
	}

	for _, dir := range dirs {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return fmt.Errorf("failed to create directory %s: %v", dir, err)
		}
	}

	// Create config with command-line flags
	cfg := config.DefaultConfig()
	cfg.DA.Type = daType
	cfg.DA.NodeAddr = nodeAddr
	cfg.DA.AuthToken = authToken
	cfg.DA.Namespace = namespace
	cfg.Rollup.RollupID = rollupID
	cfg.General.GethRPCURL = gethRPCURL
	cfg.General.GethWSURL = gethWSURL
	cfg.General.RPCPort = rpcPort
	cfg.General.WebSocketPort = wsPort
	cfg.Prover.URL = proverURL
	cfg.Junction.API = junctionAPI
	cfg.Junction.RPC = junctionRPC

	// Save config file
	configPath := filepath.Join(sequencerDir, "config.toml")
	if err := cfg.Save(configPath); err != nil {
		return fmt.Errorf("failed to create config file: %v", err)
	}
	log.Infof("Created config file at: %s", configPath)

	// Show configuration summary
	fmt.Println("\n=== Configuration Summary ===")
	fmt.Printf("DA Layer: %s\n", cfg.DA.Type)
	fmt.Printf("Node Address: %s\n", cfg.DA.NodeAddr)
	fmt.Printf("Namespace: %s\n", cfg.DA.Namespace)
	fmt.Printf("Rollup ID: %s\n", cfg.Rollup.RollupID)
	fmt.Printf("Geth RPC URL: %s\n", cfg.General.GethRPCURL)
	fmt.Printf("Geth WebSocket URL: %s\n", cfg.General.GethWSURL)
	fmt.Printf("RPC Port: %s\n", cfg.General.RPCPort)
	fmt.Printf("WebSocket Port: %s\n", cfg.General.WebSocketPort)
	fmt.Printf("Prover URL: %s\n", cfg.Prover.URL)
	fmt.Printf("Junction API: %s\n", cfg.Junction.API)
	fmt.Printf("Junction RPC: %s\n", cfg.Junction.RPC)
	fmt.Printf("Config File: %s\n", configPath)

	log.Info("\nInitialization completed successfully!")
	log.Info("Please create a genesis.json file at ~/.trusted-sequencer/genesis.json")
	log.Info("After creating the genesis file, you can start the sequencer using: ./trusted-sequencer start")

	return nil
}
