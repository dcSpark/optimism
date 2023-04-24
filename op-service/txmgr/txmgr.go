/*
	MILKOMEDA TO OP-STACK MIGRATION NOTES

TxManager reliably sends batcher transactions to L1.

There are several differences to the original handling:
* Transaction fee bumping is not implemented and not a high priority for us, as minimum fee
is almost always enough on Algorand.
* Algorand doesn't have nonces that require handling.
* Algorand has instant finality and we don't need to wait until block with our txn is buried
under k other blocks.

TODOs
Search in code.
*/
package txmgr

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/algorand/go-algorand-sdk/client/v2/algod"
	"github.com/algorand/go-algorand-sdk/client/v2/common/models"
	"github.com/algorand/go-algorand-sdk/future"
	"github.com/algorand/go-algorand-sdk/types"
	"github.com/ethereum/go-ethereum/log"

	"github.com/ethereum-optimism/optimism/op-node/algo"
	opcrypto "github.com/ethereum-optimism/optimism/op-service/milk-crypto"
	"github.com/ethereum-optimism/optimism/op-service/txmgr/metrics"
)

// TxManager is an interface that allows callers to reliably publish txs,
// bumping the gas price if needed, and obtain the receipt of the resulting tx.
//
//go:generate mockery --name TxManager --output ./mocks
type TxManager interface {
	// Send is used to create & send a transaction. It will handle potential
	// resending & ensuring that the transaction remains in the transaction pool.
	// It can be stopped by cancelling the provided context; however, the transaction
	// may be included on L1 even if the context is cancelled.
	//
	// NOTE: Send can be called concurrently.
	Send(ctx context.Context, candidate TxCandidate) (*models.PendingTransactionInfoResponse, error)

	// From returns the sending address associated with the instance of the transaction manager.
	// It is static for a single instance of a TxManager.
	From() string
}

// AlgoBackend is the set of methods the transaction manager uses to
// interact with L1.
type AlgoBackend interface {
	// PendingTransactionInformation queries the backend for information
	// associated with txid to determine transaction confirmation status.
	PendingTransactionInformation(ctx context.Context, txid string) (*models.PendingTransactionInfoResponse, error)

	// SendTransaction submits a signed transaction to L1.
	SendTransaction(ctx context.Context, tx *opcrypto.SignedTxn) (txid string, err error)

	// AccountInformation retrieves account information for given address.
	AccountInformation(ctx context.Context, address string) (models.Account, error)

	// HeaderByNumber retrieves block header for given round, latest round if unspecified
	HeaderByNumber(ctx context.Context, round uint64) (algo.L1BlockRef, error)

	// SuggestedParams returns params suggested for transaction building in the current
	// network situation.
	SuggestedParams(ctx context.Context) (types.SuggestedParams, error)
}

// AlgodClient is an implementation of AlgoBackend and a thin wrapper over
// the Algorand SDK's client.
type AlgodClient struct {
	client *algod.Client
}

func NewAlgodClient(url string, token string) (*AlgodClient, error) {
	client, err := algod.MakeClient(url, token)
	if err != nil {
		return nil, err
	}
	return &AlgodClient{client}, nil
}

func (c *AlgodClient) PendingTransactionInformation(ctx context.Context, txid string) (*models.PendingTransactionInfoResponse, error) {
	info, _, err := c.client.PendingTransactionInformation(txid).Do(ctx)
	if err != nil {
		return nil, err
	}
	return &info, nil
}

func (c *AlgodClient) AccountInformation(ctx context.Context, address string) (models.Account, error) {
	return c.client.AccountInformation(address).Do(ctx)
}

func (c *AlgodClient) SuggestedParams(ctx context.Context) (types.SuggestedParams, error) {
	return c.client.SuggestedParams().Do(ctx)
}

func (c *AlgodClient) HeaderByNumber(ctx context.Context, round uint64) (algo.L1BlockRef, error) {
	if round == 0 {
		status, err := c.client.Status().Do(ctx)
		if err != nil {
			return algo.L1BlockRef{}, err
		}
		round = status.LastRound
	}
	block, err := c.client.Block(round).Do(ctx)
	if err != nil {
		return algo.L1BlockRef{}, err
	}
	// TODO it seems we need to make a separate query on Algorand to get block hash.
	// Reason is unknown. We should be able to recompute blockhash from the block
	// query result however and spare one query.
	blockhash, err := c.client.GetBlockHash(round).Do(ctx)
	if err != nil {
		return algo.L1BlockRef{}, err
	}
	return algo.L1BlockRef{
		Hash:       blockhash.Blockhash,
		Number:     uint64(block.Round),
		ParentHash: base64.StdEncoding.EncodeToString(block.Branch[:]),
		Time:       uint64(block.TimeStamp),
	}, nil
}

func (c *AlgodClient) SendTransaction(ctx context.Context, tx *opcrypto.SignedTxn) (txid string, err error) {
	txid, err = c.client.SendRawTransaction(tx.RawTxn).Do(ctx)
	return
}

// SimpleTxManager is a implementation of TxManager that resends transaction
// without changing fees or anything else.
type SimpleTxManager struct {
	cfg  Config // embed the config directly
	name string

	backend AlgoBackend
	l       log.Logger
	metr    metrics.TxMetricer

	nonce     *uint64
	nonceLock sync.RWMutex

	pending atomic.Int64
}

// NewSimpleTxManager initializes a new SimpleTxManager with the passed Config.
func NewSimpleTxManager(name string, l log.Logger, m metrics.TxMetricer, cfg CLIConfig) (*SimpleTxManager, error) {
	conf, err := NewConfig(cfg, l)
	if err != nil {
		return nil, err
	}

	return &SimpleTxManager{
		name:    name,
		cfg:     conf,
		backend: conf.Backend,
		l:       l.New("service", name),
		metr:    m,
	}, nil
}

func (m *SimpleTxManager) From() types.Address {
	return m.cfg.From
}

// TxCandidate is a transaction candidate that can be submitted to ask the
// [TxManager] to construct a transaction.
type TxCandidate struct {
	// TxData is the transaction data to be used in the constructed tx.
	TxData []byte
	// To is the recipient of the constructed tx. Nil means contract creation.
	To types.Address
}

// Send is used to publish a transaction
// until the transaction eventually confirms. This method blocks until an
// invocation of sendTx returns. The method
// may be canceled using the passed context.
//
// The transaction manager handles all signing.
//
// NOTE: Send can be called concurrently.
func (m *SimpleTxManager) Send(ctx context.Context, candidate TxCandidate) (*models.PendingTransactionInfoResponse, error) {
	m.metr.RecordPendingTx(m.pending.Add(1))
	defer func() {
		m.metr.RecordPendingTx(m.pending.Add(-1))
	}()
	receipt, err := m.send(ctx, candidate)
	if err != nil {
		m.resetNonce()
	}
	return receipt, err
}

// send performs the actual transaction creation and sending.
func (m *SimpleTxManager) send(ctx context.Context, candidate TxCandidate) (*models.PendingTransactionInfoResponse, error) {
	if m.cfg.TxSendTimeout != 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, m.cfg.TxSendTimeout)
		defer cancel()
	}
	tx, err := m.craftTx(ctx, candidate)
	if err != nil {
		return nil, fmt.Errorf("failed to create the tx: %w", err)
	}
	return m.sendTx(ctx, tx)
}

// craftTx creates the signed transaction
// It queries L1 for suggested params such as fee.
// NOTE: This method SHOULD NOT publish the resulting transaction.
func (m *SimpleTxManager) craftTx(ctx context.Context, candidate TxCandidate) (*opcrypto.SignedTxn, error) {
	sp, err := m.backend.SuggestedParams(context.Background())
	if err != nil {
		m.metr.RPCError()
		return nil, fmt.Errorf("failed to get suggested params: %w", err)
	}

	from := m.cfg.From.String()
	to := candidate.To.String()
	tx, err := future.MakePaymentTxn(from, to, 0, candidate.TxData, "", sp)
	if err != nil {
		m.metr.RPCError()
		return nil, fmt.Errorf("failed to create payment transaction: %w", err)
	}
	m.l.Info("creating tx", "to", candidate.To, "from", m.cfg.From)

	return m.cfg.Signer(tx)
}

// send submits the same transaction several times with increasing gas prices as necessary.
// It waits for the transaction to be confirmed on chain.
func (m *SimpleTxManager) sendTx(ctx context.Context, tx *opcrypto.SignedTxn) (*models.PendingTransactionInfoResponse, error) {
	var wg sync.WaitGroup
	defer wg.Wait()
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	sendState := NewSendState(m.cfg.TxNotInMempoolTimeout)
	receiptChan := make(chan *models.PendingTransactionInfoResponse, 1)
	sendTxAsync := func(tx *opcrypto.SignedTxn) {
		defer wg.Done()
		m.publishAndWaitForTx(ctx, tx, sendState, receiptChan)
	}

	// Immediately publish a transaction before starting the resumbission loop
	wg.Add(1)
	go sendTxAsync(tx)

	ticker := time.NewTicker(m.cfg.ResubmissionTimeout)
	defer ticker.Stop()

	bumpCounter := 0
	for {
		select {
		case <-ticker.C:
			// Don't resubmit a transaction if it has been mined, but we are waiting for the conf depth.
			if sendState.IsWaitingForConfirmation() {
				continue
			}
			// If we see lots of unrecoverable errors (and no pending transactions) abort sending the transaction.
			if sendState.ShouldAbortImmediately() {
				m.l.Warn("Aborting transaction submission")
				return nil, errors.New("aborted transaction sending")
			}
			// TODO price increase in case of congestion to be done here
			wg.Add(1)
			bumpCounter += 1
			go sendTxAsync(tx)

		case <-ctx.Done():
			return nil, ctx.Err()

		case receipt := <-receiptChan:
			m.metr.RecordBumpCount(bumpCounter)
			m.metr.TxConfirmed(receipt)
			return receipt, nil
		}
	}
}

// publishAndWaitForTx publishes the transaction to the transaction pool and then waits for it with [waitMined].
// It should be called in a new go-routine. It will send the receipt to receiptChan in a non-blocking way if a receipt is found
// for the transaction.
func (m *SimpleTxManager) publishAndWaitForTx(ctx context.Context, tx *opcrypto.SignedTxn, sendState *SendState, receiptChan chan *models.PendingTransactionInfoResponse) {
	log := m.l.New("txid", tx.Txid)
	log.Info("publishing transaction")

	cCtx, cancel := context.WithTimeout(ctx, m.cfg.NetworkTimeout)
	defer cancel()
	t := time.Now()
	_, err := m.backend.SendTransaction(cCtx, tx)
	sendState.ProcessSendError(err)

	// Properly log & exit if there is an error
	if err != nil {
		switch {
		case errStringMatch(err, context.Canceled):
			m.metr.RPCError()
			log.Warn("transaction send cancelled", "err", err)
			m.metr.TxPublished("context_cancelled")
		default:
			m.metr.RPCError()
			log.Error("unable to publish transaction", "err", err)
			m.metr.TxPublished("unknown_error")
		}
		return
	}
	m.metr.TxPublished("")

	log.Info("Transaction successfully published")
	// Poll for the transaction to be ready & then send the result to receiptChan
	receipt, err := m.waitMined(ctx, tx, sendState)
	if err != nil {
		log.Warn("Transaction receipt not found", "err", err)
		return
	}
	select {
	case receiptChan <- receipt:
		m.metr.RecordTxConfirmationLatency(time.Since(t).Milliseconds())
	default:
	}
}

// waitMined waits for the transaction to be mined or for the context to be cancelled.
func (m *SimpleTxManager) waitMined(ctx context.Context, tx *opcrypto.SignedTxn, sendState *SendState) (*models.PendingTransactionInfoResponse, error) {
	queryTicker := time.NewTicker(m.cfg.ReceiptQueryInterval)
	defer queryTicker.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-queryTicker.C:
			if receipt := m.queryReceipt(ctx, tx.Txid, sendState); receipt != nil {
				return receipt, nil
			}
		}
	}
}

// queryReceipt queries for the receipt and returns the receipt if it has passed the confirmation depth
func (m *SimpleTxManager) queryReceipt(ctx context.Context, txid string, sendState *SendState) *models.PendingTransactionInfoResponse {
	ctx, cancel := context.WithTimeout(ctx, m.cfg.NetworkTimeout)
	defer cancel()
	info, err := m.backend.PendingTransactionInformation(ctx, txid)
	if err != nil {
		m.metr.RPCError()
		m.l.Info("Confirmation retrieval failed", "txid", txid, "err", err)
		return nil
	} else if info == nil || info.ConfirmedRound <= 0 {
		sendState.TxNotMined(txid)
		m.l.Trace("Transaction not yet mined", "txid", txid)
		return nil
	} else if info.PoolError != "" {
		sendState.TxNotMined(txid)
		m.l.Warn("Pool error, tx rejected", "txid", txid, "err", info.PoolError)
		return nil
	}

	// Receipt is confirmed to be valid from this point on
	sendState.TxMined(txid)

	return info
}

// errStringMatch returns true if err.Error() is a substring in target.Error() or if both are nil.
// It can accept nil errors without issue.
func errStringMatch(err, target error) bool {
	if err == nil && target == nil {
		return true
	} else if err == nil || target == nil {
		return false
	}
	return strings.Contains(err.Error(), target.Error())
}
