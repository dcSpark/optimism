package txmgr_test

import (
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/ethereum-optimism/optimism/op-service/txmgr"
)

var (
	testHash = "aabb"
)

func newSendState() *txmgr.SendState {
	return newSendStateWithTimeout(time.Hour, time.Now)
}

func newSendStateWithTimeout(t time.Duration, now func() time.Time) *txmgr.SendState {
	return txmgr.NewSendStateWithNow(t, now)
}

func processNSendErrors(sendState *txmgr.SendState, err error, n int) {
	for i := 0; i < n; i++ {
		sendState.ProcessSendError(err)
	}
}

// TestSendStateNoAbortAfterInit asserts that the default SendState won't
// trigger an abort even after the safe abort interval has elapsed.
func TestSendStateNoAbortAfterInit(t *testing.T) {
	sendState := newSendState()
	require.False(t, sendState.ShouldAbortImmediately())
	require.False(t, sendState.IsWaitingForConfirmation())
}

// TestSendStateNoAbortAfterProcessNilError asserts that nil errors are not
// considered for abort status.
func TestSendStateNoAbortAfterProcessNilError(t *testing.T) {
	sendState := newSendState()

	processNSendErrors(sendState, nil, 3)
	require.False(t, sendState.ShouldAbortImmediately())
}

// TestSendStateNoAbortAfterProcessOtherError asserts that non-nil errors other
// than ErrNonceTooLow are not considered for abort status.
func TestSendStateNoAbortAfterProcessOtherError(t *testing.T) {
	sendState := newSendState()

	otherError := errors.New("other error")
	processNSendErrors(sendState, otherError, 3)
	require.False(t, sendState.ShouldAbortImmediately())
}

// TestSendStateIsNotWaitingForConfirmationAfterTxUnmined asserts that we are
// not waiting for confirmation after a tx is mined then unmined.
func TestSendStateIsNotWaitingForConfirmationAfterTxUnmined(t *testing.T) {
	sendState := newSendState()

	sendState.TxMined(testHash)
	sendState.TxNotMined(testHash)
	require.False(t, sendState.IsWaitingForConfirmation())
}

func stepClock(step time.Duration) func() time.Time {
	i := 0
	return func() time.Time {
		var start time.Time
		i += 1
		return start.Add(time.Duration(i) * step)
	}
}

// TestSendStateTimeoutAbort ensure that this will abort if it passes the tx pool timeout
// when no successful transactions have been recorded
func TestSendStateTimeoutAbort(t *testing.T) {
	sendState := newSendStateWithTimeout(10*time.Millisecond, stepClock(20*time.Millisecond))
	require.True(t, sendState.ShouldAbortImmediately(), "Should abort after timing out")
}

// TestSendStateNoTimeoutAbortIfPublishedTx ensure that this will not abort if there is
// a successful transaction send.
func TestSendStateNoTimeoutAbortIfPublishedTx(t *testing.T) {
	sendState := newSendStateWithTimeout(10*time.Millisecond, stepClock(20*time.Millisecond))
	sendState.ProcessSendError(nil)
	require.False(t, sendState.ShouldAbortImmediately(), "Should not abort if published transcation successfully")
}
