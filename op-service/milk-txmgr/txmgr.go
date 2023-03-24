/*
	MILKOMEDA TO OP-STACK MIGRATION NOTES

This package is a replacement of the op-service/txmgr package.

It provides a TxManager object to reliably send batcher transactions to L1.

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
	"sync"
	"time"

	"github.com/algorand/go-algorand-sdk/client/v2/algod"
	"github.com/algorand/go-algorand-sdk/client/v2/common/models"
	"github.com/algorand/go-algorand-sdk/types"
	"github.com/ethereum/go-ethereum/log"

	"github.com/ethereum-optimism/optimism/op-node/algo"
	opcrypto "github.com/ethereum-optimism/optimism/op-service/milk-crypto"
)

type SendTransactionFunc = func(ctx context.Context, tx *opcrypto.SignedTxn) (txid string, err error)

// Config houses parameters for altering the behavior of a SimpleTxManager.
type Config struct {
	// ResubmissionTimeout is the interval at which, if no previously
	// published transaction has been seen on chain, a new tx will be
	// sent out.
	ResubmissionTimeout time.Duration

	// ConfirmationQueryInterval is the interval at which the tx manager
	// will query the backend to check for confirmations after a tx
	// has been published.
	ConfirmationQueryInterval time.Duration

	// Signer can be used to sign a new transaction with increased fee
	// if network is recognized as congested (not implemented currently).
	Signer opcrypto.SignerFn
	From   string
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
		Hash: blockhash.Blockhash,
		Number: uint64(block.Round),
		ParentHash: base64.StdEncoding.EncodeToString(block.Branch[:]),
		Time: uint64(block.TimeStamp),
	}, nil
}

func (c *AlgodClient) SendTransaction(ctx context.Context, tx *opcrypto.SignedTxn) (txid string, err error) {
	txid, err = c.client.SendRawTransaction(tx.RawTxn).Do(ctx)
	return
}

// TxManager is an interface that allows callers to reliably publish txs
// and obtain the confirmation of the resulting tx.
type TxManager interface {
	// Send is used to publish a transaction repeatedly
	// until it eventually confirms. This method blocks
	// until an invocation of sendTx returns.
	// The method may be canceled using the passed context.
	//
	// The initial transaction MUST be signed & ready to submit.
	//
	// NOTE: Send should be called by AT MOST one caller at a time.
	Send(ctx context.Context, tx *opcrypto.SignedTxn) (*models.PendingTransactionInfoResponse, error)
}

// SimpleTxManager is an implementation of TxManager that resubmits transactions
// without any fee increase.
type SimpleTxManager struct {
	Config // embed the config directly
	name   string

	l1Client AlgoBackend
	l        log.Logger
}

// NewSimpleTxManager initializes a new SimpleTxManager with the passed Config.
func NewSimpleTxManager(name string, l log.Logger, cfg Config, l1Client AlgoBackend) *SimpleTxManager {
	return &SimpleTxManager{
		name:     name,
		Config:   cfg,
		l1Client: l1Client,
		l:        l.New("service", name),
	}
}

// Send is used to publish a transaction repeatedly until it eventually confirms.
// The method may be canceled using the passed context.
//
// NOTE: Send should be called by AT MOST one caller at a time.
func (m *SimpleTxManager) Send(ctx context.Context, tx *opcrypto.SignedTxn) (*models.PendingTransactionInfoResponse, error) {

	// Initialize a wait group to track any spawned goroutines, and ensure
	// we properly clean up any dangling resources this method generates.
	// We assert that this is the case thoroughly in our unit tests.
	var wg sync.WaitGroup
	defer wg.Wait()

	// Initialize a subcontext for the goroutines spawned in this process.
	// The defer to cancel is done here (in reverse order of Wait) so that
	// the goroutines can exit before blocking on the wait group.
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	sendState := NewSendState()

	// Create a closure that will block on submitting the tx in the
	// background, returning the first successfully mined confirmation
	// back to the main event loop via confChan.
	confChan := make(chan *models.PendingTransactionInfoResponse, 1)
	sendTxAsync := func(signedTx *opcrypto.SignedTxn) {
		defer wg.Done()

		log := m.l.New("txid", signedTx.Txid)
		log.Info("publishing transaction")

		_, err := m.l1Client.SendTransaction(ctx, tx)
		sendState.ProcessSendError(err)
		if err != nil {
			if errors.Is(err, context.Canceled) {
				return
			}
			log.Error("unable to publish transaction", "err", err)
			if sendState.ShouldAbortImmediately() {
				log.Warn("Aborting transaction submission")
				cancel()
			}
			// TODO(conner): add retry?
			return
		}

		log.Info("transaction published successfully")

		// Wait for the transaction to be confirmed, reporting the confirmation
		// back to the main event loop if found.
		confirmation, err := m.waitConfirmed(ctx, tx, sendState)
		if err != nil {
			log.Debug("send tx failed", "err", err)
		}
		if confirmation != nil {
			// Use non-blocking select to ensure function can exit
			// if more than one confirmation is discovered.
			select {
			case confChan <- confirmation:
				log.Trace("send tx succeeded")
			default:
			}
		}
	}

	// Submit and wait for the confirmation at our submission attempt in
	// the background, before entering the event loop and waiting out the
	// resubmission timeout.
	wg.Add(1)
	go sendTxAsync(tx)

	ticker := time.NewTicker(m.ResubmissionTimeout)
	defer ticker.Stop()

	for {
		select {

		// Whenever a resubmission timeout has elapsed, bump the gas
		// price and publish a new transaction.
		case <-ticker.C:
			// Avoid republishing if we are waiting for confirmation on an
			// existing tx. This is primarily an optimization to reduce the
			// number of API calls we make, but also reduces the chances of
			// getting a false positive reading for ShouldAbortImmediately.
			if sendState.IsWaitingForConfirmation() {
				continue
			}

			// TODO we resubmit the transaction without changing it.
			// If we want to increase its fee or change anything else (validity
			// range, lease), this is the place to do it.
			wg.Add(1)
			go sendTxAsync(tx)

		// The passed context has been canceled, i.e. in the event of a
		// shutdown.
		case <-ctx.Done():
			return nil, ctx.Err()

		// The transaction has confirmed.
		case confirmation := <-confChan:
			return confirmation, nil
		}
	}
}

// waitConfirmed implements the core functionality of WaitConfirmed, with the option to
// pass in a SendState to record whether or not the transaction is confirmed.
func (m *SimpleTxManager) waitConfirmed(ctx context.Context, tx *opcrypto.SignedTxn, sendState *SendState) (*models.PendingTransactionInfoResponse, error) {
	queryTicker := time.NewTicker(m.ConfirmationQueryInterval)
	defer queryTicker.Stop()

	txid := tx.Txid

	for {
		info, err := m.l1Client.PendingTransactionInformation(ctx, txid)
		switch {
		case info != nil:
			if info.ConfirmedRound > 0 {
				if sendState != nil {
					sendState.TxMined(txid)
				}

				m.l.Info("Transaction confirmed", "txid", txid)
				return info, nil
			}

			if info.PoolError != "" {
				if sendState != nil {
					sendState.TxNotMined(txid)
				}

				m.l.Info("Pool error, transaction has been rejected", "txid", txid, "err", info.PoolError)
				return nil, fmt.Errorf("Pool error: %s", info.PoolError)
			}

		case err != nil:
			m.l.Trace("Confirmation retrieval failed", "txid", txid, "err", err)

		default:
			if sendState != nil {
				sendState.TxNotMined(txid)
			}
			m.l.Trace("Transaction not yet confirmed", "txid", txid)
		}

		select {
		case <-ctx.Done():
			m.l.Warn("context cancelled in waitConfirmed")
			return nil, ctx.Err()
		case <-queryTicker.C:
		}
	}
}
