package algo

import (
	"github.com/algorand/go-algorand-sdk/transaction"
	"github.com/algorand/go-algorand-sdk/types"
)

func NewPaymentTransaction() (types.Transaction, error) {
	from := ""
	to := ""
	fee := uint64(transaction.MinTxnFee)
	amount := uint64(0)
	firstValid := uint64(0)
	lastValid := uint64(0)
	note := []byte{}
	crt := ""
	gid := ""
	ghash := []byte{}
	return transaction.MakePaymentTxn(from, to, fee, amount, firstValid, lastValid, note, crt, gid, ghash)
}
