package proxy

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math/big"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/airchains-network/trusted-sequencer/batch"
	"github.com/airchains-network/trusted-sequencer/db"
	"github.com/airchains-network/trusted-sequencer/eth"
	"github.com/airchains-network/trusted-sequencer/pool"
	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types" 
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/rlp"
	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
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

// WebSocketClient represents a connected WebSocket client
type WebSocketClient struct {
	conn          *websocket.Conn
	send          chan []byte
	pool          *pool.TxPool
	client        *eth.Client
	txnDB         db.DB
	batchDB       db.DB
	log           *logrus.Logger
	mu            sync.Mutex
	closed        bool
	subscriptions map[string]chan interface{}
}

// WebSocketManager manages all WebSocket connections
type WebSocketManager struct {
	clients            map[*WebSocketClient]bool
	broadcast          chan []byte
	register           chan *WebSocketClient
	unregister         chan *WebSocketClient
	pool               *pool.TxPool
	client             *eth.Client
	txnDB              db.DB
	batchDB            db.DB
	log                *logrus.Logger
	subscriptionEvents chan interface{}
}

// NewWebSocketManager creates a new WebSocket manager
func NewWebSocketManager(pool *pool.TxPool, client *eth.Client, txnDB, batchDB db.DB, log *logrus.Logger) *WebSocketManager {
	return &WebSocketManager{
		clients:            make(map[*WebSocketClient]bool),
		broadcast:          make(chan []byte),
		register:           make(chan *WebSocketClient),
		unregister:         make(chan *WebSocketClient),
		pool:               pool,
		client:             client,
		txnDB:              txnDB,
		batchDB:            batchDB,
		log:                log,
		subscriptionEvents: make(chan interface{}, 100),
	}
}

// Run starts the WebSocket manager
func (manager *WebSocketManager) Run() {
	go manager.handleSubscriptionEvents()

	for {
		select {
		case client := <-manager.register:
			manager.clients[client] = true
			manager.log.Infof("New WebSocket client connected. Total clients: %d", len(manager.clients))
		case client := <-manager.unregister:
			if _, ok := manager.clients[client]; ok {
				delete(manager.clients, client)
				close(client.send)
				manager.log.Infof("WebSocket client disconnected. Total clients: %d", len(manager.clients))
			}
		case message := <-manager.broadcast:
			for client := range manager.clients {
				select {
				case client.send <- message:
				default:
					close(client.send)
					delete(manager.clients, client)
				}
			}
		}
	}
}

// handleSubscriptionEvents processes subscription events and broadcasts them to subscribed clients
func (manager *WebSocketManager) handleSubscriptionEvents() {
	for event := range manager.subscriptionEvents {
		for client := range manager.clients {
			client.mu.Lock()
			for _, subChan := range client.subscriptions {
				select {
				case subChan <- event:
				default:
				}
			}
			client.mu.Unlock()
		}
	}
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

// Start launches the GIN proxy server with WebSocket support
func Start(rpcPort string, wsPort string, client *eth.Client, txnDB, batchDB db.DB, pool *pool.TxPool, log *logrus.Logger) error {
	gin.SetMode(gin.ReleaseMode) // No debug noise

	// Create WebSocket manager
	wsManager := NewWebSocketManager(pool, client, txnDB, batchDB, log)
	go wsManager.Run()

	// WebSocket upgrader
	upgrader := websocket.Upgrader{
		CheckOrigin: func(r *http.Request) bool {
			return true // Allow all origins for now
		},
	}

	// Create RPC server
	rpcServer := gin.New()
	rpcServer.Use(gin.LoggerWithConfig(gin.LoggerConfig{
		Formatter: func(param gin.LogFormatterParams) string {
			return fmt.Sprintf("[GIN] %s - %s %s %d\n",
				param.TimeStamp.Format("2006-01-02 15:04:05"),
				param.Method,
				param.Path,
				param.StatusCode,
			)
		},
	}))
	rpcServer.Use(gin.Recovery())
	rpcServer.POST("/", func(c *gin.Context) {
		handleRPC(c, client, txnDB, batchDB, pool, log)
	})

	// Create WebSocket server
	wsServer := gin.New()
	wsServer.Use(gin.LoggerWithConfig(gin.LoggerConfig{
		Formatter: func(param gin.LogFormatterParams) string {
			return fmt.Sprintf("[WS] %s - %s %s %d\n",
				param.TimeStamp.Format("2006-01-02 15:04:05"),
				param.Method,
				param.Path,
				param.StatusCode,
			)
		},
	}))
	wsServer.Use(gin.Recovery())
	wsServer.GET("/", func(c *gin.Context) {
		handleWebSocket(c, upgrader, wsManager)
	})

	// Start both servers
	go func() {
		log.Infof("Starting WebSocket server on %s", wsPort)
		if err := wsServer.Run(wsPort); err != nil {
			log.Errorf("WebSocket server error: %v", err)
		}
	}()

	log.Infof("Starting RPC server on %s", rpcPort)
	return rpcServer.Run(rpcPort)
}

// handleWebSocket processes WebSocket connections
func handleWebSocket(c *gin.Context, upgrader websocket.Upgrader, manager *WebSocketManager) {
	conn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		manager.log.Errorf("Failed to upgrade connection to WebSocket: %v", err)
		return
	}

	client := &WebSocketClient{
		conn:          conn,
		send:          make(chan []byte, 256),
		pool:          manager.pool,
		client:        manager.client,
		txnDB:         manager.txnDB,
		batchDB:       manager.batchDB,
		log:           manager.log,
		subscriptions: make(map[string]chan interface{}),
	}

	manager.register <- client

	// Start goroutines for reading and writing
	go client.writePump()
	go client.readPump(manager)
}

// readPump pumps messages from the WebSocket connection to the hub
func (c *WebSocketClient) readPump(manager *WebSocketManager) {
	defer func() {
		manager.unregister <- c
		c.conn.Close()
	}()

	c.conn.SetReadLimit(512 * 1024) // 512KB
	c.conn.SetReadDeadline(time.Now().Add(60 * time.Second))
	c.conn.SetPongHandler(func(string) error {
		c.conn.SetReadDeadline(time.Now().Add(60 * time.Second))
		return nil
	})

	for {
		_, message, err := c.conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				c.log.Errorf("WebSocket read error: %v", err)
			}
			break
		}

		// Process the message
		var req struct {
			Jsonrpc string        `json:"jsonrpc"`
			Method  string        `json:"method"`
			Params  []interface{} `json:"params"`
			ID      interface{}   `json:"id"`
		}

		if err := json.Unmarshal(message, &req); err != nil {
			c.log.Errorf("Failed to parse WebSocket message: %v", err)
			continue
		}

		// Handle the RPC request
		resp := handleWebSocketRPC(req, c)

		// Send the response back to the client
		responseBytes, err := json.Marshal(resp)
		if err != nil {
			c.log.Errorf("Failed to marshal WebSocket response: %v", err)
			continue
		}

		c.mu.Lock()
		if !c.closed {
			c.send <- responseBytes
		}
		c.mu.Unlock()
	}
}

// writePump pumps messages from the hub to the WebSocket connection
func (c *WebSocketClient) writePump() {
	ticker := time.NewTicker(54 * time.Second)
	defer func() {
		ticker.Stop()
		c.conn.Close()
	}()

	for {
		select {
		case message, ok := <-c.send:
			c.mu.Lock()
			if !ok {
				c.conn.WriteMessage(websocket.CloseMessage, []byte{})
				c.closed = true
				c.mu.Unlock()
				return
			}

			w, err := c.conn.NextWriter(websocket.TextMessage)
			if err != nil {
				c.mu.Unlock()
				return
			}
			w.Write(message)

			n := len(c.send)
			for i := 0; i < n; i++ {
				w.Write([]byte{'\n'})
				w.Write(<-c.send)
			}

			if err := w.Close(); err != nil {
				c.mu.Unlock()
				return
			}
			c.mu.Unlock()
		case <-ticker.C:
			c.mu.Lock()
			if err := c.conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				c.mu.Unlock()
				return
			}
			c.mu.Unlock()
		}
	}
}

// handleWebSocketRPC processes RPC requests from WebSocket clients
func handleWebSocketRPC(req struct {
	Jsonrpc string        `json:"jsonrpc"`
	Method  string        `json:"method"`
	Params  []interface{} `json:"params"`
	ID      interface{}   `json:"id"`
}, client *WebSocketClient) interface{} {
	resp := struct {
		Jsonrpc string      `json:"jsonrpc"`
		Result  interface{} `json:"result,omitempty"`
		Error   *string     `json:"error,omitempty"`
		ID      interface{} `json:"id"`
	}{
		Jsonrpc: "2.0",
		ID:      req.ID,
	}

	// Handle different RPC methods
	switch req.Method {
	case "eth_sendRawTransaction":
		if len(req.Params) < 1 {
			errMsg := "Missing transaction parameter"
			resp.Error = &errMsg
			return resp
		}

		txHex, ok := req.Params[0].(string)
		if !ok {
			errMsg := "Invalid transaction parameter"
			resp.Error = &errMsg
			return resp
		}

		// Remove "0x" prefix if present
		if len(txHex) > 2 && txHex[:2] == "0x" {
			txHex = txHex[2:]
		}

		// Decode and process the transaction
		tx := decodeTransaction(txHex, client.log)
		if tx == nil {
			errMsg := "Invalid transaction"
			resp.Error = &errMsg
			return resp
		}

		// Add transaction to pool
		// TODO: Implement actual transaction processing
		resp.Result = tx.Hash
		return resp

	case "eth_subscribe":
		if len(req.Params) < 1 {
			errMsg := "Missing subscription type"
			resp.Error = &errMsg
			return resp
		}

		subType, ok := req.Params[0].(string)
		if !ok {
			errMsg := "Invalid subscription type"
			resp.Error = &errMsg
			return resp
		}

		// Generate a unique subscription ID
		subID := fmt.Sprintf("sub_%d", time.Now().UnixNano())

		// Create a channel for this subscription
		subChan := make(chan interface{}, 100)

		// Store the subscription
		client.mu.Lock()
		client.subscriptions[subID] = subChan
		client.mu.Unlock()

		// Start a goroutine to handle subscription events
		go handleSubscription(client, subID, subType, subChan, req.Params[1:])

		resp.Result = subID
		return resp

	case "eth_unsubscribe":
		if len(req.Params) < 1 {
			errMsg := "Missing subscription ID"
			resp.Error = &errMsg
			return resp
		}

		subID, ok := req.Params[0].(string)
		if !ok {
			errMsg := "Invalid subscription ID"
			resp.Error = &errMsg
			return resp
		}

		// Remove the subscription
		client.mu.Lock()
		if subChan, exists := client.subscriptions[subID]; exists {
			close(subChan)
			delete(client.subscriptions, subID)
			resp.Result = true
		} else {
			resp.Result = false
		}
		client.mu.Unlock()

		return resp

	case "logs":
		// This is a legacy method for backward compatibility
		// It's handled by the eth_subscribe method with "logs" type
		errMsg := "Use eth_subscribe with 'logs' type instead"
		resp.Error = &errMsg
		return resp

	default:
		errMsg := "Method not supported: " + req.Method
		resp.Error = &errMsg
		return resp
	}
}

// handleSubscription processes subscription events for a specific subscription
func handleSubscription(client *WebSocketClient, subID, subType string, subChan chan interface{}, params []interface{}) interface{} {
	defer func() {
		client.mu.Lock()
		delete(client.subscriptions, subID)
		client.mu.Unlock()
	}()

	// Handle different subscription types
	switch subType {
	case "newHeads":
		// Subscribe to new block headers
		headers := make(chan *types.Header)
		sub, err := client.client.Eth.SubscribeNewHead(context.Background(), headers)
		if err != nil {
			client.log.Errorf("Failed to subscribe to new heads: %v", err)
			return nil
		}
		defer sub.Unsubscribe()

		for {
			select {
			case header := <-headers:
				// Format the header as a notification
				notification := map[string]interface{}{
					"jsonrpc": "2.0",
					"method":  "eth_subscription",
					"params": map[string]interface{}{
						"subscription": subID,
						"result": map[string]interface{}{
							"hash":   header.Hash().Hex(),
							"number": header.Number.String(),
						},
					},
				}

				// Send the notification to the client
				notificationBytes, err := json.Marshal(notification)
				if err != nil {
					client.log.Errorf("Failed to marshal notification: %v", err)
					continue
				}

				client.mu.Lock()
				if !client.closed {
					client.send <- notificationBytes
				}
				client.mu.Unlock()

			case <-subChan:
				// Subscription was cancelled
				return nil
			}
		}

	case "logs":
		// Subscribe to logs matching the filter
		var filterQuery ethereum.FilterQuery

		if len(params) > 0 {
			// Parse the filter query from params
			filterMap, ok := params[0].(map[string]interface{})
			if !ok {
				client.log.Errorf("Invalid filter query format")
				return nil
			}

			// Extract address if present
			if addr, ok := filterMap["address"]; ok {
				switch v := addr.(type) {
				case string:
					if v != "" {
						filterQuery.Addresses = []common.Address{common.HexToAddress(v)}
					}
				case []interface{}:
					addresses := make([]common.Address, 0, len(v))
					for _, a := range v {
						if addrStr, ok := a.(string); ok {
							addresses = append(addresses, common.HexToAddress(addrStr))
						}
					}
					filterQuery.Addresses = addresses
				}
			}

			// Extract topics if present
			if topics, ok := filterMap["topics"]; ok {
				if topicArray, ok := topics.([]interface{}); ok {
					filterQuery.Topics = make([][]common.Hash, len(topicArray))
					for i, topic := range topicArray {
						switch t := topic.(type) {
						case string:
							if t != "" {
								filterQuery.Topics[i] = []common.Hash{common.HexToHash(t)}
							}
						case []interface{}:
							hashes := make([]common.Hash, 0, len(t))
							for _, h := range t {
								if hashStr, ok := h.(string); ok {
									hashes = append(hashes, common.HexToHash(hashStr))
								}
							}
							filterQuery.Topics[i] = hashes
						}
					}
				}
			}

			// Extract fromBlock if present
			if fromBlock, ok := filterMap["fromBlock"]; ok {
				if fromBlockStr, ok := fromBlock.(string); ok {
					if fromBlockStr == "latest" {
						filterQuery.FromBlock = nil
					} else if fromBlockStr == "earliest" {
						filterQuery.FromBlock = big.NewInt(0)
					} else if fromBlockStr == "pending" {
						filterQuery.FromBlock = nil
					} else {
						// Try to parse as a block number
						if blockNum, err := strconv.ParseInt(fromBlockStr, 10, 64); err == nil {
							filterQuery.FromBlock = big.NewInt(blockNum)
						}
					}
				}
			}

			// Extract toBlock if present
			if toBlock, ok := filterMap["toBlock"]; ok {
				if toBlockStr, ok := toBlock.(string); ok {
					if toBlockStr == "latest" {
						filterQuery.ToBlock = nil
					} else if toBlockStr == "earliest" {
						filterQuery.ToBlock = big.NewInt(0)
					} else if toBlockStr == "pending" {
						filterQuery.ToBlock = nil
					} else {
						// Try to parse as a block number
						if blockNum, err := strconv.ParseInt(toBlockStr, 10, 64); err == nil {
							filterQuery.ToBlock = big.NewInt(blockNum)
						}
					}
				}
			}
		}

		logs := make(chan types.Log)
		sub, err := client.client.Eth.SubscribeFilterLogs(context.Background(), filterQuery, logs)
		if err != nil {
			client.log.Errorf("Failed to subscribe to logs: %v", err)
			return nil
		}
		defer sub.Unsubscribe()

		for {
			select {
			case log := <-logs:
				// Format the log as a notification
				notification := map[string]interface{}{
					"jsonrpc": "2.0",
					"method":  "eth_subscription",
					"params": map[string]interface{}{
						"subscription": subID,
						"result": map[string]interface{}{
							"address":          log.Address.Hex(),
							"blockHash":        log.BlockHash.Hex(),
							"blockNumber":      log.BlockNumber,
							"data":             "0x" + hex.EncodeToString(log.Data),
							"logIndex":         log.Index,
							"removed":          log.Removed,
							"topics":           log.Topics,
							"transactionHash":  log.TxHash.Hex(),
							"transactionIndex": log.TxIndex,
						},
					},
				}

				// Send the notification to the client
				notificationBytes, err := json.Marshal(notification)
				if err != nil {
					client.log.Errorf("Failed to marshal notification: %v", err)
					continue
				}

				client.mu.Lock()
				if !client.closed {
					client.send <- notificationBytes
				}
				client.mu.Unlock()

			case <-subChan:
				// Subscription was cancelled
				return nil
			}
		}

	default:
		client.log.Errorf("Unsupported subscription type: %s", subType)
		return nil
	}
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
