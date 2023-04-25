package milk_e2e

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/ethereum-optimism/optimism/op-batcher/metrics"
	"github.com/ethereum-optimism/optimism/op-node/testlog"
	opcrypto "github.com/ethereum-optimism/optimism/op-service/milk-crypto"
	txmgr "github.com/ethereum-optimism/optimism/op-service/txmgr"
	"github.com/ethereum/go-ethereum/log"
)

type testHarness struct {
	cfg txmgr.Config
	mgr *txmgr.SimpleTxManager
}

func newTestHarnessWithConfig(t *testing.T, cfg txmgr.CLIConfig) *testHarness {
	m := metrics.NewMetrics("default")
	l := testlog.Logger(t, log.LvlTrace)
	config, err := txmgr.NewConfig(cfg, l)
	require.Nil(t, err)

	mgr, err := txmgr.NewSimpleTxManager("TEST", l, m, cfg)
	require.Nil(t, err)
	return &testHarness{
		cfg: config,
		mgr: mgr,
	}
}

func newTestHarness(t *testing.T) *testHarness {
	return newTestHarnessWithConfig(t, defaultConfig())
}

func defaultConfig() txmgr.CLIConfig {
	return txmgr.CLIConfig{
		L1RPCURL:              testConfig.algodUrl,
		L1RPCToken:            testConfig.algodToken,
		PrivateKey:            testConfig.senderPrivKey,
		ResubmissionTimeout:   5 * time.Second,
		ReceiptQueryInterval:  50 * time.Millisecond,
		NetworkTimeout:        5 * time.Second,
		TxSendTimeout:         5 * time.Second,
		TxNotInMempoolTimeout: 5 * time.Second,
	}
}

func TestSimpleTxSend(t *testing.T) {
	h := newTestHarness(t)
	candidate := txmgr.TxCandidate{
		TxData: []byte("abcd"),
		To:     h.cfg.From,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	confirmation, err := h.mgr.Send(ctx, candidate)
	require.Nil(t, err)
	require.Greater(t, confirmation.ConfirmedRound, uint64(0))
	require.Equal(t, confirmation.Transaction.Txn.Note, candidate.TxData)
}
