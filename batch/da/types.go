package da

import (
	SDK "github.com/availproject/avail-go-sdk/sdk"
	client "github.com/celestiaorg/celestia-openrpc"
	"github.com/celestiaorg/celestia-openrpc/types/share"
	"github.com/sirupsen/logrus"
	"github.com/vedhavyas/go-subkey/v2"
)

// DAClient defines a common interface for DA submissions
type DAClient interface {
	SubmitToDA(txData []byte, log *logrus.Logger) (daHash, daCommitment, daTxHash string, err error)
}

// AvailClient wraps the Avail SDK client for DA submissions
type AvailClient struct {
	Client    SDK.SDK
	Namespace uint32 // Avail AppID (Namespace)
	Account   subkey.KeyPair
}

// CelestiaClient wraps the Celestia  Light client for DA submissions
type CelestiaClient struct {
	Client    *client.Client
	Namespace share.Namespace // Your app's namespace ID
}
