package commands

import (
	"context"
	"fmt"
	"math/big"
	"os"
	"path/filepath"
	"time"

	"github.com/airchains-network/trusted-sequencer/batch"
	"github.com/airchains-network/trusted-sequencer/batch/da"
	"github.com/airchains-network/trusted-sequencer/config"
	"github.com/airchains-network/trusted-sequencer/db"
	"github.com/airchains-network/trusted-sequencer/eth"
	"github.com/airchains-network/trusted-sequencer/junction"
	"github.com/airchains-network/trusted-sequencer/pool"
	"github.com/airchains-network/trusted-sequencer/prover"
	"github.com/airchains-network/trusted-sequencer/proxy"
	"github.com/airchains-network/trusted-sequencer/state"
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

	ctx := context.Background()

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
	if _, err := os.Stat(cfg.Genesis.FilePath); os.IsNotExist(err) {
		return fmt.Errorf("genesis.json not found at %s", cfg.Genesis.FilePath)
	}

	// Initialize databases
	txnDB, batchDB, err := db.NewLevelDBs(cfg.Database.TxnDBPath, cfg.Database.BatchDBPath)
	if err != nil {
		log.Fatalf("Failed to initialize databases: %v", err)
	}
	defer txnDB.Close()
	defer batchDB.Close()

	// Initialize Ethereum client
	ethClient, err := eth.NewClient(cfg.General.GethRPCURL)
	if err != nil {
		log.Fatalf("Failed to initialize Geth client: %v", err)
	}

	// Initialize junction client
	accountPath := filepath.Join(home, ".junctiond", "keyring-test")
	junctionClient, err := junction.NewJunctionClient(ctx, cfg.Junction.AccountName, accountPath, cfg.Junction.NodeRpcAddress)
	if err != nil {
		log.Fatalf("Failed to initialize junction client: %v", err)
	}

	// Initialize DA client with retry logic
	var daClient da.DAClient
	log.Info("Initializing DA client...")
	for {
		if cfg.DA.Type == "celestia" {
			log.Info("Attempting to connect to Celestia...")
			daClient, err = da.NewCelestiaClient(cfg.DA.NodeAddr, cfg.DA.AuthToken, cfg.DA.Namespace, log)
		} else if cfg.DA.Type == "avail" {
			log.Info("Attempting to connect to Avail...")
			daClient, err = da.NewAvailClient(cfg.DA.NodeAddr, cfg.DA.AuthToken, cfg.DA.Namespace, log)
		} else {
			log.Fatalf("Unsupported DA submission type: %s", cfg.DA.Type)
		}

		if err == nil {
			log.Infof("Successfully connected to %s DA layer", cfg.DA.Type)
			break
		}

		log.Warnf("Failed to initialize %s client: %v", cfg.DA.Type, err)
		log.Info("Retrying in 5 seconds...")
		time.Sleep(5 * time.Second)
	}

	// Initialize prover client
	if cfg.Prover.URL == "" {
		log.Fatalf("Prover URL not configured")
	}
	proverClient := prover.NewProverClient(cfg.Prover.URL)
	log.Infof("Initialized prover client with URL: %s", cfg.Prover.URL)

	// Start transaction pool
	txPool := pool.NewTxPool()
	go txPool.Process(ethClient, txnDB, log)

	// Initialize state
	evmState, err := state.NewEVMState(cfg.Database.StatePath)
	if err != nil {
		log.Fatalf("Failed to initialize state: %v", err)
	}
	defer evmState.Close()

	vmProcessor := state.NewProcessor(evmState, eth.NewRpcClient(cfg.General.GethRPCURL), log)

	// Get last processed block
	lastBlockBytes, err := batchDB.Get([]byte("last_block"))
	if err != nil {
		log.Warnf("Failed to get last block from database: %v, starting from block 1", err)
		lastBlockBytes = nil
	}

	lastBlock := int64(0)
	if lastBlockBytes != nil {
		blockNum := big.NewInt(0).SetBytes(lastBlockBytes)
		if !blockNum.IsInt64() {
			log.Warnf("Block number %s is too large for int64, defaulting to 1", blockNum.String())
		} else {
			lastBlock = blockNum.Int64()
		}
	}

	// Handle genesis if starting from block 0
	if lastBlock == 0 {
		genesisHash, err := vmProcessor.LoadGenesis(cfg.Genesis.FilePath)
		if err != nil {
			log.Fatalf("Failed to load genesis: %v", err)
		}
		// Save genesis hash to database as state_0
		if err := batchDB.Put([]byte("state_0"), []byte(genesisHash)); err != nil {
			log.Fatalf("Failed to save genesis hash to database: %v", err)
		}
		log.Infof("Genesis State Hash: %s", genesisHash)
	}

	log.Infof("Starting from block %d", lastBlock)

	// Start block processing
	go batch.ProcessBlocks(ethClient, junctionClient, proverClient, txnDB, batchDB, daClient, evmState, vmProcessor, cfg.Rollup.RollupID, log)

	// Start proxy server
	log.Infof("Starting Trusted Sequencer on %s...", cfg.General.RPCPort)
	if err := proxy.Start(cfg.General.RPCPort, cfg.General.WebSocketPort, ethClient, txnDB, batchDB, txPool, log); err != nil {
		log.Fatalf("Proxy server failed: %v", err)
	}

	return nil
}
