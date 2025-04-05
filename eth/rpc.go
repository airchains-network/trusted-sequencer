package eth

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"math/big"
	"net/http"
	"strings"
	"time"
)

const maxRetries = 3

type RPCRequest struct {
	Jsonrpc string        `json:"jsonrpc"`
	Method  string        `json:"method"`
	Params  []interface{} `json:"params"`
	ID      int           `json:"id"`
}

type RPCResponse struct {
	Jsonrpc string          `json:"jsonrpc"`
	Result  json.RawMessage `json:"result"`
	Error   *struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

type RpcClient struct {
	URL string
}

func NewRpcClient(url string) *RpcClient {
	return &RpcClient{URL: url}
}

func (c *RpcClient) Call(method string, params []interface{}) (*RPCResponse, error) {
	reqBody := RPCRequest{
		Jsonrpc: "2.0",
		Method:  method,
		Params:  params,
		ID:      1,
	}
	data, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %v", err)
	}

	for attempt := 0; attempt < maxRetries; attempt++ {
		resp, err := http.Post(c.URL, "application/json", bytes.NewBuffer(data))
		if err == nil {
			defer resp.Body.Close()
			var rpcResp RPCResponse
			if err := json.NewDecoder(resp.Body).Decode(&rpcResp); err != nil {
				return nil, fmt.Errorf("failed to decode response: %v", err)
			}
			if rpcResp.Error != nil {
				return nil, fmt.Errorf("RPC error: %s", rpcResp.Error.Message)
			}
			return &rpcResp, nil
		}
		log.Printf("RPC call attempt %d failed: %v", attempt+1, err)
		if attempt < maxRetries-1 {
			time.Sleep(time.Second * time.Duration(attempt+1))
		}
	}
	return nil, fmt.Errorf("RPC call failed after %d retries", maxRetries)
}

func (c *RpcClient) GetGasUsed(txHash string) (*big.Int, error) {
	resp, err := c.Call("eth_getTransactionReceipt", []interface{}{txHash})
	if err != nil {
		return nil, err
	}
	var receipt struct {
		GasUsed string `json:"gasUsed"`
	}
	if err := json.Unmarshal(resp.Result, &receipt); err != nil {
		return nil, err
	}
	return HexToBigInt(receipt.GasUsed), nil
}

func (c *RpcClient) FetchContractState(txHash, addr, blockNumber string) (map[string]string, []byte, error) {
	// Get contract code
	codeResp, err := c.Call("eth_getCode", []interface{}{addr, blockNumber})
	if err != nil {
		return nil, nil, fmt.Errorf("failed to get code: %v", err)
	}
	var codeHex string
	if err := json.Unmarshal(codeResp.Result, &codeHex); err != nil {
		return nil, nil, fmt.Errorf("failed to unmarshal code: %v", err)
	}
	code := strings.TrimPrefix(codeHex, "0x")
	codeBytes, _ := hex.DecodeString(code)

	// Try debug_traceTransaction for full storage changes
	storage := make(map[string]string)
	traceResp, err := c.Call("debug_traceTransaction", []interface{}{txHash, map[string]string{"tracer": "stateDiffTracer"}})
	if err == nil {
		var trace struct {
			StateDiff map[string]struct {
				Storage map[string]struct {
					From string `json:"from"`
					To   string `json:"to"`
				} `json:"storage"`
			} `json:"stateDiff"`
		}
		if err := json.Unmarshal(traceResp.Result, &trace); err == nil {
			if stateDiff, ok := trace.StateDiff[addr]; ok {
				for slot, diff := range stateDiff.Storage {
					storage[slot] = diff.To // Save the new storage value
				}
				return storage, codeBytes, nil // Success with full storage
			}
		}
	}

	// Fallback: Brute force common slots (0x0 to 0x9) if tracing fails
	log.Printf("Warning: debug_traceTransaction failed or unsupported for %s, falling back to slot guessing", txHash)
	for i := 0; i < 10; i++ { // Check slots 0x0 to 0x9
		slot := fmt.Sprintf("0x%x", i)
		storageResp, err := c.Call("eth_getStorageAt", []interface{}{addr, slot, blockNumber})
		if err != nil {
			continue
		}
		var slotValue string
		if err := json.Unmarshal(storageResp.Result, &slotValue); err == nil {
			if slotValue != "0x0000000000000000000000000000000000000000000000000000000000000000" {
				storage[slot] = slotValue // Only save non-zero values
			}
		}
	}

	return storage, codeBytes, nil
}

func HexToBigInt(hexStr string) *big.Int {
	if hexStr == "" {
		return big.NewInt(0)
	}
	n := new(big.Int)
	n.SetString(strings.TrimPrefix(hexStr, "0x"), 16)
	return n
}
