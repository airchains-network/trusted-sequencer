package proxy

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"

	"github.com/airchains-network/trusted-sequencer/batch"
	"github.com/airchains-network/trusted-sequencer/db"
	"github.com/airchains-network/trusted-sequencer/eth"
	"github.com/airchains-network/trusted-sequencer/pool"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/rlp"
	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"
)

// DecodedTransaction represents a parsed transaction
type DecodedTransaction struct {
	Hash            string `json:"hash"`
	From            string `json:"from"`
	To              string `json:"to"`
	ContractAddress string `json:"contractAddress"`
	Value           string `json:"value"`
	Gas             uint64 `json:"gas"`
	GasPrice        string `json:"gasPrice"`
	Nonce           uint64 `json:"nonce"`
	Data            string `json:"data"`
}

// decodeTransaction converts raw hex to a DecodedTransaction
func decodeTransaction(txHex string, log *logrus.Logger) *DecodedTransaction {
	txBytes, err := hex.DecodeString(txHex)
	if err != nil {
		log.Errorf("Failed to decode tx hex: %v", err)
		return nil
	}

	var tx types.Transaction
	if err := rlp.DecodeBytes(txBytes, &tx); err != nil {
		log.Errorf("Failed to RLP decode tx: %v", err)
		return nil
	}
	signer := types.LatestSignerForChainID(tx.ChainId())

	// Handle Typed Transactions (EIP-2718)
	var sender string
	if tx.Type() == types.LegacyTxType {
		// Legacy Transactions (Pre-EIP-1559)
		from, err := types.Sender(signer, &tx)
		if err != nil {
			log.Errorf("Failed to get sender for LegacyTx: %v", err)
			return nil
		}
		sender = from.Hex()
	} else {
		// Typed Transactions (EIP-1559, EIP-2930)
		from, err := signer.Sender(&tx)
		if err != nil {
			log.Errorf("Failed to get sender for TypedTx: %v", err)
			return nil // Fixed: Return nil on error
		}
		sender = from.Hex()
	}

	to := ""
	contract := ""
	if tx.To() == nil {
		// Contract creation: Generate the contract address
		contractAddr := crypto.CreateAddress(common.HexToAddress(sender), tx.Nonce())
		contract = contractAddr.Hex()
		to = "nil"
	} else {
		to = tx.To().Hex()
		contract = "nil"
	}

	return &DecodedTransaction{
		Hash:            tx.Hash().Hex(),
		From:            sender,
		To:              to,
		ContractAddress: contract,
		Value:           "0x" + tx.Value().Text(16),
		Gas:             tx.Gas(),
		GasPrice:        "0x" + tx.GasPrice().Text(16),
		Nonce:           tx.Nonce(),
		Data:            "0x" + hex.EncodeToString(tx.Data()),
	}
}

// Start launches the GIN proxy server
func Start(port string, client *eth.Client, txnDB, batchDB db.DB, pool *pool.TxPool, log *logrus.Logger) error {
	gin.SetMode(gin.ReleaseMode) // No debug noise
	r := gin.New()

	// Add logger middleware
	r.Use(gin.LoggerWithConfig(gin.LoggerConfig{
		Formatter: func(param gin.LogFormatterParams) string {
			return fmt.Sprintf("[GIN] %s - %s %s %d\n",
				param.TimeStamp.Format("2006-01-02 15:04:05"),
				param.Method,
				param.Path,
				param.StatusCode,
			)
		},
	}))

	// Recovery middleware for panics
	r.Use(gin.Recovery())

	// Single RPC endpoint
	r.POST("/rpc", func(c *gin.Context) {
		handleRPC(c, client, txnDB, batchDB, pool, log)
	})

	log.Infof("Starting GIN server on %s", port)
	return r.Run(port)
}

// handleRPC processes incoming RPC requests
func handleRPC(c *gin.Context, client *eth.Client, txnDB, batchDB db.DB, pool *pool.TxPool, log *logrus.Logger) {
	var req struct {
		Jsonrpc string        `json:"jsonrpc"`
		Method  string        `json:"method"`
		Params  []interface{} `json:"params"`
		ID      interface{}   `json:"id"`
	}

	if err := c.BindJSON(&req); err != nil {
		log.Errorf("Failed to parse JSON-RPC request: %v", err)
		c.JSON(http.StatusBadRequest, gin.H{
			"jsonrpc": "2.0",
			"error":   "Invalid JSON-RPC request",
			"id":      nil,
		})
		return
	}

	resp := struct {
		Jsonrpc string      `json:"jsonrpc"`
		Result  interface{} `json:"result,omitempty"`
		Error   *string     `json:"error,omitempty"`
		ID      interface{} `json:"id"`
	}{
		Jsonrpc: "2.0",
		ID:      req.ID,
	}

	switch req.Method {
	case "eth_sendRawTransaction":
		if len(req.Params) < 1 {
			errMsg := "Missing transaction hex"
			resp.Error = &errMsg
			c.JSON(http.StatusOK, resp)
			return
		}
		txHex, ok := req.Params[0].(string)
		if !ok {
			errMsg := "Invalid transaction hex"
			resp.Error = &errMsg
			c.JSON(http.StatusOK, resp)
			return
		}
		// Re-decode to get the full transaction for signature validation
		txBytes, err := hex.DecodeString(txHex)
		if err != nil {
			errMsg := fmt.Sprintf("Failed to decode hex: %v", err)
			resp.Error = &errMsg
			c.JSON(http.StatusOK, resp)
			return
		}

		var tx types.Transaction
		if err := rlp.DecodeBytes(txBytes, &tx); err != nil {
			errMsg := fmt.Sprintf("Failed to RLP decode: %v", err)
			resp.Error = &errMsg
			c.JSON(http.StatusOK, resp)
			return
		}
		signer := types.LatestSignerForChainID(tx.ChainId())
		sender, err := signer.Sender(&tx)

		if !common.IsHexAddress(sender.Hex()) {
			errMsg := "Failed to extract sender"
			resp.Error = &errMsg
			c.JSON(http.StatusOK, resp)
			return
		}

		if err != nil {
			errMsg := fmt.Sprintf("Invalid signature: %v", err)
			resp.Error = &errMsg
			c.JSON(http.StatusOK, resp)
			return
		}

		if tx.Gas() == 0 {
			errMsg := "Gas cannot be zero"
			resp.Error = &errMsg
			c.JSON(http.StatusOK, resp)
			return
		}

		if tx.To() != nil && !common.IsHexAddress(tx.To().Hex()) {
			errMsg := "Invalid recipient address"
			resp.Error = &errMsg
			c.JSON(http.StatusOK, resp)
			return
		}
		if err := txnDB.Put([]byte("tx_"+txHex), []byte(txHex)); err != nil {
			log.Errorf("Failed to store transaction %s: %v", txHex[:10], err)
		} else {
			log.Infof("Saved transaction %s to txnDB", txHex[:10])
		}
		pool.AddTx(txHex)

		// Return the transaction hash in the exact format Ethereum expects
		txHash := "0x" + tx.Hash().Hex()[2:] // Ensure it starts with 0x
		resp.Result = txHash
		c.JSON(http.StatusOK, resp)

	case "getLatestBatch":
		batchNo := 1
		for {
			key := []byte(fmt.Sprintf("batch_%d", batchNo))
			data, err := batchDB.Get(key)
			if err != nil || data == nil {
				batchNo--
				break
			}
			batchNo++
		}
		if batchNo < 1 {
			errMsg := "No batches found"
			resp.Error = &errMsg
			c.JSON(http.StatusOK, resp)
			return
		}
		key := []byte(fmt.Sprintf("batch_%d", batchNo))
		data, err := batchDB.Get(key)
		if err != nil {
			log.Errorf("Failed to fetch latest batch #%d: %v", batchNo, err)
			errMsg := fmt.Sprintf("Failed to fetch batch: %v", err)
			resp.Error = &errMsg
			c.JSON(http.StatusOK, resp)
			return
		}
		var b batch.Batch
		if err := json.Unmarshal(data, &b); err != nil {
			log.Errorf("Failed to unmarshal latest batch #%d: %v", batchNo, err)
			errMsg := fmt.Sprintf("Failed to parse batch: %v", err)
			resp.Error = &errMsg
			c.JSON(http.StatusOK, resp)
			return
		}
		decodedTxns := make([]*DecodedTransaction, len(b.Transactions))
		for i, txHex := range b.Transactions {
			decodedTxns[i] = decodeTransaction(txHex, log)
			if decodedTxns[i] == nil {
				log.Warnf("Failed to decode tx at index %d in batch #%d", i, batchNo)
				decodedTxns[i] = &DecodedTransaction{Hash: "0x0", From: "0x0", To: "0x0"}
			}
		}
		resp.Result = struct {
			Transactions      []*DecodedTransaction `json:"transactions"`
			ExecutionTraces   []string              `json:"execution_traces"`
			PrevMerkleHash    string                `json:"prev_merkle_hash"`
			CurrentMerkleHash string                `json:"current_merkle_hash"`
			PreviousStateRoot string                `json:"previous_state_root"`
			CurrentStateRoot  string                `json:"current_state_root"`
			DAProviddr        string                `json:"da_provider"`
			DACommitment      string                `json:"da_commitment"`
			DABlockHash       string                `json:"da_block_hash"`
			DATxHash          string                `json:"da_tx_hash"`
			BatchNo           int                   `json:"batch_no"`
			Metadata          batch.BatchMetadata   `json:"metadata"`
			Commitment        string                `json:"commitment"`
			Submitted         bool                  `json:"submitted"`
			Verified          bool                  `json:"verified"`
		}{
			Transactions:      decodedTxns,
			ExecutionTraces:   b.ExecutionTraces,
			PrevMerkleHash:    b.PrevMerkleHash,
			CurrentMerkleHash: b.CurrentMerkleHash,
			PreviousStateRoot: b.PreviousStateRoot,
			CurrentStateRoot:  b.CurrentStateRoot,
			DAProviddr:        b.DAProvider,
			DACommitment:      b.DACommitment,
			DABlockHash:       b.DABlockHash,
			DATxHash:          b.DATxHash,
			BatchNo:           b.BatchNo,
			Metadata:          b.Metadata,
			Commitment:        b.Commitment,
			Submitted:         b.Submitted,
			Verified:          b.Verified,
		}
		c.JSON(http.StatusOK, resp)

	case "getBatchByNumber":
		if len(req.Params) < 1 {
			errMsg := "Missing batch number"
			resp.Error = &errMsg
			c.JSON(http.StatusOK, resp)
			return
		}
		batchNoStr, ok := req.Params[0].(string)
		if !ok {
			errMsg := "Invalid batch number"
			resp.Error = &errMsg
			c.JSON(http.StatusOK, resp)
			return
		}
		batchNo, err := strconv.Atoi(batchNoStr)
		if err != nil {
			errMsg := "Batch number must be an integer"
			resp.Error = &errMsg
			c.JSON(http.StatusOK, resp)
			return
		}
		if batchNo < 1 {
			errMsg := "Batch number must be positive"
			resp.Error = &errMsg
			c.JSON(http.StatusOK, resp)
			return
		}
		key := []byte(fmt.Sprintf("batch_%d", batchNo))
		data, err := batchDB.Get(key)
		if err != nil || data == nil {
			errMsg := fmt.Sprintf("Batch #%d not found", batchNo)
			resp.Error = &errMsg
			c.JSON(http.StatusOK, resp)
			return
		}
		var b batch.Batch
		if err := json.Unmarshal(data, &b); err != nil {
			log.Errorf("Failed to unmarshal batch #%d: %v", batchNo, err)
			errMsg := fmt.Sprintf("Failed to parse batch: %v", err)
			resp.Error = &errMsg
			c.JSON(http.StatusOK, resp)
			return
		}
		decodedTxns := make([]*DecodedTransaction, len(b.Transactions))
		for i, txHex := range b.Transactions {
			decodedTxns[i] = decodeTransaction(txHex, log)
			if decodedTxns[i] == nil {
				log.Warnf("Failed to decode tx at index %d in batch #%d", i, batchNo)
				decodedTxns[i] = &DecodedTransaction{Hash: "0x0", From: "0x0", To: "0x0"}
			}
		}
		resp.Result = struct {
			Transactions      []*DecodedTransaction `json:"transactions"`
			ExecutionTraces   []string              `json:"execution_traces"`
			PrevMerkleHash    string                `json:"prev_merkle_hash"`
			CurrentMerkleHash string                `json:"current_merkle_hash"`
			PreviousStateRoot string                `json:"previous_state_root"`
			CurrentStateRoot  string                `json:"current_state_root"`
			DAProviddr        string                `json:"da_provider"`
			DACommitment      string                `json:"da_commitment"`
			DABlockHash       string                `json:"da_block_hash"`
			DATxHash          string                `json:"da_tx_hash"`
			BatchNo           int                   `json:"batch_no"`
			Metadata          batch.BatchMetadata   `json:"metadata"`
			Commitment        string                `json:"commitment"`
			Submitted         bool                  `json:"submitted"`
			Verified          bool                  `json:"verified"`
		}{
			Transactions:      decodedTxns,
			ExecutionTraces:   b.ExecutionTraces,
			PrevMerkleHash:    b.PrevMerkleHash,
			CurrentMerkleHash: b.CurrentMerkleHash,
			PreviousStateRoot: b.PreviousStateRoot,
			CurrentStateRoot:  b.CurrentStateRoot,
			DAProviddr:        b.DAProvider,
			DACommitment:      b.DACommitment,
			DABlockHash:       b.DABlockHash,
			DATxHash:          b.DATxHash,
			BatchNo:           b.BatchNo,
			Metadata:          b.Metadata,
			Commitment:        b.Commitment,
			Submitted:         b.Submitted,
			Verified:          b.Verified,
		}
		c.JSON(http.StatusOK, resp)

	default:
		var result json.RawMessage
		err := client.Rpc.Call(&result, req.Method, req.Params...)
		if err != nil {
			log.Errorf("Geth RPC error: %v", err)
			errMsg := fmt.Sprintf("Geth error: %v", err)
			resp.Error = &errMsg
			c.JSON(http.StatusOK, resp)
			return
		}
		resp.Result = json.RawMessage(result)
		c.JSON(http.StatusOK, resp)
	}
}
