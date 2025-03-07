package pool

import (
	"encoding/hex"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/airchains-network/trusted-sequencer/internal/db"
	"github.com/airchains-network/trusted-sequencer/internal/eth"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/rlp"
	"github.com/sirupsen/logrus"
)

// TxPool manages a pool of raw transactions
type TxPool struct {
	txs   []string
	mutex sync.Mutex
}

// NewTxPool initializes a new transaction pool
func NewTxPool() *TxPool {
	return &TxPool{txs: make([]string, 0)}
}

// AddTx adds a raw transaction to the pool
func (p *TxPool) AddTx(txHex string) {
	p.mutex.Lock()
	defer p.mutex.Unlock()
	p.txs = append(p.txs, txHex)
}

// Process sorts transactions by gas limit and sends them to Geth
func (p *TxPool) Process(client *eth.Client, txnDB db.DB, log *logrus.Logger) {
	for {
		p.mutex.Lock()
		if len(p.txs) == 0 {
			p.mutex.Unlock()
			time.Sleep(1 * time.Second)
			continue
		}

		type txWithGas struct {
			txHex    string
			gasLimit uint64
		}
		txList := make([]txWithGas, 0, len(p.txs))
		for _, txHex := range p.txs {
			txBytes, _ := hex.DecodeString(txHex)
			var tx types.Transaction
			if err := rlp.DecodeBytes(txBytes, &tx); err == nil {
				txList = append(txList, txWithGas{txHex: txHex, gasLimit: tx.Gas()})
			}
		}
		sort.Slice(txList, func(i, j int) bool {
			return txList[i].gasLimit > txList[j].gasLimit
		})

		for _, tx := range txList {
			var result string
			if err := client.Rpc.Call(&result, "eth_sendRawTransaction", tx.txHex); err != nil {
				if !strings.Contains(err.Error(), "already known") {
					p.txs = append(p.txs, tx.txHex)
					log.Warnf("Failed to send tx %s, retrying: %v", tx.txHex[:10], err)
				}
			} else {
				log.Infof("Sent tx %s to Geth", tx.txHex[:10])
			}
		}
		p.txs = p.txs[:0]
		p.mutex.Unlock()
	}
}
