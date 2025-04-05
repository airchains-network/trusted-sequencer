package state

import (
	"encoding/hex"
	"fmt"
	"math/big"
	"sort"
	"strings"

	"github.com/airchains-network/trusted-sequencer/types"
	"github.com/ethereum/go-ethereum/rlp"
	"golang.org/x/crypto/sha3"
)

// SPTNode represents a node in the Sparse Merkle Tree
type SPTNode struct {
	Hash     []byte
	Children map[byte]*SPTNode
	Value    []byte
	IsLeaf   bool
}

// NewSPTNode creates a new SPT node
func NewSPTNode() *SPTNode {
	return &SPTNode{
		Children: make(map[byte]*SPTNode),
	}
}

// SPT represents the Sparse Merkle Tree
type SPT struct {
	Root *SPTNode
}

// NewSPT creates a new Sparse Merkle Tree
func NewSPT() *SPT {
	return &SPT{
		Root: NewSPTNode(),
	}
}

// Update the hash function in SPTNode
func (n *SPTNode) hash() []byte {
	if n.IsLeaf {
		return n.Value
	}

	var buf []byte
	for i := 0; i < 256; i++ {
		if child, exists := n.Children[byte(i)]; exists {
			buf = append(buf, child.Hash...)
		} else {
			// Use zero hash for empty children
			buf = append(buf, make([]byte, 32)...)
		}
	}

	// Use Keccak-256 for hashing
	hash := sha3.NewLegacyKeccak256()
	hash.Write(buf)
	return hash.Sum(nil)
}

// Insert inserts a key-value pair into the SPT
func (t *SPT) Insert(key []byte, value []byte) {
	current := t.Root
	for _, b := range key {
		if _, exists := current.Children[b]; !exists {
			current.Children[b] = NewSPTNode()
		}
		current = current.Children[b]
	}
	current.IsLeaf = true
	current.Value = value
	current.Hash = value
	t.updateHashes()
}

// Get retrieves a value from the SPT
func (t *SPT) Get(key []byte) ([]byte, bool) {
	current := t.Root
	for _, b := range key {
		if child, exists := current.Children[b]; exists {
			current = child
		} else {
			return nil, false
		}
	}
	if current.IsLeaf {
		return current.Value, true
	}
	return nil, false
}

// updateHashes updates all node hashes in the tree
func (t *SPT) updateHashes() {
	t.updateNodeHash(t.Root)
}

// updateNodeHash recursively updates the hash of a node and its children
func (t *SPT) updateNodeHash(node *SPTNode) {
	if node.IsLeaf {
		return
	}

	for _, child := range node.Children {
		t.updateNodeHash(child)
	}
	node.Hash = node.hash()
}

// GetRootHash returns the root hash of the SPT
func (t *SPT) GetRootHash() string {
	return hex.EncodeToString(t.Root.Hash)
}

// accountToKey converts an account address to a byte array for SPT insertion
func accountToKey(addr string) []byte {
	// Remove "0x" prefix if present
	addr = strings.TrimPrefix(addr, "0x")
	// Convert hex string to bytes
	key, _ := hex.DecodeString(addr)
	return key
}

func accountToValue(acc *types.EVMAccount) ([]byte, error) {
	// Create a temporary struct for RLP encoding
	type RLPAccount struct {
		Balance *big.Int
		Nonce   uint64
		Code    []byte
		Storage []KeyValue // Change from map to slice
	}

	// Convert the Storage map to a slice of KeyValue
	storageSlice := make([]KeyValue, 0)
	for contractAddr, storageMap := range acc.Storage {
		for k, v := range storageMap {
			storageSlice = append(storageSlice, KeyValue{Key: fmt.Sprintf("%s:%s", contractAddr, k), Value: v})
		}
	}

	rlpAcc := RLPAccount{
		Balance: acc.Balance,
		Nonce:   acc.Nonce,
		Code:    acc.Code,
		Storage: storageSlice,
	}

	// Use RLP encoding for account data
	return rlp.EncodeToBytes(rlpAcc)
}

// KeyValue struct for RLP encoding
type KeyValue struct {
	Key   string
	Value string
}

// CalculateStateHashSPT calculates the state hash using SPT
func CalculateStateHashSPT(accounts map[string]*types.EVMAccount) string {
	spt := NewSPT()

	// Sort addresses for deterministic ordering
	addresses := make([]string, 0, len(accounts))
	for addr := range accounts {
		addresses = append(addresses, addr)
	}
	sort.Strings(addresses)

	// Insert accounts into SPT
	for _, addr := range addresses {
		acc := accounts[addr]
		key := accountToKey(addr)
		value, err := accountToValue(acc)

		if err != nil {
			fmt.Println("Error encoding account:", err)
			return ""
		}
		spt.Insert(key, value)
	}

	return spt.GetRootHash()
}
