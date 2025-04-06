# Trusted Sequencer

A high-performance sequencer for rollups that ensures transaction ordering and data availability.

## Features

- Transaction ordering and sequencing
- Data availability layer integration (Celestia/Avail)
- State management and EVM compatibility
- High-performance transaction processing
- Configurable through TOML configuration

## Prerequisites

- Go 1.21 or later
- LevelDB
- Access to a Geth node
- Access to a DA layer (Celestia or Avail)

## Installation

```bash
git clone https://github.com/airchains-network/trusted-sequencer
cd trusted-sequencer
make build
```

## Configuration

The sequencer uses a configuration file located at `~/.trusted-sequencer/config.toml`. The first time you run the sequencer, it will create this file with default values.

### Default Configuration

```toml
[general]
geth_rpc_url = "http://127.0.0.1:8545"
proxy_port = ":8080"

[database]
txn_db_path = "~/.trusted-sequencer/data/txn_db"
batch_db_path = "~/.trusted-sequencer/data/batch_db"
state_path = "~/.trusted-sequencer/data/state_db"

[da]
type = "avail"  # or "celestia"
node_addr = "http://localhost:26657"
auth_token = "dummy-auth-token"
namespace = "airchains"

[rollup]
rollup_id = "airchains-rollup-69420"

[genesis]
file_path = "~/.trusted-sequencer/genesis.json"
```

### Required Files

1. **Genesis File**: You must create a `genesis.json` file at `~/.trusted-sequencer/genesis.json` before starting the sequencer. This file should contain the initial state configuration for your rollup.

## Usage

1. Create your `genesis.json` file in the `~/.trusted-sequencer/` directory
2. (Optional) Modify the configuration in `~/.trusted-sequencer/config.toml`
3. Start the sequencer:

```bash
./sequencer
```

The sequencer will:
- Load configuration from `~/.trusted-sequencer/config.toml`
- Verify the existence of `genesis.json`
- Initialize databases and state
- Connect to the specified DA layer
- Start processing transactions

## Architecture

The sequencer consists of several key components:

- **Transaction Pool**: Manages incoming transactions
- **Batch Processor**: Groups transactions into batches
- **State Manager**: Maintains the rollup state
- **DA Client**: Handles data availability submissions
- **Proxy Server**: Provides JSON-RPC interface

## Development

### Building

```bash
go build -o sequencer ./cmd/sequencer
```

### Testing

```bash
go test ./...
```

## License

[License Type] - See LICENSE file for details
