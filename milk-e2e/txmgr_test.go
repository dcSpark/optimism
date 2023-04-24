package milk_e2e

import (
	"context"
	"encoding/hex"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/algorand/go-algorand-sdk/client/v2/algod"
	"github.com/algorand/go-algorand-sdk/crypto"
	"github.com/algorand/go-algorand-sdk/future"
	"github.com/ethereum-optimism/optimism/op-node/testlog"
	opcrypto "github.com/ethereum-optimism/optimism/op-service/milk-crypto"
	txmgr "github.com/ethereum-optimism/optimism/op-service/txmgr"
	"github.com/ethereum/go-ethereum/log"
)

type testHarness struct {
	cfg     txmgr.Config
	mgr     *txmgr.SimpleTxManager
	backend txmgr.AlgoBackend

	sender crypto.Account
	signer opcrypto.SignerFn
}

func newTestHarnessWithConfig(t *testing.T, cfg txmgr.Config) *testHarness {
	pk, _ := hex.DecodeString(testConfig.senderPrivKey)
	sender, err := crypto.AccountFromPrivateKey(pk)
	require.Nil(t, err)

	backend, err := txmgr.NewAlgodClient(testConfig.algodUrl, testConfig.algodToken)
	require.Nil(t, err)
	mgr := txmgr.NewSimpleTxManager("TEST", testlog.Logger(t, log.LvlTrace), cfg, backend)

	signer, _, err := opcrypto.CreateSignerFn(testConfig.senderPrivKey)
	require.Nil(t, err)
	return &testHarness{
		cfg:     cfg,
		mgr:     mgr,
		backend: backend,
		sender:  sender,
		signer:  signer,
	}
}

func newTestHarness(t *testing.T) *testHarness {
	return newTestHarnessWithConfig(t, defaultConfig())
}

func defaultConfig() txmgr.Config {
	return txmgr.Config{
		ResubmissionTimeout:       time.Second,
		ConfirmationQueryInterval: 50 * time.Millisecond,
	}
}

func TestSimpleTxSend(t *testing.T) {
	h := newTestHarness(t)

	client, _ := algod.MakeClient(testConfig.algodUrl, testConfig.algodToken)

	from := h.sender.Address.String()
	to := h.sender.Address.String()
	note := []byte("Rollup data")
	sp, _ := client.SuggestedParams().Do(context.Background())
	tx, _ := future.MakePaymentTxn(from, to, 0, note, "", sp)
	signed, err := h.signer(tx)
	require.Nil(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	confirmation, err := h.mgr.Send(ctx, signed)
	require.Nil(t, err)
	require.Greater(t, confirmation.ConfirmedRound, uint64(0))
	require.Equal(t, confirmation.Transaction.Txn.Note, note)
}
