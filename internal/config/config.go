package config

// Config holds the application configuration
type Config struct {
	GethRPCURL  string // URL of the Geth RPC endpoint
	TxnDBPath   string // Path to the transaction database
	BatchDBPath string // Path to the batch database
	ProxyPort   string // Port for the proxy server
}

// Default returns the default configuration values
func Default() Config {
	return Config{
		GethRPCURL:  "http://192.168.1.37:8545",
		TxnDBPath:   "./data/txn_db",
		BatchDBPath: "./data/batch_db",
		ProxyPort:   ":8080",
	}
}
