/*
	MILKOMEDA TO OP-STACK MIGRATION NOTES

This package is a gradually built analogue to the op-node/eth package.

Note that the eth package in general is to stay, as layer 2 is still eth and will
need some eth utils.
*/
package algo

import (
	"fmt"
)

type BlockID struct {
	Hash   string `json:"hash"`
	Number uint64      `json:"number"`
}

func (id BlockID) String() string {
	return fmt.Sprintf("%s:%d", id.Hash, id.Number)
}

// TerminalString implements log.TerminalStringer, formatting a string for console
// output during logging.
func (id BlockID) TerminalString() string {
	return fmt.Sprintf("%s:%d", id.Hash, id.Number)
}

type L1BlockRef struct {
	Hash       string `json:"hash"`
	Number     uint64      `json:"number"`
	ParentHash string`json:"parentHash"`
	Time       uint64      `json:"timestamp"`
}

func (id L1BlockRef) String() string {
	return fmt.Sprintf("%s:%d", id.Hash, id.Number)
}

// TerminalString implements log.TerminalStringer, formatting a string for console
// output during logging.
func (id L1BlockRef) TerminalString() string {
	return fmt.Sprintf("%s:%d", id.Hash, id.Number)
}

func (id L1BlockRef) ID() BlockID {
	return BlockID{
		Hash:   id.Hash,
		Number: id.Number,
	}
}

func (id L1BlockRef) ParentID() BlockID {
	n := id.ID().Number
	// Saturate at 0 with subtraction
	if n > 0 {
		n -= 1
	}
	return BlockID{
		Hash:   id.ParentHash,
		Number: n,
	}
}

