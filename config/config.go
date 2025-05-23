package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/pelletier/go-toml"
)

// Config holds the application configuration
type Config struct {
	General  GeneralConfig  `toml:"general"`
	Database DatabaseConfig `toml:"database"`
	DA       DAConfig       `toml:"da"`
	Genesis  GenesisConfig  `toml:"genesis"`
	Rollup   RollupConfig   `toml:"rollup"`
	Prover   ProverConfig   `toml:"prover"`
	Junction JunctionConfig `toml:"junction"`
}

// GeneralConfig holds general settings
type GeneralConfig struct {
	GethRPCURL    string `toml:"geth_rpc_url"`
	GethWSURL     string `toml:"geth_ws_url"`
	RPCPort       string `toml:"rpc_port"`
	WebSocketPort string `toml:"ws_port"`
}

// DatabaseConfig holds database paths
type DatabaseConfig struct {
	TxnDBPath   string `toml:"txn_db_path"`
	BatchDBPath string `toml:"batch_db_path"`
	StatePath   string `toml:"state_path"`
}

// DAConfig holds DA (Data Availability) settings
type DAConfig struct {
	Type      string `toml:"type"` // "celestia" or "avail"
	NodeAddr  string `toml:"node_addr"`
	AuthToken string `toml:"auth_token"`
	Namespace string `toml:"namespace"`
}

type RollupConfig struct {
	RollupID string `toml:"rollup_id"` // TODO Change this to RollupID
}

type GenesisConfig struct {
	FilePath string `toml:"file_path"`
}

// ProverConfig holds prover settings
type ProverConfig struct {
	URL string `toml:"url"`
}

// JunctionConfig holds Junction network settings
type JunctionConfig struct {
	AccountName    string `toml:account_name`
	NodeApiAddress string `toml:"node_api_address"`
	NodeRpcAddress string `toml:"node_rpc_address"`
}

// expandHomeDir replaces ~ with the user's home directory
func expandHomeDir(path string) (string, error) {
	if !strings.HasPrefix(path, "~/") {
		return path, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, path[2:]), nil
}

// DefaultConfig returns a Config with default values
func DefaultConfig() Config {
	home, _ := os.UserHomeDir()
	basePath := filepath.Join(home, ".trusted-sequencer")

	return Config{
		General: GeneralConfig{
			GethRPCURL:    "http://127.0.0.1:8545",
			GethWSURL:     "ws://127.0.0.1:8546",
			RPCPort:       ":11111",
			WebSocketPort: ":11112",
		},
		Database: DatabaseConfig{
			TxnDBPath:   filepath.Join(basePath, "data/txn_db"),
			BatchDBPath: filepath.Join(basePath, "data/batch_db"),
			StatePath:   filepath.Join(basePath, "data/state_db"),
		},
		DA: DAConfig{
			Type:      "", // Will be set by user
			NodeAddr:  "", // Will be set by user
			AuthToken: "", // Will be set by user
			Namespace: "", // Will be set by user
		},
		Rollup: RollupConfig{
			RollupID: "", // Will be set by user
		},
		Genesis: GenesisConfig{
			FilePath: filepath.Join(basePath, "genesis.json"),
		},
		Prover: ProverConfig{
			URL: "", // Will be set by user
		},
		Junction: JunctionConfig{
			AccountName:    "", // Will be set by user
			NodeApiAddress: "", // Will be set by user
			NodeRpcAddress: "", // Will be set by user
		},
	}
}

// InitConfig creates a new config file with default values
func InitConfig(path string) error {
	// Create directory if it doesn't exist
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create config directory: %v", err)
	}

	// Create default config
	cfg := DefaultConfig()

	// Marshal to TOML
	data, err := toml.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("failed to marshal default config: %v", err)
	}

	// Write to file
	if err := os.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("failed to write config file: %v", err)
	}

	return nil
}

// LoadConfig loads the config file, creating it if it doesn't exist
func LoadConfig(path string) (Config, error) {
	var cfg Config

	// Check if config exists
	if _, err := os.Stat(path); os.IsNotExist(err) {
		fmt.Printf("Config file not found at %s. Creating with default values...\n", path)
		if err := InitConfig(path); err != nil {
			return cfg, err
		}
	}

	// Read and parse config
	file, err := os.ReadFile(path)
	if err != nil {
		return cfg, fmt.Errorf("failed to read config file: %v", err)
	}

	err = toml.Unmarshal(file, &cfg)
	if err != nil {
		return cfg, fmt.Errorf("failed to parse config file: %v", err)
	}

	// Create data directories
	dirs := []string{
		cfg.Database.TxnDBPath,
		cfg.Database.BatchDBPath,
		cfg.Database.StatePath,
		filepath.Dir(cfg.Genesis.FilePath),
	}

	for _, dir := range dirs {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return cfg, fmt.Errorf("failed to create directory %s: %v", dir, err)
		}
	}

	return cfg, nil
}

// Save saves the configuration to a file
func (c *Config) Save(path string) error {
	data, err := toml.Marshal(c)
	if err != nil {
		return fmt.Errorf("failed to marshal config: %v", err)
	}

	if err := os.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("failed to write config file: %v", err)
	}

	return nil
}
