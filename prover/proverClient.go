package prover

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"time"

	"github.com/airchains-network/trusted-sequencer/prover/types"
	"github.com/sirupsen/logrus"
)

type ProverClient struct {
	client *http.Client
	log    *logrus.Logger
}

// NewProverClient creates a new ProverClient with a 5-minute timeout.
func NewProverClient() *ProverClient {
	return &ProverClient{
		client: &http.Client{
			Timeout: 300 * time.Second, // Set timeout to 5 minutes
		},
		log: logrus.New(),
	}
}

func (p *ProverClient) proverRequest(uri string, body any) ([]byte, int, error) {
	// JSON encode the body.
	jsonBody, err := json.Marshal(body)
	if err != nil {
		return nil, 0, fmt.Errorf("error encoding JSON body: %w", err)
	}

	// Create a new request with the JSON body.
	req, err := http.NewRequest(http.MethodPost, uri, bytes.NewBuffer(jsonBody))
	if err != nil {
		return nil, 0, fmt.Errorf("error creating POST request: %w", err)
	}

	// Set content type and add headers.
	req.Header.Set("Content-Type", "application/json")

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, 0, fmt.Errorf("error making HTTP request: %w", err)
	}
	defer resp.Body.Close()

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, resp.StatusCode, fmt.Errorf("error reading response body: %w", err)
	}

	return bodyBytes, resp.StatusCode, nil
}

func (p *ProverClient) ProverGenerate(ctx context.Context, endpoint string, batchData types.BatchStruct, rollupID string, preStateRoot string, postStateRoot string, batchHash string, daCommitment string) (*types.ProofData, error) {
	proofGenerateStruct := types.ProofGenerateBodyStruct{
		RollupID:      rollupID,
		PreStateRoot:  preStateRoot,
		PostStateRoot: postStateRoot,
		BatchHash:     batchHash,
		DaCommitment:  daCommitment,
		BatchData:     batchData,
	}

	uri := fmt.Sprintf("%s/api/v1/proof/generate", endpoint)

	for attempt := 1; ; attempt++ {
		// Check for context cancellation
		if err := ctx.Err(); err != nil {
			return nil, fmt.Errorf("prover generate cancelled: %w", err)
		}

		res, statusCode, err := p.proverRequest(uri, proofGenerateStruct)
		if err == nil {
			var bodyRes types.ProofResponceStruct
			if unmarshalErr := json.Unmarshal(res, &bodyRes); unmarshalErr != nil {
				p.log.Warnf("Attempt %d: Error unmarshalling response: %v, retrying in %ds", attempt, unmarshalErr, int(math.Min(float64(uint(attempt-1)), 60)))
				time.Sleep(time.Duration(math.Min(float64(uint(attempt-1)), 60)) * time.Second)
				continue
			}

			if !bodyRes.Success {
				p.log.Warnf("Attempt %d: Proof generation failed: %s, retrying in %ds", attempt, bodyRes.Description, int(math.Min(float64(uint(attempt-1)), 60)))
				time.Sleep(time.Duration(math.Min(float64(uint(attempt-1)), 60)) * time.Second)
				continue
			}

			return &bodyRes.Data, nil
		}

		// Check for non-retryable HTTP status codes
		if statusCode == http.StatusBadRequest || // 400
			statusCode == http.StatusUnauthorized || // 401
			statusCode == http.StatusForbidden || // 403
			statusCode == http.StatusNotFound { // 404
			return nil, fmt.Errorf("non-retryable error (HTTP %d): %w", statusCode, err)
		}

		p.log.Warnf("Attempt %d: Error making prover request: %v, retrying in %ds", attempt, err, int(math.Min(float64(uint(attempt-1)), 60)))
		time.Sleep(time.Duration(math.Min(float64(uint(attempt-1)), 60)) * time.Second)
	}
}
