package metrics

import "github.com/algorand/go-algorand-sdk/client/v2/common/models"

type NoopTxMetrics struct{}

func (*NoopTxMetrics) RecordPendingTx(int64)                              {}
func (*NoopTxMetrics) RecordBumpCount(int)                                {}
func (*NoopTxMetrics) RecordTxConfirmationLatency(int64)                  {}
func (*NoopTxMetrics) TxConfirmed(*models.PendingTransactionInfoResponse) {}
func (*NoopTxMetrics) TxPublished(string)                                 {}
func (*NoopTxMetrics) RPCError()                                          {}
