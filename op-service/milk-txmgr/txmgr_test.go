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
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/algorand/go-algorand-sdk/client/v2/common/models"
	"github.com/algorand/go-algorand-sdk/crypto"
	"github.com/ethereum-optimism/optimism/op-node/testlog"
	algo "github.com/ethereum-optimism/optimism/op-service/milk-algo"
	opcrypto "github.com/ethereum-optimism/optimism/op-service/milk-crypto"
	"github.com/ethereum/go-ethereum/log"
)

// testHarness houses the necessary resources to test the SimpleTxManager.
type testHarness struct {
	cfg     Config
	mgr     *SimpleTxManager
	backend *mockBackend

	sender crypto.Account
	signer opcrypto.SignerFn
}

// newTestHarnessWithConfig initializes a testHarness with a specific
// configuration.
func newTestHarnessWithConfig(t *testing.T, cfg Config) *testHarness {
	backend := newMockBackend()
	mgr := NewSimpleTxManager("TEST", testlog.Logger(t, log.LvlCrit), cfg, backend)

	account := crypto.GenerateAccount()
	signer, _, err := opcrypto.CreateSignerFn(hex.EncodeToString(account.PrivateKey))
	require.Nil(t, err)

	return &testHarness{
		cfg:     cfg,
		mgr:     mgr,
		backend: backend,
		sender:  account,
		signer:  signer,
	}
}

// newTestHarness initializes a testHarness with a default configuration that is
// suitable for most tests.
func newTestHarness(t *testing.T) *testHarness {
	return newTestHarnessWithConfig(t, defaultConfig())
}

func defaultConfig() Config {
	return Config{
		ResubmissionTimeout:       time.Second,
		ConfirmationQueryInterval: 50 * time.Millisecond,
	}
}

type minedTxInfo struct {
	confirmedRound uint64
	poolError      string
}

type mockBackend struct {
	mu sync.RWMutex

	send SendTransactionFunc

	// blockHeight tracks the current height of the chain.
	blockHeight uint64

	// minedTxs maps the txid of a mined transaction to its details.
	minedTxs map[string]minedTxInfo
}

// newMockBackend initializes a new mockBackend.
func newMockBackend() *mockBackend {
	return &mockBackend{
		minedTxs: make(map[string]minedTxInfo),
	}
}

// setTxSender sets the implementation for the SendTransactionFunction
func (b *mockBackend) setTxSender(s SendTransactionFunc) {
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
	return b.send(ctx, tx)
}

func TestTxMgrConfirmAtMinFee(t *testing.T) {
	t.Parallel()

	h := newTestHarness(t)

	// L1 MOCK CUSTOMIZATION
	// simplest case: any send confirms on chain immediately
	sendTx := func(ctx context.Context, tx *opcrypto.SignedTxn) (string, error) {
		h.backend.confirm(tx.Txid)
		return tx.Txid, nil
	}
	h.backend.setTxSender(sendTx)

	// TRANSACTION PREPARATION
	tx, _ := algo.NewPaymentTransaction()
	tx.Sender = h.sender.Address
	tx.Receiver = h.sender.Address
	signed, err := h.signer(tx)
	require.Nil(t, err)

	// TEST
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	confirmation, err := h.mgr.Send(ctx, signed)
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
	sendTx := func(ctx context.Context, tx *opcrypto.SignedTxn) (string, error) {
		return "", nil
	}
	h.backend.setTxSender(sendTx)

	// TRANSACTION PREPARATION
	tx, _ := algo.NewPaymentTransaction()
	tx.Sender = h.sender.Address
	tx.Receiver = h.sender.Address
	signed, err := h.signer(tx)
	require.Nil(t, err)

	// TEST
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()
	confirmation, err := h.mgr.Send(ctx, signed)
	require.Equal(t, err, context.DeadlineExceeded)
	require.Nil(t, confirmation)
}
