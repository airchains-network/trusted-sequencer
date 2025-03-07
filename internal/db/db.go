package db

// DB defines the interface for database operations
type DB interface {
	Put(key, value []byte) error
	Get(key []byte) ([]byte, error)
	Close() error
}

// NewLevelDBs initializes transaction and batch databases
func NewLevelDBs(txnPath, batchPath string) (DB, DB, error) {
	txnDB, err := NewLevelDB(txnPath)
	if err != nil {
		return nil, nil, err
	}
	batchDB, err := NewLevelDB(batchPath)
	if err != nil {
		txnDB.Close()
		return nil, nil, err
	}
	return txnDB, batchDB, nil
}
