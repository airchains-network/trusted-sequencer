package batch

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math/big"
	"time"

	"github.com/airchains-network/trusted-sequencer/internal/db"
	"github.com/airchains-network/trusted-sequencer/internal/eth"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/rlp"
	"github.com/sirupsen/logrus"
)

// Batch represents a group of 128 transactions
type Batch struct {
	Transactions      []string      `json:"transactions"`
	ExecutionTraces   []string      `json:"execution_traces"`
	PrevMerkleHash    string        `json:"prev_merkle_hash"`
	CurrentMerkleHash string        `json:"current_merkle_hash"`
	PreviousStateRoot string        `json:"previous_state_root"`
	CurrentStateRoot  string        `json:"current_state_root"`
	BatchNo           int           `json:"batch_no"`
	Metadata          BatchMetadata `json:"metadata"`
	Submitted         bool          `json:"submitted"`
	Verified          bool          `json:"verified"`
}

// BatchMetadata holds metadata for a batch
type BatchMetadata struct {
	TotalGasUsed uint64 `json:"total_gas_used"`
	Timestamp    int64  `json:"timestamp"`
}

// txData holds transaction data for batching
type txData struct {
	txHex string
	trace string
	gas   uint64
	time  int64
}

// ProcessBlocks processes Ethereum blocks and creates batches

func ProcessBlocks(client *eth.Client, txnDB, batchDB db.DB, log *logrus.Logger) {
	// Load last processed block
	lastBlockBytes, _ := batchDB.Get([]byte("last_block"))
	lastBlock := int64(1)
	if lastBlockBytes != nil {
		lastBlock = big.NewInt(0).SetBytes(lastBlockBytes).Int64()
	}

	// Load partial batch
	partialBatchBytes, _ := batchDB.Get([]byte("partial_batch"))
	var partialBatch Batch
	if partialBatchBytes != nil {
		json.Unmarshal(partialBatchBytes, &partialBatch)
	}

	batchNo := 1
	if partialBatch.BatchNo > 0 {
		batchNo = partialBatch.BatchNo
	}

	txBuffer := partialBatch.Transactions
	traceBuffer := partialBatch.ExecutionTraces
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
					txBuffer = append(txBuffer, tx.txHex)
					traceBuffer = append(traceBuffer, tx.trace)
					totalGas += tx.gas
					timestamp = tx.time
					log.Infof("Added tx to batch #%d: %d/128 txns", batchNo, len(txBuffer))
				case <-time.After(5 * time.Second):
					if len(txBuffer) > 0 {
						log.Infof("Waiting for batch #%d: %d/128 txns collected", batchNo, len(txBuffer))
					}
				}
			}

			// Create batch when full
			if len(txBuffer) >= 128 {
				log.Infof("Creating batch #%d with 128 txns", batchNo)
				batch := CreateBatch(client, batchDB, batchNo, txBuffer[:128], traceBuffer[:128], totalGas, timestamp, log)
				log.Debugf("Batch #%d data: TxCount=%d, PrevHash=%s, CurrentHash=%s, Gas=%d, Time=%d",
					batch.BatchNo, len(batch.Transactions), batch.PrevMerkleHash, batch.CurrentMerkleHash, batch.Metadata.TotalGasUsed, batch.Metadata.Timestamp)
				SaveBatch(batchDB, batch, log)
				batchNo++
				txBuffer = txBuffer[128:]
				traceBuffer = traceBuffer[128:]
				totalGas = 0
			}

			// Save partial batch progress
			if len(txBuffer) > 0 {
				partial := Batch{
					Transactions:    txBuffer,
					ExecutionTraces: traceBuffer,
					BatchNo:         batchNo,
					Metadata:        BatchMetadata{TotalGasUsed: totalGas, Timestamp: timestamp},
				}
				partialBytes, _ := json.Marshal(partial)
				batchDB.Put([]byte("partial_batch"), partialBytes)
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

		log.Infof("Indexing block %d", lastBlock)
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

		// Debug: Print block data
		log.Debugf("Block %d data: Hash=%s, TxCount=%d, Time=%d, GasLimit=%d",
			block.Number().Int64(), block.Hash().Hex(), len(block.Transactions()), block.Time(), block.GasLimit())

		txns := block.Transactions()
		log.Infof("Block %d has %d transactions", lastBlock, len(txns))

		for _, tx := range txns {
			txHex, _ := rlp.EncodeToBytes(tx)
			txStr := hex.EncodeToString(txHex)
			txnDB.Put([]byte("tx_"+tx.Hash().Hex()), []byte(txStr))

			var trace json.RawMessage
			if err := client.Rpc.Call(&trace, "debug_traceTransaction", tx.Hash().Hex()); err != nil {
				log.Warnf("Failed to fetch trace for tx %s: %v", tx.Hash().Hex(), err)
				trace = []byte("{}")
			}

			// Send tx data to batching goroutine
			txChan <- txData{
				txHex: txStr,
				trace: string(trace),
				gas:   tx.Gas(),
				time:  int64(block.Time()),
			}
		}

		// Save block progress
		lastBlock++
		batchDB.Put([]byte("last_block"), big.NewInt(lastBlock).Bytes())
	}
}

// CreateBatch constructs a new batch with state computation
func CreateBatch(client *eth.Client, batchDB db.DB, batchNo int, txns, traces []string, gas uint64, timestamp int64, log *logrus.Logger) Batch {
	prevHashBytes, _ := batchDB.Get([]byte(fmt.Sprintf("batch_%d_hash", batchNo-1)))
	prevHash := string(prevHashBytes)
	if batchNo == 1 {
		prevHash = "0x0000000000050521071107"
	}

	currentHash := computeMerkleRoot(txns)
	

	// Note: We’re not passing block here anymore, so no state computation for now
	// If needed, we’ll need to fetch the latest block in this goroutine
	return Batch{
		Transactions:      txns,
		ExecutionTraces:   traces,
		PrevMerkleHash:    prevHash,
		CurrentMerkleHash: currentHash,
		BatchNo:           batchNo,
		Metadata:          BatchMetadata{TotalGasUsed: gas, Timestamp: timestamp},
	}
}

// TODO ComputeStateRoot computes the state root of a batch
func ComputeStateRoot(currentMerkleRoot string, batch Batch, log *logrus.Logger) string {
	return "0x0"
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
	return hex.EncodeToString(hashes[0])
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
		log.Infof("Saved batch #%d to batchDB", batch.BatchNo)
	}
	db.Put([]byte(fmt.Sprintf("batch_%d_hash", batch.BatchNo)), []byte(batch.CurrentMerkleHash))
}
