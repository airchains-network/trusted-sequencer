# Trusted Sequencer

A trusted sequencer implementation for rollups.

## Features

- Transaction ordering and sequencing
- Data availability layer integration (Celestia/Avail)
- State management and EVM compatibility
- High-performance transaction processing
- Configurable through command-line flags
- JSON-RPC and WebSocket interfaces with EVM chain subscription support

## Prerequisites

- Go 1.21 or later
- Access to a DA layer node (Avail or Celestia)
- A Geth node running locally or remotely

## Installation

```bash
git clone https://github.com/airchains-network/trusted-sequencer.git
cd trusted-sequencer
go build -o build/trusted-sequencer cmd/sequencer/main.go
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
  --geth.ws-url <geth-ws-url> \
  --rpc.port <rpc-port> \
  --ws.port <ws-port>
```

All DA layer configuration values are required:
- `--da.type`: DA layer type (avail/celestia)
- `--da.node-addr`: DA node address
- `--da.auth-token`: DA auth token
- `--da.namespace`: DA namespace
- `--rollup.id`: Rollup ID

Optional configuration:
- `--geth.rpc-url`: Geth RPC URL (default: http://localhost:8545)
- `--geth.ws-url`: Geth WebSocket URL (default: ws://localhost:8546)
- `--rpc.port`: RPC server port (default: :11111)
- `--ws.port`: WebSocket server port (default: :11112)

### Start the Sequencer

```bash
./build/trusted-sequencer start
```

## API Interfaces

The trusted sequencer provides two interfaces for interacting with the system:

### JSON-RPC API

The JSON-RPC API is available at the root endpoint:

```bash
curl -X POST http://localhost:11111 \
  -H "Content-Type: application/json" \
  -d '{"jsonrpc":"2.0","method":"eth_sendRawTransaction","params":["0x..."],"id":1}'
```

### WebSocket API

The WebSocket API is available at the root endpoint and supports EVM chain subscriptions:

```javascript
// JavaScript example
const ws = new WebSocket('ws://localhost:11112');

ws.onopen = function() {
  console.log('Connected to WebSocket');
  
  // Send a transaction
  ws.send(JSON.stringify({
    jsonrpc: '2.0',
    method: 'eth_sendRawTransaction',
    params: ['0x...'],
    id: 1
  }));
  
  // Subscribe to new blocks
  ws.send(JSON.stringify({
    jsonrpc: '2.0',
    method: 'eth_subscribe',
    params: ['newHeads'],
    id: 2
  }));
  
  // Subscribe to logs with filter
  ws.send(JSON.stringify({
    jsonrpc: '2.0',
    method: 'eth_subscribe',
    params: ['logs', {
      address: '0x1234...',
      topics: ['0xabcd...']
    }],
    id: 3
  }));
};

ws.onmessage = function(event) {
  const data = JSON.parse(event.data);
  console.log('Received:', data);
  
  // Handle subscription notifications
  if (data.method === 'eth_subscription') {
    const subscriptionId = data.params.subscription;
    const result = data.params.result;
    
    if (result.hash) {
      console.log('New block:', result.hash, result.number);
    } else if (result.address) {
      console.log('New log:', result);
    }
  }
};
```

#### Supported WebSocket Subscriptions

1. **New Block Headers** (`newHeads`):
   - Subscribe to new block headers as they are mined
   - Returns block hash and number

2. **Logs** (`logs`):
   - Subscribe to logs matching specific criteria
   - Supports filtering by:
     - `address`: Contract address (single or array)
     - `topics`: Event topics (array of topics)
     - `fromBlock`: Starting block (number or "latest"/"earliest"/"pending")
     - `toBlock`: Ending block (number or "latest"/"earliest"/"pending")

3. **Unsubscribe**:
   - Cancel a subscription using the subscription ID:
   ```javascript
   ws.send(JSON.stringify({
     jsonrpc: '2.0',
     method: 'eth_unsubscribe',
     params: ['subscription_id'],
     id: 4
   }));
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
geth_ws_url = "ws://localhost:8546"
rpc_port = ":11111"
ws_port = ":11112"

[genesis]
file_path = "~/.trusted-sequencer/genesis.json"
```

## Architecture

The sequencer consists of several key components:

- **Transaction Pool**: Manages incoming transactions
- **Batch Processor**: Groups transactions into batches
- **State Manager**: Maintains the rollup state
- **DA Client**: Handles data availability submissions
- **Proxy Server**: Provides JSON-RPC and WebSocket interfaces with EVM chain subscription support

## Development

### Project Structure

```
.
├── cmd/
│   └── sequencer/          # Main application entry point
├── config/                 # Configuration management
├── da/                    # Data availability layer integration
│   ├── avail/            # Avail-specific implementation
│   └── celestia/         # Celestia-specific implementation
├── proxy/                 # Proxy server implementation
├── sequencer/             # Core sequencer logic
│   ├── batch/            # Batch management
│   ├── state/            # State management
│   └── txn/              # Transaction management
└── types/                 # Common types and interfaces
```

### Building

```bash
go build -o build/trusted-sequencer cmd/sequencer/main.go
```

### Testing

```bash
go test ./...
```

## License

This project is licensed under the MIT License - see the LICENSE file for details.
