# Trusted Sequencer

A high-performance sequencer implementation for rollups, supporting multiple Data Availability (DA) layers.

## Features

- Transaction batching and sequencing
- Multiple DA layer support (Avail and Celestia)
- State management and verification
- High-performance transaction processing
- Configurable batch sizes and timeouts
- Metrics and monitoring support

## Prerequisites

- Go 1.23 or later
- LevelDB
- Access to an Operator node
- Access to a DA layer node (Avail or Celestia)

## Installation

1. Clone the repository:
```bash
git clone https://github.com/airchains-network/trusted-sequencer.git
cd trusted-sequencer
```

2. Install dependencies:
```bash
go mod download
```

3. Build the project:
```bash
go build -o sequencer ./cmd/sequencer
```

## Configuration

1. Copy the example configuration:
```bash
cp config.toml.example config.toml
```

2. Edit `config.toml` with your settings:
```toml
[general]
geth_rpc_url = "http://127.0.0.1:8545"  # Your Ethereum node RPC URL
proxy_port = ":8080"                    # Port for the proxy server

[database]
txn_db_path = "./data/txn_db"          # Path for transaction database
batch_db_path = "./data/batch_db"      # Path for batch database
state_path = "./data/state_db"         # Path for state database

[da]
type = "avail"                         # DA layer type (avail or celestia)
node_addr = "http://localhost:26657"   # DA layer node address
auth_token = "your_auth_token_here"    # DA layer authentication token
namespace = "airchains"                # DA layer namespace

[rollup]
namespace = "airchains-rollup-69420"   # Rollup ID

[genesis]
file_path = "./genesis.json"           # Path to genesis file
```

## Usage

1. Start the sequencer:
```bash
./sequencer
```

2. The sequencer will:
   - Connect to the Operator node
   - Initialize the DA layer client
   - Start processing transactions
   - Create and submit batches to the DA layer
   - Submits a Batch Proposal to Relayers



### Components

1. **Transaction Pool**
   - Collects and manages pending transactions
   - Implements transaction ordering

2. **Batch Processor**
   - Groups transactions into batches
   - Creates batch commitments
   - Submits batches to DA layer

3. **State Manager**
   - Maintains rollup state
   - Processes transactions
   - Updates state roots

4. **DA Layer Interface**
   - Supports multiple DA providers
   - Handles data submission
   - Manages commitments

### Data Flow

1. Transactions are received from the Ethereum network
2. Transactions are processed and ordered
3. Transactions are grouped into batches
4. Batches are submitted to the DA layer
5. State is updated and verified






## Support

For support, please open an issue in the GitHub repository or contact the development team.
