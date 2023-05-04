package metrics

import (
	"github.com/algorand/go-algorand-sdk/client/v2/common/models"
	"github.com/ethereum-optimism/optimism/op-service/metrics"

	"github.com/prometheus/client_golang/prometheus"
)

type TxMetricer interface {
	RecordBumpCount(int)
	RecordTxConfirmationLatency(int64)
	RecordPendingTx(pending int64)
	TxConfirmed(*models.PendingTransactionInfoResponse)
	TxPublished(string)
	RPCError()
}

type TxMetrics struct {
	TxL1Fee            prometheus.Gauge
	txFees             prometheus.Counter
	TxBump             prometheus.Gauge
	txFeeHistogram     prometheus.Histogram
	LatencyConfirmedTx prometheus.Gauge
	pendingTxs         prometheus.Gauge
	txPublishError     *prometheus.CounterVec
	publishEvent       metrics.Event
	confirmEvent       metrics.EventVec
	rpcError           prometheus.Counter
}

func receiptStatusString(receipt *models.PendingTransactionInfoResponse) string {
	if receipt != nil && receipt.PoolError == "" && receipt.ConfirmedRound > 0 {
		return "success"
	} else if receipt.PoolError != "" {
		return "failed"
	} else {
		return "unknown_status"
	}
}

var _ TxMetricer = (*TxMetrics)(nil)

func MakeTxMetrics(ns string, factory metrics.Factory) TxMetrics {
	return TxMetrics{
		TxL1Fee: factory.NewGauge(prometheus.GaugeOpts{
			Namespace: ns,
			Name:      "tx_fee",
			Help:      "L1 fee for transactions in microalgo",
			Subsystem: "txmgr",
		}),
		txFees: factory.NewCounter(prometheus.CounterOpts{
			Namespace: ns,
			Name:      "tx_fee_ualgo_total",
			Help:      "Sum of fees spent for all transactions in microalgo",
			Subsystem: "txmgr",
		}),
		txFeeHistogram: factory.NewHistogram(prometheus.HistogramOpts{
			Namespace: ns,
			Name:      "tx_fee_histogram_ualgo",
			Help:      "Tx Fee in microalgo",
			Subsystem: "txmgr",
			Buckets:   []float64{0.5, 1, 2, 5, 10, 20, 40, 60, 80, 100, 200, 400, 800, 1600},
		}),
		TxBump: factory.NewGauge(prometheus.GaugeOpts{
			Namespace: ns,
			Name:      "tx_bump",
			Help:      "Number of times a transaction needed to be bumped before it got included",
			Subsystem: "txmgr",
		}),
		LatencyConfirmedTx: factory.NewGauge(prometheus.GaugeOpts{
			Namespace: ns,
			Name:      "tx_confirmed_latency_ms",
			Help:      "Latency of a confirmed transaction in milliseconds",
			Subsystem: "txmgr",
		}),
		pendingTxs: factory.NewGauge(prometheus.GaugeOpts{
			Namespace: ns,
			Name:      "pending_txs",
			Help:      "Number of transactions pending receipts",
			Subsystem: "txmgr",
		}),
		txPublishError: factory.NewCounterVec(prometheus.CounterOpts{
			Namespace: ns,
			Name:      "tx_publish_error_count",
			Help:      "Count of publish errors. Labels are sanitized error strings",
			Subsystem: "txmgr",
		}, []string{"error"}),
		confirmEvent: metrics.NewEventVec(factory, ns, "txmgr", "confirm", "tx confirm", []string{"status"}),
		publishEvent: metrics.NewEvent(factory, ns, "txmgr", "publish", "tx publish"),
		rpcError: factory.NewCounter(prometheus.CounterOpts{
			Namespace: ns,
			Name:      "rpc_error_count",
			Help:      "Temporary: Count of RPC errors (like timeouts) that have occurred",
			Subsystem: "txmgr",
		}),
	}
}

func (t *TxMetrics) RecordPendingTx(pending int64) {
	t.pendingTxs.Set(float64(pending))
}

// TxConfirmed records lots of information about the confirmed transaction
func (t *TxMetrics) TxConfirmed(receipt *models.PendingTransactionInfoResponse) {
	fee := float64(receipt.Transaction.Txn.Fee)
	t.confirmEvent.Record(receiptStatusString(receipt))
	t.TxL1Fee.Set(fee)
	t.txFees.Add(fee)
	t.txFeeHistogram.Observe(fee)

}

func (t *TxMetrics) RecordBumpCount(times int) {
	t.TxBump.Set(float64(times))
}

func (t *TxMetrics) RecordTxConfirmationLatency(latency int64) {
	t.LatencyConfirmedTx.Set(float64(latency))
}

func (t *TxMetrics) TxPublished(errString string) {
	if errString != "" {
		t.txPublishError.WithLabelValues(errString).Inc()
	} else {
		t.publishEvent.Record()
	}
}

func (t *TxMetrics) RPCError() {
	t.rpcError.Inc()
}
