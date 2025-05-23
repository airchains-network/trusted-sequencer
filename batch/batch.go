package batch

import (
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math/big"
	"strings"
	"sync"
	"time"

	"github.com/airchains-network/trusted-sequencer/batch/da"
	"github.com/airchains-network/trusted-sequencer/db"
	"github.com/airchains-network/trusted-sequencer/eth"
	"github.com/airchains-network/trusted-sequencer/junction"
	"github.com/airchains-network/trusted-sequencer/prover"
	"github.com/airchains-network/trusted-sequencer/prover/types"
	"github.com/airchains-network/trusted-sequencer/state"
	"github.com/ethereum/go-ethereum/common"
	ethtypes "github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/rlp"
	"github.com/sirupsen/logrus"
)

// Batch represents a group of 128 transactions with DA commitment
type Batch struct {
	Transactions           []string      `json:"transactions"`
	ExecutionTraces        []string      `json:"execution_traces"`
	FromBalancesPrevBlock  []string      `json:"from_balances_prev_block"`
	ToBalancesCurrentBlock []string      `json:"to_balances_current_block"`
	PrevMerkleHash         string        `json:"prev_merkle_hash"`
	CurrentMerkleHash      string        `json:"current_merkle_hash"`
	PreviousStateRoot      string        `json:"previous_state_root"`
	CurrentStateRoot       string        `json:"current_state_root"`
	BatchNo                int           `json:"batch_no"`
	Metadata               BatchMetadata `json:"metadata"`
	Commitment             string        `json:"commitment"`
	DACommitment           string        `json:"da_commitment"`
	DABlockHash            string        `json:"da_block_hash"`
	DATxHash               string        `json:"da_tx_hash"`
	DAProvider             string        `json:"da_provider"`
	Submitted              bool          `json:"submitted"`
	Verified               bool          `json:"verified"`
}

// BatchMetadata holds metadata for a batch
type BatchMetadata struct {
	TotalGasUsed    uint64 `json:"total_gas_used"`
	Timestamp       int64  `json:"timestamp"`
	RollupNamespace string `json:"rollup_namespace"`
}

// txData holds transaction data for batching
type txData struct {
	txHex                 string
	trace                 string
	gas                   uint64
	time                  int64
	fromBalancePrevBlock  string
	toBalanceCurrentBlock string
}

func convertToBatchStruct(batch *Batch, log *logrus.Logger) types.BatchStruct {
	var from, to, amounts, txHashes []string

	for i, txHex := range batch.Transactions {
		// Remove "0x" prefix and decode hex string
		txHex = strings.TrimPrefix(txHex, "0x")
		txBytes, err := hex.DecodeString(txHex)
		if err != nil {
			log.Errorf("Failed to decode transaction hex at index %d: %v", i, err)
			continue // Skip invalid transaction
		}

		// Decode RLP-encoded transaction
		var tx ethtypes.Transaction
		if err := rlp.DecodeBytes(txBytes, &tx); err != nil {
			log.Errorf("Failed to decode RLP transaction at index %d: %v", i, err)
			continue // Skip invalid transaction
		}

		// Get sender address
		signer := ethtypes.LatestSignerForChainID(tx.ChainId())
		fromAddr, err := signer.Sender(&tx)
		if err != nil {
			log.Errorf("Failed to get sender for transaction at index %d: %v", i, err)
			from = append(from, "0x0") // Default to zero address
		} else {
			from = append(from, fromAddr.Hex())
		}

		// Get recipient address
		toAddr := tx.To()
		if toAddr == nil {
			to = append(to, "0x0") // Contract creation, no recipient
		} else {
			to = append(to, toAddr.Hex())
		}

		// Get amount (value in wei)
		amounts = append(amounts, tx.Value().String())

		// Get transaction hash
		txHashes = append(txHashes, tx.Hash().Hex())
	}

	return types.BatchStruct{
		From:              from,
		To:                to,
		Amounts:           amounts,
		TransactionHash:   txHashes,
		SenderBalances:    batch.FromBalancesPrevBlock,
		ReceiverBalances:  batch.ToBalancesCurrentBlock,
		PreStateRoot:      batch.PreviousStateRoot,
		PostStateRoot:     batch.CurrentStateRoot,
		TxMerkleRoot:      batch.CurrentMerkleHash,
		ExpectedBatchHash: batch.Commitment,
	}
}

// ProcessBlocks processes Ethereum blocks and creates batches
func ProcessBlocks(client *eth.Client, junctionClient *junction.JunctionClient, proverClient *prover.ProverClient, txnDB, batchDB db.DB, daClient da.DAClient, evmState *state.EVMState, vmProcessor *state.Processor, rollupNamespace string, log *logrus.Logger) {
	// Load last processed block
	lastBlockBytes, err := batchDB.Get([]byte("last_block"))
	if err != nil {
		log.Warnf("Failed to get last block from database: %v, starting from block 1", err)
		lastBlockBytes = nil
	}

	lastBlock := int64(1)
	if lastBlockBytes != nil {
		blockNum := big.NewInt(0).SetBytes(lastBlockBytes)
		if !blockNum.IsInt64() {
			log.Warnf("Block number %s is too large for int64, defaulting to 1", blockNum.String())
		} else {
			lastBlock = blockNum.Int64()
		}
	}

	log.Infof("Starting from block %d", lastBlock)

	// Load partial batch
	partialBatchBytes, err := batchDB.Get([]byte("partial_batch"))
	if err != nil {
		log.Warnf("Failed to load partial batch: %v", err)
	}
	var partialBatch Batch
	if partialBatchBytes != nil {
		if err := json.Unmarshal(partialBatchBytes, &partialBatch); err != nil {
			log.Errorf("Failed to unmarshal partial batch: %v", err)
		}
	}

	// Initialize batch number with validation
	batchNo := 1
	if partialBatch.BatchNo > 0 {
		// Find the highest batch number in the database
		maxBatchNo := 0
		for i := 1; ; i++ {
			key := []byte(fmt.Sprintf("batch_%d", i))
			if _, err := batchDB.Get(key); err != nil {
				break
			}
			maxBatchNo = i
		}
		if partialBatch.BatchNo <= maxBatchNo {
			log.Warnf("Partial batch number %d is not greater than last saved batch %d, using %d", partialBatch.BatchNo, maxBatchNo, maxBatchNo+1)
			batchNo = maxBatchNo + 1
		} else {
			batchNo = partialBatch.BatchNo
		}
	}

	// Initialize buffers and mutex
	var mu sync.Mutex
	txBuffer := partialBatch.Transactions
	traceBuffer := partialBatch.ExecutionTraces
	fromBalanceBuffer := partialBatch.FromBalancesPrevBlock
	toBalanceBuffer := partialBatch.ToBalancesCurrentBlock
	totalGas := partialBatch.Metadata.TotalGasUsed
	timestamp := partialBatch.Metadata.Timestamp

	// Channel for transaction data
	txChan := make(chan txData, 128)

	// Goroutine to handle batch creation
	go func() {
		for {
			// Collect transactions from channel
			if len(txBuffer) < 128 {
				select {
				case tx := <-txChan:
					mu.Lock()
					txBuffer = append(txBuffer, tx.txHex)
					traceBuffer = append(traceBuffer, tx.trace)
					fromBalanceBuffer = append(fromBalanceBuffer, tx.fromBalancePrevBlock)
					toBalanceBuffer = append(toBalanceBuffer, tx.toBalanceCurrentBlock)
					totalGas += tx.gas
					timestamp = tx.time
					mu.Unlock()
					log.Infof("Added tx to batch #%d: %d/128 txns", batchNo, len(txBuffer))
				case <-time.After(5 * time.Second):
					mu.Lock()
					if len(txBuffer) > 0 {
						log.Infof("Waiting for batch #%d: %d/128 txns collected", batchNo, len(txBuffer))
					}
					mu.Unlock()
				}
			}

			// Create batch when full
			if len(txBuffer) >= 128 {
				log.Infof("Creating batch #%d with 128 txns", batchNo)
				mu.Lock()
				batch := CreateBatch(client, batchDB, daClient, batchNo, txBuffer[:128], traceBuffer[:128], fromBalanceBuffer[:128], toBalanceBuffer[:128], totalGas, timestamp, evmState, rollupNamespace, log)
				log.Debugf("Batch #%d data: TxCount=%d, PrevHash=%s, BatchHash=%s, PreStateRoot=%s, PostStateRoot=%s, Commitment=%s, DAProvider=%s, DACommitment=%s, Gas=%d, Time=%d",
					batch.BatchNo, len(batch.Transactions), batch.PrevMerkleHash, batch.CurrentMerkleHash, batch.PreviousStateRoot, batch.CurrentStateRoot, batch.Commitment, batch.DAProvider, batch.DACommitment, batch.Metadata.TotalGasUsed, batch.Metadata.Timestamp)

				txBuffer = txBuffer[128:]
				traceBuffer = traceBuffer[128:]
				fromBalanceBuffer = fromBalanceBuffer[128:]
				toBalanceBuffer = toBalanceBuffer[128:]
				totalGas = 0
				timestamp = 0
				mu.Unlock()

				ctx := context.Background()
				junctionClient.SubmitBatchMetadata(ctx, uint64(batch.BatchNo), rollupNamespace, batch.DAProvider, batch.DACommitment, batch.DABlockHash, batch.DATxHash, rollupNamespace)
				batch.Submitted = true

				batchStruct := convertToBatchStruct(&batch, log)
				proofData, err := proverClient.ProverGenerate(context.Background(), batchStruct, rollupNamespace, batch.PreviousStateRoot, batch.CurrentStateRoot, batch.CurrentMerkleHash, batch.DACommitment)
				if err != nil {
					log.Errorf("Failed to generate proof for batch #%d: %v", batchNo, err)
					continue
				}
				log.Infof("Successfully generated proof for batch #%d", batchNo)
				proofBytes, err := json.Marshal(proofData)
				if err != nil {
					log.Errorf("Failed to marshal proof data for batch #%d: %v", batchNo, err)
					continue
				}
				if err := batchDB.Put([]byte(fmt.Sprintf("proof_%d", batchNo)), proofBytes); err != nil {
					log.Errorf("Failed to store proof for batch #%d: %v", batchNo, err)
					continue
				}
				junctionClient.SubmitBatch(context.Background(), uint64(batch.BatchNo), rollupNamespace, batch.CurrentMerkleHash, batch.PrevMerkleHash, proofData.ProofData, proofData.PublicInputs)
				batch.Verified = true

				mu.Lock()
				batchNo++
				mu.Unlock()
			}

			// Save partial batch progress
			if len(txBuffer) > 0 {
				mu.Lock()
				partial := Batch{
					Transactions:           txBuffer,
					ExecutionTraces:        traceBuffer,
					FromBalancesPrevBlock:  fromBalanceBuffer,
					ToBalancesCurrentBlock: toBalanceBuffer,
					BatchNo:                batchNo,
					Metadata:               BatchMetadata{TotalGasUsed: totalGas, Timestamp: timestamp, RollupNamespace: rollupNamespace},
				}
				partialBytes, err := json.Marshal(partial)
				if err != nil {
					log.Errorf("Failed to marshal partial batch #%d: %v", batchNo, err)
					mu.Unlock()
					continue
				}
				if err := batchDB.Put([]byte("partial_batch"), partialBytes); err != nil {
					log.Errorf("Failed to save partial batch #%d: %v", batchNo, err)
					mu.Unlock()
					continue
				}
				mu.Unlock()
			}
		}
	}()

	// Main block indexing loop
	for {
		// Get latest block number
		latestBlock, err := client.Eth.BlockNumber(context.Background())
		if err != nil {
			log.Warnf("Failed to fetch latest block number: %v, retrying in 5s", err)
			time.Sleep(5 * time.Second)
			continue
		}
		log.Debugf("Latest block number: %d", latestBlock)
		if uint64(lastBlock) > latestBlock {
			log.Infof("Reached latest block %d, waiting for new blocks", latestBlock)
			time.Sleep(5 * time.Second)
			continue
		}

		block, err := client.Eth.BlockByNumber(context.Background(), big.NewInt(lastBlock))
		if err != nil {
			log.Warnf("Failed to fetch block %d: %v, retrying every 5s", lastBlock, err)
			time.Sleep(5 * time.Second)
			continue
		}

		// Validate block
		if block == nil || block.Number() == nil || block.Hash() == (common.Hash{}) {
			log.Errorf("Block %d is invalid: Number=%v, Hash=%s", lastBlock, block.Number(), block.Hash().Hex())
			time.Sleep(5 * time.Second)
			continue
		}

		txns := block.Transactions()

		for _, tx := range txns {
			txHex, err := rlp.EncodeToBytes(tx)
			if err != nil {
				log.Errorf("Failed to encode transaction %s: %v", tx.Hash().Hex(), err)
				continue
			}
			txStr := hex.EncodeToString(txHex)

			vmProcessor.ProcessTransaction(tx, block.Number().String())

			if err := txnDB.Put([]byte("tx_"+tx.Hash().Hex()), []byte(txStr)); err != nil {
				log.Errorf("Failed to store transaction %s in txnDB: %v", tx.Hash().Hex(), err)
				continue
			}

			var trace json.RawMessage
			if err := client.Rpc.Call(&trace, "debug_traceTransaction", tx.Hash().Hex()); err != nil {
				log.Errorf("Failed to fetch trace for tx %s, skipping transaction: %v", tx.Hash().Hex(), err)
				continue
			}
			if len(trace) == 0 {
				log.Errorf("Empty trace for tx %s, skipping transaction", tx.Hash().Hex())
				continue
			}

			// Fetch sender's balance from previous block
			var fromBalancePrevBlock string
			fromAddr, err := client.Eth.TransactionSender(context.Background(), tx, block.Hash(), 0)
			if err != nil {
				log.Errorf("Failed to get sender for tx %s, skipping transaction: %v", tx.Hash().Hex(), err)
				continue
			}
			prevBlockNum := big.NewInt(lastBlock - 1)
			if prevBlockNum.Sign() <= 0 {
				fromBalancePrevBlock = "0x0"
			} else {
				balance, err := client.Eth.BalanceAt(context.Background(), fromAddr, prevBlockNum)
				if err != nil {
					log.Errorf("Failed to fetch balance for %s at block %d, skipping transaction: %v", fromAddr.Hex(), prevBlockNum, err)
					continue
				}
				fromBalancePrevBlock = "0x" + balance.Text(16)
			}

			// Fetch recipient's balance from current block
			var toBalanceCurrentBlock string
			toAddr := tx.To()
			if toAddr == nil {
				toBalanceCurrentBlock = "0x0"
			} else {
				balance, err := client.Eth.BalanceAt(context.Background(), *toAddr, big.NewInt(lastBlock))
				if err != nil {
					log.Errorf("Failed to fetch balance for %s at block %d, skipping transaction: %v", toAddr.Hex(), lastBlock, err)
					continue
				}
				toBalanceCurrentBlock = "0x" + balance.Text(16)
			}

			// Send tx data to batching goroutine
			txChan <- txData{
				txHex:                 txStr,
				trace:                 string(trace),
				gas:                   tx.Gas(),
				time:                  int64(block.Time()),
				fromBalancePrevBlock:  fromBalancePrevBlock,
				toBalanceCurrentBlock: toBalanceCurrentBlock,
			}
		}

		// Save block progress
		lastBlock++
		if err := batchDB.Put([]byte("last_block"), big.NewInt(lastBlock).Bytes()); err != nil {
			log.Errorf("Failed to save last block %d: %v", lastBlock, err)
		}
	}
}

// ComputeStateRoots computes the pre- and post-state roots for a batch
func ComputeStateRoots(client *eth.Client, txns []string, blockNumber *big.Int, log *logrus.Logger) (preStateRoot, postStateRoot string, err error) {
	block, err := client.Eth.BlockByNumber(context.Background(), blockNumber)
	if err != nil {
		return "", "", fmt.Errorf("failed to fetch block %s: %v", blockNumber.String(), err)
	}
	preStateRoot = block.Root().Hex()
	log.Debugf("Pre-state root for block %s: %s", blockNumber.String(), preStateRoot)

	latestBlock, err := client.Eth.BlockByNumber(context.Background(), nil)
	if err != nil {
		return "", "", fmt.Errorf("failed to fetch latest block: %v", err)
	}
	postStateRoot = latestBlock.Root().Hex()
	log.Debugf("Post-state root (simulated) for batch: %s", postStateRoot)

	return preStateRoot, postStateRoot, nil
}

// ComputeBatchHash computes the hash of the transaction Merkle root and post-state root
func ComputeBatchHash(txMerkleRoot, postStateRoot string, log *logrus.Logger) string {
	txMerkleRoot = strings.TrimPrefix(txMerkleRoot, "0x")
	postStateRoot = strings.TrimPrefix(postStateRoot, "0x")

	txRootBytes, err := hex.DecodeString(txMerkleRoot)
	if err != nil {
		log.Errorf("Failed to decode tx Merkle root: %v", err)
		return "0x0"
	}
	stateRootBytes, err := hex.DecodeString(postStateRoot)
	if err != nil {
		log.Errorf("Failed to decode state root: %v", err)
		return "0x0"
	}

	combined := append(txRootBytes, stateRootBytes...)
	hash := sha256.Sum256(combined)
	return "0x" + hex.EncodeToString(hash[:])
}

// ComputeBatchCommitment creates a commitment from batch hash and metadata
func ComputeBatchCommitment(batchHash string, metadata BatchMetadata, log *logrus.Logger) string {
	metadataBytes, err := json.Marshal(metadata)
	if err != nil {
		log.Errorf("Failed to marshal metadata: %v", err)
		return "0x0"
	}

	batchHash = strings.TrimPrefix(batchHash, "0x")
	batchHashBytes, err := hex.DecodeString(batchHash)
	if err != nil {
		log.Errorf("Failed to decode batch hash: %v", err)
		return "0x0"
	}

	combined := append(batchHashBytes, metadataBytes...)
	hash := sha256.Sum256(combined)
	return "0x" + hex.EncodeToString(hash[:])
}

// CreateBatch constructs a new batch with state computation, DA commitment, and batch commitment
func CreateBatch(client *eth.Client, batchDB db.DB, daClient da.DAClient, batchNo int, txns, traces, fromBalances, toBalances []string, gas uint64, timestamp int64, evmState *state.EVMState, rollupNamespace string, log *logrus.Logger) Batch {
	prevHashBytes, _ := batchDB.Get([]byte(fmt.Sprintf("batch_%d_hash", batchNo-1)))
	prevHash := string(prevHashBytes)
	if batchNo == 1 {
		prevHash = "0x0000000000000000000000000000000000000000000000000000000000000000"
	}

	// Compute transaction Merkle root
	txMerkleRoot := computeMerkleRoot(txns)

	// Fetch state roots
	preStateRootBytes, _ := batchDB.Get([]byte(fmt.Sprintf("state_%d", batchNo-1)))
	preStateRoot := string(preStateRootBytes)

	accounts, err := evmState.GetAllAccounts()
	if err != nil {
		log.Printf("Failed to get accounts: %v", err)
	}
	postStateRoot := state.CalculateStateHashSPT(accounts)

	// Compute batch hash (transactions + state root)
	batchHash := ComputeBatchHash(txMerkleRoot, postStateRoot, log)

	// Create metadata
	metadata := BatchMetadata{TotalGasUsed: gas, Timestamp: timestamp, RollupNamespace: rollupNamespace}

	// Compute batch commitment (batch hash + metadata)
	commitment := ComputeBatchCommitment(batchHash, metadata, log)

	// Submit batch transaction data to Celestia DA
	txData := []byte(strings.Join(txns, ""))

	compressedBatchData, err := compressData(txData)
	if err != nil {
		log.Errorf("Failed to compress batch #%d tx data: %v", batchNo, err)
		compressedBatchData = []byte{}
	}
	var daType string
	switch daClient.(type) {
	case *da.CelestiaClient:
		daType = "Celestia"
	case *da.AvailClient:
		daType = "Avail"
	default:
		daType = "Unknown"
	}

	daBlockHash, daTxHash, daCommitment, err := daClient.SubmitToDA(compressedBatchData, log)
	if err != nil {
		log.Errorf("Failed to submit batch #%d tx data to Celestia DA: %v", batchNo, err)
		daCommitment = "0x0"
	}

	return Batch{
		Transactions:           txns,
		ExecutionTraces:        traces,
		FromBalancesPrevBlock:  fromBalances,
		ToBalancesCurrentBlock: toBalances,
		PrevMerkleHash:         prevHash,
		CurrentMerkleHash:      batchHash,
		PreviousStateRoot:      preStateRoot,
		CurrentStateRoot:       postStateRoot,
		BatchNo:                batchNo,
		Metadata:               metadata,
		Commitment:             commitment,
		DAProvider:             daType,
		DACommitment:           daCommitment,
		DABlockHash:            daBlockHash,
		DATxHash:               daTxHash,
	}
}

// computeMerkleRoot calculates the SHA-256 Merkle root of a transaction list
func computeMerkleRoot(txns []string) string {
	if len(txns) == 0 {
		return "0x0"
	}
	hashes := make([][]byte, len(txns))
	for i, tx := range txns {
		hash := sha256.Sum256([]byte(tx))
		hashes[i] = hash[:]
	}
	for len(hashes) > 1 {
		var temp [][]byte
		for i := 0; i < len(hashes); i += 2 {
			if i+1 < len(hashes) {
				combined := append(hashes[i], hashes[i+1]...)
				hash := sha256.Sum256(combined)
				temp = append(temp, hash[:])
			} else {
				temp = append(temp, hashes[i])
			}
		}
		hashes = temp
	}
	return "0x" + hex.EncodeToString(hashes[0])
}

func compressData(data []byte) ([]byte, error) {
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	if _, err := gz.Write(data); err != nil {
		return nil, err
	}
	if err := gz.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// SaveBatch stores a batch in the batch database
func SaveBatch(db db.DB, batch Batch, log *logrus.Logger) {
	batchBytes, err := json.Marshal(batch)
	if err != nil {
		log.Errorf("Failed to marshal batch #%d: %v", batch.BatchNo, err)
		return
	}
	key := []byte(fmt.Sprintf("batch_%d", batch.BatchNo))
	if err := db.Put(key, batchBytes); err != nil {
		log.Errorf("Failed to save batch #%d: %v", batch.BatchNo, err)
	} else {
		log.Infof("Saved batch #%d to batchDB with DA commitment: %s", batch.BatchNo, batch.DACommitment)
	}
	db.Put([]byte(fmt.Sprintf("batch_%d_hash", batch.BatchNo)), []byte(batch.CurrentMerkleHash))
}
