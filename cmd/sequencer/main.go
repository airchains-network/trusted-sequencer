package main

import (
	"github.com/airchains-network/trusted-sequencer/internal/batch"
	"github.com/airchains-network/trusted-sequencer/internal/config"
	"github.com/airchains-network/trusted-sequencer/internal/db"
	"github.com/airchains-network/trusted-sequencer/internal/eth"
	"github.com/airchains-network/trusted-sequencer/internal/pool"
	"github.com/airchains-network/trusted-sequencer/internal/proxy"
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

	// Load configuration
	cfg := config.Default()

	// Initialize databases
	txnDB, batchDB, err := db.NewLevelDBs(cfg.TxnDBPath, cfg.BatchDBPath)
	if err != nil {
		log.Fatalf("Failed to initialize databases: %v", err)
	}
	defer txnDB.Close()
	defer batchDB.Close()

	// Initialize Ethereum client
	client, err := eth.NewClient(cfg.GethRPCURL)
	if err != nil {
		log.Fatalf("Failed to initialize Geth client: %v", err)
	}

	// Start transaction pool
	txPool := pool.NewTxPool()
	go txPool.Process(client, txnDB, log)

	// Start batch processingz
	go batch.ProcessBlocks(client, txnDB, batchDB, log)

	// Start proxy server
	log.Infof("Starting Trusted Sequencer on %s...", cfg.ProxyPort)
	if err := proxy.Start(cfg.ProxyPort, client, txnDB, batchDB, txPool, log); err != nil {
		log.Fatalf("Proxy server failed: %v", err)
	}
}
