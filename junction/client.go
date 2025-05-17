package junction

import (
	"context"
	"fmt"
	"math"
	"strconv"
	"time"

	"github.com/sirupsen/logrus"

	"github.com/airchains-network/junction/x/rollup/types"
	"github.com/cosmos/cosmos-sdk/types/query"
	"github.com/ignite/cli/v28/ignite/pkg/cosmosaccount"
	"github.com/ignite/cli/v28/ignite/pkg/cosmosclient"
)

type JunctionClient struct {
	*cosmosclient.Client
	*cosmosaccount.Account
	rpc *string
}

func NewJunctionClient(
	ctx context.Context,
	accountName string,
	accountPath string,
	jsonRPC string,
) (*JunctionClient, error) {

	registry, err := cosmosaccount.New(cosmosaccount.WithHome(accountPath))
	if err != nil {
		return nil, err
	}

	account, err := registry.GetByName(accountName)
	if err != nil {
		return nil, err
	}

	accountAddress, err := account.Address("air")
	if err != nil {
		return nil, err
	}
	logrus.Infof("Using account %s with address %s", accountName, accountAddress)

	client, err := cosmosclient.New(
		ctx,
		cosmosclient.WithAddressPrefix("air"),
		cosmosclient.WithNodeAddress(jsonRPC),
		cosmosclient.WithHome(accountPath),
		cosmosclient.WithGas("auto"),
		cosmosclient.WithGasAdjustment(1.7),
		cosmosclient.WithGasPrices("0.025uamf"),
	)
	if err != nil {
		return nil, err
	}

	return &JunctionClient{
		Client:  &client,
		Account: &account,
		rpc:     &jsonRPC,
	}, nil
}

func (j *JunctionClient) GetRPC() string {
	if j.rpc == nil {
		return ""
	}
	return *j.rpc
}

func (j *JunctionClient) checkBalance(ctx context.Context) error {

	accountAddress, err := j.Account.Address("air")
	if err != nil {
		return err
	}

	pageRequest := &query.PageRequest{} // Add this line to create a new PageRequest

	balances, err := j.BankBalances(ctx, accountAddress, pageRequest)
	if err != nil {
		return fmt.Errorf("failed to get balance: %w", err)
	}

	if len(balances) == 0 {
		return fmt.Errorf("account do not have balance. Address: %s", accountAddress)
	}

	var accountBalance int64

	for _, balance := range balances {
		if balance.Denom == "uamf" {
			accountBalance = balance.Amount.Int64()
		}
	}

	amountDiv, err := strconv.ParseFloat(strconv.FormatInt(accountBalance, 10), 64)
	if err != nil {
		return fmt.Errorf("failed to parse balance: %w", err)
	}
	dividedAmount := amountDiv / math.Pow(10, 6)
	if dividedAmount < 10 {
		return fmt.Errorf("account balance is less than 10 AMF. Address: %s", accountAddress)
	}

	return nil
}

func (j *JunctionClient) SubmitBatchMetadata(ctx context.Context,
	batchNo uint64,
	rollupId string,
	daName string,
	daCommitment string,
	daHash string,
	daPointer string,
	daNamespace string,
) {
	for {
		err := j.checkBalance(ctx)
		if err != nil {
			logrus.Warnf(err.Error())
			time.Sleep(5 * time.Second)
			continue
		}

		accountAddress, err := j.Account.Address("air")
		if err != nil {
			logrus.Warnf("error in getting account address : %s", err.Error())
			time.Sleep(5 * time.Second)
			continue
		}

		submitBatchMetadataBody := types.MsgSubmitBatchMetadata{
			Creator:      accountAddress,
			BatchNo:      batchNo,
			RollupId:     rollupId,
			DaName:       daName,
			DaCommitment: daCommitment,
			DaHash:       daHash,
			DaPointer:    daPointer,
			DaNamespace:  daNamespace,
		}
		account := *j.Account

		txResp, err := j.Client.BroadcastTx(ctx, account, &submitBatchMetadataBody)
		if err != nil {
			logrus.Warnf("error in broadcasting transaction into junction : %s", err.Error())
			time.Sleep(5 * time.Second)
			continue
		}

		if txResp.Code != 0 {
			logrus.Warnf("error in submit batch metadata into junction : %s", txResp.RawLog)
			time.Sleep(5 * time.Second)
			continue
		}

		break
	}

}

func (j *JunctionClient) SubmitBatch(ctx context.Context,
	batchNo uint64,
	rollupId string,
	merkleRootHash string,
	previousMerkleRootHash string,
	zkProof []byte,
	pubWitness []byte,
) {
	for {
		err := j.checkBalance(ctx)
		if err != nil {
			logrus.Warnf(err.Error())
			time.Sleep(5 * time.Second)
			continue
		}

		accountAddress, err := j.Account.Address("air")
		if err != nil {
			logrus.Warnf("error in getting account address : %s", err.Error())
			time.Sleep(5 * time.Second)
			continue
		}

		submitBatchMetadataBody := types.MsgSubmitBatch{
			Creator:                accountAddress,
			RollupId:               rollupId,
			BatchNo:                batchNo,
			MerkleRootHash:         merkleRootHash,
			PreviousMerkleRootHash: "",
			ZkProof:                []byte{},
			PublicWitness:          []byte{},
		}
		account := *j.Account

		txResp, err := j.Client.BroadcastTx(ctx, account, &submitBatchMetadataBody)
		if err != nil {
			logrus.Warnf("error in broadcasting transaction into junction : %s", err.Error())
			time.Sleep(5 * time.Second)
			continue
		}

		if txResp.Code != 0 {
			logrus.Warnf("error in submit batch into junction : %s", txResp.RawLog)
			time.Sleep(5 * time.Second)
			continue
		}

		break
	}

}
