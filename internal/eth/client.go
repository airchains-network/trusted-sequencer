package eth

import (
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/ethereum/go-ethereum/rpc"
)

// Client wraps both rpc.Client and ethclient.Client for Ethereum interactions
type Client struct {
	Rpc *rpc.Client
	Eth *ethclient.Client
}

// NewClient initializes a new Ethereum client with both RPC and ethclient
func NewClient(url string) (*Client, error) {
	rpcClient, err := rpc.Dial(url)
	if err != nil {
		return nil, err
	}

	ethClient, err := ethclient.Dial(url)
	if err != nil {
		rpcClient.Close()
		return nil, err
	}

	return &Client{
		Rpc: rpcClient,
		Eth: ethClient,
	}, nil
}
