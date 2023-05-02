package txmgr

import (
	"context"
	"errors"
	"fmt"
	"math/big"
	"testing"
	"time"

	"github.com/algorand/go-algorand-sdk/types"
	"github.com/ethereum-optimism/optimism/op-node/testlog"
	opcrypto "github.com/ethereum-optimism/optimism/op-service/milk-crypto"
	"github.com/ethereum-optimism/optimism/op-service/txmgr/metrics"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/log"
	"github.com/stretchr/testify/require"
)

type queueFunc func(id int, candidate TxCandidate, receiptCh chan TxReceipt[int], q *Queue[int]) bool

func sendQueueFunc(id int, candidate TxCandidate, receiptCh chan TxReceipt[int], q *Queue[int]) bool {
	q.Send(id, candidate, receiptCh)
	return true
}

func trySendQueueFunc(id int, candidate TxCandidate, receiptCh chan TxReceipt[int], q *Queue[int]) bool {
	return q.TrySend(id, candidate, receiptCh)
}

type queueCall struct {
	call   queueFunc // queue call (either Send or TrySend, use function helpers above)
	queued bool      // true if the send was queued
	txErr  bool      // true if the tx send should return an error
}

type testTx struct {
	sendErr bool // error to return from send for this tx
}

type testCase struct {
	name  string        // name of the test
	max   uint64        // max concurrency of the queue
	calls []queueCall   // calls to the queue
	txs   []testTx      // txs to generate from the factory (and potentially error in send)
	total time.Duration // approx. total time it should take to complete all queue calls
}

type mockBackendWithNonce struct {
	mockBackend
}

func newMockBackendWithNonce() *mockBackendWithNonce {
	return &mockBackendWithNonce{
		mockBackend: mockBackend{
			minedTxs: make(map[string]minedTxInfo),
		},
	}
}

func (b *mockBackendWithNonce) NonceAt(ctx context.Context, account common.Address, blockNumber *big.Int) (uint64, error) {
	return uint64(len(b.minedTxs)), nil
}

func TestSend(t *testing.T) {
	testCases := []testCase{
		{
			name: "success",
			max:  5,
			calls: []queueCall{
				{call: trySendQueueFunc, queued: true},
				{call: trySendQueueFunc, queued: true},
			},
			txs: []testTx{
				{},
				{},
			},
			total: 1 * time.Second,
		},
		{
			name: "no limit",
			max:  0,
			calls: []queueCall{
				{call: trySendQueueFunc, queued: true},
				{call: trySendQueueFunc, queued: true},
			},
			txs: []testTx{
				{},
				{},
			},
			total: 1 * time.Second,
		},
		{
			name: "single threaded",
			max:  1,
			calls: []queueCall{
				{call: trySendQueueFunc, queued: true},
				{call: trySendQueueFunc, queued: false},
				{call: trySendQueueFunc, queued: false},
			},
			txs: []testTx{
				{},
			},
			total: 1 * time.Second,
		},
		{
			name: "single threaded blocking",
			max:  1,
			calls: []queueCall{
				{call: trySendQueueFunc, queued: true},
				{call: trySendQueueFunc, queued: false},
				{call: sendQueueFunc, queued: true},
				{call: sendQueueFunc, queued: true},
			},
			txs: []testTx{
				{},
				{},
				{},
			},
			total: 3 * time.Second,
		},
		{
			name: "dual threaded blocking",
			max:  2,
			calls: []queueCall{
				{call: trySendQueueFunc, queued: true},
				{call: trySendQueueFunc, queued: true},
				{call: trySendQueueFunc, queued: false},
				{call: sendQueueFunc, queued: true},
				{call: sendQueueFunc, queued: true},
				{call: sendQueueFunc, queued: true},
			},
			txs: []testTx{
				{},
				{},
				{},
				{},
				{},
			},
			total: 3 * time.Second,
		},
		// TODO a test like this currently doesn't make sense since we
		// keep resending txs indefinitely (originally resend would abort on
		// SafeAbortNonceTooLowCount)
		// Leaving it for illustration if we do add a resend quitting criterion.
		// {
		// 	name: "subsequent txs fail after tx failure",
		// 	max:  1,
		// 	calls: []queueCall{
		// 		{call: sendQueueFunc, queued: true},
		// 		{call: sendQueueFunc, queued: true, txErr: true},
		// 		{call: sendQueueFunc, queued: true, txErr: true},
		// 	},
		// 	txs: []testTx{
		// 		{},
		// 		{sendErr: true},
		// 		{},
		// 	},
		// 	total: 3 * time.Second,
		// },
	}
	for _, test := range testCases {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			conf := defaultConfig()
			conf.ReceiptQueryInterval = 1 * time.Second // simulate a network send
			conf.ResubmissionTimeout = 2 * time.Second  // resubmit to detect errors
			backend := newMockBackendWithNonce()
			mgr := &SimpleTxManager{
				name:    "TEST",
				cfg:     conf,
				backend: backend,
				l:       testlog.Logger(t, log.LvlCrit),
				metr:    &metrics.NoopTxMetrics{},
			}

			sendTx := func(ctx context.Context, tx *opcrypto.SignedTxn) error {
				index := int(tx.Txn.Note[0])
				var testTx *testTx
				if index < len(test.txs) {
					testTx = &test.txs[index]
				}
				if testTx != nil && testTx.sendErr {
					return errors.New("Some error in send")
				}
				backend.confirm(tx.Txid)
				return nil
			}
			backend.setTxSender(sendTx)

			ctx, cancel := context.WithTimeout(context.Background(), 1*time.Minute)
			defer cancel()
			queue := NewQueue[int](ctx, mgr, test.max)

			// make all the queue calls given in the test case
			start := time.Now()
			for i, c := range test.calls {
				msg := fmt.Sprintf("Call %d", i)
				c := c
				receiptCh := make(chan TxReceipt[int], 1)
				candidate := TxCandidate{
					TxData: []byte{byte(i)},
					To:     types.Address{},
				}
				queued := c.call(i, candidate, receiptCh, queue)
				require.Equal(t, c.queued, queued, msg)
				go func() {
					r := <-receiptCh
					if c.txErr {
						require.Error(t, r.Err, msg)
					} else {
						require.NoError(t, r.Err, msg)
					}
				}()
			}
			// wait for the queue to drain (all txs complete or failed)
			queue.Wait()
			duration := time.Since(start)
			// expect the execution time within a certain window
			now := time.Now()
			require.WithinDuration(t, now.Add(test.total), now.Add(duration), 500*time.Millisecond, "unexpected queue transaction timing")
		})
	}
}
