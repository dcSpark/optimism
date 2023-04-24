package txmgr

import (
	"errors"
	"fmt"
	"time"

	"github.com/algorand/go-algorand-sdk/types"
	opservice "github.com/ethereum-optimism/optimism/op-service"
	opcrypto "github.com/ethereum-optimism/optimism/op-service/milk-crypto"
	"github.com/ethereum-optimism/optimism/op-signer/client"
	"github.com/ethereum/go-ethereum/log"
	"github.com/urfave/cli"
)

const (
	// Duplicated L1 RPC flag
	L1RPCUrlFlagName   = "l1-eth-rpc-url"
	L1RPCTokenFlagName = "l1-eth-rpc-token"
	// Key Management Flags (also have op-signer client flags)
	MnemonicFlagName   = "mnemonic"
	HDPathFlagName     = "hd-path"
	PrivateKeyFlagName = "private-key"
	// TxMgr Flags (new + legacy + some shared flags)
	ResubmissionTimeoutFlagName   = "resubmission-timeout"
	NetworkTimeoutFlagName        = "network-timeout"
	TxSendTimeoutFlagName         = "txmgr.send-timeout"
	TxNotInMempoolTimeoutFlagName = "txmgr.not-in-mempool-timeout"
	ReceiptQueryIntervalFlagName  = "txmgr.receipt-query-interval"
)

var (
	SequencerHDPathFlag = cli.StringFlag{
		Name: "sequencer-hd-path",
		Usage: "DEPRECATED: The HD path used to derive the sequencer wallet from the " +
			"mnemonic. The mnemonic flag must also be set.",
		EnvVar: "OP_BATCHER_SEQUENCER_HD_PATH",
	}
	L2OutputHDPathFlag = cli.StringFlag{
		Name: "l2-output-hd-path",
		Usage: "DEPRECATED:The HD path used to derive the l2output wallet from the " +
			"mnemonic. The mnemonic flag must also be set.",
		EnvVar: "OP_PROPOSER_L2_OUTPUT_HD_PATH",
	}
)

func CLIFlags(envPrefix string) []cli.Flag {
	return append([]cli.Flag{
		cli.StringFlag{
			Name:   MnemonicFlagName,
			Usage:  "The mnemonic used to derive the wallets for either the service",
			EnvVar: opservice.PrefixEnvVar(envPrefix, "MNEMONIC"),
		},
		cli.StringFlag{
			Name:   HDPathFlagName,
			Usage:  "The HD path used to derive the sequencer wallet from the mnemonic. The mnemonic flag must also be set.",
			EnvVar: opservice.PrefixEnvVar(envPrefix, "HD_PATH"),
		},
		SequencerHDPathFlag,
		L2OutputHDPathFlag,
		cli.StringFlag{
			Name:   "private-key",
			Usage:  "The private key to use with the service. Must not be used with mnemonic.",
			EnvVar: opservice.PrefixEnvVar(envPrefix, "PRIVATE_KEY"),
		},
		cli.DurationFlag{
			Name:   ResubmissionTimeoutFlagName,
			Usage:  "Duration we will wait before resubmitting a transaction to L1",
			Value:  48 * time.Second,
			EnvVar: opservice.PrefixEnvVar(envPrefix, "RESUBMISSION_TIMEOUT"),
		},
		cli.DurationFlag{
			Name:   NetworkTimeoutFlagName,
			Usage:  "Timeout for all network operations",
			Value:  2 * time.Second,
			EnvVar: opservice.PrefixEnvVar(envPrefix, "NETWORK_TIMEOUT"),
		},
		cli.DurationFlag{
			Name:   TxSendTimeoutFlagName,
			Usage:  "Timeout for sending transactions. If 0 it is disabled.",
			Value:  0,
			EnvVar: opservice.PrefixEnvVar(envPrefix, "TXMGR_TX_SEND_TIMEOUT"),
		},
		cli.DurationFlag{
			Name:   TxNotInMempoolTimeoutFlagName,
			Usage:  "Timeout for aborting a tx send if the tx does not make it to the mempool.",
			Value:  2 * time.Minute,
			EnvVar: opservice.PrefixEnvVar(envPrefix, "TXMGR_TX_NOT_IN_MEMPOOL_TIMEOUT"),
		},
		cli.DurationFlag{
			Name:   ReceiptQueryIntervalFlagName,
			Usage:  "Frequency to poll for receipts",
			Value:  12 * time.Second,
			EnvVar: opservice.PrefixEnvVar(envPrefix, "TXMGR_RECEIPT_QUERY_INTERVAL"),
		},
	}, client.CLIFlags(envPrefix)...)
}

type CLIConfig struct {
	L1RPCURL              string
	L1RPCToken            string
	Mnemonic              string
	HDPath                string
	SequencerHDPath       string
	L2OutputHDPath        string
	PrivateKey            string
	SignerCLIConfig       client.CLIConfig
	ResubmissionTimeout   time.Duration
	ReceiptQueryInterval  time.Duration
	NetworkTimeout        time.Duration
	TxSendTimeout         time.Duration
	TxNotInMempoolTimeout time.Duration
}

func (m CLIConfig) Check() error {
	if m.L1RPCURL == "" {
		return errors.New("must provide a L1 RPC url")
	}
	if m.NetworkTimeout == 0 {
		return errors.New("must provide NetworkTimeout")
	}
	if m.ResubmissionTimeout == 0 {
		return errors.New("must provide ResubmissionTimeout")
	}
	if m.ReceiptQueryInterval == 0 {
		return errors.New("must provide ReceiptQueryInterval")
	}
	if m.TxNotInMempoolTimeout == 0 {
		return errors.New("must provide TxNotInMempoolTimeout")
	}
	if err := m.SignerCLIConfig.Check(); err != nil {
		return err
	}
	return nil
}

func ReadCLIConfig(ctx *cli.Context) CLIConfig {
	return CLIConfig{
		L1RPCURL:              ctx.GlobalString(L1RPCUrlFlagName),
		L1RPCToken:            ctx.GlobalString(L1RPCTokenFlagName),
		Mnemonic:              ctx.GlobalString(MnemonicFlagName),
		HDPath:                ctx.GlobalString(HDPathFlagName),
		SequencerHDPath:       ctx.GlobalString(SequencerHDPathFlag.Name),
		L2OutputHDPath:        ctx.GlobalString(L2OutputHDPathFlag.Name),
		PrivateKey:            ctx.GlobalString(PrivateKeyFlagName),
		SignerCLIConfig:       client.ReadCLIConfig(ctx),
		ResubmissionTimeout:   ctx.GlobalDuration(ResubmissionTimeoutFlagName),
		ReceiptQueryInterval:  ctx.GlobalDuration(ReceiptQueryIntervalFlagName),
		NetworkTimeout:        ctx.GlobalDuration(NetworkTimeoutFlagName),
		TxSendTimeout:         ctx.GlobalDuration(TxSendTimeoutFlagName),
		TxNotInMempoolTimeout: ctx.GlobalDuration(TxNotInMempoolTimeoutFlagName),
	}
}

func NewConfig(cfg CLIConfig, l log.Logger) (Config, error) {
	if err := cfg.Check(); err != nil {
		return Config{}, fmt.Errorf("invalid config: %w", err)
	}

	l1, err := NewAlgodClient(cfg.L1RPCURL, cfg.L1RPCToken)
	if err != nil {
		return Config{}, fmt.Errorf("could not dial algod client: %w", err)
	}

	signerFn, from, err := opcrypto.CreateSignerFn(cfg.PrivateKey)
	if err != nil {
		return Config{}, fmt.Errorf("could not init signer: %w", err)
	}

	return Config{
		Backend:               l1,
		ResubmissionTimeout:   cfg.ResubmissionTimeout,
		TxSendTimeout:         cfg.TxSendTimeout,
		TxNotInMempoolTimeout: cfg.TxNotInMempoolTimeout,
		NetworkTimeout:        cfg.NetworkTimeout,
		ReceiptQueryInterval:  cfg.ReceiptQueryInterval,
		Signer:                signerFn,
		From:                  from,
	}, nil
}

// Config houses parameters for altering the behavior of a SimpleTxManager.
type Config struct {
	Backend AlgoBackend
	// ResubmissionTimeout is the interval at which, if no previously
	// published transaction has been mined, the new tx with a bumped gas
	// price will be published. Only one publication at MaxGasPrice will be
	// attempted.
	ResubmissionTimeout time.Duration

	// TxSendTimeout is how long to wait for sending a transaction.
	// By default it is unbounded. If set, this is recommended to be at least 20 minutes.
	TxSendTimeout time.Duration

	// TxNotInMempoolTimeout is how long to wait before aborting a transaction send if the transaction does not
	// make it to the mempool. If the tx is in the mempool, TxSendTimeout is used instead.
	TxNotInMempoolTimeout time.Duration

	// NetworkTimeout is the allowed duration for a single network request.
	// This is intended to be used for network requests that can be replayed.
	NetworkTimeout time.Duration

	// RequireQueryInterval is the interval at which the tx manager will
	// query the backend to check for confirmations after a tx at a
	// specific gas price has been published.
	ReceiptQueryInterval time.Duration

	// Signer is used to sign transactions when the gas price is increased.
	Signer opcrypto.SignerFn
	From   types.Address
}
