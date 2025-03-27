package main

import (
	"time"

	"github.com/airchains-network/trusted-sequencer/batch"
	"github.com/airchains-network/trusted-sequencer/batch/da"
	"github.com/airchains-network/trusted-sequencer/config"
	"github.com/airchains-network/trusted-sequencer/db"
	"github.com/airchains-network/trusted-sequencer/eth"
	"github.com/airchains-network/trusted-sequencer/pool"
	"github.com/airchains-network/trusted-sequencer/proxy"
	"github.com/sirupsen/logrus"
)

func main() {
	// Initialize logger
	log := logrus.New()
	log.SetFormatter(&logrus.TextFormatter{
		FullTimestamp:   true,
		TimestampFormat: "2006-01-02 15:04:05",
		ForceColors:     true,
	})
	log.SetLevel(logrus.DebugLevel)

	// Load configuration from config.toml
	cfg, err := config.LoadConfig("config.toml")
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	// Initialize databases
	txnDB, batchDB, err := db.NewLevelDBs(cfg.Database.TxnDBPath, cfg.Database.BatchDBPath)
	if err != nil {
		log.Fatalf("Failed to initialize databases: %v", err)
	}
	defer txnDB.Close()
	defer batchDB.Close()

	// Initialize Ethereum client
	client, err := eth.NewClient(cfg.General.GethRPCURL)
	if err != nil {
		log.Fatalf("Failed to initialize Geth client: %v", err)
	}

	// Replace the existing DA initialization with retry logic
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

	// Start transaction pool
	txPool := pool.NewTxPool()
	go txPool.Process(client, txnDB, log)

	// Start batch processing
	go batch.ProcessBlocks(client, txnDB, batchDB, daClient, log)

	// Start proxy server
	log.Infof("Starting Trusted Sequencer on %s...", cfg.General.ProxyPort)
	if err := proxy.Start(cfg.General.ProxyPort, client, txnDB, batchDB, txPool, log); err != nil {
		log.Fatalf("Proxy server failed: %v", err)
	}
}
