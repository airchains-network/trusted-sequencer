package da

import (
	"context"
	"encoding/hex"
	"fmt"
	"time"

	client "github.com/celestiaorg/celestia-openrpc"
	"github.com/celestiaorg/celestia-openrpc/types/blob"
	"github.com/celestiaorg/celestia-openrpc/types/share"
	"github.com/sirupsen/logrus"
)

// NewCelestiaClient initializes a Celestia client
func NewCelestiaClient(nodeAddr, authToken string, namespace string, log *logrus.Logger) (*CelestiaClient, error) {
	// Connect to Celestia node
	celestiaClient, err := client.NewClient(context.Background(), nodeAddr, authToken)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize Celestia client: %v", err)
	}
	// Convert namespace string to bytes
	nameSpaceBytes := []byte(namespace) // Default namespace
	ns, err := share.NewBlobNamespaceV0(nameSpaceBytes)
	if err != nil {
		return nil, fmt.Errorf("failed to create namespace: %v", err)
	}

	log.Infof("Initialized Celestia client for node %s with namespace %s", nodeAddr, ns)

	return &CelestiaClient{
		Client:    celestiaClient,
		Namespace: ns,
	}, nil
}

// Close closes the Celestia client connection
func (c *CelestiaClient) Close() {
	c.Client.Close()
}

// SubmitToDA submits transaction data to Celestia and returns a commitment
// SubmitToCelestiaDA submits transaction data to Celestia and retries indefinitely if it fails
func (c *CelestiaClient) SubmitToDA(txData []byte, log *logrus.Logger) (string, string, error) {
	blobData, err := blob.NewBlobV0(c.Namespace, txData)
	if err != nil {
		return "", "", fmt.Errorf("failed to create Celestia blob: %v", err)
	}

	var height uint64
	var commitment string
	var backoff time.Duration = 1 * time.Second

	// Infinite retry loop
	for {
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()

		height, err = c.Client.Blob.Submit(ctx, []*blob.Blob{blobData}, blob.NewSubmitOptions())
		if err == nil {
			commitment = hex.EncodeToString(blobData.Commitment)
			log.Infof("Successfully submitted to Celestia at height %d, commitment: %s", height, commitment)
			return fmt.Sprintf("%d", height), commitment, nil
		}

		log.Errorf("Celestia submission failed: %v. Retrying in %s...", err, backoff)

		// Exponential backoff with cap at 1 minute
		time.Sleep(backoff)
		if backoff < 1*time.Minute {
			backoff *= 2
		}
	}
}
