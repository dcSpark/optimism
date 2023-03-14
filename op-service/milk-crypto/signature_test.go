package crypto

import (
	"bytes"
	"crypto/ed25519"
	"encoding/hex"
	"testing"

	"github.com/algorand/go-algorand-sdk/crypto"
	"github.com/algorand/go-algorand-sdk/encoding/msgpack"
	"github.com/algorand/go-algorand-sdk/types"
	"github.com/ethereum-optimism/optimism/op-service/milk-algo"

	"github.com/stretchr/testify/assert"
)

func TestPkSigner(t *testing.T) {
	account := crypto.GenerateAccount()
	tx, _ := algo.NewPaymentTransaction()
	tx.Sender = account.Address
	tx.Receiver = account.Address

	signer, addr, _ := CreateSignerFn(hex.EncodeToString(account.PrivateKey))
	if addr != account.Address.String() {
        t.Fatalf(`Did not recover signer address (%s vs %s)`, addr, account.Address)
    }

	signed, _ := signer(tx)

	decoded := types.SignedTxn{}
	msgpack.Decode(signed.RawTxn, &decoded)
	txRaw := bytes.Join([][]byte{[]byte("TX"), msgpack.Encode(tx)}, nil)
	assert.True(t, ed25519.Verify(account.PublicKey, txRaw, decoded.Sig[:]))
}
