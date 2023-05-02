/*
	MILKOMEDA TO OP-STACK MIGRATION NOTES

Here we test whether TxManager reliably sends txns to L1.

These are unit tests, so we mock L1 interactions. The mocks should be designed
to be faithful by developing over a real Algorand network first (milk-e2e tests
incoming).

TODOs
Extend testing with various non-happy paths using sanity checking scheme
above to keep our L1 client mock faithful. Get inspiration in original
txmgr package and port more tests that make sense.
It may be worth trying to look for existing node mocks in go-algorand codebase
itself.
*/
package txmgr

import (
	"context"
	"encoding/hex"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/algorand/go-algorand-sdk/client/v2/common/models"
	"github.com/algorand/go-algorand-sdk/crypto"
	"github.com/algorand/go-algorand-sdk/types"
	algo "github.com/ethereum-optimism/optimism/op-node/algo"

	"github.com/ethereum-optimism/optimism/op-node/testlog"
	opcrypto "github.com/ethereum-optimism/optimism/op-service/milk-crypto"
	"github.com/ethereum-optimism/optimism/op-service/txmgr/metrics"

	"github.com/ethereum/go-ethereum/log"
)

type sendTransactionFunc func(ctx context.Context, tx *opcrypto.SignedTxn) error

// testHarness houses the necessary resources to test the SimpleTxManager.
type testHarness struct {
	cfg     Config
	mgr     *SimpleTxManager
	backend *mockBackend
}

// newTestHarnessWithConfig initializes a testHarness with a specific
// configuration.
func newTestHarnessWithConfig(t *testing.T, cfg Config) *testHarness {
	backend := newMockBackend()
	cfg.Backend = backend
	mgr := &SimpleTxManager{
		name:    "TEST",
		cfg:     cfg,
		backend: cfg.Backend,
		l:       testlog.Logger(t, log.LvlCrit),
		metr:    &metrics.NoopTxMetrics{},
	}

	return &testHarness{
		cfg:     cfg,
		mgr:     mgr,
		backend: backend,
	}
}

// newTestHarness initializes a testHarness with a default configuration that is
// suitable for most tests.
func newTestHarness(t *testing.T) *testHarness {
	return newTestHarnessWithConfig(t, defaultConfig())
}

func defaultConfig() Config {
	account := crypto.GenerateAccount()
	signer, _, _ := opcrypto.CreateSignerFn(hex.EncodeToString(account.PrivateKey))

	return Config{
		ResubmissionTimeout:   time.Second,
		ReceiptQueryInterval:  50 * time.Millisecond,
		TxNotInMempoolTimeout: 1 * time.Hour,
		Signer:                signer,
		From:                  account.Address,
	}
}

type minedTxInfo struct {
	confirmedRound uint64
	poolError      string
}

// mockBackend implements ReceiptSource that tracks mined transactions
// along with the gas price used.
type mockBackend struct {
	mu sync.RWMutex

	send sendTransactionFunc

	// blockHeight tracks the current height of the chain.
	blockHeight uint64

	// minedTxs maps the hash of a mined transaction to its details.
	minedTxs map[string]minedTxInfo
}

// newMockBackend initializes a new mockBackend.
func newMockBackend() *mockBackend {
	return &mockBackend{
		minedTxs: make(map[string]minedTxInfo),
	}
}

// setTxSender sets the implementation for the sendTransactionFunction
func (b *mockBackend) setTxSender(s sendTransactionFunc) {
	b.send = s
}

// confirm includes transaction on the mocked chain in a new block
func (b *mockBackend) confirm(txid string) {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.blockHeight++
	if txid != "" {
		b.minedTxs[txid] = minedTxInfo{
			confirmedRound: b.blockHeight,
		}
	}
}

func (b *mockBackend) PendingTransactionInformation(ctx context.Context, txid string) (info *models.PendingTransactionInfoResponse, err error) {
	b.mu.RLock()
	defer b.mu.RUnlock()

	txInfo, ok := b.minedTxs[txid]
	if !ok {
		return nil, nil
	}

	return &models.PendingTransactionInfoResponse{
		PoolError:      txInfo.poolError,
		ConfirmedRound: txInfo.confirmedRound,
	}, nil
}

func (b *mockBackend) SendTransaction(ctx context.Context, tx *opcrypto.SignedTxn) (txid string, err error) {
	if b.send == nil {
		panic("set sender function was not set")
	}
	return tx.Txid, b.send(ctx, tx)
}

func (b *mockBackend) AccountInformation(ctx context.Context, address string) (models.Account, error) {
	panic("AccountInformation not implemented on mock backend")
}

func (b *mockBackend) HeaderByNumber(ctx context.Context, round uint64) (algo.L1BlockRef, error) {
	panic("HeaderByNumber not implemented on mock backend")
}

func (b *mockBackend) SuggestedParams(ctx context.Context) (types.SuggestedParams, error) {
	return types.SuggestedParams{
		Fee:             5_000,
		GenesisID:       "",
		GenesisHash:     []byte("dummy"),
		FirstRoundValid: 1,
		LastRoundValid:  1000,
	}, nil
}

func TestTxMgrConfirmAtMinFee(t *testing.T) {
	t.Parallel()

	h := newTestHarness(t)

	// L1 MOCK CUSTOMIZATION
	// simplest case: any send confirms on chain immediately
	sendTx := func(ctx context.Context, tx *opcrypto.SignedTxn) error {
		h.backend.confirm(tx.Txid)
		return nil
	}
	h.backend.setTxSender(sendTx)

	// TEST
	candidate := TxCandidate{
		TxData: []byte("abcd"),
		To:     h.cfg.From,
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	confirmation, err := h.mgr.send(ctx, candidate)
	require.Nil(t, err)
	require.Greater(t, confirmation.ConfirmedRound, uint64(0))
}

// TestTxMgrNeverConfirmCancel asserts that a Send can be canceled even if
// transaction is not confirmed. This is done to ensure the the tx mgr can properly
// abort on shutdown, even if a txn is in the process of being published.
func TestTxMgrNeverConfirmCancel(t *testing.T) {
	t.Parallel()

	h := newTestHarness(t)

	// L1 MOCK CUSTOMIZATION
	// transaction won't get confirmed
	sendTx := func(ctx context.Context, tx *opcrypto.SignedTxn) error {
		return errors.New("err")
	}
	h.backend.setTxSender(sendTx)

	// TEST
	candidate := TxCandidate{
		TxData: []byte("abcd"),
		To:     h.cfg.From,
	}
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()
	confirmation, err := h.mgr.Send(ctx, candidate)
	require.Equal(t, err, context.DeadlineExceeded)
	require.Nil(t, confirmation)
}
