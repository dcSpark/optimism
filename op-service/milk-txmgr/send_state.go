package txmgr

import (
	"sync"
)

// SendState is ready to track information about the publication state of
// a given txn. In this context, a txn *could* correspond to multiple different
// txn hashes due to resubmitting with different properties (fee, validity range),
// and we would treat them all as the same logical txn.
// TODO we currently only resubmit txn without any changes. If we decide to
// resubmit with different properties, we can track related state here.
type SendState struct {
	minedTxs map[string]struct{}
	mu       sync.RWMutex
}

// NewSendState parameterizes a new SendState from the passed
// safeAbortNonceTooLowCount.
func NewSendState() *SendState {
	return &SendState{
		minedTxs: make(map[string]struct{}),
	}
}

// ProcessSendError should be invoked with the error returned for each
// publication. It is safe to call this method with nil or arbitrary errors.
// Currently it does not act on any errors.
func (s *SendState) ProcessSendError(err error) {
	// Nothing to do.
	if err == nil {
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	// TODO originally this handled nonce too low errors and tracked the nonce
	// info in the SendState. We don't have nonces on Algorand, but maybe we do
	// have some kinds of errors on which we want to track some information
	// useful for further resubmissions,
	// Once we figure out what they are, handle the state update here.
}

// TxMined records that the txn with txid has been confirmed.
// It is safe to call this function multiple times.
func (s *SendState) TxMined(txid string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.minedTxs[txid] = struct{}{}
}

// TxNotMined records that the txn with txid has not been mined or has been
// reorg'd out. It is safe to call this function multiple times.
func (s *SendState) TxNotMined(txid string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	delete(s.minedTxs, txid)
}

// ShouldAbortImmediately returns true if the txmgr should give up on trying a
// given txn with the target nonce. For now this doesn't happen.
func (s *SendState) ShouldAbortImmediately() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()

	// Never abort if our latest sample reports having at least one mined txn.
	if len(s.minedTxs) > 0 {
		return false
	}

	// TODO
	// Originally this signaled abort if too many nonce too low errors observed.
	// We have no nonces on Algorand and currently no criterion to stop trying
	// to submit a batcher transaction. There likely is some good one to be found
	// though. When it is, implement it here.
	return false
}

// IsWaitingForConfirmation returns true if we have at least one confirmation on
// one of our txs.
func (s *SendState) IsWaitingForConfirmation() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return len(s.minedTxs) > 0
}
