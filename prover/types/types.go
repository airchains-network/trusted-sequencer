package types

type BatchStruct struct {
	From              []string
	To                []string
	Amounts           []string
	TransactionHash   []string
	SenderBalances    []string
	ReceiverBalances  []string
	PreStateRoot      string
	PostStateRoot     string
	TxMerkleRoot      string
	ExpectedBatchHash string
}
type ProofGenerateBodyStruct struct {
	RollupID      string `json:"rollup-id"`
	PreStateRoot  string `json:"preStateRoot"`
	PostStateRoot string `json:"postStateRoot"`
	BatchHash     string `json:"batchHash"`
	DaCommitment  string `json:"daCommitment"`
	BatchData     BatchStruct
}

type ProofData struct {
	ProofData     []byte `json:"proof_data,omitempty"`
	PublicInputs  []byte `json:"public_inputs,omitempty"`
	BatchHash     string `json:"batch_hash,omitempty"`
	PreStateRoot  string `json:"pre_state_root,omitempty"`
	PostStateRoot string `json:"post_state_root,omitempty"`
	MerkleRoot    []byte `json:"merkle_root,omitempty"`
}

type ProofResponceStruct struct {
	Status      int       `json:"status"`
	Success     bool      `json:"success"`
	Message     string    `json:"message"`
	Description string    `json:"description"`
	Data        ProofData `json:"data,omitempty"`
}
