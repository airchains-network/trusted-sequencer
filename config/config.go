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

// LoadConfig reads from config.toml and returns Config struct
func LoadConfig(path string) (Config, error) {
	var cfg Config
	file, err := os.ReadFile(path)
	if err != nil {
		return cfg, fmt.Errorf("failed to read config file: %v", err)
	}

	err = toml.Unmarshal(file, &cfg)
	if err != nil {
		return cfg, fmt.Errorf("failed to parse config file: %v", err)
	}

	// Expand home directory in database paths
	if cfg.Database.TxnDBPath, err = expandHomeDir(cfg.Database.TxnDBPath); err != nil {
		return cfg, err
	}
	if cfg.Database.BatchDBPath, err = expandHomeDir(cfg.Database.BatchDBPath); err != nil {
		return cfg, err
	}
	if cfg.Database.StatePath, err = expandHomeDir(cfg.Database.StatePath); err != nil {
		return cfg, err
	}

	// Create directories if they don't exist
	dirs := []string{
		cfg.Database.TxnDBPath,
		cfg.Database.BatchDBPath,
		cfg.Database.StatePath,
	}

	for _, dir := range dirs {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return cfg, err
		}
	}

	return cfg, nil
}
