package state

import (
	"encoding/json"
	"fmt"
	"math/big"
	"strings"

	"github.com/airchains-network/trusted-sequencer/types"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/syndtr/goleveldb/leveldb"
)

const (
	accountPrefix         = "account:"
	lastProcessedBlockKey = "lastProcessedBlock"
)

type EVMState struct {
	db *leveldb.DB
}

func NewEVMState(dbPath string) (*EVMState, error) {
	db, err := leveldb.OpenFile(dbPath, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %v", err)
	}
	return &EVMState{db: db}, nil
}

func (s *EVMState) GetAccount(addr string) (*types.EVMAccount, error) {
	data, err := s.db.Get([]byte(accountPrefix+strings.ToLower(addr)), nil)
	if err == leveldb.ErrNotFound {
		return &types.EVMAccount{
			Balance: big.NewInt(0),
			Nonce:   0,
			Storage: make(map[string]map[string]string),
		}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get account: %v", err)
	}
	var acc types.EVMAccount
	if err := json.Unmarshal(data, &acc); err != nil {
		return nil, fmt.Errorf("failed to decode account: %v", err)
	}
	return &acc, nil
}

func (s *EVMState) SaveAccount(addr string, acc *types.EVMAccount) error {
	data, err := json.Marshal(acc)
	if err != nil {
		return fmt.Errorf("failed to encode account: %v", err)
	}
	return s.db.Put([]byte(accountPrefix+strings.ToLower(addr)), data, nil)
}

func (s *EVMState) GetLastProcessedBlock() (uint64, error) {
	data, err := s.db.Get([]byte(lastProcessedBlockKey), nil)
	if err == leveldb.ErrNotFound {
		return 0, nil
	}
	if err != nil {
		return 0, fmt.Errorf("failed to get last block: %v", err)
	}
	return new(big.Int).SetBytes(data).Uint64(), nil
}

func (s *EVMState) SetLastProcessedBlock(block uint64) error {
	return s.db.Put([]byte(lastProcessedBlockKey), big.NewInt(int64(block)).Bytes(), nil)
}

func (s *EVMState) GetAllAccounts() (map[string]*types.EVMAccount, error) {
	accounts := make(map[string]*types.EVMAccount)
	iter := s.db.NewIterator(nil, nil)
	defer iter.Release()

	for iter.Next() {
		key := string(iter.Key())
		if strings.HasPrefix(key, accountPrefix) {
			addr := key[len(accountPrefix):]
			var acc types.EVMAccount
			if err := json.Unmarshal(iter.Value(), &acc); err != nil {
				return nil, fmt.Errorf("failed to decode account %s: %v", addr, err)
			}
			accounts[addr] = &acc
		}
	}
	return accounts, iter.Error()
}

func (s *EVMState) Close() {
	s.db.Close()
}

func ComputeContractAddress(sender string, nonce uint64) string {
	addr := common.HexToAddress(sender)
	contractAddr := crypto.CreateAddress(addr, nonce)
	return strings.ToLower(contractAddr.Hex())
}

// CreateAccount creates a new account with the given address and initial balance.
func (s *EVMState) CreateAccount(addr string, initialBalance *big.Int) error {
	acc := &types.EVMAccount{
		Balance: initialBalance,
		Nonce:   0,
		Storage: make(map[string]map[string]string),
	}
	return s.SaveAccount(addr, acc)
}

// DeleteAccount deletes an account by setting its balance to zero and clearing its storage.
func (s *EVMState) DeleteAccount(addr string) error {
	acc := &types.EVMAccount{
		Balance: big.NewInt(0),
		Nonce:   0,
		Storage: make(map[string]map[string]string),
	}
	return s.SaveAccount(addr, acc)
}

// UpdateAccount updates the balance and nonce of an existing account.
func (s *EVMState) UpdateAccount(addr string, balance *big.Int, nonce uint64) error {
	acc, err := s.GetAccount(addr)
	if err != nil {
		return err
	}
	acc.Balance = balance
	acc.Nonce = nonce
	return s.SaveAccount(addr, acc)
}

// SetStorage sets a value in the contract's storage.
func (s *EVMState) SetStorage(addr string, contractAddr string, key string, value string) error {
	acc, err := s.GetAccount(addr)
	if err != nil {
		return err
	}
	if acc.Storage[contractAddr] == nil {
		acc.Storage[contractAddr] = make(map[string]string)
	}
	acc.Storage[contractAddr][key] = value
	return s.SaveAccount(addr, acc)
}

// GetStorage retrieves a value from the contract's storage.
func (s *EVMState) GetStorage(addr string, contractAddr string, key string) (string, error) {
	acc, err := s.GetAccount(addr)
	if err != nil {
		return "", err
	}
	return acc.Storage[contractAddr][key], nil
}
