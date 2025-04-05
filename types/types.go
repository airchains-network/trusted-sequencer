package types

import "math/big"

// EVMAccount represents an Ethereum account
type EVMAccount struct {
	Balance *big.Int                     `json:"balance"`
	Nonce   uint64                       `json:"nonce"`
	Code    []byte                       `json:"code,omitempty"`
	Storage map[string]map[string]string // Separate storage for each contract
	Root    string                       `json:"root,omitempty"`
}
