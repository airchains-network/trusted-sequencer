package da

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"time"

	SDK "github.com/availproject/avail-go-sdk/sdk"
	"github.com/sirupsen/logrus"
)

// NewAvailClient initializes an Avail DA client
func NewAvailClient(nodeAddr, authToken, namespace string, log *logrus.Logger) (*AvailClient, error) {
	acc, err := SDK.Account.NewKeyPair(authToken)
	if err != nil {
		return nil, fmt.Errorf("failed to create account: %v", err)
	}
	log.Infof("Created Avail account with address: %s", acc.SS58Address(42))

	sdk, err := SDK.NewSDK(nodeAddr)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize Avail SDK: %v", err)
	}

	var appId uint32 = 36 // Default AppID (adjust if needed)
	log.Infof("Initialized Avail client for node %s with AppID %d", nodeAddr, appId)

	return &AvailClient{
		Client:    sdk,
		Namespace: appId,
		Account:   acc,
	}, nil
}

// SubmitToDA submits transaction data to Avail and retries if it fails
func (c *AvailClient) SubmitToDA(txData []byte, log *logrus.Logger) (string, string, string, error) {
	hash := sha256.Sum256(txData)
	commitment := hex.EncodeToString(hash[:])

	var backoff time.Duration = 1 * time.Second
	for {
		tx := c.Client.Tx.DataAvailability.SubmitData(txData)
		res, err := tx.ExecuteAndWatchInclusion(c.Account, SDK.NewTransactionOptions().WithAppId(c.Namespace))
		if err != nil {
			log.Errorf("Avail transaction failed: %v. Retrying in %s...", err, backoff)
			time.Sleep(backoff)
			if backoff < 1*time.Minute {
				backoff *= 2
			}
			continue
		}

		success := res.IsSuccessful().UnsafeUnwrap()
		if !success {
			log.Errorf("Avail transaction was not successful. Retrying in %s...", backoff)
			time.Sleep(backoff)
			if backoff < 1*time.Minute {
				backoff *= 2
			}
			continue
		}

		log.Infof("Successfully submitted to Avail! Tx Hash: %s, Block Hash: %s, Block Number: %d, Tx Index: %d",
			res.TxHash.ToHexWith0x(), res.BlockHash.ToHexWith0x(), res.BlockNumber, res.TxIndex)

		return fmt.Sprintf("%d", res.BlockNumber), res.TxHash.ToHexWith0x(), commitment, nil
	}
}
