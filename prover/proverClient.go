package prover

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/airchains-network/trusted-sequencer/prover/types"
)

type ProverClient struct {
	Client *http.Client
}

// NewHTTPClient creates a new HTTPClient with a default timeout.
func NewProverClient() *ProverClient {
	return &ProverClient{
		Client: &http.Client{},
	}
}

func (p *ProverClient) proverRequest(uri string, body any) ([]byte, error) {
	// JSON encode the body.
	jsonBody, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("error encoding JSON body: %w", err)
	}

	// Create a new request with the JSON body.
	req, err := http.NewRequest(http.MethodPost, uri, bytes.NewBuffer(jsonBody))
	if err != nil {
		return nil, fmt.Errorf("error creating POST request: %w", err)
	}

	// Set content type and add headers.
	req.Header.Set("Content-Type", "application/json")

	resp, err := p.Client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	return io.ReadAll(resp.Body)

}

func (p *ProverClient) ProverGenerate(endpoint string, body any) (*types.ProofData, error) {
	proofGenerateStruct := types.ProofGenerateBodyStruct{
		RollupID:      "",
		PreStateRoot:  "",
		PostStateRoot: "",
		BatchHash:     "",
		DaCommitment:  "",
	}

	uri := fmt.Sprintf("%s/api/v1/proof/generate", endpoint)

	res, err := p.proverRequest(uri, proofGenerateStruct)
	if err != nil {
		return nil, fmt.Errorf("error prover api req : %s", err.Error())
	}

	var bodyRes types.ProofResponceStruct
	if unmarshalErr := json.Unmarshal(res, &bodyRes); unmarshalErr != nil {
		return nil, fmt.Errorf("error while unmarshalling block : %s", unmarshalErr)
	}

	if bodyRes.Success {
		return nil, fmt.Errorf("error generateing proof : %s", bodyRes.Description)
	}

	return &bodyRes.Data, nil
}
