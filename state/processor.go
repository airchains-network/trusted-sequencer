package state

import (
	"encoding/json"
	"fmt"
	"math/big"
	"os"
	"strings"

	"github.com/airchains-network/trusted-sequencer/eth"
	"github.com/airchains-network/trusted-sequencer/types"
	ethtypes "github.com/ethereum/go-ethereum/core/types"
	"github.com/sirupsen/logrus"
)

type Block struct {
	Number        string                `json:"number"`
	Hash          string                `json:"hash"`
	Miner         string                `json:"miner"`
	Transactions  ethtypes.Transactions `json:"transactions"`
	GasUsed       string                `json:"gasUsed"`
	BaseFeePerGas string                `json:"baseFeePerGas"`
}

type Genesis struct {
	Config map[string]interface{} `json:"config"`
	Alloc  map[string]struct {
		Balance string `json:"balance"`
	} `json:"alloc"`
	Extradata string `json:"extradata"`
}

type Processor struct {
	state *EVMState
	rpc   *eth.RpcClient
	log   *logrus.Logger
}

func NewProcessor(state *EVMState, rpcClient *eth.RpcClient, log *logrus.Logger) *Processor {
	return &Processor{
		state: state,
		rpc:   rpcClient,
		log:   log,
	}
}

func (p *Processor) LoadGenesis(genesisPath string) (string, error) {
	data, err := os.ReadFile(genesisPath)
	if err != nil {
		return "", fmt.Errorf("failed to read genesis.json: %v", err)
	}

	var genesis Genesis
	if err := json.Unmarshal(data, &genesis); err != nil {
		return "", fmt.Errorf("failed to parse genesis.json: %v", err)
	}

	signer, err := extractMinerFromExtradata(genesis.Extradata)
	if err != nil {
		return "", fmt.Errorf("failed to parse signer from extradata: %v", err)
	}

	for addr, alloc := range genesis.Alloc {
		if !strings.HasPrefix(addr, "0x") {
			addr = "0x" + addr
		}
		acc := &types.EVMAccount{
			Balance: eth.HexToBigInt(alloc.Balance),
			Nonce:   0,
			Storage: make(map[string]map[string]string),
		}
		fmt.Printf("Saving genesis account %s\n", addr)
		if err := p.state.CreateAccount(strings.ToLower(addr), acc.Balance); err != nil {
			return "", fmt.Errorf("failed to save genesis account %s: %v", addr, err)
		}
		fmt.Printf("Saved genesis account %s\n", addr)
	}

	if _, err := p.state.GetAccount(signer); err != nil {
		if err := p.state.CreateAccount(signer, big.NewInt(0)); err != nil {
			return "", fmt.Errorf("failed to initialize signer account: %v", err)
		}
	}
	fmt.Printf("Initialized signer account %s\n", signer)
	accounts, err := p.state.GetAllAccounts()
	if err != nil {
		return "", fmt.Errorf("failed to get all accounts: %v", err)
	}
	hash := CalculateStateHashSPT(accounts)
	fmt.Printf("Genesis state hash: %s\n", hash)
	return hash, nil
}

func (p *Processor) ProcessTransaction(tx *ethtypes.Transaction, blockNumber string) error {
	if tx == nil {
		return fmt.Errorf("transaction is nil")
	}

	if blockNumber == "" {
		return fmt.Errorf("block number is empty")
	}

	// Get sender address
	from, err := ethtypes.Sender(ethtypes.LatestSignerForChainID(tx.ChainId()), tx)
	if err != nil {
		return fmt.Errorf("failed to get sender address: %v", err)
	}

	fromAcc, err := p.state.GetAccount(from.Hex())
	if err != nil {
		return fmt.Errorf("failed to get sender account: %v", err)
	}

	// Handle EIP-1559 transactions
	var gasPrice *big.Int
	if tx.Type() == 2 { // EIP-1559
		baseFee := tx.GasFeeCap()
		priorityFee := tx.GasTipCap()
		gasPrice = new(big.Int).Add(baseFee, priorityFee)
	} else {
		gasPrice = tx.GasPrice()
	}

	gasLimit := new(big.Int).SetUint64(tx.Gas())
	value := tx.Value()

	gasUsed, err := p.rpc.GetGasUsed(tx.Hash().Hex())
	if err != nil {
		return fmt.Errorf("failed to get gas used for tx %s: %v", tx.Hash().Hex(), err)
	}

	// Validate gas usage
	if gasUsed.Cmp(gasLimit) > 0 {
		return fmt.Errorf("gas used %s exceeds gas limit %s", gasUsed.String(), gasLimit.String())
	}

	gasCost := new(big.Int).Mul(gasUsed, gasPrice)
	totalCost := new(big.Int).Add(value, gasCost)
	if fromAcc.Balance.Cmp(totalCost) < 0 {
		return fmt.Errorf("insufficient balance: have %s, need %s", fromAcc.Balance.String(), totalCost.String())
	}

	// Update sender account
	fromAcc.Balance.Sub(fromAcc.Balance, totalCost)
	fromAcc.Nonce++

	// Handle gas refund
	refund := new(big.Int).Sub(gasLimit, gasUsed)
	if refund.Sign() > 0 {
		refund.Mul(refund, gasPrice)
		fromAcc.Balance.Add(fromAcc.Balance, refund)
	}

	if err := p.state.UpdateAccount(from.Hex(), fromAcc.Balance, fromAcc.Nonce); err != nil {
		return fmt.Errorf("failed to update sender account: %v", err)
	}

	var targetAddr string
	if tx.To() == nil {
		// Contract creation
		targetAddr = ComputeContractAddress(from.Hex(), fromAcc.Nonce-1)
		contractAcc := &types.EVMAccount{
			Balance: value,
			Nonce:   0,
			Storage: make(map[string]map[string]string),
		}

		if len(tx.Data()) > 0 {
			contractAcc.Code = tx.Data()

			storage, code, err := p.rpc.FetchContractState(tx.Hash().Hex(), targetAddr, blockNumber)
			if err != nil {
				p.log.Warnf("Failed to fetch contract state for %s: %v", targetAddr, err)
				// Continue with empty storage
			} else {
				contractAcc.Code = code
				for k, v := range storage {
					if err := p.state.SetStorage(targetAddr, targetAddr, k, v); err != nil {
						p.log.Warnf("Failed to set storage for %s: %v", targetAddr, err)
					}
				}
			}
		}

		if err := p.state.CreateAccount(targetAddr, value); err != nil {
			return fmt.Errorf("failed to create contract account: %v", err)
		}
	} else {
		// Contract call or value transfer
		toAcc, err := p.state.GetAccount(tx.To().Hex())
		if err != nil {
			return fmt.Errorf("failed to get recipient account: %v", err)
		}

		if toAcc.Balance.Cmp(big.NewInt(0)) == 0 && toAcc.Nonce == 0 {
			if err := p.state.CreateAccount(tx.To().Hex(), value); err != nil {
				return fmt.Errorf("failed to create recipient account: %v", err)
			}
		} else {
			toAcc.Balance.Add(toAcc.Balance, value)
			if len(toAcc.Code) > 0 && len(tx.Data()) > 0 {
				storage, code, err := p.rpc.FetchContractState(tx.Hash().Hex(), tx.To().Hex(), blockNumber)
				if err != nil {
					p.log.Warnf("Failed to fetch contract state for %s: %v", tx.To().Hex(), err)
					// Continue with existing storage
				} else {
					toAcc.Code = code
					for k, v := range storage {
						if err := p.state.SetStorage(tx.To().Hex(), tx.To().Hex(), k, v); err != nil {
							p.log.Warnf("Failed to set storage for %s: %v", tx.To().Hex(), err)
						}
					}
				}
			}
			if err := p.state.UpdateAccount(tx.To().Hex(), toAcc.Balance, toAcc.Nonce); err != nil {
				return fmt.Errorf("failed to update recipient account: %v", err)
			}
		}
	}

	return nil
}

func (p *Processor) ProcessBlockRewards(block Block, signer string) error {
	if block.Number == "" {
		return fmt.Errorf("block number is empty")
	}

	if signer == "" {
		return fmt.Errorf("signer address is empty")
	}

	totalPriorityFees := big.NewInt(0)
	baseFee := eth.HexToBigInt(block.BaseFeePerGas)
	if baseFee == nil {
		return fmt.Errorf("invalid base fee")
	}

	for _, tx := range block.Transactions {
		gasUsed, err := p.rpc.GetGasUsed(tx.Hash().Hex())
		if err != nil {
			p.log.Warnf("Failed to get gas used for tx %s: %v", tx.Hash().Hex(), err)
			continue
		}

		var gasPrice *big.Int
		if tx.Type() == 2 { // EIP-1559
			priorityFee := tx.GasTipCap()
			if priorityFee == nil {
				p.log.Warnf("Invalid priority fee for tx %s", tx.Hash().Hex())
				continue
			}
			gasPrice = new(big.Int).Add(baseFee, priorityFee)
		} else {
			gasPrice = tx.GasPrice()
			if gasPrice == nil {
				p.log.Warnf("Invalid gas price for tx %s", tx.Hash().Hex())
				continue
			}
		}

		priorityFee := new(big.Int).Sub(gasPrice, baseFee)
		if priorityFee.Sign() < 0 {
			priorityFee = big.NewInt(0)
		}

		txPriorityFee := new(big.Int).Mul(gasUsed, priorityFee)
		totalPriorityFees.Add(totalPriorityFees, txPriorityFee)
	}

	signerAcc, err := p.state.GetAccount(signer)
	if err != nil {
		return fmt.Errorf("failed to get signer account: %v", err)
	}

	signerAcc.Balance.Add(signerAcc.Balance, totalPriorityFees)
	if err := p.state.UpdateAccount(signer, signerAcc.Balance, signerAcc.Nonce); err != nil {
		return fmt.Errorf("failed to update signer account: %v", err)
	}

	p.log.Infof("Signer %s received priority fees: %s wei", signer, totalPriorityFees.String())
	return nil
}

func extractMinerFromExtradata(extradata string) (string, error) {
	extradata = strings.TrimPrefix(extradata, "0x")
	if len(extradata) < 64 {
		return "", fmt.Errorf("invalid extradata length: %d", len(extradata))
	}
	address := "0x" + extradata[64:104]
	return strings.ToLower(address), nil
}
