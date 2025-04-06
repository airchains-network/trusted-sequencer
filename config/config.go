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
}

// GeneralConfig holds general settings
type GeneralConfig struct {
	GethRPCURL string `toml:"geth_rpc_url"`
	ProxyPort  string `toml:"proxy_port"`
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
			GethRPCURL: "http://127.0.0.1:8545",
			ProxyPort:  ":8080",
		},
		Database: DatabaseConfig{
			TxnDBPath:   filepath.Join(basePath, "data/txn_db"),
			BatchDBPath: filepath.Join(basePath, "data/batch_db"),
			StatePath:   filepath.Join(basePath, "data/state_db"),
		},
		DA: DAConfig{
			Type:      "avail",
			NodeAddr:  "http://localhost:26657",
			AuthToken: "dummy-auth-token",
			Namespace: "airchains",
		},
		Rollup: RollupConfig{
			RollupID: "airchains-rollup-69420",
		},
		Genesis: GenesisConfig{
			FilePath: filepath.Join(basePath, "genesis.json"),
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
