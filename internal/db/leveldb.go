package db

import (
	"github.com/syndtr/goleveldb/leveldb"
	"github.com/syndtr/goleveldb/leveldb/opt"
)

// LevelDB wraps a LevelDB instance
type LevelDB struct {
	db *leveldb.DB
}

// NewLevelDB creates a new LevelDB instance
func NewLevelDB(path string) (*LevelDB, error) {
	db, err := leveldb.OpenFile(path, &opt.Options{ErrorIfMissing: false})
	if err != nil {
		return nil, err
	}
	return &LevelDB{db: db}, nil
}

// Put stores a key-value pair in the database
func (l *LevelDB) Put(key, value []byte) error {
	return l.db.Put(key, value, nil)
}

// Get retrieves a value by key from the database
func (l *LevelDB) Get(key []byte) ([]byte, error) {
	data, err := l.db.Get(key, nil)
	if err == leveldb.ErrNotFound {
		return nil, nil
	}
	return data, err
}

// Close shuts down the database connection
func (l *LevelDB) Close() error {
	return l.db.Close()
}
