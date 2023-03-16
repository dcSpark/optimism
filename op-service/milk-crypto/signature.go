/*
	MILKOMEDA TO OP-STACK MIGRATION NOTES

This package is a replacement of the op-service/crypto package.

It provides a L1 signing function factory. Returned functions sign L1 transactions
with given private key, assuming L1 == Algorand.

Interface is simplified compared to original, as on Algorand we don't need to adjust signing
process based on chain ID. Furthermore, we currently only support signing with a private key
passed down from a config.

TODOs
(low priority)	support using signing services (wallets)
*/
package crypto

import (
	"encoding/hex"

	"github.com/algorand/go-algorand-sdk/crypto"
	"github.com/algorand/go-algorand-sdk/types"
)

type SignedTxn struct {
	Txid   string
	Txn    types.Transaction
	RawTxn []byte
}

type SignerFn func(types.Transaction) (*SignedTxn, error)

func CreateSignerFn(privateKey string) (fn SignerFn, address string, err error) {
	pkRaw, err := hex.DecodeString(privateKey)
	if err != nil {
		return nil, "", err
	}

	acc, err := crypto.AccountFromPrivateKey(pkRaw)
	if err != nil {
		return nil, "", err
	}

	var signer SignerFn
	signer = func(txn types.Transaction) (*SignedTxn, error) {
		txid, rawTxn, err := crypto.SignTransaction(acc.PrivateKey, txn)
		if err != nil {
			return nil, err
		}
		res := &SignedTxn{
			Txid:   txid,
			Txn:    txn,
			RawTxn: rawTxn,
		}
		return res, nil
	}

	return signer, acc.Address.String(), nil
}
