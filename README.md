# Trusted Sequencer

[![Go Version](https://img.shields.io/badge/Go-1.24+-00ADD8?style=flat-square&logo=go)](https://golang.org)
[![License](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)
[![Version](https://img.shields.io/badge/version-v0.1.0--beta-orange)](https://github.com/airchains-network/trusted-sequencer/releases)

The Trusted Sequencer is a critical infrastructure component for Airchains rollup solutions, designed to bridge the gap between scalability and security in blockchain networks. At its core, it acts as a sophisticated transaction orchestrator that:

🔗 **Orders & Batches Transactions**
- Groups transactions into optimized batches of 128
- Ensures deterministic execution order
- Maintains transaction finality and consistency

🏛️ **Manages State Transitions**
- Tracks and verifies EVM-compatible state changes
- Handles smart contract deployments and interactions
- Maintains account balances and nonces with cryptographic integrity

📡 **Ensures Data Availability**
- Integrates with specialized DA layers (Celestia/Avail)
- Guarantees permanent access to transaction data
- Implements robust retry mechanisms for reliable data submission

🔒 **Generates & Verifies Proofs**
- Coordinates with ZKFHE prover service for state validation
- Ensures cryptographic verification of state transitions
- Maintains proof integrity across the network

🌐 **Synchronizes Network State**
- Interfaces with Junction Network for cross-chain communication
- Coordinates with Operator nodes (ZBC Ethermint)
- Maintains network-wide consensus and state synchronization

Built in Go with a focus on performance and reliability, this implementation provides the foundational infrastructure needed for secure, scalable, and efficient Layer 2 solutions.

## Features 
  
  - Deterministic ordering with state validation
  - Does transaction batching (128 tx per batch)
  - Support for EIP-1559 transactions

- 🌐 **Data Availability Integration**
  - Support for multiple DA layers:
    - Celestia
    - Avail
  - Automatic retry mechanisms
  - Configurable namespace and parameters

- ⚡ **High Performance**
  - Parallel transaction processing
  - Optimized state management
  - LevelDB-based storage
  - WebSocket subscriptions

- 🔧 **Flexible Configuration**
  - TOML-based configuration
  - Command-line interface
  - Environment-specific settings

## Prerequisites

- Go 1.24 or later
- Access to a DA layer node (Celestia or Avail)
- Access to an Operator node (ZBC Ethermint Node By Zama)
- ZKFHE Prover Service endpoint
- Junction Network access

## Quick Start

### Installation

```bash
# Clone the repository
git clone https://github.com/airchains-network/trusted-sequencer.git
cd trusted-sequencer

# Install dependencies
make install-deps

# Build the binary
make build
```

### Configuration

1. Create your configuration file:
```bash
cp config.toml.example ~/.trusted-sequencer/config.toml
```

2. Edit the configuration:
```toml
[general]
geth_rpc_url = "http://your-geth-node:8545"
geth_ws_url = "ws://your-geth-node:8546"
rpc_port = ":11111"
ws_port = ":11112"

[da]
type = "celestia"  # or "avail"
node_addr = "http://your-da-node:26657"
auth_token = "your_auth_token"
namespace = "your_namespace"

[prover]
url = "http://your-prover-service:8081"

[junction]
account_name = "your_account"
node_api_address = "http://junction-node:1317"
node_rpc_address = "http://junction-node:26657"
```

### Initialize the Sequencer

```bash
./build/trusted-sequencer init
```

### Start the Sequencer

```bash
./build/trusted-sequencer start
```

## API Endpoints
docs
### JSON-RPC API

Available at `http://localhost:11111`

```bash
# Example: Send raw transaction
curl -X POST http://localhost:11111 \
  -H "Content-Type: application/json" \
  -d '{
    "jsonrpc":"2.0",
    "method":"eth_sendRawTransaction",
    "params":["0x..."],
    "id":1
  }'
```

### WebSocket API

Available at `ws://localhost:11112`

```javascript
// Example: Subscribe to new blocks
const ws = new WebSocket('ws://localhost:11112');
ws.send(JSON.stringify({
  jsonrpc: '2.0',
  method: 'eth_subscribe',
  params: ['newHeads'],
  id: 1
}));
```

## Architecture

```
├── batch/          # Batch processing logic
├── cmd/            # Command-line interface
├── config/         # Configuration management
├── da/             # Data availability layer
├── db/             # Database operations
├── eth/            # Ethereum client
├── junction/       # Junction network integration
├── pool/           # Transaction pool
├── prover/         # Proof generation
├── proxy/          # API server
├── state/          # State management
└── types/          # Common types
```

## Development

### Build

```bash
# Build for current platform
make build

# Build for all platforms
make build-all


```

### Test

```bash
# Run tests
make test

# Run linter
make lint
```

## Security Considerations

- This is a beta release and should be used with caution
- Secure all API endpoints in production
- Use proper authentication for DA layer access
- Regularly backup state data
- Monitor system resources and performance



## Contributing

1. Fork the repository
2. Create your feature branch
3. Commit your changes
4. Push to the branch
5. Create a Pull Request

## License

This project is licensed under the MIT License - see the [LICENSE](LICENSE) file for details.

## Support

For support and discussions:
- [GitHub Issues](https://github.com/airchains-network/trusted-sequencer/issues)
- [Documentation](https://docs.airchains.io)

---
<p align="center">Made with ❤️ by Team Airchains</p>

