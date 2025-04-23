# Trusted Sequencer

A trusted sequencer implementation for rollups.

## Features

- Transaction ordering and sequencing
- Data availability layer integration (Celestia/Avail)
- State management and EVM compatibility
- High-performance transaction processing
- Configurable through command-line flags

## Prerequisites

- Go 1.23 or later
- Access to a DA layer node (Avail or Celestia)
- A Geth node running locally or remotely

## Installation

```bash
git clone https://github.com/airchains-network/trusted-sequencer.git
cd trusted-sequencer
make build
```

## Usage

### Initialize the Sequencer

```bash
./build/trusted-sequencer init \
  --da.type <avail|celestia> \
  --da.node-addr <node-address> \
  --da.auth-token <auth-token> \
  --da.namespace <namespace> \
  --rollup.id <rollup-id> \
  --geth.rpc-url <geth-rpc-url> \
  --proxy.port <proxy-port>
```

All DA layer configuration values are required:
- `--da.type`: DA layer type (avail/celestia)
- `--da.node-addr`: DA node address
- `--da.auth-token`: DA auth token
- `--da.namespace`: DA namespace
- `--rollup.id`: Rollup ID

Optional configuration:
- `--geth.rpc-url`: Geth RPC URL (default: http://localhost:8545)
- `--proxy.port`: Proxy server port (default: :11111)

### Start the Sequencer

```bash
./build/trusted-sequencer start
```

## Configuration

The sequencer creates a configuration file at `~/.trusted-sequencer/config.toml` with the following structure:

```toml
[da]
type = "<avail|celestia>"  # Required
node_addr = "<node-address>"  # Required
auth_token = "<auth-token>"  # Required
namespace = "<namespace>"  # Required

[rollup]
rollup_id = "<rollup-id>"  # Required

[general]
geth_rpc_url = "http://localhost:8545"
proxy_port = ":11111"

[genesis]
file_path = "~/.trusted-sequencer/genesis.json"
```

## Architecture

The sequencer consists of several key components:

- **Transaction Pool**: Manages incoming transactions
- **Batch Processor**: Groups transactions into batches
- **State Manager**: Maintains the rollup state
- **DA Client**: Handles data availability submissions
- **Proxy Server**: Provides JSON-RPC interface


## License

This project is licensed under the MIT License - see the LICENSE file for details.
